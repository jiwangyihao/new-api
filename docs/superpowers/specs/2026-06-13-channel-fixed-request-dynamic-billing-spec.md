# 渠道固定请求扣费与动态倍率规格

> 面向 AI 代理的工作者：本规格用于指导后续在 `C:/Users/34404/source/repos/new-api` 上实现「渠道按请求固定扣费 credit」与「依赖上游返回动态倍率数扣费」。实现前必须读取仓库根目录 `AGENTS.md`、涉及前端时读取 `web/default/AGENTS.md`，遵守 Go + Gin + GORM、React 19 + TypeScript + Bun、SQLite / MySQL / PostgreSQL 全兼容约束。涉及 billing expression 时必须先阅读 `pkg/billingexpr/expr.md`；本规格不把该能力塞进 billing expression。

**目标：** 支持管理员把某个渠道配置为「每个可计费请求固定扣若干 credit」，例如每次扣 `80,000 credit`；同时把现有「根据上游返回特征写死倍率」收敛为「上游返回明确动态倍率数，结算层统一应用」。

**架构：** 套餐仍只有一个通用 credit 池。渠道 billing profile 决定 credit 的消费方式：usage token 模式按上游 usage token × 渠道 token 倍率换算为 credit，fixed request 模式按每个可计费请求固定扣 credit。上游动态倍率只在渠道显式启用后生效，并在统一结算函数中作为最后一层倍率应用，避免 Codex Pro、日志、订阅、API Key cap 各自重复乘倍率。

**技术栈：** Go 1.25.1、Gin、GORM v2、SQLite / MySQL / PostgreSQL、React 19、TypeScript、TanStack Query、Rsbuild、Bun、i18next。

---

## 1. 背景与用户已确认决策

本规格基于现有渠道 token 扣费倍率能力继续扩展。已有规格可参考：

- `docs/superpowers/specs/2026-06-10-channel-token-billing-multiplier-spec.md`

用户已确认以下修正，后续实现必须按这些决策执行：

1. **流式动态倍率不是新增风险。** 现有真实 token 用量本来也是请求结束后才知道；动态倍率同样在结算阶段使用，不应把「流结束才知道倍率」作为额外阻碍或单独风险放大。
2. **倍率计算必须收敛。** Codex Pro、`NewAPIBillingFromUsage`、`PostTextConsumeQuota`、订阅结算、API Key token cap 和日志展示不能各自散落乘倍率逻辑，必须统一到一个结算结果计算函数。
3. **保持无 usage 不扣费。** 上游没有可信 usage 时，最终结算仍为 `0 credit`，包括 fixed request 模式；预扣应在结算时退回。不得为了固定请求扣费而改变现有「无 usage 不扣费」语义。
4. **按渠道展示不同权益。** usage token 渠道继续展示该渠道下可用 token 量；fixed request 渠道展示可用请求次数。请求次数不是估算值，按 `credit / fixed_request_credits` 向下取整得到完整可用次数，不使用「约」。
5. **原有通用额度表达必须全面统一为 credit。** 这不是只改套餐页；所有用户可见、管理员可见、日志展示、导出、通知、表格列、表单说明、i18n 文案、前端类型和新增 / 改动 API DTO 中，凡是指「订阅额度池、扣费、已用、剩余、预扣、API Key 消费额度」的旧 token 表达，都必须改为 credit。
6. **保留 token 一词的范围必须收窄。** 只有上游 / 模型真实 usage（prompt tokens、completion tokens、total tokens）、上下文长度、模型 token limit、tokenizer、usage token 渠道的等价可用 token 量，以及 OpenAI 协议本身字段，才继续使用 token。
7. **动态倍率必须渠道显式启用。** 默认不信任任意 OpenAI-compatible 上游返回的倍率字段；只有渠道配置启用后，适配器才读取并应用上游动态倍率。

---

## 2. 当前事实

### 2.1 订阅计费主链路

当前 API 请求主资金来源已收敛到订阅套餐 token 池，关键链路为：

```text
controller/relay.go
  -> helper.ModelPriceHelper(...)
  -> relayInfo.FreezeChannelTokenBillingSnapshot(...)
  -> service.PreConsumeBilling(...)
  -> 上游请求
  -> service.PostTextConsumeQuota(...) / PostWssConsumeQuota(...) / PostAudioConsumeQuota(...)
  -> service.SettleBillingWithInput(...)
  -> SubscriptionFunding.Settle(...)
```

现有订阅预扣和结算底层分别由以下函数完成：

- `model.PreConsumeUserSubscriptionByUnits(...)`
- `model.PostConsumeUserSubscriptionTokenDelta(...)`

旧余额 / quota 口径已不再作为请求资金来源，不应在本功能中恢复。

### 2.2 现有渠道 token 倍率

当前项目已有渠道级 token 扣费倍率：

- `model.Channel.TokenBillingMultiplier`
- API 字段：`token_billing_multiplier`
- 上下文键：`constant.ContextKeyChannelTokenBillingMultiplier`
- `relay/common/relay_info.go` 冻结到 `RelayInfo.ChannelTokenBillingMultiplier`
- `service/text_quota.go` 使用 `tokenbilling.ApplyMultiplier(...)` 得到渠道计费 token

现有倍率语义应保留：

```text
usage token 模式下：base_credits = round(raw_metered_tokens * channel_token_billing_multiplier)
```

这里虽然字段名仍包含 `token`，但它表示「raw usage token 到 credit 的换算倍率」。

### 2.3 现有套餐等价展示

当前已有按渠道倍率展示套餐等价 token 的能力：

- `model/subscription_channel_equivalent.go`
- `controller/subscription.go`
- `web/default/src/features/wallet/lib/subscription-display.ts`
- `web/default/src/features/wallet/components/subscription-plans-card.tsx`
- `web/default/src/features/home/lib/plans-preview.ts`

本规格在此基础上扩展：

