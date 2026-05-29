# 管理员运维监控面板实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 新增管理员只读运维监控页面 `/admin-ops` 与后端 `/api/admin-ops/*` 聚合接口，包含系统健康、依赖健康、订阅并发槽位、排队队列、实时流量、渠道健康、性能摘要和最近错误。

**架构：** 后端新增 DTO、service、model 查询和 controller，路由挂在 `/api/admin-ops` 并使用 `AdminAuth`。订阅并发模块在现有 `service/subscription_concurrency.go` 上增加只读 snapshot、Redis 用户索引和进程内计数器。前端新增 `features/admin-ops` 与 `_authenticated/admin-ops` 路由，使用 React Query 轮询并复用现有 Card、SectionPageLayout、ErrorState、i18n、侧边栏配置模式。

**技术栈：** Go 1.22+、Gin、GORM、Redis、React 19、TypeScript、TanStack Router、TanStack Query、Tailwind、Bun、i18next。

---

## 规格文件

完整规格：`docs/superpowers/specs/2026-05-29-admin-ops-monitoring-design.md`

所有实现必须满足该规格。若计划与规格冲突，以规格为准，并修正计划后再继续。

## 文件职责

### 后端

- 创建：`dto/admin_ops.go`
  - 定义 admin ops API 的响应 DTO。
  - 包含 health、runtime、system、dependencies、concurrency、traffic、channels、performance、recent errors。
- 创建：`model/admin_ops.go`
  - 放跨库 GORM 查询：流量汇总、渠道汇总、最近错误、用户订阅并发信息批量查询。
  - 不放业务健康分级。
- 创建：`service/admin_ops.go`
  - 聚合 snapshot 与 concurrency response。
  - 计算健康状态和健康分数。
  - 调用 `model/admin_ops.go`、`common.GetSystemStatus()`、`middleware.GetStats()`、`perfmetrics.QuerySummaryAll()`、Redis ping。
  - 映射 `perfmetrics.ModelSummary.AvgTtftMs`，不得用 0 伪造 TTFT。
- 修改：`service/subscription_concurrency.go`
  - 增加并发统计计数器。
  - 增加内存 limiter snapshot。
  - 增加 Redis 用户索引维护和 Redis snapshot 查询，summary 必须全量计算，`limit` 只裁剪用户明细。
  - Redis queued 计数只在首次入队时增加，不能在轮询已排队 request 时重复增加。
  - 保持请求热路径失败不受监控索引影响。
- 修改：`model/perf_metric.go`
  - `PerfMetricSummary` 和 `GetPerfMetricsSummaryAll` 汇总 DB 中的 `ttft_sum_ms` / `ttft_count`。
- 修改：`pkg/perf_metrics/types.go`
  - 给 `ModelSummary` 增加 `AvgTtftMs`。
- 修改：`pkg/perf_metrics/metrics.go`
  - 在 `QuerySummaryAll` 合并 DB、热桶和 Redis 活跃桶的 TTFT 汇总并输出模型级平均 TTFT；Redis 活跃桶读取现有 `ttft` / `ttft_n` 字段。
- 测试：`model/perf_metric_test.go`、`pkg/perf_metrics/*_test.go` 或现有同包测试文件
  - 验证 DB summary 和 `QuerySummaryAll` 能透传非零平均 TTFT，包括 Redis 活跃桶 fake reader 场景。
- 创建：`controller/admin_ops.go`
  - 解析查询参数。
  - 暴露 `GetAdminOpsSnapshot` 和 `GetAdminOpsConcurrency`。
- 修改：`router/api-router.go`
  - 新增 `/api/admin-ops` 路由组，使用 `middleware.AdminAuth()`。
- 测试：`service/subscription_concurrency_test.go`
  - 添加并发 snapshot 与计数器测试。
- 测试：`service/admin_ops_test.go`
  - 添加健康分级和 summary 纯逻辑测试。
- 测试：`controller/admin_ops_test.go`
  - 添加参数归一化与响应结构测试。

### 前端

- 创建：`web/default/src/routes/_authenticated/admin-ops/index.tsx`
  - 管理员路由守卫。
  - 挂载 `AdminOpsPage`。
- 创建：`web/default/src/features/admin-ops/api.ts`
  - 封装 `/api/admin-ops/snapshot` 与 `/api/admin-ops/concurrency`。
- 创建：`web/default/src/features/admin-ops/types.ts`
  - TypeScript 类型与后端 DTO 对齐。
- 创建：`web/default/src/features/admin-ops/lib/health.ts`
  - 状态 tone 和文案 key 映射纯函数。
- 创建：`web/default/src/features/admin-ops/lib/format.ts`
  - 百分比、时长、计数、速率格式化纯函数。
- 创建：`web/default/src/features/admin-ops/index.tsx`
  - 页面容器、React Query 轮询、手动刷新、可见性控制。
- 创建：`web/default/src/features/admin-ops/components/*.tsx`
  - Header 与各指标卡片。
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
  - Admin 导航新增运维监控入口。
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
  - 新增 admin ops 默认模块与 URL 映射。
- 修改：`web/default/src/features/system-settings/maintenance/config.ts`
  - 后台维护配置默认模块加入 `ops`。
- 修改：`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx`
  - 系统设置侧边栏模块管理页面加入运维监控元数据。
- 修改：`web/default/src/features/profile/components/sidebar-modules-card.tsx`
  - 用户侧侧边栏模块配置页面加入运维监控元数据。
- 修改：`web/default/src/routeTree.gen.ts`
  - 由 TanStack Router 生成器更新，禁止手写。
- 修改：`web/default/src/i18n/static-keys.ts`
  - 添加 `adminOps.*` key。
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
  - 补齐翻译。
- 测试：`web/default/src/features/admin-ops/lib/health.test.ts`
  - 测状态映射。
- 测试：`web/default/src/features/admin-ops/lib/format.test.ts`
  - 测格式化函数。
- 测试：`web/default/src/features/admin-ops/lib/i18n-keys.test.ts`
  - 测 6 个 locale 的 admin ops key 和 routeTree 注册。
- 测试：`web/default/src/hooks/use-sidebar-config.test.ts`
  - 测 admin ops 默认可见、可配置隐藏和 URL 映射。

---

## 任务 1：后端 DTO、模型查询与健康纯逻辑

**文件：**
- 创建：`dto/admin_ops.go`
- 创建：`model/admin_ops.go`
- 创建：`service/admin_ops.go`
- 创建：`service/admin_ops_test.go`

- [ ] **步骤 1：编写失败的健康分级测试**

在 `service/admin_ops_test.go` 中新增测试。测试使用真实纯函数，不依赖 DB。

```go
package service

import (
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
)

func TestAdminOpsHealthFromReasons(t *testing.T) {
	healthy := buildAdminOpsHealth(nil)
	assert.Equal(t, dto.AdminOpsHealthStatusHealthy, healthy.Status)
	assert.Equal(t, 100, healthy.Score)
	assert.Empty(t, healthy.Reasons)

	degraded := buildAdminOpsHealth([]adminOpsHealthReason{
		{Code: "concurrency_queue_not_empty", Severity: adminOpsHealthSeverityDegraded},
		{Code: "channel_auto_disabled", Severity: adminOpsHealthSeverityDegraded},
	})
	assert.Equal(t, dto.AdminOpsHealthStatusDegraded, degraded.Status)
	assert.Equal(t, 80, degraded.Score)
	assert.Equal(t, []string{"concurrency_queue_not_empty", "channel_auto_disabled"}, degraded.Reasons)

	critical := buildAdminOpsHealth([]adminOpsHealthReason{
		{Code: "database_unhealthy", Severity: adminOpsHealthSeverityCritical},
		{Code: "concurrency_queue_not_empty", Severity: adminOpsHealthSeverityDegraded},
	})
	assert.Equal(t, dto.AdminOpsHealthStatusCritical, critical.Status)
	assert.Equal(t, 60, critical.Score)
	assert.Equal(t, []string{"database_unhealthy", "concurrency_queue_not_empty"}, critical.Reasons)
}

func TestAdminOpsConcurrencySummaryHealthReasons(t *testing.T) {
	summary := dto.AdminOpsConcurrencySummary{
		TotalQueued:    2,
		SaturatedUsers: 1,
		QueuePressure:  0.75,
	}
	reasons := adminOpsConcurrencyHealthReasons(summary, dto.AdminOpsConcurrencyCounters{})
	assert.Equal(t, []adminOpsHealthReason{
		{Code: "concurrency_queue_not_empty", Severity: adminOpsHealthSeverityDegraded},
		{Code: "concurrency_saturated_users", Severity: adminOpsHealthSeverityDegraded},
		{Code: "concurrency_queue_pressure_high", Severity: adminOpsHealthSeverityDegraded},
	}, reasons)

	criticalReasons := adminOpsConcurrencyHealthReasons(dto.AdminOpsConcurrencySummary{}, dto.AdminOpsConcurrencyCounters{
		QueueFullRejectionsTotal:   1,
		UnavailableRejectionsTotal: 1,
	})
	assert.Equal(t, []adminOpsHealthReason{
		{Code: "concurrency_queue_full_rejections", Severity: adminOpsHealthSeverityCritical},
		{Code: "concurrency_unavailable_rejections", Severity: adminOpsHealthSeverityCritical},
	}, criticalReasons)
}

func TestAdminOpsRecentErrorsAreMasked(t *testing.T) {
	raw := `Authorization: Bearer sk-secret-token prompt: "patient data should not leak" image: data:image/png;base64,aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`
	sanitized := sanitizeAdminOpsRecentErrorContent(raw)
	assert.NotContains(t, sanitized, "Authorization")
	assert.NotContains(t, sanitized, "Bearer")
	assert.NotContains(t, sanitized, "sk-secret-token")
	assert.NotContains(t, sanitized, "patient data should not leak")
	assert.NotContains(t, sanitized, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.LessOrEqual(t, utf8.RuneCountInString(sanitized), 300)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
go test ./service -run 'TestAdminOpsHealthFromReasons|TestAdminOpsConcurrencySummaryHealthReasons|TestAdminOpsRecentErrorsAreMasked' -count=1
```

