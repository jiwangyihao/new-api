# Issue #26 状态

## 当前状态

- 阶段：`CONVERSION_SAME_CURRENCY_RED_READY`，真实 SQLite 同币种 Confirm RED 已成立、格式检查通过，等待独立提交。
- 最近安全 SHA：`9c3e5ca6f`（转换 RED 起点状态提交）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`git add model/subscription_conversion_valuation_test.go .scratch/agent-progress/issue-26/status.md .scratch/agent-progress/issue-26/evidence.md && git commit -m "test(issue-26): 固化同币种转换估值 RED"`

该测试只验证现有可持久化 `SubscriptionConversion`、`CreditBalanceLedger`、`CreditValuationState` 与同币种 FX `1/1`；不新增未定义 schema。提交并确认 clean 后停止，等待下一派发。

## 未提交文件

- `model/subscription_conversion_valuation_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- RED 到达真实 Confirm → Grant seam并返回 `credit_valuation_source_required`；断言范围仅为现有持久化字段和同币种 `1/1`，未设计额外 schema。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `9c3e5ca6f`；提交当前 RED 后以最新 `test(issue-26)` 提交为恢复点。
4. 提交后停止并等待下一派发；禁止实现 GREEN、跨币种、在途 request 或 API/UI。
