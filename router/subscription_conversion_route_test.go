package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type subscriptionConversionObservableFacts struct {
	Id                       string `json:"id"`
	SourceSubscriptionId     string `json:"source_subscription_id"`
	TargetSubscriptionId     string `json:"target_subscription_id"`
	Full31DayBlocks          string `json:"full_31_day_blocks"`
	CreditBasis              string `json:"credit_basis"`
	CurrentRemainingCredit   string `json:"current_remaining_credit"`
	GrossCredit              string `json:"gross_credit"`
	DebtOffset               string `json:"debt_offset"`
	NetAvailableCredit       string `json:"net_available_credit"`
	AvailableCreditAfter     string `json:"available_credit_after"`
	SettlementDebtAfter      string `json:"settlement_debt_after"`
	SourcePriceMicros        string `json:"source_price_micros"`
	SourceCurrency           string `json:"source_currency"`
	TargetCurrency           string `json:"target_currency"`
	ValuationCreditBasis     string `json:"valuation_credit_basis"`
	GrossCostMicros          string `json:"gross_cost_micros"`
	NetCostMicros            string `json:"net_cost_micros"`
	UnitValueNumeratorMicros string `json:"unit_value_numerator_micros"`
	UnitValueDenominator     string `json:"unit_value_denominator"`
	RuleVersion              int    `json:"rule_version"`
	StateVersionAfter        string `json:"state_version_after"`
	FxNumerator              string `json:"fx_numerator"`
	FxDenominator            string `json:"fx_denominator"`
	FxCapturedAt             string `json:"fx_captured_at"`
	FxDirection              string `json:"fx_direction"`
}

type subscriptionConversionRouteResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Success bool   `json:"success"`
	Data    struct {
		Replayed   bool                                  `json:"replayed"`
		Conversion subscriptionConversionObservableFacts `json:"conversion"`
	} `json:"data"`
}

type subscriptionConversionObservableListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Conversions []subscriptionConversionObservableFacts `json:"conversions"`
	} `json:"data"`
}

type subscriptionConversionAnalyticsRouteResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Data struct {
			Summary struct {
				ConversionCount      int    `json:"conversion_count"`
				ExactConversionCount int    `json:"exact_conversion_count"`
				GrossCredit          string `json:"gross_credit"`
				DebtOffset           string `json:"debt_offset"`
				NetAvailableCredit   string `json:"net_available_credit"`
				GrossValueByCurrency []struct {
					AmountMicros string `json:"amount_micros"`
					Currency     string `json:"currency"`
				} `json:"gross_value_by_currency"`
				NetValueByCurrency []struct {
					AmountMicros string `json:"amount_micros"`
					Currency     string `json:"currency"`
				} `json:"net_value_by_currency"`
			} `json:"summary"`
		} `json:"data"`
	} `json:"data"`
}

