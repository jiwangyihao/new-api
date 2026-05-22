package monitor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

const redisInfoFixture = "# Clients\r\nconnected_clients:7\r\n# Memory\r\nused_memory:1048576\r\nused_memory_rss:2097152\r\nmem_fragmentation_ratio:1.25\r\nignored_bad_int:not-a-number\r\n# Stats\r\ninstantaneous_ops_per_sec:42\r\ntotal_commands_processed:12345\r\n# Keyspace\r\ndb0:keys=11,expires=0,avg_ttl=0\r\ndb2:keys=3,expires=1,avg_ttl=5\r\n"

func TestParseRedisInfoExtractsMemoryClientsAndCommands(t *testing.T) {
	snap := ParseRedisInfo(redisInfoFixture)
	if snap.Status != "ok" {
		t.Fatalf("status = %q reason=%q", snap.Status, snap.Reason)
	}
	if snap.ConnectedClients != 7 {
		t.Fatalf("connected clients = %d", snap.ConnectedClients)
	}
	if snap.UsedMemoryBytes != 1048576 {
		t.Fatalf("used memory = %d", snap.UsedMemoryBytes)
	}
	if snap.UsedMemoryRSSBytes != 2097152 {
		t.Fatalf("used memory rss = %d", snap.UsedMemoryRSSBytes)
	}
	if snap.MemFragmentationRatio != 1.25 {
		t.Fatalf("mem fragmentation ratio = %f", snap.MemFragmentationRatio)
	}
	if snap.InstantaneousOpsPerSec != 42 {
		t.Fatalf("ops/sec = %d", snap.InstantaneousOpsPerSec)
	}
	if snap.TotalCommandsProcessed != 12345 {
		t.Fatalf("total commands = %d", snap.TotalCommandsProcessed)
	}
	if snap.Keyspace["db0"] != 11 || snap.Keyspace["db2"] != 3 {
		t.Fatalf("keyspace = %#v", snap.Keyspace)
	}
}

func TestResourcePeaksUsesMaxAcrossSamples(t *testing.T) {
	samples := []artifact.ResourceSample{
		{
			Process:  artifact.ProcessSnapshot{RSSBytes: 1000, CPUPercent: 55.5, CPUTimeSeconds: 1.5, ThreadCount: 10, HandleCount: 100, OpenTCPSockets: 25},
			Runtime:  artifact.RuntimeSnapshot{Goroutines: 80, HeapAllocBytes: 100, HeapSysBytes: 300, GCCount: 5, PauseTotalNS: 1000, HTTPAcceptTotal: 10, HTTPActiveCurrent: 5},
			Redis:    artifact.RedisSnapshot{ConnectedClients: 3, UsedMemoryBytes: 1000, UsedMemoryRSSBytes: 2000, InstantaneousOpsPerSec: 40, TotalCommandsProcessed: 100},
			Postgres: artifact.PostgresSnapshot{ActiveConnections: 5, IdleConnections: 2, WaitingLocks: 1, DatabaseSizeBytes: 1000},
		},
		{
			Process:  artifact.ProcessSnapshot{RSSBytes: 2000, CPUPercent: 44.4, CPUTimeSeconds: 2.5, ThreadCount: 9, HandleCount: 120, OpenTCPSockets: 30},
			Runtime:  artifact.RuntimeSnapshot{Goroutines: 70, HeapAllocBytes: 150, HeapSysBytes: 250, GCCount: 8, PauseTotalNS: 900, HTTPAcceptTotal: 15, HTTPActiveCurrent: 7},
			Redis:    artifact.RedisSnapshot{ConnectedClients: 4, UsedMemoryBytes: 900, UsedMemoryRSSBytes: 3000, InstantaneousOpsPerSec: 60, TotalCommandsProcessed: 150},
			Postgres: artifact.PostgresSnapshot{ActiveConnections: 4, IdleConnections: 3, WaitingLocks: 2, DatabaseSizeBytes: 2000},
		},
	}
	peaks := Peaks(samples)
	assertUint64(t, "rss", peaks.RSSPeakBytes, 2000)
	assertFloat64(t, "cpu_percent", peaks.CPUPercentPeak, 55.5)
	assertFloat64(t, "cpu_time", peaks.CPUTimeSecondsPeak, 2.5)
	assertInt(t, "threads", peaks.ThreadCountPeak, 10)
	assertInt(t, "handles", peaks.HandleCountPeak, 120)
	assertInt(t, "tcp", peaks.OpenTCPSocketsPeak, 30)
	assertInt(t, "goroutines", peaks.GoroutinesPeak, 80)
	assertUint64(t, "heap_alloc", peaks.HeapAllocPeakBytes, 150)
	assertUint64(t, "heap_sys", peaks.HeapSysPeakBytes, 300)
	if peaks.GCCountPeak != 8 {
		t.Fatalf("gc_count = %d", peaks.GCCountPeak)
	}
	assertUint64(t, "pause_total", peaks.PauseTotalNSPeak, 1000)
	assertUint64(t, "http_accept", peaks.HTTPAcceptTotalPeak, 15)
	if peaks.HTTPActiveCurrentPeak != 7 {
		t.Fatalf("http active = %d", peaks.HTTPActiveCurrentPeak)
	}
	assertInt(t, "redis_clients", peaks.RedisConnectedClientsPeak, 4)
	assertUint64(t, "redis_used_memory", peaks.RedisUsedMemoryPeakBytes, 1000)
	assertUint64(t, "redis_rss", peaks.RedisUsedMemoryRSSPeakBytes, 3000)
	assertInt(t, "redis_ops", peaks.RedisInstantaneousOpsPeak, 60)
	assertUint64(t, "redis_commands", peaks.RedisTotalCommandsProcessedPeak, 150)
	assertInt(t, "postgres_active", peaks.PostgresActiveConnectionsPeak, 5)
	assertInt(t, "postgres_idle", peaks.PostgresIdleConnectionsPeak, 3)
	assertInt(t, "postgres_locks", peaks.PostgresWaitingLocksPeak, 2)
	assertUint64(t, "postgres_size", peaks.PostgresDatabaseSizePeakBytes, 2000)
}

