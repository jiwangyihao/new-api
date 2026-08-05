# Issue #26 状态

## 当前状态

- 阶段：`FX_FLOOR_OVERFLOW_RED`，C 组整数 floor/overflow 公共行为测试已按预期因接口缺失失败。
- 最近安全 SHA：`bbd80b32c`（B 组安全点状态校准）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`go test ./model -run TestCreditFXRateSnapshotConvertMicrosUsesOverflowSafeFloor -count=1`

提交 C 组 RED 后，只实现无分配整数 `floor(amount × numerator / denominator)`、宽中间乘积、结果溢出、分母零与余数清空；不得进入 conversion/request/API/UI。

## 未提交文件

- `model/credit_fx_rate_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已明确暂停 conversion、request、API/UI 探索，先只完成 FX A/B/C 三组。
- parser/provider 必须是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- C 组 RED 精确缺少 `ErrCreditFXOverflow` 与 `CreditFXRateSnapshot.ConvertMicros`；失败来自预期公共 seam 缺失。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `bbd80b32c`；C 组 RED 提交后以最新 `test(issue-26)` 提交为恢复点。
4. 下一步只完成 C 组最小 GREEN、`count=10`、窄 `-race` 与 diff-check；禁止进入 conversion/request/API/UI。
