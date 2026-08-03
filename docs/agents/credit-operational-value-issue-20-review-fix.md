# Issue #20 评审阻断修复 Agent 指令

## 任务目标

在既有隔离工作树 `C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-20-valuation-foundation`、当前已验收候选 HEAD `9e3329d0f4b509d1179c895c52f01af7a19f0ca4` 上，复现并修复 Standards 评审发现的三项问题。该工作树是 Issue #20 的唯一实现现场；不得新建替代工作树、不得 reset/rebase、不得修改主工作树或集成父树。

本任务不是重新实现 #20，也不是趁机实现 #21–#28。最终必须保留 #20 的既定边界：只负责前向精确价格转换、附加式 schema、Credit 估值币种配置与只读非法值诊断；不回填历史套餐价格，不修改 migration marker，不决定或阻止 `ready`，不启用 Credit 数量/估值强制双写。

## 开始前必读

1. `C:/Users/34404/source/repos/new-api/AGENTS.md` 与自动注入的全局规则。
2. 父 PRD：`gh issue view 19 --repo jiwangyihao/new-api --comments`。
3. 当前 Issue：`gh issue view 20 --repo jiwangyihao/new-api --comments`。
4. 当前工作树内：
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
   - `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md`
   - `docs/agents/credit-operational-value-execution.md`
   - `docs/agents/credit-operational-value-issue-20.md`
   - `.scratch/agent-progress/issue-20/status.md`
   - `.scratch/agent-progress/issue-20/evidence.md`
   - `.scratch/agent-progress/issue-20/contract.md`
5. 集成父树中的验收清单：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration/docs/agents/credit-operational-value-issue-20-acceptance.md`。
6. Standards 完整报告：`C:/Users/34404/AppData/Local/Temp/new-api-issue20-standards-review-progress.md`。

## 必须使用的技能与方法

- 修改前先读 `skill://diagnosing-bugs`，对每项 finding 建立“复现 → 根因 → 最小源头修复 → 反证”的诊断链；不要直接压掉错误或加特殊分支。
- 使用 `skill://tdd`：每项先提交或至少持久化能够失败的行为测试，再完成 GREEN；测试必须保护可观察合同，而非源码文本。
- 第三项涉及 controller/model 边界和线性化接缝，先读 `skill://codebase-design`，把并发不变量放进深模块，不要继续让 controller 独占业务规则。
- 修改 `web/default` 时读 `skill://shadcn-ui`；若新增或调整用户可见文字，再读 `skill://i18n-translate` 并覆盖 en、zh、fr、ru、ja、vi。
- 遇到需要多轮探索的部分使用 checkpoint/rewind；不要把大段一次性脚本作为主要实现手段。

## 中断恢复协议

开始后立即创建或更新以下文件，并尽快形成一个小提交：

- `.scratch/agent-progress/issue-20/review-fix-status.md`：当前 finding、RED/GREEN 状态、最近安全提交、下一条命令、阻塞。
- `.scratch/agent-progress/issue-20/review-fix-evidence.md`：失败复现、测试命令、关键输出、修复后证据。
- `.scratch/agent-progress/issue-20/review-fix-contract.md`：最终选择的模型/API/并发不变量及明确非所有权。

每完成一个 finding 就更新这些文件并提交一个小而可恢复的 Conventional Commit；subject 使用简体中文。发生工具/模型异常时先保留工作树与进度文件，再在原终端恢复；不要自行创建新 Agent。

## Finding 1：禁止从 JavaScript Number 伪造历史 micros（阻断）

评审证据位于 `web/default/src/features/subscriptions/lib/plan-form.ts`：当 API 返回 `price_amount_micros == null` 时，当前实现会把旧 `price_amount` 的 JavaScript `number` 转成十进制文本，保存时再生成貌似权威的 micros。无关编辑因此可能替 #27 回填历史价格并永久固化已经损失的精度。

必须先写 RED 测试，至少覆盖：

1. 加载一条有价历史套餐，`price_amount_micros: null`；只编辑名称、状态或其他非价格字段后，提交 payload 不得出现由 `price_amount` Number 推导出的 micros，数据库精确列仍为 NULL。
2. 表单可以显示兼容旧价格，但显示值不能被误标为原始精确文本，也不能在未发生明确价格输入时成为权威来源。
3. 用户明确键入新的原始十进制价格时，仍按严格字符串规则生成 micros，六位小数无漂移；创建的新有价套餐仍要求精确 micros。
4. `0`、大整数边界和易受二进制浮点影响的十进制值均不得走 Number 反推路径。

优先选择显式保存“精确值是否存在/用户是否改过价格”的表单状态。后端若需区分历史无关更新与前向价格写入，应以清晰 DTO/字段存在性和持久化原值判断实现；不得以 `toFixed`、字符串化 Number、容差比较或默认零伪造精度。公开套餐 DTO 的 nullable 精确字段必须继续存在。历史批量回填仍完全属于 #27。

