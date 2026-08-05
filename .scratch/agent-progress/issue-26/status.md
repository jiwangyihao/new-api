# Issue #26 状态

## 当前状态

- 阶段：`INFLIGHT_REFUND_RED_OBSERVED`；public reserve → conversion → refund 行为已稳定失败，等待独立 RED 安全点提交后才允许分析并实现 GREEN。
- 最近 clean SHA：`2f7c804c5`（`docs(issue-26): 校准退款续作安全点`）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

独立提交 `TestTimedReserveConversionRefundRestoresVirtualExactSnapshot` RED；随后仅分析 `SettleUserSubscriptionRequestTarget(..., 0, true)` 返回稳定 `credit_valuation_state_mismatch` 的根因并实现最小 GREEN。

已执行：`go test ./model -run TestTimedReserveConversionRefundRestoresVirtualExactSnapshot -count=1`。

精确旧行为：reserve 与 conversion 均成功，虚拟 exact snapshot 已冻结；refund 进入 public request-target 入口后返回 `*errors.errorString("credit_valuation_state_mismatch")`，未到达恢复目标 Credit 的断言。

## 未提交文件

- `model/subscription_conversion_settlement_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 当前有效 Dispatch：task `task_b6c2c840cda6`、dispatch `ctx_cdf46ee2f559`；只通过注入的 Orca orchestration capability 与协调器通信。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- 根因：timed reserve 只记录 source attribution/PreConsumed；conversion 未把它映射为可由 request-aware settle 识别的虚拟 exact deduction snapshot，导致 `AppliedCredit=0`。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点为 `85660501c`（`feat(issue-26): 固化在途请求虚拟快照`）。
4. GREEN 只覆盖 reserve → conversion → final settle 及 conversion 后新 request；refund 与双连接串行化仍未完成。