预期：FAIL，原因是 `dto.AdminOpsHealthStatusHealthy`、`buildAdminOpsHealth`、`adminOpsConcurrencyHealthReasons`、`sanitizeAdminOpsRecentErrorContent` 等未定义。

- [ ] **步骤 3：创建 DTO**

创建 `dto/admin_ops.go`。字段必须与规格一致，保持 JSON tag snake_case。

```go
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
	Mode        string                       `json:"mode"`
	GeneratedAt int64                        `json:"generated_at"`
	Enabled     bool                         `json:"enabled"`
	Summary     AdminOpsConcurrencySummary   `json:"summary"`
	Config      AdminOpsConcurrencyConfig    `json:"config"`
	Counters    AdminOpsConcurrencyCounters  `json:"counters"`
	Users       []AdminOpsConcurrencyUser    `json:"users"`
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
	// Content is a sanitized, short error summary. Never expose raw log content.
	Content   string `json:"content"`
	RequestID string `json:"request_id"`
}

```

`recent_errors.content` 必须在 service 聚合时调用专用 `sanitizeAdminOpsRecentErrorContent`，不得只依赖 `common.MaskSensitiveInfo`。该函数先替换或移除 `Authorization` header、`Bearer` token、`sk-`/`ak-` 风格 key、疑似长 base64/图片片段、`prompt`/`messages`/请求体大段文本，再调用 `common.MaskSensitiveInfo`，最后按 rune 截断为最多 300 个字符；不得返回原始请求体、响应体、prompt、Authorization、API key 或 base64 片段。新增 `TestAdminOpsRecentErrorsAreMasked`，断言 `sk-...`、`Authorization: Bearer ...`、`prompt/messages` 原文、长 base64 片段不会原样出现在 `recent_errors.content`。

如果 gofmt 或编译发现字段对齐问题，保持字段名不变后修正格式。

同时在 `pkg/perf_metrics/types.go` 的 `ModelSummary` 中增加同名 `AvgTtftMs int64 \`json:"avg_ttft_ms"\`` 字段，并在 `pkg/perf_metrics/metrics.go` 的 `QuerySummaryAll` 中用 `avg(total.ttftSumMs, total.ttftCount)` 填充；否则 admin ops 不得展示该字段。

- [ ] **步骤 4：实现健康纯逻辑最小代码**

在 `service/admin_ops.go` 中添加健康 reason 类型、`buildAdminOpsHealth`、`adminOpsConcurrencyHealthReasons` 和 `sanitizeAdminOpsRecentErrorContent`。

```go
package service

import "github.com/QuantumNous/new-api/dto"

type adminOpsHealthSeverity int

const (
	adminOpsHealthSeverityDegraded adminOpsHealthSeverity = iota + 1
	adminOpsHealthSeverityCritical
)

type adminOpsHealthReason struct {
	Code     string
	Severity adminOpsHealthSeverity
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

func adminOpsConcurrencyHealthReasons(summary dto.AdminOpsConcurrencySummary, counters dto.AdminOpsConcurrencyCounters) []adminOpsHealthReason {
	reasons := make([]adminOpsHealthReason, 0, 5)
	if counters.QueueFullRejectionsTotal > 0 {
		reasons = append(reasons, adminOpsHealthReason{Code: "concurrency_queue_full_rejections", Severity: adminOpsHealthSeverityCritical})
	}
	if counters.UnavailableRejectionsTotal > 0 {
		reasons = append(reasons, adminOpsHealthReason{Code: "concurrency_unavailable_rejections", Severity: adminOpsHealthSeverityCritical})
	}
	if summary.TotalQueued > 0 {
		reasons = append(reasons, adminOpsHealthReason{Code: "concurrency_queue_not_empty", Severity: adminOpsHealthSeverityDegraded})
	}
	if summary.SaturatedUsers > 0 {
		reasons = append(reasons, adminOpsHealthReason{Code: "concurrency_saturated_users", Severity: adminOpsHealthSeverityDegraded})
	}
	if summary.QueuePressure >= 0.5 {
		reasons = append(reasons, adminOpsHealthReason{Code: "concurrency_queue_pressure_high", Severity: adminOpsHealthSeverityDegraded})
	}
	return reasons
}

func sanitizeAdminOpsRecentErrorContent(content string) string {
	// 先用正则替换 Authorization/Bearer/sk-/ak-/长 base64/prompt/messages 等敏感片段；
	// 再调用 common.MaskSensitiveInfo；最后按 rune 截断 300。实现时不要保留原始敏感值。
	return truncateRunes(common.MaskSensitiveInfo(maskAdminOpsSensitiveErrorFragments(content)), 300)
}
```
- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
go test ./service -run 'TestAdminOpsHealthFromReasons|TestAdminOpsConcurrencySummaryHealthReasons|TestAdminOpsRecentErrorsAreMasked' -count=1
```

预期：PASS。

- [ ] **步骤 6：编写模型查询失败测试**

在 `service/admin_ops_test.go` 中追加 summary 纯计算测试，避免先依赖 DB。

```go
func TestBuildAdminOpsConcurrencySummary(t *testing.T) {
	users := []dto.AdminOpsConcurrencyUser{
		{UserID: 1, Active: 2, Limit: 2, Queued: 1, QueueCapacity: 4},
		{UserID: 2, Active: 1, Limit: 4, Queued: 0, QueueCapacity: 4},
		{UserID: 3, Active: 0, Limit: 1, Queued: 3, QueueCapacity: 3},
	}
	summary := buildAdminOpsConcurrencySummary(users)
	assert.EqualValues(t, 3, summary.TotalActive)
	assert.EqualValues(t, 4, summary.TotalQueued)
	assert.EqualValues(t, 2, summary.ActiveUsers)
	assert.EqualValues(t, 2, summary.QueuedUsers)
	assert.EqualValues(t, 1, summary.SaturatedUsers)
	assert.InDelta(t, 1.0, summary.QueuePressure, 0.0001)

	reasons := adminOpsConcurrencyHealthReasons(summary, dto.AdminOpsConcurrencyCounters{})
	assert.Contains(t, reasons, adminOpsHealthReason{Code: "concurrency_queue_pressure_high", Severity: adminOpsHealthSeverityDegraded})
}

func TestBuildAdminOpsConcurrencySummaryUsesAllUsersBeforeDetailLimit(t *testing.T) {
	users := []dto.AdminOpsConcurrencyUser{
		{UserID: 1, Active: 3, Limit: 3, Queued: 0, QueueCapacity: 5},
		{UserID: 2, Active: 0, Limit: 3, Queued: 5, QueueCapacity: 5},
		{UserID: 3, Active: 1, Limit: 3, Queued: 0, QueueCapacity: 5},
	}
	// 模拟 UI 明细只展示第一个热点用户；summary 必须仍基于完整 runtime 列表计算。
	detail := limitAdminOpsConcurrencyUsers(users, 1)
	assert.Len(t, detail, 1)
	summary := buildAdminOpsConcurrencySummary(users)
	assert.EqualValues(t, 4, summary.TotalActive)
	assert.EqualValues(t, 5, summary.TotalQueued)
	assert.EqualValues(t, 2, summary.ActiveUsers)
	assert.EqualValues(t, 1, summary.QueuedUsers)
	assert.EqualValues(t, 1, summary.SaturatedUsers)
	assert.InDelta(t, 1.0, summary.QueuePressure, 0.0001)
}
```

- [ ] **步骤 7：运行测试验证失败**

运行：

```bash
go test ./service -run 'TestBuildAdminOpsConcurrencySummary' -count=1
```

预期：FAIL，`buildAdminOpsConcurrencySummary` 和 `limitAdminOpsConcurrencyUsers` 未定义。

- [ ] **步骤 8：实现 summary 纯函数**

在 `service/admin_ops.go` 中添加：

```go
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
	if limit <= 0 || limit >= len(users) {
		return users
	}
	return users[:limit]
}
```

- [ ] **步骤 9：运行 service 测试通过**

运行：

```bash
go test ./service -run 'TestAdminOpsHealthFromReasons|TestAdminOpsConcurrencySummaryHealthReasons|TestBuildAdminOpsConcurrencySummary' -count=1
```

预期：PASS。

---

## 任务 2：订阅并发 snapshot、Redis 索引与计数器

**文件：**
- 修改：`service/subscription_concurrency.go`
- 修改：`service/subscription_concurrency_test.go`

- [ ] **步骤 1：编写失败的内存 snapshot 测试**

在 `service/subscription_concurrency_test.go` 中新增测试。使用真实 memory limiter，不使用 mock。

```go
func TestMemorySubscriptionConcurrencySnapshotReportsActiveAndQueued(t *testing.T) {
	limiter := newMemorySubscriptionConcurrencyLimiter()
	ctx := context.Background()

	lease, err := limiter.Acquire(ctx, 1001, "req-active", 1, 2)
	require.NoError(t, err)
	defer lease.Release(ctx)

	queuedCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	queuedStarted := make(chan struct{})
	go func() {
		close(queuedStarted)
		_, _ = limiter.Acquire(queuedCtx, 1001, "req-queued", 1, 2)
	}()
	<-queuedStarted
	require.Eventually(t, func() bool {
		snapshot := limiter.Snapshot(time.Now())
		return len(snapshot) == 1 && snapshot[0].Active == 1 && snapshot[0].Queued == 1
	}, time.Second, 10*time.Millisecond)

	snapshot := limiter.Snapshot(time.Now())
	require.Len(t, snapshot, 1)
	assert.Equal(t, 1001, snapshot[0].UserID)
	assert.EqualValues(t, 1, snapshot[0].Active)
	assert.EqualValues(t, 1, snapshot[0].Queued)
	assert.GreaterOrEqual(t, snapshot[0].OldestQueuedSeconds, int64(0))
}
```

需要在 import 中加入 `context`、`time`、`github.com/stretchr/testify/require`、`github.com/stretchr/testify/assert`，如果已有则复用。

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
go test ./service -run TestMemorySubscriptionConcurrencySnapshotReportsActiveAndQueued -count=1
```

