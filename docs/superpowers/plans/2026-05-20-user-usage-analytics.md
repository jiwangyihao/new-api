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
- `model/usage_analytics_test.go`：model 层统计口径、分组、Top N、P95、bucket、token 安全、分离 `DB` / `LOG_DB` 和 SQL 兼容测试。
- `controller/usage_analytics.go`：HTTP query 解析、默认值、错误响应、当前用户注入。
- `controller/usage_analytics_test.go`：controller 层参数、鉴权上下文、用户隔离、外部 token、unsupported 参数测试。

### 后端修改文件

- `router/api-router.go`：注册 `/api/usage-analytics` 用户侧路由。
- `model/log.go`：增强用户日志列表与统计过滤，支持 `token_id`、`is_stream`、`status`；self stat 使用 `user_id` 过滤。
- `controller/log.go`：解析新增 Usage Logs drilldown 参数，处理 `status` 与 `type` 冲突。
- `controller/log_usage_analytics_test.go`：覆盖 `/api/log/self` 与 `/api/log/self/stat` 的 drilldown 行为。
- `controller/log_stat_token_test.go`：更新现有 self stat 测试，显式设置当前用户 ID，防止 username 口径回归。

### 前端新增文件

- `web/default/src/features/usage-analytics/types.ts`：后端 DTO 对应类型，Phase 1 union 仅包含 `token | model | group | stream | status`。
- `web/default/src/features/usage-analytics/constants.ts`：group、metric、granularity、sort 选项，常量只存 `labelKey`。
- `web/default/src/features/usage-analytics/api.ts`：summary / timeseries / breakdown 请求函数，数组使用 repeated query params。
- `web/default/src/features/usage-analytics/lib/filters.ts`：URL search schema、归一化、canonical filters、API params、API Keys 入口 search、Usage Logs drilldown search。
- `web/default/src/features/usage-analytics/lib/filters.test.ts`：筛选纯函数测试。
- `web/default/src/features/usage-analytics/lib/format.ts`：延迟、百分比、tokens、quota wrapper 格式化。
- `web/default/src/features/usage-analytics/lib/format.test.ts`：格式化测试。
- `web/default/src/features/usage-analytics/lib/chart-data.ts`：Top N / Other、additive 与 rate/latency chart spec 数据转换。
- `web/default/src/features/usage-analytics/lib/chart-data.test.ts`：图表纯函数测试。
- `web/default/src/features/usage-analytics/lib/page-contract.ts`：页面 query key、默认筛选和排行钻取纯函数。
- `web/default/src/features/usage-analytics/lib/page-contract.test.ts`：页面合同纯函数测试。
- `web/default/src/features/usage-analytics/index.tsx`：页面容器，接收 route search 和 navigate 回调 props，整合 React Query 与布局。
- `web/default/src/features/usage-analytics/components/usage-analytics-filter-bar.tsx`：筛选栏，draft state + Apply。
- `web/default/src/features/usage-analytics/components/usage-analytics-summary-cards.tsx`：总览卡片。
- `web/default/src/features/usage-analytics/components/usage-trend-chart.tsx`：趋势图。
- `web/default/src/features/usage-analytics/components/usage-breakdown-chart.tsx`：分布图。
- `web/default/src/features/usage-analytics/components/usage-ranking-table.tsx`：排行表与钻取按钮。
- `web/default/src/routes/_authenticated/usage-analytics/index.tsx`：TanStack Router file route，读取 `Route.useSearch()` 后传入页面组件。

### 前端修改文件

- `web/default/src/hooks/use-sidebar-data.ts`：在 General 分组中新增 `Usage Analytics`，位置在 `API Keys` 与 `Usage Logs` 之间。
- `web/default/src/hooks/use-sidebar-config.ts`：`URL_TO_CONFIG_MAP` 新增 `/usage-analytics`，复用 `{ section: 'console', module: 'log' }`。
- `web/default/src/features/keys/components/api-keys-primary-buttons.tsx`：新增 `View Usage Analytics` 按钮。
- `web/default/src/features/keys/components/data-table-row-actions.tsx`：新增 `Analyze this Key` 行操作，只使用 `apiKey.id`。
- `web/default/src/routes/_authenticated/usage-logs/$section.tsx`：search schema 新增 `tokenId`、`isStream`、`status`。
- `web/default/src/features/usage-logs/types.ts`：`CommonLogFilters`、`GetLogsParams`、`GetLogStatsParams` 新增 `tokenId` / `token_id`、`isStream` / `is_stream`、`status`。
- `web/default/src/features/usage-logs/lib/utils.ts`：search 到 API 参数映射新增 drilldown 参数，`status` 不转换成 `type`。
- `web/default/src/features/usage-logs/lib/filter.ts`：Usage Logs Apply / Reset 构造 URL search 时保留或清空 `tokenId`、`isStream`、`status`。
- `web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx`：读取和清空 drilldown 条件。
- `web/default/src/i18n/static-keys.ts` 与 `web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`：新增文案。
- `web/default/src/routeTree.gen.ts`：通过 `bun run build` 生成，不手写。

## 并发与串行规则

- 任务 1（后端 model/dto）和任务 3（前端 API/types/lib）可以并行。
- 任务 2 依赖任务 1；任务 2 单独负责 `router/api-router.go`、`controller/log.go`、`model/log.go`，其他子代理不得同时编辑这些文件。
- 任务 4 依赖任务 3，只实现不直接依赖 TanStack routeTree 的纯页面与组件；任务 4 不运行项目级 typecheck / build。
- 任务 5 依赖任务 4，单独创建 route 文件、sidebar、API Keys 入口、Usage Logs 前端 drilldown、i18n，并把 routeTree 生成留给任务 6。
- 任务 5 单独负责 route 文件、sidebar、API Keys、Usage Logs 前端 drilldown、i18n，其他前端子代理不得同时编辑这些共享入口文件；`routeTree.gen.ts` 由任务 6 主代理生成。
- 主代理最终运行所有验证并处理冲突，不提交工作区中与本功能无关的既有改动。

---

### 任务 1：后端 DTO 与 model 聚合

**文件：**
- 创建：`dto/usage_analytics.go`
- 创建：`model/usage_analytics.go`
- 创建：`model/usage_analytics_test.go`

**不修改：** `router/api-router.go`、`controller/log.go`、`model/log.go`、所有前端文件。

- [ ] **步骤 1：编写 model 红灯测试骨架**

创建 `model/usage_analytics_test.go`，包含两个彼此独立的 in-memory SQLite 连接。`DB` 只迁移 `Token`，`LOG_DB` 只迁移 `Log`，这样任何误写跨库 JOIN 的实现都会在测试中失败。测试必须真实写入 `Log` 和 `Token`，不 mock model 业务逻辑。

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

type usageAnalyticsModelTestDBs struct {
    DB    *gorm.DB
    LogDB *gorm.DB
}

