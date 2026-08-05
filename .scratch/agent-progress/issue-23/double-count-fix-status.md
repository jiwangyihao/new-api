# Issue #23 兼容字段双计数修复状态

- 状态：兼容字段单一所有者修复及要求内验证已完成。
- 冻结 HEAD：`9b496ca0d46bad84b4977d63496a668388e99080`
- 当前安全提交：`a85dfabd5e2ecf86340a33afcbfdcd6c8d3df63f`（生产 GREEN 安全点）；本次文档提交后以仓库 HEAD 为最终安全提交。
- RED：`go test ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=1` 失败，`expected: 100`、`actual: 199`。
- 修复：删除 `BillingSession.Reserve` 对 `SubscriptionFunding.Settle` 已更新的 `TokenUsedAfter`、`TokenRemaining`、`AmountUsedAfter` 的重复累加；保留 session 账本更新与一次 RelayInfo 同步。
- GREEN：同一命令单次通过，`-count=10` 通过，窄 `-race` 通过；三包宽回归复跑通过；`gofmt` 与 `git diff --check` 通过。
- 宽回归说明：首轮仅 `model/TestRecordConsumeLogCoalescesConcurrentInserts` 出现并发阈值波动（`5` 大于 `4`）；该测试独立 `-count=10` 通过，随后原三包命令通过。
- 下一动作：提交最终证据，确认工作树干净并发送唯一 `worker_done`。
