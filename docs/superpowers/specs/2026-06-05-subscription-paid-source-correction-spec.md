# 套餐来源付费口径更正规格

## 1. 背景

当前用户钱包套餐来源和配额重置链路存在一个口径错误：部分代码把「付费套餐」等同于 `grant_reason == "order"`。这个判断遗漏了兑换码套餐。

产品口径已经更正为：

```text
除邀请奖励以外，所有有价套餐获取方式都应视为付费套餐。
```

其中，兑换码来源 `redemption` 是卡网销售后的兑换授予方式，属于有价套餐获取方式。它不是 Unknown，也不是免费赠送。

本规格用于修正钱包展示、公开套餐摘要和配额重置中的付费来源判断，使其与现有管理统计和业务事实保持一致。

## 2. 事故证据

生产用户 `1512834898` 对应数据库用户：

- `users.id = 54`
- `username = 1512834898`
- `display_name = 1512834898`
- `email = yang1wen2long3@gmail.com`

该用户的一听可乐套餐：

- `user_subscriptions.id = 167`
- `plan_id = 3`
- `title = 一听可乐`
- `business_code = medium`
- `status = active`
- `grant_reason = redemption`
- `source = redemption`
- `start_time = 2026-05-20 18:47:01 +08`
- `end_time = 2026-07-27 18:47:01 +08`
- `token_limit = 2,000,000,000`
- `token_used = 1,999,988,988`
- `last_reset_time = 2026-06-01 00:00:00 +08`
- `next_reset_time = 2026-07-01 00:00:00 +08`
- `quota_reset_period = monthly`

兑换记录确认该套餐来源为订阅兑换码：

| redemption_id | 套餐 | 名称 | 兑换时间 |
|---:|---|---|---|
| `601` | 一听可乐 | 一听可乐 5-19 | 2026-05-20 18:47:01 +08 |
| `581` | 一听可乐 | 一听可乐 5-19 | 2026-06-05 22:03:41 +08 |

同用户存在两笔 Kyren 一听可乐订单，但订单状态均为 `pending`，不是该套餐的授予来源。

生产库中 `redemption` 活跃套餐不止该用户一例。它是通用业务来源，不是脏数据。

## 3. 当前代码问题

### 3.1 后端付费来源判断过窄

`model/subscription.go` 当前实现：

```go
func isPaidSubscription(sub *UserSubscription) bool {
    if sub == nil {
        return false
    }
    return strings.TrimSpace(sub.GrantReason) == SubscriptionGrantOrder
}
```

该实现只把 `order` 视为付费套餐，导致以下链路全部漏掉 `redemption`：

- `subscriptionSourceLabel` 不会把 `redemption` 映射为 `paid`。
- `BuildPublicSubscriptionSummaries` 不会把 `redemption` 纳入 `paidRemainderByTier`。
- `canResetSubscriptionQuota` 对 `redemption` 返回 `false`。
- `ResetUserSubscriptionQuota` 对 `redemption` 直接拒绝。
- `findResetQuotaPaidSubscriptionTx` 无法用 `redemption` 作为邀请奖励套餐的同档付费扣减来源。
- 主套餐候选选择中依赖 `isPaidSubscription` 的分支也会继续保留 `order` 专属行为。

### 3.2 前端重复推导来源且漏掉 redemption

`web/default/src/features/wallet/components/subscription-plans-card.tsx` 当前实现：

```ts
if (grantReason === 'order' || (!grantReason && subscription?.source === 'order')) {
  return t('Paid plan')
}
if (grantReason === 'monthly_invite_entitlement') {
  return t('Invitation reward')
}
if (grantReason === 'trial_code' || grantReason === 'invite_trial') {
  return t('Trial')
}
return t('Unknown')
```

该逻辑没有识别 `redemption`，因此兑换码套餐显示为 Unknown。

前端还存在旧兜底：

```ts
subscription?.can_reset_quota ?? subscription?.grant_reason === 'order'
```

这会在后端缺失 `can_reset_quota` 时继续把重置能力错误绑定到 `order`。

### 3.3 现有其它模块已经支持更正口径

`model/admin_analytics_paid_subscription.go` 已经把非销售赠送限定为：

```text
monthly_invite_entitlement
invite_trial
trial_code
```

这意味着 `redemption`、`order`、`admin` 等有价来源不属于非销售赠送。

