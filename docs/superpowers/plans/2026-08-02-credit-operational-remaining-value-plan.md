# Credit 与计时套餐运营剩余价值实施计划

> 本计划只描述实现与发布顺序，不在当前设计任务中修改计费行为。执行时必须逐项满足 `docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`，不得缩减为只修 `end_time` 或 `price_amount` 过滤。

## 目标

在生产基线 `f446a1569c2ced54a3fe438b5c4575659a59241d` 上建立来源快照、Credit 物化估值状态、请求级可逆扣除快照和计时权益估值 grant，使「运营分析 → 付费套餐剩余价值」正确识别不限时 Credit，并在购买、兑换、转换、消费、追加结算、退款、迁移和 CNY/USD 换算下保持数量、金额与置信度一致。

## 执行基线与硬约束

- 实现分支必须从生产提交 `f446a1569c2ced54a3fe438b5c4575659a59241d` 或包含它的后继提交创建；当前根工作区不是生产实现基线，禁止从根分支状态推断生产行为。
- 先保留冻结回归夹具，再扩展 schema；不得用非生产提交的 `end_time` 查询实现替代生产复现。
- 三种数据库必须同时支持：SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6。DryRun 只作辅助证据，最终必须连接真实 MySQL/PostgreSQL 执行迁移和并发测试。
- JSON 编解码只用 `common.Marshal`、`common.Unmarshal*`。
- 金额持久化只用整数微单位；热路径不得使用浮点数、`big.Int` 分配或按请求写第二份账本。
- Credit 数量和估值状态必须在同一事务中更新。任何直接修改 Credit `token_limit/token_used` 的旁路都必须迁移到深模块。
- 计时与 Credit 套餐均不限制模型范围；不恢复 `model_limits` 行为。
- 已有停用计划权益继续可消费；停用计划拒绝新购买、兑换、转换和管理员授予。
- 强制双写开放后禁止 image-only rollback；发布必须按规格第 15 节分阶段执行。

## 顶层模块与接口合同

新增深模块 `model/credit_valuation.go`，调用方只提交来源事实或请求目标累计 Credit。平均值、欠额、FX、置信度、舍入、状态版本和不变量由模块集中处理。计时获得集中到 `GrantTimedSubscriptionTx`；分析只读取状态/grant，不重新推导来源价格。

实现阶段固定使用以下包内 model 接缝：

```go
func newForwardCreditValuationIngress(source CreditValuationSourceSnapshot) (creditValuationIngress, error)
func newBackfillCreditValuationIngress(source CreditValuationBackfillSnapshot) (creditValuationIngress, error)
func ApplyCreditValuationIngressTx(tx *gorm.DB, lockedSub *UserSubscription, ingress creditValuationIngress) (CreditValuationMutationResult, error)
func ApplyCreditValuationOutflowTx(tx *gorm.DB, lockedSub *UserSubscription, credit int64, mutationType string) (CreditValuationMutationResult, error)
func SettleCreditRequestTargetTx(tx *gorm.DB, record *SubscriptionPreConsumeRecord, targetCredit int64, final bool) error
func GrantTimedSubscriptionTx(tx *gorm.DB, request TimedSubscriptionGrantRequest) (*UserSubscriptionCreationResult, error)
```

不得为每条来源暴露独立的“算价格”“算平均值”“恢复退款”浅接口。纯计算 helper 保持包内私有，通过上述接口测试；业务调用方不能构造置信度。

---

## 任务 1：冻结生产缺陷与金额算术合同

**创建：**

- `model/credit_valuation_math.go`
- `model/credit_valuation_math_test.go`
- `model/admin_analytics_paid_subscription_credit_regression_test.go`

**修改：**

- 无业务实现文件；回归测试可以先失败。

### 步骤

- [ ] 用生产结构建立固定夹具：零价格全局 Credit 计划、`40 CNY / 1000` 充值来源、Credit 权益 `EndTime=0`、`TokenUsed=200`。
- [ ] 断言当前活动范围能找到该权益，旧 token 公式得到 32 CNY，但 summary 的金额/计数在修复前为 0；测试名称明确是永久 Credit 回归，不创建临时生产数据。
- [ ] 为 `mulDivFloor` 写表驱动测试：普通值、余数向下取整、`C=A` 清空、`MaxInt64` 中间乘积、结果溢出、分母 0。
- [ ] 为十进制标价转微单位与 CNY/USD 有理数快照写测试：同币种 1/1、正反向、无效/NaN/Inf/非正汇率、非支持币种。
- [ ] 实现无分配、无溢出的 `mulDivFloor`。可使用 `math/bits.Mul64/Div64`；负数不进入该 helper，由调用方先拒绝。
- [ ] 套餐标价只在管理员 upsert 与历史迁移入口从十进制文本严格转为微单位；购买、兑换、转换和授予只读取已持久化的 `price_amount_micros`，核心比例运算只用整数。

