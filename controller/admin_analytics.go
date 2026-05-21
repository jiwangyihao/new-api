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

func GetAdminAnalyticsOverview(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	data, err := model.GetAdminAnalyticsOverview(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsPlanDistribution(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	data, err := model.GetAdminAnalyticsPlanDistribution(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsQuotaDistribution(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	data, err := model.GetAdminAnalyticsQuotaDistribution(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsUserLifecycle(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	data, err := model.GetAdminAnalyticsUserLifecycle(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsSubscriptionConversion(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	data, err := model.GetAdminAnalyticsSubscriptionConversion(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsInvitationRewards(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	data, err := model.GetAdminAnalyticsInvitationRewards(query)
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
	data, err := model.GetAdminAnalyticsRisks(query)
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsDrilldownUsers(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	data, err := model.GetAdminAnalyticsDrilldownUsers(query, parseAdminDrilldownFilter(c))
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsDrilldownSubscriptions(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	data, err := model.GetAdminAnalyticsDrilldownSubscriptions(query, parseAdminDrilldownFilter(c))
	writeAdminAnalyticsResponse(c, data, err)
}

func GetAdminAnalyticsDrilldownInvitations(c *gin.Context) {
	query, ok := parseAdminAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	data, err := model.GetAdminAnalyticsDrilldownInvitations(query, parseAdminDrilldownFilter(c))
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
	sources, err := parseAdminAnalyticsSources(c)
	if err != nil {
		return model.AdminAnalyticsQuery{}, err
	}
	return model.AdminAnalyticsQuery{StartTimestamp: start, EndTimestamp: end, SnapshotAt: time.Now().Unix(), Granularity: granularity, Limit: limit, Offset: offset, SortBy: c.Query("sort_by"), SortOrder: sortOrder, UserGroups: parseAdminAnalyticsStringList(c, "user_groups"), RequestGroups: parseAdminAnalyticsStringList(c, "request_groups"), PlanIDs: planIDs, Sources: sources, Statuses: parseAdminAnalyticsStringList(c, "statuses")}, nil
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
	query := model.AdminAnalyticsUsageQuery{AdminAnalyticsQuery: base, GroupBy: dto.AdminUsageGroupBy(c.DefaultQuery("group_by", string(dto.AdminUsageGroupByUser))), Metric: dto.AdminUsageMetric(c.DefaultQuery("metric", string(dto.AdminUsageMetricTotalTokens))), PlanAttribution: dto.AdminPlanAttribution(c.DefaultQuery("plan_attribution", string(dto.AdminPlanAttributionCurrent))), TopN: limit, SortByProvided: c.Query("sort_by") != ""}
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

func parseAdminDrilldownFilter(c *gin.Context) model.AdminAnalyticsDrilldownFilter {
	userID, _ := strconv.Atoi(c.Query("user_id"))
	planID, _ := strconv.Atoi(c.Query("plan_id"))
	inviterID, _ := strconv.Atoi(c.Query("inviter_id"))
	return model.AdminAnalyticsDrilldownFilter{UserID: userID, PlanID: planID, InviterID: inviterID, UserGroup: strings.TrimSpace(c.Query("user_group")), UserStatus: strings.TrimSpace(c.Query("user_status")), Status: strings.TrimSpace(c.Query("status"))}
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
