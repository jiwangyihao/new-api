# 用户侧用量分析中心规格说明

## 背景

有用户希望在自己的控制台中看到按 API Key 分别统计的用量。进一步讨论后，该能力不应被设计成单一的「API Key 用量页」，因为用户后续很可能还需要按模型、分组、端点、计费来源、调用形态等维度分析自己的用量。

因此，本规格将功能定位为用户侧「用量分析中心」（Usage Analytics）：默认按 API Key 聚合，满足当前明确诉求；同时提供可扩展的聚合维度模型，避免后续为每个维度重复建设页面和接口。

## 现状与可复用能力

### 后端现状

当前消费与错误日志主要由 `model.Log` 承载，核心字段包括：

- `logs.user_id`：用户 ID。
- `logs.created_at`：日志时间戳，秒级。
- `logs.type`：日志类型，消费日志为 `LogTypeConsume`，错误日志为 `LogTypeError`。
- `logs.token_id`：API Key 对应的 token ID，已有索引。
- `logs.token_name`：API Key 名称。
- `logs.model_name`：模型名称。
- `logs.group`：用户分组。
- `logs.quota`：本次计费额度。
- `logs.prompt_tokens`：输入 token。
- `logs.completion_tokens`：输出 token。
- `logs.metered_tokens`：实际计量 token，可为空。
- `logs.use_time`：耗时，当前语义为秒。
- `logs.is_stream`：是否流式调用。
- `logs.other`：JSON 文本，包含请求路径、计费来源、订阅扣费、多模态细分等扩展字段。

现有 `model.SumUsedQuota()` 已确立 token 统计语义：

- `metered_tokens IS NOT NULL` 时使用 `metered_tokens`；
- `metered_tokens IS NULL` 时回退到 `prompt_tokens + completion_tokens`；
- `metered_tokens = 0` 是权威值，必须保留 0，不能回退。

本功能必须沿用该语义。

### 前端现状

前端已有可复用基础：

- `web/default` 使用 React 19、TanStack Router、React Query、Tailwind、Base UI。
- 图表库为 `@visactor/react-vchart` / `@visactor/vchart`。
- 现有 Dashboard 已有 chart spec 构建、主题色、空状态和统计卡片模式。
- Usage Logs 已有用户侧日志列表与统计接口。
- API Keys 页面已有密钥列表和行操作模式。

## 目标

- 新增用户侧「用量分析」页面，面向普通用户展示自己的 API 使用情况。
- 默认按 API Key 聚合，满足「按 API Key 分别统计用量」的直接诉求。
- 支持按模型、分组、端点、计费来源、流式状态、调用结果等维度切换聚合。
- 提供总览卡片、趋势图、分布图、排行表、交叉热力图和洞察卡片。
- 支持从聚合结果钻取到 Usage Logs 明细。
- 不暴露完整 API Key，不展示其他用户数据。
- 保持 SQLite、MySQL、PostgreSQL 三库兼容。

## 非目标

- 不做管理员全站分析页。管理员如果进入本页面，默认也只分析自己的用量。
- 不展示或重新引入价格、余额、可用天数、runway 等 dashboard 指标。
- 不替代 Usage Logs。分析页负责聚合洞察，日志页负责原始明细。
- 不在初版引入新的外部图表库。
- 不在初版强制新增聚合表；性能不足时再进入后续优化阶段。
- 不在用户侧暴露渠道名称、渠道密钥或上游供应商内部信息。

## 产品信息架构

### 页面与入口

新增页面：

- 导航名称：`用量分析`
- 英文文案：`Usage Analytics`
- 路由建议：`/usage-analytics`
- 前端目录：`web/default/src/features/usage-analytics/`

入口：

1. 侧边栏 `General` 分组中，放在 `API Keys` 与 `Usage Logs` 之间或紧邻 `Usage Logs`。
2. `API Keys` 页面顶部增加「查看用量分析」按钮。
3. `API Keys` 表格行操作增加「分析此 Key」，跳转到 `/usage-analytics?group_by=token&token_ids=<id>`。
4. Dashboard Overview 可增加轻量入口卡片「最近 7 天用量分析」，但不是初版必需项。
5. Usage Analytics 中点击任意聚合项，可跳转到 Usage Logs 并携带可钻取筛选参数。

### 页面布局

桌面端布局：

```text
[页面标题与说明]
[全局筛选栏]
[总览指标卡片]
[主趋势图 2/3] [分布图 1/3]
[排行条形图 1/2] [错误率趋势 1/2]
[交叉热力图]
[聚合排行表]
[洞察卡片]
```

