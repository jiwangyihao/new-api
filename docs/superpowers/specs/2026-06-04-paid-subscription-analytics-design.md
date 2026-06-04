# 付费套餐剩余价值与邀请付费统计面板设计规格

## 1. 背景

当前项目存在多种套餐获得方式：在线支付、余额购买、兑换码、管理员手动分配、售后补发、邀请奖励、邀请试用等。运营上这些来源并不都能通过 `SubscriptionOrder` 完整追溯，因此两个统计面板不能以订单是否存在作为主判断。

最终业务口径固定为：

```text
非销售赠送的有价套餐 + 用户未排除 = 已收款，计入统计。
```

其中：

- `SubscriptionPlan.price_amount > 0` 是「有价套餐」的基础条件。
- 订单购买、余额购买、兑换码销售、管理员售后分配的有价套餐都计入。
- 邀请奖励套餐不计入；即使奖励套餐绑定了有价格的 `SubscriptionPlan`，也不视为收款。
- 邀请试用、试用码等赠送 / 试用来源不计入；即使误配置了套餐价格，也不视为收款。
- `SubscriptionOrder` 只能作为辅助追溯来源，不决定是否计入统计，也不能作为主金额来源。

本规格定义两个后台统计面板：

1. **付费套餐剩余价值面板**：统计当前所有未排除用户的有效、非销售赠送有价套餐剩余价值。
2. **邀请付费统计面板**：统计邀请人名下下级用户的非销售赠送有价套餐确认金额、套餐权益明细和当前剩余价值。

## 2. 目标

### 2.1 付费套餐剩余价值面板

管理员应能看到：

- 当前所有未排除用户的付费套餐剩余价值总额。
- 每个用户、每条有效有价订阅的剩余价值。
- token 口径剩余价值、时间口径剩余价值，以及最终计入口径。
- 按套餐、来源、币种等维度聚合的剩余价值。
- 被排除用户本应贡献的金额，用于审计。

### 2.2 邀请付费统计面板

管理员应能看到：

- 每个邀请人带来的下级用户数量。
- 每个邀请人的下级付费用户数量。
- 每个邀请人的下级用户持有或持有过的非销售赠送有价套餐确认金额。
- 下级用户具体有哪些有价套餐权益记录。
- 下级用户当前有效付费套餐和剩余价值。
- 被排除用户对邀请统计的影响。

## 3. 非目标

本规格不覆盖以下内容：

- 不实现新的支付、退款、发票或财务对账系统。
- 不用 `SubscriptionOrder.money` 作为主收款口径。
- 不要求订单与订阅之间建立强一致关联。
- 不在本阶段新增复杂图表；表格、统计卡片和钻取明细优先。
- 不改变现有套餐发放、兑换码、管理员分配或邀请奖励流程。
- 不要求回填历史「每次获得」流水。现有 `UserSubscription` 是权益快照，不是一次获得一行的流水表。

## 4. 现有数据基础与限制

### 4.1 套餐与订阅

核心实体位于 `model/subscription.go`。

`SubscriptionPlan` 提供套餐定义：

- `price_amount`：套餐标价。大于 0 时视为有价套餐。
- `currency`：币种。
- `duration_unit`、`duration_value`、`custom_seconds`：套餐有效期定义。
- `monthly_token_limit`：套餐周期 token 限额。
- `quota_reset_period`、`quota_reset_custom_seconds`：额度重置周期。
- `is_trial`、`invite_trial`、`reward_eligible`、`business_code`：业务分类字段。

`UserSubscription` 提供用户实际持有的套餐权益：

- `user_id`、`plan_id`
- `amount_total`、`amount_used`
- `token_limit`、`token_used`
- `start_time`、`end_time`
- `status`
- `last_reset_time`、`next_reset_time`
- `grant_reason`、`source`

### 4.2 `UserSubscription` 不是获得流水

现有创建逻辑会在同一用户、同一套餐存在有效订阅时延长原 `UserSubscription.end_time`，而不是新增一条订阅记录。

因此：

- 一条 `UserSubscription` 可能代表同一用户同一套餐的多次购买、兑换码兑换或管理员补发。
- `source` 和 `grant_reason` 可能只反映首次创建或当前快照来源，不一定能精确代表所有被合并进来的获得来源。
- 邀请统计不能把 `UserSubscription` 行数直接等同于「购买次数」或「每次获得记录」。
- 若要精确展示历史每一次获得，后续需要新增明确的获得流水。本规格不要求补建该流水。

### 4.3 订单

`SubscriptionOrder` 记录部分购买链路：

- `user_id`、`plan_id`
- `money`
- `payment_provider`、`payment_method`
- `status`
- `create_time`、`complete_time`
- `kyren_snapshot`、`entitlement_snapshot`

订单数据只用于展示和追溯，不作为两个面板的主统计依据。

### 4.4 邀请关系

邀请关系以用户表中的邀请人字段为准：

```text
invitee.inviter_id = inviter.id
```

邀请统计面板以该关系关联下级用户，再从下级用户的 `UserSubscription + SubscriptionPlan` 计算付费金额和剩余价值。

## 5. 统一统计口径

### 5.1 有价套餐与非销售赠送例外

有价套餐的基础条件是：

```text
SubscriptionPlan.price_amount > 0
```

非销售赠送来源是自动排除项。满足以下任一条件的 `UserSubscription` 不计入两个面板的主统计，不算付费用户，也不进入「已收款」金额：

```text
UserSubscription.grant_reason IN ('monthly_invite_entitlement', 'invite_trial', 'trial_code')
OR UserSubscription.source IN ('monthly_invite_entitlement', 'invite_trial', 'trial_code')
```

说明：

- `monthly_invite_entitlement` 是用户明确确认的邀请奖励套餐例外。
- `invite_trial` 和 `trial_code` 属于试用 / 赠送来源，默认也不作为销售收款统计。
- 订单购买、余额购买、兑换码销售、管理员售后分配的有价套餐不属于非销售赠送来源，仍按主口径计入。

由于现有 `UserSubscription` 可能合并同一用户同一套餐的多次获得，不能仅凭整行 `grant_reason/source` 在所有历史数据上精确拆分奖励与非奖励时长。实现必须遵守：

