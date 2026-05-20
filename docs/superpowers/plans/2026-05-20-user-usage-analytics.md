# 用户侧用量分析中心实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现 Phase 1 用户侧 Usage Analytics，让用户按 API Key、模型、分组、流式状态、调用结果分析自己的用量，并能钻取到 Usage Logs 明细。

**架构：** 后端新增 `/api/usage-analytics/{summary,timeseries,breakdown}`，所有接口使用 `UserAuth`，强制按当前 `user_id` 过滤。聚合先查 `LOG_DB`，API Key 补充信息再从 `DB` 批量读取，避免跨库 JOIN；前端新增 `/usage-analytics` 页面，URL search 作为唯一可分享状态，React Query 只使用 canonical filters，Usage Logs 钻取统一传 `status=success|error` 而不是 numeric `type`。

**技术栈：** Go 1.22+、Gin、GORM v2、SQLite/MySQL/PostgreSQL 兼容；React 19、TypeScript、TanStack Router、React Query、Base UI、Tailwind、`@visactor/react-vchart`、Bun。

---

## 依据与边界

- 规格文件：`C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-05-20-user-usage-analytics-spec.md`
- 项目规则：`C:/Users/34404/source/repos/new-api/AGENTS.md`
- 前端规则：`C:/Users/34404/source/repos/new-api/web/default/AGENTS.md`
- 直接在 `C:/Users/34404/source/repos/new-api` 主工作区开发，不创建、不切换 Git worktree。
- 初版不新增聚合表、不实现 endpoint / billing_source / billing_tier / modality、不实现 matrix / 热力图 / 洞察卡片。
- 初版不新增 `logs` 复合索引，依赖现有索引、31 天窗口和 50,000 候选日志上限；性能验收仅覆盖功能正确性和三库 SQL 兼容约束。
- 不展示价格、余额、可用天数、runway 指标。
- 不返回完整 API Key；deleted token 的 `masked_key` 必须为 `null`。
- 所有新增业务 JSON 解析必须使用 `common.UnmarshalJsonStr` 或 `common.*` wrapper。
- 子代理不运行项目级 build / lint / format；主代理最终统一运行验证。

## 文件职责

### 后端新增文件

- `dto/usage_analytics.go`：API 枚举、请求枚举、响应 DTO。DTO 放在 `dto`，由 `controller` 和 `model` 共用。
- `model/usage_analytics.go`：日志过滤、候选日志读取、聚合、P95、bucket、RPM / TPM、Top N / Other、token 补充信息。
- `model/usage_analytics_test.go`：model 层统计口径、分组、Top N、P95、bucket、token 安全和 SQL 兼容测试。
- `controller/usage_analytics.go`：HTTP query 解析、默认值、错误响应、当前用户注入。
- `controller/usage_analytics_test.go`：controller 层参数、鉴权上下文、用户隔离、外部 token、unsupported 参数测试。

### 后端修改文件

- `router/api-router.go`：注册 `/api/usage-analytics` 用户侧路由。
- `model/log.go`：增强用户日志列表与统计过滤，支持 `token_id`、`is_stream`、`status`；self stat 使用 `user_id` 过滤。
- `controller/log.go`：解析新增 Usage Logs drilldown 参数，处理 `status` 与 `type` 冲突。
- `controller/log_usage_analytics_test.go`：覆盖 `/api/log/self` 与 `/api/log/self/stat` 的 drilldown 行为。

### 前端新增文件

- `web/default/src/features/usage-analytics/types.ts`：后端 DTO 对应类型，Phase 1 union 仅包含 `token | model | group | stream | status`。
- `web/default/src/features/usage-analytics/constants.ts`：group、metric、granularity、sort 选项，常量只存 `labelKey`。
- `web/default/src/features/usage-analytics/api.ts`：summary / timeseries / breakdown 请求函数，数组使用 repeated query params。
- `web/default/src/features/usage-analytics/lib/filters.ts`：URL search 归一化、canonical filters、API params、Usage Logs drilldown search。
- `web/default/src/features/usage-analytics/lib/filters.test.ts`：筛选纯函数测试。
- `web/default/src/features/usage-analytics/lib/format.ts`：延迟、百分比、tokens、quota wrapper 格式化。
- `web/default/src/features/usage-analytics/lib/format.test.ts`：格式化测试。
- `web/default/src/features/usage-analytics/lib/chart-data.ts`：Top N / Other、additive 与 rate/latency chart spec 数据转换。
- `web/default/src/features/usage-analytics/lib/chart-data.test.ts`：图表纯函数测试。
- `web/default/src/features/usage-analytics/index.tsx`：页面容器，整合 URL search、React Query、布局。
- `web/default/src/features/usage-analytics/components/usage-analytics-filter-bar.tsx`：筛选栏，draft state + Apply。
- `web/default/src/features/usage-analytics/components/usage-analytics-summary-cards.tsx`：总览卡片。
- `web/default/src/features/usage-analytics/components/usage-trend-chart.tsx`：趋势图。
- `web/default/src/features/usage-analytics/components/usage-breakdown-chart.tsx`：分布图。
- `web/default/src/features/usage-analytics/components/usage-ranking-table.tsx`：排行表与钻取按钮。
- `web/default/src/routes/_authenticated/usage-analytics/index.tsx`：TanStack Router file route。

