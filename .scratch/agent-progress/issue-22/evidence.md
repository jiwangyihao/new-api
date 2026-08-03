# Issue #22 验证证据

## 基线
- `git rev-parse HEAD`：`53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- `git status --porcelain`：空。
- 分支：`jiwangyihao/issue-22-credit-tracer`。
- `git merge-base HEAD jiwangyihao/credit-operational-value-integration`：`53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。

## RED/GREEN
- RED：`go test ./model -run TestCreditValuationOrderIngressCreatesExactState -count=1` 通过真实 SQLite、订单快照与 `CompleteSubscriptionOrderTx`，稳定失败于 `credit_valuation_states` 行不存在；证明现有完成入口仅增加 `token_limit`，未创建估值状态。
- GREEN：实现只读 marker predicate、订单冻结来源构造、每份权益初始状态和 `GrantCreditBalanceTx` 同事务双写后，ready 用例通过；状态 `available=1000/exact=40000000 CNY/version=1`，ledger 同步记录 exact。
- GREEN：marker 非 ready 且快照缺失 micros/估值币种时，订单仍走原数量路径成功入账 1,000 Credit，不创建状态且 ledger 估值字段保持 0；证明加表阶段兼容旧路径。
- 范围校正：协调器明确 #22 只覆盖 CNY→CNY 同币种；未新增/写入 FX 来源合同。marker 仅允许只读 predicate，测试直接预置状态；生命周期写入仍归 #27。
- RED：`TestCreditValuationRequestPreConsumeRemovesMovingAverageCost` 经真实 `request_id` 预扣后 subscription 已消费 200，但状态仍保持 `available=1000`，证明预扣旁路未双写。
- GREEN：`ApplyCreditValuationOutflowTx` 按操作前 1,000 Credit 池移除 8,000,000 exact micros，并在同事务写预扣快照；状态变为 `available=800/exact=32000000/version=2`。
- RED：同步最终结算测试编译失败 `undefined: SettleCreditRequestTarget`。
- GREEN：最小 request seam 只允许预扣目标原值最终化；同一目标重放不更新 `settlement_version/state_version/finalized_at`，不同目标稳定返回 `credit_valuation_target_conflict`。

## 约束证据
- 金额权威字段为十进制 micros；后端内部使用整数，前端使用 BigInt/字符串优先。
- Credit 分析显式按 `entitlement_type=credit_balance` 分流；不读取零价容器价格、不看 `end_time`，来源固定 `credit_balance_pool/moving_weighted_pool`。
- 状态缺失/不一致、币种、溢出、档位和幂等问题必须稳定错误码并整体回滚。

## 运行记录
- 2026-08-03：首次真实订单 tracer RED，预期状态 `available=1000/exact=40000000 CNY/version=1`，实际 `record not found`。
- 2026-08-03：收到协调器范围指令；下一 GREEN 只消费订单冻结的 CNY micros/Credit 快照，测试预置已有 `ready` marker，生产代码不创建或修改 marker。
- 2026-08-03：`go test ./model -run 'TestCreditValuationOrderIngress(CreatesExactState|PreservesLegacyPathWhenMarkerNotReady)' -count=1` 返回 `go test: 1 packages ok`。
- 2026-08-03：`go test ./model -run 'Test(CreditValuationRequestPreConsumeRemovesMovingAverageCost|CreditBalanceLifecycleAcrossBillingStrategiesAndCache|PreConsumeUserSubscriptionByUnitsReturnsPlanMetadata)' -count=1` 返回 `go test: 1 packages ok`。
- 2026-08-03：`go test ./model -run 'TestCreditValuationRequest(FinalizesSameTargetIdempotently|PreConsumeRemovesMovingAverageCost)' -count=10` 返回 `go test: 1 packages ok`。


## 五接口 Credit tracer
- RED：新增 `TestCreditValuationFiveAnalyticsViewsAgreeOnThirtyTwoCNY` 后，paid-value DTO 将 `time_based_value` 改为 nullable，但旧行构造仍写入值对象，编译稳定失败于 `cannot use dto.AdminAnalyticsMoneyAmount as *dto.AdminAnalyticsMoneyAmount`；旧 row guard 同时只接受 `plan.price_amount > 0`，无法表示零价全局 Credit 池。
- GREEN：`go test ./model -run TestCreditValuationFiveAnalyticsViewsAgreeOnThirtyTwoCNY -count=1` 返回 `go test: 1 packages ok`。
- 真实路径：订单创建冻结 `40,000,000` micros CNY / `1,000` Credit，经 `CompleteSubscriptionOrderTx` 入账，再由真实 `request_id` 预扣并最终结算 200；验收主流程未直接插入 `CreditValuationState`。
- 五视图一致：summary/users/subscriptions/plans/sources 均读取同一状态，recognized/exact 为 `32,000,000` micros CNY，estimated 为 0，unknown 为 0；summary `active_paid_subscription_count=1`。
- 明细语义：`available_credit=800`、`time_based_value=null`、basis=`credit_moving_weighted_average`、source=`credit_balance_pool`、attribution=`moving_weighted_pool`。
- 回归：`go test ./model -run 'Test(PaidSubscriptionValue|CreditValuationFiveAnalyticsViewsAgreeOnThirtyTwoCNY)' -count=1` 返回 `go test: 1 packages ok`。
- 编译诊断：`dto/admin_analytics.go` 与 `model/admin_analytics_paid_subscription.go` 的 LSP diagnostics 均为 `OK`。
- 边界：本安全点未改 paid-value 查询签名、未扩展 warning 设计、未写 FX 字段、未创建或转换 migration marker。

