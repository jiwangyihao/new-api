# Credit 与计时套餐运营剩余价值修复规格

## 1. 状态与覆盖关系

- 日期：2026-08-02
- 状态：实现就绪
- 生产审计基线：`f446a1569c2ced54a3fe438b5c4575659a59241d`
- 决策：`docs/adr/0002-credit-operational-remaining-value.md`
- 领域词汇：根目录 `CONTEXT.md`

本规格覆盖 `2026-06-04-paid-subscription-analytics-design.md` 中以下旧假设：所有有价权益都要求 `SubscriptionPlan.price_amount > 0`；查询时的套餐当前价格是主金额来源；所有权益都取 `min(token_based_value, time_based_value)`；`(user_id, plan_id)` 足以关联 Credit 权益与订单。这些假设不得再用于 Credit；计时权益也必须在前向授予时冻结价值。

## 2. 问题与冻结复现

Credit 余额套餐是零价格、不失效、不重置的服务限制容器；有价格的充值档位和实际持有的 Credit 权益不是同一 `plan_id`。生产实现有三个独立缺陷：

1. `adminBuildPaidRowsFromSubscriptions` 丢弃 `plan.PriceAmount <= 0`，因此丢弃全局 Credit 权益；
2. `adminRecognizedRemainingValue` 对 `end_time = 0` 得到零时间价值，再取 token/time 较小值，强制把 Credit 算成零；
3. `adminBestSubscriptionOrders` 按 `(user_id, plan_id)` 关联，而订单指向充值档位、权益指向全局 Credit 套餐。

冻结回归夹具：

```text
充值档位：40 CNY / 1,000 Credit
Credit 权益：entitlement_type=credit_balance, end_time=0
已消费：200 Credit；可用：800 Credit
期望：32 CNY，active_paid_subscription_count=1
生产基线：0 CNY，active_paid_subscription_count=0
```

修复不得只删除价格或时间过滤；必须建立来源估值快照、合并余额成本基础、消费分摊、请求退款恢复、历史迁移和置信度披露。

## 3. 目标与非目标

### 3.1 目标

- 为 Credit 购买、兑换、转换、售后授予和跨币种入账冻结可审计的运营估值。
- 以移动加权平均随 Credit 消费、追加结算、退款、管理员减少、订单退款和拒付同步减少剩余成本基础。
- 保证 Credit 数量与估值在同一数据库事务中原子变更，并在重试和并发下幂等。
- 为计时套餐每次购买、兑换和管理员授予冻结价格，不因以后套餐改价而重写既有价值。
- 对历史数据区分 `exact`、`estimated`、`unknown`，不把估算伪装成精确收款。
- 同时支持 SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6。

### 3.2 非目标

- 不建立会计总账、递延收入、现金负债、退款准备或发票系统。
- 不按订单实付金额给服务估值；促销和渠道差异只作辅助审计。
- 不引入 Credit FIFO/LIFO 批次，也不展示“某订单还剩多少价值”。
- 不为每个 API 请求新增永久 Credit 账本事件。
- 不承诺任意历史秒级 `snapshot_at` 回放；未来历史趋势应使用独立周期快照。
- 不改变套餐模型权限：任何套餐均不限制模型范围，遗留 `model_limits` 继续忽略。
- 不改变邀请隔离：Credit 购买、兑换和转换不产生邀请奖励，也不进入邀请付费口径。
- 本修复不把“订单已退款”自动等同于“计时服务已撤销”。生产基线的订单退款不缩短计时权益；若现有管理员失效操作实际缩短 `end_time` 或改变状态，当前剩余价值自然按缩短后的可交付窗口计算，但本修复不新增计时服务窗口 reversal schema。未来若让退款自动撤销计时服务，必须先单独定义权益缩短与不可变估值逆向合同。

## 4. 不变量

### 4.1 Credit 数量

```text
raw_balance = token_limit - token_used
available_credit = max(raw_balance, 0)
settlement_debt = max(-raw_balance, 0)
```

- `credit_balance` 身份只看显式 `entitlement_type`，不能由 `end_time = 0` 推断。
- 每用户至多一份 Credit 权益；余额为 0 或欠额时权益仍永久保留。
- 已有停用套餐权益仍可消费；停用档位不得新购买、兑换、转换或管理员授予。

### 4.2 Credit 估值状态

每份 Credit 权益必须恰有一行估值状态，且每次提交后满足：

```text
state.available_credit = max(subscription.token_limit - subscription.token_used, 0)
state.exact_cost_micros >= 0
state.estimated_cost_micros >= 0
0 <= state.unknown_credit <= state.available_credit
state.currency = global_credit_plan.currency
state.state_version 单调递增
```

迁移就绪后若状态缺失或数量不一致，业务写事务必须返回 `credit_valuation_state_missing` 或 `credit_valuation_state_mismatch` 并回滚；不得在热路径按当前套餐价格补值，也不得静默降级为 unknown。只有停写维护窗口内的显式 `--repair-missing-as-unknown` 命令可以创建“当前可用量全部 unknown”的保守状态；修复必须写入独立迁移版本、记录 critical 审计并重新执行全表校验。分析读取遇到缺失状态时金额为 0、披露 missing 计数并返回 warning，不能阻止其他完好行展示。

### 4.3 金额

