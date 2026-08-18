package controller

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubscriptionTrialPurchaseTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.SubscriptionPlan{},
		&model.SubscriptionOrder{},
		&model.User{},
		&model.UserSubscription{},
		&model.TimedSubscriptionValuationGrant{},
		&model.CreditBalanceLedger{},
		&model.TopUp{},
		&model.Log{},
		&model.InvitationRewardEvent{},
		&model.InvitationCommissionRecord{},
	))
	require.NoError(t, model.DB.Create(&model.User{Id: 8801, Username: "buyer", Status: common.UserStatusEnabled}).Error)
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "Alipay"}}
	t.Cleanup(func() {
		setting.StripeApiSecret = ""
		setting.StripeWebhookSecret = ""
		setting.CreemWebhookSecret = ""
		setting.CreemTestMode = false
		operation_setting.PayMethods = nil
		operation_setting.PayAddress = ""
		operation_setting.EpayId = ""
		operation_setting.EpayKey = ""
		operation_setting.CustomCallbackAddress = ""
	})
}

func seedSubscriptionPurchasePlan(t *testing.T, id int, trial bool, visible bool, price float64) {
	t.Helper()
	code := "plan_purchase_" + string(rune('a'+id%26))
	var priceMicros *int64
	if price > 0 {
		parsed, err := model.ParseDecimalAmountMicros(strconv.FormatFloat(price, 'f', -1, 64))
		require.NoError(t, err)
		priceMicros = &parsed
	}
	plan := &model.SubscriptionPlan{Id: id, Title: "Purchase Plan", Enabled: true, PublicVisible: visible, IsTrial: trial, PriceAmount: price, PriceAmountMicros: priceMicros, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, RewardEligible: true, BusinessCode: &code, StripePriceId: "price_test", CreemProductId: "prod_test"}
	require.NoError(t, model.DB.Create(plan).Error)
	require.NoError(t, model.DB.Model(plan).Updates(map[string]interface{}{"is_trial": trial, "public_visible": visible}).Error)
}

func seedAuthoritativeSubscriptionPurchasePlan(t *testing.T, id int, visible bool, priceMicros int64) model.SubscriptionPlan {
	t.Helper()
	code := "plan_purchase_" + string(rune('a'+id%26))
	plan := seedAuthoritativeTimedPlanFixture(t, model.SubscriptionPlan{
		Id:                id,
		Title:             "Purchase Plan",
		PublicVisible:     visible,
		DurationUnit:      model.SubscriptionDurationMonth,
		DurationValue:     1,
		MonthlyTokenLimit: 1000,
		ConcurrencyLimit:  1,
		QuotaResetPeriod:  model.SubscriptionResetNever,
		RewardEligible:    true,
		BusinessCode:      &code,
		StripePriceId:     "price_test",
		CreemProductId:    "prod_test",
	}, priceMicros)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", id).Update("public_visible", visible).Error)
	return plan
}

func seedExternalCreditPurchasePlans(t *testing.T, optionPlanID int, creditPlanID int) {
	t.Helper()
	seedSubscriptionPurchasePlan(t, optionPlanID, false, true, 40)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", optionPlanID).Updates(map[string]any{
		"quota_reset_period":         model.SubscriptionResetMonthly,
		"unlimited_purchase_enabled": true,
	}).Error)
	creditCode := "external_credit_balance_" + string(rune('a'+creditPlanID%26))
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:                           creditPlanID,
		Title:                        "Global Credit Balance",
		EntitlementType:              model.SubscriptionEntitlementCreditBalance,
		Enabled:                      true,
		CreditBalanceConfigured:      true,
		CreditBalancePurchaseEnabled: true,
		ModelLimits:                  "gpt-4o",
		ConcurrencyLimit:             7,
		QueueCapacity:                11,
		GPTAbuseWarningLimit:         5,
		BusinessCode:                 &creditCode,
	}).Error)
}

func configureEpaySubscriptionTest() {
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	operation_setting.CustomCallbackAddress = "https://callback.example.com"
}

func performSubscriptionJSON(handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/pay", bytes.NewBufferString(body))
	ctx.Set("id", 8801)
	handler(ctx)
	return recorder
}

