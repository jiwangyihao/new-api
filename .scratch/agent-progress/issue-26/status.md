# Issue #26 状态

## 当前状态

- 阶段：`FX_PARSER_RED`，首个公共行为测试已按预期因接口缺失失败。
- 最近安全 SHA：`0c0f540b7f39cda3769d81f1c1983ffb922b7823`。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`go test ./model -run TestParseCreditFXRateSnapshotCanonicalizesUSDtoCNY -count=1`

当前 RED 已观测；提交本 RED 后，最小 GREEN 只实现 `CreditFXRateSnapshotInput`、`CreditFXRateSnapshot`、`CreditFXDirectionUSDtoCNY` 与 `ParseCreditFXRateSnapshot`，使 `7.300000` 规范约分为 `73/10`。

## 未提交文件

- `model/credit_fx_rate_test.go`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；尚无 Issue #26 运行时代码。
- 最新协调指令要求不重新通读全部材料，安全提交后直接进入 FX parser 首个 RED。
- parser/provider 必须是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- 首个 RED 只覆盖 USD → CNY 规范十进制解析、约分和方向；非法值、identity、反向换算、floor 与 overflow 必须后续逐条 RED→GREEN。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 若三份文件尚未提交，执行“下一条命令”；若已提交，以日志中最新 `docs(issue-26)` SHA 为安全点。
4. 首个 RED 已由 `go test ./model -run TestParseCreditFXRateSnapshotCanonicalizesUSDtoCNY -count=1` 证明；最小 GREEN 只实现该测试要求的公共行为。