### 前端修改文件

- `web/default/src/hooks/use-sidebar-data.ts`：在 General 分组中新增 `Usage Analytics`，位置在 `API Keys` 与 `Usage Logs` 之间。
- `web/default/src/hooks/use-sidebar-config.ts`：`URL_TO_CONFIG_MAP` 新增 `/usage-analytics`，复用 `{ section: 'console', module: 'log' }`。
- `web/default/src/features/keys/components/api-keys-primary-buttons.tsx`：新增 `View Usage Analytics` 按钮。
- `web/default/src/features/keys/components/data-table-row-actions.tsx`：新增 `Analyze this Key` 行操作，只使用 `apiKey.id`。
- `web/default/src/routes/_authenticated/usage-logs/$section.tsx`：search schema 新增 `tokenId`、`isStream`、`status`。
- `web/default/src/features/usage-logs/types.ts`：`CommonLogFilters`、`GetLogsParams`、`GetLogStatsParams` 新增 `tokenId` / `token_id`、`isStream` / `is_stream`、`status`。
- `web/default/src/features/usage-logs/lib/utils.ts`：search 到 API 参数映射新增 drilldown 参数，`status` 不转换成 `type`。
- `web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx`：读取和清空 drilldown 条件。
- `web/default/src/i18n/static-keys.ts` 与 `web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`：新增文案。
- `web/default/src/routeTree.gen.ts`：通过 `bun run build` 生成，不手写。

## 并发与串行规则

- 任务 1（后端 model/dto）和任务 3（前端 API/types/lib）可以并行。
- 任务 2 依赖任务 1；任务 2 单独负责 `router/api-router.go`、`controller/log.go`、`model/log.go`，其他子代理不得同时编辑这些文件。
- 任务 4 依赖任务 3；任务 5 依赖任务 3。
- 任务 5 单独负责路由、sidebar、API Keys、Usage Logs 前端 drilldown、i18n、routeTree，其他前端子代理不得同时编辑这些共享入口文件。
- 主代理最终运行所有验证并处理冲突，不提交工作区中与本功能无关的既有改动。

---

### 任务 1：后端 DTO 与 model 聚合

**文件：**
- 创建：`dto/usage_analytics.go`
- 创建：`model/usage_analytics.go`
- 创建：`model/usage_analytics_test.go`

**不修改：** `router/api-router.go`、`controller/log.go`、`model/log.go`、所有前端文件。

- [ ] **步骤 1：编写 model 红灯测试骨架**

创建 `model/usage_analytics_test.go`，包含 in-memory SQLite helper。测试必须真实写入 `Log` 和 `Token`，不 mock model 业务逻辑。

```go
package model

import (
    "strings"
    "testing"
    "time"

    "github.com/QuantumNous/new-api/common"
    "github.com/glebarez/sqlite"
    "github.com/stretchr/testify/require"
    "gorm.io/gorm"
)

func setupUsageAnalyticsModelTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    oldDB := DB
    oldLogDB := LOG_DB
    oldSQLite := common.UsingSQLite
    oldMySQL := common.UsingMySQL
    oldPostgres := common.UsingPostgreSQL
    oldRedis := common.RedisEnabled

    common.UsingSQLite = true
    common.UsingMySQL = false
    common.UsingPostgreSQL = false
    common.RedisEnabled = false
    initCol()

    dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
    db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
    require.NoError(t, err)
    DB = db
    LOG_DB = db
    require.NoError(t, DB.AutoMigrate(&Token{}))
    require.NoError(t, LOG_DB.AutoMigrate(&Log{}))

    t.Cleanup(func() {
        DB = oldDB
        LOG_DB = oldLogDB
        common.UsingSQLite = oldSQLite
        common.UsingMySQL = oldMySQL
        common.UsingPostgreSQL = oldPostgres
        common.RedisEnabled = oldRedis
        initCol()
        sqlDB, dbErr := db.DB()
        if dbErr == nil {
            _ = sqlDB.Close()
        }
    })
    return db
}

func intPtrForUsageAnalyticsTest(value int) *int { return &value }

func seedUsageAnalyticsLog(t *testing.T, log *Log) {
    t.Helper()
    require.NoError(t, LOG_DB.Create(log).Error)
}

func seedUsageAnalyticsToken(t *testing.T, token *Token) {
    t.Helper()
    require.NoError(t, DB.Create(token).Error)
}

func usageAnalyticsNow() int64 { return time.Now().Unix() }
```

- [ ] **步骤 2：编写 token 统计口径红灯测试**

添加测试：

```go
func TestUsageAnalyticsSummaryUsesMeteredTokensAndFallback(t *testing.T) {
    setupUsageAnalyticsModelTestDB(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 10, PromptTokens: 7, CompletionTokens: 3, MeteredTokens: intPtrForUsageAnalyticsTest(80), UseTime: 1})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 9, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 20, PromptTokens: 5, CompletionTokens: 6, UseTime: 2})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 8, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 30, PromptTokens: 100, CompletionTokens: 200, MeteredTokens: intPtrForUsageAnalyticsTest(0), UseTime: 3})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 7, Type: LogTypeError, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 999, PromptTokens: 999, CompletionTokens: 999, UseTime: 4})

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Metric: UsageAnalyticsMetricTotalTokens, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 4, res.Total.RequestCount)
    require.Equal(t, 3, res.Total.SuccessCount)
    require.Equal(t, 1, res.Total.ErrorCount)
    require.Equal(t, 60, res.Total.Quota)
    require.Equal(t, 91, res.Total.TotalTokens)
    require.Equal(t, 91, res.Total.MeteredTokens)
    require.Len(t, res.Groups, 1)
    require.Equal(t, 91, res.Groups[0].TotalTokens)
}
```

