# New API token 分销平台定制规格

> 面向 AI 代理的工作者：本规格用于指导后续在 `C:/Users/34404/source/repos/new-api` 上进行二次开发。实现前请先读取仓库根目录 `AGENTS.md`，并遵守 Go + Gin + GORM、React + TypeScript + Bun、SQLite / MySQL / PostgreSQL 全兼容的项目约束。

**目标：** 将 New API 定制为面向 LLM API token 分销的订阅制 OpenAI API 兼容转发平台。

**架构：** 复用现有 `router/ -> controller/ -> service/ -> model/` 分层和订阅计费框架。分销模式取消基于价格倍率的 subscription quota 限制，只使用 token 作为唯一套餐口径；在 relay 主链路中加入用户级实时并发租约；在注册和 OAuth 首次建号环节加入手动 trial code 输入与基础人机校验；在邀请链路中按月评估直属有效付费下级并自动授予 Basic 访问权限。

**技术栈：** Go 1.25.1、Gin、GORM v2、go-redis v8（项目已存在依赖）、SQLite / MySQL / PostgreSQL、React 19、TypeScript、Rsbuild、Bun、Base UI、Tailwind CSS。

---

## 1. 本次修订决策

用户反馈后的修订结论如下：

1. **Redis 不是新增 Go 依赖。** 当前仓库 `go.mod` 已包含 `github.com/go-redis/redis/v8`，`common/redis.go` 已支持 `REDIS_CONN_STRING`。并发限制复用项目已有 Redis 能力；生产多实例部署若要精确限制实时并发，需要配置 Redis。未配置 Redis 时，系统仍可启动，但分销套餐并发限制不能在多进程间精确生效。
2. **token 统计必须包含缓存 token。** 订阅 token 口径必须覆盖 cache read、cache creation、Claude cache 5m / 1h、OpenAI Responses `cached_tokens`、Gemini `cachedContentTokenCount` 等字段，并保证不会重复计数。
3. **试用码由用户手动输入。** 邮箱注册表单增加 trial code 输入框；OAuth / GitHub 首次创建平台账号时，在 OAuth 成功后的建号确认页输入 trial code。没有 trial code 但存在邀请人或邀请码时，也可以发放试用。
4. **邀请奖励改为按月有效权益。** 每个月评估一次：如果用户直属下级中有至少 2 个用户拥有成功支付、仍在有效期内的付费套餐，则该用户自动获得当前月 Basic 访问权限。
5. **GitHub-only 只限制新用户创建方式。** 只允许通过 GitHub OAuth 创建新平台账号；GitHub OAuth 成功后的建号确认页允许设置密码。平台账号使用 GitHub 用户名和邮箱创建，并关联 GitHub 身份；之后允许该用户使用 GitHub 用户名或邮箱加密码登录。
6. **许可证不阻塞继续使用 New API。** 由于平台可以开源，AGPL-3.0 不是当前阻塞项；不需要仅因许可证切换 base 项目。仍需在部署和分发时遵守 AGPL 网络服务源码提供义务。
7. **成本风险不是核心约束。** 保留基础风控即可，重点在 trial code / OAuth 建号确认页加入 Turnstile 或同类人机校验。GitHub 身份本身也作为主要反滥用屏障之一。
8. **取消价格配额双口径。** 分销模式下不再使用原有基于价格、倍率和 quota 的 subscription 限制；套餐限制和展示只使用 token。一切钱包 / 老 quota 逻辑只作为 New API 既有功能兼容，不作为本分销套餐口径。

## 2. 业务范围

### 2.1 必须满足的能力

- 下游支持 OpenAI `chat/completions` 与 `responses` 两类格式。
- 上游只需支持标准 OpenAI API 兼容接口转发，不做账号反代、网页登录代理或账号池调度。
- 平台按订阅套餐销售 token 使用权，每个套餐包含「周期 token 限额」和「实时并发上限」。
- 24 小时试用版：用户在注册或 OAuth 首次建号确认页手动输入 trial code 后发放；若没有 trial code 但存在有效邀请人或邀请码，也可发放。
- 4 个正式月付套餐：40 元、80 元、160 元、660 元。
- 支持 EPay / 易支付等方便付款方式，并保留现有 Stripe、Creem 等支付基础能力。
- 支持永久上下级邀请关系。
- 每个月自动评估邀请权益：直属下级中有至少 2 个用户拥有有效付费套餐时，邀请人获得当月 Basic 访问权限。
- 可配置为只允许通过 GitHub OAuth 创建新用户；GitHub 创建的用户可设置密码，并可用 GitHub 用户名或邮箱登录。
- 前端保留现代 UI，并向用户提供 AI 应用配置教程或一键导入入口。

### 2.2 非目标

- 不实现账号池、账号反代、网页登录会话代理、浏览器指纹绕过。
- 不重写整个 relay 系统；基于现有 relay 和订阅链路扩展。
- 不把 RPM / TPM 当作套餐并发限制。并发是「正在处理中的 API 请求数」，必须独立实现。
- 不在分销套餐中继续使用价格倍率 quota 作为限制口径。
- 不更改 New API / QuantumNous 的许可证、版权头、NOTICE、README 中受保护信息。
- 不引入只适配单一数据库的 SQL；SQLite、MySQL、PostgreSQL 必须同时支持。

