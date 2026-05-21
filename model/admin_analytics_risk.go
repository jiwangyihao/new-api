package model

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/dto"
)

const adminRiskHighErrorMinRequests = 20
const adminRiskHighErrorRate = 0.20
const adminRiskManyFailedRequests = 50
const adminRiskInviteMinDirect = 20
const adminRiskInviteQualifiedRate = 0.10
const adminRiskRewardNeverUsedMinAgeSeconds = 7 * 24 * 3600
const adminRiskResetOverdueGraceSeconds = 24 * 3600
const adminRiskHighExhaustionRate = 0.90
const adminRiskUnderusedPlanAgeRate = 0.50
const adminRiskUnderusedUsageRate = 0.10
const adminRiskResetPressureSeconds = 3 * 24 * 3600
const adminRiskResetPressureUsageRate = 0.90
const adminRiskAbnormalStreamRatio = 0.95
const adminRiskManyModels = 5

type adminRiskListBuckets struct {
	plan       []dto.AdminAnalyticsRiskItem
	user       []dto.AdminAnalyticsRiskItem
	invitation []dto.AdminAnalyticsRiskItem
	system     []dto.AdminAnalyticsRiskItem
}

func GetAdminAnalyticsRisks(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsRisksResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	rows, err := loadAdminActiveSubscriptions(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsRisksResponse]{}, err
	}
	buckets := adminRiskListBuckets{}
	buckets.addSubscriptionRisks(rows, query)
	if err := buckets.addInvitationRisks(query); err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsRisksResponse]{}, err
	}
	warnings, err := buckets.addUsageRisks(query)
	if err != nil && err != ErrAdminAnalyticsTooManyLogs {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsRisksResponse]{}, err
	}
	data := dto.AdminAnalyticsRisksResponse{
		PlanRisks:       adminRiskList(buckets.plan, query, dto.AdminAnalyticsRiskCategoryPlan),
		UserRisks:       adminRiskList(buckets.user, query, dto.AdminAnalyticsRiskCategoryUser),
		InvitationRisks: adminRiskList(buckets.invitation, query, dto.AdminAnalyticsRiskCategoryInvitation),
		SystemRisks:     adminRiskList(buckets.system, query, dto.AdminAnalyticsRiskCategorySystem),
	}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsRisksResponse]{Range: adminAnalyticsRangeMeta(query), Data: data, Warnings: warnings}, nil
}

