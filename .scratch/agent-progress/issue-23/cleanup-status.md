# Issue #23 请求记录清理状态

## 当前状态
- 阶段：`cleanup RED_IN_PROGRESS`。
- legacy Task 生产兼容提交为 `78c487e96`，旧匿名夹具迁移与最终 evidence 提交为 `ea016089a`；后者是开始 cleanup 前的干净安全 HEAD。
- 当前仅处理 `SubscriptionPreConsumeRecord` 的安全清理；停止所有 Task 身份扩展、conversion、quota 重构及 #24–#28 探索。

## 已收敛遗留项
- 四项持久 Task identity 定向测试在 `-count=1`、`-count=10` 与窄 `-race` 下均通过。
- `SettleLegacyCreditTaskRequestTarget` 是持久 legacy Task final/refund/replay 的必要兼容：只建立 `legacy-task:<pk>` 请求快照，历史不可证明成本归类 unknown，不恢复 Credit 匿名 delta。
- `service/billing_session.go` 无差异；legacy 生产/测试/evidence 与 cleanup 严格分开提交。

## 下一步
1. 写入清理首个 RED：只有过期 `settled/refunded` 可删除，`consumed` 与未知状态必须保留，并固定 cutoff 边界。
2. 逐个推进持久引用保护、稳定 batch、幂等、失败原子性、诊断、并发与审计保留。
