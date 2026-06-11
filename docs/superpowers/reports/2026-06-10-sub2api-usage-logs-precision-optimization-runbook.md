# sub2api usage_logs 精确优化 runbook

## 背景

外部 `sub2api.usage_logs` 表约 555 万行。现场 PostgreSQL 日志显示，30 天窗口按 `model` 聚合会触发 parallel worker，并出现查询被取消的情况。

本 runbook 只提供外部执行清单：用同精度索引和同精度聚合方案降低 30 天模型聚合压力。它不修改本仓代码，也不把 `sub2api` 的实现任务纳入 `new-api`。

## 禁止项

- 不在 `new-api` 仓库实现 `sub2api` 查询改写。
- 不降低精度：结果必须与明细扫描逐字段一致。
- 不抽样，不近似，不缩短 30 天或用户指定的查询时间范围。
- 不靠资源隔离解决；资源隔离只能作为容量规划，不能替代查询优化。
- 不一次性创建多个大索引。

## 基线 EXPLAIN

先在 `sub2api` 数据库连接上记录当前查询计划。必须保留完整 SQL、参数、执行时间和 buffers。

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT
  model,
  COUNT(*) AS request_count,
  SUM(prompt_tokens) AS prompt_tokens,
  SUM(completion_tokens) AS completion_tokens,
  SUM(total_tokens) AS total_tokens
FROM usage_logs
WHERE created_at >= $1
  AND created_at < $2
  AND model LIKE 'gpt%'
GROUP BY model
ORDER BY total_tokens DESC;
```

如果线上真实 SQL 字段名或过滤条件不同，以真实 SQL 为准，但 EXPLAIN 模板必须保留 `EXPLAIN (ANALYZE, BUFFERS)`，并覆盖同一个 30 天窗口。

## 候选索引

两个候选索引用于比较不同过滤/分组形态：

```sql
CREATE INDEX CONCURRENTLY idx_usage_logs_created_model
ON usage_logs (created_at, model);

CREATE INDEX CONCURRENTLY idx_usage_logs_model_created
ON usage_logs (model, created_at);
```

执行规则：

1. PostgreSQL 低峰执行。
2. 单连接、非事务执行；`CREATE INDEX CONCURRENTLY` 不能放进事务。
3. 只允许先建一个索引并观察，不一次性建多个大索引。
4. 建完后用同一条 30 天聚合 SQL 再跑 `EXPLAIN (ANALYZE, BUFFERS)`。
5. 对比 execution time、shared/read buffers、parallel worker 数量、CPU 占用和取消次数。
6. 只有第一个索引无法满足目标，且确认写入延迟可接受时，才评估另一个候选索引。

优先顺序需要按真实查询选择：

- 若时间范围选择性更强，优先评估 `idx_usage_logs_created_model`。
- 若 `model` 前缀过滤或等值过滤选择性更强，优先评估 `idx_usage_logs_model_created`。

## invalid index 检查

并发建索引被取消或超时时，需要检查 invalid index：

```sql
SELECT
  c.relname AS index_name,
  i.indisvalid,
  i.indisready
FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
WHERE c.relname IN (
  'idx_usage_logs_created_model',
  'idx_usage_logs_model_created'
);
```

若查询结果出现 `indisvalid = false`，先确认 `index_name` 正是本 runbook 本次失败构建遗留的目标索引，再在低峰、非事务中一次只清理一个 invalid index。不得删除 `indisvalid = true` 的候选索引，也不得同时复制删除两个候选索引。

### 清理 `idx_usage_logs_created_model`

仅当上方查询返回 `index_name = 'idx_usage_logs_created_model'` 且 `indisvalid = false`，并确认它属于本 runbook 本次失败构建时，才执行：

```sql
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_created_model;
```

若 `idx_usage_logs_created_model` 的 `indisvalid = true`，保留该索引，不要删除重建。

### 清理 `idx_usage_logs_model_created`

仅当上方查询返回 `index_name = 'idx_usage_logs_model_created'` 且 `indisvalid = false`，并确认它属于本 runbook 本次失败构建时，才执行：

```sql
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_model_created;
```

若 `idx_usage_logs_model_created` 的 `indisvalid = true`，保留该索引，不要删除重建。

## 同精度聚合表方案

若单纯索引仍无法把 30 天按 `model` 聚合降到可接受范围，需要在 `sub2api` 仓库另开规格和计划，设计同精度聚合表。不得在 `new-api` 仓库实现。

推荐方案：完整小时/日聚合 + 边界明细补齐。

- 完整小时桶或完整日桶：从聚合表读取精确累计值。
- 起始边界与结束边界：继续扫描 `usage_logs` 明细补齐非完整桶。
- 聚合维度：至少包含真实查询使用的 `model` 和时间桶；如还有用户、渠道、key、状态等过滤条件，必须进入聚合键或继续由明细补齐。
- 聚合输入：以明细日志唯一 ID 做幂等 ledger，避免重复累计或漏计。
- 校验：任意查询结果必须与明细扫描逐字段一致，包括 request count、prompt tokens、completion tokens、total tokens 和按 model 分组结果。

验收方式：对同一时间窗口同时执行「聚合表 + 边界明细补齐」和原明细扫描，逐字段 diff；任何字段不一致都不能上线。

## 观察指标

每次只变更一个索引或一个聚合读取策略，观察：

- 30 天聚合查询的 execution time 和 buffers。
- PostgreSQL parallel worker 启动数量和取消次数。
- 数据库 CPU、IO、连接池等待。
- `usage_logs` 写入延迟。
- 聚合结果与明细扫描逐字段一致性。

如果新增索引没有命中，或写入延迟明显恶化，停止继续建索引并回滚该候选。