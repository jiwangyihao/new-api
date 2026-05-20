package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestclient "github.com/QuantumNous/new-api/pkg/loadtest/client"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/metrics"
	"github.com/QuantumNous/new-api/pkg/loadtest/sweep"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() { os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr)) }

var openSweepDB = func(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("loadtest-concurrency-sweep", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	deriveOnly := fs.Bool("derive-run-context-only", false, "derive context only")
	configPath := fs.String("config", "", "config")
	urlFlag := fs.String("url", "", "base url")
	apiKey := fs.String("api-key", "", "api key")
	tokenProfile := fs.String("token-profile", "", "token profile")
	path := fs.String("path", "/v1/responses", "path")
	model := fs.String("model", "gpt-5.5", "model")
	scenario := fs.String("scenario", "", "scenario")
	pointsRaw := fs.String("points", "", "points")
	rps := fs.Float64("rps", 0, "rps")
	duration := fs.Duration("duration", 0, "duration")
	maxRequests := fs.Int("max-requests-per-point", 0, "max requests")
	rampStep := fs.Int("ramp-step", 0, "ramp step")
	rampInterval := fs.Duration("ramp-interval", 0, "ramp interval")
	timeout := fs.Duration("timeout", 30*time.Second, "timeout")
	inputBytes := fs.Int("input-bytes", 0, "input bytes")
	outputBytes := fs.Int("output-bytes", 0, "output bytes")
	cooldown := fs.Duration("cooldown", 0, "cooldown")
	_ = fs.String("pid-file", "", "pid file")
	seedOutputPath := fs.String("seed-output", "", "seed output")
	mockProfile := fs.String("mock-profile", "", "mock profile")
	mockStats := fs.String("mock-stats", "", "mock stats")
	mockHash := fs.String("mock-hash", "", "mock hash")
	runContextPath := fs.String("run-context", "", "run context")
	outRunContextPath := fs.String("out-run-context", "", "out run context")
	artifactDir := fs.String("artifact-dir", "", "artifact dir")
	outPath := fs.String("out", "", "out")
	if err := fs.Parse(args); err != nil {
		writeErr(stderr, err)
		return 2
	}
	var base artifact.RunContext
	if *runContextPath != "" {
		if err := readJSON(*runContextPath, &base); err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	base.Model = firstNonEmpty(base.Model, *model)
	hash := *mockHash
	var cfg *loadtestconfig.File
	if *configPath != "" {
		loaded, err := loadtestconfig.Load(*configPath)
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
		if err := loaded.Validate(); err != nil {
			writeErr(stderr, err)
			return 2
		}
		cfg = loaded
		if *mockProfile != "" {
			hash = loaded.MockProfileHash(*mockProfile)
		}
	}
	if *deriveOnly {
		if hash == "" {
			writeErr(stderr, fmt.Errorf("--config/--mock-profile or --mock-hash is required"))
			return 2
		}
		rc, err := sweep.DeriveRunContext(base, sweep.DeriveOptions{Scenario: *scenario, Path: *path, TokenProfile: *tokenProfile, APIKey: *apiKey, MockHash: hash})
		if err != nil {
			writeErr(stderr, err)
			return 2
		}
		if *outRunContextPath == "" {
			writeErr(stderr, fmt.Errorf("--out-run-context is required"))
			return 2
		}
		if err := writeJSONFile(*outRunContextPath, rc); err != nil {
			writeErr(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "run context written %s\n", *outRunContextPath)
		return 0
	}
	points := parsePoints(*pointsRaw)
	if len(points) == 0 {
		points = []int{1}
	}
	if cfg == nil {
		writeErr(stderr, fmt.Errorf("--config is required"))
		return 2
	}
	if *urlFlag == "" || *mockStats == "" || *seedOutputPath == "" || *artifactDir == "" {
		writeErr(stderr, fmt.Errorf("--url, --mock-stats, --seed-output and --artifact-dir are required"))
		return 2
	}
	if *maxRequests <= 0 && *duration <= 0 {
		writeErr(stderr, fmt.Errorf("--max-requests-per-point or --duration is required"))
		return 2
	}
	if hash == "" {
		writeErr(stderr, fmt.Errorf("--config/--mock-profile or --mock-hash is required"))
		return 2
	}
	if err := loadtestclient.ValidateTokenProfile(*apiKey, *tokenProfile); err != nil {
		writeErr(stderr, err)
		return 2
	}
	if err := validateSweepBaseURL(*urlFlag, cfg); err != nil {
		writeErr(stderr, err)
		return 2
	}
	rc := base
	if rc.MockHash == "" {
		rc.MockHash = hash
	}
	if rc.MockHash != hash {
		writeErr(stderr, fmt.Errorf("run_context.mock_hash mismatch"))
		return 2
	}
	if rc.Scenario == "" {
		rc.Scenario = *scenario
	}
	if rc.Path == "" {
		rc.Path = *path
	}
	if rc.TokenProfile == "" {
		rc.TokenProfile = *tokenProfile
	}
	if rc.Model == "" {
		rc.Model = *model
	}
	if rc.Scenario == "" || rc.Path == "" || rc.TokenProfile == "" || rc.SeedOutputHash == "" {
		writeErr(stderr, fmt.Errorf("run_context scenario, path, token_profile and seed_output_hash are required"))
		return 2
	}
	var seed artifact.SeedOutput
	if err := readJSON(*seedOutputPath, &seed); err != nil {
		writeErr(stderr, err)
		return 1
	}
	if seedHash, err := artifact.HashSeedOutput(seed); err != nil {
		writeErr(stderr, err)
		return 1
	} else if seedHash != rc.SeedOutputHash {
		writeErr(stderr, fmt.Errorf("seed_output_hash mismatch"))
		return 2
	}
	if err := os.MkdirAll(*artifactDir, 0o755); err != nil {
		writeErr(stderr, err)
		return 1
	}
	db, err := openSweepDB(cfg.Postgres.DSN)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	result := artifact.SweepResult{SchemaVersion: artifact.SchemaVersion, RunContext: rc, Scenario: rc.Scenario, Path: rc.Path, TokenProfile: rc.TokenProfile, RunID: time.Now().UTC().Format("20060102T150405Z") + "-" + sanitizeName(rc.Scenario)}
	for _, c := range points {
		point, code := runPoint(runPointOptions{Concurrency: c, BaseURL: *urlFlag, APIKey: *apiKey, TokenProfile: *tokenProfile, Path: *path, Model: *model, Scenario: *scenario, RPS: *rps, Duration: *duration, MaxRequests: *maxRequests, RampStep: *rampStep, RampInterval: *rampInterval, Timeout: *timeout, InputBytes: *inputBytes, OutputBytes: *outputBytes, Cooldown: *cooldown, MockStats: *mockStats, RuntimeURL: mustJoinURL(*urlFlag, "/debug/loadtest/runtime"), ArtifactDir: *artifactDir, Seed: seed, RunContext: rc, Config: cfg, MockProfile: *mockProfile, Stdout: stdout, Stderr: stderr, DB: db})
		result.Points = append(result.Points, point)
		if point.Passed {
			result.HighestPassedConcurrency = c
		} else if result.FirstFailedConcurrency == nil {
			failed := c
			result.FirstFailedConcurrency = &failed
		}
		if code != 0 || !point.Passed {
			break
		}
	}
	if *outPath == "" {
		writeErr(stderr, fmt.Errorf("--out is required"))
		return 2
	}
	if err := writeJSONFile(*outPath, result); err != nil {
		writeErr(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "sweep written %s\n", *outPath)
	if result.FirstFailedConcurrency != nil {
		return 2
	}
	return 0
}

type runPointOptions struct {
	Concurrency  int
	BaseURL      string
	APIKey       string
	TokenProfile string
	Path         string
	Model        string
	Scenario     string
	RPS          float64
	Duration     time.Duration
	MaxRequests  int
	RampStep     int
	RampInterval time.Duration
	Timeout      time.Duration
	InputBytes   int
	OutputBytes  int
	Cooldown     time.Duration
	MockStats    string
	RuntimeURL   string
	ArtifactDir  string
	Seed         artifact.SeedOutput
	RunContext   artifact.RunContext
	Config       *loadtestconfig.File
	MockProfile  string
	Stdout       io.Writer
	Stderr       io.Writer
	DB           *gorm.DB
}

func runPoint(opts runPointOptions) (artifact.PointResult, int) {
	prefix := filepath.Join(opts.ArtifactDir, fmt.Sprintf("c%d", opts.Concurrency))
	point := artifact.PointResult{Concurrency: opts.Concurrency, SummaryPath: prefix + "-summary.json", MetricsBeforePath: prefix + "-before.json", MetricsAfterPath: prefix + "-after.json", MetricsDiffPath: prefix + "-diff.json"}
	beforeMock, err := readMockStats(opts.MockStats)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	beforePath := prefix + "-mock-stats-before.json"
	afterPath := prefix + "-mock-stats-after.json"
	deltaPath := prefix + "-mock-stats-delta.json"
	if err := writeJSONFile(beforePath, beforeMock); err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	beforeBusiness, err := metrics.LoadBusinessSnapshot(opts.DB, opts.Seed)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	beforeRuntime, err := readRuntimeSnapshot(opts.RuntimeURL)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	beforeSnapshot := artifact.Snapshot{SchemaVersion: artifact.SchemaVersion, RunContext: opts.RunContext, Business: beforeBusiness, Runtime: beforeRuntime, Logs: metrics.ScanServerLogs("", "")}
	if err := writeJSONFile(point.MetricsBeforePath, beforeSnapshot); err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	summary, err := loadtestclient.RunLoad(context.Background(), loadtestclient.Options{BaseURL: opts.BaseURL, APIKey: opts.APIKey, TokenProfile: opts.TokenProfile, Path: opts.Path, Model: opts.Model, Scenario: opts.Scenario, Concurrency: opts.Concurrency, RPS: opts.RPS, Duration: opts.Duration, MaxRequests: opts.MaxRequests, RampStep: opts.RampStep, RampInterval: opts.RampInterval, Timeout: opts.Timeout, InputBytes: opts.InputBytes, Stream: true, RunContext: opts.RunContext})
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	if err := writeJSONFile(point.SummaryPath, summary); err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	if opts.Cooldown > 0 {
		time.Sleep(opts.Cooldown)
	}
	afterMock, err := readMockStats(opts.MockStats)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	if err := writeJSONFile(afterPath, afterMock); err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	delta, err := sweep.BuildMockStatsDelta(beforeMock, afterMock, opts.RunContext)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 2
	}
	delta.Path = deltaPath
	delta.Hash = ""
	deltaHash, err := artifact.HashCanonical(delta)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	delta.Hash = deltaHash
	if err := writeJSONFile(deltaPath, delta); err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	afterBusiness, err := metrics.LoadBusinessSnapshot(opts.DB, opts.Seed)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	afterRuntime, err := readRuntimeSnapshot(opts.RuntimeURL)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	afterSnapshot := artifact.Snapshot{SchemaVersion: artifact.SchemaVersion, RunContext: opts.RunContext, Business: afterBusiness, Runtime: afterRuntime, Logs: metrics.ScanServerLogs("", "")}
	if err := writeJSONFile(point.MetricsAfterPath, afterSnapshot); err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	logRows, preRows, err := metrics.LoadBusinessRows(opts.DB, summary)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	diff, inv := metrics.BuildDiff(metrics.DiffInputs{Before: beforeSnapshot, After: afterSnapshot, Summary: summary, SeedOutput: opts.Seed, MockDelta: delta, RunContext: opts.RunContext, ConsumeLogRows: logRows, PreConsumeRows: preRows, BusinessBefore: beforeSnapshot.Business, BusinessAfter: afterSnapshot.Business})
	if err := writeJSONFile(point.MetricsDiffPath, diff); err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, 1
	}
	point.SummaryExcerpt = summaryExcerpt(summary, delta)
	point.MockDelta = delta
	point.Invariants = diff.BusinessDelta.Invariants
	point.Invariants = append(point.Invariants, inv)
	point.ResourceDelta = diff.ResourceDelta
	point.ResourcePeaks = resourcePeaks(beforeSnapshot.Runtime, afterSnapshot.Runtime)
	point.Gate = sweep.EvaluateGate(opts.Scenario, point, gateOptions(opts))
	point.Passed = point.Gate.Passed
	return point, 0
}

func readRuntimeSnapshot(runtimeURL string) (artifact.RuntimeSnapshot, error) {
	if runtimeURL == "" {
		return artifact.RuntimeSnapshot{}, fmt.Errorf("runtime URL is required")
	}
	if err := loadtestclient.ValidateLoopbackURL(runtimeURL); err != nil {
		return artifact.RuntimeSnapshot{}, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(runtimeURL)
	if err != nil {
		return artifact.RuntimeSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return artifact.RuntimeSnapshot{}, fmt.Errorf("runtime stats status %d", resp.StatusCode)
	}
	var snap artifact.RuntimeSnapshot
	if err := common.DecodeJson(resp.Body, &snap); err != nil {
		return artifact.RuntimeSnapshot{}, err
	}
	if snap.Goroutines <= 0 || snap.HeapAllocBytes == 0 {
		return artifact.RuntimeSnapshot{}, fmt.Errorf("runtime stats missing resource samples")
	}
	snap.Statused = artifact.Statused{Status: "ok"}
	return snap, nil
}

func resourcePeaks(before, after artifact.RuntimeSnapshot) artifact.ResourcePeaks {
	return artifact.ResourcePeaks{GoroutinesPeak: maxInt(before.Goroutines, after.Goroutines), HeapAllocPeakBytes: maxUint64(before.HeapAllocBytes, after.HeapAllocBytes)}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func validateSweepBaseURL(raw string, cfg *loadtestconfig.File) error {
	if err := loadtestclient.ValidateLoopbackURL(raw); err != nil {
		return err
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	wantHost := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	gotHost := parsed.Host
	if parsed.Port() == "" {
		gotHost = net.JoinHostPort(parsed.Hostname(), "80")
	}
	if gotHost != wantHost {
		return fmt.Errorf("--url must match configured new-api address http://%s", wantHost)
	}
	return nil
}

func mustJoinURL(baseURL string, requestPath string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	parsed.Path = requestPath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func readMockStats(source string) (artifact.MockStats, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		if err := loadtestclient.ValidateLoopbackURL(source); err != nil {
			return artifact.MockStats{}, err
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(source)
		if err != nil {
			return artifact.MockStats{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return artifact.MockStats{}, fmt.Errorf("mock stats status %d", resp.StatusCode)
		}
		var stats artifact.MockStats
		if err := common.DecodeJson(resp.Body, &stats); err != nil {
			return artifact.MockStats{}, err
		}
		return stats, nil
	}
	var stats artifact.MockStats
	if err := readJSON(source, &stats); err != nil {
		return artifact.MockStats{}, err
	}
	return stats, nil
}

func summaryExcerpt(summary artifact.Summary, delta artifact.MockStatsDelta) artifact.SummaryExcerpt {
	return artifact.SummaryExcerpt{Total: summary.Total, Success: summary.Success, Errors: summary.Errors, StatusCodes: summary.StatusCodes, LatencyP95MS: summary.LatencyP95MS, TTFTP95MS: summary.TTFTP95MS, RequestsPerSecond: summary.RequestsPerSecond, MaxObservedInFlight: summary.MaxObservedInFlight, StreamDoneReceived: boolCount(summary.Stream.DoneReceived, summary.Success), StreamUsageEvents: summary.Stream.UsageEvents, StreamBytes: summary.Stream.Bytes, Actual429: delta.Actual429, Actual502: delta.Actual502, UpstreamAttemptsTotal: delta.UpstreamAttemptsTotal, NonInjectedErrors: nonInjectedErrors(summary, delta)}
}

func boolCount(ok bool, value int) int {
	if ok {
		return value
	}
	return 0
}

func nonInjectedErrors(summary artifact.Summary, delta artifact.MockStatsDelta) int {
	injected := delta.Actual429 + delta.Actual502
	if summary.Errors > injected {
		return summary.Errors - injected
	}
	return 0
}

func gateOptions(opts runPointOptions) sweep.GateOptions {
	var statusRate map[int]float64
	var seed int64
	if opts.Config != nil && opts.MockProfile != "" {
		profile, ok := opts.Config.MockProfiles[opts.MockProfile]
		if ok {
			statusRate = profile.StatusRate
			seed = profile.Seed
		}
	}
	return sweep.GateOptions{MockOutputBytes: opts.OutputBytes, RequiredInvariantNames: sweep.RequiredInvariantNames(), Seed: seed, StatusRate: statusRate, RequireResourceSamples: opts.Scenario == "s3-long-stream" || opts.Scenario == "s5-large-payload"}
}

func sanitizeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "sweep"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	return b.String()
}

func parsePoints(raw string) []int {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && v > 0 {
			out = append(out, v)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return common.DecodeJson(f, v)
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

func writeErr(w io.Writer, err error) { _, _ = fmt.Fprintln(w, artifact.Redact(err.Error())) }
