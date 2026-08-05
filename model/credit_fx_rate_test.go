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
