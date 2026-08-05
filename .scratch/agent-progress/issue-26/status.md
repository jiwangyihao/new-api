# Issue #26 状态

## 当前状态

- 阶段：`INFLIGHT_REFUND_GREEN_VERIFIED`；public reserve → conversion → refund 已通过单次、`-count=10` 与窄 `-race`，下一步仅提交独立 GREEN 安全点并确认 clean。
- 最近 clean SHA：`f83a9eb21`（`test(issue-26): 固化转换在途退款 RED`）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

格式化 `model/credit_valuation.go`，执行 `git diff --check`，提交 `model/credit_valuation.go` 与本目录 `status.md`、`evidence.md`，随后确认 staged、unstaged、untracked 全零。

GREEN 已验证：

- `go test ./model -run TestTimedReserveConversionRefundRestoresVirtualExactSnapshot -count=1`：PASS。
- `go test ./model -run TestTimedReserveConversionRefundRestoresVirtualExactSnapshot -count=10`：PASS。
- `go test -race ./model -run TestTimedReserveConversionRefundRestoresVirtualExactSnapshot -count=1`：PASS。

## 未提交文件

- `model/credit_valuation.go`
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
