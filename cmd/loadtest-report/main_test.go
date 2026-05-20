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

func reportCommandSweep(latency float64) artifact.SweepResult {
	rc := artifact.RunContext{SchemaVersion: artifact.SchemaVersion, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "s1-smoke", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
	return artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Points: []artifact.PointResult{{Concurrency: 1, Passed: true, SummaryExcerpt: artifact.SummaryExcerpt{LatencyP95MS: latency}}}, HighestPassedConcurrency: 1}
}

func writeReportCommandJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := common.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
