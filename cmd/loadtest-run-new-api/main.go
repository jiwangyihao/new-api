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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/QuantumNous/new-api/pkg/loadtest/runner"
)

func main() { os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr)) }

const (
	statusWaitTimeout  = 30 * time.Second
	statusPollInterval = 200 * time.Millisecond
	statusRequestTTL   = 2 * time.Second
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("loadtest-run-new-api", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	binary := fs.String("binary", "", "new-api binary")
	envPath := fs.String("env", "", "env file")
	workDir := fs.String("work-dir", "", "work dir")
	pidFile := fs.String("pid-file", "", "pid file")
	stdoutLog := fs.String("stdout-log", "", "stdout log")
	stderrLog := fs.String("stderr-log", "", "stderr log")
	bootstrapOnly := fs.Bool("bootstrap-only", false, "bootstrap and stop")
	if err := fs.Parse(args); err != nil {
		writeErr(stderr, err)
		return 2
	}
	if *binary == "" || *envPath == "" || *workDir == "" || *pidFile == "" || *stdoutLog == "" || *stderrLog == "" {
		writeErr(stderr, fmt.Errorf("--binary, --env, --work-dir, --pid-file, --stdout-log and --stderr-log are required"))
		return 2
	}
	env, err := runner.ReadEnvFile(*envPath)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	statusURL, err := statusURLFromEnv(env)
	if err != nil {
		writeErr(stderr, err)
		return 2
	}
	if err := os.MkdirAll(*workDir, 0o755); err != nil {
		writeErr(stderr, err)
		return 1
	}
	cmd, err := runner.BuildCommand(runner.Config{Binary: *binary, WorkDir: *workDir, Env: env, PIDFile: *pidFile, StdoutLog: *stdoutLog, StderrLog: *stderrLog})
	if err != nil {
		writeErr(stderr, err)
		return 2
	}
	out, err := openLog(*stdoutLog)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer out.Close()
	errOut, err := openLog(*stderrLog)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer errOut.Close()
	cmd.Stdout = out
	cmd.Stderr = errOut
	running := false
	released := false
	defer func() {
		if running && !released {
			terminateProcess(cmd)
		}
	}()
	if err := cmd.Start(); err != nil {
		writeErr(stderr, err)
		return 1
	}
	running = true
	pid := cmd.Process.Pid
	if err := os.MkdirAll(filepath.Dir(*pidFile), 0o755); err != nil {
		writeErr(stderr, err)
		return 1
	}
	if err := os.WriteFile(*pidFile, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		writeErr(stderr, err)
		return 1
	}
	healthCtx, cancel := context.WithTimeout(context.Background(), statusWaitTimeout)
	defer cancel()
	client := &http.Client{Timeout: statusRequestTTL}
	if err := waitForStatus(healthCtx, client, statusURL, processProbe(pid)); err != nil {
		writeErr(stderr, err)
		return 1
	}
	if *bootstrapOnly {
		terminateProcess(cmd)
		running = false
		fmt.Fprintln(stdout, "new-api bootstrap completed")
		return 0
	}
	if err := cmd.Process.Release(); err != nil {
		writeErr(stderr, err)
		return 1
	}
	released = true
	running = false
	fmt.Fprintf(stdout, "new-api started pid=%d\n", pid)
	return 0
}

func statusURLFromEnv(env map[string]string) (string, error) {
	host := strings.TrimSpace(env["HOST"])
	port := strings.TrimSpace(env["PORT"])
	if host == "" {
		return "", fmt.Errorf("HOST is required")
	}
	if port == "" {
		return "", fmt.Errorf("PORT is required")
	}
	if err := validateLoopbackHost(host); err != nil {
		return "", fmt.Errorf("HOST %q is not allowed: %w", host, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("PORT must be numeric: %w", err)
	}
	if portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("PORT must be between 1 and 65535")
	}
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: "/api/status"}
	return u.String(), nil
}

func validateLoopbackHost(host string) error {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("host must be a loopback IP address or localhost")
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("host is not loopback")
	}
	return nil
}

func waitForStatus(ctx context.Context, client *http.Client, statusURL string, processDone func() error) error {
	if client == nil {
		client = http.DefaultClient
	}
	ticker := time.NewTicker(statusPollInterval)
	defer ticker.Stop()
	var lastErr error
	for {
		if err := checkProcessDone(processDone); err != nil {
			return err
		}
		ok, err := probeStatus(ctx, client, statusURL)
		if ok {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("waiting for %s: %w", statusURL, lastErr)
			}
			return fmt.Errorf("waiting for %s: %w", statusURL, ctx.Err())
		case <-ticker.C:
		}
	}
}

func probeStatus(ctx context.Context, client *http.Client, statusURL string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return true, nil
	}
	return false, fmt.Errorf("%s returned HTTP %d", statusURL, resp.StatusCode)
}

func processProbe(pid int) func() error {
	if pid <= 0 || runtime.GOOS == "windows" {
		return nil
	}
	return func() error {
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		defer process.Release()
		return process.Signal(syscall.Signal(0))
	}
}

func checkProcessDone(processDone func() error) error {
	if processDone == nil {
		return nil
	}
	if err := processDone(); err != nil {
		return processExitError(err)
	}
	return nil
}

func processExitError(err error) error {
	if err == nil {
		return fmt.Errorf("new-api exited before /api/status became healthy")
	}
	return fmt.Errorf("new-api exited before /api/status became healthy: %w", err)
}

func terminateProcess(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.Process != nil {
		_ = terminateProcessTree(cmd.Process.Pid)
	}
	_ = cmd.Wait()
}

func terminateProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	defer process.Release()
	return process.Kill()
}

func openLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

func writeErr(w io.Writer, err error) { _, _ = fmt.Fprintln(w, artifact.Redact(err.Error())) }
