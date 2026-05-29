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

	activeSubs, err := loadAdminOpsPrimaryActiveSubscriptions(uniqueUserIDs, usersByID)
	if err != nil {
		return nil, err
	}
	if len(activeSubs) == 0 {
		return result, nil
	}

	planIDs := make([]int, 0, len(activeSubs))
	seenPlanIDs := make(map[int]struct{}, len(activeSubs))
	for _, sub := range activeSubs {
		if sub.PlanId <= 0 {
			continue
		}
		if _, ok := seenPlanIDs[sub.PlanId]; ok {
			continue
		}
		seenPlanIDs[sub.PlanId] = struct{}{}
		planIDs = append(planIDs, sub.PlanId)
	}

	plans := make(map[int]SubscriptionPlan, len(planIDs))
	if len(planIDs) > 0 {
		var planRows []SubscriptionPlan
		if err := DB.Where("id IN ?", planIDs).Find(&planRows).Error; err != nil {
			return nil, err
		}
		for _, plan := range planRows {
			plans[plan.Id] = plan
		}
	}

	defaultQueueCapacity := runtimeDefaultAdminOpsQueueCapacity()
	for _, sub := range activeSubs {
		entry := result[sub.UserId]
		entry.UserID = sub.UserId
		entry.QueueCapacity = defaultQueueCapacity
		if plan, ok := plans[sub.PlanId]; ok {
			entry.Limit = plan.ConcurrencyLimit
			if plan.QueueCapacity > 0 {
				entry.QueueCapacity = plan.QueueCapacity
			}
		} else {
			entry.Limit = sub.ConcurrencyLimit
		}
		if entry.QueueCapacity <= 0 {
			entry.QueueCapacity = defaultQueueCapacity
		}
		result[sub.UserId] = entry
	}
	return result, nil
}

func loadAdminOpsPrimaryActiveSubscriptions(userIDs []int, usersByID map[int]User) ([]UserSubscription, error) {
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("user_id IN ? AND status = ? AND end_time > ?", userIDs, "active", now).
		Order(primaryBillableSubscriptionOrder).
		Find(&subs).Error; err != nil {
		return nil, err
	}

	selected := make([]UserSubscription, 0, len(userIDs))
	seen := make(map[int]struct{}, len(userIDs))
	deferred := make([]UserSubscription, 0)
	for _, sub := range subs {
		if _, ok := seen[sub.UserId]; ok {
			continue
		}
		if user, ok := usersByID[sub.UserId]; ok {
			activeID := user.GetSetting().ActiveSubscriptionId
			if activeID > 0 && sub.Id == activeID {
				seen[sub.UserId] = struct{}{}
				selected = append(selected, sub)
				continue
			}
		}
		deferred = append(deferred, sub)
	}
	for _, sub := range deferred {
		if _, ok := seen[sub.UserId]; ok {
			continue
		}
		seen[sub.UserId] = struct{}{}
		selected = append(selected, sub)
	}
	return selected, nil
}

func runtimeDefaultAdminOpsQueueCapacity() int {
	if common.SubscriptionConcurrencyQueueCapacity > 0 {
		return common.SubscriptionConcurrencyQueueCapacity
	}
	return 1
}
