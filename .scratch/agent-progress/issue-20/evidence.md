# Issue #20 验证证据

## 约束基线

- 父 PRD：`issue://jiwangyihao/new-api/19`
- 当前切片：`issue://jiwangyihao/new-api/20`
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- 共享执行协议：`docs/agents/credit-operational-value-execution.md`

## 已执行

- 阅读并确认 Issue #20 验收标准。
- 阅读 `CONTEXT.md`、ADR 0001、ADR 0002。
- 阅读规格 4.1–5.7 与实施计划任务 1–2。
- 加载 `skill://tdd`：后续严格采用单个可观察行为的 RED → GREEN 循环。

## RED/GREEN 记录

- RED：`go test ./model -run TestCreditValuationMathMulDivFloorOrdinaryValue -count=1`，编译失败 `undefined: mulDivFloor`。
- GREEN：最小整数实现后普通值 `40,000,000 × 800 / 1,000 = 32,000,000` 通过。
- RED：零分母缺少稳定 sentinel；补充 `ErrCreditValuationDivisionByZero` 后通过。
- RED：`MaxInt64 × MaxInt64 / MaxInt64` 旧 `int64` 中间乘法得到 0；改用 `math/bits.Mul64/Div64` 后通过。
- RED：`MaxInt64 × 2 / 1` 缺少结果溢出检测；补充 `credit_valuation_overflow` 后通过。
- RED：`ParseDecimalAmountMicros` 不存在；实现严格十进制文本解析后六位小数通过。
- RED：负数、七位小数、`MaxInt64` 边界与越界输入缺少稳定错误；补充非负、精度和防溢出检查后通过。
- GREEN：`go test ./model -run 'Test(ParseDecimalAmountMicros|CreditValuationMathMulDivFloor)' -count=1`，目标包通过。
- RED：`go test ./controller -run TestAdminCreateSubscriptionPlanRoundTripsExactPriceMicros -count=1` 返回成功但响应缺少 `price_amount_micros`，SQLite 查询报 `no such column: price_amount_micros`。
- GREEN：`SubscriptionPlan` 增加 nullable `BIGINT` 权威字段并以 JSON 字符串序列化；创建定向测试通过，数据库值与响应均为 `40123456`。
- RED：旧套餐迁移后查询 `price_amount_micros` 报列不存在；SQLite 附加迁移增加 nullable 列后，`TestSubscriptionPlanPriceMicrosMigrationLeavesLegacyRowPending` 证明旧有价行仍为 `NULL`，未填充虚假精确值。
- RED：管理员创建对缺少 micros、负数、超过六位小数、micros 溢出、兼容值不一致均未提供稳定 code，且三类非法请求会落库。
- GREEN：原始 JSON 十进制文本进入统一价格模块；上述请求在写事务前按 `subscription_plan_price_*`/`credit_valuation_overflow` 稳定 code 原子拒绝。
- GREEN：`TestAdminUpdateSubscriptionPlanRoundTripsExactPriceMicros` 证明编辑同时写兼容 DECIMAL 与权威 micros，随后管理列表刷新返回同一 micros 字符串。
- RED：Credit 计划首次配置缺少/使用 EUR 时仍成功；已有 Credit 权益后从 CNY 改 USD 还连带修改其他配置。
- GREEN：专用接口只接受 CNY/USD，首次配置必填；事务锁定计划并检查 Credit 权益、估值状态或账本，存量存在时返回 `credit_valuation_currency_locked` 并回滚全部字段。
- RED：估值状态、迁移 marker、计时 grant 与请求/低频快照结构不存在，schema 测试编译失败。
- GREEN：附加式模型注册到标准/快速迁移；真实 SQLite 两次迁移均成功，状态 user 唯一、grant 幂等键及来源组合唯一由数据库拒绝重复，marker 保持空表且 #20 未写状态。
- GREEN：比例测试补齐普通值、向下取整、完全清空吸收余数、零分母、`MaxInt64` 宽中间乘积和结果溢出。
- RED：只读价格诊断类型/入口不存在；GREEN：按套餐 ID 排序输出 `negative`、`invalid_decimal`、`precision_exceeds_six`/`overflow`，前后 `price_amount_micros` 快照完全一致。
- RED：前端 `Number` 往返破坏大十进制；GREEN：表单保持原始字符串，以 `BigInt` 生成 micros，序列化 payload 同时保留原十进制与精确 micros；刷新由 micros 还原文本。
- GREEN：Credit 计划 UI 显式选择 CNY/USD 并提示冻结条件；六语言新增文案，i18n 报告六语言 `missingCount=0`、`extrasCount=0`。
- GREEN：`bun run typecheck` 与两份定向前端测试通过（25 pass, 0 fail）。

