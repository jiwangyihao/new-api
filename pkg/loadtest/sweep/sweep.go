package sweep

import (
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
)

type DeriveOptions struct {
	Scenario     string
	Path         string
	TokenProfile string
	APIKey       string
	MockHash     string
}

type GateOptions struct {
	MockOutputBytes              int
	RequiredInvariantNames       []string
	Seed                         int64
	StatusRate                   map[int]float64
	MaxRSSBytes                  uint64
	MaxRSSAfterDrainGrowthBytes  uint64
	MaxGoroutineAfterDrainGrowth int
}

func DeriveRunContext(base artifact.RunContext, opts DeriveOptions) (artifact.RunContext, error) {
	if opts.TokenProfile == "" || opts.Path == "" || opts.Scenario == "" {
		return artifact.RunContext{}, fmt.Errorf("scenario, path and token-profile are required")
	}
	if opts.APIKey == "" || opts.MockHash == "" || base.SeedOutputHash == "" {
		return artifact.RunContext{}, fmt.Errorf("api key, mock hash and seed output hash are required")
	}
	rc := base
	rc.Scenario = opts.Scenario
	rc.Path = opts.Path
	rc.TokenProfile = opts.TokenProfile
	rc.MockHash = opts.MockHash
	return rc, nil
}

func BuildMockStatsDelta(before, after artifact.MockStats, rc artifact.RunContext) (artifact.MockStatsDelta, error) {
	if before.RunContext != rc || after.RunContext != rc {
		return artifact.MockStatsDelta{}, fmt.Errorf("mock stats run_context mismatch")
	}
	delta := artifact.MockStatsDelta{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Actual429: after.InjectedStatusCounts["429"] - before.InjectedStatusCounts["429"], Actual502: after.InjectedStatusCounts["502"] - before.InjectedStatusCounts["502"], UpstreamAttemptsTotal: after.AttemptsTotal - before.AttemptsTotal}
	hashInput := delta
	hashInput.Hash = ""
	hash, err := artifact.HashCanonical(hashInput)
	if err != nil {
		return artifact.MockStatsDelta{}, err
	}
	delta.Hash = hash
	return delta, nil
}

func RequiredInvariantNames() []string {
	return []string{"subscription_token_used_matches_success_usage", "compat_subscription_token_used_matches_success_usage", "compat_wallet_not_charged", "consume_logs_by_request", "failure_refund_by_request", "quota_data_pending_or_unavailable", "perf_metrics_no_upsert_error", "stdout_no_full_params"}
}

func EvaluateGate(scenario string, point artifact.PointResult, opts GateOptions) artifact.GateResult {
	failed := make([]string, 0)
	failed = append(failed, missingOrFailedInvariants(point.Invariants, opts.RequiredInvariantNames)...)
	switch scenario {
	case "s1-smoke", "s2-short-stream":
		if point.SummaryExcerpt.Total == 0 || point.SummaryExcerpt.Success != point.SummaryExcerpt.Total {
			failed = append(failed, "all requests must succeed")
		}
		if point.SummaryExcerpt.StreamBytes < int64(opts.MockOutputBytes*point.SummaryExcerpt.Success) {
			failed = append(failed, "stream bytes below expected mock output")
		}
	case "s4-error-refund":
		if point.MockDelta.UpstreamAttemptsTotal != point.SummaryExcerpt.UpstreamAttemptsTotal || point.MockDelta.UpstreamAttemptsTotal != point.SummaryExcerpt.Total {
			failed = append(failed, "mock delta must describe current point only")
		}
		if opts.StatusRate != nil {
			expected429, expected502 := loadtestconfig.DeterministicErrorCounts(opts.Seed, point.SummaryExcerpt.Total, opts.StatusRate)
			if point.SummaryExcerpt.Actual429 != expected429 || point.SummaryExcerpt.Actual502 != expected502 || point.MockDelta.Actual429 != expected429 || point.MockDelta.Actual502 != expected502 {
				failed = append(failed, "deterministic injected error counts mismatch")
			}
			if point.SummaryExcerpt.Success != point.SummaryExcerpt.Total-expected429-expected502 {
				failed = append(failed, "success count does not match deterministic errors")
			}
		}
		if point.SummaryExcerpt.NonInjectedErrors != 0 {
			failed = append(failed, "non injected errors present")
		}
	case "s3-long-stream", "s5-large-payload":
		if opts.MaxRSSBytes > 0 && point.ResourcePeaks.RSSPeakBytes > opts.MaxRSSBytes {
			failed = append(failed, "rss peak exceeded")
		}
		if opts.MaxRSSAfterDrainGrowthBytes > 0 && point.ResourceDelta.RSSAfterDrainBytes > point.ResourceDelta.RSSBeforeBytes+opts.MaxRSSAfterDrainGrowthBytes {
			failed = append(failed, "rss after-drain growth exceeded")
		}
		if opts.MaxGoroutineAfterDrainGrowth > 0 && point.ResourceDelta.GoroutinesAfterDrain > point.ResourceDelta.GoroutinesBefore+opts.MaxGoroutineAfterDrainGrowth {
			failed = append(failed, "goroutine after-drain growth exceeded")
		}
	}
	return artifact.GateResult{Passed: len(failed) == 0, FailedReasons: failed}
}

func missingOrFailedInvariants(invariants []artifact.Invariant, required []string) []string {
	if len(required) == 0 {
		return nil
	}
	byName := make(map[string]artifact.Invariant, len(invariants))
	for _, inv := range invariants {
		byName[inv.Name] = inv
	}
	failed := make([]string, 0)
	for _, name := range required {
		inv, ok := byName[name]
		if !ok {
			failed = append(failed, "missing invariant "+name)
			continue
		}
		if inv.Status != "passed" {
			failed = append(failed, "failed invariant "+name)
		}
	}
	sort.Strings(failed)
	return failed
}
