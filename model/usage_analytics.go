package model

import (
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

var (
	ErrUsageAnalyticsInvalidToken = errors.New("invalid usage analytics token")
	ErrUsageAnalyticsInvalidGroup = errors.New("invalid usage analytics group_by")
)

type UsageAnalyticsGroupBy = dto.UsageAnalyticsGroupBy
type UsageAnalyticsQuery = dto.UsageAnalyticsQuery
type UsageAnalyticsMetrics = dto.UsageAnalyticsMetrics
type UsageAnalyticsSummaryResponse = dto.UsageAnalyticsSummaryResponse
type UsageAnalyticsTimeseriesResponse = dto.UsageAnalyticsTimeseriesResponse
type UsageAnalyticsTimeseriesPoint = dto.UsageAnalyticsTimeseriesPoint
type UsageAnalyticsBreakdownResponse = dto.UsageAnalyticsBreakdownResponse
type UsageAnalyticsGroup = dto.UsageAnalyticsGroup
type UsageAnalyticsTokenInfo = dto.UsageAnalyticsTokenInfo
type UsageAnalyticsDrilldown = dto.UsageAnalyticsDrilldown

const (
	UsageAnalyticsGroupByToken  = dto.UsageAnalyticsGroupByToken
	UsageAnalyticsGroupByModel  = dto.UsageAnalyticsGroupByModel
	UsageAnalyticsGroupByStream = dto.UsageAnalyticsGroupByStream
	UsageAnalyticsGroupByStatus = dto.UsageAnalyticsGroupByStatus

	UsageAnalyticsGranularityHour = dto.UsageAnalyticsGranularityHour
	UsageAnalyticsGranularityDay  = dto.UsageAnalyticsGranularityDay

	UsageAnalyticsMetricRequestCount = dto.UsageAnalyticsMetricRequestCount
	UsageAnalyticsMetricTotalTokens  = dto.UsageAnalyticsMetricTotalTokens
	UsageAnalyticsMetricQuota        = dto.UsageAnalyticsMetricQuota
	UsageAnalyticsMetricErrorRate    = dto.UsageAnalyticsMetricErrorRate
	UsageAnalyticsMetricAvgLatencyMs = dto.UsageAnalyticsMetricAvgLatencyMs
	UsageAnalyticsMetricP95LatencyMs = dto.UsageAnalyticsMetricP95LatencyMs

	UsageAnalyticsStatusSuccess = dto.UsageAnalyticsStatusSuccess
	UsageAnalyticsStatusError   = dto.UsageAnalyticsStatusError
)

type usageAnalyticsAccumulator struct {
	UsageAnalyticsGroup
	latencyTotalSeconds int64
	latencyCounts       map[int]int
	tokenName           string
	tokenNameAt         int64
}

func GetUsageAnalyticsSummary(query UsageAnalyticsQuery) (UsageAnalyticsSummaryResponse, error) {
	query = usageAnalyticsNormalizeQuery(query)
	if err := usageAnalyticsValidateQuery(query); err != nil {
		return UsageAnalyticsSummaryResponse{}, err
	}

	total := usageAnalyticsNewAccumulator(query.GroupBy, "total", "total", "Total", nil)
	groups := make(map[string]*usageAnalyticsAccumulator)
	activeTokenIDs := make(map[int]struct{})
	if err := usageAnalyticsForEachLog(query, true, func(log Log) {
		usageAnalyticsAddLog(total, log)
		usageAnalyticsAddGroupedLog(groups, query.GroupBy, log)
		if log.TokenId > 0 {
			activeTokenIDs[log.TokenId] = struct{}{}
		}
	}); err != nil {
		return UsageAnalyticsSummaryResponse{}, err
	}

	usageAnalyticsFinalizeAccumulator(total)
	usageAnalyticsFinalizeAccumulators(groups)
	usageAnalyticsAttachTokenInfo(query.UserID, groups)
	usageAnalyticsApplyShares(groups, query.SortBy)
	limitedGroups := usageAnalyticsLimitGroups(groups, query.Limit, query.SortBy, query.SortOrder)

	rpm := 0
	tpm := 0
	if err := usageAnalyticsForEachLog(query, false, func(log Log) {
		rpm++
		tpm += usageAnalyticsLogTokens(log)
	}); err != nil {
		return UsageAnalyticsSummaryResponse{}, err
	}

	totalMetrics := usageAnalyticsGroupMetrics(total.UsageAnalyticsGroup)
	totalMetrics.Rpm = rpm
	totalMetrics.Tpm = tpm
	totalMetrics.ActiveKeyCount = len(activeTokenIDs)
	return UsageAnalyticsSummaryResponse{Total: totalMetrics, Groups: usageAnalyticsAccumulatorSliceToGroups(limitedGroups), GroupBy: query.GroupBy}, nil
}

func GetUsageAnalyticsTimeseries(query UsageAnalyticsQuery) (UsageAnalyticsTimeseriesResponse, error) {
	query = usageAnalyticsNormalizeQuery(query)
	if err := usageAnalyticsValidateQuery(query); err != nil {
		return UsageAnalyticsTimeseriesResponse{}, err
	}

	step := usageAnalyticsStepSeconds(query.Granularity)
	globalGroups := make(map[string]*usageAnalyticsAccumulator)
	bucketGroups := make(map[int64]map[string]*usageAnalyticsAccumulator)
	if err := usageAnalyticsForEachLog(query, true, func(log Log) {
		key, value, label, drilldown := usageAnalyticsDimension(query.GroupBy, log)
		global := globalGroups[key]
		if global == nil {
			global = usageAnalyticsNewAccumulator(query.GroupBy, key, value, label, drilldown)
			globalGroups[key] = global
		}
		usageAnalyticsAddLog(global, log)

		bucket := query.StartTimestamp + ((log.CreatedAt - query.StartTimestamp) / step * step)
		groupsByKey := bucketGroups[bucket]
		if groupsByKey == nil {
			groupsByKey = make(map[string]*usageAnalyticsAccumulator)
			bucketGroups[bucket] = groupsByKey
		}
		bucketAccumulator := groupsByKey[key]
		if bucketAccumulator == nil {
			bucketAccumulator = usageAnalyticsNewAccumulator(query.GroupBy, key, value, label, drilldown)
			groupsByKey[key] = bucketAccumulator
		}
		usageAnalyticsAddLog(bucketAccumulator, log)
	}); err != nil {
		return UsageAnalyticsTimeseriesResponse{}, err
	}

	usageAnalyticsFinalizeAccumulators(globalGroups)
	usageAnalyticsAttachTokenInfo(query.UserID, globalGroups)
	topKeys := usageAnalyticsTopKeySet(globalGroups, query.Limit, query.SortBy, query.SortOrder)
	points := make([]UsageAnalyticsTimeseriesPoint, 0)
	for bucket, groupsByKey := range bucketGroups {
		var other *usageAnalyticsAccumulator
		for key, acc := range groupsByKey {
			if !topKeys[key] {
				if other == nil {
					other = usageAnalyticsNewAccumulator(query.GroupBy, "other", "other", "Other", nil)
				}
				usageAnalyticsMergeAccumulator(other, acc)
				continue
			}

			usageAnalyticsFinalizeAccumulator(acc)
			if query.GroupBy == UsageAnalyticsGroupByToken {
				if global := globalGroups[key]; global != nil {
					acc.GroupLabel = global.GroupLabel
					acc.Token = global.Token
				}
			}
			points = append(points, UsageAnalyticsTimeseriesPoint{
				Timestamp:           bucket,
				TimeLabel:           usageAnalyticsTimeLabel(bucket, query.Granularity),
				UsageAnalyticsGroup: acc.UsageAnalyticsGroup,
			})
		}
		if other != nil {
			usageAnalyticsFinalizeAccumulator(other)
			points = append(points, UsageAnalyticsTimeseriesPoint{
				Timestamp:           bucket,
				TimeLabel:           usageAnalyticsTimeLabel(bucket, query.Granularity),
				UsageAnalyticsGroup: other.UsageAnalyticsGroup,
			})
		}
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].Timestamp == points[j].Timestamp {
			return points[i].GroupKey < points[j].GroupKey
		}
		return points[i].Timestamp < points[j].Timestamp
	})
	return UsageAnalyticsTimeseriesResponse{Points: points, Granularity: query.Granularity}, nil
}

