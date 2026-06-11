# 渠道 token 扣费倍率规格

> 面向 AI 代理的工作者：本规格用于指导后续在 `C:/Users/34404/source/repos/new-api` 上实现「不同渠道 token 扣费倍率」与「套餐界面展示不同渠道等价 token 量」。实现前必须读取仓库根目录 `AGENTS.md` 与 `web/default/AGENTS.md`，遵守 Go + Gin + GORM、React 19 + TypeScript + Bun、SQLite / MySQL / PostgreSQL 全兼容约束。若后续进入实现阶段，必须先创建实现计划并按 TDD 执行。

**目标：** 允许管理员为不同渠道配置 token 扣费倍率。请求使用某个渠道时，可信上游 usage token 先按渠道倍率换算为渠道计费 token，再扣减订阅套餐 token 与 API Key token cap。套餐界面同时展示不同渠道下的等价可用 token 量，而不是只展示统一 token 量。

**核心决策：** 套餐仍只存一个标准 token 池；渠道倍率是消费规则，不是套餐权益副本。后端按当前启用渠道倍率派生等价 token 展示值，并在请求预扣前冻结本次请求的倍率快照，用于预扣、结算和日志。为保证预扣与最终渠道一致，token-billed 请求的 retry 只能切换到相同 token 扣费倍率的渠道；没有相同倍率可用渠道时停止 retry。

**架构：** 在渠道模型上新增显式倍率字段；在 relay 预扣前从已选渠道上下文冻结请求级倍率；在订阅 token 预扣与结算链路统一应用渠道倍率；Codex Pro 的订阅额外调整与 API Key token cap 分层处理；日志记录原始 usage token、渠道计费 token、订阅最终扣费 token 和倍率快照；套餐列表、公开套餐与用户自助订阅摘要返回后端派生的 `channel_token_equivalents`；default 前端展示等价 token，并在渠道配置表单中提供倍率设置。

**技术栈：** Go 1.25.1、Gin、GORM v2、SQLite / MySQL / PostgreSQL、React 19、TypeScript、TanStack Query、Rsbuild、Bun、i18next。

---

## 1. 背景与当前事实

当前项目已经把 API 请求的主资金来源收敛为订阅套餐 token：

```text
API 请求 -> 订阅 token 预扣 -> 上游请求 -> 按实际 usage 结算订阅 token
```

已静态确认的当前事实：

1. `model/subscription.go` 中，套餐模板使用 `SubscriptionPlan.MonthlyTokenLimit` 表示标准 token 额度；用户订阅实例使用 `UserSubscription.TokenLimit` / `TokenUsed` 表示可用额度与已用额度。
2. `model/subscription.go` 的 `PreConsumeUserSubscriptionByUnits()` 负责订阅预扣，最终通过 `applySubscriptionPreConsumeUpdateTx()` 更新 `user_subscriptions.token_used`。
3. `model/subscription.go` 的 `PostConsumeUserSubscriptionTokenDelta()` / `postConsumeUserSubscriptionDeltaTx()` 负责结算后的正向补扣或负向退款。
4. `service/funding_source.go` 的 `SubscriptionFunding.PreConsume()` 调用 `model.PreConsumeUserSubscriptionByUnits()`，`SubscriptionFunding.Settle()` 在 token 计费模式下调用 `model.PostConsumeUserSubscriptionTokenDelta()`。
5. `service/billing_session.go` 的 `BillingSession.SettleWithInput()` 在资金来源为 subscription 时，使用 `input.SubscriptionTokens - s.preConsumedSubscription` 计算结算差额。
6. `service/text_quota.go` 的 `PostTextConsumeQuota()` 通过 `SubscriptionMeteredTokens(usage)` 得到 usage token，并经 `subscriptionTokensForTextSettle()` 传入 `SettleBillingWithInput()`。
7. `controller/relay.go` 当前先执行 `service.PreConsumeBilling()` 和 API Key token cap 预扣，再进入 retry loop 并调用 `relayInfo.InitChannelMeta(c)`；规格要求后续实现调整冻结时序，避免预扣拿不到渠道倍率。
8. `service/log_info_generate.go` 已记录订阅 token 字段，包括 `subscription_token_limit`、`subscription_token_used`、`subscription_token_remaining`、`subscription_tokens_consumed`。
9. `controller/subscription.go` 的 `GetSubscriptionPlans()`、`GetPublicSubscriptionPlans()`、`GetSubscriptionSelf()` 分别面向用户套餐列表、公开套餐列表和用户当前订阅摘要。
10. `web/default/src/features/wallet/components/subscription-plans-card.tsx` 当前负责套餐卡片展示；`web/default/src/features/wallet/lib/subscription-display.ts` 和 `web/default/src/features/subscriptions/lib/format.ts` 负责订阅额度展示格式化。
11. `web/default/src/features/channels/*` 已有渠道类型、渠道表单和渠道编辑抽屉，适合承载管理员配置项。

当前缺口：

- 套餐 token 目前是全局统一口径，无法表达「同样 100 万标准 token，在高成本渠道只能用约 50 万上游 token，在低成本渠道可用约 200 万上游 token」。
- 套餐界面只展示统一 token 量，用户无法预估不同渠道下的实际可用 token。
- 请求日志没有记录渠道 token 倍率，管理员后续调整倍率后，历史扣减原因难以解释。

---

## 2. 目标与非目标

### 2.1 必须满足

- 管理员可以为每个渠道配置 token 扣费倍率，默认值为 `1.0`。
- 请求使用某个渠道时，可信上游 usage token 必须按该渠道倍率换算成渠道计费 token。
- 订阅套餐 token 预扣和实际结算必须使用一致的渠道倍率口径。
- API Key token cap 必须使用渠道计费 token；Codex Pro 的订阅额外倍率不得施加到 API Key token cap，保持现有行为。
- 套餐模板和用户订阅实例仍只存标准 token 池，不为每个渠道存一份额度。
- `/api/subscription/plans`、`/api/subscription/public/plans` 和 `/api/subscription/self` 必须返回不同渠道的等价 token 展示数据。
- 历史日志必须能解释当时的扣减：记录原始 usage token、渠道计费 token、订阅最终扣费 token、API Key 计费 token 和请求时的渠道倍率快照。
- 管理员修改渠道倍率后，只影响后续请求；已完成请求和历史日志不回算。
- default 前端展示不同渠道的等价 token 量，并提供清晰文案说明「按当前渠道倍率估算」。
- SQLite、MySQL、PostgreSQL 全兼容。

