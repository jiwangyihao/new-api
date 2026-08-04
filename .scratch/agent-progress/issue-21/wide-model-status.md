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

- 已完成：读取父 PRD #19、Issue #21、已关闭 #22、执行合同、宽回归合同、Issue #21 spec/acceptance、ADR 0002、`CONTEXT.md`、最终 Spec 修复恢复文件及必需 skills。
- 进行中：保存宽回归 model 路基线安全点。
- 下一步：运行冻结最小复现，记录本工作树实际稳定错误，再做最小 test-only GREEN。
- 阻塞：无。

## 恢复命令

```text
git status --short --branch
git log -1 --oneline
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent|TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached)$' -count=1
```
