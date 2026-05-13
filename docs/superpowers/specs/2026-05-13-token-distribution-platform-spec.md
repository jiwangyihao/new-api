# New API token 分销平台定制规格

> 面向 AI 代理的工作者：本规格用于指导后续在 `C:/Users/34404/source/repos/new-api` 上进行二次开发。实现前请先读取仓库根目录 `AGENTS.md`，并遵守 Go + Gin + GORM、React + TypeScript + Bun、SQLite / MySQL / PostgreSQL 全兼容的项目约束。

**目标：** 将 New API 定制为面向 LLM API token 分销的订阅制 OpenAI API 兼容转发平台。

**架构：** 复用现有 `router/ -> controller/ -> service/ -> model/` 分层和订阅计费框架，在订阅模型中加入 token 限额、实时并发、试用与奖励语义；在 relay 主链路中加入用户级并发租约；在注册、支付回调和邀请链路中加入试用发放与真实付费奖励判定；前端同步补齐管理、购买、认证和应用教程入口。

**技术栈：** Go 1.25.1、Gin、GORM v2、go-redis v8、SQLite / MySQL / PostgreSQL、React 19、TypeScript、Rsbuild、Bun、Base UI、Tailwind CSS。

---

## 1. 业务范围

### 1.1 必须满足的能力

- 下游支持 OpenAI `chat/completions` 与 `responses` 两类格式。
- 上游只需支持标准 OpenAI API 兼容接口转发，不做账号反代、网页登录代理或账号池调度。
- 平台按订阅套餐销售 token 使用权，每个套餐包含「周期 token 限额」和「实时并发上限」。
- 支持 24 小时试用版：注册时凭优惠码或邀请码获取，不售卖、不公开展示，单并发，token 不限量。
- 支持 4 个正式月付套餐：40 元、80 元、160 元、660 元。
- 支持 EPay / 易支付等方便付款方式，并保留现有 Stripe、Creem 等支付基础能力。
- 支持永久上下级邀请关系。
- 邀请人直属下级中每累计 2 个真实付费用户，邀请人获得 1 个月 40 元档套餐。
- 下级因试用、管理员赠送、邀请奖励获得的套餐不计入真实付费。
- 可配置为只允许 GitHub 注册和登录。
- 前端保留现代 UI，并向用户提供 AI 应用配置教程或一键导入入口。

### 1.2 非目标

- 不实现账号池、账号反代、网页登录会话代理、浏览器指纹绕过。
- 不重写整个计费系统；基于现有订阅计费与 relay 链路扩展。
- 不把 RPM / TPM 当作套餐并发限制。并发是「正在处理中的 API 请求数」，必须独立实现。
- 不更改 New API / QuantumNous 的许可证、版权头、NOTICE、README 中受保护信息。
- 不引入只适配单一数据库的 SQL；SQLite、MySQL、PostgreSQL 必须同时支持。

## 2. 当前代码基线

仓库已克隆到：`C:/Users/34404/source/repos/new-api`

已确认的关键文件与现有能力：

- `AGENTS.md`
  - 后端：Go、Gin、GORM v2。
  - 前端：React 19、TypeScript、Rsbuild、Base UI、Tailwind CSS。
  - 数据库：SQLite、MySQL、PostgreSQL 必须全部支持。
  - 架构：`router/ -> controller/ -> service/ -> model/`。
- `router/relay-router.go`
  - `POST /v1/chat/completions` 已路由到 `controller.Relay(c, types.RelayFormatOpenAI)`。
  - `POST /v1/responses` 已路由到 `controller.Relay(c, types.RelayFormatOpenAIResponses)`。
  - `POST /v1/responses/compact` 已存在。
- `controller/relay.go`
  - `Relay` 是 API 转发主入口。
  - 现有顺序为：解析请求 → 生成 `RelayInfo` → 敏感词检查 → token 估算 → 价格计算 → `service.PreConsumeBilling` → 选择渠道 → 调用 relay helper → 结算或退款。
  - 并发租约应放在 `PreConsumeBilling` 成功之后、实际调用上游之前。
