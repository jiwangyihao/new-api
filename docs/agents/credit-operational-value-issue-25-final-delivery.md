# Issue #25 API/UI/分析/六语言/浏览器最终续作 Agent 指令

## 任务目标

你在唯一正确的 Orca 工作树 `C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-25-destructive-outflow` 中继续 GitHub `jiwangyihao/new-api#25`。当前 clean HEAD 必须是 `a65399f1586952f2895080442c026924fc90c633`，或只包含本续作随后产生的提交；共同实现基线与 merge-base 是 `fe1901aaf7a769fe7057c6483e30b7b1491adcdc`；Orca parent 必须严格指向 `credit-operational-value-integration`。禁止 reset、rebase、checkout、改分支、另建工作树、丢弃或覆盖 A/B/C/D/E 成果。

A/B/C/D/E 领域核心已完成并有 clean 提交：管理员 decrease 混合池 outflow、清空余数/欠额/幂等/回滚；订单 refund/chargeback/financial recovery 的 immutable purchase facts 与终态重放；请求冻结快照恢复；邀请取消隔离；四类真实 SQLite WAL 双连接并发和稳定锁重试。你不得重做核心算法。本任务负责把这些能力贯通到真实管理员 API/UI、五个运营分析接口、六语言和真实 SQLite/Chromium 主路径，完成可审查的最终交付。

## 必读材料与 Skill

开始后依次读取并遵守：

1. 自动注入的仓库及全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19` 与 `issue://jiwangyihao/new-api/25`；GitHub CLI 始终显式传 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`、`docs/agents/credit-operational-value-wave-3-contract.md`、`docs/agents/credit-operational-value-issue-25.md`、`docs/agents/credit-operational-value-issue-25-acceptance.md`、`docs/agents/credit-operational-value-issue-25-cde-continuation.md`。
4. `.scratch/agent-progress/issue-25/{contract,status,evidence}.md` 与 Git 提交链，尤其 `92482861f`、`90e6f3c80`、`d6fdcd45c`、`e3e7f4e60`、`e86fe7ba3`、`845758872`、`19c6440dd`、`a65399f15`。
5. `CONTEXT.md`、ADR 0002、2026-08-02 specification/plan 中管理员 decrease、recovery、分析与 UI 章节。
6. 先读取并执行 `skill://tdd`。遇到真实行为异常读取 `skill://diagnosing-bugs`；修改 UI 前读取 `skill://shadcn-ui`；新增/修改可见文案前读取 `skill://i18n-translate`；深模块边界不清时读取 `skill://codebase-design`，不得推翻既有 ADR/spec。

## 恢复与提交纪律

第一项动作是核对 HEAD、clean、merge-base、Orca lineage 和 A–E 提交。然后更新并提交：

- `.scratch/agent-progress/issue-25/status.md`：阶段设为 `FINAL_API_UI_ANALYTICS_IN_PROGRESS`；
- `.scratch/agent-progress/issue-25/evidence.md`：记录本续作 RED/GREEN、API、分析、浏览器与最终门禁；
- `.scratch/agent-progress/issue-25/contract.md`：仅在实际 API/UI 合同需要补充时追加，不重写核心合同。

每个可验证小步立即 Conventional Commit，subject 使用简体中文。严禁把关键成果只留在终端、临时 DB 或未提交 diff。最终必须 staged/unstaged/untracked 全零，并通过当前 Dispatch 发送一次有效 `worker_done succeeded`；不要合并、关闭 Issue 或删除工作树。

## 阶段一：管理员 decrease API 与稳定响应

复用 #24 已有管理员 Credit adjustment API，不新建平行接口。以真实路由测试先 RED 后 GREEN，证明：

- `decrease` 请求要求正整数 Credit amount、非空 reason、稳定 idempotency key；任何 `plan_id`（包括 0、空字符串转换后的值或旧 UI 残留）均返回稳定 machine code 并整笔零写入。
- Controller/service 只转发领域事实，不直接修改 `UserSubscription` 或 `CreditValuationState`，不解析错误文本。
- 响应必须从已提交结构化 ledger/领域结果投影：gross Credit、consumed available、debt formed、removed exact/estimated/unknown、currency、state_version_after；所有 micros 使用十进制字符串，兼容 float 不参与算术。
- 同 key/同完整参数重放原结果；operation、amount、reason 或其他指纹变化稳定冲突；可控失败后同事实重试复用同一 key。
- 至少一个真实订单 recovery API 入口能观察 refund/chargeback/financial recovery 的稳定终态、幂等和精确响应；不得用直接插表冒充主路径。

如既有 API 已满足合同，写真实行为测试证明并避免制造生产改动。完成后运行定向 route/controller/model 测试、`count=10`、必要窄 race、gofmt、`git diff --check`，更新 evidence 并 clean 提交。

## 阶段二：五个运营分析接口

