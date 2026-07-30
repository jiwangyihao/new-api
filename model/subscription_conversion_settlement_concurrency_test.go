package model

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConversionRejectsPlanDisabledByIndependentTransactionAfterQuote(t *testing.T) {
	db, adminDB := setupSubscriptionConversionEligibilityConcurrencyTestDB(t)
	const userID = 10_901
	const sourceID = 10_902
	const planID = 10_903

	require.NoError(t, db.Create(&User{Id: userID, Username: "conversion-plan-race", Status: common.UserStatusEnabled}).Error)
	seedConversionQuoteTimedPlan(t, planID, 100)
	now := GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, db.Create(&UserSubscription{
		Id: sourceID, UserId: userID, PlanId: planID,
		EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit:      100, TokenUsed: 25,
		GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime:               now - 40*24*60*60,
		EndTime:                 now + TimedSubscriptionConversionBlockSeconds,
		Status:                  SubscriptionStatusActive,
		LastGrantedAt:           now - 40*24*60*60,
		LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)

	planDisabled := false
	hooks := &subscriptionConversionHooks{
		at: func(phase subscriptionConversionHookPhase) error {
			if phase != subscriptionConversionAfterQuotePhase || planDisabled {
				return nil
			}
			err := adminDB.Transaction(func(tx *gorm.DB) error {
				update := tx.Model(&SubscriptionPlan{}).
					Where("id = ? AND timed_conversion_enabled = ?", planID, true).
					UpdateColumn("timed_conversion_enabled", false)
				if update.Error != nil {
					return update.Error
				}
				if update.RowsAffected != 1 {
					return fmt.Errorf("expected one plan update, got %d", update.RowsAffected)
				}
				return nil
			})
			if err == nil {
				planDisabled = true
			}
			return err
		},
	}

	result, err := confirmTimedSubscriptionConversion(userID, sourceID, "independent-plan-disable", hooks)
	require.Nil(t, result)
	require.ErrorContains(t, err, ConversionQuoteReasonPlanDisabled)

	var plan SubscriptionPlan
	require.NoError(t, adminDB.First(&plan, planID).Error)
	assert.False(t, plan.TimedConversionEnabled)
	var source UserSubscription
	require.NoError(t, db.First(&source, sourceID).Error)
	assert.Equal(t, SubscriptionStatusActive, source.Status)
	var conversionCount int64
	require.NoError(t, db.Model(&SubscriptionConversion{}).Where("source_subscription_id = ?", sourceID).Count(&conversionCount).Error)
	assert.Zero(t, conversionCount)
	var ledgerCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionConversion, sourceID).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
	var creditBalanceCount int64
	require.NoError(t, db.Model(&UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", userID, SubscriptionEntitlementCreditBalance).Count(&creditBalanceCount).Error)
	assert.Zero(t, creditBalanceCount)
}

func TestConversionGuardSerializesLaterPlanDisable(t *testing.T) {
	db, adminDB := setupSubscriptionConversionEligibilityConcurrencyTestDB(t)
	const userID = 10_911
	const sourceID = 10_912
	const planID = 10_913

	require.NoError(t, db.Create(&User{Id: userID, Username: "conversion-plan-guard", Status: common.UserStatusEnabled}).Error)
	seedConversionQuoteTimedPlan(t, planID, 100)
	now := GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, db.Create(&UserSubscription{
		Id: sourceID, UserId: userID, PlanId: planID,
		EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit:      100, TokenUsed: 25,
		GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime:               now - 40*24*60*60,
		EndTime:                 now + TimedSubscriptionConversionBlockSeconds,
		Status:                  SubscriptionStatusActive,
		LastGrantedAt:           now - 40*24*60*60,
		LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)

	adminStarted := make(chan struct{})
	adminDone := make(chan error, 1)
	hooks := &subscriptionConversionHooks{
		at: func(phase subscriptionConversionHookPhase) error {
			if phase != subscriptionConversionAfterEligibilityGuardPhase {
				return nil
			}
			go func() {
				adminDone <- adminDB.Transaction(func(tx *gorm.DB) error {
					close(adminStarted)
					update := tx.Model(&SubscriptionPlan{}).
						Where("id = ? AND timed_conversion_enabled = ?", planID, true).
						UpdateColumn("timed_conversion_enabled", false)
					if update.Error != nil {
						return update.Error
					}
					if update.RowsAffected != 1 {
						return fmt.Errorf("expected one plan update, got %d", update.RowsAffected)
					}
					return nil
				})
			}()
			<-adminStarted
			return nil
		},
	}

	result, err := confirmTimedSubscriptionConversion(userID, sourceID, "guard-before-plan-disable", hooks)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Replayed)
	require.NoError(t, <-adminDone)

	var plan SubscriptionPlan
	require.NoError(t, adminDB.First(&plan, planID).Error)
	assert.False(t, plan.TimedConversionEnabled)
	var source UserSubscription
	require.NoError(t, db.First(&source, sourceID).Error)
	assert.Equal(t, SubscriptionStatusConverted, source.Status)
	var conversionCount int64
	require.NoError(t, db.Model(&SubscriptionConversion{}).Where("source_subscription_id = ?", sourceID).Count(&conversionCount).Error)
	assert.Equal(t, int64(1), conversionCount)
	var ledgerCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionConversion, sourceID).Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
}

func TestConcurrentConvertedSourcesApplyEveryPositiveDeltaToSharedCreditBalance(t *testing.T) {
	db := setupConvertedSettlementConcurrencyTestDB(t)
	const targetID = 10_501
	sourceIDs := seedConvertedSettlementConcurrencyMappings(t, db, targetID, 1_000, 100)

	errs := runConvertedSettlementDeltasConcurrently(sourceIDs, []int64{40, 60})
	for _, err := range errs {
		require.NoError(t, err)
	}

	var target UserSubscription
	require.NoError(t, db.First(&target, targetID).Error)
	assert.Equal(t, int64(1_000), target.TokenLimit)
	assert.Equal(t, int64(200), target.TokenUsed)
	for _, sourceID := range sourceIDs {
		var source UserSubscription
		require.NoError(t, db.First(&source, sourceID).Error)
		assert.Zero(t, source.TokenUsed)
		assert.Equal(t, SubscriptionStatusConverted, source.Status)
	}
}

func TestConcurrentConvertedSourcesApplyEveryRefundToSharedCreditBalance(t *testing.T) {
	db := setupConvertedSettlementConcurrencyTestDB(t)
	const targetID = 10_601
	sourceIDs := seedConvertedSettlementConcurrencyMappings(t, db, targetID, 1_000, 50)

	errs := runConvertedSettlementDeltasConcurrently(sourceIDs, []int64{-40, -30})
	for _, err := range errs {
		require.NoError(t, err)
	}

	var target UserSubscription
	require.NoError(t, db.First(&target, targetID).Error)
	assert.Equal(t, int64(1_020), target.TokenLimit)
	assert.Zero(t, target.TokenUsed)
	for _, sourceID := range sourceIDs {
		var source UserSubscription
		require.NoError(t, db.First(&source, sourceID).Error)
		assert.Zero(t, source.TokenUsed)
		assert.Equal(t, SubscriptionStatusConverted, source.Status)
	}
}

func TestConcurrentConvertedSourcesApplyEveryPositiveAmountDeltaToSharedCreditBalance(t *testing.T) {
	db := setupConvertedSettlementConcurrencyTestDB(t)
	const targetID = 10_701
	sourceIDs := seedConvertedSettlementConcurrencyMappings(t, db, targetID, 1_000, 100)

	errs := runConvertedAmountSettlementDeltasConcurrently(sourceIDs, []int64{40, 60})
	for _, err := range errs {
		require.NoError(t, err)
	}

	var target UserSubscription
	require.NoError(t, db.First(&target, targetID).Error)
	assert.Equal(t, int64(1_000), target.TokenLimit)
	assert.Equal(t, int64(200), target.TokenUsed)
	assertConvertedSettlementSourcesUnchanged(t, db, sourceIDs)
}

func TestConcurrentConvertedSourcesApplyEveryAmountRefundToSharedCreditBalance(t *testing.T) {
	db := setupConvertedSettlementConcurrencyTestDB(t)
	const targetID = 10_801
	sourceIDs := seedConvertedSettlementConcurrencyMappings(t, db, targetID, 1_000, 50)

	errs := runConvertedAmountSettlementDeltasConcurrently(sourceIDs, []int64{-40, -30})
	for _, err := range errs {
		require.NoError(t, err)
	}

	var target UserSubscription
	require.NoError(t, db.First(&target, targetID).Error)
	assert.Equal(t, int64(1_020), target.TokenLimit)
	assert.Zero(t, target.TokenUsed)
	assertConvertedSettlementSourcesUnchanged(t, db, sourceIDs)
}

func TestConvertedAmountSettlementRetryIsBounded(t *testing.T) {
	attempts := 0
	err := runConvertedSubscriptionSettlementWithRetry(func() error {
		attempts++
		return ErrConvertedSubscriptionSettlementConflict
	})

	require.ErrorIs(t, err, ErrConvertedSubscriptionSettlementConflict)
	assert.Equal(t, convertedSubscriptionSettlementMaxAttempts, attempts)
}

func assertConvertedSettlementSourcesUnchanged(t *testing.T, db *gorm.DB, sourceIDs []int) {
	t.Helper()
	for _, sourceID := range sourceIDs {
		var source UserSubscription
		require.NoError(t, db.First(&source, sourceID).Error)
		assert.Zero(t, source.TokenUsed)
		assert.Zero(t, source.AmountUsed)
		assert.Equal(t, SubscriptionStatusConverted, source.Status)
	}
}

func setupConvertedSettlementConcurrencyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	FlushSubscriptionTokenDeltaUpdates()
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "converted-settlement.db")) + "?_pragma=busy_timeout(0)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	DB = db
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	resetDBTimestampCacheForTest()
	require.NoError(t, db.AutoMigrate(&UserSubscription{}, &SubscriptionConversion{}))

	t.Cleanup(func() {
		FlushSubscriptionTokenDeltaUpdates()
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		resetDBTimestampCacheForTest()
		_ = sqlDB.Close()
	})
	return db
}