func TestSubscriptionConversionRouteCommitsLatestQuoteAtomicallyAndReplays(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.UserSubscription{},
		&model.SubscriptionOrder{},
		&model.Redemption{},
		&model.InvitationRewardEvent{},
		&model.CreditBalanceLedger{},
		&model.SubscriptionConversion{},
	))
	model.ClearPrimaryBillableSubscriptionCacheForTest()

	const userID = 9971
	const sourceID = 9972
	const balanceID = 9973
	const creditPlanID = 9974
	const timedPlanID = 9975
	accessToken := "subscription-conversion-route-token"
	settingBytes, err := common.Marshal(map[string]any{
		"active_subscription_id":        sourceID,
		"subscription_billing_strategy": model.SubscriptionBillingStrategySingleActive,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Id: userID, Username: "subscription-conversion-route", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, AccessToken: &accessToken, Setting: string(settingBytes),
	}).Error)

	creditCode := "subscription_conversion_credit_balance"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: creditPlanID, Title: "Credit balance", EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true, CreditBalanceConversionEnabled: true,
	}).Error)
	timedCode := "subscription_conversion_timed"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: timedPlanID, Title: "Monthly convertible", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100, TimedConversionEnabled: true,
	}).Error)

	now := model.GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id: sourceID, UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed,
		TokenLimit: 100, TokenUsed: 20, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
		StartTime: now - 40*24*60*60, EndTime: now + model.TimedSubscriptionConversionBlockSeconds + 600, Status: "active",
		LastGrantedAt: now - 40*24*60*60, LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive, LastGrantSource: model.SubscriptionGrantOrder,
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id: balanceID, UserId: userID, PlanId: creditPlanID, EntitlementType: model.SubscriptionEntitlementCreditBalance,
		TokenLimit: 50, TokenUsed: 75, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
		Status: "active",
	}).Error)

	// The preview becomes stale before confirmation. The submitted Credit value is deliberately false.
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", sourceID).UpdateColumn("token_used", int64(40)).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	idempotencyKey := strings.Repeat("k", 128)
	first := performSubscriptionConversionRouteRequest(t, engine, userID, accessToken,
		`{"subscription_id":"9972","idempotency_key":"`+idempotencyKey+`","gross_credit":"999999999"}`)
	require.True(t, first.Success)
	assert.False(t, first.Data.Replayed)
	assert.Equal(t, strconv.Itoa(sourceID), first.Data.Conversion.SourceSubscriptionId)
	assert.Equal(t, strconv.Itoa(balanceID), first.Data.Conversion.TargetSubscriptionId)
	assert.Equal(t, "1", first.Data.Conversion.Full31DayBlocks)
	assert.Equal(t, "100", first.Data.Conversion.CreditBasis)
	assert.Equal(t, "60", first.Data.Conversion.CurrentRemainingCredit)
	assert.Equal(t, "160", first.Data.Conversion.GrossCredit)
	assert.Equal(t, "25", first.Data.Conversion.DebtOffset)
	assert.Equal(t, "135", first.Data.Conversion.NetAvailableCredit)
	assert.Equal(t, "135", first.Data.Conversion.AvailableCreditAfter)
	assert.Equal(t, "0", first.Data.Conversion.SettlementDebtAfter)

	var source model.UserSubscription
	require.NoError(t, db.First(&source, sourceID).Error)
	assert.Equal(t, model.SubscriptionStatusConverted, source.Status)
	assert.NotZero(t, source.ConvertedAt)
	assert.Equal(t, strconv.Itoa(source.ConversionId), first.Data.Conversion.Id)
	assert.Equal(t, balanceID, source.ConvertedToSubscriptionId)

	var balance model.UserSubscription
	require.NoError(t, db.First(&balance, balanceID).Error)
	assert.Equal(t, int64(210), balance.TokenLimit)
	assert.Equal(t, int64(75), balance.TokenUsed)
	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	assert.Equal(t, balanceID, user.GetSetting().ActiveSubscriptionId)

	var conversionCount int64
	require.NoError(t, db.Model(&model.SubscriptionConversion{}).Where("source_subscription_id = ?", sourceID).Count(&conversionCount).Error)
	assert.Equal(t, int64(1), conversionCount)
	var ledgerCount int64
	require.NoError(t, db.Model(&model.CreditBalanceLedger{}).
		Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionConversion, sourceID).
		Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
	var ledger model.CreditBalanceLedger
	require.NoError(t, db.Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionConversion, sourceID).First(&ledger).Error)
	assert.Equal(t, "subscription_conversion:9972", ledger.IdempotencyKey)
	assert.LessOrEqual(t, len(ledger.IdempotencyKey), 128)

	replay := performSubscriptionConversionRouteRequest(t, engine, userID, accessToken,
		`{"subscription_id":"9972","idempotency_key":"`+idempotencyKey+`"}`)
	require.True(t, replay.Success)
	assert.True(t, replay.Data.Replayed)
	assert.Equal(t, first.Data.Conversion, replay.Data.Conversion)
	differentKey := performSubscriptionConversionRouteRequest(t, engine, userID, accessToken,
		`{"subscription_id":"9972","idempotency_key":"different-key"}`)
	assert.False(t, differentKey.Success)
	assert.Equal(t, "subscription_conversion_idempotency_conflict", differentKey.Code)

	require.NoError(t, db.Model(&model.SubscriptionConversion{}).Where("source_subscription_id = ?", sourceID).Count(&conversionCount).Error)
	assert.Equal(t, int64(1), conversionCount)
	require.NoError(t, db.Model(&model.CreditBalanceLedger{}).
		Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionConversion, sourceID).
		Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
}