通过公开管理员 adjustment API 和至少一个真实订单 recovery 入口执行破坏性 outflow，然后读取五个 paid-value 分析 API（summary/users/subscriptions/plans/sources）。必须证明：

- outflow 后 available、exhausted、settlement debt、exact/estimated/unknown 和 active paid count 立即一致；零可用量或仅 debt 不计 active paid count。
- removed exact/estimated/unknown 使用操作前移动平均池，不按订单价格/实收/退款额撤值。
- refund→chargeback、同事实重放、request snapshot restore 不重复计价、不新增 order/invitation attribution；Credit recovery 不进入邀请付费统计。
- CNY/USD `*_by_currency`、字符串 micros、confidence、warning/current-only 语义与已集成 #22–#24 合同保持兼容；不得新增第二套分析 DTO 或用 float 重算。
- timed 多币种分析不因 Credit recovery 改变；#20 精确价格、#22 32 CNY tracer、#23 request restore、#24 ingress/debt 代表性测试继续通过。

先写真实 SQLite/API tracer RED，再做最小 GREEN。若核心已直接 GREEN，诚实记录并只迁移过时夹具。形成独立 clean 提交。

## 阶段三：管理员 UI 与六语言

在既有管理员 Credit adjustment 面板中完成 decrease 交互，不新建第二套页面：

- 从 increase 切换到 decrease 时隐藏并清空 plan、价格、FX/preview 状态；实际 payload 不含 `plan_id`。切回 increase 时也不能泄漏旧 decrease 状态。
- decrease 显示正 amount、reason 与幂等 key 生命周期；失败且业务参数不变的重试复用同一 key，成功或 amount/operation/reason 改变后生成新 key。
- 响应展示 gross、consumed available、debt formed、removed exact/estimated/unknown、currency、state version；使用现有 BigInt/十进制 micros formatter，不把原始值转成 JavaScript Number。
- 明确提示超出可用量形成“结算欠额”，exact/estimated/unknown 是“运营剩余价值/成本置信度”，不得称为现金退款、负债或可退金额。
- 业务错误按稳定 code 映射，不解析自由文本。

为所有新增/变更键补齐 en、zh、fr、ru、ja、vi，并运行项目 i18n sync，要求 missing/extras=0 且无无关全量重排。使用现有前端测试习惯写 observable 行为测试：increase→decrease 清空 plan/preview、payload 无 plan_id、key 重试/更新、精确字符串展示。运行定向前端测试、typecheck 与 production build，形成 clean 提交。

## 阶段四：真实 SQLite/API/Chromium 主路径

构建当前分支真实前端产物并启动隔离 SQLite 应用。若 Hub 的 `env` 字段被运行时丢弃，使用已验证的命令内显式环境方式；必须通过服务 describe/log/port 证明端口和 SQLite 路径正确，任何落到默认 3000/`one-api.db` 的实例立即停止且不计证据。

使用真实受控 Chromium（不得用静态资源拦截代替 API）完成：

1. 登录隔离管理员；
2. 打开现有 Credit adjustment UI，先进入 increase 并选择 plan/获得 preview；
3. 切换 decrease，观察 plan/preview 被清空；
4. 提交不含 plan_id 的 decrease，捕获真实请求与精确响应；
5. 制造一个可控失败，验证同参数重试复用 key；成功或参数变化后 key 更新；
6. 刷新五个分析视图，验证数量、exact/estimated/unknown、debt、active paid count 与 SQLite/ledger 对账；
7. 通过真实订单 recovery 入口验证稳定终态和重复调用不再扣减。

完成后关闭浏览器标签、停止服务、删除隔离 SQLite/WAL/SHM、临时 secret、错误启动的 `one-api.db` 和仅验收生成的非跟踪构建产物；不得删除仓库跟踪文件。把 API/DOM/刷新/资源清理证据写入 progress 并提交。

## 最终门禁与明确边界

最终运行并记录：

- Issue #25 A–E 定向测试，四类 SQLite 并发 `count=10` 与窄 race；
- 管理员 decrease 与订单 recovery route/controller/model tracer；
- 五个分析 API；
- #20/#22/#23/#24 代表性回归与 #21 timed 多币种回归；
- 相关 Go 包测试（不要在中途跑全仓）；
- 前端定向测试、typecheck、i18n sync、production build；
- `gofmt`、LSP diagnostics（若可用）、`git diff --check`、clean tree。

MySQL 5.7/PostgreSQL 9.6 实机矩阵明确归 #27；不得把 DryRun 或 SQLite 冒充三库。不得实现或改写 #26 conversion/FX、#27 migration/ready、#28 release；不得给 timed refund 添加 grant reversal。发现这些依赖缺口时发送 escalation，不复制逻辑。

最终 `worker_done` 必须列出：最终 HEAD 与提交 SHA、API/UI/分析合同、SQLite/API/browser/并发/race证据、六语言与 build、未运行范围、共享文件风险、progress 目录和 clean tree。