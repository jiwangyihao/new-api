# Issue #21 最终 Spec 修复证据

状态：COMPLETE

冻结基线：`af1f76f6ed006870aa20c4ef5f0b6467016fca6f`（分支 `jiwangyihao/issue-21-timed-grants`，起始工作树 clean）。

## Finding 1：权威 Plan duration/reset

- 状态：COMPLETE
- RED（模型）：`go test ./model -run TestTimedSubscriptionValuationGrantRejectsInvalidAuthoritativePlanAtomically -count=1`；unknown duration 与 invalid custom duration 返回非 sentinel，non-positive duration、unknown reset、non-positive custom reset 均旧成功写入。
- RED（controller）：`go test ./controller -run TestAdminCreateTimedSubscriptionRejectsInvalidResetPlanAtomically -count=1`；HTTP 200 且响应 `success:true`，证明 unknown reset 被静默归一并写入。
- 最小 GREEN：`freezeAuthoritativeTimedSubscriptionGrant` 在创建/续期前校验 duration 支持枚举、非 custom `DurationValue > 0`、custom `CustomSeconds > 0`，以及 reset 支持枚举、custom `QuotaResetCustomSeconds > 0`；source snapshot 保留已校验原值，不再调用 `NormalizeResetPeriod`。
- 稳定错误：所有非法资格均为 `errors.Is(err, ErrTimedSubscriptionGrantInvalid)`；外层事务回滚 subscription、grant 与 guard version。
- GREEN（模型）：`go test ./model -run TestTimedSubscriptionValuationGrantRejectsInvalidAuthoritativePlanAtomically -count=10` → PASS。
- GREEN（controller）：`go test ./controller -run TestAdminCreateTimedSubscriptionRejectsInvalidResetPlanAtomically -count=1` → PASS。
- 提交：`9235f6887`。

## Finding 2：缺失 Redemption 快照

- 状态：COMPLETE
- RED：`go test ./model -run 'TestRedeemLegacySubscriptionWithoutSnapshotRejectsWithoutWrites|TestRedeemUsedSubscriptionRejectsConflictingModeWithoutWrites|TestRedeemDisabledTrialPlansRejectWithoutWrites|TestRedemptionUpdatePreservesCommittedFulfillmentAfterStaleRead|TestRedeemCreditBalanceConcurrentClaimPersistsOneGrantAndOneReplay' -count=1`；无 snapshot 的旁路历史 Redemption 在 current Plan 改价/改币后仍成功兑换、生成 subscription/grant 并写回 current exact snapshot。
- 最小 GREEN：`redemptionFulfillmentFromSourceSnapshot` 要求已有可解析且 entitlement identity 完整的持久化 snapshot；缺失/不完整时返回 `ErrRedemptionPlanIneligible`，claim 与全部 fulfillment 写入随事务回滚。
- GREEN：上述组合命令 `-count=1` 与 `-count=10` 均 PASS；十次重复覆盖无 snapshot 零写入、双向 mode 冲突、disabled trial/invite-trial、stale Update 串行化及 Credit 并发一次 grant/一次 replay。

## Finding 3：Credit 当前资格与冻结事实

- 状态：COMPLETE
- RED：`go test ./model -run TestRedeemCreditBalanceConcurrentClaimPersistsOneGrantAndOneReplay -count=1` 稳定失败；snapshot 重建 plan 的 `Enabled=false` 被误用于当前资格，返回 `redemption.plan_ineligible`。
- 最小 GREEN：新 Redemption 通过 `Insert` 冻结授权 snapshot；`currentPlan` 仅用于 enabled/identity/type 与 Credit option 当前资格，snapshot 重建的 plan 仅提供冻结 Credit、价格、币种、duration/reset 与规则事实。
- GREEN：组合定向命令 `-count=1` → PASS；并发结果为一次 grant、一次相同 fulfillment replay、单一 ledger。

## Finding 4：成功重放 mode 冲突

- 状态：COMPLETE
- RED：组合定向命令 `-count=1` 中 timed→credit_balance 旧逻辑无错误返回 timed fulfillment；反向冲突亦未按请求 mode 建立稳定冲突合同。
- 最小 GREEN：`redemptionResultFromFulfillment` 接受规范化 requested mode，仅当其等于持久化 `FulfillmentMode` 时恢复原结果；双向冲突返回 `ErrRedemptionAlreadyUsed`，不新增 subscription、timed grant 或 Credit ledger。
- GREEN：`TestRedeemUsedSubscriptionRejectsConflictingModeWithoutWrites` → PASS。

## Finding 5：disabled trial / invite-trial

- 状态：COMPLETE
- RED：组合定向命令 `-count=1` 中 disabled trial 与 invite-trial 均成功创建 subscription；invite 路径具备产生 invitation side effect 的风险。
- 最小 GREEN：锁定 current Plan 后，在 claim 与 trial/invite/paid/Credit 分支之前统一校验 enabled 与 timed entitlement identity；失败返回 `ErrRedemptionPlanIneligible` 并零写入。
- GREEN：`TestRedeemDisabledTrialPlansRejectWithoutWrites` → PASS，subscription/grant/invitation event 均为 0。

## Finding 6：Redemption.Update 锁序

