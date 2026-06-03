# 账户余额分制迁移实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将账户余额链路从历史 `QuotaPerUnit` 放大单位迁移为 CNY 分，并保证充值、兑换码、奖励、余额购买订阅、账单历史、前端展示和 classic/default 主题不再误用历史倍率。

**架构：** 采用短停机、两阶段迁移状态：数据字段迁移与 `AccountBalanceCentsDataMigrated` 在事务内提交，用户缓存清理和运行时同步成功后才写最终 `AccountBalanceCentsMigrated` 与 `AccountBalanceCentsMigratedAt`。后端新增账户余额分制 helper，账户余额入口统一走分制；非账户余额用量字段保留原语义。新充值订单写入订单级 `amount_unit = account_balance_cents`，账单历史只信任订单级单位标记，前端通过服务端明确展示字段渲染，不再猜测 `top_ups.amount` 单位。前端通过统一余额格式化 helper 展示 CNY 元。

**技术栈：** Go 1.22+、Gin、GORM v2、SQLite/MySQL/PostgreSQL、Redis、React 19 + TypeScript（`web/default`）、React 18 + Semi UI（`web/classic`）、i18next、Bun。

---

## 规格来源

- 设计文档：`docs/superpowers/specs/2026-05-30-account-balance-cents-migration-design.md`
- 关键业务约束：账户余额只用于购买订阅套餐；模型调用、日志统计、API key quota、渠道消耗、订阅 token 用量不纳入本次分制迁移。
- 迁移公式：`新余额分 = round(旧历史 quota × 100 / QuotaPerUnit)`。

## 文件职责总览

### 后端迁移与余额 helper

- `model/account_balance.go`：账户余额分制 helper、金额转换、分制增扣 Tx helper、缓存失效契约。
- `model/account_balance_test.go`：金额转换、分制扣减 / 入账、缓存失效单测。
- `model/account_balance_migration.go`：两阶段迁移状态、数据迁移事务、Option JSON 迁移、pending topup 过期、缓存清理、账单迁移时间标记。
- `model/account_balance_migration_test.go`：三库兼容可用的 SQLite 行为测试，覆盖幂等、重试、范围、旧缓存、Option 写库失败和 batch drain 失败可重试。
- `model/utils.go`：暴露 `BatchUpdatePendingCount(type int) int` 与可重试的 `FlushBatchUpdateTypeForMigration(type int) error`，供迁移前置检查 / 测试使用。
- `model/option.go`：新增检查 DB 错误的 Option upsert helper，迁移标记禁止使用会忽略写库错误的 `UpdateOption`。
- `model/main.go`：保持 `CloseDB()` 现状；短停机迁移不得把普通关闭流程当作 `BatchUpdateTypeUserQuota` 已 flush。
- `main.go`：在 `InitResources()` 中把 `EnsureAccountBalanceCentsMigration()` 放到 `model.InitOptionMap()`、`common.InitRedisClient()` 后，`common.StartSystemMonitor()` 和返回主流程启动后台任务前。

### 后端业务链路

- `controller/subscription_payment_balance.go`：余额购买订阅按 `price_amount × 100` 扣分。
- `controller/topup.go`、`controller/topup_stripe.go`、`controller/topup_waffo.go`、`controller/topup_waffo_pancake.go`、`controller/topup_creem.go`、`controller/topup_kyren.go`：新充值订单 `TopUp.Amount` 使用余额分，写入不可变单位标记 `TopUp.AmountUnit = "account_balance_cents"`，回调 / 补单直接按分入账，不再乘 `QuotaPerUnit`；Creem 控制器业务 JSON 改用 `common.*` 包装。
- `model/topup.go`、`model/main.go`：新增 `TopUp.AmountUnit` 字段和跨三库列迁移；补单、账单历史 DTO / 展示字段、历史成功订单展示换算、pending 过期迁移支持。
- `controller/redemption.go`、`model/redemption.go`：钱包兑换码请求边界为余额分；订阅兑换码不迁移。
- `model/user.go`、`model/checkin.go`、`controller/user.go`、`controller/checkin.go`：注册赠送、邀请奖励、邀请划转、签到奖励、管理员调额改为余额分格式和日志。
- `service/funding_source.go`、`service/task_billing.go`、`service/quota.go`：移除模型调用 / 异步任务 legacy wallet funding 对 `users.quota` 的写入，或显式阻断旧钱包资金来源。

### 后端设置与 i18n / 文案

- `setting/operation_setting/payment_setting.go`：金额选项、折扣 key 语义保持 CNY 元；注释更新为账户余额 CNY 元。
- `setting/operation_setting/checkin_setting.go`：签到奖励配置语义更新为余额分，默认值按 ¥0.20 / 合理分值调整。
- `model/option.go`、`controller/option.go`：迁移状态 Option、主题策略、运行时配置同步；如果禁用 classic，则 `theme.frontend` 只能保存 `default`。
- `i18n/`：如果新增后端错误信息，必须同时补齐 en / zh 后端翻译；否则复用现有错误信息。

### web/default

- `web/default/src/features/subscriptions/lib/subscription-balance.ts`：改为 `accountBalanceCentsToCnyAmount` / `accountBalanceCnyToCents` / `formatAccountBalanceForPlanPurchase`。
- `web/default/src/features/wallet/types.ts`、`api.ts`、`lib/payment.ts`：充值、账单历史、Kyren / Creem 产品类型语义改为余额分 / CNY 元展示。
- `web/default/src/features/wallet/components/wallet-stats-card.tsx`、`recharge-form-card.tsx`、`affiliate-rewards-card.tsx`、`dialogs/payment-confirm-dialog.tsx`、`dialogs/billing-history-dialog.tsx`、`dialogs/transfer-dialog.tsx`、`dialogs/creem-confirm-dialog.tsx`、`creem-products-section.tsx`、`index.tsx`：钱包余额、充值卡、支付确认、账单历史、邀请划转、Kyren / Creem 产品显示。
- `web/default/src/features/profile/components/profile-header.tsx`、`web/default/src/features/profile/components/dialogs/checkin-calendar-card.tsx`：账户余额和签到奖励展示。
- `web/default/src/features/users/components/users-columns.tsx`、`users-mutate-drawer.tsx`、`user-quota-dialog.tsx`：用户列表、用户详情、手动调额按 CNY 元 / 分转换。
- `web/default/src/features/usage-logs/components/dialogs/user-info-dialog.tsx`：`quota` / `aff_quota` / `aff_history_quota` 使用余额分展示，`used_quota` 保留用量格式。
- `web/default/src/features/redemption-codes/lib/redemption-form.ts`、`redemption-form.test.ts`、`redemption-batch.ts`、`components/redemptions-mutate-drawer.tsx`、`components/redemptions-columns.tsx`：钱包兑换码金额输入 CNY 元、提交分。
- `web/default/src/features/system-settings/general/quota-settings-section.tsx`、`checkin-settings-section.tsx`：注册 / 邀请奖励、签到奖励配置按 CNY 元展示和分保存。
- `web/default/src/features/system-settings/integrations/payment-settings-section.tsx`、`amount-options-visual-editor.tsx`、`amount-discount-visual-editor.tsx`、`amount-discount-dialog.tsx`、`waffo-settings-section.tsx`、`waffo-pancake-settings-section.tsx`、`kyren-topup-product-dialog.tsx`、`kyren-topup-products-visual-editor.tsx`、`creem-product-dialog.tsx`、`creem-products-visual-editor.tsx`：普通充值金额选项、折扣、最低充值、Kyren / Creem 档位编辑器按 CNY 元展示。
- `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`：同步新增 / 替换账户余额文案。

### web/classic

本计划选择 **完整改造 classic**，不禁用 classic。理由：当前默认主题是 `classic`，直接禁用会改变用户部署默认行为。

- `web/classic/src/helpers/account-balance.js`（创建）：classic 账户余额分制 helper。
- `web/classic/src/helpers/quota.js`：保留模型用量 quota helper；增加注释禁止账户余额调用。
- `web/classic/src/components/topup/index.jsx`、`RechargeCard.jsx`、`SubscriptionPlansCard.jsx`、`modals/SubscriptionPurchaseModal.jsx`、`InvitationCard.jsx`、`modals/TransferModal.jsx`、`modals/PaymentConfirmModal.jsx`、`modals/TopupHistoryModal.jsx`：钱包余额、普通充值、余额购买订阅、支付确认、账单历史、邀请划转、Creem 展示。
- `web/classic/src/components/table/redemptions/RedemptionsColumnDefs.jsx`、`RedemptionsTable.jsx`、`modals/EditRedemptionModal.jsx`、`web/classic/src/hooks/redemptions/useRedemptionsData.jsx`：兑换码金额按 CNY 元 / 分转换。
- `web/classic/src/components/settings/personal/cards/CheckinCalendar.jsx`、`web/classic/src/components/settings/personal/components/UserInfoHeader.jsx`、`web/classic/src/pages/Setting/Operation/SettingsCheckin.jsx`、`SettingsCreditLimit.jsx`：个人页余额、签到、注册 / 邀请奖励配置。
- `web/classic/src/pages/Setting/Payment/SettingsGeneralPayment.jsx`、`SettingsPaymentGateway.jsx`、`SettingsPaymentGatewayStripe.jsx`、`SettingsPaymentGatewayWaffo.jsx`、`SettingsPaymentGatewayWaffoPancake.jsx`、`SettingsPaymentGatewayCreem.jsx`：充值设置、Stripe / Waffo / Waffo Pancake、Creem 产品配置。
- `web/classic/src/components/table/users/UsersColumnDefs.jsx`、`UsersTable.jsx`、`modals/EditUserModal.jsx`、`web/classic/src/components/table/usage-logs/modals/UserInfoModal.jsx`：账户余额字段分制展示，非账户用量保留 quota helper。
- `web/classic/src/i18n/locales/{en,zh,fr,ja,ru,vi,zh-CN,zh-TW}.json`：同步 classic 文案。

---

## 子代理并行执行边界

本计划允许使用多个子代理并发写入，但必须按以下依赖边界拆分，避免多个子代理同时修改同一文件或基于尚未落地的接口开发：

1. **迁移主干必须串行：任务 1 → 任务 2 → 任务 2A → 任务 3。** 任务 1 提供账户余额 helper；任务 2 提供迁移、`TopUp.AmountUnit`、batch drain helper 和 Option 写入契约；任务 2A 提供旧实例本地 drain 运维入口；任务 3 才能把迁移接入启动流程。不得在任务 2/2A 完成前实现任务 3 启动接入。
2. **充值和账单任务必须等待任务 2 的 `TopUp.AmountUnit` / `model/topup.go` / `model/main.go` 迁移契约落地，然后按文件顺序执行：任务 5 → 任务 6。** `model/topup.go`、`controller/topup.go`、`controller/topup_*` 由任务 5 先完成新订单分制和成功入账；任务 6 在此基础上新增历史 DTO 和前端账单展示，不得与任务 5 并行修改同一控制器返回结构。
3. **订阅余额支付任务 4 可在任务 1 后独立执行。** 它依赖 `AccountBalanceCentsFromCNY`，不修改 `topup` 文件；若任务 1 未完成，实现子代理必须先等待或只写红测。
4. **奖励 / 兑换码 / 管理调额任务 7 可在任务 1 和任务 2 的 helper / migration contract 稳定后执行。** 它修改 `model/user.go`、`model/checkin.go`、`controller/user.go` 等，不得与任务 8 同时修改同一旧 quota helper；若并发，任务 7 负责账户余额业务入口，任务 8 只负责非账户用量路径阻断。
5. **legacy 用量收口任务 8 可与前端任务并行，但不得与任务 7 争抢 `service/quota.go` 以外的账户余额入口。** 任务 8 的输出是 `ErrLegacyWalletFundingDisabled` 和静态扫描分类，后续最终 gate 统一验证。
6. **web/default 任务按 helper → 管理端顺序：任务 9 → 任务 10。** 任务 9 提供 `subscription-balance.ts` 账户余额 helper 和钱包/订阅展示；任务 10 复用该 helper 改管理端、兑换码、签到和产品编辑器。`web/default/src/features/wallet` 同时出现在任务 9/10 时，以任务 9 为主改，任务 10 只补管理端依赖或测试。
7. **账单历史前端依赖任务 6 的 DTO。** `billing-history-dialog.tsx` 和 classic `TopupHistoryModal.jsx` 在任务 6 中主改；classic 账单历史行为测试归任务 11 的 `account-balance.test.js`，只消费任务 6 提供的 `credited_balance_display` / `credited_balance_cents` contract；任务 9/10/11 不得重新定义账单历史字段。
8. **web/classic 任务 11 可在任务 6 DTO contract 明确后执行组件和 helper 改造；任务 12 在任务 11 组件文案稳定后补 locale 与 i18n 测试。** 任务 11 的独立验收只运行 `account-balance.test.js` 和目标 eslint；`account-balance-i18n.test.js` 归任务 12 创建和验证，避免任务 11 子代理跨任务修改 i18n 测试文件。
9. **任务 12A 必须在任务 13 前完成。** 任务 12A 只修改回滚 runbook / 文档验收，不得与任务 13 并行；任务 13 的静态扫描必须检查 12A 的回滚关键词。
10. **任务 13 只能最后执行。** 它覆盖任务 1–12A 的全部修改，是全量集成验证和静态扫描门禁，不能与任何写入任务并行。

如果实际派发时需要并发写入，子代理提示词必须明确所属任务、允许修改文件、禁止触碰文件和依赖的接口签名；遇到共享文件冲突时，以本节指定的主改任务为准。

---

### 任务 1：账户余额分制 helper 与基础扣减

**文件：**
- 修改：`model/account_balance.go`
- 测试：`model/account_balance_test.go`

- [ ] **步骤 1：编写金额转换和分制扣减失败测试**

在 `model/account_balance_test.go` 中新增测试：

```go
func TestAccountBalanceCentsFromCNY(t *testing.T) {
	cases := []struct {
		name    string
		amount  string
		want    int
		wantErr bool
	}{
		{name: "forty yuan", amount: "40", want: 4000},
		{name: "thirty nine point nine", amount: "39.9", want: 3990},
		{name: "round half up to cents", amount: "0.015", want: 2},
		{name: "reject zero", amount: "0", wantErr: true},
		{name: "reject negative", amount: "-1", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			amount, err := decimal.NewFromString(tc.amount)
			require.NoError(t, err)
			got, err := AccountBalanceCentsFromCNY(amount)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.True(t, AccountBalanceCNYFromCents(got).Equal(decimal.NewFromInt(int64(got)).Div(decimal.NewFromInt(100))))
		})
	}
}

func TestDeductAndIncreaseUserAccountBalanceTxUseCents(t *testing.T) {
	setupAccountBalanceTestDB(t)
	user := &User{Id: 9101, Username: "balance-cents", Quota: 4000, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return DeductUserAccountBalanceTx(tx, user.Id, 3990)
	}))
	assert.Equal(t, 10, getUserQuotaForAccountBalanceTest(t, user.Id))

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return IncreaseUserAccountBalanceTx(tx, user.Id, 250)
	}))
	assert.Equal(t, 260, getUserQuotaForAccountBalanceTest(t, user.Id))

	err := DB.Transaction(func(tx *gorm.DB) error {
		return DeductUserAccountBalanceTx(tx, user.Id, 261)
	})
	require.Error(t, err)
	assert.Equal(t, 260, getUserQuotaForAccountBalanceTest(t, user.Id))
}

func TestAccountBalanceTxHelpersInvalidateUserCache(t *testing.T) {
	setupAccountBalanceTestDB(t)
	user := &User{Id: 9102, Username: "cache-balance", Quota: 4000, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	seedUserCacheForMigrationTest(t, &UserBase{Id: user.Id, Username: user.Username, Quota: 4000, Status: common.UserStatusEnabled})

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return IncreaseUserAccountBalanceTx(tx, user.Id, 250)
	}))
	require.NoError(t, InvalidateUserCache(user.Id))

	cache, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 4250, cache.Quota)
}

func TestAccountBalanceTxInvalidatesAfterCommitOnly(t *testing.T) {
	setupAccountBalanceTestDB(t)
	user := &User{Id: 9103, Username: "cache-race", Quota: 4000, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	seedUserCacheForMigrationTest(t, &UserBase{Id: user.Id, Username: user.Username, Quota: 4000, Status: common.UserStatusEnabled})
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		err := IncreaseUserAccountBalanceTx(tx, user.Id, 250)
		cache, cacheErr := GetUserCache(user.Id)
		require.NoError(t, cacheErr)
		assert.Equal(t, 4000, cache.Quota)
		return err
	}))
	require.NoError(t, InvalidateUserCache(user.Id))
	cache, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 4250, cache.Quota)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
go test ./model -run 'TestAccountBalanceCentsFromCNY|TestDeductAndIncreaseUserAccountBalanceTxUseCents|TestAccountBalanceTxHelpersInvalidateUserCache|TestAccountBalanceTxInvalidatesAfterCommitOnly' -count=1
```

预期：FAIL，至少包含 `undefined: AccountBalanceCentsFromCNY` 或 `undefined: IncreaseUserAccountBalanceTx`。

