package report

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

func testRunContext() artifact.RunContext {
	return artifact.RunContext{SchemaVersion: 1, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "s2-short-stream", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
}

func TestCompareReportRejectsEveryComparableMismatch(t *testing.T) {
	fields := map[string]func(*artifact.RunContext){
		"comparison_config_hash": func(r *artifact.RunContext) { r.ComparisonConfigHash = "sha256:other" },
		"mock_hash":              func(r *artifact.RunContext) { r.MockHash = "sha256:different" },
		"cache_mode":             func(r *artifact.RunContext) { r.CacheMode = "warm" },
		"scenario":               func(r *artifact.RunContext) { r.Scenario = "s3-long-stream" },
		"path":                   func(r *artifact.RunContext) { r.Path = "/v1/chat/completions" },
		"token_profile":          func(r *artifact.RunContext) { r.TokenProfile = "compat" },
	}
	for name, mutate := range fields {
		base := sweepWithContext(testRunContext())
		candidate := sweepWithContext(testRunContext())
		mutate(&candidate.RunContext)
		if _, err := BuildCompareReport(base, candidate, Thresholds{}); err == nil {
			t.Fatalf("%s mismatch accepted", name)
		}
	}
}

func TestCompareReportFailsOnRegressionThreshold(t *testing.T) {
	baseline := sweepWithPoint(100, 100, 200)
	candidate := sweepWithPoint(100, 130, 200)
	_, err := BuildCompareReport(baseline, candidate, Thresholds{LatencyP95RegressionRatio: 1.10})
	if err == nil {
		t.Fatal("want regression error")
	}
}

func TestReportRendersFirstFailedConcurrency(t *testing.T) {
	sweep := artifact.SweepResult{RunContext: testRunContext(), Points: []artifact.PointResult{{Concurrency: 50, Passed: true}, {Concurrency: 100, Passed: false}}, FirstFailedConcurrency: ptrInt(100), HighestPassedConcurrency: 50}
	md := RenderSingleReport(sweep, nil)
	if !strings.Contains(md, "100") || !strings.Contains(md, "50") {
		t.Fatalf("missing concurrency summary: %s", md)
	}
}

func TestReportRendersUnavailableAsUnavailableNotZero(t *testing.T) {
	md := RenderSingleReport(artifact.SweepResult{RunContext: testRunContext()}, []artifact.Diff{{RuntimeDelta: artifact.RuntimeDelta{Statused: artifact.Statused{Status: "unavailable", Reason: "runtime route missing"}}}})
	if !strings.Contains(md, "unavailable") || strings.Contains(md, "runtime route missing | 0") {
		t.Fatalf("bad unavailable rendering: %s", md)
	}
}

func TestRenderResourceSweepReportIncludesCapacityAndResources(t *testing.T) {
	rc := testRunContext()
	firstFailed := 500
	input := ResourceSweepReportInput{
		Sweep: artifact.SweepResult{
			SchemaVersion:            artifact.SchemaVersion,
			RunContext:               rc,
			HighestPassedConcurrency: 250,
			FirstFailedConcurrency:   &firstFailed,
			Points: []artifact.PointResult{
				{Concurrency: 250, Passed: true, SummaryExcerpt: artifact.SummaryExcerpt{Total: 3000, Success: 3000}},
				{Concurrency: 500, Passed: false, SummaryExcerpt: artifact.SummaryExcerpt{Total: 3000, Success: 2800, Errors: 200}},
			},
		},
		Analyses: []artifact.PointAnalysis{
			{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Concurrency: 250, FailureClass: "passed", HardGate: artifact.GateResult{Passed: true}},
			{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Concurrency: 500, FailureClass: "capacity_limit", HardGate: artifact.GateResult{Passed: false, FailedReasons: []string{"latency"}}},
		},
		ResourceSamples: []artifact.ResourceSamplesArtifact{
			{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Concurrency: 250, Peaks: artifact.ResourcePeaks{RSSPeakBytes: 128 << 20, CPUPercentPeak: 95.5, HeapAllocPeakBytes: 48 << 20, RedisUsedMemoryPeakBytes: 12 << 20, PostgresActiveConnectionsPeak: 7}},
			{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Concurrency: 500, Peaks: artifact.ResourcePeaks{RSSPeakBytes: 256 << 20, CPUPercentPeak: 99.9, HeapAllocPeakBytes: 96 << 20, RedisUsedMemoryPeakBytes: 18 << 20, PostgresActiveConnectionsPeak: 10}},
		},
		Limits: artifact.ResourceLimitsArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, ServerEnv: map[string]string{"GOMEMLIMIT": "384MiB", "SQL_MAX_OPEN_CONNS": "64", "SQL_MAX_IDLE_CONNS": "64", "REDIS_POOL_SIZE": "256", "REDIS_IDLE_TIMEOUT_SECONDS": "1", "RELAY_MAX_IDLE_CONNS": "1024", "RELAY_MAX_IDLE_CONNS_PER_HOST": "1024"}, ServerProcessMemoryLimitBytes: 512 << 20, ServerCPUAffinityCores: 2, Statused: artifact.Statused{Status: "ok"}},
		Ports:  artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "closed", "15432": "closed"}, Passed: true},
	}
	md := RenderResourceSweep(input)
	for _, want := range []string{"最高通过并发", "第一失败并发", "failure_class", "GOMEMLIMIT=384MiB", "SQL_MAX_OPEN_CONNS=64", "REDIS_POOL_SIZE=256", "REDIS_IDLE_TIMEOUT_SECONDS=1", "RELAY_MAX_IDLE_CONNS=1024", "RSS peak", "CPU peak", "runtime heap", "Redis used_memory", "PostgreSQL active", "ports closed", "capacity_limit"} {
		if !strings.Contains(md, want) {
			t.Fatalf("report missing %q:\n%s", want, md)
		}
	}
}

