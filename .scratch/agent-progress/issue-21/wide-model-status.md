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

- 已完成：必读材料核查、基线恢复合同安全提交 `d85ccc1cd`、冻结三项最小复现。
- 本地 RED：两项订单稳定返回 `timed_subscription_grant_invalid`，兑换稳定返回 `redemption.plan_ineligible`；另有初始化前清表的缺表日志噪声。
- 进行中：建立合法 Plan、订单授权快照与 Redemption.Insert test-only helper，并迁移九项测试。
- 下一步：先让三项最小复现 GREEN，再覆盖其余六项及并发不变量。
- 阻塞：无；当前证据与冻结根因一致，尚未发现生产缺陷。

## 恢复命令

```text
git status --short --branch
git log -1 --oneline
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent|TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached)$' -count=1
```