### 2.2 非目标

- 不恢复旧余额 / quota 扣费模式。
- 不把渠道倍率混入 `ModelRatio`、`CompletionRatio`、`ModelPrice` 或 billing expression。
- 不修改套餐基础额度含义，不把 `MonthlyTokenLimit` 改成渠道专属字段。
- 不新增每个套餐 × 每个渠道的权益配置矩阵。
- 不删除 `logs.quota`、模型价格、模型倍率、billing expression 或管理员成本分析能力。
- 不修改账户余额、充值、订阅套餐价格、支付订单等金额语义功能。
- 不改变本地估算 usage 的现有不扣费语义；本地估算可以用于日志和审计，但不是本规格中的可信计费用 usage。
- 不重写 classic 主题；classic 可继续使用现有展示，除非后续另有明确需求。
- 不修改受保护的项目名称、组织名称、版权、模块路径或品牌信息。

---

## 3. 术语与单位

| 名称 | 建议字段 | 单位 | 用途 | 是否落库 |
|---|---|---|---|---|
| 标准 token | `monthly_token_limit` / `token_limit` | token | 套餐基础额度与订阅实例额度 | 是 |
| 可信原始 usage token | `raw_metered_tokens` | token | 上游返回且可用于计费的真实 token | 日志快照 |
| 本地估算 token | `estimated_raw_tokens` | token | 上游无 usage 时的本地估算，用于预扣保留与审计，不作为最终计费 usage | 日志快照，可选 |
| 渠道 token 扣费倍率 | `channel_token_billing_multiplier` | 倍率 | 原始 usage token 到渠道计费 token 的换算规则 | 渠道字段 + 日志快照 |
| 渠道计费 token | `channel_billable_tokens` | token | 实际扣 API Key token cap、并作为订阅扣费基础 token | 运行时 + 日志快照 |
| 订阅最终扣费 token | `subscription_billable_tokens` | token | 实际扣订阅套餐的 token，可能包含 Codex Pro 订阅额外调整 | 运行时 + 日志快照 |
| 等价 token | `equivalent_token_limit` / `equivalent_token_remaining` | token | 套餐界面展示用派生值 | 响应层派生 |
| 旧余额 quota | `logs.quota`、`ModelRatio`、`ModelPrice` 等 | legacy quota / 成本 | 管理员成本、历史统计、旧价格体系 | 现状保留 |

核心公式：

```text
channel_billable_tokens = round_half_up(raw_metered_tokens * channel_token_billing_multiplier)
subscription_billable_tokens = codex_pro_adjust(channel_billable_tokens) // 仅在现有 Codex Pro 规则命中时调整订阅
api_key_billable_tokens = channel_billable_tokens
equivalent_tokens = floor(standard_tokens / channel_token_billing_multiplier)
```

示例：

| 标准 token | 渠道倍率 | 请求原始 token | API Key 计费 token | 订阅计费 token | 套餐等价可用 token |
|---:|---:|---:|---:|---:|---:|
| 1,000,000 | 1.0 | 10,000 | 10,000 | 10,000 | 1,000,000 |
| 1,000,000 | 2.0 | 10,000 | 20,000 | 20,000 | 500,000 |
| 1,000,000 | 0.5 | 10,000 | 5,000 | 5,000 | 2,000,000 |
| 1,000,000 | 2.0 + Codex Pro served | 10,000 | 20,000 | 按现有 Codex Pro 规则额外调整 | 500,000 |

### 3.1 精确取整规则

实现必须集中使用一个 helper，禁止各调用点自行取整。

建议签名：

```go
func ApplyChannelTokenBillingMultiplier(rawTokens int64, multiplier float64) (int64, error)
func EquivalentTokensForMultiplier(standardTokens int64, multiplier float64) (int64, error)
```

扣费 token 取整规则：

```text
if raw_tokens <= 0: channel_billable_tokens = 0
else:
  product = decimal(raw_tokens) * decimal(multiplier)
  rounded = product.Round(0) // half away from zero / half-up for positive numbers
  channel_billable_tokens = max(1, rounded)
```

展示等价 token 规则：

```text
if standard_tokens <= 0: token_unlimited = true，不返回普通数值
else:
  equivalent_tokens = floor(decimal(standard_tokens) / decimal(multiplier))
```

非法输入：

- `multiplier <= 0`：拒绝。
- `NaN` / `Inf`：拒绝。
- `raw_tokens < 0`：按 `0` 处理或在调用方归零，不能产生负扣费。
- `standard_tokens < 0`：按 `0` / unlimited 兼容现有套餐语义，但新写入不应产生负数。

必须覆盖的边界：

| raw / standard | multiplier | 期望 |
|---:|---:|---:|
| raw `1` | `1.5` | 扣费 `2` |
| raw `3` | `0.5` | 扣费 `2` |
| raw `1` | `0.1` | 扣费 `1` |
| standard `1` | `2` | 等价展示 `0` |
| standard `0` | `2` | unlimited 形态 |

实现应使用项目已有 `shopspring/decimal` 风格做乘除和取整，避免二进制浮点在 `.5` 边界产生不可解释的扣费差异。

### 3.2 倍率含义

- `1.0`：按原始 token 扣费。
- `2.0`：每 1 个上游 token 扣 2 个套餐 token。
- `0.5`：每 1 个上游 token 扣 0.5 个套餐 token。

管理员界面必须避免把该字段描述为「折扣」或「赠送」。推荐名称：

- 管理端：`渠道扣费倍率`
- 用户端：`等价可用 token`
- 日志 / API：`channel_token_billing_multiplier`

---

## 4. 数据模型设计

### 4.1 `model.Channel` 新增字段

文件：`model/channel.go`

新增字段：

```go
TokenBillingMultiplier float64 `json:"token_billing_multiplier" gorm:"not null;default:1"`
```

字段语义：

- `1.0`：默认倍率，保持现有扣费行为。
- `> 1.0`：该渠道消耗套餐 token 更快。
- `< 1.0`：该渠道消耗套餐 token 更慢。
- `<= 0`：非法配置，创建和更新接口必须拒绝。

字段命名说明：

- Go 字段使用 `TokenBillingMultiplier`。
- JSON 字段使用 `token_billing_multiplier`。
- 数据库列建议使用 `token_billing_multiplier`。
- 日志快照使用 `channel_token_billing_multiplier`，突出这是请求所用渠道的快照。

### 4.2 为什么使用显式列

