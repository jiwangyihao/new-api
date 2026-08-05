# Issue #23 清理与最终交付续作 Agent 指令

## 任务目标

你负责完成父 PRD #19、GitHub Issue #23「完成 request_id 同步与异步可逆结算」最后尚未交付的请求记录清理合同，并对整个 #23 切片做最终、可审计的验证与交付。必须复用现有 Orca 工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-23-request-settlement`

开始时必须确认分支为 `jiwangyihao/issue-23-request-settlement`、HEAD 严格等于 `d9e620191f8ca02c237859cc0250f98209749016`、`git status --short` 为空。不得 reset、rebase、切换分支、合并父树、关闭 GitHub Issue、部署生产或操作其他工作树。

此前安全点已经完成且不得重做：请求级预扣/追加/减少/退款及 exact/estimated/unknown/absorbed 审计；`request_id + original_subscription_id + target_applied_credit + final` 领域入口；同步 `SubscriptionFunding`/`BillingSession`；共享事务、稳定顺序、逐请求归因的 coalescer；`TaskPrivateData.subscription_request_id` 持久化；新 Task 全生命周期复用身份；legacy Task 使用 `legacy-task:<task_pk>` 确定性身份。先完整读取 `.scratch/agent-progress/issue-23/{contract,status,evidence}.md`，把这些提交视为冻结依赖，不得重新设计或替换。

## 必读材料与 Skill

开始编码前依次读取并遵守：

1. 根 `AGENTS.md` 与自动注入的全局规则；
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/23`；
3. `docs/agents/credit-operational-value-execution.md`；
4. `docs/agents/credit-operational-value-wave-2-contract.md`；
5. `docs/agents/credit-operational-value-issue-23.md`；
6. `docs/agents/credit-operational-value-issue-23-acceptance.md`；
7. `CONTEXT.md`、`docs/adr/0001-credit-balance-entitlement.md`、`docs/adr/0002-credit-operational-remaining-value.md`；
8. 2026-08-02 规格中请求快照、异步身份、清理、并发与版本边界，以及实现计划任务 3/6；
9. `skill://tdd`、`skill://diagnosing-bugs`、`skill://codebase-design`。

本任务是永久行为变更，必须严格执行 TDD；并发、保留窗口、失败原子性或旧 Task 兼容出现异常时使用 diagnosing-bugs；清理入口和配置接口的深模块边界使用 codebase-design。不得派生子 Agent，不得运行项目全量测试或无关 formatter；最终全量由协调器统一执行。

## 唯一实现范围：请求记录清理

只实现 `SubscriptionPreConsumeRecord`（以及为该表服务的最小配置/诊断接缝）的安全清理：

1. **可删除状态**：只有明确终态 `settled` 或 `refunded` 的请求记录才可候选删除。`consumed`、未知状态、非终态、仍可能被异步 Task/回调/重放引用的记录永不因固定天数删除。
2. **时间边界**：删除阈值必须是“最大异步任务生命周期 + 运维保留窗口”，使用项目现有配置惯例与整数/持续时间解析，不写魔法常量。精确覆盖阈值前、等于阈值、阈值后；边界语义须由测试固定。
3. **引用保护**：若 Task、BillingSession 或项目当前持久结构仍能在未来结算/退款该 `request_id`，即使请求记录显示终态也不得提前删除。不得以进程内状态、随机值或当前内存队列作为唯一依据。
4. **批次与排序**：清理按稳定主键顺序和有界 batch 执行，可重复运行；相同快照、相同参数得到确定性删除集合。不得一次无界扫描/删除全表。
5. **幂等与并发**：重复清理不报错、不重复影响 ledger；清理与终态重放、失败退款、异步回调并发时只接受合法串行化结果，不得删除仍被使用的记录或产生部分状态。
6. **失败原子性**：同一批次中故障注入必须整批回滚；不得出现请求记录部分删除而低频 ledger、请求归因、Task 身份或审计引用半残留。
7. **只读诊断**：提供项目既有风格的只读诊断/预览，至少报告 cutoff、batch、候选数、受保护数、按终态汇总和稳定原因；诊断不得写数据库。不要为此创建新的管理 UI 或六语言文案。
8. **审计保留**：清理不得删除或改写 `CreditBalanceLedger`、低频来源快照、订单/兑换/Task 事实、请求日志原始 `subscription_id`，也不得抹除后续退款/重放所需的 attribution。

