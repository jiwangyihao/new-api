# 线上性能精确优化规格

## 背景

2026-06-10 通过 `AutoDLChen` 跳板只读访问 `RackNerd6C6G` 后，线上实例表现为 CPU 饱和与数据库日志大表查询放大：

- 宿主机 6 vCPU，load average 约 `17.12 / 14.64 / 14.08`。
- CPU PSI `some avg10=72.80`，CPU idle 约 `5.9%`。
- `new-api` 与 `sub2api` 合计占用约 4 个以上 vCPU。
- `new_api.logs` 表约 `4.16 GB`，约 `254 万` 活跃行。
- PostgreSQL `new_api` 库没有锁等待，慢点主要是 `logs` 表 IO 读取。
- 现场慢 SQL 中，`model/usedata_rankings.go:123` 一次读取 `95604` 行并耗时约 `30 s`。
- 用户日志查询 `model/log.go:610` 对 `logs` 做 `user_id + created_at` 时间范围 `count(*)`，也进入慢 SQL。
- `sub2api` 的 `usage_logs` 约 `555 万` 行，30 天模型聚合查询会启动 PostgreSQL parallel worker 并被用户取消。

用户明确要求：**不要做资源隔离方案**；优化必须尽量保持查询精度，不用近似统计替代精确结果。

## 目标

1. 在不影响统计精度的前提下，降低 `new-api` 日志、排行榜、用户日志接口对 PostgreSQL 与 CPU 的压力。
2. 保持免费套餐排行榜、24 小时趋势、用户日志列表、用户日志统计、运营/监控统计的语义不变。
3. 通过索引、查询形态、精确预聚合、短 TTL 精确缓存和 singleflight 优化高频展示路径。
4. 为 `sub2api` 侧 30 天 `usage_logs` 聚合提供同精度外部执行清单，但不在 `new-api` 仓库实现 `sub2api` 查询改写。
5. 所有新增迁移必须兼容 SQLite、MySQL、PostgreSQL；PostgreSQL 专用在线建索引只能作为部署步骤或方言分支，不得进入通用启动强迁移。

## 非目标

- 不迁移 `new-api` 或 `sub2api` 到独立机器。
- 不设置容器 CPU / Memory quota 作为本规格方案。
- 不关闭 `LogConsumeEnabled`。
- 不降低查询精度：不返回估算 `total`，不丢弃符合条件的日志，不用抽样近似，不缩短用户选择的时间范围，不只看前 N 条候选日志。
- 不改变用户可见的时间范围、排行榜排名、token 计量口径、订阅归属规则、分页排序。
- 不把 `quota_data` 用作免费套餐 24 小时趋势的近似数据源。
- 不改 protected project 信息。
- 不把业务功能从 `logs` 改到外部不可控服务。
- 不把前端请求合并作为本期必要实现；前端防抖只作为可选后续优化。

## 方案总览

推荐采用「**索引先行 + 精确查询重写 + 精确派生列 + 幂等聚合表**」方案：

1. **索引先行**：补齐 `LOG_DB.logs` 和 `DB.user_subscriptions` 的复合索引，让现有查询先从秒级降到可控范围。
2. **精确查询重写**：对只需 count/sum 的接口做 SQL 聚合下推；对列表和排行榜趋势保持旧结果语义。
3. **精确派生列**：把 `logs.other` 中高频过滤字段冗余为普通列，减少每次扫描 JSON 文本。
4. **幂等聚合表**：对高频统计类接口维护按小时的精确增量聚合；聚合输入以持久化后的 `logs.id` 为唯一事件键，避免补偿和回填重复累加。
5. **短 TTL 精确缓存 / singleflight**：只用于展示统计接口，缓存值必须来自完整精确查询或「精确聚合 + 边界明细补齐」，不得用于计费强一致路径。

限流只作为防止异常客户端重复触发重查询的保护，不作为主要优化，也不改变返回精度。

## 线上瓶颈映射

### 免费套餐 24 小时趋势

现状代码：

- `model/usedata_rankings.go:111-123`
- `service/rankings.go:361-430`

当前查询：

```sql
SELECT id, user_id, created_at, metered_tokens, other
FROM logs
WHERE user_id IN (...)
  AND type = 2
  AND created_at >= ?
  AND created_at < ?
  AND metered_tokens > 0;
```

