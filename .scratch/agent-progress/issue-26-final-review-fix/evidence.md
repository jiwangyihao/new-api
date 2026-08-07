# H1 证据记录

## 安全点

- 冻结实现祖先：`6f865feca3cd517a3dd744e67ea1240d5001d2ed`。
- RED 前安全提交：`5802fb461f70d2da075f13cef4f264282ed8336a`。
- 当前工作树包含仅 H1 RED 测试及 progress 校准；未执行 reset、rebase、分支切换或 origin/main 操作。

## H1 RED

- 精确命令：`go test -v ./model -run '^TestConfirmTimedSubscriptionConversionLocksInFlightRequestsBeforeTargetIngress$' -count=1`
- 真实结果：FAIL，`subscription_conversion_valuation_test.go:700` 报 `Should be empty, but was UserSubscription`。
- 失败语义：`prematureTarget` 记录为 `UserSubscription`；该值来自真实 GORM target mutation callback。独立 SQLite WAL 观察连接确认 request 仍未冻结（`valuation_subscription_id = 0`、`applied_credit = 0`），因此不是编译、迁移、连接或夹具初始化失败。
- 证据边界：测试通过真实双连接、WAL、事务和可控 callback 观察运行时顺序，不检查源码文本；SQLite 不证明其他数据库的行锁语义。
- 结论：Confirm 当前在 `GrantCreditBalanceTx`/目标 ingress 与 `tx.Create(conversion)` 路径完成后，才调用 `freezeTimedConversionInFlightRequestsTx`；request → target 顺序违反合同。

## H1 GREEN

- 状态：未运行。
- 计划：拆成目标 ingress 前 request rows 锁定/验证与捕获、目标 ingress 后仅更新已锁定 rows；相反 settle/refund 入口先按 request identity `FOR UPDATE` 锁定并验证 route，再进入 target mutation。

## 范围与未完成

- 未触碰 #24 adjustment/redemption、#25、#27、#28，也未处理 M1/M3/M2。
- MySQL/PostgreSQL 行锁门禁留给 #27；本阶段只提交跨方言 GORM 锁序实现与 SQLite 行为证据。
- GREEN 后需运行同一测试、`-count=10`、必要窄 `-race`、`gofmt`、`git diff --check`，并在 clean 提交前校准本文件与 `status.md`。
