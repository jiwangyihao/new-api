# Issue #21 宽回归修复 E：controller Kyren Credit 测试隔离

## 目标

你只负责诊断并修复 `controller/TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation` 在重复进程内运行时偶发 `subscription_required` 的测试隔离缺口。不得改变生产 CreditValuation、BillingSession 或购买语义。

工作树将由协调器从父 HEAD `3e74a2928f7e4b7c3d5c6eae3fbc8362172a4c5d` 显式创建。必读：

- 父 PRD #19、Issue #21、已关闭 #22；
- `docs/agents/credit-operational-value-execution.md`；
- `docs/agents/credit-operational-value-issue-21-wide-regression-contract.md`；
- Issue #21/#22 acceptance 与 #22 browser/Gate C evidence；
- skills：`diagnosing-bugs`、`tdd`、`codebase-design`。

## 冻结 RED

单次 `go test ./controller -count=1` 曾 PASS；合并 A/B/C 后三包同时运行时该测试失败。协调器进一步运行：

```text
go test ./controller -run '^TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation$' -count=10
```

同一进程内部分迭代稳定失败于 `service.PreConsumeBilling`，错误为：

```text
active subscription is required: no active subscription
code=subscription_required
```

在失败前，真实 Kyren snapshot 履约、Credit ledger/state 与 disabled-plan 新购拒绝均通过。高概率根因是测试 `setupKyrenPaymentControllerTestDB`/cleanup 未重置 DB timestamp、Subscription Plan cache、primary billable subscription cache、用户 Redis/cache或其他全局状态；不得先假设，必须用证据定位。

## Change

1. 先创建 `.scratch/agent-progress/issue-21/wide-controller-{status,evidence,contract}.md`，记录冻结 HEAD、RED、所有候选全局状态和恢复命令，提交安全点。
2. 用 `-count=10`（必要时更高）复现并记录第几轮开始失败、数据库中 entitlement/state/setting/request record 的实际状态。
3. 使用 `diagnosing-bugs` 逐一验证全局缓存：`ClearSubscriptionPlanCacheForTest`、`ClearPrimaryBillableSubscriptionCacheForTest`、DB timestamp cache（如现有 public test seam 可用）、Redis/user cache与测试 DB DSN 生命周期。禁止通过 sleep、换随机 user ID、降低 count 或忽略错误掩盖问题。
4. 最小修复应位于 test setup/cleanup 或 test-only helper：每次迭代在新 DB 前后清理本测试会读取的全局状态，并恢复之前的全局值。不得导出只为此测试服务的新生产 API；若现有未导出 seam 无法从 controller 调用，优先在 model 提供语义明确且可被其他测试复用的 `...ForTest` 清理函数，且须证明无生产调用。
5. 保留测试全部业务断言：immutable snapshot、disabled Plan 已授权履约、新购拒绝、一份 ledger/state、request_id 预扣/settlement replay、五接口和邀请隔离。不得删减 BillingSession 路径。
6. 运行完整 controller 包，确认修复不污染其他测试；若仍有与此隔离无关的确定性失败，完整记录并交还协调器，不越界修复。

## Acceptance

至少运行并记录：

```text
go test ./controller -run '^TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation$' -count=25
go test ./controller -run 'TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation|TestKyrenWebhookCompletesSubscriptionOrder|TestSubscriptionBalancePayIdempotent' -count=10
go test ./controller -count=1
git diff --check
git status --short
```

必须 clean tree、小步 Conventional Commits、有效 worker_done。禁止修改前端/i18n、生产业务合同、#23/#26/#27 或 MySQL/PostgreSQL 门禁。
