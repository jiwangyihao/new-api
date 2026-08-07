# Issue #24 最终续作证据

## 冻结现场（2026-08-07）

- `git rev-parse HEAD` → `c7c983d02f2161f52a9a815a452dc7d950f692fc`。
- `git status --porcelain=v1` → 无输出。
- 当前分支 → `jiwangyihao/issue-24-final`。
- `git merge-base --is-ancestor b8598f4b7add27ba237f30dec6ceae7968cc2aa3 HEAD` → 成功。
- `git merge-base --is-ancestor 49b1ece48 HEAD` → 成功。
- `git merge-base --is-ancestor 79f3f221e HEAD` → 成功。
- `orca worktree current --json` → `parentWorktreeId` 为 `.../.workspaces/new-api/credit-operational-value-integration`，当前 head 与 Git 一致。
- 近期祖先包含：`b8598f4b7` 路由冻结合同、`5a2c12698` request→target 锁序、`88fc07a02` 锁序回归、`49b1ece48` redemption H2、`79f3f221e` admin increase H2。

## 已读取权威资料

- `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/24`。
- `CONTEXT.md`。
- `docs/adr/0001-credit-balance-entitlement.md`、`0002-credit-operational-remaining-value.md`。
- `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md` 全文。
- `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md` 全文。
- `docs/agents/credit-operational-value-execution.md`。
- `docs/agents/credit-operational-value-issue-24.md`、`credit-operational-value-issue-24-acceptance.md`。
- `docs/agents/credit-operational-value-wave-2-contract.md`、`credit-operational-value-wave-2-acceptance.md`。
- `.scratch/agent-progress/issue-20/contract.md`、`issue-22/contract.md`。
- `.scratch/agent-progress/issue-24/{contract,status,evidence}.md`。

## 已确认的既有 H2 证据

- redemption H2：CNY→USD、USD→CNY、Option 变化冻结重放、invalid FX、ledger failure 原子回滚均已在既有 evidence 中记录为 GREEN；安全提交 `49b1ece48`。
- admin increase H2：双向 FX、Option 变化冻结重放、invalid FX、ledger failure 回滚均已 GREEN；安全提交 `79f3f221e`。
- 既有 `-count=10` H2 稳定组通过；同币种管理员全组通过；未修改 #26 `credit_fx_rate.go`、`credit_valuation.go`、Option 生命周期。
- 本续作不得把这些历史记录冒充新的 API/browser 证据；新证据必须来自本轮实际命令和真实请求。

## 实际范围声明

- 当前尚未运行新的 API、analytics、frontend 或 browser 验收。
- MySQL/PostgreSQL 实机验收不属于 #24，完整矩阵归 #27；本轮只做跨库静态兼容与真实 SQLite。
- 所有后续命令、关键请求/响应、RED/GREEN、浏览器观察、提交 SHA 与清理结果将持续追加到本文件。

## 管理员 preview/commit 真实 HTTP RED

- 命令：`go test ./router -run 'TestAdminCreditAdjustment(PreviewRouteReturnsAuthoritativeMicrosWithoutWrites|CommitRouteForwardsPlanAndReturnsAuthoritativeResult)$' -count=1 -v`。
- 结果：FAIL，`github.com/QuantumNous/new-api/router`，约 5.59 秒；两条测试均通过真实 Gin router、AdminAuth 与内存 SQLite。
- preview 精确失败：`POST /api/subscription/admin/users/9962/credit-balance/adjustments/preview` 得 HTTP `404`，而合同要求 HTTP `200` 与 `success:true`；证明 preview 路由/controller/service 尚不存在。
- commit 精确失败：`POST /api/subscription/admin/users/9962/credit-balance/adjustments` 得 HTTP `200`、响应 `{"message":"credit_valuation_plan_required","success":false}`；请求已包含 `plan_id=9965`，证明现有 controller DTO/转发丢失 `plan_id`。
- commit 零写入：断言 `plan_id=9965` 的 `CreditBalanceAdjustment` 与 `CreditBalanceLedger` 均为 `0`；失败未留下半提交状态。
- preview 无写入合同尚未到达 handler，因 404 先失败；测试保留 adjustment/ledger/subscription 均为 0 的断言，供 GREEN 证明。
- 夹具事实：9965 是 `40 CNY / 1,000 Credit` 的 source plan；9963 是全局 Credit 余额 plan，`valuation_currency=CNY` 正确属于 9963。不得把估值币种错误写入 source plan。
- 本 RED 未修改任何生产代码、#26 parser/provider、migration 生命周期、analytics、UI、i18n 或 browser。

## 管理员 preview/commit 真实 HTTP GREEN

- 生产改动严格限于现有 adjustment seam：`model/credit_balance_adjustment.go` 增加只读 preview；service 仅做类型别名/转发；controller DTO 增加 `plan_id` 并转发；router 注册 preview POST。
- preview 复用既有档位资格、`CreditValuationSourceSnapshot`、`newForwardCreditValuationIngress`、冻结 `CreditFXRateSnapshot` 与整数 `prorateFloor`；未修改 #26 parser/provider/Option 生命周期。
- 命令：`gofmt -w model/credit_balance_adjustment.go controller/subscription.go service/subscription_financial_recovery.go router/api-router.go router/subscription_credit_adjustment_route_test.go && go test ./router -run 'TestAdminCreditAdjustment(PreviewRouteReturnsAuthoritativeMicrosWithoutWrites|CommitRouteForwardsPlanAndReturnsAuthoritativeResult)$' -count=10 -v`。
- 结果：`go test: 1 packages ok`，约 15.86 秒；两条真实 Gin/AdminAuth/SQLite route 行为连续 10 次通过。
- preview 可观察合同：`plan_id=9965`、gross Credit 800、gross/net `amount_micros="32000000"`、source/valuation currency CNY、confidence exact、`preview=true`；adjustment/ledger/subscription 计数均保持 0。
- commit 可观察合同：请求中的 `plan_id=9965` 穿过 controller/service；响应含 gross Credit 800、gross/net `32000000`、`state_version_after=1`、`replayed=false`；数据库有且仅有一条对应 adjustment 与 ledger。

## 管理员稳定错误码与幂等 HTTP 合同

- RED：`go test ./router -run TestAdminCreditAdjustmentRoutesExposeStableCodesAndReplayCommittedResult -count=1 -v` → FAIL；缺 plan 与冲突分别只返回 `message=credit_valuation_plan_required` / `message=credit_valuation_idempotency_mismatch`，没有稳定 `code` 字段。
- GREEN：controller 的 adjustment/preview 专用错误 writer 同时返回 `message` 与 `code=err.Error()`；不解析或映射错误文本。
- 稳定验证：`gofmt -w controller/subscription.go router/subscription_credit_adjustment_route_test.go && go test ./router -run 'TestAdminCreditAdjustment(PreviewRouteReturnsAuthoritativeMicrosWithoutWrites|CommitRouteForwardsPlanAndReturnsAuthoritativeResult|RoutesExposeStableCodesAndReplayCommittedResult)$' -count=10 -v && git diff --check` → `go test: 1 packages ok`，约 10.62 秒，diff-check 无输出。
- 行为：missing plan preview 返回 HTTP 200、`success=false`、`code=credit_valuation_plan_required`；首次提交 `replayed=false`；同 key/同事实重放 `replayed=true`、`gross_amount_micros="32000000"`、`state_version_after=1`；同 key/amount 801 返回 `code=credit_valuation_idempotency_mismatch`。
- 原子性：重放与冲突后对应 idempotency key 的 adjustment/ledger 仍各一条。
