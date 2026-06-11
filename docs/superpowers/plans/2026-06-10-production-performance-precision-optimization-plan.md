# 线上性能精确优化实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。不要使用 worktree；直接在当前主分支工作。当前工作区可能已有与本任务无关的改动，必须只触碰本任务列出的文件。

**目标：** 在不隔离资源、不降低查询精度的前提下，缓解线上 `logs` 大表统计/排行榜/用户日志慢查询压力，并为 `sub2api` 同精度优化提供外部 runbook。

**架构：** 先把只读统计下推到 SQL，再为 `LOG_DB.logs` 增加可空派生列与可恢复回填；随后让免费套餐趋势在新列、旧 `other` fallback 与幂等聚合之间保持逐点一致；最后增加日志聚合 ledger/outbox、展示统计精确缓存/singleflight 和外部 runbook。计费、预扣、结算、退款、token limit/session 强一致路径不得使用展示缓存或估算值。

**技术栈：** Go 1.22+、GORM v2、SQLite/MySQL/PostgreSQL、Gin、go test。所有 JSON 解析/序列化必须使用 `common.*` 包装函数。

---

## 规格与约束

- 规格文件：`docs/superpowers/specs/2026-06-10-production-performance-precision-optimization-spec.md`。
- 项目规则：根目录 `AGENTS.md`。数据库必须兼容 SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6。
- 不做资源隔离、不迁机器、不设置容器 CPU quota。
- 不降低查询精度：不估算 total、不抽样、不缩短用户时间范围、不只查前 N 条候选日志、不用 `quota_data` 近似免费套餐趋势。
- `LOG_DB` 可能与 `DB` 不同。日志列、日志索引、日志回填、日志聚合读取都作用于 `LOG_DB`；业务 checkpoint 可存在 `DB.options`。
- `logs` 大表复合索引不得通过 GORM tag / 通用 AutoMigrate 启动强路径创建；PostgreSQL 在线建索引只进入 runbook 或显式方言低峰步骤。
- 新建子代理提示词必须包含本计划完整路径和必要上下文；新代理提示词不少于 2000 字。

## 文件结构与职责

### Batch 1A：LOG_DB 迁移骨架与小表索引（串行独占）

- 修改：`model/log.go`
  - `Log` 增加可空派生列字段，必须能区分 NULL 与真实零值：`SubscriptionID *int`、`SubscriptionTokensConsumed *int64`、`BillingSource *string`、`Endpoint *string` 或等价 nullable 类型。
  - 字段不能带会让 PostgreSQL 9.6 大表重写的 DB default / NOT NULL，也不能带大表复合 index tag。
  - 增加从 `Other` map 提取派生字段的 helper。
- 修改：`model/main.go`
  - 统一日志 schema 迁移入口，例如 `migrateLogSchema(db *gorm.DB) error`。
  - `migrateLOGDB()` 调用统一日志 schema 迁移入口。
  - `migrateDB()` / `migrateDBFast()` 在 `LOG_SQL_DSN != ""` 时不得 `AutoMigrate(&Log{})` 到业务库；默认同库模式必须显式调用同一套日志 schema 迁移。
  - `DB.user_subscriptions` 小表索引可在主迁移中确保存在。
- 修改：`model/subscription.go`
  - 在 `UserSubscription` 上增加三库通用小表索引 tag 或迁移 helper：`idx_user_sub_active_order(user_id,status,end_time,id)`。不得改变排序语义。
- 创建：`model/log_migration_test.go`
  - 覆盖 `LOG_DB != DB` 时日志列迁移作用于 `LOG_DB`，业务库不被错误迁移 `logs`。
  - 覆盖 `LOG_DB == DB` 默认同库模式也会执行日志 schema 迁移。
  - 覆盖日志派生列 nullable、无默认值、无 NOT NULL。
  - 覆盖通用迁移不生成 PostgreSQL `CREATE INDEX CONCURRENTLY`，不通过 GORM tag 生成大表复合索引。

### Batch 1B：AdminOps 统计 SQL 下推（可并发）

