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

## RED→GREEN 10：grant 时间线缺口披露

- RED：真实 SQLite 权益窗口 `[snapshot,snapshot+100)` 只有 `[snapshot+50,snapshot+100)` grant；金额按已知 CNY grant 计算，但 `unknown_timed_subscription_count` 实际为 0。
- RED 命令：`go test ./model -run '^TestPaidSubscriptionValueWarnsForMissingTimedGrantCoverage$' -count=1`；失败为 `expected 1, actual 0`。
- GREEN：对裁剪后的 grant coverage 合并区间做完整性检查；任一内部或边界缺口稳定返回 `missing_timed_grants` warning 与 unknown，不回退当前 Plan 的 EUR 价格。
- GREEN 命令：`gofmt -w model/timed_subscription_analytics.go && go test ./model -run '^TestPaidSubscriptionValueWarnsForMissingTimedGrantCoverage$' -count=1 && go test ./model -run '^TestPaidSubscriptionValueUsesTimedGrantTimelineAcrossFiveViews$' -count=1`。
- 结果：两个真实 SQLite tracer 均 PASS，耗时约 15.48 秒。

## GREEN 11：重叠 grant 稳定去重

- 测试：先创建覆盖完整剩余窗口的早期 CNY grant，再创建覆盖后半窗口的后期 USD grant。
- 命令：`go test ./model -run '^TestPaidSubscriptionValueDeduplicatesOverlappingTimedGrants$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 14.68 秒。
- 观察：recognized 仅为早期 CNY 20；后期 USD 重叠秒不重复计值；subscription warning 含 `overlapping_grants`，summary unknown timed count 为 1。
- 扩展回归：`go test ./model -run '^TestPaidSubscriptionValue' -count=1` 暴露多条旧 fixture 未创建 grant，因此按新合同得到 unknown/0；这是测试夹具迁移缺口，不是允许回退当前 Plan 价格的理由。

## GREEN 12：实际权益窗口裁剪

- 测试：grant 冻结 100 秒、20 CNY，但管理员实际权益 `end_time` 缩短至第 40 秒。
- 命令：`go test ./model -run '^TestPaidSubscriptionValueClipsTimedGrantAtActualSubscriptionEnd$' -count=1`。
- 结果：PASS，`go test: 1 packages ok`，耗时约 18.64 秒。
- 观察：recognized 为 8 CNY；grant 超出实际 `subscription.end_time` 的 60 秒不计值，裁剪后的可交付窗口完整覆盖，因此不误报 `missing_timed_grants`。

## Orca 1.4.167 恢复接管

- 当前 Dispatch：`ctx_7d91bd847e54`；协调消息：`msg_2cef5a4086be`。
- `git rev-parse HEAD`：`f812e77fcd6e3d2875ce7b973ccc49c87e612590`。
- `git status --short`：无输出，工作树 clean。
- 已读取 `.scratch/agent-progress/issue-21/{status,evidence,contract}.md`，确认不重做 timed grant 与五接口已验收实现。
- 已读取 `skill://shadcn-ui` 与 `skill://i18n-translate`；剩余范围严格收敛为管理员 timed grant UI、跨币种 timed 展示、六语言、真实浏览器和最终定向门禁。
- 禁止范围保持不变：Credit 核心、转换 FX、migration marker/ready、历史回填和生产发布。

## RED 13：管理员 timed 售后授予 UI

- 测试：`timed subscription after-sales grant > submits frozen valuation facts and reuses the retry key until grant details change`。
- 首次命令：`bun test src/features/subscriptions/components/dialogs/user-subscriptions-dialog.test.tsx`；环境先因已声明但未安装的 `happy-dom` 失败，执行 `bun install --frozen-lockfile` 恢复锁文件依赖后重跑。
- 真实行为 RED：既有转换审计测试通过，新测试失败于找不到 accessible combobox `Timed subscription plan`。
- 根因：管理员对话框仍只有旧 `plan_id` Select + Add button，既不收集 reason，也不冻结 `price_amount_micros/currency`，没有客户端 idempotency key 或失败重试状态。
- 防守合同：提交完整 `{plan_id, reason, idempotency_key, source_price_micros, source_currency}`；同一失败重试复用 key；reason 改变后使用新 key。

