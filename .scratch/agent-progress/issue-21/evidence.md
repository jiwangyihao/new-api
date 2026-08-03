# Issue #21 验证证据

## 基线

- `git rev-parse HEAD`：`53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- 当前分支：`jiwangyihao/issue-21-timed-grants`。
- 父集成工作树 `credit-operational-value-integration`：同一 HEAD。
- 初始状态：staged 0、unstaged 0、untracked 0。

## 规范证据

已读取并采用：

- `issue://jiwangyihao/new-api/19`
- `issue://jiwangyihao/new-api/21`
- `docs/agents/credit-operational-value-execution.md`
- `docs/agents/credit-operational-value-wave-1-contract.md`
- `docs/agents/credit-operational-value-issue-21.md`
- `.scratch/agent-progress/issue-20/contract.md`
- `CONTEXT.md`
- `docs/adr/0001-credit-balance-entitlement.md`
- `docs/adr/0002-credit-operational-remaining-value.md`
- 2026-08-02 spec 第 5.3、6、8、9、10、12、13、14 节
- 2026-08-02 plan 任务 5、任务 8 timed 部分、任务 9 timed/UI 部分
- `skill://tdd`
- `skill://codebase-design`

## RED / GREEN

尚未开始。首个 tracer 将从真实 SQLite 中经公开领域入口授予计时权益，断言同事务产生不可变 grant，并证明相同来源重放不续期。

## 数据库 / API / 浏览器

尚未运行；后续持续记录精确命令、关键 payload、响应与可观察结果。

## 恢复文件提交核验

- `git show --stat --oneline 99a7ce6f5`：提交包含 `contract.md` 93 行、`evidence.md` 34 行、`status.md` 26 行，共 153 行。
- `contract.md` 已覆盖 schema、来源身份、领域接口、稳定错误、分析 DTO/算法、UI payload、共享文件与明确非所有权。
- `status.md` 已覆盖阶段、完成项、下一步、阻塞与最近安全提交；`evidence.md` 已建立 RED/GREEN、数据库、API、浏览器证据分区。
- 核验时工作树 clean；上述文件不是两行占位文件。

## 领域 tracer 收敛

- 协调器要求停止扩大接缝探索，先落公开 timed 授予入口的真实 SQLite RED。
- 当前确认的最小既有 seam：`CreateUserSubscriptionFromPlanWithResultTx` 返回真实 `EventStartTime/EventEndTime`；`TimedSubscriptionValuationGrant` 已由 Issue #20 注册并具备唯一索引与 update/delete hook；尚无 `TimedSubscriptionGrantRequest` 或 `GrantTimedSubscriptionTx`。
- 首个 RED 只防守可观察合同：一次有价订单来源在同一事务创建/续期权益并写 grant；相同确定性来源重放返回既有结果，grant 数量与权益 `end_time` 均不再变化。
- 最近安全提交：`f60433f52 docs(issue-21): 记录恢复合同核验`。

## RED 1：公开 timed 授予入口

- 测试：`TestTimedSubscriptionValuationGrantCreatesTimelineAndReplaysSource`。
- 真实数据库：独立 SQLite shared-memory 数据库，实际迁移 `User`、`SubscriptionPlan`、`UserSubscription`、`TimedSubscriptionValuationGrant`。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrantCreatesTimelineAndReplaysSource$' -count=1`。
- 结果：预期 RED；编译器报告 `undefined: TimedSubscriptionGrantRequest` 与 `undefined: GrantTimedSubscriptionTx`，证明公开领域入口尚不存在，而非夹具或断言误失败。
- 防守合同：首次来源同事务创建权益与冻结 grant；相同 `subscription_order:21003` 重放返回原窗口且不延长 `end_time`、不新增第二条 grant。

## GREEN 1：公开 timed 授予入口

