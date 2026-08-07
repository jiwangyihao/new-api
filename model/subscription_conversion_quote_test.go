package model

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const conversionQuoteTestNow int64 = 1_800_000_000

func TestRecalculateTimedSubscriptionConversionQuoteFormulaBoundaries(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	const blockSeconds int64 = 31 * 24 * 60 * 60

	tests := []struct {
		name             string
		remainingSeconds int64
		wantBlocks       int64
		wantGross        int64
	}{
		{name: "no_remaining_time_keeps_current_period_unused_credit", remainingSeconds: 0, wantBlocks: 0, wantGross: 25},
		{name: "less_than_31_days_keeps_only_current_period_unused_credit", remainingSeconds: blockSeconds - 1, wantBlocks: 0, wantGross: 25},
		{name: "exact_31_days_adds_one_full_future_period", remainingSeconds: blockSeconds, wantBlocks: 1, wantGross: 125},
		{name: "31_days_plus_partial_does_not_prorate_another_period", remainingSeconds: blockSeconds + 24*60*60, wantBlocks: 1, wantGross: 125},
		{name: "multiple_full_periods_ignore_partial_remainder", remainingSeconds: 3*blockSeconds + 17, wantBlocks: 3, wantGross: 325},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			userID := 10_000 + index
			planID := 20_000 + index
			subscriptionID := 30_000 + index
			seedConversionQuoteTimedPlan(t, planID, 100)
			snapshot := int64(100)
			subscription := UserSubscription{
				Id:                      subscriptionID,
				UserId:                  userID,
				PlanId:                  planID,
				EntitlementType:         SubscriptionEntitlementTimed,
				TokenLimit:              75,
				TokenUsed:               50,
				GrantReason:             SubscriptionGrantOrder,
				Source:                  SubscriptionGrantOrder,
				StartTime:               conversionQuoteTestNow - 40*24*60*60,
				EndTime:                 conversionQuoteTestNow + test.remainingSeconds,
				Status:                  "active",
				LastGrantedAt:           conversionQuoteTestNow - TimedSubscriptionConversionCooldownSeconds,
				LastGrantCreditSnapshot: &snapshot,
			}
			require.NoError(t, DB.Create(&subscription).Error)

			quote, err := RecalculateTimedSubscriptionConversionQuoteTx(DB, userID, subscriptionID, conversionQuoteTestNow)
			require.NoError(t, err)
			require.NotNil(t, quote)
			assert.Equal(t, test.remainingSeconds, quote.RemainingSeconds)
			assert.Equal(t, test.wantBlocks, quote.Full31DayBlocks)
			assert.Equal(t, int64(100), quote.CreditBasis)
			assert.Equal(t, ConversionCreditBasisGrantSnapshot, quote.CreditBasisSource)
			assert.Equal(t, int64(25), quote.CurrentRemainingCredit)
			assert.Equal(t, test.wantGross, quote.GrossCredit)
			assert.True(t, quote.Eligible)
			assert.True(t, quote.CanConfirm)
			assert.Empty(t, quote.ReasonCodes)
		})
	}
}

func TestRecalculateTimedSubscriptionConversionQuoteLargeCreditExamplesDoNotDoubleCountPartialTime(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	const blockSeconds int64 = TimedSubscriptionConversionBlockSeconds
	tests := []struct {
		name             string
		remainingSeconds int64
		creditBasis      int64
		currentRemaining int64
		wantBlocks       int64
		wantGross        int64
	}{
		{name: "under_one_block", remainingSeconds: blockSeconds - 1, creditBasis: 5_000_000_000, currentRemaining: 4_915_690_135, wantBlocks: 0, wantGross: 4_915_690_135},
		{name: "exactly_one_block", remainingSeconds: blockSeconds, creditBasis: 36_000_000_000, currentRemaining: 35_553_920_000, wantBlocks: 1, wantGross: 71_553_920_000},
		{name: "one_block_plus_partial_time", remainingSeconds: blockSeconds + 15*24*60*60, creditBasis: 36_000_000_000, currentRemaining: 35_553_920_000, wantBlocks: 1, wantGross: 71_553_920_000},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			userID := 11_000 + index
			planID := 21_000 + index
			subscriptionID := 31_000 + index
			seedConversionQuoteTimedPlan(t, planID, test.creditBasis)
			snapshot := test.creditBasis
			subscription := UserSubscription{
				Id: subscriptionID, UserId: userID, PlanId: planID,
				EntitlementType: SubscriptionEntitlementTimed,
				TokenLimit:      test.creditBasis, TokenUsed: test.creditBasis - test.currentRemaining,
				GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
				StartTime: conversionQuoteTestNow - 40*24*60*60,
				EndTime:   conversionQuoteTestNow + test.remainingSeconds,
				Status:    "active", LastGrantedAt: conversionQuoteTestNow - TimedSubscriptionConversionCooldownSeconds,
				LastGrantCreditSnapshot: &snapshot,
			}
			require.NoError(t, DB.Create(&subscription).Error)
			quote, err := RecalculateTimedSubscriptionConversionQuoteTx(DB, userID, subscriptionID, conversionQuoteTestNow)
			require.NoError(t, err)
			assert.Equal(t, test.wantBlocks, quote.Full31DayBlocks)
			assert.Equal(t, test.currentRemaining, quote.CurrentRemainingCredit)
			assert.Equal(t, test.wantGross, quote.GrossCredit)
		})
	}
}