`model/trial_abuse.go` 的 `isTrialAbusePaidEntitlementSource` 已经把以下来源视为反滥用意义上的「已有权益来源」：

```text
order
redemption
admin
monthly_invite_entitlement
```

这里的 paid entitlement 是反滥用语义：它用于判断用户是否已经拥有过足以影响试用资格的权益，可以包含邀请奖励。它不等同于本规格中的「可重置 paid-equivalent」。本规格只把该模块作为 `redemption` 不是 Unknown / 免费赠送的旁证，不要求修改反滥用语义，也不得据此把邀请奖励改成 paid。

兑换码授予链路会通过 `CreateUserSubscriptionFromPlanWithResultTx(..., "redemption")` 写入来源，并记录邀请佣金事件。该链路已经体现了兑换码销售来源的业务属性。

## 4. 目标

本次更正需要达成以下目标：

1. 用户钱包中，`redemption` 来源的有价套餐不再显示 Unknown。
2. `redemption` 来源的有价套餐在公开摘要中归类为 paid。
3. `redemption` 来源的有价套餐可以按付费套餐规则重置配额。
4. 邀请奖励套餐查找同档付费剩余时间时，应能使用同档 `redemption` 有价套餐作为付费来源。
5. 钱包展示、公开摘要、配额重置和管理统计对 `redemption` paid-equivalent 的判断保持一致；反滥用模块保持自己的权益信号语义，不纳入本次统一重置口径。
6. 不修改生产数据；问题通过代码口径更正解决。

## 5. 非目标

本规格不覆盖以下内容：

- 不新增支付渠道。
- 不修改 Kyren 订单状态或补单逻辑。
- 不回填历史 `UserSubscription` 来源字段。
- 不改变邀请奖励套餐本身的非付费属性。
- 不新增复杂来源流水系统。
- 不改变兑换码创建、兑换、核销流程。
- 不改变套餐重置配额的扣减模型：仍按一次重置消耗 30 天有效期处理。

## 6. 统一来源口径

### 6.1 来源字段优先级

订阅来源判断应使用以下顺序：

1. 优先读取 `UserSubscription.grant_reason`。
2. 当 `grant_reason` 为空时，回退读取 `UserSubscription.source`。
3. 不应在前端重复实现与后端不一致的业务判断。

### 6.2 来源分类

| 原始来源 | 分类 | 说明 |
|---|---|---|
| `order` | `paid` | 在线支付、余额购买等站内订单来源。 |
| `redemption` | `paid` | 卡网销售兑换码来源，属于付费套餐。 |
| `admin` | `paid`，但必须 plan-aware | 仅当管理员授予的是有价、非试用套餐时按 paid-equivalent 处理；管理员授予试用套餐不得归入 paid。 |
| `monthly_invite_entitlement` | `invitation_reward` | 邀请奖励套餐，不是直接付费来源。 |
| `trial_code` | `trial` | 试用码来源，不是有价套餐获取方式。 |
| `invite_trial` | `trial` | 邀请试用来源，不是有价套餐获取方式。 |
| 空值或无法识别值 | 原始值或 `unknown` | 不得把明确的 `redemption` 归入 Unknown。 |

说明：

- 「除邀请奖励以外的有价套餐获取方式都是付费套餐」只作用于有价、非试用套餐权益。
- 试用来源不是有价套餐获取方式，不能因为不是邀请奖励就变成 paid。
- `admin` 不是纯 source-only paid。调用方必须结合套餐定义判断它是否是有价、非试用套餐，避免把管理员试用发放展示为付费套餐或允许重置。
- 如果未来新增有价套餐来源，应优先复用统一来源分类函数，而不是在各模块新增散落判断。

## 7. 后端规格

### 7.1 新增统一来源判断

`model/subscription.go` 应新增或调整统一来源判断函数，避免继续散落硬编码。来源归一化可以只看 `UserSubscription`，但 paid-equivalent 判断必须是 plan-aware，不能只根据来源字符串把 `admin` 无条件视为 paid。

推荐语义：

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

`isPaidSubscription` 如继续保留，应改为 plan-aware，或仅作为 `order` / `redemption` 的兼容包装使用。`BuildPublicSubscriptionSummaries`、`canResetSubscriptionQuota`、`ResetUserSubscriptionQuota`、`findResetQuotaPaidSubscriptionTx` 和主套餐候选选择不得再依赖只判断 `GrantReason == order` 的旧语义。

