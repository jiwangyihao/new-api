package orchestrator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/profile"
	"github.com/QuantumNous/new-api/pkg/loadtest/resource"
)

type fakeProcess struct {
	pid int
}

func (p fakeProcess) PID() int                   { return p.pid }
func (p fakeProcess) Stop(context.Context) error { return nil }

func TestRunStopsAfterFirstFailedPointAndPassesProfileTimings(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	var got []PointOptions
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		got = append(got, opts)
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: false, Gate: artifact.GateResult{Passed: false, FailedReasons: []string{"injected"}}}, artifact.PointAnalysis{SchemaVersion: artifact.SchemaVersion, Concurrency: opts.Concurrency, FailureClass: "unknown", HardGate: artifact.GateResult{Passed: false, FailedReasons: []string{"injected"}}}, artifact.ResourceSamplesArtifact{SchemaVersion: artifact.SchemaVersion, Concurrency: opts.Concurrency}, nil
	})

	result, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{250, 500}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code == 0 {
		t.Fatal("failed point returned success")
	}
	if len(result.Points) != 1 || len(got) != 1 || got[0].Concurrency != 250 {
		t.Fatalf("points=%#v got=%#v", result.Points, got)
	}
	wantProfile := profile.Benchmark()
	if got[0].RequestsPerPoint != wantProfile.RequestsPerPoint || got[0].MaxRequests != wantProfile.RequestsPerPoint || got[0].RampStep != wantProfile.RampStep || got[0].RampInterval != wantProfile.RampInterval || got[0].Duration != wantProfile.Duration || got[0].Timeout != wantProfile.Timeout || got[0].Transport.Mode != wantProfile.Transport.Mode {
		t.Fatalf("profile timings not propagated: %#v", got[0])
	}
}

func TestRunAlwaysCleansUpAndWritesPortsArtifact(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	oldCleanupPortsTimeout := cleanupPortsTimeout
	oldCleanupPortsPollInterval := cleanupPortsPollInterval
	cleanupPortsTimeout = time.Millisecond
	cleanupPortsPollInterval = time.Millisecond
	t.Cleanup(func() {
		cleanupPortsTimeout = oldCleanupPortsTimeout
		cleanupPortsPollInterval = oldCleanupPortsPollInterval
	})
	var calls []string
	var wrotePorts bool
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: false}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.CheckPorts = func(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
		calls = append(calls, "check_ports")
		return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "open"}, Passed: false}
	}
	deps.WriteJSON = func(path string, v any) error {
		if strings.HasSuffix(path, "ports-closed.json") {
			wrotePorts = true
		}
		return nil
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{250}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code == 0 {
		t.Fatal("open port cleanup should fail")
	}
	assertSubsequence(t, calls, []string{"start_infra", "start_mock", "start_server", "run_point", "stop_server", "stop_mock", "stop_infra", "check_ports"})
	if !wrotePorts {
		t.Fatal("ports artifact was not written")
	}
}

