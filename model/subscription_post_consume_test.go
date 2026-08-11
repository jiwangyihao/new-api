package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type userSubscriptionPostConsumeSnapshot struct {
	Subscriptions []UserSubscription
	Valuations    []CreditValuationState
	Requests      []SubscriptionPreConsumeRecord
	Ledgers       []CreditBalanceLedger
}

func captureUserSubscriptionPostConsumeSnapshot(t *testing.T, db *gorm.DB) userSubscriptionPostConsumeSnapshot {
	t.Helper()
	var snapshot userSubscriptionPostConsumeSnapshot
	require.NoError(t, db.Order("id").Find(&snapshot.Subscriptions).Error)
	require.NoError(t, db.Order("user_subscription_id").Find(&snapshot.Valuations).Error)
	require.NoError(t, db.Order("id").Find(&snapshot.Requests).Error)
	require.NoError(t, db.Order("id").Find(&snapshot.Ledgers).Error)
	return snapshot
}

func TestPostConsumeUserSubscriptionRequestDeltaRoutesCreditAndTimed(t *testing.T) {
	t.Run("Credit target and replay stay stable", func(t *testing.T) {
		db := setupCreditValuationTracerTestDB(t)
		user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
		completed := completeCreditValuationOrder(t, db, &order)
		const requestID = "post-consume-request-delta-credit"
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 100)
		require.NoError(t, err)
		require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)

		result, err := PostConsumeUserSubscriptionRequestDelta(requestID, preConsumed.UserSubscriptionId, 50, true)
		require.NoError(t, err)
		require.Equal(t, UserSubscriptionPostConsumeResult{PostDelta: 50, ReplacePostDelta: true}, result)

		var recordBeforeReplay SubscriptionPreConsumeRecord
		require.NoError(t, db.Where("request_id = ?", requestID).First(&recordBeforeReplay).Error)
		require.Equal(t, int64(100), recordBeforeReplay.PreConsumed)
		require.Equal(t, int64(150), recordBeforeReplay.AppliedCredit)
		var subscriptionBeforeReplay UserSubscription
		require.NoError(t, db.First(&subscriptionBeforeReplay, preConsumed.UserSubscriptionId).Error)
		require.Equal(t, int64(150), subscriptionBeforeReplay.TokenUsed)
		var valuationBeforeReplay CreditValuationState
		require.NoError(t, db.Where("user_subscription_id = ?", preConsumed.UserSubscriptionId).First(&valuationBeforeReplay).Error)
		require.Equal(t, int64(850), valuationBeforeReplay.AvailableCredit)
		require.Equal(t, int64(34_000_000), valuationBeforeReplay.ExactCostMicros)

		replayed, err := PostConsumeUserSubscriptionRequestDelta(requestID, preConsumed.UserSubscriptionId, 50, true)
		require.NoError(t, err)
		require.Equal(t, result, replayed)
		var recordAfterReplay SubscriptionPreConsumeRecord
		require.NoError(t, db.Where("request_id = ?", requestID).First(&recordAfterReplay).Error)
		require.Equal(t, recordBeforeReplay, recordAfterReplay)
		var subscriptionAfterReplay UserSubscription
		require.NoError(t, db.First(&subscriptionAfterReplay, preConsumed.UserSubscriptionId).Error)
		require.Equal(t, subscriptionBeforeReplay, subscriptionAfterReplay)
		var valuationAfterReplay CreditValuationState
		require.NoError(t, db.Where("user_subscription_id = ?", preConsumed.UserSubscriptionId).First(&valuationAfterReplay).Error)
		require.Equal(t, valuationBeforeReplay, valuationAfterReplay)
	})

	t.Run("incremental Credit calls accumulate from persisted applied target", func(t *testing.T) {
		db := setupCreditValuationTracerTestDB(t)
		user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
		completed := completeCreditValuationOrder(t, db, &order)
		const requestID = "post-consume-request-incremental-credit"
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 100)
		require.NoError(t, err)
		require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)

		first, err := ApplyUserSubscriptionRequestDelta(requestID, preConsumed.UserSubscriptionId, 25, false)
		require.NoError(t, err)
		require.Equal(t, UserSubscriptionRequestDeltaResult{PostDelta: 25, AppliedCredit: 125, Mapped: true}, first)
		second, err := ApplyUserSubscriptionRequestDelta(requestID, preConsumed.UserSubscriptionId, 25, false)
		require.NoError(t, err)
		require.Equal(t, UserSubscriptionRequestDeltaResult{PostDelta: 50, AppliedCredit: 150, Mapped: true}, second)

		var record SubscriptionPreConsumeRecord
		require.NoError(t, db.Where("request_id = ?", requestID).First(&record).Error)
		require.Equal(t, int64(150), record.AppliedCredit)
		var subscription UserSubscription
		require.NoError(t, db.First(&subscription, preConsumed.UserSubscriptionId).Error)
		require.Equal(t, int64(150), subscription.TokenUsed)
		var state CreditValuationState
		require.NoError(t, db.Where("user_subscription_id = ?", preConsumed.UserSubscriptionId).First(&state).Error)
		require.Equal(t, int64(850), state.AvailableCredit)
		require.Equal(t, int64(34_000_000), state.ExactCostMicros)
	})

	t.Run("missing Credit request fails without writes", func(t *testing.T) {
		db := setupCreditValuationTracerTestDB(t)
		_, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
		completed := completeCreditValuationOrder(t, db, &order)
		before := captureUserSubscriptionPostConsumeSnapshot(t, db)

		result, err := PostConsumeUserSubscriptionRequestDelta("post-consume-request-delta-missing", completed.CreditBalance.UserSubscriptionId, 50, true)
		require.ErrorIs(t, err, ErrCreditValuationRequestNotFound)
		require.Equal(t, UserSubscriptionPostConsumeResult{}, result)
		require.Equal(t, before, captureUserSubscriptionPostConsumeSnapshot(t, db))
	})

	t.Run("missing entitlement hides database errors", func(t *testing.T) {
		db := setupCreditValuationTracerTestDB(t)
		before := captureUserSubscriptionPostConsumeSnapshot(t, db)

		result, err := PostConsumeUserSubscriptionRequestDelta("post-consume-missing-entitlement", 99_999, 50, true)
		require.Equal(t, ErrDatabase, err)
		require.Equal(t, UserSubscriptionPostConsumeResult{}, result)
		require.Equal(t, before, captureUserSubscriptionPostConsumeSnapshot(t, db))
	})

	t.Run("Credit request mapping conflict fails without writes", func(t *testing.T) {
		db := setupCreditValuationTracerTestDB(t)
		user, _, creditPlan, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
		completed := completeCreditValuationOrder(t, db, &order)
		const requestID = "post-consume-request-delta-mapping"
		_, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 100)
		require.NoError(t, err)

		const otherUserID, otherSubscriptionID = 92_001, 92_002
		require.NoError(t, db.Create(&User{Id: otherUserID, Username: "post-consume-mapping-other", Status: common.UserStatusEnabled, AffCode: "post-consume-mapping-other"}).Error)
		require.NoError(t, db.Create(&UserSubscription{
			Id:              otherSubscriptionID,
			UserId:          otherUserID,
			PlanId:          creditPlan.Id,
			EntitlementType: SubscriptionEntitlementCreditBalance,
			TokenLimit:      1_000,
			Status:          SubscriptionStatusActive,
		}).Error)
		require.NotEqual(t, completed.CreditBalance.UserSubscriptionId, otherSubscriptionID)
		before := captureUserSubscriptionPostConsumeSnapshot(t, db)

		result, err := PostConsumeUserSubscriptionRequestDelta(requestID, otherSubscriptionID, 50, true)
		require.ErrorIs(t, err, ErrCreditValuationMappingConflict)
		require.Equal(t, UserSubscriptionPostConsumeResult{}, result)
		require.Equal(t, before, captureUserSubscriptionPostConsumeSnapshot(t, db))
	})

	t.Run("negative Credit target fails without writes", func(t *testing.T) {
		db := setupCreditValuationTracerTestDB(t)
		user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
		completed := completeCreditValuationOrder(t, db, &order)
		const requestID = "post-consume-request-delta-negative"
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 100)
		require.NoError(t, err)
		require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)
		before := captureUserSubscriptionPostConsumeSnapshot(t, db)

		result, err := PostConsumeUserSubscriptionRequestDelta(requestID, preConsumed.UserSubscriptionId, -101, true)
		require.ErrorIs(t, err, ErrCreditValuationNegativeInput)
		require.Equal(t, UserSubscriptionPostConsumeResult{}, result)
		require.Equal(t, before, captureUserSubscriptionPostConsumeSnapshot(t, db))
	})

	t.Run("Credit target overflow fails without writes", func(t *testing.T) {
		db := setupCreditValuationTracerTestDB(t)
		user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
		completed := completeCreditValuationOrder(t, db, &order)
		const requestID = "post-consume-request-delta-overflow"
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 100)
		require.NoError(t, err)
		require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)
		before := captureUserSubscriptionPostConsumeSnapshot(t, db)

		result, err := PostConsumeUserSubscriptionRequestDelta(requestID, preConsumed.UserSubscriptionId, int64(^uint64(0)>>1), true)
		require.ErrorIs(t, err, ErrCreditValuationOverflow)
		require.Equal(t, UserSubscriptionPostConsumeResult{}, result)
		require.Equal(t, before, captureUserSubscriptionPostConsumeSnapshot(t, db))
	})

	t.Run("Credit target sentinel propagates without writes", func(t *testing.T) {
		db := setupCreditValuationTracerTestDB(t)
		user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
		completed := completeCreditValuationOrder(t, db, &order)
		const requestID = "post-consume-request-delta-state-missing"
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 100)
		require.NoError(t, err)
		require.Equal(t, completed.CreditBalance.UserSubscriptionId, preConsumed.UserSubscriptionId)
		require.NoError(t, db.Where("user_subscription_id = ?", preConsumed.UserSubscriptionId).Delete(&CreditValuationState{}).Error)
		before := captureUserSubscriptionPostConsumeSnapshot(t, db)

		result, err := PostConsumeUserSubscriptionRequestDelta(requestID, preConsumed.UserSubscriptionId, 50, true)
		require.ErrorIs(t, err, ErrCreditValuationStateMissing)
		require.Equal(t, UserSubscriptionPostConsumeResult{}, result)
		require.Equal(t, before, captureUserSubscriptionPostConsumeSnapshot(t, db))
	})

	t.Run("timed distributor keeps anonymous token delta behavior", func(t *testing.T) {
		db := setupCreditValuationTracerTestDB(t)
		const userID, planID, subscriptionID = 93_001, 93_002, 93_003
		code := "post-consume-timed-distributor"
		require.NoError(t, db.Create(&User{Id: userID, Username: "post-consume-timed", Status: common.UserStatusEnabled, AffCode: "post-consume-timed"}).Error)
		require.NoError(t, db.Create(&SubscriptionPlan{
			Id:                planID,
			Title:             "Post-consume timed distributor",
			EntitlementType:   SubscriptionEntitlementTimed,
			Enabled:           true,
			MonthlyTokenLimit: 100,
			ConcurrencyLimit:  1,
			BusinessCode:      &code,
		}).Error)
		require.NoError(t, db.Create(&UserSubscription{
			Id:              subscriptionID,
			UserId:          userID,
			PlanId:          planID,
			EntitlementType: SubscriptionEntitlementTimed,
			TokenLimit:      100,
			TokenUsed:       10,
			Status:          SubscriptionStatusActive,
			EndTime:         GetDBTimestamp() + 3_600,
		}).Error)

		result, err := PostConsumeUserSubscriptionRequestDelta("ignored-for-timed", subscriptionID, 5, true)
		require.NoError(t, err)
		require.Equal(t, UserSubscriptionPostConsumeResult{PostDelta: 5}, result)
		var subscription UserSubscription
		require.NoError(t, db.First(&subscription, subscriptionID).Error)
		require.Equal(t, int64(15), subscription.TokenUsed)
		require.Zero(t, subscription.AmountUsed)
	})
}
