package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
	"github.com/QuantumNous/new-api/pkg/loadtest/localguard"
	"github.com/QuantumNous/new-api/pkg/loadtest/mockopenai"
)

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	cfg, addr, err := parseArgs(args, stderr)
	if err != nil {
		writeErr(stderr, err)
		return 2
	}
	if err := localguard.ValidateListenAddr(addr); err != nil {
		writeErr(stderr, fmt.Errorf("unsafe --addr: %w", err))
		return 2
	}
	if cfg.RunContext.MockHash != "" {
		if err := mockopenai.ValidateRunContextHash(cfg.RunContext, cfg); err != nil {
			writeErr(stderr, err)
			return 2
		}
	} else {
		writeErr(stderr, errors.New("run_context.mock_hash is required"))
		return 2
	}

	handler := mockopenai.NewServer(cfg)
	srv, ok := handler.(*mockopenai.Server)
	if !ok {
		writeErr(stderr, errors.New("mock server has unexpected type"))
		return 1
	}
	if err := srv.WriteStats(); err != nil {
		writeErr(stderr, err)
		return 1
	}

	server := &http.Server{Addr: addr, Handler: handler}
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	_, _ = fmt.Fprintf(stdout, "loadtest mock OpenAI listening on %s\n", addr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			writeErr(stderr, err)
			return 1
		}
		if err := <-errCh; err != nil {
			writeErr(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "loadtest mock OpenAI stopped: %s\n", sig)
		return 0
	case err := <-errCh:
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
		return 0
	}
}

func parseArgs(args []string, stderr io.Writer) (mockopenai.Config, string, error) {
	fs := flag.NewFlagSet("loadtest-mock-openai", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var addr string
	var runContextPath string
	var firstTokenDelay durationFlag
	var streamDuration durationFlag
	var chunkInterval durationFlag
	var outputBytes int
	var promptTokens int
	var completionTokens int
	var statusRate statusRateFlag
	var seed int64
	var statsOut string

	fs.StringVar(&addr, "addr", "127.0.0.1:19080", "loopback listen address")
	fs.StringVar(&runContextPath, "run-context", "", "run context JSON path")
	fs.Var(&firstTokenDelay, "first-token-delay", "first token delay")
	fs.Var(&streamDuration, "stream-duration", "stream duration")
	fs.Var(&chunkInterval, "chunk-interval", "chunk interval")
	fs.IntVar(&outputBytes, "output-bytes", 128, "output delta bytes")
	fs.IntVar(&promptTokens, "prompt-tokens", 11, "prompt tokens")
	fs.IntVar(&completionTokens, "completion-tokens", 17, "completion tokens")
	fs.Var(&statusRate, "status-rate", "comma-separated status=rate pairs")
	fs.Int64Var(&seed, "seed", 1, "deterministic error seed")
	fs.StringVar(&statsOut, "stats-out", "", "stats artifact output path")

	if err := fs.Parse(args); err != nil {
		return mockopenai.Config{}, "", err
	}
	if runContextPath == "" {
		return mockopenai.Config{}, "", errors.New("--run-context is required")
	}
	if outputBytes < 0 || promptTokens < 0 || completionTokens < 0 {
		return mockopenai.Config{}, "", errors.New("token and byte counts must be non-negative")
	}
	if !firstTokenDelay.set {
		firstTokenDelay.value = 0
	}
	if !streamDuration.set {
		streamDuration.value = 0
	}
	if !chunkInterval.set {
		chunkInterval.value = 0
	}
	if statusRate.values == nil {
		statusRate.values = map[int]float64{}
	}

	rc, err := readRunContext(runContextPath)
	if err != nil {
		return mockopenai.Config{}, "", err
	}
	cfg := mockopenai.Config{RunContext: rc, FirstTokenDelay: firstTokenDelay.value, StreamDuration: streamDuration.value, ChunkInterval: chunkInterval.value, OutputBytes: outputBytes, PromptTokens: promptTokens, CompletionTokens: completionTokens, StatusRate: statusRate.values, Seed: seed, StatsOut: statsOut}
	return cfg, addr, nil
}

func readRunContext(path string) (artifact.RunContext, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return artifact.RunContext{}, err
	}
	var wrapped struct {
		RunContext artifact.RunContext `json:"run_context"`
	}
	if err := common.Unmarshal(b, &wrapped); err != nil {
		return artifact.RunContext{}, err
	}
	if wrapped.RunContext.SchemaVersion != 0 || wrapped.RunContext.Commit != "" || wrapped.RunContext.ComparisonConfigHash != "" {
		return wrapped.RunContext, nil
	}
	var rc artifact.RunContext
	if err := common.Unmarshal(b, &rc); err != nil {
		return artifact.RunContext{}, err
	}
	if rc.SchemaVersion == 0 && rc.Commit == "" && rc.ComparisonConfigHash == "" {
		return artifact.RunContext{}, errors.New("run context file does not contain run_context")
	}
	return rc, nil
}

type durationFlag struct {
	value time.Duration
	set   bool
}

func (f *durationFlag) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	if v < 0 {
		return errors.New("duration must be non-negative")
	}
	f.value = v
	f.set = true
	return nil
}

func (f *durationFlag) String() string {
	return f.value.String()
}

type statusRateFlag struct {
	values map[int]float64
}

func (f *statusRateFlag) Set(s string) error {
	values, err := parseStatusRate(s)
	if err != nil {
		return err
	}
	f.values = values
	return nil
}

func (f *statusRateFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(f.values))
	for status, rate := range f.values {
		parts = append(parts, fmt.Sprintf("%d=%g", status, rate))
	}
	return strings.Join(parts, ",")
}

func parseStatusRate(raw string) (map[int]float64, error) {
	out := map[int]float64{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("invalid status-rate %q", part)
		}
		status, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil {
			return nil, fmt.Errorf("invalid status %q", key)
		}
		rate, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid rate %q", value)
		}
		if rate < 0 || rate > 1 {
			return nil, fmt.Errorf("status %d rate must be between 0 and 1", status)
		}
		out[status] = rate
	}
	return out, nil
}

func writeErr(stderr io.Writer, err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintln(stderr, artifact.Redact(err.Error()))
}

var _ = loadtestconfig.DeterministicErrorCounts