现场问题：

- 一次返回 `95604` 行。
- Go 层再解析 `other.subscription_id` 和 `subscription_tokens_consumed`，丢弃不属于免费/试用订阅的日志。
- 结果精确，但读取放大很高。

精确优化方向：

- 不能用 `quota_data` 兜底，因为它没有 `subscription_id`，无法精确保留订阅归属。
- 把 `subscription_id` 和 `subscription_tokens_consumed` 从 `logs.other` 的 JSON 文本冗余为普通列；新日志写入时同步填充，历史日志分批回填。
- 旧算法候选条件中的 `metered_tokens > 0` 必须被显式测试覆盖。若新算法想把 `subscription_tokens_consumed > 0` 作为最终归属条件，必须用 fixture 固定 `metered_tokens = 0 / NULL` 且 `subscription_tokens_consumed > 0` 时的期望，并与旧产品语义确认；默认保持旧候选过滤与旧输出逐点一致。
- 免费套餐排行榜总量仍以 `user_subscriptions.token_used` 为权威，不改为日志聚合总量。

### 用户日志列表与统计

现状代码：

- `model/log.go:577-588`
- `model/log.go:605-615`
- `model/log.go:625-652`

现场问题：

- 用户日志查询先 `Count` 再分页。
- 典型条件为 `user_id + created_at range`。
- 当前只有 `idx_logs_user_id` 与 `idx_user_id_id(user_id, id)`，缺少 `user_id + created_at` 索引。

精确优化方向：

- 补 `user_id, created_at, id` 复合索引，支持时间范围 count 与分页。
- `GetUserLogs` / `GetUserLogsWithFilter` 的 `total` 不做跨请求 TTL 缓存；可用 singleflight 合并同时发生且完整 filter/page 一致的请求，或在同一请求内复用同一查询快照。
- 用户日志列表与统计的秒级时间范围必须保持现有结束时间闭区间语义：start 使用 `>=`，end 使用 `<=`；聚合边界补齐不得漏掉 `created_at == endTimestamp` 的日志。
- `Order("logs.id desc")` 的默认排序不变。
- 对 `SumUsedQuotaWithFilter` 的 RPM / TPM 维持精确口径，近 60 秒统计只可用精确查询或精确聚合结果。

### 订阅查询慢点

现场慢 SQL：

- `model/subscription.go:1121`

现状索引：

- `idx_user_sub_active(user_id, status, end_time)`。

当前查询还需要排序：

```sql
ORDER BY CASE WHEN grant_reason IN (...) AND token_limit = 0 THEN 1 ... END,
         end_time ASC,
         id ASC
```

精确优化方向：

- 保持排序语义不变。
- 增加更覆盖的普通复合索引：`user_id, status, end_time, id`。
- 若仍慢，再考虑在查询中只选择需要字段，减少 heap 读取；不改变结果。

### `sub2api` 30 天 `usage_logs` 聚合

现场 PostgreSQL 日志显示 `usage_logs` 30 天窗口按 `model LIKE 'gpt%'` 聚合被取消，并终止 parallel worker。

本规格只提供外部执行清单，不在 `new-api` 仓库实现：

- 不隔离资源。
- 用 `EXPLAIN (ANALYZE, BUFFERS)` 评估 `usage_logs(created_at, model)` 与 `usage_logs(model, created_at)` 复合索引。
- 若要改查询为精确聚合表，必须切换到 `sub2api` 仓库另开实现计划。
- `new-api` 子代理不得修改 `sub2api` 代码或把 `sub2api` 查询改写任务纳入本仓实现。

## 数据模型设计

### `LOG_DB.logs` 新增普通列

为保持查询精度并避免每次扫描 `other` JSON，新增以下 DB 层可空列；首阶段不设置 DB default，不设置 `NOT NULL`：

- `subscription_id int NULL`
- `subscription_tokens_consumed bigint NULL`
- `billing_source varchar(32) NULL`
- `endpoint varchar(255) NULL`

写入规则：

