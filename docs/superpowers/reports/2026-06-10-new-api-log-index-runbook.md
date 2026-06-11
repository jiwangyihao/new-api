# new-api logs 大表索引 runbook

## 背景

线上 `new-api` 的 `LOG_DB.logs` 是当前主要慢点之一：表大小约 4.16 GB，活跃行约 254 万。现场慢 SQL 显示：

- `model/usedata_rankings.go:123` 一次读取约 95604 行，耗时约 30 s。
- `model/log.go:610` 用户日志 `count(*)` 以 `user_id + created_at` 时间范围过滤，也进入慢 SQL。

本 runbook 只描述低峰索引执行与验证步骤。应用代码必须在索引不存在时仍保持正确；索引只改善执行计划和 IO 成本，不改变业务语义。

## 禁止项

- 禁止通过 GORM tag 或通用 `AutoMigrate` 在启动强路径创建 `logs` 大表复合索引。
- 禁止降低查询精度：不得返回估算 `total`，不得丢弃符合条件的日志。
- 禁止抽样、近似统计或只扫描前 N 条候选日志。
- 禁止缩短用户选择的时间范围，现有 `start >=` 与 `end <=` 的闭区间语义必须保持。
- 禁止把 PostgreSQL-only 语法写成 SQLite / MySQL / PostgreSQL 通用迁移。

## PostgreSQL 低峰建索引原则

适用对象是 `LOG_DB.logs`。如果 `LOG_DB` 与 `DB` 不同，必须连接到日志库执行。

执行约束：

1. 只在低峰窗口执行。
2. 单连接执行。
3. 非事务执行；`CREATE INDEX CONCURRENTLY` 不能放进显式事务。
4. 一次一个索引；每建完一个索引后先观察，再决定是否继续。
5. 记录开始时间、结束时间、锁等待、慢 SQL 数量、写入延迟和索引命中。

### 第一优先级：用户日志列表与 count

```sql
SET lock_timeout = '5s';
SET statement_timeout = '30min';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_user_created_id
ON logs (user_id, created_at, id);
```

验证 `model/log.go:610` 的用户日志 `count(*)` 和列表查询后，再进入下一个索引。

### 第二优先级：用户消费日志筛选与免费套餐趋势候选

```sql
SET lock_timeout = '5s';
SET statement_timeout = '30min';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_user_type_created_id
ON logs (user_id, type, created_at, id);
```

验证 `model/usedata_rankings.go:123` 相关查询后，再评估是否需要后续索引。只允许先建一个索引并观察，不要在同一个维护窗口内一次性追加多个大索引。

## invalid index 检查与清理

`CREATE INDEX CONCURRENTLY` 被取消、超时或连接中断时，PostgreSQL 可能留下 invalid index。每次建索引后都要检查：

```sql
SELECT
  c.relname AS index_name,
  i.indisvalid,
  i.indisready
FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
WHERE c.relname IN (
  'idx_logs_user_created_id',
  'idx_logs_user_type_created_id'
);
```

若查询结果出现 `indisvalid = false`，先确认 `index_name` 正是本 runbook 本次失败构建遗留的目标索引，再在低峰、非事务中一次只清理一个 invalid index。不得删除 `indisvalid = true` 的候选索引，也不得同时复制删除两个候选索引。

### 清理 `idx_logs_user_created_id`

仅当上方查询返回 `index_name = 'idx_logs_user_created_id'` 且 `indisvalid = false`，并确认它属于本 runbook 本次失败构建时，才执行：

```sql
DROP INDEX CONCURRENTLY IF EXISTS idx_logs_user_created_id;
```

若 `idx_logs_user_created_id` 的 `indisvalid = true`，保留该索引，不要删除重建。

### 清理 `idx_logs_user_type_created_id`

仅当上方查询返回 `index_name = 'idx_logs_user_type_created_id'` 且 `indisvalid = false`，并确认它属于本 runbook 本次失败构建时，才执行：

```sql
DROP INDEX CONCURRENTLY IF EXISTS idx_logs_user_type_created_id;
```

若 `idx_logs_user_type_created_id` 的 `indisvalid = true`，保留该索引，不要删除重建。

## MySQL / SQLite 建索引建议

MySQL 和 SQLite 不支持 PostgreSQL 的 `CREATE INDEX CONCURRENTLY`。不要把 PostgreSQL-only 语法放入通用迁移。

### MySQL >= 5.7.8

建议在低峰窗口用独立部署步骤或显式方言迁移执行。示例：

```sql
CREATE INDEX idx_logs_user_created_id
ON logs (user_id, created_at, id);

CREATE INDEX idx_logs_user_type_created_id
ON logs (user_id, type, created_at, id);
```

执行前先确认索引不存在；大表建索引期间观察写入延迟、复制延迟和锁等待。不同 MySQL 版本、存储引擎和 DDL 参数对在线能力不同，不能假设与 PostgreSQL 并发建索引等价。

### SQLite

SQLite 适用于本地或轻量部署。建索引会阻塞写入，建议在停写或低峰窗口执行：

```sql
CREATE INDEX IF NOT EXISTS idx_logs_user_created_id
ON logs (user_id, created_at, id);

CREATE INDEX IF NOT EXISTS idx_logs_user_type_created_id
ON logs (user_id, type, created_at, id);
```

SQLite 场景也不要通过启动强路径无条件创建所有大索引。

## 验证清单

### 执行计划

对线上慢查询的等价 SQL 使用：

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT count(*)
FROM logs
WHERE user_id = $1
  AND created_at >= $2
  AND created_at <= $3;
```

以及：

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, user_id, created_at, metered_tokens, other
FROM logs
WHERE user_id = ANY($1)
  AND type = 2
  AND created_at >= $2
  AND created_at <= $3
ORDER BY id DESC;
```

重点观察：

- 是否使用 `idx_logs_user_created_id` 或 `idx_logs_user_type_created_id`。
- `shared hit/read buffers` 是否下降。
- `rows`、`execution time` 是否比基线下降。
- 是否出现新的排序、回表或大范围扫描成本。

### 索引命中

```sql
SELECT
  indexrelname,
  idx_scan,
  idx_tup_read,
  idx_tup_fetch
FROM pg_stat_user_indexes
WHERE relname = 'logs'
  AND indexrelname IN (
    'idx_logs_user_created_id',
    'idx_logs_user_type_created_id'
  )
ORDER BY indexrelname;
```

观察至少一个业务周期，确认新增索引有 scan 命中；若没有命中，不继续追加其他大索引。

### 慢 SQL 与写入延迟

建每个索引前后分别记录：

- `model/usedata_rankings.go:123` 的 rows、buffers、execution time。
- `model/log.go:610` 的 buffers、execution time。
- 10 分钟窗口内相关慢 SQL 数量。
- `logs` 写入延迟、连接池等待、数据库 CPU 与 IO。

判断标准：一次只建一个索引后观察。只有确认慢 SQL、buffers 或写入延迟没有恶化，并且索引有命中，才评估下一步。