### 定向验证

```bash
go test ./model -run 'Test(CreditValuationMath|PaidSubscriptionValueRecognizesNonExpiringCredit)' -count=1
```

首个回归测试预期保持红灯；算术测试必须先绿。

### 提交

```text
test(analytics): 固化不限时 Credit 剩余价值回归
feat(valuation): 新增无溢出微单位算术
```

---

## 任务 2：增加附加式 schema 与迁移门禁

**创建：**

- `model/credit_valuation.go`
- `model/credit_valuation_migration.go`
- `model/timed_subscription_valuation.go`
- `model/credit_valuation_schema_test.go`

**修改：**

- `model/main.go`
- `model/subscription.go`
- `model/credit_balance.go`
- `setting/operation_setting/payment_setting_old.go`
- `model/option.go`
- `model/subscription_conversion.go`
- `model/kyren_payment.go`
- `controller/subscription.go`（套餐 upsert 精确价格合同）
- `web/default/src/features/subscriptions/types.ts` 与套餐管理表单（发送精确微单位字符串）

### 步骤

- [ ] 按规格 5.1 创建 `CreditValuationState`。`UserSubscriptionId` 是非自增主键，`UserId` 单列唯一；不依赖数据库 `CHECK`。
- [ ] 按规格 5.2 创建 `CreditValuationMigration`，用 `version` 主键和 `status` 索引表达门禁。
- [ ] 按规格 5.3 创建不可变 `TimedSubscriptionValuationGrant`，对 `idempotency_key` 与 `(source_type,source_key)` 建命名唯一索引；计时原币种金额不写入 Credit 全局币种。
- [ ] `SubscriptionPlan` 增加权威 `PriceAmountMicros`，管理员套餐 upsert DTO/响应增加十进制字符串 `price_amount_micros`。前端从原始十进制输入生成该字符串；后端严格解析并派生兼容 `price_amount`，拒绝缺失、超过 6 位精度、溢出或两字段不一致。估值来源只读微单位，禁止从 float 反推。
- [ ] 加表时按方言把现有 `DECIMAL(10,6)`/SQLite 值读取为文本后严格回填 `price_amount_micros` 并做数值往返校验；不得先扫描为 `float64`。任一非法行阻止门禁 ready，并在 dry-run 输出稳定 plan ID/reason。
- [ ] 将 `USDExchangeRate` 的原始十进制字符串严格解析为约分后的正整数分子/分母，并通过只读快照原子发布；保留现有 float 仅供旧显示/支付兼容，估值不得从 float 反推。无效配置更新必须失败且保留上一份有效快照。
- [ ] 扩展 `SubscriptionPreConsumeRecord`，包含 `applied_credit`、`deducted_available_credit`、`debt_formed_credit`、三类活动扣除快照、absorbed restore、restored unknown、状态版本和终态时间。
- [ ] 扩展 `CreditBalanceLedger`、`SubscriptionConversion` 与 `SubscriptionEntitlementSnapshot` 的结构化估值/FX 字段。所有微单位和汇率分子分母使用 `int64`；币种最多 8 字符。
- [ ] 把三张新表加入 `migrateDB` 和 `migrateDBFast`。SQLite 走现有串行特殊路径；并行 fast migration 完成后再执行索引/门禁校验，避免表依赖竞态。
- [ ] 新增启动只读 accessor `CreditValuationMigrationReady()`，只在 `InitDB` 完成后加载一次。只有确认全新数据库没有任何套餐历史时允许原子自动 ready；其他数据库保持非 ready，等待显式迁移。测试提供包内重置 helper，业务代码不暴露可变全局指针。
- [ ] 写 schema 反射测试和真实 SQLite migration 测试，断言字段为 bigint、命名唯一索引存在、重复状态/grant 被数据库拒绝。

### 定向验证

```bash
go test ./model -run 'TestCreditValuation(Schema|SQLiteMigration|UniqueConstraints|MigrationGate)' -count=1
```

### 提交

```text
feat(valuation): 新增套餐估值持久化模型
feat(migration): 注册估值表与启用门禁
```

---

## 任务 3：实现 CreditValuation 深模块

**创建：**

- `model/credit_valuation_test.go`
- `model/credit_valuation_concurrency_test.go`

**修改：**

- `model/credit_valuation.go`
- `model/credit_valuation_math.go`

### 步骤

