package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/QuantumNous/new-api/pkg/loadtest/mockopenai"
)

func TestRunRejectsSweepWithoutRealInputs(t *testing.T) {
	dir := t.TempDir()
	rc := sweepTestRunContext()
	runContextPath := filepath.Join(dir, "run-context.json")
	writeTestJSON(t, runContextPath, rc)
	outPath := filepath.Join(dir, "sweep.json")

	exit := Run([]string{
		"--run-context", runContextPath,
		"--scenario", rc.Scenario,
		"--path", rc.Path,
		"--token-profile", rc.TokenProfile,
		"--api-key", "sk-loadtestsub",
		"--points", "1",
		"--max-requests-per-point", "1",
		"--out", outPath,
	}, ioDiscard(), ioDiscard())
	if exit == 0 {
		t.Fatal("sweep without URL and mock stats should fail instead of writing a passed zero-request point")
	}
}

func TestRunExecutesClientAndWritesPointArtifacts(t *testing.T) {
	dir := t.TempDir()
	rc := sweepTestRunContext()
	seed := artifact.SeedOutput{SchemaVersion: artifact.SchemaVersion, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), ExpectedUsagePerSuccess: artifact.Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}}
	seedHash, err := artifact.HashSeedOutput(seed)
	if err != nil {
		t.Fatal(err)
	}
	rc.SeedOutputHash = seedHash
	seed.RunContext = rc.WithoutSeedOutputHash().WithoutMockHash()

	mockServer := httptest.NewServer(mockopenai.NewServer(mockopenai.Config{RunContext: rc, OutputBytes: 128, PromptTokens: 11, CompletionTokens: 17, ChunkInterval: time.Millisecond}))
	defer mockServer.Close()

	runContextPath := filepath.Join(dir, "run-context.json")
	seedPath := filepath.Join(dir, "seed.json")
	outPath := filepath.Join(dir, "sweep.json")
	artifactDir := filepath.Join(dir, "artifacts")
	configPath := filepath.Join(dir, "config.yaml")
	writeTestJSON(t, runContextPath, rc)
	writeTestJSON(t, seedPath, seed)
	writeSweepConfig(t, configPath)

	var stdout, stderr bytes.Buffer
	exit := Run([]string{
		"--config", configPath,
		"--url", mockServer.URL,
		"--api-key", "sk-loadtestsub",
		"--token-profile", "subscription",
		"--path", "/v1/responses",
		"--model", "gpt-5.5",
		"--scenario", "s1-smoke",
		"--points", "2",
		"--max-requests-per-point", "3",
		"--timeout", "5s",
		"--input-bytes", "8",
		"--output-bytes", "128",
		"--seed-output", seedPath,
		"--mock-stats", mockServer.URL + "/debug/loadtest/mock-stats",
		"--run-context", runContextPath,
		"--mock-hash", rc.MockHash,
		"--artifact-dir", artifactDir,
		"--out", outPath,
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var result artifact.SweepResult
	readTestJSON(t, outPath, &result)
	if len(result.Points) != 1 {
		t.Fatalf("points=%d", len(result.Points))
	}
	point := result.Points[0]
	if point.SummaryExcerpt.Total != 3 || point.MockDelta.UpstreamAttemptsTotal != 3 {
		t.Fatalf("point did not execute real requests: %#v", point)
	}
	if point.SummaryPath == "" || point.MetricsDiffPath == "" || point.MockDelta.Path == "" {
		t.Fatalf("missing point artifacts: %#v", point)
	}
	if !strings.Contains(point.SummaryPath, "c2-summary.json") {
		t.Fatalf("summary path should include point concurrency: %s", point.SummaryPath)
	}
}

func sweepTestRunContext() artifact.RunContext {
	return artifact.RunContext{SchemaVersion: artifact.SchemaVersion, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "s1-smoke", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := writeJSONFile(path, value); err != nil {
		t.Fatal(err)
	}
}

func readTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := readJSON(path, value); err != nil {
		t.Fatal(err)
	}
}

func writeSweepConfig(t *testing.T, path string) {
	t.Helper()
	content := `server:
  host: "127.0.0.1"
  port: 13080
  pprof_addr: "127.0.0.1:8005"
  runtime_stats_enabled: true
postgres:
  dsn: "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable"
log_postgres:
  dsn: ""
redis:
  addr: "redis://127.0.0.1:16379/0"
mock_upstream:
  base_url: "http://127.0.0.1:19080"
loadtest:
  model: "gpt-5.5"
  group: "default"
  subscription_key: "sk-loadtestsub"
  compat_key: "sk-loadtestcompat"
  invalid_key: "sk-loadtestinvalid"
  token_db_key_subscription: "loadtestsub"
  token_db_key_compat: "loadtestcompat"
mock_profiles:
  s1-smoke: {first_token_delay: 50ms, stream_duration: 500ms, chunk_interval: 50ms, output_bytes: 128, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
  s2-short-stream: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
  s3-long-stream: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
  s4-error-refund: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0.05, 502: 0.01}, seed: 1}
  s5-large-payload: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
retry:
  retry_times: 0
  automatic_retry_status_codes: []
thresholds:
  latency_p95_regression_ratio: 1.10
  ttft_p95_regression_ratio: 1.10
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func ioDiscard() *bytes.Buffer { return &bytes.Buffer{} }

var _ = http.MethodGet
