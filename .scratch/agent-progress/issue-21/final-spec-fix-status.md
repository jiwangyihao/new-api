# Issue #21 最终 Spec 修复状态

状态：IN_PROGRESS

## 冻结现场

- 分支：`jiwangyihao/issue-21-timed-grants`
- 冻结 HEAD：`af1f76f6ed006870aa20c4ef5f0b6467016fca6f`
- 起始工作树：clean
- 最近安全提交：`af1f76f6ed006870aa20c4ef5f0b6467016fca6f`
- 当前未提交文件：本检查点的 `final-spec-fix-{status,evidence,contract}.md`

## 七项 Finding

1. `IN_PROGRESS`：权威 timed Plan 的 duration/reset 严格资格校验。
2. `PENDING`：缺失 Redemption 授权快照时禁止热路径补造 exact。
3. `PENDING`：Credit redemption 当前资格与冻结事实分离。
4. `PENDING`：已使用 Redemption 重放比较规范化 mode。
5. `PENDING`：disabled trial / invite-trial 禁止新兑换。
6. `PENDING`：`Redemption.Update` 事务内锁定重读并统一 `Redemption → SubscriptionPlan` 锁序。
7. `PENDING`：普通 paid timed 订单成功回调重放恢复原结果。

## 当前事务与锁序合同

- timed grant：资格校验、guard、subscription 与 grant 必须处于同一事务；失败时 guard version、subscription、grant 一并回滚。
- Redemption 新建：在 `Insert` 事务中从当时权威 Plan 冻结 `FulfillmentSnapshot`。
- Redemption 兑换/更新：统一先锁 `Redemption`，需要 Plan 时再锁 `SubscriptionPlan`。
- 订单完成：通过订单 fulfillment identity 与 immutable grant 恢复重放结果，禁止再次续期或新增 grant。

## 稳定错误合同

- 非法权威 timed Plan：`errors.Is(err, ErrTimedSubscriptionGrantInvalid)`。
- 缺失/不完整授权快照、mode 冲突、disabled 当前资格、订单 identity/grant 不一致：使用既有最合适的稳定领域 sentinel/code；测试不得依赖错误文本。

## 下一步

仅读取 Finding 1 已提交失败测试及 `model/timed_subscription_valuation.go`、必要 controller 路径，复现 RED 后完成最小 GREEN。

## 阻塞

无。
