# 套餐制 token 统计去价格化规格

> 面向 AI 代理的工作者：本规格用于指导后续在 `C:/Users/34404/source/repos/new-api` 上清理用户侧价格制统计展示。实现前必须读取仓库根目录 `AGENTS.md` 与 `web/default/AGENTS.md`，并遵守 Go + Gin + GORM、React 19 + TypeScript + Bun、SQLite / MySQL / PostgreSQL 全兼容约束。若后续进入实现阶段，必须先创建实现计划并按 TDD 执行。

**目标：** 将普通用户可见的 API 用量统计从旧价格 / quota 口径切换为套餐制 token 口径，避免概览、日志、图表和模型目录继续暗示 API 请求按价格或余额扣费。

**架构：** 后端保留旧 `quota`、价格倍率、支付金额和套餐售价字段作为兼容与管理能力，但为统计接口和日志 `other` 增加明确 token 语义字段。前端 default 主题的 dashboard、usage logs、home demo、AI context 和模型目录统一使用 token / request / RPM / TPM / concurrency 展示；价格、倍率和成本分析只在管理员或支付场景出现。

**技术栈：** Go 1.25.1、Gin、GORM v2、SQLite / MySQL / PostgreSQL、React 19、TypeScript、TanStack Query、Rsbuild、Bun、i18next。

---

## 1. 背景与当前事实

当前项目已经按以下规格切换到套餐制 token 模型：

- `docs/superpowers/specs/2026-05-13-token-distribution-platform-spec.md`
- `docs/superpowers/specs/2026-05-15-account-balance-subscription-only-spec.md`

已生效的产品语义：

1. API 请求只消耗有效订阅套餐的 token 和并发额度。
2. 账户余额只用于购买套餐，不直接参与 API 请求扣费。
3. `users.quota`、`tokens.remain_quota`、`tokens.used_quota`、`logs.quota` 等字段保留为历史兼容字段，不能继续作为用户侧套餐额度口径。
4. 套餐售价、充值、支付订单、账户余额、渠道余额仍有金额语义，不属于本次清理对象。

当前残留问题集中在用户侧统计展示：dashboard 概览、模型统计图、usage logs、首页 demo、AI context 和 `/pricing` 仍大量使用 `quota`、`formatQuota()`、`Cost`、`Pricing`、`Model Price`、`$/M` 等价格制表达。

---

## 2. 目标与非目标

### 2.1 必须满足

- 普通用户侧 API 用量统计只展示 token、请求数、RPM、TPM、并发和套餐周期信息。
- dashboard 概览不再读取 `user.quota` / `user.used_quota` 作为「余额」或「额度」展示。
- dashboard 模型图表和用户排行不再按 `quota` 排序或换算 USD / CNY，改用 `token_used`。
- usage logs 列表不再把 API 请求显示为 `Cost`，改为展示本次 token 用量或订阅扣减 token。
- `/api/log/stat` 返回筛选区间累计 token 总量，避免前端继续用 `quota` 充当用量总计。
- 订阅日志 `other` 中写入明确的 token 字段，前端无需猜测旧 `subscription_total` / `subscription_consumed` 是否为 token 语义。
- `/pricing` 用户侧改为模型目录语义，不把价格 / 倍率作为普通用户的主要信息。
- i18n 同步替换 dashboard、logs、home、model directory 中的价格制文案。

### 2.2 非目标

- 不删除数据库中的 `quota` 字段。
- 不删除 `model_price`、`model_ratio`、`completion_ratio`、billing expression 等管理员成本核算字段。
- 不删除套餐售价 `SubscriptionPlan.PriceAmount` / `Currency`。
- 不删除充值、支付、账户余额、余额购买套餐、渠道余额查询能力。
- 不改动受保护的 New API / QuantumNous 品牌、版权头、模块路径或许可证信息。
- 不重写 classic 主题；classic 可继续依赖旧字段，除非后续另有明确需求。

---

## 3. 术语与展示边界

### 3.1 用户侧主口径

普通用户侧 API 用量只允许使用以下口径：

