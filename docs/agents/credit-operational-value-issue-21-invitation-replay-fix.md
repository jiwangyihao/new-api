# Issue #21 生产修复：成功订单重放恢复邀请身份

## 目标与冻结现场

你只负责修复 Issue #21 宽回归夹具迁移后暴露的一个真实生产缺陷：普通 paid timed 订单首次履约成功时已经持久化 `InvitationRewardEvent.InviterId`，但后续成功重放通过 `subscriptionOrderCompletionResultFromExistingFulfillmentTx` → `subscriptionOrderCompletionResultFromTimedGrantTx` 只恢复 subscription/window，没有把已持久化邀请身份合回结果，导致 `InviterId` 从首次结果的 `9201`/`9231` 退化为 `0`。

必须复用工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-wide-model-fix`

当前 clean HEAD 应为 `86b49a724e32b1dfea3b43a25f73e03efb8584b7`；其中业务夹具与稳定 RED 提交是 `7b9e0038e`，`86b49a724` 只补充了 clean handoff 证据。不要 reset、rebase 或丢弃两者。

## 必读材料与 Skills

按顺序读取：

1. 自动注入的项目级与全局 `AGENTS.md`。
2. 父 PRD `issue://jiwangyihao/new-api/19`、Issue `#21`、已关闭的 `#22`。
3. `C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration/docs/agents/credit-operational-value-execution.md`。
4. `C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration/docs/agents/credit-operational-value-issue-21-fixture-migration-contract.md`。
5. 本文件。
6. 当前树 `.scratch/agent-progress/issue-21/wide-model-{status,evidence,contract}.md`。
7. `docs/agents/credit-operational-value-issue-21-acceptance.md`、ADR 0002、2026-08-02 spec 中订单履约、邀请事件与幂等重放合同。

必须使用 `skill://diagnosing-bugs` 复现并定位；使用 `skill://tdd` 完成 RED→GREEN；使用 `skill://codebase-design` 判断“恢复既有结果”应放置的领域接缝。不要派生子 Agent。不要一开始运行项目全量 formatter、lint、前端或跨数据库套件。

## 已冻结的业务事实

现有两个稳定 RED 位于 `model/invitation_commission_test.go`：

- 首次完成已持久化 `InvitationRewardEvent.InviterId=9201`，订单 `FulfilledSubscriptionID > 0`，唯一 timed grant 的 `UserSubscriptionId` 与 event `SourceSubscriptionId` 都等于该 fulfillment；成功重放返回 `InviterId=0`。
- 另一成功重放首次返回 `InviterId=9231`，重放同样返回 `0`。

精确生产接缝在 `model/subscription.go`：

- `subscriptionOrderCompletionResultFromExistingFulfillmentTx`
- `subscriptionOrderCompletionResultFromTimedGrantTx`

当前 timed-grant 恢复只应提供不可变 subscription/window；邀请身份必须来自已经持久化的 `InvitationRewardEvent`，不得从当前用户关系、当前 Plan、当前邀请设置或请求参数重新推导。

## 唯一允许的实现范围

生产代码只允许修改 `model/subscription.go` 中上述成功重放结果恢复链，以及确有必要的同目录私有 helper。测试只允许修改 `model/invitation_commission_test.go`、必要的同目录 test-only helper和本任务 progress 文件。

必须保持：

1. 首次履约流程、InvitationRewardEvent 创建语义与佣金计算不变。
2. 重放只读取既有 event；不得新建或更新 event、subscription、timed grant、ledger、订单授权快照。
3. event 存在且 `SourceSubscriptionId` 对应本订单 immutable fulfillment 时，结果恢复原 `InviterId`。
4. event 不存在时保持 `InviterId=0`，不得把 absence 当错误。
5. 若存在多个不合法或不匹配事件，不得猜测“最近一条”；应遵守现有唯一来源身份/稳定查询合同并 fail closed 或返回既有稳定错误。
6. 不得读取 current Plan 来重算 window、price、currency、duration、reset 或 rule。
7. 幂等重放不得改变 event/subscription/grant/order 数量，也不得改变 `FulfilledSubscriptionID`。
8. 保留已完成的 #21 duration/reset、Redemption、并发、overflow、stable sentinel 修复，以及 #22 Credit/current_only/权威 micros/BigInt 合同。

禁止修改 `service`、`controller`、前端、i18n、schema、migration marker/ready、FX、CreditValuation 深模块或任何 #23–#28 范围。

## TDD 与持久化要求

立即创建并尽快提交：

- `.scratch/agent-progress/issue-21/invitation-replay-fix-status.md`
- `.scratch/agent-progress/issue-21/invitation-replay-fix-evidence.md`
- `.scratch/agent-progress/issue-21/invitation-replay-fix-contract.md`

写明当前 HEAD、稳定 RED、修改文件、最近安全提交、未提交文件、测试命令和下一步。每个可验证小步使用 Conventional Commit；type/scope 英文，subject 简体中文。上下文到约 80% 前必须形成 clean 或诚实 WIP 的 `HANDOFF_READY`。

### RED

先运行并记录两个成功重放测试的原始失败。至少证明：

- 首次结果与持久化 event 的 InviterId 非零；
- fulfillment、grant、event source identity 一致；
- 重放结果 InviterId 错误为 0；
- 重放前后 event/subscription/grant 数量不变。

不要修改断言来制造 GREEN。

### 最小 GREEN

在既有 fulfillment-result 恢复事务中读取与本 fulfillment/source identity 对应的 `InvitationRewardEvent`，只把其原 `InviterId` 合入返回值。优先使用现有 query/唯一来源字段；不要新增 schema、缓存、兼容 shim 或第二套邀请计算。

### 验证

至少执行并记录：

1. 两个成功订单重放测试单次与 `-count=10`。
2. 九项 `invitation_commission_test.go` 已迁移组合，确保七项既有 GREEN 不回归。
3. 新增/保留一个无 invitation event 的 paid timed 成功重放，断言 `InviterId=0` 且不报错。
4. 重放前后 event/subscription/timed grant/order 计数与 `FulfilledSubscriptionID` 不变。
5. `go test ./model -count=1`。
6. 必要的窄 `go test -race`（只覆盖本接缝；若平台/驱动限制必须诚实记录）。
7. `gofmt` 仅格式化修改的 Go 文件；`git diff --check`；最终 `git status --short` 为空。

MySQL/PostgreSQL 实机零 SKIP 属于 #27，本任务只运行真实 SQLite/model 回归，不得冒充三数据库通过。

## 完成与生命周期

完成后更新三份 progress，列出 RED/GREEN、最终 HEAD、提交、文件、测试、未运行项和范围声明。使用当前 Dispatch 注入 capability 发送一次有效 `worker_done --outcome succeeded`，然后停止操作。不要自行合并父分支、关闭 Issue、部署或删除工作树。