func TestRecalculateTimedSubscriptionConversionQuoteRejectsFutureSubscription(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	seedConversionQuoteTimedPlan(t, 20_100, 100)
	snapshot := int64(100)
	subscription := seedConversionQuoteSubscription(
		t,
		30_100,
		10_100,
		20_100,
		SubscriptionGrantOrder,
		conversionQuoteTestNow+TimedSubscriptionConversionBlockSeconds,
		&snapshot,
	)
	require.NoError(t, DB.Model(&UserSubscription{}).
		Where("id = ?", subscription.Id).
		UpdateColumn("start_time", conversionQuoteTestNow+1).Error)

	quote, err := RecalculateTimedSubscriptionConversionQuoteTx(
		DB,
		subscription.UserId,
		subscription.Id,
		conversionQuoteTestNow,
	)
	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.False(t, quote.Eligible)
	assert.False(t, quote.CanConfirm)
	assert.Equal(t, ConversionQuoteCategoryExcluded, quote.Category)
	assert.Contains(t, quote.ReasonCodes, ConversionQuoteReasonNotStarted)
}

func TestListTimedSubscriptionConversionQuotesHandlesNullableLegacyEntitlement(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	db, err := gorm.Open(sqlite.Open("file:conversion_quote_nullable_entitlement?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
	})

	require.NoError(t, db.Exec(`CREATE TABLE user_subscriptions (
		id integer PRIMARY KEY,
		user_id integer NOT NULL,
		plan_id integer NOT NULL,
		entitlement_type varchar(32),
		token_limit bigint,
		token_used bigint,
		grant_reason varchar(32),
		last_granted_at bigint,
		last_grant_credit_snapshot bigint,
		last_grant_time_source varchar(64),
		last_grant_source varchar(32),
		start_time bigint,
		end_time bigint,
		status varchar(32),
		source varchar(32)
	)`).Error)
	require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}))
	require.NoError(t, db.AutoMigrate(&SubscriptionConversion{}))
	require.NoError(t, db.AutoMigrate(&CreditBalanceLedger{}))
	seedConversionQuoteCreditBalancePlan(t)
	seedConversionQuoteTimedPlan(t, 29_101, 100)
	require.NoError(t, db.Exec(
		"INSERT INTO user_subscriptions (id, user_id, plan_id, entitlement_type, token_limit, token_used, grant_reason, last_granted_at, last_grant_credit_snapshot, last_grant_time_source, last_grant_source, start_time, end_time, status, source) VALUES (?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)",
		39_101, 19_101, 29_101, 100, 25, SubscriptionGrantOrder,
		conversionQuoteTestNow-TimedSubscriptionConversionCooldownSeconds, 100,
		SubscriptionGrantTimeSourceLive, SubscriptionGrantOrder,
		conversionQuoteTestNow-TimedSubscriptionConversionBlockSeconds,
		conversionQuoteTestNow+TimedSubscriptionConversionBlockSeconds,
		SubscriptionGrantOrder,
	).Error)

	result, err := ListTimedSubscriptionConversionQuotes(19_101)
	require.NoError(t, err)
	require.Len(t, result.Quotes, 1)
	assert.Empty(t, result.Quotes[0].EntitlementType)
	assert.False(t, result.Quotes[0].CanConfirm)
	assert.Contains(t, result.Quotes[0].ReasonCodes, ConversionQuoteReasonEntitlementNotTimed)
	assert.Empty(t, result.Quotes[0].Status)
}

func TestListTimedSubscriptionConversionQuotesLimitsConversionHistory(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	const userID = 19_102
	for index := range 105 {
		require.NoError(t, DB.Create(&SubscriptionConversion{
			UserId: userID, IdempotencyKey: fmt.Sprintf("history-%d", index),
			SourceSubscriptionId: 40_000 + index, LedgerId: 50_000 + index,
		}).Error)
	}

	result, err := ListTimedSubscriptionConversionQuotes(userID)
	require.NoError(t, err)
	require.Len(t, result.Conversions, 100)
	assert.Greater(t, result.Conversions[0].Id, result.Conversions[99].Id)
}

