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

func sweepWithContext(rc artifact.RunContext) artifact.SweepResult {
	return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Points: []artifact.PointResult{{Concurrency: 100, Passed: true, SummaryExcerpt: artifact.SummaryExcerpt{LatencyP95MS: 100, TTFTP95MS: 200}}}, HighestPassedConcurrency: 100}
}

func sweepWithPoint(concurrency int, latency float64, ttft float64) artifact.SweepResult {
	rc := testRunContext()
	return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Points: []artifact.PointResult{{Concurrency: concurrency, Passed: true, SummaryExcerpt: artifact.SummaryExcerpt{LatencyP95MS: latency, TTFTP95MS: ttft}}}, HighestPassedConcurrency: concurrency}
}

func ptrInt(v int) *int { return &v }