- [ ] **步骤 3：运行红灯测试**

运行：

```bash
go test ./model -run TestUsageAnalyticsSummaryUsesMeteredTokensAndFallback -count=1
```

预期：FAIL，失败原因是 `UsageAnalyticsQuery`、`GetUsageAnalyticsSummary` 等未定义。

- [ ] **步骤 4：创建 DTO 与 model 类型最小实现**

`dto/usage_analytics.go` 定义常量与响应结构；`model/usage_analytics.go` 使用类型别名导出给测试和 controller。

关键类型必须包含：

```go
type UsageAnalyticsGroupBy string

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
    RequestCount    int     `json:"request_count"`
    SuccessCount    int     `json:"success_count"`
    ErrorCount      int     `json:"error_count"`
    SuccessRate     float64 `json:"success_rate"`
    ErrorRate       float64 `json:"error_rate"`
    Quota           int     `json:"quota"`
    PromptTokens    int     `json:"prompt_tokens"`
    CompletionTokens int    `json:"completion_tokens"`
    MeteredTokens   int     `json:"metered_tokens"`
    TotalTokens     int     `json:"total_tokens"`
    AvgLatencyMs    int     `json:"avg_latency_ms"`
    P95LatencyMs    int     `json:"p95_latency_ms"`
    FirstUsedAt     int64   `json:"first_used_at"`
    LastUsedAt      int64   `json:"last_used_at"`
}
```

- [ ] **步骤 5：实现 token 口径最小逻辑**

实现 `GetUsageAnalyticsSummary(query UsageAnalyticsQuery) (UsageAnalyticsSummaryResponse, error)`，读取候选日志后在 Go 层聚合。必须满足：

```go
func usageAnalyticsLogTokens(log Log) int {
    if log.Type != LogTypeConsume {
        return 0
    }
    value := 0
    if log.MeteredTokens != nil {
        value = *log.MeteredTokens
    } else {
        value = log.PromptTokens + log.CompletionTokens
    }
    if value < 0 {
        return 0
    }
    return value
}
```

错误日志 quota / tokens 必须按 0 计入；请求数和延迟样本仍包含错误日志。

- [ ] **步骤 6：运行 token 口径绿灯测试**

运行：

```bash
go test ./model -run TestUsageAnalyticsSummaryUsesMeteredTokensAndFallback -count=1
```

预期：PASS。

- [ ] **步骤 7：补充分组与安全测试**

继续在 `model/usage_analytics_test.go` 添加并先运行红灯：

```go
func TestUsageAnalyticsGroupsTokenByIDNotName(t *testing.T) { /* 同一 token_id 写入两个 token_name，断言只返回一个组 */ }
func TestUsageAnalyticsDeletedTokenDoesNotReturnMaskedKey(t *testing.T) { /* 删除 token 后断言 deleted=true, masked_key nil */ }
func TestUsageAnalyticsFiltersRejectForeignToken(t *testing.T) { /* 当前用户没有该 token 且无历史日志时返回 ErrUsageAnalyticsInvalidToken */ }
func TestUsageAnalyticsGroupsStatusStreamModelAndGroup(t *testing.T) { /* 覆盖 status、stream、model、group 维度 */ }
```

每个测试先单独运行确认 FAIL，再实现通过。

- [ ] **步骤 8：实现白名单 group expression 与 token 补充信息**

`model/usage_analytics.go` 中实现白名单函数，不把前端传入字符串拼进 SQL：

```go
func usageAnalyticsGroupExpr(groupBy UsageAnalyticsGroupBy) (string, bool) {
    switch groupBy {
    case UsageAnalyticsGroupByToken:
        return "token_id", true
    case UsageAnalyticsGroupByModel:
        return "model_name", true
    case UsageAnalyticsGroupByGroup:
        return logGroupCol, true
    case UsageAnalyticsGroupByStream:
        return "is_stream", true
    case UsageAnalyticsGroupByStatus:
        return "type", true
    default:
        return "", false
    }
}
```

Token 补充信息规则：当前 token 存在时用 `tokens.name` 和 `GetMaskedKey()`；当前 token 不存在或软删除时只用历史 `logs.token_name`，`deleted=true`，`masked_key=nil`。

- [ ] **步骤 9：补充时间序列、P95、Top N 测试**

添加并先运行红灯：

```go
func TestUsageAnalyticsTimeseriesBucketsInGo(t *testing.T) { /* day/hour bucket = start + ((created-start)/step)*step */ }
func TestUsageAnalyticsP95UsesCeilAlgorithm(t *testing.T) { /* use_time 秒转换 ms，ceil(0.95*n)-1 */ }
func TestUsageAnalyticsBreakdownMergesOther(t *testing.T) { /* limit 外合并 Other，drilldown nil */ }
func TestUsageAnalyticsRPMAndTPMUseRecentMinute(t *testing.T) { /* RPM 含错误，TPM 只含消费 total_tokens */ }
func TestUsageAnalyticsCandidateLimit(t *testing.T) { /* 超过 50000 返回明确错误 */ }
```

