# 用户侧用量分析中心规格说明

## 背景

有用户希望在自己的控制台中看到按 API Key 分别统计的用量。进一步讨论后，该能力不应被设计成单一的「API Key 用量页」，因为用户后续还会自然提出按模型、分组、调用形态、结果状态、端点、计费来源等维度分析自己的用量。

本规格将功能定位为用户侧「用量分析中心」（Usage Analytics）：第一版默认按 API Key 聚合，并同时支持模型、分组、流式状态、调用结果等结构化维度；后续阶段再扩展请求端点、计费来源和交叉热力图。这样可以满足当前 API Key 聚合诉求，也避免后续为每个维度重复建设页面和接口。

## 交付分期约定

Phase 1 是第一版必须交付范围；本规格中未特别标注 Phase 2 或 Future 的验收标准，默认均指 Phase 1。Phase 2 和 Phase 3 不阻塞 Phase 1 合并。

Phase 1 必须交付：

- 用户侧 `/usage-analytics` 页面。
- 默认按 API Key 聚合最近 7 天用量。
- 支持聚合维度：API Key、模型、分组、流式状态、调用结果。
- 支持指标：请求数、总 Tokens、额度、成功率、错误率、平均延迟、P95 延迟、RPM、TPM、活跃 API Key 数。
- 支持总览卡片、趋势图、分布图、排行表。
- 支持 API Keys 页面跳转并预选 API Key。
- 支持从分析页钻取到 `/usage-logs/common`，Phase 1 钻取参数为 `token_id`、`model`、`group`、`is_stream`、`status`、时间范围。

Phase 1 不包含：

- endpoint、billing_source、billing_tier、modality 聚合维度。
- `/api/usage-analytics/matrix`。
- 热力图。
- 洞察卡片。
- 新增小时级聚合表。
- 性能基准工程。

Phase 2 扩展 endpoint、billing_source、matrix、热力图和洞察卡片。Phase 3 只在真实数据量证明需要时进入性能优化。

## 实施约束

后续实现直接在当前主工作区 `C:/Users/34404/source/repos/new-api` 开发，不创建、不切换 Git worktree。实现采用子代理方式按文件边界并发开发；不得修改与用量分析无关的文件，不提交未要求的变更。

多子代理实现时，以下共享文件必须串行编辑，不能由多个子代理同时修改同一处：

- `router/api-router.go`
- `controller/log.go`
- `model/log.go`
- `web/default/src/hooks/use-sidebar-data.ts`
- `web/default/src/hooks/use-sidebar-config.ts`
- `web/default/src/routeTree.gen.ts`
- `web/default/src/i18n/locales/*.json`
- `web/default/src/i18n/static-keys.ts`

## 现状与可复用能力

### 后端现状

当前消费与错误日志主要由 `model.Log` 承载，核心字段包括：

- `logs.user_id`：用户 ID。
- `logs.created_at`：日志时间戳，秒级。
- `logs.type`：日志类型，消费日志为 `LogTypeConsume`，错误日志为 `LogTypeError`。
- `logs.token_id`：API Key 对应的 token ID，已有索引。
- `logs.token_name`：API Key 名称。
- `logs.model_name`：模型名称。
- `logs.group`：用户分组。该列名是保留字，原始 SQL 必须通过 `logGroupCol` 引用，不能手写裸 `group`。
- `logs.quota`：本次计费额度。
- `logs.prompt_tokens`：输入 token。
- `logs.completion_tokens`：输出 token。
- `logs.metered_tokens`：实际计量 token，可为空。
- `logs.use_time`：耗时，当前语义为秒。
- `logs.is_stream`：是否流式调用。
- `logs.channel_id`：`model.Log.ChannelId` 的数据库列名；JSON 字段名为 `channel`。Phase 1 用户侧不暴露渠道维度。
- `logs.other`：JSON 文本，包含请求路径、计费来源、订阅扣费、多模态细分等扩展字段。

日志库与主业务库可能分离：

- `logs` 必须通过 `model.LOG_DB` 查询。
- `tokens` 必须通过 `model.DB` 查询。
- 禁止写出跨 `LOG_DB` 与 `DB` 的 SQL JOIN。

现有 `model.SumUsedQuota()` 已确立 token 统计语义：

- `metered_tokens IS NOT NULL` 时使用 `metered_tokens`。
- `metered_tokens IS NULL` 时回退到 `prompt_tokens + completion_tokens`。
- `metered_tokens = 0` 是权威值，必须保留 0，不能回退。

本功能必须沿用该语义，并贯穿 summary、timeseries、breakdown、RPM、TPM 和测试。

### 前端现状

前端已有可复用基础：

- `web/default` 使用 React 19、TanStack Router、React Query、Tailwind、Base UI。
- 图表库为 `@visactor/react-vchart` / `@visactor/vchart`。
- 现有 Dashboard 已有 chart spec 构建、主题色、空状态和统计卡片模式。
- Usage Logs 已有用户侧日志列表与统计接口。
- API Keys 页面已有密钥列表和行操作模式。
- 前端规则要求：用户可见文案必须 i18n；组件 props 非必要不解构；类型显式；避免 `any`；常量 labelKey 需要静态 key 登记或字面量 `t()`。

## 目标

- 新增用户侧「用量分析」页面，面向普通用户展示自己的 API 使用情况。
- 默认按 API Key 聚合，满足「按 API Key 分别统计用量」的直接诉求。
- Phase 1 支持按 API Key、模型、分组、流式状态、调用结果切换聚合。
- 提供总览卡片、趋势图、分布图、排行表。
- 支持从聚合结果钻取到 Usage Logs 明细。
- 不暴露完整 API Key，不展示其他用户数据。
- 保持 SQLite、MySQL、PostgreSQL 三库兼容。

## 非目标

- 不做管理员全站分析页。管理员如果进入本页面，默认也只分析自己的用量。
- 不展示或重新引入价格、余额、可用天数、runway 等 dashboard 指标。
- 不替代 Usage Logs。分析页负责聚合洞察，日志页负责原始明细。
- 不在 Phase 1 引入新的外部图表库或测试框架。
- 不在 Phase 1 强制新增聚合表；性能不足时再进入 Phase 3。
- 不在用户侧暴露渠道名称、渠道密钥或上游供应商内部信息。
- 不使用 `quota_data` 伪造 API Key 维度，因为 `quota_data` 没有 `token_id`。

## 产品信息架构

### 页面与入口

新增页面：

- 导航名称：`用量分析`
- 英文文案：`Usage Analytics`
- 路由：`/usage-analytics`
- 前端目录：`web/default/src/features/usage-analytics/`

入口：

1. 侧边栏 `General` 分组中，放在 `API Keys` 与 `Usage Logs` 之间。
2. `API Keys` 页面顶部动作区增加「查看用量分析」按钮。
3. `API Keys` 表格行操作增加「分析此 Key」，跳转到 `/usage-analytics?group_by=token&token_ids=<id>`。
4. Usage Analytics 中点击可钻取聚合项，跳转到 `/usage-logs/common` 并携带可钻取筛选参数。

