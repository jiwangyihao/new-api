package controller

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubscriptionBalancePurchaseTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	model.ClearSubscriptionPlanCacheForTest()
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.UserSubscription{}, &model.TimedSubscriptionValuationGrant{}, &model.CreditBalanceLedger{}, &model.Log{}, &model.TopUp{}, &model.InvitationRewardEvent{}, &model.InvitationMonthlyEntitlement{}))
}

func enableCreditValuationRuntimeForControllerTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(
		&model.CreditValuationState{},
		&model.CreditValuationMigration{},
		&model.SubscriptionPreConsumeRecord{},
	))
	require.NoError(t, model.DB.Create(&model.CreditValuationMigration{
		Version:           1,
		Status:            model.CreditValuationMigrationReady,
		ValuationCurrency: "CNY",
	}).Error)
}

func performBalancePayRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	if !strings.Contains(body, `"purchase_mode"`) {
		body = strings.Replace(body, "{", `{"purchase_mode":"timed",`, 1)
	}
	return performRawBalancePayRequest(t, userID, body)
}

func performRawBalancePayRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/balance/pay", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	SubscriptionRequestBalance(ctx)
	return recorder
}

func TestSubscriptionBalancePayRequiresExplicitPurchaseMode(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9500
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "explicit_purchase_mode", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	code := "explicit-purchase-mode"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9500, Title: "Explicit", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

	missing := performRawBalancePayRequest(t, userID, `{"plan_id":9500,"idempotency_key":"missing-mode"}`)
	invalid := performRawBalancePayRequest(t, userID, `{"plan_id":9500,"purchase_mode":"wallet","idempotency_key":"invalid-mode"}`)

	assert.Contains(t, missing.Body.String(), "购买模式必须明确选择计时套餐或 Credit 余额")
	assert.Contains(t, invalid.Body.String(), "购买模式必须明确选择计时套餐或 Credit 余额")
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 10000, user.Quota)
	var orderCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ?", userID).Count(&orderCount).Error)
	assert.Zero(t, orderCount)
}

func TestSubscriptionBalancePayRejectsDisabledPlan(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9506
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "disabled_plan_buyer", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	code := "disabled-plan-purchase"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9507, Title: "Stopped", PriceAmount: 40, Currency: "CNY", Enabled: false, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9507).Update("enabled", false).Error)
	model.InvalidateSubscriptionPlanCache(9507)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9507,"idempotency_key":"disabled-plan"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "套餐未启用")
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 10000, user.Quota)
	var orderCount, subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ?", userID).Count(&orderCount).Error)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&subscriptionCount).Error)
	assert.Zero(t, orderCount)
	assert.Zero(t, subscriptionCount)
}

func TestSubscriptionBalancePayAmountUsesCents(t *testing.T) {
	amount, err := subscriptionBalancePayAmount(39.9)
	require.NoError(t, err)
	assert.Equal(t, 3990, amount)

	old := common.QuotaPerUnit
	common.QuotaPerUnit = 999999
	t.Cleanup(func() { common.QuotaPerUnit = old })
	amount, err = subscriptionBalancePayAmount(40)
	require.NoError(t, err)
	assert.Equal(t, 4000, amount)
}

func TestSubscriptionBalancePayCreatesSubscriptionAndDeductsBalance(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9501
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_buyer", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	code := "balance-basic"
	plan := seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{Id: 9502, Title: "Basic", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}, 40_000_000)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9502,"idempotency_key":"balance-key-1"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 6000, user.Quota)
	var sub model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9502).First(&sub).Error)
	assert.Equal(t, "order", sub.GrantReason)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9502).First(&order).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, model.PaymentProviderBalance, order.PaymentProvider)
	assert.Equal(t, model.PaymentMethodAccountBalance, order.PaymentMethod)
	assert.Equal(t, 40.0, order.Money)
	assert.NotZero(t, order.CompleteTime)
	assert.Equal(t, "BALSUBUSR9501NO"+common.Sha1([]byte("balance-key-1")), order.TradeNo)
	assertAuthorizedTimedOrderSnapshotFixture(t, &order, &plan)
	var topUpCount int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", userID).Count(&topUpCount).Error)
	assert.Equal(t, int64(0), topUpCount)

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeTopup).First(&log).Error)
	assert.Contains(t, log.Content, "账户余额购买订阅套餐：Basic")
}

