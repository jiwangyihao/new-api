# API Key token 限额去价格化规格

> 面向 AI 代理的工作者：本规格用于指导后续在 `C:/Users/34404/source/repos/new-api` 上清理 API Key 限额的旧余额 / quota 口径。实现前必须读取仓库根目录 `AGENTS.md` 与 `web/default/AGENTS.md`，遵守 Go + Gin + GORM、React 19 + TypeScript + Bun、SQLite / MySQL / PostgreSQL 全兼容约束。若后续进入实现阶段，必须先创建实现计划并按 TDD 执行。

**目标：** API Key 限额从旧余额 / quota 计价口径切换为明确的 token 限额口径。历史 API Key 限额不做换算，全部置空为「未设置 token 限额」；新配置只允许按 token 数设置。

**核心决策：** 历史 API Key 的旧 `remain_quota` / `used_quota` / `unlimited_quota` 不迁移为新 token 限额。迁移后所有既有 API Key 的新 token 限额均为空，即不启用单 key token cap。管理员和用户如需限制单个 API Key，必须手动重新设置 token 限额。

**架构：** 保留旧 API Key quota 字段作为兼容数据，不再作为 default 前端限额来源，也不参与新运行时限额。后端新增 API Key token cap 字段与预扣 / 结算会话；主请求仍消耗订阅套餐 token，API Key token cap 只作为额外 guard。前端 default 主题的 API Key 表单、列表和 usage 接口统一展示 token 数，不再使用 `formatQuota()`、`parseQuotaFromDollars()` 或 `QuotaPerUnit`。

**技术栈：** Go 1.25.1、Gin、GORM v2、SQLite / MySQL / PostgreSQL、React 19、TypeScript、TanStack Query、Rsbuild、Bun、i18next。

---

## 1. 背景与当前事实

当前项目已经废弃「请求 token 消耗先按模型价格转换成余额，再从用户或 API Key 余额扣费」的模式。API 请求的资金来源已经收敛为订阅套餐 token：

```text
API 请求 -> 订阅 token 预扣 -> 上游请求 -> 按实际 usage 结算订阅 token
```

已静态确认的当前事实：

1. `service/billing_session.go` 注释明确请求资金来源为 subscription-only，token key quota 不参与请求预扣、结算或退款。
2. `service/funding_source.go` 的 `WalletFunding` 对正数预扣、结算和退款返回 `ErrLegacyWalletFundingDisabled`。
3. `model/subscription.go` 的订阅预扣以 `TokenLimit` / `TokenUsed` 为主口径，`legacyAmount` 已被忽略。
4. `service/text_quota.go` 已按 `SubscriptionMeteredTokens(usage)` 结算订阅 token。
5. `tokens.remain_quota`、`tokens.used_quota`、`tokens.unlimited_quota` 仍存在，但不是主请求资金来源。

当前 API Key 残留问题：

- 后端 `model.Token` 仍有旧 `remain_quota`、`used_quota`、`unlimited_quota`。
- `controller/token.go` 的创建 / 更新接口仍用 `common.QuotaPerUnit` 校验 API Key 限额上限。
- `controller/token.go` 的 `GetTokenStatus()` 和 `GetTokenUsage()` 仍返回 OpenAI-like `credit_summary`，字段为 `total_granted`、`total_used`、`total_available`。
- `controller/config_guide.go` 遇到 `TokenStatusExhausted` 返回 `token quota exhausted`，与主鉴权允许 exhausted token 的行为不一致。
- `web/default/src/features/keys/lib/api-key-form.ts` 把表单字段命名为 `remain_quota_dollars`，并通过 `parseQuotaFromDollars()` / `quotaUnitsToDollars()` 做货币或旧 quota 换算。
- `web/default/src/features/keys/components/api-keys-columns.tsx` 和 `api-keys-table.tsx` 使用 `formatQuota()` 展示 API Key 限额。

结果是：用户在当前 token-only 产品语义下设置 API Key 限额时，仍需要理解旧余额、旧 quota、USD/CNY 或 `QuotaPerUnit` 换算，体验不一致且容易误配。

---

## 2. 目标与非目标

### 2.1 必须满足

- API Key 限额只按 token 数设置、存储、计算和展示。
- 历史 API Key 限额全部置空，不从旧 `remain_quota`、`used_quota` 或 `unlimited_quota` 自动换算。
- 迁移后既有 API Key 默认不启用新 token cap，避免错误限制历史用户。
- 新增 / 编辑 API Key 时，限额输入框单位固定为 token，不受 `quota_display_type`、`display_in_currency`、`usd_exchange_rate` 或 `quota_per_unit` 影响。
- API Key 列表展示 `token_used`、`token_remaining`、`token_limit`，使用 token 格式化。
- 请求运行时可以按单个 API Key token cap 做预扣、结算和退款。
- API Key token cap 只负责单 key 用量限制，不是资金来源；请求仍必须消耗订阅套餐 token。
- 旧字段保留兼容，不删除 `tokens.remain_quota`、`tokens.used_quota`、`tokens.unlimited_quota`。
- SQLite、MySQL、PostgreSQL 全兼容。
- default 前端完成新语义；相关 i18n 文案同步补齐。

### 2.2 非目标

