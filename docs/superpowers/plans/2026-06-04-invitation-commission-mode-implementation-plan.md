# 邀请返佣模式实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。用户已批准直接在当前主工作区开发；禁止创建 Git worktree。实现子代理必须读取本计划和规格文件，并遵守仓库根目录 `AGENTS.md`；修改 `web/default` 时还必须读取 `web/default/AGENTS.md`。

**目标：** 在不改变当前代码中奖励套餐计算口径的前提下，新增管理员开启的「邀请返佣」模式，使支付订单、账户余额购买和订阅兑换码等主要销售来源都能按套餐 `reward_eligible` 参与返佣；管理员可在用户列表中手动切换奖励套餐或返佣模式，并支持用户即时划转余额、用户申请私聊转账返现、管理员人工完成或拒绝返现。

**架构：** 后端新增销售来源金额快照、邀请返佣来源事件表、返佣账户/记录/ledger/withdrawal 表；订单完成和订阅兑换码兑换在事务中记录来源事件、金额快照和新增订阅权益区间。奖励套餐继续使用当前代码中基于直属 active `user_subscriptions`、`subscription_plans.reward_eligible`、非试用、非 `monthly_invite_entitlement` 的计算口径，不改为事件表驱动；返佣从来源事件表读取，并按邀请人当前管理员模式、套餐 `reward_eligible`、金额快照和事件有效区间 fresh 计算。前端在用户管理中设置邀请奖励模式，在钱包展示返佣账户与操作，在管理员侧边栏增加返现申请入口和待办 badge。

**技术栈：** Go 1.22+、Gin、GORM v2、SQLite/MySQL/PostgreSQL；React 19、TypeScript、TanStack Query/Axios、Base UI/Tailwind、i18next、Bun。

---

## 规格与执行边界

- 规格文件：`docs/superpowers/specs/2026-06-03-invitation-commission-spec.md`。
- 本计划文件：`docs/superpowers/plans/2026-06-04-invitation-commission-mode-implementation-plan.md`。
- 工作区：当前主分支工作区；不创建 worktree。
- 子代理执行：实现阶段按任务边界分派子代理。每个新子代理提示词必须包含本计划和规格的完整路径、任务范围、验收标准、禁止事项、验证命令，且不少于 2000 字。
- 并行策略：后端模型与核心事务完成前，依赖其类型的任务不并行写同一文件。核心契约稳定后，Controller、前端钱包、前端管理员页可按不重叠文件并行执行。只读审查任务必须 3 个以上并发。
- TDD：每个实现任务先写失败测试并运行确认失败，再写生产代码，再运行同一测试确认通过。
- JSON 规则：所有业务 JSON marshal/unmarshal 使用 `common.Marshal`、`common.Unmarshal`、`common.UnmarshalJsonStr`、`common.DecodeJson`；不得直接调用 `encoding/json` 的 marshal/unmarshal。
- 数据库规则：所有新增表和索引必须兼容 SQLite、MySQL、PostgreSQL。资金扣减和状态推进使用事务内条件更新、`gorm.Expr`、`RowsAffected` 校验；不得依赖 `FOR UPDATE` 作为唯一并发控制。
- 资金单位：返佣、订单快照、提现金额统一使用整数最小单位；只有 `currency = "CNY"` 时 `amount_cents` 才按 CNY 分进入返佣公式。

---

## 文件结构

### 后端模型与配置

- 修改：`model/user.go` —— 新增 `InvitationRewardMode` 字段、常量、归一化函数；用户列表继续沿用当前 active 订阅口径统计奖励套餐资格，并携带奖励模式字段。
- 修改：`model/subscription.go` —— `SubscriptionOrder` 新增 `AmountCents`、`Currency`；订单完成结果返回真实状态迁移、订阅 ID 和事件区间；订单创建入口写入金额快照。
- 修改：`model/redemption.go` —— 订阅兑换码新增 `AmountCents`、`Currency` 快照；兑换订阅套餐时落邀请销售来源事件。
- 创建：`model/invitation_commission.go` —— 定义 `InvitationRewardEvent`、`InvitationCommissionAccount`、`InvitationCommissionRecord`、`InvitationCommissionLedger`、`InvitationCommissionWithdrawal` 及常量、分页 DTO、基础模型函数。
- 修改：`model/main.go` —— `migrateDB` 与 `migrateDBFast` 纳入新增表和兑换码快照字段；SQLite 场景确认复合唯一索引可创建。
- 创建或修改：`setting/operation_setting/invitation_commission_setting.go` —— 注册 `invitation_commission_setting`，字段为 `enabled`、`rate_bps`、`minimum_withdraw_cents`、`minimum_transfer_cents`，持久化 key 固定为 `invitation_commission_setting.*`，校验 `0 <= rate_bps <= 10000`。

### 后端服务与销售来源完成

- 创建：`service/invitation_commission.go` —— 返佣计算、事件处理、账户原子更新、划转、返现申请、管理员完成/拒绝、待办摘要。
- 修改：`service/invitation_reward.go` —— `EnsureMonthlyInvitationEntitlement`、`GetInvitationEntitlementStatus`、`RunMonthlyInvitationEntitlementSweep` 只增加 mode-aware 保护：当前 `commission` 邀请人不 upsert 奖励套餐；当前 `subscription` 邀请人保持现有 active 订阅计算口径。
- 修改：`controller/subscription_payment_completion.go` —— 支付完成后调用新的销售来源事件处理服务；Kyren 快照路径同样在事务中记录来源事件。
- 修改：`controller/subscription_payment_balance.go` —— 账户余额购买订单写入 CNY 分快照，并在订单完成事务中记录来源事件。
- 修改：`controller/subscription_payment_epay.go`、`controller/subscription_payment_stripe.go`、`controller/subscription_payment_creem.go`、`controller/subscription_payment_kyren.go` —— 下单时写入可证明的 `amount_cents/currency`；无法证明时写 `0/""`，返佣跳过；外部支付回调必须校验 provider 金额 / 币种与订单快照一致。
- 修改：`controller/redemption.go`、`controller/user.go`、`model/redemption.go` —— 订阅兑换码创建 / 更新写入套餐价格快照，兑换时创建 `subscription_redemption` 来源事件并触发返佣处理。

### 后端 Controller 与路由

- 创建：`controller/invitation_commission.go` —— 用户返佣 summary、records、transfer、withdrawals；管理员 withdrawals list、complete、reject。
- 创建：`controller/admin_tasks.go` —— `GET /api/admin/tasks/summary` 返回 `pending_commission_withdrawals`。
- 修改：`controller/user.go` —— 管理员用户更新接口允许并校验 `invitation_reward_mode`；普通用户自助更新不得接受该字段；`GetSelf`/用户列表必要响应携带字段。
- 修改：`router/api-router.go` —— 注册用户返佣接口、管理员返现接口、管理员待办摘要接口。

### 后端测试

- 创建：`model/invitation_commission_test.go` —— 模型默认值、迁移、唯一索引、金额快照、事件回填。
- 创建：`service/invitation_commission_test.go` —— 返佣入账、幂等、并发扣减、划转、返现申请、管理员 complete/reject、历史 `legacy_user_subscription` 回填后切到 `commission` 的 retry 补算。
- 修改或创建：`service/invitation_reward_test.go` —— mode-aware 套餐保护、现有 active 订阅口径保护、防止误用事件表、历史返佣来源回填。
- 创建：`controller/invitation_commission_test.go` —— 用户接口权限、历史返佣账户、划转和申请。
- 创建：`controller/admin_invitation_commission_test.go` —— 管理员列表、complete/reject、待办摘要、用户奖励模式更新权限。
- 修改：现有 `controller/subscription_balance_purchase_test.go`、`controller/subscription_trial_purchase_test.go`、`controller/subscription_payment_kyren_test.go`、`controller/subscription_payment_stripe_test.go`、`controller/subscription_payment_creem_test.go`、`controller/invitation_entitlement_test.go` —— 增加订单快照、外部支付回调校验和支付完成事件断言。

### 前端用户管理与钱包

- 修改：`web/default/src/features/users/types.ts` —— 增加 `InvitationRewardMode`、`invitation_reward_mode` 字段。
- 修改：`web/default/src/features/users/lib/user-form.ts` —— 表单 schema/default/payload/defaults 增加 `invitation_reward_mode`。
- 修改：`web/default/src/features/users/components/users-mutate-drawer.tsx` —— 更新抽屉增加「邀请奖励模式」选择。
- 修改：`web/default/src/features/users/components/users-columns.tsx` —— 用户列表显示奖励模式 badge。
- 修改：`web/default/src/features/wallet/types.ts` —— 增加返佣 summary、record、withdrawal、transfer DTO。
- 修改：`web/default/src/features/wallet/api.ts` —— 用户返佣 API helper。
- 创建：`web/default/src/features/wallet/hooks/use-invitation-commission.ts` —— React Query hooks 和 mutation invalidation。
- 修改：`web/default/src/features/wallet/components/affiliate-rewards-card.tsx` —— 在套餐模式与返佣模式之间切换展示，但保留邀请链接与统计。
- 创建：`web/default/src/features/wallet/components/dialogs/commission-transfer-dialog.tsx` —— 即时划转对话框。
- 创建：`web/default/src/features/wallet/components/dialogs/commission-withdrawal-dialog.tsx` —— 私聊转账返现申请对话框。

### 前端管理员页、侧边栏与 i18n

- 创建：`web/default/src/features/invitation-commission/types.ts` —— 管理员返现申请 DTO 与筛选类型。
- 创建：`web/default/src/features/invitation-commission/api.ts` —— 管理员返现申请 API 与待办摘要 API。
- 创建：`web/default/src/features/invitation-commission/admin-withdrawals.tsx` —— 管理员返现审核页。
- 创建：`web/default/src/routes/_authenticated/invitation-commission/withdrawals/index.tsx` —— 固定路由。
- 修改：`web/default/src/hooks/use-sidebar-data.ts` —— Admin 组 Users 后插入 `Manual cashback requests` /「返现申请」入口和 badge。
- 修改：`web/default/src/hooks/use-sidebar-config.ts` —— `DEFAULT_SIDEBAR_MODULES`、`URL_TO_CONFIG_MAP` 增加 `invitation_commission` 模块。
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` —— 补齐 6 种语言文案。
- 修改：`web/default/src/i18n/static-keys.ts` —— 本计划采用若干常量 `labelKey` 和状态 key，必须登记无法被 `t('...')` 字面量扫描到的 key。
- 创建或修改：`web/default/src/features/users/users-form.test.ts`、`web/default/src/features/wallet/wallet-layout.test.ts`、`web/default/src/features/invitation-commission/admin-withdrawals.test.ts`、`web/default/src/hooks/use-sidebar-config.test.ts` —— 源码契约与组件行为测试。

---

## 任务 1：后端模型、配置、订单快照基础

**文件：**
- 修改：`model/user.go`
- 修改：`model/subscription.go`
- 创建：`model/invitation_commission.go`
- 修改：`model/main.go`
- 创建：`setting/operation_setting/invitation_commission_setting.go`
- 测试：`model/invitation_commission_test.go`
- 测试：`setting/operation_setting/invitation_commission_setting_test.go`
- 测试：`controller/option_invitation_commission_test.go`
- 测试：`controller/subscription_balance_purchase_test.go`
- 修改：`controller/subscription_payment_balance.go` —— 账户余额购买写入 CNY 订单快照。
- 修改：`controller/subscription_payment_epay.go` —— Epay 下单写入提交给 Epay 的 CNY 快照。
- 修改：`controller/subscription_payment_kyren.go` —— Kyren 写入可证明 CNY 快照，并校验 product/amount/currency mismatch。
- 修改：`controller/subscription_payment_stripe.go` —— Stripe checkout 写入 provider-proven 快照，回调校验 amount/currency。
- 修改：`controller/subscription_payment_creem.go` —— Creem checkout 写入 provider-proven 快照，回调校验 amount/currency。
- 修改：`controller/subscription_payment_completion.go` —— 统一完成路径使用订单快照校验；事件创建和后处理入口在任务 2 实现。
- 测试：`controller/subscription_payment_kyren_test.go`
- 测试：`controller/subscription_trial_purchase_test.go`
- 创建或修改：`controller/subscription_payment_stripe_test.go`
- 创建或修改：`controller/subscription_payment_creem_test.go`

- [ ] **步骤 1：编写失败测试：用户奖励模式默认值与归一化**

在 `model/invitation_commission_test.go` 增加：

```go
func TestNormalizeInvitationRewardModeDefaultsToSubscription(t *testing.T) {
    assert.Equal(t, InvitationRewardModeSubscription, NormalizeInvitationRewardMode(""))
    assert.Equal(t, InvitationRewardModeSubscription, NormalizeInvitationRewardMode("bad"))
    assert.Equal(t, InvitationRewardModeSubscription, NormalizeInvitationRewardMode(" subscription "))
    assert.Equal(t, InvitationRewardModeCommission, NormalizeInvitationRewardMode("commission"))
}

func TestUserInvitationRewardModeDefaultMigratesAsSubscription(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.AutoMigrate(&User{}))
    require.NoError(t, DB.Create(&User{Id: 9101, Username: "mode-default", Status: common.UserStatusEnabled}).Error)

    var user User
    require.NoError(t, DB.First(&user, 9101).Error)
    assert.Equal(t, InvitationRewardModeSubscription, user.NormalizedInvitationRewardMode())
}
```

- [ ] **步骤 2：编写失败测试：新增返佣表可迁移且复合唯一索引生效**

继续在 `model/invitation_commission_test.go` 增加：

```go
func TestInvitationCommissionModelsAutoMigrateAndSourceUniqueness(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.AutoMigrate(
        &InvitationCommissionAccount{},
        &InvitationRewardEvent{},
        &InvitationCommissionRecord{},
        &InvitationCommissionLedger{},
        &InvitationCommissionWithdrawal{},
    ))

    first := InvitationRewardEvent{
        InviterId: 1, InviteeId: 2,
        SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 100,
        Status: InvitationRewardEventStatusActive, CreatedAt: common.GetTimestamp(),
    }
    require.NoError(t, DB.Create(&first).Error)
    duplicate := first
    duplicate.Id = 0
    require.Error(t, DB.Create(&duplicate).Error)

    redemptionEvent := InvitationRewardEvent{
        InviterId: 1, InviteeId: 3,
        SourceType: InvitationRewardEventSourceSubscriptionRedemption, SourceId: 101, SourceRedemptionId: 101,
        Status: InvitationRewardEventStatusActive, CreatedAt: common.GetTimestamp(),
    }
    require.NoError(t, DB.Create(&redemptionEvent).Error)

    account := InvitationCommissionAccount{UserId: 1, AvailableCents: 10}
    require.NoError(t, DB.Create(&account).Error)
    require.Error(t, DB.Create(&InvitationCommissionAccount{UserId: 1}).Error)
}

func TestSubscriptionRedemptionStoresAmountSnapshot(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.AutoMigrate(&Redemption{}))

    redemption := Redemption{UserId: 1, Name: "sub-snapshot", Key: "sub-snapshot-key", Type: RedemptionTypeSubscription, PlanId: 9101, AmountCents: 8000, Currency: "CNY", Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
    require.NoError(t, DB.Create(&redemption).Error)

    var saved Redemption
    require.NoError(t, DB.Where("`key` = ?", "sub-snapshot-key").First(&saved).Error)
    assert.Equal(t, int64(8000), saved.AmountCents)
    assert.Equal(t, "CNY", saved.Currency)
}

func TestSubscriptionPlanPriceAmountSnapshotUsesDecimalCents(t *testing.T) {
    cases := []struct {
        name string
        plan SubscriptionPlan
        wantCents int64
        wantCurrency string
        wantOK bool
    }{
        {name: "one cent", plan: SubscriptionPlan{PriceAmount: 0.01, Currency: "CNY"}, wantCents: 1, wantCurrency: "CNY", wantOK: true},
        {name: "decimal price", plan: SubscriptionPlan{PriceAmount: 39.99, Currency: "CNY"}, wantCents: 3999, wantCurrency: "CNY", wantOK: true},
        {name: "non cny", plan: SubscriptionPlan{PriceAmount: 39.99, Currency: "USD"}, wantCents: 3999, wantCurrency: "USD", wantOK: true},
        {name: "overflow", plan: SubscriptionPlan{PriceAmount: math.MaxFloat64, Currency: "CNY"}, wantOK: false},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            cents, currency, ok := SubscriptionPlanAmountSnapshot(&tc.plan)
            assert.Equal(t, tc.wantOK, ok)
            assert.Equal(t, tc.wantCents, cents)
            assert.Equal(t, tc.wantCurrency, currency)
        })
    }
}

```

在 `setting/operation_setting/invitation_commission_setting_test.go` 增加：

```go
func TestValidateInvitationCommissionSettingRejectsInvalidRate(t *testing.T) {
    valid := InvitationCommissionSetting{Enabled: true, RateBps: 10000, MinimumWithdrawCents: 1000, MinimumTransferCents: 1}
    require.NoError(t, ValidateInvitationCommissionSetting(valid))

    invalid := valid
    invalid.RateBps = 10001
    require.Error(t, ValidateInvitationCommissionSetting(invalid))

    invalid = valid
    invalid.RateBps = -1
    require.Error(t, ValidateInvitationCommissionSetting(invalid))
}
```

在 `controller/option_invitation_commission_test.go` 增加持久化入口测试：

```go
func TestUpdateOptionRejectsInvalidInvitationCommissionRate(t *testing.T) {
    db := setupTokenControllerTestDB(t)
    require.NoError(t, db.AutoMigrate(&model.Option{}))
    originalOptionMap := common.OptionMap
    common.OptionMap = map[string]string{}
    t.Cleanup(func() { common.OptionMap = originalOptionMap })

    ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/option/", map[string]any{
        "key": "invitation_commission_setting.rate_bps",
        "value": "10001",
    }, 1)

    UpdateOption(ctx)

    require.Equal(t, http.StatusOK, recorder.Code)
    require.Contains(t, recorder.Body.String(), `"success":false`)
    var count int64
    require.NoError(t, db.Model(&model.Option{}).Where("`key` = ?", "invitation_commission_setting.rate_bps").Count(&count).Error)
    assert.Equal(t, int64(0), count)
}
```


- [ ] **步骤 3：编写失败测试：订单快照字段和快照来源**

在 `controller/subscription_balance_purchase_test.go` 增加账户余额购买断言：

```go
func TestSubscriptionBalancePurchaseStoresCNYAmountSnapshot(t *testing.T) {
    setupSubscriptionBalancePurchaseTestDB(t)
    userID := 9551
    require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance-snapshot", Status: common.UserStatusEnabled, Quota: 6000}).Error)
    code := "balance_snapshot"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9552, Title: "Balance Snapshot", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

    recorder := performBalancePayRequest(t, userID, `{"plan_id":9552,"idempotency_key":"snapshot"}`)

    require.Equal(t, http.StatusOK, recorder.Code)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9552).First(&order).Error)
    assert.Equal(t, int64(4000), order.AmountCents)
    assert.Equal(t, "CNY", order.Currency)
}
```

在 `controller/subscription_payment_kyren_test.go` 增加 Kyren 无法证明金额时的快照断言：
同时更新 `setupKyrenPaymentControllerTestDB`，把 `model.InvitationRewardEvent{}`、`model.InvitationCommissionRecord{}` 纳入 AutoMigrate；Kyren 快照和 mismatch 测试不得依赖单测内临时迁移掩盖正式 helper 缺表。

```go
func TestKyrenSubscriptionOrderStoresEmptySnapshotWhenCurrencyUnsupported(t *testing.T) {
    setupKyrenPaymentControllerTestDB(t)
    userID := 9561
    seedKyrenPaymentUser(t, userID)
    plan := seedKyrenPaymentPlan(t, 9562, "prod_kyren_snapshot", 1000, 1)

    tradeNo := "kyren-sub-empty-snapshot"
    require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
        UserId: userID, PlanId: plan.Id, Money: 40, TradeNo: tradeNo,
        PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
        Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
        KyrenSnapshot: kyrenPaymentSnapshotJSON(t, plan.KyrenProductId, "40.00", "USD"),
        EntitlementSnapshot: kyrenEntitlementSnapshotJSON(t, &plan),
    }).Error)

    payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, plan.KyrenProductId, "40.00", "USD")
    recorder := performSignedKyrenWebhook(t, payload)

    require.Equal(t, http.StatusOK, recorder.Code)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
    assert.Equal(t, int64(0), order.AmountCents)
    assert.Equal(t, "", order.Currency)
}

