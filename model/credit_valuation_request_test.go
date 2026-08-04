package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreditValuationRequestTargetIncreaseUsesCurrentPool(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)

	const requestID = "credit-request-target-increase"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 200)
	require.NoError(t, err)
	require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)

	require.NoError(t, SettleCreditRequestTarget(requestID, 300, false))

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