- 不恢复旧余额 / quota 扣费模式。
- 不把账户余额重新用于 API 请求扣费。
- 不把旧 API Key `remain_quota` 自动解释为 token 数。
- 不删除 OpenAI 兼容 billing 接口。
- 不删除 `logs.quota`、模型价格、模型倍率、billing expression 或管理员成本分析能力。
- 不改动账户余额、充值、订阅套餐价格、渠道余额等金额语义功能。
- 不重写 classic 主题；classic 可继续依赖旧字段，除非后续另有明确需求。
- 不修改受保护的项目名称、组织名称、版权、模块路径或品牌信息。

---

## 3. 术语与单位

| 名称 | 字段 | 单位 | 用途 | 前端展示 |
|---|---|---|---|---|
| 账户余额 | `users.quota`，建议 API alias 为 `account_balance_cents` | CNY 分 | 购买订阅套餐 | 余额 / 金额 |
| 订阅 token | `user_subscriptions.token_limit` / `token_used` | token | 请求资金来源和套餐限额 | token |
| API Key token 限额 | 新增 `tokens.token_limit_enabled` / `token_limit` / `token_used` | token | 单个 API Key 的用量 cap | token |
| 旧 API Key quota | `tokens.remain_quota` / `used_quota` / `unlimited_quota` | legacy quota | 兼容字段、历史审计 | default 主题不展示为限额 |
| 成本 quota | `logs.quota`、`model_ratio`、`model_price` | legacy quota / 成本 | 管理员成本分析和历史统计 | 管理员成本视图 |

### 3.1 API Key token cap 的含义

API Key token cap 是「单个 key 的用量上限」，不是钱包、余额或付费来源。

请求必须同时满足：

```text
API Key 基础校验通过
AND API Key token cap 未超限（如果启用）
AND 用户订阅套餐 token 可扣费
```

### 3.2 历史限额置空的含义

「历史 API Key 限额全部置空」指迁移到新 token cap 后，既有 API Key 均显示为未设置 token 限额：

```text
token_limit_enabled = false
token_limit = 0
token_used = 0
```

不代表删除旧字段，也不要求把旧 `remain_quota` 清零。旧字段可以继续留在数据库里作为兼容数据；default 前端和新运行时不能再把它们当作 API Key 限额来源。

---

## 4. 数据模型设计

### 4.1 `model.Token` 新增字段

文件：`model/token.go`

新增字段：

```go
TokenLimitEnabled bool  `json:"token_limit_enabled" gorm:"not null;default:false"`
TokenLimit        int64 `json:"token_limit" gorm:"type:bigint;not null;default:0"`
TokenUsed         int64 `json:"token_used" gorm:"type:bigint;not null;default:0"`
```

字段语义：

- `token_limit_enabled = false`：该 API Key 未设置 token 限额，列表和表单展示为「不限」或空状态。
- `token_limit_enabled = true && token_limit > 0`：启用单 key token cap。
- `token_used`：新 token cap 生效后的累计使用 token。历史旧 `used_quota` 不迁入该字段。
- `token_remaining = max(0, token_limit - token_used)` 为响应层派生字段，不落库。
- 不允许 `token_limit_enabled = true && token_limit <= 0`。

### 4.2 数据迁移

迁移只新增列，不迁移旧数值。

所有既有 API Key 在迁移后统一为：

```text
token_limit_enabled = false
token_limit = 0
token_used = 0
```

明确禁止以下迁移：

```text
token_limit = remain_quota + used_quota
token_used = used_quota
token_limit_enabled = !unlimited_quota
token_limit = quotaUnitsToTokens(remain_quota)
```

原因：旧 API Key 限额可能来自 USD、CNY、custom currency、tokens-only display 或 legacy quota 换算，无法可靠反推用户真正想限制的 token 数。全部置空是唯一不会误伤历史用户的迁移策略。

### 4.3 旧字段保留规则

旧字段继续保留：

```go
RemainQuota    int  `json:"remain_quota"`
UsedQuota      int  `json:"used_quota"`
UnlimitedQuota bool `json:"unlimited_quota"`
```

要求：

- 新 API Key token cap 不读旧字段。
- `web/default` 不再展示旧字段作为限额。
- 旧字段只用于 classic 兼容、历史审计或旧客户端兼容响应。
- 新字段与旧字段不同步，避免再次制造双写语义。
- 新代码不得调用 `parseQuotaFromDollars()`、`quotaUnitsToDollars()` 或 `formatQuota()` 处理 API Key 限额。

### 4.4 响应层派生字段

建议新增 response DTO，而不是直接把 GORM model 暴露给 default 前端。

派生字段：

```go
type TokenLimitView struct {
    TokenLimitEnabled bool  `json:"token_limit_enabled"`
    TokenLimit        int64 `json:"token_limit"`
    TokenUsed         int64 `json:"token_used"`
    TokenRemaining    int64 `json:"token_remaining"`
    TokenUnlimited    bool  `json:"token_unlimited"`
}
```

派生规则：

```text
token_unlimited = !token_limit_enabled
token_remaining = token_limit_enabled ? max(0, token_limit - token_used) : 0
```

---

## 5. 后端接口契约

### 5.1 API Key 列表与详情

涉及接口：