- [ ] 先为 ingress、outflow、restore 和状态不变量写行为测试，不直接测试私有 helper。
- [ ] 实现固定锁顺序：调用方先锁定目标 Credit `UserSubscription`，模块随后 `FOR UPDATE` 读取 `CreditValuationState`。门禁 ready 后状态缺失或数量不一致分别返回 `credit_valuation_state_missing` / `credit_valuation_state_mismatch` 并回滚；只有显式维护命令能将缺失状态修复为 unknown。
- [ ] 由 `newForwardCreditValuationIngress` 从购买、兑换、转换或售后来源事实派生 exact；业务调用方不能直接声明置信度。`newBackfillCreditValuationIngress` 仅供迁移派生 estimated/unknown。
- [ ] 实现已知/estimated/unknown 入账：毛成本、欠额抵扣、净成本、FX 快照和状态版本严格按规格 7.2。
- [ ] 实现移动平均出账：分别移除 exact、estimated、unknown；消耗全部可用量时带走全部余数；超出量只形成欠额。
- [ ] 实现请求目标结算：追加出账、退款产生的 `newly_available`、请求活动快照同比例恢复、absorbed restore 和无来源债务退款转 unknown。
- [ ] 所有错误都返回稳定可判断的 sentinel/code；禁止解析错误字符串决定业务分支。
- [ ] 并发测试覆盖两个入账、入账+预扣、出账+退款，以合法串行结果集合断言最终状态，而不是依赖 goroutine 调度顺序。
- [ ] 在每次状态保存前和事务测试提交后校验数量、非负、unknown 上界、币种与 state version。

### 核心用例

```text
40 CNY/1000 + 消费200 = 32 CNY/800
40 CNY/1000 消费500，再入账 20 CNY/1000 = 40 CNY/1500
40 CNY/1000 + 20 CNY/1000，再消费1000 = 30 CNY/1000
欠额300 + 入账1000/40 CNY = 可用700/28 CNY
```

### 定向验证

```bash
go test ./model -run 'TestCreditValuation(Ingress|Outflow|Restore|Debt|Confidence|Concurrency|Idempotency|Overflow)' -count=1
```

### 提交

```text
feat(valuation): 原子维护 Credit 移动平均成本
```

---

## 任务 4：接入购买、兑换、转换、管理员调整与回收

**修改：**

- `model/credit_balance.go`
- `model/credit_balance_recovery.go`
- `model/credit_balance_adjustment.go`
- `model/subscription.go`
- `model/redemption.go`
- `model/subscription_conversion.go`
- `controller/subscription.go`
- `service/subscription_financial_recovery.go`
- `model/subscription_recovery.go`
- `controller/subscription_payment_completion.go`
- 各支付完成链路现有定向测试

**新增/修改测试：**

- `model/credit_balance_valuation_test.go`
- `model/subscription_conversion_settlement_test.go`
- `model/subscription_recovery_concurrency_test.go`
- `controller/subscription_external_credit_lifecycle_test.go`

### 步骤

- [ ] `CreditBalanceGrantRequest` 增加结构化来源快照，并由包内构造器生成不可伪造的 `creditValuationIngress`；门禁 ready 时任何正向入账都必须显式提供来源事实，不能在 `GrantCreditBalanceTx` 内读取“当前充值档位”补猜。
- [ ] `GrantCreditBalanceTx` 锁定或创建余额权益后调用 ingress；深模块在同一事务同时更新 `token_limit` 与估值状态，再把结果写入 ledger 结构化字段和幂等指纹。调用方不得在模块前后另改数量。重放只返回已提交结果。
- [ ] 订单创建时从充值档位 `price_amount_micros` 把标价微单位、币种和规则版本写入 `EntitlementSnapshot`；支付回调只用快照，不用回调时当前套餐价格或 `float64 PriceAmount`。
- [ ] Credit 兑换在兑换成功事务读取档位、冻结标价/FX，然后调用 grant；兑换记录的 fulfillment snapshot 保存相同值。
- [ ] 转换确认在第二次事务内重算 `gross_credit = full_31_day_blocks × credit_basis + current_remaining_credit`，并冻结源档位 `price_amount_micros`、同一份 `credit_basis`、毛/净成本、规则版本和 FX；转换、ledger 与状态同事务写入，不做部分周期按秒折算。
- [ ] `RecoverCreditBalanceTx` 通过深模块在同一事务同时更新 Credit 数量和移动平均成本；退款、拒付和管理员 decrease 都不按原订单价格撤回混合池价值。
- [ ] 扩展 Credit 计划更新合同：首次配置必须明确选择 CNY/USD 估值币种；存在任一 Credit 权益、估值状态或估值 ledger 后拒绝普通币种修改。
- [ ] `CreditBalanceAdjustmentRequest` 增加 `PlanId`。increase 必填并验证合格充值档位；decrease 要求 `PlanId=0`。参数指纹包含 plan、价格/FX 快照和规则版本。
- [ ] 消除所有 Credit 余额直接 `Updates(token_limit/token_used)` 旁路，包括转换目标结算；统一进入深模块。
- [ ] 保护 `duchuanbo` 边界：消费选择不检查 `plan.Enabled`，但上述所有新入账入口检查档位/功能开关。