- 修改：`model/admin_ops.go`
  - `GetAdminOpsTrafficStats` 改为 SQL 聚合，不再 `Find` 全量日志后 Go 层汇总。
  - 保持 `adminOpsLogTokens` 精确语义：`metered_tokens != NULL` 时使用它；显式负数归 0；NULL 时使用 `prompt_tokens + completion_tokens`，负数归 0。
- 修改：`model/admin_ops_test.go`
  - 增加 SQL logger，证明不再执行明细投影查询。
  - 覆盖 `metered_tokens = 0`、NULL fallback、负数 token。

### Batch 2：日志派生列写入与回填（依赖 Batch 1A，串行独占）

- 修改：`model/log.go`
  - `RecordConsumeLog`、`RecordTaskBillingLog` 写日志前调用派生字段填充 helper。
  - `insertConsumeLogDirect` / `insertConsumeLogsDirect` 之前确保批量日志都已填充派生字段。
- 创建：`model/log_backfill.go`
  - 定义 checkpoint option key，例如 `LogDerivedColumnsBackfillCheckpoint`、`LogDerivedColumnsBackfillComplete`。
  - 实现 `BackfillLogDerivedColumnsBatch(limit int) (processed int64, complete bool, err error)`。
  - 按 `logs.id` 单调扫描 `LOG_DB`，checkpoint 存在 `DB.options`。
  - 每批只在成功处理连续 id 区间后推进 checkpoint。
- 创建：`model/log_backfill_test.go`
  - `LOG_DB != DB` 时，扫描和更新发生在 `LOG_DB`，checkpoint 写入 `DB`。
  - 中途失败不推进 checkpoint；重启后从 checkpoint 恢复。
  - 同一日志同时有新列和 `other` 时，后续趋势算法只计一次。

### Batch 3：免费套餐趋势精确查询（依赖 Batch 2，串行）

- 修改：`model/usedata_rankings.go`
- 创建：`model/usedata_rankings_test.go`
- 修改：`service/rankings.go`
- 修改：`service/rankings_test.go`

### Batch 4：幂等日志聚合 ledger/outbox（依赖 Batch 2，串行独占）

- 修改：`model/log_coalescer.go`
- 修改：`model/log.go`
- 修改：`model/main.go`
- 创建：`model/log_aggregation.go`
- 创建：`model/log_aggregation_test.go`

### Batch 5：展示统计精确缓存 / singleflight（依赖 Batch 4，串行）

- 修改或创建：`model/log_stats_cache.go`
- 修改：`model/log.go`
- 修改：`controller/log.go`
- 修改：`controller/log_stat_token_test.go` 或 `controller/log_usage_analytics_test.go`

### Batch 6：索引与 `sub2api` 外部 runbook（可并发）

- 创建：`docs/superpowers/reports/2026-06-10-new-api-log-index-runbook.md`
- 创建：`docs/superpowers/reports/2026-06-10-sub2api-usage-logs-precision-optimization-runbook.md`

---

## 任务 1：AdminOps 统计 SQL 下推

**文件：**
- 修改：`model/admin_ops.go`
- 修改：`model/admin_ops_test.go`

- [ ] **步骤 1：编写红灯测试，证明当前实现会拉取明细日志**

在 `model/admin_ops_test.go` 增加 SQL logger，构造 consume/error 日志样本。logger 只统计明细投影查询，不能把后续聚合 SQL 误判为失败：

```go
type adminOpsSQLCaptureLogger struct {
    logger.Interface
    detailSelects atomic.Int64
}

func (l *adminOpsSQLCaptureLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
    sql, rows := fc()
    normalized := strings.ToLower(strings.Join(strings.Fields(sql), " "))
    normalized = strings.ReplaceAll(normalized, "`", "")
    normalized = strings.ReplaceAll(normalized, "\"", "")
    if strings.Contains(normalized, "select type,prompt_tokens,completion_tokens,metered_tokens from logs") ||
        (strings.Contains(normalized, "select type, prompt_tokens, completion_tokens, metered_tokens") && strings.Contains(normalized, "from logs") && !strings.Contains(normalized, "sum(") && !strings.Contains(normalized, "count(")) {
        l.detailSelects.Add(1)
    }
    l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}
