# Issue #26 状态

## 当前状态

- 阶段：`INFLIGHT_REQUEST_RED`，public reserve → conversion → final settle 真实 SQLite tracer 已产生确定性行为失败。
- 最近 clean SHA：`d6ce04c9c`（`docs(issue-26): 启动在途请求 RED`）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`go test ./model -run TestTimedReserveConversionFinalSettlePreservesOriginalRequestSnapshot -count=1`

RED 已在 conversion 后读取原 request record 时失败：`AppliedCredit` 期望 `10`、实际 `0`，证明 timed reserve 尚未在 conversion 事务内获得虚拟 exact deduction snapshot。提交测试与证据后停止；禁止实现 GREEN、refund/并发扩展、API/UI。

## 未提交文件

- `model/subscription_conversion_settlement_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- RED 保持 original `UserSubscriptionId=sourceID` 与 `PreConsumed=10`，但 conversion 后 `AppliedCredit` 仍为 `0`；缺口精确位于 conversion 未给既有 timed request 固化虚拟 exact snapshot。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `d6ce04c9c`；本 RED 提交后以最新 `test(issue-26)` 提交为恢复点。
4. 提交后停止等待下一派发；禁止生产 GREEN、refund/并发扩展、API/UI。
