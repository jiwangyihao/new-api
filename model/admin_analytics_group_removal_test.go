package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminAnalyticsIgnoresBusinessGroupFiltersAndOmitsGroupDistributions(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	code := "admin-group-removal"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 301, Title: "Admin Group Removal", Enabled: true, BusinessCode: &code}).Error)
	require.NoError(t, DB.Create(&User{Id: 301, Username: "vip-user", Status: common.UserStatusEnabled, Group: "vip", AffCode: "vip-aff"}).Error)
	require.NoError(t, DB.Create(&User{Id: 302, Username: "default-user", Status: common.UserStatusEnabled, Group: "default", AffCode: "default-aff"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 301, UserId: 301, PlanId: 301, Status: "active", StartTime: now - 60, EndTime: now + 60, TokenLimit: 100, TokenUsed: 10, GrantReason: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 302, UserId: 302, PlanId: 301, Status: "active", StartTime: now - 60, EndTime: now + 60, TokenLimit: 100, TokenUsed: 20, GrantReason: "order"}).Error)

	query := AdminAnalyticsQuery{SnapshotAt: now, StartTimestamp: now - 120, EndTimestamp: now, Limit: 20, UserGroups: []string{"missing"}, RequestGroups: []string{"missing"}}
	lifecycle, err := GetAdminAnalyticsUserLifecycle(query)
	require.NoError(t, err)
	assert.Len(t, lifecycle.Data.Users.Items, 2)
	assert.Empty(t, lifecycle.Data.UserGroups)
	assert.Empty(t, lifecycle.Data.RequestGroups)
	for _, item := range lifecycle.Data.Users.Items {
		assert.Empty(t, item.UserGroup)
	}

	overview, err := GetAdminAnalyticsOverview(query)
	require.NoError(t, err)
	assert.Equal(t, 2, overview.Data.Summary.Subscriptions.ActiveCount)
}

func TestAdminUsageRejectsBusinessGroupByAndIgnoresRequestGroupFilter(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	require.NoError(t, LOG_DB.Create(&Log{UserId: 401, Username: "u1", CreatedAt: now - 20, Type: LogTypeConsume, Group: "vip", MeteredTokens: intPtrForAdminAnalyticsTest(10), Other: `{"request_group":"api"}`}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 402, Username: "u2", CreatedAt: now - 10, Type: LogTypeConsume, Group: "default", MeteredTokens: intPtrForAdminAnalyticsTest(20), Other: `{"request_group":"batch"}`}).Error)

	base := AdminAnalyticsQuery{StartTimestamp: now - 60, EndTimestamp: now, SnapshotAt: now, Limit: 20, RequestGroups: []string{"missing"}}
	_, err := GetAdminUsageConsumptionSummary(AdminAnalyticsUsageQuery{AdminAnalyticsQuery: base, GroupBy: dto.AdminUsageGroupBy("user_group"), Metric: dto.AdminUsageMetricTotalTokens})
	require.ErrorIs(t, err, ErrAdminAnalyticsInvalidGroupBy)
	_, err = GetAdminUsageConsumptionSummary(AdminAnalyticsUsageQuery{AdminAnalyticsQuery: base, GroupBy: dto.AdminUsageGroupBy("request_group"), Metric: dto.AdminUsageMetricTotalTokens})
	require.ErrorIs(t, err, ErrAdminAnalyticsInvalidGroupBy)

	res, err := GetAdminUsageConsumptionSummary(AdminAnalyticsUsageQuery{AdminAnalyticsQuery: base, GroupBy: dto.AdminUsageGroupByUser, Metric: dto.AdminUsageMetricTotalTokens})
	require.NoError(t, err)
	assert.Equal(t, int64(30), res.Data.Total.TotalTokens)
}
