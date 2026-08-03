# Issue #21 消费合同

## 所有权与共享边界

Issue #21 主改：

- `TimedSubscriptionGrantRequest`、`GrantTimedSubscriptionTx` 与不可变 `TimedSubscriptionValuationGrant` 写入。
- timed 购买、兑换、管理员售后授予、续期以及邀请/试用排除调用点。
- timed grant 时间线投影、重叠窗口去重、逐币种 time/token/recognized 计算。
- timed 专用 `*_by_currency`、`mixed_grants`、overlap/unknown warning 与 source breakdown。
- 管理员计时授予 reason/idempotency UI、跨币种 timed 展示及六语言。

Issue #22 主改通用 analytics DTO、`CreditValuation`、Credit 分支、通用 micros/BigInt 前端组件。Issue #21 只对共享文件做 timed 最小增量，并在下方逐项登记。

## 持久化合同

`TimedSubscriptionValuationGrant` 一行表示一次计时权益获得，而不是整份合并权益。冻结：

- `idempotency_key` 唯一；`(source_type, source_key)` 组合唯一。
- `user_subscription_id`、`user_id`、`plan_id`。
- `source_type`、稳定 `source_key`、可选辅助 `source_id`。
- 实际 `event_start_time/event_end_time`。
- `grant_credit`。
- 权威 `source_price_micros`、原始 `source_currency`。
- `valuation_amount_micros`、`valuation_currency`；timed 保留原币种，同币种 FX 为 `1/1`。
- 期限、重置、档位与结构化来源 `source_snapshot`。
- 前向规则版本 `1`、由模块派生的 `exact` 置信度、数据库创建时间。

记录不可变：模型/领域边界拒绝 update/delete；不提供普通 HTTP 修改或删除接口。续期追加新 grant，不覆盖历史。

## 来源身份

- 订单：`source_type=subscription_order`，`source_key` 由稳定订单主键派生。
- 兑换：`source_type=redemption`，`source_key` 由稳定兑换履约主键派生。
- 管理员售后授予：`source_type=admin`，`source_key` 使用客户端可重试 `idempotency_key`。
- 邀请奖励、邀请试用与试用码：结构化来源显式判为不估值，继续创建权益但不创建伪零价 grant。

相同来源与相同参数返回既有结果，不续期；相同 idempotency/source identity 的参数指纹变化返回稳定冲突错误。

## 领域接口

```go
type TimedSubscriptionGrantRequest struct { /* 稳定来源、用户/plan、#20 权威 micros、原币种、期限/重置、结构化来源事实 */ }

func GrantTimedSubscriptionTx(tx *gorm.DB, request TimedSubscriptionGrantRequest) (*UserSubscriptionCreationResult, error)
```

模块在调用方事务内创建或续期权益，并使用低层创建结果的实际 `EventStartTime/EventEndTime` 写 grant；模块不提交事务。调用方不能声明 `exact`、伪造窗口或从 `float64 PriceAmount` 反推价格。

## 稳定错误合同

本切片至少稳定区分：

- timed 授予请求/来源无效；
- disabled、trial 或其他不合格套餐；
- `price_amount_micros` 缺失或非正的有价来源；
- reason 或 idempotency key 缺失；
- `idempotency_key` / `(source_type, source_key)` 参数冲突；
- grant 更新或删除被拒绝；
- 整数估值溢出。

错误使用 sentinel/code，不要求调用方解析文本。

## 分析 DTO 与算法合同

分析只读 grant 时间线，不读取查询时 `SubscriptionPlan` 价格补猜。每个原币种独立计算：

- `time_based_value_by_currency`
- `token_based_value_by_currency`
- `recognized_remaining_value_by_currency`

单币种保留兼容 singular；跨币种 singular 明确为 `null`。当前额度周期按实际 `max(token_limit-token_used,0)/token_limit` 折减，未来周期完整；失效/缩短按实际状态与 `end_time` 裁剪。重叠秒只由稳定排序最早 grant 计值，后续重叠披露 `overlapping_grants` unknown/warning。无法建立时间线时不回退当前价格。

summary/users/subscriptions/plans/sources 使用同一 timed 行投影。来源按 grant `source_type` 聚合；一条权益存在多个来源时 `source_attribution=mixed_grants`。

## UI payload 合同

管理员 timed 授予 payload 必须包含非空 `reason` 与客户端生成的 `idempotency_key`。失败重试复用原 key；成功或 plan/数量/期限/原因等业务参数变化后生成新 key。跨币种响应按 `*_by_currency` 展示，不依据当前套餐币种重建 singular。术语固定为“运营剩余价值”，不使用退款、负债或实收表述。

## 预计共享文件

- `model/subscription.go`：仅把 timed 低层调用收口到领域入口。
- `model/redemption.go`：仅提供兑换来源事实。
- `controller/subscription.go` 及现有订单完成调用点：仅提供管理员/订单来源事实和稳定错误响应。
- `model/admin_analytics_paid_subscription.go`、`dto/admin_analytics.go`：只加入 timed calculator/`*_by_currency` 最小接缝；保留 Issue #22 通用骨架优先权。
- `web/default/src/features/subscriptions/**`、`web/default/src/features/admin-analytics/**`：只加入 timed 授予与 timed 多币种增量；不复制 Credit 通用实现。
- `web/default/src/i18n/**`：仅新增本切片可见文字。

实际触碰的共享文件将在实现过程中逐项补充。

## 明确非所有权

不实现 Credit 请求通用结算、Credit 正向入账、破坏性恢复、转换 FX/在途结算、历史回填、migration ready 切换、三数据库发布门禁或生产部署；不改变计时→Credit 转换数量公式；不新增退款自动撤销或 grant reversal schema。
