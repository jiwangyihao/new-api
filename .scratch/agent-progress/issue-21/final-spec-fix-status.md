# Issue #21 最终 Spec 修复状态

状态：IN_PROGRESS

## 冻结现场

- 分支：`jiwangyihao/issue-21-timed-grants`
- 冻结 HEAD：`af1f76f6ed006870aa20c4ef5f0b6467016fca6f`
- 起始工作树：clean
- 最近安全提交：`9235f6887`（Finding 1：严格校验计时权益周期）。
- 当前未提交文件：Redemption Findings 2–6 的 model/controller 实现、回归测试与本状态/证据更新。

## 七项 Finding

1. `COMPLETE`：权威 timed Plan 的 duration/reset 严格资格校验；模型矩阵 `-count=10` 与真实 controller API 已 GREEN。
2. `COMPLETE`：无 snapshot Redemption 在热路径稳定拒绝且事务零写入。
3. `COMPLETE`：Credit current Plan 仅判当前资格，持久化 snapshot 提供冻结事实；并发 claim 恢复一次 grant 与一次 replay。
4. `COMPLETE`：used Redemption 成功重放比较规范化 mode，双向冲突均稳定拒绝且零写入。
5. `COMPLETE`：disabled trial / invite-trial 在任何新兑换副作用前稳定拒绝。
6. `COMPLETE`：`Redemption.Update` 事务内锁定重读，统一 `Redemption → SubscriptionPlan` 锁序；status-only 不读取 Plan 或补 snapshot。
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

完成 Redemption Findings 2–6 的 `-count=10`、`git diff --check` 与 clean 安全提交后，开始 Finding 7 的最小订单 replay RED。

## 阻塞

无。
