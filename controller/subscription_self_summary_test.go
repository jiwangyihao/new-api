package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gptAbuseWarningResetTableForSubscriptionSelfTest struct {
	Id                int    `gorm:"primaryKey"`
	UserId            int    `gorm:"not null;index;index:idx_gpt_abuse_reset_user_window,priority:1"`
	WindowStart       int64  `gorm:"bigint;not null;index;index:idx_gpt_abuse_reset_user_window,priority:2"`
	WindowEnd         int64  `gorm:"bigint;not null;index"`
	ResetAt           int64  `gorm:"bigint;not null;index"`
	ResetBy           int    `gorm:"default:0;index"`
	PreviousRawCount  int    `gorm:"default:0"`
	PreviousCount     int    `gorm:"default:0"`
	CutoffSignalLogID int    `gorm:"default:0;index"`
	Reason            string `gorm:"type:varchar(255);default:''"`
	CreatedAt         int64  `gorm:"bigint"`
}

func (gptAbuseWarningResetTableForSubscriptionSelfTest) TableName() string {
	return "gpt_abuse_warning_resets"
}

func setupSubscriptionSelfSummaryTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.CreditBalanceLedger{}, &model.GPTAbuseSignalLog{}, &model.GPTAbuseUserSuspension{}, &gptAbuseWarningResetTableForSubscriptionSelfTest{}))
	require.NoError(t, db.AutoMigrate(&model.ChannelGroup{}, &model.ChannelGroupChannel{}, &model.TokenGroupBinding{}))
}

func performGetSubscriptionSelfSummaryRequest(t *testing.T, userID int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/self", nil)
	ctx.Set("id", userID)
	GetSubscriptionSelf(ctx)
	return recorder
}

func seedSubscriptionSelfSummaryUser(t *testing.T, id int, username string) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{Id: id, Username: username, Status: common.UserStatusEnabled}).Error)
}

func seedSubscriptionSelfSummaryPlan(t *testing.T, id int, title string, businessCode string, tokenLimit int64, concurrencyLimit int, isTrial bool, inviteTrial bool) {
	t.Helper()
	model.InvalidateSubscriptionPlanCache(id)
	t.Cleanup(func() { model.InvalidateSubscriptionPlanCache(id) })
	plan := &model.SubscriptionPlan{
		Id:                id,
		Title:             title,
		DurationUnit:      model.SubscriptionDurationMonth,
		DurationValue:     1,
		Enabled:           true,
		PublicVisible:     true,
		TotalAmount:       1,
		MonthlyTokenLimit: tokenLimit,
		ConcurrencyLimit:  concurrencyLimit,
		IsTrial:           isTrial,
		InviteTrial:       inviteTrial,
		RewardEligible:    true,
	}
	if businessCode != "" {
		plan.BusinessCode = &businessCode
	}
	require.NoError(t, model.DB.Create(plan).Error)
}

func seedSubscriptionSelfSummarySubscription(t *testing.T, id int, userID int, planID int, tokenLimit int64, tokenUsed int64, concurrencyLimit int, grantReason string, endOffsetSeconds int64) {
	t.Helper()
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id:               id,
		UserId:           userID,
		PlanId:           planID,
		Status:           "active",
		AmountTotal:      1,
		TokenLimit:       tokenLimit,
		TokenUsed:        tokenUsed,
		ConcurrencyLimit: concurrencyLimit,
		GrantReason:      grantReason,
		StartTime:        now - 60,
		EndTime:          now + endOffsetSeconds,
		NextResetTime:    now + 1800,
	}).Error)
}

func subscriptionSelfSummaryData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, true, payload["success"])
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok, "response data must be an object")
	return data
}

func requireSubscriptionSelfSummary(t *testing.T, data map[string]interface{}) map[string]interface{} {
	t.Helper()
	summaryValue, ok := data["summary"]
	require.True(t, ok, "self subscription response must include summary")
	summary, ok := summaryValue.(map[string]interface{})
	require.True(t, ok, "summary must be an object")
	return summary
}

func requireSubscriptionSelfSummaryFields(t *testing.T, summary map[string]interface{}) {
	t.Helper()
	for _, field := range []string{
		"active_count",
		"subscription_id",
		"plan_id",
		"primary_plan_title",
		"token_limit",
		"token_used",
		"token_remaining",
		"token_unlimited",
		"concurrency_limit",
		"gpt_abuse_warning_limit",
		"gpt_abuse_warning_count",
		"gpt_abuse_warning_remaining",
		"gpt_abuse_limit_enabled",
	} {
		assert.Contains(t, summary, field)
	}
}

