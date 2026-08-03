# Issue #20 协调器验收与集成清单

## 用途与基线

本清单供协调器在 Orca Dispatch `ctx_b71024f8a8ac` 发出 `worker_done` 后验收 GitHub Issue #20。验收对象是工作树 `C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-20-valuation-foundation`、分支 `jiwangyihao/issue-20-valuation-foundation`，派发基线为集成分支提交 `c1fa6845e6f3707095bbb857b2c16be3b5068ab1`。

验收必须同时满足父 PRD #19、Issue #20、ADR 0002、2026-08-02 规格与计划，以及 `docs/agents/credit-operational-value-execution.md`。任何失败都回到原 Worker 修复；不能为了尽快集成而删除测试、弱化错误码、伪造数据库证据或把工作转移到下游 Issue。

#20 只拥有：

1. 前向套餐精确价格解析、持久化、API/UI 往返与稳定错误；
2. Credit 全局估值币种首次配置和普通更新冻结；
3. 后续切片需要的附加式 schema、稳定类型、命名唯一约束；
4. 对历史非法价格的确定性只读诊断；
5. 无浮点、无按请求 `big.Int` 分配的防溢出整数比例能力。

#20 不拥有历史价格回填、历史 Credit/计时估值重建、migration marker 状态改变、非法历史行对 `ready` 的裁决，也不启用 Credit 数量/估值强制双写。这些边界只要有一项被越过，本轮不得集成。

## Gate A：Worker 交付完整性

- [ ] Orca 当前 Dispatch 恰好收到一次 `worker_done`；记录消息 ID、完成时间与 Worker 最终状态。
- [ ] `worker_done` 列出所有实现提交 SHA、修改范围、定向测试、浏览器证据、实际数据库证据、遗留风险和 `.scratch/agent-progress/issue-20` 路径。
- [ ] `status.md`、`evidence.md`、`contract.md` 已更新到最终状态；内容与代码及 `worker_done` 一致。
- [ ] Worker 工作树无 staged、unstaged 或 untracked 文件；不接受只存在于终端、stash、备份文件或未提交 `.scratch` 文件中的成果。
- [ ] 从派发基线 `c1fa6845e6f3707095bbb857b2c16be3b5068ab1` 到 Worker HEAD 的提交均属于 #20；提交消息符合 Conventional Commits，subject 使用简体中文。
- [ ] `git diff --check c1fa6845e6f3707095bbb857b2c16be3b5068ab1..jiwangyihao/issue-20-valuation-foundation` 无输出。
- [ ] 未修改受保护项目标识，未触碰主工作树的 `CLAUDE.md`，未包含凭据、DSN、生成物、临时测试数据或大段一次性脚本。

## Gate B：所有权与设计审查

逐文件审阅完整 diff，而非只看 Worker 摘要：

- [ ] `SubscriptionPlan` 的精确价格是可表示“历史待迁移”的 nullable `BIGINT`；历史行加列后保持 `NULL`，不得由旧 `float64` 或当前价格推导。
- [ ] 管理员前向写入以 micros 字符串为权威来源；兼容 `price_amount` 只能由严格十进制输入派生或做一致性校验，不能反向进入估值算术。
- [ ] `CreditValuationMigration` 在本切片只注册结构；代码没有创建/更新 marker、切换 `pending/running/ready/failed/suspended` 或据诊断阻止 `ready`。
- [ ] 没有历史价格 `UPDATE`、回填循环、历史来源重建或 `CreditValuationState` 部分初始化。
- [ ] 没有把 Credit 数量写改成 ready 后强制双写；#22 和 #27 的所有权保持完整。
- [ ] 附加模型只有一套稳定概念，没有为后续切片创建平行金额、状态或 FX 抽象。
- [ ] 所有 JSON 编解码调用遵循 `common/json.go`；所有 schema/SQL 同时考虑 SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6。
- [ ] 修改导出符号时已检查引用并更新所有现有调用点；旧购买、disabled-plan 消费和 `model_limits` 忽略行为未改变。

## Gate C：整数金额与 API 合同

