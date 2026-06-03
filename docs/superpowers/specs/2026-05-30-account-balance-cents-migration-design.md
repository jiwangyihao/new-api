# 账户余额分制迁移设计

## 背景

当前项目已经废弃「用户账户余额用于模型调用消费」的产品语义。用户充值余额的唯一作用是购买订阅套餐，订阅套餐内的用量由 `UserSubscription` 的 token 字段和套餐配置决定。

但历史代码仍复用 `quota` 字段和 `common.QuotaPerUnit`：

- 充值到账时把 CNY 金额放大为 `金额 × QuotaPerUnit` 后写入 `users.quota`。
- 余额购买订阅时按 `plan.price_amount × QuotaPerUnit` 扣减。
- 前端展示账户余额时按 `quota / quotaPerUnit` 换算。
- 钱包兑换码和 Kyren 充值档位也暴露放大后的 `quota` 值。

这导致管理员配置 Kyren 充值档位时必须理解 `500000` 这类历史倍率，与当前「账户余额只买订阅」的业务模型不一致。

## 目标

采用短停机方案，将账户余额从「历史 quota 放大单位」迁移为「CNY 分」。迁移后：

```text
users.quota = 4000 表示账户余额 ¥40.00
subscription_plans.price_amount = 40.00 表示购买套餐扣 4000
Kyren 充值档位到账 ¥40.00 时内部余额值为 4000
```

本设计只移除账户余额链路中的 `QuotaPerUnit`。模型调用计费、日志统计、API Key 额度、渠道消耗、订阅套餐内 token 用量仍保持现有语义。

## 非目标

本次不处理以下内容：

- 不重命名数据库字段 `users.quota`。
- 不删除 `common.QuotaPerUnit` 常量。
- 不修改 `logs.quota`、`tokens.remain_quota`、`channels.used_quota` 等模型调用计费用量字段。
- 不修改 `user_subscriptions.token_limit`、`token_used`、`monthly_token_limit` 等订阅套餐内 token 字段。
- 不重新设计充值支付渠道的收款金额计算规则。
- 不改变订阅套餐价格字段 `price_amount` 的 CNY 语义。

## 当前数据流

### Kyren 充值

```text
KyrenTopUpProducts[].quota（放大后的历史 quota）
  → 创建 TopUp.Amount
  → Kyren order.paid
  → users.quota += snapshot.quota
```

### 账户余额购买订阅

```text
plan.price_amount
  → price_amount × common.QuotaPerUnit
  → DeductUserAccountBalanceTx
  → users.quota -= amount
```

### 前端展示

```text
users.quota
  → quota / quotaPerUnit
  → 展示为 CNY 账户余额
```

## 目标数据流

### Kyren 充值

```text
KyrenTopUpProducts[].quota（余额分）
  → 创建 TopUp.Amount
  → Kyren order.paid
  → users.quota += snapshot.quota
```

### 账户余额购买订阅

```text
plan.price_amount
  → round(price_amount × 100)
  → DeductUserAccountBalanceTx
  → users.quota -= amount_cents
```

### 前端展示

```text
users.quota
  → quota / 100
  → 展示为 CNY 账户余额
```

## 数据单位约定

迁移后，账户余额相关字段统一使用 CNY 分：

| 字段或配置 | 新语义 | 示例 |
|---|---|---:|
| `users.quota` | 账户余额，单位：分 | `4000` 表示 ¥40.00 |
| `users.aff_quota` | 可转入账户余额的邀请奖励余额，单位：分 | `1000` 表示 ¥10.00 |
| `users.aff_history` | 邀请历史奖励余额，单位：分 | `5000` 表示 ¥50.00 |
| `redemptions.quota`（钱包类型） | 钱包兑换码到账余额，单位：分 | `4000` 表示 ¥40.00 |
| `KyrenTopUpProducts[].quota` | Kyren 充值档位到账余额，单位：分 | `4000` 表示 ¥40.00 |
| `CreemProducts[].quota` | Creem 充值产品到账余额，单位：分 | `4000` 表示 ¥40.00 |
| `checkins.quota_awarded` | 单次签到奖励到账余额，单位：分 | `20` 表示 ¥0.20 |

`top_ups.amount` 不做历史成功记录批量迁移。原因是不同支付渠道历史上对该字段的语义不一致：Stripe 订单的入账逻辑按 `money × QuotaPerUnit` 计算，Epay / Waffo / Waffo Pancake 按 `amount × QuotaPerUnit` 计算，Creem / Kyren 已更接近「到账额度」。为了避免把历史审计数据错改成不可解释的混合单位，迁移只改变新代码的入账单位。历史成功充值记录按原样保留；迁移后新建订单的 `top_ups.amount` 使用余额分，并必须写入不可变订单级单位标记 `top_ups.amount_unit = account_balance_cents`（或等价字段）。迁移前成功历史订单该字段保持空值。

迁移前仍处于 `pending` 的充值订单不能跨单位继续履约。短停机迁移事务必须把所有历史 pending `top_ups` 标记为 `expired`，要求用户重新下单；已完成或已失败订单保持原样用于审计。这样 webhook / 补单在迁移后可以统一按新订单的余额分处理。Kyren pending top-up 也采用同一策略：迁移时过期本地 pending 订单，迁移后到达的旧 webhook 不得入账。

