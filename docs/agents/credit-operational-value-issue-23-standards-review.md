# Issue #23 Standards 独立只读复评指令

## 目标与冻结现场

你是 GitHub Issue #23「完成 request_id 同步与异步可逆结算」的 Standards 只读复评 Agent。只读工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-23-request-settlement`

开始与结束时必须确认：

- `HEAD` 严格等于 `9b496ca0d46bad84b4977d63496a668388e99080`；
- `git status --short` 为空；
- 实现起始基线与 merge-base 为 `ec1858fec89509bdec9a90a230a8496047c5becd`；
- 审查范围严格为 `ec1858fec89509bdec9a90a230a8496047c5becd...9b496ca0d46bad84b4977d63496a668388e99080`。

禁止编辑、格式化、提交、stash、reset、切换分支、启动服务、写数据库、运行项目级大套件或派生子 Agent。冻结状态漂移必须 escalation。

## 必读材料与方法

1. 读取 `skill://review`，只执行 Standards 轴。
2. 阅读自动注入的项目与全局 `AGENTS.md`。
3. 阅读父 PRD `issue://jiwangyihao/new-api/19`、子 Issue `issue://jiwangyihao/new-api/23`。
4. 阅读集成父树中的：
   - `docs/agents/credit-operational-value-execution.md`
   - `docs/agents/credit-operational-value-wave-2-contract.md`
   - `docs/agents/credit-operational-value-wave-2-acceptance.md`
   - `docs/agents/credit-operational-value-issue-23.md`
   - `docs/agents/credit-operational-value-issue-23-acceptance.md`
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
5. 阅读冻结树 `.scratch/agent-progress/issue-23/{contract,status,evidence}.md`、cleanup 进度与最终 worker_done；这些只是证据索引，必须回到代码、迁移与测试核验。
6. 只做冻结 diff 和已有证据的短评审；不要重跑项目大套件，不要扩展到 #24–#28。

## 必须审查的 Standards 主题

### A. 深模块与 clean cutover

- Credit 预扣、目标累计结算、少结算、失败退款、absorbed/unknown 恢复均只通过 request-aware 深模块；
- controller/service/relay/异步 Credit 路径不得保留匿名 delta 绕路；若匿名 helper 为 timed 兼容保留，调用范围必须显式受限；
- 修改过的导出符号、调用点迁移及 stable sentinel 不留旧别名或文本解析分支；
- 不复制 #22 的购买 ingress、移动平均核心或通用 analytics DTO。

### B. 事务、锁序与整数安全

- 请求记录、目标权益与 CreditValuationState 在同一事务和固定锁序下更新；
- 目标增加/减少、debt、absorbed、unknown、舍入余数与版本累加全部使用整数 micros 和 checked arithmetic；
- 共享事务 coalescer 按稳定入队顺序逐请求校验与落账，中间失败整批回滚并给每项稳定结果；
- 不得以 sleep、宽泛 retry、吞掉数据库错误或浮点回退掩盖根因。

### C. 持久 Task identity

- 新 Credit Task 的 `subscription_request_id` 写入 JSON 与数据库投影，创建、轮询、重算、final/refund 和重启重放复用同一身份；
- legacy identity 必须由持久 Task 主键确定性生成，不使用时间、随机数或进程布尔值；
- 同 subscription 多 Task 隔离，冲突与不确定来源 fail-closed；
- timed Task 的既有匿名兼容不被误改为 Credit 路径。

### D. 清理器与数据库兼容

- 清理仅删除超过排他 cutoff 的 settled/refunded，保护 active/非终态、持久 Task 引用、混合版本 NULL 投影与可能回调；
- `tasks.subscription_request_id` 是 nullable、非唯一、跨 SQLite/MySQL 5.7/PostgreSQL 9.6 可用的索引投影，JSON/列同步与冲突行为 fail-closed；
- 稳定主键批次、幂等、单批失败原子回滚、真实 SQLite 并发串行化成立；
- 只读预览复用同一 eligibility/cutoff/reference 谓词且零写入；清理不删除低频 ledger、request attribution 或退款/重放审计；
- SQL 不依赖 SQLite 专属语义；MySQL/PostgreSQL 未实测必须诚实保留给 #27。

### E. 质量与范围

- 稳定 sentinel/code 可用 `errors.Is` 或结构字段判断，业务不解析错误文本；
- 热路径无无界扫描、按请求大整数分配、N+1 或无必要拷贝；
- 生产代码与测试基础设施边界清楚，测试不会依赖进程缓存偶然状态；
- 未越界实现 #24 ingress、#25 destructive recovery、#26 FX/conversion、#27 marker/migration 或 #28 release；
- 未新增 UI/可见文案时无需 i18n/browser，但必须有明确证据。

## 输出与完成

把不超过 650 字的报告写入：

`C:/Users/34404/AppData/Local/Temp/new-api-issue23-standards-final-review.md`

报告必须包含冻结 HEAD/基线、总评 `PASS`/`FAIL`、A–E 逐项结论、findings（严重度、文件/符号、直接证据、影响、最小修复建议）、范围外与未实测说明。推断标 `[INFERENCE]`；无 finding 写“0 findings”。

结束前再次确认 HEAD 未变且工作树 clean。随后使用当前 Dispatch 注入 capability 发送恰好一次有效 `worker_done`，正文含 PASS/FAIL、finding 数、最严重项、范围边界结论和报告绝对路径。进程完成不等于评审 PASS。
