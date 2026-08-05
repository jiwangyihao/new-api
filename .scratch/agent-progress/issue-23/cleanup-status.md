# Issue #23 请求记录清理状态

## 当前状态
- 阶段：`COMPLETE`。
- 恢复 HEAD：`952322017b37c2511ce12a84769a401e0e68b0ab`；进入本阶段前 `git status --short` 为空。
- cleanup RED 已提交于 `c31a612ae`，目标用例通过公开请求预扣/结算入口构造事实。
- 本安全点只收敛清理资格：`CleanupSubscriptionPreConsumeRecords` 仅删除 cutoff 前的 `settled`/`refunded`，保留 `consumed`、未知状态与其他非终态。
- RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=1` 失败，`expected: 2`、`actual: 4`。
- GREEN：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=1` 通过，`go test: 1 packages ok`。
- 稳定验证：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=10` 通过，`go test: 1 packages ok`；`git diff --check` 无输出。
- 生产改动安全提交：`bfa31bb09`（`fix(credit): 限制预扣记录清理终态资格`）；旧实现删除 4 条，现仅删除 2 条过期 `settled/refunded`，并保留 `consumed/unknown`。
- cutoff RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsUsesExclusiveFinalizedAtCutoff$' -count=1` 失败，`expected: 1`、`actual: 3`；旧实现依据 `updated_at`，错误删除了 `finalized_at` 等于与晚于 cutoff 的终态记录。
- cutoff GREEN：清理改按 `finalized_at < cutoff` 判断；阈值前删除，等于与阈值后保留。单测 `count=1` 与全部 cleanup 用例 `count=10` 均返回 `go test: 1 packages ok`；`git diff --check` 无输出。
- Task 投影 RED：schema 缺列/索引；Insert 后投影为 NULL；Update 不一致未报错。GREEN：`tasks.subscription_request_id` 为 nullable `varchar(64)`、命名非唯一索引 `idx_tasks_subscription_request_id`；公开 Insert/Update 从 JSON 同步，显式非空列不一致稳定返回 `task subscription request identity projection mismatch`，空身份及 timed Task 保持 NULL。
- 验证：六个投影合同用例（含 Insert/Update 两种非空冲突 fail-closed）`count=1` 与 `count=10` 均为 `go test: 1 packages ok`；Insert 冲突不落库；`git diff --check` 无输出。此安全点未实现清理 `NOT EXISTS`，未回填历史 JSON-only 行。
- 引用保护 RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsProtectsActiveTaskReferences$' -count=1` 行为失败，`expected: 1`、`actual: 3`；旧清理误删仍被 `SUBMITTED`/`IN_PROGRESS` 持久 Task 投影引用的 settled/refunded 记录。
- 引用保护 GREEN：数据库侧相关 `NOT EXISTS` 精确保护非空投影；删除前用稳定主键 `FindInBatches` 完整分类活跃 NULL 投影 Task。只有可证明为 timed 且 JSON request identity 为空时放行；缺失/未知/Credit/混合版本返回稳定 `ErrSubscriptionPreConsumeCleanupAmbiguousTaskReference`，整次删除数为 0。全部 cleanup 用例 `count=10` 为 `go test: 1 packages ok`；`git diff --check` 无输出。
- 稳定批次 RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsUsesStableBoundedBatches$' -count=1` 行为失败，`expected: 2`、`actual: 3`；旧实现一次无界删除全部候选。
- 稳定批次 GREEN：事务内按 `id ASC` 选取有界候选并以同一资格/引用条件删除；公开入口 batch=100。测试 batch=2 固定首轮删除前两条、次轮删除最后一条、第三轮零删除；全部 cleanup 用例 `count=10` 为 `go test: 1 packages ok`，`git diff --check` 无输出。
- 失败原子性安全提交：`70625ed4f90403e755ce63cab05cb842a1967fa0`；DELETE 后故障注入整批回滚，错误时稳定返回删除数 0。
- 并发验证：真实文件 SQLite、两条连接与确定性 barrier 并发执行 cleanup、final replay、失败退款重放；活跃 Task 投影使 cleanup 返回 0 且保留两条记录。用例 `count=10` 与窄 `-race count=1` 均为 `go test: 1 packages ok`；现有生产实现已满足，无需生产改动。
- 只读诊断 GREEN：`PreviewSubscriptionPreConsumeCleanup` 复用同一 cutoff/终态/Task 引用查询，报告 cutoff、batch、候选数、保护数、终态计数和稳定原因；重复调用结果相同，SQLite `total_changes()` 与记录总数不变。聚焦用例 `count=10` 为 `go test: 1 packages ok`，`git diff --check` 无输出。
- 审计保留 GREEN：公开预扣→最终退款后清理请求记录；`CreditBalanceLedger`（含低频 `SourceSnapshot`）、`CreditValuationState`、`SubscriptionOrder` 前后等价，仅请求记录删除。用例 `count=10` 为 `go test: 1 packages ok`；`git diff --check` 无输出。

## Cleanup RED 证据
- 命令：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=1`。
- 结果：期望只删除 2 条过期 `settled/refunded`，实际删除 4 条；`consumed` 与未知状态也被现有 `CleanupSubscriptionPreConsumeRecords` 按 `updated_at` 删除。
- RED 原因是现有清理入口缺少终态资格约束，而非符号未定义；测试通过公开请求预扣/结算入口构造事实。

## 后续范围
1. 先使终态资格 RED 转 GREEN，再逐项增加 cutoff 边界、Task/回调持久引用保护、稳定 batch、幂等、失败原子性、只读诊断、并发与审计保留。
2. 禁止继续 Task/legacy/quota/conversion 工作；禁止实现 #24–#28。

## 最终门禁
- cleanup/Task 投影组：`go test ./model -run "^(TestCleanupSubscriptionPreConsumeRecords|TestPreviewSubscriptionPreConsumeCleanup|TestTaskSubscriptionRequestProjection)" -count=10`，`go test: 1 packages ok`。
- model 窄 race：cleanup 并发与 coalescer 用例，`go test: 1 packages ok`。
- 请求领域 model 聚焦门禁：`go test: 1 packages ok`。
- BillingSession/Task identity service 聚焦门禁及窄 race：均 `go test: 1 packages ok`。
- controller Credit/Subscription 聚焦门禁：`go test: 1 packages ok`。
- `git diff --check` 无输出。
- 未运行：真实 MySQL/PostgreSQL 零 SKIP（归 #27）、全项目测试、生产部署。无 UI 变更，不需要浏览器验证。
