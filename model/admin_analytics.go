package model

import (
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

const (
	AdminAnalyticsDefaultLimit = 20
	AdminAnalyticsMaxLimit     = 100
)

type AdminAnalyticsQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	SnapshotAt     int64
	Granularity    dto.AdminAnalyticsGranularity
	Limit          int
	Offset         int
	SortBy         string
	SortOrder      dto.AdminAnalyticsSortOrder
	UserGroups     []string
	RequestGroups  []string
	PlanIDs        []int
	Sources        []dto.AdminAnalyticsSource
	Statuses       []string
}

func normalizeAdminAnalyticsQuery(query AdminAnalyticsQuery) AdminAnalyticsQuery {
	if query.SnapshotAt == 0 {
		query.SnapshotAt = time.Now().Unix()
	}
	if query.EndTimestamp == 0 {
		query.EndTimestamp = query.SnapshotAt
	}
	if query.StartTimestamp == 0 {
		query.StartTimestamp = query.EndTimestamp - 30*24*60*60
	}
	if query.Granularity == "" {
		query.Granularity = dto.AdminAnalyticsGranularityDay
	}
	if query.Limit <= 0 {
		query.Limit = AdminAnalyticsDefaultLimit
	} else if query.Limit > AdminAnalyticsMaxLimit {
		query.Limit = AdminAnalyticsMaxLimit
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.SortOrder == "" {
		query.SortOrder = dto.AdminAnalyticsSortDesc
	}
	return query
}

func adminAnalyticsRangeMeta(query AdminAnalyticsQuery) dto.AdminAnalyticsRangeMeta {
	return dto.AdminAnalyticsRangeMeta{StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp, SnapshotAt: query.SnapshotAt}
}

func applyAdminActiveSubscriptionScope(tx *gorm.DB, snapshotAt int64) *gorm.DB {
	return tx.Where("status = ? AND start_time <= ? AND end_time > ?", "active", snapshotAt, snapshotAt)
}

func buildAdminAnalyticsPage(total int, limit int, offset int) dto.AdminAnalyticsPage {
	if limit <= 0 {
		limit = AdminAnalyticsDefaultLimit
	}
	if offset < 0 {
		offset = 0
	}
	return dto.AdminAnalyticsPage{Limit: limit, Offset: offset, Total: total, HasMore: offset+limit < total}
}

func paginateAdminAnalyticsList[T any](items []T, limit int, offset int) ([]T, dto.AdminAnalyticsPage) {
	page := buildAdminAnalyticsPage(len(items), limit, offset)
	if offset >= len(items) {
		return []T{}, page
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], page
}

func normalizeAdminSubscriptionSource(grantReason string, source string) dto.AdminAnalyticsSource {
	reason := strings.TrimSpace(grantReason)
	src := strings.TrimSpace(source)
	switch reason {
	case "admin":
		return dto.AdminAnalyticsSourceAdmin
	case "trial_code":
		return dto.AdminAnalyticsSourceTrialCode
	case "invite_trial":
		return dto.AdminAnalyticsSourceInviteTrial
	case SubscriptionGrantMonthlyInviteEntitlement:
		return dto.AdminAnalyticsSourceMonthlyInviteEntitlement
	case SubscriptionGrantOrder:
		return dto.AdminAnalyticsSourceOrder
	}
	switch src {
	case "":
		return dto.AdminAnalyticsSourceUnknown
	case "order":
		return dto.AdminAnalyticsSourceOrder
	case "admin":
		return dto.AdminAnalyticsSourceAdmin
	case "redemption":
		return dto.AdminAnalyticsSourceRedemption
	case "trial_code":
		return dto.AdminAnalyticsSourceTrialCode
	case "invite_trial":
		return dto.AdminAnalyticsSourceInviteTrial
	case SubscriptionGrantMonthlyInviteEntitlement:
		return dto.AdminAnalyticsSourceMonthlyInviteEntitlement
	case "system":
		return dto.AdminAnalyticsSourceSystem
	default:
		return dto.AdminAnalyticsSourceUnknown
	}
}

type adminQuotaClass struct {
	UsageRate       *float64
	RemainingTokens *int64
	TokenUnlimited  bool
	Bucket          string
	ValidForRate    bool
	SystemRisk      bool
}

func classifyAdminSubscriptionQuota(tokenLimit int64, tokenUsed int64, normalizedSource dto.AdminAnalyticsSource) adminQuotaClass {
	if tokenLimit < 0 || tokenUsed < 0 {
		return adminQuotaClass{Bucket: "invalid", SystemRisk: true}
	}
	if tokenLimit == 0 {
		if normalizedSource == dto.AdminAnalyticsSourceTrialCode || normalizedSource == dto.AdminAnalyticsSourceInviteTrial {
			return adminQuotaClass{TokenUnlimited: true, Bucket: "unlimited_or_invalid"}
		}
		remaining := int64(0)
		return adminQuotaClass{RemainingTokens: &remaining, Bucket: "zero_limit", SystemRisk: true}
	}
	remaining := tokenLimit - tokenUsed
	if remaining < 0 {
		remaining = 0
	}
	rate := float64(tokenUsed) / float64(tokenLimit)
	bucket := "0_25"
	switch {
	case rate > 1:
		bucket = "over_100"
	case rate >= 0.9:
		bucket = "90_100"
	case rate >= 0.75:
		bucket = "75_90"
	case rate >= 0.5:
		bucket = "50_75"
	case rate >= 0.25:
		bucket = "25_50"
	}
	return adminQuotaClass{UsageRate: &rate, RemainingTokens: &remaining, Bucket: bucket, ValidForRate: true}
}

type adminActiveSubscriptionRow struct {
	Subscription UserSubscription
	Plan         SubscriptionPlan
	User         User
	Source       dto.AdminAnalyticsSource
	Quota        adminQuotaClass
}

func loadAdminActiveSubscriptions(query AdminAnalyticsQuery) ([]adminActiveSubscriptionRow, error) {
	query = normalizeAdminAnalyticsQuery(query)
	var subs []UserSubscription
	db := applyAdminActiveSubscriptionScope(DB.Model(&UserSubscription{}), query.SnapshotAt)
	if len(query.PlanIDs) > 0 {
		db = db.Where("plan_id IN ?", query.PlanIDs)
	}
	if err := db.Find(&subs).Error; err != nil {
		return nil, err
	}
	userIDs := make([]int, 0, len(subs))
	planIDs := make([]int, 0, len(subs))
	for i := range subs {
		userIDs = append(userIDs, subs[i].UserId)
		planIDs = append(planIDs, subs[i].PlanId)
	}
	users, err := adminUsersByID(userIDs)
	if err != nil {
		return nil, err
	}
	plans, err := adminPlansByID(planIDs)
	if err != nil {
		return nil, err
	}
	rows := make([]adminActiveSubscriptionRow, 0, len(subs))
	for i := range subs {
		sub := subs[i]
		user := users[sub.UserId]
		if len(query.UserGroups) > 0 && !adminStringInSet(user.Group, query.UserGroups) {
			continue
		}
		source := normalizeAdminSubscriptionSource(sub.GrantReason, sub.Source)
		if len(query.Sources) > 0 && !adminSourceInSet(source, query.Sources) {
			continue
		}
		rows = append(rows, adminActiveSubscriptionRow{Subscription: sub, Plan: plans[sub.PlanId], User: user, Source: source, Quota: classifyAdminSubscriptionQuota(sub.TokenLimit, sub.TokenUsed, source)})
	}
	return rows, nil
}

func adminUsersByID(userIDs []int) (map[int]User, error) {
	result := make(map[int]User, len(userIDs))
	ids := adminUniquePositiveInts(userIDs)
	if len(ids) == 0 {
		return result, nil
	}
	var users []User
	if err := DB.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		result[users[i].Id] = users[i]
	}
	return result, nil
}

