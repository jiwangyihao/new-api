package service

import (
	"reflect"
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

func TestGetRankingsSnapshotFreeUserHistorySerializesEmptyPointsArray(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Empty(t, result.FreeUsers)
	require.Empty(t, result.FreeUserHistory.Points)
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	var payload struct {
		FreeUserHistory struct {
			Points []FreeUserHistoryPoint `json:"points"`
		} `json:"free_user_history"`
	}
	require.NoError(t, common.Unmarshal(encoded, &payload))
	assert.NotNil(t, payload.FreeUserHistory.Points)
	assert.Contains(t, string(encoded), `"points":[]`)
}

func TestRankingFreeUserInternalCandidatesDoNotExposeJSONTags(t *testing.T) {
	for _, field := range []struct {
		owner interface{}
		name  string
	}{
		{owner: model.RankingFreeUserSubscription{}, name: "ID"},
		{owner: model.RankingFreeUserSubscription{}, name: "UserID"},
		{owner: model.RankingFreeUserLogCandidate{}, name: "ID"},
		{owner: model.RankingFreeUserLogCandidate{}, name: "UserID"},
	} {
		structField, ok := reflect.TypeOf(field.owner).FieldByName(field.name)
		require.True(t, ok)
		assert.Empty(t, structField.Tag.Get("json"))
	}
}

func TestGetRankingsSnapshotBuildsFreeUserHistoryFromSubscriptionLogs(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	start := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC).Unix()
	freeCode := "history-free"
	paidCode := "history-paid"
	freePlan := &model.SubscriptionPlan{Id: 9971, Title: "History Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	paidPlan := &model.SubscriptionPlan{Id: 9972, Title: "History Paid", Enabled: true, PriceAmount: 10, ConcurrencyLimit: 1, BusinessCode: &paidCode}
	require.NoError(t, model.DB.Create(freePlan).Error)
	require.NoError(t, model.DB.Create(paidPlan).Error)

	user := model.User{Id: 9973, Username: "history-user", DisplayName: "Private Name", Status: common.UserStatusEnabled, AffCode: "aff9973"}
	user.SetSetting(dto.UserSetting{RankingsDisplayName: "Token Sprinter"})
	require.NoError(t, model.DB.Create(&user).Error)

	freeSub := model.UserSubscription{Id: 9974, UserId: user.Id, PlanId: freePlan.Id, Status: "active", TokenUsed: 300, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	paidSub := model.UserSubscription{Id: 9975, UserId: user.Id, PlanId: paidPlan.Id, Status: "active", TokenUsed: 999, StartTime: start, EndTime: start + 48*3600, GrantReason: "order"}
	require.NoError(t, model.DB.Create(&freeSub).Error)
	require.NoError(t, model.DB.Create(&paidSub).Error)

	seedRankingConsumeLog(t, 9976, user.Id, start+30*60, freeSub.Id, 100)
	seedRankingConsumeLog(t, 9977, user.Id, start+90*60, freeSub.Id, 200)
	seedRankingConsumeLog(t, 9978, user.Id, start+24*3600, freeSub.Id, 300)
	seedRankingConsumeLog(t, 9979, user.Id, start+45*60, paidSub.Id, 999)
	seedRankingConsumeLog(t, 9980, user.Id, start+60*60, 0, 777)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUsers, 1)
	require.Equal(t, 24, result.FreeUserHistory.Hours)
	require.Len(t, result.FreeUserHistory.Points, 24)

	assert.Equal(t, int64(100), result.FreeUserHistory.Points[0].Tokens)
	assert.Equal(t, int64(100), result.FreeUserHistory.Points[0].CumulativeTokens)
	assert.Equal(t, int64(200), result.FreeUserHistory.Points[1].Tokens)
	assert.Equal(t, int64(300), result.FreeUserHistory.Points[1].CumulativeTokens)
	assert.Equal(t, int64(0), result.FreeUserHistory.Points[2].Tokens)
	assert.Equal(t, int64(300), result.FreeUserHistory.Points[2].CumulativeTokens)
	assert.Equal(t, int64(300), result.FreeUserHistory.Points[23].CumulativeTokens)
	assert.Equal(t, "#1 · Token Sprinter", result.FreeUserHistory.Points[0].SeriesLabel)
}

func TestFreeUserHistoryUsesDerivedColumnsWithoutDoubleCountingOtherFallback(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	start := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC).Unix()
	freeCode := "derived-free"
	plan := &model.SubscriptionPlan{Id: 10011, Title: "Derived Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	user := model.User{Id: 10012, Username: "derived-user", Status: common.UserStatusEnabled, AffCode: "aff10012"}
	require.NoError(t, model.DB.Create(&user).Error)
	sub := model.UserSubscription{Id: 10013, UserId: user.Id, PlanId: plan.Id, Status: "active", TokenUsed: 600, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	require.NoError(t, model.DB.Create(&sub).Error)

	conflictingDerivedTokens := int64(100)
	conflictingMeteredTokens := 100
	derivedOnlyTokens := int64(200)
	derivedOnlyMeteredTokens := 200
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		Id:                         10014,
		UserId:                     user.Id,
		CreatedAt:                  start + 10*60,
		Type:                       model.LogTypeConsume,
		ModelName:                  "gpt-5.5",
		MeteredTokens:              &conflictingMeteredTokens,
		SubscriptionID:             &sub.Id,
		SubscriptionTokensConsumed: &conflictingDerivedTokens,
		Other: common.MapToJsonStr(map[string]interface{}{
			"subscription_id":              sub.Id,
			"subscription_tokens_consumed": 999,
		}),
	}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		Id:                         10015,
		UserId:                     user.Id,
		CreatedAt:                  start + 20*60,
		Type:                       model.LogTypeConsume,
		ModelName:                  "gpt-5.5",
		MeteredTokens:              &derivedOnlyMeteredTokens,
		SubscriptionID:             &sub.Id,
		SubscriptionTokensConsumed: &derivedOnlyTokens,
	}).Error)
	seedRankingConsumeLog(t, 10016, user.Id, start+30*60, sub.Id, 300)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUsers, 1)
	require.Len(t, result.FreeUserHistory.Points, 24)
	assert.Equal(t, int64(600), result.FreeUserHistory.Points[0].Tokens)
	assert.Equal(t, int64(600), result.FreeUserHistory.Points[0].CumulativeTokens)
}

