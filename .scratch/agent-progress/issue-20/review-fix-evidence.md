# Issue #20 Standards 评审修复证据

## 基线证据

- `git rev-parse HEAD && git status --short --branch`
  - HEAD：`9e3329d0f4b509d1179c895c52f01af7a19f0ca4`
  - 分支：`jiwangyihao/issue-20-valuation-foundation`
  - staged 0、unstaged 0、untracked 0。
- 已读取父 PRD #19、Issue #20、ADR 0002、规格、计划、共享执行文档、Issue #20 指令、协调器验收清单及 Standards 完整报告。
- 已加载 `diagnosing-bugs`、`tdd`、`codebase-design`、`shadcn-ui`、`orca-cli` 与 `orchestration` 技能。

## Finding 1：历史精确价格污染

### 待复现症状

历史有价套餐的 `price_amount_micros` 为 NULL 时，仅修改名称、状态或其他非价格字段，提交 payload 不得从 JavaScript `number` 推导 micros，数据库精确列必须继续为 NULL。

### RED / GREEN

- RED（前端）：`bun test src/features/subscriptions/lib/plan-form.test.ts --test-name-pattern "does not promote"`；`0 pass / 1 fail`，`'price_amount' in payload.plan` 实际为 `true`，证明表单把兼容 Number 显示值重新提升为提交权威值。
- RED（后端）：`go test ./controller -run TestAdminUpdateSubscriptionPlanPreservesLegacyPriceWhenPriceFieldsAreAbsent -count=1`；失败，历史 `price_amount` 从 `40.123456` 被覆盖为 `0`，说明后端当前无法区分更新请求中价格字段缺失与显式零。
- 根因：`planToFormValues` 把历史 `price_amount` Number 字符串化但不保留来源权威性；`formValuesToPlanPayload` 无条件生成 micros；`decodeAdminUpsertSubscriptionPlanRequest` 与更新 map 又无条件写两列。
- 修复：表单显式保存 `new` / `exact` / `legacy` 来源与 `price_amount_changed`，只在新建或用户明确输入时提交原始十进制文本及由 `BigInt` 生成的 micros；后端保留价格字段存在性，仅在 update 请求提供价格时写两列。
- GREEN（前端）：`bun test src/features/subscriptions/lib/plan-form.test.ts`；`13 pass / 0 fail`。覆盖历史无关编辑、非权威显示标记、`0`、`Number.MAX_SAFE_INTEGER`、`0.1 + 0.2`、显式六位小数及 `int64` 最大边界。
- GREEN（后端）：`go test ./controller -run "TestAdmin(Create|Update)SubscriptionPlan.*ExactPrice|TestAdminUpdateSubscriptionPlanPreservesLegacyPriceWhenPriceFieldsAreAbsent" -count=1`；通过。数据库历史 `price_amount` 保持 `40.123456` 且 micros 继续为 NULL，显式更新继续精确往返。
- 反证：创建有价套餐仍由现有规范化逻辑要求 micros；显式零继续作为提供过的价格处理；没有 `toFixed`、Number 反推或容差旁路。

## Finding 2：schema fail-open

### 根因与方案

- `price_amount_micros BIGINT` 是估值权威值，但管理员前向创建/更新仍按既定兼容合同在同一请求写 `price_amount`，并允许 `int64` 最大 micros（`9223372036854.775807`）；旧 `decimal(10,6)` 无法保存该兼容展示值。因此不能删除扩宽，选择让关键迁移 fail-closed。
- RED：`go test ./model -run TestSubscriptionPlanPriceAmountMigrationFailsClosedOnMetadataError -count=1`；元数据查询和后续 MySQL ALTER 在 SQLite 夹具上均失败并仅 warning，`migrateDB` 继续执行，最终错误来自无关后续迁移，而不是 `subscription_plans.price_amount` 元数据失败。
- 修复：`migrateSubscriptionPlanPriceAmount` 返回错误；PostgreSQL / MySQL 元数据查询和 ALTER 失败均带列上下文返回；`migrateDB` 在任何后续迁移前传播该错误。SQLite 保持合法的 type-affinity 分支。
- GREEN：`go test ./model -run "TestSubscriptionPlanPrice(AmountMigrationFailsClosedOnMetadataError|MicrosMigrationLeavesLegacyRowPending)" -count=1`；通过。元数据错误稳定从 `migrateDB` 返回，历史 `price_amount_micros` 与 `valuation_currency` 仍为 NULL。
- 回归：`go test ./model -run "TestSubscriptionPlanPrice" -count=1`；通过。未回填历史行，未修改 migration marker，未改变旧展示/支付读取路径。