```

测试名：`TestGetAdminOpsTrafficStatsUsesSQLAggregation`。

样本：

- consume：prompt=10 completion=5 metered NULL，贡献 15。
- consume：metered=0 prompt=100 completion=100，贡献 0。
- error：prompt=4 completion=6 metered NULL，贡献 10，errors +1。
- consume：metered=-7，贡献 0。

断言：

```go
stats, err := GetAdminOpsTrafficStats(start, end)
require.NoError(t, err)
assert.Equal(t, int64(4), stats.Requests)
assert.Equal(t, int64(1), stats.Errors)
assert.Equal(t, int64(25), stats.TotalTokens)
assert.Zero(t, capture.detailSelects.Load(), "traffic stats must not load all log rows")
```

- [ ] **步骤 2：运行红灯测试**

运行：

```bash
go test -p=1 ./model -run TestGetAdminOpsTrafficStatsUsesSQLAggregation -count=1
```

预期：FAIL，原因是当前实现仍执行明细投影查询。

- [ ] **步骤 3：实现 SQL 聚合**

将 `GetAdminOpsTrafficStats` 改为单条聚合查询，保留精确 token 语义：

```go
var row struct {
    Requests    int64
    Errors      int64
    TotalTokens int64
}
err := LOG_DB.Model(&Log{}).
    Select(`COUNT(*) AS requests,
        COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0) AS errors,
        COALESCE(SUM(CASE
            WHEN metered_tokens IS NOT NULL AND metered_tokens > 0 THEN metered_tokens
            WHEN metered_tokens IS NOT NULL THEN 0
            WHEN prompt_tokens + completion_tokens > 0 THEN prompt_tokens + completion_tokens
            ELSE 0 END), 0) AS total_tokens`, LogTypeError).
    Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
    Where("created_at >= ? AND created_at <= ?", startTimestamp, endTimestamp).
    Scan(&row).Error
