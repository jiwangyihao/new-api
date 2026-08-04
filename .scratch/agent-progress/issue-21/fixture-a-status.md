# Issue #21 Fixture A 状态

状态：RED_CAPTURED

## 冻结现场

- 工作树：`issue-21-fixture-a-model`
- 分支：`jiwangyihao/issue-21-fixture-a-model`
- 冻结 HEAD：`774b35740c1879b285537031410731317d0142fc`
- 父工作树：`issue-21-timed-grants`
- 起始工作树：clean
- 所有权：仅 `model` paid-value analytics 测试夹具与必要的同目录 `_test.go` helper；不修改生产代码。

## 当前阶段

已在未修改任何夹具前运行 `go test ./model -count=1`。包级运行稳定进入 paid-value 旧夹具失败，并在第六个失败测试的空指针处终止。

## 失败迁移矩阵

| 测试 | 初始症状 | 迁移状态 |
|---|---|---|
| `TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue` | 期望 44 CNY，实际 0 | 待迁移 |
| `TestPaidSubscriptionValueIncludesPaidSourcesWithoutOrders` | 期望 99 CNY，实际 0 | 待迁移 |
| `TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers` | 期望 33 CNY，实际 0 | 待迁移 |
| `TestPaidSubscriptionValueEmptyExcludedListDoesNotFilterRows` | 期望 33 CNY，实际 0 | 待迁移 |
| `TestPaidSubscriptionValueSubscriptionsSortsMoneyBySelectedCurrencyOnly/recognized_remaining_value` | 期望 subscription 1，实际 2 | 待迁移 |
| `TestPaidSubscriptionValueSubscriptionsIncludesOrderAuxiliaryAmountWithPlanCurrency` | `RecognizedRemainingValue` 为 nil，测试第 989 行解引用 panic | 待迁移 |

## 下一步

1. 逐个运行尚未被 panic 执行到的 paid-value 测试，补全失败矩阵。
2. 复用或增加一个窄测试 helper，显式接收整数 micros、服务窗口和来源身份后插入 immutable timed grant。
3. 以最小失败集合逐组 RED→GREEN，保持原业务断言。
4. 运行定向单次、十次和 `go test ./model -count=1`。

## 最近安全提交

- 冻结基线：`774b35740c1879b285537031410731317d0142fc`

## 未提交文件

- 首个证据提交前：本次新增的 `fixture-a-{status,evidence,contract}.md`。

## 阻塞

无。包级输出另有 Redis 测试全局状态产生的后台 gopool panic 日志；当前 paid-value 断言失败和 nil 解引用均有独立、直接的旧 timed grant 夹具根因信号。