func adminPlansByID(planIDs []int) (map[int]SubscriptionPlan, error) {
	result := make(map[int]SubscriptionPlan, len(planIDs))
	ids := adminUniquePositiveInts(planIDs)
	if len(ids) == 0 {
		return result, nil
	}
	var plans []SubscriptionPlan
	if err := DB.Where("id IN ?", ids).Find(&plans).Error; err != nil {
		return nil, err
	}
	for i := range plans {
		result[plans[i].Id] = plans[i]
	}
	return result, nil
}

func adminUniquePositiveInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func adminStringInSet(value string, set []string) bool {
	for _, item := range set {
		if value == item {
			return true
		}
	}
	return false
}

func adminSourceInSet(value dto.AdminAnalyticsSource, set []dto.AdminAnalyticsSource) bool {
	for _, item := range set {
		if value == item {
			return true
		}
	}
	return false
}

func GetAdminAnalyticsOverview(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	rows, err := loadAdminActiveSubscriptions(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{}, err
	}
	var totalUsers, disabledUsers, newUsers int64
	if err := DB.Model(&User{}).Count(&totalUsers).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{}, err
	}
	if err := DB.Model(&User{}).Where("status <> ?", common.UserStatusEnabled).Count(&disabledUsers).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{}, err
	}
	if err := DB.Model(&User{}).Where("created_at >= ? AND created_at <= ?", query.StartTimestamp, query.EndTimestamp).Count(&newUsers).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{}, err
	}
	var totalPlans, enabledPlans, trialPlans, publicPlans int64
	if err := DB.Model(&SubscriptionPlan{}).Count(&totalPlans).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{}, err
	}
	if err := DB.Model(&SubscriptionPlan{}).Where("enabled = ?", true).Count(&enabledPlans).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{}, err
	}
	if err := DB.Model(&SubscriptionPlan{}).Where("is_trial = ? OR invite_trial = ?", true, true).Count(&trialPlans).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{}, err
	}
	if err := DB.Model(&SubscriptionPlan{}).Where("public_visible = ?", true).Count(&publicPlans).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{}, err
	}
	activeUsers := map[int]struct{}{}
	activePlans := map[int]struct{}{}
	var tokenLimit, tokenUsed, remaining int64
	var trialCount, paidCount, rewardCount int
	for i := range rows {
		row := rows[i]
		activeUsers[row.Subscription.UserId] = struct{}{}
		activePlans[row.Subscription.PlanId] = struct{}{}
		if row.Subscription.TokenLimit > 0 {
			tokenLimit += row.Subscription.TokenLimit
			tokenUsed += row.Subscription.TokenUsed
			if row.Quota.RemainingTokens != nil {
				remaining += *row.Quota.RemainingTokens
			}
		}
		switch row.Source {
		case dto.AdminAnalyticsSourceTrialCode, dto.AdminAnalyticsSourceInviteTrial:
			trialCount++
		case dto.AdminAnalyticsSourceOrder:
			paidCount++
		case dto.AdminAnalyticsSourceMonthlyInviteEntitlement:
			rewardCount++
		}
	}
	var usageRate *float64
	if tokenLimit > 0 {
		rate := float64(tokenUsed) / float64(tokenLimit)
		usageRate = &rate
	}
	invitationSummary, err := getAdminInvitationSummary(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{}, err
	}
	data := dto.AdminAnalyticsOverviewResponse{Summary: dto.AdminAnalyticsOverviewSummary{
		Users:         dto.AdminAnalyticsOverviewUsers{TotalUsers: int(totalUsers), ActiveUsers: len(activeUsers), NewUsers: int(newUsers), DisabledUsers: int(disabledUsers)},
		Plans:         dto.AdminAnalyticsOverviewPlans{TotalPlans: int(totalPlans), EnabledPlans: int(enabledPlans), TrialPlans: int(trialPlans), PublicPlans: int(publicPlans)},
		Quota:         dto.AdminAnalyticsOverviewQuota{TokenLimit: tokenLimit, TokenUsed: tokenUsed, RemainingTokens: remaining, UsageRate: usageRate},
		Conversion:    dto.AdminAnalyticsOverviewConversion{TrialUsers: trialCount, PaidUsers: paidCount},
		Invitations:   invitationSummary,
		Subscriptions: dto.AdminAnalyticsOverviewSubscriptions{ActiveCount: len(rows), TrialCount: trialCount, PaidCount: paidCount, RewardCount: rewardCount},
	}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsOverviewResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
}

