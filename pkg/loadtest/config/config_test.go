package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadValidateAndWriteEnv(t *testing.T) {
	cfg := writeValidConfig(t)
	file, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Validate(); err != nil {
		t.Fatal(err)
	}
	env := file.NewAPIEnv()
	for _, key := range []string{"HOST", "PORT", "PPROF_ADDR", "SQL_DSN", "LOG_SQL_DSN", "REDIS_CONN_STRING", "ENABLE_PPROF", "LOADTEST_RUNTIME_STATS_ENABLED", "LOADTEST_PROFILE_BLOCK_RATE", "LOADTEST_PROFILE_MUTEX_FRACTION", "GOMAXPROCS", "GOGC", "BATCH_UPDATE_ENABLED", "SQL_MAX_OPEN_CONNS", "SQL_MAX_IDLE_CONNS", "SQL_MAX_LIFETIME", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", "CHANNEL_UPDATE_FREQUENCY", "UPDATE_TASK", "CHANNEL_TEST_FREQUENCY", "PYROSCOPE_URL", "SYNC_UPSTREAM_BASE", "RetryTimes", "AutomaticRetryStatusCodes"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("missing env %s", key)
		}
	}
	if env["RetryTimes"] != "0" || env["AutomaticRetryStatusCodes"] != "" {
		t.Fatalf("retry not disabled: %#v", env)
	}
	rc, err := file.BaseRunContext("abcdef0")
	if err != nil {
		t.Fatal(err)
	}
	if rc.ComparisonConfigHash == "" || rc.MockHash != "" || rc.SeedOutputHash != "" {
		t.Fatalf("bad base context: %#v", rc)
	}
	for _, profile := range []string{"s1-smoke", "s2-short-stream", "s3-long-stream", "s4-error-refund", "s5-large-payload"} {
		if _, ok := file.MockProfiles[profile]; !ok {
			t.Fatalf("missing mock profile %s", profile)
		}
		if file.MockProfileHash(profile) == "" {
			t.Fatalf("missing mock hash for %s", profile)
		}
	}
}

func TestDeterministicErrorCountsMatchesStatusHelper(t *testing.T) {
	rate := map[int]float64{429: 0.05, 502: 0.01}
	got429, got502 := DeterministicErrorCounts(1, 100, rate)
	var want429, want502 int
	for attempt := int64(1); attempt <= 100; attempt++ {
		switch {
		case ShouldInjectStatus(1, attempt, 429, rate[429]):
			want429++
		case ShouldInjectStatus(1, attempt, 502, rate[502]):
			want502++
		}
	}
	if got429 != want429 || got502 != want502 {
		t.Fatalf("counts = %d/%d want %d/%d", got429, got502, want429, want502)
	}
}

func TestConfigRejectsUnsafeValues(t *testing.T) {
	for _, mutate := range []func(*File){
		func(f *File) { f.Postgres.DSN = "host=127.0.0.1 dbname=new_api_loadtest" },
		func(f *File) { f.Redis.Addr = "redis://10.0.0.2:6379/0" },
		func(f *File) { f.MockUpstream.BaseURL = "https://api.openai.com" },
		func(f *File) { f.Server.Host = "0.0.0.0" },
		func(f *File) { f.Server.PprofAddr = "0.0.0.0:8005" },
		func(f *File) { f.Loadtest.SubscriptionKey = "sk-loadtest-subscription" },
		func(f *File) { f.Retry.RetryTimes = 1 },
		func(f *File) { f.Retry.AutomaticRetryStatusCodes = []int{429} },
		func(f *File) { f.LogPostgres.DSN = "postgresql://new_api:secret@example.com:5432/new_api_loadtest" },
	} {
		f := validFile()
		mutate(&f)
		if err := f.Validate(); err == nil {
			t.Fatalf("unsafe config accepted: %#v", f)
		}
	}
}