- usage token 渠道：继续展示可用 token。
- fixed request 渠道：展示可用请求次数。
- 通用额度文案从 token 改为 credit。

### 2.4 现有上游返回特征倍率

当前类似「上游返回特征决定倍率」的实现主要是 Codex Pro：

- 上游 trailer `X-NewAPI-Pro-Served: codex-pro` 标记 `RelayInfo.CodexProServed`。
- `service.NewAPIBillingFromUsage(...)` 中存在 Codex Pro 额外倍率。
- `service.text_quota.go` 中也有 Codex Pro 订阅 token 调整逻辑。

后续必须收敛为一个结算结果计算点。服务层不再到处根据 `CodexProServed` 自行乘固定倍率。

---

## 3. 目标与非目标

### 3.1 必须满足

- 管理员可以为渠道选择 credit billing mode：
  - `usage_tokens`：按可信 usage token × 渠道 token 倍率扣 credit。
  - `fixed_request`：每个可计费请求固定扣 credit。
- fixed request 模式支持配置 `fixed_request_credits`，例如 `80000`。
- 上游没有可信 usage 时，无论哪种模式，最终结算都保持 `0 credit`。
- usage 存在但 `total_tokens = 0` 时：
  - `usage_tokens` 模式扣 `0 credit`。
  - `fixed_request` 模式视为一个可计费请求，扣 `fixed_request_credits`。
- 动态倍率必须由上游返回明确数值，例如 `1.5`，不能再由本地根据上游特征写死推导。
- 动态倍率必须渠道显式启用后才读取和应用。
- 动态倍率与渠道倍率、fixed request credit 必须通过统一结算函数组合。
- 全站必须统一通用额度表达：
  - 订阅额度池、套餐额度、用户剩余额度、扣费记录、预扣 / 退款、API Key 额度消耗、日志展示、导出、通知和管理端表格都使用 credit。
  - usage token 渠道下的派生权益仍展示可用 token，因为它表示「该渠道可消耗多少上游 usage token」。
  - fixed request 渠道下的派生权益展示可用请求次数。
- fixed request 请求次数必须用向下取整的完整请求数展示，不使用「约」。
- 日志必须能解释扣费：记录 billing mode、raw usage、base credits、dynamic multiplier、final credits、倍率来源。
- SQLite、MySQL、PostgreSQL 全兼容。

### 3.2 非目标

- 不恢复旧余额 / quota 作为请求资金来源。
- 不把 fixed request 或 dynamic multiplier 塞进 `billingexpr`。
- 不把每个套餐拆成「每渠道一份额度」。
- 不引入管理员可配置 JSONPath 从任意上游响应读取倍率。
- 不改变无 usage 不扣费的现有语义。
- 不把「真实模型 token」改名为 credit；prompt / completion / total usage、上下文长度、模型 token limit、tokenizer 和 OpenAI 协议字段仍使用 token。
- 不修改受保护的项目名称、组织名称、版权、模块路径或品牌信息。

---

## 4. 术语与单位

| 名称 | 建议字段 | 单位 | 说明 |
|---|---|---|---|
| credit | `credit_limit` / `credit_used` / `credit_remaining` | credit | 套餐通用额度池的产品语义；现有底层字段可在实现计划中评估是否迁移 |
| 可信 usage | `has_trusted_usage` | boolean | 上游返回可解析且被当前适配器信任的 usage 对象 |
| 原始 usage token | `raw_metered_tokens` | token | 上游 usage 中的 token 数；无可信 usage 时为 `0` |
| 渠道 token 倍率 | `channel_token_billing_multiplier` | multiplier | usage token 模式下 raw usage token 到 credit 的换算倍率 |
| 固定请求扣费 | `fixed_request_credits` | credit/request | fixed request 模式下每个可计费请求扣的 credit |
| 基础扣费 | `base_credits` | credit | 动态倍率之前的扣费值 |
| 动态倍率 | `dynamic_billing_multiplier` | multiplier | 上游返回的明确倍率数，默认 `1` |
| 最终订阅扣费 | `subscription_credits` / `final_credits` | credit | 实际扣订阅 credit 的值 |
| API Key 扣费 | `api_key_credits` | credit | 实际扣 API Key credit cap 的值；默认等于最终订阅扣费 |

核心公式：

```text
if !chargeable or !has_trusted_usage:
  base_credits = 0
  api_key_credits = 0
  subscription_credits = 0
else:
  if credit_billing_mode == "usage_tokens":
    base_credits = apply_credit_multiplier(raw_metered_tokens, channel_token_billing_multiplier)
  if credit_billing_mode == "fixed_request":
    base_credits = fixed_request_credits

  api_key_credits = apply_credit_multiplier(base_credits, dynamic_billing_multiplier)
  subscription_credits = api_key_credits
```

说明：

- `chargeable = false` 用于免费模型、无需订阅扣费的请求或其它现有零扣费路径；这些路径必须产生标准零扣费结果，避免在多个调用点继续散落特殊分支。
- `has_trusted_usage` 是最终扣费闸门。无可信 usage 时，本地估算 token 不得转为最终扣费。
- fixed request 模式不是「无条件按 HTTP 请求扣费」，而是「有可信 usage 的成功请求按固定 credit 扣费」。
- `raw_metered_tokens = 0` 但 usage 对象存在时，`has_trusted_usage = true`；fixed request 模式仍可扣固定 credit，usage token 模式扣 `0 credit`。
- `dynamic_billing_multiplier` 缺失时按 `1`。
- `apply_credit_multiplier` 必须集中实现，禁止调用点自行取整。规则沿用现有渠道倍率语义：使用 decimal 计算，正数 half-up / half-away-from-zero 取整；输入 credit 或 token 为 `0` 时结果为 `0`；输入值 `> 0` 且倍率合法时，结果最小为 `1`，避免低倍率把可计费请求压成免费；负数输入按 `0` 处理或在调用方归零；`NaN`、`Inf`、`<= 0` 或超过上限的倍率必须拒绝或忽略，不能参与结算。