移动端布局：

- 筛选栏折叠为抽屉或纵向卡片。
- 图表单列展示。
- 热力图允许横向滚动。
- 排行表使用现有 DataTable 移动卡片模式或精简列。

## 聚合维度

### 第一阶段必须支持

| 维度 | `group_by` | 数据来源 | 说明 |
|---|---|---|---|
| API Key | `token` | `logs.token_id`, `logs.token_name`, `tokens` | 默认维度。展示 Key 名称、掩码 Key、状态、分组等补充信息。 |
| 模型 | `model` | `logs.model_name` | 分析模型消耗与错误情况。 |
| 分组 | `group` | `logs.group` | 分析不同用户分组的用量。 |
| 流式状态 | `stream` | `logs.is_stream` | 对比 stream / non-stream。 |
| 调用结果 | `status` | `logs.type` | 成功 / 错误聚合。 |

### 第二阶段支持

| 维度 | `group_by` | 数据来源 | 说明 |
|---|---|---|---|
| 请求端点 | `endpoint` | `logs.other.request_path` | chat、responses、image、audio、task 等端点。 |
| 计费来源 | `billing_source` | `logs.other.billing_source` | wallet / subscription 等。 |
| 计费档位 | `billing_tier` | `logs.other.matched_tier` | tiered expression billing 场景。 |
| 多模态类型 | `modality` | `logs.other.audio/image/web_search/file_search` | 多模态消耗构成。 |

### 维度标签规则

- 空模型名显示为 `Unknown Model`。
- 空分组显示为 `Default` 或 `Unknown Group`，以现有分组展示约定为准。
- `stream = true` 显示为 `Streaming`，`false` 显示为 `Non-streaming`。
- `status` 显示为 `Success` / `Error`。
- `endpoint` 应对常见路径做归一化，例如：
  - `/v1/chat/completions` → `Chat Completions`
  - `/v1/responses` → `Responses`
  - `/v1/images/generations` → `Images`
  - `/v1/audio/*` → `Audio`
  - 其他路径保留原路径或显示为 `Other Endpoint`。
- `billing_source` 为空时显示为 `Unknown Billing Source`，不要猜测为 wallet。

## 指标定义

### 基础指标

| 指标 | 字段 | 定义 |
|---|---|---|
| 请求数 | `request_count` | 消费日志数 + 错误日志数。 |
| 成功数 | `success_count` | `type = LogTypeConsume` 的日志数。 |
| 错误数 | `error_count` | `type = LogTypeError` 的日志数。 |
| 成功率 | `success_rate` | `success_count / request_count`。无请求时为 0。 |
| 错误率 | `error_rate` | `error_count / request_count`。无请求时为 0。 |
| 额度 | `quota` | 消费日志的 `quota` 汇总。错误日志 quota 记 0。 |
| 输入 tokens | `prompt_tokens` | 消费日志 `prompt_tokens` 汇总。 |
| 输出 tokens | `completion_tokens` | 消费日志 `completion_tokens` 汇总。 |
| 计量 tokens | `metered_tokens` | 消费日志实际计量 token 汇总。 |
| 总 tokens | `total_tokens` | 展示口径，等于计量 token 语义。 |
| 平均延迟 | `avg_latency_ms` | `use_time` 换算毫秒后的平均值。 |
| P95 延迟 | `p95_latency_ms` | 当前候选日志内的 95 分位耗时。 |
| 首次使用时间 | `first_used_at` | 当前筛选范围内最早日志时间。 |
| 最后使用时间 | `last_used_at` | 当前筛选范围内最晚日志时间。 |
| 当前 RPM | `rpm` | 最近 60 秒请求数。 |
| 当前 TPM | `tpm` | 最近 60 秒总 tokens。 |

### Token 统计口径

必须使用统一表达式：

```sql
CASE
  WHEN metered_tokens IS NOT NULL THEN metered_tokens
  ELSE prompt_tokens + completion_tokens
END
```

约束：

- `metered_tokens = 0` 必须计为 0。
- 如果 prompt 或 completion 出现负数，聚合层不得产生负 token；应在 Go 层将异常结果归零或沿用现有日志层归一化规则。
- 订阅扣费日志中，`subscription_tokens_consumed` 已在写日志时进入 `metered_tokens`，分析页不再重复读取该字段作为总 tokens，避免双重口径。

### 延迟单位