API Keys 入口安全边界：

- 顶部按钮放入现有 `ApiKeysPrimaryButtons` 动作区。
- 行操作只使用当前行的 `apiKey.id` 与 `apiKey.name` 生成导航。
- 不调用 `fetchTokenKey`、`fetchTokenKeysBatch` 或任何完整密钥获取接口。
- 不把完整 API Key 写入 URL、React state、日志或 toast。
- 不把 API Keys 页面扩展成分析大屏；Key 管理页只提供入口。

### 页面布局

桌面端布局：

```text
[页面标题]
[全局筛选栏]
[总览指标卡片]
[主趋势图 2/3] [分布图 1/3]
[排行表]
```

Phase 1 不显示热力图和洞察卡片，也不放置不可点击占位。

移动端布局：

- 筛选栏使用现有 `Sheet` / `Drawer` 类组件承载，或降级为纵向卡片。
- 图表单列展示。
- 排行表使用现有 DataTable 移动卡片或精简列模式。
- 每个面板都必须有加载态、空态、错误态和 background fetching 态。

页面说明文案不能依赖当前 `SectionPageLayout.Description` 是否渲染；如果需要展示说明，应在页面内容区 header 或首屏提示卡中显式渲染。

## 聚合维度

### Phase 1 必须支持

| 维度 | `group_by` | 数据来源 | 说明 |
|---|---|---|---|
| API Key | `token` | `logs.token_id`, `logs.token_name`, `tokens` | 默认维度。展示 Key 名称、掩码 Key、状态、分组等补充信息。 |
| 模型 | `model` | `logs.model_name` | 分析模型消耗与错误情况。 |
| 分组 | `group` | `logs.group` | 分析不同用户分组的用量。 |
| 流式状态 | `stream` | `logs.is_stream` | 对比 stream / non-stream。 |
| 调用结果 | `status` | `logs.type` | 成功 / 错误聚合。 |

Phase 1 的后端请求校验只允许以上 5 个 `group_by`。传入 `endpoint`、`billing_source`、`billing_tier`、`modality` 时返回业务错误：`unsupported group_by in current phase`。

### Phase 2 支持

| 维度 | `group_by` | 数据来源 | 说明 |
|---|---|---|---|
| 请求端点 | `endpoint` | `logs.other.request_path` | chat、responses、image、audio、task 等端点。 |
| 计费来源 | `billing_source` | `logs.other.billing_source` | wallet / subscription 等。 |

### Future 维度

`billing_tier`、`modality` 标记为 Future。它们不出现在 Phase 1 的后端 union type、前端 union type、请求校验、UI 选项或验收中。若要纳入 Phase 2，必须补齐 Go 常量、DTO、解析规则、筛选参数、测试和 UI 文案。

### 维度标签规则

- 空模型名显示为 `Unknown Model`。
- 空分组、空字符串分组和 NULL 分组统一显示为 `Unknown Group`。
- `stream = true` 显示为 `Streaming`，`false` 显示为 `Non-streaming`。
- `status = success` 显示为 `Success`；`status = error` 显示为 `Error`。
- Phase 2 endpoint 常见路径归一化：
  - `/v1/chat/completions` → `Chat Completions`
  - `/v1/responses` → `Responses`
  - `/v1/images/generations` → `Images`
  - `/v1/audio/*` → `Audio`
  - 其他路径显示为 `Other Endpoint` 或原路径，具体由 Phase 2 实现固定。
- Phase 2 `billing_source` 为空时显示为 `Unknown Billing Source`，不要猜测为 wallet。

## 指标定义

### 基础日志集合

所有 Usage Analytics 查询的基础日志集合固定为：

```text
logs.user_id = 当前用户 ID
logs.type IN (LogTypeConsume, LogTypeError)
logs.created_at >= start_timestamp
logs.created_at <= end_timestamp
```

错误日志参与请求数、错误数、错误率、RPM 和延迟样本；错误日志的 quota 与 tokens 永远按 0 进入 token/quota 指标。

### 基础指标

| 指标 | 字段 | 定义 |
|---|---|---|
| 请求数 | `request_count` | 消费日志数 + 错误日志数。 |
| 成功数 | `success_count` | `type = LogTypeConsume` 的日志数。 |
| 错误数 | `error_count` | `type = LogTypeError` 的日志数。 |
| 成功率 | `success_rate` | `success_count / request_count`。无请求时为 0。 |
| 错误率 | `error_rate` | `error_count / request_count`。无请求时为 0。 |
| 额度 | `quota` | 消费日志的 `quota` 汇总，错误日志计 0。 |
| 输入 tokens | `prompt_tokens` | 消费日志 `prompt_tokens` 汇总，错误日志计 0。 |
| 输出 tokens | `completion_tokens` | 消费日志 `completion_tokens` 汇总，错误日志计 0。 |
| 计量 tokens | `metered_tokens` | 消费日志实际计量 token 汇总，错误日志计 0。 |
| 总 tokens | `total_tokens` | 展示口径，等于计量 token 语义。 |
| 平均延迟 | `avg_latency_ms` | 过滤后消费日志 + 错误日志的 `use_time` 换算毫秒后的平均值。 |
| P95 延迟 | `p95_latency_ms` | 过滤后消费日志 + 错误日志的 95 分位耗时。 |
| 首次使用时间 | `first_used_at` | 当前筛选范围内最早日志时间。 |
| 最后使用时间 | `last_used_at` | 当前筛选范围内最晚日志时间。 |
| 当前 RPM | `rpm` | 最近 60 秒、应用除 start/end 以外当前筛选条件后的请求数。 |
| 当前 TPM | `tpm` | 最近 60 秒、应用除 start/end 以外当前筛选条件后的消费日志 total_tokens，错误日志计 0。 |
| 活跃 API Key 数 | `active_key_count` | 当前筛选范围内有消费或错误日志的 distinct `token_id` 数；删除 token 的历史日志仍计入。 |

### Token 统计口径

必须使用统一表达式：

```sql
CASE
  WHEN metered_tokens IS NOT NULL THEN metered_tokens
  ELSE prompt_tokens + completion_tokens
END
```

约束：

- 该表达式只应用于 `LogTypeConsume`。
- `metered_tokens = 0` 必须计为 0。
- `metered_tokens IS NULL` 才回退到 `prompt_tokens + completion_tokens`。
- 如果 prompt、completion 或表达式结果为负数，聚合层按 0 计入，不得产生负 token。
- 订阅扣费日志中，`subscription_tokens_consumed` 已在写日志时进入 `metered_tokens`，分析页不再重复读取该字段作为总 tokens，避免双重口径。
- summary、timeseries、breakdown、TPM 必须使用同一口径；测试必须分别覆盖 NULL fallback 和显式 0。

