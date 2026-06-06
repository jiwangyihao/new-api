# 邀请返佣模式规格

> 面向 AI 代理的工作者：本规格用于在现有邀请奖励体系上新增「邀请返佣」模式。实现前必须读取仓库根目录 `AGENTS.md`；如修改 `web/default`，还必须读取 `web/default/AGENTS.md`。实现必须遵守 Go + Gin、GORM、SQLite/MySQL/PostgreSQL 兼容、React 19、TypeScript、TanStack Query、Base UI/Tailwind、i18next、Bun 以及项目受保护标识约束。

**目标：** 在不改变现有「奖励套餐」计算口径的前提下，为少数特邀用户提供由管理员手动开启的「邀请返佣」模式，并支持用户自助划转到账户余额、用户申请私聊转账返现、管理员人工处理返现申请。

**架构：** 邀请关系仍使用现有 `users.inviter_id`。奖励模式是邀请人维度的用户级配置，默认 `subscription`；当前为 `subscription` 时，奖励套餐继续使用现有代码基于直属被邀请人的 active `user_subscriptions` 与 `subscription_plans.reward_eligible` fresh 计算，不改为事件表口径。只有管理员在用户列表中将特邀用户切到 `commission` 后，该用户作为邀请人的销售来源进入返佣链路；返佣来源事实、返佣余额、返佣流水和返现申请使用独立表保存，不复用 `TopUp`，不把私聊转账返现和系统内余额划转混成同一审核流程。

**技术栈：** Go 1.22+、Gin、GORM v2、SQLite/MySQL/PostgreSQL、React 19、TypeScript、TanStack Query/Axios、Base UI/Tailwind、i18next、Bun。

---

## 1. 背景

当前项目已经有两条邀请奖励链路：

1. **老邀请余额奖励**
   - 注册或三方登录时通过 `aff_code` 查找邀请人，并写入 `users.inviter_id`。
   - 新用户、被邀请人、邀请人分别使用 `QuotaForNewUser`、`QuotaForInvitee`、`QuotaForInviter` 发放余额或邀请余额。
   - 邀请人的旧奖励写入 `users.aff_quota` / `users.aff_history`。
   - 用户可通过 `POST /api/user/aff_transfer` 把 `aff_quota` 即时划转到 `users.quota`。

2. **新邀请奖励套餐**
   - 订阅支付完成后调用 `service.TryEnsureInvitationEntitlementForPaidUser`。
   - 系统按直属合格付费邀请数计算 `invitation_monthly_entitlements`。
   - 达到阈值后为邀请人 upsert `grant_reason = monthly_invite_entitlement` 的 `user_subscriptions`。
   - 当前奖励套餐由合格邀请订阅 tier 自动择优，不是管理员指定用户模式。

新增业务能力：

- 所有用户默认继续使用「奖励套餐」。
- 「邀请返佣」只开放给少数特邀用户。
- 是否使用返佣模式只能由管理员在用户列表中设置。
- 用户可选择：
  - **划转到余额**：系统内即时完成，不需要管理员审核；
  - **申请返现**：进入待处理状态，由管理员私聊用户并线下/手动转账，完成后管理员在后台标记返现完成。

---

## 2. 当前基线

### 2.1 邀请关系建立

已确认入口：

- `controller/user.go`
- `controller/oauth.go`
- `controller/github.go`
- `controller/linuxdo.go`
- `controller/oauth_onboarding.go`

关键字段：

| 字段 | 当前用途 |
|---|---|
| `users.aff_code` | 用户自己的邀请码 |
| `users.inviter_id` | 当前用户的邀请人 ID |
| `users.aff_quota` | 旧邀请可划转余额 |
| `users.aff_history` | 旧邀请历史余额 |
| `users.quota` | 用户账户余额，当前项目按 CNY 分语义使用 |

### 2.2 奖励套餐链路

关键文件：

- `controller/subscription_payment_completion.go`
- `service/invitation_reward.go`
- `model/invitation_reward.go`
- `model/subscription.go`

当前逻辑：

```text
订阅订单支付完成
  -> TryEnsureInvitationEntitlementForPaidUser(paid_user_id)
  -> 根据 paid_user.inviter_id 查邀请人
  -> 统计邀请人的合格直属付费用户
  -> 写 invitation_monthly_entitlements
  -> 写/更新 monthly_invite_entitlement 类型 user_subscriptions
```

### 2.3 用户列表和钱包前端

关键文件：

- `web/default/src/features/users/components/users-columns.tsx`
- `web/default/src/features/users/components/users-mutate-drawer.tsx`
- `web/default/src/features/wallet/components/affiliate-rewards-card.tsx`
- `web/default/src/features/wallet/hooks/use-affiliate.ts`
- `web/default/src/features/wallet/components/dialogs/transfer-dialog.tsx`

当前能力：

- 用户列表展示邀请统计和邀请奖励套餐状态。
- 用户编辑抽屉不支持奖励模式字段。
- 钱包面板支持旧邀请余额划转到账户余额。
- 钱包面板没有返佣余额、返佣流水、返现申请、审核状态。

### 2.4 管理员待办和提现现状

当前项目没有可复用的站内管理员待办系统，也没有提现或返现审核模型。

- `service/user_notify.go` 是站外通知能力，不是站内审核事项。
- `AdminOps` 是运行健康和错误快照面板，不是业务待办系统。
- `TopUp` 是充值入账订单，管理员补单语义是给用户加余额；提现/返现是反向资金流，不应复用 `TopUp`。

---

## 3. 决策

1. **所有用户默认奖励模式为 `subscription`。** 新字段必须有数据库默认值；历史用户迁移后也按奖励套餐处理。
2. **返佣模式只用于少数特邀用户。** 用户不能自助申请、不能自助切换；注册、钱包、个人设置都不得暴露模式选择入口。
3. **奖励模式是邀请人当前配置，不是事件归属快照。** 只有管理员在用户列表中手动切换 `subscription` / `commission`；系统不得把某次来源事件永久标记成只能走某一种奖励。
4. **默认行为必须完全兼容现有用户。** 未被管理员切换的用户继续按当前代码中的奖励套餐口径 fresh 计算；不得把奖励套餐改成依赖新增返佣事件表。
5. **删除返佣与奖励套餐的事件级二选一规则。** 新增销售来源事件只服务返佣入账和补偿，不作为奖励套餐资格的来源表；切换模式不删除历史返佣账户、流水、返现申请或现有奖励套餐权益。
6. **注册级旧邀请奖励本规格不主动改动。** `QuotaForNewUser`、`QuotaForInvitee`、`QuotaForInviter` 是历史注册奖励配置；本规格只调整销售来源形成的邀请返佣，并在用户处于 `commission` 时避免继续发放奖励套餐。
7. **返佣资金使用独立表。** 不复用 `users.aff_quota`，不复用 `TopUp`，避免审计语义混乱。
8. **金额统一使用整数 CNY 分。** 所有返佣余额、销售金额快照、提现金额都使用 `int64` 分值，不使用浮点数保存资金。
9. **返佣来源必须覆盖主要销售方式。** 支付渠道购买只是销售来源之一；订阅兑换码是另一条主要销售链路，也必须纳入邀请返佣。奖励套餐是否计入兑换码兑换出的订阅，仍由当前 active 订阅口径自然决定。
10. **套餐自身的 `reward_eligible` 是总开关。** 当前奖励套餐代码已经使用该字段；返佣侧也必须尊重它。只要对应 `SubscriptionPlan.reward_eligible = false`，就不得计入奖励套餐，也不得产生可用返佣。
11. **划转到账户余额即时完成。** 用户自助把返佣可用余额转入 `users.quota`，不进入审核，不产生管理员待办。
12. **私聊转账返现需要管理员人工处理。** 用户申请后冻结返佣可用余额，管理员私聊用户、线下手动转账，再在后台标记完成。
13. **管理员完成返现不增加账户余额。** 完成动作只表示线下返现已处理；系统仅把 `pending_cents` 转为 `withdrawn_cents`。
14. **拒绝返现必须退回可用返佣余额。** 管理员拒绝后，冻结金额从 `pending_cents` 回到 `available_cents`。
15. **站内待办先做轻量摘要。** 初期只需要展示待处理返现申请数量和入口，不引入通用工作流引擎。
16. **返佣比例初期使用全局配置。** 用户当前需求只要求用户级模式切换，不要求每个特邀用户有不同返佣比例。

---

## 4. 业务范围

### 4.1 必须满足

- 所有用户默认使用「奖励套餐」。
- 管理员可以在用户列表中将某个用户的邀请奖励模式设置为：
  - `subscription`：奖励套餐；
  - `commission`：邀请返佣。