- 这些字段从现有 `other` map 中同步写入；`other` 仍保留完整 JSON，作为兼容与审计来源。
- 新日志在代码层填充零值或空串语义，但 DB 层允许 NULL，避免 PostgreSQL 9.6 大表 `ADD COLUMN DEFAULT` 重写全表。
- 读取时必须显式处理 NULL：用指针字段、可空类型或 SQL `COALESCE`。不得因为 Go 非指针字段扫描 NULL 而混淆「未回填」与「真实 0」。
- 所有 JSON marshal/unmarshal 继续使用 `common.*` 包装函数。
- 不得把 `Log.MeteredTokens` 从 `*int` 改成 `int`。`metered_tokens = 0` 是权威显式零值，按 0 统计；只有 `metered_tokens IS NULL` 才回退到 `prompt_tokens + completion_tokens`。

迁移规则：

- 所有日志列、日志索引、日志回填、日志聚合读取均作用于 `LOG_DB`，不是默认 `DB`。
- 当 `LOG_SQL_DSN` 存在时，`LOG_DB` 可能与 `DB` 是不同数据库；迁移必须使用日志库方言，不能只看主业务库方言。
- PostgreSQL 9.6 大表新增列必须是 nullable 且无 default；回填完成并验证后，如确有必要，可在单独低峰步骤设置默认值，但本期不要求设置 `NOT NULL`。
- SQLite 使用 `ALTER TABLE ... ADD COLUMN` 模式。
- MySQL 使用普通 `ALTER TABLE ADD COLUMN`，低峰执行。
- 不得仅通过 `gorm` index tag 让 `AutoMigrate(&Log{})` 在启动强路径创建大表复合索引。

历史回填：

- 增加后台分批回填任务，按 `logs.id` 单调 checkpoint 扫描 `LOG_DB.logs.other`。
- checkpoint 写入 `DB.options` 或等价业务配置表；每次推进 checkpoint 前，必须保证 `[上次 checkpoint, 新 checkpoint]` 范围内所有目标日志都已成功处理。
- 回填失败只停止本批并记录错误，不影响服务启动；重启后从最后成功 checkpoint 恢复。
- 回填完成标记为 `backfill_complete=true` 前，新查询必须兼容两类数据。
- 列查询与 `other` fallback 必须互斥：同一条日志同时有新列和 `other` 时，只能按新列或旧解析中的一种计数，不能重复。
- 对未被 checkpoint 覆盖的 `log_id` 范围，API 必须使用旧 `other` 解析补齐，保证回填前、回填中、回填后免费趋势逐点一致。

### 聚合幂等 ledger / outbox

任何增量聚合都必须以持久化后的 `logs.id` 为唯一事件键。新增表：`log_aggregation_events`。

字段：

- `log_id int not null`
- `aggregate_name varchar(64) not null`
- `status varchar(16) not null default 'pending'`
- `error text`
- `created_at bigint not null default 0`
- `updated_at bigint not null default 0`

唯一键：

- `(log_id, aggregate_name)`

语义：

- 每个聚合器处理一条日志前，必须先插入或声明处理该 `(log_id, aggregate_name)`。
- 若唯一键冲突，说明该日志对该聚合器已经处理或正在处理，不得再次累加。
- 实时聚合、补偿重放、历史回填都必须复用同一幂等路径。
- 事件初始状态必须是 `pending`；`queueLogAggregationEvents` 创建事件时也必须显式写入 `pending`。
- 只有聚合 upsert 成功后才能把事件置为 `applied`；失败必须置为 `failed` 并保留错误信息以便重放。
- fallback 只能排除 `status = 'applied'` 的 `log_id`，不得排除 `pending`、`processing` 或 `failed` 事件。
- 补偿队列持久化 `log_id` 与 `aggregate_name`，不得只持久化裸 delta。
- `RecordConsumeLog` / `insertConsumeLogsDirect` 只要求消费日志入库成功；聚合失败不得影响主请求结算，但必须留下可重放、幂等的补偿事件。
- 对账修复命令的语义是从明细重建或按差异修复，不得盲目再次执行 additive upsert。

### 精确聚合表：免费套餐趋势

新增表：`free_subscription_usage_hourly`。

字段：

- `subscription_id int not null`
- `user_id int not null`
- `hour_index int not null`：订阅开通后第 `0..23` 小时。
- `tokens bigint not null default 0`
- `updated_at bigint not null default 0`

唯一键：

- `(subscription_id, hour_index)`

语义：