func TestFreeUserHistoryUsesHourlyAggregateAndUnappliedFallback(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.LogAggregationEvent{}, &model.FreeSubscriptionUsageHourly{}))

	start := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC).Unix()
	freeCode := "aggregate-free"
	plan := &model.SubscriptionPlan{Id: 10111, Title: "Aggregate Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	user := model.User{Id: 10112, Username: "aggregate-user", Status: common.UserStatusEnabled, AffCode: "aff10112"}
	require.NoError(t, model.DB.Create(&user).Error)
	sub := model.UserSubscription{Id: 10113, UserId: user.Id, PlanId: plan.Id, Status: "active", TokenUsed: 400, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	require.NoError(t, model.DB.Create(&sub).Error)

	require.NoError(t, model.LOG_DB.Create(&model.FreeSubscriptionUsageHourly{SubscriptionID: sub.Id, UserID: user.Id, HourIndex: 0, Tokens: 100}).Error)
	seedRankingConsumeLog(t, 10114, user.Id, start+10*60, sub.Id, 100)
	require.NoError(t, model.LOG_DB.Create(&model.LogAggregationEvent{LogID: 10114, AggregateName: "free_subscription_usage_hourly", Status: "applied"}).Error)
	seedRankingConsumeLog(t, 10115, user.Id, start+20*60, sub.Id, 50)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUsers, 1)
	require.Len(t, result.FreeUserHistory.Points, 24)
	assert.Equal(t, int64(150), result.FreeUserHistory.Points[0].Tokens)
	assert.Equal(t, int64(150), result.FreeUserHistory.Points[0].CumulativeTokens)
}

