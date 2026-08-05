package model

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func completeAdditionalCreditValuationOrder(t *testing.T, db *gorm.DB, user User, option SubscriptionPlan, creditPlan SubscriptionPlan, orderID int, priceMicros int64) *SubscriptionOrderCompletionResult {
	t.Helper()
	option.PriceAmount = float64(priceMicros) / float64(amountMicrosPerUnit)
	option.PriceAmountMicros = &priceMicros
	snapshot := NewSubscriptionEntitlementSnapshot(&option, SubscriptionPurchaseModeCreditBalance, creditPlan.Id)
	snapshot.SetTargetCreditBalancePlanSnapshot(&creditPlan)
	snapshot.SetPaymentSnapshot(PaymentProviderBalance, "controlled-product", PaymentMethodAccountBalance, priceMicros/10_000, "CNY")
	payload, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := SubscriptionOrder{
		Id:                  orderID,
		UserId:              user.Id,
		PlanId:              option.Id,
		Money:               option.PriceAmount,
		AmountCents:         priceMicros / 10_000,
		Currency:            "CNY",
		CreditGrantAmount:   option.MonthlyTokenLimit,
		CreditTargetPlanID:  creditPlan.Id,
		TradeNo:             fmt.Sprintf("credit-valuation-additional-%d", orderID),
		PaymentMethod:       PaymentMethodAccountBalance,
		PaymentProvider:     PaymentProviderBalance,
		Status:              common.TopUpStatusPending,
		EntitlementSnapshot: payload,
	}
	require.NoError(t, db.Create(&order).Error)
	return completeCreditValuationOrder(t, db, &order)
}

type subscriptionPreConsumeReplaySnapshot struct {
	Record       SubscriptionPreConsumeRecord
	Subscription UserSubscription
	Valuation    CreditValuationState
	RecordCount  int64
	LedgerCount  int64
}

func captureSubscriptionPreConsumeReplaySnapshot(t *testing.T, db *gorm.DB, requestID string, subscriptionID int) subscriptionPreConsumeReplaySnapshot {
	t.Helper()
	var snapshot subscriptionPreConsumeReplaySnapshot
	require.NoError(t, db.Where("request_id = ?", requestID).First(&snapshot.Record).Error)
	require.NoError(t, db.First(&snapshot.Subscription, subscriptionID).Error)
	require.NoError(t, db.Where("user_subscription_id = ?", subscriptionID).First(&snapshot.Valuation).Error)
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).Count(&snapshot.RecordCount).Error)
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Count(&snapshot.LedgerCount).Error)
	return snapshot
}

func TestPreConsumeUserSubscriptionByUnitsRejectsConflictingRequestReplayWithoutWrites(t *testing.T) {
	tests := []struct {
		name                    string
		replayUserOffset        int
		replayModel             string
		replayQuotaType         int
		replayDistributorAmount int64
	}{
		{name: "different user", replayUserOffset: 1, replayModel: "gpt-4o-gizmo-original", replayDistributorAmount: 200},
		{name: "different normalized model", replayModel: "gpt-4-gizmo-original", replayDistributorAmount: 200},
		{name: "different quota type", replayModel: "gpt-4o-gizmo-original", replayQuotaType: 1, replayDistributorAmount: 200},
		{name: "different distributor amount", replayModel: "gpt-4o-gizmo-original", replayDistributorAmount: 201},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupCreditValuationTracerTestDB(t)
			user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
			completed := completeCreditValuationOrder(t, db, &order)
			const requestID = "pre-consume-fingerprint-conflict"
			first, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o-gizmo-original", 0, 0, 200)
			require.NoError(t, err)
			require.Equal(t, completed.CreditBalance.UserSubscriptionId, first.UserSubscriptionId)

			replayUserID := user.Id + test.replayUserOffset
			before := captureSubscriptionPreConsumeReplaySnapshot(t, db, requestID, first.UserSubscriptionId)

			_, err = PreConsumeUserSubscriptionByUnits(requestID, replayUserID, test.replayModel, test.replayQuotaType, 0, test.replayDistributorAmount)
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrSubscriptionPreConsumeRequestConflict))

			after := captureSubscriptionPreConsumeReplaySnapshot(t, db, requestID, first.UserSubscriptionId)
			require.Equal(t, before, after)
		})
	}
}