- `GET /api/token/`
- `GET /api/token/:id`
- `GET /api/token/search`

响应应包含新字段：

```json
{
  "id": 123,
  "name": "default",
  "status": 1,
  "token_limit_enabled": true,
  "token_limit": 1000000,
  "token_used": 120000,
  "token_remaining": 880000,
  "token_unlimited": false,
  "remain_quota": 0,
  "used_quota": 0,
  "unlimited_quota": true
}
```

兼容规则：

- 可以继续返回旧 `remain_quota`、`used_quota`、`unlimited_quota`。
- `web/default` 必须优先读取新字段。
- 如果新字段缺失，`web/default` 按「未设置 API Key token 限额」处理，不能 fallback 到旧 `remain_quota`。

### 5.2 创建 API Key

当前接口：`POST /api/token/`

请求体新增：

```json
{
  "name": "key-a",
  "token_limit_enabled": true,
  "token_limit": 1000000,
  "expired_time": -1,
  "model_limits_enabled": false,
  "model_limits": "",
  "allow_ips": ""
}
```

校验规则：

- `token_limit_enabled = false` 时，`token_limit` 保存为 `0`。
- `token_limit_enabled = true` 时，`token_limit` 必须大于 `0`。
- `token_limit` 使用 `int64`。
- 业务上限使用明确的 token 上限，例如 `10_000_000_000_000`，不得再使用 `1000000000 * common.QuotaPerUnit`。
- default 前端不再提交 `remain_quota`、`remain_quota_dollars` 或 `unlimited_quota` 作为限额配置。
- 后端可以继续接受旧字段以兼容旧客户端，但旧字段不得影响新 token cap。

### 5.3 更新 API Key

当前接口：`PUT /api/token/`

规则：

- 更新 API Key 限额时只更新 `token_limit_enabled` 和 `token_limit`。
- 不因编辑限额隐式重置 `token_used`。
- 如果从启用限额切到未启用限额：
  - `token_limit_enabled = false`
  - `token_limit = 0`
  - `token_used` 保留，作为新 token cap 使用历史的审计数据。
- 如果从未启用切到启用：
  - `token_used` 从当前新字段值继续累计。
  - 不从旧 `used_quota` 填充。
- 如果 `token_used > token_limit`，允许保存，但该 API Key 在后续请求中立即被判定为 token cap exhausted；前端应提示当前已超限。

### 5.4 重置 API Key token 用量

新增独立接口，避免编辑限额时隐式清空用量。

推荐接口：

```text
POST /api/token/:id/reset-token-usage
```

行为：

```text
token_used = 0
```

权限：

- 普通用户只能重置自己的 API Key。
- 管理员如需代操作，应走管理员用户管理或专用 admin endpoint。

### 5.5 API Key usage 接口

当前接口：`GET /api/usage/token/`

响应新增 token 字段：

```json
{
  "code": true,
  "message": "ok",
  "data": {
    "object": "token_usage",
    "name": "default",
    "token_limit_enabled": true,
    "token_limit": 1000000,
    "token_used": 120000,
    "token_remaining": 880000,
    "token_unlimited": false,
    "legacy_total_granted": 0,
    "legacy_total_used": 0,
    "legacy_total_available": 0
  }
}
```

兼容字段：

- 原 `total_granted`、`total_used`、`total_available` 可以保留，但 `web/default` 不再使用。
- 如果保留原字段，必须在代码注释中标明它们是 legacy quota，不是 token cap。

### 5.5.1 重置用量与在途请求

`reset-token-usage` 需要处理在途请求：

- 重置操作必须记录审计日志，至少包含 `token_id`、操作者用户 ID、重置前 `token_used`、重置后 `token_used=0` 和时间。
- 如果重置时存在已预扣但未结算的 `token_limit_pre_consume_records`，后续结算 / 退款不得把 `token_used` 回写为重置前的旧值。
- 推荐做法：预扣记录只按 delta 幂等调整当前值，且退款时最多退回该记录实际预扣仍占用的部分；必要时在记录中保存重置版本或重置时间，避免旧请求覆盖重置结果。
- 重置接口不得修改旧 `remain_quota` / `used_quota`。

### 5.6 OpenAI-compatible credit summary

`controller/token.go` 的 `GetTokenStatus()` 当前返回 `credit_summary`。新设计：

- 兼容对象名可以保留为 `credit_summary`，避免破坏旧客户端。
- 新增明确 token 字段。
- 旧 credit 字段改为 legacy 字段，或维持原字段但不被 default 前端使用。

推荐响应：

```json
{
  "object": "credit_summary",
  "token_limit_enabled": true,
  "token_limit": 1000000,
  "token_used": 120000,
  "token_remaining": 880000,
  "token_unlimited": false,
  "total_granted": 0,
  "total_used": 0,
  "total_available": 0
}
```

---

## 6. 请求运行时设计

### 6.1 责任边界

API Key token cap 是限额，不是资金来源：

```text
API Key token cap：限制单个 key 在当前累计周期内可使用的 token
订阅 token：真正的请求扣费来源
账户余额：只用于购买订阅套餐
```

因此请求必须同时满足：

1. API Key 未过期、未禁用、IP / 模型限制通过。
2. API Key token cap 未超限（如果启用）。
3. 用户有可扣费订阅且订阅 token 足够。

