package model

import (
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

var (
	trialAbuseCandidateLimit        = 5000
	trialAbuseLogScanLimit          = 200000
	trialAbuseLogAggregateUserLimit = 5000
)

var trialAbuseTrialGrantReasons = []string{"trial_code", "invite_trial"}

type TrialAbuseQuery struct {
	TrialEndStart   int64
	TrialEndEnd     int64
	RegisteredStart int64
	RegisteredEnd   int64
	SnapshotAt      int64
	MinConsumeCount int
	MinClusterSize  int
	RiskLimit       int
	GroupLimit      int
}

type trialAbuseTrialRow struct {
	SubscriptionID int
	UserID         int
	PlanID         int
	Username       string
	CreatedAt      int64
	InviterID      int
	GrantReason    string
	Source         string
	StartTime      int64
	EndTime        int64
}

type trialAbuseUsage struct {
	ConsumeCount  int
	UsedQuota     int64
	MeteredTokens int64
	ObservedIP    string
	ipCounts      map[string]int
}

type trialAbuseClassificationInput struct {
	RegistrationIPAvailable                    bool
	SameRegistrationIPCandidateCount           int
	SameRegistrationIPSelfInviteCandidateCount int
	InviterCandidateCount                      int
	InviterExpiredTrialInviteeCount            int
	InviterPaidConversionRate                  float64
	InviterManaged                             bool
	MinClusterSize                             int
}

type trialAbuseClassificationOutput struct {
	RiskLevel   string
	RiskScore   int
	RiskReasons []string
}

type trialAbuseLogWarningState struct {
	LogUnavailable bool
	LogLimit       bool
}

func GetTrialAbuseSummary(query TrialAbuseQuery) (*dto.TrialAbuseSummaryResponse, error) {
	generatedAt := time.Now().Unix()
	warnings := make([]dto.TrialAbuseWarning, 0, 4)
	warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionIPClusters, Reason: dto.TrialAbuseWarningRegistrationIPUnavailable, Message: "structured registration IP is unavailable; consume log IP is display only"})
	warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionSelfInviteChains, Reason: dto.TrialAbuseWarningRegistrationIPUnavailable, Message: "structured registration IP is unavailable; self-invite chain risk is disabled"})
	warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionRiskUsers, Reason: dto.TrialAbuseWarningRegistrationIPUnavailable, Message: "structured registration IP is unavailable; IP risk reasons are disabled"})

	overview := dto.TrialAbuseOverview{}
	stats, err := countTrialAbuseOverviewTrials(query)
	if err != nil {
		return nil, err
	}
	overview.TotalTrialUsers = stats.TotalTrialUsers
	overview.ActiveTrialUsers = stats.ActiveTrialUsers
	overview.ExpiredTrialUsers = stats.ExpiredTrialUsers

	trialRows, candidateLimited, err := queryTrialAbuseExpiredTrialRows(query)
	if err != nil {
		return nil, err
	}
	if candidateLimited {
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionOverview, Reason: dto.TrialAbuseWarningCandidateLimitExceeded, Message: "trial candidate limit exceeded; result is partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionUsageDistribution, Reason: dto.TrialAbuseWarningCandidateLimitExceeded, Message: "trial candidate limit exceeded; usage distribution is partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionRiskUsers, Reason: dto.TrialAbuseWarningCandidateLimitExceeded, Message: "trial candidate limit exceeded; risk user list is partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionRiskCounts, Reason: dto.TrialAbuseWarningCandidateLimitExceeded, Message: "trial candidate limit exceeded; risk counts are partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionInviterClusters, Reason: dto.TrialAbuseWarningCandidateLimitExceeded, Message: "trial candidate limit exceeded; inviter conversion is partial"})
	}

	denominatorRows, denominatorLimited, err := queryTrialAbuseExpiredInviteTrialRows(query)
	if err != nil {
		return nil, err
	}
	if denominatorLimited {
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionInviterClusters, Reason: dto.TrialAbuseWarningCandidateLimitExceeded, Message: "invite trial denominator limit exceeded; inviter conversion is partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionRiskUsers, Reason: dto.TrialAbuseWarningCandidateLimitExceeded, Message: "invite trial denominator limit exceeded; risk users are partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionRiskCounts, Reason: dto.TrialAbuseWarningCandidateLimitExceeded, Message: "invite trial denominator limit exceeded; risk counts are partial"})
	}
	paidUserIDs, err := queryTrialAbusePaidEntitlementUsers(mergeTrialAbuseUserIDs(trialRows, denominatorRows))
	if err != nil {
		return nil, err
	}

	expiredUnpaidRows := make([]trialAbuseTrialRow, 0, len(trialRows))
	for _, row := range trialRows {
		if paidUserIDs[row.UserID] {
			continue
		}
		expiredUnpaidRows = append(expiredUnpaidRows, row)
	}
	overview.ExpiredUnpaidTrialUsers = len(expiredUnpaidRows)

	usageByUser, logState := queryTrialAbuseUsage(expiredUnpaidRows)
	if logState.LogUnavailable {
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionOverview, Reason: dto.TrialAbuseWarningLogUnavailable, Message: "consume logs are unavailable; usage-based analysis is partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionInviterClusters, Reason: dto.TrialAbuseWarningLogUnavailable, Message: "consume logs are unavailable; inviter clusters are partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionUsageDistribution, Reason: dto.TrialAbuseWarningLogUnavailable, Message: "consume logs are unavailable; usage distribution is partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionRiskUsers, Reason: dto.TrialAbuseWarningLogUnavailable, Message: "consume logs are unavailable; risk users are partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionRiskCounts, Reason: dto.TrialAbuseWarningLogUnavailable, Message: "consume logs are unavailable; risk counts are partial"})
	}
	if logState.LogLimit {
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionUsageDistribution, Reason: dto.TrialAbuseWarningLogLimitExceeded, Message: "consume log limit exceeded; usage distribution is partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionRiskUsers, Reason: dto.TrialAbuseWarningLogLimitExceeded, Message: "consume log limit exceeded; risk users are partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionOverview, Reason: dto.TrialAbuseWarningLogLimitExceeded, Message: "consume log limit exceeded; overview is partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionInviterClusters, Reason: dto.TrialAbuseWarningLogLimitExceeded, Message: "consume log limit exceeded; inviter clusters are partial"})
		warnings = append(warnings, dto.TrialAbuseWarning{Section: dto.TrialAbuseSectionRiskCounts, Reason: dto.TrialAbuseWarningLogLimitExceeded, Message: "consume log limit exceeded; risk counts are partial"})
	}

	highUsageRows := make([]trialAbuseTrialRow, 0, len(expiredUnpaidRows))
	for _, row := range expiredUnpaidRows {
		if usageByUser[row.UserID].ConsumeCount >= query.MinConsumeCount {
			highUsageRows = append(highUsageRows, row)
		}
	}
	overview.HighUsageCandidateUsers = len(highUsageRows)

	inviterClusters, inviterFeatures, err := buildTrialAbuseInviterClusters(query, highUsageRows, denominatorRows, paidUserIDs, usageByUser)
	if err != nil {
		return nil, err
	}
	ipClusters := buildTrialAbuseIPClusters(query, expiredUnpaidRows, highUsageRows, usageByUser)
	riskUsers, riskCounts := buildTrialAbuseRiskUsers(query, highUsageRows, usageByUser, inviterFeatures, warnings)
	applyTrialAbuseRiskLimits(query.RiskLimit, &riskUsers)

	overview.RiskUserCount = riskCounts.High + riskCounts.Medium + riskCounts.Low
	overview.HighRiskUserCount = riskCounts.High
	overview.MediumRiskUserCount = riskCounts.Medium
	overview.LowRiskUserCount = riskCounts.Low
	for _, cluster := range inviterClusters {
		if cluster.Managed {
			overview.ManagedInviterClusterCount++
		}
	}

	usageDistribution := buildTrialAbuseUsageDistribution(query, expiredUnpaidRows, usageByUser)
	partialSections := buildTrialAbusePartialSections(warnings)
	applyTrialAbusePartials(partialSections, &overview, &riskCounts, &usageDistribution, ipClusters, inviterClusters, nil, riskUsers)

	return &dto.TrialAbuseSummaryResponse{
		GeneratedAt: generatedAt,
		Criteria: dto.TrialAbuseCriteria{
			TrialEndStart:   query.TrialEndStart,
			TrialEndEnd:     query.TrialEndEnd,
			RegisteredStart: query.RegisteredStart,
			RegisteredEnd:   query.RegisteredEnd,
			SnapshotAt:      query.SnapshotAt,
			MinConsumeCount: query.MinConsumeCount,
			MinClusterSize:  query.MinClusterSize,
			RiskLimit:       query.RiskLimit,
			GroupLimit:      query.GroupLimit,
		},
		Warnings:          warnings,
		PartialSections:   partialSections,
		Overview:          overview,
		RiskCounts:        riskCounts,
		UsageDistribution: usageDistribution,
		IPClusters:        ipClusters,
		InviterClusters:   inviterClusters,
		SelfInviteChains:  []dto.TrialAbuseSelfInviteChain{},
		RiskUsers:         riskUsers,
	}, nil
}