func TestPreConsumeUserSubscriptionByUnitsReplaysEquivalentNormalizedRequestWithoutWrites(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	const requestID = "pre-consume-fingerprint-replay"

	first, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o-gizmo-original", 0, 0, 200)
	require.NoError(t, err)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, first.UserSubscriptionId)
	before := captureSubscriptionPreConsumeReplaySnapshot(t, db, requestID, first.UserSubscriptionId)

	replayed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o-gizmo-retry", 0, 0, 200)
	require.NoError(t, err)
	require.Equal(t, first.UserSubscriptionId, replayed.UserSubscriptionId)
	require.Equal(t, first.PreConsumed, replayed.PreConsumed)
	require.Equal(t, first.AppliedCredit, replayed.AppliedCredit)
	require.Equal(t, first.TokenUsedAfter, replayed.TokenUsedAfter)

	after := captureSubscriptionPreConsumeReplaySnapshot(t, db, requestID, first.UserSubscriptionId)
	require.Equal(t, before, after)
}

func TestPreConsumeUserSubscriptionByUnitsRejectsMissingRequestFingerprintWithoutWrites(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	const requestID = "pre-consume-fingerprint-missing"

	first, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, first.UserSubscriptionId)
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", requestID).
		UpdateColumns(map[string]any{
			"request_fingerprint":         "",
			"request_fingerprint_version": 0,
		}).Error)
	before := captureSubscriptionPreConsumeReplaySnapshot(t, db, requestID, first.UserSubscriptionId)

	_, err = PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSubscriptionPreConsumeRequestConflict))

	after := captureSubscriptionPreConsumeReplaySnapshot(t, db, requestID, first.UserSubscriptionId)
	require.Equal(t, before, after)
}

func TestPreConsumeUserSubscriptionByUnitsConcurrentSameRequestHasSingleWrite(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)

	const requestID = "pre-consume-fingerprint-concurrent"
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	type result struct {
		preConsumed *SubscriptionPreConsumeResult
		err         error
	}
	results := make(chan result, 2)
	var start sync.WaitGroup
	start.Add(2)
	for range 2 {
		go func() {
			start.Done()
			start.Wait()
			var barrierOnce sync.Once
			preConsumed, consumeErr := preConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200, &subscriptionTransactionHooks{
				onPreConsumeAttemptStarted: func() {
					barrierOnce.Do(func() {
						ready <- struct{}{}
						<-release
					})
				},
			})
			results <- result{preConsumed: preConsumed, err: consumeErr}
		}()
	}
	<-ready
	<-ready
	close(release)

	successes := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			require.ErrorIs(t, result.err, ErrSubscriptionPreConsumeRequestConflict)
			continue
		}
		successes++
		require.NotNil(t, result.preConsumed)
		require.Equal(t, completed.CreditBalance.UserSubscriptionId, result.preConsumed.UserSubscriptionId)
		require.Equal(t, int64(200), result.preConsumed.PreConsumed)
	}
	require.GreaterOrEqual(t, successes, 1)

	snapshot := captureSubscriptionPreConsumeReplaySnapshot(t, db, requestID, completed.CreditBalance.UserSubscriptionId)
	require.Equal(t, int64(1), snapshot.RecordCount)
	require.Equal(t, int64(200), snapshot.Record.PreConsumed)
	require.Equal(t, int64(200), snapshot.Record.AppliedCredit)
	require.Equal(t, int64(200), snapshot.Subscription.TokenUsed)
	require.Equal(t, int64(800), snapshot.Valuation.AvailableCredit)
	require.Equal(t, int64(2), snapshot.Valuation.StateVersion)
}

