# Issue #20 实现 Agent 指令

## 目标与交付边界

你负责父 PRD #19 的第一个垂直切片 GitHub Issue #20「精确套餐标价与附加式估值门禁往返」。必须在你的 Orca 子工作树内完成可合并、可验证、工作树干净的永久实现；不要只写方案、脚手架或 TODO。该切片要建立后续 #21–#28 依赖的精确金额与附加式存储合同，并让管理员通过现有 UI/API 完成一次真实、可观察的套餐精确价格与 Credit 估值币种往返。

严格限制所有权：#20 **不回填任何历史套餐价格，不重建历史 Credit/计时估值，不修改 migration marker 状态，不自行阻止 `ready`，不启用 Credit 数量/估值强制双写**。你可以实现只读非法价格诊断，但它只能按稳定套餐 ID 返回确定性 reason，不能写 `price_amount_micros`，不能迁移历史数据，不能切换或裁决 `ready`。历史回填、非法历史行阻止 `ready`、历史重建及门禁切换全部属于 #27。发现下游需求时只在进度合同中记录，不得提前实现。

## 必读材料与 Skill

开始改代码前必须依次阅读并服从：

1. 仓库与全局 `AGENTS.md` 规则。
2. `issue://jiwangyihao/new-api/19` 和 `issue://jiwangyihao/new-api/20`；使用 GitHub CLI 时始终显式传 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`，这是本 Run 的共享恢复与业务协议。
4. `CONTEXT.md`、`docs/adr/0001-credit-balance-entitlement.md`、`docs/adr/0002-credit-operational-remaining-value.md`。
5. `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md` 和 `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md` 中精确价格、附加 schema、币种与算术基础部分。

这是永久 feature，必须先读并采用 `skill://tdd`：先写能证明可观察合同、且会因合理实现缺陷失败的测试，再最小实现，再重构。涉及 `web/default` 时必须先读 `skill://shadcn-ui`；新增或修改任何用户可见文字时必须读 `skill://i18n-translate` 并维护 en、zh、fr、ru、ja、vi。若数据库、迁移或并发失败不能从错误直接定位，使用 `skill://diagnosing-bugs` 先复现、定位根因再修改。深模块接口不明确时读 `skill://codebase-design`，但不得改写 ADR/spec 已冻结的决定。触及 tiered/dynamic billing expression 才额外读取 `pkg/billingexpr/expr.md`。

## 必须实现的端到端合同

- 有价套餐创建/更新接受权威整数 micros 字符串（`1 unit = 1,000,000 micros`），读取同时返回精确字段和现有兼容展示金额。明确字段类型、空值及无价套餐行为；避免 JSON number 精度损失。
- 前向请求中的负数、超过六位小数、溢出、缺少精确价格、精确与兼容字段不一致必须在同一事务边界前原子拒绝，并返回稳定、可由调用方判断的错误码；不能靠解析错误文本。
- 管理 UI 必须从用户原始十进制输入构造权威 micros 请求；创建、编辑、刷新往返无二进制浮点漂移。继续保持现有套餐购买、disabled plan、模型范围忽略及旧展示兼容。
- 全局 Credit 余额套餐首次配置时明确选择 `CNY` 或 `USD` 估值币种。只要存在 Credit 权益、估值状态或估值账本，普通 API 不得改写币种，拒绝必须原子且错误稳定。不能允许其他币种。
- 以附加式、跨数据库兼容的方式注册后续切片需要的精确价格列、每份 Credit 权益唯一估值状态、版本化 migration marker、不可变计时估值 grant、请求级扣除/恢复快照、低频来源/FX 快照。只建立字段、模型、约束和未来写入接口所需的稳定类型，不填充虚假数据、不启用历史迁移和强双写。
- 唯一约束与索引必须使用稳定命名，并在 SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6 语义一致。优先 GORM 抽象；任何原生 SQL 必须具有三方兼容分支。不得用 GORM DryRun 冒充真实数据库验收。
- 历史套餐行新增列后保持待迁移。只读非法值诊断从数据库原始 DECIMAL/SQLite 数值表示严格判定能否精确转 micros，输出稳定套餐身份和 reason；不写库、不切 marker、不阻止 `ready`。不要把当前套餐正价、最近订单或 `(user_id, plan_id)` 关联重新引入估值。
- 实现无二进制浮点的防溢出整数 `floor(a × b / d)` 基础能力，覆盖普通值、向下取整、完全清空时吸收余数、分母为零、`MaxInt64` 中间乘积与结果溢出。热路径不得按请求分配大整数；设计必须说明编译后成本和溢出策略。
- 所有新增结构和 API 命名须足够稳定，供 #21–#27 直接消费。不要为“未来可能”引入第二套并行抽象；让一个深模块拥有金额解析、比例运算与估值结构不变量。

## 实施方式与恢复协议

第一项实际改动必须创建并随后持续更新：

- `.scratch/agent-progress/issue-20/status.md`：当前阶段、已完成、下一步、阻塞、最近安全提交。
- `.scratch/agent-progress/issue-20/evidence.md`：每个失败测试、实现后通过结果、命令、精确输出摘要和修复依据。
- `.scratch/agent-progress/issue-20/contract.md`：最终 schema、类型、API 字段、错误码、UI 往返及给 #21/#22/#27 的消费接口；明确 #20 的非所有权。

这些文件是崩溃恢复入口。每完成一个可编译、可验证的小步就立即落盘并创建 Conventional Commit，格式为英文 type/scope + 简体中文 subject。不要把大段一次性脚本作为唯一成果，不要把未提交关键实现只留在终端历史。遇到不确定的跨切片合同，用 Orca `orchestration ask` 阻塞询问协调器，同时继续完成不依赖答案的部分；不要自行扩大范围。

## 验证与完成条件

按 TDD 运行精确、定向的 Go/前端测试；新增测试必须保护可观察合同并能因合理缺陷失败。至少证明：金额解析/序列化与错误码、管理员创建编辑刷新往返、币种冻结条件、只读诊断无写入、附加 migration/schema 可重复、三种方言约束语义，以及整数比例边界。UI 改动必须启动应用并用真实浏览器驱动关键路径，记录观察到的请求 payload 和刷新结果；组件测试不能代替该 smoke。本 worker 不运行项目级全量测试、全仓格式化或最终生产部署，这些由协调器在波次集成后统一执行；只格式化明确修改的文件并运行 `git diff --check`。

完成前从用户视角复查 Issue #20 每条 acceptance criterion，确保所有改动已提交且工作树干净。然后通过当前 Orca Dispatch **仅发送一次** `worker_done`，正文包含：提交 SHA 列表、修改文件/领域合同、定向测试与浏览器证据、三数据库证据的实际范围、遗留风险、进度目录路径，以及明确声明未实现历史回填/marker 切换/ready 阻断/强制双写。不要关闭 GitHub Issue，不要部署生产，不要合并或回收父/自己的工作树；等待协调器验收。
