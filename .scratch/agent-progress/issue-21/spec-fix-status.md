# Issue #21 AC2 / Gate B Spec 修复状态

## 冻结现场

- 冻结且已通过 Standards 的 HEAD：`763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`。
- 当前分支：`jiwangyihao/issue-21-timed-grants`。
- 初始工作树：staged 0、unstaged 0、untracked 0。
- 唯一 blocker：管理员 timed grant 路径允许调用方提交 `source_price_micros/source_currency`，领域入口在计划 guard 前先用客户端事实规范化，并在 guard 内重读 Plan 后仍继续采用客户端价币，故权威 `40,000,000 CNY` Plan 可被写成 `25,000,000 USD` exact grant。

## 当前阶段

- 状态：`AUTHORIZED_SOURCE_SNAPSHOTS_GREEN`。
- Standards 四项 finding：保持 `COMPLETE`，未改动。
- AC2 / Gate B：管理员新 allocation 从 guard 内当前 enabled Plan 冻结；订单从持久化 `SubscriptionOrder.EntitlementSnapshot`、兑换从创建时持久化 `Redemption.FulfillmentSnapshot` 冻结已授权事实。

## 权威事实

计划行必须在 `GrantTimedSubscriptionTx` 的同一事务和既有 Plan guard 下重新读取。计划身份/资格及冻结字段至少包括：`id`、`entitlement_type=timed`、`enabled=true`、非 trial/invite-trial、`price_amount_micros`、`currency`、`monthly_token_limit`、`duration_unit/duration_value/custom_seconds`、`quota_reset_period/quota_reset_custom_seconds`，以及现有估值规则版本和结构化 source identity。调用方只表达用户、`plan_id`/计划身份、来源身份、管理员 reason 与 idempotency key；不得控制估值事实。

## 固定锁序

```text
SubscriptionPlan guard -> committed grant identity replay -> authoritative source lock/read -> target UserSubscription -> new TimedSubscriptionValuationGrant
```

管理员新 allocation 在 guard 后重读当前 Plan；订单 source 锁定已成功订单并验证持久化 `EntitlementSnapshot`；兑换 source 锁定已使用兑换记录并读取创建时持久化的 entitlement snapshot。三条路径都在同一事务内验证 user/plan/source identity，调用方不能提供估值字段；已成功来源重放只校验冻结 request identity/reason。

## 下一步

1. 提交 order/redemption 已授权来源快照 GREEN 安全点。
2. 补非法权威 Plan 零写入、`[start,end)` 边界秒、零额度拒绝、成功重放与 disabled 新 key 四类窄回归。
3. 运行重复定向及 #22 Credit + #21 timed 组合回归。

## 最近安全提交

- `4212d2218`：复现管理员伪造计时估值（Controller/API RED）。
- `0c7ef4aec`：建立 AC2 / Gate B 修复恢复现场。
- `014e4e5aa`：提交 model 权威 Plan 快照 RED。
- `57aab92c5`：从权威计划冻结计时授予事实；完成生产/测试调用的 `PlanId` 干净迁移。

## 未提交文件

- `model/redemption.go`：兑换创建事务持久化 entitlement source snapshot，兑换事务复用该冻结事实。
- `model/timed_subscription_valuation.go`：按 admin/order/redemption source 读取各自权威事实。
- `model/timed_subscription_valuation_test.go`、`model/timed_subscription_valuation_concurrency_test.go`：真实来源回归与直接 admin 夹具。
- 本文件与 `spec-fix-evidence.md`：纠正来源快照范围并记录兑换 RED→GREEN。

## 阻塞

- 无。
