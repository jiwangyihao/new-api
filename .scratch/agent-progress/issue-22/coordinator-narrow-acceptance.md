# Issue #22 协调器窄验收

## 冻结交付

- Worker 有效完成事件：`msg_7c766d2439aa`，Dispatch `ctx_83928bda6eca`，outcome `succeeded`。
- Worker 收尾 HEAD：`0a51655bae13d246c23ead145a7e8f69ff2a1191`。
- Finding A 业务提交：`04e5611bd`；Finding B 业务提交：`12d4f5fd5`。
- 协调器最终冻结 HEAD：`bac981ab75add0b9e6d1a25ed690361b1fafe19a`。

## Finding A：权威 micros 排序

- 审阅 `model/admin_analytics_paid_subscription.go`：`recognized_remaining_value` 排序先将目标币种 `amount_micros` 严格解析为 `int64`，解析失败传播既有稳定错误，不回退兼容 `float64 amount`。
- users/subscriptions/plans/sources 均使用预解析 micros；相同 micros 保留各自既有业务主键 tie-breaker。
- 协调器复验：
  - `go test ./model -run "TestPaidSubscriptionValue(RecognizedRemainingSortUsesAuthoritativeMicros|UsersDescSortUsesUserIDTieBreaker)" -count=25`：PASS。

## Finding B：current_only 五面板警告

- 审阅共享构建路径：检测到任一 Credit 明细为 `current_only` 时，只生成一条 `section=credit_valuation, reason=current_only` 结构化 warning；summary/users/subscriptions/plans/sources 均复用 `data.Warnings`。
- subscription 明细继续保留 `snapshot_semantics`、`valuation_state_version` 和 `valuation_updated_at`。
- 首次协调器高次数复验暴露测试隔离缺口：同进程重复创建 SQLite 数据库时，全局 DB 时间缓存未重置，导致后续权益 `StartTime` 偶发晚于旧缓存时间并返回 `no active subscription`。这不是 warning 业务实现失败，但会使回归测试不确定。
- 已在 `setupCreditValuationTracerTestDB` 建库前及 cleanup 时调用 `resetDBTimestampCacheForTest()`，提交 `bac981ab7`。
- 协调器复验：
  - `go test ./model -run "^TestCreditValuationFiveAnalyticsPanelsReturnCurrentOnlyWarning$" -count=100`：PASS。
  - `go test ./model -run "TestCreditValuationFiveAnalytics(PanelsReturnCurrentOnlyWarning|ViewsAgreeOnThirtyTwoCNY)" -count=25`：PASS。

## 组合窄门禁

- `go test ./model ./service ./controller -count=1`：PASS。
- `bun test src/features/admin-analytics/lib/format.test.ts src/features/admin-analytics/panel-fields.test.ts src/features/admin-analytics/paid-value-panel.test.tsx`：17/17 PASS。
- `bun run typecheck`：PASS。
- `git diff --check`：PASS。
- 最终 `git status --short`：空。

## 边界

- 本轮未修改前端生产代码、Gate C、异步/coalescer、退款/恢复、FX 或 migration marker/ready。
- MySQL 5.7 与 PostgreSQL 9.6 未实测；真实三数据库零 SKIP 验收仍归 Issue #27。
