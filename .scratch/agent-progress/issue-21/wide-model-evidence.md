# Issue #21 宽回归修复 D 证据

状态：IN_PROGRESS

## 基线证据

- `git status --short --branch`：分支 `jiwangyihao/issue-21-wide-model-fix`，staged 0、unstaged 0、untracked 0。
- `git rev-parse HEAD`：`de6c6bbe912294e802b25a5e9bbcc37e8d9194d7`。
- `git merge-base --is-ancestor 3e74a2928f7e4b7c3d5c6eae3fbc8362172a4c5d HEAD`：退出码 0。
- `git log --oneline 3e74a2928f7e4b7c3d5c6eae3fbc8362172a4c5d..HEAD`：仅 `de6c6bbe9 docs(agents): 固化 Issue 21 宽回归修复指令`。

## 材料证据

已读取：

- GitHub `jiwangyihao/new-api` #19、#21、#22（通过 `gh issue view --repo jiwangyihao/new-api`，避免裸 `issue://` 命中上游同号 Issue）。
- `CONTEXT.md`
- `docs/agents/credit-operational-value-execution.md`
- `docs/agents/credit-operational-value-issue-21.md`
- `docs/agents/credit-operational-value-issue-21-acceptance.md`
- `docs/agents/credit-operational-value-issue-21-wide-regression-contract.md`
- `docs/agents/credit-operational-value-issue-21-wide-model-fix.md`
- `docs/adr/0002-credit-operational-remaining-value.md`
- `.scratch/agent-progress/issue-21/final-spec-fix-{status,evidence,contract}.md`
- `skill://diagnosing-bugs`、`skill://tdd`、`skill://codebase-design`、`skill://orca-cli` 及版本匹配 Orca CLI 指南。

## RED 台账

协调器冻结的失败列表见 `wide-model-status.md`；它尚不是本 Worker 的本地复现证据。

待运行本地最小复现：

```text
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent|TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached)$' -count=1
```

运行后必须在此记录精确错误与根因区分；不得用协调器输出替代。

## GREEN 与验收台账

尚未运行；完成修复后逐项记录定向测试、十次并发/重放测试、`go test ./model -count=1`、`git diff --check` 与 clean-tree 结果。
