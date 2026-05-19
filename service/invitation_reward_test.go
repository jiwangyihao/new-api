package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonthlyInvitationEntitlement(t *testing.T) {
	t.Run("grants current month Basic when two direct invitees have paid active subscriptions", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1001, 1002, 1003)
		basicPlan := seedInvitationRewardPlan(t, 2001, "basic_monthly", true)
		paidPlan := seedInvitationRewardPlan(t, 2002, "standard_monthly", true)
		seedPaidInviteeSubscription(t, 1002, paidPlan.Id, at)
		seedPaidInviteeSubscription(t, 1003, paidPlan.Id, at)

		status, err := EnsureMonthlyInvitationEntitlement(1001, at)

		require.NoError(t, err)
		assert.True(t, status.Entitled)
		assert.Equal(t, 2, status.QualifiedActiveCount)
		assert.Equal(t, 2, status.DirectInviteCount)
		assert.Equal(t, "2026-05", status.RewardMonth)
		assert.NotZero(t, status.RewardSubscriptionId)
		assert.Equal(t, at.Add(24*time.Hour).Unix(), status.EntitlementEndTime)
		var sub model.UserSubscription
		require.NoError(t, model.DB.First(&sub, status.RewardSubscriptionId).Error)
		assert.Equal(t, basicPlan.Id, sub.PlanId)
		assert.Equal(t, "monthly_invite_entitlement", sub.GrantReason)
		assert.Equal(t, 1001, sub.GrantSourceUserId)
		assert.Equal(t, at.Add(24*time.Hour).Unix(), sub.EndTime)
	})

	t.Run("is idempotent within reward month", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1101, 1102, 1103)
		seedInvitationRewardPlan(t, 2101, "basic_monthly", true)
		paidPlan := seedInvitationRewardPlan(t, 2102, "pro_monthly", true)
		seedPaidInviteeSubscription(t, 1102, paidPlan.Id, at)
		seedPaidInviteeSubscription(t, 1103, paidPlan.Id, at)

		first, err := EnsureMonthlyInvitationEntitlement(1101, at)
		require.NoError(t, err)
		second, err := EnsureMonthlyInvitationEntitlement(1101, at.Add(2*time.Hour))
		require.NoError(t, err)

		assert.Equal(t, first.RewardSubscriptionId, second.RewardSubscriptionId)
		var count int64
		require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND grant_reason = ?", 1101, "monthly_invite_entitlement").Count(&count).Error)
		assert.Equal(t, int64(1), count)
	})

	t.Run("does not grant with only one qualified active invitee", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1201, 1202, 1203)
		seedInvitationRewardPlan(t, 2201, "basic_monthly", true)
		paidPlan := seedInvitationRewardPlan(t, 2202, "standard_monthly", true)
		seedPaidInviteeSubscription(t, 1202, paidPlan.Id, at)

		status, err := EnsureMonthlyInvitationEntitlement(1201, at)

		require.NoError(t, err)
		assert.False(t, status.Entitled)
		assert.Equal(t, 1, status.QualifiedActiveCount)
		assert.Zero(t, status.RewardSubscriptionId)
		var count int64
		require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND grant_reason = ?", 1201, "monthly_invite_entitlement").Count(&count).Error)
		assert.Equal(t, int64(0), count)
	})

	t.Run("ignores active subscriptions without successful paid orders", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1301, 1302, 1303)
		seedInvitationRewardPlan(t, 2301, "basic_monthly", true)
		paidPlan := seedInvitationRewardPlan(t, 2302, "standard_monthly", true)
		seedPaidInviteeSubscription(t, 1302, paidPlan.Id, at)
		seedActiveInviteeSubscription(t, 1303, paidPlan.Id, at, "order", "order")

		status, err := EnsureMonthlyInvitationEntitlement(1301, at)

		require.NoError(t, err)
		assert.False(t, status.Entitled)
		assert.Equal(t, 1, status.QualifiedActiveCount)
		var entCount int64
		require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 1301).Count(&entCount).Error)
		assert.Equal(t, int64(0), entCount)
	})

	t.Run("ignores zero money successful orders", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1401, 1402, 1403)
		seedInvitationRewardPlan(t, 2401, "basic_monthly", true)
		paidPlan := seedInvitationRewardPlan(t, 2402, "standard_monthly", true)
		seedPaidInviteeSubscription(t, 1402, paidPlan.Id, at)
		seedInviteeSubscriptionWithOrder(t, 1403, paidPlan.Id, at, 0, common.TopUpStatusSuccess, "order", "order")

		status, err := EnsureMonthlyInvitationEntitlement(1401, at)

		require.NoError(t, err)
		assert.False(t, status.Entitled)
		assert.Equal(t, 1, status.QualifiedActiveCount)
	})

	t.Run("counts legacy source order paid subscriptions", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1501, 1502, 1503)
		seedInvitationRewardPlan(t, 2501, "basic_monthly", true)
		paidPlan := seedInvitationRewardPlan(t, 2502, "standard_monthly", true)
		seedPaidInviteeSubscription(t, 1502, paidPlan.Id, at)
		seedInviteeSubscriptionWithOrder(t, 1503, paidPlan.Id, at, 10, common.TopUpStatusSuccess, "", "order")

		status, err := EnsureMonthlyInvitationEntitlement(1501, at)

		require.NoError(t, err)
		assert.True(t, status.Entitled)
		assert.Equal(t, 2, status.QualifiedActiveCount)
	})
}