当前实现成本：比例热路径只执行 `math/bits` 128 位乘积/除法和常数次分支；不使用浮点或 `big.Int`，无设计上的堆分配。

## 最终定向门禁

- `go test ./model -run 'Test(ParseDecimalAmountMicros|CreditValuationMath|CreditValuationSchema|DiagnosePendingSubscriptionPlanPrices|SubscriptionPlanPriceDiagnosticQuerySupportsAllDialects|SubscriptionPlanPriceMicrosMigrationLeavesLegacyRowPending|EnsureSubscriptionPlanTableSQLiteCreatesCreditBalanceSchema)' -count=1`：通过，`go test: 1 packages ok`。
- `go test ./controller -run 'Test(AdminCreateSubscriptionPlan|AdminUpdateSubscriptionPlan|AdminCreditBalancePlan|AdminPlanListExcludesDedicatedCreditBalancePlan|PublicPlanListNeverExposesCreditBalancePlan|SubscriptionBalancePayRejectsDisabledPlan|SubscriptionPurchaseDoesNotUpdateUserGroup)' -count=1`：通过，`go test: 1 packages ok`。
- `go test ./router -run 'Test(CreditBalanceAdminRoutePersistsDedicatedConfiguration|SubscriptionPlansPublicRoute|SubscriptionPlansProtectedDTOOmitsLegacyBusinessGroupFields)' -count=1`：首次发现公开 DTO 允许键仍为 16；补入 nullable `price_amount_micros` 契约后通过，提交 `b1ca3541b`。
- `bun run typecheck && bun test src/features/subscriptions/lib/plan-form.test.ts src/features/subscriptions/components/credit-balance-plan-card.test.tsx`：通过，`25 pass / 0 fail`。
- `bun run i18n:sync`：完成；en、zh、fr、ru、ja、vi 均为 `missingCount=0`、`extrasCount=0`。报告中既有 untranslated 计数不属于本切片新增缺失。
- `go test ./model -run TestCreditBalanceProductionMigrationExternalDatabases -count=1 -v`：测试包通过但 MySQL、PostgreSQL 两项均因 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN` 未设置而 skip；这不是实际数据库验收，最终结论必须如实限定为真实 SQLite + 外部方言/SQL 合同测试。
- `gofmt` 已作用于本切片 Go 文件；`git diff --check` 通过。

## 隔离应用与浏览器证据

- 静态产物：`web/default` 的 `bun run build` 成功；`web/classic` 安装本地依赖后 `bun run build` 成功，仅用于满足 `go:embed` 启动前提。
- 启动命令：`PORT=3100 SQLITE_PATH='C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-20-valuation-foundation/.scratch/agent-progress/issue-20/browser-smoke.db' SESSION_SECRET='issue20-browser-smoke-session-secret' go run .`。
- 隔离性：`.scratch/agent-progress/issue-20/browser-smoke.db` 实际落盘；`http://127.0.0.1:3100/api/setup` 可访问。初始化时响应 `database_type=sqlite`；重启后同一数据库返回 `status=true`。
- default UI：受控 Chromium 打开 3100，标题为 `New API`，观察到 default 导航；使用隔离管理员账号登录后进入 `/dashboard/overview`。该证据只证明 default 首页与登录，不等同于套餐 create → edit → refresh。
- Orca 内嵌标签阻塞：`orca tab create --worktree active --url http://127.0.0.1:3100 --json` 多次返回 `runtime_error: Tab creation timed out`；`orca tab list --worktree all --json` 返回 `tabs=[]`。
- Orca 桌面控制阻塞：防火墙弹窗关闭后，`orca computer hotkey --app Orca --window-id 7870344 --restore-window --key CmdOrCtrl+L --json` 仍返回 `window_not_focused`；未继续探索新桌面控制方案。
- 已发送 escalation `msg_065b59e8fd5f`；协调器随后用内置 browser tool 接管套餐完整往返。隔离账号、页面 URL、create/edit 值与精确 payload 期望已通过回复 `msg_8f2700429491` 交接。

## 尚待协调器补入

- default 管理套餐 create：`price_amount="9007199254.740991"` 与 `price_amount_micros="9007199254740991"` 的实际 network payload/响应。
- edit 为 `40.654321` / `40654321` 后刷新显示 `40.654321` 的实际浏览器证据。
- 完整浏览器结果到达后再关闭隔离后台进程；此前不发送 `worker_done`。