func TestRenderResourceSweepReportIncludesCapacityEnvWhenLimitsPartial(t *testing.T) {
	rc := testRunContext()
	input := ResourceSweepReportInput{
		Sweep: artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc},
		Limits: artifact.ResourceLimitsArtifact{
			SchemaVersion:                 artifact.SchemaVersion,
			RunContext:                    rc,
			ServerEnv:                     map[string]string{"GOMEMLIMIT": "384MiB", "SQL_MAX_OPEN_CONNS": "64", "REDIS_POOL_SIZE": "256", "REDIS_IDLE_TIMEOUT_SECONDS": "1", "RELAY_MAX_IDLE_CONNS": "1024"},
			ServerProcessMemoryLimitBytes: 512 << 20,
			ServerCPUAffinityCores:        2,
			Statused:                      artifact.Statused{Status: "partial", Reason: "job object assignment denied"},
		},
	}
	md := RenderResourceSweep(input)
	for _, want := range []string{"limits: partial - job object assignment denied", "GOMEMLIMIT=384MiB", "SQL_MAX_OPEN_CONNS=64", "REDIS_POOL_SIZE=256", "REDIS_IDLE_TIMEOUT_SECONDS=1", "RELAY_MAX_IDLE_CONNS=1024", "process_memory_limit_bytes: 536870912", "cpu_affinity_cores: 2"} {
		if !strings.Contains(md, want) {
			t.Fatalf("partial report missing %q:\n%s", want, md)
		}
	}
}

func sweepWithContext(rc artifact.RunContext) artifact.SweepResult {
	return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Points: []artifact.PointResult{{Concurrency: 100, Passed: true, SummaryExcerpt: artifact.SummaryExcerpt{LatencyP95MS: 100, TTFTP95MS: 200}}}, HighestPassedConcurrency: 100}
}

func sweepWithPoint(concurrency int, latency float64, ttft float64) artifact.SweepResult {
	rc := testRunContext()
	return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Points: []artifact.PointResult{{Concurrency: concurrency, Passed: true, SummaryExcerpt: artifact.SummaryExcerpt{LatencyP95MS: latency, TTFTP95MS: ttft}}}, HighestPassedConcurrency: concurrency}
}

func ptrInt(v int) *int { return &v }