- 持久化金额单位固定为微单位：`1 currency unit = 1,000,000 micros`。
- 所有成本字段为非负 `BIGINT`；业务运算必须检查不超过 `math.MaxInt64`。
- 套餐标价必须从十进制文本严格转换：最多 6 位小数，乘以 `1,000,000` 后必须得到非负整数且不超过 `math.MaxInt64`；禁止舍入、截断或从二进制浮点反推。比例分摊仍按各算法明确向下取整。
- 响应中的浮点 `amount` 只是兼容展示；`amount_micros` 十进制字符串是精确合同和排序依据。

## 5. 数据模型

### 5.1 `credit_valuation_states`

新增 `CreditValuationState`：

| 字段 | 类型 | 约束/含义 |
|---|---|---|
| `user_subscription_id` | int | 主键、非自增；关联 Credit 权益 |
| `user_id` | int | 非空、唯一索引 |
| `available_credit` | bigint | 与权益实时可用量一致 |
| `exact_cost_micros` | bigint | 前向确值尚未摊销金额 |
| `estimated_cost_micros` | bigint | 历史估值尚未摊销金额 |
| `unknown_credit` | bigint | 成本基础未知的可用 Credit |
| `currency` | varchar(8) | 全局 Credit 估值币种，大写 |
| `rule_version` | int | 当前算法版本，首版为 1 |
| `migration_version` | int | 0 表示前向创建；历史首版为 1 |
| `state_version` | bigint | 每次有效变更加 1 |
| `last_mutation_type` | varchar(32) | grant/consume/restore/recovery/admin/reconcile |
| `created_at` / `updated_at` | bigint | 数据库 UTC 秒；`updated_at` 建索引 |

`state_version` 只在有效数量或估值变更时递增；幂等重放和目标值未变化的请求不得递增。

跨库约束只使用 GORM 可表达的主键和唯一索引。MySQL 5.7 不可靠执行 `CHECK`，数值不变量由领域模块验证。

### 5.2 `credit_valuation_migrations`

新增版本化迁移门禁 `CreditValuationMigration`：

| 字段 | 含义 |
|---|---|
| `version` | 主键；首版 1 |
| `status` | pending/running/ready/failed/suspended |
| `valuation_currency` | 迁移时全局币种 |
| `fx_rate_numerator` / `fx_rate_denominator` | 冻结的 `1 USD = X CNY` 有理数 |
| `fx_captured_at` | 汇率冻结时间 |
| `credit_rows_total/estimated/unknown` | Credit 审计计数；历史迁移不记 exact |
| `timed_rows_total/estimated/unknown` | 计时历史审计计数 |
| `checksum` | 确定性输出排序后的 SHA-256 |
| `started_at/completed_at/failure_reason` | 执行审计信息 |
| `suspended_at/suspended_reason` | 紧急停写回滚审计；非 suspended 时为空 |

应用启动时缓存门禁。只有 `ready` 才启用强制估值双写和新分析口径；从非 ready 变为 ready 或 suspended 后必须重启，不在热路径逐请求查询 marker。`ready -> suspended` 只允许停写维护命令携带原因执行，不能由 HTTP 或直接 SQL 作为普通开关切换。

### 5.3 `timed_subscription_valuation_grants`

新增不可变 `TimedSubscriptionValuationGrant`，一行代表一次计时权益获得，而非一份合并权益：

| 字段 | 含义 |
|---|---|
| `id` | 主键 |
| `idempotency_key` | varchar(128)，唯一 |
| `user_subscription_id/user_id/plan_id` | 获得后权益身份，均索引 |
| `source_type/source_key` | `source_key` 为 varchar(160)，组合唯一；例如 `subscription_order:123`、`redemption:456`、`admin:<idempotency_key>` |
| `source_id` | 可空/0 的整数辅助追溯 ID；不参与幂等身份 |
| `event_start_time/event_end_time` | 本次获得追加的服务窗口 |
| `grant_credit` | 本次履约 Credit 快照 |
| `source_price_micros/source_currency` | 档位标价快照 |
| `valuation_amount_micros/valuation_currency` | 入账时换算后的价值 |
| `confidence` | exact/estimated/unknown |
| `rule_version` | 首版 1 |
| `fx_rate_numerator/fx_rate_denominator/fx_captured_at` | 同币种 1/1；否则冻结汇率 |
| `source_snapshot` | TEXT；完整期限、重置、档位和来源 JSON |
| `created_at` | 数据库 UTC 秒 |

模型用 `BeforeUpdate/BeforeDelete` 拒绝修改。订单、兑换和管理员授予使用确定性幂等键；邀请奖励和试用继续创建权益，但不创建有价估值 grant。计时 grant 的 `valuation_currency` 等于授予档位原始币种，`valuation_amount_micros` 不换算到 Credit 全局币种，其 FX 固定为 1/1。只有计时权益转换进入 Credit 时，才按转换确认时汇率换算为全局 Credit 估值币种。

### 5.4 扩展 `SubscriptionPreConsumeRecord`

保留 `PreConsumed` 作为最初预估量，新增：

