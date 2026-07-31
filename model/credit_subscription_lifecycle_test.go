package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreditBalanceLifecycleAcrossBillingStrategiesAndCache(t *testing.T) {
	strategies := []string{
		SubscriptionBillingStrategySingleActive,
		SubscriptionBillingStrategyActiveFallback,
		SubscriptionBillingStrategyTimedFirst,
	}
	for index, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			truncateTables(t)
			require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
			ClearPrimaryBillableSubscriptionCacheForTest()

			baseID := 24_000 + index*10
			userID, planID, subscriptionID := baseID+1, baseID+2, baseID+3
			user := User{Id: userID, Username: fmt.Sprintf("credit-lifecycle-%s", strategy), Status: common.UserStatusEnabled}
			setting := user.GetSetting()
			setting.ActiveSubscriptionId = subscriptionID
			setting.SubscriptionBillingStrategy = strategy
			user.SetSetting(setting)
			require.NoError(t, DB.Create(&user).Error)

			businessCode := fmt.Sprintf("credit-lifecycle-%s", strategy)
			require.NoError(t, DB.Create(&SubscriptionPlan{
				Id:                             planID,
				Title:                          "Credit balance",
				EntitlementType:                SubscriptionEntitlementCreditBalance,
				Enabled:                        true,
				ModelLimits:                    "gpt-4o",
				ConcurrencyLimit:               2,
				QueueCapacity:                  3,
				BusinessCode:                   &businessCode,
				CreditBalanceConfigured:        true,
				CreditBalancePurchaseEnabled:   false,
				CreditBalanceRedemptionEnabled: false,
				CreditBalanceConversionEnabled: false,
			}).Error)
			now := common.GetTimestamp()
			require.NoError(t, DB.Create(&UserSubscription{
				Id:              subscriptionID,
				UserId:          userID,
				PlanId:          planID,
				EntitlementType: SubscriptionEntitlementCreditBalance,
				Status:          SubscriptionStatusActive,
				StartTime:       now - 60,
				EndTime:         0,
				TokenLimit:      100,
				TokenUsed:       20,
				GrantReason:     SubscriptionGrantOrder,
				Source:          SubscriptionGrantOrder,
			}).Error)

			firstRequestID := fmt.Sprintf("credit-lifecycle-first-%s", strategy)
			first, err := PreConsumeUserSubscription(firstRequestID, userID, "gpt-4o", 0, 7)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Equal(t, subscriptionID, first.UserSubscriptionId)
			assert.Equal(t, SubscriptionEntitlementCreditBalance, first.EntitlementType)
			assert.Zero(t, first.SubscriptionEndTime)
			assert.Equal(t, int64(27), first.TokenUsedAfter)

			require.NoError(t, RefundSubscriptionPreConsume(firstRequestID))
			require.NoError(t, RefundSubscriptionPreConsume(firstRequestID), "duplicate failure refunds must be idempotent")
			var afterRefund UserSubscription
			require.NoError(t, DB.First(&afterRefund, subscriptionID).Error)
			assert.Equal(t, int64(20), afterRefund.TokenUsed)

			secondRequestID := fmt.Sprintf("credit-lifecycle-cached-%s", strategy)
			second, err := PreConsumeUserSubscription(secondRequestID, userID, "gpt-4o", 0, 3)
			require.NoError(t, err)
			require.NotNil(t, second)
			assert.Equal(t, subscriptionID, second.UserSubscriptionId)
			assert.Equal(t, int64(23), second.TokenUsedAfter)
			require.NoError(t, RefundSubscriptionPreConsume(secondRequestID))
		})
	}
}

