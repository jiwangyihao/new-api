package config

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/QuantumNous/new-api/pkg/loadtest/localguard"
	"github.com/QuantumNous/new-api/pkg/loadtest/profile"
	"gopkg.in/yaml.v3"
)

const (
	SubscriptionAPIKey = "sk-loadtestsub"
	CompatAPIKey       = "sk-loadtestcompat"
	InvalidAPIKey      = "sk-loadtestinvalid"

	SubscriptionDBKey = "loadtestsub"
	CompatDBKey       = "loadtestcompat"
)

var EnvKeys = []string{
	"HOST",
	"PORT",
	"PPROF_ADDR",
	"SQL_DSN",
	"LOG_SQL_DSN",
	"REDIS_CONN_STRING",
	"REDIS_POOL_SIZE",
	"ENABLE_PPROF",
	"LOADTEST_RUNTIME_STATS_ENABLED",
	"LOADTEST_PROFILE_BLOCK_RATE",
	"LOADTEST_PROFILE_MUTEX_FRACTION",
	"GOMAXPROCS",
	"GOGC",
	"GOMEMLIMIT",
	"NODE_TYPE",
	"BATCH_UPDATE_ENABLED",
	"BATCH_UPDATE_INTERVAL",
	"SQL_MAX_OPEN_CONNS",
	"SQL_MAX_IDLE_CONNS",
	"SQL_MAX_LIFETIME",
	"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED",
	"CHANNEL_UPDATE_FREQUENCY",
	"UPDATE_TASK",
	"CHANNEL_TEST_FREQUENCY",
	"PYROSCOPE_URL",
	"SYNC_UPSTREAM_BASE",
	"RetryTimes",
	"AutomaticRetryStatusCodes",
	"MEMORY_CACHE_ENABLED",

	"RELAY_MAX_IDLE_CONNS",
	"RELAY_MAX_IDLE_CONNS_PER_HOST",
}

type File struct {
	Server       ServerConfig             `json:"server" yaml:"server"`
	Postgres     PostgresConfig           `json:"postgres" yaml:"postgres"`
	LogPostgres  PostgresConfig           `json:"log_postgres" yaml:"log_postgres"`
	Redis        RedisConfig              `json:"redis" yaml:"redis"`
	MockUpstream MockUpstreamConfig       `json:"mock_upstream" yaml:"mock_upstream"`
	Loadtest     LoadtestConfig           `json:"loadtest" yaml:"loadtest"`
	Retry        RetryConfig              `json:"retry" yaml:"retry"`
	Thresholds   ThresholdsConfig         `json:"thresholds" yaml:"thresholds"`
	Client       ClientConfig             `json:"client" yaml:"client"`
	MockProfiles map[string]MockProfile   `json:"mock_profiles" yaml:"mock_profiles"`
	Profiles     map[string]ProfileConfig `json:"profiles" yaml:"profiles"`
}

type ServerConfig struct {
	Host                 string `json:"host" yaml:"host"`
	Port                 int    `json:"port" yaml:"port"`
	PprofAddr            string `json:"pprof_addr" yaml:"pprof_addr"`
	RuntimeStatsEnabled  bool   `json:"runtime_stats_enabled" yaml:"runtime_stats_enabled"`
	ProfileBlockRate     int    `json:"profile_block_rate" yaml:"profile_block_rate"`
	ProfileMutexFraction int    `json:"profile_mutex_fraction" yaml:"profile_mutex_fraction"`
}

type PostgresConfig struct {
	DSN string `json:"dsn" yaml:"dsn"`
}

type RedisConfig struct {
	Addr string `json:"addr" yaml:"addr"`
}

type MockUpstreamConfig struct {
	BaseURL string `json:"base_url" yaml:"base_url"`
}

