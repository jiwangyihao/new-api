# Issue #26 状态

## 当前状态

- 阶段：`CONVERSION_SAME_CURRENCY_RED`，真实 SQLite Confirm 纵切已按预期因缺失 `ValuationSource` 失败。
- 最近安全 SHA：`9c3e5ca6f`（转换 RED 起点状态提交）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`go test ./model -run TestConfirmTimedSubscriptionConversionFreezesSameCurrencyValuation -count=1`

提交 RED 后只实现同币种 `1/1` 最小 GREEN：从锁定后的 source plan 与 target valuation currency 构造唯一 `CreditValuationSourceSnapshot`，冻结 conversion/ledger/state；不得进入跨币种、在途 request 或 API/UI。

## 未提交文件

- `model/subscription_conversion_valuation_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- 同币种 RED 在 `ConfirmTimedSubscriptionConversion` 返回稳定 `credit_valuation_source_required`，精确证明 conversion 尚未复用 CreditValuation ingress seam；数据库事务回滚，未产生部分写入。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `9c3e5ca6f`；本 RED 提交后以最新 `test(issue-26)` 提交为恢复点。
4. 下一步只完成同币种 `1/1` 最小 GREEN；不得提前实现跨币种、在途 request、API/UI 或其他 Issue 范围。
