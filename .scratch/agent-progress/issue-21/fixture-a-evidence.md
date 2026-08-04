# Issue #21 Fixture A 证据

状态：FIRST_GROUP_GREEN

## 基线与必读材料

- `git rev-parse HEAD`：`774b35740c1879b285537031410731317d0142fc`
- `git merge-base --is-ancestor 774b35740c1879b285537031410731317d0142fc HEAD`：成功。
- 起始 `git status --short`：无输出。
- 已按要求读取父 PRD #19、Issue #21、已关闭 #22、共享夹具迁移合同、执行上下文、Issue #21 acceptance、ADR 0001/0002、2026-08-02 spec/plan、冻结 `status/evidence/contract` 与 `final-spec-fix-*`。
- 已读取 `skill://diagnosing-bugs`、`skill://tdd`、`skill://codebase-design`。

## 包级 RED

命令：

```text
go test ./model -count=1
```

结果：FAIL，退出码 1，`github.com/QuantumNous/new-api/model` 在约 6.6 秒测试时间失败。完整工具输出保存在本次会话 artifact `artifact://5`。

在包因 panic 终止前，观察到 6 个 paid-value fixture 相关失败：

1. `TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue`
   - `admin_analytics_paid_subscription_test.go:110`
   - `Max difference between 44 and 0 allowed is 0.0001, but difference was 44`
2. `TestPaidSubscriptionValueIncludesPaidSourcesWithoutOrders`
   - `admin_analytics_paid_subscription_test.go:631`
   - `Max difference between 99 and 0 allowed is 0.0001, but difference was 99`
3. `TestPaidSubscriptionValueExcludedModeAuditsPaidExcludedUsers`
   - `admin_analytics_paid_subscription_test.go:647`
   - `Max difference between 33 and 0 allowed is 0.0001, but difference was 33`
4. `TestPaidSubscriptionValueEmptyExcludedListDoesNotFilterRows`
   - `admin_analytics_paid_subscription_test.go:677`
   - `Max difference between 33 and 0 allowed is 0.0001, but difference was 33`
5. `TestPaidSubscriptionValueSubscriptionsSortsMoneyBySelectedCurrencyOnly/recognized_remaining_value`
   - `admin_analytics_paid_subscription_test.go:762`
   - expected subscription ID `1`, actual `2`
6. `TestPaidSubscriptionValueSubscriptionsIncludesOrderAuxiliaryAmountWithPlanCurrency`
   - panic at `admin_analytics_paid_subscription_test.go:989`
   - `runtime error: invalid memory address or nil pointer dereference`
   - stack reaches `testing.tRunner`; package aborts before later tests can report.

这些失败均发生在 timed analytics 已改为只读 `TimedSubscriptionValuationGrant` 后：旧夹具仍只插入 `SubscriptionPlan + UserSubscription`，因此 recognized 金额为 0、排序事实改变，或 nullable singular 被测试直接解引用。修复信号必须是合法 immutable grant fixture，不是生产 current Plan fallback。

## 非主失败信号

同一包级运行较早出现后台 gopool panic 日志：`common.RedisHSetObj` 对 Redis client 调用 `TxPipeline` 时 nil dereference，并有 `redis: client is closed` 日志。它没有终止包；本轮真正退出发生在上述 paid-value 测试 nil dereference。待 paid-value 夹具迁移后重新运行包级命令，才能判断该全局 Redis 夹具日志是否形成独立测试失败。

## GREEN 1：最小 min(time, token) 夹具

- 新增窄测试 helper `adminPaidCreateTimedGrant`，调用方显式提供 subscription、稳定 source identity、服务窗口、`GrantCredit`、`SourcePriceMicros`、独立 `ValuationAmountMicros` 与 currency；helper 只补 exact confidence、rule version、1/1 FX 和可审计 snapshot，不读取 `PriceAmount float64`。
- 首测冻结两条首尾相接的 30 天、`40,000,000` micros CNY grant，完整覆盖原 subscription 服务窗口。
- RED：合法 grant 下端到端 summary 实际为 time `44,000,000`、token `43,466,665`、recognized `43,466,665` micros。Issue #21 的 grant 时间线合同逐段按当前周期剩余 Credit 折减 token，因此端到端 `token <= time`；旧 summary 的 token=76/recognized=44 属于被替代的 current Plan 算法，不能由合法 grant 表达。
- 已经 Orca question 获协调器批准：保留测试前半段 `adminRecognizedRemainingValue` 的旧 44/76 单元断言，summary 改断言权威 `amount_micros` 且明确 `recognized=min(token,time)`，不硬编码兼容 float。
- GREEN 命令：`go test ./model -run '^TestPaidSubscriptionValueCalculatesMinTokenAndTimeValue$' -count=1`。
- GREEN 结果：PASS，`go test: 1 packages ok`，约 5.7 秒测试时间。
