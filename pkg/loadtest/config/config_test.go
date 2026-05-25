package config

import (
	"os"
	"path/filepath"
	"strings"
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
	for _, key := range []string{"HOST", "PORT", "PPROF_ADDR", "SQL_DSN", "LOG_SQL_DSN", "REDIS_CONN_STRING", "REDIS_POOL_SIZE", "REDIS_IDLE_TIMEOUT_SECONDS", "ENABLE_PPROF", "LOADTEST_RUNTIME_STATS_ENABLED", "LOADTEST_PROFILE_BLOCK_RATE", "LOADTEST_PROFILE_MUTEX_FRACTION", "GOMAXPROCS", "GOGC", "GOMEMLIMIT", "NODE_TYPE", "BATCH_UPDATE_ENABLED", "BATCH_UPDATE_INTERVAL", "SQL_MAX_OPEN_CONNS", "SQL_MAX_IDLE_CONNS", "SQL_MAX_LIFETIME", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", "CHANNEL_UPDATE_FREQUENCY", "UPDATE_TASK", "CHANNEL_TEST_FREQUENCY", "PYROSCOPE_URL", "SYNC_UPSTREAM_BASE", "RetryTimes", "AutomaticRetryStatusCodes", "MEMORY_CACHE_ENABLED", "RELAY_MAX_IDLE_CONNS", "RELAY_MAX_IDLE_CONNS_PER_HOST"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("missing env %s", key)
		}
	}
	if env["RetryTimes"] != "0" || env["AutomaticRetryStatusCodes"] != "" {
		t.Fatalf("retry not disabled: %#v", env)
	}
	if env["MEMORY_CACHE_ENABLED"] != "true" {
		t.Fatalf("loadtest runtime guard not set: %#v", env)
	}
	if env["REDIS_POOL_SIZE"] != "256" {
		t.Fatalf("redis pool size not bounded for benchmark: %#v", env)
	}
	if env["REDIS_IDLE_TIMEOUT_SECONDS"] != "1" {
		t.Fatalf("redis idle timeout not bounded for benchmark: %#v", env)
	}
	if env["SQL_MAX_OPEN_CONNS"] != "64" || env["SQL_MAX_IDLE_CONNS"] != "64" {
		t.Fatalf("loadtest SQL pool must use a bounded reusable connection set: %#v", env)
	}
	if env["RELAY_MAX_IDLE_CONNS"] != "64" || env["RELAY_MAX_IDLE_CONNS_PER_HOST"] != "16" {
		t.Fatalf("unsafe relay connection limits: %#v", env)
	}
	if env["BATCH_UPDATE_ENABLED"] != "false" || env["BATCH_UPDATE_INTERVAL"] != "1" {
		t.Fatalf("loadtest must use synchronous quota counters: %#v", env)
	}
	if env["NODE_TYPE"] != "slave" {
		t.Fatalf("loadtest must run as a non-master node: %#v", env)
	}
	if env["REDIS_POOL_SIZE"] != "256" {
		t.Fatalf("loadtest Redis pool must remain below managed Redis crash threshold: %#v", env)
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
	secretDSN := "postgresql://new_api:supersecret@example.com:5432/new_api_loadtest"
	secretKey := "sk-realproductionkey"
	for _, tc := range []struct {
		name   string
		mutate func(*File)
	}{
		{"postgres key-value dsn", func(f *File) { f.Postgres.DSN = "host=127.0.0.1 dbname=new_api_loadtest" }},
		{"postgres default port", func(f *File) {
			f.Postgres.DSN = "postgresql://new_api_loadtest:loadtest@127.0.0.1:5432/new_api_loadtest?sslmode=disable"
		}},
		{"postgres non-loadtest user", func(f *File) {
			f.Postgres.DSN = "postgresql://new_api:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable"
		}},
		{"redis non-loopback", func(f *File) { f.Redis.Addr = "redis://10.0.0.2:16379/0" }},
		{"redis default port", func(f *File) { f.Redis.Addr = "redis://127.0.0.1:6379/0" }},
		{"mock upstream real url", func(f *File) { f.MockUpstream.BaseURL = "https://api.openai.com" }},
		{"mock upstream non-loopback", func(f *File) { f.MockUpstream.BaseURL = "http://192.0.2.10:19080" }},
		{"server wildcard", func(f *File) { f.Server.Host = "0.0.0.0" }},
		{"pprof wildcard", func(f *File) { f.Server.PprofAddr = "0.0.0.0:8005" }},
		{"subscription key", func(f *File) { f.Loadtest.SubscriptionKey = secretKey }},
		{"compat key", func(f *File) { f.Loadtest.CompatKey = secretKey }},
		{"invalid key", func(f *File) { f.Loadtest.InvalidKey = secretKey }},
		{"retry", func(f *File) { f.Retry.RetryTimes = 1 }},
		{"retry statuses", func(f *File) { f.Retry.AutomaticRetryStatusCodes = []int{429} }},
		{"log postgres", func(f *File) { f.LogPostgres.DSN = secretDSN }},
		{"top client max idle", func(f *File) { f.Client.MaxIdleConns = 129 }},
		{"top client max per host", func(f *File) { f.Client.MaxIdleConnsPerHost = 65 }},
	} {
		f := validFile()
		tc.mutate(&f)
		err := f.Validate()
		if err == nil {
			t.Fatalf("unsafe config accepted for %s: %#v", tc.name, f)
		}
		msg := err.Error()
		if strings.Contains(msg, "supersecret") || strings.Contains(msg, secretKey) || strings.Contains(msg, "example.com") || strings.Contains(msg, "api.openai.com") {
			t.Fatalf("%s leaked secret in error: %v", tc.name, err)
		}
	}
}