### 7.2 来源标签

`subscriptionSourceLabel` 应结合 `UserSubscription` 和 `SubscriptionPlan` 输出：

| 输入来源 | 输出 `source_label` |
|---|---|
| `order` | `paid` |
| `redemption` | `paid` |
| `admin` 且套餐为有价、非试用 | `paid` |
| `admin` 且套餐为试用或无价 | `trial` 或原始值，按现有试用语义保持非 paid |
| `monthly_invite_entitlement` | `invitation_reward` |
| `trial_code` | `trial` |
| `invite_trial` | `trial` |

如果来源为空且无法识别，可以保持空字符串或原始值；不得把 `redemption` 返回为原始值导致前端 Unknown。

### 7.3 公开套餐摘要

`BuildPublicSubscriptionSummaries` 中的 `paidRemainderByTier` 必须纳入同档有效的 paid-equivalent 来源：

- `order`
- `redemption`
- `admin` 授予的有价、非试用套餐

这样邀请奖励套餐计算 `effective_end_time` 和 `can_reset_quota` 时，可以使用同档兑换码有价套餐的剩余时间。管理员试用发放不得进入 `paidRemainderByTier`。

### 7.4 配额重置

`canResetSubscriptionQuota` 应符合以下规则：

| 目标订阅来源 | 条件 | `can_reset_quota` |
|---|---|---|
| `order` | 活跃且未取消 | `true` |
| `redemption` | 活跃且未取消 | `true` |
| `admin` | 活跃、未取消且套餐为有价、非试用 | `true` |
| `admin` | 套餐为试用或无价 | `false` |
| `monthly_invite_entitlement` | 同档 paid-equivalent 套餐剩余时间不少于 30 天 | `true` |
| `trial_code` / `invite_trial` | 任意 | `false` |

`ResetUserSubscriptionQuota` 应符合以下规则：

1. 当目标订阅是 paid-equivalent 来源时，目标订阅自己作为 payer。
2. 重置成功后：
   - `token_used = 0`
   - `amount_used = 0`
   - `last_reset_time = now`
   - `next_reset_time = calcNextResetTime(...)`
   - `end_time -= 30 天`
3. 当目标订阅是邀请奖励来源时，查找同用户、同档、活跃、剩余时间不少于 30 天的 paid-equivalent payer。
4. 邀请奖励重置成功后，重置奖励订阅用量，并从 payer 扣减 30 天有效期。
5. 如果没有可用 payer，继续拒绝并返回明确错误。

### 7.5 主套餐候选选择

任何依赖 `isPaidSubscription` 的主套餐候选选择逻辑都必须同步使用 paid-equivalent 口径。`redemption` 不应因为不是 `order` 而失去与邀请奖励套餐配对的行为。

## 8. 前端规格

### 8.1 来源展示

`web/default/src/features/wallet/components/subscription-plans-card.tsx` 应优先使用后端返回的 `subscription.source_label`。

推荐映射：

| `source_label` | 显示文案 |
|---|---|
| `paid` | `Paid plan` |
| `invitation_reward` | `Invitation reward` |
| `trial` | `Trial` |

兼容旧后端时，前端可以回退识别原始来源：

| 原始来源 | 显示文案 |
|---|---|
| `order` | `Paid plan` |
| `redemption` | `Paid plan` |
| `admin` 且可从套餐字段确认是有价、非试用套餐 | `Paid plan` |
| `monthly_invite_entitlement` | `Invitation reward` |
| `trial_code` | `Trial` |
| `invite_trial` | `Trial` |

前端不得把 `redemption` 显示为 Unknown。

### 8.2 重置按钮

前端重置按钮应以 `subscription.can_reset_quota` 为主判断。

旧后端兼容兜底如需保留，必须使用 paid-equivalent 口径，而不是只判断 `grant_reason === 'order'`。

推荐兜底：

```ts
const canResetQuota =
  (subscription?.can_reset_quota ?? isPaidLikeSubscription(subscription)) &&
  isActive &&
  !isCancelled
```

其中 `isPaidLikeSubscription` 至少识别：

- `order`
- `redemption`
- `admin` 且可从套餐字段确认是有价、非试用套餐

## 9. 测试规格

### 9.1 后端单元测试