- `model/subscription.go`
  - `SubscriptionPlan` 已有价格、周期、`TotalAmount`、`QuotaResetPeriod`、支付产品 ID、购买上限、升级分组等字段。
  - `SubscriptionOrder` 已用于订阅支付订单。
  - `UserSubscription` 已有 `AmountTotal`、`AmountUsed`、`Source`、周期重置、过期维护等能力。
  - `SubscriptionPreConsumeRecord` 已提供请求级预扣幂等记录。
  - `CreateUserSubscriptionFromPlanTx` 是从套餐创建用户订阅的集中函数。
  - `CompleteSubscriptionOrder` 是订阅订单支付成功后的集中完成函数。
- `service/billing_session.go`、`service/funding_source.go`、`service/billing.go`
  - 已支持钱包与订阅两类资金来源。
  - 已支持 `subscription_first`、`wallet_first`、`subscription_only`、`wallet_only`。
- `service/subscription_reset_task.go`
  - 已定时处理订阅过期、订阅周期重置、预扣记录清理。
- `controller/subscription.go`
  - 已有用户端订阅、自身订阅偏好、管理端套餐、管理端绑定用户订阅等接口。
- `controller/subscription_payment_epay.go`、`controller/subscription_payment_stripe.go`、`controller/topup_creem.go`、`controller/topup_stripe.go`
  - 已有订阅支付入口和支付回调完成订单逻辑。
- `model/user.go`、`controller/user.go`、`controller/oauth.go`、`controller/github.go`
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

## 3. 套餐规格

### 3.1 正式套餐

| 套餐标识 | 展示名 | 价格 | 周期 | 并发上限 | token 限额 | 可售卖 | 可计入邀请真实付费 |
|---|---|---:|---|---:|---:|---|---|
| `basic_monthly` | Basic | 40 元 | 1 个月 | 1 | 1,000,000,000 | 是 | 是 |
| `plus_monthly` | Plus | 80 元 | 1 个月 | 5 | 2,000,000,000 | 是 | 是 |
| `pro_monthly` | Pro | 160 元 | 1 个月 | 10 | 5,000,000,000 | 是 | 是 |
| `team_monthly` | Team | 660 元 | 1 个月 | 50 | 10,000,000,000 | 是 | 是 |

### 3.2 试用套餐

| 套餐标识 | 展示名 | 价格 | 周期 | 并发上限 | token 限额 | 获取方式 | 可售卖 | 可计入邀请真实付费 |
|---|---|---:|---|---:|---|---|---|---|
| `trial_24h` | Trial | 0 元 | 24 小时 | 1 | 不限量 | 注册时凭有效优惠码或邀请码自动发放 | 否 | 否 |

说明：

- token 限额按订阅周期内累计实际 token 计算。
- 「token 不限量」只表示套餐不设周期 token 上限；仍必须受并发、模型权限、系统限流、风控、成本保护约束。
- 正式套餐可复用现有 `TotalAmount` 与 `AmountUsed` 扣费逻辑，但必须新增 token 快照字段，避免用户展示和运营规则依赖倍率后的 quota 概念。

## 4. 数据模型设计

### 4.1 修改 `model.SubscriptionPlan`

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
- `reward_eligible`：该套餐通过真实订单购买后是否可计入邀请奖励。
- `business_code`：稳定业务标识，避免用自增 ID 绑定运营规则。

### 4.2 修改 `model.UserSubscription`

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
- `token_used`：当前周期已消耗 token。
- `concurrency_limit`：从套餐复制的并发上限快照。
- `grant_reason`：`order`、`admin`、`trial_code`、`invite_trial`、`invite_reward`。
- `grant_source_user_id`：邀请试用或邀请奖励的来源用户 ID。

`Source` 字段继续保留用于兼容旧逻辑；新增业务逻辑优先读取 `grant_reason`。

### 4.3 新增 `TrialCode`

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

### 4.4 新增 `TrialRedemption`

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

额外约束：

- 同一用户只能拥有一次 `grant_reason in ('trial_code', 'invite_trial')` 的试用订阅。
- 对 GitHub-only 部署，GitHub 用户 ID 是主要身份去重依据，OAuth 创建用户时必须避免同一 GitHub 身份重复领取试用。

### 4.5 新增 `InvitationReward`

文件：`model/invitation_reward.go`

