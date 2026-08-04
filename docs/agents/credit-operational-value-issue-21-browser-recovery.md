# Issue #21 浏览器与最终交付续作 Agent 指令

## 目标

在既有隔离工作树 `C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants` 中，从 clean HEAD `2f9701976282d1c53d7ce0914088a302498f6f32` 恢复 Issue #21 的最后一段交付。前任 Agent 已完成领域入口、不可变 timed grant、订单/兑换/管理员授予、五个运营分析端点、跨币种前端展示与六语言；你不得重新设计或重做这些部分。

本次唯一工作是：生成 Go embed 所需的真实前端产物，启动隔离应用，使用真实浏览器完成管理员 timed grant 与 CNY/USD 跨币种展示 smoke，执行既定最终窄门禁，清理临时资源，提交证据并以 clean tree 发送 `worker_done`。

## 必读资料与 Skill

开始后立即读取：

1. `.scratch/agent-progress/issue-21/status.md`
2. `.scratch/agent-progress/issue-21/evidence.md`
3. `.scratch/agent-progress/issue-21/contract.md`
4. `docs/agents/credit-operational-value-execution.md`
5. `docs/agents/credit-operational-value-wave-1-contract.md`
6. `docs/agents/credit-operational-value-issue-21.md`
7. `docs/agents/credit-operational-value-issue-21-acceptance.md`
8. 父 PRD `issue://jiwangyihao/new-api/19` 与当前 Issue `issue://jiwangyihao/new-api/21`

浏览器与 Orca 操作先读取 `skill://orca-cli`；若必须处理桌面窗口、防火墙或焦点，再读取 `skill://computer-use`。若启动、登录、构建或真实请求失败，读取 `skill://diagnosing-bugs`，从根因诊断，不用静态拦截、伪响应或跳过真实 API。不要为本次纯验收重新展开 TDD 或设计探索。

## 已冻结的恢复事实

- 当前 HEAD 必须为 `2f9701976282d1c53d7ce0914088a302498f6f32`，开始时工作树必须 clean。
- 已完成代码与验证链至少包含：
  - `f812e77f` timed grant 领域/分析基线；
  - `8e143ca77` 管理员 timed grant UI；
  - `1809124c5` 跨币种运营剩余价值展示；
  - `5ea548998` 六语言；
  - `1481d4f97` 强五接口 API tracer。
- 不得重写这些提交、reset、rebase、stash 或扩大范围。
- 首次 `go run .` 只因 `web/default/dist` 与 `web/classic/dist` 缺失而在 Go embed 编译阶段失败；这不是产品失败，也不能算浏览器证据。
- 使用隔离端口 `31021`、隔离 SQLite、隔离 session secret；不得连接或写入生产数据库。

## 持久化与中断恢复

在任何耗时构建或浏览器操作前，先更新并提交以下现有文件中的续作状态：

- `.scratch/agent-progress/issue-21/status.md`
- `.scratch/agent-progress/issue-21/evidence.md`

记录当前 HEAD、clean 状态、使用的端口、临时数据库路径、构建命令、服务名和下一动作。每完成一个可审计阶段立即追加证据并小步提交。不要把关键状态只留在终端滚屏；不要运行大段临时脚本。临时数据、截图、请求记录放在 `.scratch/agent-progress/issue-21/browser/`，最终只提交小型文本证据，不提交数据库、dist、截图二进制或依赖目录。

若上下文接近上限，立即写 `HANDOFF_READY`、提交所有有价值文本和代码、保持服务信息可恢复，并通知协调器；不得在未持久化时继续探索。

## 严格执行步骤

### 1. 恢复真实应用

- 在 `web/default/` 使用 Bun 按仓库约定生成真实 production dist。
- 在 `web/classic/` 生成 Go embed 所需真实 dist；若既有 lock/package 漂移导致 frozen install 失败，准确记录事实，使用不修改锁文件的最保守现有依赖恢复方式。不得提交 lockfile 漂移。
- 确认两个 dist 均存在后，以命令内显式环境变量启动隔离后端：`PORT=31021`、独立 `SQLITE_PATH`、独立 `SESSION_SECRET`。长驻进程必须用 Orca/hub 管理，记录稳定服务名。
- 通过健康端点证明服务实际监听 `127.0.0.1:31021`；不要把进程创建当 readiness。

