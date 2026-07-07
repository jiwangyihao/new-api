package common

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// Quota conversions are centralized here so every billing path shares one
// saturation + logging policy. Quota columns (user/token/log) are 32-bit
// integers in the database, so an oversized product must clamp to the int32
// range instead of wrapping around and turning a charge into a credit.
const (
	MaxQuota = math.MaxInt32
	MinQuota = math.MinInt32
)

// Clamp kinds reported by QuotaClamp.Kind.
const (
	QuotaClampOverflow  = "overflow"
	QuotaClampUnderflow = "underflow"
	QuotaClampNaN       = "nan"
)

// QuotaClamp describes a single saturation event: a quota conversion whose
// input fell outside the representable int32 range (or was NaN) and was
// therefore clamped. It is surfaced to billing callers so the event can be
// recorded on the related consume/task log for admin auditing.
type QuotaClamp struct {
	Op       string  `json:"op"`
	Kind     string  `json:"kind"`
	Original float64 `json:"original"`
	Clamped  int     `json:"clamped"`
}

// AuditMap renders the clamp as the marker stored under a log's
// admin_info.quota_saturation.
func (c *QuotaClamp) AuditMap() map[string]interface{} {
	if c == nil {
		return nil
	}
	return map[string]interface{}{
		"op":       c.Op,
		"kind":     c.Kind,
		"original": c.Original,
		"clamped":  c.Clamped,
	}
}

func saturateQuota(value float64, op string) (int, *QuotaClamp) {
	switch {
	case math.IsNaN(value):
		SysError(fmt.Sprintf("quota conversion (%s) received NaN, falling back to 0", op))
		return 0, &QuotaClamp{Op: op, Kind: QuotaClampNaN, Original: value, Clamped: 0}
	case value >= MaxQuota:
		SysError(fmt.Sprintf("quota conversion (%s) overflow: %g exceeds max quota, clamped to %d", op, value, MaxQuota))
		return MaxQuota, &QuotaClamp{Op: op, Kind: QuotaClampOverflow, Original: value, Clamped: MaxQuota}
	case value <= MinQuota:
		SysError(fmt.Sprintf("quota conversion (%s) underflow: %g below min quota, clamped to %d", op, value, MinQuota))
		return MinQuota, &QuotaClamp{Op: op, Kind: QuotaClampUnderflow, Original: value, Clamped: MinQuota}
	default:
		return int(value), nil
	}
}

// QuotaFromFloat converts a computed quota value to int, truncating toward
// zero, with saturation. Use for float products of prices, ratios, and
// user-controlled multipliers.
func QuotaFromFloat(value float64) int {
	quota, _ := QuotaFromFloatChecked(value)
	return quota
}

func QuotaFromFloatChecked(value float64) (int, *QuotaClamp) {
	return saturateQuota(value, "QuotaFromFloat")
}

// QuotaRound converts a float64 quota value to int using half-away-from-zero
// rounding, with saturation.
func QuotaRound(value float64) int {
	quota, _ := QuotaRoundChecked(value)
	return quota
}

func QuotaRoundChecked(value float64) (int, *QuotaClamp) {
	return saturateQuota(math.Round(value), "QuotaRound")
}

// QuotaFromDecimal converts a computed quota decimal to int with saturation.
// The decimal is rounded half-away-from-zero before conversion.
func QuotaFromDecimal(d decimal.Decimal) int {
	quota, _ := QuotaFromDecimalChecked(d)
	return quota
}

func QuotaFromDecimalChecked(d decimal.Decimal) (int, *QuotaClamp) {
	f, _ := d.Round(0).Float64()
	return saturateQuota(f, "QuotaFromDecimal")
}
