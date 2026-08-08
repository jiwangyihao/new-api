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