func TestCreditValuationRequestTargetIncreaseUsesCurrentPool(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-target-increase"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)

	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 300, false))

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, preConsumed.UserSubscriptionId).Error)
	require.Equal(t, int64(300), subscription.TokenUsed)

	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(700), state.AvailableCredit)
	require.Equal(t, int64(28_000_000), state.ExactCostMicros)
	require.Equal(t, int64(3), state.StateVersion)

	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(300), record.AppliedCredit)
	require.Equal(t, int64(300), record.DeductedAvailableCredit)
	require.Zero(t, record.DebtFormedCredit)
	require.Equal(t, int64(12_000_000), record.DeductedExactCostMicros)
	require.Equal(t, int64(2), record.SettlementVersion)
	require.Zero(t, record.FinalizedAt)
	require.Equal(t, "consumed", record.Status)
}

func TestCreditValuationRequestTargetDecreaseRestoresOriginalSnapshot(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, option, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-target-decrease"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)

	completeAdditionalCreditValuationOrder(t, db, user, option, creditPlan, 91_005, 20_000_000)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 100, true))

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, preConsumed.UserSubscriptionId).Error)
	require.Equal(t, int64(2_000), subscription.TokenLimit)
	require.Equal(t, int64(100), subscription.TokenUsed)

	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(1_900), state.AvailableCredit)
	require.Equal(t, int64(56_000_000), state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)
	require.Equal(t, int64(4), state.StateVersion)

	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(100), record.AppliedCredit)
	require.Equal(t, int64(100), record.DeductedAvailableCredit)
	require.Zero(t, record.DebtFormedCredit)
	require.Equal(t, int64(4_000_000), record.DeductedExactCostMicros)
	require.Equal(t, int64(2), record.SettlementVersion)
	require.Equal(t, "settled", record.Status)
	require.NotZero(t, record.FinalizedAt)
}

func TestCreditValuationRequestTargetDecreaseRefundsDebtBeforeSnapshot(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-debt-first"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 900)
	require.NoError(t, err)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 1_100, false))
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 950, true))

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, preConsumed.UserSubscriptionId).Error)
	require.Equal(t, int64(950), subscription.TokenUsed)

	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(50), state.AvailableCredit)
	require.Equal(t, int64(2_000_000), state.ExactCostMicros)
	require.Zero(t, state.UnknownCredit)

	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(950), record.AppliedCredit)
	require.Equal(t, int64(950), record.DeductedAvailableCredit)
	require.Zero(t, record.DebtFormedCredit)
	require.Equal(t, int64(38_000_000), record.DeductedExactCostMicros)
	require.Zero(t, record.AbsorbedRestoreExactCostMicros)
	require.Zero(t, record.RestoredUnknownCredit)
	require.Equal(t, "settled", record.Status)
}

func TestCreditValuationRequestTargetRejectsOriginalSubscriptionMismatch(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-original-subscription-mismatch"
	_, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)

	err = SettleUserSubscriptionRequestTarget(requestID, completed.CreditBalance.UserSubscriptionId+1, 200, true)
	require.ErrorIs(t, err, ErrCreditValuationMappingConflict)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, completed.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(200), subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(2), state.StateVersion)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Zero(t, record.FinalizedAt)
	require.Equal(t, int64(1), record.SettlementVersion)
}

