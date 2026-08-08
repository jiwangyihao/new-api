package model

import (
	"errors"
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
	sqlDB, err := db.DB()
	require.NoError(t, err)
	var journalMode string
	require.NoError(t, db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error)
	require.Equal(t, "wal", journalMode)

	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	var barrierMu sync.Mutex
	barrierEntries := 0
	const barrierCallback = "issue25:refund_chargeback_after_order_read"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(barrierCallback, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "SubscriptionOrder" || tx.Error != nil || tx.RowsAffected != 1 {
			return
		}
		barrierMu.Lock()
		if barrierEntries >= 2 {
			barrierMu.Unlock()
			return
		}
		barrierEntries++
		barrierMu.Unlock()
		arrived <- struct{}{}
		<-release
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(barrierCallback) })

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
	<-arrived
	<-arrived
	require.GreaterOrEqual(t, sqlDB.Stats().InUse, 2, "the barrier must hold two independent SQLite connections")
	close(release)
	waitGroup.Wait()
	close(outcomes)

	byType := make(map[string]outcome, 2)
	freshCount := 0
	replayCount := 0
	conflictCount := 0
	assertStableConflict := func(conflictErr error) {
		require.ErrorIs(t, conflictErr, ErrSubscriptionOrderRecoveryConflict)
		assert.NotContains(t, conflictErr.Error(), "SQLITE")
		assert.NotContains(t, conflictErr.Error(), "database is locked")
		assert.NotContains(t, conflictErr.Error(), "UNIQUE constraint")
		assert.NotContains(t, conflictErr.Error(), "gorm")
	}
	for outcome := range outcomes {
		byType[outcome.recoveryType] = outcome
		if outcome.err != nil {
			assert.Equal(t, SubscriptionOrderRecoveryRefund, outcome.recoveryType)
			assertStableConflict(outcome.err)
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
	require.Len(t, byType, 2)
	assert.Equal(t, 1, freshCount)
	assert.Equal(t, 1, replayCount+conflictCount)

	require.NoError(t, db.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusChargeback, order.Status)
	assert.Equal(t, SubscriptionOrderRecoveryChargeback, order.RecoveryType)
	var balance UserSubscription
	require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", order.UserId, SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(500), balance.TokenLimit)
	assert.Equal(t, int64(500), balance.TokenUsed)
	var recoveryLedgers []CreditBalanceLedger
	require.NoError(t, db.Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Order("id asc").Find(&recoveryLedgers).Error)
	require.Len(t, recoveryLedgers, 1)
	assert.Equal(t, int64(-500), recoveryLedgers[0].GrossCredit)
	assert.Equal(t, recoveryLedgers[0].Id, order.RecoveryLedgerID)
	switch recoveryLedgers[0].Type {
	case CreditBalanceLedgerTypeRefund:
		require.NoError(t, byType[SubscriptionOrderRecoveryRefund].err)
		require.False(t, byType[SubscriptionOrderRecoveryRefund].result.Replayed)
		require.NoError(t, byType[SubscriptionOrderRecoveryChargeback].err)
		require.True(t, byType[SubscriptionOrderRecoveryChargeback].result.Replayed)
	case CreditBalanceLedgerTypeChargeback:
		require.NoError(t, byType[SubscriptionOrderRecoveryChargeback].err)
		require.False(t, byType[SubscriptionOrderRecoveryChargeback].result.Replayed)
		assertStableConflict(byType[SubscriptionOrderRecoveryRefund].err)
	default:
		t.Fatalf("unexpected recovery ledger type %q", recoveryLedgers[0].Type)
	}

	type recoverySnapshot struct {
		Order   SubscriptionOrder
		Balance UserSubscription
		Ledgers []CreditBalanceLedger
	}
	capture := func() recoverySnapshot {
		var snapshot recoverySnapshot
		require.NoError(t, db.First(&snapshot.Order, order.Id).Error)
		require.NoError(t, db.First(&snapshot.Balance, balance.Id).Error)
		require.NoError(t, db.Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Order("id asc").Find(&snapshot.Ledgers).Error)
		return snapshot
	}
	beforeRejectedRefund := capture()
	rejected, rejectErr := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
		TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
		RecoveryType: SubscriptionOrderRecoveryRefund, Reason: "concurrent refund",
	})
	require.Nil(t, rejected)
	assertStableConflict(rejectErr)
	require.Equal(t, beforeRejectedRefund, capture(), "the rejected lower terminal must write nothing")
}

