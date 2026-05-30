package service

import (
	"context"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
)

type AdminOpsSnapshotQuery struct {
	WindowSeconds int64
	Top           int
}

type AdminOpsConcurrencyQuery struct {
	Limit             int
	IncludeUsers      bool
	MinActiveOrQueued int64
	PlanID            int
	Status            string
	Search            string
}

type adminOpsHealthSeverity int

const (
	adminOpsHealthSeverityDegraded adminOpsHealthSeverity = iota + 1
	adminOpsHealthSeverityCritical
)

type adminOpsHealthReason struct {
	Code     string
	Severity adminOpsHealthSeverity
}

const (
	adminOpsDefaultWindowSeconds = int64(300)
	adminOpsDefaultTop           = 5
	adminOpsMaxTop               = 20
	adminOpsChannelSlowMs        = 5000
	adminOpsChannelStaleSeconds  = int64(24 * 3600)
)

var (
	adminOpsAuthorizationHeaderPattern = regexp.MustCompile(`(?i)"?authorization"?\s*:\s*"?(?:(?:bearer|basic|token)\s+)?[^"\s,;}]+"?`)
	adminOpsBearerTokenPattern         = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._\-]{12,}`)
	adminOpsAPIKeyPattern              = regexp.MustCompile(`(?i)\b(?:sk|ak)-[a-z0-9][a-z0-9._\-]{6,}`)
	adminOpsImageDataPattern           = regexp.MustCompile(`(?i)data:image/[a-z0-9.+\-]+;base64,[a-z0-9+/=\r\n]{16,}`)
	adminOpsLongBase64Pattern          = regexp.MustCompile(`\b[A-Za-z0-9+/]{48,}={0,2}\b`)
	adminOpsPromptValuePattern         = regexp.MustCompile(`(?is)\"?(prompt|messages|request_body|request body|body|input)\"?\s*[:=]\s*("(?:\\.|[^"\\]){0,1000}"|'(?:\\.|[^'\\]){0,1000}'|\[[\s\S]{0,1000}?\]|\{[\s\S]{0,1000}?\}|[^,;\n]{0,1000})`)
)

func GetAdminOpsSnapshot(ctx context.Context, query AdminOpsSnapshotQuery) (*dto.AdminOpsSnapshotResponse, error) {
	query = normalizeAdminOpsSnapshotQuery(query)
	generatedAt := time.Now().Unix()
	start := generatedAt - query.WindowSeconds

	dependencies := buildAdminOpsDependencies(ctx)
	concurrency, err := GetAdminOpsConcurrency(ctx, AdminOpsConcurrencyQuery{Limit: query.Top, IncludeUsers: false, MinActiveOrQueued: 1})
	if err != nil {
		return nil, err
	}
	trafficStats, err := model.GetAdminOpsTrafficStats(start, generatedAt)
	if err != nil {
		return nil, err
	}
	channelStats, err := model.GetAdminOpsChannelStats(generatedAt, adminOpsChannelSlowMs, adminOpsChannelStaleSeconds)
	if err != nil {
		return nil, err
	}
	perfSummary, err := perfmetrics.QuerySummaryAll(24)
	if err != nil {
		return nil, err
	}
	recentErrors, err := model.GetAdminOpsRecentErrors(start, query.Top)
	if err != nil {
		return nil, err
	}

	runtimeStats := buildAdminOpsRuntime(generatedAt)
	system := buildAdminOpsSystem(common.RefreshSystemStatus())
	traffic := buildAdminOpsTraffic(query.WindowSeconds, trafficStats)
	channels := dto.AdminOpsChannels{
		Total:          channelStats.Total,
		Enabled:        channelStats.Enabled,
		ManualDisabled: channelStats.ManualDisabled,
		AutoDisabled:   channelStats.AutoDisabled,
		SlowCount:      channelStats.SlowCount,
		StaleTestCount: channelStats.StaleTestCount,
	}
	performance := buildAdminOpsPerformance(perfSummary, query.Top)
	errorsDTO := buildAdminOpsRecentErrors(recentErrors)
	reasons := buildAdminOpsSnapshotHealthReasons(dependencies, system, concurrency.Counters, channels, traffic)

	return &dto.AdminOpsSnapshotResponse{
		GeneratedAt:  generatedAt,
		Health:       buildAdminOpsHealth(reasons),
		Runtime:      runtimeStats,
		System:       system,
		Dependencies: dependencies,
		Concurrency:  *concurrency,
		Traffic:      traffic,
		Channels:     channels,
		Performance:  performance,
		RecentErrors: errorsDTO,
	}, nil
}