```

- [ ] **步骤 4：运行测试验证通过**

运行：

```bash
go test -p=1 ./model -run 'TestGetAdminOpsTrafficStats|TestGetAdminOps' -count=1
```

预期：PASS。

## 任务 2：LOG_DB 派生列迁移与写入

**文件：**
- 修改：`model/log.go`
- 修改：`model/main.go`
- 修改：`model/subscription.go`
- 创建：`model/log_migration_test.go`
- 创建：`model/log_backfill.go`
- 创建：`model/log_backfill_test.go`

- [ ] **步骤 1：编写红灯测试：LOG_DB 独立迁移和默认同库迁移**

在 `model/log_migration_test.go` 建两个 SQLite 内存库，一个赋给 `DB`，一个赋给 `LOG_DB`。调用新 helper `migrateLogSchema(LOG_DB)` 前，断言 `LOG_DB` 不存在 `subscription_id`；调用后断言仅 `LOG_DB.logs` 存在 `subscription_id`、`subscription_tokens_consumed`、`billing_source`、`endpoint`，`DB.logs` 不被错误迁移。

另写 `TestMigrateDefaultDBRunsLogSchemaWhenLogDBIsPrimaryDB`：`LOG_DB = DB` 时调用 `migrateLogSchema(DB)`，断言同一个 DB 上日志列存在。

运行：

```bash
go test -p=1 ./model -run 'TestMigrate(LogDerivedColumnsUsesLOGDB|DefaultDBRunsLogSchema)' -count=1
```

预期：FAIL，helper 未定义。

- [ ] **步骤 2：编写红灯测试：nullable schema 与派生字段解析**

在 `model/log_migration_test.go` 增加 `TestMigrateLogDerivedColumnsAreNullableWithoutDefaults`，SQLite 用 `PRAGMA table_info(logs)` 断言四列：

- `notnull == 0`
- `dflt_value IS NULL`

增加 `TestFillLogDerivedFieldsFromOther`：

```go
log := &Log{Other: common.MapToJsonStr(map[string]interface{}{
    "subscription_id": float64(123),
    "subscription_tokens_consumed": "456",
    "billing_source": "subscription",
    "request_path": "/v1/responses",
})}
fillLogDerivedFields(log)
require.NotNil(t, log.SubscriptionID)
assert.Equal(t, 123, *log.SubscriptionID)
require.NotNil(t, log.SubscriptionTokensConsumed)
assert.EqualValues(t, 456, *log.SubscriptionTokensConsumed)
require.NotNil(t, log.BillingSource)
assert.Equal(t, "subscription", *log.BillingSource)
require.NotNil(t, log.Endpoint)
assert.Equal(t, "/v1/responses", *log.Endpoint)
```

再插入一条旧日志不填四列，查询后断言四个字段仍为 nil，能区分 NULL 与真实空值。

- [ ] **步骤 3：实现 `Log` 字段与 helper**

在 `Log` 结构中新增：

```go
SubscriptionID             *int    `json:"subscription_id,omitempty" gorm:"column:subscription_id"`
SubscriptionTokensConsumed *int64  `json:"subscription_tokens_consumed,omitempty" gorm:"column:subscription_tokens_consumed"`
BillingSource              *string `json:"billing_source,omitempty" gorm:"column:billing_source;type:varchar(32)"`
Endpoint                   *string `json:"endpoint,omitempty" gorm:"column:endpoint;type:varchar(255)"`
```

不要给这四个字段加 `default`、`not null` 或复合 index tag。

新增 helper：

```go
func fillLogDerivedFields(log *Log) {
    if log == nil || strings.TrimSpace(log.Other) == "" {
        return
    }
    var other map[string]interface{}
    if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
        return
    }
    if v, ok := intFromMapValue(other["subscription_id"]); ok && v > 0 {
        log.SubscriptionID = &v
    }
    if v, ok := int64FromMapValue(other["subscription_tokens_consumed"]); ok && v >= 0 {
        log.SubscriptionTokensConsumed = &v
    }
    if value, ok := stringFromMapValue(other["billing_source"]); ok {
        log.BillingSource = &value
    }
    if value, ok := stringFromMapValue(other["endpoint"]); ok {
        log.Endpoint = &value
    } else if value, ok := stringFromMapValue(other["request_path"]); ok {
        log.Endpoint = &value
    }
}
```

- [ ] **步骤 4：实现统一日志 schema 迁移**

实现：

```go
func migrateLogSchema(db *gorm.DB) error
func migrateLogDerivedColumns(db *gorm.DB) error
```

要求：

- `migrateLogSchema` 负责 `AutoMigrate(&Log{})` 和 `migrateLogDerivedColumns`。
- `migrateLOGDB()` 调用 `migrateLogSchema(LOG_DB)`。
- `migrateDB()` / `migrateDBFast()` 当 `os.Getenv("LOG_SQL_DSN") != ""` 时，不把 `&Log{}` 放入业务库 AutoMigrate；当 `LOG_SQL_DSN == ""` 时，必须对 `DB` 调用同一套日志 schema 迁移。
- `migrateLogDerivedColumns` 先确保 `logs` 表存在；缺失列逐个 `ALTER TABLE logs ADD COLUMN ...`；不设置 DEFAULT / NOT NULL。

- [ ] **步骤 5：写入路径红灯测试**

新增：

- `TestRecordConsumeLogFillsDerivedColumnsFromOther`
- `TestRecordTaskBillingLogFillsDerivedColumnsFromOther`
- `TestInsertConsumeLogsDirectFillsDerivedColumnsForBatch`

断言写入 `LOG_DB` 后四个派生列已落库。

- [ ] **步骤 6：写入路径填充派生列**

在 `RecordConsumeLog`、`RecordTaskBillingLog`、`insertConsumeLogDirect`、`insertConsumeLogsDirect` 前确保调用 `fillLogDerivedFields`。批量插入时逐条填充。

- [ ] **步骤 7：编写回填红灯测试**

在 `model/log_backfill_test.go`：

- `TestBackfillLogDerivedColumnsUsesLOGDBAndDBCheckpoint`
- `TestBackfillLogDerivedColumnsDoesNotAdvanceCheckpointOnUpdateError`
- `TestBackfillLogDerivedColumnsResumesFromCheckpoint`

要求：

- `LOG_DB != DB` 时，扫描和更新发生在 `LOG_DB`，checkpoint 写在 `DB.options`。
- 注入更新失败时 checkpoint 不变。
- 恢复后从旧 checkpoint 继续。

- [ ] **步骤 8：实现回填**

新增 `model/log_backfill.go`：

```go
const (
    optionLogDerivedColumnsBackfillCheckpoint = "LogDerivedColumnsBackfillCheckpoint"
    optionLogDerivedColumnsBackfillComplete = "LogDerivedColumnsBackfillComplete"
)
```

实现：

- 从 `DB.options` 读取 checkpoint。
- 从 `LOG_DB.logs` 查询 `id > checkpoint` 的下一批，按 `id ASC`。
- 对每条日志用 `fillLogDerivedFields`，只更新派生列。
- 整批成功后 checkpoint 推进到最后一条 id。
- 若批次不足 limit，再设置 complete marker。

- [ ] **步骤 9：运行任务 2 测试**

运行：

```bash
go test -p=1 ./model -run 'Test(BackfillLogDerivedColumns|MigrateLogDerivedColumns|MigrateDefaultDBRunsLogSchema|FillLogDerivedFields|RecordConsumeLogFills|RecordTaskBillingLogFills|InsertConsumeLogsDirectFills)' -count=1
```

预期：PASS。

## 任务 3：免费套餐趋势使用派生列并保持旧算法精确一致

**文件：**
- 修改：`model/usedata_rankings.go`
- 创建：`model/usedata_rankings_test.go`
- 修改：`service/rankings.go`
- 修改：`service/rankings_test.go`

- [ ] **步骤 1：编写确定红灯测试：新列优先、Other fallback、无双计**

在 `service/rankings_test.go` 新增 `TestFreeUserHistoryUsesDerivedColumnsWithoutDoubleCountingOtherFallback`，构造三条日志：

1. 日志 A：`SubscriptionID/SubscriptionTokensConsumed=100`，`Other` 中同字段为 `999`，期望计 `100`。
2. 日志 B：只有新列、`Other` 为空，期望计入。
3. 日志 C：只有 `Other`，新列 nil，期望 fallback 计入。

断言第 0 小时 tokens 等于 `100 + B + C`，不是 `999 + B + C`，也不是漏掉 B。

运行：

```bash
go test -p=1 ./service -run TestFreeUserHistoryUsesDerivedColumnsWithoutDoubleCountingOtherFallback -count=1
```

预期：FAIL，当前实现只读 `Other`，日志 A 得到 999 或日志 B 漏计。

- [ ] **步骤 2：编写 model 查询测试**

在 `model/usedata_rankings_test.go` 验证 `GetRankingFreeUserLogCandidates` 会选择派生列字段，且不限制候选数量。

- [ ] **步骤 3：实现候选结构与解析**

`RankingFreeUserLogCandidate` 新增：

```go
SubscriptionID *int
SubscriptionTokensConsumed *int64
```

`buildFreeUserHistory` 中新增 helper：

```go
func rankingCandidateSubscriptionUsage(candidate model.RankingFreeUserLogCandidate) (subID int, consumed int64, ok bool)
```

规则：

- 若新列 `SubscriptionID != nil && SubscriptionTokensConsumed != nil`，使用新列。
- 否则解析 `Other`。
- 任何路径只返回一次，不得新列 + Other 双算。

- [ ] **步骤 4：扩展 service 测试覆盖奖励/软删除/缺失字段**

在既有 `TestGetRankingsSnapshotFreeUserHistoryIgnoresNonPositiveAndDeletedUsers` 上扩充或新增测试，覆盖：

- `grant_reason = model.SubscriptionGrantMonthlyInviteEntitlement` 的奖励订阅不计入免费趋势。
- 软删除用户订阅不计。
- `subscription_tokens_consumed <= 0` 不计。
- 付费订阅不计。

- [ ] **步骤 5：运行任务 3 测试**

运行：

```bash
go test -p=1 ./model -run TestRankingFreeUser -count=1
go test -p=1 ./service -run 'TestGetRankingsSnapshot.*FreeUser|TestFreeUserHistoryUsesDerivedColumns' -count=1
```

预期：PASS。

## 任务 4：日志聚合 ledger/outbox 与精确小时聚合

**文件：**
- 修改：`model/log.go`
- 修改：`model/log_coalescer.go`
- 修改：`model/main.go`
- 创建：`model/log_aggregation.go`
- 创建：`model/log_aggregation_test.go`

- [ ] **步骤 1：编写幂等红灯测试**

在 `model/log_aggregation_test.go`：

- `TestApplyLogUsageAggregationEventIsIdempotentByLogIDAndAggregateName`
- `TestApplyFreeSubscriptionUsageAggregationIsIdempotentByLogIDAndAggregateName`
- `TestFailedAggregationEventRemainsRetryableWithoutDoubleApply`

流程：

- 插入 consume log，`MeteredTokens=0`，prompt/completion 非零，带订阅派生列。
- 创建同一 `log_id + aggregate_name` 事件两次。
- 调用 apply 两次。
- 断言 `log_usage_hourly.request_count == 1`，`metered_tokens_sum == 0`；`free_subscription_usage_hourly.tokens` 不多算。
- 注入一次 apply 失败，断言事件为 failed；恢复后重试成功，只累加一次。

- [ ] **步骤 2：定义模型**

`model/log_aggregation.go` 增加：

```go
type LogAggregationEvent struct { ... }
type FreeSubscriptionUsageHourly struct { ... }
type LogUsageHourly struct { ... }
```

GORM tag：

- `LogAggregationEvent` 唯一索引：`idx_log_agg_event_unique(log_id, aggregate_name)`。
- `LogAggregationEvent.Status` 的 DB default 必须是 `pending`；queue 创建事件时必须显式写 `Status: "pending"`。
- 只有 apply 聚合 upsert 成功后才能置为 `applied`；失败置为 `failed` 并保留 error。
- `LogUsageHourly` 唯一索引使用 `model_key_hash`，不把 `model_name text` 放入唯一键。

- [ ] **步骤 3：迁移新表**

`migrateLogSchema` 增加：

```go
if err := db.AutoMigrate(&LogAggregationEvent{}, &FreeSubscriptionUsageHourly{}, &LogUsageHourly{}); err != nil { return err }
```

新表默认/索引允许 AutoMigrate；这是新表，不是 `logs` 大表复合索引。

- [ ] **步骤 4：实现 queue 与原子 claim/apply**

`queueLogAggregationEventsForLogs(logs []*Log) error`：

- 跳过 `nil`、`Id <= 0`、非 consume/error 日志。
- 为 `log_usage_hourly` 创建 event。
- 若 `SubscriptionID` 和 `SubscriptionTokensConsumed` 有效，为 `free_subscription_usage_hourly` 创建 event。
- 创建 event 时显式设置 `Status: "pending"`；不得依赖默认 `applied`。
- 使用 `clause.OnConflict{DoNothing: true}`。

`ApplyPendingLogAggregationEvents(limit int)`：

- 查询 pending/failed 事件 ID。
- 对每个 event 在 `LOG_DB.Transaction` 中先原子 claim：

```go
res := tx.Model(&LogAggregationEvent{}).
    Where("id = ? AND status IN ?", eventID, []string{"pending", "failed"}).
    Updates(map[string]interface{}{"status": "processing", "updated_at": common.GetTimestamp()})