- 只记录免费/试用订阅的精确 token 消耗。
- `tokens` 累加的是 `subscription_tokens_consumed`，不是 `metered_tokens`。
- 每条消费日志只能归属一个 `subscription_id`，不会因窗口重叠重复计数。
- 若日志对应付费订阅、奖励订阅、非入榜用户订阅、软删除用户订阅或没有订阅归属，不写入本表。
- 写入时不得在 relay 热路径为每条日志额外查订阅/计划；推荐由 outbox/后台按 `log_id` 批量 join `user_subscriptions` 与 `subscription_plans` 后聚合，或使用只服务统计聚合的短 TTL 订阅元信息缓存。该缓存不得用于计费判断。
- upsert 时不得因 `user_id` 不一致静默改归属；以 `subscription_id` 的当前 owner 校验或仅在全量重建时刷新 owner。

精确性：

- 聚合表是明细日志的派生数据，不是权威替代。
- 后台提供校验任务：按订阅与小时从 `logs` 明细重新聚合，与聚合表对账。
- 回填完成前，API 使用「已确认完整的聚合区间 + 未确认区间明细 fallback」组合；fallback 必须排除已由 `log_aggregation_events` 确认处理的 `log_id`，保证不重复、不漏计。

### 精确聚合表：日志统计

新增表：`log_usage_hourly`。

字段：

- `bucket_start bigint not null`：小时起点 Unix 秒。
- `user_id int not null default 0`
- `token_id int not null default 0`
- `channel_id int not null default 0`
- `model_key_hash char(64) not null default ''`：完整 `model_name` 的 SHA-256 十六进制哈希或等价稳定哈希。
- `model_name text not null`：完整模型名，仅用于展示和对账，不作为唯一键的一部分。
- `status varchar(16) not null default ''`：由现有 `LogTypeConsume` / `LogTypeError` 派生为 `success` / `error`。
- `request_count bigint not null default 0`
- `quota_sum bigint not null default 0`
- `metered_tokens_sum bigint not null default 0`
- `prompt_tokens_sum bigint not null default 0`
- `completion_tokens_sum bigint not null default 0`
- `updated_at bigint not null default 0`

唯一键：

- `(bucket_start, user_id, token_id, channel_id, status, model_key_hash)`

语义：

- 每条 `logs` 明细写入后通过幂等 ledger/outbox 增量更新。
- `model_key_hash` 按完整 `model_name` 计算，禁止截断模型名作为分组依据。
- `metered_tokens_sum` 使用现有 `meteredTokensExpr` 语义：`metered_tokens` 非空时用它，否则使用 `prompt_tokens + completion_tokens`。
- 任意接口需要按这些维度求和时，可从聚合表精确读取完整小时桶。
- 对查询起止时间不落在小时边界的部分，仍从 `logs` 明细表精确补齐。

秒级精确公式必须与现有闭区间 end 语义对齐：

```text
结果 = 明细扫描 [start, first_full_hour)
     + 聚合表 [first_full_hour, last_full_hour)
     + 明细扫描 [last_full_hour, end]
```

其中最后一段明细扫描包含 `created_at == end`。若查询范围小于 1 小时，直接走明细表。

## 索引设计

### `LOG_DB.logs`

新增索引必须按阶段低峰创建，不能一次性全部强制创建。优先级：

1. 必须优先：`idx_logs_user_created_id`
   - 字段：`user_id, created_at, id`
   - 服务：用户日志列表与 `count(*)`。

2. 必须优先：`idx_logs_user_type_created_id`
   - 字段：`user_id, type, created_at, id`
   - 服务：免费套餐趋势候选、用户消费日志筛选。

3. 观察后再建：`idx_logs_type_created_id`
   - 字段：`type, created_at, id`
   - 服务：全局日志统计、管理员流量统计。

4. 观察后再建：`idx_logs_subscription_created`
   - 字段：`subscription_id, created_at, id`
   - 服务：订阅归属明细回查与聚合校验。

PostgreSQL 可选部分索引：

```sql
CREATE INDEX CONCURRENTLY idx_logs_free_history_pg
ON logs (user_id, type, created_at, subscription_id)
WHERE subscription_tokens_consumed > 0;
```

硬性规则：

