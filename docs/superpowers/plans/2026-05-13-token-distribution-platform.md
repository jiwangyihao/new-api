# New API token 分销平台实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 基于 New API 增加 token-only 分销套餐、手动 trial code、OAuth 建号确认、月度邀请权益、实时并发和应用配置教程能力。

**架构：** 复用现有 `router/ -> controller/ -> service/ -> model/` 分层。分销套餐只使用 token 作为限制和展示口径；实时并发复用项目已有 go-redis 支持；试用码在邮箱注册表单或 OAuth 首次建号确认页由用户手动输入；邀请权益按月评估直属有效付费下级；GitHub-only 只限制新用户创建方式，不禁用 GitHub 创建账号的密码登录。

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
- 修改：`model/main.go` —— 分阶段 AutoMigrate 与 SQLite 加列；任务 1 只迁移订阅字段，任务 6 迁移试用码表，任务 7 如选择唯一索引方案迁移 OAuth provider 去重约束，任务 8 迁移月度邀请权益表。
- 创建：`model/trial_code.go` —— 试用码和兑换记录。
- 创建：`model/invitation_reward.go` —— 月度邀请权益记录与有效付费下级统计。
- 创建：`model/subscription_seed.go` —— 5 个默认分销套餐。
- 创建：`service/subscription_meter.go` —— 统一订阅 token 计量，覆盖缓存 token。
- 创建：`service/subscription_concurrency.go` —— 复用 Redis 的用户级并发租约。
- 创建：`service/trial_grant.go` —— 注册 / 建号时发放试用。
- 创建：`service/invitation_reward.go` —— 月度邀请权益评估与 sweep。
- 修改：`service/funding_source.go`、`service/billing.go`、`service/billing_session.go`、`service/quota.go`、`service/text_quota.go`、`service/task_billing.go` —— 订阅路径切到 token-only，并排除非文本异步任务的分销订阅扣费。
- 修改：`controller/relay.go` —— 订阅 token 预扣后接入并发租约。
- 创建：`controller/trial_code.go` —— 试用码管理接口。
- 创建：`controller/oauth_onboarding.go` —— OAuth 首次建号确认接口。
- 修改：`controller/subscription.go` —— 隐藏试用套餐、管理新增字段。
- 修改：`controller/subscription_payment_epay.go`、`controller/subscription_payment_stripe.go`、`controller/subscription_payment_creem.go`、`controller/topup_stripe.go`、`controller/topup_creem.go` —— 拒绝购买试用套餐并触发月度邀请权益评估。
- 修改：`controller/user.go` —— 邮箱注册 trial code 输入、人机校验、邀请试用。
- 修改：`controller/oauth.go`、`controller/github.go`、`controller/discord.go`、`controller/oidc.go`、`controller/linuxdo.go`、`controller/wechat.go`、`model/user.go`、`model/main.go` —— pending OAuth session、建号确认、provider 绑定并发保护与 GitHub-only signup 下非 GitHub 新用户创建限制。
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
- 创建：`web/default/src/features/auth/oauth-onboarding/*` —— OAuth 建号确认页。
- 创建：`web/default/src/routes/(auth)/oauth-onboarding.tsx` —— 建号确认路由。
- 修改：`web/default/src/features/auth/*` 与 `web/default/src/features/system-settings/auth/*` —— GitHub-only signup 展示与设置。
- 创建：`web/default/src/features/app-guides/*` —— AI 应用教程。
- 创建：`web/default/src/routes/_authenticated/app-guides/index.tsx` —— 应用教程路由。

## 并行开发边界

后续使用子代理直接在主分支开发时，按下列 wave 控制冲突；每个新启动子代理必须收到本计划完整路径、规格完整路径和对应 wave 边界，不运行项目级格式化器或项目级验证。

- **Wave A（基础层，串行）：** 任务 1 -> 任务 2 -> 任务 3 -> 任务 3.5 -> 任务 4。任务 1/3 都修改 `model/subscription.go`，任务 2/3 共享 token 计量契约，任务 3/3.5 共享 billing/funding/task billing 边界，任务 4 提供并发 limiter 给任务 5 使用，不能并行。
- **Wave B（relay 与注册后端，可有限并行）：** 任务 5 可在任务 4 完成后独立实现；任务 6 可在任务 1 完成后实现，但会修改 `model/main.go` 和 `controller/user.go`；任务 7 必须在任务 6 的 trial grant 契约稳定后实现；任务 9 在任务 7 之后限制 GitHub-only signup 的新用户创建入口。任务 6 与任务 7 不要同时修改 `router/api-router.go` 和 `model/main.go`。
- **Wave C（权益、购买保护、默认套餐，可按文件边界并行）：** 任务 8 与任务 10 都修改 `controller/subscription_payment_epay.go`、`controller/subscription_payment_stripe.go`、`controller/subscription_payment_creem.go`，必须串行执行或交给同一子代理；任务 11 可在 Wave A 完成后并行。这三个订阅支付文件属于共享文件锁。
- **Wave D（前端，可按页面并行）：** 任务 12（订阅 UI）可独立；任务 13（注册与 OAuth onboarding）必须等任务 7 响应契约确定；任务 14（GitHub-only signup 展示）会改 `sign-up-form.tsx` 和系统设置，需在任务 13 后或由同一子代理完成；任务 15（应用教程/邀请权益）涉及导航配置、`routeTree.gen.ts` 和侧边栏配置，需在任务 13 的 routeTree 与侧边栏更新后执行或由同一子代理完成。
- **共享文件锁：** `model/main.go` 由任务 1/6/7/8 分阶段改；`router/api-router.go` 由任务 6/7/8/10 分阶段改；`controller/subscription_payment_epay.go`、`controller/subscription_payment_stripe.go`、`controller/subscription_payment_creem.go` 由任务 8/10 串行改；`service/funding_source.go`、`service/billing.go`、`service/billing_session.go`、`service/quota.go`、`service/text_quota.go`、`service/task_billing.go` 由任务 3/3.5 串行改；`web/default/src/features/auth/sign-up/components/sign-up-form.tsx` 由任务 13/14 分阶段改；`web/default/src/routeTree.gen.ts`、`web/default/src/hooks/use-sidebar-data.ts`、`web/default/src/hooks/use-sidebar-config.ts`、`web/default/src/features/system-settings/maintenance/config.ts`、`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx` 由任务 13/15 分阶段生成和提交。子代理不得同时编辑这些文件。
- **验证职责：** 子代理只运行自己新增或修改测试的窄范围命令；前端 `bun run typecheck`、`bun run lint`、`bun run build` 和全量后端受影响包验证只在任务 16 由主会话统一执行。
- **许可边界：** 用户已接受 New API AGPL-3.0 开源合规；本计划不切换 base 项目，不修改许可证，不删除 AGPL 义务提示。

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
    businessCode := "basic_monthly"
    plan := &SubscriptionPlan{Id: 7201, Title: "Basic", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true, MonthlyTokenLimit: 1_000_000_000, ConcurrencyLimit: 1, PublicVisible: true, RewardEligible: true, BusinessCode: &businessCode}
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

func TestSubscriptionPlanBusinessCode_AllowsMultipleLegacyNulls(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7202, Title: "Legacy A", Enabled: true}).Error)
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7203, Title: "Legacy B", Enabled: true}).Error)
}

