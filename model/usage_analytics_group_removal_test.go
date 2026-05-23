package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAnalyticsRejectsBusinessGroupByAndIgnoresGroupsFilter(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 20, Type: LogTypeConsume, TokenId: 1, Group: "vip", MeteredTokens: intPtrForUsageAnalyticsTest(10)})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 2, Group: "default", MeteredTokens: intPtrForUsageAnalyticsTest(20)})

	_, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupBy("group"), Limit: 10})
	require.ErrorIs(t, err, ErrUsageAnalyticsInvalidGroup)

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 30, res.Total.TotalTokens)
	assert.Len(t, res.Groups, 2)
}

func TestUsageAnalyticsTokenInfoOmitsLegacyTokenGroup(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsToken(t, &Token{Id: 88, UserId: 101, Name: "live-key", Key: "sk-live-group-removed", Status: 1, RemainQuota: 100, Group: "vip"})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 88, TokenName: "historical", MeteredTokens: intPtrForUsageAnalyticsTest(10)})

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Len(t, res.Groups, 1)
	require.NotNil(t, res.Groups[0].Token)
	assert.NotContains(t, res.Groups[0].Token.Name, "vip")
}
