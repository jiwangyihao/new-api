package operation_setting

import (
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/pkg/routehealth"
	"github.com/QuantumNous/new-api/setting/config"
)

// RouteHealthSetting controls runtime admission, not administrative channel state.
type RouteHealthSetting struct {
	Enabled              bool    `json:"enabled"`
	FailureThreshold     int     `json:"failure_threshold"`
	FailureWindowSeconds int     `json:"failure_window_seconds"`
	SampleWindowSeconds  int     `json:"sample_window_seconds"`
	MinimumSamples       int     `json:"minimum_samples"`
	FailureRatio         float64 `json:"failure_ratio"`
	BaseCooldownSeconds  int     `json:"base_cooldown_seconds"`
	MaxCooldownSeconds   int     `json:"max_cooldown_seconds"`
	MaxInflight          int     `json:"max_inflight"`
	LeaseSeconds         int     `json:"lease_seconds"`
	RecoverySuccesses    int     `json:"recovery_successes"`
	ProbeIntervalSeconds int     `json:"probe_interval_seconds"`
	RetryBudgetSeconds   int     `json:"retry_budget_seconds"`
}

var routeHealthSetting = RouteHealthSetting{
	Enabled: true, FailureThreshold: 3, FailureWindowSeconds: 30,
	SampleWindowSeconds: 60, MinimumSamples: 20, FailureRatio: 0.5,
	BaseCooldownSeconds: 30, MaxCooldownSeconds: 300, MaxInflight: 64,
	LeaseSeconds: 30, RecoverySuccesses: 3, ProbeIntervalSeconds: 5,
	RetryBudgetSeconds: 120,
}

func init() {
	config.GlobalConfig.Register("route_health_setting", &routeHealthSetting)
}

func GetRouteHealthSetting() *RouteHealthSetting { return &routeHealthSetting }

func (s RouteHealthSetting) Policy() routehealth.Policy {
	return routehealth.Policy{
		Enabled: s.Enabled, FailureThreshold: s.FailureThreshold,
		FailureWindow:  time.Duration(s.FailureWindowSeconds) * time.Second,
		SampleWindow:   time.Duration(s.SampleWindowSeconds) * time.Second,
		MinimumSamples: s.MinimumSamples, FailureRatio: s.FailureRatio,
		BaseCooldown: time.Duration(s.BaseCooldownSeconds) * time.Second,
		MaxCooldown:  time.Duration(s.MaxCooldownSeconds) * time.Second,
		MaxInflight:  s.MaxInflight, LeaseDuration: time.Duration(s.LeaseSeconds) * time.Second,
		RecoverySuccesses: s.RecoverySuccesses, ProbeInterval: time.Duration(s.ProbeIntervalSeconds) * time.Second,
	}
}

func (s RouteHealthSetting) Validate() error {
	for _, seconds := range []int{s.FailureWindowSeconds, s.SampleWindowSeconds, s.BaseCooldownSeconds, s.MaxCooldownSeconds, s.LeaseSeconds, s.ProbeIntervalSeconds, s.RetryBudgetSeconds} {
		if seconds < 1 || seconds > 86400 {
			return fmt.Errorf("route health durations must be between 1 and 86400 seconds")
		}
	}
	if s.LeaseSeconds < 3 || s.MaxInflight > 100000 || s.MinimumSamples > 100000 || s.FailureThreshold > 100000 {
		return fmt.Errorf("invalid route health admission or sample limit")
	}
	policy := s.Policy()
	policy.Enabled = true
	return policy.Validate()
}

// WithOption strictly parses a single registered option before any persistence or mutation.
func (s RouteHealthSetting) WithOption(key, value string) (RouteHealthSetting, error) {
	var err error
	switch key {
	case "enabled":
		s.Enabled, err = strconv.ParseBool(value)
	case "failure_ratio":
		s.FailureRatio, err = strconv.ParseFloat(value, 64)
	default:
		var field *int
		switch key {
		case "failure_threshold":
			field = &s.FailureThreshold
		case "failure_window_seconds":
			field = &s.FailureWindowSeconds
		case "sample_window_seconds":
			field = &s.SampleWindowSeconds
		case "minimum_samples":
			field = &s.MinimumSamples
		case "base_cooldown_seconds":
			field = &s.BaseCooldownSeconds
		case "max_cooldown_seconds":
			field = &s.MaxCooldownSeconds
		case "max_inflight":
			field = &s.MaxInflight
		case "lease_seconds":
			field = &s.LeaseSeconds
		case "recovery_successes":
			field = &s.RecoverySuccesses
		case "probe_interval_seconds":
			field = &s.ProbeIntervalSeconds
		case "retry_budget_seconds":
			field = &s.RetryBudgetSeconds
		default:
			return s, fmt.Errorf("unknown route health option %q", key)
		}
		*field, err = strconv.Atoi(value)
	}
	if err != nil {
		return s, err
	}
	return s, s.Validate()
}
