package report

import (
	"fmt"
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
