# Issue #26 状态

## 当前状态

- 阶段：`FX_VECTORS_HANDOFF_READY`，FX A/B/C 向量已完成、验证并独立提交；协调器要求停止 conversion/request/API/UI 工作并等待交接。
- 最近安全 SHA：`fd6d316f7`（`docs(issue-26): 校准 FX 整数换算安全点`）；业务 GREEN 为 `5318e5cc2`。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

无代码命令。保持工作树 clean，向协调器报告 `FX_VECTORS_HANDOFF_READY`，等待新的显式派发。

禁止自行进入 timed conversion Quote/Confirm、在途 request、API 或 UI；此前只做过定向查看，未修改这些区域。

## 未提交文件

- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器最新要求仅校准为 `FX_VECTORS_HANDOFF_READY` 后停止，覆盖此前进入 timed conversion Quote RED 的恢复建议。
- parser/provider 是唯一运行时 FX seam，未触碰 `float64 USDExchangeRate`。
- FX parser/snapshot 已完成 A/B/C：稳定非法错误、identity/双向倒数、确定性冻结、overflow-safe floor。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点为状态提交 `fd6d316f7`，FX 业务 GREEN 为 `5318e5cc2`；若只有本交接校准未提交，先提交它。
4. 校准提交后保持 clean 并等待协调器；不得进入 conversion/request/API/UI。
