# Issue #21 宽回归修复 D 合同

状态：IN_PROGRESS

## 冻结生产合同

1. 普通 paid timed 订单必须持有合法 `EntitlementSnapshot`；缺失或非法时继续 fail-closed。
2. 新 subscription redemption 必须经 `Redemption.Insert()` 从授权时 Plan 冻结 `FulfillmentSnapshot`；兑换热路径不得读取 current Plan 补造历史 exact。
3. 测试夹具不得手写快照 JSON；订单使用 `NewSubscriptionEntitlementSnapshot`、`SetPaymentSnapshot`、`MarshalSubscriptionEntitlementSnapshot` 等现有领域接口。
4. 历史缺快照不得按当前 Plan 热补 exact；#22 的 CreditValuation、request_id、current_only、权威 micros、BigInt 与 BillingSession 行为保持不变。

## Test-only 夹具合同

### Plan

合法付费计时 Plan 必须同时满足：

- enabled；
- 非 trial、非 invite-trial；
- explicit timed entitlement；
- 正 Credit；
- 正且权威的 `price_amount_micros` 与合法 currency；
- 合法 duration、reset 与规则版本事实。

### Provider order

pending provider order 的不可变授权快照必须与 Plan、provider、amount/currency 一致，并通过现有 snapshot interface 生成和持久化。

### Redemption

subscription redemption 创建必须调用 `Redemption.Insert()`，由插入事务冻结 `FulfillmentSnapshot`；测试不得直接 `DB.Create` 绕过前向授权入口。

## 必须保留的观察合同

- 邀请事件与佣金语义；
- 续期事件仅覆盖 renewal delta；
- 成功重放恢复同一结果且不重复 transition；
- reward-ineligible Plan 仍记录原业务事件；
- 并发 worker 数不降低，并保持恰好一次 transition、一份 subscription、一份 event；
- 达到历史购买限制时仍允许合法续期。

## 清理与边界

可补齐两个所属测试文件实际使用的表清理，避免缺表日志；不得通过修改生产代码、删除断言、放宽计数或 skip 让测试通过。合法快照夹具下若仍暴露生产失败，立即停止扩大范围并通过 Orca escalation/question 报告最小复现、生产函数与事务状态。

## 恢复与验收

恢复入口为 `wide-model-status.md` 的命令。最终必须运行九项定向测试、并发/重放三项 `-count=10`、完整 `model` 包、`git diff --check` 和工作树清洁检查；所有修改采用小步 Conventional Commits。