应补充或更新 `model/subscription_test.go` 或现有订阅相关测试，覆盖以下场景：

1. `subscriptionSourceLabel` 对 `redemption` 返回 `paid`。
2. `BuildPublicSubscriptionSummaries` 对 `redemption` 订阅返回：
   - `source_label = paid`
   - `can_reset_quota = true`
3. 同档 `redemption` 付费套餐存在且剩余时间不少于 30 天时，邀请奖励套餐返回：
   - `can_reset_quota = true`
   - `effective_end_time` 叠加 paid remainder
4. `ResetUserSubscriptionQuota` 对 `redemption` 目标订阅执行成功，并扣减目标订阅 30 天有效期。
5. `ResetUserSubscriptionQuota` 对邀请奖励订阅执行成功时，可以扣减同档 `redemption` payer 的 30 天有效期。
6. `trial_code` 和 `invite_trial` 不因本次更正变成可重置套餐。
7. `admin` 授予的试用或无价套餐不返回 `source_label = paid`，不进入 `paidRemainderByTier`，不允许按 paid-equivalent 重置。

### 9.2 前端单元测试

应补充或更新钱包卡片来源展示和重置按钮测试，覆盖以下场景：

1. `source_label = paid` 时显示 `Paid plan`。
2. 旧后端只返回 `grant_reason = redemption` 时显示 `Paid plan`。
3. 活跃且未取消的 `redemption` 套餐在 `source_label = paid` 且 `can_reset_quota = true` 时显示 `Paid plan` 和 `Reset quota`。
4. 旧后端只返回 `grant_reason/source = redemption` 且缺少 `can_reset_quota` 时，如果保留兼容兜底，也应显示 `Reset quota`。
5. `grant_reason = monthly_invite_entitlement` 时显示 `Invitation reward`。
6. `grant_reason = trial_code` 或 `invite_trial` 时显示 `Trial`，且不显示 `Reset quota`。
7. `redemption` 套餐不显示 `Unknown`。

### 9.3 回归测试

应运行与改动直接相关的测试：

```bash
go test ./model -run 'Subscription|Reset|Redemption|Invitation'
```

如果前端已有针对钱包组件的测试脚本，应运行对应测试；如果没有，应至少运行前端类型检查或构建，确保新增来源判断不破坏类型约束。

## 10. 验收标准

更正完成后，以下条件必须全部满足：

1. 用户 `1512834898` 的一听可乐 `redemption` 套餐在前端不再显示 Unknown，而是显示付费套餐文案。
2. 该套餐在前端卡片上可见 `Reset quota` 按钮。
3. 该套餐在公开摘要中返回 `source_label = paid`。
4. 该套餐在活跃且未取消时返回 `can_reset_quota = true`。
5. 用户对该套餐执行配额重置时，后端允许重置，并消耗该套餐自身 30 天有效期。
6. 邀请奖励套餐可以使用同档兑换码有价套餐作为 paid payer。
7. 试用码和邀请试用套餐仍不能按付费套餐重置。
8. 管理员授予的试用或无价套餐仍不能按 paid-equivalent 展示、进入 paid remainder 或重置。
9. 不需要修改生产库中的 `grant_reason` 或 `source`。

## 11. 风险与约束

- `UserSubscription` 是权益快照，不是每次获取流水。一条记录可能合并多次购买、兑换或补发。本次更正只统一当前快照的来源分类，不解决历史流水精确拆分问题。
- `admin` 来源可能同时承担售后补发和人工赠送含义。现有管理统计已把 `admin` 作为 paid source 处理，但钱包与重置链路必须结合套餐字段，仅把有价、非试用的 `admin` 授予视为 paid-equivalent；如未来需要区分纯赠送，应新增更明确的来源值。
- 前端应尽量消费后端 `source_label`，避免后续新增来源时再次出现前后端口径漂移。
- 本次更正不应改变邀请奖励来源 `monthly_invite_entitlement` 的非付费属性。

## 12. 自检结论

- 规格覆盖了后端来源标签、公开摘要、配额重置、邀请奖励配对、前端来源展示和重置按钮。
- 没有要求修改生产数据。
- 没有把兑换码来源误归类为 Unknown 或免费赠送。
- 没有把试用来源或管理员试用授予错误纳入付费套餐。
- 当前范围可以由一个实现计划覆盖，不需要拆分为多个子项目。
