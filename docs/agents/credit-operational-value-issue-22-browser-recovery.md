# Issue #22 真实浏览器最终验收续作指令

## 唯一目标

你接管父 PRD #19、GitHub Issue #22 的既有隔离工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-22-credit-tracer`

本次不是继续实现业务功能。该工作树在恢复前必须满足：

- HEAD 严格等于 `3742bea5e5a4ea9acc20b0641923b7ba5c32fbf8`；
- `git status --short` 为空；
- Gate C、真实 SQLite 五接口、默认前端 17/17、typecheck、production build、六语言均已有 GREEN 证据；
- 唯一未完成项是真实应用、真实 SQLite/API、真实浏览器页面的最终 smoke，以及随后资源清理和 `worker_done`。

你只能完成这个最后的浏览器验收闭环。除修正纯验收夹具/启动证据外，不得修改业务代码、测试合同、数据库 schema、支付或结算逻辑。

## 必读资料与 Skill

开始后立即读取：

1. `.scratch/agent-progress/issue-22/status.md`
2. `.scratch/agent-progress/issue-22/evidence.md`
3. `.scratch/agent-progress/issue-22/contract.md`
4. `docs/agents/credit-operational-value-execution.md`
5. `docs/agents/credit-operational-value-wave-1-contract.md`
6. `docs/agents/credit-operational-value-issue-22.md`
7. `docs/agents/credit-operational-value-issue-22-acceptance.md`
8. `docs/agents/credit-operational-value-issue-22-gate-c-recovery.md`
9. 父 PRD `issue://jiwangyihao/new-api/19` 与当前 Issue `issue://jiwangyihao/new-api/22`

浏览器和 Orca 操作先读取 `skill://orca-cli`。若出现进程、端口、登录、SQLite、构建、焦点或防火墙问题，读取并使用 `skill://diagnosing-bugs`；需要操作桌面提示时再读取 `skill://computer-use`。不要重新展开 TDD、架构设计或实现探索。

## 中断恢复与持久化

任何服务启动或浏览器操作前，先在以下文件追加本次续作状态并提交一个小型恢复点：

- `.scratch/agent-progress/issue-22/status.md`
- `.scratch/agent-progress/issue-22/evidence.md`

记录当前 HEAD、工作树 clean 状态、端口 `3112`、隔离数据库相对路径、服务名、构建产物状态和下一动作。关键 API 响应、浏览器观察与清理结果必须逐步写入 `evidence.md`；不得只留在终端滚屏。文本请求/响应摘录可放在 `.scratch/agent-progress/issue-22/browser/`，不得提交 SQLite、截图二进制、dist、node_modules 或 secret。

若上下文接近上限，立即写 `HANDOFF_READY`、提交全部有价值文本、停止服务并通知协调器；不得在未落盘时继续探索。

## 严格执行步骤

### 1. 启动真实隔离应用

先确认 `web/default/dist` 与 `web/classic/dist` 已存在。若被清理，只按已记录方式重建：

- `web/default` 使用 Bun 生成真实 production dist；
- `web/classic` 若既有 frozen lock 漂移仍存在，诚实记录，并仅使用不会提交 lockfile 漂移的 `bun install --no-save` 与现有 build；
- 构建后确认跟踪文件和 lockfile 无变化。

长驻服务必须使用受监督进程。Windows 下必须通过 `cmd.exe` 命令内显式注入环境变量，不要依赖外层 `env` 静默传递：

`cmd.exe /d /s /c "set PORT=3112&& set SQLITE_PATH=.scratch/agent-progress/issue-22/browser-smoke.db&& set SESSION_SECRET=issue22-browser-only-session-secret&& go run ."`

使用稳定服务名，例如 `issue22-browser-final`。readiness 必须同时证明：

- 日志明确显示应用 ready；
- `127.0.0.1:3112` 可连接；
- 真实 `GET /api/status` 返回成功；
- `.scratch/agent-progress/issue-22/browser-smoke.db` 实际存在。

不得把进程已创建、默认 3000 端口响应或静态文件存在当成通过。

### 2. 建立隔离管理员与真实领域数据

