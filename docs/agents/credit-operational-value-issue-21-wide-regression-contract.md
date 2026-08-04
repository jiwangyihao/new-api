# Issue #21 宽回归二次修复共享合同

## 冻结现场

- 父 PRD：GitHub #19。
- 当前切片：GitHub #21；#22 已集成，#21 尚未合并或关闭。
- 共同父工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-timed-grants`。
- 共同父 HEAD：`3e74a2928f7e4b7c3d5c6eae3fbc8362172a4c5d`。
- 已集成 A/B/C：model paid-value fixtures、service Credit redemption fixture、controller 支付订单 fixtures。
- `go test ./model ./service ./controller -count=1` 的剩余失败已收敛为两组：
  1. `model/invitation_commission_test.go` 与 `model/payment_method_guard_test.go` 中仍直接构造缺少不可变授权快照的订单/兑换码；
  2. `controller/TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation` 单独重复运行会间歇出现 `subscription_required`，属于跨迭代全局缓存/测试数据库隔离问题。

## 共享不变量

1. 生产 fail-closed 合同不可放宽：普通 paid timed 订单必须有合法 `EntitlementSnapshot`；兑换必须由 `Redemption.Insert` 冻结 `FulfillmentSnapshot`；历史缺快照不得用当前 Plan 热补 exact。
2. #22 的 CreditValuation、request_id、current_only、权威 micros、BigInt、BillingSession 行为不可修改。
3. 不新增 schema，不修改前端/i18n，不处理 #23–#28、FX、migration marker 或 ready 状态机。
4. 只能修改各自明确拥有的测试文件、测试帮助函数与 `.scratch/agent-progress`；若最小复现证明生产缺陷，先通过 Orca question/escalation 报告，不得自行扩大生产代码范围。
5. 先保存 RED，再做最小 GREEN。不得删断言、降低并发或把真实失败改成 skip。
6. 每路持续写 status/evidence/contract，小步提交；交付前运行本路定向测试、所属包 `-count=1`、`git diff --check`，保持 clean tree，并发送有效 `worker_done`。

## 合入顺序

二次修复完成后按 model 授权夹具 → controller 隔离修复合入 #21 父分支，再运行：

```text
go test ./model ./service ./controller -count=1
```

只有三包宽回归全绿，才允许继续 #21 最终集成。
