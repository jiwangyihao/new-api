# Issue #26 状态

## 当前状态

- 阶段：`INFLIGHT_GREEN_HANDOFF_READY`，最窄 public reserve → conversion → final settle 虚拟 exact 快照 GREEN 已通过，待独立 clean 提交交接。
- 最近 clean SHA：`907b78e04`（在途 request RED）；当前 GREEN 提交后更新为最新 SHA。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`git add model/subscription_conversion.go .scratch/agent-progress/issue-26/status.md .scratch/agent-progress/issue-26/evidence.md && git commit -m "feat(issue-26): 固化在途请求虚拟快照"`

提交并确认 staged/unstaged/untracked 全零后立即交接。reserve → conversion → refund 与双连接 conversion/final 串行化尚未实现，禁止冒充完成或继续探索。

## 未提交文件

- `model/subscription_conversion.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- 根因：timed reserve 只记录 source attribution/PreConsumed；conversion 未把它映射为可由 request-aware settle 识别的虚拟 exact deduction snapshot，导致 `AppliedCredit=0`。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 RED `907b78e04`；当前 GREEN 提交后以最新 `feat(issue-26)` 提交为交接点。
4. GREEN 只覆盖 reserve→conversion→final settle 及 conversion 后新 request；refund 与双连接串行化明确未实现。