| 字段 | 含义 |
|---|---|
| `applied_credit` | 此请求当前累计实际承担的 Credit；预扣后等于 `pre_consumed` |
| `deducted_available_credit` | 此请求从当时可用池实际移除、具有请求快照的 Credit；不包含形成的欠额 |
| `debt_formed_credit` | 此请求累计形成且未通过本请求退款撤销的结算欠额 Credit |
| `valuation_subscription_id` | 关联的 Credit 状态；计时且未转换时为 0 |
| `deducted_exact_cost_micros` | 此请求当前占用的确值成本快照 |
| `deducted_estimated_cost_micros` | 此请求当前占用的估值成本快照 |
| `deducted_unknown_credit` | 此请求当前占用的未知成本 Credit |
| `absorbed_restore_unknown_credit` | 请求 unknown 快照中因其他结算欠额吸收而未重新成为可用量的 Credit 审计量 |
| `absorbed_restore_exact_cost_micros` | 本应从请求快照恢复、但因退款先被账户其他结算欠额吸收而仍保持摊销的确值成本审计量 |
| `absorbed_restore_estimated_cost_micros` | 同上，对应估值成本 |
| `restored_unknown_credit` | 原请求欠额后来退款并重新成为可用量、但已无可证明来源成本时计入状态的 unknown Credit |
| `valuation_rule_version` | 0 表示未触及 Credit；否则为规则版本 |
| `settlement_version` | 每次目标累计量变化加 1 |
| `finalized_at` | 最终结算时间；实时增量期间为 0 |
| `status` | consumed/settled/refunded |

现有 `request_id` 唯一约束继续作为请求级幂等键。清理任务只能删除 `settled/refunded` 且早于“最大异步任务生命周期 + 运维保留窗口”的记录；不得按固定 7 天删除仍可能结算的记录。

### 5.5 套餐标价微单位

`SubscriptionPlan` 新增 `price_amount_micros BIGINT NOT NULL DEFAULT 0`，作为运营估值快照的权威标价；现有 `price_amount` 继续用于兼容 API 和展示。管理员套餐 upsert 请求新增字符串字段 `price_amount_micros`：服务端严格解析该字段，以它派生兼容 `price_amount` 并在同一事务写入两列；有价档位缺少该字段、超出 6 位精度、溢出，或同时提交的兼容值不一致时整笔拒绝。读取来源价格时只使用 `price_amount_micros`，不得在购买、兑换、转换或授予事务中从 `float64 PriceAmount` 重新计算。

加表迁移必须以各数据库读取出的 `DECIMAL(10,6)`/SQLite 数值文本逐行生成微单位并做数值往返校验，不能先扫描到 `float64`。负数、超过 6 位小数、溢出、无法精确恢复或文本/数值往返不一致的行阻止估值迁移 ready。前向套餐写入同样拒绝这些值。`price_amount_micros = 0` 的充值档位不能作为可估值来源；全局零价格 Credit 容器本身允许为 0。响应同时返回兼容 `price_amount` 和精确 `price_amount_micros`。

### 5.6 扩展低频快照

`CreditBalanceLedger` 新增：`valuation_currency`、`valuation_gross_cost_micros`、`valuation_net_cost_micros`、`valuation_confidence`、`valuation_rule_version`、`valuation_state_version_after`、`fx_source_currency`、`fx_rate_numerator`、`fx_rate_denominator`、`fx_captured_at`。`SourceSnapshot` 继续保存完整 JSON，但关键金额不得只存在 JSON。

`SubscriptionConversion` 新增：`valuation_currency`、`valuation_source_price_micros`、`valuation_credit_basis`、`valuation_gross_cost_micros`、`valuation_net_cost_micros`、`valuation_confidence`、`valuation_rule_version` 及同一组 FX 字段。`valuation_source_price_micros / valuation_credit_basis` 是未舍入单位价值有理数，供转换期间在途请求退款恢复；不能只保存截断后的每 Credit 微单位。

`SubscriptionEntitlementSnapshot` 保留旧 `price_amount`，并新增整数 `list_price_micros`、`list_price_currency`、`valuation_rule_version`。新订单只从整数快照履约估值；旧订单缺字段进入迁移估值路径。

### 5.7 精确金额响应

扩展 `AdminAnalyticsMoneyAmount` 与 `AdminAnalyticsMoneyBreakdown`：

```json
{"amount":32,"amount_micros":"32000000","currency":"CNY"}
```

后端汇总和排序只使用微单位；前端优先使用 `amount_micros`，旧 `amount` 仅供兼容。

## 6. 深模块接口

新增 `model/credit_valuation.go`。该模块是 Credit `token_limit/token_used` 与 `CreditValuationState` 的唯一写入者；调用方不得先改数量再“通知”估值，也不得在模块返回后另补数量。模块在一次调用中同时更新两行并验证提交前不变量。

包内接口保持最小：

```go
type creditValuationIngress struct { /* 构造后不可变，字段不对调用方开放 */ }

type CreditValuationMutationResult struct {
    StateVersionAfter int64
    NetAvailableCredit int64
    GrossCostMicros int64
    NetCostMicros int64
    RemovedExactCostMicros int64
    RemovedEstimatedCostMicros int64
    RemovedUnknownCredit int64
}

func newForwardCreditValuationIngress(source CreditValuationSourceSnapshot) (creditValuationIngress, error)
func newBackfillCreditValuationIngress(source CreditValuationBackfillSnapshot) (creditValuationIngress, error)
func ApplyCreditValuationIngressTx(tx *gorm.DB, lockedSub *UserSubscription, ingress creditValuationIngress) (CreditValuationMutationResult, error)
func ApplyCreditValuationOutflowTx(tx *gorm.DB, lockedSub *UserSubscription, credit int64, mutationType string) (CreditValuationMutationResult, error)
func SettleCreditRequestTargetTx(tx *gorm.DB, record *SubscriptionPreConsumeRecord, targetCredit int64, final bool) error
```

