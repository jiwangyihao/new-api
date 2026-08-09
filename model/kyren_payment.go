package model

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type KyrenPaymentSnapshot struct {
	ProductID string `json:"product_id"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
}

type KyrenTopUpSnapshot struct {
	LocalTopUpID string `json:"local_topup_id"`
	ProductID    string `json:"product_id"`
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	Quota        int64  `json:"quota"`
}

type SubscriptionEntitlementSnapshot struct {
	PurchaseMode                            string  `json:"purchase_mode"`
	PlanID                                  int     `json:"plan_id"`
	PlanTitle                               string  `json:"plan_title"`
	PlanEntitlementType                     string  `json:"plan_entitlement_type"`
	PriceAmount                             float64 `json:"price_amount"`
	Currency                                string  `json:"currency"`
	PaymentAmountCents                      int64   `json:"payment_amount_cents"`
	PaymentCurrency                         string  `json:"payment_currency"`
	PaymentProvider                         string  `json:"payment_provider"`
	ProviderProductID                       string  `json:"provider_product_id,omitempty"`
	ProviderPaymentMethod                   string  `json:"provider_payment_method,omitempty"`
	TotalAmount                             int64   `json:"total_amount"`
	MonthlyTokenLimit                       int64   `json:"monthly_token_limit"`
	ConcurrencyLimit                        int     `json:"concurrency_limit"`
	QueueCapacity                           int     `json:"queue_capacity"`
	ModelLimits                             string  `json:"model_limits"`
	GPTAbuseWarningLimit                    int     `json:"gpt_abuse_warning_limit"`
	DurationUnit                            string  `json:"duration_unit"`
	DurationValue                           int     `json:"duration_value"`
	CustomSeconds                           int64   `json:"custom_seconds"`
	QuotaResetPeriod                        string  `json:"quota_reset_period"`
	QuotaResetCustomSeconds                 int64   `json:"quota_reset_custom_seconds"`
	MaxPurchasePerUser                      int     `json:"max_purchase_per_user"`
	BusinessCode                            string  `json:"business_code"`
	IsTrial                                 bool    `json:"is_trial"`
	InviteTrial                             bool    `json:"invite_trial"`
	RewardEligible                          bool    `json:"reward_eligible"`
	TargetCreditBalancePlanID               int     `json:"target_credit_balance_plan_id,omitempty"`
	TargetCreditBalanceTitle                string  `json:"target_credit_balance_title,omitempty"`
	TargetCreditBalanceBusinessCode         string  `json:"target_credit_balance_business_code,omitempty"`
	TargetCreditBalanceModelLimits          string  `json:"target_credit_balance_model_limits,omitempty"`
	TargetCreditBalanceConcurrencyLimit     int     `json:"target_credit_balance_concurrency_limit,omitempty"`
	TargetCreditBalanceQueueCapacity        int     `json:"target_credit_balance_queue_capacity,omitempty"`
	TargetCreditBalanceGPTAbuseWarningLimit int     `json:"target_credit_balance_gpt_abuse_warning_limit,omitempty"`
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
		ModelLimits:               plan.ModelLimits,
		GPTAbuseWarningLimit:      plan.GPTAbuseWarningLimit,
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

func (s *SubscriptionEntitlementSnapshot) SetPaymentSnapshot(provider string, productID string, paymentMethod string, amountCents int64, currency string) {
	if s == nil {
		return
	}
	s.PaymentProvider = strings.TrimSpace(provider)
	s.ProviderProductID = strings.TrimSpace(productID)
	s.ProviderPaymentMethod = strings.TrimSpace(paymentMethod)
	s.PaymentAmountCents = amountCents
	s.PaymentCurrency = strings.ToUpper(strings.TrimSpace(currency))
}

func (s *SubscriptionEntitlementSnapshot) SetTargetCreditBalancePlanSnapshot(plan *SubscriptionPlan) {
	if s == nil || plan == nil {
		return
	}
	businessCode := ""
	if plan.BusinessCode != nil {
		businessCode = strings.TrimSpace(*plan.BusinessCode)
	}
	s.TargetCreditBalancePlanID = plan.Id
	s.TargetCreditBalanceTitle = plan.Title
	s.TargetCreditBalanceBusinessCode = businessCode
	s.TargetCreditBalanceModelLimits = plan.ModelLimits
	s.TargetCreditBalanceConcurrencyLimit = plan.ConcurrencyLimit
	s.TargetCreditBalanceQueueCapacity = plan.QueueCapacity
	s.TargetCreditBalanceGPTAbuseWarningLimit = plan.GPTAbuseWarningLimit
}

func (s SubscriptionEntitlementSnapshot) CreditGrantIdentity() (int64, int) {
	if strings.TrimSpace(s.PurchaseMode) != SubscriptionPurchaseModeCreditBalance {
		return 0, 0
	}
	return s.MonthlyTokenLimit, s.TargetCreditBalancePlanID
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

func MarshalKyrenTopUpSnapshot(snapshot KyrenTopUpSnapshot) (string, error) {
	payload, err := common.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func UnmarshalKyrenTopUpSnapshot(payload string) (KyrenTopUpSnapshot, error) {
	var snapshot KyrenTopUpSnapshot
	if err := common.UnmarshalJsonStr(payload, &snapshot); err != nil {
		return KyrenTopUpSnapshot{}, err
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

func newKyrenProcessingToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
