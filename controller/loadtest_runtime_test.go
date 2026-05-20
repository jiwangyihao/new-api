package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoadtestRuntimeRouteDisabledByDefault(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestLoadtestRuntimeRouteRequiresLoopback(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestLoadtestRuntimeRouteRejectsForwardedNonLoopbackClient(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.8")
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestLoadtestRuntimeRouteNotRegisteredWhenServerNotLoopback(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	for _, listenAddr := range []string{":13080", "0.0.0.0:13080", "10.0.0.2:13080"} {
		r := gin.New()
		RegisterLoadtestRuntimeRoute(r, listenAddr)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("listen addr %q status = %d", listenAddr, rr.Code)
		}
	}
}

func TestLoadtestRuntimeRouteReturnsRuntimeFields(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	t.Setenv("LOADTEST_PROFILE_BLOCK_RATE", "1000")
	t.Setenv("LOADTEST_PROFILE_MUTEX_FRACTION", "5")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, want := range []string{"goroutines", "heap_alloc_bytes", "block_profile_rate", "mutex_profile_fraction", "batch_update", "quota_data", "perf_metrics", "unavailable"} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("missing %s in %s", want, rr.Body.String())
		}
	}
}
