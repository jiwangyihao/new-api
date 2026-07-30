package model

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCreateUserSubscriptionFromPlanTx_DistributorSnapshot(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7101, Username: "snapshot_user", Status: common.UserStatusEnabled}).Error)
	businessCode := "basic_monthly"
	plan := &SubscriptionPlan{
		Id:                7201,
		Title:             "Basic",
		DurationUnit:      SubscriptionDurationMonth,
		DurationValue:     1,
		Enabled:           true,
		MonthlyTokenLimit: 1_000_000_000,
		ConcurrencyLimit:  1,
		PublicVisible:     true,
		RewardEligible:    true,
		BusinessCode:      &businessCode,
	}
	var sub *UserSubscription
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		created, err := CreateUserSubscriptionFromPlanTx(tx, 7101, plan, "order")
		sub = created
		return err
	}))
	require.NotNil(t, sub)
	assert.Equal(t, int64(1_000_000_000), sub.TokenLimit)
	assert.Equal(t, int64(0), sub.TokenUsed)
	assert.Equal(t, 1, sub.ConcurrencyLimit)
	assert.Equal(t, "order", sub.GrantReason)
}

func TestPreConsumeUserSubscriptionUsesLivePlanConcurrencyAndQueueCapacity(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	realTimeCode := "realtime-plan-limits"
	require.NoError(t, DB.Create(&User{Id: 7102, Username: "realtime_plan_user", Status: common.UserStatusEnabled, AffCode: "aff7102"}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7202, Title: "Realtime", Enabled: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, QueueCapacity: 2, BusinessCode: &realTimeCode}).Error)
	staleSub := &UserSubscription{Id: 7203, UserId: 7102, PlanId: 7202, Status: "active", TokenLimit: 1000, TokenUsed: 0, ConcurrencyLimit: 99, EndTime: common.GetTimestamp() + 3600, GrantReason: "order"}
	require.NoError(t, DB.Create(staleSub).Error)

	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", 7202).Updates(map[string]any{"concurrency_limit": 7, "queue_capacity": 13}).Error)
	InvalidateSubscriptionPlanCache(7202)

	pre, err := PreConsumeUserSubscription("live-plan-limits", 7102, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 7203, pre.UserSubscriptionId)
	assert.Equal(t, 7, pre.ConcurrencyLimit)
	assert.Equal(t, 13, pre.QueueCapacity)
}

func TestPreConsumeUserSubscriptionRefreshesPlanLimitsWhenPrimarySelectionCached(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	realTimeCode := "cached-plan-limits"
	require.NoError(t, DB.Create(&User{Id: 7103, Username: "cached_plan_user", Status: common.UserStatusEnabled, AffCode: "aff7103"}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7204, Title: "Cached", Enabled: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 2, QueueCapacity: 3, BusinessCode: &realTimeCode}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7205, UserId: 7103, PlanId: 7204, Status: "active", TokenLimit: 1000, TokenUsed: 0, ConcurrencyLimit: 99, EndTime: common.GetTimestamp() + 3600, GrantReason: "order"}).Error)

	first, err := PreConsumeUserSubscription("cached-plan-limits-first", 7103, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 2, first.ConcurrencyLimit)
	assert.Equal(t, 3, first.QueueCapacity)

	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", 7204).Updates(map[string]any{"concurrency_limit": 8, "queue_capacity": 21}).Error)
	InvalidateSubscriptionPlanCache(7204)

	second, err := PreConsumeUserSubscription("cached-plan-limits-second", 7103, "gpt-4o", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 8, second.ConcurrencyLimit)
	assert.Equal(t, 21, second.QueueCapacity)
}

func TestCreditBalancePreConsumeRejectsExhaustionAndAllowsSettlementDebt(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.AutoMigrate(&CreditBalanceLedger{}))
	require.NoError(t, DB.Create(&User{Id: 7104, Username: "credit_balance_consumer", Status: common.UserStatusEnabled, AffCode: "aff7104"}).Error)
	creditCode := "credit_balance_consume"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7206, Title: "Credit 余额套餐", EntitlementType: SubscriptionEntitlementCreditBalance, Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 0, ConcurrencyLimit: 2, BusinessCode: &creditCode}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7207, UserId: 7104, PlanId: 7206, EntitlementType: SubscriptionEntitlementCreditBalance, Status: "active", TokenLimit: 100, TokenUsed: 90, EndTime: 0, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	hasAllowed, err := HasActiveUserSubscriptionForModel(7104, "gpt-4o")
	require.NoError(t, err)
	assert.True(t, hasAllowed)
	hasDenied, err := HasActiveUserSubscriptionForModel(7104, "claude-3-7-sonnet")
	require.NoError(t, err)
	assert.False(t, hasDenied)

	_, err = PreConsumeUserSubscription("credit-balance-model-denied", 7104, "claude-3-7-sonnet", 0, 1)
	require.ErrorContains(t, err, "subscription model not allowed")
	var deniedRecordCount int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", "credit-balance-model-denied").Count(&deniedRecordCount).Error)
	assert.Zero(t, deniedRecordCount)

	pre, err := PreConsumeUserSubscription("credit-balance-preconsume", 7104, "gpt-4o", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 7207, pre.UserSubscriptionId)
	assert.Equal(t, int64(100), pre.TokenUsedAfter)

	_, err = PreConsumeUserSubscription("credit-balance-exhausted", 7104, "gpt-4o", 0, 1)
	require.ErrorContains(t, err, "subscription token quota insufficient")
	require.NoError(t, PostConsumeUserSubscriptionTokenDelta(7207, 5))
	var balance UserSubscription
	require.NoError(t, DB.First(&balance, 7207).Error)
	assert.Equal(t, int64(105), balance.TokenUsed)

	_, err = PreConsumeUserSubscription("credit-balance-debt", 7104, "gpt-4o", 0, 1)
	require.ErrorContains(t, err, "subscription token quota insufficient")
	require.NoError(t, RefundSubscriptionPreConsume("credit-balance-preconsume"))

	require.NoError(t, DB.First(&balance, 7207).Error)
	assert.Equal(t, int64(95), balance.TokenUsed)
	var ledgerCount int64
	require.NoError(t, DB.Model(&CreditBalanceLedger{}).Where("user_id = ?", 7104).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
}

func TestActiveCreditBalanceInsufficientDoesNotFallBackToTimedSubscription(t *testing.T) {
	for _, test := range []struct {
		name           string
		tokenLimit     int64
		tokenUsed      int64
		requiredTokens int64
	}{
		{name: "positive but insufficient", tokenLimit: 100, tokenUsed: 95, requiredTokens: 10},
		{name: "explicit zero balance", tokenLimit: 0, tokenUsed: 0, requiredTokens: 1},
		{name: "exhausted", tokenLimit: 100, tokenUsed: 100, requiredTokens: 1},
		{name: "settlement debt", tokenLimit: 100, tokenUsed: 105, requiredTokens: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			ensureSubscriptionPreConsumeRecordTableForTest(t)
			ClearPrimaryBillableSubscriptionCacheForTest()
			user := User{Id: 7105, Username: "active_credit_insufficient", Status: common.UserStatusEnabled, AffCode: "aff7105"}
			setting := user.GetSetting()
			setting.ActiveSubscriptionId = 7209
			user.SetSetting(setting)
			require.NoError(t, DB.Create(&user).Error)
			creditCode := "active_credit_insufficient_balance"
			timedCode := "active_credit_insufficient_timed"
			require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7208, Title: "Credit 余额套餐", EntitlementType: SubscriptionEntitlementCreditBalance, Enabled: true, ModelLimits: "gpt-4o", ConcurrencyLimit: 2, BusinessCode: &creditCode}).Error)
			require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7210, Title: "Timed fallback", EntitlementType: SubscriptionEntitlementTimed, Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 1000, ConcurrencyLimit: 2, BusinessCode: &timedCode}).Error)
			require.NoError(t, DB.Create(&UserSubscription{Id: 7209, UserId: user.Id, PlanId: 7208, EntitlementType: SubscriptionEntitlementCreditBalance, Status: "active", TokenLimit: test.tokenLimit, TokenUsed: test.tokenUsed, EndTime: 0, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
			require.NoError(t, DB.Create(&UserSubscription{Id: 7211, UserId: user.Id, PlanId: 7210, EntitlementType: SubscriptionEntitlementTimed, Status: "active", TokenLimit: 1000, TokenUsed: 0, EndTime: common.GetTimestamp() + 3600, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

			_, err := PreConsumeUserSubscription(test.name, user.Id, "gpt-4o", 0, test.requiredTokens)

			require.ErrorContains(t, err, "subscription token quota insufficient")
			var timed UserSubscription
			require.NoError(t, DB.First(&timed, 7211).Error)
			assert.Equal(t, int64(0), timed.TokenUsed)
			var recordCount int64
			require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", test.name).Count(&recordCount).Error)
			assert.Zero(t, recordCount)
		})
	}
}

func TestPreConsumeDoesNotFallBackWhenActiveSubscriptionDisallowsModel(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	user := User{Id: 7106, Username: "model_fallback", Status: common.UserStatusEnabled, AffCode: "aff7106"}
	setting := user.GetSetting()
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyActiveFallback
	setting.ActiveSubscriptionId = 7210
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	firstCode := "model_fallback_first"
	secondCode := "model_fallback_second"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7211, Title: "First", Enabled: true, ModelLimits: "claude-3-7-sonnet", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &firstCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7212, Title: "Second", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &secondCode}).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7210, UserId: 7106, PlanId: 7211, Status: "active", TokenLimit: 100, EndTime: now + 3600, GrantReason: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7213, UserId: 7106, PlanId: 7212, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: SubscriptionGrantOrder}).Error)

	primed, err := PreConsumeUserSubscription("model-fallback-prime-cache", 7106, "claude-3-7-sonnet", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, 7210, primed.UserSubscriptionId)
	cachedValue, cached := primaryBillableSubscriptionCache.Load(primaryBillableSubscriptionCacheKey(7106))
	require.True(t, cached)
	cachedEntry, ok := cachedValue.(primaryBillableSubscriptionCacheEntry)
	require.True(t, ok)
	assert.Equal(t, 7210, cachedEntry.loaded.Subscription.Id)
	persistedSetting, err := GetUserSetting(7106, true)
	require.NoError(t, err)
	assert.Equal(t, 7210, persistedSetting.ActiveSubscriptionId)

	_, err = PreConsumeUserSubscription("model-fallback-first", 7106, "gpt-4o", 0, 5)
	require.ErrorContains(t, err, "subscription model not allowed")

	_, err = PreConsumeUserSubscription("model-fallback-cached", 7106, "gpt-4o", 0, 5)
	require.ErrorContains(t, err, "subscription model not allowed")
	var fallback UserSubscription
	require.NoError(t, DB.First(&fallback, 7213).Error)
	assert.Zero(t, fallback.TokenUsed)
}

func TestSubscriptionSelfSummaryIncludesModelRestrictedPlan(t *testing.T) {
	truncateTables(t)
	ClearPrimaryBillableSubscriptionCacheForTest()
	require.NoError(t, DB.AutoMigrate(&GPTAbuseSignalLog{}, &GPTAbuseUserSuspension{}, &GPTAbuseWarningReset{}, &GPTAbuseRepeatBlockLog{}))
	require.NoError(t, DB.Create(&User{Id: 7107, Username: "restricted_summary", Status: common.UserStatusEnabled, AffCode: "aff7107", Setting: `{"active_subscription_id":7214}`}).Error)
	businessCode := "restricted_summary_plan"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7215, Title: "Restricted", Enabled: true, ModelLimits: "gpt-4o", MonthlyTokenLimit: 100, ConcurrencyLimit: 3, BusinessCode: &businessCode}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7214, UserId: 7107, PlanId: 7215, Status: "active", TokenLimit: 100, EndTime: common.GetTimestamp() + 3600, GrantReason: SubscriptionGrantOrder}).Error)

	summary, err := GetSubscriptionSelfSummary(7107)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.ActiveCount)
	assert.Equal(t, 7214, summary.SubscriptionId)
	assert.Equal(t, "Restricted", summary.PrimaryPlanTitle)
}