- `logs` 大表新增复合索引不得通过通用 AutoMigrate / GORM index tag 创建。
- PostgreSQL 只能由部署脚本逐个、非事务、低峰执行 `CREATE INDEX CONCURRENTLY IF NOT EXISTS ...`。
- 执行前设置并记录 `lock_timeout`、`statement_timeout`；执行后检查并清理 invalid index。
- 应用代码不得依赖这些索引已存在才能正确运行。
- MySQL/SQLite 也应走可重复的方言迁移或人工低峰步骤，不应在启动时无条件创建所有大索引。
- 每创建一个大索引后，通过 `pg_stat_user_indexes`、慢 SQL 数量和写入延迟观察效果，再决定是否创建下一个。

### `DB.user_subscriptions`

新增索引：

- `idx_user_sub_active_order(user_id, status, end_time, id)`

用于 `GetActiveDistributorSubscriptionUsage` 与 active subscription 列表查询。排序中的 `CASE grant_reason/token_limit` 仍需计算，但过滤与基础顺序更稳定。

### `DB.subscription_pre_consume_records`

现有结构已包含 `request_id` 唯一索引、`user_id` 索引、`user_subscription_id` 索引、`updated_at` 索引。当前清理函数按 `updated_at < ?` 删除。

本期不新增泛化复合索引。仅检查既有索引是否存在；若未来优化清理，再评估 `updated_at` 或 `(status, updated_at)`，不得无查询依据添加 `created_at` 类索引。

### `sub2api.usage_logs`

本规格不修改 `sub2api` 代码库，只输出外部 runbook：

- 用 `EXPLAIN (ANALYZE, BUFFERS)` 比较 `usage_logs(created_at, model)` 与 `usage_logs(model, created_at)`。
- 先建一个索引观察，不一次性增加多个大索引。
- 若要实现聚合表查询改写，必须在 `sub2api` 仓库另开规格和计划。

## 查询改写设计

### 免费套餐趋势读取聚合表

`buildFreeUserHistory` 改为：

1. 查询 `free_users` 权威总量，仍来自 `user_subscriptions.token_used`。
2. 查询入榜用户的免费/试用订阅窗口。
3. 优先从 `free_subscription_usage_hourly` 读取已确认完整的 `subscription_id` 和 `hour_index`。
4. 对未被回填/聚合 checkpoint 覆盖的 `log_id` 范围，使用旧 `other` 明细算法补齐。
5. 同一日志同时有新列和 `other` 时只能计一次。
6. 输出仍补齐 24 小时，`tokens` 与 `cumulative_tokens` 精确等于旧算法。

验收要求：同一批测试数据下，新算法输出与旧算法逐点一致。

### 用户日志列表保持精确 total

`GetUserLogs` / `GetUserLogsWithFilter` 保持返回 `total`，但利用索引降低成本：

- 对 `user_id + 时间范围` 查询，保持现有 filter 语义。
- `Order("logs.id desc")` 默认排序不变。
- `total` 不做跨请求 TTL 缓存。
- 可用 singleflight 合并同时发生且完整 filter/page 一致的请求，返回同一精确查询结果。
- 若未来产品显式接受短暂陈旧，才能设计 `total + page` 同一快照缓存；本期不得作为优化手段。

### `SumUsedQuotaWithFilter` 使用精确小时聚合

适用条件：

- 查询只包含聚合表支持的维度。
- 时间范围至少覆盖一个完整小时。

不适用条件：

- `request_id` / `upstream_request_id` 等高基数字段过滤。
- `token_name`、`username`、`model_name LIKE`、`is_stream` 或任意聚合表没有覆盖的过滤条件。

回退：不适用时继续走 `logs` 明细表，保证精确。

### 运营统计避免拉全量明细

`model/admin_ops.go` 这类统计如果只需要 count/sum，不应 `Find(&logs)` 后在 Go 层求和。应改成 SQL 聚合：

```sql
SELECT
  COUNT(*) AS requests,
  SUM(CASE WHEN type = ? THEN 1 ELSE 0 END) AS errors,
  SUM(...) AS tokens
FROM logs
WHERE ...
```

这不降低精度，只把计算下推到数据库，并减少网络与 Go 内存分配。

## 缓存、singleflight 与限流边界

缓存只能用于降低重复展示查询，不改变精度定义。

### 可缓存对象