func TestConcurrentRefundAndAdminDecreaseUseLegalSerializations(t *testing.T) {
	db := setupSubscriptionRecoveryConcurrencyTestDB(t)
	order := seedRecoverableConcurrentCreditOrder(t, db, 10_401, "concurrent-refund-admin-decrease")
	require.NoError(t, db.AutoMigrate(&CreditBalanceAdjustment{}))
	require.NoError(t, migrateCreditValuationSchema(db))

	var balance UserSubscription
	require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", order.UserId, SubscriptionEntitlementCreditBalance).First(&balance).Error)
	now := GetDBTimestamp()
	require.NoError(t, db.Create(&CreditValuationState{
		UserSubscriptionId: balance.Id,
		UserId:             order.UserId,
		AvailableCredit:    500,
		ExactCostMicros:    30_000_000,
		Currency:           "CNY",
		RuleVersion:        CreditValuationRuleVersion,
		StateVersion:       1,
		LastMutationType:   CreditValuationMutationGrant,
		CreatedAt:          now,
		UpdatedAt:          now,
	}).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: CreditValuationRuleVersion, Status: CreditValuationMigrationReady,
		ValuationCurrency: "CNY", FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: now,
	}).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	var journalMode string
	require.NoError(t, db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error)
	require.Equal(t, "wal", journalMode)

	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	var barrierMu sync.Mutex
	barrierEntries := 0
	const barrierCallback = "issue25:refund_admin_decrease_after_user_read"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(barrierCallback, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "User" || tx.Error != nil || tx.RowsAffected != 1 {
			return
		}
		barrierMu.Lock()
		if barrierEntries >= 2 {
			barrierMu.Unlock()
			return
		}
		barrierEntries++
		barrierMu.Unlock()
		arrived <- struct{}{}
		<-release
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(barrierCallback) })

	type recoveryOutcome struct {
		result *SubscriptionOrderRecoveryResult
		err    error
	}
	type adjustmentOutcome struct {
		result *CreditBalanceAdjustmentResult
		err    error
	}
	recoveryDone := make(chan recoveryOutcome, 1)
	adjustmentDone := make(chan adjustmentOutcome, 1)
	start := make(chan struct{})
	go func() {
		<-start
		result, recoverErr := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
			TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
			RecoveryType: SubscriptionOrderRecoveryRefund, Reason: "concurrent refund",
		})
		recoveryDone <- recoveryOutcome{result: result, err: recoverErr}
	}()
	go func() {
		<-start
		result, adjustmentErr := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
			UserId: order.UserId, Operation: CreditBalanceAdjustmentDecrease, Amount: 100,
			IdempotencyKey: "concurrent-refund-admin-decrease", OperatorUserId: 10_499,
			Reason: "concurrent admin decrease",
		})
		adjustmentDone <- adjustmentOutcome{result: result, err: adjustmentErr}
	}()
	close(start)
	<-arrived
	<-arrived
	require.GreaterOrEqual(t, sqlDB.Stats().InUse, 2, "the barrier must hold two independent SQLite connections")
	close(release)

	recovery := <-recoveryDone
	adjustment := <-adjustmentDone
	for _, operationErr := range []error{recovery.err, adjustment.err} {
		if operationErr == nil {
			continue
		}
		assert.NotContains(t, operationErr.Error(), "SQLITE")
		assert.NotContains(t, operationErr.Error(), "database is locked")
		assert.NotContains(t, operationErr.Error(), "UNIQUE constraint")
		assert.NotContains(t, operationErr.Error(), "gorm")
	}
	require.NoError(t, recovery.err)
	require.NotNil(t, recovery.result)
	require.False(t, recovery.result.Replayed)
	require.NoError(t, adjustment.err)
	require.NotNil(t, adjustment.result)
	require.False(t, adjustment.result.Replayed)

	require.NoError(t, db.First(order, order.Id).Error)
	require.Equal(t, common.TopUpStatusRefunded, order.Status)
	require.Equal(t, SubscriptionOrderRecoveryRefund, order.RecoveryType)
	require.NoError(t, db.First(&balance, balance.Id).Error)
	require.Equal(t, int64(500), balance.TokenLimit)
	require.Equal(t, int64(600), balance.TokenUsed)
	require.Equal(t, int64(100), maxInt64(balance.TokenUsed-balance.TokenLimit, 0))

	var state CreditValuationState
	require.NoError(t, db.First(&state, balance.Id).Error)
	require.Zero(t, state.AvailableCredit)
	require.Zero(t, state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, int64(3), state.StateVersion)

	var recoveryLedgers []CreditBalanceLedger
	require.NoError(t, db.Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Find(&recoveryLedgers).Error)
	require.Len(t, recoveryLedgers, 1)
	require.Equal(t, recoveryLedgers[0].Id, order.RecoveryLedgerID)
	require.Equal(t, int64(-500), recoveryLedgers[0].GrossCredit)
	var adminLedgers []CreditBalanceLedger
	require.NoError(t, db.Where("source_type = ? AND type = ?", CreditBalanceLedgerSourceAdminAdjustment, CreditBalanceLedgerTypeAdminDecrease).Find(&adminLedgers).Error)
	require.Len(t, adminLedgers, 1)
	require.Equal(t, int64(-100), adminLedgers[0].GrossCredit)
	require.Equal(t, int64(30_000_000), recoveryLedgers[0].ValuationGrossCostMicros+adminLedgers[0].ValuationGrossCostMicros)
	require.Contains(t, []int64{0, 6_000_000}, adminLedgers[0].ValuationGrossCostMicros)
	require.Equal(t, int64(3), maxInt64(recoveryLedgers[0].ValuationStateVersionAfter, adminLedgers[0].ValuationStateVersionAfter))

	type failureSnapshot struct {
		Order       SubscriptionOrder
		Balance     UserSubscription
		State       CreditValuationState
		Ledgers     []CreditBalanceLedger
		Adjustments []CreditBalanceAdjustment
	}
	capture := func() failureSnapshot {
		var snapshot failureSnapshot
		require.NoError(t, db.First(&snapshot.Order, order.Id).Error)
		require.NoError(t, db.First(&snapshot.Balance, balance.Id).Error)
		require.NoError(t, db.First(&snapshot.State, balance.Id).Error)
		require.NoError(t, db.Order("id asc").Find(&snapshot.Ledgers).Error)
		require.NoError(t, db.Order("id asc").Find(&snapshot.Adjustments).Error)
		return snapshot
	}
	beforeConflict := capture()
	conflicted, conflictErr := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
		UserId: order.UserId, Operation: CreditBalanceAdjustmentDecrease, Amount: 101,
		IdempotencyKey: "concurrent-refund-admin-decrease", OperatorUserId: 10_499,
		Reason: "concurrent admin decrease",
	})
	require.Nil(t, conflicted)
	require.ErrorIs(t, conflictErr, ErrCreditValuationIdempotencyMismatch)
	assert.NotContains(t, conflictErr.Error(), "SQLITE")
	assert.NotContains(t, conflictErr.Error(), "database is locked")
	assert.NotContains(t, conflictErr.Error(), "UNIQUE constraint")
	require.Equal(t, beforeConflict, capture(), "the conflicting admin replay must write nothing")
}

