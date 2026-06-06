package controller

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	adminAnalyticsDefaultRangeSeconds = int64(30 * 24 * 60 * 60)
	adminAnalyticsMaxRangeSeconds     = int64(365 * 24 * 60 * 60)
)

var (
	adminAnalyticsNoSortBy   = map[string]string{}
	adminAnalyticsPlanSortBy = map[string]string{
		"user_count":          "user_count",
		"subscription_count":  "subscription_count",
		"active_count":        "subscription_count",
		"allocated_tokens":    "token_limit",
		"used_tokens":         "token_used",
		"token_used":          "token_used",
		"usage_rate":          "usage_rate",
		"expiring_soon_count": "subscription_count",
	}
	adminAnalyticsQuotaSortBy = map[string]string{
		"usage_rate":        "usage_rate",
		"token_used":        "token_used",
		"remaining_tokens":  "remaining_tokens",
		"request_count":     "request_count",
		"last_request_time": "last_request_time",
		"end_time":          "end_time",
	}
	adminAnalyticsInvitationSortBy = map[string]string{
		"qualified_active_count": "qualified_invite_count",
		"direct_invite_count":    "direct_invite_count",
		"reward_token_used":      "direct_invite_count",
		"reward_usage_rate":      "direct_invite_count",
		"last_reward_month":      "direct_invite_count",
	}
	adminAnalyticsRiskSortBy = map[string]string{
		"severity":    "severity",
		"count":       "sample_size",
		"sample_size": "sample_size",
		"risk_key":    "risk_key",
	}
	adminAnalyticsDrilldownUserSortBy = map[string]string{
		"user_id":    "user_id",
		"username":   "username",
		"status":     "status",
		"usage_rate": "usage_rate",
		"token_used": "token_used",
	}
	adminAnalyticsDrilldownSubscriptionSortBy = map[string]string{
		"subscription_id": "subscription_id",
		"user_id":         "user_id",
		"plan_id":         "plan_id",
		"status":          "status",
		"usage_rate":      "usage_rate",
		"token_used":      "token_used",
		"end_time":        "end_time",
	}
	adminAnalyticsDrilldownInvitationSortBy = map[string]string{
		"inviter_id":       "inviter_id",
		"invitee_id":       "invitee_id",
		"qualified_active": "qualified_active",
		"reward_month":     "reward_month",
	}
	adminPaidSubscriptionValueUserSortBy = map[string]string{
		"recognized_remaining_value": "recognized_remaining_value",
		"active_paid_plan_count":     "active_paid_plan_count",
		"earliest_end_time":          "earliest_end_time",
		"user_id":                    "user_id",
	}
	adminPaidSubscriptionValueSubscriptionSortBy = map[string]string{
		"recognized_remaining_value": "recognized_remaining_value",
		"end_time":                   "end_time",
		"start_time":                 "start_time",
		"plan_price":                 "plan_price",
		"subscription_id":            "subscription_id",
	}
	adminPaidSubscriptionValuePlanSortBy = map[string]string{
		"recognized_remaining_value": "recognized_remaining_value",
		"subscription_count":         "subscription_count",
		"user_count":                 "user_count",
		"plan_id":                    "plan_id",
	}
	adminPaidSubscriptionValueSourceSortBy = map[string]string{
		"recognized_remaining_value": "recognized_remaining_value",
		"subscription_count":         "subscription_count",
		"user_count":                 "user_count",
		"source":                     "source",
		"grant_reason":               "grant_reason",
	}
	adminInvitationPaidInviterSortBy = map[string]string{
		"recognized_invitation_paid_amount": "recognized_invitation_paid_amount",
		"active_invitation_paid_amount":     "active_invitation_paid_amount",
		"active_invitation_remaining_value": "active_invitation_remaining_value",
		"paid_invitee_count":                "paid_invitee_count",
		"active_paid_invitee_count":         "active_paid_invitee_count",
		"inviter_user_id":                   "inviter_user_id",
	}
	adminInvitationPaidInviteeSortBy = map[string]string{
		"recognized_paid_amount":             "recognized_paid_amount",
		"active_remaining_value":             "active_remaining_value",
		"paid_subscription_snapshot_count":   "paid_subscription_snapshot_count",
		"registered_at":                      "registered_at",
		"invitee_user_id":                    "invitee_user_id",
	}
	adminInvitationPaidSubscriptionSortBy = map[string]string{
		"recognized_paid_amount":     "recognized_paid_amount",
		"recognized_remaining_value": "recognized_remaining_value",
		"start_time":                 "start_time",
		"end_time":                   "end_time",
		"plan_price":                 "plan_price",
		"subscription_id":            "subscription_id",
	}
	adminPaidSubscriptionValueMoneySortBy = map[string]struct{}{
		"recognized_remaining_value": {},
		"plan_price":                 {},
	}
	adminInvitationPaidMoneySortBy = map[string]struct{}{
		"recognized_invitation_paid_amount": {},
		"active_invitation_paid_amount":     {},
		"active_invitation_remaining_value": {},
		"recognized_paid_amount":            {},
		"active_remaining_value":            {},
		"recognized_remaining_value":        {},
		"plan_price":                        {},
	}
)

