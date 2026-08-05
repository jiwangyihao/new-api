# Issue #26 状态

## 当前状态

- 阶段：`CONVERSION_CONCURRENT_REPLAY_NEXT`，权威事实冲突 GREEN 已 clean 提交，准备同 source/key 双连接并发测试。
- 最近 clean SHA：`7a899945d`（`feat(issue-26): 拒绝转换权威事实冲突`）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

完成本 status/evidence 校准提交后，在现有 conversion valuation 测试文件新增真实文件 SQLite 并发测试：确定性 barrier 同时触发两个同 source/key Confirm，要求两调用返回同一 conversion，且 conversion/source/ledger/state 各唯一。

先运行并诚实记录现实现 RED 或 GREEN，再决定是否需要最小修复；禁止不同 source、在途 request、API/UI。

## 未提交文件

- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- 权威事实冲突 GREEN 已在 `7a899945d` clean 提交；下一安全点严格只覆盖同 source 同事实双连接并发。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `7a899945d`；若只有本校准未提交，先提交它。
4. 下一步只写并发同事实测试；禁止不同 source、在途 request、API/UI。