func TestConcurrentLowFrequencyOutflowAndRequestFinalSettleUseLegalSerializations(t *testing.T) {
	db := setupSubscriptionRecoveryConcurrencyTestDB(t)
	order := seedRecoverableConcurrentCreditOrder(t, db, 10_501, "concurrent-outflow-request-final")
	require.NoError(t, db.AutoMigrate(&CreditBalanceAdjustment{}))
	require.NoError(t, migrateCreditValuationSchema(db))

	var balance UserSubscription
	require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", order.UserId, SubscriptionEntitlementCreditBalance).First(&balance).Error)
	now := GetDBTimestamp()
	require.NoError(t, db.Create(&CreditValuationState{
		UserSubscriptionId: balance.Id,
		UserId:             order.UserId,
		AvailableCredit:    500,
		ExactCostMicros:    30_000_000,
		Currency:           "CNY",
		RuleVersion:        CreditValuationRuleVersion,
		StateVersion:       1,
		LastMutationType:   CreditValuationMutationGrant,
		CreatedAt:          now,
		UpdatedAt:          now,
	}).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: CreditValuationRuleVersion, Status: CreditValuationMigrationReady,
		ValuationCurrency: "CNY", FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: now,
	}).Error)

	const targetRequestID = "concurrent-outflow-request-final-target"
	target, err := PreConsumeUserSubscriptionByUnits(targetRequestID, order.UserId, "gpt-4o", 0, 0, 100)
	require.NoError(t, err)
	require.Equal(t, balance.Id, target.UserSubscriptionId)
	const activeRequestID = "concurrent-outflow-request-final-active"
	active, err := PreConsumeUserSubscriptionByUnits(activeRequestID, order.UserId, "gpt-4o", 0, 0, 50)
	require.NoError(t, err)
	require.Equal(t, balance.Id, active.UserSubscriptionId)
	var targetBefore SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", targetRequestID).First(&targetBefore).Error)
	require.Equal(t, int64(100), targetBefore.DeductedAvailableCredit)
	require.Equal(t, int64(6_000_000), targetBefore.DeductedExactCostMicros)
	var activeBefore SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", activeRequestID).First(&activeBefore).Error)
	require.Equal(t, int64(3_000_000), activeBefore.DeductedExactCostMicros)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	var journalMode string
	require.NoError(t, db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error)
	require.Equal(t, "wal", journalMode)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var barrierMu sync.Mutex
	barrierEntries := 0
	const barrierCallback = "issue25:outflow_request_final_after_balance_read"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(barrierCallback, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "UserSubscription" || tx.Error != nil || tx.RowsAffected != 1 {
			return
		}
		barrierMu.Lock()
		if barrierEntries >= 2 {
			barrierMu.Unlock()
			return
		}
		barrierEntries++
		barrierMu.Unlock()
		arrived <- struct{}{}
		<-release
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(barrierCallback) })

	type settleOutcome struct{ err error }
	type outflowOutcome struct {
		result *CreditBalanceAdjustmentResult
		err    error
	}
	settleDone := make(chan settleOutcome, 1)
	outflowDone := make(chan outflowOutcome, 1)
	start := make(chan struct{})
	go func() {
		<-start
		settleDone <- settleOutcome{err: SettleUserSubscriptionRequestTarget(targetRequestID, balance.Id, 200, true)}
	}()
	go func() {
		<-start
		result, outflowErr := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
			UserId: order.UserId, Operation: CreditBalanceAdjustmentDecrease, Amount: 400,
			IdempotencyKey: "concurrent-outflow-request-final", OperatorUserId: 10_599,
			Reason: "concurrent low-frequency outflow",
		})
		outflowDone <- outflowOutcome{result: result, err: outflowErr}
	}()
	close(start)
	<-arrived
	<-arrived
	require.GreaterOrEqual(t, sqlDB.Stats().InUse, 2, "the barrier must hold two independent SQLite connections")
	releaseOnce.Do(func() { close(release) })
	settled := <-settleDone
	outflow := <-outflowDone
	require.NoError(t, settled.err)
	require.NoError(t, outflow.err)
	require.NotNil(t, outflow.result)
	require.False(t, outflow.result.Replayed)

	require.NoError(t, db.First(&balance, balance.Id).Error)
	require.Equal(t, int64(500), balance.TokenLimit)
	require.Equal(t, int64(650), balance.TokenUsed)
	require.Equal(t, int64(150), maxInt64(balance.TokenUsed-balance.TokenLimit, 0))
	var state CreditValuationState
	require.NoError(t, db.First(&state, balance.Id).Error)
	require.Zero(t, state.AvailableCredit)
	require.Zero(t, state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, int64(5), state.StateVersion)

	var terminal SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", targetRequestID).First(&terminal).Error)
	require.Equal(t, int64(200), terminal.AppliedCredit)
	require.Equal(t, "settled", terminal.Status)
	require.Positive(t, terminal.FinalizedAt)
	require.Equal(t, targetBefore.UserSubscriptionId, terminal.UserSubscriptionId)
	require.Equal(t, targetBefore.ValuationSubscriptionId, terminal.ValuationSubscriptionId)
	var activeAfter SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", activeRequestID).First(&activeAfter).Error)
	require.Equal(t, activeBefore, activeAfter, "low-frequency outflow must not rewrite another active request snapshot")

	var adminLedgers []CreditBalanceLedger
	require.NoError(t, db.Where("source_type = ? AND type = ?", CreditBalanceLedgerSourceAdminAdjustment, CreditBalanceLedgerTypeAdminDecrease).Find(&adminLedgers).Error)
	require.Len(t, adminLedgers, 1)
	require.Equal(t, int64(-400), adminLedgers[0].GrossCredit)
	type legalSerialization struct {
		AdminCostMicros        int64
		AdminStateVersion      int64
		TargetAvailableCredit  int64
		TargetDebtFormedCredit int64
		TargetExactCostMicros  int64
	}
	actual := legalSerialization{
		AdminCostMicros:        adminLedgers[0].ValuationGrossCostMicros,
		AdminStateVersion:      adminLedgers[0].ValuationStateVersionAfter,
		TargetAvailableCredit:  terminal.DeductedAvailableCredit,
		TargetDebtFormedCredit: terminal.DebtFormedCredit,
		TargetExactCostMicros:  terminal.DeductedExactCostMicros,
	}
	require.Contains(t, []legalSerialization{
		{AdminCostMicros: 15_000_000, AdminStateVersion: 5, TargetAvailableCredit: 200, TargetExactCostMicros: 12_000_000},
		{AdminCostMicros: 21_000_000, AdminStateVersion: 4, TargetAvailableCredit: 100, TargetDebtFormedCredit: 100, TargetExactCostMicros: 6_000_000},
	}, actual)
	require.Equal(t, int64(21_000_000), adminLedgers[0].ValuationGrossCostMicros+(terminal.DeductedExactCostMicros-targetBefore.DeductedExactCostMicros))

	type failureSnapshot struct {
		Balance     UserSubscription
		State       CreditValuationState
		Terminal    SubscriptionPreConsumeRecord
		Active      SubscriptionPreConsumeRecord
		Ledgers     []CreditBalanceLedger
		Adjustments []CreditBalanceAdjustment
	}
	capture := func() failureSnapshot {
		var snapshot failureSnapshot
		require.NoError(t, db.First(&snapshot.Balance, balance.Id).Error)
		require.NoError(t, db.First(&snapshot.State, balance.Id).Error)
		require.NoError(t, db.Where("request_id = ?", targetRequestID).First(&snapshot.Terminal).Error)
		require.NoError(t, db.Where("request_id = ?", activeRequestID).First(&snapshot.Active).Error)
		require.NoError(t, db.Order("id asc").Find(&snapshot.Ledgers).Error)
		require.NoError(t, db.Order("id asc").Find(&snapshot.Adjustments).Error)
		return snapshot
	}
	beforeConflict := capture()
	conflictErr := SettleUserSubscriptionRequestTarget(targetRequestID, balance.Id, 201, true)
	require.ErrorIs(t, conflictErr, ErrCreditValuationFinalizedConflict)
	assert.NotContains(t, conflictErr.Error(), "SQLITE")
	assert.NotContains(t, conflictErr.Error(), "database is locked")
	assert.NotContains(t, conflictErr.Error(), "UNIQUE constraint")
	require.Equal(t, beforeConflict, capture(), "the conflicting final settle must write nothing")
}