- [ ] **步骤 3：实现 helper**

在 `model/account_balance.go` 中实现：

```go
func AccountBalanceCentsFromCNY(amount decimal.Decimal) (int, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("invalid amount")
	}
	cents := amount.Mul(decimal.NewFromInt(100)).Round(0)
	if cents.LessThanOrEqual(decimal.Zero) || cents.GreaterThan(decimal.NewFromInt(int64(math.MaxInt))) {
		return errors.New("invalid amount")
	}
	return int(cents.IntPart()), nil
}

func AccountBalanceCNYFromCents(cents int) decimal.Decimal {
	return decimal.NewFromInt(int64(cents)).Div(decimal.NewFromInt(100))
}

func IncreaseUserAccountBalanceTx(tx *gorm.DB, userId int, cents int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if userId <= 0 {
		return errors.New("invalid user id")
	}
	if cents <= 0 {
		return errors.New("invalid amount")
	}
	if err := tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", cents)).Error; err != nil {
		return err
	}
	return nil
}
```

保留 `DeductUserAccountBalanceTx`，但把参数名改为 `cents`，错误信息保持兼容；`DeductUserAccountBalanceTx` 与 `IncreaseUserAccountBalanceTx` 的公开签名保持 `error`，事务内只更新数据库。调用方必须在 `DB.Transaction` 成功返回后统一调用 `InvalidateUserCache(userId)`，或使用 `AccountBalanceTxCacheInvalidator` 这类 after-commit 收集器记录 userId 并在事务提交后失效；不得在事务提交前删除缓存，避免并发请求在提交前把旧余额回填到 Redis。后续充值、补单、兑换码、签到、邀请奖励、管理员调额不得直接使用裸 `Update("quota", ...)` 作为完整实现，除非同事务成功后显式执行 after-commit 缓存失效并有测试覆盖。

- [ ] **步骤 4：运行测试验证通过**

运行：

```bash
go test ./model -run 'TestAccountBalanceCentsFromCNY|TestDeductAndIncreaseUserAccountBalanceTxUseCents|TestAccountBalanceTxHelpersInvalidateUserCache|TestAccountBalanceTxInvalidatesAfterCommitOnly' -count=1
```

预期：PASS。

- [ ] **步骤 5：提交**

```bash
git add model/account_balance.go model/account_balance_test.go
git commit -m "feat(balance): 新增账户余额分制工具"
```

### 任务 2：两阶段账户余额迁移与前置 drain 检查

**文件：**
- 创建：`model/account_balance_migration.go`
- 修改：`model/utils.go`
- 修改：`model/option.go`
- 修改：`model/topup.go`
- 修改：`model/main.go`
- 测试：`model/account_balance_migration_test.go`

- [ ] **步骤 1：编写迁移红测**

在 `model/account_balance_migration_test.go` 中新增测试，覆盖：

```go
func TestEnsureAccountBalanceCentsMigrationConvertsAccountBalanceFields(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	seedRuntimeBalanceOptionsForMigrationTest(t, map[string]string{
		"QuotaForNewUser":  "1000000",
		"QuotaForInviter":  "500000",
		"QuotaForInvitee":  "250000",
		"checkin_setting":  `{"enabled":true,"min_quota":5000,"max_quota":10000}`,
		"KyrenTopUpProducts": `[{"id":"topup_40","name":"40 CNY","amount":"40.00","currency":"CNY","quota":20000000,"enabled":true}]`,
		"CreemProducts": `[{"name":"Creem 40","productId":"prod_40","price":40,"quota":20000000,"currency":"USD"}]`,
	})
	require.NoError(t, DB.Create(&User{Id: 9201, Username: "migrate", Quota: 1000000, AffQuota: 250000, AffHistoryQuota: 997500}).Error)
	require.NoError(t, DB.Create(&Redemption{Id: 9202, Name: "wallet", Type: RedemptionTypeWallet, Quota: 1000000}).Error)
	require.NoError(t, DB.Create(&Redemption{Id: 9203, Name: "sub", Type: RedemptionTypeSubscription, Quota: 1000000, PlanId: 1}).Error)
	require.NoError(t, DB.Create(&Redemption{Id: 9206, Name: "blank-wallet", Type: "", Quota: 1000000}).Error)
	require.NoError(t, DB.Create(&Checkin{UserId: 9201, CheckinDate: "2026-05-30", QuotaAwarded: 5000}).Error)
	require.NoError(t, DB.Create(&TopUp{Id: 9204, UserId: 9201, Amount: 1000000, TradeNo: "pending-old", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending}).Error)
	require.NoError(t, DB.Create(&TopUp{Id: 9205, UserId: 9201, Amount: 1000000, TradeNo: "success-old", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess}).Error)

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 400, getUserQuotaForAccountBalanceTest(t, 9201))
	assert.Equal(t, 100, getUserAffQuotaForMigrationTest(t, 9201))
	assert.Equal(t, 399, getUserAffHistoryForMigrationTest(t, 9201))
	assert.Equal(t, 400, getRedemptionQuotaForMigrationTest(t, 9202))
	assert.Equal(t, 1000000, getRedemptionQuotaForMigrationTest(t, 9203))
	assert.Equal(t, 400, getRedemptionQuotaForMigrationTest(t, 9206))
	assert.Equal(t, RedemptionTypeWallet, getRedemptionTypeForMigrationTest(t, 9206))
	assert.Equal(t, 2, getCheckinQuotaForMigrationTest(t, 9201, "2026-05-30"))
	assert.Equal(t, common.TopUpStatusExpired, getTopUpStatusForMigrationTest(t, "pending-old"))
	assert.EqualValues(t, 1000000, getTopUpAmountForMigrationTest(t, "success-old"))
	assert.Equal(t, "", getTopUpAmountUnitForMigrationTest(t, "success-old"))
	assert.Equal(t, 8000, getKyrenTopUpProductQuotaForMigrationTest(t, "topup_40"))
	assert.Equal(t, 8000, getCreemProductQuotaForMigrationTest(t, "prod_40"))
	assert.Equal(t, 400, common.QuotaForNewUser)
	assert.Equal(t, 200, common.QuotaForInviter)
	assert.Equal(t, 100, common.QuotaForInvitee)
	assert.Equal(t, 2, operation_setting.GetCheckinSetting().MinQuota)
	assert.Equal(t, 4, operation_setting.GetCheckinSetting().MaxQuota)
	assert.Contains(t, common.OptionMap["KyrenTopUpProducts"], `"quota":8000`)
	assert.Contains(t, common.OptionMap["CreemProducts"], `"quota":8000`)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, "AccountBalanceCentsDataMigrated"))
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, "AccountBalanceCentsMigrated"))
	assert.NotEmpty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsMigratedAt"))
}

func TestEnsureAccountBalanceCentsMigrationUsesLoadedQuotaPerUnit(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	common.QuotaPerUnit = 500000
	require.NoError(t, DB.Create(&User{Id: 9215, Username: "runtime-rate", Quota: 1000000}).Error)

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 400, getUserQuotaForAccountBalanceTest(t, 9215))
}

func TestEnsureAccountBalanceCentsMigrationRejectsInvalidQuotaPerUnit(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		t.Run(value, func(t *testing.T) {
			setupAccountBalanceMigrationTestDB(t)
			setOptionForMigrationTest(t, "QuotaPerUnit", value)
			require.NoError(t, DB.Create(&User{Id: 9216, Username: "invalid-qpu", Quota: 1000000}).Error)

			err := EnsureAccountBalanceCentsMigration()

			require.Error(t, err)
			assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsDataMigrated"))
			assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsMigrated"))
			assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsMigratedAt"))
		})
	}
}

func TestEnsureAccountBalanceCentsMigrationLeavesNonAccountQuotaFieldsUnchanged(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	seedNonAccountQuotaFieldsForMigrationTest(t, nonAccountQuotaSeed{
		LogQuota: 1000000, TokenRemainQuota: 1000000, TokenUsedQuota: 500000,
		ChannelUsedQuota: 750000, AbilityQuota: 1000000,
		UserSubscriptionTokenLimit: 1000000, UserSubscriptionTokenUsed: 100,
		SubscriptionPlanMonthlyTokenLimit: 1000000, TopUpMoney: 40,
	})

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assertNonAccountQuotaFieldsUnchanged(t, nonAccountQuotaSeed{
		LogQuota: 1000000, TokenRemainQuota: 1000000, TokenUsedQuota: 500000,
		ChannelUsedQuota: 750000, AbilityQuota: 1000000,
		UserSubscriptionTokenLimit: 1000000, UserSubscriptionTokenUsed: 100,
		SubscriptionPlanMonthlyTokenLimit: 1000000, TopUpMoney: 40,
	})
}

func TestEnsureAccountBalanceCentsMigrationIsIdempotentAfterDataStage(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	setOptionForMigrationTest(t, "AccountBalanceCentsDataMigrated", "true")
	require.NoError(t, DB.Create(&User{Id: 9210, Username: "already-data", Quota: 4000}).Error)

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 4000, getUserQuotaForAccountBalanceTest(t, 9210))
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, "AccountBalanceCentsMigrated"))
}

func TestEnsureAccountBalanceCentsMigrationDataStageRetryReloadsRuntimeOptions(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	setOptionForMigrationTest(t, "AccountBalanceCentsDataMigrated", "true")
	setOptionForMigrationTest(t, "QuotaForNewUser", "400")
	setOptionForMigrationTest(t, "QuotaForInviter", "200")
	setOptionForMigrationTest(t, "QuotaForInvitee", "100")
	setOptionForMigrationTest(t, "checkin_setting", `{"enabled":true,"min_quota":2,"max_quota":4}`)
	setOptionForMigrationTest(t, "KyrenTopUpProducts", `[{"id":"topup_40","name":"40 CNY","amount":"40.00","currency":"CNY","quota":8000,"enabled":true}]`)
	setOptionForMigrationTest(t, "CreemProducts", `[{"name":"Creem 40","productId":"prod_40","price":40,"quota":8000,"currency":"USD"}]`)
	common.OptionMap["QuotaForNewUser"] = "1000000"
	common.OptionMap["QuotaForInviter"] = "500000"
	common.OptionMap["QuotaForInvitee"] = "250000"
	common.OptionMap["checkin_setting"] = `{"enabled":true,"min_quota":5000,"max_quota":10000}`
	common.OptionMap["KyrenTopUpProducts"] = `[{"id":"topup_40","quota":20000000}]`
	common.OptionMap["CreemProducts"] = `[{"productId":"prod_40","quota":20000000}]`
	common.QuotaForNewUser = 1000000
	common.QuotaForInviter = 500000
	common.QuotaForInvitee = 250000
	operation_setting.GetCheckinSetting().MinQuota = 5000
	operation_setting.GetCheckinSetting().MaxQuota = 10000
	setting.KyrenTopUpProducts = common.OptionMap["KyrenTopUpProducts"]
	setting.CreemProducts = common.OptionMap["CreemProducts"]

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	assert.Equal(t, 400, common.QuotaForNewUser)
	assert.Equal(t, 200, common.QuotaForInviter)
	assert.Equal(t, 100, common.QuotaForInvitee)
	assert.Equal(t, 2, operation_setting.GetCheckinSetting().MinQuota)
	assert.Equal(t, 4, operation_setting.GetCheckinSetting().MaxQuota)
	assert.Equal(t, "400", common.OptionMap["QuotaForNewUser"])
	assert.Equal(t, "200", common.OptionMap["QuotaForInviter"])
	assert.Equal(t, "100", common.OptionMap["QuotaForInvitee"])
	assert.Contains(t, common.OptionMap["checkin_setting"], `"min_quota":2`)
	assert.Contains(t, common.OptionMap["checkin_setting"], `"max_quota":4`)
	assert.Contains(t, common.OptionMap["KyrenTopUpProducts"], `"quota":8000`)
	assert.Contains(t, common.OptionMap["CreemProducts"], `"quota":8000`)
	assert.Contains(t, setting.KyrenTopUpProducts, `"quota":8000`)
	assert.Contains(t, setting.CreemProducts, `"quota":8000`)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, "AccountBalanceCentsMigrated"))
}

func TestEnsureAccountBalanceCentsMigrationDataStageRetryDoesNotFinalizeWhenRuntimeSyncFails(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "AccountBalanceCentsDataMigrated", "true")
	setOptionForMigrationTest(t, "KyrenTopUpProducts", `not-json`)

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsMigrated"))
	assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsMigratedAt"))
}

func TestEnsureAccountBalanceCentsMigrationRejectsPendingUserQuotaBatch(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	addNewRecord(BatchUpdateTypeUserQuota, 9220, 500000)

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsDataMigrated"))
	clearBatchUpdateTypeForMigrationTest(t, BatchUpdateTypeUserQuota)
}

func TestFlushBatchUpdateTypeForMigrationKeepsPendingOnFailure(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	addNewRecord(BatchUpdateTypeUserQuota, 9299, 500)

	err := FlushBatchUpdateTypeForMigration(BatchUpdateTypeUserQuota)

	require.Error(t, err)
	assert.Equal(t, 1, BatchUpdatePendingCount(BatchUpdateTypeUserQuota))
	require.NoError(t, DB.Create(&User{Id: 9299, Username: "flush-retry", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, FlushBatchUpdateTypeForMigration(BatchUpdateTypeUserQuota))
	assert.Equal(t, 0, BatchUpdatePendingCount(BatchUpdateTypeUserQuota))
	assert.Equal(t, 500, getUserQuotaForAccountBalanceTest(t, 9299))
}

func TestFlushBatchUpdateTypeForMigrationPreservesConcurrentDelta(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 9300, Username: "flush-race", Status: common.UserStatusEnabled}).Error)
	addNewRecord(BatchUpdateTypeUserQuota, 9300, 500)
	setMigrationFlushAfterSwapHookForTest(func() {
		addNewRecord(BatchUpdateTypeUserQuota, 9300, 700)
	})
	require.NoError(t, FlushBatchUpdateTypeForMigration(BatchUpdateTypeUserQuota))
	assert.Equal(t, 700, pendingBatchDeltaForMigrationTest(BatchUpdateTypeUserQuota, 9300))
	assert.Equal(t, 500, getUserQuotaForAccountBalanceTest(t, 9300))
	require.NoError(t, FlushBatchUpdateTypeForMigration(BatchUpdateTypeUserQuota))
	assert.Equal(t, 0, BatchUpdatePendingCount(BatchUpdateTypeUserQuota))
	assert.Equal(t, 1200, getUserQuotaForAccountBalanceTest(t, 9300))
}

func TestEnsureAccountBalanceCentsMigrationFailsWhenFinalOptionWriteFails(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	setOptionForMigrationTest(t, "AccountBalanceCentsDataMigrated", "true")
	closeAccountBalanceMigrationDBForTest(t)

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.NotEqual(t, "true", common.OptionMap["AccountBalanceCentsMigrated"])
}

func TestEnsureAccountBalanceCentsMigrationFailsWhenUserCacheClearFails(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	require.NoError(t, DB.Create(&User{Id: 9240, Username: "cache-fail", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	forceInvalidateAllUserCacheErrorForMigrationTest(errors.New("redis delete failed"))

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, "AccountBalanceCentsDataMigrated"))
	assert.NotEqual(t, "true", getOptionValueForMigrationTest(t, "AccountBalanceCentsMigrated"))
	assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsMigratedAt"))
	forceInvalidateAllUserCacheErrorForMigrationTest(nil)
	require.NoError(t, EnsureAccountBalanceCentsMigration())
	assert.Equal(t, 400, getUserQuotaForAccountBalanceTest(t, 9240))
}

func TestEnsureAccountBalanceCentsMigrationRollbackDoesNotChangeRuntimeOptions(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	seedRuntimeBalanceOptionsForMigrationTest(t, map[string]string{"KyrenTopUpProducts": `[{"id":"topup_40","quota":1000000}]`})
	oldRuntime := setting.KyrenTopUpProducts
	forceAccountBalanceDataMigrationErrorForTest(errors.New("stop before commit"))

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsDataMigrated"))
	assert.Equal(t, oldRuntime, setting.KyrenTopUpProducts)
	assert.Equal(t, oldRuntime, common.OptionMap["KyrenTopUpProducts"])
}
```

- [ ] **步骤 2：运行测试验证失败**

```bash
go test ./model -run 'TestEnsureAccountBalanceCentsMigration|TestFlushBatchUpdateTypeForMigration|TestTopUpAmountUnitColumnAutoMigrateSQLite' -count=1
```

预期：FAIL，包含 `undefined: EnsureAccountBalanceCentsMigration`。

- [ ] **步骤 3：实现可重试 batch drain 和检查型 Option 写入**

在 `model/utils.go` 增加：