### 延迟单位与 P95

当前 `logs.use_time` 语义为秒。接口对外统一返回毫秒：

```text
avg_latency_ms = AVG(use_time) * 1000
p95_latency_ms = percentile(use_time) * 1000
```

延迟样本规则：

- 默认包含当前候选集合内的 `LogTypeConsume` 与 `LogTypeError`。
- 如果 `status` / `type` 过滤只选择其中一种，则只统计过滤后的类型。
- `use_time = 0` 是有效样本。
- `use_time < 0` 按 0 处理。
- 无样本时 `avg_latency_ms = 0`、`p95_latency_ms = 0`。
- P95 在 Go 层对每个 group、每个 timeseries bucket 分别计算：`index = ceil(0.95 * n) - 1`，index 最小为 0，最大为 `n - 1`。

前端展示时：

- 小于 1000 ms 时显示 `xxx ms`。
- 大于等于 1000 ms 时显示 `x.x s`。
- 格式化函数统一放入 `web/default/src/features/usage-analytics/lib/format.ts` 的 `formatLatencyMs`。

## 后端 API 设计

所有用户侧接口必须使用 `UserAuth`，并强制使用当前登录用户 ID 查询。普通用户请求中的 `user_id`、`username` 一律忽略或返回 400；实现中推荐返回 400，避免误解。

### 路由

在 `router/api-router.go` 中新增：

```go
usageAnalyticsRoute := apiRouter.Group("/usage-analytics")
usageAnalyticsRoute.Use(middleware.UserAuth())
{
    usageAnalyticsRoute.GET("/summary", controller.GetUsageAnalyticsSummary)
    usageAnalyticsRoute.GET("/timeseries", controller.GetUsageAnalyticsTimeseries)
    usageAnalyticsRoute.GET("/breakdown", controller.GetUsageAnalyticsBreakdown)
}
```

Phase 2 再新增：

```go
usageAnalyticsRoute.GET("/matrix", controller.GetUsageAnalyticsMatrix)
```

所有 handler 必须从 `c.GetInt("id")` 获取当前用户 ID。

### 查询参数通用结构

Phase 1 后端接受：

```ts
type UsageAnalyticsQuery = {
  start_timestamp?: number
  end_timestamp?: number
  granularity?: 'hour' | 'day'
  group_by?: 'token' | 'model' | 'group' | 'stream' | 'status'
  metric?: 'request_count' | 'total_tokens' | 'quota' | 'error_rate' | 'avg_latency_ms' | 'p95_latency_ms'
  token_ids?: string
  model_names?: string
  groups?: string
  streams?: string
  statuses?: string
  limit?: number
  sort_by?: 'request_count' | 'total_tokens' | 'quota' | 'error_rate' | 'avg_latency_ms' | 'p95_latency_ms'
  sort_order?: 'asc' | 'desc'
}
```

参数规则：

- `start_timestamp` 与 `end_timestamp` 可同时省略；同时省略时后端默认最近 7 天。
- 只传 `start_timestamp` 或只传 `end_timestamp` 时返回 400。
- 最终时间跨度 `end_timestamp - start_timestamp` 不得超过 31 天。
- 默认 `granularity = day`。
- 默认 `group_by = token`。
- 默认 `metric = total_tokens`。
- `limit` 默认 10，最大 50。
- `sort_by` 默认等于当前 `metric`。
- `sort_order` 默认 `desc`。
- 多选参数使用英文逗号分隔；后端负责 trim、去空、去重、排序。
- `streams` 只接受 `true,false` 形式的布尔字符串。
- `statuses` 只接受 `success,error`。
- `token_ids` 中的每个 ID 必须校验属于当前用户，或存在当前用户该 `token_id` 的历史日志；否则返回 400。
- Phase 1 不接受 `endpoint`、`billing_source`、`billing_tier`、`modality` 相关参数；若传入，返回 `unsupported filter in current phase`。

### 前端维度引用 DTO

所有聚合项必须返回可直接用于展示和钻取的维度引用。前端不得解析 `group_key` 来猜 raw value。

```ts
type UsageAnalyticsDimensionRef = {
  group_by: 'token' | 'model' | 'group' | 'stream' | 'status'
  group_key: string
  group_value: string
  group_label: string
  drilldown: {
    token_id?: number
    model_name?: string
    group?: string
    is_stream?: boolean
    status?: 'success' | 'error'
  } | null
}
```

字段语义：

- `group_key`：全局稳定 key，仅用于 React key、VChart series id、表格 row id。
- `group_value`：未本地化的原始筛选值；token 维度为 token id 字符串。
- `group_label`：展示文案，可由后端归一化或前端本地化。
- `drilldown`：能精确映射到 Usage Logs 时返回具体参数；不能钻取时为 `null`。

Phase 1 各维度 drilldown：

| group_by | drilldown |
|---|---|
| token | `{ token_id }` |
| model | `{ model_name }` |
| group | `{ group }` |
| stream | `{ is_stream }` |
| status | `{ status }` |

API Key 补充信息：

```ts
type UsageAnalyticsTokenInfo = {
  id: number
  name: string
  masked_key: string | null
  status: number | null
  group: string | null
  remain_quota: number | null
  unlimited_quota: boolean | null
  deleted: boolean
}
```

不得返回完整 `key`。

### 聚合组 DTO

```ts
type UsageAnalyticsGroup = UsageAnalyticsDimensionRef & {
  request_count: number
  success_count: number
  error_count: number
  success_rate: number
  error_rate: number
  quota: number
  prompt_tokens: number
  completion_tokens: number
  metered_tokens: number
  total_tokens: number
  avg_latency_ms: number
  p95_latency_ms: number
  first_used_at: number
  last_used_at: number
  share: number | null
  token: UsageAnalyticsTokenInfo | null
}
```

`share` 定义：

- 仅 additive metric（`request_count`、`total_tokens`、`quota`）返回数值。
- 分母为当前筛选范围内所有真实 group 与 Other 合计的当前 metric 值。
- 分母为 0 时 `share = 0`。
- rate / latency metric 返回 `null`。

### 总览接口

```http
GET /api/usage-analytics/summary
```

用途：顶部指标卡与当前维度总览。

响应：