type trialAbuseOverviewTrialStats struct {
	TotalTrialUsers   int
	ActiveTrialUsers  int
	ExpiredTrialUsers int
}

func countTrialAbuseOverviewTrials(query TrialAbuseQuery) (trialAbuseOverviewTrialStats, error) {
	trialUserIDs, err := queryTrialAbuseOverviewUserIDs(query)
	if err != nil {
		return trialAbuseOverviewTrialStats{}, err
	}
	paidUserIDs, err := queryTrialAbusePaidEntitlementUsers(trialUserIDs)
	if err != nil {
		return trialAbuseOverviewTrialStats{}, err
	}
	paidIDs := trialAbusePaidUserIDSlice(paidUserIDs)
	base := func() *gorm.DB {
		db := DB.Model(&UserSubscription{}).
			Joins("JOIN users ON users.id = user_subscriptions.user_id").
			Where("TRIM(user_subscriptions.grant_reason) IN ? OR (TRIM(user_subscriptions.grant_reason) = '' AND TRIM(user_subscriptions.source) IN ?)", trialAbuseTrialGrantReasons, trialAbuseTrialGrantReasons)
		if query.RegisteredStart > 0 {
			db = db.Where("users.created_at >= ?", query.RegisteredStart)
		}
		if query.RegisteredEnd > 0 {
			db = db.Where("users.created_at <= ?", query.RegisteredEnd)
		}
		if len(paidIDs) > 0 {
			db = db.Where("user_subscriptions.user_id NOT IN ?", paidIDs)
		}
		return db
	}
	var total int64
	if err := base().Distinct("user_subscriptions.user_id").Count(&total).Error; err != nil {
		return trialAbuseOverviewTrialStats{}, err
	}
	var active int64
	if err := base().Where("user_subscriptions.end_time > ?", query.SnapshotAt).Distinct("user_subscriptions.user_id").Count(&active).Error; err != nil {
		return trialAbuseOverviewTrialStats{}, err
	}
	var expired int64
	if err := base().Where("user_subscriptions.end_time >= ? AND user_subscriptions.end_time <= ? AND user_subscriptions.end_time <= ?", query.TrialEndStart, query.TrialEndEnd, query.SnapshotAt).Distinct("user_subscriptions.user_id").Count(&expired).Error; err != nil {
		return trialAbuseOverviewTrialStats{}, err
	}
	return trialAbuseOverviewTrialStats{TotalTrialUsers: int(total), ActiveTrialUsers: int(active), ExpiredTrialUsers: int(expired)}, nil
}

