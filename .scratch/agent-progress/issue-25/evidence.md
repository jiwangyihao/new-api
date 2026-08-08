# Issue #25 验证证据

## 恢复现场

- `git status --short --branch && git rev-parse HEAD && git merge-base HEAD fe1901aaf7a769fe7057c6483e30b7b1491adcdc && git branch --show-current`
  - 分支：`jiwangyihao/issue-25-destructive-outflow`
  - staged / unstaged / untracked：均为 0
  - 恢复现场 HEAD：`fe1901aaf7a769fe7057c6483e30b7b1491adcdc`；当前 A/B HEAD 见下方交接提交。
  - merge-base：`fe1901aaf7a769fe7057c6483e30b7b1491adcdc`
- `orca status --json && orca worktree current --json && orca orchestration dispatch-show --task task_685c1c42de63 --json`
  - Orca runtime：ready
  - parentWorktreeId：`credit-operational-value-integration`
  - baseRef：`jiwangyihao/credit-operational-value-integration`
  - Run：`run_59804e39b728`
  - Dispatch：`ctx_214c53d3471f`，状态 `dispatched`，failure_count=0

## 首个 RED 前置核对

- #20 / #22 / #23 / #24 已集成接缝存在；未复制或修改其合同。
- `ApplyCreditValuationOutflowTx` 已实现按操作前可用量比例移除 exact / estimated / unknown、清空余数与超量欠额。
- `RecoverCreditBalanceTx` 当前直接增加 `UserSubscription.token_used`，未调用估值 outflow；因此公开 `AdjustCreditBalance(decrease)` 在 ready SQLite 混合池下预期出现数量变化而估值状态不变的 RED。
- decrease 携带 `plan_id` 的现有稳定拒绝为 `ErrCreditValuationPlanIneligible`；首个测试将同时锁定此无副作用合同。

## RED / GREEN

- RED：`go test ./model -run '^TestAdminCreditBalanceDecreaseRejectsPlanAndWithdrawsMixedPool$' -count=1`
  - 真实 SQLite 公开入口：`AdjustCreditBalance`。
  - `decrease` 携带 `plan_id` 返回 `ErrCreditValuationPlanIneligible`，且 adjustment 数量不变。
  - 无 `plan_id` 的 200 Credit decrease 后，`token_used` 从 200 增至 400，但 `CreditValuationState.available_credit` 错误保持 800（期望 600）。
  - 修复前 FAIL：`model/credit_positive_ingress_test.go:376`。
- GREEN：`go test ./model -run '^TestAdminCreditBalanceDecreaseRejectsPlanAndWithdrawsMixedPool$' -count=1`
  - PASS；mixed pool 结果为 available=600、exact=24,000,000、estimated=12,000,000、unknown=150、state_version=2。
- 稳定性：`gofmt -w model/credit_balance_recovery.go && go test ./model -run '^TestAdminCreditBalanceDecreaseRejectsPlanAndWithdrawsMixedPool$' -count=10 && git diff --check`
  - PASS；10 次重复均通过，格式与 whitespace 检查通过。
- 实现只修改 `model/credit_balance_recovery.go`：valuation ready 时调用现有 `ApplyCreditValuationOutflowTx`，无新 schema / interface / ledger 字段；未进入退款、拒付、UI、#27 或 #28。
- GREEN 安全提交：`92482861f fix(issue-25): 同步管理员减少的运营估值`。

## A 组：Outflow 核心

- 初次 RED：`go test ./model -run '^TestAdminCreditBalanceDecrease(ClearsRemainderAndOnlyFormsSettlementDebt|ReplaysConflictsAndRollsBackAtomically)$' -count=1`
  - 首次编译 RED：testify `require.NotNegative` 不存在，改为可编译的 `GreaterOrEqual` 行为断言。
  - 行为 RED：同事实首次结果 `state_version_after=0`、重放为 2，暴露首次 decrease 使用临时 ingress 形状而重放使用持久化 ledger 的不一致。
