# Issue #21 夹具迁移 B 证据

状态：INVESTIGATING

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
该输出同时记录了测试清理期间对缺失 `models`、`vendors` 表的 SQL 日志，但未形成额外测试失败或 panic；当前未观察到 Redis 全局夹具 panic。最小测试仍需单独运行，以捕获 `redemption.plan_ineligible` 的稳定错误路径并建立快速 RED→GREEN 回路。

## 待补证据

- 最小失败测试的稳定错误及旁路夹具位置。
- `Redemption.Insert` 冻结后的非空 `FulfillmentSnapshot`。
- invitation 原幂等、资格、金额、记录数量与隔离断言保持。
- 定向、`-count=10`、包级与 `git diff --check` 结果。
