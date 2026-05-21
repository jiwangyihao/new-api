package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/orchestrator"
	"github.com/QuantumNous/new-api/pkg/loadtest/profile"
	"github.com/QuantumNous/new-api/pkg/loadtest/resource"
	"github.com/QuantumNous/new-api/pkg/loadtest/runner"
)

func main() { os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr)) }

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	return RunWithDeps(args, stdout, stderr, orchestrator.DefaultDependencies())
}

func RunWithDeps(args []string, stdout io.Writer, stderr io.Writer, deps orchestrator.Dependencies) int {
	opts, err := parseOptions(args)
	if err != nil {
		writeErr(stderr, err)
		return 2
	}
	if err := validateCLIOptions(opts); err != nil {
		writeErr(stderr, err)
		return 2
	}
	if cfg, err := loadAndValidateConfig(opts); err != nil {
		writeErr(stderr, err)
		return 2
	} else if cfg != nil {
		opts.Config = *cfg
	}
	if deps.StartMock == nil {
		deps.StartMock = startMock
	}
	if deps.StartServer == nil {
		deps.StartServer = startServer
	}
	var capturedErr error
	deps = captureDependencyErrors(deps, &capturedErr)
	result, code := orchestrator.Run(context.Background(), opts, deps)
	if opts.ArtifactDir != "" && result.SchemaVersion != 0 {
		if err := writeJSONFile(filepath.Join(opts.ArtifactDir, "resource-sweep.json"), result); err != nil && code == 0 {
			writeErr(stderr, err)
			return 1
		}
	}
	if code != 0 {
		if capturedErr != nil {
			writeErr(stderr, capturedErr)
		} else {
			writeErr(stderr, fmt.Errorf("resource sweep failed exit=%d", code))
		}
		return code
	}
	_, _ = fmt.Fprintf(stdout, "resource sweep written %s\n", filepath.Join(opts.ArtifactDir, "resource-sweep.json"))
	return 0
}

