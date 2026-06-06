package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAdminAnalyticsControllerTestDBs(t *testing.T) *gorm.DB {
	t.Helper()

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.InvitationMonthlyEntitlement{}, &model.InvitationRewardEvent{}))
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	return db
}

func performAdminAnalyticsRequest(t *testing.T, rawURL string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)
	ctx.Set("id", 1)
	handler(ctx)
	return recorder
}

func newAdminAnalyticsParserContext(t *testing.T, rawURL string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)
	return ctx, recorder
}

func TestPaidSubscriptionValueParserDefaultsToSnapshotRangeWithoutThirtyDayWindow(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/paid-subscription-value/summary?snapshot_at=123&start_timestamp=999&end_timestamp=1")

	query, err := parseAdminPaidSubscriptionValueQuery(ctx)

	require.NoError(t, err)
	require.Equal(t, model.AdminAnalyticsRangeModeSnapshot, query.RangeMode)
	require.Equal(t, int64(0), query.StartTimestamp)
	require.Equal(t, int64(123), query.EndTimestamp)
	require.Equal(t, int64(123), query.SnapshotAt)
	require.False(t, query.TimeRangeExplicit)
	require.Equal(t, dto.AdminAnalyticsExcludedModeIncludedOnly, query.ExcludedMode)
	require.False(t, query.ActiveOnly)
}

func TestInvitationPaidParserAllowsAllHistoryWithoutRangeLimit(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/invitation-paid-subscriptions/summary?snapshot_at=456")

	query, err := parseAdminInvitationPaidSubscriptionsQuery(ctx)

	require.NoError(t, err)
	require.Equal(t, model.AdminAnalyticsRangeModeAllHistory, query.RangeMode)
	require.Equal(t, int64(0), query.StartTimestamp)
	require.Equal(t, int64(456), query.EndTimestamp)
	require.Equal(t, int64(456), query.SnapshotAt)
	require.False(t, query.TimeRangeExplicit)
	require.Equal(t, dto.AdminAnalyticsExcludedModeIncludedOnly, query.ExcludedMode)
	require.False(t, query.ActiveOnly)
}

func TestInvitationPaidParserParsesExplicitRange(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/invitation-paid-subscriptions/summary?snapshot_at=456&start_timestamp=100&end_timestamp=200")

	query, err := parseAdminInvitationPaidSubscriptionsQuery(ctx)

	require.NoError(t, err)
	require.Equal(t, model.AdminAnalyticsRangeModeAllHistory, query.RangeMode)
	require.Equal(t, int64(100), query.StartTimestamp)
	require.Equal(t, int64(200), query.EndTimestamp)
	require.Equal(t, int64(456), query.SnapshotAt)
	require.True(t, query.TimeRangeExplicit)
}

func TestPaidSubscriptionMoneySortRequiresCurrency(t *testing.T) {
	ctx, recorder := newAdminAnalyticsParserContext(t, "/api/admin-analytics/paid-subscription-value/users")
	query := model.AdminAnalyticsQuery{SortBy: "recognized_remaining_value"}

	_, ok := normalizeAdminAnalyticsMoneySortByOrAbort(ctx, query, map[string]string{"recognized_remaining_value": "recognized_remaining_value"}, map[string]struct{}{"recognized_remaining_value": {}})

	require.False(t, ok)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "currency")
}

func TestPaidSubscriptionNonMoneySortDoesNotRequireCurrency(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/paid-subscription-value/users")
	query := model.AdminAnalyticsQuery{SortBy: "user_id"}

	normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(ctx, query, map[string]string{"user_id": "user_id"}, map[string]struct{}{"recognized_remaining_value": {}})

	require.True(t, ok)
	require.Equal(t, "user_id", normalized.SortBy)
}

func TestAdminAnalyticsRejectsInvalidSnapshotAt(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/paid-subscription-value/summary?snapshot_at=-1")

	_, err := parseAdminPaidSubscriptionValueQuery(ctx)

	require.Error(t, err)
	require.Contains(t, err.Error(), "snapshot_at")
}