func TestSubscriptionPlanBusinessCode_RejectsDuplicateNonEmpty(t *testing.T) {
    truncateTables(t)
    code := "basic_monthly"
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7204, Title: "Basic A", Enabled: true, BusinessCode: &code}).Error)
    dup := "basic_monthly"
    require.Error(t, DB.Create(&SubscriptionPlan{Id: 7205, Title: "Basic B", Enabled: true, BusinessCode: &dup}).Error)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run 'TestCreateUserSubscriptionFromPlanTx_DistributorSnapshot|TestSubscriptionPlanBusinessCode' -count=1`

预期：FAIL，报错包含 `MonthlyTokenLimit undefined` 或 `TokenLimit undefined`。

- [ ] **步骤 3：实现模型字段**

在 `SubscriptionPlan` 增加 `MonthlyTokenLimit`、`ConcurrencyLimit`、`IsTrial`、`PublicVisible`、`TrialDurationHours`、`RewardEligible`、`BusinessCode`。`BusinessCode` 使用可空字符串（例如 `*string`），历史套餐和普通自定义套餐默认为 `NULL`；唯一约束只约束非空业务标识，不能让多个旧套餐因空字符串冲突。

在 `UserSubscription` 增加 `TokenLimit`、`TokenUsed`、`ConcurrencyLimit`、`GrantReason`、`GrantSourceUserId`。

在 `CreateUserSubscriptionFromPlanTx` 创建订阅时写入 token 和并发快照。`TotalAmount` / `AmountUsed` 保留兼容，但不能作为分销限制依据。

- [ ] **步骤 4：实现迁移**

仅在 `model/main.go` 中迁移 `SubscriptionPlan` / `UserSubscription` 新字段。SQLite 手写维护仅适用于现有 `ensureSubscriptionPlanTableSQLite()` 的 `subscription_plans` 新列；`user_subscriptions` 通过 GORM `AutoMigrate` 加列，并用 SQLite smoke 测试确认。新增列除 `business_code` 外都带默认值；`business_code` 迁移默认 `NULL`，SQLite/MySQL/PostgreSQL 都必须允许多条 legacy plan 的 `business_code` 为 `NULL`，同时拒绝重复非空 `business_code`。本任务不得加入 `TrialCode`、`TrialRedemption`、`InvitationMonthlyEntitlement` 表迁移；它们分别由任务 6 和任务 8 负责。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w model/subscription.go model/main.go model/subscription_distributor_test.go
go test ./model -run 'TestCreateUserSubscriptionFromPlanTx_DistributorSnapshot|TestSubscriptionPlanBusinessCode' -count=1
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

```go
func TestSubscriptionMeteredTokens_OpenAIPromptAlreadyIncludesCachedWhenTotalMissing(t *testing.T) {
    usage := &dto.Usage{
        PromptTokens:     100,
        CompletionTokens: 50,
        PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 40},
    }
    assert.Equal(t, int64(150), SubscriptionMeteredTokens(usage))
}
```

该测试固定：非 Anthropic/Claude provider 即使缺少 `TotalTokens`，也不能默认把 cached 明细加到 `PromptTokens + CompletionTokens` 外；无 `TotalTokens` 不等于 cached 未包含在 prompt/input 中。

- [ ] **步骤 2：编写 Anthropic cache creation 测试**

追加：

```go
func TestSubscriptionMeteredTokens_AnthropicNativeCacheTokens(t *testing.T) {
    usage := &dto.Usage{
        UsageSemantic:    "anthropic",
        TotalTokens:      150,
        PromptTokens:     100,
        CompletionTokens: 50,
        PromptTokensDetails: dto.InputTokenDetails{
            CachedTokens: 30,
            CachedCreationTokens: 50,
        },
        ClaudeCacheCreation5mTokens: 7,
        ClaudeCacheCreation1hTokens: 11,
    }
    assert.Equal(t, int64(230), SubscriptionMeteredTokens(usage))
}
```

该测试固定：原生 Anthropic / Claude usage 即使 `TotalTokens` 只等于 input + output，也必须额外补计 cache read/create；Claude cache creation 总量大于 5m / 1h 拆分之和时，必须按总量计入一次，不能只扣拆分之和。当前仓库已有同属 `service` 包的 `NormalizeCacheCreationSplit(total, tokens5m, tokens1h)` 处理 remainder；`service/subscription_meter.go` 内实现必须直接调用 `NormalizeCacheCreationSplit(...)`，或等价使用 `max(CachedCreationTokens, split5m+split1h)`。

```go
func TestSubscriptionMeteredTokens_AnthropicOpenAIStyleUsageDoesNotDoubleCountCache(t *testing.T) {
    usage := &dto.Usage{
        UsageSemantic:    "openai",
        UsageSource:      "anthropic",
        PromptTokens:     180,
        CompletionTokens: 50,
        TotalTokens:      230,
        PromptTokensDetails: dto.InputTokenDetails{
            CachedTokens: 30,
            CachedCreationTokens: 50,
        },
        ClaudeCacheCreation5mTokens: 7,
        ClaudeCacheCreation1hTokens: 11,
    }
    assert.Equal(t, int64(230), SubscriptionMeteredTokens(usage))
}
```

该测试固定：现有 Claude -> OpenAI-style 归一化路径若已把 cache read/create 合入 `TotalTokens`，并通过 `UsageSource = "anthropic"` 或等价字段标识来源，订阅计量直接使用 `TotalTokens`，不得再次补加 cached 明细。

- [ ] **步骤 3：编写 Gemini cached content 测试**

追加：

```go
func TestSubscriptionMeteredTokens_GeminiCachedContentTokens(t *testing.T) {
    usage := &dto.Usage{
        UsageSemantic:    "gemini",
        PromptTokens:     100,
        CompletionTokens: 50,
        TotalTokens:      150,
        PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 40},
    }
    assert.Equal(t, int64(150), SubscriptionMeteredTokens(usage))
}
```

该测试固定 `relay/channel/gemini/relay-gemini.go` 归一化后的语义：Gemini `cachedContentTokenCount` 记录到 cached 明细用于审计；若 `TotalTokens` 已给出，订阅扣减返回 `TotalTokens`，不能额外加 cached content。

- [ ] **步骤 4：运行测试验证失败**

运行：`go test ./service -run TestSubscriptionMeteredTokens -count=1`

预期：FAIL，报错包含 `undefined: SubscriptionMeteredTokens`。

- [ ] **步骤 5：实现 `SubscriptionMeteredTokens`**

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
    if usage.UsageSemantic == "anthropic" {
        total += usage.PromptTokensDetails.CachedTokens
        cacheCreation5m, cacheCreation1h := NormalizeCacheCreationSplit(
            usage.PromptTokensDetails.CachedCreationTokens,
            usage.ClaudeCacheCreation5mTokens,
            usage.ClaudeCacheCreation1hTokens,
        )
        total += cacheCreation5m + cacheCreation1h
    }
    if total < 0 {
        return 0
    }
    return int64(total)
}
```

实现时必须对照现有 `dto.Usage` 字段名调整，避免重复计入 text tokens。规则是：OpenAI / Responses / Gemini 已有 `TotalTokens` 时直接使用 `TotalTokens`；缺少 `TotalTokens` 时默认使用 `PromptTokens + CompletionTokens`，不能默认额外加 cached 明细。cached tokens 计入套餐消耗，但它们通常已经包含在 `PromptTokens` / `InputTokens` / `TotalTokens` 中，只单独记录明细，不在总 token 外再次相加。原生 Anthropic / Claude（`UsageSemantic = "anthropic"` 或现有源码等价标识）即使 `TotalTokens` 只等于 input + output，也必须补计 cache read/create；已归一化为 OpenAI-style 的 Claude usage（例如 `UsageSemantic = "openai"` 且 `UsageSource = "anthropic"`，`TotalTokens` 已包含 cache）必须直接返回 `TotalTokens`，不得重复补计。新增其他 provider 语义前必须先补测试。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w service/subscription_meter.go service/subscription_meter_test.go
go test ./service -run TestSubscriptionMeteredTokens -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add service/subscription_meter.go service/subscription_meter_test.go
git commit -m "feat(subscription): 统一订阅 token 计量"
```

## 任务 3：订阅扣费切换为 token-only

**文件：**
- 修改：`model/subscription.go`
- 修改：`service/funding_source.go`
- 修改：`service/billing.go`
- 修改：`service/billing_session.go`
- 修改：`service/quota.go`
- 修改：`service/text_quota.go`
- 测试：`model/subscription_distributor_test.go`
- 测试：`service/subscription_billing_test.go`

- [ ] **步骤 1：编写 token-only 失败测试**

追加测试：

```go
func TestPreConsumeUserSubscription_IgnoresAmountTotalForDistributorLimit(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7401, Username: "token_user", Status: common.UserStatusEnabled}).Error)
    tinyCode := "tiny"
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7402, Title: "Tiny", Enabled: true, TotalAmount: 1, MonthlyTokenLimit: 10, ConcurrencyLimit: 1, BusinessCode: &tinyCode}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7403, UserId: 7401, PlanId: 7402, Status: "active", AmountTotal: 1, TokenLimit: 10, EndTime: common.GetTimestamp() + 3600, GrantReason: "order"}).Error)
    _, err := PreConsumeUserSubscription("token-only-ok", 7401, "gpt-4o", 0, 6)
    require.NoError(t, err)
}

