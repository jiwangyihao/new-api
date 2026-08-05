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

待执行。

## 宽回归与竞态

待执行。

## 范围边界

仅修复兼容字段单一所有者；不涉及 Issue #24–#28、前端、i18n、数据库 schema、清理策略、部署或其他工作树。
