package controller

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	usageAnalyticsDefaultRangeSeconds = int64(7 * 24 * 60 * 60)
	usageAnalyticsMaxRangeSeconds     = int64(31 * 24 * 60 * 60)
	usageAnalyticsDefaultLimit        = 10
	usageAnalyticsMaxLimit            = 50
)

var (
	usageAnalyticsMetrics = map[string]struct{}{
		model.UsageAnalyticsMetricRequestCount: {},
		model.UsageAnalyticsMetricTotalTokens:  {},
		model.UsageAnalyticsMetricQuota:        {},
		model.UsageAnalyticsMetricErrorRate:    {},
		model.UsageAnalyticsMetricAvgLatencyMs: {},
		model.UsageAnalyticsMetricP95LatencyMs: {},
	}
	usageAnalyticsSortFields = map[string]struct{}{
		model.UsageAnalyticsMetricRequestCount: {},
		model.UsageAnalyticsMetricTotalTokens:  {},
		model.UsageAnalyticsMetricQuota:        {},
		model.UsageAnalyticsMetricErrorRate:    {},
		model.UsageAnalyticsMetricAvgLatencyMs: {},
		model.UsageAnalyticsMetricP95LatencyMs: {},
		"first_used_at":                        {},
		"last_used_at":                         {},
	}
)

func GetUsageAnalyticsSummary(c *gin.Context) {
	query, ok := parseUsageAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	response, err := model.GetUsageAnalyticsSummary(query)
	writeUsageAnalyticsResponse(c, response, err)
}

func GetUsageAnalyticsTimeseries(c *gin.Context) {
	query, ok := parseUsageAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	response, err := model.GetUsageAnalyticsTimeseries(query)
	writeUsageAnalyticsResponse(c, response, err)
}

func GetUsageAnalyticsBreakdown(c *gin.Context) {
	query, ok := parseUsageAnalyticsQueryOrAbort(c)
	if !ok {
		return
	}
	response, err := model.GetUsageAnalyticsBreakdown(query)
	writeUsageAnalyticsResponse(c, response, err)
}

func parseUsageAnalyticsQueryOrAbort(c *gin.Context) (model.UsageAnalyticsQuery, bool) {
	query, err := parseUsageAnalyticsQuery(c)
	if err != nil {
		writeUsageAnalyticsBadRequest(c, err.Error())
		return model.UsageAnalyticsQuery{}, false
	}
	return query, true
}

func writeUsageAnalyticsResponse(c *gin.Context, data any, err error) {
	if err == nil {
		common.ApiSuccess(c, data)
		return
	}
	if errors.Is(err, model.ErrUsageAnalyticsInvalidToken) || errors.Is(err, model.ErrUsageAnalyticsTooManyLogs) || errors.Is(err, model.ErrUsageAnalyticsInvalidGroup) {
		writeUsageAnalyticsBadRequest(c, err.Error())
		return
	}
	common.ApiError(c, err)
}

func writeUsageAnalyticsBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"message": message,
	})
}