```go
func BatchUpdatePendingCount(type_ int) int {
	if type_ < 0 || type_ >= BatchUpdateTypeCount {
		return 0
	}
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	return len(batchUpdateStores[type_])
}

func FlushBatchUpdateTypeForMigration(type_ int) error {
	if type_ != BatchUpdateTypeUserQuota {
		return errors.New("unsupported migration batch update type")
	}
	batchUpdateLocks[type_].Lock()
	snapshot := batchUpdateStores[type_]
	batchUpdateStores[type_] = make(map[int]int)
	batchUpdateLocks[type_].Unlock()
	migrationFlushAfterSwapHookForTest()

	flushed := make(map[int]struct{}, len(snapshot))
	for key, value := range snapshot {
		if err := increaseUserQuota(key, value); err != nil {
			batchUpdateLocks[type_].Lock()
			for pendingKey, pendingValue := range snapshot {
				if _, ok := flushed[pendingKey]; ok {
					continue
				}
				batchUpdateStores[type_][pendingKey] += pendingValue
			}
			batchUpdateLocks[type_].Unlock()
			return err
		}
		flushed[key] = struct{}{}
		// 新增 delta 已进入新的 batchUpdateStores[type_]，flush 结束不能删除或抵消这些新增 delta。
	}
	return nil
}
```

实现要点：flush 必须在锁内原子 swap 出当前 map，使 flush 期间新进入的 delta 写到新的 map；失败时只把尚未成功写库的 snapshot 项合并回当前 map，不能覆盖 flush 期间新增 delta。运维入口调用前仍必须停入口和异步触发源，避免持续新增；该实现用于保证极少量 in-flight 请求或测试 hook 下的新增 delta 不丢失。

在 `model/option.go` 新增检查 DB 错误的 helper；迁移标记和迁移事务内 JSON Option 只使用该 helper，不使用当前会忽略 `FirstOrCreate` / `Save` 错误的 `UpdateOption`：

```go
func UpdateOptionChecked(key string, value string) error {
	option := Option{Key: key}
	if err := DB.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
		return err
	}
	option.Value = value
	if err := DB.Save(&option).Error; err != nil {
		return err
	}
	return updateOptionMap(key, value)
}

func upsertOptionTx(tx *gorm.DB, key string, value string) error {
	return tx.Save(&Option{Key: key, Value: value}).Error
}

func syncMigratedOptionsRuntime(values map[string]string) error {
	for key, value := range values {
		if err := updateOptionMap(key, value); err != nil {
			return err
		}
	}
	return nil
}
```

在 `model/account_balance_migration.go` 实现常量和入口：

```go
const (
	OptionAccountBalanceCentsDataMigrated = "AccountBalanceCentsDataMigrated"
	OptionAccountBalanceCentsMigrated     = "AccountBalanceCentsMigrated"
	OptionAccountBalanceCentsMigratedAt   = "AccountBalanceCentsMigratedAt"
)

func EnsureAccountBalanceCentsMigration() error {
	if strings.EqualFold(strings.TrimSpace(common.OptionMap[OptionAccountBalanceCentsMigrated]), "true") {
		return nil
	}
	if BatchUpdatePendingCount(BatchUpdateTypeUserQuota) > 0 {
		return errors.New("pending user quota batch updates must be flushed on every old instance before account balance migration")
	}
	// 运维必须在每个旧实例本地完成 flush/drain；这里的检查只能防止当前进程仍有待刷余额。
	quotaPerUnit, err := accountBalanceMigrationQuotaPerUnit()
	if err != nil {
		return err
	}
	if quotaPerUnit.LessThanOrEqual(decimal.Zero) {
		return errors.New("invalid QuotaPerUnit for account balance migration")
	}
	var pendingRuntimeSync map[string]string
	if !strings.EqualFold(strings.TrimSpace(common.OptionMap[OptionAccountBalanceCentsDataMigrated]), "true") {
		var err error
		pendingRuntimeSync, err = migrateAccountBalanceData(quotaPerUnit)
		if err != nil {
			return err
		}
	} else {
		pendingRuntimeSync = loadAccountBalanceMigratedOptionValuesFromDB()
	}
	if err := syncMigratedOptionsRuntime(pendingRuntimeSync); err != nil {
		return err
	}
	if err := invalidateAllUserCachesForAccountBalanceMigration(); err != nil {
		return err
	}
	migratedAt := strconv.FormatInt(common.GetTimestamp(), 10)
	if err := UpdateOptionChecked(OptionAccountBalanceCentsMigratedAt, migratedAt); err != nil {
		return err
	}
	return UpdateOptionChecked(OptionAccountBalanceCentsMigrated, "true")
}
```

data-stage-only 重试必须从 DB Option 重新加载已迁移的 runtime 配置并同步到 `common.OptionMap`、`common.QuotaForNewUser` / `QuotaForInviter` / `QuotaForInvitee`、`setting.KyrenTopUpProducts`、`setting.CreemProducts`、`operation_setting.CheckinSetting`；如果 DB 读取、JSON 解析或 `updateOptionMap` / runtime sync 任一步失败，不得写 `AccountBalanceCentsMigratedAt` 或 `AccountBalanceCentsMigrated`。

`migrateAccountBalanceData` 必须使用 `DB.Transaction`，在事务内逐行迁移 `users`、钱包 `redemptions`（`type = wallet` 以及空类型历史钱包码，空类型迁移后归一化为 wallet）、`checkins`、`KyrenTopUpProducts`、`CreemProducts`、`QuotaForNewUser` / `QuotaForInviter` / `QuotaForInvitee`、`checkin_setting.min_quota` / `max_quota`，并将 pending `top_ups` 标记 `expired`。事务内只写数据库 Option 行并收集待同步值，不得在事务提交前修改 `common.OptionMap` 或运行时变量。数据迁移事务提交成功后，再用已提交的 DB Option 值同步 `common.OptionMap` 和运行时设置变量（例如 `setting.KyrenTopUpProducts`、`setting.CreemProducts`、`common.QuotaForNewUser`、`operation_setting.GetCheckinSetting()`）；任一运行时同步失败都不得写最终 marker。若事务中途失败或回滚，OptionMap/runtime 必须保持旧值。数据迁移不得修改非账户余额字段：`logs.quota`、`tokens.remain_quota`、`tokens.used_quota`、`channels.used_quota`、`abilities.quota`、`user_subscriptions.token_limit`、`user_subscriptions.token_used`、`subscription_plans.monthly_token_limit`、历史成功 `top_ups.amount` 和 `top_ups.money` 必须保持原值。数据迁移不得给历史成功订单回填 `amount_unit`；迁移前成功订单保持空单位，由任务 6 的历史展示 fallback 解释。事务最后用 `upsertOptionTx(tx, OptionAccountBalanceCentsDataMigrated, "true")` 写入数据阶段标记。

迁移必须记录结构化审计日志，可通过 `logAccountBalanceMigrationStats(stats)` 实现并在测试中捕获。日志字段至少包含：`quota_per_unit` 与来源、每类更新数量（users、aff_quota、aff_history、wallet redemptions、blank wallet redemptions、checkins、KyrenProducts、CreemProducts、runtime options）、pending top-up 过期数量、签到配置迁移及舍入为 0 的数量、历史成功 `top_ups.amount` / `top_ups.money` skipped 数、data/final/time marker 写入状态、用户缓存清理方式、数量、失败原因和 Redis disabled skip 原因。


- [ ] **步骤 3A：新增 TopUp 金额单位字段迁移**

在 `model/topup.go` 增加：

```go
const TopUpAmountUnitAccountBalanceCents = "account_balance_cents"

type TopUp struct {
	Id         int    `json:"id"`
	Amount     int64  `json:"amount"`
	AmountUnit string `json:"amount_unit" gorm:"size:32;default:''"`
}
```

在 `model/main.go` 的 AutoMigrate 与 SQLite 兼容路径确保 `top_ups.amount_unit` 自动补列；SQLite 使用 `ALTER TABLE ... ADD COLUMN`，MySQL / PostgreSQL 依赖 GORM 等价兼容逻辑。编写 `TestTopUpAmountUnitColumnAutoMigrateSQLite`，断言 SQLite AutoMigrate 后存在 `amount_unit`，并且旧记录空值可读、新订单可写 `account_balance_cents`。
- [ ] **步骤 4：实现 JSON Option 迁移**

在 `model/account_balance_migration.go` 中使用 `common.UnmarshalJsonStr` / `common.Marshal`：

```go
func migrateKyrenTopUpProductsOptionTx(tx *gorm.DB, quotaPerUnit decimal.Decimal) error {
	value := common.OptionMap["KyrenTopUpProducts"]
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var products []map[string]any
	if err := common.UnmarshalJsonStr(value, &products); err != nil {
		return err
	}
	for i := range products {
		quota, ok := numberFromOptionProduct(products[i]["quota"])
		if !ok {
			continue
		}
		products[i]["quota"] = legacyQuotaToCents(quota, quotaPerUnit)
	}
	encoded, err := common.Marshal(products)
	if err != nil {
		return err
	}
	return upsertOptionTx(tx, "KyrenTopUpProducts", string(encoded))
}
```

对 `CreemProducts` 同样处理 `quota` / `Quota`。不要引入 `encoding/json` marshal/unmarshal。

- [ ] **步骤 5：运行迁移测试验证通过**

```bash
go test ./model -run 'TestEnsureAccountBalanceCentsMigration|TestFlushBatchUpdateTypeForMigration|TestTopUpAmountUnitColumnAutoMigrateSQLite|TestAccountBalanceCentsFromCNY' -count=1
```

预期：PASS。

- [ ] **步骤 6：提交**

```bash
git add model/account_balance_migration.go model/account_balance_migration_test.go model/utils.go model/account_balance.go model/option.go model/topup.go model/main.go
git commit -m "feat(balance): 增加账户余额分制迁移"
```


---

### 任务 2A：短停机旧实例 drain 运维入口与手册

**文件：**
- 修改：`controller/loadtest_runtime.go`
- 测试：`controller/loadtest_runtime_test.go`
- 修改：`docs/superpowers/specs/2026-05-30-account-balance-cents-migration-design.md`

- [ ] **步骤 1：编写旧实例本地 drain 红测**

在 `controller/loadtest_runtime_test.go` 增加仅 loopback 可调用的 drain 测试。该入口复用已有 `LOADTEST_RUNTIME_STATS_ENABLED=true` + loopback 约束，不对公网开放：

发布边界：如果线上旧版本没有本地 drain 入口，必须在完成任务 2A 后先发布一次「仅含任务 1、任务 2、任务 2A，尚未包含任务 3 启动迁移接入」的预迁移版本到每个旧实例。该预迁移版本提供本地 drain 入口，但不会在启动时执行 `EnsureAccountBalanceCentsMigration()`。完成逐实例 drain 和数据库备份后，才能发布包含任务 3 及后续改造的迁移版本。

```go
func TestLoadtestRuntimeDrainUserQuotaBatchFlushesLocalPending(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	setupLoadtestRuntimeModelDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9320, Username: "runtime-drain", Status: common.UserStatusEnabled}).Error)
	model.AddUserQuotaBatchForTest(9320, 700)
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/debug/loadtest/runtime/batch-update/user-quota/drain", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, model.BatchUpdatePendingSnapshot().ByType[model.BatchUpdateTypeUserQuota])
	assert.Equal(t, 700, model.GetUserQuotaForTest(t, 9320))
}

func TestLoadtestRuntimeDrainUserQuotaBatchReportsPendingWhenFlushFails(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	setupLoadtestRuntimeModelDB(t)
	model.AddUserQuotaBatchForTest(9399, 700)
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/debug/loadtest/runtime/batch-update/user-quota/drain", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, 1, model.BatchUpdatePendingSnapshot().ByType[model.BatchUpdateTypeUserQuota])
}
```

- [ ] **步骤 2：运行测试验证失败**

```bash
go test ./controller -run 'TestLoadtestRuntimeDrainUserQuotaBatch' -count=1
```

预期：FAIL，返回 `404` 或缺少测试 helper。

- [ ] **步骤 3：实现旧实例本地 drain 入口**

在 `RegisterLoadtestRuntimeRoute` 内新增本地 POST 入口：

```go
r.POST("/debug/loadtest/runtime/batch-update/user-quota/drain", func(c *gin.Context) {
	if !remoteAddrIsLoopback(c.Request.RemoteAddr) || !forwardedClientIsLoopback(c.Request) {
		c.Status(http.StatusForbidden)
		return
	}
	before := model.BatchUpdatePendingSnapshot()
	err := model.FlushBatchUpdateTypeForMigration(model.BatchUpdateTypeUserQuota)
	after := model.BatchUpdatePendingSnapshot()
	status := http.StatusOK
	if err != nil || after.ByType[model.BatchUpdateTypeUserQuota] != 0 {
		status = http.StatusInternalServerError
	}
	c.JSON(status, gin.H{
		"flushed_type": "user_quota",
		"before":       before,
		"after":        after,
		"pending":      after.ByType[model.BatchUpdateTypeUserQuota],
		"error":        errorString(err),
		"note":         "Stop ingress before calling drain; this endpoint only flushes this local instance.",
	})
})
```

`errorString(err)` 是当前文件内的小 helper；`err == nil` 时返回空字符串。不要绕过已有 loopback / env gate。


- [ ] **步骤 4：把短停机运维顺序写入规格**

在规格文档「迁移策略」后补充可执行顺序：

```text
1. 停止入口流量和异步任务触发源，不再接收新充值、兑换码、签到、注册、邀请、模型调用请求。
2. 将每个旧实例切到本地 drain 状态：公网入口已断开，队列消费者 / 定时任务 / 异步触发源已停止，除本机 loopback drain 请求外不再有新请求进入会调用 `addNewRecord(BatchUpdateTypeUserQuota, delta)` 的路径。
3. 保持每个旧实例进程运行，逐个在对应机器本地执行：
   curl -fsS -X POST http://127.0.0.1:<port>/debug/loadtest/runtime/batch-update/user-quota/drain
4. 对每个旧实例继续执行：
   curl -fsS http://127.0.0.1:<port>/debug/loadtest/runtime
   确认响应中的 batch_update reason 里 BatchUpdateTypeUserQuota 对应 pending 为 0。
5. 任一旧实例 drain 返回非 2xx 或 pending 不为 0 时，停止迁移，修复该实例写库问题后重试 drain；不得启动新版本迁移。
6. 所有旧实例确认 pending=0 后，停止所有旧服务和写库进程，备份数据库。
7. 启动包含本计划迁移代码的新版本。新版本在 HTTP 服务和后台任务启动前执行 `EnsureAccountBalanceCentsMigration()`。
```

- [ ] **步骤 5：运行测试验证通过**

```bash
go test ./controller -run 'TestLoadtestRuntimeDrainUserQuotaBatch|TestLoadtestRuntimeRoute' -count=1
```

预期：PASS。

- [ ] **步骤 6：提交**

```bash
git add controller/loadtest_runtime.go controller/loadtest_runtime_test.go docs/superpowers/specs/2026-05-30-account-balance-cents-migration-design.md
git commit -m "feat(balance): 增加余额迁移旧实例 drain 入口"
```
---

### 任务 3：启动顺序、缓存清理和迁移状态接入

**文件：**
- 修改：`main.go`
- 修改：`model/user_cache.go`
- 测试：`model/account_balance_migration_test.go`。
- 本任务依赖任务 2A 的发布边界：预迁移版本不能包含本任务启动接入；迁移版本才包含本任务。
- [ ] **步骤 1：编写缓存清理红测**

在 `model/account_balance_migration_test.go` 增加：

```go
func TestEnsureAccountBalanceCentsMigrationInvalidatesOldUserCache(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "500000")
	require.NoError(t, DB.Create(&User{Id: 9230, Username: "cached", Quota: 20000000, Status: common.UserStatusEnabled}).Error)
	seedUserCacheForMigrationTest(t, &UserBase{Id: 9230, Quota: 20000000, Status: common.UserStatusEnabled, Username: "cached"})

	require.NoError(t, EnsureAccountBalanceCentsMigration())

	cache, err := GetUserCache(9230)
	require.NoError(t, err)
	assert.Equal(t, 4000, cache.Quota)
}

func TestEnsureAccountBalanceCentsMigrationDoesNotFinalizeWhenCacheClearFails(t *testing.T) {
	setupAccountBalanceMigrationTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "250000")
	require.NoError(t, DB.Create(&User{Id: 9231, Username: "cached-fail", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	seedUserCacheForMigrationTest(t, &UserBase{Id: 9231, Quota: 1000000, Status: common.UserStatusEnabled, Username: "cached-fail"})
	forceInvalidateAllUserCacheErrorForMigrationTest(errors.New("redis delete failed"))

	err := EnsureAccountBalanceCentsMigration()

	require.Error(t, err)
	assert.Equal(t, "true", getOptionValueForMigrationTest(t, "AccountBalanceCentsDataMigrated"))
	assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsMigrated"))
	assert.Empty(t, getOptionValueForMigrationTest(t, "AccountBalanceCentsMigratedAt"))
}
```

- [ ] **步骤 2：运行测试验证失败**

```bash
go test ./model -run 'TestEnsureAccountBalanceCentsMigrationInvalidatesOldUserCache|TestEnsureAccountBalanceCentsMigrationDoesNotFinalizeWhenCacheClearFails' -count=1
```

预期：FAIL，旧缓存仍返回 `20000000` 或缓存 seed helper 未实现。

- [ ] **步骤 3：实现用户缓存清理**

在 `model/user_cache.go` 增加批量失效接口。优先逐用户删除，不依赖 Redis `KEYS`：