```json
{
  "success": true,
  "message": "",
  "data": {
    "total": {
      "request_count": 1280,
      "success_count": 1260,
      "error_count": 20,
      "success_rate": 0.9844,
      "error_rate": 0.0156,
      "quota": 345600,
      "prompt_tokens": 800000,
      "completion_tokens": 420000,
      "metered_tokens": 1220000,
      "total_tokens": 1220000,
      "avg_latency_ms": 820,
      "p95_latency_ms": 2100,
      "rpm": 3,
      "tpm": 6200,
      "active_key_count": 6
    },
    "groups": [
      {
        "group_by": "token",
        "group_key": "token:123",
        "group_value": "123",
        "group_label": "Production Key",
        "drilldown": { "token_id": 123 },
        "request_count": 900,
        "success_count": 890,
        "error_count": 10,
        "success_rate": 0.9889,
        "error_rate": 0.0111,
        "quota": 240000,
        "prompt_tokens": 500000,
        "completion_tokens": 300000,
        "metered_tokens": 800000,
        "total_tokens": 800000,
        "avg_latency_ms": 760,
        "p95_latency_ms": 1800,
        "first_used_at": 1778716800,
        "last_used_at": 1779321600,
        "share": 0.6557,
        "token": {
          "id": 123,
          "name": "Production Key",
          "masked_key": "sk-a1b2**********x9y8",
          "status": 1,
          "group": "default",
          "remain_quota": 100000,
          "unlimited_quota": false,
          "deleted": false
        }
      }
    ]
  }
}
```

### 趋势接口

```http
GET /api/usage-analytics/timeseries
```

用途：主趋势图、错误率趋势、卡片 sparkline。

响应：

```json
{
  "success": true,
  "message": "",
  "data": {
    "points": [
      {
        "timestamp": 1778716800,
        "time_label": "05-13",
        "group_by": "token",
        "group_key": "token:123",
        "group_value": "123",
        "group_label": "Production Key",
        "drilldown": { "token_id": 123 },
        "request_count": 120,
        "success_count": 118,
        "error_count": 2,
        "success_rate": 0.9833,
        "error_rate": 0.0167,
        "quota": 32000,
        "prompt_tokens": 70000,
        "completion_tokens": 30000,
        "metered_tokens": 100000,
        "total_tokens": 100000,
        "avg_latency_ms": 810,
        "p95_latency_ms": 1600
      }
    ],
    "granularity": "day"
  }
}
```

时间桶规则：

- Phase 1 不使用数据库日期函数生成 bucket。
- 后端读取候选日志必要字段后在 Go 层计算 bucket。
- `stepSeconds = 3600`（hour）或 `86400`（day）。
- `bucket = start_timestamp + ((created_at - start_timestamp) / stepSeconds) * stepSeconds`。
- `created_at < start_timestamp` 或 `created_at > end_timestamp` 的日志不进入结果。
- 如果后续改为 SQL bucket，必须分别为 SQLite、MySQL、PostgreSQL 提供实现和测试，禁止直接使用单库函数。

### 分布与排行接口

```http
GET /api/usage-analytics/breakdown
```

用途：分布环形图、排行图、排行表。

响应：

```json
{
  "success": true,
  "message": "",
  "data": {
    "groups": [],
    "total_groups": 18,
    "other": {
      "group_by": "token",
      "group_key": "other",
      "group_value": "other",
      "group_label": "Other",
      "drilldown": null,
      "request_count": 30,
      "total_tokens": 12000,
      "quota": 3000,
      "share": 0.01
    },
    "sort_by": "total_tokens",
    "sort_order": "desc"
  }
}
```

规则：

- 图表默认取 Top 10。
- 表格可显示 Top 50。
- `total_groups` 是合并 Other 前的真实分组数量。
- 超出 Top N 的组由后端合并为 `Other`。
- `Other` 不参与日志钻取，`drilldown = null`。
- additive metric 下 `Other` 的值为真实合计。
- `error_rate` 下 `Other.error_rate = Other.error_count / Other.request_count`，不能简单平均。
- `avg_latency_ms` 与 `p95_latency_ms` 下，Other 必须由样本或后端聚合结果重算，不能简单平均各组值。

## 后端实现设计

### 文件组织

Phase 1 建议新增：

```text
dto/usage_analytics.go
model/usage_analytics.go
model/usage_analytics_test.go
controller/usage_analytics.go
controller/usage_analytics_test.go
```

需要修改：

```text
router/api-router.go
model/log.go
controller/log.go
```

### 核心类型

```go
type UsageAnalyticsGroupBy string

const (
    UsageAnalyticsGroupByToken  UsageAnalyticsGroupBy = "token"
    UsageAnalyticsGroupByModel  UsageAnalyticsGroupBy = "model"
    UsageAnalyticsGroupByGroup  UsageAnalyticsGroupBy = "group"
    UsageAnalyticsGroupByStream UsageAnalyticsGroupBy = "stream"
    UsageAnalyticsGroupByStatus UsageAnalyticsGroupBy = "status"
)

type UsageAnalyticsQuery struct {
    UserID          int
    StartTimestamp  int64
    EndTimestamp    int64
    Granularity     string
    GroupBy         UsageAnalyticsGroupBy
    Metric          string
    TokenIDs        []int
    ModelNames      []string
    Groups          []string
    Streams         []bool
    Statuses        []string
    Limit           int
    SortBy          string
    SortOrder       string
}
```

Phase 1 不定义 endpoint、billing_source、billing_tier、modality 的 Go 常量、DTO 字段和前端 union 类型。

### 查询策略

#### 基础过滤

所有查询先按结构化条件过滤：

- `logs.user_id = 当前用户 ID`
- `logs.created_at >= start_timestamp`
- `logs.created_at <= end_timestamp`
- `logs.type IN (LogTypeConsume, LogTypeError)`，除非 status 过滤进一步限制。
- `token_ids`、`model_names`、`groups`、`streams`、`statuses` 按请求过滤。

#### 结构化列维度

Phase 1 的所有维度均可从结构化列取得。查询必须通过白名单选择 group expression，不允许把前端传入的 `group_by` 直接拼接到 SQL。

示例映射：

```go
func usageAnalyticsGroupExpr(groupBy UsageAnalyticsGroupBy) (selectExpr string, groupExpr string, ok bool) {
    switch groupBy {
    case UsageAnalyticsGroupByToken:
        return "token_id", "token_id", true
    case UsageAnalyticsGroupByModel:
        return "model_name", "model_name", true
    case UsageAnalyticsGroupByGroup:
        return logGroupCol, logGroupCol, true
    case UsageAnalyticsGroupByStream:
        return "is_stream", "is_stream", true
    case UsageAnalyticsGroupByStatus:
        return "type", "type", true
    default:
        return "", "", false
    }
}
```

API Key 维度规则：

- 只按 `logs.token_id` 分组，不得按 `logs.token_name` 分组。
- `group_key = "token:" + token_id`。
- Token 补充信息禁止通过 logs 与 tokens 做 SQL JOIN。
- 聚合步骤：先使用 `LOG_DB` 聚合日志，再使用 `DB` 批量查询 `tokens` 表中 `id IN (...) AND user_id = 当前用户 ID` 的当前未删除 token。
- 当前 token 存在：`group_label` 使用 `tokens.name`，`masked_key` 使用 `token.GetMaskedKey()`。
- 当前 token 不存在或已删除：`group_label` 使用该 token_id 分组内 `last_used_at` 对应的非空 `logs.token_name`；`token.deleted = true`；`masked_key = null`。
- 如需判断软删除 token 归属，只能用 `Unscoped` 校验 `id`、`user_id`、`deleted_at`，不得返回已删除 token 的 `key` 或掩码。