func TestConcurrentLowFrequencyOutflowAndRequestRefundUseLegalSerializations(t *testing.T) {
	db := setupSubscriptionRecoveryConcurrencyTestDB(t)
	order := seedRecoverableConcurrentCreditOrder(t, db, 10_601, "concurrent-outflow-request-refund")
	require.NoError(t, db.AutoMigrate(&CreditBalanceAdjustment{}))
	require.NoError(t, migrateCreditValuationSchema(db))

	var balance UserSubscription
	require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", order.UserId, SubscriptionEntitlementCreditBalance).First(&balance).Error)
	now := GetDBTimestamp()
	require.NoError(t, db.Create(&CreditValuationState{
		UserSubscriptionId: balance.Id,
		UserId:             order.UserId,
		AvailableCredit:    500,
		ExactCostMicros:    30_000_000,
		Currency:           "CNY",
		RuleVersion:        CreditValuationRuleVersion,
		StateVersion:       1,
		LastMutationType:   CreditValuationMutationGrant,
		CreatedAt:          now,
		UpdatedAt:          now,
	}).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: CreditValuationRuleVersion, Status: CreditValuationMigrationReady,
		ValuationCurrency: "CNY", FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: now,
	}).Error)

	const refundedRequestID = "concurrent-outflow-request-refund-target"
	refunded, err := PreConsumeUserSubscriptionByUnits(refundedRequestID, order.UserId, "gpt-4o", 0, 0, 100)
	require.NoError(t, err)
	require.Equal(t, balance.Id, refunded.UserSubscriptionId)
	const activeRequestID = "concurrent-outflow-request-refund-active"
	active, err := PreConsumeUserSubscriptionByUnits(activeRequestID, order.UserId, "gpt-4o", 0, 0, 50)
	require.NoError(t, err)
	require.Equal(t, balance.Id, active.UserSubscriptionId)
	var refundedBefore SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", refundedRequestID).First(&refundedBefore).Error)
	require.Equal(t, int64(100), refundedBefore.DeductedAvailableCredit)
	require.Equal(t, int64(6_000_000), refundedBefore.DeductedExactCostMicros)
	var activeBefore SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", activeRequestID).First(&activeBefore).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	var journalMode string
	require.NoError(t, db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error)
	require.Equal(t, "wal", journalMode)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var barrierMu sync.Mutex
	barrierEntries := 0
	const barrierCallback = "issue25:outflow_request_refund_after_balance_read"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(barrierCallback, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "UserSubscription" || tx.Error != nil || tx.RowsAffected != 1 {
			return
		}
		barrierMu.Lock()
		if barrierEntries >= 2 {
			barrierMu.Unlock()
			return
		}
		barrierEntries++
		barrierMu.Unlock()
		arrived <- struct{}{}
		<-release
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(barrierCallback) })

	type refundOutcome struct{ err error }
	type outflowOutcome struct {
		result *CreditBalanceAdjustmentResult
		err    error
	}
	refundDone := make(chan refundOutcome, 1)
	outflowDone := make(chan outflowOutcome, 1)
	start := make(chan struct{})
	go func() {
		<-start
		refundDone <- refundOutcome{err: SettleUserSubscriptionRequestTarget(refundedRequestID, balance.Id, 0, true)}
	}()
	go func() {
		<-start
		result, outflowErr := AdjustCreditBalance(CreditBalanceAdjustmentRequest{
			UserId: order.UserId, Operation: CreditBalanceAdjustmentDecrease, Amount: 400,
			IdempotencyKey: "concurrent-outflow-request-refund", OperatorUserId: 10_699,
			Reason: "concurrent low-frequency outflow",
		})
		outflowDone <- outflowOutcome{result: result, err: outflowErr}
	}()
	close(start)
	<-arrived
	<-arrived
	require.GreaterOrEqual(t, sqlDB.Stats().InUse, 2, "the barrier must hold two independent SQLite connections")
	releaseOnce.Do(func() { close(release) })
	refundResult := <-refundDone
	outflow := <-outflowDone
	require.NoError(t, refundResult.err)
	require.NoError(t, outflow.err)
	require.NotNil(t, outflow.result)
	require.False(t, outflow.result.Replayed)

	require.NoError(t, db.First(&balance, balance.Id).Error)
	require.Equal(t, int64(500), balance.TokenLimit)
	require.Equal(t, int64(450), balance.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, balance.Id).Error)
	require.Equal(t, int64(50), state.AvailableCredit)
	require.Equal(t, int64(3_000_000), state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, int64(5), state.StateVersion)

	var terminal SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", refundedRequestID).First(&terminal).Error)
	require.Zero(t, terminal.AppliedCredit)
	require.Zero(t, terminal.DeductedAvailableCredit)
	require.Zero(t, terminal.DeductedExactCostMicros)
	require.Equal(t, "refunded", terminal.Status)
	require.Positive(t, terminal.FinalizedAt)
	require.Equal(t, refundedBefore.UserSubscriptionId, terminal.UserSubscriptionId)
	require.Equal(t, refundedBefore.ValuationSubscriptionId, terminal.ValuationSubscriptionId)
	var activeAfter SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", activeRequestID).First(&activeAfter).Error)
	require.Equal(t, activeBefore, activeAfter, "low-frequency outflow must not rewrite another active request snapshot")

	var adminLedgers []CreditBalanceLedger
	require.NoError(t, db.Where("source_type = ? AND type = ?", CreditBalanceLedgerSourceAdminAdjustment, CreditBalanceLedgerTypeAdminDecrease).Find(&adminLedgers).Error)
	require.Len(t, adminLedgers, 1)
	require.Equal(t, int64(-400), adminLedgers[0].GrossCredit)
	type legalSerialization struct {
		AdminCostMicros          int64
		AdminStateVersion        int64
		AbsorbedRestoreExactCost int64
	}
	require.Contains(t, []legalSerialization{
		{AdminCostMicros: 24_000_000, AdminStateVersion: 5},
		{AdminCostMicros: 21_000_000, AdminStateVersion: 4, AbsorbedRestoreExactCost: 3_000_000},
	}, legalSerialization{
		AdminCostMicros:          adminLedgers[0].ValuationGrossCostMicros,
		AdminStateVersion:        adminLedgers[0].ValuationStateVersionAfter,
		AbsorbedRestoreExactCost: terminal.AbsorbedRestoreExactCostMicros,
	})
	require.Equal(t, int64(30_000_000), adminLedgers[0].ValuationGrossCostMicros+state.ExactCostMicros+activeAfter.DeductedExactCostMicros+terminal.AbsorbedRestoreExactCostMicros)

	type failureSnapshot struct {
		Balance     UserSubscription
		State       CreditValuationState
		Terminal    SubscriptionPreConsumeRecord
		Active      SubscriptionPreConsumeRecord
		Ledgers     []CreditBalanceLedger
		Adjustments []CreditBalanceAdjustment
	}
	capture := func() failureSnapshot {
		var snapshot failureSnapshot
		require.NoError(t, db.First(&snapshot.Balance, balance.Id).Error)
		require.NoError(t, db.First(&snapshot.State, balance.Id).Error)
		require.NoError(t, db.Where("request_id = ?", refundedRequestID).First(&snapshot.Terminal).Error)
		require.NoError(t, db.Where("request_id = ?", activeRequestID).First(&snapshot.Active).Error)
		require.NoError(t, db.Order("id asc").Find(&snapshot.Ledgers).Error)
		require.NoError(t, db.Order("id asc").Find(&snapshot.Adjustments).Error)
		return snapshot
	}
	beforeConflict := capture()
	conflictErr := SettleUserSubscriptionRequestTarget(refundedRequestID, balance.Id, 1, true)
	require.ErrorIs(t, conflictErr, ErrCreditValuationFinalizedConflict)
	assert.NotContains(t, conflictErr.Error(), "SQLITE")
	assert.NotContains(t, conflictErr.Error(), "database is locked")
	assert.NotContains(t, conflictErr.Error(), "UNIQUE constraint")
	require.Equal(t, beforeConflict, capture(), "the conflicting refund replay must write nothing")
}

