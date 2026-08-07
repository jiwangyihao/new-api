package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type subscriptionConversionQuoteRouteItem struct {
	SourceSubscriptionId     string   `json:"source_subscription_id"`
	CreditBasis              string   `json:"credit_basis"`
	CreditBasisSource        string   `json:"credit_basis_source"`
	CurrentRemainingCredit   string   `json:"current_remaining_credit"`
	GrossCredit              string   `json:"gross_credit"`
	CurrentDebt              string   `json:"current_debt"`
	EstimatedDebtOffset      string   `json:"estimated_debt_offset"`
	NetAvailableCredit       string   `json:"net_available_credit"`
	Category                 string   `json:"category"`
	CanConfirm               bool     `json:"can_confirm"`
	ReasonCodes              []string `json:"reason_codes"`
	SourcePriceMicros        string   `json:"source_price_micros"`
	SourceCurrency           string   `json:"source_currency"`
	TargetCurrency           string   `json:"target_currency"`
	ValuationCreditBasis     string   `json:"valuation_credit_basis"`
	GrossCostMicros          string   `json:"gross_cost_micros"`
	NetCostMicros            string   `json:"net_cost_micros"`
	UnitValueNumeratorMicros string   `json:"unit_value_numerator_micros"`
	UnitValueDenominator     string   `json:"unit_value_denominator"`
	RuleVersion              int      `json:"rule_version"`
	FxNumerator              string   `json:"fx_numerator"`
	FxDenominator            string   `json:"fx_denominator"`
	FxCapturedAt             string   `json:"fx_captured_at"`
	FxDirection              string   `json:"fx_direction"`
}
type subscriptionConversionHistoryRouteItem struct {
	Id                     string `json:"id"`
	SourceSubscriptionId   string `json:"source_subscription_id"`
	SourcePlanTitle        string `json:"source_plan_title"`
	TargetSubscriptionId   string `json:"target_subscription_id"`
	Full31DayBlocks        string `json:"full_31_day_blocks"`
	CreditBasis            string `json:"credit_basis"`
	CurrentRemainingCredit string `json:"current_remaining_credit"`
	GrossCredit            string `json:"gross_credit"`
	DebtOffset             string `json:"debt_offset"`
	NetAvailableCredit     string `json:"net_available_credit"`
}

type subscriptionConversionQuoteRouteResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DatabaseNow string                                   `json:"database_now"`
		Quotes      []subscriptionConversionQuoteRouteItem   `json:"quotes"`
		Conversions []subscriptionConversionHistoryRouteItem `json:"conversions"`
	} `json:"data"`
}