`newForwardCreditValuationIngress` 只接受购买、兑换、转换和售后授予的结构化来源事实并派生 `exact`；`newBackfillCreditValuationIngress` 只供维护迁移使用并派生 `estimated/unknown`。业务调用方不能直接声明置信度。欠额比例、FX、移动平均、舍入、状态版本、Credit 数量更新和不变量检查全部封装在模块内。模块不提交事务、不清缓存、不发送业务日志。

计时获得集中到 `GrantTimedSubscriptionTx(tx, TimedSubscriptionGrantRequest)`；订单、兑换和管理员绑定不得继续调用低层创建函数后各自补写价值。邀请/试用由结构化来源显式判为不估值，调用方不能自行声明 `exact`。

## 7. 算法

### 7.1 无溢出乘除

所有非负比例使用：

```text
mul_div_floor(a, b, d) = floor(a × b / d), d > 0
```

实现使用 `math/bits` 的 128 位中间乘积或等价无分配算法；不得先做 `a*b` 的 `int64` 乘法，不得在热路径用 float。结果超过 `MaxInt64` 返回错误并回滚。

### 7.2 已知成本入账

```text
source_gross_cost = floor(source_price_micros × gross_credit / source_plan_credit)
debt_offset = min(gross_credit, settlement_debt_before)
net_credit = gross_credit - debt_offset
net_cost = floor(source_gross_cost × net_credit / gross_credit)
```

- `exact` 增加 `exact_cost_micros`；`estimated` 增加 `estimated_cost_micros`；
- `unknown` 不增加金额，只增加 `unknown_credit += net_credit`；
- `net_credit = 0` 时不增加任何剩余成本。

### 7.3 移动平均出账

设操作前 `A=available_credit`，出账 `Q`，实际消耗可用量 `C=min(Q,A)`：

```text
removed_exact = floor(exact_cost_micros × C / A)
removed_estimated = floor(estimated_cost_micros × C / A)
removed_unknown = floor(unknown_credit × C / A)
```

若 `C=A`，直接移除全部余额，避免永久舍入残值。`Q-A` 只形成结算欠额。API 消费、管理员减少、Credit 订单退款和拒付使用同一规则；订单原价与退款额只作审计，不尝试从混合池拿回原批次。

### 7.4 请求预扣、追加与退款

Credit 预扣在同一事务锁定权益和状态，检查足额后增加 `token_used`、按 7.3 移除成本，并把 `deducted_available_credit` 与成本份额写入预扣记录。预扣本身必须足额，不形成欠额；只有已经开始执行后的追加结算允许增加 `debt_formed_credit`。

最终结算接收 `request_id + target_applied_credit`，不接收无归属 delta：

- 目标高于当前 `applied_credit`：差额先消耗当时可用池，按移动平均累加请求快照；超出可用量的部分增加 `debt_formed_credit`，不产生剩余成本。
- 目标低于当前时，令 `refund = applied_credit - target`。先撤销本请求尚未撤销的结算欠额：`debt_refund = min(refund, debt_formed_credit)`；余量 `snapshot_refund = refund - debt_refund` 才冲减 `deducted_available_credit` 及其成本快照。不得把仅取消欠额的退款按成本快照比例处理。
- 在同一锁定事务计算 `available_before`、减少 `token_used`、再计算 `available_after`；只有 `newly_available = available_after - available_before` 可进入估值状态，仍被账户其他结算欠额吸收的部分不恢复剩余价值。
- `snapshot_refund` 对应的活动 exact/estimated/unknown 快照按其占 `deducted_available_credit` 的比例移出；若清空该请求的剩余 `deducted_available_credit`，带走全部舍入余数。该移出份额中最多只有 `min(snapshot_refund,newly_available)` 可恢复到状态；未形成可用量的成本分别累计到 `absorbed_restore_*`，不增加状态金额。
- 若 `newly_available > snapshot_refund`，超出部分来自已被其他入账抵扣过的本请求欠额；它没有可证明的剩余来源成本，必须增加 `unknown_credit` 和 `restored_unknown_credit`，不得按当前池平均值或后来入账价格伪造成本。
- 更新完成后分别扣减 `debt_formed_credit`、`deducted_available_credit` 和活动成本快照，并设置新的 `applied_credit`。重复相同目标为无操作；`final=true` 后相同目标可重放，不同目标只允许明确退款路径。

示例：40 CNY/1000 Credit，预扣 200 移除 8 CNY；期间又以 20 CNY/1000 入账。最终只用 100 且账户没有其他欠额时，必须恢复原快照中的 4 CNY，而不是按新平均值恢复。若请求追加形成 50 欠额、该欠额被后来充值抵扣后又退款 50，则新形成的 50 可用 Credit 没有保留可证明的来源成本，必须标记 unknown。


### 7.5 转换期间在途请求

计时权益转换时，已预扣 Credit 未进入 `current_remaining_credit`，所以也未进入转换入账。转换记录保存第 5.6 节定义的未舍入单位价值有理数。

在途请求首次于转换后结算时：

1. 读取原预扣记录、转换记录和目标 Credit 状态；
2. 以冻结转换单位价值为原 `applied_credit` 建立虚拟已扣成本快照，但不再次减少目标状态；
3. 最终用量小于预扣时，差额按转换快照恢复到目标 Credit；
4. 最终用量大于预扣时，超出部分按目标池当时移动平均出账；
5. 请求日志保留原 `subscription_id`，估值记录目标 `valuation_subscription_id`。

### 7.6 FX

全局 Credit 计划币种为估值币种。快照声明：

