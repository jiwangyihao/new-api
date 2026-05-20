package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/QuantumNous/new-api/pkg/loadtest/metrics"
	"github.com/QuantumNous/new-api/pkg/loadtest/mockopenai"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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

func TestRunRejectsURLOutsideConfiguredNewAPI(t *testing.T) {
	dir := t.TempDir()
	rc := sweepTestRunContext()
	seed := artifact.SeedOutput{SchemaVersion: artifact.SchemaVersion, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), UserIDSubscription: 1, TokenDBKeySubscription: "loadtestsub", ExpectedUsagePerSuccess: artifact.Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}}
	seedHash, err := artifact.HashSeedOutput(seed)
	if err != nil {
		t.Fatal(err)
	}
	rc.SeedOutputHash = seedHash
	seed.RunContext = rc.WithoutSeedOutputHash().WithoutMockHash()
	runContextPath := filepath.Join(dir, "run-context.json")
	seedPath := filepath.Join(dir, "seed.json")
	configPath := filepath.Join(dir, "config.yaml")
	writeTestJSON(t, runContextPath, rc)
	writeTestJSON(t, seedPath, seed)
	writeSweepConfig(t, configPath, "http://127.0.0.1:13080")
	exit := Run([]string{
		"--config", configPath,
		"--url", "http://127.0.0.1:13081",
		"--api-key", "sk-loadtestsub",
		"--token-profile", "subscription",
		"--path", "/v1/responses",
		"--scenario", "s1-smoke",
		"--points", "1",
		"--max-requests-per-point", "1",
		"--output-bytes", "128",
		"--seed-output", seedPath,
		"--mock-stats", filepath.Join(dir, "mock-stats.json"),
		"--run-context", runContextPath,
		"--mock-hash", rc.MockHash,
		"--artifact-dir", filepath.Join(dir, "artifacts"),
		"--out", filepath.Join(dir, "sweep.json"),
	}, ioDiscard(), ioDiscard())
	if exit == 0 {
		t.Fatal("sweep accepted a URL that does not match config.server host/port")
	}
}

