# 管理员侧综合数据分析中心规格说明

## 背景

当前站点的付费体系已经转向以套餐为主导，但管理员能看到的套餐运营信息仍然有限。现有用户侧 `/usage-analytics` 解决的是单个用户查看自身 API 调用用量的问题；管理员需要的是全站视角，覆盖套餐结构、配额消耗、用户生命周期、订阅转化、邀请奖励、调用消耗和风险异常。

本规格定义独立的管理员侧「综合数据分析中心」（Operations Analytics）。它不是用户侧 Usage Analytics 的管理员模式，也不替代现有用户、套餐、日志等管理页面，而是提供聚合分析、趋势洞察、排行和 drilldown 入口。

## 设计原则

### Token-quota 语义优先

所有核心指标围绕套餐配额和 token 使用情况：

- `token_limit`
- `token_used`
- `remaining_tokens`
- `usage_rate`
- subscription status
- grant reason
- invitation entitlement

不得把页面重新设计成价格、余额或 runway 驱动的分析页。以下指标不作为核心分析内容：

- 余额
- 可用天数
- runway
- 收入预测
- 钱包消耗口径
- 用户侧价格感知指标

如果后续需要展示订单金额，只能作为后台运营辅助字段，不能用于推导「还能用几天」等余额导向指标。

### 管理员看全站，用户侧看自己

| 页面 | 视角 | 数据范围 |
|---|---|---|
| `/usage-analytics` | 普通用户 | 当前登录用户自己的调用用量 |
| `/admin-analytics` | 管理员 | 全站用户、订阅、套餐、邀请奖励、调用消耗 |

管理员进入 `/usage-analytics` 时仍然只分析自己的调用，不能把用户侧页面改造成全站分析页。

### 分析与管理解耦

`/admin-analytics` 负责：

- 聚合
- 趋势
- 分布
- 排名
- 风险识别
- drilldown 入口

已有管理页面负责：

- 用户编辑
- 套餐 CRUD
- 订阅发放、作废和删除
- 订单管理
- 日志明细

分析页只提供跳转和筛选，不直接承担写操作，避免大屏变成复杂操作台。

### 安全边界以后端为准

前端隐藏菜单只作为 UX 优化，不能作为权限边界。所有新增管理员分析接口必须使用 `middleware.AdminAuth()`。

## 产品范围

新增管理员页面：

- 中文名称：`运营分析`
- 英文名称：`Operations Analytics`
- 前端路由：`/admin-analytics`
- 后端 API 前缀：`/api/admin-analytics`
- 权限：管理员专用，全部接口走 `AdminAuth`

页面覆盖完整分析域：

1. Overview：总览
2. Plans：套餐分布
3. Quota：配额用量
4. Users：用户生命周期
5. Conversion：订阅转化
6. Invitations：邀请奖励
7. Usage：调用消耗
8. Risks：风险与异常

## 全局数据口径

### 时间范围与快照口径

所有响应必须返回：

```ts
interface AdminAnalyticsRangeMeta {
  start_timestamp: number
  end_timestamp: number
  snapshot_at: number
}
```

`start_timestamp` 与 `end_timestamp` 是秒级 Unix timestamp。`snapshot_at = min(now, end_timestamp)`。

除非单个指标另有说明，以下指标使用 `snapshot_at` 的快照口径：

- 当前 active subscription
- 当前 active subscription user
- Allocated Tokens
- Used Tokens
- Remaining Tokens
- Usage Rate
- Plan Distribution
- Quota Bucket
- Expiring Soon
- High Usage / Exhausted / Idle subscription

快照口径下 active subscription 必须满足：

```text
status = 'active'
AND start_time <= snapshot_at
AND end_time > snapshot_at
```

事件类指标使用时间窗口口径，事件时间必须落在 `[start_timestamp, end_timestamp]` 内：

- 注册
- 创建订阅
- 过期
- 续订
- 试用开始
- 试用转付费
- 奖励发放
- API 调用

字段命名约定：

- `current_*`：只受 `snapshot_at` 影响。
- `period_*`、`new_*`、`expired_*`、`converted_*`、`granted_*`、`request_*`：受时间窗口影响。

### Token limit 与使用率分类

管理员分析必须使用同一个分类函数处理订阅额度。

1. `token_limit > 0`：有限额度订阅。
   - `usage_rate = max(token_used, 0) / token_limit`
   - `remaining_tokens = max(token_limit - token_used, 0)`
   - 参与 `allocated_tokens`、`used_tokens`、`remaining_tokens`、P50/P95 usage rate。
2. `token_limit == 0 AND normalized_source IN ('trial_code', 'invite_trial')`：显式无限试用订阅。
   - `token_unlimited = true`
   - 不参与 `allocated_tokens` 分母、平均/P50/P95 usage rate、高用量/耗尽统计。
   - `token_used` 可作为实际消耗展示，但不得产生除零 usage rate。
   - 分桶归入 `unlimited_or_invalid`，不得归入 `zero_limit` 风险。
   - `normalized_source` 必须由「订阅来源归一化」函数计算；不得直接读取 raw `grant_reason` 判断无限试用。为兼容历史数据，`raw_reason = '' AND raw_source IN ('trial_code','invite_trial')` 也应归入显式无限试用。
3. `token_limit == 0` 且不满足显式无限试用：历史或异常零额度订阅。
   - 分桶归入 `zero_limit`。
   - 如果在 `snapshot_at` 是 active subscription，进入「零配额异常」风险。
4. `token_limit < 0` 或 `token_used < 0`：异常数据。
   - 分桶归入 `unlimited_or_invalid`。
   - 聚合展示前将负 `token_used` 按 0 计入。
   - 不参与使用率分母，并生成 system risk。
5. `token_limit > 0 AND token_used > token_limit`：超额异常。
   - 分桶归入 `over_100`。
   - `remaining_tokens = 0`。
   - 进入「超额异常」风险。

`Average Usage Rate = sum(valid token_used) / sum(valid token_limit)`，其中 valid 仅指 `token_limit > 0` 的有限额度订阅。没有 valid 分母时返回 0。

所有使用率、分桶、风险和总览指标必须先计算 `normalized_source`，再套用 token limit 分类规则，确保 Quota、Risks、Overview、Plans 的来源和额度口径一致。

### 订阅来源归一化

所有分析域使用同一个 `normalized_source`，不得直接复用面向展示的 `SourceLabel`。

```text
raw_reason = trim(user_subscriptions.grant_reason)
raw_source = trim(user_subscriptions.source)

if raw_reason in ('order','trial_code','invite_trial','monthly_invite_entitlement','admin','redemption','system') => raw_reason
else if raw_reason = '' AND raw_source in ('order','trial_code','invite_trial','monthly_invite_entitlement','admin','redemption','system') => raw_source
else if raw_reason = '' AND raw_source = '' => 'unknown'
else => 'unknown'
```

归一化值：

| 归一化值 | 来源 |
|---|---|
| `order` | 付费订阅 |
| `trial_code` | 试用码 |
| `invite_trial` | 邀请注册试用 |
| `monthly_invite_entitlement` | 月度邀请奖励 |
| `admin` | 管理员手工发放 |
| `redemption` | 兑换码发放 |
| `system` | 系统发放 |
| `unknown` | 空值、历史未知值或未知来源 |

历史兼容：`raw_reason = '' AND raw_source = 'order'` 必须归为 `order`。`trial_code` 与 `invite_trial` 在归一化值中保持拆分；只有展示层可以把二者合并为 Trial。

`paid_count` 只能由归一化 `order` 且存在成功付费订单，或历史兼容记录确认；不得仅凭套餐价格或 token limit 推断付费来源。

### 用户组与调用组

必须区分两个分组概念：

| 名称 | 来源 | 含义 |
|---|---|---|
| `user_group` | `users.group` | 用户属性分组 |
| `request_group` | `logs.group` | 调用发生时的分组 |

新接口禁止使用泛化的 `groups` 作为 canonical 参数。需要兼容旧链接时，后端可以把 `groups` 映射为 `user_groups` 并返回 deprecation warning。

### 邀请奖励资格口径

邀请奖励相关分析必须与实际发放 sweep 使用同源口径。合格被邀请用户至少满足：

- `users.inviter_id = inviter_id`
- 被邀请用户存在当前有效的付费订阅。
- 订阅在 `snapshot_at` 满足 active subscription 严格定义。
- 订阅来源归一化为 `order`。
- 对应 `subscription_orders.status = common.TopUpStatusSuccess`。
- 对应订单 `money > 0`。
- 对应 `subscription_plans.reward_eligible = true`。

实现应复用或抽取现有 `service/invitation_reward.go` 中 qualified active invitee 的同源逻辑，避免分析数字与实际奖励发放不一致。

## 全局筛选系统

所有分析域共享全局筛选，筛选状态写入 URL search，便于分享和复现。

### 时间范围

支持：

- 今天
- 最近 7 天
- 最近 30 天
- 最近 90 天
- 最近 180 天
- 最近 365 天
- 自定义时间范围

默认值：最近 30 天。

管理员最大查询窗口：365 天。若真实数据量导致查询变慢，后续通过聚合表优化，不在设计层面缩小分析范围。

长窗口自动粒度：

| 时间范围 | 默认粒度 |
|---|---|
| `<= 48h` | hour |
| `<= 90d` | day |
| `<= 365d` | week |

### 公共 Query 解析规则

多选字段只使用 repeated query params，不使用逗号字符串；后端解析时必须 trim、去空、去重、排序，非法值返回 400。

```text
start_timestamp seconds
end_timestamp seconds
granularity=hour|day|week|month

plan_ids repeated positive int
user_ids repeated positive int
token_ids repeated positive int
channel_ids repeated positive int
user_groups repeated string
request_groups repeated string
subscription_statuses=active|expired|cancelled|inactive repeated
user_statuses=enabled|disabled repeated
log_statuses=success|error repeated
grant_reasons=order|trial_code|invite_trial|monthly_invite_entitlement|admin|redemption|system|unknown repeated
sources=order|trial_code|invite_trial|monthly_invite_entitlement|admin|redemption|system|unknown repeated

trial=true|false
reward_eligible=true|false
has_inviter=true|false
inviter_id positive int
username string
business_codes repeated string
registered_start_timestamp seconds
registered_end_timestamp seconds
subscription_start_timestamp seconds
subscription_end_timestamp seconds
next_reset_start_timestamp seconds
next_reset_end_timestamp seconds
reset_status=due|not_due repeated

limit positive int, default 20, max 100
offset non-negative int, default 0
sort_by endpoint-specific whitelist
sort_order=asc|desc
```

每个 endpoint 必须在 DTO 或 controller 常量中声明 `sort_by` 白名单；不在白名单内返回 400，不得拼接任意列名。

### 请求校验与错误响应规范

所有 `/api/admin-analytics/*` 接口返回项目统一 envelope。成功使用 `common.ApiSuccess`；业务校验失败返回 HTTP 400：

