package runner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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

type ExpectedLimits struct {
	RelayMaxIdleConns        string
	RelayMaxIdleConnsPerHost string
	GOMEMLIMIT               string
}

var allowedEnvKeys = map[string]struct{}{
	"HOST": {}, "PORT": {}, "PPROF_ADDR": {}, "SQL_DSN": {}, "LOG_SQL_DSN": {}, "REDIS_CONN_STRING": {}, "REDIS_POOL_SIZE": {}, "ENABLE_PPROF": {}, "LOADTEST_RUNTIME_STATS_ENABLED": {}, "LOADTEST_PROFILE_BLOCK_RATE": {}, "LOADTEST_PROFILE_MUTEX_FRACTION": {}, "GOMAXPROCS": {}, "GOGC": {}, "GOMEMLIMIT": {}, "NODE_TYPE": {}, "BATCH_UPDATE_ENABLED": {}, "BATCH_UPDATE_INTERVAL": {}, "SQL_MAX_OPEN_CONNS": {}, "SQL_MAX_IDLE_CONNS": {}, "SQL_MAX_LIFETIME": {}, "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED": {}, "CHANNEL_UPDATE_FREQUENCY": {}, "UPDATE_TASK": {}, "CHANNEL_TEST_FREQUENCY": {}, "PYROSCOPE_URL": {}, "SYNC_UPSTREAM_BASE": {}, "RetryTimes": {}, "AutomaticRetryStatusCodes": {}, "MEMORY_CACHE_ENABLED": {}, "RELAY_MAX_IDLE_CONNS": {}, "RELAY_MAX_IDLE_CONNS_PER_HOST": {},
}

func BuildCommand(cfg Config) (*exec.Cmd, error) {
	return BuildCommandWithExpectedLimits(cfg, ExpectedLimits{RelayMaxIdleConns: "64", RelayMaxIdleConnsPerHost: "16", GOMEMLIMIT: "384MiB"})
}

func BuildCommandWithExpectedLimits(cfg Config, expected ExpectedLimits) (*exec.Cmd, error) {
	if strings.TrimSpace(cfg.Binary) == "" || strings.TrimSpace(cfg.WorkDir) == "" {
		return nil, fmt.Errorf("binary and work-dir are required")
	}
	if !filepath.IsAbs(cfg.Binary) {
		return nil, fmt.Errorf("binary must be an absolute path")
	}
	if err := localguard.ValidateCleanWorkDir(cfg.WorkDir); err != nil {
		return nil, err
	}
	if err := validateExpectedLimits(expected); err != nil {
		return nil, err
	}
	if err := validateEnv(cfg.Env, expected); err != nil {
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

func validateEnv(env map[string]string, expected ExpectedLimits) error {
	for key := range env {
		if _, ok := allowedEnvKeys[key]; !ok {
			return fmt.Errorf("env contains a disallowed key")
		}
	}
	if err := localguard.ValidateCleanEnv(env); err != nil {
		return err
	}
	if strings.TrimSpace(env["SQL_DSN"]) == "" || strings.TrimSpace(env["REDIS_CONN_STRING"]) == "" {
		return fmt.Errorf("loadtest SQL_DSN and REDIS_CONN_STRING are required")
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
	wantRelayMaxIdle := expected.RelayMaxIdleConns
	wantRelayMaxIdlePerHost := expected.RelayMaxIdleConnsPerHost
	wantGOMEMLIMIT := expected.GOMEMLIMIT
	for key, want := range map[string]string{
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED": "false",
		"CHANNEL_UPDATE_FREQUENCY":                   "0",
		"UPDATE_TASK":                                "false",
		"CHANNEL_TEST_FREQUENCY":                     "0",
		"RetryTimes":                                 "0",
		"AutomaticRetryStatusCodes":                  "",
		"MEMORY_CACHE_ENABLED":                       "true",
		"BATCH_UPDATE_ENABLED":                       "false",
		"NODE_TYPE":                                  "slave",
		"RELAY_MAX_IDLE_CONNS":                       wantRelayMaxIdle,
		"RELAY_MAX_IDLE_CONNS_PER_HOST":              wantRelayMaxIdlePerHost,
		"REDIS_POOL_SIZE":                            "2048",
		"GOMEMLIMIT":                                 wantGOMEMLIMIT,
	} {
		if env[key] != want {
			return fmt.Errorf("%s must match expected loadtest value", key)
		}
	}
	return nil
}

func validateExpectedLimits(expected ExpectedLimits) error {
	if strings.TrimSpace(expected.RelayMaxIdleConns) == "" || strings.TrimSpace(expected.RelayMaxIdleConnsPerHost) == "" || strings.TrimSpace(expected.GOMEMLIMIT) == "" {
		return fmt.Errorf("expected loadtest limits must be fully specified")
	}
	maxIdle, err := parsePositiveLimit(expected.RelayMaxIdleConns)
	if err != nil {
		return fmt.Errorf("expected relay max idle conns is invalid")
	}
	maxIdlePerHost, err := parsePositiveLimit(expected.RelayMaxIdleConnsPerHost)
	if err != nil {
		return fmt.Errorf("expected relay max idle conns per host is invalid")
	}
	if !isAllowedExpectedRelayLimits(maxIdle, maxIdlePerHost) {
		return fmt.Errorf("expected relay limits must match default or benchmark profile")
	}
	if expected.GOMEMLIMIT != "384MiB" {
		return fmt.Errorf("expected GOMEMLIMIT must match benchmark server limit")
	}
	return nil
}

func parsePositiveLimit(value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid positive integer")
	}
	return parsed, nil
}

func isAllowedExpectedRelayLimits(maxIdle, maxIdlePerHost int) bool {
	return (maxIdle == 64 && maxIdlePerHost == 16) || (maxIdle == 1024 && maxIdlePerHost == 1024)
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