func TestScheduledSubscriptionLifecyclePreservesCreditAndConvertedHistory(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 24_101, Username: "scheduled-credit-lifecycle", Status: common.UserStatusEnabled}).Error)

	creditCode := "scheduled-credit-balance"
	timedCode := "scheduled-timed-control"
	require.NoError(t, DB.Create(&SubscriptionPlan{
		Id: 24_102, Title: "Credit balance", EntitlementType: SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true,
		QuotaResetPeriod: SubscriptionResetMonthly, MonthlyTokenLimit: 100,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{
		Id: 24_103, Title: "Timed control", EntitlementType: SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, PriceAmount: 10, Currency: "CNY",
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: SubscriptionResetMonthly, MonthlyTokenLimit: 100,
	}).Error)

	require.NoError(t, DB.Create(&[]UserSubscription{
		{
			Id: 24_104, UserId: 24_101, PlanId: 24_102,
			EntitlementType: SubscriptionEntitlementCreditBalance, Status: SubscriptionStatusActive,
			StartTime: now - 40*86400, EndTime: 0, TokenLimit: 100, TokenUsed: 40,
			LastResetTime: now - 31*86400, NextResetTime: now - 1,
			GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		},
		{
			Id: 24_105, UserId: 24_101, PlanId: 24_103,
			EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusConverted,
			StartTime: now - 40*86400, EndTime: now - 1, TokenLimit: 100, TokenUsed: 30,
			LastResetTime: now - 31*86400, NextResetTime: now - 1,
			GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		},
		{
			Id: 24_106, UserId: 24_101, PlanId: 24_103,
			EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive,
			StartTime: now - 40*86400, EndTime: now + 60*86400, TokenLimit: 100, TokenUsed: 70,
			LastResetTime: now - 31*86400, NextResetTime: now - 1,
			GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		},
		{
			Id: 24_107, UserId: 24_101, PlanId: 24_103,
			EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive,
			StartTime: now - 40*86400, EndTime: now - 1, TokenLimit: 100, TokenUsed: 80,
			GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		},
	}).Error)

	resetCount, err := ResetDueSubscriptions(100)
	require.NoError(t, err)
	assert.Equal(t, 1, resetCount, "scheduled reset must not count Credit or converted history")
	expiredCount, err := ExpireDueSubscriptions(100)
	require.NoError(t, err)
	assert.Equal(t, 1, expiredCount, "scheduled expiry must only expire the ordinary timed control")
	secondResetCount, err := ResetDueSubscriptions(100)
	require.NoError(t, err)
	assert.Zero(t, secondResetCount, "due Credit and converted history must not keep the reset loop alive")
	secondExpiredCount, err := ExpireDueSubscriptions(100)
	require.NoError(t, err)
	assert.Zero(t, secondExpiredCount, "Credit and converted history must not keep the expiry loop alive")

	var credit, converted, resetTimed, expiredTimed UserSubscription
	require.NoError(t, DB.First(&credit, 24_104).Error)
	require.NoError(t, DB.First(&converted, 24_105).Error)
	require.NoError(t, DB.First(&resetTimed, 24_106).Error)
	require.NoError(t, DB.First(&expiredTimed, 24_107).Error)

	assert.Equal(t, SubscriptionStatusActive, credit.Status)
	assert.Zero(t, credit.EndTime)
	assert.Equal(t, int64(40), credit.TokenUsed, "scheduled quota reset must never erase Credit balance")
	assert.Equal(t, SubscriptionStatusConverted, converted.Status)
	assert.Equal(t, int64(30), converted.TokenUsed)
	assert.Equal(t, int64(0), resetTimed.TokenUsed, "ordinary timed quota reset behavior must remain intact")
	assert.Greater(t, resetTimed.NextResetTime, now)
	assert.Equal(t, SubscriptionStatusExpired, expiredTimed.Status, "ordinary timed expiry behavior must remain intact")
}

func TestAdminOpsConcurrencyUsesTimedFirstRequestSelection(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	ClearPrimaryBillableSubscriptionCacheForTest()

	const userID, timedPlanID, creditPlanID, timedSubscriptionID, creditSubscriptionID = 24_201, 24_202, 24_203, 24_204, 24_205
	user := User{Id: userID, Username: "admin-ops-timed-first", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyTimedFirst
	setting.ActiveSubscriptionId = creditSubscriptionID
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)

	timedCode := "admin-ops-timed-first"
	creditCode := "admin-ops-active-credit"
	require.NoError(t, DB.Create(&SubscriptionPlan{
		Id: timedPlanID, Title: "Timed first", EntitlementType: SubscriptionEntitlementTimed,
		Enabled: true, BusinessCode: &timedCode, MonthlyTokenLimit: 100,
		ConcurrencyLimit: 3, QueueCapacity: 4,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{
		Id: creditPlanID, Title: "Active Credit", EntitlementType: SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true,
		ConcurrencyLimit: 8, QueueCapacity: 9,
	}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&[]UserSubscription{
		{
			Id: timedSubscriptionID, UserId: userID, PlanId: timedPlanID,
			EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive,
			StartTime: now - 60, EndTime: now + 3600, TokenLimit: 100, TokenUsed: 10,
			GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		},
		{
			Id: creditSubscriptionID, UserId: userID, PlanId: creditPlanID,
			EntitlementType: SubscriptionEntitlementCreditBalance, Status: SubscriptionStatusActive,
			StartTime: now - 60, EndTime: 0, TokenLimit: 100, TokenUsed: 20,
			GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		},
	}).Error)

	preConsumed, err := PreConsumeUserSubscription("admin-ops-timed-first-request", userID, "gpt-4o", 0, 1)
	require.NoError(t, err)
	require.NotNil(t, preConsumed)
	assert.Equal(t, timedSubscriptionID, preConsumed.UserSubscriptionId)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{userID})
	require.NoError(t, err)
	limit := limits[userID]
	assert.Equal(t, timedPlanID, limit.PlanID)
	assert.Equal(t, "Timed first", limit.PlanTitle)
	assert.Equal(t, int64(11), limit.TokenUsed)
	assert.Equal(t, 3, limit.Limit)
	assert.Equal(t, 4, limit.QueueCapacity)
	require.NoError(t, RefundSubscriptionPreConsume("admin-ops-timed-first-request"))
}
