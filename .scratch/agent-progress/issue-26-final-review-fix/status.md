# H1 当前状态

- 冻结实现祖先：`6f865feca3cd517a3dd744e67ea1240d5001d2ed`。
- 当前子树 HEAD：`a9b6d92e668e73426f72d235cd4bad10a05c3e94`（H1 行为 RED 提交）；当前 phase 为 H1 GREEN 完成、待提交 clean 实现安全点。
- 未提交文件：`model/credit_valuation.go`、`model/subscription_conversion.go`、`model/subscription_delta_coalescer.go`、本文件与 `evidence.md`；没有其他业务文件。
- RED：`go test -v ./model -run '^TestConfirmTimedSubscriptionConversionLocksInFlightRequestsBeforeTargetIngress$' -count=1` 失败；`subscription_conversion_valuation_test.go:700` 的 `prematureTarget` 实际为 `UserSubscription`，独立 SQLite WAL 连接同时观察到 request 仍未冻结。RED 已提交为 `a9b6d92e6`。
- 根因：Confirm 在捕获在途 request 前进入目标 `UserSubscription`/valuation/ledger；正向 settle 执行 target mutation 后才锁 request；coalescer 批次还可能按到达顺序交替执行 request → target。
- GREEN：Confirm 先按 `request_id asc, id asc` 锁定、验证并捕获全部在途 request，目标 ingress 后只更新已捕获 rows；settle/refund 统一在 target 前锁定并核对 request；coalescer 先按 request identity 排序并锁定全部 routes，第二轮才执行 target settlement，结果仍按原索引回填。
- 验证：同一行为测试单次 PASS、`-count=10` PASS、窄 `-race` PASS；final settle、full refund、同 source conversion、coalescer 顺序与批次回滚相关既有回归 PASS。协调器另行验证 `TestConfirmTimedSubscriptionConversion`、`TestCreditValuationRequestTarget`、`TestFlushSubscriptionRequestTargets` 相关回归均 PASS。
- 格式与静态门禁：协调器已对当前三文件运行 `gofmt` 与 `git diff --check`，均 PASS。
- 范围：未触碰 #24/#25/#27/#28 或 M1/M3/M2；SQLite 不冒充 MySQL/PostgreSQL 行锁验证，真实三数据库仍留给 #27。
- 阻塞：无。下一动作仅提交上述三文件与进度记录，确认工作树 clean 后发送 `worker_done`。
