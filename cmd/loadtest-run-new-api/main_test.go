package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStatusURLFromEnvRejectsUnsafeHosts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		host string
	}{
		{name: "empty", host: ""},
		{name: "wildcard", host: "0.0.0.0"},
		{name: "non loopback", host: "192.168.1.10"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := statusURLFromEnv(map[string]string{"HOST": tc.host, "PORT": "13080"})
			if err == nil {
				t.Fatalf("statusURLFromEnv accepted HOST %q", tc.host)
			}
		})
	}
}

func TestStatusURLFromEnvAcceptsLoopback(t *testing.T) {
	t.Parallel()

	got, err := statusURLFromEnv(map[string]string{"HOST": "127.0.0.1", "PORT": "13080"})
	if err != nil {
		t.Fatalf("statusURLFromEnv rejected loopback: %v", err)
	}
	if want := "http://127.0.0.1:13080/api/status"; got != want {
		t.Fatalf("statusURLFromEnv() = %q, want %q", got, want)
	}
}

func TestWaitForStatusSucceedsOnHTTP2xx(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := waitForStatus(ctx, server.Client(), server.URL+"/api/status", nil); err != nil {
		t.Fatalf("waitForStatus returned error: %v", err)
	}
}

func TestWaitForStatusFailsOnHTTP500(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	err := waitForStatus(ctx, server.Client(), server.URL+"/api/status", nil)
	if err == nil {
		t.Fatal("waitForStatus succeeded on HTTP 500")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("waitForStatus error = %q, want HTTP 500 context", err.Error())
	}
}

func TestWaitForStatusFailsOnUnreachableEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	statusURL := server.URL + "/api/status"
	server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	if err := waitForStatus(ctx, server.Client(), statusURL, nil); err == nil {
		t.Fatal("waitForStatus succeeded on unreachable endpoint")
	}
}

func TestWaitForStatusFailsWhenProcessExits(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	processDone := func() error { return context.Canceled }

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := waitForStatus(ctx, server.Client(), server.URL+"/api/status", processDone)
	if err == nil {
		t.Fatal("waitForStatus succeeded after process exit")
	}
	if !strings.Contains(err.Error(), "exited before /api/status") {
		t.Fatalf("waitForStatus error = %q, want process exit context", err.Error())
	}
}