账单历史必须有稳定的新旧单位判定契约。迁移时额外写入只读迁移时间标记：

```text
Option.Key   = AccountBalanceCentsMigratedAt
Option.Value = <unix_timestamp_seconds>
```

充值记录列表 API 必须由服务端返回明确展示字段，前端不得自行猜测 `top_ups.amount` 单位。推荐字段为 `credited_balance_cents`、`credited_balance_display`、`amount_unit` 或 `is_account_balance_cents`。服务端规则：新分制订单优先且只信任订单级 `amount_unit = account_balance_cents`；不得仅凭 `CreateTime >= AccountBalanceCentsMigratedAt` 推断单位，因为预迁移版本、迁移失败重试、蓝绿发布或时钟偏差都可能破坏时间边界。无单位标记的历史成功订单按旧渠道语义换算出展示用 CNY 金额或标记为 legacy；同一支付渠道迁移前后各一条成功订单时，前端也能稳定展示到账余额。`payment_method` / `payment_provider` 只能作为辅助信息，不能作为唯一单位判定依据。普通 Epay / Stripe / Waffo / Waffo Pancake 历史记录可按 `amount × 100` 作为展示用余额分；迁移前已按产品额度入账的 Kyren / Creem 历史记录必须结合快照或 provider-specific 规则生成展示字段，无法可靠换算时返回 legacy 标记和原始审计值，不得伪装成分制金额。

继续使用整数存储，避免浮点数表示金额。

## 迁移策略

采用短停机原地迁移。服务启动时检测迁移状态，未完成则执行一次性迁移，成功后才允许 HTTP 服务和后台任务开放流量。

迁移状态使用两个 Option 标记，避免「数据已迁移但缓存未清理」后无法安全重试：

```text
Option.Key   = AccountBalanceCentsDataMigrated
Option.Value = true

Option.Key   = AccountBalanceCentsMigrated
Option.Value = true
```

`AccountBalanceCentsDataMigrated` 表示数据库金额字段已经完成旧 quota 到余额分的除法迁移；`AccountBalanceCentsMigrated` 是最终完成标记，只有数据迁移、运行时配置同步和用户缓存清理全部成功后才能写入。

迁移输入倍率必须来自数据库 Option 表中最终生效的 `QuotaPerUnit`，或在 `InitOptionMap` / `loadOptionsFromDatabase` 完成后读取已加载的 `common.QuotaPerUnit`。不得在 `model.InitDB` / `migrateDB` 阶段使用默认倍率执行本迁移。为避免重复除法，数据字段更新必须与 `AccountBalanceCentsDataMigrated = true` 在同一事务内提交；重启时若发现 `AccountBalanceCentsDataMigrated = true` 但 `AccountBalanceCentsMigrated != true`，只能重试运行时同步和缓存清理，严禁再次执行除法迁移。

### 短停机旧实例 drain 顺序

迁移前必须先停止入口流量和异步任务触发源，但保留每个旧实例进程运行。每个旧实例进入 drain 状态后，除本机 loopback drain 请求外，不得再接收会写 `BatchUpdateTypeUserQuota` 的新请求；队列消费者、定时任务和异步触发源必须停止。所有旧实例必须在本机完成 `BatchUpdateTypeUserQuota` flush / drain，并确认每个旧实例自己的 `BatchUpdatePendingSnapshot().ByType[BatchUpdateTypeUserQuota] == 0` 后，才能停止旧服务和所有写库进程、备份数据库并启动新版本迁移。不得用迁移进程或新进程自己的空 batch snapshot 代替旧实例逐节点检查。

可执行顺序如下：

1. 停止入口流量和异步任务触发源，不再接收新充值、兑换码、签到、注册、邀请、模型调用请求。
2. 将每个旧实例切到本地 drain 状态：公网入口已断开，队列消费者、定时任务和异步触发源已停止，除本机 loopback drain 请求外不再有新请求进入会调用 `addNewRecord(BatchUpdateTypeUserQuota, delta)` 的路径。
3. 保持每个旧实例进程运行，逐个在对应机器本地执行：

   ```bash
   curl -fsS -X POST http://127.0.0.1:<port>/debug/loadtest/runtime/batch-update/user-quota/drain
   ```

4. 对每个旧实例继续执行：

   ```bash
   curl -fsS http://127.0.0.1:<port>/debug/loadtest/runtime
   ```

   确认响应中的 `batch_update` reason 里 `BatchUpdateTypeUserQuota` 对应 pending 为 0。
5. 任一旧实例 drain 返回非 2xx，或 pending 不为 0 时，停止迁移，修复该实例写库问题后重试 drain；不得启动新版本迁移。
6. 所有旧实例确认 pending = 0 后，停止所有旧服务和写库进程，备份数据库。
7. 启动包含任务 3 迁移接入的新版本。新版本在 HTTP 服务和后台任务启动前执行 `EnsureAccountBalanceCentsMigration()`。

