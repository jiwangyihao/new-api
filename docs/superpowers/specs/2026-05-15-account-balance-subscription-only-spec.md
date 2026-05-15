# 账户余额购买套餐与全面套餐制规格

> 面向 AI 代理的工作者：本规格是在 `docs/superpowers/specs/2026-05-13-token-distribution-platform-spec.md` 基础上的后续修订。实现前必须读取仓库根目录 `AGENTS.md`，并遵守 Go + Gin + GORM、React + TypeScript + Bun、SQLite / MySQL / PostgreSQL 全兼容约束。

**目标：** 将平台计费模型收敛为「账户余额只用于购买套餐，API 请求只消耗订阅套餐 token 和并发」的全面套餐制。

**架构：** 保留现有钱包余额、充值订单和余额流水作为储值账户能力，但从 relay / task 请求资金来源中移除钱包余额、用户 quota 和 token key quota。订阅套餐购买新增账户余额支付路径；余额不足时提示先充值，不做混合支付。

**技术栈：** Go 1.25.1、Gin、GORM v2、PostgreSQL / MySQL / SQLite、React 19、TypeScript、Rsbuild、Bun。

---

## 1. 决策

1. **保留钱包余额，但改名为账户余额。** 余额不再表示 API 可用额度，只表示可用于购买套餐的储值金额。
2. **API 请求全面套餐制。** `/v1/chat/completions`、`/v1/responses`、`/v1/responses/compact` 以及同步文本生成类 relay 请求必须使用有效订阅套餐。无有效套餐、套餐过期、token 不足或并发超限时直接拒绝。
3. **移除请求级钱包 fallback。** `wallet_first`、`wallet_only`、`subscription_first` 的钱包回退语义不再用于 API 请求。前端不再展示计费偏好下拉框。
4. **非文本任务不使用账户余额兜底。** images、audio-only、embeddings、rerank、Midjourney、Suno、视频等非文本 / 异步任务如果当前套餐不支持，返回明确错误，不再消耗钱包余额或 token key quota。
5. **账户余额可购买套餐。** 购买套餐时可选择「账户余额支付」。余额足够则立即扣减余额并创建订阅；余额不足则提示先充值。本次不做混合支付。
6. **历史字段保留但不参与请求扣费。** `users.quota`、`tokens.remain_quota`、`tokens.used_quota` 等字段不删除，用于兼容旧数据、历史展示或管理操作，但不再决定 API 请求能否执行。

## 2. 业务范围

### 2.1 必须满足

- 用户仍可充值账户余额。
- 账户余额只能用于购买套餐，不参与 API 请求扣费。
- 套餐购买页支持账户余额支付。
- API 请求只读取有效 `UserSubscription` 的 `token_limit`、`token_used`、`concurrency_limit`。
- 用户端隐藏计费偏好下拉框，不再出现「余额优先」「仅余额」等选项。
- 用户端钱包文案改为「账户余额，可用于购买套餐」。
- OpenAI 兼容 billing 查询接口返回订阅套餐 token 语义，不再返回钱包余额语义。
- 重复余额支付请求必须幂等，不能重复扣余额或重复创建订阅。
- SQLite、MySQL、PostgreSQL 兼容。

### 2.2 非目标

- 不删除数据库中的 `quota` 字段。
- 不删除充值、余额流水、管理员调整余额能力。
- 不做余额 + 在线支付的混合支付。
- 不把非文本任务重新纳入分销套餐扣费。
- 不恢复旧 quota / 价格倍率作为套餐限制口径。

## 3. 请求计费设计

### 3.1 资金来源

请求级资金来源只允许订阅：

```text
API 请求 -> 查找有效订阅 -> 预扣 estimated tokens -> 获取并发租约 -> 上游请求 -> 按实际 usage 结算 token
```

无有效订阅时返回 OpenAI 兼容错误：

```json
{
  "error": {
    "message": "active subscription is required",
    "type": "insufficient_quota",
    "code": "subscription_required"
  }
}
```

余额不足、token 不足、并发超限必须分别返回可区分错误码：

- `subscription_required`
- `subscription_token_exhausted`
- `subscription_concurrency_exceeded`

### 3.2 移除旧偏好语义

后端可保留 `billing_preference` 字段用于兼容旧用户设置，但请求执行时统一等价为：

```text
subscription_only
```

前端不再允许用户切换：

- `subscription_first`
- `wallet_first`
- `subscription_only`
- `wallet_only`

旧偏好值不会导致请求走钱包。

### 3.3 token key quota

API token 仍用于鉴权、模型权限、分组权限、过期时间、速率限制和日志归属，但不再用 `remain_quota` 作为请求资金来源。

行为要求：

- 请求前不扣 `tokens.remain_quota`。
- 请求后不因套餐 token 消耗更新 `tokens.used_quota` 作为资金口径。
- 若旧 UI 仍展示 token quota，应隐藏或标注为历史字段。

## 4. 账户余额购买套餐

### 4.1 支付方式

订阅购买新增支付方式：

```text
account_balance
```

前端展示为：

```text
账户余额支付
```

### 4.2 购买规则

- 只能购买 `enabled = true AND public_visible = true AND is_trial = false` 的套餐。
- `price_amount <= 0` 的套餐不能通过余额支付购买。
- 余额单位与套餐 `currency` 一致；当前分销套餐使用 `CNY`。
- 余额足够：扣减用户账户余额，创建成功订阅订单，发放订阅。
- 余额不足：不扣余额，不创建订阅，返回「账户余额不足，请先充值」。
- 重复提交同一个幂等键时，不重复扣款或重复发放。