func setupUsageAnalyticsModelTestDBs(t *testing.T) usageAnalyticsModelTestDBs {
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

    safeName := strings.ReplaceAll(t.Name(), "/", "_")
    businessDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_business?mode=memory&cache=shared"), &gorm.Config{})
    require.NoError(t, err)
    logDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_logs?mode=memory&cache=shared"), &gorm.Config{})
    require.NoError(t, err)

    DB = businessDB
    LOG_DB = logDB
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
        businessSQL, businessErr := businessDB.DB()
        if businessErr == nil {
            _ = businessSQL.Close()
        }
        logSQL, logErr := logDB.DB()
        if logErr == nil {
            _ = logSQL.Close()
        }
    })

    return usageAnalyticsModelTestDBs{DB: businessDB, LogDB: logDB}
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
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 10, PromptTokens: 7, CompletionTokens: 3, MeteredTokens: intPtrForUsageAnalyticsTest(80), UseTime: 1})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 9, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 20, PromptTokens: 5, CompletionTokens: 6, UseTime: 2})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 8, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 30, PromptTokens: 100, CompletionTokens: 200, MeteredTokens: intPtrForUsageAnalyticsTest(0), UseTime: 3})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 7, Type: LogTypeError, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 999, PromptTokens: 999, CompletionTokens: 999, UseTime: 4})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 6, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 40, PromptTokens: -5, CompletionTokens: 2, UseTime: 5})

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Metric: UsageAnalyticsMetricTotalTokens, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 5, res.Total.RequestCount)
    require.Equal(t, 4, res.Total.SuccessCount)
    require.Equal(t, 1, res.Total.ErrorCount)
    require.Equal(t, 100, res.Total.Quota)
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

错误日志 quota / tokens 必须按 0 计入；请求数和延迟样本仍包含错误日志。`usageAnalyticsLogQuota` 也必须对负数按 0 处理。

- [ ] **步骤 6：运行 token 口径绿灯测试**

运行：

```bash
go test ./model -run TestUsageAnalyticsSummaryUsesMeteredTokensAndFallback -count=1
```

预期：PASS。

- [ ] **步骤 7：补充分组、跨库与安全红灯测试**

继续在 `model/usage_analytics_test.go` 添加完整测试并先运行红灯：

```go
func TestUsageAnalyticsGroupsTokenByIDNotName(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 20, Type: LogTypeConsume, TokenId: 7, TokenName: "old-name", Quota: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 7, TokenName: "new-name", Quota: 1, MeteredTokens: intPtrForUsageAnalyticsTest(20)})

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Len(t, res.Groups, 1)
    require.Equal(t, "token:7", res.Groups[0].GroupKey)
    require.Equal(t, 30, res.Groups[0].TotalTokens)
}

func TestUsageAnalyticsTokenSupplementWorksWithSeparatedLogDB(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsToken(t, &Token{Id: 8, UserId: 101, Name: "live-key", Key: "sk-live-1234567890", Status: 1, RemainQuota: 100, Group: "default"})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 8, TokenName: "historical", Quota: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Len(t, res.Groups, 1)
    require.NotNil(t, res.Groups[0].Token)
    require.Equal(t, "live-key", res.Groups[0].Token.Name)
    require.NotNil(t, res.Groups[0].Token.MaskedKey)
}

func TestUsageAnalyticsDeletedTokenDoesNotReturnMaskedKey(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    token := &Token{Id: 9, UserId: 101, Name: "deleted-key", Key: "sk-deleted-1234567890", Status: 1}
    seedUsageAnalyticsToken(t, token)
    require.NoError(t, DB.Delete(token).Error)
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 9, TokenName: "deleted-history", Quota: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Len(t, res.Groups, 1)
    require.NotNil(t, res.Groups[0].Token)
    require.True(t, res.Groups[0].Token.Deleted)
    require.Nil(t, res.Groups[0].Token.MaskedKey)
    require.Equal(t, "deleted-history", res.Groups[0].GroupLabel)
}

func TestUsageAnalyticsFiltersRejectForeignToken(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsToken(t, &Token{Id: 10, UserId: 202, Name: "foreign", Key: "sk-foreign-1234567890"})

    _, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, TokenIDs: []int{10}, Limit: 10})
    require.ErrorIs(t, err, ErrUsageAnalyticsInvalidToken)
}

func TestUsageAnalyticsFiltersAllowDeletedTokenWithOwnHistory(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    token := &Token{Id: 14, UserId: 101, Name: "deleted-filter", Key: "sk-deleted-filter-1234567890"}
    seedUsageAnalyticsToken(t, token)
    require.NoError(t, DB.Delete(token).Error)
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 14, TokenName: "deleted-filter-history", MeteredTokens: intPtrForUsageAnalyticsTest(10)})

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, TokenIDs: []int{14}, Limit: 10})
    require.NoError(t, err)
    require.Len(t, res.Groups, 1)
    require.Equal(t, "token:14", res.Groups[0].GroupKey)
}

func TestUsageAnalyticsGroupsStatusStreamModelAndGroup(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 30, Type: LogTypeConsume, TokenId: 1, ModelName: "gpt-a", Group: "default", IsStream: true, MeteredTokens: intPtrForUsageAnalyticsTest(10), UseTime: 1})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 20, Type: LogTypeError, TokenId: 2, ModelName: "gpt-b", Group: "vip", IsStream: false, UseTime: 2})

    for _, groupBy := range []UsageAnalyticsGroupBy{UsageAnalyticsGroupByStatus, UsageAnalyticsGroupByStream, UsageAnalyticsGroupByModel, UsageAnalyticsGroupByGroup} {
        res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: groupBy, Limit: 10})
        require.NoError(t, err)
        require.Len(t, res.Groups, 2)
    }
}
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

Token 补充信息规则：当前 token 存在时用 `tokens.name` 和 `GetMaskedKey()`；当前 token 不存在或软删除时只用历史 `logs.token_name`，`deleted=true`，`masked_key=nil`。过滤 `TokenIDs` 时，未删除 token 必须属于当前用户；已删除 token 或当前不存在 token 只有在 `LOG_DB` 中存在 `logs.user_id = 当前用户 ID AND logs.token_id = ?` 历史日志时才允许。

- [ ] **步骤 9：补充时间序列、P95、Top N、统一 token 口径红灯测试**

添加并先运行红灯：

```go
func TestUsageAnalyticsTimeseriesBucketsInGo(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    start := int64(1778716800)
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 10, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3700, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(20)})

    res, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: start, EndTimestamp: start + 7200, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, start, res.Points[0].Timestamp)
    require.Equal(t, start+3600, res.Points[1].Timestamp)
}

func TestUsageAnalyticsP95UsesCeilAlgorithm(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    for i, useTime := range []int{1, 2, 3, 4, 5} {
        seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - int64(10-i), Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1), UseTime: useTime})
    }

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 5000, res.Total.P95LatencyMs)
}

func TestUsageAnalyticsBreakdownMergesOther(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    for tokenID := 1; tokenID <= 3; tokenID++ {
        seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - int64(tokenID), Type: LogTypeConsume, TokenId: tokenID, TokenName: "key", MeteredTokens: intPtrForUsageAnalyticsTest(tokenID * 10), Quota: tokenID})
    }

    res, err := GetUsageAnalyticsBreakdown(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Metric: UsageAnalyticsMetricTotalTokens, Limit: 2})
    require.NoError(t, err)
    require.Equal(t, 3, res.TotalGroups)
    require.Len(t, res.Groups, 2)
    require.NotNil(t, res.Other)
    require.Nil(t, res.Other.Drilldown)
    require.Equal(t, 10, res.Other.TotalTokens)
}

