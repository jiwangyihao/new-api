# Issue #23 兼容字段双计数收敛修复指令

## 任务目标

你负责修复父 PRD #19、GitHub Issue #23 冻结候选中的唯一已确认宽回归 blocker：

`service/TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields`

当前稳定失败为：

- 数据库订阅使用量：`100`；
- 兼容字段期望：`100`；
- 兼容字段实际：`199`。

冻结工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-23-request-settlement`

起始 HEAD 必须严格等于：

`9b496ca0d46bad84b4977d63496a668388e99080`

开始时确认分支为 `jiwangyihao/issue-23-request-settlement`、`git status --short` 为空。不得 reset、rebase、切换分支、合并父树、关闭 GitHub Issue、部署或操作其他工作树。

## 必读材料与技能

开始前读取：

1. `skill://diagnosing-bugs`；
2. `skill://tdd`；
3. `skill://codebase-design`；
4. 父 PRD `issue://jiwangyihao/new-api/19` 与子 Issue `issue://jiwangyihao/new-api/23`；
5. `docs/agents/credit-operational-value-issue-23-acceptance.md`；
6. `.scratch/agent-progress/issue-23/` 中最终 contract/status/evidence 与 cleanup 证据；
7. `service/billing_session.go` 中 `BillingSession.Reserve`、`reserveFunding`、结算与 RelayInfo 同步逻辑；
8. `service/funding_source.go` 中 `SubscriptionFunding.Settle`；
9. `service/subscription_billing_test.go` 中失败测试。

不要重新探索整个仓库。当前直接证据已经定位到：

- `BillingSession.Reserve` 调用 `reserveFunding(delta)`；
- subscription 分支最终调用 `SubscriptionFunding.Settle(delta)`；
- `SubscriptionFunding.Settle` 已更新 `AmountUsedAfter` 或 `TokenUsedAfter/TokenRemaining`；
- `BillingSession.Reserve` 随后再次更新同一批兼容字段，造成双计数。

## 执行与恢复协议

修改生产代码前，创建并提交：

- `.scratch/agent-progress/issue-23/double-count-fix-contract.md`
- `.scratch/agent-progress/issue-23/double-count-fix-status.md`
- `.scratch/agent-progress/issue-23/double-count-fix-evidence.md`

必须记录冻结 HEAD、失败命令/输出、单一所有者不变量、当前安全提交和下一动作。每个 RED/GREEN 安全点都要立即更新证据并小步提交。若上下文达到约 80%，先提交 clean HANDOFF_READY，不要无界探索。

## 修复合同

### 1. 先证明 RED

复跑且记录：

```text
go test ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=1
```

不得修改或降低现有 `expected 100` 断言。必要时增加最小测试，至少覆盖：

- 初始预扣 10，再 `Reserve(100)`，数据库目标与所有兼容读模型都恰为 100；
- 重复 `Reserve(100)` 或更小目标严格无操作；
- reserve 失败时数据库、funding/session 兼容字段和 RelayInfo 均不发生部分更新；
- amount-based 与 distributor-token 路径保持各自单位，不互相污染；
- Credit request-aware 路径仍将同一 request target 更新为 100，而不是 190/199；
- timed/legacy 兼容路径不回归。

### 2. 单一所有者不变量

优先采用最小、深模块清晰的修复：

- `SubscriptionFunding.Settle` 继续作为资金来源持久化及 funding 内部兼容快照（`AmountUsedAfter`、`TokenUsedAfter`、`TokenRemaining`、Credit target）的单一所有者；
- `BillingSession.Reserve` 在 `reserveFunding(delta)` 成功后，不得再次累加这些已由 funding 更新的字段；
- `BillingSession.Reserve` 仍只负责确有必要的 session 账本，例如 `sub.preConsumed`、`s.preConsumedSubscription`、`s.preConsumedQuota`、`s.extraReserved`，每项恰好增加一次，并只同步一次 RelayInfo；
- 若代码事实证明必须选择另一单一所有者，可以采用，但必须在 contract/evidence 中证明所有调用点和失败原子性，不能同时由两层写同一状态。

不得通过减 1、特殊判断当前测试值、修改断言、吞错误、浮点补偿或事后覆盖 RelayInfo 掩盖根因。

### 3. 保持 Issue #23 已完成合同

必须保留：

- Credit 稳定 `request_id` 与累计 target；
- request-aware 预扣、实时追加、final/refund；
- shared-transaction coalescer 的稳定顺序和整批回滚；
- Task `subscription_request_id` 与 deterministic `legacy-task:<pk>`；
- cleanup eligibility、Task 投影/引用保护、稳定批次、原子性、诊断和审计保留；
- timed 匿名 delta 仅保留既有兼容范围。

禁止实现或修改：

- #24 兑换/管理员正向入账；
- #25 destructive recovery；
- #26 FX/conversion；
- #27 marker/migration；
- #28 release；
- 前端、i18n、数据库 schema 或清理策略。

## 验收

修复后至少执行并记录：

```text
go test ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=10
go test ./service -run 'SubscriptionBilling|CreditBillingSession|Task.*Billing' -count=1
go test ./model ./service ./controller -count=1
go test -race ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=1
git diff --check
```

并复跑 Issue #23 已有的 request/coalescer/Task/cleanup 聚焦门禁，确认不回归。若包级测试出现其他失败，必须区分本次回归与既有噪声并提供直接证据；本 blocker 未修复前不得报告完成。

最终要求：

- 代码已 gofmt；
- progress/evidence 完整；
- 所有修复小步已提交；
- `git status --short` 为空；
- 使用当前 Dispatch 注入 capability 发送恰好一次有效 `worker_done`，说明最终 HEAD、RED/GREEN、三包宽回归、窄 race、范围边界和未实测项。

MySQL/PostgreSQL 实机、全项目测试、部署仍不属于本修复 Agent；不得冒充通过。