func TestUserSubscriptionGrantMetadataIsNotSerialized(t *testing.T) {
	snapshot := int64(9_007_199_254_740_993)
	data, err := common.Marshal(UserSubscription{
		Id:                      39_102,
		LastGrantedAt:           conversionQuoteTestNow,
		LastGrantCreditSnapshot: &snapshot,
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	})
	require.NoError(t, err)
	serialized := string(data)
	assert.NotContains(t, serialized, "last_granted_at")
	assert.NotContains(t, serialized, "last_grant_credit_snapshot")
	assert.NotContains(t, serialized, "last_grant_time_source")
	assert.NotContains(t, serialized, "last_grant_source")
}

func TestRecalculateTimedSubscriptionConversionQuoteRejectsArithmeticOverflow(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	seedConversionQuoteTimedPlan(t, 21_001, 100)
	snapshot := int64(math.MaxInt64)
	subscription := UserSubscription{
		Id:                      31_001,
		UserId:                  11_001,
		PlanId:                  21_001,
		EntitlementType:         SubscriptionEntitlementTimed,
		TokenLimit:              0,
		TokenUsed:               0,
		GrantReason:             SubscriptionGrantOrder,
		Source:                  SubscriptionGrantOrder,
		StartTime:               conversionQuoteTestNow - 40*24*60*60,
		EndTime:                 conversionQuoteTestNow + 2*TimedSubscriptionConversionBlockSeconds,
		Status:                  "active",
		LastGrantedAt:           conversionQuoteTestNow - TimedSubscriptionConversionCooldownSeconds,
		LastGrantCreditSnapshot: &snapshot,
	}
	require.NoError(t, DB.Create(&subscription).Error)

	quote, err := RecalculateTimedSubscriptionConversionQuoteTx(DB, subscription.UserId, subscription.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.False(t, quote.CanConfirm)
	assert.Equal(t, ConversionQuoteCalculationArithmeticOverflow, quote.CalculationErrorCode)
	assert.Zero(t, quote.GrossCredit)
}

func TestTimedSubscriptionConversionQuoteQualificationAndSources(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	acceptedSources := []string{
		SubscriptionGrantOrder,
		SubscriptionGrantRedemption,
		SubscriptionGrantAdmin,
		SubscriptionGrantCompensation,
	}
	for index, source := range acceptedSources {
		planID := 22_000 + index
		subscriptionID := 32_000 + index
		seedConversionQuoteTimedPlan(t, planID, 100)
		snapshot := int64(100)
		subscription := seedConversionQuoteSubscription(t, subscriptionID, 12_000+index, planID, source, conversionQuoteTestNow+TimedSubscriptionConversionBlockSeconds, &snapshot)
		quote, err := RecalculateTimedSubscriptionConversionQuoteTx(DB, subscription.UserId, subscription.Id, conversionQuoteTestNow)
		require.NoError(t, err)
		assert.Truef(t, quote.Eligible, "source %s should be eligible: %v", source, quote.ReasonCodes)
		assert.True(t, quote.CanConfirm)
	}

	tests := []struct {
		name       string
		source     string
		status     string
		planUpdate map[string]any
		wantReason string
	}{
		{name: "trial source", source: "trial_code", wantReason: ConversionQuoteReasonTrialSource},
		{name: "invite trial source", source: "invite_trial", wantReason: ConversionQuoteReasonTrialSource},
		{name: "monthly invite source", source: SubscriptionGrantMonthlyInviteEntitlement, wantReason: ConversionQuoteReasonMonthlyInviteSource},
		{name: "unknown source", source: "system", wantReason: ConversionQuoteReasonSourceNotEligible},
		{name: "trial plan", source: SubscriptionGrantOrder, planUpdate: map[string]any{"is_trial": true}, wantReason: ConversionQuoteReasonTrialPlan},
		{name: "monthly invite plan", source: SubscriptionGrantOrder, planUpdate: map[string]any{"invite_trial": true}, wantReason: ConversionQuoteReasonMonthlyInvitePlan},
		{name: "wrong duration unit", source: SubscriptionGrantOrder, planUpdate: map[string]any{"duration_unit": SubscriptionDurationDay}, wantReason: ConversionQuoteReasonDurationNotOneMonth},
		{name: "wrong duration value", source: SubscriptionGrantOrder, planUpdate: map[string]any{"duration_value": 2}, wantReason: ConversionQuoteReasonDurationNotOneMonth},
		{name: "wrong reset", source: SubscriptionGrantOrder, planUpdate: map[string]any{"quota_reset_period": SubscriptionResetDaily}, wantReason: ConversionQuoteReasonResetNotMonthly},
		{name: "zero monthly credit", source: SubscriptionGrantOrder, planUpdate: map[string]any{"monthly_token_limit": int64(0)}, wantReason: ConversionQuoteReasonMonthlyCreditInvalid},
		{name: "plan switch disabled", source: SubscriptionGrantOrder, planUpdate: map[string]any{"timed_conversion_enabled": false}, wantReason: ConversionQuoteReasonPlanDisabled},
		{name: "cancelled status", source: SubscriptionGrantOrder, status: "cancelled", wantReason: ConversionQuoteReasonStatusNotEligible},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			planID := 23_000 + index
			subscriptionID := 33_000 + index
			seedConversionQuoteTimedPlan(t, planID, 100)
			if len(test.planUpdate) > 0 {
				require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", planID).UpdateColumns(test.planUpdate).Error)
			}
			snapshot := int64(100)
			subscription := seedConversionQuoteSubscription(t, subscriptionID, 13_000+index, planID, test.source, conversionQuoteTestNow+TimedSubscriptionConversionBlockSeconds, &snapshot)
			if test.status != "" {
				require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", subscription.Id).UpdateColumn("status", test.status).Error)
			}
			quote, err := RecalculateTimedSubscriptionConversionQuoteTx(DB, subscription.UserId, subscription.Id, conversionQuoteTestNow)
			require.NoError(t, err)
			assert.False(t, quote.CanConfirm)
			assert.Contains(t, quote.ReasonCodes, test.wantReason)
		})
	}

	seedConversionQuoteTimedPlan(t, 24_001, 100)
	snapshot := int64(100)
	subscription := seedConversionQuoteSubscription(t, 34_001, 14_001, 24_001, SubscriptionGrantOrder, conversionQuoteTestNow+TimedSubscriptionConversionBlockSeconds, &snapshot)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).UpdateColumn("credit_balance_conversion_enabled", false).Error)
	quote, err := RecalculateTimedSubscriptionConversionQuoteTx(DB, subscription.UserId, subscription.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	assert.False(t, quote.CanConfirm)
	assert.Contains(t, quote.ReasonCodes, ConversionQuoteReasonGlobalDisabled)
}

