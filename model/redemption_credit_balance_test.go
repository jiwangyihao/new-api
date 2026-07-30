package model

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRedeemCreditBalanceConcurrentClaimPersistsOneGrantAndOneReplay(t *testing.T) {
	originalDB := DB
	originalLogDB := LOG_DB
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled

	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "redemption-credit-race.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	require.NoError(t, db.AutoMigrate(
		&User{},
		&SubscriptionPlan{},
		&UserSubscription{},
		&Redemption{},
		&CreditBalanceLedger{},
		&InvitationRewardEvent{},
		&Log{},
	))

	DB = db
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	t.Cleanup(func() {
		ClearPrimaryBillableSubscriptionCacheForTest()
		ClearSubscriptionPlanCacheForTest()
		DB = originalDB
		LOG_DB = originalLogDB
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		common.RedisEnabled = originalRedisEnabled
		_ = sqlDB.Close()
	})

	const userID = 10101
	const creditPlanID = 10102
	const optionPlanID = 10103
	const redemptionID = 10104
	require.NoError(t, DB.Create(&User{Id: userID, Username: "redemption-credit-race", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{
		Id:                             creditPlanID,
		Title:                          "Concurrent Credit balance",
		Enabled:                        true,
		EntitlementType:                SubscriptionEntitlementCreditBalance,
		CreditBalanceConfigured:        true,
		CreditBalanceRedemptionEnabled: true,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{
		Id:                       optionPlanID,
		Title:                    "Concurrent monthly option",
		Enabled:                  true,
		EntitlementType:          SubscriptionEntitlementTimed,
		DurationUnit:             SubscriptionDurationMonth,
		DurationValue:            1,
		MonthlyTokenLimit:        2700,
		QuotaResetPeriod:         SubscriptionResetMonthly,
		UnlimitedPurchaseEnabled: true,
	}).Error)
	require.NoError(t, DB.Create(&Redemption{
		Id:          redemptionID,
		Key:         "model-credit-race-redemption",
		Type:        RedemptionTypeSubscription,
		PlanId:      optionPlanID,
		Status:      common.RedemptionCodeStatusEnabled,
		CreatedTime: common.GetTimestamp(),
	}).Error)

	start := make(chan struct{})
	results := make(chan *RedemptionResult, 2)
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, redeemErr := Redeem("model-credit-race-redemption", userID, RedemptionModeCreditBalance)
			results <- result
			errors <- redeemErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errors)

	for redeemErr := range errors {
		require.NoError(t, redeemErr)
	}
	fulfillments := 0
	replays := 0
	ledgerIDs := make(map[int]struct{})
	for result := range results {
		require.NotNil(t, result)
		assert.Equal(t, RedemptionModeCreditBalance, result.RedemptionMode)
		require.NotNil(t, result.CreditBalance)
		assert.Equal(t, int64(2700), result.CreditBalance.GrossCredit)
		ledgerIDs[result.CreditBalance.LedgerId] = struct{}{}
		if result.Replayed {
			replays++
		} else {
			fulfillments++
		}
	}
	assert.Equal(t, 1, fulfillments)
	assert.Equal(t, 1, replays)
	assert.Len(t, ledgerIDs, 1)

	var saved Redemption
	require.NoError(t, DB.First(&saved, redemptionID).Error)
	assert.Equal(t, common.RedemptionCodeStatusUsed, saved.Status)
	assert.Equal(t, RedemptionModeCreditBalance, saved.FulfillmentMode)
	assert.NotEmpty(t, saved.FulfillmentSnapshot)
	var ledgerCount int64
	require.NoError(t, DB.Model(&CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceRedemption, redemptionID).Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
	var balance UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND entitlement_type = ?", userID, SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(2700), balance.TokenLimit)
	assert.Zero(t, balance.TokenUsed)
}