- 如果存在明确的获得流水或后续新增的前向流水，按流水拆分非销售赠送确认单元和付费确认单元。
- 如果没有流水且整条快照来源为 `monthly_invite_entitlement`、`invite_trial` 或 `trial_code`，默认整条快照不计入主统计和审计金额，并返回 `source_attribution = mixed_or_unknown` 或 warning，避免把奖励 / 试用误算成收款。
- 如果没有流水且整条快照来源为订单、余额、兑换码、管理员分配等付费来源，则按快照推导确认单元计入，同时标记 `source_attribution = snapshot`。
- 不得把可能混合的快照伪装成精确购买来源。

本阶段不要求回填历史流水；允许实现增加前向获得流水，用于后续精确拆分。

该判断不依赖：

- `SubscriptionOrder.status`
- `SubscriptionOrder.money`
- `payment_provider`
- `payment_method`

### 5.2 计入统计的用户

默认计入统计的用户满足：

```text
user_id 不在 subscription_analytics.excluded_users 中
```

排除用户必须先加载成 Go map/set，再用于统计判断。禁止对空 slice 直接拼接 `NOT IN` 查询。排除列表为空时，不应追加 `NOT IN` 条件。

被排除用户的非销售赠送以外的有价套餐不计入主统计结果，但应可在审计视图中查看其本应计入的金额。邀请奖励、邀请试用和试用码套餐不进入主统计，也不进入「本应计入」审计金额。

### 5.3 已收款判断

对于两个面板，已收款判断统一为：

```text
has_paid_subscription = exists(
  UserSubscription
  JOIN SubscriptionPlan
  WHERE UserSubscription.user_id = user.id
  AND SubscriptionPlan.price_amount > 0
  AND NOT (
    UserSubscription.grant_reason IN ('monthly_invite_entitlement', 'invite_trial', 'trial_code')
    OR UserSubscription.source IN ('monthly_invite_entitlement', 'invite_trial', 'trial_code')
  )
)
AND user.id 不在 excluded_users 中
```

只要满足该条件，即视为付费用户或已收款用户。邀请奖励、邀请试用和试用码套餐不满足该条件，不进入付费用户判断。

### 5.4 金额来源

主金额来自套餐当前标价：

```text
recognized_paid_amount = SubscriptionPlan.price_amount * recognized_paid_units
```

不是：

```text
SubscriptionOrder.money
```

当前 `UserSubscription` 不保存发放时的价格和币种快照。除非后续新增独立价格快照字段，本阶段主金额以查询时关联到的 `SubscriptionPlan.price_amount` 和 `SubscriptionPlan.currency` 为准。

`SubscriptionOrder.money` 可作为辅助字段展示，例如「订单记录金额」或「支付订单金额」，但不能命名为「实收金额」，也不能决定是否计入主统计。

实现非销售赠送排除时应使用正向命中判断：只有 `grant_reason` 或 `source` 明确属于 `monthly_invite_entitlement`、`invite_trial`、`trial_code` 时才排除。空字符串或 NULL 不等同于被排除来源，不能因为 SQL 三值逻辑被误过滤。

### 5.5 多币种金额类型

不同币种不得静默相加。所有汇总金额必须返回按币种拆分的结构。

统一金额类型：

```ts
interface MoneyAmount {
  amount: number
  currency: string
}

interface MoneyBreakdown {
  amount: number
  currency: string
}
```

规则：

- 汇总字段使用 `*_by_currency: MoneyBreakdown[]`。
- 明细行金额使用 `MoneyAmount`，或显式包含 `amount + currency`。
- 即使请求传入单一 `currency`，响应结构仍保持一致，避免前端为单币种和多币种写两套逻辑。
- 金额 UI 必须展示币种代码或币种符号，不允许裸数字。

## 6. 管理员排除用户配置

### 6.1 配置位置

最终位置固定为：

```text
System Settings -> Billing & Payment -> Statistics
```

`web/default` 中新增 Billing section：

```text
section id: statistics
labelKey: systemSettings.billing.sections.statistics
route: /system-settings/billing/statistics
file: web/default/src/features/system-settings/billing/section-registry.tsx
```

该 section 需要提供 `titleKey` 和 `descriptionKey`，保存成功后刷新系统设置 query 和 `admin-analytics` 相关 query。前端 `BillingSettings` 默认项应包含 `subscription_analytics.excluded_users`，避免配置缺失时 UI 出现未定义状态。

### 6.2 配置注册

按现有 `setting/config` 模式新增配置模块，不直接绕过配置管理读写裸 option。

建议后端结构：

```go
type SubscriptionAnalyticsSetting struct {
    ExcludedUsers []SubscriptionAnalyticsExcludedUser `json:"excluded_users"`
}

type SubscriptionAnalyticsExcludedUser struct {
    UserID     int    `json:"user_id"`
    Reason     string `json:"reason"`
    ExcludedAt int64  `json:"excluded_at"`
    ExcludedBy int    `json:"excluded_by"`
}
```

注册方式：

```go
config.GlobalConfig.Register("subscription_analytics", &SubscriptionAnalyticsSetting{})
```

数据库 key 由现有配置系统生成：

```text
subscription_analytics.excluded_users
```

统计服务通过只读 accessor 获取排除用户 set。accessor 应返回 `map[int]SubscriptionAnalyticsExcludedUser` 的拷贝，避免统计代码持有可变全局 slice。

### 6.3 前端配置 UI

配置 UI 至少支持：

- 按用户 ID 或用户名添加排除用户。
- 填写排除原因。
- 显示 `excluded_at` 和 `excluded_by`。
- 移除排除用户。
- 保存后刷新系统设置 query 和 `admin-analytics` 相关 query。

### 6.4 排除筛选三态

两个面板的明细视图使用同一个 URL 枚举：

```text
excluded_mode=included_only|include_excluded|excluded_only
```

| UI 文案 | `excluded_mode` | 后端派生参数 | 行为 |
|---|---|---|---|
| 仅计入用户 | `included_only` | `include_excluded=false&only_excluded=false` | 默认。主统计和明细只包含未排除用户。 |
| 包含已排除 | `include_excluded` | `include_excluded=true&only_excluded=false` | 主统计仍只汇总未排除用户；明细同时返回排除用户，并显示审计金额。 |
| 仅已排除 | `excluded_only` | `only_excluded=true` | 只返回排除用户审计行，主统计字段应为 0 或仅返回 `excluded_*_by_currency`。 |

被排除用户行应包含：

- `excluded`
- `excluded_reason`
- `excluded_at`
- `excluded_by`
- `would_have_*_by_currency` 或 `excluded_*_by_currency`

