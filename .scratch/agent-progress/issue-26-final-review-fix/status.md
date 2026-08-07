# H1 当前状态

- 冻结实现祖先：`6f865feca3cd517a3dd744e67ea1240d5001d2ed`
- 当前子树 HEAD：`5802fb461f70d2da075f13cef4f264282ed8336a`（保留协调文档与本阶段安全提交）
- 当前 phase：H1 request → target 固定锁序；尚未进入 M1/M3/M2。
- 最近安全提交：`5802fb461f70d2da075f13cef4f264282ed8336a`；当前未提交改动为 H1 RED 测试及本次 progress 校准。
- RED：已运行 `go test -v ./model -run '^TestConfirmTimedSubscriptionConversionLocksInFlightRequestsBeforeTargetIngress$' -count=1`；真实 SQLite WAL 双连接夹具初始化完成，测试在 `subscription_conversion_valuation_test.go:700` 的 `prematureTarget` 行为断言失败，实际首个目标 mutation schema 为 `UserSubscription`，不是初始化/编译/夹具失败。
- RED 观察：独立连接读取到 request 尚未冻结，随后断言 `prematureTarget` 应为空但实际为 `UserSubscription`；证明 Confirm 在 request rows 选择/验证前进入 target ingress。
- GREEN：尚未运行；下一步只拆分 request rows 的锁定/验证与目标写入，并将 settle/refund 入口改为 request-first。
- 未提交文件：`model/subscription_conversion_valuation_test.go`、本文件及 `evidence.md`；未触碰 #24/#25/#27/#28 或 M1/M3/M2。
- 阻塞：无 provider 阻塞；SQLite 证据不冒充 MySQL/PostgreSQL 行锁门禁。
- 下一动作：实现最小 H1 request → target 固定锁序，先跑同一测试 GREEN，再跑单次、`-count=10` 与窄 `-race`，校准证据并提交 clean 安全点。
