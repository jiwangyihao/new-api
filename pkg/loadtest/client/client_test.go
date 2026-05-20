package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseResponsesSSEExtractsUsageAndDone(t *testing.T) {
	body := strings.NewReader("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":17,\"total_tokens\":28}}}\n\ndata: [DONE]\n\n")
	rec, err := ParseResponsesStream(body)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.DoneReceived || !rec.CompletedEventReceived || !rec.FirstTokenReceived || rec.Usage.PromptTokens != 11 || rec.Usage.CompletionTokens != 17 || rec.Usage.TotalTokens != 28 {
		t.Fatalf("bad record: %#v", rec)
	}
}

func TestTokenProfileMustMatchAPIKey(t *testing.T) {
	if err := ValidateTokenProfile("sk-loadtestsub", "subscription"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTokenProfile("sk-loadtestcompat", "compat"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTokenProfile("sk-loadtestinvalid", "invalid"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTokenProfile("sk-loadtestsub", "compat"); err == nil {
		t.Fatal("mismatch accepted")
	}
	if err := ValidateTokenProfile("sk-real", "subscription"); err == nil {
		t.Fatal("unknown key accepted")
	}
}

func TestSummaryUsesRequestIDHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-loadtestsub" {
			t.Fatalf("bad authorization header: %q", got)
		}
		w.Header().Set("X-Oneapi-Request-Id", "rid-new-api")
		w.Header().Set("X-Upstream-Request-Id", "upstream-loadtest-1")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":17,\"total_tokens\":28}}}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	summary, err := RunOnceForTest(srv.URL, "sk-loadtestsub", "subscription")
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Requests) != 1 {
		t.Fatalf("request count = %d", len(summary.Requests))
	}
	rec := summary.Requests[0]
	if rec.NewAPIRequestID != "rid-new-api" || rec.UpstreamRequestID != "upstream-loadtest-1" || rec.StatusCode != http.StatusOK || !rec.Success {
		t.Fatalf("bad request record: %#v", rec)
	}
	if rec.PromptTokens != 11 || rec.CompletionTokens != 17 || rec.TotalTokens != 28 {
		t.Fatalf("bad usage: %#v", rec)
	}
}

func TestErrorSummaryRecordsRequestIDStatusAndReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Oneapi-Request-Id", "rid-error")
		w.Header().Set("X-Upstream-Request-Id", "upstream-error")
		http.Error(w, "loadtest injected upstream failure", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	summary, err := RunOnceForTest(srv.URL, "sk-loadtestsub", "subscription")
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Requests) != 1 {
		t.Fatalf("request count = %d", len(summary.Requests))
	}
	rec := summary.Requests[0]
	if rec.NewAPIRequestID != "rid-error" || rec.UpstreamRequestID != "upstream-error" || rec.StatusCode != http.StatusTooManyRequests || rec.ErrorReason != "status_non_2xx" || rec.Success {
		t.Fatalf("bad error record: %#v", rec)
	}
}

func TestHealthCheckCoversStatusRuntimePprofModelsAndInvalidToken(t *testing.T) {
	srv := newHealthCheckTestServer(t)
	result, err := HealthCheck(context.Background(), HealthCheckOptions{
		BaseURL:       srv.URL,
		ValidAPIKey:   "sk-loadtestsub",
		InvalidAPIKey: "sk-loadtestinvalid",
		RuntimeURL:    srv.URL + "/debug/loadtest/runtime",
		PprofURL:      srv.URL + "/debug/pprof/goroutine?debug=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("health check failed: %#v", result)
	}
	for _, check := range []string{"api_status", "runtime_stats", "pprof_goroutine", "models_valid_token", "invalid_token_rejected"} {
		if result.Checks[check].Status != "passed" {
			t.Fatalf("%s not passed: %#v", check, result.Checks[check])
		}
	}
}

func TestRejectsNonLoopbackURL(t *testing.T) {
	if err := ValidateLoopbackURL("https://api.example.com"); err == nil {
		t.Fatal("non-loopback URL accepted")
	}
	if err := ValidateLoopbackURL("http://127.0.0.1:13080"); err != nil {
		t.Fatal(err)
	}
}

func newHealthCheckTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/debug/loadtest/runtime":
			_, _ = w.Write([]byte(`{"goroutines":3,"heap_alloc_bytes":1024,"batch_update":{"status":"ok"},"quota_data":{"status":"unavailable"},"perf_metrics":{"status":"unavailable"}}`))
		case "/debug/pprof/goroutine":
			_, _ = w.Write([]byte("goroutine profile: total 3\n"))
		case "/v1/models":
			switch r.Header.Get("Authorization") {
			case "Bearer sk-loadtestsub":
				_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			case "Bearer sk-loadtestinvalid":
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			default:
				http.Error(w, "forbidden", http.StatusForbidden)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
