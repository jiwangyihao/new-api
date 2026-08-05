# Issue #26 状态

## 当前状态

- 阶段：`CONVERSION_RED_IN_PROGRESS`，显式派发恢复现有 Confirm/SubscriptionConversion 纵切，先写真实 SQLite 冻结估值 RED。
- 最近 clean HEAD：`6e158b0d0`（仅比协调器所指 `fd6d316f7` 多一条 FX 交接 status/evidence 提交）；FX 业务 GREEN 为 `5318e5cc2`。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

定向定位 `ConfirmTimedSubscriptionConversion`、`SubscriptionConversion` 及其真实 SQLite 测试 fixture；新增单一 RED，验证同一事务冻结 source tier 精确价格/currency/credit/duration/reset/rule、数量公式、FX snapshot、目标 valuation currency，并在失败时零写入。

本纵切禁止在途 request、Issue #24 API/UI、Issue #25 recovery、Issue #27/#28；先完成 1/1 tracer RED→GREEN，再逐条加入跨币种、幂等/冲突、原子失败与并发。

## 未提交文件

- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- Conversion 必须保留 `full_31_day_blocks × credit_basis + current_remaining_credit = gross_credit`，不按部分周期比例且不双计 current credit。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 当前 clean HEAD 为 `6e158b0d0`；`fd6d316f7` 到该提交仅为 FX 交接 status/evidence，不回退已提交历史。
4. 下一步只写真实 SQLite conversion 冻结估值 RED；不得进入在途 request、API/UI、recovery 或迁移/部署。