func TestBenchmarkProfileAllowsExplicitHighCapacityConnectionLimits(t *testing.T) {
	f := validFile()
	f.Profiles = map[string]ProfileConfig{"benchmark": benchmarkProfileConfig()}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	p, err := f.Profile("benchmark")
	if err != nil {
		t.Fatal(err)
	}
	if p.Transport.MaxConnsPerHost != 2000 || p.Transport.MaxIdleConns != 2000 || p.Transport.MaxIdleConnsPerHost != 2000 {
		t.Fatalf("benchmark transport limits = %#v", p.Transport)
	}
	if p.Relay.MaxIdleConns != 1024 || p.Relay.MaxIdleConnsPerHost != 1024 || p.ServerLimits.GOMEMLIMIT != "384MiB" {
		t.Fatalf("benchmark relay/server limits = %#v %#v", p.Relay, p.ServerLimits)
	}
}

func TestDefaultClientLimitsRemainSafeWithoutBenchmarkProfile(t *testing.T) {
	f := validFile()
	f.Client.MaxIdleConns = 129
	if err := f.Validate(); err == nil {
		t.Fatal("unsafe top-level client limit accepted")
	}
	f = validFile()
	f.Profiles = nil
	if _, err := f.Profile("benchmark"); err == nil {
		t.Fatal("benchmark profile without explicit config accepted")
	}
}