type LoadtestConfig struct {
	Model                  string `json:"model" yaml:"model"`
	Group                  string `json:"group" yaml:"group"` // Deprecated: legacy configs may include it; ignored.
	SubscriptionKey        string `json:"subscription_key" yaml:"subscription_key"`
	CompatKey              string `json:"compat_key" yaml:"compat_key"`
	InvalidKey             string `json:"invalid_key" yaml:"invalid_key"`
	TokenDBKeySubscription string `json:"token_db_key_subscription" yaml:"token_db_key_subscription"`
	TokenDBKeyCompat       string `json:"token_db_key_compat" yaml:"token_db_key_compat"`
	PIDFile                string `json:"pid_file" yaml:"pid_file"`
}

type RetryConfig struct {
	RetryTimes                int   `json:"retry_times" yaml:"retry_times"`
	AutomaticRetryStatusCodes []int `json:"automatic_retry_status_codes" yaml:"automatic_retry_status_codes"`
}

type ClientConfig struct {
	MaxIdleConns        int `json:"max_idle_conns" yaml:"max_idle_conns"`
	MaxIdleConnsPerHost int `json:"max_idle_conns_per_host" yaml:"max_idle_conns_per_host"`
}

type Duration struct {
	time.Duration
}

type TransportConfig struct {
	Mode                string `json:"mode" yaml:"mode"`
	MaxConnsPerHost     int    `json:"max_conns_per_host" yaml:"max_conns_per_host"`
	MaxIdleConns        int    `json:"max_idle_conns" yaml:"max_idle_conns"`
	MaxIdleConnsPerHost int    `json:"max_idle_conns_per_host" yaml:"max_idle_conns_per_host"`
}

type RelayConfig struct {
	MaxIdleConns        int `json:"max_idle_conns" yaml:"max_idle_conns"`
	MaxIdleConnsPerHost int `json:"max_idle_conns_per_host" yaml:"max_idle_conns_per_host"`
}

type ServerLimitsConfig struct {
	GOMAXPROCS              string `json:"gomaxprocs" yaml:"gomaxprocs"`
	GOGC                    string `json:"gogc" yaml:"gogc"`
	GOMEMLIMIT              string `json:"gomemlimit" yaml:"gomemlimit"`
	ProcessMemoryLimitBytes uint64 `json:"process_memory_limit_bytes" yaml:"process_memory_limit_bytes"`
	CPUAffinityCores        int    `json:"cpu_affinity_cores" yaml:"cpu_affinity_cores"`
}

type ProfileConfig struct {
	Points           []int              `json:"points" yaml:"points"`
	RequestsPerPoint int                `json:"requests_per_point" yaml:"requests_per_point"`
	RampStep         int                `json:"ramp_step" yaml:"ramp_step"`
	RampInterval     Duration           `json:"ramp_interval" yaml:"ramp_interval"`
	Duration         Duration           `json:"duration" yaml:"duration"`
	Timeout          Duration           `json:"timeout" yaml:"timeout"`
	Transport        TransportConfig    `json:"transport" yaml:"transport"`
	Relay            RelayConfig        `json:"relay" yaml:"relay"`
	ServerLimits     ServerLimitsConfig `json:"server_limits" yaml:"server_limits"`
}

type ThresholdsConfig struct {
	LatencyP95RegressionRatio float64 `json:"latency_p95_regression_ratio" yaml:"latency_p95_regression_ratio"`
	TTFTP95RegressionRatio    float64 `json:"ttft_p95_regression_ratio" yaml:"ttft_p95_regression_ratio"`
}

type MockProfile struct {
	FirstTokenDelay  time.Duration   `json:"first_token_delay" yaml:"first_token_delay"`
	StreamDuration   time.Duration   `json:"stream_duration" yaml:"stream_duration"`
	ChunkInterval    time.Duration   `json:"chunk_interval" yaml:"chunk_interval"`
	OutputBytes      int             `json:"output_bytes" yaml:"output_bytes"`
	PromptTokens     int             `json:"prompt_tokens" yaml:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens" yaml:"completion_tokens"`
	StatusRate       map[int]float64 `json:"status_rate" yaml:"status_rate"`
	Seed             int64           `json:"seed" yaml:"seed"`
}

