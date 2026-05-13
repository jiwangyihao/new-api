# New API token 分销平台实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 基于 New API 增加 token-only 分销套餐、手动 trial code、GitHub OAuth 建号确认、月度邀请权益、实时并发和应用配置教程能力。

**架构：** 复用现有 `router/ -> controller/ -> service/ -> model/` 分层。分销套餐只使用 token 作为限制和展示口径；实时并发复用项目已有 go-redis 支持；试用码在邮箱注册表单或 GitHub OAuth 首次建号确认页由用户手动输入；邀请权益按月评估直属有效付费下级；GitHub-only 只限制新用户创建方式，不禁用 GitHub 创建账号的密码登录。

**技术栈：** Go 1.25.1、Gin、GORM v2、go-redis v8（项目已存在依赖）、SQLite / MySQL / PostgreSQL、React 19、TypeScript、Bun、Rsbuild、Base UI、Tailwind CSS。

---

## 参考文件

- 规格：`docs/superpowers/specs/2026-05-13-token-distribution-platform-spec.md`
- 仓库规则：`AGENTS.md`
- Redis 基线：`go.mod`、`common/redis.go`
- Relay 入口：`controller/relay.go`、`router/relay-router.go`
- 订阅模型：`model/subscription.go`、`model/main.go`
- token 与缓存字段：`dto/openai_response.go`、`dto/claude.go`、`dto/gemini.go`、`service/text_quota.go`、`service/tiered_settle.go`
- 注册认证：`controller/user.go`、`controller/oauth.go`、`controller/github.go`、`controller/misc.go`、`model/option.go`
- 前端订阅：`web/default/src/features/subscriptions/*`
- 前端认证：`web/default/src/features/auth/*`、`web/default/src/features/system-settings/auth/*`

## 文件结构

### 后端

- 修改：`model/subscription.go` —— 套餐 token 快照、订阅 token-only 扣费、创建订阅。
- 修改：`model/main.go` —— AutoMigrate 与 SQLite 加列。
- 创建：`model/trial_code.go` —— 试用码和兑换记录。
- 创建：`model/invitation_reward.go` —— 月度邀请权益记录与有效付费下级统计。
- 创建：`model/subscription_seed.go` —— 5 个默认分销套餐。
- 创建：`service/subscription_meter.go` —— 统一订阅 token 计量，覆盖缓存 token。
- 创建：`service/subscription_concurrency.go` —— 复用 Redis 的用户级并发租约。
- 创建：`service/trial_grant.go` —— 注册 / 建号时发放试用。
- 创建：`service/invitation_reward.go` —— 月度邀请权益评估与 sweep。
- 修改：`service/funding_source.go`、`service/billing_session.go`、`service/quota.go`、`service/text_quota.go` —— 订阅路径切到 token-only。
- 修改：`controller/relay.go` —— 订阅 token 预扣后接入并发租约。
- 创建：`controller/trial_code.go` —— 试用码管理接口。
- 创建：`controller/oauth_onboarding.go` —— GitHub OAuth 首次建号确认接口。
- 修改：`controller/subscription.go` —— 隐藏试用套餐、拒绝购买试用套餐、管理新增字段。
- 修改：`controller/user.go` —— 邮箱注册 trial code 输入、人机校验、邀请试用。
- 修改：`controller/oauth.go`、`controller/github.go` —— pending OAuth session 与建号确认。
- 修改：`controller/discord.go`、`controller/oidc.go`、`controller/linuxdo.go`、`controller/wechat.go` —— GitHub-only signup 下拒绝非 GitHub 创建新用户。
- 修改：`common/constants.go`、`model/option.go`、`controller/misc.go`、`controller/option.go` —— 系统配置与状态输出。
- 修改：`router/api-router.go` —— 试用码管理和 OAuth 建号确认路由。

### 前端