#### `Other` JSON 维度

Phase 1 不实现 `Other` JSON 维度。

Phase 2 实现 endpoint / billing_source 时必须遵守：

- 只要请求包含 endpoint / billing_source 过滤，无论当前 `group_by` 是结构化列还是 Other 维度，都必须走 Go 层候选日志解析路径。
- SQL 先按 user_id、created_at、type、token_id、model、group、is_stream、status 等结构化条件缩小候选集合。
- 最多读取 50,000 条必要字段，超过时返回业务错误：`当前筛选范围数据量过大，请缩小时间范围或增加筛选条件`。
- 解析 `Other` 必须使用 `common.UnmarshalJsonStr`，不得直接调用 `encoding/json.Unmarshal`。
- 解析失败、字段缺失或字段类型错误时，不使接口失败，统一归入 Unknown 分类。

### 候选日志限制

Phase 1 结构化维度仍受 31 天窗口限制。若实现中某个接口需要在 Go 层计算 P95 或 bucket，需要按用户、时间和结构化过滤条件读取候选日志必要字段。单次候选日志硬上限为 50,000 条；超过时返回业务错误：`当前筛选范围数据量过大，请缩小时间范围或增加筛选条件`。

该限制是 MUST，不是建议。

### 索引与迁移

Phase 1 不新增聚合表。

Phase 1 是否新增轻量复合索引由实现阶段根据现有迁移风险决定，但必须二选一写入实现计划：

1. 新增三库兼容复合索引：
   - `logs(user_id, created_at, type)`：summary / timeseries / breakdown 的基础范围查询。
   - `logs(user_id, token_id, created_at)`：token drilldown 与 token_ids 过滤。
2. 不新增索引：依赖现有 `user_id`、`token_id`、`created_at` 单列/既有索引、31 天窗口和 50,000 候选上限；性能验收降级为功能正确性验收。

若新增索引，必须通过 GORM tag 或三库兼容迁移创建，不能使用单库 DDL。

## Usage Logs 钻取增强

增强现有：

```http
GET /api/log/self
GET /api/log/self/stat
```

Phase 1 新增查询参数：

```ts
type UsageLogsDrilldownQuery = {
  token_id?: number
  is_stream?: boolean
  status?: 'success' | 'error'
}
```

已有参数继续支持：

- `model_name`
- `group`
- `start_timestamp`
- `end_timestamp`
- `type`
- `request_id`
- `upstream_request_id`

规则：

- 新增参数同时适用于 `/api/log/self` 与 `/api/log/self/stat`，二者必须复用同一套过滤构造器。
- self stat 必须按 `logs.user_id = 当前用户 ID` 查询，不再以 username 作为主过滤条件。
- `status` 只接受 `success` / `error`。
- `status=success` 映射 `LogTypeConsume`；`status=error` 映射 `LogTypeError`。
- 如果请求同时传入 `type` 与 `status`：两者等价时允许；冲突时返回 400，`message = "status conflicts with type"`。
- `token_id` 对当前未删除 token 用 `tokens.user_id` 校验。
- 对 deleted token drilldown，若当前筛选范围内存在 `logs.user_id = 当前用户 ID AND logs.token_id = token_id` 的历史日志，也视为允许；否则返回 400。
- endpoint / billing_source 钻取仅 Phase 2。

Phase 2 的 endpoint / billing_source 过滤必须在 Go 层解析完整候选集合后，再计算 total 并做分页切片，禁止只过滤 SQL 分页后的当前页。

## 前端实现设计

所有新增 TS / TSX / 测试文件按仓库现有模式保留 AGPL / QuantumNous 版权头，不修改受保护项目标识。

### 文件组织

Phase 1 新增：

```text
web/default/src/features/usage-analytics/
  api.ts
  constants.ts
  types.ts
  index.tsx
  lib/
    chart-data.ts
    chart-data.test.ts
    filters.ts
    filters.test.ts
    format.ts
    format.test.ts
  components/
    usage-analytics-filter-bar.tsx
    usage-analytics-summary-cards.tsx
    usage-trend-chart.tsx
    usage-breakdown-chart.tsx
    usage-ranking-table.tsx
```

Phase 2 再新增：

```text
web/default/src/features/usage-analytics/lib/insights.ts
web/default/src/features/usage-analytics/lib/insights.test.ts
web/default/src/features/usage-analytics/components/usage-matrix-heatmap.tsx
web/default/src/features/usage-analytics/components/usage-insights.tsx
```

路由：

```text
web/default/src/routes/_authenticated/usage-analytics/index.tsx
```

### 路由与 Sidebar 配置

- 路由文件使用 `createFileRoute('/_authenticated/usage-analytics/')` 定义，必须提供 Zod `validateSearch`。
- `routeTree.gen.ts` 为生成文件，不手写修改；新增 file route 后必须运行能触发 TanStack Router 生成的命令，并提交生成结果。
- 页面组件入口为 `web/default/src/features/usage-analytics/index.tsx`。
- 复用现有 `SectionPageLayout`、`Sheet` / `Drawer`、`Skeleton`、空状态和 DataTable 组件，不新增平行 UI 体系。
- 侧边栏在 `use-sidebar-data.ts` 新增 `Usage Analytics`。
- 侧边栏必须在 `use-sidebar-config.ts` 的 `URL_TO_CONFIG_MAP` 中登记 `/usage-analytics`。Phase 1 复用 `{ section: 'console', module: 'log' }`，使关闭 Usage Logs / 日志模块时分析入口也一致隐藏。
- 图标实现前必须确认 `lucide-react` 实际导出，不能写死不存在的图标。

### URL 状态

`/usage-analytics` 必须支持空 URL 直接打开。路由 `validateSearch` 负责把缺省或非法 search 归一化为：最近 7 天、`granularity = 'day'`、`group_by = 'token'`、`metric = 'total_tokens'`、`limit = 10`。

Phase 1 必须进入 URL 的字段：

- `start_timestamp`、`end_timestamp`：秒级 Unix 时间；空 URL 由前端计算默认值后请求 API。
- `granularity`: `hour | day`
- `group_by`: `token | model | group | stream | status`
- `metric`: `request_count | total_tokens | quota | error_rate | avg_latency_ms | p95_latency_ms`
- `token_ids: number[]`
- `model_names: string[]`
- `groups: string[]`
- `streams: ('true' | 'false')[]`
- `statuses: ('success' | 'error')[]`
- `limit`：默认 10，最大 50。
- `sort_by`
- `sort_order`

URL 层使用类型化数组保存多选，API 层再序列化为后端需要的英文逗号分隔字符串；外部链接传入单个字符串或逗号字符串时，`validateSearch` 也要 normalize 为数组并去空、去重、排序，保证 React Query key 稳定。若某个值本身包含逗号，URL 类型化数组是权威来源；API 序列化时应使用 `URLSearchParams` 多值或后端支持的安全编码，不得不可逆拆分。