func GetAdminAnalyticsPlanDistribution(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsPlanDistributionResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	rows, err := loadAdminActiveSubscriptions(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsPlanDistributionResponse]{}, err
	}
	groupsByKey := make(map[int]*dto.AdminAnalyticsPlanGroup)
	for i := range rows {
		row := rows[i]
		group := groupsByKey[row.Subscription.PlanId]
		if group == nil {
			group = &dto.AdminAnalyticsPlanGroup{PlanID: row.Subscription.PlanId, PlanTitle: row.Plan.Title, PlanBusinessCode: subscriptionTierKey(&row.Plan), Source: row.Source}
			groupsByKey[row.Subscription.PlanId] = group
		}
		group.SubscriptionCount++
		group.UserCount++
		if row.Subscription.TokenLimit > 0 {
			group.TokenLimit += row.Subscription.TokenLimit
			group.TokenUsed += row.Subscription.TokenUsed
			if row.Quota.RemainingTokens != nil {
				group.RemainingTokens += *row.Quota.RemainingTokens
			}
		}
	}
	groups := make([]dto.AdminAnalyticsPlanGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		if group.TokenLimit > 0 {
			rate := float64(group.TokenUsed) / float64(group.TokenLimit)
			group.UsageRate = &rate
		}
		if group.TokenLimit == 0 {
			group.TokenUnlimited = true
		}
		planID := group.PlanID
		group.Drilldown = &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_subscriptions", PlanID: &planID, Status: "active"}
		groups = append(groups, *group)
	}
	adminSortPlanGroups(groups, query.SortBy, query.SortOrder)
	paged, page := paginateAdminAnalyticsList(groups, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsPlanDistributionResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminAnalyticsPlanDistributionResponse{Groups: dto.AdminAnalyticsList[dto.AdminAnalyticsPlanGroup]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}}, nil
}

