package controller

import "github.com/QuantumNous/new-api/model"

func validatePurchasableSubscriptionPlan(plan *model.SubscriptionPlan) string {
	if plan == nil {
		return "套餐不存在"
	}
	if !plan.Enabled {
		return "套餐未启用"
	}
	if plan.IsTrial || !plan.PublicVisible || plan.PriceAmount < 0.01 {
		return "套餐不可购买"
	}
	return ""
}
