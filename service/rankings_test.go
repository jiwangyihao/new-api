package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRankingsSnapshotRanksFreeSubscriptionTokenUsage(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	now := common.GetTimestamp()
	freeCode := "free-trial-rank"
	paidCode := "paid-rank"
	freePlan := &model.SubscriptionPlan{Id: 9911, Title: "Free Trial", Enabled: true, PriceAmount: 0, MonthlyTokenLimit: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	paidPlan := &model.SubscriptionPlan{Id: 9912, Title: "Paid", Enabled: true, PriceAmount: 20, MonthlyTokenLimit: 100000, ConcurrencyLimit: 1, BusinessCode: &paidCode}
	rewardPlan := &model.SubscriptionPlan{Id: 9913, Title: "Reward", Enabled: true, PriceAmount: 0, MonthlyTokenLimit: 100000, ConcurrencyLimit: 1}
	rewardPlan.BusinessCode = nil
	require.NoError(t, model.DB.Create(freePlan).Error)
	require.NoError(t, model.DB.Create(paidPlan).Error)
	require.NoError(t, model.DB.Create(rewardPlan).Error)

	anonymousTop := model.User{Id: 9921, Username: "top-user", DisplayName: "Should Hide", Status: common.UserStatusEnabled, AffCode: "aff9921"}
	anonymousTop.SetSetting(dto.UserSetting{})
	named := model.User{Id: 9922, Username: "named-user", DisplayName: "Display Name", Status: common.UserStatusEnabled, AffCode: "aff9922"}
	named.SetSetting(dto.UserSetting{RankingsDisplayName: "Token Wizard"})
	zeroNamed := model.User{Id: 9923, Username: "zero-user", Status: common.UserStatusEnabled, AffCode: "aff9923"}
	zeroNamed.SetSetting(dto.UserSetting{RankingsDisplayName: "Unused"})
	require.NoError(t, model.DB.Create(&anonymousTop).Error)
	require.NoError(t, model.DB.Create(&named).Error)
	require.NoError(t, model.DB.Create(&zeroNamed).Error)

	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9931, UserId: anonymousTop.Id, PlanId: freePlan.Id, Status: "expired", TokenLimit: 0, TokenUsed: 1200, StartTime: now - 7200, EndTime: now - 3600, GrantReason: "trial_code", Source: "trial_code"}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9932, UserId: named.Id, PlanId: freePlan.Id, Status: "active", TokenLimit: 0, TokenUsed: 800, StartTime: now - 3600, EndTime: now + 3600, GrantReason: "invite_trial", Source: "invite_trial"}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9933, UserId: named.Id, PlanId: paidPlan.Id, Status: "active", TokenLimit: 100000, TokenUsed: 9999, StartTime: now - 3600, EndTime: now + 3600, GrantReason: "order", Source: "order"}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9934, UserId: zeroNamed.Id, PlanId: freePlan.Id, Status: "expired", TokenLimit: 0, TokenUsed: 0, StartTime: now - 3600, EndTime: now - 1800, GrantReason: "trial_code", Source: "trial_code"}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9935, UserId: anonymousTop.Id, PlanId: rewardPlan.Id, Status: "active", TokenLimit: 100000, TokenUsed: 5000, StartTime: now - 3600, EndTime: now + 3600, GrantReason: model.SubscriptionGrantMonthlyInviteEntitlement, Source: model.SubscriptionGrantMonthlyInviteEntitlement}).Error)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUsers, 2)
	assert.Equal(t, int64(2000), result.FreeUserTotalTokens)

	assert.Equal(t, 1, result.FreeUsers[0].Rank)
	assert.Equal(t, anonymousRankingsDisplayName(1), result.FreeUsers[0].DisplayName)
	assert.Equal(t, int64(1200), result.FreeUsers[0].TotalTokens)
	assert.False(t, result.FreeUsers[0].Named)

	assert.Equal(t, 2, result.FreeUsers[1].Rank)
	assert.Equal(t, "Token Wizard", result.FreeUsers[1].DisplayName)
	assert.Equal(t, int64(800), result.FreeUsers[1].TotalTokens)
	assert.True(t, result.FreeUsers[1].Named)
}

func TestGetRankingsSnapshotFreeSubscriptionLeaderboardIsLifetime(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	now := time.Now()
	freeCode := "period-free"
	plan := &model.SubscriptionPlan{Id: 9941, Title: "Period Free", Enabled: true, PriceAmount: 0, MonthlyTokenLimit: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9942, Username: "period-current", Status: common.UserStatusEnabled, AffCode: "aff9942"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9943, Username: "period-old", Status: common.UserStatusEnabled, AffCode: "aff9943"}).Error)

	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9944, UserId: 9942, PlanId: plan.Id, Status: "active", TokenUsed: 100, StartTime: now.Add(-2 * time.Hour).Unix(), EndTime: now.Add(2 * time.Hour).Unix(), GrantReason: "trial_code"}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9945, UserId: 9943, PlanId: plan.Id, Status: "expired", TokenUsed: 900, StartTime: now.Add(-10 * 24 * time.Hour).Unix(), EndTime: now.Add(-9 * 24 * time.Hour).Unix(), GrantReason: "trial_code"}).Error)

	result, err := GetRankingsSnapshot("today")
	require.NoError(t, err)
	require.Len(t, result.FreeUsers, 2)
	assert.Equal(t, int64(900), result.FreeUsers[0].TotalTokens)
	assert.Equal(t, int64(100), result.FreeUsers[1].TotalTokens)
}

func TestGetRankingsSnapshotExcludesDeletedFreeUsers(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	now := common.GetTimestamp()
	freeCode := "deleted-free-rank"
	plan := &model.SubscriptionPlan{Id: 9951, Title: "Deleted Free", Enabled: true, PriceAmount: 0, MonthlyTokenLimit: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	visible := model.User{Id: 9952, Username: "visible-rank", Status: common.UserStatusEnabled, AffCode: "aff9952"}
	visible.SetSetting(dto.UserSetting{RankingsDisplayName: "Visible"})
	deleted := model.User{Id: 9953, Username: "deleted-rank", Status: common.UserStatusEnabled, AffCode: "aff9953"}
	deleted.SetSetting(dto.UserSetting{RankingsDisplayName: "Deleted Name"})
	require.NoError(t, model.DB.Create(&visible).Error)
	require.NoError(t, model.DB.Create(&deleted).Error)
	require.NoError(t, deleted.Delete())
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9954, UserId: visible.Id, PlanId: plan.Id, Status: "active", TokenUsed: 100, StartTime: now - 3600, EndTime: now + 3600, GrantReason: "trial_code"}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9955, UserId: deleted.Id, PlanId: plan.Id, Status: "expired", TokenUsed: 900, StartTime: now - 7200, EndTime: now - 3600, GrantReason: "trial_code"}).Error)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUsers, 1)
	assert.Equal(t, "Visible", result.FreeUsers[0].DisplayName)
	assert.Equal(t, int64(100), result.FreeUserTotalTokens)
}
