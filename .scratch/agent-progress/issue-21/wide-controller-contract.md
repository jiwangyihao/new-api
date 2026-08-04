# Issue #21 宽回归修复 E 合同

## 所有权

只修改以下范围：

- `controller/TestSubscriptionKyrenCreditWebhookCompletesFromSnapshotWithoutInvitation` 所在测试文件及其测试 setup/cleanup；
- 为隔离全局测试状态所必需、语义明确且可复用的 `...ForTest` helper（仅当现有接缝不足，并先证明无生产调用）；
- `.scratch/agent-progress/issue-21/wide-controller-{status,evidence,contract}.md`。

不得修改前端/i18n、schema、生产 CreditValuation、BillingSession、购买语义、#23–#28、FX、migration marker、ready 状态机或 MySQL/PostgreSQL 门禁。

## 必须保持的可观察合同

- 普通 paid timed 订单缺少合法 `EntitlementSnapshot` 必须 fail closed。
- Redemption 只能由 `Redemption.Insert` 冻结 `FulfillmentSnapshot`；历史缺快照不得按当前 Plan 热补 exact。
- Kyren 回调按不可变 snapshot 履约；计划禁用后既有已授权履约仍完成，新购继续拒绝。
- Credit 数量、`CreditValuationState` 与 ledger 保持一份且一致。
- `request_id` 预扣、settlement 与 replay 幂等合同保持不变。
- summary、users、subscriptions、plans、sources 五接口口径一致。
- Credit 继续与邀请奖励、现金佣金和邀请付费口径隔离。
- 测试所有现有业务断言与 BillingSession 路径必须保留。

## 测试隔离合同

每次测试迭代必须：

1. 在切换新测试数据库之前清除该测试读取的进程级缓存；
2. 初始化数据库和 setting 后不读取上一轮缓存值；
3. cleanup 中关闭本轮数据库并清理本轮产生的进程级缓存；
4. 恢复测试修改前的全局值，不把测试状态泄漏给后续测试；
5. 不依赖执行顺序、sleep、随机身份或降低重复次数。

协调器已批准导出 `ClearDBTimestampCacheForTest`：它是只调用既有 `resetDBTimestampCacheForTest` 的薄包装，供隔离测试数据库复用；生产代码不得调用。目标 controller 测试在新 DB setup 后清理一次，并通过 cleanup 再清理一次，保留真实 `StartTime` 路径与全部业务断言。
