package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestUsageAnalyticsRejectsBusinessGroupByAndIgnoresGroupsFilterAtAPI(t *testing.T) {
	setupUsageAnalyticsControllerTestDBs(t)
	now := usageAnalyticsControllerNow()
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 20, Type: model.LogTypeConsume, TokenId: 1, Group: "vip", MeteredTokens: intPtrForUsageAnalyticsControllerTest(10)})
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 2, Group: "default", MeteredTokens: intPtrForUsageAnalyticsControllerTest(20)})

	bad := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?group_by=group")
	require.Equal(t, http.StatusBadRequest, bad.Code)

	filtered := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?groups=missing")
	require.Equal(t, http.StatusOK, filtered.Code)
	require.Contains(t, filtered.Body.String(), `"total_tokens":30`)
}