---

## 5. 数据模型设计

### 5.1 渠道字段

建议在 `model.Channel` 增加显式列，不放入 `settings` JSON。

```go
CreditBillingMode string `json:"credit_billing_mode" gorm:"not null;default:'usage_tokens'"`
FixedRequestCredits int64 `json:"fixed_request_credits" gorm:"not null;default:0"`
DynamicBillingMultiplierEnabled bool `json:"dynamic_billing_multiplier_enabled" gorm:"not null;default:false"`
```

保留现有字段：

```go
TokenBillingMultiplier float64 `json:"token_billing_multiplier" gorm:"not null;default:1"`
```

字段语义：

- `credit_billing_mode = "usage_tokens"`：使用 `token_billing_multiplier`。
- `credit_billing_mode = "fixed_request"`：使用 `fixed_request_credits`。
- `dynamic_billing_multiplier_enabled = true`：该渠道允许适配器读取上游动态倍率。
- `dynamic_billing_multiplier_enabled = false`：忽略上游倍率字段，固定按 `1`。

### 5.2 为什么使用显式列

显式列优于 JSON 设置：

- 这是计费核心字段，不是普通渠道附加设置。
- 展示层、日志层和 API 派生视图需要汇总启用渠道的 billing profile。
- 管理端渠道列表、筛选、复制、批量导入都需要稳定字段。
- SQLite / MySQL / PostgreSQL 下普通列兼容性最好。

### 5.3 校验与更新规则

渠道创建、更新、复制、批量导入都必须共用同一校验逻辑。

```text
credit_billing_mode ∈ {"usage_tokens", "fixed_request"}
0 < token_billing_multiplier <= 100
fixed_request 模式下：fixed_request_credits > 0
usage_tokens 模式下：fixed_request_credits 可以为 0，且不参与扣费
```

动态倍率运行时校验：

```text
0 < dynamic_billing_multiplier <= 100
禁止 NaN / Inf
```

无效动态倍率不能用于降低扣费；实现可选择忽略并审计，或在响应尚未写出时返回错误，但不得把非法值应用到结算。

更新 / patch 必须处理 Go 和 GORM 零值语义：

- `credit_billing_mode`、`fixed_request_credits`、`dynamic_billing_multiplier_enabled` 必须支持 raw presence 解析，区分「未传字段」和「显式传 0 / false / 空字符串」。
- 更新渠道时必须用 `Select` 或 `map[string]any` 明确写入这些字段；不得依赖 GORM struct updates 自动跳过零值。
- 未传字段时保留旧值；显式非法值必须拒绝；显式关闭 `dynamic_billing_multiplier_enabled=false`、切回 usage 模式时清零 `fixed_request_credits=0` 都必须能落库。
- 复制渠道默认继承 billing profile；批量导入未传字段使用默认值，显式字段走相同校验。

### 5.4 credit 命名策略

产品语义从 token 改为 credit，且范围不局限于套餐页面。

必须改为 credit 的表达：

- 用户端：套餐额度、当前订阅额度、已用额度、剩余额度、续订 / 升级提示、用量日志中的扣费值、账单或钱包中的消耗值。
- 管理端：订阅套餐配置、用户订阅详情、日志详情、统计卡片、导出列、渠道 fixed request 配置说明、API Key 额度消耗说明。
- API / DTO：新增或本次改动的字段优先使用 `credit_*` 命名，例如 `credit_billing_mode`、`fixed_request_credits`、`base_credits`、`final_credits`、`credit_limit`、`credit_used`、`credit_remaining`。
- i18n：所有支持语言中，原来描述通用额度池的 `token` 文案必须同步改为 `credit` 语义。

必须保留 token 的表达：

- 上游 usage token：`prompt_tokens`、`completion_tokens`、`total_tokens`、`raw_metered_tokens`。
- 模型能力：上下文长度、最大输出 token、模型 token limit、tokenizer。
- 派生权益：usage token 渠道显示「等价可用 token」，因为它描述的是该渠道可用的真实模型 token 量。
- 兼容协议：OpenAI-compatible 请求 / 响应中的标准 token 字段名。

既有 DB 物理列如 `monthly_token_limit`、`token_limit`、`token_used` 是否重命名为 `credit_*`，应在实现计划中单独评估迁移成本；但这些旧字段名如果继续存在，只能作为内部存储细节，不得继续驱动用户可见文案或新增 API 契约。

---

## 6. 后端运行时设计

### 6.1 请求级 billing snapshot

在 `relay/common/relay_info.go` 中冻结请求级 billing profile，避免管理员修改渠道配置影响已开始请求。

建议快照字段：

```go
CreditBillingMode string
ChannelTokenBillingMultiplier float64
FixedRequestCredits int64
DynamicBillingMultiplierEnabled bool
DynamicBillingMultiplier float64
DynamicBillingMultiplierSource string
```

默认值：

```text
credit_billing_mode = "usage_tokens"
channel_token_billing_multiplier = 1
dynamic_billing_multiplier_enabled = false
dynamic_billing_multiplier = 1
dynamic_billing_multiplier_source = "default"
```

要求：

- snapshot 必须在订阅预扣和 API Key credit cap 预扣前冻结。
- retry 过程中不得隐式覆盖已冻结的 billing profile。
- 进入预扣后，retry 只能选择与冻结 billing profile 兼容的渠道，这是当前必做规则，不是未来优化。
- 兼容性字段：`credit_billing_mode` 必须相同；`usage_tokens` 模式比较 `channel_token_billing_multiplier`；`fixed_request` 模式比较 `fixed_request_credits`；`dynamic_billing_multiplier_enabled` 必须相同，避免一个请求在不同渠道上对同一上游倍率字段有不同信任策略。
- 没有兼容候选时，沿用当前策略停止 retry 并返回原渠道失败错误；不得为了 retry 成功而切到不同计费 profile。