- 实现：`TimedSubscriptionGrantRequest` 显式接收调用方冻结的 `SourcePriceMicros` 与 `SourceCurrency`；Plan 只提供权益期限、重置和 Credit 事实。
- 实现：`GrantTimedSubscriptionTx` 在同一事务先按确定性 `idempotency_key` / `(source_type, source_key)` 查重，再调用现有低层创建 seam，并使用其实际窗口写不可变 grant。
- 重放：完全相同来源返回原 grant 对应窗口与当前权益，不再次调用续期。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrantCreatesTimelineAndReplaysSource$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 11.57 秒。
- 权威来源检查：grant、source snapshot 与冲突指纹均使用请求中的 micros/原币种，不从当前 Plan 标价推导。

## GREEN 2：冲突与续期追加

- 测试：`TestTimedSubscriptionValuationGrantRejectsConflictAndAppendsRenewal`。
- 同一幂等键把冻结价格从 `40,000,000` 改为 `50,000,000` 时返回 `ErrTimedSubscriptionGrantIdempotencyMismatch`，事务后权益 `end_time` 不变。
- 第二个稳定订单来源使用新 key，续期窗口从上一 grant 结束时开始，并追加 USD grant；历史 CNY grant 的金额与币种保持不变。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrant(CreatesTimelineAndReplaysSource|RejectsConflictAndAppendsRenewal)$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 10.92 秒。

## RED 3：订单履约真实入口

- 测试：`TestTimedSubscriptionValuationGrantOrderCompletionCreatesGrant`，通过 `CompleteSubscriptionOrderTx` 完成带 #20 权威价格快照的 timed 订单。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrantOrderCompletionCreatesGrant$' -count=1`。
- 结果：预期 RED；订单与权益创建成功，但查询 `source_type=subscription_order, source_key=subscription_order:21203` 返回 `record not found`。
- 根因：订单履约仍直接调用 `CreateUserSubscriptionFromPlanWithResultTx`，未进入公开 timed grant 领域入口。

## GREEN 3：订单履约真实入口

- `CompleteSubscriptionOrderTx` 对带 #20 快照且非试用/邀请试用的 timed 订单调用 `GrantTimedSubscriptionTx`，传入 `ListPriceMicros/ListPriceCurrency`，不读取当前套餐标价。
- 待修正：当前 GREEN 暂时允许无快照订单走低层创建，违反 paid timed 必须同事务写 grant 的合同；该结果不作为可提交安全点。下一 RED 锁定无可靠快照稳定拒绝、显式试用/邀请仍创建权益但不写 grant。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrant(OrderCompletionCreatesGrant|CreatesTimelineAndReplaysSource|RejectsConflictAndAppendsRenewal)$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 11.29 秒。

## RED→GREEN 4：付费快照门禁与试用排除

- RED：`TestTimedSubscriptionValuationGrantPaidOrderWithoutSnapshotRejectsAtomically` 首次运行得到 `Expected error ... but got nil`，证明无可靠快照的付费 timed 订单仍会创建无 grant 权益。
- GREEN：`CompleteSubscriptionOrderTx` 在任何订单状态变更前对 timed paid 缺失履约快照返回 `ErrTimedSubscriptionGrantInvalid`；事务回滚后订单仍 pending，权益/grant 数量均为 0。
- 排除：`TestTimedSubscriptionValuationGrantExplicitTrialOrderCreatesNoGrant` 证明快照明确 `IsTrial=true` 时权益仍创建而 grant 数量为 0。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrant(ExplicitTrialOrderCreatesNoGrant|PaidOrderWithoutSnapshotRejectsAtomically|OrderCompletionCreatesGrant|CreatesTimelineAndReplaysSource|RejectsConflictAndAppendsRenewal)$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 12.16 秒。

## RED 5：兑换真实入口

- 测试：`TestTimedSubscriptionValuationGrantRedemptionCreatesAndReplaysGrant`，通过公开 `Redeem(key, user, timed)` 使用真实 SQLite 履约。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrantRedemptionCreatesAndReplaysGrant$' -count=1`。
- 结果：预期 RED；兑换与权益成功，但 `redemption:21503` grant 查询为 `record not found`。
- 根因：timed 兑换分支仍直接调用低层 `CreateUserSubscriptionFromPlanWithResultTx`；附带的 `logs` 表缺失只导致现有 best-effort 日志告警，不是断言失败根因。

