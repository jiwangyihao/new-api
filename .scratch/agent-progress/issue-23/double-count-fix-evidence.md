# Issue #23 兼容字段双计数修复证据

## RED

命令：

```text
go test ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=1
```

结果：`FAIL`。

```text
subscription_billing_test.go:438
Not equal:
expected: 100
actual  : 199
FAIL github.com/QuantumNous/new-api/service
```

该断言保持原样；它直接覆盖初始预扣后把同一 request target 扩至 100 时的兼容字段双计数。

## GREEN

修复：`SubscriptionFunding.Settle` 保持 funding 持久化与兼容快照的单一所有者；`BillingSession.Reserve` 不再重复累加 `TokenUsedAfter`、`TokenRemaining` 或 `AmountUsedAfter`，仅更新 session 账本并同步一次 `RelayInfo`。

```text
go test ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=1
PASS
go test ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=10
PASS
```

原有 `expected 100` 断言未修改。

## 宽回归与竞态

```text
go test -race ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=1
PASS

go test ./model ./service ./controller -count=1
首轮：service 与 controller 通过；model 的 TestRecordConsumeLogCoalescesConcurrentInserts 出现并发阈值波动（5 > 4）。

go test ./model -run '^TestRecordConsumeLogCoalescesConcurrentInserts$' -count=10
PASS

go test ./model ./service ./controller -count=1
PASS（3 packages）

git diff --check
PASS（无输出）
```

生产文件已执行 `gofmt -w service/billing_session.go`。首轮 model 波动不在本次修改路径，独立十次与完整复跑均通过。

未实测：MySQL/PostgreSQL 实机、全项目测试与部署；均明确不属于本修复范围。

## 范围边界

仅修复兼容字段单一所有者；不涉及 Issue #24–#28、前端、i18n、数据库 schema、清理策略、部署或其他工作树。
