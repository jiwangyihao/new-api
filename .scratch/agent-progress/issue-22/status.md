# Issue #22 执行状态

## 当前阶段
- 基线与合同：已完成。
- 领域 tracer：订单冻结估值、真实 `request_id` 预扣 200 与同目标最终结算已 GREEN。
- 分析 tracer：summary/users/subscriptions/plans/sources 五个 paid-value 查询共享同一 `CreditValuationState`，均返回 `32,000,000` micros CNY；`active_paid_subscription_count=1`。
- 分析 DTO：金额同时保留兼容 `amount` 与权威十进制字符串 `amount_micros`，所有分组继续使用 `*_by_currency` 数组承载多币种结果；#22 运行时仅实现 CNY→CNY，不引入 FX。
- current-only 边界：Credit 状态晚于请求 `snapshot_at` 时，明细以 `snapshot_semantics=current_only` 标记最新状态；不伪造历史值，不扩展 paid-value 查询签名或 marker 生命周期。
- 当前工作：从 `7f5455011` 恢复默认前端 UI RED；后端订单、购买、`request_id`、五接口、FX 与 marker/ready 边界均不再扩张。
- 基线：`53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- 工作树：`jiwangyihao/issue-22-credit-tracer`。

## 已完成
- 真实订单创建冻结 `40 CNY / 1,000 Credit`，完成后状态为 `available=1000/exact=40000000 CNY/version=1`。
- 真实 `request_id` 同步消费 200 后状态为 `available=800/exact=32000000/estimated=0/unknown=0/version=2`。
- paid-row builder 按 `entitlement_type` 显式分流；Credit 不读取零价全局容器价格、不看 `end_time`、不关联猜测订单。
- Credit 金额以 `int64` micros 聚合，五个视图共享 `credit_balance_pool / moving_weighted_pool` 来源事实；兼容 float 仅由 micros 派生，DTO 的 `*_by_currency` 保留多币种分组能力。
- Credit 明细返回 `time_based_value=null`、`valuation_basis=credit_moving_weighted_average`、`available_credit=800`；状态晚于快照时返回 `snapshot_semantics=current_only`，供 UI 显示非阻断提示。

## 当前目标
默认前端优先解析 `amount_micros` 并以 BigInt/字符串格式化，展示 exact/estimated/unknown、Credit 时间值不适用和移动加权平均术语；补齐 en/zh/fr/ru/ja/vi。

## 下一步
1. 先提交恢复现场：进度记录与两份 UI RED 行为测试。
2. 更新默认前端 paid-value 类型、BigInt micros 格式化和 Credit 语义字段。
3. 增加 current-only 非阻断提示与刷新，补齐六语言并运行定向测试和真实浏览器 smoke。

## 阻塞
当前无外部阻塞。#21 timed grant 只保留窄扩展 seam，不实现其时间线。
- 协调器收敛：五接口安全点不重构查询签名、不扩展 warning 设计；不实现 FX 或 migration marker 生命周期。

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
