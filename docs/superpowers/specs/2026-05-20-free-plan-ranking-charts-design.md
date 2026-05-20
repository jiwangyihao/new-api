# 免费套餐排行榜图表化设计

## 背景

当前排行榜已移除市场份额、厂商趋势等低价值指标，并新增免费套餐 token 用户榜。用户希望排行榜更偏图像化，免费榜同时提供条形图和折线图；折线图按每个用户免费套餐开通后的 24 小时周期对齐，用于比较不同用户在相同生命周期阶段的用量趋势。

## 目标

- 免费套餐排行榜主区域改为图表优先展示。
- 提供横向条形图：纵轴为用户展示名，横轴为 token 用量。
- 提供 24 小时趋势折线图：X 轴为免费套餐开通后第 0–23 小时桶。
- 折线图 Y 轴支持两种模式：每小时用量、累计用量。
- 延续隐私规则：默认匿名，只展示 `Explorer #N`；用户设置排行榜展示名后才公开该名称。
- 不重新引入市场份额、厂商份额、涨跌趋势等旧指标。

## 非目标

- 不新增外部图表依赖，继续使用 `@visactor/react-vchart`。
- 不展示账号、邮箱、用户名、普通 display name 等身份字段。
- 不做平台级自然日流量图。
- 不做付费套餐或奖励套餐排行榜。
- 不追求秒级实时刷新；排行榜可接受现有缓存与日志写入链路带来的短暂陈旧。

## 数据源决策

### 免费用户总量榜

沿用现有 `free_users` 数据：

- 按用户聚合免费/试用订阅的 `user_subscriptions.token_used`。
- 仅统计 `subscription_plans.price_amount = 0` 且满足以下之一：
  - `subscription_plans.is_trial = true`
  - `user_subscriptions.grant_reason IN ('trial_code', 'invite_trial')`
- 排除软删除用户。
- 排除 `token_used <= 0`。
- 排序：`total_tokens DESC, user_id ASC`。

### 24 小时趋势

24 小时趋势必须尽量表达“实际扣在免费/试用订阅上的 token”。不要使用 `quota_data` 作为趋势主数据源，原因是：

- `quota_data.created_at` 已按小时向下取整，不适合与非整点 `user_subscriptions.start_time` 做精确相对 24 小时对齐。
- `quota_data` 没有 `subscription_id`，无法区分同一用户同一小时内免费订阅、付费订阅或奖励订阅的实际扣费来源。
- `quota_data` 是用户/模型/小时聚合数据，不是单次消费事件，用它与订阅窗口 join 容易重复计数。

趋势数据改用消费日志 `logs`：

- 只读取 `LOG_DB` 中 `type = LogTypeConsume` 的消费日志。
- `logs.created_at` 使用秒级时间。
- `logs.metered_tokens` 是实际计量 token。
- `logs.other` 是 JSON 文本，可能包含：
  - `subscription_id`
  - `subscription_tokens_consumed`
- 数据库查询不得使用 JSON 操作符。先用 GORM 普通过滤候选日志，再在 Go 层用 `common.UnmarshalJsonStr` 或现有 JSON 包装解析 `Other`。
- 只有当日志 `other.subscription_id` 指向入榜用户的一条免费/试用订阅，且 `subscription_tokens_consumed > 0` 时，才纳入趋势。
- 如果缺少 `subscription_id` 或 `subscription_tokens_consumed`，该日志不进入 24 小时趋势；不要回退到 `quota_data` 近似统计。

这意味着 `free_user_history` 是“有订阅归属日志的 24 小时趋势”。总量榜仍以 `user_subscriptions.token_used` 为权威总量。历史日志不足时，条形图仍可用，折线图显示空状态或部分用户的可归属趋势。

## API 响应

在 `/api/rankings` 响应中新增 `free_user_history`：