预期：FAIL，`Snapshot` 未定义，或 waiter 缺少 queued timestamp。

- [ ] **步骤 3：实现内存 snapshot 最小代码**

修改 `service/subscription_concurrency.go`：

1. 给 `memorySubscriptionConcurrencyWaiter` 增加 `queuedAt int64` 字段。
2. 创建 waiter 时设置 `queuedAt: time.Now().Unix()`。
3. 新增 snapshot 类型和方法。

```go
type SubscriptionConcurrencyUserRuntime struct {
	UserID              int
	Active              int64
	Queued              int64
	OldestQueuedSeconds int64
}

func (m *memorySubscriptionConcurrencyLimiter) Snapshot(now time.Time) []SubscriptionConcurrencyUserRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()

	userIDs := make(map[int]struct{}, len(m.requests)+len(m.waiting))
	for userID := range m.requests {
		userIDs[userID] = struct{}{}
	}
	for userID := range m.waiting {
		userIDs[userID] = struct{}{}
	}

	rows := make([]SubscriptionConcurrencyUserRuntime, 0, len(userIDs))
	nowUnix := now.Unix()
	for userID := range userIDs {
		active := int64(len(m.requests[userID]))
		waiting := m.waiting[userID]
		queued := int64(len(waiting))
		oldest := int64(0)
		if len(waiting) > 0 && waiting[0].queuedAt > 0 {
			oldest = nowUnix - waiting[0].queuedAt
			if oldest < 0 {
				oldest = 0
			}
		}
		if active == 0 && queued == 0 {
			continue
		}
		rows = append(rows, SubscriptionConcurrencyUserRuntime{UserID: userID, Active: active, Queued: queued, OldestQueuedSeconds: oldest})
	}
	return rows
}
```

- [ ] **步骤 4：运行内存 snapshot 测试通过**

运行：

```bash
go test ./service -run TestMemorySubscriptionConcurrencySnapshotReportsActiveAndQueued -count=1
```

预期：PASS。

- [ ] **步骤 5：编写失败的计数器测试**

在 `service/subscription_concurrency_test.go` 中新增。测试必须通过真实 wrapper 触发队列满拒绝；不要在测试中手动调用 `recordSubscriptionConcurrencyQueueFullRejection()`。

```go
func TestSubscriptionConcurrencyCountersTrackQueueRejection(t *testing.T) {
	resetSubscriptionConcurrencyStatsForTest()
	oldRedisEnabled := common.RedisEnabled
	oldRequireRedis := common.SubscriptionConcurrencyRequireRedis
	oldMemory := subscriptionConcurrencyMemory
	common.RedisEnabled = false
	common.SubscriptionConcurrencyRequireRedis = false
	subscriptionConcurrencyMemory = newMemorySubscriptionConcurrencyLimiter()
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.SubscriptionConcurrencyRequireRedis = oldRequireRedis
		subscriptionConcurrencyMemory = oldMemory
	})

	ctx := context.Background()
	lease, err := AcquireUserConcurrencyWithQueueCapacity(ctx, 1002, "req-active", 1, 1)
	require.NoError(t, err)
	defer lease.Release(ctx)

	queuedCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _, _ = AcquireUserConcurrencyWithQueueCapacity(queuedCtx, 1002, "req-queued", 1, 1) }()
	require.Eventually(t, func() bool {
		snapshot := subscriptionConcurrencyMemory.Snapshot(time.Now())
		return len(snapshot) == 1 && snapshot[0].Queued == 1
	}, time.Second, 10*time.Millisecond)

	_, err = AcquireUserConcurrencyWithQueueCapacity(ctx, 1002, "req-rejected", 1, 1)
	assert.ErrorIs(t, err, ErrSubscriptionConcurrencyExceeded)

	counters := SubscriptionConcurrencyCountersSnapshot()
	assert.EqualValues(t, 1, counters.AcquiredTotal)
	assert.EqualValues(t, 1, counters.QueuedTotal)
	assert.EqualValues(t, 1, counters.QueueFullRejectionsTotal)
}
```

- [ ] **步骤 6：运行测试验证失败**

运行：

```bash
go test ./service -run TestSubscriptionConcurrencyCountersTrackQueueRejection -count=1
```

预期：FAIL，计数器函数未定义或真实路径尚未自动计数。

- [ ] **步骤 7：实现计数器并接入真实路径**

在 `service/subscription_concurrency.go` 中新增：

```go
type SubscriptionConcurrencyCounters struct {
	AcquiredTotal              int64
	QueuedTotal                int64
	QueueFullRejectionsTotal   int64
	UnavailableRejectionsTotal int64
	RedisErrorsTotal           int64
}

var subscriptionConcurrencyStats struct {
	acquired              atomic.Int64
	queued                atomic.Int64
	queueFullRejections   atomic.Int64
	unavailableRejections atomic.Int64
	redisErrors           atomic.Int64
}

func SubscriptionConcurrencyCountersSnapshot() SubscriptionConcurrencyCounters {
	return SubscriptionConcurrencyCounters{
		AcquiredTotal:              subscriptionConcurrencyStats.acquired.Load(),
		QueuedTotal:                subscriptionConcurrencyStats.queued.Load(),
		QueueFullRejectionsTotal:   subscriptionConcurrencyStats.queueFullRejections.Load(),
		UnavailableRejectionsTotal: subscriptionConcurrencyStats.unavailableRejections.Load(),
		RedisErrorsTotal:           subscriptionConcurrencyStats.redisErrors.Load(),
	}
}

func recordSubscriptionConcurrencyAcquired() { subscriptionConcurrencyStats.acquired.Add(1) }
func recordSubscriptionConcurrencyQueued() { subscriptionConcurrencyStats.queued.Add(1) }
func recordSubscriptionConcurrencyQueueFullRejection() { subscriptionConcurrencyStats.queueFullRejections.Add(1) }
func recordSubscriptionConcurrencyUnavailableRejection() { subscriptionConcurrencyStats.unavailableRejections.Add(1) }
func recordSubscriptionConcurrencyRedisError() { subscriptionConcurrencyStats.redisErrors.Add(1) }

func resetSubscriptionConcurrencyStatsForTest() {
	subscriptionConcurrencyStats.acquired.Store(0)
	subscriptionConcurrencyStats.queued.Store(0)
	subscriptionConcurrencyStats.queueFullRejections.Store(0)
	subscriptionConcurrencyStats.unavailableRejections.Store(0)
	subscriptionConcurrencyStats.redisErrors.Store(0)
}
```

真实路径计数位置：

- `AcquireUserConcurrencyWithQueueCapacity` 内存成功后记录 acquired。
- 内存进入等待队列时记录 queued；等待者被提升为 active 时再记录 acquired。
- 内存返回 `ErrSubscriptionConcurrencyExceeded` 前记录 queue full rejection。
- Redis allowed 记录 acquired。
- Redis 首次把 request_id 加入 queue 时记录 queued；已在 queue 中的轮询返回不重复记录 queued。
- Redis rejected 记录 queue full rejection。
- Redis required 但 disabled 且 fail-closed 时记录 unavailable rejection。
- `handleSubscriptionConcurrencyRedisError` fail-closed 记录 redis error + unavailable rejection；fail-open 只记录 redis error。

- [ ] **步骤 8：运行计数器测试通过**

运行：

```bash
go test ./service -run TestSubscriptionConcurrencyCountersTrackQueueRejection -count=1
```

预期：PASS。

- [ ] **步骤 9：编写失败的 Redis snapshot 与 queued 去重测试**

新增 fake Redis 测试，必须覆盖：索引清理、全部候选用户参与 summary、`limit` 只影响明细、最老排队秒数、Redis 已排队轮询不重复计数。