```json
{"success": false, "message": "unsupported sort_by"}
```

必须校验：

- `start_timestamp` 与 `end_timestamp` 要么同时省略，要么同时提供。
- 默认时间范围为最近 30 天。
- `end_timestamp - start_timestamp` 不得超过 365 天，超出时返回 `time range exceeds 365 days`。
- `start_timestamp > end_timestamp` 返回 `invalid time range`。
- `limit` 默认 20，最大 100；`offset` 最小 0；非法数字返回 400，超出最大值 clamp 到最大值。
- enum 字段必须白名单校验，非法值返回 400。
- 每个 endpoint 必须声明自己的 `sort_by` 白名单；未声明的字段不能排序。
- Usage Consumption 的 Go 层候选日志读取必须有硬上限，超过上限返回 `admin analytics candidate logs exceed limit` 或 panel 级 `partial_unavailable`，提示缩小时间范围或增加筛选条件。

错误消息方向：

- `invalid time range`
- `time range exceeds 365 days`
- `invalid repeated param`
- `invalid enum`
- `unsupported sort_by`
- `admin analytics candidate logs exceed limit`
- `drilldown target rejected`

## 分析域一：Overview

### 核心卡片

| 指标 | 定义 |
|---|---|
| Total Users | 用户总数 |
| Enabled Users | 启用用户数 |
| Disabled Users | 禁用用户数 |
| Subscription Users | 当前拥有 active subscription 的去重用户数 |
| Active Subscriptions | 当前 active subscription 实例数 |
| Paid Subscription Users | 当前付费订阅用户数 |
| Trial Users | 当前试用订阅用户数 |
| Reward Subscription Users | 当前邀请奖励订阅用户数 |
| Allocated Tokens | active subscriptions 的有限额度 `token_limit` 总和 |
| Used Tokens | active subscriptions 的 `token_used` 总和 |
| Remaining Tokens | `max(token_limit - token_used, 0)` 总和，仅有限额度订阅参与 |
| Average Usage Rate | `sum(valid token_used) / sum(valid token_limit)` |
| Median Usage Rate | active finite subscription 使用率 P50 |
| P95 Usage Rate | active finite subscription 使用率 P95 |
| High Usage Users | finite usage rate `>= 75%` 的用户数 |
| Exhausted Users | finite usage rate `>= 100%` 的用户数 |
| Expiring Soon | 未来 7 天内过期的 active subscriptions |
| Idle Subscription Users | 有 active subscription 但时间范围内无调用的用户数 |
| Invitation Reward Users | 时间范围内获得邀请奖励的用户数 |
| Qualified Inviters | 达到奖励条件的邀请人数量 |

### 总览趋势

`/overview` 必须返回 `trends`，覆盖：

- active subscription count
- subscription users
- allocated tokens
- used tokens
- remaining tokens
- usage rate
- new subscriptions
- expired subscriptions
- renewed subscriptions
- trial started
- trial converted
- reward granted
- active API usage users

支持粒度：hour、day、week、month。默认粒度 day。

## 分析域二：Plan Distribution

### 套餐用户分布

按套餐聚合：

| 字段 | 含义 |
|---|---|
| `plan_id` | 套餐 ID |
| `plan_title` | 套餐名称 |
| `business_code` | 业务 code |
| `is_trial` | 是否试用 |
| `reward_eligible` | 是否参与邀请奖励 |
| `user_count` | 去重用户数 |
| `subscription_count` | 订阅实例数 |
| `active_count` | active 实例数 |
| `expired_count` | expired 实例数 |
| `paid_count` | 付费来源数 |
| `trial_count` | 试用来源数 |
| `reward_count` | 邀请奖励来源数 |
| `redemption_count` | 兑换码来源数 |
| `allocated_tokens` | 有限额度 `token_limit` 总和 |
| `used_tokens` | `token_used` 总和 |
| `remaining_tokens` | 剩余额度总和 |
| `usage_rate` | `used_tokens / allocated_tokens` |
| `avg_usage_rate` | 有限额度订阅平均使用率 |
| `p50_usage_rate` | 使用率 P50 |
| `p95_usage_rate` | 使用率 P95 |
| `expiring_soon_count` | 即将过期数量 |
| `zero_limit_count` | 异常零额度订阅数 |
| `unlimited_count` | 显式无限试用或无法纳入有限额度分母的订阅数 |

图表：

- 套餐用户数柱状图
- 套餐 token allocation vs used 堆叠图
- 套餐使用率排行
- 套餐来源构成堆叠图

### 套餐生命周期

`/plan-distribution` 必须返回 `lifecycle_trends`，按套餐展示：

- 新增订阅趋势
- 续订趋势
- 过期趋势
- 试用转付费趋势
- 奖励订阅趋势
- 手工发放趋势
- 兑换码发放趋势

### 套餐设计健康度

`/plan-distribution` 必须返回 `health`，至少包含：

| 指标 | 解释 |
|---|---|
| Underused Plan Count | 使用率长期低于 10% 的订阅数 |
| Overused Plan Count | 使用率超过 90% 的订阅数 |
| Zero Usage Subscription Count | active 但 `token_used = 0` 的订阅数 |
| Exhausted Subscription Count | `token_used >= token_limit` 的订阅数 |
| Reset Pressure | `next_reset_time` 前使用率已经过高的订阅数 |

不得计算「还能用几天」，避免回到 runway 语义。

## 分析域三：Quota Usage

### 使用率分桶

固定分桶：

```text
zero
0_10
10_25
25_50
50_75
75_90
90_100
over_100
zero_limit
unlimited_or_invalid
```

每个桶展示：

- user_count
- subscription_count
- allocated_tokens
- used_tokens
- remaining_tokens
- avg_token_limit
- avg_token_used

图表：

- 分桶柱状图
- token used / remaining 堆叠柱状图
- 使用率区间趋势图

`/quota-distribution` 必须返回：

- `buckets`
- `trends`
- `high_usage_users`
- `idle_subscriptions`
- `exhausting_subscriptions`

### 高用量用户排行

列：

- user_id
- username
- group
- plan title
- subscription id
- token_limit
- token_used
- remaining_tokens
- usage_rate
- start_time
- end_time
- next_reset_time
- grant_reason
- normalized_source
- last_request_time
- request_count

`last_request_time` 与 `request_count` 当 `LOG_DB` 可查询且候选日志未超过上限时返回数值，否则返回 `null` 并在 `partial_unavailable` 中说明原因。

### 闲置配额排行

基础条件：

```text
active subscription
AND (token_used = 0 OR usage_rate < threshold)
```

如果结合日志，则额外支持：

```text
时间范围内 request_count = 0
```

### 即将耗尽订阅

条件：

```text
usage_rate >= 90%
```

或：

```text
token_limit - token_used <= threshold
```

## 分析域四：User Lifecycle

### 用户状态分布

按用户维度展示：

- total users
- enabled users
- disabled users
- active subscription users
- no subscription users
- trial users
- paid users
- reward users
- redemption users
- expired-only users
- never-used users

### 注册到订阅路径

漏斗：

```text
Registered
→ Has API Key
→ Has First Request
→ Started Trial
→ Has Active Subscription
→ Paid Subscription
→ Renewed Subscription
```

数据来源：

- Has API Key：`tokens`
- Has First Request：`logs`
- Trial / Subscription：`user_subscriptions`
- Paid：`subscription_orders`

如果需要合并 `DB` 与 `LOG_DB`，不得跨库 SQL JOIN。应分阶段查询后在 Go 内存中按 `user_id` 合并。

### 用户分组分析

按用户属性分组聚合：

- user_count
- subscription_user_count
- paid_user_count
- trial_user_count
- allocated_tokens
- used_tokens
- usage_rate
- request_count
- total_tokens from logs
- error_rate

## 分析域五：Subscription Conversion

### 试用转化

指标：

- trial_started_count
- trial_active_count
- trial_expired_count
- trial_to_paid_user_count
- trial_to_paid_rate
- avg_time_to_paid_seconds
- median_time_to_paid_seconds

试用来源：

- trial_code
- invite_trial
- trial plan direct grant

转化定义：

```text
同一 user_id
先存在 trial subscription
后存在 order source / paid order subscription
```

### 续订分析

指标：

- paid_user_count
- renewed_user_count
- renewal_rate
- expired_without_renewal_count
- consecutive_subscription_count
- avg_gap_seconds

续订定义：

```text
同一 user_id 的 paid subscription 结束后，新的 paid subscription 在 grace window 内开始
```

默认 grace window：7 天。

### 套餐迁移

展示用户从一个套餐迁移到另一个套餐的路径：

```text
Basic → Pro
Trial → Basic
Pro → Max
Pro → Expired
Reward → Paid
```

迁移矩阵表是必交付基线。只有在当前 VChart 版本确认支持 Sankey 且不新增依赖时，才可以额外提供桑基图增强展示。

字段：

- from_plan
- to_plan
- user_count
- avg_usage_rate_before
- avg_usage_rate_after

## 分析域六：Invitation Rewards

### 邀请基础指标

指标：

- users_with_inviter
- inviters_count
- direct_invite_count
- qualified_active_invite_count
- qualified_inviter_count
- reward_entitlement_count
- reward_granted_count
- reward_subscription_count
- reward_active_subscription_count
- reward_expired_subscription_count

### 月度奖励资格

按月份聚合：

- reward_month
- qualified_inviter_count
- entitled_count
- granted_count
- downgrade_count
- skipped_count
- reward_plan_distribution

`downgrade_count` 与 `skipped_count` 必须从 `InvitationMonthlyEntitlement` 已有状态和 reward/downgrade 字段可证明地推导；如果当前模型字段不足，应在实现计划中先补充字段或事件记录，不能在 UI 中猜测。没有字段支撑时接口返回 `partial_unavailable`，reason 为 `requires_snapshot` 或 `insufficient_history`。

### 邀请人排行

列：

- inviter_id
- inviter_username
- direct_invite_count
- qualified_active_count
- current_reward_status
- reward_plan_title
- reward_plan_business_code
- reward_subscription_id
- reward_token_limit
- reward_token_used
- reward_usage_rate
- last_reward_month

### 被邀请用户质量分析

按邀请来源展示：

- invitee_registered_count
- invitee_started_trial_count
- invitee_paid_count
- invitee_active_subscription_count
- invitee_total_token_used
- invitee_avg_usage_rate

用于判断邀请奖励是否带来真实使用用户，而不是只带来注册量。

## 分析域七：Usage Consumption

Usage Consumption 复用用户侧 Usage Analytics 的日志聚合口径，但管理员侧是全站视角。

### 管理员日志聚合维度

```ts
type AdminUsageGroupBy =
  | 'user'
  | 'plan'
  | 'model'
  | 'user_group'
  | 'request_group'
  | 'stream'
  | 'status'
  | 'channel'
  | 'endpoint'
  | 'billing_source'
  | 'token'
  | 'subscription_source'
```

数据来源：

