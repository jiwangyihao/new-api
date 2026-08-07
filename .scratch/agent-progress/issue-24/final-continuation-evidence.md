# Issue #24 最终续作证据

## 冻结现场（2026-08-07）

- `git rev-parse HEAD` → `c7c983d02f2161f52a9a815a452dc7d950f692fc`。
- `git status --porcelain=v1` → 无输出。
- 当前分支 → `jiwangyihao/issue-24-final`。
- `git merge-base --is-ancestor b8598f4b7add27ba237f30dec6ceae7968cc2aa3 HEAD` → 成功。
- `git merge-base --is-ancestor 49b1ece48 HEAD` → 成功。
- `git merge-base --is-ancestor 79f3f221e HEAD` → 成功。
- `orca worktree current --json` → `parentWorktreeId` 为 `.../.workspaces/new-api/credit-operational-value-integration`，当前 head 与 Git 一致。
- 近期祖先包含：`b8598f4b7` 路由冻结合同、`5a2c12698` request→target 锁序、`88fc07a02` 锁序回归、`49b1ece48` redemption H2、`79f3f221e` admin increase H2。

## 已读取权威资料

- `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/24`。
- `CONTEXT.md`。
- `docs/adr/0001-credit-balance-entitlement.md`、`0002-credit-operational-remaining-value.md`。
- `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md` 全文。
- `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md` 全文。
- `docs/agents/credit-operational-value-execution.md`。
- `docs/agents/credit-operational-value-issue-24.md`、`credit-operational-value-issue-24-acceptance.md`。
- `docs/agents/credit-operational-value-wave-2-contract.md`、`credit-operational-value-wave-2-acceptance.md`。
- `.scratch/agent-progress/issue-20/contract.md`、`issue-22/contract.md`。
- `.scratch/agent-progress/issue-24/{contract,status,evidence}.md`。

## 已确认的既有 H2 证据

- redemption H2：CNY→USD、USD→CNY、Option 变化冻结重放、invalid FX、ledger failure 原子回滚均已在既有 evidence 中记录为 GREEN；安全提交 `49b1ece48`。
- admin increase H2：双向 FX、Option 变化冻结重放、invalid FX、ledger failure 回滚均已 GREEN；安全提交 `79f3f221e`。
- 既有 `-count=10` H2 稳定组通过；同币种管理员全组通过；未修改 #26 `credit_fx_rate.go`、`credit_valuation.go`、Option 生命周期。
- 本续作不得把这些历史记录冒充新的 API/browser 证据；新证据必须来自本轮实际命令和真实请求。

## 实际范围声明

- 当前尚未运行新的 API、analytics、frontend 或 browser 验收。
- MySQL/PostgreSQL 实机验收不属于 #24，完整矩阵归 #27；本轮只做跨库静态兼容与真实 SQLite。
- 所有后续命令、关键请求/响应、RED/GREEN、浏览器观察、提交 SHA 与清理结果将持续追加到本文件。