func TestUsageAnalyticsTimeseriesUsesGlobalTopNAndOther(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    start := int64(1778716800)
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 10, Type: LogTypeConsume, TokenId: 1, TokenName: "top", MeteredTokens: intPtrForUsageAnalyticsTest(100)})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3700, Type: LogTypeConsume, TokenId: 2, TokenName: "other-a", MeteredTokens: intPtrForUsageAnalyticsTest(20)})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3710, Type: LogTypeConsume, TokenId: 3, TokenName: "other-b", MeteredTokens: intPtrForUsageAnalyticsTest(30)})

    res, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: start, EndTimestamp: start + 7200, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Metric: UsageAnalyticsMetricTotalTokens, Limit: 1})
    require.NoError(t, err)
    keys := make(map[string]bool)
    for _, point := range res.Points {
        keys[point.GroupKey] = true
        if point.GroupKey == "other" {
            require.Nil(t, point.Drilldown)
        }
    }
    require.True(t, keys["token:1"])
    require.True(t, keys["other"])
}

func TestUsageAnalyticsTokenContractAppliesToTimeseriesBreakdownAndTPM(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(0), PromptTokens: 100, CompletionTokens: 100})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 9, Type: LogTypeConsume, TokenId: 1, PromptTokens: 5, CompletionTokens: 6})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 8, Type: LogTypeError, TokenId: 1, PromptTokens: 999, CompletionTokens: 999})

    summary, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 11, summary.Total.TotalTokens)
    require.Equal(t, 11, summary.Total.Tpm)

    timeseries, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 11, timeseries.Points[0].TotalTokens)

    breakdown, err := GetUsageAnalyticsBreakdown(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 11, breakdown.Groups[0].TotalTokens)
}

func TestUsageAnalyticsActiveKeyCountIncludesDeletedHistory(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1)})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 9, Type: LogTypeError, TokenId: 2})

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 2, res.Total.ActiveKeyCount)
}

func TestUsageAnalyticsRPMAndTPMUseRecentMinute(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 1, ModelName: "gpt-a", MeteredTokens: intPtrForUsageAnalyticsTest(10)})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 9, Type: LogTypeError, TokenId: 1, ModelName: "gpt-a"})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 70, Type: LogTypeConsume, TokenId: 1, ModelName: "gpt-a", MeteredTokens: intPtrForUsageAnalyticsTest(999)})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 8, Type: LogTypeConsume, TokenId: 2, ModelName: "gpt-b", MeteredTokens: intPtrForUsageAnalyticsTest(999)})

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 3600, EndTimestamp: now - 120, GroupBy: UsageAnalyticsGroupByToken, ModelNames: []string{"gpt-a"}, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 2, res.Total.Rpm)
    require.Equal(t, 10, res.Total.Tpm)
}

func TestUsageAnalyticsTimeseriesP95UsesBucketSamples(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    start := int64(1778716800)
    for _, useTime := range []int{1, 2, 3} {
        seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + int64(useTime), Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1), UseTime: useTime})
    }
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3601, Type: LogTypeError, TokenId: 1, UseTime: 7})
    seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3602, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1), UseTime: 8})

    res, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: start, EndTimestamp: start + 7200, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 3000, res.Points[0].P95LatencyMs)
    require.Equal(t, 8000, res.Points[1].P95LatencyMs)
}

func TestUsageAnalyticsCandidateLimit(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    for i := 0; i < 50001; i++ {
        seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - int64(i%60), Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1)})
    }

    _, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.ErrorIs(t, err, ErrUsageAnalyticsTooManyLogs)
}
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
- timeseries 先按整个筛选窗口确定 Top N，再对每个 bucket 复用同一组 Top N；其余真实组在各 bucket 内合并为稳定 `Other`，`drilldown=nil`。

- [ ] **步骤 11：补充 SQL 兼容与 quota_data 隔离测试**

添加 table-driven DryRun 测试，分别用 sqlite / mysql / postgres 方言生成基础过滤和 `group_by=group` SQL，断言不包含数据库专属片段：

```go
func TestUsageAnalyticsSQLAvoidsDatabaseSpecificFunctions(t *testing.T) {
    for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
        t.Run(dialect, func(t *testing.T) {
            sql := buildUsageAnalyticsDryRunSQLForTest(t, dialect, UsageAnalyticsGroupByGroup)
            upperSQL := strings.ToUpper(sql)
            forbidden := []string{"DATE_TRUNC", "FROM_UNIXTIME", "STRFTIME", "PERCENTILE_CONT", " OVER ", "->", "JSON_EXTRACT", "GROUP_CONCAT", "IFNULL"}
            for _, fragment := range forbidden {
                require.NotContains(t, upperSQL, strings.ToUpper(fragment))
            }
            requireNoBareGroupColumnForUsageAnalyticsTest(t, sql, dialect)
        })
    }
}

func requireNoBareGroupColumnForUsageAnalyticsTest(t *testing.T, sql string, dialect string) {
    t.Helper()
    lowerSQL := strings.ToLower(sql)
    require.NotContains(t, lowerSQL, "select group,")
    require.NotContains(t, lowerSQL, "where group =")
    require.NotContains(t, lowerSQL, "group by group")
    if dialect == "postgres" {
        require.Contains(t, sql, `"group"`)
    } else {
        require.Contains(t, sql, "`group`")
    }
}

func TestUsageAnalyticsDoesNotReadQuotaDataForTokenDimension(t *testing.T) {
    setupUsageAnalyticsModelTestDBs(t)
    now := usageAnalyticsNow()
    require.NoError(t, DB.AutoMigrate(&QuotaData{}))
    require.NoError(t, DB.Create(&QuotaData{Username: "user-a", ModelName: "fake", Quota: 999, CreatedAt: now}).Error)

    res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
    require.NoError(t, err)
    require.Equal(t, 0, res.Total.RequestCount)
    require.Empty(t, res.Groups)
}
```

MySQL DryRun 使用 `mysql.New(mysql.Config{DSN: "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local", SkipInitializeWithVersion: true})`，PostgreSQL DryRun 使用 `postgres.New(postgres.Config{DSN: "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable TimeZone=UTC", PreferSimpleProtocol: true})`，不连接真实数据库。

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
- 修改：`controller/log_stat_token_test.go`

**依赖：** 任务 1 完成后执行。

- [ ] **步骤 1：编写 controller 参数红灯测试**

在 `controller/usage_analytics_test.go` 中创建 Gin 测试 router，直接设置 `c.Set("id", userID)` 模拟 `UserAuth` 后的上下文。测试 helper 使用与 model 测试相同的分离 `DB` / `LOG_DB`，并写入真实日志。

必须添加以下完整用例：