func signedEpaySubscriptionCallbackForSnapshotTest(t *testing.T, tradeNo string, money string) url.Values {
	t.Helper()
	params := map[string]string{
		"pid":          operation_setting.EpayId,
		"type":         "alipay",
		"out_trade_no": tradeNo,
		"trade_status": "TRADE_SUCCESS",
		"trade_no":     "epay_" + tradeNo,
		"name":         "subscription",
		"money":        money,
	}
	signed := signEpaySubscriptionCallbackForSnapshotTest(params)
	form := url.Values{}
	for key, value := range signed {
		form.Set(key, value)
	}
	return form
}

func signEpaySubscriptionCallbackForSnapshotTest(params map[string]string) map[string]string {
	key := operation_setting.EpayKey
	if client := GetEpayClient(); client != nil {
		key = client.Config.Key
	}
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	sum := md5.Sum([]byte(b.String() + key))
	params["sign"] = hex.EncodeToString(sum[:])
	params["sign_type"] = "MD5"
	return params
}

func cloneURLValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func performEpaySubscriptionCallbackForSnapshotTest(t *testing.T, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return performEpaySubscriptionCallback(form)
}

func performEpaySubscriptionCallback(form url.Values) *httptest.ResponseRecorder {
	cloned := cloneURLValues(form)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/epay/notify", strings.NewReader(cloned.Encode()))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	SubscriptionEpayNotify(ctx)
	return recorder
}