func GetUsageAnalyticsBreakdown(query UsageAnalyticsQuery) (UsageAnalyticsBreakdownResponse, error) {
	query = usageAnalyticsNormalizeQuery(query)
	if err := usageAnalyticsValidateQuery(query); err != nil {
		return UsageAnalyticsBreakdownResponse{}, err
	}

	groups := make(map[string]*usageAnalyticsAccumulator)
	if err := usageAnalyticsForEachLog(query, true, func(log Log) {
		usageAnalyticsAddGroupedLog(groups, query.GroupBy, log)
	}); err != nil {
		return UsageAnalyticsBreakdownResponse{}, err
	}

	usageAnalyticsFinalizeAccumulators(groups)
	usageAnalyticsAttachTokenInfo(query.UserID, groups)
	usageAnalyticsApplyShares(groups, query.SortBy)
	usageAnalyticsSortGroups(groups, query.SortBy, query.SortOrder)
	totalGroups := len(groups)
	ordered := usageAnalyticsOrderedAccumulatorsBySort(groups, query.SortBy, query.SortOrder)
	limit := query.Limit
	if limit > len(ordered) {
		limit = len(ordered)
	}
	responseGroups := ordered[:limit]
	var other *usageAnalyticsAccumulator
	if limit < len(ordered) {
		other = usageAnalyticsNewAccumulator(query.GroupBy, "other", "other", "Other", nil)
		for _, acc := range ordered[limit:] {
			usageAnalyticsMergeAccumulator(other, acc)
		}
		usageAnalyticsFinalizeAccumulator(other)
		usageAnalyticsSetShare(&other.UsageAnalyticsGroup, query.SortBy, usageAnalyticsMetricTotal(groups, query.SortBy))
	}
	return UsageAnalyticsBreakdownResponse{Groups: usageAnalyticsAccumulatorSliceToGroups(responseGroups), TotalGroups: totalGroups, Other: usageAnalyticsAccumulatorToGroupPtr(other), SortBy: query.SortBy, SortOrder: query.SortOrder}, nil
}

