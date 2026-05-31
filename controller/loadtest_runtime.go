package controller

import (
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type runtimeStatus struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

const (
	loadtestHTTPConnStateNew      = "new"
	loadtestHTTPConnStateActive   = "active"
	loadtestHTTPConnStateIdle     = "idle"
	loadtestHTTPConnStateHijacked = "hijacked"
	loadtestHTTPConnStateClosed   = "closed"
)

type LoadtestHTTPStats struct {
	acceptTotal   atomic.Uint64
	stateNew      atomic.Int64
	stateActive   atomic.Int64
	stateIdle     atomic.Int64
	stateHijack   atomic.Int64
	stateClosed   atomic.Int64
	activeCurrent atomic.Int64
}

func NewLoadtestHTTPStats() *LoadtestHTTPStats {
	return &LoadtestHTTPStats{}
}

func (s *LoadtestHTTPStats) OnAccept() {
	if s == nil {
		return
	}
	s.acceptTotal.Add(1)
}

func (s *LoadtestHTTPStats) OnConnState(state http.ConnState) {
	if s == nil {
		return
	}
	switch state {
	case http.StateNew:
		s.stateNew.Add(1)
	case http.StateActive:
		s.stateActive.Add(1)
		s.activeCurrent.Add(1)
	case http.StateIdle:
		s.stateIdle.Add(1)
		s.decrementActiveCurrent()
	case http.StateHijacked:
		s.stateHijack.Add(1)
		s.decrementActiveCurrent()
	case http.StateClosed:
		s.stateClosed.Add(1)
		s.decrementActiveCurrent()
	}
}

func (s *LoadtestHTTPStats) Snapshot() (map[string]int64, uint64, int64) {
	if s == nil {
		return loadtestHTTPConnStateSnapshot(0, 0, 0, 0, 0), 0, 0
	}
	return loadtestHTTPConnStateSnapshot(
		s.stateNew.Load(),
		s.stateActive.Load(),
		s.stateIdle.Load(),
		s.stateHijack.Load(),
		s.stateClosed.Load(),
	), s.acceptTotal.Load(), s.activeCurrent.Load()
}

func (s *LoadtestHTTPStats) decrementActiveCurrent() {
	for {
		current := s.activeCurrent.Load()
		if current <= 0 {
			return
		}
		if s.activeCurrent.CompareAndSwap(current, current-1) {
			return
		}
	}
}

func loadtestHTTPConnStateSnapshot(newCount, active, idle, hijacked, closed int64) map[string]int64 {
	return map[string]int64{
		loadtestHTTPConnStateNew:      newCount,
		loadtestHTTPConnStateActive:   active,
		loadtestHTTPConnStateIdle:     idle,
		loadtestHTTPConnStateHijacked: hijacked,
		loadtestHTTPConnStateClosed:   closed,
	}
}

type loadtestCountingListener struct {
	net.Listener
	stats *LoadtestHTTPStats
}

func NewLoadtestCountingListener(inner net.Listener, stats *LoadtestHTTPStats) net.Listener {
	if stats == nil {
		return inner
	}
	return &loadtestCountingListener{Listener: inner, stats: stats}
}

func (l *loadtestCountingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.stats.OnAccept()
	return conn, nil
}

type loadtestRuntimeResponse struct {
	Goroutines           int              `json:"goroutines"`
	HeapAllocBytes       uint64           `json:"heap_alloc_bytes"`
	GOMAXPROCS           int              `json:"gomaxprocs"`
	GOMEMLimitBytes      int64            `json:"gomemlimit_bytes"`
	GCCount              uint32           `json:"gc_count"`
	LastGCUnixMS         uint64           `json:"last_gc_unix_ms"`
	PauseTotalNS         uint64           `json:"pause_total_ns"`
	HTTPConnState        map[string]int64 `json:"http_conn_state"`
	HTTPAcceptTotal      uint64           `json:"http_accept_total"`
	HTTPActiveCurrent    int64            `json:"http_active_current"`
	BlockProfileRate     int              `json:"block_profile_rate"`
	MutexProfileFraction int              `json:"mutex_profile_fraction"`
	BatchUpdate          runtimeStatus    `json:"batch_update"`
	QuotaData            runtimeStatus    `json:"quota_data"`
	PerfMetrics          runtimeStatus    `json:"perf_metrics"`
	Unavailable          []string         `json:"unavailable,omitempty"`
}

func RegisterLoadtestRuntimeRoute(r *gin.Engine, listenAddr string, stats *LoadtestHTTPStats) {
	if os.Getenv("LOADTEST_RUNTIME_STATS_ENABLED") != "true" || !listenAddrIsLoopback(listenAddr) {
		return
	}
	applyLoadtestProfileRates()
	r.GET("/debug/loadtest/runtime", func(c *gin.Context) {
		if !remoteAddrIsLoopback(c.Request.RemoteAddr) || !forwardedClientIsLoopback(c.Request) {
			c.Status(http.StatusForbidden)
			return
		}
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		httpConnState, httpAcceptTotal, httpActiveCurrent := stats.Snapshot()
		batch := model.BatchUpdatePendingSnapshot()
		resp := loadtestRuntimeResponse{
			Goroutines:           runtime.NumGoroutine(),
			HeapAllocBytes:       mem.HeapAlloc,
			GOMAXPROCS:           runtime.GOMAXPROCS(0),
			GOMEMLimitBytes:      debug.SetMemoryLimit(-1),
			GCCount:              mem.NumGC,
			LastGCUnixMS:         mem.LastGC / uint64(time.Millisecond),
			PauseTotalNS:         mem.PauseTotalNs,
			HTTPConnState:        httpConnState,
			HTTPAcceptTotal:      httpAcceptTotal,
			HTTPActiveCurrent:    httpActiveCurrent,
			BlockProfileRate:     parseEnvInt("LOADTEST_PROFILE_BLOCK_RATE"),
			MutexProfileFraction: parseEnvInt("LOADTEST_PROFILE_MUTEX_FRACTION"),
			BatchUpdate:          runtimeStatus{Status: "ok", Reason: batch.String()},
			QuotaData:            runtimeStatus{Status: "unavailable", Reason: "quota data pending snapshot is not exposed"},
			PerfMetrics:          runtimeStatus{Status: "unavailable", Reason: "perf metrics pending snapshot is not exposed"},
			Unavailable:          []string{"quota_data", "perf_metrics"},
		}
		c.JSON(http.StatusOK, resp)
	})
	r.POST("/debug/loadtest/runtime/batch-update/user-quota/drain", func(c *gin.Context) {
		if !remoteAddrIsLoopback(c.Request.RemoteAddr) || !forwardedClientIsLoopback(c.Request) {
			c.Status(http.StatusForbidden)
			return
		}
		before := model.BatchUpdatePendingSnapshot()
		err := model.FlushBatchUpdateTypeForMigration(model.BatchUpdateTypeUserQuota)
		after := model.BatchUpdatePendingSnapshot()
		pending := after.ByType[model.BatchUpdateTypeUserQuota]
		status := http.StatusOK
		if err != nil || pending != 0 {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{
			"flushed_type": "user_quota",
			"before":       before,
			"after":        after,
			"pending":      pending,
			"error":        errorString(err),
			"note":         "Stop ingress before calling drain; this endpoint only flushes this local instance.",
		})
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func applyLoadtestProfileRates() {
	if v := parseEnvInt("LOADTEST_PROFILE_BLOCK_RATE"); v > 0 {
		runtime.SetBlockProfileRate(v)
	}
	if v := parseEnvInt("LOADTEST_PROFILE_MUTEX_FRACTION"); v > 0 {
		runtime.SetMutexProfileFraction(v)
	}
}

func parseEnvInt(key string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	return v
}

func listenAddrIsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return hostIsLoopback(host)
}

func forwardedClientIsLoopback(r *http.Request) bool {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		for _, part := range strings.Split(value, ",") {
			if !hostIsLoopback(strings.TrimSpace(part)) {
				return false
			}
		}
	}
	return true
}

func remoteAddrIsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return hostIsLoopback(host)
}

func hostIsLoopback(host string) bool {
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
