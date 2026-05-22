package service

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const (
	monthlyInviteEntitlementReason = "monthly_invite_entitlement"
	monthlyInviteQualifiedCount    = 2
)

type InvitationEntitlementStatus struct {
	DirectInviteCount                 int    `json:"direct_invite_count"`
	QualifiedActiveCount              int    `json:"qualified_active_count"`
	RewardMonth                       string `json:"reward_month"`
	Entitled                          bool   `json:"entitled"`
	EntitlementEndTime                int64  `json:"entitlement_end_time"`
	RewardSubscriptionId              int    `json:"reward_subscription_id"`
	RewardPlanId                      int    `json:"reward_plan_id"`
	RewardPlanTitle                   string `json:"reward_plan_title"`
	RewardPlanBusinessCode            string `json:"reward_plan_business_code"`
	RewardTierRank                    int    `json:"reward_tier_rank"`
	RewardTierQualifiedCount          int    `json:"reward_tier_qualified_count"`
	DowngradeRewardPlanId             int    `json:"downgrade_reward_plan_id"`
	DowngradeRewardPlanTitle          string `json:"downgrade_reward_plan_title"`
	DowngradeRewardPlanBusinessCode   string `json:"downgrade_reward_plan_business_code"`
	DowngradeRewardTierRank           int    `json:"downgrade_reward_tier_rank"`
	DowngradeRewardTierQualifiedCount int    `json:"downgrade_reward_tier_qualified_count"`
	DowngradeEntitlementEndTime       int64  `json:"downgrade_entitlement_end_time"`
}

type invitationRewardCandidate struct {
	Plan           model.SubscriptionPlan
	TierRank       int
	QualifiedCount int
	EndTime        int64
}

type qualifiedInviteePlanEndTime struct {
	InviteeId         int
	PlanId            int
	EndTime           int64
	SortOrder         int
	PriceAmount       float64
	MonthlyTokenLimit int64
	ConcurrencyLimit  int
}

func (c invitationRewardCandidate) applyToStatus(status *InvitationEntitlementStatus) {
	status.RewardPlanId = c.Plan.Id
	status.RewardPlanTitle = c.Plan.Title
	status.RewardPlanBusinessCode = subscriptionPlanBusinessCode(&c.Plan)
	status.RewardTierRank = c.TierRank
	status.RewardTierQualifiedCount = c.QualifiedCount
	status.EntitlementEndTime = c.EndTime
}

func (c invitationRewardCandidate) applyDowngradeToStatus(status *InvitationEntitlementStatus) {
	status.DowngradeRewardPlanId = c.Plan.Id
	status.DowngradeRewardPlanTitle = c.Plan.Title
	status.DowngradeRewardPlanBusinessCode = subscriptionPlanBusinessCode(&c.Plan)
	status.DowngradeRewardTierRank = c.TierRank
	status.DowngradeRewardTierQualifiedCount = c.QualifiedCount
	status.DowngradeEntitlementEndTime = c.EndTime
}

