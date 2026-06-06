# 套餐来源付费口径更正实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 修复兑换码订阅 `redemption` 在钱包中显示 Unknown 且不能重置配额的问题，并统一后端 paid-equivalent 口径。

**架构：** 后端在 `model/subscription.go` 增加 plan-aware paid-equivalent 判断，公开摘要、配额重置、邀请奖励 paid payer 查找和主套餐候选选择都复用该判断。前端在钱包卡片中优先消费后端 `source_label`，旧后端兼容时按 `grant_reason/source` 识别 `redemption`，重置按钮以 `can_reset_quota` 为主。测试先覆盖 `redemption`、邀请奖励使用 `redemption` payer、试用 / admin 试用负例，再实现最小代码。

**技术栈：** Go 1.22+、GORM、React 19、TypeScript、Bun、Node test。

---

## 1. 规格与上下文

规格文件：`docs/superpowers/specs/2026-06-05-subscription-paid-source-correction-spec.md`。

核心业务规则：

- `redemption` 是卡网销售兑换码来源，属于 paid-equivalent。
- `order` 属于 paid-equivalent。
- `admin` 只有在套餐为有价、非试用时才属于 paid-equivalent。
- `monthly_invite_entitlement` 是邀请奖励，不是直接 paid。
- `trial_code`、`invite_trial` 仍是试用，不可重置。
- 不修改生产数据，不回填历史来源。

## 2. 文件结构

### 后端

- 修改：`model/subscription.go`
  - 增加 `normalizedSubscriptionGrantSource`。
  - 增加 `isPaidEquivalentSubscription(sub, plan)`。
  - 将 `BuildPublicSubscriptionSummaries`、`toPublicUserSubscription`、`subscriptionSourceLabel`、`canResetSubscriptionQuota`、`ResetUserSubscriptionQuota`、`findResetQuotaPaidSubscriptionTx`、`selectPrimaryBillableSubscriptionTx` 中的 paid 判断切到 plan-aware helper。
  - 保持邀请奖励和试用判断不变。
- 修改：`model/admin_ops.go`
  - 将 `buildAdminOpsSubscriptionCandidates` 中依赖 `isPaidSubscription` 的分支切到 plan-aware helper。
- 修改：`model/subscription_distributor_test.go`
  - 新增后端回归测试，覆盖 `redemption` 公开摘要、配额重置、邀请奖励使用 `redemption` payer、主套餐选择、`admin` 试用负例、过期 / 取消负例。
- 修改：`model/admin_ops_billable_test.go`
  - 新增 Admin Ops 回归测试，覆盖 `redemption` paid 与同档邀请奖励配对。

### 前端

- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
  - 导出或内部使用来源归一化 helper。
  - `getSubscriptionSourceLabel` 优先使用 `source_label`。
  - 旧后端兼容映射补齐 `redemption`。
  - `canResetQuota` 旧兜底改为 paid-like 判断，不再只看 `grant_reason === 'order'`。
- 新增：`web/default/src/features/wallet/components/subscription-plans-card.test.ts`
  - 使用 Node test 测纯 helper 行为，避免渲染整组件。

---

## 任务 1：后端 paid-equivalent 口径与测试

**文件：**

- 修改：`model/subscription.go`
- 修改：`model/admin_ops.go`
- 修改：`model/subscription_distributor_test.go`

- [ ] **步骤 1：编写后端失败测试**

在 `model/subscription_distributor_test.go` 中追加测试。测试应放在现有 `TestResetUserSubscriptionQuotaConsumesOneMonthFromPaidSubscription` 附近，复用 `truncateTables`、`seedDistributorSubscriptionPlanForTest`。

新增测试 1：`redemption` 订阅公开摘要应是 paid 且可重置。

```go
func TestPublicSubscriptionSummaryTreatsRedemptionAsPaid(t *testing.T) {
    truncateTables(t)
    code := "redemption_paid_summary"
    plan := &SubscriptionPlan{Id: 7661, Title: "Redemption Paid", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
    require.NoError(t, DB.Create(plan).Error)
    now := common.GetTimestamp()
    sub := &UserSubscription{Id: 7662, UserId: 7663, PlanId: 7661, Status: "active", TokenLimit: 100, TokenUsed: 99, StartTime: now - 86400, EndTime: now + 70*86400, GrantReason: "redemption", Source: "redemption"}

    summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: sub, Plan: plan}}, sub.Id, now)

    require.Len(t, summaries, 1)
    require.NotNil(t, summaries[0].Subscription)
    assert.Equal(t, "paid", summaries[0].Subscription.SourceLabel)
    assert.True(t, summaries[0].Subscription.CanResetQuota)
}
```