```go
type InvitationReward struct {
    Id                   int   `json:"id"`
    InviterId            int   `json:"inviter_id" gorm:"index;not null"`
    QualifiedPaidCount   int   `json:"qualified_paid_count" gorm:"type:int;not null"`
    RewardPlanId         int   `json:"reward_plan_id" gorm:"not null"`
    RewardSubscriptionId int   `json:"reward_subscription_id" gorm:"index"`
    RewardCycle          int   `json:"reward_cycle" gorm:"type:int;not null;default:1"`
    CreatedAt            int64 `json:"created_at" gorm:"type:bigint"`
}
```

唯一索引：`inviter_id + reward_cycle`。

奖励周期规则：

- 默认每新增 2 个合格直属真实付费用户发放 1 次奖励。
- 第 1、2 个合格付费用户触发 `reward_cycle = 1`。
- 第 3、4 个合格付费用户触发 `reward_cycle = 2`。
- 以此类推。

## 5. 数据库迁移

文件：`model/main.go`

必须修改：

- `AutoMigrate` 增加 `TrialCode`、`TrialRedemption`、`InvitationReward`。
- `SubscriptionPlan`、`UserSubscription` 新字段必须参与迁移。
- `ensureSubscriptionPlanTableSQLite()` 增加新列维护。
- 如 `UserSubscription` 对 SQLite 也有手写迁移路径，需要同步加列；若当前依赖 GORM AutoMigrate，则通过测试确认 SQLite 可自动加列。

兼容策略：

- 所有新增列必须有默认值。
- 对旧数据：
  - `public_visible` 默认 `true`。
  - `reward_eligible` 默认 `true`。
  - `grant_reason` 为空时，旧 `source = 'order'` 的订阅可在统计时按 `order` 兼容处理。
  - `token_limit = 0` 时，旧订阅沿用 `amount_total` 兼容，不阻断原用户使用。

## 6. 并发限制设计

### 6.1 计数范围

计入用户级实时并发的接口：

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/responses/compact`
- 同步文本生成类 relay 请求

不纳入本次 token 分销套餐并发的接口：

- Midjourney、Suno、视频等异步任务接口。
- 仅查询、模型列表、余额、日志等控制台接口。

### 6.2 服务接口

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

- `AcquireUserConcurrency` 不分配不必要的大对象，不拼接复杂日志字符串。
- `Release` 必须幂等，重复调用不能导致计数变负。
- `requestId` 用于日志，不作为 Redis 计数 key 的一部分。

### 6.3 Redis 原子算法

使用 `common.RDB.Eval` 执行 Lua。

key：`sub_concurrency:user:{user_id}`

acquire 逻辑：

```lua
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
if current >= limit then
  return current
end
current = redis.call('INCR', KEYS[1])
redis.call('EXPIRE', KEYS[1], ttl)
return current
```

release 逻辑：

```lua
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
if current <= 1 then
  redis.call('DEL', KEYS[1])
  return 0
