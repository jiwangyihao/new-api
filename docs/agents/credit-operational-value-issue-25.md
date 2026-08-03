# Issue #25 实现 Agent 指令

## 目标与垂直交付

你负责父 PRD #19 的 GitHub Issue #25「覆盖 Credit 减少、退款、拒付与财务恢复」。必须在 Orca 为你创建的隔离子工作树中，把四条真实破坏性入口——管理员 Credit decrease、Credit 订单退款、拒付和财务恢复——统一接入 CreditValuation 混合池 outflow。每次操作都按操作前池平均同步减少 Credit 数量与 exact/estimated/unknown 运营剩余价值；可耗尽可用余额或形成 settlement debt，但成本不得为负。来源终态、低频幂等账本、邀请奖励取消和分析结果必须与数量/估值状态在同一事务一致。

这是永久 feature，不是只改 adjustment DTO 或新增 ledger 行。你必须贯通深模块 outflow、现有恢复/支付终态入口、管理员 API/UI、幂等与终态优先级、并发、分析生命周期、六语言和真实 SQLite/API/浏览器证据。严格禁止越界：#23 拥有请求级退款快照；#24 拥有兑换和管理员 increase；#26 拥有转换/FX/在途请求；#27/#28 拥有迁移门禁与发布。计时订单现金退款不等于服务撤销，本切片不得新增 timed grant reversal。

## 必读材料与 Skill

修改前依次阅读并服从：

1. 仓库及全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/25`；GitHub CLI 始终显式传 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-3-contract.md`；你是低频破坏性 outflow、recovery 终态和管理员 decrease 的主改者。
5. 已集成 `.scratch/agent-progress/issue-20`、`issue-22`、`issue-23`、`issue-24` 合同和最终实现。确认精确 micros、CreditValuation outflow seam、请求快照、低频 ingress/ledger 与 adjustment API 已存在。缺失时立即用 Orca `orchestration ask` 报告，不得复制依赖切片。
6. `CONTEXT.md`、ADR 0001、ADR 0002。
7. 新规格第 6、7.3–7.4、8、9、11.3、12、13、14 节；实施计划任务 3 的 outflow/debt 部分、任务 4 中仅 recovery/decrease 部分，以及任务 9 的 decrease 交互部分。

本任务必须先读取并执行 `skill://tdd`，每个终态、回滚、幂等和并发合同从合理缺陷会失败的测试开始。恢复或并发异常难以解释时必须读 `skill://diagnosing-bugs` 并先稳定复现；深模块边界不清时读 `skill://codebase-design`，但不得推翻 ADR/spec。修改管理员 UI 前读 `skill://shadcn-ui`，新增/改变可见文案前读 `skill://i18n-translate` 并维护 en、zh、fr、ru、ja、vi。只有实际触及动态计价表达式才读 `pkg/billingexpr/expr.md`。

## 破坏性 outflow 领域合同

- 从已有 `RecoverCreditBalanceTx`、管理员 adjustment、订单退款/拒付和 financial recovery 真实入口出发，不新增“仅估值用”的平行 API。调用方提供稳定来源事实与目标数量，CreditValuation 深模块负责同事务更新数量和状态。
- 操作前令 `A=max(token_limit-token_used,0)`、请求回收 `Q`、`C=min(Q,A)`。按 `floor(pool_component × C / A)` 分别移除 exact、estimated、unknown；若 `C=A`，直接移除全部余数。`Q-A` 只形成 settlement debt，不产生负 exact/estimated 成本或虚构 unknown。
- 禁止按订单原价、实际支付/退款金额、充值档位或来源批次撤值。订单与支付字段仅用于来源身份、审计、终态优先级和邀请取消。
- 低频 outflow 不读取或改写活动 `SubscriptionPreConsumeRecord` 的成本快照。若与 #23 请求 settle/refund 并发，必须按冻结锁序串行化；提交结果属于某一合法串行顺序。请求之后退款仍使用自己的活动快照。
- 每个成功事务同时提交目标 Credit 权益、`CreditValuationState`、结构化 `CreditBalanceLedger`、订单/恢复终态和邀请奖励取消。任一稳定失败注入都必须整笔回滚；不得出现“状态已减、订单仍未恢复”或相反情况。
- ledger 至少结构化保存 source type/id/key、operation、gross Credit、consumed available、debt formed、removed exact/estimated/unknown、币种、规则版本、状态版本、参数指纹与结果终态；JSON 只能补充，关键审计字段不能只放 JSON。

## 终态、幂等与邀请隔离