## 3. 当前代码基线

仓库已克隆到：`C:/Users/34404/source/repos/new-api`

已确认的关键文件与现有能力：

- `AGENTS.md`
  - 后端：Go、Gin、GORM v2。
  - 前端：React 19、TypeScript、Rsbuild、Base UI、Tailwind CSS。
  - 数据库：SQLite、MySQL、PostgreSQL 必须全部支持。
  - 架构：`router/ -> controller/ -> service/ -> model/`。
- `go.mod`、`common/redis.go`
  - 当前项目已经依赖 `github.com/go-redis/redis/v8`。
  - `REDIS_CONN_STRING` 未设置时，`common.RedisEnabled = false`，Redis 是现有可选运行时能力。
- `router/relay-router.go`
  - `POST /v1/chat/completions` 已路由到 `controller.Relay(c, types.RelayFormatOpenAI)`。
  - `POST /v1/responses` 已路由到 `controller.Relay(c, types.RelayFormatOpenAIResponses)`。
  - `POST /v1/responses/compact` 已存在。
- `controller/relay.go`
  - `Relay` 是 API 转发主入口。
  - 现有顺序为：解析请求 → 生成 `RelayInfo` → 敏感词检查 → token 估算 → 价格计算 → `service.PreConsumeBilling` → 选择渠道 → 调用 relay helper → 结算或退款。
  - 并发租约应放在订阅 token 预扣成功之后、实际调用上游之前。
- `model/subscription.go`
  - `SubscriptionPlan` 已有价格、周期、`TotalAmount`、`QuotaResetPeriod`、支付产品 ID、购买上限、升级分组等字段。
  - `SubscriptionOrder` 已用于订阅支付订单。
  - `UserSubscription` 已有 `AmountTotal`、`AmountUsed`、`Source`、周期重置、过期维护等能力。
  - `SubscriptionPreConsumeRecord` 已提供请求级预扣幂等记录。
  - `CreateUserSubscriptionFromPlanTx` 是从套餐创建用户订阅的集中函数。
  - `CompleteSubscriptionOrder` 是订阅订单支付成功后的集中完成函数。
- `dto`、`relay`、`service/text_quota.go`
  - 现有 usage 结构已经包含缓存 token 字段，例如 `CachedTokens`、`CachedCreationTokens`、`ClaudeCacheCreation5mTokens`、`ClaudeCacheCreation1hTokens`、`CacheReadInputTokens`、`CacheCreationInputTokens`、`cachedContentTokenCount`。
- `service/billing_session.go`、`service/funding_source.go`、`service/billing.go`
  - 已支持钱包与订阅两类资金来源。
  - 分销模式需要把订阅资金来源改为 token-only；钱包和旧 quota 逻辑只作为原系统兼容保留。
- `controller/user.go`、`controller/oauth.go`、`controller/github.go`
  - `User` 已有 `InviterId`、`AffCode`、邀请赠送额度相关字段。
  - 密码注册与 GitHub / OAuth 注册路径已经读取邀请参数并写入邀请关系。
- `common/constants.go`、`model/option.go`、`controller/misc.go`
  - 已有 `PasswordLoginEnabled`、`PasswordRegisterEnabled`、`RegisterEnabled`、`GitHubOAuthEnabled` 等配置。
  - `/api/status` 已向前端返回登录注册能力状态。
- `web/default/src/features/subscriptions/*`
  - 已有订阅套餐类型、API、管理端创建编辑抽屉、订阅列表和用户购买弹窗。
- `web/default/src/features/auth/*`
  - 已有登录、注册、OAuth providers、GitHub OAuth 状态展示。
- `web/default/src/features/chat/lib/chat-links.ts`、`web/default/src/features/keys/components/dialogs/cc-switch-dialog.tsx`
  - 已有 AI 应用配置链接、Cherry / AionUI / DeepChat 配置替换、CC Switch 导入能力，可扩展为更完整的教程入口。

## 4. 套餐规格

### 4.1 正式套餐

| 套餐标识 | 展示名 | 价格 | 周期 | 并发上限 | token 限额 | 可售卖 | 可计入有效付费下级 |
|---|---|---:|---|---:|---:|---|---|
| `basic_monthly` | Basic | 40 元 | 1 个月 | 1 | 1,000,000,000 | 是 | 是 |
| `plus_monthly` | Plus | 80 元 | 1 个月 | 5 | 2,000,000,000 | 是 | 是 |
| `pro_monthly` | Pro | 160 元 | 1 个月 | 10 | 5,000,000,000 | 是 | 是 |
| `team_monthly` | Team | 660 元 | 1 个月 | 50 | 10,000,000,000 | 是 | 是 |

### 4.2 试用套餐

| 套餐标识 | 展示名 | 价格 | 周期 | 并发上限 | token 限额 | 获取方式 | 可售卖 | 可计入有效付费下级 |
|---|---|---:|---|---:|---|---|---|---|
| `trial_24h` | Trial | 0 元 | 24 小时 | 1 | 不限量 | 用户手动输入 trial code，或有效邀请关系触发 | 否 | 否 |

