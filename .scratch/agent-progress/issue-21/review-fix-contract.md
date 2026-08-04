# Issue #21 Standards 修复合同

## 合并基线与所有权

- 冻结实现：`547512242578ec198034d322875c5485735b247a`。
- 父集成：`2260cd2f6369d9cd9e1bea2ac93349b45c7b0ccc`。
- 合并顺序：先把最新 `jiwangyihao/credit-operational-value-integration` 合入本分支，再修复四项 finding。
- #22 通用骨架拥有：Credit DTO/分流/current_only、整数 micros accumulator/sorter、精确 `amount_micros` 解析错误、升降序与业务主键 tie-breaker、前端 BigInt 通用展示。
- #21 只叠加：timed grant 领域入口、grant 时间线 calculator、`*_by_currency`、timed unknown/warning/source 与现有 timed UI 增量。

## 固定锁序与幂等合同

`GrantTimedSubscriptionTx` 必须先通过数据库内可跨进程成立的权威 Plan guard 建立线性化顺序，再在同一事务内读取来源身份；锁序固定为：

```text
SubscriptionPlan guard -> existing timed grant identity -> target UserSubscription -> new TimedSubscriptionValuationGrant
```

相同 identity/相同事实返回同一 entitlement/window/grant，且只续期一次；相同 identity/不同事实返回 `ErrTimedSubscriptionGrantIdempotencyMismatch` 并整体回滚。disabled Plan 的已提交来源仍可重放，新来源仍拒绝。唯一冲突兜底必须映射为稳定 sentinel 并重新读取已提交事实，不向调用者泄漏方言文本；进程内 mutex 不作为正确性来源。

## 整数金额合同

- 每个 timed currency/source value 以 `int64` micros 进入 #22 的唯一 `adminMoneyAccumulator`。
- 所有 summary/users/subscriptions/plans/sources 聚合与 recognized 排序只使用十进制 `amount_micros`。
- `Amount float64` 只在响应展示边缘派生，不参与精确聚合或排序。
- 单币种 singular 与多币种 nullable/`*_by_currency` 合同不变；不跨币种求和，不按当前 Plan 币种补猜。

## 溢出合同

所有 timed segment、currency、source 与五接口 totals 的 `int64` 加法必须通过 `checkedAddInt64` 或等价无分配窄 helper。任一溢出返回 `ErrCreditValuationOverflow`，不得返回负数、截断值、部分 totals 或降为 unknown。

## 不可变错误合同

`TimedSubscriptionValuationGrant.BeforeUpdate` 与 `BeforeDelete` 返回同一个包级稳定 sentinel（命名按仓库错误体系确定）。领域/API 通过 `errors.Is` 判断，不解析错误文本；失败后原 grant 不变。

## 明确非所有权

不修改 CreditValuation 深模块、通用 Credit request settlement、Credit 正向入账/退款恢复、转换 FX、历史迁移、marker/ready、发布；不实现 Issues #23–#28；不重做 UI/i18n/浏览器流程；MySQL/PostgreSQL 零 SKIP 验收归 #27。
