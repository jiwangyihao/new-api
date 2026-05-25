package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

type Thresholds struct {
	LatencyP95RegressionRatio float64 `json:"latency_p95_regression_ratio"`
	TTFTP95RegressionRatio    float64 `json:"ttft_p95_regression_ratio"`
}

type CompareReport struct {
	Markdown string
}

type ResourceSweepReportInput struct {
	Sweep           artifact.SweepResult
	Analyses        []artifact.PointAnalysis
	ResourceSamples []artifact.ResourceSamplesArtifact
	Limits          artifact.ResourceLimitsArtifact
	Ports           artifact.PortsClosedArtifact
}

type RegressionError struct {
	Reason string
}

func (e RegressionError) Error() string { return e.Reason }

func BuildCompareReport(baseline artifact.SweepResult, candidate artifact.SweepResult, thresholds Thresholds) (CompareReport, error) {

	if err := compareContexts(baseline.RunContext, candidate.RunContext); err != nil {
		return CompareReport{}, err
	}
	if thresholds.LatencyP95RegressionRatio > 0 {
		base := firstPointLatency(baseline)
		cand := firstPointLatency(candidate)
		if base > 0 && cand > base*thresholds.LatencyP95RegressionRatio {
			return CompareReport{}, RegressionError{Reason: fmt.Sprintf("latency p95 regression: baseline %.2f candidate %.2f", base, cand)}
		}
	}
	return CompareReport{Markdown: RenderSingleReport(candidate, nil)}, nil
}

func RenderCompareFailure(candidate artifact.SweepResult, err error) string {
	var b strings.Builder
	b.WriteString(RenderSingleReport(candidate, nil))
	b.WriteString("\nComparison: failed")
	if err != nil {
		b.WriteString(" - ")
		b.WriteString(err.Error())
	}
	b.WriteByte('\n')
	return b.String()
}

