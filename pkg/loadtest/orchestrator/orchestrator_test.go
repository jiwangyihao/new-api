package orchestrator

import (
	"context"
	"errors"
	"reflect"
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
	if got.BaseURL != "http://127.0.0.1:13080" || got.RuntimeURL != "http://127.0.0.1:8005/debug/loadtest/runtime" || got.APIKey != loadtestconfig.SubscriptionAPIKey || got.TokenProfile != "subscription" || got.Path != "/v1/responses" || got.Model != cfg.Loadtest.Model || got.Scenario != "benchmark" || got.Config == nil || got.MockProfile != "s2-short-stream" || got.MockHash == "" || got.MockStatsURL != "http://127.0.0.1:19080/debug/loadtest/mock-stats" || got.Seed.SchemaVersion == 0 || got.ArtifactDir == "" || got.RequestsPerPoint != 10 || got.MaxRequests != 10 || got.RampStep != 1 || got.RampInterval != 10*time.Millisecond || got.Duration != 5*time.Second || got.Timeout != 30*time.Second || got.Transport.Mode != profile.TransportH1KeepAlive {
		t.Fatalf("incomplete point options: %#v", got)
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