func TestSettleUserSubscription_UsesTokenUsedForDistributor(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7411, Username: "settle_user", Status: common.UserStatusEnabled}).Error)
    settleCode := "settle"
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7412, Title: "Settle", Enabled: true, MonthlyTokenLimit: 10, ConcurrencyLimit: 1, BusinessCode: &settleCode}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7413, UserId: 7411, PlanId: 7412, Status: "active", TokenLimit: 10, TokenUsed: 0, AmountTotal: 1, EndTime: common.GetTimestamp() + 3600, GrantReason: "order"}).Error)
    pre, err := PreConsumeUserSubscription("token-settle", 7411, "gpt-4o", 0, 6)
    require.NoError(t, err)
    require.NoError(t, PostConsumeUserSubscriptionDelta(pre.UserSubscriptionId, 2))
    var sub UserSubscription
    require.NoError(t, DB.First(&sub, 7413).Error)
    assert.Equal(t, int64(8), sub.TokenUsed)
}

func TestRefundUserSubscription_UsesRequestIDForDistributor(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7421, Username: "refund_user", Status: common.UserStatusEnabled}).Error)
    refundCode := "refund"
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7422, Title: "Refund", Enabled: true, MonthlyTokenLimit: 10, ConcurrencyLimit: 1, BusinessCode: &refundCode}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7423, UserId: 7421, PlanId: 7422, Status: "active", TokenLimit: 10, TokenUsed: 0, AmountTotal: 1, EndTime: common.GetTimestamp() + 3600, GrantReason: "order"}).Error)
    _, err := PreConsumeUserSubscription("token-refund", 7421, "gpt-4o", 0, 6)
    require.NoError(t, err)
    require.NoError(t, RefundSubscriptionPreConsume("token-refund"))
    var sub UserSubscription
    require.NoError(t, DB.First(&sub, 7423).Error)
    assert.Equal(t, int64(0), sub.TokenUsed)
}
```

- [ ] **步骤 2：编写 service 真实结算路径失败测试**

在 `service/subscription_billing_test.go` 增加测试，构造订阅资金来源的真实 billing/session 结算路径，调用改造后的 `BillingSession.Settle` 或最接近真实 relay 的 helper，而不是只测 `model.PostConsumeUserSubscriptionDelta`。断言订阅路径的 `TokenUsed` 增量来自 `SubscriptionMeteredTokens(usage)`，不是 `summary.Quota`；钱包路径和 token key quota 仍使用 quota。至少覆盖：OpenAI `TotalTokens`、Responses cached tokens、Claude cache creation remainder、Gemini cached content。

追加 no-usage 测试：当上游未返回 usage 时，订阅实际结算不得使用 `SubscriptionMeteredTokens(nil) == 0` 直接结算为 0；必须使用现有 relay 估算逻辑或显式 estimated tokens，并在 consume log `other` 写入 `usage_estimated = true`。测试还必须断言钱包用户 quota 和 token key quota 在该路径仍按 `summary.Quota` 扣减，而不是被 estimated token 替换。

- [ ] **步骤 3：运行测试验证失败**

运行：

```bash
go test ./model -run 'TestPreConsumeUserSubscription_IgnoresAmountTotalForDistributorLimit|TestSettleUserSubscription_UsesTokenUsedForDistributor|TestRefundUserSubscription_UsesRequestIDForDistributor' -count=1
go test ./service -run 'TestSubscriptionBillingUsesMeteredTokens|TestSubscriptionBillingUsesEstimatedTokensWhenUsageMissing|TestWalletBillingStillUsesQuota|TestTokenKeyQuotaStillUsesQuotaWhenSubscriptionUsesTokens' -count=1
```

预期：FAIL，旧逻辑会被 `AmountTotal` 限制、只更新 `AmountUsed`、或在真实结算路径继续按 `summary.Quota` / 0 usage 扣订阅。

- [ ] **步骤 4：调整订阅预扣逻辑**

`PreConsumeUserSubscription` 使用 `token_limit - token_used` 判断剩余 token。只有非分销旧订阅才回退 `amount_total`。分销订阅必须来自非空 distributor `business_code` 或显式 `token_limit` / `concurrency_limit` 配置；`token_limit = 0` 只有 `trial_code` / `invite_trial` 试用订阅表示不限量，旧计划或管理员赠送订阅不得因 `grant_reason` 非空被误判为无限正式分销套餐，并补对应测试。

- [ ] **步骤 5：调整实际结算、退款和 relay 同步逻辑**

relay 获得 usage 后，订阅路径使用 `service.SubscriptionMeteredTokens(usage)` 的返回值结算。`quota` 仍用于钱包资金来源、token key quota、日志和成本分析，不能决定订阅套餐扣减。若 usage 缺失，订阅路径必须使用现有 token 估算结果结算并记录 `usage_estimated = true`，不能把缺失 usage 结算为 0。

必须显式切开结算接口中的两种口径：新增 `SettleBillingWithUsage(ctx, relayInfo, walletQuota int, usage *dto.Usage, estimatedTokens int64, usageEstimated bool)`，或把 `BillingSession.Settle` 的入参改为结构体（例如 `BillingSettleInput{WalletQuota, SubscriptionTokens, UsageEstimated}`）。钱包资金来源和 token key quota 继续使用 `WalletQuota` / `summary.Quota`；订阅资金来源只使用 `SubscriptionTokens`。不得把 `summary.Quota` 直接替换为 token 值传入现有 `SettleBilling(ctx, relayInfo, actualQuota int)`，否则会破坏钱包与 token key 扣费。`PostTextConsumeQuota` 必须调用新接口；`PostAudioConsumeQuota` 和非目标 relay/异步任务的订阅处理见任务 3.5，不得默认消耗分销订阅 token。

非文本 relay / 异步任务（images、audio-only、embeddings、rerank、Midjourney、Suno、视频等）不纳入分销订阅 token 消耗，不能更新 `UserSubscription.TokenUsed`；这些路径的处理见任务 3.5。

必须同步修改：

- `PostConsumeUserSubscriptionDelta`：分销订阅实际结算只更新 `token_used`，补扣差额按 token 计算。
- `RefundSubscriptionPreConsume`：分销订阅退款只回退 `token_used`，不能依赖 `amount_used`。
- `SubscriptionPreConsumeResult`、`SubscriptionFunding`、`BillingSession.syncRelayInfo`：携带并同步 token limit / token used / token remaining 字段；旧 `SubscriptionAmountTotal` 仅兼容旧 UI 和旧订阅。
- 订阅余额通知、日志字段和错误提示：分销套餐展示 token 剩余，不展示价格倍率或 quota 剩余。
- 周期重置：分销订阅清零 `token_used`；旧 `amount_used` 可同步清零但不能作为限制依据。
- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w model/subscription.go service/funding_source.go service/billing.go service/billing_session.go service/quota.go service/text_quota.go model/subscription_distributor_test.go service/subscription_billing_test.go
go test ./model -run 'TestPreConsumeUserSubscription_IgnoresAmountTotalForDistributorLimit|TestSettleUserSubscription_UsesTokenUsedForDistributor|TestRefundUserSubscription_UsesRequestIDForDistributor' -count=1
go test ./service -run 'TestSubscriptionMeteredTokens|TestSubscriptionBillingUsesMeteredTokens|TestSubscriptionBillingUsesEstimatedTokensWhenUsageMissing|TestWalletBillingStillUsesQuota|TestTokenKeyQuotaStillUsesQuotaWhenSubscriptionUsesTokens' -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add model/subscription.go service/funding_source.go service/billing.go service/billing_session.go service/quota.go service/text_quota.go model/subscription_distributor_test.go service/subscription_billing_test.go
git commit -m "feat(subscription): 将订阅扣费切换为 token-only"
```