if res.RowsAffected != 1 { skip }
```

- 从 `LOG_DB` 读取日志。
- 若是免费订阅聚合，需要从 `DB` 批量或单条读取 `user_subscriptions` / `subscription_plans` 元数据，在 Go 层判断免费/试用归属；不得写跨连接 SQL join。
- upsert 聚合表。
- 成功后置 `applied`；失败后置 `failed` 和 error。
- fallback 只能排除 `status='applied'` 的 log_id。

- [ ] **步骤 5：插入日志后创建 event**

`insertConsumeLogDirect`：

```go
if err := LOG_DB.Create(log).Error; err != nil { return err }
if err := queueLogAggregationEventsForLogs([]*Log{log}); err != nil { common.SysError(...) }
return nil
```

`insertConsumeLogsDirect` 批量 create 成功后 queue 全部 logs。

- [ ] **步骤 6：编写边界一致性测试**

新增 `TestLogUsageHourlyAggregationMatchesDetailScanAtBoundaries`：同小时、跨小时、整点、非整点、`created_at == end`、空结果、多用户/多 token/多 channel，断言聚合 + 明细补齐等于全量明细扫描。

- [ ] **步骤 7：运行任务 4 测试**

运行：

```bash
go test -p=1 ./model -run 'Test.*LogAggregation|TestRecordConsumeLog.*Coalesc' -count=1
```

预期：PASS。

## 任务 5：日志统计精确缓存与 refresh

**文件：**
- 修改或创建：`model/log_stats_cache.go`
- 修改：`model/log.go`
- 修改：`controller/log.go`
- 修改：`controller/log_stat_token_test.go` 或 `controller/log_usage_analytics_test.go`

- [ ] **步骤 1：编写 refresh 红灯测试**

在 controller 测试中新增 `TestLogStatRefreshBypassesAndUpdatesCache`：

- 第一次请求 `/api/log/self/stat?type=2`，得到 total_tokens 10。
- 插入第二条日志。
- 请求同一路径不带 refresh，可命中短 TTL 缓存得到旧值 10。
- 请求 `/api/log/self/stat?type=2&refresh=true` 必须返回 total_tokens 20。
- 再请求不带 refresh，应得到 20，证明 refresh 更新缓存。

运行：

```bash
go test -p=1 ./controller -run TestLogStatRefreshBypassesAndUpdatesCache -count=1
```

预期：FAIL，refresh 未实现。

- [ ] **步骤 2：编写完整 cache key 红灯测试**

在 model 测试中新增 `TestSumUsedQuotaCacheKeyIncludesFullFilter`：同 TTL 内依次查询不同 `UserId`、`ModelName`、`TokenId`、`Status`、`IsStream`、`StartTimestamp`、`EndTimestamp`，断言结果互不污染。

- [ ] **步骤 3：编写 singleflight 红灯测试**

新增 `TestSumUsedQuotaSingleflightCoalescesConcurrentExactFilter`：用延迟 SQL logger 或 hook 让 N 个相同 filter 并发，只产生一组查询，所有返回值一致。不同 filter 不合并。

- [ ] **步骤 4：编写强一致路径保护测试**

新增 `TestStrongConsistencyPathsDoNotReferenceLogStatCache`：读取固定文件内容并断言不包含缓存 helper / `SumUsedQuotaWithFilterOptions`：

- `model/subscription.go`
- `service/billing_session.go`
- `service/funding_source.go`
- `service/token_limit_session.go`
- `model/token_limit_preconsume.go`

该测试使用 `os.ReadFile` 读取相对路径；不要 shell grep。

- [ ] **步骤 5：实现 options 结构**

新增：

```go
type LogStatOptions struct { Refresh bool }
func SumUsedQuotaWithFilterOptions(filter LogFilter, options LogStatOptions) (Stat, error)
```

保留 `SumUsedQuotaWithFilter(filter)` 调用新函数，`Refresh=false`。

- [ ] **步骤 6：实现缓存 key 与 singleflight**

- key 包含完整 `LogFilter` 所有字段。
- TTL 5 秒。
- `Refresh=true` 绕过缓存和 singleflight 旧结果，但可更新缓存。
- 不缓存 `GetUserLogsWithFilter` 的 total/page。

- [ ] **步骤 7：controller 接入 refresh**

`GetLogsStat` / `GetLogsSelfStat` 解析：

```go
refresh := c.Query("refresh") == "true"
stat, err := model.SumUsedQuotaWithFilterOptions(filter, model.LogStatOptions{Refresh: refresh})
```

- [ ] **步骤 8：运行任务 5 测试**

运行：

```bash
go test -p=1 ./model -run 'Test.*LogStat|TestSumUsedQuota|TestStrongConsistencyPaths' -count=1
go test -p=1 ./controller -run 'Test.*Log.*Stat|Test.*UsageAnalytics' -count=1
```

预期：PASS。

## 任务 6：索引与 `sub2api` 外部 runbook

**文件：**
- 创建：`docs/superpowers/reports/2026-06-10-new-api-log-index-runbook.md`
- 创建：`docs/superpowers/reports/2026-06-10-sub2api-usage-logs-precision-optimization-runbook.md`

- [ ] **步骤 1：写 new-api logs 索引 runbook**

内容必须包括：

- 不通过 GORM tag / AutoMigrate 创建 `logs` 大表复合索引。
- PostgreSQL：低峰、单连接、非事务、一次一个索引：

```sql
SET lock_timeout = '5s';
SET statement_timeout = '30min';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_user_created_id
ON logs (user_id, created_at, id);
```

以及 `idx_logs_user_type_created_id`。

- invalid index 检查和清理：`pg_index.indisvalid` / `DROP INDEX CONCURRENTLY`。
- MySQL/SQLite 低峰建索引建议。
- 验证：`pg_stat_user_indexes`、慢 SQL、写入延迟。

- [ ] **步骤 2：写 sub2api runbook**

内容必须包括：

- 背景：`sub2api.usage_logs` 约 `555 万` 行，30 天按 model 聚合触发 parallel worker 并被取消。
- 禁止项：不在 `new-api` 仓库实现 `sub2api` 查询改写；不降低精度；不抽样。
- EXPLAIN 模板：

```sql
EXPLAIN (ANALYZE, BUFFERS)
WITH stats AS (...)
SELECT ...;
```

- 候选索引：

```sql
CREATE INDEX CONCURRENTLY idx_usage_logs_created_model
ON usage_logs (created_at, model);

