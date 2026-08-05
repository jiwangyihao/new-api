# Issue #23 验证证据

## 2026-08-05 基线与材料核验

### 起始工作树
命令：
```text
git status --short --branch && git merge-base HEAD ec1858fec89509bdec9a90a230a8496047c5becd && git rev-parse HEAD
```
关键输出：
```text
branch jiwangyihao/issue-23-request-settlement
staged 0, unstaged 0, untracked 0
ec1858fec89509bdec9a90a230a8496047c5becd
ec1858fec89509bdec9a90a230a8496047c5becd
```
结论：Worker 从协调器指定的已验收 #20/#21/#22 集成提交创建，且工作树起始干净。

### 必读材料
已读取：
- `AGENTS.md` 与自动注入全局规则
- `issue://jiwangyihao/new-api/19`
- `issue://jiwangyihao/new-api/23`
- `docs/agents/credit-operational-value-execution.md`
- `docs/agents/credit-operational-value-wave-2-contract.md`
- `.scratch/agent-progress/issue-20/contract.md`
- `.scratch/agent-progress/issue-22/contract.md`
- `CONTEXT.md`
- `docs/adr/0001-credit-balance-entitlement.md`
- `docs/adr/0002-credit-operational-remaining-value.md`
- 规格 5.4、6、7.3–7.5、9、11.3、13、14
- 实施计划任务 3 和任务 6
- `skill://tdd`、`skill://diagnosing-bugs`、`skill://codebase-design`

### #22 依赖结论
`.scratch/agent-progress/issue-22/contract.md` 明确声明已交付：
- `CreditValuation` 深模块；
- 购买来源快照；
- 最小同步 request tracer（只支持足额预扣及相同目标最终重放）；
- 通用 analytics DTO 与五接口 Credit 分流。

因此 #23 可在当前基线上深化 request 分支，不复制 #22 逻辑。

## RED/GREEN 记录

### 循环 1：目标累计量增加按当前池出账
RED 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetIncreaseUsesCurrentPool$' -count=1
```
关键输出：
```text
--- FAIL: TestCreditValuationRequestTargetIncreaseUsesCurrentPool
credit_valuation_request_test.go:19: Received unexpected error
FAIL github.com/QuantumNous/new-api/model
```
根因：#22 的 `SettleCreditRequestTargetTx` 明确只允许冻结 tracer 的相同目标，`200 -> 300` 返回 `ErrCreditValuationTargetConflict`。

GREEN 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetIncreaseUsesCurrentPool$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
行为：真实 SQLite 经购买 ingress 建立 1,000 Credit / 40 CNY 状态，预扣 200 后提交目标 300；最终 `token_used=300`、可用 700、exact `28,000,000` micros、请求活动快照 `12,000,000` micros、`settlement_version=2`。

### 导出符号引用证据
修改 `SettleCreditRequestTarget` 前使用 LSP references，找到 6 处：定义、`service/funding_source.go` 以及 `model/credit_valuation_tracer_test.go` 的 4 个调用。修改包内 `SettleCreditRequestTargetTx` 前使用 LSP references，只有定义和公开 wrapper 两处。

### 循环 2：少结算恢复原请求快照
RED 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetDecreaseRestoresOriginalSnapshot$' -count=1
```
关键输出：
```text
--- FAIL: TestCreditValuationRequestTargetDecreaseRestoresOriginalSnapshot
credit_valuation_request_test.go:84: Received unexpected error
FAIL github.com/QuantumNous/new-api/model
```
场景：40 CNY / 1,000 Credit 预扣 200（8 CNY 快照），随后交错入账 20 CNY / 1,000，再把目标降为 100。旧实现拒绝减少目标。

GREEN 命令：
```text
go test ./model -run '^TestCreditValuationRequestTarget(DecreaseRestoresOriginalSnapshot|IncreaseUsesCurrentPool)$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
行为：少结算恢复原预扣快照的 4 CNY，而非使用交错入账后的新池平均；最终 `token_limit=2,000`、`token_used=100`、可用 1,900、exact `56,000,000` micros，请求活动快照降为 100 Credit / `4,000,000` micros，并终态 settled。

### 回归：退款先撤销本请求欠额
命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetDecreaseRefundsDebtBeforeSnapshot$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
行为：预扣 900，追加到 1,100 形成 100 请求欠额，再将目标降至 950；先撤销 100 欠额，仅从活动快照恢复 50，最终可用 50、exact 2,000,000 micros，absorbed/unknown 均为 0。

### 请求领域入口 clean cutover
使用 LSP rename 将 `SettleCreditRequestTarget` clean cutover 为 `SettleUserSubscriptionRequestTarget`，原子迁移 model tests 与 `service/funding_source.go` 的全部引用；未保留旧别名。验证命令：
```text
go test ./model -run '^TestCreditValuationRequest(Target|PreConsume)|^TestCreditValuationRequestFinalizesSameTargetIdempotently$' -count=1
```
关键输出：`go test: 1 packages ok`。

## 范围声明
- 尚未运行项目级全量测试、格式化器或 lint；按 Dispatch 合同由协调器最终统一运行。
- 本切片不新增 UI 或可见文案，因此当前不需要浏览器或 i18n 技能。
- MySQL/PostgreSQL 实测矩阵属于 #27；本切片仅做跨库静态语义审查和真实 SQLite 证明。


## 2026-08-05 恢复现场

### 现场保护与核对
命令：
```text
git status --short && git diff --stat && git diff
```
关键输出：
```text
staged 0, unstaged 5, untracked 0
model/credit_valuation.go              |  7 +++++--
model/credit_valuation_request_test.go | 32 ++++++++++++++++++++++++++++----
model/credit_valuation_tracer_test.go  | 14 +++++++-------
model/errors.go                        |  1 +
service/funding_source.go              |  2 +-
```
结论：严格保留五个 dirty 文件；未丢弃、覆盖或清理现场。差异为公开请求结算入口增加 `original_subscription_id`、调用点 clean cutover、稳定映射冲突 sentinel 与定向测试。

### 恢复 GREEN：追加目标与欠额优先退款
命令：
```text
go test ./model -run '^TestCreditValuationRequestTarget(IncreaseUsesCurrentPool|DecreaseRefundsDebtBeforeSnapshot)$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
结论：追加按当前池出账/形成欠额与目标减少先撤销本请求欠额的既有 GREEN 行为在恢复后保持不变。

### 恢复安全点回归
命令：
```text
go test ./model ./service -run '^TestCreditValuationRequestTarget' -count=1 && git diff --check
```
关键输出：
```text
go test: 2 packages ok
git diff --check: clean
```
结论：original subscription identity clean cutover、映射冲突回滚以及既有请求目标行为通过 model/service 定向回归；差异无空白错误。

### 行为证明：退款快照被其他请求欠额吸收
命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetDecreaseAuditsRestoreAbsorbedByOtherDebt$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
结论：真实 SQLite 通过公开预扣/结算入口交错两个请求；退款请求的 400 Credit 快照中仅 200 重新成为可用量并恢复 8,000,000 micros，另 8,000,000 micros 进入 `absorbed_restore_exact_cost_micros`，未增加物化价值。该合同在补测试时已由既有恢复实现满足，因此本循环为直接 GREEN 的行为固化，而非新增生产实现。