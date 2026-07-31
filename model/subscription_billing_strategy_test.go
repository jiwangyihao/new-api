package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSingleActiveBillingStrategyDoesNotFallBackOnModelRestriction(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()

	user := User{Id: 7891, Username: "single-active-model-restriction", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = 7894
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)

	activeCode := "single-active-claude"
	fallbackCode := "single-active-gpt"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7892, Title: "Claude only", Enabled: true, ModelLimits: "claude-3-7-sonnet", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7893, Title: "GPT fallback", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &fallbackCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7894, UserId: user.Id, PlanId: 7892, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7895, UserId: user.Id, PlanId: 7893, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	_, err := PreConsumeUserSubscription("single-active-model-restriction", user.Id, "gpt-4o", 0, 5)

	require.ErrorContains(t, err, "subscription model not allowed")
	var fallback UserSubscription
	require.NoError(t, DB.First(&fallback, 7895).Error)
	assert.Zero(t, fallback.TokenUsed)
	var records int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", "single-active-model-restriction").Count(&records).Error)
	assert.Zero(t, records)
}

func TestActiveFallbackBillingStrategyFallsBackOnInsufficientCredit(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()

	user := User{Id: 7901, Username: "active-fallback-insufficient", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyActiveFallback
	setting.ActiveSubscriptionId = 7904
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)

	activeCode := "active-fallback-low"
	fallbackCode := "active-fallback-next"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7902, Title: "Low active", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7903, Title: "Next timed", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &fallbackCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7904, UserId: user.Id, PlanId: 7902, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, TokenUsed: 98, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7905, UserId: user.Id, PlanId: 7903, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	pre, err := PreConsumeUserSubscription("active-fallback-insufficient", user.Id, "gpt-4o", 0, 5)

	require.NoError(t, err)
	assert.Equal(t, 7905, pre.UserSubscriptionId)
	var active UserSubscription
	require.NoError(t, DB.First(&active, 7904).Error)
	assert.Equal(t, int64(98), active.TokenUsed)
	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	assert.Equal(t, 7904, persisted.GetSetting().ActiveSubscriptionId)
}

func TestTimedFirstBillingStrategyIgnoresActiveAndUsesEarliestTimed(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()

	user := User{Id: 7911, Username: "timed-first", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyTimedFirst
	setting.ActiveSubscriptionId = 7915
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)

	earlyCode := "timed-first-early"
	lateCode := "timed-first-late"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7912, Title: "Early timed", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &earlyCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7913, Title: "Late timed", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &lateCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7914, UserId: user.Id, PlanId: 7912, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: "trial_code", Source: "trial_code"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7915, UserId: user.Id, PlanId: 7913, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	pre, err := PreConsumeUserSubscription("timed-first", user.Id, "gpt-4o", 0, 5)

	require.NoError(t, err)
	assert.Equal(t, 7914, pre.UserSubscriptionId)
}

func TestActiveFallbackBillingStrategyUsesTimedBeforeCreditBalance(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()

	user := User{Id: 7921, Username: "active-fallback-order", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyActiveFallback
	setting.ActiveSubscriptionId = 7925
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)

	activeCode := "active-fallback-order-active"
	timedCode := "active-fallback-order-timed"
	creditCode := "active-fallback-order-credit"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7922, Title: "Active low", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7923, Title: "Timed next", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &timedCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7924, Title: "Credit balance", EntitlementType: SubscriptionEntitlementCreditBalance, Enabled: true, ModelLimits: "gpt-4o", ConcurrencyLimit: 1, BusinessCode: &creditCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7925, UserId: user.Id, PlanId: 7922, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, TokenUsed: 99, EndTime: now + 1800, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7926, UserId: user.Id, PlanId: 7923, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: "redemption", Source: "redemption"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7927, UserId: user.Id, PlanId: 7924, EntitlementType: SubscriptionEntitlementCreditBalance, Status: "active", TokenLimit: 100, EndTime: 0, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	pre, err := PreConsumeUserSubscription("active-fallback-order", user.Id, "gpt-4o", 0, 5)

	require.NoError(t, err)
	assert.Equal(t, 7926, pre.UserSubscriptionId)
	var credit UserSubscription
	require.NoError(t, DB.First(&credit, 7927).Error)
	assert.Zero(t, credit.TokenUsed)
}