func TestCreditRequestTargetInjectedFailureRollsBackAtomically(t *testing.T) {
	for _, test := range []struct {
		name   string
		target int64
	}{
		{name: "final settle", target: 200},
		{name: "refund", target: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupSubscriptionRecoveryConcurrencyTestDB(t)
			order := seedRecoverableConcurrentCreditOrder(t, db, 10_701, "request-target-injected-failure")
			require.NoError(t, migrateCreditValuationSchema(db))

			var balance UserSubscription
			require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", order.UserId, SubscriptionEntitlementCreditBalance).First(&balance).Error)
			now := GetDBTimestamp()
			require.NoError(t, db.Create(&CreditValuationState{
				UserSubscriptionId: balance.Id,
				UserId:             order.UserId,
				AvailableCredit:    500,
				ExactCostMicros:    30_000_000,
				Currency:           "CNY",
				RuleVersion:        CreditValuationRuleVersion,
				StateVersion:       1,
				LastMutationType:   CreditValuationMutationGrant,
				CreatedAt:          now,
				UpdatedAt:          now,
			}).Error)
			require.NoError(t, db.Create(&CreditValuationMigration{
				Version: CreditValuationRuleVersion, Status: CreditValuationMigrationReady,
				ValuationCurrency: "CNY", FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: now,
			}).Error)

			const targetRequestID = "request-target-injected-failure-target"
			target, err := PreConsumeUserSubscriptionByUnits(targetRequestID, order.UserId, "gpt-4o", 0, 0, 100)
			require.NoError(t, err)
			require.Equal(t, balance.Id, target.UserSubscriptionId)
			const activeRequestID = "request-target-injected-failure-active"
			active, err := PreConsumeUserSubscriptionByUnits(activeRequestID, order.UserId, "gpt-4o", 0, 0, 50)
			require.NoError(t, err)
			require.Equal(t, balance.Id, active.UserSubscriptionId)

			type transactionSnapshot struct {
				Balance UserSubscription
				State   CreditValuationState
				Target  SubscriptionPreConsumeRecord
				Active  SubscriptionPreConsumeRecord
				Ledgers []CreditBalanceLedger
			}
			capture := func() transactionSnapshot {
				var snapshot transactionSnapshot
				require.NoError(t, db.First(&snapshot.Balance, balance.Id).Error)
				require.NoError(t, db.First(&snapshot.State, balance.Id).Error)
				require.NoError(t, db.Where("request_id = ?", targetRequestID).First(&snapshot.Target).Error)
				require.NoError(t, db.Where("request_id = ?", activeRequestID).First(&snapshot.Active).Error)
				require.NoError(t, db.Order("id asc").Find(&snapshot.Ledgers).Error)
				return snapshot
			}
			before := capture()

			injectedErr := errors.New("injected request target transaction failure")
			injected := false
			callbackName := "issue25:inject_request_target_failure_" + test.name
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				if injected || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "SubscriptionPreConsumeRecord" {
					return
				}
				injected = true
				tx.AddError(injectedErr)
			}))
			t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })

			err = SettleUserSubscriptionRequestTarget(targetRequestID, balance.Id, test.target, true)
			require.ErrorIs(t, err, injectedErr)
			require.True(t, injected)
			require.Equal(t, before, capture(), "the injected middle failure must roll back the whole request transaction")
		})
	}
}

