# Issue #23 Spec 最终冻结复评指令

## 目标与不可变现场

你是父 PRD GitHub #19、子 Issue #23「完成 request_id 同步与异步可逆结算」的最终 Spec 只读复评 Agent。只读工作树固定为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-23-request-settlement`

开始和结束时都必须核对：

- `HEAD` 严格等于 `8cdfd4acb78b502af4c0232460baf7df852b7b2c`；
- `git status --short` 无输出；
- fixed point 与 merge-base 均为 `ec1858fec89509bdec9a90a230a8496047c5becd`；
- 只审 `git diff ec1858fec89509bdec9a90a230a8496047c5becd...8cdfd4acb78b502af4c0232460baf7df852b7b2c` 和对应提交列表。

禁止编辑、格式化、提交、stash、reset、切换分支、启动服务、写数据库、运行大套件或派生 Agent。先前未落盘而被停止的 #23 Spec 评审不能当作结论；必须独立完成本次复评。

## 必读材料

1. `skill://review`，只执行 Spec 轴。
2. 父 PRD `issue://jiwangyihao/new-api/19`、子 Issue `issue://jiwangyihao/new-api/23` 及评论；读取 #22 只为依赖合同。
3. 集成父树下的 `CONTEXT.md`、ADR 0002、2026-08-02 spec/plan、wave-2 contract/acceptance、Issue #23 指令与 acceptance。
4. 冻结树 `.scratch/agent-progress/issue-23/` 下 contract/status/evidence、cleanup、double-count-fix 证据与有效 worker_done。必须回到冻结实现和测试核验。

## Acceptance/Gate 映射

对 GitHub #23 每条 acceptance 和验收 Gate A–G 给 `PASS`、`FAIL` 或“真实未覆盖”，至少核验：

### A. 请求领域

- 公开入口在同事务写目标权益、CreditValuationState 和唯一 request record；足额预扣不形成 debt，同参数重放幂等，冲突稳定回滚。
- 记录完整保存 applied/deducted/debt、valuation_subscription_id、exact/estimated/unknown 活动快照、absorbed restore、restored unknown、规则/状态/结算版本与终态。
- 唯一入口是 `request_id + original_subscription_id + target_applied_credit + final`；日志保留原 subscription，估值可路由目标权益。
- 增加按追加时池出账；减少先撤销本请求 debt，再按原请求快照恢复并吸收清空余数；absorbed 不增加物化价值，后来 ingress 抵债后重开的价值为 unknown。
- 负目标、缺记录/状态、映射/终态冲突、溢出均使用稳定 sentinel 并原子回滚。

### B. 同步、coalescer 与 Task

- SubscriptionFunding/BillingSession 的 reserve、追加、final、refund 全程传播同一 request_id/累计目标；兼容字段不得双计数；Credit 不走匿名 delta。
- coalescer 在共享事务内按稳定入队顺序逐请求处理，中间失败整批回滚；失败项保留领域错误，其余项得到稳定 batch-rolled-back 错误。
- 新 Task JSON 与列投影持久化 subscription_request_id；重启重放不重复扣款/退款。legacy 使用持久主键生成 deterministic identity，同 subscription 多 Task 隔离；来源不可证明时 fail-closed；timed Task 不回归。

### C. Cleanup

- 只删除 `settled/refunded` 且 `finalized_at < cutoff`；consumed/unknown/active/非终态不删。
- active Task 非空投影保护；混合版本 NULL 若无法证明无 Credit identity 则稳定 fail-closed，明确 timed NULL 不阻断。
- stable bounded PK batches、重复清理幂等、并发合法串行化、单批失败原子回滚。
- Preview 使用同一 eligibility/cutoff/reference 谓词、结果稳定且数据库零写入；ledger、request attribution、退款/重放审计保留。
- retention 使用现有参数入口，不新增未要求的 CLI/配置。

### D. 真实证据

- 真实 SQLite 公开入口覆盖预扣、追加、少结算、失败退款、终态重放、交错 ingress、故障回滚；不能靠直接插 request snapshot 冒充主链。
- 至少一条真实 Controller/Service 请求链记录实际 request_id、累计目标、数量、估值和终态；已知 Kyren 测试须证明 1,000→预扣 200→800/32 CNY→settled→重放无版本漂移。
- coalescer/cleanup 有真实文件 SQLite 并发与窄 race；五个运营接口/32 CNY current-only 合同和 timed CNY/USD 不回归，Credit time_based_value 仍为 null。
- 本切片没有 UI/可见文案时，应明确 browser/i18n 不需要，而非伪造前端验收。

### E. 范围与数据库

- 只深化 #22 request tracer，不复制购买 ingress、移动平均、来源或 analytics DTO。
- 未实现 #24 管理员/兑换 ingress、#25 destructive recovery、#26 FX/conversion/virtual snapshot、#27 migration/ready、#28 release。
- tasks.subscription_request_id schema/index/Insert/Update/JSON 同步和混合版本行为必须满足三数据库静态合同；MySQL/PostgreSQL 实机零 SKIP 留给 #27，不能冒充已通过。

特别审查：status 顶部旧阶段文字与最终完成记录是否造成关键证据歧义；cleanup 对“可能回调”的保护是否依赖真实持久引用；legacy Credit Task 是否仍经过深模块；最终 double-count 修复是否只去除二次累加且未破坏其他状态。

## 输出与完成

将不超过 750 字的报告写到：

`C:/Users/34404/AppData/Local/Temp/new-api-issue23-spec-final-rereview.md`

报告必须含冻结范围、总评 `PASS`/`FAIL`、GitHub acceptance 与 Gate A–G 映射、findings（条款、文件/符号、直接证据、影响、最小修复）、#20–#22 回归、范围外与未实测说明。推断标 `[INFERENCE]`；无 finding 明写 `0 findings`。若某证据真实未覆盖，不能用已有窄测猜 PASS。

结束前再次确认 HEAD 与 clean tree。随后用当前 Dispatch capability 发送恰好一次有效 `worker_done`，正文含 PASS/FAIL、finding 数、最严重项、acceptance/边界结论和报告绝对路径。