func queryTrialAbuseExpiredTrialRows(query TrialAbuseQuery) ([]trialAbuseTrialRow, bool, error) {
	rows, limited, err := queryTrialAbuseRows(query, trialAbuseTrialGrantReasons, trialAbuseCandidateLimit+1)
	if err != nil {
		return nil, false, err
	}
	if limited {
		rows = rows[:trialAbuseCandidateLimit]
	}
	return rows, limited, nil
}

func queryTrialAbuseExpiredInviteTrialRows(query TrialAbuseQuery) ([]trialAbuseTrialRow, bool, error) {
	rows, limited, err := queryTrialAbuseRows(query, []string{"invite_trial"}, trialAbuseCandidateLimit+1)
	if err != nil {
		return nil, false, err
	}
	if limited {
		rows = rows[:trialAbuseCandidateLimit]
	}
	return rows, limited, nil
}

func dedupeTrialAbuseRowsByUser(rows []trialAbuseTrialRow) []trialAbuseTrialRow {
	if len(rows) < 2 {
		return rows
	}
	seen := make(map[int]bool, len(rows))
	deduped := make([]trialAbuseTrialRow, 0, len(rows))
	for _, row := range rows {
		if row.UserID <= 0 || seen[row.UserID] {
			continue
		}
		seen[row.UserID] = true
		deduped = append(deduped, row)
	}
	return deduped
}

func queryTrialAbuseRows(query TrialAbuseQuery, grantReasons []string, limit int) ([]trialAbuseTrialRow, bool, error) {
	userIDs, limited, err := queryTrialAbuseDistinctUserIDs(query, grantReasons, limit)
	if err != nil {
		return nil, false, err
	}
	if len(userIDs) == 0 {
		return nil, limited, nil
	}
	if limited {
		userIDs = userIDs[:limit-1]
	}
	var rows []trialAbuseTrialRow
	db := DB.Table("user_subscriptions").
		Select("user_subscriptions.id AS subscription_id, user_subscriptions.user_id, user_subscriptions.plan_id, users.username, users.created_at, users.inviter_id, user_subscriptions.grant_reason, user_subscriptions.source, user_subscriptions.start_time, user_subscriptions.end_time").
		Joins("JOIN users ON users.id = user_subscriptions.user_id").
		Where("user_subscriptions.user_id IN ?", userIDs).
		Where("TRIM(user_subscriptions.grant_reason) IN ? OR (TRIM(user_subscriptions.grant_reason) = '' AND TRIM(user_subscriptions.source) IN ?)", grantReasons, grantReasons).
		Where("user_subscriptions.end_time >= ? AND user_subscriptions.end_time <= ? AND user_subscriptions.end_time <= ?", query.TrialEndStart, query.TrialEndEnd, query.SnapshotAt).
		Order("user_subscriptions.end_time ASC, user_subscriptions.id ASC")
	if query.RegisteredStart > 0 {
		db = db.Where("users.created_at >= ?", query.RegisteredStart)
	}
	if query.RegisteredEnd > 0 {
		db = db.Where("users.created_at <= ?", query.RegisteredEnd)
	}
	if err := db.Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	return dedupeTrialAbuseRowsByUser(rows), limited, nil
}