## 7. 面板一：付费套餐剩余价值

### 7.1 统计对象

默认统计范围固定为当前有效、非销售赠送的有价订阅：

```text
UserSubscription 当前有效
AND SubscriptionPlan.price_amount > 0
AND NOT (
  UserSubscription.grant_reason IN ('monthly_invite_entitlement', 'invite_trial', 'trial_code')
  OR UserSubscription.source IN ('monthly_invite_entitlement', 'invite_trial', 'trial_code')
)
AND UserSubscription.user_id 不在 excluded_users 中
```

当前有效判断必须复用或等价于现有后台统计有效订阅作用域：

```text
status = 'active'
AND start_time <= snapshot_at
AND end_time > snapshot_at
```

历史过期有价套餐不参与面板一主统计，因为该面板统计的是当前剩余价值。

### 7.2 快照时间

所有计算以同一个 `snapshot_at` 为准。

- 管理员可显式传入 `snapshot_at`，用于复盘历史时点。
- 后端需要扩展现有 admin analytics 查询解析，支持读取 `snapshot_at`，校验 `snapshot_at >= 0`。
- 后端未收到 `snapshot_at` 时，只保证单个请求内生成并通过 `range.snapshot_at` 返回。
- 跨 summary、users、subscriptions、breakdown 的一致快照由前端按第 9.3 的快照协商流程保证。
- 响应必须通过 `range.snapshot_at` 透传实际使用的快照时间。

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

汇总值按币种返回：

```text
recognized_remaining_value_by_currency
token_based_value_by_currency
time_based_value_by_currency
```

### 7.4 时间口径

```text
time_based_value = plan_price * remaining_seconds / plan_duration_seconds
```

其中：

```text
remaining_seconds = max(end_time - snapshot_at, 0)
```

`plan_duration_seconds` 不得硬编码为固定 30 天/月或 365 天/年。必须使用与现有 `calcPlanEndTime(start, plan)` 等价的方式计算本订阅周期长度。

对于 month/year 套餐，周期长度应按真实 `AddDate` 后的起止时间差计算。

### 7.5 token 口径

token 口径必须考虑额度重置周期，不能只看当前周期剩余 token。

额度周期切分必须复用或抽取现有重置语义，包括：

- `calcNextResetTime`
- `NormalizeResetPeriod`
- `quota_reset_period`
- `quota_reset_custom_seconds`
- 现有月度、周度、日度、自定义周期边界

每个额度周期的价值必须按该额度周期占套餐权益周期的比例分摊，不能把每个 daily/weekly/custom 重置周期都当作完整套餐价格：

```text
cycle_value = plan_price * cycle_seconds / plan_duration_seconds
```

当额度重置周期与套餐周期一致时，`cycle_value = plan_price`，因此仍符合第 7.6 的示例。

当前周期：

```text
current_cycle_token_ratio = max(token_limit - token_used, 0) / token_limit
current_cycle_value = cycle_value * current_cycle_token_ratio
```

完整未来周期：

```text
full_future_cycle_value = cycle_value
```

最后短周期：

```text
partial_future_cycle_value = plan_price * partial_cycle_seconds / plan_duration_seconds
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

#### 7.7.3 不重置的有限 token 套餐

当 `quota_reset_period = never` 且 `token_limit > 0` 时，不存在未来重置周期：

```text
token_based_value = plan_price * max(token_limit - token_used, 0) / token_limit
```

最终仍取：

```text
recognized_remaining_value = min(token_based_value, time_based_value)
```

#### 7.7.4 套餐价格小于等于 0

不计入主统计。

#### 7.7.5 被排除用户

不计入主统计，但可在审计视图中返回：

```text
excluded = true
would_have_remaining_value = 原本应计入的剩余价值
```

### 7.8 顶部统计卡片

建议展示：

1. **付费套餐剩余价值**：`recognized_remaining_value_by_currency`
2. **token 口径剩余价值**：`token_based_value_by_currency`，无法计算 token 口径的订阅单独计数。
3. **时间口径剩余价值**：`time_based_value_by_currency`
4. **有效有价订阅数**
5. **当前有效付费用户数**：第 7.1 范围内的 distinct user 数。
6. **已排除用户本应计入金额**：`excluded_remaining_value_by_currency`

### 7.9 聚合表

#### 按套餐聚合

| 字段 | 说明 |
|---|---|
| plan_id | 套餐 ID |
| plan_name | 套餐名称 |
| plan_business_code | 套餐业务编码 |
| active_user_count | 有效付费用户数 |
| active_subscription_count | 有效有价订阅数 |
| recognized_remaining_value_by_currency | 付费套餐剩余价值 |
| token_based_value_by_currency | token 口径剩余价值 |
| time_based_value_by_currency | 时间口径剩余价值 |
| excluded_remaining_value_by_currency | 已排除用户本应计入金额 |
| average_token_usage_ratio | 平均 token 使用率 |

#### 按来源聚合

来源只用于解释套餐来源，不影响是否计入。

| 字段 | 说明 |
|---|---|
| source | 订阅来源 |
| grant_reason | 发放原因 |
| user_count | 用户数 |
| subscription_count | 订阅数 |
| recognized_remaining_value_by_currency | 付费套餐剩余价值 |
| excluded_remaining_value_by_currency | 已排除用户本应计入金额 |
| source_attribution | `exact`、`snapshot` 或 `mixed_or_unknown` |

当同一订阅快照可能由多次不同来源合并而来时，来源聚合应标记 `source_attribution = snapshot` 或 `mixed_or_unknown`，不能伪装成精确购买来源。

### 7.10 用户明细表

| 字段 | 说明 |
|---|---|
| user_id | 用户 ID |
| username | 用户名 |
| display_name | 展示名称 |
| active_paid_plan_count | 当前有效有价套餐数 |
| recognized_remaining_value_by_currency | 付费套餐剩余价值 |
| token_based_value_by_currency | token 口径剩余价值 |
| time_based_value_by_currency | 时间口径剩余价值 |
| earliest_end_time | 最近到期时间 |
| excluded | 是否被排除 |
| excluded_reason | 排除原因 |
| excluded_at | 排除时间 |
| excluded_by | 排除操作人 |
| would_have_remaining_value_by_currency | 被排除用户本应计入金额 |

### 7.11 订阅明细表

| 字段 | 说明 |
|---|---|
| subscription_id | 订阅 ID |
| user_id | 用户 ID |
| username | 用户名 |
| plan_id | 套餐 ID |
| plan_name | 套餐名称 |
| source | 来源 |
| grant_reason | 发放原因 |
| plan_price | `MoneyAmount`，套餐标价 |
| start_time | 开始时间 |
| end_time | 结束时间 |
| remaining_seconds | 剩余秒数 |
| token_limit | 当前周期 token 限额 |
| token_used | 当前周期已用 token |
| next_reset_time | 下次重置时间 |
| token_based_value | `MoneyAmount` 或 null |
| time_based_value | `MoneyAmount` |
| recognized_remaining_value | `MoneyAmount` |
| valuation_basis | `token_and_time`、`time_only`、`token_never_reset` 等 |
| source_attribution | `exact`、`snapshot` 或 `mixed_or_unknown` |
| excluded | 是否被排除 |
| excluded_reason | 排除原因 |

### 7.12 后端接口

```text
GET /api/admin-analytics/paid-subscription-value/summary
GET /api/admin-analytics/paid-subscription-value/users
GET /api/admin-analytics/paid-subscription-value/subscriptions
GET /api/admin-analytics/paid-subscription-value/breakdown/plans
GET /api/admin-analytics/paid-subscription-value/breakdown/sources
```

查询参数：

```text
snapshot_at
plan_ids
user_ids
subscription_id
sources
grant_reasons
business_codes
currency
excluded_mode=included_only|include_excluded|excluded_only
limit
offset
sort_by
sort_order
```

后端可由 `excluded_mode` 派生 `include_excluded` 和 `only_excluded`，不再暴露含糊的 `include` 值。

### 7.13 排序白名单

新增列表接口必须使用 sort 白名单，未知 `sort_by` 返回 HTTP 400。

| 端点 | 允许 sort_by |
|---|---|
| users | `recognized_remaining_value`、`active_paid_plan_count`、`earliest_end_time`、`user_id` |
| subscriptions | `recognized_remaining_value`、`end_time`、`start_time`、`plan_price`、`subscription_id` |
| breakdown/plans | `recognized_remaining_value`、`subscription_count`、`user_count`、`plan_id` |
| breakdown/sources | `recognized_remaining_value`、`subscription_count`、`user_count`、`source`、`grant_reason` |

金额类 sort_by（如 `recognized_remaining_value`、`plan_price`）必须指定 `currency`。后端排序值只取该币种金额；该行没有对应币种金额时按 0 处理。未指定 `currency` 时请求金额类 sort_by 必须返回 HTTP 400，避免跨币种静默相加或不稳定排序。非金额字段不要求指定 `currency`。

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
  AND NOT (
    UserSubscription.grant_reason IN ('monthly_invite_entitlement', 'invite_trial', 'trial_code')
    OR UserSubscription.source IN ('monthly_invite_entitlement', 'invite_trial', 'trial_code')
  )
)
AND invitee.id 不在 excluded_users 中
```

