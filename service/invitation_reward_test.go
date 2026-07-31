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
		seedInvitationRewardPlan(t, 2001, "basic_monthly", true)
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
		assert.Equal(t, paidPlan.Id, sub.PlanId)
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

	t.Run("counts redemption and admin granted active paid plans", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1301, 1302, 1303)
		seedInvitationRewardPlan(t, 2301, "trial_monthly", false)
		paidPlan := seedInvitationRewardPlan(t, 2302, "standard_monthly", true)
		seedActiveInviteeSubscription(t, 1302, paidPlan.Id, at, "redemption", "redemption")
		seedActiveInviteeSubscription(t, 1303, paidPlan.Id, at, "admin", "admin")

		status, err := EnsureMonthlyInvitationEntitlement(1301, at)

		require.NoError(t, err)
		assert.True(t, status.Entitled)
		assert.Equal(t, 2, status.QualifiedActiveCount)
		assert.Equal(t, paidPlan.Id, status.RewardPlanId)
	})

	t.Run("excludes trial and invitation reward subscriptions", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1401, 1402, 1403, 1404)
		trialPlan := seedInvitationRewardPlan(t, 2401, "trial_monthly", true)
		require.NoError(t, model.DB.Model(trialPlan).Update("is_trial", true).Error)
		paidPlan := seedInvitationRewardPlan(t, 2402, "standard_monthly", true)
		seedActiveInviteeSubscription(t, 1402, trialPlan.Id, at, "redemption", "redemption")
		seedActiveInviteeSubscription(t, 1403, paidPlan.Id, at, model.SubscriptionGrantMonthlyInviteEntitlement, model.SubscriptionGrantMonthlyInviteEntitlement)
		seedActiveInviteeSubscription(t, 1404, paidPlan.Id, at, "redemption", "redemption")

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

	t.Run("counts redemption and admin granted paid subscriptions", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1551, 1552, 1553)
		seedInvitationRewardPlan(t, 2551, "trial_monthly", false)
		paidPlan := seedInvitationRewardPlan(t, 2552, "standard_monthly", true)
		seedActiveInviteeSubscription(t, 1552, paidPlan.Id, at, "redemption", "redemption")
		seedActiveInviteeSubscription(t, 1553, paidPlan.Id, at, "admin", "admin")

		status, err := EnsureMonthlyInvitationEntitlement(1551, at)

		require.NoError(t, err)
		assert.True(t, status.Entitled)
		assert.Equal(t, 2, status.QualifiedActiveCount)
		assert.Equal(t, paidPlan.Id, status.RewardPlanId)
	})

	t.Run("excludes trial and invitation reward subscriptions", func(t *testing.T) {
		truncate(t)
		require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
		at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
		seedInvitationRewardUsers(t, 1561, 1562, 1563, 1564)
		trialPlan := seedInvitationRewardPlan(t, 2561, "trial_monthly", true)
		require.NoError(t, model.DB.Model(trialPlan).Update("is_trial", true).Error)
		paidPlan := seedInvitationRewardPlan(t, 2562, "standard_monthly", true)
		seedActiveInviteeSubscription(t, 1562, trialPlan.Id, at, "redemption", "redemption")
		seedActiveInviteeSubscription(t, 1563, paidPlan.Id, at, "monthly_invite_entitlement", "monthly_invite_entitlement")
		seedActiveInviteeSubscription(t, 1564, paidPlan.Id, at, "redemption", "redemption")

		status, err := EnsureMonthlyInvitationEntitlement(1561, at)

		require.NoError(t, err)
		assert.False(t, status.Entitled)
		assert.Equal(t, 1, status.QualifiedActiveCount)
	})
}