如果线上旧版本没有 drain 入口，应先发布一个不接入启动迁移的预迁移版本，仅提供本地 loopback drain 入口；完成逐实例 drain 和数据库备份后，才发布包含启动迁移接入的迁移版本。任一旧实例 drain 失败或 pending 不为 0 时，迁移必须停止并修复该实例写库问题后重试，不得带着未落库的余额批量更新执行除法迁移。drain 实现还必须对极少量 in-flight 余额 delta 安全：flush 时在锁内原子 swap 当前 batch map，flush 期间新增 delta 写入新的 map；失败时只把尚未落库的 snapshot 项合并回当前 map，不能删除或抵消 flush 期间新增的 delta。

### 换算公式

```text
新余额分 = round(旧历史 quota × 100 / QuotaPerUnit)
```

示例：

| 旧值 | 旧含义 | 新值 |
|---:|---:|---:|
| `20000000` | ¥40.00 | `4000` |
| `5000000` | ¥10.00 | `1000` |
| `19950000` | ¥39.90 | `3990` |

### 迁移范围

必须迁移：

- `users.quota`
- `users.aff_quota`
- `users.aff_history`
- `redemptions.quota`，仅限 `type = wallet` 或空类型归一化为钱包语义的记录
- Option `KyrenTopUpProducts` 中每个产品的 `quota`
- Option `CreemProducts` 中每个产品的 `quota` / `Quota`
- `checkins.quota_awarded` 历史记录，如果保留签到奖励金额展示
- 账户余额相关运行时配置值：`QuotaForNewUser`、`QuotaForInviter`、`QuotaForInvitee`，如果它们当前按历史 quota 单位配置，则迁移为余额分
- 签到奖励配置：`checkin_setting.min_quota`、`checkin_setting.max_quota`，如果签到功能继续作为账户余额奖励保留，则迁移为余额分
- 所有迁移前 `pending` 的 `top_ups.status`，统一标记为 `expired`，不迁移其 `amount`

不迁移：

- `logs.quota`
- `tokens.remain_quota`
- `tokens.used_quota`
- `channels.used_quota`
- `abilities.quota`
- `user_subscriptions.token_limit`
- `user_subscriptions.token_used`
- `subscription_plans.monthly_token_limit`
- `top_ups.amount` 的历史记录
- `top_ups.money`

## 数据库兼容性

项目必须继续支持 SQLite、MySQL 和 PostgreSQL。迁移实现应遵守以下规则：

1. 使用 GORM 查询和更新，避免数据库专属 SQL。
2. 不依赖数据库整数除法、四舍五入函数或 JSON 函数。
3. 迁移逻辑在 Go 中逐行计算新值。
4. JSON 配置使用 `common.UnmarshalJsonStr` 和 `common.Marshal` 处理。迁移 `KyrenTopUpProducts` 和 `CreemProducts` 后，必须同时更新数据库 Option、`common.OptionMap` 以及对应的运行时设置变量。
5. 数据字段写入、pending 订单过期和 `AccountBalanceCentsDataMigrated = true` 放在一个事务中；如果该阶段失败，事务回滚，服务启动失败。
6. `Option` 行更新使用 GORM，并保持现有 `common.OptionMap` 与运行时设置同步。最终 `AccountBalanceCentsMigrated = true` 必须在用户缓存清理成功后写入；如果缓存清理失败，最终标记不得写入，服务启动失败，下一次启动只重试缓存清理和最终标记写入。

SQLite 的 `ALTER COLUMN` 能力较弱，因此本设计不要求变更列类型。`users.quota` 等字段继续保留 `int`，只改变业务单位。

## 后端设计

### 账户余额 helper

新增或改造账户余额 helper，使业务代码不再直接使用 `QuotaPerUnit`：

```go
func AccountBalanceCentsFromCNY(amount decimal.Decimal) (int, error)
func AccountBalanceCNYFromCents(cents int) decimal.Decimal
func DeductUserAccountBalanceTx(tx *gorm.DB, userId int, cents int) error
func IncreaseUserAccountBalanceTx(tx *gorm.DB, userId int, cents int) error
```

迁移完成后，所有写入 `users.quota` 的账户余额路径必须通过账户余额 helper 或明确等价的 Tx helper。模型调用、异步任务、legacy wallet funding 等用量路径不得再直接写 `users.quota`。

金额转分规则：

- 金额必须大于 0。
- 使用 decimal 计算，按 2 位小数四舍五入到分。
- 超出 `math.MaxInt` 时返回错误。

### `users.quota` 写入收口

实现计划必须先对 `IncreaseUserQuota`、`DecreaseUserQuota`、`DeltaUpdateUserQuota`、`PostConsumeQuota`、`WalletFunding`、`taskAdjustFunding`、Midjourney / video 等异步任务退款路径做静态扫描并分类：

- 账户余额路径：充值、兑换码、注册赠送、邀请奖励、签到奖励、账户余额购买订阅、管理端余额调整。这些路径改用余额分 helper。
- 非账户余额用量路径：relay legacy wallet funding、异步任务扣费或退款、模型调用后结算、token key quota 等。这些路径不得继续写 `users.quota`；应删除旧钱包 fallback、迁到订阅/token 用量字段，或在实现计划中明确停用。
- 迁移前已存在且仍可能回调的异步任务，如果无法安全换算，必须按迁移前任务标记做兼容结算或在迁移时阻断旧钱包资金来源；不得把模型 quota 单位直接加减到账户余额分。

验收时必须包含静态扫描：除账户余额 helper 和明确列出的账户余额入口外，仓库中不得存在直接更新 `users.quota` 的模型用量路径。

