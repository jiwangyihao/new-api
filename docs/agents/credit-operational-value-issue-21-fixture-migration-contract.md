# Issue #21 旧测试夹具迁移并行合同

## 背景与冻结基线

父 PRD 为 GitHub `jiwangyihao/new-api#19`，当前切片为 `#21`。冻结实现工作树为：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

冻结实现 HEAD：`774b35740c1879b285537031410731317d0142fc`。该 HEAD 已通过聚焦领域、并发、整数金额、溢出、稳定错误、Redemption、订单重放、前端、i18n 与浏览器门禁；尚未通过 `go test ./model ./service ./controller -count=1`，原因是旧测试夹具未迁移到 #21 已确立的不可变授权快照与 timed grant 合同。生产 fail-closed 行为不得为旧夹具降级。

三路 Agent 必须从同一冻结 HEAD 创建为 `issue-21-timed-grants` 的 Orca 子工作树，不得从 `origin/main` 或集成父树直接派生。它们并行执行但文件所有权互斥：

- A：仅 `model` 旧 paid-value analytics 测试夹具。
- B：仅 `service` invitation Credit redemption 测试夹具。
- C：仅 `controller` 余额、Kyren、Stripe、Epay 与邀请订单测试夹具。

## 共同业务边界

1. 不得修改或放宽生产 fail-closed：有价 timed 服务窗口必须有不可变 `TimedSubscriptionValuationGrant`；已授权订单必须有合法 `SubscriptionOrder.EntitlementSnapshot`；新兑换必须通过 `Redemption.Insert` 冻结 `FulfillmentSnapshot`；缺失授权事实必须稳定拒绝。
2. 测试必须通过真实领域入口或专用测试 helper 构造合法事实。不得直接篡改生产条件、屏蔽错误、放宽断言、跳过测试、捕获 panic 后当成功，或在生产路径添加“仅测试兼容”分支。
3. 不得触碰 #22 的 `CreditValuation` 深模块、moving-weighted/current_only、request settlement、BillingSession、前端 BigInt；不得实现 #23–#28、FX、migration marker/ready 或新 schema。
4. 保留 disabled-plan 边界：已有 disabled-plan 权益可消费；新购买、兑换、转换或管理员授予仍拒绝 disabled plan。已授权成功订单可按购买时不可变快照履约，不因当前 Plan 停用而撤销。
5. 金额继续使用整数 micros。不得从 `float64` 反推权威金额。
6. MySQL/PostgreSQL 实机零 SKIP 属于 #27；本波只证明真实 SQLite/受影响包回归，不得冒充三数据库通过。

## TDD 与恢复纪律

每路 Agent 必须先读取 `skill://diagnosing-bugs` 与 `skill://tdd`；涉及测试 helper 的领域边界时读取 `skill://codebase-design`。先运行本路最小失败测试，记录原始失败；再最小迁移夹具并运行 GREEN。禁止一开始跑项目全量测试、formatter、lint 或前端套件。

每路必须立即创建并持续提交：

- `.scratch/agent-progress/issue-21/fixture-<a|b|c>-status.md`
- `.scratch/agent-progress/issue-21/fixture-<a|b|c>-evidence.md`
- `.scratch/agent-progress/issue-21/fixture-<a|b|c>-contract.md`

文件中记录冻结 HEAD、失败测试、原始错误、修改文件、领域事实构造方式、测试命令、最近安全提交、未提交文件和下一步。每个可验证小步使用 Conventional Commits（英文 type/scope、简体中文 subject）提交。上下文达到约 80% 前必须形成 clean 或诚实 WIP 的 `HANDOFF_READY`。

## 合并顺序与冲突合同

协调器在三路有效 `worker_done` 后，按 A（model）→B（service）→C（controller）顺序合入 #21 分支。正常情况下三路不应修改同一文件。若发现必须跨越所有权：

- 先在 progress 中记录精确缺口；
- 通过 Orca `question` 请求协调器决策；
- 在收到答复前不要修改另一 Agent 的目录或生产代码。

三路合并后由协调器运行 `go test ./model ./service ./controller -count=1`。任何剩余失败按真实证据继续修复，不得把当前三路交付包装成全量通过。

## 完成条件

每路完成前必须：定向 RED/GREEN 有证据；对应包级 `go test ./<package> -count=1` 通过或诚实列出与本路无关且有精确证据的剩余失败；`git diff --check` 通过；工作树 staged/unstaged/untracked 全零；只发送一次当前 Dispatch 注入 capability 的 `worker_done`，列出最终 HEAD、提交、文件、测试、未运行项和范围声明。Agent 不自行合并、关闭 Issue、部署或回收工作树。