```go
func TestUsageAnalyticsSummaryDefaultsToRecentSevenDays(t *testing.T) {
    setupUsageAnalyticsControllerTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &model.Log{UserId: 101, CreatedAt: now - 24*60*60, Type: model.LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})

    recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary")
    require.Equal(t, http.StatusOK, recorder.Code)
    require.Contains(t, recorder.Body.String(), `"group_by":"token"`)
    require.Contains(t, recorder.Body.String(), `"total_tokens":10`)
}

func TestUsageAnalyticsRejectsPartialTimeRange(t *testing.T) {
    recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?start_timestamp=1778716800")
    require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestUsageAnalyticsRejectsRangeOverThirtyOneDays(t *testing.T) {
    recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?start_timestamp=1778716800&end_timestamp=1781481601")
    require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestUsageAnalyticsRejectsUnsupportedPhaseOneParams(t *testing.T) {
    for _, rawURL := range []string{
        "/api/usage-analytics/summary?group_by=endpoint",
        "/api/usage-analytics/summary?billing_source=wallet",
        "/api/usage-analytics/summary?billing_tier=gold",
        "/api/usage-analytics/summary?modality=image",
    } {
        recorder := performUsageAnalyticsRequest(t, 101, rawURL)
        require.Equal(t, http.StatusBadRequest, recorder.Code, rawURL)
        require.Contains(t, recorder.Body.String(), "unsupported")
    }
}

func TestUsageAnalyticsIgnoresUserIDAndUsernameQuery(t *testing.T) {
    setupUsageAnalyticsControllerTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &model.Log{UserId: 101, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})
    seedUsageAnalyticsLog(t, &model.Log{UserId: 202, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 2, MeteredTokens: intPtrForUsageAnalyticsTest(999)})

    recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?user_id=202&username=other")
    require.Equal(t, http.StatusOK, recorder.Code)
    require.Contains(t, recorder.Body.String(), `"total_tokens":10`)
    require.NotContains(t, recorder.Body.String(), "999")
}

func TestUsageAnalyticsRejectsForeignTokenID(t *testing.T) {
    setupUsageAnalyticsControllerTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsToken(t, &model.Token{Id: 77, UserId: 202, Name: "foreign", Key: "sk-foreign-1234567890"})

    recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?start_timestamp="+strconv.FormatInt(now-60, 10)+"&end_timestamp="+strconv.FormatInt(now, 10)+"&token_ids=77")
    require.Equal(t, http.StatusBadRequest, recorder.Code)
    require.NotContains(t, recorder.Body.String(), "foreign")
}

func TestUsageAnalyticsParsesRepeatedParamsBeforeCommaFallback(t *testing.T) {
    query, err := parseUsageAnalyticsRawQueryForTest("model_names=gpt-4&model_names=claude&groups=a%2Cb&groups=default")
    require.NoError(t, err)
    require.Equal(t, []string{"claude", "gpt-4"}, query.ModelNames)
    require.Equal(t, []string{"a,b", "default"}, query.Groups)
}

func TestUsageAnalyticsParsesCommaFallbackAndLimits(t *testing.T) {
    query, err := parseUsageAnalyticsRawQueryForTest("token_ids=2,1&streams=true,false&statuses=success,error&limit=500&sort_order=asc")
    require.NoError(t, err)
    require.Equal(t, []int{1, 2}, query.TokenIDs)
    require.Equal(t, []bool{false, true}, query.Streams)
    require.Equal(t, []string{"error", "success"}, query.Statuses)
    require.Equal(t, 50, query.Limit)
    require.Equal(t, "asc", query.SortOrder)
}

func TestUsageAnalyticsValidatesSortFields(t *testing.T) {
    query, err := parseUsageAnalyticsRawQueryForTest("metric=quota")
    require.NoError(t, err)
    require.Equal(t, "quota", query.SortBy)

    query, err = parseUsageAnalyticsRawQueryForTest("sort_by=request_count&sort_order=asc")
    require.NoError(t, err)
    require.Equal(t, "request_count", query.SortBy)
    require.Equal(t, "asc", query.SortOrder)

    recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?sort_by=bad")
    require.Equal(t, http.StatusBadRequest, recorder.Code)
    recorder = performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?sort_order=sideways")
    require.Equal(t, http.StatusBadRequest, recorder.Code)
}
```

- [ ] **步骤 2：运行 controller 红灯测试**

运行：

```bash
go test ./controller -run TestUsageAnalytics -count=1
```

预期：FAIL，失败原因是 handler、parser helper 或 route helper 未定义。

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
- repeated params 优先；没有 repeated params 时兼容 comma fallback；repeated params 中的 `a,b` 不再拆分。
- `token_ids` 解析为去重排序正整数。
- `streams` 只接受 `true` / `false`。
- `statuses` 只接受 `success` / `error`。
- `limit` 默认 10，最大 50。
- `sort_by` 只能是 Phase 1 metric 或 `request_count` / `last_used_at` / `first_used_at`。
- `sort_order` 只接受 `asc` / `desc`。
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

在 `controller/log_usage_analytics_test.go` 添加完整用例：

```go
func TestSelfLogsFiltersByTokenIDStreamAndStatus(t *testing.T) {
    setupUsageAnalyticsControllerTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsToken(t, &model.Token{Id: 11, UserId: 101, Name: "owned", Key: "sk-owned-1234567890"})
    seedUsageAnalyticsLog(t, &model.Log{UserId: 101, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 11, IsStream: true, MeteredTokens: intPtrForUsageAnalyticsTest(10)})
    seedUsageAnalyticsLog(t, &model.Log{UserId: 101, CreatedAt: now - 9, Type: model.LogTypeError, TokenId: 11, IsStream: false})

    list := performSelfLogRequest(t, 101, "/api/log/self?token_id=11&is_stream=true&status=success")
    require.Equal(t, http.StatusOK, list.Code)
    require.Contains(t, list.Body.String(), `"token_id":11`)
    require.NotContains(t, list.Body.String(), `"type":`+strconv.Itoa(model.LogTypeError))

    stat := performSelfLogRequest(t, 101, "/api/log/self/stat?token_id=11&is_stream=true&status=success")
    require.Equal(t, http.StatusOK, stat.Code)
    require.Contains(t, stat.Body.String(), `"total_tokens":10`)
}

func TestSelfLogsStatusConflictsWithType(t *testing.T) {
    recorder := performSelfLogRequest(t, 101, "/api/log/self?status=success&type="+strconv.Itoa(model.LogTypeError))
    require.Equal(t, http.StatusBadRequest, recorder.Code)
    require.Contains(t, recorder.Body.String(), "status conflicts with type")
}

func TestSelfLogsRejectsForeignTokenID(t *testing.T) {
    for _, path := range []string{"/api/log/self?token_id=12", "/api/log/self/stat?token_id=12"} {
        t.Run(path, func(t *testing.T) {
            setupUsageAnalyticsControllerTestDBs(t)
            seedUsageAnalyticsToken(t, &model.Token{Id: 12, UserId: 202, Name: "foreign", Key: "sk-foreign-1234567890"})

            recorder := performSelfLogRequest(t, 101, path)
            require.Equal(t, http.StatusBadRequest, recorder.Code, path)
            require.NotContains(t, recorder.Body.String(), "foreign")
        })
    }
}

func TestSelfLogsAllowsDeletedTokenWithOwnHistory(t *testing.T) {
    for _, path := range []string{"/api/log/self?token_id=13", "/api/log/self/stat?token_id=13"} {
        t.Run(path, func(t *testing.T) {
            setupUsageAnalyticsControllerTestDBs(t)
            now := usageAnalyticsNow()
            token := &model.Token{Id: 13, UserId: 101, Name: "deleted", Key: "sk-deleted-1234567890"}
            seedUsageAnalyticsToken(t, token)
            require.NoError(t, model.DB.Delete(token).Error)
            seedUsageAnalyticsLog(t, &model.Log{UserId: 101, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 13, TokenName: "deleted-history", MeteredTokens: intPtrForUsageAnalyticsTest(10)})

            recorder := performSelfLogRequest(t, 101, path)
            require.Equal(t, http.StatusOK, recorder.Code, path)
            require.Contains(t, recorder.Body.String(), `"token_id":13`)
        })
    }
}

func TestSelfLogsStatUsesUserIDInsteadOfUsername(t *testing.T) {
    setupUsageAnalyticsControllerTestDBs(t)
    now := usageAnalyticsNow()
    seedUsageAnalyticsLog(t, &model.Log{UserId: 101, Username: "same", CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})
    seedUsageAnalyticsLog(t, &model.Log{UserId: 202, Username: "same", CreatedAt: now - 9, Type: model.LogTypeConsume, TokenId: 2, MeteredTokens: intPtrForUsageAnalyticsTest(999)})

    recorder := performSelfLogRequest(t, 101, "/api/log/self/stat?username=same")
    require.Equal(t, http.StatusOK, recorder.Code)
    require.Contains(t, recorder.Body.String(), `"total_tokens":10`)
    require.NotContains(t, recorder.Body.String(), "999")
}
```