func TestCreditBalanceZeroLimitIsNotUnlimited(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.Create(&User{Id: 7105, Username: "credit_balance_zero", Status: common.UserStatusEnabled, AffCode: "aff7105"}).Error)
	creditCode := "credit_balance_zero"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7208, Title: "Zero Credit", EntitlementType: SubscriptionEntitlementCreditBalance, Enabled: true, ConcurrencyLimit: 2, BusinessCode: &creditCode}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7209, UserId: 7105, PlanId: 7208, EntitlementType: SubscriptionEntitlementCreditBalance, Status: "active", TokenLimit: 0, TokenUsed: 0, EndTime: 0, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	hasActive, err := HasActiveUserSubscription(7105)
	require.NoError(t, err)
	assert.False(t, hasActive)

	_, err = PreConsumeUserSubscription("credit-balance-zero", 7105, "gpt-4o", 0, 1)
	require.ErrorContains(t, err, "subscription token quota insufficient")
	var recordCount int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", "credit-balance-zero").Count(&recordCount).Error)
	assert.Zero(t, recordCount)
}

func TestCreateUserSubscriptionFromPlanTx_IgnoresHistoricalPurchaseLimitOnRenewal(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7301, Username: "extend_user", Status: common.UserStatusEnabled}).Error)
	businessCode := "extend_daily"
	plan := &SubscriptionPlan{
		Id:                 7302,
		Title:              "Daily",
		DurationUnit:       SubscriptionDurationDay,
		DurationValue:      1,
		Enabled:            true,
		MonthlyTokenLimit:  1_000,
		ConcurrencyLimit:   2,
		PublicVisible:      true,
		RewardEligible:     true,
		BusinessCode:       &businessCode,
		MaxPurchasePerUser: 1,
	}
	require.NoError(t, DB.Create(plan).Error)

	var first *UserSubscription
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		created, err := CreateUserSubscriptionFromPlanTx(tx, 7301, plan, "order")
		first = created
		return err
	}))
	require.NotNil(t, first)
	firstEnd := first.EndTime
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", first.Id).Update("token_used", int64(123)).Error)

	var second *UserSubscription
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		created, err := CreateUserSubscriptionFromPlanTx(tx, 7301, plan, "order")
		second = created
		return err
	}))
	require.NotNil(t, second)
	assert.Equal(t, first.Id, second.Id)
	assert.Equal(t, firstEnd+86400, second.EndTime)
	assert.Equal(t, int64(123), second.TokenUsed)
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", 7301, 7302).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestCreateUserSubscriptionFromPlanTx_ExtendsActiveSamePlanWhenUnlimited(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7303, Username: "extend_unlimited_user", Status: common.UserStatusEnabled}).Error)
	businessCode := "extend_unlimited_daily"
	plan := &SubscriptionPlan{
		Id:                7304,
		Title:             "Daily Unlimited",
		DurationUnit:      SubscriptionDurationDay,
		DurationValue:     1,
		Enabled:           true,
		MonthlyTokenLimit: 1_000,
		ConcurrencyLimit:  2,
		PublicVisible:     true,
		RewardEligible:    true,
		BusinessCode:      &businessCode,
	}
	require.NoError(t, DB.Create(plan).Error)

	var first *UserSubscription
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		created, err := CreateUserSubscriptionFromPlanTx(tx, 7303, plan, "order")
		first = created
		return err
	}))
	require.NotNil(t, first)
	firstEnd := first.EndTime
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", first.Id).Update("token_used", int64(123)).Error)

	var second *UserSubscription
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		created, err := CreateUserSubscriptionFromPlanTx(tx, 7303, plan, "order")
		second = created
		return err
	}))
	require.NotNil(t, second)

	assert.Equal(t, first.Id, second.Id)
	assert.Equal(t, firstEnd+86400, second.EndTime)
	assert.Equal(t, int64(123), second.TokenUsed)
	assert.Equal(t, "order", second.GrantReason)
	assert.Equal(t, "order", second.Source)
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", 7303, 7304).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestTimedSubscriptionConversionIdentityRejectsTrialAndMonthlyInvite(t *testing.T) {
	timedPlan := &SubscriptionPlan{EntitlementType: SubscriptionEntitlementTimed}
	assert.True(t, IsTimedSubscriptionConversionIdentityEligible(&UserSubscription{EntitlementType: SubscriptionEntitlementTimed, GrantReason: SubscriptionGrantOrder}, timedPlan))
	assert.False(t, IsTimedSubscriptionConversionIdentityEligible(&UserSubscription{EntitlementType: SubscriptionEntitlementTimed, GrantReason: "trial_code"}, timedPlan))
	assert.False(t, IsTimedSubscriptionConversionIdentityEligible(&UserSubscription{EntitlementType: SubscriptionEntitlementTimed, GrantReason: "invite_trial"}, timedPlan))
	assert.False(t, IsTimedSubscriptionConversionIdentityEligible(&UserSubscription{EntitlementType: SubscriptionEntitlementTimed, GrantReason: SubscriptionGrantMonthlyInviteEntitlement}, timedPlan))
	assert.False(t, IsTimedSubscriptionConversionIdentityEligible(&UserSubscription{EntitlementType: SubscriptionEntitlementCreditBalance, GrantReason: SubscriptionGrantOrder}, timedPlan))
	assert.False(t, IsTimedSubscriptionConversionIdentityEligible(&UserSubscription{EntitlementType: SubscriptionEntitlementTimed, GrantReason: SubscriptionGrantOrder}, &SubscriptionPlan{EntitlementType: SubscriptionEntitlementTimed, InviteTrial: true}))
}

func TestCreateUserSubscriptionPropagatesTimedIdentityAndRejectsCreditBalancePlan(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&UserSubscription{}))

	timedPlan := &SubscriptionPlan{Id: 1, EntitlementType: "", DurationUnit: SubscriptionDurationMonth, DurationValue: 1}
	var timed *UserSubscription
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		created, createErr := CreateUserSubscriptionFromPlanTx(tx, 1, timedPlan, SubscriptionGrantOrder)
		timed = created
		return createErr
	}))
	require.NotNil(t, timed)
	assert.Equal(t, SubscriptionEntitlementTimed, timed.EntitlementType)
	assert.Nil(t, timed.SingletonKey)

	creditPlan := &SubscriptionPlan{Id: 2, EntitlementType: SubscriptionEntitlementCreditBalance, DurationUnit: SubscriptionDurationMonth, DurationValue: 1}
	err = db.Transaction(func(tx *gorm.DB) error {
		_, createErr := CreateUserSubscriptionFromPlanTx(tx, 1, creditPlan, SubscriptionGrantOrder)
		return createErr
	})
	require.ErrorContains(t, err, "dedicated credit service")
}

func TestOrdinaryAdminLifecycleCannotDestroyCreditBalanceEntitlement(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() { DB = originalDB })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	require.NoError(t, DB.AutoMigrate(&UserSubscription{}))
	require.NoError(t, DB.Create(&UserSubscription{UserId: 1, EntitlementType: SubscriptionEntitlementCreditBalance, Status: "active"}).Error)
	var sub UserSubscription
	require.NoError(t, DB.First(&sub).Error)

	_, err = AdminInvalidateUserSubscription(sub.Id)
	require.ErrorContains(t, err, "不能通过普通接口失效")
	_, err = AdminDeleteUserSubscription(sub.Id)
	require.ErrorContains(t, err, "不能通过普通接口删除")

	require.NoError(t, DB.First(&sub, sub.Id).Error)
	assert.Equal(t, "active", sub.Status)
}

func TestCreditBalanceSingletonIndexesGenerateForMySQLAndPostgres(t *testing.T) {
	baseDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	baseStatement := baseDB.Session(&gorm.Session{DryRun: true}).Find(&[]SubscriptionPlan{}).Statement

	tests := []struct {
		name      string
		dialector gorm.Dialector
	}{
		{
			name: "mysql",
			dialector: mysql.New(mysql.Config{
				Conn:                      baseStatement.ConnPool,
				SkipInitializeWithVersion: true,
			}),
		},
		{
			name: "postgres",
			dialector: postgres.New(postgres.Config{
				DSN:                  "host=localhost user=test dbname=test sslmode=disable",
				PreferSimpleProtocol: true,
				Conn:                 baseStatement.ConnPool,
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := &sqlCaptureLogger{Interface: logger.Default.LogMode(logger.Silent)}
			db, openErr := gorm.Open(test.dialector, &gorm.Config{
				DryRun:                                   true,
				Logger:                                   capture,
				DisableForeignKeyConstraintWhenMigrating: true,
			})
			require.NoError(t, openErr)
			require.NoError(t, db.Migrator().CreateTable(&SubscriptionPlan{}, &UserSubscription{}))

			expectedColumns := map[string][]string{
				"idx_subscription_plans_singleton_key": {"singleton_key"},
				"idx_user_subscription_singleton":      {"user_id", "singleton_key"},
			}
			for indexName, columns := range expectedColumns {
				definitionPattern := regexp.MustCompile(`(?i)unique\s+(?:index\s+)?(?:if\s+not\s+exists\s+)?["` + "`" + `]?` + regexp.QuoteMeta(indexName) + `["` + "`" + `]?\s+(?:on\s+["` + "`" + `]?[^"` + "`" + `\s]+["` + "`" + `]?\s*)?\(([^)]*)\)`)
				matched := false
				for _, statement := range capture.statements {
					matches := definitionPattern.FindStringSubmatch(statement)
					if len(matches) != 2 {
						continue
					}
					definition := strings.ToLower(matches[1])
					matched = true
					for _, column := range columns {
						matched = matched && strings.Contains(definition, column)
					}
					if matched {
						break
					}
				}
				assert.True(t, matched, "expected UNIQUE index %s on %v; statements=%#v", indexName, columns, capture.statements)
			}
		})
	}
}

func TestCreditBalanceMySQL57MigrationDDLUsesStoredGeneratedColumns(t *testing.T) {
	constraints := creditBalanceMySQL57ConstraintDDL()
	require.Len(t, constraints, 2)

	expectedStatements := []string{
		"ALTER TABLE `subscription_plans` ADD COLUMN `credit_balance_identity_guard` TINYINT GENERATED ALWAYS AS (CASE WHEN `entitlement_type` = 'credit_balance' THEN 1 ELSE NULL END) STORED",
		"CREATE UNIQUE INDEX `idx_subscription_plans_credit_balance_identity` ON `subscription_plans` (`credit_balance_identity_guard`)",
		"ALTER TABLE `user_subscriptions` ADD COLUMN `credit_balance_identity_guard` BIGINT GENERATED ALWAYS AS (CASE WHEN `entitlement_type` = 'credit_balance' THEN `user_id` ELSE NULL END) STORED",
		"CREATE UNIQUE INDEX `idx_user_subscriptions_credit_balance_identity` ON `user_subscriptions` (`credit_balance_identity_guard`)",
	}
	actualStatements := make([]string, 0, len(expectedStatements))
	for _, constraint := range constraints {
		actualStatements = append(actualStatements, constraint.addColumnSQL, constraint.createIndexSQL)
	}
	require.Equal(t, expectedStatements, actualStatements)

	baseDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	baseStatement := baseDB.Session(&gorm.Session{DryRun: true}).Find(&[]SubscriptionPlan{}).Statement
	capture := &sqlCaptureLogger{Interface: logger.Default.LogMode(logger.Silent)}
	mysqlDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      baseStatement.ConnPool,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, Logger: capture})
	require.NoError(t, err)
	for _, statement := range actualStatements {
		require.NoError(t, mysqlDB.Exec(statement).Error)
	}
	require.Equal(t, expectedStatements, capture.statements)
}