不建议放入 `ChannelOtherSettings` JSON，原因：

- 这是计费核心字段，不是普通渠道附加设置。
- 套餐展示需要快速汇总启用渠道倍率，显式列更直接。
- 管理端渠道列表和表单需要稳定字段。
- 跨 SQLite、MySQL、PostgreSQL 时，显式列比 JSON 查询更可靠。

### 4.3 数据迁移

迁移只新增列，默认值为 `1`。

要求：

- 所有既有渠道迁移后等价于当前行为。
- 不需要回写历史日志。
- 不需要重算历史订单或历史订阅。
- 不使用数据库特有语法；遵循 `model/main.go` 现有迁移模式。
- 新列必须对 SQLite、MySQL、PostgreSQL 均可自动迁移。

### 4.4 创建、更新、复制与批量导入兼容策略

后端创建 / 更新渠道时必须校验：

```text
0 < token_billing_multiplier <= 100
```

倍率上限固定为 `100`，不是实现者自由选择项。原因：避免管理员误填极端值导致套餐瞬间扣空，同时保证前后端校验和错误文案一致。

兼容策略：

- 创建渠道：如果请求没有传 `token_billing_multiplier`，默认写入 `1`；如果显式传 `<= 0`、`> 100`、`NaN` 或 `Inf`，拒绝。
- 更新渠道：必须能区分「未传字段」和「显式传 0」。建议在请求 DTO 或 patch 解析层使用 `*float64` / raw payload presence。未传字段时保留原倍率；显式非法值时拒绝。
- 旧前端 / 旧 API 更新其它字段时，不得把已有倍率覆盖为 `0` 或默认重置为 `1`。
- 复制渠道：默认继承原渠道倍率；如果复制接口允许覆盖倍率，仍必须走相同校验。
- 批量新增渠道：未传倍率时默认 `1`；显式非法值时拒绝该条或整批失败，具体行为需与现有批量错误策略一致。
- `model.Channel.Update()` 若继续使用 GORM struct updates，必须用 `Select` 或 map 确保倍率字段的零值语义不会误清或漏写。

---

## 5. 后端运行时设计

### 5.1 请求级倍率快照与 retry 约束

文件：`relay/common/relay_info.go`、`controller/relay.go`、`middleware/distributor.go`

新增不可变 billing snapshot 字段。该快照独立于 `ChannelMeta`，不能用 `ChannelMeta == nil` 作为是否已冻结的状态位：

```go
ChannelTokenBillingMultiplier float64
InitialChannelId              int
InitialChannelType            int
RawMeteredTokens              int64
ChannelBillableTokens         int64
SubscriptionBillableTokens    int64
ApiKeyBillableTokens          int64
EstimatedRawTokens            int64 // 用于预扣估算与审计，不作为最终计费 usage
```

冻结时序必须调整为：

```text
渠道分发 middleware 选中初始渠道并写入 Gin context
-> RelayInfo 创建和 token 估算
-> 从当前 Gin context 冻结独立 billing snapshot：initial_channel_id / initial_channel_type / channel_token_billing_multiplier
-> 订阅 token 预扣
-> API Key token cap 预扣
-> retry loop / 上游请求；retry 后 ChannelMeta 表示最终上游渠道
```

要求：

- 冻结必须发生在 `service.PreConsumeBilling()` 和 `TokenLimit.PreConsume()` 之前。
- 不得通过提前调用 `RelayInfo.InitChannelMeta(c)` 来冻结倍率；否则会破坏当前 `getChannel()` 依赖 `info.ChannelMeta == nil` 区分首轮和 retry 的语义。实现必须使用独立 billing snapshot，或同步改造 `getChannel()` 的首轮判断，确保首轮不会被误判为 retry。
- `RelayInfo.InitChannelMeta(c)` 可以读取已冻结的倍率并写入最终 `ChannelMeta`，但不得覆盖独立 billing snapshot。
- 同一请求内 billing snapshot 一旦进入预扣阶段，不允许被后续 helper 隐式覆盖。
- consume log 的 `channel_id` / `channel_type` 必须表示最终实际上游渠道；预扣前的初始渠道如需审计，只能写入 `initial_channel_id` / `initial_channel_type` 等独立日志字段，不能让最终日志归属初始渠道。
- token-billed 请求的 retry 只能切换到相同 `token_billing_multiplier` 的渠道。retry 选择阶段必须先排除已用渠道，并按冻结的 `channel_token_billing_multiplier` 过滤候选；过滤后再沿用现有 priority / weight 策略选择渠道。只有过滤后没有同倍率候选时，才停止 retry 并返回原有渠道失败错误。
- 这样可以保证订阅预扣、API Key cap 预扣、实际结算、日志倍率与最终上游渠道倍率一致，并避免为 retry 追加复杂的预扣补偿事务。
- 对免费模型仍执行订阅校验的现有行为保持不变；如果该请求进入订阅 token 口径，仍遵守同倍率 retry 约束。

### 5.2 统一换算 helper

建议新增一个服务层 helper，例如：

```go
func ApplyChannelTokenBillingMultiplier(rawTokens int64, multiplier float64) (int64, error)
func EquivalentTokensForMultiplier(standardTokens int64, multiplier float64) (int64, error)
```

约束：

- helper 不读数据库。
- helper 不依赖 Gin context。
- helper 只处理 token 口径，不处理旧 `quota` 或金额。
- helper 覆盖 `raw = 0`、`raw = 1`、`multiplier < 1`、`multiplier = 1`、`multiplier > 1`、`.5` 取整、`NaN`、`Inf` 等边界。

### 5.3 预扣链路

当前预扣使用估算 prompt token。改造后：

```text
estimated_raw_tokens = relayInfo.GetEstimatePromptTokens()
preconsume_billable_tokens = ApplyChannelTokenBillingMultiplier(estimated_raw_tokens, relayInfo.ChannelTokenBillingMultiplier)
```

影响位置：

- `controller/relay.go`
- `service/billing_session.go`
- `service/funding_source.go`
- `model.PreConsumeUserSubscriptionByUnits()` 的调用参数
- `service/token_limit_session.go`

要求：

- `SubscriptionFunding.distributorAmount` 应传入计费 token，而不是原始 token。
- `relayInfo.SubscriptionPreConsumed` 表示已预扣计费 token。
- `TokenLimit.PreConsume()` 使用同一个 `preconsume_billable_tokens`。
- 预扣不足时直接拒绝请求，避免高倍率渠道先放行、结算时才发现套餐不足。
- 本地估算 token 只用于预扣估算；若上游最终没有可信 usage，仍按现有不扣费路径处理并退还预扣。