func TestRunDoesNotStartMockWhenBootstrapContextCancels(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	oldCleanupPortsTimeout := cleanupPortsTimeout
	oldCleanupPortsPollInterval := cleanupPortsPollInterval
	cleanupPortsTimeout = time.Millisecond
	cleanupPortsPollInterval = time.Millisecond
	t.Cleanup(func() {
		cleanupPortsTimeout = oldCleanupPortsTimeout
		cleanupPortsPollInterval = oldCleanupPortsPollInterval
	})
	var calls []string
	deps := testDependencies(&calls, nil)
	deps.BootstrapAndSeed = func(ctx context.Context, opts Options, rc artifact.RunContext) (artifact.SeedOutput, error) {
		calls = append(calls, "seed")
		<-ctx.Done()
		return artifact.SeedOutput{}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, code := Run(ctx, Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code == 0 {
		t.Fatalf("bootstrap cancellation returned success; calls=%v", calls)
	}
	for _, call := range calls {
		if call == "start_mock" || call == "start_server" || call == "run_point" {
			t.Fatalf("started downstream work after bootstrap cancellation: %v", calls)
		}
	}
	assertSubsequence(t, calls, []string{"start_infra", "seed", "stop_infra", "check_ports"})
}

func TestRunAppliesStartupDeadlineBeforeStartingMock(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	oldCleanupPortsTimeout := cleanupPortsTimeout
	oldCleanupPortsPollInterval := cleanupPortsPollInterval
	cleanupPortsTimeout = time.Millisecond
	cleanupPortsPollInterval = time.Millisecond
	t.Cleanup(func() {
		cleanupPortsTimeout = oldCleanupPortsTimeout
		cleanupPortsPollInterval = oldCleanupPortsPollInterval
	})
	var calls []string
	deps := testDependencies(&calls, nil)
	deadlineSeen := false
	deps.BootstrapAndSeed = func(ctx context.Context, opts Options, rc artifact.RunContext) (artifact.SeedOutput, error) {
		calls = append(calls, "seed")
		if _, ok := ctx.Deadline(); ok {
			deadlineSeen = true
		}
		<-ctx.Done()
		return artifact.SeedOutput{}, ctx.Err()
	}
	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream", StartupTimeout: 20 * time.Millisecond}, deps)
	if code == 0 {
		t.Fatalf("startup deadline returned success; calls=%v", calls)
	}
	if !deadlineSeen {
		t.Fatalf("bootstrap did not receive a startup deadline; calls=%v", calls)
	}
	for _, call := range calls {
		if call == "start_mock" || call == "start_server" || call == "run_point" {
			t.Fatalf("started downstream work after startup deadline: %v", calls)
		}
	}
}

func TestRunReportsWhenCleanupPortGateFails(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	oldCleanupPortsTimeout := cleanupPortsTimeout
	oldCleanupPortsPollInterval := cleanupPortsPollInterval
	cleanupPortsTimeout = time.Millisecond
	cleanupPortsPollInterval = time.Millisecond
	t.Cleanup(func() {
		cleanupPortsTimeout = oldCleanupPortsTimeout
		cleanupPortsPollInterval = oldCleanupPortsPollInterval
	})
	var calls []string
	var reportPorts artifact.PortsClosedArtifact
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.CheckPorts = func(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
		calls = append(calls, "check_ports")
		return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "open"}, Passed: false}
	}
	deps.RenderReport = func(ctx context.Context, opts Options, sweep artifact.SweepResult, analyses []artifact.PointAnalysis, samples []artifact.ResourceSamplesArtifact, limits artifact.ResourceLimitsArtifact, ports artifact.PortsClosedArtifact) error {
		calls = append(calls, "report")
		reportPorts = ports
		return nil
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code == 0 {
		t.Fatal("open port cleanup should fail")
	}
	assertSubsequence(t, calls, []string{"check_ports", "report"})
	if reportPorts.Passed || reportPorts.Ports["13080"] != "open" {
		t.Fatalf("report did not receive cleanup ports artifact: %#v", reportPorts)
	}
}

func TestRunWaitsForPortsToCloseBeforeWritingPortsArtifact(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	checks := 0
	var wrotePorts artifact.PortsClosedArtifact
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.CheckPorts = func(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
		checks++
		calls = append(calls, "check_ports")
		if checks == 1 {
			return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "open"}, Passed: false}
		}
		return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "closed"}, Passed: true}
	}
	deps.WriteJSON = func(path string, v any) error {
		if strings.HasSuffix(path, "ports-closed.json") {
			wrotePorts = v.(artifact.PortsClosedArtifact)
		}
		return nil
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code != 0 {
		t.Fatalf("code=%d calls=%v ports=%#v", code, calls, wrotePorts)
	}
	if checks < 2 {
		t.Fatalf("ports were not rechecked after cleanup: checks=%d calls=%v", checks, calls)
	}
	if !wrotePorts.Passed || wrotePorts.Ports["13080"] != "closed" {
		t.Fatalf("ports artifact captured pre-drain state: %#v", wrotePorts)
	}
}

