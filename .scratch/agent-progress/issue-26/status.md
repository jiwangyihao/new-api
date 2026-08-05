# Issue #26 状态

## 当前状态

- 阶段：`INFLIGHT_REQUEST_RED`，conversion 并发安全点已 clean 提交，开始 public reserve → conversion → final settle 的在途 request 行为 RED。
- 最近 clean SHA：`3d2baf4c7`（`test(issue-26): 验证转换并发幂等`）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

完成本 status/evidence 校准提交后，只定位并调用现有 public reserve 与 final settle 入口，创建一条真实 SQLite 确定性交错 RED：timed request reserve 后转换，再 settle 同一 request，必须保留 original subscription_id/request deduction snapshot、不改写到 Credit、不重复扣减；conversion 后新请求才进入 Credit。

RED 必须以真实行为断言失败，不得用编译失败；提交前禁止生产 GREEN、refund/并发扩展、API/UI。

## 未提交文件

- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- conversion 原子纵切已在 `3d2baf4c7` clean 提交；当前仅允许一条在途 request reserve→conversion→settle 行为 RED。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `3d2baf4c7`；若只有本校准未提交，先提交它。
4. 下一步只创建在途 request 行为 RED；禁止生产 GREEN、refund/并发扩展、API/UI。
