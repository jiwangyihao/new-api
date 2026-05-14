package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type TrialCode struct {
	Id             int    `json:"id"`
	Code           string `json:"code" gorm:"type:varchar(64);uniqueIndex;not null"`
	PlanId         int    `json:"plan_id" gorm:"index;not null"`
	Enabled        bool   `json:"enabled" gorm:"default:true"`
	MaxRedemptions int    `json:"max_redemptions" gorm:"type:int;not null;default:0"`
	RedeemedCount  int    `json:"redeemed_count" gorm:"type:int;not null;default:0"`
	ExpiresAt      int64  `json:"expires_at" gorm:"type:bigint;default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint"`
	UpdatedAt      int64  `json:"updated_at" gorm:"type:bigint"`
}

type TrialRedemption struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"uniqueIndex:ux_trial_user_code"`
	TrialCodeId int    `json:"trial_code_id" gorm:"uniqueIndex:ux_trial_user_code"`
	Code        string `json:"code" gorm:"type:varchar(64);index"`
	CreatedAt   int64  `json:"created_at" gorm:"type:bigint"`
}

func (c *TrialCode) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	c.Code = normalizeTrialCode(c.Code)
	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

func (c *TrialCode) BeforeUpdate(tx *gorm.DB) error {
	c.Code = normalizeTrialCode(c.Code)
	c.UpdatedAt = common.GetTimestamp()
	return nil
}

func (r *TrialRedemption) BeforeCreate(tx *gorm.DB) error {
	r.Code = normalizeTrialCode(r.Code)
	r.CreatedAt = common.GetTimestamp()
	return nil
}

func normalizeTrialCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func UserHasTrialSubscriptionTx(tx *gorm.DB, userId int) (bool, error) {
	if tx == nil {
		return false, errors.New("tx is nil")
	}
	if userId <= 0 {
		return false, errors.New("invalid user id")
	}
	var count int64
	if err := tx.Model(&UserSubscription{}).
		Where("user_id = ? AND grant_reason IN ?", userId, []string{"trial_code", "invite_trial"}).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func ConsumeTrialCode(tx *gorm.DB, userId int, rawCode string) (*TrialCode, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	code := normalizeTrialCode(rawCode)
	if code == "" {
		return nil, errors.New("trial code is required")
	}
	hasTrial, err := UserHasTrialSubscriptionTx(tx, userId)
	if err != nil {
		return nil, err
	}
	if hasTrial {
		return nil, errors.New("user has already received trial")
	}
	var trialCode TrialCode
	if err := tx.Where("code = ?", code).First(&trialCode).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid trial code")
		}
		return nil, err
	}
	if !trialCode.Enabled {
		return nil, errors.New("trial code disabled")
	}
	now := common.GetTimestamp()
	if trialCode.ExpiresAt > 0 && trialCode.ExpiresAt <= now {
		return nil, errors.New("trial code expired")
	}
	var plan SubscriptionPlan
	if err := tx.Where("id = ?", trialCode.PlanId).First(&plan).Error; err != nil {
		return nil, err
	}
	if !plan.IsTrial {
		return nil, errors.New("trial code plan is not trial")
	}
	if err := reserveTrialCodeRedemptionSlot(tx, &trialCode, now); err != nil {
		return nil, err
	}
	trialCode.RedeemedCount++
	redemption := &TrialRedemption{UserId: userId, TrialCodeId: trialCode.Id, Code: trialCode.Code}
	if err := tx.Create(redemption).Error; err != nil {
		return nil, err
	}
	return &trialCode, nil
}

func reserveTrialCodeRedemptionSlot(tx *gorm.DB, trialCode *TrialCode, now int64) error {
	if tx == nil || trialCode == nil || trialCode.Id <= 0 {
		return errors.New("invalid trial code reservation")
	}
	query := tx.Model(&TrialCode{}).Where("id = ?", trialCode.Id)
	if trialCode.MaxRedemptions > 0 {
		query = query.Where("redeemed_count < ?", trialCode.MaxRedemptions)
	}
	result := query.Updates(map[string]any{
		"redeemed_count": gorm.Expr("redeemed_count + ?", 1),
		"updated_at":     now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("trial code redemption limit reached")
	}
	return nil
}
