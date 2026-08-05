# Issue #23 请求记录清理状态

## 当前状态
- 阶段：`cleanup RED_IN_PROGRESS`。
- 冻结基线分支：`jiwangyihao/issue-23-request-settlement`。
- 冻结基线 HEAD：`d9e620191f8ca02c237859cc0250f98209749016`。
- 当前仅处理 `SubscriptionPreConsumeRecord` 的安全清理；停止所有 Task 身份扩展、conversion、quota 重构及 #24–#28 探索。

## 恢复现场
- 起始核验发现两处继承的未提交改动：`service/billing_session.go`、`service/task_billing_test.go`。
- 当前判断：这两处属于旧匿名 Credit Task 兼容缺口，不是清理主线。
- 唯一待裁决的生产差异是 `BillingSession.SettleWithInput` 对 Credit 请求是否必须无条件传入 `final=true`，以使新持久 `subscription_request_id` 的成功路径进入 `settled` 终态。
- 裁决方法：只运行现有新持久 identity 成功/重放定向测试。若该生产差异没有明确 RED 证据，则撤销该一行生产改动并只保留必要的旧夹具迁移；若有明确 RED，则记录证据并独立小步提交。

## 下一步
1. 用现有定向测试裁决 `final=true` 差异。
2. 停止 Task 探索。
3. 以 RED 测试直接推进终态资格、cutoff、持久引用保护、稳定 batch、幂等、并发和失败原子性。
