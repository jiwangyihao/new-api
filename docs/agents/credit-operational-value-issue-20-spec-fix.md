# Issue #20 Spec H1/M1 收敛修复 Agent 指令

## 任务目标

在既有隔离工作树 `C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-20-valuation-foundation`、冻结候选 HEAD `79982d773d127779c9c3835c2e1c771b7a829268` 上，只修复 Spec 复评留下的两项缺口：

1. H1：前向显式提交 `price_amount_micros: "0"` 时，必须持久化为非 NULL 的精确零值，并与历史待迁移 `NULL` 明确区分。
2. M1：只读历史价格诊断必须用数据库原始十进制/SQLite 数值表示做不经 Go `float32/float64` 的数值往返校验，并对往返不一致返回稳定 reason `roundtrip_mismatch`，同时严格保持零写入。

这不是重做 Issue #20，也不是实现 #27。禁止历史回填、migration marker 写入、`ready` 裁决、Credit 数量/估值强制双写以及 #21–#26 的业务路径。不得修改主工作树或集成父树，不得 reset/rebase，不得关闭 GitHub Issue，不得部署。

## 必读材料

开始前必须读取：

1. 仓库 `AGENTS.md` 与自动注入的全局规则。
2. `gh issue view 19 --repo jiwangyihao/new-api --comments`。
3. `gh issue view 20 --repo jiwangyihao/new-api --comments`。
4. `gh issue view 27 --repo jiwangyihao/new-api --comments`，确认历史回填和 `ready` 所有权仍只属于 #27。
5. 当前工作树中的 `CONTEXT.md`、`docs/adr/0002-credit-operational-remaining-value.md`、新规格和实施计划。
6. `docs/agents/credit-operational-value-execution.md`、`docs/agents/credit-operational-value-issue-20.md` 与现有 `.scratch/agent-progress/issue-20/` 证据。
7. 集成父树验收清单：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration/docs/agents/credit-operational-value-issue-20-acceptance.md`。
8. Spec 复评完整报告：`C:/Users/34404/AppData/Local/Temp/new-api-issue20-spec-rereview.md`。

## 必须使用的技能与方法

- 先读 `skill://diagnosing-bugs`，分别建立 H1、M1 的“可观察复现 → 根因 → 最小源头修复 → 反证”链。
- 使用 `skill://tdd`：每项先落能在冻结 HEAD 上失败的行为测试，记录 RED 原始失败，再实施 GREEN。测试保护 API/数据库可观察合同，禁止源码文本断言。
- M1 涉及跨方言数据库表示与诊断边界，先读 `skill://codebase-design`；让一个明确的诊断模块拥有“读取原始表示、严格解析、往返比较、稳定 reason”不变量，不得把 #27 迁移 writer 偷渡进来。
- 如需多轮调查，使用 checkpoint/rewind；不要靠一段大脚本承载唯一成果。
- 本任务原则上不改 UI、不新增文案；若事实证明必须改用户可见内容，先通过 Orca escalation 说明原因，未经协调器确认不要扩展。

## 中断恢复协议

立即创建或更新并持续落盘：

- `.scratch/agent-progress/issue-20/spec-fix-status.md`：当前 H1/M1、RED/GREEN、最近安全提交、下一条命令、阻塞。
- `.scratch/agent-progress/issue-20/spec-fix-evidence.md`：失败复现、命令、关键输出、修复后证据。
- `.scratch/agent-progress/issue-20/spec-fix-contract.md`：零值/NULL 语义、各方言诊断读取与往返算法、稳定 reason、明确非所有权。

每完成一项即更新进度并创建小而可恢复的 Conventional Commit；提交 subject 使用简体中文。工具或模型中断时先保存现场，在原终端恢复，不自行创建替代 Agent。

## H1：显式精确零值必须保真

冻结实现的 `NormalizeSubscriptionPlanPrice` 在 `AmountMicrosProvided=true` 且解析结果为 0 时返回空 `SubscriptionPlanPrice{}`，从而把显式 `"0"` 折叠为 `nil`。必须从“字段是否提供”而非数值真假判断可空性。

先补 RED，至少覆盖：

