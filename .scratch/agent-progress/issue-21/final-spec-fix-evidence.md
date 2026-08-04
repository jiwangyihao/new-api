# Issue #21 最终 Spec 修复证据

状态：IN_PROGRESS

冻结基线：`af1f76f6ed006870aa20c4ef5f0b6467016fca6f`（分支 `jiwangyihao/issue-21-timed-grants`，起始工作树 clean）。

## Finding 1：权威 Plan duration/reset

- 状态：IN_PROGRESS
- RED：待运行已提交的 duration/reset model 资格矩阵与 controller 非法 reset API 测试；将记录精确命令、失败断言和旧写入行为。
- 最小 GREEN：在唯一权威 timed grant 资格入口严格验证 duration/reset 枚举及 custom seconds，全部映射为 `ErrTimedSubscriptionGrantInvalid`，事务失败零写入且 guard version 不变。
- 稳定错误：`errors.Is(err, ErrTimedSubscriptionGrantInvalid)`。

## Finding 2：缺失 Redemption 快照

- 状态：PENDING
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

尚未运行修复测试。每个 finding 将追加精确 RED/GREEN 命令、旧行为、稳定错误、事务/锁序和对应提交 SHA。

## 非目标

不新增 schema，不修改前端/i18n，不实现 #23–#28，不改变 Issue #22 合同，不运行项目全量测试或部署。