说明：

- token 限额按订阅周期内累计实际 token 计算。
- 「token 不限量」只表示套餐不设周期 token 上限；仍必须受并发、模型权限、系统限流和基础人机校验约束。
- 分销模式不使用原有 `TotalAmount` / `AmountUsed` 作为套餐限制口径。旧字段可保留用于迁移兼容，但不能作为用户展示或订阅限制依据。

## 5. 数据模型设计

### 5.1 修改 `model.SubscriptionPlan`

文件：`model/subscription.go`

新增字段：

```go
MonthlyTokenLimit  int64  `json:"monthly_token_limit" gorm:"type:bigint;not null;default:0"`
ConcurrencyLimit   int    `json:"concurrency_limit" gorm:"type:int;not null;default:0"`
IsTrial            bool   `json:"is_trial" gorm:"default:false"`
PublicVisible      bool   `json:"public_visible" gorm:"default:true"`
TrialDurationHours int    `json:"trial_duration_hours" gorm:"type:int;not null;default:0"`
RewardEligible     bool   `json:"reward_eligible" gorm:"default:true"`
BusinessCode       string `json:"business_code" gorm:"type:varchar(64);uniqueIndex"`
```

字段语义：

- `monthly_token_limit`：周期 token 限额；`0` 表示不限制。
- `concurrency_limit`：用户级实时并发上限；正式套餐和试用套餐均必须大于 0。
- `is_trial`：试用套餐标记。试用套餐不允许支付购买。
- `public_visible`：用户端是否展示和售卖。试用套餐为 `false`。
- `trial_duration_hours`：试用时长。试用套餐为 `24`，正式套餐为 `0`。
- `reward_eligible`：该套餐通过真实订单购买后是否可计入邀请月度权益判断。
- `business_code`：稳定业务标识，避免用自增 ID 绑定运营规则。

### 5.2 修改 `model.UserSubscription`

文件：`model/subscription.go`

新增字段：

```go
TokenLimit        int64  `json:"token_limit" gorm:"type:bigint;not null;default:0"`
TokenUsed         int64  `json:"token_used" gorm:"type:bigint;not null;default:0"`
ConcurrencyLimit  int    `json:"concurrency_limit" gorm:"type:int;not null;default:0"`
GrantReason       string `json:"grant_reason" gorm:"type:varchar(32);default:'';index"`
GrantSourceUserId int    `json:"grant_source_user_id" gorm:"type:int;default:0;index"`
```

字段语义：

- `token_limit`：从套餐复制的 token 限额快照。
- `token_used`：当前周期已消耗 token，包含缓存 token 口径。
- `concurrency_limit`：从套餐复制的并发上限快照。
- `grant_reason`：`order`、`admin`、`trial_code`、`invite_trial`、`monthly_invite_entitlement`。
- `grant_source_user_id`：邀请试用或月度邀请权益来源用户 ID。

`Source` 字段继续保留用于兼容旧逻辑；新增业务逻辑优先读取 `grant_reason`。

### 5.3 新增 `TrialCode`

文件：`model/trial_code.go`

```go
type TrialCode struct {
    Id             int    `json:"id"`
    Code           string `json:"code" gorm:"type:varchar(64);uniqueIndex;not null"`
    PlanId         int    `json:"plan_id" gorm:"index;not null"`
    Enabled        bool   `json:"enabled" gorm:"default:true"`
    MaxRedemptions int    `json:"max_redemptions" gorm:"type:int;not null;default:0"`
    RedeemedCount  int    `json:"redeemed_count" gorm:"type:int;not null;default:0"`
    ExpiresAt      int64  `json:"expires_at" gorm:"type:bigint;default:0"`
    CreatedAt      int64  `json:"created_at" gorm:"type:bigint"`
    UpdatedAt      int64  `json:"updated_at" gorm:"type:bigint"`
}
```

规则：

- `code` 统一按 trim 后大写存储与匹配。
- `plan_id` 必须指向 `is_trial = true` 的套餐。
- `max_redemptions = 0` 表示不限制总兑换次数。
- `expires_at = 0` 表示不过期。

### 5.4 新增 `TrialRedemption`

文件：`model/trial_redemption.go`

```go
type TrialRedemption struct {
    Id          int    `json:"id"`
    UserId      int    `json:"user_id" gorm:"uniqueIndex:ux_trial_user_code"`
    TrialCodeId int    `json:"trial_code_id" gorm:"uniqueIndex:ux_trial_user_code"`
    Code        string `json:"code" gorm:"type:varchar(64);index"`
    CreatedAt   int64  `json:"created_at" gorm:"type:bigint"`
}
```

约束：

- 同一用户只能拥有一次 `grant_reason in ('trial_code', 'invite_trial')` 的试用订阅。
- GitHub OAuth 建号流程中，GitHub ID 是主要身份去重依据，避免同一 GitHub 身份重复领取试用。

### 5.5 新增 `InvitationMonthlyEntitlement`

文件：`model/invitation_reward.go`

