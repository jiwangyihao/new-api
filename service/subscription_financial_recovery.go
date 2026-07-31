package service

import "github.com/QuantumNous/new-api/model"

type SubscriptionOrderRecoveryRequest = model.SubscriptionOrderRecoveryRequest
type SubscriptionOrderRecoveryResult = model.SubscriptionOrderRecoveryResult
type CreditBalanceAdjustmentRequest = model.CreditBalanceAdjustmentRequest
type CreditBalanceAdjustmentResult = model.CreditBalanceAdjustmentResult

const (
	SubscriptionOrderRecoveryRefund     = model.SubscriptionOrderRecoveryRefund
	SubscriptionOrderRecoveryChargeback = model.SubscriptionOrderRecoveryChargeback
)

func RecoverSubscriptionOrder(request SubscriptionOrderRecoveryRequest) (*SubscriptionOrderRecoveryResult, error) {
	return model.RecoverSubscriptionOrder(request)
}

func AdjustCreditBalance(request CreditBalanceAdjustmentRequest) (*CreditBalanceAdjustmentResult, error) {
	return model.AdjustCreditBalance(request)
}