- 最小 GREEN：decrease 首次返回改为复用 `creditBalanceAdjustmentResultTx` 持久化 ledger 投影；未改变 increase 路径。
- A 组 GREEN：同一定向命令 PASS。
- 稳定性与 race：`go test ./model -run '^TestAdminCreditBalanceDecrease(RejectsPlanAndWithdrawsMixedPool|ClearsRemainderAndOnlyFormsSettlementDebt|ReplaysConflictsAndRollsBackAtomically)$' -count=10 && go test -race ./model -run '^TestAdminCreditBalanceDecrease(RejectsPlanAndWithdrawsMixedPool|ClearsRemainderAndOnlyFormsSettlementDebt|ReplaysConflictsAndRollsBackAtomically)$' -count=1 && git diff --check`
  - PASS / PASS；10 次真实 SQLite 重复、窄 `-race` 与 whitespace 检查通过。
- 行为结果：A=3 时 Q=3 清空 exact=10、estimated=7、unknown=2 的全部余数；Q=5 仍只移除上述非负池成本并形成 2 Credit settlement debt。
- A 组 clean 安全提交：`90e6f3c80 test(issue-25): 覆盖管理员减少边界与原子性`。

## B 组：订单退款 / 拒付 immutable facts

- RED：`go test ./model -run '^TestCreditOrderRecoveryUsesImmutablePurchaseFactsAndTerminalReplay$' -count=1`
  - 先后暴露订单 recovery 依赖可变 order snapshot、recovery ledger 未复制 purchase price / FX facts、`ParameterFingerprint` 为空、payload / reason 冲突被误判 replay，以及 refund→chargeback 被原 refund 指纹误拒绝。
- GREEN：同一命令 PASS。
  - `subscriptionOrderCreditRecoveryIdentityTx` 优先读取 immutable purchase ledger；无 purchase ledger 时保留既有兼容路径。
  - recovery ledger 复制 purchase `SourcePriceMicros`、`SourcePlanCredit` 与完整 FX facts；实际撤值仍为操作前 mixed pool 的 60,000,000 micros，不按订单原价撤值。
  - 使用现有 `CreditBalanceLedger.ParameterFingerprint` 绑定 recovery type / payload / operator / reason / credit-only；同终态仅完全相同事实 replay，payload / reason 变化返回 `ErrSubscriptionOrderRecoveryConflict`。
  - refund→chargeback 晋升复用原 refund ledger，不再次 outflow；随后 chargeback replay 稳定。
- 最终命令：`gofmt -w model/subscription_recovery.go model/credit_balance_recovery.go model/credit_valuation_tracer_test.go && go test ./model -run '^TestCreditOrderRecoveryUsesImmutablePurchaseFactsAndTerminalReplay$' -count=1` → PASS。
- B 组 clean 安全提交：`d6fdcd45c fix(issue-25): 固化订单回收事实与终态幂等`。
- 硬截止范围：未执行 B 组 count=10 / 相邻 recovery / race；未进入 C/D/E、UI、#27/#28。

## PENDING：下一续作精确测试接缝

- **C 请求快照恢复**：真实 SQLite，公开 `SettleUserSubscriptionRequestTarget` / `SettleCreditRequestTargetTx`；场景为 deduction snapshot → later ingress absorbs debt → request refund。验证原 exact / estimated / unknown attribution、absorbed 恢复审计、重新可用 unknown 与其他活动 request snapshot 不变。
- **D 邀请取消隔离**：真实 SQLite，公开 `RecoverSubscriptionOrder`；构造 Credit 订单与错误/既有 `InvitationRewardEvent`，验证 recovery / order terminal / reward cancellation 同事务幂等、Credit 不产生邀请收益且不进入邀请付费统计。
- **E 并发原子性**：文件型 SQLite WAL；覆盖 refund + chargeback、refund + admin decrease、outflow + request settle、outflow + request refund，断言合法串行集合、单一 recovery ledger、数量/估值/状态版本一致、活动 request snapshot 不变；随后跑相关窄 `-race`。
- **仍待后续阶段**：管理员 decrease API、五个运营分析接口、UI/browser、六语言、MySQL/PostgreSQL 静态审查；不得在 C/D/E 提前进入 UI 或 #27/#28。