### 定向验证

```bash
go test ./model ./controller -run 'Test(CreditBalance.*Valuation|SubscriptionConversion.*Valuation|SubscriptionOrderRecovery.*Valuation|AdminCreditBalance.*Valuation|ExternalCreditLifecycle)' -count=1
```

### 提交

```text
feat(subscription): 为 Credit 来源冻结运营估值
fix(subscription): 原子同步退款与 Credit 成本基础
```

---

## 任务 5：为计时权益建立不可变 grant 时间线

**修改：**

- `model/timed_subscription_valuation.go`
- `model/subscription.go`
- `web/default/src/features/subscriptions/components/dialogs/user-subscriptions-dialog.tsx`（对应管理员计时授予 UI 在任务 9 完成）
- `model/redemption.go`
- `controller/subscription.go`
- 订单完成与管理员绑定调用点

**新增测试：**

- `model/timed_subscription_valuation_test.go`
- `model/timed_subscription_valuation_concurrency_test.go`

### 步骤

- [ ] 定义 `TimedSubscriptionGrantRequest`：来源唯一身份、用户/plan、整数标价、原始币种、期限/重置快照、结构化来源类别和规则版本。前向有价来源由模块派生 exact；邀请/试用由来源派生不估值，调用方不能直接指定置信度。
- [ ] 用 `GrantTimedSubscriptionTx` 包住现有 `CreateUserSubscriptionFromPlanWithResultTx`，利用返回的 `EventStartTime/EventEndTime` 写不可变 grant；现有低层函数降为模块内部实现，所有业务调用点迁移。
- [ ] 订单、兑换和管理员绑定提供来源幂等键与档位快照。管理员绑定视为售后授予；邀请/试用不通过价格大于 0 自动判断。
- [ ] 同一权益续期必须新增 grant，不覆盖旧 grant；重复来源重放不再次延长期限。
- [ ] 计时 grant 保留档位原币种并使用 1/1 FX；套餐改价或改币种后旧 grant 的金额、币种、Credit 和期限保持不变。只有转换进入 Credit 时才按确认时 FX 换算。
- [ ] 不新增退款自动撤销计时服务或 reversal schema；现有管理员失效后的分析使用实际状态/`end_time`。未来若增加退款联动撤销，先单独定义权益缩短与估值逆向记录合同。

### 定向验证

```bash
go test ./model ./controller -run 'TestTimedSubscriptionValuation(Grant|Renewal|Reprice|Redemption|Admin|Idempotency|Concurrency)' -count=1
```

### 提交

```text
feat(subscription): 冻结每次计时权益授予价值
```

---

## 任务 6：把请求预扣、实时增量和最终结算接入请求级估值

**修改：**

- `model/subscription.go`
- `model/subscription_delta_coalescer.go`
- `service/funding_source.go`
- `service/billing_session.go`
- `service/quota.go`
- `model/task.go`
- 异步任务创建、轮询、退款和重算的现有调用点

**新增/修改测试：**

- `model/credit_valuation_request_test.go`
- `model/subscription_delta_coalescer_test.go`
- `model/subscription_conversion_settlement_test.go`
- `service/credit_billing_settlement_test.go`
- `service/subscription_billing_test.go`
- 异步任务计费测试

### 步骤

- [ ] 将 Credit 预扣改为：锁定已选权益和 `SubscriptionPreConsumeRecord`，由深模块在同一调用内同时增加 `token_used`、移除成本并写请求快照。计时权益保留现有数量语义，记录的估值字段为 0；调用方不得在模块外再更新 Credit 数量。
- [ ] 新增 `SettleUserSubscriptionRequestTarget(requestId, originalSubscriptionId, targetCredit, final)`，由 model 根据预扣记录和转换映射选择实际 Credit 状态；删除业务调用方对 Credit 使用无 `request_id` 的 `PostConsumeUserSubscriptionTokenDelta`。
- [ ] `SubscriptionFunding` 保存 `requestId` 和当前请求目标累计量。`Settle(delta)` 将 delta 转成目标累计量后调用新接口；`Refund()` 以目标 0 执行原样恢复。
- [ ] `BillingSession.SettleWithInput`、`SettleSubscriptionIncrement`、`Reserve`、同步/异步失败退款都只通过同一请求接口更新 Credit。重复 settle/refund 必须由数据库预扣记录幂等，而不只依赖进程内布尔值。
- [ ] 按规格 7.4 处理结算欠额退款：先撤销本请求的 `debt_formed_credit`，再冲减请求成本快照；只向状态恢复 `newly_available`，仍被其他欠额吸收的成本进入 absorbed audit，已被后来入账覆盖的债务退款转 unknown。
- [ ] 按规格 7.5 处理转换期间在途请求：首次结算从转换快照建立虚拟扣除快照，小额最终用量按转换单位价值恢复，追加用量按目标池当前平均值扣除。
- [ ] `TaskPrivateData` 新增 `subscription_request_id`。新任务创建时保存原请求 ID；轮询结算、退款和重算同时要求 request ID 与 subscription ID。ready 后历史任务缺 request ID 时，以持久化 Task 主键生成确定性 legacy identity 并仍通过深模块处理：追加消费按当前平均值，退款新形成的可用量全部标为 unknown；禁止走直接 token delta 旁路。
- [ ] 调整预扣记录清理：只删除终态 `settled/refunded`；默认保留 90 天并允许环境配置。非终态记录不自动删除，另提供只读诊断统计，由运维调查后处理。
- [ ] 搜索并迁移所有 `PostConsumeUserSubscriptionTokenDelta` 调用点。实现完成后，该函数只能留作包内计时兼容 helper，不能从 controller/service/relay 直接调用 Credit。