## Finding 3：计划级线性化

### 当前入口与锁序证据

- 生产 allocation 入口：订单完成 `CompleteSubscriptionOrderTx`、兑换 `Redeem`、计时转换 `confirmTimedSubscriptionConversion`、管理员 increase `AdjustCreditBalance`；四者最终均调用 `GrantCreditBalanceTx`。直接调用的其余引用均为测试。
- `GrantCreditBalanceTx` 当前锁序是用户行 → 幂等 ledger 查询 → 可选未锁 `TargetPlanSnapshot` / 普通计划读取 → 用户现有权益行。订单回调明确传入从购买快照构造的 `TargetPlanSnapshot`，因此可能完全不重读权威全局 Credit plan。
- controller 币种更新当前锁序是全局 Credit 套餐行 → `CreditValuationCurrencyLockedTx` 计数权益/估值状态/ledger。两条路径没有共享计划锁，故首个 grant 与币种更新可各自观察旧状态并同时成功。
- 转换入口另有 `conversion_guard_version` 原子 update，但仅覆盖 conversion enablement；订单、兑换、管理员 increase 仍没有同一接缝。disabled-plan 的新 grant 拒绝分别存在于购买/兑换/转换入口，既有权益消费不经过 grant。
- 最终方案：新增 model 计划级数据库 guard，MySQL/PostgreSQL 使用同一全局 Credit plan 行 `FOR UPDATE`，SQLite 依靠同一写事务的单写语义；grant 与币种更新都必须在任何权益/估值存在性判断或写入前调用。锁序固定为 Credit plan → 权益/估值/ledger；controller 不再私有持有冻结规则。
- `TargetPlanSnapshot` 只保留订单定价/授予事实，不能充当当前全局 Credit plan 权威状态或绕过 guard；grant 必须按 `TargetPlanId` 重读并验证唯一计划。
- disabled-plan 合同：新的 allocation 由共享 guard 稳定拒绝；已有权益消费不调用 allocation guard，语义保持不变。

### RED / GREEN

- RED：先持久化 `TestCreditBalancePlanGuardLinearizesFirstGrantAndCurrencyUpdate` 与 `TestCreditBalancePlanGuardRejectsNewAllocationWhenPlanDisabled`；旧实现因共享 guard 和稳定 allocation 错误不存在而无法构建，证明 model 接缝缺失。
- Worker 修复：`AcquireCreditBalancePlanGuardTx` 在 MySQL/PostgreSQL 对唯一全局 Credit plan 使用 `SELECT ... FOR UPDATE`；SQLite 在同一行执行无值变化的事务写以取得单写 guard。`GuardCreditValuationCurrencyUpdateTx` 在同一 guard 下执行冻结检查；controller 只消费该 model 合同。
- Worker GREEN：定向用例 `-count=1`、`-count=10` 与窄范围 `go test -race` 均通过。
- 协调器接管后发现完整 controller 回归中的两个既有不可变支付快照测试失败：当前计划在下单后停用或删除时，Stripe/Epay 回调无法按已授权订单快照履约。最小复现为 `go test ./controller -run "Test(StripeCreditWebhookFulfillsImmutableSnapshotWithoutInvitation|SubscriptionEpayCreditPurchaseUsesImmutableSnapshot)" -count=1`，修复前稳定 FAIL。
- 协调器修正：所有 allocation 先尝试取得当前全局计划 guard；新兑换、转换和管理员授予必须读取并服从当前计划状态；只有 `source_type=subscription_order` 且计划身份与 entitlement 类型匹配的不可变订单快照可在 guard 后提供履约配置，当前计划已删除时也可完成已授权订单。幂等重放先于停用拒绝，已有权益消费不经过 allocation 接缝。
- 最终 GREEN：guard 用例 `-count=10` 通过；窄范围 `-race` 通过；上述 Stripe/Epay 最小复现通过；`go test ./model ./controller ./router -count=1` 三包全部通过。SQLite 使用真实文件数据库、WAL 与单写事务证明合法串行结果；未把它宣称为 MySQL/PostgreSQL 行锁实测。

## 外部数据库边界

尚未检测 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN`；没有真实 DSN 时必须记录 SKIP，不宣称三库实测通过。
