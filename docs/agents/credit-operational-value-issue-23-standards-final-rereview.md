# Issue #23 Standards 最终冻结复评指令

## 目标与不可变现场

你是父 PRD GitHub #19、子 Issue #23「完成 request_id 同步与异步可逆结算」的最终 Standards 只读复评 Agent。你必须只读审查以下冻结工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-23-request-settlement`

开始和结束时都必须执行并核对：

- `HEAD` 严格等于 `8cdfd4acb78b502af4c0232460baf7df852b7b2c`；
- `git status --short` 无输出；
- 固定点与 merge-base 为 `ec1858fec89509bdec9a90a230a8496047c5becd`；
- 审查 diff 只能是 `git diff ec1858fec89509bdec9a90a230a8496047c5becd...8cdfd4acb78b502af4c0232460baf7df852b7b2c`；
- 提交列表只能来自 `git log ec1858fec89509bdec9a90a230a8496047c5becd..8cdfd4acb78b502af4c0232460baf7df852b7b2c --oneline`。

禁止编辑、格式化、提交、stash、reset、切分/切换分支、启动服务、写数据库、派生 Agent 或运行项目级大套件。冻结状态漂移必须 escalation。先前两个未落盘且被停止的 #23 评审 Task 不构成证据；你必须独立给出有效结论。

## 必读材料

1. `skill://review`，只执行 Standards 轴。
2. 自动注入的仓库与全局 `AGENTS.md`。
3. 父 PRD `issue://jiwangyihao/new-api/19`、Issue `issue://jiwangyihao/new-api/23`。
4. 集成父树下的：
   - `docs/agents/credit-operational-value-execution.md`
   - `docs/agents/credit-operational-value-wave-2-contract.md`
   - `docs/agents/credit-operational-value-issue-23.md`
   - `docs/agents/credit-operational-value-issue-23-acceptance.md`
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
5. 冻结树的 `.scratch/agent-progress/issue-23/{contract,status,evidence}.md`、`cleanup-*`、`double-count-fix-*`。这些只是索引，结论必须回到代码和测试。

## Standards 审查主题

逐项核验并报告直接证据：

1. **深模块与 clean cutover**：Credit 预扣、目标累计结算、少结算、失败退款、absorbed/unknown 恢复只能走 request-aware 深模块；Credit 不得保留匿名 delta 绕路，timed 兼容必须显式受限；导出符号迁移无旧别名和文本错误判断。
2. **事务、锁序和整数安全**：请求记录、目标权益、估值状态同事务固定锁序；所有金额为 int64 micros 和 checked arithmetic；共享 coalescer 在同一事务按稳定入队顺序逐请求处理，中间失败整批回滚且逐项错误稳定归属；不得用 sleep/宽泛 retry/吞错/float 掩盖根因。
3. **同步与异步 identity**：SubscriptionFunding/BillingSession 不重复累计兼容字段；新 Task JSON 与数据库投影持久化同一 subscription_request_id；legacy 只从持久 Task 主键生成确定性 identity；重启、成功 final、失败 refund、同 subscription 多 Task 均隔离；timed Task 不回归。
4. **cleanup 和三数据库静态兼容**：仅清理超过排他 cutoff 的 settled/refunded；保护 active、非终态、持久 Task 引用与混合版本 NULL；tasks.subscription_request_id 必须 nullable、非唯一、有命名索引、JSON/列同步且跨 SQLite/MySQL 5.7/PostgreSQL 9.6 语义合理；稳定主键 batch、幂等、失败原子回滚、SQLite 并发、只读 preview 零写入、审计事实保留成立。MySQL/PostgreSQL 未实测必须留给 #27。
5. **性能和范围**：热路径不得有无界扫描、N+1、按请求大整数分配或无必要复制；未越界实现 #24 ingress、#25 recovery、#26 FX/conversion、#27 migration/ready、#28 release；没有 UI/新文案时无需浏览器/i18n，但须有明确证据。
6. **已知修复复核**：最终 double-count 修复只能去除 BillingSession.Reserve 对 SubscriptionFunding.Settle 已拥有字段的重复累加，不能破坏 session ledger、preConsumed、RelayInfo、request target 或 timed 路径。

## 输出与完成

将不超过 650 字的报告写到：

`C:/Users/34404/AppData/Local/Temp/new-api-issue23-standards-final-rereview.md`

必须包含冻结范围、总评 `PASS`/`FAIL`、上述 1–6 逐项结论、findings（严重度、文件/符号、直接证据、影响、最小修复）、未实测范围和范围边界。推断标 `[INFERENCE]`；无 finding 明写 `0 findings`。不要把测试日志中的既有 teardown 噪声冒充通过或失败。

结束前再次确认 frozen HEAD 和 clean tree。随后使用当前 Dispatch 注入 capability 发送恰好一次有效 `worker_done`，正文包含 PASS/FAIL、finding 数、最严重项、边界结论和报告绝对路径。进程完成不等于评审 PASS。