func usageAnalyticsNormalizeQuery(query UsageAnalyticsQuery) UsageAnalyticsQuery {
	if query.GroupBy == "" {
		query.GroupBy = UsageAnalyticsGroupByToken
	}
	if query.Granularity == "" {
		query.Granularity = UsageAnalyticsGranularityDay
	}
	if query.Metric == "" {
		query.Metric = UsageAnalyticsMetricTotalTokens
	}
	if query.SortBy == "" {
		query.SortBy = query.Metric
	}
	if query.SortOrder == "" {
		query.SortOrder = dto.UsageAnalyticsSortOrderDescending
	}
	if query.Limit <= 0 {
		query.Limit = 10
	}
	return query
}

func usageAnalyticsValidateQuery(query UsageAnalyticsQuery) error {
	if _, ok := usageAnalyticsGroupExpr(query.GroupBy); !ok {
		return ErrUsageAnalyticsInvalidGroup
	}
	return usageAnalyticsValidateTokenIDs(query)
}

func usageAnalyticsForEachLog(query UsageAnalyticsQuery, useQueryTime bool, visit func(Log)) error {
	db := usageAnalyticsBaseLogQuery(LOG_DB, query, useQueryTime).Select(
		"created_at, type, token_name, model_name, quota, prompt_tokens, completion_tokens, metered_tokens, use_time, is_stream, token_id",
	)
	rows, err := db.Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var log Log
		if err := db.ScanRows(rows, &log); err != nil {
			return err
		}
		visit(log)
	}
	return rows.Err()
}

func usageAnalyticsBaseLogQuery(db *gorm.DB, query UsageAnalyticsQuery, useQueryTime bool) *gorm.DB {
	q := db.Model(&Log{}).Where("user_id = ?", query.UserID).Where("type IN ?", []int{LogTypeConsume, LogTypeError})
	if useQueryTime {
		q = q.Where("created_at >= ?", query.StartTimestamp).Where("created_at <= ?", query.EndTimestamp)
	} else {
		now := time.Now().Unix()
		q = q.Where("created_at >= ?", now-60).Where("created_at <= ?", now)
	}
	if len(query.TokenIDs) > 0 {
		q = q.Where("token_id IN ?", query.TokenIDs)
	}
	if len(query.ModelNames) > 0 {
		q = q.Where("model_name IN ?", query.ModelNames)
	}
	if len(query.Streams) > 0 {
		q = q.Where("is_stream IN ?", query.Streams)
	}
	statusTypes := usageAnalyticsStatusTypes(query.Statuses)
	if len(statusTypes) > 0 {
		q = q.Where("type IN ?", statusTypes)
	}
	return q
}

