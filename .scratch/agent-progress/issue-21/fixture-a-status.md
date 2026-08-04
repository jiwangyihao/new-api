# Issue #21 Fixture A 状态

状态：HANDOFF_READY_WITH_PRODUCTION_BLOCKER

## 冻结现场

- 工作树：`issue-21-fixture-a-model`
- 分支：`jiwangyihao/issue-21-fixture-a-model`
- 冻结 HEAD：`774b35740c1879b285537031410731317d0142fc`
- 父工作树：`issue-21-timed-grants`
- 起始工作树：clean
- 所有权：仅 `model` paid-value analytics 测试夹具与必要的同目录 `_test.go` helper；不修改生产代码。

## 当前阶段

六个初始 paid-value 失败均已迁移 immutable grant 夹具：五个测试 GREEN；excluded timed summary 保留 expected=33 / actual=0 的真实生产 blocker。协调器已裁定不改生产代码、不降低断言，提交 clean handoff 后另派生产修复。

## 失败迁移矩阵

| 测试 | 初始症状 | 迁移状态 |
|---|---|---|
| `TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue` | 期望 44 CNY，实际 0 | GREEN：合法 grant + 权威 micros/min 不变量 |
| `TestPaidSubscriptionValueIncludesPaidSourcesWithoutOrders` | 期望 99 CNY，实际 0 | GREEN：三种来源完整 grant 时间线 |
| `TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers` | 期望 33 CNY，实际 0 | BLOCKED：合法 grant 后暴露 summary excluded 生产聚合缺口 |
| `TestPaidSubscriptionValueEmptyExcludedListDoesNotFilterRows` | 期望 33 CNY，实际 0 | GREEN：完整 order grant 时间线 |
| `TestPaidSubscriptionValueSubscriptionsSortsMoneyBySelectedCurrencyOnly/recognized_remaining_value` | 期望 subscription 1，实际 2 | GREEN：CNY/USD 原币种 grant 排序 |
| `TestPaidSubscriptionValueSubscriptionsIncludesOrderAuxiliaryAmountWithPlanCurrency` | `RecognizedRemainingValue` 为 nil，测试第 989 行解引用 panic | GREEN：完整 grant + 权威 micros |

## 下一步

无 Fixture A 可执行工作。协调器需另派生产修复处理 timed excluded summary，并由其他所有者迁移 `invitation_commission_test.go` / `payment_method_guard_test.go` 的冻结授权快照夹具。

## 最近安全提交

- 冻结基线：`774b35740c1879b285537031410731317d0142fc`
- RED 证据提交：`c9225c603`
- 第一组 GREEN 提交：`f44d52b5f`

## 未提交文件

- 最终 clean handoff 提交前：第二组测试迁移及本次 status/evidence 收口。

## 阻塞

- Fixture A 生产 blocker：summary 顶层 excluded 分支未使用 timed-aware 金额累加；expected 33 / actual 0。
- 包级其余九个失败属于 invitation/payment 旧授权快照夹具，超出本路所有权。