- [ ] **步骤 10：实现 timeseries 与 breakdown**

实现：

```go
func GetUsageAnalyticsTimeseries(query UsageAnalyticsQuery) (UsageAnalyticsTimeseriesResponse, error)
func GetUsageAnalyticsBreakdown(query UsageAnalyticsQuery) (UsageAnalyticsBreakdownResponse, error)
```

实现要求：

- `granularity=hour` step 为 3600，`day` step 为 86400。
- P95 在 Go 层用排序后的 `ceil(0.95*n)-1`。
- `share` 只对 `request_count`、`total_tokens`、`quota` 返回数值，rate/latency 返回 `nil`。
- `Other` 重新汇总 request/success/error/quota/tokens/latency 样本，不平均组结果。

- [ ] **步骤 11：补充 SQL 兼容测试**

添加测试断言生成查询 SQL 不包含数据库专属片段：

```go
func TestUsageAnalyticsSQLAvoidsDatabaseSpecificFunctions(t *testing.T) {
    sql := buildUsageAnalyticsDryRunSQLForTest(t, UsageAnalyticsGroupByGroup)
    forbidden := []string{"DATE_TRUNC", "FROM_UNIXTIME", "strftime", "PERCENTILE_CONT", " OVER ", "->", "JSON_EXTRACT", "GROUP_CONCAT"}
    for _, fragment := range forbidden {
        require.NotContains(t, strings.ToUpper(sql), strings.ToUpper(fragment))
    }
    require.NotContains(t, sql, " group ")
}
```

- [ ] **步骤 12：运行 model 完整验证并提交任务 1**

运行：

```bash
go test ./model -run 'UsageAnalytics' -count=1
```

预期：PASS。

提交仅包含任务 1 文件：

```bash
git add dto/usage_analytics.go model/usage_analytics.go model/usage_analytics_test.go
git commit -m "feat(usage-analytics): 新增用户用量聚合模型"
```

---

### 任务 2：后端 controller、router 与 Usage Logs 钻取

**文件：**
- 创建：`controller/usage_analytics.go`
- 创建：`controller/usage_analytics_test.go`
- 创建：`controller/log_usage_analytics_test.go`
- 修改：`router/api-router.go`
- 修改：`controller/log.go`
- 修改：`model/log.go`

**依赖：** 任务 1 完成后执行。

- [ ] **步骤 1：编写 controller 参数红灯测试**

在 `controller/usage_analytics_test.go` 中创建 Gin 测试 router，直接设置 `c.Set("id", userID)` 模拟 `UserAuth` 后的上下文，覆盖：

```go
func TestUsageAnalyticsSummaryDefaultsToRecentSevenDays(t *testing.T) { /* 空 query 返回 200，group_by 默认 token */ }
func TestUsageAnalyticsRejectsPartialTimeRange(t *testing.T) { /* 只传 start_timestamp 返回 400 */ }
func TestUsageAnalyticsRejectsUnsupportedPhaseOneParams(t *testing.T) { /* group_by=endpoint 或 billing_source 返回 400 */ }
func TestUsageAnalyticsIgnoresUserIDAndUsernameQuery(t *testing.T) { /* query user_id/username 不影响当前用户结果 */ }
func TestUsageAnalyticsRejectsForeignTokenID(t *testing.T) { /* 外部 token_id 返回 400 */ }
```

- [ ] **步骤 2：运行 controller 红灯测试**

运行：

```bash
go test ./controller -run TestUsageAnalytics -count=1
```

预期：FAIL，失败原因是 handler 或 route helper 未定义。

- [ ] **步骤 3：实现 controller query parser**

`controller/usage_analytics.go` 实现：

```go
func GetUsageAnalyticsSummary(c *gin.Context)
func GetUsageAnalyticsTimeseries(c *gin.Context)
func GetUsageAnalyticsBreakdown(c *gin.Context)
func parseUsageAnalyticsQuery(c *gin.Context) (model.UsageAnalyticsQuery, error)
```

解析规则：

- `c.GetInt("id")` 是唯一用户来源。
- start/end 同时省略时默认最近 7 天；只传一个返回 400。
- 最大跨度 31 天。
- `group_by` 默认 `token`，只接受 Phase 1 五个值。
- `metric` 默认 `total_tokens`，只接受 Phase 1 六个值。
- repeated params 优先；没有 repeated params 时兼容 comma fallback。
- `token_ids` 解析为去重排序正整数。
- `streams` 只接受 `true` / `false`。
- `statuses` 只接受 `success` / `error`。
- 出现 `endpoint`、`billing_source`、`billing_tier`、`modality` 参数返回 400，消息包含 `unsupported filter in current phase`。

- [ ] **步骤 4：注册 usage analytics route**

在 `router/api-router.go` 新增：