func queryTrialAbuseOverviewUserIDs(query TrialAbuseQuery) ([]int, error) {
	var ids []int
	db := DB.Model(&UserSubscription{}).
		Select("user_subscriptions.user_id").
		Joins("JOIN users ON users.id = user_subscriptions.user_id").
		Where("TRIM(user_subscriptions.grant_reason) IN ? OR (TRIM(user_subscriptions.grant_reason) = '' AND TRIM(user_subscriptions.source) IN ?)", trialAbuseTrialGrantReasons, trialAbuseTrialGrantReasons).
		Group("user_subscriptions.user_id")
	if query.RegisteredStart > 0 {
		db = db.Where("users.created_at >= ?", query.RegisteredStart)
	}
	if query.RegisteredEnd > 0 {
		db = db.Where("users.created_at <= ?", query.RegisteredEnd)
	}
	if err := db.Pluck("user_subscriptions.user_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func queryTrialAbuseDistinctUserIDs(query TrialAbuseQuery, grantReasons []string, limit int) ([]int, bool, error) {
	var ids []int
	db := DB.Table("user_subscriptions").
		Select("user_subscriptions.user_id").
		Joins("JOIN users ON users.id = user_subscriptions.user_id").
		Where("TRIM(user_subscriptions.grant_reason) IN ? OR (TRIM(user_subscriptions.grant_reason) = '' AND TRIM(user_subscriptions.source) IN ?)", grantReasons, grantReasons).
		Where("user_subscriptions.end_time >= ? AND user_subscriptions.end_time <= ? AND user_subscriptions.end_time <= ?", query.TrialEndStart, query.TrialEndEnd, query.SnapshotAt).
		Group("user_subscriptions.user_id").
		Order("MIN(user_subscriptions.end_time) ASC, MIN(user_subscriptions.id) ASC")
	if query.RegisteredStart > 0 {
		db = db.Where("users.created_at >= ?", query.RegisteredStart)
	}
	if query.RegisteredEnd > 0 {
		db = db.Where("users.created_at <= ?", query.RegisteredEnd)
	}
	if limit > 0 {
		db = db.Limit(limit)
	}
	if err := db.Pluck("user_subscriptions.user_id", &ids).Error; err != nil {
		return nil, false, err
	}
	limited := limit > 0 && len(ids) >= limit
	return ids, limited, nil
}

func trialAbusePaidUserIDSlice(userIDs map[int]bool) []int {
	ids := make([]int, 0, len(userIDs))
	for id := range userIDs {
		ids = append(ids, id)
	}
	return ids
}

func mergeTrialAbuseUserIDs(groups ...[]trialAbuseTrialRow) []int {
	seen := make(map[int]bool)
	ids := make([]int, 0)
	for _, rows := range groups {
		for _, row := range rows {
			if row.UserID <= 0 || seen[row.UserID] {
				continue
			}
			seen[row.UserID] = true
			ids = append(ids, row.UserID)
		}
	}
	return ids
}

func queryTrialAbusePaidEntitlementUsers(userIDs []int) (map[int]bool, error) {
	result := make(map[int]bool)
	if len(userIDs) == 0 {
		return result, nil
	}
	var rows []struct {
		UserID      int
		GrantReason string
		Source      string
	}
	if err := DB.Model(&UserSubscription{}).
		Select("user_subscriptions.user_id, user_subscriptions.grant_reason, user_subscriptions.source").
		Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Where("user_subscriptions.user_id IN ?", userIDs).
		Where("subscription_plans.price_amount > ?", 0).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if isTrialAbusePaidEntitlementSource(normalizeTrialAbuseSource(row.GrantReason, row.Source)) {
			result[row.UserID] = true
		}
	}
	return result, nil
}

func normalizeTrialAbuseSource(grantReason string, source string) string {
	trimmedGrantReason := strings.TrimSpace(grantReason)
	if trimmedGrantReason != "" {
		return trimmedGrantReason
	}
	return strings.TrimSpace(source)
}

func isTrialAbusePaidEntitlementSource(source string) bool {
	switch strings.TrimSpace(source) {
	case SubscriptionGrantOrder, "redemption", "admin", SubscriptionGrantMonthlyInviteEntitlement:
		return true
	default:
		return false
	}
}

