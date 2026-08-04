# Issue #21 最终 Spec 缺口收敛修复 Agent 指令

## 任务目标与冻结现场

你负责关闭父 PRD #19、GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」在两次最终聚焦 Spec 复审中确认的全部剩余缺口。工作目录必须复用现有 Orca 工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

开始时必须确认：

- 分支为 `jiwangyihao/issue-21-timed-grants`；
- HEAD 严格等于 `af1f76f6ed006870aa20c4ef5f0b6467016fca6f`；
- `git status --short` 为空；
- 该分支已经包含 Issue #22 的通用 Credit/current_only/权威 micros/BigInt 合同，以及 Issue #21 已通过 Standards 的四项修复；
- 不得 reset、rebase、重新合并父分支、创建替代工作树、关闭 Issue、部署生产或回收工作树。

本任务不是重新设计 timed grant，也不是实现下游 Issue。你只修复下列七个已经有代码证据或可重复失败的 Spec 缺口，并使相关调用链恢复为可审计、确定、原子、幂等的行为。

## 必读资料与 Skills

修改前依次读取并服从：

1. 仓库与自动注入的全局 `AGENTS.md`。
2. `issue://jiwangyihao/new-api/19`、`issue://jiwangyihao/new-api/21`、已关闭的 `issue://jiwangyihao/new-api/22`。
3. `docs/agents/credit-operational-value-execution.md`。
4. `docs/agents/credit-operational-value-wave-1-contract.md`、`credit-operational-value-wave-1-acceptance.md`。
5. `docs/agents/credit-operational-value-issue-21.md`、`credit-operational-value-issue-21-acceptance.md`。
6. `docs/agents/credit-operational-value-issue-21-review-fix.md`、`credit-operational-value-issue-21-spec-fix.md`。
7. `.scratch/agent-progress/issue-21/review-fix-*` 与 `.scratch/agent-progress/issue-21/spec-fix-*`。
8. `C:/Users/34404/AppData/Local/Temp/new-api-issue21-gate-b-focused-spec-rereview.md`。
9. `C:/Users/34404/AppData/Local/Temp/new-api-issue21-gate-b-source-snapshot-adversarial-rereview.md`。
10. `CONTEXT.md`、ADR 0002、2026-08-02 specification/plan 中 timed grant、来源快照、整数金额、事务、幂等与历史 unknown 章节。

必须使用 `skill://diagnosing-bugs` 逐项复现，使用 `skill://tdd` 完成 RED→最小 GREEN，使用 `skill://codebase-design` 保持 Redemption、Plan、订单和 grant 的领域边界。修改导出符号前使用 LSP references。不要使用子 Agent，不要跑 formatter/lint/项目全量测试；只运行本任务需要的定向验证，完整组合门禁由协调器执行。

## 可恢复进度与提交纪律

第一项实际改动必须创建并提交：

- `.scratch/agent-progress/issue-21/final-spec-fix-status.md`
- `.scratch/agent-progress/issue-21/final-spec-fix-evidence.md`
- `.scratch/agent-progress/issue-21/final-spec-fix-contract.md`

三个文件必须记录：冻结 HEAD、七项 finding、每项 RED 的精确命令与旧行为、最小 GREEN、事务/锁序、稳定错误、最近安全提交、当前未提交文件、下一步与阻塞。每完成一个可独立验证的小步就更新并提交，使用 Conventional Commits（英文 type/scope、简体中文 subject）。上下文达到约 80% 前必须先形成 clean 或诚实 WIP 的 `HANDOFF_READY`，不得把成果只留在终端、临时脚本、stash 或大块未提交 diff。

推荐按四个安全提交组推进，但每个 finding 必须在 evidence 中独立列出 RED/GREEN：

1. 权威 Plan duration/reset 严格资格校验；
2. Redemption 不可变来源、资格、mode、disabled 与锁序；
3. paid timed 订单成功重放结果恢复；
4. 最终组合回归与证据收尾。

## 固定修复合同

### Finding 1：权威 timed Plan 的 duration/reset 必须稳定拒绝

`freezeAuthoritativeTimedSubscriptionGrant` 或其唯一资格入口必须在创建/续期权益之前，严格验证数据库权威 Plan：

- duration unit 必须是现有受支持枚举；
- custom duration 必须具有正的 custom seconds；
- 非 custom duration 的相关值必须满足既有 Plan 规则，不得静默归一；
- reset period 必须是现有受支持枚举；
- custom reset 必须具有正的 custom seconds；
- 未知 reset 不能经 `NormalizeResetPeriod` 静默降级为 never。