当前 `logs.use_time` 语义为秒。接口对外统一返回毫秒：

```text
avg_latency_ms = AVG(use_time) * 1000
p95_latency_ms = percentile(use_time) * 1000
```

前端展示时：

- 小于 1000 ms 时显示 `xxx ms`；
- 大于等于 1000 ms 时显示 `x.x s`。

## 后端 API 设计

所有用户侧接口必须使用 `UserAuth`，并强制使用当前登录用户 ID 查询。

### 查询参数通用结构

```ts
type UsageAnalyticsQuery = {
  start_timestamp: number
  end_timestamp: number
  granularity?: 'hour' | 'day'
  group_by?: 'token' | 'model' | 'group' | 'stream' | 'status' | 'endpoint' | 'billing_source' | 'billing_tier' | 'modality'
  metric?: 'request_count' | 'total_tokens' | 'quota' | 'error_rate' | 'avg_latency_ms' | 'p95_latency_ms'
  token_ids?: string
  model_names?: string
  groups?: string
  endpoints?: string
  billing_sources?: string
  streams?: string
  statuses?: string
  limit?: number
}
```

参数规则：

- `start_timestamp`、`end_timestamp` 必填。
- 普通用户最大时间跨度为 31 天。
- 默认 `granularity = day`。
- 默认 `group_by = token`。
- 默认 `metric = total_tokens`。
- `limit` 默认 10，最大 50。
- 多选参数使用英文逗号分隔；后端负责 trim、去空、去重。
- 普通用户接口不接受 `user_id`、`username`。
- `token_ids` 中的每个 ID 必须校验属于当前用户；不属于当前用户的 ID 直接忽略或返回 400，推荐返回 400，避免用户误以为筛选生效。

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
        "group_key": "token:123",
        "group_label": "Production Key",
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
        "group_key": "token:123",
        "group_label": "Production Key",
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
        "avg_latency_ms": 810
      }
    ],
    "granularity": "day"
  }
}
```

### 分布与排行接口

```http
GET /api/usage-analytics/breakdown
```

用途：分布环形图、排行条形图、排行表。

响应结构可与 `summary.data.groups` 一致，但支持 `limit` 和 `sort`。

```json
{
  "success": true,
  "message": "",
  "data": {
    "groups": [],
    "total_groups": 18,
    "other": {
      "group_key": "other",
      "group_label": "Other",
      "request_count": 30,
      "total_tokens": 12000,
      "quota": 3000
    }
  }
}
```

规则：

- 图表默认取 Top 10。
- 超出部分合并为 `Other`。
- 表格可显示 Top 50。
- `Other` 不参与日志钻取。

### 交叉分析接口

```http
GET /api/usage-analytics/matrix
```

查询参数：

```ts
type UsageAnalyticsMatrixQuery = UsageAnalyticsQuery & {
  x_group_by: 'token' | 'model' | 'group' | 'endpoint' | 'billing_source'
  y_group_by: 'token' | 'model' | 'group' | 'endpoint' | 'billing_source'
}
```

响应：

```json
{
  "success": true,
  "message": "",
  "data": {
    "x": [
      { "key": "model:gpt-4.1-mini", "label": "gpt-4.1-mini" }
    ],
    "y": [
      { "key": "token:123", "label": "Production Key" }
    ],
    "cells": [
      {
        "x": "model:gpt-4.1-mini",
        "y": "token:123",
        "value": 100000,
        "request_count": 120,
        "success_count": 118,
        "error_count": 2,
        "total_tokens": 100000,
        "quota": 32000,
        "error_rate": 0.0167
      }
    ],
    "metric": "total_tokens"
  }
}
```

初版推荐固定提供两个矩阵视图：

- API Key × 模型。
- API Key × 请求端点。

### 日志钻取增强

增强现有：

```http
GET /api/log/self
GET /api/log/self/stat
```

新增查询参数：

```ts
{
  token_id?: number
  endpoint?: string
  billing_source?: string
  is_stream?: boolean
  status?: 'success' | 'error'
}
```

规则：

- `token_id` 必须校验属于当前用户。
- `status=success` 等价于 `type=LogTypeConsume`。
- `status=error` 等价于 `type=LogTypeError`。
- endpoint / billing_source 初版如基于 `Other` 解析，可只用于分析页 drilldown 后的用户侧过滤；若性能不足，后续结构化列优化。

## 后端实现设计

### 文件组织

建议新增：

```text
model/usage_analytics.go
controller/usage_analytics.go
controller/usage_analytics_test.go
model/usage_analytics_test.go
```

### 核心类型

```go
type UsageAnalyticsGroupBy string

