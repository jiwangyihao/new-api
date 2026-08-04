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

	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, 300, false))

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
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, 100, true))

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
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, 1_100, false))
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, 950, true))

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