func ParseDuration(value string) (Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return Duration{}, err
	}
	return Duration{Duration: d}, nil
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err != nil {
		return err
	}
	parsed, err := ParseDuration(raw)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	raw, err := strconv.Unquote(string(data))
	if err != nil {
		return err
	}
	parsed, err := ParseDuration(raw)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(d.Duration.String())), nil
}

func (d Duration) String() string {
	return d.Duration.String()
}

func (p *MockProfile) UnmarshalYAML(value *yaml.Node) error {
	type rawProfile struct {
		FirstTokenDelay  string          `yaml:"first_token_delay"`
		StreamDuration   string          `yaml:"stream_duration"`
		ChunkInterval    string          `yaml:"chunk_interval"`
		OutputBytes      int             `yaml:"output_bytes"`
		PromptTokens     int             `yaml:"prompt_tokens"`
		CompletionTokens int             `yaml:"completion_tokens"`
		StatusRate       map[int]float64 `yaml:"status_rate"`
		Seed             int64           `yaml:"seed"`
	}
	var raw rawProfile
	if err := value.Decode(&raw); err != nil {
		return err
	}
	firstTokenDelay, err := time.ParseDuration(raw.FirstTokenDelay)
	if err != nil {
		return fmt.Errorf("first_token_delay: %w", err)
	}
	streamDuration, err := time.ParseDuration(raw.StreamDuration)
	if err != nil {
		return fmt.Errorf("stream_duration: %w", err)
	}
	chunkInterval, err := time.ParseDuration(raw.ChunkInterval)
	if err != nil {
		return fmt.Errorf("chunk_interval: %w", err)
	}
	*p = MockProfile{
		FirstTokenDelay:  firstTokenDelay,
		StreamDuration:   streamDuration,
		ChunkInterval:    chunkInterval,
		OutputBytes:      raw.OutputBytes,
		PromptTokens:     raw.PromptTokens,
		CompletionTokens: raw.CompletionTokens,
		StatusRate:       raw.StatusRate,
		Seed:             raw.Seed,
	}
	return nil
}

func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &f, nil
}

func (f File) Validate() error {
	f = f.withDefaults()
	if f.Server.Host == "" {
		return fmt.Errorf("server.host is required")
	}
	if f.Server.Port <= 0 || f.Server.Port > 65535 {
		return fmt.Errorf("server.port is invalid")
	}
	if err := localguard.ValidateListenAddr(f.serverListenAddr()); err != nil {
		return fmt.Errorf("server listen address is not safe loopback")
	}
	if err := localguard.ValidateListenAddr(f.Server.PprofAddr); err != nil {
		return fmt.Errorf("server.pprof_addr is not safe loopback")
	}
	if f.Server.ProfileBlockRate <= 0 {
		return fmt.Errorf("server.profile_block_rate must be positive")
	}
	if f.Server.ProfileMutexFraction <= 0 {
		return fmt.Errorf("server.profile_mutex_fraction must be positive")
	}
	if err := localguard.ValidatePostgresDSN(f.Postgres.DSN); err != nil {
		return fmt.Errorf("postgres.dsn is not a safe loadtest PostgreSQL DSN")
	}
	if f.LogPostgres.DSN != "" {
		if err := localguard.ValidatePostgresDSN(f.LogPostgres.DSN); err != nil {
			return fmt.Errorf("log_postgres.dsn is not a safe loadtest PostgreSQL DSN")
		}
	}
	if err := localguard.ValidateRedisAddr(f.Redis.Addr); err != nil {
		return fmt.Errorf("redis.addr is not a safe loadtest Redis address")
	}
	if err := localguard.ValidateURL(f.MockUpstream.BaseURL); err != nil {
		return fmt.Errorf("mock_upstream.base_url is not a safe loopback URL")
	}
	if err := validateLoadtest(f.Loadtest); err != nil {
		return err
	}
	if f.Retry.RetryTimes != 0 {
		return fmt.Errorf("retry.retry_times must be 0")
	}
	if len(f.Retry.AutomaticRetryStatusCodes) != 0 {
		return fmt.Errorf("retry.automatic_retry_status_codes must be empty")
	}
	if f.Client.MaxIdleConns <= 0 || f.Client.MaxIdleConnsPerHost <= 0 {
		return fmt.Errorf("client connection limits must be positive")
	}
	if f.Client.MaxIdleConns > 128 || f.Client.MaxIdleConnsPerHost > 64 {
		return fmt.Errorf("client connection limits are too high for local loopback loadtest")
	}
	for name, cfg := range f.Profiles {
		if err := validateProfileConfig(name, cfg); err != nil {
			return err
		}
	}
	if err := validateMockProfiles(f.MockProfiles); err != nil {
		return err
	}
	if f.Thresholds.LatencyP95RegressionRatio <= 0 || f.Thresholds.TTFTP95RegressionRatio <= 0 {
		return fmt.Errorf("threshold ratios must be positive")
	}
	return nil
}