const (
    UsageAnalyticsGroupByToken         UsageAnalyticsGroupBy = "token"
    UsageAnalyticsGroupByModel         UsageAnalyticsGroupBy = "model"
    UsageAnalyticsGroupByGroup         UsageAnalyticsGroupBy = "group"
    UsageAnalyticsGroupByStream        UsageAnalyticsGroupBy = "stream"
    UsageAnalyticsGroupByStatus        UsageAnalyticsGroupBy = "status"
    UsageAnalyticsGroupByEndpoint      UsageAnalyticsGroupBy = "endpoint"
    UsageAnalyticsGroupByBillingSource UsageAnalyticsGroupBy = "billing_source"
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
    Endpoints      []string
    BillingSources []string
    Streams        []bool
    Statuses       []int
    Limit          int
}
```

响应 DTO 可放在 controller 或 dto 包。若其他层也要复用，放入 `dto/usage_analytics.go`。

### 查询策略

#### 结构化列维度

以下维度可以使用 SQL 聚合：

- token
- model
- group
- stream
- status

查询必须通过白名单选择 group expression，不允许把前端传入的 `group_by` 直接拼接到 SQL。

示例映射：

```go
func usageAnalyticsGroupExpr(groupBy UsageAnalyticsGroupBy) (selectExpr string, groupExpr string, ok bool) {
    switch groupBy {
    case UsageAnalyticsGroupByToken:
        return "token_id, token_name", "token_id, token_name", true
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

#### `Other` JSON 维度

以下维度初版在 Go 层解析：

- endpoint
- billing_source
- billing_tier
- modality

原因：

- `logs.other` 是 TEXT JSON。
- SQLite、MySQL、PostgreSQL 的 JSON 函数与索引能力不一致。
- 项目要求三库兼容。

实现方式：

1. SQL 只筛选 `user_id`、`created_at`、`type`、基础列和必要过滤条件。
2. 读取候选日志的有限字段。
3. 使用 `common.UnmarshalJsonStr` 解析 `Other`。
4. 在 Go 内完成分组、聚合、排序、Top N 合并。

注意：业务代码不得直接调用 `encoding/json.Unmarshal`，必须使用 `common.UnmarshalJsonStr` 或项目 JSON wrapper。

### 候选日志限制

为避免单用户大窗口查询拖垮服务：

- 普通用户最大查询跨度：31 天。
- 默认时间范围：7 天。
- `Other` 维度 Go 内聚合时，候选日志上限建议为 50,000 条；超过时返回明确错误：`当前筛选范围数据量过大，请缩小时间范围或增加筛选条件`。
- 结构化 SQL 聚合不使用该候选日志上限，但仍受 31 天窗口限制。

### P95 计算

初版不使用数据库专属 percentile 函数。

实现：

- SQL 聚合提供 `avg_latency_ms`。
- P95 使用 Go 层计算。
- 对每个 group 收集 `use_time` 样本并排序。
- 样本数过大时允许固定上限采样，但必须在代码注释中说明近似口径；推荐第一版在 31 天用户侧窗口内直接精确计算。

P95 算法：

```text
index = ceil(0.95 * n) - 1
index 最小为 0，最大为 n - 1
```

### API Key 补充信息

当 `group_by = token` 时，需要补充 token 信息：

- `tokens.id`
- `tokens.name`
- `tokens.key` 的掩码结果
- `tokens.status`
- `tokens.group`
- `tokens.remain_quota`
- `tokens.unlimited_quota`

规则：

- 只能查询当前用户自己的 tokens。
- 返回 `masked_key`，不得返回完整 `key`。
- 已删除 token 或 token 表中查不到的历史日志，仍显示日志中的 `token_name`，并标记 `deleted = true`。

## 前端实现设计

### 文件组织

```text
web/default/src/features/usage-analytics/
  api.ts
  constants.ts
  types.ts
  index.tsx
  lib/
    chart-data.ts
    filters.ts
    format.ts
    insights.ts
  components/
    usage-analytics-filter-bar.tsx
    usage-analytics-summary-cards.tsx
    usage-trend-chart.tsx
    usage-breakdown-chart.tsx
    usage-ranking-chart.tsx
    usage-error-rate-chart.tsx
    usage-matrix-heatmap.tsx
    usage-breakdown-table.tsx
    usage-insights.tsx
```

路由：

```text
web/default/src/routes/_authenticated/usage-analytics/index.tsx
```

导航：

- `web/default/src/hooks/use-sidebar-data.ts` 增加 `Usage Analytics`。
- 图标建议使用 `ChartNoAxesCombined`、`BarChart3` 或项目当前 lucide 可用图标。

### React Query

查询 key 必须包含筛选条件：

```ts
['usage-analytics-summary', filters]
['usage-analytics-timeseries', filters]
['usage-analytics-breakdown', filters]
['usage-analytics-matrix', filters]
```

要求：

- 使用 `placeholderData` 保持切换筛选时图表稳定。
- 查询失败使用 `handleServerError` 或现有错误处理模式。
- 空数据必须显示友好空状态，不显示空白图表。

### URL 状态

筛选条件必须进入 URL search params：

- `start_timestamp`
- `end_timestamp`
- `group_by`
- `metric`
- `granularity`
- `token_ids`
- `model_names`
- `groups`

这样从 API Keys 页面跳转时可以预选 Key，刷新后也能保留分析上下文。

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

- 默认图表：堆叠面积图。
- X 轴：时间。
- Y 轴：当前 metric。
- Series：当前 group_by。
- 支持切换：面积图 / 折线图 / 堆叠柱状图。
- Top N 之外合并为 `Other`。

#### 分布图

- 默认环形图。
- 展示当前维度的 Top N 占比。
- Tooltip 显示请求数、Tokens、额度、错误率、占比。

#### 排行图

- 横向条形图。
- 默认按当前 metric 降序。
- 可切换 metric。

#### 错误率趋势图

- 双轴图：请求数 + 错误率。
- 错误率超过阈值时用 warning / destructive 色。

#### 热力图

第一版提供：

- API Key × 模型。
- API Key × 端点（如果 endpoint 维度已实现）。

指标可切换：

- Tokens
- 请求数
- 额度
- 错误率

### 洞察卡片

`lib/insights.ts` 根据聚合结果生成轻量提示。

第一版建议：

- 消耗最高的 API Key。
- 错误率最高的模型。
- 最近 24 小时增长最快的 API Key。
- 长时间未使用的 API Key。
- 只产生错误没有成功请求的 API Key。
- P95 延迟最高的聚合项。

洞察限制：

- 不做推送或告警。
- 不做复杂预测。
- 不使用价格或余额类指标。

## 权限与安全

必须满足：

- 所有 `/api/usage-analytics/*` 接口使用 `UserAuth`。
- 后端强制 `WHERE logs.user_id = 当前用户 ID`。
- 普通用户不能传入或影响 `user_id` / `username`。
- `token_id` 过滤必须校验 token 归属。
- 返回 API Key 时只返回 `masked_key`。
- 不返回完整 `key`。
- 不返回 `Other.admin_info`、渠道密钥、上游密钥或内部调试字段。
- 删除的 API Key 只显示历史 `token_name`，不能恢复密钥。
- endpoint、billing_source 等从 `Other` 解析出的值必须作为普通字符串处理，前端不得以 HTML 注入方式渲染。

## 三库兼容要求

- 优先使用 GORM 查询构造。
- 原始 SQL 必须限制在三库通用表达式。
- 不使用数据库 JSON 操作符。
- 不使用 PostgreSQL 专属 `FILTER`、`PERCENTILE_CONT`、`DISTINCT ON`。
- 不使用 MySQL 专属 JSON 函数或 `GROUP_CONCAT`。
- 不使用 SQLite 不支持的 `ALTER COLUMN`。
- 保留 `group` 相关字段时使用现有 `logGroupCol` / `commonGroupCol` 风格，避免保留字问题。

## 性能设计

### 初版策略

- 结构化维度使用 SQL 聚合。
- JSON 扩展维度使用 Go 内解析。
- 用户侧查询窗口限制为 31 天。
- 默认时间范围为 7 天。
- Top N 默认 10，最大 50。
- Matrix 默认限制 X/Y 各 Top 10。

### 后续优化触发条件

如果出现以下情况，应进入 Phase 3：

- 单用户 7 天日志量导致接口 P95 超过可接受范围。
- endpoint / billing_source 维度频繁查询超出候选日志上限。
- Matrix 查询明显拖慢数据库或 Go 进程。

优化方案：

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

新增 key 示例：

- `Usage Analytics`
- `Analyze your API usage across keys, models, groups, and endpoints`
- `Group by`
- `Metric`
- `API Key Usage`
- `Model Usage`
- `Group Usage`
- `Endpoint Usage`
- `Billing Source`
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
- `Usage Heatmap`
- `View Logs`
- `No usage data`
- `Try adjusting the time range or filters`
- `Deleted API Key`
- `Unknown Model`
- `Unknown Billing Source`

## 测试计划

### 后端测试

必须覆盖：

1. 用户只能看到自己的日志聚合。
2. `token_id` 筛选必须校验归属。
3. `metered_tokens = 0` 保留 0，不回退。
4. `metered_tokens IS NULL` 回退到 `prompt_tokens + completion_tokens`。
5. 成功日志和错误日志分别进入 `success_count`、`error_count`。
6. 错误率、成功率在 0 请求场景不产生 NaN。
7. 已删除 API Key 仍能显示历史 `token_name`，且 `deleted = true`。
8. `group_by = model`、`group`、`stream`、`status` 的聚合结果正确。
9. endpoint / billing_source 从 `Other` 解析失败时归入 unknown，不导致接口失败。
10. 查询时间范围生效。
11. 查询跨度超过 31 天时返回错误。
12. 日志钻取参数 `token_id`、`is_stream`、`status` 生效。
13. SQLite 测试必须通过；SQL 不得使用单库专属语法。

### 前端测试

必须覆盖：

1. URL search params 与筛选状态互相同步。
2. 从 API Keys 跳转时预选 API Key。
3. 空数据展示空状态。
4. 总览卡片格式化正确。
5. 图表数据转换正确处理 Top N 与 Other。
6. API Key 掩码展示，不出现完整 Key。
7. 点击「查看日志」生成正确 Usage Logs 跳转参数。
8. group_by 切换后表格第一列与图表标题变化正确。
9. 错误率、成功率、延迟格式化正确。
10. i18n 文案通过同步检查。

### 验证命令

后端：

```bash
go test ./model ./controller
```

前端：

```bash
cd web/default
bun run typecheck
bun test usage-analytics
bun run i18n:sync
```

如果项目没有 `bun test usage-analytics` 精确脚本，则运行相关新增测试文件对应的 Vitest 命令或现有 test 脚本。

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

- 用户可以默认看到最近 7 天按 API Key 聚合的用量。
- 用户可以切换到模型、分组、流式状态、调用结果维度。
- 用户可以点击聚合项查看日志明细。
- 不展示完整 API Key。
- 不泄露其他用户数据。

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

### Phase 3：性能优化

触发后交付：

- 结构化 `request_path`、`billing_source` 列。
- 写日志同步填充新列。
- 必要索引。
- 可选小时级聚合表。

验收：

- 常用查询在目标数据量下保持稳定。
- 旧日志仍可回退解析 `Other` 或显示 unknown，不破坏历史兼容。

## 验收标准

最终功能必须满足：

- 用户侧有完整「用量分析」页面。
- 默认按 API Key 聚合展示最近 7 天用量。
- 至少支持 API Key、模型、分组、流式状态、调用结果 5 种聚合维度。
- 展示请求数、Tokens、额度、成功率、错误率、平均延迟、P95 延迟、RPM、TPM。
- 至少包含总览卡片、趋势图、分布图、排行表。
- API Key 页面可以跳转到分析页并预选 Key。
- 分析页可以跳转到 Usage Logs 明细。
- 所有用户侧接口都只能访问当前用户数据。
- 前端不展示完整 API Key。
- 后端和前端相关测试通过。
- i18n 覆盖所有新增用户可见文案。

## 风险与处理

### 风险一：`Other` JSON 维度查询性能不足

处理：初版限制时间范围和候选日志数量；后续结构化 `request_path`、`billing_source`。

### 风险二：日志缺失导致统计不完整

处理：页面空状态说明「当前筛选范围暂无用量数据」；不回退到 `quota_data` 伪造 API Key 维度，因为 `quota_data` 没有 `token_id`。

### 风险三：API Key 名称可变导致历史展示混淆

处理：聚合主键使用 `token_id`；展示名称使用当前 token name。删除 Key 使用日志中的历史 `token_name` 并标记 deleted。

### 风险四：P95 计算成本较高

处理：初版在用户侧 31 天窗口内精确计算；数据量过大时先返回清晰错误或要求缩小范围。后续再做采样或聚合表。

### 风险五：多数据库 SQL 差异

处理：SQL 只用于基础过滤和通用聚合；JSON、P95、复杂维度在 Go 层处理。