func TestGetSubscriptionPlans_HidesTrialPlans(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8811, false, true, 40)
	seedSubscriptionPurchasePlan(t, 8812, true, false, 0)
	seedSubscriptionPurchasePlan(t, 8813, false, false, 40)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil)
	GetSubscriptionPlans(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"id":8811`)
	assert.NotContains(t, body, `"id":8812`)
	assert.NotContains(t, body, `"id":8813`)
}

func TestSubscriptionEpayRejectsTrialPlan(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8821, true, false, 0)

	recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":8821,"purchase_mode":"timed","payment_method":"alipay"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "套餐不可购买")
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSubscriptionStripeRejectsTrialPlan(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8831, true, false, 0)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":8831,"purchase_mode":"timed"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "套餐不可购买")
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSubscriptionCreemRejectsTrialPlan(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8841, true, false, 0)
	setting.CreemWebhookSecret = "creem_secret"

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":8841,"purchase_mode":"timed"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "套餐不可购买")
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSubscriptionStripeAllowsRenewalWhenHistoricalPurchaseLimitReached(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8851, false, true, 40)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 8851).Update("max_purchase_per_user", 1).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: 8801, PlanId: 8851, Status: "active", StartTime: common.GetTimestamp() - 10, EndTime: common.GetTimestamp() + 3600, GrantReason: "order", Source: "order"}).Error)
	SetStripeSubscriptionPriceSnapshotForTest(t, func(string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(string, string, string, string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/renewal"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":8851,"purchase_mode":"timed"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 8851).First(&order).Error)
	var snapshot model.SubscriptionEntitlementSnapshot
	require.NoError(t, common.UnmarshalJsonStr(order.EntitlementSnapshot, &snapshot))
	assert.Zero(t, snapshot.MaxPurchasePerUser)
}

func TestSubscriptionCreemAllowsRenewalWhenHistoricalPurchaseLimitReached(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8861, false, true, 40)
	setting.CreemWebhookSecret = "creem_secret"
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 8861).Update("max_purchase_per_user", 1).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: 8801, PlanId: 8861, Status: "active", StartTime: common.GetTimestamp() - 10, EndTime: common.GetTimestamp() + 3600, GrantReason: "order", Source: "order"}).Error)
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	SetCreemSubscriptionCheckoutForTest(t, func(string, *CreemProduct, string, string) (CreemSubscriptionCheckoutResult, error) {
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/renewal", AmountCents: 4000, Currency: "CNY"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":8861,"purchase_mode":"timed"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 8861).First(&order).Error)
	var snapshot model.SubscriptionEntitlementSnapshot
	require.NoError(t, common.UnmarshalJsonStr(order.EntitlementSnapshot, &snapshot))
	assert.Zero(t, snapshot.MaxPurchasePerUser)
}

func TestSubscriptionEpayAllowsRenewalWhenHistoricalPurchaseLimitReached(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8871, false, true, 40)
	configureEpaySubscriptionTest()
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 8871).Update("max_purchase_per_user", 1).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: 8801, PlanId: 8871, Status: "active", StartTime: common.GetTimestamp() - 10, EndTime: common.GetTimestamp() + 3600, GrantReason: "order", Source: "order"}).Error)

	recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":8871,"purchase_mode":"timed","payment_method":"alipay"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 8871).First(&order).Error)
	var snapshot model.SubscriptionEntitlementSnapshot
	require.NoError(t, common.UnmarshalJsonStr(order.EntitlementSnapshot, &snapshot))
	assert.Zero(t, snapshot.MaxPurchasePerUser)
}

func TestSubscriptionEpayStoresSubmittedCNYAmountSnapshot(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9563, false, true, 40)
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	operation_setting.CustomCallbackAddress = "https://callback.example.com"

	recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9563,"purchase_mode":"timed","payment_method":"alipay"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9563).First(&order).Error)
	assert.Equal(t, int64(4000), order.AmountCents)
	assert.Equal(t, "CNY", order.Currency)
}

func TestSubscriptionEpayRejectsAmountMismatch(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9569, false, true, 40)
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	operation_setting.CustomCallbackAddress = "https://callback.example.com"

	recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9569,"purchase_mode":"timed","payment_method":"alipay"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9569).First(&order).Error)
	require.Equal(t, int64(4000), order.AmountCents)
	require.Equal(t, "CNY", order.Currency)

	callback := signedEpaySubscriptionCallbackForSnapshotTest(t, order.TradeNo, "39.99")
	callbackRecorder := performEpaySubscriptionCallbackForSnapshotTest(t, callback)

	require.Equal(t, http.StatusOK, callbackRecorder.Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.NotEqual(t, common.TopUpStatusSuccess, order.Status)
	var events int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&events).Error)
	assert.Equal(t, int64(0), events)
	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", order.Id).Count(&records).Error)
	assert.Equal(t, int64(0), records)
}

func TestSubscriptionEpayRequiresExplicitPurchaseMode(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9580, false, true, 40)
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	operation_setting.CustomCallbackAddress = "https://callback.example.com"

	recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9580,"payment_method":"alipay"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "购买模式必须明确选择")
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", 8801, 9580).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSubscriptionEpayCreditPurchaseUsesImmutableSnapshot(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9581, false, true, 40)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9581).Updates(map[string]any{
		"quota_reset_period":         model.SubscriptionResetMonthly,
		"unlimited_purchase_enabled": true,
	}).Error)
	creditCode := "external_credit_balance"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:                           9582,
		Title:                        "Global Credit Balance",
		EntitlementType:              model.SubscriptionEntitlementCreditBalance,
		Enabled:                      true,
		CreditBalanceConfigured:      true,
		CreditBalancePurchaseEnabled: true,
		ModelLimits:                  "gpt-4o",
		ConcurrencyLimit:             7,
		QueueCapacity:                11,
		BusinessCode:                 &creditCode,
	}).Error)
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	operation_setting.CustomCallbackAddress = "https://callback.example.com"

	createRecorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9581,"purchase_mode":"credit_balance","payment_method":"alipay"}`)
	require.Equal(t, http.StatusOK, createRecorder.Code, createRecorder.Body.String())
	require.Contains(t, createRecorder.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9581).First(&order).Error)
	require.NotEmpty(t, order.EntitlementSnapshot)

	changedCode := "changed_option"
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9581).Updates(map[string]any{
		"title":                      "Changed option",
		"price_amount":               99,
		"monthly_token_limit":        9000,
		"enabled":                    false,
		"public_visible":             false,
		"unlimited_purchase_enabled": false,
		"business_code":              changedCode,
	}).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9582).Updates(map[string]any{
		"enabled":                         false,
		"credit_balance_purchase_enabled": false,
		"model_limits":                    "changed-model",
		"concurrency_limit":               1,
		"queue_capacity":                  1,
	}).Error)
	model.InvalidateSubscriptionPlanCache(9581)
	model.InvalidateSubscriptionPlanCache(9582)

	callbackRecorder := performEpaySubscriptionCallbackForSnapshotTest(t, signedEpaySubscriptionCallbackForSnapshotTest(t, order.TradeNo, "40.00"))
	require.Equal(t, http.StatusOK, callbackRecorder.Code)
	assert.Equal(t, "success", callbackRecorder.Body.String())
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, 9582, balance.PlanId)
	assert.Equal(t, int64(1000), balance.TokenLimit)
	var ledger model.CreditBalanceLedger
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrder, order.Id).First(&ledger).Error)
	assert.Equal(t, int64(1000), ledger.GrossCredit)
	assert.Equal(t, int64(0), ledger.DebtOffset)
	var timedCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementTimed).Count(&timedCount).Error)
	assert.Zero(t, timedCount)
	var invitationCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&invitationCount).Error)
	assert.Zero(t, invitationCount)
}

