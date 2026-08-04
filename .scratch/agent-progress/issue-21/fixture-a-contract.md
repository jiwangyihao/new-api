# Issue #21 Fixture A 迁移合同

状态：ACTIVE

## 冻结边界

- 仅迁移 `model` 包旧 paid-value analytics 测试夹具及必要的同目录 `_test.go` helper。
- 不修改 `service/**`、`controller/**`、前端、locale、生产 analytics/timed 代码或 schema。
- 生产 fail-closed 保持不变：有价 timed 服务窗口缺少 immutable grant 时必须 missing/unknown，不回退查询时当前 Plan 价格。
- 金额权威来源只能是显式整数 micros；禁止从 `SubscriptionPlan.PriceAmount float64` 反推。

## 测试夹具 seam

若需要共用 helper，只提供一个窄测试 seam：调用方必须显式给出服务窗口、`GrantCredit`、`SourcePriceMicros`、`SourceCurrency`、`ValuationAmountMicros`、`ValuationCurrency` 和稳定 source identity；helper 只负责填入固定的 exact confidence、rule/version、同币种 1/1 FX 与持久化错误断言。

helper 不读取 Plan 当前价格，不计算 float→micros，不调用生产 fallback，也不掩盖 missing/overlap warning。不同业务事实（例如有意缺口、重叠、跨币种）仍由测试直接构造，不经“自动补全”helper。

## 聚合夹具与主 tracer 的关系

本迁移目标中的旧测试主要验证 paid-value 聚合、筛选、排序、排除名单、never-reset、超用归零、缩短窗口和订单辅助金额，不重新证明领域授予入口。允许为这些聚合测试直接插入 grant；Issue #21 的真实订单、兑换、管理员授予与续期主 tracer 已由既有 `timed_subscription_valuation_*` 及五接口测试覆盖。

## 必须冻结的 grant 字段

- `UserSubscriptionId`、`UserId`、`PlanId`
- `IdempotencyKey`
- `SourceType`、`SourceKey`、可选稳定 `SourceId`
- `EventStartTime`、`EventEndTime`
- `GrantCredit`
- `SourcePriceMicros`、`SourceCurrency`
- `ValuationAmountMicros`、`ValuationCurrency`
- `Confidence=exact`
- `RuleVersion=CreditValuationRuleVersion`
- `FxRateNumerator=1`、`FxRateDenominator=1`
- 可审计的非空 `SourceSnapshot`

## 断言迁移原则

- 保持原可观察业务断言，不用降低金额、数量、排序或筛选断言换 GREEN。
- “无 token 仅 time value”“超用归零”“never reset”“缩短窗口”“排序”等事实必须继续由测试防守。
- 若旧夹具在新时间线合同下确实应披露 missing/unknown，则断言稳定 warning/unknown；不得伪造 exact。
- 空指针测试先通过合法 grant 恢复应有 singular 金额，再保留原订单辅助金额断言；不得用 nil guard 把失败变成通过。

## 验证合同

1. 每组最小 RED→GREEN。
2. `go test ./model -run 'PaidSubscriptionValue|AdminPaidSubscription|TimedSubscription' -count=1`。
3. 相同关键集合 `-count=10`。
4. `go test ./model -count=1`。
5. `git diff --check`，最终工作树 clean。
