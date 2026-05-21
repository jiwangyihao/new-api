package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const adminAnalyticsCandidateLogLimit = 100000

var (
	ErrAdminAnalyticsTooManyLogs    = errors.New("admin analytics candidate logs exceed limit")
	ErrAdminAnalyticsInvalidGroupBy = errors.New("invalid group_by")
	ErrAdminAnalyticsInvalidMetric  = errors.New("invalid metric")
	ErrAdminAnalyticsInvalidSortBy  = errors.New("unsupported sort_by")
)

type AdminAnalyticsUsageQuery struct {
	AdminAnalyticsQuery
	GroupBy         dto.AdminUsageGroupBy
	Metric          dto.AdminUsageMetric
	SortMetric      dto.AdminUsageMetric
	PlanAttribution dto.AdminPlanAttribution
	TopN            int
	SortByProvided  bool
}

type adminUsageCandidateLog struct {
	Log
	BillingSource              string
	SubscriptionSource         string
	Endpoint                   string
	RequestGroup               string
	OtherSubscriptionID        int
	OtherSubscriptionPlanID    int
	SubscriptionTokensConsumed *int64
	AttributedPlanID           int
	AttributedPlanTitle        string
	AttributedSubscriptionID   int
	AttributedSource           dto.AdminAnalyticsSource
}

type adminUsageAccumulator struct {
	dto.AdminUsageGroup
	latencySamples []int
	userIDs        map[int]struct{}
	tokenIDs       map[int]struct{}
}

func normalizeAdminUsageQuery(query AdminAnalyticsUsageQuery) AdminAnalyticsUsageQuery {
	query.AdminAnalyticsQuery = normalizeAdminAnalyticsQuery(query.AdminAnalyticsQuery)
	if query.GroupBy == "" {
		query.GroupBy = dto.AdminUsageGroupByUser
	}
	if query.Metric == "" {
		query.Metric = dto.AdminUsageMetricTotalTokens
	}
	if query.SortMetric == "" {
		query.SortMetric = query.Metric
	}
	if query.PlanAttribution == "" {
		query.PlanAttribution = dto.AdminPlanAttributionCurrent
	}
	if query.TopN <= 0 {
		query.TopN = query.Limit
	}
	if query.TopN <= 0 {
		query.TopN = AdminAnalyticsDefaultLimit
	} else if query.TopN > AdminAnalyticsMaxLimit {
		query.TopN = AdminAnalyticsMaxLimit
	}
	if query.Limit <= 0 {
		query.Limit = query.TopN
	}
	return query
}

func ValidateAdminUsageQuery(query AdminAnalyticsUsageQuery, endpoint string) error {
	switch query.GroupBy {
	case dto.AdminUsageGroupByUser, dto.AdminUsageGroupByPlan, dto.AdminUsageGroupByModel, dto.AdminUsageGroupByUserGroup, dto.AdminUsageGroupByRequestGroup, dto.AdminUsageGroupByStream, dto.AdminUsageGroupByStatus, dto.AdminUsageGroupByChannel, dto.AdminUsageGroupByEndpoint, dto.AdminUsageGroupByBillingSource, dto.AdminUsageGroupByToken, dto.AdminUsageGroupBySubscriptionSource:
	default:
		return ErrAdminAnalyticsInvalidGroupBy
	}
	switch query.Metric {
	case dto.AdminUsageMetricRequestCount, dto.AdminUsageMetricTotalTokens, dto.AdminUsageMetricQuota, dto.AdminUsageMetricErrorRate, dto.AdminUsageMetricAvgLatencyMs, dto.AdminUsageMetricP95LatencyMs, dto.AdminUsageMetricActiveUsers, dto.AdminUsageMetricActiveAPIKeys:
	default:
		return ErrAdminAnalyticsInvalidMetric
	}
	switch query.SortMetric {
	case dto.AdminUsageMetricRequestCount, dto.AdminUsageMetricTotalTokens, dto.AdminUsageMetricQuota, dto.AdminUsageMetricErrorRate, dto.AdminUsageMetricAvgLatencyMs, dto.AdminUsageMetricP95LatencyMs, dto.AdminUsageMetricActiveUsers, dto.AdminUsageMetricActiveAPIKeys:
	default:
		return ErrAdminAnalyticsInvalidSortBy
	}
	switch query.PlanAttribution {
	case dto.AdminPlanAttributionCurrent, dto.AdminPlanAttributionEventTime:
	default:
		return errors.New("invalid plan_attribution")
	}
	if endpoint == "timeseries" && query.SortByProvided {
		return ErrAdminAnalyticsInvalidSortBy
	}
	return nil
}