| 口径 | 来源 | 用途 |
|---|---|---|
| `token_limit` | `UserSubscription.TokenLimit` | 当前周期套餐 token 上限；`0` 只有在后端同时返回 `token_unlimited = true` 时才表示不限量 |
| `token_used` | `UserSubscription.TokenUsed` 或 `quota_data.token_used` | 当前周期或统计区间已用 token |
| `token_remaining` | `token_limit - token_used` | 当前周期剩余 token；前端是否显示 Unlimited 必须以 `token_unlimited` 为准 |
| `total_tokens` | 订阅日志 `subscription_tokens_consumed`，旧日志 fallback 到 `prompt_tokens + completion_tokens` | 日志筛选区间累计规范化 token |
| `request_count` / `count` | 用户或统计接口 | 请求数 |
| `rpm` | 最近 60 秒请求数 | 近期吞吐指标 |
| `tpm` | 最近 60 秒 token 数 | 近期吞吐指标 |
| `concurrency_limit` | `UserSubscription.ConcurrencyLimit` | 当前套餐实时并发上限 |

### 3.2 仍允许展示金额的场景

以下信息仍可展示金额：

- 套餐购买页的套餐价格。
- 账户余额、充值金额、支付订单金额、余额购买套餐金额。
- 管理员渠道余额查询。
- 管理员成本分析、模型倍率配置、billing expression 调试。
- 历史钱包或 legacy 审计视图，但必须标注为兼容 / 管理语义，不能出现在普通用户默认用量面板。

### 3.3 禁止在普通用户用量面板出现的表达

普通用户的 dashboard、usage logs、home demo 和模型目录默认视图不得出现：

- `Cost`
- `Total Cost`
- `Price`
- `Pricing`
- `Model Price`
- `Dynamic Pricing`
- `Per-token` 作为计费模式
- `$/M`
- `Credit remaining`
- `Balance depleted`
- `Low balance`
- `Total Quota`
- `Statistical quota`
- 将 `quota` 换算成 USD / CNY 的展示

例外：支付、订阅售价、账户余额购买套餐、管理员高级成本分析。

---

## 4. 后端接口与日志契约

### 4.1 `/api/log/stat` 增加区间累计 token

文件：

- `model/log.go`
- `controller/log.go`
- `web/default/src/features/usage-logs/types.ts`

当前 `model.Stat` 仅返回：

```go
type Stat struct {
    Quota int `json:"quota"`
    Rpm   int `json:"rpm"`
    Tpm   int `json:"tpm"`
}
```

修正为：

```go
type Stat struct {
    Quota       int `json:"quota"`
    TotalTokens int `json:"total_tokens"`
    Rpm         int `json:"rpm"`
    Tpm         int `json:"tpm"`
}
```

语义要求：

- `quota`：保留兼容，仍为 `sum(logs.quota)`，default 前端不作为主展示。
- `total_tokens`：按当前筛选条件和时间范围累计规范化 token 用量；订阅日志优先使用 `other.subscription_tokens_consumed`，旧日志和非订阅日志使用 `prompt_tokens + completion_tokens` 作为 legacy fallback。该字段是用户侧统计展示口径，不再使用 `quota` 换算金额。
- `rpm`：最近 60 秒请求数，仍受当前筛选条件影响。
- `tpm`：最近 60 秒 token 数，仍受当前筛选条件影响；cache read、cache creation、Claude cache 5m / 1h、OpenAI Responses cached tokens、Gemini cached content 等已经进入订阅扣减的 token，必须通过 `subscription_tokens_consumed` 纳入统计，且不得重复计数。

`SumUsedQuota` 应拆分为两个聚合：

1. 区间聚合：受 `start_timestamp` / `end_timestamp` 影响，计算 `quota` 和 `total_tokens`。`total_tokens` 对订阅日志聚合 `subscription_tokens_consumed`，字段缺失时退回 `prompt_tokens + completion_tokens`。
2. 近期聚合：固定 `created_at >= now - 60s`，计算 `rpm` 和 `tpm`；`tpm` 使用同一规范化 token 口径，避免与 `total_tokens` 展示口径不一致。

数据库兼容要求：

- 不使用 MySQL 专属 `IFNULL` 作为唯一实现。
- 不在统计查询中直接从 `logs.other` 做跨数据库 JSON 提取。`logs.other` 是 TEXT JSON 字段，SQLite、MySQL、PostgreSQL 的 JSON 提取语法和空字符串处理不同。
- 实现必须在写日志时把规范化 token 写入可直接求和的数值字段：优先复用 `logs.prompt_tokens + logs.completion_tokens` 以外的新增数值字段；若不新增表字段，则必须在 `LogQuotaData(... tokenUsed)` 和 `/api/log/stat` 使用同一个 Go 侧 helper 解析 `other.subscription_tokens_consumed` 并做分页 / 批量聚合，且三库测试必须覆盖空 `other`、字段缺失和旧日志 fallback。
- `total_tokens` 和 `tpm` 必须复用同一个规范化 token helper。
- `group` 条件继续使用现有 `logGroupCol`。

