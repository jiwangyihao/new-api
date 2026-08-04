# Issue #21 邀请身份重放修复证据

状态：HANDOFF_READY

## 基线与提交

- 起始 HEAD：`86b49a724e32b1dfea3b43a25f73e03efb8584b7`。
- 稳定 RED：`7b9e0038e`。
- 进度基线：`d5ad009c4`（`docs(issue-21): 建立邀请身份重放修复现场`）。
- 生产修复：`1786fd9b015dac7213efbb999cd7035c29398cc4`（`fix(subscription): 恢复成功订单重放邀请身份`）。

## RED

执行：

```text
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestCompleteSubscriptionOrderReturnsResultForSuccessRetry)$' -count=1 -v
```

实际结果：`FAIL`，命令退出码 1。精确失败：

- `TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition`：`invitation_commission_test.go:319`，expected `9201`，actual `0`。
- `TestCompleteSubscriptionOrderReturnsResultForSuccessRetry`：`invitation_commission_test.go:435`，expected `9231`，actual `0`。
- 既有测试在失败前已证明首次结果与持久化 event 的 inviter 非零，且 order `FulfilledSubscriptionID`、timed grant `UserSubscriptionId`、event `SourceSubscriptionId` 一致。

## GREEN

### 两项成功重放单次

```text
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestCompleteSubscriptionOrderReturnsResultForSuccessRetry)$' -count=1 -v
```

结果：PASS，`go test: 1 packages ok`，12.49s。

### 两项成功重放十次

```text
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestCompleteSubscriptionOrderReturnsResultForSuccessRetry)$' -count=10
```

结果：PASS，`go test: 1 packages ok`，3.87s。

### 无 invitation event

```text
go test ./model -run '^TestTimedSubscriptionValuationGrantPaidOrderReplayRestoresImmutableResult$' -count=1 -v
```

结果：PASS，`go test: 1 packages ok`，2.92s。该既有用例首次完成后断言 event count 为 0，重放恢复同一 subscription/source/window，并断言 subscription 与 timed grant 数量不变；修复路径未把 event absence 当错误，`InviterId` 保持 Go 零值 `0`。

### 九项 invitation fixture 组合

```text
go test ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestCompleteSubscriptionOrderTxEventIntervalUsesOnlyRenewalDelta|TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent|TestRedeemSubscriptionRedemptionRecordsEventForRewardIneligiblePlan|TestCompleteSubscriptionOrderReturnsResultForSuccessRetry|TestCompleteSubscriptionOrderRecordsEventForRewardIneligiblePlan|TestCompleteSubscriptionOrderConcurrentClaimCreatesSingleSubscriptionAndEvent|TestRedeemSubscriptionRedemptionConcurrentClaimCreatesSingleSubscriptionAndEvent|TestCompleteSubscriptionOrderAllowsRenewalWhenHistoricalPurchaseLimitReached)$' -count=1
```

结果：PASS，`go test: 1 packages ok`，8.84s。

### model 包完整回归

```text
go test ./model -count=1
```

结果：PASS，`go test: 1 packages ok`，33.62s。

### 窄范围 race

```text
go test -race ./model -run '^(TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition|TestCompleteSubscriptionOrderReturnsResultForSuccessRetry|TestTimedSubscriptionValuationGrantPaidOrderReplayRestoresImmutableResult)$' -count=1
```

结果：PASS，`go test: 1 packages ok`，27.00s。

## 副作用与来源证明

- 两个邀请重放用例均保留唯一 event count 为 1，首次与重放的 `SourceSubscriptionId`、event window 完全相同。
- 无 event 用例保留 event count 0，并证明 subscription/grant count 不变。
- 生产实现只执行限定 `source_type=subscription_order AND source_id=order.Id`、最多两行的只读 event 查询；不执行任何 event/subscription/grant/order 写入。
- event 必须匹配 `SourceOrderId=order.Id`、`SourceSubscriptionId=FulfilledSubscriptionID`、`InviteeId=order.UserId` 且 `InviterId>0`；否则 fail closed。
- 未读取 current Plan。

## 清理

- `gofmt -w model/subscription.go`：完成。
- 修复提交前 `git diff --check`：PASS。
- 修复提交后 HEAD：`1786fd9b015dac7213efbb999cd7035c29398cc4`；最终进度提交后再次验证 clean。

## 未运行

- MySQL 5.7/PostgreSQL 9.6 实机未运行，属于 Issue #27。
- service/controller/frontend/i18n 未运行，未修改且超出本生产修复范围。