## 任务 3.5：非文本任务不消耗分销订阅 token

**文件：**
- 修改：`service/funding_source.go`
- 修改：`service/billing.go`
- 修改：`service/task_billing.go`
- 修改：`controller/relay.go`
- 修改：`relay/relay_task.go`
- 测试：`service/task_billing_test.go`
- 测试：`controller/subscription_non_text_billing_test.go`

- [ ] **步骤 1：编写失败测试**

增加测试覆盖非文本 relay / 异步任务不能使用分销订阅 funding：

- `TestRelayTaskDoesNotPreConsumeDistributorSubscription`：Suno / video / Midjourney 等 `RelayTask` 提交时，即使用户 billing preference 是 `subscription_first`，也不能把 `BillingSourceSubscription` / `SubscriptionId` 写入 `Task.PrivateData`。
- `TestTaskBillingDoesNotAdjustDistributorSubscription`：构造带订阅来源的历史任务或异常路径时，`taskAdjustFunding` 不得更新分销 `UserSubscription.TokenUsed`；应拒绝、退款到钱包旧逻辑或返回明确错误。
- `TestPostAudioConsumeQuotaDoesNotConsumeDistributorSubscription`：audio-only 或非文本 quota 结算路径不得调用分销订阅 token 结算；文本生成响应中的 audio token 明细仍可由任务 3 的 text usage 路径计入。

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
go test ./service -run 'TestTaskBillingDoesNotAdjustDistributorSubscription|TestPostAudioConsumeQuotaDoesNotConsumeDistributorSubscription' -count=1
go test ./controller -run 'TestRelayTaskDoesNotPreConsumeDistributorSubscription' -count=1
```

预期：FAIL，现有 `RelayTask` / `task_billing.go` 仍可能通过订阅 funding 预扣、结算或退款非文本任务。

- [ ] **步骤 3：实现非文本 funding 过滤**

新增 helper（例如 `IsDistributorSubscriptionEligibleRelay(relayInfo)`）集中判断只有 chat / responses / responses compact / 同步文本生成类 relay 可使用分销订阅 funding。`PreConsumeBilling` 或 funding 选择层在非目标 relay mode 下不得选择 `SubscriptionFunding`；若用户是 `subscription_only` 且请求非文本任务，返回明确错误，提示该任务不支持订阅套餐扣费。`task_billing.go` 对历史 private data 中的订阅来源做防御：遇到分销订阅 id 时不得调用 `PostConsumeUserSubscriptionDelta` 更新 `TokenUsed`。

- [ ] **步骤 4：运行测试验证通过**

运行：

```bash
gofmt -w service/funding_source.go service/billing.go service/task_billing.go controller/relay.go relay/relay_task.go service/task_billing_test.go controller/subscription_non_text_billing_test.go
go test ./service -run 'TestTaskBillingDoesNotAdjustDistributorSubscription|TestPostAudioConsumeQuotaDoesNotConsumeDistributorSubscription' -count=1
go test ./controller -run 'TestRelayTaskDoesNotPreConsumeDistributorSubscription' -count=1
```

预期：PASS。

- [ ] **步骤 5：Commit**

运行：

```bash
git add service/funding_source.go service/billing.go service/task_billing.go controller/relay.go relay/relay_task.go service/task_billing_test.go controller/subscription_non_text_billing_test.go
git commit -m "feat(subscription): 排除非文本任务订阅 token 扣费"
```

## 任务 4：Redis 用户级并发租约

**文件：**
- 创建：`service/subscription_concurrency.go`
- 测试：`service/subscription_concurrency_test.go`
- 修改：`common/constants.go`
- 修改：`model/option.go`

- [ ] **步骤 1：编写失败测试**

创建测试，覆盖 acquire 达到上限、release 幂等、Redis disabled 单实例进程内 limiter、Redis disabled 且要求共享 Redis 时 fail-closed / fail-open、Redis enabled 但命令失败时 fail-closed / fail-open 行为。

测试 seam：实现 `redisEvaler` 接口并让生产代码默认使用 `common.RDB`；测试中用 `setSubscriptionConcurrencyRedisForTest(t, fakeEvaler)` 注入返回错误或成功的 fake，并在 `t.Cleanup` 恢复。新增 `setSubscriptionConcurrencyOptionsForTest(t, requireRedis, failOpen bool, ttlSeconds int)` 或在每个测试里用 `t.Cleanup` 恢复 `common.RedisEnabled`、`SubscriptionConcurrencyRequireRedis`、`SubscriptionConcurrencyFailOpen`、TTL 和内存 limiter，避免全局状态污染包级测试。不得通过真实网络 Redis 制造错误。

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

func TestSubscriptionConcurrencyRequiresRedisWhenConfigured(t *testing.T) {
    common.RedisEnabled = false
    common.SubscriptionConcurrencyRequireRedis = true
    common.SubscriptionConcurrencyFailOpen = false
    _, err := AcquireUserConcurrency(context.Background(), 7502, "req", 1)
    require.ErrorIs(t, err, ErrSubscriptionConcurrencyUnavailable)
}

func TestSubscriptionConcurrencyFailOpenWhenRedisRequired(t *testing.T) {
    common.RedisEnabled = false
    common.SubscriptionConcurrencyRequireRedis = true
    common.SubscriptionConcurrencyFailOpen = true
    lease, err := AcquireUserConcurrency(context.Background(), 7503, "req", 1)
    require.NoError(t, err)
    require.NoError(t, lease.Release(context.Background()))
}

func TestSubscriptionConcurrencyFailClosedWhenRedisCommandFails(t *testing.T) {
    common.RedisEnabled = true
    common.SubscriptionConcurrencyFailOpen = false
    setSubscriptionConcurrencyRedisForTest(t, brokenRedisEvaler{})
    _, err := AcquireUserConcurrency(context.Background(), 7504, "req", 1)
    require.ErrorIs(t, err, ErrSubscriptionConcurrencyUnavailable)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./service -run 'TestMemoryConcurrencyLimiter_AcquireRelease|TestSubscriptionConcurrencyRequiresRedisWhenConfigured|TestSubscriptionConcurrencyFailOpenWhenRedisRequired|TestSubscriptionConcurrencyFailClosedWhenRedisCommandFails' -count=1`

预期：FAIL，报错包含 `undefined: newMemorySubscriptionConcurrencyLimiter`。

- [ ] **步骤 3：实现配置**

新增：

```go
var SubscriptionConcurrencyTTLSeconds = 600
var SubscriptionConcurrencyFailOpen = false
var SubscriptionConcurrencyRequireRedis = false
```

在 `model/option.go` 初始化、更新、解析这三个选项。

- [ ] **步骤 4：实现 Redis Lua 与 fallback 租约**

创建 `AcquireUserConcurrency(ctx context.Context, userId int, requestId string, limit int)` 和 `ConcurrencyLease.Release(ctx)`。使用项目已有 `common.RDB.Eval` 原子执行 Redis acquire / release。release 必须用 `atomic.Bool` 防止重复释放。

实现决策：

- `common.RedisEnabled = true` 且 Redis 命令成功：使用 Redis Lua 精确计数。
- `common.RedisEnabled = true` 但 Redis 命令失败：默认 fail-closed，返回 `ErrSubscriptionConcurrencyUnavailable`；`SubscriptionConcurrencyFailOpen = true` 时返回 no-op lease 并记录错误日志。
- `common.RedisEnabled = false && SubscriptionConcurrencyRequireRedis = false`：使用进程内 limiter；仅保证单进程准确，启动或首次使用时记录风险日志。
- `common.RedisEnabled = false && SubscriptionConcurrencyRequireRedis = true`：默认 fail-closed，返回 `ErrSubscriptionConcurrencyUnavailable`；`SubscriptionConcurrencyFailOpen = true` 时返回 no-op lease 并记录错误日志。
- `limit <= 0`：不限制，返回 no-op lease。