所有上述非法新管理员 allocation 必须统一满足 `errors.Is(err, ErrTimedSubscriptionGrantInvalid)`，并证明 `UserSubscription=0`、`TimedSubscriptionValuationGrant=0`、计划 guard version 回滚不变。不得依赖错误文本。必须至少覆盖 unknown duration、非正 custom duration、unknown reset、非正 custom reset 的 model 测试，并覆盖一个真实 controller/API 非法 reset 请求，证明不再 HTTP 200 成功写入。

### Finding 2：缺失 Redemption 授权快照时禁止热路径补造 exact

`redemptionFulfillmentFromSourceSnapshot`、`Redeem`、`Redemption.Update` 等任何兑换热路径都不得在 `FulfillmentSnapshot` 缺失或其中 entitlement identity 不完整时，用兑换时的 current Plan 生成并写回 exact 来源快照。

固定语义：

- 新建 Redemption 必须继续在 `Insert` 事务中从当时权威 Plan 冻结 `FulfillmentSnapshot`；
- 合法且确实变更 Plan/模式的前向 `Update` 可以在同一事务、同一锁序中冻结新的授权快照；
- status-only Update 不得补造、刷新或覆盖历史快照；
- 历史/旁路产生的无 snapshot Redemption 在兑换热路径必须返回稳定领域错误并零写入，不能猜当前 Plan、不能标为 exact、不能在本 Issue 内实现 estimated/unknown 历史迁移；历史恢复所有权仍属于 #27。

新增真实 SQLite model/controller 测试：直接构造无 snapshot 的旧 Redemption，Plan 后续改价/改币后兑换必须稳定拒绝，且 Redemption 状态、用户权益、grant、fulfillment 字段都不改变。

### Finding 3：Credit redemption 的当前资格与冻结事实必须分离

不能把 `SubscriptionPlanFromEntitlementSnapshot` 重建出的 plan（其 `Enabled` 默认 false）用于当前资格检查。固定边界：

- currentPlan 只负责当前是否仍允许新兑换：identity、enabled、entitlement type、试用/资格等；
- 创建时持久化的 `FulfillmentSnapshot` 只负责冻结授权事实：价格、币种、Credit、duration/reset、规则和来源；
- timed 与 Credit redemption 都必须遵循这个分离；
- 不修改或绕过 Issue #22 的 `CreditValuation` 深模块、moving-weighted、current_only、请求结算或 ledger 合同。

现有 `TestRedeemCreditBalanceConcurrentClaimPersistsOneGrantAndOneReplay` 必须从稳定 `redemption.plan_ineligible` FAIL 恢复为 GREEN，并以 `-count=10` 证明只有一次 grant/claim、另一次为合法 replay。

### Finding 4：已使用 Redemption 重放必须比较请求 mode

同一 code、同一 user 的成功重放，仅当本次规范化 mode 与持久化 `FulfillmentMode` 一致时才能返回原 fulfillment。timed→credit_balance 或 credit_balance→timed 的 mode 冲突必须返回现有最合适的稳定冲突 sentinel/code，不得返回另一种 fulfillment 的成功响应，不得依赖错误文本，也不得产生任何新写入或续期。

新增双向 mode 冲突测试，并保留相同 mode 重放返回同一 subscription/window/grant 的合同。

### Finding 5：disabled trial / invite-trial 禁止新兑换

`currentPlan.Enabled` 的资格检查必须发生在 trial、invite-trial、paid timed、Credit 等分支之前。disabled Plan 的尚未使用兑换码不得创建任何新权益、grant 或 invitation side effect；已成功兑换的相同 mode replay 仍可返回既有 fulfillment。不得改变“已有 disabled-plan entitlement 仍可消费”的生产边界。

新增 trial 与 invite-trial 的真实 SQLite 测试，断言 disabled 新兑换稳定拒绝且零写入。

### Finding 6：Redemption.Update 必须事务内锁定重读并统一锁序

当前 controller 事务外读取完整 Redemption 后再写回会形成陈旧覆盖风险；Update 的 Plan→Redemption 顺序还与 Redeem 的 Redemption→Plan 顺序冲突。最小修复必须：

