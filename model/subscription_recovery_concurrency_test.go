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

func TestSubscriptionRecoveryTerminalOrderingContracts(t *testing.T) {
	for _, test := range []struct {
		name                      string
		firstType                 string
		secondType                string
		secondConflicts           bool
		expectedInitialLedgerType string
	}{
		{
			name:      "refund then chargeback advances terminal without another withdrawal",
			firstType: SubscriptionOrderRecoveryRefund, secondType: SubscriptionOrderRecoveryChargeback,
			expectedInitialLedgerType: CreditBalanceLedgerTypeRefund,
		},
		{
			name:      "chargeback then refund rejects stale lower terminal",
			firstType: SubscriptionOrderRecoveryChargeback, secondType: SubscriptionOrderRecoveryRefund,
			secondConflicts: true, expectedInitialLedgerType: CreditBalanceLedgerTypeChargeback,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupSubscriptionRecoveryConcurrencyTestDB(t)
			order := seedRecoverableConcurrentCreditOrder(t, db, 10_250, "ordered-"+test.firstType)
			first, err := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
				TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
				RecoveryType: test.firstType, Reason: "first " + test.firstType,
			})
			require.NoError(t, err)
			require.False(t, first.Replayed)
			second, err := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
				TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
				RecoveryType: test.secondType, Reason: "second " + test.secondType,
			})
			if test.secondConflicts {
				require.Nil(t, second)
				require.ErrorIs(t, err, ErrSubscriptionOrderRecoveryConflict)
			} else {
				require.NoError(t, err)
				require.True(t, second.Replayed)
				assert.Equal(t, common.TopUpStatusChargeback, second.Status)
			}
			require.NoError(t, db.First(order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusChargeback, order.Status)
			assert.Equal(t, SubscriptionOrderRecoveryChargeback, order.RecoveryType)
			var ledger CreditBalanceLedger
			require.NoError(t, db.Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).First(&ledger).Error)
			assert.Equal(t, test.expectedInitialLedgerType, ledger.Type)
			var ledgerCount int64
			require.NoError(t, db.Model(&CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Count(&ledgerCount).Error)
			assert.Equal(t, int64(1), ledgerCount)
			var balance UserSubscription
			require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", order.UserId, SubscriptionEntitlementCreditBalance).First(&balance).Error)
			assert.Equal(t, int64(500), balance.TokenUsed)
		})
	}
}

func TestConcurrentRefundAndChargebackRecoverCreditOnceWithChargebackPrecedence(t *testing.T) {
	db := setupSubscriptionRecoveryConcurrencyTestDB(t)
	order := seedRecoverableConcurrentCreditOrder(t, db, 10_301, "concurrent-refund-chargeback")

	type outcome struct {
		recoveryType string
		result       *SubscriptionOrderRecoveryResult
		err          error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var waitGroup sync.WaitGroup
	for _, recoveryType := range []string{SubscriptionOrderRecoveryRefund, SubscriptionOrderRecoveryChargeback} {
		waitGroup.Add(1)
		go func(recoveryType string) {
			defer waitGroup.Done()
			<-start
			result, recoverErr := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
				TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
				RecoveryType: recoveryType, Reason: "concurrent " + recoveryType,
			})
			outcomes <- outcome{recoveryType: recoveryType, result: result, err: recoverErr}
		}(recoveryType)
	}
	close(start)
	waitGroup.Wait()
	close(outcomes)

	freshCount := 0
	replayCount := 0
	conflictCount := 0
	for outcome := range outcomes {
		if outcome.err != nil {
			assert.Equal(t, SubscriptionOrderRecoveryRefund, outcome.recoveryType)
			assert.ErrorIs(t, outcome.err, ErrSubscriptionOrderRecoveryConflict)
			assert.Nil(t, outcome.result)
			conflictCount++
			continue
		}
		require.NotNil(t, outcome.result)
		if outcome.result.Replayed {
			replayCount++
		} else {
			freshCount++
		}
	}
	assert.Equal(t, 1, freshCount)
	assert.Equal(t, 1, replayCount+conflictCount)
	assert.LessOrEqual(t, replayCount, 1)
	assert.LessOrEqual(t, conflictCount, 1)
	require.NoError(t, db.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusChargeback, order.Status)
	assert.Equal(t, SubscriptionOrderRecoveryChargeback, order.RecoveryType)
	var balance UserSubscription
	require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", order.UserId, SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(500), balance.TokenLimit)
	assert.Equal(t, int64(500), balance.TokenUsed)
	var recoveryLedgers []CreditBalanceLedger
	require.NoError(t, db.Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Find(&recoveryLedgers).Error)
	require.Len(t, recoveryLedgers, 1)
	assert.Equal(t, int64(-500), recoveryLedgers[0].GrossCredit)
}