func TestCreditValuationRequestTargetDecreaseAuditsRestoreAbsorbedByOtherDebt(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const restoredRequestID = "credit-request-absorbed-restore"
	restoredRequest, err := PreConsumeUserSubscriptionByUnits(restoredRequestID, user.Id, "gpt-4o", 0, 0, 400)
	require.NoError(t, err)
	const debtRequestID = "credit-request-other-debt"
	debtRequest, err := PreConsumeUserSubscriptionByUnits(debtRequestID, user.Id, "gpt-4o", 0, 0, 600)
	require.NoError(t, err)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, restoredRequest.UserSubscriptionId)
	require.Equal(t, restoredRequest.UserSubscriptionId, debtRequest.UserSubscriptionId)
	require.NoError(t, SettleUserSubscriptionRequestTarget(debtRequestID, debtRequest.UserSubscriptionId, 800, false))

	require.NoError(t, SettleUserSubscriptionRequestTarget(restoredRequestID, restoredRequest.UserSubscriptionId, 0, true))

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, restoredRequest.UserSubscriptionId).Error)
	require.Equal(t, int64(800), subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(200), state.AvailableCredit)
	require.Equal(t, int64(8_000_000), state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Zero(t, state.UnknownCredit)

	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", restoredRequestID).First(&record).Error)
	require.Zero(t, record.AppliedCredit)
	require.Zero(t, record.DeductedAvailableCredit)
	require.Zero(t, record.DeductedExactCostMicros)
	require.Equal(t, int64(8_000_000), record.AbsorbedRestoreExactCostMicros)
	require.Zero(t, record.AbsorbedRestoreEstimatedCostMicros)
	require.Zero(t, record.AbsorbedRestoreUnknownCredit)
	require.Zero(t, record.RestoredUnknownCredit)
	require.Equal(t, "refunded", record.Status)
}

func TestCreditValuationRequestTargetDecreaseMarksReopenedDebtUnknown(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, option, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-reopened-debt-unknown"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 1_000)
	require.NoError(t, err)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 1_200, false))

	option.MonthlyTokenLimit = 200
	additional := completeAdditionalCreditValuationOrder(t, db, user, option, creditPlan, 91_006, 8_000_000)
	require.Equal(t, int64(200), additional.CreditBalance.GrossCredit)
	require.Equal(t, int64(200), additional.CreditBalance.DebtOffset)
	require.Zero(t, additional.CreditBalance.AvailableCredit)
	require.Zero(t, additional.CreditBalance.SettlementDebt)

	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 1_000, true))

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, preConsumed.UserSubscriptionId).Error)
	require.Equal(t, int64(1_200), subscription.TokenLimit)
	require.Equal(t, int64(1_000), subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(200), state.AvailableCredit)
	require.Zero(t, state.ExactCostMicros)
	require.Zero(t, state.EstimatedCostMicros)
	require.Equal(t, int64(200), state.UnknownCredit)

	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(1_000), record.AppliedCredit)
	require.Zero(t, record.DebtFormedCredit)
	require.Equal(t, int64(200), record.RestoredUnknownCredit)
	require.Zero(t, record.AbsorbedRestoreExactCostMicros)
	require.Equal(t, "settled", record.Status)
}

func TestCreditValuationRequestTargetFullRestoreClearsSnapshotRemainders(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, option, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	option.MonthlyTokenLimit = 3
	additional := completeAdditionalCreditValuationOrder(t, db, user, option, creditPlan, 91_007, 10_000)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, additional.CreditBalance.UserSubscriptionId)

	const requestID = "credit-request-full-restore-remainder"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 1_003)
	require.NoError(t, err)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 1_002, false))
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 0, true))

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, preConsumed.UserSubscriptionId).Error)
	require.Equal(t, int64(1_003), subscription.TokenLimit)
	require.Zero(t, subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(1_003), state.AvailableCredit)
	require.Equal(t, int64(40_010_000), state.ExactCostMicros)

	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Zero(t, record.AppliedCredit)
	require.Zero(t, record.DeductedAvailableCredit)
	require.Zero(t, record.DeductedExactCostMicros)
	require.Zero(t, record.DeductedEstimatedCostMicros)
	require.Zero(t, record.DeductedUnknownCredit)
	require.Equal(t, "refunded", record.Status)
}

