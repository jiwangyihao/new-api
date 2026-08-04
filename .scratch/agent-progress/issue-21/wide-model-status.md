# Issue #21 宽回归修复 D 状态

状态：IN_PROGRESS

## 冻结现场

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-wide-model-fix`
- 分支：`jiwangyihao/issue-21-wide-model-fix`
- 当前起始 HEAD：`de6c6bbe912294e802b25a5e9bbcc37e8d9194d7`
- 共同父 HEAD：`3e74a2928f7e4b7c3d5c6eae3fbc8362172a4c5d`
- 祖先关系：已确认共同父 HEAD 是当前起始 HEAD 的祖先；其后仅有协调合同提交 `de6c6bbe9`。
- 起始工作树：clean。
- 最近安全提交：`de6c6bbe9`（Issue #21 宽回归修复指令）。

## 所有权

仅修改：

- `model/invitation_commission_test.go`
- `model/payment_method_guard_test.go`
- 上述测试实际复用或新增的 test-only helper
- `.scratch/agent-progress/issue-21/wide-model-{status,evidence,contract}.md`

不修改生产 fail-closed、`service`、`controller`、前端、i18n、schema、#22 深模块或 #23–#28。

## 冻结失败

- `TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition`
- `TestCompleteSubscriptionOrderTxEventIntervalUsesOnlyRenewalDelta`
- `TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent`
- `TestRedeemSubscriptionRedemptionRecordsEventForRewardIneligiblePlan`
- `TestCompleteSubscriptionOrderReturnsResultForSuccessRetry`
- `TestCompleteSubscriptionOrderRecordsEventForRewardIneligiblePlan`
- `TestCompleteSubscriptionOrderConcurrentClaimCreatesSingleSubscriptionAndEvent`
- `TestRedeemSubscriptionRedemptionConcurrentClaimCreatesSingleSubscriptionAndEvent`
- `TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached`

## 当前阶段

- 状态：HANDOFF_READY（夹具迁移完成，独立生产 blocker 已隔离）。
- 已完成：合法 paid timed Plan helper、订单不可变 `EntitlementSnapshot` helper、`Redemption.Insert()`/`FulfillmentSnapshot` helper；九项均已迁移，七项独立组合 GREEN。
- 生产 blocker：两项 order success replay 在 event、grant、`FulfilledSubscriptionID` 均存在且一致时仍返回 `InviterId=0`；断言保留，生产代码未修改。
- 精确接缝：`subscriptionOrderCompletionResultFromExistingFulfillmentTx` → `subscriptionOrderCompletionResultFromTimedGrantTx`（`model/subscription.go:1137-1196`）遗漏已持久化 invitation identity。
- 下一步：专门生产修复 Agent 合并 event identity 后重跑九项、十次重放与完整 `model` 包；本 Worker 仅提交 clean 可恢复现场。
- 阻塞：独立生产缺陷，已通过 Orca escalation 上报。

## 恢复命令

```text
git status --short --branch
git log -1 --oneline
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent|TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached)$' -count=1
```