func seedRecoverableConcurrentCreditOrder(t *testing.T, db *gorm.DB, userID int, tradeNo string) *SubscriptionOrder {
	t.Helper()
	optionPlanID := userID + 1
	creditPlanID := userID + 2
	require.NoError(t, db.Create(&User{Id: userID, Username: "recovery-" + tradeNo, Status: common.UserStatusEnabled}).Error)
	optionCode := "option-" + tradeNo
	creditCode := "balance-" + tradeNo
	optionPlan := SubscriptionPlan{
		Id: optionPlanID, Title: "Recovery option", EntitlementType: SubscriptionEntitlementTimed,
		MonthlyTokenLimit: 500, Enabled: true, BusinessCode: &optionCode,
	}
	creditPlan := SubscriptionPlan{
		Id: creditPlanID, Title: "Credit balance", EntitlementType: SubscriptionEntitlementCreditBalance,
		Enabled: true, CreditBalanceConfigured: true, BusinessCode: &creditCode,
	}
	require.NoError(t, db.Create(&optionPlan).Error)
	require.NoError(t, db.Create(&creditPlan).Error)
	snapshot := NewSubscriptionEntitlementSnapshot(&optionPlan, SubscriptionPurchaseModeCreditBalance, creditPlanID)
	snapshot.SetTargetCreditBalancePlanSnapshot(&creditPlan)
	snapshot.SetPaymentSnapshot(PaymentProviderStripe, "price_"+tradeNo, PaymentMethodStripe, 3000, "CNY")
	snapshotJSON, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := &SubscriptionOrder{
		UserId: userID, PlanId: optionPlanID, TradeNo: tradeNo,
		AmountCents: 3000, Currency: "CNY", CreditGrantAmount: 500, CreditTargetPlanID: creditPlanID,
		PaymentProvider: PaymentProviderStripe, PaymentMethod: PaymentMethodStripe,
		Status: common.TopUpStatusSuccess, CompleteTime: common.GetTimestamp(), EntitlementSnapshot: snapshotJSON,
	}
	require.NoError(t, db.Create(order).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, grantErr := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
			UserId: userID, GrossCredit: 500, IdempotencyKey: order.TradeNo,
			SourceType: CreditBalanceLedgerSourceSubscriptionOrder, SourceId: order.Id,
			Type: CreditBalanceLedgerTypePurchase, TargetPlanId: creditPlanID,
			TargetPlanSnapshot: &creditPlan, PaymentProvider: PaymentProviderStripe, Reason: "purchase",
		})
		return grantErr
	}))
	return order
}

func setupSubscriptionRecoveryConcurrencyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "subscription-recovery.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
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
	ClearPrimaryBillableSubscriptionCacheForTest()
	ClearSubscriptionPlanCacheForTest()
	require.NoError(t, db.AutoMigrate(
		&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{},
		&CreditBalanceLedger{}, &SubscriptionConversion{}, &InvitationRewardEvent{},
	))

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
	return db
}