## GREEN 5：兑换真实入口与冻结价格

- `Redeem(..., timed)` 对有价、启用的非试用计划调用 `GrantTimedSubscriptionTx`；兑换码 `AmountCents/Currency` 是来源快照，使用整数 `mulDivFloor(cents, 1_000_000, 100)` 转为 micros。
- 改价/改币种测试：兑换码创建时冻结 `80 CNY`，随后当前套餐改为 `50 USD`；grant 仍为 `80,000,000 CNY`。
- 重放同一兑换码返回既有 fulfillment，权益 `end_time` 不变化且 grant 数量保持 1。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrantRedemptionCreatesAndReplaysGrant$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 15.00 秒。

## RED 6：管理员 timed 售后授予

- 测试：`TestAdminCreateTimedSubscriptionRequiresRetryableAuditAndReplays`，直接驱动真实 Gin handler 与 SQLite。
- 命令：`go test ./controller -run '^TestAdminCreateTimedSubscriptionRequiresRetryableAuditAndReplays$' -count=1`。
- 结果：预期 RED；仅提交 `plan_id` 的旧 payload 返回 `success:true` 并创建权益，而合同要求 reason/idempotency 必填。
- 后续 GREEN 让 handler 校验 reason/idempotency 与显式 `source_price_micros/source_currency`，不把当前套餐标价伪装成 exact；失败重试相同 key 不续期。

## GREEN 6：管理员 timed 售后授予