### 余额购买订阅

`subscriptionBalancePayAmount` 从：

```go
price × common.QuotaPerUnit
```

改为：

```go
round(price × 100)
```

`createBalanceSubscriptionOrderTx` 继续使用事务和行锁，扣减的是余额分。

### 注册、邀请与邀请额度转余额

注册赠送、邀请人奖励和被邀请人奖励继续复用现有配置键，但迁移后这些配置值的单位改为余额分：

```text
QuotaForNewUser = 1000 表示新用户注册赠送 ¥10.00
QuotaForInviter = 500 表示邀请人获得 ¥5.00 可转余额奖励
QuotaForInvitee = 500 表示被邀请人获得 ¥5.00 账户余额
```

`User.Insert`、`User.InsertWithTx`、`FinalizeCreationTx`、`FinalizeOAuthUserCreation`、`inviteUser` 和 `inviteUserTx` 写入 `users.quota`、`users.aff_quota`、`users.aff_history` 时，必须直接写入配置中的余额分，不再把这些配置视为历史 quota。

`TransferAffQuotaToQuota` 继续在一个事务中扣减 `aff_quota` 并增加 `quota`，但请求值、最小转移值和日志展示都按余额分处理。旧的 `common.QuotaPerUnit` 最小门槛必须移除；建议最低转移单位为 1 分，或由前端限制为 0.01 CNY。

### 签到奖励

签到功能如果保留为账户余额奖励，`checkin_setting.min_quota` 和 `checkin_setting.max_quota` 迁移后也使用余额分。`model.UserCheckin` 写入 `users.quota` 和 `checkins.quota_awarded` 时直接使用余额分，日志展示为账户余额金额，不再使用 `logger.LogQuota` 的历史倍率展示。

如果实现阶段决定产品上不再支持签到奖励账户余额，则必须在实现计划中明确停用签到入账路径和前端签到金额展示；不得保留旧倍率入账。

### Kyren 充值档位

`KyrenTopUpProduct.Quota` 保留字段名，但语义改为「到账余额分」。

后端校验：

- `quota` 必须大于 0。
- `amount` 仍为 Kyren 实际收款 CNY 金额字符串。
- `currency` 首期仍仅支持 `CNY`。

Kyren webhook 入账逻辑继续使用快照：

```go
users.quota += snapshot.quota
```

但 `snapshot.quota` 已是余额分。

### 普通充值渠道

Epay、Stripe、Waffo、Waffo Pancake、Creem 等现有充值渠道需要统一新订单的到账单位。实现时必须区分三类值：

| 概念 | 含义 | 存储或传输位置 |
|---|---|---|
| 用户输入金额 | 用户想获得的账户余额金额，单位：CNY 元；token 展示模式下也必须先归一化为等价 CNY 元 | 请求 `amount` 或产品配置 |
| 渠道收款金额 | 支付网关实际收取的金额，继续按各渠道现有 `Price`、`UnitPrice`、折扣、汇率规则计算 | `top_ups.money`、支付网关订单 |
| 到账余额分 | 用户支付成功后增加的账户余额，单位：分 | `top_ups.amount`、`users.quota` |

统一公式：

```text
到账余额分 = round(用户获得的账户余额金额 × 100)
```

渠道收款金额计算规则保持现有逻辑；只移除「到账余额」环节的 `QuotaPerUnit` 放大。具体落地规则：新建 `TopUp.Amount` 时直接存余额分；webhook / 补单成功时直接把 `TopUp.Amount` 或渠道产品快照中的余额分加到 `users.quota`，不再乘 `QuotaPerUnit`。

Epay、Waffo 和 Waffo Pancake 当前会在 token 展示模式下把请求 `amount` 除以 `QuotaPerUnit` 后再落库。迁移后账户余额链路不得再依赖 `QuotaDisplayTypeTokens` 和 `QuotaPerUnit` 做到账换算；若前端仍允许 token 展示，充值表单必须先把用户输入转换为 CNY 元，再由后端统一转成分。

Stripe 当前成功入账按 `TopUp.Money × QuotaPerUnit` 计算。迁移后应在创建订单时把预计到账余额换算为分写入 `TopUp.Amount`，成功回调按 `TopUp.Amount` 入账。这样可以避免 webhook 阶段再从支付金额反推余额。

Creem 当前使用产品配置的 `Quota` 作为到账额度。迁移后 Creem 产品配置中的到账额度也必须改为余额分，webhook 继续按 `TopUp.Amount` 入账。

### 管理端补单与余额调整

`controller.AdminCompleteTopUp -> model.ManualCompleteTopUp` 必须显式改造：

- pending 订单补单成功时，只把 `TopUp.Amount` 作为余额分加到 `users.quota`。
- 不再按支付网关区分 `Money × QuotaPerUnit` 或 `Amount × QuotaPerUnit`。
- 已成功订单仍保持幂等，不重复入账。
- 日志展示使用账户余额分格式化，不再使用 `logger.FormatQuota` / `logger.LogQuota` 的历史倍率展示。

管理员手动调整用户账户余额的接口和前端输入也应按 CNY 元展示、按余额分提交。服务端保存的是分，不得再通过 `parseQuotaFromDollars` 或 `QuotaPerUnit` 换算。