说明：这不新增 Go 依赖；仓库已经依赖 go-redis。生产多实例要精确并发限制时，需要配置 `REDIS_CONN_STRING`。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w service/subscription_concurrency.go service/subscription_concurrency_test.go common/constants.go model/option.go
go test ./service -run 'TestMemoryConcurrencyLimiter_AcquireRelease|TestSubscriptionConcurrencyRequiresRedisWhenConfigured|TestSubscriptionConcurrencyFailOpenWhenRedisRequired|TestSubscriptionConcurrencyFailClosedWhenRedisCommandFails' -count=1
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

在 `controller/relay.go` 的订阅 token 预扣成功后、上游请求前调用 `service.AcquireSubscriptionConcurrency(c.Request.Context(), relayInfo)`，成功后立即 `defer lease.Release(context.Background())`。只 acquire 一次，租约覆盖整个上游请求和内部重试循环；所有早退、上游 helper 返回错误、流式响应结束和客户端断开路径都必须释放。钱包旧逻辑或非订阅请求不受订阅并发限制。

`AcquireSubscriptionConcurrency` 必须先判断 relay mode/path 是否属于规格 §8.2 的计数范围：`/v1/chat/completions`、`/v1/responses`、`/v1/responses/compact` 和同步文本生成类 relay 请求才 acquire；images、audio、embeddings、rerank、Midjourney/Suno/视频等非目标模式不得 acquire。

增加可观测 release 与过滤测试：在 `controller/relay.go` 增加测试 seam，例如把 relay helper 分发封装成包级可替换函数 / 接口，并用 `t.Cleanup` 恢复；或把 acquire / release 包成可注入 adapter。测试中注入返回错误的 relay handler 与计数 lease，模拟 limiter 成功 acquire 后让上游 helper 返回错误或提前返回，断言 release 被调用一次；覆盖错误返回、成功提前返回、内部 retry loop 只 acquire 一次并 release 一次。测试非订阅/钱包不 acquire，非目标 relay mode 不 acquire，chat/responses/responses compact acquire。

- [ ] **步骤 5：运行测试验证通过**

运行：

```bash
gofmt -w service/subscription_concurrency.go controller/relay.go model/subscription.go controller/subscription_trial_purchase_test.go
go test ./controller -run 'TestSubscriptionConcurrencyErrorToOpenAI429|TestSubscriptionConcurrencyLeaseReleasedOnRelayExit|TestSubscriptionConcurrencySkipsNonTargetRelayModes|TestSubscriptionConcurrencyAcquiresForTextRelayModes' -count=1
go test ./service -run 'TestMemoryConcurrencyLimiter_AcquireRelease|TestSubscriptionConcurrencyRequiresRedisWhenConfigured|TestSubscriptionConcurrencyFailOpenWhenRedisRequired|TestSubscriptionConcurrencyFailClosedWhenRedisCommandFails' -count=1
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
- 修改：`model/main.go`
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

- [ ] **步骤 5：接入迁移与邮箱注册表单提交**

在 `model/main.go` 的 AutoMigrate 中加入 `TrialCode`、`TrialRedemption`，并补充 SQLite smoke 测试或现有迁移测试，确认两张表能被自动创建。

`controller/user.go` 注册请求增加 `TrialCode`。邮箱注册继续沿用现有注册接口的人机校验传输方式：前端通过 `POST /api/user/register?turnstile=...` 提交 Turnstile token，后端复用现有 middleware / helper 校验；`trial_code` 放在 JSON body。输入了 trial code 时必须校验 Turnstile 或现有等价人机校验。用户创建和试用发放必须在同一事务内完成。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w model/trial_code.go model/main.go service/trial_grant.go controller/user.go model/trial_code_test.go controller/auth_github_only_test.go
go test ./model -run 'TestConsumeTrialCode|TestAutoMigrateTrialCodeTables|TestGrantTrialOnRegistration_InviteTrialWithoutTrialCode' -count=1
go test ./controller -run 'TestPasswordRegister_GrantsTrialCode|TestPasswordRegister_GrantsInviteTrialWithoutTrialCode' -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add model/trial_code.go model/main.go service/trial_grant.go controller/user.go model/trial_code_test.go controller/auth_github_only_test.go
git commit -m "feat(trial): 支持邮箱注册手动试用码"
```

## 任务 7：OAuth 建号确认页后端

**文件：**
- 创建：`controller/oauth_onboarding.go`
- 修改：`controller/github.go`
- 修改：`controller/oauth.go`
- 修改：`controller/discord.go`
- 修改：`controller/oidc.go`
- 修改：`controller/linuxdo.go`
- 修改：`controller/wechat.go`
- 修改：`model/user.go`
- 修改：`model/main.go`
- 修改：`router/api-router.go`
- 修改：`middleware/turnstile-check.go`
- 测试：`controller/auth_github_only_test.go`

- [ ] **步骤 1：编写失败测试**

测试 GitHub OAuth 首次登录未知用户时不直接创建账号，而是返回 pending onboarding 状态；提交 onboarding 时可设置密码、trial code、terms accepted、Turnstile token，并创建平台账号与 GitHub 绑定。若其它 OAuth provider 允许创建新用户，也必须返回同样的 `oauth_onboarding_required` 契约。GitHub-only signup 下非 GitHub provider 创建新用户的拒绝逻辑由任务 9 覆盖，本任务不依赖 `GitHubOnlySignupEnabled`。同时测试 pending token 只能使用一次，重复提交或两个 pending token 指向同一 provider user id 时不得创建重复平台账号。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./controller -run TestGitHubOAuthOnboarding -count=1`

预期：FAIL，当前逻辑会直接创建账号或没有 onboarding 接口。

- [ ] **步骤 3：实现 pending OAuth session 与响应契约**

OAuth 成功但没有匹配现有平台用户时，生成短期 pending token，保存 provider、provider user id、login、email、avatar、inviterId。GitHub provider 必须优先拉取 verified email：如果 `/user` 没有返回可用邮箱，应调用 GitHub `/user/emails`，选择 `verified=true` 且 `primary=true` 的邮箱；仍没有可用邮箱时，pending session 的 email 为空并由建号确认页要求用户输入。pending token 可存 session 或 Redis；多实例部署优先 Redis，复用现有 Redis 能力。pending token 必须 single-use：`CompleteOAuthOnboarding` 成功时在同一事务或同一 Redis 原子操作中 consume 并失效；重复提交同一 pending token 必须拒绝，不得再次创建用户或发放试用。

后端返回：

```json
{
  "success": true,
  "message": "oauth_onboarding_required",
  "data": {"pending_token": "...", "provider": "github", "login": "octocat", "email": "octocat@example.com"}
}
```

- [ ] **步骤 4：实现建号确认接口**

创建：

```go
func GetOAuthOnboarding(c *gin.Context)
func CompleteOAuthOnboarding(c *gin.Context)
```

接口路径：

- `GET /api/oauth/onboarding?pending_token=...`：返回 pending session 的 `pending_token`、`provider`、`login`、`email`。
- `POST /api/oauth/onboarding`：完成平台账号创建。

提交字段：

```json
{
  "pending_token": "...",
  "email": "user@example.com",
  "trial_code": "TRIAL2026",
  "password": "...",
  "terms_accepted": true,
  "turnstile_token": "..."
}
```

规则：每次完成 OAuth 新账号创建都必须校验 JSON body 中的 `turnstile_token` 或等价人机校验，不能只在填写 trial code 时校验；`terms_accepted` 必须为 true；密码可选但如果填写必须符合现有密码规则；provider 未返回可用邮箱时，`email` 必填并复用现有邮箱唯一性与验证流程；创建用户、绑定 OAuth 身份、设置密码、试用发放在同一事务内完成。确认密码由前端本地校验，不提交给后端。

新增专用 Turnstile helper（例如 `VerifyTurnstileToken(ctx, token, remoteIP)`）：OAuth onboarding POST 必须从 JSON body 读取 token 并调用该 helper；不得读取 query 参数 `turnstile`，也不得因为 session 中已有 `turnstile = true` 跳过本次校验。现有邮箱注册 middleware 可以复用该 helper 的底层 siteverify 调用，但仍保持 `POST /api/user/register?turnstile=...` 的既有传输契约。测试必须覆盖：无 JSON token 拒绝、只有 query token 拒绝、session 已有 turnstile 但 body token 缺失仍拒绝、有效 body token 通过。

创建用户前必须在事务内按 provider 类型重新检查 provider user id 未被绑定。Generic OAuth provider 使用 `model.UserOAuthBinding` 的唯一约束并在同一事务内创建绑定；内置 GitHub / Discord / OIDC / LinuxDO / WeChat 字段若当前数据库没有唯一约束，必须新增可执行的并发保护（唯一索引、事务锁或应用锁三选一，需兼容 SQLite/MySQL/PostgreSQL），防止两个 pending token 并发完成时绑定同一 provider user id。若完成阶段发现 provider 已绑定，返回明确错误并不得消费 trial code 或发放试用。

- [ ] **步骤 5：增加路由**

在 `router/api-router.go` 增加 OAuth onboarding 路由。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w controller/oauth_onboarding.go controller/github.go controller/oauth.go controller/discord.go controller/oidc.go controller/linuxdo.go controller/wechat.go middleware/turnstile-check.go router/api-router.go model/user.go model/main.go controller/auth_github_only_test.go
go test ./controller -run 'TestGitHubOAuthOnboarding|TestOAuthOnboardingUsesGitHubVerifiedPrimaryEmail|TestOAuthOnboardingRequiresEmailWhenGitHubHasNoVerifiedEmail|TestOAuthOnboardingRequiredForNewOAuthUser|TestOAuthOnboardingRequiresTurnstile|TestOAuthOnboardingRejectsQueryOnlyTurnstile|TestOAuthOnboardingIgnoresSessionTurnstileWithoutBodyToken|TestOAuthOnboardingRequiresEmailWhenProviderEmailMissing|TestOAuthOnboardingRejectsReusedPendingToken|TestOAuthOnboardingRejectsProviderAlreadyBoundDuringCompletion' -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add controller/oauth_onboarding.go controller/github.go controller/oauth.go controller/discord.go controller/oidc.go controller/linuxdo.go controller/wechat.go middleware/turnstile-check.go router/api-router.go model/user.go model/main.go controller/auth_github_only_test.go
git commit -m "feat(auth): 增加 OAuth 建号确认接口"
```

