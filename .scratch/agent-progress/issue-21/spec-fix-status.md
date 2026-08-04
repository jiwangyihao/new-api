# Issue #21 AC2 / Gate B Spec 修复状态

## 冻结现场

- 冻结且已通过 Standards 的 HEAD：`763b0f40bdc8fb7d5c11bc69f46749fd40a8763b`。
- 当前分支：`jiwangyihao/issue-21-timed-grants`。
- 初始工作树：staged 0、unstaged 0、untracked 0。
- 唯一 blocker：管理员 timed grant 路径允许调用方提交 `source_price_micros/source_currency`，领域入口在计划 guard 前先用客户端事实规范化，并在 guard 内重读 Plan 后仍继续采用客户端价币，故权威 `40,000,000 CNY` Plan 可被写成 `25,000,000 USD` exact grant。

## 当前阶段

- 状态：`COMPLETE`。
- Standards 四项 finding：保持 `COMPLETE`，未改动。
- AC2 / Gate B：管理员伪造价币已从根因关闭；admin/order/redemption 三类来源的权威事实、失败原子性、半开秒边界、零额度、重放/disabled 及 #22 组合合同均已验证。

## 权威事实

计划行必须在 `GrantTimedSubscriptionTx` 的同一事务和既有 Plan guard 下重新读取。计划身份/资格及冻结字段至少包括：`id`、`entitlement_type=timed`、`enabled=true`、非 trial/invite-trial、`price_amount_micros`、`currency`、`monthly_token_limit`、`duration_unit/duration_value/custom_seconds`、`quota_reset_period/quota_reset_custom_seconds`，以及现有估值规则版本和结构化 source identity。调用方只表达用户、`plan_id`/计划身份、来源身份、管理员 reason 与 idempotency key；不得控制估值事实。

## 固定锁序

```text
SubscriptionPlan guard -> committed grant identity replay -> authoritative source lock/read -> target UserSubscription -> new TimedSubscriptionValuationGrant
```

管理员新 allocation 在 guard 后重读当前 Plan；订单 source 锁定已成功订单并验证持久化 `EntitlementSnapshot`；兑换 source 锁定已使用兑换记录并读取创建时持久化的 entitlement snapshot。三条路径都在同一事务内验证 user/plan/source identity，调用方不能提供估值字段；已成功来源重放只校验冻结 request identity/reason。

## 完成证据

1. Controller/model 权威快照定向 `-count=10` PASS；并发接缝 `-race` PASS。
2. 非法 Plan 零写入、zero Credit、`[start,end)`、成功重放/disabled 新 key PASS。
3. #22 32 CNY Credit/current_only/权威 micros 与 #21 timed 五接口组合 PASS；前端 BigInt 两文件 7 tests PASS。
4. MySQL/PostgreSQL 未运行，仍归 Issue #27；未修改 UI/i18n，未运行浏览器。

## 安全提交

- `4212d2218`：复现管理员伪造计时估值（Controller/API RED）。
- `0c7ef4aec`：建立 AC2 / Gate B 修复恢复现场。
- `014e4e5aa`：提交 model 权威 Plan 快照 RED。
- `57aab92c5`：从权威计划冻结计时授予事实并收窄调用接口。
- `30079605e`：迁移兑换回归到权威来源语义。
- `3f58584bb`：恢复订单不可变履约快照。
- `66a9c9c46`：固化订单与兑换授权快照并迁移来源夹具。
- `39ee1b3b4`：覆盖非法 Plan 原子性、zero Credit 与边界秒。
- `bcc0468ef`：持久化兑换创建/更新时的授权计划快照。

## 未提交文件

- 本文件与 `spec-fix-evidence.md` 最终 COMPLETE 证据；提交后必须为 0。

## 阻塞

- 无。