func TestConvertedTimedInviteePreservesGrantedRewardAndLosesFutureQualification(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.InvitationMonthlyEntitlement{},
		&model.CreditBalanceLedger{},
		&model.SubscriptionConversion{},
	))
	at := time.Unix(common.GetTimestamp(), 0).UTC()
	const inviterID = 15_701
	const convertedInviteeID = 15_702
	const activeInviteeID = 15_703
	const timedPlanID = 25_701
	const creditPlanID = 25_702
	const convertedSourceID = 35_701
	seedInvitationRewardUsers(t, inviterID, convertedInviteeID, activeInviteeID)
	timedCode := "conversion-reward-history"
	creditCode := "conversion-reward-credit"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id: timedPlanID, Title: "Conversion reward history", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, RewardEligible: true, BusinessCode: &timedCode,
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100,
		ConcurrencyLimit: 1, TimedConversionEnabled: true,
	}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id: creditPlanID, Title: "Conversion reward Credit", EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true,
		CreditBalanceConversionEnabled: true,
	}).Error)
	basis := int64(100)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id: convertedSourceID, UserId: convertedInviteeID, PlanId: timedPlanID,
		EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusActive,
		TokenLimit: 100, TokenUsed: 10, StartTime: at.Add(-48 * time.Hour).Unix(), EndTime: at.AddDate(0, 2, 0).Unix(),
		GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
		LastGrantedAt: at.Add(-48 * time.Hour).Unix(), LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive, LastGrantSource: model.SubscriptionGrantOrder,
	}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id: 35_702, UserId: activeInviteeID, PlanId: timedPlanID,
		EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusActive,
		TokenLimit: 100, StartTime: at.Add(-48 * time.Hour).Unix(), EndTime: at.AddDate(0, 2, 0).Unix(),
		GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
	}).Error)

	granted, err := EnsureMonthlyInvitationEntitlement(inviterID, at)
	require.NoError(t, err)
	require.True(t, granted.Entitled)
	require.Equal(t, 2, granted.QualifiedActiveCount)
	require.Positive(t, granted.RewardSubscriptionId)
	var rewardBefore model.UserSubscription
	require.NoError(t, model.DB.First(&rewardBefore, granted.RewardSubscriptionId).Error)
	require.Equal(t, model.SubscriptionStatusActive, rewardBefore.Status)
	var historyBefore model.InvitationMonthlyEntitlement
	require.NoError(t, model.DB.Where("inviter_id = ? AND reward_month = ?", inviterID, granted.RewardMonth).First(&historyBefore).Error)
	require.Equal(t, model.InvitationEntitlementStatusQualified, historyBefore.Status)
	require.Equal(t, rewardBefore.Id, historyBefore.RewardSubscriptionId)

	conversion, err := model.ConfirmTimedSubscriptionConversion(convertedInviteeID, convertedSourceID, "conversion-reward-history")
	require.NoError(t, err)
	require.False(t, conversion.Replayed)
	var rewardAfter model.UserSubscription
	require.NoError(t, model.DB.First(&rewardAfter, rewardBefore.Id).Error)
	assert.Equal(t, rewardBefore.Status, rewardAfter.Status)
	assert.Equal(t, rewardBefore.PlanId, rewardAfter.PlanId)
	assert.Equal(t, rewardBefore.StartTime, rewardAfter.StartTime)
	assert.Equal(t, rewardBefore.EndTime, rewardAfter.EndTime)
	assert.Equal(t, rewardBefore.TokenLimit, rewardAfter.TokenLimit)
	assert.Equal(t, rewardBefore.GrantReason, rewardAfter.GrantReason)
	var historyAfter model.InvitationMonthlyEntitlement
	require.NoError(t, model.DB.First(&historyAfter, historyBefore.Id).Error)
	assert.Equal(t, model.InvitationEntitlementStatusQualified, historyAfter.Status)
	assert.Equal(t, historyBefore.RewardSubscriptionId, historyAfter.RewardSubscriptionId)
	assert.Equal(t, historyBefore.QualifiedActiveCount, historyAfter.QualifiedActiveCount)

	futureAt := at.AddDate(0, 1, 0)
	future, err := EnsureMonthlyInvitationEntitlement(inviterID, futureAt)
	require.NoError(t, err)
	assert.Equal(t, 1, future.QualifiedActiveCount)
	assert.False(t, future.Entitled)
	assert.Zero(t, future.RewardSubscriptionId)
	var futureHistoryCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).
		Where("inviter_id = ? AND reward_month = ?", inviterID, future.RewardMonth).
		Count(&futureHistoryCount).Error)
	assert.Zero(t, futureHistoryCount)
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