func GetAdminOpsConcurrency(ctx context.Context, query AdminOpsConcurrencyQuery) (*dto.AdminOpsConcurrencyResponse, error) {
	query = normalizeAdminOpsConcurrencyQuery(query)
	generatedAt := time.Now().Unix()
	mode := adminOpsConcurrencyMode()
	config := dto.AdminOpsConcurrencyConfig{
		TTLSeconds:           adminOpsConcurrencyTTLSeconds(),
		DefaultQueueCapacity: adminOpsConcurrencyDefaultQueueCapacity(),
		RequireRedis:         common.SubscriptionConcurrencyRequireRedis,
		FailOpen:             common.SubscriptionConcurrencyFailOpen,
	}
	counters := adminOpsConcurrencyCountersDTO(SubscriptionConcurrencyCountersSnapshot())

	response := &dto.AdminOpsConcurrencyResponse{
		Mode:        mode,
		GeneratedAt: generatedAt,
		Enabled:     mode != dto.AdminOpsConcurrencyModeDisabled,
		Config:      config,
		Counters:    counters,
		Users:       []dto.AdminOpsConcurrencyUser{},
	}
	if !response.Enabled {
		return response, nil
	}

	rows, err := adminOpsSubscriptionConcurrencyRows(ctx, mode, time.Unix(generatedAt, 0))
	if err != nil {
		recordSubscriptionConcurrencyRedisError()
		response.Counters = adminOpsConcurrencyCountersDTO(SubscriptionConcurrencyCountersSnapshot())
		return response, nil
	}
	users, err := buildAdminOpsConcurrencyUsers(rows, query.IncludeUsers)
	if err != nil {
		return nil, err
	}
	response.Summary = buildAdminOpsConcurrencySummary(users)
	if query.IncludeUsers {
		response.Users = limitAdminOpsConcurrencyUsers(filterAdminOpsConcurrencyUsers(users, query), query.Limit)
	}
	return response, nil
}

func normalizeAdminOpsSnapshotQuery(query AdminOpsSnapshotQuery) AdminOpsSnapshotQuery {
	switch query.WindowSeconds {
	case 60, 300, 900, 3600:
	default:
		query.WindowSeconds = adminOpsDefaultWindowSeconds
	}
	if query.Top <= 0 {
		query.Top = adminOpsDefaultTop
	}
	if query.Top > adminOpsMaxTop {
		query.Top = adminOpsMaxTop
	}
	return query
}

func normalizeAdminOpsConcurrencyQuery(query AdminOpsConcurrencyQuery) AdminOpsConcurrencyQuery {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	query.Status = strings.TrimSpace(query.Status)
	query.Search = strings.TrimSpace(query.Search)
	if query.MinActiveOrQueued < 0 {
		query.MinActiveOrQueued = 1
	}
	return query
}

func buildAdminOpsHealth(reasons []adminOpsHealthReason) dto.AdminOpsHealth {
	status := dto.AdminOpsHealthStatusHealthy
	score := 100
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, reason.Code)
		switch reason.Severity {
		case adminOpsHealthSeverityCritical:
			status = dto.AdminOpsHealthStatusCritical
			score -= 30
		case adminOpsHealthSeverityDegraded:
			if status != dto.AdminOpsHealthStatusCritical {
				status = dto.AdminOpsHealthStatusDegraded
			}
			score -= 10
		}
	}
	if score < 0 {
		score = 0
	}
	return dto.AdminOpsHealth{Status: status, Score: score, Reasons: codes}
}

func adminOpsConcurrencyHealthReasons(counters dto.AdminOpsConcurrencyCounters) []adminOpsHealthReason {
	reasons := make([]adminOpsHealthReason, 0, 2)
	if counters.RedisErrorsTotal > 0 {
		reasons = append(reasons, adminOpsHealthReason{Code: "concurrency_redis_errors", Severity: adminOpsHealthSeverityCritical})
	}
	if counters.UnavailableRejectionsTotal > 0 {
		reasons = append(reasons, adminOpsHealthReason{Code: "concurrency_unavailable_rejections", Severity: adminOpsHealthSeverityCritical})
	}
	return reasons
}