- 修改：`web/default/src/features/subscriptions/types.ts` —— 新套餐字段类型。
- 修改：`web/default/src/features/subscriptions/lib/plan-form.ts` —— 表单 schema、默认值、payload。
- 修改：`web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx` —— 管理端套餐字段。
- 修改：`web/default/src/features/subscriptions/components/subscriptions-columns.tsx` —— 管理端列表列。
- 修改：`web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx` —— 用户购买弹窗规格展示。
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx` —— 套餐卡展示 token 与并发。
- 修改：`web/default/src/features/wallet/components/affiliate-rewards-card.tsx` —— 月度邀请权益进度。
- 创建：`web/default/src/features/trial-codes/*` —— 试用码管理页。
- 创建：`web/default/src/routes/_authenticated/trial-codes/index.tsx` —— 试用码路由。
- 创建：`web/default/src/features/auth/oauth-onboarding/*` —— GitHub OAuth 建号确认页。
- 创建：`web/default/src/routes/(auth)/oauth-onboarding.tsx` —— 建号确认路由。
- 修改：`web/default/src/features/auth/*` 与 `web/default/src/features/system-settings/auth/*` —— GitHub-only signup 展示与设置。
- 创建：`web/default/src/features/app-guides/*` —— AI 应用教程。
- 创建：`web/default/src/routes/_authenticated/app-guides/index.tsx` —— 应用教程路由。

## 任务 1：订阅模型字段与迁移

**文件：**
- 修改：`model/subscription.go`
- 修改：`model/main.go`
- 测试：`model/subscription_distributor_test.go`

- [ ] **步骤 1：编写失败测试**

在 `model/subscription_distributor_test.go` 创建测试，断言从套餐创建订阅时快照 token、并发和 grant reason：

```go
func TestCreateUserSubscriptionFromPlanTx_DistributorSnapshot(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7101, Username: "snapshot_user", Status: common.UserStatusEnabled}).Error)
    plan := &SubscriptionPlan{Id: 7201, Title: "Basic", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true, MonthlyTokenLimit: 1_000_000_000, ConcurrencyLimit: 1, PublicVisible: true, RewardEligible: true, BusinessCode: "basic_monthly"}
    require.NoError(t, DB.Create(plan).Error)
    var sub *UserSubscription
    require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
        created, err := CreateUserSubscriptionFromPlanTx(tx, 7101, plan, "order")
        sub = created
        return err
    }))
    assert.Equal(t, int64(1_000_000_000), sub.TokenLimit)
    assert.Equal(t, int64(0), sub.TokenUsed)
    assert.Equal(t, 1, sub.ConcurrencyLimit)
    assert.Equal(t, "order", sub.GrantReason)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run TestCreateUserSubscriptionFromPlanTx_DistributorSnapshot -count=1`

预期：FAIL，报错包含 `MonthlyTokenLimit undefined` 或 `TokenLimit undefined`。

- [ ] **步骤 3：实现模型字段**

在 `SubscriptionPlan` 增加 `MonthlyTokenLimit`、`ConcurrencyLimit`、`IsTrial`、`PublicVisible`、`TrialDurationHours`、`RewardEligible`、`BusinessCode`。

在 `UserSubscription` 增加 `TokenLimit`、`TokenUsed`、`ConcurrencyLimit`、`GrantReason`、`GrantSourceUserId`。

在 `CreateUserSubscriptionFromPlanTx` 创建订阅时写入 token 和并发快照。`TotalAmount` / `AmountUsed` 保留兼容，但不能作为分销限制依据。

- [ ] **步骤 4：实现迁移**

在 `model/main.go` 的 AutoMigrate 中加入新增表，并在 SQLite 手写表结构中加入 `subscription_plans` 新列。所有新增列都带默认值。

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
git commit -m "feat(subscription): 增加分销套餐 token 快照"
```

## 任务 2：统一 token-only 计量并覆盖缓存 token

**文件：**
- 创建：`service/subscription_meter.go`
- 测试：`service/subscription_meter_test.go`

- [ ] **步骤 1：编写 OpenAI / Responses 缓存 token 测试**

创建 `service/subscription_meter_test.go`：

```go
func TestSubscriptionMeteredTokens_OpenAITotalIncludesCachedTokens(t *testing.T) {
    usage := &dto.Usage{
        PromptTokens:     100,
        CompletionTokens: 50,
        TotalTokens:      150,
        PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 40},
    }
    assert.Equal(t, int64(150), SubscriptionMeteredTokens(usage))
}
```

该测试固定：OpenAI / Responses 的 cached tokens 是 input tokens 子集，不能在 `TotalTokens` 外重复相加。

- [ ] **步骤 2：编写 Anthropic cache creation 测试**

追加：

```go
func TestSubscriptionMeteredTokens_AnthropicCacheTokens(t *testing.T) {
    usage := &dto.Usage{
        UsageSemantic:    "anthropic",
        PromptTokens:     100,
        CompletionTokens: 50,
        PromptTokensDetails: dto.InputTokenDetails{
            CachedTokens: 30,
        },
        ClaudeCacheCreation5mTokens: 7,
        ClaudeCacheCreation1hTokens: 11,
    }
    assert.Equal(t, int64(198), SubscriptionMeteredTokens(usage))
}
```

- [ ] **步骤 3：运行测试验证失败**

运行：`go test ./service -run TestSubscriptionMeteredTokens -count=1`

预期：FAIL，报错包含 `undefined: SubscriptionMeteredTokens`。

- [ ] **步骤 4：实现 `SubscriptionMeteredTokens`**

创建 `service/subscription_meter.go`：

```go
func SubscriptionMeteredTokens(usage *dto.Usage) int64 {
    if usage == nil {
        return 0
    }
    if usage.TotalTokens > 0 && usage.UsageSemantic != "anthropic" {
        return int64(usage.TotalTokens)
    }
    total := usage.PromptTokens + usage.CompletionTokens
    total += usage.PromptTokensDetails.CachedTokens
    if usage.UsageSemantic == "anthropic" {
        total += usage.ClaudeCacheCreation5mTokens
        total += usage.ClaudeCacheCreation1hTokens
        if usage.ClaudeCacheCreation5mTokens == 0 && usage.ClaudeCacheCreation1hTokens == 0 {
            total += usage.PromptTokensDetails.CachedCreationTokens
        }
    } else {
        total += usage.PromptTokensDetails.CachedCreationTokens
    }
    if total < 0 {
        return 0
    }
    return int64(total)
}
```

实现时必须对照现有 `dto.Usage` 字段名调整，避免重复计入 text tokens。若 `PromptTokens` 已包含 text / image / audio 明细，则明细只用于日志，不重复加总。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w service/subscription_meter.go service/subscription_meter_test.go
go test ./service -run TestSubscriptionMeteredTokens -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add service/subscription_meter.go service/subscription_meter_test.go
git commit -m "feat(subscription): 统一订阅 token 计量"
```

## 任务 3：订阅扣费切换为 token-only

**文件：**
- 修改：`model/subscription.go`
- 修改：`service/funding_source.go`
- 修改：`service/billing_session.go`
- 修改：`service/quota.go`
- 修改：`service/text_quota.go`
- 测试：`model/subscription_distributor_test.go`

- [ ] **步骤 1：编写 token-only 失败测试**

追加测试：

```go
func TestPreConsumeUserSubscription_IgnoresAmountTotalForDistributorLimit(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7401, Username: "token_user", Status: common.UserStatusEnabled}).Error)
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7402, Title: "Tiny", Enabled: true, TotalAmount: 1, MonthlyTokenLimit: 10, ConcurrencyLimit: 1}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7403, UserId: 7401, PlanId: 7402, Status: "active", AmountTotal: 1, TokenLimit: 10, EndTime: common.GetTimestamp() + 3600}).Error)
    _, err := PreConsumeUserSubscription("token-only-ok", 7401, "gpt-4o", 0, 6)
    require.NoError(t, err)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run TestPreConsumeUserSubscription_IgnoresAmountTotalForDistributorLimit -count=1`

预期：FAIL，旧逻辑会被 `AmountTotal` 限制。

- [ ] **步骤 3：调整订阅预扣逻辑**

`PreConsumeUserSubscription` 使用 `token_limit - token_used` 判断剩余 token。只有非分销旧订阅才回退 `amount_total`。分销套餐通过 `SubscriptionPlan.business_code != ''` 或订阅 `token_limit > 0 || grant_reason != ''` 判断。

- [ ] **步骤 4：调整实际结算逻辑**

relay 获得 usage 后，订阅路径使用 `service.SubscriptionMeteredTokens(usage)` 的返回值结算。`quota` 仍可用于钱包旧逻辑、日志和成本分析，但不能决定订阅套餐扣减。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w model/subscription.go service/funding_source.go service/billing_session.go service/quota.go service/text_quota.go model/subscription_distributor_test.go
go test ./model -run TestPreConsumeUserSubscription_IgnoresAmountTotalForDistributorLimit -count=1
go test ./service -run TestSubscriptionMeteredTokens -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add model/subscription.go service/funding_source.go service/billing_session.go service/quota.go service/text_quota.go model/subscription_distributor_test.go
git commit -m "feat(subscription): 将订阅扣费切换为 token-only"
```

## 任务 4：Redis 用户级并发租约

**文件：**
- 创建：`service/subscription_concurrency.go`
- 测试：`service/subscription_concurrency_test.go`
- 修改：`common/constants.go`
- 修改：`model/option.go`

- [ ] **步骤 1：编写失败测试**

创建测试，覆盖 acquire 达到上限、release 幂等、Redis 不可用且 fail-closed 时拒绝。

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

创建 `AcquireUserConcurrency(ctx context.Context, userId int, requestId string, limit int)` 和 `ConcurrencyLease.Release(ctx)`。使用项目已有 `common.RDB.Eval` 原子执行 acquire / release。release 必须用 `atomic.Bool` 防止重复释放。

说明：这不新增 Go 依赖；仓库已经依赖 go-redis。生产多实例要精确并发限制时，需要配置 `REDIS_CONN_STRING`。

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

在 `controller/relay.go` 的订阅 token 预扣成功后调用 `service.AcquireSubscriptionConcurrency(c.Request.Context(), relayInfo)`，成功后 `defer lease.Release(context.Background())`。钱包旧逻辑或非订阅请求不受订阅并发限制。

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

## 任务 6：试用码模型与邮箱注册发放

**文件：**
- 创建：`model/trial_code.go`
- 创建：`service/trial_grant.go`
- 修改：`controller/user.go`
- 测试：`model/trial_code_test.go`
- 测试：`controller/auth_github_only_test.go`

- [ ] **步骤 1：编写试用码模型失败测试**

在 `model/trial_code_test.go` 测试有效试用码可创建兑换记录，禁用、过期、超次数和重复领取会拒绝。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run TestConsumeTrialCode -count=1`

预期：FAIL，报错包含 `undefined: TrialCode`。

- [ ] **步骤 3：实现 `TrialCode` 和 `TrialRedemption`**

创建 `model/trial_code.go`，包含 `TrialCode`、`TrialRedemption`、`ConsumeTrialCode(tx *gorm.DB, userId int, rawCode string)`。`ConsumeTrialCode` 必须 trim + 大写 code、校验启用状态、过期时间、最大兑换次数、用户是否已经领取试用，并在同一事务内写入兑换记录和递增 `redeemed_count`。

- [ ] **步骤 4：实现发放服务**

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

逻辑：有 `trial_code` 时消费试用码；没有 `trial_code` 但存在邀请人时发放 `invite_trial`；同一用户已领取试用则拒绝。

- [ ] **步骤 5：接入邮箱注册表单提交**

`controller/user.go` 注册请求增加 `TrialCode` 和 `TurnstileToken`。输入了 trial code 时必须校验 Turnstile 或现有等价人机校验。用户创建和试用发放必须在同一事务内完成。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w model/trial_code.go service/trial_grant.go controller/user.go model/trial_code_test.go controller/auth_github_only_test.go
go test ./model -run TestConsumeTrialCode -count=1
go test ./controller -run TestPasswordRegister_GrantsTrialCode -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add model/trial_code.go service/trial_grant.go controller/user.go model/trial_code_test.go controller/auth_github_only_test.go
git commit -m "feat(trial): 支持邮箱注册手动试用码"
```

## 任务 7：GitHub OAuth 建号确认页后端

**文件：**
- 创建：`controller/oauth_onboarding.go`
- 修改：`controller/github.go`
- 修改：`controller/oauth.go`
- 修改：`router/api-router.go`
- 测试：`controller/auth_github_only_test.go`

- [ ] **步骤 1：编写失败测试**

测试 GitHub OAuth 首次登录未知用户时不直接创建账号，而是返回 pending onboarding 状态；提交 onboarding 时可设置密码、trial code 和 Turnstile token，并创建平台账号与 GitHub 绑定。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./controller -run TestGitHubOAuthOnboarding -count=1`

预期：FAIL，当前逻辑会直接创建账号或没有 onboarding 接口。

- [ ] **步骤 3：实现 pending OAuth session**

GitHub OAuth 成功但没有匹配现有平台用户时，生成短期 pending token，保存 GitHub ID、login、email、avatar、inviterId。pending token 可存 session 或 Redis；多实例部署优先 Redis，复用现有 Redis 能力。

- [ ] **步骤 4：实现建号确认接口**

创建：

```go
func GetOAuthOnboarding(c *gin.Context)
func CompleteOAuthOnboarding(c *gin.Context)
```

提交字段：

```json
{
  "pending_token": "...",
  "trial_code": "TRIAL2026",
  "password": "...",
  "turnstile_token": "..."
}
```

规则：输入 trial code 时必须校验人机；密码可选但如果填写必须符合现有密码规则；创建用户、绑定 GitHub、设置密码、试用发放在同一事务内完成。

- [ ] **步骤 5：增加路由**

在 `router/api-router.go` 增加 OAuth onboarding 路由。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w controller/oauth_onboarding.go controller/github.go controller/oauth.go router/api-router.go controller/auth_github_only_test.go
go test ./controller -run TestGitHubOAuthOnboarding -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add controller/oauth_onboarding.go controller/github.go controller/oauth.go router/api-router.go controller/auth_github_only_test.go
git commit -m "feat(auth): 增加 GitHub 建号确认接口"
```

## 任务 8：月度邀请权益

**文件：**
- 创建：`model/invitation_reward.go`
- 创建：`service/invitation_reward.go`
- 修改：`service/subscription_reset_task.go`
- 修改：`controller/subscription_payment_epay.go`
- 修改：`controller/subscription_payment_stripe.go`
- 测试：`service/invitation_reward_test.go`

- [ ] **步骤 1：编写失败测试**

测试当月存在 2 个直属有效付费下级时，邀请人获得一个 `grant_reason = monthly_invite_entitlement` 的 Basic 权益；同月重复评估不重复发放；只有 1 个有效下级不发放。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./service -run TestMonthlyInvitationEntitlement -count=1`

预期：FAIL，报错包含 `undefined: EnsureMonthlyInvitationEntitlement`。

- [ ] **步骤 3：实现模型和统计函数**

创建 `InvitationMonthlyEntitlement`，唯一索引为 `inviter_id + reward_month`。统计函数只计算直属用户、成功订单、订单金额大于 0、套餐 `reward_eligible = true`、当前存在有效付费订阅的用户，并按用户去重。

- [ ] **步骤 4：实现权益评估服务**

`EnsureMonthlyInvitationEntitlement(inviterId int, at time.Time)`：当前月合格有效直属付费下级数大于等于 2 时，创建当月 Basic 权益订阅，有效期到当前自然月结束。

`RunMonthlyInvitationEntitlementSweep(at time.Time, limit int)`：批量扫描存在直属下级的邀请人，评估当月权益。

- [ ] **步骤 5：接入触发点**

支付成功后触发一次评估。订阅重置 / 过期任务中定期调用 sweep 作为保底。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w model/invitation_reward.go service/invitation_reward.go service/subscription_reset_task.go controller/subscription_payment_epay.go controller/subscription_payment_stripe.go service/invitation_reward_test.go
go test ./service -run TestMonthlyInvitationEntitlement -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add model/invitation_reward.go service/invitation_reward.go service/subscription_reset_task.go controller/subscription_payment_epay.go controller/subscription_payment_stripe.go service/invitation_reward_test.go
git commit -m "feat(invitation): 增加月度邀请权益"
```

## 任务 9：GitHub-only signup 开关

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

测试 `GitHubOnlySignupEnabled = true` 时密码注册被拒绝，但密码登录不因该开关被拒绝。测试 `GetStatus` 返回 `github_only_signup_enabled`。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./controller -run 'TestGitHubOnlySignupRejectsPasswordRegister|TestGitHubOnlySignupKeepsPasswordLogin|TestStatusIncludesGitHubOnlySignup' -count=1`

预期：FAIL，报错包含 `undefined: common.GitHubOnlySignupEnabled`。

- [ ] **步骤 3：增加配置和状态**

新增 `common.GitHubOnlySignupEnabled`，加入 `model.OptionMap` 初始化和更新解析，在 `controller/misc.go` 状态中输出 `github_only_signup_enabled`。在 `controller/option.go` 校验开启前必须已启用 GitHub OAuth 且配置 Client ID。

- [ ] **步骤 4：限制新用户创建入口**

`Register` 开头拒绝 GitHub-only signup。非 GitHub OAuth provider 创建新用户时拒绝。GitHub provider 进入建号确认流程。`Login` 不因该开关拒绝。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w common/constants.go model/option.go controller/misc.go controller/option.go controller/user.go controller/oauth.go controller/discord.go controller/oidc.go controller/linuxdo.go controller/wechat.go controller/auth_github_only_test.go
go test ./controller -run 'TestGitHubOnlySignupRejectsPasswordRegister|TestGitHubOnlySignupKeepsPasswordLogin|TestStatusIncludesGitHubOnlySignup' -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add common/constants.go model/option.go controller/misc.go controller/option.go controller/user.go controller/oauth.go controller/discord.go controller/oidc.go controller/linuxdo.go controller/wechat.go controller/auth_github_only_test.go
git commit -m "feat(auth): 支持仅 GitHub 创建新用户"
```

## 任务 10：订阅购买保护与试用码接口

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

`controller/trial_code.go` 提供 `AdminListTrialCodes`、`AdminCreateTrialCode`、`AdminUpdateTrialCode`、`AdminUpdateTrialCodeStatus`、`AdminDeleteTrialCode`。创建和更新必须校验 `plan_id` 指向试用套餐。

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

## 任务 11：默认分销套餐

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

## 任务 12：前端订阅套餐字段

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

在购买弹窗和钱包套餐卡展示：并发上限、token 限额；`0` token 显示为 `Unlimited tokens`，十亿级显示为 `1B tokens`。不展示原 quota 或价格倍率配额。

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

## 任务 13：前端试用码、邮箱注册和 OAuth 建号确认

**文件：**
- 创建：`web/default/src/features/trial-codes/api.ts`
- 创建：`web/default/src/features/trial-codes/types.ts`
- 创建：`web/default/src/features/trial-codes/index.tsx`
- 创建：`web/default/src/features/trial-codes/components/trial-codes-table.tsx`
- 创建：`web/default/src/features/trial-codes/components/trial-code-mutate-drawer.tsx`
- 创建：`web/default/src/features/trial-codes/components/trial-code-delete-dialog.tsx`
- 创建：`web/default/src/routes/_authenticated/trial-codes/index.tsx`
- 修改：`web/default/src/features/auth/sign-up/components/sign-up-form.tsx`
- 创建：`web/default/src/features/auth/oauth-onboarding/index.tsx`
- 创建：`web/default/src/features/auth/oauth-onboarding/components/oauth-onboarding-form.tsx`
- 创建：`web/default/src/routes/(auth)/oauth-onboarding.tsx`

- [ ] **步骤 1：创建试用码管理类型和 API**

`types.ts` 定义 `trialCodeSchema` 和 `TrialCodePayload`。`api.ts` 封装管理端 CRUD 接口。

- [ ] **步骤 2：创建试用码管理页面**

按现有 `features/subscriptions` 表格、抽屉、删除对话框模式实现。列表展示 code、plan_id、enabled、max_redemptions、redeemed_count、expires_at。

- [ ] **步骤 3：邮箱注册表单增加 trial code**

`sign-up-form.tsx` 增加可选 trial code 输入框。用户填写 trial code 时，提交 payload 包含 `trial_code` 和 Turnstile token。

- [ ] **步骤 4：创建 OAuth 建号确认页**

页面展示 GitHub 用户名和邮箱，提供 trial code、密码、确认密码、Turnstile、协议勾选。提交到后端 `CompleteOAuthOnboarding`。

- [ ] **步骤 5：运行前端检查**

运行：

```bash
cd web/default
bun run typecheck
bun run lint
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add web/default/src/features/trial-codes web/default/src/routes/_authenticated/trial-codes/index.tsx web/default/src/features/auth/sign-up/components/sign-up-form.tsx web/default/src/features/auth/oauth-onboarding web/default/src/routes/(auth)/oauth-onboarding.tsx
git commit -m "feat(web): 增加试用码与 OAuth 建号确认"
```

## 任务 14：前端 GitHub-only signup 体验

**文件：**
- 修改：`web/default/src/features/auth/types.ts`
- 修改：`web/default/src/features/auth/sign-in/components/user-auth-form.tsx`
- 修改：`web/default/src/features/auth/sign-up/components/sign-up-form.tsx`
- 修改：`web/default/src/features/auth/components/oauth-providers.tsx`
- 修改：`web/default/src/features/system-settings/auth/basic-auth-section.tsx`
- 修改：`web/default/src/features/system-settings/auth/oauth-section.tsx`

- [ ] **步骤 1：更新状态类型**

`SystemStatus` 增加 `github_only_signup_enabled?: boolean`，同时兼容直接字段和 `data` 内字段。

- [ ] **步骤 2：调整注册和登录页**

GitHub-only signup 为 true 时，注册页隐藏邮箱注册表单，只显示 GitHub 创建账号入口。登录页保留密码登录，因为 GitHub 创建账号可以设置密码。

- [ ] **步骤 3：过滤注册 provider**

`OAuthProviders` 在注册语境下只展示 GitHub provider；登录语境按现有配置展示。

- [ ] **步骤 4：增加系统设置开关**

认证设置页增加「仅允许 GitHub 创建新用户」开关。开启时提示必须先配置 GitHub OAuth。

- [ ] **步骤 5：运行前端检查**

运行：

```bash
cd web/default
bun run typecheck
bun run lint
```

预期：PASS。

- [ ] **步骤 6：Commit**

运行：

```bash
git add web/default/src/features/auth/types.ts web/default/src/features/auth/sign-in/components/user-auth-form.tsx web/default/src/features/auth/sign-up/components/sign-up-form.tsx web/default/src/features/auth/components/oauth-providers.tsx web/default/src/features/system-settings/auth/basic-auth-section.tsx web/default/src/features/system-settings/auth/oauth-section.tsx
git commit -m "feat(web): 适配仅 GitHub 创建新用户"
```

## 任务 15：应用配置教程和邀请权益展示

**文件：**
- 创建：`web/default/src/features/app-guides/types.ts`
- 创建：`web/default/src/features/app-guides/index.tsx`
- 创建：`web/default/src/features/app-guides/components/app-guide-card.tsx`
- 创建：`web/default/src/features/app-guides/lib/build-config.ts`
- 创建：`web/default/src/routes/_authenticated/app-guides/index.tsx`
- 修改：`web/default/src/features/wallet/components/affiliate-rewards-card.tsx`

- [ ] **步骤 1：创建配置生成工具**

`build-config.ts` 提供 `buildOpenAIBaseUrl`、`buildCherryStudioConfig` 等纯函数。

- [ ] **步骤 2：创建教程页面**

教程覆盖 Cherry Studio、Chatbox、LobeChat、NextChat、Open WebUI、Continue、Cline、Claude Code / CC Switch。每张卡包含 Base URL、API Key 选择、复制按钮和一键导入链接。

- [ ] **步骤 3：更新邀请权益卡**

展示直属邀请人数、当前有效付费直属下级人数、本月是否已获得 Basic 权益、权益有效期。

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

## 任务 16：最终验证

**文件：**
- 检查全部已修改文件

- [ ] **步骤 1：后端精准测试**

运行：

```bash
go test ./model -run 'Test.*Subscription|Test.*Trial|Test.*Invitation|TestCompleteSubscriptionOrder' -count=1
go test ./service -run 'Test.*SubscriptionConcurrency|Test.*Meter|Test.*Billing|Test.*TextQuota' -count=1
go test ./controller -run 'Test.*GitHubOnly|Test.*Subscription|Test.*Trial|Test.*OAuthOnboarding' -count=1
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

验证场景：邮箱用户手动输入 trial code 注册获得 Trial；GitHub OAuth 新用户完成建号确认页，输入 trial code、设置密码，获得 Trial，并可用 GitHub 用户名或邮箱加密码登录；Trial 同时发起 2 个流式请求时第 2 个返回 429；Basic 用户调用 `/v1/chat/completions` 和 `/v1/responses` 后 token 增加且缓存 token 不重复计入；两个直属有效付费下级触发邀请人当月 Basic 权益；重复支付回调不重复发放。

- [ ] **步骤 5：最终 Commit**

运行：

```bash
git status --short
git add .
git commit -m "feat(distributor): 完成 token 分销平台定制"
```

预期：工作区只包含本次实现相关文件并成功提交。
