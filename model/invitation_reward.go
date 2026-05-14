package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	InvitationEntitlementStatusQualified    = "qualified"
	InvitationEntitlementStatusNotQualified = "not_qualified"
)

type InvitationMonthlyEntitlement struct {
	Id                   int    `json:"id"`
	InviterId            int    `json:"inviter_id" gorm:"uniqueIndex:ux_inviter_reward_month;index;not null"`
	RewardMonth          string `json:"reward_month" gorm:"uniqueIndex:ux_inviter_reward_month;type:varchar(7);not null"`
	QualifiedActiveCount int    `json:"qualified_active_count" gorm:"type:int;not null"`
	RewardPlanId         int    `json:"reward_plan_id" gorm:"not null"`
	RewardSubscriptionId int    `json:"reward_subscription_id" gorm:"index"`
	Status               string `json:"status" gorm:"type:varchar(32);index"`
	CreatedAt            int64  `json:"created_at" gorm:"type:bigint"`
	UpdatedAt            int64  `json:"updated_at" gorm:"type:bigint"`
}

func (e *InvitationMonthlyEntitlement) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	e.CreatedAt = now
	e.UpdatedAt = now
	return nil
}

func (e *InvitationMonthlyEntitlement) BeforeUpdate(_ *gorm.DB) error {
	e.UpdatedAt = common.GetTimestamp()
	return nil
}