func sanitizeAdminOpsRecentErrorContent(content string) string {
	masked := maskAdminOpsSensitiveErrorFragments(content)
	masked = common.MaskSensitiveInfo(masked)
	return truncateAdminOpsRunes(masked, 300)
}

func maskAdminOpsSensitiveErrorFragments(content string) string {
	if content == "" {
		return ""
	}
	masked := adminOpsAuthorizationHeaderPattern.ReplaceAllString(content, "[authorization_redacted]")
	masked = adminOpsBearerTokenPattern.ReplaceAllString(masked, "[bearer_redacted]")
	masked = adminOpsAPIKeyPattern.ReplaceAllString(masked, "[api_key_redacted]")
	masked = adminOpsImageDataPattern.ReplaceAllString(masked, "[image_redacted]")
	masked = adminOpsLongBase64Pattern.ReplaceAllString(masked, "[base64_redacted]")
	masked = adminOpsPromptValuePattern.ReplaceAllString(masked, "$1: [redacted]")
	return masked
}

func truncateAdminOpsRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	var builder strings.Builder
	builder.Grow(limit)
	count := 0
	for _, r := range value {
		if count >= limit {
			break
		}
		builder.WriteRune(r)
		count++
	}
	return builder.String()
}

func buildAdminOpsConcurrencySummary(users []dto.AdminOpsConcurrencyUser) dto.AdminOpsConcurrencySummary {
	var summary dto.AdminOpsConcurrencySummary
	var totalQueueCapacity int64
	for _, user := range users {
		summary.TotalActive += user.Active
		summary.TotalQueued += user.Queued
		if user.Active > 0 {
			summary.ActiveUsers++
		}
		if user.Queued > 0 {
			summary.QueuedUsers++
		}
		if user.Limit > 0 && user.Active >= int64(user.Limit) {
			summary.SaturatedUsers++
		}
		if user.QueueCapacity > 0 {
			totalQueueCapacity += int64(user.QueueCapacity)
		}
	}
	if totalQueueCapacity > 0 {
		summary.QueuePressure = float64(summary.TotalQueued) / float64(totalQueueCapacity)
	}
	maxQueueUtilization := buildAdminOpsConcurrencyMaxQueueUtilization(users)
	if maxQueueUtilization > summary.QueuePressure {
		summary.QueuePressure = maxQueueUtilization
	}
	return summary
}

func buildAdminOpsConcurrencyMaxQueueUtilization(users []dto.AdminOpsConcurrencyUser) float64 {
	maxUtilization := 0.0
	for _, user := range users {
		if user.Queued <= 0 || user.QueueCapacity <= 0 {
			continue
		}
		utilization := float64(user.Queued) / float64(user.QueueCapacity)
		if utilization > maxUtilization {
			maxUtilization = utilization
		}
	}
	return maxUtilization
}

func limitAdminOpsConcurrencyUsers(users []dto.AdminOpsConcurrencyUser, limit int) []dto.AdminOpsConcurrencyUser {
	if len(users) == 0 {
		return []dto.AdminOpsConcurrencyUser{}
	}
	if limit <= 0 || limit >= len(users) {
		return users
	}
	return users[:limit]
}

func filterAdminOpsConcurrencyUsers(users []dto.AdminOpsConcurrencyUser, query AdminOpsConcurrencyQuery) []dto.AdminOpsConcurrencyUser {
	status := strings.TrimSpace(query.Status)
	search := strings.ToLower(strings.TrimSpace(query.Search))
	if query.MinActiveOrQueued <= 0 && query.PlanID <= 0 && status == "" && search == "" {
		return users
	}
	filtered := make([]dto.AdminOpsConcurrencyUser, 0, len(users))
	for _, user := range users {
		if query.MinActiveOrQueued > 0 && user.Active+user.Queued < query.MinActiveOrQueued {
			continue
		}
		if query.PlanID > 0 && user.PlanID != query.PlanID {
			continue
		}
		if status != "" && user.Status != status {
			continue
		}
		if search != "" && !adminOpsConcurrencyUserMatchesSearch(user, search) {
			continue
		}
		filtered = append(filtered, user)
	}
	return filtered
}

