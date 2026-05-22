package sweep

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/monitor"
)

func TestRunPointSamplesConfiguredProcessPID(t *testing.T) {
	dir := t.TempDir()
	rc := testRunContext()
	statsPath := filepath.Join(dir, "mock-stats.json")
	writeRunPointJSON(t, statsPath, artifact.MockStats{SchemaVersion: artifact.SchemaVersion, RunContext: rc})
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"goroutines":3,"heap_alloc_bytes":1024,"heap_sys_bytes":2048,"batch_update":{"status":"ok"},"quota_data":{"status":"unavailable"},"perf_metrics":{"status":"unavailable"}}`))
	}))
	defer runtimeServer.Close()
	loadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":0,\"total_tokens\":0}}}\n\ndata: [DONE]\n\n"))
	}))
	defer loadServer.Close()
	pid := os.Getpid()

	_, _, samples, err := RunPoint(context.Background(), RunPointOptions{
		Concurrency:      1,
		BaseURL:          loadServer.URL,
		RuntimeURL:       runtimeServer.URL,
		APIKey:           loadtestconfig.SubscriptionAPIKey,
		TokenProfile:     "subscription",
		Path:             "/v1/responses",
		Model:            "gpt-5.5",
		Scenario:         "benchmark",
		ArtifactDir:      dir,
		RunContext:       rc,
		Config:           &loadtestconfig.File{Redis: loadtestconfig.RedisConfig{Addr: "redis://127.0.0.1:16379/0"}},
		MockProfile:      "s2-short-stream",
		MockStats:        statsPath,
		RequestsPerPoint: 1,
		MaxRequests:      1,
		Duration:         time.Second,
		Timeout:          50 * time.Millisecond,
		Transport:        artifact.TransportProfile{Mode: "h1_no_keepalive", MaxConnsPerHost: 1, MaxIdleConns: 1, MaxIdleConnsPerHost: 1},
		Seed:             artifact.SeedOutput{SchemaVersion: artifact.SchemaVersion, ExpectedUsagePerSuccess: artifact.Usage{TotalTokens: 0}},
		ServerPID:        pid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(samples.Samples) == 0 {
		t.Fatal("no resource samples collected")
	}
	if samples.Samples[0].Process.PID != pid || samples.Peaks.RSSPeakBytes == 0 {
		t.Fatalf("process sampler did not use server pid %d: %#v peaks=%#v", pid, samples.Samples[0].Process, samples.Peaks)
	}
}

func TestBusinessSnapshotAfterDrainUsesDrainSampleTokenUsed(t *testing.T) {
	fallback := artifact.BusinessSnapshot{Statused: artifact.Statused{Status: "ok"}, SubscriptionTokenUsed: 20, CompatSubscriptionTokenUsed: 30}

	subscription := businessSnapshotAfterDrain(RunPointOptions{TokenProfile: "subscription"}, monitor.DrainSample{SubscriptionTokenUsed: 280}, fallback)
	if subscription.SubscriptionTokenUsed != 280 || subscription.CompatSubscriptionTokenUsed != 30 {
		t.Fatalf("subscription snapshot did not use drain sample: %#v", subscription)
	}

	compat := businessSnapshotAfterDrain(RunPointOptions{TokenProfile: "compat"}, monitor.DrainSample{SubscriptionTokenUsed: 560}, fallback)
	if compat.CompatSubscriptionTokenUsed != 560 || compat.SubscriptionTokenUsed != 20 {
		t.Fatalf("compat snapshot did not use drain sample: %#v", compat)
	}
}

func writeRunPointJSON(t *testing.T, path string, value any) {
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
