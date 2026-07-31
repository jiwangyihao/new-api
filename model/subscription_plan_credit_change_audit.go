package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// SubscriptionPlanCreditChangeAudit is the immutable primary-database record
// proving that an administrator accepted the renewal-merging risk before
// changing monthly Credit for a plan with active timed entitlements.
type SubscriptionPlanCreditChangeAudit struct {
	Id                            int    `json:"id"`
	PlanId                        int    `json:"plan_id" gorm:"not null;index"`
	PlanTitle                     string `json:"plan_title" gorm:"type:varchar(128);not null"`
	AdminUserId                   int    `json:"admin_user_id" gorm:"not null;index"`
	AdminUsername                 string `json:"admin_username" gorm:"type:varchar(64);not null;default:''"`
	OldMonthlyCredit              int64  `json:"old_monthly_credit" gorm:"type:bigint;not null"`
	NewMonthlyCredit              int64  `json:"new_monthly_credit" gorm:"type:bigint;not null"`
	ExistingTimedEntitlementCount int64  `json:"existing_timed_entitlement_count" gorm:"type:bigint;not null"`
	RiskConfirmed                 bool   `json:"risk_confirmed" gorm:"not null"`
	RiskReason                    string `json:"risk_reason" gorm:"type:varchar(255);not null"`
	CreatedAt                     int64  `json:"created_at" gorm:"type:bigint;not null;index"`
}

func (a *SubscriptionPlanCreditChangeAudit) BeforeUpdate(_ *gorm.DB) error {
	return errors.New("subscription plan Credit change audit is immutable")
}

func (a *SubscriptionPlanCreditChangeAudit) BeforeDelete(_ *gorm.DB) error {
	return errors.New("subscription plan Credit change audit is immutable")
}

func CreateSubscriptionPlanCreditChangeAuditTx(tx *gorm.DB, audit *SubscriptionPlanCreditChangeAudit) error {
	if tx == nil || audit == nil {
		return errors.New("invalid subscription plan Credit change audit")
	}
	audit.PlanTitle = strings.TrimSpace(audit.PlanTitle)
	audit.AdminUsername = strings.TrimSpace(audit.AdminUsername)
	audit.RiskReason = strings.TrimSpace(audit.RiskReason)
	if audit.PlanId <= 0 || audit.AdminUserId <= 0 || audit.OldMonthlyCredit == audit.NewMonthlyCredit || audit.ExistingTimedEntitlementCount <= 0 || !audit.RiskConfirmed || audit.RiskReason == "" || len(audit.RiskReason) > 255 {
		return errors.New("invalid subscription plan Credit change audit")
	}
	now, err := getDBTimestampStrictTx(tx)
	if err != nil {
		return err
	}
	audit.CreatedAt = now
	return tx.Create(audit).Error
}