func queryTrialAbuseUsage(rows []trialAbuseTrialRow) (map[int]trialAbuseUsage, trialAbuseLogWarningState) {
	usageByUser := make(map[int]trialAbuseUsage, len(rows))
	for _, row := range rows {
		usageByUser[row.UserID] = trialAbuseUsage{ipCounts: make(map[string]int)}
	}
	if len(rows) == 0 {
		return usageByUser, trialAbuseLogWarningState{}
	}
	state := trialAbuseLogWarningState{}
	if !common.LogConsumeEnabled || LOG_DB == nil {
		state.LogUnavailable = true
		return usageByUser, state
	}
	if len(rows) > trialAbuseLogAggregateUserLimit {
		state.LogLimit = true
		rows = rows[:trialAbuseLogAggregateUserLimit]
	}
	byUser := make(map[int]trialAbuseTrialRow, len(rows))
	userIDs := make([]int, 0, len(rows))
	minStart := rows[0].StartTime
	maxEnd := rows[0].EndTime
	for _, row := range rows {
		byUser[row.UserID] = row
		userIDs = append(userIDs, row.UserID)
		if row.StartTime < minStart {
			minStart = row.StartTime
		}
		if row.EndTime > maxEnd {
			maxEnd = row.EndTime
		}
	}
	var logs []Log
	if err := LOG_DB.Model(&Log{}).
		Select("user_id, created_at, quota, metered_tokens, ip").
		Where("user_id IN ? AND type = ? AND created_at >= ? AND created_at <= ?", userIDs, LogTypeConsume, minStart, maxEnd).
		Order("id ASC").
		Limit(trialAbuseLogScanLimit + 1).
		Find(&logs).Error; err != nil {
		state.LogUnavailable = true
		return usageByUser, state
	}
	if len(logs) > trialAbuseLogScanLimit {
		state.LogLimit = true
		logs = logs[:trialAbuseLogScanLimit]
	}
	for _, log := range logs {
		row, ok := byUser[log.UserId]
		if !ok || log.CreatedAt < row.StartTime || log.CreatedAt > row.EndTime {
			continue
		}
		usage := usageByUser[log.UserId]
		if usage.ipCounts == nil {
			usage.ipCounts = make(map[string]int)
		}
		usage.ConsumeCount++
		usage.UsedQuota += int64(log.Quota)
		if log.MeteredTokens != nil {
			usage.MeteredTokens += int64(*log.MeteredTokens)
		}
		ip := strings.TrimSpace(log.Ip)
		if ip != "" {
			usage.ipCounts[ip]++
			if usage.ObservedIP == "" || usage.ipCounts[ip] > usage.ipCounts[usage.ObservedIP] {
				usage.ObservedIP = ip
			}
		}
		usageByUser[log.UserId] = usage
	}
	return usageByUser, state
}

type trialAbuseInviterFeature struct {
	InviterID                int
	InviterUsername          string
	Managed                  bool
	CandidateCount           int
	ExpiredTrialInviteeCount int
	PaidEntitlementCount     int
	PaidConversionRate       float64
}

func buildTrialAbuseInviterClusters(query TrialAbuseQuery, candidateRows []trialAbuseTrialRow, denominatorRows []trialAbuseTrialRow, paidUserIDs map[int]bool, usageByUser map[int]trialAbuseUsage) ([]dto.TrialAbuseInviterCluster, map[int]trialAbuseInviterFeature, error) {
	type accumulator struct {
		feature               trialAbuseInviterFeature
		expiredUnpaidTrialIDs map[int]bool
		totalConsumeCount     int
		sampleUserIDs         []int
	}
	clustersByInviter := make(map[int]*accumulator)
	for _, row := range denominatorRows {
		if row.InviterID <= 0 {
			continue
		}
		acc := clustersByInviter[row.InviterID]
		if acc == nil {
			acc = &accumulator{expiredUnpaidTrialIDs: make(map[int]bool)}
			clustersByInviter[row.InviterID] = acc
		}
		acc.feature.InviterID = row.InviterID
		acc.feature.ExpiredTrialInviteeCount++
		if paidUserIDs[row.UserID] {
			acc.feature.PaidEntitlementCount++
		} else {
			acc.expiredUnpaidTrialIDs[row.UserID] = true
		}
	}
	for _, row := range candidateRows {
		if row.InviterID <= 0 {
			continue
		}
		acc := clustersByInviter[row.InviterID]
		if acc == nil {
			acc = &accumulator{expiredUnpaidTrialIDs: make(map[int]bool)}
			clustersByInviter[row.InviterID] = acc
		}
		acc.feature.InviterID = row.InviterID
		acc.feature.CandidateCount++
		acc.totalConsumeCount += usageByUser[row.UserID].ConsumeCount
		appendSampleUserID(&acc.sampleUserIDs, row.UserID, 10)
	}
	inviterIDs := make([]int, 0, len(clustersByInviter))
	for inviterID := range clustersByInviter {
		inviterIDs = append(inviterIDs, inviterID)
	}
	inviterUsers, err := trialAbuseUsersByID(inviterIDs)
	if err != nil {
		return nil, nil, err
	}
	clusters := make([]dto.TrialAbuseInviterCluster, 0, len(clustersByInviter))
	features := make(map[int]trialAbuseInviterFeature, len(clustersByInviter))
	for inviterID, acc := range clustersByInviter {
		inviter := inviterUsers[inviterID]
		acc.feature.InviterUsername = inviter.Username
		acc.feature.Managed = isTrialAbuseManagedInviter(inviter.Role)
		if acc.feature.ExpiredTrialInviteeCount > 0 {
			acc.feature.PaidConversionRate = float64(acc.feature.PaidEntitlementCount) / float64(acc.feature.ExpiredTrialInviteeCount)
		}
		features[inviterID] = acc.feature
		participation := "display_only"
		if !acc.feature.Managed && acc.feature.CandidateCount >= 10 && acc.feature.ExpiredTrialInviteeCount > 0 && acc.feature.PaidConversionRate < 0.10 {
			participation = "risk"
		}
		clusters = append(clusters, dto.TrialAbuseInviterCluster{InviterID: inviterID, InviterUsername: acc.feature.InviterUsername, Managed: acc.feature.Managed, CandidateCount: acc.feature.CandidateCount, ExpiredTrialInviteeCount: acc.feature.ExpiredTrialInviteeCount, ExpiredUnpaidTrialCount: len(acc.expiredUnpaidTrialIDs), PaidEntitlementCount: acc.feature.PaidEntitlementCount, PaidConversionRate: acc.feature.PaidConversionRate, TotalConsumeCount: acc.totalConsumeCount, RiskParticipation: participation, SampleUserIDs: acc.sampleUserIDs})
	}
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].CandidateCount != clusters[j].CandidateCount {
			return clusters[i].CandidateCount > clusters[j].CandidateCount
		}
		return clusters[i].InviterID < clusters[j].InviterID
	})
	if len(clusters) > query.GroupLimit {
		clusters = clusters[:query.GroupLimit]
	}
	return clusters, features, nil
}

