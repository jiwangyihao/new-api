package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestHasActiveUserSubscriptionReadOnlyFallsBackWithoutRepairingSetting(t *testing.T) {
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()

	const userID = 8181
	const planID = 8182
	const subscriptionID = 8183
	const staleSubscriptionID = 8199
	user := User{Id: userID, Username: "read_only_stale_active", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = staleSubscriptionID
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	code := "read-only-stale-active"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: planID, Title: "Read-only fallback", Enabled: true, MonthlyTokenLimit: 100, BusinessCode: &code}).Error)
	now := GetDBTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: subscriptionID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive, StartTime: now - 60, EndTime: now + 3600, TokenLimit: 100, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	hasSubscription, err := HasActiveUserSubscription(userID)

	require.NoError(t, err)
	assert.True(t, hasSubscription)
	var persisted User
	require.NoError(t, DB.First(&persisted, userID).Error)
	assert.Equal(t, staleSubscriptionID, persisted.GetSetting().ActiveSubscriptionId, "read-only precheck must not repair active_subscription_id")
}

func TestHasActiveUserSubscriptionReadOnlyProjectsDueResetWithoutPersisting(t *testing.T) {
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()

	const userID = 8191
	const planID = 8192
	const subscriptionID = 8193
	user := User{Id: userID, Username: "read_only_due_reset", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = subscriptionID
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	code := "read-only-due-reset"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: planID, Title: "Read-only due reset", Enabled: true, MonthlyTokenLimit: 100, BusinessCode: &code, QuotaResetPeriod: SubscriptionResetDaily}).Error)
	now := GetDBTimestamp()
	originalLastReset := now - 2*86400
	originalNextReset := now - 3600
	require.NoError(t, DB.Create(&UserSubscription{Id: subscriptionID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive, StartTime: now - 2*86400, EndTime: now + 3*86400, TokenLimit: 100, TokenUsed: 100, LastResetTime: originalLastReset, NextResetTime: originalNextReset, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	hasSubscription, err := HasActiveUserSubscription(userID)

	require.NoError(t, err)
	assert.True(t, hasSubscription, "a due reset makes the subscription usable for the subsequent locked pre-consume")
	var persisted UserSubscription
	require.NoError(t, DB.First(&persisted, subscriptionID).Error)
	assert.Equal(t, int64(100), persisted.TokenUsed, "read-only precheck must not reset quota")
	assert.Equal(t, originalLastReset, persisted.LastResetTime)
	assert.Equal(t, originalNextReset, persisted.NextResetTime)
}

func TestHasActiveUserSubscriptionReadOnlySingleActiveDoesNotFallbackWhenSelectedExhausted(t *testing.T) {
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()

	const userID = 8196
	const selectedPlanID = 8197
	const selectedSubscriptionID = 8198
	const fallbackPlanID = 8199
	const fallbackSubscriptionID = 8200
	user := User{Id: userID, Username: "read_only_single_active_exhausted", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = selectedSubscriptionID
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	selectedCode := "read-only-single-active-selected"
	fallbackCode := "read-only-single-active-fallback"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: selectedPlanID, Title: "Selected exhausted", Enabled: true, MonthlyTokenLimit: 100, BusinessCode: &selectedCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: fallbackPlanID, Title: "Fallback available", Enabled: true, MonthlyTokenLimit: 100, BusinessCode: &fallbackCode}).Error)
	now := GetDBTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: selectedSubscriptionID, UserId: userID, PlanId: selectedPlanID, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive, StartTime: now - 60, EndTime: now + 3600, TokenLimit: 100, TokenUsed: 100, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: fallbackSubscriptionID, UserId: userID, PlanId: fallbackPlanID, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive, StartTime: now - 60, EndTime: now + 7200, TokenLimit: 100, TokenUsed: 0, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	hasSubscription, err := HasActiveUserSubscription(userID)

	require.NoError(t, err)
	assert.False(t, hasSubscription, "single_active must not fall back while its selected subscription remains active but exhausted")
	var persisted User
	require.NoError(t, DB.First(&persisted, userID).Error)
	assert.Equal(t, selectedSubscriptionID, persisted.GetSetting().ActiveSubscriptionId)
}

func TestHasActiveUserSubscriptionReadOnlyDoesNotLockOrUpdateUsers(t *testing.T) {
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()

	const userID = 8201
	const planID = 8202
	const subscriptionID = 8203
	const staleSubscriptionID = 8299
	user := User{Id: userID, Username: "read_only_no_user_lock", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = staleSubscriptionID
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	code := "read-only-no-user-lock"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: planID, Title: "Read-only no lock", Enabled: true, MonthlyTokenLimit: 100, BusinessCode: &code}).Error)
	now := GetDBTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: subscriptionID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive, StartTime: now - 60, EndTime: now + 3600, TokenLimit: 100, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	const queryCallback = "test:has_active_subscription_no_for_update"
	const updateCallback = "test:has_active_subscription_no_user_update"
	var sawForUpdate bool
	var sawUserUpdate bool
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
		if tx.Statement != nil {
			if _, ok := tx.Statement.Clauses["FOR"]; ok {
				sawForUpdate = true
			}
		}
	}))
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(updateCallback, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "User" {
			sawUserUpdate = true
		}
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Query().Remove(queryCallback)
		_ = DB.Callback().Update().Remove(updateCallback)
	})

	hasSubscription, err := HasActiveUserSubscription(userID)

	require.NoError(t, err)
	assert.True(t, hasSubscription)
	assert.False(t, sawForUpdate, "read-only precheck must not add a FOR UPDATE clause")
	assert.False(t, sawUserUpdate, "read-only precheck must not update users")
}
