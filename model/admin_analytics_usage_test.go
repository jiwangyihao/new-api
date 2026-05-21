package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func intPtrForAdminAnalyticsTest(value int) *int { return &value }

func TestAdminAnalyticsUsageUsesSeparatedDBAndLogDB(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	code := "basic"
	require.NoError(t, DB.Create(&User{Id: 101, Username: "usage-user", Status: common.UserStatusEnabled, Group: "vip", AffCode: "aff-usage"}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 201, Title: "Basic", Enabled: true, BusinessCode: &code}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 301, UserId: 101, PlanId: 201, Status: "active", StartTime: now - 100, EndTime: now + 100, TokenLimit: 1000, TokenUsed: 10, GrantReason: "order"}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 101, Username: "usage-user", CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 401, TokenName: "key", ModelName: "gpt", MeteredTokens: intPtrForAdminAnalyticsTest(10), Other: `{"subscription_id":301}`}).Error)

	res, err := GetAdminUsageConsumptionSummary(AdminAnalyticsUsageQuery{AdminAnalyticsQuery: AdminAnalyticsQuery{StartTimestamp: now - 60, EndTimestamp: now, SnapshotAt: now, Limit: 20}, GroupBy: dto.AdminUsageGroupByPlan, Metric: dto.AdminUsageMetricTotalTokens})
	require.NoError(t, err)
	require.Len(t, res.Data.Groups.Items, 1)
	require.Equal(t, "Basic", res.Data.Groups.Items[0].GroupLabel)
}

func TestAdminAnalyticsUsageUsesMeteredTokensFallbackAndExplicitZero(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "u", CreatedAt: now - 10, Type: LogTypeConsume, MeteredTokens: intPtrForAdminAnalyticsTest(80), PromptTokens: 1, CompletionTokens: 1}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "u", CreatedAt: now - 9, Type: LogTypeConsume, PromptTokens: 5, CompletionTokens: 6}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "u", CreatedAt: now - 8, Type: LogTypeConsume, MeteredTokens: intPtrForAdminAnalyticsTest(0), PromptTokens: 100, CompletionTokens: 100}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "u", CreatedAt: now - 7, Type: LogTypeError, PromptTokens: 999, CompletionTokens: 999, Quota: 999}).Error)

	res, err := GetAdminUsageConsumptionSummary(AdminAnalyticsUsageQuery{AdminAnalyticsQuery: AdminAnalyticsQuery{StartTimestamp: now - 60, EndTimestamp: now, SnapshotAt: now, Limit: 20}, GroupBy: dto.AdminUsageGroupByUser, Metric: dto.AdminUsageMetricTotalTokens})
	require.NoError(t, err)
	require.Equal(t, 91, int(res.Data.Total.TotalTokens))
	require.Equal(t, 4, res.Data.Total.RequestCount)
	require.Equal(t, 1, res.Data.Total.ErrorCount)
}

func TestAdminAnalyticsUsageParsesOtherBillingFieldsAndExplicitSubscriptionTokens(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "u", CreatedAt: now - 10, Type: LogTypeConsume, MeteredTokens: intPtrForAdminAnalyticsTest(80), PromptTokens: 100, CompletionTokens: 100, Other: `{"billing_source":"subscription","subscription_tokens_consumed":0,"endpoint":"/v1/chat/completions","request_group":"api"}`}).Error)

	res, err := GetAdminUsageConsumptionSummary(AdminAnalyticsUsageQuery{AdminAnalyticsQuery: AdminAnalyticsQuery{StartTimestamp: now - 60, EndTimestamp: now, SnapshotAt: now, Limit: 20}, GroupBy: dto.AdminUsageGroupByBillingSource, Metric: dto.AdminUsageMetricTotalTokens})
	require.NoError(t, err)
	require.Equal(t, int64(0), res.Data.Total.TotalTokens)
	require.Len(t, res.Data.Groups.Items, 1)
	require.Equal(t, "subscription", res.Data.Groups.Items[0].GroupValue)
}