func TestSubscriptionConversionRoutesExposeFrozenCrossCurrencyFactsAcrossHistoryAndAnalytics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.UserSubscription{},
		&model.SubscriptionOrder{},
		&model.Redemption{},
		&model.InvitationRewardEvent{},
		&model.CreditBalanceLedger{},
		&model.SubscriptionConversion{},
		&model.CreditValuationState{},
		&model.CreditValuationMigration{},
		&model.SubscriptionPreConsumeRecord{},
		&model.Option{},
	))
	model.ClearPrimaryBillableSubscriptionCacheForTest()

	const (
		userID       = 26_901
		sourceID     = 26_902
		conflictID   = 26_903
		creditPlanID = 26_904
		timedPlanID  = 26_905
	)
	accessToken := "subscription-conversion-frozen-facts-token"
	settingBytes, err := common.Marshal(map[string]any{
		"active_subscription_id":        sourceID,
		"subscription_billing_strategy": model.SubscriptionBillingStrategySingleActive,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Id: userID, Username: "subscription-conversion-frozen-facts", Status: common.UserStatusEnabled,
		Role: common.RoleRootUser, AccessToken: &accessToken, Setting: string(settingBytes),
	}).Error)

	creditCode := "subscription_conversion_frozen_facts_credit"
	valuationCurrency := "USD"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: creditPlanID, Title: "USD Credit balance", EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true, CreditBalanceConversionEnabled: true,
		ValuationCurrency: &valuationCurrency,
	}).Error)
	timedCode := "subscription_conversion_frozen_facts_timed"
	sourcePriceMicros := int64(40_000_000)
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: timedPlanID, Title: "CNY monthly convertible", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100, TimedConversionEnabled: true,
		PriceAmount: 40, PriceAmountMicros: &sourcePriceMicros, Currency: "CNY",
	}).Error)
	now := model.GetDBTimestamp()
	require.NoError(t, db.Create(&model.CreditValuationMigration{
		Version: model.CreditValuationRuleVersion, Status: model.CreditValuationMigrationReady,
		ValuationCurrency: "USD", FxRateNumerator: 10, FxRateDenominator: 73, FxCapturedAt: now,
	}).Error)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	require.NoError(t, model.UpdateOption("USDExchangeRate", "7.3"))

	basis := int64(100)
	for _, id := range []int{sourceID, conflictID} {
		require.NoError(t, db.Create(&model.UserSubscription{
			Id: id, UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed,
			TokenLimit: 100, TokenUsed: 20, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
			StartTime: now - 40*24*60*60, EndTime: now + model.TimedSubscriptionConversionBlockSeconds + 600,
			Status: model.SubscriptionStatusActive, LastGrantedAt: now - 40*24*60*60,
			LastGrantCreditSnapshot: &basis, LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive,
			LastGrantSource: model.SubscriptionGrantOrder,
		}).Error)
	}

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	quote := performConversionQuoteRouteRequest(t, engine, userID, accessToken)
	require.Len(t, quote.Data.Quotes, 2)
	quoteByID := conversionQuoteRouteItemsByID(quote.Data.Quotes)
	require.Equal(t, "180", quoteByID[strconv.Itoa(sourceID)].GrossCredit)

	const idempotencyKey = "cross-currency-observable-facts"
	confirmed := performSubscriptionConversionRouteRequest(t, engine, userID, accessToken,
		`{"subscription_id":"26902","idempotency_key":"`+idempotencyKey+`"}`)
	require.True(t, confirmed.Success, confirmed.Message)
	facts := confirmed.Data.Conversion
	assert.Equal(t, "40000000", facts.SourcePriceMicros)
	assert.Equal(t, "CNY", facts.SourceCurrency)
	assert.Equal(t, "USD", facts.TargetCurrency)
	assert.Equal(t, "100", facts.ValuationCreditBasis)
	assert.Equal(t, "9863013", facts.GrossCostMicros)
	assert.Equal(t, "9863013", facts.NetCostMicros)
	assert.Equal(t, "4000000", facts.UnitValueNumeratorMicros)
	assert.Equal(t, "73", facts.UnitValueDenominator)
	assert.Equal(t, model.CreditValuationRuleVersion, facts.RuleVersion)
	var conversionLedger model.CreditBalanceLedger
	require.NoError(t, db.Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionConversion, sourceID).First(&conversionLedger).Error)
	assert.Equal(t, strconv.FormatInt(conversionLedger.ValuationStateVersionAfter, 10), facts.StateVersionAfter)
	assert.Equal(t, "10", facts.FxNumerator)
	assert.Equal(t, "73", facts.FxDenominator)
	assert.NotEmpty(t, facts.FxCapturedAt)
	assert.Equal(t, model.CreditFXDirectionCNYtoUSD, facts.FxDirection)

	const committedUnitValueNumeratorMicros int64 = 1_234_567
	const committedUnitValueDenominator int64 = 89
	require.NoError(t, db.Exec(
		"UPDATE subscription_conversions SET valuation_unit_value_numerator_micros = ?, valuation_unit_value_denominator = ? WHERE source_subscription_id = ?",
		committedUnitValueNumeratorMicros,
		committedUnitValueDenominator,
		sourceID,
	).Error)
	facts.UnitValueNumeratorMicros = strconv.FormatInt(committedUnitValueNumeratorMicros, 10)
	facts.UnitValueDenominator = strconv.FormatInt(committedUnitValueDenominator, 10)

	require.NoError(t, model.UpdateOption("USDExchangeRate", "8.1"))
	require.NoError(t, db.Model(&model.SubscriptionPlan{}).Where("id = ?", timedPlanID).
		UpdateColumn("price_amount_micros", int64(41_000_000)).Error)
	historyRecorder := httptest.NewRecorder()
	historyRequest := httptest.NewRequest(http.MethodGet, "/api/subscription/self/conversion-quotes", nil)
	historyRequest.Header.Set("Authorization", "Bearer "+accessToken)
	historyRequest.Header.Set("New-Api-User", strconv.Itoa(userID))
	engine.ServeHTTP(historyRecorder, historyRequest)
	require.Equal(t, http.StatusOK, historyRecorder.Code)
	var history subscriptionConversionObservableListResponse
	require.NoError(t, common.Unmarshal(historyRecorder.Body.Bytes(), &history))
	require.True(t, history.Success)
	require.Len(t, history.Data.Conversions, 1)
	assert.Equal(t, facts, history.Data.Conversions[0], "history must expose the original frozen facts after price and FX updates")

	analyticsRecorder := httptest.NewRecorder()
	analyticsRequest := httptest.NewRequest(http.MethodGet, "/api/admin-analytics/subscription-conversion", nil)
	analyticsRequest.Header.Set("Authorization", "Bearer "+accessToken)
	analyticsRequest.Header.Set("New-Api-User", strconv.Itoa(userID))
	engine.ServeHTTP(analyticsRecorder, analyticsRequest)
	require.Equal(t, http.StatusOK, analyticsRecorder.Code)
	var analytics subscriptionConversionAnalyticsRouteResponse
	require.NoError(t, common.Unmarshal(analyticsRecorder.Body.Bytes(), &analytics))
	require.True(t, analytics.Success)
	require.Equal(t, 1, analytics.Data.Data.Summary.ConversionCount)
	require.Equal(t, 1, analytics.Data.Data.Summary.ExactConversionCount)
	require.Equal(t, "180", analytics.Data.Data.Summary.GrossCredit)
	require.Equal(t, "0", analytics.Data.Data.Summary.DebtOffset)
	require.Equal(t, "180", analytics.Data.Data.Summary.NetAvailableCredit)
	require.Len(t, analytics.Data.Data.Summary.GrossValueByCurrency, 1)
	require.Equal(t, "USD", analytics.Data.Data.Summary.GrossValueByCurrency[0].Currency)
	require.Equal(t, "9863013", analytics.Data.Data.Summary.GrossValueByCurrency[0].AmountMicros)
	require.Equal(t, analytics.Data.Data.Summary.GrossValueByCurrency, analytics.Data.Data.Summary.NetValueByCurrency)

	drilldownRecorder := httptest.NewRecorder()
	drilldownRequest := httptest.NewRequest(http.MethodGet, "/api/admin-analytics/drilldown/subscriptions?subscription_statuses=converted&limit=20", nil)
	drilldownRequest.Header.Set("Authorization", "Bearer "+accessToken)
	drilldownRequest.Header.Set("New-Api-User", strconv.Itoa(userID))
	engine.ServeHTTP(drilldownRecorder, drilldownRequest)
	require.Equal(t, http.StatusOK, drilldownRecorder.Code)
	assert.Contains(t, drilldownRecorder.Body.String(), strconv.Itoa(sourceID))
	assert.Contains(t, drilldownRecorder.Body.String(), facts.TargetSubscriptionId)

	conflict := performSubscriptionConversionRouteRequest(t, engine, userID, accessToken,
		`{"subscription_id":"26903","idempotency_key":"`+idempotencyKey+`"}`)
	require.False(t, conflict.Success)
	assert.Equal(t, "subscription_conversion_idempotency_conflict", conflict.Code)
}

