# Issue #26 证据

## 2026-08-05 — Lineage 与 Git 基线自检

### Git 分支与工作树

命令：`git status --short --branch`

观测：

- branch：`jiwangyihao/issue-26-conversion-fx`
- staged：`0`
- unstaged：`0`
- untracked：`0`

命令：`git rev-parse HEAD`

观测：`60e71da8d5be73816dd7c892b0d4f96768db98b3`

### 已验收实现祖先

命令：`git merge-base HEAD fd4d4683bc3b3b2cdd78c8e5c851c58263e61971`

观测：`fd4d4683bc3b3b2cdd78c8e5c851c58263e61971`

### 基线后的提交范围

命令：`git log --oneline fd4d4683bc3b3b2cdd78c8e5c851c58263e61971..HEAD`

观测仅有两条调度/Agent 文档提交：

- `60e71da8d docs(agents): 修正 Issue 26 开工基线规则`
- `5409da885 docs(agents): 补充 Issue 26 调度与恢复边界`

### Orca 原生 lineage

命令：`orca status --json`

观测：runtime `ready`，runtimeId `ce771a98-c24e-477f-b2bd-fcccd853bd5b`，Orca `1.4.170`。

命令：`orca worktree current --json`

观测：

- worktree id：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-26-conversion-fx`
- head：`60e71da8d5be73816dd7c892b0d4f96768db98b3`
- branch：`refs/heads/jiwangyihao/issue-26-conversion-fx`
- parentWorktreeId：`1bd24578-ec8b-4492-961c-108ab229f4e7::C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration`
- parent capture source：`explicit-cli-flag`
- parent confidence：`explicit`
- baseRef：`jiwangyihao/credit-operational-value-integration`

结论：五项开工前置条件全部符合；允许创建 Issue #26 的首批持久化文件。此时尚未修改运行时代码。

## TDD 记录

尚未开始。安全提交后首个周期必须为：单一 FX parser 公共行为测试 → 观察真实 RED → 更新本文件并提交 RED。