## 任务 8：月度邀请权益

**文件：**
- 创建：`model/invitation_reward.go`
- 修改：`model/main.go`
- 创建：`service/invitation_reward.go`
- 修改：`service/subscription_reset_task.go`
- 修改：`model/subscription.go`
- 修改：`controller/subscription_payment_epay.go`
- 修改：`controller/subscription_payment_stripe.go`
- 修改：`controller/subscription_payment_creem.go`
- 修改：`controller/topup_stripe.go`
- 修改：`controller/topup_creem.go`
- 修改：`controller/user.go`
- 修改：`router/api-router.go`
- 测试：`service/invitation_reward_test.go`
- 测试：`controller/invitation_entitlement_test.go`

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

- [ ] **步骤 5：接入迁移与触发点**

在 `model/main.go` 的 AutoMigrate 中加入 `InvitationMonthlyEntitlement`，并补充 SQLite smoke 测试或现有迁移测试，确认表和唯一索引存在。

支付成功后触发一次评估。不得从 `model` 包反向 import `service` 包，避免 `model` ↔ `service` import cycle。推荐方案：保留权益评估核心在 `service/invitation_reward.go`，并新增 controller 层 helper（例如 `completeSubscriptionOrderAndEvaluateInvitation`）统一包装 `model.CompleteSubscriptionOrder` + `service.EnsureMonthlyInvitationEntitlement`；所有真实订阅支付完成入口都必须改用该 helper，包括 EPay notify 和 EPay return、Stripe webhook 的 `controller/topup_stripe.go`、Creem webhook 的 `controller/topup_creem.go`。同时把 Stripe / Creem 订阅下单入口 `controller/subscription_payment_stripe.go`、`controller/subscription_payment_creem.go` 纳入同一支付边界，保证订单 provider、plan、pay product 映射与后续 webhook helper 的 provider mismatch 校验一致。helper 契约：在完成前读取并校验 `SubscriptionOrder`，只有订单存在且 provider 匹配时才完成；完成后按 `order.UserId` 查询邀请人，`inviter_id > 0` 才调用权益评估；重复 success 可再次调用幂等评估，provider mismatch / 订单不存在不得触发权益。EPay return 目前也可能完成订单，因此必须与 notify 使用同一个 helper，或明确移除 return 路径的订单完成能力。订阅重置 / 过期任务中定期调用 sweep 作为保底。因任务 10 也会修改 EPay / Stripe / Creem 订阅支付入口，任务 8 与任务 10 不得并行修改这些文件；如果分给不同子代理，先完成任务 8 的 helper 接入，再让任务 10 在同一文件上补购买保护。

同时扩展用户邀请权益数据契约：可以在 `controller/user.go` 的 `GetSelf` 响应中增加字段，或在 `router/api-router.go` 新增 `/api/user/aff/entitlement`。响应必须包含 `direct_invite_count`、`qualified_active_count`、`reward_month`、`entitled`、`entitlement_end_time`，供任务 15 前端展示。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w model/invitation_reward.go model/main.go model/subscription.go service/invitation_reward.go service/subscription_reset_task.go controller/subscription_payment_epay.go controller/subscription_payment_stripe.go controller/subscription_payment_creem.go controller/topup_stripe.go controller/topup_creem.go controller/user.go router/api-router.go service/invitation_reward_test.go controller/invitation_entitlement_test.go
go test ./service -run 'TestMonthlyInvitationEntitlement' -count=1
go test ./controller -run 'TestCompleteSubscriptionOrderTriggersInvitationEntitlement|TestEpaySubscriptionNotifyTriggersInvitationEntitlement|TestEpaySubscriptionReturnTriggersInvitationEntitlement|TestSubscriptionPaymentProviderMismatchDoesNotTriggerInvitationEntitlement|TestStripeSubscriptionWebhookTriggersInvitationEntitlement|TestCreemSubscriptionWebhookTriggersInvitationEntitlement|TestStripeCreemSubscriptionOrderProviderMatchesWebhook|TestInvitationEntitlementStatusResponse' -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add model/invitation_reward.go model/main.go model/subscription.go service/invitation_reward.go service/subscription_reset_task.go controller/subscription_payment_epay.go controller/subscription_payment_stripe.go controller/subscription_payment_creem.go controller/topup_stripe.go controller/topup_creem.go controller/user.go router/api-router.go service/invitation_reward_test.go controller/invitation_entitlement_test.go
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
- 修改：`controller/subscription_payment_epay.go`
- 修改：`controller/subscription_payment_stripe.go`
- 修改：`controller/subscription_payment_creem.go`
- 创建：`controller/trial_code.go`
- 修改：`router/api-router.go`
- 测试：`controller/subscription_trial_purchase_test.go`

- [ ] **步骤 1：编写失败测试**