新增测试 2：邀请奖励可以使用同档 `redemption` paid payer 叠加有效期并重置。

```go
func TestInvitationRewardCanResetWithSameTierRedemptionPayer(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7664, Username: "reward_redemption_reset", Status: common.UserStatusEnabled, AffCode: "aff7664"}).Error)
    code := "reward_redemption_tier"
    plan := &SubscriptionPlan{Id: 7665, Title: "Reward Redemption Tier", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
    require.NoError(t, DB.Create(plan).Error)
    now := common.GetTimestamp()
    redemptionEnd := now + 70*86400
    rewardEnd := now + 3*86400
    require.NoError(t, DB.Create(&UserSubscription{Id: 7666, UserId: 7664, PlanId: 7665, Status: "active", TokenLimit: 100, TokenUsed: 0, StartTime: now - 86400, EndTime: redemptionEnd, GrantReason: "redemption", Source: "redemption"}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7667, UserId: 7664, PlanId: 7665, Status: "active", TokenLimit: 100, TokenUsed: 88, AmountUsed: 12, StartTime: now - 86400, EndTime: rewardEnd, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)

    summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: &UserSubscription{Id: 7666, UserId: 7664, PlanId: 7665, Status: "active", EndTime: redemptionEnd, GrantReason: "redemption", Source: "redemption"}, Plan: plan}, {Subscription: &UserSubscription{Id: 7667, UserId: 7664, PlanId: 7665, Status: "active", EndTime: rewardEnd, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}, Plan: plan}}, 7667, now)
    require.Len(t, summaries, 2)
    require.NotNil(t, summaries[1].Subscription)
    assert.True(t, summaries[1].Subscription.CanResetQuota)
    assert.Equal(t, rewardEnd+(redemptionEnd-now), summaries[1].Subscription.EffectiveEndTime)

    result, err := ResetUserSubscriptionQuota(7664, 7667)

    require.NoError(t, err)
    require.NotNil(t, result)
    var reward UserSubscription
    require.NoError(t, DB.First(&reward, 7667).Error)
    assert.Equal(t, int64(0), reward.TokenUsed)
    assert.Equal(t, int64(0), reward.AmountUsed)
    assert.InDelta(t, rewardEnd, reward.EndTime, 2)
    var payer UserSubscription
    require.NoError(t, DB.First(&payer, 7666).Error)
    assert.InDelta(t, redemptionEnd-30*86400, payer.EndTime, 2)
}
```

新增测试 3：`redemption` 目标订阅自身可重置并扣减 30 天。

```go
func TestResetUserSubscriptionQuotaConsumesOneMonthFromRedemptionSubscription(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7668, Username: "reset_redemption", Status: common.UserStatusEnabled, AffCode: "aff7668"}).Error)
    code := "reset_redemption_tier"
    require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7669, Title: "Reset Redemption", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
    now := common.GetTimestamp()
    end := now + 70*86400
    require.NoError(t, DB.Create(&UserSubscription{Id: 7670, UserId: 7668, PlanId: 7669, Status: "active", TokenLimit: 100, TokenUsed: 88, AmountUsed: 12, StartTime: now - 86400, EndTime: end, GrantReason: "redemption", Source: "redemption"}).Error)

    result, err := ResetUserSubscriptionQuota(7668, 7670)

    require.NoError(t, err)
    require.NotNil(t, result)
    var sub UserSubscription
    require.NoError(t, DB.First(&sub, 7670).Error)
    assert.Equal(t, int64(0), sub.TokenUsed)
    assert.Equal(t, int64(0), sub.AmountUsed)
    assert.InDelta(t, end-30*86400, sub.EndTime, 2)
    assert.NotZero(t, sub.LastResetTime)
}
```

新增测试 4：过期或取消的 paid-equivalent 不应暴露可重置能力。