```json
{
  "free_user_history": {
    "points": [
      {
        "rank": 1,
        "display_name": "Explorer #1",
        "series_label": "#1 · Explorer #1",
        "hour": 0,
        "hour_label": "0h",
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
- `series_label`：前端图表唯一展示标签，格式为 `#<rank> · <display_name>`。它避免两个用户设置相同展示名时折线或条形图被合并。
- `hour`：免费订阅开通后的小时序号，范围 `0..23`。
- `hour_label`：前端 X 轴标签，格式为 `0h` 到 `23h`。
- `tokens`：该小时内新增 token。
- `cumulative_tokens`：从第 0 小时到当前小时的累计 token。
- `hours`：固定为 `24`。

隐私边界：

- 后端内部结构可以携带 `UserID`、`SubscriptionID` 等字段用于计算。
- 外部 JSON 响应的 `FreeUserHistoryPoint` 不得包含 `user_id`、`subscription_id`、`username`、`email`、普通账号 `display_name` 等身份字段。
- 响应中的 `display_name` 只允许是 `Explorer #N` 或用户主动设置的 `rankings_display_name`。

## 后端设计

### model 层

在 `model/usedata_rankings.go` 增加只返回候选日志行的查询函数，而不是直接在 SQL 中聚合最终小时桶：

- `RankingFreeUserSubscription`
  - `ID int`
  - `UserID int`
  - `StartTime int64`
  - `EndTime int64`
- `RankingFreeUserLogCandidate`
  - `ID int`
  - `UserID int`
  - `CreatedAt int64`
  - `MeteredTokens *int`
  - `Other string`
- `GetRankingFreeUserSubscriptions(userIDs []int) ([]RankingFreeUserSubscription, error)`
- `GetRankingFreeUserLogCandidates(userIDs []int, startTime int64, endTime int64) ([]RankingFreeUserLogCandidate, error)`

查询规则：

- `GetRankingFreeUserSubscriptions` 使用与 `GetRankingFreeUserTotals` 一致的免费/试用订阅过滤条件，并排除软删除用户。
- `GetRankingFreeUserLogCandidates` 只做三库通用过滤：`user_id IN ?`、`type = LogTypeConsume`、`created_at >= minStart`、`created_at < maxStart + 24h`、`metered_tokens > 0`。
- 不在 SQL 中解析 `Other` JSON。
- 不使用窗口函数、`DISTINCT ON`、数据库专属 JSON 查询或数据库专属时间函数。

### service 层

在 `service/rankings.go` 中新增：

- `FreeUserHistorySeries`
- `FreeUserHistoryPoint`
- 内部结构 `rankedFreeUserInternal` 或等效映射，保留 `UserID` 但不带 JSON tag。
- `buildFreeUserHistory(...)`

构建流程：

1. 先构建 `free_users`，同时保留入榜用户的内部 `UserID`、`Rank`、`DisplayName`、`SeriesLabel`。
2. 查询这些用户的免费/试用订阅窗口。
3. 计算候选日志查询范围：所有入榜免费订阅 `min(start_time)` 到 `max(start_time) + 24h`。
4. 查询候选消费日志。
5. 对每条日志解析 `Other`：
   - `subscription_id` 必须能解析为 int。
   - `subscription_tokens_consumed` 必须能解析为正整数。
6. 用 `subscription_id` 找到对应免费/试用订阅；订阅的 `user_id` 必须与日志 `user_id` 相同。
7. 日志时间必须满足 `[subscription.start_time, subscription.start_time + 24h)`。
8. `hour = (log.created_at - subscription.start_time) / 3600`，整数除法向下取整，范围必须是 `0..23`。
9. `tokens` 累加 `subscription_tokens_consumed`，不使用 `metered_tokens` 兜底，以保证只统计实际订阅扣费 token。
10. 为每个入榜用户补齐 24 个小时点。没有消费的小时 `tokens = 0`，`cumulative_tokens` 延续上一小时累计值。
11. 输出 `FreeUserHistorySeries{Hours: 24, Points: ...}`，不输出任何内部 ID。

重叠窗口处理：

