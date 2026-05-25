package mockopenai

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

func testRunContext() artifact.RunContext {
	return artifact.RunContext{SchemaVersion: 1, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "s2-short-stream", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
}

func TestResponsesSSEContractAndUsage(t *testing.T) {
	srv := NewServer(Config{RunContext: testRunContext(), FirstTokenDelay: time.Millisecond, StreamDuration: 10 * time.Millisecond, ChunkInterval: time.Millisecond, OutputBytes: 12, PromptTokens: 11, CompletionTokens: 17})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","stream":true,"input":"hello"}`))
	srv.ServeHTTP(rr, req)
	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, body)
	}
	if rr.Header().Get("X-Oneapi-Request-Id") != "upstream-loadtest-1" {
		t.Fatalf("missing upstream id")
	}
	for _, want := range []string{"event: response.created", "event: response.output_text.delta", "event: response.completed", "\"input_tokens\":11", "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	stats := srv.(*Server).Snapshot()
	if stats.AttemptsTotal != 1 || len(stats.Attempts) != 1 || stats.Attempts[0].UpstreamRequestID != "upstream-loadtest-1" {
		t.Fatalf("bad stats: %#v", stats)
	}
}

func TestResponsesDoesNotWriteStatsFilePerRequest(t *testing.T) {
	statsPath := filepath.Join(t.TempDir(), "mock-stats.json")
	srv := NewServer(Config{RunContext: testRunContext(), StatsOut: statsPath, OutputBytes: 1})
	if err := srv.(*Server).WriteStats(); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(statsPath)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","stream":true,"input":"hello"}`))
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	after, err := os.Stat(statsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("stats file changed during request: before size=%d mod=%s after size=%d mod=%s", before.Size(), before.ModTime(), after.Size(), after.ModTime())
	}
	stats := srv.(*Server).Snapshot()
	if stats.AttemptsTotal != 1 {
		t.Fatalf("in-memory stats not updated: %#v", stats)
	}
}

func TestChatCompletionsSSEContractAndErrorInjection(t *testing.T) {
	srv := NewServer(Config{RunContext: testRunContext(), StatusRate: map[int]float64{429: 1}, Seed: 1, PromptTokens: 11, CompletionTokens: 17})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","stream":true,"messages":[{"role":"user","content":"hello"}]}`))
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("X-Oneapi-Request-Id") == "" {
		t.Fatal("missing upstream id on error")
	}
	stats := srv.(*Server).Snapshot()
	if stats.AttemptsTotal != 1 || stats.InjectedStatusCounts["429"] != 1 {
		t.Fatalf("bad injected stats: %#v", stats)
	}
}

func TestChatCompletionsSSEContractAndUsage(t *testing.T) {
	srv := NewServer(Config{RunContext: testRunContext(), FirstTokenDelay: time.Millisecond, StreamDuration: 10 * time.Millisecond, ChunkInterval: time.Millisecond, OutputBytes: 9, PromptTokens: 11, CompletionTokens: 17})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hello"}]}`))
	srv.ServeHTTP(rr, req)
	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, body)
	}
	if rr.Header().Get("X-Oneapi-Request-Id") != "upstream-loadtest-1" {
		t.Fatalf("missing upstream id")
	}
	for _, want := range []string{"\"object\":\"chat.completion.chunk\"", "\"role\":\"assistant\"", "\"content\":", "\"finish_reason\":\"stop\"", "\"prompt_tokens\":11", "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestModelsNotAffectedByStatusRate(t *testing.T) {
	srv := NewServer(Config{RunContext: testRunContext(), StatusRate: map[int]float64{429: 1}, Seed: 1})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	srv.ServeHTTP(rr, req)
	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, body)
	}
	if !strings.Contains(body, "gpt-5.5") {
		t.Fatalf("missing model in %s", body)
	}
	stats := srv.(*Server).Snapshot()
	if stats.AttemptsTotal != 0 {
		t.Fatalf("models request counted as main attempt: %#v", stats)
	}
}

func TestDebugStatsEndpointLoopbackOnly(t *testing.T) {
	srv := NewServer(Config{RunContext: testRunContext(), PromptTokens: 11, CompletionTokens: 17})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","stream":true,"input":"hello"}`))
	srv.ServeHTTP(httptest.NewRecorder(), req)

	loopback := httptest.NewRecorder()
	loopbackReq := httptest.NewRequest(http.MethodGet, "/debug/loadtest/mock-stats", nil)
	loopbackReq.RemoteAddr = "127.0.0.1:12345"
	srv.ServeHTTP(loopback, loopbackReq)
	if loopback.Code != http.StatusOK || !strings.Contains(loopback.Body.String(), "run_context") || !strings.Contains(loopback.Body.String(), "attempts_total") {
		t.Fatalf("bad stats response status=%d body=%s", loopback.Code, loopback.Body.String())
	}

	remoteReq := httptest.NewRequest(http.MethodGet, "/debug/loadtest/mock-stats", nil)
	remoteReq.RemoteAddr = "10.0.0.2:12345"
	remoteReq.Header.Set("X-Forwarded-For", "10.0.0.2")
	remote := httptest.NewRecorder()
	srv.ServeHTTP(remote, remoteReq)
	if remote.Code != http.StatusForbidden {
		t.Fatalf("remote stats status = %d body=%s", remote.Code, remote.Body.String())
	}
}

func TestMockStatsIncludesRunContext(t *testing.T) {
	stats := MockStatsForTest(testRunContext())
	b, err := artifact.MarshalCanonical(stats)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "run_context") {
		t.Fatalf("missing run_context: %s", b)
	}
}
