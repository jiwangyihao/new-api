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

type Summary struct {
	SchemaVersion       int             `json:"schema_version"`
	RunContext          RunContext      `json:"run_context"`
	Path                string          `json:"path"`
	Scenario            string          `json:"scenario"`
	TokenProfile        string          `json:"token_profile"`
	Model               string          `json:"model"`
	TargetConcurrency   int             `json:"target_concurrency,omitempty"`
	Total               int             `json:"total"`
	Success             int             `json:"success"`
	Errors              int             `json:"errors"`
	StatusCodes         map[string]int  `json:"status_codes,omitempty"`
	ErrorReasons        map[string]int  `json:"error_reasons,omitempty"`
	MaxObservedInFlight int             `json:"max_observed_in_flight,omitempty"`
	LatencyP95MS        float64         `json:"latency_p95_ms,omitempty"`
	TTFTP95MS           float64         `json:"ttft_p95_ms,omitempty"`
	RequestsPerSecond   float64         `json:"requests_per_second,omitempty"`
	Stream              StreamStats     `json:"stream,omitempty"`
	Requests            []RequestRecord `json:"requests,omitempty"`
	StopReason          string          `json:"stop_reason,omitempty"`
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
	PID          int     `json:"pid,omitempty"`
	RSSBytes     uint64  `json:"rss_bytes"`
	CPUPercent   float64 `json:"cpu_percent,omitempty"`
	RSSPeakBytes uint64  `json:"rss_peak_bytes,omitempty"`
}

type PostgresSnapshot struct {
	Statused
	Rows map[string]int64 `json:"rows,omitempty"`
}

type RedisSnapshot struct {
	Statused
	Info map[string]string `json:"info,omitempty"`
}

type RuntimeSnapshot struct {
	Statused
	Goroutines           int      `json:"goroutines,omitempty"`
	HeapAllocBytes       uint64   `json:"heap_alloc_bytes,omitempty"`
	BlockProfileRate     int      `json:"block_profile_rate,omitempty"`
	MutexProfileFraction int      `json:"mutex_profile_fraction,omitempty"`
	BatchUpdate          Statused `json:"batch_update,omitempty"`
	QuotaData            Statused `json:"quota_data,omitempty"`
	PerfMetrics          Statused `json:"perf_metrics,omitempty"`
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
	Actual429             int            `json:"actual_429,omitempty"`
	Actual502             int            `json:"actual_502,omitempty"`
	UpstreamAttemptsTotal int            `json:"upstream_attempts_total,omitempty"`
	NonInjectedErrors     int            `json:"non_injected_errors,omitempty"`
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
	RSSPeakBytes       uint64 `json:"rss_peak_bytes,omitempty"`
	GoroutinesPeak     int    `json:"goroutines_peak,omitempty"`
	HeapAllocPeakBytes uint64 `json:"heap_alloc_peak_bytes,omitempty"`
}

type GateResult struct {
	Passed        bool     `json:"passed"`
	FailedReasons []string `json:"failed_reasons,omitempty"`
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
	ResourcePeaks     ResourcePeaks  `json:"resource_peaks,omitempty"`
	ResourceDelta     ResourceDelta  `json:"resource_delta,omitempty"`
	ProfilePaths      ProfilePaths   `json:"profile_paths,omitempty"`
	Gate              GateResult     `json:"gate,omitempty"`
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
