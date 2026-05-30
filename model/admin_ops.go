package model

import (
	"github.com/QuantumNous/new-api/common"
)

type AdminOpsTrafficStats struct {
	Requests    int64
	Errors      int64
	TotalTokens int64
}

type adminOpsTrafficLogRow struct {
	Type             int
	PromptTokens     int
	CompletionTokens int
	MeteredTokens    *int
}

func GetAdminOpsTrafficStats(startTimestamp int64, endTimestamp int64) (AdminOpsTrafficStats, error) {
	var logs []adminOpsTrafficLogRow
	err := LOG_DB.Model(&Log{}).
		Select("type", "prompt_tokens", "completion_tokens", "metered_tokens").
		Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
		Where("created_at >= ? AND created_at <= ?", startTimestamp, endTimestamp).
		Find(&logs).Error
	if err != nil {
		return AdminOpsTrafficStats{}, err
	}

	stats := AdminOpsTrafficStats{Requests: int64(len(logs))}
	for _, log := range logs {
		if log.Type == LogTypeError {
			stats.Errors++
		}
		stats.TotalTokens += adminOpsLogTokens(log)
	}
	return stats, nil
}

func adminOpsLogTokens(log adminOpsTrafficLogRow) int64 {
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

type AdminOpsChannelStats struct {
	Total          int64
	Enabled        int64
	ManualDisabled int64
	AutoDisabled   int64
	SlowCount      int64
	StaleTestCount int64
}

func GetAdminOpsChannelStats(now int64, slowThresholdMs int, staleAfterSeconds int64) (AdminOpsChannelStats, error) {
	var channels []Channel
	if err := DB.Model(&Channel{}).Select("status, response_time, test_time").Find(&channels).Error; err != nil {
		return AdminOpsChannelStats{}, err
	}

	stats := AdminOpsChannelStats{Total: int64(len(channels))}
	staleBefore := now - staleAfterSeconds
	for _, channel := range channels {
		switch channel.Status {
		case common.ChannelStatusEnabled:
			stats.Enabled++
		case common.ChannelStatusManuallyDisabled:
			stats.ManualDisabled++
		case common.ChannelStatusAutoDisabled:
			stats.AutoDisabled++
		}
		if slowThresholdMs > 0 && channel.ResponseTime >= slowThresholdMs {
			stats.SlowCount++
		}
		if staleAfterSeconds > 0 && channel.TestTime <= staleBefore {
			stats.StaleTestCount++
		}
	}
	return stats, nil
}

func GetAdminOpsRecentErrors(startTimestamp int64, limit int) ([]*Log, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	var logs []*Log
	err := LOG_DB.Model(&Log{}).
		Where("type = ?", LogTypeError).
		Where("created_at >= ?", startTimestamp).
		Order("created_at desc, id desc").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

type AdminOpsUserConcurrencyLimit struct {
	UserID        int
	Username      string
	Limit         int
	QueueCapacity int
	PlanID        int
	PlanTitle     string
	PlanCode      string
	AmountTotal   int64
	AmountUsed    int64
	TokenLimit    int64
	TokenUsed     int64
}

func GetAdminOpsUserConcurrencyLimits(userIDs []int) (map[int]AdminOpsUserConcurrencyLimit, error) {
	result := make(map[int]AdminOpsUserConcurrencyLimit, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	uniqueUserIDs := make([]int, 0, len(userIDs))
	seen := make(map[int]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		uniqueUserIDs = append(uniqueUserIDs, userID)
		result[userID] = AdminOpsUserConcurrencyLimit{
			UserID:        userID,
			QueueCapacity: runtimeDefaultAdminOpsQueueCapacity(),
		}
	}
	if len(uniqueUserIDs) == 0 {
		return result, nil
	}

	var users []User
	if err := DB.Model(&User{}).Select("id, username, setting").Where("id IN ?", uniqueUserIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	usersByID := make(map[int]User, len(users))
	for _, user := range users {
		usersByID[user.Id] = user
		entry := result[user.Id]
		entry.Username = user.Username
		result[user.Id] = entry
	}

	defaultQueueCapacity := runtimeDefaultAdminOpsQueueCapacity()
	now := GetDBTimestamp()
	for _, userID := range uniqueUserIDs {
		selection, err := selectAdminOpsPrimarySubscription(userID, usersByID[userID], now)
		if err != nil {
			return nil, err
		}
		if selection == nil {
			continue
		}
		entry := result[userID]
		fillAdminOpsUserConcurrencyLimitFromSelection(&entry, selection, defaultQueueCapacity)
		result[userID] = entry
	}
	return result, nil
}

func selectAdminOpsPrimarySubscription(userID int, user User, now int64) (*primaryBillableSubscription, error) {
	var subs []UserSubscription
	if err := DB.Where("user_id = ? AND status = ? AND end_time > ?", userID, "active", now).
		Order(primaryBillableSubscriptionOrder).
		Find(&subs).Error; err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, nil
	}
	activeID := user.GetSetting().ActiveSubscriptionId
	defaultCandidate, rewardCandidate, err := buildAdminOpsSubscriptionCandidates(subs, activeID)
	if err != nil {
		return nil, err
	}
	if rewardCandidate != nil {
		return buildPrimaryBillableSubscription(*rewardCandidate), nil
	}
	if defaultCandidate != nil {
		return buildPrimaryBillableSubscription(*defaultCandidate), nil
	}
	return nil, nil
}

func buildAdminOpsSubscriptionCandidates(subs []UserSubscription, activeID int) (*billableSubscriptionCandidate, *billableSubscriptionCandidate, error) {
	candidates := make([]billableSubscriptionCandidate, 0, len(subs))
	for i, candidate := range subs {
		sub := candidate
		plan, err := getAdminOpsSubscriptionPlan(sub.PlanId)
		if err != nil {
			return nil, nil, err
		}
		entry := billableSubscriptionCandidate{sub: sub, plan: plan, distributor: isDistributorSubscription(&sub, plan), index: i}
		ok, unlimited := isBillableSubscriptionCandidate(&sub, plan, 1)
		if !entry.distributor || !ok {
			continue
		}
		entry.unlimited = unlimited
		if activeID > 0 && sub.Id == activeID {
			return &entry, nil, nil
		}
		candidates = append(candidates, entry)
	}
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	defaultCandidate := candidates[0]
	if isPaidSubscription(&defaultCandidate.sub) {
		tier := subscriptionTierKey(defaultCandidate.plan)
		if tier != "" {
			for i := 1; i < len(candidates); i++ {
				candidate := candidates[i]
				if isInvitationRewardSubscription(&candidate.sub) && subscriptionTierKey(candidate.plan) == tier {
					return &defaultCandidate, &candidate, nil
				}
			}
		}
	}
	return &defaultCandidate, nil, nil
}

func getAdminOpsSubscriptionPlan(planID int) (*SubscriptionPlan, error) {
	if planID <= 0 {
		return nil, nil
	}
	var plan SubscriptionPlan
	if err := DB.Where("id = ?", planID).First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func fillAdminOpsUserConcurrencyLimitFromSelection(entry *AdminOpsUserConcurrencyLimit, selection *primaryBillableSubscription, defaultQueueCapacity int) {
	if entry == nil || selection == nil {
		return
	}
	sub := selection.Subscription
	entry.UserID = sub.UserId
	entry.PlanID = sub.PlanId
	entry.AmountTotal = sub.AmountTotal
	entry.AmountUsed = sub.AmountUsed
	entry.TokenLimit = sub.TokenLimit
	entry.TokenUsed = sub.TokenUsed
	entry.Limit = livePlanConcurrencyLimit(&sub, selection.Plan)
	entry.QueueCapacity = defaultQueueCapacity
	if selection.Plan != nil {
		entry.PlanTitle = selection.Plan.Title
		if selection.Plan.BusinessCode != nil {
			entry.PlanCode = *selection.Plan.BusinessCode
		}
		if selection.Plan.QueueCapacity > 0 {
			entry.QueueCapacity = selection.Plan.QueueCapacity
		}
	}
	if entry.QueueCapacity <= 0 {
		entry.QueueCapacity = defaultQueueCapacity
	}
}

func runtimeDefaultAdminOpsQueueCapacity() int {
	if common.SubscriptionConcurrencyQueueCapacity > 0 {
		return common.SubscriptionConcurrencyQueueCapacity
	}
	return 1
}
