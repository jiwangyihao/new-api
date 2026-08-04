# Issue #21 宽回归修复 D：model 授权快照夹具

## 目标

你只负责修复当前 #21 父分支中 `model` 包剩余的订单/兑换授权测试夹具，使其遵守已经冻结的不可变来源快照合同；不得放宽生产 fail-closed。

工作树将由协调器从父 HEAD `3e74a2928f7e4b7c3d5c6eae3fbc8362172a4c5d` 显式创建。必读：

- 父 PRD #19、Issue #21、已关闭 #22；
- `docs/agents/credit-operational-value-execution.md`；
- `docs/agents/credit-operational-value-issue-21-wide-regression-contract.md`；
- Issue #21 spec/acceptance、ADR 0002；
- `.scratch/agent-progress/issue-21/final-spec-fix-*`；
- skills：`diagnosing-bugs`、`tdd`、`codebase-design`。

## 冻结 RED

协调器在三路合入后的父分支运行 `go test ./model ./service ./controller -count=1`，`service` 已 PASS，`model` 仍失败：

- `model/invitation_commission_test.go`
  - `TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition`
  - `TestCompleteSubscriptionOrderTxEventIntervalUsesOnlyRenewalDelta`
  - `TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent`
  - `TestRedeemSubscriptionRedemptionRecordsEventForRewardIneligiblePlan`
  - `TestCompleteSubscriptionOrderReturnsResultForSuccessRetry`
  - `TestCompleteSubscriptionOrderRecordsEventForRewardIneligiblePlan`
  - `TestCompleteSubscriptionOrderConcurrentClaimCreatesSingleSubscriptionAndEvent`
  - `TestRedeemSubscriptionRedemptionConcurrentClaimCreatesSingleSubscriptionAndEvent`
- `model/payment_method_guard_test.go`
  - `TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached`

最小复现也稳定失败：

```text
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent|TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached)$' -count=1
```

根因候选已经定位：测试直接 `DB.Create` pending `SubscriptionOrder` 或 `Redemption`，没有通过前向授权入口冻结 `EntitlementSnapshot` / `FulfillmentSnapshot`；Plan helper 也缺少 explicit timed entitlement、权威 `price_amount_micros`、合法 duration/reset 等事实。

## Change

1. 先在 `.scratch/agent-progress/issue-21/wide-model-{status,evidence,contract}.md` 固化基线、失败列表、所有权和恢复命令，提交安全点。
2. 运行上述最小复现，记录精确稳定错误。不得只引用协调器输出。
3. 添加或复用 test-only helper：
   - 创建 enabled、非 trial/invite-trial、explicit timed、正 Credit、权威 micros/currency、合法 duration/reset/rule 的 Plan；
   - 创建 pending provider order 时，使用 `NewSubscriptionEntitlementSnapshot`/`SetPaymentSnapshot`/`MarshalSubscriptionEntitlementSnapshot` 冻结与 Plan、provider、amount/currency 一致的授权快照；
   - 创建 subscription redemption 时，使用 `Redemption.Insert()` 冻结 `FulfillmentSnapshot`，不得手写 JSON 或在兑换时读取 current Plan 补造。
4. 迁移上述九个测试。保留原邀请事件、佣金、重放、续期区间、并发单权益/单事件、历史购买限制等所有业务断言。
5. 并发测试不得降 worker 数或放宽为“至少一个结果”；仍须证明恰好一次 transition/一份 subscription/一份 event。
6. 清理函数可补齐本文件实际使用的表，避免缺表日志；不得改生产代码来迎合测试。
7. 若合法快照夹具下仍有生产行为失败，停止扩大范围，通过 Orca question/escalation 报告最小复现、生产函数和事务状态。

## Acceptance

至少运行并记录：

```text
go test ./model -run 'TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestCompleteSubscriptionOrderTxEventIntervalUsesOnlyRenewalDelta|TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent|TestRedeemSubscriptionRedemptionRecordsEventForRewardIneligiblePlan|TestCompleteSubscriptionOrderReturnsResultForSuccessRetry|TestCompleteSubscriptionOrderRecordsEventForRewardIneligiblePlan|TestCompleteSubscriptionOrderConcurrentClaimCreatesSingleSubscriptionAndEvent|TestRedeemSubscriptionRedemptionConcurrentClaimCreatesSingleSubscriptionAndEvent|TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached' -count=1
go test ./model -run 'TestCompleteSubscriptionOrderConcurrentClaimCreatesSingleSubscriptionAndEvent|TestRedeemSubscriptionRedemptionConcurrentClaimCreatesSingleSubscriptionAndEvent|TestCompleteSubscriptionOrderReturnsResultForSuccessRetry' -count=10
go test ./model -count=1
git diff --check
git status --short
```

必须 clean tree、小步 Conventional Commits、有效 worker_done。禁止修改 service/controller、前端、locale、生产 fail-closed、#22 深模块或下游切片。