### 定向验证

```bash
go test ./model ./service ./controller -run 'Test(CreditValuationRequest|SubscriptionDeltaCoalescer|SubscriptionConversionSettlement|CreditBillingSettlement|SubscriptionBilling|.*Task.*Subscription)' -count=1
```

### 提交

```text
refactor(billing): 按请求结算 Credit 目标用量
fix(billing): 精确恢复请求估值扣除快照
```

---

## 任务 7：实现可预演、可重跑的历史迁移命令

**创建：**

- `model/credit_valuation_backfill.go`
- `model/credit_valuation_backfill_test.go`
- `model/timed_subscription_valuation_backfill.go`
- `model/timed_subscription_valuation_backfill_test.go`
- `credit_valuation_command.go`（根 `main` package）

**修改：**

- `main.go`
- `model/main.go`
- `model/option.go`（只复用现有选项加载，不新增动态重估）

### 步骤

- [ ] 在根二进制增加早期子命令：`/new-api credit-valuation-migrate (--dry-run|--apply|--verify|--repair-missing-as-unknown|--suspend) --version N [--batch-size N] [--reason TEXT]`。五种模式必须互斥；`--reason` 仅供 `--suspend` 且必填，`--batch-size` 仅供 `--apply`。命令完成后退出，不启动 HTTP、Redis、定时任务或后台轮询。
- [ ] 从 `InitDB` 抽出只建立连接且不运行 migration 的维护初始化接口。`--dry-run` 与 `--verify` 必须只读；`--apply` 假设加表版本已经部署，只允许写 `CreditValuationState`、`TimedSubscriptionValuationGrant` 和迁移 marker，不触发其他业务迁移。
- [ ] 维护命令在连接后加载 Option 原始字符串，通过与运行时相同的严格解析器读取一次 `USDExchangeRate` 有理数并固化到 marker；不能读取二进制 float 后反推，也不能在批次中再次读取可变全局值。
- [ ] 实现 Credit 来源收集器。来源优先使用结构化 ledger/快照，按 `(source_type,source_id)` 去重；管理员计时 grant 使用 `(source_type,source_key)`，不得以 `(user_id,plan_id)` 猜订单。
- [ ] 严格按规格 11.1 使用最终可用余额比例。迁移结果全部为 estimated；无分母、歧义、unsupported FX 的份额进入 unknown，并记录稳定 reason code。
- [ ] 实现计时 grant 恢复优先级。只有来源与 `FulfilledSubscriptionID`/兑换 fulfillment/管理员记录能唯一关联时才落 estimated grant；歧义窗口输出 unknown，不选“最近订单”。
- [ ] dry-run 输出稳定 JSON：版本、估值币种、FX、各置信度行数/金额/Credit、歧义原因、批次边界、checksum；两次相同数据库快照输出必须逐字相同（时间字段除外，checksum 不包含运行时间）。
- [ ] apply 使用 marker CAS 防并发，按稳定主键分批 upsert；同版本 ready 重放为无操作。failed/running 只能在写流量停止且输出 checksum 与本次 dry-run 一致时继续。
- [ ] 正式完成前运行全表不变量校验，再把 marker 原子置为 ready。任何一项失败都保持 failed，不允许部分 ready。
- [ ] apply/verify 必须检查没有非终态 `SubscriptionPreConsumeRecord`、仍会回调结算的订阅资金异步 Task 或旧进程写会话；存在任一项时拒绝 ready，并输出稳定 blocker reason。
- [ ] 增加 `--repair-missing-as-unknown`，仅在停写维护窗口、显式新 migration version 下运行；它不得成为 HTTP 写路径的自动降级。修复后必须重新 `--verify` 才能 ready。
- [ ] 增加 `--suspend --version N --reason TEXT`，只允许在停写维护窗口将 ready marker 原子改为 suspended；没有原因或 marker 非 ready 时拒绝。suspended 后只能运行只读验证、修复或新版本迁移，不能恢复正常 HTTP 写入。
- [ ] 若数据库完全没有任何套餐权益、成功订单、已兑换套餐或管理员授予历史，可在初始化事务直接创建 ready marker；除此之外一律要求显式 dry-run/apply。
- [ ] 增加 `--verify` 只读模式，供发布后检查 marker、状态数量一致、币种、非负、unknown 上界、重复 grant 和 checksum。