- 状态：COMPLETE
- RED：组合定向命令 `-count=1` 的确定性 stale-read 交错中，事务外 DTO 的 `Update` 将已兑换状态从 used 覆盖回 enabled，并可丢失 fulfillment。
- 最小 GREEN：model `Update` 在事务内先 `FOR UPDATE` 重读 Redemption，只应用允许的 name/expired/plan-mode 意图；真实 Plan 变化时才按 `Redemption → SubscriptionPlan` 锁序冻结新 snapshot。`UpdateStatus` 只锁 Redemption，不读取 Plan、不补 snapshot，且不得恢复 used code。
- GREEN（model）：`TestRedemptionUpdatePreservesCommittedFulfillmentAfterStaleRead` → PASS，既有 fulfillment 与单一 grant 保持不变。
- GREEN（controller）：`go test ./controller -run 'TestUpdateRedemptionStatusOnlyDoesNotBackfillMissingSnapshot|TestUpdateSubscriptionRedemptionPreservesSnapshotWhenPlanUnchanged|TestUpdateUsedSubscriptionRedemptionRejectsSnapshotMutation' -count=10` → PASS。

## Finding 7：paid timed 订单重放

- 状态：COMPLETE
- RED：`go test ./model -run TestTimedSubscriptionValuationGrantPaidOrderReplayRestoresImmutableResult -count=1`；首次完成成功，但无 invitation event 的普通 paid timed 成功重放只返回空 `Transitioned=false`，`Subscription=nil` 且窗口/identity 丢失。
- 最小 GREEN：成功 replay 在 immutable entitlement 判定为普通 paid timed 后，按订单 source identity 查询唯一 timed grant，校验 `FulfilledSubscriptionID`、user、plan、source、半开窗口，并读取同一 subscription；任一 identity 不一致返回 `ErrTimedSubscriptionGrantInvalid`，不读取 current Plan 推断历史。
- GREEN：`go test ./model -run TestTimedSubscriptionValuationGrantPaidOrderReplayRestoresImmutableResult -count=10` → PASS；首次与 replay 返回相同 subscription/window，`Transitioned=false`，subscription/grant 计数保持 1。
- 提交：`4564ca949`。

## 验证台账

- Finding 1 model 资格矩阵：`go test ./model -run TestTimedSubscriptionValuationGrantRejectsInvalidAuthoritativePlanAtomically -count=10` → PASS。
- Finding 1 controller 非法 reset：`go test ./controller -run TestAdminCreateTimedSubscriptionRejectsInvalidResetPlanAtomically -count=1` → PASS。
- Findings 2–6 model 组合：`go test ./model -run 'TestRedeemCreditBalanceConcurrentClaimPersistsOneGrantAndOneReplay|TestRedeemLegacySubscriptionWithoutSnapshotRejectsWithoutWrites|TestRedeemUsedSubscriptionRejectsConflictingModeWithoutWrites|TestRedeemDisabledTrialPlansRejectWithoutWrites|TestRedemptionUpdatePreservesCommittedFulfillmentAfterStaleRead' -count=10` → PASS（141.31s）。
- Finding 6 controller 更新组合：`go test ./controller -run 'TestUpdateRedemptionStatusOnlyDoesNotBackfillMissingSnapshot|TestUpdateSubscriptionRedemptionPreservesSnapshotWhenPlanUnchanged|TestUpdateUsedSubscriptionRedemptionRejectsSnapshotMutation' -count=10` → PASS。
- Finding 7 普通 paid timed replay：`go test ./model -run TestTimedSubscriptionValuationGrantPaidOrderReplayRestoresImmutableResult -count=10` → PASS。
- 并发窄 race：`go test -race ./model -run 'TestRedeemCreditBalanceConcurrentClaimPersistsOneGrantAndOneReplay|TestRedemptionUpdatePreservesCommittedFulfillmentAfterStaleRead' -count=1` → PASS。
- controller 最终窄集合：`go test ./controller -run 'TimedSubscription|Redeem|Redemption' -count=1` → PASS；合法历史测试夹具已改为通过 `Redemption.Insert` 冻结 snapshot，提交 `b87474bb3`。
- `go test ./model -run 'TimedSubscription|Redeem|Redemption|SubscriptionOrder' -count=1` 未通过：命中 `invitation_commission_test.go`、`payment_method_guard_test.go` 中未持久化 `EntitlementSnapshot` 的旧订单/兑换夹具，以及测试清理阶段缺表日志；这些旧夹具与本任务“paid timed 无 snapshot 稳定拒绝”合同冲突，按最终收敛指令未扩展邀请/订单测试范围。
- 完整项目测试、前端套件、MySQL/PostgreSQL 实机、#22 全组合门禁未运行；由协调器按冻结分工执行。

## 提交

- `2d9f200e2`：建立最终 Spec 修复检查点。
- `9235f6887`：严格校验计时权益周期。
- `ffdfd46ba`：固化兑换授权与并发更新。
- `4564ca949`：恢复计时订单重放结果。
- `b87474bb3`：通过冻结入口构造合法 controller 兑换测试夹具。

## 非目标

不新增 schema，不修改前端/i18n，不实现 #23–#28，不改变 Issue #22 合同，不关闭 Issue、不部署、不回收工作树。
