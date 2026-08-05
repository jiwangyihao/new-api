# Issue #26 状态

## 当前状态

- 阶段：`CONVERSION_SAME_CURRENCY_GREEN_CLEANUP`，同币种 Confirm 最小 GREEN 与定向回归已通过，待提交 clean 安全点。
- 最近安全 SHA：`778e57a5e`（同币种 RED 证据收敛）；RED 测试提交为 `47a86598f`。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`git add model/subscription_conversion.go model/credit_balance.go .scratch/agent-progress/issue-26/status.md .scratch/agent-progress/issue-26/evidence.md && git commit -m "feat(issue-26): 冻结同币种转换估值"`

提交并确认 staged/unstaged/untracked 全零后停止等待；禁止进入跨币种、并发、在途 request 或 API/UI。

## 未提交文件

- `model/subscription_conversion.go`
- `model/credit_balance.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- GREEN 只连接锁定后的 source plan 精确 micros/currency、credit basis/gross credit 与目标 valuation currency；沿原事务复用 `CreditValuationSourceSnapshot` ingress 并冻结既有 conversion/ledger/state 字段。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `778e57a5e`；提交当前 GREEN 后以最新 `feat(issue-26)` 提交为恢复点。
4. 提交后停止等待；禁止进入下一阶段。