func TestSubscriptionEpayCreditPurchaseRejectsClosedOrIneligibleEntry(t *testing.T) {
	for _, test := range []struct {
		name          string
		planID        int
		creditPlanID  int
		updates       map[string]any
		creditUpdates map[string]any
		expected      string
	}{
		{name: "option ineligible", planID: 9590, creditPlanID: 9591, updates: map[string]any{"unlimited_purchase_enabled": false}, expected: "未开启 Credit 余额购买资格"},
		{name: "entry closed", planID: 9592, creditPlanID: 9593, creditUpdates: map[string]any{"credit_balance_purchase_enabled": false}, expected: "Credit 余额购买入口未开启"},
		{name: "non monthly option", planID: 9594, creditPlanID: 9595, updates: map[string]any{"duration_value": 2}, expected: "只有标准单月计时套餐"},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupSubscriptionTrialPurchaseTest(t)
			seedExternalCreditPurchasePlans(t, test.planID, test.creditPlanID)
			configureEpaySubscriptionTest()
			if len(test.updates) > 0 {
				require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", test.planID).Updates(test.updates).Error)
			}
			if len(test.creditUpdates) > 0 {
				require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", test.creditPlanID).Updates(test.creditUpdates).Error)
			}
			model.InvalidateSubscriptionPlanCache(test.planID)
			model.InvalidateSubscriptionPlanCache(test.creditPlanID)

			recorder := performSubscriptionJSON(SubscriptionRequestEpay, fmt.Sprintf(`{"plan_id":%d,"purchase_mode":"credit_balance","payment_method":"alipay"}`, test.planID))

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), test.expected)
			var count int64
			require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("plan_id = ?", test.planID).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestSubscriptionEpayCreditCallbackRejectsInvalidSignatureUnpaidAndWrongAmount(t *testing.T) {
	for _, test := range []struct {
		name         string
		planID       int
		creditPlanID int
		mutate       func(url.Values)
	}{
		{name: "invalid signature", planID: 9596, creditPlanID: 9597, mutate: func(values url.Values) { values.Set("sign", "invalid") }},
		{name: "unpaid", planID: 9598, creditPlanID: 9599, mutate: func(values url.Values) {
			params := map[string]string{}
			for key := range values {
				params[key] = values.Get(key)
			}
			params["trade_status"] = "TRADE_CLOSED"
			delete(params, "sign")
			delete(params, "sign_type")
			values = url.Values{}
			for key, value := range signEpaySubscriptionCallbackForSnapshotTest(params) {
				values.Set(key, value)
			}
		}},
		{name: "wrong amount", planID: 9600, creditPlanID: 9601, mutate: func(values url.Values) {
			params := map[string]string{}
			for key := range values {
				params[key] = values.Get(key)
			}
			params["money"] = "39.99"
			delete(params, "sign")
			delete(params, "sign_type")
			for key := range values {
				values.Del(key)
			}
			for key, value := range signEpaySubscriptionCallbackForSnapshotTest(params) {
				values.Set(key, value)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupSubscriptionTrialPurchaseTest(t)
			seedExternalCreditPurchasePlans(t, test.planID, test.creditPlanID)
			configureEpaySubscriptionTest()
			create := performSubscriptionJSON(SubscriptionRequestEpay, fmt.Sprintf(`{"plan_id":%d,"purchase_mode":"credit_balance","payment_method":"alipay"}`, test.planID))
			require.Contains(t, create.Body.String(), `"message":"success"`)
			var order model.SubscriptionOrder
			require.NoError(t, model.DB.Where("plan_id = ?", test.planID).First(&order).Error)
			callback := signedEpaySubscriptionCallbackForSnapshotTest(t, order.TradeNo, "40.00")
			if test.name == "unpaid" {
				params := map[string]string{"pid": operation_setting.EpayId, "type": "alipay", "out_trade_no": order.TradeNo, "trade_status": "TRADE_CLOSED", "trade_no": "epay_" + order.TradeNo, "name": "subscription", "money": "40.00"}
				callback = url.Values{}
				for key, value := range signEpaySubscriptionCallbackForSnapshotTest(params) {
					callback.Set(key, value)
				}
			} else {
				test.mutate(callback)
			}

			recorder := performEpaySubscriptionCallbackForSnapshotTest(t, callback)

			if test.name == "unpaid" {
				assert.Equal(t, "success", recorder.Body.String())
				require.NoError(t, model.DB.First(&order, order.Id).Error)
				assert.Equal(t, common.TopUpStatusFailed, order.Status)
			} else {
				assert.Equal(t, "fail", recorder.Body.String())
				require.NoError(t, model.DB.First(&order, order.Id).Error)
				assert.Equal(t, common.TopUpStatusPending, order.Status)
			}
			var count int64
			require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestSubscriptionEpayReturnClosesPendingCreditOrder(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9632, 9633)
	configureEpaySubscriptionTest()
	create := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9632,"purchase_mode":"credit_balance","payment_method":"alipay"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9632).First(&order).Error)
	params := map[string]string{
		"pid":          operation_setting.EpayId,
		"type":         "alipay",
		"out_trade_no": order.TradeNo,
		"trade_status": "TRADE_CLOSED",
		"trade_no":     "epay_" + order.TradeNo,
		"name":         "subscription",
		"money":        "40.00",
	}
	form := url.Values{}
	for key, value := range signEpaySubscriptionCallbackForSnapshotTest(params) {
		form.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/subscription/epay/return", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	engine := gin.New()
	engine.POST("/api/subscription/epay/return", SubscriptionEpayReturn)
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusFound, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Location"), "pay=fail")
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
}

func TestSubscriptionEpayCreditCallbackIsIdempotentAndOffsetsDebt(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9602, 9603)
	configureEpaySubscriptionTest()
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: 8801, PlanId: 9603, EntitlementType: model.SubscriptionEntitlementCreditBalance, Status: "active", TokenLimit: 100, TokenUsed: 350, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	create := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9602,"purchase_mode":"credit_balance","payment_method":"alipay"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9602).First(&order).Error)
	callback := signedEpaySubscriptionCallbackForSnapshotTest(t, order.TradeNo, "40.00")

	start := make(chan struct{})
	results := make(chan string, 2)
	forms := []url.Values{cloneURLValues(callback), cloneURLValues(callback)}
	var callbacks sync.WaitGroup
	for _, form := range forms {
		callbacks.Add(1)
		go func(callbackForm url.Values) {
			defer callbacks.Done()
			<-start
			results <- performEpaySubscriptionCallback(callbackForm).Body.String()
		}(form)
	}
	close(start)
	callbacks.Wait()
	close(results)
	for result := range results {
		assert.Equal(t, "success", result)
	}
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(1100), balance.TokenLimit)
	assert.Equal(t, int64(350), balance.TokenUsed)
	var ledger model.CreditBalanceLedger
	require.NoError(t, model.DB.Where("source_id = ?", order.Id).First(&ledger).Error)
	assert.Equal(t, int64(250), ledger.DebtOffset)
	assert.Equal(t, int64(750), ledger.AvailableCreditAfter)
	var count int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementCreditBalance).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestSubscriptionEpayTimedCallbackPreservesInvitationBehavior(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	plan := seedAuthoritativeSubscriptionPurchasePlan(t, 9604, true, 40_000_000)
	configureEpaySubscriptionTest()
	inviter := model.User{Id: 9605, Username: "epay-inviter", Status: common.UserStatusEnabled, AffCode: "epay-inviter"}
	require.NoError(t, model.DB.Create(&inviter).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 8801).Update("inviter_id", inviter.Id).Error)
	create := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9604,"purchase_mode":"timed","payment_method":"alipay"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9604).First(&order).Error)
	assertAuthorizedTimedOrderSnapshotFixture(t, &order, &plan)

	recorder := performEpaySubscriptionCallbackForSnapshotTest(t, signedEpaySubscriptionCallbackForSnapshotTest(t, order.TradeNo, "40.00"))

	assert.Equal(t, "success", recorder.Body.String())
	var event model.InvitationRewardEvent
	require.NoError(t, model.DB.Where("source_order_id = ?", order.Id).First(&event).Error)
	assert.Equal(t, inviter.Id, event.InviterId)
	assert.Equal(t, 8801, event.InviteeId)
}