邀请奖励、邀请试用和试用码套餐是显式排除项：即使它们绑定有价套餐且用户未被排除，也不计入邀请付费金额、下级付费用户数或当前有效下级套餐剩余价值。

### 8.2 历史邀请付费金额

由于 `UserSubscription` 不是获得流水，邀请付费金额不能简单按 `UserSubscription` 行数求和。

本阶段定义「有价套餐确认单元」用于计算历史邀请付费金额：

```text
recognized_invitation_paid_amount = sum(
  SubscriptionPlan.price_amount * recognized_paid_units
)
```

`recognized_paid_units` 的计算规则：

1. 只对非销售赠送来源的有价 `UserSubscription` 或明确获得流水计算确认单元。
2. `monthly_invite_entitlement`、`invite_trial`、`trial_code` 确认单元不计入 `recognized_paid_units`，也不进入排除用户审计金额。
3. 如果存在明确获得流水，按流水拆分付费确认单元和非销售赠送确认单元。
4. 如果没有流水，则将非销售赠送来源的 `UserSubscription` 快照整体排除；将订单、余额、兑换码、管理员售后等付费来源快照作为 `snapshot` 推导。
5. 对可推导的付费快照，使用与 `calcPlanEndTime(start, plan)` 等价的方式，从 `start_time` 开始逐段推进到 `end_time`。
6. 每推进一个完整套餐周期，计 1 个确认单元。
7. 如果最后剩余时间不足一个完整套餐周期，按剩余时长占该周期时长的比例计入小数确认单元，并标记为推导值。
8. 只有在周期无法可靠推导、总时长异常但存在非销售赠送以外的有价快照时，才使用 `snapshot_minimum = 1`。可可靠推导的短周期不强制提升到 1 个整包。

返回字段必须包含：

- `recognized_paid_units`
- `recognized_paid_amount`
- `unit_inference_basis`

`unit_inference_basis` 取值：

| 值 | 含义 |
|---|---|
| `period_aligned` | 订阅时长刚好由完整套餐周期组成。 |
| `period_fraction` | 包含不足完整周期的尾段，金额有按时长比例推导。 |
| `snapshot_minimum` | 无法可靠推导周期，但因非销售赠送以外的有价套餐快照存在，按 1 个确认单元计。 |

该口径仍然只使用套餐价格作为主金额，不使用 `SubscriptionOrder.money` 决定是否计入。

### 8.3 时间范围语义

面板二同时存在历史金额和当前剩余价值，时间参数必须明确：

- `start_timestamp` / `end_timestamp` 过滤套餐确认单元的获得时间。
- 未传 `start_timestamp` / `end_timestamp` 时，历史邀请付费金额统计全部历史。
- 面板二必须使用专用查询解析语义；不得复用现有默认最近 30 天和 365 天最大范围限制来截断历史金额。
- 如果后端仍对自定义时间范围设置最大跨度限制，该限制只适用于用户显式传入的时间范围；未传时间范围表示 all-history。
- `snapshot_at` 只用于判断当前有效套餐和计算当前剩余价值。
- `active_only=true` 只过滤明细或当前有效字段，不改变历史总额定义，除非接口文档明确该端点是 active-only 明细端点。

获得时间推导规则：

- 对 period-aligned 的确认单元，获得时间为每个推导周期的起始时间。
- 对 period-fraction 的尾段，获得时间为尾段起始时间。
- 对 snapshot-minimum，获得时间使用 `UserSubscription.start_time`。