```go
func TestPublicSubscriptionSummaryDoesNotResetInactivePaidEquivalentSubscriptions(t *testing.T) {
    truncateTables(t)
    code := "inactive_redemption_reset"
    plan := &SubscriptionPlan{Id: 7671, Title: "Inactive Redemption", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
    now := common.GetTimestamp()
    expired := &UserSubscription{Id: 7672, UserId: 7673, PlanId: 7671, Status: "active", EndTime: now - 60, GrantReason: "redemption", Source: "redemption"}
    cancelled := &UserSubscription{Id: 7674, UserId: 7673, PlanId: 7671, Status: "cancelled", EndTime: now + 70*86400, GrantReason: "redemption", Source: "redemption"}

    summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: expired, Plan: plan}, {Subscription: cancelled, Plan: plan}}, 0, now)

    require.Len(t, summaries, 2)
    assert.False(t, summaries[0].Subscription.CanResetQuota)
    assert.False(t, summaries[1].Subscription.CanResetQuota)
}
```

新增测试 5：取消或过期的同档 `redemption` 不得作为邀请奖励 paid remainder / payer。

```go
func TestInvitationRewardIgnoresInactiveRedemptionPaidRemainder(t *testing.T) {
    truncateTables(t)
    code := "inactive_redemption_payer"
    plan := &SubscriptionPlan{Id: 7675, Title: "Inactive Redemption Payer", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
    now := common.GetTimestamp()
    cancelledPaid := &UserSubscription{Id: 7676, UserId: 7677, PlanId: 7675, Status: "cancelled", EndTime: now + 70*86400, GrantReason: "redemption", Source: "redemption"}
    expiredPaid := &UserSubscription{Id: 7678, UserId: 7677, PlanId: 7675, Status: "active", EndTime: now - 60, GrantReason: "redemption", Source: "redemption"}
    reward := &UserSubscription{Id: 7679, UserId: 7677, PlanId: 7675, Status: "active", EndTime: now + 3*86400, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}

    summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: cancelledPaid, Plan: plan}, {Subscription: expiredPaid, Plan: plan}, {Subscription: reward, Plan: plan}}, reward.Id, now)

    require.Len(t, summaries, 3)
    require.NotNil(t, summaries[2].Subscription)
    assert.False(t, summaries[2].Subscription.CanResetQuota)
    assert.Equal(t, reward.EndTime, summaries[2].Subscription.EffectiveEndTime)
}
```

新增测试 6：`admin` 有价、非试用套餐应按 paid-equivalent 处理。

```go
func TestAdminPaidSubscriptionIsPaidEquivalentForReset(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7680, Username: "admin_paid_reset", Status: common.UserStatusEnabled, AffCode: "aff7680"}).Error)
    code := "admin_paid_reset_tier"
    plan := &SubscriptionPlan{Id: 7681, Title: "Admin Paid", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
    require.NoError(t, DB.Create(plan).Error)
    now := common.GetTimestamp()
    end := now + 70*86400
    sub := &UserSubscription{Id: 7682, UserId: 7680, PlanId: 7681, Status: "active", TokenLimit: 100, TokenUsed: 88, AmountUsed: 12, StartTime: now - 3600, EndTime: end, GrantReason: "admin", Source: "admin"}
    require.NoError(t, DB.Create(sub).Error)

    summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: sub, Plan: plan}}, sub.Id, now)
    require.Len(t, summaries, 1)
    require.NotNil(t, summaries[0].Subscription)
    assert.Equal(t, "paid", summaries[0].Subscription.SourceLabel)
    assert.True(t, summaries[0].Subscription.CanResetQuota)

    result, err := ResetUserSubscriptionQuota(7680, 7682)
    require.NoError(t, err)
    require.NotNil(t, result)
    var saved UserSubscription
    require.NoError(t, DB.First(&saved, 7682).Error)
    assert.Equal(t, int64(0), saved.TokenUsed)
    assert.Equal(t, int64(0), saved.AmountUsed)
    assert.InDelta(t, end-30*86400, saved.EndTime, 2)
}
```

新增测试 7：`admin` 试用或无价套餐不应按 paid-equivalent 处理。