func performSubscriptionOrderStatusRequest(userID int, tradeNo string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/orders/"+tradeNo, nil)
	ctx.Set("id", userID)
	ctx.Params = gin.Params{{Key: "trade_no", Value: tradeNo}}
	GetSubscriptionOrderStatus(ctx)
	return recorder
}

func TestGetSubscriptionOrderStatusReturnsCreditFulfillmentToOwnerOnly(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9626, 9627)
	var optionPlan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&optionPlan, 9626).Error)
	snapshot, err := prepareExternalSubscriptionEntitlementSnapshot(&optionPlan, model.SubscriptionPurchaseModeCreditBalance)
	require.NoError(t, err)
	serialized, err := marshalExternalSubscriptionEntitlementSnapshot(snapshot, model.PaymentProviderStripe, "price_test", model.PaymentMethodStripe, 4000, "CNY")
	require.NoError(t, err)
	order := model.SubscriptionOrder{UserId: 8801, PlanId: 9626, Money: 40, AmountCents: 4000, Currency: "CNY", CreditGrantAmount: 1000, CreditTargetPlanID: 9627, TradeNo: "status-credit-owner", PaymentMethod: model.PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(), EntitlementSnapshot: serialized}
	require.NoError(t, model.DB.Create(&order).Error)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		_, completeErr := model.CompleteSubscriptionOrderTx(tx, &order, `{}`, model.PaymentMethodStripe)
		return completeErr
	}))

	owner := performSubscriptionOrderStatusRequest(8801, order.TradeNo)
	other := performSubscriptionOrderStatusRequest(8802, order.TradeNo)

	require.Equal(t, http.StatusOK, owner.Code, owner.Body.String())
	assert.Contains(t, owner.Body.String(), `"payment_provider":"stripe"`)
	assert.Contains(t, owner.Body.String(), `"purchase_mode":"credit_balance"`)
	assert.Contains(t, owner.Body.String(), `"status":"success"`)
	assert.Contains(t, owner.Body.String(), `"gross_credit":1000`)
	assert.Contains(t, owner.Body.String(), `"debt_offset":0`)
	assert.Contains(t, owner.Body.String(), `"available_credit":1000`)
	assert.NotContains(t, owner.Body.String(), "provider_payload")
	require.Equal(t, http.StatusNotFound, other.Code, other.Body.String())
}

