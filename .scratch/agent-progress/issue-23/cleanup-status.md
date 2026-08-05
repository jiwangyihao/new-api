# Issue #23 请求记录清理状态

## 当前状态
- 阶段：`HANDOFF_READY`。
- Task lifecycle 安全提交：`e83551e89`。提交后同一窄命令复跑为 `go test: 1 packages ok`；提交内 evidence 保留首次组合运行的 schema 隔离失败现场。
- cleanup RED 提交：`c31a612ae`，仅包含本状态文件与 `model/subscription_preconsume_cleanup_test.go`。
- 当前不实现 cleanup GREEN；后续 owner 从终态资格 RED 继续。

## Cleanup RED 证据
- 命令：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=1`。
- 结果：期望只删除 2 条过期 `settled/refunded`，实际删除 4 条；`consumed` 与未知状态也被现有 `CleanupSubscriptionPreConsumeRecords` 按 `updated_at` 删除。
- RED 原因是现有清理入口缺少终态资格约束，而非符号未定义；测试通过公开请求预扣/结算入口构造事实。

## 后续范围
1. 先使终态资格 RED 转 GREEN，再逐项增加 cutoff 边界、Task/回调持久引用保护、稳定 batch、幂等、失败原子性、只读诊断、并发与审计保留。
2. 禁止继续 Task/legacy/quota/conversion 工作；禁止实现 #24–#28。
