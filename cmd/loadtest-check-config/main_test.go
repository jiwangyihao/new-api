package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesBenchmarkProfileEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	outEnv := filepath.Join(dir, "new-api.env")
	if err := os.WriteFile(configPath, []byte(validCheckConfigYAML()), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"--config", configPath, "--out-env", outEnv, "--commit", "abcdef0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run code=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	content, err := os.ReadFile(outEnv)
	if err != nil {
		t.Fatal(err)
	}
	env := string(content)
	for _, want := range []string{"REDIS_POOL_SIZE=2048", "SQL_MAX_OPEN_CONNS=256", "SQL_MAX_IDLE_CONNS=64", "RELAY_MAX_IDLE_CONNS=1024", "RELAY_MAX_IDLE_CONNS_PER_HOST=1024", "GOMEMLIMIT=384MiB"} {
		if !strings.Contains(env, want+"\n") {
			t.Fatalf("env missing %q:\n%s", want, env)
		}
	}
}

func validCheckConfigYAML() string {
	return `server:
  host: "127.0.0.1"
  port: 13080
  pprof_addr: "127.0.0.1:8005"
  runtime_stats_enabled: true
  profile_block_rate: 1000
  profile_mutex_fraction: 5
postgres:
  dsn: "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable"
log_postgres:
  dsn: ""
redis:
  addr: "redis://127.0.0.1:16379/0"
mock_upstream:
  base_url: "http://127.0.0.1:19080"
loadtest:
  model: "gpt-5.5"
  subscription_key: "sk-loadtestsub"
  compat_key: "sk-loadtestcompat"
  invalid_key: "sk-loadtestinvalid"
  token_db_key_subscription: "loadtestsub"
  token_db_key_compat: "loadtestcompat"
  pid_file: ".loadtest/new-api.pid"
client:
  max_idle_conns: 64
  max_idle_conns_per_host: 16
mock_profiles:
  s1-smoke: {first_token_delay: 50ms, stream_duration: 500ms, chunk_interval: 50ms, output_bytes: 128, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
  s2-short-stream: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
  s3-long-stream: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
  s4-error-refund: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0.05, 502: 0.01}, seed: 1}
  s5-large-payload: {first_token_delay: 1ms, stream_duration: 1ms, chunk_interval: 1ms, output_bytes: 32, prompt_tokens: 11, completion_tokens: 17, status_rate: {429: 0, 502: 0}, seed: 1}
retry:
  retry_times: 0
  automatic_retry_status_codes: []
profiles:
  benchmark:
    points: [250, 500, 750, 1000, 1250, 1500, 1750, 2000]
    requests_per_point: 3000
    ramp_step: 25
    ramp_interval: "200ms"
    duration: "45s"
    timeout: "120s"
    transport: {mode: "h1_keepalive", max_conns_per_host: 1024, max_idle_conns: 1024, max_idle_conns_per_host: 1024}
    relay: {max_idle_conns: 1024, max_idle_conns_per_host: 1024}
    server_limits: {gomaxprocs: "2", gogc: "100", gomemlimit: "384MiB", process_memory_limit_bytes: 536870912, cpu_affinity_cores: 2}
thresholds:
  latency_p95_regression_ratio: 1.10
  ttft_p95_regression_ratio: 1.10
`
}