```text
1 USD = rate_numerator / rate_denominator CNY
```

USD→CNY 用 `floor(usd_micros × numerator / denominator)`；CNY→USD 反向；同币种为 1/1。只支持 CNY/USD；其他新异币种、非正汇率均原子拒绝。以后汇率变化不改写已有状态。全局 Credit 计划已有余额、状态或账本后不可普通修改币种。
运行时 FX 不直接读取或运算 `float64 USDExchangeRate`。`operation_setting` 必须维护由配置原始十进制字符串解析得到的只读 `CreditFXRateSnapshot`（约分后的正整数分子/分母）；Option 初始化和更新在接受配置前原子替换该快照。每次异币种入账复制当前快照到来源记录。无法把原始配置精确解析为有界有理数时，配置更新或入账失败，不允许从二进制浮点反推汇率。
来源档位标价必须读取 `SubscriptionPlan.price_amount_micros`。兼容字段 `price_amount` 只能展示，不能进入估值算术。

## 8. 来源规则

该单一币种与 FX 规则只作用于 Credit 余额池。尚未转换的计时权益继续按各自 grant 原币种分组汇总，不做跨币种合计或动态换算；转换时才生成 Credit 汇率快照。

| 来源 | 价格依据 | 前向置信度 | 备注 |
|---|---|---|---|
| Credit 购买 | 订单创建时充值档位标价快照 | exact | 不用实际支付金额 |
| Credit 兑换 | 兑换成功事务读取的档位标价 | exact | 视为在其他渠道购买 |
| Credit 转换 | 确认时源档位 `price_amount_micros / credit_basis` | exact | `credit_basis` 必须与 `full_31_day_blocks × credit_basis + current_remaining_credit` 使用同一快照；规则确值，不是新收款 |
| 管理员增加 | 管理员选择的启用档位，按授予量比例 | exact | 售后授予，`plan_id` 必填 |
| 管理员减少 | 操作前池移动平均 | 继承池拆分 | `plan_id` 必须缺省 |
| Credit 退款/拒付 | 操作前池移动平均 | 继承池拆分 | 可形成欠额 |
| 计时购买/兑换/管理员授予 | 授予时档位快照 | exact | 后续改价不回写 |
| 邀请/试用 | 不估值 | 不适用 | 继续排除 |

管理员增加所选档位必须满足：`entitlement_type=timed`、`enabled=true`、`is_trial=false`、`price_amount_micros>0`、`monthly_token_limit>0`、`unlimited_purchase_enabled=true`。

## 9. 事务、锁与幂等

各领域入口可以先锁自己的来源行，但进入 Credit 模块后的顺序固定为：

```text
target UserSubscription -> CreditValuationState -> SubscriptionPreConsumeRecord/ledger result
```

转换后结算先按稳定 ID 锁原预扣记录和源映射，再锁目标权益与状态；禁止从状态反向锁权益。

低频幂等继续依赖 `CreditBalanceLedger` 的 `(user_id,idempotency_key)` 与 `(source_type,source_id)`。参数指纹增加来源价格微单位、源币种、FX、毛/净 Credit、档位 ID 和规则版本。状态更新、ledger 创建和来源终态必须同事务；唯一键冲突回滚后从已提交记录重放。

请求结算必须携带 `request_id`。`SubscriptionFunding`、`BillingSession`、`service/quota.go` 和异步任务都要传递该值；`TaskPrivateData` 增加 `subscription_request_id`，不能只保存 `subscription_id`。

正 delta 合并器可以在同一权益上批量开启一个事务，但不得把多请求相加后丢失身份。它必须保留入队顺序，逐条校验预扣记录、应用舍入和回写请求结果，使批量执行与相同顺序的逐条执行完全一致。

## 10. 计时权益估值

计时权益继续按币种分别取时间口径与 token 口径的较小值，但价格和履约参数来自 `TimedSubscriptionValuationGrant`，不再从查询时的 `SubscriptionPlan` 读取，也不在币种之间换算或求和。

先把所有 grant 与 `[snapshot_at, subscription.end_time)` 的重叠部分投影为价值密度。对 grant `g`：

```text
grant_duration = g.event_end_time - g.event_start_time
overlap_value(g, start, end) = floor(
  g.valuation_amount_micros
  × overlap_seconds([g.start,g.end), [start,end))
  / grant_duration
)
time_based_value[currency] = sum(
  overlap_value(g, snapshot_at, subscription.end_time)
  where g.valuation_currency = currency
)
```

再复用现有 `calcNextResetTime` / `NormalizeResetPeriod` 把未来服务时间切成额度周期。当前周期的时间价值为相同币种 grant 与该周期剩余区间的 `overlap_value` 之和，并乘以实际当前权益 `max(token_limit-token_used,0)/token_limit`；未来周期不折减。若 `token_limit <= 0`，token 口径不可用并退化为时间口径。每个币种独立计算：

```text
token_based_value[currency] = floor(
  current_cycle_time_value[currency]
  × current_remaining_credit
  / token_limit
) + sum(future_cycle_time_value[currency])

recognized_remaining_value[currency] = min(
  time_based_value[currency],
  token_based_value[currency]
)
```

`grant_credit` 是授予时履约审计快照和迁移分母；本修复不改变运行时 reset 的 Credit 规则。当前周期比例必须使用实际 `UserSubscription.token_limit/token_used`，不能使用后来套餐配置，也不能把每个 grant 当成独立可消费 token 池。

