# Issue #21 邀请身份重放修复证据

状态：INVESTIGATING

## 基线

- `git rev-parse HEAD`：`86b49a724e32b1dfea3b43a25f73e03efb8584b7`。
- `git status --short`：无输出，工作树 clean。
- 稳定 RED 来自 `7b9e0038e`；`86b49a724` 仅补充 clean handoff 证据。

## 已读取材料

- GitHub 父 PRD `jiwangyihao/new-api#19`、Issue `#21`、已关闭 `#22`。
- `credit-operational-value-execution.md`。
- `credit-operational-value-issue-21-fixture-migration-contract.md`。
- `.scratch/agent-progress/issue-21/wide-model-{status,evidence,contract}.md`。
- `credit-operational-value-issue-21-acceptance.md`。
- ADR 0002 与 2026-08-02 spec 中订单履约、计时 grant、邀请隔离及幂等合同。
- `skill://diagnosing-bugs`、`skill://tdd`、`skill://codebase-design`、Orca CLI 与 orchestration 指南。

## 待执行 RED

```text
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestCompleteSubscriptionOrderReturnsResultForSuccessRetry)$' -count=1
```

预期捕获的精确缺陷：首次完成与持久化 `InvitationRewardEvent.InviterId` 非零，order fulfillment、timed grant subscription identity、event source subscription identity 一致；成功重放却返回 `InviterId=0`，且重放不应改变 event/subscription/grant/order 数量或 `FulfilledSubscriptionID`。

## 尚未运行

- 上述 RED 单次复现。
- 两项成功重放 `-count=10`。
- 九项邀请迁移组合。
- `go test ./model -count=1`。
- 窄范围 `go test -race`。
- 修改文件 `gofmt`、`git diff --check`、最终 clean 检查。
