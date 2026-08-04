# Issue #21 AC2 / Gate B Spec 修复合同

## 深模块接口

管理员 controller 只允许提交意图：目标用户、`plan_id`、`reason`、`idempotency_key`。领域入口只接受调用者可证明的稳定身份（用户、计划 identity、source type/id/key、管理员 reason/idempotency）；价币、Credit、期限、reset、billing/pricing rule、窗口、confidence 均由领域模块从数据库权威 Plan 与实际创建结果派生。

兼容 JSON 若包含旧 `source_price_micros/source_currency`，服务端必须忽略且测试证明不能影响 grant；内部 Go request 不保留这些估值参数。不得把客户端值传入领域层再比较后采用。

## 事务与 Plan 权威读取

固定顺序：

```text
SubscriptionPlan guard
-> authoritative SubscriptionPlan reread
-> existing timed grant identity
-> target UserSubscription
-> new TimedSubscriptionValuationGrant
```

同一事务内必须验证 Plan identity、`entitlement_type=timed`、`enabled=true`、非 trial/invite-trial、权威整数微单位价格、合法原币种、正 Credit 与合法期限/reset。随后从该行冻结 `price_amount_micros`、`currency`、`monthly_token_limit`、`duration_unit/duration_value/custom_seconds`、`quota_reset_period/quota_reset_custom_seconds`、规则版本和完整 source snapshot；窗口只使用 `CreateUserSubscriptionFromPlanWithResultTx` 返回的实际 `EventStartTime/EventEndTime`。

不得从 `PriceAmount float64` 反推 micros，不得读取当前全局设置替代 Plan 快照，不新增进程内 mutex、retry 或通用框架。

## 幂等与 disabled 边界

- 同一 source/idempotency identity 成功重放返回同一 entitlement/window/grant，不新增行、不二次续期、不改写快照。
- 为保持 disabled 后的成功重放，guard 后先用请求 identity 定位既有 grant；匹配目标用户/plan/source/reason 与既有不可变 snapshot，返回既有结果，不以当前 Plan enabled 状态否决。
- 新 identity 必须使用 guard 内重读 Plan；disabled、缺失、类型不符、trial/invite-trial、权威 micros 缺失/非正、非法币种、Credit/期限/reset 不合法均稳定拒绝并整体回滚。
- 相同 identity 但目标或事实冲突继续返回 `ErrTimedSubscriptionGrantIdempotencyMismatch`；不泄漏数据库方言错误文本。

## 原子性与窗口语义

权益创建/续期与 grant 写入同一事务；任一步失败均不留下 entitlement/window/grant/相关状态部分写入。沿用既有 `[start,end)` 半开窗口与 reset 规则；不发明新时间语义。零 Credit 的新有价 timed Plan 依据现有领域资格为非法，稳定拒绝且零写入。

## #22 与范围边界

保留 #22 的 CreditValuation、current_only、权威 micros sorter、BigInt DTO/UI，不修改 Credit 结算。仅收敛管理员 timed grant 调用接口、Plan 权威读取与对应 model/controller 测试；若不修改前端请求构造则不触碰 UI/i18n。明确不实现 Issues #23–#28，不合并父分支、不关闭 Issue、不部署、不回收工作树。
