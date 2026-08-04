# Issue #21 AC2 / Gate B Spec 修复合同

## 深模块接口

管理员 controller 只允许提交意图：目标用户、`plan_id`、`reason`、`idempotency_key`。领域入口不接受价币、Credit、期限、reset、billing/pricing rule、窗口或 confidence；管理员事实来自数据库当前 Plan，订单与兑换事实来自服务端此前持久化的授权 source snapshot。

兼容 JSON 即使包含旧 `source_price_micros/source_currency`，服务端也会因 DTO 不声明字段而忽略；内部 Go request 已删除 Plan 对象及价币参数。不得把客户端估值值传入领域层再比较或采用。

## 事务与权威来源读取

固定顺序：

```text
SubscriptionPlan guard
-> committed grant identity replay
-> authoritative source lock/read
-> target UserSubscription
-> new TimedSubscriptionValuationGrant
```

- 管理员新 allocation：在 guard 后重读当前数据库 Plan，要求 identity 匹配、`entitlement_type=timed`、`enabled=true`、非 trial/invite-trial、权威整数 micros、CNY/USD、正 Credit 与合法期限/reset。
- 已授权订单：同事务锁定成功 `SubscriptionOrder`，验证 user/plan/source identity 与持久化 `EntitlementSnapshot`，从购买时快照冻结价币、Credit、duration/reset；Plan 后来改价、改币或 disabled 不撤销已授权履约。
- 已授权兑换：创建/更新兑换码的事务锁定 Plan 并把 entitlement source snapshot 持久化到 `Redemption.FulfillmentSnapshot`；兑换事务锁定当前 Plan 进行资格检查，但 grant 事实来自创建快照，后来改价不得重写。

三条路径都只从权威记录冻结 `price_amount_micros`、`currency`、`monthly_token_limit`、`duration_unit/duration_value/custom_seconds`、`quota_reset_period/quota_reset_custom_seconds`、规则版本和结构化 source identity；窗口只使用 `CreateUserSubscriptionFromPlanWithResultTx` 返回的实际 `EventStartTime/EventEndTime`。不得从 `PriceAmount float64` 反推 micros，不得读取当前全局设置替代冻结事实，不新增进程内 mutex、retry 或通用框架。

## 幂等与 disabled 边界

- 同一 source/idempotency identity 成功重放返回同一 entitlement/window/grant，不新增行、不二次续期、不改写快照。
- guard 后先用请求 identity 定位既有 grant；匹配目标用户/plan/source/reason 与既有不可变 snapshot 后直接返回，因此 Plan 后续 disabled 不否决成功重放。
- 管理员新 identity 必须使用 guard 内当前 Plan；disabled、缺失、类型不符、trial/invite-trial、权威 micros 缺失/非正、非法币种、零 Credit 或期限/reset 不合法均稳定拒绝并整体回滚。订单/兑换新履约还必须通过各自 source 记录的身份与状态校验。
- 相同 identity 但目标或 source intent 冲突继续返回 `ErrTimedSubscriptionGrantIdempotencyMismatch`；不泄漏数据库方言错误文本。

## 原子性与窗口语义

权益创建/续期与 grant 写入同一事务；任一步失败均不留下 entitlement/window/grant/相关状态部分写入。沿用既有 `[start,end)` 半开窗口与 reset 规则；不发明新时间语义。零 Credit 的新有价 timed Plan 依据现有领域资格为非法，稳定拒绝且零写入。

## #22 与范围边界

保留 #22 的 CreditValuation、current_only、权威 micros sorter、BigInt DTO/UI，不修改 Credit 结算。范围仅为管理员 timed grant 输入与 Plan 权威读取、订单/兑换既有授权快照不回归，以及对应 model/controller 测试；未修改前端请求构造、UI 或 i18n。明确不实现 Issues #23–#28，不合并父分支、不关闭 Issue、不部署、不回收工作树。