func TestRunStillWaitsForPortsAfterRunContextCancellation(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	var calls []string
	checks := 0
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		cancel()
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.CheckPorts = func(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
		checks++
		calls = append(calls, "check_ports")
		if checks == 1 {
			return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "open"}, Passed: false}
		}
		return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "closed"}, Passed: true}
	}

	_, code := Run(ctx, Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code != 0 {
		t.Fatalf("cleanup should use an independent bounded wait after run context cancellation; code=%d calls=%v", code, calls)
	}
	if checks < 2 {
		t.Fatalf("ports were not rechecked after run context cancellation: checks=%d calls=%v", checks, calls)
	}
}

func TestRunRequiresStableClosedPortsBeforeWritingArtifact(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	oldCleanupPortsPollInterval := cleanupPortsPollInterval
	oldCleanupPortsTimeout := cleanupPortsTimeout
	cleanupPortsTimeout = time.Second
	cleanupPortsPollInterval = time.Millisecond
	t.Cleanup(func() {
		cleanupPortsTimeout = oldCleanupPortsTimeout
		cleanupPortsPollInterval = oldCleanupPortsPollInterval
	})
	var calls []string
	checks := 0
	var wrotePorts artifact.PortsClosedArtifact
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.CheckPorts = func(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
		checks++
		calls = append(calls, "check_ports")
		if checks == 1 || checks == 3 {
			return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "open"}, Passed: false}
		}
		return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "closed"}, Passed: true}
	}
	deps.WriteJSON = func(path string, v any) error {
		if strings.HasSuffix(path, "ports-closed.json") {
			wrotePorts = v.(artifact.PortsClosedArtifact)
		}
		return nil
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code != 0 {
		t.Fatalf("cleanup should wait past transient closed states; code=%d calls=%v ports=%#v", code, calls, wrotePorts)
	}
	if checks < 5 {
		t.Fatalf("ports were not checked until a stable closed state: checks=%d calls=%v", checks, calls)
	}
	if !wrotePorts.Passed || wrotePorts.Ports["13080"] != "closed" {
		t.Fatalf("ports artifact did not capture stable closed state: %#v", wrotePorts)
	}
}