func TestDrainStatusRequiresEachTableStable(t *testing.T) {
	passed := EvaluateDrain([]DrainSample{{ConsumeLogs: 2, PreConsumeRecords: 2, SubscriptionTokenUsed: 20}}, DrainExpectations{Success: 2, Tokens: 20})
	if passed.Status != "passed" {
		t.Fatalf("passed status = %#v", passed)
	}

	cases := []struct {
		name   string
		sample DrainSample
		table  string
	}{
		{name: "consume logs", sample: DrainSample{ConsumeLogs: 1, PreConsumeRecords: 2, SubscriptionTokenUsed: 20}, table: "consume_logs"},
		{name: "pre consume", sample: DrainSample{ConsumeLogs: 2, PreConsumeRecords: 1, SubscriptionTokenUsed: 20}, table: "subscription_pre_consume_records"},
		{name: "token used", sample: DrainSample{ConsumeLogs: 2, PreConsumeRecords: 2, SubscriptionTokenUsed: 19}, table: "user_subscriptions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateDrain([]DrainSample{tc.sample}, DrainExpectations{Success: 2, Tokens: 20})
			if got.Status != "failed" {
				t.Fatalf("status = %#v", got)
			}
			if !strings.Contains(got.Reason, tc.table) {
				t.Fatalf("reason %q does not contain %q", got.Reason, tc.table)
			}
		})
	}
}

func TestDrainStatusUsesDeltaFromFirstSample(t *testing.T) {
	got := EvaluateDrain([]DrainSample{
		{ConsumeLogs: 100, PreConsumeRecords: 100, SubscriptionTokenUsed: 1000},
		{ConsumeLogs: 101, PreConsumeRecords: 101, SubscriptionTokenUsed: 1027},
	}, DrainExpectations{Success: 2, Tokens: 56})
	if got.Status != "failed" {
		t.Fatalf("absolute pre-existing rows must not satisfy drain: %#v", got)
	}
	if !strings.Contains(got.Reason, "consume_logs") || !strings.Contains(got.Reason, "subscription_pre_consume_records") || !strings.Contains(got.Reason, "user_subscriptions") {
		t.Fatalf("reason should identify all missing deltas: %q", got.Reason)
	}

	passed := EvaluateDrain([]DrainSample{
		{ConsumeLogs: 100, PreConsumeRecords: 100, SubscriptionTokenUsed: 1000},
		{ConsumeLogs: 102, PreConsumeRecords: 102, SubscriptionTokenUsed: 1056},
	}, DrainExpectations{Success: 2, Tokens: 56})
	if passed.Status != "passed" {
		t.Fatalf("delta drain should pass: %#v", passed)
	}
}