func TestSubscriptionBalancePayCreditModeAtomicallyCreditsUniqueBalance(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	enableCreditValuationRuntimeForControllerTest(t)
	userID := 9581
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "credit_balance_buyer", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	priceMicros := int64(40_000_000)
	optionCode := "credit_balance_option"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:                       9582,
		Title:                    "Credit option",
		PriceAmount:              40,
		PriceAmountMicros:        &priceMicros,
		Currency:                 "CNY",
		DurationUnit:             model.SubscriptionDurationMonth,
		DurationValue:            1,
		QuotaResetPeriod:         model.SubscriptionResetMonthly,
		MonthlyTokenLimit:        1000,
		ConcurrencyLimit:         1,
		Enabled:                  true,
		PublicVisible:            true,
		BusinessCode:             &optionCode,
		UnlimitedPurchaseEnabled: true,
	}).Error)
	creditCode := "credit_balance_global"
	valuationCurrency := "CNY"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:                           9583,
		Title:                        "Credit 余额套餐",
		Currency:                     "CNY",
		ValuationCurrency:            &valuationCurrency,
		EntitlementType:              model.SubscriptionEntitlementCreditBalance,
		Enabled:                      true,
		PublicVisible:                false,
		RewardEligible:               false,
		BusinessCode:                 &creditCode,
		CreditBalanceConfigured:      true,
		CreditBalancePurchaseEnabled: true,
	}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9582,"purchase_mode":"credit_balance","idempotency_key":"credit-balance-first"}`)
	replay := performBalancePayRequest(t, userID, `{"plan_id":9582,"purchase_mode":"credit_balance","idempotency_key":"credit-balance-first"}`)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	assert.Contains(t, replay.Body.String(), `"message":"success"`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 6000, user.Quota)
	assert.Equal(t, model.SubscriptionEntitlementCreditBalance, user.GetSetting().LastSubscriptionPurchaseMode)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, 9583, balance.PlanId)
	assert.Equal(t, int64(1000), balance.TokenLimit)
	assert.Equal(t, int64(0), balance.TokenUsed)
	assert.Equal(t, int64(0), balance.EndTime)
	assert.Equal(t, balance.Id, user.GetSetting().ActiveSubscriptionId)
	var timedCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementTimed).Count(&timedCount).Error)
	assert.Zero(t, timedCount)

	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9582).First(&order).Error)
	snapshot, err := model.UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
	require.NoError(t, err)
	require.NotNil(t, snapshot.ListPriceMicros)
	assert.Equal(t, int64(40_000_000), *snapshot.ListPriceMicros)
	assert.Equal(t, int64(1000), snapshot.MonthlyTokenLimit)
	assert.Equal(t, "CNY", snapshot.ListPriceCurrency)
	assert.Equal(t, 9583, snapshot.TargetCreditBalancePlanID)
	assert.Equal(t, "CNY", snapshot.TargetCreditBalanceValuationCurrency)
	assert.Equal(t, model.PaymentProviderBalance, snapshot.PaymentProvider)
	assert.Equal(t, model.PaymentMethodAccountBalance, snapshot.ProviderPaymentMethod)
	assert.Equal(t, int64(4000), snapshot.PaymentAmountCents)
	assert.Equal(t, "CNY", snapshot.PaymentCurrency)
	assert.Equal(t, model.CreditValuationRuleVersion, snapshot.ValuationRuleVersion)

	var state model.CreditValuationState
	require.NoError(t, model.DB.First(&state, balance.Id).Error)
	assert.Equal(t, int64(1000), state.AvailableCredit)
	assert.Equal(t, int64(40_000_000), state.ExactCostMicros)
	assert.Zero(t, state.EstimatedCostMicros)
	assert.Zero(t, state.UnknownCredit)
	assert.Equal(t, "CNY", state.Currency)
	assert.Equal(t, int64(1), state.StateVersion)
	var orderCount, ledgerCount, stateCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ?", userID).Count(&orderCount).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", userID).Count(&ledgerCount).Error)
	require.NoError(t, model.DB.Model(&model.CreditValuationState{}).Where("user_id = ?", userID).Count(&stateCount).Error)
	assert.Equal(t, int64(1), orderCount)
	assert.Equal(t, int64(1), ledgerCount)
	assert.Equal(t, int64(1), stateCount)
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 6000, user.Quota)
}

func TestSubscriptionBalancePayCreditModeRollsBackEveryWriteOnLedgerFailure(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	enableCreditValuationRuntimeForControllerTest(t)
	const userID = 9584
	const optionPlanID = 9585
	const creditPlanID = 9586
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "credit_balance_rollback", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	priceMicros := int64(40_000_000)
	valuationCurrency := "CNY"
	optionCode := "credit_balance_rollback_option"
	creditCode := "credit_balance_rollback_global"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: optionPlanID, Title: "Credit rollback", PriceAmount: 40, PriceAmountMicros: &priceMicros, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, Enabled: true, PublicVisible: true, BusinessCode: &optionCode, UnlimitedPurchaseEnabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", Currency: "CNY", ValuationCurrency: &valuationCurrency, EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true, BusinessCode: &creditCode}).Error)
	require.NoError(t, model.DB.Exec(`CREATE TRIGGER reject_credit_balance_ledger_insert BEFORE INSERT ON credit_balance_ledgers BEGIN SELECT RAISE(FAIL, 'injected ledger failure'); END`).Error)

	recorder := performRawBalancePayRequest(t, userID, `{"plan_id":9585,"purchase_mode":"credit_balance","idempotency_key":"credit-rollback"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), `"message":"success"`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 10000, user.Quota)
	assert.Zero(t, user.GetSetting().ActiveSubscriptionId)
	assert.Empty(t, user.GetSetting().LastSubscriptionPurchaseMode)
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ?", userID).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", userID).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, model.DB.Model(&model.CreditValuationState{}).Where("user_id = ?", userID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreditBalanceCompletionSupportsExternalPaymentProviders(t *testing.T) {
	for index, provider := range []string{model.PaymentProviderStripe, model.PaymentProviderCreem, model.PaymentProviderKyren, model.PaymentProviderEpay} {
		t.Run(provider, func(t *testing.T) {
			setupSubscriptionBalancePurchaseTestDB(t)
			userID := 9600 + index
			optionPlanID := 9610 + index
			creditPlanID := 9620 + index
			require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "external_credit_" + provider, Status: common.UserStatusEnabled}).Error)
			optionCode := fmt.Sprintf("external_credit_option_%d", index)
			creditCode := fmt.Sprintf("external_credit_global_%d", index)
			optionPlan := &model.SubscriptionPlan{Id: optionPlanID, Title: "External Credit", PriceAmount: 40, Currency: "CNY", MonthlyTokenLimit: 1000, BusinessCode: &optionCode, Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetMonthly, UnlimitedPurchaseEnabled: true}
			require.NoError(t, model.DB.Create(optionPlan).Error)
			require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true, BusinessCode: &creditCode}).Error)
			paymentMethod := map[string]string{
				model.PaymentProviderStripe: model.PaymentMethodStripe,
				model.PaymentProviderCreem:  model.PaymentMethodCreem,
				model.PaymentProviderKyren:  model.PaymentMethodKyren,
				model.PaymentProviderEpay:   "alipay",
			}[provider]
			snapshot, err := prepareExternalSubscriptionEntitlementSnapshot(optionPlan, model.SubscriptionPurchaseModeCreditBalance)
			require.NoError(t, err)
			serialized, err := marshalExternalSubscriptionEntitlementSnapshot(snapshot, provider, "", paymentMethod, 4000, "CNY")
			require.NoError(t, err)
			order := model.SubscriptionOrder{UserId: userID, PlanId: optionPlanID, Money: 40, AmountCents: 4000, Currency: "CNY", CreditGrantAmount: 1000, CreditTargetPlanID: creditPlanID, TradeNo: fmt.Sprintf("external-credit-%d", index), PaymentMethod: paymentMethod, PaymentProvider: provider, Status: common.TopUpStatusPending, EntitlementSnapshot: serialized, CreateTime: common.GetTimestamp()}
			require.NoError(t, model.DB.Create(&order).Error)

			require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
				_, completeErr := model.CompleteSubscriptionOrderTx(tx, &order, "", paymentMethod)
				return completeErr
			}))

			require.NoError(t, model.DB.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusSuccess, order.Status)
			var count int64
			require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
			assert.Equal(t, int64(1), count)
			require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", userID).Count(&count).Error)
			assert.Equal(t, int64(1), count)
			var topUp model.TopUp
			require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&topUp).Error)
			assert.Equal(t, provider, topUp.PaymentProvider)
		})
	}
}

