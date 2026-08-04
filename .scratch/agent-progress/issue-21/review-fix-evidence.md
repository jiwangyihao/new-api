# Issue #21 Standards 修复证据

## 冻结输入

- Standards 评审：`C:/Users/34404/AppData/Local/Temp/new-api-issue21-standards-final-review.md`，结论为 Findings 1–4 阻塞。
- 冻结实现 HEAD：`547512242578ec198034d322875c5485735b247a`，初始工作树 staged/unstaged/untracked 均为 0。
- 父集成 HEAD：`2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc`，父集成工作树 staged/unstaged/untracked 均为 0。
- #22 集成提交说明：Issue #22 记录 `ac830971a32e24f5b88c42b312d62fffd4229e21`；当前父集成 HEAD 已在其后继续前进。

## 父集成合并恢复安全点

- 合并提交：`9cee335ddb0638af7b5bb9229d5d2a03db5a0712`，父集成 HEAD `2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc`。
- 冲突：23 个冲突块按冻结所有权解决；保留 #22 的通用整数 `adminMoneyAccumulator`、Credit/current_only DTO/状态/排序与前端 BigInt 语义，叠加 #21 的 timed `*_by_currency`、calculator、warning/source 和现有 timed UI。
- 后端验证：#22 权威 micros 排序与 #21 timed 五接口 tracer 同批通过；nullable recognized singular 在跨币种时从 by-currency 读取权威 micros。
- 前端验证：`panel-fields` 11/11 PASS；`bun run typecheck` PASS；Credit 不适用与 timed 跨币种展示语义均保留。
- 清洁度：冲突 0，staged 0、unstaged 0、untracked 0；cached diff check 通过。

## 已读取合同

- Issue #19、#21、#22。
- `docs/agents/credit-operational-value-execution.md`。
- Wave 1 contract/acceptance、Issue #21 instruction/acceptance。
- `.scratch/agent-progress/issue-21/{status,evidence,contract}.md`。
- `CONTEXT.md`、ADR 0002、2026-08-02 spec/plan 的金额、timed、锁/幂等、错误与测试章节。
- `skill://diagnosing-bugs`、`skill://tdd`、`skill://codebase-design`、Orca orchestration/CLI 实时指南。
- 父集成 #22 的 `model/admin_analytics_paid_subscription.go`、`dto/admin_analytics.go`、权威 micros 排序与 current_only 相关测试接缝。

## RED / GREEN 证据

### Finding 1：并发同源重放合法串行化

- RED seam：新增 `TestTimedSubscriptionValuationGrantConcurrentReplayLinearizes`，使用 `t.TempDir()` 文件型 SQLite、WAL、`busy_timeout(5000)`、`SetMaxOpenConns(4)` 与两个独立事务；测试侧 GORM query callback/barrier 强制旧实现的两个事务均在计划锁前完成“无 grant” replay read，再同时放行，不使用 `sleep` 或生产 test hook。
- RED 命令：`gofmt -w model/timed_subscription_valuation_concurrency_test.go && go test ./model -run '^TestTimedSubscriptionValuationGrantConcurrentReplayLinearizes$' -count=1 -v`。
- RED 精确症状：一个事务在 `model/subscription.go:880` 插入 `user_subscriptions` 时返回 `database is locked (5) (SQLITE_BUSY)`；测试在并发 outcome 的 `require.NoError` 失败，成功结果数为 1、错误结果数为 1。旧实现因此没有给调用者两个成功的同源重放结果。
- 根因：`GrantTimedSubscriptionTx` 在权威计划行锁之前读取 grant 身份。两个连接都可观察“来源不存在”，再并发创建/续期；SQLite 在写升级处泄漏方言锁错误，其他方言还可能在唯一约束处失败。
- 最小 GREEN：复用 `SubscriptionPlan.conversion_guard_version` 既有数据库 guard；入口先对目标计划行执行原子自增写，成功后再读取 replay identity，随后读取数据库计划资格并创建权益/grant。锁序现在为 `SubscriptionPlan guard -> existing grant identity -> target UserSubscription -> new grant`。未新增进程内 mutex、retry wrapper、savepoint 或泛化框架。
- GREEN 单次：`go test ./model -run '^TestTimedSubscriptionValuationGrantConcurrentReplayLinearizes$' -count=1 -v` → PASS。
- GREEN 重复：`go test ./model -run '^TestTimedSubscriptionValuationGrantConcurrentReplayLinearizes$' -count=10` → PASS。
- 领域回归：`go test ./model -run '^TestTimedSubscriptionValuationGrant' -count=1` → PASS，覆盖参数 mismatch、disabled 已提交来源重放/新来源拒绝、事务回滚、续期与不可变性既有合同。
- 竞态：`go test -race ./model -run '^TestTimedSubscriptionValuationGrantConcurrentReplayLinearizes$' -count=1` → PASS。
- SQLite 结论：两个调用均成功，返回同一 subscription/window；最终 `user_subscriptions=1`、`timed_subscription_valuation_grants=1`，只续期一次，未泄漏 `UNIQUE constraint` 或 `SQLITE_BUSY`。
- MySQL 5.7/PostgreSQL 9.6：未运行；三库零 SKIP 仍归 Issue #27。
## 数据库范围

- SQLite：待运行真实文件型或共享多连接并发证明。
- MySQL 5.7：未运行，三库零 SKIP 归 Issue #27。
- PostgreSQL 9.6：未运行，三库零 SKIP 归 Issue #27。
