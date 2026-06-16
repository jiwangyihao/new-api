package service

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
)

func TestAdminOpsHealthFromReasons(t *testing.T) {
	healthy := buildAdminOpsHealth(nil)
	assert.Equal(t, dto.AdminOpsHealthStatusHealthy, healthy.Status)
	assert.Equal(t, 100, healthy.Score)
	assert.Empty(t, healthy.Reasons)

	degraded := buildAdminOpsHealth([]adminOpsHealthReason{
		{Code: "concurrency_queue_not_empty", Severity: adminOpsHealthSeverityDegraded},
		{Code: "channel_auto_disabled", Severity: adminOpsHealthSeverityDegraded},
	})
	assert.Equal(t, dto.AdminOpsHealthStatusDegraded, degraded.Status)
	assert.Equal(t, 80, degraded.Score)
	assert.Equal(t, []string{"concurrency_queue_not_empty", "channel_auto_disabled"}, degraded.Reasons)

	critical := buildAdminOpsHealth([]adminOpsHealthReason{
		{Code: "database_unhealthy", Severity: adminOpsHealthSeverityCritical},
		{Code: "concurrency_queue_not_empty", Severity: adminOpsHealthSeverityDegraded},
	})
	assert.Equal(t, dto.AdminOpsHealthStatusCritical, critical.Status)
	assert.Equal(t, 60, critical.Score)
	assert.Equal(t, []string{"database_unhealthy", "concurrency_queue_not_empty"}, critical.Reasons)
}

func TestAdminOpsConcurrencyHealthReasonsIgnoreUserQueuePressure(t *testing.T) {
	reasons := adminOpsConcurrencyHealthReasons(dto.AdminOpsConcurrencyCounters{QueueFullRejectionsTotal: 3})
	assert.Empty(t, reasons)

	criticalReasons := adminOpsConcurrencyHealthReasons(dto.AdminOpsConcurrencyCounters{
		RedisErrorsTotal:           1,
		UnavailableRejectionsTotal: 1,
	})
	assert.Equal(t, []adminOpsHealthReason{
		{Code: "concurrency_redis_errors", Severity: adminOpsHealthSeverityCritical},
		{Code: "concurrency_unavailable_rejections", Severity: adminOpsHealthSeverityCritical},
	}, criticalReasons)
}

