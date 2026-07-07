package types

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPriceDataAddOtherRatioRejectsUnsafeRatios(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
	}{
		{name: "NaN", ratio: math.NaN()},
		{name: "+Inf", ratio: math.Inf(1)},
		{name: "zero", ratio: 0},
		{name: "negative", ratio: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priceData := PriceData{OtherRatios: map[string]float64{"existing": 2}}

			priceData.AddOtherRatio("unsafe", tt.ratio)

			require.NotContains(t, priceData.OtherRatios, "unsafe")
			require.Equal(t, 2.0, priceData.OtherRatios["existing"])
		})
	}
}

func TestPriceDataAddOtherRatioAcceptsPositiveFiniteRatio(t *testing.T) {
	priceData := PriceData{}

	priceData.AddOtherRatio("n", 2.5)

	require.Equal(t, map[string]float64{"n": 2.5}, priceData.OtherRatios)
}