func TestMonthlyInvitationEntitlementUsesTopTwoPaidInviteeOverlapEndTime(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	seedInvitationRewardUsers(t, 1601, 1602, 1603, 1604)
	seedInvitationRewardPlan(t, 2601, "basic_monthly", true)
	paidPlan := seedInvitationRewardPlan(t, 2602, "standard_monthly", true)
	seedPaidInviteeSubscriptionWithEnd(t, 1602, paidPlan.Id, at, at.Add(10*24*time.Hour).Unix())
	seedPaidInviteeSubscriptionWithEnd(t, 1603, paidPlan.Id, at, at.Add(20*24*time.Hour).Unix())
	seedPaidInviteeSubscriptionWithEnd(t, 1604, paidPlan.Id, at, at.Add(30*24*time.Hour).Unix())

	status, err := EnsureMonthlyInvitationEntitlement(1601, at)

	require.NoError(t, err)
	require.True(t, status.Entitled)
	assert.Equal(t, at.Add(20*24*time.Hour).Unix(), status.EntitlementEndTime)
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, status.RewardSubscriptionId).Error)
	assert.Equal(t, at.Add(20*24*time.Hour).Unix(), sub.EndTime)
	readStatus, err := GetInvitationEntitlementStatus(1601, at)
	require.NoError(t, err)
	require.True(t, readStatus.Entitled)
	assert.Equal(t, at.Add(20*24*time.Hour).Unix(), readStatus.EntitlementEndTime)
}

func TestMonthlyInvitationEntitlementUsesConfiguredRewardPlanCode(t *testing.T) {
	truncate(t)
	restore := setInvitationRewardPlanCodeForTest("invite_reward_monthly")
	t.Cleanup(restore)
	require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	seedInvitationRewardUsers(t, 1701, 1702, 1703)
	basicPlan := seedInvitationRewardPlan(t, 2701, "basic_monthly", true)
	rewardPlan := seedInvitationRewardPlan(t, 2702, "invite_reward_monthly", true)
	paidPlan := seedInvitationRewardPlan(t, 2703, "standard_monthly", true)
	seedPaidInviteeSubscription(t, 1702, paidPlan.Id, at)
	seedPaidInviteeSubscription(t, 1703, paidPlan.Id, at)

	status, err := EnsureMonthlyInvitationEntitlement(1701, at)

	require.NoError(t, err)
	require.True(t, status.Entitled)
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, status.RewardSubscriptionId).Error)
	assert.Equal(t, rewardPlan.Id, sub.PlanId)
	assert.NotEqual(t, basicPlan.Id, sub.PlanId)
}