func TestRunExecutesClientAndWritesPointArtifacts(t *testing.T) {
	dir := t.TempDir()
	rc := sweepTestRunContext()
	seed := artifact.SeedOutput{SchemaVersion: artifact.SchemaVersion, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), UserIDSubscription: 1, UserIDCompat: 2, TokenDBKeySubscription: "loadtestsub", TokenDBKeyCompat: "loadtestcompat", ExpectedUsagePerSuccess: artifact.Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}}
	seedHash, err := artifact.HashSeedOutput(seed)
	if err != nil {
		t.Fatal(err)
	}
	rc.SeedOutputHash = seedHash
	seed.RunContext = rc.WithoutSeedOutputHash().WithoutMockHash()
	db := openSweepCommandTestDB(t)
	seedSweepBusinessRows(t, db, seed)
	newAPIServer := httptest.NewServer(newSweepHarnessHandler(rc, db, seed, true))
	defer newAPIServer.Close()

	runContextPath := filepath.Join(dir, "run-context.json")
	seedPath := filepath.Join(dir, "seed.json")
	outPath := filepath.Join(dir, "sweep.json")
	artifactDir := filepath.Join(dir, "artifacts")
	configPath := filepath.Join(dir, "config.yaml")
	writeTestJSON(t, runContextPath, rc)
	writeTestJSON(t, seedPath, seed)
	writeSweepConfig(t, configPath, newAPIServer.URL)
	originalOpenSweepDB := openSweepDB
	openSweepDB = func(string) (*gorm.DB, error) { return db, nil }
	t.Cleanup(func() { openSweepDB = originalOpenSweepDB })

	var stdout, stderr bytes.Buffer
	exit := Run([]string{
		"--config", configPath,
		"--url", newAPIServer.URL,
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
		"--mock-stats", newAPIServer.URL + "/debug/loadtest/mock-stats",
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
	assertPassedInvariant(t, point.Invariants, "subscription_token_used_matches_success_usage")
	assertPassedInvariant(t, point.Invariants, "consume_logs_by_request")
	if point.SummaryExcerpt.StatusCodes["200"] != 3 || point.SummaryExcerpt.StreamUsageEvents != 3 || point.SummaryExcerpt.StreamDoneReceived != 3 {
		t.Fatalf("point did not record required stream/status fields: %#v", point.SummaryExcerpt)
	}
	if got, err := artifact.HashCanonical(artifact.MockStatsDelta{SchemaVersion: point.MockDelta.SchemaVersion, RunContext: point.MockDelta.RunContext, Path: point.MockDelta.Path, Actual429: point.MockDelta.Actual429, Actual502: point.MockDelta.Actual502, UpstreamAttemptsTotal: point.MockDelta.UpstreamAttemptsTotal}); err != nil || got != point.MockDelta.Hash {
		t.Fatalf("mock delta hash mismatch got=%s want=%s err=%v", got, point.MockDelta.Hash, err)
	}
	if point.SummaryPath == "" || point.MetricsDiffPath == "" || point.MockDelta.Path == "" {
		t.Fatalf("missing point artifacts: %#v", point)
	}
	if !strings.Contains(point.SummaryPath, "c2-summary.json") {
		t.Fatalf("summary path should include point concurrency: %s", point.SummaryPath)
	}
}

func TestRunFailsS3WithoutRuntimeResourceSamples(t *testing.T) {
	dir := t.TempDir()
	rc := sweepTestRunContext()
	rc.Scenario = "s3-long-stream"
	seed := artifact.SeedOutput{SchemaVersion: artifact.SchemaVersion, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), UserIDSubscription: 1, UserIDCompat: 2, TokenDBKeySubscription: "loadtestsub", TokenDBKeyCompat: "loadtestcompat", ExpectedUsagePerSuccess: artifact.Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}}
	seedHash, err := artifact.HashSeedOutput(seed)
	if err != nil {
		t.Fatal(err)
	}
	rc.SeedOutputHash = seedHash
	seed.RunContext = rc.WithoutSeedOutputHash().WithoutMockHash()
	db := openSweepCommandTestDB(t)
	seedSweepBusinessRows(t, db, seed)
	newAPIServer := httptest.NewServer(newSweepHarnessHandler(rc, db, seed, false))
	defer newAPIServer.Close()
	runContextPath := filepath.Join(dir, "run-context.json")
	seedPath := filepath.Join(dir, "seed.json")
	outPath := filepath.Join(dir, "sweep.json")
	configPath := filepath.Join(dir, "config.yaml")
	writeTestJSON(t, runContextPath, rc)
	writeTestJSON(t, seedPath, seed)
	writeSweepConfig(t, configPath, newAPIServer.URL)
	originalOpenSweepDB := openSweepDB
	openSweepDB = func(string) (*gorm.DB, error) { return db, nil }
	t.Cleanup(func() { openSweepDB = originalOpenSweepDB })

	exit := Run([]string{
		"--config", configPath,
		"--url", newAPIServer.URL,
		"--api-key", "sk-loadtestsub",
		"--token-profile", "subscription",
		"--path", "/v1/responses",
		"--model", "gpt-5.5",
		"--scenario", "s3-long-stream",
		"--points", "2",
		"--max-requests-per-point", "1",
		"--timeout", "5s",
		"--input-bytes", "8",
		"--output-bytes", "128",
		"--seed-output", seedPath,
		"--mock-stats", newAPIServer.URL + "/debug/loadtest/mock-stats",
		"--run-context", runContextPath,
		"--mock-hash", rc.MockHash,
		"--artifact-dir", filepath.Join(dir, "artifacts"),
		"--out", outPath,
	}, ioDiscard(), ioDiscard())
	if exit == 0 {
		t.Fatal("S3 sweep without runtime resource samples passed")
	}
	var result artifact.SweepResult
	readTestJSON(t, outPath, &result)
	if len(result.Points) == 0 || !containsReason(result.Points[0].Gate.FailedReasons, "resource samples") {
		t.Fatalf("missing resource sample gate failure: %#v", result)
	}
}

