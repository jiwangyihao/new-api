# 账户余额购买套餐与全面套餐制实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 让账户余额只用于购买订阅套餐，API 请求全面使用订阅套餐 token 与并发限制，不再消耗钱包余额、用户 quota 或 token key quota。

**架构：** 后端新增余额购买套餐入口，并把 relay / task 计费工厂收敛到订阅资金来源；前端移除计费偏好 UI，改为账户余额说明和余额购买按钮。保留充值与余额流水作为储值账户能力，但请求链路不再 fallback 到钱包。

**技术栈：** Go 1.25.1、Gin、GORM v2、PostgreSQL / MySQL / SQLite、React 19、TypeScript、Rsbuild、Bun。

---

## 参考文件

- 规格：`docs/superpowers/specs/2026-05-15-account-balance-subscription-only-spec.md`
- 原分销规格：`docs/superpowers/specs/2026-05-13-token-distribution-platform-spec.md`
- 计费入口：`service/billing_session.go`、`service/funding_source.go`
- 订阅模型：`model/subscription.go`
- 用户余额模型：`model/user.go`、`model/topup.go`
- 订阅支付：`controller/subscription.go`、`controller/subscription_payment_epay.go`、`controller/subscription_payment_stripe.go`、`controller/subscription_payment_creem.go`
- OpenAI billing 兼容接口：`controller/billing.go`
- 前端钱包与订阅：`web/default/src/features/wallet/*`、`web/default/src/features/subscriptions/*`

## 文件结构

### 后端

- 修改：`service/billing_session.go` —— 请求计费强制订阅，删除钱包 fallback 和 token key quota 预扣 / 结算。
- 修改：`service/funding_source.go` —— 保留 `WalletFunding` 给余额购买以外的旧代码兼容，但 relay 不再构造它。
- 修改：`controller/subscription.go` —— 更新偏好接口兼容、返回固定套餐制状态。
- 修改：`controller/subscription_payment_balance.go`（新建）—— 账户余额购买套餐接口。
- 修改：`controller/subscription_payment_epay.go`、`controller/subscription_payment_stripe.go`、`controller/subscription_payment_creem.go` —— 共用可购买校验，确保余额支付不影响在线支付。
- 修改：`model/subscription.go` —— 增加余额支付完成订单 helper，或暴露可事务调用的完成订阅函数。
- 修改：`model/user.go` 或新增 `model/account_balance.go` —— 增加原子扣减账户余额 helper。
- 修改：`router/api-router.go` —— 增加 `/api/subscription/balance/pay`。
- 修改：`controller/billing.go` —— OpenAI 兼容 billing 接口改为订阅 token 语义。
- 测试：`service/subscription_only_billing_test.go`、`controller/subscription_balance_purchase_test.go`、`controller/billing_subscription_only_test.go`、更新 `controller/subscription_non_text_billing_test.go`。

### 前端