### 6.2 统一结算结果函数

新增统一计算函数，所有调用点只能消费它的结果。

建议签名：

```go
type CreditBillingInput struct {
    Chargeable bool
    HasTrustedUsage bool
    RawMeteredTokens int64
    CreditBillingMode string
    ChannelTokenBillingMultiplier float64
    FixedRequestCredits int64
    DynamicBillingMultiplier float64
    DynamicBillingMultiplierSource string
}

type CreditBillingResult struct {
    Chargeable bool
    HasTrustedUsage bool
    RawMeteredTokens int64
    CreditBillingMode string
    ChannelTokenBillingMultiplier float64
    FixedRequestCredits int64
    BaseCredits int64
    DynamicBillingMultiplier float64
    DynamicBillingMultiplierSource string
    APIKeyCredits int64
    SubscriptionCredits int64
    ZeroReason string
}

func CalculateCreditBillingResult(input CreditBillingInput) (CreditBillingResult, error)
```

职责边界：

- 只计算 credit，不读数据库。
- 不访问 Gin context。
- 不处理旧 quota / 金额。
- 负责校验倍率、取整、fixed request、免费模型 / 零扣费和无 usage 闸门。
- 返回完整中间值给订阅、API Key credit cap 和日志复用。
- 默认 `APIKeyCredits == SubscriptionCredits`。如果未来出现订阅专属调整，必须显式扩展结果字段和日志，不得隐藏在 Codex Pro 或 service 分支里。

必须替换 / 收敛的现有逻辑：

- `service.NewAPIBillingFromUsage(...)` 中的 Codex Pro 特征倍率。
- `service.PostTextConsumeQuota(...)` 中订阅 token 计算。
- `codexProAdjustedSubscriptionTokens(...)` 之类的局部额外乘法。
- WebSocket / Realtime / Audio 中重复的 token 结算换算。
- 日志中自行重算 billing multiplier 的逻辑。

### 6.3 预扣策略

预扣仍是保守估算，最终以结算结果为准。

usage token 模式：

```text
estimated_raw_tokens = relayInfo.GetEstimatePromptTokens()
preconsume_credits = round(estimated_raw_tokens * channel_token_billing_multiplier)
```

fixed request 模式：

```text
preconsume_credits = fixed_request_credits
```

动态倍率不要求在预扣阶段提前知道。它和最终 usage 一样，在结算阶段基于上游返回结果调整差额。

结算：

```text
settle_delta = final_credits - preconsume_credits
```

无可信 usage：

```text
final_credits = 0
settle_delta = -preconsume_credits
```

这保持现有「无 usage 不扣费」语义。

### 6.3.1 Realtime / WebSocket 增量 usage

Realtime / WebSocket 路径可能在同一个请求内收到多次 usage 增量。fixed request 的语义仍然是「每个可计费请求固定扣一次 credit」，不是「每个 usage chunk / event 扣一次」。

要求：

- `usage_tokens` 模式可以继续按增量 usage 累计 raw usage，并按统一 helper 计算增量或最终差额。
- `fixed_request` 模式初始只预留一次 `fixed_request_credits`。
- 同一请求内多次可信 usage 增量只用于标记 `HasTrustedUsage=true`、累计 raw usage 供日志 / 审计，不得重复产生 `base_credits`。
- fixed request 模式必须通过请求级幂等标记保证只结算一次 `fixed_request_credits × dynamic_billing_multiplier`，订阅 credit 与 API Key credit cap 都使用同一个一次性结果。
- 如果请求结束前始终没有可信 usage，则仍按无 usage 规则退回预扣。

### 6.4 可信 usage 闸门

需要显式区分以下状态：

| 状态 | chargeable | has_trusted_usage | raw_metered_tokens | usage_tokens 模式 | fixed_request 模式 |
|---|---:|---:|---:|---:|---:|
| 免费模型 / 不可扣费请求 | false | 任意 | 任意 | 标准零扣费结果 | 标准零扣费结果 |
| 上游没有 usage | true | false | 0 | 退回预扣，最终 0 credit | 退回预扣，最终 0 credit |
| usage 解析失败 / 不可信 | true | false | 0 | 退回预扣，最终 0 credit | 退回预扣，最终 0 credit |
| usage 存在且 total 为 0 | true | true | 0 | 最终 0 credit | 扣 fixed_request_credits |
| usage 存在且 total > 0 | true | true | total | 按倍率扣 credit | 扣 fixed_request_credits |

`has_trusted_usage` 的来源必须是独立 settlement metadata，由各 adapter 或响应解析层在成功解析上游 usage 对象时设置；不得由 `TotalTokens > 0`、`raw_metered_tokens > 0` 或本地估算反推。本地估算 token、解析失败、兼容层补出的空 usage 都必须保持 `has_trusted_usage = false`，除非确实来自上游可信 usage 对象。

本地估算 token 只用于预扣和审计，不得把 `has_trusted_usage` 从 `false` 改成 `true`。

---

## 7. 动态倍率设计

### 7.1 标准承载字段

在 `RelayInfo` 或等价 settlement metadata 中承载动态倍率：

```go
DynamicBillingMultiplier float64
DynamicBillingMultiplierSource string
```

只有在 `DynamicBillingMultiplierEnabled` 为 `true` 时，适配器才读取上游返回并写入该字段。

默认：

```text
dynamic_billing_multiplier = 1
dynamic_billing_multiplier_source = "default"
```

### 7.2 推荐上游协议

非流式响应体可使用：

```json
{
  "usage": {
    "total_tokens": 10000
  },
  "newapi_billing": {
    "billing_multiplier": 1.5,
    "billing_multiplier_source": "priority_tier"
  }
}
```

Header / trailer 可使用：

```http
X-NewAPI-Billing-Multiplier: 1.5
X-NewAPI-Billing-Multiplier-Source: priority_tier
```

要求：

