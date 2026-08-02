package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDecimalAmountMicrosPreservesSixDecimalPlaces(t *testing.T) {
	got, err := ParseDecimalAmountMicros("40.123456")

	require.NoError(t, err)
	require.Equal(t, int64(40_123_456), got)
}

func TestParseDecimalAmountMicrosRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		text string
		err  error
	}{
		{name: "negative", text: "-0.000001", err: ErrSubscriptionPlanPriceNegative},
		{name: "too precise", text: "1.0000001", err: ErrSubscriptionPlanPricePrecision},
		{name: "overflow", text: "9223372036854.775808", err: ErrCreditValuationOverflow},
		{name: "empty", text: "", err: ErrSubscriptionPlanPriceInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseDecimalAmountMicros(test.text)
			require.ErrorIs(t, err, test.err)
		})
	}
}

func TestParseDecimalAmountMicrosAcceptsMaxInt64(t *testing.T) {
	got, err := ParseDecimalAmountMicros("9223372036854.775807")

	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), got)
}