- `/api/rankings` 完整 snapshot：沿用现有 rankings cache，这是已有产品行为。
- 用户日志统计 `SumUsedQuotaWithFilter`：可按完整 filter 做短 TTL 缓存，例如 `5-15 s`；缓存值必须来自完整精确查询或精确聚合 + 明细补齐。
- 用户 active subscription usage 展示接口：可按 `user_id` 做极短 TTL 缓存，例如 `1-3 s`，但函数命名必须带 `Display` / `Stats` / `Cache` 等展示语义。

### 不可缓存对象

- 计费扣减、预扣费、结算、退款。
- API Key 限额判断。
- token limit/session 判断。
- `PreConsume*`、`PostConsume*`、`Validate*`、settlement、refund、relay 热路径中的强一致函数。

### 强隔离要求

- 缓存函数不能替换 `PreConsumeUserSubscriptionByUnits`、`PostConsumeUserSubscriptionTokenDelta`、token limit/session、API Key 限额、结算、退款等路径。
- 新增测试或静态断言必须覆盖 relay/preconsume/settlement 不调用统计缓存。
- `refresh=true` 只影响展示统计接口，不参与扣费、预扣费、结算、退款。
- 缓存值不得是估算值；`refresh=true` 必须绕过缓存并返回当前精确结果。

### 限流与防抖

建议：

- 对 `/v1/dashboard/billing/usage`、`/api/usage/token/`、`/api/log/self/stat`、`/api/log/self` 增加独立低成本限流。
- 后端可对相同 user/filter 的统计请求做 singleflight，避免并发重复扫库。

精度要求：singleflight 只合并同时发生的相同查询，返回同一精确结果；不返回估算。

前端请求合并属于可选后续优化，不纳入本期必要实现，避免扩大 `web/default` 文件边界。

## 实施批次与文件边界

后续实现计划必须按以下边界拆分，避免多个子代理同时修改同一热点文件。

### Batch 1A：索引与迁移骨架（串行独占）

文件边界：

- `model/main.go`
- `model/log.go`
- `model/subscription.go`
- `model/*migration*_test.go` 或新增迁移测试文件

内容：

- 新增日志派生列的可重复迁移骨架，作用于 `LOG_DB`。
- 新增 `user_subscriptions` 小表复合索引。
- 新增 PostgreSQL 大表索引 runbook / 方言迁移入口，但不在启动强路径直接建大索引。

### Batch 1B：运营统计 SQL 下推（可与非 `model/log.go` 任务并发）

文件边界：

- `model/admin_ops.go`
- `model/admin_ops_test.go`

内容：

- 把只需 count/sum 的运营统计从 `Find + Go 汇总` 改为 SQL 聚合。
- 测试数值一致，并用 SQL logger 证明不再拉全量明细。

### Batch 2：日志派生列写入与回填（串行独占）

文件边界：

- `model/log.go`
- `model/main.go`
- 新增 `model/log_backfill*.go`
- 新增 `model/log_backfill*_test.go`

内容：

- `RecordConsumeLog` / batch insert 写入新派生列。
- 回填按 `logs.id` checkpoint 运行，作用于 `LOG_DB`，checkpoint 存在 `DB`。
- 回填中断后可恢复。

### Batch 3：免费套餐趋势精确查询（依赖 Batch 2，串行）

文件边界：

- `model/usedata_rankings.go`
- `model/usedata_rankings_test.go`
- `service/rankings.go`
- `service/rankings_test.go`

内容：

- 新列可用时使用列过滤。
- 回填未覆盖范围使用旧 `other` fallback。
- 端到端测试证明新旧算法逐点一致，不暴露内部 ID。

### Batch 4：幂等聚合/outbox（串行独占）

文件边界：

- `model/log_coalescer.go`
- `model/log.go`
- 新增 `model/log_aggregation*.go`
- 新增 `model/log_aggregation*_test.go`

内容：

- 新增 `log_aggregation_events`。
- 新增 `free_subscription_usage_hourly` 与 `log_usage_hourly`。
- 消费日志入库后创建幂等聚合事件。
- 后台或同步轻量处理聚合；失败留下可重放事件。

### Batch 5：展示统计缓存 / singleflight / 限流（查询稳定后）

文件边界：

- 仅统计展示 controller/service。
- 不修改 relay、preconsume、settlement、refund、token limit/session 强一致路径。

内容：

