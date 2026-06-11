package tokenbilling

import (
	"math"
	"testing"
)

func TestApplyMultiplier(t *testing.T) {
	tests := []struct {
		name       string
		rawTokens  int64
		multiplier float64
		want       int64
	}{
		{name: "zero raw tokens", rawTokens: 0, multiplier: 2, want: 0},
		{name: "identity multiplier", rawTokens: 1, multiplier: 1, want: 1},
		{name: "half up rounds .5 up", rawTokens: 1, multiplier: 1.5, want: 2},
		{name: "half up rounds fractional product", rawTokens: 3, multiplier: 0.5, want: 2},
		{name: "positive result floors to one token", rawTokens: 1, multiplier: 0.1, want: 1},
		{name: "large multiplier", rawTokens: 10000, multiplier: 2, want: 20000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplyMultiplier(tt.rawTokens, tt.multiplier)
			if err != nil {
				t.Fatalf("ApplyMultiplier() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ApplyMultiplier() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestValidateMultiplierRejectsInvalidValues(t *testing.T) {
	invalid := []float64{0, -1, 100.01, math.NaN(), math.Inf(1)}
	for _, multiplier := range invalid {
		if err := ValidateMultiplier(multiplier); err == nil {
			t.Fatalf("ValidateMultiplier(%v) expected error", multiplier)
		}
	}
}

func TestEquivalentTokens(t *testing.T) {
	tests := []struct {
		name           string
		standardTokens int64
		multiplier     float64
		want           int64
	}{
		{name: "floor below one", standardTokens: 1, multiplier: 2, want: 0},
		{name: "half standard tokens", standardTokens: 1000000, multiplier: 2, want: 500000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EquivalentTokens(tt.standardTokens, tt.multiplier)
			if err != nil {
				t.Fatalf("EquivalentTokens() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("EquivalentTokens() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSameMultiplier(t *testing.T) {
	if !SameMultiplier(1, 1+5e-10) {
		t.Fatal("SameMultiplier() expected values within epsilon to match")
	}
	if SameMultiplier(1, 1.0001) {
		t.Fatal("SameMultiplier() expected values outside epsilon not to match")
	}
}