func TestAdminAnalyticsAcceptsExplicitZeroSnapshotAt(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/paid-subscription-value/summary?snapshot_at=0")

	query, err := parseAdminPaidSubscriptionValueQuery(ctx)

	require.NoError(t, err)
	require.Equal(t, int64(0), query.SnapshotAt)
	require.Equal(t, int64(0), query.EndTimestamp)
	require.Equal(t, int64(0), query.StartTimestamp)
}

func TestAdminAnalyticsOverviewParserStillDefaultsToThirtyDays(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/overview")

	query, err := parseAdminAnalyticsQuery(ctx)

	require.NoError(t, err)
	require.Equal(t, int64(30*24*60*60), query.EndTimestamp-query.StartTimestamp)
	require.Equal(t, model.AdminAnalyticsRangeModeDefault, query.RangeMode)
	require.False(t, query.TimeRangeExplicit)
}

func TestAdminAnalyticsParserDefaultsToConservativeLimit(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/overview")

	query, err := parseAdminAnalyticsQuery(ctx)

	require.NoError(t, err)
	require.Equal(t, model.AdminAnalyticsDefaultLimit, query.Limit)
}

func TestAdminAnalyticsParserAcceptsNoLimit(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/overview?limit=0")

	query, err := parseAdminAnalyticsQuery(ctx)

	require.NoError(t, err)
	require.Equal(t, model.AdminAnalyticsNoLimit, query.Limit)
}

func TestPaidSubscriptionValueParserParsesSharedFilters(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/paid-subscription-value/users?snapshot_at=1000&plan_ids=2&plan_ids=1&user_ids=4&user_ids=3&sources=order&sources=admin&grant_reasons=order&grant_reasons=redemption&business_codes=pro&business_codes=team&currency=CNY&excluded_mode=include_excluded&active_only=true&subscription_id=11&inviter_id=5&invitee_id=6&limit=50&offset=7&sort_order=asc")

	query, err := parseAdminPaidSubscriptionValueQuery(ctx)

	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, query.PlanIDs)
	require.Equal(t, []int{3, 4}, query.UserIDs)
	require.Equal(t, []dto.AdminAnalyticsSource{dto.AdminAnalyticsSourceAdmin, dto.AdminAnalyticsSourceOrder}, query.Sources)
	require.Equal(t, []string{"order", "redemption"}, query.GrantReasons)
	require.Equal(t, []string{"pro", "team"}, query.BusinessCodes)
	require.Equal(t, "CNY", query.Currency)
	require.Equal(t, dto.AdminAnalyticsExcludedModeIncludeExcluded, query.ExcludedMode)
	require.True(t, query.ActiveOnly)
	require.Equal(t, 11, query.SubscriptionID)
	require.Equal(t, 5, query.InviterID)
	require.Equal(t, 6, query.InviteeID)
	require.Equal(t, 50, query.Limit)
	require.Equal(t, 7, query.Offset)
	require.Equal(t, dto.AdminAnalyticsSortAsc, query.SortOrder)
}

func TestInvitationPaidParserParsesSharedFilters(t *testing.T) {
	ctx, _ := newAdminAnalyticsParserContext(t, "/api/admin-analytics/invitation-paid-subscriptions/invitees?snapshot_at=1000&plan_ids=2&plan_ids=1&user_ids=4&user_ids=3&sources=order&sources=admin&grant_reasons=order&grant_reasons=redemption&business_codes=pro&business_codes=team&inviter_id=5&invitee_id=6&active_only=true&subscription_id=11&currency=USD&excluded_mode=excluded_only&limit=50&offset=7&sort_order=asc")

	query, err := parseAdminInvitationPaidSubscriptionsQuery(ctx)

	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, query.PlanIDs)
	require.Equal(t, []int{3, 4}, query.UserIDs)
	require.Equal(t, []dto.AdminAnalyticsSource{dto.AdminAnalyticsSourceAdmin, dto.AdminAnalyticsSourceOrder}, query.Sources)
	require.Equal(t, []string{"order", "redemption"}, query.GrantReasons)
	require.Equal(t, []string{"pro", "team"}, query.BusinessCodes)
	require.Equal(t, 5, query.InviterID)
	require.Equal(t, 6, query.InviteeID)
	require.True(t, query.ActiveOnly)
	require.Equal(t, 11, query.SubscriptionID)
	require.Equal(t, "USD", query.Currency)
	require.Equal(t, dto.AdminAnalyticsExcludedModeExcludedOnly, query.ExcludedMode)
	require.Equal(t, 50, query.Limit)
	require.Equal(t, 7, query.Offset)
	require.Equal(t, dto.AdminAnalyticsSortAsc, query.SortOrder)

}

type adminAnalyticsPanelEnvelopeResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Range struct {
			StartTimestamp int64 `json:"start_timestamp"`
			EndTimestamp   int64 `json:"end_timestamp"`
			SnapshotAt     int64 `json:"snapshot_at"`
		} `json:"range"`
		Data struct {
			Summary map[string]any `json:"summary"`
			Users   struct {
				Items []map[string]any `json:"items"`
			} `json:"users"`
			Subscriptions struct {
				Items []map[string]any `json:"items"`
			} `json:"subscriptions"`
			Plans struct {
				Items []map[string]any `json:"items"`
			} `json:"plans"`
			Sources struct {
				Items []map[string]any `json:"items"`
			} `json:"sources"`
			Inviters struct {
				Items []map[string]any `json:"items"`
			} `json:"inviters"`
			Invitees struct {
				Items []map[string]any `json:"items"`
			} `json:"invitees"`
		} `json:"data"`
	} `json:"data"`
}

func TestPaidSubscriptionValueEndpointsReturnPanelEnvelope(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	now := int64(1767225600)
	seedAdminPaidSubscriptionControllerData(t, now)

	endpoints := []struct {
		path         string
		handler      gin.HandlerFunc
		field        string
		expectedItem int
	}{
		{path: "/api/admin-analytics/paid-subscription-value/summary?snapshot_at=0&subscription_id=1002", handler: GetAdminPaidSubscriptionValueSummary, field: "summary"},
		{path: "/api/admin-analytics/paid-subscription-value/users?snapshot_at=1767225600&subscription_id=1002", handler: GetAdminPaidSubscriptionValueUsers, field: "users", expectedItem: 1001},
		{path: "/api/admin-analytics/paid-subscription-value/subscriptions?snapshot_at=1767225600&subscription_id=1002", handler: GetAdminPaidSubscriptionValueSubscriptions, field: "subscriptions", expectedItem: 1002},
		{path: "/api/admin-analytics/paid-subscription-value/breakdown/plans?snapshot_at=1767225600&subscription_id=1002", handler: GetAdminPaidSubscriptionValuePlanBreakdown, field: "plans", expectedItem: 10},
		{path: "/api/admin-analytics/paid-subscription-value/breakdown/sources?snapshot_at=1767225600&subscription_id=1002", handler: GetAdminPaidSubscriptionValueSourceBreakdown, field: "sources"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.field, func(t *testing.T) {
			recorder := performAdminAnalyticsRequest(t, endpoint.path, endpoint.handler)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			payload := decodeAdminAnalyticsPanelEnvelope(t, recorder)
			require.True(t, payload.Success)
			require.NotNil(t, payload.Data.Data.Summary)
			if endpoint.field == "summary" {
				require.Equal(t, int64(0), payload.Data.Range.SnapshotAt)
				require.Equal(t, int64(0), payload.Data.Range.EndTimestamp)
				require.Contains(t, payload.Data.Data.Summary, "recognized_remaining_value_by_currency")
				return
			}
			require.Equal(t, now, payload.Data.Range.SnapshotAt)
			switch endpoint.field {
			case "users":
				require.NotEmpty(t, payload.Data.Data.Users.Items)
				ids := adminAnalyticsJSONIntValues(payload.Data.Data.Users.Items, "user_id")
				require.Contains(t, ids, endpoint.expectedItem)
				require.Contains(t, ids, 1003)
			case "subscriptions":
				require.Len(t, payload.Data.Data.Subscriptions.Items, 1)
				require.Equal(t, endpoint.expectedItem, adminAnalyticsJSONInt(payload.Data.Data.Subscriptions.Items[0], "subscription_id"))
			case "plans":
				require.NotEmpty(t, payload.Data.Data.Plans.Items)
				ids := adminAnalyticsJSONIntValues(payload.Data.Data.Plans.Items, "plan_id")
				require.Contains(t, ids, endpoint.expectedItem)
				require.Contains(t, ids, 11)
			case "sources":
				require.NotEmpty(t, payload.Data.Data.Sources.Items)
				sources := adminAnalyticsJSONStringValues(payload.Data.Data.Sources.Items, "source")
				require.Contains(t, sources, "order")
				require.Contains(t, sources, "admin")
			}
		})
	}
}

