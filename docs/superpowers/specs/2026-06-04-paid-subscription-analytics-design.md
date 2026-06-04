# 付费套餐剩余价值与邀请付费统计面板设计规格

## 1. 背景

当前项目存在多种套餐获得方式：在线支付、余额购买、兑换码、管理员手动分配、售后补发、邀请权益等。运营上这些来源并不都能通过 `SubscriptionOrder` 完整追溯，但业务上只要用户持有有价套餐，且该用户没有被管理员排除，就应视为已经收过款并计入统计。

因此，本规格定义两个后台统计面板：

1. **付费套餐剩余价值面板**：统计当前所有未排除用户的有效有价套餐剩余价值。
2. **邀请付费统计面板**：统计邀请人名下下级用户的有价套餐金额、套餐明细和剩余价值。

两个面板使用同一条核心会计口径：

```text
有价套餐 + 用户未排除 = 已收款，计入统计。
```

`SubscriptionOrder` 只能作为辅助追溯来源，不决定是否计入收款统计。

## 2. 目标

### 2.1 付费套餐剩余价值面板

管理员应能看到：

- 当前所有未排除用户的付费套餐剩余价值总额。
- 每个用户、每个套餐订阅的剩余价值。
- token 口径剩余价值、时间口径剩余价值，以及最终计入口径。
- 按套餐、来源、币种等维度聚合的剩余价值。
- 被排除用户本应贡献的金额，用于审计。

### 2.2 邀请付费统计面板

管理员应能看到：

- 每个邀请人带来的下级用户数量。
- 每个邀请人的下级付费用户数量。
- 每个邀请人的下级用户持有的有价套餐总金额。
- 下级用户具体获得过哪些有价套餐。
- 下级用户当前有效付费套餐和剩余价值。
- 被排除用户对邀请统计的影响。

## 3. 非目标

本规格不覆盖以下内容：

- 不实现新的支付、退款、发票或财务对账系统。
- 不用 `SubscriptionOrder.money` 作为主收款口径。
- 不要求订单与订阅之间建立强一致关联。
- 不在本阶段新增复杂图表；表格、统计卡片和钻取明细优先。
- 不改变现有套餐发放、兑换码、管理员分配或邀请奖励流程。

## 4. 现有数据基础

### 4.1 套餐与订阅

核心实体位于 `model/subscription.go`。

`SubscriptionPlan` 提供套餐定义：

- `price_amount`：套餐标价。大于 0 时视为有价套餐。
- `currency`：币种。
- `duration_unit`、`duration_value`、`custom_seconds`：套餐有效期定义。
- `monthly_token_limit`：套餐周期 token 限额。
- `quota_reset_period`、`quota_reset_custom_seconds`：额度重置周期。
- `is_trial`、`reward_eligible`、`business_code`：业务分类字段。

`UserSubscription` 提供用户实际持有的套餐权益：

- `user_id`、`plan_id`
- `token_limit`、`token_used`
- `amount_total`、`amount_used`
- `start_time`、`end_time`
- `last_reset_time`、`next_reset_time`
- `grant_reason`、`source`

### 4.2 订单

`SubscriptionOrder` 记录部分购买链路：

- `user_id`、`plan_id`
- `money`
- `payment_provider`、`payment_method`
- `status`
- `create_time`、`complete_time`
- `kyren_snapshot`、`entitlement_snapshot`

订单数据只用于展示和追溯，不作为两个面板的主统计依据。

### 4.3 邀请关系

邀请关系以用户表中的邀请人字段为准：

```text
invitee.inviter_id = inviter.id
```

邀请统计面板以该关系关联下级用户，再从下级用户的 `UserSubscription + SubscriptionPlan` 计算付费金额。

## 5. 统一统计口径

### 5.1 有价套餐

满足以下条件的套餐为有价套餐：

```text
SubscriptionPlan.price_amount > 0
```

该判断不依赖：

- `SubscriptionOrder.status`
- `SubscriptionOrder.money`
- `UserSubscription.source`
- `UserSubscription.grant_reason`
- `payment_provider`
- `payment_method`

### 5.2 计入统计的用户

默认计入统计的用户满足：

```text
user_id NOT IN subscription_analytics.excluded_users
```

被排除用户的有价套餐不计入主统计结果，但应可在审计视图中查看其本应计入的金额。

### 5.3 已收款判断

对于两个面板，已收款判断统一为：

```text
has_paid_subscription = exists(
  UserSubscription
  JOIN SubscriptionPlan
  WHERE UserSubscription.user_id = user.id
  AND SubscriptionPlan.price_amount > 0
)
AND user.id NOT IN excluded_users
```