func usageAnalyticsValidateTokenIDs(query UsageAnalyticsQuery) error {
	if len(query.TokenIDs) == 0 {
		return nil
	}
	for _, tokenID := range query.TokenIDs {
		var token Token
		err := DB.Where("id = ? AND user_id = ?", tokenID, query.UserID).First(&token).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		err = LOG_DB.Model(&Log{}).Where("user_id = ? AND token_id = ? AND type IN ?", query.UserID, tokenID, []int{LogTypeConsume, LogTypeError}).Limit(1).Count(&count).Error
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrUsageAnalyticsInvalidToken
		}
	}
	return nil
}

func usageAnalyticsStatusTypes(statuses []string) []int {
	if len(statuses) == 0 {
		return nil
	}
	hasSuccess := false
	hasError := false
	for _, status := range statuses {
		switch status {
		case UsageAnalyticsStatusSuccess:
			hasSuccess = true
		case UsageAnalyticsStatusError:
			hasError = true
		}
	}
	if hasSuccess && hasError {
		return []int{LogTypeConsume, LogTypeError}
	}
	if hasSuccess {
		return []int{LogTypeConsume}
	}
	if hasError {
		return []int{LogTypeError}
	}
	return nil
}

func usageAnalyticsAddGroupedLog(groups map[string]*usageAnalyticsAccumulator, groupBy UsageAnalyticsGroupBy, log Log) {
	key, value, label, drilldown := usageAnalyticsDimension(groupBy, log)
	acc := groups[key]
	if acc == nil {
		acc = usageAnalyticsNewAccumulator(groupBy, key, value, label, drilldown)
		groups[key] = acc
	}
	usageAnalyticsAddLog(acc, log)
}

func usageAnalyticsNewAccumulator(groupBy UsageAnalyticsGroupBy, key string, value string, label string, drilldown *UsageAnalyticsDrilldown) *usageAnalyticsAccumulator {
	return &usageAnalyticsAccumulator{UsageAnalyticsGroup: UsageAnalyticsGroup{GroupBy: groupBy, GroupKey: key, GroupValue: value, GroupLabel: label, Drilldown: drilldown}}
}

func usageAnalyticsAddLog(acc *usageAnalyticsAccumulator, log Log) {
	acc.RequestCount++
	if log.Type == LogTypeConsume {
		acc.SuccessCount++
	} else if log.Type == LogTypeError {
		acc.ErrorCount++
	}
	acc.Quota += usageAnalyticsLogQuota(log)
	acc.PromptTokens += usageAnalyticsLogPromptTokens(log)
	acc.CompletionTokens += usageAnalyticsLogCompletionTokens(log)
	tokens := usageAnalyticsLogTokens(log)
	acc.MeteredTokens += tokens
	acc.TotalTokens += tokens
	useTime := usageAnalyticsLogUseTime(log)
	acc.latencyTotalSeconds += int64(useTime)
	if acc.latencyCounts == nil {
		acc.latencyCounts = make(map[int]int)
	}
	acc.latencyCounts[useTime]++
	if acc.FirstUsedAt == 0 || log.CreatedAt < acc.FirstUsedAt {
		acc.FirstUsedAt = log.CreatedAt
	}
	if log.CreatedAt > acc.LastUsedAt {
		acc.LastUsedAt = log.CreatedAt
	}
	if log.TokenName != "" && log.CreatedAt >= acc.tokenNameAt {
		acc.tokenName = log.TokenName
		acc.tokenNameAt = log.CreatedAt
	}
}

func usageAnalyticsMergeAccumulator(dst *usageAnalyticsAccumulator, src *usageAnalyticsAccumulator) {
	dst.RequestCount += src.RequestCount
	dst.SuccessCount += src.SuccessCount
	dst.ErrorCount += src.ErrorCount
	dst.Quota += src.Quota
	dst.PromptTokens += src.PromptTokens
	dst.CompletionTokens += src.CompletionTokens
	dst.MeteredTokens += src.MeteredTokens
	dst.TotalTokens += src.TotalTokens
	if dst.FirstUsedAt == 0 || (src.FirstUsedAt != 0 && src.FirstUsedAt < dst.FirstUsedAt) {
		dst.FirstUsedAt = src.FirstUsedAt
	}
	if src.LastUsedAt > dst.LastUsedAt {
		dst.LastUsedAt = src.LastUsedAt
	}
	dst.latencyTotalSeconds += src.latencyTotalSeconds
	if len(src.latencyCounts) > 0 {
		if dst.latencyCounts == nil {
			dst.latencyCounts = make(map[int]int, len(src.latencyCounts))
		}
		for value, count := range src.latencyCounts {
			dst.latencyCounts[value] += count
		}
	}
	if src.tokenName != "" && src.tokenNameAt >= dst.tokenNameAt {
		dst.tokenName = src.tokenName
		dst.tokenNameAt = src.tokenNameAt
	}
}