func parseUsageAnalyticsQuery(c *gin.Context) (model.UsageAnalyticsQuery, error) {
	if hasAnyQuery(c, "endpoint", "billing_source", "billing_tier", "modality") {
		return model.UsageAnalyticsQuery{}, errors.New("unsupported filter in current phase")
	}

	start, end, err := parseUsageAnalyticsTimeRange(c)
	if err != nil {
		return model.UsageAnalyticsQuery{}, err
	}

	groupBy := model.UsageAnalyticsGroupBy(c.DefaultQuery("group_by", string(model.UsageAnalyticsGroupByToken)))
	switch groupBy {
	case model.UsageAnalyticsGroupByToken, model.UsageAnalyticsGroupByModel, model.UsageAnalyticsGroupByStream, model.UsageAnalyticsGroupByStatus:
	default:
		return model.UsageAnalyticsQuery{}, errors.New("unsupported group_by in current phase")
	}

	metric := c.DefaultQuery("metric", model.UsageAnalyticsMetricTotalTokens)
	if _, ok := usageAnalyticsMetrics[metric]; !ok {
		return model.UsageAnalyticsQuery{}, errors.New("invalid metric")
	}

	granularity := c.DefaultQuery("granularity", model.UsageAnalyticsGranularityDay)
	if granularity != model.UsageAnalyticsGranularityHour && granularity != model.UsageAnalyticsGranularityDay {
		return model.UsageAnalyticsQuery{}, errors.New("invalid granularity")
	}

	tokenIDs, err := parseUsageAnalyticsIntList(c, "token_ids")
	if err != nil {
		return model.UsageAnalyticsQuery{}, err
	}
	modelNames := parseUsageAnalyticsStringList(c, "model_names")
	streams, err := parseUsageAnalyticsBoolList(c, "streams")
	if err != nil {
		return model.UsageAnalyticsQuery{}, err
	}
	statuses, err := parseUsageAnalyticsStatusList(c, "statuses")
	if err != nil {
		return model.UsageAnalyticsQuery{}, err
	}

	limit := usageAnalyticsDefaultLimit
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			return model.UsageAnalyticsQuery{}, errors.New("invalid limit")
		}
		limit = parsed
	}
	if limit <= 0 {
		limit = usageAnalyticsDefaultLimit
	} else if limit > usageAnalyticsMaxLimit {
		limit = usageAnalyticsMaxLimit
	}

	sortBy := c.Query("sort_by")
	if sortBy == "" {
		sortBy = metric
	}
	if _, ok := usageAnalyticsSortFields[sortBy]; !ok {
		return model.UsageAnalyticsQuery{}, errors.New("invalid sort_by")
	}

	sortOrder := c.DefaultQuery("sort_order", "desc")
	if sortOrder != "asc" && sortOrder != "desc" {
		return model.UsageAnalyticsQuery{}, errors.New("invalid sort_order")
	}

	return model.UsageAnalyticsQuery{
		UserID:         c.GetInt("id"),
		StartTimestamp: start,
		EndTimestamp:   end,
		Granularity:    granularity,
		GroupBy:        groupBy,
		Metric:         metric,
		TokenIDs:       tokenIDs,
		ModelNames:     modelNames,
		Streams:        streams,
		Statuses:       statuses,
		Limit:          limit,
		SortBy:         sortBy,
		SortOrder:      sortOrder,
	}, nil
}

func parseUsageAnalyticsTimeRange(c *gin.Context) (int64, int64, error) {
	startRaw, hasStart := c.GetQuery("start_timestamp")
	endRaw, hasEnd := c.GetQuery("end_timestamp")
	if !hasStart && !hasEnd {
		end := time.Now().Unix()
		return end - usageAnalyticsDefaultRangeSeconds, end, nil
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
		return 0, 0, errors.New("start_timestamp must be before end_timestamp")
	}
	if end-start > usageAnalyticsMaxRangeSeconds {
		return 0, 0, errors.New("time range exceeds 31 days")
	}
	return start, end, nil
}

func hasAnyQuery(c *gin.Context, keys ...string) bool {
	values := c.Request.URL.Query()
	for _, key := range keys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func usageAnalyticsQueryValues(c *gin.Context, key string) []string {
	values, ok := c.Request.URL.Query()[key]
	if !ok {
		return nil
	}
	if len(values) <= 1 {
		if len(values) == 0 || values[0] == "" {
			return nil
		}
		return splitUsageAnalyticsCommaFallback(values[0])
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitUsageAnalyticsCommaFallback(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseUsageAnalyticsIntList(c *gin.Context, key string) ([]int, error) {
	values := usageAnalyticsQueryValues(c, key)
	if len(values) == 0 {
		return nil, nil
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return nil, errors.New("invalid " + key)
		}
		if _, ok := seen[parsed]; ok {
			continue
		}
		seen[parsed] = struct{}{}
		result = append(result, parsed)
	}
	sort.Ints(result)
	return result, nil
}

func parseUsageAnalyticsStringList(c *gin.Context, key string) []string {
	values := usageAnalyticsQueryValues(c, key)
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func parseUsageAnalyticsBoolList(c *gin.Context, key string) ([]bool, error) {
	values := usageAnalyticsQueryValues(c, key)
	if len(values) == 0 {
		return nil, nil
	}
	seen := make(map[bool]struct{}, len(values))
	result := make([]bool, 0, len(values))
	for _, value := range values {
		var parsed bool
		switch value {
		case "true":
			parsed = true
		case "false":
			parsed = false
		default:
			return nil, errors.New("invalid " + key)
		}
		if _, ok := seen[parsed]; ok {
			continue
		}
		seen[parsed] = struct{}{}
		result = append(result, parsed)
	}
	sort.Slice(result, func(i, j int) bool { return !result[i] && result[j] })
	return result, nil
}

func parseUsageAnalyticsStatusList(c *gin.Context, key string) ([]string, error) {
	values := usageAnalyticsQueryValues(c, key)
	if len(values) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != model.UsageAnalyticsStatusSuccess && value != model.UsageAnalyticsStatusError {
			return nil, errors.New("invalid " + key)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}