```go
func TestAdminTrialSubscriptionIsNotPaidEquivalentForReset(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7683, Username: "admin_trial_reset", Status: common.UserStatusEnabled, AffCode: "aff7683"}).Error)
    trialCode := "admin_trial_reset_tier"
    trialPlan := &SubscriptionPlan{Id: 7684, Title: "Admin Trial", Enabled: true, IsTrial: true, PriceAmount: 0, Currency: "CNY", BusinessCode: &trialCode}
    require.NoError(t, DB.Create(trialPlan).Error)
    freeCode := "admin_free_reset_tier"
    freePlan := &SubscriptionPlan{Id: 7685, Title: "Admin Free", Enabled: true, PriceAmount: 0, Currency: "CNY", BusinessCode: &freeCode}
    require.NoError(t, DB.Create(freePlan).Error)
    now := common.GetTimestamp()
    trialSub := &UserSubscription{Id: 7686, UserId: 7683, PlanId: 7684, Status: "active", TokenLimit: 0, TokenUsed: 0, StartTime: now - 3600, EndTime: now + 24*3600, GrantReason: "admin", Source: "admin"}
    freeSub := &UserSubscription{Id: 7687, UserId: 7683, PlanId: 7685, Status: "active", TokenLimit: 100, TokenUsed: 10, StartTime: now - 3600, EndTime: now + 24*3600, GrantReason: "admin", Source: "admin"}
    require.NoError(t, DB.Create(trialSub).Error)
    require.NoError(t, DB.Create(freeSub).Error)

    summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: trialSub, Plan: trialPlan}, {Subscription: freeSub, Plan: freePlan}}, trialSub.Id, now)
    require.Len(t, summaries, 2)
    assert.NotEqual(t, "paid", summaries[0].Subscription.SourceLabel)
    assert.False(t, summaries[0].Subscription.CanResetQuota)
    assert.NotEqual(t, "paid", summaries[1].Subscription.SourceLabel)
    assert.False(t, summaries[1].Subscription.CanResetQuota)

    _, err := ResetUserSubscriptionQuota(7683, 7686)
    require.Error(t, err)
    _, err = ResetUserSubscriptionQuota(7683, 7687)
    require.Error(t, err)
}
```

新增测试 8：`redemption` paid 与同档邀请奖励共存时，主套餐选择仍应优先使用邀请奖励。

```go
func TestPreConsumeUserSubscriptionUsesSameTierRewardWhenRedemptionIsPaid(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.Create(&User{Id: 7688, Username: "same_tier_redemption_reward", Status: common.UserStatusEnabled, AffCode: "aff7688"}).Error)
    ensureSubscriptionPreConsumeRecordTableForTest(t)
    seedDistributorSubscriptionPlanForTest(t, 7689, "same_tier_redemption_reward", 100)
    now := common.GetTimestamp()
    require.NoError(t, DB.Create(&UserSubscription{Id: 7690, UserId: 7688, PlanId: 7689, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: now + 24*3600, GrantReason: "redemption", Source: "redemption"}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7691, UserId: 7688, PlanId: 7689, Status: "active", TokenLimit: 100, TokenUsed: 25, EndTime: now + 3*86400, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)

    pre, err := PreConsumeUserSubscription("same-tier-redemption-reward", 7688, "gpt-4o", 0, 6)

    require.NoError(t, err)
    assert.Equal(t, 7691, pre.UserSubscriptionId)
    var paid UserSubscription
    require.NoError(t, DB.First(&paid, 7690).Error)
    assert.Equal(t, int64(0), paid.TokenUsed)
    var reward UserSubscription
    require.NoError(t, DB.First(&reward, 7691).Error)
    assert.Equal(t, int64(31), reward.TokenUsed)
}
```

在 `model/admin_ops_billable_test.go` 中新增测试 9：Admin Ops 也应把同档 `redemption` paid 与邀请奖励配对。