## 当前边界

- A/B 已完成；C/D/E 均为 PENDING，不得误报完成。
- MySQL 5.7.44 / PostgreSQL 9.6.24 完整零 SKIP 矩阵归 #27；当前只实测 SQLite。

## 数据库实测边界

- 本切片必须真实执行 SQLite。
- MySQL 5.7.44 / PostgreSQL 9.6.24 完整零 SKIP 矩阵归 #27；本切片只做跨库静态审查，不把 DryRun 宣称为实测。

## C 组：请求冻结归因最小公开路径探针

- 新增唯一测试：`TestCreditRequestRefundRestoresFrozenAttributionAfterDebtAbsorption`。
- 公开真实 SQLite 路径：`PreConsumeUserSubscriptionByUnits` 冻结 mixed exact / estimated / unknown deduction snapshot；第二个 request 保持 active；`SettleUserSubscriptionRequestTarget(..., 1_200, false)` 形成 400 Credit debt；后续 500 Credit ingress 吸收该 debt；`SettleUserSubscriptionRequestTarget(..., 0, true)` 全额退款。
- 冻结快照：deducted available=800、debt formed=400、exact=19,200,000 micros、estimated=9,600,000 micros、unknown=240。
- 退款后：available=1,300、exact=23,200,000 micros（保留 later ingress 的 4,000,000 micros，仅恢复原冻结 exact）、estimated=9,600,000 micros、unknown=640；request deduction 与 debt 余数清零，absorbed exact / estimated / unknown 均为 0，`restored_unknown=400`，另一 active request 逐字段不变。
- 首次命令因测试误引用不存在的 `CreditBalanceGrantResult.NetCostMicros` 编译失败；这是测试编译错误，不是行为 RED，已仅修正测试断言。
- 行为命令：`go test ./model -run '^TestCreditRequestRefundRestoresFrozenAttributionAfterDebtAbsorption$' -count=1`
  - 结果：`go test: 1 packages ok`，现有生产实现意外 GREEN。
- 结论：本硬截止未实现生产 GREEN、未进入 D/E；保留该公开路径回归测试并交接。

## D 组：邀请取消隔离

- 新增表驱动真实 SQLite 测试：`TestCreditOrderRecoveryCancelsOnlyMatchingInvitationReward`，覆盖 refund 与 chargeback。
- 初次命令：`go test ./model -run '^TestCreditOrderRecoveryCancelsOnlyMatchingInvitationReward$' -count=1`
  - 初次 FAIL 原因是测试夹具误以为 Credit 订单完成会自动创建 `InvitationRewardEvent`；实际查询 `source_type=subscription_order` / 对应 order id 返回 record not found，直接证明 Credit ingress 不新增邀请收益。
  - 按 D 合同改为显式插入两个错误/既有 active 事件，分别绑定两个独立订单稳定 identity；未修改生产代码。
- 修正夹具后的相同命令：PASS（`go test: 1 packages ok`）。
- 隔离结果：recover 目标订单后仅目标事件变为 `cancelled` 并记录 recovery reason；另一订单事件与 recovery 前结构体逐字段相同。
- 幂等结果：同事实 replay 返回 `Replayed=true` 并复用首次 recovery ledger；订单、目标/其他事件、Credit subscription、valuation state、总 ledger / recovery ledger / reward / commission 计数全量快照不变。
- Credit 排除结果：两个 Credit 订单完成后自动生成的 reward event 数为 0、commission 数为 0；邀请付费统计 `PaidInviteeCount=0`、`ActivePaidInviteeCount=0`、recognized / active amount 均为空。
- 生产代码：零修改；现有实现直接 GREEN。
- 后续边界：E 仍 PENDING；本提交不进入并发、API、UI、i18n 或 browser。

## E 首场：refund + chargeback

