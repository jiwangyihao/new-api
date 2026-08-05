# Issue #26 状态

## 当前状态

- 阶段：`CONVERSION_IDEMPOTENCY_GREEN_CLEANUP`，同 source/key 权威价格变化已稳定冲突且零写入，单次与 count=10 通过，待提交 clean 安全点。
- 最近 clean SHA：`28b77ba73`（conversion 权威事实冲突 RED）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`git add model/errors.go model/subscription_conversion.go .scratch/agent-progress/issue-26/status.md .scratch/agent-progress/issue-26/evidence.md && git commit -m "feat(issue-26): 拒绝转换权威事实冲突"`

提交并确认 staged/unstaged/untracked 全零后停止。并发双确认另起 TDD 周期；本提交禁止并发测试、在途 request、API/UI 或额外抽象。

## 未提交文件

- `model/errors.go`
- `model/subscription_conversion.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已显式恢复 conversion 冻结纵切，覆盖此前 `FX_VECTORS_HANDOFF_READY` 停止指令。
- parser/provider 是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- GREEN 新增稳定 `ErrConversionIdempotencyConflict`；重放返回前核对 committed conversion 冻结价格/currency/basis 与当前权威 source plan，冲突直接返回且阻止 committed fallback 覆盖错误。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 RED `28b77ba73`；提交当前 GREEN 后以最新 `feat(issue-26)` 提交为恢复点。
4. 提交后停止；不得提前实现并发双确认或进入其他范围。
