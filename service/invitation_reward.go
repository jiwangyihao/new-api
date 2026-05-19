package service

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const (
	monthlyInviteEntitlementReason     = "monthly_invite_entitlement"
	defaultMonthlyInviteRewardPlanCode = "basic_monthly"
	monthlyInviteRewardPlanOptionKey   = "MonthlyInvitationRewardPlanCode"
	monthlyInviteRewardPlanEnvKey      = "MONTHLY_INVITATION_REWARD_PLAN_CODE"
	monthlyInviteQualifiedCount        = 2
)

type InvitationEntitlementStatus struct {
	DirectInviteCount    int    `json:"direct_invite_count"`
	QualifiedActiveCount int    `json:"qualified_active_count"`
	RewardMonth          string `json:"reward_month"`
	Entitled             bool   `json:"entitled"`
	EntitlementEndTime   int64  `json:"entitlement_end_time"`
	RewardSubscriptionId int    `json:"reward_subscription_id"`
}

type qualifiedInviteeEndTime struct {
	InviteeId int
	EndTime   int64
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
	endTime := int64(0)
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
		var entitlement model.InvitationMonthlyEntitlement
		entitlementErr := tx.Set("gorm:query_option", "FOR UPDATE").Where("inviter_id = ? AND reward_month = ?", inviterId, rewardMonth).First(&entitlement).Error
		if entitlementErr != nil && !errors.Is(entitlementErr, gorm.ErrRecordNotFound) {
			return entitlementErr
		}
		entitlementMissing := errors.Is(entitlementErr, gorm.ErrRecordNotFound)
		qualified := qualifiedCount >= monthlyInviteQualifiedCount
		if !qualified {
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
		qualifiedEndTimes, err := listQualifiedActiveInviteeEndTimesTx(tx, inviterId, at.Unix())
		if err != nil {
			return err
		}
		if len(qualifiedEndTimes) < monthlyInviteQualifiedCount {
			return errors.New("qualified invitee end times inconsistent with qualified count")
		}
		endTime = qualifiedEndTimes[monthlyInviteQualifiedCount-1].EndTime
		plan, err := findMonthlyInvitationRewardPlanTx(tx)
		if err != nil {
			return err
		}
		status.Entitled = true
		status.EntitlementEndTime = endTime
		if entitlementMissing {
			entitlement = model.InvitationMonthlyEntitlement{InviterId: inviterId, RewardMonth: rewardMonth}
		}
		if entitlement.RewardSubscriptionId == 0 {
			sub, err := model.CreateUserSubscriptionFromPlanTx(tx, inviterId, plan, monthlyInviteEntitlementReason)
			if err != nil {
				return err
			}
			sub.EndTime = endTime
			sub.GrantSourceUserId = inviterId
			if err := tx.Save(sub).Error; err != nil {
				return err
			}
			entitlement.RewardSubscriptionId = sub.Id
		} else {
			if err := tx.Model(&model.UserSubscription{}).Where("id = ?", entitlement.RewardSubscriptionId).Updates(map[string]interface{}{
				"status":               "active",
				"end_time":             endTime,
				"grant_source_user_id": inviterId,
			}).Error; err != nil {
				return err
			}
		}
		entitlement.QualifiedActiveCount = qualifiedCount
		entitlement.RewardPlanId = plan.Id
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
	if inviterId <= 0 {
		return nil, errors.New("invalid inviter id")
	}
	if at.IsZero() {
		at = time.Now()
	}
	at = at.UTC()
	directCount, err := countDirectInviteesTx(model.DB, inviterId)
	if err != nil {
		return nil, err
	}
	qualifiedCount, err := countQualifiedActiveInviteesTx(model.DB, inviterId, at.Unix())
	if err != nil {
		return nil, err
	}
	status := &InvitationEntitlementStatus{DirectInviteCount: directCount, QualifiedActiveCount: qualifiedCount, RewardMonth: rewardMonthString(at)}
	var entitlement model.InvitationMonthlyEntitlement
	if err := model.DB.Where("inviter_id = ? AND reward_month = ?", inviterId, status.RewardMonth).First(&entitlement).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return status, nil
		}
		return nil, err
	}
	if entitlement.Status == model.InvitationEntitlementStatusQualified && entitlement.RewardSubscriptionId > 0 {
		var sub model.UserSubscription
		if err := model.DB.Select("id", "end_time").First(&sub, entitlement.RewardSubscriptionId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return status, nil
			}
			return nil, err
		}
		status.Entitled = true
		status.EntitlementEndTime = sub.EndTime
		status.RewardSubscriptionId = entitlement.RewardSubscriptionId
	}
	return status, nil
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
		Joins("JOIN subscription_orders ON subscription_orders.user_id = users.id AND subscription_orders.plan_id = user_subscriptions.plan_id").
		Where("users.inviter_id = ?", inviterId).
		Where("user_subscriptions.status = ?", "active").
		Where("user_subscriptions.start_time <= ? AND user_subscriptions.end_time > ?", now, now).
		Where("(user_subscriptions.grant_reason = ? OR (user_subscriptions.grant_reason = ? AND user_subscriptions.source = ?))", "order", "", "order").
		Where("subscription_plans.reward_eligible = ?", true).
		Where("subscription_orders.status = ?", common.TopUpStatusSuccess).
		Where("subscription_orders.money > ?", 0).
		Distinct("users.id").
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func listQualifiedActiveInviteeEndTimesTx(tx *gorm.DB, inviterId int, now int64) ([]qualifiedInviteeEndTime, error) {
	var endTimes []qualifiedInviteeEndTime
	err := tx.Table("users").
		Select("users.id AS invitee_id, MAX(user_subscriptions.end_time) AS end_time").
		Joins("JOIN user_subscriptions ON user_subscriptions.user_id = users.id").
		Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Joins("JOIN subscription_orders ON subscription_orders.user_id = users.id AND subscription_orders.plan_id = user_subscriptions.plan_id").
		Where("users.inviter_id = ?", inviterId).
		Where("user_subscriptions.status = ?", "active").
		Where("user_subscriptions.start_time <= ? AND user_subscriptions.end_time > ?", now, now).
		Where("(user_subscriptions.grant_reason = ? OR (user_subscriptions.grant_reason = ? AND user_subscriptions.source = ?))", "order", "", "order").
		Where("subscription_plans.reward_eligible = ?", true).
		Where("subscription_orders.status = ?", common.TopUpStatusSuccess).
		Where("subscription_orders.money > ?", 0).
		Group("users.id").
		Order("end_time DESC").
		Limit(monthlyInviteQualifiedCount).
		Scan(&endTimes).Error
	if err != nil {
		return nil, err
	}
	return endTimes, nil
}

func monthlyInvitationRewardPlanCode() string {
	common.OptionMapRWMutex.RLock()
	code := common.OptionMap[monthlyInviteRewardPlanOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if code != "" {
		return code
	}
	return common.GetEnvOrDefaultString(monthlyInviteRewardPlanEnvKey, defaultMonthlyInviteRewardPlanCode)
}

func findMonthlyInvitationRewardPlanTx(tx *gorm.DB) (*model.SubscriptionPlan, error) {
	code := monthlyInvitationRewardPlanCode()
	var plan model.SubscriptionPlan
	if err := tx.Where("business_code = ? AND enabled = ?", code, true).First(&plan).Error; err != nil {
		return nil, errors.New("monthly invitation reward plan not found or disabled: " + code)
	}
	return &plan, nil
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
