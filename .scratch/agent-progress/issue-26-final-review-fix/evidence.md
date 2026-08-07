# H1 证据记录

## 安全点与 RED

- 冻结实现祖先：`6f865feca3cd517a3dd744e67ea1240d5001d2ed`；progress 安全提交：`5802fb461f70d2da075f13cef4f264282ed8336a`；H1 RED 提交：`a9b6d92e668e73426f72d235cd4bad10a05c3e94`。
- RED 命令：`go test -v ./model -run '^TestConfirmTimedSubscriptionConversionLocksInFlightRequestsBeforeTargetIngress$' -count=1`。
- RED 结果：FAIL；`subscription_conversion_valuation_test.go:700` 报 `Should be empty, but was UserSubscription`。真实 SQLite WAL 双连接夹具已完成初始化；独立连接观察到 `valuation_subscription_id = 0`、`applied_credit = 0`，所以失败不是编译、迁移或夹具故障。
- 结论：Confirm 在 request rows 选择/验证前进入目标 `UserSubscription` mutation，违反 request → target 固定锁序。

## H1 GREEN

- `subscription_conversion.go`：新增两阶段深模块 seam。`prepareTimedConversionInFlightRequestsTx` 在目标 ingress 前按 `request_id asc, id asc` 加 `UPDATE` 锁、验证并捕获 rows；`applyTimedConversionInFlightRequestsTx` 在目标 ingress 后只更新已捕获 rows。
- `credit_valuation.go`：`SettleCreditRequestTargetTx` 入口按 `request_id` 加 `UPDATE` 锁，完整校验 route 的 id、user、source 与 valuation mapping，以锁定 record 覆盖传入 route 后才进入正向 settle 或 refund target mutation。
- `subscription_delta_coalescer.go`：批次按 request identity 稳定排序，第一轮锁定并验证全部 routes，第二轮才执行 target settlement；错误仍按原始请求索引回填。

## GREEN 验证

- `go test -v ./model -run '^TestConfirmTimedSubscriptionConversionLocksInFlightRequestsBeforeTargetIngress$' -count=1`：PASS。
- `go test ./model -run '^TestConfirmTimedSubscriptionConversionLocksInFlightRequestsBeforeTargetIngress$' -count=10`：PASS。
- `go test -race ./model -run '^TestConfirmTimedSubscriptionConversionLocksInFlightRequestsBeforeTargetIngress$' -count=1`：PASS。
- `go test ./model -run '^(TestTimedConversionConcurrentWithFinalSettleUsesLegalSerialization|TestTimedConversionConcurrentWithFullRefundUsesLegalSerialization|TestConfirmTimedSubscriptionConversionConcurrentSameFactsWritesOnce|TestCreditRequestTargetCoalescerPreservesEnqueueOrderAndResults|TestCreditRequestTargetCoalescerRollsBackBatchAndAttributesMiddleFailure)$' -count=1`：PASS。
- 同一 H1/settle/refund/concurrent/coalescer 定向集合 `-count=10`：PASS；同集合窄 `-race -count=1`：PASS。
- 协调器独立验证当前三文件的 `TestConfirmTimedSubscriptionConversion`、`TestCreditValuationRequestTarget`、`TestFlushSubscriptionRequestTargets` 相关回归：PASS；`gofmt` 与 `git diff --check`：PASS。

## 边界

- 未触碰 #24 adjustment/redemption、#25、#27、#28，也未处理 M1/M3/M2。
- SQLite 证据验证真实事务顺序与合法串行化，但不代替 MySQL/PostgreSQL 行锁门禁；三数据库验证留给 #27。
- 本次提交将包含三个 H1 生产文件与本状态/证据校准；提交后必须确认 `git status --short` 为空。