func GetAdminAnalyticsQuotaDistribution(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsQuotaDistributionResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	rows, err := loadAdminActiveSubscriptions(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsQuotaDistributionResponse]{}, err
	}
	bucketsByName := make(map[string]*dto.AdminAnalyticsQuotaBucket)
	rankings := make([]dto.AdminAnalyticsSubscriptionRankingItem, 0, len(rows))
	for i := range rows {
		row := rows[i]
		bucketName := row.Quota.Bucket
		bucket := bucketsByName[bucketName]
		if bucket == nil {
			bucket = &dto.AdminAnalyticsQuotaBucket{Bucket: bucketName}
			bucketsByName[bucketName] = bucket
		}
		bucket.SubscriptionCount++
		bucket.UserCount++
		if row.Subscription.TokenLimit > 0 {
			bucket.TokenLimit += row.Subscription.TokenLimit
			bucket.TokenUsed += row.Subscription.TokenUsed
		}
		userID := row.Subscription.UserId
		planID := row.Subscription.PlanId
		remaining := int64(0)
		if row.Quota.RemainingTokens != nil {
			remaining = *row.Quota.RemainingTokens
		}
		rankings = append(rankings, dto.AdminAnalyticsSubscriptionRankingItem{SubscriptionID: row.Subscription.Id, UserID: row.Subscription.UserId, Username: row.User.Username, UserGroup: row.User.Group, PlanID: row.Subscription.PlanId, PlanTitle: row.Plan.Title, Source: row.Source, Status: row.Subscription.Status, StartTime: row.Subscription.StartTime, EndTime: row.Subscription.EndTime, TokenLimit: row.Subscription.TokenLimit, TokenUsed: row.Subscription.TokenUsed, RemainingTokens: remaining, UsageRate: row.Quota.UsageRate, Drilldown: &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_users", UserID: &userID, PlanID: &planID}})
	}
	buckets := make([]dto.AdminAnalyticsQuotaBucket, 0, len(bucketsByName))
	for _, bucket := range bucketsByName {
		if bucket.TokenLimit > 0 {
			rate := float64(bucket.TokenUsed) / float64(bucket.TokenLimit)
			bucket.UsageRate = &rate
		}
		buckets = append(buckets, *bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return adminQuotaBucketRank(buckets[i].Bucket) < adminQuotaBucketRank(buckets[j].Bucket)
	})
	sort.Slice(rankings, func(i, j int) bool {
		left, right := float64(-1), float64(-1)
		if rankings[i].UsageRate != nil {
			left = *rankings[i].UsageRate
		}
		if rankings[j].UsageRate != nil {
			right = *rankings[j].UsageRate
		}
		return left > right
	})
	highItems, highPage := paginateAdminAnalyticsList(rankings, query.Limit, query.Offset)
	idle := append([]dto.AdminAnalyticsSubscriptionRankingItem(nil), rankings...)
	sort.Slice(idle, func(i, j int) bool {
		left, right := float64(2), float64(2)
		if idle[i].UsageRate != nil {
			left = *idle[i].UsageRate
		}
		if idle[j].UsageRate != nil {
			right = *idle[j].UsageRate
		}
		return left < right
	})
	idleItems, idlePage := paginateAdminAnalyticsList(idle, query.Limit, query.Offset)
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsQuotaDistributionResponse]{Range: adminAnalyticsRangeMeta(query), Data: dto.AdminAnalyticsQuotaDistributionResponse{Buckets: buckets, HighUsageUsers: dto.AdminAnalyticsList[dto.AdminAnalyticsSubscriptionRankingItem]{Items: highItems, Page: highPage, SortBy: "usage_rate", SortOrder: dto.AdminAnalyticsSortDesc}, IdleSubscriptions: dto.AdminAnalyticsList[dto.AdminAnalyticsSubscriptionRankingItem]{Items: idleItems, Page: idlePage, SortBy: "usage_rate", SortOrder: dto.AdminAnalyticsSortAsc}, ExhaustingSubscriptions: dto.AdminAnalyticsList[dto.AdminAnalyticsSubscriptionRankingItem]{Items: highItems, Page: highPage, SortBy: "usage_rate", SortOrder: dto.AdminAnalyticsSortDesc}}}, nil
}