func loadAdminUsageCandidateLogs(query AdminAnalyticsUsageQuery) ([]adminUsageCandidateLog, []dto.AdminAnalyticsAvailabilityWarning, error) {
	query = normalizeAdminUsageQuery(query)
	base := LOG_DB.Model(&Log{}).Where("type IN ?", []int{LogTypeConsume, LogTypeError}).Where("created_at >= ? AND created_at <= ?", query.StartTimestamp, query.EndTimestamp)
	if len(query.RequestGroups) > 0 {
		base = base.Where(logGroupCol+" IN ?", query.RequestGroups)
	}
	if len(query.UserIDs) > 0 {
		base = base.Where("user_id IN ?", query.UserIDs)
	}
	if len(query.TokenIDs) > 0 {
		base = base.Where("token_id IN ?", query.TokenIDs)
	}
	if len(query.ChannelIDs) > 0 {
		base = base.Where("channel_id IN ?", query.ChannelIDs)
	}
	if len(query.LogStatuses) > 0 {
		hasSuccess := adminStringInSet("success", query.LogStatuses)
		hasError := adminStringInSet("error", query.LogStatuses)
		if hasSuccess && !hasError {
			base = base.Where("type = ?", LogTypeConsume)
		} else if hasError && !hasSuccess {
			base = base.Where("type = ?", LogTypeError)
		}
	}
	var logs []Log
	if err := base.Order("created_at asc").Limit(adminAnalyticsCandidateLogLimit + 1).Find(&logs).Error; err != nil {
		return nil, nil, err
	}
	if len(logs) > adminAnalyticsCandidateLogLimit {
		warning := dto.AdminAnalyticsAvailabilityWarning{Section: "usage", Reason: "candidate_limit_exceeded", Message: "candidate log limit exceeded"}
		return nil, []dto.AdminAnalyticsAvailabilityWarning{warning}, ErrAdminAnalyticsTooManyLogs
	}
	candidates := make([]adminUsageCandidateLog, 0, len(logs))
	for i := range logs {
		candidate := adminUsageCandidateLog{Log: logs[i]}
		parseAdminUsageOther(&candidate)
		candidates = append(candidates, candidate)
	}
	return candidates, nil, nil
}

func parseAdminUsageOther(log *adminUsageCandidateLog) {
	if strings.TrimSpace(log.Other) == "" {
		return
	}
	var other map[string]any
	if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
		return
	}
	log.BillingSource = adminStringFromAny(other["billing_source"])
	log.SubscriptionSource = adminStringFromAny(other["subscription_source"])
	log.Endpoint = adminStringFromAny(other["endpoint"])
	if log.Endpoint == "" {
		log.Endpoint = adminStringFromAny(other["request_path"])
	}
	log.RequestGroup = adminStringFromAny(other["request_group"])
	if log.RequestGroup == "" {
		log.RequestGroup = log.Group
	}
	log.OtherSubscriptionID = adminIntFromAny(other["subscription_id"])
	log.OtherSubscriptionPlanID = adminIntFromAny(other["subscription_plan_id"])
	if value, ok := adminOptionalInt64FromAny(other["subscription_tokens_consumed"]); ok {
		log.SubscriptionTokensConsumed = &value
	}
}

func adminStringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func adminIntFromAny(value any) int {
	if value == nil {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(v))
		return parsed
	default:
		return 0
	}
}