func trialAbuseUsersByID(userIDs []int) (map[int]User, error) {
	result := make(map[int]User, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	var users []User
	if err := DB.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		result[user.Id] = user
	}
	return result, nil
}

func isTrialAbuseManagedInviter(role int) bool {
	return role == common.RoleAdminUser || role == common.RoleRootUser
}

func buildTrialAbuseIPClusters(query TrialAbuseQuery, expiredUnpaidRows []trialAbuseTrialRow, candidateRows []trialAbuseTrialRow, usageByUser map[int]trialAbuseUsage) []dto.TrialAbuseIPCluster {
	type ipAccumulator struct {
		candidateCount          int
		expiredUnpaidTrialCount int
		totalConsumeCount       int
		sampleUserIDs           []int
	}
	clustersByIP := make(map[string]*ipAccumulator)
	for _, row := range candidateRows {
		usage := usageByUser[row.UserID]
		if usage.ObservedIP == "" {
			continue
		}
		acc := clustersByIP[usage.ObservedIP]
		if acc == nil {
			acc = &ipAccumulator{}
			clustersByIP[usage.ObservedIP] = acc
		}
		acc.candidateCount++
		acc.totalConsumeCount += usage.ConsumeCount
		appendSampleUserID(&acc.sampleUserIDs, row.UserID, 10)
	}
	for _, row := range expiredUnpaidRows {
		ip := usageByUser[row.UserID].ObservedIP
		if ip == "" {
			continue
		}
		acc := clustersByIP[ip]
		if acc == nil {
			continue
		}
		acc.expiredUnpaidTrialCount++
	}
	clusters := make([]dto.TrialAbuseIPCluster, 0, len(clustersByIP))
	for ip, acc := range clustersByIP {
		if acc.candidateCount < query.MinClusterSize {
			continue
		}
		clusters = append(clusters, dto.TrialAbuseIPCluster{ObservedIP: ip, IPSource: "consume_log", RegistrationIPAvailable: false, CandidateCount: acc.candidateCount, ExpiredUnpaidTrialCount: acc.expiredUnpaidTrialCount, PaidEntitlementCount: 0, TotalConsumeCount: acc.totalConsumeCount, SampleUserIDs: acc.sampleUserIDs})
	}
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].CandidateCount != clusters[j].CandidateCount {
			return clusters[i].CandidateCount > clusters[j].CandidateCount
		}
		return clusters[i].ObservedIP < clusters[j].ObservedIP
	})
	if len(clusters) > query.GroupLimit {
		clusters = clusters[:query.GroupLimit]
	}
	return clusters
}