func GetAdminAnalyticsUserLifecycle(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsUserLifecycleResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	var users []User
	if err := DB.Find(&users).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsUserLifecycleResponse]{}, err
	}
	rows, err := loadAdminActiveSubscriptions(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsUserLifecycleResponse]{}, err
	}
	activeByUser := make(map[int]adminActiveSubscriptionRow, len(rows))
	for i := range rows {
		if _, ok := activeByUser[rows[i].Subscription.UserId]; !ok {
			activeByUser[rows[i].Subscription.UserId] = rows[i]
		}
	}
	items := make([]dto.AdminAnalyticsUserLifecycleItem, 0, len(users))
	groupCounts := make(map[string]int)
	var newUsers, disabledUsers int
	for i := range users {
		user := users[i]
		if len(query.UserGroups) > 0 && !adminStringInSet(user.Group, query.UserGroups) {
			continue
		}
		if user.CreatedAt >= query.StartTimestamp && user.CreatedAt <= query.EndTimestamp {
			newUsers++
		}
		if user.Status != common.UserStatusEnabled {
			disabledUsers++
		}
		groupCounts[user.Group]++
		item := dto.AdminAnalyticsUserLifecycleItem{UserID: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email, UserGroup: user.Group, Status: user.Status, CreatedAt: user.CreatedAt, LastLoginAt: user.LastLoginAt, RequestCount: user.RequestCount}
		if row, ok := activeByUser[user.Id]; ok {
			item.ActivePlanID = row.Subscription.PlanId
			item.ActivePlanTitle = row.Plan.Title
			item.ActiveSource = row.Source
			item.TokenLimit = row.Subscription.TokenLimit
			item.TokenUsed = row.Subscription.TokenUsed
		}
		userID := user.Id
		item.Drilldown = &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_users", UserID: &userID}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UserID < items[j].UserID })
	paged, page := paginateAdminAnalyticsList(items, query.Limit, query.Offset)
	userGroups := make([]dto.AdminAnalyticsUserGroupDistribution, 0, len(groupCounts))
	for group, count := range groupCounts {
		share := 0.0
		if len(items) > 0 {
			share = float64(count) / float64(len(items))
		}
		userGroups = append(userGroups, dto.AdminAnalyticsUserGroupDistribution{Group: group, UserCount: count, Share: share})
	}
	sort.Slice(userGroups, func(i, j int) bool { return userGroups[i].UserCount > userGroups[j].UserCount })
	data := dto.AdminAnalyticsUserLifecycleResponse{Summary: dto.AdminAnalyticsUserLifecycleSummary{TotalUsers: len(items), NewUsers: newUsers, ActiveUsers: len(activeByUser), DisabledUsers: disabledUsers}, UserGroups: userGroups, Users: dto.AdminAnalyticsList[dto.AdminAnalyticsUserLifecycleItem]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsUserLifecycleResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
}

func GetAdminAnalyticsSubscriptionConversion(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsSubscriptionConversionResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	rows, err := loadAdminActiveSubscriptions(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsSubscriptionConversionResponse]{}, err
	}
	trialUsers := map[int]struct{}{}
	paidUsers := map[int]struct{}{}
	for i := range rows {
		switch rows[i].Source {
		case dto.AdminAnalyticsSourceTrialCode, dto.AdminAnalyticsSourceInviteTrial:
			trialUsers[rows[i].Subscription.UserId] = struct{}{}
		case dto.AdminAnalyticsSourceOrder:
			paidUsers[rows[i].Subscription.UserId] = struct{}{}
		}
	}
	trialToPaid := 0
	for userID := range trialUsers {
		if _, ok := paidUsers[userID]; ok {
			trialToPaid++
		}
	}
	rate := 0.0
	if len(trialUsers) > 0 {
		rate = float64(trialToPaid) / float64(len(trialUsers))
	}
	data := dto.AdminAnalyticsSubscriptionConversionResponse{Summary: dto.AdminAnalyticsSubscriptionConversionSummary{TrialUsers: len(trialUsers), PaidUsers: len(paidUsers), TrialToPaidUsers: trialToPaid, TrialToPaidRate: rate}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsSubscriptionConversionResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
}

func GetAdminAnalyticsInvitationRewards(query AdminAnalyticsQuery) (dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsInvitationRewardsResponse], error) {
	query = normalizeAdminAnalyticsQuery(query)
	summary, err := getAdminInvitationSummary(query)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsInvitationRewardsResponse]{}, err
	}
	var users []User
	if err := DB.Where("inviter_id > ?", 0).Find(&users).Error; err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsInvitationRewardsResponse]{}, err
	}
	byInviter := make(map[int]*dto.AdminAnalyticsInviterItem)
	for i := range users {
		user := users[i]
		item := byInviter[user.InviterId]
		if item == nil {
			inviterID := user.InviterId
			item = &dto.AdminAnalyticsInviterItem{InviterID: inviterID, Drilldown: &dto.AdminAnalyticsDrilldownTarget{Kind: "admin_invitations", InviterID: &inviterID}}
			byInviter[user.InviterId] = item
		}
		item.DirectInviteCount++
	}
	inviterIDs := make([]int, 0, len(byInviter))
	for inviterID := range byInviter {
		inviterIDs = append(inviterIDs, inviterID)
	}
	inviters, err := adminUsersByID(inviterIDs)
	if err != nil {
		return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsInvitationRewardsResponse]{}, err
	}
	items := make([]dto.AdminAnalyticsInviterItem, 0, len(byInviter))
	for inviterID, item := range byInviter {
		item.InviterUsername = inviters[inviterID].Username
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DirectInviteCount > items[j].DirectInviteCount })
	paged, page := paginateAdminAnalyticsList(items, query.Limit, query.Offset)
	data := dto.AdminAnalyticsInvitationRewardsResponse{Summary: dto.AdminAnalyticsInvitationRewardsSummary{UsersWithInviter: summary.UsersWithInviter, InvitersCount: summary.InvitersCount, DirectInviteCount: summary.DirectInviteCount, QualifiedInviteCount: summary.QualifiedInviteCount, RewardUsers: summary.RewardUsers, RewardSubscriptions: summary.RewardSubscriptions}, Inviters: dto.AdminAnalyticsList[dto.AdminAnalyticsInviterItem]{Items: paged, Page: page, SortBy: query.SortBy, SortOrder: query.SortOrder}}
	return dto.AdminAnalyticsPanelResponse[dto.AdminAnalyticsInvitationRewardsResponse]{Range: adminAnalyticsRangeMeta(query), Data: data}, nil
}