```go
func TestGetAdminOpsUserConcurrencyLimitsMatchesRedemptionPaidInviteRewardSelection(t *testing.T) {
    truncateTables(t)
    ClearPrimaryBillableSubscriptionCacheForTest()
    seedAdminOpsSubscriptionPlanForBillableTest(t, 7742, "Basic", "admin_ops_redemption_reward", 100, 3, 5)
    now := common.GetTimestamp()
    require.NoError(t, DB.Create(&User{Id: 7741, Username: "admin-ops-redemption-tier", Status: common.UserStatusEnabled, AffCode: "aff7741"}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7743, UserId: 7741, PlanId: 7742, Status: "active", TokenLimit: 100, TokenUsed: 10, EndTime: now + 24*3600, GrantReason: "redemption", Source: "redemption"}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 7744, UserId: 7741, PlanId: 7742, Status: "active", TokenLimit: 100, TokenUsed: 25, EndTime: now + 3*86400, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)

    limits, err := GetAdminOpsUserConcurrencyLimits([]int{7741})

    require.NoError(t, err)
    limit := limits[7741]
    assert.Equal(t, 7742, limit.PlanID)
    assert.EqualValues(t, 100, limit.TokenLimit)
    assert.EqualValues(t, 25, limit.TokenUsed)
    assert.Equal(t, 3, limit.Limit)
    assert.Equal(t, 5, limit.QueueCapacity)
}
```

- [ ] **步骤 2：运行后端测试确认红灯**

运行：

```bash
go test ./model -run 'PublicSubscriptionSummaryTreatsRedemptionAsPaid|InvitationRewardCanResetWithSameTierRedemptionPayer|ResetUserSubscriptionQuotaConsumesOneMonthFromRedemptionSubscription|PublicSubscriptionSummaryDoesNotResetInactivePaidEquivalentSubscriptions|InvitationRewardIgnoresInactiveRedemptionPaidRemainder|AdminPaidSubscriptionIsPaidEquivalentForReset|AdminTrialSubscriptionIsNotPaidEquivalentForReset|PreConsumeUserSubscriptionUsesSameTierRewardWhenRedemptionIsPaid|GetAdminOpsUserConcurrencyLimitsMatchesRedemptionPaidInviteRewardSelection'
```

预期：`redemption` 正例、admin 有价正例、主套餐选择和 Admin Ops 配对测试失败，失败原因应是 `source_label` 不是 `paid`、`can_reset_quota` 为 false、重置接口拒绝 paid subscription，或候选选择没有把 `redemption` 当 paid。`admin` 试用 / 无价与 inactive 负例可能已经通过；如果通过，保留作为边界回归。

- [ ] **步骤 3：实现后端 paid-equivalent helper**

在 `model/subscription.go` 中替换旧 `isPaidSubscription` 附近的 helper。实现要点：

```go
func normalizedSubscriptionGrantSource(sub *UserSubscription) string {
    if sub == nil {
        return ""
    }
    if reason := strings.TrimSpace(sub.GrantReason); reason != "" {
        return reason
    }
    return strings.TrimSpace(sub.Source)
}

func isPaidEquivalentSubscription(sub *UserSubscription, plan *SubscriptionPlan) bool {
    switch normalizedSubscriptionGrantSource(sub) {
    case SubscriptionGrantOrder, "redemption":
        return true
    case "admin":
        return plan != nil && plan.PriceAmount > 0 && !plan.IsTrial && !plan.InviteTrial
    default:
        return false
    }
}
```

如果保留 `isPaidSubscription`，只能让它代理 `order/redemption` 的 source-only 兼容判断，不能用于需要判断 `admin` 的链路。推荐直接把受影响调用点改为 `isPaidEquivalentSubscription(sub, plan)`。

- [ ] **步骤 4：接入后端链路**

必须更新以下位置：

1. `buildPublicSubscriptionSummaries` 收集 `paidRemainderByTier`：

```go
if summary.Subscription == nil || summary.Plan == nil || !isActiveResettableSubscription(summary.Subscription, now) || !isPaidEquivalentSubscription(summary.Subscription, summary.Plan) {
    continue
}
```

`paidRemainderByTier` 只能收集 active、未过期的 paid-equivalent 订阅。取消、过期或非活跃订阅不得作为邀请奖励的 paid remainder / payer。
2. `toPublicUserSubscription` 的 `SourceLabel`：将 `subscriptionSourceLabel(sub)` 改为 `subscriptionSourceLabel(sub, plan)`。

3. `subscriptionSourceLabel` 签名改为：

```go
func subscriptionSourceLabel(sub *UserSubscription, plan *SubscriptionPlan) string
```

并按顺序判断：paid-equivalent → invitation reward → trial code / invite trial / admin trial → 原始来源。

4. `canResetSubscriptionQuota`：

```go
if !isActiveResettableSubscription(sub, now) {
    return false
}
if isPaidEquivalentSubscription(sub, plan) {
    return true
}
```