func TestActiveFallbackBillingStrategyDoesNotFallBackOnModelRestriction(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()

	user := User{Id: 7931, Username: "active-fallback-model", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyActiveFallback
	setting.ActiveSubscriptionId = 7934
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)

	activeCode := "active-fallback-claude"
	fallbackCode := "active-fallback-gpt"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7932, Title: "Claude active", Enabled: true, ModelLimits: "claude-3-7-sonnet", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7933, Title: "GPT next", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &fallbackCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7934, UserId: user.Id, PlanId: 7932, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7935, UserId: user.Id, PlanId: 7933, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	_, err := PreConsumeUserSubscription("active-fallback-model", user.Id, "gpt-4o", 0, 5)

	require.ErrorContains(t, err, "subscription model not allowed")
	var fallback UserSubscription
	require.NoError(t, DB.First(&fallback, 7935).Error)
	assert.Zero(t, fallback.TokenUsed)
}

func TestSingleActiveBillingStrategyKeepsExistingEntitlementWhenPlanDisabled(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()

	user := User{Id: 7941, Username: "single-active-disabled-cache", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = 7944
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	activeCode := "single-active-disabled"
	fallbackCode := "single-active-repair"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7942, Title: "Active then disabled", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7943, Title: "Fallback target", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &fallbackCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7944, UserId: user.Id, PlanId: 7942, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7945, UserId: user.Id, PlanId: 7943, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	first, err := PreConsumeUserSubscription("single-active-disabled-first", user.Id, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 7944, first.UserSubscriptionId)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", 7942).Update("enabled", false).Error)
	InvalidateSubscriptionPlanCache(7942)

	second, err := PreConsumeUserSubscription("single-active-disabled-second", user.Id, "gpt-4o", 0, 5)

	require.NoError(t, err)
	assert.Equal(t, 7944, second.UserSubscriptionId)
	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	assert.Equal(t, 7944, persisted.GetSetting().ActiveSubscriptionId)
	var active, fallback UserSubscription
	require.NoError(t, DB.First(&active, 7944).Error)
	require.NoError(t, DB.First(&fallback, 7945).Error)
	assert.Equal(t, int64(10), active.TokenUsed)
	assert.Zero(t, fallback.TokenUsed)
}

func TestSingleActiveBillingStrategyRepairsCancelledCachedActiveSubscription(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()

	user := User{Id: 7951, Username: "single-active-cancelled-cache", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = 7954
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	activeCode := "single-active-cancelled"
	fallbackCode := "single-active-after-cancel"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7952, Title: "Cached then cancelled", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7953, Title: "After cancellation", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &fallbackCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7954, UserId: user.Id, PlanId: 7952, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7955, UserId: user.Id, PlanId: 7953, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	first, err := PreConsumeUserSubscription("single-active-cancelled-first", user.Id, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 7954, first.UserSubscriptionId)
	_, cached := primaryBillableSubscriptionCache.Load(primaryBillableSubscriptionCacheKey(user.Id))
	require.True(t, cached)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", 7954).Updates(map[string]any{"status": "cancelled", "end_time": now}).Error)

	second, err := PreConsumeUserSubscription("single-active-cancelled-second", user.Id, "gpt-4o", 0, 5)

	require.NoError(t, err)
	assert.Equal(t, 7955, second.UserSubscriptionId)
	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	assert.Equal(t, 7955, persisted.GetSetting().ActiveSubscriptionId)
}

func TestSingleActiveBillingStrategyRejectsConvertedCachedSubscription(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	t.Cleanup(ClearPrimaryBillableSubscriptionCacheForTest)

	user := User{Id: 7956, Username: "single-active-converted-cache", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = 7959
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	activeCode := "single-active-converted"
	fallbackCode := "single-active-after-conversion"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7957, Title: "Cached then converted", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7958, Title: "After conversion", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &fallbackCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7959, UserId: user.Id, PlanId: 7957, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive, TokenLimit: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7960, UserId: user.Id, PlanId: 7958, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive, TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	first, err := PreConsumeUserSubscription("single-active-converted-first", user.Id, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 7959, first.UserSubscriptionId)
	_, cached := primaryBillableSubscriptionCache.Load(primaryBillableSubscriptionCacheKey(user.Id))
	require.True(t, cached)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", 7959).Updates(map[string]any{"status": SubscriptionStatusConverted, "converted_at": now, "conversion_id": 1, "converted_to_subscription_id": 7960}).Error)

	second, err := PreConsumeUserSubscription("single-active-converted-second", user.Id, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 7960, second.UserSubscriptionId)
	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	assert.Equal(t, 7960, persisted.GetSetting().ActiveSubscriptionId)
	var source UserSubscription
	require.NoError(t, DB.First(&source, 7959).Error)
	assert.Equal(t, int64(5), source.TokenUsed)
}

func TestExpiredActiveSubscriptionRepairCommitsWhenReplacementRejectsModel(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()

	user := User{Id: 7961, Username: "repair-before-model-error", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = 7964
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	expiredCode := "repair-expired-active"
	replacementCode := "repair-model-denied"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7962, Title: "Expired active", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &expiredCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7963, Title: "Claude replacement", Enabled: true, ModelLimits: "claude-3-7-sonnet", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &replacementCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7964, UserId: user.Id, PlanId: 7962, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now - 1, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7965, UserId: user.Id, PlanId: 7963, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	_, err := PreConsumeUserSubscription("repair-before-model-error", user.Id, "gpt-4o", 0, 5)

	require.ErrorContains(t, err, "subscription model not allowed")
	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	assert.Equal(t, 7965, persisted.GetSetting().ActiveSubscriptionId)
	var records int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", "repair-before-model-error").Count(&records).Error)
	assert.Zero(t, records)
}

func TestActiveDistributorUsageUsesTimedFirstStrategySelection(t *testing.T) {
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	user := User{Id: 7971, Username: "usage-timed-first", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyTimedFirst
	setting.ActiveSubscriptionId = 7975
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	earlyCode := "usage-timed-first-early"
	activeCode := "usage-timed-first-active"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7972, Title: "Usage early", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &earlyCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7973, Title: "Usage active", Enabled: true, MonthlyTokenLimit: 200, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7974, UserId: user.Id, PlanId: 7972, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, TokenUsed: 12, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7975, UserId: user.Id, PlanId: 7973, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 200, TokenUsed: 34, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	usage, err := GetActiveDistributorSubscriptionUsage(user.Id)

	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, int64(100), usage.TokenLimit)
	assert.Equal(t, int64(12), usage.TokenUsed)
}

func TestActiveDistributorUsageUsesActiveFallbackSelection(t *testing.T) {
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	user := User{Id: 7981, Username: "usage-active-fallback", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyActiveFallback
	setting.ActiveSubscriptionId = 7985
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	earlyCode := "usage-active-fallback-early"
	activeCode := "usage-active-fallback-active"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7982, Title: "Usage early fallback", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &earlyCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7983, Title: "Usage active fallback", Enabled: true, MonthlyTokenLimit: 200, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7984, UserId: user.Id, PlanId: 7982, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, TokenUsed: 12, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7985, UserId: user.Id, PlanId: 7983, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 200, TokenUsed: 34, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	usage, err := GetActiveDistributorSubscriptionUsage(user.Id)

	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, int64(200), usage.TokenLimit)
	assert.Equal(t, int64(34), usage.TokenUsed)
}

func TestActiveDistributorUsageKeepsExhaustedActiveFallbackSubscription(t *testing.T) {
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	user := User{Id: 7991, Username: "usage-exhausted-active", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyActiveFallback
	setting.ActiveSubscriptionId = 7994
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	activeCode := "usage-exhausted-active"
	fallbackCode := "usage-exhausted-fallback"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7992, Title: "Exhausted active", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7993, Title: "Available fallback", Enabled: true, MonthlyTokenLimit: 200, ConcurrencyLimit: 1, BusinessCode: &fallbackCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7994, UserId: user.Id, PlanId: 7992, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, TokenUsed: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7995, UserId: user.Id, PlanId: 7993, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 200, TokenUsed: 20, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	usage, err := GetActiveDistributorSubscriptionUsage(user.Id)

	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, int64(100), usage.TokenLimit)
	assert.Equal(t, int64(100), usage.TokenUsed)
}

func TestBillingStrategySwitchOnlyAffectsNewPreconsumes(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	user := User{Id: 8001, Username: "strategy-switch-in-flight", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = 8005
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	earlyCode := "strategy-switch-early"
	activeCode := "strategy-switch-active"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 8002, Title: "Early timed", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &earlyCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 8003, Title: "Active timed", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 8004, UserId: user.Id, PlanId: 8002, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 8005, UserId: user.Id, PlanId: 8003, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	first, err := PreConsumeUserSubscription("strategy-switch-first", user.Id, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 8005, first.UserSubscriptionId)
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyTimedFirst
	_, err = SaveUserSetting(user.Id, setting)
	require.NoError(t, err)
	second, err := PreConsumeUserSubscription("strategy-switch-second", user.Id, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 8004, second.UserSubscriptionId)

	require.NoError(t, PostConsumeUserSubscriptionTokenDelta(first.UserSubscriptionId, 3))
	var active UserSubscription
	require.NoError(t, DB.First(&active, 8005).Error)
	assert.Equal(t, int64(8), active.TokenUsed)
	var early UserSubscription
	require.NoError(t, DB.First(&early, 8004).Error)
	assert.Equal(t, int64(5), early.TokenUsed)
}

func TestActiveFallbackUsesCreditBalanceOnlyAfterTimedCandidates(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	user := User{Id: 8011, Username: "active-fallback-credit-last", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyActiveFallback
	setting.ActiveSubscriptionId = 8015
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	activeCode := "active-fallback-credit-last-active"
	timedCode := "active-fallback-credit-last-timed"
	creditCode := "active-fallback-credit-last-balance"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 8012, Title: "Exhausted active", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 8013, Title: "Insufficient timed", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &timedCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 8014, Title: "Credit balance", EntitlementType: SubscriptionEntitlementCreditBalance, Enabled: true, ConcurrencyLimit: 1, BusinessCode: &creditCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 8015, UserId: user.Id, PlanId: 8012, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, TokenUsed: 100, EndTime: now + 1800, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 8016, UserId: user.Id, PlanId: 8013, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, TokenUsed: 98, EndTime: now + 3600, GrantReason: "monthly_invite_entitlement", Source: "monthly_invite_entitlement"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 8017, UserId: user.Id, PlanId: 8014, EntitlementType: SubscriptionEntitlementCreditBalance, Status: "active", TokenLimit: 100, TokenUsed: 10, EndTime: 0, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	pre, err := PreConsumeUserSubscription("active-fallback-credit-last", user.Id, "gpt-4o", 0, 5)

	require.NoError(t, err)
	assert.Equal(t, 8017, pre.UserSubscriptionId)
	var records []SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "active-fallback-credit-last").Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, 8017, records[0].UserSubscriptionId)
	var active UserSubscription
	require.NoError(t, DB.First(&active, 8015).Error)
	assert.Equal(t, int64(100), active.TokenUsed)
	var timed UserSubscription
	require.NoError(t, DB.First(&timed, 8016).Error)
	assert.Equal(t, int64(98), timed.TokenUsed)
	var credit UserSubscription
	require.NoError(t, DB.First(&credit, 8017).Error)
	assert.Equal(t, int64(15), credit.TokenUsed)
}

func TestRequestFailureRefundDoesNotSelectAnotherSubscription(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	user := User{Id: 8021, Username: "strategy-request-failure", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyActiveFallback
	setting.ActiveSubscriptionId = 8024
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	activeCode := "strategy-failure-active"
	fallbackCode := "strategy-failure-fallback"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 8022, Title: "Failure active", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &activeCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 8023, Title: "Failure fallback", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &fallbackCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 8024, UserId: user.Id, PlanId: 8022, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 8025, UserId: user.Id, PlanId: 8023, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	pre, err := PreConsumeUserSubscription("strategy-request-failure", user.Id, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 8024, pre.UserSubscriptionId)
	require.NoError(t, RefundSubscriptionPreConsume("strategy-request-failure"))

	var active UserSubscription
	require.NoError(t, DB.First(&active, 8024).Error)
	assert.Zero(t, active.TokenUsed)
	var fallback UserSubscription
	require.NoError(t, DB.First(&fallback, 8025).Error)
	assert.Zero(t, fallback.TokenUsed)
	var records []SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "strategy-request-failure").Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, 8024, records[0].UserSubscriptionId)
	assert.Equal(t, "refunded", records[0].Status)
}

func TestPreConsumeExcludesFutureSubscriptionCandidates(t *testing.T) {
	tests := []struct {
		name             string
		futureType       string
		withCurrentTimed bool
	}{
		{name: "future timed benefit", futureType: SubscriptionEntitlementTimed, withCurrentTimed: true},
		{name: "future credit balance", futureType: SubscriptionEntitlementCreditBalance},
	}
	for testIndex, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			ensureSubscriptionPreConsumeRecordTableForTest(t)
			ClearPrimaryBillableSubscriptionCacheForTest()
			baseID := 8031 + testIndex*10
			futurePlanID := baseID + 1
			futureSubscriptionID := baseID + 2
			InvalidateSubscriptionPlanCache(futurePlanID)
			t.Cleanup(func() { InvalidateSubscriptionPlanCache(futurePlanID) })
			user := User{Id: baseID, Username: fmt.Sprintf("future-candidate-%d", testIndex), Status: common.UserStatusEnabled}
			setting := user.GetSetting()
			setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
			setting.ActiveSubscriptionId = futureSubscriptionID
			user.SetSetting(setting)
			require.NoError(t, DB.Create(&user).Error)
			futureCode := fmt.Sprintf("future-candidate-%d", testIndex)
			require.NoError(t, DB.Create(&SubscriptionPlan{Id: futurePlanID, Title: "Future candidate", EntitlementType: test.futureType, Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &futureCode}).Error)
			now := GetDBTimestamp()
			futureEndTime := now + 7200
			if test.futureType == SubscriptionEntitlementCreditBalance {
				futureEndTime = 0
			}
			require.NoError(t, DB.Create(&UserSubscription{Id: futureSubscriptionID, UserId: user.Id, PlanId: futurePlanID, EntitlementType: test.futureType, Status: "active", StartTime: now + 3600, EndTime: futureEndTime, TokenLimit: 100, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

			wantSelectedID := 0
			if test.withCurrentTimed {
				currentPlanID := baseID + 3
				wantSelectedID = baseID + 4
				InvalidateSubscriptionPlanCache(currentPlanID)
				t.Cleanup(func() { InvalidateSubscriptionPlanCache(currentPlanID) })
				currentCode := fmt.Sprintf("current-candidate-%d", testIndex)
				require.NoError(t, DB.Create(&SubscriptionPlan{Id: currentPlanID, Title: "Current candidate", EntitlementType: SubscriptionEntitlementTimed, Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &currentCode}).Error)
				require.NoError(t, DB.Create(&UserSubscription{Id: wantSelectedID, UserId: user.Id, PlanId: currentPlanID, EntitlementType: SubscriptionEntitlementTimed, Status: "active", StartTime: now - 60, EndTime: now + 3600, TokenLimit: 100, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
			}

			requestID := "future-candidate-" + test.name
			pre, err := PreConsumeUserSubscription(requestID, user.Id, "gpt-4o", 0, 5)
			if wantSelectedID == 0 {
				require.ErrorIs(t, err, ErrNoActiveSubscription)
				assert.Nil(t, pre)
			} else {
				require.NoError(t, err)
				require.NotNil(t, pre)
				assert.Equal(t, wantSelectedID, pre.UserSubscriptionId)
			}
			var future UserSubscription
			require.NoError(t, DB.First(&future, futureSubscriptionID).Error)
			assert.Zero(t, future.TokenUsed)
			var persisted User
			require.NoError(t, DB.First(&persisted, user.Id).Error)
			assert.Equal(t, wantSelectedID, persisted.GetSetting().ActiveSubscriptionId)
			var records []SubscriptionPreConsumeRecord
			require.NoError(t, DB.Where("request_id = ?", requestID).Find(&records).Error)
			if wantSelectedID == 0 {
				assert.Empty(t, records)
			} else {
				require.Len(t, records, 1)
				assert.Equal(t, wantSelectedID, records[0].UserSubscriptionId)
			}
		})
	}
}

func TestSetUserActiveSubscriptionRejectsFutureBenefits(t *testing.T) {
	tests := []struct {
		name            string
		entitlementType string
	}{
		{name: "timed", entitlementType: SubscriptionEntitlementTimed},
		{name: "credit balance", entitlementType: SubscriptionEntitlementCreditBalance},
	}
	for testIndex, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			baseID := 8071 + testIndex*10
			planID := baseID + 1
			InvalidateSubscriptionPlanCache(planID)
			t.Cleanup(func() { InvalidateSubscriptionPlanCache(planID) })
			subscriptionID := baseID + 2
			user := User{Id: baseID, Username: fmt.Sprintf("future-active-%d", testIndex), Status: common.UserStatusEnabled}
			require.NoError(t, DB.Create(&user).Error)
			code := fmt.Sprintf("future-active-%d", testIndex)
			require.NoError(t, DB.Create(&SubscriptionPlan{Id: planID, Title: "Future active", EntitlementType: test.entitlementType, Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
			now := GetDBTimestamp()
			endTime := now + 7200
			if test.entitlementType == SubscriptionEntitlementCreditBalance {
				endTime = 0
			}
			require.NoError(t, DB.Create(&UserSubscription{Id: subscriptionID, UserId: user.Id, PlanId: planID, EntitlementType: test.entitlementType, Status: "active", StartTime: now + 3600, EndTime: endTime, TokenLimit: 100, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

			err := SetUserActiveSubscription(user.Id, subscriptionID)

			require.Error(t, err)
			var persisted User
			require.NoError(t, DB.First(&persisted, user.Id).Error)
			assert.Zero(t, persisted.GetSetting().ActiveSubscriptionId)
		})
	}
}

func TestCachedSingleActiveSelectionAppliesDueQuotaResetBeforePreConsume(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	t.Cleanup(func() {
		ClearPrimaryBillableSubscriptionCacheForTest()
		ClearSubscriptionPlanCacheForTest()
	})
	const userID = 8061
	const planID = 8062
	InvalidateSubscriptionPlanCache(planID)
	t.Cleanup(func() { InvalidateSubscriptionPlanCache(planID) })
	const subscriptionID = 8063
	user := User{Id: userID, Username: "cached-due-reset", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = subscriptionID
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	code := "cached-due-reset"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: planID, Title: "Cached due reset", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code, QuotaResetPeriod: SubscriptionResetDaily}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: subscriptionID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed, Status: "active", StartTime: now - 3600, EndTime: now + 3*86400, TokenLimit: 100, TokenUsed: 80, LastResetTime: now - 3600, NextResetTime: now + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	first, err := PreConsumeUserSubscription("cached-due-reset-first", userID, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, subscriptionID, first.UserSubscriptionId)
	cachedValue, cached := primaryBillableSubscriptionCache.Load(primaryBillableSubscriptionCacheKey(userID))
	require.True(t, cached)
	cachedEntry, ok := cachedValue.(primaryBillableSubscriptionCacheEntry)
	require.True(t, ok)
	cachedEntry.loaded.Subscription.TokenUsed = 90
	cachedEntry.loaded.Subscription.LastResetTime = now - 2*86400
	cachedEntry.loaded.Subscription.NextResetTime = now - 3600
	primaryBillableSubscriptionCache.Store(primaryBillableSubscriptionCacheKey(userID), cachedEntry)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", subscriptionID).Updates(map[string]any{
		"token_used":      cachedEntry.loaded.Subscription.TokenUsed,
		"last_reset_time": cachedEntry.loaded.Subscription.LastResetTime,
		"next_reset_time": cachedEntry.loaded.Subscription.NextResetTime,
	}).Error)

	second, err := PreConsumeUserSubscription("cached-due-reset-second", userID, "gpt-4o", 0, 5)

	require.NoError(t, err)
	assert.Equal(t, subscriptionID, second.UserSubscriptionId)
	var persisted UserSubscription
	require.NoError(t, DB.First(&persisted, subscriptionID).Error)
	assert.Equal(t, int64(5), persisted.TokenUsed)
	assert.Greater(t, persisted.NextResetTime, now)
}

func TestRolledBackQuotaResetCacheCannotBypassDueReset(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	t.Cleanup(func() {
		ClearPrimaryBillableSubscriptionCacheForTest()
		ClearSubscriptionPlanCacheForTest()
	})
	const userID = 8064
	const planID = 8065
	const subscriptionID = 8066
	user := User{Id: userID, Username: "rolled-back-reset-cache", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	setting.ActiveSubscriptionId = subscriptionID
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	code := "rolled-back-reset-cache"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: planID, Title: "Rolled back reset cache", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code, QuotaResetPeriod: SubscriptionResetDaily}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: subscriptionID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed, Status: "active", StartTime: now - 2*86400, EndTime: now + 3*86400, TokenLimit: 100, TokenUsed: 90, LastResetTime: now - 2*86400, NextResetTime: now - 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	err := DB.Transaction(func(tx *gorm.DB) error {
		outcome, selectErr := selectPrimaryBillableSubscriptionTx(tx, userID, now, 5, true, true, "gpt-4o")
		if selectErr != nil {
			return selectErr
		}
		if outcome.Selection == nil {
			return fmt.Errorf("expected reset selection")
		}
		return fmt.Errorf("force rollback")
	})
	require.EqualError(t, err, "force rollback")
	cachedValue, cached := primaryBillableSubscriptionCache.Load(primaryBillableSubscriptionCacheKey(userID))
	require.True(t, cached)
	cachedEntry, ok := cachedValue.(primaryBillableSubscriptionCacheEntry)
	require.True(t, ok)
	assert.Greater(t, cachedEntry.loaded.Subscription.NextResetTime, now)
	var rolledBack UserSubscription
	require.NoError(t, DB.First(&rolledBack, subscriptionID).Error)
	assert.Equal(t, int64(90), rolledBack.TokenUsed)
	assert.LessOrEqual(t, rolledBack.NextResetTime, now)

	result, err := PreConsumeUserSubscription("rolled-back-reset-cache", userID, "gpt-4o", 0, 5)

	require.NoError(t, err)
	assert.Equal(t, subscriptionID, result.UserSubscriptionId)
	var persisted UserSubscription
	require.NoError(t, DB.First(&persisted, subscriptionID).Error)
	assert.Equal(t, int64(5), persisted.TokenUsed)
	assert.Greater(t, persisted.NextResetTime, now)
}

func TestSubscriptionBillingStrategySelectionMatrix(t *testing.T) {
	type candidateSpec struct {
		entitlementType string
		status          string
		enabled         bool
		endOffset       int64
		tokenLimit      int64
		tokenUsed       int64
		grantReason     string
		modelLimits     string
	}
	tests := []struct {
		name               string
		strategy           string
		activeIndex        int
		candidates         []candidateSpec
		modelName          string
		requiredTokens     int64
		wantSelectionIndex int
		wantActiveIndex    int
		wantOrder          []int
	}{
		{
			name:        "default single active repairs to earliest timed trial",
			activeIndex: -1,
			candidates: []candidateSpec{
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 7200, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 1800, tokenLimit: 100, grantReason: "trial_code"},
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 3600, tokenLimit: 100, grantReason: SubscriptionGrantMonthlyInviteEntitlement},
				{entitlementType: SubscriptionEntitlementCreditBalance, status: "active", enabled: true, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
			},
			modelName:          "gpt-4o",
			requiredTokens:     5,
			wantSelectionIndex: 1,
			wantActiveIndex:    1,
			wantOrder:          []int{1},
		},
		{
			name:        "active fallback keeps disabled plan entitlement and orders timed before credit",
			strategy:    SubscriptionBillingStrategyActiveFallback,
			activeIndex: 0,
			candidates: []candidateSpec{
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: false, endOffset: 900, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 1800, tokenLimit: 100, grantReason: SubscriptionGrantMonthlyInviteEntitlement},
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 3600, tokenLimit: 100, grantReason: "redemption"},
				{entitlementType: SubscriptionEntitlementCreditBalance, status: "active", enabled: true, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
			},
			modelName:          "gpt-4o",
			requiredTokens:     5,
			wantSelectionIndex: 0,
			wantActiveIndex:    0,
			wantOrder:          []int{0, 1, 2, 3},
		},
		{
			name:        "single active repairs expired timed benefit to credit balance",
			strategy:    SubscriptionBillingStrategySingleActive,
			activeIndex: 0,
			candidates: []candidateSpec{
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: -1, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
				{entitlementType: SubscriptionEntitlementCreditBalance, status: "active", enabled: true, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
			},
			modelName:          "gpt-4o",
			requiredTokens:     5,
			wantSelectionIndex: 1,
			wantActiveIndex:    1,
			wantOrder:          []int{1},
		},
		{
			name:        "timed first uses expiry and id order then credit",
			strategy:    SubscriptionBillingStrategyTimedFirst,
			activeIndex: 1,
			candidates: []candidateSpec{
				{entitlementType: SubscriptionEntitlementCreditBalance, status: "active", enabled: true, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 3600, tokenLimit: 100, grantReason: "admin"},
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 1800, tokenLimit: 100, grantReason: "trial_code"},
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 1800, tokenLimit: 100, grantReason: SubscriptionGrantMonthlyInviteEntitlement},
			},
			modelName:          "gpt-4o",
			requiredTokens:     5,
			wantSelectionIndex: 2,
			wantActiveIndex:    1,
			wantOrder:          []int{2, 3, 1, 0},
		},
		{
			name:        "active fallback skips insufficient timed benefits and selects credit",
			strategy:    SubscriptionBillingStrategyActiveFallback,
			activeIndex: 0,
			candidates: []candidateSpec{
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 1800, tokenLimit: 100, tokenUsed: 98, grantReason: SubscriptionGrantOrder},
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 3600, tokenLimit: 100, tokenUsed: 100, grantReason: "redemption"},
				{entitlementType: SubscriptionEntitlementCreditBalance, status: "active", enabled: true, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
			},
			modelName:          "gpt-4o",
			requiredTokens:     5,
			wantSelectionIndex: 2,
			wantActiveIndex:    0,
			wantOrder:          []int{0, 1, 2},
		},
		{
			name:        "model restriction on first candidate stops fallback",
			strategy:    SubscriptionBillingStrategyActiveFallback,
			activeIndex: 0,
			candidates: []candidateSpec{
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 1800, tokenLimit: 100, grantReason: SubscriptionGrantOrder, modelLimits: "claude-3-7-sonnet"},
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: 3600, tokenLimit: 100, grantReason: SubscriptionGrantOrder, modelLimits: "gpt-4o"},
			},
			modelName:          "gpt-4o",
			requiredTokens:     5,
			wantSelectionIndex: -1,
			wantActiveIndex:    0,
			wantOrder:          []int{0, 1},
		},
		{
			name:        "no eligible candidate clears dangling active reference",
			strategy:    SubscriptionBillingStrategySingleActive,
			activeIndex: 0,
			candidates: []candidateSpec{
				{entitlementType: SubscriptionEntitlementTimed, status: "active", enabled: true, endOffset: -1, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
				{entitlementType: SubscriptionEntitlementTimed, status: "cancelled", enabled: true, endOffset: 3600, tokenLimit: 100, grantReason: SubscriptionGrantOrder},
			},
			modelName:          "gpt-4o",
			requiredTokens:     5,
			wantSelectionIndex: -1,
			wantActiveIndex:    -1,
			wantOrder:          []int{},
		},
	}

	for testIndex, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			ClearPrimaryBillableSubscriptionCacheForTest()
			baseID := 18000 + testIndex*20
			user := User{Id: baseID, Username: "strategy-matrix", Status: common.UserStatusEnabled}
			setting := user.GetSetting()
			setting.SubscriptionBillingStrategy = test.strategy
			if test.activeIndex >= 0 {
				setting.ActiveSubscriptionId = baseID + 11 + test.activeIndex
			}
			user.SetSetting(setting)
			require.NoError(t, DB.Create(&user).Error)
			now := common.GetTimestamp()
			for candidateIndex, spec := range test.candidates {
				planID := baseID + 1 + candidateIndex
				subscriptionID := baseID + 11 + candidateIndex
				businessCode := fmt.Sprintf("strategy-matrix-%d-%d", testIndex, candidateIndex)
				InvalidateSubscriptionPlanCache(planID)
				t.Cleanup(func() { InvalidateSubscriptionPlanCache(planID) })
				require.NoError(t, DB.Create(&SubscriptionPlan{
					Id:                planID,
					Title:             test.name,
					EntitlementType:   spec.entitlementType,
					Enabled:           spec.enabled,
					MonthlyTokenLimit: spec.tokenLimit,
					ConcurrencyLimit:  1,
					BusinessCode:      &businessCode,
					ModelLimits:       spec.modelLimits,
				}).Error)
				if !spec.enabled {
					require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", planID).Update("enabled", false).Error)
					InvalidateSubscriptionPlanCache(planID)
				}
				endTime := now + spec.endOffset
				if spec.entitlementType == SubscriptionEntitlementCreditBalance {
					endTime = 0
				}
				require.NoError(t, DB.Create(&UserSubscription{
					Id:              subscriptionID,
					UserId:          user.Id,
					PlanId:          planID,
					EntitlementType: spec.entitlementType,
					Status:          spec.status,
					TokenLimit:      spec.tokenLimit,
					TokenUsed:       spec.tokenUsed,
					EndTime:         endTime,
					GrantReason:     spec.grantReason,
					Source:          spec.grantReason,
				}).Error)
			}

			var outcome primaryBillableSelectionOutcome
			require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
				selected, err := selectPrimaryBillableSubscriptionTx(tx, user.Id, now, test.requiredTokens, true, false, test.modelName)
				outcome = selected
				return err
			}))

			wantOrder := make([]int, 0, len(test.wantOrder))
			for _, index := range test.wantOrder {
				wantOrder = append(wantOrder, baseID+11+index)
			}
			assert.Equal(t, wantOrder, outcome.BillingCandidateSubscriptionIds)
			if test.wantSelectionIndex < 0 {
				assert.Nil(t, outcome.Selection)
			} else {
				require.NotNil(t, outcome.Selection)
				assert.Equal(t, baseID+11+test.wantSelectionIndex, outcome.Selection.Subscription.Id)
			}
			wantActiveID := 0
			if test.wantActiveIndex >= 0 {
				wantActiveID = baseID + 11 + test.wantActiveIndex
			}
			assert.Equal(t, wantActiveID, outcome.ActiveSubscriptionId)
			var persisted User
			require.NoError(t, DB.First(&persisted, user.Id).Error)
			assert.Equal(t, wantActiveID, persisted.GetSetting().ActiveSubscriptionId)
		})
	}
}
