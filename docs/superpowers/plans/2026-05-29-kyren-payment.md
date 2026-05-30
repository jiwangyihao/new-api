# Kyren 支付接入实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 Kyren Pay 作为独立原生支付 provider 接入，支持 CNY 订阅套餐支付、CNY 固定档位余额充值、后台一键创建/同步 Kyren 产品和安全 Webhook 入账。

**架构：** 后端新增 Kyren 配置、client、产品同步、Checkout 和 Webhook 控制器；订阅订单在创建时保存 Kyren 产品/金额/币种支付快照，并同时冻结本次发放所需的套餐权益快照；充值订单在创建时保存充值档位快照。Webhook 只按订单快照完成校验和入账，不读取当前套餐或充值档位配置决定历史订单权益。前端新增 Kyren 管理入口、订阅购买入口、钱包充值固定档位入口和 i18n 文案。

**技术栈：** Go 1.22+、Gin、GORM、shopspring/decimal、React 19、TypeScript、React Query、React Hook Form、Zod、Bun、Vitest。

---

## 规格与硬约束

- 规格文件：`docs/superpowers/specs/2026-05-29-kyren-payment-design.md`。
- 本计划只实现 Kyren 原生 Checkout，不实现 Epay-compatible 主链路。
- 首版 Kyren 支付主路径只允许 `CNY`。
- 所有业务 JSON marshal/unmarshal 必须使用 `common.Marshal` / `common.Unmarshal` / `common.DecodeJson`，不得直接调用 `encoding/json` 序列化。
- 数据库兼容 SQLite、MySQL、PostgreSQL。
- 涉及 `web/default` 的实现必须遵守 `web/default/AGENTS.md`：用户文案 i18n、TypeScript 类型检查、Bun 优先。
- 使用 TDD：实现生产代码前先写测试，并运行目标测试确认失败。

## 子代理执行边界

- 后端任务 0 → 1 → 2 → 3 → 4 必须串行，或由同一个后端子代理连续执行；这些任务共享 `controller/kyren_client.go`、`controller/topup_kyren.go`、`controller/subscription_payment_kyren.go`、`router/api-router.go` 和支付常量。
- 任务 5 的后端部分依赖任务 4；任务 5 的前端部分依赖其后端 `topup info` 契约。任务 6 依赖任务 5 的用户侧 `enable_kyren_subscription` 契约和公开套餐 `kyren_product_id`。
- 前端任务 7 可与任务 6 并行，但必须独占 `payment-settings-section.tsx`、system-settings billing defaultValues 数据流和 Kyren 充值档位编辑器文件；任务 7 必须使用 Kyren 充值档位专用 API，不得通过通用 option 保存整段 `KyrenTopUpProducts`。
- 任务 8 必须最后执行，统一收集任务 5-7 实际新增的全部用户文案；全量 `bun run typecheck` 只由主代理在最终验证阶段执行，不作为 i18n 子代理任务内 gate。
- 并行开发时不要让两个子代理同时修改同一文件；如确需共享，指定一个主责代理，其他代理通过 IRC 或后续补丁协调。

## 文件结构

### 后端配置、模型与 provider

- 创建：`setting/payment_kyren.go` — Kyren 运行时配置变量。
- 创建：`controller/kyren_types.go` — Kyren API DTO、Webhook DTO、充值档位 DTO。
- 创建：`controller/kyren_client.go` — Kyren API client、BaseURL 校验、decimal 金额工具、配置校验 helper、测试注入点。
- 创建：`controller/topup_kyren.go` — 钱包 Kyren 支付、充值档位管理、充值档位 sync、Kyren Webhook。
- 创建：`controller/subscription_payment_kyren.go` — 订阅 Kyren 支付、套餐 Kyren 产品刷新/创建/同步。
- 修改：`model/option.go` — Kyren option 默认值和运行时加载。
- 修改：`controller/option.go` — Kyren option 保存前校验，避免空 secret 或无效配置落库。
- 修改：`model/subscription.go` — `KyrenProductId`、订阅订单 Kyren 支付快照、订阅权益快照、Kyren provider/method 常量。
- 修改：`model/topup.go` — 充值订单 Kyren 快照、`RechargeKyren` 或等价完成逻辑。
- 修改：`model/main.go` — SQLite 手写迁移和补列逻辑。
- 修改：`controller/subscription.go` — Admin create/update、公开套餐 DTO 返回 `kyren_product_id`。
- 修改：`controller/payment_webhook_availability.go` / `controller/topup.go` — 用户侧 Kyren 可用性和充值档位。
- 修改：`router/api-router.go` — Kyren webhook、用户支付和管理路由。

### 前端

- 订阅：`web/default/src/features/subscriptions/{types.ts,api.ts,lib/plan-form.ts,components/subscriptions-mutate-drawer.tsx,components/dialogs/subscription-purchase-dialog.tsx}`。
- 钱包：`web/default/src/features/wallet/{index.tsx,types.ts,api.ts,hooks/use-topup-info.ts,hooks/use-payment.ts,components/recharge-form-card.tsx,components/subscription-plans-card.tsx}`。
- 支付设置：`web/default/src/features/system-settings/types.ts`、`web/default/src/features/system-settings/billing/section-registry.tsx`、`web/default/src/features/system-settings/integrations/payment-settings-section.tsx`、新增 Kyren 充值档位编辑器和弹窗。
- i18n：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`。

---

## 任务 0：Kyren metadata 透传确认门禁

**文件：**

- 不修改生产代码。
- 如需记录证据，可在实现记录或 PR 描述中记录脱敏摘要。

- [x] **步骤 1：确认 metadata 透传**

通过以下任一方式确认 Kyren `metadata.trade_no` 和 `metadata.kind` 会完整出现在 `order.paid` Webhook：Kyren staging 凭据端到端测试、低金额 `CNY` live 订单、Kyren 官方文档或官方支持确认。可接受的文档证据：Kyren 官方文档的 `order.paid.data.metadata` 字段说明，配合 `POST /v1/checkouts` 的 `metadata` 参数说明。

- [x] **步骤 2：记录证据**

记录确认方式、日期和脱敏后的关键字段摘要。不得保存 raw webhook payload。

- [x] **步骤 3：门禁判断**

未确认 metadata 透传时，不进入任务 3/4 的原生 Checkout 实现；需要先回到规格，改为保存 Kyren checkout/session/order 可关联 ID 的设计。

---

## 任务 1：后端 Kyren 配置、类型和金额工具

**文件：**

- 创建：`setting/payment_kyren.go`
- 创建：`controller/kyren_types.go`
- 创建：`controller/kyren_client.go`
- 创建：`controller/kyren_client_test.go`
- 修改：`model/option.go`
- 修改：`controller/option.go`

- [x] **步骤 1：编写失败的配置和工具测试**

在 `controller/kyren_client_test.go` 新增：

```go
package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
)

func TestNormalizeKyrenBaseURL(t *testing.T) {
	got, err := normalizeKyrenBaseURL("https://api.kyren.top/")
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if got != "https://api.kyren.top" { t.Fatalf("expected trimmed production URL, got %q", got) }
}

func TestNormalizeKyrenBaseURLRejectsUntrustedHost(t *testing.T) {
	if _, err := normalizeKyrenBaseURL("https://evil.example.com"); err == nil { t.Fatal("expected untrusted host to be rejected") }
}

func TestFormatKyrenAmountCNY(t *testing.T) {
	got, err := formatKyrenAmountFromFloat(40)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if got != "40.00" { t.Fatalf("expected 40.00, got %q", got) }
}

func TestNormalizeKyrenAmountString(t *testing.T) {
	got, err := normalizeKyrenAmountString("9.9")
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if got != "9.90" { t.Fatalf("expected 9.90, got %q", got) }
}