### 定向验证

```bash
go test ./model . -run 'Test(CreditValuationBackfill|TimedSubscriptionValuationBackfill|CreditValuationCommand)' -count=1
```

并在固定 SQLite fixture 上执行：

```bash
go run . credit-valuation-migrate --dry-run --version 1
go run . credit-valuation-migrate --apply --version 1 --batch-size 100
go run . credit-valuation-migrate --verify --version 1
```

### 提交

```text
feat(migration): 新增套餐估值预演与回填命令
```

---

## 任务 8：切换付费套餐剩余价值后端口径

**修改：**

- `dto/admin_analytics.go`
- `model/admin_analytics_paid_subscription.go`
- `model/admin_analytics_paid_subscription_test.go`
- `controller/admin_analytics.go`
- `controller/admin_analytics_test.go`

### 步骤

- [ ] 为 `AdminAnalyticsMoneyAmount/Breakdown` 增加 `amount_micros` 字符串；所有内部汇总和排序改为 `int64` 微单位，最后一步才生成兼容 `amount float64`。
- [ ] 扩展 summary、user、plan、source、subscription DTO 的 exact、estimated、unknown、状态版本和 `snapshot_semantics` 字段；将 `time_based_value` 改为可空。
- [ ] `adminBuildPaidRowsFromSubscriptions` 显式按 `entitlement_type` 分流。Credit 分支不检查余额套餐价格和 `end_time`；门禁 ready 时只读估值状态，门禁未 ready 时继续旧面板并返回 `migration_not_ready` warning，不展示半迁移金额。
- [ ] 计时分支按 grant 时间线计算。窗口重叠、来源歧义或缺 grant 时分别返回 warning/unknown，不用当前套餐价格补猜。
- [ ] 计时 grant 按原币种分别计算时间/token/recognized 微单位。单条合并权益跨多个历史币种时返回 `*_by_currency`；旧 singular MoneyAmount 只在恰有一个币种时返回，否则为 null，绝不静默相加。
- [ ] Credit 行使用 `recognized=token=exact+estimated`、`time=null`、`valuation_basis=credit_moving_weighted_average`。余额为正即计为一条有效有价权益，即使金额全 unknown；exhausted/debt 保留明细但不计 active。
- [ ] 删除 Credit 的 `adminBestSubscriptionOrders` 关联。辅助订单只能经 ledger source 追溯；source breakdown 使用 `credit_balance_pool/moving_weighted_pool`。
- [ ] `snapshot_at` 早于状态 `updated_at` 时返回最新状态和 `current_only` warning，不声称历史回放。
- [ ] 保证邀请付费统计显式只查 `timed` 且排除邀请/试用来源；共享 helper 不能把 Credit 混入邀请金额或付费用户数。
- [ ] 让任务 1 的 32 CNY 冻结回归转绿，并在 summary/users/subscriptions/plans/sources 五个 endpoint 断言一致。

### 定向验证

```bash
go test ./model ./controller -run 'Test(PaidSubscriptionValue|InvitationPaid).*' -count=1
```

### 提交

```text
fix(analytics): 按权益估值状态统计剩余价值
```

---

## 任务 9：更新管理员调整与运营分析前端

**修改：**

- `web/default/src/features/subscriptions/types.ts`
- `web/default/src/features/subscriptions/api.ts`
- `web/default/src/features/subscriptions/components/admin-credit-balance-panel.tsx`
- `web/default/src/features/subscriptions/components/admin-credit-balance-panel.test.tsx`
- `web/default/src/features/admin-analytics/types.ts`
- `web/default/src/features/admin-analytics/index.tsx`
- `web/default/src/features/admin-analytics/lib/format.ts`
- `web/default/src/features/admin-analytics/lib/panel-fields.ts`
- 对应 frontend tests
- `web/default/src/features/subscriptions/components/credit-balance-plan-card.tsx`
- `web/default/src/features/subscriptions/components/dialogs/user-subscriptions-dialog.tsx`
- `web/default/src/i18n/static-keys.ts`
- `web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`

### 步骤