测试 `GetSubscriptionPlans` 不返回 `is_trial = true` 或 `public_visible = false` 的套餐；测试 EPay、Stripe、Creem 订阅支付入口在创建订单前拒绝 `is_trial = true`、`public_visible = false` 或 `price_amount <= 0` 的套餐。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./controller -run 'TestGetSubscriptionPlans_HidesTrialPlans|TestSubscriptionEpayRejectsTrialPlan|TestSubscriptionStripeRejectsTrialPlan|TestSubscriptionCreemRejectsTrialPlan' -count=1`

预期：FAIL，响应仍包含试用套餐或支付入口仍允许创建订单。

- [ ] **步骤 3：隐藏并拒绝购买试用套餐**

用户套餐列表查询条件改为 `enabled = true AND public_visible = true AND is_trial = false`。EPay、Stripe、Creem 订阅支付入口在创建订单前拒绝 `is_trial = true`、`public_visible = false` 或 `price_amount <= 0` 的套餐。

- [ ] **步骤 4：实现试用码管理接口**

`controller/trial_code.go` 提供 `AdminListTrialCodes`、`AdminCreateTrialCode`、`AdminUpdateTrialCode`、`AdminUpdateTrialCodeStatus`、`AdminDeleteTrialCode`。创建和更新必须校验 `plan_id` 指向试用套餐。

- [ ] **步骤 5：增加路由**

在 `router/api-router.go` 增加 `/api/trial-codes/admin` 管理路由，使用 `middleware.AdminAuth()`。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w controller/subscription.go controller/subscription_payment_epay.go controller/subscription_payment_stripe.go controller/subscription_payment_creem.go controller/trial_code.go router/api-router.go controller/subscription_trial_purchase_test.go
go test ./controller -run 'TestGetSubscriptionPlans_HidesTrialPlans|TestSubscriptionEpayRejectsTrialPlan|TestSubscriptionStripeRejectsTrialPlan|TestSubscriptionCreemRejectsTrialPlan' -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

运行：

```bash
git add controller/subscription.go controller/subscription_payment_epay.go controller/subscription_payment_stripe.go controller/subscription_payment_creem.go controller/trial_code.go router/api-router.go controller/subscription_trial_purchase_test.go
git commit -m "feat(subscription): 隐藏试用套餐并增加试用码接口"
```

## 任务 11：默认分销套餐

**文件：**
- 创建：`model/subscription_seed.go`
- 修改：`main.go`
- 测试：`model/subscription_distributor_test.go`

- [ ] **步骤 1：编写失败测试**

测试 `EnsureDistributorDefaultPlans()` 创建 `trial_24h`、`basic_monthly`、`standard_monthly`、`pro_monthly`、`max_monthly`，并断言价格、`currency = "CNY"`、token、并发。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./model -run TestEnsureDistributorDefaultPlans -count=1`

预期：FAIL，报错包含 `undefined: EnsureDistributorDefaultPlans`。

- [ ] **步骤 3：实现种子函数**

创建 `model/subscription_seed.go`，按 `business_code` 查询，不存在才创建，避免覆盖管理员修改。5 个套餐值按规格写入，默认 `Currency` 必须为 `CNY`。

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

- [ ] **步骤 4：记录本任务验证点**

本子任务不运行项目级 `typecheck`、`lint` 或 `build`。完成后记录受影响文件和建议由任务 16 统一验证的点：套餐类型可解析后端新字段；管理端表单提交 payload 包含 token / 并发字段；用户端不展示旧 quota 或价格倍率配额。

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
- 修改：`web/default/src/features/auth/types.ts`
- 修改：`web/default/src/features/auth/constants.ts`
- 修改：`web/default/src/features/auth/api.ts`
- 修改：`web/default/src/features/auth/lib/oauth.ts`
- 修改：`web/default/src/features/auth/sign-up/components/sign-up-form.tsx`
- 修改：`web/default/src/features/auth/sign-in/components/user-auth-form.tsx`
- 修改：`web/default/src/routes/oauth/$provider.tsx`
- 修改：`web/default/src/routes/(auth)/oauth.tsx`
- 创建：`web/default/src/features/auth/oauth-onboarding/index.tsx`
- 创建：`web/default/src/features/auth/oauth-onboarding/components/oauth-onboarding-form.tsx`
- 创建：`web/default/src/routes/(auth)/oauth-onboarding.tsx`
- 修改：`web/default/src/routeTree.gen.ts`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 修改：`web/default/src/features/system-settings/maintenance/config.ts`
- 修改：`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx`

- [ ] **步骤 1：创建试用码管理类型和 API**

`types.ts` 定义 `trialCodeSchema` 和 `TrialCodePayload`。`api.ts` 封装管理端 CRUD 接口。

- [ ] **步骤 2：创建试用码管理页面**

按现有 `features/subscriptions` 表格、抽屉、删除对话框模式实现。列表展示 code、plan_id、enabled、max_redemptions、redeemed_count、expires_at。

- [ ] **步骤 3：邮箱注册表单增加 trial code**

`web/default/src/features/auth/constants.ts` 的 `registerFormSchema`、`web/default/src/features/auth/types.ts` 的注册 payload、`web/default/src/features/auth/api.ts` 的请求封装和 `sign-up-form.tsx` 同步增加可选 `trial_code`。用户填写 trial code 时，提交 payload 包含 `trial_code` 和 Turnstile token。

- [ ] **步骤 4：创建 OAuth 建号确认页并接入回调分发**

实现共享分发函数（例如 `handleOAuthOnboardingRequired`），供标准 OAuth 回调和微信回调共用。`web/default/src/routes/oauth/$provider.tsx`、`web/default/src/routes/(auth)/oauth.tsx` 以及 `wechatLoginByCode` 调用链收到后端 `message = "oauth_onboarding_required"` 和 `data.pending_token` 后，都导航到 `/oauth-onboarding?pending_token=...`，并传递或通过 `GET /api/oauth/onboarding` 重新拉取 provider、login、email。

页面展示 provider 用户名和邮箱；provider 未返回可用邮箱时显示邮箱输入框并按现有邮箱校验流程处理。页面提供 trial code、密码、确认密码、Turnstile、协议勾选。确认密码只做前端本地校验，不提交；协议勾选提交 `terms_accepted: true`；每次完成 OAuth 新账号创建都提交 `turnstile_token`；提交到 `POST /api/oauth/onboarding`。

- [ ] **步骤 5：接入试用码路由与侧边栏入口**

新增 trial-codes route 后必须更新并提交 `web/default/src/routeTree.gen.ts`。在 `web/default/src/hooks/use-sidebar-data.ts`、`web/default/src/hooks/use-sidebar-config.ts`、`web/default/src/features/system-settings/maintenance/config.ts`、`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx` 中加入试用码管理入口和侧边栏模块配置，确保管理员能从导航发现页面。

- [ ] **步骤 6：记录本任务验证点**

本子任务不运行项目级 `typecheck`、`lint` 或 `build`。完成后记录受影响文件和建议由任务 16 统一验证的点：邮箱注册 payload 包含 trial code；标准 OAuth 和微信 OAuth 新用户回调都进入 onboarding；onboarding 提交字段匹配后端契约；provider email 缺失时页面要求输入邮箱；试用码管理页已进入 `routeTree.gen.ts` 且侧边栏入口可发现。


- [ ] **步骤 7：Commit**

运行：

```bash
git add web/default/src/features/trial-codes web/default/src/routes/_authenticated/trial-codes/index.tsx web/default/src/features/auth/types.ts web/default/src/features/auth/constants.ts web/default/src/features/auth/api.ts web/default/src/features/auth/lib/oauth.ts web/default/src/features/auth/sign-up/components/sign-up-form.tsx web/default/src/features/auth/sign-in/components/user-auth-form.tsx web/default/src/routes/oauth/$provider.tsx web/default/src/routes/(auth)/oauth.tsx web/default/src/features/auth/oauth-onboarding web/default/src/routes/(auth)/oauth-onboarding.tsx web/default/src/routeTree.gen.ts web/default/src/hooks/use-sidebar-data.ts web/default/src/hooks/use-sidebar-config.ts web/default/src/features/system-settings/maintenance/config.ts web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx
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
- 修改：`web/default/src/features/system-settings/auth/index.tsx`
- 修改：`web/default/src/features/system-settings/auth/section-registry.tsx`
- 修改：`web/default/src/features/system-settings/types.ts`
- 修改：`web/default/src/features/system-settings/hooks/use-update-option.ts`

- [ ] **步骤 1：更新状态类型**

`SystemStatus` 增加 `github_only_signup_enabled?: boolean`，同时兼容直接字段和 `data` 内字段。

- [ ] **步骤 2：调整注册和登录页**

GitHub-only signup 为 true 时，注册页隐藏邮箱注册表单，只显示 GitHub 创建账号入口。登录页保留密码登录，因为 GitHub 创建账号可以设置密码。

- [ ] **步骤 3：过滤注册 provider**

为 `OAuthProviders` 增加显式 `mode` / `context` / `registrationOnly` 之类 prop；只有 `sign-up-form.tsx` 传注册语境并只展示 GitHub provider，`user-auth-form.tsx` 保持登录语境，不过滤已有 provider。

- [ ] **步骤 4：增加系统设置开关**

认证设置页增加「仅允许 GitHub 创建新用户」开关。开启时提示必须先配置 GitHub OAuth。同步更新 `web/default/src/features/system-settings/types.ts` 的 `AuthSettings`、`web/default/src/features/system-settings/auth/index.tsx` 的 `defaultAuthSettings` / `getOptionValue` 字段白名单、`web/default/src/features/system-settings/auth/section-registry.tsx` 的 props 传递，使 `GitHubOnlySignupEnabled` 能进入表单默认值并传给承载开关的 section。同步更新 `web/default/src/features/system-settings/hooks/use-update-option.ts` 的 `STATUS_RELATED_KEYS`，保存 `GitHubOnlySignupEnabled`、`RegisterEnabled`、`PasswordRegisterEnabled`、`EmailVerificationEnabled`、`GitHubOAuthEnabled`、`GitHubClientId`、`TurnstileCheckEnabled` 等会影响 `/api/status` 和 auth 页面渲染的 key 后必须失效并刷新 `/api/status`，避免注册页继续展示旧入口。

- [ ] **步骤 5：记录本任务验证点**

本子任务不运行项目级 `typecheck`、`lint` 或 `build`。完成后记录受影响文件和建议由任务 16 统一验证的点：注册页只展示 GitHub 创建入口；登录页保留密码登录；注册语境只展示 GitHub provider；保存设置后状态刷新。

- [ ] **步骤 6：Commit**

运行：

```bash
git add web/default/src/features/auth/types.ts web/default/src/features/auth/sign-in/components/user-auth-form.tsx web/default/src/features/auth/sign-up/components/sign-up-form.tsx web/default/src/features/auth/components/oauth-providers.tsx web/default/src/features/system-settings/auth/basic-auth-section.tsx web/default/src/features/system-settings/auth/oauth-section.tsx web/default/src/features/system-settings/auth/index.tsx web/default/src/features/system-settings/auth/section-registry.tsx web/default/src/features/system-settings/types.ts web/default/src/features/system-settings/hooks/use-update-option.ts
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
- 修改：`web/default/src/features/wallet/api.ts`
- 修改：`web/default/src/features/wallet/types.ts`
- 修改：`web/default/src/features/wallet/hooks/use-affiliate.ts`
- 修改：`web/default/src/routeTree.gen.ts`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 修改：`web/default/src/features/system-settings/maintenance/config.ts`
- 修改：`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx`
- 修改：`model/user.go`

