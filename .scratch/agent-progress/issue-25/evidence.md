# Issue #25 验证证据

## 恢复现场

- `git status --short --branch && git rev-parse HEAD && git merge-base HEAD fe1901aaf7a769fe7057c6483e30b7b1491adcdc && git branch --show-current`
  - 分支：`jiwangyihao/issue-25-destructive-outflow`
  - staged / unstaged / untracked：均为 0
  - HEAD：`fe1901aaf7a769fe7057c6483e30b7b1491adcdc`
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

## 待收集证据

- 混合池比例 outflow、清空余数、欠额、零余额、溢出与成本非负。
- 事务故障注入和完整回滚。
- 幂等重放、指纹冲突、refund / chargeback 终态竞争。
- outflow 与 request settle / refund 的数据库并发及活动请求快照不变。
- 管理员 decrease API、真实订单 recovery、五个运营分析接口的真实 SQLite tracer。
- 相关 Go 包 `-race`。
- 管理员 increase→decrease 浏览器交互、真实 payload / response 与分析刷新。
- 前端定向测试、typecheck/build、六语言 missing/extras 与 `git diff --check`。

## 数据库实测边界

- 本切片必须真实执行 SQLite。
- MySQL 5.7.44 / PostgreSQL 9.6.24 完整零 SKIP 矩阵归 #27；本切片只做跨库静态审查，不把 DryRun 宣称为实测。