func adminOpsConcurrencyUserMatchesSearch(user dto.AdminOpsConcurrencyUser, search string) bool {
	if search == "" {
		return true
	}
	if strings.Contains(strings.ToLower(user.Username), search) || strings.Contains(strings.ToLower(user.PlanTitle), search) || strings.Contains(strings.ToLower(user.PlanCode), search) {
		return true
	}
	return strings.Contains(strconv.Itoa(user.UserID), search)
}

func buildAdminOpsRuntime(generatedAt int64) dto.AdminOpsRuntime {
	uptime := generatedAt - common.StartTime
	if uptime < 0 {
		uptime = 0
	}
	return dto.AdminOpsRuntime{
		Version:           common.Version,
		StartTime:         common.StartTime,
		UptimeSeconds:     uptime,
		NodeName:          common.NodeName,
		ActiveConnections: common.GetActiveConnections(),
		Goroutines:        runtime.NumGoroutine(),
	}
}

func buildAdminOpsSystem(status common.SystemStatus) dto.AdminOpsSystem {
	return dto.AdminOpsSystem{
		CPUUsage:    status.CPUUsage,
		MemoryUsage: status.MemoryUsage,
		DiskUsage:   status.DiskUsage,
	}
}

func buildAdminOpsDependencies(ctx context.Context) dto.AdminOpsDependencies {
	return dto.AdminOpsDependencies{
		Database: pingAdminOpsDatabaseDependency(),
		Redis:    pingAdminOpsRedisDependency(ctx),
	}
}

func pingAdminOpsDatabaseDependency() dto.AdminOpsDependency {
	start := time.Now()
	dep := dto.AdminOpsDependency{Enabled: true, Status: dto.AdminOpsDependencyStatusHealthy}
	if err := model.PingDB(); err != nil {
		dep.Status = dto.AdminOpsDependencyStatusCritical
		dep.Message = common.MaskSensitiveInfo(err.Error())
	}
	dep.LatencyMs = time.Since(start).Milliseconds()
	return dep
}

func pingAdminOpsRedisDependency(ctx context.Context) dto.AdminOpsDependency {
	dep := dto.AdminOpsDependency{Enabled: common.RedisEnabled, Status: dto.AdminOpsDependencyStatusDisabled}
	if !common.RedisEnabled {
		if common.SubscriptionConcurrencyRequireRedis && !common.SubscriptionConcurrencyFailOpen {
			dep.Status = dto.AdminOpsDependencyStatusCritical
			dep.Message = "redis_required_but_disabled"
		}
		return dep
	}
	dep.Status = dto.AdminOpsDependencyStatusHealthy
	if common.RDB == nil {
		dep.Status = dto.AdminOpsDependencyStatusCritical
		dep.Message = "redis_client_unavailable"
		return dep
	}
	start := time.Now()
	if err := common.RDB.Ping(ctx).Err(); err != nil {
		dep.Status = dto.AdminOpsDependencyStatusCritical
		dep.Message = common.MaskSensitiveInfo(err.Error())
	}
	dep.LatencyMs = time.Since(start).Milliseconds()
	return dep
}

func buildAdminOpsTraffic(windowSeconds int64, stats model.AdminOpsTrafficStats) dto.AdminOpsTraffic {
	traffic := dto.AdminOpsTraffic{
		WindowSeconds: windowSeconds,
		Requests:      stats.Requests,
		Errors:        stats.Errors,
	}
	if windowSeconds > 0 {
		traffic.RPM = float64(stats.Requests) * 60 / float64(windowSeconds)
		traffic.TPM = float64(stats.TotalTokens) * 60 / float64(windowSeconds)
	}
	if stats.Requests > 0 {
		traffic.ErrorRate = float64(stats.Errors) / float64(stats.Requests)
	}
	return traffic
}

func buildAdminOpsPerformance(summary perfmetrics.SummaryAllResult, top int) dto.AdminOpsPerformance {
	if top <= 0 || top > len(summary.Models) {
		top = len(summary.Models)
	}
	models := make([]dto.AdminOpsPerformanceModel, 0, top)
	for i := 0; i < top; i++ {
		modelSummary := summary.Models[i]
		models = append(models, dto.AdminOpsPerformanceModel{
			ModelName:    modelSummary.ModelName,
			AvgLatencyMs: modelSummary.AvgLatencyMs,
			AvgTtftMs:    modelSummary.AvgTtftMs,
			SuccessRate:  modelSummary.SuccessRate,
			AvgTPS:       modelSummary.AvgTps,
			RequestCount: modelSummary.RequestCount,
		})
	}
	return dto.AdminOpsPerformance{Models: models}
}