func TestCreditBalanceCompletionRejectsTamperedEntitlementIdentity(t *testing.T) {
	for index, test := range []struct {
		name   string
		mutate func(*model.SubscriptionEntitlementSnapshot, *model.SubscriptionOrder)
	}{
		{
			name: "grant amount",
			mutate: func(snapshot *model.SubscriptionEntitlementSnapshot, _ *model.SubscriptionOrder) {
				snapshot.MonthlyTokenLimit++
			},
		},
		{
			name: "target plan",
			mutate: func(_ *model.SubscriptionEntitlementSnapshot, order *model.SubscriptionOrder) {
				order.CreditTargetPlanID++
			},
		},
		{
			name: "source entitlement type",
			mutate: func(snapshot *model.SubscriptionEntitlementSnapshot, _ *model.SubscriptionOrder) {
				snapshot.PlanEntitlementType = model.SubscriptionEntitlementCreditBalance
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupSubscriptionBalancePurchaseTestDB(t)
			const userID = 9650
			const optionPlanID = 9651
			const creditPlanID = 9652
			require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "tampered_credit_snapshot", Status: common.UserStatusEnabled}).Error)
			optionCode := "tampered_credit_option"
			creditCode := "tampered_credit_global"
			optionPlan := &model.SubscriptionPlan{Id: optionPlanID, Title: "Tamper option", PriceAmount: 40, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, Enabled: true, PublicVisible: true, BusinessCode: &optionCode, UnlimitedPurchaseEnabled: true}
			require.NoError(t, model.DB.Create(optionPlan).Error)
			require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true, BusinessCode: &creditCode}).Error)
			snapshot, err := prepareExternalSubscriptionEntitlementSnapshot(optionPlan, model.SubscriptionPurchaseModeCreditBalance)
			require.NoError(t, err)
			order := model.SubscriptionOrder{UserId: userID, PlanId: optionPlanID, Money: 40, AmountCents: 4000, Currency: "CNY", CreditGrantAmount: 1000, CreditTargetPlanID: creditPlanID, TradeNo: fmt.Sprintf("tampered-credit-%d", index), PaymentMethod: model.PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
			test.mutate(&snapshot, &order)
			serialized, err := marshalExternalSubscriptionEntitlementSnapshot(snapshot, model.PaymentProviderStripe, "price_tampered", model.PaymentMethodStripe, 4000, "CNY")
			require.NoError(t, err)
			order.EntitlementSnapshot = serialized
			require.NoError(t, model.DB.Create(&order).Error)

			err = model.DB.Transaction(func(tx *gorm.DB) error {
				_, completeErr := model.CompleteSubscriptionOrderTx(tx, &order, "", model.PaymentMethodStripe)
				return completeErr
			})

			require.ErrorIs(t, err, model.ErrSubscriptionOrderSnapshotMismatch)
			require.NoError(t, model.DB.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
			var count int64
			require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
			assert.Zero(t, count)
			require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestSubscriptionBalancePayCreditModePreservesUsableTimedActiveSelection(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9580
	optionCode := "credit_balance_preserve_option"
	creditCode := "credit_balance_preserve_global"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9577, Title: "Timed active", PriceAmount: 40, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 500, ConcurrencyLimit: 1, Enabled: true, PublicVisible: true, BusinessCode: &optionCode, UnlimitedPurchaseEnabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9578, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true, BusinessCode: &creditCode}).Error)
	timed := model.UserSubscription{Id: 9579, UserId: userID, PlanId: 9577, EntitlementType: model.SubscriptionEntitlementTimed, Status: "active", TokenLimit: 500, TokenUsed: 100, EndTime: common.GetTimestamp() + 3600, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}
	user := model.User{Id: userID, Username: "credit_balance_preserve", Quota: 10000, Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = timed.Id
	user.SetSetting(setting)
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&timed).Error)

	recorder := performRawBalancePayRequest(t, userID, `{"plan_id":9577,"purchase_mode":"credit_balance","idempotency_key":"credit-preserve-active"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, timed.Id, user.GetSetting().ActiveSubscriptionId)
	assert.Contains(t, recorder.Body.String(), `"active":false`)
}

func TestSubscriptionBalancePayCreditModeAccumulatesAndOffsetsDebt(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9591
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "credit_balance_repeat", Quota: 20000, Status: common.UserStatusEnabled}).Error)
	optionCode := "credit_balance_repeat_option"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9592, Title: "Credit repeat", PriceAmount: 40, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, Enabled: true, PublicVisible: true, BusinessCode: &optionCode, UnlimitedPurchaseEnabled: true}).Error)
	creditCode := "credit_balance_repeat_global"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9593, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true, BusinessCode: &creditCode}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9594, UserId: userID, PlanId: 9593, EntitlementType: model.SubscriptionEntitlementCreditBalance, Status: "active", TokenLimit: 100, TokenUsed: 350, EndTime: 0, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)

	first := performRawBalancePayRequest(t, userID, `{"plan_id":9592,"purchase_mode":"credit_balance","idempotency_key":"credit-repeat-one"}`)
	second := performRawBalancePayRequest(t, userID, `{"plan_id":9592,"purchase_mode":"credit_balance","idempotency_key":"credit-repeat-two"}`)

	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, first.Body.String(), `"gross_credit":1000`)
	assert.Contains(t, first.Body.String(), `"debt_offset":250`)
	assert.Contains(t, first.Body.String(), `"available_credit":750`)
	assert.Contains(t, second.Body.String(), `"gross_credit":1000`)
	assert.Contains(t, second.Body.String(), `"debt_offset":0`)
	assert.Contains(t, second.Body.String(), `"available_credit":1750`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 12000, user.Quota)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(2100), balance.TokenLimit)
	assert.Equal(t, int64(350), balance.TokenUsed)
	var count int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", userID).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestCreditBalanceLedgerScopesIdempotencyKeyPerUser(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.CreditBalanceLedger{}))
	base := model.CreditBalanceLedger{
		UserSubscriptionId:   1,
		Type:                 model.CreditBalanceLedgerTypePurchase,
		IdempotencyKey:       "shared-key",
		SourceType:           model.CreditBalanceLedgerSourceSubscriptionOrder,
		GrossCredit:          100,
		BalanceBefore:        0,
		BalanceAfter:         100,
		AvailableCreditAfter: 100,
		CreatedAt:            common.GetTimestamp(),
	}
	first := base
	first.UserId = 1
	first.SourceId = 1
	require.NoError(t, model.DB.Create(&first).Error)
	secondUser := base
	secondUser.UserId = 2
	secondUser.SourceId = 2
	require.NoError(t, model.DB.Create(&secondUser).Error)
	sameUser := base
	sameUser.UserId = 1
	sameUser.SourceId = 3
	require.Error(t, model.DB.Create(&sameUser).Error)
	require.True(t, model.DB.Migrator().HasIndex(&model.CreditBalanceLedger{}, "idx_credit_balance_ledger_user_key"))
	updateErr := model.DB.Model(&first).Update("gross_credit", 999).Error
	require.ErrorContains(t, updateErr, "immutable")
	deleteErr := model.DB.Delete(&first).Error
	require.ErrorContains(t, deleteErr, "immutable")
	var persisted model.CreditBalanceLedger
	require.NoError(t, model.DB.First(&persisted, first.Id).Error)
	assert.Equal(t, int64(100), persisted.GrossCredit)

}

func TestCreditBalanceGrantRejectsReplayWithMismatchedIdentity(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	const userID = 9595
	const creditPlanID = 9596
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "credit_target_replay", Status: common.UserStatusEnabled}).Error)
	businessCode := "credit_target_replay_plan"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, BusinessCode: &businessCode}).Error)
	request := model.CreditBalanceGrantRequest{
		UserId:         userID,
		GrossCredit:    1000,
		IdempotencyKey: "credit-target-replay",
		SourceType:     model.CreditBalanceLedgerSourceSubscriptionOrder,
		SourceId:       9597,
		Type:           model.CreditBalanceLedgerTypePurchase,
		TargetPlanId:   creditPlanID,
	}
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		_, err := model.GrantCreditBalanceTx(tx, request)
		return err
	}))

	request.IdempotencyKey = "credit-target-replay-different-key"
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		_, grantErr := model.GrantCreditBalanceTx(tx, request)
		return grantErr
	})
	require.ErrorContains(t, err, "idempotency key mismatch")

	request.IdempotencyKey = "credit-target-replay"
	request.TargetPlanId++
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		_, grantErr := model.GrantCreditBalanceTx(tx, request)
		return grantErr
	})
	require.ErrorContains(t, err, "target plan mismatch")

	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(1000), balance.TokenLimit)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", userID).Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
}

func TestSubscriptionBalancePayCreditReplayUsesStoredSnapshotAfterPlanChanges(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9587
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "credit_snapshot_replay", Quota: 20000, Status: common.UserStatusEnabled}).Error)
	optionCode := "credit_snapshot_replay"
	plan := &model.SubscriptionPlan{Id: 9588, Title: "Credit snapshot option", PriceAmount: 40, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, Enabled: true, PublicVisible: true, BusinessCode: &optionCode, UnlimitedPurchaseEnabled: true}
	require.NoError(t, model.DB.Create(plan).Error)
	creditCode := "credit_snapshot_global"
	const creditPlanID = 9589
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, ModelLimits: "gpt-4o,claude-3-7-sonnet", ConcurrencyLimit: 7, QueueCapacity: 13, GPTAbuseWarningLimit: 5, CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true, BusinessCode: &creditCode}).Error)

	firstResponse := performBalancePayRequest(t, userID, `{"plan_id":9588,"purchase_mode":"credit_balance","idempotency_key":"credit-snapshot-replay"}`)
	require.Equal(t, http.StatusOK, firstResponse.Code, firstResponse.Body.String())
	require.Contains(t, firstResponse.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", subscriptionBalanceTradeNo(userID, "credit-snapshot-replay")).First(&order).Error)
	storedSnapshot, err := model.UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
	require.NoError(t, err)
	assert.Equal(t, creditPlanID, storedSnapshot.TargetCreditBalancePlanID)
	assert.Equal(t, "Credit 余额套餐", storedSnapshot.TargetCreditBalanceTitle)
	assert.Equal(t, creditCode, storedSnapshot.TargetCreditBalanceBusinessCode)
	assert.Equal(t, "gpt-4o,claude-3-7-sonnet", storedSnapshot.TargetCreditBalanceModelLimits)
	assert.Equal(t, 7, storedSnapshot.TargetCreditBalanceConcurrencyLimit)
	assert.Equal(t, 13, storedSnapshot.TargetCreditBalanceQueueCapacity)
	assert.Equal(t, 5, storedSnapshot.TargetCreditBalanceGPTAbuseWarningLimit)
	assert.Equal(t, model.PaymentProviderBalance, storedSnapshot.PaymentProvider)
	assert.Equal(t, model.PaymentMethodAccountBalance, storedSnapshot.ProviderPaymentMethod)
	assert.Equal(t, int64(4000), storedSnapshot.PaymentAmountCents)
	assert.Equal(t, "CNY", storedSnapshot.PaymentCurrency)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{"title": "Changed title", "price_amount": 80, "monthly_token_limit": 9000, "enabled": false}).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)

	secondResponse := performRawBalancePayRequest(t, userID, `{"plan_id":9588,"purchase_mode":"credit_balance","idempotency_key":"credit-snapshot-replay"}`)

	require.Equal(t, http.StatusOK, secondResponse.Code, secondResponse.Body.String())
	assert.Contains(t, secondResponse.Body.String(), `"message":"success"`)
	assert.Contains(t, secondResponse.Body.String(), `"gross_credit":1000`)
	require.NoError(t, model.DB.Delete(&model.SubscriptionPlan{}, plan.Id).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)
	thirdResponse := performRawBalancePayRequest(t, userID, `{"plan_id":9588,"purchase_mode":"credit_balance","idempotency_key":"credit-snapshot-replay"}`)
	require.Equal(t, http.StatusOK, thirdResponse.Code, thirdResponse.Body.String())
	assert.Contains(t, thirdResponse.Body.String(), `"gross_credit":1000`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 16000, user.Quota)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(1000), balance.TokenLimit)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", userID).Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
}

func TestSubscriptionBalancePayRejectsUnverifiablePendingCreditOrder(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	setupSubscriptionControllerRedis(t)
	const userID = 9584
	const optionPlanID = 9585
	const creditPlanID = 9586
	user := model.User{Id: userID, Username: "pending_credit_replay", Quota: 6000, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&user).Error)
	seedUserCacheForSubscriptionControllerTest(t, user)
	optionCode := "pending_credit_replay_option"
	optionPlan := &model.SubscriptionPlan{Id: optionPlanID, Title: "Pending credit replay", PriceAmount: 40, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, Enabled: true, PublicVisible: true, BusinessCode: &optionCode, UnlimitedPurchaseEnabled: true}
	require.NoError(t, model.DB.Create(optionPlan).Error)
	creditCode := "pending_credit_replay_global"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: creditPlanID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true, BusinessCode: &creditCode}).Error)
	snapshot, err := model.MarshalSubscriptionEntitlementSnapshot(model.NewSubscriptionEntitlementSnapshot(optionPlan, model.SubscriptionPurchaseModeCreditBalance, creditPlanID))
	require.NoError(t, err)
	tradeNo := subscriptionBalanceTradeNo(userID, "pending-credit-replay")
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: userID, PlanId: optionPlanID, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: tradeNo, PaymentProvider: model.PaymentProviderBalance, PaymentMethod: model.PaymentMethodAccountBalance, Status: common.TopUpStatusPending, EntitlementSnapshot: snapshot, CreateTime: common.GetTimestamp()}).Error)

	first := performRawBalancePayRequest(t, userID, `{"plan_id":9585,"purchase_mode":"credit_balance","idempotency_key":"pending-credit-replay"}`)
	second := performRawBalancePayRequest(t, userID, `{"plan_id":9585,"purchase_mode":"credit_balance","idempotency_key":"pending-credit-replay"}`)

	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), "subscription order status invalid")
	assert.NotContains(t, first.Body.String(), `"message":"success"`)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), "subscription order status invalid")
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 6000, user.Quota)
	assert.Empty(t, user.GetSetting().LastSubscriptionPurchaseMode)
	assert.Zero(t, user.GetSetting().ActiveSubscriptionId)
	cachedUser, err := model.GetUserCache(userID)
	require.NoError(t, err)
	assert.Empty(t, cachedUser.GetSetting().LastSubscriptionPurchaseMode)
	assert.Zero(t, cachedUser.GetSetting().ActiveSubscriptionId)
	var count int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", userID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSubscriptionBalancePayRejectsLegacyOrderReplayWithExplicitCreditMode(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9584
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "legacy_replay", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	optionCode := "legacy_replay_option"
	plan := &model.SubscriptionPlan{Id: 9585, Title: "Legacy replay option", PriceAmount: 40, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, Enabled: true, PublicVisible: true, BusinessCode: &optionCode, UnlimitedPurchaseEnabled: true}
	require.NoError(t, model.DB.Create(plan).Error)
	creditCode := "legacy_replay_credit"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9586, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true, BusinessCode: &creditCode}).Error)
	tradeNo := subscriptionBalanceTradeNo(userID, "legacy-replay-key")
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: userID, PlanId: plan.Id, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: tradeNo, PaymentProvider: model.PaymentProviderBalance, PaymentMethod: model.PaymentMethodAccountBalance, Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp(), CompleteTime: common.GetTimestamp()}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9585,"purchase_mode":"credit_balance","idempotency_key":"legacy-replay-key"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "幂等键已绑定其他购买模式或订单快照")
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 10000, user.Quota)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
}

func TestSubscriptionBalancePurchaseStoresCNYAmountSnapshot(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9551
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance-snapshot", Status: common.UserStatusEnabled, Quota: 6000}).Error)
	code := "balance_snapshot"
	plan := seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{Id: 9552, Title: "Balance Snapshot", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}, 40_000_000)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9552,"idempotency_key":"snapshot"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9552).First(&order).Error)
	assert.Equal(t, int64(4000), order.AmountCents)
	assert.Equal(t, "CNY", order.Currency)
	assertAuthorizedTimedOrderSnapshotFixture(t, &order, &plan)
}

func TestSubscriptionBalancePurchaseInvokesInvitationRewardHandlerAndCreatesEvent(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	inviterID := 9571
	inviteeID := 9572
	planID := 9573
	model.InvalidateSubscriptionPlanCache(planID)
	t.Cleanup(func() { model.InvalidateSubscriptionPlanCache(planID) })
	inviter := model.User{Id: inviterID, Username: "balance-handler-inviter", Status: common.UserStatusEnabled, AffCode: "balance-handler-inviter", InvitationRewardMode: model.InvitationRewardModeSubscription}
	invitee := model.User{Id: inviteeID, Username: "balance-handler-invitee", Status: common.UserStatusEnabled, AffCode: "balance-handler-invitee", InviterId: inviterID, Quota: 10000}
	require.NoError(t, model.DB.Create(&inviter).Error)
	require.NoError(t, model.DB.Create(&invitee).Error)
	code := "balance_handler_event"
	seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{Id: planID, Title: "Balance Handler Event", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, RewardEligible: true, BusinessCode: &code}, 40_000_000)

	receivedOrderIDs := make([]int, 0, 1)
	SetInvitationRewardOrderHandlerForTest(t, func(orderId int) error {
		receivedOrderIDs = append(receivedOrderIDs, orderId)
		var order model.SubscriptionOrder
		require.NoError(t, model.DB.First(&order, orderId).Error)
		assert.Equal(t, common.TopUpStatusSuccess, order.Status)
		var event model.InvitationRewardEvent
		require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error)
		assert.Equal(t, order.Id, event.SourceOrderId)
		assert.Equal(t, order.AmountCents, event.SourceAmountCents)
		assert.Equal(t, order.Currency, event.SourceCurrency)
		return nil
	})

	recorder := performBalancePayRequest(t, inviteeID, `{"plan_id":9573,"idempotency_key":"balance-handler-event"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	require.Len(t, receivedOrderIDs, 1)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", inviteeID, planID).First(&order).Error)
	assert.Equal(t, []int{order.Id}, receivedOrderIDs)
	var event model.InvitationRewardEvent
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error)
	assert.Equal(t, inviterID, event.InviterId)
	assert.Equal(t, inviteeID, event.InviteeId)
	assert.Equal(t, order.Id, event.SourceOrderId)
	assert.Equal(t, int64(4000), event.SourceAmountCents)
	assert.Equal(t, "CNY", event.SourceCurrency)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestSubscriptionBalancePurchaseReturnsSuccessWhenInvitationRewardHandlerFails(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	inviteeID := 9574
	planID := 9575
	model.InvalidateSubscriptionPlanCache(planID)
	t.Cleanup(func() { model.InvalidateSubscriptionPlanCache(planID) })
	require.NoError(t, model.DB.Create(&model.User{Id: inviteeID, Username: "balance-handler-fail", Status: common.UserStatusEnabled, Quota: 10000}).Error)
	code := "balance_handler_failure"
	seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{Id: planID, Title: "Balance Handler Failure", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, RewardEligible: true, BusinessCode: &code}, 40_000_000)
	SetInvitationRewardOrderHandlerForTest(t, func(orderId int) error {
		return errors.New("temporary reward handler failure")
	})

	recorder := performBalancePayRequest(t, inviteeID, `{"plan_id":9575,"idempotency_key":"balance-handler-failure"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var user model.User
	require.NoError(t, model.DB.First(&user, inviteeID).Error)
	assert.Equal(t, 6000, user.Quota)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", inviteeID, planID).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
}

func TestSubscriptionBalancePurchaseRejectsFractionalCentPrice(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9553
	planID := 9554
	model.InvalidateSubscriptionPlanCache(planID)
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance-fractional-cent", Status: common.UserStatusEnabled, Quota: 10000}).Error)
	code := "balance_fractional_cent"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Fractional Cent", PriceAmount: 10.005, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9554,"idempotency_key":"fractional-cent-price"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), `"message":"success"`)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 10000, user.Quota)
	var orderCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", userID, planID).Count(&orderCount).Error)
	assert.Equal(t, int64(0), orderCount)
}

func TestSubscriptionBalancePayAllowsDecimalPlanPrice(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9505
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_decimal", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	code := "balance-decimal"
	seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{Id: 9506, Title: "Decimal", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}, 39_900_000)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9506,"idempotency_key":"balance-decimal"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 6010, user.Quota)
}

func TestSubscriptionBalancePayInsufficientBalanceDoesNotDeduct(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9511
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_low", Quota: 1000, Status: common.UserStatusEnabled}).Error)
	code := "balance-pro"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9512, Title: "Pro", PriceAmount: 160, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9512,"idempotency_key":"balance-low"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "余额不足")
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 1000, user.Quota)
	var orderCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", userID, 9512).Count(&orderCount).Error)
	assert.Equal(t, int64(0), orderCount)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9512).Count(&subCount).Error)
	assert.Equal(t, int64(0), subCount)
}

func TestSubscriptionBalancePayRejectsNonCNYPlanCurrency(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9513
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_usd", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	code := "balance-usd"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9514, Title: "USD Plan", PriceAmount: 40, Currency: "USD", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9514,"idempotency_key":"balance-usd"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "账户余额仅支持 CNY 套餐")
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 10000, user.Quota)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9514).Count(&subCount).Error)
	assert.Equal(t, int64(0), subCount)
}

func TestSubscriptionBalancePayExistingPendingDoesNotDeduct(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9531
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_pending", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	code := "balance-pending"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9532, Title: "Pending", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	tradeNo := "BALSUBUSR9531NO" + common.Sha1([]byte("pending-key"))
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: userID, PlanId: 9532, Money: 40, TradeNo: tradeNo, PaymentProvider: model.PaymentProviderBalance, PaymentMethod: model.PaymentMethodAccountBalance, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9532,"idempotency_key":"pending-key"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "subscription order status invalid")
	assert.NotContains(t, recorder.Body.String(), `"message":"success"`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 10000, user.Quota)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9532).Count(&subCount).Error)
	assert.Equal(t, int64(0), subCount)
}

func TestSubscriptionBalancePayIdempotent(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9521
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_idem", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	code := "balance-standard"
	seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{Id: 9522, Title: "Standard", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}, 80_000_000)

	first := performBalancePayRequest(t, userID, `{"plan_id":9522,"idempotency_key":"idem-key"}`)
	second := performBalancePayRequest(t, userID, `{"plan_id":9522,"idempotency_key":"idem-key"}`)

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	assert.Contains(t, first.Body.String(), `"message":"success"`)
	assert.Contains(t, second.Body.String(), `"message":"success"`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 2000, user.Quota)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9522).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
	var orderCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", userID, 9522).Count(&orderCount).Error)
	assert.Equal(t, int64(1), orderCount)
	var logCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", userID, model.LogTypeTopup).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)
}

func TestSubscriptionBalancePayTimedModeIgnoresHistoricalPurchaseLimit(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9541
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_limit", Quota: 20000, Status: common.UserStatusEnabled}).Error)
	code := "balance-limit"
	seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{Id: 9542, Title: "Limit", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 1, MaxPurchasePerUser: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}, 40_000_000)

	first := performBalancePayRequest(t, userID, `{"plan_id":9542,"idempotency_key":"limit-one"}`)
	second := performBalancePayRequest(t, userID, `{"plan_id":9542,"idempotency_key":"limit-two"}`)

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	assert.Contains(t, first.Body.String(), `"message":"success"`)
	assert.Contains(t, second.Body.String(), `"message":"success"`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 12000, user.Quota)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9542).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
	var orders []model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9542).Order("id asc").Find(&orders).Error)
	require.Len(t, orders, 2)
	for _, order := range orders {
		var snapshot model.SubscriptionEntitlementSnapshot
		require.NoError(t, common.UnmarshalJsonStr(order.EntitlementSnapshot, &snapshot))
		assert.Zero(t, snapshot.MaxPurchasePerUser)
	}
}

func TestSubscriptionBalancePayExtendsActiveSubscriptionWithoutNewRecord(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9561
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_extend", Quota: 20000, Status: common.UserStatusEnabled}).Error)
	code := "balance-extend"
	seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{Id: 9562, Title: "Extend", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}, 40_000_000)
	initialEnd := common.GetTimestamp() + 3600
	existing := &model.UserSubscription{UserId: userID, PlanId: 9562, Status: "active", StartTime: common.GetTimestamp() - 60, EndTime: initialEnd, TokenLimit: 1000, TokenUsed: 125, GrantReason: "order", Source: "order"}
	require.NoError(t, model.DB.Create(existing).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9562,"idempotency_key":"balance-extend"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, existing.Id).Error)
	assert.Equal(t, initialEnd+86400, sub.EndTime)
	assert.Equal(t, int64(125), sub.TokenUsed)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9562).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 16000, user.Quota)
}

func TestSubscriptionBalancePayLocksUserBeforePurchaseLimitCheck(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9551
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_lock", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	plan := seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{Id: 9552, Title: "Lock", Enabled: true, PublicVisible: true, MaxPurchasePerUser: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}, 40_000_000)
	snapshot, err := model.MarshalSubscriptionEntitlementSnapshot(model.NewSubscriptionEntitlementSnapshot(&plan, model.SubscriptionPurchaseModeTimed, 0))
	require.NoError(t, err)

	var order model.SubscriptionOrder
	created := false
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var createErr error
		created, _, createErr = service.CreateBalanceSubscriptionOrderTx(tx, userID, &plan, "BALSUBUSR9551NOdb-lock", 4000, model.SubscriptionPurchaseModeTimed, snapshot, &order)
		return createErr
	})

	require.NoError(t, err)
	assert.True(t, created)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 6000, user.Quota)
}