从 API Keys 跳转使用类型安全导航：`/usage-analytics?group_by=token&token_ids=<id>`；前端不得把完整 API Key 放入 URL 或 state。

### Usage Logs 前端钻取契约

Usage Analytics 的「查看日志」必须跳转到 `/usage-logs/common`，不要跳到 `/usage-logs`，避免默认 redirect 丢失 search。

需要同步更新：

- `web/default/src/routes/_authenticated/usage-logs/$section.tsx` 的 `validateSearch`。
- `web/default/src/features/usage-logs/types.ts` 的 `CommonLogFilters`、`GetLogsParams`、`GetLogStatsParams`。
- `web/default/src/features/usage-logs/lib/utils.ts` 或现有 search/API 参数映射函数。
- `CommonLogsFilterBar` 的展示、清空和敏感信息处理。

Phase 1 新增 Usage Logs search 字段：

```ts
type UsageLogsDrilldownSearch = {
  startTime?: number
  endTime?: number
  tokenId?: number
  model?: string
  group?: string
  isStream?: boolean
  status?: 'success' | 'error'
}
```

映射规则：

- `tokenId` → `/api/log/self?token_id=<id>`，后端校验归属。
- `status='success'` → `type=LogTypeConsume`；`status='error'` → `type=LogTypeError`。
- `isStream` → `/api/log/self?is_stream=<true|false>`。
- `model` → 现有 `model_name`。
- `group` → 现有 group 过滤。
- 普通用户路径不得接受或发送 `username`、`channel`。

### React Query

API 查询参数必须由 `buildUsageAnalyticsApiParams(search)` 生成 canonical 对象：数组去空、去重、排序；数字和布尔值完成 coercion；不把 Date、函数、`t` 或临时 UI draft state 放进 query key。

query key 使用层级数组：

```ts
['usage-analytics', 'summary', canonicalFilters]
['usage-analytics', 'timeseries', canonicalFilters]
['usage-analytics', 'breakdown', canonicalFilters]
```

要求：

- 筛选表单采用 draft state，点击 Search / Apply 后一次性写 URL。
- 文本输入如要即时搜索必须 debounce，避免每次键入触发 summary / timeseries / breakdown 并发请求。
- `placeholderData` 只复用同 endpoint 的 previousData。
- 切换 `group_by` 或 `metric` 时可以保留旧图表但必须展示 fetching 状态，不能让标题 / 筛选与旧数据不一致。
- HTTP 错误依赖全局 axios / React Query 错误处理。
- 业务 `success=false` 只展示一次 toast，并在对应面板显示可重试错误态。

### 图表数据语义

- VChart 的 `seriesField` 使用 `group_key`，tooltip / legend / title 展示 `group_label`；同名 Key、模型或分组不得合并成同一 series。
- 颜色复用现有 dashboard / VChart 主题：组件使用现有 chart theme、`VCHART_OPTION`、主题定制和圆角 token；不要新增图表库或并行主题系统。
- Top N 必须在整个筛选窗口内按当前排序指标确定，同一个时间序列请求中所有时间点使用同一组 Top N；其余项合并为同一个稳定 `Other` series。
- `Other` 不参与日志钻取。
- `share` 仅对 additive metric 返回。
- additive metric（`request_count`、`total_tokens`、`quota`）可用堆叠面积图、堆叠柱状图、环图、Top N + Other。
- rate / latency metric（`error_rate`、`avg_latency_ms`、`p95_latency_ms`）不得堆叠或求和；趋势图使用非堆叠折线，排行按值排序。

### 图表行为

#### 总览卡片

展示：

- 总请求数
- 总 Tokens
- 计费额度
- 成功率
- 错误率
- 平均延迟
- P95 延迟
- 活跃 API Key 数
- 当前 RPM
- 当前 TPM

注意：

- 不展示价格、余额、可用天数。
- 敏感数值如果未来接入隐藏开关，应复用 Usage Logs 的敏感信息可见性模式。

#### 主趋势图

- additive metric 默认图表：堆叠面积图。
- rate / latency metric 默认图表：非堆叠折线图。
- X 轴：时间。
- Y 轴：当前 metric。
- Series：当前 group_by，对应 `group_key`。
- Tooltip 展示 `group_label`、当前 metric、请求数、错误率、Tokens。

#### 分布图

- additive metric 使用环形图。
- rate / latency metric 不使用环形图，改用排行条形图或显示「该指标不支持占比图」空状态。
- Tooltip 显示请求数、Tokens、额度、错误率、占比。

#### 排行表

列：

- 聚合项名称。
- 请求数。
- 成功数。
- 错误数。
- 错误率。
- Tokens。
- 额度。
- 平均延迟。
- P95 延迟。
- 首次使用时间。
- 最后使用时间。
- 操作：查看日志。

API Key 维度额外列：

- 掩码 Key。
- 状态。
- 分组。
- 剩余额度。
- Deleted 状态。

### 格式化

- quota 使用现有 `formatQuota`。
- tokens 使用 `formatTokens` 或分析页封装的等价函数，必须正确显示 0。
- 延迟使用新增 `formatLatencyMs`。
- 百分比保留 1–2 位小数，0 请求时显示 `0%`。

## 权限与安全

必须满足：

- 所有 `/api/usage-analytics/*` 接口使用 `UserAuth`。
- 后端强制 `WHERE logs.user_id = 当前用户 ID`。
- 普通用户不能传入或影响 `user_id` / `username`。
- `token_id` 过滤必须校验 token 归属或当前用户历史日志归属。
- 返回 API Key 时只返回 `masked_key`。
- 删除 API Key 时 `masked_key = null`，不得通过已删除 token 的 key 生成掩码。
- 不返回完整 `key`。
- 不返回 `Other.admin_info`、渠道密钥、上游密钥或内部调试字段。
- 删除的 API Key 只显示历史 `token_name`，不能恢复密钥。
- 所有从日志或 `Other` 得到的值必须作为普通字符串渲染，前端不得以 HTML 注入方式渲染。

## 三库兼容要求

- 优先使用 GORM 查询构造。
- 原始 SQL 必须限制在三库通用表达式。
- 不使用数据库 JSON 操作符。
- 不使用 PostgreSQL 专属 `FILTER`、`PERCENTILE_CONT`、`DISTINCT ON`。
- 不使用 MySQL 专属 JSON 函数、`GROUP_CONCAT` 或 `IFNULL`。
- 不使用 SQLite 不支持的 `ALTER COLUMN`。
- 不使用数据库专属时间函数：`DATE_TRUNC`、`FROM_UNIXTIME`、`strftime` 等。
- 保留 `group` 相关字段时使用现有 `logGroupCol` / `commonGroupCol` 风格，避免保留字问题。
- SQLite 为必跑测试；MySQL / PostgreSQL DSN 测试可沿用仓库 env-gated 模式，但不能把「只在 SQLite 通过」作为三库兼容的唯一证据。
- 结构化聚合 SQL 必须由白名单函数生成，并用单元测试断言不包含 JSON 操作符、库专属时间函数、percentile/window 函数、未引用的 `group` 字段。