```go
usageAnalyticsRoute := apiRouter.Group("/usage-analytics")
usageAnalyticsRoute.Use(middleware.UserAuth())
{
    usageAnalyticsRoute.GET("/summary", controller.GetUsageAnalyticsSummary)
    usageAnalyticsRoute.GET("/timeseries", controller.GetUsageAnalyticsTimeseries)
    usageAnalyticsRoute.GET("/breakdown", controller.GetUsageAnalyticsBreakdown)
}
```

位置放在用户侧业务路由附近，不能使用 admin/root auth。

- [ ] **步骤 5：运行 controller 绿灯测试**

运行：

```bash
go test ./controller -run TestUsageAnalytics -count=1
```

预期：PASS。

- [ ] **步骤 6：编写 Usage Logs drilldown 红灯测试**

在 `controller/log_usage_analytics_test.go` 添加：

```go
func TestSelfLogsFiltersByTokenIDStreamAndStatus(t *testing.T) { /* /api/log/self 与 /api/log/self/stat 一致 */ }
func TestSelfLogsStatusConflictsWithType(t *testing.T) { /* status=success&type=LogTypeError 返回 message=status conflicts with type */ }
func TestSelfLogsStatUsesUserIDInsteadOfUsername(t *testing.T) { /* username 相同或 query username 不影响 self stat */ }
```

- [ ] **步骤 7：运行 Usage Logs 红灯测试**

运行：

```bash
go test ./controller -run 'TestSelfLogs' -count=1
```

预期：FAIL，新增参数未生效或冲突未处理。

- [ ] **步骤 8：增强 `model/log.go` 过滤结构**

扩展现有日志查询参数结构，增加：

```go
TokenId *int
IsStream *bool
Status string
SelfUserId *int
```

同一过滤构造函数必须同时供 list 和 stat 使用。规则：

- `TokenId` 使用 `token_id = ?`。
- `IsStream` 使用 `is_stream = ?`。
- `Status=success` 映射 `type = LogTypeConsume`，`error` 映射 `type = LogTypeError`。
- self stat 使用 `user_id = ?`，不是 username。
- `group` 条件继续使用 `logGroupCol`。

- [ ] **步骤 9：增强 `controller/log.go` query 解析**

新增解析：

```go
func parseLogStatusType(c *gin.Context) (int, bool, error)
```

要求：

- `status` 为空时保留现有 `type` 行为。
- `status=success` 与 `type=LogTypeConsume` 等价时允许。
- `status=error` 与 `type=LogTypeError` 等价时允许。
- 冲突返回 HTTP 400，JSON `message` 为 `status conflicts with type`。
- `token_id` 与 `is_stream` 同时传给 list 和 stat。

- [ ] **步骤 10：运行后端 controller 定向测试并提交任务 2**

运行：

```bash
go test ./controller -run 'TestUsageAnalytics|TestSelfLogs' -count=1
```

预期：PASS。

提交：

```bash
git add controller/usage_analytics.go controller/usage_analytics_test.go controller/log_usage_analytics_test.go router/api-router.go controller/log.go model/log.go
git commit -m "feat(usage-analytics): 接入用户用量分析接口"
```

---

### 任务 3：前端 API、类型、纯函数与测试

**文件：**
- 创建：`web/default/src/features/usage-analytics/types.ts`
- 创建：`web/default/src/features/usage-analytics/constants.ts`
- 创建：`web/default/src/features/usage-analytics/api.ts`
- 创建：`web/default/src/features/usage-analytics/lib/filters.ts`
- 创建：`web/default/src/features/usage-analytics/lib/filters.test.ts`
- 创建：`web/default/src/features/usage-analytics/lib/format.ts`
- 创建：`web/default/src/features/usage-analytics/lib/format.test.ts`
- 创建：`web/default/src/features/usage-analytics/lib/chart-data.ts`
- 创建：`web/default/src/features/usage-analytics/lib/chart-data.test.ts`

**不修改：** 路由、sidebar、API Keys、Usage Logs、i18n locale 文件。

- [ ] **步骤 1：编写 filters 红灯测试**

`filters.test.ts` 使用 `node:test` 与 `node:assert/strict`：

```ts
import test from 'node:test'
import assert from 'node:assert/strict'
import { normalizeUsageAnalyticsSearch, buildUsageAnalyticsApiParams, buildUsageLogsDrilldownSearch } from './filters'

test('normalizes empty search to recent seven days token analytics', () => {
  const now = 1779321600
  const normalized = normalizeUsageAnalyticsSearch({}, now)
  assert.equal(normalized.group_by, 'token')
  assert.equal(normalized.metric, 'total_tokens')
  assert.equal(normalized.granularity, 'day')
  assert.equal(normalized.limit, 10)
  assert.equal(normalized.end_timestamp, now)
  assert.equal(normalized.start_timestamp, now - 7 * 24 * 60 * 60)
})

test('normalizes repeated and comma fallback values without irreversible comma join', () => {
  const normalized = normalizeUsageAnalyticsSearch({ model_names: ['gpt-4', 'gpt-4', 'claude'], groups: ['a,b', 'default'] }, 1779321600)
  assert.deepEqual(normalized.model_names, ['claude', 'gpt-4'])
  assert.deepEqual(normalized.groups, ['a,b', 'default'])
})

test('builds usage logs drilldown search with status not numeric type', () => {
  const search = buildUsageLogsDrilldownSearch({ start_timestamp: 10, end_timestamp: 20 }, { token_id: 5, status: 'error', is_stream: true })
  assert.deepEqual(search, { startTime: 10000, endTime: 20000, tokenId: 5, status: 'error', isStream: true })
  assert.equal(Object.prototype.hasOwnProperty.call(search, 'type'), false)
})
```

