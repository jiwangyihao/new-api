# Issue #21 最终 Spec 修复合同

状态：IN_PROGRESS

## 冻结输入

- 基线：`af1f76f6ed006870aa20c4ef5f0b6467016fca6f`
- 分支：`jiwangyihao/issue-21-timed-grants`
- 保留 Issue #22 通用 Credit/current_only/权威 micros/BigInt 合同及 Issue #21 已通过的 Standards 修复。

## 领域边界

- **Plan**：提供当前兑换资格与授权时冻结的权威事实；未知/非法 duration/reset 不得归一化为合法值。
- **Redemption**：持有不可变授权快照与 fulfillment；当前资格不能覆盖冻结事实，冻结事实也不能冒充当前资格。
- **Timed grant**：不可变、同源线性化；grant 与 subscription 变更同事务。
- **Order**：成功重放由持久化 fulfillment identity 与 immutable grant 恢复，不读 current Plan 推断历史。

## 七项固定合同

1. 权威 timed Plan 的 duration/reset 枚举与 custom seconds 严格校验；非法值统一满足 `errors.Is(err, ErrTimedSubscriptionGrantInvalid)`，subscription/grant/guard 零变化。
2. 新 Redemption 仅在 `Insert` 或合法 Plan/mode 前向变更事务中冻结快照；status-only 与兑换热路径不得补造或刷新历史 exact。
3. current Plan 只判 identity/enabled/type/trial 等当前资格；持久化快照只提供 Credit、价格、币种、duration/reset、规则与来源。
4. 已使用 code 的同 user 成功重放必须比较规范化 mode；不同 mode 返回既有稳定冲突错误并零写入。
5. `currentPlan.Enabled` 在所有新兑换分支前检查；已成功同 mode 重放保持可用。
6. `Redemption.Update` 在事务内先锁定重读 Redemption，再按需锁 Plan；仅应用允许字段，不能陈旧覆盖 fulfillment。
7. 普通 paid timed 订单重放恢复同一 subscription、`[start,end)` window 与 grant/source，且不再次迁移。

## 统一锁序

`Redemption → SubscriptionPlan → subscription/grant 写入`。status-only Update 止于 Redemption；任何失败由事务整体回滚。

## 验收边界

仅运行相关 model/controller 定向测试、必要的窄 race、Issue #22/#21 组合回归、`git diff --check` 和 clean-tree 检查。MySQL/PostgreSQL 实机与后续发布范围不在本任务内。

## 可恢复点

最近安全提交：`af1f76f6ed006870aa20c4ef5f0b6467016fca6f`。下一步：提交本检查点，然后 Finding 1 RED→GREEN。
