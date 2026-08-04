# Issue #22 窄验收修复 Agent 指令

## 目标与冻结基线

你只负责修复父 PRD #19 / GitHub Issue #22 在协调器窄验收中发现的两个高优先级缺口。工作树为 `C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-22-credit-tracer`，冻结 clean 基线必须是 `d5bba460f633ffd2943b1d13bb88b65cea338733`。不得重新实现已经通过的 CreditValuation、订单/余额/Kyren/BillingSession、32 CNY tracer、UI、六语言或浏览器 smoke。

原始 findings 位于 `C:/Users/34404/AppData/Local/Temp/new-api-issue22-coordinator-narrow-review.md`：

1. users/subscriptions/plans/sources 的 `recognized_remaining_value` 排序仍通过兼容 `float64 amount` 和 `adminCompareFloat`，而不是权威十进制 `amount_micros`。不同的超大 micros 可能被比较为相等或错误顺序。
2. Credit 明细虽有 `snapshot_semantics=current_only`，但 summary/users/subscriptions/plans/sources 五个 `AdminAnalyticsPanelResponse` 没有稳定结构化 panel warning；前端从 subscription 明细自行推断不能替代 API 合同。

必须用 TDD 从两个会在当前 frozen HEAD 稳定失败的行为测试开始，做最小根因修复并保持工作树 clean。完成后只发送一次有效 `worker_done`，等待协调器重新验收；不要自行合并、关闭 Issue、部署或回收工作树。

## 必读材料与 Skill

开始修改前依次阅读：

1. 仓库与全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/22`；GitHub CLI 始终使用 `--repo jiwangyihao/new-api`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-1-contract.md`。
5. `docs/agents/credit-operational-value-issue-22.md`。
6. `docs/agents/credit-operational-value-issue-22-acceptance.md`。
7. `C:/Users/34404/AppData/Local/Temp/new-api-issue22-coordinator-narrow-review.md`。
8. `.scratch/agent-progress/issue-22/{status,evidence,contract}.md`，复用已有事实，不重新探索完整切片。
9. `CONTEXT.md`、ADR 0002 与 2026-08-02 规格中 micros、current-only、五接口响应的对应章节。

必须读取并使用 `skill://tdd`。若比较器或 warning 聚合的根因不清楚，读取 `skill://diagnosing-bugs`；若需要确定最窄模块 seam，读取 `skill://codebase-design`。只有实际修改可见文案时才读取 `skill://i18n-translate`；优先复用现有 current-only 翻译，不新增无必要文字。不要运行格式化器、lint 或全仓测试；只格式化实际修改文件并运行本任务所需定向门禁。

## 修复 A：权威 micros 排序

- 为 users、subscriptions、plans、sources 四个列表新增可失败的行为测试。测试数据至少包含兼容 `float64 amount` 相同或因 IEEE-754 精度丢失而不可区分、但十进制 `amount_micros` 明确不同的两行；同时覆盖升序与降序、稳定 tie-breaker。
- 排序必须解析并比较 `AdminAnalyticsMoneyAmount.amount_micros` 的十进制整数语义，禁止将 micros 先转 `float64`，禁止用兼容 `amount` 作为权威排序依据。
- 使用已有固定宽度整数/严格十进制 helper；如果响应范围允许 `int64`，解析后比较 `int64`。无效 micros 必须遵循已有稳定错误/warning 合同，不能静默回退 float 或解析错误文本决定分支。
- 兼容 `amount` 仍可保留用于旧客户端展示，但只能由 micros 最后派生。相同 micros 的二级排序必须保持现有确定性（例如稳定 ID/名称），不得引入随机顺序。
- summary 没有列表排序，不要为了“统一”扩大修改范围。

## 修复 B：五面板 current-only 结构化 warning

- 为 summary、users、subscriptions、plans、sources 五个接口分别增加当前 frozen HEAD 会失败的测试：当 Credit 状态 `updated_at > snapshot_at` 时，响应的 `AdminAnalyticsPanelResponse.Warnings` 必须包含同一个稳定结构化 current-only warning。
- warning 必须使用现有 DTO/稳定 code/reason 形状；不得让调用者解析人类错误文本。五个面板的 code、语义和确定性必须一致，不能只有 subscriptions 有提示。
- 明细仍必须保留 `snapshot_semantics=current_only`、最新 state version/updated_at；不得伪造历史快照，也不得把 warning 变成阻断错误。
- 前端继续显示现有 current-only 非阻断提示和刷新动作。若当前 UI 仅从 subscription item 推断，最小调整为优先消费 panel warning，并保留兼容旧响应的明细推断；相关测试必须验证 API warning 驱动提示。禁止新增重复提示或改变 32 CNY/Exact/Not applicable/Moving weighted average 展示。
- 五接口在没有 current-only 行时不得返回该 warning；多条 current-only 行只能形成去重且稳定排序的 warning，不得重复。

## 严格非目标

- 不修改人民币余额、Kyren、BillingSession/request_id 或 Credit 数量/估值深模块。
- 不实现 Issue #23 的 target 增减、退款、异步 task identity、coalescer。
- 不实现 Issue #24/#25 的正向入账或破坏性恢复。
- 不实现 Issue #26 FX。
- 不创建、CAS、更新 migration marker，不切换 `ready`，不回填历史；这些属于 Issue #27。
- 不重跑或伪造真实浏览器 smoke；业务代码修复后只需定向前端测试，协调器会决定是否需要复验浏览器。
- 不修改受保护项目身份、生产配置、凭据、锁文件或无关用户文件。

## 持久化与崩溃恢复

开始实际修改前，在 `.scratch/agent-progress/issue-22/` 创建并尽快提交：

- `narrow-fix-status.md`：冻结基线、阶段、已完成、下一步、阻塞、最近安全提交。
- `narrow-fix-evidence.md`：两个 finding 的 RED/GREEN 命令与精确失败/通过信号。
- `narrow-fix-contract.md`：micros 比较器输入/错误/tie-breaker、warning code/去重/传播与严格非所有权。

每完成一个 finding 即形成一个小步 Conventional Commit，subject 使用简体中文。工具或模型意外中断前先把当前状态、dirty 文件和下一条命令写入 status；不要把关键成果只留在终端、stash、临时脚本或未提交测试中。

## 验收与完成条件

至少完成以下证据：

1. 四列表升/降序在 precision-boundary micros 数据上 RED→GREEN；证明没有兼容 float 回退。
2. 五接口 current-only warning RED→GREEN；证明无 current-only 时为空、多行时去重稳定，明细语义仍保留。
3. 既有 `TestCreditValuationFiveAnalyticsViewsAgreeOnThirtyTwoCNY`、paid-value 相关 model/controller 定向测试通过，32,000,000 micros、active count 1、nullable time、source/filter 不回归。
4. 相关前端 format/panel/page 定向测试通过；若未改前端生产代码，如实说明。
5. `go test` 仅运行受影响包/用例；前端仅运行受影响测试及必要 typecheck。不要运行全仓套件。
6. 对实际修改文件执行格式化，运行 `git diff --check`。
7. 最终 `git status --short` 为空，progress 文件与证据全部提交。

`worker_done` 必须列出最终 HEAD、每个提交、两个 RED/GREEN、修改文件、warning code/排序合同、定向测试、未运行范围（尤其 MySQL/PostgreSQL）、工作树 clean 状态，并明确未实现 Issue #23–#28。