func adminOptionalInt64FromAny(value any) (int64, bool) {
	if value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func adminUsageLogTokens(log adminUsageCandidateLog) int64 {
	if log.Type == LogTypeError {
		return 0
	}
	if log.SubscriptionTokensConsumed != nil && log.BillingSource == "subscription" {
		if *log.SubscriptionTokensConsumed < 0 {
			return 0
		}
		return *log.SubscriptionTokensConsumed
	}
	if log.MeteredTokens != nil {
		if *log.MeteredTokens < 0 {
			return 0
		}
		return int64(*log.MeteredTokens)
	}
	total := log.PromptTokens + log.CompletionTokens
	if total < 0 {
		return 0
	}
	return int64(total)
}

func enrichAdminUsageWithCurrentPlans(logs []adminUsageCandidateLog, snapshotAt int64) ([]adminUsageCandidateLog, []dto.AdminAnalyticsAvailabilityWarning, error) {
	userIDs := make([]int, 0, len(logs))
	for i := range logs {
		userIDs = append(userIDs, logs[i].UserId)
	}
	activeRows, err := loadAdminActiveSubscriptions(AdminAnalyticsQuery{SnapshotAt: snapshotAt, Limit: AdminAnalyticsMaxLimit})
	if err != nil {
		return nil, nil, err
	}
	byUser := make(map[int]adminActiveSubscriptionRow, len(userIDs))
	for i := range activeRows {
		if _, ok := byUser[activeRows[i].Subscription.UserId]; !ok {
			byUser[activeRows[i].Subscription.UserId] = activeRows[i]
		}
	}
	for i := range logs {
		if row, ok := byUser[logs[i].UserId]; ok {
			logs[i].AttributedPlanID = row.Subscription.PlanId
			logs[i].AttributedPlanTitle = row.Plan.Title
			logs[i].AttributedSubscriptionID = row.Subscription.Id
			logs[i].AttributedSource = row.Source
		}
	}
	return logs, nil, nil
}

func enrichAdminUsageWithEventTimePlans(logs []adminUsageCandidateLog) ([]adminUsageCandidateLog, []dto.AdminAnalyticsAvailabilityWarning, error) {
	warnings := make([]dto.AdminAnalyticsAvailabilityWarning, 0)
	subIDs := make([]int, 0, len(logs))
	for i := range logs {
		if logs[i].OtherSubscriptionID > 0 {
			subIDs = append(subIDs, logs[i].OtherSubscriptionID)
		}
	}
	subsByID := map[int]UserSubscription{}
	if len(subIDs) > 0 {
		var subs []UserSubscription
		if err := DB.Where("id IN ?", adminUniquePositiveInts(subIDs)).Find(&subs).Error; err != nil {
			return nil, nil, err
		}
		for i := range subs {
			subsByID[subs[i].Id] = subs[i]
		}
	}
	for i := range logs {
		var sub UserSubscription
		found := false
		if logs[i].OtherSubscriptionID > 0 {
			sub, found = subsByID[logs[i].OtherSubscriptionID]
		}
		if !found && logs[i].UserId > 0 {
			var matches []UserSubscription
			if err := DB.Where("user_id = ? AND start_time <= ? AND end_time > ?", logs[i].UserId, logs[i].CreatedAt, logs[i].CreatedAt).Find(&matches).Error; err != nil {
				return nil, nil, err
			}
			if len(matches) == 1 {
				sub = matches[0]
				found = true
			} else if len(matches) > 1 {
				warnings = append(warnings, dto.AdminAnalyticsAvailabilityWarning{Section: "usage", Reason: "insufficient_history", Message: "ambiguous subscription history"})
			}
		}
		if found {
			logs[i].AttributedPlanID = sub.PlanId
			logs[i].AttributedSubscriptionID = sub.Id
			logs[i].AttributedSource = normalizeAdminSubscriptionSource(sub.GrantReason, sub.Source)
		}
	}
	planIDs := make([]int, 0, len(logs))
	for i := range logs {
		if logs[i].AttributedPlanID > 0 {
			planIDs = append(planIDs, logs[i].AttributedPlanID)
		}
	}
	plans, err := adminPlansByID(planIDs)
	if err != nil {
		return nil, nil, err
	}
	for i := range logs {
		logs[i].AttributedPlanTitle = plans[logs[i].AttributedPlanID].Title
	}
	return logs, warnings, nil
}

func GetAdminUsageConsumptionSummary(query AdminAnalyticsUsageQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminUsageConsumptionSummaryResponse], error) {
	query = normalizeAdminUsageQuery(query)
	if err := ValidateAdminUsageQuery(query, "summary"); err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminUsageConsumptionSummaryResponse]{}, err
	}
	logs, warnings, err := loadAndEnrichAdminUsageLogs(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminUsageConsumptionSummaryResponse]{Warnings: warnings}, err
	}
	total, groups := aggregateAdminUsage(logs, query.GroupBy)
	ordered := adminUsageOrderedGroups(groups, query.SortMetric, query.SortOrder)
	limited, other := adminUsageTopNWithOther(ordered, query.TopN)
	paged, page := paginateAdminAnalyticsList(limited, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminUsageConsumptionSummaryResponse]{Range: adminAnalyticsRangeMeta(query.AdminAnalyticsQuery), Data: dto.AdminUsageConsumptionSummaryResponse{Total: total, Groups: dto.AdminAnalyticsList[dto.AdminUsageGroup]{Items: paged, Page: page, SortBy: string(query.SortMetric), SortOrder: query.SortOrder}, GroupBy: query.GroupBy, Other: other}, Warnings: warnings}, nil
}