func TestDrainStatusRequiresTokenDeltaEvenWhenRowsAlreadyObserved(t *testing.T) {
	got := EvaluateDrain([]DrainSample{
		{ConsumeLogs: 100, PreConsumeRecords: 100, SubscriptionTokenUsed: 1000},
		{ConsumeLogs: 110, PreConsumeRecords: 110, SubscriptionTokenUsed: 1000},
	}, DrainExpectations{Success: 10, Tokens: 280})
	if got.Status != "failed" || !strings.Contains(got.Reason, "user_subscriptions") {
		t.Fatalf("token delta must be required independently from log/preconsume rows: %#v", got)
	}
}

func TestWaitDrainDoesNotTreatBaselineAsProgress(t *testing.T) {
	samples := []DrainSample{
		{ConsumeLogs: 100, PreConsumeRecords: 100, SubscriptionTokenUsed: 1000},
		{ConsumeLogs: 110, PreConsumeRecords: 110, SubscriptionTokenUsed: 1000},
		{ConsumeLogs: 110, PreConsumeRecords: 110, SubscriptionTokenUsed: 1280},
	}
	var calls atomic.Int64
	gotSamples, status := WaitDrain(context.Background(), time.Millisecond, func() DrainSample {
		idx := int(calls.Add(1)) - 1
		if idx >= len(samples) {
			idx = len(samples) - 1
		}
		return samples[idx]
	}, DrainExpectations{Success: 10, Tokens: 280})
	if status.Status != "passed" {
		t.Fatalf("status = %#v", status)
	}
	if len(gotSamples) < 3 {
		t.Fatalf("baseline was treated as completed drain: samples=%#v", gotSamples)
	}
}

func TestSamplerCollectsAtLeastTwoSamples(t *testing.T) {
	var count atomic.Int64
	sam := NewSampler(SamplerOptions{
		Interval: 10 * time.Millisecond,
		Process: func() artifact.ProcessSnapshot {
			return artifact.ProcessSnapshot{Statused: artifact.Statused{Status: "ok"}, PID: int(count.Add(1))}
		},
		Runtime: func() artifact.RuntimeSnapshot {
			return artifact.RuntimeSnapshot{Statused: artifact.Statused{Status: "ok"}, Goroutines: 1}
		},
		Postgres: func() artifact.PostgresSnapshot {
			return artifact.PostgresSnapshot{Statused: artifact.Statused{Status: "ok"}}
		},
		Redis: func() artifact.RedisSnapshot {
			return artifact.RedisSnapshot{Statused: artifact.Statused{Status: "ok"}}
		},
	})
	stop := sam.Start()
	time.Sleep(35 * time.Millisecond)
	samples := stop()
	if len(samples) < 2 {
		t.Fatalf("samples len = %d", len(samples))
	}
	samples[0].Process.PID = 9999
	again := stop()
	if again[0].Process.PID == 9999 {
		t.Fatalf("stop returned aliased samples")
	}
}