func TestTimedSubscriptionConversionQuoteCooldownAndGraceBoundaries(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	seedConversionQuoteTimedPlan(t, 25_001, 100)
	snapshot := int64(100)

	exactCooldown := seedConversionQuoteSubscription(t, 35_001, 15_001, 25_001, SubscriptionGrantOrder, conversionQuoteTestNow+60, &snapshot)
	exactCooldown.LastGrantedAt = conversionQuoteTestNow - TimedSubscriptionConversionCooldownSeconds
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", exactCooldown.Id).UpdateColumn("last_granted_at", exactCooldown.LastGrantedAt).Error)
	quote, err := RecalculateTimedSubscriptionConversionQuoteTx(DB, exactCooldown.UserId, exactCooldown.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	assert.Equal(t, ConversionQuoteCooldownReady, quote.CooldownStatus)
	assert.Zero(t, quote.CooldownRemainingSeconds)
	assert.True(t, quote.CanConfirm)

	beforeCooldown := seedConversionQuoteSubscription(t, 35_002, 15_002, 25_001, SubscriptionGrantOrder, conversionQuoteTestNow+60, &snapshot)
	beforeCooldown.LastGrantedAt = conversionQuoteTestNow - TimedSubscriptionConversionCooldownSeconds + 1
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", beforeCooldown.Id).UpdateColumn("last_granted_at", beforeCooldown.LastGrantedAt).Error)
	quote, err = RecalculateTimedSubscriptionConversionQuoteTx(DB, beforeCooldown.UserId, beforeCooldown.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	assert.Equal(t, ConversionQuoteCooldownActive, quote.CooldownStatus)
	assert.Equal(t, int64(1), quote.CooldownRemainingSeconds)
	assert.Contains(t, quote.ReasonCodes, ConversionQuoteReasonCooldownActive)
	assert.False(t, quote.CanConfirm)

	exactGrace := seedConversionQuoteSubscription(t, 35_003, 15_003, 25_001, SubscriptionGrantOrder, conversionQuoteTestNow-TimedSubscriptionConversionGraceSeconds, &snapshot)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", exactGrace.Id).Updates(map[string]any{"status": "expired", "last_granted_at": conversionQuoteTestNow - 40*24*60*60}).Error)
	quote, err = RecalculateTimedSubscriptionConversionQuoteTx(DB, exactGrace.UserId, exactGrace.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	assert.True(t, quote.WithinGrace)
	assert.Equal(t, ConversionQuoteGraceActive, quote.GraceStatus)
	assert.Zero(t, quote.GraceRemainingSeconds)
	assert.Zero(t, quote.Full31DayBlocks)
	assert.Equal(t, ConversionQuoteCategoryGrace, quote.Category)
	assert.True(t, quote.CanConfirm)

	outsideGrace := seedConversionQuoteSubscription(t, 35_004, 15_004, 25_001, SubscriptionGrantOrder, conversionQuoteTestNow-TimedSubscriptionConversionGraceSeconds-1, &snapshot)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", outsideGrace.Id).Updates(map[string]any{"status": "expired", "last_granted_at": conversionQuoteTestNow - 40*24*60*60}).Error)
	quote, err = RecalculateTimedSubscriptionConversionQuoteTx(DB, outsideGrace.UserId, outsideGrace.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	assert.False(t, quote.WithinGrace)
	assert.Equal(t, ConversionQuoteGraceExpired, quote.GraceStatus)
	assert.Contains(t, quote.ReasonCodes, ConversionQuoteReasonOutsideGrace)
	assert.False(t, quote.CanConfirm)
}

func TestTimedSubscriptionConversionQuoteSnapshotFallbackUsageAndDebt(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	seedConversionQuoteTimedPlan(t, 26_001, 200)
	snapshot := int64(120)

	withSnapshot := seedConversionQuoteSubscription(t, 36_001, 16_001, 26_001, SubscriptionGrantOrder, conversionQuoteTestNow+TimedSubscriptionConversionBlockSeconds, &snapshot)
	withSnapshot.TokenLimit = 80
	withSnapshot.TokenUsed = 30
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", withSnapshot.Id).Updates(map[string]any{"token_limit": 80, "token_used": 30}).Error)
	quote, err := RecalculateTimedSubscriptionConversionQuoteTx(DB, withSnapshot.UserId, withSnapshot.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	assert.Equal(t, int64(120), quote.CreditBasis)
	assert.Equal(t, ConversionCreditBasisGrantSnapshot, quote.CreditBasisSource)
	assert.Equal(t, int64(50), quote.CurrentRemainingCredit)
	assert.Equal(t, int64(170), quote.GrossCredit)

	fallback := seedConversionQuoteSubscription(t, 36_002, 16_002, 26_001, SubscriptionGrantOrder, conversionQuoteTestNow+TimedSubscriptionConversionBlockSeconds, nil)
	fallback.TokenLimit = 100
	fallback.TokenUsed = 100
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", fallback.Id).Updates(map[string]any{"token_limit": 100, "token_used": 100, "last_grant_credit_snapshot": nil}).Error)
	quote, err = RecalculateTimedSubscriptionConversionQuoteTx(DB, fallback.UserId, fallback.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	assert.Equal(t, int64(200), quote.CreditBasis)
	assert.Equal(t, ConversionCreditBasisCurrentPlan, quote.CreditBasisSource)
	assert.Zero(t, quote.CurrentRemainingCredit)
	assert.Equal(t, int64(200), quote.GrossCredit)

	overused := seedConversionQuoteSubscription(t, 36_003, 16_003, 26_001, SubscriptionGrantOrder, conversionQuoteTestNow, &snapshot)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", overused.Id).Updates(map[string]any{"token_limit": 100, "token_used": 101}).Error)
	quote, err = RecalculateTimedSubscriptionConversionQuoteTx(DB, overused.UserId, overused.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	assert.Zero(t, quote.CurrentRemainingCredit)
	assert.Zero(t, quote.GrossCredit)
	assert.False(t, quote.CanConfirm)
	assert.False(t, quote.Eligible)
	assert.Contains(t, quote.ReasonCodes, ConversionQuoteReasonGrossNotPositive)

	debtSource := seedConversionQuoteSubscription(t, 36_004, 16_004, 26_001, SubscriptionGrantOrder, conversionQuoteTestNow+TimedSubscriptionConversionBlockSeconds, &snapshot)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", debtSource.Id).Updates(map[string]any{"token_limit": 80, "token_used": 30}).Error)
	creditBalance := UserSubscription{UserId: debtSource.UserId, PlanId: 9_999, EntitlementType: SubscriptionEntitlementCreditBalance, TokenLimit: 50, TokenUsed: 125, Status: "active"}
	require.NoError(t, DB.Create(&creditBalance).Error)
	quote, err = RecalculateTimedSubscriptionConversionQuoteTx(DB, debtSource.UserId, debtSource.Id, conversionQuoteTestNow)
	require.NoError(t, err)
	assert.Equal(t, int64(170), quote.GrossCredit)
	assert.Equal(t, int64(75), quote.CurrentDebt)
	assert.Equal(t, int64(75), quote.EstimatedDebtOffset)
	assert.Equal(t, int64(95), quote.NetAvailableCredit)
}

func TestCreateUserSubscriptionRefreshesStableGrantMetadata(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	plan := seedConversionQuoteTimedPlan(t, 27_001, 100)
	var created *UserSubscription
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		created, err = CreateUserSubscriptionFromPlanTx(tx, 17_001, plan, SubscriptionGrantOrder)
		return err
	}))
	require.NotNil(t, created)
	require.NotNil(t, created.LastGrantCreditSnapshot)
	assert.Equal(t, int64(100), *created.LastGrantCreditSnapshot)
	assert.Equal(t, SubscriptionGrantOrder, created.LastGrantSource)
	assert.Equal(t, SubscriptionGrantTimeSourceLive, created.LastGrantTimeSource)
	assert.Positive(t, created.LastGrantedAt)

	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", created.Id).UpdateColumn("last_granted_at", int64(1)).Error)
	plan.MonthlyTokenLimit = 250
	var extended *UserSubscription
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		extended, err = CreateUserSubscriptionFromPlanTx(tx, 17_001, plan, SubscriptionGrantCompensation)
		return err
	}))
	require.Equal(t, created.Id, extended.Id)
	require.NotNil(t, extended.LastGrantCreditSnapshot)
	assert.Equal(t, int64(250), *extended.LastGrantCreditSnapshot)
	assert.Equal(t, SubscriptionGrantCompensation, extended.LastGrantSource)
	assert.Equal(t, SubscriptionGrantTimeSourceLive, extended.LastGrantTimeSource)
	assert.Greater(t, extended.LastGrantedAt, int64(1))
}

func TestBackfillTimedSubscriptionGrantMetadataUsesReliableHistoryAndConservativeFallback(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	plan := seedConversionQuoteTimedPlan(t, 28_001, 300)

	orderSub := seedLegacyConversionQuoteSubscription(t, 38_001, 18_001, plan.Id, SubscriptionGrantOrder)
	orderSnapshot, err := MarshalSubscriptionEntitlementSnapshot(NewSubscriptionEntitlementSnapshotFromPlan(&SubscriptionPlan{Id: plan.Id, MonthlyTokenLimit: 333}))
	require.NoError(t, err)
	require.NoError(t, DB.Create(&SubscriptionOrder{
		Id:                  48_001,
		UserId:              orderSub.UserId,
		PlanId:              plan.Id,
		Status:              common.TopUpStatusSuccess,
		CompleteTime:        conversionQuoteTestNow - 100,
		EntitlementSnapshot: orderSnapshot,
	}).Error)

	redemptionSub := seedLegacyConversionQuoteSubscription(t, 38_002, 18_002, plan.Id, SubscriptionGrantRedemption)
	require.NoError(t, DB.Create(&Redemption{
		Id:           48_002,
		UsedUserId:   redemptionSub.UserId,
		PlanId:       plan.Id,
		Type:         RedemptionTypeSubscription,
		Status:       common.RedemptionCodeStatusUsed,
		RedeemedTime: conversionQuoteTestNow - 200,
		Key:          "conversion-history-redemption",
	}).Error)

	unknownSub := seedLegacyConversionQuoteSubscription(t, 38_003, 18_003, plan.Id, SubscriptionGrantAdmin)
	require.NoError(t, BackfillTimedSubscriptionGrantMetadataTx(DB, conversionQuoteTestNow))

	var gotOrder UserSubscription
	require.NoError(t, DB.First(&gotOrder, orderSub.Id).Error)
	assert.Equal(t, conversionQuoteTestNow-100, gotOrder.LastGrantedAt)
	assert.Equal(t, SubscriptionGrantTimeSourceOrder, gotOrder.LastGrantTimeSource)
	assert.Equal(t, SubscriptionGrantOrder, gotOrder.LastGrantSource)
	require.NotNil(t, gotOrder.LastGrantCreditSnapshot)
	assert.Equal(t, int64(333), *gotOrder.LastGrantCreditSnapshot)

	var gotRedemption UserSubscription
	require.NoError(t, DB.First(&gotRedemption, redemptionSub.Id).Error)
	assert.Equal(t, conversionQuoteTestNow-200, gotRedemption.LastGrantedAt)
	assert.Equal(t, SubscriptionGrantTimeSourceRedemption, gotRedemption.LastGrantTimeSource)
	assert.Equal(t, SubscriptionGrantRedemption, gotRedemption.LastGrantSource)
	assert.Nil(t, gotRedemption.LastGrantCreditSnapshot)

	var gotUnknown UserSubscription
	require.NoError(t, DB.First(&gotUnknown, unknownSub.Id).Error)
	assert.Equal(t, conversionQuoteTestNow, gotUnknown.LastGrantedAt)
	assert.Equal(t, SubscriptionGrantTimeSourceConservative, gotUnknown.LastGrantTimeSource)
	assert.Equal(t, SubscriptionGrantAdmin, gotUnknown.LastGrantSource)
	assert.Nil(t, gotUnknown.LastGrantCreditSnapshot)

	require.NoError(t, BackfillTimedSubscriptionGrantMetadataTx(DB, conversionQuoteTestNow+1000))
	require.NoError(t, DB.First(&gotOrder, orderSub.Id).Error)
	require.NoError(t, DB.First(&gotUnknown, unknownSub.Id).Error)
	assert.Equal(t, conversionQuoteTestNow-100, gotOrder.LastGrantedAt)
	assert.Equal(t, conversionQuoteTestNow, gotUnknown.LastGrantedAt)
}

func TestBackfillTimedSubscriptionGrantMetadataRejectsAmbiguousHistory(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	plan := seedConversionQuoteTimedPlan(t, 28_101, 300)
	first := seedLegacyConversionQuoteSubscription(t, 38_101, 18_101, plan.Id, SubscriptionGrantOrder)
	second := seedLegacyConversionQuoteSubscription(t, 38_102, 18_101, plan.Id, SubscriptionGrantOrder)
	second.StartTime = first.StartTime
	second.EndTime = first.EndTime
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", second.Id).Updates(map[string]any{"start_time": second.StartTime, "end_time": second.EndTime}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{
		Id:           48_101,
		UserId:       first.UserId,
		PlanId:       plan.Id,
		Status:       common.TopUpStatusSuccess,
		CompleteTime: conversionQuoteTestNow - 100,
	}).Error)

	require.NoError(t, BackfillTimedSubscriptionGrantMetadataTx(DB, conversionQuoteTestNow))
	for _, id := range []int{first.Id, second.Id} {
		var subscription UserSubscription
		require.NoError(t, DB.First(&subscription, id).Error)
		assert.Equal(t, conversionQuoteTestNow, subscription.LastGrantedAt)
		assert.Equal(t, SubscriptionGrantTimeSourceConservative, subscription.LastGrantTimeSource)
	}
}