func (f File) NewAPIEnv() map[string]string {
	f = f.withDefaults()
	return map[string]string{
		"HOST":                            f.Server.Host,
		"PORT":                            strconv.Itoa(f.Server.Port),
		"PPROF_ADDR":                      f.Server.PprofAddr,
		"SQL_DSN":                         f.Postgres.DSN,
		"LOG_SQL_DSN":                     f.LogPostgres.DSN,
		"REDIS_CONN_STRING":               redisConnString(f.Redis.Addr),
		"REDIS_POOL_SIZE":                 "2048",
		"ENABLE_PPROF":                    "true",
		"LOADTEST_RUNTIME_STATS_ENABLED":  boolEnv(f.Server.RuntimeStatsEnabled),
		"LOADTEST_PROFILE_BLOCK_RATE":     strconv.Itoa(f.Server.ProfileBlockRate),
		"LOADTEST_PROFILE_MUTEX_FRACTION": strconv.Itoa(f.Server.ProfileMutexFraction),
		"GOMAXPROCS":                      "2",
		"GOGC":                            "100",
		"GOMEMLIMIT":                      "384MiB",
		"NODE_TYPE":                       "slave",
		"BATCH_UPDATE_ENABLED":            "false",
		"BATCH_UPDATE_INTERVAL":           "1",
		"SQL_MAX_OPEN_CONNS":              "256",
		"SQL_MAX_IDLE_CONNS":              "64",
		"SQL_MAX_LIFETIME":                "60",
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED": "false",
		"CHANNEL_UPDATE_FREQUENCY":                   "0",
		"UPDATE_TASK":                                "false",
		"CHANNEL_TEST_FREQUENCY":                     "0",
		"PYROSCOPE_URL":                              "",
		"SYNC_UPSTREAM_BASE":                         "",
		"RetryTimes":                                 "0",
		"AutomaticRetryStatusCodes":                  "",
		"MEMORY_CACHE_ENABLED":                       "true",
		"RELAY_MAX_IDLE_CONNS":                       strconv.Itoa(f.Client.MaxIdleConns),
		"RELAY_MAX_IDLE_CONNS_PER_HOST":              strconv.Itoa(f.Client.MaxIdleConnsPerHost),
	}
}

func (f File) Profile(name string) (profile.Profile, error) {
	f = f.withDefaults()
	name = strings.TrimSpace(name)
	switch name {
	case "benchmark":
		cfg, ok := f.Profiles[name]
		if !ok {
			return profile.Profile{}, fmt.Errorf("benchmark profile must be explicitly configured")
		}
		if err := validateProfileConfig(name, cfg); err != nil {
			return profile.Profile{}, err
		}
		return profileFromConfig(name, cfg), nil
	case "smoke":
		return profile.Smoke(), nil
	case "h2c_diagnostic":
		return profile.Profile{}, fmt.Errorf("h2c diagnostic profile is not implemented in this phase")
	default:
		return profile.Profile{}, fmt.Errorf("unknown loadtest profile")
	}
}