func usageAnalyticsFinalizeAccumulators(groups map[string]*usageAnalyticsAccumulator) {
	for _, acc := range groups {
		usageAnalyticsFinalizeAccumulator(acc)
	}
}

func usageAnalyticsFinalizeAccumulator(acc *usageAnalyticsAccumulator) {
	if acc == nil {
		return
	}
	if acc.RequestCount > 0 {
		acc.SuccessRate = float64(acc.SuccessCount) / float64(acc.RequestCount)
		acc.ErrorRate = float64(acc.ErrorCount) / float64(acc.RequestCount)
		acc.AvgLatencyMs = int(acc.latencyTotalSeconds * 1000 / int64(acc.RequestCount))
		acc.P95LatencyMs = usageAnalyticsP95LatencyMs(acc.latencyCounts, acc.RequestCount)
	}
}

func usageAnalyticsGroupMetrics(group UsageAnalyticsGroup) UsageAnalyticsMetrics {
	return UsageAnalyticsMetrics{
		RequestCount:     group.RequestCount,
		SuccessCount:     group.SuccessCount,
		ErrorCount:       group.ErrorCount,
		SuccessRate:      group.SuccessRate,
		ErrorRate:        group.ErrorRate,
		Quota:            group.Quota,
		PromptTokens:     group.PromptTokens,
		CompletionTokens: group.CompletionTokens,
		MeteredTokens:    group.MeteredTokens,
		TotalTokens:      group.TotalTokens,
		AvgLatencyMs:     group.AvgLatencyMs,
		P95LatencyMs:     group.P95LatencyMs,
		FirstUsedAt:      group.FirstUsedAt,
		LastUsedAt:       group.LastUsedAt,
	}
}

func usageAnalyticsP95LatencyMs(counts map[int]int, total int) int {
	if total <= 0 || len(counts) == 0 {
		return 0
	}
	values := make([]int, 0, len(counts))
	for value := range counts {
		values = append(values, value)
	}
	sort.Ints(values)
	target := (95*int64(total) + 99) / 100
	seen := int64(0)
	for _, value := range values {
		seen += int64(counts[value])
		if seen >= target {
			return value * 1000
		}
	}
	return values[len(values)-1] * 1000
}

func usageAnalyticsLogTokens(log Log) int {
	if log.Type != LogTypeConsume {
		return 0
	}
	value := 0
	if log.MeteredTokens != nil {
		value = *log.MeteredTokens
	} else {
		value = log.PromptTokens + log.CompletionTokens
	}
	if value < 0 {
		return 0
	}
	return value
}

func usageAnalyticsLogQuota(log Log) int {
	if log.Type != LogTypeConsume || log.Quota < 0 {
		return 0
	}
	return log.Quota
}

func usageAnalyticsLogPromptTokens(log Log) int {
	if log.Type != LogTypeConsume || log.PromptTokens < 0 {
		return 0
	}
	return log.PromptTokens
}

func usageAnalyticsLogCompletionTokens(log Log) int {
	if log.Type != LogTypeConsume || log.CompletionTokens < 0 {
		return 0
	}
	return log.CompletionTokens
}

func usageAnalyticsLogUseTime(log Log) int {
	if log.UseTime < 0 {
		return 0
	}
	return log.UseTime
}