### 钱包兑换码

钱包类型兑换码迁移后的 API 边界统一使用余额分：前端或调用方输入 CNY 元并转换为分后提交，后端只校验 `redemptions.quota` 为正整数分并保存 / 更新，不再把请求值乘以 `QuotaPerUnit` 或再乘 100。

```text
redemptions.quota = amount_cents
```

兑换时继续把 `redemptions.quota` 加到 `users.quota`，但语义已是余额分。

### 迁移执行入口

在 Option 已加载完成后执行：

```go
func EnsureAccountBalanceCentsMigration() error
```

执行条件：

- `AccountBalanceCentsMigrated != true` 时进入迁移流程。
- `AccountBalanceCentsDataMigrated != true` 时执行数据库金额字段迁移、pending 订单过期和数据阶段标记写入；`AccountBalanceCentsDataMigrated = true` 且最终标记缺失时，不得重复除法迁移，只能重试运行时同步、用户缓存清理和最终标记写入。
- 迁移入口必须在 `InitOptionMap` / `loadOptionsFromDatabase` 和 Redis 初始化完成后运行，并且在 HTTP 服务、定时任务、异步任务消费者启动前完成；或在事务内显式读取 Option 表中的 `QuotaPerUnit` 作为唯一倍率来源。
- `QuotaPerUnit <= 0` 时拒绝迁移并返回错误。
- 数据迁移阶段成功后写入 `AccountBalanceCentsDataMigrated = true`。
- 成功提交数据库迁移后必须清理或重建用户缓存，避免 Redis / 进程缓存继续返回迁移前的历史倍率余额。当前 `model.GetUserCache` 会缓存 `UserBase.Quota`，`UserBase.WriteContext` 会把缓存值写入请求上下文，因此迁移后至少要让所有 `user:{id}` 缓存失效；若缓存清理失败，最终 `AccountBalanceCentsMigrated` 标记不得写入，迁移命令必须以失败状态退出并要求运维重试或手工清理，不能在仍可能读到旧倍率缓存时开放流量。Redis 未启用时该步骤应记录跳过原因并依赖后续 DB 读取。
- 用户缓存清理成功、运行时配置同步成功并写入 `AccountBalanceCentsMigratedAt` 后，才能写入最终标记 `AccountBalanceCentsMigrated = true`。

迁移日志应记录：

- 使用的 `QuotaPerUnit` 及其来源（Option 表或已加载运行时设置）。
- 更新的用户数、钱包兑换码数、Kyren 档位数、Creem 产品数、签到记录数和运行时配置项数量。
- 被标记为 `expired` 的历史 pending 充值订单数。
- 签到配置是否迁移，以及是否出现正数旧值被舍入为 0 分的情况。
- 历史已完成 `top_ups.amount` 明确跳过，不记录为已更新订单数。
- 迁移是否成功。
- 数据阶段标记、最终标记、迁移时间标记、用户余额缓存清理方式、清理数量和失败降级情况。
## 前端设计

### 账户余额展示与输入

`subscription-balance.ts` 中的换算从：

```ts
quota / quotaPerUnit
```

改为：

```ts
balanceCents / 100
```

建议重命名 helper，并要求所有账户余额展示复用同一套 helper：

```ts
accountBalanceCentsToCnyAmount(balanceCents)
formatAccountBalanceForPlanPurchase(balanceCents)
accountBalanceCnyToCents(amountCny)
```

余额是否足够的判断改为：

```ts
balanceCents >= Math.round(priceAmount * 100)
```

必须改造的前端账户余额入口：

- `wallet-stats-card.tsx`：`users.quota` 显示为账户余额 CNY 金额。
- `profile-header.tsx`：`Current Balance` 使用余额分格式，不再使用 `formatQuota(profile.quota)`。
- `profile/components/checkin-calendar-card.tsx`：用户侧签到奖励展示也属于账户余额链路。`quota_awarded`、本月奖励和累计奖励按余额分格式化为 CNY 金额；签到成功 toast、日历 tooltip 和说明文案使用账户余额奖励语义。如果停用签到余额奖励，用户侧展示也必须同步隐藏或标注停用。
- `usage-logs/components/dialogs/user-info-dialog.tsx`：`quota`、`aff_quota`、`aff_history_quota` 使用余额分格式；`used_quota`、日志用量和 token 用量仍保留原有用量格式。
- `users/components/users-columns.tsx`：账户余额单独展示；不得再把 `users.quota` 与 `used_quota` 相加计算使用率。`used_quota` 只能作为历史模型用量字段展示。
- `users/components/users-mutate-drawer.tsx`、`users/components/user-quota-dialog.tsx`：管理员手动设置或调整账户余额时，输入单位为 CNY 元，提交前转换为余额分；禁止继续通过 `parseQuotaFromDollars`、`formatQuota` 或 `quotaPerUnit` 换算账户余额。
- `wallet/components/dialogs/transfer-dialog.tsx`、`wallet/hooks/use-affiliate.ts`、`wallet/constants.ts`：邀请奖励转账户余额的输入、最小值和步长按 CNY 元或分处理，移除 `QUOTA_PER_DOLLAR = 500000` 在账户余额链路中的作用。
- `wallet/components/recharge-form-card.tsx`：用户侧 Kyren 充值档位显示「到账余额 ¥xx.xx」，不显示 raw `quota`、raw cents 或 `{{quota}} quota`。
- `wallet/components/dialogs/billing-history-dialog.tsx`：迁移后的新订单 `record.amount` 按余额分展示为到账余额。历史 `top_ups.amount` 不批量迁移，账单历史必须通过服务端提供的展示字段或按支付渠道/迁移标记区分；不得把所有历史记录统一按分误读。

