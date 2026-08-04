# Issue #21 AC2 / Gate B Spec 修复状态

## 冻结现场

- 冻结且已通过 Standards 的 HEAD：`763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`。
- 当前分支：`jiwangyihao/issue-21-timed-grants`。
- 初始工作树：staged 0、unstaged 0、untracked 0。
- 唯一 blocker：管理员 timed grant 路径允许调用方提交 `source_price_micros/source_currency`，领域入口在计划 guard 前先用客户端事实规范化，并在 guard 内重读 Plan 后仍继续采用客户端价币，故权威 `40,000,000 CNY` Plan 可被写成 `25,000,000 USD` exact grant。

## 当前阶段

- 状态：`BOUNDARY_REGRESSIONS_GREEN`。
- Standards 四项 finding：保持 `COMPLETE`，未改动。
- AC2 / Gate B：权威来源快照、非法 Plan 失败原子性、零额度拒绝、`[start,end)` 边界秒、成功重放与 disabled 新 key 均已定向 GREEN。

## 权威事实

计划行必须在 `GrantTimedSubscriptionTx` 的同一事务和既有 Plan guard 下重新读取。计划身份/资格及冻结字段至少包括：`id`、`entitlement_type=timed`、`enabled=true`、非 trial/invite-trial、`price_amount_micros`、`currency`、`monthly_token_limit`、`duration_unit/duration_value/custom_seconds`、`quota_reset_period/quota_reset_custom_seconds`，以及现有估值规则版本和结构化 source identity。调用方只表达用户、`plan_id`/计划身份、来源身份、管理员 reason 与 idempotency key；不得控制估值事实。

## 固定锁序

```text
SubscriptionPlan guard -> committed grant identity replay -> authoritative source lock/read -> target UserSubscription -> new TimedSubscriptionValuationGrant
```

管理员新 allocation 在 guard 后重读当前 Plan；订单 source 锁定已成功订单并验证持久化 `EntitlementSnapshot`；兑换 source 锁定已使用兑换记录并读取创建时持久化的 entitlement snapshot。三条路径都在同一事务内验证 user/plan/source identity，调用方不能提供估值字段；已成功来源重放只校验冻结 request identity/reason。

## 下一步

1. 提交四类窄回归与证据安全点。
2. 运行 `-count=10` 权威/边界定向、窄 `-race` 并发、#22 Credit + #21 timed 五接口组合回归。
3. 更新最终 COMPLETE 状态，确认 clean tree 并汇报协调器。

## 最近安全提交

- `4212d2218`：复现管理员伪造计时估值（Controller/API RED）。
- `0c7ef4aec`：建立 AC2 / Gate B 修复恢复现场。
- `014e4e5aa`：提交 model 权威 Plan 快照 RED。
- `57aab92c5`：从权威计划冻结计时授予事实；完成生产/测试调用的 `PlanId` 干净迁移。

## 未提交文件

- `model/timed_subscription_valuation_test.go`：非法 Plan 原子性、zero Credit、半开秒边界回归。
- 本文件与 `spec-fix-evidence.md`：记录四类定向 GREEN。

## 阻塞

- 无。