### 8.4 当前有效邀请付费金额和剩余价值

当前有效邀请付费金额按下级用户当前仍有效、非销售赠送的有价套餐快照计算：

```text
active_invitation_paid_amount = sum(
  当前有效且非销售赠送的 UserSubscription 对应 SubscriptionPlan.price_amount
)
```

当前有效邀请剩余价值复用面板一算法：

```text
active_invitation_remaining_value = sum(recognized_remaining_value)
```

两者都按币种分组返回。

### 8.5 下级付费用户数

```text
paid_invitee_count = count(distinct invitee.id)
```

条件：

```text
invitee 存在至少一个非销售赠送的有价套餐
AND invitee.id 不在 excluded_users 中
```

当前有效付费下级数另行计算：

```text
active_paid_invitee_count = count(distinct invitee.id with active non-gift paid subscription at snapshot_at)
```

### 8.6 被排除金额

对被排除的下级用户，单独计算：

```text
excluded_invitation_paid_amount_by_currency
excluded_active_remaining_value_by_currency
```

这些字段只用于审计，不计入主统计。非销售赠送以外的有价套餐才进入排除审计金额；邀请奖励、邀请试用和试用码套餐不进入审计金额。

### 8.7 顶部统计卡片

建议展示：

1. **邀请人数量**
2. **下级用户数量**
3. **下级付费用户数量**
4. **邀请付费总金额**：`recognized_invitation_paid_amount_by_currency`
5. **当前有效付费下级数**
6. **当前有效邀请付费金额**：`active_invitation_paid_amount_by_currency`
7. **当前有效下级套餐剩余价值**：`active_invitation_remaining_value_by_currency`
8. **被排除下级本应计入金额**：`excluded_invitation_paid_amount_by_currency`
9. **被排除下级当前剩余价值**：`excluded_active_remaining_value_by_currency`

### 8.8 邀请人汇总表

| 字段 | 说明 |
|---|---|
| inviter_user_id | 邀请人用户 ID |
| inviter_username | 邀请人用户名 |
| invitee_count | 下级用户数 |
| paid_invitee_count | 下级付费用户数 |
| active_paid_invitee_count | 当前有效付费下级数 |
| recognized_invitation_paid_amount_by_currency | 邀请付费总金额 |
| active_invitation_paid_amount_by_currency | 当前有效邀请付费金额 |
| active_invitation_remaining_value_by_currency | 当前有效下级套餐剩余价值 |
| excluded_invitation_paid_amount_by_currency | 被排除下级本应计入金额 |
| excluded_active_remaining_value_by_currency | 被排除下级当前有效套餐本应计入剩余价值 |
| latest_paid_subscription_time | 最近下级获得有价套餐时间 |
| drilldown | 指向该邀请人的下级明细 |

### 8.9 下级用户明细表

| 字段 | 说明 |
|---|---|
| invitee_user_id | 下级用户 ID |
| invitee_username | 下级用户名 |
| inviter_user_id | 邀请人用户 ID |
| registered_at | 注册时间 |
| paid_subscription_snapshot_count | 历史有价订阅快照数 |
| recognized_paid_units | 推导出的有价套餐确认单元数 |
| active_paid_subscription_count | 当前有效有价套餐数 |
| recognized_paid_amount_by_currency | 下级有价套餐确认金额 |
| active_remaining_value_by_currency | 当前有效有价套餐剩余价值 |
| active_paid_amount_by_currency | 当前有效有价套餐金额 |
| excluded | 是否被排除 |
| excluded_reason | 排除原因 |
| excluded_at | 排除时间 |
| excluded_by | 排除操作人 |
| would_have_paid_amount_by_currency | 被排除下级本应计入金额 |
| would_have_active_remaining_value_by_currency | 被排除下级当前有效套餐本应计入剩余价值 |
| drilldown | 指向该下级的套餐权益记录 |

### 8.10 下级套餐权益记录表

本面板中的「购买记录」在当前数据模型下展示为「有价套餐权益记录」。它是统计主口径的来源，但不声称一行就是一次交易或一次发放。

| 字段 | 说明 |
|---|---|
| subscription_id | 订阅 ID |
| invitee_user_id | 下级用户 ID |
| inviter_user_id | 邀请人用户 ID |
| plan_id | 套餐 ID |
| plan_name | 套餐名称 |
| plan_price | `MoneyAmount`，套餐标价 |
| recognized_paid_units | 推导确认单元数 |
| recognized_paid_amount | `MoneyAmount`，计入邀请付费金额 |
| unit_inference_basis | `period_aligned`、`period_fraction`、`snapshot_minimum` |
| source | 订阅快照来源 |
| grant_reason | 订阅快照发放原因 |
| source_attribution | `exact`、`snapshot` 或 `mixed_or_unknown` |
| start_time | 开始时间 |
| end_time | 结束时间 |
| status | 当前状态 |
| recognized_remaining_value | `MoneyAmount`，当前剩余价值；非当前有效时为 0 或 null |
| excluded | 是否被排除 |
| excluded_reason | 排除原因 |
| possible_order_id | 可推断的订单 ID，可为空 |
| payment_provider | 可追溯支付渠道，可为空 |
| payment_method | 可追溯支付方式，可为空 |
| order_recorded_amount | `MoneyAmount`，可追溯订单记录金额，可为空 |

### 8.11 订单辅助展示

如果存在可关联订单，可作为辅助信息展示：

- `possible_order_id`
- `payment_provider`
- `payment_method`
- `order_recorded_amount`
- `order_status`
- `complete_time`

关联只能作为 best-effort debug 信息。当前订单与订阅缺少显式外键时：

- 多订单匹配时按稳定顺序选择最可信的一条，建议候选订单先限定同 `user_id + plan_id`，再按 `complete_time DESC, id DESC` 排序；无法可信匹配时返回空。
- 不得用推断订单金额覆盖套餐确认金额。
- 不得用订单是否存在决定是否计入。

