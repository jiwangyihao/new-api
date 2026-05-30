package model

import "github.com/QuantumNous/new-api/common"

type KyrenPaymentSnapshot struct {
	ProductID string `json:"product_id"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
}

type SubscriptionEntitlementSnapshot struct {
	PlanID                  int    `json:"plan_id"`
	TotalAmount             int64  `json:"total_amount"`
	MonthlyTokenLimit       int64  `json:"monthly_token_limit"`
	ConcurrencyLimit        int    `json:"concurrency_limit"`
	QueueCapacity           int    `json:"queue_capacity"`
	DurationUnit            string `json:"duration_unit"`
	DurationValue           int    `json:"duration_value"`
	CustomSeconds           int64  `json:"custom_seconds"`
	QuotaResetPeriod        string `json:"quota_reset_period"`
	QuotaResetCustomSeconds int64  `json:"quota_reset_custom_seconds"`
	MaxPurchasePerUser      int    `json:"max_purchase_per_user"`
	BusinessCode            string `json:"business_code"`
}

func NewSubscriptionEntitlementSnapshotFromPlan(plan *SubscriptionPlan) SubscriptionEntitlementSnapshot {
	if plan == nil {
		return SubscriptionEntitlementSnapshot{}
	}
	businessCode := ""
	if plan.BusinessCode != nil {
		businessCode = *plan.BusinessCode
	}
	return SubscriptionEntitlementSnapshot{
		PlanID:                  plan.Id,
		TotalAmount:             plan.TotalAmount,
		MonthlyTokenLimit:       plan.MonthlyTokenLimit,
		ConcurrencyLimit:        plan.ConcurrencyLimit,
		QueueCapacity:           plan.QueueCapacity,
		DurationUnit:            plan.DurationUnit,
		DurationValue:           plan.DurationValue,
		CustomSeconds:           plan.CustomSeconds,
		QuotaResetPeriod:        plan.QuotaResetPeriod,
		QuotaResetCustomSeconds: plan.QuotaResetCustomSeconds,
		MaxPurchasePerUser:      plan.MaxPurchasePerUser,
		BusinessCode:            businessCode,
	}
}

func MarshalKyrenPaymentSnapshot(snapshot KyrenPaymentSnapshot) (string, error) {
	payload, err := common.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func UnmarshalKyrenPaymentSnapshot(payload string) (KyrenPaymentSnapshot, error) {
	var snapshot KyrenPaymentSnapshot
	if err := common.UnmarshalJsonStr(payload, &snapshot); err != nil {
		return KyrenPaymentSnapshot{}, err
	}
	return snapshot, nil
}

func MarshalSubscriptionEntitlementSnapshot(snapshot SubscriptionEntitlementSnapshot) (string, error) {
	payload, err := common.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func UnmarshalSubscriptionEntitlementSnapshot(payload string) (SubscriptionEntitlementSnapshot, error) {
	var snapshot SubscriptionEntitlementSnapshot
	if err := common.UnmarshalJsonStr(payload, &snapshot); err != nil {
		return SubscriptionEntitlementSnapshot{}, err
	}
	return snapshot, nil
}
