package controller

import "github.com/QuantumNous/new-api/model"

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

func prepareExternalSubscriptionEntitlementSnapshot(plan *model.SubscriptionPlan, purchaseMode string) (model.SubscriptionEntitlementSnapshot, error) {
	mode, err := model.NormalizeSubscriptionPurchaseMode(purchaseMode)
	if err != nil {
		return model.SubscriptionEntitlementSnapshot{}, err
	}
	targetCreditPlanID := 0
	var creditPlan *model.SubscriptionPlan
	if mode == model.SubscriptionPurchaseModeCreditBalance {
		creditPlan, err = model.GetCreditBalancePlanTx(model.DB)
		if err != nil {
			return model.SubscriptionEntitlementSnapshot{}, err
		}
		if err := model.ValidateCreditBalancePurchaseOption(plan, creditPlan); err != nil {
			return model.SubscriptionEntitlementSnapshot{}, err
		}
		targetCreditPlanID = creditPlan.Id
	}
	snapshot := model.NewSubscriptionEntitlementSnapshot(plan, mode, targetCreditPlanID)
	snapshot.MaxPurchasePerUser = 0
	if creditPlan != nil {
		snapshot.SetTargetCreditBalancePlanSnapshot(creditPlan)
	}
	return snapshot, nil
}

func marshalExternalSubscriptionEntitlementSnapshot(snapshot model.SubscriptionEntitlementSnapshot, paymentProvider string, providerProductID string, paymentMethod string, amountCents int64, currency string) (string, error) {
	snapshot.SetPaymentSnapshot(paymentProvider, providerProductID, paymentMethod, amountCents, currency)
	return model.MarshalSubscriptionEntitlementSnapshot(snapshot)
}