- `billing_multiplier` 必须是数字，支持小数。
- 现有 `dto.NewAPIBilling.BillingMultiplier` 若是整数，应改为能表达小数的类型。
- source 是审计字段，不参与计算。
- 未启用动态倍率的渠道必须忽略这些字段。

### 7.3 标准化责任

动态倍率解析必须发生在渠道 adapter / relay response 标准化层，而不是 service 结算层。

要求：

- 提供共享 helper，从非流式响应体、SSE final event、HTTP header、HTTP trailer 中解析 `billing_multiplier` 和 `billing_multiplier_source`。
- helper 必须先检查请求冻结的 `DynamicBillingMultiplierEnabled`。未启用时忽略上游字段，并记录 ignored reason 供日志审计。
- helper 必须做数值校验，拒绝 `<= 0`、`NaN`、`Inf`、超过上限或非数字值。非法值不得降低扣费。
- helper 解析成功后只写入 `RelayInfo` 或等价 settlement metadata；service 层只读取标准化后的 metadata，不直接解析上游 JSON、header 或 trailer。
- 首批覆盖范围至少包括 OpenAI-compatible Chat Completions / Responses 路径、流式 final event 或 trailer、Realtime / WebSocket 与 Audio 进入统一结算函数的路径。无法在第一版覆盖的 provider 必须显式保持 `dynamic_billing_multiplier = 1`，不得半支持。

### 7.4 Codex Pro 收敛规则

后续不再允许服务层多处写死：

```text
if CodexProServed { multiplier *= 2 }
```

推荐新规则：

- `X-NewAPI-Pro-Served` 仍可作为「上游实际服务形态」日志标记。
- 扣费倍率只来自明确数值字段，例如 `X-NewAPI-Billing-Multiplier: 2` 或响应体 `newapi_billing.billing_multiplier = 2`。
- 如果没有明确数值倍率，则 `dynamic_billing_multiplier = 1`。
- 第一版不做 Codex Pro legacy 特征到倍率的隐式兼容转换，避免重新引入「上游特征 -> 本地写死倍率」。如果未来必须兼容旧特征，也只能在 adapter 标准化点转换为显式 dynamic multiplier，并同时明确它是否影响 API Key credit cap；不得在 service 层多点乘 `2`。

---

## 8. 套餐与用户展示 API

### 8.1 后端派生视图

后端继续负责生成按渠道的派生展示数据，前端不应拉全量渠道自行计算。

建议从旧 `channel_token_equivalents` 升级为 credit 语义：

```go
type PlanChannelCreditEquivalent struct {
    Kind string `json:"kind"` // usage_tokens | fixed_request | unlimited
    ValueType string `json:"value_type"` // usage_tokens/fixed_request 必填：single | range；unlimited 使用 unlimited
    ChannelType int `json:"channel_type"`
    ChannelTypeName string `json:"channel_type_name"`
    VariantCount int `json:"variant_count"`

    // usage_tokens + single
    Multiplier float64 `json:"multiplier,omitempty"`
    EquivalentTokenLimit int64 `json:"equivalent_token_limit,omitempty"`

    // usage_tokens + range
    MinMultiplier float64 `json:"min_multiplier,omitempty"`
    MaxMultiplier float64 `json:"max_multiplier,omitempty"`
    EquivalentTokenLimitMin int64 `json:"equivalent_token_limit_min,omitempty"`
    EquivalentTokenLimitMax int64 `json:"equivalent_token_limit_max,omitempty"`

    // fixed_request + single
    FixedRequestCredits int64 `json:"fixed_request_credits,omitempty"`
    EquivalentRequestLimit int64 `json:"equivalent_request_limit,omitempty"`

    // fixed_request + range
    FixedRequestCreditsMin int64 `json:"fixed_request_credits_min,omitempty"`
    FixedRequestCreditsMax int64 `json:"fixed_request_credits_max,omitempty"`
    EquivalentRequestLimitMin int64 `json:"equivalent_request_limit_min,omitempty"`
    EquivalentRequestLimitMax int64 `json:"equivalent_request_limit_max,omitempty"`

    CreditUnlimited bool `json:"credit_unlimited,omitempty"`
}
```

用户当前订阅摘要增加 remaining 视图：

```go
type SubscriptionChannelCreditEquivalent struct {
    Kind string `json:"kind"` // usage_tokens | fixed_request | unlimited
    ValueType string `json:"value_type"` // usage_tokens/fixed_request 必填：single | range；unlimited 使用 unlimited
    ChannelType int `json:"channel_type"`
    ChannelTypeName string `json:"channel_type_name"`
    VariantCount int `json:"variant_count"`

    // usage_tokens + single
    Multiplier float64 `json:"multiplier,omitempty"`
    EquivalentTokenLimit int64 `json:"equivalent_token_limit,omitempty"`
    EquivalentTokenRemaining int64 `json:"equivalent_token_remaining,omitempty"`

    // usage_tokens + range
    MinMultiplier float64 `json:"min_multiplier,omitempty"`
    MaxMultiplier float64 `json:"max_multiplier,omitempty"`
    EquivalentTokenLimitMin int64 `json:"equivalent_token_limit_min,omitempty"`
    EquivalentTokenLimitMax int64 `json:"equivalent_token_limit_max,omitempty"`
    EquivalentTokenRemainingMin int64 `json:"equivalent_token_remaining_min,omitempty"`
    EquivalentTokenRemainingMax int64 `json:"equivalent_token_remaining_max,omitempty"`

    // fixed_request + single
    FixedRequestCredits int64 `json:"fixed_request_credits,omitempty"`
    EquivalentRequestLimit int64 `json:"equivalent_request_limit,omitempty"`
    EquivalentRequestRemaining int64 `json:"equivalent_request_remaining,omitempty"`

    // fixed_request + range
    FixedRequestCreditsMin int64 `json:"fixed_request_credits_min,omitempty"`
    FixedRequestCreditsMax int64 `json:"fixed_request_credits_max,omitempty"`
    EquivalentRequestLimitMin int64 `json:"equivalent_request_limit_min,omitempty"`
    EquivalentRequestLimitMax int64 `json:"equivalent_request_limit_max,omitempty"`
    EquivalentRequestRemainingMin int64 `json:"equivalent_request_remaining_min,omitempty"`
    EquivalentRequestRemainingMax int64 `json:"equivalent_request_remaining_max,omitempty"`

    CreditUnlimited bool `json:"credit_unlimited,omitempty"`
}
```