func buildTrialAbuseRiskUsers(query TrialAbuseQuery, candidateRows []trialAbuseTrialRow, usageByUser map[int]trialAbuseUsage, inviterFeatures map[int]trialAbuseInviterFeature, warnings []dto.TrialAbuseWarning) ([]dto.TrialAbuseRiskUser, dto.TrialAbuseRiskCounts) {
	riskUsers := make([]dto.TrialAbuseRiskUser, 0, len(candidateRows))
	counts := dto.TrialAbuseRiskCounts{}
	for _, row := range candidateRows {
		feature := inviterFeatures[row.InviterID]
		classification := classifyTrialAbuseRisks(trialAbuseClassificationInput{RegistrationIPAvailable: false, InviterCandidateCount: feature.CandidateCount, InviterExpiredTrialInviteeCount: feature.ExpiredTrialInviteeCount, InviterPaidConversionRate: feature.PaidConversionRate, InviterManaged: feature.Managed, MinClusterSize: query.MinClusterSize})
		if classification.RiskLevel == "" {
			continue
		}
		usage := usageByUser[row.UserID]
		riskUser := dto.TrialAbuseRiskUser{UserID: row.UserID, Username: row.Username, CreatedAt: row.CreatedAt, TrialSource: normalizeTrialAbuseSource(row.GrantReason, row.Source), TrialStartTime: row.StartTime, TrialEndTime: row.EndTime, InviterID: row.InviterID, InviterUsername: feature.InviterUsername, ConsumeCount: usage.ConsumeCount, UsedQuota: usage.UsedQuota, MeteredTokens: usage.MeteredTokens, ObservedIP: usage.ObservedIP, IPSource: ipSourceForObservedIP(usage.ObservedIP), RegistrationIPAvailable: false, RiskLevel: classification.RiskLevel, RiskScore: classification.RiskScore, RiskReasons: classification.RiskReasons, PaidEntitlementExcluded: false}
		applyWarningsToRiskUser(&riskUser, warnings)
		riskUsers = append(riskUsers, riskUser)
		switch classification.RiskLevel {
		case dto.TrialAbuseRiskLevelHigh:
			counts.High++
		case dto.TrialAbuseRiskLevelMedium:
			counts.Medium++
		case dto.TrialAbuseRiskLevelLow:
			counts.Low++
		}
	}
	sort.Slice(riskUsers, func(i, j int) bool {
		if riskUsers[i].RiskScore != riskUsers[j].RiskScore {
			return riskUsers[i].RiskScore > riskUsers[j].RiskScore
		}
		if riskUsers[i].ConsumeCount != riskUsers[j].ConsumeCount {
			return riskUsers[i].ConsumeCount > riskUsers[j].ConsumeCount
		}
		return riskUsers[i].UserID < riskUsers[j].UserID
	})
	return riskUsers, counts
}

func classifyTrialAbuseRisks(input trialAbuseClassificationInput) trialAbuseClassificationOutput {
	output := trialAbuseClassificationOutput{RiskReasons: []string{}}
	if input.RegistrationIPAvailable {
		if input.SameRegistrationIPSelfInviteCandidateCount >= 3 {
			output.RiskLevel = dto.TrialAbuseRiskLevelHigh
			output.RiskScore = 100
			output.RiskReasons = append(output.RiskReasons, dto.TrialAbuseRiskReasonSameRegistrationIPSelfInviteChain)
		}
		if input.SameRegistrationIPCandidateCount >= 5 {
			if output.RiskScore < 90 {
				output.RiskLevel = dto.TrialAbuseRiskLevelHigh
				output.RiskScore = 90
			}
			output.RiskReasons = appendIfMissing(output.RiskReasons, dto.TrialAbuseRiskReasonSameRegistrationIPCluster)
		} else if input.SameRegistrationIPCandidateCount >= 3 && output.RiskScore < 60 {
			output.RiskLevel = dto.TrialAbuseRiskLevelMedium
			output.RiskScore = 60
			output.RiskReasons = append(output.RiskReasons, dto.TrialAbuseRiskReasonSameRegistrationIPCluster)
		} else if input.SameRegistrationIPCandidateCount >= input.MinClusterSize && input.SameRegistrationIPCandidateCount > 0 && output.RiskScore == 0 {
			output.RiskLevel = dto.TrialAbuseRiskLevelLow
			output.RiskScore = 20
			output.RiskReasons = append(output.RiskReasons, dto.TrialAbuseRiskReasonSameRegistrationIPCluster)
		}
	}
	if input.InviterCandidateCount >= 10 && input.InviterExpiredTrialInviteeCount > 0 && input.InviterPaidConversionRate < 0.10 {
		if input.InviterManaged {
			if output.RiskLevel != "" {
				output.RiskReasons = appendIfMissing(output.RiskReasons, dto.TrialAbuseRiskReasonManagedInviterDisplayOnly)
			}
		} else {
			if output.RiskScore < 60 {
				output.RiskLevel = dto.TrialAbuseRiskLevelMedium
				output.RiskScore = 60
			}
			output.RiskReasons = appendIfMissing(output.RiskReasons, dto.TrialAbuseRiskReasonInviterLowPaidConversion)
		}
	}
	return output
}

func buildTrialAbuseUsageDistribution(query TrialAbuseQuery, rows []trialAbuseTrialRow, usageByUser map[int]trialAbuseUsage) dto.TrialAbuseUsageDistribution {
	counts := make([]int, 0, len(rows))
	distribution := dto.TrialAbuseUsageDistribution{SampleSize: len(rows)}
	for _, row := range rows {
		count := usageByUser[row.UserID].ConsumeCount
		counts = append(counts, count)
		if count == 0 {
			distribution.ZeroUsageCount++
		}
		if count >= query.MinConsumeCount {
			distribution.AboveThresholdCount++
		}
	}
	sort.Ints(counts)
	distribution.P50 = trialAbusePercentile(counts, 50)
	distribution.P75 = trialAbusePercentile(counts, 75)
	distribution.P90 = trialAbusePercentile(counts, 90)
	distribution.P95 = trialAbusePercentile(counts, 95)
	distribution.P99 = trialAbusePercentile(counts, 99)
	return distribution
}

