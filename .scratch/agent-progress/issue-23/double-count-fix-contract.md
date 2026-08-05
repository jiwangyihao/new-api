# Issue #23 兼容字段双计数修复合同

- 冻结基线：`9b496ca0d46bad84b4977d63496a668388e99080`
- 唯一 RED：`go test ./service -run '^TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields$' -count=1`
- 稳定症状：数据库订阅使用量为 `100`；兼容字段期望 `100`，实际 `199`。
- 单一所有者不变量：`SubscriptionFunding.Settle` 独占资金来源持久化与 funding 内部兼容快照（`AmountUsedAfter`、`TokenUsedAfter`、`TokenRemaining`、Credit target）的更新；`BillingSession.Reserve` 在 `reserveFunding(delta)` 成功后不得再次累加同一批字段。
- session 责任：`BillingSession.Reserve` 仅更新必要的 session 账本（`sub.preConsumed`、`s.preConsumedSubscription`、`s.preConsumedQuota`、`s.extraReserved`），每项恰好一次，并同步一次 `RelayInfo`。
- 范围：仅修改 `service/billing_session.go`；必要时补强 `service/subscription_billing_test.go`。不修改 schema、清理策略、前端、i18n 或 Issue #24–#28。
- 失败原子性：`reserveFunding(delta)` 失败时，数据库、funding/session 兼容字段与 `RelayInfo` 均保持调用前状态。