func TestMonthlyInvitationEntitlementNextResetUsesShanghaiNaturalMonth(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	seedInvitationRewardUsers(t, 1651, 1652, 1653)
	plan := seedInvitationRewardPlan(t, 2651, "standard_monthly", true)
	require.NoError(t, model.DB.Model(plan).Update("quota_reset_period", model.SubscriptionResetMonthly).Error)
	end := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC).Unix()
	seedPaidInviteeSubscriptionWithEnd(t, 1652, plan.Id, at, end)
	seedPaidInviteeSubscriptionWithEnd(t, 1653, plan.Id, at, end)

	status, err := EnsureMonthlyInvitationEntitlement(1651, at)

	require.NoError(t, err)
	require.True(t, status.Entitled)
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, status.RewardSubscriptionId).Error)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, shanghai).Unix(), sub.NextResetTime)
}

func TestNextInvitationEntitlementRefreshAtUsesShanghaiMidnight(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	got := nextInvitationEntitlementRefreshAt(time.Date(2026, 5, 28, 15, 30, 0, 0, time.UTC))
	assert.Equal(t, time.Date(2026, 5, 29, 0, 0, 0, 0, shanghai).Unix(), got.Unix())

	got = nextInvitationEntitlementRefreshAt(time.Date(2026, 5, 28, 16, 0, 0, 0, time.UTC))
	assert.Equal(t, time.Date(2026, 5, 30, 0, 0, 0, 0, shanghai).Unix(), got.Unix())
}

func TestMonthlyInvitationEntitlementSelectsHighestQualifiedPaidTierAndDowngrade(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	seedInvitationRewardUsers(t, 1701, 1702, 1703, 1704, 1705)
	basicPlan := seedInvitationRewardPlanWithRank(t, 2701, "basic_monthly", true, 4, 40)
	proPlan := seedInvitationRewardPlanWithRank(t, 2702, "pro_monthly", true, 2, 80)
	maxPlan := seedInvitationRewardPlanWithRank(t, 2703, "max_monthly", true, 0, 160)
	seedPaidInviteeSubscriptionWithEnd(t, 1702, basicPlan.Id, at, at.Add(90*24*time.Hour).Unix())
	seedPaidInviteeSubscriptionWithEnd(t, 1703, basicPlan.Id, at, at.Add(80*24*time.Hour).Unix())
	seedPaidInviteeSubscriptionWithEnd(t, 1704, proPlan.Id, at, at.Add(20*24*time.Hour).Unix())
	seedPaidInviteeSubscriptionWithEnd(t, 1705, proPlan.Id, at, at.Add(30*24*time.Hour).Unix())
	seedPaidInviteeSubscriptionWithEnd(t, 1702, maxPlan.Id, at, at.Add(10*24*time.Hour).Unix())

	status, err := EnsureMonthlyInvitationEntitlement(1701, at)

	require.NoError(t, err)
	require.True(t, status.Entitled)
	assert.Equal(t, proPlan.Id, status.RewardPlanId)
	assert.Equal(t, "pro_monthly", status.RewardPlanBusinessCode)
	assert.Equal(t, "pro_monthly", status.RewardPlanTitle)
	assert.Equal(t, 3, status.RewardTierQualifiedCount)
	assert.Equal(t, at.Add(20*24*time.Hour).Unix(), status.EntitlementEndTime)
	assert.Equal(t, basicPlan.Id, status.DowngradeRewardPlanId)
	assert.Equal(t, "basic_monthly", status.DowngradeRewardPlanBusinessCode)
	assert.Equal(t, "basic_monthly", status.DowngradeRewardPlanTitle)
	assert.Equal(t, at.Add(80*24*time.Hour).Unix(), status.DowngradeEntitlementEndTime)
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, status.RewardSubscriptionId).Error)
	assert.Equal(t, proPlan.Id, sub.PlanId)
	assert.Equal(t, at.Add(20*24*time.Hour).Unix(), sub.EndTime)

	afterProExpires, err := EnsureMonthlyInvitationEntitlement(1701, at.Add(21*24*time.Hour))
	require.NoError(t, err)
	require.True(t, afterProExpires.Entitled)
	assert.Equal(t, basicPlan.Id, afterProExpires.RewardPlanId)
	assert.Equal(t, at.Add(80*24*time.Hour).Unix(), afterProExpires.EntitlementEndTime)
	assert.Zero(t, afterProExpires.DowngradeRewardPlanId)
	sub = model.UserSubscription{}
	require.NoError(t, model.DB.First(&sub, afterProExpires.RewardSubscriptionId).Error)
	assert.Equal(t, basicPlan.Id, sub.PlanId)
	assert.Equal(t, at.Add(80*24*time.Hour).Unix(), sub.EndTime)
}

