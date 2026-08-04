# Issue #21 夹具迁移 B 证据

状态：COMPLETE

冻结 HEAD：`774b35740c1879b285537031410731317d0142fc`。

## 原始包级 RED

命令：

```text
go test ./service -count=1
```

结果：退出码 1，`github.com/QuantumNous/new-api/service` FAIL。

唯一测试断言失败：

```text
--- FAIL: TestCreditFulfillmentPathsDoNotCreateInvitationBenefits
    --- FAIL: TestCreditFulfillmentPathsDoNotCreateInvitationBenefits/Credit_redemption
        invitation_commission_test.go:375: Received unexpected error
```


## 最小 RED

命令：

```text
go test ./service -run 'TestCreditFulfillmentPathsDoNotCreateInvitationBenefits/Credit_redemption' -count=1 -v
```

结果：退出码 1，约 0.79 秒；唯一失败为 `service/invitation_commission_test.go:375` 的 `require.NoError(t, err)`。夹具在 `service/invitation_commission_test.go:368-373` 直接 `model.DB.Create(&redemption)`，因此没有经过 `Redemption.Insert` 的事务冻结入口；`Redeem` 在缺少 `FulfillmentSnapshot` 时沿 `ErrRedemptionPlanIneligible` 稳定拒绝。
包级原始输出同时记录了测试清理期间对缺失 `models`、`vendors` 表的 SQL 日志，但未形成额外测试失败或 panic；当前未观察到 Redis 全局夹具 panic。

## 最小 GREEN

夹具迁移：

- option Plan 保持 enabled、显式 timed entitlement、月度 duration/reset、正 Credit 与不限时购买资格，并补齐权威 `30_000_000` micros CNY 标价。
- Redemption 改由 `redemption.Insert()` 创建；紧接着断言 `strings.TrimSpace(redemption.FulfillmentSnapshot)` 非空，证明快照由权威前向入口冻结而非手写 JSON。
- 随后仍通过真实 `model.Redeem(..., model.RedemptionModeCreditBalance)` 与 `HandleInvitationRewardForSubscriptionRedemption` 调用链验证 Credit 邀请隔离。

命令：

```text
go test ./service -run 'TestCreditFulfillmentPathsDoNotCreateInvitationBenefits/Credit_redemption' -count=1 -v
```

结果：PASS，`github.com/QuantumNous/new-api/service` 通过。

## service 包级 GREEN

命令：

```text
go test ./service -count=1
```

结果：PASS，`github.com/QuantumNous/new-api/service` 通过，约 8.83 秒。未出现 Redis panic；原包级唯一失败已消失。

## 断言保留

测试继续断言：只产生一份 Credit 余额权益；邀请奖励事件、佣金记录和佣金账户均为零；两名直接邀请中仅 timed 对照用户符合活动资格；邀请人不获得月度权益。

## 最终实跑复核

命令：

```text
go test ./service -run 'TestCreditFulfillmentPathsDoNotCreateInvitationBenefits/Credit_redemption' -count=1 -v && go test ./service -count=1
```

实际结果：两条命令均输出 `go test: 1 packages ok`，组合命令退出码 0。

## 提交与范围

- `52cc9b193`：建立兑换夹具迁移检查点。
- `1866aa042`：持久化最小 RED。
- `df6531cf6`：通过 `Redemption.Insert` 构造合法 Credit 兑换夹具并记录 GREEN。
- 修改范围仅为 `service/invitation_commission_test.go` 与 `.scratch/agent-progress/issue-21/fixture-b-{status,evidence,contract}.md`；未修改生产代码、model/controller、CreditValuation、ledger 或 request settlement。