```go
func TestSubscriptionConcurrencyUserIndexKey(t *testing.T) {
	assert.Equal(t, "subscription:concurrency:users", subscriptionConcurrencyUserIndexKey())
	assert.Equal(t, "subscription:concurrency:user:42", subscriptionConcurrencyKey(42))
	assert.Equal(t, "subscription:concurrency:user:42:queue", subscriptionConcurrencyQueueKey(42))
}

func TestRedisSubscriptionConcurrencySnapshotSummarizesAllUsersBeforeLimit(t *testing.T) {
	now := time.Unix(1780000000, 0)
	fake := newFakeSubscriptionConcurrencyRedisSnapshot(map[int]SubscriptionConcurrencyUserRuntime{
		10: {UserID: 10, Active: 3, Queued: 0, OldestQueuedSeconds: 0},
		20: {UserID: 20, Active: 0, Queued: 4, OldestQueuedSeconds: 12},
		30: {UserID: 30, Active: 1, Queued: 0, OldestQueuedSeconds: 0},
	})
	rows, err := snapshotRedisSubscriptionConcurrency(context.Background(), fake, SubscriptionConcurrencySnapshotQuery{Now: now, Limit: 1, MinActiveOrQueued: 1})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, 20, rows[0].UserID)
	assert.EqualValues(t, 4, rows[0].Queued)
	assert.EqualValues(t, 12, rows[0].OldestQueuedSeconds)
}

func TestRedisSubscriptionConcurrencyQueuedCounterCountsOnlyFirstEnqueue(t *testing.T) {
	resetSubscriptionConcurrencyStatsForTest()
	fake := &fakeRedisAcquireEvaler{results: []interface{}{int64(2), int64(3)}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	_, _ = acquireRedisUserConcurrencyWithEvaler(ctx, fake, 2001, "req-queued", 1, 1)
	assert.EqualValues(t, 1, SubscriptionConcurrencyCountersSnapshot().QueuedTotal)
}
```

如果 fake 类型在测试内实现更简单，可以只实现 `Eval` 并返回预设 `redisResult`；不要引入真实 Redis 依赖。

- [ ] **步骤 10：实现 Redis 用户索引与 Redis snapshot 抽象**

在 `service/subscription_concurrency.go` 中新增：

```go
type SubscriptionConcurrencySnapshotQuery struct {
	Now               time.Time
	Limit             int
	MinActiveOrQueued int64
}

func subscriptionConcurrencyUserIndexKey() string {
	return "subscription:concurrency:users"
}

func snapshotRedisSubscriptionConcurrency(ctx context.Context, evaler redisEvaler, query SubscriptionConcurrencySnapshotQuery) ([]SubscriptionConcurrencyUserRuntime, error)
```

`snapshotRedisSubscriptionConcurrency` 使用单个 Lua `Eval` 返回未按明细阈值过滤的扁平数组：`user_id, active, queued, oldest_queued_score` 重复项。Lua 内部必须：

1. `ZREMRANGEBYSCORE` 清理索引；
2. `ZRANGE` 遍历索引中全部有效用户；
3. 对每个用户执行 active/queue 的过期清理、`ZCARD`、最老队列 `ZRANGE 0 0 WITHSCORES`；
4. 只丢弃 active 和 queued 都为 0 的用户，不能应用 `MinActiveOrQueued`；
5. Go 侧按 `active+queued`、`queued`、`active`、`user_id` 排序；service 层先对完整 rows 批量补充 limit / queue_capacity，计算 summary 和 health，再应用 `MinActiveOrQueued` 与 `Limit` 构造 `Users` 明细。

新增 `recordRedisSubscriptionConcurrencyUser(ctx, evaler, userId, ttl, now)`，在 Redis allowed/首次 queued 路径 best-effort 写 `subscription:concurrency:users`。该函数错误只记录 `common.SysLog`，不得影响 acquire 结果。

同时调整 `subscriptionConcurrencyAcquireScript` 返回码：`0=rejected`、`1=allowed`、`2=queued_new`、`3=queued_existing`。`redisAcquireState` 要能区分 `queued_new` 和 `queued_existing`，只有 `queued_new` 增加 `queued_total` 和写用户索引。为便于测试，可把 Redis acquire 主体抽成 `acquireRedisUserConcurrencyWithEvaler(ctx, evaler, userId, requestId, limit, queueCapacity)`。

- [ ] **步骤 11：运行订阅并发相关测试**

运行：

```bash
go test ./service -run 'Test.*SubscriptionConcurrency|TestMemorySubscriptionConcurrencySnapshotReportsActiveAndQueued' -count=1
```

预期：PASS。

---

## 任务 3：后端 Admin Ops 聚合接口与路由

**文件：**
- 创建：`controller/admin_ops.go`
- 修改：`service/admin_ops.go`
- 创建：`model/admin_ops.go`
- 修改：`router/api-router.go`
- 创建：`controller/admin_ops_test.go`
- 创建或修改：`model/admin_ops_test.go`
- 修改：`pkg/perf_metrics/types.go`
- 修改：`pkg/perf_metrics/metrics.go`
- 创建或修改：`pkg/perf_metrics/*_test.go`

- [ ] **步骤 1：编写失败的参数归一化测试**

在 `controller/admin_ops_test.go` 中创建：

```go
package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestParseAdminOpsSnapshotQueryNormalizesBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin-ops/snapshot?window_seconds=999&top=999", nil)

	query := parseAdminOpsSnapshotQuery(ctx)
	assert.EqualValues(t, 300, query.WindowSeconds)
	assert.Equal(t, 20, query.Top)
}

func TestParseAdminOpsConcurrencyQueryNormalizesBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin-ops/concurrency?limit=999&include_users=false&min_active_or_queued=0", nil)

	query := parseAdminOpsConcurrencyQuery(ctx)
	assert.Equal(t, 100, query.Limit)
	assert.False(t, query.IncludeUsers)
	assert.EqualValues(t, 0, query.MinActiveOrQueued)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
go test ./controller -run 'TestParseAdminOps.*Query' -count=1
```

预期：FAIL，解析函数未定义。

- [ ] **步骤 3：实现 controller 查询解析**

创建 `controller/admin_ops.go`：

```go
package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type adminOpsSnapshotQuery struct {
	WindowSeconds int64
	Top           int
}

type adminOpsConcurrencyQuery struct {
	Limit             int
	IncludeUsers      bool
	MinActiveOrQueued int64
}

func parseAdminOpsSnapshotQuery(c *gin.Context) adminOpsSnapshotQuery {
	query := adminOpsSnapshotQuery{WindowSeconds: 300, Top: 5}
	if raw := strings.TrimSpace(c.Query("window_seconds")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			switch parsed {
			case 60, 300, 900, 3600:
				query.WindowSeconds = parsed
			}
		}
	}
	if raw := strings.TrimSpace(c.Query("top")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			if parsed < 1 {
				parsed = 1
			}
			if parsed > 20 {
				parsed = 20
			}
			query.Top = parsed
		}
	}
	return query
}

func parseAdminOpsConcurrencyQuery(c *gin.Context) adminOpsConcurrencyQuery {
	query := adminOpsConcurrencyQuery{Limit: 20, IncludeUsers: true, MinActiveOrQueued: 1}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			if parsed < 1 {
				parsed = 1
			}
			if parsed > 100 {
				parsed = 100
			}
			query.Limit = parsed
		}
	}
	if raw := strings.TrimSpace(c.Query("include_users")); raw != "" {
		query.IncludeUsers = raw != "false" && raw != "0"
	}
	if raw := strings.TrimSpace(c.Query("min_active_or_queued")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed >= 0 {
			query.MinActiveOrQueued = parsed
		}
	}
	return query
}

func GetAdminOpsSnapshot(c *gin.Context) {
	query := parseAdminOpsSnapshotQuery(c)
	data, err := service.GetAdminOpsSnapshot(c.Request.Context(), service.AdminOpsSnapshotQuery{WindowSeconds: query.WindowSeconds, Top: query.Top})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

func GetAdminOpsConcurrency(c *gin.Context) {
	query := parseAdminOpsConcurrencyQuery(c)
	data, err := service.GetAdminOpsConcurrency(c.Request.Context(), service.AdminOpsConcurrencyQuery{Limit: query.Limit, IncludeUsers: query.IncludeUsers, MinActiveOrQueued: query.MinActiveOrQueued})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}
```

- [ ] **步骤 4：运行 controller 解析测试通过**

运行：

```bash
go test ./controller -run 'TestParseAdminOps.*Query' -count=1
```

预期：PASS 或因 service 类型未定义失败。若 service 类型未定义，先在 `service/admin_ops.go` 增加空类型：

```go
type AdminOpsSnapshotQuery struct { WindowSeconds int64; Top int }
type AdminOpsConcurrencyQuery struct { Limit int; IncludeUsers bool; MinActiveOrQueued int64 }
```

- [ ] **步骤 5：实现 model 查询，并测试并发上限口径**

创建 `model/admin_ops.go`。必须使用 GORM，保持跨库。

需要提供函数：

```go
type AdminOpsTrafficStats struct {
	Requests    int64
	Errors      int64
	TotalTokens int64
}

func GetAdminOpsTrafficStats(startTimestamp int64, endTimestamp int64) (AdminOpsTrafficStats, error)

type AdminOpsChannelStats struct {
	Total          int64
	Enabled        int64
	ManualDisabled int64
	AutoDisabled   int64
	SlowCount      int64
	StaleTestCount int64
}

func GetAdminOpsChannelStats(now int64, slowThresholdMs int, staleAfterSeconds int64) (AdminOpsChannelStats, error)

func GetAdminOpsRecentErrors(startTimestamp int64, limit int) ([]*Log, error)

type AdminOpsUserConcurrencyLimit struct {
	UserID        int
	Username      string
	Limit         int
	QueueCapacity int
}

func GetAdminOpsUserConcurrencyLimits(userIDs []int) (map[int]AdminOpsUserConcurrencyLimit, error)
```

新增模型查询测试，确保展示口径与运行时一致：

