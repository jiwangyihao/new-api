package billingexpr_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

func TestComputeTieredQuotaIgnoresLegacyQuotaMultiplier(t *testing.T) {
	snap := &billingexpr.BillingSnapshot{
		BillingMode:               "tiered_expr",
		ExprString:                `tier("base", p * 2 + c * 4)`,
		ExprHash:                  billingexpr.ExprHashString(`tier("base", p * 2 + c * 4)`),
		QuotaMultiplier:           9,
		EstimatedPromptTokens:     100,
		EstimatedCompletionTokens: 10,
		EstimatedQuotaBeforeRatio: (100*2 + 10*4) / 1_000_000.0 * 500_000,
		EstimatedQuota:            billingexpr.QuotaRound((100*2 + 10*4) / 1_000_000.0 * 500_000),
		EstimatedTier:             "base",
		QuotaPerUnit:              500_000,
	}

	result, err := billingexpr.ComputeTieredQuota(snap, billingexpr.TokenParams{P: 200, C: 20})
	if err != nil {
		t.Fatal(err)
	}

	want := billingexpr.QuotaRound((200*2 + 20*4) / 1_000_000.0 * 500_000)
	if result.ActualQuota != want {
		t.Fatalf("quota should ignore legacy multiplier: got %d, want %d", result.ActualQuota, want)
	}
}