func (f File) NewAPIEnvForProfile(name string) (map[string]string, error) {
	p, err := f.Profile(name)
	if err != nil {
		return nil, err
	}
	env := f.NewAPIEnv()
	if p.ServerLimits.GOMAXPROCS != "" {
		env["GOMAXPROCS"] = p.ServerLimits.GOMAXPROCS
	}
	if p.ServerLimits.GOGC != "" {
		env["GOGC"] = p.ServerLimits.GOGC
	}
	if p.ServerLimits.GOMEMLIMIT != "" {
		env["GOMEMLIMIT"] = p.ServerLimits.GOMEMLIMIT
	}
	if p.Relay.MaxIdleConns > 0 {
		env["RELAY_MAX_IDLE_CONNS"] = strconv.Itoa(p.Relay.MaxIdleConns)
	}
	if p.Relay.MaxIdleConnsPerHost > 0 {
		env["RELAY_MAX_IDLE_CONNS_PER_HOST"] = strconv.Itoa(p.Relay.MaxIdleConnsPerHost)
	}
	return env, nil
}

func (f File) BaseRunContext(commit string) (artifact.RunContext, error) {
	f = f.withDefaults()
	if err := f.Validate(); err != nil {
		return artifact.RunContext{}, err
	}
	if strings.TrimSpace(commit) == "" {
		commit = "unknown"
	}
	hash, err := f.comparisonConfigHash()
	if err != nil {
		return artifact.RunContext{}, err
	}
	return artifact.RunContext{
		SchemaVersion:        artifact.SchemaVersion,
		Role:                 "baseline",
		Commit:               strings.TrimSpace(commit),
		ComparisonConfigHash: hash,
		CacheMode:            "cold-fresh-role,warm-per-point",
		Model:                f.Loadtest.Model,
	}, nil
}

func (f File) MockProfileHash(profile string) string {
	f = f.withDefaults()
	p, ok := f.MockProfiles[profile]
	if !ok {
		return ""
	}
	hash, err := HashMockProfile(p)
	if err != nil {
		return ""
	}
	return hash
}

func HashMockProfile(profile MockProfile) (string, error) {
	input := struct {
		FirstTokenDelay  string          `json:"first_token_delay"`
		StreamDuration   string          `json:"stream_duration"`
		ChunkInterval    string          `json:"chunk_interval"`
		OutputBytes      int             `json:"output_bytes"`
		PromptTokens     int             `json:"prompt_tokens"`
		CompletionTokens int             `json:"completion_tokens"`
		StatusRate       map[int]float64 `json:"status_rate"`
		Seed             int64           `json:"seed"`
	}{
		FirstTokenDelay:  profile.FirstTokenDelay.String(),
		StreamDuration:   profile.StreamDuration.String(),
		ChunkInterval:    profile.ChunkInterval.String(),
		OutputBytes:      profile.OutputBytes,
		PromptTokens:     profile.PromptTokens,
		CompletionTokens: profile.CompletionTokens,
		StatusRate:       profile.StatusRate,
		Seed:             profile.Seed,
	}
	return artifact.HashCanonical(input)
}

func ShouldInjectStatus(seed int64, attempt int64, status int, rate float64) bool {
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	key := fmt.Sprintf("%d:%d:%d", seed, attempt, status)
	sum := sha256.Sum256([]byte(key))
	bucket := binary.BigEndian.Uint64(sum[:8]) % 10000
	return bucket < uint64(rate*10000)
}

func DeterministicErrorCounts(seed int64, total int, statusRate map[int]float64) (actual429 int, actual502 int) {
	statuses := make([]int, 0, len(statusRate))
	for status := range statusRate {
		statuses = append(statuses, status)
	}
	sort.Ints(statuses)
	for attempt := int64(1); attempt <= int64(total); attempt++ {
		for _, status := range statuses {
			if !ShouldInjectStatus(seed, attempt, status, statusRate[status]) {
				continue
			}
			switch status {
			case 429:
				actual429++
			case 502:
				actual502++
			}
			break
		}
	}
	return actual429, actual502
}