func summaryInt64(t *testing.T, summary map[string]interface{}, field string) int64 {
	t.Helper()
	value, ok := summary[field]
	require.Truef(t, ok, "summary.%s must exist", field)
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		t.Fatalf("summary.%s must be numeric, got %T", field, value)
		return 0
	}
}

func summaryBool(t *testing.T, summary map[string]interface{}, field string) bool {
	t.Helper()
	value, ok := summary[field]
	require.Truef(t, ok, "summary.%s must exist", field)
	result, ok := value.(bool)
	require.Truef(t, ok, "summary.%s must be bool, got %T", field, value)
	return result
}

func summaryString(t *testing.T, summary map[string]interface{}, field string) string {
	t.Helper()
	value, ok := summary[field]
	require.Truef(t, ok, "summary.%s must exist", field)
	result, ok := value.(string)
	require.Truef(t, ok, "summary.%s must be string, got %T", field, value)
	return result
}

func TestGetSubscriptionSelfReturnsSummaryAndCompatFields(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 8101
	const planID = 8111
	const subscriptionID = 8121
	seedSubscriptionSelfSummaryUser(t, userID, "self_summary_compat")
	seedSubscriptionSelfSummaryPlan(t, planID, "Basic Self Summary", "self-summary-basic", 1000, 3, false, false)
	seedSubscriptionSelfSummarySubscription(t, subscriptionID, userID, planID, 1000, 250, 3, "order", 3600)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	data := subscriptionSelfSummaryData(t, recorder)
	assert.Contains(t, data, "billing_preference")
	assert.Contains(t, data, "subscriptions")
	assert.Contains(t, data, "all_subscriptions")
	assert.Equal(t, "subscription_first", data["billing_preference"])
	summary := requireSubscriptionSelfSummary(t, data)
	requireSubscriptionSelfSummaryFields(t, summary)
	assert.Equal(t, int64(1), summaryInt64(t, summary, "active_count"))
	assert.Equal(t, int64(subscriptionID), summaryInt64(t, summary, "subscription_id"))
	assert.Equal(t, int64(planID), summaryInt64(t, summary, "plan_id"))
	assert.Equal(t, "Basic Self Summary", summaryString(t, summary, "primary_plan_title"))
	assert.Equal(t, int64(1000), summaryInt64(t, summary, "token_limit"))
	assert.Equal(t, int64(250), summaryInt64(t, summary, "token_used"))
	assert.Equal(t, int64(750), summaryInt64(t, summary, "token_remaining"))
	assert.False(t, summaryBool(t, summary, "token_unlimited"))
	assert.Equal(t, int64(3), summaryInt64(t, summary, "concurrency_limit"))
	assert.Equal(t, int64(5), summaryInt64(t, summary, "gpt_abuse_warning_limit"))
	assert.Equal(t, int64(0), summaryInt64(t, summary, "gpt_abuse_warning_count"))
	assert.Equal(t, int64(5), summaryInt64(t, summary, "gpt_abuse_warning_remaining"))
	assert.False(t, summaryBool(t, summary, "gpt_abuse_limit_enabled"))
}

func TestGetSubscriptionSelfReturnsCreditBalanceStateHistoryAndPreference(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.CreditBalanceLedger{}))
	const userID = 8151
	const planID = 8152
	const subscriptionID = 8153
	user := model.User{Id: userID, Username: "credit_balance_self", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = subscriptionID
	setting.LastSubscriptionPurchaseMode = model.SubscriptionPurchaseModeCreditBalance
	user.SetSetting(setting)
	require.NoError(t, model.DB.Create(&user).Error)
	code := "credit_balance_self"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, ConcurrencyLimit: 2, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subscriptionID, UserId: userID, PlanId: planID, EntitlementType: model.SubscriptionEntitlementCreditBalance, Status: "active", TokenLimit: 100, TokenUsed: 125, EndTime: 0, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	require.NoError(t, model.DB.Create(&model.CreditBalanceLedger{UserId: userID, UserSubscriptionId: subscriptionID, Type: model.CreditBalanceLedgerTypePurchase, IdempotencyKey: "credit-self", SourceType: model.CreditBalanceLedgerSourceSubscriptionOrder, SourceId: 8154, GrossCredit: 100, DebtOffset: 25, BalanceBefore: -25, BalanceAfter: 75, AvailableCreditAfter: 75, CreatedAt: common.GetTimestamp()}).Error)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	data := subscriptionSelfSummaryData(t, recorder)
	assert.Equal(t, model.SubscriptionPurchaseModeCreditBalance, data["last_subscription_purchase_mode"])
	creditState, ok := data["credit_balance"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(0), creditState["available_credit"])
	assert.Equal(t, float64(25), creditState["settlement_debt"])
	assert.Equal(t, model.CreditBalanceStatusDebt, creditState["status"])
	assert.Equal(t, true, creditState["active"])
	ledger, ok := data["credit_balance_ledger"].([]interface{})
	require.True(t, ok)
	require.Len(t, ledger, 1)
	entry := ledger[0].(map[string]interface{})
	assert.Equal(t, float64(100), entry["gross_credit"])
	assert.Equal(t, float64(25), entry["debt_offset"])
	active, ok := data["subscriptions"].([]interface{})
	require.True(t, ok)
	require.Len(t, active, 1)
}