前端 TypeScript 必须按 `kind + value_type` 建模为 discriminated union，而不是靠 optional 字段猜测形态。`kind = "usage_tokens"` 或 `kind = "fixed_request"` 时，`value_type` 必须返回 `single` 或 `range`，不得省略；`kind = "unlimited"` 时，`value_type` 固定返回 `unlimited`。`equivalent_token_*` 仅在 `usage_tokens` 分支表示「由 credit 池折算出的可用 usage token」，不得作为通用额度 / credit 展示；`fixed_request` 分支只能显示 `equivalent_request_*`。

### 8.2 展示规则

通用 credit：

```text
Monthly credits: 1,000,000
Remaining credits: 600,000
```

usage token 渠道：

```text
OpenAI: 500,000 tokens
Claude: 300,000 - 500,000 tokens
```

fixed request 渠道：

```text
Codex: 12 requests
Realtime: 7 requests remaining
```

计算规则：

```text
equivalent_request_limit = floor(credit_limit / fixed_request_credits)
equivalent_request_remaining = floor(credit_remaining / fixed_request_credits)
```

不使用「约」。如果不能整除，展示完整可用次数；例如 `1,000,000 / 80,000 = 12`，不是 `12.5`，也不是「约 12」。

### 8.3 多配置聚合

同一 channel type 下可能同时存在多个 billing profile。

usage token 模式：

- 同倍率：`kind = "usage_tokens"`，返回单值。
- 多倍率：返回 token range，保守值对应最大倍率，乐观值对应最小倍率。

fixed request 模式：

- 同 fixed credit：返回单个 request count。
- 多 fixed credit：返回 request count range，保守值对应最大 fixed credit，乐观值对应最小 fixed credit。

同一 channel type 同时存在 usage token 与 fixed request：

- 不合并成一个 range。
- 返回两条展示项，分别标明 `kind`。
- 前端分两行展示，避免把 token 和请求次数混在一起。

### 8.4 API 挂载位置

现有接口需要返回派生展示：

- `GET /api/subscription/plans`
- `GET /api/subscription/public/plans`
- `GET /api/subscription/self`

建议新增字段名使用 credit：

```json
{
  "plan": {
    "credit_limit": 1000000,
    "channel_credit_equivalents": [
      {
        "kind": "usage_tokens",
        "value_type": "single",
        "channel_type": 1,
        "channel_type_name": "OpenAI",
        "multiplier": 2,
        "equivalent_token_limit": 500000
      },
      {
        "kind": "usage_tokens",
        "value_type": "range",
        "channel_type": 14,
        "channel_type_name": "Claude",
        "min_multiplier": 1.5,
        "max_multiplier": 2,
        "equivalent_token_limit_min": 500000,
        "equivalent_token_limit_max": 666666
      },
      {
        "kind": "fixed_request",
        "value_type": "single",
        "channel_type": 99,
        "channel_type_name": "Codex",
        "fixed_request_credits": 80000,
        "equivalent_request_limit": 12
      },
      {
        "kind": "fixed_request",
        "value_type": "range",
        "channel_type": 100,
        "channel_type_name": "Realtime",
        "fixed_request_credits_min": 80000,
        "fixed_request_credits_max": 100000,
        "equivalent_request_limit_min": 10,
        "equivalent_request_limit_max": 12
      }
    ]
  }
}
```

如果实现阶段决定保留旧 API 字段名以降低破坏面，则前端仍必须把通用额度显示为 credit，并让 fixed request 渠道展示请求次数。

---

## 9. 管理端配置

渠道创建 / 编辑抽屉增加：

```text
Credit billing mode:
  - By usage tokens
  - Fixed per request

Token billing multiplier:
  visible when mode = usage_tokens

Fixed request credits:
  visible when mode = fixed_request

Accept upstream dynamic billing multiplier:
  switch, default off
```

文案建议：

- `By usage tokens`：根据上游返回的 usage token 和渠道倍率扣 credit。
- `Fixed per request`：当上游返回可信 usage 时，每个请求固定扣指定 credit；无 usage 时不扣费。
- `Accept upstream dynamic billing multiplier`：仅信任该渠道上游返回的明确倍率数，默认关闭。

前端校验必须与后端一致：

- `token_billing_multiplier`：`0 < value <= 100`
- `fixed_request_credits`：fixed request 模式下必须为正整数
- 动态倍率开关默认关闭

---

## 10. 日志与审计

日志 `other` 建议增加：

```json
{
  "credit_billing_mode": "fixed_request",
  "has_trusted_usage": true,
  "raw_metered_tokens": 10000,
  "channel_token_billing_multiplier": 1,
  "fixed_request_credits": 80000,
  "base_credits": 80000,
  "dynamic_billing_multiplier": 1.5,
  "dynamic_billing_multiplier_source": "upstream_newapi_billing",
  "api_key_credits": 120000,
  "subscription_credits": 120000,
  "final_credits": 120000,
  "subscription_credits_consumed": 120000,
  "api_key_credits_consumed": 120000
}
```

要求：

