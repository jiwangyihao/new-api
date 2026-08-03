package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTimedSubscriptionValuationGrantCreatesTimelineAndReplaysSource(t *testing.T) {
	setupTimedSubscriptionValuationTestDB(t)
	priceMicros := int64(40_000_000)
	user := User{Id: 21_001, Username: "timed-grant", Status: common.UserStatusEnabled, AffCode: "timed-grant-aff"}
	plan := SubscriptionPlan{
		Id: 21_002, Title: "Timed Basic", Enabled: true,
		EntitlementType: SubscriptionEntitlementTimed,
		PriceAmount:     40, PriceAmountMicros: &priceMicros, Currency: "CNY",
		DurationUnit: SubscriptionDurationCustom, CustomSeconds: 3600,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&plan).Error)

	request := TimedSubscriptionGrantRequest{
		UserId:            user.Id,
		Plan:              &plan,
		IdempotencyKey:    "subscription-order:21003",
		SourceType:        TimedSubscriptionGrantSourceOrder,
		SourceId:          21_003,
		SourcePriceMicros: priceMicros,
		SourceCurrency:    "CNY",
	}
	var first *UserSubscriptionCreationResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		first, err = GrantTimedSubscriptionTx(tx, request)
		return err
	}))
	require.NotNil(t, first)
	require.NotNil(t, first.Subscription)
	require.Equal(t, int64(3600), first.EventEndTime-first.EventStartTime)

	var grant TimedSubscriptionValuationGrant
	require.NoError(t, DB.Where("source_type = ? AND source_key = ?", "subscription_order", "subscription_order:21003").First(&grant).Error)
	require.Equal(t, first.Subscription.Id, grant.UserSubscriptionId)
	require.Equal(t, user.Id, grant.UserId)
	require.Equal(t, plan.Id, grant.PlanId)
	require.Equal(t, first.EventStartTime, grant.EventStartTime)
	require.Equal(t, first.EventEndTime, grant.EventEndTime)
	require.Equal(t, int64(1000), grant.GrantCredit)
	require.Equal(t, priceMicros, grant.SourcePriceMicros)
	require.Equal(t, priceMicros, grant.ValuationAmountMicros)
	require.Equal(t, "CNY", grant.SourceCurrency)
	require.Equal(t, "CNY", grant.ValuationCurrency)
	require.Equal(t, "exact", grant.Confidence)
	require.Equal(t, CreditValuationRuleVersion, grant.RuleVersion)
	require.Equal(t, int64(1), grant.FxRateNumerator)
	require.Equal(t, int64(1), grant.FxRateDenominator)
	require.NotZero(t, grant.CreatedAt)
	require.False(t, strings.TrimSpace(grant.SourceSnapshot) == "")

	originalEndTime := first.Subscription.EndTime
	var replay *UserSubscriptionCreationResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		replay, err = GrantTimedSubscriptionTx(tx, request)
		return err
	}))
	require.NotNil(t, replay)
	require.NotNil(t, replay.Subscription)
	require.Equal(t, first.Subscription.Id, replay.Subscription.Id)
	require.Equal(t, first.EventStartTime, replay.EventStartTime)
	require.Equal(t, first.EventEndTime, replay.EventEndTime)

	var persisted UserSubscription
	require.NoError(t, DB.First(&persisted, first.Subscription.Id).Error)
	require.Equal(t, originalEndTime, persisted.EndTime)
	var grantCount int64
	require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
	require.Equal(t, int64(1), grantCount)
}

func setupTimedSubscriptionValuationTestDB(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &TimedSubscriptionValuationGrant{}))

	t.Cleanup(func() {
		_ = sqlDB.Close()
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		ClearSubscriptionPlanCacheForTest()
		resetDBTimestampCacheForTest()
	})
}