- 管理员 payload 现显式要求 `reason`、`idempotency_key`、`source_price_micros`、`source_currency`；模型不从当前套餐标价推导 exact。
- `AdminBindSubscription` 只接受启用、非试用、非邀请试用的 timed plan，并把显式价格/币种与原因传入统一领域入口。
- 缺少审计字段返回 `success:false` 且不创建权益；同一完整 payload 重试复用原 grant，不续期，grant 数量保持 1；`source_snapshot` 冻结原因。
- 命令：`go test ./controller -run '^TestAdminCreateTimedSubscriptionRequiresRetryableAuditAndReplays$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 16.51 秒。
- 额外证明：当前 Plan 为 `40 CNY`，管理员 payload 显式冻结 `25,000,000 USD`，grant 按 payload 持久化；未从 Plan 推导估值。
- 组合命令：`go test ./controller -run '^TestAdminCreateTimedSubscriptionRequiresRetryableAuditAndReplays$' -count=1 && go test ./model -run '^TestTimedSubscriptionValuationGrant' -count=1`。
- 结果：两包均 PASS，耗时约 25.80 秒。

## GREEN 7：数据库计划资格与不可变性

- `GrantTimedSubscriptionTx` 先按来源重放；新来源随后锁定并完整加载数据库 `SubscriptionPlan`，资格、期限、重置与 Credit 均使用数据库事实，仅价格/币种使用调用方权威来源快照。
- disabled 语义：成功来源在计划停用后仍可原 key 重放；新 key 原子拒绝，权益 `end_time` 与 grant 数量不变。
- 不可变：真实 SQLite 上 GORM update/delete 均被模型 hook 拒绝，grant 原金额仍保留。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrant' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 14.49 秒。

## GREEN 8：规范化身份落库与重放

- grant 落库、快照和查重统一使用 `normalized.request`；带前后空白的 idempotency/source/currency 分别持久化为 `subscription_order:21053`、`subscription_order`、`CNY`。
- 使用无空白的同一身份重试返回既有权益，grant 数量保持 1，不再次续期。
- 新来源资格锁定后完整读取数据库 Plan；期限、重置和 Credit 使用数据库事实，来源价格/币种仍来自调用方冻结值。
- 命令：`go test ./model -run '^TestTimedSubscriptionValuationGrant' -count=1 && go test ./controller -run '^TestAdminCreateTimedSubscriptionRequiresRetryableAuditAndReplays$' -count=1`。
- 结果：两包均 PASS，耗时约 25.97 秒。

## RED 9：timed grant 五接口时间线

- 测试：`TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews`，真实 SQLite 创建当前 Plan=`999 EUR`，同一 timed 权益含 `40 CNY` 与 `10 USD` 两条首尾相接 grant，当前 Credit 剩余 50%。
- 预期：summary/users/subscriptions/plans/sources 统一只读 grant；recognized 为 `10 CNY + 5 USD`，不得出现 EUR。
- 命令：`go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`。
- 观察 RED：CNY 期望 10、实际 0，证明现有 paid row 仍按查询时当前 Plan 价格/币种计算。
- 已创建 `model/timed_subscription_analytics.go`：按 `CreatedAt,Id` 最早 grant 去重、实际 `end_time` 裁剪、当前周期 Credit 比例、逐币种 time/token/recognized、按 grant source 投影、missing/invalid/overlap warning。
- 曾观察编译错误：新文件自定义 `minInt64/maxInt64` 与 `model/credit_balance.go` 重复；已删除重复 helper，改为复用现有包内函数。尚未接入五接口，因此当前保持 RED，不能记录为 GREEN。
- `dto/admin_analytics.go` 当前只落盘 timed 最窄字段：`amount_micros`、nullable singular、三组 `*_by_currency`、timed confidence/warnings/unknown count；尚未验证编译兼容与五接口响应。
- 最窄测试重跑首次停在编译层：nullable singular DTO 尚未同步现有 row builder/sorter，错误位于 `admin_analytics_paid_subscription.go:826,827,970`（值/指针类型不匹配）。该半接线不是可恢复安全状态；安全提交前撤回 DTO 代码增量，只在 contract 保留冻结 shape，确保测试回到业务 RED 而非编译 RED。


## 恢复交接：最窄 tracer 编译 RED

- 日期：2026-08-03。
- 格式化：`gofmt -w dto/admin_analytics.go model/admin_analytics_paid_subscription.go`，仅触碰协调器指定的两个 Go 文件。
- tracer：`go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`。
- 结果：`FAIL github.com/QuantumNous/new-api/model [build failed]`，耗时约 3.07 秒；真实 SQLite fixture 尚未启动，因此本次没有新的业务断言结果。
- 编译错误：`admin_analytics_paid_subscription.go:855,856` 将 `dto.AdminAnalyticsMoneyAmount` 值赋给 `*dto.AdminAnalyticsMoneyAmount`；`:999` 左右两侧将 `*dto.AdminAnalyticsMoneyAmount` 传给接受值的 `adminMoneyAmountForCurrency`。
- 上一个已观察到的业务 RED 保持为 `expected 10, actual 0`；不得把当前编译失败记作 GREEN，也不得声称五接口已完成。
- 当前范围主动收敛：未尝试 UI、六语言或浏览器；未触碰 Credit 核心、FX、marker/ready。

## 恢复交接：编译恢复为业务 RED

- 修复：`model/admin_analytics_paid_subscription.go` 的两处 nullable singular 构造和两处排序 helper 调用。
- 格式化：`gofmt -w model/admin_analytics_paid_subscription.go`。
- 命令：`go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`。
- 结果：编译成功，真实 SQLite fixture 运行；业务断言 RED 为 CNY `expected 10, actual 0`，耗时约 11.70 秒。
- 结论：当前失败已准确收敛到 timed grant calculator 与 paid row/五接口未接线；没有把编译失败冒充业务证据。

## GREEN 9：timed grant 五接口时间线

- 实现：`adminBuildPaidRowsFromSubscriptions` 对 timed 权益批量读取 grant，并调用 `adminCalculateTimedSubscriptionValue`；不再以当前 Plan 正价/币种过滤或估值。
- 实现：summary、users、subscriptions、plans 统一按 `ByCurrency` 累加；sources 按 grant `source_type` 分拆；混合权益行标记 `mixed_grants`。
- 实现：跨币种 subscription singular 为 null，`recognized/token/time_based_value_by_currency` 返回 grant 原币种 micros；单币种保留兼容 singular。
- 命令：`gofmt -w model/admin_analytics_paid_subscription.go && go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 14.41 秒。
- 真实 SQLite 观察：CNY recognized 10、USD recognized 5，summary/users/plans/sources 对账一致，当前 Plan 的 `999 EUR` 未出现。