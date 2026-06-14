package creditbilling

import (
	"math"
	"strings"
	"testing"
)

func TestCalculateUsageTokensModeUsesChannelMultiplierForBaseCredits(t *testing.T) {
	got, err := Calculate(CreditBillingInput{
		Chargeable:                    true,
		HasTrustedUsage:               true,
		RawMeteredTokens:              3,
		CreditBillingMode:             ModeUsageTokens,
		ChannelTokenBillingMultiplier: 1.5,
		DynamicBillingMultiplier:      1,
	})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}

	assertCredits(t, got, 5, 5, 5)
	if got.CreditBillingMode != ModeUsageTokens {
		t.Fatalf("CreditBillingMode = %q, want %q", got.CreditBillingMode, ModeUsageTokens)
	}
	if got.ZeroReason != "" {
		t.Fatalf("ZeroReason = %q, want empty", got.ZeroReason)
	}
}

func TestCalculateFixedRequestModeUsesFixedCreditsForBaseCredits(t *testing.T) {
	got, err := Calculate(CreditBillingInput{
		Chargeable:                    true,
		HasTrustedUsage:               true,
		RawMeteredTokens:              10000,
		CreditBillingMode:             ModeFixedRequest,
		FixedRequestCredits:           80000,
		ChannelTokenBillingMultiplier: 99,
		DynamicBillingMultiplier:      1,
	})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}

	assertCredits(t, got, 80000, 80000, 80000)
}

func TestCalculateAppliesDynamicMultiplierOnlyToFinalCredits(t *testing.T) {
	got, err := Calculate(CreditBillingInput{
		Chargeable:                    true,
		HasTrustedUsage:               true,
		RawMeteredTokens:              10,
		CreditBillingMode:             ModeUsageTokens,
		ChannelTokenBillingMultiplier: 2,
		DynamicBillingMultiplier:      1.5,
		DynamicBillingMultiplierSource: "upstream_header",
	})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}

	assertCredits(t, got, 20, 30, 30)
	if got.DynamicBillingMultiplier != 1.5 {
		t.Fatalf("DynamicBillingMultiplier = %v, want 1.5", got.DynamicBillingMultiplier)
	}
	if got.DynamicBillingMultiplierSource != "upstream_header" {
		t.Fatalf("DynamicBillingMultiplierSource = %q, want upstream_header", got.DynamicBillingMultiplierSource)
	}
}

func TestCalculateDefaultsDynamicMultiplierAndSource(t *testing.T) {
	got, err := Calculate(CreditBillingInput{
		Chargeable:                    true,
		HasTrustedUsage:               true,
		RawMeteredTokens:              10,
		CreditBillingMode:             ModeUsageTokens,
		ChannelTokenBillingMultiplier: 2,
	})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}

	assertCredits(t, got, 20, 20, 20)
	if got.DynamicBillingMultiplier != 1 {
		t.Fatalf("DynamicBillingMultiplier = %v, want 1", got.DynamicBillingMultiplier)
	}
	if got.DynamicBillingMultiplierSource != DynamicMultiplierDefaultSource {
		t.Fatalf("DynamicBillingMultiplierSource = %q, want %q", got.DynamicBillingMultiplierSource, DynamicMultiplierDefaultSource)
	}
}

func TestCalculateNonChargeableReturnsStandardZeroResult(t *testing.T) {
	got, err := Calculate(CreditBillingInput{
		Chargeable:                    false,
		HasTrustedUsage:               true,
		RawMeteredTokens:              100,
		CreditBillingMode:             ModeUsageTokens,
		ChannelTokenBillingMultiplier: 2,
		DynamicBillingMultiplier:      2,
	})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}

	assertCredits(t, got, 0, 0, 0)
	if got.ZeroReason != ZeroReasonNotChargeable {
		t.Fatalf("ZeroReason = %q, want %q", got.ZeroReason, ZeroReasonNotChargeable)
	}
}

func TestCalculateWithoutTrustedUsageReturnsStandardZeroResult(t *testing.T) {
	got, err := Calculate(CreditBillingInput{
		Chargeable:                    true,
		HasTrustedUsage:               false,
		RawMeteredTokens:              100,
		CreditBillingMode:             ModeFixedRequest,
		FixedRequestCredits:           80000,
		ChannelTokenBillingMultiplier: 2,
		DynamicBillingMultiplier:      2,
	})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}

	assertCredits(t, got, 0, 0, 0)
	if got.ZeroReason != ZeroReasonNoTrustedUsage {
		t.Fatalf("ZeroReason = %q, want %q", got.ZeroReason, ZeroReasonNoTrustedUsage)
	}
}