```go
type InvitationMonthlyEntitlement struct {
    Id                   int    `json:"id"`
    InviterId            int    `json:"inviter_id" gorm:"uniqueIndex:ux_inviter_reward_month;index;not null"`
    RewardMonth          string `json:"reward_month" gorm:"uniqueIndex:ux_inviter_reward_month;type:varchar(7);not null"`
    QualifiedActiveCount int    `json:"qualified_active_count" gorm:"type:int;not null"`
    RewardPlanId         int    `json:"reward_plan_id" gorm:"not null"`
    RewardSubscriptionId int    `json:"reward_subscription_id" gorm:"index"`
    Status               string `json:"status" gorm:"type:varchar(32);index"`
    CreatedAt            int64  `json:"created_at" gorm:"type:bigint"`
    UpdatedAt            int64  `json:"updated_at" gorm:"type:bigint"`
}
```

唯一索引：`inviter_id + reward_month`。

语义：

- `reward_month` 使用 `YYYY-MM`。
- 每月最多授予一个 Basic 权益，不按 2 人一组重复叠加。
- 只要当前月评估时合格有效直属付费下级数大于等于 2，就确保该用户当月拥有 Basic 访问权限。
- 下个月重新评估；若不满足条件，不续授本月之后的权益。

## 6. 数据库迁移

文件：`model/main.go`

必须修改：

- `AutoMigrate` 增加 `TrialCode`、`TrialRedemption`、`InvitationMonthlyEntitlement`。
- `SubscriptionPlan`、`UserSubscription` 新字段必须参与迁移。
- `ensureSubscriptionPlanTableSQLite()` 增加新列维护。
- 如果 `UserSubscription` 对 SQLite 也有手写迁移路径，同步加列；如果依赖 GORM AutoMigrate，通过测试确认 SQLite 可自动加列。

兼容策略：

- 所有新增列必须有默认值。
- 旧订阅数据的 `token_limit = 0` 不应误判为无限正式套餐；只有 `is_trial = true` 或明确迁移标记的订阅可按不限量处理。
- 旧 `TotalAmount` / `AmountUsed` 只用于旧功能兼容，不参与分销套餐限制。

## 7. token-only 计量与扣费

### 7.1 唯一业务口径

分销套餐只使用 token 限额。价格、模型倍率、分组倍率、cache ratio 仍可用于成本分析、日志、渠道选择或钱包老逻辑，但不能决定订阅套餐是否可用。

新增统一函数：

```go
func SubscriptionMeteredTokens(usage *dto.Usage) int64
```

该函数返回本次订阅应扣 token 数，并保证缓存 token 不漏计、不重复计。

### 7.2 缓存 token 规则

- OpenAI Chat Completions：优先使用 `usage.TotalTokens`；`PromptTokensDetails.CachedTokens` 是 input tokens 的子集时只单独记录，不额外相加。
- OpenAI Responses：优先使用 `usage.TotalTokens`；`input_tokens_details.cached_tokens` 单独记录，不对 `total_tokens` 重复相加。
- Anthropic / Claude：`input_tokens`、`output_tokens`、`cache_read_input_tokens`、`cache_creation_input_tokens` 都计入；如果已归一到 `PromptTokensDetails.CachedTokens`、`PromptTokensDetails.CachedCreationTokens`、`ClaudeCacheCreation5mTokens`、`ClaudeCacheCreation1hTokens`，按归一化后的字段精确计入一次。
- Gemini：计入 `promptTokenCount`、`candidatesTokenCount`、`toolUsePromptTokenCount`，并确保 `cachedContentTokenCount` 被记录；如果 provider 的总 token 已包含 cached content，则不重复相加。
- 音频、图片、reasoning token 按现有 `dto.Usage` 明细纳入统一 token 口径。
- 上游没有 usage 时，使用现有估算逻辑兜底，并在消费日志 `other` 中标记 `usage_estimated = true`。

### 7.3 预扣与结算

修改文件：

- `model/subscription.go`
- `service/funding_source.go`
- `service/billing_session.go`
- `service/quota.go`
- `service/text_quota.go`

设计：

- 订阅预扣参数从「quota」改为「estimated tokens」。
- 正式套餐：`token_limit > 0` 时按 `token_limit - token_used` 检查剩余 token。
- 试用套餐：`token_limit = 0 && grant_reason in ('trial_code', 'invite_trial')` 表示不限量；仍创建预扣记录，以便绑定具体订阅并读取并发上限。
- 月度邀请权益套餐：按 Basic 套餐 token 限额和并发执行，`grant_reason = 'monthly_invite_entitlement'`。
- 结算时用 `SubscriptionMeteredTokens` 的实际 token 值调整预扣差额。
- 日志中记录 token、cached token、cache creation token、usage 来源。用户端展示只展示 token。

### 7.4 周期重置

修改：`maybeResetUserSubscriptionWithPlanTx`、`ResetDueSubscriptions`

要求：

- 周期重置时清零 `token_used`。
- 对 `trial_24h`，`quota_reset_period` 设置为 `never`，到期后直接失效。
- 对正式月付套餐，周期为 1 个月，按现有 `SubscriptionResetPeriod` 机制重置 token。
- 旧 `amount_used` 可同步清零用于兼容，但不能作为分销判断依据。