func TestNewAPIEnvForProfileOnlyRaisesRelayPoolForBenchmark(t *testing.T) {
	f := validFile()
	f.Profiles = map[string]ProfileConfig{"benchmark": benchmarkProfileConfig()}
	base := f.NewAPIEnv()
	profileEnv, err := f.NewAPIEnvForProfile("benchmark")
	if err != nil {
		t.Fatal(err)
	}
	if base["RELAY_MAX_IDLE_CONNS"] != "64" || base["RELAY_MAX_IDLE_CONNS_PER_HOST"] != "16" {
		t.Fatalf("base relay env unsafe: %#v", base)
	}
	if profileEnv["RELAY_MAX_IDLE_CONNS"] != "1024" || profileEnv["RELAY_MAX_IDLE_CONNS_PER_HOST"] != "1024" || profileEnv["GOMEMLIMIT"] != "384MiB" {
		t.Fatalf("benchmark env mismatch: %#v", profileEnv)
	}
	if profileEnv["SQL_MAX_OPEN_CONNS"] != "64" || profileEnv["SQL_MAX_IDLE_CONNS"] != "64" {
		t.Fatalf("benchmark env must cap SQL open connections at idle capacity: %#v", profileEnv)
	}
	for _, key := range []string{"SQL_DSN", "LOG_SQL_DSN", "REDIS_CONN_STRING", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", "RetryTimes", "AutomaticRetryStatusCodes"} {
		if profileEnv[key] != base[key] {
			t.Fatalf("profile env changed safety key %s: %q != %q", key, profileEnv[key], base[key])
		}
	}
	f.Profiles["benchmark"] = benchmarkProfileConfig()
	cfg := f.Profiles["benchmark"]
	cfg.Relay.MaxIdleConns = 2048
	f.Profiles["benchmark"] = cfg
	if _, err := f.NewAPIEnvForProfile("benchmark"); err == nil {
		t.Fatal("profile env accepted relay limit above declared safety maximum")
	}
	f = validFile()
	f.Profiles = map[string]ProfileConfig{"benchmark": benchmarkProfileConfig()}
	cfg = f.Profiles["benchmark"]
	cfg.Relay.MaxIdleConns = 512
	f.Profiles["benchmark"] = cfg
	if _, err := f.NewAPIEnvForProfile("benchmark"); err == nil {
		t.Fatal("profile env accepted non-canonical relay limit")
	}
	if base["RELAY_MAX_IDLE_CONNS"] != "64" {
		t.Fatalf("base env was mutated: %#v", base)
	}
}

func TestBenchmarkProfileCapsRedisPoolBelowManagedRedisCrashThreshold(t *testing.T) {
	f := validFile()
	f.Profiles = map[string]ProfileConfig{"benchmark": benchmarkProfileConfig()}
	profileEnv, err := f.NewAPIEnvForProfile("benchmark")
	if err != nil {
		t.Fatal(err)
	}
	if profileEnv["REDIS_POOL_SIZE"] != "256" {
		t.Fatalf("benchmark Redis pool size = %s, want bounded 256", profileEnv["REDIS_POOL_SIZE"])
	}
	if profileEnv["REDIS_IDLE_TIMEOUT_SECONDS"] != "1" {
		t.Fatalf("benchmark Redis idle timeout = %s, want 1", profileEnv["REDIS_IDLE_TIMEOUT_SECONDS"])
	}
}

func TestBenchmarkProfileRejectsNonCanonicalValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ProfileConfig)
	}{
		{"non increasing points", func(c *ProfileConfig) { c.Points = []int{250, 250} }},
		{"zero requests", func(c *ProfileConfig) { c.RequestsPerPoint = 0 }},
		{"zero ramp", func(c *ProfileConfig) { c.RampStep = 0 }},
		{"zero duration", func(c *ProfileConfig) { c.Duration = Duration{} }},
		{"zero timeout", func(c *ProfileConfig) { c.Timeout = Duration{} }},
		{"bad transport", func(c *ProfileConfig) { c.Transport.Mode = "h2c_diagnostic" }},
		{"wrong requests", func(c *ProfileConfig) { c.RequestsPerPoint = 2999 }},
		{"missing upper point", func(c *ProfileConfig) { c.Points = []int{250, 500, 750, 1000} }},
		{"wrong point", func(c *ProfileConfig) { c.Points = []int{250, 500, 750, 1000, 1250, 1500, 1750, 1999} }},
		{"wrong gomemlimit", func(c *ProfileConfig) { c.ServerLimits.GOMEMLIMIT = "512MiB" }},
		{"wrong transport mode", func(c *ProfileConfig) { c.Transport.Mode = "h1_no_keepalive" }},
	} {
		f := validFile()
		cfg := benchmarkProfileConfig()
		tc.mutate(&cfg)
		f.Profiles = map[string]ProfileConfig{"benchmark": cfg}
		if err := f.Validate(); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
}

