package runner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/pkg/loadtest/localguard"
)

type Config struct {
	Binary    string
	WorkDir   string
	Env       map[string]string
	PIDFile   string
	StdoutLog string
	StderrLog string
}

var allowedEnvKeys = map[string]struct{}{
	"HOST": {}, "PORT": {}, "PPROF_ADDR": {}, "SQL_DSN": {}, "LOG_SQL_DSN": {}, "REDIS_CONN_STRING": {}, "ENABLE_PPROF": {}, "LOADTEST_RUNTIME_STATS_ENABLED": {}, "LOADTEST_PROFILE_BLOCK_RATE": {}, "LOADTEST_PROFILE_MUTEX_FRACTION": {}, "GOMAXPROCS": {}, "GOGC": {}, "BATCH_UPDATE_ENABLED": {}, "SQL_MAX_OPEN_CONNS": {}, "SQL_MAX_IDLE_CONNS": {}, "SQL_MAX_LIFETIME": {}, "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED": {}, "CHANNEL_UPDATE_FREQUENCY": {}, "UPDATE_TASK": {}, "CHANNEL_TEST_FREQUENCY": {}, "PYROSCOPE_URL": {}, "SYNC_UPSTREAM_BASE": {}, "RetryTimes": {}, "AutomaticRetryStatusCodes": {}, "MEMORY_CACHE_ENABLED": {}, "RELAY_MAX_IDLE_CONNS": {}, "RELAY_MAX_IDLE_CONNS_PER_HOST": {},
}

func BuildCommand(cfg Config) (*exec.Cmd, error) {
	if strings.TrimSpace(cfg.Binary) == "" || strings.TrimSpace(cfg.WorkDir) == "" {
		return nil, fmt.Errorf("binary and work-dir are required")
	}
	if !filepath.IsAbs(cfg.Binary) {
		return nil, fmt.Errorf("binary must be an absolute path")
	}
	if _, err := os.Stat(filepath.Join(cfg.WorkDir, ".env")); err == nil {
		return nil, fmt.Errorf("work-dir .env is not allowed")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := validateEnv(cfg.Env); err != nil {
		return nil, err
	}
	cmd := exec.Command(cfg.Binary)
	cmd.Dir = cfg.WorkDir
	cmd.Env = envSlice(cfg.Env)
	return cmd, nil
}

func ReadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	env := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid env line %q", line)
		}
		env[strings.TrimSpace(key)] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return env, nil
}

func validateEnv(env map[string]string) error {
	for key := range env {
		if _, ok := allowedEnvKeys[key]; !ok {
			return fmt.Errorf("env key %s is not allowed", key)
		}
	}
	if err := localguard.ValidatePostgresDSN(env["SQL_DSN"]); err != nil {
		return fmt.Errorf("SQL_DSN: %w", err)
	}
	if logDSN := env["LOG_SQL_DSN"]; logDSN != "" {
		if err := localguard.ValidatePostgresDSN(logDSN); err != nil {
			return fmt.Errorf("LOG_SQL_DSN: %w", err)
		}
	}
	if err := localguard.ValidateRedisAddr(env["REDIS_CONN_STRING"]); err != nil {
		return fmt.Errorf("REDIS_CONN_STRING: %w", err)
	}
	if err := localguard.ValidateListenAddr(env["HOST"] + ":" + env["PORT"]); err != nil {
		return fmt.Errorf("HOST/PORT: %w", err)
	}
	if err := localguard.ValidateListenAddr(env["PPROF_ADDR"]); err != nil {
		return fmt.Errorf("PPROF_ADDR: %w", err)
	}
	if env["PYROSCOPE_URL"] != "" || env["SYNC_UPSTREAM_BASE"] != "" {
		return fmt.Errorf("external sync/profile URLs must be empty")
	}
	for key, want := range map[string]string{
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED": "false",
		"CHANNEL_UPDATE_FREQUENCY":                   "0",
		"UPDATE_TASK":                                "false",
		"CHANNEL_TEST_FREQUENCY":                     "0",
		"RetryTimes":                                 "0",
		"AutomaticRetryStatusCodes":                  "",
		"RELAY_MAX_IDLE_CONNS":                       "64",
		"RELAY_MAX_IDLE_CONNS_PER_HOST":              "16",
	} {
		if env[key] != want {
			return fmt.Errorf("%s must be %q", key, want)
		}
	}
	return nil
}

func envSlice(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}
