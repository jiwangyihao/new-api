package analysis

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

const mib = 1024 * 1024

func TestBenchmarkHardGateRequiresAllRequestsAndResources(t *testing.T) {
	point := analysisPassedPoint()
	point.Passed = false
	point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{"all requests must succeed"}}

	got := EvaluateBenchmarkPoint(Inputs{Point: point})
	if got.HardGate.Passed || !hasExactReason(got.HardGate.FailedReasons, "all requests must succeed") {
		t.Fatalf("hard gate did not preserve sweep failure: %#v", got.HardGate)
	}

	point = analysisPassedPoint()
	got = EvaluateBenchmarkPoint(Inputs{Point: point})
	if got.HardGate.Passed || !hasExactReason(got.HardGate.FailedReasons, "resource samples are required") {
		t.Fatalf("hard gate did not require resource samples: %#v", got.HardGate)
	}
}

func TestFailureClassPrioritizesBillingInvariant(t *testing.T) {
	in := analysisBaseInput()
	in.Summary.ErrorReasons = map[string]int{"connect_refused": 1, "http_500": 3}
	in.BusinessDiff.BusinessDelta.Invariants = []artifact.Invariant{{Name: "subscription_token_used_matches_success_usage", Status: "failed", Reason: "token delta mismatch"}}

	if got := ClassifyFailure(in); got != "billing_invariant" {
		t.Fatalf("failure class = %q", got)
	}
}

func TestFailureClassDetectsClientTransport(t *testing.T) {
	for _, reason := range []string{"connect_refused", "connect_timeout"} {
		t.Run(reason, func(t *testing.T) {
			in := analysisBaseInput()
			in.Summary.ErrorReasons = map[string]int{reason: 8, "json_error": 1}
			if got := ClassifyFailure(in); got != "client_transport" {
				t.Fatalf("failure class = %q", got)
			}
		})
	}
}

func TestFailureClassDetectsStreamProtocol(t *testing.T) {
	in := analysisBaseInput()
	in.Summary.ErrorReasons = map[string]int{"missing_done": 9, "json_error": 1}
	in.Summary.StatusCodes = map[string]int{"200": 10}
	if got := ClassifyFailure(in); got != "stream_protocol" {
		t.Fatalf("missing_done failure class = %q", got)
	}

	for _, reason := range []string{"json_error", "read_error", "stream_parse_invalid_event"} {
		t.Run(reason, func(t *testing.T) {
			in := analysisBaseInput()
			in.Summary.ErrorReasons = map[string]int{reason: 7, "missing_done": 1}
			in.Summary.StatusCodes = map[string]int{"200": 8}
			if got := ClassifyFailure(in); got != "client_parser_failure" {
				t.Fatalf("failure class = %q", got)
			}
		})
	}
}

func TestFailureClassDetectsCleanupFailed(t *testing.T) {
	in := analysisBaseInput()
	in.RequirePorts = true
	in.Ports = artifact.PortsClosedArtifact{Passed: false, Ports: map[string]string{"13080": "open"}}
	in.Summary.ErrorReasons = map[string]int{"connect_refused": 10}

	if got := ClassifyFailure(in); got != "cleanup_failed" {
		t.Fatalf("failure class = %q", got)
	}
}

func TestDiagnosticGateClassifiesResourceBottlenecks(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
		edit func(*Inputs)
	}{
		{
			name: "server memory close to limit",
			want: "server_resource_limit",
			edit: func(in *Inputs) {
				in.ResourceSamples.Peaks.RSSPeakBytes = 370 * mib
				in.ResourceSamples.Samples[0].Runtime.GOMEMLimitBytes = 384 * mib
			},
		},
		{
			name: "postgres pool close to limit",
			want: "postgres_bottleneck",
			edit: func(in *Inputs) {
				in.ResourceSamples.Peaks.PostgresActiveConnectionsPeak = 9
				in.ResourceSamples.Peaks.PostgresIdleConnectionsPeak = 1
			},
		},
		{
			name: "redis commands per success abnormal",
			want: "redis_bottleneck",
			edit: func(in *Inputs) {
				in.Summary.Success = 3000
				in.Point.SummaryExcerpt.Success = 3000
				in.ResourceSamples.Peaks.RedisTotalCommandsProcessedPeak = 10
			},
		},
		{
			name: "redis unavailable",
			want: "redis_bottleneck",
			edit: func(in *Inputs) {
				in.ResourceSamples.Samples[0].Redis.Statused = artifact.Statused{Status: "unavailable", Reason: "redis ping failed"}
			},
		},
		{
			name: "mock delta mismatch",
			want: "upstream_mock",
			edit: func(in *Inputs) {
				in.Point.MockDelta.UpstreamAttemptsTotal = 2999
			},
		},
		{
			name: "profile capacity limit",
			want: "capacity_limit",
			edit: func(in *Inputs) {
				in.Point.Concurrency = 2000
				in.Point.SummaryExcerpt.MaxObservedInFlight = 2000
				in.Summary.TargetConcurrency = 2000
				in.Summary.MaxObservedInFlight = 2000
			},
		},
		{
			name: "unknown",
			want: "unknown",
			edit: func(in *Inputs) {},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := analysisBaseInput()
			tc.edit(&in)
			got := EvaluateBenchmarkPoint(in)
			if got.FailureClass != tc.want {
				t.Fatalf("failure class = %q, want %q, analysis=%#v", got.FailureClass, tc.want, got)
			}
			if tc.want != "unknown" && !hasDiagnosticClass(got.DiagnosticGate.DiagnosticReasons, tc.want) {
				t.Fatalf("diagnostic reasons missing %q: %#v", tc.want, got.DiagnosticGate)
			}
		})
	}
}