- [ ] 扩展 `CreditBalanceAdjustmentRequest`：increase 要求 `plan_id`，decrease 不发送该字段。复用 `getAdminPlans()`，筛选规格第 8 节的合格充值档位。
- [ ] Credit 计划表单增加 CNY/USD 估值币种；后端返回已冻结时禁用修改并解释原因，不能只靠前端禁用保护。
- [ ] 管理员计时授予表单增加必填原因和可重试幂等键；失败重试复用同一键，成功或参数变化后生成新键，避免重复续期。
- [ ] 在 increase 表单新增 Base UI `Select`；选项显示标题、档位标价和 Credit。根据整数/十进制输入显示预计运营价值；预览仅供确认，服务端结果是最终值。
- [ ] operation 切换到 decrease 时清空所选 plan；提交 payload 和幂等键重置测试必须覆盖 plan 变化，避免复用旧参数的幂等键。
- [ ] 扩展分析类型与格式化：优先解析 `amount_micros`，以 `BigInt`/字符串完成格式化，禁止先转 JS Number 导致大金额精度损失；旧响应缺字段时才回退 `amount`。
- [ ] 对跨币种计时权益展示按币种拆分的价值；旧 singular 字段为 null 时不得用当前套餐币种重新合并。
- [ ] 顶部卡片分开展示运营剩余价值、其中 estimated 金额和 unknown Credit；Credit 明细将时间价值显示为“不适用”，估值依据显示“移动加权平均”。
- [ ] 对 `current_only`、estimated、unknown 和 migration-not-ready 显示语义准确的 Alert，并提供刷新；不得使用会计负债、可退款或实收文案。
- [ ] 更新 panel field tests、API payload tests 和组件交互测试。只组合仓库已有 shadcn/Base UI 组件，不引入新 UI 依赖或第二套表单模式。
- [ ] 所有新增 UI 文案使用 `t()`，补齐六种语言；运行 `bun run i18n:sync` 后读取报告，要求 missing/extras 为 0。不得把英语值复制到其他语言充数。

### 定向验证

从 `web/default/` 运行：

```bash
bun test src/features/subscriptions/components/admin-credit-balance-panel.test.tsx src/features/admin-analytics
bun run i18n:sync
bun run typecheck
```

### 浏览器验收

- increase 时没有档位不能提交；选 `40 CNY/1000`、输入 800 时预览 32 CNY，payload 含正确 `plan_id` 和原始整数 800。
- decrease 不显示/发送档位。
- 付费套餐剩余价值显示 32 CNY、exact 32、estimated 0、unknown 0；Credit 时间价值显示“不适用”。
- 人为返回 `current_only` warning 时页面展示提示并可刷新。

### 提交

```text
feat(subscription-ui): 为 Credit 售后授予选择估值档位
fix(analytics-ui): 披露套餐剩余价值置信度
```

---

## 任务 10：完成三数据库、并发与兼容验收

**修改/新增：**

- `model/credit_valuation_crossdb_test.go`
- 既有 migration、conversion、recovery、billing 并发测试
- `.github/workflows/pr-check.yml`（仅当现有 CI 能安全提供对应服务；否则保留外部验收命令，不虚构 CI）

### 步骤

- [ ] SQLite 使用真实文件或共享内存数据库运行完整 schema、迁移、grant、预扣、结算、退款和分析回归。
- [ ] 复用 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 惯例连接真实 MySQL 5.7.44 与 PostgreSQL 9.6.24；环境变量缺失时测试必须明确 SKIP，本地/发布验收要求零 SKIP。
- [ ] 验证命名唯一索引、`BIGINT`、不可变 hooks、`FOR UPDATE`、幂等冲突与批次重放在三种数据库上行为一致。
- [ ] 并发矩阵覆盖 grant+grant、grant+consume、consume+restore、conversion+settlement、refund+admin decrease；断言结果属于合法串行化集合且每次提交后状态与数量一致。
- [ ] 运行 `-race` 覆盖纯 Go 算术、合并器和进程内门禁缓存；数据库测试无需用竞态检测代替数据库并发验收。
- [ ] 兼容测试覆盖旧订单快照、旧前端不带 `plan_id` 的 decrease，以及 increase 明确返回 plan-required；不得为旧 increase 静默选择某档位。
- [ ] 兼容测试覆盖 ready 后历史 Task 缺 request ID：相同 Task 主键重放不重复变更，追加消费按当前平均值，退款可用量为 unknown。
- [ ] 加入性能基准：Credit 预扣/结算相对生产基线只允许同一事务多一次状态行更新和预扣记录字段更新，不得出现 N+1 来源查询或每请求 ledger insert。

### 验证命令

```bash
go test ./model -run 'TestCreditValuationCrossDatabase' -count=1 -v
go test ./model ./service -run 'Test.*(CreditValuation|Subscription).*Concurrency' -count=20
go test -race ./model ./service -run 'Test(CreditValuationMath|SubscriptionDeltaCoalescer|CreditValuationMigrationGate)' -count=1
```