```go
func InvalidateAllUserCacheByIDs(userIds []int) error {
	for _, userId := range userIds {
		if userId <= 0 {
			continue
		}
		if err := invalidateUserCache(userId); err != nil {
			return err
		}
	}
	return nil
}
```

迁移阶段先查询所有用户 ID，数据提交后调用该函数；Redis 未启用时返回 nil。Redis 删除任一用户缓存失败时，`EnsureAccountBalanceCentsMigration()` 必须返回错误，保留 `AccountBalanceCentsDataMigrated=true`，但不得写 `AccountBalanceCentsMigratedAt` 和 `AccountBalanceCentsMigrated`；下一次启动只重试运行时同步、缓存清理和最终标记，严禁重复除法。

- [ ] **步骤 4：接入启动顺序**

在 `main.go` 的 `InitResources()` 中，移动或插入迁移调用，顺序必须是：

```go
model.InitOptionMap()
err = common.InitRedisClient()
if err != nil { return err }
if err = model.EnsureAccountBalanceCentsMigration(); err != nil {
	common.FatalLog("failed to migrate account balance cents: " + err.Error())
	return err
}
```

`EnsureAccountBalanceCentsMigration()` 必须早于 `common.StartSystemMonitor()`、`model.SyncOptions()`、`model.InitBatchUpdater()`、异步任务轮询和 HTTP 服务启动。

- [ ] **步骤 5：运行测试验证通过**

```bash
go test ./model -run 'TestEnsureAccountBalanceCentsMigrationInvalidatesOldUserCache|TestEnsureAccountBalanceCentsMigrationDoesNotFinalizeWhenCacheClearFails|TestEnsureAccountBalanceCentsMigration' -count=1
```

预期：PASS。

- [ ] **步骤 6：提交**

```bash
git add main.go model/user_cache.go model/account_balance_migration.go model/account_balance_migration_test.go
git commit -m "feat(balance): 接入分制迁移启动流程"
```

---

### 任务 4：余额购买订阅改为按分扣款

**文件：**
- 修改：`controller/subscription_payment_balance.go`
- 测试：`controller/subscription_payment_balance_test.go` 或现有订阅支付测试文件

- [ ] **步骤 1：编写余额购买红测**

新增测试：

```go
func TestSubscriptionBalancePayAmountUsesCents(t *testing.T) {
	amount, err := subscriptionBalancePayAmount(39.9)
	require.NoError(t, err)
	assert.Equal(t, 3990, amount)

	old := common.QuotaPerUnit
	common.QuotaPerUnit = 999999
	t.Cleanup(func() { common.QuotaPerUnit = old })
	amount, err = subscriptionBalancePayAmount(40)
	require.NoError(t, err)
	assert.Equal(t, 4000, amount)
}
```

- [ ] **步骤 2：运行测试验证失败**

```bash
go test ./controller -run 'TestSubscriptionBalancePayAmountUsesCents' -count=1
```

预期：FAIL，返回旧倍率金额。

- [ ] **步骤 3：修改实现**

将 `subscriptionBalancePayAmount` 改为：

```go
func subscriptionBalancePayAmount(price float64) (int, error) {
	if price <= 0 {
		return errors.New("套餐不可购买")
	}
	amount, err := model.AccountBalanceCentsFromCNY(decimal.NewFromFloat(price))
	if err != nil {
		return errors.New("套餐价格无效")
	}
	return amount, nil
}
```

移除本函数对 `common.QuotaPerUnit` 的依赖。

- [ ] **步骤 4：运行测试验证通过**

```bash
go test ./controller -run 'TestSubscriptionBalancePayAmountUsesCents|Test.*Balance.*Subscription' -count=1
```

预期：PASS。

- [ ] **步骤 5：提交**

```bash
git add controller/subscription_payment_balance.go controller/subscription_payment_balance_test.go
git commit -m "fix(subscription): 余额购买按分扣款"
```

---

### 任务 5：普通充值、Stripe、Waffo、Creem、Kyren 新订单按分入账

**文件：**
- 修改：`controller/topup.go`
- 修改：`controller/topup_stripe.go`
- 修改：`controller/topup_waffo.go`
- 修改：`controller/topup_waffo_pancake.go`
- 修改：`controller/topup_creem.go`
- 修改：`controller/topup_kyren.go`
- 修改：`model/topup.go`
- 修改：`setting/payment_kyren.go`
- 测试：`controller/topup_account_balance_cents_test.go`、既有 `topup_*_test.go`

- [x] **步骤 1：编写新订单分制红测**

在 `controller/topup_account_balance_cents_test.go` 新增测试覆盖：

```go
func TestEpayTopUpCreatesAmountInCentsWhenTokenDisplayEnabled(t *testing.T) {
	setupTopUpCentsTestDB(t)
	oldDisplay := operation_setting.GetQuotaDisplayType()
	oldQPU := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	setQuotaDisplayTypeForTopUpTest(t, operation_setting.QuotaDisplayTypeTokens)
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQPU
		setQuotaDisplayTypeForTopUpTest(t, oldDisplay)
	})

	recorder := requestEpayForTopUpCentsTest(t, 40)
	require.Equal(t, http.StatusOK, recorder.Code)
	topUp := getLatestTopUpForUserTest(t, topUpCentsUserID)
	assert.EqualValues(t, 4000, topUp.Amount)
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
}

func TestStripeTopUpCreatesAmountInCents(t *testing.T) {
	setupTopUpCentsTestDB(t)
	recorder := requestStripeForTopUpCentsTest(t, 40)
	require.Equal(t, http.StatusOK, recorder.Code)
	topUp := getLatestTopUpForUserTest(t, topUpCentsUserID)
	assert.EqualValues(t, 4000, topUp.Amount)
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
}

func TestStripeWebhookCreditsTopUpAmountCents(t *testing.T) {
	setupTopUpCentsTestDB(t)
	seedStripeTopUpForWebhookTest(t, "stripe-cents", 4000, 40)
	require.NoError(t, completeStripeTopUpForTest("stripe-cents"))
	topUp := getTopUpByTradeNoForTopUpCentsTest(t, "stripe-cents")
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
	assert.Equal(t, 4000, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
}

func TestStripeWebhookPreservesCustomerIDWhenCreditingCents(t *testing.T) {
	setupTopUpCentsTestDB(t)
	seedStripeTopUpForWebhookTest(t, "stripe-customer", 4000, 40)
	require.NoError(t, completeStripeTopUpForTestWithCustomer("stripe-customer", "cus_balance_cents"))
	assert.Equal(t, "cus_balance_cents", getUserStripeCustomerForTopUpCentsTest(t, topUpCentsUserID))
	assert.Equal(t, 4000, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
}

func TestCreemWebhookPreservesEmailBackfillWhenCreditingCents(t *testing.T) {
	setupTopUpCentsTestDB(t)
	seedUserEmailForTopUpCentsTest(t, topUpCentsUserID, "")
	tradeNo := seedCreemTopUpForWebhookTest(t, 3990, 40)
	require.NoError(t, completeCreemTopUpForTestWithCustomer(tradeNo, "paid@example.com", "Paid User"))
	assert.Equal(t, "paid@example.com", getUserEmailForTopUpCentsTest(t, topUpCentsUserID))
	assert.Equal(t, 3990, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
}

func TestEveryProviderCreatesAndCreditsAccountBalanceCents(t *testing.T) {
	setupTopUpCentsTestDB(t)
	oldQPU := common.QuotaPerUnit
	common.QuotaPerUnit = 987654
	t.Cleanup(func() { common.QuotaPerUnit = oldQPU })
	providers := []topUpCentsProviderCase{
		newEpayCentsProviderCase(3990),
		newWaffoCentsProviderCase(3990),
		newWaffoPancakeCentsProviderCase(3990),
		newCreemCentsProviderCase(3990),
		newKyrenCentsProviderCase(3990),
	}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			tradeNo := provider.create(t)
			topUp := getTopUpByTradeNoForTopUpCentsTest(t, tradeNo)
			assert.EqualValues(t, 3990, topUp.Amount)
			assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
			before := getUserQuotaForTopUpCentsTest(t, topUp.UserId)
			require.NoError(t, provider.complete(tradeNo))
			assert.Equal(t, before+3990, getUserQuotaForTopUpCentsTest(t, topUp.UserId))
			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForTopUpCentsTest(t, tradeNo))
			assertTopUpSuccessLogUsesAccountBalanceFormat(t, tradeNo, "39.90")
		})
	}
}

func TestNewTopUpPersistsAccountBalanceCentsAmountUnit(t *testing.T) {
	setupTopUpCentsTestDB(t)
	recorder := requestEpayForTopUpCentsTest(t, 40)
	require.Equal(t, http.StatusOK, recorder.Code)
	topUp := getLatestTopUpForUserTest(t, topUpCentsUserID)
	assert.EqualValues(t, 4000, topUp.Amount)
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
}

func TestNormalizeKyrenTopUpProductsRejectsInvalidBalanceProducts(t *testing.T) {
	invalidCases := []string{
		`[{"id":"zero","name":"Zero","amount":"10.00","currency":"CNY","quota":0,"enabled":true}]`,
		`[{"id":"negative","name":"Negative","amount":"10.00","currency":"CNY","quota":-1,"enabled":true}]`,
		`[{"id":"usd","name":"USD","amount":"10.00","currency":"USD","quota":1000,"enabled":true}]`,
		`[{"id":"bad-amount","name":"Bad","amount":"1.234","currency":"CNY","quota":1000,"enabled":true}]`,
	}
	for _, raw := range invalidCases {
		_, err := setting.NormalizeKyrenTopUpProductsJSON(raw)
		require.Error(t, err)
	}
	normalized, err := setting.NormalizeKyrenTopUpProductsJSON(`[{"id":"ok","name":"OK","amount":"10","currency":"CNY","quota":1000,"enabled":true}]`)
	require.NoError(t, err)
	assert.Contains(t, normalized, `"amount":"10.00"`)
}

func TestExpiredTopUpCannotBeCompletedAfterCentsMigration(t *testing.T) {
	setupTopUpCentsTestDB(t)
	seedExpiredTopUpForProviderTest(t, "expired-manual", model.PaymentProviderEpay, "alipay", 20000000, 40)
	before := getUserQuotaForTopUpCentsTest(t, topUpCentsUserID)

	err := model.ManualCompleteTopUp("expired-manual", "127.0.0.1")

	require.Error(t, err)
	assert.Equal(t, before, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
	assert.Equal(t, common.TopUpStatusExpired, getTopUpStatusForTopUpCentsTest(t, "expired-manual"))
}

func TestExpiredProviderTopUpsCannotBeCreditedByLateWebhook(t *testing.T) {
	setupTopUpCentsTestDB(t)
	providers := []struct {
		name     string
		provider string
		method   string
		complete func(tradeNo string) error
	}{
		{name: "epay", provider: model.PaymentProviderEpay, method: "alipay", complete: completeEpayTopUpForTest},
		{name: "stripe", provider: model.PaymentProviderStripe, method: model.PaymentMethodStripe, complete: completeStripeTopUpForTest},
		{name: "waffo", provider: model.PaymentProviderWaffo, method: model.PaymentMethodWaffo, complete: completeWaffoTopUpForTest},
		{name: "waffo-pancake", provider: model.PaymentProviderWaffoPancake, method: model.PaymentMethodWaffoPancake, complete: completeWaffoPancakeTopUpForTest},
		{name: "creem", provider: model.PaymentProviderCreem, method: model.PaymentMethodCreem, complete: completeCreemTopUpForTest},
		{name: "kyren", provider: model.PaymentProviderKyren, method: model.PaymentMethodKyren, complete: completeKyrenTopUpForTest},
	}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			tradeNo := "expired-" + provider.name
			seedExpiredTopUpForProviderTest(t, tradeNo, provider.provider, provider.method, 20000000, 40)
			before := getUserQuotaForTopUpCentsTest(t, topUpCentsUserID)

			err := provider.complete(tradeNo)

			require.Error(t, err)
			assert.Equal(t, before, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
			assert.Equal(t, common.TopUpStatusExpired, getTopUpStatusForTopUpCentsTest(t, tradeNo))
		})
	}
}
```

Epay / Stripe / Waffo / Waffo Pancake / Creem / Kyren 的成功路径必须逐渠道硬断言：`TopUp.Amount = 3990`、`TopUp.AmountUnit = account_balance_cents`、成功回调按 `TopUp.Amount` 增加用户余额、状态变 success、日志使用账户余额格式；测试中修改 `common.QuotaPerUnit` 后结果不变。过期订单红测覆盖 `ManualCompleteTopUp`、Epay、Stripe、Waffo、Waffo Pancake、Creem、Kyren，断言迁移前 `expired` 旧订单不会被补单或迟到 webhook 入账。

- [x] **步骤 2：运行测试验证失败**

```bash
go test ./controller -run 'TopUp.*Cents|Expired.*TopUp|Test.*WebhookCreditsTopUpAmountCents|TestKyrenWebhookCompletesTopUpUsingSnapshot' -count=1
```

预期：FAIL，旧路径仍乘 `QuotaPerUnit` 或保存旧 amount。

- [x] **步骤 3：修改创建订单金额**

普通充值请求中：

```go
amountCents, err := model.AccountBalanceCentsFromCNY(decimal.NewFromInt(req.Amount))
if err != nil {
	common.ApiError(c, err)
	return
}
topUp.Amount = int64(amountCents)
topUp.AmountUnit = model.TopUpAmountUnitAccountBalanceCents
```

Stripe / Waffo / Waffo Pancake 同样在创建订单时把用户获得的账户余额 CNY 元转为分写入 `TopUp.Amount`，并写 `TopUp.AmountUnit = model.TopUpAmountUnitAccountBalanceCents`。`getMinTopup()` 和各渠道 min topup 不再在 token 展示模式下乘 `QuotaPerUnit`。

Creem / Kyren 产品配置中的 `quota` 已是分，创建订单继续复制分值，不再做旧倍率换算，并同样写 `TopUp.AmountUnit = model.TopUpAmountUnitAccountBalanceCents`。`setting.NormalizeKyrenTopUpProductsJSON` / 管理端保存路径必须拒绝 `quota <= 0`、`currency != CNY`、不可解析金额、超过两位小数金额，并把合法 `amount` 规范为 CNY 金额字符串。`controller/topup_creem.go` 中 `setting.CreemProducts`、Creem checkout request / response 的业务 JSON 使用 `common.UnmarshalJsonStr`、`common.Marshal`、`common.Unmarshal`，不得继续调用 `encoding/json`。

- [x] **步骤 4：修改 webhook / 补单入账**

所有成功回调、`model.ManualCompleteTopUp` 和 Kyren / Creem completion 使用账户余额 helper，并在事务成功提交后清理缓存：

```go
if err := model.DB.Transaction(func(tx *gorm.DB) error {
	if err := model.IncreaseUserAccountBalanceTx(tx, topUp.UserId, int(topUp.Amount)); err != nil {
		return err
	}
	return markTopUpSuccessTx(tx, topUp.TradeNo)
}); err != nil {
	return err
}
return model.InvalidateUserCache(topUp.UserId)
```

所有成功回调、`model.ManualCompleteTopUp` 和 Kyren / Creem completion 必须调用 `IncreaseUserAccountBalanceTx` 这类账户余额 helper，并在 `DB.Transaction` 成功返回后按 userId 调用 `InvalidateUserCache`；不得在事务提交前清理缓存，不得直接使用裸 `Update("quota", ...)`。不得乘 `QuotaPerUnit`。余额 helper 只负责余额入账，provider-specific 非余额字段必须保留原行为：Stripe 成功回调继续在同事务保存 `stripe_customer`，Creem 成功回调继续在用户邮箱为空时回填支付邮箱，Kyren / Waffo / Waffo Pancake 的既有订单审计字段也不得被删除。

所有入口只允许 `pending` 订单入账。迁移时已标记 `expired` 的历史订单必须返回错误或幂等忽略，但不得改变状态、不得写 `users.quota`、不得记录成功充值日志。`UpdatePendingTopUpStatus`、`Recharge`、`RechargeCreem`、`RechargeWaffo`、`RechargeWaffoPancake`、`completeKyrenTopUpWithSnapshot` 和 `ManualCompleteTopUp` 都必须保持该状态门禁。

日志使用账户余额格式：`AccountBalanceCNYFromCents(int(topUp.Amount)).StringFixed(2)`，不得使用 `logger.LogQuota` 展示账户余额。

- [x] **步骤 5：运行测试验证通过**

```bash
go test ./controller ./model -run 'TopUp.*Cents|Expired.*TopUp|Test.*WebhookCreditsTopUpAmountCents|TestKyrenWebhookCompletesTopUpUsingSnapshot|TestManualCompleteTopUp|TestGetTopUpInfoIncludesKyrenProducts' -count=1
```

预期：PASS。

- [x] **步骤 6：提交**

```bash
git add controller/topup.go controller/topup_stripe.go controller/topup_waffo.go controller/topup_waffo_pancake.go controller/topup_creem.go controller/topup_kyren.go model/topup.go setting/payment_kyren.go controller/topup_account_balance_cents_test.go
git commit -m "fix(topup): 充值订单按账户余额分入账"
```