```go
func TestGetAdminOpsUserConcurrencyLimitsPrefersPlanValues(t *testing.T) {
	db := setupAdminOpsModelTestDB(t)
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })
	now := GetDBTimestamp()
	code := "admin-ops-plan"
	require.NoError(t, DB.Create(&User{Id: 7101, Username: "ops-user", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7102, Title: "Ops Plan", Enabled: true, ConcurrencyLimit: 7, QueueCapacity: 9, BusinessCode: &code}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7103, UserId: 7101, PlanId: 7102, Status: "active", StartTime: now - 60, EndTime: now + 3600, ConcurrencyLimit: 2, GrantReason: "order"}).Error)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{7101})
	require.NoError(t, err)
	assert.Equal(t, 7, limits[7101].Limit)
	assert.Equal(t, 9, limits[7101].QueueCapacity)
}

func TestGetAdminOpsUserConcurrencyLimitsFallsBackToRuntimeDefaultQueueCapacity(t *testing.T) {
	db := setupAdminOpsModelTestDB(t)
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })
	now := GetDBTimestamp()
	oldDefaultQueueCapacity := common.SubscriptionConcurrencyQueueCapacity
	common.SubscriptionConcurrencyQueueCapacity = 6
	t.Cleanup(func() { common.SubscriptionConcurrencyQueueCapacity = oldDefaultQueueCapacity })
	require.NoError(t, DB.Create(&User{Id: 7104, Username: "ops-fallback", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7105, UserId: 7104, Status: "active", StartTime: now - 60, EndTime: now + 3600, ConcurrencyLimit: 2, GrantReason: "order"}).Error)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{7104})
	require.NoError(t, err)
	assert.Equal(t, 2, limits[7104].Limit)
	assert.Equal(t, 6, limits[7104].QueueCapacity)
}
```

测试 helper 使用内存 SQLite，并 `AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{})`。

实现原则：

- traffic 从 `LOG_DB.Model(&Log{})` 查询。
- requests 统计 consume + error。
- errors 统计 `LogTypeError`。
- total tokens 使用 `prompt_tokens + completion_tokens` 的 GORM Select；如果跨库表达式有问题，退回读取汇总行后在 Go 中累加。
- channels 从 `DB.Model(&Channel{})` 查询 status、response_time、test_time。
- active subscription limit 查询必须与运行时 `livePlanConcurrencyLimit` / `livePlanQueueCapacity` / `AcquireUserConcurrencyWithQueueCapacity` 口径一致：先按用户查询当前 active subscription，再批量查询 plan；如果 plan 存在，优先用 `plan.ConcurrencyLimit` 和 `plan.QueueCapacity`；只有 plan 缺失时才回退 `UserSubscription.ConcurrencyLimit`；当 plan queue_capacity 缺失、plan 不存在或容量 `<= 0` 时，必须回退 `common.SubscriptionConcurrencyQueueCapacity`，不能回退 0。不要写 raw SQL join；可分两步查询 subscription、plan 和 user。

- [ ] **步骤 5.5：扩展 perf metrics 模型级 TTFT 汇总并测试**

在 `pkg/perf_metrics/types.go` 给 `ModelSummary` 增加：

```go
AvgTtftMs int64 `json:"avg_ttft_ms"`
```

在 `model/perf_metric.go` 中先扩展历史 DB 汇总：

1. `PerfMetricSummary` 增加 `TtftSumMs int64`、`TtftCount int64`；
2. `GetPerfMetricsSummaryAll` 的 `Select` 增加 `SUM(ttft_sum_ms) as ttft_sum_ms, SUM(ttft_count) as ttft_count`；
3. 增加 DB 汇总测试，验证已落库 bucket 的 TTFT 会出现在 `PerfMetricSummary`。

在 `pkg/perf_metrics/metrics.go` 的 `QuerySummaryAll` 中：

1. 从 `model.GetPerfMetricsSummaryAll` 行合并 `TtftSumMs` 和 `TtftCount`；
2. 从本进程 `hotBuckets` 合并 `snap.ttftSumMs` 和 `snap.ttftCount`；
3. 从 Redis 活跃 bucket 合并现有 writer 使用的字段 `ttft` / `ttft_n`（复用 `redisCounters`），不要读取不存在的 `ttft_sum_ms` / `ttft_count`。实现时新增可测试的内部 helper（例如 `queryRedisActivePerfMetricBuckets(ctx, redisReader, hours)`），用 Redis 模型索引或受控 bucket 索引读取，禁止全库 `KEYS`；如果当前 Redis 写入缺少模型索引，本任务需补 `perf:metrics:active_models` 这类轻量索引并在 `recordRedis` 写入活跃桶时 best-effort 写入；
4. 输出 `ModelSummary{AvgTtftMs: avg(total.ttftSumMs, total.ttftCount)}`。

```go
func TestQuerySummaryAllIncludesAvgTtftMs(t *testing.T) {
	resetPerfMetricsTestState(t)
	Record(Sample{Model: "ops-ttft", LatencyMs: 200, TtftMs: 40, HasTtft: true, Success: true})
	Record(Sample{Model: "ops-ttft", LatencyMs: 300, TtftMs: 80, HasTtft: true, Success: true})
	result, err := QuerySummaryAll(24)
	require.NoError(t, err)
	require.NotEmpty(t, result.Models)
	assert.EqualValues(t, 60, result.Models[0].AvgTtftMs)
}

func TestQuerySummaryAllIncludesRedisActiveBucketTtft(t *testing.T) {
	resetPerfMetricsTestState(t)
	fake := newPerfMetricsRedisFake()
	seedPerfMetricsRedisActiveBucket(fake, "ops-redis-ttft", 300, 3)
	result, err := querySummaryAllWithRedisReader(24, fake)
	require.NoError(t, err)
	assert.EqualValues(t, 100, findModelSummary(result.Models, "ops-redis-ttft").AvgTtftMs)
}
```

如果同包现有测试没有 `resetPerfMetricsTestState`，新增 helper 清空 `hotBuckets` 并临时开启 `perf_metrics_setting`；Redis 活跃桶测试使用 fake reader，不依赖真实 Redis。

- [ ] **步骤 6：实现 service 聚合**

在 `service/admin_ops.go` 中实现：

```go
func GetAdminOpsSnapshot(ctx context.Context, query AdminOpsSnapshotQuery) (*dto.AdminOpsSnapshotResponse, error)
func GetAdminOpsConcurrency(ctx context.Context, query AdminOpsConcurrencyQuery) (*dto.AdminOpsConcurrencyResponse, error)
```

必须聚合：

- generated_at；
- runtime：`common.Version`、`common.StartTime`、`common.NodeName`、`middleware.GetStats()`、`runtime.NumGoroutine()`；
- system：`common.GetSystemStatus()`；
- dependencies：DB ping，Redis ping；
- concurrency：调用订阅并发 snapshot，先用完整 runtime rows 批量查询并补充所有 active/queued 用户的 limit / queue_capacity，用富化后的完整 rows 计算 summary 和 health，再用 query.MinActiveOrQueued 与 query.Limit 裁剪 users 明细；`include_users=false` 只能省略明细 username/列表，不能省略 summary 所需容量查询；
- traffic：`model.GetAdminOpsTrafficStats`；
- channels：`model.GetAdminOpsChannelStats`；
- performance：`perfmetrics.QuerySummaryAll(24)`，最多取 query.Top，并映射 `AvgTtftMs`；
- recent_errors：`model.GetAdminOpsRecentErrors`，映射 DTO 时必须对 `Log.Content` 调用 `sanitizeAdminOpsRecentErrorContent`，不得把原始 content 赋给 `AdminOpsRecentError.Content`；
- health：合并 DB、Redis、system、concurrency、channels、traffic reasons。

- [ ] **步骤 7：注册路由**

修改 `router/api-router.go`，在 `adminAnalyticsRoute` 附近或 channelRoute 之前添加：

```go
adminOpsRoute := apiRouter.Group("/admin-ops")
adminOpsRoute.Use(middleware.AdminAuth())
{
	adminOpsRoute.GET("/snapshot", controller.GetAdminOpsSnapshot)
	adminOpsRoute.GET("/concurrency", controller.GetAdminOpsConcurrency)
}
```

- [ ] **步骤 8：运行后端针对性测试**

运行：

```bash
go test ./service ./controller ./model ./pkg/perf_metrics -run 'TestAdminOps|TestParseAdminOps|Test.*SubscriptionConcurrency|TestGetAdminOpsUserConcurrencyLimitsPrefersPlanValues|TestQuerySummaryAllIncludesAvgTtftMs|TestQuerySummaryAllIncludesRedisActiveBucketTtft' -count=1
```

预期：PASS。

---

## 任务 4：前端类型、API、纯函数与测试

**文件：**
- 创建：`web/default/src/features/admin-ops/types.ts`
- 创建：`web/default/src/features/admin-ops/api.ts`
- 创建：`web/default/src/features/admin-ops/lib/health.ts`
- 创建：`web/default/src/features/admin-ops/lib/format.ts`
- 创建：`web/default/src/features/admin-ops/lib/health.test.ts`
- 创建：`web/default/src/features/admin-ops/lib/format.test.ts`

- [ ] **步骤 1：编写失败的 health 测试**

创建 `web/default/src/features/admin-ops/lib/health.test.ts`。前端测试使用当前仓库已有 `node:test` + `node:assert/strict` 风格，不引入 Vitest。