func TestSubscriptionConversionQuotesRouteIsAuthenticatedLiveAndReadOnly(t *testing.T) {
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
		&model.CreditValuationMigration{},
	))

	const userID = 9951
	const sourceSubscriptionID = 9952
	const expiredSubscriptionID = 9953
	const excludedSubscriptionID = 9954
	const futureSubscriptionID = 9956
	const balanceSubscriptionID = 9955
	accessToken := "conversion-quotes-route-token"
	valuationCurrency := "CNY"
	sourcePriceMicros := int64(40_000_000)
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "conversion-quotes-route", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}).Error)

	creditCode := "conversion_quote_credit_balance"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:                             9960,
		Title:                          "Credit balance",
		EntitlementType:                model.SubscriptionEntitlementCreditBalance,
		Enabled:                        true,
		BusinessCode:                   &creditCode,
		CreditBalanceConfigured:        true,
		CreditBalanceConversionEnabled: true,
		ValuationCurrency:              &valuationCurrency,
	}).Error)
	timedCode := "conversion_quote_monthly"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:                     9961,
		Title:                  "Monthly convertible",
		EntitlementType:        model.SubscriptionEntitlementTimed,
		Enabled:                true,
		BusinessCode:           &timedCode,
		DurationUnit:           model.SubscriptionDurationMonth,
		DurationValue:          1,
		QuotaResetPeriod:       model.SubscriptionResetMonthly,
		MonthlyTokenLimit:      100,
		TimedConversionEnabled: true,
		PriceAmountMicros:      &sourcePriceMicros,
		Currency:               valuationCurrency,
	}).Error)
	require.NoError(t, model.DB.Create(&model.CreditValuationMigration{
		Version:           model.CreditValuationRuleVersion,
		Status:            model.CreditValuationMigrationReady,
		ValuationCurrency: valuationCurrency,
		FxRateNumerator:   1,
		FxRateDenominator: 1,
		FxCapturedAt:      model.GetDBTimestamp(),
	}).Error)

	databaseNow := model.GetDBTimestamp()
	snapshot := int64(100)
	subscriptions := []model.UserSubscription{
		{
			Id: sourceSubscriptionID, UserId: userID, PlanId: 9961,
			EntitlementType: model.SubscriptionEntitlementTimed,
			TokenLimit:      100, TokenUsed: 25, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
			StartTime: databaseNow - 40*24*60*60, EndTime: databaseNow + model.TimedSubscriptionConversionBlockSeconds + 60, Status: "active",
			LastGrantedAt: databaseNow - 40*24*60*60, LastGrantTimeSource: model.SubscriptionGrantTimeSourceConservative, LastGrantSource: model.SubscriptionGrantOrder,
		},
		{
			Id: expiredSubscriptionID, UserId: userID, PlanId: 9961,
			EntitlementType: model.SubscriptionEntitlementTimed,
			TokenLimit:      100, TokenUsed: 50, GrantReason: model.SubscriptionGrantRedemption, Source: model.SubscriptionGrantRedemption,
			StartTime: databaseNow - 60*24*60*60, EndTime: databaseNow - 60, Status: "expired",
			LastGrantedAt: databaseNow - 60*24*60*60, LastGrantCreditSnapshot: &snapshot, LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive, LastGrantSource: model.SubscriptionGrantRedemption,
		},
		{
			Id: excludedSubscriptionID, UserId: userID, PlanId: 9961,
			EntitlementType: model.SubscriptionEntitlementTimed,
			TokenLimit:      100, TokenUsed: 0, GrantReason: model.SubscriptionGrantMonthlyInviteEntitlement, Source: model.SubscriptionGrantMonthlyInviteEntitlement,
			StartTime: databaseNow - 40*24*60*60, EndTime: databaseNow + 60, Status: "active",
			LastGrantedAt: databaseNow - 40*24*60*60, LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive, LastGrantSource: model.SubscriptionGrantMonthlyInviteEntitlement,
		},
		{
			Id: futureSubscriptionID, UserId: userID, PlanId: 9961,
			EntitlementType: model.SubscriptionEntitlementTimed,
			TokenLimit:      100, TokenUsed: 0, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
			StartTime: databaseNow + 60, EndTime: databaseNow + model.TimedSubscriptionConversionBlockSeconds + 60, Status: model.SubscriptionStatusActive,
			LastGrantedAt: databaseNow - 40*24*60*60, LastGrantCreditSnapshot: &snapshot, LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive, LastGrantSource: model.SubscriptionGrantOrder,
		},
		{
			Id: balanceSubscriptionID, UserId: userID, PlanId: 9960,
			EntitlementType: model.SubscriptionEntitlementCreditBalance,
			TokenLimit:      50, TokenUsed: 75, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder, Status: "active",
		},
	}
	require.NoError(t, model.DB.Create(&subscriptions).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	unauthorized := httptest.NewRecorder()
	engine.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/subscription/self/conversion-quotes", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	before := snapshotConversionQuoteRouteState(t, userID)
	first := performConversionQuoteRouteRequest(t, engine, userID, accessToken)
	require.Len(t, first.Data.Quotes, 4)
	byID := conversionQuoteRouteItemsByID(first.Data.Quotes)
	assert.Equal(t, model.ConversionQuoteCategoryConvertible, byID[strconv.Itoa(sourceSubscriptionID)].Category)
	assert.Equal(t, "100", byID[strconv.Itoa(sourceSubscriptionID)].CreditBasis)
	assert.NotNil(t, byID[strconv.Itoa(sourceSubscriptionID)].ReasonCodes)
	assert.Equal(t, model.ConversionCreditBasisCurrentPlan, byID[strconv.Itoa(sourceSubscriptionID)].CreditBasisSource)
	assert.Equal(t, "75", byID[strconv.Itoa(sourceSubscriptionID)].CurrentRemainingCredit)
	assert.Equal(t, "175", byID[strconv.Itoa(sourceSubscriptionID)].GrossCredit)
	assert.Equal(t, "25", byID[strconv.Itoa(sourceSubscriptionID)].CurrentDebt)
	assert.Equal(t, "25", byID[strconv.Itoa(sourceSubscriptionID)].EstimatedDebtOffset)
	assert.Equal(t, "150", byID[strconv.Itoa(sourceSubscriptionID)].NetAvailableCredit)
	assert.Equal(t, "40000000", byID[strconv.Itoa(sourceSubscriptionID)].SourcePriceMicros)
	assert.Equal(t, "CNY", byID[strconv.Itoa(sourceSubscriptionID)].SourceCurrency)
	assert.Equal(t, "CNY", byID[strconv.Itoa(sourceSubscriptionID)].TargetCurrency)
	assert.Equal(t, "100", byID[strconv.Itoa(sourceSubscriptionID)].ValuationCreditBasis)
	assert.Equal(t, "70000000", byID[strconv.Itoa(sourceSubscriptionID)].GrossCostMicros)
	assert.Equal(t, "60000000", byID[strconv.Itoa(sourceSubscriptionID)].NetCostMicros)
	assert.Equal(t, "400000", byID[strconv.Itoa(sourceSubscriptionID)].UnitValueNumeratorMicros)
	assert.Equal(t, "1", byID[strconv.Itoa(sourceSubscriptionID)].UnitValueDenominator)
	assert.Equal(t, model.CreditValuationRuleVersion, byID[strconv.Itoa(sourceSubscriptionID)].RuleVersion)
	assert.Equal(t, "1", byID[strconv.Itoa(sourceSubscriptionID)].FxNumerator)
	assert.Equal(t, "1", byID[strconv.Itoa(sourceSubscriptionID)].FxDenominator)
	assert.NotEqual(t, "0", byID[strconv.Itoa(sourceSubscriptionID)].FxCapturedAt)
	assert.Equal(t, model.CreditFXDirectionIdentity, byID[strconv.Itoa(sourceSubscriptionID)].FxDirection)
	assert.Equal(t, model.ConversionQuoteCategoryGrace, byID[strconv.Itoa(expiredSubscriptionID)].Category)
	assert.True(t, byID[strconv.Itoa(expiredSubscriptionID)].CanConfirm)
	assert.Contains(t, byID[strconv.Itoa(excludedSubscriptionID)].ReasonCodes, model.ConversionQuoteReasonMonthlyInviteSource)
	assert.Equal(t, model.ConversionQuoteCategoryExcluded, byID[strconv.Itoa(futureSubscriptionID)].Category)
	assert.False(t, byID[strconv.Itoa(futureSubscriptionID)].CanConfirm)
	assert.Contains(t, byID[strconv.Itoa(futureSubscriptionID)].ReasonCodes, model.ConversionQuoteReasonNotStarted)
	_, exposedBalance := byID[strconv.Itoa(balanceSubscriptionID)]
	assert.False(t, exposedBalance)
	assertConversionQuoteRouteStateUnchanged(t, userID, before)

	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9961).UpdateColumn("monthly_token_limit", int64(200)).Error)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", sourceSubscriptionID).UpdateColumn("token_used", int64(80)).Error)
	second := performConversionQuoteRouteRequest(t, engine, userID, accessToken)
	byID = conversionQuoteRouteItemsByID(second.Data.Quotes)
	assert.Equal(t, "200", byID[strconv.Itoa(sourceSubscriptionID)].CreditBasis)
	assert.Equal(t, "20", byID[strconv.Itoa(sourceSubscriptionID)].CurrentRemainingCredit)
	assert.Equal(t, "220", byID[strconv.Itoa(sourceSubscriptionID)].GrossCredit)
	assert.Equal(t, "195", byID[strconv.Itoa(sourceSubscriptionID)].NetAvailableCredit)
	assertConversionQuoteRouteStateUnchanged(t, userID, snapshotConversionQuoteRouteStateWithoutUsage(t, before, sourceSubscriptionID, 80))

	const unsafeCredit int64 = 9_007_199_254_740_993
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9961).UpdateColumn("monthly_token_limit", unsafeCredit).Error)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", sourceSubscriptionID).Updates(map[string]any{"token_limit": int64(0), "token_used": int64(0)}).Error)
	third := performConversionQuoteRouteRequest(t, engine, userID, accessToken)
	byID = conversionQuoteRouteItemsByID(third.Data.Quotes)
	assert.Equal(t, "9007199254740993", byID[strconv.Itoa(sourceSubscriptionID)].CreditBasis)
	assert.Equal(t, "9007199254740993", byID[strconv.Itoa(sourceSubscriptionID)].GrossCredit)
	assert.True(t, model.DB.Migrator().HasTable("subscription_conversions"))
	assert.Empty(t, third.Data.Conversions)
}