| 维度 | 数据来源 |
|---|---|
| user | `logs.user_id` + `users` |
| plan | `user_subscriptions`，按归因模式匹配 |
| model | `logs.model_name` |
| request_group | `logs.group` |
| user_group | `users.group` |
| stream | `logs.is_stream` |
| status | `logs.type` |
| channel | `logs.channel_id` |
| endpoint | `logs.other.request_path` |
| billing_source | `logs.other.billing_source` |
| token | `logs.token_id` |
| subscription_source | `logs.other` 或 subscription records |

`LOG_DB` 与 `DB` 可能分离，禁止跨库 SQL JOIN。需要补充用户、套餐、渠道信息时：

1. 从 `LOG_DB` 查询或聚合日志。
2. 提取 `user_ids`、`token_ids`、`channel_ids`。
3. 从 `DB` 批量读取补充信息。
4. 在 Go 内存中合并。

### 调用核心指标

```ts
type AdminUsageMetric =
  | 'request_count'
  | 'total_tokens'
  | 'quota'
  | 'error_rate'
  | 'avg_latency_ms'
  | 'p95_latency_ms'
  | 'active_users'
  | 'active_api_keys'
```

管理员侧统一使用毫秒后缀：`avg_latency_ms`、`p95_latency_ms`。

Token 口径必须沿用用户侧 Usage Analytics：

```text
metered_tokens != nil 时使用 metered_tokens
metered_tokens == nil 时 fallback 到 prompt_tokens + completion_tokens
metered_tokens = 0 是权威值，不能 fallback
错误日志 tokens / quota 按 0 计入消耗
```

### Usage Consumption Query

`/usage-consumption/summary`、`/timeseries`、`/breakdown` 除公共 Query 外支持：

```text
group_by=<AdminUsageGroupBy>
metric=<AdminUsageMetric>
plan_attribution=current|event_time
top_n positive int, default 20, max 100
```

默认值：

- `group_by=user`
- `metric=total_tokens`
- `plan_attribution=current`
- `limit=20`
- summary / breakdown 默认 `sort_by=metric`，`metric` 表示按当前请求的 `metric` 排序。
- summary / breakdown 默认 `sort_order=desc`。
- timeseries 默认不带 `sort_by`；如果请求显式包含 `sort_by`，返回 `unsupported sort_by`。

### 套餐归因模式

#### 当前套餐口径

```text
plan_attribution=current
```

按用户当前 active subscription 归因。默认使用该模式。

#### 发生时套餐口径

```text
plan_attribution=event_time
```

只能在候选日志未超过上限时执行。实现必须先从 `LOG_DB` 读取候选日志，并在 Go 内存解析 `logs.other`。当 `logs.other.billing_source = 'subscription'` 且存在 `subscription_id` 时，`subscription_id` 是发生时套餐归因的权威来源：从 `DB` 批量读取这些 `user_subscriptions` 与 `subscription_plans` 后按 `subscription_id` join；若同时存在 `subscription_plan_id`，只能作为校验或缺失订阅记录时的降级展示，不得覆盖 `subscription_id`。只有历史日志缺少 `subscription_id` 时，才允许从 `DB` 批量读取相关用户的订阅，并按 `user_id` 与 `logs.created_at ∈ [start_time,end_time)` 在 Go 内存匹配；如果同一条日志匹配多个订阅，必须按与 `selectPrimaryBillableSubscriptionTx` 同源的优先级选择，无法无歧义选择时归入 `unknown` 并返回 panel 级 `partial_unavailable`（reason: `insufficient_history`），不得随机取第一条或重复计数。全流程禁止跨 `DB` / `LOG_DB` SQL JOIN。

历史订阅归因 selector 必须是只读纯逻辑，不读取当前 `ActiveSubscriptionId`，不调用 quota reset，不修改 `UserSubscription` 或用户设置。缺少权威 `subscription_id` 且存在多个候选订阅时，必须归入 `unknown` 并返回 `partial_unavailable(reason='insufficient_history')`；不得依赖当前用户偏好、不得触发 reset、不得随机选择第一条。

### `logs.other` 字段解析

`endpoint`、`billing_source`、`subscription_source` 等来自 `logs.other` 的维度不得使用数据库 JSON operator 或数据库专属函数实现。实现必须：

1. 在 `LOG_DB` 上按时间、用户、状态、模型、分组、channel、token 等可索引字段先过滤。
2. 只选择必要列：`id,user_id,created_at,type,quota,prompt_tokens,completion_tokens,metered_tokens,use_time,is_stream,channel_id,token_id,model_name,group,other`。
3. 对候选行数量执行硬限制；超过限制时返回 `partial_unavailable` 或 400，不继续扫描。
4. 使用项目 JSON wrapper（如 `common.UnmarshalJsonStr` 或现有 `common.StrToMap`）在 Go 内存中解析 `other.request_path`、`other.billing_source` 等字段。
5. 空值归一化为 `unknown`，不得猜测为空 billing_source 是 wallet。
6. `logs.other` 中与订阅计费相关字段的优先级为：`billing_source` 判定是否订阅计费；`subscription_id` 作为订阅实例归因权威字段；`subscription_plan_id` / `subscription_plan_title` 仅用于缺失订阅记录时的降级展示或一致性校验；`subscription_tokens_consumed` 作为 subscription-source token 消耗的权威字段，显式 0 不得 fallback。
7. 空 `billing_source` 归一化为 `unknown`，不得猜测为 wallet 或 subscription。

## 分析域八：Risk Insights

### 风险项统一格式

所有风险项必须定义：

- `risk_key`
- 数据源
- 筛选窗口
- 最小样本量
- 阈值
- severity 映射
- drilldown

### 套餐风险

规则：

- `high_exhaustion_risk`：finite usage rate `>= 90%`。
- `overused_subscription`：`token_used > token_limit AND token_limit > 0`。
- `zero_limit_active_subscription`：`token_limit = 0` 且不满足显式无限试用，并且在 `snapshot_at` 是 active subscription。
- `expired_active_status`：`status = active AND end_time <= snapshot_at`。
- `overlapping_active_subscription`：同一用户存在多个时间窗口重叠的 active subscriptions。
- `reset_overdue`：`next_reset_time <= snapshot_at - 24h` 且 finite subscription 的 `token_used > 0`。

### 风险默认阈值

实现必须将以下默认值写入后端常量并由测试覆盖：

| 风险 | 默认阈值 |
|---|---|
| `many_invites_low_qualified` | 窗口内 `direct_invite_count >= 20` 且 `qualified_active_count / direct_invite_count < 10%`，最小样本量 20。 |
| `reward_subscription_never_used` | 奖励订阅发放后满 7 天，且在 `[start_time, min(snapshot_at,end_time))` 内 `token_used = 0` 且无 API 调用。 |
| `reset_overdue` | `next_reset_time <= snapshot_at - 24h` 且 finite subscription 的 `token_used > 0`，最小样本量 1。 |
| `underused_plan_subscription` | active finite subscription 在当前窗口内 usage rate `< 10%` 且订阅已持续至少 7 天，最小样本量 1。 |
| `reset_pressure` | `next_reset_time - snapshot_at <= 7d` 且 finite usage rate `>= 90%`。 |

风险测试需断言阈值边界：低于阈值不触发，达到阈值触发。

### 用户风险

默认规则：

- `active_subscription_no_api_key`：active subscription user 没有 API Key。
- `active_subscription_no_request`：active subscription user 在时间窗口内没有请求。
- `high_error_rate_user`：最近窗口请求数 `>= 20` 且 error rate `>= 20%`。
- `sudden_usage_spike`：最近 24h `total_tokens` 大于过去 7 日日均的 3 倍，且差值 `>= 10,000 tokens`。
- `many_failed_requests`：最近窗口错误数 `>= 50`。
- `many_tokens_across_many_models`：最近窗口 `total_tokens` 位于 P95 且 distinct model count `>= 5`。
- `abnormal_stream_ratio`：请求数 `>= 50` 且 stream ratio `<= 5%` 或 `>= 95%`，并与全站中位数差异 `>= 50` 个百分点。

### 邀请奖励风险

默认规则：

- `many_invites_low_qualified`：窗口内 `direct_invite_count >= 20` 且 `qualified_active_count / direct_invite_count < 10%`。
- `reward_subscription_never_used`：奖励订阅发放后满 7 天，且在 `[start_time, min(snapshot_at,end_time))` 内 `token_used = 0` 且无 API 调用。
- `reward_downgrade_frequently_triggered`：同一 inviter 最近 3 个 `reward_month` 中 downgrade count `>= 2`。
- `reward_plan_exhausted_rapidly`：奖励订阅在发放后 7 天内 finite usage rate `>= 90%`。

阈值后续可以配置，但默认值必须写入常量并有测试覆盖。

### 系统数据风险

默认规则：

- `invalid_negative_token_quota`：`token_limit < 0` 或 `token_used < 0` 的订阅记录，数据源为 `user_subscriptions`，筛选窗口为 `snapshot_at` 快照，最小样本量 1，阈值为任一负值，severity 为 `critical`，drilldown 到受影响用户或订阅列表。
- `candidate_log_limit_exceeded`：Usage 或 Risk 查询命中候选日志上限，数据源为 `logs`，筛选窗口为当前请求时间范围，最小样本量为候选上限 + 1，severity 为 `warning`，drilldown 为 `null`，同时返回 panel 级 `partial_unavailable`。

## 后端 API 设计

### 后端路由权限

`/api/admin-analytics` 必须使用统一 router group 绑定 `middleware.AdminAuth()`，不得只在前端隐藏菜单，也不得把权限判断分散到每个 handler 中。

```go
adminAnalyticsRoute := apiRouter.Group("/admin-analytics")
adminAnalyticsRoute.Use(middleware.AdminAuth())
{
    adminAnalyticsRoute.GET("/overview", controller.GetAdminAnalyticsOverview)
    adminAnalyticsRoute.GET("/plan-distribution", controller.GetAdminAnalyticsPlanDistribution)
    adminAnalyticsRoute.GET("/quota-distribution", controller.GetAdminAnalyticsQuotaDistribution)
    adminAnalyticsRoute.GET("/user-lifecycle", controller.GetAdminAnalyticsUserLifecycle)
    adminAnalyticsRoute.GET("/subscription-conversion", controller.GetAdminAnalyticsSubscriptionConversion)
    adminAnalyticsRoute.GET("/invitation-rewards", controller.GetAdminAnalyticsInvitationRewards)
    adminAnalyticsRoute.GET("/usage-consumption/summary", controller.GetAdminUsageConsumptionSummary)
    adminAnalyticsRoute.GET("/usage-consumption/timeseries", controller.GetAdminUsageConsumptionTimeseries)
    adminAnalyticsRoute.GET("/usage-consumption/breakdown", controller.GetAdminUsageConsumptionBreakdown)
    adminAnalyticsRoute.GET("/risks", controller.GetAdminAnalyticsRisks)
    adminAnalyticsRoute.GET("/drilldown/users", controller.GetAdminAnalyticsDrilldownUsers)
    adminAnalyticsRoute.GET("/drilldown/subscriptions", controller.GetAdminAnalyticsDrilldownSubscriptions)
    adminAnalyticsRoute.GET("/drilldown/invitations", controller.GetAdminAnalyticsDrilldownInvitations)
}
```