func TestTimedSubscriptionGrantMetadataMigrationSupportsFreshAndLegacySQLite(t *testing.T) {
	t.Run("fresh schema", func(t *testing.T) {
		setupSubscriptionConversionQuoteTestDB(t)
		for _, column := range []string{
			"last_granted_at",
			"last_grant_credit_snapshot",
			"last_grant_time_source",
			"last_grant_source",
		} {
			assert.True(t, DB.Migrator().HasColumn(&UserSubscription{}, column), column)
		}
	})

	t.Run("legacy schema conservative cooldown", func(t *testing.T) {
		oldDB := DB
		oldLogDB := LOG_DB
		oldUsingSQLite := common.UsingSQLite
		oldUsingMySQL := common.UsingMySQL
		oldUsingPostgreSQL := common.UsingPostgreSQL
		common.UsingSQLite = true
		common.UsingMySQL = false
		common.UsingPostgreSQL = false

		db, err := gorm.Open(sqlite.Open("file:conversion_quote_legacy_migration?mode=memory&cache=shared"), &gorm.Config{})
		require.NoError(t, err)
		DB = db
		LOG_DB = db
		t.Cleanup(func() {
			sqlDB, closeErr := db.DB()
			if closeErr == nil {
				_ = sqlDB.Close()
			}
			DB = oldDB
			LOG_DB = oldLogDB
			common.UsingSQLite = oldUsingSQLite
			common.UsingMySQL = oldUsingMySQL
			common.UsingPostgreSQL = oldUsingPostgreSQL
		})

		require.NoError(t, db.Exec(`CREATE TABLE user_subscriptions (
			id integer PRIMARY KEY,
			user_id integer NOT NULL,
			plan_id integer NOT NULL,
			entitlement_type varchar(32) NOT NULL DEFAULT 'timed',
			token_limit bigint NOT NULL DEFAULT 0,
			token_used bigint NOT NULL DEFAULT 0,
			grant_reason varchar(32) DEFAULT '',
			start_time bigint,
			end_time bigint,
			status varchar(32),
			source varchar(32) DEFAULT 'order',
			updated_at bigint
		)`).Error)
		require.NoError(t, db.Exec(
			"INSERT INTO user_subscriptions (id, user_id, plan_id, entitlement_type, token_limit, token_used, grant_reason, start_time, end_time, status, source, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			39_001, 19_001, 29_001, SubscriptionEntitlementTimed, 100, 25, SubscriptionGrantAdmin,
			conversionQuoteTestNow-1000, conversionQuoteTestNow+1000, "active", SubscriptionGrantAdmin, int64(123),
		).Error)

		for _, statement := range []string{
			"ALTER TABLE user_subscriptions ADD COLUMN last_granted_at bigint NOT NULL DEFAULT 0",
			"ALTER TABLE user_subscriptions ADD COLUMN last_grant_credit_snapshot bigint",
			"ALTER TABLE user_subscriptions ADD COLUMN last_grant_time_source varchar(64) NOT NULL DEFAULT ''",
			"ALTER TABLE user_subscriptions ADD COLUMN last_grant_source varchar(32) NOT NULL DEFAULT ''",
		} {
			require.NoError(t, db.Exec(statement).Error)
		}
		require.NoError(t, db.AutoMigrate(&SubscriptionOrder{}, &Redemption{}, &InvitationRewardEvent{}))
		for _, column := range []string{
			"last_granted_at",
			"last_grant_credit_snapshot",
			"last_grant_time_source",
			"last_grant_source",
		} {
			assert.True(t, db.Migrator().HasColumn(&UserSubscription{}, column), column)
		}
		require.NoError(t, BackfillTimedSubscriptionGrantMetadataTx(db, conversionQuoteTestNow))

		var migrated UserSubscription
		require.NoError(t, db.First(&migrated, 39_001).Error)
		assert.Equal(t, conversionQuoteTestNow, migrated.LastGrantedAt)
		assert.Equal(t, SubscriptionGrantTimeSourceConservative, migrated.LastGrantTimeSource)
		assert.Equal(t, SubscriptionGrantAdmin, migrated.LastGrantSource)
		assert.Nil(t, migrated.LastGrantCreditSnapshot)
		assert.Equal(t, int64(123), migrated.UpdatedAt, "updated_at must never seed conversion cooldown")
	})
}