在 Worker 工作树运行定向测试；记录命令、退出码和测试名：

- [ ] `mulDivFloor` 覆盖普通值、余数向下取整、完全清空时吸收余数、零分母、`MaxInt64` 中间乘积仍可表示、最终结果溢出。
- [ ] 比例热路径使用 `math/bits` 或等价固定宽度整数实现；不使用二进制浮点，不按请求分配 `big.Int`，负值在 helper 外被拒绝。
- [ ] 严格十进制解析覆盖 `0`、六位小数、前导/尾随零、负数、七位小数、非法文本、`MaxInt64` 边界和溢出。
- [ ] 有价套餐创建与更新缺少精确字段时整笔拒绝；负数、精度超限、溢出、精确/兼容字段不一致分别返回稳定、可机判的错误码。
- [ ] 创建、更新、读取响应把 `price_amount_micros` 序列化为十进制字符串，能够无损往返超过 JavaScript 安全整数范围的合法值。
- [ ] 无价套餐与全局零价 Credit 容器的零值语义明确，不能把历史 `NULL` 与前向显式 `"0"` 混同。
- [ ] 拒绝路径无部分数据库写入；测试在失败前后比较套餐记录或事务状态。
- [ ] 运行实现所影响 Go 包的定向测试，至少覆盖 `./model`、`./controller` 中新增合同；不得只运行单个 happy-path 测试。

## Gate D：Credit 币种与附加 schema

- [ ] 全局 `credit_balance` 套餐首次配置只接受大写 `CNY` 或 `USD`；不支持币种及空值返回稳定错误。
- [ ] 只要存在任一 Credit 权益、估值状态或估值 ledger，普通 API 修改币种会与套餐更新一起原子回滚。
- [ ] 未存在上述数据时，首次配置与允许的同值更新能正常往返；测试明确区分“首次设置”“同值更新”“被冻结后改值”。
- [ ] `CreditValuationState` 保证每份 Credit 权益唯一状态；`TimedSubscriptionValuationGrant` 的幂等键及 `(source_type, source_key)` 使用稳定命名唯一索引。
- [ ] migration marker、请求扣除/恢复快照、低频 ledger、转换、权益与 FX 快照字段使用正确的 `BIGINT`/字符串/时间/可空语义。
- [ ] SQLite `AutoMigrate` 在空库和含历史套餐行的数据库上可重复执行；第二次执行无破坏，历史精确价格仍为 `NULL`，且不会自动创建 marker 或估值状态。
- [ ] SQLite 实际数据库证明重复状态/grant 被数据库唯一约束拒绝；不能只检查 GORM tag。
- [ ] MySQL/PostgreSQL 方言的类型、索引名、保留字和可空唯一语义有可执行测试或 SQL 证据。若本机没有 `TEST_MYSQL_DSN`/`TEST_POSTGRES_DSN`，必须明确记录未运行范围，不能把 DryRun 写成真实数据库 PASS；真实三库零 SKIP 总门禁仍由 #27 承担。

## Gate E：只读历史诊断

- [ ] 诊断直接读取数据库原始 DECIMAL/SQLite 数值表示，不先扫描到二进制浮点。
- [ ] 合法、负数、精度超限、溢出、无法精确恢复和往返不一致均产生稳定 reason，并按稳定套餐 ID 排序。
- [ ] 相同数据库快照重复运行输出确定；不包含影响排序或比较的运行时随机值。
- [ ] 测试在诊断前后比较套餐精确价格、migration marker、估值状态和相关表，证明零写入。
- [ ] 诊断只报告事实，不返回或写入 `ready` 决策；非法历史行阻止 `ready` 的验收完整留给 #27。

## Gate F：管理员 UI、i18n 与真实浏览器