控制器响应示例：

```json
{
  "success": true,
  "message": "",
  "data": {
    "quota": 123456,
    "total_tokens": 987654,
    "rpm": 12,
    "tpm": 34567
  }
}
```

前端 `LogStatistics` 增加：

```ts
export interface LogStatistics {
  quota: number
  total_tokens: number
  rpm: number
  tpm: number
}
```

`DEFAULT_LOG_STATS` 同步增加 `total_tokens: 0`。

### 4.2 `quota_data` API 保留字段，前端改用 `token_used`

文件：

- `model/usedata.go`
- `controller/usedata.go`
- `web/default/src/features/dashboard/types.ts`
- `web/default/src/features/dashboard/lib/charts.ts`

`QuotaData` 当前已有：

```go
type QuotaData struct {
    TokenUsed int `json:"token_used"`
    Count     int `json:"count"`
    Quota     int `json:"quota"`
}
```

本次保留后端结构，但 default 前端必须改用 `token_used`；后端写入 `quota_data.token_used` 时也必须使用规范化 token 口径。

- `quota` 继续返回，兼容 classic 和历史调用方。
- `token_used` 对订阅日志必须等于实际套餐扣减 token；旧日志和非订阅日志退回 `prompt_tokens + completion_tokens`。
- default 前端 dashboard 所有用量排序、总计、tooltip 使用 `token_used`。
- 图表可继续显示请求数 `count`，但不能再用 `quota` 表示用量。

### 4.3 订阅日志写入明确 token 字段

文件：

- `service/log_info_generate.go`
- `relay/common` 中 `RelayInfo` 相关字段定义所在文件
- `web/default/src/features/usage-logs/types.ts`

当 `billing_source == "subscription"` 时，`other` 必须写入明确 token 字段：

```json
{
  "billing_source": "subscription",
  "subscription_id": 123,
  "subscription_plan_id": 2,
  "subscription_plan_title": "Basic",
  "subscription_token_limit": 1000000000,
  "subscription_token_used": 123456,
  "subscription_token_remaining": 999876544,
  "subscription_tokens_consumed": 2048,
  "subscription_pre_consumed": 3000,
  "subscription_post_delta": -952
}
```

语义要求：

- `subscription_tokens_consumed` 表示本次请求最终扣减的 token，不能为负数。
- `subscription_token_used` 表示结算完成后的当前周期累计已用 token。
- `subscription_token_remaining` 表示结算完成后的剩余 token；不限量套餐必须返回 `subscription_token_unlimited: true`，并将 `subscription_token_remaining` 置为 `0`。
- 旧字段 `subscription_total`、`subscription_used`、`subscription_remain`、`subscription_consumed` 保留兼容，但前端必须优先读取新字段。

实现阶段必须先定位当前预扣和结算路径中的 `RelayInfo.SubscriptionAmountTotal` / `RelayInfo.SubscriptionAmountUsedAfterPreConsume` 赋值来源，确认这些字段当前是否已经承载 token 语义：

- 若已经是 token 语义：新增 token 别名字段，旧字段保留。
- 若仍是旧 amount 语义：必须在预扣和结算路径中增加 token 字段，不得把 amount 字段误标为 token。

### 4.4 `/api/subscription/self` 必须返回概览聚合

文件：

- `controller/subscription.go`
- `model/subscription.go`
- `web/default/src/features/subscriptions/types.ts`
- `web/default/src/features/subscriptions/api.ts`

当前前端已有 `getSelfSubscriptionFull()`，类型为：

```ts
export interface SelfSubscriptionData {
  subscriptions: UserSubscriptionRecord[]
  all_subscriptions: UserSubscriptionRecord[]
}
```

修正后必须扩展响应，避免 dashboard 在多个页面重复实现订阅额度聚合，同时保留旧调用方依赖的兼容字段：

```ts
export interface SelfSubscriptionSummary {
  active_count: number
  subscription_id?: number
  plan_id?: number
  primary_plan_title?: string
  token_limit: number
  token_used: number
  token_remaining: number
  token_unlimited: boolean
  concurrency_limit: number
  next_reset_time?: number
  end_time?: number
}

export interface SelfSubscriptionData {
  billing_preference: string
  subscriptions: UserSubscriptionRecord[]
  all_subscriptions: UserSubscriptionRecord[]
  summary: SelfSubscriptionSummary
}
```

聚合与兼容规则：

