# Issue #23 兼容字段双计数修复状态

- 状态：最小生产修复已完成；唯一回归单次与 `count=10` 均为 GREEN。
- 冻结 HEAD：`9b496ca0d46bad84b4977d63496a668388e99080`
- 当前安全提交：`8e79c4273`（RED 合同与证据）；生产修复待提交为 GREEN 安全点。
- RED：`go test ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=1` 失败，`expected: 100`、`actual: 199`。
- 修复：删除 `BillingSession.Reserve` 对 `SubscriptionFunding.Settle` 已更新的 `TokenUsedAfter`、`TokenRemaining`、`AmountUsedAfter` 的重复累加；保留 session 账本更新与一次 RelayInfo 同步。
- GREEN：同一命令单次通过；`-count=10` 通过。
- 下一动作：提交 GREEN 安全点，执行三包宽回归、窄 race 与 `git diff --check`。