func TestInvitationPaidSubscriptionsEndpointsReturnPanelEnvelope(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	now := int64(1767225600)
	seedAdminPaidSubscriptionControllerData(t, now)

	endpoints := []struct {
		path         string
		handler      gin.HandlerFunc
		field        string
		expectedItem int
	}{
		{path: "/api/admin-analytics/invitation-paid-subscriptions/summary?snapshot_at=0&subscription_id=1002", handler: GetAdminInvitationPaidSubscriptionsSummary, field: "summary"},
		{path: "/api/admin-analytics/invitation-paid-subscriptions/inviters?snapshot_at=1767225600&subscription_id=1002", handler: GetAdminInvitationPaidSubscriptionsInviters, field: "inviters", expectedItem: 9001},
		{path: "/api/admin-analytics/invitation-paid-subscriptions/invitees?snapshot_at=1767225600&subscription_id=1002", handler: GetAdminInvitationPaidSubscriptionsInvitees, field: "invitees", expectedItem: 1001},
		{path: "/api/admin-analytics/invitation-paid-subscriptions/subscriptions?snapshot_at=1767225600&subscription_id=1002", handler: GetAdminInvitationPaidSubscriptionsSubscriptions, field: "subscriptions", expectedItem: 1002},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.field, func(t *testing.T) {
			recorder := performAdminAnalyticsRequest(t, endpoint.path, endpoint.handler)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			payload := decodeAdminAnalyticsPanelEnvelope(t, recorder)
			require.True(t, payload.Success)
			require.NotNil(t, payload.Data.Data.Summary)
			if endpoint.field == "summary" {
				require.Equal(t, int64(0), payload.Data.Range.SnapshotAt)
				require.Equal(t, int64(0), payload.Data.Range.EndTimestamp)
				require.Contains(t, payload.Data.Data.Summary, "recognized_invitation_paid_amount_by_currency")
				return
			}
			require.Equal(t, now, payload.Data.Range.SnapshotAt)
			switch endpoint.field {
			case "inviters":
				require.NotEmpty(t, payload.Data.Data.Inviters.Items)
				ids := adminAnalyticsJSONIntValues(payload.Data.Data.Inviters.Items, "inviter_user_id")
				require.Contains(t, ids, endpoint.expectedItem)
				require.Contains(t, ids, 9002)
			case "invitees":
				require.NotEmpty(t, payload.Data.Data.Invitees.Items)
				ids := adminAnalyticsJSONIntValues(payload.Data.Data.Invitees.Items, "invitee_user_id")
				require.Contains(t, ids, endpoint.expectedItem)
				require.Contains(t, ids, 1003)
			case "subscriptions":
				require.Len(t, payload.Data.Data.Subscriptions.Items, 1)
				require.Equal(t, endpoint.expectedItem, adminAnalyticsJSONInt(payload.Data.Data.Subscriptions.Items[0], "subscription_id"))
			}
		})
	}
}

func TestPaidSubscriptionValueRejectsMoneySortWithoutCurrencyEndpoint(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	seedAdminPaidSubscriptionControllerData(t, 1767225600)

	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/paid-subscription-value/users?snapshot_at=1767225600&sort_by=recognized_remaining_value", GetAdminPaidSubscriptionValueUsers)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "currency")
}

func TestInvitationPaidSubscriptionsAllowsNonMoneySortWithoutCurrencyEndpoint(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	seedAdminPaidSubscriptionControllerData(t, 1767225600)

	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/invitation-paid-subscriptions/inviters?snapshot_at=1767225600&sort_by=inviter_user_id", GetAdminInvitationPaidSubscriptionsInviters)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func TestInvitationPaidSubscriptionsDefaultRangeIsAllHistorySnapshotEndpoint(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	seedAdminPaidSubscriptionControllerData(t, 1767225600)

	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/invitation-paid-subscriptions/summary?snapshot_at=1767225600", GetAdminInvitationPaidSubscriptionsSummary)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	payload := decodeAdminAnalyticsPanelEnvelope(t, recorder)
	require.Equal(t, int64(0), payload.Data.Range.StartTimestamp)
	require.Equal(t, int64(1767225600), payload.Data.Range.EndTimestamp)
	require.Equal(t, int64(1767225600), payload.Data.Range.SnapshotAt)
}