func TestAdminOpsRecentErrorsAreMasked(t *testing.T) {
	raw := `Authorization: Bearer sk-secret-token prompt: "patient data should not leak" messages: [{"role":"user","content":"patient data should not leak"}] image: data:image/png;base64,aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`

	sanitized := sanitizeAdminOpsRecentErrorContent(raw)

	assert.NotContains(t, sanitized, "Authorization")
	assert.NotContains(t, sanitized, "Bearer")
	assert.NotContains(t, sanitized, "sk-secret-token")
	assert.NotContains(t, sanitized, "patient data should not leak")
	assert.NotContains(t, sanitized, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.LessOrEqual(t, utf8.RuneCountInString(sanitized), 300)
	jsonSanitized := sanitizeAdminOpsRecentErrorContent(`{"messages":[{"role":"user","content":"json patient data"}],"prompt":"json prompt data"}`)
	assert.NotContains(t, jsonSanitized, "json patient data")
	assert.NotContains(t, jsonSanitized, "json prompt data")
	authJSONSanitized := sanitizeAdminOpsRecentErrorContent(`{"Authorization":"Basic abc123","authorization":"Token secret"}`)
	assert.NotContains(t, authJSONSanitized, "abc123")
	assert.NotContains(t, authJSONSanitized, "secret")
}

func TestBuildAdminOpsConcurrencySummary(t *testing.T) {
	users := []dto.AdminOpsConcurrencyUser{
		{UserID: 1, Active: 2, Limit: 2, Queued: 1, QueueCapacity: 4},
		{UserID: 2, Active: 1, Limit: 4, Queued: 0, QueueCapacity: 4},
		{UserID: 3, Active: 0, Limit: 1, Queued: 3, QueueCapacity: 3},
	}

	summary := buildAdminOpsConcurrencySummary(users)

	assert.EqualValues(t, 3, summary.TotalActive)
	assert.EqualValues(t, 4, summary.TotalQueued)
	assert.EqualValues(t, 2, summary.ActiveUsers)
	assert.EqualValues(t, 2, summary.QueuedUsers)
	assert.EqualValues(t, 1, summary.SaturatedUsers)
	assert.InDelta(t, 1.0, summary.QueuePressure, 0.0001)

	reasons := adminOpsConcurrencyHealthReasons(dto.AdminOpsConcurrencyCounters{})
	assert.Empty(t, reasons)
}

func TestBuildAdminOpsConcurrencySummaryUsesAllUsersBeforeDetailLimit(t *testing.T) {
	users := []dto.AdminOpsConcurrencyUser{
		{UserID: 1, Active: 3, Limit: 3, Queued: 0, QueueCapacity: 5},
		{UserID: 2, Active: 0, Limit: 3, Queued: 5, QueueCapacity: 5},
		{UserID: 3, Active: 1, Limit: 3, Queued: 0, QueueCapacity: 5},
	}

	detail := limitAdminOpsConcurrencyUsers(users, 1)
	assert.Len(t, detail, 1)

	summary := buildAdminOpsConcurrencySummary(users)
	assert.EqualValues(t, 4, summary.TotalActive)
	assert.EqualValues(t, 5, summary.TotalQueued)
	assert.EqualValues(t, 2, summary.ActiveUsers)
	assert.EqualValues(t, 1, summary.QueuedUsers)
	assert.EqualValues(t, 1, summary.SaturatedUsers)
	assert.InDelta(t, 1.0, summary.QueuePressure, 0.0001)
}

func TestAdminOpsConcurrencySummaryUsesFilteredUsersBeforeDetailLimit(t *testing.T) {
	users := []dto.AdminOpsConcurrencyUser{
		{UserID: 1, PlanID: 10, Active: 3, Limit: 3, Queued: 0, QueueCapacity: 5, Status: "saturated"},
		{UserID: 2, PlanID: 11, Active: 0, Limit: 3, Queued: 5, QueueCapacity: 5, Status: "queued"},
		{UserID: 3, PlanID: 10, Active: 1, Limit: 3, Queued: 0, QueueCapacity: 5, Status: "normal"},
	}

	filtered := filterAdminOpsConcurrencyUsers(users, AdminOpsConcurrencyQuery{PlanID: 10, MinActiveOrQueued: 1})
	detail := limitAdminOpsConcurrencyUsers(filtered, 1)
	assert.Len(t, detail, 1)

	summary := buildAdminOpsConcurrencySummary(filtered)
	assert.EqualValues(t, 4, summary.TotalActive)
	assert.EqualValues(t, 0, summary.TotalQueued)
	assert.EqualValues(t, 2, summary.ActiveUsers)
	assert.EqualValues(t, 0, summary.QueuedUsers)
	assert.EqualValues(t, 1, summary.SaturatedUsers)
}

func TestGetAdminOpsConcurrencySummaryMatchesFilteredUsers(t *testing.T) {
	resetSubscriptionConcurrencyForTest(t)
	now := time.Now().Unix()
	subscriptionConcurrencyMemory.mu.Lock()
	subscriptionConcurrencyMemory.requests[1] = map[string]struct{}{"active-one": {}}
	subscriptionConcurrencyMemory.requests[2] = map[string]struct{}{"active-two": {}}
	subscriptionConcurrencyMemory.waiting[2] = []memorySubscriptionConcurrencyWaiter{{queuedAt: now - 10}}
	subscriptionConcurrencyMemory.mu.Unlock()

	response, err := GetAdminOpsConcurrency(context.Background(), AdminOpsConcurrencyQuery{Limit: 20, IncludeUsers: true, Status: "queued", MinActiveOrQueued: 1})

	assert.NoError(t, err)
	assert.Len(t, response.Users, 1)
	assert.Equal(t, 2, response.Users[0].UserID)
	assert.EqualValues(t, 1, response.Summary.TotalActive)
	assert.EqualValues(t, 1, response.Summary.TotalQueued)
	assert.EqualValues(t, 1, response.Summary.ActiveUsers)
	assert.EqualValues(t, 1, response.Summary.QueuedUsers)
}

func TestFilterAdminOpsConcurrencyUsersByPlanAndStatus(t *testing.T) {
	users := []dto.AdminOpsConcurrencyUser{
		{UserID: 1, PlanID: 10, PlanTitle: "Pro", Active: 2, Queued: 0, Limit: 2, QueueCapacity: 4, Status: "saturated"},
		{UserID: 2, PlanID: 11, PlanTitle: "Basic", Active: 1, Queued: 3, Limit: 5, QueueCapacity: 4, Status: "queued"},
		{UserID: 3, PlanID: 10, PlanTitle: "Pro", Active: 1, Queued: 0, Limit: 5, QueueCapacity: 4, Status: "normal"},
	}

	filtered := filterAdminOpsConcurrencyUsers(users, AdminOpsConcurrencyQuery{PlanID: 10, Status: "saturated", MinActiveOrQueued: 0})

	assert.Equal(t, []dto.AdminOpsConcurrencyUser{users[0]}, filtered)
}

func TestFillAdminOpsConcurrencyUserUsagePrefersTokenQuota(t *testing.T) {
	user := dto.AdminOpsConcurrencyUser{
		AmountTotal: 1000,
		AmountUsed:  250,
		TokenLimit:  500,
		TokenUsed:   125,
	}

	fillAdminOpsConcurrencyUserDerivedFields(&user)

	assert.InDelta(t, 0.25, user.Usage, 0.0001)
	assert.EqualValues(t, 500, user.UsageTotal)
	assert.EqualValues(t, 125, user.UsageUsed)
}