func TestKyrenSubscriptionOrderStoresCNYAmountSnapshot(t *testing.T) {
    setupKyrenPaymentControllerTestDB(t)
    userID := 9568
    seedKyrenPaymentUser(t, userID)
    plan := seedKyrenPaymentPlan(t, 9570, "prod_kyren_cny_snapshot", 1000, 1)
    tradeNo := "kyren-sub-cny-snapshot"
    require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
        UserId: userID, PlanId: plan.Id, Money: 40, TradeNo: tradeNo,
        PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
        Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
        KyrenSnapshot: kyrenPaymentSnapshotJSON(t, plan.KyrenProductId, "40.00", "CNY"),
        EntitlementSnapshot: kyrenEntitlementSnapshotJSON(t, &plan),
    }).Error)

    payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, plan.KyrenProductId, "40.00", "CNY")
    recorder := performSignedKyrenWebhook(t, payload)

    require.Equal(t, http.StatusOK, recorder.Code)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
    assert.Equal(t, int64(4000), order.AmountCents)
    assert.Equal(t, "CNY", order.Currency)
}

func TestKyrenSubscriptionCompletionRejectsProductAmountCurrencyMismatch(t *testing.T) {
    setupKyrenPaymentControllerTestDB(t)
    inviterID := 9571
    inviteeID := 9572
    require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "kyren-mismatch-inviter", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
    seedKyrenPaymentUser(t, inviteeID)
    require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviteeID).Update("inviter_id", inviterID).Error)
    plan := seedKyrenPaymentPlan(t, 9573, "prod_kyren_mismatch_snapshot", 1000, 1)
    for _, tc := range []struct {
        name string
        productId string
        amount string
        currency string
    }{
        {name: "product", productId: "prod_kyren_other_snapshot", amount: "40.00", currency: "CNY"},
        {name: "amount", productId: plan.KyrenProductId, amount: "41.00", currency: "CNY"},
        {name: "currency", productId: plan.KyrenProductId, amount: "40.00", currency: "USD"},
    } {
        t.Run(tc.name, func(t *testing.T) {
            tradeNo := "kyren-sub-snapshot-mismatch-" + tc.name
            require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
                UserId: inviteeID, PlanId: plan.Id, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: tradeNo,
                PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
                Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
                KyrenSnapshot: kyrenPaymentSnapshotJSON(t, plan.KyrenProductId, "40.00", "CNY"),
                EntitlementSnapshot: kyrenEntitlementSnapshotJSON(t, &plan),
            }).Error)

            payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, tc.productId, tc.amount, tc.currency)
            recorder := performSignedKyrenWebhook(t, payload)

            require.Equal(t, http.StatusOK, recorder.Code)
            var order model.SubscriptionOrder
            require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
            assert.Equal(t, common.TopUpStatusPending, order.Status)
            var events int64
            require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&events).Error)
            assert.Equal(t, int64(0), events)
            var records int64
            require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", order.Id).Count(&records).Error)
            assert.Equal(t, int64(0), records)
        })
    }
}
```
同时更新 `setupSubscriptionTrialPurchaseTest`，把 `model.InvitationRewardEvent{}`、`model.InvitationCommissionRecord{}` 纳入 AutoMigrate；Stripe/Creem mismatch 测试会统计事件表，后续 handler 路径也可能访问返佣记录表，不得因为 helper 缺表导致失败。

在 `controller/subscription_trial_purchase_test.go` 增加 Epay 订阅下单快照断言：

```go
func TestSubscriptionEpayStoresSubmittedCNYAmountSnapshot(t *testing.T) {
    setupSubscriptionTrialPurchaseTest(t)
    seedSubscriptionPurchasePlan(t, 9563, false, true, 40)
    operation_setting.PayAddress = "https://pay.example.com"
    operation_setting.EpayId = "epay_id"
    operation_setting.EpayKey = "epay_key"

    recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9563,"payment_method":"alipay"}`)

    require.Equal(t, http.StatusOK, recorder.Code)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9563).First(&order).Error)
    assert.Equal(t, int64(4000), order.AmountCents)
    assert.Equal(t, "CNY", order.Currency)
}
```

在 `controller/subscription_trial_purchase_test.go` 增加 Epay 回调金额 mismatch 断言：创建 Epay 订阅订单后，模拟已验签回调返回不同 `Money`，预期订单仍为 pending / 失败，且 `InvitationRewardEvent` 和 `InvitationCommissionRecord` 都没有创建。

```go
func TestSubscriptionEpayRejectsAmountMismatch(t *testing.T) {
    setupSubscriptionTrialPurchaseTest(t)
    seedSubscriptionPurchasePlan(t, 9569, false, true, 40)
    operation_setting.PayAddress = "https://pay.example.com"
    operation_setting.EpayId = "epay_id"
    operation_setting.EpayKey = "epay_key"

    recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9569,"payment_method":"alipay"}`)
    require.Equal(t, http.StatusOK, recorder.Code)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9569).First(&order).Error)
    require.Equal(t, int64(4000), order.AmountCents)

    callback := signedEpaySubscriptionCallback(t, order.TradeNo, "39.99")
    callbackRecorder := performEpaySubscriptionCallback(callback)

    require.Equal(t, http.StatusOK, callbackRecorder.Code)
    require.NoError(t, model.DB.First(&order, order.Id).Error)
    assert.NotEqual(t, common.TopUpStatusSuccess, order.Status)
    var events int64
    require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&events).Error)
    assert.Equal(t, int64(0), events)
    var records int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", order.Id).Count(&records).Error)
    assert.Equal(t, int64(0), records)
}
```

在 `controller/subscription_payment_stripe_test.go` 创建或增加 Stripe 快照来源与回调校验断言。实现时先在 `controller/subscription_payment_stripe.go` 抽出可替换的 checkout 创建函数，测试 helper 固定为 `SetStripeSubscriptionCheckoutForTest(t, fake)`：

```go
func TestStripeSubscriptionOrderStoresEmptySnapshotWhenCheckoutAmountNotVerified(t *testing.T) {
    setupSubscriptionTrialPurchaseTest(t)
    seedSubscriptionPurchasePlan(t, 9564, false, true, 40)
    setting.StripeApiSecret = "sk_test_123"
    setting.StripeWebhookSecret = "whsec_test"
    SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
        return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/checkout", AmountCents: 0, Currency: ""}, nil
    })

    recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9564}`)

    require.Equal(t, http.StatusOK, recorder.Code)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9564).First(&order).Error)
    assert.Equal(t, int64(0), order.AmountCents)
    assert.Equal(t, "", order.Currency)
}

func TestStripeSubscriptionOrderStoresCheckoutAmountSnapshotWhenVerified(t *testing.T) {
    setupSubscriptionTrialPurchaseTest(t)
    seedSubscriptionPurchasePlan(t, 9568, false, true, 40)
    setting.StripeApiSecret = "sk_test_123"
    setting.StripeWebhookSecret = "whsec_test"
    SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
        return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/checkout", AmountCents: 4000, Currency: "CNY"}, nil
    })

    recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9568}`)

    require.Equal(t, http.StatusOK, recorder.Code)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9568).First(&order).Error)
    assert.Equal(t, int64(4000), order.AmountCents)
    assert.Equal(t, "CNY", order.Currency)
}

func TestStripeSubscriptionCompletionRejectsAmountCurrencyMismatch(t *testing.T) {
    setupSubscriptionTrialPurchaseTest(t)
    seedSubscriptionPurchasePlan(t, 9565, false, true, 40)
    require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: 8801, PlanId: 9565, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: "stripe-snapshot-mismatch", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}).Error)

    err := completeSubscriptionOrderAndEvaluateInvitation("stripe-snapshot-mismatch", `{"amount_total":"4100","currency":"CNY"}`, model.PaymentProviderStripe, model.PaymentMethodStripe)

    require.Error(t, err)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("trade_no = ?", "stripe-snapshot-mismatch").First(&order).Error)
    assert.Equal(t, common.TopUpStatusPending, order.Status)
    var events int64
    require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&events).Error)
    assert.Equal(t, int64(0), events)
}
```

在 `controller/subscription_payment_creem_test.go` 创建或增加 Creem 快照来源与回调校验断言。实现时先在 `controller/subscription_payment_creem.go` 抽出可替换的 checkout 创建函数，测试 helper 固定为 `SetCreemSubscriptionCheckoutForTest(t, fake)`：

```go
func TestCreemSubscriptionOrderStoresCheckoutAmountSnapshot(t *testing.T) {
    setupSubscriptionTrialPurchaseTest(t)
    seedSubscriptionPurchasePlan(t, 9566, false, true, 40)
    setting.CreemWebhookSecret = "creem_secret"
    operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
    SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
        return CreemSubscriptionCheckoutResult{URL: "https://creem.test/checkout", AmountCents: 4000, Currency: "CNY"}, nil
    })

    recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9566}`)

    require.Equal(t, http.StatusOK, recorder.Code)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9566).First(&order).Error)
    assert.Equal(t, int64(4000), order.AmountCents)
    assert.Equal(t, "CNY", order.Currency)
}

func TestCreemSubscriptionCompletionRejectsAmountCurrencyMismatch(t *testing.T) {
    setupSubscriptionTrialPurchaseTest(t)
    seedSubscriptionPurchasePlan(t, 9567, false, true, 40)
    require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: 8801, PlanId: 9567, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: "creem-snapshot-mismatch", PaymentProvider: model.PaymentProviderCreem, PaymentMethod: model.PaymentMethodCreem, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}).Error)

    err := completeSubscriptionOrderAndEvaluateInvitation("creem-snapshot-mismatch", `{"amount_total":"4100","currency":"CNY"}`, model.PaymentProviderCreem, model.PaymentMethodCreem)

    require.Error(t, err)
    var order model.SubscriptionOrder
    require.NoError(t, model.DB.Where("trade_no = ?", "creem-snapshot-mismatch").First(&order).Error)
    assert.Equal(t, common.TopUpStatusPending, order.Status)
    var events int64
    require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&events).Error)
    assert.Equal(t, int64(0), events)
}
```

在 `controller/redemption_cny_test.go` 增加订阅兑换码创建 / 更新时的套餐价格快照断言；同时把 `setupRedemptionCNYTestDB` 的 AutoMigrate 扩展为包含 `model.InvitationRewardEvent{}`，避免兑换订阅时事件表缺失：

```go
func TestAddSubscriptionRedemptionStoresPlanAmountSnapshot(t *testing.T) {
    setupRedemptionCNYTestDB(t)
    code := "snapshot-redemption-plan"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9574, Title: "Snapshot Redemption", PriceAmount: 80, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 2, BusinessCode: &code}).Error)

    created, err := buildRedemptionsForCreate(1, model.Redemption{Name: "sub-snapshot", Type: model.RedemptionTypeSubscription, PlanId: 9574, Count: 1}, func() string { return "sub-snapshot-key" })

    require.NoError(t, err)
    require.Len(t, created, 1)
    assert.Equal(t, model.RedemptionTypeSubscription, created[0].Type)
    assert.Equal(t, 9574, created[0].PlanId)
    assert.Zero(t, created[0].Quota)
    assert.Equal(t, int64(8000), created[0].AmountCents)
    assert.Equal(t, "CNY", created[0].Currency)
}

func TestUpdateSubscriptionRedemptionRefreshesPlanAmountSnapshot(t *testing.T) {
    setupRedemptionCNYTestDB(t)
    code := "update-snapshot-redemption-plan"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9575, Title: "Update Snapshot Redemption", PriceAmount: 120, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 2000, ConcurrencyLimit: 3, BusinessCode: &code}).Error)
    existing := &model.Redemption{Name: "old", Type: model.RedemptionTypeWallet, Quota: 1000, Count: 1}

    err := applyRedemptionUpdate(existing, model.Redemption{Name: "new", Type: model.RedemptionTypeSubscription, PlanId: 9575, ExpiredTime: 0})

    require.NoError(t, err)
    assert.Equal(t, model.RedemptionTypeSubscription, existing.Type)
    assert.Equal(t, 9575, existing.PlanId)
    assert.Zero(t, existing.Quota)
    assert.Equal(t, int64(12000), existing.AmountCents)
    assert.Equal(t, "CNY", existing.Currency)
}

func TestUpdateUsedSubscriptionRedemptionRejectsSnapshotMutation(t *testing.T) {
    setupRedemptionCNYTestDB(t)
    oldCode := "used-snapshot-old-plan"
    newCode := "used-snapshot-new-plan"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9576, Title: "Used Snapshot Old", PriceAmount: 80, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 2, BusinessCode: &oldCode}).Error)
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9577, Title: "Used Snapshot New", PriceAmount: 120, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 2000, ConcurrencyLimit: 3, BusinessCode: &newCode}).Error)
    existing := &model.Redemption{Id: 9578, Name: "used", Type: model.RedemptionTypeSubscription, PlanId: 9576, AmountCents: 8000, Currency: "CNY", Status: common.RedemptionCodeStatusUsed}

    err := applyRedemptionUpdate(existing, model.Redemption{Name: "mutated", Type: model.RedemptionTypeSubscription, PlanId: 9577})

    require.Error(t, err)
    assert.Equal(t, model.RedemptionTypeSubscription, existing.Type)
    assert.Equal(t, 9576, existing.PlanId)
    assert.Equal(t, int64(8000), existing.AmountCents)
    assert.Equal(t, "CNY", existing.Currency)
}
```

- [ ] **步骤 4：运行测试验证失败**

运行：

```bash
go test ./model -run 'Test(NormalizeInvitationRewardModeDefaultsToSubscription|UserInvitationRewardModeDefaultMigratesAsSubscription|InvitationCommissionModelsAutoMigrateAndSourceUniqueness|SubscriptionRedemptionStoresAmountSnapshot|SubscriptionPlanPriceAmountSnapshotUsesDecimalCents)' -count=1
go test ./setting/operation_setting -run 'TestValidateInvitationCommissionSettingRejectsInvalidRate' -count=1
go test ./controller -run 'TestUpdateOptionRejectsInvalidInvitationCommissionRate|TestSubscriptionBalancePurchaseStoresCNYAmountSnapshot|TestKyrenSubscription(OrderStoresEmptySnapshotWhenCurrencyUnsupported|OrderStoresCNYAmountSnapshot|CompletionRejectsProductAmountCurrencyMismatch)|TestSubscriptionEpay(StoresSubmittedCNYAmountSnapshot|RejectsAmountMismatch)|TestStripeSubscription(OrderStoresEmptySnapshotWhenCheckoutAmountNotVerified|OrderStoresCheckoutAmountSnapshotWhenVerified|CompletionRejectsAmountCurrencyMismatch)|TestCreemSubscription(OrderStoresCheckoutAmountSnapshot|CompletionRejectsAmountCurrencyMismatch)|Test(Add|Update|UpdateUsed)SubscriptionRedemption.*Snapshot' -count=1
```

预期：FAIL，原因包括缺少模型类型、缺少 `InvitationRewardMode`、缺少 `AmountCents/Currency` 字段、兑换码快照字段或销售来源创建未写快照。

在 `model/invitation_commission_test.go` 同时增加本任务使用的 helper：

```go
func seedInvitationCommissionPlan(t *testing.T, id int, code string, price float64, currency string) *SubscriptionPlan {
    t.Helper()
    plan := &SubscriptionPlan{Id: id, Title: code, PriceAmount: price, Currency: currency, Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code, DurationUnit: SubscriptionDurationDay, DurationValue: 30}
    require.NoError(t, DB.Create(plan).Error)
    return plan
}
```


- [ ] **步骤 5：实现模型和配置最小代码**

实现要求：

```go
const (
    InvitationRewardModeSubscription = "subscription"
    InvitationRewardModeCommission   = "commission"
)

func NormalizeInvitationRewardMode(mode string) string {
    switch strings.TrimSpace(mode) {
    case InvitationRewardModeCommission:
        return InvitationRewardModeCommission
    default:
        return InvitationRewardModeSubscription
    }
}

func (user *User) NormalizedInvitationRewardMode() string {
    if user == nil {
        return InvitationRewardModeSubscription
    }
    return NormalizeInvitationRewardMode(user.InvitationRewardMode)
}
```

`User` 增加：

```go
InvitationRewardMode string `json:"invitation_reward_mode" gorm:"type:varchar(32);default:'subscription'"`
```

`SubscriptionOrder` 和订阅兑换码 `Redemption` 增加：