## GREEN 13：管理员 timed 售后授予 UI

- 实现：管理员用户套餐 Sheet 只列出启用、非试用、非邀请试用、timed 且具有正 `price_amount_micros` 的计划。
- payload 冻结所选计划的 `price_amount_micros` 与原币种，并提交非空 reason 与客户端 `admin-timed-<user>-<uuid>` 幂等键。
- 重试语义：相同 user/plan/reason/price/currency 的失败重试复用 key；任一事实变化时生成新 key；成功后清空 attempt，后续授予使用新 key。
- 可见反馈：失败 Alert 明确提示重试将复用 key；成功 Alert 明确完成状态。
- 格式化与测试：`bunx prettier --write src/features/subscriptions/types.ts src/features/subscriptions/components/dialogs/user-subscriptions-dialog.tsx src/features/subscriptions/components/dialogs/user-subscriptions-dialog.test.tsx && bun test src/features/subscriptions/components/dialogs/user-subscriptions-dialog.test.tsx`。
- 结果：`2 pass, 0 fail`；既有转换审计行为保持 GREEN，新 timed UI 测试验证完整 payload、失败同 key 重试和 reason 变化换 key。

## RED→GREEN 14：timed 跨币种运营剩余价值展示

- RED：新增跨币种 timed 明细夹具，singular 三字段均为 null，CNY/USD 只在 `*_by_currency`；旧字段映射显示 `—`，而期望 `¥10.00, $5.00`。
- RED 命令：`bun test src/features/admin-analytics/panel-fields.test.ts`；结果 `9 pass, 1 fail`，recognized 实际 `—`。
- GREEN：前端类型接受 nullable singular、三组 by-currency、`valuation_confidence`、`valuation_warnings`、`amount_micros` 与 summary unknown timed count；卡片在 singular null 时展示 by-currency，且展示置信度/warning。面板 summary 新增 unknown timed 权益数量。
- GREEN 命令：`bunx prettier --write src/features/admin-analytics/types.ts src/features/admin-analytics/lib/panel-fields.ts src/features/admin-analytics/panel-fields.test.ts && bun test src/features/admin-analytics/panel-fields.test.ts`。
- 结果：`10 pass, 0 fail`；跨币种显示 CNY 10/USD 5，token CNY 12/USD 6，time CNY 20/USD 10；`bun run typecheck` PASS。

## GREEN 15：管理员 timed 与分析六语言

- 新增 15 个 UI 键，覆盖 reason、精确估值资格、失败同 key 重试、成功/失败状态、timed 计划、估值置信度、估值 warning 与 unknown timed 数量。
- en、zh、fr、ja、ru、vi 均提供人工翻译；非英语值逐键验证不等于英文源字符串。
- 命令：`node scripts/add-issue-21-translations.mjs && bun run i18n:sync`；临时脚本随后删除。
- 同步报告：六个 locale 的 `missingCount=0`、`extrasCount=0`；本切片 15 个键全部存在且非空。
- 全仓既有 untranslated 基线仍由 `_reports/*.untranslated.json` 披露；本切片新增非英语键均已翻译，不把英语复制为其他语言。

## 最终定向 SQLite / API tracer

