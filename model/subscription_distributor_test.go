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
