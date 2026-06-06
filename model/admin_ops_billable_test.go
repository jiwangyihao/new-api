package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedAdminOpsSubscriptionPlanForBillableTest(t *testing.T, id int, title string, code string, tokenLimit int64, concurrencyLimit int, queueCapacity int) {
	t.Helper()
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: id, Title: title, Enabled: true, MonthlyTokenLimit: tokenLimit, ConcurrencyLimit: concurrencyLimit, QueueCapacity: queueCapacity, BusinessCode: &code}).Error)
}

func TestGetAdminOpsUserConcurrencyLimitsMatchesPrimaryBillableInviteRewardSelection(t *testing.T) {
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	seedAdminOpsSubscriptionPlanForBillableTest(t, 7712, "Basic", "basic_monthly", 100, 3, 5)
	paidEnd := common.GetTimestamp() + 24*3600
	rewardEnd := common.GetTimestamp() + 3*86400
	require.NoError(t, DB.Create(&User{Id: 7711, Username: "admin-ops-same-tier", Status: common.UserStatusEnabled, AffCode: "aff7711"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7713, UserId: 7711, PlanId: 7712, Status: "active", TokenLimit: 100, TokenUsed: 10, EndTime: paidEnd, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7714, UserId: 7711, PlanId: 7712, Status: "active", TokenLimit: 100, TokenUsed: 25, EndTime: rewardEnd, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{7711})

	require.NoError(t, err)
	limit := limits[7711]
	assert.Equal(t, 7712, limit.PlanID)
	assert.EqualValues(t, 100, limit.TokenLimit)
	assert.EqualValues(t, 25, limit.TokenUsed)
	assert.Equal(t, 3, limit.Limit)
	assert.Equal(t, 5, limit.QueueCapacity)
}

func TestGetAdminOpsUserConcurrencyLimitsMatchesRedemptionPaidInviteRewardSelection(t *testing.T) {
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	seedAdminOpsSubscriptionPlanForBillableTest(t, 7742, "Basic", "admin_ops_redemption_reward", 100, 3, 5)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 7741, Username: "admin-ops-redemption-tier", Status: common.UserStatusEnabled, AffCode: "aff7741"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7743, UserId: 7741, PlanId: 7742, Status: "active", TokenLimit: 100, TokenUsed: 10, EndTime: now + 24*3600, GrantReason: "redemption", Source: "redemption"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7744, UserId: 7741, PlanId: 7742, Status: "active", TokenLimit: 100, TokenUsed: 25, EndTime: now + 3*86400, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{7741})

	require.NoError(t, err)
	limit := limits[7741]
	assert.Equal(t, 7742, limit.PlanID)
	assert.EqualValues(t, 100, limit.TokenLimit)
	assert.EqualValues(t, 25, limit.TokenUsed)
	assert.Equal(t, 3, limit.Limit)
	assert.Equal(t, 5, limit.QueueCapacity)
}

func TestGetAdminOpsUserConcurrencyLimitsMatchesActiveSubscriptionSelection(t *testing.T) {
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	seedAdminOpsSubscriptionPlanForBillableTest(t, 7722, "Basic", "basic_monthly", 100, 2, 4)
	seedAdminOpsSubscriptionPlanForBillableTest(t, 7723, "Pro", "pro_monthly", 200, 6, 8)
	user := User{Id: 7721, Username: "admin-ops-active-sub", Status: common.UserStatusEnabled, AffCode: "aff7721"}
	require.NoError(t, DB.Create(&user).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7724, UserId: 7721, PlanId: 7722, Status: "active", TokenLimit: 100, TokenUsed: 20, EndTime: now + 3*86400, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7725, UserId: 7721, PlanId: 7723, Status: "active", TokenLimit: 200, TokenUsed: 40, EndTime: now + 30*86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = 7725
	user.SetSetting(setting)
	require.NoError(t, DB.Save(&user).Error)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{7721})

	require.NoError(t, err)
	limit := limits[7721]
	assert.Equal(t, 7723, limit.PlanID)
	assert.Equal(t, "Pro", limit.PlanTitle)
	assert.EqualValues(t, 200, limit.TokenLimit)
	assert.EqualValues(t, 40, limit.TokenUsed)
	assert.Equal(t, 6, limit.Limit)
	assert.Equal(t, 8, limit.QueueCapacity)
}

func TestGetAdminOpsUserConcurrencyLimitsSkipsExhaustedSubscription(t *testing.T) {
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	seedAdminOpsSubscriptionPlanForBillableTest(t, 7732, "Basic", "basic_monthly", 100, 2, 4)
	seedAdminOpsSubscriptionPlanForBillableTest(t, 7733, "Pro", "pro_monthly", 200, 6, 8)
	require.NoError(t, DB.Create(&User{Id: 7731, Username: "admin-ops-exhausted", Status: common.UserStatusEnabled, AffCode: "aff7731"}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7734, UserId: 7731, PlanId: 7732, Status: "active", TokenLimit: 100, TokenUsed: 100, EndTime: now + 24*3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7735, UserId: 7731, PlanId: 7733, Status: "active", TokenLimit: 200, TokenUsed: 40, EndTime: now + 30*86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{7731})

	require.NoError(t, err)
	limit := limits[7731]
	assert.Equal(t, 7733, limit.PlanID)
	assert.Equal(t, "Pro", limit.PlanTitle)
	assert.EqualValues(t, 200, limit.TokenLimit)
	assert.EqualValues(t, 40, limit.TokenUsed)
	assert.Equal(t, 6, limit.Limit)
	assert.Equal(t, 8, limit.QueueCapacity)
}