func TestMonthlyInvitationEntitlementCountsHigherTierTowardLowerTier(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	seedInvitationRewardUsers(t, 1901, 1902, 1903)
	seedInvitationRewardPlanWithRank(t, 2901, "basic_monthly", true, 4, 40)
	proPlan := seedInvitationRewardPlanWithRank(t, 2902, "pro_monthly", true, 2, 80)
	maxPlan := seedInvitationRewardPlanWithRank(t, 2903, "max_monthly", true, 0, 160)
	seedPaidInviteeSubscriptionWithEnd(t, 1902, maxPlan.Id, at, at.Add(30*24*time.Hour).Unix())
	seedPaidInviteeSubscriptionWithEnd(t, 1903, proPlan.Id, at, at.Add(20*24*time.Hour).Unix())

	status, err := EnsureMonthlyInvitationEntitlement(1901, at)

	require.NoError(t, err)
	require.True(t, status.Entitled)
	assert.Equal(t, proPlan.Id, status.RewardPlanId)
	assert.Equal(t, "pro_monthly", status.RewardPlanBusinessCode)
	assert.Equal(t, 2, status.RewardTierQualifiedCount)
	assert.Equal(t, at.Add(20*24*time.Hour).Unix(), status.EntitlementEndTime)
	assert.Zero(t, status.DowngradeRewardPlanId)
	assert.Zero(t, status.DowngradeEntitlementEndTime)
}

func TestMonthlyInvitationEntitlementDoesNotRequireConfiguredRewardPlanCode(t *testing.T) {
	truncate(t)
	common.OptionMapRWMutex.Lock()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{"MonthlyInvitationRewardPlanCode": "missing_reward_monthly"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldMap
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, model.DB.AutoMigrate(&model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	seedInvitationRewardUsers(t, 1801, 1802, 1803)
	seedInvitationRewardPlan(t, 2801, "basic_monthly", true)
	paidPlan := seedInvitationRewardPlan(t, 2802, "standard_monthly", true)
	seedPaidInviteeSubscription(t, 1802, paidPlan.Id, at)
	seedPaidInviteeSubscription(t, 1803, paidPlan.Id, at)

	status, err := EnsureMonthlyInvitationEntitlement(1801, at)

	require.NoError(t, err)
	require.True(t, status.Entitled)
	assert.Equal(t, paidPlan.Id, status.RewardPlanId)
	assert.NotZero(t, status.RewardSubscriptionId)
}

func TestGetInvitationEntitlementStatusSkipsCommissionInviterWithoutUpsert(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	require.NoError(t, model.DB.Create(&model.User{Id: 9301, Username: "commission-inviter", Status: common.UserStatusEnabled, AffCode: "aff-commission-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9302, Username: "commission-child-a", Status: common.UserStatusEnabled, AffCode: "aff-commission-child-a", InviterId: 9301}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9303, Username: "commission-child-b", Status: common.UserStatusEnabled, AffCode: "aff-commission-child-b", InviterId: 9301}).Error)
	plan := seedInvitationRewardPlan(t, 9304, "commission_paid", true)
	seedActiveInviteeSubscription(t, 9302, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)
	seedActiveInviteeSubscription(t, 9303, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)

	status, err := GetInvitationEntitlementStatus(9301, at)

	require.NoError(t, err)
	require.NotNil(t, status)
	assert.False(t, status.Entitled)
	assert.Equal(t, 2, status.DirectInviteCount)
	assert.Equal(t, 2, status.QualifiedActiveCount)
	assert.Zero(t, status.RewardSubscriptionId)
	var entitlementCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 9301).Count(&entitlementCount).Error)
	assert.Equal(t, int64(0), entitlementCount)
	var rewardSubCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND grant_reason = ?", 9301, model.SubscriptionGrantMonthlyInviteEntitlement).Count(&rewardSubCount).Error)
	assert.Equal(t, int64(0), rewardSubCount)
}

func TestInvitationEntitlementKeepsExistingActiveSubscriptionCriteria(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.InvitationMonthlyEntitlement{}, &model.InvitationRewardEvent{}))
	t.Cleanup(func() {
		if model.DB.Migrator().HasTable("invitation_reward_events") {
			model.DB.Exec("DELETE FROM invitation_reward_events")
		}
	})
	at := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	seedInvitationRewardUsers(t, 9311, 9312, 9313, 9314)
	paidPlan := seedInvitationRewardPlan(t, 9315, "current_paid", true)
	seedActiveInviteeSubscription(t, 9312, paidPlan.Id, at, "redemption", "redemption")
	seedActiveInviteeSubscription(t, 9313, paidPlan.Id, at, "admin", "admin")
	require.NoError(t, model.DB.Create(&model.InvitationRewardEvent{InviterId: 9311, InviteeId: 9314, SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: 9316, EventStartTime: at.Add(-time.Hour).Unix(), EventEndTime: at.Add(24 * time.Hour).Unix(), Status: model.InvitationRewardEventStatusActive}).Error)

	status, err := EnsureMonthlyInvitationEntitlement(9311, at)

	require.NoError(t, err)
	assert.True(t, status.Entitled)
	assert.Equal(t, 2, status.QualifiedActiveCount)
	assert.Equal(t, paidPlan.Id, status.RewardPlanId)
}

func TestInvitationEntitlementExcludesInviteTrialPlansFromQualifiedActiveCount(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	require.NoError(t, model.DB.Create(&model.User{Id: 9371, Username: "invite-trial-boundary-inviter", Status: common.UserStatusEnabled, AffCode: "aff-invite-trial-boundary-inviter"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9372, Username: "invite-trial-boundary-child-a", Status: common.UserStatusEnabled, AffCode: "aff-invite-trial-boundary-child-a", InviterId: 9371}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9373, Username: "invite-trial-boundary-child-b", Status: common.UserStatusEnabled, AffCode: "aff-invite-trial-boundary-child-b", InviterId: 9371}).Error)
	plan := seedInvitationRewardPlan(t, 9374, "invite_trial_paid_boundary", true)
	require.NoError(t, model.DB.Model(plan).Updates(map[string]interface{}{
		"invite_trial": true,
		"is_trial":     false,
	}).Error)
	seedActiveInviteeSubscription(t, 9372, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)
	seedActiveInviteeSubscription(t, 9373, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)

	status, err := GetInvitationEntitlementStatus(9371, at)

	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, 2, status.DirectInviteCount)
	assert.Equal(t, 0, status.QualifiedActiveCount)
	assert.False(t, status.Entitled)
	assert.Zero(t, status.RewardSubscriptionId)
	var entitlementCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 9371).Count(&entitlementCount).Error)
	assert.Equal(t, int64(0), entitlementCount)
	var rewardSubCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND grant_reason = ?", 9371, model.SubscriptionGrantMonthlyInviteEntitlement).Count(&rewardSubCount).Error)
	assert.Equal(t, int64(0), rewardSubCount)
}