func TestNormalizeKyrenTopUpProductsJSONRejectsInvalidProducts(t *testing.T) {
	invalid := `[{"id":"topup","name":"bad","amount":"0","currency":"USD","quota":0,"enabled":true}]`
	if _, err := normalizeKyrenTopUpProductsJSON(invalid); err == nil { t.Fatal("expected invalid top-up products to be rejected") }
}

func TestValidateKyrenOptionBeforePersistRejectsTopUpProducts(t *testing.T) {
	if _, persist, err := validateKyrenOptionBeforePersist("KyrenTopUpProducts", "[]"); err == nil || persist { t.Fatalf("expected KyrenTopUpProducts to require the versioned API, persist=%v err=%v", persist, err) }
}

func TestApplyKyrenRuntimeOptionDoesNotOverwriteSecretWithEmptyValue(t *testing.T) {
	old := setting.KyrenApiKey
	defer func() { setting.KyrenApiKey = old }()
	setting.KyrenApiKey = "kyren_live_existing"
	if err := applyKyrenRuntimeOption("KyrenApiKey", ""); err != nil { t.Fatalf("unexpected error: %v", err) }
	if setting.KyrenApiKey != "kyren_live_existing" { t.Fatalf("empty key overwrote runtime value: %q", setting.KyrenApiKey) }
}
```

- [x] **步骤 2：运行测试验证失败**

```bash
go test ./controller -run 'TestNormalizeKyrenBaseURL|TestFormatKyrenAmountCNY|TestNormalizeKyrenAmountString|TestNormalizeKyrenTopUpProductsJSON|TestValidateKyrenOptionBeforePersistRejectsTopUpProducts|TestApplyKyrenRuntimeOption' -count=1
```

预期：FAIL，错误包含 `undefined: normalizeKyrenBaseURL`、`undefined: normalizeKyrenTopUpProductsJSON`、`undefined: validateKyrenOptionBeforePersist` 或 `undefined: applyKyrenRuntimeOption`。

- [x] **步骤 3：实现 Kyren 配置变量**

创建 `setting/payment_kyren.go`：

```go
package setting

var KyrenApiKey = ""
var KyrenWebhookSecret = ""
var KyrenBaseURL = "https://api.kyren.top"
var KyrenTopUpProducts = "[]"
```

- [x] **步骤 4：实现 Kyren 类型和工具**

在 `controller/kyren_types.go` 定义 DTO：

```go
package controller

type kyrenAPIResponse[T any] struct { Code int `json:"code"`; Message string `json:"message"`; Data T `json:"data"` }

type kyrenProduct struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Description string `json:"description"`
	Image string `json:"image"`
	Price string `json:"price"`
	Currency string `json:"currency"`
	Status string `json:"status"`
	Metadata map[string]string `json:"metadata"`
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

type kyrenCreateProductRequest struct { Name string `json:"name"`; Description string `json:"description,omitempty"`; Image string `json:"image,omitempty"`; Price string `json:"price"`; Currency string `json:"currency,omitempty"`; Metadata map[string]string `json:"metadata,omitempty"` }
type kyrenUpdateProductRequest struct { Name string `json:"name,omitempty"`; Description string `json:"description,omitempty"`; Image string `json:"image,omitempty"`; Price string `json:"price,omitempty"`; Currency string `json:"currency,omitempty"`; Metadata map[string]string `json:"metadata,omitempty"` }
type kyrenCreateCheckoutRequest struct { ProductID string `json:"productId"`; SuccessURL string `json:"successUrl"`; CancelURL string `json:"cancelUrl,omitempty"`; CustomerEmail string `json:"customerEmail,omitempty"`; CustomerName string `json:"customerName,omitempty"`; Metadata map[string]string `json:"metadata,omitempty"` }
type kyrenCheckoutSession struct { ID string `json:"id"`; URL string `json:"url"`; Status string `json:"status"`; ExpiresAt int64 `json:"expiresAt"` }

type kyrenTopUpProduct struct { ID string `json:"id"`; Name string `json:"name"`; Description string `json:"description,omitempty"`; ProductID string `json:"product_id,omitempty"`; Amount string `json:"amount"`; Currency string `json:"currency"`; Quota int64 `json:"quota"`; Enabled bool `json:"enabled"` }
```

在 `controller/kyren_client.go` 实现金额和配置 helper。必须使用 `github.com/shopspring/decimal`，JSON 使用 `common.Unmarshal` / `common.Marshal`：

```go
const (
	kyrenDefaultBaseURL = "https://api.kyren.top"
	kyrenStagingBaseURL = "https://staging-api.kyren.top"
	kyrenCurrencyCNY    = "CNY"
)

func normalizeKyrenBaseURL(raw string) (string, error) { /* HTTPS + allowlist + trim */ }
func normalizeKyrenAmountString(raw string) (string, error) { /* decimal >= 0.01 -> 2 decimals */ }
func formatKyrenAmountFromFloat(raw float64) (string, error) { /* decimal -> 2 decimals */ }
func kyrenDecimalEqual(a string, b string) bool { /* decimal Round(2).Equal */ }
func normalizeKyrenTopUpProductsJSON(raw string) (string, error) { /* parse []kyrenTopUpProduct, validate, normalize amount, marshal */ }
```

- [x] **步骤 5：接入 option 默认值、运行时加载和保存前校验**

修改 `model/option.go` 的 `InitOptionMap`：

```go
common.OptionMap["KyrenApiKey"] = setting.KyrenApiKey
common.OptionMap["KyrenWebhookSecret"] = setting.KyrenWebhookSecret
common.OptionMap["KyrenBaseURL"] = setting.KyrenBaseURL
common.OptionMap["KyrenTopUpProducts"] = setting.KyrenTopUpProducts
```

实现并使用保存前校验：

```go
func validateKyrenOptionBeforePersist(key string, value string) (normalized string, persist bool, err error) {
	switch key {
	case "KyrenApiKey", "KyrenWebhookSecret":
		trimmed := strings.TrimSpace(value)
		if trimmed == "" { return "", false, nil }
		return trimmed, true, nil
	case "KyrenBaseURL":
		normalized, err := normalizeKyrenBaseURL(value)
		return normalized, err == nil, err
	case "KyrenTopUpProducts":
		return "", false, errors.New("KyrenTopUpProducts must be updated via /api/payment/kyren/topup-products")
	default:
		return value, true, nil
	}
}
```

修改 `controller/option.go` 的 `UpdateOption`：在调用 `model.UpdateOption` 前拦截 Kyren key，调用 `validateKyrenOptionBeforePersist`；`persist=false` 时直接返回 success，不写库；`persist=true` 时用 normalized value 调用 `model.UpdateOption`。`KyrenTopUpProducts` 必须返回业务错误或 HTTP 400，不允许通过通用 option 更新接口持久化，避免绕过任务 3 的 version/CAS 专用接口。

实现 `applyKyrenRuntimeOption(key, value string) error`，并由 `model.updateOptionMap` 的 Kyren 分支调用。`applyKyrenRuntimeOption` 只接收已校验值：密钥类空字符串保持运行时旧值不变；`KyrenBaseURL` 写入前调用 `normalizeKyrenBaseURL`；`KyrenTopUpProducts` 只在启动加载、定时同步或专用 topup-products 接口成功保存后进入运行时，写入前调用 `normalizeKyrenTopUpProductsJSON`。如果为避免循环依赖不能把 helper 放在 controller，则把 Kyren 校验、规范化和 runtime apply helper 放到不依赖 controller/model 的新包或 `setting` 包，并同步调整测试包名；不得让无效 BaseURL 或畸形 topup products 写入运行时。

- [x] **步骤 6：运行测试验证通过**

```bash
go test ./controller -run 'TestNormalizeKyrenBaseURL|TestFormatKyrenAmountCNY|TestNormalizeKyrenAmountString|TestNormalizeKyrenTopUpProductsJSON|TestValidateKyrenOptionBeforePersistRejectsTopUpProducts|TestApplyKyrenRuntimeOption' -count=1
```

预期：PASS。

- [x] **步骤 7：Commit**

```bash
git add setting/payment_kyren.go controller/kyren_types.go controller/kyren_client.go controller/kyren_client_test.go controller/option.go model/option.go
git commit -m "feat(payment): 添加 Kyren 基础配置和工具"
```

---

## 任务 2：后端模型迁移、支付常量和 Kyren 订单快照

**文件：**

- 修改：`model/subscription.go`
- 修改：`model/topup.go`
- 修改：`model/main.go`
- 修改：`model/subscription_distributor_test.go`
- 修改：`router/subscription_public_plans_route_test.go`
- 修改：`controller/subscription.go`
- 修改：`controller/subscription_admin_plan_fields_test.go`
- 创建或修改：`model/kyren_payment_test.go`

- [x] **步骤 1：编写失败的套餐字段和公开 DTO 测试**

修改 `controller/subscription_admin_plan_fields_test.go`，在 admin create/update 字段覆盖测试中加入 `KyrenProductId: "prod_kyren_plan"`，断言数据库读取值保持一致。扩展公开套餐路由测试 `router/subscription_public_plans_route_test.go`，断言 `/api/subscription/public/plans` 返回 `kyren_product_id`，并更新严格字段白名单。

- [x] **步骤 2：编写失败的快照和常量测试**

创建 `model/kyren_payment_test.go`：

```go
func TestKyrenPaymentSnapshotRoundTrip(t *testing.T) {
	snapshot := KyrenPaymentSnapshot{ProductID: "prod_sub", Amount: "40.00", Currency: "CNY"}
	payload, err := MarshalKyrenPaymentSnapshot(snapshot)
	if err != nil { t.Fatalf("marshal snapshot: %v", err) }
	got, err := UnmarshalKyrenPaymentSnapshot(payload)
	if err != nil { t.Fatalf("unmarshal snapshot: %v", err) }
	if got.ProductID != snapshot.ProductID || got.Amount != snapshot.Amount || got.Currency != snapshot.Currency { t.Fatalf("snapshot mismatch: %#v", got) }
}