- 测试：`TestConcurrentRefundAndChargebackRecoverCreditOnceWithChargebackPrecedence`。
- 并发夹具：真实临时文件 SQLite，`PRAGMA journal_mode=wal`；query callback 确定性阻塞两个 `SubscriptionOrder` 读取，`sql.DB.Stats().InUse >= 2` 证明两个独立连接同时到达 barrier；无 sleep 猜时序。
- 单次：`go test ./model -run '^TestConcurrentRefundAndChargebackRecoverCreditOnceWithChargebackPrecedence$' -count=1` → PASS。
- 稳定性：相同命令 `-count=10` → PASS。
- 合法串行集合：refund 先行时 chargeback 晋升并 replay 原 ledger；chargeback 先行时 refund 返回 `ErrSubscriptionOrderRecoveryConflict`。最终始终为 chargeback，只有一条 `subscription_order_recovery` ledger、一次 -500 Credit outflow，balance limit=500 / used=500。
- 错误合同：拒绝结果不得含 `SQLITE`、`database is locked`、`UNIQUE constraint` 或 `gorm`；稳定 sentinel 经 `errors.Is` 验证。
- 失败零写入：终态 chargeback 后重放低优先级 refund，订单、余额与 recovery ledger 全量快照不变。
- 结论：现有生产实现直接满足首场合同；未修改生产代码。

## E 第二场：refund + admin decrease

- 新增测试：`TestConcurrentRefundAndAdminDecreaseUseLegalSerializations`。
- 并发夹具：复用 `setupSubscriptionRecoveryConcurrencyTestDB` 的真实临时文件 SQLite WAL；User query callback 确定性阻塞 refund 与 admin decrease 两条事务，`sql.DB.Stats().InUse >= 2` 证明两个独立连接同时到达；无 sleep 猜时序。
- 单次：`go test ./model -run '^TestConcurrentRefundAndAdminDecreaseUseLegalSerializations$' -count=1` → PASS；现有生产实现直接满足，未修改生产代码。
- 合法串行结果：500 Credit refund recovery 与 100 Credit admin decrease 均成功且各写一次；最终 subscription token limit=500、token used=600、settlement debt=100；valuation available/exact/estimated/unknown 全为 0、state_version=3。
- 账本不变量：恰好一条 `subscription_order_recovery` ledger（-500）和一条 `admin_decrease` ledger（-100）；两条 ledger 的 valuation gross cost 合计恒为 30,000,000 micros。admin 先执行时撤值 6,000,000 micros，refund 先执行时 admin 撤值 0，均属于合法串行集合；最大 ledger state version 为 3。
- 错误合同与零写入：使用相同 idempotency key 但 amount=101 的 admin replay 返回 `ErrCreditValuationIdempotencyMismatch`，错误不含 `SQLITE`、`database is locked` 或 `UNIQUE constraint`；订单、余额、估值、全部 ledger 与 adjustment 全量快照不变。
- 边界：按协调器裁决停在第二场；第三、四场及生产修改均未进入。

## E 第三场：low-frequency outflow + request final settle

- 新增 `TestConcurrentLowFrequencyOutflowAndRequestFinalSettleUseLegalSerializations`：真实文件 SQLite WAL、两个独立连接、确定性 `UserSubscription` query barrier；另一个 active request 全结构保持不变。
- RED：`go test ./model -run '^TestConcurrentLowFrequencyOutflowAndRequestFinalSettleUseLegalSerializations$' -count=1` 泄漏 `database is locked (5) (SQLITE_BUSY)`，定位于 request target 批事务与管理员低频 outflow 同时从读事务升级为写事务。
- 最小 GREEN：`flushSubscriptionRequestTargets` 复用既有 `transactionWithUserSettingCASRetry`；每次 attempt 清空 `failureIndex` 和 `results`，避免成功 retry 返回首轮失败残留。未新增公开接口或数据库特例。
- GREEN：相同命令 PASS。最终 token limit=500 / used=650 / debt=150，valuation 池清零、state_version=5；request 终态 `settled`，一条 admin outflow ledger。两种合法串行结果分别冻结 request available/exact 为 200/12,000,000 或 100/6,000,000，并保持总撤值 21,000,000 micros，不重复撤值。
- 终态异参 settle 返回 `ErrCreditValuationFinalizedConflict`，全量快照零写入且不泄漏 SQLite/唯一约束文本。

