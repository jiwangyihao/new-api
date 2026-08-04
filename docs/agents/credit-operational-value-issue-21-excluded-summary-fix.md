# Issue #21 生产修复：excluded timed summary 聚合

## 任务目标

你只负责修复 Fixture A 在迁移到不可变 `TimedSubscriptionValuationGrant` 后暴露的一个真实生产缺陷：运营分析 paid-subscription summary 的顶层 excluded 金额仍直接读取普通 `row.Value.RecognizedRemainingValueMicros`，因此 timed entitlement 虽然已经在 `row.TimedValue.ByCurrency` 中得到正确的 33 CNY，`ExcludedRemainingValueByCurrency` 却返回 0。

工作树必须复用：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-fixture-a-model`

起始 clean HEAD 必须为 `8c428160d54a04921a566d3e0a6005f442c0fca4`。该 HEAD 已完成 6 个旧 paid-value fixture 的不可变 grant 迁移，其中 5 个 GREEN；唯一保留的 RED 是：

`TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers`，expected 33 / actual 0。

不要重做 Fixture A 已完成的测试迁移，也不要合并 B/service 或 C/controller 分支。

## 必读材料与 Skills

按顺序读取：

1. 自动注入的项目/全局 `AGENTS.md`。
2. 父 PRD `issue://jiwangyihao/new-api/19`、Issue `#21`、已关闭的 `#22`。
3. `C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration/docs/agents/credit-operational-value-issue-21-fixture-migration-contract.md`。
4. 本文件。
5. 当前工作树 `.scratch/agent-progress/issue-21/fixture-a-{status,evidence,contract}.md`。
6. `docs/agents/credit-operational-value-issue-21-acceptance.md`、ADR 0002、2026-08-02 spec 中 excluded analytics 与 timed grant 规则。

必须使用 `skill://diagnosing-bugs` 先复现；使用 `skill://tdd` 完成 RED→GREEN；若需要判断 helper 边界，读取 `skill://codebase-design`。不要派生子 Agent，不要运行项目全量 formatter/lint/跨数据库套件。

## 已冻结的根因和所有权

当前生产接缝位于：

`model/admin_analytics_paid_subscription.go`

`adminBuildPaidSubscriptionValueDataFromRows` 在 `row.Excluded` 分支直接执行等价于：

`excluded.addMicros(currency, row.Value.RecognizedRemainingValueMicros)`

这对 Credit/non-timed row 有效，但 timed row 的权威逐币种 recognized 值位于 `row.TimedValue.ByCurrency`。同文件现有 `adminPaidRowAccumulateRecognized` 已区分 timed 与 non-timed，并按 `amount_micros` 进入 `adminMoneyAccumulator`。优先复用既有 helper，不新增第二套金额聚合规则。

你拥有的生产代码范围仅限修复这一个顶层 excluded summary 接缝；测试范围仅限 `model/admin_analytics_paid_subscription_test.go` 及必要的同目录窄 `_test.go` helper、进度文件。不得修改 invitation analytics、DTO schema、前端、locale、CreditValuation 深模块、request settlement、FX、migration marker、ready/suspended 门禁或任何 #23–#28 范围。

## 必须完成的 TDD

1. **冻结基线**
   - 核对 HEAD 与 clean tree。
   - 创建并尽快提交：
     - `.scratch/agent-progress/issue-21/excluded-summary-fix-status.md`
     - `.scratch/agent-progress/issue-21/excluded-summary-fix-evidence.md`
     - `.scratch/agent-progress/issue-21/excluded-summary-fix-contract.md`
   - 写明本任务只修生产聚合，不降低 fixture 的 expected=33 断言。

2. **真实 RED**
   - 运行：
     `go test ./model -run '^TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers$' -count=1`
   - 必须记录 expected 33 / actual 0 的原始失败。
   - 证明 paid row 的 `TimedValue.ByCurrency` 已含 33 CNY，而顶层 excluded accumulator 忽略它。可通过窄测试断言或现有 evidence 证明；不要加生产日志。

3. **最小 GREEN**
   - 让顶层 excluded summary 使用同一个 timed-aware recognized 聚合 helper。
   - non-timed/Credit 路径仍按权威 `RecognizedRemainingValueMicros`。
   - timed 路径逐币种累加 `TimedValue.ByCurrency[*].RecognizedMicros`。
   - 保留 `included`、`include_excluded`、`excluded_only` 三种模式语义；excluded 金额不得进入主 recognized，总 active paid count 不得变化。
   - 保留 accumulator 的 checked integer overflow/fail-closed 行为；禁止 float64、字符串转 float、当前 Plan fallback 或新 API。

4. **行为验证**
   - 原 RED 单测单次和 `-count=10` 均 PASS。
   - 至少运行相关组合：
     `go test ./model -run 'PaidSubscriptionValueExcluded|PaidSubscriptionValueEmptyExcluded|PaidSubscriptionValueIncludesPaidSourcesWithoutOrders' -count=10`
   - 运行：
     `go test ./model -run 'PaidSubscriptionValue|AdminPaidSubscription|TimedSubscription' -count=1`
   - 运行 `git diff --check`。
   - 可以运行 `go test ./model -count=1` 观察宽回归，但 B/C 及其他旧授权夹具尚未合入；必须逐项诚实记录，不能把它们冒充为本任务失败，也不能越界修复。

5. **提交与交付**
   - 生产修复和对应测试/证据使用小步 Conventional Commit，subject 使用简体中文。
   - 最终 `git status --short` 必须为空。
   - 更新三份 progress，列出 RED、GREEN、命令、提交、仍由 A/B/C 或 #27 拥有的风险。
   - 用注入的 task/dispatch capability 发送一次有效 `worker_done --outcome succeeded`，然后停止执行。

## 禁止事项

- 不得把 expected 33 改为 0，不得删除或弱化 excluded 断言。
- 不得回退到 `SubscriptionPlan.PriceAmount` 或任何 binary float 计算。
- 不得给 timed row 填造普通 `row.Value` 以绕过根因。
- 不得修改已集成 #22 的 Credit/current_only/权威 micros/BigInt sorter 合同。
- 不得新增 schema、迁移、兼容 shim、marker/ready 写入。
- 不得处理 `invitation_commission_test.go`、`payment_method_guard_test.go`、service/controller 夹具；它们由其他冻结分支负责。
- 不得合并父分支、关闭 Issue 或操作生产环境。

## 验收信号

完成时必须同时满足：原 expected 33 / actual 0 测试在旧实现有真实 RED；最小生产修复后单次与重复运行 GREEN；included/excluded 模式和 Credit/non-timed 路径不回归；金额全程整数 micros 且 overflow fail-closed；工作树 clean；证据可审计；有效 worker_done 已送达。