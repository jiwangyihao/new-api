# Issue #21 Standards 最终短复评指令

## 冻结现场

你是 GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」的只读 Standards 复评 Agent。只读工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

开始和结束时必须确认：

- `HEAD` 严格等于 `763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`；
- `git status --short` 为空；
- 已含 #22 的集成父基线是 `2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc`；
- `git merge-base 2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc 763b0f40bdc8fb7d5c11bc69f46749fd40a8763b` 必须返回父基线；
- 复评范围严格为 `2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc...763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`。

禁止编辑、格式化、提交、stash、reset、切换分支、启动服务、写数据库、运行大套件或派生子 Agent。冻结状态漂移时立即 escalation，不得继续。

## 必读材料与方法

1. 读取 `skill://review`，仅执行 Standards 轴。
2. 阅读自动注入的项目与全局 `AGENTS.md`。
3. 阅读父 PRD `issue://jiwangyihao/new-api/19`、子 Issue `issue://jiwangyihao/new-api/21`。
4. 阅读集成父树中的：
   - `docs/agents/credit-operational-value-execution.md`
   - `docs/agents/credit-operational-value-wave-1-contract.md`
   - `docs/agents/credit-operational-value-wave-1-acceptance.md`
   - `docs/agents/credit-operational-value-issue-21.md`
   - `docs/agents/credit-operational-value-issue-21-acceptance.md`
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
5. 阅读工作树 `.scratch/agent-progress/issue-21/` 中 contract/status/evidence 与 review-fix 文件，但它们仅是证据索引，必须回到最终代码与测试核验。
6. 阅读旧 Standards 报告 `C:/Users/34404/AppData/Local/Temp/new-api-issue21-standards-final-review.md`，只复核原四项 blocker 是否从根因关闭，并检查修复是否引入新的 blocker/high；不要重新扩展为无边界审计。

## 必须复核的四项 finding

### A. 并发同源重放

核验 `GrantTimedSubscriptionTx` 与相关 guard/测试：

- 查重与写入必须位于合法锁序和同一事务内；
- 同一 `(source_type, source_key)` 同参数并发重放属于合法串行化结果，不泄漏 `SQLITE_BUSY`、唯一约束或数据库文本错误；
- 参数冲突稳定拒绝；
- 真实文件 SQLite 多连接确定性交错测试确实会让旧实现失败，修复后 `-count=10` 与窄 `-race` 通过；
- 不得通过 sleep、宽泛重试、吞错或 savepoint 复杂化掩盖根因。

### B. 权威整数 micros

核验 paid-row 与五接口聚合/排序：

- 保留 #22 的 `amount_micros` 严格十进制解析、四列表排序、稳定业务主键 tie-breaker；
- #21 timed 增量和 non-timed/Credit row 都只能经整数 micros 累加，不得把 micros 转成 `float64` 后再参与权威值；
- precision-boundary 测试能够区分 float 无法区分的数值；
- #22 的 32 CNY 结果、current_only warning、BigInt 前端路径不回归。

### C. 溢出 fail-closed

核验 timed calculator、按币种 accumulator、source 与当前/未来周期所有 micros 加法：

- `int64 +=` 不得静默溢出；
- 统一使用项目内 checked helper 或同等最小实现；
- `MaxInt64 + 1` 有真实 RED，修复后五接口返回稳定错误并保持原子/只读；
- 不得用 `big.Int` 热路径分配或浮点绕过。

### D. 稳定不可变错误

核验 `TimedSubscriptionValuationGrant` 的 update/delete hook：

- 使用包级稳定 sentinel 或稳定 code；
- 重复调用返回可被 `errors.Is` 判断的同一语义；
- 真实 SQLite update/delete 行为测试存在；
- 不得每次临时 `errors.New`，也不得解析文本决定业务分支。

## 组合与边界检查

确认最终 diff 仍遵守：

- #22 通用 CreditValuation、Credit paid row、current_only、权威 micros、BigInt/DTO 骨架优先；#21 只叠加 timed grant、`*_by_currency`、timed warning/source/UI；
- 冻结 32 CNY：recognized/exact 为 `32,000,000` micros CNY、available=800、active count=1、Credit `time_based_value=null`；
- timed CNY/USD 分币种，跨币种 singular 为 null，source 为 `timed_grant_timeline`；
- 未实现 #23 request settlement、#24 其他 ingress、#25 outflow、#26 FX/转换、#27 marker/ready/历史迁移、#28 发布；
- MySQL/PostgreSQL 未实测必须如实说明，三数据库零 SKIP 归 #27；明显方言不兼容仍应报告。

## 输出与完成

把不超过 500 字的最终报告写入：

`C:/Users/34404/AppData/Local/Temp/new-api-issue21-standards-rereview-final.md`

报告必须包含冻结 HEAD/基线、总评 `PASS` 或 `FAIL`、四项 finding 的逐项结论、新 findings（按严重度，含符号/直接证据/影响）、#22 组合 seam 结论、未实测说明。推断标 `[INFERENCE]`；无新 finding 时写“0 findings”。

完成前再次确认 HEAD 未变且工作树 clean。随后使用当前 Dispatch 注入 capability 发送恰好一次有效 `worker_done`，body 包含 PASS/FAIL、finding 数、最严重项、组合 seam 结论与报告绝对路径。不得把任务进程成功等同于评审 PASS。