### 4.3 幂等设计

新增余额支付请求字段：

```json
{
  "plan_id": 2,
  "idempotency_key": "uuid-from-client"
}
```

后端规则：

- `idempotency_key` 为空时后端生成请求 ID，但前端必须传入，避免用户重复点击导致重复扣款。
- `SubscriptionOrder.trade_no` 使用余额支付专用前缀，例如 `balance-sub-<uuid>`。
- 如果同一用户同一 `idempotency_key` 已成功购买，直接返回已有订单和订阅结果。
- 如果已有 pending 记录，返回当前状态，不再次扣款。

### 4.4 订单与日志

余额支付也创建 `SubscriptionOrder`：

- `payment_provider = "balance"`
- `payment_method = "account_balance"`
- `status = success`
- `money = plan.price_amount`
- `complete_time` 为当前时间

同时记录账户余额流水，说明：

```text
账户余额购买订阅套餐：Basic
```

## 5. OpenAI 兼容 billing 接口

这些接口不得继续返回钱包余额语义：

- `/dashboard/billing/subscription`
- `/dashboard/billing/usage`

新语义：

- 有有效订阅：返回套餐 token 总量、已用 token、剩余 token 对应的兼容金额字段。
- Trial 无限 token：返回足够大的兼容额度或明确无限语义；内部不读取钱包余额。
- 无有效订阅：返回 0 或无订阅状态。

由于 OpenAI 字段名包含 `*_usd`，响应字段名可保持兼容，但注释和内部逻辑必须明确它们映射的是套餐 token 语义，而不是钱包余额。

## 6. 前端改造

### 6.1 钱包页文案

将「钱包余额」相关文案改为：

```text
账户余额
账户余额可用于购买套餐，不能直接用于 API 请求。
```

### 6.2 移除计费偏好 UI

删除或隐藏订阅卡片中的计费偏好下拉框。

不再展示：

- 订阅优先
- 余额优先
- 仅订阅
- 仅余额

如果需要展示当前模式，显示静态说明：

```text
API 请求按当前有效套餐扣减 token。
```

### 6.3 套餐购买 UI

购买弹窗增加账户余额支付按钮。

展示：

- 套餐价格，例如 `¥40.00`。
- 当前账户余额，例如 `¥120.00`。
- 余额足够时按钮可点击。
- 余额不足时按钮禁用，并提示先充值。
- 在线支付方式仍可保留。

### 6.4 隐藏旧请求余额信息

用户端不再展示「余额可用于 API 调用」「剩余额度」「余额优先」等旧文案。

## 7. 管理端行为

- 管理员仍可调整用户账户余额。
- 管理员仍可查看充值与余额流水。
- 管理员给用户发放订阅不依赖余额。
- 管理员查看用户时，余额字段标注为「账户余额」。

## 8. 兼容与迁移

无需数据库破坏性迁移。

需要数据修正：

- 将所有用户 `billing_preference` 逻辑等价为 `subscription_only`。
- 可选：批量把用户设置中的旧偏好更新为 `subscription_only`，但请求层必须即使遇到旧值也不走钱包。
- 已有用户余额保留，可用于购买套餐。
- 已有充值订单和日志保留。

## 9. 测试方案

### 9.1 后端测试

新增或修改测试：

- 请求计费：旧 `wallet_first` / `wallet_only` 用户也必须走订阅；无订阅时返回 `subscription_required`。
- token key quota：请求不会扣 `Token.RemainQuota` 或用其绕过订阅。
- 非文本任务：不 fallback 钱包。
- 余额购买套餐：余额足够成功扣款并创建订阅；余额不足不扣款；重复提交不重复扣款。
- OpenAI billing 接口：返回订阅 token 语义，不读取钱包余额。

### 9.2 前端验证

- 钱包页不再出现计费偏好下拉框。
- 文案显示「账户余额可用于购买套餐」。
- 套餐购买弹窗可以使用账户余额支付。
- 余额不足时按钮禁用或显示明确提示。
- 用户无订阅时，API 使用提示引导购买套餐。

### 9.3 手动联调

- 给用户充值 100 元账户余额。
- 使用账户余额购买 Basic，余额变为 60 元，并获得 Basic 订阅。
- 重复点击购买不会二次扣款。
- 无有效订阅用户调用 `/v1/chat/completions` 返回 `subscription_required`。
- 有余额但无订阅用户调用 API 仍返回 `subscription_required`。
- 有 Basic 订阅用户调用 API，只增加 `token_used`，账户余额不变。
- token key `remain_quota = 0` 但用户有有效订阅时，请求仍可执行。
- `wallet_first` 旧偏好用户无订阅时不再 fallback 钱包。

## 10. 验收标准

- API 请求不再消耗账户余额。
- API 请求不再依赖用户 quota 或 token key quota。
- 账户余额只能购买套餐。
- 用户端没有余额优先 / 仅余额等旧计费偏好入口。
- 余额购买套餐幂等，不重复扣款。
- 旧余额用户可以用余额购买套餐。
- 非文本任务不消耗分销订阅 token，也不 fallback 账户余额。
- OpenAI 兼容 billing 接口不再暴露钱包余额语义。
