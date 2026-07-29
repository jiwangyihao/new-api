package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type KyrenPaymentSnapshot struct {
	ProductID string `json:"product_id"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
}

type SubscriptionEntitlementSnapshot struct {
	PurchaseMode              string  `json:"purchase_mode"`
	PlanID                    int     `json:"plan_id"`
	PlanTitle                 string  `json:"plan_title"`
	PlanEntitlementType       string  `json:"plan_entitlement_type"`
	PriceAmount               float64 `json:"price_amount"`
	Currency                  string  `json:"currency"`
	TotalAmount               int64   `json:"total_amount"`
	MonthlyTokenLimit         int64   `json:"monthly_token_limit"`
	ConcurrencyLimit          int     `json:"concurrency_limit"`
	QueueCapacity             int     `json:"queue_capacity"`
	DurationUnit              string  `json:"duration_unit"`
	DurationValue             int     `json:"duration_value"`
	CustomSeconds             int64   `json:"custom_seconds"`
	QuotaResetPeriod          string  `json:"quota_reset_period"`
	QuotaResetCustomSeconds   int64   `json:"quota_reset_custom_seconds"`
	MaxPurchasePerUser        int     `json:"max_purchase_per_user"`
	BusinessCode              string  `json:"business_code"`
	IsTrial                   bool    `json:"is_trial"`
	InviteTrial               bool    `json:"invite_trial"`
	RewardEligible            bool    `json:"reward_eligible"`
	TargetCreditBalancePlanID int     `json:"target_credit_balance_plan_id,omitempty"`
}

func NewSubscriptionEntitlementSnapshotFromPlan(plan *SubscriptionPlan) SubscriptionEntitlementSnapshot {
	return NewSubscriptionEntitlementSnapshot(plan, SubscriptionEntitlementTimed, 0)
}

func NewSubscriptionEntitlementSnapshot(plan *SubscriptionPlan, purchaseMode string, targetCreditBalancePlanID int) SubscriptionEntitlementSnapshot {
	if plan == nil {
		return SubscriptionEntitlementSnapshot{}
	}
	businessCode := ""
	if plan.BusinessCode != nil {
		businessCode = *plan.BusinessCode
	}
	planEntitlementType := strings.TrimSpace(plan.EntitlementType)
	if planEntitlementType == "" {
		planEntitlementType = SubscriptionEntitlementTimed
	}
	return SubscriptionEntitlementSnapshot{
		PurchaseMode:              strings.TrimSpace(purchaseMode),
		PlanID:                    plan.Id,
		PlanTitle:                 plan.Title,
		PlanEntitlementType:       planEntitlementType,
		PriceAmount:               plan.PriceAmount,
		Currency:                  strings.ToUpper(strings.TrimSpace(plan.Currency)),
		TotalAmount:               plan.TotalAmount,
		MonthlyTokenLimit:         plan.MonthlyTokenLimit,
		ConcurrencyLimit:          plan.ConcurrencyLimit,
		QueueCapacity:             plan.QueueCapacity,
		DurationUnit:              plan.DurationUnit,
		DurationValue:             plan.DurationValue,
		CustomSeconds:             plan.CustomSeconds,
		QuotaResetPeriod:          plan.QuotaResetPeriod,
		QuotaResetCustomSeconds:   plan.QuotaResetCustomSeconds,
		MaxPurchasePerUser:        plan.MaxPurchasePerUser,
		BusinessCode:              businessCode,
		IsTrial:                   plan.IsTrial,
		InviteTrial:               plan.InviteTrial,
		RewardEligible:            plan.RewardEligible,
		TargetCreditBalancePlanID: targetCreditBalancePlanID,
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