### 8.12 后端接口

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
subscription_id
sources
grant_reasons
business_codes
currency
excluded_mode=included_only|include_excluded|excluded_only
active_only=false
limit
offset
sort_by
sort_order
```

面板二接口必须使用第 8.3 的专用 all-history 时间范围语义。未提供 `start_timestamp/end_timestamp` 时，不应套用现有后台统计默认最近 30 天。

后端可由 `excluded_mode` 派生 `include_excluded` 和 `only_excluded`。

### 8.13 排序白名单

新增列表接口必须使用 sort 白名单，未知 `sort_by` 返回 HTTP 400。

| 端点 | 允许 sort_by |
|---|---|
| inviters | `recognized_invitation_paid_amount`、`active_invitation_paid_amount`、`active_invitation_remaining_value`、`paid_invitee_count`、`active_paid_invitee_count`、`inviter_user_id` |
| invitees | `recognized_paid_amount`、`active_remaining_value`、`paid_subscription_snapshot_count`、`registered_at`、`invitee_user_id` |
| subscriptions | `recognized_paid_amount`、`recognized_remaining_value`、`start_time`、`end_time`、`plan_price`、`subscription_id` |

金额类 sort_by（如 `recognized_invitation_paid_amount`、`active_invitation_paid_amount`、`active_invitation_remaining_value`、`recognized_paid_amount`、`active_remaining_value`、`plan_price`、`recognized_remaining_value`）必须指定 `currency`。后端排序值只取该币种金额；该行没有对应币种金额时按 0 处理。未指定 `currency` 时请求金额类 sort_by 必须返回 HTTP 400。非金额字段不要求指定 `currency`。

## 9. 前端集成

### 9.1 页面位置与 tab 契约

两个面板追加到现有后台统计页面：

```text
/admin-analytics
```

不替换现有 `invitations` tab。

新增 tab：

| id | labelKey | 中文展示名 | 英文展示名 | 建议位置 |
|---|---|---|---|---|
| `paid-subscription-value` | `adminAnalytics.tabs.paidSubscriptionValue` | 付费套餐剩余价值 | Paid Subscription Value | `quota` 之后 |
| `invitation-paid-subscriptions` | `adminAnalytics.tabs.invitationPaidSubscriptions` | 邀请付费 | Invitation Paid Subscriptions | 现有 `invitations` 之后 |

### 9.2 路由搜索参数

需要扩展现有 `AdminAnalyticsSearch` / canonical filters。

新增或明确使用的参数：

```text
snapshot_at
currency
excluded_mode
active_only
user_ids
plan_ids
inviter_id
invitee_id
subscription_id
```

语义：

- 面板一使用 `snapshot_at` 做单点剩余价值计算。
- 面板一请求默认不发送 `start_timestamp/end_timestamp`，不能被现有最近 30 天默认值限制。
- 面板二使用 `start_timestamp/end_timestamp` 过滤套餐确认单元获得时间。
- 面板二初始请求默认不发送 `start_timestamp/end_timestamp`，表示全部历史；只有用户显式设置时间范围后才发送。
- `subscription_id` 进入 URL 后必须序列化到 `paid-subscription-value/subscriptions` 和 `invitation-paid-subscriptions/subscriptions` 请求；其他 endpoint 可忽略该参数。若实现阶段发现当前 search schema 暂不支持精确订阅筛选，必须显式走第 9.5 的退化规则，而不是静默丢弃 URL 参数。
- 前端搜索状态必须记录时间范围是否由用户显式设置，避免 URL 恢复后把默认最近 30 天误当成业务过滤。
- 面板二使用 `snapshot_at` 计算当前有效和当前剩余价值。
- `excluded_mode` 映射第 6.4 的三态排除筛选。

### 9.3 请求模式与快照协商

前端继续使用现有模式：

```ts
ApiResponse<AdminAnalyticsPanelResponse<T>>
```

其中：

```ts
interface AdminAnalyticsPanelResponse<T> {
  range: {
    start_timestamp: number
    end_timestamp: number
    snapshot_at: number
  }
  data: T
  warnings?: AdminAnalyticsAvailabilityWarning[]
}
```

两个新面板仍复用现有非空 `range` 结构：面板一未发送 `start_timestamp/end_timestamp` 时，后端返回 `range.start_timestamp = 0`、`range.end_timestamp = range.snapshot_at`；面板二未发送 `start_timestamp/end_timestamp` 时，后端返回 `range.start_timestamp = 0`、`range.end_timestamp = range.snapshot_at`，表示 all-history 截止到快照时刻。用户显式传入时间范围时，`range.start_timestamp/end_timestamp` 回显归一化后的请求范围。

两个新 tab 使用多 endpoint 加载，但必须先协商快照：

1. 前端首次进入 tab 时先请求 `summary`。
2. 如果 URL 中没有 `snapshot_at`，前端读取 `summary.range.snapshot_at`，写入 canonical filters / URL。
3. 后续 `users`、`subscriptions`、`breakdown`、`inviters`、`invitees` 等请求必须携带同一个 `snapshot_at`。
4. 如果 URL 中已有 `snapshot_at`，所有 endpoint 直接使用该值。

endpoint：

- `paid-subscription-value/summary`
- `paid-subscription-value/breakdown/plans`
- `paid-subscription-value/breakdown/sources`
- `paid-subscription-value/users`
- `paid-subscription-value/subscriptions`
- `invitation-paid-subscriptions/summary`
- `invitation-paid-subscriptions/inviters`
- `invitation-paid-subscriptions/invitees`
- `invitation-paid-subscriptions/subscriptions`

### 9.4 DTO 示例

#### 9.4.1 付费套餐剩余价值 summary

```ts
interface PaidSubscriptionValueSummary {
  recognized_remaining_value_by_currency: MoneyBreakdown[]
  token_based_value_by_currency: MoneyBreakdown[]
  time_based_value_by_currency: MoneyBreakdown[]
  excluded_remaining_value_by_currency: MoneyBreakdown[]
  active_paid_subscription_count: number
  active_paid_user_count: number
  token_value_unavailable_count: number
}
```

#### 9.4.2 付费套餐剩余价值 response

```ts
interface PaidSubscriptionValueResponse {
  summary: PaidSubscriptionValueSummary
  plan_breakdown: AdminAnalyticsList<PaidSubscriptionValuePlanGroup>
  source_breakdown: AdminAnalyticsList<PaidSubscriptionValueSourceGroup>
  users: AdminAnalyticsList<PaidSubscriptionValueUser>
  subscriptions: AdminAnalyticsList<PaidSubscriptionValueSubscription>
}
```

实际实现可以按 endpoint 拆分，但字段语义保持一致。

#### 9.4.3 邀请付费 summary

```ts
interface InvitationPaidSubscriptionsSummary {
  inviter_count: number
  invitee_count: number
  paid_invitee_count: number
  active_paid_invitee_count: number
  recognized_invitation_paid_amount_by_currency: MoneyBreakdown[]
  active_invitation_paid_amount_by_currency: MoneyBreakdown[]
  active_invitation_remaining_value_by_currency: MoneyBreakdown[]
  excluded_invitation_paid_amount_by_currency: MoneyBreakdown[]
  excluded_active_remaining_value_by_currency: MoneyBreakdown[]
}
```

#### 9.4.4 邀请付费 response

```ts
interface InvitationPaidSubscriptionsResponse {
  summary: InvitationPaidSubscriptionsSummary
  inviters: AdminAnalyticsList<InvitationPaidInviter>
  invitees: AdminAnalyticsList<InvitationPaidInvitee>
  subscriptions: AdminAnalyticsList<InvitationPaidSubscriptionRecord>
}
```

### 9.5 钻取与信息架构

面板一布局：

```text
summary cards
-> plan/source breakdown
-> user table
-> subscription detail table or drawer
```

面板二布局：

```text
summary cards
-> inviter table
-> invitee table
-> invitee subscription records
```

建议新增 drilldown kind：

| kind | 说明 |
|---|---|
| `paid_subscription_value_user` | 面板一用户明细 |
| `paid_subscription_value_subscription` | 面板一订阅明细 |
| `invitation_paid_inviter` | 面板二邀请人下级明细 |
| `invitation_paid_invitee` | 面板二下级套餐权益记录 |

现有 `AdminAnalyticsDrilldownTarget` 需要扩展 `invitee_id`、`subscription_id` 和上述新 kind。映射规则：

| kind | payload | route search 更新 |
|---|---|---|
| `paid_subscription_value_user` | `user_id` | `tab=paid-subscription-value`，`user_ids=[user_id]` |
| `paid_subscription_value_subscription` | `subscription_id`、`user_id`、`plan_id` | `tab=paid-subscription-value`，设置 `subscription_id`；若现有 search 暂不支持，则退化为 `user_ids=[user_id]&plan_ids=[plan_id]` |
| `invitation_paid_inviter` | `inviter_id` | `tab=invitation-paid-subscriptions&inviter_id=<id>` |
| `invitation_paid_invitee` | `invitee_id`、`inviter_id` | `tab=invitation-paid-subscriptions&inviter_id=<id>&invitee_id=<id>` |

钻取必须保留当前 filters：

- `snapshot_at`
- `start_timestamp/end_timestamp`
- `currency`
- `excluded_mode`
- `plan_ids`
- `sources`
- `grant_reasons`
- `business_codes`
- `inviter_id`
- `invitee_id`

`isSupportedDrilldownTarget` 和 `buildAdminAnalyticsDrilldown` 必须同步支持这些 kind，并保留当前 filters。

### 9.6 文案原则

主口径应使用以下文案：

- 「付费套餐」
- 「有价套餐」
- 「计入收款」
- 「邀请付费金额」
- 「套餐剩余价值」
- 「有价套餐权益记录」

避免使用会造成误解的文案：

- 「权益价值」作为主指标名称。
- 「真实订单金额」作为主金额。
- 「实收金额」指向 `SubscriptionOrder.money`。
- 「每次购买记录」指向 `UserSubscription` 快照行。

如果展示订单金额，应命名为：

- 「订单记录金额」
- 「支付订单金额」
- 「可追溯订单金额」

金额格式化应新增或复用统一 helper 处理 `MoneyAmount` / `MoneyBreakdown[]`：空数组显示为 `—`，0 金额显示币种和 0，订单辅助金额缺失显示 `—`。

### 9.7 国际化

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

后台统计页文案使用 `adminAnalytics.*` key；系统设置导航和表单文案使用 `systemSettings.*` 或现有系统设置命名约定。常量中的 labelKey 必须登记到 `src/i18n/static-keys.ts`，或以 `t('...')` 字面量形式出现，确保 i18n 工具能扫描。新增 tab 的标题不得用 `adminAnalytics.tabs.${tab}` 从 hyphen id 直接拼接；`ADMIN_ANALYTICS_TABS` 应作为 tab id 到 labelKey 的唯一来源，面板标题也应读取该配置。

实现验收时必须在 `web/default/` 运行：

```text
bun run i18n:sync
```

并确认没有缺失 key。

## 10. 后端实现边界

### 10.1 代码位置

建议沿用现有后台统计体系：

- controller：`controller/admin_analytics.go`
- model/service 查询计算：优先放在现有 admin analytics 模块附近，或新增专用文件但保持同包模式。
- DTO：`dto/admin_analytics.go`
- 路由：`router/api-router.go` 中 `/api/admin-analytics` AdminAuth group。

### 10.2 响应 envelope 与错误格式

新增接口返回外层保持现有格式：

```go
dto.AdminAnalyticsPanelResponse[T]
```

通过现有成功响应封装返回。

非法参数统一：

```text
HTTP 400
{ success: false, message: "..." }
```

### 10.3 DTO 字段命名

DTO 字段应区分：

- `recognized_*`：按本规格主口径计入统计。
- `excluded_*`：被排除用户本应计入但未计入。
- `would_have_*`：单个排除用户或排除行本应计入的金额。
- `order_recorded_*`：仅来自订单记录的辅助金额。
- `remaining_*`：当前剩余价值。
- `*_by_currency`：按币种拆分的汇总金额。

金额 `amount` 沿用当前后端 `SubscriptionPlan.price_amount` 的 `float64` 金额口径，不在本规格中改为整数分。

### 10.4 有效订阅作用域

有效订阅必须复用或等价于现有后台统计作用域：

```text
applyAdminActiveSubscriptionScope(status='active' AND start_time<=snapshot AND end_time>snapshot)
```

如果实现中抽取公用函数，面板一和面板二当前有效统计应共用同一函数。

### 10.5 数据库兼容性

所有查询必须兼容：

- SQLite
- MySQL >= 5.7.8
- PostgreSQL >= 9.6

优先使用 GORM 查询和 Go 内存聚合。必须写 raw SQL 时，需要遵守项目现有跨数据库规则。

订单辅助关联不得依赖：

- PostgreSQL-only `DISTINCT ON`
- 数据库专用 JSON 运算符
- 不可移植窗口函数
- MySQL-only 或 PostgreSQL-only 日期函数

### 10.6 现有重置逻辑复用

剩余价值算法不得另写一套与业务消费不一致的自然月算法。实现应复用或抽取现有函数：

- 套餐结束时间：与 `calcPlanEndTime` 等价。
- 下次重置时间：与 `calcNextResetTime` 等价。
- reset period 标准化：与现有 `NormalizeResetPeriod` 等价。

## 11. 验收标准

### 11.1 付费套餐剩余价值

应满足：

- 非销售赠送的有价套餐用户未被排除时计入统计。
- 非销售赠送以外的有价套餐用户被排除时不计入主统计，但进入审计字段。
- 兑换码来源的有价套餐计入统计。
- 管理员分配的有价套餐计入统计。
- 没有 `SubscriptionOrder` 的有价套餐仍计入统计。
- 免费试用、邀请试用、试用码和邀请奖励套餐不计入统计。
- token 口径、时间口径和最终取较小值计算正确。
- 示例中的 40 元套餐、33 天剩余、200M 已用 token 场景最终值为 44 元。
- 多币种金额不混加，汇总字段均按币种返回。
- 同一用户多个有效有价订阅可以正确聚合。

### 11.2 邀请付费统计

应满足：

- 下级用户持有非销售赠送的有价套餐且未被排除时，视为邀请付费用户。
- 下级用户通过兑换码获得有价套餐时，计入邀请付费金额。
- 下级用户通过管理员分配获得有价套餐时，计入邀请付费金额。
- 下级用户有价套餐没有订单时，仍计入邀请付费金额。
- 被排除下级用户的非销售赠送以外的有价套餐不计入主统计。
- 被排除下级用户的非销售赠送以外的本应计入金额进入审计字段。
- 邀请付费总金额以套餐标价和推导确认单元为准，不以 `SubscriptionOrder.money` 为准。
- 订单记录只作为辅助追溯展示。
- 同一下级同一有价套餐被多次续期、兑换或管理员补发时，不得只按一条 `UserSubscription` 计一次；必须按第 8.2 的确认单元推导口径计入，并返回 `unit_inference_basis`。
- 混合来源延长同一订阅时，来源展示必须标记 `snapshot` 或 `mixed_or_unknown`，不得伪装成精确来源。

### 11.3 前端

应满足：

- 两个 tab 出现在现有 `/admin-analytics`。
- 不替换现有 `invitations` tab。
- URL search 能保留 `snapshot_at`、`currency`、`excluded_mode`、`inviter_id`、`invitee_id` 等过滤条件。
- 管理员可以查看汇总、用户明细、订阅明细、邀请人明细、下级明细。
- 管理员可以使用排除用户三态筛选。
- 金额字段均展示币种，不混加不同币种。
- 被排除用户行显示排除原因和本应计入金额。
- 订单金额只显示为订单记录金额、支付订单金额或可追溯订单金额。
- 主指标文案不会把订单金额误称为实收金额。
- 所有新增文案完成六语言 i18n。

## 12. 测试要求

### 12.1 剩余价值算法

- 当前周期部分 token 已使用。
- 后续存在完整重置周期。
- 最后一个周期不足完整周期，按时间折算。
- `recognized_remaining_value = min(token_based_value, time_based_value)`。
- token 超用时当前周期 token 价值为 0。
- 无限 token 或 token limit 缺失时退化为时间口径。
- 不重置的有限 token 套餐没有未来重置周期。
- month/year 套餐按真实日历周期计算，不硬编码 30 天/月。
- 套餐有效期与额度重置周期不一致时，token 周期价值按周期占套餐周期比例折算；例如 30 天套餐每日或每周重置时，未来每日 / 每周周期不能按完整套餐价高估。

### 12.2 统计过滤

- 排除用户不计入主统计。
- 排除用户进入审计金额。
- 排除列表为空时主统计正常返回，不能因空 `NOT IN` 变为空。
- 免费套餐、邀请试用、试用码和邀请奖励套餐不计入统计。
- 被排除用户持有邀请试用、试用码或邀请奖励套餐时，主统计和 `excluded_*` / `would_have_*` 审计金额均为 0。
- 非销售赠送的有价套餐不因订单、余额、兑换码、管理员等来源不同而被排除。
- 多币种汇总按币种拆分。

### 12.3 邀请统计

- 下级用户通过订单获得有价套餐。
- 下级用户通过兑换码获得有价套餐。
- 下级用户通过管理员分配获得有价套餐。
- 下级用户获得邀请奖励、邀请试用或试用码套餐时，即使套餐有价格，也不计入邀请付费金额、付费用户数或剩余价值。
- 下级用户被排除。
- 多个下级用户属于同一个邀请人。
- 一个下级用户拥有多个不同有价套餐。
- 同一下级同一有价套餐在 active 期间再次通过订单、兑换码、管理员分配获得，`UserSubscription` 被合并或延长时，邀请付费金额不得只计一次。
- 合并或延长订阅返回正确的 `recognized_paid_units` 和 `unit_inference_basis`。
- 混合来源延长时，来源聚合或明细必须按 `snapshot` / `mixed_or_unknown` 展示，不得静默归到第一次来源。

### 12.4 前端

- 新 tab 路由和 search 参数解析正确。
- 筛选条件刷新后可从 URL 恢复。
- 多 endpoint 加载时 loading、error、empty 状态正确。
- 三态排除筛选行为正确。
- 金额格式化带币种。
- 所有新增 i18n key 在 `en`、`zh`、`fr`、`ru`、`ja`、`vi` 中存在。

## 13. 已确认决策

1. 面板一主统计只展示当前有效、非销售赠送的有价订阅，历史过期有价套餐不参与剩余价值统计。
2. 面板二历史邀请付费金额默认统计全部历史；传入 `start_timestamp/end_timestamp` 时，按套餐确认单元获得时间过滤。
3. 邀请奖励套餐不计入两个面板。即使奖励套餐绑定有价格的 `SubscriptionPlan`，也不视为收款，不计入付费用户、邀请付费金额、套餐剩余价值或排除用户审计金额。
4. 邀请试用和试用码套餐按非销售赠送来源处理，即使误配置了套餐价格，也不计入两个面板。
5. `SubscriptionOrder` 不决定是否计入统计，订单金额只作辅助追溯。
6. 当前阶段不补建历史获得流水；邀请付费明细基于有价套餐权益快照和确认单元推导。