func parseOptions(args []string) (orchestrator.Options, error) {
	fs := flag.NewFlagSet("loadtest-resource-sweep", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts orchestrator.Options
	var points string
	fs.StringVar(&opts.ConfigPath, "config", "", "loadtest config")
	fs.StringVar(&opts.Profile, "profile", "", "loadtest profile")
	fs.StringVar(&opts.Binary, "binary", "", "new-api binary")
	fs.StringVar(&opts.WorkDir, "work-dir", "", "runtime work dir")
	fs.StringVar(&opts.ArtifactDir, "artifact-dir", "", "artifact dir")
	fs.StringVar(&opts.Scenario, "scenario", "", "scenario")
	fs.StringVar(&opts.Path, "path", "/v1/responses", "request path")
	fs.StringVar(&opts.TokenProfile, "token-profile", "", "token profile")
	fs.StringVar(&opts.APIKey, "api-key", "", "api key")
	fs.StringVar(&opts.MockProfile, "mock-profile", "", "mock profile")
	fs.StringVar(&points, "points", "", "comma-separated concurrency points")
	fs.IntVar(&opts.RequestsPerPoint, "requests-per-point", 0, "requests per point")
	fs.IntVar(&opts.RampStep, "ramp-step", 0, "ramp step")
	fs.DurationVar(&opts.RampInterval, "ramp-interval", 0, "ramp interval")
	fs.DurationVar(&opts.Duration, "duration", 0, "duration")
	fs.DurationVar(&opts.Timeout, "timeout", 0, "timeout")
	fs.BoolVar(&opts.ExternalIsolatedInfra, "external-isolated-infra", false, "use externally started isolated infra")
	if err := fs.Parse(args); err != nil {
		return orchestrator.Options{}, err
	}
	parsedPoints, err := parsePoints(points)
	if err != nil {
		return orchestrator.Options{}, err
	}
	opts.Points = parsedPoints
	return opts, nil
}

func validateCLIOptions(opts orchestrator.Options) error {
	if strings.TrimSpace(opts.Profile) == "" {
		return fmt.Errorf("--profile benchmark is required")
	}
	if opts.Profile == "h2c_diagnostic" {
		return fmt.Errorf("h2c diagnostic profile is not implemented in this phase")
	}
	if opts.Profile != "benchmark" {
		return fmt.Errorf("--profile benchmark is required")
	}
	if err := loadtestconfigKey(opts.APIKey); err != nil {
		return err
	}
	if strings.TrimSpace(opts.WorkDir) != "" {
		if _, err := os.Stat(filepath.Join(opts.WorkDir, ".env")); err == nil {
			return fmt.Errorf("work-dir .env is not allowed")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func loadAndValidateConfig(opts orchestrator.Options) (*loadtestconfig.File, error) {
	if strings.TrimSpace(opts.ConfigPath) == "" {
		return nil, nil
	}
	cfg, err := loadtestconfig.Load(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if _, err := cfg.Profile(opts.Profile); err != nil {
		return nil, err
	}
	return cfg, nil
}

func captureDependencyErrors(deps orchestrator.Dependencies, captured *error) orchestrator.Dependencies {
	record := func(err error) {
		if err != nil && captured != nil && *captured == nil {
			*captured = err
		}
	}
	if deps.BuildOrVerifyBinary != nil {
		fn := deps.BuildOrVerifyBinary
		deps.BuildOrVerifyBinary = func(ctx context.Context, opts orchestrator.Options) error {
			err := fn(ctx, opts)
			record(err)
			return err
		}
	}
	if deps.RunConfigCheck != nil {
		fn := deps.RunConfigCheck
		deps.RunConfigCheck = func(ctx context.Context, opts orchestrator.Options) error {
			err := fn(ctx, opts)
			record(err)
			return err
		}
	}
	if deps.PreflightInfra != nil {
		fn := deps.PreflightInfra
		deps.PreflightInfra = func(ctx context.Context, opts orchestrator.Options, cfg loadtestconfig.File) error {
			err := fn(ctx, opts, cfg)
			record(err)
			return err
		}
	}
	if deps.StartInfra != nil {
		fn := deps.StartInfra
		deps.StartInfra = func(ctx context.Context, opts orchestrator.Options, cfg loadtestconfig.File) (orchestrator.Process, error) {
			proc, err := fn(ctx, opts, cfg)
			record(err)
			return proc, err
		}
	}
	if deps.StartMock != nil {
		fn := deps.StartMock
		deps.StartMock = func(ctx context.Context, opts orchestrator.Options, rc artifact.RunContext) (orchestrator.Process, error) {
			proc, err := fn(ctx, opts, rc)
			record(err)
			return proc, err
		}
	}
	if deps.StartServer != nil {
		fn := deps.StartServer
		deps.StartServer = func(ctx context.Context, opts orchestrator.Options, env map[string]string) (orchestrator.Process, error) {
			proc, err := fn(ctx, opts, env)
			record(err)
			return proc, err
		}
	}
	if deps.BootstrapAndSeed != nil {
		fn := deps.BootstrapAndSeed
		deps.BootstrapAndSeed = func(ctx context.Context, opts orchestrator.Options, rc artifact.RunContext) (artifact.SeedOutput, error) {
			seed, err := fn(ctx, opts, rc)
			record(err)
			return seed, err
		}
	}
	if deps.RunPoint != nil {
		fn := deps.RunPoint
		deps.RunPoint = func(ctx context.Context, opts orchestrator.PointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
			point, analysis, samples, err := fn(ctx, opts)
			record(err)
			return point, analysis, samples, err
		}
	}
	if deps.ApplyLimits != nil {
		fn := deps.ApplyLimits
		deps.ApplyLimits = func(pid int, limits profile.ServerLimits) (resource.ApplyResult, error) {
			result, err := fn(pid, limits)
			record(err)
			return result, err
		}
	}
	if deps.RenderReport != nil {
		fn := deps.RenderReport
		deps.RenderReport = func(ctx context.Context, opts orchestrator.Options, sweep artifact.SweepResult, analyses []artifact.PointAnalysis, samples []artifact.ResourceSamplesArtifact, limits artifact.ResourceLimitsArtifact, ports artifact.PortsClosedArtifact) error {
			err := fn(ctx, opts, sweep, analyses, samples, limits, ports)
			record(err)
			return err
		}
	}
	return deps
}

func loadtestconfigKey(key string) error {
	key = strings.TrimSpace(strings.TrimPrefix(key, "Bearer "))
	switch key {
	case loadtestconfig.SubscriptionAPIKey, loadtestconfig.CompatAPIKey, loadtestconfig.InvalidAPIKey:
		return nil
	default:
		return fmt.Errorf("API key is not an allowed loadtest key")
	}
}

func parsePoints(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("--points contains an invalid concurrency")
		}
		out = append(out, value)
	}
	return out, nil
}

type commandProcess struct{ cmd *exec.Cmd }

func (p *commandProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *commandProcess) Stop(context.Context) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Kill()
	return p.cmd.Wait()
}

func startMock(ctx context.Context, opts orchestrator.Options, rc artifact.RunContext) (orchestrator.Process, error) {
	binary, err := siblingBinary(opts.Binary, "loadtest-mock-openai")
	if err != nil {
		return nil, err
	}
	cfg := opts.Config
	profileCfg, ok := cfg.MockProfiles[opts.MockProfile]
	if !ok {
		return nil, fmt.Errorf("mock profile %q is not configured", opts.MockProfile)
	}
	addr, err := listenAddrFromURL(cfg.MockUpstream.BaseURL)
	if err != nil {
		return nil, err
	}
	runContextPath := filepath.Join(opts.ArtifactDir, "run-context.json")
	statsPath := filepath.Join(opts.ArtifactDir, "mock-stats.json")
	if err := writeJSONFile(runContextPath, rc); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, binary,
		"--addr", addr,
		"--run-context", runContextPath,
		"--first-token-delay", profileCfg.FirstTokenDelay.String(),
		"--stream-duration", profileCfg.StreamDuration.String(),
		"--chunk-interval", profileCfg.ChunkInterval.String(),
		"--output-bytes", strconv.Itoa(profileCfg.OutputBytes),
		"--prompt-tokens", strconv.Itoa(profileCfg.PromptTokens),
		"--completion-tokens", strconv.Itoa(profileCfg.CompletionTokens),
		"--status-rate", statusRateFlagValue(profileCfg.StatusRate),
		"--seed", strconv.FormatInt(profileCfg.Seed, 10),
		"--stats-out", statsPath,
	)
	stdout, stderr, err := processLogs(opts.ArtifactDir, "mock")
	if err != nil {
		return nil, err
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	if err := waitHTTP(ctx, strings.TrimRight(cfg.MockUpstream.BaseURL, "/")+"/v1/models", 30*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	return &commandProcess{cmd: cmd}, nil
}

func startServer(ctx context.Context, opts orchestrator.Options, env map[string]string) (orchestrator.Process, error) {
	profileCfg := profile.Benchmark()
	cmd, err := orchestratorServerCommand(opts, env, profileCfg)
	if err != nil {
		return nil, err
	}
	stdout, stderr, err := processLogs(opts.ArtifactDir, "new-api")
	if err != nil {
		return nil, err
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	if err := waitHTTP(ctx, serverStatusURL(env), 30*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	return &commandProcess{cmd: cmd}, nil
}

func orchestratorServerCommand(opts orchestrator.Options, env map[string]string, p profile.Profile) (*exec.Cmd, error) {
	return runner.BuildCommandWithExpectedLimits(runner.Config{Binary: opts.Binary, WorkDir: opts.WorkDir, Env: env}, runner.ExpectedLimits{RelayMaxIdleConns: strconv.Itoa(p.Relay.MaxIdleConns), RelayMaxIdleConnsPerHost: strconv.Itoa(p.Relay.MaxIdleConnsPerHost), GOMEMLIMIT: p.ServerLimits.GOMEMLIMIT})
}

func siblingBinary(reference string, name string) (string, error) {
	if strings.TrimSpace(reference) == "" {
		return "", fmt.Errorf("binary is required")
	}
	ext := ""
	if strings.HasSuffix(strings.ToLower(reference), ".exe") {
		ext = ".exe"
	}
	candidate := filepath.Join(filepath.Dir(reference), name+ext)
	if _, err := os.Stat(candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

func listenAddrFromURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Host == "" || parsed.Port() == "" {
		return "", fmt.Errorf("mock upstream URL must include host and port")
	}
	return parsed.Host, nil
}

func statusRateFlagValue(values map[int]float64) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	for _, status := range []int{429, 502} {
		if rate, ok := values[status]; ok {
			parts = append(parts, fmt.Sprintf("%d=%g", status, rate))
		}
	}
	return strings.Join(parts, ",")
}

func serverStatusURL(env map[string]string) string {
	return "http://" + env["HOST"] + ":" + env["PORT"] + "/api/status"
}

func waitHTTP(ctx context.Context, rawURL string, timeout time.Duration) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		request, err := http.NewRequestWithContext(deadlineCtx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(request)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
				return nil
			}
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else if err != nil {
			lastErr = err
		}
		select {
		case <-deadlineCtx.Done():
			if lastErr != nil {
				return fmt.Errorf("waiting for %s: %w", rawURL, lastErr)
			}
			return fmt.Errorf("waiting for %s: %w", rawURL, deadlineCtx.Err())
		case <-ticker.C:
		}
	}
}

func processLogs(artifactDir string, name string) (*os.File, *os.File, error) {
	logDir := filepath.Join(artifactDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, err
	}
	stdout, err := os.OpenFile(filepath.Join(logDir, name+".stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, err
	}
	stderr, err := os.OpenFile(filepath.Join(logDir, name+".stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = stdout.Close()
		return nil, nil, err
	}
	return stdout, stderr, nil
}

func writeJSONFile(path string, v any) error {
	b, err := common.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func writeErr(stderr io.Writer, err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintln(stderr, artifact.Redact(err.Error()))
}
