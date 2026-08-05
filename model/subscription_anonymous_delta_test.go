package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type creditAnonymousDeltaSnapshot struct {
	Subscription UserSubscription
	Valuation    CreditValuationState
	RequestCount int64
	LedgerCount  int64
}

func captureCreditAnonymousDeltaSnapshot(t *testing.T, db *gorm.DB, subscriptionID int) creditAnonymousDeltaSnapshot {
	t.Helper()
	var snapshot creditAnonymousDeltaSnapshot
	require.NoError(t, db.First(&snapshot.Subscription, subscriptionID).Error)
	require.NoError(t, db.Where("user_subscription_id = ?", subscriptionID).First(&snapshot.Valuation).Error)
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).Count(&snapshot.RequestCount).Error)
	require.NoError(t, db.Model(&CreditBalanceLedger{}).Count(&snapshot.LedgerCount).Error)
	return snapshot
}

func TestCreditValuationAnonymousSubscriptionDeltasAreForbidden(t *testing.T) {
	tests := []struct {
		name  string
		apply func(int) error
	}{
		{name: "token delta", apply: func(subscriptionID int) error {
			return PostConsumeUserSubscriptionTokenDelta(subscriptionID, 10)
		}},
		{name: "amount delta", apply: func(subscriptionID int) error {
			return PostConsumeUserSubscriptionAmountDelta(subscriptionID, 10)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupCreditValuationTracerTestDB(t)
			_, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
			completed := completeCreditValuationOrder(t, db, &order)
			subscriptionID := completed.CreditBalance.UserSubscriptionId
			before := captureCreditAnonymousDeltaSnapshot(t, db, subscriptionID)

			err := test.apply(subscriptionID)
			after := captureCreditAnonymousDeltaSnapshot(t, db, subscriptionID)

			require.ErrorIs(t, err, ErrCreditValuationAnonymousDeltaForbidden)
			require.Equal(t, before, after)
		})
	}
}

func TestTimedSubscriptionAnonymousDeltasRemainCompatible(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	const userID, planID, subscriptionID = 92_101, 92_102, 92_103
	code := "timed-anonymous-delta"
	require.NoError(t, db.Create(&User{Id: userID, Username: "timed-anonymous-delta", Status: common.UserStatusEnabled, AffCode: "timed-anonymous-delta"}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Id:                planID,
		Title:             "Timed anonymous delta",
		EntitlementType:   SubscriptionEntitlementTimed,
		Enabled:           true,
		TotalAmount:       100,
		MonthlyTokenLimit: 100,
		ConcurrencyLimit:  1,
		BusinessCode:      &code,
	}).Error)
	require.NoError(t, db.Create(&UserSubscription{
		Id:              subscriptionID,
		UserId:          userID,
		PlanId:          planID,
		EntitlementType: SubscriptionEntitlementTimed,
		AmountTotal:     100,
		AmountUsed:      10,
		TokenLimit:      100,
		TokenUsed:       10,
		Status:          SubscriptionStatusActive,
		EndTime:         GetDBTimestamp() + 3_600,
	}).Error)

	require.NoError(t, PostConsumeUserSubscriptionTokenDelta(subscriptionID, 5))
	require.NoError(t, PostConsumeUserSubscriptionAmountDelta(subscriptionID, 7))

	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, subscriptionID).Error)
	require.Equal(t, int64(15), subscription.TokenUsed)
	require.Equal(t, int64(17), subscription.AmountUsed)
}