- `system-settings/general/quota-settings-section.tsx`：`QuotaForNewUser`、`QuotaForInviter`、`QuotaForInvitee` 属于账户余额奖励配置，表单展示和输入使用 CNY 元，保存时转为余额分；`PreConsumedQuota`、`QuotaRemindThreshold` 等模型调用或 token 用量配置继续保留旧语义，不得误改。
- `wallet/components/recharge-form-card.tsx`、`wallet/components/dialogs/payment-confirm-dialog.tsx`、`wallet/index.tsx`：普通 Epay / Stripe / Waffo / Waffo Pancake 充值的预设金额、自定义金额和支付确认弹窗都以「用户获得的账户余额 CNY 元」为输入语义；提交请求仍传 CNY 元，后端写入余额分。支付确认同时展示「到账余额 ¥xx.xx」和「渠道实付金额」。`calculatePresetPricing` 不得再用 `usdExchangeRate` 或 `quota_per_unit` 对账户余额充值额做二次展示。
- `system-settings/general/checkin-settings-section.tsx`：签到奖励上下限展示和输入使用 CNY 元，保存时转为余额分；如果实现阶段停用签到余额奖励，则 UI 必须同步隐藏或标注停用，不得继续显示历史 quota 单位。
- `features/redemption-codes` 钱包兑换码管理入口：钱包类型兑换码的金额展示和输入使用 CNY 元，提交时转为余额分；订阅类型兑换码仍按套餐选择处理，不涉及余额分。
- Creem 产品配置到账额度入口：如果当前前端暴露 Creem 产品的 `quota` / top-up credit 配置，展示和输入使用 CNY 元，保存时转为余额分，编辑回显和未修改保存必须保持稳定。
- `wallet/components/creem-products-section.tsx` 和 `wallet/components/dialogs/creem-confirm-dialog.tsx`：用户侧 Creem 产品卡片和确认弹窗按 `quota / 100` 显示到账余额 CNY 金额，例如 `¥40.00`，不得显示 `Quota: 4000` 或 raw cents。
- `system-settings/integrations/payment-settings-section.tsx`、`amount-options-visual-editor.tsx`、`amount-discount-*`、`waffo-settings-section.tsx`、`waffo-pancake-settings-section.tsx`：`AmountOptions`、`AmountDiscount` key、`MinTopUp`、`StripeMinTopUp`、`WaffoMinTopUp`、`WaffoPancakeMinTopUp` 面向用户输入金额，单位统一为账户余额 CNY 元。`Price` / `UnitPrice` 仍表示渠道收款规则，但标签和说明必须明确其作用是计算实付金额，不是到账余额倍率。
非目标：`used_quota`、日志统计、渠道用量、API key quota、订阅 token / quota reset 相关 UI 仍保留原有用量语义，不在本次改为余额分。

### Kyren 充值档位编辑器

管理员不应再看到历史倍率。API 和持久化层短期仍使用 `KyrenTopUpProduct.quota` 字段，但该字段语义为整数分；前端表单状态必须使用 CNY 元字段，避免把分误当元。

推荐前端表单 contract：

- 表单字段使用 `balance_cny` 或 `credit_amount_cny`，显示和输入 CNY 元，允许最多 2 位小数。
- 编辑已有产品时，用 `quota / 100` 回填表单，例如后端 `quota = 4000` 回显 `40.00`。
- 提交时用 `Math.round(amount * 100)` 转回 `quota`，例如输入 `39.90` 提交 `quota = 3990`。
- 未修改保存时必须保持数值稳定：`quota = 4000` 回显 `40.00`，保存仍提交 `4000`。
- 桌面和移动端列表列名改为「到账余额」，显示 `quota / 100` 的 CNY 金额，例如 `¥40.00`，不得使用 `formatQuotaShort(product.quota)`。
- 校验文案改为「到账余额必须至少 ¥0.01」或等价 i18n key；placeholder 使用 `40.00` 这类 CNY 金额，不再出现 `Quota must be at least 1` 或 `500000`。

提交给后端的 API 仍为：

```json
{
  "amount": "40.00",
  "quota": 4000
}
```

### i18n 与系统配置

前端不应再用 `quota_per_unit` 计算账户余额展示、账户余额充值、邀请奖励转余额或余额购买订阅。`quota_per_unit` 可以继续保留给历史用量展示和兼容代码，但不得出现在账户余额链路。

所有新增或变更的用户可见文案必须同步到默认前端 `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`。如果迁移版本仍允许 classic 主题，也必须同步 `web/classic/src/i18n/locales/{en,zh,fr,ja,ru,vi,zh-CN,zh-TW}.json`。账户余额链路统一使用「Account Balance / Wallet Balance / Top-up credit / 账户余额 / 到账余额」语义；不得在账户余额链路继续使用 `Quota`、`额度`、`quota units`、`{{quota}} quota`、`credited with quota` 等历史倍率文案。

