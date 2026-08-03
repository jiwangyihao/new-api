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


## 2026-08-04 Gate C 恢复执行
- 恢复 HEAD：`6d8d001867a6922eb1a8da9df08befa69a037d1b`；恢复前 `git status --porcelain=v1` 为空。
- 当前阶段：只补 Gate C 的人民币余额真实入口、Kyren 签名 webhook 冻结快照，以及 BillingSession 同步消费 200；完成安全点后再收敛既有 UI RED、六语言与真实浏览器。
- 下一条 RED：增强 `controller/subscription_balance_purchase_test.go` 的真实 Gin + SQLite 购买夹具，预置测试所需 ready 前置，断言订单冻结 `40,000,000` micros CNY / `1,000` Credit 且唯一估值状态 exact 入账；禁止直接插入 `CreditValuationState`。
- 随后 RED：Kyren 订单创建后改价再走真实签名 webhook，必须仍按冻结 `40,000,000` micros 入账并保持重复 webhook 幂等；再从真实购买结果经 `NewBillingSession` / `PreConsumeBilling` / `SettleBillingWithInput` 同步消费目标 200。
- 禁止范围：不实现 #23 target 增减、少结算、退款、异步 identity 或 coalescer；不实现 FX；不创建/CAS/切换 migration marker 或 ready；不重做既有估值、请求 seam 与五接口。
- 恢复指令：`credit-operational-value-issue-22-gate-c-recovery.md` 已完整读取并作为当前范围优先级。

## 2026-08-04 Gate C 余额入口 GREEN
- 真实 `SubscriptionRequestBalance` Gin + SQLite 夹具已进入既有 ready 运行时路径：测试专用 `_test.go` helper 仅 AutoMigrate 估值表并预置 ready 行，不增加任何生产 marker seam 或生命周期逻辑。
- `40 CNY / 1,000 Credit` 权威 micros、CNY、规则版本、目标全局池和 balance payment identity 均冻结到订单快照。
- 首次购买与相同幂等键重放后仅有一张订单、一条 ledger、一行估值状态；人民币余额只扣一次。唯一状态为 available=1000、exact=40000000、estimated=0、unknown=0、version=1。
- ready 估值路径下的 ledger 注入失败回归也已 GREEN：人民币余额、订单、权益、ledger、估值状态与用户选择全部原子回滚。
- 余额入口安全点：`3ed1ef70c test(subscription): 验证余额购买冻结估值`。

## 2026-08-04 Gate C Kyren GREEN
- 真实 Kyren fake checkout 创建订单时冻结 `40,000,000` micros CNY / `1,000` Credit、目标池币种与规则版本；当前档位随后改为 99 CNY 并 disabled。
- 真实 HMAC 签名 `order.paid` webhook 仍按订单快照履约；state/ledger exact 均为 `40,000,000`，重复 webhook 后只有一条 ledger、一行 state 且 version=1。
- 已授权订单按冻结快照履约；同一 disabled 档位的新 checkout 被拒绝且不创建第二张订单。
- 下一步：从此真实 Kyren 购买结果经 BillingSession 的真实 request_id 同步预扣/结算目标 200。

## 2026-08-04 Gate C BillingSession GREEN
- 从真实 Kyren webhook 入账得到的 1,000 Credit 状态继续走 `PreConsumeBilling`，`relayInfo.RequestId=kyren-credit-billing-session-200` 传播到 `SubscriptionFunding` 与 `SubscriptionPreConsumeRecord`。
- BillingSession 足额预扣 200 后，状态为 available=800、exact=32000000、estimated=0、unknown=0、version=2；请求快照扣除 exact=8000000。
- `SettleBillingWithInput` 现通过 `SubscriptionFunding` 的窄 seam 识别已跟踪 Credit 请求，以相同累计目标 200 调用既有深模块最终化；新建第二个 BillingSession 并复用同一 request_id/目标后，数量、价值、version 与 finalized_at 均不变。
- 保持范围：仅同目标一次同步最终化；未实现 target 增减、少结算、退款、异步 identity、coalescer 或 FX。
- Gate C 三段已闭环；下一步提交安全点并收敛既有 RouterProvider UI RED。

## 2026-08-04 UI 与六语言 GREEN 安全点
- TanStack 测试夹具已使用真实 `createRouter` + `createMemoryHistory` + `RouterProvider`，未删除 `Link`、绕过页面或弱化关键 32 CNY/current-only 行为。
- `format` / `panel-fields` / paid-value 页面共 17 项测试 GREEN；`amount_micros`、BigInt、exact/estimated/unknown、Credit 时间值不适用、moving-weighted、confidence、current-only 均被覆盖。
- 修复两个已有 TypeScript nullable 映射错误：仅收紧 `adminAnalyticsCreditOverviewValues` 与 `adminAnalyticsCreditRankingValue` 的返回类型，实际实现本就总返回 string。
- 新增 22 个 Credit 分析可见键，en/zh/fr/ja/ru/vi 全部存在且非空；`i18n:sync` 对六语言均为 missing=0、extras=0。
- `bun run typecheck` 与 `bun run build` 已 GREEN；下一步只做真实 SQLite 五接口回归、真实浏览器 smoke 与最终窄门禁。