- [ ] **步骤 2：运行 filters 红灯测试**

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/filters.test.ts
```

预期：FAIL，模块未定义。

- [ ] **步骤 3：实现类型、常量、filters 和 API params**

`types.ts` 定义与后端一致的 union：

```ts
export type UsageAnalyticsGroupBy = 'token' | 'model' | 'group' | 'stream' | 'status'
export type UsageAnalyticsMetric = 'request_count' | 'total_tokens' | 'quota' | 'error_rate' | 'avg_latency_ms' | 'p95_latency_ms'
export type UsageAnalyticsGranularity = 'hour' | 'day'
export type UsageAnalyticsStatus = 'success' | 'error'
```

`buildUsageAnalyticsApiParams` 返回 `URLSearchParams` 或可由 `api.ts` 序列化的 tuple，数组使用 `append` repeated params，不用 comma join。

- [ ] **步骤 4：运行 filters 绿灯测试**

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/filters.test.ts
```

预期：PASS。

- [ ] **步骤 5：编写 format 红灯测试**

`format.test.ts`：

```ts
import test from 'node:test'
import assert from 'node:assert/strict'
import { formatLatencyMs, formatUsagePercent, formatUsageTokens } from './format'

test('formats latency without hiding zero', () => {
  assert.equal(formatLatencyMs(0), '0 ms')
  assert.equal(formatLatencyMs(999), '999 ms')
  assert.equal(formatLatencyMs(1500), '1.5 s')
})

test('formats percent with zero request semantics', () => {
  assert.equal(formatUsagePercent(0), '0%')
  assert.equal(formatUsagePercent(0.0156), '1.6%')
})

test('formats tokens preserving zero', () => {
  assert.equal(formatUsageTokens(0), '0')
  assert.equal(formatUsageTokens(1220000), '1.22M')
})
```

- [ ] **步骤 6：运行 format 红灯并实现绿灯**

运行红灯：

```bash
cd web/default && bun test src/features/usage-analytics/lib/format.test.ts
```

实现 `format.ts` 后再次运行，预期 PASS。

- [ ] **步骤 7：编写 chart-data 红灯测试**

`chart-data.test.ts` 覆盖：

```ts
import test from 'node:test'
import assert from 'node:assert/strict'
import { buildTrendChartData, isAdditiveMetric, mergeTopNWithOther } from './chart-data'

test('keeps series separated by group_key even when labels match', () => { /* 两个同 label 不同 group_key 均保留 */ })
test('merges extra additive groups into non-drillable Other', () => { /* Other.drilldown === null */ })
test('does not stack rate or latency metrics', () => { /* isAdditiveMetric('error_rate') === false */ })
test('returns share only for additive metrics', () => { /* rate metric share 为 null 或不生成占比 */ })
```

- [ ] **步骤 8：实现 chart-data 并运行前端纯函数测试**

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/format.test.ts src/features/usage-analytics/lib/chart-data.test.ts src/features/usage-analytics/lib/filters.test.ts
```

预期：PASS。

- [ ] **步骤 9：实现 `api.ts` 并提交任务 3**

`api.ts` 使用项目统一 `api` 实例：

```ts
export async function getUsageAnalyticsSummary(filters: UsageAnalyticsCanonicalFilters): Promise<ApiResponse<UsageAnalyticsSummaryResponse>> {
  const params = buildUsageAnalyticsApiParams(filters)
  const res = await api.get(`/api/usage-analytics/summary?${params.toString()}`)
  return res.data
}
```

提交：

```bash
git add web/default/src/features/usage-analytics
git commit -m "feat(usage-analytics): 新增前端分析数据工具"
```

---

### 任务 4：前端页面与图表组件

**文件：**
- 创建：`web/default/src/features/usage-analytics/index.tsx`
- 创建：`web/default/src/features/usage-analytics/components/usage-analytics-filter-bar.tsx`
- 创建：`web/default/src/features/usage-analytics/components/usage-analytics-summary-cards.tsx`
- 创建：`web/default/src/features/usage-analytics/components/usage-trend-chart.tsx`
- 创建：`web/default/src/features/usage-analytics/components/usage-breakdown-chart.tsx`
- 创建：`web/default/src/features/usage-analytics/components/usage-ranking-table.tsx`

**依赖：** 任务 3 完成后执行。

- [ ] **步骤 1：编写组件数据约束红灯测试或复用纯函数测试**

组件不引入 jsdom。先扩展 `chart-data.test.ts`，加入页面组件依赖的数据 shape 测试：

```ts
test('builds ranking rows with deleted token safe fields', () => { /* masked_key null 不崩溃，label 为 Deleted API Key fallback */ })
test('builds empty states for unsupported share metrics', () => { /* error_rate 分布图返回 empty reason */ })
```

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/chart-data.test.ts
```

预期：FAIL。

- [ ] **步骤 2：实现数据 helper 绿灯**

补充 `chart-data.ts`，再次运行同一命令，预期 PASS。