### 6.2 预扣与结算流程

新增 API Key token cap 会话，建议与现有 `BillingSession` 并列，不嵌入旧 wallet 或 token quota 代码。

建议类型：

```go
type TokenLimitSession struct {
    RequestId         string
    TokenId           int
    UserId            int
    Enabled           bool
    PreConsumedTokens int64
}
```

流程：

```text
Relay 解析请求
  -> TokenAuth 写入 token_id / token_limit_enabled / token_limit / token_used
  -> 使用订阅计费路径的同一预扣 token 数作为 key cap 预扣输入
  -> BillingSession.PreConsume(subscriptionTokens)
  -> TokenLimitSession.PreConsume(subscriptionPreConsumedTokens)
  -> 上游请求
  -> 订阅结算生成最终 metered token / subscription_tokens_consumed
  -> TokenLimitSession.Settle(subscriptionMeteredTokens)
  -> BillingSession.Settle(subscriptionMeteredTokens)
```

反向补偿要求：如果订阅预扣、订阅并发租约已经成功，但随后 `TokenLimitSession.PreConsume()` 因 API Key cap 不足而拒绝请求，必须释放订阅并发租约，并幂等退款 `BillingSession` / 订阅预扣记录，使订阅 `token_used` 保持请求前状态。最终错误码必须是 `api_key_token_limit_exhausted`，不得被订阅退款错误污染；如果退款失败，必须记录错误日志和审计记录，但不能把该请求视为成功消费。

如果 API Key 未启用 token cap，`TokenLimitSession` 直接 no-op。

顺序要求：优先完成本地校验、订阅可用性判断、订阅预扣和订阅并发租约，再预扣 API Key token cap。这样可以避免没有有效订阅、订阅 token 不足、订阅并发超限或本地前置校验失败时消耗 key cap。若某条 relay 分支因现有结构必须先预扣 key cap，则该分支必须在所有后续失败出口幂等调用 `TokenLimitSession.Refund()`。

补偿要求：只要 API Key token cap 已预扣，后续任何失败都必须幂等退回，包括订阅预扣失败、订阅并发租约失败、渠道选择失败、本地校验失败、上游重试最终失败、panic/recover 触发的失败，以及客户端连接提前中断导致请求未完成的失败。退款不得影响订阅错误码；例如订阅 token 不足仍返回订阅错误，而不是 key cap 错误。

计量口径要求：API Key token cap 预扣和结算必须绑定订阅扣费使用的同一 metered token 结果。文本请求应复用订阅路径最终写入 `subscription_tokens_consumed` 的 token 数，或复用同一个 `SubscriptionMeteredTokens` / `subscriptionTokensForTextSettle` 结果；不得直接使用 `usage.TotalTokens`、`prompt_tokens + completion_tokens`、旧价格 quota 或模型价格换算结果作为 key cap 结算口径。

如果 API Key token cap 和订阅 token cap 都需要记录预扣值，二者可以各自有记录表，但必须引用同一个 `request_id`，便于审计两边是否使用同一 token 数。

### 6.3 幂等记录

新增表：`token_limit_pre_consume_records`

建议模型：

```go
type TokenLimitPreConsumeRecord struct {
    Id                int    `json:"id"`
    RequestId         string `json:"request_id" gorm:"type:varchar(64);uniqueIndex;not null"`
    UserId            int    `json:"user_id" gorm:"index;not null"`
    TokenId           int    `json:"token_id" gorm:"index;not null"`
    PreConsumedTokens int64  `json:"pre_consumed_tokens" gorm:"type:bigint;not null;default:0"`
    ActualTokens      int64  `json:"actual_tokens" gorm:"type:bigint;not null;default:0"`
    DeltaTokens       int64  `json:"delta_tokens" gorm:"type:bigint;not null;default:0"`
    FailureCode       string `json:"failure_code" gorm:"type:varchar(64);not null;default:''"`
    Status            string `json:"status" gorm:"type:varchar(16);not null;default:'consumed'"`
    CreatedAt         int64  `json:"created_at" gorm:"bigint"`
    UpdatedAt         int64  `json:"updated_at" gorm:"bigint"`
}
```

状态：

- `consumed`：已预扣，等待结算或失败退款。
- `refunded`：请求失败或下游前置条件失败，预扣已退还。
- `settled`：已按最终订阅 metered token 结算。
- `settle_failed`：请求响应已经产生，但结束结算时发现补扣会超过 key cap；该状态必须写入审计记录和日志 `other`，不能静默吞掉。

迁移入口：新增字段和 `token_limit_pre_consume_records` 必须加入 `model/main.go` 的常规迁移路径，覆盖 `migrateDB` 和快速迁移路径中当前项目使用的等价入口。SQLite 至少有单测覆盖 AutoMigrate；MySQL / PostgreSQL 不得依赖 SQLite-only SQL 或数据库专属锁语法。

缓存一致性：凡是修改 `token_limit_enabled`、`token_limit`、`token_used` 或重置 token usage 的路径，都必须更新或失效 token cache。`GetTokenUsage()` 和鉴权路径不得因为读取旧缓存返回陈旧 token cap 字段。