只要满足该条件，即视为付费用户或已收款用户。

### 5.4 金额口径

主金额来自套餐标价：

```text
recognized_paid_amount = sum(SubscriptionPlan.price_amount)
```

不是：

```text
sum(SubscriptionOrder.money)
```

`SubscriptionOrder.money` 可作为辅助字段展示，例如「订单记录金额」或「支付订单金额」，但不能命名为「实收金额」，也不能决定是否计入主统计。

### 5.5 多币种

金额必须按 `currency` 分组展示。不同币种不得静默相加。

如果后续需要折算为统一币种，必须新增明确的汇率配置和折算时间点，本规格不包含该能力。

## 6. 管理员排除用户配置

### 6.1 配置位置

建议在现有系统设置中追加：

```text
System Settings -> Billing -> Statistics
```

或者在现有 Billing 设置页面中增加「统计排除用户」区域。

### 6.2 配置结构

建议配置 key：

```text
subscription_analytics.excluded_users
```

建议 JSON 结构：

```json
{
  "users": [
    {
      "user_id": 1001,
      "reason": "内部测试账号",
      "excluded_at": 1760000000,
      "excluded_by": 1
    }
  ]
}
```

### 6.3 行为

被排除用户：

- 不计入付费套餐剩余价值主统计。
- 不计入邀请付费金额。
- 不算作邀请付费用户。
- 可在「已排除」过滤视图中查看。
- 应展示 `excluded = true`、`excluded_reason`、`would_have_amount` 等审计字段。

## 7. 面板一：付费套餐剩余价值

### 7.1 统计对象

默认统计范围：

```text
UserSubscription.status = active
AND UserSubscription.start_time <= snapshot_at
AND UserSubscription.end_time > snapshot_at
AND SubscriptionPlan.price_amount > 0
AND UserSubscription.user_id NOT IN excluded_users
```

如果现有状态字段或有效性判断已有封装函数，后续实现应复用现有判断，避免统计口径和业务逻辑不一致。

### 7.2 快照时间

所有计算以同一个 `snapshot_at` 为准。

- 前端首次进入页面时由后端生成默认 `snapshot_at`。
- 同一次查询中的 summary、users、subscriptions 应使用相同 `snapshot_at`。
- 允许管理员手动指定 `snapshot_at`，用于复盘历史时点。

### 7.3 剩余价值计算

每条有效有价订阅计算 3 个值：

```text
token_based_value
time_based_value
recognized_remaining_value = min(token_based_value, time_based_value)
```

如果无法计算 token 口径，则：

```text
recognized_remaining_value = time_based_value
valuation_basis = time_only
```

### 7.4 时间口径

```text
time_based_value = plan_price * remaining_seconds / plan_duration_seconds
```

其中：

```text
remaining_seconds = max(end_time - snapshot_at, 0)
```

`plan_duration_seconds` 应由套餐时长字段计算得出，并与现有订阅创建逻辑保持一致。

### 7.5 token 口径

token 口径必须考虑额度重置周期，不能只看当前周期剩余 token。

推荐计算步骤：

1. 将订阅剩余时间 `[snapshot_at, end_time)` 按额度周期切分。
2. 当前周期使用真实 token 剩余额度。
3. 后续完整周期按完整周期价值计入。
4. 最后不足一个完整周期的片段按时间比例折算。

当前周期：

```text
current_cycle_token_ratio = max(token_limit - token_used, 0) / token_limit
current_cycle_value = cycle_price * current_cycle_token_ratio
```

完整未来周期：

```text
full_future_cycle_value = cycle_price
```

最后短周期：

```text
partial_future_cycle_value = cycle_price * partial_cycle_seconds / cycle_seconds
```

总 token 口径：

```text
token_based_value = current_cycle_value
  + sum(full_future_cycle_value)
  + partial_future_cycle_value
```

### 7.6 示例

套餐：

```text
40 元 / 30 天 / 每自然月 1B token
```

用户当前剩余：

```text
剩余 33 天
当前周期剩余 1 天
下一个完整自然月 30 天
最后一个自然月可用 2 天
当前周期已用 200M token
```

计算：

```text
当前周期 token 价值 = (1B - 200M) / 1B * 40 = 32 元
下一个完整周期价值 = 40 元
最后 2 天价值 = 2 / 30 * 40 = 2.67 元

token_based_value = 32 + 40 + 2.67 = 74.67 元

time_based_value = 33 / 30 * 40 = 44 元

recognized_remaining_value = min(74.67, 44) = 44 元
```

### 7.7 特殊情况

#### 7.7.1 token 限额为空或无限

