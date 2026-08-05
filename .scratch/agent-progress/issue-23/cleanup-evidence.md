# Issue #23 请求记录清理证据

## 2026-08-05 恢复安全点

### 基线核验
命令：
```text
git branch --show-current && git rev-parse HEAD && git status --short
```
关键输出：
```text
jiwangyihao/issue-23-request-settlement
d9e620191f8ca02c237859cc0250f98209749016
M service/billing_session.go
M service/task_billing_test.go
```
结论：分支和 HEAD 与协调器指定值严格一致；工作树并非干净，存在两处继承的 Task 兼容改动，因此在裁决前不进行清理生产代码写入。

### 收敛判断
- `service/task_billing_test.go` 的差异把 legacy 匿名 Credit Task 夹具迁移到持久主键身份，并补充成功、失败与重放断言。
- `service/billing_session.go` 的唯一差异把 Task relay 的 Credit `SettleWithInput` 从 `final=false` 改为 `final=true`。
- 该生产差异只允许由现有新持久 identity 成功终态定向测试证明；没有明确 RED 即撤销，不继续探索 Task 路径。

### 下一条 RED/GREEN
待运行现有 `TestCreditTaskSuccessFinalAndReplayReusePersistedRequestID`（及直接相关持久 identity 用例），分别在当前差异与撤销该一行差异的状态下验证是否只有 `final=true` 能得到 `settled` 终态。

## 终态清理资格安全点（bfa31bb09）
- RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=1` 失败；期望删除 2 条，旧实现实际删除 4 条，错误删除了过期 `consumed` 与未知状态记录。
- GREEN：`CleanupSubscriptionPreConsumeRecords` 增加明确终态资格，仅允许删除 cutoff 前的 `settled`/`refunded`。
- 结果：同一场景现仅删除 2 条过期终态记录，并保留 `consumed` 与未知状态记录。
- 稳定验证：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords$' -count=10` 返回 `go test: 1 packages ok`；`git diff --check` 无输出。
- 本安全点未实现 Task/回调引用保护、稳定 batch、并发、失败原子性或只读诊断。

## cutoff 精确边界安全点
- RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsUsesExclusiveFinalizedAtCutoff$' -count=1` 失败；期望删除 1 条，旧实现实际删除 3 条。
- 根因：旧实现使用可被后续写入改变的 `updated_at`，没有以终态落定时间 `finalized_at` 判断保留期。
- GREEN：使用排他条件 `finalized_at < cutoff`；cutoff 前删除，等于 cutoff 与 cutoff 后保留。
- 验证：单用例 `count=1` 与 `go test ./model -run '^TestCleanupSubscriptionPreConsumeRecords' -count=10` 均返回 `go test: 1 packages ok`；`git diff --check` 无输出。

## Task 请求身份投影安全点
- RED：Task schema 缺少 `subscription_request_id` 与命名索引；公开 Insert 后投影仍为 NULL；JSON 与显式非空列不一致时 Update 未 fail-closed。
- GREEN：新增 nullable `varchar(64)` 投影列与非唯一索引 `idx_tasks_subscription_request_id`；公开 Insert/Update 从 `TaskPrivateData.subscription_request_id` 同步，显式非空列不一致返回稳定错误；空身份与 timed Task 保持 NULL。
- 验证：六个投影合同用例（含 Insert/Update 两种非空冲突 fail-closed）`count=1`、`count=10` 均返回 `go test: 1 packages ok`；Insert 冲突不落库；`git diff --check` 无输出。
- 范围：未回填历史 JSON-only/NULL 行；未实现清理 `NOT EXISTS`；真实 MySQL/PostgreSQL 零 SKIP 验收仍归 #27。

## 持久 Task 引用保护安全点
- RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsProtectsActiveTaskReferences$' -count=1` 失败，`expected: 1`、`actual: 3`；旧清理误删仍被 `SUBMITTED`/`IN_PROGRESS` 持久 Task 引用的 settled/refunded 请求记录。
- GREEN：数据库侧相关 `NOT EXISTS` 精确保护非空投影；删除前按稳定主键分批完整分类活跃 NULL 投影 Task。仅可证明为 timed 且 JSON request identity 为空时放行；缺失/未知/Credit/混合版本返回稳定 `ErrSubscriptionPreConsumeCleanupAmbiguousTaskReference`，事务内删除数为 0。
- 验证：测试分别覆盖 ambiguous NULL fail-closed 且零删除、明确 timed NULL 不阻断，以及非空 `SUBMITTED`/`IN_PROGRESS` 精确保护；`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecords' -count=10` 返回 `go test: 1 packages ok`；`git diff --check` 无输出。
- 范围：历史 JSON-only/NULL 行不回填；真实 MySQL/PostgreSQL 零 SKIP 验收仍归 #27。

## 稳定有界批次安全点
- RED：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecordsUsesStableBoundedBatches$' -count=1` 失败，`expected: 2`、`actual: 3`；旧实现无界删除全部候选。
- GREEN：事务内按稳定 `id ASC` 选择至多 batch 条候选，再以同一终态/cutoff/引用条件删除；公开入口固定 batch=100。
- 幂等：测试 batch=2 时首轮删除前两条、次轮删除最后一条、第三轮返回 0；相同快照与参数产生确定删除集合。
- 验证：`go test ./model -run '^TestCleanupSubscriptionPreConsumeRecords' -count=10` 返回 `go test: 1 packages ok`；`git diff --check` 无输出。

## 批次失败原子性安全点
- RED：DELETE 后注入稳定错误，事务实际回滚，但 helper 错误返回已删除 2 条，违反错误时零影响合同。
- GREEN：事务返回任何错误时稳定返回 `(0, err)`；测试确认同批两条请求记录均保留。
- 验证：故障注入单用例 `count=1` 与 `go test ./model -run '^TestCleanupSubscriptionPreConsumeRecords' -count=10` 均返回 `go test: 1 packages ok`；`git diff --check` 无输出。

## 清理并发串行化安全点
- 场景：真实文件 SQLite、两条数据库连接、无 sleep 的确定性 barrier，同时执行过期 settled/refunded cleanup、final replay 与失败退款重放。
- 结果：活跃 `SUBMITTED`/`IN_PROGRESS` Task 投影使 cleanup 返回 0，两条请求记录均保留，两种重放均成功；结果属于合法串行化集合。
- 验证：用例 `count=10` 与 `go test -race ./model -run '^TestCleanupSubscriptionPreConsumeRecordsSerializesWithTerminalTaskReplays$' -count=1` 均返回 `go test: 1 packages ok`。
- 生产改动：无；引用保护安全点已满足该并发合同。

## 只读清理诊断安全点
- 接口：`PreviewSubscriptionPreConsumeCleanup(retentionSeconds, batchSize)`；复用清理的终态、排他 cutoff、活跃 Task 引用与稳定主键 batch 条件。
- 输出：cutoff、batch、候选数、受保护数、按终态汇总和稳定原因 `active_task_reference`。
- 只读证明：重复调用返回相同结构；SQLite `total_changes()` 与请求记录总数调用前后不变。
- 验证：诊断与 cleanup 聚焦用例 `count=10` 返回 `go test: 1 packages ok`；`git diff --check` 无输出。