- [ ] 表单始终保留用户原始十进制字符串，并由字符串运算生成 micros；代码中没有从 JavaScript `Number`/浮点展示值反推权威 micros。
- [ ] 组件/库测试覆盖创建、编辑、刷新、边界值和后端稳定错误码呈现；请求 payload 中 `price_amount_micros` 是 JSON 字符串。
- [ ] 所有新增或修改的用户可见文字均通过 `t(...)`，en、zh、fr、ru、ja、vi 六个 locale 无 missing/extras。
- [ ] 从 `web/default` 运行相关定向测试、`bun run typecheck` 和 `bun run build`；只格式化明确修改文件，禁止全仓无关重排。
- [ ] 使用监督式开发服务和真实浏览器完成管理员创建→读取→编辑→刷新 smoke；记录可见输入值、实际请求 payload、响应精确字段及刷新后的值。组件测试不能替代此项。
- [ ] 浏览器 smoke 同时确认现有购买入口、disabled plan 显示/行为和模型范围忽略未出现可见回归；结束后关闭浏览器 tab 并停止本轮服务。

## Gate G：集成前回归与证据判定

- [ ] Worker 声明的每条测试命令均可由协调器在同一 HEAD 重现；无法重现的证据标记失败，不凭摘要放行。
- [ ] Go 定向包测试、前端定向测试、typecheck/build、SQLite migration smoke 和浏览器 smoke 全部通过。
- [ ] `git diff --check`、工作树清洁度和进度文件最终状态通过。
- [ ] Issue #20 十条 acceptance criteria 逐条映射到测试或实测证据；每项标注 PASS、FAIL 或真实未覆盖，禁止把推断标成 PASS。
- [ ] 真实 MySQL/PostgreSQL 未运行时只记录为 #27 的明确剩余门禁；若 #20 实现已经提供外部 DSN 测试，则记录实际版本、命令及是否存在 SKIP。
- [ ] 没有未决 blocker、失败测试、未解释告警或会改变 #21/#22 消费合同的模糊点。

## 安全集成操作

1. 在集成工作树确认分支为 `jiwangyihao/credit-operational-value-integration` 且工作树干净；若出现非本步骤改动，停止并识别来源。
2. 记录 Worker HEAD、派发基线和 `git merge-base`；确认 Worker 是从 `c1fa6845e6f3707095bbb857b2c16be3b5068ab1` 派生，且完整提交链可达。
3. 在 Worker 工作树完成 Gate A–G。验收失败时通过当前 Orca Dispatch 发精确修复要求，并让原 Worker 在原工作树继续；不能先合并再修。
4. 验收通过后，在集成分支使用非 fast-forward merge 集成 `jiwangyihao/issue-20-valuation-foundation`，提交信息使用 `feat(valuation): 集成精确价格与附加式结构`。不要 cherry-pick 部分提交，以免遗漏 RED/GREEN、恢复记录或调用点。
5. 合并后立即在集成工作树重跑受影响 Go 定向包、前端定向测试/typecheck/build、SQLite migration smoke 与 `git diff --check`。合并冲突必须按完整合同解决，不能任选一侧。
6. 合并后审阅 #21/#22 指令与实际暴露接口是否一致；如符号发生变化，只更新 Agent 指令/共享合同，不扩大 #20 代码范围。
7. 所有集成证据持久化后才关闭 GitHub Issue #20，并将对应 TODO 标记完成；随后才能并行派发 #21/#22。
8. Issue 关闭和集成提交确认后，使用 Orca 原生命令停止/释放本 Run 创建的 #20 Worker，并仅回收 `issue-20-valuation-foundation` 工作树；不得回收集成树、主树、`account` 或 `disk`。

## 不放行条件

出现下列任一情况，保持 #20 OPEN 并回到原 Worker：

- 历史 `price_amount_micros` 被自动回填或非法历史行直接改变 `ready`；
- marker 状态被创建/更新，或 Credit 数量写已启用强制估值双写；
- API 以 JSON number 传递 micros，或估值从兼容浮点价格反推；
- 错误只能通过文本解析判断，或拒绝路径留下部分写入；
- UI 没有真实浏览器证据，六语言缺失，或构建失败；
- 工作树不干净、关键成果未提交、证据与 HEAD 不一致；
- 把 DryRun、mock 或方言编译检查表述为真实 MySQL/PostgreSQL PASS。
