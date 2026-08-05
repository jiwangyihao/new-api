# Issue #26 状态

## 当前状态

- 阶段：`FX_IDENTITY_REVERSE_GREEN_CLEAN`，B 组 RED/GREEN 已独立提交且工作树已验证 clean。
- 最近安全 SHA：`c4d419e0e`（`feat(issue-26): 冻结 FX 双向比率快照`）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

保持工作树 clean 并向协调器报告 B 组安全点。最新指令禁止进入 C 组或 conversion；收到明确后续指令前不扩展实现。

B 组无需重做：table-driven RED 为 `b9b3098c9`，最小 GREEN 为 `c4d419e0e`，`count=10`、联合定向回归、gofmt 与 diff-check 已完成。

## 未提交文件

- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已明确暂停 conversion、request、API/UI 探索，先只完成 FX A/B/C 三组。
- parser/provider 必须是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- B 组 GREEN 已在 `c4d419e0e` clean 提交；禁止重复制造同一 RED，也禁止进入 C 组或 conversion。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 B 组 GREEN `c4d419e0e`；若只有本状态校准未提交，先提交它。
4. 完成状态校准后保持 clean 并报告协调器；不得进入 C 组或 conversion。