func trialAbusePercentile(sortedValues []int, percentile int) int {
	if len(sortedValues) == 0 {
		return 0
	}
	if len(sortedValues) == 1 {
		return sortedValues[0]
	}
	index := (percentile*len(sortedValues) + 99) / 100
	if index < 1 {
		index = 1
	}
	if index > len(sortedValues) {
		index = len(sortedValues)
	}
	return sortedValues[index-1]
}

func buildTrialAbusePartialSections(warnings []dto.TrialAbuseWarning) map[string][]string {
	partialSections := make(map[string][]string)
	for _, warning := range warnings {
		if warning.Section == "" || warning.Reason == "" {
			continue
		}
		partialSections[warning.Section] = appendIfMissing(partialSections[warning.Section], warning.Reason)
	}
	if len(partialSections) == 0 {
		return map[string][]string{}
	}
	return partialSections
}

func applyTrialAbusePartials(partialSections map[string][]string, overview *dto.TrialAbuseOverview, riskCounts *dto.TrialAbuseRiskCounts, usageDistribution *dto.TrialAbuseUsageDistribution, ipClusters []dto.TrialAbuseIPCluster, inviterClusters []dto.TrialAbuseInviterCluster, selfInviteChains []dto.TrialAbuseSelfInviteChain, riskUsers []dto.TrialAbuseRiskUser) {
	if reasons := partialSections[dto.TrialAbuseSectionOverview]; len(reasons) > 0 {
		overview.Partial = true
		overview.PartialReasons = reasons
	}
	if reasons := partialSections[dto.TrialAbuseSectionRiskCounts]; len(reasons) > 0 {
		riskCounts.Partial = true
		riskCounts.PartialReasons = reasons
	}
	if reasons := partialSections[dto.TrialAbuseSectionUsageDistribution]; len(reasons) > 0 {
		usageDistribution.Partial = true
		usageDistribution.PartialReasons = reasons
	}
	if reasons := partialSections[dto.TrialAbuseSectionIPClusters]; len(reasons) > 0 {
		for i := range ipClusters {
			ipClusters[i].Partial = true
			ipClusters[i].PartialReasons = reasons
		}
	}
	if reasons := partialSections[dto.TrialAbuseSectionInviterClusters]; len(reasons) > 0 {
		for i := range inviterClusters {
			inviterClusters[i].Partial = true
			inviterClusters[i].PartialReasons = reasons
		}
	}
	if reasons := partialSections[dto.TrialAbuseSectionSelfInviteChains]; len(reasons) > 0 {
		for i := range selfInviteChains {
			selfInviteChains[i].Partial = true
			selfInviteChains[i].PartialReasons = reasons
		}
	}
	if reasons := partialSections[dto.TrialAbuseSectionRiskUsers]; len(reasons) > 0 {
		for i := range riskUsers {
			riskUsers[i].Partial = true
			riskUsers[i].PartialReasons = reasons
		}
	}
}

func applyWarningsToRiskUser(user *dto.TrialAbuseRiskUser, warnings []dto.TrialAbuseWarning) {
	for _, warning := range warnings {
		switch warning.Reason {
		case dto.TrialAbuseWarningRegistrationIPUnavailable:
			user.RiskReasons = appendIfMissing(user.RiskReasons, dto.TrialAbuseRiskReasonRegistrationIPUnavailable)
		case dto.TrialAbuseWarningLogUnavailable:
			user.RiskReasons = appendIfMissing(user.RiskReasons, dto.TrialAbuseRiskReasonLogUnavailable)
		case dto.TrialAbuseWarningCandidateLimitExceeded:
			user.RiskReasons = appendIfMissing(user.RiskReasons, dto.TrialAbuseRiskReasonCandidateLimitExceeded)
		case dto.TrialAbuseWarningLogLimitExceeded:
			user.RiskReasons = appendIfMissing(user.RiskReasons, dto.TrialAbuseRiskReasonLogLimitExceeded)
		}
	}
}

func applyTrialAbuseRiskLimits(limit int, users *[]dto.TrialAbuseRiskUser) {
	if limit > 0 && len(*users) > limit {
		*users = (*users)[:limit]
	}
}

func appendSampleUserID(sampleUserIDs *[]int, userID int, limit int) {
	if len(*sampleUserIDs) >= limit {
		return
	}
	for _, existing := range *sampleUserIDs {
		if existing == userID {
			return
		}
	}
	*sampleUserIDs = append(*sampleUserIDs, userID)
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func ipSourceForObservedIP(ip string) string {
	if strings.TrimSpace(ip) == "" {
		return ""
	}
	return "consume_log"
}