所有 handler 默认从已认证管理员上下文执行，不接受前端传入的 role/admin 标记作为授权依据。

### API 总览

```text
GET /api/admin-analytics/overview
GET /api/admin-analytics/plan-distribution
GET /api/admin-analytics/quota-distribution
GET /api/admin-analytics/user-lifecycle
GET /api/admin-analytics/subscription-conversion
GET /api/admin-analytics/invitation-rewards
GET /api/admin-analytics/usage-consumption/summary
GET /api/admin-analytics/usage-consumption/timeseries
GET /api/admin-analytics/usage-consumption/breakdown
GET /api/admin-analytics/risks
GET /api/admin-analytics/drilldown/users
GET /api/admin-analytics/drilldown/subscriptions
GET /api/admin-analytics/drilldown/invitations
```

### Endpoint 到分析域的完成映射

每个分析域必须至少有一个稳定 API response 字段、一个 panel 组件和一个测试覆盖点；正文中列出的能力不得只停留在概念描述。

| 分析域 | API | 前端 panel | 必测文件 |
|---|---|---|---|
| Overview | `/overview` 返回 `summary` 与 `trends` | `overview-panel.tsx` | `model/admin_analytics_test.go`、`controller/admin_analytics_test.go`、`lib/chart-data.test.ts` |
| Plans | `/plan-distribution` 返回 `groups`、`lifecycle_trends`、`health` | `plan-distribution-panel.tsx` | `model/admin_analytics_test.go`、`controller/admin_analytics_test.go`、`lib/chart-data.test.ts` |
| Quota | `/quota-distribution` 返回 `buckets`、`trends`、`high_usage_users`、`idle_subscriptions`、`exhausting_subscriptions` | `quota-distribution-panel.tsx` | `model/admin_analytics_test.go`、`controller/admin_analytics_test.go`、`lib/chart-data.test.ts` |
| Users | `/user-lifecycle` 返回 `status_distribution`、`funnel`、`groups`、`trends` | `user-lifecycle-panel.tsx` | `model/admin_analytics_test.go`、`controller/admin_analytics_test.go`、`lib/chart-data.test.ts` |
| Conversion | `/subscription-conversion` 返回 `trial`、`renewal`、`migrations` | `subscription-conversion-panel.tsx` | `model/admin_analytics_test.go`、`controller/admin_analytics_test.go` |
| Invitations | `/invitation-rewards` 返回 `summary`、`monthly`、`inviters`、`invitee_quality` | `invitation-rewards-panel.tsx` | `model/admin_analytics_test.go`、`controller/admin_analytics_test.go` |
| Usage | `/usage-consumption/{summary,timeseries,breakdown}` | `usage-consumption-panel.tsx` | `model/admin_analytics_usage_test.go`、`lib/chart-data.test.ts` |
| Risks | `/risks` 返回 `plan_risks`、`user_risks`、`invitation_risks`、`system_risks` | `risks-panel.tsx` | `model/admin_analytics_risk_test.go`、`lib/drilldown.test.ts` |

### 稳定 DTO 与分页 envelope

管理员分析接口不得返回 `map[string]any`、`gin.H` 或未声明字段作为业务数据。所有 response 必须定义在 `dto/admin_analytics.go`，controller 只负责解析 query、调用 service/model、返回明确 DTO。

```ts
interface AdminAnalyticsPage {
  limit: number
  offset: number
  total: number
  has_more: boolean
}

interface AdminAnalyticsAvailabilityWarning {
  section: 'overview' | 'plans' | 'quota' | 'users' | 'conversion' | 'invitations' | 'usage' | 'risks'
  reason:
    | 'candidate_limit_exceeded'
    | 'log_db_unavailable'
    | 'insufficient_history'
    | 'requires_snapshot'
    | 'unsupported_attribution'
    | 'aggregation_timeout'
  message: string
}

interface AdminAnalyticsPanelResponse<T> {
  range: AdminAnalyticsRangeMeta
  data: T
  warnings?: AdminAnalyticsAvailabilityWarning[]
}

interface AdminAnalyticsList<T> {
  page: AdminAnalyticsPage
  items: T[]
  sort_by: string
  sort_order: 'asc' | 'desc'
}

各 endpoint 小节展示的是 `AdminAnalyticsPanelResponse<T>.data` 内的业务 DTO；实际 HTTP 成功响应必须包装为 `AdminAnalyticsPanelResponse<业务 DTO>`，因此每个接口都必须同时返回 `range`，并可返回 `warnings`。例如 `/overview` 的 data 类型是 `AdminAnalyticsOverviewResponse`，实际数据类型是 `AdminAnalyticsPanelResponse<AdminAnalyticsOverviewResponse>`。

所有返回列表或排行的字段必须使用 `AdminAnalyticsList<T>` 包装，不返回裸数组；`limit` 默认 20、最大 100，`offset` 默认 0。每个列表必须声明独立 `sort_by` 白名单，不在白名单内返回 `unsupported sort_by`。Plan Distribution 的 `groups` 是 Top N + Other 展示，不支持 offset；其 Top N 仍受 `limit` 和 sort whitelist 约束。

最低 `sort_by` 白名单：

| 列表 | 允许 `sort_by` |
|---|---|
| plan-distribution groups | `user_count`、`subscription_count`、`active_count`、`allocated_tokens`、`used_tokens`、`usage_rate`、`expiring_soon_count` |
| quota high_usage_users | `usage_rate`、`token_used`、`remaining_tokens`、`request_count`、`last_request_time` |
| quota idle_subscriptions | `usage_rate`、`token_used`、`request_count`、`last_request_time` |
| quota exhausting_subscriptions | `usage_rate`、`remaining_tokens`、`token_used`、`end_time` |
| invitation inviters | `qualified_active_count`、`direct_invite_count`、`reward_token_used`、`reward_usage_rate`、`last_reward_month` |
| risks | `severity`、`count`、`sample_size`、`risk_key` |
| usage-consumption summary groups | `metric` 或任一 `AdminUsageMetric` 值；`metric` 表示按当前请求的 `metric` 排序。 |
| usage-consumption breakdown groups | `metric` 或任一 `AdminUsageMetric` 值；`metric` 表示按当前请求的 `metric` 排序。 |
| usage-consumption timeseries | 不接受 `sort_by`；传入 `sort_by` 返回 `unsupported sort_by`。 |
| plan-distribution health | 不分页，随 `groups` 的 Top N plan 返回；排序与 `plan-distribution groups` 一致。 |
| drilldown users | `user_id`、`username`、`status`、`usage_rate`、`token_used` |
| drilldown subscriptions | `subscription_id`、`user_id`、`plan_id`、`status`、`usage_rate`、`token_used`、`end_time` |
| drilldown invitations | `inviter_id`、`invitee_id`、`qualified_active`、`reward_month` |
```

### `GET /api/admin-analytics/overview`

```ts
interface AdminAnalyticsOverviewResponse {
  summary: AdminAnalyticsOverviewSummary
  trends: AdminAnalyticsOverviewTrendPoint[]
}

interface AdminAnalyticsOverviewSummary {
  users: {
    total_users: number
    enabled_users: number
    disabled_users: number
    active_subscription_users: number
    no_subscription_users: number
    idle_subscription_users: number
  }
  subscriptions: {
    active_subscriptions: number
    paid_subscriptions: number
    trial_subscriptions: number
    reward_subscriptions: number
    redemption_subscriptions: number
    expired_subscriptions: number
    expiring_soon_subscriptions: number
  }
  quota: {
    allocated_tokens: number
    used_tokens: number
    remaining_tokens: number
    average_usage_rate: number
    p50_usage_rate: number
    p95_usage_rate: number
    high_usage_users: number
    exhausted_users: number
  }
  invitations: {
    users_with_inviter: number
    inviters_count: number
    direct_invite_count: number
    qualified_invite_count: number
    qualified_inviter_count: number
    reward_users: number
    reward_subscriptions: number
    reward_active_subscription_count: number
    reward_expired_subscription_count: number
  }
}

interface AdminAnalyticsOverviewTrendPoint {
  timestamp: number
  active_subscription_count: number
  subscription_user_count: number
  allocated_tokens: number
  used_tokens: number
  remaining_tokens: number
  usage_rate: number
  new_subscription_count: number
  expired_subscription_count: number
  renewed_subscription_count: number
  trial_started_count: number
  trial_converted_count: number
  reward_granted_count: number
  active_api_usage_user_count: number
}
```

### `GET /api/admin-analytics/plan-distribution`

```ts
interface AdminAnalyticsPlanDistributionResponse {
  groups: AdminAnalyticsList<AdminAnalyticsPlanGroup>
  other: AdminAnalyticsPlanGroup | null
  lifecycle_trends: AdminAnalyticsPlanLifecycleTrendPoint[]
  health: AdminAnalyticsPlanHealth[]
}

interface AdminAnalyticsPlanGroup {
  plan_id: number
  plan_title: string
  business_code: string | null
  is_trial: boolean
  reward_eligible: boolean
  user_count: number
  subscription_count: number
  active_count: number
  expired_count: number
  paid_count: number
  trial_count: number
  reward_count: number
  redemption_count: number
  allocated_tokens: number
  used_tokens: number
  remaining_tokens: number
  usage_rate: number
  avg_usage_rate: number
  p50_usage_rate: number
  p95_usage_rate: number
  expiring_soon_count: number
  zero_limit_count: number
  unlimited_count: number
}

interface AdminAnalyticsPlanLifecycleTrendPoint {
  timestamp: number
  plan_id: number
  new_subscription_count: number
  renewed_subscription_count: number
  expired_subscription_count: number
  trial_converted_count: number
  reward_subscription_count: number
  manual_grant_count: number
  redemption_count: number
}

interface AdminAnalyticsPlanHealth {
  plan_id: number
  plan_title: string
  underused_subscription_count: number
  overused_subscription_count: number
  zero_usage_subscription_count: number
  exhausted_subscription_count: number
  reset_pressure_count: number
}
```

### `GET /api/admin-analytics/quota-distribution`