- 只有管理员可以修改奖励模式。
- 用户当前为 `subscription` 时，奖励套餐按当前代码中的直属 active 订阅口径 fresh 计算，不读取新增返佣事件表。
- 用户当前为 `commission` 时，其直属被邀请人通过支付订单、账户余额购买或订阅兑换码获得 `reward_eligible = true` 的可结算套餐后生成返佣。
- 奖励套餐和返佣都必须尊重 `SubscriptionPlan.reward_eligible`；关闭该开关的套餐不计入任何邀请奖励或返佣。
- 用户侧钱包在返佣模式下展示返佣账户信息；存在历史返佣账户时，即使当前切回奖励套餐也允许处理历史余额。
- 用户可以把可用返佣余额即时划转到账户余额。
- 用户可以提交私聊转账返现申请。
- 提交返现申请后，对应金额从可用返佣余额冻结为待处理返现金额。
- 管理员后台可以查看待处理返现申请。
- 管理员后台有待处理事项提示，至少显示待处理返现申请数量。
- 管理员可以标记返现完成。
- 管理员可以拒绝返现申请，拒绝后金额退回用户可用返佣余额。
- 所有资金变化必须有可审计流水或申请记录。
- 返佣入账必须幂等，支付回调或兑换码重复处理不能重复发放。
- SQLite、MySQL、PostgreSQL 都必须兼容。
- 新增前端用户可见文案必须补齐 `en`、`zh`、`fr`、`ja`、`ru`、`vi` locale。

### 4.2 非目标

- 不开放用户自助选择返佣模式。
- 不开放用户申请成为返佣特邀用户。
- 不做自动打款、支付渠道出款、企业付款 API。
- 不把返现申请接入站外通知作为唯一提醒方式。
- 不复用充值订单 `TopUp` 表做提现或返现。
- 不把 `users.aff_quota` 改造成返佣账户。
- 不设计多级分销或间接邀请返佣。
- 不设计用户级返佣比例。
- 不处理退款后的自动返佣冲正；但数据模型必须为后续冲正留出口。
- 不重构全部邀请系统或删除旧邀请余额奖励。
- 不修改受保护项目标识、品牌、版权或归属信息。

---

## 5. 术语

| 术语 | 说明 |
|---|---|
| 邀请人 | `users.inviter_id` 指向的上级用户 |
| 被邀请人 | 通过邀请人邀请码注册或绑定邀请关系的用户 |
| 奖励套餐 | 当前 `monthly_invite_entitlement` 订阅权益奖励 |
| 返佣模式 | 特邀用户专属模式，被邀请人付费后生成返佣余额 |
| 返佣余额 | 返佣账户中的可用金额，可划转余额或申请返现 |
| 划转到余额 | 系统内把返佣余额转入 `users.quota`，即时完成 |
| 私聊转账返现 | 用户提交申请，管理员线下沟通并手动转账 |
| 待处理事项 | 管理后台展示的 `pending` 返现申请数量和入口 |

---

## 6. 数据模型设计

### 6.1 用户奖励模式字段

修改：`model/user.go`

在 `User` 模型增加：

```go
InvitationRewardMode string `json:"invitation_reward_mode" gorm:"type:varchar(32);default:'subscription'"`
```

新增常量建议放在 `model/user.go` 或新的邀请返佣模型文件中：

```go
const (
    InvitationRewardModeSubscription = "subscription"
    InvitationRewardModeCommission   = "commission"
)
```

约束：

- 空值、非法值读取时必须视为 `subscription`。
- 管理员更新时只能写入 `subscription` 或 `commission`。
- API 响应需要返回该字段，方便用户列表和钱包判断展示。
- 数据库迁移后历史用户必须等价于 `subscription`。

### 6.2 销售来源金额快照字段

修改：`model/subscription.go`

在销售来源上保存不可变金额快照，用于返佣金额证明和审计。支付订单使用 `SubscriptionOrder`，订阅兑换码使用 `Redemption`：

```go
// SubscriptionOrder
AmountCents int64  `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
Currency    string `json:"currency" gorm:"type:varchar(8);not null;default:''"`

// Redemption，仅订阅兑换码使用；钱包兑换码继续使用 Quota 作为到账余额分值
AmountCents int64  `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
Currency    string `json:"currency" gorm:"type:varchar(8);not null;default:''"`
```

规则：

- 所有订阅下单入口（账户余额、Epay、Stripe、Creem、Kyren 等）创建 `SubscriptionOrder` 时必须写入订单级 `amount_cents` 和 `currency` 快照。
- 创建或更新订阅兑换码时必须写入兑换码级 `amount_cents` 和 `currency` 快照，来源为当时绑定的 `SubscriptionPlan.price_amount/currency`，并转换为整数最小单位。
- 快照一旦创建成功，不得因为管理员后来修改套餐价格或币种而变化；更新未使用的订阅兑换码并更换套餐时才允许刷新该兑换码快照。
- Kyren 可继续使用已验签/已匹配的快照校验远端金额币种，但校验后的 CNY 分必须写入订单快照。
- 返佣只读取来源事件上的 `source_amount_cents` / `source_currency`；不得使用当前 `subscription_plans.currency` 或当前套餐价格补判已落库来源事件。
- 迁移前或异常来源缺少快照时，返佣必须记录 `invalid_source_amount` 或 `unsupported_currency` 并跳过可用入账。

销售快照来源矩阵：

| 销售来源 | `amount_cents` / `currency` 来源 |
|---|---|
| 账户余额购买 | 写入实际扣减的 CNY 分，`currency = "CNY"`。 |
| Epay | 写入创建支付单时提交给 Epay 的 CNY 分；回调完成前必须解析并校验回调金额与订单 `amount_cents/currency` 一致，只能使用订单快照完成订阅和返佣来源事件。 |
| Kyren | 写入已验签或已匹配的 Kyren 快照中的 CNY 分；若远端金额/币种不能证明为 CNY，则写 `amount_cents = 0`、`currency = ""`，返佣跳过。 |
| Creem | 写入创建 checkout 时提交并可在回调中核对的金额/币种；只有 `currency = "CNY"` 时进入可用返佣。无法证明时写 `amount_cents = 0`、`currency = ""`。 |
| Stripe | 必须读取并校验 Price 或 Checkout Session 的 amount/currency；如果无法证明远端金额和币种，则写 `amount_cents = 0`、`currency = ""`，返佣跳过。 |
| 订阅兑换码 | 创建 / 更新兑换码时从绑定套餐价格生成快照；使用兑换码时将该快照复制到邀请来源事件。 |

`amount_cents` 表示 `currency` 对应的最小货币单位。只有 `currency = "CNY"` 时，`amount_cents` 才能作为 CNY 分进入返佣公式；非 CNY 不转换。

无法证明金额或币种时，不得把 `unsupported` / `invalid` 等状态写入 `currency`。跳过原因写入 `InvitationRewardEvent.Reason` 或 `InvitationCommissionRecord.Reason`。



### 6.3 返佣账户表

创建：`model/invitation_commission.go`

```go
type InvitationCommissionAccount struct {
    Id               int   `json:"id"`
    UserId           int   `json:"user_id" gorm:"type:int;not null;uniqueIndex"`
    AvailableCents   int64 `json:"available_cents" gorm:"type:bigint;not null;default:0"`
    PendingCents     int64 `json:"pending_cents" gorm:"type:bigint;not null;default:0"`
    WithdrawnCents   int64 `json:"withdrawn_cents" gorm:"type:bigint;not null;default:0"`
    TransferredCents int64 `json:"transferred_cents" gorm:"type:bigint;not null;default:0"`
    CreatedAt        int64 `json:"created_at" gorm:"type:bigint;not null;default:0"`
    UpdatedAt        int64 `json:"updated_at" gorm:"type:bigint;not null;default:0"`
}
```

字段语义：

| 字段 | 说明 |
|---|---|
| `available_cents` | 可划转或可申请返现的返佣余额 |
| `pending_cents` | 已提交返现申请、等待管理员处理的冻结金额 |
| `withdrawn_cents` | 已通过私聊转账返现完成的累计金额 |
| `transferred_cents` | 已自助划转到账户余额的累计金额 |

### 6.4 邀请销售来源事件表

创建：`model/invitation_commission.go`

这张表记录「直属被邀请人通过可审计销售来源获得一段套餐权益」这一返佣来源事实，用于返佣入账、幂等和补偿。它不保存、冻结或推导邀请人的奖励模式，也不作为奖励套餐计算来源；奖励套餐继续使用当前代码中基于 active `user_subscriptions` 的口径。

