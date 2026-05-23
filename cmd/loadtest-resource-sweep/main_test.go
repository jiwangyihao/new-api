package main

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/orchestrator"
	"github.com/QuantumNous/new-api/pkg/loadtest/profile"
	"github.com/QuantumNous/new-api/pkg/loadtest/resource"
)

func TestRunRejectsMissingBenchmarkProfile(t *testing.T) {
	cfgPath := writeResourceSweepConfig(t, t.TempDir(), nil)
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}

	code := RunWithDeps([]string{
		"--config", cfgPath,
		"--binary", filepath.Join(t.TempDir(), "new-api.exe"),
		"--work-dir", filepath.Join(t.TempDir(), "runtime"),
		"--artifact-dir", filepath.Join(t.TempDir(), "artifacts"),
		"--scenario", "benchmark",
		"--path", "/v1/responses",
		"--token-profile", "subscription",
		"--api-key", loadtestconfig.SubscriptionAPIKey,
		"--mock-profile", "s2-short-stream",
	}, stdout, stderr, orchestrator.Dependencies{})
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--profile benchmark is required") {
		t.Fatalf("missing profile error: %s", stderr.String())
	}
}

func TestRunRejectsH2CDiagnosticInFirstStage(t *testing.T) {
	cfgPath := writeResourceSweepConfig(t, t.TempDir(), nil)
	stderr := &strings.Builder{}
	code := RunWithDeps(minimalArgs(t, cfgPath, "h2c_diagnostic"), &strings.Builder{}, stderr, orchestrator.Dependencies{})
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "h2c diagnostic profile is not implemented") {
		t.Fatalf("missing h2c diagnostic error: %s", stderr.String())
	}
}

func TestRunRejectsNonLoopbackURLInConfig(t *testing.T) {
	cfgPath := writeResourceSweepConfig(t, t.TempDir(), func(s string) string {
		return strings.Replace(s, `base_url: "http://127.0.0.1:19080"`, `base_url: "https://api.openai.com"`, 1)
	})
	stderr := &strings.Builder{}
	code := RunWithDeps(minimalArgs(t, cfgPath, "benchmark"), &strings.Builder{}, stderr, orchestrator.Dependencies{})
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "safe loopback") && !strings.Contains(stderr.String(), "loopback") {
		t.Fatalf("missing loopback error: %s", stderr.String())
	}
}

func TestRunRejectsDefaultPostgresOrRedisPortsWithoutExplicitIsolatedInfra(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(string) string
		want string
	}{
		{name: "postgres", want: "PostgreSQL", edit: func(s string) string {
			return strings.Replace(s, ":15432/new_api_loadtest", ":5432/new_api_loadtest", 1)
		}},
		{name: "redis", want: "Redis", edit: func(s string) string {
			return strings.Replace(s, "127.0.0.1:16379", "127.0.0.1:6379", 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := writeResourceSweepConfig(t, t.TempDir(), tc.edit)
			stderr := &strings.Builder{}
			code := RunWithDeps(minimalArgs(t, cfgPath, "benchmark"), &strings.Builder{}, stderr, orchestrator.Dependencies{})
			if code != 2 {
				t.Fatalf("code=%d stderr=%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) || (!strings.Contains(stderr.String(), "default") && !strings.Contains(stderr.String(), "safe loadtest")) {
				t.Fatalf("missing default infra error: %s", stderr.String())
			}
		})
	}
}

func TestRunRejectsExternalInfraMarkerMismatch(t *testing.T) {
	cfgPath := writeResourceSweepConfig(t, t.TempDir(), nil)
	stderr := &strings.Builder{}
	deps := orchestrator.Dependencies{
		PreflightInfra: func(context.Context, orchestrator.Options, loadtestconfig.File) error {
			return errors.New("postgres marker mismatch for sk-secret")
		},
	}
	code := RunWithDeps(append(minimalArgs(t, cfgPath, "benchmark"), "--external-isolated-infra"), &strings.Builder{}, stderr, deps)
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "sk-secret") || !strings.Contains(stderr.String(), "postgres marker mismatch") {
		t.Fatalf("stderr was not redacted or lost marker reason: %s", stderr.String())
	}
}

