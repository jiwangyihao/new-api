# Issue #26 状态

## 当前状态

- 阶段：`FX_INVALID_INPUTS_RED`，A 组稳定错误公共行为测试已按预期因接口缺失失败。
- 最近安全 SHA：`2cb9feaed`（恢复安全点校准）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`go test ./model -run TestParseCreditFXRateSnapshotRejectsInvalidInputsWithStableErrors -count=1`

提交 A 组 RED 后，只实现缺失/空/非法十进制/超精度/非正/不支持币种/方向不匹配的稳定 sentinel 与严格分类；不得进入 B 组 identity/反向或 C 组 floor/overflow。

## 未提交文件

- `model/credit_fx_rate_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已明确暂停 conversion、request、API/UI 探索，先只完成 FX A/B/C 三组。
- parser/provider 必须是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- A 组 RED 当前缺失 8 个稳定 sentinel/方向输入字段；失败来自预期公共 seam 缺失，不是环境或无关测试。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `2cb9feaed`；A 组 RED 提交后以最新 `test(issue-26)` 提交为恢复点。
4. 仅完成 A 组最小 GREEN；禁止提前扩展 B 组 identity/反向、C 组 floor/overflow 或 conversion/request/API/UI。
