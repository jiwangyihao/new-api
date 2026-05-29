# Kyren 支付接入设计

## 背景

当前支付系统包含两条主链路：

1. **钱包充值**：用户购买余额或额度，后端以 `model.TopUp` 记录本地订单，支付成功后增加用户余额。
2. **订阅套餐购买**：用户购买套餐，后端以 `model.SubscriptionOrder` 记录本地订单，支付成功后通过 `model.CompleteSubscriptionOrder` 创建或续期 `UserSubscription`。

现有支付 provider 包括 Epay、Stripe、Creem、Waffo/Pancake 和账户余额。订阅套餐模型 `SubscriptionPlan` 已包含 `stripe_price_id` 与 `creem_product_id`，说明项目现有设计倾向于将外部支付商品 ID 绑定到本地套餐，而不是让外部支付平台成为套餐主数据源。

Kyren Pay 原生 API 使用 `x-api-key` 鉴权，创建 Checkout Session 时必须传 `productId`。产品 API 支持创建、查询、列表和更新产品；Webhook 使用 `X-Kyren-Signature`、`X-Kyren-Timestamp` 与 HMAC-SHA256 验签。金额字段使用 decimal string，文档示例支持 `USD`、`CNY`、`HKD`。本项目当前大概率只计划使用 `CNY`，首版设计以 `CNY` 为主路径。

相关 Kyren 文档：