func TestFailureClassDoesNotClassifySubCeilingNearTargetAsCapacityLimit(t *testing.T) {
	in := analysisBaseInput()
	in.Point.Concurrency = 250
	in.Point.SummaryExcerpt.MaxObservedInFlight = 250
	in.Summary.TargetConcurrency = 250
	in.Summary.MaxObservedInFlight = 250

	if got := ClassifyFailure(in); got == FailureCapacityLimit {
		t.Fatalf("sub-ceiling near-target point classified as capacity limit")
	}
}

func analysisRunContext() artifact.RunContext {
	return artifact.RunContext{SchemaVersion: artifact.SchemaVersion, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "benchmark", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
}

func analysisPassedPoint() artifact.PointResult {
	rc := analysisRunContext()
	return artifact.PointResult{
		Concurrency: 250,
		Passed:      true,
		SummaryExcerpt: artifact.SummaryExcerpt{
			Total:                 3000,
			Success:               3000,
			Errors:                0,
			StatusCodes:           map[string]int{"200": 3000},
			MaxObservedInFlight:   250,
			StreamDoneReceived:    3000,
			StreamUsageEvents:     3000,
			StreamBytes:           384000,
			StopReason:            "max_requests",
			UpstreamAttemptsTotal: 3000,
		},
		MockDelta: artifact.MockStatsDelta{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Actual429: 0, Actual502: 0, UpstreamAttemptsTotal: 3000},
		Gate:      artifact.GateResult{Passed: true},
	}
}

func analysisBaseInput() Inputs {
	rc := analysisRunContext()
	point := analysisPassedPoint()
	point.Passed = false
	point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{"unclassified failure"}}
	point.SummaryExcerpt.MaxObservedInFlight = 100
	return Inputs{
		Point: point,
		Summary: artifact.Summary{
			SchemaVersion:       artifact.SchemaVersion,
			RunContext:          rc,
			Scenario:            "benchmark",
			TokenProfile:        "subscription",
			TargetConcurrency:   point.Concurrency,
			Total:               3000,
			Success:             3000,
			Errors:              1,
			StatusCodes:         map[string]int{"200": 2999, "500": 1},
			MaxObservedInFlight: 100,
		},
		ResourceSamples: artifact.ResourceSamplesArtifact{
			SchemaVersion: artifact.SchemaVersion,
			RunContext:    rc,
			Concurrency:   point.Concurrency,
			Samples: []artifact.ResourceSample{{
				UnixMilli: 1,
				Process:   artifact.ProcessSnapshot{Statused: artifact.Statused{Status: "ok"}, RSSBytes: 128 * mib},
				Runtime:   artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "ok"}, Goroutines: 100, HeapAllocBytes: 64 * mib, GOMEMLimitBytes: 384 * mib},
				Postgres:  artifact.PostgresSnapshot{Statused: artifact.Statused{Status: "ok"}, ActiveConnections: 1, IdleConnections: 1},
				Redis:     artifact.RedisSnapshot{Statused: artifact.Statused{Status: "ok"}, TotalCommandsProcessed: 6000, UsedMemoryBytes: 8 * mib},
			}},
			Peaks: artifact.ResourcePeaks{RSSPeakBytes: 128 * mib, GoroutinesPeak: 100, HeapAllocPeakBytes: 64 * mib, RedisTotalCommandsProcessedPeak: 6000, PostgresActiveConnectionsPeak: 1},
			Drain: artifact.Statused{Status: "passed"},
		},
		BusinessDiff: artifact.Diff{SchemaVersion: artifact.SchemaVersion, RunContext: rc},
		MaxRequests:  3000,
	}
}

func hasExactReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func hasDiagnosticClass(reasons []string, want string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, want) {
			return true
		}
	}
	return false
}