func TestGetRankingsSnapshotFreeUserHistoryDoesNotDuplicateOverlappingSubscriptions(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	start := time.Date(2026, 5, 20, 8, 15, 0, 0, time.UTC).Unix()
	freeCode := "overlap-free"
	plan := &model.SubscriptionPlan{Id: 9981, Title: "Overlap Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	user := model.User{Id: 9982, Username: "overlap-user", DisplayName: "Hidden Overlap", Status: common.UserStatusEnabled, AffCode: "aff9982"}
	require.NoError(t, model.DB.Create(&user).Error)

	first := model.UserSubscription{Id: 9983, UserId: user.Id, PlanId: plan.Id, Status: "active", TokenUsed: 100, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	second := model.UserSubscription{Id: 9984, UserId: user.Id, PlanId: plan.Id, Status: "active", TokenUsed: 500, StartTime: start + 30*60, EndTime: start + 48*3600, GrantReason: "trial_code"}
	require.NoError(t, model.DB.Create(&first).Error)
	require.NoError(t, model.DB.Create(&second).Error)
	seedRankingConsumeLog(t, 9985, user.Id, start+45*60, second.Id, 500)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUserHistory.Points, 24)
	assert.Equal(t, int64(500), result.FreeUserHistory.Points[0].Tokens)
	assert.Equal(t, int64(500), result.FreeUserHistory.Points[0].CumulativeTokens)
	assert.Equal(t, int64(500), result.FreeUserHistory.Points[23].CumulativeTokens)
}

func TestGetRankingsSnapshotFreeUserHistoryIgnoresNonPositiveAndDeletedUsers(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	start := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC).Unix()
	freeCode := "ignore-free"
	plan := &model.SubscriptionPlan{Id: 9986, Title: "Ignore Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	rewardPlan := &model.SubscriptionPlan{Id: 9985, Title: "Reward Ignore", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1}
	require.NoError(t, model.DB.Create(rewardPlan).Error)
	require.NoError(t, model.DB.Create(plan).Error)
	active := model.User{Id: 9987, Username: "active-history", Status: common.UserStatusEnabled, AffCode: "aff9987"}
	deleted := model.User{Id: 9988, Username: "deleted-history", Status: common.UserStatusEnabled, AffCode: "aff9988"}
	require.NoError(t, model.DB.Create(&active).Error)
	require.NoError(t, model.DB.Create(&deleted).Error)
	activeSub := model.UserSubscription{Id: 9989, UserId: active.Id, PlanId: plan.Id, Status: "active", TokenUsed: 50, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	deletedSub := model.UserSubscription{Id: 9990, UserId: deleted.Id, PlanId: plan.Id, Status: "active", TokenUsed: 500, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	require.NoError(t, model.DB.Create(&activeSub).Error)
	require.NoError(t, model.DB.Create(&deletedSub).Error)
	require.NoError(t, deleted.Delete())
	seedRankingConsumeLogWithMetered(t, 9991, active.Id, start+60, activeSub.Id, 1, 0, true)
	seedRankingConsumeLogWithMetered(t, 9992, active.Id, start+120, activeSub.Id, 1, -10, true)
	seedRankingConsumeLogWithMetered(t, 9993, active.Id, start+150, activeSub.Id, 1, 999, false)
	seedRankingConsumeLog(t, 9994, active.Id, start+180, activeSub.Id, 50)
	seedRankingConsumeLog(t, 9995, deleted.Id, start+180, deletedSub.Id, 500)
	rewardSub := model.UserSubscription{Id: 9996, UserId: active.Id, PlanId: rewardPlan.Id, Status: "active", TokenUsed: 700, StartTime: start, EndTime: start + 48*3600, GrantReason: model.SubscriptionGrantMonthlyInviteEntitlement}
	require.NoError(t, model.DB.Create(&rewardSub).Error)
	seedRankingConsumeLog(t, 9997, active.Id, start+240, rewardSub.Id, 700)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUsers, 1)
	require.Len(t, result.FreeUserHistory.Points, 24)
	assert.Equal(t, int64(50), result.FreeUserHistory.Points[0].Tokens)
	assert.Equal(t, int64(50), result.FreeUserHistory.Points[23].CumulativeTokens)
}

func TestGetRankingsSnapshotFreeUserHistoryResponseHidesAccountIdentifiers(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	start := time.Date(2026, 5, 20, 7, 0, 0, 0, time.UTC).Unix()
	freeCode := "privacy-free"
	plan := &model.SubscriptionPlan{Id: 9995, Title: "Privacy Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	user := model.User{Id: 9996, Username: "private-user", DisplayName: "Private Display", Email: "private@example.com", Status: common.UserStatusEnabled, AffCode: "aff9996"}
	require.NoError(t, model.DB.Create(&user).Error)
	sub := model.UserSubscription{Id: 9997, UserId: user.Id, PlanId: plan.Id, Status: "active", TokenUsed: 100, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	require.NoError(t, model.DB.Create(&sub).Error)
	seedRankingConsumeLog(t, 9998, user.Id, start+60, sub.Id, 100)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	body := string(encoded)
	assert.NotContains(t, body, "user_id")
	assert.NotContains(t, body, "subscription_id")
	assert.NotContains(t, body, "private-user")
	assert.NotContains(t, body, "private@example.com")
	assert.NotContains(t, body, "Private Display")
	assert.Contains(t, body, "Explorer #1")
	assert.Contains(t, body, "#1 · Explorer #1")
}

func seedRankingConsumeLog(t *testing.T, id int, userID int, createdAt int64, subscriptionID int, subscriptionTokens int) {
	t.Helper()
	seedRankingConsumeLogWithMetered(t, id, userID, createdAt, subscriptionID, subscriptionTokens, subscriptionTokens, subscriptionTokens != 0)
}

func seedRankingConsumeLogWithMetered(t *testing.T, id int, userID int, createdAt int64, subscriptionID int, meteredTokens int, subscriptionTokens int, includeSubscriptionTokens bool) {
	t.Helper()
	other := map[string]interface{}{}
	if subscriptionID > 0 {
		other["subscription_id"] = subscriptionID
	}
	if includeSubscriptionTokens {
		other["subscription_tokens_consumed"] = subscriptionTokens
	}
	otherStr := common.MapToJsonStr(other)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		Id:            id,
		UserId:        userID,
		CreatedAt:     createdAt,
		Type:          model.LogTypeConsume,
		ModelName:     "gpt-5.5",
		MeteredTokens: &meteredTokens,
		Other:         otherStr,
	}).Error)
}