func (f File) withDefaults() File {
	if f.Loadtest.SubscriptionKey == "" {
		f.Loadtest.SubscriptionKey = SubscriptionAPIKey
	}
	if f.Loadtest.CompatKey == "" {
		f.Loadtest.CompatKey = CompatAPIKey
	}
	if f.Loadtest.InvalidKey == "" {
		f.Loadtest.InvalidKey = InvalidAPIKey
	}
	if f.Loadtest.TokenDBKeySubscription == "" {
		f.Loadtest.TokenDBKeySubscription = SubscriptionDBKey
	}
	if f.Loadtest.TokenDBKeyCompat == "" {
		f.Loadtest.TokenDBKeyCompat = CompatDBKey
	}
	if f.Server.ProfileBlockRate == 0 {
		f.Server.ProfileBlockRate = 1000
	}
	if f.Server.ProfileMutexFraction == 0 {
		f.Server.ProfileMutexFraction = 5
	}
	if f.Client.MaxIdleConns == 0 {
		f.Client.MaxIdleConns = 64
	}
	if f.Client.MaxIdleConnsPerHost == 0 {
		f.Client.MaxIdleConnsPerHost = 16
	}
	return f
}

func (f File) serverListenAddr() string {
	return net.JoinHostPort(f.Server.Host, strconv.Itoa(f.Server.Port))
}

func (f File) comparisonConfigHash() (string, error) {
	profileHashes := make(map[string]string, len(f.MockProfiles))
	for name, profile := range f.MockProfiles {
		hash, err := HashMockProfile(profile)
		if err != nil {
			return "", err
		}
		profileHashes[name] = hash
	}
	input := struct {
		SchemaVersion int               `json:"schema_version"`
		File          File              `json:"file"`
		Env           map[string]string `json:"env"`
		ProfileHashes map[string]string `json:"profile_hashes"`
	}{
		SchemaVersion: artifact.SchemaVersion,
		File:          f,
		Env:           f.NewAPIEnv(),
		ProfileHashes: profileHashes,
	}
	return artifact.HashCanonical(input)
}

func validateLoadtest(loadtest LoadtestConfig) error {
	if loadtest.Model == "" {
		return fmt.Errorf("loadtest.model is required")
	}
	for _, key := range []string{loadtest.SubscriptionKey, loadtest.CompatKey, loadtest.InvalidKey} {
		if err := localguard.ValidateAPIKey(key); err != nil {
			return fmt.Errorf("loadtest api key: %w", err)
		}
	}
	if loadtest.SubscriptionKey != SubscriptionAPIKey {
		return fmt.Errorf("loadtest.subscription_key must be the fixed loadtest subscription key")
	}
	if loadtest.CompatKey != CompatAPIKey {
		return fmt.Errorf("loadtest.compat_key must be the fixed loadtest compat key")
	}
	if loadtest.InvalidKey != InvalidAPIKey {
		return fmt.Errorf("loadtest.invalid_key must be the fixed loadtest invalid key")
	}
	if loadtest.TokenDBKeySubscription != SubscriptionDBKey {
		return fmt.Errorf("loadtest.token_db_key_subscription must be the fixed loadtest subscription db key")
	}
	if loadtest.TokenDBKeyCompat != CompatDBKey {
		return fmt.Errorf("loadtest.token_db_key_compat must be the fixed loadtest compat db key")
	}
	return nil
}

func validateProfileConfig(name string, cfg ProfileConfig) error {
	name = strings.TrimSpace(name)
	if name == "h2c_diagnostic" || cfg.Transport.Mode == "h2c_diagnostic" {
		return fmt.Errorf("h2c diagnostic profile is not implemented in this phase")
	}
	if name != "benchmark" {
		return fmt.Errorf("loadtest profile is not implemented in this phase")
	}
	if err := validateProfileConfigPositive(name, cfg); err != nil {
		return err
	}
	if !profileConfigMatchesProfile(cfg, profile.Benchmark()) {
		return fmt.Errorf("profiles.benchmark must match the canonical benchmark profile")
	}
	return nil
}