- [ ] **步骤 3：实现页面容器**

`index.tsx` 必须：

- 使用 `SectionPageLayout`。
- 使用 `route.useSearch()` 和 `useNavigate()`，不读取 `window.location`。
- 用 `buildUsageAnalyticsCanonicalFilters` 构建 query key。
- 三个 query key 分别为 `['usage-analytics','summary',canonicalFilters]`、`['usage-analytics','timeseries',canonicalFilters]`、`['usage-analytics','breakdown',canonicalFilters]`。
- 页面说明文案在内容区显式渲染，不依赖 `SectionPageLayout.Description`。

- [ ] **步骤 4：实现筛选栏**

`usage-analytics-filter-bar.tsx` 必须：

- 使用 draft state。
- Apply 后一次性写 URL search。
- Reset 写入默认最近 7 天、`group_by=token`、`metric=total_tokens`、`limit=10`。
- 多选数组保持类型化数组，不用 comma string。

- [ ] **步骤 5：实现总览卡片、趋势图、分布图、排行表**

要求：

- 卡片展示请求数、Tokens、额度、成功率、错误率、平均延迟、P95 延迟、活跃 API Key、RPM、TPM。
- 趋势图 additive metric 使用堆叠面积，rate/latency 使用非堆叠线图。
- 分布图 rate/latency 显示「该指标不支持占比图」类空态，不画环图。
- 排行表 `View Logs` 使用 `buildUsageLogsDrilldownSearch` 跳转 `/usage-logs/common`。
- `Other` 禁用钻取按钮。

- [ ] **步骤 6：运行前端纯函数测试和类型检查片段**

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/chart-data.test.ts
cd web/default && bun run typecheck
```

预期：PASS。

- [ ] **步骤 7：提交任务 4**

```bash
git add web/default/src/features/usage-analytics
git commit -m "feat(usage-analytics): 新增用量分析页面组件"
```

---

### 任务 5：前端路由、入口、Usage Logs 钻取与 i18n

**文件：**
- 创建：`web/default/src/routes/_authenticated/usage-analytics/index.tsx`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 修改：`web/default/src/features/keys/components/api-keys-primary-buttons.tsx`
- 修改：`web/default/src/features/keys/components/data-table-row-actions.tsx`
- 修改：`web/default/src/routes/_authenticated/usage-logs/$section.tsx`
- 修改：`web/default/src/features/usage-logs/types.ts`
- 修改：`web/default/src/features/usage-logs/lib/utils.ts`
- 修改：`web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx`
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`
- 生成：`web/default/src/routeTree.gen.ts`

**依赖：** 任务 3 完成后执行；任务 4 完成后最终 typecheck。

- [ ] **步骤 1：扩展 filters 测试覆盖 API Keys 入口与 Usage Logs 映射**

在 `filters.test.ts` 添加：

```ts
test('builds api key entry search without full key material', () => { /* token_ids 仅包含 id */ })
test('maps usage logs search to api params with status string', () => { /* status=success 不产生 type */ })
```

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/filters.test.ts
```

预期：FAIL。

- [ ] **步骤 2：实现 route search schema**

新增 `web/default/src/routes/_authenticated/usage-analytics/index.tsx`：

```ts
import { createFileRoute } from '@tanstack/react-router'
import { UsageAnalyticsPage } from '@/features/usage-analytics'
import { usageAnalyticsSearchSchema } from '@/features/usage-analytics/lib/filters'

export const Route = createFileRoute('/_authenticated/usage-analytics/')({
  validateSearch: usageAnalyticsSearchSchema,
  component: UsageAnalyticsPage,
})
```

`usageAnalyticsSearchSchema` 必须将单值、数组、comma fallback 归一化为数组，并限制 Phase 1 union。

- [ ] **步骤 3：实现 sidebar 与 API Keys 入口**

- `use-sidebar-data.ts`：`Usage Analytics` 放在 `API Keys` 与 `Usage Logs` 之间，图标从 `lucide-react` 实际导出中选择已存在图标。
- `use-sidebar-config.ts`：`URL_TO_CONFIG_MAP['/usage-analytics'] = { section: 'console', module: 'log' }`。
- `api-keys-primary-buttons.tsx`：新增按钮跳转 `/usage-analytics`。
- `data-table-row-actions.tsx`：新增 `Analyze this Key`，跳转 `{ group_by: 'token', token_ids: [apiKey.id] }`，不读取完整 key。

- [ ] **步骤 4：实现 Usage Logs 前端 drilldown search**

- `$section.tsx` search schema 新增 `tokenId?: number`、`isStream?: boolean`、`status?: 'success' | 'error'`。
- `types.ts` 同步新增字段。
- `utils.ts` 将 `tokenId` → `token_id`、`isStream` → `is_stream`、`status` → `status`。
- `common-logs-filter-bar.tsx` 从 search 初始化这三个字段，Reset 清空它们，Apply 保留它们；前端不把 `status` 改写为 numeric `type`。

- [ ] **步骤 5：运行 Usage Logs / filters 测试绿灯**

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/filters.test.ts
```

预期：PASS。

- [ ] **步骤 6：添加 i18n 文案**

在 `static-keys.ts` 和 6 个 locale 文件加入规格列出的文案，至少包含：