```go
type InvitationRewardEvent struct {
    Id                   int    `json:"id"`
    InviterId            int    `json:"inviter_id" gorm:"type:int;not null;index"`
    InviteeId            int    `json:"invitee_id" gorm:"type:int;not null;index"`
    SourceType           string `json:"source_type" gorm:"type:varchar(64);not null;index:idx_invitation_reward_event_source,unique"`
    SourceId             int    `json:"source_id" gorm:"type:int;not null;index:idx_invitation_reward_event_source,unique"`
    SourceOrderId        int    `json:"source_order_id" gorm:"type:int;not null;default:0;index"`
    SourceRedemptionId   int    `json:"source_redemption_id" gorm:"type:int;not null;default:0;index"`
    SourceSubscriptionId int    `json:"source_subscription_id" gorm:"type:int;not null;default:0;index"`
    SourceAmountCents    int64  `json:"source_amount_cents" gorm:"type:bigint;not null;default:0"`
    SourceCurrency       string `json:"source_currency" gorm:"type:varchar(8);not null;default:''"`
    EventStartTime       int64  `json:"event_start_time" gorm:"type:bigint;not null;default:0;index"`
    EventEndTime         int64  `json:"event_end_time" gorm:"type:bigint;not null;default:0;index"`
    Status               string `json:"status" gorm:"type:varchar(32);not null;default:'active';index"`
    Reason               string `json:"reason" gorm:"type:varchar(255);not null;default:''"`
    CreatedAt            int64  `json:"created_at" gorm:"type:bigint;not null;default:0"`
}
```

常量：

```go
const (
    InvitationRewardEventSourceSubscriptionOrder = "subscription_order"
    InvitationRewardEventSourceSubscriptionRedemption = "subscription_redemption"
    InvitationRewardEventSourceLegacySubscription = "legacy_user_subscription"

    InvitationRewardEventStatusActive    = "active"
    InvitationRewardEventStatusCancelled = "cancelled"
)
```

规则：

- 支付订单或账户余额购买订阅成功完成后，若付费用户存在邀请人，必须创建 1 条 `InvitationRewardEvent`，记录返佣销售来源事实；`reward_eligible` 不是事件创建门槛。
- 订阅兑换码成功兑换后，若兑换用户存在邀请人，必须创建 1 条 `InvitationRewardEvent`，`source_type = subscription_redemption`、`source_id = redemptions.id`、`source_redemption_id = redemptions.id`。
- `source_type + source_id` 必须唯一。正常新订单使用 `source_type = subscription_order`、`source_id = subscription_orders.id`；订阅兑换码使用 `source_type = subscription_redemption`、`source_id = redemptions.id`；历史返佣补偿回填使用 `source_type = legacy_user_subscription`、`source_id = user_subscriptions.id`。
- `event_start_time` / `event_end_time` 表示这次销售来源贡献的有效时间段。新来源事件必须记录该来源新增的权益区间；如果延长已有订阅，`event_start_time` 为延长前的 `end_time`，`event_end_time` 为延长后的 `end_time`，不得记录更新后订阅的完整区间造成重复覆盖。
- 奖励套餐 fresh 计算保持当前代码口径，不从 `InvitationRewardEvent` 读取候选；它仍以 `users.inviter_id` 找直属被邀请人，关联 active `user_subscriptions` 和 `subscription_plans`，要求订阅当前有效、套餐 `reward_eligible = true`、非试用、非 `monthly_invite_entitlement`，并按 `invitee_id` 去重；候选 tier 和 end_time 仍沿用现有 `service/invitation_reward.go` 中的比较和择优逻辑。
- 返佣 fresh 计算从 `InvitationRewardEvent` 读取可审计销售来源；是否执行返佣只取决于邀请人当前 `invitation_reward_mode = commission`、来源套餐当前 `reward_eligible = true`、全局返佣配置、来源金额快照是否可结算以及事件是否已存在返佣记录，不取决于事件发生时的历史模式。
- 事件创建必须与订单完成或兑换码兑换、订阅创建/续费、邀请关系读取共享同一个事务性上下文。`CompleteSubscriptionOrderTx`、`Redeem` 或等价函数必须返回或持久化本次真实状态迁移信息：`Transitioned`、`SourceSubscriptionId`、`EventStartTime`、`EventEndTime`、`InviterId`。
- 订单完成和订阅兑换码兑换必须在授予权益前使用跨库有效的条件状态更新原子 claim 来源：订单使用 `WHERE id = ? AND status = pending`，兑换码使用 `WHERE id = ? AND status = enabled`，并检查 `RowsAffected`。claim 失败只能重读已完成结果或返回已使用错误，不得再次创建 / 延长 `user_subscriptions`。
- 只有本次销售来源真实首次生效时才创建新事件；重复支付回调、重复兑换请求或补偿任务只能命中已有事件 / 返佣记录。
- 如果返佣账户入账需要异步处理，也必须先在同一事务内落 `InvitationRewardEvent` 或等价 outbox 记录，确保来源事实和新增权益区间不会丢失。

### 6.5 返佣流水表

创建：`model/invitation_commission.go`

```go
type InvitationCommissionRecord struct {
    Id                int    `json:"id"`
    EventId           int    `json:"event_id" gorm:"type:int;not null;default:0;index"`
    InviterId         int    `json:"inviter_id" gorm:"type:int;not null;index"`
    InviteeId         int    `json:"invitee_id" gorm:"type:int;not null;index"`
    SourceType        string `json:"source_type" gorm:"type:varchar(64);not null;index:idx_invitation_commission_source,unique"`
    SourceId          int    `json:"source_id" gorm:"type:int;not null;index:idx_invitation_commission_source,unique"`
    SourceTradeNo     string `json:"source_trade_no" gorm:"type:varchar(128);not null;default:''"`
    SourceAmountCents int64  `json:"source_amount_cents" gorm:"type:bigint;not null;default:0"`
    SourceCurrency    string `json:"source_currency" gorm:"type:varchar(8);not null;default:''"`
    CommissionRateBps int    `json:"commission_rate_bps" gorm:"type:int;not null;default:0"`
    CommissionCents   int64  `json:"commission_cents" gorm:"type:bigint;not null;default:0"`
    Status            string `json:"status" gorm:"type:varchar(32);not null;default:'available';index"`
    Reason            string `json:"reason" gorm:"type:varchar(255);not null;default:''"`
    CreatedAt         int64  `json:"created_at" gorm:"type:bigint;not null;default:0"`
    AvailableAt       int64  `json:"available_at" gorm:"type:bigint;not null;default:0"`
    CancelledAt       int64  `json:"cancelled_at" gorm:"type:bigint;not null;default:0"`
}
```

常量：

```go
const (
    InvitationCommissionSourceSubscriptionOrder = InvitationRewardEventSourceSubscriptionOrder
    InvitationCommissionSourceSubscriptionRedemption = InvitationRewardEventSourceSubscriptionRedemption
    InvitationCommissionSourceLegacySubscription = InvitationRewardEventSourceLegacySubscription

    InvitationCommissionStatusAvailable = "available"
    InvitationCommissionStatusSkipped   = "skipped"
    InvitationCommissionStatusCancelled = "cancelled"
)

const (
    InvitationCommissionReasonUnsupportedCurrency = "unsupported_currency"
    InvitationCommissionReasonInvalidSourceAmount = "invalid_source_amount"
    InvitationCommissionReasonCommissionOverflow = "commission_overflow"
)
```

第一版对可结算 CNY 销售来源直接入账为 available。非 CNY、金额缺失等来源自身不可结算的场景可以写 skipped 并记录 reason；邀请人当前不是 `commission`、`reward_eligible = false`、全局返佣关闭、`rate_bps <= 0` 都属于运行时过滤，不得创建会消耗 `(source_type, source_id)` 唯一键的 skipped 返佣记录。

幂等约束：

```text
unique(source_type, source_id)
```

同一销售来源事件只能生成 1 条返佣记录。

### 6.6 返佣账户流水表


为审计「返佣入账、划转到账户余额、申请返现冻结、拒绝退回、返现完成」等余额变化，建议新增账户流水表，而不是只依赖返佣记录和返现申请推导。

创建：`model/invitation_commission.go`

