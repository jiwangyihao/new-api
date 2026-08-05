package model

import (
	"math"
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

func TestParseCreditFXRateSnapshotFreezesDirectionalRatios(t *testing.T) {
	tests := []struct {
		name              string
		sourceCurrency    string
		valuationCurrency string
		direction         string
		rateProvided      bool
		wantNumerator     int64
		wantDenominator   int64
	}{
		{name: "CNY identity", sourceCurrency: "CNY", valuationCurrency: "CNY", direction: "IDENTITY", wantNumerator: 1, wantDenominator: 1},
		{name: "USD identity", sourceCurrency: "USD", valuationCurrency: "USD", direction: "IDENTITY", wantNumerator: 1, wantDenominator: 1},
		{name: "USD to CNY", sourceCurrency: "USD", valuationCurrency: "CNY", direction: "USD_TO_CNY", rateProvided: true, wantNumerator: 73, wantDenominator: 10},
		{name: "CNY to USD", sourceCurrency: "CNY", valuationCurrency: "USD", direction: "CNY_TO_USD", rateProvided: true, wantNumerator: 10, wantDenominator: 73},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			optionRate := "7.300000"
			input := CreditFXRateSnapshotInput{
				SourceCurrency:    test.sourceCurrency,
				ValuationCurrency: test.valuationCurrency,
				Direction:         test.direction,
				CapturedAt:        1_800_000_000,
			}
			if test.rateProvided {
				input.RateText = &optionRate
			}

			first, err := ParseCreditFXRateSnapshot(input)
			require.NoError(t, err)
			second, err := ParseCreditFXRateSnapshot(input)
			require.NoError(t, err)
			require.Equal(t, first, second, "same input and captured_at must be deterministic")
			require.Equal(t, CreditFXRateSnapshot{
				SourceCurrency:    test.sourceCurrency,
				ValuationCurrency: test.valuationCurrency,
				Numerator:         test.wantNumerator,
				Denominator:       test.wantDenominator,
				CapturedAt:        1_800_000_000,
				Direction:         test.direction,
			}, first)

			if test.rateProvided {
				optionRate = "8.100000"
				updated, updateErr := ParseCreditFXRateSnapshot(input)
				require.NoError(t, updateErr)
				require.NotEqual(t, first, updated)
				require.Equal(t, test.wantNumerator, first.Numerator, "frozen snapshot must not follow later Option changes")
				require.Equal(t, test.wantDenominator, first.Denominator, "frozen snapshot must not follow later Option changes")
			}
		})
	}
}

func TestCreditFXRateSnapshotConvertMicrosUsesOverflowSafeFloor(t *testing.T) {
	tests := []struct {
		name        string
		amount      int64
		numerator   int64
		denominator int64
		want        int64
		wantErr     error
	}{
		{name: "floor fractional result", amount: 10, numerator: 2, denominator: 3, want: 6},
		{name: "wide intermediate", amount: math.MaxInt64, numerator: math.MaxInt64, denominator: math.MaxInt64, want: math.MaxInt64},
		{name: "result overflow", amount: math.MaxInt64, numerator: 2, denominator: 1, wantErr: ErrCreditFXOverflow},
		{name: "zero denominator", amount: 10, numerator: 1, denominator: 0, wantErr: ErrCreditFXRateInvalid},
		{name: "remainder clears to zero", amount: 1, numerator: 1, denominator: 3, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := CreditFXRateSnapshot{
				Numerator:   test.numerator,
				Denominator: test.denominator,
			}

			got, err := snapshot.ConvertMicros(test.amount)

			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}
