# Issue #23 兼容字段双计数修复状态

- 状态：RED 已复现，尚未修改生产代码。
- 冻结 HEAD：`9b496ca0d46bad84b4977d63496a668388e99080`
- 当前安全提交：`9b496ca0d46bad84b4977d63496a668388e99080`
- RED 命令：`go test ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=1`
- RED 结果：失败；`expected: 100`，`actual: 199`。
- 下一动作：审阅 `BillingSession.Reserve`、`reserveFunding` 与 `SubscriptionFunding.Settle`，删除成功结算后的重复兼容字段累加，保持 session 账本一次更新。