- 修改：`web/default/src/features/subscriptions/api.ts` —— 增加余额购买套餐 API。
- 修改：`web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx` —— 增加账户余额支付按钮和余额不足提示。
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx` —— 删除计费偏好下拉框，展示套餐制说明。
- 修改：`web/default/src/features/wallet/components/recharge-form-card.tsx` —— 文案从钱包余额改为账户余额。
- 修改：`web/default/src/features/wallet/lib/*`、`types.ts` —— 如有余额文案或 API 类型需要同步更新。
- 测试：`web/default/src/features/subscriptions/lib/format.test.ts` 或新增轻量纯函数测试；交互用 `bun run typecheck` 覆盖。

## 并行开发边界

- 任务 1 和任务 2 都修改 `service/billing_session.go`，必须串行。
- 任务 3 修改订阅购买模型和 controller，可在任务 1 完成后并行于任务 2，但如果需要复用错误码或 helper，应先同步接口名称。
- 任务 4 前端依赖任务 3 的 API 路径与响应契约，必须在任务 3 后执行。
- 任务 5 是最终验证，不与其他任务并行。

## 任务 1：请求计费强制订阅

**文件：**
- 修改：`service/billing_session.go`
- 修改：`service/funding_source.go`
- 测试：`service/subscription_only_billing_test.go`
- 测试：`controller/subscription_non_text_billing_test.go`

- [ ] **步骤 1：编写失败测试：旧偏好不走钱包**

创建 `service/subscription_only_billing_test.go`，使用现有 model 测试库 helper 模式构造用户、token 和订阅。测试 `wallet_first` / `wallet_only` / `subscription_first` 在请求链路都不能走钱包：

```go
func TestNewBillingSessionRequiresSubscriptionWhenWalletPreferenceSet(t *testing.T) {
    setupSubscriptionOnlyBillingTestDB(t)
    userID := 9301
    tokenID := 9302
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "wallet_pref", Quota: 1_000_000, Status: common.UserStatusEnabled}).Error)
    require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-wallet-pref", Status: common.TokenStatusEnabled, RemainQuota: 1_000_000}).Error)

    for _, pref := range []string{"wallet_first", "wallet_only", "subscription_first", ""} {
        t.Run(pref, func(t *testing.T) {
            ctx := ginTestContextWithUser(t, userID, tokenID, pref)
            relayInfo := subscriptionOnlyRelayInfo(userID, tokenID, pref)
            session, apiErr := NewBillingSession(ctx, relayInfo, 10)
            require.Nil(t, session)
            require.NotNil(t, apiErr)
            assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
            assert.Contains(t, apiErr.Error(), "subscription")

            var user model.User
            require.NoError(t, model.DB.First(&user, userID).Error)
            assert.Equal(t, 1_000_000, user.Quota)
            var token model.Token
            require.NoError(t, model.DB.First(&token, tokenID).Error)
            assert.Equal(t, 1_000_000, token.RemainQuota)
        })
    }
}
```

测试辅助函数必须放在同文件内，明确设置：

```go
func subscriptionOnlyRelayInfo(userID int, tokenID int, pref string) *relaycommon.RelayInfo {
    info := &relaycommon.RelayInfo{
        RelayFormat:     types.RelayFormatOpenAI,
        RelayMode:       relayconstant.RelayModeChatCompletions,
        RequestId:       fmt.Sprintf("req-sub-only-%d", time.Now().UnixNano()),
        UserId:          userID,
        TokenId:         tokenID,
        TokenKey:        "sk-wallet-pref",
        OriginModelName: "gpt-4o-mini",
        UserSetting:     dto.UserSetting{BillingPreference: pref},
        ChannelMeta:     &relaycommon.ChannelMeta{},
    }
    info.SetEstimatePromptTokens(10)
    return info
}
```

- [ ] **步骤 2：编写失败测试：有订阅时不扣 token key quota**

同文件追加：

```go
func TestSubscriptionBillingDoesNotConsumeTokenKeyQuota(t *testing.T) {
    setupSubscriptionOnlyBillingTestDB(t)
    userID := 9311
    tokenID := 9312
    planCode := "sub-only-basic"
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "sub_only", Quota: 0, Status: common.UserStatusEnabled}).Error)
    require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-sub-only", Status: common.TokenStatusEnabled, RemainQuota: 0}).Error)
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9313, Title: "Basic", Enabled: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &planCode}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9314, UserId: userID, PlanId: 9313, TokenLimit: 1000, TokenUsed: 0, ConcurrencyLimit: 1, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order", Source: "order"}).Error)

    ctx := ginTestContextWithUser(t, userID, tokenID, "wallet_only")
    relayInfo := subscriptionOnlyRelayInfo(userID, tokenID, "wallet_only")
    relayInfo.TokenKey = "sk-sub-only"
    session, apiErr := NewBillingSession(ctx, relayInfo, 10)
    require.Nil(t, apiErr)
    require.NotNil(t, session)
    require.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
    require.NoError(t, session.SettleWithInput(BillingSettleInput{WalletQuota: 20, SubscriptionTokens: 20}))

    var token model.Token
    require.NoError(t, model.DB.First(&token, tokenID).Error)
    assert.Equal(t, 0, token.RemainQuota)
    assert.Equal(t, 0, token.UsedQuota)
    var sub model.UserSubscription
    require.NoError(t, model.DB.First(&sub, 9314).Error)
    assert.Equal(t, int64(20), sub.TokenUsed)
}
```

- [ ] **步骤 3：运行测试验证失败**

运行：

```bash
go test ./service -run 'TestNewBillingSessionRequiresSubscriptionWhenWalletPreferenceSet|TestSubscriptionBillingDoesNotConsumeTokenKeyQuota' -count=1
```

预期：FAIL。旧逻辑会走 `WalletFunding`，或因 token key `RemainQuota = 0` 导致预扣失败。

- [ ] **步骤 4：实现订阅-only 请求计费**

在 `service/billing_session.go` 中修改：

1. `NewBillingSession` 不再根据 `billing_preference` 分支构造 `WalletFunding`。
2. 删除 `tryWallet` 或仅保留未使用 helper。
3. 所有偏好都调用 `trySubscriptionForCurrentRelay(false)`。
4. 如果无订阅，返回包含 `subscription_required` 语义的错误。可先复用 `types.ErrorCodeInsufficientUserQuota`，但错误消息必须包含 `active subscription is required`。
5. 非目标 relay mode 直接返回 `distributorSubscriptionRelayError(relayInfo)`，不再 fallback 钱包。
6. `preConsume` 不再调用 `PreConsumeTokenQuota`。
7. `Reserve` 不再调用 `reserveToken`。
8. `SettleWithInput` 不再调整 `model.DecreaseTokenQuota` / `model.IncreaseTokenQuota`。
9. `Refund` 不再退 token key quota，只退订阅预扣。

保留 `WalletFunding` 类型，但 relay 请求不再构造它。

- [ ] **步骤 5：更新非文本任务测试期望**

`controller/subscription_non_text_billing_test.go` 中旧测试如果断言 fallback 到 wallet，应改为断言明确错误：

```go
assert.NotNil(t, apiErr)
assert.Contains(t, apiErr.Error(), "does not support subscription billing")
assert.Empty(t, relayInfo.BillingSource)
```

并断言用户 `Quota`、token `RemainQuota` 均不变。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
gofmt -w service/billing_session.go service/funding_source.go service/subscription_only_billing_test.go controller/subscription_non_text_billing_test.go
go test ./service -run 'TestNewBillingSessionRequiresSubscriptionWhenWalletPreferenceSet|TestSubscriptionBillingDoesNotConsumeTokenKeyQuota|Test.*SubscriptionBilling|Test.*TaskBilling' -count=1
go test ./controller -run 'TestRelayTaskDoesNotPreConsumeDistributorSubscription|TestRelayTaskDistributorFallbackRestoresTokenBeforeWallet' -count=1
```

预期：PASS。若 `TestRelayTaskDistributorFallbackRestoresTokenBeforeWallet` 名称不再匹配新语义，重命名为 `TestRelayTaskDistributorDoesNotFallbackToWallet`。

- [ ] **步骤 7：Commit**

```bash
git add service/billing_session.go service/funding_source.go service/subscription_only_billing_test.go controller/subscription_non_text_billing_test.go
git commit -m "fix(billing): API 请求强制使用订阅套餐"
```

## 任务 2：OpenAI billing 兼容接口改为订阅语义

**文件：**
- 修改：`controller/billing.go`
- 测试：`controller/billing_subscription_only_test.go`

- [ ] **步骤 1：编写失败测试**

创建 `controller/billing_subscription_only_test.go`：

```go
func TestOpenAIBillingSubscriptionUsesActiveSubscriptionTokens(t *testing.T) {
    setupBillingSubscriptionOnlyTestDB(t)
    userID := 9401
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "billing_sub", Quota: 999999, UsedQuota: 123, Status: common.UserStatusEnabled}).Error)
    code := "billing-basic"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9402, Title: "Basic", Enabled: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9403, UserId: userID, PlanId: 9402, TokenLimit: 1000, TokenUsed: 250, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order"}).Error)

    recorder := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(recorder)
    ctx.Request = httptest.NewRequest(http.MethodGet, "/dashboard/billing/subscription", nil)
    ctx.Set("id", userID)

    GetSubscription(ctx)

    require.Equal(t, http.StatusOK, recorder.Code)
    body := recorder.Body.String()
    assert.Contains(t, body, "hard_limit_usd")
    assert.NotContains(t, body, "999999")
}
```

追加 usage 测试：

```go
func TestOpenAIUsageUsesSubscriptionTokenUsed(t *testing.T) {
    setupBillingSubscriptionOnlyTestDB(t)
    userID := 9411
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "usage_sub", Quota: 999999, UsedQuota: 888888, Status: common.UserStatusEnabled}).Error)
    code := "usage-basic"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9412, Title: "Basic", Enabled: true, MonthlyTokenLimit: 2000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9413, UserId: userID, PlanId: 9412, TokenLimit: 2000, TokenUsed: 333, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order"}).Error)

    recorder := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(recorder)
    ctx.Request = httptest.NewRequest(http.MethodGet, "/dashboard/billing/usage", nil)
    ctx.Set("id", userID)

    GetUsage(ctx)

    require.Equal(t, http.StatusOK, recorder.Code)
    assert.Contains(t, recorder.Body.String(), "333")
    assert.NotContains(t, recorder.Body.String(), "888888")
}
```

- [ ] **步骤 2：运行测试验证失败**

```bash
go test ./controller -run 'TestOpenAI(BillingSubscriptionUsesActiveSubscriptionTokens|UsageUsesSubscriptionTokenUsed)' -count=1
```

预期：FAIL。旧实现读取 `model.GetUserQuota` 和 `model.GetUserUsedQuota`。

- [ ] **步骤 3：实现订阅 token 查询 helper**

在 `model/subscription.go` 增加 helper：

```go
func GetActiveDistributorSubscriptionUsage(userId int) (tokenLimit int64, tokenUsed int64, endTime int64, unlimited bool, err error)
```

规则：

- 查询 `user_subscriptions` 中 `status = active AND user_id = ? AND end_time > now`。
- 优先返回 `grant_reason = order` 或分销套餐记录；若多条，按 `end_time desc, id desc`。
- `token_limit = 0` 且 `grant_reason in ('trial_code','invite_trial')` 表示 unlimited。
- 无订阅时返回 0 值和 nil error。

- [ ] **步骤 4：修改 `controller/billing.go`**

`GetSubscription` 和 `GetUsage` 不再读取用户钱包余额。改为读取 `GetActiveDistributorSubscriptionUsage`：

- `hard_limit_usd` 映射 token limit。
- `total_usage` 映射 token used。
- `system_hard_limit_usd` 同 token limit。
- unlimited 可返回大数 `100000000` 保持兼容。

字段名保持原 OpenAI 兼容命名。

- [ ] **步骤 5：运行测试验证通过**

```bash
gofmt -w controller/billing.go controller/billing_subscription_only_test.go model/subscription.go
go test ./controller -run 'TestOpenAI(BillingSubscriptionUsesActiveSubscriptionTokens|UsageUsesSubscriptionTokenUsed)' -count=1
```

预期：PASS。

- [ ] **步骤 6：Commit**

```bash
git add controller/billing.go controller/billing_subscription_only_test.go model/subscription.go
git commit -m "fix(billing): OpenAI 余额接口返回订阅 token 语义"
```

## 任务 3：账户余额购买套餐后端

**文件：**
- 创建：`controller/subscription_payment_balance.go`
- 修改：`controller/subscription.go`
- 修改：`model/subscription.go`
- 修改：`model/user.go` 或创建 `model/account_balance.go`
- 修改：`router/api-router.go`
- 测试：`controller/subscription_balance_purchase_test.go`

- [ ] **步骤 1：编写失败测试：余额足够购买成功**

创建 `controller/subscription_balance_purchase_test.go`：

```go
func TestSubscriptionBalancePayCreatesSubscriptionAndDeductsBalance(t *testing.T) {
    setupSubscriptionBalancePurchaseTestDB(t)
    userID := 9501
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_buyer", Quota: 100, Status: common.UserStatusEnabled}).Error)
    code := "balance-basic"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9502, Title: "Basic", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

    recorder := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(recorder)
    ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/balance/pay", bytes.NewBufferString(`{"plan_id":9502,"idempotency_key":"balance-key-1"}`))
    ctx.Request.Header.Set("Content-Type", "application/json")
    ctx.Set("id", userID)

    SubscriptionRequestBalance(ctx)

    require.Equal(t, http.StatusOK, recorder.Code)
    var user model.User
    require.NoError(t, model.DB.First(&user, userID).Error)
    assert.Equal(t, 60, user.Quota)
    var sub model.UserSubscription
    require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9502).First(&sub).Error)
    assert.Equal(t, "order", sub.GrantReason)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9502).First(&order).Error)
    assert.Equal(t, common.TopUpStatusSuccess, order.Status)
    assert.Equal(t, model.PaymentProviderBalance, order.PaymentProvider)
}
```

- [ ] **步骤 2：编写失败测试：余额不足和幂等**

追加：

```go
func TestSubscriptionBalancePayInsufficientBalanceDoesNotDeduct(t *testing.T) {
    setupSubscriptionBalancePurchaseTestDB(t)
    userID := 9511
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_low", Quota: 10, Status: common.UserStatusEnabled}).Error)
    code := "balance-pro"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9512, Title: "Pro", PriceAmount: 160, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

    recorder := performBalancePayRequest(t, userID, `{"plan_id":9512,"idempotency_key":"balance-low"}`)
    require.Equal(t, http.StatusOK, recorder.Code)
    assert.Contains(t, recorder.Body.String(), "余额不足")

    var user model.User
    require.NoError(t, model.DB.First(&user, userID).Error)
    assert.Equal(t, 10, user.Quota)
}

func TestSubscriptionBalancePayIdempotent(t *testing.T) {
    setupSubscriptionBalancePurchaseTestDB(t)
    userID := 9521
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_idem", Quota: 100, Status: common.UserStatusEnabled}).Error)
    code := "balance-standard"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9522, Title: "Standard", PriceAmount: 80, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

    first := performBalancePayRequest(t, userID, `{"plan_id":9522,"idempotency_key":"idem-key"}`)
    second := performBalancePayRequest(t, userID, `{"plan_id":9522,"idempotency_key":"idem-key"}`)
    require.Equal(t, http.StatusOK, first.Code)
    require.Equal(t, http.StatusOK, second.Code)

    var user model.User
    require.NoError(t, model.DB.First(&user, userID).Error)
    assert.Equal(t, 20, user.Quota)
    var count int64
    require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9522).Count(&count).Error)
    assert.Equal(t, int64(1), count)
}
```

- [ ] **步骤 3：运行测试验证失败**

```bash
go test ./controller -run 'TestSubscriptionBalancePay' -count=1
```

预期：FAIL，`SubscriptionRequestBalance` 或 `PaymentProviderBalance` 未定义。

- [ ] **步骤 4：实现模型和 provider 常量**

在 `model/subscription.go` 增加：

```go
const PaymentProviderBalance = "balance"
const PaymentMethodAccountBalance = "account_balance"
```

如当前 provider 常量在其他文件，放在同一常量组中。

新增 helper：

```go
func CompleteSubscriptionOrderTx(tx *gorm.DB, order *SubscriptionOrder, providerPayload string, actualPaymentMethod string) (*UserSubscription, error)
```

把 `CompleteSubscriptionOrder` 内部创建订阅的核心逻辑抽出来，让余额支付能在同一个事务里扣余额、创建订单、创建订阅。

在 `model/account_balance.go` 或 `model/user.go` 增加：

```go
func DeductUserAccountBalanceTx(tx *gorm.DB, userId int, amount int) error
```

规则：

- `amount` 单位沿用当前 `User.Quota` 数值语义；本部署已把 `price_amount` 当人民币展示，余额支付本次也按整数元扣减。
- 使用单条条件更新防并发超扣：`WHERE id = ? AND quota >= ?`。
- `RowsAffected = 0` 返回余额不足错误。

- [ ] **步骤 5：实现 controller 和路由**

创建 `controller/subscription_payment_balance.go`：

```go
type SubscriptionBalancePayRequest struct {
    PlanId         int    `json:"plan_id"`
    IdempotencyKey string `json:"idempotency_key"`
}

func SubscriptionRequestBalance(c *gin.Context) { ... }
```

行为：

1. 校验登录用户 ID。
2. 校验 plan 可购买。
3. 校验 `idempotency_key` 非空。
4. `trade_no = fmt.Sprintf("BALSUBUSR%dNO%s", userId, safeKey)`。
5. 若同 trade_no 已 success，返回已有订单。
6. 在事务中扣余额、创建 success 订单、创建订阅。
7. 成功后触发 `service.EnsureMonthlyInvitationEntitlement`，保持邀请权益逻辑一致。

在 `router/api-router.go` 的 subscriptionRoute 增加：

```go
subscriptionRoute.POST("/balance/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestBalance)
```

- [ ] **步骤 6：运行测试验证通过**

```bash
gofmt -w controller/subscription_payment_balance.go controller/subscription.go model/subscription.go model/user.go router/api-router.go controller/subscription_balance_purchase_test.go
go test ./controller -run 'TestSubscriptionBalancePay' -count=1
```

预期：PASS。

- [ ] **步骤 7：Commit**

```bash
git add controller/subscription_payment_balance.go controller/subscription.go model/subscription.go model/user.go router/api-router.go controller/subscription_balance_purchase_test.go
git commit -m "feat(subscription): 支持账户余额购买套餐"
```

## 任务 4：前端账户余额购买与去除计费偏好

**文件：**
- 修改：`web/default/src/features/subscriptions/api.ts`
- 修改：`web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx`
- 修改：`web/default/src/features/wallet/components/subscription-plans-card.tsx`
- 修改：`web/default/src/features/wallet/components/recharge-form-card.tsx`
- 修改：`web/default/src/features/wallet/types.ts`

- [ ] **步骤 1：新增 API 封装**

在 `web/default/src/features/subscriptions/api.ts` 增加：

```ts
export async function paySubscriptionBalance(data: {
  plan_id: number
  idempotency_key: string
}) {
  const res = await api.post('/api/subscription/balance/pay', data)
  return res.data
}
```

- [ ] **步骤 2：购买弹窗增加账户余额支付**

`subscription-purchase-dialog.tsx`：

- 增加 props：`accountBalance?: number`。
- 余额显示为 `¥xx.xx`。
- 增加「账户余额支付」按钮。
- 每次打开弹窗生成一次 `idempotency_key`，重复点击同一按钮复用同一 key 直到请求完成。
- 余额不足时禁用按钮并显示 `账户余额不足，请先充值`。

按钮调用：

```ts
await paySubscriptionBalance({ plan_id: plan.id, idempotency_key })
```

成功后关闭弹窗、刷新订阅和余额信息。

- [ ] **步骤 3：移除计费偏好下拉框**

`subscription-plans-card.tsx`：

删除 Select 中这些选项和更新逻辑：

- `subscription_first`
- `wallet_first`
- `subscription_only`
- `wallet_only`

替换为静态说明：

```tsx
<p className='text-muted-foreground text-xs'>
  {t('API requests consume tokens from your active subscription plan.')}
</p>
```

保留刷新按钮和当前订阅展示。

- [ ] **步骤 4：账户余额文案**

`recharge-form-card.tsx`：

- 标题 `Add Funds` 改为 `Add Account Balance`。
- 描述 `Choose an amount and payment method` 改为 `Account balance can be used to purchase subscription plans.`。
- 充值金额文案保持支付含义，但避免说「API balance」或「quota」。

- [ ] **步骤 5：运行前端验证**

```bash
cd web/default
bun run typecheck
```

预期：PASS。

- [ ] **步骤 6：Commit**

```bash
git add web/default/src/features/subscriptions/api.ts web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx web/default/src/features/wallet/components/subscription-plans-card.tsx web/default/src/features/wallet/components/recharge-form-card.tsx web/default/src/features/wallet/types.ts
git commit -m "feat(web): 支持账户余额购买套餐"
```

## 任务 5：最终验证与部署检查

**文件：**
- 检查全部已修改文件

- [ ] **步骤 1：后端精准测试**

```bash
go test ./service -run 'TestNewBillingSessionRequiresSubscriptionWhenWalletPreferenceSet|TestSubscriptionBillingDoesNotConsumeTokenKeyQuota' -count=1
go test ./controller -run 'TestSubscriptionBalancePay|TestOpenAI(BillingSubscriptionUsesActiveSubscriptionTokens|UsageUsesSubscriptionTokenUsed)|TestRelayTaskDoesNotPreConsumeDistributorSubscription' -count=1
```

预期：全部 PASS。

- [ ] **步骤 2：受影响包测试**

```bash
go test ./model ./service ./controller ./relay/... -count=1
```

预期：全部 PASS。

- [ ] **步骤 3：前端验证**

```bash
cd web/default
bun run typecheck
bun run lint
bun run build
```

预期：全部 PASS。

- [ ] **步骤 4：手动联调清单**

部署后验证：

1. 用户充值 100 元账户余额。
2. 使用账户余额购买 Basic，余额变为 60 元，获得 Basic 订阅。
3. 重复点击余额购买不二次扣款。
4. 无订阅但有余额用户调用 `/v1/chat/completions` 返回 `subscription_required`。
5. 有订阅且 token key `remain_quota = 0` 时仍可调用文本 API。
6. 文本 API 只增加 `UserSubscription.TokenUsed`，不扣账户余额。
7. 非文本任务不 fallback 账户余额。
8. 前端不再展示计费偏好下拉框。
9. OpenAI billing 兼容接口不返回钱包余额语义。

- [ ] **步骤 5：最终提交状态检查**

```bash
git status --short
```

确认没有无关文件。若任务 1-4 已逐步提交，本任务不需要新提交；如验证中修复问题，提交：

```bash
git add <修复文件>
git commit -m "fix(billing): 完善账户余额套餐制验证"
```