### 6.4 原子更新规则

预扣：

```text
UPDATE tokens
SET token_used = token_used + :estimated
WHERE id = :token_id
  AND token_limit_enabled = true
  AND token_limit > 0
  AND token_used + :estimated <= token_limit
```

结算：

```text
actual_delta = actual_tokens - pre_consumed_tokens
```

- `actual_delta > 0`：同样用条件更新，防止超限。普通非流式响应尚未提交前，补扣超限返回 `api_key_token_limit_exhausted`。
- `actual_delta < 0`：减少 `token_used`，不得低于 `0`。
- `actual_delta = 0`：只更新记录状态。

流式或响应已经发送后的补扣超限不能伪造成普通 429 响应。实现必须采用以下策略之一，并在代码注释和测试中固定：

1. **优先策略：保守预扣。** 对 streaming / long-running 请求，预扣使用当前订阅路径可得到的保守上界，避免结束时出现正向补扣超限。
2. **补偿策略：已发送响应后补扣失败。** 如果上游已向客户端发送内容，订阅仍按实际 metered token 结算；API Key cap 保留预扣值，不再追加超过 cap 的部分；预扣记录标记为 `settle_failed`，日志 `other` 写入 `api_key_token_limit_settle_failed=true`、`api_key_token_limit_actual_tokens`、`api_key_token_limit_pre_consumed` 和失败原因。后续请求会因为剩余额度不足被拒绝。

无论选择哪种策略，都不得在响应已发送后回滚订阅实际扣费，也不得让 `token_used` 超过 `token_limit`。

三库兼容要求：

- 使用 GORM `Updates` + `gorm.Expr`。
- 不使用数据库专属 JSON、锁语法或 `RETURNING` 作为唯一实现。
- 需要防并发超扣时，用条件更新的 `RowsAffected` 判断是否超限。

### 6.5 错误码

API Key token cap 超限时返回 OpenAI 兼容错误：

```json
{
  "error": {
    "message": "api key token limit exhausted",
    "type": "insufficient_quota",
    "code": "api_key_token_limit_exhausted"
  }
}
```

HTTP 状态：`429 Too Many Requests`。

该错误必须区别于订阅 token 用尽：

```text
api_key_token_limit_exhausted       // 单 key 限额用尽
subscription_token_exhausted        // 套餐 token 用尽
subscription_required               // 无有效套餐
subscription_concurrency_exceeded   // 并发超限
```

### 6.6 流式和 Realtime

- 预扣使用订阅路径同一预扣 token 数；对于 streaming / long-running 请求，应优先采用保守预扣策略，避免结束时正向补扣。
- 结束时按订阅路径最终 metered token 结算，文本请求必须与 `subscription_tokens_consumed` 一致。
- 如果上游没有 usage，沿用当前订阅逻辑的估算 / 无 usage fallback。
- cache read/create、Claude cache 5m/1h、Gemini cached content、Responses cached tokens 等归一化场景，API Key token cap 必须与订阅扣减使用同一归一化 token 数。
- API Key token cap 与订阅 token 必须使用同一 token 数，避免两边统计不一致。
- 如果响应已发送后补扣会超限，按 §6.4 的 streaming 补扣超限策略处理并写入审计，不得静默丢失。

Realtime：

- 如果已有订阅增量结算路径，API Key token cap 也需要增量扣减。
- 增量扣减失败必须终止连接并返回 `api_key_token_limit_exhausted`。

### 6.7 异步任务

当前 `service/task_billing.go` 中仍存在 `taskAdjustTokenQuota()`，会调整旧 token key quota。

新设计：

- 所有 API 请求和异步任务的新运行时路径都不得再因结算、退款或重算调用 `taskAdjustTokenQuota()`、`PreConsumeTokenQuota()`、`model.IncreaseTokenQuota()` 或 `model.DecreaseTokenQuota()` 修改旧 token key quota。
- 新 token cap 不得以 `taskAdjustTokenQuota()` 作为实现。
- 如果异步任务不支持订阅 token 扣费，则不接入 API Key token cap，也不得继续扣 / 退旧 `remain_quota`。
- 如果异步任务支持订阅 token 扣费，则必须使用同一 `TokenLimitSession` 或等价的 token cap 会话，且与订阅任务结算使用同一 metered token 数。
- `RefundTaskQuota`、`RecalculateTaskQuota` 等历史任务路径需要在实现阶段逐项审查：凡是仍会修改旧 API Key quota 的路径，都必须停止或隔离为 legacy-only，不得影响新订阅 token-only 请求。

---

## 7. API Key 状态设计

### 7.1 不再把用尽写入 `status = exhausted`

旧状态 `TokenStatusExhausted` 会造成行为不一致：

- `model.ValidateUserToken()` 当前允许 `TokenStatusExhausted` 通过。
- `controller/config_guide.go` 当前拒绝 `TokenStatusExhausted`。

新设计：

- `status` 只表示人工和生命周期状态：enabled / disabled / expired。
- API Key token cap 用尽是派生状态，不写入 `status`。
- 列表展示可以根据 `token_limit_enabled && token_used >= token_limit` 显示 `Token Limit Reached`。

### 7.2 旧 exhausted 兼容

历史 `status = exhausted` 的 API Key：