func TestGetSubscriptionSelfSummaryUsesPrimaryBillableSubscription(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 8201
	seedSubscriptionSelfSummaryUser(t, userID, "self_summary_primary")
	seedSubscriptionSelfSummaryPlan(t, 8211, "Primary Earlier", "self-summary-primary-a", 1000, 1, false, false)
	seedSubscriptionSelfSummaryPlan(t, 8212, "Later Larger", "self-summary-primary-b", 9999, 50, false, false)
	seedSubscriptionSelfSummarySubscription(t, 8221, userID, 8211, 1000, 200, 1, "order", 3600)
	seedSubscriptionSelfSummarySubscription(t, 8222, userID, 8212, 9999, 0, 50, "order", 7200)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	summary := requireSubscriptionSelfSummary(t, subscriptionSelfSummaryData(t, recorder))
	assert.Equal(t, int64(8221), summaryInt64(t, summary, "subscription_id"))
	assert.Equal(t, int64(8211), summaryInt64(t, summary, "plan_id"))
	assert.Equal(t, "Primary Earlier", summaryString(t, summary, "primary_plan_title"))
	assert.Equal(t, int64(1000), summaryInt64(t, summary, "token_limit"))
	assert.Equal(t, int64(200), summaryInt64(t, summary, "token_used"))
	assert.Equal(t, int64(800), summaryInt64(t, summary, "token_remaining"))
	assert.Equal(t, int64(1), summaryInt64(t, summary, "concurrency_limit"))
}

func TestGetSubscriptionSelfSummaryIncludesGPTAbuseWarningUsage(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	oldDefault := common.GPTAbuseDefaultWarningLimit
	oldEnabled := common.GPTAbuseLimitEnabled
	common.GPTAbuseDefaultWarningLimit = 5
	common.GPTAbuseLimitEnabled = true
	t.Cleanup(func() {
		common.GPTAbuseDefaultWarningLimit = oldDefault
		common.GPTAbuseLimitEnabled = oldEnabled
	})
	const userID = 8251
	const planID = 8252
	seedSubscriptionSelfSummaryUser(t, userID, "self_summary_gpt_abuse")
	seedSubscriptionSelfSummaryPlan(t, planID, "GPT Abuse Plan", "self-summary-gpt-abuse", 1000, 3, false, false)
	seedSubscriptionSelfSummarySubscription(t, 8253, userID, planID, 1000, 100, 3, "order", 3600)
	start, _ := model.GPTAbuseDayWindow(common.GetTimestamp())
	records := []model.GPTAbuseSignalLog{
		{CreatedAt: start + 10, UserId: userID, Kind: "cyber_policy", CountEligible: true, DedupeKey: "summary-gpt-abuse-a"},
		{CreatedAt: start + 20, UserId: userID, Kind: "content_policy_violation", CountEligible: true, DedupeKey: "summary-gpt-abuse-b"},
	}
	require.NoError(t, model.DB.Create(&records).Error)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)
	summary := requireSubscriptionSelfSummary(t, subscriptionSelfSummaryData(t, recorder))

	assert.Equal(t, int64(5), summaryInt64(t, summary, "gpt_abuse_warning_limit"))
	assert.Equal(t, int64(2), summaryInt64(t, summary, "gpt_abuse_warning_count"))
	assert.Equal(t, int64(3), summaryInt64(t, summary, "gpt_abuse_warning_remaining"))
	assert.True(t, summaryBool(t, summary, "gpt_abuse_limit_enabled"))
}