func GetAdminUsageConsumptionTimeseries(query AdminAnalyticsUsageQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminUsageTimeseriesResponse], error) {
	query = normalizeAdminUsageQuery(query)
	if err := ValidateAdminUsageQuery(query, "timeseries"); err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminUsageTimeseriesResponse]{}, err
	}
	logs, warnings, err := loadAndEnrichAdminUsageLogs(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminUsageTimeseriesResponse]{Warnings: warnings}, err
	}
	step := adminAnalyticsStepSeconds(query.Granularity)
	if step <= 0 {
		step = 24 * 60 * 60
	}
	buckets := map[string]*adminUsageAccumulator{}
	for i := range logs {
		bucket := query.StartTimestamp + ((logs[i].CreatedAt - query.StartTimestamp) / step * step)
		key, value, label, drilldown := adminUsageDimension(query.GroupBy, logs[i])
		bucketKey := fmt.Sprintf("%d:%s", bucket, key)
		acc := buckets[bucketKey]
		if acc == nil {
			acc = newAdminUsageAccumulator(query.GroupBy, key, value, label, drilldown)
			buckets[bucketKey] = acc
		}
		adminUsageAddLog(acc, logs[i])
	}
	points := make([]dto.AdminUsageTimeseriesPoint, 0, len(buckets))
	for key, acc := range buckets {
		adminUsageFinalize(acc)
		parts := strings.SplitN(key, ":", 2)
		timestamp, _ := strconv.ParseInt(parts[0], 10, 64)
		points = append(points, dto.AdminUsageTimeseriesPoint{Timestamp: timestamp, TimeLabel: adminUsageTimeLabel(timestamp, query.Granularity), AdminUsageGroup: acc.AdminUsageGroup})
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].Timestamp == points[j].Timestamp {
			return points[i].GroupKey < points[j].GroupKey
		}
		return points[i].Timestamp < points[j].Timestamp
	})
	return dto.AdminAnalyticsPanelResponse[dto.AdminUsageTimeseriesResponse]{Range: adminAnalyticsRangeMeta(query.AdminAnalyticsQuery), Data: dto.AdminUsageTimeseriesResponse{Points: points, Granularity: query.Granularity, GroupBy: query.GroupBy}, Warnings: warnings}, nil
}