- 不自动恢复或删除状态。
- 如果后端当前允许其鉴权，应保留现状，避免历史行为突变。
- 配置向导需要统一到主鉴权语义，不能继续因为 legacy exhausted 拒绝一个实际可用的 key。
- 前端可以显示 `Legacy exhausted` 或按后端返回状态展示，但新 token cap 不写该状态。

---

## 8. 前端设计

### 8.1 类型定义

文件：`web/default/src/features/keys/types.ts`

新增字段：

```ts
export const apiKeySchema = z.object({
  // legacy fields remain
  remain_quota: z.number(),
  used_quota: z.number(),
  unlimited_quota: z.boolean(),

  // new token cap fields
  token_limit_enabled: z.boolean().default(false),
  token_limit: z.number().default(0),
  token_used: z.number().default(0),
  token_remaining: z.number().default(0),
  token_unlimited: z.boolean().default(true),
})
```

`web/default` 不得从 `remain_quota` fallback 计算 token limit。

### 8.2 表单模型

文件：`web/default/src/features/keys/lib/api-key-form.ts`

删除旧表单字段：

```ts
remain_quota_dollars
```

新增表单字段：

```ts
const apiKeyFormSchema = z.object({
  token_limit_enabled: z.boolean().default(false),
  token_limit: z.number().optional(),
}).superRefine((value, ctx) => {
  if (value.token_limit_enabled && (!value.token_limit || value.token_limit <= 0)) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['token_limit'],
      message: 'Token limit must be greater than 0',
    })
  }
})
```

转换规则：

```ts
function transformFormDataToPayload(data: ApiKeyFormValues) {
  return {
    ...data,
    token_limit_enabled: data.token_limit_enabled,
    token_limit: data.token_limit_enabled ? data.token_limit : 0,
  }
}
```

`token_limit_enabled=false` 时允许 `token_limit` 为空并提交 `0`；`token_limit_enabled=true` 时必须通过跨字段校验保证 `token_limit > 0`。不要把仅供前端展示或格式化的临时字段透传到后端。

禁止在 API Key 限额表单中使用：

```ts
parseQuotaFromDollars()
quotaUnitsToDollars()
formatQuota()
getCurrencyDisplay()
getCurrencyLabel()
```

### 8.3 创建 / 编辑抽屉

文件：`web/default/src/features/keys/components/api-keys-mutate-drawer.tsx`

文案和控件：

- `Quota Settings` -> `API Key Token Limit`
- `Unlimited Quota` -> `No token limit for this API key`
- `Quota (USD/CNY/...)` -> `Token limit`
- 输入框 placeholder：`Enter token limit`。
- 描述：`Limits only this API key. Requests still consume subscription tokens.`

交互规则：

- 默认关闭 token limit，即 `token_limit_enabled = false`。
- 打开 token limit 后，`token_limit` 必填且必须大于 `0`。
- 编辑历史 API Key 时，即使旧 `remain_quota` 有值，也展示为未设置 token limit。
- 如果 `token_used > token_limit`，保存后显示超限状态，不自动重置 `token_used`。

### 8.4 列表和移动端卡片

文件：

- `web/default/src/features/keys/components/api-keys-columns.tsx`
- `web/default/src/features/keys/components/api-keys-table.tsx`

展示规则：

- 未启用 token limit：显示 `Unlimited` 或 `No key limit`。
- 启用 token limit：显示 `formatTokens(token_used) / formatTokens(token_limit)`。
- 进度条使用 `token_used / token_limit`。
- 剩余量显示 `formatTokens(token_remaining)`。
- 启用限额且 `token_remaining === 0` 时必须显示 `0` 或 `0 tokens`，不能显示空状态或 `-`。
- 不使用旧 `remain_quota + used_quota` 计算总量。

### 8.5 i18n

