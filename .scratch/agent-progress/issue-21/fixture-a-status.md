# Issue #21 Fixture A 状态

状态：HANDOFF_READY_PRODUCTION_FIX_COMPLETE

## 冻结现场

- 工作树：`issue-21-fixture-a-model`
- 分支：`jiwangyihao/issue-21-fixture-a-model`
- 本次生产修复起始 HEAD：`8c428160d54a04921a566d3e0a6005f442c0fca4`
- 父工作树：`issue-21-timed-grants`
- 起始工作树：clean
- 本次所有权：仅修复 `model/admin_analytics_paid_subscription.go` 的 excluded 顶层 summary 聚合接缝，并更新 Fixture A status/evidence；未修改测试断言或其他生产范围。

## 当前阶段

六个旧 paid-value fixture 已迁移 immutable grant：五个原有测试继续 GREEN；此前 `TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers` 暴露的 expected=33 / actual=0 生产缺陷已通过复用 timed-aware recognized 聚合 helper 修复，断言未改。

## 失败迁移矩阵

| 测试 | 初始症状 | 迁移状态 |
|---|---|---|
| `TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue` | 期望 44 CNY，实际 0 | GREEN：合法 grant + 权威 micros/min 不变量 |
| `TestPaidSubscriptionValueIncludesPaidSourcesWithoutOrders` | 期望 99 CNY，实际 0 | GREEN：三种来源完整 grant 时间线 |
| `TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers` | 期望 33 CNY，实际 0 | GREEN：顶层 excluded summary 复用 timed-aware recognized 聚合，expected=33 断言不变 |
| `TestPaidSubscriptionValueEmptyExcludedListDoesNotFilterRows` | 期望 33 CNY，实际 0 | GREEN：完整 order grant 时间线 |
| `TestPaidSubscriptionValueSubscriptionsSortsMoneyBySelectedCurrencyOnly/recognized_remaining_value` | 期望 subscription 1，实际 2 | GREEN：CNY/USD 原币种 grant 排序 |
| `TestPaidSubscriptionValueSubscriptionsIncludesOrderAuxiliaryAmountWithPlanCurrency` | `RecognizedRemainingValue` 为 nil，测试第 989 行解引用 panic | GREEN：完整 grant + 权威 micros |

## 下一步

本路生产修复已完成。协调器可合入生产提交与证据提交；`invitation_commission_test.go` / `payment_method_guard_test.go` 的九个旧授权快照夹具仍由其他冻结分支负责。

## 最近安全提交

- 冻结基线：`774b35740c1879b285537031410731317d0142fc`
- Fixture A 最终迁移 HEAD：`8c428160d54a04921a566d3e0a6005f442c0fca4`
- 生产修复：`5b4eab4b7`

## 未提交文件

- 无；最终验收以 `git status --short` 无输出为准。

## 剩余风险

- 本次 excluded timed summary 缺陷无剩余 blocker；目标测试 `-count=10` 已通过。
- `go test ./model -count=1` 仍有九个 invitation/payment 旧授权快照夹具失败，属于其他冻结分支，不在本生产修复范围内。