func TestSubscriptionConversionRoutePreservesNonActiveSelectionAndCreatesSingleCreditBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.UserSubscription{},
		&model.SubscriptionOrder{},
		&model.Redemption{},
		&model.InvitationRewardEvent{},
		&model.CreditBalanceLedger{},
		&model.SubscriptionConversion{},
	))
	model.ClearPrimaryBillableSubscriptionCacheForTest()

	const userID = 9981
	const activeID = 9982
	const sourceID = 9983
	const creditPlanID = 9984
	const timedPlanID = 9985
	accessToken := "subscription-conversion-non-active-token"
	settingBytes, err := common.Marshal(map[string]any{
		"active_subscription_id":        activeID,
		"subscription_billing_strategy": model.SubscriptionBillingStrategyTimedFirst,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Id: userID, Username: "subscription-conversion-non-active", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, AccessToken: &accessToken, Setting: string(settingBytes),
	}).Error)
	creditCode := "subscription_conversion_non_active_credit"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: creditPlanID, Title: "Credit balance", EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true, CreditBalanceConversionEnabled: true,
	}).Error)
	timedCode := "subscription_conversion_non_active_timed"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: timedPlanID, Title: "Monthly convertible", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100, TimedConversionEnabled: true,
	}).Error)
	now := model.GetDBTimestamp()
	basis := int64(100)
	for _, subscription := range []model.UserSubscription{
		{
			Id: activeID, UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed,
			TokenLimit: 100, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
			StartTime: now - 40*24*60*60, EndTime: now + 2*model.TimedSubscriptionConversionBlockSeconds, Status: "active",
			LastGrantedAt: now - 40*24*60*60, LastGrantCreditSnapshot: &basis,
		},
		{
			Id: sourceID, UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed,
			TokenLimit: 100, TokenUsed: 25, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
			StartTime: now - 40*24*60*60, EndTime: now + model.TimedSubscriptionConversionBlockSeconds, Status: "active",
			LastGrantedAt: now - 40*24*60*60, LastGrantCreditSnapshot: &basis,
		},
	} {
		require.NoError(t, db.Create(&subscription).Error)
	}

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)
	response := performSubscriptionConversionRouteRequest(t, engine, userID, accessToken,
		`{"subscription_id":"9983","idempotency_key":"convert-non-active"}`)
	require.True(t, response.Success, response.Message)

	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	assert.Equal(t, activeID, user.GetSetting().ActiveSubscriptionId)
	assert.Equal(t, model.SubscriptionBillingStrategyTimedFirst, user.GetSetting().SubscriptionBillingStrategy)
	var source model.UserSubscription
	require.NoError(t, db.First(&source, sourceID).Error)
	assert.Equal(t, model.SubscriptionStatusConverted, source.Status)
	var balanceCount int64
	require.NoError(t, db.Model(&model.UserSubscription{}).
		Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).
		Count(&balanceCount).Error)
	assert.Equal(t, int64(1), balanceCount)
	var rewardCount int64
	require.NoError(t, db.Model(&model.InvitationRewardEvent{}).Where("source_subscription_id = ?", sourceID).Count(&rewardCount).Error)
	assert.Zero(t, rewardCount)
}

