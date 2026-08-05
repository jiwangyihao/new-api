package model

import (
	"fmt"
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