```ts
interface AdminAnalyticsQuotaDistributionResponse {
  buckets: AdminAnalyticsQuotaBucket[]
  trends: AdminAnalyticsQuotaTrendPoint[]
  high_usage_users: AdminAnalyticsList<AdminAnalyticsSubscriptionRankingItem>
  idle_subscriptions: AdminAnalyticsList<AdminAnalyticsSubscriptionRankingItem>
  exhausting_subscriptions: AdminAnalyticsList<AdminAnalyticsSubscriptionRankingItem>
}

interface AdminAnalyticsQuotaBucket {
  bucket:
    | 'zero'
    | '0_10'
    | '10_25'
    | '25_50'
    | '50_75'
    | '75_90'
    | '90_100'
    | 'over_100'
    | 'zero_limit'
    | 'unlimited_or_invalid'
  label: string
  user_count: number
  subscription_count: number
  allocated_tokens: number
  used_tokens: number
  remaining_tokens: number | null
  avg_token_limit: number
  avg_token_used: number
}

interface AdminAnalyticsQuotaTrendPoint {
  timestamp: number
  bucket: AdminAnalyticsQuotaBucket['bucket']
  subscription_count: number
  used_tokens: number
  remaining_tokens: number | null
}

interface AdminAnalyticsSubscriptionRankingItem {
  user_id: number
  username: string
  user_group: string
  subscription_id: number
  plan_id: number
  plan_title: string
  token_limit: number
  token_used: number
  remaining_tokens: number | null
  usage_rate: number | null
  token_unlimited: boolean
  start_time: number
  end_time: number
  next_reset_time: number
  grant_reason: string
  normalized_source: string
  last_request_time: number | null
  request_count: number | null
  drilldown: AdminAnalyticsDrilldownTarget | null
}
```

### `GET /api/admin-analytics/user-lifecycle`

```ts
interface AdminAnalyticsUserLifecycleResponse {
  status_distribution: AdminAnalyticsUserStatusDistribution
  funnel: AdminAnalyticsUserLifecycleFunnel
  groups: AdminAnalyticsUserGroup[]
  trends: AdminAnalyticsLifecycleTrendPoint[]
}

interface AdminAnalyticsUserStatusDistribution {
  total_users: number
  enabled_users: number
  disabled_users: number
  active_subscription_users: number
  no_subscription_users: number
  trial_users: number
  paid_users: number
  reward_users: number
  redemption_users: number
  expired_only_users: number
  never_used_users: number
}

interface AdminAnalyticsUserLifecycleFunnel {
  registered: number
  has_api_key: number
  has_first_request: number
  started_trial: number
  active_subscription: number
  paid_subscription: number
  renewed_subscription: number
}

interface AdminAnalyticsUserGroup {
  user_group: string
  user_count: number
  subscription_user_count: number
  paid_user_count: number
  trial_user_count: number
  allocated_tokens: number
  used_tokens: number
  usage_rate: number
  request_count: number | null
  total_tokens: number | null
  error_rate: number | null
}

interface AdminAnalyticsLifecycleTrendPoint {
  timestamp: number
  registered_count: number
  first_request_count: number
  started_trial_count: number
  active_subscription_count: number
  paid_subscription_count: number
  renewed_subscription_count: number
}
```

### `GET /api/admin-analytics/subscription-conversion`

```ts
interface AdminAnalyticsSubscriptionConversionResponse {
  trial: AdminAnalyticsTrialConversion
  renewal: AdminAnalyticsRenewalConversion
  migrations: AdminAnalyticsPlanMigration[]
}

interface AdminAnalyticsTrialConversion {
  trial_started_count: number
  trial_active_count: number
  trial_expired_count: number
  trial_to_paid_user_count: number
  trial_to_paid_rate: number
  avg_time_to_paid_seconds: number
  median_time_to_paid_seconds: number
}

interface AdminAnalyticsRenewalConversion {
  paid_user_count: number
  renewed_user_count: number
  renewal_rate: number
  expired_without_renewal_count: number
  consecutive_subscription_count: number
  avg_gap_seconds: number
}

interface AdminAnalyticsPlanMigration {
  from_plan_id: number | null
  from_plan_title: string
  to_plan_id: number | null
  to_plan_title: string
  user_count: number
  avg_usage_rate_before: number
  avg_usage_rate_after: number
}
```

### `GET /api/admin-analytics/invitation-rewards`

```ts
interface AdminAnalyticsInvitationRewardsResponse {
  summary: AdminAnalyticsInvitationRewardSummary
  monthly: AdminAnalyticsInvitationRewardMonth[]
  inviters: AdminAnalyticsList<AdminAnalyticsInviterRankingItem>
  invitee_quality: AdminAnalyticsInviteeQuality[]
}

interface AdminAnalyticsInvitationRewardSummary {
  users_with_inviter: number
  inviters_count: number
  direct_invite_count: number
  qualified_active_count: number
  qualified_inviter_count: number
  reward_entitlement_count: number
  reward_granted_count: number
  reward_subscription_count: number
  reward_active_subscription_count: number
  reward_expired_subscription_count: number
}

interface AdminAnalyticsInvitationRewardMonth {
  reward_month: string
  qualified_inviter_count: number
  entitled_count: number
  granted_count: number
  downgrade_count: number | null
  skipped_count: number | null
  reward_plan_distribution: AdminAnalyticsRewardPlanDistribution[]
}

interface AdminAnalyticsRewardPlanDistribution {
  plan_id: number
  plan_title: string
  reward_subscription_count: number
}

interface AdminAnalyticsInviterRankingItem {
  inviter_id: number
  inviter_username: string
  direct_invite_count: number
  qualified_active_count: number
  current_reward_status: string
  reward_plan_title: string | null
  reward_plan_business_code: string | null
  reward_subscription_id: number | null
  reward_token_limit: number | null
  reward_token_used: number | null
  reward_usage_rate: number | null
  last_reward_month: string | null
  drilldown: AdminAnalyticsDrilldownTarget | null
}

interface AdminAnalyticsInviteeQuality {
  source: string
  invitee_registered_count: number
  invitee_started_trial_count: number
  invitee_paid_count: number
  invitee_active_subscription_count: number
  invitee_total_token_used: number
  invitee_avg_usage_rate: number
}
```

### Usage Consumption response DTO

```ts
interface AdminUsageMetrics {
  request_count: number
  success_count: number
  error_count: number
  success_rate: number
  error_rate: number
  total_tokens: number
  metered_tokens: number
  prompt_tokens: number
  completion_tokens: number
  quota: number
  avg_latency_ms: number
  p95_latency_ms: number
  rpm: number
  tpm: number
  active_users: number
  active_api_keys: number
}

interface AdminUsageGroup {
  key: string
  label: string
  group_by: AdminUsageGroupBy
  metrics: AdminUsageMetrics
  share: number
  drilldown: AdminAnalyticsDrilldownTarget | null
}

interface AdminUsageConsumptionSummaryResponse {
  total: AdminUsageMetrics
  groups: AdminAnalyticsList<AdminUsageGroup>
  group_by: AdminUsageGroupBy
  other?: AdminUsageGroup | null
}

interface AdminUsageTimeseriesPoint {
  timestamp: number
  key: string
  label: string
  metrics: AdminUsageMetrics
}

interface AdminUsageTimeseriesResponse {
  points: AdminUsageTimeseriesPoint[]
  granularity: 'hour' | 'day' | 'week' | 'month'
}

interface AdminUsageBreakdownResponse {
  groups: AdminAnalyticsList<AdminUsageGroup>
  group_by: AdminUsageGroupBy
  other: AdminUsageGroup | null
}
```

### `GET /api/admin-analytics/risks`

```ts
interface AdminAnalyticsRisksResponse {
  plan_risks: AdminAnalyticsList<AdminAnalyticsRiskItem>
  user_risks: AdminAnalyticsList<AdminAnalyticsRiskItem>
  invitation_risks: AdminAnalyticsList<AdminAnalyticsRiskItem>
  system_risks: AdminAnalyticsList<AdminAnalyticsRiskItem>
}

interface AdminAnalyticsRiskItem {
  id: string
  risk_key: string
  severity: 'info' | 'warning' | 'critical'
  category: 'plan' | 'user' | 'invitation' | 'system'
  title: string
  description: string
  count: number
  threshold: string
  sample_size: number
  drilldown: AdminAnalyticsDrilldownTarget | null
}
```

### Drilldown DTO

后端只返回受控枚举，不返回任意 URL。

```ts
type AdminAnalyticsDrilldownTarget =
  | { kind: 'admin_user'; user_id: number }
  | { kind: 'admin_users'; user_ids?: number[]; user_group?: string; user_status?: 'enabled' | 'disabled'; plan_id?: number; inviter_id?: number }
  | { kind: 'admin_subscriptions'; user_id?: number; plan_id?: number; status?: 'active' | 'expired' | 'cancelled' | 'inactive' }
  | { kind: 'admin_usage_logs'; user_id?: number; username?: string; token_id?: number; model?: string; request_group?: string; channel_id?: number; status?: 'success' | 'error'; start_timestamp: number; end_timestamp: number }
  | { kind: 'admin_analytics_tab'; tab: 'overview' | 'plans' | 'quota' | 'users' | 'conversion' | 'invitations' | 'usage' | 'risks'; inviter_id?: number }
```

Drilldown list endpoints 使用分页 envelope，并返回瘦 DTO：

```ts
interface AdminAnalyticsDrilldownUsersResponse {
  page: AdminAnalyticsPage
  items: AdminAnalyticsDrilldownUserItem[]
}

interface AdminAnalyticsDrilldownUserItem {
  user_id: number
  username: string
  user_group: string
  status: 'enabled' | 'disabled'
  current_plan_title: string | null
  token_limit: number | null
  token_used: number | null
  usage_rate: number | null
  inviter_id: number | null
}

interface AdminAnalyticsDrilldownSubscriptionsResponse {
  page: AdminAnalyticsPage
  items: AdminAnalyticsDrilldownSubscriptionItem[]
}

interface AdminAnalyticsDrilldownSubscriptionItem {
  subscription_id: number
  user_id: number
  username: string
  plan_id: number
  plan_title: string
  status: string
  normalized_source: string
  token_limit: number
  token_used: number
  usage_rate: number | null
  start_time: number
  end_time: number
}

interface AdminAnalyticsDrilldownInvitationsResponse {
  page: AdminAnalyticsPage
  items: AdminAnalyticsDrilldownInvitationItem[]
}

interface AdminAnalyticsDrilldownInvitationItem {
  inviter_id: number
  inviter_username: string
  invitee_id: number
  invitee_username: string
  qualified_active: boolean
  current_plan_title: string | null
  reward_month: string | null
}
```

## 后端文件结构

新增：

```text
dto/admin_analytics.go
model/admin_analytics.go
model/admin_analytics_test.go
controller/admin_analytics.go
controller/admin_analytics_test.go
```

如果 Usage Consumption 或风险分析实现过大，可以进一步拆分：

```text
model/admin_analytics_subscription.go
model/admin_analytics_invitation.go
model/admin_analytics_usage.go
model/admin_analytics_usage_test.go
model/admin_analytics_risk.go
model/admin_analytics_risk_test.go
```

聚合 SQL / DB 访问放在 model；跨 `DB` / `LOG_DB` 编排与 DTO 组装可以放在 `service/admin_analytics.go`，也可以沿用用户侧 Usage Analytics 的 model 聚合模式。无论放在哪层，都不得让 controller 直接堆业务聚合逻辑。

## 前端文件结构

新增：

