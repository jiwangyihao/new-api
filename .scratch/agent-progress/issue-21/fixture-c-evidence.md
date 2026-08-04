# Issue #21 夹具迁移 C 证据

## 冻结基线

- `git rev-parse HEAD` → `774b35740c1879b285537031410731317d0142fc`。
- `git branch --show-current` → `jiwangyihao/issue-21-fixture-c-controller`。
- `git status --short` → 无输出，初始工作树 clean。

## 已读取材料

- 父 PRD #19、Issues #21/#22。
- `docs/agents/credit-operational-value-execution.md`。
- `docs/agents/credit-operational-value-wave-1-contract.md`。
- `docs/agents/credit-operational-value-issue-21.md` 与 acceptance。
- 共享夹具迁移合同。
- `CONTEXT.md`、ADR 0002、2026-08-02 spec。
- `.scratch/agent-progress/issue-21/spec-fix-*` 与 `final-spec-fix-*`。
- Skills：`diagnosing-bugs`、`tdd`、`codebase-design`、Orca orchestration/CLI。

## 待捕获 RED

尚未运行包级基线。首条诊断命令固定为：

```text
go test ./controller -count=1
```

必须记录每个 `timed_subscription_grant_invalid`、panic、缺表日志的测试名、错误与堆栈，并按余额、Kyren、Stripe、Epay/通用支付、邀请订单分类。之后为每组记录最小 RED→GREEN 命令、关键 provider/重放 `-count=10`、聚焦正则回归和完整 controller 包级结果。
