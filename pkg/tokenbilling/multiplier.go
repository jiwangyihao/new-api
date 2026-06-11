package tokenbilling

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

const (
	DefaultMultiplier = 1.0
	MaxMultiplier     = 100.0
	Epsilon           = 1e-9
)

func ValidateMultiplier(multiplier float64) error {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 || multiplier > MaxMultiplier {
		return fmt.Errorf("渠道扣费倍率必须大于 0 且不超过 100")
	}
	return nil
}

func EffectiveMultiplier(multiplier float64) float64 {
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return DefaultMultiplier
	}
	return multiplier
}

func SameMultiplier(a, b float64) bool {
	return math.Abs(a-b) <= Epsilon
}

func ApplyMultiplier(rawTokens int64, multiplier float64) (int64, error) {
	if rawTokens <= 0 {
		return 0, nil
	}
	if err := ValidateMultiplier(multiplier); err != nil {
		return 0, err
	}
	product := decimal.NewFromInt(rawTokens).Mul(decimal.NewFromFloat(multiplier))
	rounded := product.Round(0).IntPart()
	if rounded < 1 {
		return 1, nil
	}
	return rounded, nil
}

func EquivalentTokens(standardTokens int64, multiplier float64) (int64, error) {
	if standardTokens <= 0 {
		return 0, nil
	}
	if err := ValidateMultiplier(multiplier); err != nil {
		return 0, err
	}
	return decimal.NewFromInt(standardTokens).Div(decimal.NewFromFloat(multiplier)).Floor().IntPart(), nil
}