```text
web/default/src/features/admin-analytics/
  api.ts
  constants.ts
  types.ts
  index.tsx

  lib/
    filters.ts
    format.ts
    chart-data.ts
    page-contract.ts
    drilldown.ts

  components/
    admin-analytics-tabs.tsx
    admin-analytics-filter-bar.tsx
    admin-analytics-summary-cards.tsx
    admin-analytics-section-card.tsx

    overview-panel.tsx
    plan-distribution-panel.tsx
    quota-distribution-panel.tsx
    user-lifecycle-panel.tsx
    subscription-conversion-panel.tsx
    invitation-rewards-panel.tsx
    usage-consumption-panel.tsx
    risks-panel.tsx

    plan-distribution-chart.tsx
    quota-usage-bucket-chart.tsx
    subscription-trend-chart.tsx
    lifecycle-funnel-chart.tsx
    plan-migration-table.tsx
    inviter-ranking-table.tsx
    high-usage-users-table.tsx
    risk-insights-list.tsx
```

新增路由：

```text
web/default/src/routes/_authenticated/admin-analytics/index.tsx
```

修改：

```text
web/default/src/hooks/use-sidebar-data.ts
web/default/src/hooks/use-sidebar-config.ts
web/default/src/i18n/static-keys.ts
web/default/src/i18n/locales/en.json
web/default/src/i18n/locales/zh.json
web/default/src/i18n/locales/fr.json
web/default/src/i18n/locales/ru.json
web/default/src/i18n/locales/ja.json
web/default/src/i18n/locales/vi.json
web/default/src/routeTree.gen.ts
```

`routeTree.gen.ts` 只能由 TanStack Router 生成，不得手写业务逻辑。

### 前端文件职责

- `api.ts`：只封装 `/api/admin-analytics/*` 请求，所有 GET 参数必须由 `lib/filters.ts` 生成的 `URLSearchParams` 序列化；多选字段使用 repeated query params。
- `types.ts`：维护后端 DTO、route search、canonical filters、tab id、drilldown target 等跨组件类型。
- `constants.ts`：维护 tab、metric、bucket、risk severity、sort option 等常量；展示文案只保存 `labelKey`，并同步登记 `static-keys.ts`。
- `lib/filters.ts`：负责 Zod `validateSearch`、默认值、enum 校验、数字/布尔 coercion、数组去空去重排序、时间窗口 clamp，输出稳定的 `AdminAnalyticsCanonicalFilters`。
- `lib/page-contract.ts`：负责默认 filters、React Query query key、active tab 到 endpoint 的映射，不包含 JSX。
- `lib/drilldown.ts`：只负责白名单 drilldown target 的构建与校验，不允许直接透传后端返回的任意 `to` 字符串。
- `lib/chart-data.ts`：负责 Top N / Other、VChart data transform、bucket ordering、series key 稳定性，不发起请求。
- `components/*-panel.tsx`：只渲染对应 tab 的 UI 状态、图表和表格；不得重复实现 filters/query key/drilldown 规则。

### 前端路由契约

`web/default/src/routes/_authenticated/admin-analytics/index.tsx` 必须使用：

```ts
createFileRoute('/_authenticated/admin-analytics/')
```

并提供：

- `beforeLoad`：读取 `authStore` 或现有认证上下文，当用户不存在或 `role < ROLE.ADMIN` 时 `redirect({ to: '/403' })`。
- `validateSearch`：使用 `adminAnalyticsSearchSchema` 归一化 URL search，空 URL 默认 `tab='overview'`、最近 30 天、`granularity='day'`。
- URL search 是页面可分享状态；筛选草稿不进入 query key，只有 Apply 后写入 URL 并生成 canonical filters。
- `routeTree.gen.ts` 只能由 TanStack Router 生成；新增 route 后运行能触发 router 生成的命令并提交生成结果，不得手写业务逻辑。

## 前端布局

### 桌面端

```text
[Page Header: Operations Analytics]
[Description: Analyze subscription adoption, quota usage, invitation rewards, and usage risks.]

[Global Filter Bar]
  Time Range | Plans | User Groups | Request Groups | Sources | Statuses | Granularity | Apply

[Tabs]
  Overview | Plans | Quota | Users | Conversion | Invitations | Usage | Risks

[Tab Content]
  Summary Cards
  Main Chart
  Secondary Chart / Ranking
  Drilldown Table
```

### 移动端

- 筛选栏折叠为 Sheet。
- Summary cards 双列或单列。
- 图表纵向排列。
- 表格降级为卡片列表。
- 排行表默认展示 top 10，更多通过分页或「View all」。
- 移动端表格必须复用现有 `DataTablePage` / `MobileCardList` 降级模式。列定义需要为移动端指定优先级：主识别字段使用 `mobileTitle`，状态/风险等级使用 `mobileBadge`，低优先级审计字段使用 `mobileHidden` 或折叠到详情。

### 状态处理

每个 panel 都必须有：

- loading
- error
- empty
- background refetching
- partial unavailable

`partial unavailable` 用于某些高级分析依赖日志库或历史数据不可用时，保证页面其他模块仍可展示。

### Partial unavailable contract

后端响应允许携带 panel 级可用性信息，前端据此展示局部不可用，不把整个页面置为 error。对应类型见 `AdminAnalyticsAvailabilityWarning`。

当某个高级分析不可用时，对应 panel/card/chart 展示 `partial unavailable` 提示并保留其他可用数据；只有网络错误、权限错误或该 panel 关键数据完全缺失时才进入 error 状态。

### React Query 与前端请求策略

- 页面只请求当前 active tab 所需接口；未激活 tab 不发起请求。切换 tab 时再加载该 tab，可使用 `enabled: activeTab === '<tab>'` 或仅 mount active panel。
- panel 组件可使用 `React.lazy` / `Suspense` 按需加载，避免首屏加载 8 个 tab 的图表和表格代码。
- query key 必须使用层级数组并包含 tab、endpoint、canonical filters，例如：

```ts
['admin-analytics', 'overview', 'overview', canonicalFilters]
['admin-analytics', 'plans', 'plan-distribution', canonicalFilters]
['admin-analytics', 'usage', 'usage-consumption-timeseries', canonicalFilters]
```

- canonical filters 必须只包含可序列化的稳定值：number、string、boolean、排序后的数组；不得放入 Date、函数、`t`、React state draft 或未排序数组。
- 切换 tab 或筛选时可保留旧数据，但必须展示 background refetching 状态，不能让标题/筛选与旧数据看起来已经同步完成。
- 多选字段在 URL/API wire format 中使用 repeated query params；外部传入单值时 `validateSearch` 归一化为数组。

### 图表实现约束

管理员分析中心图表必须复用当前 `web/default` 的 VChart 体系：`@visactor/react-vchart`、`@visactor/vchart`、`VCHART_OPTION`、现有 theme provider / chart theme / Skeleton / Empty / ErrorState 模式。不得为本功能新增图表库或并行主题系统。

VChart `seriesField` 使用稳定业务 key（如 `group_key` / `bucket` / `risk_category`），tooltip、legend、标题展示本地化 label。相同展示名但不同 key 的 series 不得合并。

套餐迁移图：只有在当前 VChart 版本确认支持 Sankey 且不新增依赖时才使用桑基图；否则必须使用迁移矩阵表 `plan-migration-table.tsx`。

### Top N / Other 前端语义

- 所有排行、饼图、堆叠图和多 series 趋势图默认 Top N 为 20，最大 100；移动端默认展示 Top 10，更多通过分页或 View all。
- Top N 必须在整个筛选窗口内按当前 sort metric 确定，同一个趋势图的所有时间点使用同一组 Top N。
- 超出 Top N 的项合并为稳定 `Other`，`Other` 使用固定 key（如 `__other__`），展示本地化 label，不参与 drilldown。
- additive metric（count、tokens、quota）可以对 `Other` 求和；rate、percentile、latency 类指标必须由后端或 `chart-data.ts` 按样本/分子分母重算，不能简单平均。
- 桶类图表必须固定排序，不能按本地化 label 排序。

## Drilldown 设计

### 白名单与目标页 search contract

前端不得直接执行后端返回的任意 `to`。`lib/drilldown.ts` 必须把后端 target 映射到白名单之一：

```ts
type FrontendAdminAnalyticsDrilldownTarget =
  | { to: '/users'; search: { userId?: number; planId?: number; inviterId?: number; status?: string[] } }
  | { to: '/subscriptions'; search: { planId?: number; status?: 'active' | 'expired' | 'cancelled' | 'inactive'; userId?: number } }
  | { to: '/usage-logs/$section'; params: { section: 'common' }; search: { userId?: number; username?: string; startTime?: number; endTime?: number; model?: string; group?: string; tokenId?: number; channel?: string; status?: 'success' | 'error'; isStream?: boolean } }
  | { to: '/admin-analytics'; search: Partial<AdminAnalyticsSearch> }
```

如果 `/users` 或 `/subscriptions` 当前 route 不支持上述 search，实现时必须同步补齐轻量 `validateSearch`、URL state 和表格/API 筛选；不得降级为无筛选跳转。若 `/subscriptions` 保持为套餐计划管理页，`status=active` 的用户订阅实例 drilldown 应改为 `/users?planId=...` 或新增订阅实例视图后再启用。

### 用户 drilldown

从高用量、闲置、风险用户跳转到用户管理页时使用白名单 search：

```text
/users?userId=...
```

实现时必须同步扩展 `web/default/src/routes/_authenticated/users/index.tsx` 的 `validateSearch` 与 `UsersTable` 的轻量筛选能力。若目标页暂不支持 `userId`，不得只跳转到无筛选的 `/users`。

从管理员调用消耗跳转到 Usage Logs 时必须跳转到 TanStack route `/_authenticated/usage-logs/$section`，参数为 `{ section: 'common' }`，URL 形态为：

```text
/usage-logs/common?username=...&startTime=...&endTime=...
```

`startTime` / `endTime` 使用现有 Usage Logs route 的时间单位，保持与 `usage-logs/$section` route、`CommonLogsFilterBar`、`buildApiParams` 一致；后端 API 参数 `start_timestamp` / `end_timestamp` 只能由 Usage Logs 的 API 参数映射层生成。管理员 Usage Logs 只能使用 admin 支持的筛选字段，不能误用用户侧 self logs。

当后端 `AdminAnalyticsDrilldownTarget.kind = 'admin_usage_logs'` 且携带 `user_id` 时，前端必须优先映射为 Usage Logs route search 的 `userId`；`username` 仅作为兼容筛选或展示字段。实现时必须同步扩展 `web/default/src/routes/_authenticated/usage-logs/$section.tsx` 的 `validateSearch`、`CommonLogFilters`、`GetLogsParams` / `GetLogStatsParams`、`buildApiParams`、`CommonLogsFilterBar`，以及管理员日志后端 `GetAllLogs` / `GetLogStat` 的 `user_id` 过滤；非管理员 self logs 不接受前端传入的 `userId`。如果本轮不扩展 Usage Logs 的 `userId` 筛选，则必须从 `admin_usage_logs` DTO 中移除 `user_id`，并要求后端只在能提供 `username` 时返回 Usage Logs drilldown，否则返回 `drilldown: null` 或使用 `/users?userId=...`。