```go
type InvitationCommissionLedger struct {
    Id                  int    `json:"id"`
    UserId              int    `json:"user_id" gorm:"type:int;not null;index"`
    Type                string `json:"type" gorm:"type:varchar(64);not null;index"`
    AmountCents         int64  `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
    AvailableAfterCents int64  `json:"available_after_cents" gorm:"type:bigint;not null;default:0"`
    PendingAfterCents   int64  `json:"pending_after_cents" gorm:"type:bigint;not null;default:0"`
    ReferenceType       string `json:"reference_type" gorm:"type:varchar(64);not null;default:'';index"`
    ReferenceId         int    `json:"reference_id" gorm:"type:int;not null;default:0;index"`
    CreatedAt           int64  `json:"created_at" gorm:"type:bigint;not null;default:0"`
}
```

流水类型：

```go
const (
    InvitationCommissionLedgerEarned              = "earned"
    InvitationCommissionLedgerTransferred         = "transferred_to_balance"
    InvitationCommissionLedgerWithdrawalCreated   = "withdrawal_created"
    InvitationCommissionLedgerWithdrawalRejected  = "withdrawal_rejected"
    InvitationCommissionLedgerWithdrawalCompleted = "withdrawal_completed"
)
```

流水约定：

- `amount_cents` 始终为正数，资金方向由 `type` 决定。
- `earned` 引用 `reference_type = commission_record`、`reference_id = invitation_commission_records.id`。
- `withdrawal_created`、`withdrawal_rejected`、`withdrawal_completed` 引用 `reference_type = withdrawal`、`reference_id = invitation_commission_withdrawals.id`。
- `transferred_to_balance` 引用 `reference_type = transfer_to_balance`，`reference_id` 可为 0 或对应扩展出的转账记录 ID。
- ledger 写入后不得更新或删除；`available_after_cents` / `pending_after_cents` 必须等于同一事务提交后的账户值。

### 6.7 返现申请表

创建：`model/invitation_commission.go`

```go
type InvitationCommissionWithdrawal struct {
    Id              int    `json:"id"`
    UserId          int    `json:"user_id" gorm:"type:int;not null;index"`
    AmountCents     int64  `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
    Status          string `json:"status" gorm:"type:varchar(32);not null;default:'pending';index"`
    Method          string `json:"method" gorm:"type:varchar(32);not null;default:'manual'"`
    ContactSnapshot string `json:"contact_snapshot" gorm:"type:text"`
    UserRemark      string `json:"user_remark" gorm:"type:text"`
    AdminRemark     string `json:"admin_remark" gorm:"type:text"`
    ReviewerId      int    `json:"reviewer_id" gorm:"type:int;not null;default:0"`
    ReviewedAt      int64  `json:"reviewed_at" gorm:"type:bigint;not null;default:0"`
    CompletedBy     int    `json:"completed_by" gorm:"type:int;not null;default:0"`
    CompletedAt     int64  `json:"completed_at" gorm:"type:bigint;not null;default:0"`
    CreatedAt       int64  `json:"created_at" gorm:"type:bigint;not null;default:0"`
    UpdatedAt       int64  `json:"updated_at" gorm:"type:bigint;not null;default:0"`
}
```

状态：

```go
const (
    InvitationCommissionWithdrawalPending   = "pending"
    InvitationCommissionWithdrawalCompleted = "completed"
    InvitationCommissionWithdrawalRejected  = "rejected"
)
```

第一版不实现用户取消申请，避免冻结资金退回路径和管理员待办统计出现歧义。

`Method` 第一版固定：

```go
const InvitationCommissionWithdrawalMethodManual = "manual"
```

`ContactSnapshot` 使用 JSON 字符串保存用户提交的联系方式快照。业务代码必须使用 `common.Marshal` / `common.Unmarshal`，不得直接调用 `encoding/json`。API 响应对前端返回解析后的 `contact` 对象，不直接暴露原始 JSON 字符串。

---

## 7. 后端业务设计

### 7.1 奖励模式与销售来源处理

修改：`controller/subscription_payment_completion.go`、`controller/redemption.go`、`model/subscription.go`、`model/redemption.go`、`service/invitation_reward.go`，并新增返佣服务文件。

所有销售型套餐来源都必须在首次真实生效的事务内落 `InvitationRewardEvent`，用于返佣入账、幂等和补偿；奖励套餐仍保持现有 direct subscription 口径，不改为事件表驱动。

销售来源事件创建入口只包括：

- 常规支付完成：`completeSubscriptionOrderAndEvaluateInvitation`。
- Kyren claimed 完成：`completeKyrenSubscriptionOrderWithSnapshotAndEvaluateInvitation`。
- 账户余额购买订阅完成后，如当前代码路径会触发邀请奖励，也必须走同一服务。
- 订阅兑换码兑换：`model.Redeem` 中 `RedemptionTypeSubscription` 分支。

以下入口不得创建新的 `InvitationRewardEvent`：

- 用户查询邀请权益：`GetInvitationEntitlementStatus` / `/api/user/aff/entitlement`，只按现有 active `user_subscriptions` 口径刷新奖励套餐。
- 每日刷新任务：`RunMonthlyInvitationEntitlementSweep`，只按现有 active `user_subscriptions` 口径刷新奖励套餐。
- 返佣补偿任务：`RetryPendingInvitationRewardEvents`，只读取既有 `InvitationRewardEvent` 补偿返佣，不生成新的销售来源事件。

新增统一服务函数，例如：

```go
func HandleInvitationRewardForCompletedSubscriptionOrder(orderId int) error
func HandleInvitationRewardForSubscriptionRedemption(redemptionId int) error
```

销售来源完成后的流程：

```text
读取销售来源、subscription_plan、invitee、创建或延长出的 user_subscription
  -> invitee.inviter_id == 0 则结束
  -> 若来源不是销售型套餐（试用、管理员赠送、邀请奖励套餐）则结束
  -> 创建 InvitationRewardEvent，记录来源类型、来源 ID、订阅 ID、金额快照和新增权益区间；事件不保存奖励模式，也不以 reward_eligible 作为创建门槛
  -> 若 inviter 当前 invitation_reward_mode == commission 且 plan.reward_eligible == true：调用 CreateInvitationCommissionForRewardEvent(event.id)
  -> 若 inviter 当前 invitation_reward_mode == subscription：继续沿用现有 `TryEnsureInvitationEntitlementForPaidUser` / `EnsureMonthlyInvitationEntitlement` 口径，奖励套餐仍按当前 `user_subscriptions` 计算，不读取 `InvitationRewardEvent`
```

奖励套餐刷新规则：

- `EnsureMonthlyInvitationEntitlement` 必须读取并归一化邀请人的当前 `invitation_reward_mode`。
- 邀请人当前为 `subscription` 时，继续允许 create/update `invitation_monthly_entitlements` 和 `monthly_invite_entitlement` 类型 `user_subscriptions`，并保持当前 active `user_subscriptions` 口径。
- 邀请人当前为 `commission` 时，`EnsureMonthlyInvitationEntitlement` 返回 neutral/not_applicable 状态，不得 create/update `invitation_monthly_entitlements`，不得 create/update `monthly_invite_entitlement` 类型 `user_subscriptions`。
- `GetInvitationEntitlementStatus` 对当前 `commission` 用户不得触发套餐 upsert；用户被管理员切回 `subscription` 后，下一次查询或 sweep 按当前 active 订阅口径 fresh 计算奖励套餐。
- `RunMonthlyInvitationEntitlementSweep` 必须按运行时当前模式处理 inviter；当前 `commission` 邀请人跳过套餐 upsert，当前 `subscription` 邀请人按当前 active 订阅口径计算。
- 奖励套餐统计和展示层如果表达「奖励套餐合格人数」或「奖励套餐状态」，必须继续与当前代码中的 active `user_subscriptions` 口径一致，并按 `invitee_id` 去重。若某个统计刻意展示全部付费被邀请人，字段名和文案必须与奖励套餐资格区分。

约束：

- 邀请人不存在、被禁用、销售来源未真实生效时，不生成返佣。
- `reward_eligible = false` 不生成返佣，也不计入奖励套餐；试用套餐和邀请奖励套餐不生成返佣，且按当前奖励套餐代码也不计入奖励套餐。`reward_eligible = false` 只作为 fresh 计算过滤条件，不删除或阻止销售来源事件。
- 订阅兑换码属于销售来源；只要绑定套餐当前 `reward_eligible = true` 且金额快照可证明为 CNY，就必须参与返佣计算。
- 管理员赠送不进入本次返佣销售来源链路；奖励套餐是否计入管理员赠送订阅，继续遵循当前代码中的 active 订阅口径。若未来要让管理员赠送进入返佣，必须先为管理员发放建立同等可审计销售来源事件和金额快照。
- 同一来源事件在返佣侧最多生成 1 条返佣记录；管理员从 `subscription` 切到 `commission` 后，历史尚未生成返佣记录且当前仍合格的有效事件可以被补偿任务按当前模式创建返佣。


### 7.2 返佣计算

配置建议放在 `setting/operation_setting` 或现有可持久化 Option 体系中，第一版需要：

配置注册名固定为 `invitation_commission_setting`，Option 持久化 key 固定使用 `invitation_commission_setting.enabled`、`invitation_commission_setting.rate_bps`、`invitation_commission_setting.minimum_withdraw_cents`、`invitation_commission_setting.minimum_transfer_cents`。如果通过管理接口保存 `rate_bps > 10000`，必须拒绝持久化并保持运行时配置不变。

```go
type InvitationCommissionSetting struct {
    Enabled              bool  `json:"enabled"`
    RateBps              int   `json:"rate_bps"`
    MinimumWithdrawCents int64 `json:"minimum_withdraw_cents"`
    MinimumTransferCents int64 `json:"minimum_transfer_cents"`
}
```

默认建议：

```json
{
  "enabled": true,
  "rate_bps": 1000,
  "minimum_withdraw_cents": 1000,
  "minimum_transfer_cents": 1
}
```

`enabled=false` 的语义：

- 只暂停新的返佣入账，不改变奖励套餐 fresh 计算。
- 历史返佣余额的划转、申请返现、管理员完成/拒绝仍允许。
- 当前应返佣的有效事件在关闭期间不发放返佣，也不得创建会消耗 `(source_type, source_id)` 唯一键的 skipped 返佣记录；重新开启后仍可按当前模式 fresh 计算。

`rate_bps` 配置必须满足 `0 <= rate_bps <= 10000`。超出范围的持久化配置必须拒绝保存，或在运行时按安全默认禁用并记录错误；不得允许返佣比例超过 100%。计算前必须避免 `source_amount_cents * rate_bps` 整数溢出。
`rate_bps <= 0` 与 `enabled=false` 一样只暂停新的返佣入账，不冻结已有 `InvitationCommissionAccount`；历史余额仍允许 summary、划转、申请返现以及管理员完成/拒绝。
金额来源规则：

- 第一版只对来源级快照能证明为 CNY 的合格销售型套餐事件生成可用返佣。
- 支付订单和订阅兑换码都必须保存不可变 `amount_cents` 与 `currency` 快照；返佣事件只从来源快照读取 `source_amount_cents` / `source_currency`。
- 非 CNY、币种缺失、金额无法可靠转换或迁移前缺少来源金额快照的事件不得生成可用返佣，必须记录 `reason = unsupported_currency` 或 `invalid_source_amount`。
- 不得使用当前 `subscription_plans.currency`、当前套餐价格、当前全局汇率或管理员后来修改后的配置补判已落库来源事件金额。

计算公式：

```text
commission_cents = floor(source_amount_cents * rate_bps / 10000)
```

规则：

- `rate_bps <= 0` 时不入账，不创建返佣记录；若配置重新启用，后续补偿任务可按当前规则 fresh 计算。
- `commission_cents <= 0` 时不入账，不创建返佣记录；若后续配置比例变化使返佣金额可结算，补偿任务可按当前规则 fresh 计算。
- 所有金额计算必须使用整数；订单创建阶段如需从现有 decimal/float 金额转分，必须只做一次边界明确的转换并写入订单快照，返佣阶段不得再次使用浮点金额。

### 7.3 创建返佣记录与账户原子更新

服务函数建议：

```go
func CreateInvitationCommissionForRewardEvent(eventId int) error
```

事务内流程：

```text
1. 读取 `InvitationRewardEvent`，校验邀请人当前 `invitation_reward_mode == commission`；当前不是 commission 直接返回 nil，不写返佣记录
2. 通过 `event.SourceSubscriptionId -> UserSubscription -> SubscriptionPlan` fresh 校验当前资格；`reward_eligible = false`、试用、`monthly_invite_entitlement`、订阅不存在或不在有效期内直接返回 nil，不写返佣记录
3. 全局 `Enabled=false` 或 `rate_bps <= 0` 直接返回 nil，不写返佣记录，保留后续 fresh 补偿能力
4. 校验来源金额和币种；非 CNY 写 `unsupported_currency`，金额缺失或非正数写 `invalid_source_amount`，乘法溢出写 `commission_overflow`，这些来源自身不可结算场景可以写 skipped 返佣记录和固定 reason
5. 使用唯一索引尝试插入 `InvitationCommissionRecord`，唯一键为 `(source_type, source_id)`
6. 如果唯一键冲突，说明已处理，直接幂等返回 nil
7. 若记录状态为 skipped，则提交事务，不改账户余额
8. 确保 inviter 的 `InvitationCommissionAccount` 存在
9. 原子增加 `account.available_cents`
10. 读取账户新值，写 `InvitationCommissionLedgerEarned`
11. 提交事务
```

跨数据库并发要求：

- 使用 GORM transaction。
- 使用唯一索引保证返佣记录幂等。
- 确保账户存在时可使用 `OnConflict{DoNothing:true}` 或等价 GORM 写法；唯一冲突后必须按 `user_id` 重读，不得把并发创建视为业务错误。
- 不得依赖 `FOR UPDATE` 作为唯一并发控制，因为 SQLite 不提供同等行级锁语义。
- 所有扣减必须使用条件更新和 `gorm.Expr`，例如：`available_cents = available_cents - ?` 且 `WHERE user_id = ? AND available_cents >= ?`，并检查 `RowsAffected == 1`。
- 所有状态推进必须使用条件更新，例如 `WHERE id = ? AND status = 'pending'`，并检查 `RowsAffected == 1`，防止管理员重复完成/拒绝。
- 写 ledger 前必须读取同一事务内账户变更后的余额，`*_after_cents` 必须等于最终账户值。
- 划转到 `users.quota` 时必须先校验 `amount_cents` 可安全转换为 `int`，并复用 `model.IncreaseUserAccountBalanceTx` 或等价原子更新；失败时返佣账户扣减和 ledger 必须整体回滚。

### 7.4 用户返佣概览

新增用户接口：

```http
GET /api/user/invitation-commission/summary
```

响应示例：

```json
{
  "reward_mode": "commission",
  "has_commission_account": true,
  "can_transfer": true,
  "can_request_withdrawal": true,
  "direct_invite_count": 8,
  "qualified_paid_invite_count": 3,
  "account": {
    "available_cents": 12500,
    "pending_cents": 5000,
    "withdrawn_cents": 30000,
    "transferred_cents": 10000
  },
  "setting": {
    "enabled": true,
    "minimum_withdraw_cents": 1000,
    "minimum_transfer_cents": 1,
    "rate_bps": 1000
  }
}
```

所有接口示例均表示 `data` payload。实际 HTTP 响应沿用项目标准 `common.ApiSuccess(c, payload)` / `ApiResponse<T>` 包装；前端 API helper 必须从响应的 `data` 字段解包，不得把 payload 当作 HTTP 响应根对象。

在 `commission` 模式下，`direct_invite_count` / `qualified_paid_invite_count` 必须无副作用返回，不得 upsert `invitation_monthly_entitlements` 或 `monthly_invite_entitlement` 订阅。钱包三种状态统一使用 summary 返回的统计展示邀请人数；不再依赖会产生套餐 upsert 副作用的旧接口。

规则：

- `reward_mode` 返回用户当前模式。
- `has_commission_account` 表示用户历史上是否存在返佣账户或返佣记录/返现申请。
- 用户当前为 `subscription` 但存在历史返佣账户时，仍必须返回账户信息，并将 `can_transfer` / `can_request_withdrawal` 按历史余额和全局设置计算为 true/false。
- 用户从 `commission` 切回 `subscription` 后，奖励套餐按当前 active 直属订阅口径 fresh 计算；已存在的合法历史返佣余额仍允许用户划转或申请返现，避免冻结余额。
- 无返佣账户且当前为 `subscription` 时，可返回 0 值账户，前端继续展示奖励套餐卡。

### 7.5 用户返佣流水列表

新增用户接口：
```http
GET /api/user/invitation-commission/records?page=1&page_size=20
```

返回当前用户作为邀请人的返佣入账记录，不返回其他用户数据。

响应 envelope 固定为：

```json
{
  "items": [
    {
      "id": 101,
      "event_id": 201,
      "invitee_id": 88,
      "source_type": "subscription_order",
      "source_id": 301,
      "source_trade_no": "sub_20260603001",
      "source_amount_cents": 10000,
      "source_currency": "CNY",
      "commission_rate_bps": 1000,
      "commission_cents": 1000,
      "status": "available",
      "reason": "",
      "created_at": 1780000000,
      "available_at": 1780000000,
      "cancelled_at": 0
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

### 7.6 用户划转到账户余额

新增用户接口：

```http
POST /api/user/invitation-commission/transfer
```

请求：

```json
{
  "amount_cents": 1000
}
```

成功响应：

```json
{
  "available_cents": 11500,
  "transferred_cents": 11000,
  "user_quota": 21000
}
```

事务内流程：

```text
1. 校验用户当前为 commission，或存在历史返佣账户且 available_cents > 0
2. 校验 amount_cents >= minimum_transfer_cents
3. 校验 amount_cents 可安全转换为 users.quota 的 int 分值
4. 使用条件更新扣减 account.available_cents 并增加 account.transferred_cents
5. 调用 model.IncreaseUserAccountBalanceTx 或等价原子更新增加 users.quota
6. 读取账户新值，写 InvitationCommissionLedgerTransferred
7. 清理或更新用户缓存
8. 提交事务
```

必须明确：

- 此流程即时完成。
- 不创建返现申请。
- 不进入管理员待办。
- 不需要管理员审核。

### 7.7 用户申请私聊转账返现

新增用户接口：

```http
POST /api/user/invitation-commission/withdrawals
```

请求：

```json
{
  "amount_cents": 5000,
  "contact": {
    "type": "wechat",
    "value": "user-contact"
  },
  "remark": "请联系我确认返现方式"
}
```

联系方式契约：

- `contact.type` 第一版固定枚举：`wechat`、`telegram`、`email`、`other`。
- `contact.value` trim 后必填，长度 1–128。
- `remark` 可选，长度 0–500。
- 管理员列表中联系方式以类型 label + value 展示，并提供复制能力。

响应：

```json
{
  "id": 123,
  "status": "pending",
  "amount_cents": 5000
}
```

事务内流程：

```text
1. 校验用户当前为 commission，或存在历史返佣账户且 available_cents > 0
2. 校验 amount_cents >= minimum_withdraw_cents
3. 校验联系方式和备注长度
4. 使用条件更新扣减 account.available_cents 并增加 account.pending_cents
5. 创建 InvitationCommissionWithdrawal(status = pending, method = manual)
6. 读取账户新值，写 InvitationCommissionLedgerWithdrawalCreated
7. 提交事务
```

### 7.8 用户返现申请列表

新增用户接口：

```http
GET /api/user/invitation-commission/withdrawals?page=1&page_size=20
```

用户只能查看自己的申请。响应 envelope 固定为：

```json
{
  "items": [
    {
      "id": 123,
      "user_id": 7,
      "amount_cents": 5000,
      "status": "pending",
      "method": "manual",
      "contact": { "type": "wechat", "value": "user-contact" },
      "user_remark": "请联系我确认返现方式",
      "admin_remark": "",
      "reviewer_id": 0,
      "reviewed_at": 0,
      "completed_by": 0,
      "completed_at": 0,
      "created_at": 1780000000,
      "updated_at": 1780000000
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

### 7.9 管理员返现申请列表

新增管理员接口：

```http
GET /api/admin/invitation-commission/withdrawals?status=pending&page=1&page_size=20&user_id=123
```

响应 envelope 固定为：

```json
{
  "items": [
    {
      "id": 123,
      "user_id": 7,
      "username": "user7",
      "display_name": "User 7",
      "amount_cents": 5000,
      "status": "pending",
      "method": "manual",
      "contact": { "type": "wechat", "value": "user-contact" },
      "user_remark": "请联系我确认返现方式",
      "admin_remark": "",
      "reviewer_id": 0,
      "reviewed_at": 0,
      "completed_by": 0,
      "completed_at": 0,
      "created_at": 1780000000,
      "updated_at": 1780000000
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

状态筛选只允许空值、`pending`、`completed`、`rejected`。
### 7.10 管理员标记返现完成

新增管理员接口：

```http
POST /api/admin/invitation-commission/withdrawals/:id/complete
```

请求：

```json
{
  "admin_remark": "已通过微信线下转账"
}
```

事务内流程：

```text
1. 使用条件更新把 withdrawal 从 pending 推进到 completed：WHERE id = ? AND status = 'pending'；同一次更新必须写入 `status = completed`、`admin_remark`、`reviewer_id`、`reviewed_at`、`completed_by`、`completed_at`、`updated_at`
2. 若 RowsAffected == 0，返回业务错误，防止重复完成
3. 使用条件更新扣减 account.pending_cents 并增加 account.withdrawn_cents：WHERE user_id = ? AND pending_cents >= amount
4. 读取账户新值，写 InvitationCommissionLedgerWithdrawalCompleted，ledger reference 必须指向该 withdrawal
5. 提交事务
```

必须明确：

- 不修改 `users.quota`。
- 不触发充值成功逻辑。
- 不创建 `TopUp`。

### 7.11 管理员拒绝返现申请

新增管理员接口：

```http
POST /api/admin/invitation-commission/withdrawals/:id/reject
```

请求：

```json
{
  "admin_remark": "联系方式无效，请重新提交"
}
```

事务内流程：

```text
1. 使用条件更新把 withdrawal 从 pending 推进到 rejected：WHERE id = ? AND status = 'pending'；同一次更新必须写入 `status = rejected`、`admin_remark`、`reviewer_id`、`reviewed_at`、`updated_at`
2. 若 RowsAffected == 0，返回业务错误，防止重复拒绝
3. 使用条件更新扣减 account.pending_cents 并增加 account.available_cents：WHERE user_id = ? AND pending_cents >= amount
4. 读取账户新值，写 InvitationCommissionLedgerWithdrawalRejected，ledger reference 必须指向该 withdrawal
5. 提交事务
```

### 7.12 管理员待办摘要

新增管理员接口：

```http
GET /api/admin/tasks/summary
```

第一版响应：

```json
{
  "pending_commission_withdrawals": 3
}
```

后续如果存在其他业务待办，可以扩展字段；第一版不需要抽象通用任务表。


---

## 8. 前端设计

### 8.1 用户列表奖励模式配置

涉及文件：

- `web/default/src/features/users/types.ts`
- `web/default/src/features/users/api.ts`
- `web/default/src/features/users/components/users-columns.tsx`
- `web/default/src/features/users/components/users-mutate-drawer.tsx`
- `web/default/src/features/users/lib/user-form.ts`

需求：

- 用户列表展示邀请奖励模式：
  - `subscription` → 奖励套餐；
  - `commission` → 邀请返佣。
- 用户编辑抽屉增加管理员可编辑字段「邀请奖励模式」。
- 默认值显示为「奖励套餐」。
- 只有管理员用户管理页面出现该设置。
- 普通用户设置页不出现该字段。

建议 UI：

```text
邀请奖励模式
○ 奖励套餐（默认，适用于普通用户）
○ 邀请返佣（仅特邀用户，管理员手动开启）
```

提交给后端的字段：

```json
{
  "invitation_reward_mode": "commission"
}
```

### 8.2 钱包邀请奖励卡

涉及文件：

- `web/default/src/features/wallet/components/affiliate-rewards-card.tsx`
- `web/default/src/features/wallet/hooks/use-affiliate.ts`
- 新增：`web/default/src/features/wallet/hooks/use-invitation-commission.ts`
- 新增：`web/default/src/features/wallet/components/dialogs/commission-transfer-dialog.tsx`
- 新增：`web/default/src/features/wallet/components/dialogs/commission-withdrawal-dialog.tsx`

展示规则：

```text
reward_mode == subscription 且无历史返佣账户
  -> 继续展示当前奖励套餐规则与邀请统计

reward_mode == commission 或 has_commission_account == true
  -> 展示返佣余额卡、返佣入账记录、返现申请记录
```

邀请链接输入框、复制按钮和邀请统计在 `subscription` 与 `commission` 两种模式下都必须保留；差异只在奖励说明和账户操作区。

如果用户当前 `reward_mode = subscription` 但 `has_commission_account = true`，钱包必须显示历史返佣处理区，并提示：

```text
新的付费邀请将获得奖励套餐，历史返佣余额仍可处理。
```

返佣模式卡片字段：

- 可用返佣余额
- 待处理返现金额
- 已返现金额
- 已划转余额
- 当前返佣比例
- 最低返现金额

按钮：

1. **划转到余额**
   - 描述：立即转入账户余额，可用于购买套餐。
   - 成功后刷新返佣概览和用户余额。
   - 不显示「待审核」文案。

2. **申请返现**
   - 描述：提交后由管理员按用户填写的联系方式私聊确认，并线下/手动转账。
   - 成功后状态为待处理。
   - 必须提示「这不是自动打款」。

`setting.enabled = false` 时：

- 不影响历史余额展示。
- 划转和申请返现按钮是否可用由 `can_transfer` / `can_request_withdrawal` 决定。
- mutation 失败统一使用现有错误处理/toast 约定，不自行解析非标准错误结构。

### 8.3 前端 API 与缓存约定

用户侧 API helper 放在 `web/default/src/features/wallet/api.ts` 或同目录新增专用文件；管理员侧 API helper 放在 `web/default/src/features/invitation-commission/api.ts`。

推荐 React Query keys：

```ts
['wallet', 'invitation-commission', 'summary', userId]
['wallet', 'invitation-commission', 'records', userId, params]
['wallet', 'invitation-commission', 'withdrawals', userId, params]
['admin', 'invitation-commission', 'withdrawals', params]
['admin', 'tasks', 'summary']
```

刷新规则：

- 用户划转成功后必须刷新返佣 summary、当前钱包用户余额（调用钱包 `fetchUser` 或更新 auth store 中 `quota`）、必要时刷新 records。
- 用户申请返现成功后必须刷新返佣 summary 和用户 withdrawals。
- 管理员 complete/reject 成功后必须刷新 admin withdrawals 和 admin tasks summary。
- 在返佣模式下，钱包邀请统计使用 `GET /api/user/invitation-commission/summary` 返回的 `direct_invite_count` / `qualified_paid_invite_count`，不得调用会产生套餐 upsert 副作用的接口。

### 8.4 管理员返现审核页

新增固定路由和组件，避免并行实现时自行选择落点：

- 路由：`web/default/src/routes/_authenticated/invitation-commission/withdrawals/index.tsx`
- 页面组件：`web/default/src/features/invitation-commission/admin-withdrawals.tsx`

权限：

- 页面必须使用管理员权限 guard，普通用户不可访问。
- 侧边栏入口放在 Admin 组 Users 之后，标题为 `Manual cashback requests` /「返现申请」，URL 为 `/invitation-commission/withdrawals`。
- badge 显示 `GET /api/admin/tasks/summary` 返回的 `pending_commission_withdrawals`；计数为 0 或请求失败时隐藏 badge。
- 侧边栏权限复用用户管理权限；如果实现阶段发现现有配置需要显式 URL 映射，必须同步更新 `URL_TO_CONFIG_MAP` 或对应 sidebar 配置。
- `GET /api/admin/tasks/summary` 的前端查询必须只在当前用户具备管理员权限且 Admin sidebar 项可见时启用；非管理员不得发起该请求。该查询失败只隐藏 badge，不显示 toast，不影响普通用户页面。

页面能力：

- 默认筛选 `pending`。
- 支持状态筛选：pending / completed / rejected。
- 支持按用户 ID 搜索。
- 展示联系方式类型、联系方式值、复制按钮和用户备注。
- 提供「标记完成」按钮。
- 提供「拒绝」按钮。
- 两个操作都必须要求管理员填写或确认备注。

完成按钮文案建议：

```text
标记已线下返现
```

不要使用「打款成功」「充值成功」等容易和系统内支付混淆的文案。

### 8.5 管理员待办入口

管理员侧边栏 badge 是第一版待办入口；不新增通用站内任务表。

约束：

- 只统计 `pending` 返现申请。
- 不统计用户自助划转到账户余额。
- 不把待办摘要和运维健康状态混为一个健康分数。
- 不把该数量写入 AdminOps health score。

### 8.6 i18n

新增用户可见文案必须写入：

- `web/default/src/i18n/locales/en.json`
- `web/default/src/i18n/locales/zh.json`
- `web/default/src/i18n/locales/fr.json`
- `web/default/src/i18n/locales/ja.json`
- `web/default/src/i18n/locales/ru.json`
- `web/default/src/i18n/locales/vi.json`

建议新增 key 使用英文源文案，例如：

```text
Invitation reward mode
Reward package
Commission
Commission is only available for invited special users enabled by administrators.
Transfer to balance
Request manual cashback
Pending cashback requests
Mark manual cashback as completed
Manual cashback requests
This is not an automatic payout.
New paid invitations will receive reward packages. Historical commission balance can still be handled.
```

如果奖励模式、返现状态、管理员导航等文案通过常量 `labelKey` 或动态 key 渲染，必须同步更新 `web/default/src/i18n/static-keys.ts`；否则必须以 `t('English source key')` 字面量出现在组件中，并运行 `bun run i18n:sync` 补齐 6 个 locale。

---

## 9. 路由与权限

### 9.1 用户接口

所有用户接口必须使用 `middleware.UserAuth()`：

```text
GET  /api/user/invitation-commission/summary
GET  /api/user/invitation-commission/records
POST /api/user/invitation-commission/transfer
GET  /api/user/invitation-commission/withdrawals
POST /api/user/invitation-commission/withdrawals
```

权限规则：

- 用户只能读取自己的返佣账户、流水和返现申请。
- 用户写接口允许当前 `reward_mode = commission` 的用户操作。
- 用户当前 `reward_mode = subscription` 但存在历史返佣账户且 `available_cents` 足够时，也允许划转或申请返现。
- 无返佣模式且无历史返佣账户的用户调用写接口应返回业务错误。
- 读接口必须返回 `reward_mode`、`has_commission_account`、`can_transfer`、`can_request_withdrawal`，用于前端决定展示奖励套餐卡或历史返佣处理区。

### 9.2 管理员接口

所有管理员接口必须使用 `middleware.AdminAuth()`：

```text
GET  /api/admin/invitation-commission/withdrawals
POST /api/admin/invitation-commission/withdrawals/:id/complete
POST /api/admin/invitation-commission/withdrawals/:id/reject
GET  /api/admin/tasks/summary
```

用户奖励模式设置复用现有用户管理更新接口时，必须确保只有管理员路由能写 `invitation_reward_mode`。普通用户的个人资料更新接口不得接受该字段。

---

## 10. 状态机与资金流

### 10.1 奖励模式状态

```text
subscription（默认） <-> commission（管理员设置）
```

切换规则：

- 管理员可以把特邀用户从 `subscription` 切到 `commission`。
- 管理员也可以把用户从 `commission` 切回 `subscription`。
- 切回 `subscription` 不应删除已有返佣账户、销售来源事件、流水或返现申请。
- 切回后奖励套餐按当前 active 直属订阅口径 fresh 计算；历史返佣记录保持审计用途，不会被删除或冲正。
- 已存在的合法历史返佣余额仍允许用户划转或申请返现，避免冻结余额。

### 10.2 划转到账户余额

```text
available_cents -= amount
transferred_cents += amount
users.quota += amount
ledger: transferred_to_balance
```

特征：

- 即时完成。
- 不进入审核。
- 不创建返现申请。
- 不产生管理员待办。

### 10.3 私聊转账返现

创建申请：

```text
available_cents -= amount
pending_cents += amount
withdrawal.status = pending
ledger: withdrawal_created
```

管理员标记完成：

```text
pending_cents -= amount
withdrawn_cents += amount
withdrawal.status = completed
ledger: withdrawal_completed
```

管理员拒绝：

```text
pending_cents -= amount
available_cents += amount
withdrawal.status = rejected
ledger: withdrawal_rejected
```

管理员完成返现时不得修改 `users.quota`。

---

## 11. 兼容性与迁移

### 11.1 数据库兼容

- 所有新增表使用 GORM `AutoMigrate`。
- 金额字段使用整数类型，避免 `decimal` 或浮点数跨库差异。
- 避免原生 SQL；如必须使用，必须同时支持 SQLite、MySQL、PostgreSQL。
- 新增唯一索引必须在三种数据库上可创建。

### 11.2 历史用户默认值

历史用户必须满足：

```text
invitation_reward_mode 为空或不存在
  -> 运行时视为 subscription
```

迁移后如数据库写入默认值，默认值必须是：

```text
subscription
```

### 11.3 历史奖励数据

- 不迁移 `users.aff_quota` 到返佣账户。
- 不迁移 `invitation_monthly_entitlements` 到返佣流水。
- 不删除历史奖励套餐订阅。
- 不改变已有 `aff_transfer` 行为。
- 必须为现有活跃销售型邀请订阅保守回填 `InvitationRewardEvent`，`source_type = legacy_user_subscription`、`source_id = user_subscriptions.id`，用于保持返佣 fresh 计算和历史审计的来源事实；奖励套餐仍沿用现有 active 订阅口径，不读取该表。
- 回填事件只能覆盖当前仍 active 的历史销售型邀请订阅；不得为试用、管理员赠送、邀请奖励套餐本身生成事件。`reward_eligible = false` 的历史销售型套餐可以回填来源事件，但当前 fresh 计算不得计入奖励套餐或产生可用返佣。金额快照只能在存在明确来源字段，或能通过唯一成功订单 / 订阅兑换码与 `user_subscriptions` 的用户、套餐、时间区间建立可审计匹配时复制；存在多个候选、无快照、区间不匹配或来源不明时必须写 `source_amount_cents = 0`、`source_currency = ""`，后续返佣侧记录 `invalid_source_amount` 且不得生成可用入账。

### 11.4 并发与幂等

必须覆盖以下并发场景：

- 同一支付回调或兑换码重复触发事件和返佣创建。
- 首次并发创建同一个返佣账户。
- 用户同时发起两次划转。
- 用户提交返现申请同时发起划转。
- 管理员重复点击完成或拒绝。

要求：

- 同一销售来源只生成 1 条 `InvitationRewardEvent`。
- 支付回调或兑换码重复处理只生成 1 条返佣记录和 1 条 earned ledger。
- 账户余额不能扣成负数。
- `pending` 申请只能完成或拒绝一次。
- 所有资金变化在同一数据库事务内完成。
- SQLite AutoMigrate 必须能创建新增表和复合唯一索引。

---

## 12. 测试要求

### 12.1 后端模型与服务测试

建议新增或扩展：

- `model/invitation_commission_test.go`
- `service/invitation_commission_test.go`
- `service/invitation_reward_test.go`

必须覆盖：

- 历史用户默认 `subscription`。
- 历史活跃销售型邀请订阅会回填 `legacy_user_subscription` 事件；当前 `reward_eligible = false` 的套餐被返佣 fresh 计算过滤。
- 管理员设置 `commission` 后，当前有效、当前 `reward_eligible = true` 且未入账的销售来源可以生成返佣。
- 默认 `subscription` 用户继续按当前 active 直属订阅口径 fresh 计算奖励套餐，不创建新返佣。
- `RunMonthlyInvitationEntitlementSweep` 对当前 `commission` 邀请人不创建套餐。
- `GetInvitationEntitlementStatus` / `/api/user/aff/entitlement` 对当前 `commission` 邀请人不创建套餐；切回 `subscription` 后重新按当前 active 直属订阅口径 fresh 计算。
- 同一订阅订单或订阅兑换码重复处理时只产生 1 条事件、1 条返佣记录和 1 条 earned ledger。
- 返佣金额按 `source_amount_cents * rate_bps / 10000` 向下取整。
- `rate_bps` 边界：0 不入账，10000 最多返还来源快照金额，超出 10000 的配置被拒绝或运行时禁用，计算不会整数溢出。
- 非 CNY 或金额无法可靠转换的销售来源不产生可用返佣，并记录 reason。
- 订单或兑换码创建后修改套餐币种或价格，返佣仍按来源级 `amount_cents` / `currency` 快照判断。
- 奖励套餐继续按当前代码口径对同一 invitee 去重；同一 invitee 的多个 active 合格订阅不能满足两个奖励套餐邀请名额。
- 续费或兑换码延长事件时间段记录新增区间，不因重叠区间重复计数。
- 销售来源完成事务捕获续费前后的 `EventStartTime` / `EventEndTime`；重复回调或重复兑换不得重新创建事件。
- Stripe/Creem/Kyren 等外部支付订单金额快照来自可校验的 provider 金额/币种；无法证明时跳过可用返佣。
- 账户余额购买创建的 `SubscriptionOrder.amount_cents/currency` 等于实际扣减的 CNY 分 / `CNY`。
- 订阅兑换码创建或更新时写入 `Redemption.amount_cents/currency`，兑换时复制到 `InvitationRewardEvent`。
- Epay 创建订单时快照等于提交给 Epay 的 CNY 分 / `CNY`；回调必须校验实际支付金额与订单快照一致，重复回调仍只使用首次订单快照。
- 试用套餐、管理员赠送、邀请奖励套餐不产生返佣。
- 首次并发创建返佣账户不会返回业务错误。
- 并发划转与返现申请不会让账户余额为负数，且 `users.quota` 与 ledger 一致。
- 划转到账户余额即时增加 `users.quota`，且不创建返现申请。
- 申请返现冻结 `available_cents` 并增加 `pending_cents`。
- 管理员重复完成或拒绝只有第一次成功且只写 1 条对应 ledger。
- 管理员 complete/reject 后写入处理人、处理时间、管理员备注，且 ledger reference 指向对应 withdrawal。
- 管理员完成返现只移动返佣账户金额，不增加 `users.quota`。
- 管理员拒绝返现退回可用返佣余额。
- SQLite AutoMigrate 能创建 `InvitationRewardEvent`、返佣账户、返佣记录、返佣账户流水、返现申请表和复合唯一索引。

### 12.2 后端 Controller 测试

建议新增：

- `controller/invitation_commission_test.go`
- `controller/admin_invitation_commission_test.go`

必须覆盖：

- 普通用户不能访问他人的返佣记录或返现申请。
- 无返佣模式且无历史返佣账户的用户不能发起划转或返现申请。
- 当前为 `subscription` 但有历史返佣余额的用户仍能划转或申请返现。
- 管理员可以筛选 `pending` 返现申请。
- 管理员完成和拒绝接口对非 `pending` 申请返回错误。
- 普通用户更新个人资料不能修改 `invitation_reward_mode`。
- 管理员用户列表更新接口可以修改 `invitation_reward_mode`。

### 12.3 前端测试

建议新增或扩展源码契约测试：

- `web/default/src/features/users/users-form.test.ts`
- `web/default/src/features/wallet/wallet-layout.test.ts`
- `web/default/src/features/invitation-commission/admin-withdrawals.test.ts`

必须覆盖：

- 用户编辑抽屉包含「邀请奖励模式」字段。
- 奖励模式字段只出现在管理员用户管理上下文。
- 钱包在 `subscription` 模式下继续展示奖励套餐文案。
- 钱包在 `commission` 模式下展示「划转到余额」和「申请返现」。
- 钱包在 `commission` 模式下仍保留邀请链接复制能力和邀请统计。
- 当前为 `subscription` 但有历史返佣账户时，钱包仍展示历史返佣处理区。
- 「划转到余额」文案不包含审核语义。
- 「申请返现」文案包含不是自动打款或手动处理语义。
- 管理员返现审核页包含路由、管理员 guard、状态筛选、联系方式复制、「标记已线下返现」操作。
- 管理员侧边栏在 Admin 组 Users 之后显示 `Manual cashback requests` /「返现申请」入口，普通用户不可见；badge 使用 `['admin', 'tasks', 'summary']` 的 `pending_commission_withdrawals`，计数为 0 或请求失败时隐藏；入口 URL 固定为 `/invitation-commission/withdrawals`，且 sidebar 配置 / `URL_TO_CONFIG_MAP` 映射同步生效。
- 普通用户不会请求 `/api/admin/tasks/summary`；管理员但对应 sidebar 配置不可见时也不会请求；只有管理员且入口实际可见时才请求；请求失败隐藏 badge 且不 toast。
- 用户划转成功后刷新 `['wallet', 'invitation-commission', 'summary', userId]` 并刷新当前钱包用户余额或 auth store `quota`；用户申请返现成功后刷新 summary 和 `['wallet', 'invitation-commission', 'withdrawals', userId, params]`。
- 管理员 complete/reject 后刷新 admin withdrawals 和 `['admin', 'tasks', 'summary']`。
- API/types 源码契约固定 `amount_cents`、`reward_mode`、`invitation_reward_mode`、`has_commission_account`、`can_transfer`、`can_request_withdrawal` 字段名。

### 12.4 验证命令

实现完成后至少运行：

```bash
go test ./model ./service ./controller -run 'Invitation|Commission|Withdrawal|User' -count=1
```

如修改 `web/default`：

```bash
bun run typecheck
bun test
```

如果新增或修改 i18n key：

```bash
bun run i18n:sync
```

---

## 13. 风险与处理

### 13.1 模式切换后统计不刷新

风险：实现继续把来源事件永久绑定到历史模式，导致管理员切换用户模式后，返佣不能按当前规则 fresh 计算；或误把奖励套餐改成依赖新增事件表，改变现有奖励套餐口径。

处理：`InvitationRewardEvent` 只记录销售来源事实，不保存奖励模式；奖励套餐继续使用当前代码中对 active `user_subscriptions` 的口径，返佣服务每次读取邀请人当前 `invitation_reward_mode`，并用 `reward_eligible` 和当前有效区间重新计算。测试覆盖支付订单、订阅兑换码、`GetInvitationEntitlementStatus`、每日 sweep 和返佣补偿任务。

### 13.2 默认模式误伤普通用户

风险：历史用户字段为空，前端或后端误判为返佣模式。

处理：空值和非法值运行时一律归一为 `subscription`；数据库默认值也是 `subscription`。

### 13.3 把划转和返现审核混淆

风险：用户自助划转也进入管理员待办，或管理员完成返现时给 `users.quota` 加钱。

处理：划转接口只做 `available -> users.quota`；返现完成只做 `pending -> withdrawn`。文案中明确「申请返现」是线下手动处理。

### 13.4 复用 `TopUp` 导致审计混乱

风险：返现申请被当作充值订单，管理员完成时触发充值入账。

处理：返佣返现必须使用独立表和接口；不得创建 `TopUp`。

### 13.5 资金并发扣减

风险：并发划转和返现导致 `available_cents` 负数，或 `users.quota`、返佣账户、ledger 三者不一致。

处理：所有账户扣减必须使用事务内条件更新和 `RowsAffected` 校验；写 ledger 前读取同事务内账户最终值；任一环节失败必须整体回滚。

---

## 14. 验收清单

- [ ] 新用户和历史用户默认奖励模式均为 `subscription`。
- [ ] 用户不能自助开启返佣模式。
- [ ] 管理员能在用户列表中为特邀用户开启或关闭 `commission`。
- [ ] 每个新销售型邀请来源都会写入 `InvitationRewardEvent`，包括支付订单、账户余额购买和订阅兑换码。
- [ ] 历史 active 销售型邀请订阅会回填 `legacy_user_subscription` 来源事件；其中当前 `reward_eligible = false` 的来源只保留事实，fresh 计算不得计入奖励套餐或可用返佣。
- [ ] 奖励套餐按当前 `subscription` 模式、当前代码中的 active 直属订阅口径和 `reward_eligible` fresh 计算。
- [ ] 当前 `commission` 模式的用户按有效销售来源获得返佣。
- [ ] `GetInvitationEntitlementStatus` 和每日 sweep 不会给当前 `commission` 邀请人创建奖励套餐；切回 `subscription` 后会按当前 active 订阅口径 fresh 计算。
- [ ] 返佣记录对重复支付回调和重复兑换码处理幂等。
- [ ] 用户自助划转到账户余额即时完成，不进入管理员待办。
- [ ] 用户申请私聊转账返现后状态为 `pending`，金额进入冻结。
- [ ] 管理员待办只统计 `pending` 返现申请。
- [ ] 管理员标记完成后只更新返佣账户和申请状态，不增加 `users.quota`。
- [ ] 管理员拒绝后金额退回可用返佣余额。
- [ ] 所有新增金额字段使用 CNY 分整数。
- [ ] 所有 JSON marshal/unmarshal 使用 `common.*` 包装函数。
- [ ] SQLite、MySQL、PostgreSQL 兼容。
- [ ] 本功能新增的联系方式快照、provider 校验载荷、配置 JSON、outbox/审计 JSON 均使用 `common.Marshal`、`common.Unmarshal`、`common.UnmarshalJsonStr` 或 `common.DecodeJson`，不得直接导入 `encoding/json`。
- [ ] 前端新增文案已补齐 6 个 locale。