func buildAdminOpsRecentErrors(logs []*model.Log) []dto.AdminOpsRecentError {
	result := make([]dto.AdminOpsRecentError, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		result = append(result, dto.AdminOpsRecentError{
			ID:        log.Id,
			CreatedAt: log.CreatedAt,
			UserID:    log.UserId,
			Username:  log.Username,
			ModelName: log.ModelName,
			ChannelID: log.ChannelId,
			Content:   sanitizeAdminOpsRecentErrorContent(log.Content),
			RequestID: log.RequestId,
		})
	}
	return result
}

func buildAdminOpsSnapshotHealthReasons(dependencies dto.AdminOpsDependencies, system dto.AdminOpsSystem, counters dto.AdminOpsConcurrencyCounters, channels dto.AdminOpsChannels, traffic dto.AdminOpsTraffic) []adminOpsHealthReason {
	reasons := make([]adminOpsHealthReason, 0, 12)
	if dependencies.Database.Status == dto.AdminOpsDependencyStatusCritical {
		reasons = append(reasons, adminOpsHealthReason{Code: "database_unhealthy", Severity: adminOpsHealthSeverityCritical})
	}
	if dependencies.Redis.Status == dto.AdminOpsDependencyStatusCritical {
		reasons = append(reasons, adminOpsHealthReason{Code: "redis_unhealthy", Severity: adminOpsHealthSeverityCritical})
	}
	reasons = append(reasons, adminOpsSystemHealthReasons(system)...)
	reasons = append(reasons, adminOpsConcurrencyHealthReasons(counters)...)
	if channels.AutoDisabled > 0 {
		reasons = append(reasons, adminOpsHealthReason{Code: "channel_auto_disabled", Severity: adminOpsHealthSeverityDegraded})
	}
	if traffic.Requests > 0 && traffic.ErrorRate > 0.05 {
		reasons = append(reasons, adminOpsHealthReason{Code: "traffic_error_rate_high", Severity: adminOpsHealthSeverityDegraded})
	}
	return reasons
}

func adminOpsSystemHealthReasons(system dto.AdminOpsSystem) []adminOpsHealthReason {
	config := common.GetPerformanceMonitorConfig()
	if !config.Enabled {
		return nil
	}
	reasons := make([]adminOpsHealthReason, 0, 3)
	if config.CPUThreshold > 0 && system.CPUUsage >= float64(config.CPUThreshold) {
		reasons = append(reasons, adminOpsHealthReason{Code: "system_cpu_high", Severity: adminOpsHealthSeverityDegraded})
	}
	if config.MemoryThreshold > 0 && system.MemoryUsage >= float64(config.MemoryThreshold) {
		reasons = append(reasons, adminOpsHealthReason{Code: "system_memory_high", Severity: adminOpsHealthSeverityDegraded})
	}
	if config.DiskThreshold > 0 && system.DiskUsage >= float64(config.DiskThreshold) {
		reasons = append(reasons, adminOpsHealthReason{Code: "system_disk_high", Severity: adminOpsHealthSeverityDegraded})
	}
	return reasons
}

func adminOpsConcurrencyMode() string {
	if common.RedisEnabled {
		return dto.AdminOpsConcurrencyModeRedis
	}
	if common.SubscriptionConcurrencyRequireRedis {
		return dto.AdminOpsConcurrencyModeDisabled
	}
	return dto.AdminOpsConcurrencyModeMemory
}

func adminOpsConcurrencyTTLSeconds() int {
	if common.SubscriptionConcurrencyTTLSeconds > 0 {
		return common.SubscriptionConcurrencyTTLSeconds
	}
	return 600
}

func adminOpsConcurrencyDefaultQueueCapacity() int {
	if common.SubscriptionConcurrencyQueueCapacity > 0 {
		return common.SubscriptionConcurrencyQueueCapacity
	}
	return 1
}

func adminOpsConcurrencyCountersDTO(counters SubscriptionConcurrencyCounters) dto.AdminOpsConcurrencyCounters {
	return dto.AdminOpsConcurrencyCounters{
		AcquiredTotal:              counters.AcquiredTotal,
		QueuedTotal:                counters.QueuedTotal,
		QueueFullRejectionsTotal:   counters.QueueFullRejectionsTotal,
		UnavailableRejectionsTotal: counters.UnavailableRejectionsTotal,
		RedisErrorsTotal:           counters.RedisErrorsTotal,
	}
}