1. 在事务内首先按 ID/稳定 identity 锁定并重读 Redemption；
2. 以重读后的当前状态应用允许变更，禁止陈旧 DTO 覆盖并发 Redeem 写入的 used/status/redeemed_time/fulfillment；
3. 需要 Plan 时再按统一 `Redemption → SubscriptionPlan` 顺序锁定 Plan；
4. status-only Update 不读取 Plan 来补历史 snapshot；
5. Plan/模式真实变更时才按当前权威 Plan 冻结新的前向 snapshot；
6. 与 Redeem 并发的结果必须属于合法串行化集合，不得恢复已使用 code、丢失 fulfillment 或死锁。

优先修改 model 深模块接口，让 controller 只表达允许变更的意图。新增真实文件 SQLite 多连接的确定性交错或等价并发测试，至少重复 `-count=10`；若修改并发接缝，运行窄 `go test -race`。MySQL/PostgreSQL 实机仍由 #27 验收，不得宣称本任务通过。

### Finding 7：普通 paid timed 订单成功回调重放恢复原结果

`subscriptionOrderCompletionResultFromExistingFulfillmentTx` 对无 invitation event 的普通 paid timed 订单，不能只返回空的 `Transitioned`。成功重放必须从持久化 `FulfilledSubscriptionID` 与匹配的 immutable timed grant/source identity 恢复原：

- subscription identity；
- `[start,end)` window；
- grant/source 事实或既有结果 DTO 所要求的字段；
- `Transitioned=false` 或现有等价的“未再次迁移”语义。

不得读取当前 Plan 推断窗口，不得再次续期或新增 grant。若持久化 identity 与 grant 不一致，返回稳定领域错误而不是伪造结果。新增一个没有 invitation reward event 的普通 paid timed 订单测试：首次完成后再调用完成路径，返回同一 subscription/window，subscription/grant 计数不变。

## 必须保留的既有行为

- 管理员入口继续只接受 user/plan_id/reason/idempotency_key，并从 guard 内 current enabled Plan 冻结权威事实；客户端旧价币字段无效。
- 已授权订单继续使用购买时 `SubscriptionOrder.EntitlementSnapshot`；Plan 后续改价、改币或 disabled 不撤销已授权履约。
- 新 Redemption 的 `Insert` 与合法 Plan/模式 Update 继续冻结当时授权快照；后续 current Plan 改价不得回写该事实。
- 已通过的四项 Standards 修复保持不变：并发同源 grant 线性化、权威 micros 聚合/排序、checked int64 overflow、稳定 immutable sentinel。
- Issue #22 的 CreditValuation、32 CNY tracer、current_only warning、权威 micros sorter、BigInt UI 与 BillingSession 合同保持不变。
- 不新增 schema，不修改前端/i18n，不实现 #23 request settlement、#24 正向 ingress、#25 recovery、#26 FX/conversion、#27 migration marker/ready、#28 release。

## 验收命令与证据

每项先运行能在当前 `af1f76f6...` 稳定失败的最小测试，记录准确失败，再实现 GREEN。至少执行并记录：

1. duration/reset model 资格矩阵 `-count=10` 与 controller 非法 reset API 测试；
2. 无 snapshot redemption、Credit redemption 并发、mode 冲突、disabled trial/invite-trial、status-only Update 与并发 Update/Redeem 定向测试；
3. paid timed order 无 invitation event 的首次完成/成功 replay 测试 `-count=10`；
4. `go test ./model -run 'TimedSubscription|Redeem|Redemption|SubscriptionOrder' -count=1` 的合理窄集合；
5. `go test ./controller -run 'TimedSubscription|Redeem|Redemption' -count=1` 的合理窄集合；
6. 如触及并发接缝，窄 `go test -race`；
7. #22 32 CNY/current_only/micros 与 #21 timed CNY/USD 五接口组合定向回归；
8. `git diff --check` 与最终 clean tree。

不要运行项目全量测试、前端大套件、formatter/lint 或浏览器；本任务不改 UI。所有外部临时复现必须转成仓库内行为测试，不能把临时脚本当交付物。

## 完成条件

完成前必须：

1. 将 `final-spec-fix-*` 标记 `COMPLETE`，逐项列出 RED、GREEN、稳定错误、事务/锁序、提交 SHA 和非目标；
2. 每个业务改动和对应测试均已提交，工作树 staged/unstaged/untracked 全部为 0；
3. 只发送一次当前 Dispatch 注入 capability 的 `worker_done`，正文包含最终 HEAD、提交列表、七项结果、测试、未运行项和范围声明；
4. 不合并父分支、不关闭 GitHub Issue、不部署、不回收工作树。
