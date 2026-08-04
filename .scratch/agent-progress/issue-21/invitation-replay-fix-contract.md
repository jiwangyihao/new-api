# Issue #21 邀请身份重放修复合同

状态：INVESTIGATING

## 缺陷合同

普通 paid timed 订单首次履约成功时，`InvitationRewardEvent` 已持久化邀请来源身份。后续成功重放必须恢复首次已提交的可观察结果；当前 `subscriptionOrderCompletionResultFromExistingFulfillmentTx` → `subscriptionOrderCompletionResultFromTimedGrantTx` 只恢复 subscription/window，丢失 `InviterId`。

## 领域接缝

“恢复既有成功结果”属于现有 fulfillment-result 恢复模块的实现责任。保持其调用接口不变，在同一数据库事务、已锁定订单和 immutable fulfillment identity 下读取既有 `InvitationRewardEvent`，只把已持久化的 `InviterId` 合入返回结果；不新增跨层接口或第二套邀请计算。

## 不变量

1. 邀请身份只能来自已经持久化的 `InvitationRewardEvent`，不得从当前用户关系、当前 Plan、当前邀请设置或请求参数重新推导。
2. event 的 `SourceSubscriptionId` 必须对应本订单的 immutable `FulfilledSubscriptionID`；匹配时恢复原 `InviterId`。
3. event 不存在时返回 `InviterId=0` 且不报错。
4. 多个不合法或不匹配 event 时不得猜测最近一条；遵守现有唯一来源身份/稳定查询合同并 fail closed 或返回既有稳定错误。
5. 重放只读：不得新建或更新 event、subscription、timed grant、ledger 或订单授权快照。
6. 重放前后 event/subscription/timed grant/order 数量与 `FulfilledSubscriptionID` 不变。
7. 首次履约、event 创建与佣金计算语义不变；不读取 current Plan 重算 window、price、currency、duration、reset 或 rule。
8. #21 既有 duration/reset、Redemption、并发、overflow、stable sentinel 以及 #22 Credit/current_only/权威 micros/BigInt 合同保持不变。

## TDD 验收

保留既有两个稳定 RED 作为公开领域入口回归；最小 GREEN 后补/保留无 invitation event 的 paid timed 成功重放，断言 `InviterId=0` 且无副作用。
