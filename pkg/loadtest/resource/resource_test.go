package resource

import (
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/profile"
)

func TestBuildLimitsArtifactRecordsServerOnlyScope(t *testing.T) {
	limits := profile.Benchmark().ServerLimits
	result := ApplyResult{
		Status:                  "applied",
		Reason:                  "test",
		MemoryLimitEnforced:     true,
		CPUAffinityEnforced:     true,
		CPUAffinityMask:         3,
		ProcessMemoryLimitBytes: limits.ProcessMemoryLimitBytes,
		CPUAffinityCores:        limits.CPUAffinityCores,
	}

	got := BuildLimitsArtifact(testRunContext(), limits, result)
	if got.TargetProcess != "server" {
		t.Fatalf("TargetProcess = %q, want server", got.TargetProcess)
	}
	for _, phrase := range []string{"new-api server process only", "load generator", "mock upstream", "PostgreSQL", "Redis", "orchestrator remain uncapped"} {
		if !strings.Contains(got.Scope, phrase) {
			t.Fatalf("scope %q does not contain %q", got.Scope, phrase)
		}
	}
	if !got.OSProcessMemoryLimitEnforced || !got.OSCPUAffinityEnforced {
		t.Fatalf("enforced flags not recorded: %#v", got)
	}
	if got.ServerProcessMemoryLimitBytes != 512*1024*1024 {
		t.Fatalf("ServerProcessMemoryLimitBytes = %d", got.ServerProcessMemoryLimitBytes)
	}
	if got.ServerCPUAffinityCores != 2 || got.ServerCPUAffinityMask != 3 {
		t.Fatalf("CPU fields not recorded: cores=%d mask=%d", got.ServerCPUAffinityCores, got.ServerCPUAffinityMask)
	}
	for key, want := range map[string]string{
		"GOMAXPROCS":                    "2",
		"GOGC":                          "100",
		"GOMEMLIMIT":                    "384MiB",
		"SQL_MAX_OPEN_CONNS":            "64",
		"SQL_MAX_IDLE_CONNS":            "64",
		"REDIS_POOL_SIZE":               "256",
		"REDIS_IDLE_TIMEOUT_SECONDS":    "1",
		"RELAY_MAX_IDLE_CONNS":          "1024",
		"RELAY_MAX_IDLE_CONNS_PER_HOST": "1024",
	} {
		if got.ServerEnv[key] != want {
			t.Fatalf("server env %s = %q, want %q in %#v", key, got.ServerEnv[key], want, got.ServerEnv)
		}
	}
	if got.Status != "applied" || got.Reason != "test" {
		t.Fatalf("status mismatch: %#v", got.Statused)
	}
}

func TestPortsClosedDetectsOpenAndClosedLoopbackPorts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	openPort := ln.Addr().(*net.TCPAddr).Port
	closedPort := mustClosedPort(t)
	got := CheckPortsClosed(testRunContext(), []int{openPort, closedPort})
	if got.Passed {
		t.Fatalf("Passed = true with open port: %#v", got.Ports)
	}
	if got.Ports[strconv.Itoa(openPort)] != "open" {
		t.Fatalf("open port %d reported %q", openPort, got.Ports[strconv.Itoa(openPort)])
	}
	if got.Ports[strconv.Itoa(closedPort)] != "closed" {
		t.Fatalf("closed port %d reported %q", closedPort, got.Ports[strconv.Itoa(closedPort)])
	}
}

func TestPortsFromConfigIncludesAllLoadtestPorts(t *testing.T) {
	cfg := loadtestconfig.File{
		Server:       loadtestconfig.ServerConfig{Host: "127.0.0.1", Port: 13080, PprofAddr: "127.0.0.1:8005"},
		Postgres:     loadtestconfig.PostgresConfig{DSN: "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable"},
		Redis:        loadtestconfig.RedisConfig{Addr: "redis://127.0.0.1:16379/0"},
		MockUpstream: loadtestconfig.MockUpstreamConfig{BaseURL: "http://127.0.0.1:19080"},
	}

	got, err := PortsFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sort.Ints(got)
	want := []int{15432, 16379, 13080, 19080, 8005}
	sort.Ints(want)
	if len(got) != len(want) {
		t.Fatalf("ports = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ports = %v, want %v", got, want)
		}
	}
}

func TestPortsFromConfigRejectsDefaultInfraPorts(t *testing.T) {
	cfg := loadtestconfig.File{
		Server:       loadtestconfig.ServerConfig{Host: "127.0.0.1", Port: 13080, PprofAddr: "127.0.0.1:8005"},
		Postgres:     loadtestconfig.PostgresConfig{DSN: "postgresql://new_api_loadtest:loadtest@127.0.0.1:5432/new_api_loadtest?sslmode=disable"},
		Redis:        loadtestconfig.RedisConfig{Addr: "redis://127.0.0.1:16379/0"},
		MockUpstream: loadtestconfig.MockUpstreamConfig{BaseURL: "http://127.0.0.1:19080"},
	}
	if _, err := PortsFromConfig(cfg); err == nil {
		t.Fatal("default PostgreSQL port accepted")
	}
	cfg.Postgres.DSN = "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable"
	cfg.Redis.Addr = "redis://127.0.0.1:6379/0"
	if _, err := PortsFromConfig(cfg); err == nil {
		t.Fatal("default Redis port accepted")
	}
}

func TestSampleProcessStatusBoundaries(t *testing.T) {
	invalid := SampleProcess(0)
	if invalid.Status != "unavailable" {
		t.Fatalf("invalid status = %#v", invalid.Statused)
	}
	current := SampleProcess(os.Getpid())
	if current.Status != "ok" {
		t.Fatalf("current process status = %#v", current.Statused)
	}
	if current.PID != os.Getpid() || current.RSSBytes == 0 || current.ThreadCount == 0 {
		t.Fatalf("current process snapshot = %#v", current)
	}
}

func TestApplyServerLimitsInvalidPIDFailsClosed(t *testing.T) {
	result, err := ApplyServerLimits(0, profile.Benchmark().ServerLimits)
	if runtime.GOOS == "windows" {
		if err == nil || result.Status != "unavailable" {
			t.Fatalf("windows invalid pid result=%#v err=%v", result, err)
		}
		return
	}
	if err != nil || result.Status != "best_effort" || result.MemoryLimitEnforced || result.CPUAffinityEnforced {
		t.Fatalf("non-windows best-effort result=%#v err=%v", result, err)
	}
}

func testRunContext() artifact.RunContext {
	return artifact.RunContext{SchemaVersion: artifact.SchemaVersion, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", CacheMode: "cold-fresh-role,warm-per-point", Model: "gpt-5.5"}
}

func mustClosedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}