- 因为日志携带 `subscription_id`，每条日志只归属到一个订阅。
- 即使同一用户存在多个重叠免费订阅窗口，也只按日志中的 `subscription_id` 归属，不会因时间窗口重叠重复计数。
- 如果一条日志的 `subscription_id` 指向付费订阅、奖励订阅、软删除用户订阅或非入榜用户订阅，忽略该日志。

缓存：

- 继续使用现有 rankings cache。
- 展示名更新继续调用 `FlushRankingsCache()`。
- token 消费写入点不新增 cache flush；排行榜和趋势最多受 rankings cache TTL 影响。若 `LogConsumeEnabled` 关闭或日志写入延迟，`free_user_history` 可能为空或滞后，这是当前版本接受的行为。

## 前端设计

### 类型

在 `web/default/src/features/rankings/types.ts` 增加：

- `FreeUserHistoryPoint`
  - `rank: number`
  - `display_name: string`
  - `series_label: string`
  - `hour: number`
  - `hour_label: string`
  - `tokens: number`
  - `cumulative_tokens: number`
- `FreeUserHistorySeries`
  - `points: FreeUserHistoryPoint[]`
  - `hours: number`
- `RankingsSnapshot.free_user_history: FreeUserHistorySeries`

`FreeUserRanking` 不新增后端响应字段。条形图需要的 `series_label` 由前端从 `rank` 与 `display_name` 派生，格式同 `FreeUserHistoryPoint.series_label`：`#<rank> · <display_name>`。这样可避免扩大 `free_users` API，同时保证两个用户设置相同展示名时条形图分类仍唯一。

### 页面调用

`web/default/src/features/rankings/index.tsx` 必须把 `snapshot.free_user_history` 传给 `FreeUsersSection`：

```tsx
<FreeUsersSection
  rows={snapshot.free_users}
  totalTokens={snapshot.free_user_total_tokens}
  history={snapshot.free_user_history}
/>
```

`FreeUsersSectionProps.history` 是必填字段。没有趋势点时由 `FreeUsersSection` 或 `FreeUsersLineChart` 显示空状态。

### 组件

`FreeUsersSection` 变为容器组件，提供两级切换：

- 主视图切换：`Bar chart` / `24-hour trend`
- 折线图模式切换：`Hourly usage` / `Cumulative usage`

默认视图：`Bar chart`。
折线图默认模式：`Hourly usage`。
折线图最多绘制前 10 名用户；简化榜单列表仍展示 `free_users` 中所有用户。

新增组件：

- `free-users-bar-chart.tsx`
  - 横向条形图。
  - VChart spec 必须设置 `direction: 'horizontal'` 或等效横向配置。
  - `yField = series_label`，其中 `series_label` 由前端根据 `rank` 与 `display_name` 派生，不要求 `free_users` 后端响应新增该字段。
  - `xField = total_tokens`。
  - tooltip 显示排名、展示名、token 用量、匿名/自定义展示名状态。
- `free-users-line-chart.tsx`
  - 多线折线图。
  - `xField = hour_label`。
  - `seriesField = series_label`。
  - `yField` 根据模式选择 `tokens` 或 `cumulative_tokens`。
  - tooltip 显示小时、展示名、当前模式下 token 值。
- 简化榜单列表可保留在 `FreeUsersSection` 内，若文件接近 200 行，应拆到 `free-users-list.tsx`。

图表实现应复用现有 `ModelsSection` 模式：

- `useChartTheme()`
- `VCHART_OPTION`
- `formatTokens`
- `useMemo` 构造 spec
- 每个含文案的子组件自行调用 `useTranslation()`

### 文案与 i18n

新增或复用这些英文 key，并补齐 `en/zh/fr/ja/ru/vi`：

- `Bar chart`
- `24-hour trend`
- `Hourly usage`
- `Cumulative usage`
- `Usage after free-plan activation`
- `Compare each ranked user within their first 24 hours of free-plan access`
- `No free-plan trend data available`
- `Rank #{{rank}}`

可复用已有 key：

- `tokens`
- `Custom display name`
- `Anonymous entry`
- `Free-plan token leaderboard`

## 测试计划

### 后端测试

扩展 `service/rankings_test.go`：