func GetAdminUsageConsumptionBreakdown(query AdminAnalyticsUsageQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminUsageBreakdownResponse], error) {
	query = normalizeAdminUsageQuery(query)
	if err := ValidateAdminUsageQuery(query, "breakdown"); err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminUsageBreakdownResponse]{}, err
	}
	logs, warnings, err := loadAndEnrichAdminUsageLogs(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminUsageBreakdownResponse]{Warnings: warnings}, err
	}
	_, groups := aggregateAdminUsage(logs, query.GroupBy)
	ordered := adminUsageOrderedGroups(groups, query.SortMetric, query.SortOrder)
	limited, other := adminUsageTopNWithOther(ordered, query.TopN)
	paged, page := paginateAdminAnalyticsList(limited, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminUsageBreakdownResponse]{Range: adminAnalyticsRangeMeta(query.AdminAnalyticsQuery), Data: dto.AdminUsageBreakdownResponse{Groups: dto.AdminAnalyticsList[dto.AdminUsageGroup]{Items: paged, Page: page, SortBy: string(query.SortMetric), SortOrder: query.SortOrder}, GroupBy: query.GroupBy, Other: other}, Warnings: warnings}, nil
}

func loadAndEnrichAdminUsageLogs(query AdminAnalyticsUsageQuery) ([]adminUsageCandidateLog, []dto.AdminAnalyticsAvailabilityWarning, error) {
	logs, warnings, err := loadAdminUsageCandidateLogs(query)
	if err != nil {
		return nil, warnings, err
	}
	var attributionWarnings []dto.AdminAnalyticsAvailabilityWarning
	if query.PlanAttribution == dto.AdminPlanAttributionEventTime {
		logs, attributionWarnings, err = enrichAdminUsageWithEventTimePlans(logs)
	} else {
		logs, attributionWarnings, err = enrichAdminUsageWithCurrentPlans(logs, query.SnapshotAt)
	}
	warnings = append(warnings, attributionWarnings...)
	return logs, warnings, err
}

func aggregateAdminUsage(logs []adminUsageCandidateLog, groupBy dto.AdminUsageGroupBy) (dto.AdminUsageMetrics, []dto.AdminUsageGroup) {
	total := newAdminUsageAccumulator(groupBy, "total", "total", "Total", nil)
	groups := map[string]*adminUsageAccumulator{}
	for i := range logs {
		key, value, label, drilldown := adminUsageDimension(groupBy, logs[i])
		acc := groups[key]
		if acc == nil {
			acc = newAdminUsageAccumulator(groupBy, key, value, label, drilldown)
			groups[key] = acc
		}
		adminUsageAddLog(total, logs[i])
		adminUsageAddLog(acc, logs[i])
	}
	adminUsageFinalize(total)
	result := make([]dto.AdminUsageGroup, 0, len(groups))
	for _, acc := range groups {
		adminUsageFinalize(acc)
		result = append(result, acc.AdminUsageGroup)
	}
	return adminUsageMetrics(total.AdminUsageGroup), result
}

func newAdminUsageAccumulator(groupBy dto.AdminUsageGroupBy, key string, value string, label string, drilldown *dto.AdminAnalyticsDrilldownTarget) *adminUsageAccumulator {
	return &adminUsageAccumulator{AdminUsageGroup: dto.AdminUsageGroup{GroupBy: groupBy, GroupKey: key, GroupValue: value, GroupLabel: label, Drilldown: drilldown}, userIDs: map[int]struct{}{}, tokenIDs: map[int]struct{}{}}
}

func adminUsageAddLog(acc *adminUsageAccumulator, log adminUsageCandidateLog) {
	acc.RequestCount++
	if log.Type == LogTypeError {
		acc.ErrorCount++
	} else {
		acc.SuccessCount++
		acc.Quota += int64(log.Quota)
		acc.PromptTokens += int64(log.PromptTokens)
		acc.CompletionTokens += int64(log.CompletionTokens)
		acc.MeteredTokens += adminUsageLogTokens(log)
		acc.TotalTokens += adminUsageLogTokens(log)
	}
	if log.UseTime > 0 {
		acc.latencySamples = append(acc.latencySamples, log.UseTime*1000)
	}
	if log.UserId > 0 {
		acc.userIDs[log.UserId] = struct{}{}
	}
	if log.TokenId > 0 {
		acc.tokenIDs[log.TokenId] = struct{}{}
	}
	if acc.FirstUsedAt == 0 || log.CreatedAt < acc.FirstUsedAt {
		acc.FirstUsedAt = log.CreatedAt
	}
	if log.CreatedAt > acc.LastUsedAt {
		acc.LastUsedAt = log.CreatedAt
	}
}