同步修改 `controller/log_stat_token_test.go`：现有 self stat 测试必须显式设置 `ctx.Set("id", 5001)`，不能只依赖 username。

- [ ] **步骤 7：运行 Usage Logs 红灯测试**

运行：

```bash
go test ./controller -run 'TestSelfLogs|TestGetLogsSelfStatReturnsTotalTokensAndTpm' -count=1
```

预期：FAIL，新增参数未生效、冲突未处理或现有 self stat 仍缺当前用户 ID。

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

- [ ] **步骤 9：增强 `controller/log.go` query 解析与 token 归属校验**

新增解析：

```go
func parseLogStatusType(c *gin.Context) (int, bool, error)
func validateSelfLogTokenFilter(userID int, tokenID int, startTimestamp int64, endTimestamp int64) error
```

要求：

- `status` 为空时保留现有 `type` 行为。
- `status=success` 与 `type=LogTypeConsume` 等价时允许。
- `status=error` 与 `type=LogTypeError` 等价时允许。
- 冲突返回 HTTP 400，JSON `message` 为 `status conflicts with type`。
- `token_id` 与 `is_stream` 同时传给 list 和 stat。
- `token_id` 先用 `model.DB` 校验未删除 token 属于当前用户；未通过时再用 `model.LOG_DB` 检查当前筛选时间范围内是否存在当前用户该 token 的历史日志；两者都不满足时返回 400，响应不得包含 token 名称或 key。

- [ ] **步骤 10：运行后端 controller 定向测试并提交任务 2**

运行：

```bash
go test ./controller -run 'TestUsageAnalytics|TestSelfLogs' -count=1
```

预期：PASS。

提交：

```bash
git add controller/usage_analytics.go controller/usage_analytics_test.go controller/log_usage_analytics_test.go controller/log_stat_token_test.go router/api-router.go controller/log.go model/log.go
git commit -m "feat(usage-analytics): 接入用户用量分析接口"
```

---

### 任务 3：前端 API、类型、共享筛选合同与纯函数测试

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

`filters.test.ts` 使用 `node:test` 与 `node:assert/strict`，并明确共享合同：

```ts
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildApiKeyUsageAnalyticsSearch,
  buildUsageAnalyticsApiParams,
  buildUsageAnalyticsCanonicalFilters,
  buildUsageLogsDrilldownSearch,
  normalizeUsageAnalyticsSearch,
} from './filters'

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

test('builds stable canonical filters from normalized search', () => {
  const canonical = buildUsageAnalyticsCanonicalFilters({ token_ids: ['2', '1', '2'], model_names: 'gpt-4,claude', streams: 'true,false', statuses: 'success,error' }, 1779321600)
  assert.deepEqual(canonical.token_ids, [1, 2])
  assert.deepEqual(canonical.model_names, ['claude', 'gpt-4'])
  assert.deepEqual(canonical.streams, ['false', 'true'])
  assert.deepEqual(canonical.statuses, ['error', 'success'])
})

test('preserves comma values when input is repeated array', () => {
  const normalized = normalizeUsageAnalyticsSearch({ groups: ['a,b', 'default'] }, 1779321600)
  assert.deepEqual(normalized.groups, ['a,b', 'default'])
})

test('serializes api params as repeated query params', () => {
  const canonical = buildUsageAnalyticsCanonicalFilters({ model_names: ['gpt-4', 'claude'], groups: ['a,b', 'default'] }, 1779321600)
  const params = buildUsageAnalyticsApiParams(canonical)
  assert.deepEqual(params.getAll('model_names'), ['claude', 'gpt-4'])
  assert.deepEqual(params.getAll('groups'), ['a,b', 'default'])
  assert.equal(params.toString().includes('groups=a%2Cb'), true)
})

test('builds api key entry search without full key material', () => {
  const search = buildApiKeyUsageAnalyticsSearch({ id: 42, name: 'prod', key: 'sk-secret' })
  assert.deepEqual(search, { group_by: 'token', token_ids: [42] })
  assert.equal(JSON.stringify(search).includes('sk-secret'), false)
})

test('builds usage logs drilldown search with status not numeric type', () => {
  const search = buildUsageLogsDrilldownSearch({ start_timestamp: 10, end_timestamp: 20 }, { token_id: 5, model_name: 'gpt-4', group: 'default', status: 'error', is_stream: true })
  assert.deepEqual(search, { startTime: 10000, endTime: 20000, tokenId: 5, model: 'gpt-4', group: 'default', status: 'error', isStream: true })
  assert.equal(Object.prototype.hasOwnProperty.call(search, 'type'), false)
})
```

- [ ] **步骤 2：运行 filters 红灯测试**

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/filters.test.ts
```

预期：FAIL，模块未定义。

- [ ] **步骤 3：实现类型、常量、共享筛选合同和 API params**

`types.ts` 定义与后端一致的 union 和导出合同：

```ts
export type UsageAnalyticsGroupBy = 'token' | 'model' | 'group' | 'stream' | 'status'
export type UsageAnalyticsMetric = 'request_count' | 'total_tokens' | 'quota' | 'error_rate' | 'avg_latency_ms' | 'p95_latency_ms'
export type UsageAnalyticsGranularity = 'hour' | 'day'
export type UsageAnalyticsStatus = 'success' | 'error'
export type UsageAnalyticsSortOrder = 'asc' | 'desc'

