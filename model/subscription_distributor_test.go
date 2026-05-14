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

func TestPreConsumeUserSubscriptionByUnits_UsesUnitsForSelectedLegacy(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 7471, Username: "mixed_legacy", Status: common.UserStatusEnabled, AffCode: "aff7471"}).Error)
	ensureSubscriptionPreConsumeRecordTableForTest(t)
	seedDistributorSubscriptionPlanForTest(t, 7472, "mixed-legacy", 5)
	seedUserSubscriptionForDistributorTest(t, 7473, 7471, 7472, 5, 0, 1, "order")
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7474, Title: "Legacy", Enabled: true, TotalAmount: 1}).Error)
	seedUserSubscriptionForDistributorTest(t, 7475, 7471, 7474, 0, 0, 1000, "order")

	pre, err := PreConsumeUserSubscriptionByUnits("mixed-legacy-ok", 7471, "gpt-4o", 0, 100, 10)
	require.NoError(t, err)
	assert.False(t, pre.DistributorTokenBilling)
	assert.Equal(t, int64(100), pre.PreConsumed)

	var distributor UserSubscription
	require.NoError(t, DB.First(&distributor, 7473).Error)
	assert.Equal(t, int64(0), distributor.TokenUsed)
	var legacy UserSubscription
	require.NoError(t, DB.First(&legacy, 7475).Error)
	assert.Equal(t, int64(100), legacy.AmountUsed)
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
