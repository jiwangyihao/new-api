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
