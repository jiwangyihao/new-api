package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type TrialGrantInput struct {
	UserId       int
	TrialCode    string
	InviterId    int
	Source       string
	SourceUserId int
}

func GrantTrialOnRegistration(tx *gorm.DB, input TrialGrantInput) (*model.UserSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if input.UserId <= 0 {
		return nil, errors.New("invalid user id")
	}
	trialCode := strings.TrimSpace(input.TrialCode)
	if trialCode != "" {
		consumed, err := model.ConsumeTrialCode(tx, input.UserId, trialCode)
		if err != nil {
			return nil, err
		}
		plan, err := getTrialPlanForGrant(tx, consumed.PlanId)
		if err != nil {
			return nil, err
		}
		return createTrialSubscription(tx, input.UserId, plan, "trial_code", 0)
	}
	if input.InviterId <= 0 {
		return nil, nil
	}
	hasTrial, err := model.UserHasTrialSubscriptionTx(tx, input.UserId)
	if err != nil {
		return nil, err
	}
	if hasTrial {
		return nil, errors.New("user has already received trial")
	}
	plan, err := getTrialPlanForGrant(tx, 0)
	if err != nil {
		return nil, err
	}
	return createTrialSubscription(tx, input.UserId, plan, "invite_trial", input.InviterId)
}

func getTrialPlanForGrant(tx *gorm.DB, planId int) (*model.SubscriptionPlan, error) {
	var plan model.SubscriptionPlan
	query := tx.Where("enabled = ? AND is_trial = ?", true, true)
	if planId > 0 {
		query = query.Where("id = ?", planId)
	} else {
		query = query.Order("id asc")
	}
	if err := query.First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func createTrialSubscription(tx *gorm.DB, userId int, plan *model.SubscriptionPlan, reason string, sourceUserId int) (*model.UserSubscription, error) {
	sub, err := model.CreateUserSubscriptionFromPlanTx(tx, userId, plan, reason)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"grant_reason": reason, "grant_source_user_id": sourceUserId}
	if err := tx.Model(&model.UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
		return nil, err
	}
	sub.GrantReason = reason
	sub.GrantSourceUserId = sourceUserId
	return sub, nil
}
