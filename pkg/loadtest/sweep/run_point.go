package sweep

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/analysis"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestclient "github.com/QuantumNous/new-api/pkg/loadtest/client"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/metrics"
	"github.com/QuantumNous/new-api/pkg/loadtest/monitor"
	"github.com/QuantumNous/new-api/pkg/loadtest/resource"
	"gorm.io/gorm"
)

type RunPointOptions struct {
	Concurrency      int
	BaseURL          string
	RuntimeURL       string
	APIKey           string
	TokenProfile     string
	Path             string
	Model            string
	Scenario         string
	ArtifactDir      string
	RunContext       artifact.RunContext
	Config           *loadtestconfig.File
	MockProfile      string
	MockHash         string
	MockStats        string
	RequestsPerPoint int
	RPS              float64
	MaxRequests      int
	RampStep         int
	RampInterval     time.Duration
	Duration         time.Duration
	Timeout          time.Duration
	Transport        artifact.TransportProfile
	Seed             artifact.SeedOutput
	DB               *gorm.DB
	LogDB            *gorm.DB
	InputBytes       int
	StdoutLog        string
	StderrLog        string
	ServerPID        int
}

func RunPoint(ctx context.Context, opts RunPointOptions) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
	prefix := filepath.Join(opts.ArtifactDir, "points", fmt.Sprintf("c%d", opts.Concurrency))
	point := artifact.PointResult{Concurrency: opts.Concurrency, SummaryPath: prefix + "-summary.json", MetricsBeforePath: prefix + "-before.json", MetricsAfterPath: prefix + "-after.json", MetricsDiffPath: prefix + "-diff.json"}
	beforeMock, err := readMockStats(opts.MockStats)
	if err != nil {
		return failedRunPoint(point, opts, err)
	}
	beforePath := prefix + "-mock-stats-before.json"
	afterPath := prefix + "-mock-stats-after.json"
	deltaPath := prefix + "-mock-stats-delta.json"
	if err := writeJSONFile(beforePath, beforeMock); err != nil {
		return failedRunPoint(point, opts, err)
	}
	beforeBusiness, err := metrics.LoadBusinessSnapshot(opts.DB, opts.Seed)
	if err != nil {
		return failedRunPoint(point, opts, err)
	}
	beforeRuntime := monitor.ReadRuntimeSnapshot(ctx, opts.RuntimeURL)
	beforeProcess := resource.SampleProcess(opts.ServerPID)
	beforeSnapshot := artifact.Snapshot{SchemaVersion: artifact.SchemaVersion, RunContext: opts.RunContext, Business: beforeBusiness, Process: beforeProcess, Runtime: beforeRuntime, Logs: metrics.ScanServerLogs(readText(opts.StdoutLog), readText(opts.StderrLog))}
	if err := writeJSONFile(point.MetricsBeforePath, beforeSnapshot); err != nil {
		return failedRunPoint(point, opts, err)
	}
	drainBaseline, _ := metrics.BusinessDrainSampleWithLogDB(opts.DB, opts.LogDB, opts.TokenProfile)
	stopSampler := monitor.NewSampler(monitor.SamplerOptions{
		Interval: 200 * time.Millisecond,
		Process:  func() artifact.ProcessSnapshot { return resource.SampleProcess(opts.ServerPID) },
		Runtime:  func() artifact.RuntimeSnapshot { return monitor.ReadRuntimeSnapshot(ctx, opts.RuntimeURL) },
		Postgres: func() artifact.PostgresSnapshot {
			return monitor.LoadPostgresSnapshotWithLogDB(opts.DB, opts.LogDB, nil)
		},
		Redis: func() artifact.RedisSnapshot {
			if opts.Config == nil {
				return artifact.RedisSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "config is not provided"}}
			}
			return monitor.LoadRedisSnapshot(ctx, opts.Config.Redis.Addr)
		},
	}).Start()
	summary, err := loadtestclient.RunLoad(ctx, loadtestclient.Options{BaseURL: opts.BaseURL, APIKey: opts.APIKey, TokenProfile: opts.TokenProfile, Path: opts.Path, Model: opts.Model, Scenario: opts.Scenario, Concurrency: opts.Concurrency, RPS: opts.RPS, Duration: opts.Duration, MaxRequests: opts.MaxRequests, RampStep: opts.RampStep, RampInterval: opts.RampInterval, Timeout: opts.Timeout, InputBytes: opts.InputBytes, Stream: true, RunContext: opts.RunContext, Transport: loadtestclient.TransportOptions{Mode: opts.Transport.Mode, MaxConnsPerHost: opts.Transport.MaxConnsPerHost, MaxIdleConns: opts.Transport.MaxIdleConns, MaxIdleConnsPerHost: opts.Transport.MaxIdleConnsPerHost}})
	if err != nil {
		if writeErr := writeJSONFile(point.SummaryPath, summary); writeErr != nil {
			return failedRunPoint(point, opts, writeErr)
		}
		samples := resourceSamples(opts, stopSampler(), artifact.Statused{Status: "failed", Reason: err.Error()})
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		analysisResult := analysis.EvaluateBenchmarkPoint(analysis.Inputs{Point: point, Summary: summary, ResourceSamples: samples, MaxRequests: opts.MaxRequests})
		return point, analysisResult, samples, err
	}
	if err := writeJSONFile(point.SummaryPath, summary); err != nil {
		return failedRunPoint(point, opts, err)
	}
	drainCtx, cancelDrain := drainContext(ctx, opts)
	drainSamples, drainStatus := waitDrain(drainCtx, opts, drainBaseline, summary)
	cancelDrain()
	afterBusiness := beforeBusiness
	if len(drainSamples) > 0 {
		afterBusiness = businessSnapshotAfterDrain(opts, drainSamples[len(drainSamples)-1], beforeBusiness)
	}
	samples := resourceSamples(opts, stopSampler(), drainStatus)
	if err := writeJSONFile(prefix+"-resource-samples.json", samples); err != nil {
		return failedRunPoint(point, opts, err)
	}
	if err := writeJSONFile(prefix+"-resource-peaks.json", samples.Peaks); err != nil {
		return failedRunPoint(point, opts, err)
	}
	afterMock, err := readMockStats(opts.MockStats)
	if err != nil {
		return failedRunPoint(point, opts, err)
	}
	if err := writeJSONFile(afterPath, afterMock); err != nil {
		return failedRunPoint(point, opts, err)
	}
	delta, err := BuildMockStatsDelta(beforeMock, afterMock, opts.RunContext)
	if err != nil {
		point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
		return point, analysis.EvaluateBenchmarkPoint(analysis.Inputs{Point: point, Summary: summary, ResourceSamples: samples, MaxRequests: opts.MaxRequests}), samples, err
	}
	delta.Path = deltaPath
	delta.Hash = ""
	deltaHash, err := artifact.HashCanonical(delta)
	if err != nil {
		return failedRunPoint(point, opts, err)
	}
	delta.Hash = deltaHash
	if err := writeJSONFile(deltaPath, delta); err != nil {
		return failedRunPoint(point, opts, err)
	}
	if afterBusiness.Status != "ok" {
		var err error
		afterBusiness, err = metrics.LoadBusinessSnapshot(opts.DB, opts.Seed)
		if err != nil {
			return failedRunPoint(point, opts, err)
		}
	}
	afterRuntime := monitor.ReadRuntimeSnapshot(ctx, opts.RuntimeURL)
	afterProcess := resource.SampleProcess(opts.ServerPID)
	afterSnapshot := artifact.Snapshot{SchemaVersion: artifact.SchemaVersion, RunContext: opts.RunContext, Business: afterBusiness, Process: afterProcess, Runtime: afterRuntime, Logs: metrics.ScanServerLogs(readText(opts.StdoutLog), readText(opts.StderrLog))}
	if err := writeJSONFile(point.MetricsAfterPath, afterSnapshot); err != nil {
		return failedRunPoint(point, opts, err)
	}
	logRows, preRows, err := metrics.LoadBusinessRowsWithLogDB(opts.DB, opts.LogDB, summary)
	if err != nil {
		return failedRunPoint(point, opts, err)
	}
	diff, inv := metrics.BuildDiff(metrics.DiffInputs{Before: beforeSnapshot, After: afterSnapshot, Summary: summary, SeedOutput: opts.Seed, MockDelta: delta, RunContext: opts.RunContext, StdoutLog: readText(opts.StdoutLog), StderrLog: readText(opts.StderrLog), ConsumeLogRows: logRows, PreConsumeRows: preRows, BusinessBefore: beforeSnapshot.Business, BusinessAfter: afterSnapshot.Business})
	if err := writeJSONFile(point.MetricsDiffPath, diff); err != nil {
		return failedRunPoint(point, opts, err)
	}
	point.SummaryExcerpt = summaryExcerpt(summary, delta)
	point.MockDelta = delta
	point.Invariants = diff.BusinessDelta.Invariants
	point.Invariants = append(point.Invariants, inv)
	point.ResourceDelta = diff.ResourceDelta
	point.ResourcePeaks = samples.Peaks
	point.Gate = EvaluateGate(opts.Scenario, point, gateOptions(opts))
	point.Passed = point.Gate.Passed
	analysisResult := analysis.EvaluateBenchmarkPoint(analysis.Inputs{Point: point, Summary: summary, ResourceSamples: samples, BusinessDiff: diff, MaxRequests: opts.MaxRequests})
	if err := writeJSONFile(prefix+"-analysis.json", analysisResult); err != nil {
		return failedRunPoint(point, opts, err)
	}
	return point, analysisResult, samples, nil
}