func TestRunFailsClosedWhenPortDerivationFails(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	cfg.MockUpstream.BaseURL = "http://127.0.0.1"
	var calls []string
	var reportPorts artifact.PortsClosedArtifact
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.CheckPorts = func(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
		calls = append(calls, "check_ports")
		return resource.CheckPortsClosed(rc, ports)
	}
	deps.RenderReport = func(ctx context.Context, opts Options, sweep artifact.SweepResult, analyses []artifact.PointAnalysis, samples []artifact.ResourceSamplesArtifact, limits artifact.ResourceLimitsArtifact, ports artifact.PortsClosedArtifact) error {
		calls = append(calls, "report")
		reportPorts = ports
		return nil
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code == 0 {
		t.Fatalf("invalid port derivation returned success; calls=%v", calls)
	}
	for _, call := range calls {
		if call == "check_ports" {
			t.Fatalf("port gate delegated invalid empty port list to CheckPorts: %v", calls)
		}
	}
	if reportPorts.Passed || !strings.Contains(reportPorts.Ports["config"], "invalid") {
		t.Fatalf("invalid port derivation did not produce failed ports artifact: %#v", reportPorts)
	}
}

func TestRunAppliesLimitsOnlyToServerPID(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	var limitPID int
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.ApplyLimits = func(pid int, limits profile.ServerLimits) (resource.ApplyResult, error) {
		limitPID = pid
		return resource.ApplyResult{Status: "ok", MemoryLimitEnforced: true, CPUAffinityEnforced: true}, nil
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{250}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code != 0 {
		t.Fatalf("code=%d calls=%v", code, calls)
	}
	if limitPID != 333 {
		t.Fatalf("limits applied to pid %d, want server pid 333", limitPID)
	}
}

func TestRunWritesPartialLimitArtifactWhenApplyLimitsReportsNestedJob(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	var wroteLimits artifact.ResourceLimitsArtifact
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.ApplyLimits = func(pid int, limits profile.ServerLimits) (resource.ApplyResult, error) {
		return resource.ApplyResult{Status: "partial", Reason: "job object assignment denied by current Windows job; env limits and CPU affinity still applied", CPUAffinityEnforced: true, ProcessMemoryLimitBytes: limits.ProcessMemoryLimitBytes, CPUAffinityCores: limits.CPUAffinityCores}, nil
	}
	deps.WriteJSON = func(path string, v any) error {
		if strings.HasSuffix(path, "resource-limits.json") {
			wroteLimits = v.(artifact.ResourceLimitsArtifact)
		}
		return nil
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code != 0 {
		t.Fatalf("report-only partial limit result failed run; code=%d calls=%v", code, calls)
	}
	if wroteLimits.Status != "partial" || wroteLimits.Reason == "" || !wroteLimits.OSCPUAffinityEnforced {
		t.Fatalf("partial limits artifact not recorded: %#v", wroteLimits)
	}
}

func TestRunWritesLimitArtifactBeforeFailingApplyLimitError(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	var wroteLimits artifact.ResourceLimitsArtifact
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.ApplyLimits = func(pid int, limits profile.ServerLimits) (resource.ApplyResult, error) {
		return resource.ApplyResult{Status: "partial", Reason: "set job object memory limit: injected", ProcessMemoryLimitBytes: limits.ProcessMemoryLimitBytes, CPUAffinityCores: limits.CPUAffinityCores}, errors.New("apply server limits: injected")
	}
	deps.WriteJSON = func(path string, v any) error {
		if strings.HasSuffix(path, "resource-limits.json") {
			wroteLimits = v.(artifact.ResourceLimitsArtifact)
		}
		return nil
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code == 0 {
		t.Fatalf("limit error returned success; calls=%v", calls)
	}
	if wroteLimits.Status != "partial" || wroteLimits.Reason != "set job object memory limit: injected" {
		t.Fatalf("limit failure artifact was not written before exit: %#v", wroteLimits)
	}
}

func TestRunFailsWhenPortsClosedArtifactCannotBeWritten(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.WriteJSON = func(path string, v any) error {
		if strings.HasSuffix(path, "ports-closed.json") {
			return errors.New("write ports artifact: injected")
		}
		return nil
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code == 0 {
		t.Fatalf("ports artifact write failure returned success; calls=%v", calls)
	}
}

func TestRunWithExternalInfraDoesNotRequireInfraPortsClosed(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	var checkedPorts []int
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})
	deps.CheckPorts = func(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
		calls = append(calls, "check_ports")
		checkedPorts = append([]int(nil), ports...)
		return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "closed", "19080": "closed", "8005": "closed"}, Passed: true}
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream", ExternalIsolatedInfra: true}, deps)
	if code != 0 {
		t.Fatalf("external isolated infra run failed; code=%d calls=%v checkedPorts=%v", code, calls, checkedPorts)
	}
	for _, port := range checkedPorts {
		if port == managedPostgresPort || port == managedRedisPort {
			t.Fatalf("external infra port %d was included in owned cleanup ports: %v", port, checkedPorts)
		}
	}
}

func TestRunFailsClosedWhenIsolatedInfraUnavailable(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	deps := testDependencies(&calls, nil)
	deps.PreflightInfra = func(context.Context, Options, loadtestconfig.File) error {
		calls = append(calls, "preflight_infra")
		return errors.New("postgres marker mismatch: sk-secret")
	}

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{250}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code != 2 {
		t.Fatalf("code=%d calls=%v", code, calls)
	}
	for _, call := range calls {
		if call == "start_mock" || call == "start_server" || call == "run_point" {
			t.Fatalf("started unsafe work after infra preflight failure: %v", calls)
		}
	}
}

func TestRunPreflightBinaryAndConfigBeforeStartingProcesses(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	for _, tc := range []struct {
		name string
		edit func(*Dependencies, *[]string)
	}{
		{name: "binary", edit: func(deps *Dependencies, calls *[]string) {
			deps.BuildOrVerifyBinary = func(context.Context, Options) error {
				*calls = append(*calls, "build")
				return errors.New("binary missing sk-secret")
			}
		}},
		{name: "config", edit: func(deps *Dependencies, calls *[]string) {
			deps.RunConfigCheck = func(context.Context, Options) error {
				*calls = append(*calls, "config")
				return errors.New("config failed sk-secret")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			deps := testDependencies(&calls, nil)
			tc.edit(&deps, &calls)
			_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{250}, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
			if code != 2 {
				t.Fatalf("code=%d calls=%v", code, calls)
			}
			for _, call := range calls {
				if strings.HasPrefix(call, "start_") || call == "run_point" {
					t.Fatalf("process work started before preflight completed: %v", calls)
				}
			}
		})
	}
}

func TestRunPointReceivesFullSweepOptions(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	var got PointOptions
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		got = opts
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{4}, RequestsPerPoint: 10, RampStep: 1, RampInterval: 10 * time.Millisecond, Duration: 5 * time.Second, Timeout: 30 * time.Second, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code != 0 {
		t.Fatalf("code=%d calls=%v", code, calls)
	}
	if got.BaseURL != "http://127.0.0.1:13080" || got.RuntimeURL != "http://127.0.0.1:13080/debug/loadtest/runtime" || got.APIKey != loadtestconfig.SubscriptionAPIKey || got.TokenProfile != "subscription" || got.Path != "/v1/responses" || got.Model != cfg.Loadtest.Model || got.Scenario != "benchmark" || got.Config == nil || got.MockProfile != "s2-short-stream" || got.MockHash == "" || got.MockStatsURL != "http://127.0.0.1:19080/debug/loadtest/mock-stats" || got.Seed.SchemaVersion == 0 || got.ArtifactDir == "" || got.RequestsPerPoint != 10 || got.MaxRequests != 10 || got.RampStep != 1 || got.RampInterval != 10*time.Millisecond || got.Duration != 5*time.Second || got.Timeout != 30*time.Second || got.Transport.Mode != profile.TransportH1KeepAlive {
		t.Fatalf("incomplete point options: %#v", got)
	}
}

func TestRunPointReceivesServerPID(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	var calls []string
	var got PointOptions
	deps := testDependencies(&calls, func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
		got = opts
		return artifact.PointResult{Concurrency: opts.Concurrency, Passed: true, Gate: artifact.GateResult{Passed: true}}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, nil
	})

	_, code := Run(context.Background(), Options{Config: cfg, Profile: "benchmark", Points: []int{2}, RequestsPerPoint: 10, ArtifactDir: t.TempDir(), APIKey: loadtestconfig.SubscriptionAPIKey, TokenProfile: "subscription", Scenario: "benchmark", Path: "/v1/responses", MockProfile: "s2-short-stream"}, deps)
	if code != 0 {
		t.Fatalf("code=%d calls=%v", code, calls)
	}
	if got.ServerPID != 333 {
		t.Fatalf("ServerPID = %d, want server pid 333", got.ServerPID)
	}
}

func TestStartInfraRequiresArtifactDir(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	proc, err := startInfra(context.Background(), Options{}, cfg)
	if err == nil {
		if proc != nil {
			_ = proc.Stop(context.Background())
		}
		t.Fatal("startInfra without artifact-dir returned nil error")
	}
	if !strings.Contains(err.Error(), "artifact-dir") {
		t.Fatalf("err = %v", err)
	}
}

func TestStartInfraUsesManagedStarterByDefault(t *testing.T) {
	cfg := orchestratorTestConfig(t)
	old := startManagedInfra
	t.Cleanup(func() { startManagedInfra = old })
	var called bool
	startManagedInfra = func(context.Context, Options, loadtestconfig.File) (Process, error) {
		called = true
		return fakeProcess{pid: 444}, nil
	}

	proc, err := startInfra(context.Background(), Options{ArtifactDir: t.TempDir()}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if proc.PID() != 444 || !called {
		t.Fatalf("managed starter was not used: proc=%#v called=%v", proc, called)
	}
}

func TestPostgresBinDirIsAbsoluteOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific path regression")
	}
	dir := postgresBinDir()
	if !filepath.IsAbs(dir) || !strings.Contains(dir, ":\\") {
		t.Fatalf("postgresBinDir = %q, want absolute Windows path", dir)
	}
}

func TestBuildCreateDatabaseCommandUsesPostgresAdminDatabase(t *testing.T) {
	cmd, err := buildCreateDatabaseCommand(context.Background(), `C:\PostgreSQL\bin\createdb.exe`, "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/postgres?sslmode=disable") || !strings.Contains(joined, postgresLoadtestDatabase) {
		t.Fatalf("createdb args = %v", cmd.Args)
	}
}

func TestRedisPortUsesConfiguredAddress(t *testing.T) {
	got, err := redisPort("redis://127.0.0.1:16444/0")
	if err != nil {
		t.Fatal(err)
	}
	if got != 16444 {
		t.Fatalf("redisPort = %d, want 16444", got)
	}
	got, err = redisPort("127.0.0.1:16445")
	if err != nil {
		t.Fatal(err)
	}
	if got != 16445 {
		t.Fatalf("redisPort bare addr = %d, want 16445", got)
	}
}

func TestRunCommandRedactedHonorsContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(cmd.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_WANT_FAKE_REDIS_SERVER=")
	started := time.Now()
	err := runCommandRedacted(cmd)
	if err == nil {
		t.Fatal("runCommandRedacted returned nil for a command killed by context timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("runCommandRedacted ignored context timeout; elapsed=%s err=%v", elapsed, err)
	}
}

func TestRedisManagedProcessStopDoesNotWaitForever(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	proc := &redisManagedProcess{cmd: exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestHelperProcess$")}
	proc.cmd.Env = append(proc.cmd.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_WANT_FAKE_REDIS_SERVER=")
	if err := proc.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- proc.Stop(ctx) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop waited forever after Kill")
	}
}

func TestStartManagedRedisOutlivesStartupContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	oldFindExecutable := findExecutableFn
	findExecutableFn = func(name string, windowsName string, candidates []string) (string, error) {
		if name == "redis-server" || windowsName == "redis-server.exe" {
			return os.Args[0], nil
		}
		return oldFindExecutable(name, windowsName, candidates)
	}
	oldManagedRedisCommand := managedRedisCommand
	managedRedisCommand = func(name string, arg ...string) *exec.Cmd {
		args := append([]string{"-test.run=^TestHelperProcess$", "--"}, arg...)
		cmd := exec.Command(name, args...)
		cmd.Env = append(cmd.Environ(), "GO_WANT_FAKE_REDIS_SERVER=1", "GO_WANT_HELPER_PROCESS=")
		return cmd
	}
	t.Cleanup(func() {
		findExecutableFn = oldFindExecutable
		managedRedisCommand = oldManagedRedisCommand
	})

	ctx, cancel := context.WithCancel(context.Background())
	proc, err := startManagedRedis(ctx, Options{ArtifactDir: t.TempDir()}, loadtestconfig.File{Redis: loadtestconfig.RedisConfig{Addr: "redis://" + addr + "/0"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Stop(context.Background()) }()
	cancel()
	time.Sleep(time.Second)
	if proc, ok := proc.(*redisManagedProcess); ok {
		exited := make(chan error, 1)
		go func() { exited <- proc.cmd.Wait() }()
		select {
		case err := <-exited:
			proc.cmd.Process = nil
			t.Fatalf("managed redis exited after startup context cancellation: %v", err)
		case <-time.After(200 * time.Millisecond):
		}
	}
	if err := waitRedis(context.Background(), "redis://"+addr+"/0", time.Second); err != nil {
		t.Fatalf("managed redis process did not survive startup context cancellation: %v", err)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_FAKE_REDIS_SERVER") == "1" {
		runFakeRedisServer()
		os.Exit(0)
	}
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	select {}
}

func runFakeRedisServer() {
	confPath := ""
	for index, arg := range os.Args {
		if arg == "--" && index+1 < len(os.Args) {
			confPath = os.Args[index+1]
			break
		}
	}
	port := "16379"
	if confPath != "" {
		if file, err := os.Open(confPath); err == nil {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				fields := strings.Fields(scanner.Text())
				if len(fields) == 2 && fields[0] == "port" {
					port = fields[1]
					break
				}
			}
			_ = file.Close()
		}
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		os.Exit(1)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go handleFakeRedisConn(conn)
	}
}

func handleFakeRedisConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if strings.HasPrefix(line, "*") {
			count := 0
			_, _ = fmt.Sscanf(strings.TrimSpace(line), "*%d", &count)
			args := make([]string, 0, count)
			for i := 0; i < count; i++ {
				bulk, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				var size int
				_, _ = fmt.Sscanf(strings.TrimSpace(bulk), "$%d", &size)
				buf := make([]byte, size+2)
				if _, err := reader.Read(buf); err != nil {
					return
				}
				args = append(args, string(buf[:size]))
			}
			if len(args) > 0 && strings.EqualFold(args[0], "ping") {
				_, _ = conn.Write([]byte("+PONG\r\n"))
				continue
			}
		}
		_, _ = conn.Write([]byte("+OK\r\n"))
	}
}

func TestRedisMarkerAllowsKnownLoadtestKeys(t *testing.T) {
	for _, key := range []string{
		"loadtest:marker",
		"token:" + strings.Repeat("a", 64),
		"user:1",
		"notify_limit:1:test:2026052201",
		"rateLimit:success:1",
		"rateLimit:1",
		"subscription:concurrency:user:1",
		"subscription:concurrency:user:1:queue",
		"perf:gpt-5.5:default:1770000000",
		"new-api:subscription_plan:v1:910010",
		"new-api:subscription_plan_info:v1:sub:910011",
	} {
		if !isLoadtestRedisKey(key) {
			t.Fatalf("known loadtest Redis key rejected: %s", key)
		}
	}
}

func TestRedisMarkerRejectsUnknownKeys(t *testing.T) {
	for _, key := range []string{"session:prod", "token:not-a-hash", "user:abc", "rateLimit:", "subscription:concurrency:user:abc"} {
		if isLoadtestRedisKey(key) {
			t.Fatalf("unknown Redis key accepted: %s", key)
		}
	}
}

func TestRenderReportUsesResourceSweepRenderer(t *testing.T) {
	dir := t.TempDir()
	rc := artifact.RunContext{SchemaVersion: artifact.SchemaVersion, Role: "baseline", Commit: "abcdef0", ComparisonConfigHash: "sha256:cfg", SeedOutputHash: "sha256:seed", MockHash: "sha256:mock", CacheMode: "cold-fresh-role,warm-per-point", Scenario: "benchmark", Path: "/v1/responses", TokenProfile: "subscription", Model: "gpt-5.5"}
	sweep := artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, HighestPassedConcurrency: 2, Points: []artifact.PointResult{{Concurrency: 2, Passed: true}}}
	resources := []artifact.ResourceSamplesArtifact{{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Concurrency: 2, Peaks: artifact.ResourcePeaks{RSSPeakBytes: 64 << 20, CPUPercentPeak: 12.5, HeapAllocPeakBytes: 32 << 20, RedisUsedMemoryPeakBytes: 8 << 20, PostgresActiveConnectionsPeak: 1}}}
	limits := artifact.ResourceLimitsArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, ServerEnv: map[string]string{"GOMEMLIMIT": "384MiB"}, Statused: artifact.Statused{Status: "ok"}}
	ports := artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "closed", "15432": "closed", "16379": "closed"}, Passed: true}

	if err := renderReport(context.Background(), Options{ArtifactDir: dir}, sweep, nil, resources, limits, ports); err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(filepath.Join(dir, "reports", "resource-sweep.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Resource Sweep Report", "GOMEMLIMIT=384MiB", "Redis used_memory", "ports closed"} {
		if !strings.Contains(string(md), want) {
			t.Fatalf("report missing %q:\n%s", want, string(md))
		}
	}
}

