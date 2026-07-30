package router

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSubscriptionConversionRouteRejectsEveryIneligibleStateWithoutSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		wantReason string
		mutate     func(*testing.T, *gorm.DB, int64)
	}{
		{
			name:       "global conversion disabled",
			wantReason: model.ConversionQuoteReasonGlobalDisabled,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.SubscriptionPlan{}).Where("id = ?", conversionEligibilityCreditPlanID).UpdateColumn("credit_balance_conversion_enabled", false).Error)
			},
		},
		{
			name:       "entitlement is not timed",
			wantReason: model.ConversionQuoteReasonEntitlementNotTimed,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).UpdateColumn("entitlement_type", "legacy").Error)
			},
		},
		{
			name:       "source plan is missing",
			wantReason: model.ConversionQuoteReasonPlanNotFound,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).UpdateColumn("plan_id", 999_999).Error)
			},
		},
		{
			name:       "source plan is not timed",
			wantReason: model.ConversionQuoteReasonPlanNotTimed,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.SubscriptionPlan{}).Where("id = ?", conversionEligibilityTimedPlanID).UpdateColumn("entitlement_type", "legacy").Error)
			},
		},
		{
			name:       "duration is not one month",
			wantReason: model.ConversionQuoteReasonDurationNotOneMonth,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.SubscriptionPlan{}).Where("id = ?", conversionEligibilityTimedPlanID).UpdateColumn("duration_value", 2).Error)
			},
		},
		{
			name:       "reset is not monthly",
			wantReason: model.ConversionQuoteReasonResetNotMonthly,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.SubscriptionPlan{}).Where("id = ?", conversionEligibilityTimedPlanID).UpdateColumn("quota_reset_period", model.SubscriptionResetDaily).Error)
			},
		},
		{
			name:       "monthly credit is zero",
			wantReason: model.ConversionQuoteReasonMonthlyCreditInvalid,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.SubscriptionPlan{}).Where("id = ?", conversionEligibilityTimedPlanID).UpdateColumn("monthly_token_limit", 0).Error)
			},
		},
		{
			name:       "trial plan",
			wantReason: model.ConversionQuoteReasonTrialPlan,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.SubscriptionPlan{}).Where("id = ?", conversionEligibilityTimedPlanID).UpdateColumn("is_trial", true).Error)
			},
		},
		{
			name:       "monthly invitation plan",
			wantReason: model.ConversionQuoteReasonMonthlyInvitePlan,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.SubscriptionPlan{}).Where("id = ?", conversionEligibilityTimedPlanID).UpdateColumn("invite_trial", true).Error)
			},
		},
		{
			name:       "trial source",
			wantReason: model.ConversionQuoteReasonTrialSource,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).Updates(map[string]any{"grant_reason": "trial_code", "source": "trial_code", "last_grant_source": "trial_code"}).Error)
			},
		},
		{
			name:       "monthly invitation source",
			wantReason: model.ConversionQuoteReasonMonthlyInviteSource,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).Updates(map[string]any{"grant_reason": model.SubscriptionGrantMonthlyInviteEntitlement, "source": model.SubscriptionGrantMonthlyInviteEntitlement, "last_grant_source": model.SubscriptionGrantMonthlyInviteEntitlement}).Error)
			},
		},
		{
			name:       "unknown source",
			wantReason: model.ConversionQuoteReasonSourceNotEligible,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).Updates(map[string]any{"grant_reason": "system", "source": "system", "last_grant_source": "system"}).Error)
			},
		},
		{
			name:       "plan conversion disabled",
			wantReason: model.ConversionQuoteReasonPlanDisabled,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.SubscriptionPlan{}).Where("id = ?", conversionEligibilityTimedPlanID).UpdateColumn("timed_conversion_enabled", false).Error)
			},
		},
		{
			name:       "status is not eligible",
			wantReason: model.ConversionQuoteReasonStatusNotEligible,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).UpdateColumn("status", model.SubscriptionStatusCancelled).Error)
			},
		},
		{
			name:       "subscription has not started",
			wantReason: model.ConversionQuoteReasonNotStarted,
			mutate: func(t *testing.T, db *gorm.DB, now int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).Updates(map[string]any{"start_time": now + 60, "end_time": now + model.TimedSubscriptionConversionBlockSeconds + 60}).Error)
			},
		},
		{
			name:       "outside grace period",
			wantReason: model.ConversionQuoteReasonOutsideGrace,
			mutate: func(t *testing.T, db *gorm.DB, now int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).Updates(map[string]any{"status": "expired", "end_time": now - model.TimedSubscriptionConversionGraceSeconds - 1}).Error)
			},
		},
		{
			name:       "grant time missing",
			wantReason: model.ConversionQuoteReasonGrantTimeMissing,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).UpdateColumn("last_granted_at", 0).Error)
			},
		},
		{
			name:       "cooldown active",
			wantReason: model.ConversionQuoteReasonCooldownActive,
			mutate: func(t *testing.T, db *gorm.DB, now int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).UpdateColumn("last_granted_at", now-1).Error)
			},
		},
		{
			name:       "gross credit is zero",
			wantReason: model.ConversionQuoteReasonGrossNotPositive,
			mutate: func(t *testing.T, db *gorm.DB, now int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).Updates(map[string]any{"token_limit": 100, "token_used": 100, "end_time": now + 60}).Error)
			},
		},
		{
			name:       "calculation data is invalid",
			wantReason: model.ConversionQuoteCalculationInvalidData,
			mutate: func(t *testing.T, db *gorm.DB, _ int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).UpdateColumn("token_limit", -1).Error)
			},
		},
		{
			name:       "calculation overflows",
			wantReason: model.ConversionQuoteCalculationArithmeticOverflow,
			mutate: func(t *testing.T, db *gorm.DB, now int64) {
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).Updates(map[string]any{"token_limit": 0, "token_used": 0, "end_time": now + 3*model.TimedSubscriptionConversionBlockSeconds, "last_grant_credit_snapshot": int64(math.MaxInt64)}).Error)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, engine, accessToken, now := seedSubscriptionConversionEligibilityRouteTest(t)
			test.mutate(t, db, now)

			var sourceBefore model.UserSubscription
			require.NoError(t, db.First(&sourceBefore, conversionEligibilitySourceID).Error)
			var userBefore model.User
			require.NoError(t, db.First(&userBefore, conversionEligibilityUserID).Error)
			var subscriptionCountBefore int64
			require.NoError(t, db.Model(&model.UserSubscription{}).Count(&subscriptionCountBefore).Error)

			response := performSubscriptionConversionRouteRequest(t, engine, conversionEligibilityUserID, accessToken,
				`{"subscription_id":"9952","idempotency_key":"reject-ineligible"}`)
			assert.False(t, response.Success)
			assert.Contains(t, response.Message, test.wantReason)

			var sourceAfter model.UserSubscription
			require.NoError(t, db.First(&sourceAfter, conversionEligibilitySourceID).Error)
			assert.Equal(t, sourceBefore.Status, sourceAfter.Status)
			assert.Equal(t, sourceBefore.EntitlementType, sourceAfter.EntitlementType)
			assert.Equal(t, sourceBefore.PlanId, sourceAfter.PlanId)
			assert.Equal(t, sourceBefore.TokenLimit, sourceAfter.TokenLimit)
			assert.Equal(t, sourceBefore.TokenUsed, sourceAfter.TokenUsed)
			assert.Zero(t, sourceAfter.ConvertedAt)
			assert.Zero(t, sourceAfter.ConversionId)
			assert.Zero(t, sourceAfter.ConvertedToSubscriptionId)
			var userAfter model.User
			require.NoError(t, db.First(&userAfter, conversionEligibilityUserID).Error)
			assert.Equal(t, userBefore.Setting, userAfter.Setting)
			var subscriptionCountAfter int64
			require.NoError(t, db.Model(&model.UserSubscription{}).Count(&subscriptionCountAfter).Error)
			assert.Equal(t, subscriptionCountBefore, subscriptionCountAfter)
			assertSubscriptionConversionSideEffectCounts(t, db, 0, 0, 0)
		})
	}
}