- [Create a product](https://docs.kyren.top/api-reference/products/create-a-product.md)
- [Create a checkout session](https://docs.kyren.top/api-reference/checkouts/create-a-checkout-session.md)
- [Webhook Signatures](https://docs.kyren.top/webhooks/signatures.md)
- [Webhook Events](https://docs.kyren.top/webhooks/events.md)
- [Epay-compatible migration](https://docs.kyren.top/epay-migration/overview.md)

## 目标

1. 将 Kyren 作为独立支付 provider 接入，而不是复用或伪装成 Epay。
2. 支持订阅套餐使用 Kyren 原生 Checkout 支付。
3. 支持钱包余额充值使用 Kyren 原生 Checkout 支付。
4. 后台允许管理员将本地订阅套餐绑定到已有 Kyren 产品。
5. 后台允许管理员从本地订阅套餐一键创建或同步 Kyren 产品，避免频繁手动登录 Kyren 控制台。
6. 后台允许管理员配置若干固定余额充值档位，并一键创建或同步为 Kyren 产品。
7. 支付成功以 Kyren Webhook 为准，本地回调处理必须幂等、安全、可重试。
8. 首版以 `CNY` 为默认和推荐币种；保留字段兼容 Kyren 支持的其他币种，但不为多币种定价设计复杂策略。
9. 保持数据库兼容 SQLite、MySQL、PostgreSQL。
10. 前端遵循 `web/default` 现有 React、React Query、Base UI/Tailwind 和 i18n 约定。

## 非目标

- 不把 Kyren Epay-compatible 接口作为正式主链路；它仅可作为临时迁移或应急方案。
- 不在用户每次输入任意充值金额时动态创建 Kyren 产品。
- 不实现自动退款、自动扣回余额或自动撤销订阅权益。
- 不实现 Kyren 对账报表、订单 CSV 导出、结算流水查询或商户余额展示。
- 不实现多币种价格换算、汇率同步或同一套餐多币种价格矩阵。
- 不修改现有 Stripe、Creem、Epay、Waffo/Pancake 的支付语义。
- 不引入新的通用支付抽象层；本次只添加 Kyren provider 所需的最小边界。

## 方案选择

### 方案 A：将 Kyren 配置为 Epay-compatible 地址

把现有 `PayAddress` 指向 Kyren Epay-compatible `submit.php` 或 `mapi.php`，继续使用 Epay 签名与回调。

优点：改动最小，可能较快跑通充值和订阅。

缺点：本地订单仍显示为 Epay；provider guard 无法区分真实 Epay 与 Kyren；无法使用 Kyren 原生 HMAC Webhook、metadata 和产品模型；长期对账与故障定位会混淆。

结论：不作为正式方案。

### 方案 B：Kyren 原生 provider + 手动 productId 绑定

新增 `PaymentProviderKyren`，使用 Kyren 原生产品、Checkout 与 Webhook；后台只提供 `kyren_product_id` 输入框，由管理员在 Kyren 控制台手动创建产品后复制 ID。

优点：安全边界清晰，改动适中。

缺点：运营流程不够便捷；套餐价格或名称调整后需要重复跨系统手动同步；余额充值档位维护成本高。

结论：不单独采用，但保留「手动绑定已有 productId」能力。

### 方案 C：Kyren 原生 provider + 本地快捷创建/同步产品（采用）

新增 Kyren 独立 provider。订阅套餐和余额充值档位仍以本地配置为主，Kyren 产品作为外部支付商品映射。后台既允许手动绑定已有 productId，也允许从当前套餐或充值档位一键创建、同步 Kyren 产品。

优点：provider 边界清晰；支付安全模型更强；后台维护便捷；订阅套餐是主产品时体验最好；充值档位也能集中管理。

缺点：需要新增 Kyren client、产品管理接口、后台按钮和状态展示。

结论：采用。

## 核心设计原则

1. **本地套餐是主数据源**：`SubscriptionPlan` 决定套餐名称、价格、时长、额度、并发和可见性。
2. **Kyren 产品是支付映射**：`productId` 只用于创建 Kyren Checkout，不反向驱动本地套餐配置。
3. **Webhook 决定最终到账**：前端跳转成功页不发放权益，只有 `order.paid` 通过验签后才完成本地订单。
4. **metadata 必须携带本地订单号**：Checkout metadata 必须包含 `trade_no` 和 `kind`，用于 Webhook 精准定位。
5. **金额使用 decimal string**：与 Kyren 交互时使用固定两位小数字符串，例如 `40.00`，避免 float 精度问题。
6. **首版主路径为 CNY**：后台默认创建 `CNY` 产品；如果保留其他币种输入，也只做字段透传和校验，不做换算。

## 数据模型

### `SubscriptionPlan`

新增字段：

```go
KyrenProductId string `json:"kyren_product_id" gorm:"type:varchar(128);default:''"`
```

更新位置：

- `model.SubscriptionPlan` 结构体；
- `controller.AdminCreateSubscriptionPlan`；
- `controller.AdminUpdateSubscriptionPlan` 的 `updateMap`；
- 前端 `subscriptionPlanSchema`；
- 前端套餐表单默认值、回填和提交 payload；
- 相关测试 fixture。

### 余额充值档位

首版不新增表，使用 option JSON 保存 Kyren 充值档位：

```go
setting.KyrenTopUpProducts = "[]"
```

建议 JSON 结构：

```json
[
  {
    "id": "topup_cny_10",
    "name": "余额充值 10 元",
    "description": "充值 10 元账户余额",
    "product_id": "prod_xxx",
    "amount": "10.00",
    "currency": "CNY",
    "quota": 5000000,
    "enabled": true
  }
]
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `id` | string | 是 | 本地稳定 ID，用于更新、同步和前端选择。 |
| `name` | string | 是 | Kyren 产品名称和前端展示名。 |
| `description` | string | 否 | Kyren 产品描述。 |
| `product_id` | string | 否 | 已绑定的 Kyren 产品 ID。为空时可一键创建。 |
| `amount` | string | 是 | 支付金额，decimal string，首版使用 CNY 元。 |
| `currency` | string | 是 | 首版默认 `CNY`。 |
| `quota` | int64 | 是 | 充值到账额度。 |
| `enabled` | bool | 是 | 是否对用户开放。 |

`amount` 使用 string 是为了与 Kyren decimal string 保持一致。后端保存前需校验格式 `^\d+(\.\d{1,2})?$`，且金额大于等于 `0.01`。

## 配置项

新增运行时配置：

```go
package setting

var KyrenApiKey = ""
var KyrenWebhookSecret = ""
var KyrenBaseURL = "https://api.kyren.top"
var KyrenTopUpProducts = "[]"
```

新增 option keys：

| Key | 默认值 | 说明 |
|---|---|---|
| `KyrenApiKey` | `""` | Kyren API Key。前端保存时空值不覆盖旧值。 |
| `KyrenWebhookSecret` | `""` | Kyren Webhook Secret。前端保存时空值不覆盖旧值。 |
| `KyrenBaseURL` | `https://api.kyren.top` | Kyren API 地址，允许配置 staging 地址。保存时去除末尾 `/`。 |
| `KyrenTopUpProducts` | `[]` | Kyren 余额充值档位 JSON。 |

Webhook 可用性判断：

- `KyrenApiKey` 非空；
- `KyrenWebhookSecret` 非空；
- 订阅支付至少存在一个公开可购买且 `kyren_product_id` 非空的套餐，或 `KyrenTopUpProducts` 中至少存在一个启用且 `product_id` 非空的档位。

## Kyren Client

新增内部 client，集中处理 Kyren API 调用。

职责：

- 拼接 `KyrenBaseURL`；
- 添加 `x-api-key`；
- 使用 `common.Marshal` / `common.Unmarshal`；
- 对非 2xx 或 Kyren `code != 0` 返回明确错误；
- 统一设置请求超时；
- 暴露产品和 Checkout 所需方法。

建议方法：

```go
type KyrenClient struct {
    BaseURL string
    APIKey  string
}

func (c *KyrenClient) CreateProduct(ctx context.Context, req KyrenCreateProductRequest) (*KyrenProduct, error)
func (c *KyrenClient) UpdateProduct(ctx context.Context, id string, req KyrenUpdateProductRequest) (*KyrenProduct, error)
func (c *KyrenClient) RetrieveProduct(ctx context.Context, id string) (*KyrenProduct, error)
func (c *KyrenClient) ListProducts(ctx context.Context, status string, page, size int) (*KyrenProductList, error)
func (c *KyrenClient) CreateCheckout(ctx context.Context, req KyrenCreateCheckoutRequest) (*KyrenCheckoutSession, error)
```

金额格式化：

- 本地 `float64` 套餐价格转 Kyren 字符串时使用 decimal，保留 2 位小数；
- 余额充值档位直接保存 string，但后端仍需用 decimal 解析并重新标准化为 2 位；
- 不使用 `fmt.Sprintf("%.2f", float64)` 作为唯一金额校验依据。

## 后端 API

### 支付配置

支付设置页继续通过现有 option 更新接口保存全局配置。新增 keys 后，`model.InitOptionMap` 和 `model.updateOptionMap` 负责运行时加载。

### Kyren 产品通用管理

路由：`/api/payment/kyren/*`，权限：`middleware.AdminAuth()`。

#### `GET /api/payment/kyren/products`

用途：查询 Kyren 产品列表，用于手动绑定时搜索已有产品。

查询参数：

| 参数 | 类型 | 默认 | 说明 |
|---|---|---:|---|
| `status` | string | `ACTIVE` | Kyren 产品状态。 |
| `page` | int | `1` | 页码。 |
| `size` | int | `20` | 每页数量，范围 1–100。 |

#### `GET /api/payment/kyren/products/:id`

用途：查询单个 Kyren 产品，用于刷新绑定状态。

#### `POST /api/payment/kyren/products`

用途：创建通用 Kyren 产品。首版后台主要通过套餐和充值档位快捷接口调用，通用创建接口可供后续复用。

请求示例：

```json
{
  "name": "余额充值 10 元",
  "description": "充值 10 元账户余额",
  "price": "10.00",
  "currency": "CNY",
  "metadata": {
    "source": "new-api",
    "kind": "wallet_topup"
  }
}
```

#### `PATCH /api/payment/kyren/products/:id`

用途：更新 Kyren 产品名称、描述、价格、币种或 metadata。

### 订阅套餐快捷绑定

路由：`/api/subscription/admin/plans/:id/kyren/product`，权限：`middleware.AdminAuth()`。

#### `POST /api/subscription/admin/plans/:id/kyren/product`

用途：从本地套餐创建或同步 Kyren 产品。

请求：

```json
{
  "mode": "create_or_update"
}
```

`mode` 取值：

| 值 | 行为 |
|---|---|
| `create_or_update` | 如果 `kyren_product_id` 为空则创建；否则更新已绑定产品。 |
| `create_new` | 创建新产品并覆盖绑定。用于原绑定产品不存在或已归档后重新绑定。 |
| `update_existing` | 只更新已有绑定；如果未绑定则报错。 |

创建 Kyren 产品时字段映射：

```json
{
  "name": "套餐标题",
  "description": "订阅套餐：套餐标题，周期：1 month",
  "price": "40.00",
  "currency": "CNY",
  "metadata": {
    "source": "new-api",
    "kind": "subscription_plan",
    "plan_id": "123",
    "business_code": "basic_monthly"
  }
}
```

响应：

```json
{
  "success": true,
  "data": {
    "product_id": "prod_xxx",
    "status": "ACTIVE",
    "price": "40.00",
    "currency": "CNY",
    "synced": true
  }
}
```

处理规则：

1. 套餐不存在：返回业务错误。
2. `KyrenApiKey` 未配置：返回业务错误。
3. 套餐价格小于 `0.01`：返回业务错误，免费或试用套餐不创建 Kyren 产品。
4. 首版套餐币种为空时按 `CNY` 处理；非 `CNY` 允许保存但创建 Kyren 产品前必须确认属于 Kyren 支持币种。
5. `create_or_update` 遇到已绑定产品 404：返回明确错误，不自动创建新产品，避免悄悄覆盖外部绑定。
6. `create_new` 成功后必须在同一后端请求中回填 `SubscriptionPlan.KyrenProductId` 并清理套餐缓存。

#### `GET /api/subscription/admin/plans/:id/kyren/product`

用途：刷新当前套餐绑定的 Kyren 产品状态。

返回：

```json
{
  "success": true,
  "data": {
    "bound": true,
    "product_id": "prod_xxx",
    "status": "ACTIVE",
    "price": "40.00",
    "currency": "CNY",
    "price_matches": true,
    "currency_matches": true
  }
}
```

### 充值档位快捷管理

路由：`/api/payment/kyren/topup-products/*`，权限：`middleware.AdminAuth()`。

#### `GET /api/payment/kyren/topup-products`

返回本地 Kyren 充值档位配置。

#### `PUT /api/payment/kyren/topup-products`

整体保存充值档位 JSON。后端负责校验：

- `id` 非空且唯一；
- `name` 非空；
- `amount` 是合法 decimal string 且大于等于 `0.01`；
- `currency` 首版默认和推荐 `CNY`；
- `quota` 大于 0；
- `product_id` 可为空，但不为空时必须形如非空字符串。

#### `POST /api/payment/kyren/topup-products/:id/sync`

用途：为本地充值档位创建或同步 Kyren 产品。

请求：

```json
{
  "mode": "create_or_update"
}
```

创建产品 metadata：

```json
{
  "source": "new-api",
  "kind": "wallet_topup",
  "topup_product_id": "topup_cny_10",
  "quota": "5000000"
}
```

同步成功后更新 `KyrenTopUpProducts` 中对应档位的 `product_id`。

## 支付流程

### 订阅套餐 Kyren 支付

路由：`POST /api/subscription/kyren/pay`，权限：`middleware.UserAuth()` + `middleware.CriticalRateLimit()`。

请求：

```json
{
  "plan_id": 123
}
```

流程：

```text
用户点击 Kyren 支付
  -> 后端读取 SubscriptionPlan
  -> 校验套餐可购买
  -> 校验 plan.kyren_product_id 非空
  -> 创建本地 SubscriptionOrder，provider=kyren，status=pending
  -> 调用 POST /v1/checkouts
       productId = plan.kyren_product_id
       successUrl / cancelUrl
       customerEmail / customerName
       metadata.kind = subscription
       metadata.trade_no = 本地订单号
       metadata.plan_id = 套餐 ID
       metadata.user_id = 用户 ID
  -> 返回 checkout_url
  -> 前端跳转 Kyren Checkout
```

响应：

```json
{
  "success": true,
  "data": {
    "checkout_url": "https://..."
  }
}
```

### 钱包充值 Kyren 支付

路由：`POST /api/user/kyren/pay`，权限：`middleware.UserAuth()` + `middleware.CriticalRateLimit()`。

请求：

```json
{
  "product_id": "topup_cny_10"
}
```

这里的 `product_id` 是本地充值档位 ID，不是 Kyren `prod_xxx`。后端根据本地档位找到 Kyren `product_id`。

流程：

```text
用户选择固定充值档位
  -> 后端读取 KyrenTopUpProducts
  -> 校验本地档位存在、启用且已绑定 Kyren product_id
  -> 创建本地 TopUp，provider=kyren，status=pending
  -> 调用 POST /v1/checkouts
       productId = 档位绑定的 Kyren product_id
       successUrl / cancelUrl
       metadata.kind = topup
       metadata.trade_no = 本地订单号
       metadata.topup_product_id = 本地档位 ID
       metadata.user_id = 用户 ID
  -> 返回 checkout_url
  -> 前端跳转 Kyren Checkout
```

## Webhook 设计

路由：`POST /api/kyren/webhook`，不要求用户登录。

### 验签

必须使用原始请求体：

1. 读取 raw body。
2. 读取 `X-Kyren-Signature` 与 `X-Kyren-Timestamp`。
3. 校验 timestamp 可解析，且与当前时间差不超过 5 分钟。
4. 计算：`HMAC-SHA256(timestamp + "." + raw_body, KyrenWebhookSecret)`。
5. 期望签名格式：`sha256=<hex>`。
6. 使用常量时间比较。
7. 验签失败返回非 2xx。

### 事件处理

支持事件：

| 事件 | 行为 |
|---|---|
| `order.paid` | 完成本地订单。 |
| `order.closed` | 将仍处于 pending 的本地订单标记为 expired 或 failed。 |
| `order.refunded` | 首版只记录日志，不自动扣回余额或撤销订阅。 |

`order.paid` 处理：

1. 从 `data.metadata.trade_no` 读取本地订单号。
2. 从 `data.metadata.kind` 判断 `subscription` 或 `topup`。
3. 校验本地订单存在、provider 为 `kyren`。
4. 校验本地订单仍为 pending；如果已 success，直接返回成功，保证幂等。
5. 校验金额与币种匹配。
6. 订阅订单调用现有订阅完成边界。
7. 充值订单调用新增 `RechargeKyren` 或等价完成逻辑。
8. 保存 Kyren webhook payload 到 `ProviderPayload` 或日志，便于排查。

金额校验规则：

- 订阅：Kyren `amount` 必须等于套餐订单 `Money` 标准化后的 decimal string。
- 充值：Kyren `amount` 必须等于本地充值档位 `amount`。
- 币种首版必须匹配 `CNY`，除非后续明确启用多币种。

## 前端设计

### 后台支付设置页

在支付设置中新增「Kyren Pay」分区：

- API Key；
- Webhook Secret；
- Base URL，默认 `https://api.kyren.top`；
- Webhook URL 展示：`{ServerAddress}/api/kyren/webhook`；
- 充值档位可视化编辑器；
- 每个充值档位提供「创建 Kyren 产品」「同步 Kyren 产品」「刷新状态」。

API Key 和 Webhook Secret 保存规则沿用 Stripe/Creem：空值不覆盖旧值。

### 后台订阅套餐抽屉

在套餐编辑抽屉新增「Kyren 支付」区域：

- `Kyren Product ID` 输入框；
- 当前绑定状态；
- 「创建 Kyren 产品」按钮；
- 「同步到 Kyren」按钮；
- 「刷新状态」按钮。

交互规则：

1. 新建套餐时，需先保存本地套餐，再创建 Kyren 产品。
2. 编辑套餐时，如果 `kyren_product_id` 为空，显示「创建 Kyren 产品」。
3. 编辑套餐时，如果 `kyren_product_id` 非空，显示「同步到 Kyren」和「刷新状态」。
4. 如果本地价格或币种与 Kyren 产品不一致，显示醒目提示，但不自动修改。

### 用户订阅购买弹窗

新增 Kyren 支付按钮：

- 仅当 Kyren 全局配置可用且当前套餐 `kyren_product_id` 非空时显示；
- 点击后调用 `/api/subscription/kyren/pay`；
- 成功后跳转 `checkout_url`。

### 用户钱包充值页

余额充值区域新增 Kyren 固定档位：

- 后端返回启用且已绑定 Kyren product 的档位；
- 用户选择档位后调用 `/api/user/kyren/pay`；
- 成功后跳转 `checkout_url`。

## 路由汇总

新增公开 webhook：

```text
POST /api/kyren/webhook
```

新增用户支付路由：

```text
POST /api/user/kyren/pay
POST /api/subscription/kyren/pay
```

新增管理员路由：

```text
GET   /api/payment/kyren/products
POST  /api/payment/kyren/products
GET   /api/payment/kyren/products/:id
PATCH /api/payment/kyren/products/:id

GET  /api/payment/kyren/topup-products
PUT  /api/payment/kyren/topup-products
POST /api/payment/kyren/topup-products/:id/sync

GET  /api/subscription/admin/plans/:id/kyren/product
POST /api/subscription/admin/plans/:id/kyren/product
```

## 安全与幂等

1. Kyren Webhook 必须先验签再解析业务数据。
2. Webhook timestamp 超过 5 分钟必须拒绝。
3. 签名比较必须使用常量时间比较。
4. `trade_no` 必须唯一，并且只允许完成 provider 为 `kyren` 的本地订单。
5. Webhook 重复投递时，已完成订单应直接返回成功。
6. Webhook payload 中的 `product_id`、`amount`、`currency` 必须与本地订单或档位匹配。
7. 管理员创建或同步 Kyren 产品时，不在前端暴露 API Key。
8. 后端日志不得输出完整 API Key、Webhook Secret 或用户敏感支付信息。

## 测试计划

### 后端单元测试

- Kyren webhook 签名成功。
- Kyren webhook 签名错误被拒绝。
- Kyren webhook timestamp 超时被拒绝。
- `order.paid` 完成订阅订单。
- 重复 `order.paid` 保持幂等。
- provider guard 阻止 Kyren webhook 完成非 Kyren 订单。
- 金额不匹配时拒绝完成订单。
- 币种不匹配时拒绝完成订单。
- `order.closed` 只影响 pending 订单。
- `order.refunded` 不自动撤销权益。
- Kyren 充值档位 JSON 校验。
- 订阅套餐快捷创建产品成功后回填 `kyren_product_id`。

### 前端测试

- 套餐表单能读写 `kyren_product_id`。
- 新建套餐后不直接创建 Kyren 产品，提示需先保存。
- 编辑套餐时能触发创建/同步 Kyren 产品请求。
- 订阅购买弹窗仅在套餐已绑定 Kyren 产品时显示 Kyren 支付按钮。
- Kyren 充值档位编辑器校验 amount、quota、id 唯一性。

### 集成验证

若 Kyren 提供 staging 凭据：

1. 配置 staging API Key、Webhook Secret、Base URL。
2. 从套餐后台一键创建 Kyren 产品。
3. 购买套餐并跳转 Checkout。
4. 支付成功后收到 `order.paid` Webhook。
5. 本地订阅到账。
6. 重放同一 Webhook，确认不会重复发放权益。
7. 修改签名，确认 Webhook 被拒绝。

若没有 staging 凭据：

- 使用低金额 `CNY` live 产品做一次端到端验证；
- 验证后保留订单和 webhook payload，作为上线检查证据。

## 上线与迁移

1. 新增配置默认关闭，未配置 Kyren API Key 时前端不展示 Kyren 支付入口。
2. 数据库迁移只新增 `subscription_plans.kyren_product_id` 字段，默认空字符串。
3. 现有 Epay、Stripe、Creem、Waffo/Pancake 支付不受影响。
4. 管理员先配置 Kyren API Key、Webhook Secret 和 Base URL。
5. 管理员为主要订阅套餐一键创建或绑定 Kyren 产品。
6. 管理员配置若干 `CNY` 余额充值档位，并一键创建 Kyren 产品。
7. 配置 Kyren Webhook URL：`/api/kyren/webhook`。
8. 完成低金额真实支付或 staging 验证后，对用户开放 Kyren 支付入口。

## 待实现时确认

- Kyren `metadata` 在 `order.paid` Webhook 中是否完整透传。当前设计依赖 `metadata.trade_no`。
- Kyren `order.closed` 是否区分超时关闭和主动关闭；若不能区分，首版统一按 failed/expired 的一种本地状态处理。
- Kyren 产品更新后是否影响已经创建但未支付的 Checkout Session；如果不影响，后台同步按钮需提示「只影响后续新 Checkout」。
- Kyren 是否对产品数量、metadata 大小或 API 频率有限制；若有限制，需要在后台同步操作中增加更明确的错误提示。
