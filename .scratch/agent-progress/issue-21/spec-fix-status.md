# Issue #21 AC2 / Gate B Spec 修复状态

## 冻结现场

- 冻结且已通过 Standards 的 HEAD：`763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`。
- 当前分支：`jiwangyihao/issue-21-timed-grants`。
- 初始工作树：staged 0、unstaged 0、untracked 0。
- 唯一 blocker：管理员 timed grant 路径允许调用方提交 `source_price_micros/source_currency`，领域入口在计划 guard 前先用客户端事实规范化，并在 guard 内重读 Plan 后仍继续采用客户端价币，故权威 `40,000,000 CNY` Plan 可被写成 `25,000,000 USD` exact grant。

## 当前阶段

- 状态：`RED_CONFIRMED`。
- Standards 四项 finding：保持 `COMPLETE`，不得回退。
- AC2 / Gate B：Controller/API 权限攻击 RED 已确认；model 权威快照/原子性 RED 待补。

## 权威事实

计划行必须在 `GrantTimedSubscriptionTx` 的同一事务和既有 Plan guard 下重新读取。计划身份/资格及冻结字段至少包括：`id`、`entitlement_type=timed`、`enabled=true`、非 trial/invite-trial、`price_amount_micros`、`currency`、`monthly_token_limit`、`duration_unit/duration_value/custom_seconds`、`quota_reset_period/quota_reset_custom_seconds`，以及现有估值规则版本和结构化 source identity。调用方只表达用户、`plan_id`/计划身份、来源身份、管理员 reason 与 idempotency key；不得控制估值事实。

## 固定锁序

```text
SubscriptionPlan guard -> authoritative SubscriptionPlan reread -> existing timed grant identity -> target UserSubscription -> new TimedSubscriptionValuationGrant
```

重放判断必须使用数据库权威 Plan 事实。已成功来源在 Plan 后续 disabled 时仍返回原 grant；disabled Plan 的新 identity 必须拒绝且零写入。

## 下一步

1. 提交 Controller/API RED 与本进度证据。
2. 补最小 model 权威 Plan 快照与非法 Plan 零写入 RED。
3. 最小 GREEN：领域入口只在 Plan guard 内重读并冻结权威事实，controller 不再向领域层传估值字段。

## 最近安全提交

- `0c7ef4aec`：建立 AC2 / Gate B 修复恢复现场。

## 未提交文件

- `controller/timed_subscription_grant_test.go`：40 CNY Plan 对 25 USD 攻击 RED。
- 本目录 `spec-fix-status.md`、`spec-fix-evidence.md`：RED 证据更新。

## 阻塞

- 无。