func TestMonthlyInvitationEntitlementRequiresConfiguredRewardPlan(t *testing.T) {
	truncate(t)
	restore := setInvitationRewardPlanCodeForTest("missing_reward_monthly")
	t.Cleanup(restore)
	require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	seedInvitationRewardUsers(t, 1801, 1802, 1803)
	seedInvitationRewardPlan(t, 2801, "basic_monthly", true)
	paidPlan := seedInvitationRewardPlan(t, 2802, "standard_monthly", true)
	seedPaidInviteeSubscription(t, 1802, paidPlan.Id, at)
	seedPaidInviteeSubscription(t, 1803, paidPlan.Id, at)

	status, err := EnsureMonthlyInvitationEntitlement(1801, at)

	require.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "missing_reward_monthly")
	var count int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", 1801).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func setInvitationRewardPlanCodeForTest(code string) func() {
	common.OptionMapRWMutex.Lock()
	oldMap := common.OptionMap
	if oldMap == nil {
		common.OptionMap = map[string]string{}
	} else {
		cloned := make(map[string]string, len(oldMap)+1)
		for key, value := range oldMap {
			cloned[key] = value
		}
		common.OptionMap = cloned
	}
	common.OptionMap["MonthlyInvitationRewardPlanCode"] = code
	common.OptionMapRWMutex.Unlock()
	return func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldMap
		common.OptionMapRWMutex.Unlock()
	}
}
func seedPaidInviteeSubscriptionWithEnd(t *testing.T, userId int, planId int, at time.Time, end int64) {
	t.Helper()
	start := at.Add(-24 * time.Hour).Unix()
	tradeNo := fmt.Sprintf("paid-order-%d-%d", userId, end)
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: userId, PlanId: planId, Money: 10, TradeNo: tradeNo, PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: start, CompleteTime: start}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: userId, PlanId: planId, Status: "active", StartTime: start, EndTime: end, GrantReason: "order", Source: "order"}).Error)
}

func seedInvitationRewardUsers(t *testing.T, inviterId int, inviteeIds ...int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{Id: inviterId, Username: "inviter-" + string(rune('a'+inviterId%26)), Status: common.UserStatusEnabled, AffCode: "aff-inviter-" + string(rune('a'+inviterId%26))}).Error)
	for idx, id := range inviteeIds {
		require.NoError(t, model.DB.Create(&model.User{Id: id, Username: "invitee-" + string(rune('a'+id%26)), DisplayName: "invitee", Status: common.UserStatusEnabled, AffCode: "aff-invitee-" + string(rune('a'+id%26)), InviterId: inviterId, Quota: idx}).Error)
	}
}

func seedInvitationRewardPlan(t *testing.T, id int, businessCode string, rewardEligible bool) *model.SubscriptionPlan {
	t.Helper()
	code := businessCode
	plan := &model.SubscriptionPlan{Id: id, Title: businessCode, Enabled: true, PriceAmount: 10, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1_000_000, ConcurrencyLimit: 1, RewardEligible: rewardEligible, BusinessCode: &code}
	require.NoError(t, model.DB.Create(plan).Error)
	return plan
}

func seedPaidInviteeSubscription(t *testing.T, userId int, planId int, at time.Time) {
	t.Helper()
	seedInviteeSubscriptionWithOrder(t, userId, planId, at, 10, common.TopUpStatusSuccess, "order", "order")
}

func seedActiveInviteeSubscription(t *testing.T, userId int, planId int, at time.Time, grantReason string, source string) {
	t.Helper()
	start := at.Add(-24 * time.Hour).Unix()
	end := at.Add(24 * time.Hour).Unix()
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: userId, PlanId: planId, Status: "active", StartTime: start, EndTime: end, GrantReason: grantReason, Source: source}).Error)
}

func seedInviteeSubscriptionWithOrder(t *testing.T, userId int, planId int, at time.Time, money float64, status string, grantReason string, source string) {
	t.Helper()
	start := at.Add(-24 * time.Hour).Unix()
	tradeNo := "paid-order-" + time.Now().Format("150405.000000000") + "-" + string(rune('a'+userId%26))
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: userId, PlanId: planId, Money: money, TradeNo: tradeNo, PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: status, CreateTime: start, CompleteTime: start}).Error)
	seedActiveInviteeSubscription(t, userId, planId, at, grantReason, source)
}