func TestPaidSubscriptionValueRejectsUnknownSortEndpoint(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	seedAdminPaidSubscriptionControllerData(t, 1767225600)

	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/paid-subscription-value/users?snapshot_at=1767225600&sort_by=unknown", GetAdminPaidSubscriptionValueUsers)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), model.ErrAdminAnalyticsInvalidSortBy.Error())
}

func decodeAdminAnalyticsPanelEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) adminAnalyticsPanelEnvelopeResponse {
	t.Helper()
	var payload adminAnalyticsPanelEnvelopeResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}

func adminAnalyticsJSONIntValues(items []map[string]any, key string) []int {
	values := make([]int, 0, len(items))
	for _, item := range items {
		values = append(values, adminAnalyticsJSONInt(item, key))
	}
	return values
}

func adminAnalyticsJSONStringValues(items []map[string]any, key string) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		value, _ := item[key].(string)
		values = append(values, value)
	}
	return values
}

func adminAnalyticsJSONInt(item map[string]any, key string) int {
	value, _ := item[key].(float64)
	return int(value)
}

func seedAdminPaidSubscriptionControllerData(t *testing.T, snapshot int64) {
	t.Helper()
	paidCode := "controller-paid"
	adminCode := "controller-admin-paid"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 10, Title: "Controller Paid", Enabled: true, PriceAmount: 40, Currency: "CNY", DurationUnit: model.SubscriptionDurationDay, DurationValue: 30, MonthlyTokenLimit: 1000000000, QuotaResetPeriod: model.SubscriptionResetMonthly, BusinessCode: &paidCode}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 11, Title: "Controller Admin Paid", Enabled: true, PriceAmount: 80, Currency: "CNY", DurationUnit: model.SubscriptionDurationDay, DurationValue: 30, MonthlyTokenLimit: 1000000000, QuotaResetPeriod: model.SubscriptionResetMonthly, BusinessCode: &adminCode}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9001, Username: "controller-inviter", DisplayName: "Controller Inviter", Status: 1, Group: "default", AffCode: "aff-controller-inviter", CreatedAt: snapshot - 5000}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9002, Username: "controller-inviter-two", DisplayName: "Controller Inviter Two", Status: 1, Group: "default", AffCode: "aff-controller-inviter-two", CreatedAt: snapshot - 5000}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 1001, Username: "controller-invitee", DisplayName: "Controller Invitee", Status: 1, Group: "default", AffCode: "aff-controller-invitee", InviterId: 9001, CreatedAt: snapshot - 4000}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 1003, Username: "controller-invitee-two", DisplayName: "Controller Invitee Two", Status: 1, Group: "default", AffCode: "aff-controller-invitee-two", InviterId: 9002, CreatedAt: snapshot - 3000}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 1001, UserId: 1001, PlanId: 10, Status: "active", StartTime: snapshot - 20*86400, EndTime: snapshot + 10*86400, TokenLimit: 1000000000, TokenUsed: 100000000, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder, LastResetTime: snapshot - 20*86400, NextResetTime: snapshot + 2*86400}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 1002, UserId: 1001, PlanId: 10, Status: "active", StartTime: snapshot - 10*86400, EndTime: snapshot + 20*86400, TokenLimit: 1000000000, TokenUsed: 200000000, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder, LastResetTime: snapshot - 10*86400, NextResetTime: snapshot + 2*86400}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 1003, UserId: 1003, PlanId: 11, Status: "active", StartTime: snapshot - 5*86400, EndTime: snapshot + 25*86400, TokenLimit: 1000000000, TokenUsed: 300000000, GrantReason: "admin", Source: "admin", LastResetTime: snapshot - 5*86400, NextResetTime: snapshot + 2*86400}).Error)
}

func TestAdminAnalyticsRejectsInvalidTimeRange(t *testing.T) {
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/overview?start_timestamp=200&end_timestamp=100", GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid time range")
}