func TestKyrenSubscriptionEntitlementSnapshotRoundTrip(t *testing.T) {
	plan := SubscriptionPlan{Id: 1001, TotalAmount: 100000, MonthlyTokenLimit: 2000, ConcurrencyLimit: 3, QueueCapacity: 9, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetMonthly, MaxPurchasePerUser: 2}
	snapshot := NewSubscriptionEntitlementSnapshotFromPlan(&plan)
	payload, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	if err != nil { t.Fatalf("marshal entitlement snapshot: %v", err) }
	got, err := UnmarshalSubscriptionEntitlementSnapshot(payload)
	if err != nil { t.Fatalf("unmarshal entitlement snapshot: %v", err) }
	if got.QueueCapacity != 9 || got.ConcurrencyLimit != 3 || got.MonthlyTokenLimit != 2000 || got.DurationUnit != SubscriptionDurationMonth { t.Fatalf("entitlement snapshot mismatch: %#v", got) }
}

func TestKyrenPaymentConstants(t *testing.T) {
	if PaymentProviderKyren != "kyren" || PaymentMethodKyren != "kyren" { t.Fatalf("unexpected kyren constants: %q %q", PaymentProviderKyren, PaymentMethodKyren) }
}
```

- [x] **步骤 3：运行测试验证失败**

```bash
go test ./model ./controller ./router -run 'TestKyrenPaymentSnapshotRoundTrip|TestKyrenSubscriptionEntitlementSnapshotRoundTrip|TestKyrenPaymentConstants|KyrenProduct|TestEnsureSubscriptionPlanTableSQLite|TestSubscriptionPlansPublicRoute' -count=1
```

预期：FAIL，错误包含 `undefined: KyrenPaymentSnapshot`、`undefined: SubscriptionEntitlementSnapshot`、`undefined: PaymentProviderKyren`、`unknown field KyrenProductId`，或 SQLite 迁移缺列断言失败。

- [x] **步骤 4：新增模型字段、常量和快照 helper**

- `SubscriptionPlan` 新增 `KyrenProductId string`。
- `SubscriptionOrder` 新增 `KyrenSnapshot string` 和 `EntitlementSnapshot string`。`KyrenSnapshot` 保存 Kyren `product_id`、`amount`、`currency`；`EntitlementSnapshot` 保存本次订阅发放所需的套餐权益字段。
- `TopUp` 新增 `KyrenSnapshot string`。
- 新增 `PaymentProviderKyren = "kyren"` 和 `PaymentMethodKyren = "kyren"`。
- 新增 `model/kyren_payment.go`，使用 `common.Marshal` / `common.Unmarshal` 实现 `KyrenPaymentSnapshot` round-trip。
- 新增 `SubscriptionEntitlementSnapshot` 及 `NewSubscriptionEntitlementSnapshotFromPlan`、`MarshalSubscriptionEntitlementSnapshot`、`UnmarshalSubscriptionEntitlementSnapshot`。快照字段至少包括 `plan_id`、`total_amount`、`monthly_token_limit`、`concurrency_limit`、`queue_capacity`、`duration_unit`、`duration_value`、`custom_seconds`、`quota_reset_period`、`quota_reset_custom_seconds`、`max_purchase_per_user`、`business_code`。Kyren 订阅 Webhook 完成路径必须使用该快照发放权益，不得用当前 `SubscriptionPlan` 的可变权益字段决定历史订单。

- [x] **步骤 5：更新 admin create/update 和公开套餐 DTO**

- `AdminUpdateSubscriptionPlan` 的 `updateMap` 增加 `kyren_product_id`。
- 公开套餐 DTO / 转换函数增加 `kyren_product_id`。
- 管理端 DTO 如有手写白名单，也增加 `kyren_product_id`。

- [x] **步骤 6：更新 SQLite 迁移并补迁移测试**

- SQLite 订阅套餐手写建表 SQL 增加 `kyren_product_id varchar(128) DEFAULT ''`。
- required columns / add column 逻辑增加 `kyren_product_id`。
- 如新增 `kyren_snapshot` / `entitlement_snapshot` 物理列且对应表有手写补列逻辑，则同步增加 `TEXT DEFAULT ''`。
- 扩展 `model/subscription_distributor_test.go` 的 SQLite 迁移测试，覆盖 fresh table 和 legacy table：调用 `ensureSubscriptionPlanTableSQLite()` 后，断言 `subscription_plans.kyren_product_id` 存在且默认值为空字符串；legacy table 场景先手写创建只包含旧列的 `subscription_plans`，再调用迁移并断言补列成功。新增快照列迁移断言：`subscription_orders.kyren_snapshot`、`subscription_orders.entitlement_snapshot`、`top_ups.kyren_snapshot` 在 SQLite AutoMigrate 后存在且可写入。

- [x] **步骤 7：运行测试验证通过**

```bash
go test ./model ./controller ./router -run 'TestKyrenPaymentSnapshotRoundTrip|TestKyrenSubscriptionEntitlementSnapshotRoundTrip|TestKyrenPaymentConstants|KyrenProduct|TestEnsureSubscriptionPlanTableSQLite|TestSubscriptionPlansPublicRoute' -count=1
```

预期：PASS。


- [x] **步骤 8：Commit**

```bash
git add model/subscription.go model/topup.go model/main.go model/kyren_payment.go model/kyren_payment_test.go model/subscription_distributor_test.go controller/subscription.go controller/subscription_admin_plan_fields_test.go router/subscription_public_plans_route_test.go
git commit -m "feat(payment): 增加 Kyren 订单快照和套餐绑定字段"
```

---

## 任务 3：Kyren client 外部 API 和产品同步接口

**文件：**

- 修改：`controller/kyren_client.go`
- 修改：`controller/kyren_client_test.go`
- 创建：`controller/kyren_products_test.go`
- 创建或修改：`controller/topup_kyren.go`
- 创建或修改：`controller/subscription_payment_kyren.go`
- 修改：`router/api-router.go`

- [x] **步骤 1：编写失败的 Kyren client HTTP 测试**

在 `controller/kyren_client_test.go` 增加 `TestKyrenClientCreateProductUsesAPIKey`，用 `httptest.NewServer` 验证 `x-api-key` 请求头、`/v1/products` path 和响应解析。直接构造 `kyrenClient{baseURL: server.URL, apiKey: "kyren_live_test", httpClient: server.Client()}`，不要通过生产 BaseURL allowlist。

- [x] **步骤 2：运行测试验证失败**

```bash
go test ./controller -run TestKyrenClientCreateProductUsesAPIKey -count=1
```

预期：FAIL，错误包含 `undefined: kyrenClient` 或 `createProduct undefined`。

- [x] **步骤 3：实现 Kyren HTTP client 和测试注入点**

实现 `kyrenAPI` interface、`kyrenClient`、`newKyrenClient()`、`newKyrenClientForController` 注入点，以及 `createProduct`、`updateProduct`、`retrieveProduct`、`listProducts`、`createCheckout`。生产 `newKyrenClient()` 必须调用 `normalizeKyrenBaseURL(setting.KyrenBaseURL)`；handler 测试替换 `newKyrenClientForController` 为 fake client。

- [x] **步骤 4：编写失败的产品同步和 option 并发测试**

创建 `controller/kyren_products_test.go`，至少包含：

```go
func TestAdminSyncSubscriptionKyrenProductCreatesAndBindsProduct(t *testing.T) { /* fake client CreateProduct -> prod_sub，断言 plan.KyrenProductId 回填 */ }
func TestAdminSyncSubscriptionKyrenProductReturnsProductIDWhenLocalBindFails(t *testing.T) { /* fake client CreateProduct 成功后强制 DB 回填失败，断言响应包含 product_id 和可重试错误 */ }
func TestAdminSyncSubscriptionKyrenProductReusesMetadataMatchedProduct(t *testing.T) { /* fake ListProducts 返回 source/kind/plan_id 匹配产品，断言不会再次调用 CreateProduct */ }
func TestAdminUpdateKyrenTopUpProductsRejectsStaleVersion(t *testing.T) { /* GET 返回 version，PUT 携带过期 version 返回 409，数据库原值未被覆盖 */ }
func TestAdminSyncSubscriptionKyrenProductWritesManageLog(t *testing.T) { /* 订阅产品 create/update 成功后写入 LogTypeManage，Other.admin_info 含管理员信息 */ }
func TestAdminUpdateKyrenTopUpProductsWritesManageLog(t *testing.T) { /* 充值档位 PUT 成功后写入 LogTypeManage */ }
func TestAdminSyncKyrenTopUpProductMergesLatestOptionValue(t *testing.T) { /* sync A 只更新 A.product_id，保留 B 最新修改 */ }
func TestAdminSyncKyrenTopUpProductUpdatesExistingProduct(t *testing.T) { /* 已绑定 product_id 时 retrieve ACTIVE 后 update 远端 name/description/price/currency/metadata，只合并目标档位 */ }
func TestAdminSyncKyrenTopUpProductReturnsProductIDWhenOptionSaveFails(t *testing.T) { /* fake client CreateProduct 成功后强制 option 保存失败，断言响应包含 product_id */ }
func TestAdminSyncKyrenTopUpProductReusesMetadataMatchedProduct(t *testing.T) { /* fake ListProducts 返回 source/kind/topup_product_id 匹配产品，断言不会再次调用 CreateProduct */ }
func TestAdminSyncKyrenTopUpProductWritesManageLog(t *testing.T) { /* 充值档位 sync 成功后写入 LogTypeManage */ }
func TestAdminSyncKyrenTopUpProductReturnsLatestProductsAndVersion(t *testing.T) { /* sync 成功响应包含最新 products/version/product_id，前端可直接更新 CAS 状态 */ }
```

- [x] **步骤 5：运行测试验证失败**

```bash
go test ./controller -run 'TestKyrenClientCreateProductUsesAPIKey|TestAdminSyncSubscriptionKyrenProductCreatesAndBindsProduct|TestAdminSyncSubscriptionKyrenProductReturnsProductIDWhenLocalBindFails|TestAdminSyncSubscriptionKyrenProductReusesMetadataMatchedProduct|TestAdminUpdateKyrenTopUpProductsRejectsStaleVersion|TestAdminSyncSubscriptionKyrenProductWritesManageLog|TestAdminUpdateKyrenTopUpProductsWritesManageLog|TestAdminSyncKyrenTopUpProductMergesLatestOptionValue|TestAdminSyncKyrenTopUpProductUpdatesExistingProduct|TestAdminSyncKyrenTopUpProductReturnsProductIDWhenOptionSaveFails|TestAdminSyncKyrenTopUpProductReusesMetadataMatchedProduct|TestAdminSyncKyrenTopUpProductWritesManageLog|TestAdminSyncKyrenTopUpProductReturnsLatestProductsAndVersion' -count=1
```

预期：client 测试 PASS，同步接口测试 FAIL，错误包含 handler undefined、冲突检测缺失、补偿响应缺失或 metadata 复用缺失。


- [x] **步骤 6：实现产品查询和 sync handler**

- `AdminGetSubscriptionKyrenProduct` / `AdminSyncSubscriptionKyrenProduct`：校验 plan、CNY、价格；`create_or_update` 非空时 retrieve+update，404 返回错误；`create_new` 前先按 metadata（`source=new-api`、`kind=subscription_plan`、`plan_id`）查询并复用匹配产品，未找到才创建；创建成功但本地回填失败时，响应必须包含新 `product_id` 和错误原因，供管理员手动绑定或安全重试；metadata 包含 `source=new-api`、`kind=subscription_plan`、`plan_id`、`business_code`；订阅产品 create/update 成功必须写入 `model.LogTypeManage`，`Other.admin_info` 包含管理员 ID/用户名等受控信息。
- `AdminListKyrenProducts` / `AdminGetKyrenProduct`：只读查询。
- `AdminListKyrenTopUpProducts` 返回 `{ products, version }`；`AdminGetKyrenTopUpProductStatus` 返回单档远端状态和最新 version，响应包含 `product_id`、`status`、`price`、`currency`、`price_matches`、`currency_matches`、`version`；`AdminUpdateKyrenTopUpProducts` 请求必须包含 `products` 和 `version`，后端基于数据库中最新 `options.KyrenTopUpProducts` 计算 version（推荐 canonical JSON hash 或 `updated_at`），version 不匹配返回 HTTP 409 和可重试错误，不覆盖原值；保存前仍调用 `normalizeKyrenTopUpProductsJSON`；保存成功必须写入 `model.LogTypeManage`。
- `AdminSyncKyrenTopUpProduct`：使用 `common.Unmarshal`；在事务行锁下读取最新 option JSON，只合并目标档位 `product_id` / 同步状态；请求支持 `mode=create_or_update|create_new`，已有 `product_id` 且 `mode=create_or_update` 时必须 retrieve ACTIVE 产品并 update 远端 `name`、`description`、`price`、`currency`、`metadata`，404/ARCHIVED 返回错误不创建新产品；`mode=create_new` 或未绑定时，先按 metadata（`source=new-api`、`kind=wallet_topup`、`topup_product_id`）查询并复用匹配产品，未找到才创建；Kyren 创建成功但 option 回写失败时，响应必须包含新 `product_id` 和错误原因；sync 成功响应必须包含最新 `{ products, version, product_id }`，供前端直接更新 CAS 状态；不得覆盖同一时间由其他管理员修改的其他档位内容；sync 成功必须写入 `model.LogTypeManage`。

- [x] **步骤 7：增加路由**

```go
apiRouter.GET("/payment/kyren/products", middleware.AdminAuth(), controller.AdminListKyrenProducts)
apiRouter.GET("/payment/kyren/products/:id", middleware.AdminAuth(), controller.AdminGetKyrenProduct)
apiRouter.GET("/payment/kyren/topup-products", middleware.AdminAuth(), controller.AdminListKyrenTopUpProducts)
apiRouter.GET("/payment/kyren/topup-products/:id/status", middleware.AdminAuth(), controller.AdminGetKyrenTopUpProductStatus)
apiRouter.PUT("/payment/kyren/topup-products", middleware.AdminAuth(), middleware.CriticalRateLimit(), controller.AdminUpdateKyrenTopUpProducts)
apiRouter.POST("/payment/kyren/topup-products/:id/sync", middleware.AdminAuth(), middleware.CriticalRateLimit(), controller.AdminSyncKyrenTopUpProduct)
subscriptionAdminRoute.GET("/plans/:id/kyren/product", controller.AdminGetSubscriptionKyrenProduct)
subscriptionAdminRoute.POST("/plans/:id/kyren/product", middleware.CriticalRateLimit(), controller.AdminSyncSubscriptionKyrenProduct)
```

- [x] **步骤 8：运行测试验证通过**

```bash
go test ./controller -run 'TestKyrenClientCreateProductUsesAPIKey|TestAdminSyncSubscriptionKyrenProductCreatesAndBindsProduct|TestAdminSyncSubscriptionKyrenProductReturnsProductIDWhenLocalBindFails|TestAdminSyncSubscriptionKyrenProductReusesMetadataMatchedProduct|TestAdminUpdateKyrenTopUpProductsRejectsStaleVersion|TestAdminSyncSubscriptionKyrenProductWritesManageLog|TestAdminUpdateKyrenTopUpProductsWritesManageLog|TestAdminSyncKyrenTopUpProductMergesLatestOptionValue|TestAdminSyncKyrenTopUpProductUpdatesExistingProduct|TestAdminSyncKyrenTopUpProductReturnsProductIDWhenOptionSaveFails|TestAdminSyncKyrenTopUpProductReusesMetadataMatchedProduct|TestAdminSyncKyrenTopUpProductWritesManageLog|TestAdminSyncKyrenTopUpProductReturnsLatestProductsAndVersion' -count=1
```

预期：PASS。


- [x] **步骤 9：Commit**

```bash
git add controller/kyren_client.go controller/kyren_client_test.go controller/kyren_products_test.go controller/topup_kyren.go controller/subscription_payment_kyren.go router/api-router.go
git commit -m "feat(payment): 添加 Kyren 产品同步接口"
```

---

## 任务 4：Kyren Checkout 和 Webhook 入账

**文件：**

- 修改：`controller/topup_kyren.go`
- 修改：`controller/subscription_payment_kyren.go`
- 修改：`controller/subscription_payment_completion.go`
- 修改：`router/api-router.go`
- 创建：`controller/topup_kyren_test.go`
- 创建：`controller/subscription_payment_kyren_test.go`

- [x] **步骤 1：编写失败的 Webhook 和 Checkout 测试**

新增以下测试：

```go
func TestVerifyKyrenWebhookSignature(t *testing.T) { /* 正确签名通过 */ }
func TestVerifyKyrenWebhookSignatureRejectsEmptySecret(t *testing.T) { /* KyrenWebhookSecret 为空时拒绝验签 */ }
func TestVerifyKyrenWebhookSignatureRejectsTamperedBody(t *testing.T) { /* 篡改 body 拒绝 */ }
func TestVerifyKyrenWebhookSignatureRejectsExpiredTimestamp(t *testing.T) { /* 10 分钟前 timestamp 拒绝 */ }
func TestKyrenWebhookCompletesSubscriptionOrder(t *testing.T) { /* pending Kyren subscription -> success，重复投递幂等 */ }
func TestKyrenWebhookCompletesSubscriptionOrderUsingEntitlementSnapshot(t *testing.T) { /* 创建 Checkout 后修改 plan 权益，回调仍按 order.EntitlementSnapshot 发放 */ }
func TestKyrenWebhookCompletesTopUpUsingSnapshot(t *testing.T) { /* 档位后续改绑，仍按 TopUp 快照到账 */ }
func TestKyrenWebhookMissingMetadataDoesNotFulfillOrder(t *testing.T) { /* 缺 trade_no/kind 返回 200 不发放 */ }
func TestKyrenWebhookRejectsProviderMismatch(t *testing.T) { /* provider=stripe 的订单不会被 Kyren 完成 */ }
func TestKyrenWebhookRejectsAmountCurrencyOrProductMismatch(t *testing.T) { /* amount/currency/product 任一不匹配不完成 */ }
func TestKyrenWebhookClosedOnlyExpiresPendingOrder(t *testing.T) { /* closed 只影响 pending，success 不变 */ }
func TestKyrenWebhookRefundedRecordsManualActionAndReturnsSuccess(t *testing.T) { /* refunded 返回 200，不撤权 */ }
func TestKyrenCheckoutFailureFinalizesPendingOrder(t *testing.T) { /* createCheckout 失败后不遗留 pending */ }
func TestKyrenCheckoutRejectsArchivedOrMismatchedProductBeforeOrderCreation(t *testing.T) { /* ARCHIVED/价格币种不匹配不创建 pending */ }
func TestKyrenPayRejectsMissingWebhookSecret(t *testing.T) { /* API Key 存在但 Webhook Secret 缺失时拒绝创建订阅/充值 pending 订单 */ }
```

- [x] **步骤 2：运行测试验证失败**

```bash
go test ./controller -run 'TestVerifyKyrenWebhookSignature|TestKyrenWebhookCompletesSubscriptionOrder|TestKyrenWebhookCompletesSubscriptionOrderUsingEntitlementSnapshot|TestKyrenWebhookCompletesTopUpUsingSnapshot|TestKyrenWebhookMissingMetadata|TestKyrenWebhookRejectsProviderMismatch|TestKyrenWebhookRejectsAmountCurrencyOrProductMismatch|TestKyrenWebhookClosedOnlyExpiresPendingOrder|TestKyrenWebhookRefundedRecordsManualActionAndReturnsSuccess|TestKyrenCheckoutFailureFinalizesPendingOrder|TestKyrenCheckoutRejectsArchivedOrMismatchedProductBeforeOrderCreation|TestKyrenPayRejectsMissingWebhookSecret' -count=1
```

预期：FAIL，错误包含 `undefined: verifyKyrenWebhookSignature` 或 handler 未实现。

- [x] **步骤 3：实现签名、订阅支付、充值支付和 Webhook**

- `verifyKyrenWebhookSignature`：`KyrenWebhookSecret` 为空时直接拒绝；timestamp 是 Unix milliseconds；校验 5 分钟容忍；HMAC-SHA256(`timestamp + "." + raw_body`)；签名格式 `sha256=<hex>`；使用 `hmac.Equal`。
- `SubscriptionRequestKyrenPay`：校验 user、plan、CNY、`KyrenProductId`、`KyrenApiKey` 和 `KyrenWebhookSecret` 均已配置；缺少 API Key 或 Webhook Secret 时不得创建 pending 订单；retrieve product ACTIVE 且 price/currency 匹配；创建 pending 订单并保存 Kyren payment snapshot 和 entitlement snapshot；createCheckout 失败则终态化 pending；返回 checkout_url。
- `RequestKyrenPay`：请求 `product_id` 是本地档位 ID；校验档位 enabled/CNY/product/amount/quota，且 `KyrenApiKey` 与 `KyrenWebhookSecret` 均已配置；缺少任一密钥时不得创建 pending 订单；retrieve product ACTIVE 且 price/currency 匹配；创建 TopUp pending 和 snapshot；createCheckout 失败则终态化 pending；返回 checkout_url。
- `KyrenWebhook`：raw body 验签；metadata 缺失返回 2xx 不发放；subscription/topup provider guard；读取 snapshot 校验 product/amount/currency；订阅完成时调用 Kyren 专用完成函数（如 `completeKyrenSubscriptionOrderWithSnapshotAndEvaluateInvitation`），该函数在事务中锁定订单、解析 `SubscriptionOrder.EntitlementSnapshot`、用快照构造发放用 `SubscriptionPlan` 调用 `CreateUserSubscriptionFromPlanTx`，更新订单状态和受控 provider payload，并继续触发 `service.TryEnsureInvitationEntitlementForPaidUser`；不得直接调用会重新读取当前套餐的 `completeSubscriptionOrderAndEvaluateInvitation` / `model.CompleteSubscriptionOrder` 完成 Kyren 订阅；`order.closed` 只影响 pending；`order.refunded` 记录人工处理并返回 2xx；保存受控摘要，不保存 raw payload。

- [x] **步骤 4：增加路由**

```go
apiRouter.POST("/kyren/webhook", controller.KyrenWebhook)
selfRoute.POST("/kyren/pay", middleware.CriticalRateLimit(), controller.RequestKyrenPay)
subscriptionRoute.POST("/kyren/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestKyrenPay)
```

- [x] **步骤 5：运行测试验证通过**

```bash
go test ./controller -run 'TestVerifyKyrenWebhookSignature|TestKyrenWebhookCompletesSubscriptionOrder|TestKyrenWebhookCompletesSubscriptionOrderUsingEntitlementSnapshot|TestKyrenWebhookCompletesTopUpUsingSnapshot|TestKyrenWebhookMissingMetadata|TestKyrenWebhookRejectsProviderMismatch|TestKyrenWebhookRejectsAmountCurrencyOrProductMismatch|TestKyrenWebhookClosedOnlyExpiresPendingOrder|TestKyrenWebhookRefundedRecordsManualActionAndReturnsSuccess|TestKyrenCheckoutFailureFinalizesPendingOrder|TestKyrenCheckoutRejectsArchivedOrMismatchedProductBeforeOrderCreation|TestKyrenPayRejectsMissingWebhookSecret' -count=1
```

预期：PASS。


- [x] **步骤 6：Commit**

```bash
git add controller/topup_kyren.go controller/subscription_payment_kyren.go controller/subscription_payment_completion.go controller/topup_kyren_test.go controller/subscription_payment_kyren_test.go router/api-router.go
git commit -m "feat(payment): 接入 Kyren Checkout 和 Webhook"
```

---

## 任务 5：用户侧 Kyren 可用性和钱包充值接口

**文件：**

- 修改：`controller/topup.go`
- 修改：`controller/payment_webhook_availability.go`
- 修改：`web/default/src/features/wallet/index.tsx`
- 修改：`web/default/src/features/wallet/types.ts`
- 修改：`web/default/src/features/wallet/api.ts`
- 修改：`web/default/src/features/wallet/hooks/use-topup-info.ts`
- 修改：`web/default/src/features/wallet/hooks/use-payment.ts`
- 修改：`web/default/src/features/wallet/components/recharge-form-card.tsx`
- 修改或创建：`web/default/src/features/wallet/lib/payment.ts`
- 修改：`web/default/src/features/wallet/wallet-layout.test.ts`

- [x] **步骤 1：编写失败的后端 topup info 测试**

新增 `TestGetTopUpInfoIncludesKyrenProducts`：设置 Kyren 配置和 enabled CNY 档位，调用 `GetTopUpInfo`，断言 `enable_kyren_topup=true`，`kyren_topup_products` 只包含本地 id/name/amount/currency/quota，不包含 Kyren `prod_xxx`。

- [x] **步骤 2：运行后端测试验证失败**

```bash
go test ./controller -run TestGetTopUpInfoIncludesKyrenProducts -count=1
```

预期：FAIL，响应缺 Kyren 字段。

- [x] **步骤 3：实现 topup info Kyren 字段**

- 增加 `enable_kyren_topup`、`enable_kyren_subscription`、`kyren_topup_products`。
- `enable_kyren_topup` / `enable_kyren_subscription` 只有在 `KyrenApiKey` 和 `KyrenWebhookSecret` 都已配置时才为 true；缺少 Webhook Secret 时不展示用户侧入口，避免创建无法验签入账的 Checkout。
- 只返回 enabled、CNY、product_id 非空、amount 有效的本地档位。
- 不返回 Kyren `prod_xxx`。

- [x] **步骤 4：运行后端测试验证通过**

```bash
go test ./controller -run TestGetTopUpInfoIncludesKyrenProducts -count=1
```

预期：PASS。

- [x] **步骤 5：编写失败的前端钱包测试**

在 `wallet-layout.test.ts` 或新测试文件中测试真实导出 helper：

```ts
test('submits Kyren payment with local top-up product id', async () => {
  const requestKyrenPayment = vi.fn().mockResolvedValue({ success: true, data: { checkout_url: 'https://checkout.example/kyren' } })
  const openCheckout = vi.fn()

  await processKyrenTopUpProductPayment({ productId: 'topup_cny_10', requestKyrenPayment, openCheckout })

  expect(requestKyrenPayment).toHaveBeenCalledWith({ product_id: 'topup_cny_10' })
  expect(openCheckout).toHaveBeenCalledWith('https://checkout.example/kyren')
})
```

- [x] **步骤 6：运行前端测试验证失败**

```bash
bun test src/features/wallet/wallet-layout.test.ts
```

工作目录：`web/default`。预期：FAIL，缺少 `processKyrenTopUpProductPayment` 或类型。

- [x] **步骤 7：实现前端钱包 Kyren 类型、API 和页面链路**

- `types.ts` 增加 `KyrenTopUpProduct`、`enable_kyren_topup`、`enable_kyren_subscription`、`kyren_topup_products`。
- `api.ts` 增加 `requestKyrenPayment({ product_id })`。
- 新增 `processKyrenTopUpProductPayment` helper，传本地档位 ID，成功后打开 checkout_url。
- `wallet/index.tsx` 增加 `selectedKyrenTopUpProduct` 状态，将档位和回调传给 `RechargeFormCard`。
- `recharge-form-card.tsx` 展示 Kyren 固定档位列表，文案使用 `t()`。

- [x] **步骤 8：运行前端测试验证通过**

```bash
bun test src/features/wallet/wallet-layout.test.ts
```

工作目录：`web/default`。预期：PASS。

- [x] **步骤 9：Commit**

```bash
git add controller/topup.go controller/payment_webhook_availability.go web/default/src/features/wallet/index.tsx web/default/src/features/wallet/types.ts web/default/src/features/wallet/api.ts web/default/src/features/wallet/hooks/use-topup-info.ts web/default/src/features/wallet/hooks/use-payment.ts web/default/src/features/wallet/components/recharge-form-card.tsx web/default/src/features/wallet/lib/payment.ts web/default/src/features/wallet/wallet-layout.test.ts
git commit -m "feat(payment): 暴露 Kyren 用户侧充值入口"
```

---

## 任务 6：前端订阅套餐 Kyren 管理与购买

**文件：**

- 修改：`web/default/src/features/subscriptions/types.ts`
- 修改：`web/default/src/features/subscriptions/lib/plan-form.ts`
- 修改：`web/default/src/features/subscriptions/api.ts`
- 修改：`web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx`
- 修改：`web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx`
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
- 修改：`web/default/src/features/subscriptions/lib/plan-form.test.ts`
- 修改：`web/default/src/features/subscriptions/api.test.ts`
- 创建或修改：`web/default/src/features/subscriptions/components/subscription-kyren-payment.test.tsx`

- [x] **步骤 1：编写失败的 plan form 测试**

```ts
test('preserves kyren product id in plan form payload', () => {
  const payload = formValuesToPlanPayload({ ...PLAN_FORM_DEFAULTS, title: 'Pro', price_amount: 40, kyren_product_id: 'prod_kyren' })
  expect(payload.plan.kyren_product_id).toBe('prod_kyren')
})
```

- [x] **步骤 2：运行测试验证失败**

```bash
bun test src/features/subscriptions/lib/plan-form.test.ts
```

工作目录：`web/default`。预期：FAIL，`kyren_product_id` 不存在。

- [x] **步骤 3：实现订阅类型和表单字段**

- `SubscriptionPlan` 和 `PublicSubscriptionPlan` 增加 `kyren_product_id?: string`，并确保前端可读取可购买性所需字段：`currency`、`price_amount`、`enabled`、`public_visible`、`is_trial`、`max_purchase_per_user` 或后端等价可购买状态。
- `plan-form.ts` 的 schema、defaults、`planToFormValues`、`formValuesToPlanPayload` 增加 `kyren_product_id`。

- [x] **步骤 4：实现订阅 API 和可购买性 helper**

增加 `paySubscriptionKyren`、`getSubscriptionKyrenProduct`、`syncSubscriptionKyrenProduct`，均使用统一 `api` 实例。`getSubscriptionKyrenProduct` 的响应类型必须包含 `status`、`price`、`currency`、`price_matches`、`currency_matches`、`product_id` 和缺失/归档状态，供抽屉状态卡片渲染。

新增 `getKyrenSubscriptionAvailability(plan, topupInfo, purchaseContext)` 纯 helper，返回 `{ available, reasonKey }`，统一判断全局 Kyren 可用性、`kyren_product_id`、CNY、`price_amount >= 0.01`、非试用、enabled/public-visible、购买上限等条件。

- [x] **步骤 5：实现套餐抽屉 Kyren 区域**

- `Kyren Product ID` 输入框。
- 编辑模式显示 `Create Kyren product`、`Sync to Kyren`、`Refresh Kyren status`。
- 新建模式提示先保存套餐。
- 抽屉维护 Kyren 产品状态查询结果：打开编辑抽屉或点击刷新时调用 `getSubscriptionKyrenProduct`；创建/同步成功后立即刷新状态。
- 渲染状态卡片，展示 `product_id`、`status`、`price`、`currency`；当产品 missing、archived、`price_matches=false` 或 `currency_matches=false` 时，用醒目告警展示 `Kyren product is missing`、`Kyren product is archived`、`Kyren product price mismatch`、`Kyren product currency mismatch` 等 i18n 文案。
- 文案全部使用 `t()`。

- [x] **步骤 6：编写失败的订阅 Kyren 点击链路和可购买性测试**

创建或扩展 `subscription-kyren-payment.test.tsx`，测试用户点击 Kyren 按钮会调用真实支付 helper/API 并打开 checkout，并测试不可购买条件：

```ts
test('opens Kyren checkout for a purchasable CNY plan', async () => {
  const paySubscriptionKyren = vi.fn().mockResolvedValue({ success: true, data: { checkout_url: 'https://checkout.example/sub' } })
  const openCheckout = vi.fn()

  await processKyrenSubscriptionPayment({ planId: 1001, paySubscriptionKyren, openCheckout })

  expect(paySubscriptionKyren).toHaveBeenCalledWith({ plan_id: 1001 })
  expect(openCheckout).toHaveBeenCalledWith('https://checkout.example/sub')
})

test('marks Kyren subscription unavailable for trial, non-CNY, free, hidden, disabled, missing product, disabled plan, or purchase limit reached plans', () => {
  expect(getKyrenSubscriptionAvailability({ kyren_product_id: '', currency: 'CNY', price_amount: 40, enabled: true, public_visible: true, is_trial: false }, { enable_kyren_subscription: true }, {}).available).toBe(false)
  expect(getKyrenSubscriptionAvailability({ kyren_product_id: 'prod', currency: 'USD', price_amount: 40, enabled: true, public_visible: true, is_trial: false }, { enable_kyren_subscription: true }, {}).available).toBe(false)
  expect(getKyrenSubscriptionAvailability({ kyren_product_id: 'prod', currency: 'CNY', price_amount: 0, enabled: true, public_visible: true, is_trial: false }, { enable_kyren_subscription: true }, {}).available).toBe(false)
  expect(getKyrenSubscriptionAvailability({ kyren_product_id: 'prod', currency: 'CNY', price_amount: 40, enabled: true, public_visible: true, is_trial: true }, { enable_kyren_subscription: true }, {}).available).toBe(false)
  expect(getKyrenSubscriptionAvailability({ kyren_product_id: 'prod', currency: 'CNY', price_amount: 40, enabled: false, public_visible: true, is_trial: false }, { enable_kyren_subscription: true }, {}).available).toBe(false)
  expect(getKyrenSubscriptionAvailability({ kyren_product_id: 'prod', currency: 'CNY', price_amount: 40, enabled: true, public_visible: false, is_trial: false }, { enable_kyren_subscription: true }, {}).available).toBe(false)
  expect(getKyrenSubscriptionAvailability({ kyren_product_id: 'prod', currency: 'CNY', price_amount: 40, enabled: true, public_visible: true, is_trial: false, max_purchase_per_user: 1 }, { enable_kyren_subscription: true }, { purchaseCount: 1 }).available).toBe(false)
})
```

- [x] **步骤 7：实现订阅购买弹窗和父组件传参**

- `subscription-purchase-dialog.tsx` 增加 Kyren 按钮，按钮可用性必须由 `getKyrenSubscriptionAvailability` 决定：只有全局 Kyren 可用、plan `kyren_product_id` 非空、CNY、价格大于等于 `0.01`、非试用、enabled/public-visible、购买上限未达成时启用。
- 按钮点击调用 `paySubscriptionKyren({ plan_id })`，成功读取 `data.checkout_url` 并打开 Checkout；失败展示 `Kyren checkout creation failed` 或 `Kyren payment is unavailable` 等 i18n 错误；不可用时展示 `reasonKey` 对应 i18n 原因，而不是让用户点击后才看到通用失败。
- 新增 `processKyrenSubscriptionPayment` 和 `getKyrenSubscriptionAvailability` helper 供组件和测试复用。
- `subscription-plans-card.tsx` 从 `topupInfo?.enable_kyren_subscription` 派生全局可用性并传给弹窗。

- [x] **步骤 8：运行测试验证通过**

```bash
bun test src/features/subscriptions/lib/plan-form.test.ts src/features/subscriptions/api.test.ts src/features/subscriptions/components/subscription-kyren-payment.test.tsx
```

工作目录：`web/default`。预期：PASS。

- [x] **步骤 9：Commit**

```bash
git add web/default/src/features/subscriptions/types.ts web/default/src/features/subscriptions/lib/plan-form.ts web/default/src/features/subscriptions/api.ts web/default/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx web/default/src/features/wallet/components/subscription-plans-card.tsx web/default/src/features/subscriptions/lib/plan-form.test.ts web/default/src/features/subscriptions/api.test.ts web/default/src/features/subscriptions/components/subscription-kyren-payment.test.tsx
git commit -m "feat(payment): 添加订阅 Kyren 支付入口"
```

---

## 任务 7：前端支付设置 Kyren 配置和充值档位编辑器

**文件：**

- 修改：`web/default/src/features/system-settings/types.ts`
- 修改：`web/default/src/features/system-settings/billing/index.tsx`
- 修改：`web/default/src/features/system-settings/billing/section-registry.tsx`
- 修改：`web/default/src/features/system-settings/integrations/payment-settings-section.tsx`
- 创建：`web/default/src/features/system-settings/integrations/kyren-topup-products-visual-editor.tsx`
- 创建：`web/default/src/features/system-settings/integrations/kyren-topup-product-dialog.tsx`
- 创建或修改：`web/default/src/features/system-settings/integrations/payment-settings-section.test.tsx`
- 创建或修改：`web/default/src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx`

- [x] **步骤 1：编写失败的充值档位校验测试**

```ts
import { validateKyrenTopUpProducts } from './kyren-topup-products-visual-editor'

test('rejects duplicate kyren topup product ids', () => {
  expect(() => validateKyrenTopUpProducts([
    { id: 'topup_cny_10', name: '10 CNY', amount: '10.00', currency: 'CNY', quota: 1000, enabled: true },
    { id: 'topup_cny_10', name: '10 CNY again', amount: '10.00', currency: 'CNY', quota: 1000, enabled: true },
  ])).toThrow(/duplicate/i)
})
```

- [x] **步骤 2：运行测试验证失败**

```bash
bun test src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx
```

工作目录：`web/default`。预期：FAIL，module/function undefined。

- [x] **步骤 3：实现 Kyren 充值档位编辑器**

创建 `kyren-topup-products-visual-editor.tsx` 和 `kyren-topup-product-dialog.tsx`：

- `currency` 固定 `CNY`。
- `validateKyrenTopUpProducts` 校验 id 非空唯一、amount 格式且 >= 0.01、currency=CNY、quota>0。
- UI 风格参考 `creem-products-visual-editor.tsx`。
- 每档提供创建/同步/刷新状态入口；刷新调用任务 3 的 `GET /api/payment/kyren/topup-products/:id/status`，用返回的 `status`、`price_matches`、`currency_matches` 渲染只读状态；sync 调用任务 3 的 `POST /api/payment/kyren/topup-products/:id/sync`，成功后用服务端最新 `{ products, version }` 覆盖本地编辑器状态。

- [x] **步骤 4：接入支付设置数据流**

- `system-settings/types.ts` 的 `BillingSettings` 增加 `KyrenApiKey`、`KyrenWebhookSecret`、`KyrenBaseURL`、`KyrenTopUpProducts`、`ServerAddress`；其中 `ServerAddress` 用于渲染 Webhook URL，不作为 Kyren 保存项提交。
- `billing/index.tsx` 的 `defaultBillingSettings` 增加 `ServerAddress: ''`，确保 `SettingsPage` 的 option 归并逻辑会读取 `/api/option/` 返回的真实 `ServerAddress`。
- `billing/section-registry.tsx` 传入 `PaymentSettingsSection.defaultValues`：Kyren API Key、Webhook Secret、BaseURL 默认 `https://api.kyren.top`、TopUpProducts 默认 `[]`、`ServerAddress`。
- `payment-settings-section.tsx` 的 zod schema、form default/reset、保存逻辑、UI 分区增加 Kyren 字段。
- 空 `KyrenApiKey` / `KyrenWebhookSecret` 不提交。
- BaseURL 去尾斜杠。
- 显示 Webhook URL：使用去尾斜杠后的 `{ServerAddress}/api/kyren/webhook`；`ServerAddress` 为空时显示只读提示 `Server address is not configured`，并说明部署后需配置服务器地址。测试需断言已配置 `ServerAddress` 时渲染完整 Webhook URL。
- Kyren 充值档位不得通过通用 `UpdateOption` 保存整段 `KyrenTopUpProducts`。加载时调用 `GET /api/payment/kyren/topup-products` 读取 `{ products, version }`；保存时调用 `PUT /api/payment/kyren/topup-products` 并携带当前 `version`；收到 409 时提示配置已更新并 refetch；refresh 调用 `GET /api/payment/kyren/topup-products/:id/status`；sync 调用 `POST /api/payment/kyren/topup-products/:id/sync`；所有操作成功后合并服务端最新数据。

- [x] **步骤 5：运行测试验证通过**

```bash
bun test src/features/system-settings/integrations/payment-settings-section.test.tsx src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx
```

工作目录：`web/default`。预期：PASS。

- [x] **步骤 6：Commit**

```bash
git add web/default/src/features/system-settings/types.ts web/default/src/features/system-settings/billing/index.tsx web/default/src/features/system-settings/billing/section-registry.tsx web/default/src/features/system-settings/integrations/payment-settings-section.tsx web/default/src/features/system-settings/integrations/kyren-topup-products-visual-editor.tsx web/default/src/features/system-settings/integrations/kyren-topup-product-dialog.tsx web/default/src/features/system-settings/integrations/payment-settings-section.test.tsx web/default/src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx
git commit -m "feat(payment): 添加 Kyren 支付设置界面"
```

---

## 任务 8：前端 i18n 收口

**文件：**

- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/vi.json`
- 创建：`web/default/src/features/subscriptions/kyren-i18n.test.ts`

- [x] **步骤 1：编写失败的 i18n 覆盖测试**

```ts
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'

const requiredKeys = [
  'Kyren Pay',
  'Create Kyren product',
  'Sync to Kyren',
  'Refresh Kyren status',
  'Kyren product is missing',
  'Kyren product is archived',
  'Kyren product price mismatch',
  'Kyren product currency mismatch',
  'CNY only',
  'Save Kyren settings',
  'Kyren top-up products',
  'Please save the plan first',
  'Product status',
  'Webhook URL',
  'Kyren checkout creation failed',
  'Kyren payment is unavailable',
  'Kyren settings were updated elsewhere. Please reload and try again.',
  'Server address is not configured',
  'Kyren product binding status',
  'Open Kyren Checkout',
]

const locales = { en, zh, fr, ja, ru, vi }

test.each(Object.entries(locales))('%s has Kyren translations', (_name, locale) => {
  expect(locale).toHaveProperty('translation')
  for (const key of requiredKeys) expect(locale.translation).toHaveProperty(key)
})
```

- [x] **步骤 2：运行测试验证失败**

```bash
bun test src/features/subscriptions/kyren-i18n.test.ts
```

工作目录：`web/default`。预期：FAIL，缺少 Kyren 翻译 key。

- [x] **步骤 3：补充 6 个 locale 文件**

向 6 个 JSON 文件的 `translation` 节点添加 `requiredKeys` 中的全部 key，且同步任务 5-7 实际新增的其他 `t('...')` 文案。不得只补测试中的基础 key；实现过程中新增的 Kyren 用户可见文案都必须加入 en、zh、fr、ja、ru、vi。

- [x] **步骤 4：运行 i18n 测试验证通过**

```bash
bun test src/features/subscriptions/kyren-i18n.test.ts
```

工作目录：`web/default`。预期：PASS。

- [x] **步骤 5：运行前端目标测试**

```bash
bun test src/features/subscriptions/lib/plan-form.test.ts src/features/subscriptions/api.test.ts src/features/wallet/wallet-layout.test.ts src/features/system-settings/integrations/payment-settings-section.test.tsx src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx src/features/subscriptions/components/subscription-kyren-payment.test.tsx src/features/subscriptions/kyren-i18n.test.ts
```

工作目录：`web/default`。预期：PASS。

- [x] **步骤 6：Commit**

```bash
git add web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/vi.json web/default/src/features/subscriptions/kyren-i18n.test.ts
git commit -m "feat(payment): 补充 Kyren 支付多语言文案"
```

---

## 最终验证

完成所有任务后由主代理运行：

```bash
go test ./model ./controller ./router -run 'Kyren|TestGetTopUpInfoIncludesKyrenProducts|TestEnsureSubscriptionPlanTableSQLite|TestSubscriptionPlansPublicRoute' -count=1
```

```bash
bun test src/features/subscriptions/lib/plan-form.test.ts src/features/subscriptions/api.test.ts src/features/wallet/wallet-layout.test.ts src/features/system-settings/integrations/payment-settings-section.test.tsx src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx src/features/subscriptions/components/subscription-kyren-payment.test.tsx src/features/subscriptions/kyren-i18n.test.ts
```

工作目录：`web/default`。

```bash
bun run typecheck
```

工作目录：`web/default`。

全部通过后，派发 3 个只读 reviewer 并发审查：后端支付生命周期、安全/Webhook、前端体验/i18n。发布声明中必须把代码验证与外部集成验证分开：如有 Kyren staging 或低金额 live 凭据，记录脱敏订单号和 Webhook 受控字段摘要；如没有凭据，明确报告“未执行外部集成验证”，不得声明 staging/live 集成通过。所有 reviewer 通过后再报告完成。