当 `token_limit <= 0` 或套餐被视为无限 token 时：

```text
token_based_value = null
recognized_remaining_value = time_based_value
valuation_basis = time_only
```

#### 7.7.2 token 已超用

当 `token_used > token_limit` 时：

```text
current_cycle_token_ratio = 0
```

未来周期仍按重置后的周期价值计算。

#### 7.7.3 套餐价格小于等于 0

不计入主统计。

#### 7.7.4 被排除用户

不计入主统计，但可在审计视图中返回：

```text
excluded = true
would_have_remaining_value = 原本应计入的剩余价值
```

### 7.8 顶部统计卡片

建议展示：

1. **付费套餐剩余价值**：`sum(recognized_remaining_value)`
2. **token 口径剩余价值**：`sum(token_based_value)`，无法计算 token 口径的订阅单独计数。
3. **时间口径剩余价值**：`sum(time_based_value)`
4. **有效有价订阅数**
5. **付费用户数**
6. **已排除用户本应计入金额**

### 7.9 聚合表

#### 按套餐聚合

| 字段 | 说明 |
|---|---|
| plan_id | 套餐 ID |
| plan_name | 套餐名称 |
| currency | 币种 |
| active_user_count | 有效付费用户数 |
| active_subscription_count | 有效有价订阅数 |
| recognized_remaining_value | 付费套餐剩余价值 |
| token_based_value | token 口径剩余价值 |
| time_based_value | 时间口径剩余价值 |
| average_token_usage_ratio | 平均 token 使用率 |

#### 按来源聚合

| 字段 | 说明 |
|---|---|
| source | 订阅来源 |
| grant_reason | 发放原因 |
| user_count | 用户数 |
| subscription_count | 订阅数 |
| recognized_remaining_value | 付费套餐剩余价值 |

来源只用于解释套餐来源，不影响是否计入。

### 7.10 用户明细表

| 字段 | 说明 |
|---|---|
| user_id | 用户 ID |
| username | 用户名 |
| display_name | 展示名称 |
| active_paid_plan_count | 当前有效有价套餐数 |
| recognized_remaining_value | 付费套餐剩余价值 |
| token_based_value | token 口径剩余价值 |
| time_based_value | 时间口径剩余价值 |
| earliest_end_time | 最近到期时间 |
| excluded | 是否被排除 |
| excluded_reason | 排除原因 |

### 7.11 订阅明细表

| 字段 | 说明 |
|---|---|
| subscription_id | 订阅 ID |
| user_id | 用户 ID |
| plan_id | 套餐 ID |
| plan_name | 套餐名称 |
| source | 来源 |
| grant_reason | 发放原因 |
| plan_price | 套餐标价 |
| currency | 币种 |
| start_time | 开始时间 |
| end_time | 结束时间 |
| remaining_seconds | 剩余秒数 |
| token_limit | 当前周期 token 限额 |
| token_used | 当前周期已用 token |
| next_reset_time | 下次重置时间 |
| token_based_value | token 口径剩余价值 |
| time_based_value | 时间口径剩余价值 |
| recognized_remaining_value | 最终计入剩余价值 |
| valuation_basis | 估值依据 |
| excluded | 是否被排除 |

### 7.12 后端接口建议

```text
GET /api/admin-analytics/paid-subscription-value/summary
GET /api/admin-analytics/paid-subscription-value/users
GET /api/admin-analytics/paid-subscription-value/subscriptions
GET /api/admin-analytics/paid-subscription-value/breakdown
```

查询参数：

```text
snapshot_at
plan_ids
user_ids
sources
grant_reasons
business_codes
currency
include_excluded=false
only_excluded=false
limit
offset
sort_by
sort_order
```

## 8. 面板二：邀请付费统计

### 8.1 统计对象

邀请关系：

```text
invitee.inviter_id = inviter.id
```

下级用户的付费判断：

```text
exists(
  UserSubscription
  JOIN SubscriptionPlan
  WHERE UserSubscription.user_id = invitee.id
  AND SubscriptionPlan.price_amount > 0
)
AND invitee.id NOT IN excluded_users
```

### 8.2 邀请付费金额

邀请付费金额按下级用户获得的有价套餐标价计算：

```text
recognized_invitation_paid_amount = sum(invitee 的 UserSubscription 对应 SubscriptionPlan.price_amount)
```

该金额不依赖 `SubscriptionOrder.money`。

### 8.3 当前有效邀请付费金额

当前有效邀请付费金额按下级用户当前仍有效的有价套餐计算：

```text
active_invitation_paid_amount = sum(
  active UserSubscription 对应 SubscriptionPlan.price_amount
)
```

