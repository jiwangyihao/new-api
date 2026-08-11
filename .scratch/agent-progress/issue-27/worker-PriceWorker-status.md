# PriceWorker 状态

## 已实现

- 新增 `model/credit_valuation_price_backfill.go`，实现合同要求的请求、诊断、报告类型及唯一导出入口 `RunCreditValuationPlanPriceMigration`。
- 复用 `subscriptionPlanPriceDiagnosticQuery`、`ParseDecimalAmountMicros` 与 `sqliteSubscriptionPlanPriceRoundtripMatches`，按 SQLite/PostgreSQL `CAST(... AS TEXT)`、MySQL `CAST(... AS CHAR)` 读取原始价格文本，全程不经 `float64`。
- 全表按计划 ID 稳定排序；已有 nullable micros 值计入 total/already-exact 且不重写；缺失值严格区分 invalid、negative、precision、overflow、roundtrip mismatch，保留原始文本并按 ID 输出诊断。
- dry-run 零写且报告可重复；候选 ID 在 dry-run/apply 中使用相同稳定批次边界，默认批大小 100。
- apply 在单一事务内先完成全表诊断，非法价格以包装 `ErrSubscriptionPlanPriceInvalid` fail closed；合法时仅更新 `id=? AND price_amount_micros IS NULL`，逐批核对 RowsAffected，任一失败整体回滚；RowsBackfilled 只统计成功提交的 UPDATE。
- 新增 `model/credit_valuation_price_backfill_test.go`，覆盖精确回填、已有 exact、合法零价、dry-run 稳定零写、负数、非法文本、超六位精度、int64 溢出、SQLite roundtrip mismatch、非法 apply 零写、batch=1 边界、失败回滚、重跑幂等、三方言 cast 与 JSON 字段合同。

## 集成依赖与检查

- 已确认 Marker/Engine Worker 落地共享 `CreditValuationMigrationBatchBoundary{Entity, StartID, EndID, Rows}` 合同。
- 两个新增 Go 文件的 LSP diagnostics 均为 `OK`。
- 遵循并行任务约束，未运行 formatter、测试、构建、lint 或项目级命令；最终统一验证由协调器执行。