end
return redis.call('DECR', KEYS[1])
```

默认 TTL：600 秒。新增系统配置 `SubscriptionConcurrencyTTLSeconds`，最小值 60，默认 600。

Redis 不可用策略：

- 新增系统配置 `SubscriptionConcurrencyFailOpen`，默认 `false`。
- 默认 fail-closed，返回 HTTP 429，避免超卖并发。
- 若运营明确开启 fail-open，则记录系统错误日志并放行请求。

### 6.4 relay 接入点

文件：`controller/relay.go`

在 `service.PreConsumeBilling` 成功后、`retryParam` 创建前加入：

```go
lease, leaseErr := service.AcquireSubscriptionConcurrency(c, relayInfo)
if leaseErr != nil {
    newAPIError = leaseErr
    return
}
if lease != nil {
    defer func() {
        if err := lease.Release(c.Request.Context()); err != nil {
            common.SysLog("error releasing subscription concurrency: " + err.Error())
        }
    }()
}
```

`AcquireSubscriptionConcurrency` 从 `relayInfo.SubscriptionId` 查询订阅快照中的 `concurrency_limit`。如果本次请求使用钱包扣费，不套用订阅并发限制。

### 6.5 错误响应

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

## 7. token 限额与订阅扣费

### 7.1 统计口径

- 以 relay 已获得的 usage 为准。
- Chat Completions：`prompt_tokens + completion_tokens`。
- Responses API：`input_tokens + output_tokens` 或适配层映射后的 `TotalTokens`。
- 上游没有返回 usage 时，使用现有估算逻辑兜底，并在消费日志 `other` 中标记 `usage_estimated = true`。

### 7.2 预扣与结算

修改文件：

- `model/subscription.go`
- `service/funding_source.go`
- `service/billing_session.go`
- `service/quota.go`
- `service/text_quota.go`

设计：

- `PreConsumeUserSubscription` 继续负责请求级预扣和选中用户订阅。
- 正式套餐：`token_limit > 0` 时按 token 上限检查剩余额度。
- 试用套餐：`token_limit = 0` 表示 token 不限量，但仍创建预扣记录，保证本次请求绑定到具体订阅并能读取并发上限。
- 当前 `PreConsumeUserSubscription` 要求 `amount > 0`；为支持试用不限量，需要新增内部预扣最小记录值或允许 `amount = 0` 但仍创建记录。本规格推荐允许 `amount = 0` 创建记录，并在 `SubscriptionFunding.PreConsume` 中保持幂等。
- `PostConsumeUserSubscriptionDelta` 同步维护 `amount_used` 与 `token_used`。如果 `token_limit = 0`，只记录 `token_used`，不做余额不足拒绝。
- 日志中同时记录 quota 与 token，用户端展示优先使用 token。

### 7.3 周期重置

修改：`maybeResetUserSubscriptionWithPlanTx`、`ResetDueSubscriptions`

要求：

- 周期重置时同时清零 `amount_used` 与 `token_used`。
- 对 `trial_24h`，`quota_reset_period` 设置为 `never`，到期后直接失效。
- 对正式月付套餐，周期为 1 个月，按现有 `SubscriptionResetPeriod` 机制重置。

## 8. 试用发放设计

### 8.1 注册请求字段

密码注册请求增加可选字段：

```json
{
  "username": "user",
  "password": "pass",
  "email": "user@example.com",
  "aff_code": "INVITER",
  "trial_code": "TRIAL2026"
}
```

GitHub / OAuth 注册：

- 继续使用已有 `aff` query 参数写入 session。
- 新增 `trial_code` query 参数，并在 `GenerateOAuthCode` 或 GitHub OAuth 发起路径写入 session。
- OAuth 回调创建新用户成功后，在同一事务或紧邻事务中发放试用。

### 8.2 发放服务

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
- 没有 `trial_code` 但有有效 `inviter_id` 且系统开启邀请试用时，以 `invite_trial` 发放。
- 同一用户已存在试用订阅时不再发放。
- 试用发放失败不得留下半创建的用户状态；密码注册应与用户创建放入同一事务。若改造成本过高，必须保证失败时返回明确错误并删除新建用户记录。

### 8.3 管理端试用码接口

新增 controller：`controller/trial_code.go`

新增路由：`router/api-router.go`

接口：

- `GET /api/admin/trial-codes`
- `POST /api/admin/trial-codes`
- `PUT /api/admin/trial-codes/:id`
- `PATCH /api/admin/trial-codes/:id/status`
- `DELETE /api/admin/trial-codes/:id`

用户端注册只消费试用码，不提供登录后自行兑换试用码入口，避免注册后反复薅试用。

## 9. 邀请奖励设计

### 9.1 合格真实付费定义

一个直属下级用户满足以下全部条件，计为邀请人的 1 个合格真实付费用户：

- `users.inviter_id = inviter.id`。
- 存在支付成功的 `SubscriptionOrder`。
- 订单对应 `SubscriptionPlan.reward_eligible = true`。
- 发放的 `UserSubscription.grant_reason = 'order'` 或旧数据兼容 `source = 'order'`。
- 订单金额大于 0，订单状态为支付成功。
- 该下级用户每个自然身份只计一次；同一用户多次续费不重复增加合格人数。

不计入：

- 试用套餐。
- 管理员手动绑定。
- 邀请奖励套餐。
- 下级通过再次邀请用户获得的奖励套餐。
- 退款、失败、过期未支付订单。

### 9.2 触发点

修改：`model.CompleteSubscriptionOrder`

在订单完成、用户订阅创建成功、订单状态更新成功后，仍在数据库事务中或事务完成后立即调用：

```go
service.EvaluateInvitationReward(order.UserId)
```

`EvaluateInvitationReward`：

- 读取付费用户的 `InviterId`。
- 统计邀请人的合格直属真实付费用户数。
- 计算应发奖励周期数：`qualifiedPaidCount / 2`。
- 查询已发放的最大 `reward_cycle`。
- 对缺失的奖励周期逐个发放 `basic_monthly` 套餐。
- 创建 `InvitationReward` 记录，并将 `RewardSubscriptionId` 指向奖励订阅。

奖励发放函数必须幂等。并发支付回调同时到达时，唯一索引阻止重复发放。

### 9.3 奖励套餐

奖励套餐使用 40 元档正式套餐快照，但：

- `grant_reason = 'invite_reward'`。
- `grant_source_user_id = inviter_id` 或触发支付用户 ID，推荐使用触发支付用户 ID 并在 `InvitationReward` 里记录邀请人。
- 不计入任何上级的真实付费统计。

## 10. GitHub-only 注册登录

### 10.1 后端配置

新增系统选项：`GitHubOnlyAuthEnabled`，默认 `false`。

修改文件：

- `common/constants.go`
- `model/option.go`
- `controller/misc.go`
- `controller/user.go`
- `controller/github.go`
- `controller/oauth.go`
- `controller/discord.go`
- `controller/oidc.go`
- `controller/linuxdo.go`
- `controller/wechat.go`

规则：

- 当 `GitHubOnlyAuthEnabled = true`：
  - 密码登录接口直接拒绝。
  - 密码注册接口直接拒绝。
  - 非 GitHub OAuth 创建新用户直接拒绝。
  - GitHub OAuth 登录与注册允许。
  - 已有非 GitHub 用户是否允许登录由运营策略决定。本规格要求严格只允许 GitHub，因此密码登录关闭后自然无法登录。
- `GetStatus` 返回 `github_only_auth_enabled`。
- 管理端保存配置时，如果开启 GitHub-only 但 `GitHubOAuthEnabled = false` 或 `GitHubClientId` 为空，必须拒绝保存。

### 10.2 前端行为

修改：

- `web/default/src/features/auth/types.ts`
- `web/default/src/features/auth/sign-in/components/user-auth-form.tsx`
- `web/default/src/features/auth/sign-up/components/sign-up-form.tsx`
- `web/default/src/features/auth/components/oauth-providers.tsx`
- `web/default/src/features/system-settings/auth/basic-auth-section.tsx`
- `web/default/src/features/system-settings/auth/oauth-section.tsx`

规则：

- GitHub-only 开启时：
  - 登录页隐藏用户名密码表单，只显示 GitHub 登录和必要的协议勾选。
  - 注册页隐藏密码注册表单，只显示 GitHub 注册入口。
  - 非 GitHub provider 不展示。
- 管理端 Auth 设置增加「仅允许 GitHub 注册和登录」开关，并在 UI 上提示开启前必须配置 GitHub OAuth。

## 11. 支付与订阅购买

### 11.1 用户购买限制

修改：`controller/subscription.go`

- `GetSubscriptionPlans` 只返回 `enabled = true AND public_visible = true AND is_trial = false` 的套餐。
- 订阅支付请求接口拒绝 `is_trial = true` 的套餐。
- 订阅支付请求接口拒绝 `price_amount <= 0` 的正式购买。

### 11.2 支付回调幂等

现有 `CompleteSubscriptionOrder` 已有订单状态判断和支付渠道 guard。新增邀请奖励后必须保持：

- 同一支付回调重复到达只完成一次订单。
- 奖励评估幂等。
- 支付 provider 不匹配时不创建订阅、不发奖励。

## 12. 前端订阅与运营 UI

### 12.1 订阅类型与表单

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
- 是否可计入邀请真实付费。
- 业务标识 `business_code`。

用户购买页：

- 不展示 `public_visible = false` 或 `is_trial = true` 的套餐。
- 套餐卡展示并发上限与 token 限额。
- token 展示用 `1B`、`2B`、`5B`、`10B` 等格式。

### 12.2 试用码管理 UI

新增：`web/default/src/features/trial-codes/*`

建议文件：

- `api.ts`：封装管理端试用码接口。
- `types.ts`：Zod schema 和 TypeScript 类型。
- `index.tsx`：试用码列表页。
- `components/trial-codes-table.tsx`：表格。
- `components/trial-code-mutate-drawer.tsx`：创建 / 编辑抽屉。
- `components/trial-code-delete-dialog.tsx`：删除确认。

路由：

- `web/default/src/routes/_authenticated/trial-codes/index.tsx`

导航：

- 管理员侧边栏新增「试用码」入口。

### 12.3 邀请奖励展示

现有 `web/default/src/features/wallet/components/affiliate-rewards-card.tsx` 可扩展：

- 展示直属邀请人数。
- 展示合格真实付费人数。
- 展示距离下一次奖励还差几名真实付费用户。
- 展示已获得的邀请奖励套餐次数。

## 13. AI 应用配置教程

现有代码已经支持部分应用配置能力：

- `web/default/src/features/chat/lib/chat-links.ts` 支持 `{key}`、`{cherryConfig}`、`{aionuiConfig}`、`{deepchatConfig}`。
- `web/default/src/features/keys/components/dialogs/cc-switch-dialog.tsx` 支持 Claude、Codex、Gemini 导入到 CC Switch。

扩展方案：

- 新增 `web/default/src/features/app-guides/*`。
- 新增路由 `web/default/src/routes/_authenticated/app-guides/index.tsx`。
- 教程覆盖：Cherry Studio、Chatbox、LobeChat、NextChat、Open WebUI、Continue、Cline、Claude Code / CC Switch。
- 每个教程包含：
  - Base URL：当前站点 `/v1`。
  - API Key：用户选择的 token。
  - 推荐模型：从用户可用模型列表选择。
  - 一键复制配置。
  - 可用时提供一键导入链接。
- 不在教程中硬编码密钥；页面只读取用户当前选择的 token 并在前端生成配置。

## 14. 种子数据与部署配置

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

- 设置 `REDIS_CONN_STRING`，并为并发限制启用独立 Redis 或独立 DB。
- 设置 `SESSION_SECRET`，多机部署必须一致。
- 设置 GitHub OAuth Client ID / Secret。
- 开启 `GitHubOnlyAuthEnabled`、`GitHubOAuthEnabled`，关闭 `PasswordLoginEnabled`、`PasswordRegisterEnabled`。
- 配置 EPay / 易支付回调域名，并验证支付回调签名。

## 15. 测试方案

### 15.1 后端单元测试

新增 / 修改测试文件：

- `model/subscription_distributor_test.go`
  - `CreateUserSubscriptionFromPlanTx` 会快照 `token_limit`、`concurrency_limit`、`grant_reason`。
  - 试用套餐 24 小时 end time 正确。
  - 周期重置会清零 `token_used`。
- `model/trial_code_test.go`
  - 有效试用码可发放试用订阅。
  - 禁用、过期、超次数试用码拒绝。
  - 同一用户重复领取试用拒绝。
- `model/invitation_reward_test.go`
  - 2 个直属真实付费用户触发 1 次 Basic 奖励。
  - 试用、管理员赠送、邀请奖励不计入真实付费。
  - 重复支付回调不重复发奖励。
  - 第 4 个合格真实付费用户触发第 2 次奖励。
- `service/subscription_concurrency_test.go`
  - acquire 达到上限后返回超并发错误。
  - release 幂等。
  - release 不会让计数变负。
  - Redis 不可用且 fail-closed 时拒绝。
- `controller/auth_github_only_test.go`
  - GitHub-only 开启时密码登录拒绝。
  - GitHub-only 开启时密码注册拒绝。
  - GitHub-only 开启时非 GitHub OAuth 注册拒绝。
  - GitHub OAuth 配置缺失时不能开启 GitHub-only。
- `controller/subscription_trial_purchase_test.go`
  - 用户购买接口不返回试用套餐。
  - 支付接口拒绝购买试用套餐。

### 15.2 前端验证

命令：

```bash
cd web/default
bun run typecheck
bun run lint
bun run build
```

重点场景：

- 管理端创建 / 编辑套餐时能保存新增字段。
- 用户购买页只展示正式套餐。
- GitHub-only 开启后登录 / 注册页只显示 GitHub 入口。
- 试用码管理页可创建、禁用、删除试用码。
- 应用教程页可复制 Base URL、API Key 和配置片段。

### 15.3 后端验证命令

推荐先跑精准测试：

```bash
go test ./model -run 'Test.*Subscription|Test.*Trial|Test.*Invitation|TestCompleteSubscriptionOrder' -count=1
go test ./service -run 'Test.*SubscriptionConcurrency|Test.*Billing|Test.*TextQuota' -count=1
go test ./controller -run 'Test.*GitHubOnly|Test.*Subscription|Test.*Trial' -count=1
```

再跑受影响包：

```bash
go test ./model ./service ./controller ./relay/... -count=1
```

### 15.4 手动联调场景

- 新 GitHub 用户携带有效 `trial_code` 注册，获得 24 小时 Trial。
- Trial 用户同时发起 2 个流式请求，第 2 个返回 429。
- Basic 用户发起并完成 Chat Completions 请求，token 用量增加。
- Basic 用户发起 Responses 请求，token 用量增加。
- 直属下级 A、B 分别真实购买 Basic，邀请人获得 1 个月 Basic 奖励。
- 直属下级 C 获得邀请奖励但未付款，不增加邀请人的真实付费人数。
- 支付回调重复投递，不重复创建订阅和奖励。

## 16. 实施顺序

- [ ] 数据模型与迁移：新增套餐、订阅、试用码、奖励字段和表。
- [ ] 套餐种子数据：写入 5 个默认分销套餐。
- [ ] token 快照与订阅扣费：让正式套餐与试用套餐都能绑定到用户订阅。
- [ ] Redis 并发租约：实现 acquire / release 与 relay 接入。
- [ ] 试用码注册链路：密码注册、GitHub OAuth 注册、OAuth session 参数。
- [ ] 邀请奖励：真实付费统计、奖励发放、幂等保护。
- [ ] GitHub-only：后端开关、状态接口、前端登录注册页。
- [ ] 支付购买保护：隐藏试用套餐并拒绝购买试用套餐。
- [ ] 管理端 UI：订阅套餐新增字段、试用码管理。
- [ ] 用户端 UI：套餐展示、邀请奖励进度、应用配置教程。
- [ ] 测试与验证：后端精准测试、前端 typecheck / lint / build、手动联调。

## 17. 验收标准

- `GET /v1/models` 等普通查询接口不受订阅并发误伤。
- `/v1/chat/completions` 和 `/v1/responses` 均可通过订阅计费链路扣减 token。
- 正式套餐按 1B / 2B / 5B / 10B token 限额阻断超额请求。
- 试用套餐 token 不限量，但只能 1 并发，且 24 小时后失效。
- 用户端无法看到或购买试用套餐。
- 管理员可创建、禁用、删除试用码。
- GitHub-only 开启后，密码登录、密码注册、非 GitHub OAuth 注册都不可用。
- 邀请奖励只统计直属真实付费用户；试用、赠送、邀请奖励不计入。
- 支付回调重复、并发到达时，不重复发放订阅或奖励。
- SQLite、MySQL、PostgreSQL 迁移均可通过。
- 前端 `bun run typecheck`、`bun run lint`、`bun run build` 通过。
- 后端受影响包 `go test ./model ./service ./controller ./relay/... -count=1` 通过。

## 18. 风险与注意事项

- New API 使用 AGPL-3.0；如计划闭源商业分发，需要先评估 AGPL 网络服务源码提供义务或联系商业授权。
- 试用 token 不限量有成本风险，生产必须配合模型权限、IP / GitHub 身份风控、Turnstile 或其他反滥用策略。
- Redis 是实时并发限制的关键依赖，多机部署必须使用共享 Redis，不能退化为单进程内存计数。
- 流式响应必须覆盖客户端断开、上游超时、panic、重试失败等出口，避免并发计数泄漏。
- 支付回调和奖励发放必须幂等；任何非幂等的余额增加或套餐发放都不能放在可重复执行路径里。
- 现有 `amount_total` / `quota` 与新增 `token_limit` / `token_used` 同时存在，前端展示和运营规则必须明确优先使用 token 字段，避免口径混乱。