- 相同来源身份和完全相同参数重放返回原提交结果，不再次扣减。相同幂等键或来源但数量、operation、目标权益、终态、规则版本或指纹不同，返回稳定 idempotency/terminal conflict 并回滚。
- 对同一订单的退款与拒付遵循仓库现有支付终态优先级，最多发生一次实际 Credit 回收。先后顺序、重复 webhook、进程重启和并发回调都必须依赖数据库终态/唯一键，而非内存标志。
- 财务恢复可处理已有 disabled-plan 权益；恢复已授权订单不因套餐后来 disabled 而失效。该例外不允许创建新分配。已有 disabled 权益仍可消费。
- Credit 回收不得产生邀请收益；若原 Credit 订单曾产生错误或既有奖励，取消必须与回收同事务且幂等。Credit 不进入邀请付费统计。计时订单退款若不缩短实际服务，不能修改 timed grant 或运营剩余价值。
- 所有业务可分支失败使用稳定 code/sentinel；controller/service/frontend 不解析错误文本。JSON 操作使用 `common/json.go` 包装。

## 管理员 decrease API/UI

- 扩展并复用 #24 的管理员 Credit adjustment 接口。`decrease` 要求正 amount、非空 reason、稳定 idempotency key，且不得接受 `plan_id`；携带任何 plan 立即稳定拒绝。不要调用 increase 的档位预览或 ingress。
- 响应返回 gross Credit、消耗可用 Credit、形成欠额、移除的 exact/estimated/unknown、币种和 `state_version_after`；精确 micros 使用字符串，兼容字段不得参与算术。
- UI 切换到 decrease 时必须隐藏并清空 increase 的 plan、价格和预览状态；最终 payload 不得残留 `plan_id`。失败后不改变业务参数的重试复用同一 key；amount/operation/reason 等指纹参数变化后生成新 key。
- 清晰提示超出可用量会形成结算欠额，而不是负退款或现金负债；使用“运营剩余价值”“回收”“成本未知”等领域术语。新增文案补齐六语言并通过 missing/extras。

## 崩溃恢复与提交纪律

第一项实际改动必须创建并提交：

- `.scratch/agent-progress/issue-25/status.md`：阶段、完成项、下一步、阻塞、最近安全提交；
- `.scratch/agent-progress/issue-25/evidence.md`：RED/GREEN 命令、事务失败、并发、API/浏览器和分析证据；
- `.scratch/agent-progress/issue-25/contract.md`：outflow 输入/结果、锁序、来源身份、终态优先级、幂等指纹、ledger 字段、管理员 payload/key 生命周期、邀请合同、共享文件和明确非所有权。

每个可编译、可验证小步立即使用 Conventional Commits 提交并更新进度文件。关键成果不得只留在终端、大段脚本或未提交工作树。修改导出符号前使用 LSP references。需要 #26 接口不是本切片阻塞理由；禁止复制转换逻辑。

## 验证与完成条件

至少以定向行为测试证明：普通比例 outflow、exact/estimated/unknown 混合池、完全清空余数、余额不足形成欠额、零可用量、成本永不为负；管理员 decrease 拒绝 plan；退款/拒付/财务恢复均不按订单原价；相同重放/冲突；退款与拒付终态竞争；事务每一失败点整笔回滚；与 request settle/refund 的合法并发串行化；活动请求快照不变；邀请隔离；disabled-plan 恢复边界；分析立即显示 available/exhausted/debt 且零余额/欠额不计 active paid count。

运行真实 SQLite 领域/API tracer，至少通过真实管理员 adjustment API 和一个真实订单 recovery 入口执行 outflow，再读取五个运营分析 API 的相关明细。启动应用并用真实浏览器验证 increase→decrease 切换会清除 plan/preview、请求 payload 无 `plan_id`、响应和分析按移动平均变化。静态拦截只证明渲染，不能替代 API 行为。并发测试要验证合法串行结果集合，并对相关定向包运行 Go `-race`；完整 MySQL/PostgreSQL 矩阵由 #27 负责，DryRun 不是验收。

只运行本切片定向测试和必要 smoke，格式化明确修改文件并执行 `git diff --check`；不要运行全仓测试或部署。完成前逐条复核 Issue #25 acceptance criteria，提交全部代码/恢复记录并保持工作树干净。随后在当前 Dispatch 只发送一次 `worker_done`，列出提交 SHA、领域/API/UI 合同、定向测试、SQLite/API/浏览器/并发证据、共享文件、三数据库实际范围、风险和进度目录；明确声明未实现 #26–#28。不要关闭 Issue、合并或回收工作树，等待协调器验收。
