package model

import (
	"errors"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

const usageAnalyticsCandidateLimit = 50000

var (
	ErrUsageAnalyticsInvalidToken = errors.New("invalid usage analytics token")
	ErrUsageAnalyticsTooManyLogs  = errors.New("usage analytics candidate logs exceed limit")
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
	UsageAnalyticsGroupByGroup  = dto.UsageAnalyticsGroupByGroup
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
	UsageAnalyticsMetricLastUsedAt   = dto.UsageAnalyticsMetricLastUsedAt
	UsageAnalyticsMetricFirstUsedAt  = dto.UsageAnalyticsMetricFirstUsedAt

	UsageAnalyticsStatusSuccess = dto.UsageAnalyticsStatusSuccess
	UsageAnalyticsStatusError   = dto.UsageAnalyticsStatusError
)

type usageAnalyticsAccumulator struct {
	UsageAnalyticsGroup
	latencySamples []int
	tokenName       string
	tokenNameAt     int64
}

func GetUsageAnalyticsSummary(query UsageAnalyticsQuery) (UsageAnalyticsSummaryResponse, error) {
	query = usageAnalyticsNormalizeQuery(query)
	logs, err := usageAnalyticsLoadCandidateLogs(query, true)
	if err != nil {
		return UsageAnalyticsSummaryResponse{}, err
	}
	total, groups := usageAnalyticsAggregate(logs, query.GroupBy)
	usageAnalyticsFinalizeAccumulator(total)
	usageAnalyticsFinalizeAccumulators(groups)
	usageAnalyticsAttachTokenInfo(query.UserID, groups)
	usageAnalyticsApplyShares(groups, query.SortBy)
	usageAnalyticsSortGroups(groups, query.SortBy, query.SortOrder)
	groups = usageAnalyticsLimitGroups(groups, query.Limit)
	recentLogs, err := usageAnalyticsLoadCandidateLogs(query, false)
	if err != nil {
		return UsageAnalyticsSummaryResponse{}, err
	}
	rpm, tpm := usageAnalyticsRPMAndTPM(recentLogs)
	totalMetrics := usageAnalyticsGroupMetrics(total.UsageAnalyticsGroup)
	totalMetrics.Rpm = rpm
	totalMetrics.Tpm = tpm
	totalMetrics.ActiveKeyCount = usageAnalyticsActiveKeyCount(logs)
	return UsageAnalyticsSummaryResponse{Total: totalMetrics, Groups: usageAnalyticsAccumulatorsToGroups(groups), GroupBy: query.GroupBy}, nil
}

func GetUsageAnalyticsTimeseries(query UsageAnalyticsQuery) (UsageAnalyticsTimeseriesResponse, error) {
	query = usageAnalyticsNormalizeQuery(query)
	logs, err := usageAnalyticsLoadCandidateLogs(query, true)
	if err != nil {
		return UsageAnalyticsTimeseriesResponse{}, err
	}
	step := usageAnalyticsStepSeconds(query.Granularity)
	_, globalGroups := usageAnalyticsAggregate(logs, query.GroupBy)
	usageAnalyticsFinalizeAccumulators(globalGroups)
	usageAnalyticsAttachTokenInfo(query.UserID, globalGroups)
	usageAnalyticsSortGroups(globalGroups, query.SortBy, query.SortOrder)
	topKeys := usageAnalyticsTopKeySet(globalGroups, query.Limit)
	bucketGroups := make(map[int64]map[string]*usageAnalyticsAccumulator)
	for i := range logs {
		log := logs[i]
		if log.CreatedAt < query.StartTimestamp || log.CreatedAt > query.EndTimestamp {
			continue
		}
		bucket := query.StartTimestamp + ((log.CreatedAt - query.StartTimestamp) / step * step)
		key, value, label, drilldown := usageAnalyticsDimension(query.GroupBy, log)
		if !topKeys[key] {
			key = "other"
			value = "other"
			label = "Other"
			drilldown = nil
		}
		groupsByKey := bucketGroups[bucket]
		if groupsByKey == nil {
			groupsByKey = make(map[string]*usageAnalyticsAccumulator)
			bucketGroups[bucket] = groupsByKey
		}
		acc := groupsByKey[key]
		if acc == nil {
			acc = usageAnalyticsNewAccumulator(query.GroupBy, key, value, label, drilldown)
			groupsByKey[key] = acc
		}
		usageAnalyticsAddLog(acc, log)
	}
	points := make([]UsageAnalyticsTimeseriesPoint, 0)
	for bucket, groupsByKey := range bucketGroups {
		for key, acc := range groupsByKey {
			usageAnalyticsFinalizeAccumulator(acc)
			if key != "other" && query.GroupBy == UsageAnalyticsGroupByToken {
				if global := globalGroups[key]; global != nil {
					acc.GroupLabel = global.GroupLabel
					acc.Token = global.Token
				}
			}
			point := UsageAnalyticsTimeseriesPoint{Timestamp: bucket, TimeLabel: usageAnalyticsTimeLabel(bucket, query.Granularity), UsageAnalyticsGroup: acc.UsageAnalyticsGroup}
			points = append(points, point)
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
	logs, err := usageAnalyticsLoadCandidateLogs(query, true)
	if err != nil {
		return UsageAnalyticsBreakdownResponse{}, err
	}
	_, groups := usageAnalyticsAggregate(logs, query.GroupBy)
	usageAnalyticsFinalizeAccumulators(groups)
	usageAnalyticsAttachTokenInfo(query.UserID, groups)
	usageAnalyticsApplyShares(groups, query.SortBy)
	usageAnalyticsSortGroups(groups, query.SortBy, query.SortOrder)
	totalGroups := len(groups)
	ordered := usageAnalyticsOrderedAccumulators(groups)
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

func usageAnalyticsLoadCandidateLogs(query UsageAnalyticsQuery, useQueryTime bool) ([]Log, error) {
	if _, ok := usageAnalyticsGroupExpr(query.GroupBy); !ok {
		return nil, ErrUsageAnalyticsInvalidGroup
	}
	if err := usageAnalyticsValidateTokenIDs(query); err != nil {
		return nil, err
	}
	logs := make([]Log, 0)
	db := usageAnalyticsBaseLogQuery(LOG_DB, query, useQueryTime).Order("created_at asc").Limit(usageAnalyticsCandidateLimit + 1)
	if err := db.Find(&logs).Error; err != nil {
		return nil, err
	}
	if len(logs) > usageAnalyticsCandidateLimit {
		return nil, ErrUsageAnalyticsTooManyLogs
	}
	return logs, nil
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
	if len(query.Groups) > 0 {
		q = q.Where(logGroupCol+" IN ?", query.Groups)
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

func usageAnalyticsAggregate(logs []Log, groupBy UsageAnalyticsGroupBy) (*usageAnalyticsAccumulator, map[string]*usageAnalyticsAccumulator) {
	total := usageAnalyticsNewAccumulator(groupBy, "total", "total", "Total", nil)
	groups := make(map[string]*usageAnalyticsAccumulator)
	for i := range logs {
		log := logs[i]
		usageAnalyticsAddLog(total, log)
		key, value, label, drilldown := usageAnalyticsDimension(groupBy, log)
		acc := groups[key]
		if acc == nil {
			acc = usageAnalyticsNewAccumulator(groupBy, key, value, label, drilldown)
			groups[key] = acc
		}
		usageAnalyticsAddLog(acc, log)
	}
	return total, groups
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
	acc.latencySamples = append(acc.latencySamples, usageAnalyticsLogUseTime(log))
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
	dst.latencySamples = append(dst.latencySamples, src.latencySamples...)
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
	}
	if len(acc.latencySamples) > 0 {
		sum := 0
		for _, sample := range acc.latencySamples {
			sum += sample
		}
		acc.AvgLatencyMs = sum * 1000 / len(acc.latencySamples)
		acc.P95LatencyMs = usageAnalyticsP95LatencyMs(acc.latencySamples)
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

func usageAnalyticsP95LatencyMs(samples []int) int {
	if len(samples) == 0 {
		return 0
	}
	copySamples := append([]int(nil), samples...)
	sort.Ints(copySamples)
	index := int(math.Ceil(0.95*float64(len(copySamples)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(copySamples) {
		index = len(copySamples) - 1
	}
	return copySamples[index] * 1000
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
	case UsageAnalyticsGroupByGroup:
		value := log.Group
		label := value
		if label == "" {
			label = "Unknown Group"
		}
		return "group:" + value, value, label, &UsageAnalyticsDrilldown{Group: &value}
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
	case UsageAnalyticsGroupByGroup:
		return logGroupCol, true
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
			group := token.Group
			remainQuota := token.RemainQuota
			unlimitedQuota := token.UnlimitedQuota
			acc.GroupLabel = token.Name
			acc.Token = &UsageAnalyticsTokenInfo{ID: token.Id, Name: token.Name, MaskedKey: &maskedKey, Status: &status, Group: &group, RemainQuota: &remainQuota, UnlimitedQuota: &unlimitedQuota, Deleted: false}
			continue
		}
		label := acc.tokenName
		if label == "" {
			label = acc.GroupLabel
		}
		acc.GroupLabel = label
		acc.Token = &UsageAnalyticsTokenInfo{ID: id, Name: label, MaskedKey: nil, Status: nil, Group: nil, RemainQuota: nil, UnlimitedQuota: nil, Deleted: true}
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
	case UsageAnalyticsMetricFirstUsedAt:
		return int(group.FirstUsedAt)
	case UsageAnalyticsMetricLastUsedAt:
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

func usageAnalyticsLimitGroups(groups map[string]*usageAnalyticsAccumulator, limit int) map[string]*usageAnalyticsAccumulator {
	ordered := usageAnalyticsOrderedAccumulators(groups)
	if limit > len(ordered) {
		limit = len(ordered)
	}
	limited := make(map[string]*usageAnalyticsAccumulator, limit)
	for _, acc := range ordered[:limit] {
		limited[acc.GroupKey] = acc
	}
	return limited
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

func usageAnalyticsTopKeySet(groups map[string]*usageAnalyticsAccumulator, limit int) map[string]bool {
	ordered := usageAnalyticsOrderedAccumulators(groups)
	if limit > len(ordered) {
		limit = len(ordered)
	}
	keys := make(map[string]bool, limit)
	for _, acc := range ordered[:limit] {
		keys[acc.GroupKey] = true
	}
	return keys
}

func usageAnalyticsRPMAndTPM(logs []Log) (int, int) {
	rpm := len(logs)
	tpm := 0
	for i := range logs {
		tpm += usageAnalyticsLogTokens(logs[i])
	}
	return rpm, tpm
}

func usageAnalyticsActiveKeyCount(logs []Log) int {
	ids := make(map[int]bool)
	for _, log := range logs {
		if log.TokenId > 0 {
			ids[log.TokenId] = true
		}
	}
	return len(ids)
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