func usageAnalyticsDimension(groupBy UsageAnalyticsGroupBy, log Log) (string, string, string, *UsageAnalyticsDrilldown) {
	switch groupBy {
	case UsageAnalyticsGroupByToken:
		value := strconv.Itoa(log.TokenId)
		id := log.TokenId
		return "token:" + value, value, usageAnalyticsTokenFallbackLabel(log), &UsageAnalyticsDrilldown{TokenID: &id}
	case UsageAnalyticsGroupByModel:
		value := log.ModelName
		label := value
		if label == "" {
			label = "Unknown Model"
		}
		return "model:" + value, value, label, &UsageAnalyticsDrilldown{ModelName: &value}
	case UsageAnalyticsGroupByStream:
		value := strconv.FormatBool(log.IsStream)
		isStream := log.IsStream
		label := "Non-streaming"
		if log.IsStream {
			label = "Streaming"
		}
		return "stream:" + value, value, label, &UsageAnalyticsDrilldown{IsStream: &isStream}
	case UsageAnalyticsGroupByStatus:
		status := UsageAnalyticsStatusError
		label := "Error"
		if log.Type == LogTypeConsume {
			status = UsageAnalyticsStatusSuccess
			label = "Success"
		}
		return "status:" + status, status, label, &UsageAnalyticsDrilldown{Status: &status}
	default:
		return "unknown", "unknown", "Unknown", nil
	}
}

func usageAnalyticsTokenFallbackLabel(log Log) string {
	if log.TokenName != "" {
		return log.TokenName
	}
	return "Deleted API Key"
}

func usageAnalyticsGroupExpr(groupBy UsageAnalyticsGroupBy) (string, bool) {
	switch groupBy {
	case UsageAnalyticsGroupByToken:
		return "token_id", true
	case UsageAnalyticsGroupByModel:
		return "model_name", true
	case UsageAnalyticsGroupByStream:
		return "is_stream", true
	case UsageAnalyticsGroupByStatus:
		return "type", true
	default:
		return "", false
	}
}

func usageAnalyticsAttachTokenInfo(userID int, groups map[string]*usageAnalyticsAccumulator) {
	ids := make([]int, 0, len(groups))
	for _, acc := range groups {
		if acc.GroupBy == UsageAnalyticsGroupByToken {
			id, err := strconv.Atoi(acc.GroupValue)
			if err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return
	}
	var tokens []Token
	if err := DB.Where("user_id = ? AND id IN ?", userID, ids).Find(&tokens).Error; err != nil {
		return
	}
	tokenByID := make(map[int]Token, len(tokens))
	for _, token := range tokens {
		tokenByID[token.Id] = token
	}
	for _, acc := range groups {
		if acc.GroupBy != UsageAnalyticsGroupByToken {
			continue
		}
		id, err := strconv.Atoi(acc.GroupValue)
		if err != nil {
			continue
		}
		if token, ok := tokenByID[id]; ok {
			maskedKey := token.GetMaskedKey()
			status := token.Status
			remainQuota := token.RemainQuota
			unlimitedQuota := token.UnlimitedQuota
			acc.GroupLabel = token.Name
			acc.Token = &UsageAnalyticsTokenInfo{ID: token.Id, Name: token.Name, MaskedKey: &maskedKey, Status: &status, RemainQuota: &remainQuota, UnlimitedQuota: &unlimitedQuota, Deleted: false}
			continue
		}
		label := acc.tokenName
		if label == "" {
			label = acc.GroupLabel
		}
		acc.GroupLabel = label
		acc.Token = &UsageAnalyticsTokenInfo{ID: id, Name: label, MaskedKey: nil, Status: nil, RemainQuota: nil, UnlimitedQuota: nil, Deleted: true}
	}
}

func usageAnalyticsApplyShares(groups map[string]*usageAnalyticsAccumulator, metric string) {
	total := usageAnalyticsMetricTotal(groups, metric)
	for _, acc := range groups {
		usageAnalyticsSetShare(&acc.UsageAnalyticsGroup, metric, total)
	}
}

func usageAnalyticsSetShare(group *UsageAnalyticsGroup, metric string, total int) {
	if !usageAnalyticsMetricSupportsShare(metric) {
		group.Share = nil
		return
	}
	share := 0.0
	if total > 0 {
		share = float64(usageAnalyticsMetricValue(*group, metric)) / float64(total)
	}
	group.Share = &share
}

func usageAnalyticsMetricSupportsShare(metric string) bool {
	return metric == UsageAnalyticsMetricRequestCount || metric == UsageAnalyticsMetricTotalTokens || metric == UsageAnalyticsMetricQuota
}

func usageAnalyticsMetricTotal(groups map[string]*usageAnalyticsAccumulator, metric string) int {
	total := 0
	for _, acc := range groups {
		total += usageAnalyticsMetricValue(acc.UsageAnalyticsGroup, metric)
	}
	return total
}

func usageAnalyticsMetricValue(group UsageAnalyticsGroup, metric string) int {
	switch metric {
	case UsageAnalyticsMetricRequestCount:
		return group.RequestCount
	case UsageAnalyticsMetricQuota:
		return group.Quota
	case UsageAnalyticsMetricErrorRate:
		return int(group.ErrorRate * 1000000000)
	case UsageAnalyticsMetricAvgLatencyMs:
		return group.AvgLatencyMs
	case UsageAnalyticsMetricP95LatencyMs:
		return group.P95LatencyMs
	case "first_used_at":
		return int(group.FirstUsedAt)
	case "last_used_at":
		return int(group.LastUsedAt)
	case UsageAnalyticsMetricTotalTokens:
		fallthrough
	default:
		return group.TotalTokens
	}
}

func usageAnalyticsSortGroups(groups map[string]*usageAnalyticsAccumulator, sortBy string, sortOrder string) {
	ordered := usageAnalyticsOrderedAccumulatorsBySort(groups, sortBy, sortOrder)
	for key := range groups {
		delete(groups, key)
	}
	for _, acc := range ordered {
		groups[acc.GroupKey] = acc
	}
}

func usageAnalyticsOrderedAccumulators(groups map[string]*usageAnalyticsAccumulator) []*usageAnalyticsAccumulator {
	return usageAnalyticsOrderedAccumulatorsBySort(groups, "", dto.UsageAnalyticsSortOrderDescending)
}

func usageAnalyticsOrderedAccumulatorsBySort(groups map[string]*usageAnalyticsAccumulator, sortBy string, sortOrder string) []*usageAnalyticsAccumulator {
	ordered := make([]*usageAnalyticsAccumulator, 0, len(groups))
	for _, acc := range groups {
		ordered = append(ordered, acc)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left := usageAnalyticsMetricValue(ordered[i].UsageAnalyticsGroup, sortBy)
		right := usageAnalyticsMetricValue(ordered[j].UsageAnalyticsGroup, sortBy)
		if left == right {
			return ordered[i].GroupKey < ordered[j].GroupKey
		}
		if sortOrder == dto.UsageAnalyticsSortOrderAscending {
			return left < right
		}
		return left > right
	})
	return ordered
}