1. model 归一化：`DisplayAmountProvided=true, DisplayAmount="0", AmountMicrosProvided=true, AmountMicros="0"` 返回 `AmountMicros != nil` 且 `*AmountMicros == 0`；兼容展示为 0。
2. controller/API 创建零价非 Credit 套餐或仓库合同允许的零价套餐时，数据库 `price_amount_micros` 为非 NULL 0，读取 JSON 为十进制字符串 `"0"`，不是 `null`。
3. 更新路径显式提交 `"0"` 后同样非 NULL；字段完全缺失的历史/无关编辑继续保留 NULL，不被默认零污染。
4. 拒绝路径和既有非零、边界、历史 NULL 测试继续通过。

最小修复应只调整精确字段存在性语义及必要调用测试；不得把所有历史 NULL 自动写零，不得把 `0` 用作 migration 完成标志。

## M1：只读诊断必须识别往返不一致

冻结实现只 `CAST(price_amount AS TEXT)` 后解析最多六位小数，未证明由该文本得到的 micros 再转回数据库数值时仍与原值数值相等。先调查 SQLite NUMERIC/REAL、MySQL DECIMAL、PostgreSQL NUMERIC 的实际返回语义和仓库现有跨库模式，再选择最小、明确、可测试的方言方案。

必须满足：

1. 应用层不得把历史价格扫描为 Go `float32/float64`，不得用容差比较、`fmt` 浮点格式化或当前套餐价格猜测。
2. 合法非负、最多六位小数且数值往返一致的行不报告；负数、超精度、溢出、非法表示保留现有稳定 reason。
3. 原始数值能够被表面文本解析，但严格 micros→十进制数值与数据库原值不相等时，报告 `roundtrip_mismatch`；reason 为稳定常量，不依赖错误文本。
4. 真实 SQLite 夹具必须在冻结实现上产生 RED，并在修复后只报告预期 plan ID/reason，排序稳定、重复运行确定。夹具需说明为何是真正的数值往返不一致，而非测试注入伪字符串。
5. 诊断前后比较 `subscription_plans.price_amount_micros`、migration marker、估值 state/ledger 或至少所有已存在相关表的行数/内容，证明零写入；不得创建 marker，不得更新历史行，不得返回 `ready` 决策。
6. MySQL 5.7/PostgreSQL 9.6 的 SQL 必须有合法分支并接受现有可用的定向语义检查；若环境未提供真实 DSN，明确记录 SKIP，绝不宣称实测 PASS。真实三数据库迁移矩阵仍属于 #27。

若调查证明 SQLite 在现有列类型和驱动下无法构造合法的 `roundtrip_mismatch` 数据，而不依赖伪造 query/字符串，立即发送 escalation，附最小实验和数据库事实；不要编造测试或把 `invalid_decimal` 改名冒充完成。

## 修改边界

允许修改 H1/M1 根因直接相关的 `model`、`controller` 测试/实现及进度证据。禁止：

- 修改 `CLAUDE.md`、父/集成工作树或不相关用户文件。
- 历史价格回填、批次 apply/verify、marker 状态变化、非法行阻止 `ready`、repair/suspend CLI。
- Credit moving average、timed grant、请求结算、兑换、恢复、FX、转换或分析实现。
- 浮点容差、默认零、文本错误码解析、测试专用生产分支、吞错或削弱既有测试。
- 重跑项目级全量套件、全量 lint 或无关格式化；协调器会统一验收。

## 最小验证要求

1. H1 的 model + controller/API RED→GREEN，显式 0、更新 0、历史 NULL 三类均有断言。
2. M1 的真实 SQLite RED→GREEN，包含 `roundtrip_mismatch`、稳定排序/重复执行和诊断前后零写入。
3. 受影响 `model`/`controller` 定向测试与必要包级 `go test ... -count=1`。
4. 若未改前端，只运行已有 plan-form 定向测试作为回归即可；无需重复浏览器 smoke。若行为意外改变真实 UI 合同，先 escalation。
5. `git diff --check`；所有实现与进度均提交，最终工作树 clean；无服务、临时数据库或未跟踪产物。

## 完成交付

完成前逐项更新三份 spec-fix 进度文件，列出两个 RED 的失败表现、根因、提交 SHA、逐条命令与结果、真实 SQLite 证据、外部数据库实际 SKIP/PASS 范围、残余风险，并再次声明未实现 #27 历史写入/marker/ready。确认 HEAD 与工作树 clean 后，使用当前 Dispatch 注入的 capability 仅发送一次 `worker_done`；只有 H1/M1 均完成才能 outcome=succeeded。存在真实 blocker 时发送 escalation 并保留现场，不得虚报成功。