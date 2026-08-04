# Issue #21 宽回归修复 E 证据

## 冻结 RED

协调器已在共同父现场执行：

```text
go test ./controller -run '^TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation$' -count=10
```

同一测试进程内部分迭代失败；精确症状发生在 `service.PreConsumeBilling`：

```text
active subscription is required: no active subscription
code=subscription_required
```

失败前真实 Kyren snapshot 履约、Credit ledger/state、disabled-plan 新购拒绝均已通过。该结果先作为外部冻结证据；本工作树仍须自行重现并保存命令输出。

## 已执行检查

```text
git branch --show-current
git rev-parse HEAD
git merge-base --is-ancestor 3e74a2928f7e4b7c3d5c6eae3fbc8362172a4c5d HEAD
git status --short
git log -3 --oneline
```

结果：分支 `jiwangyihao/issue-21-wide-controller-fix`；当前 HEAD `de6c6bbe912294e802b25a5e9bbcc37e8d9194d7`；共同父是祖先；工作树 clean。

## 候选全局状态与可证伪预测

1. **Subscription Plan cache**：若缓存跨迭代保留前一数据库的 plan，则每轮前后调用现有 `ClearSubscriptionPlanCacheForTest` 会消除 RED。
2. **primary billable subscription cache**：若缓存保留旧 subscription ID/空结果，则每轮前后调用现有 `ClearPrimaryBillableSubscriptionCacheForTest` 会消除 RED。
3. **DB timestamp cache**：若测试切换 SQLite DB 后仍复用上一轮数据库时间，则清除现有测试时间接缝会消除 RED，且失败轮的 DB 当前时间与缓存时间可观察到偏差。
4. **Redis/user cache**：若用户或活动权益缓存跨迭代保留空结果，则清理现有用户测试缓存会消除 RED。
5. **测试 DB DSN/连接生命周期**：若重复迭代复用命名 DSN、连接池或残留行，则每轮唯一 DB 生命周期与彻底关闭会消除 RED，且失败轮可见旧 entitlement/state/setting/request record。

验证时一次只改变一个变量；禁止 sleep、随机 user ID、降低 `-count`、skip 或删除断言。

## 恢复与验证命令

```text
go test ./controller -run '^TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation$' -count=10
go test ./controller -run '^TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation$' -count=25
go test ./controller -run 'TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation|TestKyrenWebhookCompletesSubscriptionOrder|TestSubscriptionBalancePayIdempotent' -count=10
go test ./controller -count=1
git diff --check
git status --short
```

## 本地 RED

```text
go test ./controller -run '^TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation$' -count=10 -v
```

结果：FAIL。第 1–3 轮 PASS；第 4–10 轮在 `subscription_payment_kyren_test.go:387` 失败，均为：

```text
active subscription is required: no active subscription
code=subscription_required
```

失败与测试进程从 `05:38:26` 跨到 `05:38:27` 同时发生。失败前 immutable Kyren snapshot 履约、disabled-plan 新购拒绝、单份 ledger/state 等断言均已通过。

## 根因与单变量验证

- `getOrCreateCreditBalanceSubscriptionTx` 通过新测试 DB 的 `getDBTimestampTx` 设置 Credit 权益 `StartTime`。
- 预扣选择通过进程级 `GetDBTimestamp` 读取最多保留 900 ms 的 DB timestamp cache。
- `go test -count` 每轮替换 `model.DB`，目标测试原 setup/cleanup 没有清理该缓存。秒边界后，旧缓存时间小于新权益 `StartTime`，活动权益查询的 `start_time <= now` 暂时不成立，于是返回 `subscription_required`。
- `model/credit_valuation_tracer_test.go` 已用同一个私有 reset 关闭同类跨 DB 时间污染，形成可复用先例。
- 单变量修复只新增 `ClearDBTimestampCacheForTest` 薄包装，并在目标测试 setup 后及 cleanup 中调用。未清理其他缓存，未改 DSN、user ID、真实 StartTime、并发、断言或生产行为，也未引入 sleep。

## GREEN

```text
go test ./controller -run '^TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation$' -count=25
```

结果：PASS。

```text
go test ./controller -run 'TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation|TestKyrenWebhookCompletesSubscriptionOrder|TestSubscriptionBalancePayIdempotent' -count=10
```

结果：PASS。

```text
go test ./controller -count=1
```

结果：PASS。

## 无生产调用证明

LSP references 与 `ClearDBTimestampCacheForTest\(` 仓库搜索结果一致：共 3 个引用位置——`model/db_time.go` 的定义，以及 `controller/subscription_payment_kyren_test.go` 的 setup 调用和 cleanup 注册；生产代码调用为 0。