## 性能设计

### Phase 1 策略

- 结构化维度使用 SQL 过滤和基础聚合。
- P95 和时间 bucket 在 Go 层计算。
- 用户侧查询窗口限制为 31 天。
- 默认时间范围为 7 天。
- Top N 默认 10，最大 50。
- Go 层候选日志硬上限为 50,000 条。

### Phase 3 触发条件

如果出现以下情况，进入 Phase 3 前必须先定义目标数据集和验收阈值：

- 单用户 31 天 50,000 条日志下 summary / timeseries / breakdown P95 超过目标阈值。
- endpoint / billing_source 维度频繁查询超出候选日志上限。
- Matrix 查询明显拖慢数据库或 Go 进程。

示例目标阈值只有在 Phase 3 计划中固定后才生效：

- 单用户 31 天 50,000 条日志。
- summary / timeseries / breakdown P95 < 1s。
- matrix P95 < 2s。

没有目标数据集和阈值时，不得把「稳定」作为自动验收项。

Phase 3 优化方案：

1. 在 `logs` 增加结构化列：
   - `request_path`
   - `billing_source`
2. 写日志时从 `Other` 同步填充。
3. 对新列加索引。
4. 必要时新增 `usage_analytics_hourly` 聚合表。

## 国际化

新增前端文案必须覆盖：

```text
web/default/src/i18n/locales/en.json
web/default/src/i18n/locales/zh.json
web/default/src/i18n/locales/fr.json
web/default/src/i18n/locales/ru.json
web/default/src/i18n/locales/ja.json
web/default/src/i18n/locales/vi.json
```

动态常量和配置项的 labelKey 必须满足以下任一条件：

1. 以 `t('...')` 字面量形式出现在组件中，可被同步脚本扫描；或
2. 登记到 `web/default/src/i18n/static-keys.ts`。

必须覆盖的文案包括：

- `Usage Analytics`
- `Analyze your API usage across keys, models, groups, and request outcomes`
- `View Usage Analytics`
- `Analyze this Key`
- `Group by`
- `Metric`
- `Apply filters`
- `Reset filters`
- `Time range`
- `Granularity`
- `Top N`
- `API Key Usage`
- `Model Usage`
- `Group Usage`
- `Streaming`
- `Non-streaming`
- `Success Rate`
- `Error Rate`
- `Average Latency`
- `P95 Latency`
- `Active API Keys`
- `Usage Trend`
- `Usage Breakdown`
- `Usage Ranking`
- `View Logs`
- `View logs for this item`
- `This item cannot be drilled down`
- `No usage data`
- `No matching usage data`
- `Try adjusting the time range or filters`
- `Deleted API Key`
- `Unknown Model`
- `Unknown Group`
- `Other`
- `Retry`
- `Failed to load usage analytics`

新增或修改文案后运行 `bun run i18n:sync`，并检查 `web/default/src/i18n/locales/_reports/_sync-report.json`。新增 key 不得留在 untranslated 报告中。

## 测试计划

### 后端测试

新增 `model/usage_analytics_test.go`，必须覆盖：

1. `metered_tokens = 0` 保留 0，不回退。
2. `metered_tokens IS NULL` 回退到 `prompt_tokens + completion_tokens`。
3. prompt / completion 或表达式结果为负时按 0 计入。
4. 成功日志和错误日志分别进入 `success_count`、`error_count`。
5. 错误日志 token / quota 按 0 计入。
6. `status` 维度聚合正确。
7. `group_by = token` 在同一 `token_id` 多个历史 `token_name` 时只产生一个组。
8. 删除 token 时 `deleted = true` 且不返回 `masked_key`。
9. hour / day 分桶边界正确。
10. P95 使用 `ceil(0.95*n)-1` 算法，并同时覆盖消费日志和错误日志样本。
11. RPM / TPM 语义正确：RPM 统计最近 60 秒消费 + 错误请求，TPM 只统计最近 60 秒消费日志 total_tokens。
12. Top N / Other 合并正确，Other 不可钻取。
13. 查询时间范围生效。
14. 查询跨度超过 31 天时返回错误。
15. 候选日志超过 50,000 条时返回明确错误。
16. 不读取 `quota_data` 伪造 token 维度。
17. GORM DryRun 或 SQL 构造测试分别以 sqlite / mysql / postgres 方言生成查询，断言不包含 JSON 操作符、`DATE_TRUNC`、`FROM_UNIXTIME`、`strftime`、`PERCENTILE_CONT`、窗口函数、裸 `group` 字段。

新增 `controller/usage_analytics_test.go`，必须覆盖：

1. 用户只能看到自己的日志聚合。
2. 外部 `token_id` 返回 400，且不泄露名称或 key。
3. 当前用户 deleted token 的历史日志可钻取，`deleted = true`，`masked_key = null`。
4. 同时省略 start/end 时默认最近 7 天。
5. 只传 start 或只传 end 返回 400。
6. Phase 1 传入 endpoint / billing_source / billing_tier / modality 返回 unsupported 错误。
7. API 不接受 `user_id` / `username` 影响结果。

增强 `controller/log.go` / `model/log.go` 对应测试，必须覆盖：

1. `/api/log/self` 与 `/api/log/self/stat` 的 `token_id` 过滤一致。
2. `is_stream` 过滤生效。
3. `status=success` / `status=error` 映射正确。
4. `type` 与 `status` 冲突时返回 400。
5. self stat 改为按 `user_id` 过滤，结果与同筛选条件的列表一致。

测试 fixture 使用 in-memory SQLite 和真实 GORM 数据，不 mock analytics 业务逻辑。

### 前端测试

当前 `web/default/package.json` 没有 `test` 脚本，新增测试使用 `node:test` 风格并通过 `bun test <具体文件>` 运行。除非本规格另行升级测试基础设施，不引入 `@testing-library/react`、`user-event`、`jsdom`、MSW 或 Vitest。

纯函数测试：

- `web/default/src/features/usage-analytics/lib/format.test.ts`
- `web/default/src/features/usage-analytics/lib/chart-data.test.ts`
- `web/default/src/features/usage-analytics/lib/filters.test.ts`

测试内容：

