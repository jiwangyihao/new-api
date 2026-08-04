# Issue #21 夹具迁移 B 合同

状态：VERIFYING

## 冻结输入

- 基线：`774b35740c1879b285537031410731317d0142fc`
- 所有权：`service/invitation_commission_test.go`；仅当相邻 invitation service 测试存在同一失败时才扩展到相邻 `_test.go`。
- 禁止修改 `model`、`controller` 或任何生产代码。

## 领域合同

- 新 Redemption 必须通过 `Redemption.Insert` 或代码库既有权威前向入口，在事务内从当时合法 Plan 冻结不可变 `FulfillmentSnapshot`。
- current Plan 只负责兑换时的当前资格：identity、enabled、entitlement type、trial 与 Credit option 资格。
- 持久化 snapshot 负责已授权事实：Credit、权威整数标价、币种、duration/reset、规则与来源；测试不得手写成功路径 snapshot JSON。
- 历史缺失或损坏 snapshot 必须保持 fail-closed，不能从 current Plan 补造 exact。
- Credit 兑换使用显式 `credit_balance` mode；充值档位必须 enabled、非 trial、具有正确 timed identity、正 Credit、合法权威 micros/币种及不限时购买资格。
- Credit 购买、兑换与转换继续与邀请奖励、现金佣金和邀请付费口径隔离。
- disabled Plan 仍拒绝新兑换；已有权益消费与已授权履约语义不在本测试迁移中改变。

## 验收合同

- 保留 invitation 测试原有幂等、奖励资格、金额、账户/记录数量和邀请隔离断言。
- 用真实 Redeem/commission 调用链观察行为，不调用生产私有捷径，不删除或放宽断言。
- 证明 Insert 后 `FulfillmentSnapshot` 实际非空且可供真实兑换使用；current Plan 后续仅参与当前资格。
- 运行最小 RED→GREEN、邀请/佣金/兑换定向集合、关键测试 `-count=10`、完整 `./service`、`git diff --check` 与 clean-tree 检查。

## 非所有权

不触碰 #22 `CreditValuation`、moving-weighted/current_only、ledger、request settlement、生产 Redemption 资格逻辑、前端/i18n、历史迁移或发布。
