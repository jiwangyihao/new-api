package controller

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestLoadtestRuntimeRouteDisabledByDefault(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)
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
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)
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
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)
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
	for _, listenAddr := range []string{":13080", "0.0.0.0:13080", "10.0.0.2:13080", "[::]:13080"} {
		r := gin.New()
		RegisterLoadtestRuntimeRoute(r, listenAddr, nil)
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
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)
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

func TestLoadtestRuntimeRouteIncludesGCAndHTTPStats(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	t.Setenv("LOADTEST_PROFILE_BLOCK_RATE", "1000")
	t.Setenv("LOADTEST_PROFILE_MUTEX_FRACTION", "5")
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	r := gin.New()
	stats := NewLoadtestHTTPStats()
	stats.OnAccept()
	stats.OnConnState(http.StateNew)
	stats.OnConnState(http.StateActive)
	stats.OnConnState(http.StateIdle)
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", stats)
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := common.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"gomaxprocs", "gomemlimit_bytes", "gc_count", "last_gc_unix_ms", "pause_total_ns", "http_conn_state", "http_accept_total", "http_active_current"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing %s in %s", key, w.Body.String())
		}
	}
	if got := body["http_accept_total"]; got != float64(1) {
		t.Fatalf("http_accept_total = %v", got)
	}
	if got := body["http_active_current"]; got != float64(0) {
		t.Fatalf("http_active_current = %v", got)
	}
	connState, ok := body["http_conn_state"].(map[string]any)
	if !ok {
		t.Fatalf("http_conn_state has type %T", body["http_conn_state"])
	}
	for key, want := range map[string]float64{"new": 1, "active": 1, "idle": 1, "hijacked": 0, "closed": 0} {
		if got := connState[key]; got != want {
			t.Fatalf("http_conn_state[%s] = %v want %v", key, got, want)
		}
	}
}

func TestLoadtestHTTPStatsSnapshotCountsAndClampsActiveCurrent(t *testing.T) {
	stats := NewLoadtestHTTPStats()
	stats.OnConnState(http.StateIdle)
	stats.OnAccept()
	stats.OnConnState(http.StateNew)
	stats.OnConnState(http.StateActive)
	stats.OnConnState(http.StateHijacked)
	stats.OnConnState(http.StateClosed)

	connState, acceptTotal, activeCurrent := stats.Snapshot()
	if acceptTotal != 1 {
		t.Fatalf("accept total = %d", acceptTotal)
	}
	if activeCurrent != 0 {
		t.Fatalf("active current = %d", activeCurrent)
	}
	for key, want := range map[string]int64{"new": 1, "active": 1, "idle": 1, "hijacked": 1, "closed": 1} {
		if got := connState[key]; got != want {
			t.Fatalf("conn state %s = %d want %d", key, got, want)
		}
	}
}

func TestLoadtestCountingListenerCountsSuccessfulAcceptOnly(t *testing.T) {
	stats := NewLoadtestHTTPStats()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	listener := NewLoadtestCountingListener(&fakeLoadtestListener{conn: serverConn}, stats)
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if accepted != serverConn {
		t.Fatalf("accepted connection mismatch")
	}
	_, acceptTotal, _ := stats.Snapshot()
	if acceptTotal != 1 {
		t.Fatalf("accept total = %d", acceptTotal)
	}

	acceptErr := errors.New("accept failed")
	failingListener := NewLoadtestCountingListener(&fakeLoadtestListener{err: acceptErr}, stats)
	accepted, err = failingListener.Accept()
	if !errors.Is(err, acceptErr) || accepted != nil {
		t.Fatalf("Accept() = (%v, %v)", accepted, err)
	}
	_, acceptTotal, _ = stats.Snapshot()
	if acceptTotal != 1 {
		t.Fatalf("failed accept changed total to %d", acceptTotal)
	}

	passthrough := &fakeLoadtestListener{}
	if NewLoadtestCountingListener(passthrough, nil) != passthrough {
		t.Fatalf("nil stats should return the inner listener")
	}
}

type fakeLoadtestListener struct {
	conn net.Conn
	err  error
}

func (l *fakeLoadtestListener) Accept() (net.Conn, error) {
	if l.err != nil {
		return nil, l.err
	}
	return l.conn, nil
}

func (l *fakeLoadtestListener) Close() error {
	return nil
}

func (l *fakeLoadtestListener) Addr() net.Addr {
	return fakeLoadtestAddr("loadtest")
}

type fakeLoadtestAddr string

func (a fakeLoadtestAddr) Network() string {
	return string(a)
}

func (a fakeLoadtestAddr) String() string {
	return string(a)
}