## Finding 2：关键 schema 变化必须 fail-closed（高）

评审指出 `model/main.go` 把旧 `price_amount` 目标扩大为 `decimal(19,6)`，但元数据查询或 `ALTER TABLE` 失败只记录 warning，服务仍可能在不兼容 schema 上启动。

先证明真实根因，再选最小方案：

- 若权威 `price_amount_micros` 已使旧兼容列无需扩宽，优先删除这项非必要、风险更高的旧列 ALTER，并用测试证明 #20 的精确范围和旧展示/支付合同仍成立。
- 若扩宽确属 #20 前向合同不可缺少，则迁移函数必须返回并传播错误，使 `migrateDB` 在元数据查询或 ALTER 失败时 fail-closed；SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6 均要走合法分支。

测试至少覆盖失败传播或“不再需要 ALTER”的可观察结果，以及历史 `price_amount_micros` 仍保持 NULL。不得把 warning 文案或源码字符串测试当成验收；不得在此任务回填历史行或切换 migration marker。

## Finding 3：币种冻结与首个 Credit 权益必须共享线性化接缝（高）

当前币种冻结检查位于 `controller/subscription.go`，更新路径锁套餐行后计数；Credit grant 路径在 `model/credit_balance.go` 只锁用户/权益，并可能消费未锁定的 `TargetPlanSnapshot`。并发“修改 valuation_currency”与“创建首个 Credit 权益”可能同时成功，违反币种一旦存在权益即冻结的不变量。

先画出所有现存 Credit grant/购买/兑换/管理员入口及锁顺序，再把规则移动到 model/service 深模块：

- 普通套餐更新和每个现存 Credit allocation/grant 路径必须在同一数据库事务中经过同一个计划级线性化 guard。
- guard 必须重新读取并锁定权威全局 Credit plan，而不是信任未锁快照；所有路径采用一致锁顺序，避免新死锁。
- 合法串行结果只有两类：币种修改先提交，随后首个权益使用新币种；或首个权益先提交，随后币种修改被稳定错误拒绝。不得出现“权益已经存在且普通接口仍成功改币种”。
- 保持 disabled-plan 边界：既有 disabled-plan 权益仍可消费，但新的 purchase/redemption/conversion/admin grant 仍拒绝；不要借本修复改变该语义。
- #20 只建立并使用线性化 guard；不得创建 #22 的 CreditValuation 数量/成本状态，不得启用强双写。

必须增加行为级并发/事务测试。SQLite 锁语义不足以证明所有行锁细节时，要明确记录证据边界；可以用现有事务接缝证明两个业务入口都经过同一 guard，但禁止源码文本测试。没有 MySQL/PostgreSQL DSN 时必须报告 SKIP，不能宣称三库实测通过。

## 修改边界

允许修改与三项根因直接相关的 model/controller/DTO/form/test 文件及进度证据。禁止：

- 修改 `CLAUDE.md`、父/集成工作树或不相关用户文件。
- 实现历史价格回填、历史 Credit/timed 重建、marker 状态变化、`ready` 阻断或 repair CLI（#27）。
- 实现 Credit moving-average 成本、购买/请求结算/兑换/恢复/FX/转换（#21–#26）。
- 通过兼容 shim、默认值、浮点容差或吞掉迁移错误掩盖问题。
- 删除或弱化已有测试以获得通过。

## 验证要求

按增量执行，避免每个小步运行全项目套件；本 Agent 跳过项目级全量测试、全量 lint 和无关格式化，协调器在集成后统一运行。至少完成：

1. 三项 finding 各自的 RED→GREEN 行为测试。
2. `go test` 覆盖受影响 model/controller/router 包及相关定向用例，`-count=1`。
3. 若修改并发接缝，运行该并发用例多次并在可行处运行相关 `-race` 窄范围。
4. 前端相关定向测试与 `bun run typecheck`；仅格式化显式改动文件。
5. 若前端行为变化影响既有真实浏览器路径，复验 legacy NULL 无关编辑不会生成 micros，以及显式原始十进制编辑仍精确往返。若浏览器工具受阻，保留隔离服务/数据并发送 escalation，绝不把 API 或组件测试冒充浏览器通过。
6. `git diff --check`，工作树最终 clean；不得保留服务、临时数据库、浏览器 tab 或未跟踪构建产物。

## 完成交付

最终更新三个 review-fix 进度文件，列出：根因、修复提交、逐项测试命令与结果、浏览器结果或明确未完成项、外部 DB SKIP、残余风险。确认所有改动均已提交且工作树 clean。然后使用当前 Dispatch 注入的 capability 发送一次 `worker_done`，outcome 只能在三项均完成时为 `succeeded`；如仍有真实 blocker，发送 escalation 并保持现场，不得虚报成功。发送完成事件后立即停止工作。
