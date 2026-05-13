# New API token 分销平台实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 基于 New API 增加 token 分销所需的订阅套餐、试用、邀请奖励、实时并发、GitHub-only 认证和应用配置教程能力。

**架构：** 复用现有 `router/ -> controller/ -> service/ -> model/` 分层，在订阅模型保存套餐快照，在 relay 预扣成功后获取 Redis 并发租约，在支付完成后评估邀请奖励，在注册成功时发放试用订阅。前端沿用现有 `features/*` 结构，补齐管理端字段、试用码页面、登录注册展示和应用教程页。

**技术栈：** Go 1.25.1、Gin、GORM v2、go-redis v8、SQLite / MySQL / PostgreSQL、React 19、TypeScript、Bun、Rsbuild、Base UI、Tailwind CSS。

---

## 参考文件

- 规格：`docs/superpowers/specs/2026-05-13-token-distribution-platform-spec.md`
- 仓库规则：`AGENTS.md`
- Relay 入口：`controller/relay.go`、`router/relay-router.go`
- 订阅模型：`model/subscription.go`、`model/main.go`
- 计费服务：`service/billing_session.go`、`service/funding_source.go`、`service/quota.go`
- 注册认证：`controller/user.go`、`controller/oauth.go`、`controller/github.go`、`controller/misc.go`、`model/option.go`
- 前端订阅：`web/default/src/features/subscriptions/*`
- 前端认证：`web/default/src/features/auth/*`、`web/default/src/features/system-settings/auth/*`

## 文件结构

### 后端

- 修改：`model/subscription.go` —— 套餐快照字段、订阅扣费、创建订阅、支付完成入口。
- 修改：`model/main.go` —— AutoMigrate 与 SQLite 加列。
- 创建：`model/trial_code.go` —— 试用码和兑换记录。
- 创建：`model/invitation_reward.go` —— 邀请奖励记录与合格付费统计。
- 创建：`model/subscription_seed.go` —— 5 个默认分销套餐。
- 创建：`service/subscription_concurrency.go` —— Redis 用户级并发租约。
- 创建：`service/trial_grant.go` —— 注册时发放试用。
- 创建：`service/invitation_reward.go` —— 支付后发放邀请奖励。
- 修改：`controller/relay.go` —— 订阅预扣后接入并发租约。
- 创建：`controller/trial_code.go` —— 试用码管理接口。
- 修改：`controller/subscription.go` —— 隐藏试用套餐、拒绝购买试用套餐、管理新增字段。
- 修改：`controller/user.go`、`controller/oauth.go`、`controller/github.go` —— 注册试用发放。
- 修改：`controller/discord.go`、`controller/oidc.go`、`controller/linuxdo.go`、`controller/wechat.go` —— GitHub-only 下拒绝非 GitHub 注册。
- 修改：`common/constants.go`、`model/option.go`、`controller/misc.go`、`controller/option.go` —— 系统配置与状态输出。
- 修改：`router/api-router.go` —— 试用码管理路由。

### 前端