func TestCreditRequestTargetPersistentSQLiteLockReturnsStableConflictWithoutWrites(t *testing.T) {
	db := setupSubscriptionRecoveryConcurrencyTestDB(t)
	order := seedRecoverableConcurrentCreditOrder(t, db, 10_801, "request-target-persistent-lock")
	require.NoError(t, migrateCreditValuationSchema(db))

	var balance UserSubscription
	require.NoError(t, db.Where("user_id = ? AND entitlement_type = ?", order.UserId, SubscriptionEntitlementCreditBalance).First(&balance).Error)
	now := GetDBTimestamp()
	require.NoError(t, db.Create(&CreditValuationState{
		UserSubscriptionId: balance.Id,
		UserId:             order.UserId,
		AvailableCredit:    500,
		ExactCostMicros:    30_000_000,
		Currency:           "CNY",
		RuleVersion:        CreditValuationRuleVersion,
		StateVersion:       1,
		LastMutationType:   CreditValuationMutationGrant,
		CreatedAt:          now,
		UpdatedAt:          now,
	}).Error)
	require.NoError(t, db.Create(&CreditValuationMigration{
		Version: CreditValuationRuleVersion, Status: CreditValuationMigrationReady,
		ValuationCurrency: "CNY", FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: now,
	}).Error)

	const targetRequestID = "request-target-persistent-lock-target"
	target, err := PreConsumeUserSubscriptionByUnits(targetRequestID, order.UserId, "gpt-4o", 0, 0, 100)
	require.NoError(t, err)
	require.Equal(t, balance.Id, target.UserSubscriptionId)
	const activeRequestID = "request-target-persistent-lock-active"
	active, err := PreConsumeUserSubscriptionByUnits(activeRequestID, order.UserId, "gpt-4o", 0, 0, 50)
	require.NoError(t, err)
	require.Equal(t, balance.Id, active.UserSubscriptionId)

	type transactionSnapshot struct {
		User    User
		Balance UserSubscription
		State   CreditValuationState
		Target  SubscriptionPreConsumeRecord
		Active  SubscriptionPreConsumeRecord
		Ledgers []CreditBalanceLedger
	}
	capture := func() transactionSnapshot {
		var snapshot transactionSnapshot
		require.NoError(t, db.First(&snapshot.User, order.UserId).Error)
		require.NoError(t, db.First(&snapshot.Balance, balance.Id).Error)
		require.NoError(t, db.First(&snapshot.State, balance.Id).Error)
		require.NoError(t, db.Where("request_id = ?", targetRequestID).First(&snapshot.Target).Error)
		require.NoError(t, db.Where("request_id = ?", activeRequestID).First(&snapshot.Active).Error)
		require.NoError(t, db.Order("id asc").Find(&snapshot.Ledgers).Error)
		return snapshot
	}
	before := capture()

	locker := db.Begin()
	require.NoError(t, locker.Error)
	lockerClosed := false
	defer func() {
		if !lockerClosed {
			_ = locker.Rollback().Error
		}
	}()
	require.NoError(t, locker.Model(&User{}).Where("id = ?", order.UserId).UpdateColumn("username", "request-target-lock-holder").Error)

	err = SettleUserSubscriptionRequestTarget(targetRequestID, balance.Id, 200, true)
	require.ErrorIs(t, err, ErrCreditValuationStateMismatch)
	require.Equal(t, ErrCreditValuationStateMismatch.Error(), err.Error())
	assert.NotContains(t, err.Error(), "SQLITE")
	assert.NotContains(t, err.Error(), "database is locked")
	assert.NotContains(t, err.Error(), "UNIQUE constraint")
	require.NoError(t, locker.Rollback().Error)
	lockerClosed = true
	require.Equal(t, before, capture(), "retry exhaustion and the rolled-back lock holder must leave no writes")
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