多个续期 grant 按 `event_start_time,event_end_time,id` 组成服务时间线。正常续期窗口首尾相接；若历史或并发异常导致窗口重叠，重叠秒只按最早创建的 grant 计值，后续重叠部分标记 unknown，并返回 `overlapping_grants` warning，禁止重复计值。

- 套餐改价只影响以后 grant 和以后转换。
- 订单退款但服务未撤销时，运营剩余价值不因现金状态单独变化。
- 本修复不新增“退款自动撤销计时服务”或 grant reversal schema；现有管理员失效导致的状态/`end_time` 缩短仍直接限制可交付窗口。未来若新增退款联动撤销，必须同时定义实际权益缩短与不可变估值逆向记录，不能只改金额而保留可交付权益。
- 历史无法建立可靠 grant 时间线时返回 estimated 或 unknown 披露，不得回退为“当前套餐价格 × 全部历史期限”。

## 11. 历史迁移

### 11.1 Credit

迁移在写流量停止时运行，按 `user_subscription_id` 稳定排序并允许分批提交。对每份 Credit 权益：

1. 计算最终 `available_credit` 与结算欠额；
2. 按来源唯一身份合并现有 `CreditBalanceLedger`、订单履约快照、兑换结果、转换记录和管理员调整，防止重复计数；
3. 对每个正向来源计算扣除当时可证明欠额后的净新增可用量及可恢复成本；
4. 不重放 API 日志、低频出账顺序或当前套餐改价历史；
5. 令 `K` 为有可靠 Credit 数量且成本可恢复的正向净新增 Credit，`U` 为数量可恢复但成本未知的正向净新增 Credit，`T=K+U`，`C` 为 `K` 按迁移汇率换算的成本总额，`A` 为最终可用量；
6. 若 `T > 0`，令 `R=min(A,T)`，迁移剩余成本为 `floor(C × R / T)`，全部写入 `estimated_cost_micros`，`exact_cost_micros=0`；
7. 同一比例下的成本未知剩余量为 `floor(U × R / T)`；若 `A > T`，超出 `T` 的部分也全部计入 `unknown_credit`；
8. 若无法可靠恢复正向净新增 Credit 的总分母，写入 `available_credit=A`、金额 0、`unknown_credit=A`；不得只用已知价格来源的分母放大其剩余成本；
9. 历史异币种仅 CNY/USD 使用迁移启动时冻结的汇率，其他币种对应 Credit 计入 `U`。

即使历史来源表面完整，迁移结果也统一为 `estimated`，因为最终剩余分配没有重放真实消费顺序。

### 11.2 计时权益

历史计时 grant 的恢复优先级固定为：

1. `SubscriptionOrder.FulfilledSubscriptionID + EntitlementSnapshot`；
2. 兑换记录的 `FulfillmentSnapshot`；
3. 明确的管理员获得记录；
4. 能唯一证明来源和服务窗口的历史事件；
5. 否则 unknown。

可恢复记录生成 `confidence=estimated` 的 grant。来源或窗口存在一对多歧义时不得用 `(user_id,plan_id)` 猜测；该服务窗口返回 unknown。

全新数据库若在 HTTP 服务启动前确认不存在任何 `UserSubscription`、成功套餐订单、已兑换套餐记录或管理员授予来源，迁移初始化可在同一事务直接创建 version 1 `ready` marker；这是唯一允许自动 ready 的情况。只要存在任一相关历史行，就必须运行显式 dry-run/apply，禁止把“没有 Credit 权益”误当成“没有待迁移计时权益”。

### 11.3 切换前后的写入语义

- 加表版本在 marker 非 ready 时继续执行生产基线的 Credit 数量写入，避免发布 schema 阶段中断业务；同时必须为新订单、兑换、转换和计时授予捕获结构化整数来源快照。
- marker 非 ready 时不得创建或局部维护 `CreditValuationState`，避免同时存在两套“部分可信”状态；分析继续旧口径并返回 `migration_not_ready` warning。
- 非 ready 时发生的 Credit 来源即使快照完整，仍在正式迁移中标记 estimated，因为该时段的高频消费顺序没有进入估值状态。已经由加表版本前向写入且来源、窗口完整的计时 grant 可以保留 exact；回填逻辑只为缺失历史补 estimated/unknown，不覆盖它。
- marker ready 后，任何 Credit 数量写都必须通过估值深模块；来源事实或状态缺失时整笔拒绝。禁止退回旧数量旁路。
- marker suspended 时 HTTP 写流量必须由部署层保持关闭；进程只允许只读验证与维护命令，不能将 suspended 当作正常非 ready 模式继续写入。
- ready 后若仍收到缺少 `subscription_request_id` 的历史异步任务回调，只能走以持久化 Task 主键生成确定性身份的 legacy task 接缝：追加用量按当前移动平均出账；退款新形成的可用 Credit 全部标记 unknown。该接缝仍由深模块同时更新数量和状态，禁止直接 delta 写 `token_used`。

### 11.4 可重跑、校验与门禁