func TestRunRejectsProductionEnvAndRealAPIKey(t *testing.T) {
	cfgPath := writeResourceSweepConfig(t, t.TempDir(), nil)
	workDir := t.TempDir()
	if err := writeTestFile(filepath.Join(workDir, ".env"), "OPENAI_API_KEY=sk-prodsecret\n"); err != nil {
		t.Fatal(err)
	}
	stderr := &strings.Builder{}
	args := minimalArgs(t, cfgPath, "benchmark")
	args = replaceFlag(args, "--work-dir", workDir)
	args = replaceFlag(args, "--api-key", "sk-realproductionkey")
	code := RunWithDeps(args, &strings.Builder{}, stderr, orchestrator.Dependencies{})
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "sk-realproductionkey") || strings.Contains(stderr.String(), "sk-prodsecret") {
		t.Fatalf("stderr leaked secret: %s", stderr.String())
	}
}

func TestCommandDependenciesUseCLIProcessStarters(t *testing.T) {
	deps := commandDependencies()
	defaults := orchestrator.DefaultDependencies()
	if reflect.ValueOf(deps.StartMock).Pointer() == reflect.ValueOf(defaults.StartMock).Pointer() {
		t.Fatal("command dependencies kept orchestrator placeholder StartMock")
	}
	if reflect.ValueOf(deps.StartServer).Pointer() == reflect.ValueOf(defaults.StartServer).Pointer() {
		t.Fatal("command dependencies kept orchestrator StartServer")
	}
	if reflect.ValueOf(deps.BuildOrVerifyBinary).Pointer() != reflect.ValueOf(defaults.BuildOrVerifyBinary).Pointer() {
		t.Fatal("command dependencies unexpectedly replaced non-process orchestrator defaults")
	}
	if reflect.ValueOf(deps.StartInfra).Pointer() != reflect.ValueOf(defaults.StartInfra).Pointer() {
		t.Fatal("command dependencies unexpectedly replaced infra default")
	}
}

