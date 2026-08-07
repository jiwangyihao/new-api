# Issue #26 最终复评续作证据

## 冻结现场

- `git status --short --branch`：`branch jiwangyihao/issue-26-final-review-fix`，staged/unstaged/untracked 均为 0。
- `git rev-parse HEAD`：`44009213cb8e4a582de34f884deecd5a8d687b2c`。
- `git merge-base --is-ancestor b8598f4b7add27ba237f30dec6ceae7968cc2aa3 HEAD`：退出码 0。
- `git merge-base --is-ancestor 3feb091159aef26731c1698647791acc03c29c0a HEAD`：退出码 0。
- `orca worktree current --json`：`baseRef=jiwangyihao/credit-operational-value-integration`，`parentWorktreeId` 指向 `credit-operational-value-integration`，HEAD 与本文件记录一致。
- 最近提交含 `eb493ded0`、`41f0d4dec`（#24 H2 跨币种 ingress）与 `b8598f4b7`、`44009213c`（路由夹具校准）；本任务禁止覆盖或回退。

## 复评 finding 与当前代码证据

- M1：`model/errors.go` 当前只有 `ErrConversionIdempotencyConflict`；`subscriptionConversionRejection`/plan guard 仍生成自由文本；controller 曾以文本前缀归类。首个 RED 必须证明 `errors.Is(..., ErrConversionIneligible)` 失败，并由稳定 code 路由测试捕获文本耦合。
- M3：`SubscriptionConversion` 已有 committed numerator/denominator，ledger→conversion 写入也存在；controller 响应 helper 仍需核对并以测试证明其未直接使用 committed 字段。无需新增 schema，目标是删除响应层重算并 fail closed。
- M2：quote DTO 当前没有 `quote_id`/`created_at`/`expires_at`/`facts_fingerprint`；Confirm 入口当前只接收 `subscription_id` 与 `idempotency_key`。真实 SQLite RED 必须证明 quote 后事实变化仍可能被确认或旧 quote 可重放。

## RED/GREEN

- RED：尚未运行。
- GREEN：尚未运行。
- 最后安全提交：`44009213cb8e4a582de34f884deecd5a8d687b2c`。
- 当前未提交：三份 `final-review-continuation-*` 恢复文档。
- 阻塞：无。
- 下一条命令：读取 `model/errors.go`、`model/subscription_conversion.go`、`controller/subscription_conversion.go` 相关完整区段和既有 route 测试，写 M1 第一个行为 RED。

## 未实测边界

- 尚未运行本续作任何 RED/GREEN、SQLite tracer、race、前端或包级门禁。
- MySQL 5.7/PostgreSQL 9.6 实机零 SKIP 属于 #27；本任务只做静态兼容与真实 SQLite 行为证明。
- 未部署、未写生产数据。
