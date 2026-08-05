# Issue #26 状态

## 当前状态

- 阶段：`BASELINE_VERIFIED`，待提交三份开工恢复文件。
- 最近安全 SHA：`60e71da8d5be73816dd7c892b0d4f96768db98b3`。
- 工作分支：`jiwangyihao/issue-26-conversion-fx`。
- 当前工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`。
- Orca parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`。
- 父分支/baseRef：`jiwangyihao/credit-operational-value-integration`。

## 下一条命令

`git add .scratch/agent-progress/issue-26 && git commit -m "docs(issue-26): 固化转换与 FX 恢复合同"`

提交后立即进入 FX parser 的第一个 RED：仅增加一个通过公共接口验证规范十进制解析、约分与快照方向的失败测试，不预写 GREEN。

## 未提交文件

- `.scratch/agent-progress/issue-26/contract.md`
- `.scratch/agent-progress/issue-26/status.md`
- `.scratch/agent-progress/issue-26/evidence.md`

## 上下文风险

- 本次为正确 lineage 的全新 Dispatch `ctx_7b66c7730806`；旧 Dispatch `ctx_74254621cf66` 已失败，禁止复用旧 attempt 结论。
- 基线后的两条提交仅为 Issue #26 调度/恢复文档；尚无 Issue #26 运行时代码。
- 最新协调指令要求不重新通读全部材料，安全提交后直接进入 FX parser 首个 RED。
- parser/provider 必须是唯一运行时 FX seam，禁止触碰 `float64 USDExchangeRate`。
- 首个 RED 前需用定向检索定位现有 Credit value 包和测试惯例；不得在 RED 同步实现 GREEN。

## 恢复入口

1. 运行 `git status --short --branch`，确认分支与 clean/预期未提交文件。
2. 读取本目录 `contract.md`、`status.md`、`evidence.md`。
3. 若三份文件尚未提交，执行“下一条命令”；若已提交，以日志中最新 `docs(issue-26)` SHA 为安全点。
4. 只定位 FX seam 的现有包/测试惯例，写一个公共行为 RED 并运行对应窄测试，记录真实失败输出。
