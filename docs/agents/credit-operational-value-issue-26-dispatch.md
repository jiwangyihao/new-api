# Issue #26 调度补充与恢复入口

## 冻结开工基线

Issue #26 的已验收实现祖先固定为 `fd4d4683bc3b3b2cdd78c8e5c851c58263e61971`，该提交已经集成并关闭 Issues #20、#21、#22、#23。新子工作树的 `HEAD` 必须等于**创建当时**父集成分支 `jiwangyihao/credit-operational-value-integration` 的 tip；`fd4d4683...HEAD` 之间只允许包含调度协议、Agent 指令、验收清单和恢复记录等上下文提交，不要求子工作树 `HEAD` 恰好等于 `fd4d4683`。

父工作树固定为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`

Orca `parentWorktreeId` 必须严格等于该父工作树的完整 `<repo-id>::<path>` id，父分支必须为 `jiwangyihao/credit-operational-value-integration`。不得从 `origin/main`、仓库根工作树、生产提交、Issue #24 冻结分支或任何旧 Worker 分支派生。开工后必须记录：父集成 tip、子树 `HEAD`、`merge-base(fd4d4683, HEAD)`、`fd4d4683..HEAD` 提交列表、完整 Orca parent id 与干净状态。若子树 `HEAD` 不等于创建时父 tip、`fd4d4683` 不是祖先、调度区间混入运行时代码，或 lineage 不符，必须在修改任何文件前立即停止并报告。

Issue #21、#22、#23 已完成并关闭。Issue #24 尚未集成，其冻结实现只完成无 FX 的正向入账；不要把 Issue #24 合入本分支，也不要复制它的管理员/兑换入口。Issue #26 的硬依赖已经满足，它负责提供唯一 FX 与转换 seam；完成并集成后，协调器才会恢复 Issue #24 消费该 seam。

## 必读顺序

除仓库及全局规则外，按顺序读取：

1. `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/26`；
2. `docs/agents/credit-operational-value-execution.md`；
3. `docs/agents/credit-operational-value-wave-3-contract.md`；
4. `docs/agents/credit-operational-value-issue-26.md`；
5. `docs/agents/credit-operational-value-issue-26-acceptance.md`；
6. `CONTEXT.md`、ADR 0001、ADR 0002、新规格与实施计划中上述指令指定章节；
7. 已集成的 `.scratch/agent-progress/issue-20`、`issue-21`、`issue-22`、`issue-23`；
8. 只读参考 Issue #24 的 `.scratch/agent-progress/issue-24/contract.md` 中“跨币种接缝需求”，不得修改 Issue #24 工作树。

必须使用 `skill://tdd`。转换、并发、锁序或幂等出现异常时使用 `skill://diagnosing-bugs`；调整跨模块 seam 前使用 `skill://codebase-design`。修改 `web/default` 前使用 `skill://shadcn-ui`，新增或修改可见文案前使用 `skill://i18n-translate`，覆盖 en、zh、fr、ru、ja、vi。

## 唯一 FX seam

Issue #26 独占从持久化 Option 原始十进制文本解析、校验、约分并原子发布运行时 `CreditFXRateSnapshot` 的职责。不得读取、运算或反推 `float64 USDExchangeRate`。同币种固定为 1/1；只支持 CNY/USD；语义固定为 `1 USD = numerator / denominator CNY`。

为后续 Issue #24 消费者提供一个唯一、不可变、可结构化持久化的窄接口，至少表达：

- `source_currency` 与 `valuation_currency`；
- 正整数 `numerator`、`denominator`；
- 正 `captured_at`；
- `floor(source_amount_micros × numerator / denominator)` 的 overflow-safe 转换；
- 缺失、零/负值、币种方向不匹配、不支持币种、非法原始文本、超精度与溢出的稳定 sentinel/code。

普通 Credit ingress 可以携带冻结快照，但 #26 不得实现管理员 increase、兑换 API/UI 或它们的幂等生命周期。若必须扩展 #22 的 `CreditValuationSourceSnapshot`，只做向后兼容附加字段和窄构造器，并先把符号、调用点和不变部分写入 `contract.md`；不要复制第二套 FX 类型、parser 或 provider。

## 实施顺序与持久化

第一项实际修改必须创建并提交：

- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`
- `.scratch/agent-progress/issue-26/contract.md`

`contract.md` 必须先冻结转换数量公式、FX 方向、结构化字段、幂等指纹、锁序、在途请求状态机、稳定错误、UI DTO、共享文件与明确非所有权。每个 RED、最小 GREEN、并发修复、API/UI 小步均立即更新进度并用 Conventional Commits 提交。不要把关键结论只留在终端、临时脚本或未提交 diff 中。

建议按以下可恢复安全点推进：

1. 严格有理数 FX parser、原子快照发布与向量测试；
2. 转换数量/冻结精确估值字段与 quote RED→GREEN；
3. confirm 事务、转换 ingress、ledger、converted 状态、活动接替、幂等与故障全回滚；
4. 转换期间在途请求的虚拟 exact 快照、少结算/追加/退款/重放与并发；
5. API、UI、六语言；
6. 真实 SQLite tracer、真实应用/API/browser、可控请求 smoke、窄 race 与最终清理。

每个安全点提交后更新 `status.md` 的最近安全 SHA、下一条命令、未提交文件和已知风险。上下文接近上限、模型/网络异常或工具卡住时，先持久化并提交可恢复现场，再发送 escalation；不要重复探索或重开健康 Worker。

## 严格范围边界

只实现 timed→Credit 转换估值、运行时 FX 快照和转换期间在途请求桥接。必须复用 #21 timed grant、#22 Credit 混合池/ingress/analytics、#23 request identity/累计目标/cleanup。禁止：

- 修改 Issue #24 的 redemption/admin increase 或它的 UI；
- 实现 Issue #25 的 decrease/refund/chargeback/recovery；
- 实现 Issue #27 的历史回填、迁移命令、marker/ready/failed/suspended；
- 实现 Issue #28 的镜像、备份、部署或生产操作；
- 改变 31 天业务月公式、按秒折算部分周期、动态重估历史、恢复 `model_limits`；
- 把转换记为新增收款或邀请收入；
- 用匿名 delta 绕过 #23 request-aware 入口；
- 直接修改数量后补写估值状态。

新转换继续拒绝 disabled、trial、invitation 与不合格计划；已有 disabled-plan 权益继续消费的合同必须保留。

## 验收与完成

只运行本切片定向测试和必要 smoke，不运行全仓测试或部署。最终至少需要：

- 真实 SQLite quote→confirm→conversion/ledger/目标 Credit/五接口 tracer；
- CNY→USD、USD→CNY、同币种、约分、floor、非法/非正/不支持/溢出 FX 向量；
- 事务失败点全回滚、幂等重放/冲突、conversion+settlement 并发与窄 `-race`；
- 转换期间少结算、追加、退款和重复终态；
- 本地真实应用/API/browser 对精确估值、FX 提示和稳定错误的验证；
- 受影响前后端定向测试、typecheck、build、六语言 missing/extras=0、`git diff --check`；
- 明确记录 MySQL/PostgreSQL 未实测并保留给 Issue #27。

完成前逐条映射 GitHub Issue #26 与独立验收门禁，保持工作树干净，只发送一次有效 `worker_done`。不要关闭 Issue、合并、部署或回收工作树。