export interface UsageAnalyticsCanonicalFilters {
  start_timestamp: number
  end_timestamp: number
  granularity: UsageAnalyticsGranularity
  group_by: UsageAnalyticsGroupBy
  metric: UsageAnalyticsMetric
  token_ids: number[]
  model_names: string[]
  groups: string[]
  streams: Array<'true' | 'false'>
  statuses: UsageAnalyticsStatus[]
  limit: number
  sort_by: string
  sort_order: UsageAnalyticsSortOrder
}
```

`filters.ts` 必须导出以下精确入口，任务 4/5 只能消费这些导出，不得自建平行规范化逻辑：

```ts
export const usageAnalyticsSearchSchema: z.ZodType<UsageAnalyticsSearch>
export function normalizeUsageAnalyticsSearch(search: unknown, nowSeconds?: number): UsageAnalyticsCanonicalFilters
export function buildUsageAnalyticsCanonicalFilters(search: unknown, nowSeconds?: number): UsageAnalyticsCanonicalFilters
export function buildUsageAnalyticsApiParams(filters: UsageAnalyticsCanonicalFilters): URLSearchParams
export function buildUsageLogsDrilldownSearch(filters: Pick<UsageAnalyticsCanonicalFilters, 'start_timestamp' | 'end_timestamp'>, drilldown: UsageAnalyticsDrilldown): UsageLogsDrilldownSearch
export function buildApiKeyUsageAnalyticsSearch(input: { id: number; name?: string; key?: string }): Pick<UsageAnalyticsCanonicalFilters, 'group_by' | 'token_ids'>
```

`buildUsageAnalyticsApiParams` 数组使用 `append` repeated params，不用 comma join。`usageAnalyticsSearchSchema` 允许单值、数组和 comma fallback 输入，输出 canonical search。

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

`chart-data.test.ts` 使用完整 fixture：

```ts
import test from 'node:test'
import assert from 'node:assert/strict'
import { buildBreakdownChartData, buildRankingRows, buildTrendChartData, isAdditiveMetric, mergeTopNWithOther } from './chart-data'
import type { UsageAnalyticsGroup, UsageAnalyticsTimeseriesPoint } from '../types'

test('keeps series separated by group_key even when labels match', () => {
  const points = [
    { timestamp: 10, group_key: 'token:1', group_label: 'Same', total_tokens: 10 },
    { timestamp: 10, group_key: 'token:2', group_label: 'Same', total_tokens: 20 },
  ] as UsageAnalyticsTimeseriesPoint[]
  const data = buildTrendChartData(points, 'total_tokens')
  assert.equal(new Set(data.map((item) => item.group_key)).size, 2)
})

test('merges extra additive groups into non-drillable Other', () => {
  const groups = [
    { group_key: 'a', group_label: 'A', total_tokens: 30, request_count: 3, quota: 3, drilldown: { model_name: 'a' } },
    { group_key: 'b', group_label: 'B', total_tokens: 20, request_count: 2, quota: 2, drilldown: { model_name: 'b' } },
    { group_key: 'c', group_label: 'C', total_tokens: 10, request_count: 1, quota: 1, drilldown: { model_name: 'c' } },
  ] as UsageAnalyticsGroup[]
  const merged = mergeTopNWithOther(groups, 'total_tokens', 2)
  assert.equal(merged.length, 3)
  assert.equal(merged[2].group_key, 'other')
  assert.equal(merged[2].total_tokens, 10)
  assert.equal(merged[2].drilldown, null)
})

test('does not stack rate or latency metrics', () => {
  assert.equal(isAdditiveMetric('total_tokens'), true)
  assert.equal(isAdditiveMetric('error_rate'), false)
  assert.equal(isAdditiveMetric('avg_latency_ms'), false)
})

test('builds ranking rows with deleted token safe fields', () => {
  const rows = buildRankingRows([{ group_key: 'token:7', group_label: '', token: { id: 7, name: 'deleted-history', masked_key: null, status: null, group: null, remain_quota: null, unlimited_quota: null, deleted: true }, total_tokens: 0, request_count: 0, drilldown: { token_id: 7 } } as UsageAnalyticsGroup])
  assert.equal(rows[0].display_label, 'deleted-history')
  assert.equal(rows[0].masked_key, null)
})

test('builds empty state for non-additive breakdown metric', () => {
  const data = buildBreakdownChartData([], 'error_rate')
  assert.equal(data.kind, 'unsupported-share')
})
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
git add web/default/src/features/usage-analytics/types.ts web/default/src/features/usage-analytics/constants.ts web/default/src/features/usage-analytics/api.ts web/default/src/features/usage-analytics/lib/filters.ts web/default/src/features/usage-analytics/lib/filters.test.ts web/default/src/features/usage-analytics/lib/format.ts web/default/src/features/usage-analytics/lib/format.test.ts web/default/src/features/usage-analytics/lib/chart-data.ts web/default/src/features/usage-analytics/lib/chart-data.test.ts
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
- 创建：`web/default/src/features/usage-analytics/lib/page-contract.ts`
- 创建：`web/default/src/features/usage-analytics/lib/page-contract.test.ts`

**依赖：** 任务 3 完成后执行。任务 4 不创建 route 文件，不调用 `getRouteApi()`，不运行项目级 `typecheck` 或 `build`。

- [ ] **步骤 1：编写页面合同红灯测试**

创建 `web/default/src/features/usage-analytics/lib/page-contract.test.ts`，使用纯函数约束页面关键行为，不引入 jsdom：

```ts
import test from 'node:test'
import assert from 'node:assert/strict'
import { buildDefaultUsageAnalyticsFilters, buildUsageAnalyticsQueryKeys, buildUsageAnalyticsRankingDrilldown } from './page-contract'

test('builds default usage analytics filters for reset', () => {
  const filters = buildDefaultUsageAnalyticsFilters(1779321600)
  assert.equal(filters.start_timestamp, 1779321600 - 7 * 24 * 60 * 60)
  assert.equal(filters.end_timestamp, 1779321600)
  assert.equal(filters.group_by, 'token')
  assert.equal(filters.metric, 'total_tokens')
  assert.equal(filters.limit, 10)
})

test('builds stable query keys for three analytics endpoints', () => {
  const filters = buildDefaultUsageAnalyticsFilters(1779321600)
  const keys = buildUsageAnalyticsQueryKeys(filters)
  assert.deepEqual(keys.summary, ['usage-analytics', 'summary', filters])
  assert.deepEqual(keys.timeseries, ['usage-analytics', 'timeseries', filters])
  assert.deepEqual(keys.breakdown, ['usage-analytics', 'breakdown', filters])
})

test('builds ranking drilldown target for usage logs common route', () => {
  const filters = buildDefaultUsageAnalyticsFilters(1779321600)
  const target = buildUsageAnalyticsRankingDrilldown(filters, { token_id: 5, status: 'success' })
  assert.equal(target.to, '/usage-logs/$section')
  assert.deepEqual(target.params, { section: 'common' })
  assert.equal(target.search.tokenId, 5)
  assert.equal(target.search.status, 'success')
  assert.equal(Object.prototype.hasOwnProperty.call(target.search, 'type'), false)
})

test('returns disabled drilldown target for Other rows', () => {
  const filters = buildDefaultUsageAnalyticsFilters(1779321600)
  const target = buildUsageAnalyticsRankingDrilldown(filters, null)
  assert.equal(target, null)
})
```

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/page-contract.test.ts
```

预期：FAIL，失败原因是 `page-contract` 新增纯函数未定义。

- [ ] **步骤 2：实现页面容器 props 合同**

`index.tsx` 导出：

```ts
export interface UsageAnalyticsPageProps {
  search: UsageAnalyticsCanonicalFilters
  onSearchChange: (next: Partial<UsageAnalyticsCanonicalFilters>) => void
}

