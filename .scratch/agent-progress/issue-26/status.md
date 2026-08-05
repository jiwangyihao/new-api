# Issue #26 状态

## 当前状态

- 阶段：`CONVERSION_CROSS_CURRENCY_GREEN_CLEANUP`，CNY→USD 与 USD→CNY 冻结估值、Option 变化后重放及冲突零写入 tracer 已通过，待提交 clean 安全点。
- 最近 clean SHA：`3989d26c7`（跨币种 conversion RED）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`git add model/credit_fx_rate.go model/credit_valuation.go model/option.go model/subscription_conversion.go model/credit_balance.go model/subscription_conversion_valuation_test.go .scratch/agent-progress/issue-26/status.md .scratch/agent-progress/issue-26/evidence.md && git commit -m "feat(issue-26): 冻结跨币种转换估值"`

提交并确认 staged/unstaged/untracked 全零后停止等待。当前安全点不声称完成同 source 权威事实指纹冲突或并发双确认；按最新指令禁止在本提交扩展并发幂等、在途 request、API/UI 与 Issue #24/#25/#27/#28。

## 未提交文件

- `model/credit_fx_rate.go`
- `model/credit_valuation.go`
- `model/option.go`
- `model/subscription_conversion.go`
- `model/credit_balance.go`
- `model/subscription_conversion_valuation_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- GREEN 通过唯一 FX provider 从持久化 Option 原始十进制原子发布 snapshot；Confirm 冻结有理数、captured_at 与整数 floor，并沿既有 ingress 写入 conversion/ledger/state。并发同事实确认与同 source 权威事实指纹冲突明确留待下一安全点。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 RED `3989d26c7`；提交当前 GREEN 后以最新 `feat(issue-26)` 提交为恢复点。
4. 提交后停止等待；禁止自行进入并发幂等、在途 request、API/UI 或其他 Issue 范围。