## 8. 并发限制设计

### 8.1 是否引入额外依赖

不新增 Go module 依赖。New API 当前已经依赖 go-redis，并已有 `common.RDB`、`common.RedisEnabled`、`REDIS_CONN_STRING` 初始化逻辑。本功能复用现有 Redis 能力。

部署层面：

- 单进程开发环境可以关闭并发限制或使用本地测试 limiter。
- 生产单机可以使用 Redis 或进程内 limiter，但进程内 limiter 不支持多实例精确限制。
- 生产多实例必须配置共享 Redis，否则无法保证用户级实时并发准确性。

### 8.2 计数范围

计入用户级实时并发的接口：

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/responses/compact`
- 同步文本生成类 relay 请求

不纳入本次 token 分销套餐并发的接口：

- Midjourney、Suno、视频等异步任务接口。
- 仅查询、模型列表、余额、日志等控制台接口。

### 8.3 服务接口

创建：`service/subscription_concurrency.go`

```go
type ConcurrencyLease struct {
    key       string
    userId    int
    requestId string
    limit     int
    released  atomic.Bool
}

func AcquireUserConcurrency(ctx context.Context, userId int, requestId string, limit int) (*ConcurrencyLease, error)
func (l *ConcurrencyLease) Release(ctx context.Context) error
```

要求：

- `Release` 必须幂等，重复调用不能导致计数变负。
- `requestId` 用于日志，不作为 Redis 计数 key 的一部分。
- Redis TTL 默认 600 秒，最小值 60 秒。

### 8.4 Redis 原子算法

key：`sub_concurrency:user:{user_id}`

acquire 使用 Lua：当前值达到 limit 则拒绝，否则 `INCR` 并刷新 TTL。release 使用 Lua：当前值小于等于 1 时删除 key，否则 `DECR`。

Redis 不可用策略：

- 新增系统配置 `SubscriptionConcurrencyFailOpen`，默认 `false`。
- 默认 fail-closed，返回 HTTP 429，避免超卖并发。
- 若运营明确开启 fail-open，则记录系统错误日志并放行请求。

### 8.5 relay 接入点

文件：`controller/relay.go`

在订阅 token 预扣成功后、实际请求上游前获取租约。钱包旧逻辑或非订阅资金来源不套用订阅并发限制。

超并发返回 HTTP 429，OpenAI 兼容错误体：

```json
{
  "error": {
    "message": "subscription concurrency limit exceeded: current limit is 5",
    "type": "rate_limit_error",
    "code": "subscription_concurrency_exceeded"
  }
}
```

## 9. 试用发放与建号流程

### 9.1 邮箱注册

邮箱注册表单增加可选字段：

```json
{
  "username": "user",
  "password": "pass",
  "email": "user@example.com",
  "aff_code": "INVITER",
  "trial_code": "TRIAL2026",
  "turnstile_token": "..."
}
```

规则：

- 用户手动输入 trial code 后才按 trial code 发放试用。
- 如果没有 trial code，但存在有效邀请人或邀请码，可发放 `invite_trial`。
- 输入了 trial code 时必须进行 Turnstile 或现有等价人机校验。
- 同一用户只能领取一次试用。

### 9.2 GitHub OAuth 首次建号

GitHub-only 模式下，新用户创建流程为：

1. 用户点击 GitHub 登录。
2. GitHub OAuth 成功后，后端保存短期 pending OAuth session，不立即创建平台用户。
3. 前端跳转到「完成账号创建」页面。
4. 页面展示 GitHub 用户名和邮箱，并提供：
   - trial code 输入框（可选）；
   - 密码和确认密码输入框（可选但推荐）；
   - 协议勾选；
   - Turnstile 或同类人机校验。
5. 用户提交后，后端在同一事务中创建平台账号、绑定 GitHub 身份、设置密码、处理邀请关系、发放试用。

如果 OAuth 匹配到已有平台账号，则直接登录，不进入建号确认页。

### 9.3 非 GitHub OAuth

GitHub-only 开启时，非 GitHub OAuth 不允许创建新用户。是否允许已有用户绑定或登录非 GitHub OAuth 由现有配置控制，但不能绕过「新用户必须由 GitHub 创建」规则。

### 9.4 发放服务

创建：`service/trial_grant.go`

```go
type TrialGrantInput struct {
    UserId       int
    TrialCode    string
    InviterId    int
    Source       string
    SourceUserId int
}