通过真实 `POST /api/setup` 创建隔离 root，再通过真实 `/api/user/login` 建立浏览器 session。只使用当前工作树中的公开 API、现有领域入口和隔离 SQLite 创建最小验收数据：

- 全局零价 Credit 容器，估值币种 CNY；
- 有价充值档位 `40 CNY / 1,000 Credit`；
- 通过真实人民币余额入口或已实现的受控购买入口完成入账；
- 通过真实 BillingSession/request_id 同步消费并最终化 200；
- 最终状态必须为 available=800、exact/recognized=`32,000,000` micros CNY、estimated=0、unknown=0、active paid count=1。

不得直接插入 `CreditValuationState`、分析结果或伪造响应。测试前置 marker 仅可按已有隔离测试/维护前置构造；生产代码不得创建、CAS 或切换 marker/ready。

### 3. 真实浏览器五接口 smoke

使用真实受控 Chromium 打开当前应用的 default UI，登录隔离管理员并进入运营分析付费套餐剩余价值页面。禁止 request interception、mock fetch、静态 JSON 注入或直接改 DOM。

捕获并核对真实五个 API：

- summary
- users
- subscriptions
- plans
- sources

真实响应和页面必须共同证明：

1. recognized/exact 为 `32,000,000` micros CNY，页面显示 `¥32.00`；
2. `active_paid_subscription_count=1`；
3. Credit 明细的 `time_based_value=null`，页面显示本地化的“时间值不适用”；
4. `valuation_basis=credit_moving_weighted_average`，页面显示移动加权平均语义；
5. `confidence=exact` / Exact 文案可见；
6. 当既有 tracer 场景返回 `snapshot_semantics=current_only` 时，页面显示非阻断 current-only 提示；
7. summary/users/subscriptions/plans/sources 的金额和可用 Credit 800 可相互对账；
8. full reload 后结果仍一致。

记录页面 URL、操作步骤、关键请求 URL、HTTP 状态、权威 `amount_micros` 字符串、可见 UI 文本和刷新后结果。若 current-only 需要独立的真实数据时序，必须通过现有真实领域入口形成，不得修改生产代码或直接写分析 DTO。

### 4. 最终窄复核

浏览器通过后，仅复跑必要窄门禁，不跑全仓大套件：

- 已有 32 CNY 五接口 SQLite tracer；
- 人民币余额、Kyren、BillingSession Gate C 定向测试；
- `format`、`panel-fields`、`paid-value-panel` 三个前端定向文件；
- `bun run typecheck`；
- 六语言 i18n missing/extras=0；
- 必要时 production build；
- `git diff --check`。

真实 MySQL/PostgreSQL 没有 DSN 就明确记为未运行；三数据库零 SKIP 归 Issue #27。

### 5. 清理与交付

- 关闭本次浏览器 tab；
- 停止本次创建的 `issue22-browser-final` 服务；
- 删除 `browser-smoke.db`、临时 secret/请求文件和非提交产物；
- 更新 `status.md` 为 COMPLETE，补齐 `evidence.md` 的真实 API/UI、刷新、窄门禁和清理证据；
- 提交所有有价值文本，最终 `git status --short` 必须为空；
- 只发送一次有效 `worker_done`，包含最终 HEAD、提交列表、真实浏览器结论、测试结果、清理状态与明确 SKIP。

## 严格禁止范围

- 不修改 CreditValuation 深模块、购买/支付/BillingSession 业务实现或分析 DTO；
- 不实现 Issue #23 的 target 增减、退款、异步 identity 或 coalescer；
- 不实现 Issue #24–#28；
- 不实现 FX；不创建/CAS/切换 migration marker 或 ready；
- 不关闭 GitHub Issue、不合并分支、不部署、不回收工作树；
- 不用组件测试、API 单测、静态资源拦截或直接数据库造分析结果替代真实浏览器验收。

只有真实浏览器 smoke、窄复核、资源清理、持久化证据、提交与 clean tree 全部完成，才能报告 succeeded。环境仍阻断时必须持久化准确命令、原始错误、已尝试动作和下一恢复点，不得包装成完成。
