package common

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestQuotaFromFloatSaturatesUnsafeInputs(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  int
	}{
		{name: "huge positive clamps to MaxInt32", input: math.MaxFloat64, want: math.MaxInt32},
		{name: "huge negative clamps to MinInt32", input: -math.MaxFloat64, want: math.MinInt32},
		{name: "NaN falls back to zero", input: math.NaN(), want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, QuotaFromFloat(tt.input))

			got, clamp := QuotaFromFloatChecked(tt.input)
			require.Equal(t, tt.want, got)
			require.NotNil(t, clamp, "unsafe quota conversion must be auditable")
		})
	}
}

func TestQuotaRoundSaturatesUnsafeInputs(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  int
	}{
		{name: "huge positive clamps to MaxInt32", input: math.MaxFloat64, want: math.MaxInt32},
		{name: "huge negative clamps to MinInt32", input: -math.MaxFloat64, want: math.MinInt32},
		{name: "NaN falls back to zero", input: math.NaN(), want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, QuotaRound(tt.input))

			got, clamp := QuotaRoundChecked(tt.input)
			require.Equal(t, tt.want, got)
			require.NotNil(t, clamp, "unsafe quota rounding must be auditable")
		})
	}
}

func TestQuotaFromDecimalSaturatesUnsafeInputs(t *testing.T) {
	hugePositive := decimal.NewFromInt(math.MaxInt32).Add(decimal.NewFromInt(1))
	hugeNegative := decimal.NewFromInt(math.MinInt32).Sub(decimal.NewFromInt(1))

	tests := []struct {
		name  string
		input decimal.Decimal
		want  int
	}{
		{name: "huge positive clamps to MaxInt32", input: hugePositive, want: math.MaxInt32},
		{name: "huge negative clamps to MinInt32", input: hugeNegative, want: math.MinInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, QuotaFromDecimal(tt.input))

			got, clamp := QuotaFromDecimalChecked(tt.input)
			require.Equal(t, tt.want, got)
			require.NotNil(t, clamp, "decimal quota overflow must be auditable")
		})
	}
}

func TestQuotaCheckedLeavesSafeValuesUnclamped(t *testing.T) {
	gotFloat, floatClamp := QuotaFromFloatChecked(123.4)
	require.Equal(t, 123, gotFloat)
	require.Nil(t, floatClamp)

	gotRound, roundClamp := QuotaRoundChecked(123.5)
	require.Equal(t, 124, gotRound)
	require.Nil(t, roundClamp)

	gotDecimal, decimalClamp := QuotaFromDecimalChecked(decimal.NewFromInt(123))
	require.Equal(t, 123, gotDecimal)
	require.Nil(t, decimalClamp)
}