```go
AmountCents int64  `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
Currency    string `json:"currency" gorm:"type:varchar(8);not null;default:''"`
```

`model/invitation_commission.go` 定义规格中的 5 个模型和常量。`InvitationRewardEvent` 不包含 `RewardMode` 字段；它只记录销售来源事实、金额快照和新增订阅区间。`InvitationCommissionRecord.SourceCurrency` 默认必须为 `''`，不能为 `CNY`。必须定义固定 reason 常量：`InvitationCommissionReasonUnsupportedCurrency = "unsupported_currency"`、`InvitationCommissionReasonInvalidSourceAmount = "invalid_source_amount"`、`InvitationCommissionReasonCommissionOverflow = "commission_overflow"`。`setting/operation_setting/invitation_commission_setting.go` 定义 `InvitationCommissionSetting`、默认值、`GetInvitationCommissionSetting()` 与 `ValidateInvitationCommissionSetting()`；`RateBps` 校验必须拒绝或运行时禁用 `>10000`。

`ValidateInvitationCommissionSetting()` 必须接入现有 Option / operation setting 持久化更新路径，管理员保存 `invitation_commission_setting.rate_bps` 时若值 `>10000` 必须返回错误并拒绝持久化；`config.GlobalConfig.Register("invitation_commission_setting", ...)` 与 DB key 前缀必须一致。若现有设置系统只能运行时兜底，也必须在 Task 4 的服务测试覆盖 `rate_bps > 10000` 不入账且不消耗来源。

`model/main.go` 的 `migrateDB` 和 `migrateDBFast` 增加：

```go
&InvitationCommissionAccount{},
&InvitationRewardEvent{},
&InvitationCommissionRecord{},
&InvitationCommissionLedger{},
&InvitationCommissionWithdrawal{},
```

- [ ] **步骤 6：实现销售来源快照写入**

写入规则：

- 账户余额购买：`AmountCents = int64(amount)`、`Currency = "CNY"`。
- Epay：创建支付单时把提交给 Epay 的 CNY 分写入 `AmountCents`，`Currency = "CNY"`；回调必须使用 decimal 解析 `verifyInfo.Money`，转换为 CNY 分并与 `SubscriptionOrder.AmountCents/Currency` 比较，不一致时保持 pending / 失败且不得创建 `InvitationRewardEvent` 或 `InvitationCommissionRecord`。回调不能按当前套餐价格或币种重算。
- Kyren：可证明为 CNY 时写 CNY 分；不能证明时写 `0` 和 `""`。Kyren 回调必须比较 provider 返回的 product、amount、currency 与持久订单快照；product mismatch、amount mismatch 或 currency mismatch 都必须拒绝完成订单，且不得创建 `InvitationRewardEvent` 或 `InvitationCommissionRecord`。Kyren 事件创建属于任务 2 的完成事务，不在任务 1 提前实现。
- Creem/Stripe：创建 checkout 时写入可由 provider checkout 创建结果或显式 price 查询证明的金额最小单位和币种；回调必须校验 provider 返回的 amount/currency 与订单快照一致。若现有 provider 响应不能证明金额和币种，则订单创建阶段写 `AmountCents = 0`、`Currency = ""`，回调不得补判。Stripe 使用 Price ID 且未查询到 price 金额/币种时属于无法证明，必须写 `0` 和 `""`；若测试 fake 或真实 price 查询返回 `AmountCents/Currency`，必须写入订单快照。
- 订阅兑换码：`buildRedemptionsForCreate` / `applyRedemptionUpdate` 在 `RedemptionTypeSubscription` 分支读取绑定套餐价格并写 `Redemption.AmountCents/Currency`；钱包兑换码不使用该快照字段；已使用订阅兑换码不得修改 `Type`、`PlanId`、`AmountCents` 或 `Currency` 审计快照。
- 套餐价格转分统一使用 `SubscriptionPlanAmountSnapshot(plan *SubscriptionPlan) (amountCents int64, currency string, ok bool)` helper；该 helper 必须使用 `shopspring/decimal` 或等价十进制转换，允许任意非空 ISO 币种并返回对应最小单位，拒绝空币种、负数、NaN/Inf 和超过 `int64` 的结果，不能用 `int64(price*100)` 直接截断。返佣阶段只允许 `currency == "CNY"` 入账，非 CNY 按 `unsupported_currency` 跳过。
- 返佣阶段不得再读取 `SubscriptionPlan.PriceAmount` 或 `Currency` 补判已落库来源事件。

- [ ] **步骤 7：运行测试验证通过**

运行步骤 4 命令，预期 PASS。

---

## 任务 2：销售来源完成事务内记录邀请来源事件

**文件：**
- 修改：`model/subscription.go`
- 修改：`model/redemption.go`
- 修改：`controller/subscription_payment_completion.go`
- 修改：`controller/subscription_payment_balance.go`
- 修改：`controller/user.go`
- 修改：`model/payment_method_guard_test.go` —— 适配 `CompleteSubscriptionOrder` 新返回值。
- 测试：`model/invitation_commission_test.go`
- 测试：`controller/invitation_entitlement_test.go`
- 测试：`controller/redemption_cny_test.go`
- [ ] **步骤 1：编写失败测试：订单完成只在 pending -> success 时创建销售来源事件**

在 `model/invitation_commission_test.go` 增加：

```go
func TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &InvitationRewardEvent{}))
    require.NoError(t, DB.Create(&User{Id: 9201, Username: "inviter", Status: common.UserStatusEnabled, InvitationRewardMode: InvitationRewardModeCommission}).Error)
    require.NoError(t, DB.Create(&User{Id: 9202, Username: "invitee", Status: common.UserStatusEnabled, InviterId: 9201}).Error)
    _ = seedInvitationCommissionPlan(t, 9203, "commission_plan", 100, "CNY")
    order := SubscriptionOrder{UserId: 9202, PlanId: 9203, Money: 100, AmountCents: 10000, Currency: "CNY", TradeNo: "source-at-transition", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
    require.NoError(t, DB.Create(&order).Error)

    var result *SubscriptionOrderCompletionResult
    require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
        var locked SubscriptionOrder
        require.NoError(t, tx.Where("trade_no = ?", order.TradeNo).First(&locked).Error)
        var err error
        result, err = CompleteSubscriptionOrderTx(tx, &locked, "{}", "alipay")
        return err
    }))

    require.NotNil(t, result)
    assert.True(t, result.Transitioned)
    assert.Equal(t, 9201, result.InviterId)
    require.Greater(t, result.SourceSubscriptionId, 0)
    assert.Greater(t, result.EventEndTime, result.EventStartTime)
    var event InvitationRewardEvent
    require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error)
    assert.Equal(t, 9201, event.InviterId)
    assert.Equal(t, 9202, event.InviteeId)
    assert.Equal(t, order.Id, event.SourceOrderId)
    assert.Equal(t, result.SourceSubscriptionId, event.SourceSubscriptionId)
    assert.Equal(t, int64(10000), event.SourceAmountCents)
    assert.Equal(t, "CNY", event.SourceCurrency)

    require.NoError(t, DB.Model(&User{}).Where("id = ?", 9201).Update("invitation_reward_mode", InvitationRewardModeSubscription).Error)
    require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
        var completed SubscriptionOrder
        require.NoError(t, tx.Where("trade_no = ?", order.TradeNo).First(&completed).Error)
        retry, err := CompleteSubscriptionOrderTx(tx, &completed, "{}", "alipay")
        require.NoError(t, err)
        assert.NotNil(t, retry)
        assert.False(t, retry.Transitioned)
        return nil
    }))
    var count int64
    require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&count).Error)
    assert.Equal(t, int64(1), count)
}
```

- [ ] **步骤 2：编写失败测试：续费事件区间只记录新增区间**

在同文件增加：

```go
func TestCompleteSubscriptionOrderTxEventIntervalUsesOnlyRenewalDelta(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &InvitationRewardEvent{}))
    now := common.GetTimestamp()
    require.NoError(t, DB.Create(&User{Id: 9211, Username: "renew-inviter", Status: common.UserStatusEnabled}).Error)
    require.NoError(t, DB.Create(&User{Id: 9212, Username: "renew-invitee", Status: common.UserStatusEnabled, InviterId: 9211}).Error)
    _ = seedInvitationCommissionPlan(t, 9213, "renew_plan", 50, "CNY")
    require.NoError(t, DB.Create(&UserSubscription{UserId: 9212, PlanId: 9213, Status: "active", StartTime: now - 100, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
    order := SubscriptionOrder{UserId: 9212, PlanId: 9213, AmountCents: 5000, Currency: "CNY", TradeNo: "renew-delta", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: now}
    require.NoError(t, DB.Create(&order).Error)

    require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
        var locked SubscriptionOrder
        require.NoError(t, tx.Where("trade_no = ?", order.TradeNo).First(&locked).Error)
        _, err := CompleteSubscriptionOrderTx(tx, &locked, "{}", "alipay")
        return err
    }))

    var event InvitationRewardEvent
    require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error)
    assert.Equal(t, now+86400, event.EventStartTime)
    assert.Greater(t, event.EventEndTime, event.EventStartTime)
}
```

- [ ] **步骤 3：编写失败测试：订阅兑换码创建销售来源事件并复制快照**

在 `model/invitation_commission_test.go` 增加：

```go
func TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &Redemption{}, &InvitationRewardEvent{}))
    require.NoError(t, DB.Create(&User{Id: 9221, Username: "redeem-inviter", Status: common.UserStatusEnabled, InvitationRewardMode: InvitationRewardModeCommission}).Error)
    require.NoError(t, DB.Create(&User{Id: 9222, Username: "redeem-invitee", Status: common.UserStatusEnabled, InviterId: 9221}).Error)
    _ = seedInvitationCommissionPlan(t, 9223, "redeem_plan", 80, "CNY")
    redemption := Redemption{Id: 9224, Key: "redeem-source-key", Status: common.RedemptionCodeStatusEnabled, Type: RedemptionTypeSubscription, PlanId: 9223, AmountCents: 8000, Currency: "CNY", CreatedTime: common.GetTimestamp()}
    require.NoError(t, DB.Create(&redemption).Error)

    result, err := Redeem("redeem-source-key", 9222)

    require.NoError(t, err)
    assert.Equal(t, RedemptionTypeSubscription, result.Type)
    var event InvitationRewardEvent
    require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionRedemption, redemption.Id).First(&event).Error)
    assert.Equal(t, redemption.Id, event.SourceRedemptionId)
    assert.Equal(t, int64(8000), event.SourceAmountCents)
    assert.Equal(t, "CNY", event.SourceCurrency)
}

