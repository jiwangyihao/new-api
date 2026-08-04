# Issue #21 七项最终 Spec 聚焦复评指令

## 冻结现场与只读边界

你是 GitHub Issue #21「固化计时权益 grant 时间线与多币种分析」的最终只读 Spec 复评 Agent。只读工作树：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`

开始和结束必须确认：

- 分支为 `jiwangyihao/issue-21-timed-grants`；
- HEAD 严格等于 `774b35740c1879b285537031410731317d0142fc`；
- `git status --short` 为空；
- 已含 #22 通用 Credit/current_only/权威 micros/BigInt 合同与此前通过的四项 Standards 修复；
- 评审范围以已含 #22 的父基线 `2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc` 到冻结 HEAD 的最终状态为准。

禁止编辑、格式化、提交、stash、reset、切换分支、启动服务、写数据库、运行项目大套件或派生子 Agent。冻结状态漂移必须 escalation。MySQL/PostgreSQL 未实测不得写成通过。

## 必读材料与方法

1. 读取 `skill://review`，只执行 Spec 轴。
2. 阅读父 PRD `issue://jiwangyihao/new-api/19`、子 Issue `issue://jiwangyihao/new-api/21`、已关闭的 `issue://jiwangyihao/new-api/22` 及相关评论。
3. 阅读集成父树中的：
   - `CONTEXT.md`
   - `docs/adr/0002-credit-operational-remaining-value.md`
   - `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
   - `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md`
   - `docs/agents/credit-operational-value-wave-1-contract.md`
   - `docs/agents/credit-operational-value-wave-1-acceptance.md`
   - `docs/agents/credit-operational-value-issue-21.md`
   - `docs/agents/credit-operational-value-issue-21-acceptance.md`
   - `docs/agents/credit-operational-value-issue-21-final-spec-fix.md`
4. 阅读冻结树 `.scratch/agent-progress/issue-21/final-spec-fix-{contract,status,evidence}.md`，但必须回到实现和测试核验。
5. 阅读此前两份聚焦失败报告：
   - `C:/Users/34404/AppData/Local/Temp/new-api-issue21-gate-b-focused-spec-rereview.md`
   - `C:/Users/34404/AppData/Local/Temp/new-api-issue21-gate-b-source-snapshot-adversarial-rereview.md`
6. 只做冻结 diff、实现、测试和已有证据的聚焦复评。允许运行能裁决单一 finding 的小型定向测试；禁止重复大套件。

## 必须逐项裁决的七个缺口

每项必须给出 `PASS` 或 `FAIL`，引用具体文件、符号和测试；不能只复述 evidence。

### 1. 权威 Plan duration/reset

核验 `freezeAuthoritativeTimedSubscriptionGrant` 或等价唯一入口严格拒绝未知/非法 duration、非正 custom duration、未知 reset、非正 custom reset；统一满足 `errors.Is(err, ErrTimedSubscriptionGrantInvalid)`，并在创建/续期前事务零写入。管理员非法 reset API 不得再 HTTP 200 成功。

### 2. 缺失 Redemption 授权快照

核验 Redemption 热路径在 `FulfillmentSnapshot` 缺失或 entitlement identity 不完整时稳定拒绝且零写入，不能按 current Plan 补造 exact。只有新建 `Insert` 和合法 Plan/模式变更的前向 `Update` 可冻结新快照；status-only 不得补历史。

### 3. Credit 当前资格与冻结事实分离

核验 `currentPlan` 只负责当前 identity/enabled/type/资格，持久化 snapshot 只提供冻结价格、币种、Credit、duration/reset、规则与来源；不得用 snapshot-derived plan 的默认 `Enabled=false` 误拒绝合法 Credit redemption，也不得绕过 #22 CreditValuation 深模块。

### 4. 成功重放 mode 一致性

核验已使用 Redemption 仅在请求规范化 mode 与持久化 `FulfillmentMode` 一致时返回原结果；timed↔credit_balance 双向冲突稳定拒绝、零写入，相同 mode 重放仍返回相同 fulfillment。

### 5. disabled trial / invite-trial

核验 `currentPlan.Enabled` 在所有新兑换分支和 invitation side effect 前统一检查；disabled trial/invite-trial 不创建权益、grant 或 invitation event。既有成功兑换同 mode 重放与已有 disabled entitlement 消费边界不得回归。

### 6. Redemption.Update 事务与锁序

核验 Update 在事务内先锁并重读 Redemption，只应用允许变更；需要 Plan 时再遵循 `Redemption → SubscriptionPlan` 锁序。陈旧 DTO 不得覆盖并发 Redeem 的 used/status/redeemed_time/fulfillment；status-only 不读 Plan、不补 snapshot；并发结果属于合法串行化集合。

### 7. 普通 paid timed 订单成功重放

核验无 invitation event 的普通 paid timed 订单成功重放从持久化 `FulfilledSubscriptionID` 与匹配的 immutable timed grant/source identity 恢复原 subscription 和 `[start,end)`，`Transitioned=false`，不得读取 current Plan 推断窗口、不得再次续期或新增 grant；identity 不一致必须稳定失败。

## 必须专门裁决的宽回归风险

冻结 evidence 诚实记录：

`go test ./model -run 'TimedSubscription|Redeem|Redemption|SubscriptionOrder' -count=1`

命中 `invitation_commission_test.go`、`payment_method_guard_test.go` 中缺 `EntitlementSnapshot` 的旧夹具，以及清理阶段缺表日志而未通过。你必须回到具体失败测试和生产调用链判断：

- 若这些夹具模拟的是真实合法订单/兑换路径，而新合同要求其通过冻结入口构造授权快照，则这是当前 #21 合同必须更新的 blocker；不得称为“既有噪声”。
- 若失败测试只依赖明确非法的旁路历史状态，且生产路径已经在创建时冻结 snapshot、拒绝旧旁路是本次明确合同，则说明为何测试应改为断言稳定拒绝或为何不属于本切片。
- 清理缺表日志若会掩盖真实失败、污染并行/重复测试或造成 false positive/negative，必须报告；若仅是测试 teardown 噪声，也要给出可核验依据。
- 不得因为窄测试通过而忽略这些失败，也不得未经分析要求修复无关邀请/支付业务。

## 组合合同与范围边界

同时确认：

- 已通过的四项 Standards 修复保持：同源并发 grant 线性化、权威 micros 聚合/排序、checked int64 overflow、稳定 immutable sentinel。
- #22 的 32 CNY tracer、Credit current_only warning、权威 micros sorter、BigInt UI、BillingSession 与 CreditValuation 合同没有被绕过或改写。
- 管理员入口仍只接受 user/plan_id/reason/idempotency_key，并从 guard 内 current enabled Plan 冻结权威事实；已授权订单仍使用购买时 entitlement snapshot。
- 没有新增 schema、前端/i18n改动，也没有实现 #23 request settlement、#24 ingress、#25 recovery、#26 FX/conversion、#27 migration marker/ready 或 #28 release。

## 输出与完成

把不超过 900 字的最终报告写入：

`C:/Users/34404/AppData/Local/Temp/new-api-issue21-seven-findings-spec-final-review.md`

报告必须包含：冻结范围、总评 `PASS`/`FAIL`、七项逐项结论、宽回归风险裁决、#22 组合结论、findings（严重级别、条款、文件/符号、可复现证据）、未实测说明。推断标 `[INFERENCE]`；无 finding 写“0 findings”。

结束前再次确认冻结 HEAD 与 clean tree。随后使用当前 Dispatch 注入 capability 发送恰好一次有效 `worker_done`，正文包含 PASS/FAIL、finding 数、最严重项、宽回归裁决、#22 组合结论与报告绝对路径。读取失败、冻结漂移或风险未裁决不得报告成功。