func TestRunMonthlyInvitationEntitlementSweepSkipsCommissionInviters(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.InvitationMonthlyEntitlement{}))
	at := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	require.NoError(t, model.DB.Create(&model.User{Id: 9321, Username: "sweep-commission", Status: common.UserStatusEnabled, AffCode: "aff-sweep-commission", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9322, Username: "sweep-child-a", Status: common.UserStatusEnabled, AffCode: "aff-sweep-child-a", InviterId: 9321}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9323, Username: "sweep-child-b", Status: common.UserStatusEnabled, AffCode: "aff-sweep-child-b", InviterId: 9321}).Error)
	plan := seedInvitationRewardPlan(t, 9324, "sweep_paid", true)
	seedActiveInviteeSubscription(t, 9322, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)
	seedActiveInviteeSubscription(t, 9323, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)

	processed, err := RunMonthlyInvitationEntitlementSweep(at, 10)

	require.NoError(t, err)
	assert.Equal(t, 0, processed)
	var entitlementCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 9321).Count(&entitlementCount).Error)
	assert.Equal(t, int64(0), entitlementCount)
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

func seedInvitationRewardPlanWithRank(t *testing.T, id int, businessCode string, rewardEligible bool, sortOrder int, priceAmount float64) *model.SubscriptionPlan {
	t.Helper()
	plan := seedInvitationRewardPlan(t, id, businessCode, rewardEligible)
	require.NoError(t, model.DB.Model(plan).Updates(map[string]interface{}{
		"sort_order":   sortOrder,
		"price_amount": priceAmount,
	}).Error)
	plan.SortOrder = sortOrder
	plan.PriceAmount = priceAmount
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