func seedConvertedSettlementConcurrencyMappings(t *testing.T, db *gorm.DB, targetID int, tokenLimit int64, tokenUsed int64) []int {
	t.Helper()
	const userID = 10_500
	require.NoError(t, db.Create(&UserSubscription{
		Id: targetID, UserId: userID,
		EntitlementType: SubscriptionEntitlementCreditBalance,
		TokenLimit:      tokenLimit, TokenUsed: tokenUsed,
		Status: SubscriptionStatusActive,
	}).Error)

	sourceIDs := []int{targetID + 1, targetID + 2}
	for index, sourceID := range sourceIDs {
		conversionID := targetID + 10 + index
		require.NoError(t, db.Create(&SubscriptionConversion{
			Id: conversionID, UserId: userID,
			IdempotencyKey:       fmt.Sprintf("concurrent-settlement-%d", index),
			SourceSubscriptionId: sourceID,
			TargetSubscriptionId: targetID,
			LedgerId:             targetID + 20 + index,
		}).Error)
		require.NoError(t, db.Create(&UserSubscription{
			Id: sourceID, UserId: userID,
			EntitlementType:           SubscriptionEntitlementTimed,
			Status:                    SubscriptionStatusConverted,
			ConversionId:              conversionID,
			ConvertedToSubscriptionId: targetID,
		}).Error)
	}
	return sourceIDs
}