- 正式 apply 前必须确认没有 `status=consumed` 的非终态预扣记录、仍会回调结算的订阅资金异步任务或旧进程写会话；否则迁移拒绝 ready。发布流程应等待其终态，而不是迁移中猜测在途成本。
- `dry-run` 全程只读，输出行数、币种、estimated/unknown 总量、歧义原因分布和 checksum。
- 正式运行使用迁移 marker；同版本 `ready` 再次执行为无操作。
- `running/failed` 只能在无业务写入时从最后完成的稳定主键继续；每批 upsert 只允许覆盖同一迁移版本且 `state_version=0` 的迁移行。
- `ready` 后一旦发生前向写入，首版迁移不得覆盖状态；后续重建需要新版本和维护窗口。
- 完成前检查每份 Credit 权益都有状态、数量一致、币种一致、无负数、unknown 不越界、无重复 grant、无缺失来源键。
- `--repair-missing-as-unknown` 只允许在停写维护窗口、显式新 migration version 下运行；修复后必须重新 `--verify` 才能 ready。

## 12. 分析接口与 UI

### 12.1 后端显式分流

`adminBuildPaidRowsFromSubscriptions` 先按显式 `entitlement_type` 分流：

- `timed`：读取不可变 grant；
- `credit_balance`：不检查余额套餐正价格和 `end_time`，读取 `CreditValuationState`；
- 其他类型：跳过并返回 warning。

Credit 行固定为：

```text
token_based_value = exact_cost_micros + estimated_cost_micros
time_based_value = null
recognized_remaining_value = token_based_value
valuation_basis = credit_moving_weighted_average
```

余额为 0 或欠额时保留明细状态但价值为 0，不进入 `active_paid_subscription_count`。正可用量且置信度为 exact、estimated、mixed 或 unknown 的 Credit 计入一条有效有价权益；unknown 不增加金额，但增加披露计数。

Credit 不再调用 `(user_id,plan_id)` 订单关联。辅助订单只可沿 ledger 的 `source_type/source_id` 追溯。由于余额已经混合，来源 breakdown 统一返回：

```text
source = credit_balance_pool
source_attribution = moving_weighted_pool
```

不得按某个充值档位伪造剩余来源。`plan_ids` 过滤 Credit 时只匹配全局 Credit 计划 ID。邀请付费接口不得因为复用 paid row builder 而纳入 Credit。

### 12.2 DTO 扩展

`AdminAnalyticsMoneyAmount` 和 `AdminAnalyticsMoneyBreakdown` 增加精确 `amount_micros` 字符串。Summary 新增：

- `exact_remaining_value_by_currency`
- `estimated_remaining_value_by_currency`
- `unknown_cost_credit`
- `unknown_timed_subscription_count`
- `credit_valuation_state_missing_count`

用户、套餐组和权益明细增加对应 exact/estimated/unknown 字段。权益明细另增加：

- `entitlement_type`
- `valuation_confidence`：exact/estimated/mixed/unknown
- `valuation_state_version`
- `valuation_updated_at`
- `snapshot_semantics`：snapshot/current_only

`time_based_value` 改为可空。旧字段继续返回，以免一次发布破坏前端；所有金额类排序改用微单位整数。

计时权益明细增加 `recognized_remaining_value_by_currency`、`token_based_value_by_currency`、`time_based_value_by_currency`。旧 singular MoneyAmount 字段仅在恰有一个币种时返回；跨多个 grant 币种时为 null。计时 source breakdown 按 `TimedSubscriptionValuationGrant.source_type` 聚合；一条合并权益含多个来源时，权益行使用 `source_attribution=mixed_grants`，不得把最后一次来源套到整条权益。

### 12.3 `snapshot_at`

Summary 继续返回数据库时间供同一轮其他接口使用。Credit 只读取当前物化状态：

- `state.updated_at <= snapshot_at`：`snapshot_semantics=snapshot`；
- `state.updated_at > snapshot_at`：返回最新状态、`snapshot_semantics=current_only`，并在 panel warnings 增加 `section=credit_valuation, reason=current_only`；
- 不允许把最新状态倒填成目标历史时点。

前端显示“Credit 为当前值，页面加载期间发生余额变动”的非阻断提示，并提供刷新操作。

### 12.4 管理员合同

`PUT /api/subscription/admin/credit-balance-plan` 增加 `currency`。首次配置时必须明确选择 CNY 或 USD；存在任一 Credit 权益、估值状态或估值 ledger 后不得通过普通接口修改。该币种只控制 Credit 运营估值，不改变订单原币种。

`POST /api/subscription/admin/users/:id/credit-balance/adjustments` 扩展为：

```json
{
  "operation": "increase",
  "amount": 800,
  "plan_id": 123,
  "idempotency_key": "...",
  "reason": "售后补偿"
}
```

- increase 必须提供合格档位 `plan_id`；
- decrease 不需要且不得提供 `plan_id`；
- 参数指纹必须包含 `plan_id`；
- 响应增加毛/净估值金额、币种、置信度、FX 和状态版本。

管理员计时授予请求增加 `idempotency_key` 和必填 `reason`。`TimedSubscriptionValuationGrant.source_key` 使用该幂等键，重放不得再次续期；旧客户端缺字段时明确拒绝，不自动生成不可追溯来源。

`AdminCreditBalancePanel` 在 increase 时加载并要求选择充值档位，展示档位标价、档位 Credit 和按本次数量计算的运营价值；decrease 时隐藏档位。Credit 计划设置显示估值币种及冻结条件；计时授予表单收集原因并生成可重试的幂等键。所有新文案补齐 en/zh/fr/ru/ja/vi。

分析 UI 将 Credit 的 token/time 口径显示为“移动加权平均 / 不适用”，并分开展示确值、估值和成本未知 Credit。跨币种计时权益按币种拆分；不得把 headline 标为“应退款”“负债”或“实收余额”。
## 13. 失败语义与可观测性