func RenderSingleReport(sweep artifact.SweepResult, diffs []artifact.Diff) string {
	var b strings.Builder
	b.WriteString("# Loadtest Report\n\n")
	b.WriteString(fmt.Sprintf("- scenario: %s\n", sweep.RunContext.Scenario))
	b.WriteString(fmt.Sprintf("- highest_passed_concurrency: %d\n", sweep.HighestPassedConcurrency))
	if sweep.FirstFailedConcurrency != nil {
		b.WriteString(fmt.Sprintf("- first_failed_concurrency: %d\n", *sweep.FirstFailedConcurrency))
	} else {
		b.WriteString("- first_failed_concurrency: none\n")
	}
	b.WriteString("\n| concurrency | passed | latency_p95_ms | ttft_p95_ms |\n")
	b.WriteString("|---:|:---:|---:|---:|\n")
	for _, point := range sweep.Points {
		b.WriteString(fmt.Sprintf("| %d | %t | %.2f | %.2f |\n", point.Concurrency, point.Passed, point.SummaryExcerpt.LatencyP95MS, point.SummaryExcerpt.TTFTP95MS))
	}
	for _, diff := range diffs {
		if diff.RuntimeDelta.Status == "unavailable" {
			b.WriteString("\nRuntime: unavailable")
			if diff.RuntimeDelta.Reason != "" {
				b.WriteString(" - ")
				b.WriteString(diff.RuntimeDelta.Reason)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func RenderResourceSweep(input ResourceSweepReportInput) string {
	analysisByConcurrency := make(map[int]artifact.PointAnalysis, len(input.Analyses))
	for _, analysis := range input.Analyses {
		analysisByConcurrency[analysis.Concurrency] = analysis
	}
	resourceByConcurrency := make(map[int]artifact.ResourceSamplesArtifact, len(input.ResourceSamples))
	for _, samples := range input.ResourceSamples {
		resourceByConcurrency[samples.Concurrency] = samples
	}

	var b strings.Builder
	b.WriteString("# Resource Sweep Report\n\n")
	b.WriteString(fmt.Sprintf("- scenario: %s\n", input.Sweep.RunContext.Scenario))
	b.WriteString(fmt.Sprintf("- path: %s\n", input.Sweep.RunContext.Path))
	b.WriteString(fmt.Sprintf("- token_profile: %s\n", input.Sweep.RunContext.TokenProfile))
	b.WriteString(fmt.Sprintf("- 最高通过并发: %d\n", input.Sweep.HighestPassedConcurrency))
	if input.Sweep.FirstFailedConcurrency != nil {
		b.WriteString(fmt.Sprintf("- 第一失败并发: %d\n", *input.Sweep.FirstFailedConcurrency))
	} else {
		b.WriteString("- 第一失败并发: none\n")
	}

	b.WriteString("\n## Resource Limits\n\n")
	if input.Limits.Status != "" && input.Limits.Status != "ok" {
		b.WriteString(statusLine("limits", input.Limits.Statused))
	}
	writeServerLimits(&b, input.Limits)

	b.WriteString("\n## Points\n\n")
	b.WriteString("| concurrency | passed | failure_class | RSS peak | CPU peak | runtime heap | Redis used_memory | PostgreSQL active |\n")
	b.WriteString("|---:|:---:|---|---:|---:|---:|---:|---:|\n")
	for _, point := range input.Sweep.Points {
		analysis := analysisByConcurrency[point.Concurrency]
		failureClass := analysis.FailureClass
		if failureClass == "" && point.Passed {
			failureClass = "passed"
		}
		peaks := point.ResourcePeaks
		if samples, ok := resourceByConcurrency[point.Concurrency]; ok {
			peaks = samples.Peaks
		}
		b.WriteString(fmt.Sprintf("| %d | %t | %s | %s | %s | %s | %s | %s |\n",
			point.Concurrency,
			point.Passed,
			valueOrUnavailable(failureClass),
			bytesOrUnavailable(peaks.RSSPeakBytes),
			floatOrUnavailable(peaks.CPUPercentPeak),
			bytesOrUnavailable(peaks.HeapAllocPeakBytes),
			bytesOrUnavailable(peaks.RedisUsedMemoryPeakBytes),
			intOrUnavailable(peaks.PostgresActiveConnectionsPeak),
		))
	}

	b.WriteString("\n## Resource Status\n\n")
	for _, samples := range input.ResourceSamples {
		if samples.Drain.Status != "" && samples.Drain.Status != "passed" && samples.Drain.Status != "ok" {
			b.WriteString(fmt.Sprintf("- c%d drain: %s", samples.Concurrency, samples.Drain.Status))
			if samples.Drain.Reason != "" {
				b.WriteString(" - ")
				b.WriteString(samples.Drain.Reason)
			}
			b.WriteByte('\n')
		}
		if len(samples.Samples) == 0 && samples.Peaks == (artifact.ResourcePeaks{}) {
			b.WriteString(fmt.Sprintf("- c%d resources: unavailable\n", samples.Concurrency))
		}
	}

	b.WriteString("\n## Ports\n\n")
	if input.Ports.Passed {
		b.WriteString("- ports closed: true\n")
	} else {
		b.WriteString("- ports closed: false\n")
	}
	for _, port := range sortedPortKeys(input.Ports.Ports) {
		b.WriteString(fmt.Sprintf("  - %s: %s\n", port, input.Ports.Ports[port]))
	}
	return b.String()
}

func writeServerLimits(b *strings.Builder, limits artifact.ResourceLimitsArtifact) {
	gomem := limits.ServerEnv["GOMEMLIMIT"]
	if gomem != "" {
		b.WriteString(fmt.Sprintf("- GOMEMLIMIT=%s\n", gomem))
	}
	for _, key := range []string{"SQL_MAX_OPEN_CONNS", "SQL_MAX_IDLE_CONNS", "REDIS_POOL_SIZE", "REDIS_IDLE_TIMEOUT_SECONDS", "RELAY_MAX_IDLE_CONNS", "RELAY_MAX_IDLE_CONNS_PER_HOST"} {
		if value := limits.ServerEnv[key]; value != "" {
			b.WriteString(fmt.Sprintf("- %s=%s\n", key, value))
		}
	}
	if limits.ServerProcessMemoryLimitBytes > 0 {
		b.WriteString(fmt.Sprintf("- process_memory_limit_bytes: %d\n", limits.ServerProcessMemoryLimitBytes))
	}
	if limits.ServerCPUAffinityCores > 0 {
		b.WriteString(fmt.Sprintf("- cpu_affinity_cores: %d\n", limits.ServerCPUAffinityCores))
	}
}

func statusLine(name string, status artifact.Statused) string {
	if status.Reason == "" {
		return fmt.Sprintf("- %s: %s\n", name, status.Status)
	}
	return fmt.Sprintf("- %s: %s - %s\n", name, status.Status, status.Reason)
}

func valueOrUnavailable(value string) string {
	if value == "" {
		return "unavailable"
	}
	return value
}

func bytesOrUnavailable(value uint64) string {
	if value == 0 {
		return "unavailable"
	}
	return fmt.Sprintf("%d", value)
}

func intOrUnavailable(value int) string {
	if value == 0 {
		return "unavailable"
	}
	return fmt.Sprintf("%d", value)
}

func floatOrUnavailable(value float64) string {
	if value == 0 {
		return "unavailable"
	}
	return fmt.Sprintf("%.2f", value)
}

func sortedPortKeys(ports map[string]string) []string {
	keys := make([]string, 0, len(ports))
	for key := range ports {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func compareContexts(a, b artifact.RunContext) error {
	checks := map[string][2]string{
		"comparison_config_hash": {a.ComparisonConfigHash, b.ComparisonConfigHash},
		"mock_hash":              {a.MockHash, b.MockHash},
		"cache_mode":             {a.CacheMode, b.CacheMode},
		"scenario":               {a.Scenario, b.Scenario},
		"path":                   {a.Path, b.Path},
		"token_profile":          {a.TokenProfile, b.TokenProfile},
	}
	for name, pair := range checks {
		if pair[0] != pair[1] {
			return fmt.Errorf("%s mismatch", name)
		}
	}
	return nil
}

func firstPointLatency(sweep artifact.SweepResult) float64 {
	if len(sweep.Points) == 0 {
		return 0
	}
	return sweep.Points[0].SummaryExcerpt.LatencyP95MS
}