```ts
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getAdminOpsHealthTone,
  getAdminOpsConcurrencyUserStatus,
} from './health'

// This file tests pure display classification only; React components use these helpers.
describe('admin ops health helpers', () => {
  test('maps backend health status to display tones', () => {
    assert.equal(getAdminOpsHealthTone('healthy'), 'success')
    assert.equal(getAdminOpsHealthTone('degraded'), 'warning')
    assert.equal(getAdminOpsHealthTone('critical'), 'destructive')
    assert.equal(getAdminOpsHealthTone('unknown'), 'muted')
  })

  test('classifies concurrency user pressure', () => {
    assert.equal(getAdminOpsConcurrencyUserStatus({ active: 1, limit: 4, queued: 0, queue_capacity: 4 }), 'normal')
    assert.equal(getAdminOpsConcurrencyUserStatus({ active: 4, limit: 4, queued: 0, queue_capacity: 4 }), 'saturated')
    assert.equal(getAdminOpsConcurrencyUserStatus({ active: 4, limit: 4, queued: 2, queue_capacity: 4 }), 'queued')
    assert.equal(getAdminOpsConcurrencyUserStatus({ active: 4, limit: 4, queued: 4, queue_capacity: 4 }), 'queue_full_risk')
  })
})
```

- [ ] **步骤 2：运行 health 测试验证失败**

运行：

```bash
cd web/default && bunx tsx --test src/features/admin-ops/lib/health.test.ts
```

预期：FAIL，模块不存在或导出函数未定义。

- [ ] **步骤 3：实现前端类型与 health helper**

创建 `web/default/src/features/admin-ops/types.ts`，类型字段必须与后端 JSON 对齐。

```ts
export type AdminOpsHealthStatus = 'healthy' | 'degraded' | 'critical'
export type AdminOpsDependencyStatus = 'healthy' | 'disabled' | 'critical'
export type AdminOpsConcurrencyMode = 'redis' | 'memory' | 'disabled'

export type AdminOpsHealth = { status: AdminOpsHealthStatus; score: number; reasons: string[] }
export type AdminOpsRuntime = { version: string; start_time: number; uptime_seconds: number; node_name: string; active_connections: number; goroutines: number }
export type AdminOpsSystem = { cpu_usage: number; memory_usage: number; disk_usage: number }
export type AdminOpsDependency = { enabled: boolean; status: AdminOpsDependencyStatus; latency_ms: number; message: string }
export type AdminOpsDependencies = { database: AdminOpsDependency; redis: AdminOpsDependency }
export type AdminOpsConcurrencySummary = { total_active: number; total_queued: number; active_users: number; queued_users: number; saturated_users: number; queue_pressure: number }
export type AdminOpsConcurrencyConfig = { ttl_seconds: number; default_queue_capacity: number; require_redis: boolean; fail_open: boolean }
export type AdminOpsConcurrencyCounters = { acquired_total: number; queued_total: number; queue_full_rejections_total: number; unavailable_rejections_total: number; redis_errors_total: number }
export type AdminOpsConcurrencyUser = { user_id: number; username: string; active: number; limit: number; queued: number; queue_capacity: number; oldest_queued_seconds: number; utilization: number; queue_utilization: number; status: string }
export type AdminOpsConcurrencyResponse = { mode: AdminOpsConcurrencyMode; generated_at: number; enabled: boolean; summary: AdminOpsConcurrencySummary; config: AdminOpsConcurrencyConfig; counters: AdminOpsConcurrencyCounters; users: AdminOpsConcurrencyUser[] }
export type AdminOpsTraffic = { window_seconds: number; requests: number; errors: number; rpm: number; tpm: number; error_rate: number }
export type AdminOpsChannels = { total: number; enabled: number; manual_disabled: number; auto_disabled: number; slow_count: number; stale_test_count: number }
export type AdminOpsPerformanceModel = { model_name: string; avg_latency_ms: number; avg_ttft_ms: number; success_rate: number; avg_tps: number; request_count: number }
export type AdminOpsPerformance = { models: AdminOpsPerformanceModel[] }
export type AdminOpsRecentError = { id: number; created_at: number; user_id: number; username: string; model_name: string; channel_id: number; content: string; request_id: string }

export type AdminOpsSnapshot = {
  generated_at: number
  health: AdminOpsHealth
  runtime: AdminOpsRuntime
  system: AdminOpsSystem
  dependencies: AdminOpsDependencies
  concurrency: AdminOpsConcurrencyResponse
  traffic: AdminOpsTraffic
  channels: AdminOpsChannels
  performance: AdminOpsPerformance
  recent_errors: AdminOpsRecentError[]
}

export type AdminOpsApiResponse<T> = { success: boolean; message?: string; data: T }
```

创建 `web/default/src/features/admin-ops/lib/health.ts`：

```ts
import type { AdminOpsHealthStatus } from '../types'

type Tone = 'success' | 'warning' | 'destructive' | 'muted'

type ConcurrencyPressureInput = {
  active: number
  limit: number
  queued: number
  queue_capacity: number
}

export function getAdminOpsHealthTone(status: string): Tone {
  if (status === 'healthy') return 'success'
  if (status === 'degraded') return 'warning'
  if (status === 'critical') return 'destructive'
  return 'muted'
}

export function getAdminOpsHealthLabelKey(status: AdminOpsHealthStatus): string {
  return `adminOps.health.${status}`
}

export function getAdminOpsConcurrencyUserStatus(
  value: ConcurrencyPressureInput
): 'normal' | 'saturated' | 'queued' | 'queue_full_risk' {
  if (value.queue_capacity > 0 && value.queued >= value.queue_capacity) {
    return 'queue_full_risk'
  }
  if (value.queued > 0) return 'queued'
  if (value.limit > 0 && value.active >= value.limit) return 'saturated'
  return 'normal'
}
```

- [ ] **步骤 4：运行 health 测试通过**

运行：

```bash
cd web/default && bunx tsx --test src/features/admin-ops/lib/health.test.ts
```

预期：PASS。

- [ ] **步骤 5：编写失败的 format 测试**

创建 `web/default/src/features/admin-ops/lib/format.test.ts`：

```ts
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  formatAdminOpsBytes,
  formatAdminOpsCount,
  formatAdminOpsDuration,
  formatAdminOpsPercent,
  formatAdminOpsRate,
} from './format'

describe('admin ops format helpers', () => {
  test('formats counts compactly', () => {
    assert.equal(formatAdminOpsCount(999), '999')
    assert.equal(formatAdminOpsCount(1200), '1.2K')
  })

  test('formats percentage from ratio', () => {
    assert.equal(formatAdminOpsPercent(0.1234), '12.3%')
    assert.equal(formatAdminOpsPercent(1), '100.0%')
  })

  test('formats seconds as readable duration', () => {
    assert.equal(formatAdminOpsDuration(59), '59s')
    assert.equal(formatAdminOpsDuration(61), '1m 1s')
    assert.equal(formatAdminOpsDuration(3661), '1h 1m')
  })

  test('formats rate with unit', () => {
    assert.equal(formatAdminOpsRate(12.345, 'rpm'), '12.3 rpm')
  })

  test('formats bytes compactly', () => {
    assert.equal(formatAdminOpsBytes(1023), '1023 B')
    assert.equal(formatAdminOpsBytes(1024), '1.0 KB')
    assert.equal(formatAdminOpsBytes(1536 * 1024), '1.5 MB')
  })
})
```

- [ ] **步骤 6：运行 format 测试验证失败**

运行：

```bash
cd web/default && bunx tsx --test src/features/admin-ops/lib/format.test.ts
```

预期：FAIL，模块不存在。

- [ ] **步骤 7：实现 format helper**

创建 `web/default/src/features/admin-ops/lib/format.ts`：

```ts
export function formatAdminOpsCount(value: number): string {
  if (!Number.isFinite(value)) return '0'
  const abs = Math.abs(value)
  if (abs >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (abs >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return Math.round(value).toString()
}

export function formatAdminOpsPercent(value: number): string {
  if (!Number.isFinite(value)) return '0.0%'
  return `${(value * 100).toFixed(1)}%`
}

export function formatAdminOpsDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s'
  const whole = Math.floor(seconds)
  if (whole < 60) return `${whole}s`
  const minutes = Math.floor(whole / 60)
  const restSeconds = whole % 60
  if (minutes < 60) return `${minutes}m ${restSeconds}s`
  const hours = Math.floor(minutes / 60)
  const restMinutes = minutes % 60
  return `${hours}h ${restMinutes}m`
}

export function formatAdminOpsRate(value: number, unit: string): string {
  if (!Number.isFinite(value)) return `0.0 ${unit}`
  return `${value.toFixed(1)} ${unit}`
}

export function formatAdminOpsBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let current = value
  let unitIndex = 0
  for (; unitIndex < units.length - 1 && current >= 1024; unitIndex++) {
    current /= 1024
  }
  if (unitIndex === 0) return `${Math.round(current)} ${units[unitIndex]}`
  return `${current.toFixed(1)} ${units[unitIndex]}`
}
```

- [ ] **步骤 8：补充 i18n 与侧边栏配置契约测试**

在 `web/default/src/features/admin-ops/lib/i18n-keys.test.ts` 增加 `node:test`，读取 `src/i18n/locales/en.json`、`zh.json`、`fr.json`、`ja.json`、`ru.json`、`vi.json`，断言任务 5 列出的所有静态 `adminOps.*` key 都存在且值非空；对动态 key（如 `adminOps.concurrency.status.normal/saturated/queued/queue_full_risk`、`adminOps.health.healthy/degraded/critical`）逐项列入测试清单，不能只依赖 `static-keys.ts`。

更新现有 `web/default/src/hooks/use-sidebar-config.test.ts`，增加：

1. admin ops 在默认 admin sidebar 中可见；
2. 用户配置关闭 admin ops 后入口可隐藏；
3. `/admin-ops` 映射到正确 sidebar module key；
4. `web/default/src/routeTree.gen.ts` 包含 admin-ops route import/path，确保新增路由已进入 TanStack route tree。

