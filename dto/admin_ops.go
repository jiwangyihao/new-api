package dto

const (
	AdminOpsHealthStatusHealthy  = "healthy"
	AdminOpsHealthStatusDegraded = "degraded"
	AdminOpsHealthStatusCritical = "critical"

	AdminOpsDependencyStatusHealthy  = "healthy"
	AdminOpsDependencyStatusDisabled = "disabled"
	AdminOpsDependencyStatusCritical = "critical"

	AdminOpsConcurrencyModeRedis    = "redis"
	AdminOpsConcurrencyModeMemory   = "memory"
	AdminOpsConcurrencyModeDisabled = "disabled"
)

type AdminOpsSnapshotResponse struct {
	GeneratedAt  int64                       `json:"generated_at"`
	Health       AdminOpsHealth              `json:"health"`
	Runtime      AdminOpsRuntime             `json:"runtime"`
	System       AdminOpsSystem              `json:"system"`
	Dependencies AdminOpsDependencies        `json:"dependencies"`
	Concurrency  AdminOpsConcurrencyResponse `json:"concurrency"`
	Traffic      AdminOpsTraffic             `json:"traffic"`
	Channels     AdminOpsChannels            `json:"channels"`
	Performance  AdminOpsPerformance         `json:"performance"`
	RecentErrors []AdminOpsRecentError       `json:"recent_errors"`
}

type AdminOpsHealth struct {
	Status  string   `json:"status"`
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
}

type AdminOpsRuntime struct {
	Version           string `json:"version"`
	StartTime         int64  `json:"start_time"`
	UptimeSeconds     int64  `json:"uptime_seconds"`
	NodeName          string `json:"node_name"`
	ActiveConnections int64  `json:"active_connections"`
	Goroutines        int    `json:"goroutines"`
}

type AdminOpsSystem struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
}

type AdminOpsDependencies struct {
	Database AdminOpsDependency `json:"database"`
	Redis    AdminOpsDependency `json:"redis"`
}

type AdminOpsDependency struct {
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Message   string `json:"message"`
}

type AdminOpsConcurrencyResponse struct {
	Mode        string                      `json:"mode"`
	GeneratedAt int64                       `json:"generated_at"`
	Enabled     bool                        `json:"enabled"`
	Summary     AdminOpsConcurrencySummary  `json:"summary"`
	Config      AdminOpsConcurrencyConfig   `json:"config"`
	Counters    AdminOpsConcurrencyCounters `json:"counters"`
	Users       []AdminOpsConcurrencyUser   `json:"users"`
}

type AdminOpsConcurrencySummary struct {
	TotalActive    int64   `json:"total_active"`
	TotalQueued    int64   `json:"total_queued"`
	ActiveUsers    int64   `json:"active_users"`
	QueuedUsers    int64   `json:"queued_users"`
	SaturatedUsers int64   `json:"saturated_users"`
	QueuePressure  float64 `json:"queue_pressure"`
}

type AdminOpsConcurrencyConfig struct {
	TTLSeconds           int  `json:"ttl_seconds"`
	DefaultQueueCapacity int  `json:"default_queue_capacity"`
	RequireRedis         bool `json:"require_redis"`
	FailOpen             bool `json:"fail_open"`
}

type AdminOpsConcurrencyCounters struct {
	AcquiredTotal              int64 `json:"acquired_total"`
	QueuedTotal                int64 `json:"queued_total"`
	QueueFullRejectionsTotal   int64 `json:"queue_full_rejections_total"`
	UnavailableRejectionsTotal int64 `json:"unavailable_rejections_total"`
	RedisErrorsTotal           int64 `json:"redis_errors_total"`
}

type AdminOpsConcurrencyUser struct {
	UserID              int     `json:"user_id"`
	Username            string  `json:"username"`
	Active              int64   `json:"active"`
	Limit               int     `json:"limit"`
	Queued              int64   `json:"queued"`
	QueueCapacity       int     `json:"queue_capacity"`
	OldestQueuedSeconds int64   `json:"oldest_queued_seconds"`
	Utilization         float64 `json:"utilization"`
	QueueUtilization    float64 `json:"queue_utilization"`
	Status              string  `json:"status"`
}

type AdminOpsTraffic struct {
	WindowSeconds int64   `json:"window_seconds"`
	Requests      int64   `json:"requests"`
	Errors        int64   `json:"errors"`
	RPM           float64 `json:"rpm"`
	TPM           float64 `json:"tpm"`
	ErrorRate     float64 `json:"error_rate"`
}

type AdminOpsChannels struct {
	Total          int64 `json:"total"`
	Enabled        int64 `json:"enabled"`
	ManualDisabled int64 `json:"manual_disabled"`
	AutoDisabled   int64 `json:"auto_disabled"`
	SlowCount      int64 `json:"slow_count"`
	StaleTestCount int64 `json:"stale_test_count"`
}

type AdminOpsPerformance struct {
	Models []AdminOpsPerformanceModel `json:"models"`
}

type AdminOpsPerformanceModel struct {
	ModelName    string  `json:"model_name"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	AvgTtftMs    int64   `json:"avg_ttft_ms"`
	SuccessRate  float64 `json:"success_rate"`
	AvgTPS       float64 `json:"avg_tps"`
	RequestCount int64   `json:"request_count"`
}

type AdminOpsRecentError struct {
	ID        int    `json:"id"`
	CreatedAt int64  `json:"created_at"`
	UserID    int    `json:"user_id"`
	Username  string `json:"username"`
	ModelName string `json:"model_name"`
	ChannelID int    `json:"channel_id"`
	Content   string `json:"content"`
	RequestID string `json:"request_id"`
}