如果需要剩余价值，则复用面板一的剩余价值算法：

```text
active_invitation_remaining_value = sum(recognized_remaining_value)
```

### 8.4 下级付费用户数

```text
paid_invitee_count = count(distinct invitee.id)
```

条件：

```text
invitee 存在至少一个有价套餐
AND invitee.id NOT IN excluded_users
```

### 8.5 被排除金额

对被排除的下级用户，单独计算：

```text
excluded_invitation_paid_amount = sum(被排除下级用户的有价套餐 price_amount)
```

该字段只用于审计，不计入主统计。

### 8.6 顶部统计卡片

建议展示：

1. **邀请人数量**
2. **下级用户数量**
3. **下级付费用户数量**
4. **邀请付费总金额**：`sum(recognized_invitation_paid_amount)`
5. **当前有效付费下级数**
6. **被排除下级本应计入金额**

金额仍需按币种分组。

### 8.7 邀请人汇总表

| 字段 | 说明 |
|---|---|
| inviter_user_id | 邀请人用户 ID |
| inviter_username | 邀请人用户名 |
| invitee_count | 下级用户数 |
| paid_invitee_count | 下级付费用户数 |
| active_paid_invitee_count | 当前有效付费下级数 |
| recognized_invitation_paid_amount | 邀请付费总金额 |
| active_invitation_remaining_value | 当前有效下级套餐剩余价值 |
| excluded_invitation_paid_amount | 被排除下级本应计入金额 |
| latest_paid_subscription_time | 最近下级获得有价套餐时间 |

### 8.8 下级用户明细表

| 字段 | 说明 |
|---|---|
| invitee_user_id | 下级用户 ID |
| invitee_username | 下级用户名 |
| inviter_user_id | 邀请人用户 ID |
| registered_at | 注册时间 |
| paid_subscription_count | 历史有价套餐数 |
| active_paid_subscription_count | 当前有效有价套餐数 |
| recognized_paid_amount | 下级有价套餐总金额 |
| active_remaining_value | 当前有效有价套餐剩余价值 |
| excluded | 是否被排除 |
| excluded_reason | 排除原因 |

### 8.9 下级套餐记录表

「购买记录」在本面板中定义为：用户每次获得有价套餐的记录。

| 字段 | 说明 |
|---|---|
| subscription_id | 订阅 ID |
| invitee_user_id | 下级用户 ID |
| inviter_user_id | 邀请人用户 ID |
| plan_id | 套餐 ID |
| plan_name | 套餐名称 |
| plan_price | 套餐标价 |
| currency | 币种 |
| source | 来源 |
| grant_reason | 发放原因 |
| start_time | 开始时间 |
| end_time | 结束时间 |
| status | 当前状态 |
| recognized_paid_amount | 计入邀请付费金额 |
| recognized_remaining_value | 当前剩余价值 |
| excluded | 是否被排除 |
| possible_order_id | 可推断的订单 ID，可为空 |
| payment_provider | 可追溯支付渠道，可为空 |
| payment_method | 可追溯支付方式，可为空 |

### 8.10 订单辅助展示

如果存在可关联订单，可作为辅助信息展示：

- `possible_order_id`
- `payment_provider`
- `payment_method`
- `order_recorded_amount`
- `order_status`
- `complete_time`

关联只能作为辅助追溯。当前订单与订阅缺少显式外键时，不应将推断结果作为强事实。

### 8.11 后端接口建议

```text
GET /api/admin-analytics/invitation-paid-subscriptions/summary
GET /api/admin-analytics/invitation-paid-subscriptions/inviters
GET /api/admin-analytics/invitation-paid-subscriptions/invitees
GET /api/admin-analytics/invitation-paid-subscriptions/subscriptions
```

查询参数：

```text
start_timestamp
end_timestamp
snapshot_at
inviter_id
invitee_id
user_ids
plan_ids
sources
grant_reasons
business_codes
currency
include_excluded=false
only_excluded=false
active_only=false
limit
offset
sort_by
sort_order
```

## 9. 前端集成

### 9.1 页面位置

推荐追加到现有后台统计页面：

```text
/admin-analytics
```

新增两个 tab：

- `paid-subscription-value`
- `invitation-paid-subscriptions`

### 9.2 交互行为

两个 tab 应支持：

- 时间筛选。
- 套餐筛选。
- 用户筛选。
- 来源筛选。
- 币种筛选。
- 是否包含已排除用户。
- 表格分页、排序。
- 从汇总钻取到用户，再钻取到订阅记录。

### 9.3 文案原则

主口径应使用以下文案：