允许保留 `Quota` 文案的范围仅限非账户余额用量：usage logs 的 `used_quota`、channel `used_quota`、API key quota、subscription token/quota reset、monitoring token quota 等。实现计划必须用静态扫描或定向测试区分这两类文案，避免误删模型用量语义。

当前服务默认主题为 `classic`，且系统配置允许在 `classic` 与 `default` 之间切换。实现计划必须二选一并写入验收：

1. 完整改造 `web/classic` 的账户余额链路，包括钱包余额、充值、账单历史、用户资料、用户列表、手动调额、钱包兑换码、签到奖励、注册 / 邀请奖励、Kyren / Creem（如 classic 暴露相关入口）和支付设置中的金额选项；这些入口与 `web/default` 使用相同的元 / 分 contract。
2. 或在迁移版本禁用 / 隐藏 classic 主题及其账户余额入口：将默认主题、`theme.frontend` 配置校验、路由回退和已有 classic 配置处理都收口到 `default`，并在验收中确认 classic 不能继续提供旧倍率账户余额 UI。

不得只改 `web/default` 后仍让 classic 入口可用。

## 运维流程

推荐部署步骤：

1. 通知短暂停机窗口。
2. 停止入口流量、新请求接入和新任务投递，但先保留每个旧版本实例运行，避免丢失进程内待刷数据。
3. 在每个旧版本实例上显式 flush 或等待 `BatchUpdateTypeUserQuota` 全部落库，并分别校验该实例的 `BatchUpdatePendingSnapshot().ByType[BatchUpdateTypeUserQuota] == 0`。该快照是进程内状态，迁移工具 / 停机脚本不能只检查新进程本地快照。当前 `model.CloseDB()` 不会 flush `BatchUpdateTypeUserQuota`，因此必须提供专门的 flush / drain 机制或等待批量更新周期完成；任一旧实例校验未通过时不得继续。
4. 所有旧实例均确认余额批量队列为空后，停止旧版本应用服务、后台任务、异步任务消费者和所有可能写库的实例，确认没有旧进程继续连接数据库。
5. 备份数据库。
6. 部署包含迁移逻辑的新版本。
7. 启动新版本。
8. 启动期执行 `EnsureAccountBalanceCentsMigration`。
9. 迁移成功后服务开始接收请求。
10. 抽查用户余额、Kyren 充值档位、钱包兑换码和余额购买订阅。

如果迁移失败：

- 数据阶段失败：事务回滚，不写入任何迁移标记，服务启动失败，运维人员根据日志修复后重新启动。
- 数据阶段已成功但缓存清理或最终标记写入失败：不得回滚已迁移数据；服务启动失败，下一次启动检测到 `AccountBalanceCentsDataMigrated = true` 后只重试运行时同步、缓存清理、`AccountBalanceCentsMigratedAt` 和最终标记写入。

## 回滚策略

推荐以数据库备份作为唯一支持的回滚方案。因为迁移会原地改变金额单位，应用代码和数据库必须匹配，不能在已迁移数据库上直接启动旧版本。

回滚步骤：

1. 停止新版本服务、入口流量、队列消费者、定时任务和所有会写库的后台进程。
2. 恢复迁移前数据库备份；禁止在已迁移数据库上直接启动旧版本。
3. 部署旧版本服务。
4. 验证用户余额和充值入口：抽样用户余额应按旧 quota 单位显示；普通充值、Kyren 充值档位、钱包兑换码和余额购买订阅入口应恢复到旧版本行为。
5. 验证回滚后的服务健康检查、登录、充值下单和余额购买订阅链路。

不实现自动反向迁移；如必须回退，唯一支持路径是恢复迁移前数据库备份。反向迁移会重新引入 `QuotaPerUnit`，且可能因四舍五入导致不可逆误差。

## 测试计划

后端测试：

