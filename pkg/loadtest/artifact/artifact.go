package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const SchemaVersion = 1

type RunContext struct {
	SchemaVersion        int    `json:"schema_version"`
	Role                 string `json:"role"`
	Commit               string `json:"commit"`
	ComparisonConfigHash string `json:"comparison_config_hash"`
	SeedOutputHash       string `json:"seed_output_hash,omitempty"`
	MockHash             string `json:"mock_hash,omitempty"`
	CacheMode            string `json:"cache_mode"`
	Scenario             string `json:"scenario,omitempty"`
	Path                 string `json:"path,omitempty"`
	TokenProfile         string `json:"token_profile,omitempty"`
	Model                string `json:"model"`
}

func (r RunContext) WithoutSeedOutputHash() RunContext {
	r.SeedOutputHash = ""
	return r
}

func (r RunContext) WithoutMockHash() RunContext {
	r.MockHash = ""
	return r
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type StreamStats struct {
	DoneReceived bool  `json:"done_received"`
	UsageEvents  int   `json:"usage_events"`
	Bytes        int64 `json:"bytes"`
}

type RequestRecord struct {
	RequestIndex      int    `json:"request_index"`
	ClientRequestID   string `json:"client_request_id"`
	NewAPIRequestID   string `json:"new_api_request_id"`
	UpstreamRequestID string `json:"upstream_request_id"`
	MockRequestID     string `json:"mock_request_id,omitempty"`
	StatusCode        int    `json:"status_code"`
	Success           bool   `json:"success"`
	ErrorReason       string `json:"error_reason,omitempty"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	TotalTokens       int    `json:"total_tokens"`
}

type TransportProfile struct {
	Mode                string `json:"mode,omitempty"`
	MaxConnsPerHost     int    `json:"max_conns_per_host,omitempty"`
	MaxIdleConns        int    `json:"max_idle_conns,omitempty"`
	MaxIdleConnsPerHost int    `json:"max_idle_conns_per_host,omitempty"`
}

type ErrorSample struct {
	RequestIndex int     `json:"request_index"`
	Phase        string  `json:"phase"`
	Reason       string  `json:"reason"`
	StatusCode   int     `json:"status_code,omitempty"`
	LatencyMS    float64 `json:"latency_ms,omitempty"`
	RequestID    string  `json:"request_id,omitempty"`
}

type Summary struct {
	SchemaVersion       int              `json:"schema_version"`
	RunContext          RunContext       `json:"run_context"`
	Path                string           `json:"path"`
	Scenario            string           `json:"scenario"`
	TokenProfile        string           `json:"token_profile"`
	Model               string           `json:"model"`
	TargetConcurrency   int              `json:"target_concurrency,omitempty"`
	Total               int              `json:"total"`
	Success             int              `json:"success"`
	Errors              int              `json:"errors"`
	StatusCodes         map[string]int   `json:"status_codes,omitempty"`
	ErrorReasons        map[string]int   `json:"error_reasons,omitempty"`
	ProtocolCounts      map[string]int   `json:"protocol_counts,omitempty"`
	MaxObservedInFlight int              `json:"max_observed_in_flight,omitempty"`
	LatencyP95MS        float64          `json:"latency_p95_ms,omitempty"`
	TTFTP95MS           float64          `json:"ttft_p95_ms,omitempty"`
	RequestsPerSecond   float64          `json:"requests_per_second,omitempty"`
	Stream              StreamStats      `json:"stream,omitempty"`
	Requests            []RequestRecord  `json:"requests,omitempty"`
	FirstErrorSamples   []ErrorSample    `json:"first_error_samples,omitempty"`
	Transport           TransportProfile `json:"transport,omitzero"`
	StopReason          string           `json:"stop_reason,omitempty"`
}

type SeedOutput struct {
	SchemaVersion           int            `json:"schema_version"`
	RunContext              RunContext     `json:"run_context"`
	UserIDSubscription      int            `json:"user_id_subscription"`
	UserIDCompat            int            `json:"user_id_compat"`
	TokenSubscription       string         `json:"token_subscription"`
	TokenCompat             string         `json:"token_compat"`
	TokenDBKeySubscription  string         `json:"token_db_key_subscription"`
	TokenDBKeyCompat        string         `json:"token_db_key_compat"`
	ChannelID               int            `json:"channel_id"`
	Model                   string         `json:"model"`
	Group                   string         `json:"group"`
	MockBaseURL             string         `json:"mock_base_url"`
	ExpectedUsagePerSuccess Usage          `json:"expected_usage_per_success"`
	RatioConfig             map[string]any `json:"ratio_config,omitempty"`
	FeatureOptions          map[string]any `json:"feature_options,omitempty"`
}

type MockAttempt struct {
	AttemptIndex      int    `json:"attempt_index"`
	Method            string `json:"method"`
	Path              string `json:"path"`
	UpstreamRequestID string `json:"upstream_request_id"`
	InjectedStatus    int    `json:"injected_status"`
}

type MockStats struct {
	SchemaVersion        int            `json:"schema_version"`
	RunContext           RunContext     `json:"run_context"`
	AttemptsTotal        int            `json:"attempts_total"`
	InjectedStatusCounts map[string]int `json:"injected_status_counts,omitempty"`
	Attempts             []MockAttempt  `json:"attempts,omitempty"`
	MockHash             string         `json:"mock_hash,omitempty"`
}

type MockStatsDelta struct {
	SchemaVersion         int        `json:"schema_version"`
	RunContext            RunContext `json:"run_context"`
	Path                  string     `json:"path,omitempty"`
	Hash                  string     `json:"hash,omitempty"`
	Actual429             int        `json:"actual_429"`
	Actual502             int        `json:"actual_502"`
	UpstreamAttemptsTotal int        `json:"upstream_attempts_total"`
}

type Statused struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type ProcessSnapshot struct {
	Statused
	PID            int     `json:"pid,omitempty"`
	RSSBytes       uint64  `json:"rss_bytes"`
	CPUPercent     float64 `json:"cpu_percent,omitempty"`
	RSSPeakBytes   uint64  `json:"rss_peak_bytes,omitempty"`
	ThreadCount    int     `json:"thread_count,omitempty"`
	HandleCount    int     `json:"handle_count,omitempty"`
	OpenTCPSockets int     `json:"open_tcp_sockets,omitempty"`
	CPUTimeSeconds float64 `json:"cpu_time_seconds,omitempty"`
}

type PostgresSnapshot struct {
	Statused
	Rows              map[string]int64 `json:"rows,omitempty"`
	ActiveConnections int              `json:"active_connections,omitempty"`
	IdleConnections   int              `json:"idle_connections,omitempty"`
	WaitingLocks      int              `json:"waiting_locks,omitempty"`
	DatabaseSizeBytes uint64           `json:"database_size_bytes,omitempty"`
}

type RedisSnapshot struct {
	Statused
	Info                    map[string]string `json:"info,omitempty"`
	ConnectedClients        int               `json:"connected_clients,omitempty"`
	UsedMemoryBytes         uint64            `json:"used_memory_bytes,omitempty"`
	UsedMemoryRSSBytes      uint64            `json:"used_memory_rss_bytes,omitempty"`
	MemFragmentationRatio   float64           `json:"mem_fragmentation_ratio,omitempty"`
	InstantaneousOpsPerSec  int               `json:"instantaneous_ops_per_sec,omitempty"`
	TotalCommandsProcessed  uint64            `json:"total_commands_processed,omitempty"`
	Keyspace                map[string]int64  `json:"keyspace,omitempty"`
}

type RuntimeSnapshot struct {
	Statused
	Goroutines           int              `json:"goroutines,omitempty"`
	HeapAllocBytes       uint64           `json:"heap_alloc_bytes,omitempty"`
	HeapSysBytes         uint64           `json:"heap_sys_bytes,omitempty"`
	GOMAXPROCS           int              `json:"gomaxprocs,omitempty"`
	GOMEMLimitBytes      int64            `json:"gomemlimit_bytes,omitempty"`
	GCCount              uint32           `json:"gc_count,omitempty"`
	LastGCUnixMS         int64            `json:"last_gc_unix_ms,omitempty"`
	PauseTotalNS         uint64           `json:"pause_total_ns,omitempty"`
	HTTPConnState        map[string]int64 `json:"http_conn_state,omitempty"`
	HTTPAcceptTotal      uint64           `json:"http_accept_total,omitempty"`
	HTTPActiveCurrent    int64            `json:"http_active_current,omitempty"`
	BlockProfileRate     int              `json:"block_profile_rate,omitempty"`
	MutexProfileFraction int              `json:"mutex_profile_fraction,omitempty"`
	BatchUpdate          Statused         `json:"batch_update,omitempty"`
	QuotaData            Statused         `json:"quota_data,omitempty"`
	PerfMetrics          Statused         `json:"perf_metrics,omitempty"`
}

type LogsSnapshot struct {
	Statused
	StdoutFullParamsLines  int `json:"stdout_full_params_lines"`
	PerfMetricUpsertErrors int `json:"perf_metric_upsert_errors"`
}

type Snapshot struct {
	SchemaVersion int              `json:"schema_version"`
	RunContext    RunContext       `json:"run_context"`
	Process       ProcessSnapshot  `json:"process,omitempty"`
	Postgres      PostgresSnapshot `json:"postgres,omitempty"`
	Redis         RedisSnapshot    `json:"redis,omitempty"`
	Runtime       RuntimeSnapshot  `json:"runtime,omitempty"`
	Logs          LogsSnapshot     `json:"logs,omitempty"`
	Business      BusinessSnapshot `json:"business,omitempty"`
}

type PostgresDelta struct{ Statused }
type RedisDelta struct{ Statused }
type RuntimeDelta struct{ Statused }

type ResourceDelta struct {
	RSSBeforeBytes       uint64 `json:"rss_before_bytes,omitempty"`
	RSSAfterDrainBytes   uint64 `json:"rss_after_drain_bytes,omitempty"`
	GoroutinesBefore     int    `json:"goroutines_before,omitempty"`
	GoroutinesAfterDrain int    `json:"goroutines_after_drain,omitempty"`
}

type BusinessSnapshot struct {
	Statused
	SubscriptionTokenUsed       int64 `json:"subscription_token_used,omitempty"`
	CompatSubscriptionTokenUsed int64 `json:"compat_subscription_token_used,omitempty"`
	SubscriptionUserQuota       int   `json:"subscription_user_quota,omitempty"`
	CompatUserQuota             int   `json:"compat_user_quota,omitempty"`
	SubscriptionTokenRemain     int   `json:"subscription_token_remain,omitempty"`
	CompatTokenRemain           int   `json:"compat_token_remain,omitempty"`
}

type ChargeByRequest struct {
	NewAPIRequestID           string `json:"new_api_request_id"`
	ClientRequestID           string `json:"client_request_id,omitempty"`
	UpstreamRequestID         string `json:"upstream_request_id,omitempty"`
	StatusCode                int    `json:"status_code"`
	Success                   bool   `json:"success"`
	LogQuota                  int    `json:"log_quota,omitempty"`
	SubscriptionTokenDelta    int    `json:"subscription_token_delta,omitempty"`
	NetSubscriptionTokenDelta int    `json:"net_subscription_token_delta,omitempty"`
}

type Invariant struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type BusinessDelta struct {
	Statused
	ChargesByRequest []ChargeByRequest `json:"charges_by_request,omitempty"`
	Invariants       []Invariant       `json:"invariants,omitempty"`
}

type Diff struct {
	SchemaVersion      int              `json:"schema_version"`
	RunContext         RunContext       `json:"run_context"`
	BeforePath         string           `json:"before_path,omitempty"`
	AfterPath          string           `json:"after_path,omitempty"`
	SummaryPath        string           `json:"summary_path,omitempty"`
	MockStatsDeltaPath string           `json:"mock_stats_delta_path,omitempty"`
	MockStatsDeltaHash string           `json:"mock_stats_delta_hash,omitempty"`
	MockDelta          MockStatsDelta   `json:"mock_delta,omitempty"`
	BusinessSnapshot   BusinessSnapshot `json:"business_snapshot,omitempty"`
	BusinessDelta      BusinessDelta    `json:"business_delta,omitempty"`
	PostgresDelta      PostgresDelta    `json:"postgres_delta,omitempty"`
	RedisDelta         RedisDelta       `json:"redis_delta,omitempty"`
	RuntimeDelta       RuntimeDelta     `json:"runtime_delta,omitempty"`
	ResourceDelta      ResourceDelta    `json:"resource_delta,omitempty"`
	Logs               LogsSnapshot     `json:"logs,omitempty"`
}

type SummaryExcerpt struct {
	Total                 int            `json:"total"`
	Success               int            `json:"success"`
	Errors                int            `json:"errors,omitempty"`
	StatusCodes           map[string]int `json:"status_codes,omitempty"`
	LatencyP95MS          float64        `json:"latency_p95_ms,omitempty"`
	TTFTP95MS             float64        `json:"ttft_p95_ms,omitempty"`
	RequestsPerSecond     float64        `json:"requests_per_second,omitempty"`
	MaxObservedInFlight   int            `json:"max_observed_in_flight,omitempty"`
	StreamDoneReceived    int            `json:"stream_done_received,omitempty"`
	StreamUsageEvents     int            `json:"stream_usage_events,omitempty"`
	StreamBytes           int64          `json:"stream_bytes,omitempty"`
	StopReason            string         `json:"stop_reason,omitempty"`
	Actual429             int            `json:"actual_429,omitempty"`
	Actual502             int            `json:"actual_502,omitempty"`
	UpstreamAttemptsTotal int            `json:"upstream_attempts_total,omitempty"`
	NonInjectedErrors     int            `json:"non_injected_errors,omitempty"`
}

type ResourceLimitsArtifact struct {
	SchemaVersion                 int               `json:"schema_version"`
	RunContext                    RunContext        `json:"run_context"`
	TargetProcess                 string            `json:"target_process"`
	OSProcessMemoryLimitEnforced  bool              `json:"os_process_memory_limit_enforced"`
	OSCPUAffinityEnforced         bool              `json:"os_cpu_affinity_enforced"`
	ServerCPUAffinityCores        int               `json:"server_cpu_affinity_cores"`
	ServerCPUAffinityMask         uintptr           `json:"server_cpu_affinity_mask"`
	ServerProcessMemoryLimitBytes uint64            `json:"server_process_memory_limit_bytes"`
	ServerEnv                     map[string]string `json:"server_env"`
	Scope                         string            `json:"scope"`
	Statused
}

type ResourceSample struct {
	UnixMilli int64            `json:"unix_milli"`
	Process   ProcessSnapshot  `json:"process,omitempty"`
	Runtime   RuntimeSnapshot  `json:"runtime,omitempty"`
	Postgres  PostgresSnapshot `json:"postgres,omitempty"`
	Redis     RedisSnapshot    `json:"redis,omitempty"`
}

type ProfilePaths struct {
	CPU       string `json:"cpu,omitempty"`
	Heap      string `json:"heap,omitempty"`
	Block     string `json:"block,omitempty"`
	Mutex     string `json:"mutex,omitempty"`
	Goroutine string `json:"goroutine,omitempty"`
	Statused
}

type ResourcePeaks struct {
	RSSPeakBytes                     uint64  `json:"rss_peak_bytes,omitempty"`
	CPUPercentPeak                   float64 `json:"cpu_percent_peak,omitempty"`
	CPUTimeSecondsPeak               float64 `json:"cpu_time_seconds_peak,omitempty"`
	ThreadCountPeak                  int     `json:"thread_count_peak,omitempty"`
	HandleCountPeak                  int     `json:"handle_count_peak,omitempty"`
	OpenTCPSocketsPeak               int     `json:"open_tcp_sockets_peak,omitempty"`
	GoroutinesPeak                   int     `json:"goroutines_peak,omitempty"`
	HeapAllocPeakBytes               uint64  `json:"heap_alloc_peak_bytes,omitempty"`
	HeapSysPeakBytes                 uint64  `json:"heap_sys_peak_bytes,omitempty"`
	GCCountPeak                      uint32  `json:"gc_count_peak,omitempty"`
	PauseTotalNSPeak                 uint64  `json:"pause_total_ns_peak,omitempty"`
	HTTPAcceptTotalPeak              uint64  `json:"http_accept_total_peak,omitempty"`
	HTTPActiveCurrentPeak            int64   `json:"http_active_current_peak,omitempty"`
	RedisConnectedClientsPeak        int     `json:"redis_connected_clients_peak,omitempty"`
	RedisUsedMemoryPeakBytes         uint64  `json:"redis_used_memory_peak_bytes,omitempty"`
	RedisUsedMemoryRSSPeakBytes      uint64  `json:"redis_used_memory_rss_peak_bytes,omitempty"`
	RedisInstantaneousOpsPeak        int     `json:"redis_instantaneous_ops_peak,omitempty"`
	RedisTotalCommandsProcessedPeak  uint64  `json:"redis_total_commands_processed_peak,omitempty"`
	PostgresActiveConnectionsPeak    int     `json:"postgres_active_connections_peak,omitempty"`
	PostgresIdleConnectionsPeak      int     `json:"postgres_idle_connections_peak,omitempty"`
	PostgresWaitingLocksPeak         int     `json:"postgres_waiting_locks_peak,omitempty"`
	PostgresDatabaseSizePeakBytes    uint64  `json:"postgres_database_size_peak_bytes,omitempty"`
}

type ResourceSamplesArtifact struct {
	SchemaVersion int              `json:"schema_version"`
	RunContext    RunContext       `json:"run_context"`
	Concurrency   int              `json:"concurrency"`
	Samples       []ResourceSample `json:"samples"`
	Peaks         ResourcePeaks    `json:"peaks"`
	Drain         Statused         `json:"drain,omitempty"`
}

type PortsClosedArtifact struct {
	SchemaVersion int               `json:"schema_version"`
	RunContext    RunContext        `json:"run_context"`
	Ports         map[string]string `json:"ports"`
	Passed        bool              `json:"passed"`
}

type GateResult struct {
	Passed            bool     `json:"passed"`
	FailedReasons     []string `json:"failed_reasons,omitempty"`
	DiagnosticReasons []string `json:"diagnostic_reasons,omitempty"`
}

type PointAnalysis struct {
	SchemaVersion  int        `json:"schema_version"`
	RunContext     RunContext `json:"run_context"`
	Concurrency    int        `json:"concurrency"`
	FailureClass   string     `json:"failure_class"`
	HardGate       GateResult `json:"hard_gate"`
	DiagnosticGate GateResult `json:"diagnostic_gate,omitzero"`
}

type PointResult struct {
	Concurrency       int            `json:"concurrency"`
	Passed            bool           `json:"passed"`
	SummaryPath       string         `json:"summary_path,omitempty"`
	MetricsBeforePath string         `json:"metrics_before_path,omitempty"`
	MetricsAfterPath  string         `json:"metrics_after_path,omitempty"`
	MetricsDiffPath   string         `json:"metrics_diff_path,omitempty"`
	SummaryExcerpt    SummaryExcerpt `json:"summary_excerpt"`
	MockDelta         MockStatsDelta `json:"mock_delta"`
	Invariants        []Invariant    `json:"invariants,omitempty"`
	ResourcePeaks ResourcePeaks  `json:"resource_peaks,omitzero"`
	ResourceDelta ResourceDelta  `json:"resource_delta,omitzero"`
	ProfilePaths  ProfilePaths   `json:"profile_paths,omitzero"`
	Gate          GateResult     `json:"gate,omitzero"`
}

type SweepResult struct {
	SchemaVersion            int           `json:"schema_version"`
	RunContext               RunContext    `json:"run_context"`
	RunID                    string        `json:"run_id,omitempty"`
	Scenario                 string        `json:"scenario,omitempty"`
	Path                     string        `json:"path,omitempty"`
	TokenProfile             string        `json:"token_profile,omitempty"`
	Points                   []PointResult `json:"points"`
	FirstFailedConcurrency   *int          `json:"first_failed_concurrency"`
	HighestPassedConcurrency int           `json:"highest_passed_concurrency"`
}

func MarshalCanonical(v any) ([]byte, error) {
	return common.Marshal(v)
}

func HashCanonical(v any) (string, error) {
	b, err := MarshalCanonical(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func HashSeedOutput(seed SeedOutput) (string, error) {
	seed.RunContext = seed.RunContext.WithoutSeedOutputHash().WithoutMockHash()
	return HashCanonical(seed)
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)OPENAI_API_KEY=[^\s]+`),
	regexp.MustCompile(`(?i)sk-[A-Za-z0-9_-]+`),
}

func Redact(s string) string {
	out := s
	for _, re := range secretPatterns {
		out = re.ReplaceAllString(out, "[REDACTED]")
	}
	fields := strings.Fields(out)
	for i, field := range fields {
		if u, err := url.Parse(field); err == nil && u.Scheme != "" && u.Host != "" {
			if u.User != nil {
				u.User = url.UserPassword("[REDACTED]", "[REDACTED]")
			}
			if !isSafeHostForDisplay(u.Hostname()) {
				u.Host = "[REDACTED]"
			}
			fields[i] = u.String()
		}
	}
	out = strings.Join(fields, " ")
	out = strings.ReplaceAll(out, "example.com", "[REDACTED]")
	out = strings.ReplaceAll(out, "secret", "[REDACTED]")
	return out
}

func isSafeHostForDisplay(host string) bool {
	host = strings.ToLower(host)
	return host == "" || host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1"
}

func SortedInvariantNames(in []Invariant) []string {
	out := make([]string, 0, len(in))
	for _, inv := range in {
		out = append(out, inv.Name)
	}
	sort.Strings(out)
	return out
}
