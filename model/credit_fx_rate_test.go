package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCreditFXRateSnapshotCanonicalizesUSDtoCNY(t *testing.T) {
	rateText := "7.300000"

	snapshot, err := ParseCreditFXRateSnapshot(CreditFXRateSnapshotInput{
		SourceCurrency:    "USD",
		ValuationCurrency: "CNY",
		RateText:          &rateText,
		CapturedAt:        1_800_000_000,
	})

	require.NoError(t, err)
	require.Equal(t, CreditFXRateSnapshot{
		SourceCurrency:    "USD",
		ValuationCurrency: "CNY",
		Numerator:         73,
		Denominator:       10,
		CapturedAt:        1_800_000_000,
		Direction:         CreditFXDirectionUSDtoCNY,
	}, snapshot)
}

func TestParseCreditFXRateSnapshotRejectsInvalidInputsWithStableErrors(t *testing.T) {
	rateText := func(value string) *string { return &value }
	tests := []struct {
		name   string
		mutate func(*CreditFXRateSnapshotInput)
		want   error
	}{
		{name: "missing rate", mutate: func(input *CreditFXRateSnapshotInput) { input.RateText = nil }, want: ErrCreditFXRateMissing},
		{name: "empty rate", mutate: func(input *CreditFXRateSnapshotInput) { input.RateText = rateText("") }, want: ErrCreditFXRateEmpty},
		{name: "invalid decimal", mutate: func(input *CreditFXRateSnapshotInput) { input.RateText = rateText("7e0") }, want: ErrCreditFXInvalidDecimal},
		{name: "precision exceeded", mutate: func(input *CreditFXRateSnapshotInput) { input.RateText = rateText("7.1234567890123456789") }, want: ErrCreditFXPrecisionExceeded},
		{name: "zero rate", mutate: func(input *CreditFXRateSnapshotInput) { input.RateText = rateText("0") }, want: ErrCreditFXNonPositive},
		{name: "negative rate", mutate: func(input *CreditFXRateSnapshotInput) { input.RateText = rateText("-7.3") }, want: ErrCreditFXNonPositive},
		{name: "unsupported currency", mutate: func(input *CreditFXRateSnapshotInput) { input.SourceCurrency = "EUR" }, want: ErrCreditFXUnsupportedCurrency},
		{name: "direction mismatch", mutate: func(input *CreditFXRateSnapshotInput) { input.Direction = "CNY_TO_USD" }, want: ErrCreditFXDirectionMismatch},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := CreditFXRateSnapshotInput{
				SourceCurrency:    "USD",
				ValuationCurrency: "CNY",
				RateText:          rateText("7.3"),
				CapturedAt:        1_800_000_000,
			}
			test.mutate(&input)

			_, err := ParseCreditFXRateSnapshot(input)

			require.ErrorIs(t, err, test.want)
		})
	}
}