func TestAdminAnalyticsRejectsRangeOver365Days(t *testing.T) {
	start := int64(1000)
	end := start + 366*24*60*60
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/overview?start_timestamp=1000&end_timestamp="+strconv.FormatInt(end, 10), GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "time range exceeds 365 days")
}

func TestAdminAnalyticsRejectsInvalidRepeatedPlanIDs(t *testing.T) {
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/overview?plan_ids=1&plan_ids=abc", GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid plan_ids")
}

func TestAdminAnalyticsRejectsInvalidSources(t *testing.T) {
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/overview?sources=bad", GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid source")
}

func TestAdminAnalyticsSnapshotAtUsesHistoricalRangeEnd(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	end := time.Now().Unix() - 24*60*60
	start := end - 60
	target := "/api/admin-analytics/overview?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(end, 10)
	recorder := performAdminAnalyticsRequest(t, target, GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"snapshot_at":`+strconv.FormatInt(end, 10))
}

func TestAdminAnalyticsRejectsUnsupportedSortBy(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/plan-distribution?sort_by=not_a_column", GetAdminAnalyticsPlanDistribution)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "unsupported sort_by")
}

func TestAdminAnalyticsDrilldownRejectsInvalidUserID(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/drilldown/users?user_id=abc", GetAdminAnalyticsDrilldownUsers)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid user_id")
}

func TestAdminAnalyticsDrilldownUsersAcceptsRepeatedUserID(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	seedAdminAnalyticsControllerUsers(t, 3)

	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/drilldown/users?user_id=1&user_id=2", GetAdminAnalyticsDrilldownUsers)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	require.Contains(t, body, `"total":2`)
	require.Contains(t, body, `"user_id":1`)
	require.Contains(t, body, `"user_id":2`)
	require.NotContains(t, body, `"user_id":3`)
}

func TestAdminAnalyticsDrilldownUsersAllowsAllRows(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	seedAdminAnalyticsControllerUsers(t, 150)

	now := time.Now().Unix()
	target := "/api/admin-analytics/drilldown/users?start_timestamp=" + strconv.FormatInt(now-60, 10) + "&end_timestamp=" + strconv.FormatInt(now, 10) + "&limit=0"
	recorder := performAdminAnalyticsRequest(t, target, GetAdminAnalyticsDrilldownUsers)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"limit":150`)
	require.Contains(t, recorder.Body.String(), `"total":150`)
	require.NotContains(t, recorder.Body.String(), `"has_more":true`)
}

func TestAdminAnalyticsDrilldownUsersDoesNotExposeSensitiveFields(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	accessToken := "access-token-secret"
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.User{
		Id:             1,
		Username:       "safe-admin-analytics-user",
		Password:       "password-secret",
		DisplayName:    "safe user",
		Email:          "safe@example.com",
		Status:         1,
		Role:           1,
		Group:          "default",
		AccessToken:    &accessToken,
		AffCode:        "aff-secret",
		Setting:        `{"secret":true}`,
		Remark:         "remark-secret",
		StripeCustomer: "stripe-customer-secret",
		CreatedAt:      now - 10,
		LastLoginAt:    now - 5,
	}).Error)

	target := "/api/admin-analytics/drilldown/users?start_timestamp=" + strconv.FormatInt(now-60, 10) + "&end_timestamp=" + strconv.FormatInt(now, 10)
	recorder := performAdminAnalyticsRequest(t, target, GetAdminAnalyticsDrilldownUsers)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	for _, field := range []string{"password", "access_token", "stripe_customer", "aff_code", "setting", "remark", "key", "allow_ips"} {
		require.NotContains(t, body, `"`+field+`"`)
	}
	for _, secret := range []string{"password-secret", "access-token-secret", "aff-secret", "remark-secret", "stripe-customer-secret"} {
		require.NotContains(t, body, secret)
	}
}

func seedAdminAnalyticsControllerUsers(t *testing.T, count int) {
	t.Helper()

	users := make([]model.User, 0, count)
	for i := 1; i <= count; i++ {
		users = append(users, model.User{Id: i, Username: "analytics-user-" + strconv.Itoa(i), Status: 1, Group: "default", AffCode: "analytics-aff-" + strconv.Itoa(i)})
	}
	require.NoError(t, model.DB.Create(&users).Error)
}