1. `/usage-analytics` 空 search 归一化为最近 7 天默认筛选，并向 API 发送秒级 `start_timestamp` / `end_timestamp`。
2. 多选 URL 参数去空、去重、排序，并序列化为后端参数。
3. 从 API Keys 顶部按钮和行操作跳转时，只携带 token id，不请求或暴露完整 API Key。
4. 筛选 draft 变化不会立即触发 API 参数提交；Apply 后 canonical filters 稳定。
5. Usage Analytics「查看日志」跳到 `/usage-logs/common`，`tokenId`、`status`、`isStream`、`model`、`group`、`startTime`、`endTime` 在 Usage Logs route、filter bar 和 API params 中全部保留。
6. additive metric 的 Top N / Other、share、tooltip 数据转换正确。
7. rate / latency metric 不堆叠、不求和。
8. `group_key` 相同 / 不同、`group_label` 重名时 series 不合并。
9. 空数据、业务错误、background fetching 有明确状态数据。
10. 删除 API Key 只展示 `Deleted API Key` 和历史名称，不展示完整 key，nullable token 字段不导致渲染崩溃。
11. 常量驱动 labelKey 已进入 `static-keys.ts` 或可被 `i18n:sync` 扫描。

图表测试只断言 chart spec 的 data、field、metric、Other 合并和空态，不做整份 snapshot。

### 验证命令

后端：

```bash
go test ./model ./controller
```

前端：

```bash
cd web/default
bun test src/features/usage-analytics/lib/format.test.ts src/features/usage-analytics/lib/chart-data.test.ts src/features/usage-analytics/lib/filters.test.ts
bun run i18n:sync
bun run typecheck
```

新增 file route 后，必须运行能触发 TanStack Router 生成的命令，并确认 `web/default/src/routeTree.gen.ts` 包含 `/usage-analytics`。

## 分期实施计划

### Phase 1：核心用量分析

交付：

- `/usage-analytics` 页面。
- `/api/usage-analytics/summary`。
- `/api/usage-analytics/timeseries`。
- `/api/usage-analytics/breakdown`。
- 支持 group_by：token、model、group、stream、status。
- 总览卡片、趋势图、分布图、排行表。
- API Keys 页面跳转并预选 Key。
- Usage Logs 支持 `token_id`、`is_stream`、`status` 钻取。

验收：

- 用户打开空 URL `/usage-analytics` 时，默认看到最近 7 天按 API Key 聚合的用量。
- 用户可以切换到模型、分组、流式状态、调用结果维度。
- 用户可以点击可钻取聚合项进入 `/usage-logs/common`，并保留对应筛选条件。
- 页面展示请求数、Tokens、额度、成功率、错误率、平均延迟、P95 延迟、RPM、TPM。
- 页面至少包含总览卡片、趋势图、分布图、排行表。
- API Key 页面可以跳转到分析页并预选 Key。
- 所有用户侧接口都只能访问当前用户数据。
- 前端不展示完整 API Key。
- i18n 覆盖所有新增用户可见文案。
- 后端和前端相关测试通过。

### Phase 2：扩展维度与交叉分析

交付：

- 支持 endpoint、billing_source。
- `/api/usage-analytics/matrix`。
- API Key × 模型热力图。
- API Key × 端点热力图。
- 洞察卡片。

验收：

- 用户可以看到端点与计费来源构成。
- 用户可以通过热力图识别 Key 与模型、端点之间的消耗关系。
- endpoint / billing_source 在 `Other` 缺失时有稳定 unknown 归类。
- 洞察卡片由 `lib/insights.ts` 的纯函数生成，并定义输入、最小样本数、并列排序和无数据返回空数组规则。

### Phase 3：性能优化

触发后交付：

- 结构化 `request_path`、`billing_source` 列。
- 写日志同步填充新列。
- 必要索引。
- 可选小时级聚合表。

验收：

- 进入 Phase 3 前必须定义目标数据集和接口 P95 阈值。
- 常用查询在目标数据量下达到阈值。
- 旧日志仍可回退解析 `Other` 或显示 unknown，不破坏历史兼容。

## 多子代理实施边界

后续实现计划应按低冲突边界拆分：

### 任务 A：后端 DTO / model / model 测试

负责文件：

- `dto/usage_analytics.go`
- `model/usage_analytics.go`
- `model/usage_analytics_test.go`

不得修改：

- `router/api-router.go`
- `controller/log.go`
- `model/log.go`
- 前端文件

### 任务 B：后端 controller / router / 日志钻取

负责文件：

- `controller/usage_analytics.go`
- `controller/usage_analytics_test.go`
- `controller/log.go`
- `model/log.go`
- `router/api-router.go`

该任务触碰共享文件，必须串行执行或由单一子代理主改。

### 任务 C：前端 API / types / lib / 纯函数测试

负责文件：

- `web/default/src/features/usage-analytics/api.ts`
- `web/default/src/features/usage-analytics/types.ts`
- `web/default/src/features/usage-analytics/constants.ts`
- `web/default/src/features/usage-analytics/lib/*`

不得修改：

- 路由文件
- i18n locale JSON
- sidebar 文件
- API Keys 页面
- Usage Logs 页面

### 任务 D：前端页面 / 组件 / 图表

负责文件：

- `web/default/src/features/usage-analytics/index.tsx`
- `web/default/src/features/usage-analytics/components/*`

依赖任务 C 的 API/types/lib 合同。不得修改 i18n locale JSON、routeTree、sidebar 配置。

### 任务 E：入口 / 路由 / Usage Logs drilldown / i18n

负责文件：

- `web/default/src/routes/_authenticated/usage-analytics/index.tsx`
- `web/default/src/routes/_authenticated/usage-logs/$section.tsx`
- `web/default/src/hooks/use-sidebar-data.ts`
- `web/default/src/hooks/use-sidebar-config.ts`
- `web/default/src/features/keys/*`
- `web/default/src/features/usage-logs/*`
- `web/default/src/i18n/*`
- `web/default/src/routeTree.gen.ts`

该任务触碰多个共享入口文件，必须在前端 API/types 稳定后执行。

主代理最终统一运行定向验证命令；子代理不运行项目级 build/lint/format。

## 风险与处理

### 风险一：日志库与主库分离导致跨库 JOIN 失败

处理：规格明确禁止 `LOG_DB` 与 `DB` 跨库 JOIN。日志聚合先在 `LOG_DB` 完成，token 补充信息再用 `DB` 批量查询。

### 风险二：日志缺失导致统计不完整

处理：页面空状态说明「当前筛选范围暂无用量数据」。不回退到 `quota_data` 伪造 API Key 维度，因为 `quota_data` 没有 `token_id`。

### 风险三：API Key 名称可变导致历史展示混淆

处理：聚合主键只使用 `token_id`；展示名称优先使用当前 token name。删除 Key 使用日志中的历史 `token_name` 并标记 `deleted = true`。

### 风险四：P95 计算成本较高

处理：Phase 1 在用户侧 31 天窗口和 50,000 候选日志上限内计算；数据量过大时返回清晰错误。后续再做采样或聚合表。

### 风险五：多数据库 SQL 差异

处理：SQL 只用于基础过滤和通用聚合；时间桶、P95、复杂维度在 Go 层处理；测试断言禁止单库函数。

### 风险六：图表指标语义错误

处理：明确 additive metric 与 rate / latency metric 的图表限制。错误率和延迟不得堆叠、求和或简单平均。