优先更新现有文件，不为一个 helper 创建不必要抽象。预计主改 `model` 中请求记录/估值深模块及其定向测试，必要时增加最小 `setting` 配置接缝；任何跨文件导出符号变更先用 LSP references。所有 JSON 编解码继续使用 `common/json.go` 包装。数据库实现必须静态兼容 SQLite、MySQL 5.7、PostgreSQL 9.6；真实 MySQL/PostgreSQL 零 SKIP 验收仍属于 #27，不得冒充完成。

## 明确禁止范围

- 不实现 #24 的兑换、管理员 increase、UI/i18n 或跨币种正向入账；
- 不实现 #25 的管理员 decrease、退款、拒付、财务恢复或 destructive recovery；
- 不实现 #26 的转换估值、FX parser/provider/Option 生命周期、虚拟请求成本快照；
- 不实现 #27 的历史迁移、marker 生命周期、`ready/suspended` 切换或三数据库最终矩阵；
- 不实现 #28 的镜像、备份、部署或生产发布；
- 不恢复 Credit 匿名 delta。若旧测试仍期待匿名 Credit Task 行为，只允许把测试夹具迁移到已交付的 deterministic identity，不得放宽生产合同；
- 不修改前端、locale、报告文案，不新增 schema，除非清理合同绝对需要且先通过 Orca question 获得协调器批准。

## TDD 与验证顺序

先更新并提交恢复进度文件，再做生产代码：

- `.scratch/agent-progress/issue-23/cleanup-status.md`
- `.scratch/agent-progress/issue-23/cleanup-contract.md`
- `.scratch/agent-progress/issue-23/cleanup-evidence.md`

每个安全点立即写明当前 HEAD、dirty 文件、精确 RED/GREEN 命令与关键输出。优先形成以下可失败测试：

1. 仅过期 settled/refunded 被删除；active/non-terminal 保留；
2. cutoff 前/等于/后边界；
3. 活跃 Task/request identity 引用保护；
4. 稳定主键 batch、重复清理幂等；
5. 清理与 final replay/failure refund 并发；
6. 中间失败整批回滚；
7. 只读诊断重复调用一致、数据库完整快照/`total_changes` 不变；
8. ledger、request attribution、Task 与订单事实不受影响。

每个 RED 都必须在旧实现上因目标行为缺失而失败；随后做最小 GREEN。定向真实 SQLite 用公开领域入口构造事实，不直接伪造请求快照冒充主路径。运行相关用例 `-count=10`；并发路径运行窄 `go test -race`。不得用 sleep/调度碰巧通过。

## 最终交付门禁

清理安全点完成后，继续验证整个 #23，而不是只验证新 helper：

- 请求领域：预扣、追加、减少、欠额、absorbed/unknown、终态冲突和原子回滚；
- 同步链路：稳定 request_id 的 reserve/追加/final/refund；
- coalescer：共享事务、稳定入队顺序、逐请求结果与 batch rollback；
- Task identity：新/legacy、重启重放、success/failure、同 subscription 多 Task 隔离；
- 清理：全部上述边界、诊断、并发与审计保留；
- `go test ./model ./service ./controller -count=1` 或至少受影响包，任何既有失败必须精确记录并在本切片范围内完成修复，不能隐瞒；
- 32 CNY Credit tracer 与 #21 timed CNY/USD 聚焦回归保持通过；Credit `time_based_value` 仍为 null；
- 一个真实本地请求链 smoke：预扣→增量追加→少结算或失败退款，记录实际 request_id、累计目标、原 subscription_id、valuation_subscription_id、终态与五接口结果；无 UI 变更时明确记录“不需要浏览器 UI 变更”；
- gofmt 仅作用于明确修改的 Go 文件；`git diff --check`；最终工作树 staged/unstaged/untracked 全零。

MySQL/PostgreSQL、全项目测试、生产部署均不在 Worker 自行执行范围。不得声称未运行项通过。

## 恢复、提交与完成协议

频繁小步提交，提交消息遵循 Conventional Commits 且 subject 使用简体中文。任何 socket/model/kernel 异常前先把 progress 和可恢复代码提交；不要运行长脚本，把诊断/结果持续写入上述文件。最终更新主 `.scratch/agent-progress/issue-23/{status,evidence,contract}.md`，逐条映射 GitHub #23 acceptance，列出提交 SHA、命令、关键输出、未运行范围和 #26 路由接缝。

只有在实现、定向验证、请求 smoke、资源清理和 clean tree 均完成后，使用注入 capability 从本 Worker 终端发送且仅发送一次有效 `worker_done --outcome succeeded`。若存在不能在本范围修复的真实 blocker，使用 Orca question/escalation，不得把 HANDOFF_READY 冒充 Issue 完成。