const (
	conversionEligibilityUserID       = 9_951
	conversionEligibilitySourceID     = 9_952
	conversionEligibilityCreditPlanID = 9_953
	conversionEligibilityTimedPlanID  = 9_954
)

func seedSubscriptionConversionEligibilityRouteTest(t *testing.T) (*gorm.DB, *gin.Engine, string, int64) {
	t.Helper()
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
	accessToken := "subscription-conversion-eligibility-token"
	settingBytes, err := common.Marshal(map[string]any{
		"active_subscription_id":        conversionEligibilitySourceID,
		"subscription_billing_strategy": model.SubscriptionBillingStrategySingleActive,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Id: conversionEligibilityUserID, Username: "subscription-conversion-eligibility", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, AccessToken: &accessToken, Setting: string(settingBytes),
	}).Error)
	creditCode := "subscription_conversion_eligibility_credit"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: conversionEligibilityCreditPlanID, Title: "Credit balance", EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true, CreditBalanceConversionEnabled: true,
	}).Error)
	timedCode := "subscription_conversion_eligibility_timed"
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Id: conversionEligibilityTimedPlanID, Title: "Monthly convertible", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100, TimedConversionEnabled: true,
	}).Error)
	now := model.GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id: conversionEligibilitySourceID, UserId: conversionEligibilityUserID, PlanId: conversionEligibilityTimedPlanID,
		EntitlementType: model.SubscriptionEntitlementTimed, TokenLimit: 100, TokenUsed: 25,
		GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
		StartTime: now - 40*24*60*60, EndTime: now + model.TimedSubscriptionConversionBlockSeconds, Status: model.SubscriptionStatusActive,
		LastGrantedAt: now - 40*24*60*60, LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive, LastGrantSource: model.SubscriptionGrantOrder,
	}).Error)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)
	return db, engine, accessToken, now
}

func assertSubscriptionConversionSideEffectCounts(t *testing.T, db *gorm.DB, conversions int64, ledgers int64, rewards int64) {
	t.Helper()
	var conversionCount int64
	require.NoError(t, db.Model(&model.SubscriptionConversion{}).Count(&conversionCount).Error)
	assert.Equal(t, conversions, conversionCount)
	var ledgerCount int64
	require.NoError(t, db.Model(&model.CreditBalanceLedger{}).Count(&ledgerCount).Error)
	assert.Equal(t, ledgers, ledgerCount)
	var rewardCount int64
	require.NoError(t, db.Model(&model.InvitationRewardEvent{}).Count(&rewardCount).Error)
	assert.Equal(t, rewards, rewardCount)
}

func TestSubscriptionConversionRouteRejectsAnotherUsersSource(t *testing.T) {
	db, engine, accessToken, _ := seedSubscriptionConversionEligibilityRouteTest(t)
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("id = ?", conversionEligibilitySourceID).UpdateColumn("user_id", conversionEligibilityUserID+1).Error)

	response := performSubscriptionConversionRouteRequest(t, engine, conversionEligibilityUserID, accessToken,
		`{"subscription_id":"9952","idempotency_key":"wrong-owner"}`)
	assert.False(t, response.Success)
	assertSubscriptionConversionSideEffectCounts(t, db, 0, 0, 0)
}