- 响应必须继续包含 `billing_preference`、`subscriptions`、`all_subscriptions`，新增 `summary` 不得破坏 classic 主题和旧调用方。
- `summary` 必须与请求扣费层使用同一套有效订阅筛选、排序、不限量判断和并发口径；实现时应抽取 helper 复用 `PreConsumeUserSubscriptionByUnits` 的选择规则，或把该选择规则集中到 model 层供预扣与 self API 共用。
- 当前请求层按排序选择单个可用分销订阅，不合并多个订阅；因此 `summary.token_limit`、`summary.token_used`、`summary.token_remaining`、`summary.concurrency_limit`、`summary.end_time` 必须来自后端实际会用于请求扣费的主订阅。
- 如果后续产品需要展示跨订阅汇总，必须新增 `aggregate_token_limit`、`aggregate_token_used`、`aggregate_token_remaining` 等字段；不得用跨订阅求和覆盖当前 `summary` 主口径。
- `token_limit == 0` 只有在后端请求层明确判定为不限量的订阅上才令 `token_unlimited = true`；历史 `token_limit = 0` 订阅、非分销订阅或 legacy amount 订阅不得显示为 Unlimited。
- 非不限量主订阅：`token_remaining = max(0, token_limit - token_used)`。
- `next_reset_time` 取主订阅下一次 reset 时间。
- 无有效可扣费订阅时返回 `active_count = 0`、`token_limit = 0`、`token_used = 0`、`token_remaining = 0`、`token_unlimited = false`、`concurrency_limit = 0`。

---

## 5. 前端改造规格

### 5.1 Dashboard 概览卡片 token-only

文件：

- `web/default/src/features/dashboard/components/overview/summary-cards.tsx`
- `web/default/src/features/dashboard/hooks/use-dashboard-config.tsx`
- `web/default/src/features/dashboard/api.ts`
- `web/default/src/features/subscriptions/api.ts`
- `web/default/src/features/subscriptions/types.ts`

当前问题：

- `remainQuota = Number(user?.quota ?? 0)`
- `usedQuota = Number(user?.used_quota ?? 0)`
- `usage[index] += Number(item.quota) || 0`
- `formatQuota(usedQuota)` / `formatQuota(recentUsage)`
- 健康状态文案 `Low balance` / `Balance depleted`
- 根据 `display_in_currency` 拼接 currency label

修正后卡片：

| 卡片 | 数据来源 | 展示 |
|---|---|---|
| 套餐剩余 Token | `/api/subscription/self.summary` 主订阅口径 | `formatTokens(token_remaining)` 或 `Unlimited` |
| 本周期已用 Token | `/api/subscription/self.summary` 主订阅口径 | `formatTokens(token_used)` |
| 最近 24h Token 用量 | `/api/data/self` 的规范化 `token_used` | `formatTokens(sum(token_used))` |
| 请求数 | `user.request_count` 或最近 24h `count` | `formatNumber(...)` |

行为要求：

- 删除 `getCurrencyLabel()`、`isCurrencyDisplayEnabled()` 和 `formatQuota()` 在该组件中的使用。
- sparkline 的 usage 使用 `item.token_used`。
- 删除 balance 回推趋势；如果保留趋势，只展示 token usage / requests。
- 健康状态改为 token 语义：
  - `healthy`：有有效订阅且不限量，或剩余 token 可覆盖近期消耗。
  - `caution`：预计剩余不足 3 天。
  - `critical`：无有效订阅或剩余 token 为 0。
- 文案替换：
  - `Usage at a glance` 可保留。
  - `Monitor balance, usage, and request volume` 改为 `Monitor subscription tokens and request volume`。
  - `Low balance` 改为 `Low token balance` 或 `Low tokens`。
  - `Balance depleted` 改为 `Tokens depleted` 或 `Subscription required`。

`useSummaryCardsConfig` 入参改为 token-only：

```ts
export function useSummaryCardsConfig(totals: {
  recentTokensDisplay: string
  cycleTokensDisplay: string
  requestCountDisplay: string
})
```

不得再传 `currencyEnabled` / `currencyLabel`。

### 5.2 Dashboard 模型统计图和用户排行改用 `token_used`

文件：

- `web/default/src/features/dashboard/lib/charts.ts`
- `web/default/src/features/dashboard/lib/stats.ts`
- `web/default/src/features/dashboard/components/models/log-stat-cards.tsx`
- `web/default/src/features/dashboard/components/models/consumption-distribution-chart.tsx`
- `web/default/src/features/dashboard/components/users/user-charts.tsx`
- `web/default/src/features/dashboard/types.ts`