func (b *adminRiskListBuckets) addSubscriptionRisks(rows []adminActiveSubscriptionRow, query AdminAnalyticsQuery) {
	activeByUser := map[int][]adminActiveSubscriptionRow{}
	for i := range rows {
		row := rows[i]
		activeByUser[row.Subscription.UserId] = append(activeByUser[row.Subscription.UserId], row)
		userID := row.Subscription.UserId
		planID := row.Subscription.PlanId
		subID := row.Subscription.Id
		drilldown := &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_subscriptions", UserID: &userID, PlanID: &planID, Status: "active"}
		if row.Quota.UsageRate != nil && *row.Quota.UsageRate >= adminRiskHighExhaustionRate && *row.Quota.UsageRate <= 1 {
			b.plan = append(b.plan, newAdminRisk("high_exhaustion_risk", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategoryPlan, "usage_rate >= 90%", 1, *row.Quota.UsageRate, drilldown))
		}
		if row.Quota.UsageRate != nil && *row.Quota.UsageRate > 1 {
			b.plan = append(b.plan, newAdminRisk("overused_subscription", dto.AdminAnalyticsRiskSeverityCritical, dto.AdminAnalyticsRiskCategoryPlan, "usage_rate > 100%", 1, *row.Quota.UsageRate, drilldown))
		}
		if row.Subscription.TokenLimit == 0 && !row.Quota.TokenUnlimited {
			b.plan = append(b.plan, newAdminRisk("zero_limit_active_subscription", dto.AdminAnalyticsRiskSeverityCritical, dto.AdminAnalyticsRiskCategoryPlan, "token_limit = 0", 1, 0, drilldown))
		}
		if row.Quota.SystemRisk {
			b.system = append(b.system, newAdminRisk("invalid_negative_token_quota", dto.AdminAnalyticsRiskSeverityCritical, dto.AdminAnalyticsRiskCategorySystem, "negative token quota", 1, float64(row.Subscription.TokenLimit), drilldown))
		}
		if row.Subscription.NextResetTime > 0 && row.Subscription.NextResetTime <= query.SnapshotAt-adminRiskResetOverdueGraceSeconds && row.Subscription.TokenLimit > 0 && row.Subscription.TokenUsed > 0 {
			b.plan = append(b.plan, newAdminRisk("reset_overdue", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategoryPlan, "next_reset_time overdue by 24h", 1, float64(row.Subscription.NextResetTime), drilldown))
		}
		if row.Subscription.NextResetTime > query.SnapshotAt && row.Subscription.NextResetTime-query.SnapshotAt <= adminRiskResetPressureSeconds && row.Quota.UsageRate != nil && *row.Quota.UsageRate >= adminRiskResetPressureUsageRate {
			b.plan = append(b.plan, newAdminRisk("reset_pressure", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategoryPlan, "reset soon and usage high", 1, *row.Quota.UsageRate, drilldown))
		}
		age := query.SnapshotAt - row.Subscription.StartTime
		duration := row.Subscription.EndTime - row.Subscription.StartTime
		if duration > 0 && float64(age)/float64(duration) >= adminRiskUnderusedPlanAgeRate && row.Quota.UsageRate != nil && *row.Quota.UsageRate < adminRiskUnderusedUsageRate {
			b.user = append(b.user, newAdminRisk("underused_plan_subscription", dto.AdminAnalyticsRiskSeverityInfo, dto.AdminAnalyticsRiskCategoryUser, "subscription half elapsed and usage < 10%", 1, *row.Quota.UsageRate, drilldown))
		}
		if query.SnapshotAt-row.Subscription.StartTime >= adminRiskRewardNeverUsedMinAgeSeconds && row.Source == dto.AdminAnalyticsSourceMonthlyInviteEntitlement && row.Subscription.TokenUsed == 0 {
			b.invitation = append(b.invitation, newAdminRisk("reward_subscription_never_used", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategoryInvitation, "reward age >= 7d and unused", 1, 0, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_subscriptions", UserID: &userID, PlanID: &planID}))
		}
		_ = subID
	}
	for userID, userRows := range activeByUser {
		if len(userRows) > 1 {
			b.plan = append(b.plan, newAdminRisk("overlapping_active_subscription", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategoryPlan, "multiple active subscriptions", len(userRows), float64(len(userRows)), &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_subscriptions", UserID: &userID, Status: "active"}))
		}
	}
	var expiredActive []UserSubscription
	if err := DB.Where("status = ? AND end_time <= ?", "active", query.SnapshotAt).Find(&expiredActive).Error; err == nil {
		for i := range expiredActive {
			userID := expiredActive[i].UserId
			planID := expiredActive[i].PlanId
			b.plan = append(b.plan, newAdminRisk("expired_active_status", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategoryPlan, "status active but end_time elapsed", 1, float64(expiredActive[i].EndTime), &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_subscriptions", UserID: &userID, PlanID: &planID, Status: "active"}))
		}
	}
}

func (b *adminRiskListBuckets) addInvitationRisks(query AdminAnalyticsQuery) error {
	var inviteRows []struct {
		InviterID int
		Count     int
	}
	if err := DB.Model(&User{}).Select("inviter_id, COUNT(*) AS count").Where("inviter_id > ?", 0).Group("inviter_id").Scan(&inviteRows).Error; err != nil {
		return err
	}
	for _, row := range inviteRows {
		if row.Count < adminRiskInviteMinDirect {
			continue
		}
		qualified, err := adminQualifiedInviteCount(query, row.InviterID)
		if err != nil {
			return err
		}
		rate := 0.0
		if row.Count > 0 {
			rate = float64(qualified) / float64(row.Count)
		}
		if rate < adminRiskInviteQualifiedRate {
			inviterID := row.InviterID
			b.invitation = append(b.invitation, newAdminRisk("many_invites_low_qualified", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategoryInvitation, "direct invites >= 20 and qualified rate < 10%", row.Count, rate, &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_invitations", InviterID: &inviterID}))
		}
	}
	return nil
}

func adminQualifiedInviteCount(query AdminAnalyticsQuery, inviterID int) (int64, error) {
	rows, err := loadAdminActiveSubscriptions(AdminAnalyticsQuery{SnapshotAt: query.SnapshotAt, StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp, Limit: AdminAnalyticsMaxLimit, InviterID: inviterID})
	if err != nil {
		return 0, err
	}
	seen := make(map[int]struct{}, len(rows))
	for _, row := range rows {
		if row.Plan.RewardEligible && row.Source == dto.AdminAnalyticsSourceOrder && row.Subscription.UserId != 0 {
			seen[row.Subscription.UserId] = struct{}{}
		}
	}
	return int64(len(seen)), nil
}

func (b *adminRiskListBuckets) addUsageRisks(query AdminAnalyticsQuery) ([]dto.AdminAnalyticsAvailabilityWarning, error) {
	usageQuery := AdminAnalyticsUsageQuery{AdminAnalyticsQuery: query, GroupBy: dto.AdminUsageGroupByUser, Metric: dto.AdminUsageMetricRequestCount, TopN: AdminAnalyticsMaxLimit}
	logs, warnings, err := loadAdminUsageCandidateLogs(usageQuery)
	if errors.Is(err, ErrAdminAnalyticsTooManyLogs) {
		b.system = append(b.system, newAdminRisk("candidate_log_limit_exceeded", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategorySystem, "candidate log limit exceeded", adminAnalyticsCandidateLogLimit, float64(adminAnalyticsCandidateLogLimit), nil))
		return warnings, nil
	}
	if err != nil {
		return warnings, err
	}
	byUser := map[int]struct {
		requests int
		errors   int
		models   map[string]struct{}
		stream   int
		tokens   int64
	}{}
	for i := range logs {
		entry := byUser[logs[i].UserId]
		if entry.models == nil {
			entry.models = map[string]struct{}{}
		}
		entry.requests++
		if logs[i].Type == LogTypeError {
			entry.errors++
		}
		if logs[i].ModelName != "" {
			entry.models[logs[i].ModelName] = struct{}{}
		}
		if logs[i].IsStream {
			entry.stream++
		}
		entry.tokens += adminUsageLogTokens(logs[i])
		byUser[logs[i].UserId] = entry
	}
	for userID, entry := range byUser {
		drilldown := &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_usage_logs", UserID: &userID, StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp}
		if entry.requests >= adminRiskHighErrorMinRequests && float64(entry.errors)/float64(entry.requests) >= adminRiskHighErrorRate {
			b.user = append(b.user, newAdminRisk("high_error_rate_user", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategoryUser, "error_rate >= 20%", entry.requests, float64(entry.errors)/float64(entry.requests), drilldown))
		}
		if entry.errors >= adminRiskManyFailedRequests {
			b.user = append(b.user, newAdminRisk("many_failed_requests", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategoryUser, "failed requests >= 50", entry.requests, float64(entry.errors), drilldown))
		}
		if len(entry.models) >= adminRiskManyModels && entry.tokens > 0 {
			b.user = append(b.user, newAdminRisk("many_tokens_across_many_models", dto.AdminAnalyticsRiskSeverityInfo, dto.AdminAnalyticsRiskCategoryUser, "many models with token usage", len(entry.models), float64(entry.tokens), drilldown))
		}
		if entry.requests > 0 && float64(entry.stream)/float64(entry.requests) >= adminRiskAbnormalStreamRatio {
			b.user = append(b.user, newAdminRisk("abnormal_stream_ratio", dto.AdminAnalyticsRiskSeverityInfo, dto.AdminAnalyticsRiskCategoryUser, "stream ratio >= 95%", entry.requests, float64(entry.stream)/float64(entry.requests), drilldown))
		}
	}
	return warnings, nil
}

func newAdminRisk(key string, severity dto.AdminAnalyticsRiskSeverity, category dto.AdminAnalyticsRiskCategory, threshold string, sampleSize int, value float64, drilldown *dto.AdminAnalyticsDrilldownTarget) dto.AdminAnalyticsRiskItem {
	return dto.AdminAnalyticsRiskItem{RiskKey: key, Severity: severity, Category: category, Title: key, Description: key, Threshold: threshold, SampleSize: sampleSize, Value: value, Drilldown: drilldown}
}

func adminRiskList(items []dto.AdminAnalyticsRiskItem, query AdminAnalyticsQuery, category dto.AdminAnalyticsRiskCategory) dto.AdminAnalyticsList[dto.AdminAnalyticsRiskItem] {
	ordered := append([]dto.AdminAnalyticsRiskItem(nil), items...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Severity == ordered[j].Severity {
			return ordered[i].RiskKey < ordered[j].RiskKey
		}
		return adminRiskSeverityRank(ordered[i].Severity) > adminRiskSeverityRank(ordered[j].Severity)
	})
	paged, page := paginateAdminAnalyticsList(ordered, query.Limit, query.Offset)
	return dto.AdminAnalyticsList[dto.AdminAnalyticsRiskItem]{Items: paged, Page: page, SortBy: "severity", SortOrder: dto.AdminAnalyticsSortDesc}
}

func adminRiskSeverityRank(severity dto.AdminAnalyticsRiskSeverity) int {
	switch severity {
	case dto.AdminAnalyticsRiskSeverityCritical:
		return 3
	case dto.AdminAnalyticsRiskSeverityWarning:
		return 2
	default:
		return 1
	}
}

func adminRiskSnapshotNow() int64 { return time.Now().Unix() }

func adminRiskKeySet(response dto.AdminAnalyticsRisksResponse) map[string]struct{} {
	result := map[string]struct{}{}
	for _, item := range response.PlanRisks.Items {
		result[item.RiskKey] = struct{}{}
	}
	for _, item := range response.UserRisks.Items {
		result[item.RiskKey] = struct{}{}
	}
	for _, item := range response.InvitationRisks.Items {
		result[item.RiskKey] = struct{}{}
	}
	for _, item := range response.SystemRisks.Items {
		result[item.RiskKey] = struct{}{}
	}
	return result
}

func adminRiskDebugKeys(response dto.AdminAnalyticsRisksResponse) string {
	keys := adminRiskKeySet(response)
	values := make([]string, 0, len(keys))
	for key := range keys {
		values = append(values, key)
	}
	sort.Strings(values)
	return fmt.Sprintf("%v", values)
}