func seedLegacyConversionQuoteSubscription(t *testing.T, subscriptionID int, userID int, planID int, source string) *UserSubscription {
	t.Helper()
	subscription := &UserSubscription{
		Id:              subscriptionID,
		UserId:          userID,
		PlanId:          planID,
		EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit:      100,
		TokenUsed:       25,
		GrantReason:     source,
		Source:          source,
		StartTime:       conversionQuoteTestNow - 1000,
		EndTime:         conversionQuoteTestNow + TimedSubscriptionConversionBlockSeconds,
		Status:          "active",
	}
	require.NoError(t, DB.Create(subscription).Error)
	return subscription
}

func seedConversionQuoteSubscription(t *testing.T, subscriptionID int, userID int, planID int, source string, endTime int64, snapshot *int64) *UserSubscription {
	t.Helper()
	subscription := &UserSubscription{
		Id:                      subscriptionID,
		UserId:                  userID,
		PlanId:                  planID,
		EntitlementType:         SubscriptionEntitlementTimed,
		TokenLimit:              100,
		TokenUsed:               75,
		GrantReason:             source,
		Source:                  source,
		StartTime:               conversionQuoteTestNow - 40*24*60*60,
		EndTime:                 endTime,
		Status:                  "active",
		LastGrantedAt:           conversionQuoteTestNow - 40*24*60*60,
		LastGrantCreditSnapshot: snapshot,
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         source,
	}
	require.NoError(t, DB.Create(subscription).Error)
	return subscription
}

