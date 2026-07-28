package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func validatePurchasableSubscriptionPlan(plan *model.SubscriptionPlan) string {
	if plan == nil {
		return "套餐不存在"
	}
	if plan.EntitlementType == model.SubscriptionEntitlementCreditBalance {
		return "Credit 余额套餐不可作为普通计时商品购买"
	}
	if !plan.Enabled {
		return "套餐未启用"
	}
	if plan.IsTrial || !plan.PublicVisible || plan.PriceAmount < 0.01 {
		return "套餐不可购买"
	}
	return ""
}

func validateSubscriptionPurchaseLimit(userId int, plan *model.SubscriptionPlan) error {
	if plan == nil || plan.MaxPurchasePerUser <= 0 {
		return nil
	}

	count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
	if err != nil {
		return err
	}
	if count >= int64(plan.MaxPurchasePerUser) {
		return errors.New("已达到该套餐购买上限")
	}
	return nil
}

func validateSubscriptionPurchaseLimitTx(tx *gorm.DB, userId int, plan *model.SubscriptionPlan) error {
	if tx == nil || plan == nil || plan.MaxPurchasePerUser <= 0 {
		return nil
	}

	var count int64
	if err := tx.Model(&model.UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", userId, plan.Id).
		Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(plan.MaxPurchasePerUser) {
		return errors.New("已达到该套餐购买上限")
	}
	return nil
}