func TestAdminAnalyticsUsageEventTimeAttributionUsesSubscriptionIDFromOther(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	code := "event"
	require.NoError(t, DB.Create(&User{Id: 1, Username: "u", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-event"}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 2, Title: "Event Plan", Enabled: true, BusinessCode: &code}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 3, UserId: 1, PlanId: 2, Status: "expired", StartTime: now - 1000, EndTime: now - 100, GrantReason: "order"}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "u", CreatedAt: now - 500, Type: LogTypeConsume, MeteredTokens: intPtrForAdminAnalyticsTest(10), Other: `{"subscription_id":3,"subscription_plan_id":999}`}).Error)

	res, err := GetAdminUsageConsumptionSummary(AdminAnalyticsUsageQuery{AdminAnalyticsQuery: AdminAnalyticsQuery{StartTimestamp: now - 600, EndTimestamp: now - 400, SnapshotAt: now, Limit: 20}, GroupBy: dto.AdminUsageGroupByPlan, Metric: dto.AdminUsageMetricTotalTokens, PlanAttribution: dto.AdminPlanAttributionEventTime})
	require.NoError(t, err)
	require.Len(t, res.Data.Groups.Items, 1)
	require.Equal(t, "Event Plan", res.Data.Groups.Items[0].GroupLabel)
}

func TestAdminAnalyticsUsageEventTimeAttributionAmbiguousHistoryReturnsUnknownWarning(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&User{Id: 1, Username: "u", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-amb"}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 2, Title: "A", Enabled: true}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 3, Title: "B", Enabled: true}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 4, UserId: 1, PlanId: 2, Status: "active", StartTime: now - 100, EndTime: now + 100, GrantReason: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 5, UserId: 1, PlanId: 3, Status: "active", StartTime: now - 100, EndTime: now + 100, GrantReason: "order"}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "u", CreatedAt: now, Type: LogTypeConsume, MeteredTokens: intPtrForAdminAnalyticsTest(10)}).Error)

	res, err := GetAdminUsageConsumptionSummary(AdminAnalyticsUsageQuery{AdminAnalyticsQuery: AdminAnalyticsQuery{StartTimestamp: now - 1, EndTimestamp: now + 1, SnapshotAt: now, Limit: 20}, GroupBy: dto.AdminUsageGroupByPlan, Metric: dto.AdminUsageMetricTotalTokens, PlanAttribution: dto.AdminPlanAttributionEventTime})
	require.NoError(t, err)
	require.NotEmpty(t, res.Warnings)
	require.Equal(t, "plan:unknown", res.Data.Groups.Items[0].GroupKey)
}

func TestAdminAnalyticsUsageQueryValidatesGroupByMetricAttributionAndTopN(t *testing.T) {
	require.ErrorIs(t, ValidateAdminUsageQuery(AdminAnalyticsUsageQuery{GroupBy: "bad", Metric: dto.AdminUsageMetricTotalTokens, PlanAttribution: dto.AdminPlanAttributionCurrent}, "summary"), ErrAdminAnalyticsInvalidGroupBy)
	require.ErrorIs(t, ValidateAdminUsageQuery(AdminAnalyticsUsageQuery{GroupBy: dto.AdminUsageGroupByUser, Metric: "bad", PlanAttribution: dto.AdminPlanAttributionCurrent}, "summary"), ErrAdminAnalyticsInvalidMetric)
	require.Error(t, ValidateAdminUsageQuery(AdminAnalyticsUsageQuery{GroupBy: dto.AdminUsageGroupByUser, Metric: dto.AdminUsageMetricTotalTokens, PlanAttribution: "bad"}, "summary"))
	require.ErrorIs(t, ValidateAdminUsageQuery(AdminAnalyticsUsageQuery{GroupBy: dto.AdminUsageGroupByUser, Metric: dto.AdminUsageMetricTotalTokens, PlanAttribution: dto.AdminPlanAttributionCurrent, SortByProvided: true}, "timeseries"), ErrAdminAnalyticsInvalidSortBy)
}

func TestAdminAnalyticsUsageLogDBBooleanAndDialectAreIndependentFromBusinessDB(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	stream := true
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "u", CreatedAt: now, Type: LogTypeConsume, IsStream: stream, MeteredTokens: intPtrForAdminAnalyticsTest(1)}).Error)
	res, err := GetAdminUsageConsumptionSummary(AdminAnalyticsUsageQuery{AdminAnalyticsQuery: AdminAnalyticsQuery{StartTimestamp: now - 1, EndTimestamp: now + 1, SnapshotAt: now, Limit: 20}, GroupBy: dto.AdminUsageGroupByStream, Metric: dto.AdminUsageMetricRequestCount})
	require.NoError(t, err)
	require.Len(t, res.Data.Groups.Items, 1)
	require.Equal(t, "true", res.Data.Groups.Items[0].GroupValue)
}
