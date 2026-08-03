# Issue #20 H1/M1 最终 Spec 短复评 Agent 指令

## 任务目标

你是 Issue #20 的最终、只读、严格收敛 Spec 复评 Agent。冻结候选工作树为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-20-valuation-foundation`

冻结 HEAD 必须严格等于：

`e0bb98e043d982de407759533855af443d83c52b`

只复评上一轮 Spec 报告留下并已修复的两项：

1. **H1：显式 `price_amount_micros: "0"` 保真。** 字段被明确提供时，数据库必须保存非 NULL 的整数 0，对外 API 必须返回十进制字符串 `"0"`；只有字段完全缺失时，历史待迁移值才保持 NULL。
2. **M1：SQLite 只读历史价格诊断。** 对表面文本可严格解析、但规范 micros 十进制与数据库原始数值严格往返不一致的真实 SQLite 行，必须返回稳定 reason `roundtrip_mismatch`；结果顺序和重复调用必须确定，且诊断前后数据库必须零写入。

本任务不重新评审已经通过的其他八项，不重新做 Standards 评审，不修改代码，不运行项目级大套件，不合并分支，不关闭 Issue，不部署，也不派生任何子 Agent。目标是快速、证据化地给出 H1/M1 的 PASS 或 FAIL，并通过当前 Dispatch 发送一次 `worker_done`。

## 必读材料

开始后按顺序读取：

1. 仓库 `AGENTS.md` 与自动注入规则。
2. `skill://review`，但仅执行其中的 **Spec** 轴；Standards 轴已有最终 PASS，不得重做。
3. `gh issue view 19 --repo jiwangyihao/new-api --comments`。
4. `gh issue view 20 --repo jiwangyihao/new-api --comments`。
5. `gh issue view 27 --repo jiwangyihao/new-api --comments`，确认历史回填、marker 和 `ready` 仍只属于 #27。
6. 当前工作树的 `CONTEXT.md`、`docs/adr/0002-credit-operational-remaining-value.md`、`docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`。
7. 执行协议 `docs/agents/credit-operational-value-execution.md`。
8. 集成父树验收清单：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration/docs/agents/credit-operational-value-issue-20-acceptance.md`。
9. 上一轮 Spec 报告：`C:/Users/34404/AppData/Local/Temp/new-api-issue20-spec-rereview.md`。
10. 修复交付证据：`.scratch/agent-progress/issue-20/spec-fix-status.md`、`spec-fix-evidence.md`、`spec-fix-contract.md`。

复评范围固定为 `79982d773d127779c9c3835c2e1c771b7a829268..e0bb98e043d982de407759533855af443d83c52b` 中 H1/M1 相关变更及其最终状态。先确认 `git status --short` 为空、HEAD 精确匹配；不满足时立即 escalation，不得继续在漂移现场给结论。

## 必须检查的 H1 合同

通过源码、真实测试断言和已有 RED/GREEN 证据逐条确认：

1. 字段存在性由 `AmountMicrosProvided` 或等价的显式存在性信号决定，不能再由 `exactMicros == 0`、truthiness 或默认值决定。
2. model 归一化对显式显示金额 `"0"` 和精确 micros `"0"` 产生非 nil 的零值。
3. controller/API 创建路径把显式零持久化为 SQL 非 NULL 0，读取 JSON 是字符串 `"0"`。
4. controller/API 更新路径显式提交零时同样保存非 NULL 0。
5. 完全缺失价格字段的历史行或无关编辑继续保留 NULL，不被默认零污染。
6. H1 修复未把零当作迁移完成标志，未回填历史行，未弱化负数、无效格式、超精度、溢出或双字段不一致拒绝。

允许为消除不确定性只运行 H1 已有的精确定向测试；禁止重复包级全量、前端 build、浏览器、i18n 或项目全量测试。若现有证据和源码足够，应直接引用，不为“看起来更完整”重复耗时门禁。

## 必须检查的 M1 合同

逐条确认：

1. 夹具使用真实 SQLite `NUMERIC/REAL` 数值行为，不是注入伪字符串、mock query 或测试专用生产分支。
2. 原值的表面文本可被严格 micros 解析，但规范六位 micros 十进制经 SQLite 数值比较与原值不等，因此确为数值往返不一致。
3. Go 应用层没有把历史值扫描或转换为 `float32/float64`，没有容差比较或浮点格式化。
4. 往返不一致返回稳定常量 `roundtrip_mismatch`，而不是复用 `invalid_decimal` 或依赖错误文本。
5. 结果按稳定 plan ID 排序；同一快照重复诊断得到完全相同结果。
6. 测试通过完整相关行快照及 SQLite `total_changes()`（或同等强证据）证明诊断零写入。
7. 非 SQLite 现有查询/错误语义未被意外破坏；没有声称真实 MySQL/PostgreSQL PASS。
8. 没有新增历史回填、migration marker、`ready` 判断、修复写入或 #21–#26 业务行为。

允许只运行 M1 的已有真实 SQLite 定向测试一次，必要时用 `-count=10`确认确定性；禁止扩展成 #27 的三数据库迁移验收。

## 报告与结论格式

立即创建并增量更新：

`C:/Users/34404/AppData/Local/Temp/new-api-issue20-h1-m1-spec-final-review.md`

报告必须包含：

- `status: started|completed`；
- 冻结 HEAD、工作树起止 clean 状态；
- H1 六条逐项 `PASS|FAIL` 与文件/测试证据；
- M1 八条逐项 `PASS|FAIL` 与文件/测试证据；
- `#27 非侵占` 明确结论；
- Findings 按 Blocker/High/Medium/Low 分组；无 finding 时每组写 `None`；
- 最终 `PASS` 或 `FAIL`；
- 实际运行的命令及精确结果，未运行的门禁必须明确说明复用既有证据；
- MySQL/PostgreSQL 未提供 DSN 时明确写未实测，不得称 PASS。

任何 FAIL 必须给出具体文件/符号、可观察影响和最小修复，不得重新提出已经由最终 Standards 复评关闭的旧 finding，除非新 HEAD 有确凿回归证据。

## 禁止事项

- 禁止编辑、格式化、暂存或提交工作树中的任何文件。
- 禁止创建子 Agent、Advisor 或额外评审任务。
- 禁止重新运行项目全量 Go、前端 build、浏览器 smoke 或六语言同步。
- 禁止扩大到 Issue #20 其他八项、Standards、#21–#28 实现。
- 禁止把 SQLite 定向证据描述成三数据库实测。
- 禁止因运行时间、模型或工具短暂异常自行重启/重派；先持久化报告并向协调器 escalation。

## 完成交付

报告落盘且状态为 `completed` 后，再次确认 HEAD 仍为 `e0bb98e043d982de407759533855af443d83c52b`、`git status --short` 仍无输出。然后使用当前 Dispatch 注入的 capability 发送且只发送一次 `worker_done`：

- H1/M1 均通过且无新 blocker/high/medium 时：`outcome=succeeded`，正文给出报告路径、最终 PASS、实际定向命令和零代码改动。
- 任一项失败时：`outcome=failed`，正文列出 finding 与报告路径；不要修改代码。

不要等待协调器再次催促；本任务是短复评，形成证据化结论后立即结束。
