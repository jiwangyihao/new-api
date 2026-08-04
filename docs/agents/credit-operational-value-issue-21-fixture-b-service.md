# Issue #21 旧夹具迁移 B：service invitation Credit redemption

## 任务目标

你负责修复冻结 Issue #21 分支中 `service` 包的 invitation/Credit redemption 旧测试夹具。当前失败来自测试直接创建缺少 `Redemption.FulfillmentSnapshot` 的兑换码，然后调用新 fail-closed 兑换路径，得到 `redemption.plan_ineligible`。测试必须通过 `Redemption.Insert` 或等价权威前向入口冻结授权快照，而不是放宽生产拒绝行为。

工作树由协调器创建为冻结 `issue-21-timed-grants` 的 Orca 子工作树，基线必须包含 `774b35740c1879b285537031410731317d0142fc`。共享合同：

`C:/Users/34404/source/repos/new-api/.workspaces/new-api/credit-operational-value-integration/docs/agents/credit-operational-value-issue-21-fixture-migration-contract.md`

## 必读材料与 Skills

读取自动注入规则、父 PRD #19、Issue #21/#22、共享合同、执行协议、Issue #21 acceptance、ADR/spec 的 Redemption snapshot/disabled-plan/Credit grant 段，以及冻结树 `final-spec-fix-*`。使用 `skill://diagnosing-bugs` 和 `skill://tdd`；测试 helper 领域边界不清时读 `skill://codebase-design`。禁止子 Agent、项目全量 formatter/lint/前端套件。

## 精确所有权

主目标文件：

- `service/invitation_commission_test.go`
- 仅当同一失败实际位于相邻 invitation service `_test.go` 时，可修改对应测试文件
- 可在 `service` 测试中新增窄 helper，但不得修改 `model`/`controller` 文件或生产代码

不得触碰 #22 CreditValuation 深模块、moving-weighted/current_only、ledger、request settlement 或生产 Redemption 资格逻辑。

## 必须完成的行为

1. 先运行 `go test ./service -count=1`，记录具体失败测试、`redemption.plan_ineligible` 路径与任何其他 service 包失败。
2. 定位直接 `DB.Create(&model.Redemption{...})` 或等价旁路构造。将需要合法兑换的夹具改为：
   - 创建合法、enabled 且 entitlement identity 正确的当前 Plan；
   - 使用 `Redemption.Insert`（或代码库已确立的权威测试入口）在事务内冻结 `FulfillmentSnapshot`；
   - 如测试关注 Credit redemption，确保 Plan 为显式 `credit_balance`、option/金额/币种事实合法；
   - 再通过真实 Redeem/commission 调用链触发行为。
3. 不得为历史无 snapshot 记录补造 exact；若测试意图是历史缺失，应改为断言稳定拒绝与零写入，而不是成功。
4. 保留邀请佣金测试原业务断言：幂等、奖励资格、金额、账户/记录数量、邀请隔离。不得为了通过而删除断言或跳过。
5. 证明 `FulfillmentSnapshot` 由 Insert 冻结，current Plan 仅负责当前资格，snapshot 负责授权事实；不要直接手写 snapshot JSON，除非测试明确验证损坏/历史数据。
6. 运行：
   - 最小失败测试 RED→GREEN；
   - `go test ./service -run 'Invitation|Commission|Redeem|Redemption' -count=1`；
   - 关键测试 `-count=10`；
   - `go test ./service -count=1`。
7. 若包级出现 Redis 全局夹具 panic 或 teardown 缺表，记录精确证据。属于本文件 setup/migration 缺失则最小修复测试基础设施；不属于 B 则用 Orca `question` 请求协调器分流，不得屏蔽 panic。

## 可恢复进度

第一项改动创建并提交：

- `.scratch/agent-progress/issue-21/fixture-b-status.md`
- `.scratch/agent-progress/issue-21/fixture-b-evidence.md`
- `.scratch/agent-progress/issue-21/fixture-b-contract.md`

每个迁移小步提交。推荐提交：`test(invitation): 通过授权入口构造兑换夹具`。上下文约 80% 前形成 clean/HANDOFF_READY。

## 验收与非目标

验收：旧旁路夹具可重复 RED；Insert/权威入口迁移后 GREEN；snapshot 字段实际存在且 invitation 原断言保持；service 定向、count=10 和包级通过；diff-check；clean tree；有效 worker_done。

非目标：修改生产 Redemption fail-closed、CreditValuation、管理员 UI/i18n、controller 支付夹具、三数据库实机、部署。Agent 不合并、不关闭 Issue、不回收工作树。