`admin_usage_logs.start_timestamp` / `end_timestamp` 是后端秒级字段；映射到 Usage Logs route search 时必须转换为现有毫秒级 `startTime = start_timestamp * 1000`、`endTime = end_timestamp * 1000`。Usage Logs 的 `buildApiParams` 再负责把毫秒级 route search 转成后端 `/api/log` 的 `start_timestamp` / `end_timestamp`。

### 套餐 drilldown

从套餐分布跳转：

```text
/subscriptions?planId=...&status=active
```

或：

```text
/users?planId=...
```

如果现有页面不支持这些 search 参数，实现时应补齐轻量筛选能力。

### 邀请 drilldown

从邀请排行跳转：

```text
/users?userId=<inviter_id>
```

或回到当前页面的邀请 tab：

```text
/admin-analytics?tab=invitations&inviter_id=...
```

## 数据兼容与性能设计

### 三库兼容

必须支持：

- SQLite
- MySQL >= 5.7.8
- PostgreSQL >= 9.6

规则：

- 优先使用 GORM。
- raw SQL 必须处理 quoting。
- 不使用数据库专属 JSON operator。
- 不依赖 PostgreSQL `FILTER`、MySQL 专有函数等无 fallback 特性。
- P50 / P95 等 percentile 可以在 Go 内存中计算，避免三库 SQL 差异。

### Raw SQL 兼容细则

优先使用 GORM 查询构造。确需 raw SQL 时：

- 业务库 `users.group`、`tokens.group`、`channels.group` 等列使用 `commonGroupCol`。
- 日志库 `logs.group` 使用 `logGroupCol`，因为 `LOG_DB` 可与 `DB` 使用不同数据库类型。
- `key` 保留字列使用 `commonKeyCol` / `logKeyCol`。
- 业务库 raw SQL boolean 值使用 `commonTrueVal` / `commonFalseVal`，或优先使用 GORM 参数绑定布尔值避免手写。
- 日志库 `LOG_DB` 的 boolean 字段（例如 `logs.is_stream`）不得使用 `commonTrueVal` / `commonFalseVal`，因为 `LOG_DB` 可与 `DB` 使用不同数据库类型；应使用 GORM 参数绑定。若确需手写日志库 boolean literal，必须在 `model/main.go` 中新增并初始化 `logTrueVal` / `logFalseVal`（按 `LOG_SQL_DSN` / `common.LogSqlType` 方言），并补充 DB=SQLite、LOG_DB=PostgreSQL 以及 DB=PostgreSQL、LOG_DB=SQLite/MySQL 的 DryRun 或分离 fixture 测试。
- 不使用 PostgreSQL `FILTER`、JSON/JSONB operator、MySQL-only 函数或 SQLite-only JSON1 函数；P50/P95 在 Go 内存计算。

### DB 与 LOG_DB 分离

业务分析使用：

```text
model.DB
```

调用消耗分析使用：

```text
model.LOG_DB
```

需要合并时：

1. 查询 `LOG_DB` 聚合。
2. 收集 `user_ids`、`token_ids`、`channel_ids`。
3. 查询 `DB` 补充信息。
4. 在 Go 内存 join。

禁止跨库 SQL JOIN。

### 分离数据库测试要求

后端测试必须构造真实分离的 `model.DB` 与 `model.LOG_DB`：

1. `model.DB` 使用一个 in-memory SQLite 连接，只迁移 `users`、`tokens`、`subscription_plans`、`user_subscriptions`、`subscription_orders`、`invitation_monthly_entitlements`、`channels` 等业务表。
2. `model.LOG_DB` 使用另一个 in-memory SQLite 连接，只迁移 `logs`。
3. 任何需要用户、套餐、渠道、token 补充信息的接口都必须先查 `LOG_DB`，收集 id 后查 `DB`，再在 Go 内存合并。
4. 测试不得把业务表迁移到 `LOG_DB`，也不得把 `logs` 迁移到 `DB`，以确保跨库 SQL JOIN 会失败。

### 查询窗口与上限

- 默认窗口：30 天。
- 最大窗口：365 天。
- Top N 默认 20，最大 100。
- drilldown 表分页默认 20，最大 100。
- Usage Consumption 管理员日志候选行默认上限：100,000；超过上限返回 `partial_unavailable` 或业务错误，要求缩小时间范围或增加筛选条件。

### 高成本查询边界

- `plan_attribution=current` 为默认且可在聚合后按 `user_id` 合并当前 active subscription。
- `plan_attribution=event_time` 只能在候选行未超过上限时执行。
- Risk 中的 spike、many failed requests、many tokens across many models 默认只分析最近 30 天；更长窗口必须依赖后续 `admin_analytics_daily_snapshots` 或返回 partial unavailable。
- 所有风险列表默认 Top 20，最大 100，必须分页或截断。

### 后续聚合表

当真实数据量证明查询慢时，再新增每日快照表，例如：

```text
admin_analytics_daily_snapshots
```

可能字段：

```text
date
plan_id
user_group
grant_reason
active_subscription_count
subscription_user_count
allocated_tokens
used_tokens
remaining_tokens
new_subscription_count
expired_subscription_count
reward_subscription_count
request_count
total_tokens
quota
error_count
```

聚合表是性能层，不改变 API contract。

## 安全设计

必须满足：

- 后端全部使用 `AdminAuth`。
- 前端菜单隐藏只作为 UX，不作为权限边界。
- 不返回完整 API Key。
- 不返回支付 provider payload。
- 不返回 OAuth token / binding secret。
- 不返回渠道密钥。
- 邀请详情不得一次性暴露大量被邀请用户敏感字段。

### 管理员分析安全 DTO 白名单

Admin Analytics 不得直接返回 `model.User`、`model.Token`、`model.Channel`、`SubscriptionOrder`、OAuth provider/binding 原始结构体。

允许的用户字段仅限：

- `id`
- `username`
- `group`
- `status`
- `created_at`
- `last_login_at`
- 订阅摘要
- 配额摘要
- 邀请摘要

不得返回：

- `password`
- `access_token`
- email verification code
- OAuth IDs
- `stripe_customer`
- `aff_code`
- `setting`
- `remark`

允许的 API Key 字段仅限：

- `id`
- `name`
- `masked_key`
- `status`
- `group`
- `deleted`

不得返回完整 `key`、`allow_ips`、模型限制细节，除非某个分析字段明确需要且经过 DTO 白名单。

允许的渠道字段仅限：

- `id`
- `name`
- `type`
- `status`
- `tag`

不得返回 `key`、`keys`、`setting`、`settings`、`other_info`、`param_override`、`header_override`、上游认证信息或渠道密钥。

## i18n 设计

新增 key 使用命名空间式结构：

```text
adminAnalytics.title
adminAnalytics.description
adminAnalytics.tabs.overview
adminAnalytics.tabs.plans
adminAnalytics.tabs.quota
adminAnalytics.tabs.users
adminAnalytics.tabs.conversion
adminAnalytics.tabs.invitations
adminAnalytics.tabs.usage
adminAnalytics.tabs.risks
adminAnalytics.metrics.activeSubscriptionUsers
adminAnalytics.metrics.allocatedTokens
adminAnalytics.metrics.usedTokens
```

实现必须更新：

```text
web/default/src/i18n/static-keys.ts
web/default/src/i18n/locales/en.json
web/default/src/i18n/locales/zh.json
web/default/src/i18n/locales/fr.json
web/default/src/i18n/locales/ru.json
web/default/src/i18n/locales/ja.json
web/default/src/i18n/locales/vi.json
```

并运行：

```bash
cd web/default && bun run i18n:sync
```

所有用户可见文案和枚举 labelKey 都必须覆盖：tab、metric、bucket、risk severity、source、空态、错误态、partial unavailable、drilldown 操作、表格列名。新增 key 必须进入 `static-keys.ts` 或以 `t('...')` 字面量可扫描，并确认六语言 locale 不留新增 untranslated key。

## 测试设计

### 后端 model 测试

新增或拆分：

```text
model/admin_analytics_test.go
model/admin_analytics_usage_test.go
model/admin_analytics_risk_test.go
```

必须包含以下测试方向：

- `TestAdminAnalyticsActiveSubscriptionWindowExcludesExpiredActiveStatus`
- `TestAdminAnalyticsActiveSubscriptionSharedFilterAcrossDomains`
- `TestAdminAnalyticsQuotaBucketsHandleZeroLimitUnlimitedAndOver100`
- `TestAdminAnalyticsPlanDistributionAggregatesTokenQuotaOnly`
- `TestAdminAnalyticsTrialToPaidConversionUsesUserTimeline`
- `TestAdminAnalyticsRenewalUsesGraceWindow`
- `TestAdminAnalyticsPlanMigrationBuildsMatrixWithoutPriceRunway`
- `TestAdminAnalyticsInvitationRewardsDeriveMonthlyStatus`
- `TestAdminAnalyticsUsageUsesSeparatedDBAndLogDB`
- `TestAdminAnalyticsUsageUsesMeteredTokensFallbackAndExplicitZero`
- `TestAdminAnalyticsUsageEventTimeAttributionJoinsInMemory`
- `TestAdminAnalyticsSQLBuilderAvoidsDialectSpecificFunctions`
- `TestAdminAnalyticsActiveSubscriptionScopeIsSharedBySnapshotDomains`：构造 active、expired-but-status-active、future-start、boundary-end subscriptions，断言 Overview、Plans、Quota、Invitations、Risks 使用同一 active scope；实现必须通过共享 helper/query scope 生成条件，禁止各域复制不同条件。
- `TestAdminAnalyticsNormalizesSubscriptionSourceAcrossDomains`：覆盖 `grant_reason` 优先于 `source`、空 reason fallback 到合法 source、`redemption`、`unknown`、历史 `source=order`，并断言 `paid_count` 只来自归一化 `order` 且存在成功付费订单或明确历史兼容记录。
- `TestAdminAnalyticsSeparatesUserGroupsAndRequestGroups`：业务库 `users.group` 与日志库 `logs.group` 取不同值，断言 `user_groups` 只过滤用户属性，`request_groups` 只过滤调用日志，canonical query 不接受泛化 `groups` 作为新参数。
- `TestAdminAnalyticsUsageCandidateLimitReturnsPartialUnavailable`：构造超过管理员候选日志上限的查询，断言 Go 层停止扫描并返回 `candidate_limit_exceeded` warning 或 400 `admin analytics candidate logs exceed limit`，不得继续解析 `logs.other`。
- `TestAdminAnalyticsUsageQueryValidatesGroupByMetricAttributionAndTopN`：覆盖 summary、timeseries、breakdown 三个接口的 `group_by`、`metric`、`plan_attribution` 白名单，`top_n` 默认 20、最大 100、非法值 400、超出值 clamp。
- `TestAdminAnalyticsUsageCurrentAndEventTimeAttributionRespectCandidateLimit`：断言 `plan_attribution=current` 可走聚合后 user_id 合并，`event_time` 必须在候选行未超限时才读取候选日志并在 Go 内存按订阅窗口匹配。

