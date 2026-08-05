# Issue #23 Spec 独立只读复评指令

## 目标与冻结现场

你是 GitHub Issue #23「完成 request_id 同步与异步可逆结算」的 Spec 只读复评 Agent。只读工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-23-request-settlement`

开始与结束时必须确认：

- `HEAD` 严格等于 `9b496ca0d46bad84b4977d63496a668388e99080`；
- `git status --short` 为空；
- 实现起始基线与 merge-base 为 `ec1858fec89509bdec9a90a230a8496047c5becd`；
- 审查范围严格为 `ec1858fec89509bdec9a90a230a8496047c5becd...9b496ca0d46bad84b4977d63496a668388e99080`。

禁止编辑、格式化、提交、stash、reset、切换分支、启动服务、写数据库、运行项目级大套件或派生子 Agent。冻结状态漂移必须 escalation。

## 必读材料与方法

1. 读取 `skill://review`，只执行 Spec 轴。
2. 阅读父 PRD `issue://jiwangyihao/new-api/19`、子 Issue `issue://jiwangyihao/new-api/23` 及评论；读取 #22 仅用于依赖合同。
3. 阅读集成父树中的：
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
   - `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md`
   - `docs/agents/credit-operational-value-wave-2-contract.md`
   - `docs/agents/credit-operational-value-wave-2-acceptance.md`
   - `docs/agents/credit-operational-value-issue-23.md`
   - `docs/agents/credit-operational-value-issue-23-acceptance.md`
4. 阅读 `.scratch/agent-progress/issue-23/` 的 contract/status/evidence、cleanup 证据和有效 worker_done；必须回到冻结代码与测试核验。
5. 只审冻结 diff 与已有证据；不要因为好奇扩展到 #24–#28，也不要重跑项目大套件。

## Acceptance/Gate 映射

对 GitHub #23 每条 acceptance criteria 和验收 Gate A–G 给出 `PASS`、`FAIL` 或“真实未覆盖”，至少核验：

### A. 请求级领域合同

- 预扣在同一事务写数量、CreditValuationState 与唯一 request 记录；足额预扣不形成欠额；同参数重放幂等，冲突稳定回滚；
- 请求记录完整保存 applied/deducted/debt、valuation_subscription_id、exact/estimated/unknown 活动快照、absorbed restore、restored unknown、规则/结算/状态版本和终态；
- 唯一结算入口使用 `request_id + original_subscription_id + target_applied_credit + final`，日志保留原 subscription，估值可路由目标权益；
- 增加按追加时池出账，超出可用只形成 debt；相同目标严格无操作；
- 减少先撤销本请求 debt，再按原请求快照恢复，清空吸收舍入余数；不得按退款时池平均；
- absorbed restore 不增加物化价值；后来 ingress 抵债后退款重新形成的可用量标 unknown；
- 负目标、缺记录/状态、映射冲突、终态冲突、溢出均为稳定错误和原子回滚。

### B. 同步链路、coalescer 与 Task

- SubscriptionFunding/BillingSession/quota/流式增量/重算/final/refund 持久传播同一 request_id 与累计目标，不走 Credit 匿名 delta；
- coalescer 在共享事务中按入队顺序逐请求处理，逐请求舍入/错误/结果归属；中间失败整批 DB 回滚，失败项保留领域错误，其余项返回稳定 rolled-back 错误；结果与同序逐条合同一致；
- 新 Task 持久化 `subscription_request_id` 到 JSON/投影，进程重启后重放不重复扣款/退款；
- legacy Task 使用持久主键生成 `legacy-task:<pk>` 等确定性身份，同 subscription 多 Task 隔离；来源不可证明时 fail-closed；timed Task 兼容不回归。

### C. Cleanup 合同

- 只删除 `settled/refunded` 且 `finalized_at < cutoff`；cutoff 边界排他；consumed/unknown/active/非终态不删除；
- retention 参数是可测试的现有入口，不新增未要求的配置/CLI；
- 持久 Task 投影与混合版本 NULL 行保护正确：明确 timed 的 NULL 不阻断，无法证明无 Credit 引用则稳定 fail-closed；
- 稳定有界主键批次、重复清理幂等、并发合法串行化、单批失败原子回滚；
- 只读候选预览复用完全相同的 eligibility/cutoff/reference 谓词且零写入；
- ledger、request attribution、退款与重放审计保留。

### D. 真实证据与可观察结果

- 真实 SQLite 公开入口覆盖预扣、追加、少结算、失败退款、终态重放、交错 ingress、原子回滚；不得靠直接插请求快照冒充主链；
- 至少一条真实 SubscriptionFunding/BillingSession 请求链记录实际 request_id、累计目标、数量、估值与终态；
- coalescer/cleanup 的真实文件 SQLite 并发与窄 `-race` 证据可追溯；
- 五个运营分析接口在结算前后保持一致，Credit `time_based_value` 仍为 null；
- 本切片未新增 UI/可见文案时明确说明不需要 browser/i18n；若实际新增则必须六语言完整。

### E. 依赖与范围边界

- 保留 #20 精确价格、#21 timed grant/CNY+USD、#22 32 CNY Credit/current_only/BigInt/权威 micros 合同；
- 只深化 #22 request tracer，不复制购买 ingress、移动平均核心、低频来源或 analytics DTO；
- 未实现 #24 兑换/管理员 increase、#25 decrease/退款/拒付/财务恢复、#26 转换/FX/虚拟快照、#27 历史迁移/ready、#28 发布；
- 转换期间仅留稳定目标路由接缝，不伪造转换价值；
- MySQL/PostgreSQL 零 SKIP 明确保留给 #27；明显方言错误仍是 finding。

## 特别核验的风险

- `.scratch` status 顶部旧阶段文字与底部最终完成记录是否造成交付歧义；以最终代码/提交为准，但缺失关键证据应报告；
- `tasks.subscription_request_id` 新投影的迁移、Insert/Update/JSON 同步、NULL 混合版本行为和索引是否满足三数据库合同；
- cleanup 的“可能回调”保护是否有真实持久引用依据，不依赖内存状态；
- legacy Credit Task 兼容是否仍进入深模块，且没有恢复生产匿名 delta。

## 输出与完成

把不超过 750 字的报告写入：

`C:/Users/34404/AppData/Local/Temp/new-api-issue23-spec-final-review.md`

报告必须包含冻结范围、总评 `PASS`/`FAIL`、GitHub acceptance 与 Gate A–G 映射、findings（具体条款、文件/符号、直接证据、影响和最小修复建议）、#20–#22 回归结论、范围外/未实测说明。推断标 `[INFERENCE]`；无 finding 写“0 findings”。

结束前再次确认冻结 HEAD 与 clean tree。随后使用当前 Dispatch 注入 capability 发送恰好一次有效 `worker_done`，正文包含 PASS/FAIL、finding 数、最严重项、acceptance/边界结论和报告绝对路径。未完成或冻结状态漂移不得报告成功。