- 领域命令：`go test ./model -run "^(TestTimedSubscriptionValuationGrant|TestPaidSubscriptionValue(UsesTimedGrantTimelineAcrossFiveViews|WarnsForMissingTimedGrantCoverage|DeduplicatesOverlappingTimedGrants|ClipsTimedGrantAtActualSubscriptionEnd))" -count=1`。
- 领域结果：PASS；真实 SQLite 覆盖统一授予、重放/冲突、续期追加、冻结价格/币种、不可变、五视图跨币种、缺口、重叠与实际失效裁剪。
- API 初始门禁发现 controller fixture 未迁移 `timed_subscription_valuation_grants`，四个明细端点真实失败为 `no such table`。修复仅更新测试数据库迁移与 timed grant 夹具，不修改生产计算或恢复当前 Plan 价格回退。
- 新增强 API tracer `TestPaidSubscriptionValueEndpointsReturnTimedGrantAmountsAcrossFiveViews`：当前 Plan 为 `999 EUR`，不可变 grant 为 `40 CNY + 10 USD`，当前 Credit 50%；逐个真实 handler 解析响应。
- 实际 API 响应摘要：summary recognized/token=`10,000,000 CNY + 5,000,000 USD` micros，time=`20,000,000 CNY + 10,000,000 USD` micros，active count=1；users、plans 与 summary recognized 相同；subscriptions 三个 singular 均为 `null`、三组 by-currency 与 summary 对账、`source_attribution=mixed_grants`、confidence=`exact`；sources 两行 order/admin 合计同一 CNY/USD recognized，无 EUR。
- API 命令：`go test ./controller -run "^(TestPaidSubscriptionValueEndpointsReturnTimedGrantAmountsAcrossFiveViews|TestPaidSubscriptionValueEndpointsReturnPanelEnvelope|TestAdminCreateTimedSubscriptionRequiresRetryableAuditAndReplays)$" -count=1`。
- API 结果：PASS；五个运营分析端点的真实 SQLite 金额/nullable/mixed-source 合同与管理员授予 API 同批通过。

## HANDOFF_READY：真实浏览器启动现场

- 受监督进程：`issue21-backend`，应用 `go run .`。
- 结果：进程在 601 ms 内以 exit 1 结束，未达到 HTTP readiness；完整错误为 `main.go:77:12: pattern web/classic/dist: no matching files found`。
- 根因：`main.go` 使用 `//go:embed web/default/dist` 与 `//go:embed web/classic/dist`，当前工作树两个 dist 均不存在；因此这次没有数据库、API、登录或浏览器行为证据。
- 已确认浏览器路径接缝：新库 `POST /api/setup` → `/api/user/login`；管理员 `/users` 的 User Subscription Management Sheet 调用 `/api/subscription/admin/users/:id/subscriptions`；跨币种面板位于 `/admin-analytics` 的 paid-subscription-value tab，读取五个 paid-subscription-value 端点。
- 恢复后必须用真实 session 和真实 API 数据观察：失败重试复用同一 `admin-timed-*` key，成功后产生新 key；计时明细显示 CNY/USD by-currency，singular null 不按当前 Plan 币种补猜。
- 本轮按协调指令停止探索；浏览器 smoke 与其后的最终窄门禁仍未执行，不能宣称完成。

## 浏览器续作恢复（2026-08-04）

- 恢复指令：已完整读取父树 `docs/agents/credit-operational-value-issue-21-browser-recovery.md`，并按其唯一范围执行浏览器与最终交付。
- 基线：`git rev-parse HEAD` 为 `2f9701976282d1c53d7ce0914088a302498f6f32`；`git status --short` 无输出，工作树 clean。
- 固定现场：端口 `31021`；临时数据库 `.scratch/agent-progress/issue-21/browser/issue21-smoke.db`；受监督服务名 `issue21-backend-recovery`；隔离 session secret 仅通过启动环境传入，不写入仓库。
- 计划构建：在 `web/default` 与 `web/classic` 分别执行 `bun install --frozen-lockfile`、`bun run build`；若 classic frozen install 因既有锁文件漂移失败，只采用不改锁文件的最保守恢复，并记录原始错误。
- 下一动作：提交本恢复状态安全点，然后生成两个真实 dist、启动隔离后端并用健康端点证明 `127.0.0.1:31021` readiness。
- 范围：不修改既有 timed 领域实现；不触碰 Credit、FX、request settlement、migration marker/ready、历史迁移或生产发布。