func GrantTrialOnRegistration(tx *gorm.DB, input TrialGrantInput) (*model.UserSubscription, error)
```

规则：

- 有有效 `trial_code` 时，以 `trial_code` 发放。
- 没有 `trial_code` 但有有效 `inviter_id` 时，以 `invite_trial` 发放。
- 同一用户已存在试用订阅时不再发放。
- 试用发放失败不得留下半创建的用户状态。

## 10. 月度邀请权益

### 10.1 合格有效直属付费下级定义

一个直属下级用户满足以下全部条件时，计入邀请人的当月权益判断：

- `users.inviter_id = inviter.id`。
- 存在支付成功的 `SubscriptionOrder`。
- 订单对应 `SubscriptionPlan.reward_eligible = true`。
- 订单金额大于 0，订单状态为支付成功。
- 该下级当前存在有效期内的付费订阅：`grant_reason = 'order'` 或旧数据兼容 `source = 'order'`，且 `status = active`、`end_time > now`。
- 同一直属下级用户只计 1 个名额。

不计入：

- 试用套餐。
- 管理员手动绑定。
- 月度邀请权益套餐。
- 下级通过邀请获得的试用或权益。
- 已过期、退款、失败、未支付订单。

### 10.2 权益发放

创建：`service/invitation_reward.go`

```go
func EnsureMonthlyInvitationEntitlement(inviterId int, at time.Time) error
func RunMonthlyInvitationEntitlementSweep(at time.Time, limit int) error
```

规则：

- 当前月 `qualifiedActiveCount >= 2` 时，确保邀请人拥有当月 Basic 访问权限。
- 当月最多创建 1 条 `InvitationMonthlyEntitlement`。
- 当月权益订阅使用 `basic_monthly` 套餐快照，`grant_reason = 'monthly_invite_entitlement'`。
- 权益订阅有效期到当前月结束，或按自然月加 1 个月结束；实现必须统一选择一种并在测试中固定。本规格采用「当前自然月结束」。
- 支付完成、订阅过期任务、周期任务都可以触发评估；另设定时 sweep 保底。
- 下个月重新评估；如果不满足条件，不创建新权益。

## 11. GitHub-only 新用户创建模式

### 11.1 后端配置

新增系统选项：`GitHubOnlySignupEnabled`，默认 `false`。

该选项含义：只限制「创建新用户」的方式，不等于禁用密码登录。

开启后：

- 密码注册接口拒绝创建新用户。
- 非 GitHub OAuth 不允许创建新用户。
- GitHub OAuth 可创建新用户。
- GitHub OAuth 首次建号确认页允许设置密码。
- 已设置密码的 GitHub 创建账号，可以用 GitHub 用户名或邮箱加密码登录。
- `GetStatus` 返回 `github_only_signup_enabled`。
- 管理端保存配置时，如果开启 GitHub-only signup 但 `GitHubOAuthEnabled = false` 或 `GitHubClientId` 为空，必须拒绝保存。

### 11.2 平台账号字段

GitHub 首次建号时：

- 平台用户名优先使用 GitHub login；如冲突，追加短后缀。
- 邮箱使用 GitHub verified email；如 GitHub 未返回可用邮箱，建号确认页要求用户输入邮箱并进行现有邮箱校验。
- 密码由用户在建号确认页手动设置；若用户跳过密码，仍可通过 GitHub OAuth 登录，后续可在账户安全设置中设置密码。

### 11.3 前端行为

- 登录页保留密码登录入口，因为 GitHub 创建账号可设置密码。
- 注册页在 GitHub-only signup 开启时隐藏邮箱注册表单，只展示 GitHub 创建账号入口。
- OAuth 建号确认页展示 trial code、密码、人机校验和协议勾选。
- 非 GitHub provider 不展示为「注册」入口。

## 12. 支付与订阅购买

### 12.1 用户购买限制

修改：`controller/subscription.go`

- `GetSubscriptionPlans` 只返回 `enabled = true AND public_visible = true AND is_trial = false` 的套餐。
- 订阅支付请求接口拒绝 `is_trial = true` 的套餐。
- 订阅支付请求接口拒绝 `price_amount <= 0` 的正式购买。

### 12.2 支付回调幂等

现有 `CompleteSubscriptionOrder` 已有订单状态判断和支付渠道 guard。新增 token-only 与月度邀请权益后必须保持：

- 同一支付回调重复到达只完成一次订单。
- 月度邀请权益评估幂等。
- 支付 provider 不匹配时不创建订阅、不发权益。

## 13. 前端订阅与运营 UI

### 13.1 订阅类型与表单

修改：

- `web/default/src/features/subscriptions/types.ts`
- `web/default/src/features/subscriptions/lib/plan-form.ts`
- `web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx`
- `web/default/src/features/subscriptions/components/subscriptions-columns.tsx`
- `web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx`
- `web/default/src/features/wallet/components/subscription-plans-card.tsx`

新增字段 UI：

- 周期 token 限额。
- 实时并发上限。
- 是否试用套餐。
- 是否用户端展示。
- 试用时长小时数。
- 是否可计入有效付费下级。
- 业务标识 `business_code`。

用户购买页：

- 不展示 `public_visible = false` 或 `is_trial = true` 的套餐。
- 套餐卡展示并发上限与 token 限额。
- token 展示用 `1B`、`2B`、`5B`、`10B` 等格式。
- 不展示原 quota 或价格倍率配额。

### 13.2 试用码管理 UI

新增：`web/default/src/features/trial-codes/*`

建议文件：

- `api.ts`：封装管理端试用码接口。
- `types.ts`：Zod schema 和 TypeScript 类型。
- `index.tsx`：试用码列表页。
- `components/trial-codes-table.tsx`：表格。
- `components/trial-code-mutate-drawer.tsx`：创建 / 编辑抽屉。
- `components/trial-code-delete-dialog.tsx`：删除确认。

### 13.3 OAuth 建号确认页

新增：`web/default/src/features/auth/oauth-onboarding/*`

页面职责：

- 展示 GitHub 返回的用户名和邮箱。
- 允许用户输入 trial code。
- 允许用户设置密码。
- 执行 Turnstile 或同类人机校验。
- 提交后完成平台账号创建。

### 13.4 邀请权益展示

扩展 `web/default/src/features/wallet/components/affiliate-rewards-card.tsx`：

- 展示直属邀请人数。
- 展示当前有效付费直属下级人数。
- 展示本月是否已获得 Basic 权益。
- 展示权益有效期。

## 14. AI 应用配置教程

现有代码已经支持部分应用配置能力：

- `web/default/src/features/chat/lib/chat-links.ts` 支持 `{key}`、`{cherryConfig}`、`{aionuiConfig}`、`{deepchatConfig}`。
- `web/default/src/features/keys/components/dialogs/cc-switch-dialog.tsx` 支持 Claude、Codex、Gemini 导入到 CC Switch。

扩展方案：

- 新增 `web/default/src/features/app-guides/*`。
- 新增路由 `web/default/src/routes/_authenticated/app-guides/index.tsx`。
- 教程覆盖：Cherry Studio、Chatbox、LobeChat、NextChat、Open WebUI、Continue、Cline、Claude Code / CC Switch。
- 每个教程包含：Base URL、API Key、推荐模型、一键复制配置、可用时的一键导入链接。
- 不在教程中硬编码密钥；页面只读取用户当前选择的 token 并在前端生成配置。

## 15. 种子数据与部署配置

新增：`model/subscription_seed.go`

提供函数：

```go
func EnsureDistributorDefaultPlans() error
```

行为：

- 按 `business_code` upsert 5 个套餐：`trial_24h`、`basic_monthly`、`plus_monthly`、`pro_monthly`、`team_monthly`。
- 仅在不存在同名 `business_code` 时创建，不覆盖管理员已修改的价格、标题和支付产品 ID。
- 可由管理员通过环境变量或系统设置启用一次性初始化：`DISTRIBUTOR_DEFAULT_PLANS_ENABLED=true`。

建议生产配置：

- 多实例部署设置 `REDIS_CONN_STRING`，复用 New API 现有 Redis 支持。
- 设置 `SESSION_SECRET`，多机部署必须一致。
- 设置 GitHub OAuth Client ID / Secret。
- 开启 `GitHubOnlySignupEnabled`、`GitHubOAuthEnabled`，关闭 `PasswordRegisterEnabled`。`PasswordLoginEnabled` 可以保留开启，以支持 GitHub 建号后设置密码的用户登录。
- 配置 EPay / 易支付回调域名，并验证支付回调签名。

## 16. 许可证策略

New API 使用 AGPL-3.0。当前策略是继续使用 New API 作为 base 项目，并接受开源分发和网络服务源码提供义务。

要求：

- 二开代码保留 AGPL-3.0 合规声明。
- 如果对外提供网络服务，准备源码获取入口或对应仓库链接。
- 不因为许可证单独切换 base 项目。
- 如果未来业务明确要求闭源，再重新评估商业授权或替代项目。

## 17. 测试方案

### 17.1 后端单元测试

新增 / 修改测试文件：

- `model/subscription_distributor_test.go`
  - `CreateUserSubscriptionFromPlanTx` 会快照 `token_limit`、`concurrency_limit`、`grant_reason`。
  - 周期重置会清零 `token_used`。
  - 旧 `TotalAmount` 不参与分销订阅限制。
- `service/subscription_meter_test.go`
  - OpenAI cached tokens 不重复计入。
  - Anthropic cache read / cache creation 会计入。
  - Claude cache 5m / 1h 会计入且不重复。
  - Gemini cached content token 会被记录并按 provider 语义计入一次。
- `model/trial_code_test.go`
  - 有效试用码可发放试用订阅。
  - 禁用、过期、超次数试用码拒绝。
  - 同一用户重复领取试用拒绝。
- `service/subscription_concurrency_test.go`
  - acquire 达到上限后返回超并发错误。
  - release 幂等。
  - Redis 不可用且 fail-closed 时拒绝。
- `service/invitation_reward_test.go`
  - 当月存在 2 个有效直属付费下级时发放 Basic 权益。
  - 只有 1 个有效直属付费下级时不发放。
  - 试用、管理员赠送、月度邀请权益不计入。
  - 同月重复评估不重复发放。
  - 下月重新评估。
- `controller/auth_github_only_test.go`
  - GitHub-only signup 开启时密码注册拒绝。
  - GitHub-only signup 开启时密码登录仍允许已设置密码的 GitHub 创建账号。
  - 非 GitHub OAuth 不能创建新用户。
  - GitHub OAuth 首次建号确认页可设置密码和 trial code。
- `controller/subscription_trial_purchase_test.go`
  - 用户购买接口不返回试用套餐。
  - 支付接口拒绝购买试用套餐。

### 17.2 前端验证

命令：

```bash
cd web/default
bun run typecheck
bun run lint
bun run build
```

重点场景：

- 管理端创建 / 编辑套餐时能保存新增字段。
- 用户购买页只展示正式套餐，只展示 token 与并发口径。
- 邮箱注册页显示 trial code 输入框。
- GitHub-only signup 开启后注册页只显示 GitHub 建号入口，但登录页保留密码登录。
- GitHub OAuth 首次建号确认页可输入 trial code、设置密码并进行人机校验。
- 试用码管理页可创建、禁用、删除试用码。
- 应用教程页可复制 Base URL、API Key 和配置片段。

### 17.3 后端验证命令

推荐先跑精准测试：

```bash
go test ./model -run 'Test.*Subscription|Test.*Trial|Test.*Invitation|TestCompleteSubscriptionOrder' -count=1
go test ./service -run 'Test.*SubscriptionConcurrency|Test.*Meter|Test.*Billing|Test.*TextQuota' -count=1
go test ./controller -run 'Test.*GitHubOnly|Test.*Subscription|Test.*Trial|Test.*OAuthOnboarding' -count=1
```

再跑受影响包：

```bash
go test ./model ./service ./controller ./relay/... -count=1
```

### 17.4 手动联调场景

- 邮箱注册用户手动输入 trial code，获得 24 小时 Trial。
- GitHub OAuth 新用户完成建号确认页，输入 trial code、设置密码，获得 Trial，并可用 GitHub 用户名或邮箱加密码登录。
- Trial 用户同时发起 2 个流式请求，第 2 个返回 429。
- Basic 用户发起 Chat Completions 请求，含 cache read / cache creation 的 usage 被正确计入 token。
- Basic 用户发起 Responses 请求，`input_tokens_details.cached_tokens` 被记录，token 不重复扣减。
- 邀请人直属下级中有 2 个用户拥有有效付费套餐时，邀请人获得当月 Basic 权益。
- 下个月不满足条件时，不再创建新的月度邀请权益。
- 支付回调重复投递，不重复创建订阅和权益。

## 18. 实施顺序

- [ ] 数据模型与迁移：新增套餐、订阅、试用码、月度邀请权益字段和表。
- [ ] token-only 计量：实现包含缓存 token 的 `SubscriptionMeteredTokens`。
- [ ] token-only 订阅扣费：订阅预扣、结算和重置只使用 token。
- [ ] Redis 并发租约：复用项目现有 Redis 能力实现 acquire / release 与 relay 接入。
- [ ] 试用码注册链路：邮箱注册 trial code 输入、人机校验、邀请试用。
- [ ] GitHub OAuth 建号确认页：trial code、密码设置、人机校验、平台账号创建。
- [ ] 月度邀请权益：有效付费下级统计、月度 Basic 权益发放、定时 sweep。
- [ ] GitHub-only signup：只限制新用户创建方式，保留 GitHub 创建账号的密码登录。
- [ ] 支付购买保护：隐藏试用套餐并拒绝购买试用套餐。
- [ ] 管理端 UI：订阅套餐新增字段、试用码管理。
- [ ] 用户端 UI：套餐展示、邀请权益进度、应用配置教程。
- [ ] 测试与验证：后端精准测试、前端 typecheck / lint / build、手动联调。

## 19. 验收标准

- `GET /v1/models` 等普通查询接口不受订阅并发误伤。
- `/v1/chat/completions` 和 `/v1/responses` 均可通过订阅 token-only 链路扣减 token。
- 缓存 token 被覆盖：cache read、cache creation、Claude cache 5m / 1h、Responses cached tokens、Gemini cached content 均有测试。
- 正式套餐按 1B / 2B / 5B / 10B token 限额阻断超额请求。
- 订阅套餐不再使用价格倍率 quota 作为限制口径。
- 试用套餐 token 不限量，但只能 1 并发，且 24 小时后失效。
- 邮箱注册和 GitHub OAuth 建号确认页都支持手动输入 trial code。
- trial code 输入路径有人机校验。
- GitHub-only signup 开启后，只有 GitHub OAuth 能创建新用户；GitHub 创建账号可设置密码，并可用 GitHub 用户名或邮箱登录。
- 用户端无法看到或购买试用套餐。
- 管理员可创建、禁用、删除试用码。
- 月度邀请权益只统计直属有效付费下级；试用、赠送、月度权益不计入。
- 支付回调重复、并发到达时，不重复发放订阅或权益。
- SQLite、MySQL、PostgreSQL 迁移均可通过。
- 前端 `bun run typecheck`、`bun run lint`、`bun run build` 通过。
- 后端受影响包 `go test ./model ./service ./controller ./relay/... -count=1` 通过。

## 20. 风险与注意事项

- Redis 不是新增代码依赖，但生产多实例要精确并发限制时是运行时要求。
- AGPL-3.0 在当前开源策略下不是阻塞项；需要遵守网络服务源码提供义务。
- 试用 token 不限量不是核心风险，但 trial code / OAuth 建号确认页必须有人机校验，避免低成本批量注册。
- 流式响应必须覆盖客户端断开、上游超时、panic、重试失败等出口，避免并发计数泄漏。
- 支付回调和月度邀请权益发放必须幂等。
- 分销模式统一使用 token 口径；旧 quota 字段和价格倍率不得重新进入套餐限制和用户展示。