### 5.4 实际结算链路

文件：`service/text_quota.go`

当前概念：

```text
apiKeyTokens = SubscriptionMeteredTokens(usage)
subscriptionTokens = apiKeyTokens
```

改造后概念：

```text
rawMeteredTokens = trusted upstream SubscriptionMeteredTokens(usage)
channelBillableTokens = ApplyChannelTokenBillingMultiplier(rawMeteredTokens, relayInfo.ChannelTokenBillingMultiplier)
apiKeyTokens = channelBillableTokens
subscriptionBaseTokens = channelBillableTokens
subscriptionTokens = codexProAdjustedSubscriptionTokens(relayInfo, subscriptionBaseTokens, summary.Quota) // 仅现有 Codex Pro 命中时可能调整订阅
```

要求：

- `SettleBillingWithInput.ApiKeyTokens` 使用 `apiKeyTokens`。
- `SettleBillingWithInput.SubscriptionTokens` 使用 `subscriptionTokens`。
- `rawMeteredTokens`、`channelBillableTokens`、`apiKeyTokens`、`subscriptionTokens` 写入 `RelayInfo`，供日志使用。
- `summary.Quota` 和 `WalletQuota` 不乘渠道 token 倍率，避免污染旧成本 / quota 口径。
- `logs.metered_tokens` 继续表示当前项目已用于订阅展示的最终扣费 token，即 `subscription_billable_tokens`；原始 usage token 通过日志 `other.raw_metered_tokens` 保存，不改变现有 analytics 的主字段含义。

### 5.5 Codex Pro、usage unavailable 与本地估算场景

现有 `subscriptionTokensForTextSettle()` 包含 Codex Pro 调整和部分格式可计费判断。改造时必须保持现有语义：

- 渠道倍率同时作用于订阅基础 token 和 API Key token cap。
- Codex Pro 的额外订阅调整只作用于订阅 token，不作用于 API Key token cap；现有 `TestPostTextConsumeQuotaCodexProServedDoesNotDoubleApiKeyTokenLimit` 语义必须保留，只是 API Key 基础值先经过渠道倍率。
- `usage == nil`：没有可信 usage，不扣订阅和 API Key token；可记录倍率和 `raw_metered_tokens = 0`。
- `TotalTokens == 0`：不扣订阅和 API Key token；可记录倍率和 `raw_metered_tokens = 0`。
- `usageEstimated == true` / `ContextKeyLocalCountTokens == true`：保持当前不扣费语义；本地估算值可记录为 `estimated_raw_tokens`，但 `raw_metered_tokens = 0`、`channel_billable_tokens = 0`。
- 只有可信上游 usage token 才进入 `ApplyChannelTokenBillingMultiplier()` 作为扣费依据。

推荐顺序：

```text
可信上游 raw usage token -> 渠道倍率 -> channel_billable_tokens
channel_billable_tokens -> API Key token cap
channel_billable_tokens -> 现有 Codex Pro 订阅额外调整 -> subscription_billable_tokens
```

### 5.6 Realtime / WebSocket / 非文本任务

凡是最终扣订阅 token 的入口，都必须统一使用同一个 helper。

Realtime / WebSocket 增量规则：

```text
raw_increment_tokens = usage.TotalTokens for this increment
billable_increment_tokens = ApplyChannelTokenBillingMultiplier(raw_increment_tokens, frozen_multiplier)
TokenLimit.ConsumeIncrement(billable_increment_tokens)
BillingSession.SettleSubscriptionIncrement(billable_increment_tokens)
relayInfo.RawMeteredTokens += raw_increment_tokens
relayInfo.ChannelBillableTokens += billable_increment_tokens
relayInfo.ApiKeyBillableTokens += billable_increment_tokens
relayInfo.SubscriptionBillableTokens += billable_increment_tokens
```

要求：

- `PostWssConsumeQuota()` 不得再次对 `RealtimeSubscriptionTokens()` 乘倍率，避免二次扣费。
- `GenerateWssOtherInfo()` 日志使用累计 raw / billable 字段。
- 音频、图像、任务类接口中，如果仍走旧 quota / per-call 价格，不应强行套用渠道 token 倍率。
- 规则是：只有进入订阅 token 口径的扣减，才应用渠道 token 倍率。

---

## 6. 日志与审计

文件：`service/log_info_generate.go`、`model/log.go`

在现有 `appendBillingInfo()` / `appendSubscriptionTokenInfo()` 基础上追加快照字段。

建议字段：

```json
{
  "channel_token_billing_multiplier": 2,
  "raw_metered_tokens": 10000,
  "channel_billable_tokens": 20000,
  "api_key_billable_tokens": 20000,
  "subscription_billable_tokens": 20000,
  "subscription_tokens_consumed": 20000
}
```

要求：

- 历史日志无需回填。
- 新日志必须记录请求时倍率，而不是展示时动态读取渠道当前倍率。
- 日志中 `subscription_tokens_consumed` 继续表示实际扣减订阅 token，即 `subscription_billable_tokens`。
- `logs.metered_tokens` 继续表示最终扣订阅的计费 token；`raw_metered_tokens` 只写入 `other`。
- 如果没有可信 usage 或本次不扣费，`raw_metered_tokens` / `channel_billable_tokens` / `subscription_billable_tokens` 可为 `0`，但倍率仍可记录，便于排查。
- 预扣阶段因额度不足被拒绝时，现有请求通常不会产生 consume log；规格不要求新增失败日志，但错误日志或调试信息应包含冻结倍率，便于排查。
- API Key token cap 结算失败但响应已开始的 audit 路径，必须记录 `api_key_billable_tokens` 和现有 token limit audit 字段。

---

## 7. 套餐展示 API 设计

### 7.1 后端返回派生视图

套餐界面不应由前端自行拉全量渠道并计算。后端应返回派生字段。

字段挂载位置必须固定：

- `GET /api/subscription/plans`：每条记录为 `{ "plan": { ... } }`，新增字段在 `record.plan.channel_token_equivalents`。
- `GET /api/subscription/public/plans`：每条记录为 `{ "plan": { ... } }`，新增字段在 `record.plan.channel_token_equivalents`。
- `GET /api/subscription/self`：新增字段在 `summary.channel_token_equivalents`。

原因：

