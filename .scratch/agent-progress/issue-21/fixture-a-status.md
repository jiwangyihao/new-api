# Issue #21 Fixture A 状态

状态：FIRST_GROUP_GREEN

## 冻结现场

- 工作树：`issue-21-fixture-a-model`
- 分支：`jiwangyihao/issue-21-fixture-a-model`
- 冻结 HEAD：`774b35740c1879b285537031410731317d0142fc`
- 父工作树：`issue-21-timed-grants`
- 起始工作树：clean
- 所有权：仅 `model` paid-value analytics 测试夹具与必要的同目录 `_test.go` helper；不修改生产代码。

## 当前阶段

第一组已 GREEN 并准备提交：`TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue` 使用两条首尾相接 immutable grant，summary 断言权威 micros 与 `recognized=min(token,time)`；其余五个已知失败留给第二组。

## 失败迁移矩阵

| 测试 | 初始症状 | 迁移状态 |
|---|---|---|
| `TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue` | 期望 44 CNY，实际 0 | GREEN：合法 grant + 权威 micros/min 不变量 |
| `TestPaidSubscriptionValueIncludesPaidSourcesWithoutOrders` | 期望 99 CNY，实际 0 | 待迁移 |
| `TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers` | 期望 33 CNY，实际 0 | 待迁移 |
| `TestPaidSubscriptionValueEmptyExcludedListDoesNotFilterRows` | 期望 33 CNY，实际 0 | 待迁移 |
| `TestPaidSubscriptionValueSubscriptionsSortsMoneyBySelectedCurrencyOnly/recognized_remaining_value` | 期望 subscription 1，实际 2 | 待迁移 |
| `TestPaidSubscriptionValueSubscriptionsIncludesOrderAuxiliaryAmountWithPlanCurrency` | `RecognizedRemainingValue` 为 nil，测试第 989 行解引用 panic | 待迁移 |

## 下一步

1. 提交第一组安全点。
2. 迁移其余五个已知失败，不再枚举其他测试。
3. 运行六测试组合与 `go test ./model -count=1`。

## 最近安全提交

- 冻结基线：`774b35740c1879b285537031410731317d0142fc`
- RED 证据提交：`c9225c603`

## 未提交文件

- 第一组提交前：`model/admin_analytics_paid_subscription_test.go` 与更新后的 fixture-a evidence/status。

## 阻塞

无。包级输出另有 Redis 测试全局状态产生的后台 gopool panic 日志；当前 paid-value 断言失败和 nil 解引用均有独立、直接的旧 timed grant 夹具根因信号。