---

### 任务 6：账单历史返回稳定展示字段

**文件：**
- 修改：`model/topup.go`
- 修改：`controller/topup.go`
- 修改：`web/default/src/features/wallet/types.ts`
- 修改：`web/default/src/features/wallet/components/dialogs/billing-history-dialog.tsx`
- 修改：`web/classic/src/components/topup/modals/TopupHistoryModal.jsx`
- 测试：`model/topup_history_cents_test.go`、`web/default/src/features/wallet/wallet-layout.test.ts`。classic 账单历史行为断言归任务 11 的 `web/classic/src/helpers/account-balance.test.js`，任务 6 只改 `TopupHistoryModal.jsx` 并通过目标 eslint 保证可用。

- [x] **步骤 1：编写服务端账单历史红测**

```go
func TestTopUpHistoryReturnsStableCreditedBalanceFields(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setOptionForMigrationTest(t, model.OptionAccountBalanceCentsMigratedAt, "2000")
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9301, Amount: 40, Money: 40, TradeNo: "legacy-epay", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: 3000}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9301, Amount: 4000, Money: 40, TradeNo: "new-epay", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: 1000, AmountUnit: model.TopUpAmountUnitAccountBalanceCents}).Error)

	items, _, err := model.GetUserTopUpHistoryItems(9301, common.NewPageInfo(0, 10))
	require.NoError(t, err)
	legacy := findHistoryItem(t, items, "legacy-epay")
	newer := findHistoryItem(t, items, "new-epay")
	assert.EqualValues(t, 4000, legacy.CreditedBalanceCents)
	assert.Equal(t, "legacy", legacy.AmountUnit)
	assert.EqualValues(t, 4000, newer.CreditedBalanceCents)
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, newer.AmountUnit)
	assert.True(t, newer.IsAccountBalanceCents)
}

func TestTopUpHistoryKyrenAndCreemSnapshotFallbacks(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setOptionForMigrationTest(t, model.OptionAccountBalanceCentsMigratedAt, "2000")
	kyrenSnapshot := `{"local_topup_id":"kyren_40","product_id":"prod_kyren_40","amount":"40.00","currency":"CNY","quota":4000}`
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9302, Amount: 20000000, Money: 40, TradeNo: "legacy-kyren-snapshot", PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren, Status: common.TopUpStatusSuccess, CreateTime: 1000, KyrenSnapshot: kyrenSnapshot}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9302, Amount: 20000000, Money: 40, TradeNo: "legacy-kyren-raw", PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren, Status: common.TopUpStatusSuccess, CreateTime: 1000}).Error)
	setOptionForMigrationTest(t, "CreemProducts", `[{"productId":"prod_creem_40","name":"Creem 40","price":40,"currency":"USD","quota":4000}]`)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9302, Amount: 20000000, Money: 40, TradeNo: "legacy-creem-product", PaymentProvider: model.PaymentProviderCreem, PaymentMethod: model.PaymentMethodCreem, Status: common.TopUpStatusSuccess, CreateTime: 1000}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9302, Amount: 12345678, Money: 13, TradeNo: "legacy-creem-raw", PaymentProvider: model.PaymentProviderCreem, PaymentMethod: model.PaymentMethodCreem, Status: common.TopUpStatusSuccess, CreateTime: 1000}).Error)

	items, _, err := model.GetUserTopUpHistoryItems(9302, common.NewPageInfo(0, 20))
	require.NoError(t, err)
	assert.EqualValues(t, 4000, findHistoryItem(t, items, "legacy-kyren-snapshot").CreditedBalanceCents)
	assert.Equal(t, "legacy", findHistoryItem(t, items, "legacy-kyren-raw").AmountUnit)
	assert.Zero(t, findHistoryItem(t, items, "legacy-kyren-raw").CreditedBalanceCents)
	assert.EqualValues(t, 4000, findHistoryItem(t, items, "legacy-creem-product").CreditedBalanceCents)
	assert.Equal(t, "legacy", findHistoryItem(t, items, "legacy-creem-raw").AmountUnit)
	assert.Zero(t, findHistoryItem(t, items, "legacy-creem-raw").CreditedBalanceCents)
}

func TestTopUpHistoryKyrenLegacySnapshotConvertsOldQuotaWithMigrationRate(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "500000")
	kyrenSnapshot := `{"local_topup_id":"kyren_40","product_id":"prod_kyren_40","amount":"40.00","currency":"CNY","quota":20000000}`
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9304, Amount: 20000000, Money: 40, TradeNo: "legacy-kyren-old-quota-snapshot", PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren, Status: common.TopUpStatusSuccess, KyrenSnapshot: kyrenSnapshot}).Error)
	items, _, err := model.GetUserTopUpHistoryItems(9304, common.NewPageInfo(0, 20))
	require.NoError(t, err)
	assert.EqualValues(t, 4000, findHistoryItem(t, items, "legacy-kyren-old-quota-snapshot").CreditedBalanceCents)
}

func TestTopUpHistoryKyrenLegacySnapshotDowngradesWhenMigrationRateUnavailable(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	setOptionForMigrationTest(t, "QuotaPerUnit", "0")
	kyrenSnapshot := `{"local_topup_id":"kyren_40","product_id":"prod_kyren_40","amount":"40.00","currency":"CNY","quota":20000000}`
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9305, Amount: 20000000, Money: 40, TradeNo: "legacy-kyren-old-quota-no-rate", PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren, Status: common.TopUpStatusSuccess, KyrenSnapshot: kyrenSnapshot}).Error)
	items, _, err := model.GetUserTopUpHistoryItems(9305, common.NewPageInfo(0, 20))
	require.NoError(t, err)
	item := findHistoryItem(t, items, "legacy-kyren-old-quota-no-rate")
	assert.Equal(t, "legacy", item.AmountUnit)
	assert.False(t, item.IsAccountBalanceCents)
	assert.Zero(t, item.CreditedBalanceCents)
	assert.NotEmpty(t, item.CreditedBalanceDisplay)
}

func TestAdminAndSearchTopUpHistoryReturnStableCreditedBalanceFields(t *testing.T) {
	setupTopUpHistoryCentsTestDB(t)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9303, Amount: 40, Money: 40, TradeNo: "legacy-admin", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: 9303, Amount: 4000, Money: 40, TradeNo: "new-admin", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, AmountUnit: model.TopUpAmountUnitAccountBalanceCents}).Error)

	allItems, _, err := model.GetAllTopUpHistoryItems(common.NewPageInfo(0, 10))
	require.NoError(t, err)
	searchedUserItems, _, err := model.SearchUserTopUpHistoryItems(9303, "admin", common.NewPageInfo(0, 10))
	require.NoError(t, err)
	searchedAllItems, _, err := model.SearchAllTopUpHistoryItems("admin", common.NewPageInfo(0, 10))
	require.NoError(t, err)
	for _, items := range [][]model.TopUpHistoryItem{allItems, searchedUserItems, searchedAllItems} {
		assert.EqualValues(t, 4000, findHistoryItem(t, items, "legacy-admin").CreditedBalanceCents)
		assert.EqualValues(t, 4000, findHistoryItem(t, items, "new-admin").CreditedBalanceCents)
		assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, findHistoryItem(t, items, "new-admin").AmountUnit)
	}
}
```

- [x] **步骤 2：运行测试验证失败**

```bash
go test ./model -run 'TestTopUpHistoryReturnsStableCreditedBalanceFields|TestTopUpHistoryKyrenAndCreemSnapshotFallbacks|TestTopUpHistoryKyrenLegacySnapshot|TestAdminAndSearchTopUpHistoryReturnStableCreditedBalanceFields' -count=1
```

预期：FAIL，缺少 DTO / 函数。

- [x] **步骤 3：实现 DTO 和控制器返回**

新增 DTO：

```go
type TopUpHistoryItem struct {
	TopUp
	CreditedBalanceCents int64  `json:"credited_balance_cents"`
	CreditedBalanceDisplay string `json:"credited_balance_display"`
	AmountUnit string `json:"amount_unit"`
	IsAccountBalanceCents bool `json:"is_account_balance_cents"`
}
```

`GetUserTopUps` / `GetAllTopUps` / `SearchUserTopUps` / `SearchAllTopUps` 变体返回 `[]TopUpHistoryItem`。新分制订单只信任订单级 `TopUp.AmountUnit == TopUpAmountUnitAccountBalanceCents`，不可依赖 `CreateTime >= AccountBalanceCentsMigratedAt` 推断单位；这样可以覆盖蓝绿发布、时钟偏差、迁移失败重试和人工补单导致的时间边界不可靠场景。迁移前旧订单 `amount_unit` 为空，普通 Epay / Stripe / Waffo / Pancake 历史成功记录可用 `amount * 100` 生成展示分并返回 `AmountUnit = "legacy"`；Kyren 历史记录解析 `TopUp.KyrenSnapshot` 的 `quota` 字段：若快照 quota 已明显为分制小额正数则返回该分值；若是迁移前历史 quota（例如 `20000000`），必须使用可确定的迁移倍率按 `round(snapshot.quota * 100 / QuotaPerUnit)` 换算；倍率不可确定或解析失败时返回 legacy/raw audit，不能填充 `credited_balance_cents`；Creem 历史记录用 `TopUp.Money` + `setting.CreemProducts` 中同价格 / 币种产品的 `quota` 匹配，唯一命中且 `quota > 0` 时返回该分值；Kyren / Creem 无可靠快照或唯一产品匹配时 `AmountUnit = "legacy"`、`IsAccountBalanceCents = false`、`CreditedBalanceCents = 0`，不得用 `amount * 100` 伪造分制金额。控制器不得继续返回裸 `[]TopUp`；用户、管理员、用户搜索和全局搜索四个列表入口必须共用同一 DTO 构造函数。

- [x] **步骤 4：前端改用展示字段**

新增 default `billing-history-dialog` 测试：新订单、legacy 可换算订单、legacy 不可换算订单三类记录都不得从 `record.amount` 推断到账余额；必须优先使用后端 `credited_balance_display`，其次使用 `credited_balance_cents`。classic `TopupHistoryModal` 的行为测试归任务 11 的 `account-balance.test.js`，任务 6 只负责让组件消费同一后端字段并通过目标 eslint。

`web/default` 和 `web/classic` 账单历史显示充值到账金额时优先使用：

```ts
record.credited_balance_display || formatAccountBalanceForPlanPurchase(record.credited_balance_cents)
```

如果 `is_account_balance_cents === false` 且 `amount_unit === 'legacy'`，显示后端提供的 legacy 文案 / 原始审计值，不自行用 `record.amount` 推断。

- [x] **步骤 5：运行测试验证通过**

```bash
go test ./model ./controller -run 'TopUpHistory|GetUserTopUps|GetAllTopUps|Search.*TopUp|KyrenAndCreemSnapshotFallbacks' -count=1
cd web/default && bun test src/features/wallet/wallet-layout.test.ts
cd ../classic && bun run eslint -- --quiet src/components/topup/modals/TopupHistoryModal.jsx
```

预期：后端 PASS；default 测试 PASS；classic eslint 对目标文件无错误。

- [x] **步骤 6：提交**

```bash
git add model/topup.go controller/topup.go model/topup_history_cents_test.go web/default/src/features/wallet/types.ts web/default/src/features/wallet/components/dialogs/billing-history-dialog.tsx web/classic/src/components/topup/modals/TopupHistoryModal.jsx
git commit -m "fix(wallet): 账单历史返回分制展示字段"
```

---

### 任务 7：钱包兑换码、注册邀请奖励、签到奖励和管理员调额分制化

**文件：**
- 修改：`controller/redemption.go`
- 修改：`controller/user.go`
- 修改：`controller/oauth_onboarding.go`
- 修改：`model/redemption.go`
- 修改：`model/user.go`
- 修改：`model/checkin.go`
- 修改：`controller/checkin.go`
- 修改：`setting/operation_setting/checkin_setting.go`
- 测试：`controller/redemption_cny_test.go`、`controller/user_manage_account_balance_test.go`、`model/user_account_balance_rewards_test.go`、`model/checkin_account_balance_test.go`

- [x] **步骤 1：编写红测**

```go
func TestBuildWalletRedemptionUsesSubmittedCents(t *testing.T) {
	setupRedemptionTestDB(t)
	redemptions, err := buildRedemptionsForCreate(1, model.Redemption{Name: "wallet", Type: model.RedemptionTypeWallet, Quota: 4000, Count: 1}, func() string { return "key" })
	require.NoError(t, err)
	assert.Equal(t, 4000, redemptions[0].Quota)
}

func TestInviteAndCheckinRewardsUseAccountBalanceCents(t *testing.T) {
	setupRewardCentsTestDB(t)
	oldInviter := common.QuotaForInviter
	oldInvitee := common.QuotaForInvitee
	oldNewUser := common.QuotaForNewUser
	checkinSetting := operation_setting.GetCheckinSetting()
	oldMin := checkinSetting.MinQuota
	oldMax := checkinSetting.MaxQuota
	t.Cleanup(func() {
		common.QuotaForInviter = oldInviter
		common.QuotaForInvitee = oldInvitee
		common.QuotaForNewUser = oldNewUser
		checkinSetting.MinQuota = oldMin
		checkinSetting.MaxQuota = oldMax
	})
	common.QuotaForNewUser = 2000
	common.QuotaForInviter = 1000
	common.QuotaForInvitee = 500
	checkinSetting.MinQuota = 20
	checkinSetting.MaxQuota = 20

	inviter := &User{Id: 9410, Username: "inviter", Status: common.UserStatusEnabled, AffCode: "AFF9410"}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{Id: 9411, Username: "invitee", Status: common.UserStatusEnabled, InviterId: 9410, AffCode: "AFF9411", Quota: common.QuotaForNewUser}
	require.NoError(t, DB.Create(invitee).Error)
	require.NoError(t, inviteUser(9410))
	require.NoError(t, increaseUserQuotaTx(DB, 9411, common.QuotaForInvitee))
	assert.Equal(t, 1000, getUserAffQuotaForMigrationTest(t, 9410))
	assert.Equal(t, 2500, getUserQuotaForAccountBalanceTest(t, 9411))

	require.NoError(t, inviter.TransferAffQuotaToQuota(100))
	assert.Equal(t, 900, getUserAffQuotaForMigrationTest(t, 9410))
	assert.Equal(t, 100, getUserQuotaForAccountBalanceTest(t, 9410))

	awarded, err := doCheckinRewardForTest(9411, "2026-05-30")
	require.NoError(t, err)
	assert.Equal(t, 20, awarded)
	assert.Equal(t, 2520, getUserQuotaForAccountBalanceTest(t, 9411))
}



func TestRegistrationRewardEntryPointsUseAccountBalanceCents(t *testing.T) {
	setupRewardCentsTestDB(t)
	common.QuotaForNewUser = 2000
	common.QuotaForInviter = 1000
	common.QuotaForInvitee = 500

	inserted := &User{Username: "insert", Quota: common.QuotaForNewUser}
	require.NoError(t, inserted.Insert(0))
	assert.Equal(t, 2000, getUserQuotaForAccountBalanceTest(t, inserted.Id))

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		withTx := &User{Username: "insert-tx", Quota: common.QuotaForNewUser}
		return withTx.InsertWithTx(tx, 0)
	}))
	assert.Equal(t, 2000, getUserQuotaByUsernameForRewardTest(t, "insert-tx"))

	finalized := finalizeCreationForAccountBalanceTest(t, "finalize", "AFF9410")
	assert.Equal(t, 2500, getUserQuotaForAccountBalanceTest(t, finalized.Id))
	assert.Equal(t, 1000, getUserAffQuotaForMigrationTest(t, 9410))

	oauth := finalizeOAuthCreationForAccountBalanceTest(t, "oauth", "AFF9410")
	assert.Equal(t, 2500, getUserQuotaForAccountBalanceTest(t, oauth.Id))
	assertInviteLogUsesAccountBalanceFormat(t, oauth.Id, "5.00")
}

func TestManageUserQuotaUsesAccountBalanceCents(t *testing.T) {
	setupManageUserAccountBalanceTestDB(t)
	admin := seedAdminForManageUserTest(t, 9420)
	user := seedUserForManageUserTest(t, 9421, 4000)

	performManageUserQuotaRequest(t, admin.Id, user.Id, "add", 250)
	assert.Equal(t, 4250, getUserQuotaForAccountBalanceTest(t, user.Id))
	assertManageLogContainsAccountBalance(t, user.Id, "2.50")

	performManageUserQuotaRequest(t, admin.Id, user.Id, "subtract", 125)
	assert.Equal(t, 4125, getUserQuotaForAccountBalanceTest(t, user.Id))
	assertManageLogContainsAccountBalance(t, user.Id, "1.25")

	performManageUserQuotaRequest(t, admin.Id, user.Id, "override", 3990)
	assert.Equal(t, 3990, getUserQuotaForAccountBalanceTest(t, user.Id))
	assertManageLogContainsAccountBalance(t, user.Id, "39.90")
}
```

- [x] **步骤 2：运行测试验证失败**