func TestHashMockProfileUsesOnlyProfileFields(t *testing.T) {
	profile := validFile().MockProfiles["s1-smoke"]
	hash1, err := HashMockProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := HashMockProfile(MockProfile{
		FirstTokenDelay:  profile.FirstTokenDelay,
		StreamDuration:   profile.StreamDuration,
		ChunkInterval:    profile.ChunkInterval,
		OutputBytes:      profile.OutputBytes,
		PromptTokens:     profile.PromptTokens,
		CompletionTokens: profile.CompletionTokens,
		StatusRate:       map[int]float64{502: 0, 429: 0},
		Seed:             profile.Seed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hash1 != hash2 {
		t.Fatalf("hash not stable: %s != %s", hash1, hash2)
	}
}

func writeValidConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(validYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func validYAML() string {
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
  group: "default"
  subscription_key: "sk-loadtestsub"
  compat_key: "sk-loadtestcompat"
  invalid_key: "sk-loadtestinvalid"
  token_db_key_subscription: "loadtestsub"
  token_db_key_compat: "loadtestcompat"
  pid_file: ".loadtest/new-api.pid"
mock_profiles:
  s1-smoke:
    first_token_delay: 50ms
    stream_duration: 500ms
    chunk_interval: 50ms
    output_bytes: 128
    prompt_tokens: 11
    completion_tokens: 17
    status_rate: {429: 0, 502: 0}
    seed: 1
  s2-short-stream:
    first_token_delay: 100ms
    stream_duration: 1s
    chunk_interval: 100ms
    output_bytes: 128
    prompt_tokens: 11
    completion_tokens: 17
    status_rate: {429: 0, 502: 0}
    seed: 1
  s3-long-stream:
    first_token_delay: 150ms
    stream_duration: 30s
    chunk_interval: 250ms
    output_bytes: 4096
    prompt_tokens: 11
    completion_tokens: 17
    status_rate: {429: 0, 502: 0}
    seed: 1
  s4-error-refund:
    first_token_delay: 100ms
    stream_duration: 1s
    chunk_interval: 100ms
    output_bytes: 128
    prompt_tokens: 11
    completion_tokens: 17
    status_rate: {429: 0.05, 502: 0.01}
    seed: 1
  s5-large-payload:
    first_token_delay: 150ms
    stream_duration: 3s
    chunk_interval: 100ms
    output_bytes: 65536
    prompt_tokens: 11
    completion_tokens: 17
    status_rate: {429: 0, 502: 0}
    seed: 1
retry:
  retry_times: 0
  automatic_retry_status_codes: []
thresholds:
  latency_p95_regression_ratio: 1.10
  ttft_p95_regression_ratio: 1.10
`
}

func validFile() File {
	profiles := map[string]MockProfile{
		"s1-smoke": {
			FirstTokenDelay:  50 * time.Millisecond,
			StreamDuration:   500 * time.Millisecond,
			ChunkInterval:    50 * time.Millisecond,
			OutputBytes:      128,
			PromptTokens:     11,
			CompletionTokens: 17,
			StatusRate:       map[int]float64{429: 0, 502: 0},
			Seed:             1,
		},
		"s2-short-stream": {
			FirstTokenDelay:  100 * time.Millisecond,
			StreamDuration:   time.Second,
			ChunkInterval:    100 * time.Millisecond,
			OutputBytes:      128,
			PromptTokens:     11,
			CompletionTokens: 17,
			StatusRate:       map[int]float64{429: 0, 502: 0},
			Seed:             1,
		},
		"s3-long-stream": {
			FirstTokenDelay:  150 * time.Millisecond,
			StreamDuration:   30 * time.Second,
			ChunkInterval:    250 * time.Millisecond,
			OutputBytes:      4096,
			PromptTokens:     11,
			CompletionTokens: 17,
			StatusRate:       map[int]float64{429: 0, 502: 0},
			Seed:             1,
		},
		"s4-error-refund": {
			FirstTokenDelay:  100 * time.Millisecond,
			StreamDuration:   time.Second,
			ChunkInterval:    100 * time.Millisecond,
			OutputBytes:      128,
			PromptTokens:     11,
			CompletionTokens: 17,
			StatusRate:       map[int]float64{429: 0.05, 502: 0.01},
			Seed:             1,
		},
		"s5-large-payload": {
			FirstTokenDelay:  150 * time.Millisecond,
			StreamDuration:   3 * time.Second,
			ChunkInterval:    100 * time.Millisecond,
			OutputBytes:      65536,
			PromptTokens:     11,
			CompletionTokens: 17,
			StatusRate:       map[int]float64{429: 0, 502: 0},
			Seed:             1,
		},
	}
	return File{
		Server: ServerConfig{
			Host:                 "127.0.0.1",
			Port:                 13080,
			PprofAddr:            "127.0.0.1:8005",
			RuntimeStatsEnabled:  true,
			ProfileBlockRate:     1000,
			ProfileMutexFraction: 5,
		},
		Postgres:    PostgresConfig{DSN: "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable"},
		LogPostgres: PostgresConfig{},
		Redis:       RedisConfig{Addr: "redis://127.0.0.1:16379/0"},
		MockUpstream: MockUpstreamConfig{
			BaseURL: "http://127.0.0.1:19080",
		},
		Loadtest: LoadtestConfig{
			Model:                  "gpt-5.5",
			Group:                  "default",
			SubscriptionKey:        "sk-loadtestsub",
			CompatKey:              "sk-loadtestcompat",
			InvalidKey:             "sk-loadtestinvalid",
			TokenDBKeySubscription: "loadtestsub",
			TokenDBKeyCompat:       "loadtestcompat",
			PIDFile:                ".loadtest/new-api.pid",
		},
		Retry: RetryConfig{
			RetryTimes:                0,
			AutomaticRetryStatusCodes: nil,
		},
		Thresholds: ThresholdsConfig{
			LatencyP95RegressionRatio: 1.10,
			TTFTP95RegressionRatio:    1.10,
		},
		MockProfiles: profiles,
	}
}