func failedRunPoint(point artifact.PointResult, opts RunPointOptions, err error) (artifact.PointResult, artifact.PointAnalysis, artifact.ResourceSamplesArtifact, error) {
	point.Gate = artifact.GateResult{Passed: false, FailedReasons: []string{err.Error()}}
	samples := artifact.ResourceSamplesArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: opts.RunContext, Concurrency: opts.Concurrency, Drain: artifact.Statused{Status: "unavailable", Reason: "point failed before resource sampling completed"}}
	return point, analysis.EvaluateBenchmarkPoint(analysis.Inputs{Point: point, ResourceSamples: samples, MaxRequests: opts.MaxRequests}), samples, err
}

func drainContext(ctx context.Context, opts RunPointOptions) (context.Context, context.CancelFunc) {
	if opts.Timeout > 0 {
		return context.WithTimeout(ctx, opts.Timeout)
	}
	return ctx, func() {}
}

func waitDrain(ctx context.Context, opts RunPointOptions, baseline monitor.DrainSample, summary artifact.Summary) ([]monitor.DrainSample, artifact.Statused) {
	first := true
	return monitor.WaitDrain(ctx, 200*time.Millisecond, func() monitor.DrainSample {
		if first {
			first = false
			return baseline
		}
		sample, err := metrics.BusinessDrainSampleWithLogDB(opts.DB, opts.LogDB, opts.TokenProfile)
		if err != nil {
			return baseline
		}
		return sample
	}, monitor.DrainExpectations{Success: summary.Success, Tokens: int64(summary.Success * opts.Seed.ExpectedUsagePerSuccess.TotalTokens)})
}