- 历史日志不回填。
- 新日志记录请求时的 billing snapshot，不展示时动态读取渠道当前配置。
- 无可信 usage 时：`has_trusted_usage = false`、`final_credits = 0`、`subscription_credits = 0`、`api_key_credits = 0`。
- 动态倍率被忽略时要记录原因，例如渠道未启用或倍率非法。
- 用户端日志展示 `Consumption multiplier` 时，应优先展示 `dynamic_billing_multiplier` 和 source；同时展示 billing mode 和 final credits。
- `logs.quota`、`users.used_quota`、`channels.used_quota`、`PriceData.Quota` / `QuotaToPreConsume` 继续表示 legacy quota / 成本口径，不得改成 credit，也不得作为用户可见 credit 消耗展示来源。
- 用户端和管理端 credit 消耗统计必须读取 `final_credits`、`subscription_credits_consumed`、`api_key_credits_consumed` 或后续明确新增的 credit 列 / 聚合字段；如果第一版仍从 `other` 聚合，接口必须显式解析这些 credit 字段，避免混用 legacy quota。

### 10.1 API Key credit cap 兼容策略

现有 API Key 限制内部字段如 `TokenLimitEnabled`、`TokenLimit`、`TokenUsed` 可以暂时作为兼容存储细节保留，但产品语义必须迁移为 credit cap。

要求：

- 新增或改动的 API 响应优先返回 `credit_limit_enabled`、`credit_limit`、`credit_used`、`credit_remaining`、`credit_reset_at` 等 credit 字段。
- 旧 `token_limit*` 字段如需保留，只能作为 deprecated 兼容字段；前端必须读取 credit 字段展示。
- `TokenLimitSession` 或等价服务层结算必须消费统一 `APIKeyCredits`，不得继续把通用额度称为 token cap。
- 管理端和用户端所有 API Key cap 文案必须使用 credit，真实 API key 字符串 / Access Token / bearer token 等鉴权概念不得改名。

---

## 11. 与 billing expression 的关系

本规格不修改 `pkg/billingexpr` 的职责边界。

原因：

- billing expression 是模型价格表达式系统，输出主要服务旧 quota / 成本口径。
- fixed request 是渠道级 credit 消费规则。
- dynamic multiplier 依赖上游响应结算元数据，而现有表达式主要读取请求 body/header 和 usage。

后续如果要让表达式读取上游倍率，应作为单独的 billing expression 版本演进讨论，不与本功能混做。

---

## 12. 迁移与兼容策略

### 12.1 数据迁移

新增渠道列应默认保持现有行为：

```text
credit_billing_mode = "usage_tokens"
fixed_request_credits = 0
dynamic_billing_multiplier_enabled = false
token_billing_multiplier = 既有值或默认 1
```

迁移要求：

- 不回算历史日志。
- 不回算历史订阅订单。
- 不修改历史套餐额度。
- 不使用数据库特有 SQL；遵循 `model/main.go` 现有迁移模式。

### 12.2 全面文案与命名迁移

本次迁移不是只改套餐页面。前端、后端响应、日志展示、管理端和 i18n 都必须把「通用额度池」从 token 统一为 credit。

必须逐项审查并迁移的 default 前端入口：

- `features/subscriptions`：套餐配置、购买弹窗、用户订阅详情、套餐表单。
- `features/wallet`：当前订阅摘要、钱包 / 账单提示、套餐卡片。
- `features/home`：公开套餐预览；API 示例里的 `usage.total_tokens` 保留 token。
- `features/keys`：API Key credit limit / used / remaining / reset / status。
- `features/usage-logs`：扣费 / 订阅消费列、详情、tooltip；真实 prompt / completion / total / metered tokens 保留 token。
- `features/usage-analytics` 与 `features/admin-analytics`：quota / consumption 指标中表示通用额度扣费的项改为 credit；真实 total tokens、TPM、tokens per second 保留 token。
- `features/dashboard`：订阅额度 / 扣费概览改为 credit；模型 usage token 指标保留 token。
- `features/channels`：billing profile 表单说明、fixed request 和动态倍率相关文案。
- 导出字段、通知、toast、错误提示、空状态、表格列、tooltip 中描述通用额度池的旧 token 文案。

i18n 要求：

- 必须同时更新 `web/default/src/i18n/static-keys.ts` 与 `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`。
- 所有新增 / 改名的 label、tooltip、toast、FormMessage / Zod 错误、状态枚举和常量文案必须通过 `t()` 或 `labelKey` 渲染，禁止直接写英文最终文案。
- 从 `web/default` 运行 `bun run i18n:sync`，并检查 `web/default/src/i18n/locales/_reports/_sync-report.json` 中本次新增 key 没有 missing / untranslated。

推荐用户可见文案：

- `Monthly credits`
- `Credits remaining`
- `Credits used`
- `Credits consumed`
- `Fixed credits per request`
- `Equivalent tokens by channel`（仅用于 usage token 渠道派生权益）
- `Requests by channel`（仅用于 fixed request 渠道派生权益）

不得改名的 UI / API：

- `Prompt tokens`
- `Completion tokens`
- `Total tokens`
- `Metered tokens`、cache / audio / image tokens 等真实 usage token 指标。
- `Max tokens`、`Context tokens`、`Tokenizer`、`max_tokens`、`max_input_tokens`。
- `Total Tokens`、TPM、tokens per second 等模型吞吐 / usage 指标。
- OpenAI-compatible 示例字段，例如 `usage.total_tokens`。
- API key credential、Access Token、bearer token 等鉴权概念。

### 12.3 API 命名迁移

原则：本次新增或被本功能触达的 API / DTO 字段必须优先使用 `credit_*` 命名；旧 `token_*` 字段只能作为兼容字段或内部存储细节。

要求：

- 如果某个响应同时保留旧 `token_limit` 和新增 `credit_limit`，前端必须读取 `credit_limit` 并把 `token_limit` 标记为兼容字段。
- 新增派生字段使用 `channel_credit_equivalents`，不要继续扩展旧 `channel_token_equivalents` 表达通用额度。
- 日志展示 API 中，订阅扣费相关新增字段使用 `*_credits_*` 或 `final_credits`，真实 usage 仍使用 `*_tokens`。
- 实现计划必须列出所有受影响 DTO 和前端类型，不得只覆盖套餐接口。