- 当前 wallet 套餐卡片和 home 公开套餐预览都读取 `record.plan.*`。
- 前端不应知道哪些渠道参与用户展示。
- 公开套餐页不应泄露渠道密钥、base URL、组织、内部备注等敏感信息。
- 后端可以统一过滤启用渠道、去重、排序、处理同类型多倍率。
- 用户当前订阅摘要和公开套餐列表能保持一致。

### 7.2 后端结构体契约

建议定义统一基础字段，并分别定义计划额度和订阅摘要两种视图。

计划套餐等价视图：

```go
type PlanChannelTokenEquivalent struct {
    Kind                     string  `json:"kind"` // single/range/unlimited
    ChannelType              int     `json:"channel_type"`
    ChannelTypeName          string  `json:"channel_type_name"`
    ChannelTypeLabelKey      string  `json:"channel_type_label_key,omitempty"`
    VariantCount             int     `json:"variant_count"`
    Multiplier               float64 `json:"multiplier,omitempty"`
    MinMultiplier            float64 `json:"min_multiplier,omitempty"`
    MaxMultiplier            float64 `json:"max_multiplier,omitempty"`
    EquivalentTokenLimit     int64   `json:"equivalent_token_limit,omitempty"`
    EquivalentTokenLimitMin  int64   `json:"equivalent_token_limit_min,omitempty"`
    EquivalentTokenLimitMax  int64   `json:"equivalent_token_limit_max,omitempty"`
    TokenUnlimited           bool    `json:"token_unlimited,omitempty"`
}
```

订阅摘要等价视图：

```go
type SubscriptionChannelTokenEquivalent struct {
    Kind                         string  `json:"kind"` // single/range/unlimited
    ChannelType                  int     `json:"channel_type"`
    ChannelTypeName              string  `json:"channel_type_name"`
    ChannelTypeLabelKey          string  `json:"channel_type_label_key,omitempty"`
    VariantCount                 int     `json:"variant_count"`
    Multiplier                   float64 `json:"multiplier,omitempty"`
    MinMultiplier                float64 `json:"min_multiplier,omitempty"`
    MaxMultiplier                float64 `json:"max_multiplier,omitempty"`
    EquivalentTokenLimit         int64   `json:"equivalent_token_limit,omitempty"`
    EquivalentTokenLimitMin      int64   `json:"equivalent_token_limit_min,omitempty"`
    EquivalentTokenLimitMax      int64   `json:"equivalent_token_limit_max,omitempty"`
    EquivalentTokenRemaining     int64   `json:"equivalent_token_remaining,omitempty"`
    EquivalentTokenRemainingMin  int64   `json:"equivalent_token_remaining_min,omitempty"`
    EquivalentTokenRemainingMax  int64   `json:"equivalent_token_remaining_max,omitempty"`
    TokenUnlimited               bool    `json:"token_unlimited,omitempty"`
}
```

形态约束：

- `kind = "single"`：`variant_count = 1`，必须返回 `multiplier`；有限额度视图必须返回对应的单值 `equivalent_token_limit`，订阅摘要还必须返回 `equivalent_token_remaining`。
- `kind = "range"`：`variant_count > 1`，必须返回 `min_multiplier`、`max_multiplier`、`equivalent_token_limit_min`、`equivalent_token_limit_max`；订阅摘要还必须返回 `equivalent_token_remaining_min`、`equivalent_token_remaining_max`。
- `kind = "unlimited"`：必须返回 `token_unlimited = true`；不得返回普通等价 token 数值字段，避免前端把 `0` 误解为耗尽。

区间字段含义：

- `min_multiplier` 是同渠道类型下最小倍率。
- `max_multiplier` 是同渠道类型下最大倍率。
- `equivalent_token_limit_min` 对应 `max_multiplier`，表示最保守可用量。
- `equivalent_token_limit_max` 对应 `min_multiplier`，表示最乐观可用量。
- remaining 字段同理。

### 7.3 渠道过滤、排序与名称

后端汇总规则：

- 仅包含 `Channel.Status == enabled` 的渠道。
- 多 key 渠道如果项目已能判断全部 key 不可用，应排除；如果无法低成本判断，至少不包含已禁用渠道。
- 排除仅 channel test 或内部调试专用渠道；如果当前没有显式标识，则按现有启用渠道集合处理，不新增复杂可见性模型。
- 按 `channel_type_name` 或 `channel_type` 稳定排序；前端只展示前 3 项时必须得到稳定结果。
- `channel_type_name` 是后端稳定显示名，不要求后端本地化；前端若已有 `CHANNEL_TYPE_OPTIONS` / i18n label，应优先按 `channel_type` 或 `channel_type_label_key` 本地化展示，找不到时回退到 `channel_type_name`。

### 7.4 公开套餐列表与用户套餐列表

接口：`GetSubscriptionPlans()` 和 `GetPublicSubscriptionPlans()`。

当前返回：

```json
[
  { "plan": { "monthly_token_limit": 1000000 } }
]
```

应改为：

```json
[
  {
    "plan": {
      "monthly_token_limit": 1000000,
      "channel_token_equivalents": [
        {
          "kind": "single",
          "channel_type": 1,
          "channel_type_name": "OpenAI",
          "variant_count": 1,
          "multiplier": 1,
          "equivalent_token_limit": 1000000
        },
        {
          "kind": "range",
          "channel_type": 14,
          "channel_type_name": "Claude",
          "variant_count": 2,
          "min_multiplier": 1.5,
          "max_multiplier": 2,
          "equivalent_token_limit_min": 500000,
          "equivalent_token_limit_max": 666666
        }
      ]
    }
  }
]
```

无限套餐示例：

```json
{
  "kind": "unlimited",
  "channel_type": 1,
  "channel_type_name": "OpenAI",
  "variant_count": 1,
  "token_unlimited": true
}
```

要求：

- 如果所有展示渠道倍率都是 `1.0`，仍可返回数据，前端可选择折叠；不要依赖前端再猜默认倍率。
- `model.SubscriptionPlan` 如直接承载该字段，必须加 `gorm:"-"`，不得持久化派生数据。

### 7.5 用户当前订阅摘要

接口：`GetSubscriptionSelf()`。

当前 `SelfSubscriptionSummary` 包含：

```text
token_limit
token_used
token_remaining
token_unlimited
```

应新增：

```go
ChannelTokenEquivalents []SubscriptionChannelTokenEquivalent `json:"channel_token_equivalents,omitempty"`
```

有限订阅示例：