func TestRedeemSubscriptionRedemptionRecordsEventForRewardIneligiblePlan(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &Redemption{}, &InvitationRewardEvent{}))
    require.NoError(t, DB.Create(&User{Id: 9225, Username: "redeem-ineligible-inviter", Status: common.UserStatusEnabled}).Error)
    require.NoError(t, DB.Create(&User{Id: 9226, Username: "redeem-ineligible-invitee", Status: common.UserStatusEnabled, InviterId: 9225}).Error)
    plan := seedInvitationCommissionPlan(t, 9227, "redeem_ineligible_plan", 80, "CNY")
    require.NoError(t, DB.Model(plan).Update("reward_eligible", false).Error)
    redemption := Redemption{Id: 9228, Key: "redeem-ineligible-key", Status: common.RedemptionCodeStatusEnabled, Type: RedemptionTypeSubscription, PlanId: 9227, AmountCents: 8000, Currency: "CNY", CreatedTime: common.GetTimestamp()}
    require.NoError(t, DB.Create(&redemption).Error)

    result, err := Redeem("redeem-ineligible-key", 9226)

    require.NoError(t, err)
    assert.Equal(t, RedemptionTypeSubscription, result.Type)
    var event InvitationRewardEvent
    require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionRedemption, redemption.Id).First(&event).Error)
    assert.Equal(t, redemption.Id, event.SourceRedemptionId)
    assert.Equal(t, int64(8000), event.SourceAmountCents)
    assert.Equal(t, "CNY", event.SourceCurrency)
}
```

- [ ] **步骤 4：编写失败测试：已完成订单重试仍返回可处理结果**

在 `model/invitation_commission_test.go` 增加：

```go
func TestCompleteSubscriptionOrderReturnsResultForSuccessRetry(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &InvitationRewardEvent{}))
    require.NoError(t, DB.Create(&User{Id: 9231, Username: "retry-inviter", Status: common.UserStatusEnabled, InvitationRewardMode: InvitationRewardModeCommission}).Error)
    require.NoError(t, DB.Create(&User{Id: 9232, Username: "retry-invitee", Status: common.UserStatusEnabled, InviterId: 9231}).Error)
    _ = seedInvitationCommissionPlan(t, 9233, "retry_plan", 60, "CNY")
    order := SubscriptionOrder{UserId: 9232, PlanId: 9233, AmountCents: 6000, Currency: "CNY", TradeNo: "retry-existing-event", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
    require.NoError(t, DB.Create(&order).Error)

    first, err := CompleteSubscriptionOrder("retry-existing-event", "{}", PaymentProviderEpay, "alipay")
    require.NoError(t, err)
    require.NotNil(t, first)
    assert.True(t, first.Transitioned)

    retry, err := CompleteSubscriptionOrder("retry-existing-event", "{}", PaymentProviderEpay, "alipay")
    require.NoError(t, err)
    require.NotNil(t, retry)
    assert.False(t, retry.Transitioned)
    assert.Equal(t, first.InviterId, retry.InviterId)
    assert.Equal(t, first.SourceSubscriptionId, retry.SourceSubscriptionId)

    var count int64
    require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&count).Error)
    assert.Equal(t, int64(1), count)
}
```

该测试保障后处理服务在首次事务提交后失败时，重复回调仍能拿到非 nil completion，并重新调用幂等事件处理服务。

- [ ] **步骤 5：编写失败测试：并发订单完成和订阅兑换码兑换只能原子 claim 一次**

在 `model/invitation_commission_test.go` 增加并发测试：同一 pending 订阅订单由多个 goroutine 同时调用完成函数时，只允许 1 次 `pending -> success` 状态迁移，只创建 1 个 `UserSubscription` 区间和 1 条 `InvitationRewardEvent`；其他调用必须返回 `Transitioned=false` 的幂等结果或可重试错误，但不得再次授予权益。

同时增加订阅兑换码并发测试：两个不同 invitee 并发兑换同一个 enabled 订阅兑换码时，只允许一个成功，`redemptions.used_user_id` 不得被后提交用户覆盖，只创建 1 个 `UserSubscription` 和 1 条 `subscription_redemption` 事件。

这两个测试必须在 SQLite 下也能稳定失败 / 通过，不能依赖 `FOR UPDATE` 行级锁；实现必须用条件更新和 `RowsAffected` claim 状态。

- [ ] **步骤 6：编写失败测试：后处理失败后可通过成功订单重试和后台扫表恢复**

在 `controller/invitation_entitlement_test.go` 增加控制器级失败注入测试。实现时在 `controller/subscription_payment_completion.go` 提供仅测试使用的订单 handler 替换 helper：`SetInvitationRewardOrderHandlerForTest(t, handler func(orderId int) error)`；在 `controller/user.go` 或兑换码后处理所在文件提供兑换码 handler 替换 helper：`SetInvitationRewardRedemptionHandlerForTest(t, handler func(redemptionId int) error)`。任务 2 的默认生产订单 handler 先使用兼容实现：按 `orderId` 读取订单并调用现有 `service.TryEnsureInvitationEntitlementForPaidUser(order.UserId)`；任务 4 实现完整 `service.HandleInvitationRewardForCompletedSubscriptionOrder` 后再切换默认 handler。

测试代码沿用原 `TestCompleteSubscriptionOrderRetriesInvitationRewardHandlerForSuccessfulOrder`，但只断言 handler 被同一 order id 调用两次、`InvitationRewardEvent` 只有 1 条，不断言返佣记录。

在 `controller/redemption_cny_test.go` 增加 `TestRedeemSubscriptionRedemptionInvokesInvitationRewardHandlerAfterCommit`：通过测试 helper 替换 `handleInvitationRewardForSubscriptionRedemption`，调用用户兑换接口成功兑换订阅兑换码，断言 handler 收到 `redemption.Id`，且 handler 被调用时能在 DB 中读到 `status = used` 的兑换码和对应 `subscription_redemption` 事件。该测试只验证提交后处理入口，不断言返佣记录。

- [ ] **步骤 7：运行测试验证失败**

```bash
go test ./model -run 'TestCompleteSubscriptionOrder(TxCreatesInvitationRewardEventAtTransition|TxEventIntervalUsesOnlyRenewalDelta|ReturnsResultForSuccessRetry|ConcurrentClaimCreatesSingleSubscriptionAndEvent)|TestRedeemSubscriptionRedemption(CreatesInvitationRewardEvent|RecordsEventForRewardIneligiblePlan|ConcurrentClaimCreatesSingleSubscriptionAndEvent)' -count=1
go test ./controller -run 'TestCompleteSubscriptionOrderRetriesInvitationRewardHandlerForSuccessfulOrder|TestRedeemSubscriptionRedemptionInvokesInvitationRewardHandlerAfterCommit' -count=1
```

预期：FAIL，原因包括缺少 `SubscriptionOrderCompletionResult`、没有事件创建、兑换码兑换未创建事件或事件区间不正确。

- [ ] **步骤 8：实现订单 / 兑换码完成结果与事件创建**

将 `CompleteSubscriptionOrder` 和 `CompleteSubscriptionOrderTx` 返回值都改为包含完整结果：

```go
type SubscriptionOrderCompletionResult struct {
    Subscription *UserSubscription
    Transitioned bool
    SourceSubscriptionId int
    EventStartTime int64
    EventEndTime int64
    InviterId int
}
```

实现要点：

1. `CreateUserSubscriptionFromPlanTx` 或包装函数必须返回本次新增 / 续费前后的区间。若复用旧函数，新增 `CreateUserSubscriptionFromPlanWithResultTx`，不要让调用方猜测区间。
2. `CompleteSubscriptionOrderTx` 必须在授予权益前用条件更新原子 claim：`UPDATE subscription_orders SET status = success ... WHERE id = ? AND status = pending`，检查 `RowsAffected`。claim 成功者创建 / 延长订阅并创建事件；claim 失败者重读成功订单和既有事件，返回 `Transitioned=false`，不得再次创建 / 延长订阅。不得依赖 `FOR UPDATE` 或进程内 `LockOrder` 保证跨库并发正确性。
3. `model.Redeem` 的订阅兑换码分支必须先用 `UPDATE redemptions SET status = used, used_user_id = ?, redeemed_time = ? WHERE id = ? AND status = enabled` 原子 claim；claim 失败只返回已使用错误或重读已使用结果，不得再次创建 / 延长订阅，不得覆盖 `used_user_id`。
4. 在同一事务中读取 invitee 的 `InviterId` 和套餐 `RewardEligible`；事件表不写奖励模式。
5. `plan.IsTrial == true` 或 `monthly_invite_entitlement` 套餐不得创建销售来源事件；`plan.RewardEligible == false` 仍必须记录销售来源事实事件，但奖励套餐与返佣 fresh 计算会过滤该事件。
6. `InvitationRewardEvent` 写入 `source_amount_cents = order.AmountCents`、`source_currency = order.Currency`，并关联 `source_order_id`。
7. `model.Redeem` 的订阅兑换码分支在创建 / 延长 `UserSubscription` 后创建 `source_type = subscription_redemption` 事件，复制 `Redemption.AmountCents/Currency`，并关联 `source_redemption_id`；不得因为绑定套餐当前 `RewardEligible == false` 而跳过事件。
8. 唯一键冲突时重读已有事件；重复回调或重复兑换不得重算来源。
9. `CompleteSubscriptionOrder` 包装函数必须返回 `(*SubscriptionOrderCompletionResult, error)`，现有调用点全部适配新返回类型。
10. Kyren entitlement snapshot 路径也必须设置 `EventStartTime/EventEndTime`，不能绕过事件表。

- [ ] **步骤 9：支付完成入口改为事件处理服务**

`controller/subscription_payment_completion.go` 中，外部支付完成后不再直接调用 `service.TryEnsureInvitationEntitlementForPaidUser(userId)`。改为：

```go
if completion != nil {
    if err := handleInvitationRewardForCompletedSubscriptionOrder(order.Id); err != nil {
        common.SysError("failed to handle invitation reward: " + err.Error())
        return err
    }
}
```

`controller/subscription_payment_balance.go` 必须把余额购买 helper 的返回值改成包含完成结果，例如 `(*model.SubscriptionOrder, *model.SubscriptionOrderCompletionResult, bool, error)`；余额购买完成后同样使用 `handleInvitationRewardForCompletedSubscriptionOrder(order.Id)`。不能引用未定义的 `completion`，也不能继续只按 `created` 调用旧奖励套餐逻辑。

正式返佣入账在销售来源事务提交后执行；事务内必须已经落库 `InvitationRewardEvent`。任务 2 的兼容 handler 只保留现有奖励套餐即时刷新，任务 4 的正式 handler 再按邀请人当前模式分发到返佣或奖励套餐。handler 失败时必须向调用方返回错误，外部 webhook 得到可重试响应，余额购买或幂等请求也能在成功订单上再次调用该服务。不能只 `SysError` 后返回成功。

`service.HandleInvitationRewardForCompletedSubscriptionOrder(orderId)` 和 `service.HandleInvitationRewardForSubscriptionRedemption(redemptionId)` 的完整分发在任务 4 实现；任务 2 只需要提供控制器侧可注入的 `handleInvitationRewardForCompletedSubscriptionOrder`、`handleInvitationRewardForSubscriptionRedemption` 变量和对应 `SetInvitationRewardOrderHandlerForTest`、`SetInvitationRewardRedemptionHandlerForTest` helper，确保所有支付完成路径在订单 handler 返回错误时向上返回错误。订阅兑换码已经在 `model.Redeem` 事务内持久化来源事件；`controller/user.go` 的兑换接口在事务提交后调用 redemption handler，handler 失败时记录 `SysError` 但不回滚已使用兑换码，后续由任务 4 的补偿任务处理。

任务 2 的默认 handler 使用现有奖励套餐兼容实现：订单 handler 按 `orderId` 读取订单并调用 `service.TryEnsureInvitationEntitlementForPaidUser(order.UserId)`；兑换码 handler 按 `redemptionId` 读取兑换码，若 `UsedUserId > 0` 则调用 `service.TryEnsureInvitationEntitlementForPaidUser(redemption.UsedUserId)`。默认 handler 不引用任务 4 尚未存在的服务函数；任务 4 完成后再切换为正式 service 函数。Kyren claimed 路径、普通外部支付路径和账户余额路径都必须使用同一订单 handler，不得绕过事件表。

`model.RedemptionResult` 增加 `RedemptionId int`，`model.Redeem` 成功后填充已使用的兑换码 ID，供 `controller/user.go` 调用 redemption handler。`main.go` 后台重试启动不在任务 2 接入；任务 4 实现 `StartInvitationRewardEventRetryTask` 后再统一从 `main.go` 启动，避免任务 2 验证阶段出现未定义符号。

- [ ] **步骤 10：运行测试验证通过**

运行步骤 7 命令，预期 PASS。再运行：
```bash
go test ./controller -run 'Test.*Subscription.*(Complete|Balance|Kyren|Entitlement)' -count=1
```

预期：相关旧测试仍 PASS。

---

## 任务 3：奖励套餐 mode-aware 保护与历史返佣来源回填

**文件：**
- 修改：`service/invitation_reward.go`
- 修改：`model/main.go`
- 创建或修改：`service/invitation_reward_test.go`
- 修改：`controller/invitation_entitlement_test.go`
- 测试：`model/invitation_commission_test.go`

- [ ] **步骤 1：编写失败测试：commission 邀请人查询权益不 upsert 套餐**

在 `service/invitation_reward_test.go` 增加：

```go
func TestGetInvitationEntitlementStatusSkipsCommissionInviterWithoutUpsert(t *testing.T) {
    truncate(t)
    require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.InvitationMonthlyEntitlement{}))
    at := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
    require.NoError(t, model.DB.Create(&model.User{Id: 9301, Username: "commission-inviter", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
    require.NoError(t, model.DB.Create(&model.User{Id: 9302, Username: "commission-child-a", Status: common.UserStatusEnabled, InviterId: 9301}).Error)
    require.NoError(t, model.DB.Create(&model.User{Id: 9303, Username: "commission-child-b", Status: common.UserStatusEnabled, InviterId: 9301}).Error)
    plan := seedInvitationRewardPlan(t, 9304, "commission_paid", true)
    seedActiveInviteeSubscription(t, 9302, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)
    seedActiveInviteeSubscription(t, 9303, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)

    status, err := GetInvitationEntitlementStatus(9301, at)

    require.NoError(t, err)
    require.NotNil(t, status)
    assert.False(t, status.Entitled)
    assert.Equal(t, 2, status.DirectInviteCount)
    assert.Equal(t, 2, status.QualifiedActiveCount)
    assert.Zero(t, status.RewardSubscriptionId)
    var entitlementCount int64
    require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 9301).Count(&entitlementCount).Error)
    assert.Equal(t, int64(0), entitlementCount)
    var rewardSubCount int64
    require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND grant_reason = ?", 9301, model.SubscriptionGrantMonthlyInviteEntitlement).Count(&rewardSubCount).Error)
    assert.Equal(t, int64(0), rewardSubCount)
}
```

- [ ] **步骤 2：编写保护测试：subscription 模式保留当前 active 订阅口径**

```go
func TestInvitationEntitlementKeepsExistingActiveSubscriptionCriteria(t *testing.T) {
    truncate(t)
    require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.InvitationMonthlyEntitlement{}, &model.InvitationRewardEvent{}))
    at := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
    seedInvitationRewardUsers(t, 9311, 9312, 9313, 9314)
    paidPlan := seedInvitationRewardPlan(t, 9315, "current_paid", true)
    seedActiveInviteeSubscription(t, 9312, paidPlan.Id, at, "redemption", "redemption")
    seedActiveInviteeSubscription(t, 9313, paidPlan.Id, at, "admin", "admin")
    require.NoError(t, model.DB.Create(&model.InvitationRewardEvent{InviterId: 9311, InviteeId: 9314, SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: 9316, EventStartTime: at.Add(-time.Hour).Unix(), EventEndTime: at.Add(24*time.Hour).Unix(), Status: model.InvitationRewardEventStatusActive}).Error)

    status, err := EnsureMonthlyInvitationEntitlement(9311, at)

    require.NoError(t, err)
    assert.True(t, status.Entitled)
    assert.Equal(t, 2, status.QualifiedActiveCount)
    assert.Equal(t, paidPlan.Id, status.RewardPlanId)
}
```

该测试刻意 seed 1 条没有 active `user_subscriptions` 支撑的 `InvitationRewardEvent`，用于防止实现把奖励套餐改成事件表口径；当前代码中 redemption/admin 发放的 active 合格订阅仍应按现有口径计入奖励套餐。

- [ ] **步骤 3：编写失败测试：sweep 跳过当前 commission 邀请人**

```go
func TestRunMonthlyInvitationEntitlementSweepSkipsCommissionInviters(t *testing.T) {
    truncate(t)
    require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.InvitationMonthlyEntitlement{}))
    at := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
    require.NoError(t, model.DB.Create(&model.User{Id: 9321, Username: "sweep-commission", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
    require.NoError(t, model.DB.Create(&model.User{Id: 9322, Username: "sweep-child-a", Status: common.UserStatusEnabled, InviterId: 9321}).Error)
    require.NoError(t, model.DB.Create(&model.User{Id: 9323, Username: "sweep-child-b", Status: common.UserStatusEnabled, InviterId: 9321}).Error)
    plan := seedInvitationRewardPlan(t, 9324, "sweep_paid", true)
    seedActiveInviteeSubscription(t, 9322, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)
    seedActiveInviteeSubscription(t, 9323, plan.Id, at, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)

    processed, err := RunMonthlyInvitationEntitlementSweep(at, 10)

    require.NoError(t, err)
    assert.Equal(t, 0, processed)
    var entitlementCount int64
    require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 9321).Count(&entitlementCount).Error)
    assert.Equal(t, int64(0), entitlementCount)
}
```

- [ ] **步骤 4：编写失败测试：历史返佣来源事件幂等回填**

在 `model/invitation_commission_test.go` 增加：

```go
func TestBackfillLegacyInvitationRewardEventsPreservesExistingCommissionSources(t *testing.T) {
    truncateTables(t)
    require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &InvitationRewardEvent{}))
    now := common.GetTimestamp()
    paidPlan := seedInvitationCommissionPlan(t, 9331, "legacy_paid", 80, "CNY")
    trialPlan := seedInvitationCommissionPlan(t, 9332, "legacy_trial", 0, "CNY")
    require.NoError(t, DB.Model(trialPlan).Update("is_trial", true).Error)
    rewardPlan := seedInvitationCommissionPlan(t, 9333, "legacy_reward", 0, "CNY")
    ineligiblePlan := seedInvitationCommissionPlan(t, 9330, "legacy_ineligible", 80, "CNY")
    require.NoError(t, DB.Model(ineligiblePlan).Update("reward_eligible", false).Error)
    require.NoError(t, DB.Create(&User{Id: 9334, Username: "legacy-inviter", Status: common.UserStatusEnabled}).Error)
    for _, userID := range []int{9335, 9336, 9337, 9338, 9349, 9352, 9353, 9360} {
        require.NoError(t, DB.Create(&User{Id: userID, Username: fmt.Sprintf("legacy-child-%d", userID), Status: common.UserStatusEnabled, InviterId: 9334}).Error)
    }
    require.NoError(t, DB.Create(&UserSubscription{Id: 9339, UserId: 9335, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
    require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9340, UserId: 9335, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-paid-order", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 9341, UserId: 9336, PlanId: trialPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: "trial_code", Source: "trial_code"}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 9342, UserId: 9337, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: "admin", Source: "admin"}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 9343, UserId: 9338, PlanId: rewardPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 9350, UserId: 9349, PlanId: ineligiblePlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
    require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9351, UserId: 9349, PlanId: ineligiblePlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-ineligible-order", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 9354, UserId: 9352, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
    require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9355, UserId: 9352, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-ambiguous-a", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
    require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9356, UserId: 9352, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 90, AmountCents: 9000, Currency: "CNY", TradeNo: "legacy-ambiguous-b", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3400}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 9357, UserId: 9353, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 9361, UserId: 9360, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
    require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9362, UserId: 9360, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-existing-order", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
    require.NoError(t, DB.Create(&InvitationRewardEvent{InviterId: 9334, InviteeId: 9360, SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 9362, SourceOrderId: 9362, SourceSubscriptionId: 9361, SourceAmountCents: 8000, SourceCurrency: "CNY", EventStartTime: now - 3600, EventEndTime: now + 86400, Status: InvitationRewardEventStatusActive, CreatedAt: now}).Error)

    require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))
    require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))

    var events []InvitationRewardEvent
    require.NoError(t, DB.Find(&events).Error)
    require.Len(t, events, 5)
    bySource := map[int]InvitationRewardEvent{}
    for _, event := range events {
        bySource[event.SourceId] = event
    }
    paidEvent := bySource[9339]
    assert.Equal(t, InvitationRewardEventSourceLegacySubscription, paidEvent.SourceType)
    assert.Equal(t, 9339, paidEvent.SourceSubscriptionId)
    assert.Equal(t, 9334, paidEvent.InviterId)
    assert.Equal(t, 9335, paidEvent.InviteeId)
    assert.Equal(t, InvitationRewardEventStatusActive, paidEvent.Status)
    assert.Equal(t, int64(8000), paidEvent.SourceAmountCents)
    assert.Equal(t, "CNY", paidEvent.SourceCurrency)
    ineligibleEvent := bySource[9350]
    assert.Equal(t, InvitationRewardEventSourceLegacySubscription, ineligibleEvent.SourceType)
    assert.Equal(t, 9350, ineligibleEvent.SourceSubscriptionId)
    assert.Equal(t, 9334, ineligibleEvent.InviterId)
    assert.Equal(t, 9349, ineligibleEvent.InviteeId)
    assert.Equal(t, int64(8000), ineligibleEvent.SourceAmountCents)
    assert.Equal(t, "CNY", ineligibleEvent.SourceCurrency)
    ambiguousEvent := bySource[9354]
    assert.Equal(t, InvitationRewardEventSourceLegacySubscription, ambiguousEvent.SourceType)
    assert.Equal(t, int64(0), ambiguousEvent.SourceAmountCents)
    assert.Equal(t, "", ambiguousEvent.SourceCurrency)
    noSnapshotEvent := bySource[9357]
    assert.Equal(t, InvitationRewardEventSourceLegacySubscription, noSnapshotEvent.SourceType)
    assert.Equal(t, int64(0), noSnapshotEvent.SourceAmountCents)
    assert.Equal(t, "", noSnapshotEvent.SourceCurrency)
    existingOrderEvent := bySource[9362]
    assert.Equal(t, InvitationRewardEventSourceSubscriptionOrder, existingOrderEvent.SourceType)
    assert.Equal(t, 9361, existingOrderEvent.SourceSubscriptionId)
    var duplicateLegacyEvents int64
    require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_type = ? AND source_subscription_id = ?", InvitationRewardEventSourceLegacySubscription, 9361).Count(&duplicateLegacyEvents).Error)
    assert.Equal(t, int64(0), duplicateLegacyEvents)
}
```

该回填只为返佣补偿和历史审计服务，不得改变奖励套餐计算；管理员赠送仍不回填为返佣来源，即使当前奖励套餐口径会把合格 admin active 订阅计入套餐资格；当同一 `source_subscription_id` 已有 `subscription_order` / `subscription_redemption` 等合法现存事件时，回填不得再为该订阅插入新的 `legacy_user_subscription` 事件。

在 `model/invitation_commission_test.go` 同步增加迁移顺序约束测试：

```go
func TestMigrateDBFastBackfillRunsAfterSubscriptionPlanMigration(t *testing.T) {
    oldDB := DB
    oldUsingSQLite := common.UsingSQLite
    oldUsingMySQL := common.UsingMySQL
    oldUsingPostgreSQL := common.UsingPostgreSQL
    db, err := gorm.Open(sqlite.Open("file:migrate_fast_backfill?mode=memory&cache=shared"), &gorm.Config{})
    require.NoError(t, err)
    DB = db
    common.UsingSQLite = true
    common.UsingMySQL = false
    common.UsingPostgreSQL = false
    t.Cleanup(func() {
        DB = oldDB
        common.UsingSQLite = oldUsingSQLite
        common.UsingMySQL = oldUsingMySQL
        common.UsingPostgreSQL = oldUsingPostgreSQL
    })
    require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}))
    now := common.GetTimestamp()
    plan := seedInvitationCommissionPlan(t, 9344, "legacy_fast_paid", 80, "CNY")
    require.NoError(t, DB.Create(&User{Id: 9345, Username: "legacy-fast-inviter", Status: common.UserStatusEnabled}).Error)
    require.NoError(t, DB.Create(&User{Id: 9346, Username: "legacy-fast-child", Status: common.UserStatusEnabled, InviterId: 9345}).Error)
    require.NoError(t, DB.Create(&UserSubscription{Id: 9347, UserId: 9346, PlanId: plan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
    require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9348, UserId: 9346, PlanId: plan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-fast-order", PaymentProvider: PaymentProviderEpay}).Error)

    require.NoError(t, migrateDBFast())

    var event InvitationRewardEvent
    require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceLegacySubscription, 9347).First(&event).Error)
    assert.Equal(t, 9345, event.InviterId)
    assert.Equal(t, int64(8000), event.SourceAmountCents)
}
```

该测试需要按 `model/main.go` 现有 SQLite 迁移 helper 写实实现，不允许只做源码字符串断言。

`model/main.go` 的 `migrateDB` 和 `migrateDBFast` 必须在所有依赖表迁移完成后调用回填函数：`users`、`user_subscriptions`、`subscription_orders`、`subscription_plans`、`invitation_reward_events` 都必须已经可查询；SQLite 下必须在 `ensureSubscriptionPlanTableSQLite()` 之后。`migrateDBFast` 禁止在并行 AutoMigrate goroutine 内调用回填，必须等 `wg.Wait()`、错误检查、`SubscriptionPlan` 特殊迁移和 `migrateLegacyTrialPlanTitle()` 成功后，再开启单独事务执行。失败必须记录 `SysError` 并中止迁移或启动初始化，不能静默继续。
该回填函数签名固定为 `func BackfillLegacyInvitationRewardEventsTx(tx *gorm.DB, now int64) error`，使用 `(source_type, source_id)` 唯一键保证重复执行不重复插入。

- [ ] **步骤 5：运行测试验证失败**

```bash
go test ./service -run 'Test(GetInvitationEntitlementStatusSkipsCommissionInviterWithoutUpsert|InvitationEntitlementKeepsExistingActiveSubscriptionCriteria|RunMonthlyInvitationEntitlementSweepSkipsCommissionInviters)' -count=1
go test ./model -run 'Test(BackfillLegacyInvitationRewardEventsPreservesExistingCommissionSources|MigrateDBFastBackfillRunsAfterSubscriptionPlanMigration)' -count=1
```

预期：FAIL，当前实现没有 `commission` mode guard，也没有历史返佣来源回填函数。

- [ ] **步骤 6：实现 mode-aware 保护和历史返佣来源回填**

实现要点：

1. `EnsureMonthlyInvitationEntitlement` 开头读取 inviter，归一化 `InvitationRewardMode`。
2. 当前模式为 `commission` 时：仍使用现有 `countDirectInviteesTx` / `countQualifiedActiveInviteesTx` 计算展示统计，但返回 `Entitled=false`，不写 `InvitationMonthlyEntitlement`，不写 `monthly_invite_entitlement` 订阅。
3. 当前模式为 `subscription` 时：不得改变 `eligibleInvitationRewardSubscriptionScope`、`countQualifiedActiveInviteesTx`、`listInvitationRewardCandidatesTx` 的现有 active 订阅计算口径。
4. `RunMonthlyInvitationEntitlementSweep` 查询 inviter IDs 时排除当前 `commission` 邀请人；返回 processed 数量只统计实际尝试处理的 inviter。
5. `model.fillUserInvitationSummariesTx` 继续使用当前 active 订阅口径统计 `QualifiedPaidInviteCount`，只补充/透传 `InvitationRewardMode` 字段；不得改为事件表口径。
6. 历史返佣来源回填函数命名为 `model.BackfillLegacyInvitationRewardEventsTx`，迁移或启动初始化由 `model/main.go` 的 `migrateDB` 和 `migrateDBFast` 在新增事件表迁移完成后、所有依赖表可查询后调用；回填现有 active 销售型邀请订阅，`source_type = legacy_user_subscription`、`source_id = user_subscriptions.id`、`source_subscription_id = user_subscriptions.id`。排除试用、管理员赠送、`monthly_invite_entitlement` 奖励套餐和非直属邀请关系；`reward_eligible = false` 的历史销售型订阅仍可回填来源事件，但返佣 fresh 计算必须过滤；重复执行必须幂等；如果同一 `source_subscription_id` 已有合法 `subscription_order` / `subscription_redemption` 事件，回填不得再创建 `legacy_user_subscription` 重复来源。历史金额快照只能从明确来源字段，或唯一且可审计匹配的成功 `subscription_orders` / 订阅兑换码复制；同一用户同一套餐多订单、续费区间无法证明、无快照、无唯一候选时，事件仍可作为审计来源回填，但必须写 `SourceAmountCents = 0`、`SourceCurrency = ""`，由返佣阶段按 `invalid_source_amount` 跳过，不能用当前套餐价格补判。

- [ ] **步骤 7：运行测试验证通过**

运行步骤 5 命令，预期 PASS。再运行：

```bash
go test ./service -run 'InvitationEntitlement|MonthlyInvitation' -count=1
go test ./controller -run 'InvitationEntitlement|ModelList' -count=1
```

预期：旧邀请权益测试仍 PASS，且不会给 `commission` 用户创建套餐。

---

## 任务 4：返佣账户服务、划转、返现申请与管理员处理

**文件：**
- 创建/修改：`service/invitation_commission.go`
- 修改：`controller/subscription_payment_completion.go` —— 默认订单后处理 handler 切换为正式 service 分发器。
- 修改：`controller/subscription_payment_balance.go` —— 余额购买使用正式订单后处理 handler。
- 修改：`controller/user.go` —— 订阅兑换码兑换使用正式 redemption 后处理 handler。
- 修改：`main.go` —— 在现有后台任务启动区调用 `service.StartInvitationRewardEventRetryTask()`。
- 测试：`service/invitation_commission_test.go`
- 测试：`controller/invitation_entitlement_test.go`
- 测试：`controller/redemption_cny_test.go`
- 测试：`main_task_startup_test.go` 或等价可测试启动注册源码契约。
- 修改：`model/account_balance.go` —— 新增 `AccountBalanceIntFromCents(amountCents int64) (int, error)`，统一校验正数与 `math.MaxInt` 上限。

- [ ] **步骤 1：增加服务测试 helper**

在 `service/invitation_commission_test.go` 增加：

```go
func setupInvitationCommissionServiceDB(t *testing.T) {
    t.Helper()
    oldDB := model.DB
    oldLogDB := model.LOG_DB
    oldUsingSQLite := common.UsingSQLite
    oldUsingMySQL := common.UsingMySQL
    oldUsingPostgreSQL := common.UsingPostgreSQL
    common.UsingSQLite = true
    common.UsingMySQL = false
    common.UsingPostgreSQL = false
    safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
    db, err := gorm.Open(sqlite.Open("file:"+safeName+"_invitation_commission?mode=memory&cache=shared"), &gorm.Config{})
    require.NoError(t, err)
    sqlDB, err := db.DB()
    require.NoError(t, err)
    sqlDB.SetMaxOpenConns(1)
    model.DB = db
    model.LOG_DB = db
    require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.UserSubscription{}, &model.Redemption{}, &model.InvitationMonthlyEntitlement{}, &model.InvitationRewardEvent{}, &model.InvitationCommissionAccount{}, &model.InvitationCommissionRecord{}, &model.InvitationCommissionLedger{}, &model.InvitationCommissionWithdrawal{}, &model.TopUp{}))
    t.Cleanup(func() {
        model.DB = oldDB
        model.LOG_DB = oldLogDB
        common.UsingSQLite = oldUsingSQLite
        common.UsingMySQL = oldUsingMySQL
        common.UsingPostgreSQL = oldUsingPostgreSQL
    })
}

func seedCommissionRewardEvent(t *testing.T, inviterId int, inviteeId int, sourceId int, amountCents int64, currency string) model.InvitationRewardEvent {
    t.Helper()
    require.NoError(t, model.DB.Create(&model.User{Id: inviterId, Username: fmt.Sprintf("inviter-%d", inviterId), Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
    require.NoError(t, model.DB.Create(&model.User{Id: inviteeId, Username: fmt.Sprintf("invitee-%d", inviteeId), Status: common.UserStatusEnabled, InviterId: inviterId}).Error)
    now := common.GetTimestamp()
    planId := sourceId + 100000
    subscriptionId := sourceId + 200000
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planId, Title: fmt.Sprintf("commission-plan-%d", sourceId), PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subscriptionId, UserId: inviteeId, PlanId: planId, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
    event := model.InvitationRewardEvent{InviterId: inviterId, InviteeId: inviteeId, SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: sourceId, SourceOrderId: sourceId, SourceSubscriptionId: subscriptionId, SourceAmountCents: amountCents, SourceCurrency: currency, EventStartTime: now, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, CreatedAt: now}
    require.NoError(t, model.DB.Create(&event).Error)
    return event
}

func seedCommissionAccount(t *testing.T, userId int, available int64, pending int64, withdrawn int64, transferred int64) {
    t.Helper()
    account := model.InvitationCommissionAccount{UserId: userId, AvailableCents: available, PendingCents: pending, WithdrawnCents: withdrawn, TransferredCents: transferred, CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}
    require.NoError(t, model.DB.Create(&account).Error)
}

func setInvitationCommissionSettingForTest(t *testing.T, value operation_setting.InvitationCommissionSetting) {
    t.Helper()
    setting := operation_setting.GetInvitationCommissionSetting()
    old := *setting
    *setting = value
    t.Cleanup(func() { *setting = old })
}
```

该 helper 需要 imports：`fmt`、`math`、`strings`、`sync`、`github.com/glebarez/sqlite`、`gorm.io/gorm`、`github.com/QuantumNous/new-api/setting/operation_setting`。

- [ ] **步骤 2：编写失败测试：返佣入账、禁用、来源资格与非法金额**

在 `service/invitation_commission_test.go` 增加：

```go
func TestCreateInvitationCommissionForRewardEventCreditsAvailableOnce(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    event := seedCommissionRewardEvent(t, 9401, 9402, 9403, 10000, "CNY")

    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

    var account model.InvitationCommissionAccount
    require.NoError(t, model.DB.Where("user_id = ?", 9401).First(&account).Error)
    assert.Equal(t, int64(1000), account.AvailableCents)
    var records int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
    assert.Equal(t, int64(1), records)
    var ledgers int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND type = ?", 9401, model.InvitationCommissionLedgerEarned).Count(&ledgers).Error)
    assert.Equal(t, int64(1), ledgers)
}

func TestHandleInvitationRewardForSubscriptionRedemptionCreditsCommission(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    require.NoError(t, model.DB.Create(&model.User{Id: 9407, Username: "redemption-inviter", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
    require.NoError(t, model.DB.Create(&model.User{Id: 9408, Username: "redemption-child", Status: common.UserStatusEnabled, InviterId: 9407}).Error)
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9409, Title: "Redemption Commission", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
    now := common.GetTimestamp()
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9410, UserId: 9408, PlanId: 9409, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: "redemption", Source: "redemption"}).Error)
    redemption := model.Redemption{Id: 9411, Key: "commission-redemption-key", Status: common.RedemptionCodeStatusUsed, Type: model.RedemptionTypeSubscription, PlanId: 9409, AmountCents: 10000, Currency: "CNY", UsedUserId: 9408, CreatedTime: now, RedeemedTime: now}
    require.NoError(t, model.DB.Create(&redemption).Error)
    event := model.InvitationRewardEvent{InviterId: 9407, InviteeId: 9408, SourceType: model.InvitationRewardEventSourceSubscriptionRedemption, SourceId: redemption.Id, SourceRedemptionId: redemption.Id, SourceSubscriptionId: 9410, SourceAmountCents: 10000, SourceCurrency: "CNY", EventStartTime: now - 3600, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, CreatedAt: now}
    require.NoError(t, model.DB.Create(&event).Error)

    require.NoError(t, HandleInvitationRewardForSubscriptionRedemption(redemption.Id))

    var record model.InvitationCommissionRecord
    require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
    assert.Equal(t, model.InvitationRewardEventSourceSubscriptionRedemption, record.SourceType)
    assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
    assert.Equal(t, int64(1000), record.CommissionCents)
}

func TestCreateInvitationCommissionForRewardEventUsesCurrentInviterModeFreshly(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    event := seedCommissionRewardEvent(t, 9404, 9405, 9406, 10000, "CNY")
    require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9404).Update("invitation_reward_mode", model.InvitationRewardModeSubscription).Error)

    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

    var records int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
    assert.Equal(t, int64(0), records)
    require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9404).Update("invitation_reward_mode", model.InvitationRewardModeCommission).Error)
    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ? AND status = ?", event.Id, model.InvitationCommissionStatusAvailable).Count(&records).Error)
    assert.Equal(t, int64(1), records)
}
func seedSecondQualifiedInviteeForEntitlement(t *testing.T, inviterId int, inviteeId int, planId int) {
    t.Helper()
    now := common.GetTimestamp()
    require.NoError(t, model.DB.Create(&model.User{Id: inviteeId, Username: fmt.Sprintf("second-invitee-%d", inviteeId), Status: common.UserStatusEnabled, InviterId: inviterId}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: inviteeId + 200000, UserId: inviteeId, PlanId: planId, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
}

// 以下两个 handler 测试必须构造足够的直属合格 active invitee，使现有奖励套餐口径在 `subscription` 模式下会产生 `InvitationMonthlyEntitlement`；不能只断言 `>= 0` 这种恒真条件。
func TestInvitationRewardHandlersPreserveSubscriptionRewardPackagePath(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    event := seedCommissionRewardEvent(t, 9455, 9456, 9457, 10000, "CNY")
    require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9455).Update("invitation_reward_mode", model.InvitationRewardModeSubscription).Error)
    var sourceSub model.UserSubscription
    require.NoError(t, model.DB.First(&sourceSub, event.SourceSubscriptionId).Error)
    seedSecondQualifiedInviteeForEntitlement(t, 9455, 9466, sourceSub.PlanId)

    require.NoError(t, HandleInvitationRewardForCompletedSubscriptionOrder(event.SourceOrderId))

    var commissionRecords int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&commissionRecords).Error)
    assert.Equal(t, int64(0), commissionRecords)
    var entitlementCount int64
    require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ? AND status = ?", 9455, model.InvitationEntitlementStatusQualified).Count(&entitlementCount).Error)
    assert.Equal(t, int64(1), entitlementCount)
}