func EnsureMonthlyInvitationEntitlement(inviterId int, at time.Time) (*InvitationEntitlementStatus, error) {
	if inviterId <= 0 {
		return nil, errors.New("invalid inviter id")
	}
	if at.IsZero() {
		at = time.Now()
	}
	at = at.UTC()
	rewardMonth := rewardMonthString(at)
	var status InvitationEntitlementStatus
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		directCount, err := countDirectInviteesTx(tx, inviterId)
		if err != nil {
			return err
		}
		qualifiedCount, err := countQualifiedActiveInviteesTx(tx, inviterId, at.Unix())
		if err != nil {
			return err
		}
		status.DirectInviteCount = directCount
		status.QualifiedActiveCount = qualifiedCount
		status.RewardMonth = rewardMonth
		candidates, err := listInvitationRewardCandidatesTx(tx, inviterId, at.Unix())
		if err != nil {
			return err
		}
		if len(candidates) > 0 {
			status.Entitled = true
			candidates[0].applyToStatus(&status)
			for _, candidate := range candidates[1:] {
				if candidate.EndTime > status.EntitlementEndTime {
					candidate.applyDowngradeToStatus(&status)
					break
				}
			}
		}
		var entitlement model.InvitationMonthlyEntitlement
		entitlementErr := tx.Set("gorm:query_option", "FOR UPDATE").Where("inviter_id = ? AND reward_month = ?", inviterId, rewardMonth).First(&entitlement).Error
		if entitlementErr != nil && !errors.Is(entitlementErr, gorm.ErrRecordNotFound) {
			return entitlementErr
		}
		entitlementMissing := errors.Is(entitlementErr, gorm.ErrRecordNotFound)
		if len(candidates) == 0 {
			if entitlementMissing {
				entitlement = model.InvitationMonthlyEntitlement{InviterId: inviterId, RewardMonth: rewardMonth}
			}
			entitlement.QualifiedActiveCount = qualifiedCount
			entitlement.Status = model.InvitationEntitlementStatusNotQualified
			if entitlement.Id == 0 {
				return nil
			}
			return tx.Save(&entitlement).Error
		}
		if entitlementMissing {
			entitlement = model.InvitationMonthlyEntitlement{InviterId: inviterId, RewardMonth: rewardMonth}
		}
		sub, err := upsertInvitationRewardSubscriptionTx(tx, inviterId, &candidates[0].Plan, status.EntitlementEndTime, entitlement.RewardSubscriptionId)
		if err != nil {
			return err
		}
		entitlement.RewardSubscriptionId = sub.Id
		entitlement.QualifiedActiveCount = qualifiedCount
		entitlement.RewardPlanId = candidates[0].Plan.Id
		entitlement.Status = model.InvitationEntitlementStatusQualified
		if entitlement.Id == 0 {
			if err := tx.Create(&entitlement).Error; err != nil {
				return err
			}
		} else if err := tx.Save(&entitlement).Error; err != nil {
			return err
		}
		status.RewardSubscriptionId = entitlement.RewardSubscriptionId
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func GetInvitationEntitlementStatus(inviterId int, at time.Time) (*InvitationEntitlementStatus, error) {
	return EnsureMonthlyInvitationEntitlement(inviterId, at)
}

func RunMonthlyInvitationEntitlementSweep(at time.Time, limit int) (int, error) {
	if at.IsZero() {
		at = time.Now()
	}
	query := model.DB.Model(&model.User{}).Where("inviter_id > 0").Distinct("inviter_id")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var inviterIds []int
	if err := query.Pluck("inviter_id", &inviterIds).Error; err != nil {
		return 0, err
	}
	for _, inviterId := range inviterIds {
		if _, err := EnsureMonthlyInvitationEntitlement(inviterId, at); err != nil {
			return 0, err
		}
	}
	return len(inviterIds), nil
}

func countDirectInviteesTx(tx *gorm.DB, inviterId int) (int, error) {
	var count int64
	if err := tx.Model(&model.User{}).Where("inviter_id = ?", inviterId).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func countQualifiedActiveInviteesTx(tx *gorm.DB, inviterId int, now int64) (int, error) {
	var count int64
	err := tx.Model(&model.User{}).
		Joins("JOIN user_subscriptions ON user_subscriptions.user_id = users.id").
		Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Where("users.inviter_id = ?", inviterId).
		Where("user_subscriptions.status = ?", "active").
		Where("user_subscriptions.start_time <= ? AND user_subscriptions.end_time > ?", now, now).
		Where("(user_subscriptions.grant_reason = ? OR (user_subscriptions.grant_reason = ? AND user_subscriptions.source = ?))", model.SubscriptionGrantOrder, "", model.SubscriptionGrantOrder).
		Where("subscription_plans.reward_eligible = ?", true).
		Where("EXISTS (SELECT 1 FROM subscription_orders WHERE subscription_orders.user_id = users.id AND subscription_orders.plan_id = user_subscriptions.plan_id AND subscription_orders.status = ? AND subscription_orders.money > ?)", common.TopUpStatusSuccess, 0).
		Distinct("users.id").
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func listInvitationRewardCandidatesTx(tx *gorm.DB, inviterId int, now int64) ([]invitationRewardCandidate, error) {
	var rows []qualifiedInviteePlanEndTime
	err := tx.Table("users").
		Select("users.id AS invitee_id, subscription_plans.id AS plan_id, MAX(user_subscriptions.end_time) AS end_time, subscription_plans.sort_order, subscription_plans.price_amount, subscription_plans.monthly_token_limit, subscription_plans.concurrency_limit").
		Joins("JOIN user_subscriptions ON user_subscriptions.user_id = users.id").
		Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Where("users.inviter_id = ?", inviterId).
		Where("user_subscriptions.status = ?", "active").
		Where("user_subscriptions.start_time <= ? AND user_subscriptions.end_time > ?", now, now).
		Where("(user_subscriptions.grant_reason = ? OR (user_subscriptions.grant_reason = ? AND user_subscriptions.source = ?))", model.SubscriptionGrantOrder, "", model.SubscriptionGrantOrder).
		Where("subscription_plans.reward_eligible = ?", true).
		Where("EXISTS (SELECT 1 FROM subscription_orders WHERE subscription_orders.user_id = users.id AND subscription_orders.plan_id = user_subscriptions.plan_id AND subscription_orders.status = ? AND subscription_orders.money > ?)", common.TopUpStatusSuccess, 0).
		Group("users.id, subscription_plans.id, subscription_plans.sort_order, subscription_plans.price_amount, subscription_plans.monthly_token_limit, subscription_plans.concurrency_limit").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	planById := make(map[int]model.SubscriptionPlan)
	for _, row := range rows {
		planById[row.PlanId] = model.SubscriptionPlan{}
	}
	planIds := make([]int, 0, len(planById))
	for planId := range planById {
		planIds = append(planIds, planId)
	}
	var plans []model.SubscriptionPlan
	if err := tx.Where("id IN ?", planIds).Find(&plans).Error; err != nil {
		return nil, err
	}
	for _, plan := range plans {
		planById[plan.Id] = plan
	}
	planOrder := make([]model.SubscriptionPlan, 0, len(plans))
	planOrder = append(planOrder, plans...)
	sort.Slice(planOrder, func(i, j int) bool {
		return compareSubscriptionPlanTier(planOrder[i], planOrder[j]) > 0
	})
	rankByPlanId := make(map[int]int, len(planOrder))
	for idx, plan := range planOrder {
		rankByPlanId[plan.Id] = len(planOrder) - idx
	}
	inviteeEndTimesByPlan := make(map[int]map[int]int64, len(planOrder))
	for _, row := range rows {
		endTimesByInvitee, ok := inviteeEndTimesByPlan[row.PlanId]
		if !ok {
			endTimesByInvitee = make(map[int]int64)
			inviteeEndTimesByPlan[row.PlanId] = endTimesByInvitee
		}
		endTimesByInvitee[row.InviteeId] = row.EndTime
	}
	candidates := make([]invitationRewardCandidate, 0, len(planOrder))
	for _, plan := range planOrder {
		endTimeByInvitee := make(map[int]int64)
		for _, purchasedPlan := range planOrder {
			if compareSubscriptionPlanTier(purchasedPlan, plan) < 0 {
				continue
			}
			for inviteeId, endTime := range inviteeEndTimesByPlan[purchasedPlan.Id] {
				if existingEndTime, ok := endTimeByInvitee[inviteeId]; !ok || endTime > existingEndTime {
					endTimeByInvitee[inviteeId] = endTime
				}
			}
		}
		if len(endTimeByInvitee) < monthlyInviteQualifiedCount {
			continue
		}
		endTimes := make([]int64, 0, len(endTimeByInvitee))
		for _, endTime := range endTimeByInvitee {
			endTimes = append(endTimes, endTime)
		}
		sort.Slice(endTimes, func(i, j int) bool { return endTimes[i] > endTimes[j] })
		candidates = append(candidates, invitationRewardCandidate{
			Plan:           plan,
			TierRank:       rankByPlanId[plan.Id],
			QualifiedCount: len(endTimes),
			EndTime:        endTimes[monthlyInviteQualifiedCount-1],
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareSubscriptionPlanTier(candidates[i].Plan, candidates[j].Plan) > 0
	})
	return candidates, nil
}

func compareSubscriptionPlanTier(left model.SubscriptionPlan, right model.SubscriptionPlan) int {
	if left.PriceAmount != right.PriceAmount {
		if left.PriceAmount > right.PriceAmount {
			return 1
		}
		return -1
	}
	if left.SortOrder != right.SortOrder {
		if left.SortOrder > right.SortOrder {
			return 1
		}
		return -1
	}
	if left.MonthlyTokenLimit != right.MonthlyTokenLimit {
		if left.MonthlyTokenLimit > right.MonthlyTokenLimit {
			return 1
		}
		return -1
	}
	if left.ConcurrencyLimit != right.ConcurrencyLimit {
		if left.ConcurrencyLimit > right.ConcurrencyLimit {
			return 1
		}
		return -1
	}
	if left.Id > right.Id {
		return 1
	}
	if left.Id < right.Id {
		return -1
	}
	return 0
}

func upsertInvitationRewardSubscriptionTx(tx *gorm.DB, userId int, plan *model.SubscriptionPlan, endTime int64, rewardSubscriptionId int) (*model.UserSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if plan == nil || plan.Id == 0 {
		return nil, errors.New("invalid plan")
	}
	now := common.GetTimestamp()
	nextReset := calcInvitationRewardNextResetTime(now, plan, endTime)
	fields := map[string]interface{}{
		"plan_id":              plan.Id,
		"amount_total":         plan.TotalAmount,
		"amount_used":          0,
		"token_limit":          plan.MonthlyTokenLimit,
		"concurrency_limit":    plan.ConcurrencyLimit,
		"grant_reason":         monthlyInviteEntitlementReason,
		"grant_source_user_id": userId,
		"start_time":           now,
		"end_time":             endTime,
		"status":               "active",
		"source":               monthlyInviteEntitlementReason,
		"next_reset_time":      nextReset,
		"upgrade_group":        strings.TrimSpace(plan.UpgradeGroup),
		"updated_at":           common.GetTimestamp(),
	}
	if rewardSubscriptionId > 0 {
		var existing model.UserSubscription
		query := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ? AND user_id = ? AND grant_reason = ?", rewardSubscriptionId, userId, monthlyInviteEntitlementReason).First(&existing)
		if query.Error != nil && !errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return nil, query.Error
		}
		if query.Error == nil {
			if err := tx.Model(&existing).Updates(fields).Error; err != nil {
				return nil, err
			}
			if err := tx.First(&existing, existing.Id).Error; err != nil {
				return nil, err
			}
			return &existing, nil
		}
	}
	sub := &model.UserSubscription{
		UserId:            userId,
		PlanId:            plan.Id,
		AmountTotal:       plan.TotalAmount,
		AmountUsed:        0,
		TokenLimit:        plan.MonthlyTokenLimit,
		TokenUsed:         0,
		ConcurrencyLimit:  plan.ConcurrencyLimit,
		GrantReason:       monthlyInviteEntitlementReason,
		GrantSourceUserId: userId,
		StartTime:         now,
		EndTime:           endTime,
		Status:            "active",
		Source:            monthlyInviteEntitlementReason,
		LastResetTime:     now,
		NextResetTime:     nextReset,
		UpgradeGroup:      strings.TrimSpace(plan.UpgradeGroup),
		PrevUserGroup:     "",
		CreatedAt:         common.GetTimestamp(),
		UpdatedAt:         common.GetTimestamp(),
	}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

func calcInvitationRewardNextResetTime(now int64, plan *model.SubscriptionPlan, endTime int64) int64 {
	if plan == nil || endTime <= now {
		return 0
	}
	if plan.QuotaResetPeriod == model.SubscriptionResetNever || strings.TrimSpace(plan.QuotaResetPeriod) == "" {
		return 0
	}
	resetTime := now
	switch plan.QuotaResetPeriod {
	case model.SubscriptionResetDaily:
		resetTime += 86400
	case model.SubscriptionResetWeekly:
		resetTime += 7 * 86400
	case model.SubscriptionResetMonthly:
		resetTime = time.Unix(now, 0).AddDate(0, 1, 0).Unix()
	case model.SubscriptionResetCustom:
		if plan.QuotaResetCustomSeconds <= 0 {
			return 0
		}
		resetTime += plan.QuotaResetCustomSeconds
	default:
		return 0
	}
	if resetTime > endTime {
		return endTime
	}
	return resetTime
}

func subscriptionPlanBusinessCode(plan *model.SubscriptionPlan) string {
	if plan == nil || plan.BusinessCode == nil {
		return ""
	}
	return *plan.BusinessCode
}

func rewardMonthString(at time.Time) string {
	return at.UTC().Format("2006-01")
}

func monthEndUnix(at time.Time) int64 {
	at = at.UTC()
	end := time.Date(at.Year(), at.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return end.Unix()
}

func TryEnsureInvitationEntitlementForPaidUser(userId int) {
	if userId <= 0 {
		return
	}
	var user model.User
	if err := model.DB.Select("id", "inviter_id").First(&user, userId).Error; err != nil || user.InviterId <= 0 {
		return
	}
	if _, err := EnsureMonthlyInvitationEntitlement(user.InviterId, time.Now()); err != nil {
		common.SysError("failed to ensure monthly invitation entitlement: " + err.Error())
	}
}
