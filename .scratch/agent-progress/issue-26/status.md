# Issue #26 状态

## 当前状态

- 阶段：`FX_FLOOR_OVERFLOW_GREEN`，C 组整数 floor/overflow 测试、重复验证与窄 race 已通过，待提交独立 GREEN 安全点。
- 最近安全 SHA：`a783ff3c1`（C 组整数换算 RED）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`git add model/credit_fx_rate.go .scratch/agent-progress/issue-26/status.md .scratch/agent-progress/issue-26/evidence.md && git commit -m "feat(issue-26): 实现 FX 整数安全换算"`

提交并确认 clean 后报告协调器；C 组完成前未进入 conversion/request/API/UI。

## 未提交文件

- `model/credit_fx_rate.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已明确暂停 conversion、request、API/UI 探索，先只完成 FX A/B/C 三组。
- parser/provider 必须是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- C 组 GREEN 使用现有定宽 `bits.Mul64`/`bits.Div64` 路径，无 `float64` 或大整数热路径分配；`count=10`、窄 `-race`、A/B/C 联合回归与 diff-check 均通过。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 C 组 RED `a783ff3c1`；提交当前 GREEN 后以最新 `feat(issue-26)` 提交为恢复点。
4. 当前只允许完成 C 组 clean 提交并报告；提交前未进入 conversion/request/API/UI。
