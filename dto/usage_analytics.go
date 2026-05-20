package dto

const (
	UsageAnalyticsGranularityHour = "hour"
	UsageAnalyticsGranularityDay  = "day"
)

type UsageAnalyticsGroupBy string

const (
	UsageAnalyticsGroupByToken  UsageAnalyticsGroupBy = "token"
	UsageAnalyticsGroupByModel  UsageAnalyticsGroupBy = "model"
	UsageAnalyticsGroupByGroup  UsageAnalyticsGroupBy = "group"
	UsageAnalyticsGroupByStream UsageAnalyticsGroupBy = "stream"
	UsageAnalyticsGroupByStatus UsageAnalyticsGroupBy = "status"
)

const (
	UsageAnalyticsMetricRequestCount  = "request_count"
	UsageAnalyticsMetricTotalTokens   = "total_tokens"
	UsageAnalyticsMetricQuota         = "quota"
	UsageAnalyticsMetricErrorRate     = "error_rate"
	UsageAnalyticsMetricAvgLatencyMs  = "avg_latency_ms"
	UsageAnalyticsMetricP95LatencyMs  = "p95_latency_ms"
	UsageAnalyticsMetricLastUsedAt    = "last_used_at"
	UsageAnalyticsMetricFirstUsedAt   = "first_used_at"
	UsageAnalyticsStatusSuccess       = "success"
	UsageAnalyticsStatusError         = "error"
	UsageAnalyticsSortOrderAscending  = "asc"
	UsageAnalyticsSortOrderDescending = "desc"
)

type UsageAnalyticsQuery struct {
	UserID         int
	StartTimestamp int64
	EndTimestamp   int64
	Granularity    string
	GroupBy        UsageAnalyticsGroupBy
	Metric         string
	TokenIDs       []int
	ModelNames     []string
	Groups         []string
	Streams        []bool
	Statuses       []string
	Limit          int
	SortBy         string
	SortOrder      string
}

type UsageAnalyticsMetrics struct {
	RequestCount     int     `json:"request_count"`
	SuccessCount     int     `json:"success_count"`
	ErrorCount       int     `json:"error_count"`
	SuccessRate      float64 `json:"success_rate"`
	ErrorRate        float64 `json:"error_rate"`
	Quota            int     `json:"quota"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	MeteredTokens    int     `json:"metered_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	AvgLatencyMs     int     `json:"avg_latency_ms"`
	P95LatencyMs     int     `json:"p95_latency_ms"`
	FirstUsedAt      int64   `json:"first_used_at"`
	LastUsedAt       int64   `json:"last_used_at"`
	Rpm              int     `json:"rpm"`
	Tpm              int     `json:"tpm"`
	ActiveKeyCount   int     `json:"active_key_count"`
}

type UsageAnalyticsDrilldown struct {
	TokenID   *int    `json:"token_id,omitempty"`
	ModelName *string `json:"model_name,omitempty"`
	Group     *string `json:"group,omitempty"`
	IsStream  *bool   `json:"is_stream,omitempty"`
	Status    *string `json:"status,omitempty"`
}

type UsageAnalyticsTokenInfo struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	MaskedKey      *string `json:"masked_key"`
	Status         *int    `json:"status"`
	Group          *string `json:"group"`
	RemainQuota    *int    `json:"remain_quota"`
	UnlimitedQuota *bool   `json:"unlimited_quota"`
	Deleted        bool    `json:"deleted"`
}

type UsageAnalyticsGroup struct {
	GroupBy          UsageAnalyticsGroupBy    `json:"group_by"`
	GroupKey         string                   `json:"group_key"`
	GroupValue       string                   `json:"group_value"`
	GroupLabel       string                   `json:"group_label"`
	Drilldown        *UsageAnalyticsDrilldown `json:"drilldown"`
	Share            *float64                 `json:"share"`
	Token            *UsageAnalyticsTokenInfo `json:"token"`
	RequestCount     int                      `json:"request_count"`
	SuccessCount     int                      `json:"success_count"`
	ErrorCount       int                      `json:"error_count"`
	SuccessRate      float64                  `json:"success_rate"`
	ErrorRate        float64                  `json:"error_rate"`
	Quota            int                      `json:"quota"`
	PromptTokens     int                      `json:"prompt_tokens"`
	CompletionTokens int                      `json:"completion_tokens"`
	MeteredTokens    int                      `json:"metered_tokens"`
	TotalTokens      int                      `json:"total_tokens"`
	AvgLatencyMs     int                      `json:"avg_latency_ms"`
	P95LatencyMs     int                      `json:"p95_latency_ms"`
	FirstUsedAt      int64                    `json:"first_used_at"`
	LastUsedAt       int64                    `json:"last_used_at"`
}

type UsageAnalyticsTimeseriesPoint struct {
	Timestamp int64  `json:"timestamp"`
	TimeLabel string `json:"time_label"`
	UsageAnalyticsGroup
}

type UsageAnalyticsSummaryResponse struct {
	Total   UsageAnalyticsMetrics `json:"total"`
	Groups  []UsageAnalyticsGroup `json:"groups"`
	GroupBy UsageAnalyticsGroupBy `json:"group_by"`
}

type UsageAnalyticsTimeseriesResponse struct {
	Points      []UsageAnalyticsTimeseriesPoint `json:"points"`
	Granularity string                          `json:"granularity"`
}

type UsageAnalyticsBreakdownResponse struct {
	Groups     []UsageAnalyticsGroup  `json:"groups"`
	TotalGroups int                   `json:"total_groups"`
	Other      *UsageAnalyticsGroup   `json:"other"`
	SortBy     string                 `json:"sort_by"`
	SortOrder  string                 `json:"sort_order"`
}