export function UsageAnalyticsPage(props: UsageAnalyticsPageProps): JSX.Element
```

页面容器必须：

- 使用 `SectionPageLayout`。
- 不直接访问 `Route.useSearch()`、`getRouteApi()` 或 `window.location`；route 适配在任务 5 完成。
- 用 `props.search` 生成 React Query key。
- 三个 query key 分别为 `['usage-analytics','summary',props.search]`、`['usage-analytics','timeseries',props.search]`、`['usage-analytics','breakdown',props.search]`。
- 页面说明文案在内容区显式渲染，不依赖 `SectionPageLayout.Description`。

- [ ] **步骤 3：实现筛选栏**

`usage-analytics-filter-bar.tsx` 必须：

- 接收 `value: UsageAnalyticsCanonicalFilters` 与 `onApply(next)`。
- 使用 draft state。
- Apply 后一次性调用 `onApply`。
- Reset 写入默认最近 7 天、`group_by=token`、`metric=total_tokens`、`limit=10`。
- 多选数组保持类型化数组，不用 comma string。

- [ ] **步骤 4：实现总览卡片、趋势图、分布图、排行表**

要求：

- 卡片展示请求数、Tokens、额度、成功率、错误率、平均延迟、P95 延迟、活跃 API Key、RPM、TPM。
- 趋势图使用 `@visactor/react-vchart`，复用现有 dashboard chart 的 `VCHART_OPTION` / 主题模式；additive metric 使用堆叠面积，rate/latency 使用非堆叠线图。
- 分布图使用同一 VChart 主题；rate/latency 显示「该指标不支持占比图」类空态，不画环图。
- 排行表优先复用现有 DataTable / table / empty state 模式，不自建平行 UI 体系。
- 排行表 `View Logs` 使用 `buildUsageLogsDrilldownSearch` 并通过 props 回调或 Link 数据跳转 `/usage-logs/common`。
- `Other` 禁用钻取按钮。

- [ ] **步骤 5：运行前端纯函数测试并提交任务 4**

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/chart-data.test.ts src/features/usage-analytics/lib/page-contract.test.ts
```

预期：PASS。

提交：

```bash
git add web/default/src/features/usage-analytics/index.tsx web/default/src/features/usage-analytics/lib/page-contract.ts web/default/src/features/usage-analytics/lib/page-contract.test.ts web/default/src/features/usage-analytics/components/usage-analytics-filter-bar.tsx web/default/src/features/usage-analytics/components/usage-analytics-summary-cards.tsx web/default/src/features/usage-analytics/components/usage-trend-chart.tsx web/default/src/features/usage-analytics/components/usage-breakdown-chart.tsx web/default/src/features/usage-analytics/components/usage-ranking-table.tsx
git commit -m "feat(usage-analytics): 新增用量分析页面组件"
```

---

### 任务 5：前端路由、入口、Usage Logs 钻取与 i18n

**文件：**
- 创建：`web/default/src/routes/_authenticated/usage-analytics/index.tsx`
- 创建：`web/default/src/features/usage-logs/lib/usage-analytics-drilldown.test.ts`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 修改：`web/default/src/features/keys/components/api-keys-primary-buttons.tsx`
- 修改：`web/default/src/features/keys/components/data-table-row-actions.tsx`
- 修改：`web/default/src/routes/_authenticated/usage-logs/$section.tsx`
- 修改：`web/default/src/features/usage-logs/types.ts`
- 修改：`web/default/src/features/usage-logs/lib/utils.ts`
- 修改：`web/default/src/features/usage-logs/lib/filter.ts`
- 修改：`web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx`
- 修改：`web/default/src/i18n/static-keys.ts`
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`

**依赖：** 任务 4 完成后执行。任务 5 不运行项目级 `typecheck` 或 `build`；`routeTree.gen.ts` 由任务 6 主代理统一生成。

- [ ] **步骤 1：编写 Usage Logs drilldown 链路红灯测试**

创建 `web/default/src/features/usage-logs/lib/usage-analytics-drilldown.test.ts`：

```ts
import test from 'node:test'
import assert from 'node:assert/strict'
import { buildSearchParams } from './filter'
import { buildApiParams } from './utils'

test('preserves usage analytics drilldown through filter apply and api params', () => {
  const routeSearch = {
    startTime: 10000,
    endTime: 20000,
    tokenId: 5,
    model: 'gpt-4',
    group: 'default',
    isStream: true,
    status: 'success',
  }
  const nextSearch = buildSearchParams({ startTime: new Date(10000), endTime: new Date(20000), tokenId: 5, model: 'gpt-4', group: 'default', isStream: true, status: 'success' }, 'common')
  assert.deepEqual(nextSearch, routeSearch)

  const apiParams = buildApiParams({ page: 1, pageSize: 20, searchParams: routeSearch, isAdmin: false })
  assert.equal(apiParams.token_id, 5)
  assert.equal(apiParams.model_name, 'gpt-4')
  assert.equal(apiParams.group, 'default')
  assert.equal(apiParams.is_stream, true)
  assert.equal(apiParams.status, 'success')
  assert.equal(Object.prototype.hasOwnProperty.call(apiParams, 'type'), false)
})

test('targets usage logs common route search without numeric type', () => {
  const search = buildSearchParams({ startTime: new Date(10000), endTime: new Date(20000), status: 'error' }, 'common')
  assert.deepEqual(search, { startTime: 10000, endTime: 20000, status: 'error' })
})
```

- [ ] **步骤 2：运行 Usage Logs drilldown 红灯测试**

运行：

```bash
cd web/default && bun test src/features/usage-logs/lib/usage-analytics-drilldown.test.ts
```

预期：FAIL，`tokenId` / `isStream` / `status` 字段未定义或 API params 未映射。

- [ ] **步骤 3：实现 route search schema 与路由适配**

新增 `web/default/src/routes/_authenticated/usage-analytics/index.tsx`：

```ts
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { UsageAnalyticsPage } from '@/features/usage-analytics'
import { usageAnalyticsSearchSchema } from '@/features/usage-analytics/lib/filters'

export const Route = createFileRoute('/_authenticated/usage-analytics/')({
  validateSearch: usageAnalyticsSearchSchema,
  component: UsageAnalyticsRoute,
})

