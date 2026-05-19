package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestCreateUserSubscriptionFromPlanTx_RejectsRenewalWhenPurchaseLimitReached(t *testing.T) {
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

	err := DB.Transaction(func(tx *gorm.DB) error {
		_, err := CreateUserSubscriptionFromPlanTx(tx, 7301, plan, "order")
		return err
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "已达到该套餐购买上限")
	var sub UserSubscription
	require.NoError(t, DB.First(&sub, first.Id).Error)
	assert.Equal(t, firstEnd, sub.EndTime)
	assert.Equal(t, int64(123), sub.TokenUsed)
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

	code := "basic_monthly"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7210, Title: "Basic A", Enabled: true, BusinessCode: &code}).Error)
	dup := "basic_monthly"
	require.Error(t, DB.Create(&SubscriptionPlan{Id: 7211, Title: "Basic B", Enabled: true, BusinessCode: &dup}).Error)
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

func TestPreConsumeUserSubscription_TokenLimitZeroOnlyTrialIsUnlimited(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7431, Username: "legacy_admin", Status: common.UserStatusEnabled, AffCode: "aff7431"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
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
}

func TestPreConsumeUserSubscriptionPrefersPaidDistributorBeforeUnlimitedTrial(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7445, Username: "paid_before_trial", Status: common.UserStatusEnabled, AffCode: "aff7445"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7446, "trial-first", 0)
	seedUserSubscriptionForDistributorTest(t, 7447, 7445, 7446, 0, 0, 0, "trial_code")
	seedDistributorSubscriptionPlanForTest(t, 7448, "paid-second", 100)
	seedUserSubscriptionForDistributorTest(t, 7449, 7445, 7448, 100, 0, 1, "order")

	pre, err := PreConsumeUserSubscription("paid-before-trial", 7445, "gpt-4o", 0, 6)
	require.NoError(t, err)
	assert.Equal(t, 7449, pre.UserSubscriptionId)

	var trial UserSubscription
	require.NoError(t, DB.First(&trial, 7447).Error)
	assert.Equal(t, int64(0), trial.TokenUsed)
	var paid UserSubscription
	require.NoError(t, DB.First(&paid, 7449).Error)
	assert.Equal(t, int64(6), paid.TokenUsed)
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