### 2. 创建隔离管理员与真实数据

- 通过真实 `POST /api/setup` 创建隔离 root，再通过真实 `/api/user/login` 建立浏览器 session。
- 使用公开 API/现有领域入口建立浏览器所需的最小用户、启用的有价 timed 计划，以及 CNY/USD 两条不可变 grant 数据。数据只存在隔离 SQLite。
- 不得直接插入 grant/分析结果绕过领域入口；允许通过 UI/API 创建前置业务事实。

### 3. 真实管理员 timed grant smoke

在 default UI `/users` → 用户行操作 → User Subscription Management 中，观察真实 `POST /api/subscription/admin/users/:id/subscriptions`：

- 计划选择仅出现启用、非试用、非邀请试用、timed 且具有正精确价格的计划。
- payload 必须包含 `plan_id`、非空 `reason`、`idempotency_key`、`source_price_micros`、`source_currency`；价格/币种来自选中计划的冻结事实。
- 人为制造一次可控失败；记录第一次失败 payload/key。
- 不改变任何授予事实，点击重试，证明第二次请求复用同一 key。
- 修改 reason 或在成功后重新授予，证明新 attempt 使用新 key。
- 成功后通过真实 API/数据库可观察到权益与唯一 immutable grant；不得只看 toast。

### 4. 真实跨币种分析 smoke

在 `/admin-analytics` 的 paid-subscription-value 面板中，使用真实五个 `/api/admin-analytics/paid-subscription-value/**` 请求证明：

- 同一卡片/明细显示 CNY 与 USD 的 `*_by_currency` 值；
- 跨币种 subscription 的三个兼容 singular 均为 `null`/界面不显示伪造单币种值；
- 当前 Plan 币种不得补猜或覆盖 grant 原币种；
- confidence、warning、unknown timed 数量按真实响应展示；
- summary/users/subscriptions/plans/sources 的金额可对账。

保留请求 URL、关键真实响应字段、可见 UI 文本与操作结果。禁止拦截 JSON、mock fetch 或注入静态资源来冒充端到端行为。

### 5. 最终窄门禁

浏览器通过后，按 acceptance 文档执行并记录精确命令/结果：

- timed grant 领域 SQLite 定向测试；
- 五接口 timed grant/缺口/重叠/裁剪 tracer；
- 管理员 grant controller 测试与强五端点 API tracer；
- `user-subscriptions-dialog` 与 `panel-fields` 定向前端测试；
- `bun run typecheck`；
- 六语言 i18n sync，必须 missing/extras=0 且同步后无意外 diff；
- 需要时 production build；
- `git diff --check`。

不得宣称未运行的 MySQL/PostgreSQL 通过；真实三数据库零 SKIP 属于 #27。

### 6. 清理与交付

- 关闭浏览器 tab；停止本 Agent 创建的服务；删除隔离 SQLite、临时 secret、非提交 dist/临时文件。
- 更新 `status.md` 为 COMPLETE，补齐 `evidence.md` 的浏览器 payload/响应、最终门禁和清理证据。
- 提交所有有价值文本/必要代码，最终 `git status --short` 必须为空。
- 通过 Orca `worker_done` 报告最终 HEAD、提交清单、真实浏览器结论、测试结果、清理状态和任何明确 SKIP。

## 禁止范围

- 不修改 CreditValuation 核心、Credit 购买/消费、转换、FX、request settlement、migration marker/ready、历史回填或生产发布。
- 不实现 #22–#28 的合同。
- 不改变 disabled-plan 既有权益消费边界。
- 不关闭 GitHub Issue、不合并分支、不修改集成父树。
- 不以组件测试、静态页面、拦截响应或数据库直接造分析结果代替真实浏览器/API smoke。

## 验收结果

只有真实浏览器两条 smoke、最终窄门禁、资源清理、持久化证据、提交和 clean tree 全部完成，才能发送 succeeded。若环境仍阻断，必须给出可复现命令、原始错误、已尝试动作、保留服务/数据位置和下一恢复动作；不得把阻塞包装成完成。