## E 第四场：low-frequency outflow + request refund

- 新增 `TestConcurrentLowFrequencyOutflowAndRequestRefundUseLegalSerializations`：同一真实 WAL 双连接 barrier；原 request 全退款，另一 active request 的 original attribution / snapshot 不变。
- 单次 GREEN：`go test ./model -run '^TestConcurrentLowFrequencyOutflowAndRequestRefundUseLegalSerializations$' -count=1` → PASS。
- 最终 token limit=500 / used=450 / available=50，exact=3,000,000、state_version=5；request `refunded` 且 deduction snapshot 余数清零。合法串行集合为 admin 撤值 24,000,000（refund 先）或 21,000,000 + absorbed restore exact 3,000,000（outflow 先）；连同剩余 state 与 active request snapshot，总成本恒守恒 30,000,000 micros。
- 终态异参 refund 返回 `ErrCreditValuationFinalizedConflict`，全量快照零写入且无数据库文本泄漏。

## E 可控失败与重试耗尽

- `TestCreditRequestTargetInjectedFailureRollsBackAtomically` 表驱动覆盖 final settle 与 full refund；在 request 中间 update 注入错误后，subscription、valuation、目标/另一 active request、ledger 全量快照不变。
- `TestCreditRequestTargetPersistentSQLiteLockReturnsStableConflictWithoutWrites` 使用独立写事务确定性持锁至重试耗尽。RED 为原始 `SQLITE_BUSY` 泄漏；GREEN 为 `ErrCreditValuationStateMismatch` 精确错误文本，持锁事务回滚后全量快照不变。
- 通用事务重试 helper 在耗尽可重试 SQLite/CAS 错误时统一返回 `ErrUserSettingCASConflict`；request coalescer 将其映射为既有 `ErrCreditValuationStateMismatch`，避免数据库文本穿透。

## E 稳定性、race 与联合回归

- 四场 `-count=10`：`go test ./model -run '^TestConcurrent(RefundAndChargebackRecoverCreditOnceWithChargebackPrecedence|RefundAndAdminDecreaseUseLegalSerializations|LowFrequencyOutflowAndRequest(FinalSettle|Refund)UseLegalSerializations)$' -count=10` → PASS。
- 窄 race：相同 regex，`go test -race ./model ... -count=1` → PASS。
- CDE / A/B / Issue #23 / Issue #24 代表性联合回归：`go test ./model -run '^(TestCreditRequestRefundRestoresFrozenAttributionAfterDebtAbsorption|TestCreditOrderRecoveryCancelsOnlyMatchingInvitationReward|TestConcurrent(RefundAndChargebackRecoverCreditOnceWithChargebackPrecedence|RefundAndAdminDecreaseUseLegalSerializations|LowFrequencyOutflowAndRequest(FinalSettle|Refund)UseLegalSerializations)|TestCreditRequestTarget(InjectedFailureRollsBackAtomically|PersistentSQLiteLockReturnsStableConflictWithoutWrites)|TestAdminCreditBalanceDecrease(RejectsPlanAndWithdrawsMixedPool|ClearsRemainderAndOnlyFormsSettlementDebt|ReplaysConflictsAndRollsBackAtomically)|TestCreditOrderRecoveryUsesImmutablePurchaseFactsAndTerminalReplay|TestCreditValuationRequestTargetDecrease(RestoresOriginalSnapshot|AuditsRestoreAbsorbedByOtherDebt|MarksReopenedDebtUnknown)|TestAdminCreditBalanceIncreaseOffsetsDebtBeforeExactValue|TestRedemptionCreditBalanceOffsetsDebtBeforeExactValue)$' -count=1` → PASS。
- request coalescer 相邻回归：`go test ./model -run '^TestCreditRequestTargetCoalescer' -count=10` → PASS。
- MySQL 5.7 / PostgreSQL 9.6 未运行，归 Issue #27；API/UI/i18n/browser 与 Issue #28 未进入。