## 分析安全点复核
- 提交：`452a75ccd feat(analytics): 接入 Credit 精确剩余价值`。
- GREEN 命令：`go test ./model -run 'Test(PaidSubscriptionValue|CreditValuationFiveAnalyticsViewsAgreeOnThirtyTwoCNY)' -count=1`；2026-08-03 复跑结果为 `go test: 1 packages ok`。
- 五接口事实：summary/users/subscriptions/plans/sources 均由同一状态得到 recognized/exact=`32,000,000` micros CNY、estimated=`0`、unknown=`0`；summary `active_paid_subscription_count=1`。
- micros 与多币种 DTO：后端聚合和排序使用 `int64` micros，JSON 权威金额为十进制字符串 `amount_micros`；兼容 `amount` 只由 micros 派生。summary/user/plan/source 的 `*_by_currency` 保持数组形状，可按原币种并列承载，不跨币种相加。
- current-only 边界：当 `CreditValuationState.updated_at > snapshot_at` 时，返回最新状态、版本和 `snapshot_semantics=current_only`，不伪造历史回放。现有 paid-value 查询签名与顶层 warning shape 未扩展；UI 只把该稳定语义显示为非阻断提示。
- 范围边界：#22 仅验证 CNY→CNY；不实现运行时 FX、汇率字段写入、marker 创建/CAS/状态转换或启动自动 ready。

## 2026-08-04 默认前端恢复 RED
- 恢复提交：`7f5455011`；后端安全点已覆盖订单冻结来源、人民币余额/受控完成入口、真实 `request_id` 消费和五接口 32 CNY。
- RED 命令：`bun test src/features/admin-analytics/lib/format.test.ts src/features/admin-analytics/panel-fields.test.ts`。
- RED 结果：`13 pass / 3 fail`。`amount=999, amount_micros=32000000` 实际显示 `¥999.00`；`amount_micros=9007199254740993000000` 实际显示 `$0.00`；Credit 明细缺少 exact 字段与本地化语义值。
- 后续 GREEN 仅改 `web/default` 通用 micros/BigInt、Credit UI、current-only 与 i18n；不再扩张后端或 #23–#28 范围。

## 2026-08-04 UI WIP/RED 实际状态
- 依赖恢复：`bun install --frozen-lockfile` 成功，`816 packages installed`。
- UI 合跑：`bun test src/features/admin-analytics/lib/format.test.ts src/features/admin-analytics/panel-fields.test.ts src/features/admin-analytics/paid-value-panel.test.tsx` 返回 `16 pass / 1 fail`。
- 已 GREEN：`amount_micros` 优先于兼容 float；超出 JS safe integer 的 micros 由 BigInt/字符串格式化；Credit exact、时间值不适用、moving-weighted、confidence、current-only 字段映射测试通过。
- 仍 RED：新增页面行为测试未提供 TanStack `RouterProvider`，`useLinkProps` 读取空 `router.isServer`；这不是页面行为已通过的证据。
- `bun run typecheck` 在本安全点落盘时尚未结束；不得将其记录为 PASS。
- 本提交明确为 WIP/RED 恢复点；未运行真实浏览器 smoke，未补六语言，未声明 Issue #22 UI 完成。

## Gate C 未完成交接
- 人民币余额已定位：`controller/subscription_payment_balance.go::SubscriptionRequestBalance`，现有真实 HTTP/SQLite 夹具为 `controller/subscription_balance_purchase_test.go::TestSubscriptionBalancePayCreditModeAtomicallyCreditsUniqueBalance`。缺口：该夹具尚未启用估值 schema/ready 前置并断言订单快照 `list_price_micros=40000000` 与 `CreditValuationState exact=40000000`。
- 受控外部支付已定位：`controller/subscription_payment_kyren_test.go` 提供 `kyrenCheckoutFakeAPI`、`performSignedKyrenWebhook` 和 `TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation`。缺口：尚未在订单创建后改当前档位价格、再走签名 webhook，并断言 ledger/state 仍为冻结 40,000,000 micros。
- BillingSession 已定位：`service/billing_session.go::NewBillingSession`、`service/billing.go::PreConsumeBilling/SettleBillingWithInput`、`service/funding_source.go::SubscriptionFunding.PreConsume`。缺口：尚未从上述外部购买结果用真实 `relayInfo.RequestId` 预扣 200、相同目标最终结算并断言 available=800/exact=32,000,000。
- 最小 RED：分别增强现有余额与 Kyren 测试，或新增一个窄垂直测试；必须从 HTTP/payment 入口开始，禁止直接创建 `CreditValuationState`。BillingSession 只复用现有接口，不新增 API。
- 禁止范围：不碰 #23 的 target 增减/退款/异步/coalescer，不碰 #26 FX，不碰 #27 marker 生命周期，不重做已有订单/request_id/五接口。
- 本轮未运行 Gate C 新测试，故状态为诚实未完成；UI WIP 仍以 `91df0bd08` 的 RED/类型失败证据为准。