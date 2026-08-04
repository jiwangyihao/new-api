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

本地最小复现已运行：

```text
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent|TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached)$' -count=1
```

结果：稳定 FAIL（`github.com/QuantumNous/new-api/model`，7.333s）。精确业务错误：

- `TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition`：`timed_subscription_grant_invalid`；pending order 没有 `EntitlementSnapshot`，且 Plan 不满足完整权威 timed 事实。
- `TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent`：`redemption.plan_ineligible`；测试以 `DB.Create` 绕过 `Redemption.Insert()`，没有冻结 `FulfillmentSnapshot`。
- `TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached`：`timed_subscription_grant_invalid`；guard helper 创建的 Plan 缺 explicit timed、权威 micros/reset，order 缺不可变授权快照。

同次运行还观察到清理噪声：`invitation_commission_withdrawals`、`invitation_commission_ledgers`、`invitation_commission_records`、`invitation_commission_accounts`、`invitation_reward_events`、`redemptions` 与 `subscription_pre_consume_records` 在相应测试初始化前被无条件删除而不存在。该噪声不是三项业务断言失败原因；所属文件内将只补齐实际使用表的安全清理。

## GREEN 与验收台账

- 九项定向命令 `-count=1`：FAIL（12.388s），仅两项成功重放身份断言 RED：
  - `TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition`：首次完成已成功，持久化 `InvitationRewardEvent.InviterId=9201`；order `FulfilledSubscriptionID > 0`，唯一 order timed grant 的 `UserSubscriptionId` 与 event `SourceSubscriptionId` 都等于该 fulfillment；成功重放仍返回 `InviterId=0`。
  - `TestCompleteSubscriptionOrderReturnsResultForSuccessRetry`：首次结果 `InviterId=9231`，成功重放返回 `InviterId=0`。
- 精确生产接缝：`model/subscription.go:1137-1161` 的 `subscriptionOrderCompletionResultFromExistingFulfillmentTx` 对普通 paid timed 直接转到 `subscriptionOrderCompletionResultFromTimedGrantTx`；`model/subscription.go:1164-1196` 只从 grant/subscription 恢复窗口和 subscription identity，未合入已持久化的 `InvitationRewardEvent.InviterId`。禁止在本夹具切片修改生产代码或弱化身份断言。
- 其余七项独立组合（续期 delta、两项兑换、reward-ineligible order、两项并发、历史购买限制续期）`-count=1`：PASS；证明合法 timed Plan、订单 `EntitlementSnapshot`、`Redemption.Insert()`/`FulfillmentSnapshot` 与并发恰好一次夹具已经 GREEN。
- 原三项中 `TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent` 与 `TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached` 已 GREEN；`TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition` 仅剩上述独立生产重放身份 blocker。
- `go test ./model -count=1` 和十次重放门禁被已确认的生产 RED 阻断，未伪报 PASS；现场以 HANDOFF_READY 交由专门生产修复 Agent。
- 两个测试文件已 `gofmt`；`git diff --check` PASS；夹具与 HANDOFF_READY 证据提交 `7b9e0038e` 后 `git status --short && git diff --check HEAD^ HEAD` 无输出，工作树 clean。
