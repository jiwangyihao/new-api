# Issue #23 实现 Agent 指令

## 目标与垂直交付

你负责父 PRD #19 的 GitHub Issue #23「完成 request_id 同步与异步可逆结算」。必须在 Orca 为你创建的隔离子工作树中，把 #22 的最小同步 Credit tracer 深化为完整、持久、可重放的 `request_id + target_applied_credit` 领域合同：预扣、实时追加、少结算、最终结算、失败退款、正 delta 合并以及异步任务都沿同一请求身份更新 Credit 数量、请求活动快照和运营剩余价值。

这是永久 feature，不是只扩 DTO 或只修合并器。你必须贯通 Model 深模块、现有 `SubscriptionFunding`/`BillingSession`/quota 调用链、流式与异步任务入口、记录清理、稳定错误和真实数据库行为测试。严格禁止越界：购买与 Credit 分析基线属于 #22；兑换与管理员 increase 属于 #24；管理员 decrease、订单退款/拒付和财务恢复属于 #25；计时转 Credit 的冻结价值、FX 与转换期间虚拟请求快照属于 #26；历史迁移、ready 门禁和生产发布属于 #27/#28。

## 必读材料与 Skill

修改前按顺序阅读并服从：

1. 仓库及全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/23`；GitHub CLI 一律显式传 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-2-contract.md`，你是请求结算、合并器和异步身份传播的主改者。
5. `.scratch/agent-progress/issue-20/contract.md`、`issue-22/contract.md` 及其最终实现提交；确认 #22 的 CreditValuation 深模块、最小同步 request tracer 与稳定错误接缝确实存在。缺失时立即通过 Orca `orchestration ask` 报告，不能自行复制 #22。
6. `CONTEXT.md`、ADR 0001、ADR 0002。
7. 新规格第 5.4、6、7.3–7.5、9、11.3、13、14 节，以及实施计划任务 3 的 request restore 部分和任务 6。第 7.5 节只实现身份/目标路由接缝，价值与 FX 算法留给 #26。

本任务必须先读取并执行 `skill://tdd`：每个可观察合同先写会因合理缺陷失败的测试，再最小实现，再重构。请求结算深模块边界需要调整时读 `skill://codebase-design`，但不得推翻 ADR/spec；并发、锁、合并器、异步重放或数据库失败无法从错误直接解释时必须读 `skill://diagnosing-bugs`，先稳定复现再改。只有实际新增用户可见文案时才读 `skill://i18n-translate` 并补齐六语言；本切片不应为后端结算无故新增 UI。只有真正触及动态计价表达式才读 `pkg/billingexpr/expr.md`。

## 请求级领域合同

- `request_id` 是持久化请求级幂等身份。Credit 预扣必须在同一事务锁定目标权益、估值状态和请求记录，同时增加实际消费量、按操作前混合池移除 exact/estimated/unknown，并写入 `applied_credit`、`deducted_available_credit`、`debt_formed_credit`、`valuation_subscription_id`、三类活动扣除快照、规则/结算版本和状态。禁止先改 `token_used` 再补快照。
- 预扣要求可用 Credit 足额，不得形成结算欠额；若相同 `request_id` 已存在，完全相同的预扣重放返回已提交结果，不重复扣除；订阅、预扣量或不可变参数冲突返回稳定错误并整笔回滚。
- 提供一个窄领域入口，语义为 `request_id + original_subscription_id + target_applied_credit + final`。调用方提交目标累计量而不是匿名 delta；model 根据预扣记录与已存在的转换映射选择实际估值权益。请求日志始终保留原 `subscription_id`，估值记录可通过 `valuation_subscription_id` 指向目标 Credit 权益。
- 目标大于当前 `applied_credit` 时，差额按追加时池的移动平均分别扣除 exact/estimated/unknown；超出当前可用量只增加 `debt_formed_credit`，不产生虚构成本。目标相同是严格无操作，不增加 `state_version` 或 `settlement_version`。
- 目标减小时先撤销本请求尚未撤销的结算欠额，再按活动请求快照比例减少 `deducted_available_credit` 和对应成本。清空活动快照时必须带走全部舍入余数；不能按退款时的新池平均恢复。
- 同一锁定事务内比较退款前后可用量，只有 `newly_available` 可进入估值状态。仍被账户其他欠额吸收的 exact/estimated/unknown 份额进入 `absorbed_restore_*` 审计，不增加物化价值；已被后来入账抵扣的本请求欠额退款重新形成可用量时增加 `restored_unknown_credit`，不得使用后来入账价格或当前池平均伪造成本。
- `final=true` 后相同目标允许幂等重放；不同目标仅允许规格明确的退款/纠正路径。终态后非法增加、目标负数、记录缺失、状态缺失/不一致、订阅映射冲突和算术溢出均返回稳定 code/sentinel 并回滚，controller/service 不解析错误文本。
- #23 只建立“转换前创建、转换后结束”请求的稳定映射与目标路由：不得在这里计算转换单位价值、FX、创建虚拟扣除快照或按转换价值恢复。这些行为必须留出窄接缝，由依赖本切片的 #26 完成。