稳定错误码至少包括：

- `credit_valuation_state_mismatch`
- `credit_valuation_state_missing`
- `credit_valuation_overflow`
- `credit_valuation_unsupported_currency`
- `credit_valuation_invalid_fx`
- `credit_valuation_plan_required`
- `credit_valuation_plan_ineligible`
- `credit_valuation_idempotency_mismatch`
- `credit_valuation_migration_not_ready`

结构化日志记录 mutation type、subscription ID、state version、错误码和耗时，不记录用户名、密钥或支付载荷。新增诊断统计：状态缺失、状态不一致、unsupported FX、unknown Credit、迁移歧义、请求结算重放和合并器 batch 大小。

分析面板对 estimated、unknown、current_only 使用 warnings；写路径的数量或状态不变量错误不能降级为 warning 后继续提交。

## 14. 测试矩阵

实现至少覆盖：

1. `40 CNY / 1000`，消费 200，永久 Credit 剩 800 ⇒ 32 CNY、有效权益数 1；
2. 零价格 Credit 容器仍由来源状态计入；
3. 两个不同价格入账后的移动加权平均；
4. 先消费再低价入账与先入账再消费结果不同且各自正确；
5. 入账先抵扣欠额，只有净 Credit/净成本进入状态；
6. 兑换来源；
7. 转换确认时价格快照，后续改价不回写；
8. 管理员任意正 Credit 按所选档位比例估值；缺少、停用或不合格档位拒绝；
9. 计时管理员授予冻结价格；
10. 预扣后交错入账，再少结算时恢复原请求成本；
11. 追加结算按追加时平均值，最终全退款消除全部请求舍入余数；
12. 请求 unknown 快照被其他欠额吸收时记录 `absorbed_restore_unknown_credit`；
13. 转换期间在途请求少结算时按转换单位价值恢复；
14. 最终结算超过可用量形成欠额并清空剩余成本；
15. 订单退款、拒付、管理员减少均按操作前平均值且不低于零；
16. Credit 的 available/exhausted/debt 三种生命周期；
17. 历史完整来源迁移仍标 estimated；
18. 历史无可靠分母全部 unknown；
19. 历史 CNY/USD 使用迁移时 FX，之后改汇率不变化；
20. 新 EUR 等不支持币种原子拒绝；
21. 同币种不依赖 USD 汇率；
22. 计时和 Credit 套餐改价不回写既有快照；
23. 并发 grant、预扣、结算、转换在任一合法串行顺序下保持数量一致；
24. 低频和请求级幂等重放不重复变更；
25. `MaxInt64` 边界、乘法溢出和向下取整；
26. unknown 与 exact/estimated 混合后按比例消费；
27. `snapshot_at` 早于状态更新时间时返回 current_only；
28. Credit 不进入邀请付费或邀请奖励；
29. SQLite、MySQL 5.7、PostgreSQL 9.6 的 schema、唯一约束、锁和迁移实测；
30. 旧响应字段兼容，新 `amount_micros` 驱动精确展示；
31. 冻结生产夹具在 summary/users/subscriptions/plans/sources 五个 endpoint 结果一致；
32. 迁移存在非终态预扣或活跃旧异步任务时拒绝 ready；
33. ready 后历史 Task 缺 request ID 时用持久化 Task 身份幂等处理，退款量标记 unknown。

## 15. 发布、回滚与验收门禁

### 15.1 发布阶段

1. **加表版本**：只增加 schema、快照字段、dry-run 和迁移命令；门禁非 ready 时继续旧分析，不启用强制状态写。此阶段可镜像回滚。
2. **迁移预演**：在线只读 dry-run，记录 checksum、estimated/unknown 数量、unsupported currency 和歧义。
3. **维护切换**：停止写流量，创建一致数据库备份并校验 SHA-256；使用不可变镜像 digest 和服务器本地 `flock`/`trap` 运行正式迁移。
4. **门禁检查**：确认 marker=ready、每份 Credit 权益一行状态、数量/币种/非负不变量、跨库迁移测试和冻结夹具通过。
5. **强制双写**：重启同一 digest，使进程启动时读到 ready；开启新分析和所有写路径原子估值。
6. **观察**：检查健康、业务探针、状态 mismatch、unknown 增长、结算延迟和数据库写负载。

### 15.2 回滚

- ready 前可回滚旧镜像，因为旧镜像只忽略附加表。
- ready 后但开放写流量前，可回滚并保留附加表，随后重新迁移。
- 强制双写开放流量后，旧镜像会修改 Credit 数量却不修改估值状态，**禁止 image-only rollback**。优先向前修复；若必须回滚，先停止写流量，通过维护命令把 marker 原子置为 `suspended` 并记录原因，再回滚镜像；再次切换前必须从新备份执行新版本重建与验证。
- schema 为附加式，常规回滚不删表、不删快照、不覆盖账本。

### 15.3 最终验收

冻结生产夹具必须满足：

```text
active scope credit rows = 1
token/recognized remaining value = 32,000,000 micros CNY
active_paid_subscription_count = 1
estimated value = 0
unknown credit = 0
end_time = 0 不影响结果
```

同时必须通过后端全套测试、前端全套测试、生产构建、六语言 i18n 校验、`git diff --check`，并在真实 MySQL 5.7、PostgreSQL 9.6、SQLite 上执行迁移验收。具体实施顺序见 `docs/superpowers/plans/2026-08-02-credit-operational-remaining-value-plan.md`。