- 修改：`web/default/src/features/subscriptions/types.ts` —— 新套餐字段类型。
- 修改：`web/default/src/features/subscriptions/lib/plan-form.ts` —— 表单 schema、默认值、payload。
- 修改：`web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx` —— 管理端套餐字段。
- 修改：`web/default/src/features/subscriptions/components/subscriptions-columns.tsx` —— 管理端列表列。
- 修改：`web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx` —— 用户购买弹窗规格展示。
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx` —— 套餐卡展示 token 与并发。
- 修改：`web/default/src/features/wallet/components/affiliate-rewards-card.tsx` —— 邀请奖励进度。
- 创建：`web/default/src/features/trial-codes/*` —— 试用码管理页。
- 创建：`web/default/src/routes/_authenticated/trial-codes/index.tsx` —— 试用码路由。
- 修改：`web/default/src/features/auth/*` 与 `web/default/src/features/system-settings/auth/*` —— GitHub-only 展示与设置。
- 创建：`web/default/src/features/app-guides/*` —— AI 应用教程。
- 创建：`web/default/src/routes/_authenticated/app-guides/index.tsx` —— 应用教程路由。

## 任务 1：订阅模型字段与迁移

**文件：**
- 修改：`model/subscription.go`
- 修改：`model/main.go`
- 测试：`model/subscription_distributor_test.go`

- [ ] **步骤 1：编写失败测试**

在 `model/subscription_distributor_test.go` 创建测试，断言从套餐创建订阅时快照 token 和并发字段：

```go
func TestCreateUserSubscriptionFromPlanTx_DistributorSnapshot(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7101, Username: "snapshot_user", Status: common.UserStatusEnabled}).Error)
    plan := &SubscriptionPlan{Id: 7201, Title: "Basic", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 1_000_000_000, MonthlyTokenLimit: 1_000_000_000, ConcurrencyLimit: 1, PublicVisible: true, RewardEligible: true, BusinessCode: "basic_monthly"}
    require.NoError(t, DB.Create(plan).Error)
    var sub *UserSubscription
    require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
        created, err := CreateUserSubscriptionFromPlanTx(tx, 7101, plan, "order")
        sub = created
        return err
    }))
    assert.Equal(t, int64(1_000_000_000), sub.TokenLimit)
    assert.Equal(t, 1, sub.ConcurrencyLimit)
    assert.Equal(t, "order", sub.GrantReason)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run TestCreateUserSubscriptionFromPlanTx_DistributorSnapshot -count=1`

预期：FAIL，报错包含 `MonthlyTokenLimit undefined` 或 `TokenLimit undefined`。

- [ ] **步骤 3：实现模型字段**

在 `SubscriptionPlan` 增加：

```go
MonthlyTokenLimit  int64  `json:"monthly_token_limit" gorm:"type:bigint;not null;default:0"`
ConcurrencyLimit   int    `json:"concurrency_limit" gorm:"type:int;not null;default:0"`
IsTrial            bool   `json:"is_trial" gorm:"default:false"`
PublicVisible      bool   `json:"public_visible" gorm:"default:true"`
TrialDurationHours int    `json:"trial_duration_hours" gorm:"type:int;not null;default:0"`
RewardEligible     bool   `json:"reward_eligible" gorm:"default:true"`
BusinessCode       string `json:"business_code" gorm:"type:varchar(64);uniqueIndex"`
```

在 `UserSubscription` 增加：

```go
TokenLimit        int64  `json:"token_limit" gorm:"type:bigint;not null;default:0"`
TokenUsed         int64  `json:"token_used" gorm:"type:bigint;not null;default:0"`
ConcurrencyLimit  int    `json:"concurrency_limit" gorm:"type:int;not null;default:0"`
GrantReason       string `json:"grant_reason" gorm:"type:varchar(32);default:'';index"`
GrantSourceUserId int    `json:"grant_source_user_id" gorm:"type:int;default:0;index"`
```

在 `CreateUserSubscriptionFromPlanTx` 创建订阅时写入这些快照字段。

- [ ] **步骤 4：实现迁移**

在 `model/main.go` 的 AutoMigrate 中加入新增表，并在 SQLite 手写表结构中加入 `subscription_plans` 新列。所有新增列都带默认值，避免升级旧部署失败。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w model/subscription.go model/main.go model/subscription_distributor_test.go
go test ./model -run TestCreateUserSubscriptionFromPlanTx_DistributorSnapshot -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add model/subscription.go model/main.go model/subscription_distributor_test.go
git commit -m "feat(subscription): 增加分销套餐快照字段"
```

## 任务 2：试用码模型与注册发放服务

**文件：**
- 创建：`model/trial_code.go`
- 创建：`service/trial_grant.go`
- 测试：`model/trial_code_test.go`

- [ ] **步骤 1：编写失败测试**

在 `model/trial_code_test.go` 写入：

```go
func TestConsumeTrialCode_ValidCodeCreatesRedemption(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7301, Username: "trial_user", Status: common.UserStatusEnabled}).Error)
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7302, Title: "Trial", Enabled: true, IsTrial: true, TrialDurationHours: 24, ConcurrencyLimit: 1, BusinessCode: "trial_24h"}).Error)
    require.NoError(t, DB.Create(&TrialCode{Code: "TRIAL2026", PlanId: 7302, Enabled: true, MaxRedemptions: 1}).Error)
    code, err := ConsumeTrialCode(DB, 7301, " trial2026 ")
    require.NoError(t, err)
    assert.Equal(t, "TRIAL2026", code.Code)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run TestConsumeTrialCode_ValidCodeCreatesRedemption -count=1`

预期：FAIL，报错包含 `undefined: TrialCode`。

- [ ] **步骤 3：实现 `TrialCode` 和 `TrialRedemption`**

创建 `model/trial_code.go`，包含 `TrialCode`、`TrialRedemption`、`ConsumeTrialCode(tx *gorm.DB, userId int, rawCode string)`。`ConsumeTrialCode` 必须 trim + 大写 code、校验启用状态、过期时间、最大兑换次数、用户是否已经领取试用，并在同一事务内写入兑换记录和递增 `redeemed_count`。

- [ ] **步骤 4：实现注册发放服务**

创建 `service/trial_grant.go`：

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

逻辑：有 `trial_code` 时消费试用码；没有 `trial_code` 但存在邀请人时发放 `trial_24h`；同一用户已领取试用则拒绝。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w model/trial_code.go service/trial_grant.go model/trial_code_test.go
go test ./model -run TestConsumeTrialCode_ValidCodeCreatesRedemption -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add model/trial_code.go service/trial_grant.go model/trial_code_test.go
git commit -m "feat(trial): 增加注册试用码模型"
```

## 任务 3：token 限额与不限量试用扣费

**文件：**
- 修改：`model/subscription.go`
- 修改：`service/funding_source.go`
- 修改：`service/billing_session.go`
- 测试：`model/subscription_distributor_test.go`

- [ ] **步骤 1：编写失败测试**

追加测试：

```go
func TestPreConsumeUserSubscription_AllowsZeroAmountForUnlimitedTrial(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7404, Username: "unlimited_trial", Status: common.UserStatusEnabled}).Error)
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7405, Title: "Trial", Enabled: true, IsTrial: true, MonthlyTokenLimit: 0, ConcurrencyLimit: 1}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7406, UserId: 7404, PlanId: 7405, Status: "active", AmountTotal: 0, TokenLimit: 0, ConcurrencyLimit: 1, EndTime: common.GetTimestamp() + 3600}).Error)
    res, err := PreConsumeUserSubscription("trial-zero", 7404, "gpt-4o", 0, 0)
    require.NoError(t, err)
    assert.Equal(t, 7406, res.UserSubscriptionId)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run TestPreConsumeUserSubscription_AllowsZeroAmountForUnlimitedTrial -count=1`

预期：FAIL，报错包含 `amount must be > 0`。

- [ ] **步骤 3：调整订阅预扣逻辑**

修改 `PreConsumeUserSubscription`：允许 `amount = 0`，仍创建 `SubscriptionPreConsumeRecord` 并绑定订阅。正式套餐使用 `token_limit - token_used` 判断余额；旧数据 `token_limit = 0 && amount_total > 0` 时回退到 `amount_total - amount_used`。

- [ ] **步骤 4：同步 token 使用量**

在预扣、补扣、退款、周期重置路径同步维护 `token_used`。周期重置时同时清零 `amount_used` 和 `token_used`。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w model/subscription.go service/funding_source.go service/billing_session.go model/subscription_distributor_test.go
go test ./model -run TestPreConsumeUserSubscription_AllowsZeroAmountForUnlimitedTrial -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add model/subscription.go service/funding_source.go service/billing_session.go model/subscription_distributor_test.go
git commit -m "feat(subscription): 支持 token 限额与不限量试用"
```

## 任务 4：Redis 用户级并发租约

**文件：**
- 创建：`service/subscription_concurrency.go`
- 测试：`service/subscription_concurrency_test.go`
- 修改：`common/constants.go`
- 修改：`model/option.go`

- [ ] **步骤 1：编写失败测试**

创建测试，覆盖 acquire 到达上限、release 幂等、Redis 不可用且 fail-closed 时拒绝。

```go
func TestMemoryConcurrencyLimiter_AcquireRelease(t *testing.T) {
    limiter := newMemorySubscriptionConcurrencyLimiter()
    ctx := context.Background()
    lease, err := limiter.Acquire(ctx, 7501, "req-1", 1)
    require.NoError(t, err)
    _, err = limiter.Acquire(ctx, 7501, "req-2", 1)
    require.ErrorIs(t, err, ErrSubscriptionConcurrencyExceeded)
    require.NoError(t, lease.Release(ctx))
    require.NoError(t, lease.Release(ctx))
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./service -run TestMemoryConcurrencyLimiter_AcquireRelease -count=1`

预期：FAIL，报错包含 `undefined: newMemorySubscriptionConcurrencyLimiter`。

- [ ] **步骤 3：实现配置**

新增：

```go
var SubscriptionConcurrencyTTLSeconds = 600
var SubscriptionConcurrencyFailOpen = false
```

在 `model/option.go` 初始化、更新、解析这两个选项。

- [ ] **步骤 4：实现 Redis Lua 租约**

创建 `AcquireUserConcurrency(ctx context.Context, userId int, requestId string, limit int)` 和 `ConcurrencyLease.Release(ctx)`。使用 Redis `Eval` 原子执行 acquire / release。release 必须用 `atomic.Bool` 防止重复释放。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w service/subscription_concurrency.go service/subscription_concurrency_test.go common/constants.go model/option.go
go test ./service -run TestMemoryConcurrencyLimiter_AcquireRelease -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add service/subscription_concurrency.go service/subscription_concurrency_test.go common/constants.go model/option.go
git commit -m "feat(subscription): 增加用户级并发租约"
```

## 任务 5：relay 接入并发限制

**文件：**
- 修改：`service/subscription_concurrency.go`
- 修改：`controller/relay.go`
- 修改：`model/subscription.go`
- 测试：`controller/subscription_trial_purchase_test.go`

- [ ] **步骤 1：编写失败测试**

测试 `SubscriptionConcurrencyAPIError(5)` 返回 HTTP 429 且错误码为 `subscription_concurrency_exceeded`。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./controller -run TestSubscriptionConcurrencyErrorToOpenAI429 -count=1`

预期：FAIL，报错包含 `undefined: service.SubscriptionConcurrencyAPIError`。

- [ ] **步骤 3：实现错误转换与订阅查询**

实现 `SubscriptionConcurrencyAPIError(limit int) *types.NewAPIError`，并在 `model/subscription.go` 增加 `GetUserSubscriptionConcurrencyLimit(id int) (int, error)`。

- [ ] **步骤 4：在 relay 中接入**

在 `controller/relay.go` 的 `service.PreConsumeBilling` 成功后调用 `service.AcquireSubscriptionConcurrency(c.Request.Context(), relayInfo)`，成功后 `defer lease.Release(context.Background())`。钱包扣费请求不受订阅并发限制。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w service/subscription_concurrency.go controller/relay.go model/subscription.go controller/subscription_trial_purchase_test.go
go test ./controller -run TestSubscriptionConcurrencyErrorToOpenAI429 -count=1
go test ./service -run TestMemoryConcurrencyLimiter_AcquireRelease -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add service/subscription_concurrency.go controller/relay.go model/subscription.go controller/subscription_trial_purchase_test.go
git commit -m "feat(relay): 接入订阅并发限制"
```

## 任务 6：邀请奖励

**文件：**
- 创建：`model/invitation_reward.go`
- 创建：`service/invitation_reward.go`
- 修改：`controller/subscription_payment_epay.go`
- 修改：`controller/subscription_payment_stripe.go`
- 修改：`controller/topup_creem.go`
- 修改：`controller/topup_stripe.go`
- 测试：`model/invitation_reward_test.go`

- [ ] **步骤 1：编写失败测试**

测试两个直属用户各完成真实支付订单后，邀请人获得 1 个 `grant_reason = invite_reward` 的 Basic 订阅。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run TestInvitationReward_TwoQualifiedPaidUsersGrantBasic -count=1`

预期：FAIL，报错包含 `undefined: InvitationReward`。

- [ ] **步骤 3：实现模型和统计函数**

创建 `InvitationReward`，唯一索引为 `inviter_id + reward_cycle`。统计函数只计算直属用户、成功订单、订单金额大于 0、套餐 `reward_eligible = true`、发放来源为真实订单的用户，并按用户去重。

- [ ] **步骤 4：实现奖励评估服务**

`EvaluateInvitationReward(paidUserId int)` 计算 `qualifiedPaidCount / 2`，为缺失周期发放 `business_code = basic_monthly` 的订阅，`grant_reason = invite_reward`。唯一索引保证重复回调不重复发放。

- [ ] **步骤 5：接入支付回调**

所有订阅支付成功调用 `CompleteSubscriptionOrder` 后，读取订单用户 ID 并调用 `service.EvaluateInvitationReward(order.UserId)`。支付 provider 不匹配或订单已失败时不调用。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w model/invitation_reward.go service/invitation_reward.go controller/subscription_payment_epay.go controller/subscription_payment_stripe.go controller/topup_creem.go controller/topup_stripe.go model/invitation_reward_test.go
go test ./model -run TestInvitationReward_TwoQualifiedPaidUsersGrantBasic -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add model/invitation_reward.go service/invitation_reward.go controller/subscription_payment_epay.go controller/subscription_payment_stripe.go controller/topup_creem.go controller/topup_stripe.go model/invitation_reward_test.go
git commit -m "feat(invitation): 增加真实付费邀请奖励"
```

## 任务 7：订阅购买保护与试用码接口

**文件：**
- 修改：`controller/subscription.go`
- 创建：`controller/trial_code.go`
- 修改：`router/api-router.go`
- 测试：`controller/subscription_trial_purchase_test.go`

- [ ] **步骤 1：编写失败测试**

测试 `GetSubscriptionPlans` 不返回 `is_trial = true` 或 `public_visible = false` 的套餐。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./controller -run TestGetSubscriptionPlans_HidesTrialPlans -count=1`

预期：FAIL，响应仍包含试用套餐。

- [ ] **步骤 3：隐藏并拒绝购买试用套餐**

用户套餐列表查询条件改为 `enabled = true AND public_visible = true AND is_trial = false`。EPay、Stripe、Creem 订阅支付入口在创建订单前拒绝 `is_trial = true`、`public_visible = false` 或 `price_amount <= 0` 的套餐。

- [ ] **步骤 4：实现试用码管理接口**

`controller/trial_code.go` 提供：

```go
func AdminListTrialCodes(c *gin.Context)
func AdminCreateTrialCode(c *gin.Context)
func AdminUpdateTrialCode(c *gin.Context)
func AdminUpdateTrialCodeStatus(c *gin.Context)
func AdminDeleteTrialCode(c *gin.Context)
```

创建和更新必须校验 `plan_id` 指向试用套餐。

- [ ] **步骤 5：增加路由**

在 `router/api-router.go` 增加 `/api/trial-codes/admin` 管理路由，使用 `middleware.AdminAuth()`。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w controller/subscription.go controller/trial_code.go router/api-router.go controller/subscription_trial_purchase_test.go
go test ./controller -run TestGetSubscriptionPlans_HidesTrialPlans -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add controller/subscription.go controller/trial_code.go router/api-router.go controller/subscription_trial_purchase_test.go
git commit -m "feat(subscription): 隐藏试用套餐并增加试用码接口"
```

## 任务 8：注册链路发放试用

**文件：**
- 修改：`controller/user.go`
- 修改：`controller/oauth.go`
- 修改：`controller/github.go`
- 测试：`controller/auth_github_only_test.go`

- [ ] **步骤 1：编写失败测试**

测试密码注册请求携带 `trial_code` 后创建 `grant_reason = trial_code` 的订阅。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./controller -run TestPasswordRegister_GrantsTrialCode -count=1`

预期：FAIL，未创建试用订阅。

- [ ] **步骤 3：扩展注册请求**

密码注册请求结构增加 `TrialCode string json:"trial_code"`。用户创建和 `service.GrantTrialOnRegistration` 放入同一事务。

- [ ] **步骤 4：扩展 OAuth 参数**

GitHub / OAuth 发起时读取 `trial_code` query 并写入 session。OAuth 回调创建新用户后读取 session 中的 `trial_code`，调用试用发放服务。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w controller/user.go controller/oauth.go controller/github.go controller/auth_github_only_test.go
go test ./controller -run TestPasswordRegister_GrantsTrialCode -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add controller/user.go controller/oauth.go controller/github.go controller/auth_github_only_test.go
git commit -m "feat(trial): 注册时发放试用套餐"
```

## 任务 9：GitHub-only 认证开关

**文件：**
- 修改：`common/constants.go`
- 修改：`model/option.go`
- 修改：`controller/misc.go`
- 修改：`controller/option.go`
- 修改：`controller/user.go`
- 修改：`controller/oauth.go`
- 修改：`controller/discord.go`
- 修改：`controller/oidc.go`
- 修改：`controller/linuxdo.go`
- 修改：`controller/wechat.go`
- 测试：`controller/auth_github_only_test.go`

- [ ] **步骤 1：编写失败测试**

测试 `GitHubOnlyAuthEnabled = true` 时密码登录被拒绝，`GetStatus` 返回 `github_only_auth_enabled`。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./controller -run 'TestGitHubOnlyRejectsPasswordLogin|TestStatusIncludesGitHubOnlyAuth' -count=1`

预期：FAIL，报错包含 `undefined: common.GitHubOnlyAuthEnabled`。

- [ ] **步骤 3：增加配置和状态**

新增 `common.GitHubOnlyAuthEnabled`，加入 `model.OptionMap` 初始化和更新解析，在 `controller/misc.go` 状态中输出 `github_only_auth_enabled`。在 `controller/option.go` 校验开启前必须已启用 GitHub OAuth 且配置 Client ID。

- [ ] **步骤 4：限制认证入口**

`Login`、`Register` 开头拒绝 GitHub-only。非 GitHub OAuth provider 创建新用户时拒绝。GitHub provider 继续允许登录和注册。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w common/constants.go model/option.go controller/misc.go controller/option.go controller/user.go controller/oauth.go controller/discord.go controller/oidc.go controller/linuxdo.go controller/wechat.go controller/auth_github_only_test.go
go test ./controller -run 'TestGitHubOnlyRejectsPasswordLogin|TestStatusIncludesGitHubOnlyAuth' -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add common/constants.go model/option.go controller/misc.go controller/option.go controller/user.go controller/oauth.go controller/discord.go controller/oidc.go controller/linuxdo.go controller/wechat.go controller/auth_github_only_test.go
git commit -m "feat(auth): 支持仅 GitHub 登录注册"
```

## 任务 10：默认分销套餐

**文件：**
- 创建：`model/subscription_seed.go`
- 修改：`main.go`
- 测试：`model/subscription_distributor_test.go`

- [ ] **步骤 1：编写失败测试**

测试 `EnsureDistributorDefaultPlans()` 创建 `trial_24h`、`basic_monthly`、`plus_monthly`、`pro_monthly`、`team_monthly`，并断言价格、token、并发。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run TestEnsureDistributorDefaultPlans -count=1`

预期：FAIL，报错包含 `undefined: EnsureDistributorDefaultPlans`。

- [ ] **步骤 3：实现种子函数**

创建 `model/subscription_seed.go`，按 `business_code` 查询，不存在才创建，避免覆盖管理员修改。5 个套餐值按规格写入。

- [ ] **步骤 4：接入启动配置**

在 `main.go` 数据库初始化后读取 `DISTRIBUTOR_DEFAULT_PLANS_ENABLED`，为 `true` 时调用种子函数并记录错误日志。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w model/subscription_seed.go main.go model/subscription_distributor_test.go
go test ./model -run TestEnsureDistributorDefaultPlans -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add model/subscription_seed.go main.go model/subscription_distributor_test.go
git commit -m "feat(subscription): 增加默认分销套餐"
```

## 任务 11：前端订阅套餐字段

**文件：**
- 修改：`web/default/src/features/subscriptions/types.ts`
- 修改：`web/default/src/features/subscriptions/lib/plan-form.ts`
- 修改：`web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx`
- 修改：`web/default/src/features/subscriptions/components/subscriptions-columns.tsx`
- 修改：`web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx`
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`

- [ ] **步骤 1：更新类型与表单**

在 `subscriptionPlanSchema`、`userSubscriptionSchema`、`getPlanFormSchema`、`PLAN_FORM_DEFAULTS`、`planToFormValues`、`formValuesToPlanPayload` 中加入 token、并发、试用、展示、奖励、业务标识字段。

- [ ] **步骤 2：更新管理端 UI**

在套餐创建 / 编辑抽屉增加输入：`monthly_token_limit`、`concurrency_limit`、`is_trial`、`public_visible`、`trial_duration_hours`、`reward_eligible`、`business_code`。在套餐表格增加对应列。

- [ ] **步骤 3：更新用户端展示**

在购买弹窗和钱包套餐卡展示：并发上限、token 限额；`0` token 显示为 `Unlimited tokens`，十亿级显示为 `1B tokens`。

- [ ] **步骤 4：运行前端检查**

运行：

```bash
cd web/default
bun run typecheck
bun run lint
```

预期：PASS。

- [ ] **步骤 5：Commit**

运行：

```bash
git add web/default/src/features/subscriptions/types.ts web/default/src/features/subscriptions/lib/plan-form.ts web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx web/default/src/features/subscriptions/components/subscriptions-columns.tsx web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx web/default/src/features/wallet/components/subscription-plans-card.tsx
git commit -m "feat(web): 展示订阅 token 与并发字段"
```

## 任务 12：前端试用码管理

**文件：**
- 创建：`web/default/src/features/trial-codes/api.ts`
- 创建：`web/default/src/features/trial-codes/types.ts`
- 创建：`web/default/src/features/trial-codes/index.tsx`
- 创建：`web/default/src/features/trial-codes/components/trial-codes-table.tsx`
- 创建：`web/default/src/features/trial-codes/components/trial-code-mutate-drawer.tsx`
- 创建：`web/default/src/features/trial-codes/components/trial-code-delete-dialog.tsx`
- 创建：`web/default/src/routes/_authenticated/trial-codes/index.tsx`

- [ ] **步骤 1：创建类型和 API**

`types.ts` 定义 `trialCodeSchema` 和 `TrialCodePayload`。`api.ts` 封装 `GET /api/trial-codes/admin/`、`POST /api/trial-codes/admin/`、`PUT /api/trial-codes/admin/:id`、`PATCH /api/trial-codes/admin/:id/status`、`DELETE /api/trial-codes/admin/:id`。

- [ ] **步骤 2：创建列表和抽屉**

按现有 `features/subscriptions` 表格、抽屉、删除对话框模式实现。列表展示 code、plan_id、enabled、max_redemptions、redeemed_count、expires_at。

- [ ] **步骤 3：创建路由**

`web/default/src/routes/_authenticated/trial-codes/index.tsx` 渲染 `TrialCodesPage`。如侧边栏导航由动态配置驱动，则通过后台配置添加入口；如存在静态 registry，则增加管理员可见入口。

- [ ] **步骤 4：运行前端检查**

运行：

```bash
cd web/default
bun run typecheck
bun run lint
```

预期：PASS。

- [ ] **步骤 5：Commit**

运行：

```bash
git add web/default/src/features/trial-codes web/default/src/routes/_authenticated/trial-codes/index.tsx
git commit -m "feat(web): 增加试用码管理页面"
```

## 任务 13：前端 GitHub-only 体验

**文件：**
- 修改：`web/default/src/features/auth/types.ts`
- 修改：`web/default/src/features/auth/sign-in/components/user-auth-form.tsx`
- 修改：`web/default/src/features/auth/sign-up/components/sign-up-form.tsx`
- 修改：`web/default/src/features/auth/components/oauth-providers.tsx`
- 修改：`web/default/src/features/system-settings/auth/basic-auth-section.tsx`
- 修改：`web/default/src/features/system-settings/auth/oauth-section.tsx`

- [ ] **步骤 1：更新状态类型**

`SystemStatus` 增加 `github_only_auth_enabled?: boolean`，同时兼容直接字段和 `data` 内字段。

- [ ] **步骤 2：隐藏密码表单和非 GitHub provider**

登录页和注册页读取 `github_only_auth_enabled`。为 true 时隐藏密码表单，只显示 GitHub OAuth 入口和协议勾选。`OAuthProviders` 在 GitHub-only 时过滤掉非 GitHub provider。

- [ ] **步骤 3：增加系统设置开关**

认证设置页增加「仅允许 GitHub 注册和登录」开关。开启时提示必须先配置 GitHub OAuth。

- [ ] **步骤 4：运行前端检查**

运行：

```bash
cd web/default
bun run typecheck
bun run lint
```

预期：PASS。

- [ ] **步骤 5：Commit**

运行：

```bash
git add web/default/src/features/auth/types.ts web/default/src/features/auth/sign-in/components/user-auth-form.tsx web/default/src/features/auth/sign-up/components/sign-up-form.tsx web/default/src/features/auth/components/oauth-providers.tsx web/default/src/features/system-settings/auth/basic-auth-section.tsx web/default/src/features/system-settings/auth/oauth-section.tsx
git commit -m "feat(web): 适配仅 GitHub 登录注册"
```

## 任务 14：应用配置教程和邀请奖励展示

**文件：**
- 创建：`web/default/src/features/app-guides/types.ts`
- 创建：`web/default/src/features/app-guides/index.tsx`
- 创建：`web/default/src/features/app-guides/components/app-guide-card.tsx`
- 创建：`web/default/src/features/app-guides/lib/build-config.ts`
- 创建：`web/default/src/routes/_authenticated/app-guides/index.tsx`
- 修改：`web/default/src/features/wallet/components/affiliate-rewards-card.tsx`

- [ ] **步骤 1：创建配置生成工具**

`build-config.ts` 提供：

```ts
export function buildOpenAIBaseUrl(origin: string) {
  return `${origin.replace(/\/$/, '')}/v1`
}

export function buildCherryStudioConfig(baseUrl: string, apiKey: string) {
  return { id: 'new-api', baseUrl, apiKey }
}
```

- [ ] **步骤 2：创建教程页面**

教程覆盖 Cherry Studio、Chatbox、LobeChat、NextChat、Open WebUI、Continue、Cline、Claude Code / CC Switch。每张卡包含 Base URL、API Key 选择、复制按钮和一键导入链接。

- [ ] **步骤 3：更新邀请奖励卡**

展示直属邀请人数、合格真实付费人数、距下一次奖励还差几人、已获得奖励次数。后端接口若尚未返回这些字段，在任务 7 的管理接口附近补一个用户端 affiliate summary 接口。

- [ ] **步骤 4：运行前端检查**

运行：

```bash
cd web/default
bun run typecheck
bun run lint
bun run build
```

预期：PASS。

- [ ] **步骤 5：Commit**

运行：

```bash
git add web/default/src/features/app-guides web/default/src/routes/_authenticated/app-guides/index.tsx web/default/src/features/wallet/components/affiliate-rewards-card.tsx
git commit -m "feat(web): 增加 AI 应用配置教程"
```

## 任务 15：最终验证

**文件：**
- 检查全部已修改文件

- [ ] **步骤 1：后端精准测试**

运行：

```bash
go test ./model -run 'Test.*Subscription|Test.*Trial|Test.*Invitation|TestCompleteSubscriptionOrder' -count=1
go test ./service -run 'Test.*SubscriptionConcurrency|Test.*Billing|Test.*TextQuota' -count=1
go test ./controller -run 'Test.*GitHubOnly|Test.*Subscription|Test.*Trial' -count=1
```

预期：全部 PASS。

- [ ] **步骤 2：后端受影响包测试**

运行：

```bash
go test ./model ./service ./controller ./relay/... -count=1
```

预期：全部 PASS。

- [ ] **步骤 3：前端验证**

运行：

```bash
cd web/default
bun run typecheck
bun run lint
bun run build
```

预期：全部 PASS。

- [ ] **步骤 4：手动联调**

验证场景：GitHub 用户携带试用码注册获得 Trial；Trial 同时发起 2 个流式请求时第 2 个返回 429；Basic 用户调用 `/v1/chat/completions` 和 `/v1/responses` 后 token 使用量增加；两个直属用户真实购买后邀请人获得 Basic 奖励；重复支付回调不重复发放。

- [ ] **步骤 5：最终 Commit**

运行：

```bash
git status --short
git add .
git commit -m "feat(distributor): 完成 token 分销平台定制"
```

预期：工作区只包含本次实现相关文件并成功提交。