范围边界：`UserCharts` 是管理员专用视图，继续调用管理员接口 `/api/data/users`，按所选时间范围聚合全站用户；普通用户 dashboard 不展示全站用户排行，也不得误改为 `/api/data/self`。本节只改变管理员用户排行的用量口径和文案，不改变权限边界。

修正要求：

- 删除或停止使用 `renderQuotaCompat()`。
- 删除 `getCurrencyDisplay()`、`quotaPerUnit` 在 dashboard chart 中的使用。
- 聚合结构中的 `quota` 不再参与排序、total、tooltip。
- 原 `rawQuota` 字段改为 `rawTokens` 或 `tokenUsed`。
- 原 yField `Usage` 必须改名为 `Tokens`，值为规范化 token 数。
- `totalQuotaDisplay` 改为 `totalTokensDisplay`。
- `Quota Distribution` 改为 `Token Usage Distribution`。
- `User Consumption Ranking` 改为 `User Token Usage Ranking`。
- `User Consumption Trend` 改为 `User Token Usage Trend`。
- `Total Quota` / `Statistical quota` 改为 `Total Tokens Used` / `Tokens used in selected range`。

图表排序要求：

- 模型 token 用量趋势和分布按 `token_used` 降序。
- 用户排行和用户趋势按 `token_used` 聚合、排序和展示。
- 请求数分布和排行继续按 `count` 降序。

格式化要求：

- token 数使用 `formatTokens()` 或 `Intl.NumberFormat`，不得使用 `formatQuota()`。
- tooltip 中总计显示 `Total:` + token 数。

### 5.3 Usage logs 顶部统计改用 `total_tokens`

文件：

- `web/default/src/features/usage-logs/components/common-logs-stats.tsx`
- `web/default/src/features/usage-logs/constants.ts`
- `web/default/src/features/usage-logs/types.ts`

修正要求：

- 第一个 badge label 从 `Usage` 改为 `Tokens` 或 `Total Tokens`。
- value 从 `formatLogQuota(stats?.quota || 0)` 改为 `formatTokens(stats?.total_tokens || 0)`。
- `sensitiveVisible` 仍控制可见性。
- RPM / TPM 保留。

### 5.4 Usage logs 表格列改为 token usage

文件：

- `web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx`
- `web/default/src/features/usage-logs/components/dialogs/details-dialog.tsx`
- `web/default/src/features/usage-logs/lib/format.ts`
- `web/default/src/features/usage-logs/types.ts`

表格列要求：

- Token Usage 列必须使用独立的 `accessorFn` 或派生字段作为展示与排序依据，值来自 `getLogTokenUsage(log, other)`；不得继续使用 `accessorKey: 'quota'` 作为普通用户用量列的排序依据。
- legacy `quota` 只保留在管理员详情或 legacy audit 字段中。
- 对 `billing_source == "subscription"` 的日志：
  - 展示 `Subscription` badge。
  - tooltip 展示 `subscription_tokens_consumed`。
  - 若新字段缺失，fallback 到 `subscription_consumed`。
  - 最后才 fallback 到 `prompt_tokens + completion_tokens`。
  - 不用 `formatLogQuota(quota)`。
- 对非订阅或 legacy consume 日志：
  - 普通用户显示 token 总量，即 `getLogTokenUsage(log, other)` 的结果。
  - 管理员可在高级详情中查看 legacy quota / cost。
- 普通用户表格内联 Details 摘要（`buildDetailSegments` 等）也必须使用 token-only 文案，不展示 price、ratio、`$/M`、`Dynamic Pricing`；这些成本摘要仅管理员可见。
- 普通用户 Token 列 / `TokenNameCell` 不得展示 `group_ratio`、`user_group_ratio` 或任何 `x` 倍率；`getGroupRatioText` 只能在管理员成本审计或高级详情中使用。普通用户 Token 列只展示 token name、允许展示的分组名和 token 用量。

详情弹窗要求：

- `Token Breakdown` 保留并作为主要区块。
- `Billing Details` 对普通用户默认不显示价格、倍率、`$/M`、`Total Cost`。
- 管理员可见的成本区块改名为 `Cost Analysis` 或 `Legacy Billing Audit`。
- `DynamicPricingBreakdown` 仅管理员可见。
- 违规扣费 `Violation Fee` 如果仍是旧 quota 语义，普通用户文案应改为 `Violation Deduction`，并优先展示 token；无法准确映射 token 时仅管理员显示 legacy fee quota。

新增 helper：