- 迁移 `users.quota = 20000000` 后得到 `4000`。
- 迁移 `users.aff_quota` 和 `users.aff_history`。
- 不迁移历史已完成 `top_ups.amount`，并验证旧成功充值记录保持原值。
- 迁移前 pending `top_ups` 被统一标记为 `expired`，迁移后旧 webhook 或补单不会按新分制入账。
- 仅迁移钱包类型 `redemptions.quota`，不迁移订阅类型兑换码。
- 迁移 `KyrenTopUpProducts[].quota`。
- 迁移 `CreemProducts[].quota`，并同步数据库 Option、`common.OptionMap` 和运行时设置。
- 迁移 `QuotaForNewUser`、`QuotaForInviter`、`QuotaForInvitee` 运行时配置。
- 迁移 `checkin_setting.min_quota` 和 `checkin_setting.max_quota`，或在停用签到余额奖励时验证不再入账。
- 迁移历史 `checkins.quota_awarded`，或在停用签到余额奖励时验证历史金额展示被隐藏。
- 已存在迁移标记时不会重复迁移。
- `QuotaPerUnit <= 0` 时迁移失败且不写标记。
- 余额购买 `price_amount = 39.9` 的套餐扣 `3990`。
- 注册赠送、邀请奖励、邀请额度转余额按分入账和转移，不再使用 `QuotaPerUnit` 最小门槛。
- 管理端手动补单按 `TopUp.Amount` 分入账，不再按渠道乘 `QuotaPerUnit`。
- Kyren 充值档位 `quota = 4000` 入账后用户余额增加 `4000`。
- 普通充值渠道的新订单成功后按分入账，且 webhook 不再乘 `QuotaPerUnit`。
- 钱包兑换码按分入账。
- 静态扫描确认 `users.quota` 写入已收口：模型调用、异步任务、legacy wallet funding 不再通过 `IncreaseUserQuota` / `DecreaseUserQuota` 直接改账户余额。
- `logs.quota`、`tokens.remain_quota`、`user_subscriptions.token_used` 不受迁移影响。
- 迁移前已存在旧倍率 `UserBase.Quota` 用户缓存时，迁移成功后的首次请求不能读到旧余额值；Redis 可用时验证 `user:{id}` 被清理或重建，Redis 不可用时验证不会阻断数据库迁移。
- 迁移前任一旧实例仍存在未刷新的 `BatchUpdateTypeUserQuota` 时，迁移前置校验失败且不写任何迁移标记；只有每个旧实例都确认 pending=0 后才能停止旧进程并继续迁移。
- 充值记录列表 API 对同一支付渠道迁移前成功订单和迁移后成功订单返回稳定的展示字段；前端使用展示字段而不是 `record.amount` 自行判断单位。

前端测试：

- `4000` 在钱包、资料页、用户详情和管理端用户列表中展示为 `¥40.00`。
- `3990` 展示为 `¥39.90`。
- 余额 `3990` 可购买 `price_amount = 39.9` 套餐。
- 余额 `3989` 不可购买 `price_amount = 39.9` 套餐。
- 管理员手动调整账户余额输入 `40.00` 时提交 `4000`，不再通过 `quotaPerUnit` 换算。
- 邀请奖励转余额输入和最小步长不再依赖 `QUOTA_PER_DOLLAR = 500000`。
- Kyren 充值档位编辑已有 `quota = 4000` 时回显 `40.00`，未修改保存仍提交 `4000`。
- Kyren 充值档位输入 `39.90` 时提交 `quota = 3990`，列表显示 `¥39.90` 而不是 `3.99K`。
- 用户侧 Kyren 充值档位显示「到账余额 ¥xx.xx」，不显示 raw `quota` 或 `{{quota}} quota`。
- 用户侧签到日历和签到成功提示按余额分显示奖励；停用签到余额奖励时相关展示同步隐藏或标注停用。
- 用户侧 Creem 产品卡片和确认弹窗显示「到账余额 ¥40.00」，不显示 `Quota: 4000`。
- 账单历史不会把未迁移的历史 `top_ups.amount` 统一按分误读；同一渠道迁移前后订单均可稳定展示。
- i18n 六种语言 `en`、`zh`、`fr`、`ja`、`ru`、`vi` 均包含新增或替换文案；账户余额链路不再出现误导管理员填写历史「额度倍率」的文案。
- 普通 Epay / Stripe / Waffo / Waffo Pancake 充值预设、自定义输入和确认弹窗均以账户余额 CNY 元展示到账金额；`quota_display_type = TOKENS` 或修改 `quota_per_unit` 后展示、提交和折扣命中不变化。
- 管理端 `AmountOptions`、`AmountDiscount`、各渠道 `MinTopUp` 以账户余额 CNY 元展示和保存；`Price` / `UnitPrice` 文案明确为渠道实付计算规则。
- 系统设置中的注册赠送、邀请奖励和签到奖励输入 `10.00` 时保存为 `1000`，编辑回显为 `10.00`；`PreConsumedQuota` 等非账户余额配置不受影响。
- 钱包兑换码金额输入 `40.00` 时提交 `quota = 4000`；订阅类型兑换码仍按套餐处理。
- Creem 产品配置到账额度输入和编辑回显按 CNY 元处理，保存为余额分。
- 若选择继续支持 classic 主题，classic 钱包、充值、兑换码、签到、调额、支付设置和 i18n 与 default 具有同样的分制行为；若选择禁用 classic，主题默认值、配置校验和路由回退均不能再提供 classic 账户余额入口。

## 验收标准

- 管理员配置 Kyren / Creem 充值档位时只需要理解 CNY 金额，不需要理解 `QuotaPerUnit`。
- 用户钱包余额显示与订阅价格直接对应。
- 用户资料页、钱包页、用户详情、管理端用户列表和手动调额弹窗都按余额分显示或输入账户余额。
- 账户余额购买订阅不再依赖 `common.QuotaPerUnit`。
- 钱包兑换码、注册赠送、邀请奖励、签到奖励、Creem / Kyren 充值档位和管理端补单不再依赖 `common.QuotaPerUnit`。
- 迁移在 SQLite、MySQL 和 PostgreSQL 上都可执行。
- 迁移可重复启动安全：成功后不会再次执行，失败后不会写入标记。
- 迁移前 pending 充值订单不会跨单位履约。
- 迁移后用户余额缓存不会保留历史倍率值。
- classic 主题要么完整按分制改造，要么在迁移版本不可用于账户余额链路。
- 非账户余额用量字段不被迁移，也不改变行为；模型调用和异步任务用量路径不再写入 `users.quota`。