```text
Usage Analytics
Analyze your API usage across keys, models, groups, and request outcomes
View Usage Analytics
Analyze this Key
Group by
Metric
Apply filters
Reset filters
Time range
Granularity
Top N
API Key Usage
Model Usage
Group Usage
Streaming
Non-streaming
Success Rate
Error Rate
Average Latency
P95 Latency
Active API Keys
Usage Trend
Usage Breakdown
Usage Ranking
View Logs
View logs for this item
This item cannot be drilled down
No usage data
No matching usage data
Try adjusting the time range or filters
Deleted API Key
Unknown Model
Unknown Group
Other
Retry
Failed to load usage analytics
```

翻译必须覆盖 `en`、`zh`、`fr`、`ru`、`ja`、`vi`，保留占位符和英文技术词。

- [ ] **步骤 7：同步 i18n 并生成 routeTree**

运行：

```bash
cd web/default && bun run i18n:sync
cd web/default && bun run build
```

确认 `web/default/src/routeTree.gen.ts` 包含 `/usage-analytics`。如果 build 暴露 unrelated 既有错误，记录完整错误并先修复由本任务引入的错误。

- [ ] **步骤 8：提交任务 5**

```bash
git add web/default/src/routes/_authenticated/usage-analytics/index.tsx web/default/src/hooks/use-sidebar-data.ts web/default/src/hooks/use-sidebar-config.ts web/default/src/features/keys/components/api-keys-primary-buttons.tsx web/default/src/features/keys/components/data-table-row-actions.tsx web/default/src/routes/_authenticated/usage-logs/\$section.tsx web/default/src/features/usage-logs/types.ts web/default/src/features/usage-logs/lib/utils.ts web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx web/default/src/i18n/static-keys.ts web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/vi.json web/default/src/routeTree.gen.ts
git commit -m "feat(usage-analytics): 接入前端路由与入口"
```

---

### 任务 6：最终整合、审查与验证

**文件：** 只修改任务 1–5 引入或触碰的文件；不得提交与本功能无关的既有 rankings 改动。

- [ ] **步骤 1：检查工作区变更边界**

运行：

```bash
git status --short
```

确认只处理 Usage Analytics 相关文件。若看到既有 unrelated 改动（例如 rankings 文件），不要 revert、不要 stash、不要加入 commit。

- [ ] **步骤 2：运行后端定向测试**

运行：

```bash
go test ./model ./controller
```

预期：PASS。

- [ ] **步骤 3：运行前端纯函数测试**

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/format.test.ts src/features/usage-analytics/lib/chart-data.test.ts src/features/usage-analytics/lib/filters.test.ts
```

预期：PASS。

- [ ] **步骤 4：运行 i18n 同步与类型检查**

运行：

```bash
cd web/default && bun run i18n:sync
cd web/default && bun run typecheck
```

预期：PASS，且 `_sync-report.json` 中本次新增 key 没有缺失翻译。

- [ ] **步骤 5：运行 build 生成 routeTree 并确认路由**

运行：

```bash
cd web/default && bun run build
```

随后使用 `search` 工具检查 `web/default/src/routeTree.gen.ts` 包含 `/usage-analytics`。

- [ ] **步骤 6：整体代码审查**

派发 3 个只读审查子代理并发审查：后端正确性、前端正确性、执行与安全边界。审查上下文必须包含本计划、规格文件路径、`git diff` 范围、验证命令输出。

- [ ] **步骤 7：修复审查反馈并复验**

所有 Critical / Important 问题必须修复并复审通过；Minor 问题若不修复，必须有明确技术理由。

- [ ] **步骤 8：最终提交**

如果任务 1–5 子代理已经分步提交，最终只提交整合修复；如果尚未提交，则分后端、前端、整合三类提交，避免把 unrelated 文件加入 commit。

推荐提交信息：

```bash
git commit -m "feat(usage-analytics): 完成用户侧用量分析中心"
```

## 最终验收标准

- 空 URL `/usage-analytics` 默认请求最近 7 天、`group_by=token`、`metric=total_tokens`。
- 用户能切换 token、model、group、stream、status 维度。
- Summary、timeseries、breakdown 使用同一 token 口径：`metered_tokens IS NOT NULL` 优先，显式 0 保留，错误日志 token/quota 为 0。
- P95 和 bucket 在 Go 层计算，不使用数据库专属函数。
- Token 聚合只按 `token_id`，不按 `token_name`；deleted token 不返回完整 key 或掩码。
- `/api/log/self` 与 `/api/log/self/stat` 支持 `token_id`、`is_stream`、`status`，且 `status` 冲突返回 `status conflicts with type`。
- 前端 Usage Analytics 钻取到 `/usage-logs/common`，携带 `tokenId`、`model`、`group`、`isStream`、`status`、时间范围；API 请求发送 `status=success|error` 而不是 numeric `type`。
- API Keys 顶部和行操作均可跳转到分析页，不请求、不暴露完整 API Key。
- i18n 覆盖 6 个 locale；`routeTree.gen.ts` 由 build 生成并包含 `/usage-analytics`。
- 验证命令全部通过：`go test ./model ./controller`、前端 3 个 `bun test` 文件、`bun run i18n:sync`、`bun run typecheck`、`bun run build`。