```ts
function getLogTokenUsage(log: UsageLog, other: LogOtherData | null): number {
  if (other?.subscription_tokens_consumed != null) {
    return Math.max(0, Number(other.subscription_tokens_consumed) || 0)
  }
  if (other?.subscription_consumed != null) {
    return Math.max(0, Number(other.subscription_consumed) || 0)
  }
  return getLegacyPromptCompletionTokens(log)
}

function getLegacyPromptCompletionTokens(log: UsageLog): number {
  return Math.max(0, (log.prompt_tokens || 0) + (log.completion_tokens || 0))
}
```

`getLegacyPromptCompletionTokens` 只作为旧日志兼容 fallback；订阅新日志和统计接口不得绕过 `subscription_tokens_consumed`。

`LogOtherData` 增加字段：

```ts
subscription_token_limit?: number
subscription_token_used?: number
subscription_token_remaining?: number
subscription_token_unlimited?: boolean
subscription_tokens_consumed?: number
```

### 5.5 AI context 删除默认成本展示

文件：

- `web/default/src/components/ai-elements/context.tsx`

修正要求：

- 默认展示 token 使用情况：input、output、reasoning、cached、total。
- 删除默认 `Total cost`、input cost、output cost、reasoning cost、cache cost。
- 如调用方传入自定义 children，可由调用方自行决定是否展示成本；组件默认不展示 USD。

### 5.6 首页 demo 删除 cost

文件：

- `web/default/src/features/home/components/hero-terminal-demo.tsx`

修正要求：

- 删除 `cost $...`。
- 替换为 token / latency / plan 语义，例如：

```text
tokens 1,248 · latency 0.82s · plan Basic
```

不得用模拟单价计算成本。

### 5.7 `/pricing` 改为模型目录，并脱敏公开 API

文件：

- `controller/pricing.go`
- `model/pricing.go`
- `web/default/src/features/pricing/*`
- `web/default/src/features/pricing/types.ts`
- `web/default/src/features/pricing/api.ts`
- 导航中引用 `/pricing` 的文件
- i18n locale 文件

产品决策：普通用户侧 `/pricing` 不再作为价格页，而是模型目录。

后端 API 要求：

- 非管理员访问 `/api/pricing` 时，响应不得包含 `model_ratio`、`model_price`、`completion_ratio`、`cache_ratio`、`create_cache_ratio`、`image_ratio`、`audio_ratio`、`audio_completion_ratio`、`billing_expr`、`group_ratio` 等成本和倍率字段。
- 公开 `/api/pricing` 脱敏后，倍率同步不得继续把脱敏响应当作有效成本源。实现必须同步迁移 `controller/ratio_sync.go` 和 `web/default/src/features/system-settings/models/constants.ts`：默认与推荐同步入口改为 `/api/ratio_config`，或新增仅管理员 / 机器认证可访问的成本 DTO 接口。
- 管理员仍可通过管理员视图、`/api/ratio_config` 或受控成本接口获取成本分析所需字段；不得破坏现有倍率同步、模型倍率配置和 billing expression 管理能力。
- 后端测试必须覆盖普通用户 / 未登录访问不返回成本字段，管理员成本接口仍能获取成本分析字段，倍率同步不会从脱敏 `/api/pricing` 静默同步空成本数据。

前端要求：

- 页面标题从 `Pricing` 改为 `Model Directory`。
- 导航文案从 `Pricing` 改为 `Models` 或 `Model Directory`。
- 用户侧默认表格列展示：
  - model name
  - provider / vendor
  - endpoint
  - context length
  - modalities
  - capabilities
  - tags / status
- 普通用户侧必须清理 `pricing-toolbar.tsx` 的价格显示模式、`pricing-sidebar.tsx` 的 Pricing Type 过滤、`model-card.tsx`、`pricing-columns.tsx`、`model-details.tsx`、模型详情路由中的 Base Price、Pricing by Group、DynamicPricingBreakdown、Price sort keys 和导航 quick action 文案。
- 价格列、倍率、dynamic pricing breakdown 仅管理员可见，或移到管理员模型配置 / 成本分析入口。
- 搜索和筛选继续保留，但筛选维度不应以价格为主。
- `Review model rates`、`compare pricing` 等文案全部替换为模型能力查询语义。

---

## 6. i18n 要求

文件：

- `web/default/src/i18n/locales/en.json`
- `web/default/src/i18n/locales/zh.json`
- `web/default/src/i18n/locales/fr.json`
- `web/default/src/i18n/locales/ja.json`
- `web/default/src/i18n/locales/ru.json`
- `web/default/src/i18n/locales/vi.json`
- `web/default/src/i18n/static-keys.ts`（如新增静态 key 需要登记）

必须新增或替换以下用户侧 key：