其中 `isActiveResettableSubscription` 必须确认 `sub.Status == "active"` 且 `sub.EndTime > now`。取消、过期或非活跃订阅不得返回 `can_reset_quota = true`。

5. `selectPrimaryBillableSubscriptionTx`：

```go
if len(candidates) > 0 && isPaidEquivalentSubscription(&candidates[0].sub, candidates[0].plan) {
    ...
}
```

6. `ResetUserSubscriptionQuota`：

```go
if !isPaidEquivalentSubscription(target, targetPlan) {
    if !isInvitationRewardSubscription(target) {
        return errors.New("subscription quota reset requires paid subscription")
    }
    payer, err = findResetQuotaPaidSubscriptionTx(...)
}
```

7. `findResetQuotaPaidSubscriptionTx`：先加载 `plan`，再判断 `isPaidEquivalentSubscription(sub, plan)`，避免 `admin` 试用误判。推荐循环顺序：取 `sub` → `plan` → paid-equivalent 判断 → tier 判断。

8. `model/admin_ops.go` 的 `buildAdminOpsSubscriptionCandidates`：

```go
if isPaidEquivalentSubscription(&defaultCandidate.sub, defaultCandidate.plan) {
    ...
}
```

- [ ] **步骤 5：运行后端测试确认绿灯**

运行：

```bash
go test ./model -run 'PublicSubscriptionSummaryTreatsRedemptionAsPaid|InvitationRewardCanResetWithSameTierRedemptionPayer|ResetUserSubscriptionQuotaConsumesOneMonthFromRedemptionSubscription|PublicSubscriptionSummaryDoesNotResetInactivePaidEquivalentSubscriptions|InvitationRewardIgnoresInactiveRedemptionPaidRemainder|AdminPaidSubscriptionIsPaidEquivalentForReset|AdminTrialSubscriptionIsNotPaidEquivalentForReset|PreConsumeUserSubscriptionUsesSameTierRewardWhenRedemptionIsPaid|GetAdminOpsUserConcurrencyLimitsMatchesRedemptionPaidInviteRewardSelection|ResetUserSubscriptionQuotaConsumesOneMonthFromPaidSubscription'
```

预期：全部 PASS。

---

## 任务 2：前端钱包来源展示、重置按钮与测试

**文件：**

- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
- 创建：`web/default/src/features/wallet/components/subscription-plans-card.test.ts`

- [ ] **步骤 1：暴露纯 helper 以便测试**

在 `subscription-plans-card.tsx` 中导出以下 helper，测试直接导入它们。保持函数纯逻辑，不依赖 React。

目标签名：

```ts
type TranslationFn = (key: string, options?: Record<string, unknown>) => string

export function getSubscriptionSourceLabel(
  record: UserSubscriptionRecord | null | undefined,
  t: TranslationFn
): string

export function canResetSubscriptionQuotaFromRecord(
  record: UserSubscriptionRecord | null | undefined,
  now: number
): boolean
```

`canResetSubscriptionQuotaFromRecord` 的 `now` 使用秒级时间戳，组件渲染处传 `Date.now() / 1000`。helper 接收完整 `UserSubscriptionRecord`，这样旧后端兼容兜底可以读取 `record.plan` 并对 `admin` 做 plan-aware 判断。

- [ ] **步骤 2：编写前端失败测试**

创建 `web/default/src/features/wallet/components/subscription-plans-card.test.ts`。