func TestSubscriptionEpayCreditCallbackTerminalOrdering(t *testing.T) {
	for _, test := range []struct {
		name           string
		optionPlanID   int
		creditPlanID   int
		firstClosed    bool
		expectedStatus string
		expectedLedger int64
	}{
		{name: "closed then success", optionPlanID: 9628, creditPlanID: 9629, firstClosed: true, expectedStatus: common.TopUpStatusFailed, expectedLedger: 0},
		{name: "success then closed", optionPlanID: 9630, creditPlanID: 9631, firstClosed: false, expectedStatus: common.TopUpStatusSuccess, expectedLedger: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupSubscriptionTrialPurchaseTest(t)
			seedExternalCreditPurchasePlans(t, test.optionPlanID, test.creditPlanID)
			configureEpaySubscriptionTest()
			create := performSubscriptionJSON(SubscriptionRequestEpay, fmt.Sprintf(`{"plan_id":%d,"purchase_mode":"credit_balance","payment_method":"alipay"}`, test.optionPlanID))
			require.Contains(t, create.Body.String(), `"message":"success"`)
			var order model.SubscriptionOrder
			require.NoError(t, model.DB.Where("plan_id = ?", test.optionPlanID).First(&order).Error)
			closedParams := map[string]string{
				"pid":          operation_setting.EpayId,
				"type":         "alipay",
				"out_trade_no": order.TradeNo,
				"trade_status": "TRADE_CLOSED",
				"trade_no":     "epay_" + order.TradeNo,
				"name":         "subscription",
				"money":        "40.00",
			}
			closed := url.Values{}
			for key, value := range signEpaySubscriptionCallbackForSnapshotTest(closedParams) {
				closed.Set(key, value)
			}
			success := signedEpaySubscriptionCallbackForSnapshotTest(t, order.TradeNo, "40.00")
			if test.firstClosed {
				assert.Equal(t, "success", performEpaySubscriptionCallbackForSnapshotTest(t, closed).Body.String())
				assert.Equal(t, "fail", performEpaySubscriptionCallbackForSnapshotTest(t, success).Body.String())
			} else {
				assert.Equal(t, "success", performEpaySubscriptionCallbackForSnapshotTest(t, success).Body.String())
				assert.Equal(t, "success", performEpaySubscriptionCallbackForSnapshotTest(t, closed).Body.String())
			}
			require.NoError(t, model.DB.First(&order, order.Id).Error)
			assert.Equal(t, test.expectedStatus, order.Status)
			var ledgerCount int64
			require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&ledgerCount).Error)
			assert.Equal(t, test.expectedLedger, ledgerCount)
		})
	}
}
