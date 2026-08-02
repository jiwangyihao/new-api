# Credit 运营剩余价值 Agent 执行上下文

## 任务与基线

- 父 PRD：`issue://jiwangyihao/new-api/19`。
- 子 Issue：`#20` 至 `#28`，全部位于 GitHub 仓库 `jiwangyihao/new-api`；查询和修改 Issue 时必须显式传入 `--repo jiwangyihao/new-api`。
- 生产行为基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`。
- 当前协调基线：`73c658daa8e7954cb6f229348aac80287253391c`，其中代码基线 `c51ee86a33d87c30f080567d3d59b801f064ba5b` 是生产基线的后代，并增加了本任务的设计资料。
- 集成工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`，分支 `jiwangyihao/credit-operational-value-integration`。
- Orca Run：`run_59804e39b728`。
- 主工作树含用户自有的未提交 `CLAUDE.md`，不得触碰、覆盖、暂存或清理。`account`、`disk` 等既有工作树不是本 Run 创建的资源，禁止回收。

## 必读材料

开始实现前按顺序阅读：

1. `AGENTS.md` 及自动注入的全局规则。
2. 父 PRD `issue://jiwangyihao/new-api/19` 与当前子 Issue。
3. `CONTEXT.md`。
4. `docs/adr/0001-credit-balance-entitlement.md`。
5. `docs/adr/0002-credit-operational-remaining-value.md`。
6. `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`。
7. `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md` 中与当前 Issue 对应的任务段。
8. `pkg/billingexpr/expr.md`，仅当改动触及 tiered/dynamic billing expression 路径时必须额外读取。

旧的 2026-06-04 付费套餐分析文档已注明被新设计取代；不得重新引入“按当前套餐正价推导 Credit 价值”或 `(user_id, plan_id)` 最近订单关联。

## 固定业务合同

- Credit 是唯一显式 `credit_balance` 权益。可用量为 `max(token_limit-token_used, 0)`，负部为结算债务。
- 全局 Credit 套餐是零价格、永久、不重置的服务额度容器；有价计时套餐是充值层级。不能用全局容器当前价格估值，也不能仅凭 `end_time = 0` 推断 Credit。
- 任何套餐都不限制模型范围；遗留 `model_limits` 继续忽略并在保存时清理。
- 已持有的 disabled-plan 权益仍可消费；新购买、兑换、转换、管理员授予继续拒绝 disabled plan。
- `运营剩余价值` 是未交付服务的运营估计，不是递延收入、现金负债、发票额、实收额或可退款金额。
- Credit 使用每份权益一行物化 `CreditValuation` 状态和移动加权平均单位价值；数量与价值必须由同一深模块、同一事务、同一行锁维护。
- 金额一律为整数 micros（`1 unit = 1,000,000 micros`）。比例运算采用防溢出的整数 `floor(a × b / d)`；估值路径禁止二进制浮点。
- CNY/USD 跨币种入账在入账时冻结不可变有理数 FX；拒绝其他币种，历史状态不动态重估。
- 请求扣除必须保存 request-specific 快照；异步任务携带 `subscription_request_id`，不得只靠 `subscription_id` 恢复价值。
- 历史重建只能标记 estimated/unknown，不能把不可证明的数据伪装为 exact；历史迁移和 `ready` 所有权只属于 #27。
- 门禁进入 `ready` 后，所有 Credit 数量写必须强制双写估值状态并失败关闭；接受强双写流量后禁止 image-only rollback。
- 冻结验收信号：`40 CNY / 1,000 Credit`，已消耗 200、可用 800，应得到 `32,000,000` micros CNY、`active_paid_subscription_count=1`、estimated=0、unknown=0，且 `end_time=0` 不影响结果。

## Issue 拓扑与所有权

- `#20`：前向精确价格转换、附加式 schema、只读非法值诊断；不回填历史、不修改 migration marker、不阻止 `ready`、不启用强制双写。
- `#21`：依赖 #20；计时 grant 时间线与多币种分析。
- `#22`：依赖 #20；32 CNY Credit 购买、消费、物化状态和五接口分析。
- `#23`：依赖 #22；同步/异步 `request_id` 与可逆结算。
- `#24`：依赖 #22；转换与售后正向 Credit 入账。
- `#25`：依赖 #23、#24；Credit 减少、退款、拒付和财务恢复。
- `#26`：依赖 #21、#22、#23；转换估值、FX 与在途请求结算。
- `#27`：依赖 #21–#26；历史价格/估值迁移、三数据库矩阵、`ready/failed/suspended` 门禁。
- `#28`：依赖 #27；不可变镜像、备份、切换、生产证据和强双写回滚边界。

子 Agent 只能实现当前 Dispatch 指定的 Issue。发现其他 Issue 的需求时，将具体接口缺口与建议写入进度文件并向协调器提问，不得越界实现下游切片。

## Skill 与实现方式

- 永久 feature 使用 `skill://tdd`：先写能证明可观察合同的失败测试，再实现，再重构。
- 修改 `web/default` 组件时先读 `skill://shadcn-ui`；新增或变更用户可见文本时读 `skill://i18n-translate`，维护 en、zh、fr、ru、ja、vi。
- 遇到不能从失败信息直接定位的并发、迁移、数据库或性能问题时读 `skill://diagnosing-bugs`，先复现再修复。
- 深模块边界出现新疑问时读 `skill://codebase-design`，但既有 ADR/spec 的业务决策不得擅自改写。
- 严守 Router → Controller → Service → Model 层次、`common/json.go` JSON 包装、SQLite/MySQL 5.7/PostgreSQL 9.6 兼容与可选零值指针合同。
- 修改导出符号前使用 LSP references；修改 UI 后必须真实驱动浏览器证明关键路径，而不能只靠组件测试。

## 可恢复进度协议

Agent 一开始就在自己的工作树创建并持续更新：

- `.scratch/agent-progress/issue-<N>/status.md`：当前阶段、已完成项、下一步、阻塞项、最近安全提交。
- `.scratch/agent-progress/issue-<N>/evidence.md`：执行过的命令、精确结果、失败和修复。
- `.scratch/agent-progress/issue-<N>/contract.md`：新增/修改的持久化、领域、API、UI 和迁移合同，以及给下游的接口。

这些文件用于意外中断后的原位恢复；每完成一个可编译或可验证的小步就落盘，并使用 Conventional Commits（英文 type/scope、简体中文 subject）提交安全点。不要把大段一次性脚本作为唯一实现载体；优先逐文件实现和小提交。若 Agent 意外退出，协调器先检查工作树、上述进度文件、Git 提交和 Orca Dispatch，再决定原位续作或 `retry-of`；运行时间长本身不是重派理由。

## 验证与交付

- Agent 运行当前 Issue 的定向测试、必要的数据库/UI smoke 与 `git diff --check`，记录证据；项目级全量回归由每波集成验收和最终阶段统一执行。
- 不得用 GORM DryRun 代替真实三数据库验收；#27 最终必须在 SQLite、MySQL 5.7.44、PostgreSQL 9.6.24 上同矩阵零 SKIP。
- 不得在生产插入临时用户、套餐、订阅或权益；#28 生产验证只读，除非已有明确授权的受控账号。
- 完成前确保工作树改动全部提交、工作树干净，并通过 Orca 当前 Dispatch 发送且只发送一次 `worker_done`，正文列出提交、修改、验证、遗留风险和进度目录。
- 协调器验收并集成后才关闭 Issue 和回收由本 Run 创建的工作树；Agent 不自行关闭 Issue、不部署生产、不回收父工作树。