func validateProfileConfigPositive(name string, cfg ProfileConfig) error {
	if len(cfg.Points) == 0 {
		return fmt.Errorf("profiles.%s.points is required", name)
	}
	last := 0
	for _, point := range cfg.Points {
		if point <= 0 || point <= last {
			return fmt.Errorf("profiles.%s.points must be positive and strictly increasing", name)
		}
		last = point
	}
	if cfg.RequestsPerPoint <= 0 || cfg.RampStep <= 0 || cfg.RampInterval.Duration <= 0 || cfg.Duration.Duration <= 0 || cfg.Timeout.Duration <= 0 {
		return fmt.Errorf("profiles.%s requests, ramp, duration, and timeout must be positive", name)
	}
	switch cfg.Transport.Mode {
	case profile.TransportH1KeepAlive, profile.TransportH1NoKeepAlive:
	case "":
		return fmt.Errorf("profiles.%s.transport.mode is required", name)
	default:
		return fmt.Errorf("profiles.%s.transport.mode is not implemented", name)
	}
	if cfg.Transport.MaxConnsPerHost <= 0 || cfg.Transport.MaxIdleConns <= 0 || cfg.Transport.MaxIdleConnsPerHost <= 0 {
		return fmt.Errorf("profiles.%s transport connection limits must be positive", name)
	}
	if cfg.Relay.MaxIdleConns <= 0 || cfg.Relay.MaxIdleConnsPerHost <= 0 {
		return fmt.Errorf("profiles.%s relay connection limits must be positive", name)
	}
	if cfg.Transport.MaxConnsPerHost > 1024 || cfg.Transport.MaxIdleConns > 1024 || cfg.Transport.MaxIdleConnsPerHost > 1024 || cfg.Relay.MaxIdleConns > 1024 || cfg.Relay.MaxIdleConnsPerHost > 1024 {
		return fmt.Errorf("profiles.%s connection limits exceed benchmark maximum", name)
	}
	if cfg.ServerLimits.GOMAXPROCS == "" || cfg.ServerLimits.GOGC == "" || cfg.ServerLimits.GOMEMLIMIT == "" || cfg.ServerLimits.ProcessMemoryLimitBytes == 0 || cfg.ServerLimits.CPUAffinityCores <= 0 {
		return fmt.Errorf("profiles.%s server limits must be fully specified", name)
	}
	return nil
}

func profileConfigMatchesProfile(cfg ProfileConfig, want profile.Profile) bool {
	if !sameIntSlice(cfg.Points, want.Points) {
		return false
	}
	if cfg.RequestsPerPoint != want.RequestsPerPoint ||
		cfg.RampStep != want.RampStep ||
		cfg.RampInterval.Duration != want.RampInterval ||
		cfg.Duration.Duration != want.Duration ||
		cfg.Timeout.Duration != want.Timeout {
		return false
	}
	if cfg.Transport.Mode != want.Transport.Mode ||
		cfg.Transport.MaxConnsPerHost != want.Transport.MaxConnsPerHost ||
		cfg.Transport.MaxIdleConns != want.Transport.MaxIdleConns ||
		cfg.Transport.MaxIdleConnsPerHost != want.Transport.MaxIdleConnsPerHost {
		return false
	}
	if cfg.Relay.MaxIdleConns != want.Relay.MaxIdleConns ||
		cfg.Relay.MaxIdleConnsPerHost != want.Relay.MaxIdleConnsPerHost {
		return false
	}
	return cfg.ServerLimits.GOMAXPROCS == want.ServerLimits.GOMAXPROCS &&
		cfg.ServerLimits.GOGC == want.ServerLimits.GOGC &&
		cfg.ServerLimits.GOMEMLIMIT == want.ServerLimits.GOMEMLIMIT &&
		cfg.ServerLimits.ProcessMemoryLimitBytes == want.ServerLimits.ProcessMemoryLimitBytes &&
		cfg.ServerLimits.CPUAffinityCores == want.ServerLimits.CPUAffinityCores
}

func sameIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func profileFromConfig(name string, cfg ProfileConfig) profile.Profile {
	points := append([]int(nil), cfg.Points...)
	return profile.Profile{
		Name:             name,
		Points:           points,
		RequestsPerPoint: cfg.RequestsPerPoint,
		RampStep:         cfg.RampStep,
		RampInterval:     cfg.RampInterval.Duration,
		Duration:         cfg.Duration.Duration,
		Timeout:          cfg.Timeout.Duration,
		Transport: profile.Transport{
			Mode:                cfg.Transport.Mode,
			MaxConnsPerHost:     cfg.Transport.MaxConnsPerHost,
			MaxIdleConns:        cfg.Transport.MaxIdleConns,
			MaxIdleConnsPerHost: cfg.Transport.MaxIdleConnsPerHost,
		},
		Relay: profile.Relay{
			MaxIdleConns:        cfg.Relay.MaxIdleConns,
			MaxIdleConnsPerHost: cfg.Relay.MaxIdleConnsPerHost,
		},
		ServerLimits: profile.ServerLimits{
			GOMAXPROCS:              cfg.ServerLimits.GOMAXPROCS,
			GOGC:                    cfg.ServerLimits.GOGC,
			GOMEMLIMIT:              cfg.ServerLimits.GOMEMLIMIT,
			ProcessMemoryLimitBytes: cfg.ServerLimits.ProcessMemoryLimitBytes,
			CPUAffinityCores:        cfg.ServerLimits.CPUAffinityCores,
		},
	}
}

func validateMockProfiles(profiles map[string]MockProfile) error {
	required := []string{"s1-smoke", "s2-short-stream", "s3-long-stream", "s4-error-refund", "s5-large-payload"}
	for _, name := range required {
		profile, ok := profiles[name]
		if !ok {
			return fmt.Errorf("mock_profiles.%s is required", name)
		}
		if err := validateMockProfile(name, profile); err != nil {
			return err
		}
	}
	for name, profile := range profiles {
		if err := validateMockProfile(name, profile); err != nil {
			return err
		}
	}
	if err := validateSmokeProfile(profiles["s1-smoke"]); err != nil {
		return err
	}
	return nil
}

func validateMockProfile(name string, profile MockProfile) error {
	prefix := "mock_profiles." + name
	if profile.FirstTokenDelay <= 0 || profile.StreamDuration <= 0 || profile.ChunkInterval <= 0 {
		return fmt.Errorf("%s durations must be positive", prefix)
	}
	if profile.OutputBytes <= 0 || profile.PromptTokens <= 0 || profile.CompletionTokens <= 0 {
		return fmt.Errorf("%s token and output sizes must be positive", prefix)
	}
	if profile.Seed == 0 {
		return fmt.Errorf("%s seed is required", prefix)
	}
	if profile.StatusRate == nil {
		return fmt.Errorf("%s status_rate is required", prefix)
	}
	for _, status := range []int{429, 502} {
		if _, ok := profile.StatusRate[status]; !ok {
			return fmt.Errorf("%s status_rate.%d is required", prefix, status)
		}
	}
	for status, rate := range profile.StatusRate {
		if status != 429 && status != 502 {
			return fmt.Errorf("%s status_rate.%d is not allowed", prefix, status)
		}
		if rate < 0 || rate > 1 {
			return fmt.Errorf("%s status_rate.%d must be between 0 and 1", prefix, status)
		}
	}
	return nil
}

func validateSmokeProfile(profile MockProfile) error {
	if profile.FirstTokenDelay != 50*time.Millisecond ||
		profile.StreamDuration != 500*time.Millisecond ||
		profile.ChunkInterval != 50*time.Millisecond ||
		profile.OutputBytes != 128 ||
		profile.PromptTokens != 11 ||
		profile.CompletionTokens != 17 ||
		profile.StatusRate[429] != 0 ||
		profile.StatusRate[502] != 0 ||
		profile.Seed != 1 {
		return fmt.Errorf("mock_profiles.s1-smoke does not match smoke command parameters")
	}
	return nil
}

func boolEnv(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func redisConnString(addr string) string {
	if strings.Contains(addr, "://") {
		return addr
	}
	return "redis://" + addr + "/0"
}