func adminUsageFinalize(acc *adminUsageAccumulator) {
	if acc.RequestCount > 0 {
		acc.SuccessRate = float64(acc.SuccessCount) / float64(acc.RequestCount)
		acc.ErrorRate = float64(acc.ErrorCount) / float64(acc.RequestCount)
	}
	if len(acc.latencySamples) > 0 {
		sort.Ints(acc.latencySamples)
		sum := 0
		for _, sample := range acc.latencySamples {
			sum += sample
		}
		acc.AvgLatencyMs = sum / len(acc.latencySamples)
		idx := int(math.Ceil(float64(len(acc.latencySamples))*0.95)) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(acc.latencySamples) {
			idx = len(acc.latencySamples) - 1
		}
		acc.P95LatencyMs = acc.latencySamples[idx]
	}
	acc.ActiveUsers = len(acc.userIDs)
	acc.ActiveAPIKeys = len(acc.tokenIDs)
}

func adminUsageMetrics(group dto.AdminUsageGroup) dto.AdminUsageMetrics {
	return dto.AdminUsageMetrics{RequestCount: group.RequestCount, SuccessCount: group.SuccessCount, ErrorCount: group.ErrorCount, SuccessRate: group.SuccessRate, ErrorRate: group.ErrorRate, Quota: group.Quota, PromptTokens: group.PromptTokens, CompletionTokens: group.CompletionTokens, MeteredTokens: group.MeteredTokens, TotalTokens: group.TotalTokens, AvgLatencyMs: group.AvgLatencyMs, P95LatencyMs: group.P95LatencyMs, Rpm: group.Rpm, Tpm: group.Tpm, ActiveUsers: group.ActiveUsers, ActiveAPIKeys: group.ActiveAPIKeys, FirstUsedAt: group.FirstUsedAt, LastUsedAt: group.LastUsedAt}
}

func adminUsageOrderedGroups(groups []dto.AdminUsageGroup, metric dto.AdminUsageMetric, order dto.AdminAnalyticsSortOrder) []dto.AdminUsageGroup {
	ordered := append([]dto.AdminUsageGroup(nil), groups...)
	desc := order != dto.AdminAnalyticsSortAsc
	sort.Slice(ordered, func(i, j int) bool {
		less := adminUsageMetricValue(ordered[i], metric) < adminUsageMetricValue(ordered[j], metric)
		if desc {
			return !less
		}
		return less
	})
	return ordered
}

func adminUsageMetricValue(group dto.AdminUsageGroup, metric dto.AdminUsageMetric) float64 {
	switch metric {
	case dto.AdminUsageMetricRequestCount:
		return float64(group.RequestCount)
	case dto.AdminUsageMetricQuota:
		return float64(group.Quota)
	case dto.AdminUsageMetricErrorRate:
		return group.ErrorRate
	case dto.AdminUsageMetricAvgLatencyMs:
		return float64(group.AvgLatencyMs)
	case dto.AdminUsageMetricP95LatencyMs:
		return float64(group.P95LatencyMs)
	case dto.AdminUsageMetricActiveUsers:
		return float64(group.ActiveUsers)
	case dto.AdminUsageMetricActiveAPIKeys:
		return float64(group.ActiveAPIKeys)
	default:
		return float64(group.TotalTokens)
	}
}

func adminUsageTopNWithOther(ordered []dto.AdminUsageGroup, topN int) ([]dto.AdminUsageGroup, *dto.AdminUsageGroup) {
	if topN <= 0 || topN >= len(ordered) {
		return ordered, nil
	}
	limited := append([]dto.AdminUsageGroup(nil), ordered[:topN]...)
	otherAcc := newAdminUsageAccumulator(ordered[0].GroupBy, "__other__", "__other__", "Other", nil)
	for _, group := range ordered[topN:] {
		otherAcc.RequestCount += group.RequestCount
		otherAcc.SuccessCount += group.SuccessCount
		otherAcc.ErrorCount += group.ErrorCount
		otherAcc.Quota += group.Quota
		otherAcc.PromptTokens += group.PromptTokens
		otherAcc.CompletionTokens += group.CompletionTokens
		otherAcc.MeteredTokens += group.MeteredTokens
		otherAcc.TotalTokens += group.TotalTokens
	}
	adminUsageFinalize(otherAcc)
	return limited, &otherAcc.AdminUsageGroup
}