CREATE INDEX CONCURRENTLY idx_usage_logs_model_created
ON usage_logs (model, created_at);
```

- 只允许先建一个索引并观察。
- 同精度聚合表方案：完整小时/日聚合 + 边界明细补齐。

- [ ] **步骤 3：文本自检**

使用 `read` 工具或 Go 测试检查两个 runbook 包含关键短语：

- `EXPLAIN (ANALYZE, BUFFERS)`
- `CREATE INDEX CONCURRENTLY`
- `只允许先建一个索引`
- `不在 new-api 仓库实现 sub2api 查询改写`

不要用 shell grep。

## 最终验证

所有任务完成后，主代理运行：

```bash
go test -p=1 ./model -run 'Test(AdminOps|.*Log|.*Ranking|.*Migration|.*Aggregation|.*Backfill|SumUsedQuota|StrongConsistencyPaths)' -count=1
go test -p=1 ./service -run 'TestGetRankingsSnapshot.*FreeUser|TestFreeUserHistory' -count=1
go test -p=1 ./controller -run 'Test.*Log.*Stat|Test.*UsageAnalytics' -count=1
go test ./model ./service ./controller -count=1
```

如果涉及文档新增，无需前端 typecheck；本期不修改 `web/default`。

## 提交建议

开发完成并验证后，**不得使用目录级 `git add model controller service`**。必须先查看 `git status --short`，确认当前工作区仍可能包含无关 dirty 文件，然后只暂存本任务精确路径：

```bash
git add \
  model/admin_ops.go model/admin_ops_test.go \
  model/log.go model/main.go model/subscription.go \
  model/log_migration_test.go model/log_backfill.go model/log_backfill_test.go \
  model/usedata_rankings.go model/usedata_rankings_test.go \
  service/rankings.go service/rankings_test.go \
  model/log_coalescer.go model/log_aggregation.go model/log_aggregation_test.go \
  model/log_stats_cache.go \
  controller/log.go controller/log_stat_token_test.go controller/log_usage_analytics_test.go \
  docs/superpowers/specs/2026-06-10-production-performance-precision-optimization-spec.md \
  docs/superpowers/plans/2026-06-10-production-performance-precision-optimization-plan.md \
  docs/superpowers/reports/2026-06-10-new-api-log-index-runbook.md \
  docs/superpowers/reports/2026-06-10-sub2api-usage-logs-precision-optimization-runbook.md

git commit -m "perf(logs): 优化日志统计精确查询路径"
```

如果某个列出的文件未被创建或未修改，不要强行暂存；若实现新增了计划未列出的文件，必须先解释其职责并确认它属于本任务范围。