package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadclient "github.com/QuantumNous/new-api/pkg/loadtest/client"
)

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("loadtest-client", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts loadclient.Options
	var healthOpts loadclient.HealthCheckOptions
	var healthCheck bool
	var runContextPath string
	var outPath string
	var duration string
	var rampInterval string
	var timeoutValue string

	fs.StringVar(&opts.BaseURL, "url", "", "loopback new-api base URL")
	fs.StringVar(&opts.APIKey, "api-key", "", "loadtest API key")
	fs.StringVar(&opts.TokenProfile, "token-profile", "", "subscription, compat, or invalid")
	fs.StringVar(&opts.Path, "path", loadclient.DefaultPath, "relay path")
	fs.StringVar(&opts.Model, "model", loadclient.DefaultModel, "model")
	fs.StringVar(&opts.Scenario, "scenario", "", "scenario label")
	fs.IntVar(&opts.Concurrency, "concurrency", 1, "target concurrency")
	fs.Float64Var(&opts.RPS, "rps", 0, "requests per second; 0 means unlimited")
	fs.StringVar(&duration, "duration", "0", "load duration")
	fs.IntVar(&opts.MaxRequests, "max-requests", 0, "max requests")
	fs.IntVar(&opts.RampStep, "ramp-step", 0, "concurrency added per ramp interval")
	fs.StringVar(&rampInterval, "ramp-interval", "0", "ramp interval")
	fs.StringVar(&timeoutValue, "timeout", "30s", "single request timeout")
	fs.IntVar(&opts.InputBytes, "input-bytes", 0, "input bytes")
	fs.BoolVar(&opts.Stream, "stream", true, "request stream responses")
	fs.StringVar(&runContextPath, "run-context", "", "run context JSON path")
	fs.StringVar(&outPath, "out", "", "output JSON path")

	fs.BoolVar(&healthCheck, "health-check", false, "run S0 health check")
	fs.StringVar(&healthOpts.ValidAPIKey, "valid-api-key", "", "valid loadtest API key for S0")
	fs.StringVar(&healthOpts.InvalidAPIKey, "invalid-api-key", "sk-loadtestinvalid", "invalid loadtest API key for S0")
	fs.StringVar(&healthOpts.RuntimeURL, "runtime-url", "", "runtime stats URL")
	fs.StringVar(&healthOpts.PprofURL, "pprof-url", "", "goroutine pprof URL")

	if err := fs.Parse(args); err != nil {
		writeErr(stderr, err)
		return 2
	}
	if fs.NArg() != 0 {
		writeErr(stderr, fmt.Errorf("unexpected positional arguments"))
		return 2
	}

	parsedDuration, err := parseOptionalDuration(duration)
	if err != nil {
		writeErr(stderr, fmt.Errorf("invalid --duration: %w", err))
		return 2
	}
	parsedRampInterval, err := parseOptionalDuration(rampInterval)
	if err != nil {
		writeErr(stderr, fmt.Errorf("invalid --ramp-interval: %w", err))
		return 2
	}
	parsedTimeout, err := time.ParseDuration(timeoutValue)
	if err != nil || parsedTimeout <= 0 {
		if err == nil {
			err = fmt.Errorf("must be greater than zero")
		}
		writeErr(stderr, fmt.Errorf("invalid --timeout: %w", err))
		return 2
	}

	client := &http.Client{}
	if healthCheck {
		healthOpts.BaseURL = opts.BaseURL
		if healthOpts.ValidAPIKey == "" {
			healthOpts.ValidAPIKey = opts.APIKey
		}
		healthOpts.Timeout = parsedTimeout
		healthOpts.HTTPClient = client
		result, err := loadclient.HealthCheck(fsContext(), healthOpts)
		if err != nil {
			writeErr(stderr, err)
			return exitCode(err)
		}
		if err := writeJSON(outPath, stdout, result); err != nil {
			writeErr(stderr, err)
			return 1
		}
		if !result.Passed {
			writeErr(stderr, fmt.Errorf("health check failed"))
			if loadclient.HealthResultHasRuntimeFailure(result) {
				return 1
			}
			return 2
		}
		return 0
	}

	opts.Duration = parsedDuration
	opts.RampInterval = parsedRampInterval
	opts.Timeout = parsedTimeout
	opts.HTTPClient = client
	if runContextPath != "" {
		if err := readRunContext(runContextPath, &opts.RunContext); err != nil {
			writeErr(stderr, err)
			return exitCode(err)
		}
	}

	summary, err := loadclient.RunLoad(fsContext(), opts)
	if err != nil {
		writeErr(stderr, err)
		return exitCode(err)
	}
	if err := writeJSON(outPath, stdout, summary); err != nil {
		writeErr(stderr, err)
		return 1
	}
	return 0
}

func fsContext() context.Context {
	return context.Background()
}

func parseOptionalDuration(value string) (time.Duration, error) {
	if value == "" || value == "0" {
		return 0, nil
	}
	return time.ParseDuration(value)
}

func readRunContext(path string, runContext *artifact.RunContext) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read run context: %w", err)
	}
	defer file.Close()
	if err := common.DecodeJson(file, runContext); err != nil {
		return &loadclient.ConfigError{Err: fmt.Errorf("decode run context: %w", err)}
	}
	return nil
}

func writeJSON(path string, stdout io.Writer, value any) error {
	payload, err := common.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal output: %w", err)
	}
	payload = append(payload, '\n')
	if path == "" {
		_, err = stdout.Write(payload)
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	defer file.Close()
	_, err = file.Write(payload)
	if err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func writeErr(stderr io.Writer, err error) {
	_, _ = fmt.Fprintln(stderr, artifact.Redact(err.Error()))
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if loadclient.IsConfigError(err) {
		return 2
	}
	if loadclient.IsRuntimeError(err) {
		return 1
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return 1
	}
	return 1
}