func GetAdminAnalyticsOverview(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsNoSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsOverview(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsPlanDistribution(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsPlanSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsPlanDistribution(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsQuotaDistribution(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsQuotaSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsQuotaDistribution(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsUserLifecycle(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsNoSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsUserLifecycle(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsSubscriptionConversion(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsNoSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsSubscriptionConversion(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsInvitationRewards(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsInvitationSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsInvitationRewards(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminPaidSubscriptionValueSummary(c *gin.Context) {
	query, ok := parseAdminPaidSubscriptionValueQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(c, query, adminAnalyticsNoSortBy, adminPaidSubscriptionValueMoneySortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminPaidSubscriptionValueSummary(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminPaidSubscriptionValueUsers(c *gin.Context) {
	query, ok := parseAdminPaidSubscriptionValueQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(c, query, adminPaidSubscriptionValueUserSortBy, adminPaidSubscriptionValueMoneySortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminPaidSubscriptionValueUsers(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminPaidSubscriptionValueSubscriptions(c *gin.Context) {
	query, ok := parseAdminPaidSubscriptionValueQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(c, query, adminPaidSubscriptionValueSubscriptionSortBy, adminPaidSubscriptionValueMoneySortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminPaidSubscriptionValueSubscriptions(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminPaidSubscriptionValuePlanBreakdown(c *gin.Context) {
	query, ok := parseAdminPaidSubscriptionValueQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(c, query, adminPaidSubscriptionValuePlanSortBy, adminPaidSubscriptionValueMoneySortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminPaidSubscriptionValuePlanBreakdown(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminPaidSubscriptionValueSourceBreakdown(c *gin.Context) {
	query, ok := parseAdminPaidSubscriptionValueQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(c, query, adminPaidSubscriptionValueSourceSortBy, adminPaidSubscriptionValueMoneySortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminPaidSubscriptionValueSourceBreakdown(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminInvitationPaidSubscriptionsSummary(c *gin.Context) {
	query, ok := parseAdminInvitationPaidSubscriptionsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(c, query, adminAnalyticsNoSortBy, adminInvitationPaidMoneySortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminInvitationPaidSubscriptionsSummary(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminInvitationPaidSubscriptionsInviters(c *gin.Context) {
	query, ok := parseAdminInvitationPaidSubscriptionsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(c, query, adminInvitationPaidInviterSortBy, adminInvitationPaidMoneySortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminInvitationPaidSubscriptionsInviters(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminInvitationPaidSubscriptionsInvitees(c *gin.Context) {
	query, ok := parseAdminInvitationPaidSubscriptionsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(c, query, adminInvitationPaidInviteeSortBy, adminInvitationPaidMoneySortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminInvitationPaidSubscriptionsInvitees(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminInvitationPaidSubscriptionsSubscriptions(c *gin.Context) {
	query, ok := parseAdminInvitationPaidSubscriptionsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsMoneySortByOrAbort(c, query, adminInvitationPaidSubscriptionSortBy, adminInvitationPaidMoneySortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminInvitationPaidSubscriptionsSubscriptions(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminUsageConsumptionSummary(c *gin.Context) {
	query, ok := parseAdminUsageAnalyticsQueryOrAbort(c, "summary")
	if !ok {
		return
	}
	data, err := model.GetAdminUsageConsumptionSummary(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminUsageConsumptionTimeseries(c *gin.Context) {
	query, ok := parseAdminUsageAnalyticsQueryOrAbort(c, "timeseries")
	if !ok {
		return
	}
	data, err := model.GetAdminUsageConsumptionTimeseries(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminUsageConsumptionBreakdown(c *gin.Context) {
	query, ok := parseAdminUsageAnalyticsQueryOrAbort(c, "breakdown")
	if !ok {
		return
	}
	data, err := model.GetAdminUsageConsumptionBreakdown(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsRisks(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsRiskSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsRisks(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func normalizeAdminAnalyticsSortByOrAbort(c *gin.Context, query model.AdminAnalyticsQuery, allowed map[string]string) (model.AdminAnalyticsQuery, bool) {
	if query.SortBy == "" {
		return query, true
	}
	normalized, ok := allowed[query.SortBy]
	if !ok {
		writeAdminAnalyticsBadRequest(c, model.ErrAdminAnalyticsInvalidSortBy.Error())
		return model.AdminAnalyticsQuery{}, false
	}
	query.SortBy = normalized
	return query, true
}

func GetAdminAnalyticsDrilldownUsers(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	filter, err := parseAdminDrilldownFilter(c)
	if err != nil {
		writeAdminAnalyticsBadRequest(c, err.Error())
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsDrilldownUserSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsDrilldownUsers(query, filter)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsDrilldownSubscriptions(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	filter, err := parseAdminDrilldownFilter(c)
	if err != nil {
		writeAdminAnalyticsBadRequest(c, err.Error())
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsDrilldownSubscriptionSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsDrilldownSubscriptions(query, filter)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsDrilldownInvitations(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	filter, err := parseAdminDrilldownFilter(c)
	if err != nil {
		writeAdminAnalyticsBadRequest(c, err.Error())
		return
	}
	if normalized, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, adminAnalyticsDrilldownInvitationSortBy); !ok {
		return
	} else {
		query = normalized
	}
	data, err := model.GetAdminAnalyticsDrilldownInvitations(query, filter)
	writeAdminAnalyticsResponse(c, data, err)
}

func parseAdminAnalyticsQueryOrAbort(c *gin.Context) (model.AdminAnalyticsQuery, bool) {
	query, err := parseAdminAnalyticsQuery(c)
	if err != nil {
		writeAdminAnalyticsBadRequest(c, err.Error())
		return model.AdminAnalyticsQuery{}, false
	}
	return query, true
}

func parseAdminPaidSubscriptionValueQueryOrAbort(c *gin.Context) (model.AdminAnalyticsQuery, bool) {
	query, err := parseAdminPaidSubscriptionValueQuery(c)
	if err != nil {
		writeAdminAnalyticsBadRequest(c, err.Error())
		return model.AdminAnalyticsQuery{}, false
	}
	return query, true
}

func parseAdminInvitationPaidSubscriptionsQueryOrAbort(c *gin.Context) (model.AdminAnalyticsQuery, bool) {
	query, err := parseAdminInvitationPaidSubscriptionsQuery(c)
	if err != nil {
		writeAdminAnalyticsBadRequest(c, err.Error())
		return model.AdminAnalyticsQuery{}, false
	}
	return query, true
}

func parseAdminPaidSubscriptionValueQuery(c *gin.Context) (model.AdminAnalyticsQuery, error) {
	snapshotAt, err := parseAdminAnalyticsSnapshotAt(c)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	query, err := parseAdminPaidSubscriptionSharedQuery(c)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	query.StartTimestamp = 0
	query.EndTimestamp = snapshotAt
	query.SnapshotAt = snapshotAt
	query.RangeMode = model.AdminAnalyticsRangeModeSnapshot
	query.TimeRangeExplicit = false
	return query, nil
}

func parseAdminInvitationPaidSubscriptionsQuery(c *gin.Context) (model.AdminAnalyticsQuery, error) {
	snapshotAt, err := parseAdminAnalyticsSnapshotAt(c)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	query, err := parseAdminPaidSubscriptionSharedQuery(c)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	startRaw, hasStart := c.GetQuery("start_timestamp")
	endRaw, hasEnd := c.GetQuery("end_timestamp")
	query.SnapshotAt = snapshotAt
	query.RangeMode = model.AdminAnalyticsRangeModeAllHistory
	if !hasStart && !hasEnd {
		query.StartTimestamp = 0
		query.EndTimestamp = snapshotAt
		query.TimeRangeExplicit = false
		return query, nil
	}
	if hasStart != hasEnd {
		return model.AdminAnalyticsQuery{}, errors.New("start_timestamp and end_timestamp must be provided together")
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startRaw), 10, 64)
	if err != nil || start < 0 {
		return model.AdminAnalyticsQuery{}, errors.New("invalid start_timestamp")
	}
	end, err := strconv.ParseInt(strings.TrimSpace(endRaw), 10, 64)
	if err != nil || end < 0 {
		return model.AdminAnalyticsQuery{}, errors.New("invalid end_timestamp")
	}
	if start > end {
		return model.AdminAnalyticsQuery{}, errors.New("invalid time range")
	}
	if end-start > adminAnalyticsMaxRangeSeconds {
		return model.AdminAnalyticsQuery{}, errors.New("time range exceeds 365 days")
	}
	query.StartTimestamp = start
	query.EndTimestamp = end
	query.TimeRangeExplicit = true
	return query, nil
}

func parseAdminPaidSubscriptionSharedQuery(c *gin.Context) (model.AdminAnalyticsQuery, error) {
	limit, err := parseAdminAnalyticsLimit(c, "limit", model.AdminAnalyticsDefaultLimit)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return model.AdminAnalyticsQuery{}, errors.New("invalid offset")
		}
		offset = parsed
	}
	sortOrder := dto.AdminAnalyticsSortOrder(c.DefaultQuery("sort_order", string(dto.AdminAnalyticsSortDesc)))
	if sortOrder != dto.AdminAnalyticsSortAsc && sortOrder != dto.AdminAnalyticsSortDesc {
		return model.AdminAnalyticsQuery{}, errors.New("invalid sort_order")
	}
	planIDs, err := parseAdminAnalyticsIntList(c, "plan_ids")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	userIDs, err := parseAdminAnalyticsIntList(c, "user_ids")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	sources, err := parseAdminAnalyticsSources(c)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	grantReasons, err := parseAdminAnalyticsEnumList(c, "grant_reasons", adminAnalyticsSourceValues())
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	subscriptionID, err := parseAdminAnalyticsOptionalPositiveInt(c, "subscription_id")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	inviterID, err := parseAdminAnalyticsOptionalPositiveInt(c, "inviter_id")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	inviteeID, err := parseAdminAnalyticsOptionalPositiveInt(c, "invitee_id")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	activeOnly := false
	if raw := strings.TrimSpace(c.Query("active_only")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return model.AdminAnalyticsQuery{}, errors.New("invalid active_only")
		}
		activeOnly = parsed
	}
	excludedMode := dto.AdminAnalyticsExcludedMode(c.DefaultQuery("excluded_mode", string(dto.AdminAnalyticsExcludedModeIncludedOnly)))
	switch excludedMode {
	case dto.AdminAnalyticsExcludedModeIncludedOnly, dto.AdminAnalyticsExcludedModeIncludeExcluded, dto.AdminAnalyticsExcludedModeExcludedOnly:
	default:
		return model.AdminAnalyticsQuery{}, errors.New("invalid excluded_mode")
	}
	return model.AdminAnalyticsQuery{Granularity: dto.AdminAnalyticsGranularityDay, Limit: limit, Offset: offset, SortBy: c.Query("sort_by"), SortOrder: sortOrder, PlanIDs: planIDs, UserIDs: userIDs, Sources: sources, GrantReasons: grantReasons, BusinessCodes: parseAdminAnalyticsStringList(c, "business_codes"), InviterID: inviterID, SubscriptionID: subscriptionID, InviteeID: inviteeID, Currency: strings.TrimSpace(c.Query("currency")), ExcludedMode: excludedMode, ActiveOnly: activeOnly}, nil
}

func parseAdminAnalyticsSnapshotAt(c *gin.Context) (int64, error) {
	raw, ok := c.GetQuery("snapshot_at")
	if !ok {
		return time.Now().Unix(), nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid snapshot_at")
	}
	return parsed, nil
}

func normalizeAdminAnalyticsMoneySortByOrAbort(c *gin.Context, query model.AdminAnalyticsQuery, allowed map[string]string, moneySorts map[string]struct{}) (model.AdminAnalyticsQuery, bool) {
	originalSortBy := query.SortBy
	query, ok := normalizeAdminAnalyticsSortByOrAbort(c, query, allowed)
	if !ok || query.SortBy == "" {
		return query, ok
	}
	_, normalizedMoneySort := moneySorts[query.SortBy]
	_, originalMoneySort := moneySorts[originalSortBy]
	if (normalizedMoneySort || originalMoneySort) && strings.TrimSpace(query.Currency) == "" {
		writeAdminAnalyticsBadRequest(c, "currency is required for money sort")
		return model.AdminAnalyticsQuery{}, false
	}
	return query, true
}
func parseAdminUsageAnalyticsQueryOrAbort(c *gin.Context, endpoint string) (model.AdminAnalyticsUsageQuery, bool) {
	query, err := parseAdminUsageAnalyticsQuery(c, endpoint)
	if err != nil {
		writeAdminAnalyticsBadRequest(c, err.Error())
		return model.AdminAnalyticsUsageQuery{}, false
	}
	return query, true
}

func parseAdminAnalyticsQuery(c *gin.Context) (model.AdminAnalyticsQuery, error) {
	start, end, err := parseAdminAnalyticsTimeRange(c)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	limit, err := parseAdminAnalyticsLimit(c, "limit", model.AdminAnalyticsDefaultLimit)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return model.AdminAnalyticsQuery{}, errors.New("invalid offset")
		}
		offset = parsed
	}
	granularity := dto.AdminAnalyticsGranularity(c.DefaultQuery("granularity", string(dto.AdminAnalyticsGranularityDay)))
	switch granularity {
	case dto.AdminAnalyticsGranularityHour, dto.AdminAnalyticsGranularityDay, dto.AdminAnalyticsGranularityWeek, dto.AdminAnalyticsGranularityMonth:
	default:
		return model.AdminAnalyticsQuery{}, errors.New("invalid granularity")
	}
	sortOrder := dto.AdminAnalyticsSortOrder(c.DefaultQuery("sort_order", string(dto.AdminAnalyticsSortDesc)))
	if sortOrder != dto.AdminAnalyticsSortAsc && sortOrder != dto.AdminAnalyticsSortDesc {
		return model.AdminAnalyticsQuery{}, errors.New("invalid sort_order")
	}
	planIDs, err := parseAdminAnalyticsIntList(c, "plan_ids")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	userIDs, err := parseAdminAnalyticsIntList(c, "user_ids")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	tokenIDs, err := parseAdminAnalyticsIntList(c, "token_ids")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	channelIDs, err := parseAdminAnalyticsIntList(c, "channel_ids")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	sources, err := parseAdminAnalyticsSources(c)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	subscriptionStatuses, err := parseAdminAnalyticsEnumList(c, "subscription_statuses", map[string]struct{}{"active": {}, "expired": {}, "cancelled": {}, "inactive": {}})
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	userStatuses, err := parseAdminAnalyticsUserStatuses(c)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	logStatuses, err := parseAdminAnalyticsEnumList(c, "log_statuses", map[string]struct{}{"success": {}, "error": {}})
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	grantReasons, err := parseAdminAnalyticsEnumList(c, "grant_reasons", adminAnalyticsSourceValues())
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	resetStatuses, err := parseAdminAnalyticsEnumList(c, "reset_status", map[string]struct{}{"due": {}, "not_due": {}})
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	trial, err := parseAdminAnalyticsOptionalBool(c, "trial")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	rewardEligible, err := parseAdminAnalyticsOptionalBool(c, "reward_eligible")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	hasInviter, err := parseAdminAnalyticsOptionalBool(c, "has_inviter")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	inviterID, err := parseAdminAnalyticsOptionalPositiveInt(c, "inviter_id")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	registeredStart, err := parseAdminAnalyticsOptionalTimestamp(c, "registered_start_timestamp")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	registeredEnd, err := parseAdminAnalyticsOptionalTimestamp(c, "registered_end_timestamp")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	subscriptionStart, err := parseAdminAnalyticsOptionalTimestamp(c, "subscription_start_timestamp")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	subscriptionEnd, err := parseAdminAnalyticsOptionalTimestamp(c, "subscription_end_timestamp")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	nextResetStart, err := parseAdminAnalyticsOptionalTimestamp(c, "next_reset_start_timestamp")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	nextResetEnd, err := parseAdminAnalyticsOptionalTimestamp(c, "next_reset_end_timestamp")
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	snapshotAt := time.Now().Unix()
	if end < snapshotAt {
		snapshotAt = end
	}
	return model.AdminAnalyticsQuery{StartTimestamp: start, EndTimestamp: end, SnapshotAt: snapshotAt, Granularity: granularity, Limit: limit, Offset: offset, SortBy: c.Query("sort_by"), SortOrder: sortOrder, PlanIDs: planIDs, UserIDs: userIDs, TokenIDs: tokenIDs, ChannelIDs: channelIDs, Sources: sources, Statuses: parseAdminAnalyticsStringList(c, "statuses"), SubscriptionStatuses: subscriptionStatuses, UserStatuses: userStatuses, LogStatuses: logStatuses, GrantReasons: grantReasons, BusinessCodes: parseAdminAnalyticsStringList(c, "business_codes"), ResetStatuses: resetStatuses, Trial: trial, RewardEligible: rewardEligible, HasInviter: hasInviter, InviterID: inviterID, TimeRangeExplicit: c.Query("start_timestamp") != "" || c.Query("end_timestamp") != "", RangeMode: model.AdminAnalyticsRangeModeDefault, Username: strings.TrimSpace(c.Query("username")), RegisteredStartTimestamp: registeredStart, RegisteredEndTimestamp: registeredEnd, SubscriptionStartTimestamp: subscriptionStart, SubscriptionEndTimestamp: subscriptionEnd, NextResetStartTimestamp: nextResetStart, NextResetEndTimestamp: nextResetEnd}, nil
}

func parseAdminUsageAnalyticsQuery(c *gin.Context, endpoint string) (model.AdminAnalyticsUsageQuery, error) {
	base, err := parseAdminAnalyticsQuery(c)
	if err != nil {
		return model.AdminAnalyticsUsageQuery{}, err
	}
	limit, err := parseAdminAnalyticsLimit(c, "top_n", base.Limit)
	if err != nil {
		return model.AdminAnalyticsUsageQuery{}, err
	}
	sortMetric := dto.AdminUsageMetric(c.DefaultQuery("metric", string(dto.AdminUsageMetricTotalTokens)))
	if base.SortBy != "" && base.SortBy != "metric" {
		sortMetric = dto.AdminUsageMetric(base.SortBy)
	}
	query := model.AdminAnalyticsUsageQuery{AdminAnalyticsQuery: base, GroupBy: dto.AdminUsageGroupBy(c.DefaultQuery("group_by", string(dto.AdminUsageGroupByUser))), Metric: dto.AdminUsageMetric(c.DefaultQuery("metric", string(dto.AdminUsageMetricTotalTokens))), PlanAttribution: dto.AdminPlanAttribution(c.DefaultQuery("plan_attribution", string(dto.AdminPlanAttributionCurrent))), TopN: limit, SortByProvided: c.Query("sort_by") != ""}
	query.SortMetric = sortMetric
	if err := model.ValidateAdminUsageQuery(query, endpoint); err != nil {
		return model.AdminAnalyticsUsageQuery{}, err
	}
	return query, nil
}

func parseAdminAnalyticsTimeRange(c *gin.Context) (int64, int64, error) {
	startRaw, hasStart := c.GetQuery("start_timestamp")
	endRaw, hasEnd := c.GetQuery("end_timestamp")
	if !hasStart && !hasEnd {
		end := time.Now().Unix()
		return end - adminAnalyticsDefaultRangeSeconds, end, nil
	}
	if hasStart != hasEnd {
		return 0, 0, errors.New("start_timestamp and end_timestamp must be provided together")
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startRaw), 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid start_timestamp")
	}
	end, err := strconv.ParseInt(strings.TrimSpace(endRaw), 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid end_timestamp")
	}
	if start > end {
		return 0, 0, errors.New("invalid time range")
	}
	if end-start > adminAnalyticsMaxRangeSeconds {
		return 0, 0, errors.New("time range exceeds 365 days")
	}
	return start, end, nil
}

func parseAdminAnalyticsLimit(c *gin.Context, key string, defaultValue int) (int, error) {
	limit := defaultValue
	if limit <= 0 {
		limit = model.AdminAnalyticsDefaultLimit
	}
	if raw := strings.TrimSpace(c.Query(key)); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, errors.New("invalid " + key)
		}
		limit = parsed
	}
	if limit <= 0 {
		return 0, errors.New("invalid " + key)
	}
	if limit > model.AdminAnalyticsMaxLimit {
		limit = model.AdminAnalyticsMaxLimit
	}
	return limit, nil
}

func parseAdminAnalyticsOptionalPositiveInt(c *gin.Context, key string) (int, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid " + key)
	}
	return parsed, nil
}

func parseAdminAnalyticsOptionalTimestamp(c *gin.Context, key string) (int64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid " + key)
	}
	return parsed, nil
}

func parseAdminAnalyticsOptionalBool(c *gin.Context, key string) (*bool, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, errors.New("invalid " + key)
	}
	return &parsed, nil
}

func parseAdminAnalyticsStringList(c *gin.Context, key string) []string {
	values := c.Request.URL.Query()[key]
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func parseAdminAnalyticsIntList(c *gin.Context, key string) ([]int, error) {
	values := parseAdminAnalyticsStringList(c, key)
	result := make([]int, 0, len(values))
	for _, raw := range values {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return nil, errors.New("invalid " + key)
		}
		result = append(result, parsed)
	}
	return result, nil
}

func parseAdminAnalyticsEnumList(c *gin.Context, key string, allowed map[string]struct{}) ([]string, error) {
	values := parseAdminAnalyticsStringList(c, key)
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return nil, errors.New("invalid " + key)
		}
	}
	return values, nil
}

func parseAdminAnalyticsUserStatuses(c *gin.Context) ([]int, error) {
	values := parseAdminAnalyticsStringList(c, "user_statuses")
	result := make([]int, 0, len(values))
	for _, value := range values {
		switch value {
		case "enabled":
			result = append(result, common.UserStatusEnabled)
		case "disabled":
			result = append(result, common.UserStatusDisabled)
		default:
			return nil, errors.New("invalid user_statuses")
		}
	}
	return result, nil
}

func adminAnalyticsSourceValues() map[string]struct{} {
	return map[string]struct{}{"order": {}, "trial_code": {}, "invite_trial": {}, "monthly_invite_entitlement": {}, "admin": {}, "redemption": {}, "system": {}, "unknown": {}}
}

func parseAdminAnalyticsSources(c *gin.Context) ([]dto.AdminAnalyticsSource, error) {
	values := parseAdminAnalyticsStringList(c, "sources")
	result := make([]dto.AdminAnalyticsSource, 0, len(values))
	for _, value := range values {
		source := dto.AdminAnalyticsSource(value)
		switch source {
		case dto.AdminAnalyticsSourceOrder, dto.AdminAnalyticsSourceTrialCode, dto.AdminAnalyticsSourceInviteTrial, dto.AdminAnalyticsSourceMonthlyInviteEntitlement, dto.AdminAnalyticsSourceAdmin, dto.AdminAnalyticsSourceRedemption, dto.AdminAnalyticsSourceSystem, dto.AdminAnalyticsSourceUnknown:
			result = append(result, source)
		default:
			return nil, errors.New("invalid source")
		}
	}
	return result, nil
}

func parseAdminDrilldownFilter(c *gin.Context) (model.AdminAnalyticsDrilldownFilter, error) {
	userIDs, err := parseAdminAnalyticsIntList(c, "user_id")
	if err != nil {
		return model.AdminAnalyticsDrilldownFilter{}, err
	}
	planID, err := parseAdminAnalyticsOptionalPositiveInt(c, "plan_id")
	if err != nil {
		return model.AdminAnalyticsDrilldownFilter{}, err
	}
	inviterID, err := parseAdminAnalyticsOptionalPositiveInt(c, "inviter_id")
	if err != nil {
		return model.AdminAnalyticsDrilldownFilter{}, err
	}
	return model.AdminAnalyticsDrilldownFilter{UserIDs: userIDs, PlanID: planID, InviterID: inviterID, UserStatus: strings.TrimSpace(c.Query("user_status")), Status: strings.TrimSpace(c.Query("status"))}, nil
}

func writeAdminAnalyticsResponse[T any](c *gin.Context, data T, err error) {
	if err == nil {
		common.ApiSuccess(c, data)
		return
	}
	if errors.Is(err, model.ErrAdminAnalyticsTooManyLogs) || errors.Is(err, model.ErrAdminAnalyticsInvalidGroupBy) || errors.Is(err, model.ErrAdminAnalyticsInvalidMetric) || errors.Is(err, model.ErrAdminAnalyticsInvalidSortBy) {
		writeAdminAnalyticsBadRequest(c, err.Error())
		return
	}
	common.ApiError(c, err)
}

func writeAdminAnalyticsBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": message})
}
