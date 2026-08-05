# Issue #23 请求记录清理证据

## 2026-08-05 恢复安全点

### 基线核验
命令：
```text
git branch --show-current && git rev-parse HEAD && git status --short
```
关键输出：
```text
jiwangyihao/issue-23-request-settlement
d9e620191f8ca02c237859cc0250f98209749016
M service/billing_session.go
M service/task_billing_test.go
```
结论：分支和 HEAD 与协调器指定值严格一致；工作树并非干净，存在两处继承的 Task 兼容改动，因此在裁决前不进行清理生产代码写入。

### 收敛判断
- `service/task_billing_test.go` 的差异把 legacy 匿名 Credit Task 夹具迁移到持久主键身份，并补充成功、失败与重放断言。
- `service/billing_session.go` 的唯一差异把 Task relay 的 Credit `SettleWithInput` 从 `final=false` 改为 `final=true`。
- 该生产差异只允许由现有新持久 identity 成功终态定向测试证明；没有明确 RED 即撤销，不继续探索 Task 路径。

### 下一条 RED/GREEN
待运行现有 `TestCreditTaskSuccessFinalAndReplayReusePersistedRequestID`（及直接相关持久 identity 用例），分别在当前差异与撤销该一行差异的状态下验证是否只有 `final=true` 能得到 `settled` 终态。

## 终态清理资格安全点（bfa31bb09）
- RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=1` 失败；期望删除 2 条，旧实现实际删除 4 条，错误删除了过期 `consumed` 与未知状态记录。
- GREEN：`CleanupSubscriptionPreConsumeRecords` 增加明确终态资格，仅允许删除 cutoff 前的 `settled`/`refunded`。
- 结果：同一场景现仅删除 2 条过期终态记录，并保留 `consumed` 与未知状态记录。
- 稳定验证：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=10` 返回 `go test: 1 packages ok`；`git diff --check` 无输出。
- 本安全点未实现 Task/回调引用保护、稳定 batch、并发、失败原子性或只读诊断。

## cutoff 精确边界安全点
- RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsUsesExclusiveFinalizedAtCutoff$' -count=1` 失败；期望删除 1 条，旧实现实际删除 3 条。
- 根因：旧实现使用可被后续写入改变的 `updated_at`，没有以终态落定时间 `finalized_at` 判断保留期。
- GREEN：使用排他条件 `finalized_at < cutoff`；cutoff 前删除，等于 cutoff 与 cutoff 后保留。
- 验证：单用例 `count=1` 与 `go test ./model -run '^TestCleanupSubscriptionPreConsumeRecords' -count=10` 均返回 `go test: 1 packages ok`；`git diff --check` 无输出。