1. 24 小时历史按日志 `other.subscription_id` 精确归属到免费/试用订阅，并按订阅 `start_time` 对齐。
2. 非整点 `start_time` 的边界正确：开通后首小时内日志进入 `hour = 0`，第 24 小时边界外日志不计入。
3. `tokens` 为每小时增量，`cumulative_tokens` 为累计值。
4. 每个入榜用户补齐 24 个小时点；空小时 `tokens = 0`，`cumulative_tokens` 保持上一小时累计。
5. 仅统计免费/试用订阅日志，排除付费订阅、奖励订阅、缺少订阅归属的日志、非正数 `subscription_tokens_consumed`。
6. 软删除用户即使有日志也不出现在 `free_user_history`。
7. 重叠免费订阅窗口不重复计数：同一用户两条重叠免费订阅和一条日志，只按日志 `subscription_id` 对应订阅计入一次。
8. JSON 响应隐私测试：允许公开字段 `display_name`，但 marshal 后不得包含 `user_id`、`subscription_id`、`username`、`email`，也不得包含种子用户普通账号 `DisplayName` 值；公开展示名只能是 `Explorer #N` 或 `rankings_display_name`。

### 前端测试

扩展 `web/default/src/features/rankings/rankings-free-users.test.ts`：

1. `types.ts` 包含 `FreeUserHistoryPoint`、`FreeUserHistorySeries`、`free_user_history`、`series_label`、`hour_label`、`cumulative_tokens`。
2. `index.tsx` 把 `snapshot.free_user_history` 传给 `FreeUsersSection`。
3. `FreeUsersSectionProps` 要求 `history` 必填，并引用 `FreeUsersBarChart`、`FreeUsersLineChart`。
4. `free-users-bar-chart.tsx` 包含横向配置 `direction: 'horizontal'`，并从 `rank` 与 `display_name` 派生 `series_label` 作为用户轴、使用 `total_tokens` 作为用量轴。
5. `free-users-line-chart.tsx` 根据模式选择 `tokens` 或 `cumulative_tokens`，并使用 `hour_label` 与 `series_label`。
6. 保留旧 UI 防回归断言：不得重新引用 `MarketShareSection`、`PulseSection`、`VendorRanking`、`RankingMover`、`vendor_share_history`、`top_movers`、`top_droppers`。
7. i18n smoke test：六个 locale 文件都包含本次新增 key，值非空；新增 key 不应在非英语 locale 中保持英文原文，除非是技术词或占位模板。

测试不要锁死颜色、圆角、高度、默认选中样式或完整翻译句子。

## 实施拆分建议

为了降低并发子代理冲突：

1. 后端数据/API 子任务独占：
   - `model/usedata_rankings.go`
   - `service/rankings.go`
   - `service/rankings_test.go`
2. 前端图表子任务独占：
   - `web/default/src/features/rankings/types.ts`
   - `web/default/src/features/rankings/index.tsx`
   - `web/default/src/features/rankings/components/free-users-section.tsx`
   - 新增图表组件
3. 测试与 i18n 子任务在图表文案和类型稳定后串行处理：
   - `web/default/src/features/rankings/rankings-free-users.test.ts`
   - `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`

## 验收标准

- `/api/rankings` 返回免费榜总量与 `free_user_history`。
- 免费榜条形图纵轴显示 `series_label`，横轴显示 token 用量，VChart 使用横向配置。
- 24 小时折线图支持每小时和累计两种模式。
- 趋势只统计带 `subscription_id` 和正数 `subscription_tokens_consumed` 的免费/试用订阅日志。
- 不泄露任何账号标识或内部 ID。
- 市场份额、厂商份额、涨跌趋势 UI 不回归。
- 通过以下命令：
  - `go test ./service ./controller -count=1`
  - `cd web/default && bun test src/features/rankings/rankings-free-users.test.ts`
  - `cd web/default && bun run i18n:sync`，并确认 `_sync-report.json` 中所有 locale `missingCount = 0`、`extrasCount = 0`
  - `cd web/default && bun run typecheck`
