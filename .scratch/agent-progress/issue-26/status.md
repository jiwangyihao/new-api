# Issue #26 状态

## 当前状态

- 阶段：`CONVERSION_CROSS_CURRENCY_RED`，真实 SQLite CNY→USD 与 USD→CNY tracer 已按预期返回 unsupported currency。
- 最近 clean SHA：`ca02e62e3`（同币种 conversion GREEN）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`go test ./model -run TestConfirmTimedSubscriptionConversionFreezesCrossCurrencyValuationAndReplay -count=1`

提交本 RED 后只实现唯一 FX snapshot 读取、整数换算、冻结 conversion/ledger/state 与既有 idempotency replay；禁止在途 request、API/UI 与 Issue #24/#25/#27/#28。

## 未提交文件

- `model/subscription_conversion_valuation_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- RED 两个方向均稳定返回 `credit_valuation_unsupported_currency`，精确证明 Confirm 仍在同币种 guard fail-closed；未产生转换写入。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `ca02e62e3`；本 RED 提交后以最新 `test(issue-26)` 提交为恢复点。
4. 下一步只做最小 GREEN；不得进入在途 request、API/UI 或其他 Issue 范围。
