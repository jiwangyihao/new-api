package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
	if err := fs.Parse(args); err != nil {
		writeErr(stderr, err)
		return 2
	}
	var md string
	if *baselineSweep != "" || *candidateSweep != "" {
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
			writeErr(stderr, err)
			_ = failOnRegression
			return 2
		}
		md = cmp.Markdown
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

func writeErr(w io.Writer, err error) { _, _ = fmt.Fprintln(w, artifact.Redact(err.Error())) }