func TestReadRuntimeSnapshotRequiresLoopbackURL(t *testing.T) {
	remote := ReadRuntimeSnapshot(context.Background(), "http://example.com/debug/loadtest/runtime")
	if remote.Status != "unavailable" || !strings.Contains(remote.Reason, "config") {
		t.Fatalf("remote status = %#v", remote.Statused)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"goroutines":12,"heap_alloc_bytes":4096,"heap_sys_bytes":8192,"gomaxprocs":2,"gomemlimit_bytes":1024,"gc_count":3,"pause_total_ns":9,"http_conn_state":{"active":2},"http_accept_total":5,"http_active_current":2,"batch_update":{"status":"ok"},"quota_data":{"status":"unavailable","reason":"not exposed"},"perf_metrics":{"status":"unavailable","reason":"not exposed"}}`)
	}))
	defer srv.Close()

	got := ReadRuntimeSnapshot(context.Background(), srv.URL)
	if got.Status != "ok" {
		t.Fatalf("status = %#v", got.Statused)
	}
	if got.Goroutines != 12 || got.HeapAllocBytes != 4096 || got.HeapSysBytes != 8192 || got.GOMAXPROCS != 2 || got.GOMEMLimitBytes != 1024 || got.GCCount != 3 || got.PauseTotalNS != 9 || got.HTTPAcceptTotal != 5 || got.HTTPActiveCurrent != 2 || got.HTTPConnState["active"] != 2 {
		t.Fatalf("runtime snapshot = %#v", got)
	}
}

func TestCollectSnapshotReadsPIDFileAndRuntimeURL(t *testing.T) {
	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "new-api.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"goroutines":21,"heap_alloc_bytes":2048,"heap_sys_bytes":4096,"gomaxprocs":2,"gc_count":4,"pause_total_ns":11,"http_accept_total":8,"http_active_current":1}`)
	}))
	defer runtimeServer.Close()

	out := filepath.Join(tmp, "snapshot.json")
	runCollector(t, "--pid-file", pidFile, "--runtime-url", runtimeServer.URL, "--out-snapshot", out)
	snap := readSnapshotFile(t, out)
	if snap.Process.Status == "unavailable" || snap.Process.PID != os.Getpid() {
		t.Fatalf("process snapshot = %#v", snap.Process)
	}
	if snap.Runtime.Status != "ok" || snap.Runtime.Goroutines != 21 || snap.Runtime.HeapAllocBytes != 2048 {
		t.Fatalf("runtime snapshot = %#v", snap.Runtime)
	}
}

func TestCollectSnapshotReadsRedisInfo(t *testing.T) {
	tmp := t.TempDir()
	addr := startFakeRedisInfoServer(t, redisInfoFixture)
	out := filepath.Join(tmp, "snapshot.json")
	runCollector(t, "--redis-addr", addr, "--out-snapshot", out)
	snap := readSnapshotFile(t, out)
	if snap.Redis.Status != "ok" {
		t.Fatalf("redis status = %#v", snap.Redis.Statused)
	}
	if snap.Redis.UsedMemoryBytes == 0 || snap.Redis.TotalCommandsProcessed == 0 || snap.Redis.ConnectedClients == 0 {
		t.Fatalf("redis snapshot = %#v", snap.Redis)
	}
}

func runCollector(t *testing.T, args ...string) {
	t.Helper()
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	cmdArgs := append([]string{"run", "./cmd/loadtest-collect"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = repoRoot
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("collector failed: %v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
}

func readSnapshotFile(t *testing.T, path string) artifact.Snapshot {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var snap artifact.Snapshot
	if err := common.DecodeJson(f, &snap); err != nil {
		t.Fatal(err)
	}
	return snap
}

func startFakeRedisInfoServer(t *testing.T, info string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveFakeRedisConn(conn, info)
		}
	}()
	return ln.Addr().String()
}

func serveFakeRedisConn(conn net.Conn, info string) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		command, err := readRedisCommand(reader)
		if err != nil {
			return
		}
		switch command {
		case "info":
			_, _ = fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(info), info)
		case "ping":
			_, _ = io.WriteString(conn, "+PONG\r\n")
		case "client":
			_, _ = io.WriteString(conn, "+OK\r\n")
		default:
			_, _ = io.WriteString(conn, "+OK\r\n")
		}
	}
}

func readRedisCommand(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "*") {
		count, err := strconv.Atoi(strings.TrimPrefix(line, "*"))
		if err != nil {
			return "", err
		}
		args := make([]string, 0, count)
		for i := 0; i < count; i++ {
			lenLine, err := reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			argLen, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(lenLine), "$"))
			if err != nil {
				return "", err
			}
			buf := make([]byte, argLen+2)
			if _, err := io.ReadFull(reader, buf); err != nil {
				return "", err
			}
			args = append(args, strings.ToLower(string(buf[:argLen])))
		}
		if len(args) == 0 {
			return "", fmt.Errorf("empty redis command")
		}
		return args[0], nil
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty redis command")
	}
	return strings.ToLower(fields[0]), nil
}

func assertInt(t *testing.T, name string, got int, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %d want %d", name, got, want)
	}
}

func assertUint64(t *testing.T, name string, got uint64, want uint64) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %d want %d", name, got, want)
	}
}

func assertFloat64(t *testing.T, name string, got float64, want float64) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %f want %f", name, got, want)
	}
}