func adminUsageDimension(groupBy dto.AdminUsageGroupBy, log adminUsageCandidateLog) (string, string, string, *dto.AdminAnalyticsDrilldownTarget) {
	switch groupBy {
	case dto.AdminUsageGroupByPlan:
		if log.AttributedPlanID > 0 {
			planID := log.AttributedPlanID
			return fmt.Sprintf("plan:%d", planID), strconv.Itoa(planID), log.AttributedPlanTitle, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_subscriptions", PlanID: &planID}
		}
		return "plan:unknown", "unknown", "Unknown", nil
	case dto.AdminUsageGroupByModel:
		return "model:" + log.ModelName, log.ModelName, log.ModelName, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs", Model: log.ModelName}
	case dto.AdminUsageGroupByUserGroup:
		return "user_group:" + log.Group, log.Group, log.Group, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_users", UserGroup: log.Group}
	case dto.AdminUsageGroupByRequestGroup:
		group := log.RequestGroup
		if group == "" {
			group = log.Group
		}
		return "request_group:" + group, group, group, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs", RequestGroup: group}
	case dto.AdminUsageGroupByStream:
		value := strconv.FormatBool(log.IsStream)
		return "stream:" + value, value, value, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs"}
	case dto.AdminUsageGroupByStatus:
		status := "success"
		if log.Type == LogTypeError {
			status = "error"
		}
		return "status:" + status, status, status, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs", Status: status}
	case dto.AdminUsageGroupByChannel:
		channelID := log.ChannelId
		return fmt.Sprintf("channel:%d", channelID), strconv.Itoa(channelID), strconv.Itoa(channelID), &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs", ChannelID: &channelID}
	case dto.AdminUsageGroupByEndpoint:
		endpoint := log.Endpoint
		if endpoint == "" {
			endpoint = "unknown"
		}
		return "endpoint:" + endpoint, endpoint, endpoint, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs"}
	case dto.AdminUsageGroupByBillingSource:
		source := log.BillingSource
		if source == "" {
			source = "unknown"
		}
		return "billing_source:" + source, source, source, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs"}
	case dto.AdminUsageGroupByToken:
		tokenID := log.TokenId
		return fmt.Sprintf("token:%d", tokenID), strconv.Itoa(tokenID), log.TokenName, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs", TokenID: &tokenID}
	case dto.AdminUsageGroupBySubscriptionSource:
		source := string(log.AttributedSource)
		if source == "" {
			source = log.SubscriptionSource
		}
		if source == "" {
			source = "unknown"
		}
		return "subscription_source:" + source, source, source, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs"}
	default:
		userID := log.UserId
		label := log.Username
		if label == "" {
			label = strconv.Itoa(userID)
		}
		return fmt.Sprintf("user:%d", userID), strconv.Itoa(userID), label, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_users", UserID: &userID, Username: log.Username}
	}
}

func adminAnalyticsStepSeconds(granularity dto.AdminAnalyticsGranularity) int64 {
	switch granularity {
	case dto.AdminAnalyticsGranularityHour:
		return 3600
	case dto.AdminAnalyticsGranularityWeek:
		return 7 * 24 * 3600
	case dto.AdminAnalyticsGranularityMonth:
		return 30 * 24 * 3600
	default:
		return 24 * 3600
	}
}

func adminUsageTimeLabel(timestamp int64, granularity dto.AdminAnalyticsGranularity) string {
	layout := "2006-01-02"
	if granularity == dto.AdminAnalyticsGranularityHour {
		layout = "2006-01-02 15:00"
	}
	return time.Unix(timestamp, 0).UTC().Format(layout)
}