func TestH2CDiagnosticProfileRejectedInFirstStage(t *testing.T) {
	f := validFile()
	if _, err := f.Profile("h2c_diagnostic"); err == nil || !strings.Contains(err.Error(), "h2c diagnostic profile is not implemented in this phase") {
		t.Fatalf("h2c_diagnostic profile error = %v", err)
	}
	f.Profiles = map[string]ProfileConfig{"h2c_diagnostic": benchmarkProfileConfig()}
	if err := f.Validate(); err == nil || !strings.Contains(err.Error(), "h2c diagnostic profile is not implemented in this phase") {
		t.Fatalf("h2c_diagnostic config error = %v", err)
	}
}

func TestUnknownProfileErrorDoesNotEchoInput(t *testing.T) {
	f := validFile()
	f.Profiles = map[string]ProfileConfig{"sk-realproductionkey": benchmarkProfileConfig()}
	err := f.Validate()
	if err == nil {
		t.Fatal("unsafe profile name accepted")
	}
	if strings.Contains(err.Error(), "sk-realproductionkey") {
		t.Fatalf("profile error leaked input: %v", err)
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
profiles:
  benchmark:
    points: [250, 500, 750, 1000, 1250, 1500, 1750, 2000]
    requests_per_point: 3000
    ramp_step: 125
    ramp_interval: 200ms
    duration: 75s
    timeout: 120s
    transport:
      mode: h1_keepalive
      max_conns_per_host: 2000
      max_idle_conns: 2000
      max_idle_conns_per_host: 2000
    relay:
      max_idle_conns: 1024
      max_idle_conns_per_host: 1024
    server_limits:
      gomaxprocs: "2"
      gogc: "100"
      gomemlimit: "384MiB"
      process_memory_limit_bytes: 536870912
      cpu_affinity_cores: 2
      sql_max_open_conns: "64"
      sql_max_idle_conns: "64"
      redis_pool_size: "256"
      redis_idle_timeout_seconds: "1"
      relay_max_idle_conns: "1024"
      relay_max_idle_conns_per_host: "1024"
thresholds:
  latency_p95_regression_ratio: 1.10
  ttft_p95_regression_ratio: 1.10
`
}

func mustDuration(value string) Duration {
	d, err := ParseDuration(value)
	if err != nil {
		panic(err)
	}
	return d
}

func benchmarkProfileConfig() ProfileConfig {
	return ProfileConfig{
		Points:           []int{250, 500, 750, 1000, 1250, 1500, 1750, 2000},
		RequestsPerPoint: 3000,
		RampStep:         125,
		RampInterval:     mustDuration("200ms"),
		Duration:         mustDuration("75s"),
		Timeout:          mustDuration("120s"),
		Transport: TransportConfig{
			Mode:                "h1_keepalive",
			MaxConnsPerHost:     2000,
			MaxIdleConns:        2000,
			MaxIdleConnsPerHost: 2000,
		},
		Relay: RelayConfig{
			MaxIdleConns:        1024,
			MaxIdleConnsPerHost: 1024,
		},
		ServerLimits: ServerLimitsConfig{
			GOMAXPROCS:               "2",
			GOGC:                     "100",
			GOMEMLIMIT:               "384MiB",
			ProcessMemoryLimitBytes:  536870912,
			CPUAffinityCores:         2,
			SQLMaxOpenConns:          "64",
			SQLMaxIdleConns:          "64",
			RedisPoolSize:            "256",
			RedisIdleTimeoutSeconds:  "1",
			RelayMaxIdleConns:        "1024",
			RelayMaxIdleConnsPerHost: "1024",
		},
	}
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
		Client: ClientConfig{
			MaxIdleConns:        64,
			MaxIdleConnsPerHost: 16,
		},
		MockProfiles: profiles,
	}
}