```bash
go test ./controller ./model -run 'TestBuildWalletRedemptionUsesSubmittedCents|TestInviteAndCheckinRewardsUseAccountBalanceCents|TestRegistrationRewardEntryPointsUseAccountBalanceCents|TestManageUserQuotaUsesAccountBalanceCents' -count=1
```

预期：FAIL，兑换码仍按 `QuotaPerUnit` 放大，或奖励 / 管理员调额日志不符合分制。

- [x] **步骤 3：修改兑换码请求边界**

`redemptionCNYAmountToQuota` 改为校验正整数分，或重命名为 `validateRedemptionWalletCents`：

```go
func validateRedemptionWalletCents(cents int) (int, error) {
	if cents <= 0 {
		return errors.New("兑换码金额必须大于0")
	}
	return cents, nil
}
```

`buildRedemptionsForCreate` / `applyRedemptionUpdate` 对钱包类型直接保存分。

- [x] **步骤 4：修改奖励、签到和管理员调额路径**

`User.Insert`、`User.InsertWithTx`、`FinalizeCreationTx`、`FinalizeOAuthUserCreation`、`inviteUser`、`inviteUserTx`、邀请划转、签到奖励保持 `common.QuotaForNewUser` / `QuotaForInviter` / `QuotaForInvitee` / `checkin_setting.*` 为分，移除这些函数中账户余额链路对 `QuotaPerUnit` 的依赖。日志展示使用账户余额格式，不用 `logger.LogQuota`。

`TransferAffQuotaToQuota` 的最小划转门槛从 `common.QuotaPerUnit` 改为 1 分或与前端一致的 100 分；不得再按历史「1 美元 quota」限制邀请奖励划转。

签到事务内用 `IncreaseUserAccountBalanceTx` 或等价 Tx helper，事务成功提交后失效 / 更新用户缓存。

`controller/user.go` 的 `ManageUser` / `add_quota` 分支按分处理 `req.Value`：`add` / `subtract` 调用账户余额 helper，`override` 设置分值后在事务成功提交后清理用户缓存；管理日志使用 `AccountBalanceCNYFromCents(req.Value).StringFixed(2)` 账户余额格式，不得使用 `logger.LogQuota`。

- [x] **步骤 5：运行测试验证通过**

```bash
go test ./controller ./model -run 'Redemption|Invite.*Reward|RegistrationRewardEntryPointsUseAccountBalanceCents|Checkin.*AccountBalance|ManageUserQuotaUsesAccountBalanceCents|TestBuildWalletRedemptionUsesSubmittedCents' -count=1
```

预期：PASS。

- [x] **步骤 6：提交**

```bash
git add controller/redemption.go controller/user.go controller/oauth_onboarding.go model/redemption.go model/user.go model/checkin.go controller/checkin.go setting/operation_setting/checkin_setting.go controller/redemption_cny_test.go controller/user_manage_account_balance_test.go model/user_account_balance_rewards_test.go model/checkin_account_balance_test.go
git commit -m "fix(balance): 奖励兑换码和管理调额按分处理"
```

---

### 任务 8：移除模型调用和异步任务对 users.quota 的 legacy wallet 写入

**文件：**
- 修改：`service/funding_source.go`
- 修改：`service/billing_session.go`
- 修改：`service/pre_consume_quota.go`
- 修改：`service/task_billing.go`
- 修改：`service/quota.go`
- 修改：`controller/midjourney.go`
- 修改：`controller/task_video.go`
- 修改：`service/subscription_only_billing_test.go`
- 修改：`service/task_billing_test.go`
- 修改：`service/task_group_removal_test.go`
- 修改：`controller/subscription_non_text_billing_test.go`

- [x] **步骤 1：编写阻断 legacy wallet 红测**

```go
func TestWalletFundingDoesNotWriteAccountBalanceForRelay(t *testing.T) {
	setupServiceBillingTestDB(t)
	user := seedUserWithQuota(t, 9401, 4000)
	funding := NewWalletFundingForTest(user.Id)
	err := funding.PreConsume(100)
	require.Error(t, err)
	assert.Equal(t, 4000, getUserQuotaForServiceTest(t, user.Id))
}

func TestPostConsumeQuotaRejectsNonSubscriptionWalletFallback(t *testing.T) {
	setupServiceBillingTestDB(t)
	relayInfo := &relaycommon.RelayInfo{UserId: 9402, BillingSource: service.BillingSourceWallet, TokenId: 1, TokenKey: "sk"}
	err := service.PostConsumeQuota(relayInfo, 100, 0, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscription")
}

func TestLegacyTaskRefundAndRecalculateDoNotWriteAccountBalance(t *testing.T) {
	setupAsyncTaskAccountBalanceTestDB(t)
	seedUserWithQuota(t, 9404, 4000)
	require.ErrorIs(t, refundTaskQuotaForAccountBalanceTest(9404, 500000), service.ErrLegacyWalletFundingDisabled)
	require.ErrorIs(t, recalculateTaskQuotaForAccountBalanceTest(9404, 500000), service.ErrLegacyWalletFundingDisabled)
	assert.Equal(t, 4000, getUserQuotaForServiceTest(t, 9404))
}

func TestAsyncTaskRefundDoesNotWriteAccountBalance(t *testing.T) {
	setupAsyncTaskAccountBalanceTestDB(t)
	seedUserWithQuota(t, 9403, 4000)
	err := completeFailedMidjourneyTaskForAccountBalanceTest(9403, 500000)
	require.Error(t, err)
	assert.Equal(t, 4000, getUserQuotaForServiceTest(t, 9403))
	err = completeFailedVideoTaskForAccountBalanceTest(9403, 500000)
	require.Error(t, err)
	assert.Equal(t, 4000, getUserQuotaForServiceTest(t, 9403))
}
```

- [x] **步骤 2：运行测试验证失败**

```bash
go test ./service ./controller -run 'WalletFundingDoesNotWrite|PostConsumeQuotaRejects|AsyncTaskRefundDoesNotWrite|RefundTaskQuota|RecalculateTaskQuota|TaskBilling|subscription.*billing|task.*billing' -count=1
```

预期：FAIL，旧代码仍写 `users.quota`。

- [x] **步骤 3：修改 funding 入口**

`WalletFunding` 保留类型但 relay / task 不得使用其写 `users.quota`。实现为明确错误：

```go
var ErrLegacyWalletFundingDisabled = errors.New("legacy wallet funding is disabled; use subscription billing")

func (w *WalletFunding) PreConsume(amount int) error { return ErrLegacyWalletFundingDisabled }
func (w *WalletFunding) Settle(delta int) error { return ErrLegacyWalletFundingDisabled }
func (w *WalletFunding) Refund() error { return ErrLegacyWalletFundingDisabled }
```

非 relay 兼容路径如果仍需要账户余额写入，必须调用 `IncreaseUserAccountBalanceTx` / `DeductUserAccountBalanceTx`，不得复用 `WalletFunding`。

- [x] **步骤 4：修改 `PostConsumeQuota` 和 `taskAdjustFunding`**

`service/billing_session.go`、`service/pre_consume_quota.go`、`service/quota.go` 的非订阅 fallback 返回错误，不再 `DecreaseUserQuota` / `IncreaseUserQuota`。既有 `RefundTaskQuota` / `RecalculateTaskQuota` / task billing 钱包退款或差额结算测试必须改写为断言用户余额不变，并返回 `ErrLegacyWalletFundingDisabled` 或记录人工处理；不得保留旧测试继续期待 `users.quota` 增减。`controller/midjourney.go` 和 `controller/task_video.go` 中任务失败退款不得写账户余额；迁移前旧任务如无法安全换算，返回明确错误并记录人工处理日志，不能把模型 quota delta 加到账户余额分。

- [x] **步骤 5：运行静态扫描与测试**

```bash
go test ./service ./controller -run 'WalletFundingDoesNotWrite|PostConsumeQuotaRejects|AsyncTaskRefundDoesNotWrite|RefundTaskQuota|RecalculateTaskQuota|TaskBilling|subscription.*billing|task.*billing|TestPreConsumeUserSubscription' -count=1
```

并使用 `search` 工具确认用量路径不再直接写账户余额：