func TestRunWritesPortsClosedOnInjectedFailure(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeResourceSweepConfig(t, dir, nil)
	var wrotePorts bool
	var startedPoint bool
	deps := orchestrator.Dependencies{
		BuildOrVerifyBinary: func(context.Context, orchestrator.Options) error { return nil },
		RunConfigCheck:      func(context.Context, orchestrator.Options) error { return nil },
		PreflightInfra:      func(context.Context, orchestrator.Options, loadtestconfig.File) error { return nil },
		StartInfra: func(context.Context, orchestrator.Options, loadtestconfig.File) (orchestrator.Process, error) {
			return fakeResourceSweepProcess{pid: 11}, nil
		},
		StopInfra: func(context.Context, orchestrator.Process) error { return nil },
		StartMock: func(context.Context, orchestrator.Options, artifact.RunContext) (orchestrator.Process, error) {
			return fakeResourceSweepProcess{pid: 22}, nil
		},
		StopMock: func(context.Context, orchestrator.Process) error { return nil },
		StartServer: func(context.Context, orchestrator.Options, map[string]string) (orchestrator.Process, error) {
			return fakeResourceSweepProcess{pid: 33}, nil
		},
		StopServer: func(context.Context, orchestrator.Process) error { return nil },
		BootstrapAndSeed: func(context.Context, orchestrator.Options, artifact.RunContext) (artifact.SeedOutput, error) {
			return artifact.SeedOutput{SchemaVersion: artifact.SchemaVersion, ExpectedUsagePerSuccess: artifact.Usage{TotalTokens: 28}}, nil
		},
		RunPoint: func(context.Context, orchestrator.PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
			startedPoint = true
			return artifact.PointResult{Concurrency: 2, Passed: false, Gate: artifact.GateResult{Passed: false, FailedReasons: []string{"injected"}}}, artifact.PointAnalysis{SchemaVersion: artifact.SchemaVersion, Concurrency: 2}, artifact.ResourceSamplesArtifact{SchemaVersion: artifact.SchemaVersion, Concurrency: 2}, nil
		},
		ApplyLimits: func(int, profile.ServerLimits) (resource.ApplyResult, error) {
			return resource.ApplyResult{Status: "ok"}, nil
		},
		CheckPorts: func(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
			return artifact.PortsClosedArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Ports: map[string]string{"13080": "open"}, Passed: false}
		},
		RenderReport: func(context.Context, orchestrator.Options, artifact.SweepResult, []artifact.PointAnalysis, []artifact.ResourceSamplesArtifact, artifact.ResourceLimitsArtifact, artifact.PortsClosedArtifact) error {
			return nil
		},
		WriteJSON: func(path string, v any) error {
			if strings.HasSuffix(path, "ports-closed.json") {
				wrotePorts = true
			}
			return nil
		},
	}
	stderr := &strings.Builder{}
	code := RunWithDeps(append(minimalArgs(t, cfgPath, "benchmark"), "--points", "2", "--requests-per-point", "10", "--ramp-step", "1", "--ramp-interval", "10ms", "--duration", "5s", "--timeout", "30s"), &strings.Builder{}, stderr, deps)
	if code == 0 {
		t.Fatalf("open ports after injected failure returned success; stderr=%s", stderr.String())
	}
	if !startedPoint {
		t.Fatal("injected point did not run")
	}
	if !wrotePorts {
		t.Fatal("ports-closed artifact was not written")
	}
}
func TestStartMockProcessOutlivesStartupContextCancellation(t *testing.T) {
	dir := t.TempDir()
	ext := ""
	if strings.HasSuffix(strings.ToLower(os.Args[0]), ".exe") {
		ext = ".exe"
	}
	mockBinary := filepath.Join(dir, "loadtest-mock-openai"+ext)
	copyCurrentExecutable(t, mockBinary)
	t.Setenv("GO_WANT_RESOURCE_SWEEP_MOCK_HELPER", "1")
	baseURL := freeLoopbackBaseURL(t)
	opts := orchestrator.Options{
		Binary:      filepath.Join(dir, "new-api"+ext),
		ArtifactDir: dir,
		MockProfile: "s2-short-stream",
		Config: loadtestconfig.File{
			MockUpstream: loadtestconfig.MockUpstreamConfig{BaseURL: baseURL},
			MockProfiles: map[string]loadtestconfig.MockProfile{
				"s2-short-stream": {FirstTokenDelay: time.Millisecond, StreamDuration: time.Millisecond, ChunkInterval: time.Millisecond, OutputBytes: 1, PromptTokens: 1, CompletionTokens: 1},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc, err := startMock(ctx, opts, artifact.RunContext{SchemaVersion: artifact.SchemaVersion})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Stop(context.Background()) }()
	cancel()
	time.Sleep(200 * time.Millisecond)
	if err := waitHTTP(context.Background(), strings.TrimRight(baseURL, "/")+"/v1/models", 2*time.Second); err != nil {
		t.Fatalf("mock process did not survive startup context cancellation: %v", err)
	}
}

func minimalArgs(t *testing.T, cfgPath string, profileName string) []string {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "new-api.exe")
	if err := writeTestFile(binary, "binary"); err != nil {
		t.Fatal(err)
	}
	return []string{
		"--config", cfgPath,
		"--profile", profileName,
		"--binary", binary,
		"--work-dir", filepath.Join(dir, "runtime"),
		"--artifact-dir", filepath.Join(dir, "artifacts"),
		"--scenario", "benchmark",
		"--path", "/v1/responses",
		"--token-profile", "subscription",
		"--api-key", loadtestconfig.SubscriptionAPIKey,
		"--mock-profile", "s2-short-stream",
	}
}

func replaceFlag(args []string, name string, value string) []string {
	out := append([]string(nil), args...)
	for i := 0; i+1 < len(out); i++ {
		if out[i] == name {
			out[i+1] = value
			return out
		}
	}
	return append(out, name, value)
}

type fakeResourceSweepProcess struct{ pid int }

func (p fakeResourceSweepProcess) PID() int                   { return p.pid }
func (p fakeResourceSweepProcess) Stop(context.Context) error { return nil }

func copyCurrentExecutable(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func freeLoopbackBaseURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return "http://" + addr
}

func init() {
	if os.Getenv("GO_WANT_RESOURCE_SWEEP_MOCK_HELPER") == "1" {
		runResourceSweepMockHelper()
		os.Exit(0)
	}
}

func runResourceSweepMockHelper() {
	fs := flag.NewFlagSet("loadtest-mock-openai-helper", flag.ExitOnError)
	addr := fs.String("addr", "", "addr")
	fs.String("run-context", "", "run context")
	fs.String("first-token-delay", "", "first token delay")
	fs.String("stream-duration", "", "stream duration")
	fs.String("chunk-interval", "", "chunk interval")
	fs.Int("output-bytes", 0, "output bytes")
	fs.Int("prompt-tokens", 0, "prompt tokens")
	fs.Int("completion-tokens", 0, "completion tokens")
	fs.String("status-rate", "", "status rate")
	fs.Int64("seed", 0, "seed")
	fs.String("stats-out", "", "stats out")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	if err := http.ListenAndServe(*addr, mux); err != nil {
		os.Exit(1)
	}
}

func writeResourceSweepConfig(t *testing.T, dir string, edit func(string) string) string {
	t.Helper()
	cfg := `server:
  host: "127.0.0.1"
  port: 13080
  pprof_addr: "127.0.0.1:8005"
  runtime_stats_enabled: true
postgres:
  dsn: "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable"
log_postgres:
  dsn: ""
redis:
  addr: "redis://127.0.0.1:16379/0"
mock_upstream:
  base_url: "http://127.0.0.1:19080"
loadtest:
  model: "gpt-5.5"
  group: "default"
  subscription_key: "sk-loadtestsub"
  compat_key: "sk-loadtestcompat"
  invalid_key: "sk-loadtestinvalid"
  token_db_key_subscription: "loadtestsub"
  token_db_key_compat: "loadtestcompat"
client:
  max_idle_conns: 64
  max_idle_conns_per_host: 16
profiles:
  benchmark:
    points: [250, 500, 750, 1000, 1250, 1500, 1750, 2000]
    requests_per_point: 3000
    ramp_step: 25
    ramp_interval: "200ms"
    duration: "45s"
    timeout: "120s"
    transport: {mode: "h1_keepalive", max_conns_per_host: 1024, max_idle_conns: 1024, max_idle_conns_per_host: 1024}
    relay: {max_idle_conns: 1024, max_idle_conns_per_host: 1024}
    server_limits: {gomaxprocs: "2", gogc: "100", gomemlimit: "384MiB", process_memory_limit_bytes: 536870912, cpu_affinity_cores: 2}
mock_profiles:
  s1-smoke: {first_token_delay: 50ms, stream_duration: 500ms, chunk_interval: 50ms, output_bytes: 128, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
  s2-short-stream: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
  s3-long-stream: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
  s4-error-refund: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0.05, 502: 0.01}, seed: 1}
  s5-large-payload: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
retry:
  retry_times: 0
  automatic_retry_status_codes: []
thresholds:
  latency_p95_regression_ratio: 1.10
  ttft_p95_regression_ratio: 1.10
`
	if edit != nil {
		cfg = edit(cfg)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := writeTestFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