func testDependencies(calls *[]string, runPoint func(context.Context, PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error)) Dependencies {
	if runPoint == nil {
		runPoint = func(context.Context, PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
			*calls = append(*calls, "run_point")
			return artifact.PointResult{}, artifact.PointAnalysis{}, artifact.ResourceSamplesArtifact{}, errors.New("run point should not be called")
		}
	}
	return Dependencies{
		BuildOrVerifyBinary: func(context.Context, Options) error { *calls = append(*calls, "build"); return nil },
		RunConfigCheck:      func(context.Context, Options) error { *calls = append(*calls, "config"); return nil },
		PreflightInfra: func(context.Context, Options, loadtestconfig.File) error {
			*calls = append(*calls, "preflight_infra")
			return nil
		},
		StartInfra: func(context.Context, Options, loadtestconfig.File) (Process, error) {
			*calls = append(*calls, "start_infra")
			return fakeProcess{pid: 111}, nil
		},
		StopInfra: func(context.Context, Process) error { *calls = append(*calls, "stop_infra"); return nil },
		StartMock: func(context.Context, Options, artifact.RunContext) (Process, error) {
			*calls = append(*calls, "start_mock")
			return fakeProcess{pid: 222}, nil
		},
		StopMock: func(context.Context, Process) error { *calls = append(*calls, "stop_mock"); return nil },
		StartServer: func(context.Context, Options, map[string]string) (Process, error) {
			*calls = append(*calls, "start_server")
			return fakeProcess{pid: 333}, nil
		},
		StopServer: func(context.Context, Process) error { *calls = append(*calls, "stop_server"); return nil },
		BootstrapAndSeed: func(context.Context, Options, artifact.RunContext) (artifact.SeedOutput, error) {
			*calls = append(*calls, "seed")
			return artifact.SeedOutput{SchemaVersion: artifact.SchemaVersion, RunContext: artifact.RunContext{SchemaVersion: artifact.SchemaVersion}, ExpectedUsagePerSuccess: artifact.Usage{TotalTokens: 28}}, nil
		},
		RunPoint: func(ctx context.Context, opts PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
			*calls = append(*calls, "run_point")
			return runPoint(ctx, opts)
		},
		ApplyLimits: func(pid int, limits profile.ServerLimits) (resource.ApplyResult, error) {
			*calls = append(*calls, "limits")
			return resource.ApplyResult{Status: "ok", ProcessMemoryLimitBytes: limits.ProcessMemoryLimitBytes, CPUAffinityCores: limits.CPUAffinityCores}, nil
		},
		CheckPorts: func(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
			*calls = append(*calls, "check_ports")
			return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "closed"}, Passed: true}
		},
		RenderReport: func(context.Context, Options, artifact.SweepResult, []artifact.PointAnalysis, []artifact.ResourceSamplesArtifact, artifact.ResourceLimitsArtifact, artifact.PortsClosedArtifact) error {
			*calls = append(*calls, "report")
			return nil
		},
		WriteJSON: func(string, any) error { *calls = append(*calls, "write_json"); return nil },
	}
}

func orchestratorTestConfig(t *testing.T) loadtestconfig.File {
	t.Helper()
	cfg, err := loadtestconfig.Load("../../../config.loadtest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return *cfg
}

func assertSubsequence(t *testing.T, got []string, want []string) {
	t.Helper()
	pos := 0
	for _, call := range got {
		if pos < len(want) && call == want[pos] {
			pos++
		}
	}
	if pos != len(want) {
		t.Fatalf("calls %v do not contain subsequence %v", got, want)
	}
	if reflect.DeepEqual(got, want) {
		return
	}
}
