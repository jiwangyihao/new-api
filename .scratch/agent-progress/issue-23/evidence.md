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

### 行为证明：后来 ingress 抵债后的退款转 unknown
命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetDecreaseMarksReopenedDebtUnknown$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
结论：真实 SQLite 通过公开入口预扣 1,000、追加到 1,200 形成 200 请求欠额；随后 200 Credit / 8,000,000 micros exact ingress 全部抵债、净可用与净成本均为 0；目标回退到 1,000 后重新形成的 200 可用 Credit 全部标记 unknown，状态 exact/estimated 为 0，请求 `restored_unknown_credit=200`。未把后来 ingress 价格或当前池平均伪造成恢复成本。

### 行为证明：全退款清空活动快照舍入余数
初次命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetFullRestoreClearsSnapshotRemainders$' -count=1
```
初次关键输出：测试夹具使用 `10` micros，低于套餐精确价格最小有效单位，订单完成在测试助手处失败；该失败不在请求恢复路径。

修正夹具后 GREEN 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetFullRestoreClearsSnapshotRemainders$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
结论：1,003 Credit 混合池包含不可整除成本，预扣后先少退 1 Credit、再全退到 0；最终请求 exact/estimated/unknown 活动快照全部归零，物化池完整恢复到 1,003 Credit / 40,010,000 micros，没有舍入残值。

### RED/GREEN：请求记录缺失返回稳定 sentinel
RED 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetMissingRecordReturnsStableError$' -count=1
```
关键输出：
```text
undefined: ErrCreditValuationRequestNotFound
FAIL github.com/QuantumNous/new-api/model [build failed]
```
根因：公开请求结算入口直接透传 `gorm.ErrRecordNotFound`，调用方无法依赖稳定领域错误。

GREEN 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetMissingRecordReturnsStableError$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
结论：缺失请求记录稳定返回 `ErrCreditValuationRequestNotFound`，真实 Credit 数量、估值金额和版本保持不变。

### RED/GREEN：终态后非法增加返回稳定 sentinel
RED 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetRejectsIncreaseAfterFinalization$' -count=1
```
关键输出：
```text
undefined: ErrCreditValuationFinalizedConflict
FAIL github.com/QuantumNous/new-api/model [build failed]
```
根因：终态后增加与普通目标冲突共用 `ErrCreditValuationTargetConflict`，调用方无法区分稳定终态冲突。

GREEN 命令：
```text
go test ./model -run '^TestCreditValuationRequestTarget(RejectsIncreaseAfterFinalization|FinalizesSameTargetIdempotently)$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
结论：终态后目标增加稳定返回 `ErrCreditValuationFinalizedConflict` 且数量/价值/版本不变；相同终态目标重放仍严格无操作。

### RED/GREEN：负目标返回稳定算术 sentinel
RED 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetRejectsNegativeTargetAtomically$' -count=1
```
关键输出：
```text
expected: credit_valuation_negative_input
FAIL github.com/QuantumNous/new-api/model
```
根因：公开入口把负目标泛化为目标冲突。

GREEN 命令：
```text
go test ./model -run '^TestCreditValuationRequestTargetRejectsNegativeTargetAtomically$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
结论：负目标稳定返回 `ErrCreditValuationNegativeInput`，请求数量、成本、状态版本、结算版本和终态均保持不变。