- [ ] **步骤 9：运行前端纯函数测试，并确认配置契约测试暂时失败**

运行：

```bash
cd web/default && bunx tsx --test src/features/admin-ops/lib/health.test.ts src/features/admin-ops/lib/format.test.ts
cd web/default && bunx tsx --test src/features/admin-ops/lib/i18n-keys.test.ts src/hooks/use-sidebar-config.test.ts
```

预期：health/format PASS；i18n/sidebar 配置契约测试 FAIL，因为任务 5 尚未补齐 locale、sidebar 配置和 routeTree 注册。不得删除或放宽这些契约测试；任务 5 完成后必须让完整前端测试命令 PASS。

- [ ] **步骤 10：实现 API 封装**

创建 `web/default/src/features/admin-ops/api.ts`：

```ts
import { api } from '@/lib/api'
import type {
  AdminOpsApiResponse,
  AdminOpsConcurrencyResponse,
  AdminOpsSnapshot,
} from './types'

export async function getAdminOpsSnapshot(params: {
  window_seconds: number
  top: number
}): Promise<AdminOpsApiResponse<AdminOpsSnapshot>> {
  const res = await api.get<AdminOpsApiResponse<AdminOpsSnapshot>>(
    '/api/admin-ops/snapshot',
    { params, disableDuplicate: true } as Record<string, unknown>
  )
  return res.data
}

export async function getAdminOpsConcurrency(params: {
  limit: number
  include_users: boolean
  min_active_or_queued: number
}): Promise<AdminOpsApiResponse<AdminOpsConcurrencyResponse>> {
  const res = await api.get<AdminOpsApiResponse<AdminOpsConcurrencyResponse>>(
    '/api/admin-ops/concurrency',
    { params, disableDuplicate: true } as Record<string, unknown>
  )
  return res.data
}
```

---

## 任务 5：前端页面、卡片组件、路由、导航与 i18n

**文件：**
- 创建：`web/default/src/routes/_authenticated/admin-ops/index.tsx`
- 创建：`web/default/src/features/admin-ops/index.tsx`
- 创建：`web/default/src/features/admin-ops/components/admin-ops-header.tsx`
- 创建：`web/default/src/features/admin-ops/components/health-summary-cards.tsx`
- 创建：`web/default/src/features/admin-ops/components/concurrency-queue-card.tsx`
- 创建：`web/default/src/features/admin-ops/components/realtime-traffic-card.tsx`
- 创建：`web/default/src/features/admin-ops/components/channel-health-card.tsx`
- 创建：`web/default/src/features/admin-ops/components/performance-models-card.tsx`
- 创建：`web/default/src/features/admin-ops/components/recent-errors-card.tsx`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 修改：`web/default/src/features/system-settings/maintenance/config.ts`
- 修改：`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx`
- 修改：`web/default/src/features/profile/components/sidebar-modules-card.tsx`
- 修改：`web/default/src/routeTree.gen.ts`（由 TanStack Router 生成，禁止手写）
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/vi.json`

- [ ] **步骤 1：创建路由守卫**

创建 `web/default/src/routes/_authenticated/admin-ops/index.tsx`：

```tsx
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { AdminOpsPage } from '@/features/admin-ops'

export const Route = createFileRoute('/_authenticated/admin-ops/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: AdminOpsPage,
})
```

- [ ] **步骤 2：创建页面容器**

创建 `web/default/src/features/admin-ops/index.tsx`。使用 React Query 轮询，snapshot 30 秒、concurrency 5 秒。

```tsx
import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { getAdminOpsConcurrency, getAdminOpsSnapshot } from './api'
import { AdminOpsHeader } from './components/admin-ops-header'
import { ChannelHealthCard } from './components/channel-health-card'
import { ConcurrencyQueueCard } from './components/concurrency-queue-card'
import { HealthSummaryCards } from './components/health-summary-cards'
import { PerformanceModelsCard } from './components/performance-models-card'
import { RealtimeTrafficCard } from './components/realtime-traffic-card'
import { RecentErrorsCard } from './components/recent-errors-card'

const SNAPSHOT_REFETCH_MS = 30_000
const CONCURRENCY_REFETCH_MS = 5_000