文件：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`

至少新增或更新以下 key：

```json
{
  "API Key Token Limit": "API Key Token Limit",
  "No token limit for this API key": "No token limit for this API key",
  "Token limit": "Token limit",
  "Enter token limit": "Enter token limit",
  "Limits only this API key. Requests still consume subscription tokens.": "Limits only this API key. Requests still consume subscription tokens.",
  "Token Limit Reached": "Token Limit Reached",
  "Reset token usage": "Reset token usage",
  "This API key uses the new token limit model. Historical quota limits were not migrated.": "This API key uses the new token limit model. Historical quota limits were not migrated."
}
```

如果文案通过配置对象、状态枚举或数组间接传入 `t(config.label)`，必须同步登记到 `web/default/src/i18n/static-keys.ts`，避免 `bun run i18n:sync` 删除或漏扫动态 key。

实现阶段必须从 `web/default/` 运行：

```bash
bun run i18n:sync
```

---

## 9. 系统设置边界

API Key token cap 不再受以下配置影响：

- `QuotaPerUnit`
- `DisplayInCurrencyEnabled`
- `DisplayTokenStatEnabled`
- `general_setting.quota_display_type`
- `USDExchangeRate`
- custom currency 配置

这些配置仍可用于：

- 管理员成本分析。
- 模型价格 / 倍率配置。
- 历史日志 legacy quota 展示。
- OpenAI billing compatibility。

但不得驱动 default 前端 API Key token limit 的输入、校验或展示。

---

## 10. 测试要求

### 10.1 后端模型与迁移测试

覆盖点：

- 迁移后既有 Token 的新字段默认值为：
  - `token_limit_enabled = false`
  - `token_limit = 0`
  - `token_used = 0`
- 即使旧 `remain_quota > 0` 或 `used_quota > 0`，也不会填充新字段。
- SQLite、MySQL、PostgreSQL 迁移语句兼容；单测至少覆盖 SQLite，数据库兼容逻辑不得使用专属 SQL。

### 10.2 后端接口测试

覆盖点：

- 创建 API Key，不启用 token limit，响应返回 `token_limit_enabled = false`。
- 创建 API Key，启用 token limit 且设置 `token_limit = 1000`，响应返回 `token_remaining = 1000`。
- 创建 API Key，启用 token limit 但 `token_limit <= 0`，返回参数错误。
- 更新 API Key 从未启用切到启用，不从旧 `used_quota` 填充 `token_used`。
- 更新 API Key 从启用切到未启用，`token_limit = 0`，`token_used` 保留。
- `GET /api/usage/token/` 返回新 token 字段。
- `GetTokenStatus()` 兼容返回不破坏旧字段，同时包含新 token 字段。

### 10.3 后端运行时测试

覆盖点：

- 未启用 token cap 的 API Key 不触发 cap 检查。
- 启用 token cap 且剩余足够时，预扣成功并增加 `token_used`。
- 启用 token cap 且剩余不足时，返回 `api_key_token_limit_exhausted`。
- 上游请求失败时，预扣 token 退回。
- 实际 usage 小于预扣时，退回差额。
- 实际 usage 大于预扣且仍未超限时，补扣成功。
- 实际 usage 大于预扣且补扣会超限时，返回明确错误并保持数据一致。
- 并发请求下，两个请求不能同时突破同一个 API Key token cap。
- key cap 剩余不足发生在订阅预扣和订阅并发租约成功之后时，响应必须为 `api_key_token_limit_exhausted`，订阅预扣记录必须 refunded 或等价回滚，订阅 `token_used` 不变，API Key `token_used` 不变，订阅并发租约释放。
- key cap 已预扣后，如果订阅预扣失败、订阅并发超限、渠道选择失败、本地前置校验失败、上游重试最终失败或 panic/recover 触发失败，`token_used` 必须退回，预扣记录标记为 `refunded`，原错误码保持订阅或上游错误。
- 有有效订阅且 `token_limit_enabled=false` 时，即使旧 `remain_quota=0`、`unlimited_quota=false`、`used_quota` 很大，请求仍成功，且旧 `remain_quota` / `used_quota` 不变。
- `token_limit_enabled=true` 时，请求通过或拒绝只看 `token_limit` / `token_used`；旧 `remain_quota`、`used_quota`、`unlimited_quota` 不影响结果。
- cache read/create、Claude cache 5m/1h、Claude cache 1h、Gemini cached content、Responses cached tokens、无 usage fallback、streaming 结束结算下，API Key `token_used` 与订阅实际扣减 token 完全一致。
- streaming 已发送响应后如果补扣会超限，必须按 §6.4 固定策略处理：不让 `token_used` 超过 `token_limit`，写入 `settle_failed` 或证明保守预扣不会出现正向补扣超限。
- 异步任务提交、失败退款、重算场景下，旧 `remain_quota` / `used_quota` 不得变化；支持订阅 token 扣费的任务如启用 API Key cap，必须通过新 token cap 会话更新 `token_used`。

### 10.4 前端测试

覆盖点：

- API Key 创建表单默认不启用 token limit。
- 历史 key 即使有 `remain_quota`，编辑表单也不自动显示旧限额。
- 启用 token limit 后，未填写或填写 `0` 会提示错误。
- 提交 payload 包含 `token_limit_enabled` / `token_limit`，不包含 `remain_quota_dollars`。
- 列表使用 `token_used` / `token_limit` 展示，不调用 `formatQuota()`。
- 移动端卡片展示与桌面表格一致。
- i18n key 存在于 en、zh、fr、ja、ru、vi。

### 10.5 验证命令

后端建议运行定向测试：

```bash
go test ./model ./controller ./service ./relay/... -run 'Token|Billing|Subscription|ConfigGuide|Relay'
```

前端建议从 `web/default/` 运行：

```bash
bun run i18n:sync
bun run typecheck
```

如果实现新增了前端单测，还必须运行对应测试命令。


---

## 11. 实施顺序建议

1. 后端新增 Token token cap 字段和响应 DTO。
2. 编写迁移测试，确认历史 API Key 限额全部置空为未启用新 token cap。
3. 更新 `controller/token.go` 创建、更新、列表、详情和 usage 接口。
4. 新增 API Key token cap 预扣 / 结算 / 退款服务。
5. 接入 relay 主请求生命周期，并保持订阅 token 扣费为唯一资金来源。
6. 处理 `TokenStatusExhausted` 与 config guide 行为一致性。
7. 更新 `web/default` API Key 类型、表单、抽屉、列表和移动端卡片。
8. 同步 i18n。
9. 移除 default 前端 API Key 路径中的 `formatQuota()`、`parseQuotaFromDollars()`、`quotaUnitsToDollars()` 依赖。
10. 运行定向后端测试、relay 出口测试、前端 typecheck 和 i18n sync。

---

## 12. 验收标准

功能验收：

- 既有 API Key 在新版本中显示为未设置 token limit，不显示旧余额 / quota 限额。
- 新建 API Key 默认不启用 token limit。
- 新建或编辑 API Key 可以设置明确 token limit。
- API Key 列表显示 token 用量和剩余额度，不显示 USD/CNY/custom quota。
- 启用 token limit 后，请求会按单 key token cap 拒绝超限请求。
- 超限错误区分 API Key token cap 与订阅 token 不足。
- 请求实际扣费仍走订阅 token，不扣账户余额，也不扣旧 `remain_quota`。
- 有效订阅请求不因旧 `remain_quota=0` 或 `unlimited_quota=false` 被拒绝。
- 如果 API Key cap 在订阅预扣后拒绝请求，订阅预扣和并发占用必须回滚，不能消耗订阅 token。

兼容验收：

- 旧字段仍存在，旧客户端不会因为字段删除而崩溃。
- OpenAI-compatible status / usage 接口仍能返回兼容结构。
- API Key 列表 / 详情示例中的 legacy `remain_quota`、`used_quota`、`unlimited_quota` 只是兼容字段示例；实际响应可保留历史值，default 前端必须忽略这些字段作为限额来源。
- classic 主题不被本次变更强制重写。
- SQLite、MySQL、PostgreSQL 迁移兼容。

代码验收：

- default 前端 API Key 路径不再出现 `remain_quota_dollars`。
- default 前端 API Key 路径不再使用 `formatQuota()` 展示 key limit。
- 后端新 token cap 不使用 `common.QuotaPerUnit` 计算 API Key 限额。
- 新 token cap 不调用旧 `PreConsumeTokenQuota()` 或 `taskAdjustTokenQuota()` 作为实现。
- 新订阅 token-only 请求、配置向导、relay 和异步任务路径不得调用 `model.IncreaseTokenQuota()` / `model.DecreaseTokenQuota()` 修改旧 API Key quota。
- 定向测试、relay 出口测试和前端 typecheck 通过。

---

## 13. 风险与处理

### 13.1 历史用户以为旧限额仍生效

风险：迁移后历史 API Key 限额全部置空，原来非无限 key 不再有单 key cap。

处理：

- 管理端和用户端 API Key 编辑页显示说明：历史 quota 限额不会迁移，请按 token 重新设置。
- 如需强提醒，可在列表中对存在旧 `remain_quota > 0 || used_quota > 0` 且 `token_limit_enabled = false` 的 key 显示 `Legacy quota ignored`。

### 13.2 并发请求突破 cap

风险：如果只在日志后聚合，多个并发请求会同时通过检查。

处理：

- 必须使用条件更新 + `RowsAffected` 做原子预扣。
- 必须记录预扣记录，失败或差额结算时可幂等退款。

### 13.2.1 失败请求误扣 key cap

风险：API Key cap 先预扣后，订阅预扣、订阅并发、渠道选择、本地校验或上游请求失败，可能导致没有完成的请求消耗 key cap。

处理：

- 优先把 key cap 预扣放在订阅预扣和并发租约之后。
- 所有失败出口必须幂等调用 `TokenLimitSession.Refund()`。
- 预扣记录必须能证明失败请求最终为 `refunded`。

### 13.3 与订阅结算不一致

风险：API Key token cap 和订阅 token 使用不同 token 数，导致用户看到两个不同用量。

处理：

- 两者必须共用同一个 metered token 计算结果，文本请求以订阅结算最终写入 `subscription_tokens_consumed` 的数值为准。
- 上游无 usage、cache token、streaming 和 Realtime 场景下，API Key token cap 沿用订阅路径的估算 / fallback / 归一化结果。

### 13.3.1 流式结束补扣超限

风险：流式响应已经向客户端发送内容后，最终 usage 大于预扣且补扣会超过 key cap，无法再返回普通 429。

处理：

- 优先采用保守预扣，避免结束时正向补扣超限。
- 如果仍发生，订阅按实际 metered token 结算，API Key `token_used` 不得超过 `token_limit`，预扣记录写 `settle_failed` 并在日志 `other` 中记录审计字段。

### 13.4 旧 `TokenStatusExhausted` 行为冲突

风险：主鉴权允许 exhausted，config guide 拒绝 exhausted。

处理：

- 新 token cap 不再写 `TokenStatusExhausted`。
- config guide 与主鉴权统一到新 token cap 派生状态。

---

## 14. 明确禁止

- 禁止把 `remain_quota` 或 `used_quota` 自动换算成 `token_limit` / `token_used`。
- 禁止用 `QuotaPerUnit` 计算 API Key token limit。
- 禁止 default 前端继续用 `remain_quota_dollars`。
- 禁止 default 前端 API Key 限额展示受货币配置影响。
- 禁止把账户余额重新接入 API 请求扣费。
- 禁止把 API Key token cap 用尽写回旧 `status = exhausted`。
- 禁止用只读日志聚合代替运行时预扣。
- 禁止新增只支持单一数据库的 SQL。