func TestSubscriptionConversionRouteRollsBackEveryEffectWhenLedgerInsertFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.UserSubscription{},
		&model.SubscriptionOrder{},
		&model.Redemption{},
		&model.InvitationRewardEvent{},
		&model.CreditBalanceLedger{},
		&model.SubscriptionConversion{},
	))
	model.ClearPrimaryBillableSubscriptionCacheForTest()

	const userID = 9_961
	const sourceID = 9_962
	const creditPlanID = 9_963
	const timedPlanID = 9_964
	accessToken := "subscription-conversion-rollback-token"
	settingBytes, err := common.Marshal(map[string]any{
		"active_subscription_id":        sourceID,
		"subscription_billing_strategy": model.SubscriptionBillingStrategySingleActive,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Id: userID, Username: "subscription-conversion-rollback", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, AccessToken: &accessToken, Setting: string(settingBytes),
	}).Error)
	creditCode := "subscription_conversion_rollback_credit"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: creditPlanID, Title: "Credit balance", EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true, CreditBalanceConversionEnabled: true,
	}).Error)
	timedCode := "subscription_conversion_rollback_timed"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: timedPlanID, Title: "Monthly convertible", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100, TimedConversionEnabled: true,
	}).Error)
	now := model.GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id: sourceID, UserId: userID, PlanId: timedPlanID, EntitlementType: model.SubscriptionEntitlementTimed,
		TokenLimit: 100, TokenUsed: 25, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
		StartTime: now - 40*24*60*60, EndTime: now + model.TimedSubscriptionConversionBlockSeconds, Status: "active",
		LastGrantedAt: now - 40*24*60*60, LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive, LastGrantSource: model.SubscriptionGrantOrder,
	}).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_subscription_conversion_ledger
		BEFORE INSERT ON credit_balance_ledgers
		WHEN NEW.source_type = 'subscription_conversion'
		BEGIN
			SELECT RAISE(ABORT, 'forced conversion ledger failure');
		END`).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)
	response := performSubscriptionConversionRouteRequest(t, engine, userID, accessToken,
		`{"subscription_id":"9962","idempotency_key":"rollback-key"}`)
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "forced conversion ledger failure")

	var source model.UserSubscription
	require.NoError(t, db.First(&source, sourceID).Error)
	assert.Equal(t, "active", source.Status)
	assert.Zero(t, source.ConvertedAt)
	assert.Zero(t, source.ConversionId)
	assert.Zero(t, source.ConvertedToSubscriptionId)
	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	assert.Equal(t, sourceID, user.GetSetting().ActiveSubscriptionId)
	assert.Equal(t, model.SubscriptionBillingStrategySingleActive, user.GetSetting().SubscriptionBillingStrategy)
	var conversionCount int64
	require.NoError(t, db.Model(&model.SubscriptionConversion{}).Count(&conversionCount).Error)
	assert.Zero(t, conversionCount)
	var ledgerCount int64
	require.NoError(t, db.Model(&model.CreditBalanceLedger{}).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
	var balanceCount int64
	require.NoError(t, db.Model(&model.UserSubscription{}).
		Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).
		Count(&balanceCount).Error)
	assert.Zero(t, balanceCount)
	var rewardCount int64
	require.NoError(t, db.Model(&model.InvitationRewardEvent{}).Count(&rewardCount).Error)
	assert.Zero(t, rewardCount)
}

func performSubscriptionConversionRouteRequest(t *testing.T, engine *gin.Engine, userID int, accessToken string, body string) subscriptionConversionRouteResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/subscription/self/conversions", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", strconv.Itoa(userID))
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload subscriptionConversionRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}