发布验收日志必须明确记录三个数据库用例均为 PASS、零 SKIP。

### 提交

```text
test(valuation): 覆盖三数据库估值一致性
```

---

## 任务 11：全量验证、分阶段发布与生产验收

### 本地最终门禁

- [ ] 运行格式化，仅格式化本计划实际修改的 Go/TS/JSON 文件；不要全仓库重排用户文件。
- [ ] 运行后端全套、前端全套、生产构建、i18n、版权和 diff 检查。
- [ ] 确认无临时回归文件、无空文件、无测试账户/计划种子进入生产代码。

```bash
go test ./... -count=1
cd web/default && bun test
cd web/default && bun run i18n:sync
cd web/default && bun run build:check
cd web/default && bun run copyright:check
git diff --check
```

### 加表版本发布

- [ ] 构建并记录不可变镜像 digest；确认只增加 schema、快照捕获和维护命令，门禁仍非 ready。
- [ ] 按服务器惯例使用本地一次性脚本、`flock` 和 `trap` 部署；探测健康、版本、旧计费与停用计划现有权益消费。
- [ ] 运行在线只读 `credit-valuation-migrate --dry-run` 两次，确认 checksum 相同；审阅 estimated/unknown、歧义、unsupported currency，不接受空或异常狭窄输出。

### 维护窗口切换

- [ ] 停止所有写流量和后台任务；确认没有活跃预扣/异步结算，或等待其结束。不得在仍有旧进程写 Credit 时应用迁移。
- [ ] 创建一致 PostgreSQL 备份，记录路径与 SHA-256；证明新 schema 只附加表/列，但仍按强制双写不可逆边界处理。
- [ ] 使用同一镜像 digest 运行 `--apply`，随后 `--verify`。任何不变量失败都保持旧分析和写门禁，不启动服务。
- [ ] marker ready 后重启所有实例，使它们在启动时读取同一版本；先保持外部写流量关闭，运行受控购买、消费、退款和分析探针。
- [ ] 准备紧急 `--suspend` 命令并做演练；在任何外部写流量开放前确认该命令、停写层和恢复流程均可用。
- [ ] 开放写流量，观察状态 mismatch、unknown 增长、unsupported FX、结算延迟、数据库锁等待和错误率。

### 生产只读/受控验收

必须证明以下数据，而不是只证明页面静态渲染：

```text
40 CNY / 1,000 Credit
消费 200
Credit end_time = 0
available_credit = 800
exact_cost_micros = 32,000,000
recognized_remaining_value = 32 CNY
active_paid_subscription_count = 1
```

不得插入未授权临时生产用户、套餐或权益。若没有合适的真实受控账号，使用备份克隆或隔离验收数据库完成行为证明；生产数据库只做只读不变量查询。

### 回滚门禁

- [ ] ready 前可执行 image-only rollback。
- [ ] ready 后开放写流量前，允许停止服务后回滚并保留附加表。
- [ ] 强制双写开放后禁止 image-only rollback；故障优先向前修复。必须回滚时先停止写流量，通过维护命令将 marker 原子置为 `suspended` 并记录原因，回滚镜像；再次切换前从备份或新迁移版本重建并验证状态。
- [ ] 不删除估值表、不改写 immutable grant/ledger、不把 migration marker 伪装回 pending。

### 最终提交建议

```text
chore(release): 完成套餐运营剩余价值切换
```

---

## 最终验收清单

- [ ] 冻结 32 CNY 回归通过，五个 paid-value endpoint 一致。
- [ ] Credit 不受零价格容器和 `end_time=0` 影响。
- [ ] exact、estimated、unknown 和 current-only 均有独立测试与 UI 披露。
- [ ] 移动平均、欠额抵扣、请求快照退款和转换在途结算无价值漂移。
- [ ] 购买、兑换、转换、管理员增加、退款、拒付、管理员减少都在数量事务内更新估值。
- [ ] 计时购买、兑换、管理员授予均有不可变 grant；改价不回写。
- [ ] CNY/USD 在入账时冻结，其他新异币种拒绝，历史 unsupported 为 unknown。
- [ ] 邀请/试用继续排除，Credit 不产生邀请收益。
- [ ] 停用计划现有权益继续可消费，新入账仍拒绝。
- [ ] SQLite、MySQL 5.7、PostgreSQL 9.6 实测零 SKIP。
- [ ] dry-run 可重复、apply 可重放、verify 全部通过。
- [ ] 强制双写发布后没有 image-only rollback 路径。
- [ ] 后端全套、前端全套、生产构建、i18n、版权和 `git diff --check` 全部通过。
