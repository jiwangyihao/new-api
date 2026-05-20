# 免费套餐排行榜图表化设计

## 背景

当前排行榜已移除市场份额、厂商趋势等低价值指标，并新增免费套餐 token 用户榜。用户希望排行榜更偏图像化，免费榜同时提供条形图和折线图；折线图按每个用户免费套餐开通后的 24 小时周期对齐，用于比较不同用户在相同生命周期阶段的用量趋势。

## 目标

- 免费套餐排行榜主区域改为图表优先展示。
- 提供横向条形图：纵轴为用户展示名，横轴为 token 用量。
- 提供 24 小时趋势折线图：X 轴为免费套餐开通后第 0–24 小时。
- 折线图 Y 轴支持两种模式：每小时用量、累计用量。
- 延续隐私规则：默认匿名，只展示 `Explorer #N`；用户设置排行榜展示名后才公开该名称。
- 不重新引入市场份额、厂商份额、涨跌趋势等旧指标。

## 非目标

- 不新增外部图表依赖，继续使用 `@visactor/react-vchart`。
- 不展示账号、邮箱、用户名、普通 display name 等身份字段。
- 不做平台级自然日流量图。
- 不做付费套餐或奖励套餐排行榜。

## 数据语义

### 免费用户总量榜

沿用现有 `free_users` 数据：

- 按用户聚合免费/试用订阅的 `token_used`。
- 仅统计 `subscription_plans.price_amount = 0` 且满足以下之一：
  - `subscription_plans.is_trial = true`
  - `user_subscriptions.grant_reason IN ('trial_code', 'invite_trial')`
- 排除软删除用户。
- 排除 `token_used <= 0`。
- 排序：`total_tokens DESC, user_id ASC`。

### 24 小时趋势

新增 `free_user_history`：

```json
{
  "free_user_history": {
    "points": [
      {
        "rank": 1,
        "display_name": "Explorer #1",
        "hour": 0,
        "tokens": 120,
        "cumulative_tokens": 120
      }
    ],
    "hours": 24
  }
}
```

字段含义：

- `rank`：与 `free_users` 中的排名一致。
- `display_name`：匿名名或用户设置的排行榜展示名。
- `hour`：免费订阅开通后的小时序号，范围 `0..23`；前端展示为 `0h` 到 `23h` 的 24 个小时桶。
- `tokens`：该小时内新增 token。
- `cumulative_tokens`：从第 0 小时到当前小时的累计 token。
- `hours`：固定为 `24`。

统计规则：

- X 轴对齐到每条免费订阅自己的 `start_time`。
- 只统计消费时间在 `[start_time, start_time + 24h)` 内的数据。
- 使用 `quota_data.created_at` 与 `user_subscriptions.start_time` 计算小时桶。
- 同一用户多个免费订阅可聚合到同一条线。
- 如果同一用户存在重叠免费订阅窗口，必须避免同一条消费被重复计数。

## 后端设计

### model 层

在 `model/usedata_rankings.go` 增加查询函数：

- `RankingFreeUserHourTotal`
- `GetRankingFreeUserHourlyTotals(userIDs []int, hours int)`

查询以 `quota_data` 为消费事件源，以 `user_subscriptions` 为免费订阅窗口源。为了跨 SQLite/MySQL/PostgreSQL，优先使用 GORM 普通查询和少量数据库分支表达式，避免数据库专属 JSON 或窗口函数。

关键点：

- 查询只接受已经入榜的 user id，避免扫描所有用户历史。
- 只返回 0–23 小时内有 token 的桶。
- 查询结果只包含 `user_id`、`hour`、`tokens`。
- 如果 SQL 层无法稳定跨库去重重叠订阅窗口，则在 service 层对 `user_id + quota_data event` 做确定性归属；实现时不能重复计数。

### service 层

在 `service/rankings.go` 中新增：

- `FreeUserHistorySeries`
- `FreeUserHistoryPoint`
- `buildFreeUserHistory(...)`

构建流程：

1. 先构建 `free_users`，得到排名、展示名和 user id 对应关系。
2. 对入榜 user id 查询 24 小时 hourly totals。
3. 按 user id 和 hour 累加 token。
4. 计算 cumulative tokens。
5. 输出只包含展示所需字段，不输出 user id。

缓存仍使用现有 rankings cache。展示名更新继续刷新缓存。

## 前端设计

### 类型

在 `web/default/src/features/rankings/types.ts` 增加：

- `FreeUserHistoryPoint`
- `FreeUserHistorySeries`
- `RankingsSnapshot.free_user_history`

### 组件

`FreeUsersSection` 变为容器组件，提供两级切换：

- 主视图切换：`Bar chart` / `24-hour trend`
- 折线图模式切换：`Hourly` / `Cumulative`

新增组件：

- `free-users-bar-chart.tsx`
  - 横向条形图。
  - `yField = display_name`。
  - `xField = total_tokens`。
- `free-users-line-chart.tsx`
  - 多线折线图。
  - `xField = hour_label`。
  - `yField` 根据模式选择 `tokens` 或 `cumulative_tokens`。
  - `seriesField = display_name`。

保留简化榜单列表作为图表下方补充信息。

### 文案与 i18n

新增或复用这些英文 key，并补齐 `en/zh/fr/ja/ru/vi`：

- `Bar chart`
- `24-hour trend`
- `Hourly usage`
- `Cumulative usage`
- `Usage after free-plan activation`
- `Compare each ranked user within their first 24 hours of free-plan access`
- `No free-plan trend data available`

## 测试计划

### 后端测试

扩展 `service/rankings_test.go`：

1. 24 小时历史按免费订阅 `start_time` 对齐。
2. `tokens` 为每小时增量，`cumulative_tokens` 为累计值。
3. 仅统计免费/试用订阅，排除付费和奖励订阅。
4. 不输出 user id、username、display_name、email。
5. 重叠免费订阅窗口不重复计数。

### 前端测试

扩展 `web/default/src/features/rankings/rankings-free-users.test.ts`：

1. 类型中包含 `free_user_history`。
2. 免费榜 section 引用 `FreeUsersBarChart` 和 `FreeUsersLineChart`。
3. 条形图使用横向语义：用户维度 + token 用量。
4. 折线图支持 `Hourly usage` 与 `Cumulative usage`。

## 验收标准

- `/api/rankings` 返回免费榜总量与 24 小时趋势数据。
- 免费榜条形图纵轴显示匿名名/排行榜展示名，横轴显示 token 用量。
- 24 小时折线图支持每小时和累计两种模式。
- 不泄露任何账号标识。
- 市场份额、厂商份额、涨跌趋势 UI 不回归。
- 通过后端定向测试、前端排行榜测试、前端 typecheck 和 i18n sync。