## 调用链、合并器与异步任务

- `SubscriptionFunding` 保存 `request_id` 与当前请求目标累计量；其 settle/refund 把 delta 转换为目标累计量后调用同一领域入口。`BillingSession`、同步 quota、流式增量、失败退款和重算均不得对 Credit 调用无身份 token delta。
- 使用符号引用定位并迁移所有 `PostConsumeUserSubscriptionTokenDelta` 等匿名调用点。实现结束后，若该 helper 必须为 timed 兼容保留，应限制在包内/显式 timed 路径；controller、service、relay 和异步 Credit 路径不得绕过请求入口。
- 正 delta 合并器可按权益共享一个事务，但队列元素必须保留 request identity、目标累计量和稳定入队顺序。事务内逐请求验证、逐请求舍入、逐请求写回结果；最终状态必须与相同顺序逐条事务一致。禁止先按 subscription 求和再匿名落账。
- 并发请求必须遵循冻结锁序；测试断言合法串行结果集合，不依赖 goroutine 调度。重试、进程重启与合并器重建只能依靠数据库请求记录恢复，不依赖内存 `settled/refunded` 布尔值。
- `TaskPrivateData` 增加 `subscription_request_id`，新任务创建时持久化；轮询结算、重算和失败退款复用同一 ID。旧任务缺失该字段时以持久化 Task 主键生成确定性兼容身份，同一 Task 重放保持一致，不得用当前时间或随机数。
- 旧 Task 兼容入口仍必须走 Credit 深模块：追加按当时池平均出账；退款新形成的可用 Credit 在无法证明来源时标 unknown。不要在本切片伪造 #26 的转换价值。
- 预扣记录清理只删除 `settled/refunded` 且早于“最大异步任务生命周期 + 运维保留窗口”的记录；非终态记录永不按固定天数删除。沿用项目配置惯例提供可测试的保留参数和只读诊断，不引入第二份高频永久 ledger。

## 崩溃恢复与提交纪律

第一项实际改动必须创建并提交：

- `.scratch/agent-progress/issue-23/status.md`：阶段、完成项、下一步、阻塞、最近安全提交；
- `.scratch/agent-progress/issue-23/evidence.md`：每次 RED/GREEN 命令、关键输出、并发/异步现场和失败根因；
- `.scratch/agent-progress/issue-23/contract.md`：请求记录字段、领域入口、锁序、终态状态机、稳定错误、调用点、Task 兼容身份、清理策略、共享文件及明确非所有权。

持续更新并在每个可编译、可验证小步使用 Conventional Commits 提交；不要把关键代码只留在未提交工作树或大段一次性脚本。需要 #24 尚未提供的低频入口时继续独立工作并 Orca ask，不得复制兑换/管理员逻辑。修改导出符号前使用 LSP references。

## 验证与完成条件

至少用定向行为测试证明：预扣快照与数量同事务；重复预扣幂等/冲突；目标增加按追加时平均；目标减少恢复原请求成本；清空带走余数；欠额先撤销；absorbed restore；后来入账抵债后的退款转 unknown；终态规则；两个请求稳定顺序；合并器与逐条同序等价；资金会话/流式调用传 request ID；新旧异步 Task 重放；非终态清理保护；状态缺失、冲突与溢出原子回滚。

运行真实 SQLite 领域/API tracer，并以本地真实应用或现有可控 mock-upstream smoke 至少走一次“预扣 → 增量 → 少结算/退款”请求链，记录 request ID、目标累计量和最终数量/估值；不能靠直接插入请求快照冒充主链路。运行针对合并器的 Go `-race` 定向检查。完整 MySQL/PostgreSQL 矩阵由 #27 负责，但 SQL 与锁语义必须跨库；GORM DryRun 不是验收。

只运行当前切片的定向测试和必要 smoke，格式化明确修改文件，执行 `git diff --check`；不要运行项目级全量测试或部署生产。完成前逐条复核 Issue #23 acceptance criteria，提交所有代码/恢复记录并保持工作树干净。随后在当前 Dispatch 只发送一次 `worker_done`，列出提交 SHA、领域/API 合同、迁移调用点、定向测试、SQLite/smoke/race 证据、共享文件、遗留风险和进度目录；明确声明未实现 #24–#28。不要关闭 Issue、合并或回收工作树，等待协调器验收。
