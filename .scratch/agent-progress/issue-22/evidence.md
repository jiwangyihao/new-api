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

## 约束证据
- 金额权威字段为十进制 micros；后端内部使用整数，前端使用 BigInt/字符串优先。
- Credit 分析显式按 `entitlement_type=credit_balance` 分流；不读取零价容器价格、不看 `end_time`，来源固定 `credit_balance_pool/moving_weighted_pool`。
- 状态缺失/不一致、币种、溢出、档位和幂等问题必须稳定错误码并整体回滚。

## 运行记录
- 2026-08-03：首次真实订单 tracer RED，预期状态 `available=1000/exact=40000000 CNY/version=1`，实际 `record not found`。
- 2026-08-03：收到协调器范围指令；下一 GREEN 只消费订单冻结的 CNY micros/Credit 快照，测试预置已有 `ready` marker，生产代码不创建或修改 marker。
- 2026-08-03：`go test ./model -run 'TestCreditValuationOrderIngress(CreatesExactState|PreservesLegacyPathWhenMarkerNotReady)' -count=1` 返回 `go test: 1 packages ok`。
- 2026-08-03：`go test ./model -run 'Test(CreditValuationRequestPreConsumeRemovesMovingAverageCost|CreditBalanceLifecycleAcrossBillingStrategiesAndCache|PreConsumeUserSubscriptionByUnitsReturnsPlanMetadata)' -count=1` 返回 `go test: 1 packages ok`。
