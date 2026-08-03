# Issue #22 执行状态

## 当前阶段
- 基线与合同：已完成。
- 领域 tracer：订单冻结估值、真实 `request_id` 预扣 200 与同目标最终结算已 GREEN。
- 分析 tracer：summary/users/subscriptions/plans/sources 五个 paid-value 查询共享同一 `CreditValuationState`，均返回 `32,000,000` micros CNY；`active_paid_subscription_count=1`。
- 分析 DTO：金额同时保留兼容 `amount` 与权威十进制字符串 `amount_micros`，所有分组继续使用 `*_by_currency` 数组承载多币种结果；#22 运行时仅实现 CNY→CNY，不引入 FX。
- current-only 边界：Credit 状态晚于请求 `snapshot_at` 时，明细以 `snapshot_semantics=current_only` 标记最新状态；不伪造历史值，不扩展 paid-value 查询签名或 marker 生命周期。
- 当前工作：已停止实现并进入交接；Gate C 两个真实入口的补充验收尚未完成，UI RED 已由 `91df0bd08` 保存。
- 基线：`53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- 工作树：`jiwangyihao/issue-22-credit-tracer`。

## 已完成
- 真实订单创建冻结 `40 CNY / 1,000 Credit`，完成后状态为 `available=1000/exact=40000000 CNY/version=1`。
- 真实 `request_id` 同步消费 200 后状态为 `available=800/exact=32000000/estimated=0/unknown=0/version=2`。
- paid-row builder 按 `entitlement_type` 显式分流；Credit 不读取零价全局容器价格、不看 `end_time`、不关联猜测订单。
- Credit 金额以 `int64` micros 聚合，五个视图共享 `credit_balance_pool / moving_weighted_pool` 来源事实；兼容 float 仅由 micros 派生，DTO 的 `*_by_currency` 保留多币种分组能力。
- Credit 明细返回 `time_based_value=null`、`valuation_basis=credit_moving_weighted_average`、`available_credit=800`；状态晚于快照时返回 `snapshot_semantics=current_only`，供 UI 显示非阻断提示。

## 当前目标
形成干净、可恢复的交接安全点；不再修改代码或扩张范围。

## Gate C 未完成交接
1. 人民币余额入口已定位：`controller/subscription_payment_balance.go` 的 `SubscriptionRequestBalance` → `service.CreateBalanceSubscriptionOrder` → `model.CompleteSubscriptionOrderTx`；现有夹具在 `controller/subscription_balance_purchase_test.go`。
2. 受控外部入口已定位：`controller/subscription_payment_kyren_test.go` 的 Kyren fake checkout、`performSignedKyrenWebhook` 与 `TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation`；生产完成仍进入 `CompleteSubscriptionOrderTx`。
3. BillingSession 入口已定位：`service.NewBillingSession` / `PreConsumeBilling` / `SettleBillingWithInput`，底层 `SubscriptionFunding.PreConsume` 传播 `relayInfo.RequestId`。最小 RED 应从已完成购买的 1,000 Credit 状态开始，经真实 request_id 预扣并以相同目标 200 最终化，断言 available=800、exact=32,000,000；不得设计新 API。

## 禁止范围
- 不重做已有订单、model request seam 或五接口；不实现 target 增减、退款、异步 identity、coalescer、FX、marker/ready 或 #23–#28。
- 不宣称 Gate C 完成：尚未新增/运行人民币余额 exact 状态测试，也未新增/运行 Kyren 改价后 webhook + BillingSession 200 的垂直测试。

## 阻塞
无外部阻塞；由下一执行者按上述最小 RED 续作。

## 安全点
- 已提交领域安全点：`06619f81b feat(valuation): 幂等完成 Credit 同步结算`。
- 已提交分析安全点：`452a75ccd feat(analytics): 接入 Credit 精确剩余价值`。
- GREEN：`go test ./model -run 'Test(PaidSubscriptionValue|CreditValuationFiveAnalyticsViewsAgreeOnThirtyTwoCNY)' -count=1` 返回 `go test: 1 packages ok`；覆盖五接口 32 CNY、权威 micros、paid count 1，并回归既有 paid-value 行为。

## 非所有权
不实现 #23 的追加/少结算/退款/异步任务/coalescer；不实现 #24–#28 的兑换/转换/售后正向入账、恢复、FX 在途、历史迁移/ready、发布。

## 2026-08-04 恢复安全点
- 恢复 HEAD：`7f5455011 chore(issue-22): 固化五接口验证证据`。
- 恢复时仅有 `format.test.ts` 与 `panel-fields.test.ts` 两份 UI RED 未提交；后端工作树无未提交改动。
- RED：`bun test src/features/admin-analytics/lib/format.test.ts src/features/admin-analytics/panel-fields.test.ts` 得到 `13 pass / 3 fail`，失败分别证明旧 float 覆盖 micros、大整数精度丢失、Credit exact/时间值/估值语义字段缺失。
- 协调边界：本轮只完成默认前端、六语言、浏览器与最终门禁，不继续修改已验收的后端 tracer。

## 2026-08-04 UI WIP/RED 安全点
- `bun install --frozen-lockfile` 已按锁文件恢复 816 个依赖。
- `format.test.ts` 与 `panel-fields.test.ts` 已 GREEN；与新增页面测试合跑为 `16 pass / 1 fail`。
- 唯一 RED 是 `paid-value-panel.test.tsx` 缺少 TanStack `RouterProvider` 测试夹具，渲染 `Link` 时触发 `router.isServer` 空引用；尚未取得真实页面 GREEN 或浏览器证据。
- 类型检查已启动但保存安全点时仍在运行，不声明通过。
