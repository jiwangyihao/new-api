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