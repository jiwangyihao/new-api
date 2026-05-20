package controller

import (
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type runtimeStatus struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type loadtestRuntimeResponse struct {
	Goroutines           int           `json:"goroutines"`
	HeapAllocBytes       uint64        `json:"heap_alloc_bytes"`
	BlockProfileRate     int           `json:"block_profile_rate"`
	MutexProfileFraction int           `json:"mutex_profile_fraction"`
	BatchUpdate          runtimeStatus `json:"batch_update"`
	QuotaData            runtimeStatus `json:"quota_data"`
	PerfMetrics          runtimeStatus `json:"perf_metrics"`
	Unavailable          []string      `json:"unavailable,omitempty"`
}

func RegisterLoadtestRuntimeRoute(r *gin.Engine, listenAddr string) {
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
		batch := model.BatchUpdatePendingSnapshot()
		resp := loadtestRuntimeResponse{
			Goroutines:           runtime.NumGoroutine(),
			HeapAllocBytes:       mem.HeapAlloc,
			BlockProfileRate:     parseEnvInt("LOADTEST_PROFILE_BLOCK_RATE"),
			MutexProfileFraction: parseEnvInt("LOADTEST_PROFILE_MUTEX_FRACTION"),
			BatchUpdate:          runtimeStatus{Status: "ok", Reason: batch.String()},
			QuotaData:            runtimeStatus{Status: "unavailable", Reason: "quota data pending snapshot is not exposed"},
			PerfMetrics:          runtimeStatus{Status: "unavailable", Reason: "perf metrics pending snapshot is not exposed"},
			Unavailable:          []string{"quota_data", "perf_metrics"},
		}
		c.JSON(http.StatusOK, resp)
	})
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
		first, _, _ := strings.Cut(value, ",")
		if !hostIsLoopback(strings.TrimSpace(first)) {
			return false
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