func TestInvitationRewardHandlersSkipSubscriptionPackageForCommissionMode(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    event := seedCommissionRewardEvent(t, 9458, 9459, 9460, 10000, "CNY")
    var sourceSub model.UserSubscription
    require.NoError(t, model.DB.First(&sourceSub, event.SourceSubscriptionId).Error)
    seedSecondQualifiedInviteeForEntitlement(t, 9458, 9467, sourceSub.PlanId)

    require.NoError(t, HandleInvitationRewardForCompletedSubscriptionOrder(event.SourceOrderId))

    var record model.InvitationCommissionRecord
    require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
    assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
    var entitlementCount int64
    require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 9458).Count(&entitlementCount).Error)
    assert.Equal(t, int64(0), entitlementCount)
}

func TestCreateInvitationCommissionForRewardEventDoesNotConsumeWhenCommissionDisabled(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: false, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    event := seedCommissionRewardEvent(t, 9441, 9442, 9443, 10000, "CNY")

    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

    var records int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
    assert.Equal(t, int64(0), records)
    var accounts int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionAccount{}).Where("user_id = ?", 9441).Count(&accounts).Error)
    assert.Equal(t, int64(0), accounts)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ? AND status = ?", event.Id, model.InvitationCommissionStatusAvailable).Count(&records).Error)
    assert.Equal(t, int64(1), records)
}

func TestCreateInvitationCommissionForRewardEventDoesNotConsumeRewardIneligibleSource(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    event := seedCommissionRewardEvent(t, 9444, 9445, 9446, 10000, "CNY")
    var sub model.UserSubscription
    require.NoError(t, model.DB.First(&sub, event.SourceSubscriptionId).Error)
    require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", sub.PlanId).Update("reward_eligible", false).Error)

    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

    var records int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
    assert.Equal(t, int64(0), records)
    var accounts int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionAccount{}).Where("user_id = ?", 9444).Count(&accounts).Error)
    assert.Equal(t, int64(0), accounts)
    require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", sub.PlanId).Update("reward_eligible", true).Error)
    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ? AND status = ?", event.Id, model.InvitationCommissionStatusAvailable).Count(&records).Error)
    assert.Equal(t, int64(1), records)
}

func TestCreateInvitationCommissionForRewardEventSkipsInvalidSourceAmountAndRate(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    cases := []struct {
        name string
        eventAmount int64
        currency string
        rateBps int
        wantReason string
    }{
        {name: "non cny", eventAmount: 10000, currency: "USD", rateBps: 1000, wantReason: "unsupported_currency"},
        {name: "zero amount", eventAmount: 0, currency: "CNY", rateBps: 1000, wantReason: "invalid_source_amount"},
        {name: "overflow", eventAmount: math.MaxInt64, currency: "CNY", rateBps: 10000, wantReason: model.InvitationCommissionReasonCommissionOverflow},
    }
    for idx, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: tc.rateBps, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
            event := seedCommissionRewardEvent(t, 9411+idx, 9421+idx, 9431+idx, tc.eventAmount, tc.currency)
            require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
            var record model.InvitationCommissionRecord
            require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
            assert.Equal(t, model.InvitationCommissionStatusSkipped, record.Status)
            assert.Equal(t, tc.wantReason, record.Reason)
        })
    }
}

func TestCreateInvitationCommissionForRewardEventDoesNotConsumeWhenRateDisabled(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 0, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    event := seedCommissionRewardEvent(t, 9447, 9448, 9449, 10000, "CNY")

    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

    var records int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
    assert.Equal(t, int64(0), records)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ? AND status = ?", event.Id, model.InvitationCommissionStatusAvailable).Count(&records).Error)
    assert.Equal(t, int64(1), records)
}
```

- [ ] **步骤 3：编写失败测试：划转即时完成且不创建返现申请**

```go
func TestTransferInvitationCommissionToBalanceCompletesImmediately(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    seedCommissionAccount(t, 9451, 5000, 0, 0, 0)
    require.NoError(t, model.DB.Create(&model.User{Id: 9451, Username: "transfer-user", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission, Quota: 100}).Error)
    var topUpsBefore int64
    require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9451).Count(&topUpsBefore).Error)

    result, err := TransferInvitationCommissionToBalance(9451, 1200)

    require.NoError(t, err)
    assert.Equal(t, int64(3800), result.AvailableCents)
    assert.Equal(t, int64(1200), result.TransferredCents)
    assert.Equal(t, 1300, result.UserQuota)
    var user model.User
    require.NoError(t, model.DB.First(&user, 9451).Error)
    assert.Equal(t, 1300, user.Quota)
    var topUpsAfter int64
    require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9451).Count(&topUpsAfter).Error)
    assert.Equal(t, topUpsBefore, topUpsAfter)
    var withdrawals int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionWithdrawal{}).Where("user_id = ?", 9451).Count(&withdrawals).Error)
    assert.Equal(t, int64(0), withdrawals)
}
```

- [ ] **步骤 4：编写失败测试：申请、完成、拒绝和历史账户返现**

```go
func TestInvitationCommissionWithdrawalLifecycle(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    seedCommissionAccount(t, 9461, 6000, 0, 0, 0)
    require.NoError(t, model.DB.Create(&model.User{Id: 9461, Username: "withdraw-user", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission, Quota: 777}).Error)

    withdrawal, err := RequestInvitationCommissionWithdrawal(9461, InvitationCommissionWithdrawalRequest{
        AmountCents: 5000,
        Contact: InvitationCommissionContact{Type: "wechat", Value: "user-contact"},
        Remark: "请联系我",
    })
    require.NoError(t, err)
    assert.Equal(t, model.InvitationCommissionWithdrawalPending, withdrawal.Status)

    var account model.InvitationCommissionAccount
    require.NoError(t, model.DB.Where("user_id = ?", 9461).First(&account).Error)
    assert.Equal(t, int64(1000), account.AvailableCents)
    assert.Equal(t, int64(5000), account.PendingCents)

    var quotaBefore int
    require.NoError(t, model.DB.Model(&model.User{}).Select("quota").Where("id = ?", 9461).Scan(&quotaBefore).Error)
    var topUpsBefore int64
    require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9461).Count(&topUpsBefore).Error)

    require.NoError(t, CompleteInvitationCommissionWithdrawal(withdrawal.Id, 1001, "已线下转账"))
    require.Error(t, CompleteInvitationCommissionWithdrawal(withdrawal.Id, 1001, "重复完成"))
    require.NoError(t, model.DB.Where("user_id = ?", 9461).First(&account).Error)
    assert.Equal(t, int64(0), account.PendingCents)
    assert.Equal(t, int64(5000), account.WithdrawnCents)

    var completed model.InvitationCommissionWithdrawal
    require.NoError(t, model.DB.First(&completed, withdrawal.Id).Error)
    assert.Equal(t, 1001, completed.ReviewerId)
    assert.Equal(t, 1001, completed.CompletedBy)
    assert.NotZero(t, completed.ReviewedAt)
    assert.NotZero(t, completed.CompletedAt)
    assert.Equal(t, "已线下转账", completed.AdminRemark)
    var quotaAfter int
    require.NoError(t, model.DB.Model(&model.User{}).Select("quota").Where("id = ?", 9461).Scan(&quotaAfter).Error)
    assert.Equal(t, quotaBefore, quotaAfter)
    var topUpsAfter int64
    require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9461).Count(&topUpsAfter).Error)
    assert.Equal(t, topUpsBefore, topUpsAfter)
    var completedLedgers int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND type = ? AND reference_id = ?", 9461, model.InvitationCommissionLedgerWithdrawalCompleted, withdrawal.Id).Count(&completedLedgers).Error)
    assert.Equal(t, int64(1), completedLedgers)
}

func TestRejectInvitationCommissionWithdrawalReturnsPendingToAvailable(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    seedCommissionAccount(t, 9471, 1000, 5000, 0, 0)
    require.NoError(t, model.DB.Create(&model.User{Id: 9471, Username: "reject-user", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission, Quota: 888}).Error)
    contactSnapshot, err := common.Marshal(map[string]string{"type": "telegram", "value": "reject-user"})
    require.NoError(t, err)
    now := common.GetTimestamp()
    withdrawal := model.InvitationCommissionWithdrawal{UserId: 9471, AmountCents: 5000, Status: model.InvitationCommissionWithdrawalPending, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), CreatedAt: now, UpdatedAt: now}
    require.NoError(t, model.DB.Create(&withdrawal).Error)
    var quotaBefore int
    require.NoError(t, model.DB.Model(&model.User{}).Select("quota").Where("id = ?", 9471).Scan(&quotaBefore).Error)
    var topUpsBefore int64
    require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9471).Count(&topUpsBefore).Error)

    require.NoError(t, RejectInvitationCommissionWithdrawal(withdrawal.Id, 1002, "资料不完整"))

    var quotaAfter int
    require.NoError(t, model.DB.Model(&model.User{}).Select("quota").Where("id = ?", 9471).Scan(&quotaAfter).Error)
    assert.Equal(t, quotaBefore, quotaAfter)
    var topUpsAfter int64
    require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9471).Count(&topUpsAfter).Error)
    assert.Equal(t, topUpsBefore, topUpsAfter)
    var account model.InvitationCommissionAccount
    require.NoError(t, model.DB.Where("user_id = ?", 9471).First(&account).Error)
    assert.Equal(t, int64(6000), account.AvailableCents)
    assert.Equal(t, int64(0), account.PendingCents)
    var rejected model.InvitationCommissionWithdrawal
    require.NoError(t, model.DB.First(&rejected, withdrawal.Id).Error)
    assert.Equal(t, model.InvitationCommissionWithdrawalRejected, rejected.Status)
    assert.Equal(t, 1002, rejected.ReviewerId)
    assert.NotZero(t, rejected.ReviewedAt)
    assert.Equal(t, "资料不完整", rejected.AdminRemark)
    var ledger model.InvitationCommissionLedger
    require.NoError(t, model.DB.Where("user_id = ? AND type = ? AND reference_id = ?", 9471, model.InvitationCommissionLedgerWithdrawalRejected, withdrawal.Id).First(&ledger).Error)
    assert.Equal(t, int64(6000), ledger.AvailableAfterCents)
    assert.Equal(t, int64(0), ledger.PendingAfterCents)
}

func TestCommissionDisabledStillAllowsHistoricalBalanceOperations(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: false, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    seedCommissionAccount(t, 9472, 5000, 0, 0, 0)
    require.NoError(t, model.DB.Create(&model.User{Id: 9472, Username: "disabled-history", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeSubscription, Quota: 0}).Error)

    disabledSummary, err := GetInvitationCommissionSummary(9472)
    require.NoError(t, err)
    assert.True(t, disabledSummary.CanTransfer)
    assert.True(t, disabledSummary.CanRequestWithdrawal)

    transfer, err := TransferInvitationCommissionToBalance(9472, 1000)
    require.NoError(t, err)
    assert.Equal(t, int64(4000), transfer.AvailableCents)
    assert.Equal(t, int64(1000), transfer.TransferredCents)

    withdrawal, err := RequestInvitationCommissionWithdrawal(9472, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "email", Value: "disabled@example.com"}, Remark: "disabled but historical"})
    require.NoError(t, err)
    assert.Equal(t, model.InvitationCommissionWithdrawalPending, withdrawal.Status)
    require.NoError(t, CompleteInvitationCommissionWithdrawal(withdrawal.Id, 1001, "已线下返现"))

    rejected, err := RequestInvitationCommissionWithdrawal(9472, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "wechat", Value: "disabled-reject"}})
    require.NoError(t, err)
    require.NoError(t, RejectInvitationCommissionWithdrawal(rejected.Id, 1002, "资料不完整"))

    var account model.InvitationCommissionAccount
    require.NoError(t, model.DB.Where("user_id = ?", 9472).First(&account).Error)
    assert.Equal(t, int64(3000), account.AvailableCents)
    assert.Equal(t, int64(0), account.PendingCents)
    assert.Equal(t, int64(1000), account.WithdrawnCents)
    assert.Equal(t, int64(1000), account.TransferredCents)


    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 0, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    rateZeroSummary, err := GetInvitationCommissionSummary(9472)
    require.NoError(t, err)
    assert.True(t, rateZeroSummary.CanTransfer)
    assert.True(t, rateZeroSummary.CanRequestWithdrawal)
    _, err = TransferInvitationCommissionToBalance(9472, 1000)
    require.NoError(t, err)
    zeroRateWithdrawal, err := RequestInvitationCommissionWithdrawal(9472, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "email", Value: "zero-rate@example.com"}, Remark: "rate zero but historical"})
    require.NoError(t, err)
    require.NoError(t, CompleteInvitationCommissionWithdrawal(zeroRateWithdrawal.Id, 1004, "费率关闭但历史余额可正常完成"))
    zeroRateRejected, err := RequestInvitationCommissionWithdrawal(9472, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "email", Value: "zero-rate-reject@example.com"}, Remark: "rate zero but historical reject"})
    require.NoError(t, err)
    require.NoError(t, RejectInvitationCommissionWithdrawal(zeroRateRejected.Id, 1003, "费率关闭但历史余额可退回"))
    require.NoError(t, model.DB.Where("user_id = ?", 9472).First(&account).Error)
    assert.Equal(t, int64(2000), account.AvailableCents)
    assert.Equal(t, int64(0), account.PendingCents)
    assert.Equal(t, int64(2000), account.WithdrawnCents)
    assert.Equal(t, int64(2000), account.TransferredCents)
}

func TestSubscriptionModeUserWithHistoricalCommissionAccountCanRequestWithdrawal(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    seedCommissionAccount(t, 9452, 5000, 0, 0, 0)
    require.NoError(t, model.DB.Create(&model.User{Id: 9452, Username: "history-withdraw-user", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeSubscription}).Error)

    withdrawal, err := RequestInvitationCommissionWithdrawal(9452, InvitationCommissionWithdrawalRequest{AmountCents: 2000, Contact: InvitationCommissionContact{Type: "wechat", Value: "history-contact"}, Remark: "历史返佣返现"})

    require.NoError(t, err)
    assert.Equal(t, model.InvitationCommissionWithdrawalPending, withdrawal.Status)
    var account model.InvitationCommissionAccount
    require.NoError(t, model.DB.Where("user_id = ?", 9452).First(&account).Error)
    assert.Equal(t, int64(3000), account.AvailableCents)
    assert.Equal(t, int64(2000), account.PendingCents)
}
```

- [ ] **步骤 5：编写失败测试：并发幂等、余额不为负与事件重试补偿**

```go
func TestInvitationCommissionConcurrentOperationsRemainIdempotentAndNonNegative(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    event := seedCommissionRewardEvent(t, 9481, 9482, 9483, 10000, "CNY")

    var wg sync.WaitGroup
    errs := make(chan error, 8)
    for i := 0; i < 8; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            errs <- CreateInvitationCommissionForRewardEvent(event.Id)
        }()
    }
    wg.Wait()
    close(errs)
    for err := range errs {
        require.NoError(t, err)
    }
    var account model.InvitationCommissionAccount
    require.NoError(t, model.DB.Where("user_id = ?", 9481).First(&account).Error)
    assert.Equal(t, int64(1000), account.AvailableCents)
    var records int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
    assert.Equal(t, int64(1), records)

    seedCommissionAccount(t, 9484, 1500, 0, 0, 0)
    require.NoError(t, model.DB.Create(&model.User{Id: 9484, Username: "race-user", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
    raceErrs := make(chan error, 2)
    var raceWg sync.WaitGroup
    raceWg.Add(2)
    go func() {
        defer raceWg.Done()
        _, err := TransferInvitationCommissionToBalance(9484, 1000)
        raceErrs <- err
    }()
    go func() {
        defer raceWg.Done()
        _, err := RequestInvitationCommissionWithdrawal(9484, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "email", Value: "race@example.com"}})
        raceErrs <- err
    }()
    raceWg.Wait()
    close(raceErrs)
    successes := 0
    for err := range raceErrs {
        if err == nil {
            successes++
        }
    }
    assert.Equal(t, 1, successes)
    require.NoError(t, model.DB.Where("user_id = ?", 9484).First(&account).Error)
    assert.GreaterOrEqual(t, account.AvailableCents, int64(0))
    assert.Equal(t, int64(1500), account.AvailableCents+account.PendingCents+account.TransferredCents)
}

func TestRetryPendingInvitationRewardEventsProcessesExistingEvents(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    commissionEvent := seedCommissionRewardEvent(t, 9485, 9486, 9487, 10000, "CNY")
    ineligibleEvent := seedCommissionRewardEvent(t, 9488, 9489, 9490, 10000, "CNY")
    var ineligibleSub model.UserSubscription
    require.NoError(t, model.DB.First(&ineligibleSub, ineligibleEvent.SourceSubscriptionId).Error)
    require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", ineligibleSub.PlanId).Update("reward_eligible", false).Error)
    now := common.GetTimestamp()
    code := "retry_subscription_plan"
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9491, Title: "Retry Subscription", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
    require.NoError(t, model.DB.Create(&model.User{Id: 9492, Username: "retry-subscription-inviter", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeSubscription}).Error)
    require.NoError(t, model.DB.Create(&model.User{Id: 9493, Username: "retry-subscription-child", Status: common.UserStatusEnabled, InviterId: 9492}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9494, UserId: 9493, PlanId: 9491, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
    subscriptionEvent := model.InvitationRewardEvent{SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: 9495, InviterId: 9492, InviteeId: 9493, SourceSubscriptionId: 9494, EventStartTime: now - 3600, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, SourceAmountCents: 10000, SourceCurrency: "CNY", CreatedAt: now}
    require.NoError(t, model.DB.Create(&subscriptionEvent).Error)

    processed, err := RetryPendingInvitationRewardEvents(10)

    require.NoError(t, err)
    assert.Equal(t, 1, processed)
    var record model.InvitationCommissionRecord
    require.NoError(t, model.DB.Where("event_id = ?", commissionEvent.Id).First(&record).Error)
    assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
    var account model.InvitationCommissionAccount
    require.NoError(t, model.DB.Where("user_id = ?", 9485).First(&account).Error)
    assert.Equal(t, int64(1000), account.AvailableCents)
    var ineligibleRecords int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", ineligibleEvent.Id).Count(&ineligibleRecords).Error)
    assert.Equal(t, int64(0), ineligibleRecords)
    var subscriptionRecords int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", subscriptionEvent.Id).Count(&subscriptionRecords).Error)
    assert.Equal(t, int64(0), subscriptionRecords)
    var entitlementCount int64
    require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 9492).Count(&entitlementCount).Error)
    assert.Equal(t, int64(0), entitlementCount)

    processedAgain, err := RetryPendingInvitationRewardEvents(10)
    require.NoError(t, err)
    assert.Equal(t, 0, processedAgain)
}