func TestCreditBalancePostgres96MigrationDDLUsesPartialUniqueIndexes(t *testing.T) {
	statements := creditBalancePartialUniqueIndexDDL()
	require.Equal(t, []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS "idx_subscription_plans_credit_balance_identity" ON "subscription_plans" ("entitlement_type") WHERE "entitlement_type" = 'credit_balance'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "idx_user_subscriptions_credit_balance_identity" ON "user_subscriptions" ("user_id") WHERE "entitlement_type" = 'credit_balance'`,
	}, statements)

	baseDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	baseStatement := baseDB.Session(&gorm.Session{DryRun: true}).Find(&[]SubscriptionPlan{}).Statement
	capture := &sqlCaptureLogger{Interface: logger.Default.LogMode(logger.Silent)}
	postgresDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=localhost user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
		Conn:                 baseStatement.ConnPool,
	}), &gorm.Config{DryRun: true, Logger: capture})
	require.NoError(t, err)
	for _, statement := range statements {
		require.NoError(t, postgresDB.Exec(statement).Error)
	}
	require.Equal(t, statements, capture.statements)
}

func TestSubscriptionPlanBusinessCode_AllowsMultipleLegacyNulls(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7202, Title: "Legacy A", Enabled: true}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7203, Title: "Legacy B", Enabled: true}).Error)
}

func TestSubscriptionPlanBusinessCode_RejectsDuplicateNonEmpty(t *testing.T) {
	truncateTables(t)
	code := "basic_monthly"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7204, Title: "Basic A", Enabled: true, BusinessCode: &code}).Error)
	dup := "basic_monthly"
	require.Error(t, DB.Create(&SubscriptionPlan{Id: 7205, Title: "Basic B", Enabled: true, BusinessCode: &dup}).Error)
}

func TestCreditBalanceSingletonKeysEnforceDatabaseUniqueness(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}, &UserSubscription{}))

	require.NoError(t, db.Create(&SubscriptionPlan{Title: "Timed A", EntitlementType: SubscriptionEntitlementTimed}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{Title: "Timed B", EntitlementType: SubscriptionEntitlementTimed}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{Title: "Credit balance", EntitlementType: SubscriptionEntitlementCreditBalance}).Error)
	require.Error(t, db.Create(&SubscriptionPlan{Title: "Duplicate credit balance", EntitlementType: SubscriptionEntitlementCreditBalance}).Error)

	require.NoError(t, db.Create(&UserSubscription{UserId: 1, EntitlementType: SubscriptionEntitlementTimed}).Error)
	require.NoError(t, db.Create(&UserSubscription{UserId: 1, EntitlementType: SubscriptionEntitlementTimed}).Error)
	require.NoError(t, db.Create(&UserSubscription{UserId: 1, EntitlementType: SubscriptionEntitlementCreditBalance}).Error)
	require.Error(t, db.Create(&UserSubscription{UserId: 1, EntitlementType: SubscriptionEntitlementCreditBalance}).Error)
	require.NoError(t, db.Create(&UserSubscription{UserId: 2, EntitlementType: SubscriptionEntitlementCreditBalance}).Error)
}

func TestCreditBalanceDatabaseConstraintsRejectRawDuplicateIdentity(t *testing.T) {
	originalDB := DB
	originalUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true
	require.NoError(t, DB.AutoMigrate(&SubscriptionPlan{}, &UserSubscription{}))
	require.NoError(t, ensureCreditBalanceSingletonConstraints())
	require.NoError(t, ensureCreditBalanceSingletonConstraints())

	require.NoError(t, DB.Exec("INSERT INTO subscription_plans (title, entitlement_type, singleton_key) VALUES (?, ?, ?)", "Raw credit balance", SubscriptionEntitlementCreditBalance, creditBalancePlanSingletonKey).Error)
	require.Error(t, DB.Exec("INSERT INTO subscription_plans (title, entitlement_type) VALUES (?, ?)", "Duplicate raw credit balance with null key", SubscriptionEntitlementCreditBalance).Error)
	require.Error(t, DB.Exec("INSERT INTO subscription_plans (title, entitlement_type, singleton_key) VALUES (?, ?, ?)", "Duplicate raw credit balance with different key", SubscriptionEntitlementCreditBalance, "different-credit-plan").Error)
	require.NoError(t, DB.Exec("INSERT INTO subscription_plans (title, entitlement_type) VALUES (?, ?), (?, ?)", "Raw timed A", SubscriptionEntitlementTimed, "Raw timed B", SubscriptionEntitlementTimed).Error)
	require.Error(t, DB.Exec("UPDATE subscription_plans SET entitlement_type = ?, singleton_key = ? WHERE title = ?", SubscriptionEntitlementCreditBalance, "updated-credit-plan", "Raw timed A").Error)

	require.NoError(t, DB.Exec("INSERT INTO user_subscriptions (user_id, entitlement_type, singleton_key) VALUES (?, ?, ?)", 1, SubscriptionEntitlementCreditBalance, creditBalanceUserSingletonKey).Error)
	require.Error(t, DB.Exec("INSERT INTO user_subscriptions (user_id, entitlement_type) VALUES (?, ?)", 1, SubscriptionEntitlementCreditBalance).Error)
	require.Error(t, DB.Exec("INSERT INTO user_subscriptions (user_id, entitlement_type, singleton_key) VALUES (?, ?, ?)", 1, SubscriptionEntitlementCreditBalance, "different-credit-entitlement").Error)
	require.NoError(t, DB.Exec("INSERT INTO user_subscriptions (user_id, entitlement_type) VALUES (?, ?)", 2, SubscriptionEntitlementCreditBalance).Error)
	require.NoError(t, DB.Exec("INSERT INTO user_subscriptions (user_id, entitlement_type) VALUES (?, ?), (?, ?)", 1, SubscriptionEntitlementTimed, 1, SubscriptionEntitlementTimed).Error)
	require.Error(t, DB.Exec("UPDATE user_subscriptions SET entitlement_type = ?, singleton_key = ? WHERE user_id = ? AND entitlement_type = ?", SubscriptionEntitlementCreditBalance, "updated-credit-entitlement", 1, SubscriptionEntitlementTimed).Error)
}

func TestEnsureCreditBalanceSubscriptionPlanConcurrentInitializationConverges(t *testing.T) {
	originalDB := DB
	originalUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalUsingSQLite
	})

	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared&_pragma=busy_timeout(5000)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true
	require.NoError(t, DB.AutoMigrate(&SubscriptionPlan{}))

	const workers = 8
	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			errorsByWorker <- ensureCreditBalanceSubscriptionPlan()
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errorsByWorker)
	for workerErr := range errorsByWorker {
		require.NoError(t, workerErr)
	}

	var plans []SubscriptionPlan
	require.NoError(t, DB.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Find(&plans).Error)
	require.Len(t, plans, 1)
	require.NotNil(t, plans[0].SingletonKey)
	assert.Equal(t, creditBalancePlanSingletonKey, *plans[0].SingletonKey)
}

func TestEnsureSubscriptionPlanTableSQLiteCreatesCreditBalanceSchema(t *testing.T) {
	originalDB := DB
	originalUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true

	require.NoError(t, DB.AutoMigrate(&UserSubscription{}))
	require.NoError(t, ensureSubscriptionPlanTableSQLite())
	require.NoError(t, ensureCreditBalanceSingletonConstraints())
	require.NoError(t, ensureCreditBalanceSubscriptionPlan())
	require.NoError(t, ensureSubscriptionPlanTableSQLite())
	require.NoError(t, ensureCreditBalanceSingletonConstraints())
	require.NoError(t, ensureCreditBalanceSubscriptionPlan())

	for _, column := range []string{
		"entitlement_type",
		"singleton_key",
		"model_limits",
		"credit_balance_configured",
		"credit_balance_purchase_enabled",
		"credit_balance_redemption_enabled",
		"credit_balance_conversion_enabled",
		"unlimited_purchase_enabled",
		"timed_conversion_enabled",
	} {
		require.True(t, DB.Migrator().HasColumn(&SubscriptionPlan{}, column), column)
	}
	require.True(t, DB.Migrator().HasIndex("subscription_plans", "idx_subscription_plans_singleton_key"))
	require.True(t, DB.Migrator().HasIndex("user_subscriptions", "idx_user_subscription_singleton"))
	require.True(t, DB.Migrator().HasIndex("subscription_plans", creditBalancePlanIdentityIndex))
	require.True(t, DB.Migrator().HasIndex("user_subscriptions", creditBalanceUserIdentityIndex))

	var plans []SubscriptionPlan
	require.NoError(t, DB.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Find(&plans).Error)
	require.Len(t, plans, 1)
	plan := plans[0]
	assert.False(t, plan.CreditBalanceConfigured)
	assert.False(t, plan.CreditBalancePurchaseEnabled)
	assert.False(t, plan.CreditBalanceRedemptionEnabled)
	assert.False(t, plan.CreditBalanceConversionEnabled)
	assert.False(t, plan.PublicVisible)
	assert.False(t, plan.RewardEligible)
	assert.Equal(t, float64(0), plan.PriceAmount)
}

func TestEnsureSubscriptionPlanTableSQLiteUpgradesLegacyRowsToTimed(t *testing.T) {
	originalDB := DB
	originalUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true
	require.NoError(t, DB.Exec(`CREATE TABLE subscription_plans (id integer PRIMARY KEY, title varchar(128) NOT NULL, price_amount decimal(10,6) NOT NULL)`).Error)
	require.NoError(t, DB.Exec(`INSERT INTO subscription_plans (id, title, price_amount) VALUES (1, 'Legacy timed', 40)`).Error)

	require.NoError(t, ensureSubscriptionPlanTableSQLite())
	require.NoError(t, DB.AutoMigrate(&UserSubscription{}))
	require.NoError(t, ensureCreditBalanceSubscriptionPlan())

	var legacy SubscriptionPlan
	require.NoError(t, DB.First(&legacy, 1).Error)
	assert.Equal(t, SubscriptionEntitlementTimed, legacy.EntitlementType)
	assert.Nil(t, legacy.SingletonKey)
	var count int64
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestEnsureSubscriptionPlanTableSQLite_CreatesBusinessCodeUniqueIndexOnFreshTable(t *testing.T) {
	originalDB := DB
	originalUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true

	require.NoError(t, ensureSubscriptionPlanTableSQLite())
	require.True(t, DB.Migrator().HasIndex("subscription_plans", "idx_subscription_plans_business_code"))
	require.True(t, DB.Migrator().HasColumn(&SubscriptionPlan{}, "kyren_product_id"))
	require.True(t, DB.Migrator().HasColumn(&SubscriptionPlan{}, "gpt_abuse_warning_limit"))

	code := "basic_monthly"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7210, Title: "Basic A", Enabled: true, BusinessCode: &code}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7212, Title: "Kyren Fresh", Enabled: true, KyrenProductId: "prod_fresh"}).Error)
	var freshPlan SubscriptionPlan
	require.NoError(t, DB.First(&freshPlan, 7212).Error)
	assert.Equal(t, "prod_fresh", freshPlan.KyrenProductId)
	dup := "basic_monthly"
	require.Error(t, DB.Create(&SubscriptionPlan{Id: 7211, Title: "Basic B", Enabled: true, BusinessCode: &dup}).Error)
}

func TestEnsureSubscriptionPlanTableSQLite_AddsKyrenProductIdToLegacyTable(t *testing.T) {
	originalDB := DB
	originalUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true

	require.NoError(t, DB.Exec(`CREATE TABLE subscription_plans (
		id integer PRIMARY KEY,
		title varchar(128) NOT NULL,
		subtitle varchar(255) DEFAULT '',
		price_amount decimal(10,6) NOT NULL,
		currency varchar(8) NOT NULL DEFAULT 'USD',
		duration_unit varchar(16) NOT NULL DEFAULT 'month',
		duration_value integer NOT NULL DEFAULT 1,
		custom_seconds bigint NOT NULL DEFAULT 0,
		enabled numeric DEFAULT 1,
		sort_order integer DEFAULT 0,
		stripe_price_id varchar(128) DEFAULT '',
		creem_product_id varchar(128) DEFAULT '',
		max_purchase_per_user integer DEFAULT 0,
		upgrade_group varchar(64) DEFAULT '',
		total_amount bigint NOT NULL DEFAULT 0,
		monthly_token_limit bigint NOT NULL DEFAULT 0,
		concurrency_limit integer NOT NULL DEFAULT 0,
		queue_capacity integer NOT NULL DEFAULT 0,
		is_trial numeric DEFAULT 0,
		invite_trial numeric DEFAULT 0,
		public_visible numeric DEFAULT 1,
		trial_duration_hours integer NOT NULL DEFAULT 0,
		reward_eligible numeric DEFAULT 1,
		business_code varchar(64) DEFAULT NULL,
		quota_reset_period varchar(16) DEFAULT 'never',
		quota_reset_custom_seconds bigint DEFAULT 0,
		created_at bigint,
		updated_at bigint
	)`).Error)
	require.NoError(t, DB.Exec(`INSERT INTO subscription_plans (id, title, price_amount, business_code, enabled) VALUES (7213, 'Legacy', 40, 'legacy_kyren', 1)`).Error)

	require.NoError(t, ensureSubscriptionPlanTableSQLite())
	require.True(t, DB.Migrator().HasColumn(&SubscriptionPlan{}, "kyren_product_id"))
	require.True(t, DB.Migrator().HasColumn(&SubscriptionPlan{}, "gpt_abuse_warning_limit"))

	var legacyPlan SubscriptionPlan
	require.NoError(t, DB.First(&legacyPlan, 7213).Error)
	assert.Equal(t, "", legacyPlan.KyrenProductId)
	assert.Equal(t, 0, legacyPlan.GPTAbuseWarningLimit)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", 7213).Update("kyren_product_id", "prod_legacy").Error)
	require.NoError(t, DB.First(&legacyPlan, 7213).Error)
	assert.Equal(t, "prod_legacy", legacyPlan.KyrenProductId)
}

func TestKyrenSnapshotColumnsAutoMigrateSQLite(t *testing.T) {
	originalDB := DB
	originalUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true

	require.NoError(t, DB.AutoMigrate(&TopUp{}, &SubscriptionOrder{}))
	require.True(t, DB.Migrator().HasColumn(&TopUp{}, "kyren_snapshot"))
	require.True(t, DB.Migrator().HasColumn(&SubscriptionOrder{}, "kyren_snapshot"))
	require.True(t, DB.Migrator().HasColumn(&SubscriptionOrder{}, "entitlement_snapshot"))

	require.NoError(t, DB.Create(&TopUp{
		Id:              7214,
		UserId:          7215,
		Amount:          4000,
		Money:           40,
		TradeNo:         "kyren_topup_snapshot",
		PaymentMethod:   PaymentMethodKyren,
		PaymentProvider: PaymentProviderKyren,
		Status:          common.TopUpStatusPending,
		KyrenSnapshot:   `{"product_id":"prod_topup","amount":"40.00","currency":"CNY"}`,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{
		Id:                  7216,
		UserId:              7217,
		PlanId:              7218,
		Money:               40,
		TradeNo:             "kyren_subscription_snapshot",
		PaymentMethod:       PaymentMethodKyren,
		PaymentProvider:     PaymentProviderKyren,
		Status:              common.TopUpStatusPending,
		KyrenSnapshot:       `{"product_id":"prod_subscription","amount":"40.00","currency":"CNY"}`,
		EntitlementSnapshot: `{"plan_id":7218,"queue_capacity":9}`,
	}).Error)

	var topUp TopUp
	require.NoError(t, DB.First(&topUp, 7214).Error)
	assert.Contains(t, topUp.KyrenSnapshot, "prod_topup")
	var order SubscriptionOrder
	require.NoError(t, DB.First(&order, 7216).Error)
	assert.Contains(t, order.KyrenSnapshot, "prod_subscription")
	assert.Contains(t, order.EntitlementSnapshot, "queue_capacity")
}

func seedDistributorSubscriptionPlanForTest(t *testing.T, id int, code string, tokenLimit int64) {
	t.Helper()
	plan := &SubscriptionPlan{Id: id, Title: code, Enabled: true, TotalAmount: 1, MonthlyTokenLimit: tokenLimit, ConcurrencyLimit: 1, BusinessCode: &code}
	require.NoError(t, DB.Create(plan).Error)
}

func ensureSubscriptionPreConsumeRecordTableForTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
}

func seedUserSubscriptionForDistributorTest(t *testing.T, id int, userId int, planId int, tokenLimit int64, tokenUsed int64, amountTotal int64, grantReason string) {
	t.Helper()
	sub := &UserSubscription{Id: id, UserId: userId, PlanId: planId, Status: "active", AmountTotal: amountTotal, TokenLimit: tokenLimit, TokenUsed: tokenUsed, EndTime: common.GetTimestamp() + 3600, GrantReason: grantReason}
	require.NoError(t, DB.Create(sub).Error)
}

func TestPreConsumeUserSubscriptionPrioritizesSameTierInviteRewardOverPaid(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7601, Username: "same_tier", Status: common.UserStatusEnabled, AffCode: "aff7601"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7602, "basic_monthly", 100)
	paidEnd := common.GetTimestamp() + 30*86400
	rewardEnd := common.GetTimestamp() + 3*86400
	require.NoError(t, DB.Create(&UserSubscription{Id: 7603, UserId: 7601, PlanId: 7602, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: paidEnd, GrantReason: "order", Source: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7604, UserId: 7601, PlanId: 7602, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: rewardEnd, GrantReason: "monthly_invite_entitlement", Source: "monthly_invite_entitlement"}).Error)

	pre, err := PreConsumeUserSubscription("same-tier-reward", 7601, "gpt-4o", 0, 6)

	require.NoError(t, err)
	assert.Equal(t, 7604, pre.UserSubscriptionId)
	var paid UserSubscription
	require.NoError(t, DB.First(&paid, 7603).Error)
	assert.Equal(t, int64(0), paid.TokenUsed)
	var reward UserSubscription
	require.NoError(t, DB.First(&reward, 7604).Error)
	assert.Equal(t, int64(6), reward.TokenUsed)
}

func TestPreConsumeUserSubscriptionUsesSelectedDifferentTierSubscription(t *testing.T) {
	truncateTables(t)
	user := User{Id: 7611, Username: "selected_tier", Status: common.UserStatusEnabled, AffCode: "aff7611"}
	require.NoError(t, DB.Create(&user).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7612, "basic_monthly", 100)
	seedDistributorSubscriptionPlanForTest(t, 7613, "pro_monthly", 100)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7614, UserId: 7611, PlanId: 7612, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: now + 3*86400, GrantReason: "monthly_invite_entitlement", Source: "monthly_invite_entitlement"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7615, UserId: 7611, PlanId: 7613, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: now + 30*86400, GrantReason: "order", Source: "order"}).Error)
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = 7615
	user.SetSetting(setting)
	require.NoError(t, DB.Save(&user).Error)

	pre, err := PreConsumeUserSubscription("selected-paid", 7611, "gpt-4o", 0, 5)

	require.NoError(t, err)
	assert.Equal(t, 7615, pre.UserSubscriptionId)
	var reward UserSubscription
	require.NoError(t, DB.First(&reward, 7614).Error)
	assert.Equal(t, int64(0), reward.TokenUsed)
}

func TestResetUserSubscriptionQuotaConsumesOneMonthFromPaidSubscription(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7621, Username: "reset_quota", Status: common.UserStatusEnabled, AffCode: "aff7621"}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7622, "basic_monthly", 100)
	now := common.GetTimestamp()
	paidEnd := now + 70*86400
	require.NoError(t, DB.Create(&UserSubscription{Id: 7623, UserId: 7621, PlanId: 7622, Status: "active", TokenLimit: 100, TokenUsed: 88, AmountUsed: 12, StartTime: now - 86400, EndTime: paidEnd, GrantReason: "order", Source: "order"}).Error)

	result, err := ResetUserSubscriptionQuota(7621, 7623)

	require.NoError(t, err)
	require.NotNil(t, result)
	var sub UserSubscription
	require.NoError(t, DB.First(&sub, 7623).Error)
	assert.Equal(t, int64(0), sub.TokenUsed)
	assert.Equal(t, int64(0), sub.AmountUsed)
	assert.InDelta(t, paidEnd-30*86400, sub.EndTime, 2)
	assert.NotZero(t, sub.LastResetTime)
}

func TestPublicSubscriptionSummaryTreatsRedemptionAsPaid(t *testing.T) {
	truncateTables(t)
	code := "redemption_paid_summary"
	plan := &SubscriptionPlan{Id: 7661, Title: "Redemption Paid", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
	require.NoError(t, DB.Create(plan).Error)
	now := common.GetTimestamp()
	sub := &UserSubscription{Id: 7662, UserId: 7663, PlanId: 7661, Status: "active", TokenLimit: 100, TokenUsed: 99, StartTime: now - 86400, EndTime: now + 70*86400, GrantReason: "redemption", Source: "redemption"}

	summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: sub, Plan: plan}}, sub.Id, now)

	require.Len(t, summaries, 1)
	require.NotNil(t, summaries[0].Subscription)
	assert.Equal(t, "paid", summaries[0].Subscription.SourceLabel)
	assert.True(t, summaries[0].Subscription.CanResetQuota)
}

func TestInvitationRewardCanResetWithSameTierRedemptionPayer(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7664, Username: "reward_redemption_reset", Status: common.UserStatusEnabled, AffCode: "aff7664"}).Error)
	code := "reward_redemption_tier"
	plan := &SubscriptionPlan{Id: 7665, Title: "Reward Redemption Tier", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
	require.NoError(t, DB.Create(plan).Error)
	now := common.GetTimestamp()
	redemptionEnd := now + 70*86400
	rewardEnd := now + 3*86400
	require.NoError(t, DB.Create(&UserSubscription{Id: 7666, UserId: 7664, PlanId: 7665, Status: "active", TokenLimit: 100, TokenUsed: 0, StartTime: now - 86400, EndTime: redemptionEnd, GrantReason: "redemption", Source: "redemption"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7667, UserId: 7664, PlanId: 7665, Status: "active", TokenLimit: 100, TokenUsed: 88, AmountUsed: 12, StartTime: now - 86400, EndTime: rewardEnd, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)

	summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: &UserSubscription{Id: 7666, UserId: 7664, PlanId: 7665, Status: "active", EndTime: redemptionEnd, GrantReason: "redemption", Source: "redemption"}, Plan: plan}, {Subscription: &UserSubscription{Id: 7667, UserId: 7664, PlanId: 7665, Status: "active", EndTime: rewardEnd, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}, Plan: plan}}, 7667, now)
	require.Len(t, summaries, 2)
	require.NotNil(t, summaries[1].Subscription)
	assert.True(t, summaries[1].Subscription.CanResetQuota)
	assert.Equal(t, rewardEnd+(redemptionEnd-now), summaries[1].Subscription.EffectiveEndTime)

	result, err := ResetUserSubscriptionQuota(7664, 7667)

	require.NoError(t, err)
	require.NotNil(t, result)
	var reward UserSubscription
	require.NoError(t, DB.First(&reward, 7667).Error)
	assert.Equal(t, int64(0), reward.TokenUsed)
	assert.Equal(t, int64(0), reward.AmountUsed)
	assert.InDelta(t, rewardEnd, reward.EndTime, 2)
	var payer UserSubscription
	require.NoError(t, DB.First(&payer, 7666).Error)
	assert.InDelta(t, redemptionEnd-30*86400, payer.EndTime, 2)
}

func TestResetUserSubscriptionQuotaConsumesOneMonthFromRedemptionSubscription(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7668, Username: "reset_redemption", Status: common.UserStatusEnabled, AffCode: "aff7668"}).Error)
	code := "reset_redemption_tier"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7669, Title: "Reset Redemption", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	now := common.GetTimestamp()
	end := now + 70*86400
	require.NoError(t, DB.Create(&UserSubscription{Id: 7670, UserId: 7668, PlanId: 7669, Status: "active", TokenLimit: 100, TokenUsed: 88, AmountUsed: 12, StartTime: now - 86400, EndTime: end, GrantReason: "redemption", Source: "redemption"}).Error)

	result, err := ResetUserSubscriptionQuota(7668, 7670)

	require.NoError(t, err)
	require.NotNil(t, result)
	var sub UserSubscription
	require.NoError(t, DB.First(&sub, 7670).Error)
	assert.Equal(t, int64(0), sub.TokenUsed)
	assert.Equal(t, int64(0), sub.AmountUsed)
	assert.InDelta(t, end-30*86400, sub.EndTime, 2)
	assert.NotZero(t, sub.LastResetTime)
}

func TestPublicSubscriptionSummaryDoesNotResetInactivePaidEquivalentSubscriptions(t *testing.T) {
	truncateTables(t)
	code := "inactive_redemption_reset"
	plan := &SubscriptionPlan{Id: 7671, Title: "Inactive Redemption", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
	now := common.GetTimestamp()
	expired := &UserSubscription{Id: 7672, UserId: 7673, PlanId: 7671, Status: "active", EndTime: now - 60, GrantReason: "redemption", Source: "redemption"}
	cancelled := &UserSubscription{Id: 7674, UserId: 7673, PlanId: 7671, Status: "cancelled", EndTime: now + 70*86400, GrantReason: "redemption", Source: "redemption"}

	summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: expired, Plan: plan}, {Subscription: cancelled, Plan: plan}}, 0, now)

	require.Len(t, summaries, 2)
	assert.False(t, summaries[0].Subscription.CanResetQuota)
	assert.False(t, summaries[1].Subscription.CanResetQuota)
}

func TestInvitationRewardIgnoresInactiveRedemptionPaidRemainder(t *testing.T) {
	truncateTables(t)
	code := "inactive_redemption_payer"
	plan := &SubscriptionPlan{Id: 7675, Title: "Inactive Redemption Payer", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
	now := common.GetTimestamp()
	cancelledPaid := &UserSubscription{Id: 7676, UserId: 7677, PlanId: 7675, Status: "cancelled", EndTime: now + 70*86400, GrantReason: "redemption", Source: "redemption"}
	expiredPaid := &UserSubscription{Id: 7678, UserId: 7677, PlanId: 7675, Status: "active", EndTime: now - 60, GrantReason: "redemption", Source: "redemption"}
	reward := &UserSubscription{Id: 7679, UserId: 7677, PlanId: 7675, Status: "active", EndTime: now + 3*86400, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}

	summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: cancelledPaid, Plan: plan}, {Subscription: expiredPaid, Plan: plan}, {Subscription: reward, Plan: plan}}, reward.Id, now)

	require.Len(t, summaries, 3)
	require.NotNil(t, summaries[2].Subscription)
	assert.False(t, summaries[2].Subscription.CanResetQuota)
	assert.Equal(t, reward.EndTime, summaries[2].Subscription.EffectiveEndTime)
}

func TestAdminPaidSubscriptionIsPaidEquivalentForReset(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7680, Username: "admin_paid_reset", Status: common.UserStatusEnabled, AffCode: "aff7680"}).Error)
	code := "admin_paid_reset_tier"
	plan := &SubscriptionPlan{Id: 7681, Title: "Admin Paid", Enabled: true, PriceAmount: 80, Currency: "CNY", TotalAmount: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}
	require.NoError(t, DB.Create(plan).Error)
	now := common.GetTimestamp()
	end := now + 70*86400
	sub := &UserSubscription{Id: 7682, UserId: 7680, PlanId: 7681, Status: "active", TokenLimit: 100, TokenUsed: 88, AmountUsed: 12, StartTime: now - 3600, EndTime: end, GrantReason: "admin", Source: "admin"}
	require.NoError(t, DB.Create(sub).Error)

	summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: sub, Plan: plan}}, sub.Id, now)
	require.Len(t, summaries, 1)
	require.NotNil(t, summaries[0].Subscription)
	assert.Equal(t, "paid", summaries[0].Subscription.SourceLabel)
	assert.True(t, summaries[0].Subscription.CanResetQuota)

	result, err := ResetUserSubscriptionQuota(7680, 7682)
	require.NoError(t, err)
	require.NotNil(t, result)
	var saved UserSubscription
	require.NoError(t, DB.First(&saved, 7682).Error)
	assert.Equal(t, int64(0), saved.TokenUsed)
	assert.Equal(t, int64(0), saved.AmountUsed)
	assert.InDelta(t, end-30*86400, saved.EndTime, 2)
}

func TestAdminTrialSubscriptionIsNotPaidEquivalentForReset(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7683, Username: "admin_trial_reset", Status: common.UserStatusEnabled, AffCode: "aff7683"}).Error)
	trialCode := "admin_trial_reset_tier"
	trialPlan := &SubscriptionPlan{Id: 7684, Title: "Admin Trial", Enabled: true, IsTrial: true, PriceAmount: 0, Currency: "CNY", BusinessCode: &trialCode}
	require.NoError(t, DB.Create(trialPlan).Error)
	freeCode := "admin_free_reset_tier"
	freePlan := &SubscriptionPlan{Id: 7685, Title: "Admin Free", Enabled: true, PriceAmount: 0, Currency: "CNY", BusinessCode: &freeCode}
	require.NoError(t, DB.Create(freePlan).Error)
	now := common.GetTimestamp()
	trialSub := &UserSubscription{Id: 7686, UserId: 7683, PlanId: 7684, Status: "active", TokenLimit: 0, TokenUsed: 0, StartTime: now - 3600, EndTime: now + 24*3600, GrantReason: "admin", Source: "admin"}
	freeSub := &UserSubscription{Id: 7687, UserId: 7683, PlanId: 7685, Status: "active", TokenLimit: 100, TokenUsed: 10, StartTime: now - 3600, EndTime: now + 24*3600, GrantReason: "admin", Source: "admin"}
	require.NoError(t, DB.Create(trialSub).Error)
	require.NoError(t, DB.Create(freeSub).Error)

	summaries := buildPublicSubscriptionSummaries([]SubscriptionSummary{{Subscription: trialSub, Plan: trialPlan}, {Subscription: freeSub, Plan: freePlan}}, trialSub.Id, now)
	require.Len(t, summaries, 2)
	assert.NotEqual(t, "paid", summaries[0].Subscription.SourceLabel)
	assert.False(t, summaries[0].Subscription.CanResetQuota)
	assert.NotEqual(t, "paid", summaries[1].Subscription.SourceLabel)
	assert.False(t, summaries[1].Subscription.CanResetQuota)

	_, err := ResetUserSubscriptionQuota(7683, 7686)
	require.Error(t, err)
	_, err = ResetUserSubscriptionQuota(7683, 7687)
	require.Error(t, err)
}

func TestPreConsumeUserSubscriptionUsesEarlierPaidRedemptionBeforeLaterReward(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7688, Username: "same_tier_redemption_reward", Status: common.UserStatusEnabled, AffCode: "aff7688"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7689, "same_tier_redemption_reward", 100)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7690, UserId: 7688, PlanId: 7689, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: now + 24*3600, GrantReason: "redemption", Source: "redemption"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7691, UserId: 7688, PlanId: 7689, Status: "active", TokenLimit: 100, TokenUsed: 25, EndTime: now + 3*86400, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)

	pre, err := PreConsumeUserSubscription("same-tier-redemption-reward", 7688, "gpt-4o", 0, 6)

	require.NoError(t, err)
	assert.Equal(t, 7690, pre.UserSubscriptionId)
	var paid UserSubscription
	require.NoError(t, DB.First(&paid, 7690).Error)
	assert.Equal(t, int64(6), paid.TokenUsed)
	var reward UserSubscription
	require.NoError(t, DB.First(&reward, 7691).Error)
	assert.Equal(t, int64(25), reward.TokenUsed)
}

func TestPreConsumeUserSubscription_IgnoresAmountTotalForDistributorLimit(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7401, Username: "token_user", Status: common.UserStatusEnabled, AffCode: "aff7401"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7402, "tiny", 10)
	seedUserSubscriptionForDistributorTest(t, 7403, 7401, 7402, 10, 0, 1, "order")

	_, err := PreConsumeUserSubscription("token-only-ok", 7401, "gpt-4o", 0, 6)
	require.NoError(t, err)

	var sub UserSubscription
	require.NoError(t, DB.First(&sub, 7403).Error)
	assert.Equal(t, int64(6), sub.TokenUsed)
}

func TestPreConsumeUserSubscriptionByUnits_UsesUnitsForSelectedDistributor(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7461, Username: "mixed_dist", Status: common.UserStatusEnabled, AffCode: "aff7461"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7462, Title: "Legacy", Enabled: true, TotalAmount: 1}).Error)
	seedUserSubscriptionForDistributorTest(t, 7463, 7461, 7462, 0, 0, 20, "order")
	seedDistributorSubscriptionPlanForTest(t, 7464, "mixed-dist", 100)
	seedUserSubscriptionForDistributorTest(t, 7465, 7461, 7464, 100, 40, 1, "order")

	pre, err := PreConsumeUserSubscriptionByUnits("mixed-dist-ok", 7461, "gpt-4o", 0, 1000, 10)
	require.NoError(t, err)
	assert.True(t, pre.DistributorTokenBilling)
	assert.Equal(t, int64(10), pre.PreConsumed)

	var legacy UserSubscription
	require.NoError(t, DB.First(&legacy, 7463).Error)
	assert.Equal(t, int64(0), legacy.AmountUsed)
	var distributor UserSubscription
	require.NoError(t, DB.First(&distributor, 7465).Error)
	assert.Equal(t, int64(50), distributor.TokenUsed)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "mixed-dist-ok").First(&record).Error)
	assert.Equal(t, int64(10), record.PreConsumed)
}

func TestPreConsumeUserSubscriptionByUnitsReturnsPlanMetadata(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7631, Username: "plan_meta_user", Status: common.UserStatusEnabled, AffCode: "aff7631"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7632, "plan-meta", 100)
	seedUserSubscriptionForDistributorTest(t, 7633, 7631, 7632, 100, 0, 1, "order")

	pre, err := PreConsumeUserSubscriptionByUnits("plan-meta-ok", 7631, "gpt-4o", 0, 0, 10)
	require.NoError(t, err)

	assert.Equal(t, 7632, pre.PlanId)
	assert.Equal(t, "plan-meta", pre.PlanTitle)

	repeat, err := PreConsumeUserSubscriptionByUnits("plan-meta-ok", 7631, "gpt-4o", 0, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 7632, repeat.PlanId)
	assert.Equal(t, "plan-meta", repeat.PlanTitle)
}

func TestPreConsumeUserSubscriptionByUnitsReturnsPlanTrialMarker(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7651, Username: "trial_marker_user", Status: common.UserStatusEnabled, AffCode: "aff7651"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	trialCode := "trial-marker"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7652, Title: "Trial Marker", Enabled: true, IsTrial: true, BusinessCode: &trialCode}).Error)
	seedUserSubscriptionForDistributorTest(t, 7653, 7651, 7652, 0, 0, 0, "trial_code")

	pre, err := PreConsumeUserSubscriptionByUnits("trial-marker-ok", 7651, "gpt-4o", 0, 0, 10)
	require.NoError(t, err)
	assert.True(t, pre.PlanIsTrial)

	repeat, err := PreConsumeUserSubscriptionByUnits("trial-marker-ok", 7651, "gpt-4o", 0, 0, 10)
	require.NoError(t, err)
	assert.True(t, repeat.PlanIsTrial)
}

func TestPreConsumeUserSubscriptionByUnitsRejectsLegacyAmountSubscriptions(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7471, Username: "mixed_legacy", Status: common.UserStatusEnabled, AffCode: "aff7471"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7472, "mixed-legacy", 5)
	seedUserSubscriptionForDistributorTest(t, 7473, 7471, 7472, 5, 0, 1, "order")
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7474, Title: "Legacy", Enabled: true, TotalAmount: 1}).Error)
	seedUserSubscriptionForDistributorTest(t, 7475, 7471, 7474, 0, 0, 1000, "order")

	_, err := PreConsumeUserSubscriptionByUnits("mixed-legacy-rejected", 7471, "gpt-4o", 0, 100, 10)
	require.Error(t, err)

	var distributor UserSubscription
	require.NoError(t, DB.First(&distributor, 7473).Error)
	assert.Equal(t, int64(0), distributor.TokenUsed)
	var legacy UserSubscription
	require.NoError(t, DB.First(&legacy, 7475).Error)
	assert.Equal(t, int64(0), legacy.AmountUsed)
}

func TestSettleUserSubscription_UsesTokenUsedForDistributor(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7411, Username: "settle_user", Status: common.UserStatusEnabled, AffCode: "aff7411"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7412, "settle", 10)
	seedUserSubscriptionForDistributorTest(t, 7413, 7411, 7412, 10, 0, 1, "order")

	pre, err := PreConsumeUserSubscription("token-settle", 7411, "gpt-4o", 0, 6)
	require.NoError(t, err)
	require.NoError(t, PostConsumeUserSubscriptionDelta(pre.UserSubscriptionId, 2))

	var sub UserSubscription
	require.NoError(t, DB.First(&sub, 7413).Error)
	assert.Equal(t, int64(8), sub.TokenUsed)
}

func TestPostConsumeUserSubscriptionDeltaOnlyChangesTokenUsed(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7451, Username: "delta_user", Status: common.UserStatusEnabled, AffCode: "aff7451"}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7452, "delta", 0)
	seedUserSubscriptionForDistributorTest(t, 7453, 7451, 7452, 0, 10, 1, "trial_code")
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", 7453).Update("source", "trial_code").Error)

	require.NoError(t, PostConsumeUserSubscriptionDelta(7453, 7))

	var got UserSubscription
	require.NoError(t, DB.First(&got, 7453).Error)
	assert.Equal(t, int64(17), got.TokenUsed)
	assert.Equal(t, "active", got.Status)
	assert.Equal(t, "trial_code", got.GrantReason)
	assert.Equal(t, "trial_code", got.Source)
	assert.Equal(t, int64(0), got.TokenLimit)
}

func TestPostConsumeUserSubscriptionDeltaSQLUsesAtomicIncrement(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7481, Username: "delta_sql_user", Status: common.UserStatusEnabled, AffCode: "aff7481"}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7482, "delta-sql", 100)
	seedUserSubscriptionForDistributorTest(t, 7483, 7481, 7482, 100, 10, 1, "order")

	stmt := DB.Session(&gorm.Session{DryRun: true}).Model(&UserSubscription{}).
		Where("id = ?", 7483).
		Updates(map[string]interface{}{
			"token_used": tokenUsedDeltaExpr(7),
			"updated_at": 123,
		}).Statement
	require.NoError(t, stmt.Error)
	sql := stmt.SQL.String()
	if !strings.Contains(sql, "token_used") || !strings.Contains(sql, "+") {
		t.Fatalf("token delta SQL must increment existing token_used atomically, got SQL: %s", sql)
	}
}

func TestPostConsumeUserSubscriptionDeltaSQLAvoidsPreUpdateRowLockRead(t *testing.T) {
	stmt := DB.Session(&gorm.Session{DryRun: true}).Model(&UserSubscription{}).
		Where("id = ?", 7483).
		Where("token_limit <= 0 OR token_used + ? <= token_limit", int64(7)).
		Updates(map[string]interface{}{
			"token_used": tokenUsedDeltaExpr(7),
			"updated_at": 123,
		}).Statement
	require.NoError(t, stmt.Error)
	sql := stmt.SQL.String()
	if strings.Contains(sql, "FOR UPDATE") || strings.Contains(sql, "SELECT") {
		t.Fatalf("subscription delta should be a single conditional UPDATE without pre-lock SELECT, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "token_limit") || !strings.Contains(sql, "token_used") || !strings.Contains(sql, "+") {
		t.Fatalf("subscription token delta SQL must atomically guard and increment token_used, got SQL: %s", sql)
	}
}

type sqlCaptureLogger struct {
	logger.Interface
	statements []string
}

func (l *sqlCaptureLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.statements = append(l.statements, sql)
	if l.Interface != nil {
		l.Interface.Trace(ctx, begin, fc, err)
	}
}

func TestPostConsumeUserSubscriptionDeltaDoesNotPreLockRead(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7641, Username: "delta_no_select_user", Status: common.UserStatusEnabled, AffCode: "aff7641"}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7642, "delta-no-select", 100)
	seedUserSubscriptionForDistributorTest(t, 7643, 7641, 7642, 100, 10, 1, "order")

	capture := &sqlCaptureLogger{Interface: logger.Default.LogMode(logger.Silent)}
	originalDB := DB
	DB = DB.Session(&gorm.Session{Logger: capture})
	t.Cleanup(func() { DB = originalDB })

	require.NoError(t, PostConsumeUserSubscriptionDelta(7643, 7))

	for _, sql := range capture.statements {
		upper := strings.ToUpper(sql)
		if strings.Contains(upper, "SELECT") && strings.Contains(sql, "user_subscriptions") {
			t.Fatalf("subscription delta should avoid pre-lock SELECT on hot user_subscriptions row; saw SQL: %s; all SQL: %#v", sql, capture.statements)
		}
	}
	if len(capture.statements) != 1 || !strings.Contains(strings.ToUpper(capture.statements[0]), "UPDATE") || !strings.Contains(capture.statements[0], "user_subscriptions") {
		t.Fatalf("subscription delta should execute one hot-row UPDATE, got SQL: %#v", capture.statements)
	}
	var got UserSubscription
	require.NoError(t, DB.First(&got, 7643).Error)
	assert.Equal(t, int64(17), got.TokenUsed)
}

func TestPreConsumeUserSubscriptionDoesNotClobberConcurrentPostDelta(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.Create(&User{Id: 7487, Username: "preconsume_atomic_user", Status: common.UserStatusEnabled, AffCode: "aff7487"}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7488, "preconsume-atomic", 1000)
	seedUserSubscriptionForDistributorTest(t, 7489, 7487, 7488, 1000, 0, 1, "order")

	const callbackName = "loadtest:inject_post_delta_before_preconsume_update"
	var injected atomic.Bool
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "UserSubscription" {
			return
		}
		if !injected.CompareAndSwap(false, true) {
			return
		}
		_ = tx.Exec("UPDATE user_subscriptions SET token_used = token_used + ? WHERE id = ?", 26, 7489).Error
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callbackName) })

	_, err := PreConsumeUserSubscriptionByUnits("preconsume-atomic", 7487, "gpt-4o", 0, 0, 2)
	require.NoError(t, err)
	require.True(t, injected.Load(), "test callback did not exercise the stale update window")

	var got UserSubscription
	require.NoError(t, DB.First(&got, 7489).Error)
	assert.Equal(t, int64(28), got.TokenUsed)
}

func TestPreConsumeUserSubscriptionByUnitsRejectsStaleSelectionWhenConditionalUpdateMatchesNoRows(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.Create(&User{Id: 7491, Username: "preconsume_conditional_user", Status: common.UserStatusEnabled, AffCode: "aff7491"}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7492, "preconsume-conditional", 3)
	seedUserSubscriptionForDistributorTest(t, 7493, 7491, 7492, 3, 1, 1, "order")

	const callbackName = "loadtest:consume_remaining_before_preconsume_update"
	var injected atomic.Bool
	var injectedErr atomic.Value
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "UserSubscription" {
			return
		}
		if !injected.CompareAndSwap(false, true) {
			return
		}
		if err := tx.Exec("UPDATE user_subscriptions SET token_used = token_used + ? WHERE id = ?", 2, 7493).Error; err != nil {
			injectedErr.Store(err.Error())
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callbackName) })

	_, err := PreConsumeUserSubscriptionByUnits("preconsume-conditional", 7491, "gpt-4o", 0, 0, 2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription token quota insufficient")
	require.True(t, injected.Load(), "test callback did not exercise the stale update window")
	if value := injectedErr.Load(); value != nil {
		t.Fatalf("failed to inject concurrent subscription update: %v", value)
	}

	var got UserSubscription
	require.NoError(t, DB.First(&got, 7493).Error)
	assert.Equal(t, int64(1), got.TokenUsed)
}

func TestPreConsumeUserSubscriptionByUnitsDryRunContainsConditionalTokenGuard(t *testing.T) {
	stmt := DB.Session(&gorm.Session{DryRun: true}).Model(&UserSubscription{}).
		Where("id = ?", 7493).
		Where("token_limit <= 0 OR token_used + ? <= token_limit", int64(2)).
		Updates(map[string]interface{}{
			"token_used": tokenUsedDeltaExpr(2),
			"updated_at": 123,
		}).Statement
	require.NoError(t, stmt.Error)
	sql := stmt.SQL.String()
	if !strings.Contains(sql, "token_used") || !strings.Contains(sql, "token_limit") {
		t.Fatalf("conditional preconsume SQL must guard token_used against token_limit, got SQL: %s", sql)
	}
}

func TestPostConsumeUserSubscriptionDeltaConcurrentSettlePreservesEveryIncrement(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.Create(&User{Id: 7484, Username: "delta_concurrent_user", Status: common.UserStatusEnabled, AffCode: "aff7484"}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7485, "delta-concurrent", 1000)
	seedUserSubscriptionForDistributorTest(t, 7486, 7484, 7485, 1000, 0, 1, "order")

	const requests = 10
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, requests)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- DB.Transaction(func(tx *gorm.DB) error {
				if err := postConsumeUserSubscriptionDeltaTx(tx, 7486, 2); err != nil {
					return err
				}
				return postConsumeUserSubscriptionDeltaTx(tx, 7486, 26)
			})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var got UserSubscription
	require.NoError(t, DB.First(&got, 7486).Error)
	assert.Equal(t, int64(280), got.TokenUsed)
}

func TestPreConsumeUserSubscriptionByUnitsReusesCachedPrimarySelection(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.Create(&User{Id: 7496, Username: "cached_selection", Status: common.UserStatusEnabled, AffCode: "aff7496"}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7497, "cached-selection", 1000)
	seedUserSubscriptionForDistributorTest(t, 7498, 7496, 7497, 1000, 0, 1, "order")

	first, err := PreConsumeUserSubscriptionByUnits("cached-selection-1", 7496, "gpt-4o", 0, 0, 2)
	require.NoError(t, err)
	require.Equal(t, 7498, first.UserSubscriptionId)

	subscriptionQueryCount := 0
	selectionQueryCount := 0
	callbackName := "loadtest:count_cached_selection_subscription_query"
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "UserSubscription" {
			return
		}
		subscriptionQueryCount++
		if _, ok := tx.Statement.Clauses["ORDER BY"]; ok {
			selectionQueryCount++
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callbackName) })

	second, err := PreConsumeUserSubscriptionByUnits("cached-selection-2", 7496, "gpt-4o", 0, 0, 2)
	require.NoError(t, err)
	require.Equal(t, 7498, second.UserSubscriptionId)
	assert.Equal(t, 1, subscriptionQueryCount, "cached selection should only refresh subscription usage counters")
	assert.Equal(t, 0, selectionQueryCount, "cached selection should skip ordered user_subscriptions hot-row selection query")

	var got UserSubscription
	require.NoError(t, DB.First(&got, 7498).Error)
	assert.Equal(t, int64(4), got.TokenUsed)
}

func TestPreConsumeUserSubscriptionByUnitsCacheHonorsActiveSubscriptionSetting(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	user := User{Id: 7499, Username: "cached_setting", Status: common.UserStatusEnabled, AffCode: "aff7499"}
	require.NoError(t, DB.Create(&user).Error)
	seedDistributorSubscriptionPlanForTest(t, 7500, "cached-setting-a", 1000)
	seedDistributorSubscriptionPlanForTest(t, 7501, "cached-setting-b", 1000)
	seedUserSubscriptionForDistributorTest(t, 7502, 7499, 7500, 1000, 0, 1, "order")
	seedUserSubscriptionForDistributorTest(t, 7503, 7499, 7501, 1000, 0, 1, "order")

	first, err := PreConsumeUserSubscriptionByUnits("cached-setting-1", 7499, "gpt-4o", 0, 0, 2)
	require.NoError(t, err)
	require.Equal(t, 7502, first.UserSubscriptionId)

	setting := user.GetSetting()
	setting.ActiveSubscriptionId = 7503
	user.SetSetting(setting)
	require.NoError(t, DB.Save(&user).Error)

	second, err := PreConsumeUserSubscriptionByUnits("cached-setting-2", 7499, "gpt-4o", 0, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, 7503, second.UserSubscriptionId)
}

func TestRefundUserSubscription_UsesRequestIDForDistributor(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7421, Username: "refund_user", Status: common.UserStatusEnabled, AffCode: "aff7421"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7422, "refund", 10)
	seedUserSubscriptionForDistributorTest(t, 7423, 7421, 7422, 10, 0, 1, "order")

	_, err := PreConsumeUserSubscription("token-refund", 7421, "gpt-4o", 0, 6)
	require.NoError(t, err)
	require.NoError(t, RefundSubscriptionPreConsume("token-refund"))

	var sub UserSubscription
	require.NoError(t, DB.First(&sub, 7423).Error)
	assert.Equal(t, int64(0), sub.TokenUsed)
}

func TestPreConsumeUserSubscription_TokenLimitZeroUnlimitedTrials(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.Exec("DELETE FROM subscription_pre_consume_records").Error)
	require.NoError(t, DB.Exec("DELETE FROM user_subscriptions").Error)
	require.NoError(t, DB.Exec("DELETE FROM subscription_plans").Error)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)

	require.NoError(t, DB.Create(&User{Id: 7431, Username: "legacy_admin", Status: common.UserStatusEnabled, AffCode: "aff7431"}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7432, Title: "Legacy", Enabled: true, TotalAmount: 1}).Error)
	seedUserSubscriptionForDistributorTest(t, 7433, 7431, 7432, 0, 0, 1, "admin")

	_, err := PreConsumeUserSubscription("legacy-admin-over-limit", 7431, "gpt-4o", 0, 6)
	require.Error(t, err)

	require.NoError(t, DB.Create(&User{Id: 7441, Username: "trial_user", Status: common.UserStatusEnabled, AffCode: "aff7441"}).Error)
	trialCode := "trial_24h"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7442, Title: "Trial", Enabled: true, IsTrial: true, BusinessCode: &trialCode}).Error)
	seedUserSubscriptionForDistributorTest(t, 7443, 7441, 7442, 0, 0, 0, "trial_code")

	_, err = PreConsumeUserSubscription("trial-unlimited", 7441, "gpt-4o", 0, 6)
	require.NoError(t, err)

	var sub UserSubscription
	require.NoError(t, DB.First(&sub, 7443).Error)
	assert.Equal(t, int64(6), sub.TokenUsed)

	require.NoError(t, DB.Create(&User{Id: 7481, Username: "admin_trial_user", Status: common.UserStatusEnabled, AffCode: "aff7481"}).Error)
	adminTrialCode := "admin_trial_24h"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7482, Title: "Admin Trial", Enabled: true, IsTrial: true, BusinessCode: &adminTrialCode}).Error)
	seedUserSubscriptionForDistributorTest(t, 7483, 7481, 7482, 0, 0, 0, "admin")

	_, err = PreConsumeUserSubscription("admin-trial-unlimited", 7481, "gpt-4o", 0, 6)
	require.NoError(t, err)

	var adminTrial UserSubscription
	require.NoError(t, DB.First(&adminTrial, 7483).Error)
	assert.Equal(t, int64(6), adminTrial.TokenUsed)

	usage, err := GetActiveDistributorSubscriptionUsage(7481)
	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.True(t, usage.Unlimited)
}

func TestPreConsumeUserSubscriptionUsesEarlierUnlimitedTrialBeforeLaterPaid(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7445, Username: "trial_before_paid", Status: common.UserStatusEnabled, AffCode: "aff7445"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7446, "trial-first", 0)
	seedDistributorSubscriptionPlanForTest(t, 7448, "paid-second", 100)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7447, UserId: 7445, PlanId: 7446, Status: "active", TokenLimit: 0, EndTime: now + 3600, GrantReason: "trial_code"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7449, UserId: 7445, PlanId: 7448, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: "order"}).Error)

	pre, err := PreConsumeUserSubscription("trial-before-paid", 7445, "gpt-4o", 0, 6)
	require.NoError(t, err)
	assert.Equal(t, 7447, pre.UserSubscriptionId)

	var trial UserSubscription
	require.NoError(t, DB.First(&trial, 7447).Error)
	assert.Equal(t, int64(6), trial.TokenUsed)
	var paid UserSubscription
	require.NoError(t, DB.First(&paid, 7449).Error)
	assert.Zero(t, paid.TokenUsed)
}

func TestPreConsumeUserSubscriptionUsesEarlierAdminTrialBeforeLaterPaid(t *testing.T) {
	truncateTables(t)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	require.NoError(t, DB.Exec("DELETE FROM subscription_pre_consume_records").Error)
	InvalidateSubscriptionPlanCache(7492)
	t.Cleanup(func() { InvalidateSubscriptionPlanCache(7492) })
	require.NoError(t, DB.Create(&User{Id: 7491, Username: "admin_trial_before_paid", Status: common.UserStatusEnabled, AffCode: "aff7491"}).Error)
	adminTrialCode := "admin-trial-first"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7492, Title: "Admin Trial", Enabled: true, IsTrial: true, BusinessCode: &adminTrialCode}).Error)
	seedDistributorSubscriptionPlanForTest(t, 7494, "paid-after-admin-trial", 100)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{Id: 7493, UserId: 7491, PlanId: 7492, Status: "active", TokenLimit: 0, EndTime: now + 3600, GrantReason: "admin"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7495, UserId: 7491, PlanId: 7494, Status: "active", TokenLimit: 100, EndTime: now + 7200, GrantReason: "order"}).Error)

	pre, err := PreConsumeUserSubscription("admin-trial-before-paid", 7491, "gpt-4o", 0, 6)
	require.NoError(t, err)
	assert.Equal(t, 7493, pre.UserSubscriptionId)

	var adminTrial UserSubscription
	require.NoError(t, DB.First(&adminTrial, 7493).Error)
	assert.Equal(t, int64(6), adminTrial.TokenUsed)
	var paid UserSubscription
	require.NoError(t, DB.First(&paid, 7495).Error)
	assert.Zero(t, paid.TokenUsed)
}

func TestEnsureDistributorDefaultPlans(t *testing.T) {
	truncateTables(t)
	require.NoError(t, EnsureDistributorDefaultPlans())

	assertDefaultDistributorPlan(t, "trial_24h", "试用装可乐", 0, "CNY", 0, 1, true, false, 24, false, SubscriptionResetNever)
	assertDefaultDistributorPlan(t, "basic_monthly", "Basic", 40, "CNY", 1_000_000_000, 1, false, true, 0, true, SubscriptionResetMonthly)
	assertDefaultDistributorPlan(t, "standard_monthly", "Standard", 80, "CNY", 2_000_000_000, 5, false, true, 0, true, SubscriptionResetMonthly)
	assertDefaultDistributorPlan(t, "pro_monthly", "Pro", 160, "CNY", 5_000_000_000, 10, false, true, 0, true, SubscriptionResetMonthly)
	assertDefaultDistributorPlan(t, "max_monthly", "Max", 660, "CNY", 10_000_000_000, 50, false, true, 0, true, SubscriptionResetMonthly)
}

func TestMigrateLegacyTrialPlanTitleUpdatesLegacyTitles(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		title string
	}{
		{name: "old default", title: "Trial"},
		{name: "empty", title: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			trialCode := "trial_24h"
			require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7501, Title: testCase.title, Enabled: true, IsTrial: true, BusinessCode: &trialCode}).Error)

			require.NoError(t, migrateLegacyTrialPlanTitle())

			var plan SubscriptionPlan
			require.NoError(t, DB.Where("business_code = ?", trialCode).First(&plan).Error)
			assert.Equal(t, "试用装可乐", plan.Title)
		})
	}
}

func TestMigrateLegacyTrialPlanTitlePreservesCustomTrialTitle(t *testing.T) {
	truncateTables(t)
	trialCode := "trial_24h"
	basicCode := "basic_monthly"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7511, Title: "自定义试用", Enabled: true, IsTrial: true, BusinessCode: &trialCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7512, Title: "自定义基础套餐", Enabled: true, BusinessCode: &basicCode}).Error)

	require.NoError(t, migrateLegacyTrialPlanTitle())

	var trialPlan SubscriptionPlan
	require.NoError(t, DB.Where("business_code = ?", trialCode).First(&trialPlan).Error)
	assert.Equal(t, "自定义试用", trialPlan.Title)

	var basicPlan SubscriptionPlan
	require.NoError(t, DB.Where("business_code = ?", basicCode).First(&basicPlan).Error)
	assert.Equal(t, "自定义基础套餐", basicPlan.Title)
}

func TestMigrateLegacyTrialPlanTitlePreservesNonTrialPlan(t *testing.T) {
	truncateTables(t)
	trialCode := "trial_24h"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7513, Title: "Trial", Enabled: true, IsTrial: false, BusinessCode: &trialCode}).Error)

	require.NoError(t, migrateLegacyTrialPlanTitle())

	var plan SubscriptionPlan
	require.NoError(t, DB.Where("business_code = ?", trialCode).First(&plan).Error)
	assert.Equal(t, "Trial", plan.Title)
}

func TestGetAllUserSubscriptionsIncludesPlanTitles(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 7521, Username: "summary_user", Status: common.UserStatusEnabled}).Error)
	trialCode := "trial_24h"
	paidCode := "summary_paid"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7522, Title: "试用装可乐", Enabled: true, IsTrial: true, PublicVisible: false, BusinessCode: &trialCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7523, Title: "标准可乐", Enabled: true, IsTrial: false, PublicVisible: true, BusinessCode: &paidCode}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7524, UserId: 7521, PlanId: 7522, Status: "active", StartTime: now - 60, EndTime: now + 3600, GrantReason: "trial_code"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7525, UserId: 7521, PlanId: 7523, Status: "active", StartTime: now - 60, EndTime: now + 7200, GrantReason: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7526, UserId: 7521, PlanId: 7599, Status: "active", StartTime: now - 60, EndTime: now + 1800, GrantReason: "admin"}).Error)

	allSubscriptions, err := GetAllUserSubscriptions(7521)
	require.NoError(t, err)
	require.Len(t, allSubscriptions, 3)
	assertSummaryPlanTitleByPlanId(t, allSubscriptions, 7522, "试用装可乐")
	assertSummaryPlanTitleByPlanId(t, allSubscriptions, 7523, "标准可乐")
	assertSummaryPlanMissingByPlanId(t, allSubscriptions, 7599)

	activeSubscriptions, err := GetAllActiveUserSubscriptions(7521)
	require.NoError(t, err)
	require.Len(t, activeSubscriptions, 3)
	assertSummaryPlanTitleByPlanId(t, activeSubscriptions, 7522, "试用装可乐")
	assertSummaryPlanTitleByPlanId(t, activeSubscriptions, 7523, "标准可乐")
	assertSummaryPlanMissingByPlanId(t, activeSubscriptions, 7599)
}

func assertSummaryPlanTitleByPlanId(t *testing.T, summaries []SubscriptionSummary, planId int, title string) {
	t.Helper()
	for _, summary := range summaries {
		if summary.Subscription == nil || summary.Subscription.PlanId != planId {
			continue
		}
		require.NotNil(t, summary.Plan)
		assert.Equal(t, title, summary.Plan.Title)
		return
	}
	require.Failf(t, "subscription summary not found", "plan_id=%d", planId)
}

func assertSummaryPlanMissingByPlanId(t *testing.T, summaries []SubscriptionSummary, planId int) {
	t.Helper()
	for _, summary := range summaries {
		if summary.Subscription == nil || summary.Subscription.PlanId != planId {
			continue
		}
		assert.Nil(t, summary.Plan)
		return
	}
	require.Failf(t, "subscription summary not found", "plan_id=%d", planId)
}

func assertDefaultDistributorPlan(t *testing.T, businessCode string, title string, price float64, currency string, tokenLimit int64, concurrency int, isTrial bool, publicVisible bool, trialHours int, rewardEligible bool, resetPeriod string) {
	t.Helper()
	var plan SubscriptionPlan
	require.NoError(t, DB.Where("business_code = ?", businessCode).First(&plan).Error)
	assert.Equal(t, title, plan.Title)
	assert.Equal(t, price, plan.PriceAmount)
	assert.Equal(t, currency, plan.Currency)
	assert.Equal(t, tokenLimit, plan.MonthlyTokenLimit)
	assert.Equal(t, concurrency, plan.ConcurrencyLimit)
	assert.Equal(t, isTrial, plan.IsTrial)
	assert.Equal(t, publicVisible, plan.PublicVisible)
	assert.Equal(t, trialHours, plan.TrialDurationHours)
	assert.Equal(t, rewardEligible, plan.RewardEligible)
	assert.Equal(t, resetPeriod, plan.QuotaResetPeriod)
}

func TestPreConsumeUserSubscriptionByUnitsPaidSubscriptionCodexProEligibleMetadata(t *testing.T) {
	for _, tc := range []struct {
		name        string
		grantSource string
	}{
		{name: "order", grantSource: SubscriptionGrantOrder},
		{name: "redemption", grantSource: "redemption"},
		{name: "admin_after_sales", grantSource: "admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			require.NoError(t, DB.Create(&User{Id: 7801, Username: "codex_pro_paid_" + tc.name, Status: common.UserStatusEnabled, AffCode: "aff7801"}).Error)
			ensureSubscriptionPreConsumeRecordTableForTest(t)
			seedCodexProEligibilityPlanForTest(t, 7802, "paid-codex-pro-"+tc.name, 100, 80, false, false)
			seedCodexProEligibilitySubscriptionForTest(t, 7803, 7801, 7802, 100, 0, tc.grantSource, tc.grantSource, "active", common.GetTimestamp()+3600)

			pre, err := PreConsumeUserSubscriptionByUnits("paid-codex-pro-"+tc.name, 7801, "gpt-4o", 0, 0, 10)

			require.NoError(t, err)
			assert.Equal(t, 7803, pre.UserSubscriptionId)
			requireCodexProPreConsumeEligibility(t, pre, true)
		})
	}
}

func TestPreConsumeUserSubscriptionByUnitsCodexProUnavailableForTrialsInviteRewardsAndInvalidSubscriptions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		price       float64
		isTrial     bool
		inviteTrial bool
		grantReason string
		source      string
		status      string
		endOffset   int64
		tokenLimit  int64
		tokenUsed   int64
		wantError   string
	}{
		{name: "is_trial_plan", price: 80, isTrial: true, grantReason: "trial_code", source: "trial_code", status: "active", endOffset: 3600, tokenLimit: 0},
		{name: "invite_trial_plan", price: 80, inviteTrial: true, grantReason: "invite_trial", source: "invite_trial", status: "active", endOffset: 3600, tokenLimit: 0},
		{name: "trial_code_source_paid_plan", price: 80, grantReason: "trial_code", source: "trial_code", status: "active", endOffset: 3600, tokenLimit: 100},
		{name: "invite_trial_source_paid_plan", price: 80, grantReason: "invite_trial", source: "invite_trial", status: "active", endOffset: 3600, tokenLimit: 100},
		{name: "monthly_invite_entitlement", price: 80, grantReason: SubscriptionGrantMonthlyInviteEntitlement, source: SubscriptionGrantMonthlyInviteEntitlement, status: "active", endOffset: 3600, tokenLimit: 100},
		{name: "zero_price_paid_source", price: 0, grantReason: SubscriptionGrantOrder, source: SubscriptionGrantOrder, status: "active", endOffset: 3600, tokenLimit: 100},
		{name: "expired", price: 80, grantReason: SubscriptionGrantOrder, source: SubscriptionGrantOrder, status: "active", endOffset: -60, tokenLimit: 100, wantError: "no active subscription"},
		{name: "exhausted", price: 80, grantReason: SubscriptionGrantOrder, source: SubscriptionGrantOrder, status: "active", endOffset: 3600, tokenLimit: 100, tokenUsed: 100, wantError: "subscription token quota insufficient"},
		{name: "cancelled", price: 80, grantReason: SubscriptionGrantOrder, source: SubscriptionGrantOrder, status: "cancelled", endOffset: 3600, tokenLimit: 100, wantError: "no active subscription"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			require.NoError(t, DB.Create(&User{Id: 7811, Username: "codex_pro_unavailable_" + tc.name, Status: common.UserStatusEnabled, AffCode: "aff7811"}).Error)
			ensureSubscriptionPreConsumeRecordTableForTest(t)
			seedCodexProEligibilityPlanForTest(t, 7812, "codex-pro-unavailable-"+tc.name, tc.tokenLimit, tc.price, tc.isTrial, tc.inviteTrial)
			seedCodexProEligibilitySubscriptionForTest(t, 7813, 7811, 7812, tc.tokenLimit, tc.tokenUsed, tc.grantReason, tc.source, tc.status, common.GetTimestamp()+tc.endOffset)

			pre, err := PreConsumeUserSubscriptionByUnits("codex-pro-unavailable-"+tc.name, 7811, "gpt-4o", 0, 0, 10)

			if tc.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantError)
				return
			}
			require.NoError(t, err)
			requireCodexProPreConsumeEligibility(t, pre, false)
		})
	}
}

func TestPreConsumeUserSubscriptionByUnitsCodexProUnavailableWhenGrantReasonOrSourceIsTrialOrReward(t *testing.T) {
	for _, tc := range []struct {
		name        string
		grantReason string
		source      string
	}{
		{name: "trial_grant_reason", grantReason: "trial_code", source: SubscriptionGrantOrder},
		{name: "trial_source", grantReason: SubscriptionGrantOrder, source: "invite_trial"},
		{name: "reward_grant_reason", grantReason: SubscriptionGrantMonthlyInviteEntitlement, source: SubscriptionGrantOrder},
		{name: "reward_source", grantReason: SubscriptionGrantOrder, source: SubscriptionGrantMonthlyInviteEntitlement},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			require.NoError(t, DB.Create(&User{Id: 7841, Username: "codex_pro_independent_" + tc.name, Status: common.UserStatusEnabled, AffCode: "aff7841"}).Error)
			ensureSubscriptionPreConsumeRecordTableForTest(t)
			seedCodexProEligibilityPlanForTest(t, 7842, "codex-pro-independent-"+tc.name, 100, 80, false, false)
			seedCodexProEligibilitySubscriptionForTest(t, 7843, 7841, 7842, 100, 0, tc.grantReason, tc.source, "active", common.GetTimestamp()+3600)

			pre, err := PreConsumeUserSubscriptionByUnits("codex-pro-independent-"+tc.name, 7841, "gpt-4o", 0, 0, 10)

			require.NoError(t, err)
			requireCodexProPreConsumeEligibility(t, pre, false)
		})
	}
}

func TestPreConsumeUserSubscriptionByUnitsCodexProUnavailableWhenInviteRewardActuallySelected(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7821, Username: "codex_pro_reward_selected", Status: common.UserStatusEnabled, AffCode: "aff7821"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedCodexProEligibilityPlanForTest(t, 7822, "codex-pro-same-tier", 100, 80, false, false)
	now := common.GetTimestamp()
	seedCodexProEligibilitySubscriptionForTest(t, 7823, 7821, 7822, 100, 0, SubscriptionGrantOrder, SubscriptionGrantOrder, "active", now+30*86400)
	seedCodexProEligibilitySubscriptionForTest(t, 7824, 7821, 7822, 100, 0, SubscriptionGrantMonthlyInviteEntitlement, SubscriptionGrantMonthlyInviteEntitlement, "active", now+3*86400)

	pre, err := PreConsumeUserSubscriptionByUnits("codex-pro-selected-reward", 7821, "gpt-4o", 0, 0, 10)

	require.NoError(t, err)
	assert.Equal(t, 7824, pre.UserSubscriptionId, "Codex Pro eligibility must describe the subscription that was actually pre-consumed")
	requireCodexProPreConsumeEligibility(t, pre, false)
}

func TestPreConsumeUserSubscriptionByUnitsCodexProUnavailableWithoutActiveSubscription(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7831, Username: "codex_pro_no_subscription", Status: common.UserStatusEnabled, AffCode: "aff7831"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)

	_, err := PreConsumeUserSubscriptionByUnits("codex-pro-no-subscription", 7831, "gpt-4o", 0, 0, 10)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active subscription")
}

func seedCodexProEligibilityPlanForTest(t *testing.T, id int, code string, tokenLimit int64, price float64, isTrial bool, inviteTrial bool) {
	t.Helper()
	plan := &SubscriptionPlan{Id: id, Title: code, Enabled: true, TotalAmount: 1, PriceAmount: price, MonthlyTokenLimit: tokenLimit, ConcurrencyLimit: 1, IsTrial: isTrial, InviteTrial: inviteTrial, BusinessCode: &code}
	require.NoError(t, DB.Create(plan).Error)
	InvalidateSubscriptionPlanCache(id)
}

func seedCodexProEligibilitySubscriptionForTest(t *testing.T, id int, userId int, planId int, tokenLimit int64, tokenUsed int64, grantReason string, source string, status string, endTime int64) {
	t.Helper()
	require.NoError(t, DB.Create(&UserSubscription{Id: id, UserId: userId, PlanId: planId, Status: status, AmountTotal: 1, TokenLimit: tokenLimit, TokenUsed: tokenUsed, StartTime: common.GetTimestamp() - 60, EndTime: endTime, GrantReason: grantReason, Source: source}).Error)
}

func requireCodexProPreConsumeEligibility(t *testing.T, pre *SubscriptionPreConsumeResult, wantEligible bool) {
	t.Helper()
	require.NotNil(t, pre)
	meta := codexProPreConsumeEligibilityMetadata(t, pre)
	eligible := pre.UserSubscriptionId > 0 && meta.status == "active" && meta.endTime > common.GetTimestamp() && (meta.tokenUnlimited || meta.tokenRemaining > 0) && meta.priceAmount > 0 && !meta.planIsTrial && !meta.planInviteTrial && !codexProIneligibleGrantSourceForTest(meta.grantReason) && !codexProIneligibleGrantSourceForTest(meta.source)
	assert.Equal(t, wantEligible, eligible, "metadata=%+v", meta)
}

type codexProPreConsumeMetadata struct {
	priceAmount     float64
	planIsTrial     bool
	planInviteTrial bool
	source          string
	grantReason     string
	status          string
	endTime         int64
	tokenRemaining  int64
	tokenUnlimited  bool
}

func codexProPreConsumeEligibilityMetadata(t *testing.T, pre *SubscriptionPreConsumeResult) codexProPreConsumeMetadata {
	t.Helper()
	v := reflect.ValueOf(pre).Elem()
	tokenRemaining := getInt64FieldForTest(t, v, "SubscriptionTokenRemaining", "TokenRemaining")
	return codexProPreConsumeMetadata{
		priceAmount:     getFloat64FieldForTest(t, v, "PlanPriceAmount"),
		planIsTrial:     getBoolFieldForTest(t, v, "PlanIsTrial"),
		planInviteTrial: getBoolFieldForTest(t, v, "PlanInviteTrial"),
		source:          getStringFieldForTest(t, v, "SubscriptionSource"),
		grantReason:     getStringFieldForTest(t, v, "SubscriptionGrantReason"),
		status:          getStringFieldForTest(t, v, "SubscriptionStatus"),
		endTime:         getInt64FieldForTest(t, v, "SubscriptionEndTime"),
		tokenRemaining:  tokenRemaining,
		tokenUnlimited:  getInt64FieldForTest(t, v, "TokenLimit") == 0,
	}
}

func codexProIneligibleGrantSourceForTest(value string) bool {
	switch strings.TrimSpace(value) {
	case "trial_code", "invite_trial", SubscriptionGrantMonthlyInviteEntitlement:
		return true
	default:
		return false
	}
}

func getFloat64FieldForTest(t *testing.T, v reflect.Value, name string) float64 {
	t.Helper()
	field := requireStructFieldForTest(t, v, name)
	return field.Float()
}

func getBoolFieldForTest(t *testing.T, v reflect.Value, name string) bool {
	t.Helper()
	field := requireStructFieldForTest(t, v, name)
	return field.Bool()
}

func getStringFieldForTest(t *testing.T, v reflect.Value, name string) string {
	t.Helper()
	field := requireStructFieldForTest(t, v, name)
	return field.String()
}

func getInt64FieldForTest(t *testing.T, v reflect.Value, names ...string) int64 {
	t.Helper()
	for _, name := range names {
		field := v.FieldByName(name)
		if field.IsValid() {
			return field.Int()
		}
	}
	require.Failf(t, "missing field", "SubscriptionPreConsumeResult must expose one of %v for Codex Pro eligibility", names)
	return 0
}

func requireStructFieldForTest(t *testing.T, v reflect.Value, name string) reflect.Value {
	t.Helper()
	field := v.FieldByName(name)
	require.Truef(t, field.IsValid(), "SubscriptionPreConsumeResult must expose %s for Codex Pro eligibility", name)
	return field
}