func usageAnalyticsLimitGroups(groups map[string]*usageAnalyticsAccumulator, limit int, sortBy string, sortOrder string) []*usageAnalyticsAccumulator {
	ordered := usageAnalyticsOrderedAccumulatorsBySort(groups, sortBy, sortOrder)
	if limit > len(ordered) {
		limit = len(ordered)
	}
	return ordered[:limit]
}

func usageAnalyticsAccumulatorsToGroups(groups map[string]*usageAnalyticsAccumulator) []UsageAnalyticsGroup {
	return usageAnalyticsAccumulatorSliceToGroups(usageAnalyticsOrderedAccumulators(groups))
}

func usageAnalyticsAccumulatorSliceToGroups(accumulators []*usageAnalyticsAccumulator) []UsageAnalyticsGroup {
	groups := make([]UsageAnalyticsGroup, 0, len(accumulators))
	for _, acc := range accumulators {
		groups = append(groups, acc.UsageAnalyticsGroup)
	}
	return groups
}

func usageAnalyticsAccumulatorToGroupPtr(acc *usageAnalyticsAccumulator) *UsageAnalyticsGroup {
	if acc == nil {
		return nil
	}
	return &acc.UsageAnalyticsGroup
}

func usageAnalyticsTopKeySet(groups map[string]*usageAnalyticsAccumulator, limit int, sortBy string, sortOrder string) map[string]bool {
	ordered := usageAnalyticsOrderedAccumulatorsBySort(groups, sortBy, sortOrder)
	if limit > len(ordered) {
		limit = len(ordered)
	}
	keys := make(map[string]bool, limit)
	for _, acc := range ordered[:limit] {
		keys[acc.GroupKey] = true
	}
	return keys
}

func usageAnalyticsStepSeconds(granularity string) int64 {
	if granularity == UsageAnalyticsGranularityHour {
		return 3600
	}
	return 86400
}

func usageAnalyticsTimeLabel(timestamp int64, granularity string) string {
	layout := "01-02"
	if granularity == UsageAnalyticsGranularityHour {
		layout = "01-02 15:04"
	}
	return time.Unix(timestamp, 0).Format(layout)
}