func getAdminInvitationSummary(query AdminAnalyticsQuery) (dto.AdminAnalyticsOverviewInvitations, error) {
	var usersWithInviter int64
	if err := DB.Model(&User{}).Where("inviter_id > ?", 0).Count(&usersWithInviter).Error; err != nil {
		return dto.AdminAnalyticsOverviewInvitations{}, err
	}
	var inviterIDs []int
	if err := DB.Model(&User{}).Where("inviter_id > ?", 0).Distinct("inviter_id").Pluck("inviter_id", &inviterIDs).Error; err != nil {
		return dto.AdminAnalyticsOverviewInvitations{}, err
	}
	var rewardSubs int64
	if err := DB.Model(&UserSubscription{}).Where("grant_reason = ? OR source = ?", SubscriptionGrantMonthlyInviteEntitlement, string(dto.AdminAnalyticsSourceMonthlyInviteEntitlement)).Count(&rewardSubs).Error; err != nil {
		return dto.AdminAnalyticsOverviewInvitations{}, err
	}
	var rewardActive int64
	if err := applyAdminActiveSubscriptionScope(DB.Model(&UserSubscription{}), query.SnapshotAt).Where("grant_reason = ? OR source = ?", SubscriptionGrantMonthlyInviteEntitlement, string(dto.AdminAnalyticsSourceMonthlyInviteEntitlement)).Count(&rewardActive).Error; err != nil {
		return dto.AdminAnalyticsOverviewInvitations{}, err
	}
	return dto.AdminAnalyticsOverviewInvitations{UsersWithInviter: int(usersWithInviter), InvitersCount: len(inviterIDs), DirectInviteCount: int(usersWithInviter), RewardUsers: int(rewardActive), RewardSubscriptions: int(rewardSubs), RewardActiveSubscriptionCount: int(rewardActive), RewardExpiredSubscriptionCount: int(rewardSubs - rewardActive)}, nil
}

func adminSortPlanGroups(groups []dto.AdminAnalyticsPlanGroup, sortBy string, order dto.AdminAnalyticsSortOrder) {
	desc := order != dto.AdminAnalyticsSortAsc
	sort.Slice(groups, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "user_count":
			less = groups[i].UserCount < groups[j].UserCount
		case "token_used":
			less = groups[i].TokenUsed < groups[j].TokenUsed
		case "usage_rate":
			left, right := -1.0, -1.0
			if groups[i].UsageRate != nil {
				left = *groups[i].UsageRate
			}
			if groups[j].UsageRate != nil {
				right = *groups[j].UsageRate
			}
			less = left < right
		default:
			less = groups[i].SubscriptionCount < groups[j].SubscriptionCount
		}
		if desc {
			return !less
		}
		return less
	})
}

func adminQuotaBucketRank(bucket string) int {
	switch bucket {
	case "invalid":
		return 0
	case "unlimited_or_invalid":
		return 1
	case "zero_limit":
		return 2
	case "0_25":
		return 3
	case "25_50":
		return 4
	case "50_75":
		return 5
	case "75_90":
		return 6
	case "90_100":
		return 7
	case "over_100":
		return 8
	default:
		return 99
	}
}