无论底层 DB 是否立即重命名，用户可见层、管理可见层、日志可见层和新增 API 契约都必须完成 credit 命名切换。

---

## 13. 验收标准

后续实现完成后必须满足：

1. usage token 渠道：
   - raw usage `10,000`，渠道倍率 `2`，动态倍率关闭，最终扣 `20,000 credit`。
2. fixed request 渠道：
   - fixed `80,000 credit`，可信 usage 存在，动态倍率关闭，最终扣 `80,000 credit`。
3. fixed request + 动态倍率：
   - fixed `80,000 credit`，上游倍率 `1.5`，渠道启用动态倍率，最终扣 `120,000 credit`。
4. 无 usage：
   - 任意 billing mode，最终扣 `0 credit`，预扣退回。
5. 可信 usage 存在但 total tokens 为 0：
   - usage token 模式扣 `0 credit`。
   - fixed request 模式扣 `fixed_request_credits`。
6. Realtime / WebSocket fixed request：
   - 同一个请求内收到多次可信 usage 增量时，usage token 模式可累计 usage，fixed request 模式只扣一次 `fixed_request_credits`。
   - API Key credit cap 和订阅 credit 对 fixed request 使用同一个一次性结算结果。
7. 动态倍率未启用：
   - 上游返回 `billing_multiplier = 1.5`，渠道开关关闭，最终按 `1` 处理。
8. 非法动态倍率：
   - `0`、负数、NaN、Inf、超过上限不得应用到结算。
9. 取整边界：
   - `raw = 1`、倍率 `0.1` 的可计费 usage token 请求最终至少扣 `1 credit`。
   - `base_credits = 1`、动态倍率 `0.1` 的可计费请求最终至少扣 `1 credit`。
   - 输入为 `0` 时结果为 `0 credit`。
10. 全面 credit 表达：
   - 所有通用额度池、扣费、已用、剩余、预扣、退款、API Key 消费额度的用户可见和管理员可见表达都使用 credit。
   - `features/subscriptions`、`wallet`、`home`、`keys`、`usage-logs`、`usage-analytics`、`admin-analytics`、`dashboard`、`channels` 中触达的通用额度表达均完成迁移。
   - `static-keys.ts` 与 `en/zh/fr/ja/ru/vi` locale 同步，无本次新增 key 的 missing / untranslated。
   - usage token 渠道仍显示等价可用 token 量。
   - fixed request 渠道显示请求次数。
   - `1,000,000 credit / 80,000 credit/request` 显示 `12 requests`，不显示「约」。
11. 日志：
    - 能看到 billing mode、base credits、dynamic multiplier、source、API key credits、subscription credits、final credits。
    - legacy quota 字段仍保持成本口径，不能作为 credit 展示来源。
12. Codex Pro：
    - 不存在多个服务层分支重复乘固定倍率；无明确 numeric multiplier 时按 `1`。
13. 数据库：
    - SQLite、MySQL、PostgreSQL 均能完成迁移和基本读写。

---

## 14. 推荐测试覆盖

后续实现计划应至少覆盖：

- `creditbilling` helper 单元测试：
  - usage token 模式倍率取整。
  - fixed request 模式。
  - `Chargeable=false` 标准零扣费。
  - `HasTrustedUsage=false` 无 usage 闸门。
  - `HasTrustedUsage=true && RawMeteredTokens=0` 的 usage token / fixed request 差异。
  - 动态倍率。
  - 非法倍率。
- `service/text_quota.go` 和 API Key cap 相关结算测试：
  - 预扣后实际 usage 存在。
  - 预扣后 usage 缺失退款。
  - usage object present + total tokens 为 0。
  - 动态倍率启用 / 禁用。
  - `APIKeyCredits` 与 `SubscriptionCredits` 均来自统一结果。
  - Realtime / WebSocket 同一请求多次 usage 增量时，fixed request 只扣一次。
  - fixed request 的一次性结果同时驱动 API Key credit cap 与订阅 credit。
- relay / adapter 动态倍率解析测试：
  - body `newapi_billing.billing_multiplier`。
  - header / trailer `X-NewAPI-Billing-Multiplier`。
  - 启用 / 禁用开关。
  - 非法值和 source 记录。
- Codex Pro 回归测试：
  - 确认没有重复倍率。
  - 确认无明确 numeric multiplier 时不按旧特征写死扣费。
- 套餐等价展示测试：
  - usage token 等价 token。
  - fixed request 等价请求次数。
  - 同渠道类型多配置 range。
  - token 和 request 不混合。
- 前端展示测试：
  - credit 文案。
  - request count 不带「约」。
  - fixed request 渠道 tooltip / detail 显示 fixed request credits。
  - 真实 usage token 文案保留 token。
  - API Key cap 显示 credit。
- 验证命令应在实现计划中落到具体任务，至少包括定向 Go 单测、`cd web/default && bun run i18n:sync`、`cd web/default && bun run typecheck`。

---

## 15. 后续实现建议顺序

1. 建立 `creditbilling` 统一计算 helper，并用测试锁定公式。
2. 给渠道模型和 API 增加 billing profile 字段与校验。
3. 在 `RelayInfo` 冻结 billing profile、可信 usage 和动态倍率元数据。
4. 收敛文本 / WSS / Audio / Codex Pro / API Key cap 的扣费计算到统一 helper。
5. 实现上游动态倍率解析，且只在渠道显式启用时生效。
6. 扩展 subscription credit equivalents 后端派生视图。
7. 修改 default 前端渠道配置、订阅 / wallet / home / keys / logs / analytics / dashboard 文案和类型，完成 credit 迁移。
8. 补齐日志、i18n、测试和迁移验证。

实现计划必须拆分低冲突任务边界，并指定每个共享文件的唯一主责子代理；至少包含一个 spec-compliance reviewer 逐项核对本规格验收标准。