```json
{
  "summary": {
    "token_limit": 1000000,
    "token_remaining": 600000,
    "channel_token_equivalents": [
      {
        "kind": "single",
        "channel_type": 1,
        "channel_type_name": "OpenAI",
        "variant_count": 1,
        "multiplier": 1,
        "equivalent_token_limit": 1000000,
        "equivalent_token_remaining": 600000
      }
    ]
  }
}
```

要求：

- `TokenLimit` 派生 `equivalent_token_limit`。
- `TokenRemaining` 派生 `equivalent_token_remaining`。
- `TokenUnlimited = true` 时，等价 token 也必须使用 `kind = "unlimited"`，而不是返回巨大数值或 `0`。

---

## 8. 前端设计

### 8.1 类型与 API

涉及文件：

- `web/default/src/features/subscriptions/types.ts`
- `web/default/src/features/subscriptions/api.ts`
- `web/default/src/features/channels/types.ts`

前端类型必须使用可判别联合类型，不允许用一组松散 optional 字段表达全部形态。

计划套餐视图建议：

```ts
export type PlanChannelTokenEquivalent =
  | {
      kind: 'single'
      channel_type: number
      channel_type_name: string
      channel_type_label_key?: string
      variant_count: 1
      multiplier: number
      equivalent_token_limit: number
    }
  | {
      kind: 'range'
      channel_type: number
      channel_type_name: string
      channel_type_label_key?: string
      variant_count: number
      min_multiplier: number
      max_multiplier: number
      equivalent_token_limit_min: number
      equivalent_token_limit_max: number
    }
  | {
      kind: 'unlimited'
      channel_type: number
      channel_type_name: string
      channel_type_label_key?: string
      variant_count: number
      token_unlimited: true
    }
```

订阅摘要视图建议：

```ts
export type SubscriptionChannelTokenEquivalent =
  | (Extract<PlanChannelTokenEquivalent, { kind: 'single' }> & {
      equivalent_token_remaining: number
    })
  | (Extract<PlanChannelTokenEquivalent, { kind: 'range' }> & {
      equivalent_token_remaining_min: number
      equivalent_token_remaining_max: number
    })
  | Extract<PlanChannelTokenEquivalent, { kind: 'unlimited' }>
```

类型更新点：

- `SubscriptionPlan` / `PlanRecord.plan.channel_token_equivalents`
- `PublicSubscriptionPlan` / `PublicPlanRecord.plan.channel_token_equivalents`
- `SelfSubscriptionSummary.channel_token_equivalents`
- 渠道类型 `Channel.token_billing_multiplier`
- 渠道创建 / 更新 payload 中的 `token_billing_multiplier`

要求：

- 类型中不要使用 `any`。
- React 组件必须通过 `kind` 做分支渲染。
- 前端不得自行拉全量渠道计算等价 token。

### 8.2 套餐卡片展示

涉及文件：

- `web/default/src/features/wallet/components/subscription-plans-card.tsx`
- `web/default/src/features/home/components/sections/plans-preview.tsx`
- `web/default/src/features/wallet/lib/subscription-display.ts`
- `web/default/src/features/subscriptions/lib/format.ts`

推荐展示层级：

```text
标准额度：1M tokens
等价可用：
OpenAI：约 1M tokens
Claude：约 500K - 666K tokens
Gemini：约 800K tokens
```

规则：

- Wallet 套餐卡片必须展示等价 token。
- Home 公开套餐预览可以保持简化，但如果展示 token 额度，也必须避免与 wallet 不一致；推荐同样使用简化的等价提示或链接到套餐详情。
- 卡片默认最多展示 3 个渠道类型，使用后端稳定排序的前 3 项。
- 超过 3 个时用「查看更多」或折叠区域，避免套餐卡片过高。
- 如果所有倍率都是 `1.0`，可以只展示标准额度，并通过 tooltip 或详情保留等价说明。
- 禁止使用旧 `formatQuota()` 展示等价 token。
- 不能直接用当前 `formatTokenLimit()` 格式化 remaining，因为它对 `0` 返回 `Unlimited tokens`。必须区分：
  - limit：只有 `token_unlimited = true` / `kind = "unlimited"` 时显示 unlimited。
  - finite count / remaining：`0` 必须显示 `0 tokens`。
  - 可新增或复用不会把 `0` 当 unlimited 的 token count formatter。

### 8.3 当前订阅摘要展示

推荐展示：

```text
剩余额度：600K 标准 tokens
按当前渠道倍率估算：
OpenAI：约 600K tokens
Claude：约 300K - 400K tokens
```

文案必须包含「估算」或「按当前渠道倍率」，避免用户误以为套餐实际拥有多份独立额度。

### 8.4 管理端渠道配置

涉及文件：

- `web/default/src/features/channels/types.ts`
- `web/default/src/features/channels/lib/channel-form.ts`
- `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`

新增表单字段：

```text
渠道扣费倍率
```

说明文案：

```text
1 表示按原始 token 扣减；2 表示每 1 个上游 token 扣 2 个套餐 token；0.5 表示每 1 个上游 token 扣 0.5 个套餐 token。
```

必须同步更新：

- `Channel` 类型。
- `ChannelFormValues`。
- Zod schema。
- `CHANNEL_FORM_DEFAULT_VALUES`，默认值为 `1`。
- `transformChannelToFormDefaults()`。
- `transformFormDataToCreatePayload()`。
- `transformFormDataToUpdatePayload()`。

校验：

- 必填。
- 必须大于 `0`。
- 最大值 `100`。
- 默认值 `1`。

### 8.5 缓存刷新

渠道倍率保存成功后，必须刷新或失效所有会显示等价 token 的数据源。

当前已知数据源：

- 渠道列表：`channelsQueryKeys.lists()`。
- Home 公开套餐预览：`['home', 'subscription-public-plans']`。
- 当前订阅摘要：`['subscriptions', 'self', 'summary']`。
- Dashboard 概览订阅摘要：`['dashboard', 'overview', 'self-subscriptions', user?.id]`。
- Admin 套餐列表如果接入 React Query，应刷新其对应 query；如果仍使用 `refreshTrigger`，需要触发现有刷新机制。
- Wallet `SubscriptionPlansCard` 当前使用本地 state 手动调用 `getPublicPlans()` / `getSelfSubscriptionFull()`；实现可选择迁移到共享 query key helper，或在相关页面重新进入 / 保存后显式重新拉取。计划必须选一种，不允许遗漏。