- 「付费套餐」
- 「有价套餐」
- 「计入收款」
- 「邀请付费金额」
- 「套餐剩余价值」

避免使用会造成误解的文案：

- 「权益价值」作为主指标名称。
- 「真实订单金额」作为主金额。
- 「实收金额」指向 `SubscriptionOrder.money`。

如果展示订单金额，应命名为：

- 「订单记录金额」
- 「支付订单金额」
- 「可追溯订单金额」

### 9.4 国际化

`web/default` 使用 i18next。新增前端文案必须进入：

```text
web/default/src/i18n/locales/{lang}.json
```

支持语言：

- `en`
- `zh`
- `fr`
- `ru`
- `ja`
- `vi`

## 10. 后端实现边界

### 10.1 服务层

建议新增独立统计服务，职责包括：

- 加载排除用户配置。
- 查询有效有价订阅。
- 计算剩余价值。
- 聚合套餐、用户、来源、邀请人维度。
- 返回可分页明细。

### 10.2 DTO

建议为两个面板新增明确 DTO，不复用语义不一致的订单 DTO。

DTO 字段应区分：

- `recognized_*`：按本规格主口径计入统计。
- `excluded_*`：被排除用户本应计入但未计入。
- `order_recorded_*`：仅来自订单记录的辅助金额。
- `remaining_*`：当前剩余价值。

### 10.3 数据库兼容性

所有查询必须兼容：

- SQLite
- MySQL >= 5.7.8
- PostgreSQL >= 9.6

优先使用 GORM 查询和聚合。必须写 raw SQL 时，需要遵守项目现有跨数据库规则。

## 11. 验收标准

### 11.1 付费套餐剩余价值

应满足：

- 有价套餐用户未被排除时计入统计。
- 有价套餐用户被排除时不计入主统计，但进入审计字段。
- 兑换码来源的有价套餐计入统计。
- 管理员分配的有价套餐计入统计。
- 没有 `SubscriptionOrder` 的有价套餐仍计入统计。
- 免费试用套餐不计入统计。
- token 口径、时间口径和最终取较小值计算正确。
- 示例中的 40 元套餐、33 天剩余、200M 已用 token 场景最终值为 44 元。
- 多币种金额不混加。

### 11.2 邀请付费统计

应满足：

- 下级用户持有有价套餐且未被排除时，视为邀请付费用户。
- 下级用户通过兑换码获得有价套餐时，计入邀请付费金额。
- 下级用户通过管理员分配获得有价套餐时，计入邀请付费金额。
- 下级用户有价套餐没有订单时，仍计入邀请付费金额。
- 被排除下级用户不计入主统计。
- 被排除下级用户本应计入金额进入审计字段。
- 邀请付费总金额以套餐标价为准，不以 `SubscriptionOrder.money` 为准。
- 订单记录只作为辅助追溯展示。

### 11.3 前端

应满足：

- 两个面板出现在现有后台统计页面中。
- 管理员可以查看汇总、用户明细、订阅明细。
- 管理员可以筛选是否包含已排除用户。
- 金额字段展示币种。
- 主指标文案不会把订单金额误称为实收金额。

## 12. 测试要求

后续实现时应至少覆盖以下测试：

### 12.1 剩余价值算法

- 当前周期部分 token 已使用。
- 后续存在完整重置周期。
- 最后一个周期不足完整周期，按时间折算。
- `recognized_remaining_value = min(token_based_value, time_based_value)`。
- token 超用时当前周期 token 价值为 0。
- 无限 token 或 token limit 缺失时退化为时间口径。

### 12.2 统计过滤

- 排除用户不计入主统计。
- 排除用户进入审计金额。
- 免费套餐不计入统计。
- 有价套餐不因来源不同而被排除。

### 12.3 邀请统计

- 下级用户通过订单获得有价套餐。
- 下级用户通过兑换码获得有价套餐。
- 下级用户通过管理员分配获得有价套餐。
- 下级用户被排除。
- 多个下级用户属于同一个邀请人。
- 一个下级用户拥有多个有价套餐。

## 13. 开放问题

当前规格已固定核心统计口径。仍建议后续实现前确认以下展示偏好：

1. 面板一是否默认只展示当前有效订阅，历史过期有价套餐不参与剩余价值统计。推荐答案：是。
2. 面板二的邀请付费金额是否统计历史所有有价套餐，还是默认按时间范围过滤。推荐答案：默认统计筛选时间范围内获得的有价套餐，未传时间范围时统计全部历史。
3. 月度邀请权益如果绑定有价套餐是否计入。按当前统一口径，推荐答案：计入，除非对应用户被排除。