func runConvertedSettlementDeltasConcurrently(sourceIDs []int, deltas []int64) []error {
	start := make(chan struct{})
	errs := make(chan error, len(sourceIDs))
	var waitGroup sync.WaitGroup
	for index, sourceID := range sourceIDs {
		waitGroup.Add(1)
		go func(sourceID int, delta int64) {
			defer waitGroup.Done()
			<-start
			errs <- PostConsumeUserSubscriptionTokenDelta(sourceID, delta)
		}(sourceID, deltas[index])
	}
	close(start)
	waitGroup.Wait()
	close(errs)

	results := make([]error, 0, len(sourceIDs))
	for err := range errs {
		results = append(results, err)
	}
	return results
}

func runConvertedAmountSettlementDeltasConcurrently(sourceIDs []int, deltas []int64) []error {
	start := make(chan struct{})
	errs := make(chan error, len(sourceIDs))
	var waitGroup sync.WaitGroup
	for index, sourceID := range sourceIDs {
		waitGroup.Add(1)
		go func(sourceID int, delta int64) {
			defer waitGroup.Done()
			<-start
			errs <- PostConsumeUserSubscriptionAmountDelta(sourceID, delta)
		}(sourceID, deltas[index])
	}
	close(start)
	waitGroup.Wait()
	close(errs)

	results := make([]error, 0, len(sourceIDs))
	for err := range errs {
		results = append(results, err)
	}
	return results
}

func setupSubscriptionConversionEligibilityConcurrencyTestDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "conversion-plan-race.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	adminSQLDB, err := adminDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	adminSQLDB.SetMaxOpenConns(1)

	DB = db
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	resetDBTimestampCacheForTest()
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	require.NoError(t, db.AutoMigrate(
		&User{},
		&SubscriptionPlan{},
		&UserSubscription{},
		&SubscriptionOrder{},
		&Redemption{},
		&InvitationRewardEvent{},
		&CreditBalanceLedger{},
		&SubscriptionConversion{},
	))
	seedConversionQuoteCreditBalancePlan(t)

	t.Cleanup(func() {
		_ = adminSQLDB.Close()
		_ = sqlDB.Close()
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		resetDBTimestampCacheForTest()
		ClearPrimaryBillableSubscriptionCacheForTest()
		ClearSubscriptionPlanCacheForTest()
	})
	return db, adminDB
}
