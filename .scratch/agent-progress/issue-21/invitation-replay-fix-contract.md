# Issue #21 邀请身份重放修复合同

状态：HANDOFF_READY

## 缺陷合同

普通 paid timed 订单首次履约成功时，`InvitationRewardEvent` 已持久化邀请来源身份。后续成功重放必须恢复首次已提交的可观察结果；此前 `subscriptionOrderCompletionResultFromExistingFulfillmentTx` → `subscriptionOrderCompletionResultFromTimedGrantTx` 只恢复 subscription/window，丢失 `InviterId`。

## 领域接缝

“恢复既有成功结果”属于 existing fulfillment result 恢复模块的实现责任。接口保持不变：在 `subscriptionOrderCompletionResultFromTimedGrantTx` 已验证 immutable timed grant、order fulfillment 与 subscription identity 后，以订单唯一来源 `(source_type=subscription_order, source_id=order.Id)` 读取既有 `InvitationRewardEvent`，只合并持久化 `InviterId`。

## 最终不变量

1. 邀请身份只来自已持久化 `InvitationRewardEvent`；不从当前用户邀请关系、当前 Plan、当前邀请设置或请求参数重新推导。
2. 查询只使用订单稳定来源身份；最多加载两行以发现违反唯一来源合同的异常数据。
3. 唯一 event 必须匹配 `SourceOrderId`、`FulfilledSubscriptionID`、`InviteeId`，且 `InviterId > 0`。
4. event 匹配时只设置返回值 `InviterId`；subscription/window 仍只来自已验证的 immutable timed grant 与 fulfillment subscription。
5. event 不存在时返回成功且 `InviterId=0`。
6. 多行 event 或 identity 不匹配时不猜测最近一条，返回既有稳定 `ErrTimedSubscriptionGrantInvalid`。
7. 重放只读；不创建或更新任何实体，不读取 current Plan，不重算履约参数。
8. 首次履约、event 创建、佣金计算及 #21/#22 既有合同不变。

## 测试合同

- 两个稳定 RED 修复前分别观察 `9201→0` 与 `9231→0`，修复后首次/重放 inviter identity 一致。
- 原测试继续验证 immutable fulfillment identity 与 event count 保持 1。
- 既有无 event paid timed 重放覆盖 event count 0、`InviterId=0`、subscription/grant count 与 source/window 不变。
- 九项 invitation fixture、完整 model 包与窄范围 race 均通过。
