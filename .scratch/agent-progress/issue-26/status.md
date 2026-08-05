# Issue #26 状态

## 当前状态

- 阶段：`FX_INVALID_INPUTS_NEXT`，首个 parser RED/GREEN 已独立提交且工作树已验证 clean。
- 最近安全 SHA：`bb399d868`（`feat(issue-26): 实现 FX 快照规范解析`）。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

为 A 组新增单一 table-driven 公共行为测试，覆盖 FX rate 缺失、空值、非法十进制、超精度、零/负数、不支持币种与方向不匹配；运行定向测试取得真实 RED，不包含 identity、反向换算、floor 或 overflow。

A 组按独立 RED 提交 → 最小 GREEN 提交推进；随后 B 组 identity/反向、C 组 floor/overflow，均保持相同节奏。

## 未提交文件

- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；Issue #26 首个 FX parser 已在后续 `58866ae7b` RED 与 `bb399d868` GREEN 中落地。
- 协调器已明确暂停 conversion、request、API/UI 探索，先只完成 FX A/B/C 三组。
- parser/provider 必须是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- 已完成的首个 GREEN 只覆盖 USD → CNY 的 `7.300000` 规范解析、约分和方向，并暂用单一导出 sentinel `ErrCreditFXRateInvalid`。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 最近 clean 安全点是 `bb399d868`；若只有本进度修正未提交，先提交它。
4. 从 A 组非法输入 RED 开始；禁止提前扩展 B 组 identity/反向、C 组 floor/overflow 或 conversion/request/API/UI。
