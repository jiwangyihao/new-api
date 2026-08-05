# Issue #26 API、浏览器与最终交付续作 Agent 指令

## 任务目标

你负责父 PRD GitHub Issue #19、子 Issue #26「固化转换估值、FX 与在途请求结算」最后一个受监督交付步骤。必须复用现有 Orca 工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`

冻结起点必须是 clean HEAD `c709ccb2c375031eabf43703334dffd44b39856a`。此前 Task 已完成 FX parser/非法输入/双向冻结/整数 floor-overflow、同币种与 CNY↔USD conversion、权威事实冲突、并发幂等、转换期间在途 final/refund 与双连接合法串行化。不要重新实现、改写或“优化”这些已通过合同。本任务只补可观察 API/browser 证据、最终回归、资源清理和可审计交付。

## 必读资料与技能

开始后先核对工作树 clean、HEAD 和 Orca parent，再读取：

- GitHub 父 PRD `jiwangyihao/new-api#19` 与子 Issue `#26`；
- `CONTEXT.md`；
- `docs/adr/0002-credit-operational-remaining-value.md`；
- `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`；
- `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md`；
- `docs/agents/credit-operational-value-execution.md`；
- `docs/agents/credit-operational-value-wave-3-contract.md`；
- `docs/agents/credit-operational-value-issue-26.md`；
- `docs/agents/credit-operational-value-issue-26-acceptance.md`；
- `.scratch/agent-progress/issue-26/{contract,status,evidence}.md`。

按阶段使用 `diagnosing-bugs`；若发现必须改代码，必须使用 `tdd`，涉及既有模块接口边界时使用 `codebase-design`；只有确实存在 #26 可见 UI 时使用 `shadcn-ui` 与 `i18n-translate`。不要派生子 Agent，不要运行项目全量测试。

## 冻结合同

必须保留以下已完成行为：

1. FX 只支持 CNY/USD；同币种严格 `1/1`；持久化 Option 的原始十进制不得经 `float64`，快照冻结正的 numerator/denominator/captured_at。
2. 金额以整数 micros 和 overflow-safe floor 计算；不得动态重估既有 conversion、ledger 或 Credit state。
3. 数量公式保持 `full_31_day_blocks × credit_basis + current_remaining_credit`；31 天是业务月，不做部分周期按秒折算。
4. conversion 冻结权威 Plan 精确价、币种、basis、gross/net Credit、成本、规则版本与 FX；同事实重放幂等，权威事实冲突稳定拒绝。
5. conversion、Credit ingress/ledger、source converted、目标 state 与活动接替保持原子。
6. 转换前已 reserve 的 timed request 保留原 `subscription_id`、窗口、扣除、成本、FX、rounding；final/refund 使用虚拟 exact snapshot，不重复扣目标池；转换后新 request 才进入 Credit。
7. 保留 #20–#23 已集成合同，尤其 #23 request identity、累计 target、coalescer、Task identity 与 cleanup；不得恢复匿名 Credit delta。

## 允许的改动

### 一、API 可观察性

使用现有 quote、confirm、history、conversion detail 和五个运营分析 API 完成真实 SQLite tracer。若现有 DTO 无法返回 Issue #26 已要求且已经持久化的结构化事实，可以做最小纵向补齐：

- 字符串 micros；
- source/target currency；
- FX numerator/denominator/captured_at；
- 未舍入有理数单位价值的分子/分母或项目规格要求的现有结构；
- rule/state version；
- 原 source subscription 与目标 valuation subscription attribution；
- 稳定错误 code。

不得新建第二套 conversion API、第二套 FX parser/provider、动态重估端点或 #24 管理 ingress API。

### 二、真实应用与浏览器

先判断 Issue #26 的现有产品路径：如果现有钱包 conversion dialog/history/analytics 已展示转换，则必须从当前工作树构建真实前端和 Go embed 产物，以命令内显式环境启动隔离 SQLite 应用，使用真实 Chromium 完成：

- quote 显示 31 天业务月；
- confirm 后显示“规则确值但不是新增收款”；
- 显示冻结精确价格/币种/FX，后续改价或汇率不回写；
- 原始数量与 micros 请求不被紧凑展示改写；
- history/analytics 刷新后仍一致；
- 一条可控在途少结算或退款保持原 subscription attribution。

如果 #26 在当前产品中确实没有任何可见 UI，必须用代码与路由证据证明，并把真实 API 作为可观察终点；禁止为了“有 browser”而新增无规格依据的 UI。仍需启动真实应用并通过浏览器或真实 HTTP 路径验证现有入口、错误映射与页面无回归。

### 三、最终验证与清理

至少运行并持久化：

- FX A/B/C 向量的定向测试；
- 同币种与 CNY↔USD conversion、权威事实冲突、同 source 并发幂等；
- 在途 final/refund 与双连接交错；
- #20 精确价格、#21 timed grants、多币种分析、#22 32 CNY Credit、#23 普通 request restore 的相关定向回归；
- 受影响 `model/service/controller` 包测试，不运行全项目测试；
- 必要窄 `-race`；
- 相关前端测试、typecheck、build 与六语言 missing/extras（仅当前切片触及前端/i18n 时）；
- `git diff --check` 和 clean tree。

MySQL 5.7/PostgreSQL 9.6 的零 SKIP 实机门禁归 #27；不得冒充已运行。生产部署归 #28。

## 禁止事项

- 不实现 #24 redemption/admin increase 或其 UI；
- 不实现 #25 decrease/refund/chargeback/recovery；
- 不实现 #27 migration marker、ready/suspended 或历史回填；
- 不执行 #28 发布；
- 不新增 schema，除非 API 缺口证明现有已持久化字段不足，并先向协调器 escalation；
- 不改变 conversion 数量公式、disabled-plan 既有消费边界、`model_limits` 忽略合同；
- 不把 mock、直接插表或静态拦截当真实主路径证据；
- 不把未运行测试写成 PASS。

## 恢复与提交协议

持续更新并提交：

- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`
- `.scratch/agent-progress/issue-26/contract.md`

每个可验证小步立即使用 Conventional Commit 提交。每次停在可恢复点时写清最近 clean SHA、当前阶段、下一条命令、未提交文件和阻塞。服务、浏览器、临时 SQLite、secret 与构建临时目录在交付前全部清理；不要删除可审计 progress。

## 完成条件

只有同时满足以下条件才能发送 `worker_done --outcome succeeded`：

- Issue #26 Gate A–G 有逐项证据；
- API 与必要 browser 真实主路径完成；
- 聚焦测试/race/build/i18n/diff-check 通过；
- 服务、tab、临时 DB 清理；
- 工作树 staged/unstaged/untracked 全零；
- 最终 progress 提交已列出 SHA、命令、关键输出、未运行范围和共享文件风险。

如果发现业务 blocker，保留 clean/可恢复现场后发送 escalation，不得用降级断言、跳过或假证据绕过。
