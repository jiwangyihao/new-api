# Issue #21 最终 Spec 修复证据

状态：IN_PROGRESS

冻结基线：`af1f76f6ed006870aa20c4ef5f0b6467016fca6f`（分支 `jiwangyihao/issue-21-timed-grants`，起始工作树 clean）。

## Finding 1：权威 Plan duration/reset

- 状态：COMPLETE
- RED（模型）：`go test ./model -run TestTimedSubscriptionValuationGrantRejectsInvalidAuthoritativePlanAtomically -count=1`；unknown duration 与 invalid custom duration 返回非 sentinel，non-positive duration、unknown reset、non-positive custom reset 均旧成功写入。
- RED（controller）：`go test ./controller -run TestAdminCreateTimedSubscriptionRejectsInvalidResetPlanAtomically -count=1`；HTTP 200 且响应 `success:true`，证明 unknown reset 被静默归一并写入。
- 最小 GREEN：`freezeAuthoritativeTimedSubscriptionGrant` 在创建/续期前校验 duration 支持枚举、非 custom `DurationValue > 0`、custom `CustomSeconds > 0`，以及 reset 支持枚举、custom `QuotaResetCustomSeconds > 0`；source snapshot 保留已校验原值，不再调用 `NormalizeResetPeriod`。
- 稳定错误：所有非法资格均为 `errors.Is(err, ErrTimedSubscriptionGrantInvalid)`；外层事务回滚 subscription、grant 与 guard version。
- GREEN（模型）：`go test ./model -run TestTimedSubscriptionValuationGrantRejectsInvalidAuthoritativePlanAtomically -count=10` → PASS。
- GREEN（controller）：`go test ./controller -run TestAdminCreateTimedSubscriptionRejectsInvalidResetPlanAtomically -count=1` → PASS。
- 提交：待本次 Finding 1 安全提交补记 SHA。

## Finding 2：缺失 Redemption 快照

- 状态：IN_PROGRESS
- RED：待运行真实 SQLite 无 snapshot、Plan 后续改价/改币后的兑换测试。
- 最小 GREEN：兑换热路径不再从 current Plan 补造 exact；缺失或 identity 不完整时稳定拒绝并零写入。

## Finding 3：Credit 当前资格与冻结事实

- 状态：PENDING
- RED：并发 Credit claim 当前会稳定失败 `redemption.plan_ineligible`。
- 最小 GREEN：current Plan 仅判当前资格；持久化 `FulfillmentSnapshot` 提供冻结授权事实。

## Finding 4：成功重放 mode 冲突

- 状态：PENDING
- RED：待运行 timed→credit_balance 与 credit_balance→timed 双向冲突测试。
- 最小 GREEN：相同 mode 才恢复原 fulfillment；冲突返回既有稳定错误且零写入。

## Finding 5：disabled trial / invite-trial

- 状态：PENDING
- RED：待运行真实 SQLite disabled trial 与 invite-trial 新兑换测试。
- 最小 GREEN：所有新兑换分支前统一检查 `currentPlan.Enabled`；既有相同 mode 成功重放不受影响。

## Finding 6：Redemption.Update 锁序

- 状态：PENDING
- RED：待运行真实文件 SQLite 多连接的 status-only 与 Update/Redeem 并发测试。
- 最小 GREEN：事务内锁定重读 Redemption，仅应用允许变更意图；按需依 `Redemption → SubscriptionPlan` 锁序读取 Plan。

## Finding 7：paid timed 订单重放

- 状态：PENDING
- RED：待运行无 invitation event 的首次完成/成功 replay 测试。
- 最小 GREEN：用持久化 subscription identity 与 immutable grant/source 恢复同一窗口，计数不变。

## 验证台账

Finding 1 的精确 RED/GREEN 已记录；下一步进入 Finding 2 的独立 RED→GREEN。

## 非目标

不新增 schema，不修改前端/i18n，不实现 #23–#28，不改变 Issue #22 合同，不运行项目全量测试或部署。