- 相同完整 filter 的 singleflight。
- 短 TTL 精确缓存。
- `refresh=true` 绕过缓存。

### Batch 6：`sub2api` 外部 runbook

文件边界：

- `docs/superpowers/reports` 或部署 runbook 文档。

内容：

- 记录 `sub2api.usage_logs` 的 EXPLAIN、候选索引和验收指标。
- 不在 `new-api` 仓库实现 `sub2api` 查询改写。

## 部署与迁移策略

### 阶段 1：只加索引与查询下推

目标：低风险快速降低慢查询。

内容：

- 新增 `LOG_DB.logs` 复合索引 runbook，先建 `idx_logs_user_created_id` 与 `idx_logs_user_type_created_id`。
- 新增 `DB.user_subscriptions` 复合索引。
- 把只需聚合的代码从 `Find + Go 汇总` 改为 SQL 聚合。
- 对排行榜候选查询增加 `Limit` 不是可接受优化，因为会影响精度；不得使用。

验证：

- PostgreSQL 用 `EXPLAIN (ANALYZE, BUFFERS)` 对比 `GetRankingFreeUserLogCandidates` 与用户日志 count。
- 单测覆盖 SQL 条件与输出一致性。

### 阶段 2：日志派生列与回填

目标：让订阅归属查询不再依赖扫描 JSON 文本。

内容：

- 新增 `logs.subscription_id`、`logs.subscription_tokens_consumed` 等 nullable 列。
- 写入新日志时同步填充列。
- 增加后台回填任务。
- 查询同时支持新列与旧 `other` fallback，并保证互斥不重复。

验证：

- 同一批 `other` 样本，新列解析结果与旧 Go 解析一致。
- 回填前、回填中、回填后免费趋势结果一致。

### 阶段 3：精确聚合表

目标：高频统计从扫明细转为查精确聚合。

内容：

- 新增 `log_aggregation_events`。
- 新增 `free_subscription_usage_hourly`。
- 新增 `log_usage_hourly`。
- 写日志后以 `logs.id` 为事件键幂等聚合。
- 增加对账和修复命令。
- 接口读取聚合表，并用明细表补齐非整小时边界。

验证：

- 聚合表查询结果与明细表全量扫描结果一致。
- 重复重放同一 `log_id` 不会多算。
- 边界时间测试覆盖：同小时、跨小时、整点、非整点、空结果、多用户、多 token、多 channel。

### 阶段 4：`sub2api` 外部执行清单

目标：减少 30 天 `usage_logs` 聚合对宿主机 CPU 的冲击，但不在本仓实现。

内容：

- 输出候选索引 SQL 与 EXPLAIN 验证步骤。
- 标明若要改查询为精确聚合表，必须在 `sub2api` 仓库另开规格和计划。

## 测试计划

### 单元测试

- `model/log_test.go`
  - `RecordConsumeLog` 从 `Other` 提取 `subscription_id`、`subscription_tokens_consumed`、`billing_source`、`endpoint`。
  - 覆盖 `int`、`int64`、`string`、`json.Number`、显式 `0`。
  - `metered_tokens = 0` 显式零值按 0 统计，只有 NULL 回退 prompt + completion。

- `model/log_backfill_test.go`
  - `LOG_DB != DB` 时，新列迁移和回填作用在 `LOG_DB`。
  - checkpoint 存在 `DB`，回填失败后可从 checkpoint 恢复。
  - 同一条日志同时有新列和 `other` 时不重复计数。

- `model/usedata_rankings_test.go`
  - model 查询/列解析保持旧候选规则。
  - 付费订阅、奖励订阅、非入榜用户、软删除用户、缺失 `subscription_id`、缺失或非正数 `subscription_tokens_consumed` 都不误计。

- `service/rankings_test.go`
  - 新旧免费趋势算法逐点一致。
  - 多个重叠订阅按 `subscription_id` 唯一归属。
  - 回填前、回填中、回填后 `free_user_history` 一致且不暴露内部 ID。

- `model/log_aggregation_test.go`
  - 单条日志生成幂等聚合事件。
  - 聚合失败落补偿记录。
  - 重放同一 `log_id + aggregate_name` 不多算。
  - 聚合表查询与明细扫描一致。
  - unsupported filter（`request_id`、`upstream_request_id`、`is_stream`、`token_name`、`model_name LIKE` 等）回退明细扫描。