```ts
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  canResetSubscriptionQuotaFromRecord,
  getSubscriptionSourceLabel,
} from './subscription-plans-card'
import type { UserSubscriptionRecord } from '@/features/subscriptions/types'

type TranslationFn = (key: string, options?: Record<string, unknown>) => string

const t: TranslationFn = (key) => key

function makeRecord(
  subscriptionOverrides: Partial<UserSubscriptionRecord['subscription']> = {},
  planOverrides: Partial<NonNullable<UserSubscriptionRecord['plan']>> = {}
): UserSubscriptionRecord {
  return {
    subscription: {
      id: 1,
      user_id: 1,
      plan_id: 1,
      status: 'active',
      source: '',
      start_time: 0,
      end_time: 2_000,
      amount_total: 0,
      amount_used: 0,
      ...subscriptionOverrides,
    },
    plan: {
      id: 1,
      title: 'Plan',
      price_amount: 80,
      currency: 'CNY',
      duration_unit: 'month',
      duration_value: 1,
      quota_reset_period: 'monthly',
      enabled: true,
      sort_order: 1,
      max_purchase_per_user: 0,
      total_amount: 1,
      is_trial: false,
      invite_trial: false,
      ...planOverrides,
    },
  }
}

describe('wallet subscription source labels', () => {
  test('uses backend source_label before raw grant reason', () => {
    assert.equal(
      getSubscriptionSourceLabel(
        makeRecord({ source_label: 'paid', grant_reason: 'redemption' }),
        t
      ),
      'Paid plan'
    )
  })

  test('treats legacy redemption source as paid', () => {
    assert.equal(
      getSubscriptionSourceLabel(makeRecord({ grant_reason: 'redemption' }), t),
      'Paid plan'
    )
    assert.equal(
      getSubscriptionSourceLabel(makeRecord({ source: 'redemption' }), t),
      'Paid plan'
    )
  })

  test('treats legacy admin source as paid only for paid non-trial plans', () => {
    assert.equal(
      getSubscriptionSourceLabel(makeRecord({ grant_reason: 'admin' }), t),
      'Paid plan'
    )
    assert.equal(
      getSubscriptionSourceLabel(
        makeRecord({ grant_reason: 'admin' }, { price_amount: 0, is_trial: true }),
        t
      ),
      'Unknown'
    )
  })

  test('keeps invitation reward and trial labels distinct', () => {
    assert.equal(
      getSubscriptionSourceLabel(
        makeRecord({ grant_reason: 'monthly_invite_entitlement' }),
        t
      ),
      'Invitation reward'
    )
    assert.equal(
      getSubscriptionSourceLabel(makeRecord({ grant_reason: 'trial_code' }), t),
      'Trial'
    )
    assert.equal(
      getSubscriptionSourceLabel(makeRecord({ grant_reason: 'invite_trial' }), t),
      'Trial'
    )
  })
})

describe('wallet subscription quota reset visibility', () => {
  test('shows reset for active redemption when backend allows reset', () => {
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({
          grant_reason: 'redemption',
          source_label: 'paid',
          can_reset_quota: true,
          end_time: 2_000,
        }),
        1_000
      ),
      true
    )
  })

  test('legacy redemption and paid admin fallback can reset when backend flag is absent', () => {
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'redemption', end_time: 2_000 }),
        1_000
      ),
      true
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'admin', end_time: 2_000 }),
        1_000
      ),
      true
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'admin', end_time: 2_000 }, { price_amount: 0, is_trial: true }),
        1_000
      ),
      false
    )
  })

  test('does not show reset for trial or expired subscriptions', () => {
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'trial_code', end_time: 2_000 }),
        1_000
      ),
      false
    )
    assert.equal(
      canResetSubscriptionQuotaFromRecord(
        makeRecord({ grant_reason: 'redemption', end_time: 500 }),
        1_000
      ),
      false
    )
  })
})
```

- [ ] **步骤 3：运行前端测试确认红灯**

运行：

```bash
bun test src/features/wallet/components/subscription-plans-card.test.ts
```

工作目录：`web/default`。

预期：测试失败，原因是 helper 未导出或 `redemption` 仍显示 Unknown / 旧兜底不允许重置。

- [ ] **步骤 4：实现前端 helper 与组件接入**

在 `subscription-plans-card.tsx` 中实现：

```ts
function getRawSubscriptionSource(
  record: UserSubscriptionRecord | null | undefined
): string {
  const subscription = record?.subscription
  return subscription?.grant_reason?.trim() || subscription?.source?.trim() || ''
}

function isPaidLikeSubscription(
  record: UserSubscriptionRecord | null | undefined
): boolean {
  const subscription = record?.subscription
  const sourceLabel = subscription?.source_label?.trim()
  if (sourceLabel === 'paid') return true
  const source = getRawSubscriptionSource(record)
  if (source === 'order' || source === 'redemption') return true
  if (source !== 'admin') return false
  const plan = record?.plan
  return !!plan && plan.price_amount > 0 && !plan.is_trial && !plan.invite_trial
}
```

