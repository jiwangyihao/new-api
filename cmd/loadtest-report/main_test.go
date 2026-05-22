package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

func TestCompareReportHonorsFailOnRegression(t *testing.T) {
	dir := t.TempDir()
	base := reportCommandSweep(100)
	candidate := reportCommandSweep(130)
	basePath := filepath.Join(dir, "base.json")
	candidatePath := filepath.Join(dir, "candidate.json")
	thresholdsPath := filepath.Join(dir, "thresholds.json")
	outPath := filepath.Join(dir, "report.md")
	writeReportCommandJSON(t, basePath, base)
	writeReportCommandJSON(t, candidatePath, candidate)
	writeReportCommandJSON(t, thresholdsPath, map[string]float64{"latency_p95_regression_ratio": 1.10})

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"--baseline-sweep", basePath, "--candidate-sweep", candidatePath, "--thresholds", thresholdsPath, "--out", outPath}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("default compare mode should write a regression report, exit=%d stderr=%s", exit, stderr.String())
	}
	md, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "Comparison: failed") {
		t.Fatalf("missing regression marker: %s", string(md))
	}

	exit = Run([]string{"--baseline-sweep", basePath, "--candidate-sweep", candidatePath, "--thresholds", thresholdsPath, "--fail-on-regression", "--out", filepath.Join(dir, "fail.md")}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("--fail-on-regression should return non-zero on regression")
	}
}

func TestResourceSweepReportReadsAnalysesSamplesLimitsAndPorts(t *testing.T) {
	dir := t.TempDir()
	rc := reportRunContext()
	firstFailed := 4
	sweep := artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, HighestPassedConcurrency: 2, FirstFailedConcurrency: &firstFailed, Points: []artifact.PointResult{{Concurrency: 2, Passed: true}, {Concurrency: 4, Passed: false}}}
	writeReportCommandJSON(t, filepath.Join(dir, "resource-sweep.json"), sweep)
	pointsDir := filepath.Join(dir, "points")
	writeReportCommandJSON(t, filepath.Join(pointsDir, "c2-analysis.json"), artifact.PointAnalysis{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Concurrency: 2, FailureClass: "passed"})
	writeReportCommandJSON(t, filepath.Join(pointsDir, "c4-analysis.json"), artifact.PointAnalysis{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Concurrency: 4, FailureClass: "capacity_limit"})
	writeReportCommandJSON(t, filepath.Join(pointsDir, "c2-resource-samples.json"), artifact.ResourceSamplesArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Concurrency: 2, Peaks: artifact.ResourcePeaks{RSSPeakBytes: 128 << 20, CPUPercentPeak: 50, HeapAllocPeakBytes: 32 << 20, RedisUsedMemoryPeakBytes: 8 << 20, PostgresActiveConnectionsPeak: 5}})
	writeReportCommandJSON(t, filepath.Join(pointsDir, "c4-resource-samples.json"), artifact.ResourceSamplesArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Concurrency: 4, Peaks: artifact.ResourcePeaks{RSSPeakBytes: 256 << 20, CPUPercentPeak: 90, HeapAllocPeakBytes: 64 << 20, RedisUsedMemoryPeakBytes: 10 << 20, PostgresActiveConnectionsPeak: 9}})
	limitsPath := filepath.Join(dir, "resource-limits.json")
	portsPath := filepath.Join(dir, "ports-closed.json")
	writeReportCommandJSON(t, limitsPath, artifact.ResourceLimitsArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, ServerEnv: map[string]string{"GOMEMLIMIT": "384MiB"}, Statused: artifact.Statused{Status: "ok"}})
	writeReportCommandJSON(t, portsPath, artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "closed"}, Passed: true})

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"--resource-sweep", filepath.Join(dir, "resource-sweep.json"), "--analysis-dir", pointsDir, "--resource-samples-dir", pointsDir, "--resource-limits", limitsPath, "--ports-closed", portsPath}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	for _, want := range []string{"最高通过并发", "failure_class", "capacity_limit", "GOMEMLIMIT=384MiB", "RSS peak", "ports closed"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("report missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestResourceSweepReportRequiresLimitsAndPorts(t *testing.T) {
	dir := t.TempDir()
	writeReportCommandJSON(t, filepath.Join(dir, "resource-sweep.json"), artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: reportRunContext()})

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"--resource-sweep", filepath.Join(dir, "resource-sweep.json")}, &stdout, &stderr)
	if exit != 2 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "resource-limits") || !strings.Contains(stderr.String(), "ports-closed") {
		t.Fatalf("missing required artifact error: %s", stderr.String())
	}
}

func reportCommandSweep(latency float64) artifact.SweepResult {
	rc := reportRunContext()
	return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Points: []artifact.PointResult{{Concurrency: 1, Passed: true, SummaryExcerpt: artifact.SummaryExcerpt{LatencyP95MS: latency}}}, HighestPassedConcurrency: 1}
}

func reportRunContext() artifact.RunContext {
	return artifact.RunContext{SchemaVersion: artifact.SchemaVersion, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "s1-smoke", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
}

func writeReportCommandJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := common.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
