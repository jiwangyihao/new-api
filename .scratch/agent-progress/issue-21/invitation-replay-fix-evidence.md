# Issue #21 邀请身份重放修复证据

状态：RED_CONFIRMED

## 基线

- `git rev-parse HEAD`：`86b49a724e32b1dfea3b43a25f73e03efb8584b7`。
- `git status --short`：起始无输出，工作树 clean。
- 稳定 RED 来自 `7b9e0038e`；`86b49a724` 仅补充 clean handoff 证据。
- 进度基线提交：`d5ad009c4`（`docs(issue-21): 建立邀请身份重放修复现场`）。

## 已读取材料

- GitHub 父 PRD `jiwangyihao/new-api#19`、Issue `#21`、已关闭 `#22`。
- `credit-operational-value-execution.md`。
- `credit-operational-value-issue-21-fixture-migration-contract.md`。
- `.scratch/agent-progress/issue-21/wide-model-{status,evidence,contract}.md`。
- `credit-operational-value-issue-21-acceptance.md`。
- ADR 0002 与 2026-08-02 spec 中订单履约、计时 grant、邀请隔离及幂等合同。
- `skill://diagnosing-bugs`、`skill://tdd`、`skill://codebase-design`、Orca CLI 与 orchestration 指南。

## 稳定 RED

执行：

```text
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestCompleteSubscriptionOrderReturnsResultForSuccessRetry)$' -count=1 -v
```

实际结果：`FAIL`，`github.com/QuantumNous/new-api/model`，命令退出码 1。两项均只在成功重放的邀请身份断言失败：

- `TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition`：`invitation_commission_test.go:319`，首次结果与持久化 event 的 `InviterId=9201`，重放实际为 `0`。
- `TestCompleteSubscriptionOrderReturnsResultForSuccessRetry`：`invitation_commission_test.go:435`，首次结果 `InviterId=9231`，重放实际为 `0`。
- 同次输出中的 `top_ups record not found` 是既有查询日志，不是断言失败原因。
- 既有测试在失败断言前已经证明 order `FulfilledSubscriptionID > 0`、timed grant `UserSubscriptionId`、event `SourceSubscriptionId` 三者一致；失败断言后已有唯一 event 计数保护，修复后继续执行。

## 尚未运行

- 两项成功重放修复后单次与 `-count=10`。
- 无 invitation event 的 paid timed 重放（`InviterId=0`、计数不变）。
- 九项邀请迁移组合。
- `go test ./model -count=1`。
- 窄范围 `go test -race`。
- 修改文件 `gofmt`、`git diff --check`、最终 clean 检查。
