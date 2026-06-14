package creditbilling

import (
	"fmt"

	"github.com/QuantumNous/new-api/pkg/tokenbilling"
)

const (
	ModeUsageTokens  = "usage_tokens"
	ModeFixedRequest = "fixed_request"

	DynamicMultiplierDefaultSource = "default"

	ZeroReasonNotChargeable  = "not_chargeable"
	ZeroReasonNoTrustedUsage = "no_trusted_usage"
	ZeroReasonZeroUsage      = "zero_usage"
)

type CreditBillingInput struct {
	Chargeable                     bool
	HasTrustedUsage                bool
	RawMeteredTokens               int64
	CreditBillingMode              string
	FixedRequestCredits            int64
	ChannelTokenBillingMultiplier  float64
	DynamicBillingMultiplier       float64
	DynamicBillingMultiplierSource string
}

type CreditBillingResult struct {
	Chargeable                     bool
	HasTrustedUsage                bool
	CreditBillingMode              string
	BaseCredits                    int64
	APIKeyCredits                  int64
	SubscriptionCredits            int64
	DynamicBillingMultiplier       float64
	DynamicBillingMultiplierSource string
	ZeroReason                     string
}

func Calculate(input CreditBillingInput) (CreditBillingResult, error) {
	if err := ValidateBillingMode(input.CreditBillingMode); err != nil {
		return CreditBillingResult{}, err
	}
	if err := ValidateFixedRequestCredits(input.CreditBillingMode, input.FixedRequestCredits); err != nil {
		return CreditBillingResult{}, err
	}
	if input.CreditBillingMode == ModeUsageTokens {
		if err := tokenbilling.ValidateMultiplier(input.ChannelTokenBillingMultiplier); err != nil {
			return CreditBillingResult{}, fmt.Errorf("channel token billing multiplier: %w", err)
		}
	}

	dynamicMultiplier, err := effectiveDynamicMultiplier(input.DynamicBillingMultiplier)
	if err != nil {
		return CreditBillingResult{}, err
	}
	dynamicSource := input.DynamicBillingMultiplierSource
	if dynamicSource == "" {
		dynamicSource = DynamicMultiplierDefaultSource
	}

	result := CreditBillingResult{
		Chargeable:                     input.Chargeable,
		HasTrustedUsage:                input.HasTrustedUsage,
		CreditBillingMode:              input.CreditBillingMode,
		DynamicBillingMultiplier:       dynamicMultiplier,
		DynamicBillingMultiplierSource: dynamicSource,
	}

	if !input.Chargeable {
		result.ZeroReason = ZeroReasonNotChargeable
		return result, nil
	}
	if !input.HasTrustedUsage {
		result.ZeroReason = ZeroReasonNoTrustedUsage
		return result, nil
	}

	baseCredits, err := baseCredits(input)
	if err != nil {
		return CreditBillingResult{}, err
	}
	result.BaseCredits = baseCredits
	if baseCredits == 0 {
		result.ZeroReason = ZeroReasonZeroUsage
		return result, nil
	}

	finalCredits, err := ApplyCreditMultiplier(baseCredits, dynamicMultiplier)
	if err != nil {
		return CreditBillingResult{}, fmt.Errorf("dynamic billing multiplier: %w", err)
	}
	result.APIKeyCredits = finalCredits
	result.SubscriptionCredits = finalCredits
	return result, nil
}

func ApplyCreditMultiplier(units int64, multiplier float64) (int64, error) {
	return tokenbilling.ApplyMultiplier(units, multiplier)
}

func ValidateBillingMode(mode string) error {
	switch mode {
	case ModeUsageTokens, ModeFixedRequest:
		return nil
	default:
		return fmt.Errorf("credit billing mode must be %q or %q", ModeUsageTokens, ModeFixedRequest)
	}
}

func ValidateFixedRequestCredits(mode string, credits int64) error {
	if mode == ModeFixedRequest && credits <= 0 {
		return fmt.Errorf("fixed request credits must be greater than 0")
	}
	if mode == ModeUsageTokens && credits < 0 {
		return fmt.Errorf("fixed request credits must not be negative")
	}
	return nil
}

func baseCredits(input CreditBillingInput) (int64, error) {
	switch input.CreditBillingMode {
	case ModeUsageTokens:
		credits, err := ApplyCreditMultiplier(input.RawMeteredTokens, input.ChannelTokenBillingMultiplier)
		if err != nil {
			return 0, fmt.Errorf("channel token billing multiplier: %w", err)
		}
		return credits, nil
	case ModeFixedRequest:
		return input.FixedRequestCredits, nil
	default:
		return 0, fmt.Errorf("credit billing mode must be %q or %q", ModeUsageTokens, ModeFixedRequest)
	}
}

func effectiveDynamicMultiplier(multiplier float64) (float64, error) {
	if multiplier == 0 {
		return tokenbilling.DefaultMultiplier, nil
	}
	if err := tokenbilling.ValidateMultiplier(multiplier); err != nil {
		return 0, fmt.Errorf("dynamic billing multiplier: %w", err)
	}
	return multiplier, nil
}