- `model/admin_ops_test.go`
  - 运营统计数值与旧 Go 汇总一致。
  - SQL logger 证明不再 `Find` 全量日志。

- `model/migration_test.go`
  - SQLite 新列迁移可重复执行。
  - 索引存在性检查可重复执行。
  - PostgreSQL DryRun / 集成测试确认通用启动迁移不包含 `CREATE INDEX CONCURRENTLY`；在线索引脚本单独执行。

### 集成测试

- 构造 10 万条 `logs` 样本，验证：
  - 免费趋势结果与旧算法一致。
  - 用户日志列表 total 一致。
  - 统计接口返回值一致。
  - 精确聚合 + 明细边界补齐与全量明细扫描一致。

### 线上验证

上线前记录基线：

- `usedata_rankings.go:123` 慢 SQL 的 rows、shared/read buffers、execution time。
- `log.go:610` 用户日志 count 的 buffers 与 execution time。
- 10 分钟窗口内 `SLOW SQL >= 200ms` 数量。
- `pg_stat_user_indexes` 旧索引 scan 命中。
- `log_aggregation_events` backlog。

上线后验收：

- `usedata_rankings.go:123` 不再一次返回约 `9.5 万` 候选行。
- 用户日志 count 的 shared/read buffers 与 execution time 比基线下降。
- 新增索引在 `pg_stat_user_indexes` 中有 scan 命中。
- 10 分钟窗口内 `usedata_rankings.go:123`、`log.go:610` 相关慢 SQL 数量下降。
- `log_aggregation_events` backlog 为 `0` 或低于明确运维阈值。
- 没有新增订阅预扣、结算、退款、订阅并发错误。

## 验收标准

1. 免费套餐排行榜总量与旧版本一致。
2. 免费套餐 24 小时趋势每个用户、每个小时的 `tokens` 与 `cumulative_tokens` 与旧算法一致。
3. 用户日志列表的 `total`、分页内容、排序与旧版本一致。
4. 用户日志统计的 `quota`、`total_tokens`、`rpm`、`tpm` 与旧版本一致。
5. `metered_tokens = 0` 显式零值语义不变。
6. 线上 `GetRankingFreeUserLogCandidates` 不再出现一次读取约 9 万行、耗时 30 秒的查询形态。
7. `logs` 相关慢 SQL 数量下降，且没有新增计费、结算、订阅并发错误。
8. 三库迁移可重复执行，不破坏 SQLite、MySQL、PostgreSQL。
9. 不需要资源隔离即可缓解当前性能压力；若硬件仍饱和，资源隔离只能作为后续容量规划，不属于本规格实现范围。

## 风险与处理

### 写放大

新增聚合表会增加消费日志写入后的数据库写操作。

处理：

- 使用幂等 ledger/outbox；同一 `log_id + aggregate_name` 最多应用一次。
- 聚合失败不影响主请求结算，但必须写入可重放补偿记录。
- 保留明细日志为权威来源，可随时重建聚合表。

### 回填期间结果必须完整

处理：

- API 在回填完成前使用「已确认完整聚合区间 + 未确认明细 fallback」。
- `options` 中记录回填进度与完成标记。
- fallback 与新列/聚合结果必须按 `log_id` 互斥，不重复、不漏计。
- 回填完成后才允许关闭旧 `other` fallback。

### PostgreSQL 在线建索引占用 IO

处理：

- 使用 `CREATE INDEX CONCURRENTLY`。
- 低峰执行。
- 一次只建一个大索引。
- 执行前设置 `lock_timeout`、`statement_timeout`。
- 执行后检查 invalid index。
- 建索引前后记录 `pg_stat_progress_create_index`、`pg_stat_user_indexes` 和慢 SQL 指标。

### 三库 SQL 差异

处理：

- 迁移和 upsert 封装在 `model` 层。
- PostgreSQL 在线建索引用部署脚本，不放入通用 AutoMigrate 强路径。
- 优先使用 GORM `clause.OnConflict`；只有 GORM 生成 SQL 不满足时才按 `common.LogSqlType` 或等价日志库方言封装 raw SQL。
- 单测覆盖 SQLite 实际行为；MySQL/PostgreSQL 用 DryRun 或集成环境覆盖 SQL 生成。