推荐：新增 subscriptions query key helper，逐步让 wallet 和 dashboard 复用，降低漏刷风险。

### 8.6 i18n

default 前端新增文案必须同步到：

```text
web/default/src/i18n/locales/en.json
web/default/src/i18n/locales/zh.json
web/default/src/i18n/locales/fr.json
web/default/src/i18n/locales/ru.json
web/default/src/i18n/locales/ja.json
web/default/src/i18n/locales/vi.json
```

如果新增文案出现在常量、helper 返回的 `labelKey` 或动态 key 中，必须同步 `web/default/src/i18n/static-keys.ts`。

推荐中文术语：

| 场景 | 中文 | 英文建议 |
|---|---|---|
| 管理端字段名 | 渠道扣费倍率 | Channel token billing multiplier |
| 用户端说明 | 等价可用 token | Equivalent usable tokens |
| 说明文案 | 按当前渠道倍率估算，实际扣减以请求使用的渠道为准 | Estimated by current channel multiplier. Actual deduction depends on the channel used. |
| 日志字段说明 | 原始 token / 计费 token | Raw tokens / billable tokens |

---

## 9. 历史订单、现有订阅与倍率修改语义

### 9.1 已购买订阅

已购买订阅的 `UserSubscription.TokenLimit` 不变。

管理员修改渠道倍率后：

- 后续请求按新倍率扣费。
- 当前套餐界面的等价 token 实时按新倍率展示。
- 历史请求日志按请求时快照展示，不回算。
- 历史订单不新增渠道倍率快照。

理由：

- `TokenLimit` 是购买 / 发放时的标准 token 权益快照。
- 渠道倍率是消费规则，不是订单权益本身。
- 若按购买时倍率展示，会导致不同用户对同一渠道看到不同兑换率，产品解释成本过高。

### 9.2 试用、续费、取消、重置

以下路径不改变基础额度语义：

- 试用套餐
- 邀请奖励套餐
- 订阅续费
- 订阅取消
- 手动重置周期额度
- 活跃套餐切换

要求：

- 这些路径继续维护 `TokenLimit` / `TokenUsed`。
- 渠道等价 token 只在展示和请求扣减时派生。
- `ResetUserSubscriptionQuota()` 仍把 `TokenUsed` 清零，不需要按渠道拆分重置。

---

## 10. 缓存、并发与数据库兼容

### 10.1 缓存

需要注意的缓存：

- 订阅套餐缓存：倍率变更后，套餐等价展示可能变化。
- 渠道选择缓存：retry 约束需要能读取候选渠道倍率；缓存结构应包含 `token_billing_multiplier` 或能快速按 channel id 查询。
- 定价缓存：渠道倍率不应污染模型价格缓存。
- 前端缓存：见第 8.5 节。

要求：

- 渠道倍率变更后，后端应使派生的 `channel_token_equivalents` 尽快反映新值。
- 不需要让历史日志更新。

### 10.2 并发

必须保持现有事务与原子更新模式：

- `PreConsumeUserSubscriptionByUnits()` 继续通过事务预扣。
- `applySubscriptionPreConsumeUpdateTx()` 继续使用条件更新避免超扣。
- `PostConsumeUserSubscriptionTokenDelta()` 继续通过 delta 更新 token_used。
- 正在处理的请求使用 `RelayInfo` 中的倍率快照，不受渠道配置并发修改影响。
- retry 只允许同倍率渠道，避免中途切换倍率导致预扣补偿复杂化。

### 10.3 数据库兼容

要求：

- 新列类型使用 GORM 可跨数据库生成的普通浮点类型或 decimal 兼容类型。
- 不使用 MySQL-only、PostgreSQL-only 或 SQLite 不支持的 SQL。
- 需要 raw SQL 时遵循 `model/main.go` 的跨数据库分支模式。
- 新增字段默认值必须在 SQLite、MySQL、PostgreSQL 下均可正常迁移。

---

## 11. 测试要求

### 11.1 后端单元测试

必须覆盖：

1. `multiplier = 1.0` 时行为与当前一致。
2. `multiplier = 2.0` 时，预扣和实际结算均翻倍。
3. `multiplier = 0.5` 时，扣减减半并按 half-up 规则处理 `.5`。
4. `raw_tokens > 0` 且换算结果小于 `1` 时，实际扣 `1`。
5. `multiplier <= 0`、`> 100`、`NaN`、`Inf` 时，渠道创建 / 更新接口拒绝。
6. 旧客户端更新渠道未传倍率时保留原值；显式传 `0` 时拒绝。
7. 复制渠道继承倍率；批量新增未传倍率默认 `1`。
8. 预扣使用渠道计费 token，不是原始 token。
9. API Key token cap 使用渠道计费 token。
10. Codex Pro：渠道倍率作用于 API Key cap；Codex Pro 额外调整只作用于订阅 token，现有“不翻倍 API Key cap”的语义保持。
11. `usage == nil`、`TotalTokens == 0`、`usageEstimated == true` 均不扣费，并记录合理日志字段。
12. 结算差额使用订阅最终扣费 token，退款不会把 `token_used` 扣成负数。
13. retry 不会切换到不同倍率渠道；没有同倍率候选时停止 retry。
14. Realtime / WebSocket 增量扣费只乘一次倍率，post 阶段不重复乘倍率。
15. 日志记录 `raw_metered_tokens`、`channel_billable_tokens`、`api_key_billable_tokens`、`subscription_billable_tokens`、`channel_token_billing_multiplier`。
16. `logs.metered_tokens` 继续记录订阅最终扣费 token。
17. 管理员修改倍率后，已生成日志中的倍率快照不变。

候选测试文件：

- `service/billing_session_test.go`
- `service/text_quota_test.go`
- `service/subscription_billing_test.go`
- `service/task_billing_test.go`
- `model/subscription_distributor_test.go`
- `controller/subscription_*_test.go`
- 新增渠道更新相关 controller 测试

### 11.2 前端测试与检查

必须覆盖：

1. 套餐卡片展示标准 token 与不同渠道等价 token。
2. 同渠道类型多倍率时展示区间。
3. 无限套餐 / 无限订阅使用 `kind = "unlimited"`，不把 `0` 显示为耗尽。
4. finite remaining 为 `0` 时显示 `0 tokens`，不能显示 `Unlimited tokens`。
5. 所有倍率为 `1.0` 时界面不产生误导性重复信息。
6. 渠道表单默认倍率为 `1`。
7. 渠道表单拒绝 `0`、负数、超过 `100`、非数字。
8. 渠道保存成功后，会刷新渠道列表、套餐列表、当前订阅摘要和 dashboard 相关订阅展示。
9. i18n key 完整，`bun run i18n:sync` 不产生缺失项。