export function AdminOpsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [autoRefresh, setAutoRefresh] = useState(true)

  const snapshotQuery = useQuery({
    queryKey: ['admin-ops', 'snapshot', 300, 5],
    queryFn: () => getAdminOpsSnapshot({ window_seconds: 300, top: 5 }),
    refetchInterval: autoRefresh ? SNAPSHOT_REFETCH_MS : false,
    refetchIntervalInBackground: false,
  })

  const concurrencyQuery = useQuery({
    queryKey: ['admin-ops', 'concurrency', 20],
    queryFn: () =>
      getAdminOpsConcurrency({
        limit: 20,
        include_users: true,
        min_active_or_queued: 1,
      }),
    refetchInterval: autoRefresh ? CONCURRENCY_REFETCH_MS : false,
    refetchIntervalInBackground: false,
  })

  const snapshot = snapshotQuery.data?.success ? snapshotQuery.data.data : undefined
  const concurrency = concurrencyQuery.data?.success
    ? concurrencyQuery.data.data
    : snapshot?.concurrency

  const hasError = snapshotQuery.isError || concurrencyQuery.isError
  const isRefreshing = snapshotQuery.isFetching || concurrencyQuery.isFetching
  const generatedAt = useMemo(
    () => Math.max(snapshot?.generated_at ?? 0, concurrency?.generated_at ?? 0),
    [snapshot?.generated_at, concurrency?.generated_at]
  )

  function refreshAll() {
    void queryClient.invalidateQueries({ queryKey: ['admin-ops'] })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('adminOps.title')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('adminOps.description')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <AdminOpsHeader
            health={snapshot?.health}
            generatedAt={generatedAt}
            refreshing={isRefreshing}
            autoRefresh={autoRefresh}
            onAutoRefreshChange={setAutoRefresh}
            onRefresh={refreshAll}
          />
          {hasError ? (
            <ErrorState
              title={t('adminOps.failedToLoad')}
              description={t('adminOps.failedToLoadDescription')}
            />
          ) : null}
          <HealthSummaryCards snapshot={snapshot} loading={snapshotQuery.isLoading} />
          <ConcurrencyQueueCard
            concurrency={concurrency}
            loading={concurrencyQuery.isLoading && !concurrency}
          />
          <div className='grid grid-cols-1 gap-4 xl:grid-cols-2'>
            <RealtimeTrafficCard snapshot={snapshot} loading={snapshotQuery.isLoading} />
            <ChannelHealthCard snapshot={snapshot} loading={snapshotQuery.isLoading} />
          </div>
          <div className='grid grid-cols-1 gap-4 xl:grid-cols-2'>
            <PerformanceModelsCard snapshot={snapshot} loading={snapshotQuery.isLoading} />
            <RecentErrorsCard snapshot={snapshot} loading={snapshotQuery.isLoading} />
          </div>
          <Button variant='outline' className='sr-only' onClick={refreshAll}>
            {t('Refresh')}
          </Button>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
```

如果 `SectionPageLayout` 或 `ErrorState` 的 import 路径不同，使用现有 `admin-analytics` 中的准确路径。

- [ ] **步骤 3：创建 Header 和卡片组件**

每个组件保持专注。必须使用 `useTranslation()`。示例 `concurrency-queue-card.tsx`：

```tsx
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { AdminOpsConcurrencyResponse } from '../types'
import { formatAdminOpsCount, formatAdminOpsDuration, formatAdminOpsPercent } from '../lib/format'
import { getAdminOpsConcurrencyUserStatus } from '../lib/health'

export function ConcurrencyQueueCard(props: {
  concurrency?: AdminOpsConcurrencyResponse
  loading: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) return <Skeleton className='h-64 w-full' />
  const data = props.concurrency
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('adminOps.concurrency.title')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='grid grid-cols-2 gap-3 lg:grid-cols-4'>
          <Metric label={t('adminOps.concurrency.activeSlots')} value={formatAdminOpsCount(data?.summary.total_active ?? 0)} />
          <Metric label={t('adminOps.concurrency.queuedRequests')} value={formatAdminOpsCount(data?.summary.total_queued ?? 0)} />
          <Metric label={t('adminOps.concurrency.saturatedUsers')} value={formatAdminOpsCount(data?.summary.saturated_users ?? 0)} />
          <Metric label={t('adminOps.concurrency.queuePressure')} value={formatAdminOpsPercent(data?.summary.queue_pressure ?? 0)} />
          <Metric label={t('adminOps.concurrency.acquiredTotal')} value={formatAdminOpsCount(data?.counters.acquired_total ?? 0)} />
          <Metric label={t('adminOps.concurrency.queuedTotal')} value={formatAdminOpsCount(data?.counters.queued_total ?? 0)} />
          <Metric label={t('adminOps.concurrency.queueFullRejections')} value={formatAdminOpsCount(data?.counters.queue_full_rejections_total ?? 0)} />
          <Metric label={t('adminOps.concurrency.unavailableRejections')} value={formatAdminOpsCount(data?.counters.unavailable_rejections_total ?? 0)} />
          <Metric label={t('adminOps.concurrency.redisErrors')} value={formatAdminOpsCount(data?.counters.redis_errors_total ?? 0)} />
        </div>
        <div className='text-muted-foreground text-xs'>
          {t('adminOps.concurrency.mode')}: {data?.mode ?? 'disabled'} · {t('adminOps.concurrency.ttl')}: {data?.config.ttl_seconds ?? 0}s
        </div>
        <div className='overflow-x-auto'>
          <table className='w-full text-sm'>
            <thead className='text-muted-foreground text-left text-xs'>
              <tr>
                <th className='py-2'>{t('User')}</th>
                <th className='py-2'>{t('adminOps.concurrency.active')}</th>
                <th className='py-2'>{t('adminOps.concurrency.queued')}</th>
                <th className='py-2'>{t('adminOps.concurrency.oldestQueued')}</th>
                <th className='py-2'>{t('Status')}</th>
              </tr>
            </thead>
            <tbody>
              {(data?.users ?? []).map((user) => {
                const status = getAdminOpsConcurrencyUserStatus(user)
                return (
                  <tr key={user.user_id} className='border-t'>
                    <td className='py-2'>{user.username || `#${user.user_id}`}</td>
                    <td className='py-2'>{user.active}/{user.limit || '∞'}</td>
                    <td className='py-2'>{user.queued}/{user.queue_capacity}</td>
                    <td className='py-2'>{formatAdminOpsDuration(user.oldest_queued_seconds)}</td>
                    <td className='py-2'>{t(`adminOps.concurrency.status.${status}`)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </CardContent>
    </Card>
  )
}

function Metric(props: { label: string; value: string }) {
  return (
    <div className='rounded-lg border p-3'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='text-xl font-semibold'>{props.value}</div>
    </div>
  )
}
```

其他卡片按同样方式展示 snapshot 数据，不引入复杂交互。

- [ ] **步骤 4：更新导航配置**

修改 `web/default/src/hooks/use-sidebar-data.ts`：

- import 图标增加 `Gauge` 或复用 `Activity`。
- Admin items 中 `adminAnalytics` 后新增：

```tsx
{
  title: t('adminOps.title'),
  url: '/admin-ops',
  icon: Activity,
},
```

修改 `web/default/src/hooks/use-sidebar-config.ts`：

- `DEFAULT_SIDEBAR_MODULES.admin.ops = true`。
- `URL_TO_CONFIG_MAP` 增加：

```ts
'/admin-ops': { section: 'admin', module: 'ops' },
```

同时更新所有侧边栏配置入口：

- `web/default/src/features/system-settings/maintenance/config.ts` 的 `SIDEBAR_MODULES_DEFAULT.admin.ops = true`。
- `web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx` 增加 admin ops 模块展示元数据。
- `web/default/src/features/profile/components/sidebar-modules-card.tsx` 增加用户侧可配置模块元数据。

新增路由文件后运行项目现有生成流程更新 `web/default/src/routeTree.gen.ts`，不要手写该文件：

```bash
cd web/default && bun run build
```

- [ ] **步骤 5：更新 i18n**

`static-keys.ts` 增加以下 key；新增组件中的用户可见字符串必须全部使用这些 key 或已有通用 key（如 `User`、`Status`），不得硬编码：

```ts
'adminOps.title',
'adminOps.description',
'adminOps.failedToLoad',
'adminOps.failedToLoadDescription',
'adminOps.header.lastUpdated',
'adminOps.header.autoRefresh',
'adminOps.header.manualRefresh',
'adminOps.header.refreshing',
'adminOps.health.healthy',
'adminOps.health.degraded',
'adminOps.health.critical',
'adminOps.health.score',
'adminOps.health.reasons',
'adminOps.healthSummary.title',
'adminOps.healthSummary.database',
'adminOps.healthSummary.redis',
'adminOps.healthSummary.cpu',
'adminOps.healthSummary.memory',
'adminOps.healthSummary.disk',
'adminOps.healthSummary.activeConnections',
'adminOps.healthSummary.goroutines',
'adminOps.dependency.healthy',
'adminOps.dependency.disabled',
'adminOps.dependency.critical',
'adminOps.concurrency.title',
'adminOps.concurrency.activeSlots',
'adminOps.concurrency.queuedRequests',
'adminOps.concurrency.activeUsers',
'adminOps.concurrency.queuedUsers',
'adminOps.concurrency.saturatedUsers',
'adminOps.concurrency.queuePressure',
'adminOps.concurrency.acquiredTotal',
'adminOps.concurrency.queuedTotal',
'adminOps.concurrency.queueFullRejections',
'adminOps.concurrency.unavailableRejections',
'adminOps.concurrency.redisErrors',
'adminOps.concurrency.mode',
'adminOps.concurrency.ttl',
'adminOps.concurrency.defaultQueueCapacity',
'adminOps.concurrency.requireRedis',
'adminOps.concurrency.failOpen',
'adminOps.concurrency.active',
'adminOps.concurrency.queued',
'adminOps.concurrency.oldestQueued',
'adminOps.concurrency.utilization',
'adminOps.concurrency.queueUtilization',
'adminOps.concurrency.status.normal',
'adminOps.concurrency.status.saturated',
'adminOps.concurrency.status.queued',
'adminOps.concurrency.status.queue_full_risk',
'adminOps.traffic.title',
'adminOps.traffic.requests',
'adminOps.traffic.errors',
'adminOps.traffic.rpm',
'adminOps.traffic.tpm',
'adminOps.traffic.errorRate',
'adminOps.traffic.window',
'adminOps.channels.title',
'adminOps.channels.total',
'adminOps.channels.enabled',
'adminOps.channels.manualDisabled',
'adminOps.channels.autoDisabled',
'adminOps.channels.slow',
'adminOps.channels.staleTest',
'adminOps.performance.title',
'adminOps.performance.model',
'adminOps.performance.avgLatency',
'adminOps.performance.avgTtft',
'adminOps.performance.successRate',
'adminOps.performance.avgTps',
'adminOps.performance.requestCount',
'adminOps.recentErrors.title',
'adminOps.recentErrors.empty',
'adminOps.recentErrors.model',
'adminOps.recentErrors.channel',
'adminOps.recentErrors.requestId',
'adminOps.recentErrors.createdAt',
'adminOps.empty.noData'
```

同步更新 `en.json`、`zh.json`、`fr.json`、`ja.json`、`ru.json`、`vi.json`。如果不会准确翻译某语言，使用清晰英文 fallback，后续 `i18n:sync` 可补齐，但不能缺 key。实现后运行或增加轻量检查，确保所有 `adminOps.*` key 在 6 个 locale 中都存在。

- [ ] **步骤 6：运行前端类型检查**

运行：

```bash
cd web/default && bun run typecheck
```

预期：PASS。

---

## 任务 6：最终集成验证与审查修复

**文件：**
- 可能修改所有前述文件，仅修复验证和审查发现的问题。

- [ ] **步骤 1：运行后端针对性测试**

运行：

```bash
go test ./service ./controller ./model ./pkg/perf_metrics -run 'TestAdminOps|TestParseAdminOps|Test.*SubscriptionConcurrency|TestGetAdminOpsUserConcurrencyLimitsPrefersPlanValues|TestQuerySummaryAllIncludesAvgTtftMs|TestQuerySummaryAllIncludesRedisActiveBucketTtft' -count=1
```

预期：PASS。

- [ ] **步骤 2：运行前端 admin-ops 纯函数、i18n 与侧边栏配置测试**

运行：

```bash
cd web/default && bunx tsx --test src/features/admin-ops/lib/health.test.ts src/features/admin-ops/lib/format.test.ts src/features/admin-ops/lib/i18n-keys.test.ts src/hooks/use-sidebar-config.test.ts
```

预期：PASS。

- [ ] **步骤 3：运行前端构建并更新路由树**

运行：

```bash
cd web/default && bun run build
```

预期：PASS，且 `web/default/src/routeTree.gen.ts` 包含新 `/admin-ops` 路由。若 build 未更新 route tree，使用项目认可的 TanStack Router 生成方式更新后再运行 typecheck。

- [ ] **步骤 4：运行前端类型检查**

运行：

```bash
cd web/default && bun run typecheck
```

预期：PASS。

- [ ] **步骤 5：审查变更范围**

运行：

```bash
git diff --stat
git diff -- router/api-router.go controller/admin_ops.go service/admin_ops.go service/subscription_concurrency.go dto/admin_ops.go model/admin_ops.go pkg/perf_metrics/types.go pkg/perf_metrics/metrics.go web/default/src/features/admin-ops web/default/src/routes/_authenticated/admin-ops web/default/src/hooks/use-sidebar-data.ts web/default/src/hooks/use-sidebar-config.ts web/default/src/features/system-settings/maintenance/config.ts web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx web/default/src/features/profile/components/sidebar-modules-card.tsx web/default/src/routeTree.gen.ts
```

预期：只包含本计划相关变更；没有受保护品牌信息修改；没有无关格式化。

- [ ] **步骤 6：根据 review 子代理反馈修复**

若 review 发现问题：

1. 修复对应代码。
2. 重新运行相关测试。
3. 重新发起 review。
4. 直到所有 review 子代理通过。

- [ ] **步骤 7：最终验证**

运行：

```bash
go test ./service ./controller ./model ./pkg/perf_metrics -run 'TestAdminOps|TestParseAdminOps|Test.*SubscriptionConcurrency|TestGetAdminOpsUserConcurrencyLimitsPrefersPlanValues|TestQuerySummaryAllIncludesAvgTtftMs|TestQuerySummaryAllIncludesRedisActiveBucketTtft' -count=1
(cd web/default && bunx tsx --test src/features/admin-ops/lib/health.test.ts src/features/admin-ops/lib/format.test.ts src/features/admin-ops/lib/i18n-keys.test.ts src/hooks/use-sidebar-config.test.ts)
(cd web/default && bun run build)
(cd web/default && bun run typecheck)
```

预期：全部 PASS。