function UsageAnalyticsRoute() {
  const search = Route.useSearch()
  const navigate = useNavigate()
  return (
    <UsageAnalyticsPage
      search={search}
      onSearchChange={(next) => {
        navigate({ to: '/usage-analytics', search: (prev) => ({ ...prev, ...next }) })
      }}
    />
  )
}
```

`usageAnalyticsSearchSchema` 已在任务 3 定义，必须将单值、数组、comma fallback 归一化为数组，并限制 Phase 1 union。

- [ ] **步骤 4：实现 sidebar 与 API Keys 入口**

- `use-sidebar-data.ts`：`Usage Analytics` 放在 `API Keys` 与 `Usage Logs` 之间，图标从 `lucide-react` 实际导出中选择已存在图标。
- `use-sidebar-config.ts`：`URL_TO_CONFIG_MAP['/usage-analytics'] = { section: 'console', module: 'log' }`。
- `api-keys-primary-buttons.tsx`：新增按钮跳转 `/usage-analytics`。
- `data-table-row-actions.tsx`：新增 `Analyze this Key`，跳转 `{ group_by: 'token', token_ids: [apiKey.id] }` 或调用 `buildApiKeyUsageAnalyticsSearch({ id: apiKey.id, name: apiKey.name })`，不读取完整 key。

- [ ] **步骤 5：实现 Usage Logs 前端 drilldown search**

- `$section.tsx` search schema 新增 `tokenId?: number`、`isStream?: boolean`、`status?: 'success' | 'error'`。
- `types.ts` 同步新增 `CommonLogFilters.tokenId?: number`、`isStream?: boolean`、`status?: 'success' | 'error'`，以及 `GetLogsParams.token_id?: number`、`is_stream?: boolean`、`status?: 'success' | 'error'`。
- `utils.ts` 将 `tokenId` → `token_id`、`isStream` → `is_stream`、`status` → `status`。
- `filter.ts` 的 `buildSearchParams` 对 common filters 保留 `tokenId`、`isStream`、`status`。
- `common-logs-filter-bar.tsx` 从 search 初始化这三个字段；Reset 清空它们；Apply 保留它们；前端不把 `status` 改写为 numeric `type`。

- [ ] **步骤 6：运行 Usage Logs drilldown 绿灯测试**

运行：

```bash
cd web/default && bun test src/features/usage-logs/lib/usage-analytics-drilldown.test.ts src/features/usage-analytics/lib/filters.test.ts
```

预期：PASS。

- [ ] **步骤 7：添加 i18n 文案**

在 `static-keys.ts` 和 6 个 locale 文件加入规格列出的文案，并把 `usage-analytics/constants.ts` 导出的所有 `labelKey` 逐项登记。必须覆盖：

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
Request Count
Total Tokens
Quota
Hourly
Daily
Ascending
Descending
```

翻译必须覆盖 `en`、`zh`、`fr`、`ru`、`ja`、`vi`，保留变量占位符和英文技术词。`constants.ts` 每增加一个 `labelKey`，此步骤都必须同步补齐。

- [ ] **步骤 8：运行定向前端测试并提交任务 5**

运行：

```bash
cd web/default && bun test src/features/usage-logs/lib/usage-analytics-drilldown.test.ts src/features/usage-analytics/lib/filters.test.ts
```

预期：PASS。

提交：

```bash
git add web/default/src/routes/_authenticated/usage-analytics/index.tsx web/default/src/features/usage-logs/lib/usage-analytics-drilldown.test.ts web/default/src/hooks/use-sidebar-data.ts web/default/src/hooks/use-sidebar-config.ts web/default/src/features/keys/components/api-keys-primary-buttons.tsx web/default/src/features/keys/components/data-table-row-actions.tsx web/default/src/routes/_authenticated/usage-logs/\$section.tsx web/default/src/features/usage-logs/types.ts web/default/src/features/usage-logs/lib/utils.ts web/default/src/features/usage-logs/lib/filter.ts web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx web/default/src/i18n/static-keys.ts web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/vi.json
git commit -m "feat(usage-analytics): 接入前端路由与入口"
```

---

### 任务 6：最终整合、审查与验证

**文件：** 只修改任务 1–5 引入或触碰的文件；不得提交与本功能无关的既有 rankings 改动。

- [ ] **步骤 1：检查工作区变更边界**

运行：

```bash
git status --short
git diff --name-only
git diff --cached --name-only
```

确认只处理 Usage Analytics 相关文件。若看到既有 unrelated 改动（例如 rankings 文件），不要 revert、不要 stash、不要加入 commit。禁止使用 `git add .`。

- [ ] **步骤 2：运行后端定向测试**

运行：

```bash
go test ./model ./controller
```

预期：PASS。

- [ ] **步骤 3：运行前端纯函数测试**

运行：

```bash
cd web/default && bun test src/features/usage-analytics/lib/format.test.ts src/features/usage-analytics/lib/chart-data.test.ts src/features/usage-analytics/lib/filters.test.ts src/features/usage-analytics/lib/page-contract.test.ts src/features/usage-logs/lib/usage-analytics-drilldown.test.ts
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

随后使用 `search` 工具检查 `web/default/src/routeTree.gen.ts` 包含 `/usage-analytics`。`routeTree.gen.ts` 只在此步骤由主代理提交。

- [ ] **步骤 6：整体代码审查**

派发 3 个只读审查子代理并发审查：后端正确性、前端正确性、执行与安全边界。审查上下文必须包含本计划、规格文件路径、`git diff` 范围、验证命令输出。

- [ ] **步骤 7：修复审查反馈并复验**

所有 Critical / Important 问题必须修复并复审通过；Minor 问题若不修复，必须有明确技术理由。

- [ ] **步骤 8：最终提交**

如果任务 1–5 子代理已经分步提交，最终只提交整合修复。若尚未提交，按下面白名单分组提交，禁止使用 `git add .` 或宽泛目录：

后端白名单：

```bash
git add dto/usage_analytics.go model/usage_analytics.go model/usage_analytics_test.go controller/usage_analytics.go controller/usage_analytics_test.go controller/log_usage_analytics_test.go controller/log_stat_token_test.go router/api-router.go controller/log.go model/log.go
git commit -m "feat(usage-analytics): 接入用户用量分析接口"
```

前端白名单：

```bash
git add web/default/src/features/usage-analytics/types.ts web/default/src/features/usage-analytics/constants.ts web/default/src/features/usage-analytics/api.ts web/default/src/features/usage-analytics/lib/filters.ts web/default/src/features/usage-analytics/lib/filters.test.ts web/default/src/features/usage-analytics/lib/format.ts web/default/src/features/usage-analytics/lib/format.test.ts web/default/src/features/usage-analytics/lib/chart-data.ts web/default/src/features/usage-analytics/lib/chart-data.test.ts web/default/src/features/usage-analytics/index.tsx web/default/src/features/usage-analytics/lib/page-contract.ts web/default/src/features/usage-analytics/lib/page-contract.test.ts web/default/src/features/usage-analytics/components/usage-analytics-filter-bar.tsx web/default/src/features/usage-analytics/components/usage-analytics-summary-cards.tsx web/default/src/features/usage-analytics/components/usage-trend-chart.tsx web/default/src/features/usage-analytics/components/usage-breakdown-chart.tsx web/default/src/features/usage-analytics/components/usage-ranking-table.tsx web/default/src/routes/_authenticated/usage-analytics/index.tsx web/default/src/features/usage-logs/lib/usage-analytics-drilldown.test.ts web/default/src/hooks/use-sidebar-data.ts web/default/src/hooks/use-sidebar-config.ts web/default/src/features/keys/components/api-keys-primary-buttons.tsx web/default/src/features/keys/components/data-table-row-actions.tsx web/default/src/routes/_authenticated/usage-logs/\$section.tsx web/default/src/features/usage-logs/types.ts web/default/src/features/usage-logs/lib/utils.ts web/default/src/features/usage-logs/lib/filter.ts web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx web/default/src/i18n/static-keys.ts web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/vi.json web/default/src/routeTree.gen.ts
git commit -m "feat(usage-analytics): 新增用户侧用量分析页面"
```

整合修复白名单按实际修复文件逐项列出 pathspec，再提交：

```bash
git add <本功能相关的精确文件路径>
git commit -m "fix(usage-analytics): 修复审查发现的问题"
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
- 验证命令全部通过：`go test ./model ./controller`、前端定向 `bun test` 文件、`bun run i18n:sync`、`bun run typecheck`、`bun run build`。