- [ ] **步骤 1：创建配置生成工具**

`build-config.ts` 提供 `buildOpenAIBaseUrl`、`buildCherryStudioConfig` 等纯函数。

- [ ] **步骤 2：创建教程页面**

教程覆盖 Cherry Studio、Chatbox、LobeChat、NextChat、Open WebUI、Continue、Cline、Claude Code / CC Switch。每张卡包含 Base URL、API Key 选择、复制按钮和一键导入链接。

- [ ] **步骤 3：更新邀请权益卡和数据契约**

在任务 8 后端数据契约基础上，扩展 `/api/user/self` 或新增 `/api/user/aff/entitlement` 的前端 API、类型和 hook，提供 `direct_invite_count`、`qualified_active_count`、`reward_month`、`entitled`、`entitlement_end_time`。`affiliate-rewards-card.tsx` 展示直属邀请人数、当前有效付费直属下级人数、本月是否已获得 Basic 权益、权益有效期。

- [ ] **步骤 4：接入路由、routeTree 与侧边栏入口**

新增 app guides route 后必须更新并提交 `web/default/src/routeTree.gen.ts`。在 `web/default/src/hooks/use-sidebar-data.ts`、`web/default/src/hooks/use-sidebar-config.ts`、`web/default/src/features/system-settings/maintenance/config.ts`、`web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx` 中加入应用教程入口和侧边栏模块配置。同步更新后端 `model/user.go` 的 `generateDefaultSidebarConfigForRole`，把 app guides 和 trial codes 模块键加入新建管理员 / Root 用户默认 `sidebar_modules`；任务 13 已负责试用码前端入口，任务 15 负责后端默认持久化配置的最终兜底。

- [ ] **步骤 5：记录本任务验证点**

本子任务不运行项目级 `typecheck`、`lint` 或 `build`。完成后记录受影响文件和建议由任务 16 统一验证的点：教程页 Base URL 生成正确；复制按钮和一键导入链接可用；邀请权益卡从后端契约读取并展示直属邀请、有效付费直属下级、本月权益和有效期；新增路由已进入 `routeTree.gen.ts`；侧边栏或配置入口可发现试用码管理和应用教程页面；新建管理员 / Root 用户默认 `sidebar_modules` 包含 trial-codes 和 app-guides 模块键。

- [ ] **步骤 6：Commit**

运行：

```bash
git add web/default/src/features/app-guides web/default/src/routes/_authenticated/app-guides/index.tsx web/default/src/features/wallet/components/affiliate-rewards-card.tsx web/default/src/features/wallet/api.ts web/default/src/features/wallet/types.ts web/default/src/features/wallet/hooks/use-affiliate.ts web/default/src/routeTree.gen.ts web/default/src/hooks/use-sidebar-data.ts web/default/src/hooks/use-sidebar-config.ts web/default/src/features/system-settings/maintenance/config.ts web/default/src/features/system-settings/maintenance/sidebar-modules-section.tsx model/user.go
git commit -m "feat(web): 增加 AI 应用配置教程"
```

## 任务 16：最终验证

**文件：**
- 检查全部已修改文件

- [ ] **步骤 1：后端精准测试**

运行：

```bash
go test ./model -run 'Test.*Subscription|Test.*Trial|Test.*Invitation|TestCompleteSubscriptionOrder' -count=1
go test ./service -run 'Test.*SubscriptionConcurrency|Test.*Meter|Test.*Billing|Test.*TextQuota|TestTaskBillingDoesNotAdjustDistributorSubscription|TestPostAudioConsumeQuotaDoesNotConsumeDistributorSubscription' -count=1
go test ./controller -run 'Test.*GitHubOnly|Test.*Subscription|Test.*Trial|Test.*OAuthOnboarding|TestInvitationEntitlementStatusResponse|TestRelayTaskDoesNotPreConsumeDistributorSubscription' -count=1
```

预期：全部 PASS。

- [ ] **步骤 2：后端受影响包测试**

运行：

```bash
go test ./model ./service ./controller ./relay/... -count=1
```

预期：全部 PASS。

- [ ] **步骤 2.5：迁移 smoke 验证**

运行 SQLite 自动迁移测试，确认新增列、试用码表、月度邀请权益表和唯一索引可以创建。迁移 smoke 必须覆盖 `business_code` 可空唯一语义：已有多条 legacy `subscription_plans` 的 `business_code = NULL` 可迁移成功，重复非空 `business_code` 被拒绝，且没有默认空字符串。没有 MySQL / PostgreSQL 真实 DSN 时，仍需运行 GORM DryRun / 迁移 SQL 断言，确认 MySQL / PostgreSQL 方言不会生成 `business_code` 默认空字符串，并会创建非空业务标识唯一约束；如果提供 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN`，必须运行真实跨库 smoke，失败为发布阻断，不作为普通跳过。

```bash
go test ./model -run 'TestAutoMigrate.*Distributor|TestAutoMigrateTrialCodeTables|TestAutoMigrateInvitationEntitlements|TestAutoMigrateBusinessCodeNullableUnique|TestAutoMigrateBusinessCodeDryRunDialects' -count=1
```

- [ ] **步骤 3：前端验证**

新增或删除 file route 后，先通过 Rsbuild / TanStack Router plugin 生成 `web/default/src/routeTree.gen.ts`，并确认生成文件已包含 trial-codes、oauth-onboarding、app-guides 路由。若没有单独 route generation 脚本，可先运行 `bun run build` 触发生成，再运行 typecheck/lint/build；最终必须提交更新后的 `routeTree.gen.ts`。

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

- [ ] **步骤 5：最终提交**

运行：

```bash
git status --short
```

审阅输出，确认只包含本计划的实现文件、测试文件和文档文件。然后使用本计划维护的变更清单显式 `git add <files>`；不要使用 `git add .`。若任务 1-15 已逐任务提交，最终只提交验证过程中产生的修正或跳过本步骤。

预期：工作区没有无关文件；如需最终提交，提交信息为 `feat(distributor): 完成 token 分销平台定制`。