建议测试文件或覆盖点：

- `web/default/src/features/wallet/lib/subscription-display*.test.ts`
- `web/default/src/features/subscriptions/lib/format*.test.ts`
- `web/default/src/features/channels/lib/channel-form*.test.ts`
- 套餐卡片组件测试或轻量渲染测试

变更 TS / TSX 后必须运行：

```bash
bun run typecheck
```

如实现中新增或修改测试，还应运行对应测试命令；不得只依赖构建通过。

### 11.3 验收场景

端到端验收场景：

1. 创建渠道 A，倍率 `1`；创建渠道 B，倍率 `2`。
2. 创建套餐，`monthly_token_limit = 1,000,000`。
3. 套餐页面展示：渠道 A 约 `1,000,000` tokens，渠道 B 约 `500,000` tokens。
4. 用户使用渠道 B 发起一次上游 usage 为 `10,000` tokens 的请求。
5. 订阅 `token_used` 增加 `20,000`。
6. API Key token cap 已启用时，API Key `token_used` 也增加 `20,000`。
7. 日志显示 `raw_metered_tokens = 10000`、`channel_billable_tokens = 20000`、`api_key_billable_tokens = 20000`、`subscription_billable_tokens = 20000`、`channel_token_billing_multiplier = 2`。
8. Codex Pro served + 渠道 B 倍率 `2` 时，API Key cap 只按渠道倍率后的 token 记账，订阅 token 再按现有 Codex Pro 规则额外调整。
9. retry 时若下一个候选渠道倍率不同，应跳过该候选或停止 retry；日志倍率与最终使用渠道倍率一致。
10. 管理员把渠道 B 倍率改为 `1.5` 后，套餐页面展示渠道 B 约 `666,666` tokens；历史日志仍显示倍率 `2`。

---

## 12. 实施顺序建议

1. 新增后端数据模型字段与迁移，默认倍率 `1`。
2. 后端渠道创建 / 更新 / 复制 / 批量新增接口增加倍率校验与兼容处理。
3. 新增统一 token 倍率换算 helper 与测试。
4. 在 relay 预扣前冻结渠道倍率快照，并约束 retry 只能选择同倍率渠道。
5. 改订阅 token 和 API Key token cap 预扣链路。
6. 改文本订阅 token、API Key token cap 和 Codex Pro 分层结算链路。
7. 改 Realtime / WebSocket 增量扣费链路。
8. 日志追加倍率、原始 token、渠道计费 token、API Key 计费 token、订阅最终扣费 token 快照。
9. 后端套餐 API 增加 `channel_token_equivalents` 派生字段。
10. 前端渠道管理表单增加倍率配置。
11. 前端套餐卡片和当前订阅摘要展示等价 token。
12. 补齐 i18n 和相关测试。
13. 运行后端定向测试、前端 typecheck、i18n 同步检查。

---

## 13. 关键风险与规避

| 风险 | 影响 | 规避 |
|---|---|---|
| 预扣前没有冻结渠道倍率 | 高倍率渠道可能先放行，结算时额度不足 | 在预扣前从已选渠道 context 冻结倍率 |
| retry 切换到不同倍率渠道 | 预扣、结算、日志渠道不一致 | token-billed 请求 retry 只允许同倍率渠道 |
| 订阅和 API Key token cap 口径混淆 | 用户看到两个 token 限额但扣减不一致 | 渠道倍率共同作用；Codex Pro 额外倍率只作用订阅 |
| 倍率混入旧 quota / 模型价格 | 旧成本统计、模型价格、动态表达式语义混乱 | 渠道倍率只作用于 token 额度口径 |
| 倍率允许 `0` | 误配成免费渠道 | 后端强制 `0 < multiplier <= 100`，前端同步校验 |
| float `.5` 边界不可解释 | 计费争议 | 用 decimal helper 和 half-up 规则集中计算 |
| 不记录倍率快照 | 历史日志无法解释 | 请求开始冻结，日志写入快照 |
| 同渠道类型多倍率只取第一个 | 套餐展示误导用户 | 后端返回 `kind = "range"` 区间 |
| 无限套餐返回数值 `0` | 前端误判耗尽或 unlimited | 使用 `kind = "unlimited"` 专门形态 |
| 前端自行拉渠道计算 | 泄露内部渠道结构，逻辑重复 | 后端返回派生 `channel_token_equivalents` |
| 缓存刷新遗漏 | 管理员保存倍率后页面仍显示旧等价量 | 明确所有相关 query / 本地 state 刷新策略 |
| 数据库迁移使用特有语法 | 三数据库兼容失败 | 使用 GORM 迁移和普通列类型 |
| i18n 文案误导 | 用户以为获得多份额度 | 使用「等价」「估算」「按当前渠道倍率」 |

---

## 14. 完成定义

实现完成必须同时满足：

- 管理端可以创建和编辑渠道 token 扣费倍率。
- 倍率默认 `1`，现有渠道迁移后行为不变。
- 倍率校验固定为 `0 < multiplier <= 100`。
- 请求在订阅 / API Key token cap 预扣前冻结独立 billing snapshot，不依赖提前初始化 `ChannelMeta`。
- token-billed 请求 retry 只在同倍率候选集合内按现有 priority / weight 策略选择；consume log 渠道归属最终实际上游渠道。
- 订阅预扣、订阅结算、API Key token cap 均使用渠道计费 token。
- Codex Pro 额外订阅调整不作用于 API Key token cap。
- 本地估算 usage 保持现有不扣费语义，只用于预扣保留与审计。
- 订阅 `token_used` 按渠道倍率扣减。
- `/api/subscription/plans`、`/api/subscription/public/plans` 和 `/api/subscription/self` 返回类型安全的渠道等价 token。
- default 前端展示不同渠道等价 token，并提供清晰说明。
- finite remaining 为 `0` 时显示 `0 tokens`，无限额度使用专门 unlimited 形态。
- 日志记录原始 usage token、渠道计费 token、API Key 计费 token、订阅最终扣费 token、渠道倍率快照和必要的初始渠道审计字段。
- 覆盖后端定向测试、前端新增 / 修改的定向测试、前端类型检查、i18n 同步检查。
- 未修改受保护项，未引入旧余额 / quota 扣费回归。
