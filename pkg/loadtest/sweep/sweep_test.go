package sweep

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
)

func testRunContext() artifact.RunContext {
	return artifact.RunContext{SchemaVersion: 1, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "s2-short-stream", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
}

func TestDeriveRunContextRequiresTokenProfileAndSeed(t *testing.T) {
	base := artifact.RunContext{SchemaVersion: 1, Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", Model: "gpt-5.5"}
	got, err := DeriveRunContext(base, DeriveOptions{Scenario: "s2-short-stream", Path: "/v1/responses", TokenProfile: "subscription", APIKey: "sk-loadtestsub", MockHash: "sha256:mock"})
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenProfile != "subscription" || got.Path != "/v1/responses" {
		t.Fatalf("bad context: %#v", got)
	}
}

func TestBuildMockStatsDeltaUsesPointWindowAndContext(t *testing.T) {
	rc := testRunContext()
	before := artifact.MockStats{SchemaVersion: 1, RunContext: rc, AttemptsTotal: 100, InjectedStatusCounts: map[string]int{"429": 2}}
	after := artifact.MockStats{SchemaVersion: 1, RunContext: rc, AttemptsTotal: 160, InjectedStatusCounts: map[string]int{"429": 5, "502": 1}}
	delta, err := BuildMockStatsDelta(before, after, rc)
	if err != nil {
		t.Fatal(err)
	}
	if delta.UpstreamAttemptsTotal != 60 || delta.Actual429 != 3 || delta.Actual502 != 1 || delta.RunContext != rc || delta.Hash == "" {
		t.Fatalf("bad delta: %#v", delta)
	}
}

func TestS1S2GateRequiresAllBusinessConditions(t *testing.T) {
	invariants := make([]artifact.Invariant, 0, len(RequiredInvariantNames()))
	for _, name := range RequiredInvariantNames() {
		invariants = append(invariants, artifact.Invariant{Name: name, Status: "passed"})
	}
	point := artifact.PointResult{Concurrency: 100, SummaryExcerpt: artifact.SummaryExcerpt{Total: 1000, Success: 1000, StatusCodes: map[string]int{"200": 1000}, MaxObservedInFlight: 95, StreamDoneReceived: 1000, StreamUsageEvents: 1000, StreamBytes: 128000}, Invariants: invariants}
	gate := EvaluateGate("s2-short-stream", point, GateOptions{MockOutputBytes: 128, RequiredInvariantNames: RequiredInvariantNames()})
	if !gate.Passed {
		t.Fatalf("gate failed: %#v", gate)
	}
	point.Invariants = point.Invariants[:1]
	gate = EvaluateGate("s2-short-stream", point, GateOptions{MockOutputBytes: 128, RequiredInvariantNames: RequiredInvariantNames()})
	if gate.Passed {
		t.Fatalf("missing invariant passed")
	}
	point.Invariants = invariants
	point.SummaryExcerpt.MaxObservedInFlight = 89
	gate = EvaluateGate("s2-short-stream", point, GateOptions{MockOutputBytes: 128, RequiredInvariantNames: RequiredInvariantNames()})
	if gate.Passed {
		t.Fatalf("low observed concurrency passed")
	}
	point.SummaryExcerpt.MaxObservedInFlight = 95
	point.SummaryExcerpt.StreamUsageEvents = 999
	gate = EvaluateGate("s2-short-stream", point, GateOptions{MockOutputBytes: 128, RequiredInvariantNames: RequiredInvariantNames()})
	if gate.Passed {
		t.Fatalf("missing stream usage event passed")
	}
	point.SummaryExcerpt.StreamUsageEvents = 1000
	point.SummaryExcerpt.StatusCodes = map[string]int{"200": 999, "500": 1}
	gate = EvaluateGate("s2-short-stream", point, GateOptions{MockOutputBytes: 128, RequiredInvariantNames: RequiredInvariantNames()})
	if gate.Passed {
		t.Fatalf("non-200 status passed")
	}
}

func TestS4GateUsesCurrentPointMockDeltaAndRefundInvariant(t *testing.T) {
	point := artifact.PointResult{SummaryExcerpt: artifact.SummaryExcerpt{Total: 100, Actual429: 5, Actual502: 1, UpstreamAttemptsTotal: 100}, MockDelta: artifact.MockStatsDelta{Actual429: 5, Actual502: 1, UpstreamAttemptsTotal: 100}, Invariants: []artifact.Invariant{{Name: "failure_refund_by_request", Status: "passed"}, {Name: "compat_wallet_not_charged", Status: "passed"}}}
	gate := EvaluateGate("s4-error-refund", point, GateOptions{RequiredInvariantNames: []string{"failure_refund_by_request", "compat_wallet_not_charged"}})
	if !gate.Passed {
		t.Fatalf("gate failed: %#v", gate)
	}
	point.MockDelta.UpstreamAttemptsTotal = 99
	gate = EvaluateGate("s4-error-refund", point, GateOptions{RequiredInvariantNames: []string{"failure_refund_by_request", "compat_wallet_not_charged"}})
	if gate.Passed {
		t.Fatalf("bad mock delta passed")
	}
}

func TestS4GateRequiresDeterministicExpectedErrors(t *testing.T) {
	rate := map[int]float64{429: 0.05, 502: 0.01}
	expected429, expected502 := loadtestconfig.DeterministicErrorCounts(1, 100, rate)
	expectedSuccess := 100 - expected429 - expected502
	point := artifact.PointResult{SummaryExcerpt: artifact.SummaryExcerpt{Total: 100, Success: expectedSuccess, Actual429: expected429, Actual502: expected502, UpstreamAttemptsTotal: 100, NonInjectedErrors: 0}, MockDelta: artifact.MockStatsDelta{Actual429: expected429, Actual502: expected502, UpstreamAttemptsTotal: 100}, Invariants: []artifact.Invariant{{Name: "failure_refund_by_request", Status: "passed"}, {Name: "compat_wallet_not_charged", Status: "passed"}}}
	gate := EvaluateGate("s4-error-refund", point, GateOptions{Seed: 1, StatusRate: rate, RequiredInvariantNames: []string{"failure_refund_by_request", "compat_wallet_not_charged"}})
	if !gate.Passed {
		t.Fatalf("gate failed: %#v", gate)
	}
	point.SummaryExcerpt.Success++
	gate = EvaluateGate("s4-error-refund", point, GateOptions{Seed: 1, StatusRate: rate, RequiredInvariantNames: []string{"failure_refund_by_request", "compat_wallet_not_charged"}})
	if gate.Passed {
		t.Fatalf("wrong deterministic error count passed")
	}
}

func TestS3S5GateRequiresRequestsAndResourceSamples(t *testing.T) {
	invariants := []artifact.Invariant{{Name: "subscription_token_used_matches_success_usage", Status: "passed"}, {Name: "consume_logs_by_request", Status: "passed"}}
	point := artifact.PointResult{Concurrency: 10, SummaryExcerpt: artifact.SummaryExcerpt{Total: 0}, Invariants: invariants}
	gate := EvaluateGate("s5-large-payload", point, GateOptions{RequiredInvariantNames: []string{"subscription_token_used_matches_success_usage", "consume_logs_by_request"}, RequireResourceSamples: true})
	if gate.Passed {
		t.Fatalf("zero request resource scenario passed: %#v", gate)
	}
	point = artifact.PointResult{Concurrency: 10, SummaryExcerpt: artifact.SummaryExcerpt{Total: 10, Success: 10, StatusCodes: map[string]int{"200": 10}, MaxObservedInFlight: 10, StreamDoneReceived: 10, StreamUsageEvents: 10, StreamBytes: 1280}, Invariants: invariants}
	gate = EvaluateGate("s5-large-payload", point, GateOptions{MockOutputBytes: 128, RequiredInvariantNames: []string{"subscription_token_used_matches_success_usage", "consume_logs_by_request"}, RequireResourceSamples: true})
	if gate.Passed {
		t.Fatalf("missing resource sample passed: %#v", gate)
	}
	point.ResourcePeaks = artifact.ResourcePeaks{RSSPeakBytes: 2 << 30, GoroutinesPeak: 100000}
	point.ResourceDelta = artifact.ResourceDelta{RSSBeforeBytes: 100, RSSAfterDrainBytes: 500, GoroutinesBefore: 10, GoroutinesAfterDrain: 100}
	gate = EvaluateGate("s5-large-payload", point, GateOptions{MockOutputBytes: 128, RequiredInvariantNames: []string{"subscription_token_used_matches_success_usage", "consume_logs_by_request"}, RequireResourceSamples: true, MaxRSSBytes: 1 << 30, MaxRSSAfterDrainGrowthBytes: 100, MaxGoroutineAfterDrainGrowth: 20})
	if gate.Passed {
		t.Fatalf("resource leak gate passed: %#v", gate)
	}
}