type conversionQuoteRouteState struct {
	SubscriptionCount int64
	LedgerCount       int64
	TokenLimits       map[int]int64
	TokenUsed         map[int]int64
	Statuses          map[int]string
}

func snapshotConversionQuoteRouteState(t *testing.T, userID int) conversionQuoteRouteState {
	t.Helper()
	state := conversionQuoteRouteState{TokenLimits: map[int]int64{}, TokenUsed: map[int]int64{}, Statuses: map[int]string{}}
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&state.SubscriptionCount).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("user_id = ?", userID).Count(&state.LedgerCount).Error)
	var subscriptions []model.UserSubscription
	require.NoError(t, model.DB.Select("id", "token_limit", "token_used", "status").Where("user_id = ?", userID).Find(&subscriptions).Error)
	for _, subscription := range subscriptions {
		state.TokenLimits[subscription.Id] = subscription.TokenLimit
		state.TokenUsed[subscription.Id] = subscription.TokenUsed
		state.Statuses[subscription.Id] = subscription.Status
	}
	return state
}

func snapshotConversionQuoteRouteStateWithoutUsage(t *testing.T, state conversionQuoteRouteState, subscriptionID int, tokenUsed int64) conversionQuoteRouteState {
	t.Helper()
	copyState := conversionQuoteRouteState{
		SubscriptionCount: state.SubscriptionCount,
		LedgerCount:       state.LedgerCount,
		TokenLimits:       map[int]int64{},
		TokenUsed:         map[int]int64{},
		Statuses:          map[int]string{},
	}
	for id, value := range state.TokenLimits {
		copyState.TokenLimits[id] = value
	}
	for id, value := range state.TokenUsed {
		copyState.TokenUsed[id] = value
	}
	for id, value := range state.Statuses {
		copyState.Statuses[id] = value
	}
	copyState.TokenUsed[subscriptionID] = tokenUsed
	return copyState
}

func assertConversionQuoteRouteStateUnchanged(t *testing.T, userID int, want conversionQuoteRouteState) {
	t.Helper()
	assert.Equal(t, want, snapshotConversionQuoteRouteState(t, userID))
}

func performConversionQuoteRouteRequest(t *testing.T, engine *gin.Engine, userID int, accessToken string) subscriptionConversionQuoteRouteResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/subscription/self/conversion-quotes", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", strconv.Itoa(userID))
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload subscriptionConversionQuoteRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.NotEqual(t, "0", payload.Data.DatabaseNow)
	return payload
}

func conversionQuoteRouteItemsByID(items []subscriptionConversionQuoteRouteItem) map[string]subscriptionConversionQuoteRouteItem {
	result := make(map[string]subscriptionConversionQuoteRouteItem, len(items))
	for _, item := range items {
		result[item.SourceSubscriptionId] = item
	}
	return result
}
