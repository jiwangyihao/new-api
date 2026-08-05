# Issue #23 请求记录清理状态

## 当前状态
- 阶段：`GREEN_COMMITTED`。
- 恢复 HEAD：`952322017b37c2511ce12a84769a401e0e68b0ab`；进入本阶段前 `git status --short` 为空。
- cleanup RED 已提交于 `c31a612ae`，目标用例通过公开请求预扣/结算入口构造事实。
- 本安全点只收敛清理资格：`CleanupSubscriptionPreConsumeRecords` 仅删除 cutoff 前的 `settled`/`refunded`，保留 `consumed`、未知状态与其他非终态。
- RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=1` 失败，`expected: 2`、`actual: 4`。
- GREEN：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=1` 通过，`go test: 1 packages ok`。
- 稳定验证：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=10` 通过，`go test: 1 packages ok`；`git diff --check` 无输出。
- 生产改动安全提交：`bfa31bb09`（`fix(credit): 限制预扣记录清理终态资格`）；旧实现删除 4 条，现仅删除 2 条过期 `settled/refunded`，并保留 `consumed/unknown`。

## Cleanup RED 证据
- 命令：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=1`。
- 结果：期望只删除 2 条过期 `settled/refunded`，实际删除 4 条；`consumed` 与未知状态也被现有 `CleanupSubscriptionPreConsumeRecords` 按 `updated_at` 删除。
- RED 原因是现有清理入口缺少终态资格约束，而非符号未定义；测试通过公开请求预扣/结算入口构造事实。

## 后续范围
1. 先使终态资格 RED 转 GREEN，再逐项增加 cutoff 边界、Task/回调持久引用保护、稳定 batch、幂等、失败原子性、只读诊断、并发与审计保留。
2. 禁止继续 Task/legacy/quota/conversion 工作；禁止实现 #24–#28。
