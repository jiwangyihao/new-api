# Issue #26 状态

## 当前状态

- 阶段：`FX_COMPLETE`，A/B/C 三组 RED/GREEN 已独立提交，C 组提交后工作树已验证 clean。
- 最近安全 SHA：`5318e5cc2`（`feat(issue-26): 实现 FX 整数安全换算`）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

完成仅含 status/evidence 的安全点校准，然后定向定位现有 timed conversion quote/confirm seam 与测试惯例；下一条行为 RED 必须只覆盖 Quote 冻结 source tier 单位价值、currency、转换数量和 FX snapshot。

不得并行展开 request、API 或 UI；先完成 Quote 冻结估值的单一 RED→GREEN。

## 未提交文件

- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已明确暂停 conversion、request、API/UI 探索，先只完成 FX A/B/C 三组。
- parser/provider 必须是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- FX parser/snapshot 已完成 A/B/C：稳定非法错误、identity/双向倒数、确定性冻结、overflow-safe floor；下一步进入 timed conversion Quote 冻结估值。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 C 组 GREEN `5318e5cc2`；若只有本状态校准未提交，先提交它。
4. 下一步只定位并写 Quote 冻结估值首个 RED；不得提前展开 Confirm、request、API 或 UI。