func adminOpsSubscriptionConcurrencyRows(ctx context.Context, mode string, now time.Time) ([]SubscriptionConcurrencyUserRuntime, error) {
	switch mode {
	case dto.AdminOpsConcurrencyModeRedis:
		evaler := subscriptionConcurrencyRedis
		if evaler == nil && common.RDB != nil {
			evaler = redisClientEvaler{client: common.RDB}
		}
		if evaler == nil {
			return nil, nil
		}
		return snapshotRedisSubscriptionConcurrency(ctx, evaler, SubscriptionConcurrencySnapshotQuery{Now: now})
	case dto.AdminOpsConcurrencyModeMemory:
		return subscriptionConcurrencyMemory.Snapshot(now), nil
	default:
		return nil, nil
	}
}

func buildAdminOpsConcurrencyUsers(rows []SubscriptionConcurrencyUserRuntime, includeUsername bool) ([]dto.AdminOpsConcurrencyUser, error) {
	if len(rows) == 0 {
		return []dto.AdminOpsConcurrencyUser{}, nil
	}
	userIDs := make([]int, 0, len(rows))
	for _, row := range rows {
		if row.UserID > 0 && (row.Active > 0 || row.Queued > 0) {
			userIDs = append(userIDs, row.UserID)
		}
	}
	limits, err := model.GetAdminOpsUserConcurrencyLimits(userIDs)
	if err != nil {
		return nil, err
	}
	users := make([]dto.AdminOpsConcurrencyUser, 0, len(rows))
	for _, row := range rows {
		if row.UserID <= 0 || (row.Active <= 0 && row.Queued <= 0) {
			continue
		}
		limit := limits[row.UserID]
		user := dto.AdminOpsConcurrencyUser{
			UserID:              row.UserID,
			Active:              row.Active,
			Limit:               limit.Limit,
			Queued:              row.Queued,
			QueueCapacity:       limit.QueueCapacity,
			PlanID:              limit.PlanID,
			PlanTitle:           limit.PlanTitle,
			PlanCode:            limit.PlanCode,
			AmountTotal:         limit.AmountTotal,
			AmountUsed:          limit.AmountUsed,
			TokenLimit:          limit.TokenLimit,
			TokenUsed:           limit.TokenUsed,
			OldestQueuedSeconds: row.OldestQueuedSeconds,
		}
		if includeUsername {
			user.Username = limit.Username
		}
		fillAdminOpsConcurrencyUserDerivedFields(&user)
		users = append(users, user)
	}
	sortAdminOpsConcurrencyUsers(users)
	return users, nil
}

func fillAdminOpsConcurrencyUserDerivedFields(user *dto.AdminOpsConcurrencyUser) {
	if user == nil {
		return
	}
	if user.Limit > 0 {
		user.Utilization = float64(user.Active) / float64(user.Limit)
	}
	if user.TokenLimit > 0 {
		user.UsageUsed = user.TokenUsed
		user.UsageTotal = user.TokenLimit
	} else if user.AmountTotal > 0 {
		user.UsageUsed = user.AmountUsed
		user.UsageTotal = user.AmountTotal
	}
	if user.UsageTotal > 0 {
		user.Usage = float64(user.UsageUsed) / float64(user.UsageTotal)
	}
	if user.QueueCapacity > 0 {
		user.QueueUtilization = float64(user.Queued) / float64(user.QueueCapacity)
	}
	switch {
	case user.QueueCapacity > 0 && user.Queued >= int64(user.QueueCapacity):
		user.Status = "queue_full_risk"
	case user.Queued > 0:
		user.Status = "queued"
	case user.Limit > 0 && user.Active >= int64(user.Limit):
		user.Status = "saturated"
	default:
		user.Status = "normal"
	}
}

func sortAdminOpsConcurrencyUsers(users []dto.AdminOpsConcurrencyUser) {
	sort.Slice(users, func(i, j int) bool {
		leftTotal := users[i].Active + users[i].Queued
		rightTotal := users[j].Active + users[j].Queued
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		if users[i].Queued != users[j].Queued {
			return users[i].Queued > users[j].Queued
		}
		if users[i].Active != users[j].Active {
			return users[i].Active > users[j].Active
		}
		return users[i].UserID < users[j].UserID
	})
}
