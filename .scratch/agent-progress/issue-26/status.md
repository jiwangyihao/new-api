# Issue #26 状态

## 当前状态

- 阶段：`CONVERSION_CONCURRENT_REPLAY_GREEN_CLEANUP`，真实文件 SQLite 同 source/key 双连接并发测试现实现直接 GREEN，count=10 与窄 race 均通过。
- 最近 clean SHA：`28fca8b98`（幂等 GREEN 安全点校准）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`git add model/subscription_conversion_valuation_test.go .scratch/agent-progress/issue-26/status.md .scratch/agent-progress/issue-26/evidence.md && git commit -m "test(issue-26): 验证转换并发幂等"`

该测试无需生产修复：确定性 barrier 下两个同 source/key Confirm 返回同一 conversion，一次首次、一次 replay，且 conversion/source/ledger/state 各唯一。提交并确认 clean 后停止；禁止不同 source、在途 request、API/UI。

## 未提交文件

- `model/subscription_conversion_valuation_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- 并发同事实测试现实现直接 GREEN，无生产代码修改；真实文件 SQLite WAL、两个连接、after_quote barrier、count=10 与窄 race 均已验证。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `28fca8b98`；提交当前并发测试后以最新 `test(issue-26)` 提交为恢复点。
4. 提交后停止；禁止不同 source、在途 request、API/UI。
