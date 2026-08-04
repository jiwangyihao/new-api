# Issue #21 旧夹具迁移 C：controller 支付与邀请订单

## 任务目标

你负责修复冻结 Issue #21 分支中 `controller` 包的旧余额、Kyren、Stripe、Epay 与邀请订单测试夹具。新合同要求：购买请求/订单在创建或授权时冻结合法 `SubscriptionOrder.EntitlementSnapshot`；timed Plan 必须具有权威 `price_amount_micros`、币种、正 Credit、合法 duration/reset；成功订单回调从不可变快照履约。旧测试直接创建缺快照订单或不完整 Plan，现返回 `timed_subscription_grant_invalid`，必须迁移夹具而非放宽生产 fail-closed。

工作树由协调器创建为冻结 `issue-21-timed-grants` 的 Orca 子工作树，基线必须包含 `774b35740c1879b285537031410731317d0142fc`。共享合同：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration/docs/agents/credit-operational-value-issue-21-fixture-migration-contract.md`

## 必读材料与 Skills

读取自动注入规则、父 PRD #19、Issue #21/#22、共享合同、执行协议、Issue #21 acceptance、ADR/spec 的订单快照/支付回调/timed grant/disabled-plan 段，以及冻结树 `spec-fix-*`、`final-spec-fix-*`。使用 `skill://diagnosing-bugs` 与 `skill://tdd`；共享测试 helper 的接口设计需读 `skill://codebase-design`。禁止子 Agent、项目全量 formatter/lint/前端套件。

## 精确所有权

优先范围：

- `controller/subscription_balance_purchase_test.go`
- `controller/subscription_payment_kyren_test.go`
- `controller/subscription_payment_stripe_test.go`
- Epay/通用 payment webhook 对应 `_test.go`
- `controller/invitation_entitlement_test.go`
- 与上述测试共用的 controller `_test.go` setup/helper

可根据 `go test ./controller -count=1` 的真实失败补充明确相关的 controller 测试文件。不得修改 `model`/`service` 目录、前端、locale 或生产支付/履约代码，除非可重复测试证明新生产缺陷且先通过 Orca `question` 获得协调器授权。

## 必须完成的行为

1. 先运行 `go test ./controller -count=1`，记录每个 `timed_subscription_grant_invalid`、panic、缺表日志的具体测试与堆栈，按余额/Kyren/Stripe/Epay/邀请订单分组。
2. 为测试 Plan 提供完整权威事实：显式 timed entitlement、enabled、非 trial/invite-trial、`price_amount_micros` 非 NULL、CNY/USD、正 `monthly_token_limit`、合法 duration unit/value/custom seconds、合法 reset period/custom seconds、稳定 business code。不得仅设置兼容 `PriceAmount float64`。
3. 对“通过真实购买入口创建订单”的测试，优先让现有 controller purchase helper 自然冻结 `EntitlementSnapshot`；不要再手写缺快照成功订单。
4. 对必须从“已授权 provider order”开始的 webhook 测试，使用代码库已有订单快照构造 helper或从权威 Plan 生成 `SubscriptionEntitlementSnapshot` 并通过模型的合法插入/创建入口持久化。必须断言快照非空、identity 与 Plan 一致、精确 micros/币种/duration/reset 正确。
5. 保留关键合同：
   - 余额、Kyren、Stripe、Epay 各自原有签名/金额/provider 幂等/HTTP 状态断言；
   - 已授权成功订单即使当前 Plan 后续 disabled/改价，仍按购买快照履约；
   - 新购买 disabled Plan 仍拒绝；
   - webhook 重放返回同一 subscription/window，不新增 grant；
   - 邀请订单奖励/entitlement 原断言不减少。
6. 不得把 `EntitlementSnapshot` 设为任意 `{}`、零值或仅为绕过 nil 检查的假数据。必须是与订单授权时 Plan 相符的完整快照。
7. 对 Redis 全局夹具 panic：先判断是否由本路测试 setup 污染/缺少恢复。若属于本路，确保测试保存并恢复全局 DB/Redis/setting 状态，不能依赖测试顺序；若属于无关测试，用 `question` 交给协调器。缺表 teardown 若因本路 setup 未迁移必要表，最小补 AutoMigrate；不要吞日志。
8. 运行每组最小 RED→GREEN，再运行：
   - `go test ./controller -run 'Balance|Kyren|Stripe|Epay|Payment|Invitation|SubscriptionOrder' -count=1`
   - 关键 provider/重放测试 `-count=10`
   - `go test ./controller -count=1`

## 可恢复进度

第一项改动创建并提交：

- `.scratch/agent-progress/issue-21/fixture-c-status.md`
- `.scratch/agent-progress/issue-21/fixture-c-evidence.md`
- `.scratch/agent-progress/issue-21/fixture-c-contract.md`

按 provider/夹具组小步提交，例如 `test(subscription): 迁移支付订单授权快照夹具`。进度必须列出每个失败测试的迁移结果和仍红项。上下文约 80% 前形成 clean/HANDOFF_READY。

## 验收与非目标

验收：旧夹具在冻结基线真实 RED；完整 Plan/订单快照迁移后余额、Kyren、Stripe、Epay、邀请订单合同 GREEN；关键 count=10；controller 包级通过；diff-check；clean tree；有效 worker_done。

非目标：修改生产支付、timed grant、CreditValuation、service/model 测试、前端/i18n、三数据库实机、部署。Agent 不合并、不关闭 Issue、不回收工作树。