func TestRetryPendingInvitationRewardEventsCreditsBackfilledLegacySubscriptionAfterModeSwitch(t *testing.T) {
    setupInvitationCommissionServiceDB(t)
    setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
    now := common.GetTimestamp()
    require.NoError(t, model.DB.Create(&model.User{Id: 9701, Username: "legacy-switch-inviter", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeSubscription}).Error)
    for _, userID := range []int{9702, 9703, 9704} {
        require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: fmt.Sprintf("legacy-switch-child-%d", userID), Status: common.UserStatusEnabled, InviterId: 9701}).Error)
    }
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9705, Title: "Legacy Switch Paid", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
    require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9706, Title: "Legacy Switch Ineligible", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: false, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9707, UserId: 9702, PlanId: 9705, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
    require.NoError(t, model.DB.Create(&model.SubscriptionOrder{Id: 9708, UserId: 9702, PlanId: 9705, Status: common.TopUpStatusSuccess, Money: 100, AmountCents: 10000, Currency: "CNY", TradeNo: "legacy-switch-paid", PaymentProvider: model.PaymentProviderEpay, CreateTime: now - 3500}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9709, UserId: 9703, PlanId: 9706, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
    require.NoError(t, model.DB.Create(&model.SubscriptionOrder{Id: 9710, UserId: 9703, PlanId: 9706, Status: common.TopUpStatusSuccess, Money: 100, AmountCents: 10000, Currency: "CNY", TradeNo: "legacy-switch-ineligible", PaymentProvider: model.PaymentProviderEpay, CreateTime: now - 3500}).Error)
    require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9711, UserId: 9704, PlanId: 9705, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)

    require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error { return model.BackfillLegacyInvitationRewardEventsTx(tx, now) }))
    require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9701).Update("invitation_reward_mode", model.InvitationRewardModeCommission).Error)

    processed, err := RetryPendingInvitationRewardEvents(10)

    require.NoError(t, err)
    assert.Equal(t, 1, processed)
    var record model.InvitationCommissionRecord
    require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceLegacySubscription, 9707).First(&record).Error)
    assert.Equal(t, model.InvitationRewardEventSourceLegacySubscription, record.SourceType)
    assert.Equal(t, int64(10000), record.SourceAmountCents)
    assert.Equal(t, int64(1000), record.CommissionCents)
    assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
    var ledger model.InvitationCommissionLedger
    require.NoError(t, model.DB.Where("user_id = ? AND type = ? AND reference_id = ?", 9701, model.InvitationCommissionLedgerEarned, record.Id).First(&ledger).Error)
    assert.Equal(t, int64(1000), ledger.AvailableAfterCents)
    var account model.InvitationCommissionAccount
    require.NoError(t, model.DB.Where("user_id = ?", 9701).First(&account).Error)
    assert.Equal(t, int64(1000), account.AvailableCents)
    var ineligibleRecords int64
    require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceLegacySubscription, 9709).Count(&ineligibleRecords).Error)
    assert.Equal(t, int64(0), ineligibleRecords)
    var invalidRecord model.InvitationCommissionRecord
    require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceLegacySubscription, 9711).First(&invalidRecord).Error)
    assert.Equal(t, model.InvitationCommissionStatusSkipped, invalidRecord.Status)
    assert.Equal(t, model.InvitationCommissionReasonInvalidSourceAmount, invalidRecord.Reason)
    assert.Equal(t, int64(0), invalidRecord.SourceAmountCents)
    assert.Equal(t, "", invalidRecord.SourceCurrency)
}
```

- [ ] **步骤 6：运行测试验证失败**

```bash
go test ./service -run 'Test(CreateInvitationCommissionForRewardEvent|HandleInvitationRewardForSubscriptionRedemptionCreditsCommission|InvitationRewardHandlersPreserveSubscriptionRewardPackagePath|InvitationRewardHandlersSkipSubscriptionPackageForCommissionMode|TransferInvitationCommissionToBalance|InvitationCommissionWithdrawal|RejectInvitationCommissionWithdrawalReturnsPendingToAvailable|SubscriptionModeUserWithHistoricalCommissionAccountCanRequestWithdrawal|CommissionDisabledStillAllowsHistoricalBalanceOperations|InvitationCommissionConcurrentOperationsRemainIdempotentAndNonNegative|RetryPendingInvitationRewardEventsProcessesExistingEvents|RetryPendingInvitationRewardEventsCreditsBackfilledLegacySubscriptionAfterModeSwitch)' -count=1
go test ./controller -run 'Test(DefaultInvitationRewardHandlersUseFormalService|CompleteSubscriptionOrderRetriesInvitationRewardHandlerForSuccessfulOrder|RedeemSubscriptionRedemptionInvokesInvitationRewardHandlerAfterCommit)' -count=1
go test . -run 'TestMainStartsInvitationRewardEventRetryTask' -count=1
```

预期：FAIL，服务函数尚不存在。

控制器级契约测试必须固定默认 handler 切换：`controller/invitation_entitlement_test.go` 和 `controller/redemption_cny_test.go` 需断言默认 `handleInvitationReward...` 已切到正式 service 分发器；`main_task_startup_test.go` 需断言生产启动路径包含 `service.StartInvitationRewardEventRetryTask()`。

- [ ] **步骤 7：实现返佣服务**

实现函数签名：

```go
type InvitationCommissionContact struct {
    Type  string `json:"type"`
    Value string `json:"value"`
}

type InvitationCommissionWithdrawalRequest struct {
    AmountCents int64 `json:"amount_cents"`
    Contact InvitationCommissionContact `json:"contact"`
    Remark string `json:"remark"`
}

type InvitationCommissionTransferResult struct {
    AvailableCents int64 `json:"available_cents"`
    TransferredCents int64 `json:"transferred_cents"`
    UserQuota int `json:"user_quota"`
}

type InvitationCommissionPageResult[T any] struct {
    Items []T `json:"items"`
    Total int64 `json:"total"`
    Page int `json:"page"`
    PageSize int `json:"page_size"`
}

type InvitationCommissionWithdrawalFilter struct {
    Status string
    UserId int
}

func HandleInvitationRewardForCompletedSubscriptionOrder(orderId int) error
func HandleInvitationRewardForSubscriptionRedemption(redemptionId int) error
func CreateInvitationCommissionForRewardEvent(eventId int) error
func TransferInvitationCommissionToBalance(userId int, amountCents int64) (*InvitationCommissionTransferResult, error)
func RequestInvitationCommissionWithdrawal(userId int, req InvitationCommissionWithdrawalRequest) (*model.InvitationCommissionWithdrawal, error)
func CompleteInvitationCommissionWithdrawal(withdrawalId int, reviewerId int, adminRemark string) error
func RejectInvitationCommissionWithdrawal(withdrawalId int, reviewerId int, adminRemark string) error
func RetryPendingInvitationRewardEvents(limit int) (int, error)
func StartInvitationRewardEventRetryTask()
func CountPendingInvitationCommissionWithdrawals() (int64, error)
func GetInvitationCommissionSummary(userId int) (*InvitationCommissionSummary, error)
func ListInvitationCommissionRecords(userId int, page int, pageSize int) (*InvitationCommissionPageResult[InvitationCommissionRecordResponse], error)
func ListInvitationCommissionWithdrawals(userId int, page int, pageSize int) (*InvitationCommissionPageResult[InvitationCommissionWithdrawalResponse], error)
func AdminListInvitationCommissionWithdrawals(filter InvitationCommissionWithdrawalFilter) (*InvitationCommissionPageResult[AdminInvitationCommissionWithdrawalResponse], error)

```

查询契约：

- Summary、records、withdrawals 和 admin withdrawals 的查询逻辑必须在 service/model 层实现，Controller 只做鉴权、参数校验和 `common.ApiSuccess` 包装。查询函数负责用户隔离、历史账户判定、无副作用统计 `direct_invite_count` / `qualified_paid_invite_count`、分页 envelope、管理员 join 用户字段、contact 使用 `common.UnmarshalJsonStr` 解析。
- `CreateInvitationCommissionForRewardEvent` 只在邀请人当前 `invitation_reward_mode = commission` 时入账；当前不是 `commission` 时返回 nil 且不创建 skipped 返佣记录，确保管理员之后切回 `commission` 时仍可按同一来源 fresh 计算。
- `CreateInvitationCommissionForRewardEvent` 必须通过 `event.SourceSubscriptionId -> UserSubscription -> SubscriptionPlan` fresh 校验来源当前资格：对应订阅仍存在且不是试用、不是 `monthly_invite_entitlement`，套餐当前 `reward_eligible = true`。当前不合格来源返回 nil，不创建返佣记录、不入账、不写 earned ledger；若后续套餐重新变为 `reward_eligible = true` 且来源仍有效，补偿任务可按当前规则入账。
- `HandleInvitationRewardForCompletedSubscriptionOrder(orderId)` 和 `HandleInvitationRewardForSubscriptionRedemption(redemptionId)` 是销售来源完成后的统一分发器：先读取对应 `InvitationRewardEvent`、invitee 和 inviter 当前模式；邀请人当前为 `commission` 时调用 `CreateInvitationCommissionForRewardEvent`；邀请人当前为 `subscription` 时继续调用现有 `service.TryEnsureInvitationEntitlementForPaidUser(inviteeId)` 或等价 `EnsureMonthlyInvitationEntitlement(inviterId, now)`，保持奖励套餐现有 active `user_subscriptions` 口径和即时刷新时机。`commission` 模式不得 upsert 奖励套餐，`subscription` 模式不得创建返佣记录。兑换码 handler 用于用户兑换接口事务提交后的同步处理，失败只影响同步处理，不回滚已使用兑换码，后续由补偿任务重试。
- `RetryPendingInvitationRewardEvents` 使用 `InvitationRewardEvent` 作为持久补偿源：对仍未生成返佣记录的 active 事件，若邀请人当前为 `commission` 则调用 `CreateInvitationCommissionForRewardEvent`；若邀请人当前为 `subscription` 则跳过返佣处理，不调用奖励套餐 upsert，因为奖励套餐补偿由 `GetInvitationEntitlementStatus` 和 sweep 按 active 直属订阅口径完成。已经有返佣记录或已处理的事件不能重复计数。`processed` 计数只统计本轮实际新建返佣记录、写入返佣账户 / ledger 的事件；不得为了避免重复计数而把 active 销售来源事件改成 skipped / cancelled。
- `StartInvitationRewardEventRetryTask` 作为后台任务从 `main.go` 启动，按小批量循环调用 `RetryPendingInvitationRewardEvents`，错误只记录但不丢失事件；它不能替代支付完成或兑换码兑换后的同步调用，只用于补偿同步处理失败。接入位置放在 `main.go` 现有后台任务启动区，例如 `service.StartInvitationEntitlementRefreshTask()` 之后；必须有 `main_task_startup_test.go` 或等价启动注册测试保护生产 `main.go` 中的 `service.StartInvitationRewardEventRetryTask()` 调用。
- 全局 `Enabled=false` 或 `RateBps <= 0` 只阻止新返佣入账，不创建会消耗 `(source_type, source_id)` 唯一键的 skipped 返佣记录；不得冻结历史返佣账户；已有 `InvitationCommissionAccount` 的用户仍可 summary、transfer、request withdrawal、admin complete/reject，`can_transfer` / `can_request_withdrawal` 必须基于历史账户余额和最小金额判断。
- 可用入账使用唯一索引 `(source_type, source_id)` 保证幂等；冲突时返回 nil。
- `rate_bps > 10000` 必须被配置校验拒绝；运行时遇到非法比例必须返回错误或不消耗来源，不能让 `source_amount_cents * rate_bps` 溢出。
- `ContactSnapshot` 使用 `common.Marshal` 写入；列表/响应使用 `common.UnmarshalJsonStr` 转回对象。
- 划转金额 `int64` 转 `int` 前必须调用 `model.AccountBalanceIntFromCents(amountCents)`；该 helper 校验 `amountCents > 0` 且不超过 `math.MaxInt`，失败时整个事务回滚。
- 任何扣减使用条件更新：`WHERE user_id = ? AND available_cents >= ?` 或 `WHERE user_id = ? AND pending_cents >= ?`。
- 控制器级契约测试必须固定默认 handler 切换：`controller/invitation_entitlement_test.go` 和 `controller/redemption_cny_test.go` 需断言默认 `handleInvitationReward...` 已切到正式 service 分发器；`main_task_startup_test.go` 需断言生产启动路径包含 `service.StartInvitationRewardEventRetryTask()`。
- 完成/拒绝使用条件更新：`WHERE id = ? AND status = 'pending'`。
- ledger 的 `*_after_cents` 必须读取同事务内变更后的账户值。

- [ ] **步骤 8：运行测试验证通过**

额外检查 `main.go`：确认现有后台任务启动区已经调用 `service.StartInvitationRewardEventRetryTask()`，且调用位于 `service.StartInvitationEntitlementRefreshTask()` 附近；不得在任务 2 提前接入。

运行步骤 6 命令，预期 PASS。


---

## 任务 5：用户与管理员返佣 API、用户奖励模式管理、路由

**文件：**
- 创建：`controller/invitation_commission.go`
- 创建：`controller/admin_tasks.go`
- 修改：`controller/user.go`
- 修改：`router/api-router.go`
- 测试：`controller/invitation_commission_test.go`
- 测试：`controller/admin_invitation_commission_test.go`

测试文件先增加本任务使用的 helper，放在 `controller/invitation_commission_test.go`，`controller/admin_invitation_commission_test.go` 直接复用同一 package helper：

```go
func setupInvitationCommissionControllerDB(t *testing.T) {
    t.Helper()
    gin.SetMode(gin.TestMode)
    db := setupModelListControllerTestDB(t)
    require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.UserSubscription{}, &model.InvitationRewardEvent{}, &model.InvitationCommissionAccount{}, &model.InvitationCommissionRecord{}, &model.InvitationCommissionLedger{}, &model.InvitationCommissionWithdrawal{}))
    setting := operation_setting.GetInvitationCommissionSetting()
    oldSetting := *setting
    *setting = operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000}
    t.Cleanup(func() { *setting = oldSetting })
    require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}).Error)
}

func seedControllerCommissionAccount(t *testing.T, userId int, available int64, pending int64, withdrawn int64, transferred int64) {
    t.Helper()
    require.NoError(t, model.DB.Create(&model.InvitationCommissionAccount{UserId: userId, AvailableCents: available, PendingCents: pending, WithdrawnCents: withdrawn, TransferredCents: transferred, CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}).Error)
}