```text
搜索 `DecreaseUserQuota(`、`IncreaseUserQuota(`、`DeltaUpdateUserQuota(`、`increaseUserQuota(`、`decreaseUserQuota(`、`addNewRecord(BatchUpdateTypeUserQuota`、`BatchUpdateTypeUserQuota`、`Updates(` + `quota`。
每个命中必须落入以下清单之一：
1. 账户余额业务入口：充值入账、余额购买订阅、钱包兑换码、邀请 / 注册 / 签到奖励、管理员调额。
2. 已阻断 legacy 路径：返回 `ErrLegacyWalletFundingDisabled`，不会写库。
3. 非账户余额用量字段：token / channel / log / subscription token，未写 `users.quota`。
任何不在清单内的命中必须在本任务修复并重跑测试。
```

- [x] **步骤 6：提交**

```bash
git add service/funding_source.go service/billing_session.go service/pre_consume_quota.go service/task_billing.go service/quota.go controller/midjourney.go controller/task_video.go service/subscription_only_billing_test.go service/task_billing_test.go service/task_group_removal_test.go controller/subscription_non_text_billing_test.go
git commit -m "fix(billing): 禁止模型调用写账户余额"
```

---

### 任务 9：web/default 账户余额 helper 与钱包 / 订阅展示

**文件：**
- 修改：`web/default/src/features/subscriptions/lib/subscription-balance.ts`
- 修改：`web/default/src/features/wallet/components/wallet-stats-card.tsx`
- 修改：`web/default/src/features/wallet/components/recharge-form-card.tsx`
- 修改：`web/default/src/features/wallet/components/dialogs/payment-confirm-dialog.tsx`
- 修改：`web/default/src/features/wallet/components/dialogs/creem-confirm-dialog.tsx`
- 修改：`web/default/src/features/wallet/components/creem-products-section.tsx`
- 修改：`web/default/src/features/wallet/index.tsx`
- 修改：`web/default/src/features/wallet/hooks/use-affiliate.ts`
- 修改：`web/default/src/features/wallet/constants.ts`
- 修改：`web/default/src/features/wallet/lib/payment.ts`
- 测试：`web/default/src/features/subscriptions/lib/subscription-balance.test.ts`、`web/default/src/features/wallet/wallet-layout.test.ts`

- [x] **步骤 1：编写前端红测**

```ts
test('account balance cents convert to CNY for plan purchase', () => {
  expect(accountBalanceCentsToCnyAmount(4000)).toBe(40)
  expect(formatAccountBalanceForPlanPurchase(3990)).toContain('39.90')
  expect(getAccountBalancePaymentState({ accountBalanceQuota: 3990, priceAmount: 39.9, currency: 'CNY' }).sufficient).toBe(true)
  expect(getAccountBalancePaymentState({ accountBalanceQuota: 3989, priceAmount: 39.9, currency: 'CNY' }).sufficient).toBe(false)
})
```

钱包测试断言：`quota_per_unit` 改为任意值时，`4000` 仍展示 `¥40.00`。

补充钱包产品卡 / 确认弹窗测试：`creem-products-section.tsx`、`dialogs/creem-confirm-dialog.tsx`、`recharge-form-card.tsx` 中 Creem 和 Kyren 用户侧产品 `quota=4000` / `quota=3990` 必须展示为 `¥40.00` / `¥39.90`，Creem 确认弹窗也展示 CNY 元；Kyren 直接跳转策略必须在点击前的产品按钮/说明中展示到账余额 CNY 元，并在测试中明确断言“Kyren 无额外确认弹窗，点击展示了 `¥39.90` 的产品按钮后才调用 `processKyrenPayment`”。不得出现 `Quota: 4000`、`Quota: 3990`、`4000 quota`、`3990 quota`、`{{quota}} quota` 或 raw `formatNumber(product.quota)`。

普通充值矩阵测试断言：Epay / Stripe / Waffo / Waffo Pancake 的预设金额、自定义金额和 `PaymentConfirmDialog` 都以到账余额 CNY 元展示；`calculatePresetPricing` 在 `quota_per_unit`、`quota_display_type = TOKENS`、`usdExchangeRate` 变化时仍以 CNY 元匹配 `AmountDiscount` key，提交 payload 使用 CNY 元，不乘旧倍率；确认弹窗同时显示到账余额和渠道实付金额。

邀请奖励转余额测试断言：`transfer-dialog.tsx` 输入 `40.00` 时 `use-affiliate.ts` 提交 `4000` 分，最小单位 / step 为 `0.01` CNY 或 1 分；`wallet/constants.ts` 中 `QUOTA_PER_DOLLAR = 500000` 不得被账户余额划转链路引用，修改旧倍率不影响提交值。


- [x] **步骤 2：运行测试验证失败**

```bash
cd web/default
bun test src/features/subscriptions/lib/subscription-balance.test.ts src/features/wallet/wallet-layout.test.ts
```

预期：FAIL，当前 helper 仍除以 `quotaPerUnit`。

- [x] **步骤 3：实现 default helper**

`subscription-balance.ts` 改为：

```ts
export function accountBalanceCentsToCnyAmount(balanceCents: number): number {
  if (!Number.isFinite(balanceCents) || balanceCents <= 0) return 0
  return balanceCents / 100
}

export function accountBalanceCnyToCents(amountCny: number): number {
  if (!Number.isFinite(amountCny) || amountCny <= 0) return 0
  return Math.round(amountCny * 100)
}
```

`getAccountBalancePaymentState` 用 `balanceCents >= Math.round(priceAmount * 100)`。

- [x] **步骤 4：更新钱包充值和产品展示**

普通充值输入、预设、支付确认和邀请奖励转余额使用「到账余额 CNY 元」。Kyren 直接跳转策略不新增确认弹窗，但产品按钮/说明必须在跳转前展示到账余额 CNY 元，测试覆盖点击 `¥39.90` 产品后调用 Kyren checkout。`calculatePresetPricing` 不再用 `usdExchangeRate` / `quota_per_unit` 二次展示账户余额。确认弹窗展示到账余额和渠道实付金额。`transfer-dialog.tsx` / `hooks/use-affiliate.ts` 将 CNY 元输入转为分提交，最小值 / step 不再依赖 `QUOTA_PER_DOLLAR`。`creem-products-section.tsx`、`dialogs/creem-confirm-dialog.tsx` 和 `recharge-form-card.tsx` 中 Kyren 产品列表均通过 account-balance helper 展示 `quota` 分值，删除 `t('{{quota}} quota')`、`t('Quota') + formatNumber(product.quota)` / `formatNumber(product.quota)` 这类 raw quota 文案。

- [x] **步骤 5：运行测试验证通过**

```bash
cd web/default
bun test src/features/subscriptions/lib/subscription-balance.test.ts src/features/wallet/wallet-layout.test.ts
bun run typecheck
```

预期：测试 PASS，`tsc -b` 退出码 0。

- [x] **步骤 6：提交**

```bash
git add web/default/src/features/subscriptions/lib/subscription-balance.ts web/default/src/features/subscriptions/lib/subscription-balance.test.ts web/default/src/features/wallet/components/wallet-stats-card.tsx web/default/src/features/wallet/components/recharge-form-card.tsx web/default/src/features/wallet/components/creem-products-section.tsx web/default/src/features/wallet/components/dialogs/payment-confirm-dialog.tsx web/default/src/features/wallet/components/dialogs/creem-confirm-dialog.tsx web/default/src/features/wallet/index.tsx web/default/src/features/wallet/hooks/use-affiliate.ts web/default/src/features/wallet/constants.ts web/default/src/features/wallet/lib/payment.ts web/default/src/features/wallet/wallet-layout.test.ts
git commit -m "fix(web-default): 账户余额按分展示"
```

---

### 任务 10：web/default 管理端、兑换码、奖励、签到和产品配置

**文件：**
- 修改：`web/default/src/features/users/components/users-columns.tsx`
- 修改：`web/default/src/features/users/components/users-mutate-drawer.tsx`
- 修改：`web/default/src/features/users/components/user-quota-dialog.tsx`
- 修改：`web/default/src/features/users/lib/user-form.ts`
- 修改：`web/default/src/features/profile/components/profile-header.tsx`
- 修改：`web/default/src/features/profile/components/dialogs/checkin-calendar-card.tsx`
- 修改：`web/default/src/features/usage-logs/components/dialogs/user-info-dialog.tsx`
- 修改：`web/default/src/features/redemption-codes/lib/redemption-form.ts`
- 修改：`web/default/src/features/redemption-codes/lib/redemption-form.test.ts`
- 修改：`web/default/src/features/redemption-codes/lib/redemption-batch.ts`
- 修改：`web/default/src/features/redemption-codes/components/redemptions-mutate-drawer.tsx`
- 修改：`web/default/src/features/redemption-codes/components/redemptions-columns.tsx`
- 修改：`web/default/src/features/system-settings/general/quota-settings-section.tsx`
- 修改：`web/default/src/features/system-settings/general/checkin-settings-section.tsx`
- 修改：`web/default/src/features/system-settings/integrations/payment-settings-section.tsx`
- 修改：`web/default/src/features/system-settings/integrations/amount-options-visual-editor.tsx`
- 修改：`web/default/src/features/system-settings/integrations/amount-discount-visual-editor.tsx`
- 修改：`web/default/src/features/system-settings/integrations/amount-discount-dialog.tsx`
- 修改：`web/default/src/features/system-settings/integrations/waffo-settings-section.tsx`
- 修改：`web/default/src/features/system-settings/integrations/waffo-pancake-settings-section.tsx`
- 修改：`web/default/src/features/system-settings/integrations/kyren-topup-product-dialog.tsx`
- 修改：`web/default/src/features/system-settings/integrations/kyren-topup-products-visual-editor.tsx`
- 修改：`web/default/src/features/system-settings/integrations/creem-product-dialog.tsx`
- 修改：`web/default/src/features/system-settings/integrations/creem-products-visual-editor.tsx`
- 测试：`web/default/src/features/redemption-codes/lib/redemption-form.test.ts`、`web/default/src/features/system-settings/general/quota-settings-section.test.ts`、`web/default/src/features/system-settings/general/checkin-settings-section.test.ts`、`web/default/src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx`、`web/default/src/features/system-settings/integrations/creem-products-visual-editor.test.tsx`、`web/default/src/features/system-settings/integrations/payment-settings-section.test.tsx`、`web/default/src/features/users/components/users-columns.test.ts`、`web/default/src/features/wallet/wallet-layout.test.ts`。

- [x] **步骤 1：编写红测**

补充测试断言：

```ts
test('wallet redemption submits cents', () => {
  const payload = transformFormDataToPayload({
    type: 'wallet',
    name: 'forty-cny',
    quota_cny: 40,
    plan_id: 0,
    count: 1,
  })
  assert.equal(payload.quota, 4000)
})

test('wallet redemption restores cents as CNY', () => {
  const defaults = transformRedemptionToFormDefaults({
    id: 1,
    user_id: 1,
    name: 'forty-cny',
    key: 'key',
    status: 1,
    type: 'wallet',
    quota: 4000,
    plan_id: 0,
    created_time: 0,
    redeemed_time: 0,
    expired_time: 0,
    used_user_id: 0,
    batch_id: 'batch-wallet',
  })
  assert.equal(defaults.quota_cny, 40)
})

test('quota settings save account balance rewards as cents', () => {
  assert.deepEqual(buildQuotaSettingsOptionUpdates({ QuotaForNewUser: 10, QuotaForInviter: 5, QuotaForInvitee: 3, PreConsumedQuota: 1000 }), [
    { key: 'QuotaForNewUser', value: '1000' },
    { key: 'QuotaForInviter', value: '500' },
    { key: 'QuotaForInvitee', value: '300' },
    { key: 'PreConsumedQuota', value: '1000' },
  ])
})

test('user form restores account balance cents as CNY', () => {
  const defaults = transformUserToFormDefaults({ id: 1, quota: 4000 })
  assert.equal(defaults.quota_cny, '40.00')
})

test('payment amount settings keep CNY yuan independent from quotaPerUnit', () => {
  const values = makePaymentValues({ MinTopUp: 40, AmountOptions: '[10,40]', AmountDiscount: '{"40":0.95}', StripeMinTopUp: 20, StripeUnitPrice: 7.3, WaffoMinTopUp: 30, WaffoUnitPrice: 7.1, WaffoPancakeMinTopUp: 25, WaffoPancakeUnitPrice: 6.9 })
  const updates = buildPaymentOptionUpdates(values, makePaymentValues({ MinTopUp: 1, AmountOptions: '[]', AmountDiscount: '{}', StripeMinTopUp: 1, StripeUnitPrice: 8, WaffoMinTopUp: 1, WaffoUnitPrice: 8, WaffoPancakeMinTopUp: 1, WaffoPancakeUnitPrice: 8 }))
  assert.deepEqual(updates.filter((item) => ['MinTopUp', 'payment_setting.amount_options', 'payment_setting.amount_discount', 'StripeMinTopUp', 'StripeUnitPrice', 'WaffoMinTopUp', 'WaffoUnitPrice', 'WaffoPancakeMinTopUp', 'WaffoPancakeUnitPrice'].includes(item.key)), [
    { key: 'MinTopUp', value: 40 },
    { key: 'payment_setting.amount_options', value: '[10,40]' },
    { key: 'payment_setting.amount_discount', value: '{"40":0.95}' },
    { key: 'StripeUnitPrice', value: 7.3 },
    { key: 'StripeMinTopUp', value: 20 },
    { key: 'WaffoMinTopUp', value: 30 },
    { key: 'WaffoUnitPrice', value: 7.1 },
    { key: 'WaffoPancakeMinTopUp', value: 25 },
    { key: 'WaffoPancakeUnitPrice', value: 6.9 },
  ])
})

test('kyren product editor round trips cents as CNY', () => {
  assert.equal(kyrenTopUpProductToForm({ quota: 3990 }).balance_cny, '39.90')
  assert.equal(kyrenTopUpProductFromForm({ balance_cny: '39.90' }).quota, 3990)
})

test('creem product editor round trips cents as CNY', () => {
  assert.equal(creemProductToForm({ quota: 3990 }).balance_cny, '39.90')
  assert.equal(creemProductFromForm({ balance_cny: '39.90' }).quota, 3990)
  const unchanged = creemProductFromForm(creemProductToForm({ quota: 4000 }))
  assert.equal(unchanged.quota, 4000)
})



test('checkin settings round trip account balance cents as CNY', () => {
  const defaults = checkinSettingsToFormDefaults({ enabled: true, minQuota: 2000, maxQuota: 3990 })
  assert.equal(defaults.minQuotaCny, '20.00')
  assert.equal(defaults.maxQuotaCny, '39.90')
  const updates = buildCheckinSettingsOptionUpdates(
    { enabled: true, minQuotaCny: '0.20', maxQuotaCny: '1.50' },
    defaults
  )
  assert.deepEqual(updates.filter((item) => ['checkin_setting.min_quota', 'checkin_setting.max_quota'].includes(item.key)), [
    { key: 'checkin_setting.min_quota', value: '20' },
    { key: 'checkin_setting.max_quota', value: '150' },
  ])
})

test('waffo settings describe min top-up as credited CNY balance', () => {
  const waffoSource = readFileSync('src/features/system-settings/integrations/waffo-settings-section.tsx', 'utf8')
  const pancakeSource = readFileSync('src/features/system-settings/integrations/waffo-pancake-settings-section.tsx', 'utf8')
  assert.match(waffoSource, /Minimum credited balance \(CNY\)/)
  assert.match(pancakeSource, /Minimum credited balance \(CNY\)/)
  assert.doesNotMatch(waffoSource, /Minimum top-up \(USD\)|Minimum top-up quantity/)
  assert.doesNotMatch(pancakeSource, /Minimum top-up \(USD\)|最低充值美元数量/)
})
```

- [x] **步骤 2：运行测试验证失败**

```bash
cd web/default
bun test src/features/redemption-codes/lib/redemption-form.test.ts src/features/system-settings/general/quota-settings-section.test.ts src/features/system-settings/general/checkin-settings-section.test.ts src/features/system-settings/integrations/payment-settings-section.test.tsx src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx src/features/system-settings/integrations/creem-products-visual-editor.test.tsx
```

预期：FAIL，仍按旧 quota 或 raw cents。

- [x] **步骤 3：实现 default 管理端转换**

所有账户余额输入用 CNY 元，提交前 `accountBalanceCnyToCents`；编辑回显用 `accountBalanceCentsToCnyAmount`。`checkin-settings-section.tsx` 的 `min_quota` / `max_quota` 从分回显 CNY 元，保存时转回分。非账户余额字段（`PreConsumedQuota`、`QuotaRemindThreshold`、used quota、subscription token）不改。

在 `quota-settings-section.tsx` 导出并使用 `buildQuotaSettingsOptionUpdates(values)`；`QuotaForNewUser`、`QuotaForInviter`、`QuotaForInvitee` 转分，`PreConsumedQuota` 保持原值。在 `payment-settings-section.tsx` 抽出并导出 `buildPaymentOptionUpdates(values, initial)`，`AmountOptions`、`AmountDiscount` key、`MinTopUp`、`StripeMinTopUp` 按 CNY 元原样保存，`Price` / `StripeUnitPrice` 只作为渠道实付计算规则保存和说明。

`amount-options-visual-editor.tsx`、`amount-discount-visual-editor.tsx`、`amount-discount-dialog.tsx` 的标签 / 描述改为账户余额 CNY 元；`waffo-settings-section.tsx`、`waffo-pancake-settings-section.tsx` 的 `WaffoMinTopUp` / `WaffoPancakeMinTopUp` 标签改为最低到账余额 CNY 元，`WaffoUnitPrice` / `WaffoPancakeUnitPrice` 标签说明为渠道实付单价，不是到账倍率。

Kyren / Creem 产品 dialog 导出并使用 `kyrenTopUpProductToForm`、`kyrenTopUpProductFromForm`、`creemProductToForm`、`creemProductFromForm`；表单字段使用 `balance_cny`，后端字段 `quota` 保持分。

- [x] **步骤 4：实现签到、用户、兑换码展示**

新增 / 导出的前端 helper 签名固定为：

```ts
export function buildQuotaSettingsOptionUpdates(values: QuotaFormValues): Array<{ key: string; value: string }>
export function buildPaymentOptionUpdates(values: PaymentFormValues, initial: PaymentFormValues): OptionUpdate[]
export function checkinSettingsToFormDefaults(values: CheckinSettingsValues): CheckinSettingsFormDefaults
export function buildCheckinSettingsOptionUpdates(values: CheckinSettingsFormValues, initial: CheckinSettingsFormDefaults): OptionUpdate[]
export function kyrenTopUpProductToForm(product: Pick<KyrenTopUpProduct, 'quota'>): { balance_cny: string }
export function kyrenTopUpProductFromForm(values: { balance_cny: string }): Pick<KyrenTopUpProduct, 'quota'>
export function creemProductToForm(product: Pick<CreemProductData, 'quota'>): { balance_cny: string }
export function creemProductFromForm(values: { balance_cny: string }): Pick<CreemProductData, 'quota'>
```

`profile-header`、签到日历、用户列表 / 详情、`users/lib/user-form.ts` 中 `quota` 默认值字段统一为 `quota_cny` 且 `4000 -> "40.00"`，邀请余额 `aff_quota` / `aff_history_quota` 的展示放在 users columns 和 usage-logs dialog 测试中；手动调额显示账户余额分为 CNY；`user-quota-dialog.tsx` 导出并测试调额转换 helper，输入 `40.00` 提交 `value:4000`，当前 `4000` 显示 `¥40.00`；钱包兑换码 `quota=4000` 回显 `40.00`，提交仍 `4000`。`used_quota`、日志用量和 token 用量继续使用原用量格式。

- [x] **步骤 5：运行测试和类型检查**

```bash
cd web/default
bun test src/features/redemption-codes/lib/redemption-form.test.ts src/features/system-settings/general/quota-settings-section.test.ts src/features/system-settings/general/checkin-settings-section.test.ts src/features/system-settings/integrations/payment-settings-section.test.tsx src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx src/features/system-settings/integrations/creem-products-visual-editor.test.tsx src/features/users/components/users-columns.test.ts src/features/users/components/user-quota-dialog.test.tsx src/features/wallet/wallet-layout.test.ts
bun run typecheck
```

预期：PASS。

- [x] **步骤 6：提交**

```bash
git add web/default/src/features/users web/default/src/features/profile web/default/src/features/usage-logs/components/dialogs/user-info-dialog.tsx web/default/src/features/redemption-codes web/default/src/features/system-settings/general web/default/src/features/system-settings/integrations web/default/src/features/wallet
git commit -m "fix(web-default): 管理端余额输入改为元"
```

---

### 任务 11：web/classic 账户余额链路完整分制化

**文件：**
- 创建：`web/classic/src/helpers/account-balance.js`
- 测试：`web/classic/src/helpers/account-balance.test.js`
- 修改：`web/classic/src/helpers/quota.js`
- 修改：`web/classic/src/components/topup/index.jsx`
- 修改：`web/classic/src/components/topup/RechargeCard.jsx`
- 修改：`web/classic/src/components/topup/SubscriptionPlansCard.jsx`
- 修改：`web/classic/src/components/topup/modals/SubscriptionPurchaseModal.jsx`
- 修改：`web/classic/src/components/topup/InvitationCard.jsx`
- 修改：`web/classic/src/components/topup/modals/TransferModal.jsx`
- 修改：`web/classic/src/components/topup/modals/PaymentConfirmModal.jsx`
- 修改：`web/classic/src/components/topup/modals/TopupHistoryModal.jsx`
- 修改：`web/classic/src/components/table/redemptions/RedemptionsColumnDefs.jsx`
- 修改：`web/classic/src/components/table/redemptions/RedemptionsTable.jsx`
- 修改：`web/classic/src/components/table/redemptions/modals/EditRedemptionModal.jsx`
- 修改：`web/classic/src/hooks/redemptions/useRedemptionsData.jsx`
- 修改：`web/classic/src/components/settings/personal/cards/CheckinCalendar.jsx`
- 修改：`web/classic/src/components/settings/personal/components/UserInfoHeader.jsx`
- 修改：`web/classic/src/pages/Setting/Operation/SettingsCheckin.jsx`
- 修改：`web/classic/src/pages/Setting/Operation/SettingsCreditLimit.jsx`
- 修改：`web/classic/src/pages/Setting/Payment/SettingsGeneralPayment.jsx`
- 修改：`web/classic/src/pages/Setting/Payment/SettingsPaymentGateway.jsx`
- 修改：`web/classic/src/pages/Setting/Payment/SettingsPaymentGatewayStripe.jsx`
- 修改：`web/classic/src/pages/Setting/Payment/SettingsPaymentGatewayWaffo.jsx`
- 修改：`web/classic/src/pages/Setting/Payment/SettingsPaymentGatewayWaffoPancake.jsx`
- 修改：`web/classic/src/pages/Setting/Payment/SettingsPaymentGatewayCreem.jsx`
- 修改：`web/classic/src/components/table/users/UsersColumnDefs.jsx`
- 修改：`web/classic/src/components/table/users/UsersTable.jsx`
- 修改：`web/classic/src/components/table/users/modals/EditUserModal.jsx`
- 修改：`web/classic/src/components/table/usage-logs/modals/UserInfoModal.jsx`
- 测试：`web/classic/src/helpers/account-balance.test.js`。

- [x] **步骤 1：创建 classic helper 红测**

```js
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import { accountBalanceCentsToCnyAmount, accountBalanceCnyToCents, formatAccountBalance } from './account-balance'

describe('classic account balance helper', () => {
  test('uses cents', () => {
    assert.equal(accountBalanceCentsToCnyAmount(4000), 40)
    assert.equal(accountBalanceCnyToCents(39.9), 3990)
    assert.match(formatAccountBalance(4000), /40\.00/)
  })

  test('classic payment gateway labels use credited CNY balance', () => {
    const epay = readFileSync('src/pages/Setting/Payment/SettingsPaymentGateway.jsx', 'utf8')
    const stripe = readFileSync('src/pages/Setting/Payment/SettingsPaymentGatewayStripe.jsx', 'utf8')
    const waffo = readFileSync('src/pages/Setting/Payment/SettingsPaymentGatewayWaffo.jsx', 'utf8')
    const pancake = readFileSync('src/pages/Setting/Payment/SettingsPaymentGatewayWaffoPancake.jsx', 'utf8')
    assert.doesNotMatch(epay + stripe + waffo + pancake, /最低充值美元数量|充值价格（x元\/美金）|getQuotaPerUnit/)
    assert.match(epay + stripe + waffo + pancake, /到账余额|实付单价/)
  })


  test('classic subscription balance purchase calls balance API', () => {
    const card = readFileSync('src/components/topup/SubscriptionPlansCard.jsx', 'utf8')
    const modal = readFileSync('src/components/topup/modals/SubscriptionPurchaseModal.jsx', 'utf8')
    assert.match(card, /\/api\/subscription\/balance\/pay/)
    assert.match(card, /idempotency_key/)
    assert.match(modal, /Pay with Account Balance|账户余额支付|余额支付/)
    assert.match(modal, /Math\.round\([^)]*price_amount[^)]*\*\s*100\)|balanceCents\s*>=/)
  })
})
```

- [x] **步骤 2：运行测试验证失败**

```bash
cd web/classic
bun test src/helpers/account-balance.test.js
```

预期：FAIL，文件不存在。

- [x] **步骤 3：实现 classic helper**

创建 `account-balance.js`：

```js
export const accountBalanceCentsToCnyAmount = (cents) => {
  const value = Number(cents || 0)
  if (!Number.isFinite(value) || value <= 0) return 0
  return value / 100
}

export const accountBalanceCnyToCents = (amount) => {
  const value = Number(amount || 0)
  if (!Number.isFinite(value) || value <= 0) return 0
  return Math.round(value * 100)
}

export const formatAccountBalance = (cents) => {
  return `¥${accountBalanceCentsToCnyAmount(cents).toFixed(2)}`
}
```

- [x] **步骤 4：替换 classic 账户余额入口**

将以下账户余额显示 / 输入从 `renderQuota`、`renderQuotaWithAmount`、`getQuotaPerUnit`、`quotaToDisplayAmount`、`displayAmountToQuota` 改为 `accountBalance` helper：

- 当前余额：`RechargeCard.jsx` 用 `formatAccountBalance(userState.user.quota)`；`used_quota` 继续用 `renderQuota`。
- 邀请奖励余额 / 历史：`InvitationCard.jsx` 用 `formatAccountBalance(aff_quota)`、`formatAccountBalance(aff_history_quota)`。
- 邀请划转：`TransferModal.jsx` 和 `topup/index.jsx` 中 `transfer()` 的输入以 CNY 元展示，提交前用 `accountBalanceCnyToCents`，最小值不再 `getQuotaPerUnit()`。
- 充值输入 / 预设 / 确认：`RechargeCard.jsx`、`PaymentConfirmModal.jsx`、`topup/index.jsx` 展示到账余额 CNY 元与渠道实付金额；不要再用 `renderQuotaWithAmount(topUpCount)` 表示到账余额。
- Creem 产品确认：`topup/index.jsx` 中 `selectedCreemProduct.quota` 显示 `formatAccountBalance(product.quota)`。
- 账单历史：`TopupHistoryModal.jsx` 使用后端 `credited_balance_display` / `credited_balance_cents`。
- 兑换码：`RedemptionsColumnDefs.jsx` 钱包类型显示 `formatAccountBalance(record.quota)`；`EditRedemptionModal.jsx` 回显 `quota / 100`，提交 `Math.round(amount * 100)`；`useRedemptionsData.jsx` 和 `RedemptionsTable.jsx` 不再预先把钱包 quota 走旧 helper。
- 签到：`CheckinCalendar.jsx`、`SettingsCheckin.jsx` 将奖励上下限按 CNY 元回显 / 保存分。
- 注册 / 邀请奖励配置：`SettingsCreditLimit.jsx` 中 `QuotaForNewUser`、`QuotaForInviter`、`QuotaForInvitee` 用 CNY 元，`PreConsumedQuota` 保留 Token / quota 语义。
- 支付配置：`SettingsPaymentGateway.jsx`、`SettingsPaymentGatewayStripe.jsx`、`SettingsPaymentGatewayWaffo.jsx`、`SettingsPaymentGatewayWaffoPancake.jsx` 的 `MinTopUp` / `StripeMinTopUp` / `WaffoMinTopUp` / `WaffoPancakeMinTopUp` 标签改为最低到账余额 CNY 元；`Price` / `StripeUnitPrice` / `WaffoUnitPrice` / `WaffoPancakeUnitPrice` 标签说明为渠道实付单价，不是到账倍率。
- Creem 产品配置：`SettingsPaymentGatewayCreem.jsx` 中产品 `quota` 用 CNY 元回显，保存为分。
- 订阅余额购买：`SubscriptionPlansCard.jsx` 必须实现 `payBalance` 等价回调，调用 `API.post('/api/subscription/balance/pay', { plan_id, idempotency_key })`，把当前用户余额分传入 `SubscriptionPurchaseModal.jsx`；弹窗在 CNY 套餐且余额充足时显示余额支付按钮 / 文案，按 `balanceCents >= Math.round(priceAmount * 100)` 判断启用状态，余额不足时禁用或提示，成功后刷新订阅和用户余额；不得再用 `renderQuota` 或货币兑换倍率推断。
- 个人页头：`UserInfoHeader.jsx` 中 `user.quota` 改为 `formatAccountBalance(userState?.user?.quota)`，`used_quota` 或模型用量字段继续使用原 `renderQuota`。
- 用户列表 / 用户详情：`UsersColumnDefs.jsx`、`UsersTable.jsx`、`EditUserModal.jsx`、`UserInfoModal.jsx` 中账户余额、邀请余额用 `formatAccountBalance`，`used_quota` 保留 `renderQuota`；不要再计算 `used + quota` 当总额度。

classic 测试矩阵必须覆盖每个规格入口至少一个分制行为或禁止旧 helper 的 source 断言：`RechargeCard`、`InvitationCard`、`TransferModal`、`RedemptionsColumnDefs`、`EditRedemptionModal`、`CheckinCalendar`、`SettingsCreditLimit`、`SettingsCheckin`、`EditUserModal`、`UserInfoModal`、`TopupHistoryModal` 分别验证 `4000 -> ¥40.00`、`40.00 -> 4000` 或使用 `credited_balance_*`，不得只依赖 eslint。

- [x] **步骤 5：运行 classic 测试 / lint**

```bash
cd web/classic
bun test src/helpers/account-balance.test.js
bun run eslint -- --quiet src/helpers/account-balance.js src/components/topup/index.jsx src/components/topup/RechargeCard.jsx src/components/topup/SubscriptionPlansCard.jsx src/components/topup/InvitationCard.jsx src/components/topup/modals/SubscriptionPurchaseModal.jsx src/components/topup/modals/TransferModal.jsx src/components/topup/modals/PaymentConfirmModal.jsx src/components/topup/modals/TopupHistoryModal.jsx src/components/table/redemptions/RedemptionsColumnDefs.jsx src/components/table/redemptions/RedemptionsTable.jsx src/components/table/redemptions/modals/EditRedemptionModal.jsx src/hooks/redemptions/useRedemptionsData.jsx src/components/settings/personal/cards/CheckinCalendar.jsx src/components/settings/personal/components/UserInfoHeader.jsx src/pages/Setting/Operation/SettingsCheckin.jsx src/pages/Setting/Operation/SettingsCreditLimit.jsx src/pages/Setting/Payment/SettingsGeneralPayment.jsx src/pages/Setting/Payment/SettingsPaymentGateway.jsx src/pages/Setting/Payment/SettingsPaymentGatewayStripe.jsx src/pages/Setting/Payment/SettingsPaymentGatewayWaffo.jsx src/pages/Setting/Payment/SettingsPaymentGatewayWaffoPancake.jsx src/pages/Setting/Payment/SettingsPaymentGatewayCreem.jsx src/components/table/users/UsersColumnDefs.jsx src/components/table/users/UsersTable.jsx src/components/table/users/modals/EditUserModal.jsx src/components/table/usage-logs/modals/UserInfoModal.jsx
```

预期：测试 PASS，eslint 对目标文件无错误。

- [x] **步骤 6：提交**

```bash
git add web/classic/src/helpers web/classic/src/components/topup web/classic/src/components/table/redemptions web/classic/src/components/settings/personal web/classic/src/pages/Setting/Operation web/classic/src/pages/Setting/Payment web/classic/src/components/table/users web/classic/src/components/table/usage-logs
git commit -m "fix(web-classic): 账户余额按分展示和输入"
```

---

### 任务 12：i18n 同步与误导性 quota 文案收口

**文件：**
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
- 修改：`web/classic/src/i18n/locales/{en,zh,fr,ja,ru,vi,zh-CN,zh-TW}.json`
- 测试：`web/default/src/features/subscriptions/account-balance-i18n.test.ts`、`web/default/src/features/subscriptions/kyren-i18n.test.ts`、`web/classic/src/helpers/account-balance-i18n.test.js`。

- [ ] **步骤 1：编写 i18n 红测**

新增测试断言新增 key 在所有 locale 存在：

```ts
const requiredAccountBalanceKeys = [
  'Account balance',
  'Top-up credit',
  'Credited balance',
  'Credited balance must be at least ¥0.01',
  'Amount is in CNY',
]
```

classic 同样检查 8 个 locale。

- [ ] **步骤 2：运行测试验证失败**

```bash
cd web/default
bun test src/features/subscriptions/account-balance-i18n.test.ts
cd ../classic
bun test src/helpers/account-balance-i18n.test.js
```

预期：FAIL，缺少 key。

- [ ] **步骤 3：补齐翻译**

更新所有 locale。账户余额链路使用 Account Balance / Wallet Balance / Top-up credit / 账户余额 / 到账余额，不再使用 Quota / 额度 / quota units。非账户余额用量文案不改。

- [ ] **步骤 4：运行静态扫描**

使用 `search` 工具检查账户余额相关文件中误导性文案：

```text
Quota|额度|quota units|{{quota}} quota|credited with quota
```

每个命中必须分类记录：账户余额文件中的误导性命中必须修复；usage logs、channel used_quota、API key quota、subscription token 中允许保留。修复后重跑本扫描，直到所有命中都有明确分类且账户余额链路无误导性文案。

- [ ] **步骤 5：运行 i18n 测试**

```bash
cd web/default
bun test src/features/subscriptions/account-balance-i18n.test.ts src/features/subscriptions/kyren-i18n.test.ts
cd ../classic
bun test src/helpers/account-balance-i18n.test.js
```

预期：PASS。

- [ ] **步骤 6：提交**

```bash
git add web/default/src/i18n/locales web/default/src/features/subscriptions/account-balance-i18n.test.ts web/classic/src/i18n/locales web/classic/src/helpers/account-balance-i18n.test.js
git commit -m "fix(i18n): 更新账户余额分制文案"
```

---

### 任务 12A：短停机迁移回滚手册与验收

**文件：**
- 修改：`docs/superpowers/specs/2026-05-30-account-balance-cents-migration-design.md`
- 修改：`docs/superpowers/plans/2026-05-30-account-balance-cents-migration.md`
- 测试：文档验收清单由任务 13 静态检查覆盖。

- [ ] **步骤 1：补充回滚 runbook 验收**

在规格「回滚策略」下保留并细化以下可执行检查项：

```text
1. 停止新版本服务和所有写库进程。
2. 恢复迁移前数据库备份；禁止在已迁移数据库上直接启动旧版本。
3. 部署旧版本服务。
4. 验证抽样用户余额仍按旧 quota 单位显示。
5. 验证普通充值、Kyren 充值档位、钱包兑换码和余额购买订阅入口恢复到旧版本行为。
6. 明确不实现自动反向迁移；如必须回退，唯一支持路径是恢复数据库备份。
```

- [ ] **步骤 2：最终 gate 检查**

任务 13 静态扫描必须确认计划和规格中同时包含「恢复迁移前数据库备份」「部署旧版本服务」「验证用户余额和充值入口」「不实现自动反向迁移」。缺任一项不得交付。

- [ ] **步骤 3：提交**

```bash
git add docs/superpowers/specs/2026-05-30-account-balance-cents-migration-design.md docs/superpowers/plans/2026-05-30-account-balance-cents-migration.md
git commit -m "docs(balance): 补充分制迁移回滚验收"
```

---

### 任务 13：最终集成验证与静态扫描

**文件：**
- 汇总验证本计划任务 1–12A 修改的后端、`web/default`、`web/classic`、i18n 和回滚 runbook 文件。

- [ ] **步骤 1：后端定向测试**

运行：

```bash
go test ./model ./controller ./service -run 'AccountBalance|TopUp.*Cents|TopUpHistory|Redemption|Checkin|Invite|ManageUserQuotaUsesAccountBalanceCents|WalletFunding|PostConsumeQuota|RefundTaskQuota|RecalculateTaskQuota|TaskBilling|SubscriptionBalance|Kyren|Creem|Waffo|Stripe|TopUpAmountUnitColumnAutoMigrateSQLite' -count=1
```

预期：PASS。

如 CI 环境提供 MySQL / PostgreSQL 服务，继续运行迁移 dry-run 集成测试；未提供服务时必须至少运行本步骤的 SQL 方言静态扫描，禁止引入 SQLite-only、MySQL-only 或 PostgreSQL-only 迁移逻辑：

```bash
go test ./model -run 'TestEnsureAccountBalanceCentsMigration.*MySQL|TestEnsureAccountBalanceCentsMigration.*PostgreSQL' -count=1
```

- [ ] **步骤 2：前端 default 测试与类型检查**

运行：

```bash
cd web/default
bun test src/features/subscriptions/lib/subscription-balance.test.ts src/features/wallet/wallet-layout.test.ts src/features/redemption-codes/lib/redemption-form.test.ts src/features/system-settings/general/quota-settings-section.test.ts src/features/system-settings/general/checkin-settings-section.test.ts src/features/system-settings/integrations/payment-settings-section.test.tsx src/features/system-settings/integrations/kyren-topup-products-visual-editor.test.tsx src/features/system-settings/integrations/creem-products-visual-editor.test.tsx src/features/users/components/user-quota-dialog.test.tsx src/features/subscriptions/account-balance-i18n.test.ts src/features/subscriptions/kyren-i18n.test.ts src/features/users/components/users-columns.test.ts
bun run typecheck
```

预期：PASS，`tsc -b` 退出码 0。

- [ ] **步骤 3：前端 classic 测试与目标 lint**

运行：

```bash
cd web/classic
bun test src/helpers/account-balance.test.js src/helpers/account-balance-i18n.test.js
bun run eslint -- --quiet src/helpers/account-balance.js src/components/topup/index.jsx src/components/topup/RechargeCard.jsx src/components/topup/SubscriptionPlansCard.jsx src/components/topup/InvitationCard.jsx src/components/topup/modals/SubscriptionPurchaseModal.jsx src/components/topup/modals/TransferModal.jsx src/components/topup/modals/PaymentConfirmModal.jsx src/components/topup/modals/TopupHistoryModal.jsx src/components/table/redemptions/RedemptionsColumnDefs.jsx src/components/table/redemptions/RedemptionsTable.jsx src/components/table/redemptions/modals/EditRedemptionModal.jsx src/hooks/redemptions/useRedemptionsData.jsx src/components/settings/personal/cards/CheckinCalendar.jsx src/components/settings/personal/components/UserInfoHeader.jsx src/pages/Setting/Operation/SettingsCheckin.jsx src/pages/Setting/Operation/SettingsCreditLimit.jsx src/pages/Setting/Payment/SettingsGeneralPayment.jsx src/pages/Setting/Payment/SettingsPaymentGateway.jsx src/pages/Setting/Payment/SettingsPaymentGatewayStripe.jsx src/pages/Setting/Payment/SettingsPaymentGatewayWaffo.jsx src/pages/Setting/Payment/SettingsPaymentGatewayWaffoPancake.jsx src/pages/Setting/Payment/SettingsPaymentGatewayCreem.jsx src/components/table/users/UsersColumnDefs.jsx src/components/table/users/UsersTable.jsx src/components/table/users/modals/EditUserModal.jsx src/components/table/usage-logs/modals/UserInfoModal.jsx
```

预期：PASS。

- [ ] **步骤 4：静态扫描账户余额链路**

使用 `search` 工具，不要用 shell grep：

- 搜索 `QuotaPerUnit|getQuotaPerUnit|quotaPerUnit|renderQuota|logger.LogQuota|logger.FormatQuota|DecreaseUserQuota|IncreaseUserQuota|DeltaUpdateUserQuota`。
- 对每个命中分类：账户余额链路必须已迁移；非账户余额用量链路允许保留。
- 搜索 Kyren / Creem / redemptions / checkin / topup 文件中 `encoding/json|json.Marshal|json.Unmarshal|json.NewDecoder`，确认业务 JSON 未直接调用标准库。
- 搜索 `ROUND\(|JSON_EXTRACT|jsonb|::json|ALTER COLUMN|AUTO_INCREMENT|SERIAL|GROUP_CONCAT|STRING_AGG|Update\(("|`)quota|UpdateColumn(s)?\(("|`)quota|Updates\([^\n]*("|`)quota|gorm.Expr\("quota|increaseUserQuota\(|decreaseUserQuota\(|increaseUserQuotaTx\(|decreaseUserQuotaTx\(|updateUserQuota|addNewRecord\(BatchUpdateTypeUserQuota|BatchUpdateTypeUserQuota`，确认迁移没有使用数据库专属 SQL / JSON / ROUND 函数，且所有 `users.quota` 写入都归类为账户余额 helper 或明确账户余额入口。
- 检查回滚 runbook：计划和规格中必须包含「恢复迁移前数据库备份」「部署旧版本服务」「验证用户余额和充值入口」「不实现自动反向迁移」。
- 硬门禁：任何账户余额链路残留命中、业务 JSON 标准库调用命中、数据库方言风险命中、回滚 runbook 缺项或无法明确分类的命中，必须回到对应任务修复，重跑该任务测试和本静态扫描。只有扫描结果全部为「非账户余额允许保留」或「无命中」后，才能进入 whitespace 检查。

- [ ] **步骤 5：whitespace 检查**

运行：

```bash
git diff --check
```

预期：无 whitespace error。CRLF/LF warning 需要记录；实际 whitespace error 必须修复。

- [ ] **步骤 6：最终提交**

所有前置任务必须已经提交；如果最终扫描触发回修，则提交回修：

```bash
git status --short
git add docs/superpowers/specs/2026-05-30-account-balance-cents-migration-design.md docs/superpowers/plans/2026-05-30-account-balance-cents-migration.md model controller service setting web/default web/classic main.go
git commit -m "fix(balance): 完成账户余额分制迁移"
```

- [ ] **步骤 7：请求代码审查**

派发至少 3 个只读审查子代理，方向：

1. 后端迁移 / 数据一致性 / 三库兼容。
2. 支付与账单 / 幂等 / webhook / 补单。
3. default + classic 前端 / i18n / 金额 UX。

所有 `[必须修复]` 必须修复并复审通过后，才能宣称完成。