- `Subscription tokens remaining`
- `Tokens used this cycle`
- `Tokens used in the last 24 hours`
- `Monitor subscription tokens and request volume`
- `Low token balance`
- `Tokens depleted`
- `Subscription required`
- `Total Tokens Used`
- `Tokens used in selected range`
- `Token Usage Distribution`
- `Deducted Tokens`
- `Model Directory`
- `Browse available models and capabilities`

必须保留以下金额相关 key 的使用场景：

- 套餐售价
- 账户余额
- 充值
- 支付订单
- 渠道余额
- 管理员成本分析

翻译要求：

- 默认英文 key 可作为 fallback。
- 中文翻译使用自然中文，不用「花费」「价格」描述 API token 用量。
- 其他语言必须补齐，不能留下英文以外 locale 缺 key。

---

## 7. 数据兼容与迁移

无需破坏性数据库迁移。

兼容策略：

- `logs.quota` 继续写入，供旧接口、classic 或管理员 legacy 审计使用。
- `quota_data.quota` 继续写入，供兼容使用。
- default 前端不再依赖这些字段作为普通用户展示主口径。
- 新增 JSON 字段写入 `logs.other`，无需表结构迁移。
- `/api/subscription/self` 的 summary 扩展必须保持旧 `billing_preference`、`subscriptions`、`all_subscriptions` 字段不变。

---

## 8. 测试方案

### 8.1 后端测试

新增或修改测试：

1. `model/log` 统计测试：
   - 插入多条 consume log，包含不同 `prompt_tokens`、`completion_tokens`、`quota`、`created_at` 和 `other.subscription_tokens_consumed`。
   - 查询指定时间范围。
   - 断言订阅日志的 `total_tokens` 优先累计 `subscription_tokens_consumed`，旧日志 fallback 到 `prompt_tokens + completion_tokens`。
   - 断言 cache read/cache creation、Claude cache 5m / 1h、Responses cached tokens、Gemini cached content 等已进入 `subscription_tokens_consumed` 的用例不会被低估或重复计数。
   - 断言 `quota == sum(quota)` 仍兼容返回。
   - 断言 `rpm` / `tpm` 只统计最近 60 秒，且 `tpm` 使用同一规范化 token 口径。

2. `quota_data.token_used` 写入测试：
   - 通过 `RecordConsumeLog` / `LogQuotaData` / `SaveQuotaDataCache` 或等价入口构造订阅消费日志，使 `other.subscription_tokens_consumed` 大于 `prompt_tokens + completion_tokens`。
   - 断言 `quota_data.token_used` 使用规范化 token，而不是 `prompt_tokens + completion_tokens`。
   - 断言新字段缺失时 fallback 到 `prompt_tokens + completion_tokens`。
   - 断言 `quota_data.quota` 仍按旧 `quota` 累加。

3. `controller/log` 响应测试：
   - `/api/log/stat` 和 self stat 均返回 `total_tokens`。
   - 旧字段 `quota` 仍存在。

4. `service/log_info_generate` 订阅 other 测试：
   - 构造 distributor token subscription billing 的 `RelayInfo`。
   - 断言输出包含 `subscription_tokens_consumed`、`subscription_token_used`、`subscription_token_remaining`、`subscription_token_unlimited`。
   - 断言本次消耗为负数时归零。
   - 断言 legacy amount 订阅不会被误写成 `subscription_token_*` 权威字段。

5. `/api/subscription/self` summary 测试：
   - 响应同时包含 `billing_preference`、`subscriptions`、`all_subscriptions`、`summary`。
   - 只选择 active 且未过期、后端请求层实际可扣费的分销订阅。
   - 有限套餐 `token_remaining` 负数归零。
   - 只有请求层判定为不限量的试用 / 不限量订阅返回 `token_unlimited = true`。
   - 历史 `token_limit = 0` 订阅、非分销订阅或 legacy amount 订阅不得显示为 Unlimited。
   - `concurrency_limit`、`end_time`、`next_reset_time` 与请求扣费选择策略一致。

6. `/api/pricing` 脱敏与倍率同步测试：
   - 普通用户和未登录访问不返回价格、倍率、billing expression、group ratio 等成本字段。
   - 管理员成本接口仍能获取成本分析所需字段。
   - `controller/ratio_sync.go` 默认入口和前端系统设置默认选项不再依赖脱敏 `/api/pricing`。
   - 从脱敏 `/api/pricing` 同步倍率时必须明确失败或跳过，不能静默写入空成本配置。

运行建议：

