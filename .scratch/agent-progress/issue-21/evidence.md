# Issue #21 验证证据

## 基线

- `git rev-parse HEAD`：`53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- 当前分支：`jiwangyihao/issue-21-timed-grants`。
- 父集成工作树 `credit-operational-value-integration`：同一 HEAD。
- 初始状态：staged 0、unstaged 0、untracked 0。

## 规范证据

已读取并采用：

- `issue://jiwangyihao/new-api/19`
- `issue://jiwangyihao/new-api/21`
- `docs/agents/credit-operational-value-execution.md`
- `docs/agents/credit-operational-value-wave-1-contract.md`
- `docs/agents/credit-operational-value-issue-21.md`
- `.scratch/agent-progress/issue-20/contract.md`
- `CONTEXT.md`
- `docs/adr/0001-credit-balance-entitlement.md`
- `docs/adr/0002-credit-operational-remaining-value.md`
- 2026-08-02 spec 第 5.3、6、8、9、10、12、13、14 节
- 2026-08-02 plan 任务 5、任务 8 timed 部分、任务 9 timed/UI 部分
- `skill://tdd`
- `skill://codebase-design`

## RED / GREEN

尚未开始。首个 tracer 将从真实 SQLite 中经公开领域入口授予计时权益，断言同事务产生不可变 grant，并证明相同来源重放不续期。

## 数据库 / API / 浏览器

尚未运行；后续持续记录精确命令、关键 payload、响应与可观察结果。

## 恢复文件提交核验

- `git show --stat --oneline 99a7ce6f5`：提交包含 `contract.md` 93 行、`evidence.md` 34 行、`status.md` 26 行，共 153 行。
- `contract.md` 已覆盖 schema、来源身份、领域接口、稳定错误、分析 DTO/算法、UI payload、共享文件与明确非所有权。
- `status.md` 已覆盖阶段、完成项、下一步、阻塞与最近安全提交；`evidence.md` 已建立 RED/GREEN、数据库、API、浏览器证据分区。
- 核验时工作树 clean；上述文件不是两行占位文件。

## 领域 tracer 收敛

- 协调器要求停止扩大接缝探索，先落公开 timed 授予入口的真实 SQLite RED。
- 当前确认的最小既有 seam：`CreateUserSubscriptionFromPlanWithResultTx` 返回真实 `EventStartTime/EventEndTime`；`TimedSubscriptionValuationGrant` 已由 Issue #20 注册并具备唯一索引与 update/delete hook；尚无 `TimedSubscriptionGrantRequest` 或 `GrantTimedSubscriptionTx`。
- 首个 RED 只防守可观察合同：一次有价订单来源在同一事务创建/续期权益并写 grant；相同确定性来源重放返回既有结果，grant 数量与权益 `end_time` 均不再变化。
- 最近安全提交：`f60433f52 docs(issue-21): 记录恢复合同核验`。

## RED 1：公开 timed 授予入口

- 测试：`TestTimedSubscriptionValuationGrantCreatesTimelineAndReplaysSource`。
- 真实数据库：独立 SQLite shared-memory 数据库，实际迁移 `User`、`SubscriptionPlan`、`UserSubscription`、`TimedSubscriptionValuationGrant`。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrantCreatesTimelineAndReplaysSource$' -count=1`。
- 结果：预期 RED；编译器报告 `undefined: TimedSubscriptionGrantRequest` 与 `undefined: GrantTimedSubscriptionTx`，证明公开领域入口尚不存在，而非夹具或断言误失败。
- 防守合同：首次来源同事务创建权益与冻结 grant；相同 `subscription_order:21003` 重放返回原窗口且不延长 `end_time`、不新增第二条 grant。

## GREEN 1：公开 timed 授予入口

- 实现：`TimedSubscriptionGrantRequest` 显式接收调用方冻结的 `SourcePriceMicros` 与 `SourceCurrency`；Plan 只提供权益期限、重置和 Credit 事实。
- 实现：`GrantTimedSubscriptionTx` 在同一事务先按确定性 `idempotency_key` / `(source_type, source_key)` 查重，再调用现有低层创建 seam，并使用其实际窗口写不可变 grant。
- 重放：完全相同来源返回原 grant 对应窗口与当前权益，不再次调用续期。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrantCreatesTimelineAndReplaysSource$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 11.57 秒。
- 权威来源检查：grant、source snapshot 与冲突指纹均使用请求中的 micros/原币种，不从当前 Plan 标价推导。
