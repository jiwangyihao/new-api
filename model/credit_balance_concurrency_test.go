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

func TestConcurrentCreditBalanceGrantsCreateOneBalanceAndAccumulateBothGrants(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "credit-balance-first-grants.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	DB = db
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	resetDBTimestampCacheForTest()
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &CreditBalanceLedger{}))
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	t.Cleanup(func() {
		ClearPrimaryBillableSubscriptionCacheForTest()
		ClearSubscriptionPlanCacheForTest()
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		resetDBTimestampCacheForTest()
		_ = sqlDB.Close()
	})

	const userID = 10_201
	const planID = 10_202
	require.NoError(t, db.Create(&User{Id: userID, Username: "concurrent-credit-first-grants", Status: common.UserStatusEnabled}).Error)
	code := "concurrent_credit_first_grants"
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id: planID, Title: "Credit balance", Enabled: true,
		EntitlementType: SubscriptionEntitlementCreditBalance, BusinessCode: &code,
		CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true,
		CreditBalanceConversionEnabled: true,
	}).Error)

	type grantOutcome struct {
		result *CreditBalanceGrantResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan grantOutcome, 2)
	var waitGroup sync.WaitGroup
	for index, grossCredit := range []int64{75, 125} {
		waitGroup.Add(1)
		go func(index int, grossCredit int64) {
			defer waitGroup.Done()
			<-start
			var result *CreditBalanceGrantResult
			err := transactionWithUserSettingCASRetry(func(tx *gorm.DB) error {
				var grantErr error
				result, grantErr = GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
					UserId: userID, GrossCredit: grossCredit,
					IdempotencyKey: fmt.Sprintf("first-grant-%d", index),
					SourceType:     CreditBalanceLedgerSourceSubscriptionOrder,
					SourceId:       20_000 + index,
					Type:           CreditBalanceLedgerTypePurchase,
					TargetPlanId:   planID,
				})
				return grantErr
			})
			outcomes <- grantOutcome{result: result, err: err}
		}(index, grossCredit)
	}
	close(start)
	waitGroup.Wait()
	close(outcomes)

	balanceIDs := make(map[int]struct{})
	for outcome := range outcomes {
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.result)
		balanceIDs[outcome.result.UserSubscriptionId] = struct{}{}
	}
	assert.Len(t, balanceIDs, 1)
	var balances []UserSubscription
	require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", userID, SubscriptionEntitlementCreditBalance).Find(&balances).Error)
	require.Len(t, balances, 1)
	assert.Equal(t, int64(200), balances[0].TokenLimit)
	assert.Zero(t, balances[0].TokenUsed)
	var ledgerCount int64
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Where("user_id = ?", userID).Count(&ledgerCount).Error)
	assert.Equal(t, int64(2), ledgerCount)
}