func TestRunPointUsesRealBusinessSnapshots(t *testing.T) {
	db := openSweepCommandTestDB(t)
	rc := sweepTestRunContext()
	seed := artifact.SeedOutput{SchemaVersion: artifact.SchemaVersion, RunContext: rc.WithoutSeedOutputHash().WithoutMockHash(), UserIDSubscription: 910001, UserIDCompat: 910002, TokenDBKeySubscription: "loadtestsub", TokenDBKeyCompat: "loadtestcompat", ExpectedUsagePerSuccess: artifact.Usage{PromptTokens: 11, CompletionTokens: 17, TotalTokens: 28}}
	seedHash, err := artifact.HashSeedOutput(seed)
	if err != nil {
		t.Fatal(err)
	}
	rc.SeedOutputHash = seedHash
	seed.RunContext = rc.WithoutSeedOutputHash().WithoutMockHash()
	seedSweepBusinessRows(t, db, seed)
	summary := artifact.Summary{RunContext: rc, Total: 1, Success: 1, Requests: []artifact.RequestRecord{{NewAPIRequestID: "rid-1", ClientRequestID: "client-1", UpstreamRequestID: "upstream-1", StatusCode: 200, Success: true}}}
	before, err := metrics.LoadBusinessSnapshot(db, seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.UserSubscription{}).Where("user_id = ?", seed.UserIDSubscription).Update("token_used", int64(28)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Log{RequestId: "rid-1", Type: model.LogTypeConsume, Quota: 28}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SubscriptionPreConsumeRecord{RequestId: "rid-1", UserId: seed.UserIDSubscription, PreConsumed: 28, Status: "consumed"}).Error; err != nil {
		t.Fatal(err)
	}
	after, err := metrics.LoadBusinessSnapshot(db, seed)
	if err != nil {
		t.Fatal(err)
	}
	logRows, preRows, err := metrics.LoadBusinessRows(db, summary)
	if err != nil {
		t.Fatal(err)
	}
	mock := artifact.MockStatsDelta{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Path: "mock-delta.json", Hash: "sha256:mockdelta", UpstreamAttemptsTotal: 1}
	diff, inv := metrics.BuildDiff(metrics.DiffInputs{Before: artifact.Snapshot{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Business: before}, After: artifact.Snapshot{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Business: after}, Summary: summary, SeedOutput: seed, MockDelta: mock, RunContext: rc, ConsumeLogRows: logRows, PreConsumeRows: preRows, BusinessBefore: before, BusinessAfter: after})
	if inv.Status != "passed" || diff.BusinessDelta.Status != "passed" {
		t.Fatalf("business diff did not pass: inv=%#v diff=%#v", inv, diff.BusinessDelta)
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

func writeSweepConfig(t *testing.T, path string, baseURL string) {
	t.Helper()
	content := `server:
  host: "127.0.0.1"
  port: ` + serverPortForConfig(t, baseURL) + `
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

func serverPortForConfig(t *testing.T, baseURL string) string {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	port := parsed.Port()
	if port == "" {
		t.Fatalf("test server URL has no port: %s", baseURL)
	}
	return port
}

func openSweepCommandTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserSubscription{}, &model.Token{}, &model.Log{}, &model.SubscriptionPreConsumeRecord{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedSweepBusinessRows(t *testing.T, db *gorm.DB, seed artifact.SeedOutput) {
	t.Helper()
	if err := db.Create(&model.User{Id: seed.UserIDSubscription, Username: "sweep-sub", Quota: 1_000_000, AffCode: "sweep-sub"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{Id: seed.UserIDCompat, Username: "sweep-compat", Quota: 1_000_000, AffCode: "sweep-compat"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserSubscription{Id: 910011, UserId: seed.UserIDSubscription, Status: "active", TokenUsed: 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserSubscription{Id: 910012, UserId: seed.UserIDCompat, Status: "active", TokenUsed: 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Token{UserId: seed.UserIDSubscription, Key: seed.TokenDBKeySubscription, RemainQuota: 1_000_000}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Token{UserId: seed.UserIDCompat, Key: seed.TokenDBKeyCompat, RemainQuota: 1_000_000}).Error; err != nil {
		t.Fatal(err)
	}
}

func newSweepHarnessHandler(rc artifact.RunContext, db *gorm.DB, seed artifact.SeedOutput, runtimeSamples bool) http.Handler {
	mockHandler := mockopenai.NewServer(mockopenai.Config{RunContext: rc, OutputBytes: 128, PromptTokens: 11, CompletionTokens: 17, ChunkInterval: time.Millisecond})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/debug/loadtest/runtime" {
			w.Header().Set("Content-Type", "application/json")
			if runtimeSamples {
				_, _ = w.Write([]byte(`{"goroutines":5,"heap_alloc_bytes":2048,"batch_update":{"status":"ok"},"quota_data":{"status":"unavailable"},"perf_metrics":{"status":"unavailable"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"goroutines":0,"heap_alloc_bytes":0}`))
			return
		}
		mockHandler.ServeHTTP(w, r)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			return
		}
		requestID := w.Header().Get("X-Oneapi-Request-Id")
		if requestID == "" {
			return
		}
		usage := seed.ExpectedUsagePerSuccess.TotalTokens
		if err := db.Model(&model.UserSubscription{}).Where("user_id = ?", seed.UserIDSubscription).Update("token_used", gorm.Expr("token_used + ?", usage)).Error; err != nil {
			panic(err)
		}
		if err := db.Create(&model.Log{RequestId: requestID, Type: model.LogTypeConsume, Quota: usage}).Error; err != nil {
			panic(err)
		}
		if err := db.Create(&model.SubscriptionPreConsumeRecord{RequestId: requestID, UserId: seed.UserIDSubscription, PreConsumed: int64(usage), Status: "consumed"}).Error; err != nil {
			panic(err)
		}
	})
}

func assertPassedInvariant(t *testing.T, invariants []artifact.Invariant, name string) {
	t.Helper()
	for _, inv := range invariants {
		if inv.Name != name {
			continue
		}
		if inv.Status != "passed" {
			t.Fatalf("invariant %s = %s (%s)", name, inv.Status, inv.Reason)
		}
		return
	}
	t.Fatalf("missing invariant %s: %#v", name, invariants)
}

func containsReason(reasons []string, needle string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, needle) {
			return true
		}
	}
	return false
}

func ioDiscard() *bytes.Buffer { return &bytes.Buffer{} }

var _ = http.MethodGet