SQLite fixture 必跑；MySQL/PostgreSQL 使用 env-gated DSN 或 GORM DryRun 方言测试。断言 raw SQL 不包含 `DATE_TRUNC`、`FROM_UNIXTIME`、`strftime`、`FILTER`、`PERCENTILE_CONT`、窗口函数、JSON operator、裸 `group`。

### 后端 controller 测试

覆盖：

- 非管理员拒绝。
- 管理员可访问。
- 默认时间范围。
- 超过 365 天的时间范围必须返回 HTTP 400，message 为 `time range exceeds 365 days`，不得静默 clamp。
- `start_timestamp > end_timestamp` 必须返回 HTTP 400，message 为 `invalid time range`。
- 只有 `limit` / `top_n` 超过最大值允许 clamp 到 100；非法数字仍返回 HTTP 400。
- invalid plan id。
- invalid enum。
- unsupported sort_by。
- limit clamp。
- repeated query params。
- drilldown 不泄露敏感字段。
- response shape 稳定。

### 前端测试文件

新增：

```text
web/default/src/features/admin-analytics/lib/filters.test.ts
web/default/src/features/admin-analytics/lib/chart-data.test.ts
web/default/src/features/admin-analytics/lib/page-contract.test.ts
web/default/src/features/admin-analytics/lib/drilldown.test.ts
```

增强：

```text
web/default/src/hooks/use-sidebar-config.test.ts
```

覆盖：

- 空 URL 归一化为最近 30 天、默认 granularity、默认 tab。
- repeated query params 去空、去重、排序并序列化为后端 canonical 参数。
- `limit` clamp、`sort_by` 白名单、非法 enum 进入错误状态。
- additive metric 允许 Top N + Other 与 share；rate/latency metric 不堆叠、不求和。
- quota bucket 固定排序。
- drilldown target 只允许白名单 route；`Other` 和未知 target 不可跳转。
- drilldown search 不包含完整 API Key、渠道密钥、支付 payload、OAuth secret。
- 管理员可见 `/admin-analytics` sidebar，普通用户隐藏；`URL_TO_CONFIG_MAP` 已登记。
- i18n 常量 labelKey 已进入 `static-keys.ts` 或可被 `i18n:sync` 扫描。
- `page-contract.test.ts` 必须覆盖 tab 到 endpoint 的完整映射：overview/plans/quota/users/conversion/invitations/risks 各只产生一个 endpoint，usage 产生 summary、timeseries、breakdown 三个 endpoint；未激活 tab 不产生请求 descriptor 或 query enabled 为 false。
- `page-contract.test.ts` 必须断言 query key 形如 `['admin-analytics', tab, endpoint, canonicalFilters]`，canonical filters 只含 number/string/boolean/排序数组，不含 Date、函数、`t`、React draft state 或未排序数组。
- `page-contract.test.ts` 必须断言筛选草稿变化不改变 query key，Apply 后 URL search 归一化并生成新的 canonical filters。
- `filters.test.ts` 必须覆盖 `user_groups` 与 `request_groups` 分别去空、去重、排序并序列化为 repeated query params；不得把新 canonical filters 序列化为 `groups`。
- `filters.test.ts` 必须覆盖外部单值 query 归一化为数组、非法 enum 进入错误状态、over-365d 前端归一化策略与后端 400 策略不冲突。
- `drilldown.test.ts` 必须覆盖 `/users` 目标 search 的 `userId`、`planId`、`inviterId`、`status` 白名单，以及 Usage Logs 目标使用 `/usage-logs/$section` + `{ section: 'common' }` 和 `startTime/endTime`。
- `drilldown.test.ts` 必须断言 drilldown search 不包含完整 API Key、渠道密钥、支付 payload、OAuth secret，也不包含后端未白名单的任意 `to` 字符串。
- `page-contract.test.ts` 或纯函数测试必须覆盖 warnings 到 partial unavailable 状态的映射，确保局部 warning 不会把整个 panel 置为 error。

前端新增纯函数测试沿用当前仓库的 `node:test` / `bun test <具体文件>` 模式；除非另有单独技术方案，不为管理员分析引入新的 Vitest、jsdom、MSW 或浏览器测试基础设施。

### 前端验证

建议命令：

```bash
cd web/default && bun test src/features/admin-analytics/lib/filters.test.ts src/features/admin-analytics/lib/chart-data.test.ts src/features/admin-analytics/lib/page-contract.test.ts src/features/admin-analytics/lib/drilldown.test.ts
cd web/default && bun run typecheck
cd web/default && bun run build
cd web/default && bun run i18n:sync
```

### 后端验证

建议命令：

```bash
go test ./model ./controller -run 'AdminAnalytics|Subscription|Invitation' -count=1
```

## 多子代理实施边界

后续实现直接在主工作区开发，不创建或切换 worktree。按低冲突边界拆分。

### 任务 A：后端 DTO / filter contract / model 基础聚合

负责：

```text
dto/admin_analytics.go
model/admin_analytics*.go
model/admin_analytics*_test.go
```

不得修改 router、controller、前端文件。

### 任务 B：后端 controller / router / drilldown

负责：

```text
controller/admin_analytics.go
controller/admin_analytics_test.go
router/api-router.go
```

`router/api-router.go` 为共享文件，必须单代理串行修改。

### 任务 C：前端 API / types / lib / 纯函数测试

负责：

```text
web/default/src/features/admin-analytics/api.ts
web/default/src/features/admin-analytics/types.ts
web/default/src/features/admin-analytics/constants.ts
web/default/src/features/admin-analytics/lib/*
web/default/src/features/admin-analytics/lib/*.test.ts
```

不得修改 route、sidebar、i18n、routeTree。

### 任务 D：前端页面与 panel 组件

负责：

```text
web/default/src/features/admin-analytics/index.tsx
web/default/src/features/admin-analytics/components/*
```

依赖任务 C 的 types/lib 合同，不修改 i18n 和 routeTree。

### 任务 E：入口 / sidebar / i18n / routeTree

负责：

```text
web/default/src/routes/_authenticated/admin-analytics/index.tsx
web/default/src/hooks/use-sidebar-data.ts
web/default/src/hooks/use-sidebar-config.ts
web/default/src/i18n/*
web/default/src/routeTree.gen.ts
```

该任务必须在前端 API/types 稳定后执行，`routeTree.gen.ts` 只由生成命令产生。

### 任务 F：Drilldown 目标页 search contract

负责：

```text
web/default/src/routes/_authenticated/users/index.tsx
web/default/src/features/users/components/users-table.tsx
web/default/src/features/users/api.ts
web/default/src/features/users/types.ts
controller/user.go
model/user.go
web/default/src/routes/_authenticated/usage-logs/$section.tsx
web/default/src/features/usage-logs/types.ts
web/default/src/features/usage-logs/lib/utils.ts
web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx
web/default/src/features/admin-analytics/lib/drilldown.test.ts
web/default/src/features/users/**/*.test.ts
controller/log.go
model/log.go
controller/log_usage_analytics_test.go
```

仅当本迭代新增订阅实例筛选页时，任务 F 才可修改：

```text
web/default/src/routes/_authenticated/subscriptions/index.tsx
web/default/src/features/subscriptions/*
```

验收：`/users?userId=...`、`/users?planId=...`、`/users?inviterId=...` 不退化为无筛选列表，且不得只在当前页数据上做客户端过滤；实现可以扩展用户列表 API/types/后端筛选，或让目标页调用 `/api/admin-analytics/drilldown/users` 分页接口。Usage Logs drilldown 支持 `userId` 或明确从 DTO 移除 `user_id`。若支持 `userId`，仅管理员 `/api/log` 与 `/api/log/stat` 接受 `user_id`，self logs 不接受前端传入 `userId`；前端 `userId` route search 序列化为 API 参数 `user_id`。若 `/subscriptions` 仍为套餐计划管理页，则前端白名单不生成 `/subscriptions?status=active`。该任务必须在任务 C 的 drilldown 类型稳定后执行，避免与 sidebar/i18n/routeTree 并发冲突。

主代理最终统一运行定向 Go 测试、Bun 测试、typecheck、build、i18n sync；子代理不运行项目级 build/lint/format。

## 实现顺序

完整产品目标一次性确定，工程实现按依赖顺序推进。每个任务必须同时交付对应测试，最后一步只运行统一验证；不得在没有测试的情况下先完成全部实现。

1. 后端 DTO 与 filter contract。
2. 套餐、配额、邀请奖励 model 聚合与测试。
3. 用户生命周期与订阅转化 model 聚合与测试。
4. 管理员 Usage Consumption 聚合与测试。
5. Risk Insights 聚合与测试。
6. controller、router 与 controller 测试。
7. 前端 API、types、filters 与纯函数测试。
8. 前端页面 shell 与 tabs。
9. 各 panel 图表和表格。
10. drilldown 与测试。
11. Drilldown 目标页 search contract。
12. sidebar、routeTree、i18n。
13. 后端测试、前端测试、typecheck、build、i18n sync。

该顺序不是产品范围缩减，只是降低实现风险的工程顺序。

## 完成标准

功能完成必须满足：

- `/admin-analytics` 仅管理员可见且可访问。
- 所有 `/api/admin-analytics/*` 接口使用 `AdminAuth`。
- 页面覆盖 Overview、Plans、Quota、Users、Conversion、Invitations、Usage、Risks 8 个分析域。
- 每个分析域的 API response 字段、前端 panel、测试文件均已交付，不允许只放空 tab 或概念占位。
- Usage Consumption 三个接口拥有完整 DTO，并覆盖 `group_by`、`metric`、`plan_attribution`、Top N + Other。
- 所有列表、排行、drilldown endpoint 明确定义分页、limit clamp 和 sort whitelist。
- 套餐、配额、邀请奖励、调用消耗、风险异常均有可验证数据。
- 多选 query 参数使用 repeated query params。
- 所有新增文案完成 6 个 locale 翻译并通过 `bun run i18n:sync`。
- `web/default/src/i18n/locales/_reports/_sync-report.json` 不存在新增 untranslated key。
- 不返回完整 API Key、渠道密钥、支付 payload、OAuth secret。
- 不引入余额、可用天数、runway 等指标。
- 保持 SQLite、MySQL、PostgreSQL 兼容。
- 不跨 `DB` 与 `LOG_DB` 做 SQL JOIN。
- `model.DB` 与 `model.LOG_DB` 分离 fixture 测试通过，证明没有跨库 SQL JOIN。
- `routeTree.gen.ts` 由 TanStack Router 生成；新增 route 后 build 或等价 route generation 命令已确认包含 `/admin-analytics`。
- 直接受影响的后端与前端测试通过。