func TestCalculateTrustedZeroUsageDiffersByMode(t *testing.T) {
	t.Run("usage tokens mode charges zero credits", func(t *testing.T) {
		got, err := Calculate(CreditBillingInput{
			Chargeable:                    true,
			HasTrustedUsage:               true,
			RawMeteredTokens:              0,
			CreditBillingMode:             ModeUsageTokens,
			ChannelTokenBillingMultiplier: 2,
			DynamicBillingMultiplier:      2,
		})
		if err != nil {
			t.Fatalf("Calculate() error = %v", err)
		}

		assertCredits(t, got, 0, 0, 0)
		if got.ZeroReason != ZeroReasonZeroUsage {
			t.Fatalf("ZeroReason = %q, want %q", got.ZeroReason, ZeroReasonZeroUsage)
		}
	})

	t.Run("fixed request mode charges fixed credits", func(t *testing.T) {
		got, err := Calculate(CreditBillingInput{
			Chargeable:                    true,
			HasTrustedUsage:               true,
			RawMeteredTokens:              0,
			CreditBillingMode:             ModeFixedRequest,
			FixedRequestCredits:           80000,
			ChannelTokenBillingMultiplier: 2,
			DynamicBillingMultiplier:      2,
		})
		if err != nil {
			t.Fatalf("Calculate() error = %v", err)
		}

		assertCredits(t, got, 80000, 160000, 160000)
		if got.ZeroReason != "" {
			t.Fatalf("ZeroReason = %q, want empty", got.ZeroReason)
		}
	})
}

func TestApplyCreditMultiplierRoundingAndMinimum(t *testing.T) {
	tests := []struct {
		name       string
		units      int64
		multiplier float64
		want       int64
	}{
		{name: "zero stays zero", units: 0, multiplier: 2, want: 0},
		{name: "negative units are zero", units: -10, multiplier: 2, want: 0},
		{name: "half up rounds fractional product", units: 3, multiplier: 0.5, want: 2},
		{name: "positive input with sub one product charges one", units: 1, multiplier: 0.1, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplyCreditMultiplier(tt.units, tt.multiplier)
			if err != nil {
				t.Fatalf("ApplyCreditMultiplier() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ApplyCreditMultiplier() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCalculateRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		in   CreditBillingInput
		want string
	}{
		{
			name: "unknown billing mode",
			in: CreditBillingInput{
				Chargeable:                    true,
				HasTrustedUsage:               true,
				CreditBillingMode:             "per_second",
				ChannelTokenBillingMultiplier: 1,
				DynamicBillingMultiplier:      1,
			},
			want: "credit billing mode",
		},
		{
			name: "fixed request mode requires positive credits",
			in: CreditBillingInput{
				Chargeable:                    true,
				HasTrustedUsage:               true,
				CreditBillingMode:             ModeFixedRequest,
				FixedRequestCredits:           0,
				ChannelTokenBillingMultiplier: 1,
				DynamicBillingMultiplier:      1,
			},
			want: "fixed request credits",
		},
		{
			name: "usage token mode rejects invalid channel multiplier",
			in: CreditBillingInput{
				Chargeable:                    true,
				HasTrustedUsage:               true,
				RawMeteredTokens:              1,
				CreditBillingMode:             ModeUsageTokens,
				ChannelTokenBillingMultiplier: 0,
				DynamicBillingMultiplier:      1,
			},
			want: "multiplier",
		},
		{
			name: "dynamic multiplier rejects nan",
			in: CreditBillingInput{
				Chargeable:                    true,
				HasTrustedUsage:               true,
				RawMeteredTokens:              1,
				CreditBillingMode:             ModeUsageTokens,
				ChannelTokenBillingMultiplier: 1,
				DynamicBillingMultiplier:      math.NaN(),
			},
			want: "dynamic billing multiplier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Calculate(tt.in)
			if err == nil {
				t.Fatal("Calculate() expected error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("Calculate() error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidationHelpers(t *testing.T) {
	if err := ValidateBillingMode(ModeUsageTokens); err != nil {
		t.Fatalf("ValidateBillingMode(usage_tokens) error = %v", err)
	}
	if err := ValidateBillingMode(ModeFixedRequest); err != nil {
		t.Fatalf("ValidateBillingMode(fixed_request) error = %v", err)
	}
	if err := ValidateBillingMode(""); err == nil {
		t.Fatal("ValidateBillingMode(empty) expected error")
	}

	if err := ValidateFixedRequestCredits(ModeUsageTokens, 0); err != nil {
		t.Fatalf("ValidateFixedRequestCredits(usage_tokens, 0) error = %v", err)
	}
	if err := ValidateFixedRequestCredits(ModeFixedRequest, 1); err != nil {
		t.Fatalf("ValidateFixedRequestCredits(fixed_request, 1) error = %v", err)
	}
	if err := ValidateFixedRequestCredits(ModeFixedRequest, 0); err == nil {
		t.Fatal("ValidateFixedRequestCredits(fixed_request, 0) expected error")
	}
}

func assertCredits(t *testing.T, got CreditBillingResult, base, apiKey, subscription int64) {
	t.Helper()
	if got.BaseCredits != base {
		t.Fatalf("BaseCredits = %d, want %d", got.BaseCredits, base)
	}
	if got.APIKeyCredits != apiKey {
		t.Fatalf("APIKeyCredits = %d, want %d", got.APIKeyCredits, apiKey)
	}
	if got.SubscriptionCredits != subscription {
		t.Fatalf("SubscriptionCredits = %d, want %d", got.SubscriptionCredits, subscription)
	}
}
