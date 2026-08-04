# Issue #21 夹具迁移 C 证据

## 冻结基线

- `git rev-parse HEAD` → `774b35740c1879b285537031410731317d0142fc`。
- `git branch --show-current` → `jiwangyihao/issue-21-fixture-c-controller`。
- `git status --short` → 无输出，初始工作树 clean。

## 已读取材料

- 父 PRD #19、Issues #21/#22。
- `docs/agents/credit-operational-value-execution.md`。
- `docs/agents/credit-operational-value-wave-1-contract.md`。
- `docs/agents/credit-operational-value-issue-21.md` 与 acceptance。
- 共享夹具迁移合同。
- `CONTEXT.md`、ADR 0002、2026-08-02 spec。
- `.scratch/agent-progress/issue-21/spec-fix-*` 与 `final-spec-fix-*`。
- Skills：`diagnosing-bugs`、`tdd`、`codebase-design`、Orca orchestration/CLI。

## controller 包级冻结 RED

- 命令：`go test ./controller -count=1`。
- 结果：FAIL，`25.967s`；共 27 个失败测试，日志中出现 29 次 `timed_subscription_grant_invalid`，无 `panic:`。
- 余额/真实购买（9）：`TestSubscriptionBalancePayCreatesSubscriptionAndDeductsBalance`、`TestSubscriptionBalancePurchaseStoresCNYAmountSnapshot`、`TestSubscriptionBalancePurchaseInvokesInvitationRewardHandlerAndCreatesEvent`、`TestSubscriptionBalancePurchaseReturnsSuccessWhenInvitationRewardHandlerFails`、`TestSubscriptionBalancePayAllowsDecimalPlanPrice`、`TestSubscriptionBalancePayIdempotent`、`TestSubscriptionBalancePayTimedModeIgnoresHistoricalPurchaseLimit`、`TestSubscriptionBalancePayExtendsActiveSubscriptionWithoutNewRecord`、`TestSubscriptionBalancePayLocksUserBeforePurchaseLimitCheck`。典型失败为 HTTP body `timed_subscription_grant_invalid`、余额未扣、订单/权益/grant 未创建。
- 共享余额入口（1）：`TestSubscriptionPurchaseDoesNotUpdateUserGroup`，同样因非法 timed grant 夹具返回失败。
- 邀请订单（2）：`TestCompleteSubscriptionOrderTriggersInvitationEntitlement`、`TestCompleteSubscriptionOrderRetriesInvitationRewardHandlerForSuccessfulOrder`；直接成功订单缺合法快照，导致履约失败、handler 未调用、订单仍 pending、奖励/权益未创建。
- Kyren（11）：`TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation`、`TestKyrenWebhookCompletesSubscriptionOrder`、`TestKyrenWebhookRecordsInvitationRewardEventForRewardIneligibleSnapshot`、`TestKyrenWebhookRetriesInvitationRewardHandlerForSuccessfulSubscriptionOrder`、`TestKyrenWebhookRecoversStaleClaimedSubscriptionOrder`、`TestKyrenWebhookCompletesSubscriptionOrderUsingEntitlementSnapshot`、`TestKyrenWebhookCapturesOrderIDFromSnakeCasePayload`、`TestKyrenSuccessfulSubscriptionReplayStillValidatesPaymentSnapshot`、`TestKyrenSubscriptionOrderStoresEmptySnapshotWhenCurrencyUnsupported`、`TestKyrenSubscriptionOrderStoresCNYAmountSnapshot`、`TestKyrenWebhookRefundedRecordsManualActionAndReturnsSuccess`。成功/重放/退款前置订单均缺完整合法授权事实，主要表现为 HTTP 500 或直接 `timed_subscription_grant_invalid`；Credit webhook 因未履约而无活动权益。
- Stripe（3）：`TestCompletedSubscriptionOrderReplayStillValidatesAmountAndCurrency`、`TestStripeSubscriptionWebhookPropagatesInvitationRewardHandlerFailureOverHTTP`、`TestStripeSubscriptionWebhookPropagatesInvitationRewardHandlerFailure`；成功订单/回调前置缺合法快照，重放失败或订单保持 pending。
- Epay（1）：`TestSubscriptionEpayTimedCallbackPreservesInvitationBehavior`；回调返回 `fail` 且邀请事件不存在。
- 缺表日志：`two_fas`、`models`、`vendors`、`channel_group_channels`、`logs`、`token_group_bindings` 来自其他 controller 测试的局部数据库 setup 且未对应本次 FAIL；`subscription_orders` 缺表来自 `TestKyrenWebhookReturnsRetryableFailureWhenOrderLookupFails` 的故障注入，属于其预期接缝。本路当前没有缺表导致的失败。
- Redis：仅观察到 `redis: client is closed` 的缓存失效日志，无 panic、无对应失败堆栈；后续迁移相关 setup 后再以完整包级回归确认本路未污染全局状态。

后续为每组记录最小 RED→GREEN 命令、关键 provider/重放 `-count=10`、聚焦正则回归和完整 controller 包级结果。