### 行为证明：状态缺失、不一致与溢出原子回滚
命令：
```text
go test ./model -run '^TestCreditValuationRequestTarget(StateMissing|StateMismatch|Overflow)RollsBackAtomically$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
结论：真实 SQLite 分别移除估值状态、制造可用量不一致、把状态版本置为 `MaxInt64`；请求追加稳定返回 state missing/state mismatch/overflow sentinel，订阅数量和请求快照/结算版本均未发生部分提交。补测试时现有深模块已满足合同，因此为直接 GREEN 的故障原子性固化。

### 请求领域核心定向回归
命令：
```text
go test ./model -run '^TestCreditValuationRequest(Target|PreConsume)|^TestCreditValuationRequestFinalizesSameTargetIdempotently$' -count=1 && git diff --check
```
关键输出：
```text
go test: 1 packages ok
git diff --check: clean
```
结论：预扣、追加、欠额、恢复、映射、absorbed、unknown、舍入余数、重放和稳定错误/原子回滚的请求领域合同整体 GREEN。

## 链路安全点 1：同步 request_id 与目标累计量
- 范围冻结：仅覆盖现有 service 测试接缝中的 Credit `SubscriptionFunding`/`BillingSession` Reserve、实时追加、最终结算与失败退款；本安全点不改 coalescer、Task、清理或 #24–#27。
- RED 目标：数据库中的同一 `SubscriptionPreConsumeRecord.request_id` 必须随累计目标更新；链路不得通过 `PostConsumeUserSubscriptionTokenDelta` 匿名修改 Credit。

### RED：Reserve 仍绕过请求累计目标
命令：
```text
go test ./service -run '^TestCreditBillingSessionRefundUsesStableRequestTarget$' -count=1
```
关键输出：
```text
expected: 150
actual  : 100
FAIL github.com/QuantumNous/new-api/service
```
精确症状：同一请求预扣 100 后 `BillingSession.Reserve(150)` 已把 `token_used` 匿名增加 50，但数据库中同一 `SubscriptionPreConsumeRecord.request_id` 的 `applied_credit` 仍为 100。旧实现因此无法让后续实时追加、最终结算或失败退款仅凭持久请求记录重放目标累计量。

### GREEN：Reserve、实时追加与失败退款复用同一请求目标
GREEN 命令：
```text
go test ./service -run '^TestCreditBillingSessionRefundUsesStableRequestTarget$' -count=1
```
关键输出：
```text
go test: 1 packages ok
```
行为：同一 request ID 预扣 100，`Reserve(150)` 将持久目标更新至 150，实时追加 25 更新至 175；失败退款以同一 request ID 把目标最终结算为 0/refunded，数量和 40,000,000 micros exact 状态完整恢复，数据库仅存在一条请求记录。

兼容定向回归与空白检查：
```text
go test ./service -run '^(TestCreditBillingSessionRefundUsesStableRequestTarget|TestCreditBalanceTaskBillingUsesTokenUnitsAndRefundsReserve|TestSubscriptionBillingSettleAvoidsHotSubscriptionRead)$' -count=1 && git diff --check
```
关键输出：
```text
go test: 1 packages ok
git diff --check: clean
```
结论：最小 GREEN 仅迁移 `SubscriptionFunding`/`BillingSession` 的 request-aware 同步链路；timed 兼容与既有 task reserve 回归保持通过。本安全点未进入 coalescer、Task 身份或清理。

## 链路安全点 2：Credit 逐请求目标合并器

### RED：公开请求目标入口绕过合并器
命令：
```text
go test ./model -run '^TestCreditRequestTargetCoalescerPreservesEnqueueOrderAndResults$' -count=1
```
关键输出：
```text
Credit request target bypassed the coalescer
FAIL github.com/QuantumNous/new-api/model
```
精确症状：配置 100ms 合并窗口后，第一个请求目标调用立即返回；现有合并器只接受匿名 subscription delta，公开 Credit 请求目标没有入队身份、稳定顺序或逐请求结果。

### GREEN：逐请求身份、顺序与结果
GREEN 命令：
```text
go test ./model -run '^TestCreditRequestTargetCoalescerPreservesEnqueueOrderAndResults$' -count=1
```
关键输出：`go test: 1 packages ok`。

定向回归、race 与空白检查：
```text
go test ./model -run '^(TestCreditRequestTargetCoalescerPreservesEnqueueOrderAndResults|TestPostConsumeUserSubscriptionTokenDeltaCoalescesConcurrentHotWrites|TestCreditValuationRequestTarget)' -count=1
go test -race ./model -run '^TestCreditRequestTargetCoalescerPreservesEnqueueOrderAndResults$' -count=1
git diff --check
```
关键输出：
```text
go test: 1 packages ok
go test -race: 1 packages ok
git diff --check: clean
```
结论：Credit 请求目标在原 coalescer 内按 original subscription 分组，队列项持久携带 request ID、原订阅 ID、目标累计量与 final，按入队顺序逐条事务结算并逐请求返回错误；499/500 预扣的两个请求各追加 1 后，第一请求获得最后 1 可用 Credit 的成本，第二请求形成 1 欠额，结果等同同序逐条事务。原 timed 匿名 delta 合并测试保持通过。