func resourceSamples(opts RunPointOptions, samples []artifact.ResourceSample, drain artifact.Statused) artifact.ResourceSamplesArtifact {
	return artifact.ResourceSamplesArtifact{SchemaVersion: artifact.SchemaVersion, RunContext: opts.RunContext, Concurrency: opts.Concurrency, Samples: samples, Peaks: monitor.Peaks(samples), Drain: drain}
}

func businessSnapshotAfterDrain(opts RunPointOptions, sample monitor.DrainSample, fallback artifact.BusinessSnapshot) artifact.BusinessSnapshot {
	if fallback.Status != "ok" {
		return fallback
	}
	if opts.TokenProfile != "subscription" && opts.TokenProfile != "compat" {
		return fallback
	}
	snapshot := fallback
	if opts.TokenProfile == "subscription" {
		snapshot.SubscriptionTokenUsed = sample.SubscriptionTokenUsed
	} else {
		snapshot.CompatSubscriptionTokenUsed = sample.SubscriptionTokenUsed
	}
	return snapshot
}

func gateOptions(opts RunPointOptions) GateOptions {
	var statusRate map[int]float64
	var seed int64
	if opts.Config != nil && opts.MockProfile != "" {
		profile, ok := opts.Config.MockProfiles[opts.MockProfile]
		if ok {
			statusRate = profile.StatusRate
			seed = profile.Seed
		}
	}
	return GateOptions{MockOutputBytes: mockOutputBytes(opts), MaxRequests: opts.MaxRequests, RequiredInvariantNames: RequiredInvariantNames(), Seed: seed, StatusRate: statusRate, RequireResourceSamples: opts.Scenario == "benchmark" || opts.Scenario == "s3-long-stream" || opts.Scenario == "s5-large-payload"}
}

func mockOutputBytes(opts RunPointOptions) int {
	if opts.Config == nil || opts.MockProfile == "" {
		return 0
	}
	return opts.Config.MockProfiles[opts.MockProfile].OutputBytes
}

func readMockStats(source string) (artifact.MockStats, error) {
	if source == "" {
		return artifact.MockStats{}, fmt.Errorf("mock stats source is required")
	}
	if source[0] != 'h' {
		var stats artifact.MockStats
		if err := readJSON(source, &stats); err != nil {
			return artifact.MockStats{}, err
		}
		return stats, nil
	}
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

func summaryExcerpt(summary artifact.Summary, delta artifact.MockStatsDelta) artifact.SummaryExcerpt {
	return artifact.SummaryExcerpt{Total: summary.Total, Success: summary.Success, Errors: summary.Errors, StatusCodes: summary.StatusCodes, LatencyP95MS: summary.LatencyP95MS, TTFTP95MS: summary.TTFTP95MS, RequestsPerSecond: summary.RequestsPerSecond, MaxObservedInFlight: summary.MaxObservedInFlight, StreamDoneReceived: boolCount(summary.Stream.DoneReceived, summary.Success), StreamUsageEvents: summary.Stream.UsageEvents, StreamBytes: summary.Stream.Bytes, StopReason: summary.StopReason, Actual429: delta.Actual429, Actual502: delta.Actual502, UpstreamAttemptsTotal: delta.UpstreamAttemptsTotal, NonInjectedErrors: nonInjectedErrors(summary, delta)}
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

func readText(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