func setupSubscriptionConversionQuoteTestDB(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	resetDBTimestampCacheForTest()
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	dsn := fmt.Sprintf("file:%s_conversion_quote?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &Redemption{}, &InvitationRewardEvent{}, &CreditBalanceLedger{}, &SubscriptionConversion{}))
	seedConversionQuoteCreditBalancePlan(t)

	t.Cleanup(func() {
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		resetDBTimestampCacheForTest()
		ClearPrimaryBillableSubscriptionCacheForTest()
		ClearSubscriptionPlanCacheForTest()
	})
}

func seedConversionQuoteCreditBalancePlan(t *testing.T) {
	t.Helper()
	code := fmt.Sprintf("credit-balance-%s", strings.NewReplacer("/", "-", " ", "-", ":", "-").Replace(t.Name()))
	plan := SubscriptionPlan{
		Id:                             9_999,
		Title:                          "Credit balance",
		EntitlementType:                SubscriptionEntitlementCreditBalance,
		Enabled:                        true,
		BusinessCode:                   &code,
		CreditBalanceConfigured:        true,
		CreditBalanceConversionEnabled: true,
	}
	require.NoError(t, DB.Create(&plan).Error)
}

func seedConversionQuoteTimedPlan(t *testing.T, planID int, monthlyCredit int64) *SubscriptionPlan {
	t.Helper()
	code := fmt.Sprintf("conversion-%d", planID)
	plan := &SubscriptionPlan{
		Id:                     planID,
		Title:                  fmt.Sprintf("Plan %d", planID),
		EntitlementType:        SubscriptionEntitlementTimed,
		Enabled:                true,
		BusinessCode:           &code,
		DurationUnit:           SubscriptionDurationMonth,
		DurationValue:          1,
		QuotaResetPeriod:       SubscriptionResetMonthly,
		MonthlyTokenLimit:      monthlyCredit,
		TimedConversionEnabled: true,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}