func TestGetSubscriptionSelfSummaryKeepsExhaustedSingleActiveCandidate(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 8301
	seedSubscriptionSelfSummaryUser(t, userID, "self_summary_exhausted_active")
	seedSubscriptionSelfSummaryPlan(t, 8311, "Exhausted Earlier", "self-summary-exhausted-a", 1000, 1, false, false)
	seedSubscriptionSelfSummaryPlan(t, 8312, "Usable Later", "self-summary-exhausted-b", 9999, 9, false, false)
	seedSubscriptionSelfSummarySubscription(t, 8321, userID, 8311, 1000, 1000, 1, "order", 3600)
	seedSubscriptionSelfSummarySubscription(t, 8322, userID, 8312, 9999, 0, 9, "order", 7200)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	summary := requireSubscriptionSelfSummary(t, subscriptionSelfSummaryData(t, recorder))
	assert.Equal(t, int64(1), summaryInt64(t, summary, "active_count"))
	assert.Equal(t, int64(8321), summaryInt64(t, summary, "subscription_id"))
	assert.Equal(t, int64(8311), summaryInt64(t, summary, "plan_id"))
	assert.Equal(t, "Exhausted Earlier", summaryString(t, summary, "primary_plan_title"))
	assert.Equal(t, int64(1000), summaryInt64(t, summary, "token_limit"))
	assert.Equal(t, int64(1000), summaryInt64(t, summary, "token_used"))
	assert.Zero(t, summaryInt64(t, summary, "token_remaining"))
	assert.False(t, summaryBool(t, summary, "token_unlimited"))
}

func TestGetSubscriptionSelfSummaryReturnsExplicitUnlimitedTrial(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 8401
	const planID = 8411
	const subscriptionID = 8421
	seedSubscriptionSelfSummaryUser(t, userID, "self_summary_trial")
	seedSubscriptionSelfSummaryPlan(t, planID, "Trial Unlimited", "trial_24h", 0, 1, true, false)
	seedSubscriptionSelfSummarySubscription(t, subscriptionID, userID, planID, 0, 123, 1, "trial_code", 3600)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	summary := requireSubscriptionSelfSummary(t, subscriptionSelfSummaryData(t, recorder))
	assert.Equal(t, int64(1), summaryInt64(t, summary, "active_count"))
	assert.Equal(t, int64(subscriptionID), summaryInt64(t, summary, "subscription_id"))
	assert.Equal(t, int64(planID), summaryInt64(t, summary, "plan_id"))
	assert.Equal(t, "Trial Unlimited", summaryString(t, summary, "primary_plan_title"))
	assert.Equal(t, int64(0), summaryInt64(t, summary, "token_limit"))
	assert.Equal(t, int64(123), summaryInt64(t, summary, "token_used"))
	assert.Equal(t, int64(0), summaryInt64(t, summary, "token_remaining"))
	assert.True(t, summaryBool(t, summary, "token_unlimited"))
	assert.Equal(t, int64(1), summaryInt64(t, summary, "concurrency_limit"))
}

func TestGetSubscriptionSelfSummaryReturnsZeroWhenNoEligibleSubscription(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 8501
	seedSubscriptionSelfSummaryUser(t, userID, "self_summary_no_eligible")
	seedSubscriptionSelfSummaryPlan(t, 8511, "Expired Only", "self-summary-no-eligible", 1000, 2, false, false)
	seedSubscriptionSelfSummarySubscription(t, 8521, userID, 8511, 1000, 1000, 2, "order", -1)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	summary := requireSubscriptionSelfSummary(t, subscriptionSelfSummaryData(t, recorder))
	assert.Equal(t, int64(0), summaryInt64(t, summary, "active_count"))
	assert.Equal(t, int64(0), summaryInt64(t, summary, "token_limit"))
	assert.Equal(t, int64(0), summaryInt64(t, summary, "token_used"))
	assert.Equal(t, int64(0), summaryInt64(t, summary, "token_remaining"))
	assert.Equal(t, int64(0), summaryInt64(t, summary, "concurrency_limit"))
	assert.False(t, summaryBool(t, summary, "token_unlimited"))
}

func TestGetSubscriptionSelfSummaryDoesNotTreatLegacyZeroLimitAsUnlimited(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 8601
	seedSubscriptionSelfSummaryUser(t, userID, "self_summary_legacy_zero")
	seedSubscriptionSelfSummaryPlan(t, 8611, "Legacy Zero Limit", "", 0, 0, false, false)
	seedSubscriptionSelfSummarySubscription(t, 8621, userID, 8611, 0, 0, 0, "admin", 3600)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	summary := requireSubscriptionSelfSummary(t, subscriptionSelfSummaryData(t, recorder))
	assert.False(t, summaryBool(t, summary, "token_unlimited"))
}