func seedPendingCommissionWithdrawal(t *testing.T, userId int, withdrawalId int, amountCents int64, contactType string, contactValue string) {
    t.Helper()
    require.NoError(t, model.DB.Create(&model.User{Id: userId, Username: fmt.Sprintf("withdraw-user-%d", userId), Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
    seedControllerCommissionAccount(t, userId, 0, amountCents, 0, 0)
    contactSnapshot, err := common.Marshal(map[string]string{"type": contactType, "value": contactValue})
    require.NoError(t, err)
    now := common.GetTimestamp()
    require.NoError(t, model.DB.Create(&model.InvitationCommissionWithdrawal{Id: withdrawalId, UserId: userId, AmountCents: amountCents, Status: model.InvitationCommissionWithdrawalPending, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), UserRemark: "用户希望私聊确认", CreatedAt: now, UpdatedAt: now}).Error)
}

func performUserRequest(t *testing.T, userId int, method string, target string, body string) *httptest.ResponseRecorder {
    t.Helper()
    recorder := httptest.NewRecorder()
    router := gin.New()
    router.Use(func(c *gin.Context) {
        c.Set("id", userId)
        c.Set("username", fmt.Sprintf("user-%d", userId))
        c.Set("role", common.RoleCommonUser)
        c.Next()
    })
    router.GET("/api/user/invitation-commission/summary", GetInvitationCommissionSummary)
    router.GET("/api/user/invitation-commission/records", GetInvitationCommissionRecords)
    router.POST("/api/user/invitation-commission/transfer", TransferInvitationCommission)
    router.GET("/api/user/invitation-commission/withdrawals", GetInvitationCommissionWithdrawals)
    router.POST("/api/user/invitation-commission/withdrawals", CreateInvitationCommissionWithdrawal)
    router.PUT("/api/user/self", UpdateSelf)
    req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    router.ServeHTTP(recorder, req)
    return recorder
}

func performAdminRequest(t *testing.T, adminId int, method string, target string, body string) *httptest.ResponseRecorder {
    return performAdminRequestWithRole(t, adminId, common.RoleAdminUser, method, target, body)
}

func performAdminRequestWithRole(t *testing.T, adminId int, role int, method string, target string, body string) *httptest.ResponseRecorder {
    t.Helper()
    recorder := httptest.NewRecorder()
    router := gin.New()
    router.Use(func(c *gin.Context) {
        c.Set("id", adminId)
        c.Set("username", fmt.Sprintf("admin-%d", adminId))
        c.Set("role", role)
        c.Next()
    })
    router.GET("/api/admin/invitation-commission/withdrawals", AdminListInvitationCommissionWithdrawals)
    router.POST("/api/admin/invitation-commission/withdrawals/:id/complete", AdminCompleteInvitationCommissionWithdrawal)
    router.POST("/api/admin/invitation-commission/withdrawals/:id/reject", AdminRejectInvitationCommissionWithdrawal)
    router.GET("/api/admin/tasks/summary", GetAdminTasksSummary)
    // Test helper only: mirrors the existing admin user update endpoint mounted as `/api/user/` in api-router adminRoute.
    router.PUT("/api/user/", UpdateUser)
    req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    router.ServeHTTP(recorder, req)
    return recorder
}
```

该 helper 需要 imports：`bytes`、`fmt`、`net/http/httptest`、`github.com/gin-gonic/gin`、`github.com/QuantumNous/new-api/setting/operation_setting`；写入联系方式快照必须使用 `common.Marshal`。

- [ ] **步骤 1：编写失败测试：普通用户权限与历史返佣账户**

在 `controller/invitation_commission_test.go` 增加：

```go
func TestInvitationCommissionSummaryAndTransferPermissions(t *testing.T) {
    setupInvitationCommissionControllerDB(t)
    require.NoError(t, model.DB.Create(&model.User{Id: 9501, Username: "history-user", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeSubscription, Quota: 0}).Error)
    seedControllerCommissionAccount(t, 9501, 3000, 0, 0, 0)
    setting := operation_setting.GetInvitationCommissionSetting()
    oldSetting := *setting
    *setting = operation_setting.InvitationCommissionSetting{Enabled: false, RateBps: 1000, MinimumTransferCents: 1000, MinimumWithdrawCents: 1000}
    t.Cleanup(func() { *setting = oldSetting })

    summary := performUserRequest(t, 9501, http.MethodGet, "/api/user/invitation-commission/summary", "")
    require.Equal(t, http.StatusOK, summary.Code)
    assert.Contains(t, summary.Body.String(), `"reward_mode":"subscription"`)
    assert.Contains(t, summary.Body.String(), `"has_commission_account":true`)
    assert.Contains(t, summary.Body.String(), `"can_transfer":true`)
    assert.Contains(t, summary.Body.String(), `"can_request_withdrawal":true`)
    assert.Contains(t, summary.Body.String(), `"enabled":false`)

    transfer := performUserRequest(t, 9501, http.MethodPost, "/api/user/invitation-commission/transfer", `{"amount_cents":1000}`)
    require.Equal(t, http.StatusOK, transfer.Code)
    assert.Contains(t, transfer.Body.String(), `"available_cents":2000`)
}

func TestHistoricalCommissionAccountCanRequestManualCashbackInSubscriptionMode(t *testing.T) {
    setupInvitationCommissionControllerDB(t)
    require.NoError(t, model.DB.Create(&model.User{Id: 9502, Username: "history-withdraw", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeSubscription}).Error)
    seedControllerCommissionAccount(t, 9502, 3000, 0, 0, 0)

    withdrawal := performUserRequest(t, 9502, http.MethodPost, "/api/user/invitation-commission/withdrawals", `{"amount_cents":1000,"contact":{"type":"wechat","value":"history-contact"},"remark":"历史余额返现"}`)

    require.Equal(t, http.StatusOK, withdrawal.Code)
    assert.Contains(t, withdrawal.Body.String(), `"status":"pending"`)
    assert.Contains(t, withdrawal.Body.String(), `"user_remark":"历史余额返现"`)
    assert.NotContains(t, withdrawal.Body.String(), `"remark":"历史余额返现"`)
    var account model.InvitationCommissionAccount
    require.NoError(t, model.DB.Where("user_id = ?", 9502).First(&account).Error)
    assert.Equal(t, int64(2000), account.AvailableCents)
    assert.Equal(t, int64(1000), account.PendingCents)
}

func TestInvitationCommissionRecordsExposeStableFieldContract(t *testing.T) {
    setupInvitationCommissionControllerDB(t)
    require.NoError(t, model.DB.Create(&model.User{Id: 9503, Username: "record-inviter", Status: common.UserStatusEnabled, InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
    require.NoError(t, model.DB.Create(&model.User{Id: 9504, Username: "record-invitee", Status: common.UserStatusEnabled, InviterId: 9503}).Error)
    now := common.GetTimestamp()
    order := model.SubscriptionOrder{Id: 9505, UserId: 9504, TradeNo: "record-contract-order", AmountCents: 4000, Currency: "CNY", Status: common.TopUpStatusSuccess, CreateTime: now}
    require.NoError(t, model.DB.Create(&order).Error)
    event := model.InvitationRewardEvent{Id: 9506, InviterId: 9503, InviteeId: 9504, SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: order.Id, SourceOrderId: order.Id, SourceAmountCents: 4000, SourceCurrency: "CNY", Status: model.InvitationRewardEventStatusActive, CreatedAt: now}
    require.NoError(t, model.DB.Create(&event).Error)
    record := model.InvitationCommissionRecord{EventId: event.Id, InviterId: 9503, InviteeId: 9504, SourceType: model.InvitationCommissionSourceSubscriptionOrder, SourceId: order.Id, SourceTradeNo: order.TradeNo, SourceAmountCents: 4000, SourceCurrency: "CNY", CommissionRateBps: 1000, CommissionCents: 400, Status: model.InvitationCommissionStatusAvailable, CreatedAt: now, AvailableAt: now}
    require.NoError(t, model.DB.Create(&record).Error)

    response := performUserRequest(t, 9503, http.MethodGet, "/api/user/invitation-commission/records?page=1&page_size=20", "")

    require.Equal(t, http.StatusOK, response.Code)
    assert.Contains(t, response.Body.String(), `"invitee_id":9504`)
    assert.Contains(t, response.Body.String(), `"source_trade_no":"record-contract-order"`)
    assert.Contains(t, response.Body.String(), `"status":"available"`)
    assert.Contains(t, response.Body.String(), `"available_at":`)
    assert.Contains(t, response.Body.String(), `"cancelled_at":0`)
}
```

- [ ] **步骤 2：编写失败测试：无返佣模式且无历史账户不能写操作**

```go
func TestInvitationCommissionWriteRejectsNonCommissionWithoutHistory(t *testing.T) {
    setupInvitationCommissionControllerDB(t)
    require.NoError(t, model.DB.Create(&model.User{Id: 9511, Username: "plain-user", Status: common.UserStatusEnabled}).Error)

    transfer := performUserRequest(t, 9511, http.MethodPost, "/api/user/invitation-commission/transfer", `{"amount_cents":100}`)
    assert.Contains(t, transfer.Body.String(), "返佣")

    withdrawal := performUserRequest(t, 9511, http.MethodPost, "/api/user/invitation-commission/withdrawals", `{"amount_cents":1000,"contact":{"type":"wechat","value":"u"}}`)
    assert.Contains(t, withdrawal.Body.String(), "返佣")
}
```

- [ ] **步骤 3：编写失败测试：管理员列表、complete/reject、待办摘要**

在 `controller/admin_invitation_commission_test.go` 增加：

```go
func TestAdminInvitationCommissionWithdrawalListCompleteRejectAndTaskSummary(t *testing.T) {
    setupInvitationCommissionControllerDB(t)
    seedPendingCommissionWithdrawal(t, 9521, 9522, 5000, "wechat", "contact")

    list := performAdminRequest(t, 1, http.MethodGet, "/api/admin/invitation-commission/withdrawals?status=pending&page=1&page_size=20", "")
    require.Equal(t, http.StatusOK, list.Code)
    assert.Contains(t, list.Body.String(), `"status":"pending"`)
    assert.Contains(t, list.Body.String(), `"contact":{"type":"wechat","value":"contact"}`)

    summary := performAdminRequest(t, 1, http.MethodGet, "/api/admin/tasks/summary", "")
    require.Equal(t, http.StatusOK, summary.Code)
    assert.Contains(t, list.Body.String(), `"user_remark":"用户希望私聊确认"`)
    assert.NotContains(t, list.Body.String(), `"remark":"用户希望私聊确认"`)
    assert.Contains(t, summary.Body.String(), `"pending_commission_withdrawals":1`)

    complete := performAdminRequest(t, 1, http.MethodPost, "/api/admin/invitation-commission/withdrawals/9522/complete", `{"admin_remark":"已线下返现"}`)
    require.Equal(t, http.StatusOK, complete.Code)
    retry := performAdminRequest(t, 1, http.MethodPost, "/api/admin/invitation-commission/withdrawals/9522/reject", `{"admin_remark":"重复"}`)
    assert.Contains(t, retry.Body.String(), "pending")

    summaryAfter := performAdminRequest(t, 1, http.MethodGet, "/api/admin/tasks/summary", "")
    assert.Contains(t, summaryAfter.Body.String(), `"pending_commission_withdrawals":0`)
}
```

- [ ] **步骤 4：编写失败测试：用户奖励模式只能管理员更新**

```go
func TestInvitationRewardModeCanOnlyBeUpdatedByAdminUserEndpoint(t *testing.T) {
    setupInvitationCommissionControllerDB(t)
    require.NoError(t, model.DB.Create(&model.User{Id: 9531, Username: "mode-target", Status: common.UserStatusEnabled}).Error)

    adminUpdate := performAdminRequest(t, 1, http.MethodPut, "/api/user/", `{"id":9531,"username":"mode-target","display_name":"mode-target","invitation_reward_mode":"commission"}`)
    require.Equal(t, http.StatusOK, adminUpdate.Code)
    var user model.User
    require.NoError(t, model.DB.First(&user, 9531).Error)
    assert.Equal(t, model.InvitationRewardModeCommission, user.InvitationRewardMode)

    rootUpdate := performAdminRequestWithRole(t, 2, common.RoleRootUser, http.MethodPut, "/api/user/", `{"id":9531,"username":"mode-target","display_name":"mode-target","invitation_reward_mode":"subscription"}`)
    require.Equal(t, http.StatusOK, rootUpdate.Code)
    require.NoError(t, model.DB.First(&user, 9531).Error)
    assert.Equal(t, model.InvitationRewardModeSubscription, user.InvitationRewardMode)

    invalidUpdate := performAdminRequest(t, 1, http.MethodPut, "/api/user/", `{"id":9531,"username":"mode-target","display_name":"mode-target","invitation_reward_mode":"invalid"}`)
    require.Equal(t, http.StatusOK, invalidUpdate.Code)
    assert.Contains(t, invalidUpdate.Body.String(), `"success":false`)
    require.NoError(t, model.DB.First(&user, 9531).Error)
    assert.Equal(t, model.InvitationRewardModeSubscription, user.InvitationRewardMode)

    selfUpdate := performUserRequest(t, 9531, http.MethodPut, "/api/user/self", `{"display_name":"self","invitation_reward_mode":"commission"}`)
    require.Equal(t, http.StatusOK, selfUpdate.Code)
    require.NoError(t, model.DB.First(&user, 9531).Error)
    assert.Equal(t, model.InvitationRewardModeSubscription, user.InvitationRewardMode)
}
```

- [ ] **步骤 5：运行测试验证失败**

```bash
go test ./controller -run 'TestInvitationCommission|TestAdminInvitationCommission|TestInvitationRewardModeCanOnlyBeUpdatedByAdminUserEndpoint' -count=1
```

预期：FAIL，路由和 controller 尚不存在或字段未更新。

- [ ] **步骤 6：实现 Controller 与路由**

路由注册：

```go
selfRoute.GET("/invitation-commission/summary", controller.GetInvitationCommissionSummary)
selfRoute.GET("/invitation-commission/records", controller.GetInvitationCommissionRecords)
selfRoute.POST("/invitation-commission/transfer", middleware.CriticalRateLimit(), controller.TransferInvitationCommission)
selfRoute.GET("/invitation-commission/withdrawals", controller.GetInvitationCommissionWithdrawals)
selfRoute.POST("/invitation-commission/withdrawals", middleware.CriticalRateLimit(), controller.CreateInvitationCommissionWithdrawal)

adminCommissionRoute := apiRouter.Group("/admin/invitation-commission")
adminCommissionRoute.Use(middleware.AdminAuth())
adminCommissionRoute.GET("/withdrawals", controller.AdminListInvitationCommissionWithdrawals)
adminCommissionRoute.POST("/withdrawals/:id/complete", middleware.CriticalRateLimit(), controller.AdminCompleteInvitationCommissionWithdrawal)
adminCommissionRoute.POST("/withdrawals/:id/reject", middleware.CriticalRateLimit(), controller.AdminRejectInvitationCommissionWithdrawal)

adminTasksRoute := apiRouter.Group("/admin/tasks")
adminTasksRoute.Use(middleware.AdminAuth())
adminTasksRoute.GET("/summary", controller.GetAdminTasksSummary)
```

实现注意：

- 列表统一使用 `page` / `page_size`，响应 data payload 为 `{items,total,page,page_size}`。
- Controller 用 `common.ApiSuccess(c, payload)` 包装响应。
- `admin_remark` trim 后必须非空，长度不超过 500。
- `contact.type` 枚举：`wechat`、`telegram`、`email`、`other`；`contact.value` trim 后 1–128。
- 返现申请响应字段必须使用 `user_remark`，不得把 `InvitationCommissionWithdrawal.UserRemark` 序列化为 `remark`；请求体仍使用 `remark` 作为用户提交备注字段。
- 返佣记录响应字段必须包含 `invitee_id`、`source_trade_no`、`available_at`、`cancelled_at`；状态只允许 `available`、`skipped`、`cancelled`。
- 当前 `reward_mode = subscription` 但存在历史返佣账户且余额足够的用户，仍可调用 transfer 和 withdrawals；无历史账户时返回业务错误。
- `controller/user.go` 的管理员 `UpdateUser` 必须只接受 `subscription` 或 `commission`；非法值返回业务错误。普通用户 `UpdateSelf` 不读取该字段。
- `GetSelf` 和用户列表响应中包含 `invitation_reward_mode`，但不能泄露敏感字段。

- [ ] **步骤 7：运行测试验证通过**

运行步骤 5 命令，预期 PASS。

---

## 任务 6：前端用户管理和钱包返佣体验

**文件：**
- 修改：`web/default/src/features/users/types.ts`
- 修改：`web/default/src/features/users/lib/user-form.ts`
- 修改：`web/default/src/features/users/components/users-mutate-drawer.tsx`
- 修改：`web/default/src/features/users/components/users-columns.tsx`
- 修改：`web/default/src/features/wallet/types.ts`
- 修改：`web/default/src/features/wallet/api.ts`
- 创建：`web/default/src/features/wallet/hooks/use-invitation-commission.ts`
- 修改：`web/default/src/features/wallet/components/affiliate-rewards-card.tsx`
- 创建：`web/default/src/features/wallet/components/dialogs/commission-transfer-dialog.tsx`
- 创建：`web/default/src/features/wallet/components/dialogs/commission-withdrawal-dialog.tsx`
- 创建：`web/default/src/features/users/users-form.test.ts`
- 测试：`web/default/src/features/wallet/wallet-layout.test.ts`

- [ ] **步骤 1：编写失败测试：用户表单契约包含邀请奖励模式**

创建 `web/default/src/features/users/users-form.test.ts`，使用项目现有 Bun + `node:test` 模式，不引入新的测试框架：

```ts
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import {
  transformFormDataToPayload,
  transformUserToFormDefaults,
} from './lib/user-form'

test('includes invitation_reward_mode when updating an admin-managed user', () => {
  const payload = transformFormDataToPayload(
    {
      username: 'agent',
      display_name: 'Agent',
      password: '',
      role: 1,
      quota_cny: '0.00',
      remark: '',
      invitation_reward_mode: 'commission',
    },
    100
  )

  assert.deepEqual(
    {
      id: payload.id,
      username: payload.username,
      invitation_reward_mode: payload.invitation_reward_mode,
    },
    {
      id: 100,
      username: 'agent',
      invitation_reward_mode: 'commission',
    }
  )
})

test('defaults missing invitation_reward_mode to subscription', () => {
  const defaults = transformUserToFormDefaults({
    id: 1,
    username: 'plain',
    display_name: 'Plain',
    quota: 0,
    status: 1,
    role: 1,
    used_quota: 0,
    request_count: 0,
  })

  assert.equal(defaults.invitation_reward_mode, 'subscription')
})

test('user drawer and columns expose invitation reward mode controls', () => {
  const drawer = readFileSync(
    'src/features/users/components/users-mutate-drawer.tsx',
    'utf8'
  )
  const columns = readFileSync(
    'src/features/users/components/users-columns.tsx',
    'utf8'
  )

  assert.match(drawer, /invitation_reward_mode/)
  assert.match(drawer, /Reward package/)
  assert.match(drawer, /Commission/)
  assert.match(drawer, /Commission is only available for invited special users enabled by administrators\./)
  assert.match(columns, /invitation_reward_mode/)
  assert.match(columns, /subscription/)
  assert.match(columns, /commission/)
})
```

- [ ] **步骤 2：编写失败测试：钱包源码契约不把划转描述为审核**

在 `web/default/src/features/wallet/wallet-layout.test.ts` 增加源码契约或组件测试。该文件已有 `assert`、`readFileSync`、`test` imports 时必须复用，不要重复导入：

```ts
test('commission transfer copy is immediate and withdrawal copy is manual', () => {
  const card = readFileSync(
    'src/features/wallet/components/affiliate-rewards-card.tsx',
    'utf8'
  )
  const transferDialog = readFileSync(
    'src/features/wallet/components/dialogs/commission-transfer-dialog.tsx',
    'utf8'
  )
  const withdrawalDialog = readFileSync(
    'src/features/wallet/components/dialogs/commission-withdrawal-dialog.tsx',
    'utf8'
  )

  assert.match(card, /Transfer to balance/)
  assert.match(card, /Request manual cashback/)
  assert.match(card, /This is not an automatic payout\./)
  assert.doesNotMatch(transferDialog, /review/i)
  assert.doesNotMatch(transferDialog, /approval/i)
  assert.match(withdrawalDialog, /manual/i)
})

test('commission wallet keeps referral link and uses side-effect-free summary stats', () => {
  const card = readFileSync(
    'src/features/wallet/components/affiliate-rewards-card.tsx',
    'utf8'
  )
  const hook = readFileSync(
    'src/features/wallet/hooks/use-invitation-commission.ts',
    'utf8'
  )

  assert.match(card, /Copy referral link/)
  assert.match(card, /direct_invite_count/)
  assert.match(card, /qualified_paid_invite_count/)
  assert.match(hook, /\['wallet', 'invitation-commission', 'summary', userId\]/)
  assert.match(hook, /\['wallet', 'invitation-commission', 'records', userId, params\]/)
  assert.match(hook, /\['wallet', 'invitation-commission', 'withdrawals', userId, params\]/)
  assert.match(hook, /enabled:\s*Boolean\(userId\)/)
})

```

上述源码契约测试只是最低限度。若实现中已存在可直接导入的纯函数或组件测试工具，优先补可执行行为测试：传入 `reward_mode = 'commission'` 或 `has_commission_account = true` 时出现返佣区，传入 `reward_mode = 'subscription'` 且 `has_commission_account = false` 时保持奖励套餐区；划转和返现 mutation 成功后通过 fake query client 断言同一 `userId` 的 summary、records、withdrawals 均被 invalidated，划转返回 `user_quota` 后 auth store/current user quota 被更新。

```ts
test('commission mutations refresh private wallet data and auth balance', () => {
  const hook = readFileSync(
    'src/features/wallet/hooks/use-invitation-commission.ts',
    'utf8'
  )
  const api = readFileSync('src/features/wallet/api.ts', 'utf8')

  assert.match(api, /\.data\.data/)
  assert.match(hook, /invalidateQueries\(\{\s*queryKey:\s*\['wallet', 'invitation-commission', 'summary', userId\]/)
  assert.match(hook, /invalidateQueries\(\{\s*queryKey:\s*\['wallet', 'invitation-commission', 'records', userId\]/)
  assert.match(hook, /invalidateQueries\(\{\s*queryKey:\s*\['wallet', 'invitation-commission', 'withdrawals', userId\]/)
  assert.match(hook, /auth\.setUser|setUser\(/)
  assert.match(hook, /user_quota/)
})

test('subscription mode with historical commission account keeps commission actions visible', () => {
  const card = readFileSync(
    'src/features/wallet/components/affiliate-rewards-card.tsx',
    'utf8'
  )

  assert.match(card, /has_commission_account/)
  assert.match(card, /Historical commission balance can still be handled/)
})
```

- [ ] **步骤 3：运行测试验证失败**

```bash
(cd web/default && bun test src/features/users/users-form.test.ts src/features/wallet/wallet-layout.test.ts)
```

预期：FAIL，类型和新文件尚不存在。

- [ ] **步骤 4：实现用户管理类型和表单**

要求：

```ts
export type InvitationRewardMode = 'subscription' | 'commission'
```

`userSchema` 增加：

```ts
invitation_reward_mode: z.enum(['subscription', 'commission']).optional(),
```

`UserFormData` 增加：

```ts
invitation_reward_mode?: InvitationRewardMode
```

`userFormSchema`、`USER_FORM_DEFAULT_VALUES`、`transformUserToFormDefaults`、`transformFormDataToPayload` 增加同名字段。`UsersMutateDrawer` 在 update 模式下展示 Select：

- `subscription` label 为 `Reward package`。
- `commission` label 为 `Commission`。
- 描述为 `Commission is only available for invited special users enabled by administrators.`。

`users-columns.tsx` 显示模式 badge，空值按 `subscription` 展示。

- [ ] **步骤 5：实现钱包返佣 API、hooks 与 UI**

新增类型必须完整固定后端契约：

```ts
export interface PageParams {
  page: number
  page_size: number
}

export interface PageEnvelope<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface InvitationCommissionContact {
  type: 'wechat' | 'telegram' | 'email' | 'other'
  value: string
}

export interface InvitationCommissionSummary {
  reward_mode: 'subscription' | 'commission'
  has_commission_account: boolean
  can_transfer: boolean
  can_request_withdrawal: boolean
  direct_invite_count: number
  qualified_paid_invite_count: number
  account: {
    available_cents: number
    pending_cents: number
    withdrawn_cents: number
    transferred_cents: number
  }
  setting: {
    enabled: boolean
    minimum_withdraw_cents: number
    minimum_transfer_cents: number
    rate_bps: number
  }
}

export interface InvitationCommissionRecord {
  id: number
  event_id: number
  invitee_id: number
  source_type: string
  source_id: number
  source_trade_no: string
  source_amount_cents: number
  source_currency: string
  commission_rate_bps: number
  commission_cents: number
  status: 'available' | 'skipped' | 'cancelled'
  reason: string
  created_at: number
  available_at: number
  cancelled_at: number
}

export interface InvitationCommissionTransferResult {
  available_cents: number
  transferred_cents: number
  user_quota: number
}

export interface InvitationCommissionWithdrawalPayload {
  amount_cents: number
  contact: InvitationCommissionContact
  remark?: string
}

export interface InvitationCommissionWithdrawal {
  id: number
  amount_cents: number
  status: 'pending' | 'completed' | 'rejected'
  method: 'manual'
  contact: InvitationCommissionContact
  user_remark: string
  admin_remark: string
  reviewer_id: number
  completed_by: number
  completed_at: number
  reviewed_at: number
  created_at: number
  updated_at: number
}
```

API helper 必须解包 `common.ApiSuccess` / `ApiResponse<T>` 的外层响应：HTTP `res.data` 是 `{ success, message, data }`，UI 和 hook 只能消费其中的 `data` payload；不得把规格示例中的 payload 当成响应根对象。固定签名：

```ts
export async function getInvitationCommissionSummary(): Promise<InvitationCommissionSummary>
export async function getInvitationCommissionRecords(params: PageParams): Promise<PageEnvelope<InvitationCommissionRecord>>
export async function transferInvitationCommission(amount_cents: number): Promise<InvitationCommissionTransferResult>
export async function requestInvitationCommissionWithdrawal(payload: InvitationCommissionWithdrawalPayload): Promise<InvitationCommissionWithdrawal>
export async function getInvitationCommissionWithdrawals(params: PageParams): Promise<PageEnvelope<InvitationCommissionWithdrawal>>
```

若需要在 API helper 内保留 `ApiResponse<T>` 类型，只能作为 axios 响应类型参数使用；函数返回值必须是已解包的 payload。失败提示继续使用项目统一 axios 拦截器和现有错误处理约定。

React Query keys 必须包含 `userId` 和分页参数，避免同一 SPA 会话切换用户后复用私有缓存：

```ts
['wallet', 'invitation-commission', 'summary', userId]
['wallet', 'invitation-commission', 'records', userId, params]
['wallet', 'invitation-commission', 'withdrawals', userId, params]
```

`records` 和 `withdrawals` hooks 的 `enabled` 必须依赖 `Boolean(userId)`；mutation invalidation 使用同一用户维度 key 前缀。

mutation invalidation：

- 划转成功后刷新同一 `userId` 的 summary、records、withdrawals，使用返回的 `user_quota` 更新当前用户余额/auth store `quota`，不得留下钱包余额 stale cache。
- 申请返现成功后刷新同一 `userId` 的 summary、records、withdrawals。
- 所有 invalidation 使用 `['wallet', 'invitation-commission', ... , userId]`，不得用不带 `userId` 的宽泛私有 key。

`AffiliateRewardsCard` 接收 `commissionSummary` 或在父层传入；当 `reward_mode === 'commission' || has_commission_account` 时显示返佣区；否则保持现有奖励套餐区。邀请链接输入框、复制按钮、邀请统计始终保留。历史返佣账户但当前套餐模式时显示：`New paid invitations will receive reward packages. Historical commission balance can still be handled.`

- [ ] **步骤 6：运行测试验证通过**

```bash
(cd web/default && bun test src/features/users/users-form.test.ts src/features/wallet/wallet-layout.test.ts)
(cd web/default && bun run typecheck)
```

预期：测试 PASS，typecheck PASS。

---

## 任务 7：前端管理员返现审核页、侧边栏待办 badge、i18n

**文件：**
- 创建：`web/default/src/features/invitation-commission/types.ts`
- 创建：`web/default/src/features/invitation-commission/api.ts`
- 创建：`web/default/src/features/invitation-commission/admin-withdrawals.tsx`
- 创建：`web/default/src/routes/_authenticated/invitation-commission/withdrawals/index.tsx`
- 修改：`web/default/src/hooks/use-sidebar-data.ts`
- 修改：`web/default/src/hooks/use-sidebar-config.ts`
- 检查：`web/default/src/components/layout/types.ts`、`web/default/src/components/layout/components/nav-group.tsx` —— 若当前尚未支持 badge，则补齐；若已支持，不重复修改。
- 修改：`controller/user.go` —— 后端默认 sidebar config 增加 `invitation_commission`。
- 修改：`model/user.go` —— 新用户默认 sidebar config 增加 `invitation_commission`。
- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/vi.json`
- 修改：`web/default/src/i18n/static-keys.ts`
- 测试：`web/default/src/features/invitation-commission/admin-withdrawals.test.ts`
- 测试：`web/default/src/hooks/use-sidebar-config.test.ts`
- [ ] **步骤 1：编写失败测试：管理员 API、路由 guard 和 query invalidation 契约**

在 `web/default/src/features/invitation-commission/admin-withdrawals.test.ts` 增加，使用 `node:test`。先保留页面/API 源码契约测试：

```ts
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

test('admin withdrawals page uses fixed route guard and refreshes withdrawals plus task summary', () => {
  const route = readFileSync(
    'src/routes/_authenticated/invitation-commission/withdrawals/index.tsx',
    'utf8'
  )
  const page = readFileSync(
    'src/features/invitation-commission/admin-withdrawals.tsx',
    'utf8'
  )
  const api = readFileSync('src/features/invitation-commission/api.ts', 'utf8')

  assert.match(route, /\/_authenticated\/invitation-commission\/withdrawals\//)
  assert.match(route, /beforeLoad/)
  assert.match(route, /ROLE\.ADMIN/)
  assert.match(route, /redirect\(\{\s*to:\s*'\/403'/)
  assert.match(page, /Mark manual cashback as completed/)
  assert.match(page, /\['admin', 'invitation-commission', 'withdrawals'/)
  assert.match(page, /\['admin', 'tasks', 'summary'\]/)
  assert.match(page, /user_remark/)
  assert.doesNotMatch(page, /\.remark\b/)
  assert.match(api, /\/api\/admin\/invitation-commission\/withdrawals/)
  assert.match(api, /\/api\/admin\/tasks\/summary/)
})
```

同时增加一个可执行行为测试，直接导入路由模块或其导出的 guard helper，设置 `useAuthStore` 为普通用户并断言 `beforeLoad` 抛出的 redirect 目标为 `/403`；再设置 `ROLE.ADMIN` 和 `ROLE.SUPER_ADMIN` 用户断言不抛错。不得只用源码字符串匹配替代该权限行为测试。


- [ ] **步骤 2：编写失败测试：侧边栏入口权限、URL 映射和 badge 查询条件**

在 `web/default/src/hooks/use-sidebar-config.test.ts` 增加或扩展。该文件已有 `assert` 和 `test` imports；若仍保留源码契约读取文件，只新增 `import { readFileSync } from 'node:fs'`，不要重复导入 `assert` 或 `test`。侧边栏待办摘要必须有可执行行为测试覆盖 4 个状态；源码正则测试只能保留为辅助，不能替代行为测试：

```ts
test('maps invitation commission withdrawals to admin invitation_commission module', () => {
  const defaults = getDefaultSidebarModulesForTest()
  const groups: NavGroup[] = [
    {
      id: 'admin',
      title: 'Admin',
      items: [{ title: 'Manual cashback requests', url: '/invitation-commission/withdrawals' }],
    },
  ]

  const filtered = filterSidebarNavGroupsForConfig(groups, defaults, null)

  assert.equal(defaults.admin?.invitation_commission, true)
  assert.equal(filtered[0]?.items[0]?.url, '/invitation-commission/withdrawals')
  assert.equal(
    filterSidebarNavGroupsForConfig(groups, defaults, {
      admin: { enabled: true, invitation_commission: false },
    }).length,
    0
  )
})

test('admin task summary query is enabled only for visible admin commission entry', () => {
  const adminGroups: NavGroup[] = [
    {
      id: 'admin',
      title: 'Admin',
      items: [{ title: 'Manual cashback requests', url: '/invitation-commission/withdrawals' }],
    },
  ]

  assert.equal(shouldFetchAdminTasksSummaryForTest(undefined, adminGroups), false)
  assert.equal(shouldFetchAdminTasksSummaryForTest(ROLE.USER, adminGroups), false)
  assert.equal(
    shouldFetchAdminTasksSummaryForTest(
      ROLE.ADMIN,
      filterSidebarNavGroupsForConfig(adminGroups, {
        admin: { enabled: true, invitation_commission: false },
      }, null)
    ),
    false
  )
  assert.equal(shouldFetchAdminTasksSummaryForTest(ROLE.ADMIN, adminGroups), true)
  assert.equal(shouldFetchAdminTasksSummaryForTest(ROLE.SUPER_ADMIN, adminGroups), true)
})

test('admin task summary failure hides badge without toast', async () => {
  let toastCalled = false
  const result = await resolvePendingCommissionWithdrawalsBadgeForTest(
    async () => {
      throw new Error('network')
    },
    () => { toastCalled = true }
  )

  assert.equal(result, undefined)
  assert.equal(toastCalled, false)
})
```

实现 `shouldFetchAdminTasksSummaryForTest` 和 `resolvePendingCommissionWithdrawalsBadgeForTest` 时必须复用生产判断逻辑；不得创建只供测试通过但生产不用的平行逻辑。badge 查询应在侧边栏已经经过 `useSidebarConfig` / `filterSidebarNavGroupsForConfig` 过滤后启用，推荐在过滤后的组件层或导出的共享 helper（例如 `isAdminCommissionNavVisible(navGroups)`）中判断；不得在 `use-sidebar-data.ts` 复制一套独立的可见性判断。若实现者选择组件行为测试，也必须覆盖同样 4 个状态：普通用户不请求 `/api/admin/tasks/summary`；管理员但 `invitation_commission` sidebar 配置不可见时不请求；管理员且入口可见时才请求；请求失败时 badge 为 `undefined` 且不触发 toast。

- [ ] **步骤 3：运行测试验证失败**

```bash
(cd web/default && bun test src/features/invitation-commission/admin-withdrawals.test.ts src/hooks/use-sidebar-config.test.ts)
```

预期：FAIL，新页面和映射尚不存在。

- [ ] **步骤 4：实现管理员返现审核页**
管理员类型必须固定接口契约：

```ts
export interface AdminInvitationCommissionWithdrawal {
  id: number
  user_id: number
  username: string
  display_name: string
  amount_cents: number
  status: 'pending' | 'completed' | 'rejected'
  method: 'manual'
  contact: InvitationCommissionContact
  user_remark: string
  admin_remark: string
  reviewer_id: number
  completed_by: number
  reviewed_at: number
  completed_at: number
  created_at: number
  updated_at: number
}

export interface AdminTasksSummary {
  pending_commission_withdrawals: number
}

export interface AdminInvitationCommissionWithdrawalParams extends PageParams {
  status?: 'pending' | 'completed' | 'rejected'
  user_id?: number
}
```

管理员 API helper 必须与钱包 helper 一样解包 `common.ApiSuccess` / `ApiResponse<T>` 外层响应，只返回 payload：

```ts
export async function listAdminInvitationCommissionWithdrawals(params: AdminInvitationCommissionWithdrawalParams): Promise<PageEnvelope<AdminInvitationCommissionWithdrawal>>
export async function completeInvitationCommissionWithdrawal(id: number, admin_remark: string): Promise<void>
export async function rejectInvitationCommissionWithdrawal(id: number, admin_remark: string): Promise<void>
export async function getAdminTasksSummary(): Promise<AdminTasksSummary>
```

`web/default/src/features/invitation-commission/admin-withdrawals.test.ts` 还必须增加成功路径契约，断言 API helper 或页面读取的是 `res.data.data.items` / `res.data.data.pending_commission_withdrawals`，不得把 `res.data` 当作 payload 根对象。
基础分页与联系方式类型来自任务 6 的 `web/default/src/features/wallet/types.ts`：`PageParams`、`PageEnvelope`、`InvitationCommissionContact` 必须从钱包类型文件 `import type` 后 re-export，或抽到共享 `web/default/src/features/invitation-commission/shared-types.ts` 并同步更新任务 6 import。不得在管理员模块直接引用未定义类型。

页面要求：

- 默认筛选 `pending`。
- 状态筛选：`pending`、`completed`、`rejected`。
- 支持按用户 ID 搜索。
- 表格展示：用户 ID、用户名、显示名、金额、状态、联系方式类型、联系方式值、用户备注、管理员备注、创建时间、处理时间。
- 联系方式提供复制按钮。
- `complete` 和 `reject` 都要求管理员填写备注；完成按钮文案 `Mark manual cashback as completed` /「标记已线下返现」。
- mutation 成功后 invalidate：
  - `['admin', 'invitation-commission', 'withdrawals', params]`
  - `['admin', 'tasks', 'summary']`
- 失败使用现有 `handleServerError` 或 toast 约定，不自行解析非标准错误。

路由文件必须包含管理员 guard：

```tsx
import { createFileRoute, redirect } from '@tanstack/react-router'
import { AdminInvitationCommissionWithdrawals } from '@/features/invitation-commission/admin-withdrawals'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute(
  '/_authenticated/invitation-commission/withdrawals/'
)({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: AdminInvitationCommissionWithdrawals,
})
```

TanStack Router 生成器必须生成 URL `/invitation-commission/withdrawals`；若生成器要求不同的 route id 字符串，只调整 `createFileRoute` 的字符串，不改 URL 和文件路径。

- [ ] **步骤 5：实现侧边栏入口和 badge**

在 `use-sidebar-data.ts`：

- 引入合适图标，例如 `BadgeDollarSign` 或 `HandCoins`。
- Admin 组中 `Users` 后插入：

```ts
{
  title: t('Manual cashback requests'),
  url: '/invitation-commission/withdrawals',
  icon: HandCoins,
  badge: pendingCommissionWithdrawals > 0 ? String(pendingCommissionWithdrawals) : undefined,
}
```

`NavItem` 已支持 `badge?: string`；如当前 `nav-group.tsx` 已在普通 link、折叠项触发器和子项中渲染 badge，不重复改造。若缺少任一位置，则补齐。badge 只显示正数，不改变现有 active 判断。

查询条件：

- 只在当前用户为管理员/root 且 Admin sidebar 项实际可见时启用 `GET /api/admin/tasks/summary`。
- 普通用户不得发起该请求。
- 管理员但 `invitation_commission` sidebar 配置不可见时不得发起该请求。
- 请求失败只隐藏 badge，不 toast；API helper 必须同时抑制 HTTP 错误 toast 和业务错误 toast，按项目约定同时设置 `skipErrorHandler` 与 `skipBusinessError`，或使用等效且已有的双通道错误抑制机制。

在 `use-sidebar-config.ts`：

- `DEFAULT_SIDEBAR_MODULES.admin` 增加 `invitation_commission: true`。
- `URL_TO_CONFIG_MAP` 增加 `'/invitation-commission/withdrawals': { section: 'admin', module: 'invitation_commission' }`。
- 后端默认 sidebar config 的 admin 区域也要加入 `invitation_commission`，见 `controller/user.go` 和 `model/user.go` 的默认配置函数。

- [ ] **步骤 6：补齐 6 个 locale**

至少加入以下 key：

```text
Invitation reward mode
Reward package
Commission
Commission is only available for invited special users enabled by administrators.
Transfer to balance
Request manual cashback
Pending cashback requests
Mark manual cashback as completed
Manual cashback requests
This is not an automatic payout.
New paid invitations will receive reward packages. Historical commission balance can still be handled.
Available commission balance
Pending cashback amount
Withdrawn cashback amount
Transferred to balance
Manual cashback contact
Admin remark
User remark
Cashback status
```

翻译要求：

- `en.json` value 与 key 一致。
- `zh.json` 使用自然中文，例如「邀请奖励模式」「奖励套餐」「邀请返佣」「返现申请」「这不是自动打款」。
- `fr`、`ja`、`ru`、`vi` 使用完整翻译，不保留英文 UI 文案。
- 将侧边栏、表格列或状态映射中无法被 `t('...')` 字面量扫描到的动态 label key 登记到 `static-keys.ts`。
- 运行 `bun run i18n:sync` 后不得留下新增缺失 key。

- [ ] **步骤 7：运行测试验证通过**

```bash
(cd web/default && bun test src/features/invitation-commission/admin-withdrawals.test.ts src/hooks/use-sidebar-config.test.ts)
(cd web/default && bun run i18n:sync)
(cd web/default && bun run typecheck)
```

预期：测试 PASS，i18n 同步成功，typecheck PASS。

---

## 任务 8：端到端回归验证与审查准备

**文件：**
- 不新增生产文件。
- 本任务仅允许为暴露出的回归补充精确测试；不得新增说明性 Markdown。

- [ ] **步骤 1：运行后端聚焦测试**

```bash
go test ./model ./service ./controller -run 'Invitation|Commission|Withdrawal|Subscription|User' -count=1
go test . -run 'TestMainStartsInvitationRewardEventRetryTask' -count=1
go test ./service -run 'TestRetryPendingInvitationRewardEventsCreditsBackfilledLegacySubscriptionAfterModeSwitch' -count=1
```

预期：PASS。若失败，按失败测试回到对应任务补红灯测试或修正实现；不得删除测试或放宽断言掩盖问题。

- [ ] **步骤 2：运行前端测试、类型检查、i18n 同步**

```bash
(cd web/default && bun test)
(cd web/default && bun run typecheck)
(cd web/default && bun run i18n:sync)
```

预期：全部 PASS。若 `bun test` 范围过大且存在与本功能无关的既有失败，必须记录失败用例名称和错误，并单独运行本功能新增/修改测试确认 PASS。

- [ ] **步骤 3：运行 JSON 规则检查**

用仓库搜索确认本功能新增或修改的 Go 业务文件没有直接调用 `encoding/json` marshal/unmarshal。允许仅作为类型引用，例如 `json.RawMessage`；实际序列化必须使用 `common.*`。

需要检查文件：

```text
model/invitation_commission.go
service/invitation_commission.go
controller/invitation_commission.go
controller/admin_tasks.go
controller/subscription_payment_*.go
controller/user.go
controller/redemption.go
model/subscription.go
model/redemption.go
model/user.go
setting/operation_setting/invitation_commission_setting.go
service/invitation_reward.go
model/main.go
model/account_balance.go
```

- [ ] **步骤 4：请求三类只读审查**

并发启动至少 3 个只读子代理：

1. **规格合规审查**：对照 `docs/superpowers/specs/2026-06-03-invitation-commission-spec.md` 和本计划检查所有业务要求是否实现。
2. **后端质量审查**：检查事务、幂等、跨数据库兼容、资金安全、JSON wrapper、并发扣减、销售来源事件与当前模式 fresh 计算。
3. **前端质量审查**：检查路由、权限、query keys、badge 查询条件、i18n、用户文案、钱包交互。

审查者发现 Critical 或 Important 时，必须修复并重新审查对应范围。所有审查通过后才进入最终交付。

---

## 子代理拆分策略

### 实现波次

1. **波次 A：后端核心契约（串行）**
   - 执行任务 1。
   - 原因：新增模型、订单快照字段和常量会被所有后续任务依赖。
2. **波次 B：销售来源事件与奖励套餐（串行或单实现者）**
   - 执行任务 2 和任务 3。
   - 原因：两者共同修改 `model/subscription.go`、`service/invitation_reward.go`，并且销售来源事件区间、`reward_eligible` 与套餐统计强相关，不应并行写同一函数。
3. **波次 C：返佣服务与 API（可在契约明确后拆分）**
   - 任务 4 修改 `service/invitation_commission.go`。
   - 任务 5 修改 Controller/router/user 更新。若任务 4 的函数签名已经稳定，可与任务 5 后半部分并行；否则任务 5 等任务 4 完成。
4. **波次 D：前端用户钱包与管理员页（并行）**
   - 任务 6 和任务 7 可并行，因为文件基本不重叠；共同修改 i18n locale 时由任务 7 主改，任务 6 只列出所需 key 或通过 IRC 告知任务 7。
5. **波次 E：整体验证与审查（并行只读）**
   - 任务 8 的三类审查并行。

### 实现子代理通用禁止事项

- 不创建 worktree。
- 不修改受保护项目标识、品牌、版权或归属信息。
- 不复用 `TopUp` 做返现或提现。
- 不复用 `users.aff_quota` 做返佣账户。
- 不把用户自助划转做成 pending 或管理员审核。
- 不让管理员完成返现增加 `users.quota`。
- 不用直接 `encoding/json` marshal/unmarshal。
- 不新增自动打款、支付出款、多级分销、用户自助开启返佣、用户级返佣比例。
- 不跳过失败测试验证。

---

## 最终验收命令

后端：

```bash
go test ./model ./service ./controller -run 'Invitation|Commission|Withdrawal|Subscription|User' -count=1
go test ./service -run 'TestRetryPendingInvitationRewardEventsCreditsBackfilledLegacySubscriptionAfterModeSwitch' -count=1
go test . -run 'TestMainStartsInvitationRewardEventRetryTask' -count=1
```

前端：

```bash
(cd web/default && bun test)
(cd web/default && bun run typecheck)
(cd web/default && bun run i18n:sync)
```

如果新增或修改的某个测试文件不在 `bun test` 默认范围内，必须单独运行该测试文件并记录 PASS 输出。

---

## 规格覆盖自检

- 默认 `subscription`：任务 1、任务 5、任务 6 覆盖。
- 管理员设置模式：任务 5、任务 6 覆盖。
- 订单金额快照：任务 1 覆盖。
- 销售来源事件不保存奖励模式，只保存来源事实和区间：任务 1、任务 2 覆盖。
- 奖励套餐按当前 `subscription` 模式、直属 active `user_subscriptions`、`subscription_plans.reward_eligible`、非试用、非 `monthly_invite_entitlement`、按 invitee 去重 fresh 计算：任务 3 覆盖。
- 当前 `commission` 模式按同一来源事件 fresh 创建返佣，当前非返佣模式不冻结事件归属；管理员从 `subscription` 切到 `commission` 后，历史 active `legacy_user_subscription` 来源可通过 backfill + retry 补算入账，`reward_eligible = false` 和缺失金额快照的历史来源会安全跳过：任务 3、任务 4、任务 8 覆盖。
- 返佣计算、幂等、非 CNY 跳过：任务 4 覆盖。
- 划转即时完成：任务 4、任务 5、任务 6 覆盖。
- 私聊转账返现 pending 和管理员处理：任务 4、任务 5、任务 7 覆盖。
- 管理员待办摘要和 badge：任务 5、任务 7 覆盖。
- i18n 6 locale：任务 7 覆盖。
- 跨库迁移和唯一索引：任务 1、任务 8 覆盖。
- 历史奖励套餐回填、已有正常来源去重与迁移顺序：任务 3 覆盖。
- 支付后处理失败后的重复回调和后台补偿：任务 2、任务 4 覆盖。
- 后台补偿任务启动注册：任务 4、任务 8 覆盖。
- 全局返佣关闭或 `rate_bps <= 0` 但历史余额仍可处理：任务 4、任务 5、任务 6 覆盖。
- 返佣记录/返现申请 API 字段契约：任务 5、任务 6、任务 7 覆盖。
- JSON wrapper：任务 1、任务 4、任务 8 覆盖。