```bash
go test ./model ./controller ./service -run 'Test.*(LogStat|Subscription.*Other|SubscriptionSelf|SelfSummary|Pricing|RatioSync|QuotaData|TotalTokens)'
```

### 8.2 前端测试

新增或修改测试：

1. dashboard chart 纯函数测试：
   - 输入数据同时包含 `quota` 与 `token_used`。
   - 让 `quota` 排序与 `token_used` 排序相反。
   - 断言模型图表和管理员用户排行的排序、tooltip 总计、`totalTokensDisplay` 均使用 `token_used`。
   - 断言 `/api/data/users` 仍需管理员权限，普通用户 dashboard 不出现全站用户排行。

2. summary 聚合测试：
   - 输入 self summary，包括有限 token、无有效订阅和不限量套餐。
   - 断言 remaining、used、unlimited、health level 正确。
   - 断言 legacy `token_limit = 0` 订阅不会被前端显示为 Unlimited。

3. usage logs helper 测试：
   - 有 `subscription_tokens_consumed` 时优先使用。
   - 缺新字段但有 `subscription_consumed` 时 fallback。
   - 都缺失时使用 `getLegacyPromptCompletionTokens(log)`。
   - Token Usage 列排序使用 helper 结果，不使用 `quota`。

运行建议：

```bash
cd web/default
bun run typecheck
bun test src/features/dashboard/lib/charts.test.ts src/features/usage-logs/lib/format.test.ts
```

实现计划必须先读取 `web/default/package.json` 确认实际测试脚本；不得虚构未观察到的测试命令或通过结果。

### 8.3 手动验收

使用普通用户账号：

- dashboard 概览不出现 Cost、Price、Credit remaining、Balance depleted、USD/CNY 用量换算。
- 有套餐时显示当前周期 token limit / used / remaining。
- 无套餐时提示 Subscription required。
- usage logs 表格列显示 token usage，不显示 Cost。
- 日志详情默认先显示 token breakdown。
- 模型目录页面不显示普通用户价格列。

使用管理员账号：

- 支付、账户余额、充值、套餐售价仍显示金额。
- 渠道余额仍可查看。
- 管理员成本分析或 legacy billing audit 可查看倍率 / price / quota，不影响普通用户视图。

---

## 9. 实施分阶段建议

### 阶段 1：后端 token 契约补强

- `/api/log/stat` 返回规范化 `total_tokens`，`tpm` 使用同一规范化 token 口径。
- 订阅日志 `other` 写入明确 token 字段。
- `quota_data.token_used` 写入规范化 token 口径。
- `/api/subscription/self` 增加 summary 并保留兼容字段。
- `/api/pricing` 对普通用户和未登录访问脱敏成本字段，并迁移倍率同步默认入口，避免依赖脱敏响应。

### 阶段 2：用户侧止血

- Dashboard 概览切到订阅 token summary。
- Dashboard 模型图表和用户排行切到 `token_used`。
- Usage logs 顶部统计和表格列切到 token。
- 首页 demo 和 AI context 删除默认 cost。
- 补齐 i18n。

### 阶段 3：模型目录去价格化

- `/pricing` 改为 `Model Directory`。
- 普通用户隐藏价格 / 倍率列。
- 管理员保留成本分析入口。

### 阶段 4：清理残余文案与文档

- 清理 default 前端用户侧 `Cost`、`Price`、`Pricing`、`Credit`、`Balance` 在 API 用量语境中的残留。
- 保留支付、套餐售价、账户余额语境中的金额文案。
- 如 README 或 docs 仍把 API 请求描述为价格制计费，后续另立文档清理任务。

---

## 10. 验收标准

完成后必须满足：

- 普通用户 dashboard 不再以旧 quota / 钱包余额展示 API 可用额度。
- 普通用户 usage logs 不再以 Cost / Total Cost 展示请求消耗。
- 模型统计图和用户排行按 `token_used` 展示和排序。
- `/api/log/stat` 返回筛选区间规范化 `total_tokens`，包含订阅日志的 `subscription_tokens_consumed`。
- `/api/subscription/self` 返回 `summary`，并保留 `billing_preference`、`subscriptions`、`all_subscriptions` 兼容字段。
- `/api/pricing` 对普通用户和未登录访问不返回价格、倍率和 billing expression 等成本字段。
- 订阅日志包含明确 token 字段，前端优先使用这些字段。
- 支付、套餐售价、账户余额、渠道余额和管理员成本分析未被误删。
- 所有新增或修改的前端文案已补齐 6 种 locale。
- 定向后端测试、前端 typecheck 和相关前端测试通过。