func TestCreditValuationRequestTargetMissingRecordReturnsStableError(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	_, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	err := SettleUserSubscriptionRequestTarget("credit-request-missing", completed.CreditBalance.UserSubscriptionId, 1, true)
	require.ErrorIs(t, err, ErrCreditValuationRequestNotFound)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, completed.CreditBalance.UserSubscriptionId).Error)
	require.Zero(t, subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(1_000), state.AvailableCredit)
	require.Equal(t, int64(40_000_000), state.ExactCostMicros)
	require.Equal(t, int64(1), state.StateVersion)
}

func TestCreditValuationRequestTargetRejectsIncreaseAfterFinalization(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-finalized-increase"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 200, true))

	err = SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 300, true)
	require.ErrorIs(t, err, ErrCreditValuationFinalizedConflict)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, completed.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(200), subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(800), state.AvailableCredit)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)
	require.Equal(t, int64(2), state.StateVersion)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(200), record.AppliedCredit)
	require.Equal(t, int64(1), record.SettlementVersion)
	require.Equal(t, "settled", record.Status)
}

func TestCreditValuationRequestTargetRejectsNegativeTargetAtomically(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-negative-target"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)

	err = SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, -1, true)
	require.ErrorIs(t, err, ErrCreditValuationNegativeInput)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, completed.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(200), subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(800), state.AvailableCredit)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)
	require.Equal(t, int64(2), state.StateVersion)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(200), record.AppliedCredit)
	require.Equal(t, int64(1), record.SettlementVersion)
	require.Zero(t, record.FinalizedAt)
}

func TestCreditValuationRequestTargetStateMissingRollsBackAtomically(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-state-missing"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.NoError(t, db.Where("user_subscription_id = ?", preConsumed.UserSubscriptionId).Delete(&CreditValuationState{}).Error)

	err = SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 300, false)
	require.ErrorIs(t, err, ErrCreditValuationStateMissing)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, completed.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(200), subscription.TokenUsed)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(200), record.AppliedCredit)
	require.Equal(t, int64(200), record.DeductedAvailableCredit)
	require.Equal(t, int64(8_000_000), record.DeductedExactCostMicros)
	require.Equal(t, int64(1), record.SettlementVersion)
	require.Zero(t, record.FinalizedAt)
}

func TestCreditValuationRequestTargetStateMismatchRollsBackAtomically(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-state-mismatch"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.NoError(t, db.Model(&CreditValuationState{}).Where("user_subscription_id = ?", preConsumed.UserSubscriptionId).Update("available_credit", 801).Error)

	err = SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 300, false)
	require.ErrorIs(t, err, ErrCreditValuationStateMismatch)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, completed.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(200), subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(801), state.AvailableCredit)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)
	require.Equal(t, int64(2), state.StateVersion)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(200), record.AppliedCredit)
	require.Equal(t, int64(1), record.SettlementVersion)
}

func TestCreditValuationRequestTargetOverflowRollsBackAtomically(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-overflow"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.NoError(t, db.Model(&CreditValuationState{}).Where("user_subscription_id = ?", preConsumed.UserSubscriptionId).Update("state_version", int64(math.MaxInt64)).Error)

	err = SettleUserSubscriptionRequestTarget(requestID, preConsumed.UserSubscriptionId, 300, false)
	require.ErrorIs(t, err, ErrCreditValuationOverflow)

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, completed.CreditBalance.UserSubscriptionId).Error)
	require.Equal(t, int64(200), subscription.TokenUsed)
	var state CreditValuationState
	require.NoError(t, db.First(&state, subscription.Id).Error)
	require.Equal(t, int64(800), state.AvailableCredit)
	require.Equal(t, int64(32_000_000), state.ExactCostMicros)
	require.Equal(t, int64(math.MaxInt64), state.StateVersion)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
	require.Equal(t, int64(200), record.AppliedCredit)
	require.Equal(t, int64(1), record.SettlementVersion)
}
