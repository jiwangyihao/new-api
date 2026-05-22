package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/QuantumNous/new-api/pkg/loadtest/report"
)

func main() { os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr)) }

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("loadtest-report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sweepPath := fs.String("sweep", "", "sweep artifact")
	baselineSweep := fs.String("baseline-sweep", "", "baseline sweep")
	candidateSweep := fs.String("candidate-sweep", "", "candidate sweep")
	_ = fs.String("baseline-metrics", "", "baseline metrics")
	_ = fs.String("candidate-metrics", "", "candidate metrics")
	thresholdsPath := fs.String("thresholds", "", "thresholds json")
	outPath := fs.String("out", "", "markdown output")
	failOnRegression := fs.Bool("fail-on-regression", false, "fail on regression")
	resourceSweepPath := fs.String("resource-sweep", "", "resource sweep artifact")
	analysisDir := fs.String("analysis-dir", "", "point analysis directory")
	resourceLimitsPath := fs.String("resource-limits", "", "resource limits artifact")
	portsClosedPath := fs.String("ports-closed", "", "ports closed artifact")
	resourceSamplesDir := fs.String("resource-samples-dir", "", "resource samples directory")
	if err := fs.Parse(args); err != nil {
		writeErr(stderr, err)
		return 2
	}
	var md string
	if *resourceSweepPath != "" {
		if strings.TrimSpace(*resourceLimitsPath) == "" || strings.TrimSpace(*portsClosedPath) == "" {
			writeErr(stderr, fmt.Errorf("--resource-limits and --ports-closed are required with --resource-sweep"))
			return 2
		}
		resourceReport, err := loadResourceSweepReport(*resourceSweepPath, *analysisDir, *resourceSamplesDir, *resourceLimitsPath, *portsClosedPath)
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
		md = report.RenderResourceSweep(resourceReport)
	} else if *baselineSweep != "" || *candidateSweep != "" {
		var base, candidate artifact.SweepResult
		if err := readJSON(*baselineSweep, &base); err != nil {
			writeErr(stderr, err)
			return 1
		}
		if err := readJSON(*candidateSweep, &candidate); err != nil {
			writeErr(stderr, err)
			return 1
		}
		thresholds := report.Thresholds{}
		if *thresholdsPath != "" {
			if err := readJSON(*thresholdsPath, &thresholds); err != nil {
				writeErr(stderr, err)
				return 1
			}
		}
		cmp, err := report.BuildCompareReport(base, candidate, thresholds)
		if err != nil {
			var regression report.RegressionError
			if !errors.As(err, &regression) || *failOnRegression {
				writeErr(stderr, err)
				return 2
			}
			md = report.RenderCompareFailure(candidate, err)
		} else {
			md = cmp.Markdown
		}
	} else {
		if *sweepPath == "" {
			writeErr(stderr, fmt.Errorf("--sweep or --baseline-sweep/--candidate-sweep is required"))
			return 2
		}
		var sweep artifact.SweepResult
		if err := readJSON(*sweepPath, &sweep); err != nil {
			writeErr(stderr, err)
			return 1
		}
		md = report.RenderSingleReport(sweep, nil)
	}
	if *outPath == "" {
		_, err := io.WriteString(stdout, md)
		if err != nil {
			return 1
		}
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		writeErr(stderr, err)
		return 1
	}
	if err := os.WriteFile(*outPath, []byte(md), 0o600); err != nil {
		writeErr(stderr, err)
		return 1
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

func loadResourceSweepReport(sweepPath, analysisDir, samplesDir, limitsPath, portsPath string) (report.ResourceSweepReportInput, error) {
	var input report.ResourceSweepReportInput
	if err := readJSON(sweepPath, &input.Sweep); err != nil {
		return report.ResourceSweepReportInput{}, err
	}
	analyses, err := readPointAnalyses(analysisDir)
	if err != nil {
		return report.ResourceSweepReportInput{}, err
	}
	resources, err := readResourceSamples(samplesDir)
	if err != nil {
		return report.ResourceSweepReportInput{}, err
	}
	input.Analyses = analyses
	input.ResourceSamples = resources
	if strings.TrimSpace(limitsPath) == "" || strings.TrimSpace(portsPath) == "" {
		return report.ResourceSweepReportInput{}, fmt.Errorf("--resource-limits and --ports-closed are required with --resource-sweep")
	}
	if err := readJSON(limitsPath, &input.Limits); err != nil {
		return report.ResourceSweepReportInput{}, err
	}
	if err := readJSON(portsPath, &input.Ports); err != nil {
		return report.ResourceSweepReportInput{}, err
	}
	return input, nil
}

func readPointAnalyses(dir string) ([]artifact.PointAnalysis, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "c*-analysis.json"))
	if err != nil {
		return nil, err
	}
	out := make([]artifact.PointAnalysis, 0, len(paths))
	for _, path := range paths {
		var analysis artifact.PointAnalysis
		if err := readJSON(path, &analysis); err != nil {
			return nil, err
		}
		out = append(out, analysis)
	}
	return out, nil
}

func readResourceSamples(dir string) ([]artifact.ResourceSamplesArtifact, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "c*-resource-samples.json"))
	if err != nil {
		return nil, err
	}
	out := make([]artifact.ResourceSamplesArtifact, 0, len(paths))
	seen := make(map[int]int, len(paths))
	for _, path := range paths {
		var samples artifact.ResourceSamplesArtifact
		if err := readJSON(path, &samples); err != nil {
			return nil, err
		}
		seen[samples.Concurrency] = len(out)
		out = append(out, samples)
	}
	peakPaths, err := filepath.Glob(filepath.Join(dir, "c*-resource-peaks.json"))
	if err != nil {
		return nil, err
	}
	for _, path := range peakPaths {
		concurrency, ok := concurrencyFromResourcePath(path)
		if !ok {
			continue
		}
		var peaks artifact.ResourcePeaks
		if err := readJSON(path, &peaks); err != nil {
			return nil, err
		}
		if index, ok := seen[concurrency]; ok {
			out[index].Peaks = peaks
			continue
		}
		seen[concurrency] = len(out)
		out = append(out, artifact.ResourceSamplesArtifact{SchemaVersion: artifact.SchemaVersion, Concurrency: concurrency, Peaks: peaks})
	}
	return out, nil
}

func concurrencyFromResourcePath(path string) (int, bool) {
	name := filepath.Base(path)
	name = strings.TrimPrefix(name, "c")
	value, _, ok := strings.Cut(name, "-")
	if !ok {
		return 0, false
	}
	var concurrency int
	if _, err := fmt.Sscanf(value, "%d", &concurrency); err != nil || concurrency <= 0 {
		return 0, false
	}
	return concurrency, true
}

func writeErr(w io.Writer, err error) { _, _ = fmt.Fprintln(w, artifact.Redact(err.Error())) }
