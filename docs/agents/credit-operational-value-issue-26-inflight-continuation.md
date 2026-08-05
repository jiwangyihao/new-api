# Issue #26 在途请求退款与并发续作 Agent 指令

## 目标与冻结现场

你负责父 PRD GitHub #19、子 Issue #26「固化转换估值、FX 与在途请求结算」中尚未完成的在途请求退款与转换/结算并发合同。你必须复用 Orca 工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`

开工前必须核对：

- 当前分支为 `jiwangyihao/issue-26-conversion-fx`；
- 当前 clean HEAD 为 `85660501ca95fae1fbcc6a1bff2fc07adf0424bd`；
- `git status --short` 无输出；
- Orca parentWorktreeId 严格指向 `credit-operational-value-integration`；
- 已验收实现祖先 `fd4d4683bc3b3b2cdd78c8e5c851c58263e61971` 仍为祖先；
- 已完成的 FX parser/向量、同币种与跨币种 conversion、权威事实冲突、同 source 并发幂等及最窄 reserve→conversion→final settle 提交链均完整存在。

先读取仓库与全局 `AGENTS.md`、父 PRD #19、Issue #26、`docs/agents/credit-operational-value-execution.md`、wave-3 contract、Issue #26 主指令/acceptance、`CONTEXT.md`、ADR 0002、新规格与计划相关章节，以及 `.scratch/agent-progress/issue-26/{contract,status,evidence}.md`。必须使用 `skill://diagnosing-bugs`、`skill://tdd` 与 `skill://codebase-design`；不要重新探索已冻结的 FX 或 conversion 设计。

## 本续作唯一范围

只完成以下两个仍缺的可观察合同，然后 clean HANDOFF：

1. `reserve → conversion → refund`：
   - 转换前已在 source timed 权益上 reserve 的 request 保留原 `subscription_id`、source timed window、request identity、PreConsumed/AppliedCredit 映射、冻结单位成本、FX、rule/version 与 rounding；
   - conversion 后第一次 refund/目标减少必须沿虚拟 exact deduction snapshot 恢复目标 Credit，不能把旧 request 重定向为一个新 Credit request，不能重复扣减或重复恢复；
   - 全退款必须带走该 request 的舍入余数；部分退款必须严格依照原请求快照；
   - 重放相同 refund 为无操作；目标冲突、mapping 缺失、状态缺失、溢出必须返回稳定错误且零写入；
   - conversion 后新 request 才正常选择 Credit。

2. `conversion ↔ final settle/refund` 真实双连接并发：
   - 使用真实文件 SQLite、WAL、两个独立连接和确定性 barrier；
   - conversion 与同一在途 request 的 final settle 或 refund 并发时，结果必须属于合法串行化集合；
   - 不允许 `SQLITE_BUSY`、唯一约束、自由文本错误泄漏到领域边界；
   - conversion/source/ledger/valuation/request snapshot 数量不得重复，request attribution 与 frozen facts 必须一致；
   - 相同调用重放稳定；不同 request 不互相污染。

## 明确非所有权

- 不实现 Issue #24 管理员/兑换跨币种 API、UI 或六语言；仅消费既有 FX seam。
- 不实现 Issue #25 destructive recovery、退款/拒付/财务恢复业务入口。
- 不实现 Issue #27 migration marker、ready/suspended、历史回填或真实三数据库矩阵。
- 不实现 Issue #28 构建、部署、备份或生产发布。
- 不新增第二套 request ledger、FX parser/provider、conversion 入口或动态重估。
- 不修改钱包布局、充值隐藏、Credit 激活或无关前端。
- 不运行全仓测试或部署；只运行本续作所需定向测试。

## TDD 与实现顺序

1. 第一项文件改动必须更新 `.scratch/agent-progress/issue-26/status.md` 与 `evidence.md`，将真实当前 HEAD `85660501c`、已完成/未完成边界和下一条命令写准确；必要时同步 `contract.md`，并立即提交恢复安全点。
2. 先写一条真实 SQLite public-path RED，走 `PreConsumeUserSubscriptionByUnits → ConfirmTimedSubscriptionConversion → SettleUserSubscriptionRequestTarget` 的退款/目标减少路径；必须是行为失败，不得用编译失败冒充。
3. 仅做最小 GREEN。复用 Issue #23 的 request record、累计目标、退款、absorbed/unknown 与虚拟 snapshot seam；复用 conversion 现有事务。若现有持久字段不足，先通过 Orca `ask` 列出最窄字段缺口，不自行扩 schema。
4. 将 refund 单次、`-count=10`、必要窄 `-race`、实体计数、失败零写入和重放证据写入 evidence，并立即小步提交。
5. 再编写真实文件 SQLite 双连接 deterministic barrier 测试，分别覆盖 conversion vs final settle、conversion vs refund。先观测旧实现 RED 或诚实记录直接 GREEN；如需修复，只做锁序、错误归一和事务原子性的最小改动。
6. 运行并记录单次、`-count=10` 与窄 `-race`。必须检查 conversion/source/ledger/state/request 计数及 attribution/frozen facts，而不只检查函数返回。
7. 运行既有 Issue #26 同币种、跨币种、事实冲突、同 source 并发及在途 final-settle tracer 的联合回归；运行 `gofmt`、`git diff --check`。
8. 更新 progress；提交全部代码和证据；确认 staged/unstaged/untracked 全零。
9. 发送一次有效 `worker_done --outcome succeeded`，主题明确写 `HANDOFF_READY`，正文明确：完成 refund 与双连接并发，但 Issue #26 的 API/UI/六语言/browser/final delivery 仍待后续 Agent。不要冒充整个 Issue 完成。

## 验收信号

交接前必须给出：

- refund RED 的精确旧行为与 GREEN 后结果；
- conversion/final 与 conversion/refund 两条并发测试的合法结果集合；
- 单次、count=10、窄 race 结果；
- conversion/source/ledger/valuation/request 的计数与 attribution 断言；
- 与已有同币种/跨币种/事实冲突/并发/final-settle 回归的组合 PASS；
- 修改文件和提交 SHA；
- clean tree 证据；
- 未实现 API/UI/六语言/browser、#24/#25/#27/#28、MySQL/PostgreSQL、全项目测试和部署的诚实声明。

任何异常先把现场和精确命令写入 `.scratch/agent-progress/issue-26/` 并提交；只有工具或契约真正阻塞才使用 Orca question/escalation。不要在终端里保留唯一进度。