说明：前端兼容兜底必须覆盖 `redemption`，并且在完整 `record.plan` 可用时对 `admin` 做 plan-aware 判断。没有 plan 的 `admin` 不能 source-only 判为 paid；后端应通过 `source_label = paid` 和 `can_reset_quota` 提供最终判断。

`getSubscriptionSourceLabel` 逻辑：

```ts
const sourceLabel = subscription?.source_label?.trim()
if (sourceLabel === 'paid') return t('Paid plan')
if (sourceLabel === 'invitation_reward') return t('Invitation reward')
if (sourceLabel === 'trial') return t('Trial')
const source = getRawSubscriptionSource(record)
if (source === 'order' || source === 'redemption' || isPaidLikeSubscription(record)) return t('Paid plan')
if (source === 'monthly_invite_entitlement') return t('Invitation reward')
if (source === 'trial_code' || source === 'invite_trial') return t('Trial')
return t('Unknown')
```

`canResetSubscriptionQuotaFromRecord` 逻辑：

```ts
export function canResetSubscriptionQuotaFromRecord(
  record: UserSubscriptionRecord | null | undefined,
  now: number
): boolean {
  const subscription = record?.subscription
  if (!subscription) return false
  const endTime = getSubscriptionEndTime(subscription)
  const isExpired = endTime < now
  const isCancelled = subscription.status === 'cancelled'
  const isActive = subscription.status === 'active' && !isExpired
  return (
    (subscription.can_reset_quota ?? isPaidLikeSubscription(record)) &&
    isActive &&
    !isCancelled
  )
}
```

组件中把原本内联 `canResetQuota` 替换为：

```ts
const canResetQuota = canResetSubscriptionQuotaFromRecord(sub, now)
```

- [ ] **步骤 5：运行前端测试确认绿灯**

运行：

```bash
bun test src/features/wallet/components/subscription-plans-card.test.ts
```

工作目录：`web/default`。

预期：全部 PASS。

---

## 任务 3：最终验证与直接影响测试

**文件：**

- 不新增业务文件。
- 如前两任务调整测试命令，本任务只运行验证，不编辑。

- [ ] **步骤 1：运行后端直接回归测试**

运行：

```bash
go test ./model -run 'PublicSubscriptionSummaryTreatsRedemptionAsPaid|InvitationRewardCanResetWithSameTierRedemptionPayer|ResetUserSubscriptionQuotaConsumesOneMonthFromRedemptionSubscription|PublicSubscriptionSummaryDoesNotResetInactivePaidEquivalentSubscriptions|InvitationRewardIgnoresInactiveRedemptionPaidRemainder|AdminPaidSubscriptionIsPaidEquivalentForReset|AdminTrialSubscriptionIsNotPaidEquivalentForReset|PreConsumeUserSubscriptionUsesSameTierRewardWhenRedemptionIsPaid|GetAdminOpsUserConcurrencyLimitsMatchesRedemptionPaidInviteRewardSelection|ResetUserSubscriptionQuotaConsumesOneMonthFromPaidSubscription'
```

预期：全部 PASS。

- [ ] **步骤 2：运行前端直接回归测试**

运行：

```bash
bun test src/features/wallet/components/subscription-plans-card.test.ts
```

工作目录：`web/default`。

预期：全部 PASS。

- [ ] **步骤 3：运行前端类型检查**

由于改动 `tsx` / TypeScript 文件，按 `web/default/AGENTS.md` 必须运行：

```bash
bun run typecheck
```

工作目录：`web/default`。

预期：退出码 0。

## 4. 验收核对

实现完成后逐项核对：

- `redemption` 在 `source_label` 中映射为 `paid`。
- `redemption` 订阅公开摘要 `can_reset_quota = true`。
- `redemption` 目标订阅可以重置，扣减自身 30 天有效期。
- 邀请奖励可以使用同档 `redemption` 作为 paid payer。
- `trial_code`、`invite_trial`、过期 / 取消订阅、admin 试用 / 无价套餐不变成 paid-equivalent。
- 主套餐选择和 Admin Ops 都能把 `redemption` paid 与同档邀请奖励配对。
- 前端优先使用 `source_label`。
- 前端旧后端兼容时 `redemption` 显示 `Paid plan`，有价非试用 `admin` 在 plan 可用时显示 `Paid plan`。
- 不修改生产数据。
