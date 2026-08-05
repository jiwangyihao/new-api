package model

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	subscriptionID := completed.CreditBalance.UserSubscriptionId

	const (
		settledRequestID  = "cleanup-expired-settled"
		refundedRequestID = "cleanup-expired-refunded"
		consumedRequestID = "cleanup-expired-consumed"
		unknownRequestID  = "cleanup-expired-unknown"
	)
	for requestID, credit := range map[string]int64{
		settledRequestID:  100,
		refundedRequestID: 50,
		consumedRequestID: 25,
		unknownRequestID:  10,
	} {
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, credit)
		require.NoError(t, err)
		require.Equal(t, subscriptionID, preConsumed.UserSubscriptionId)
	}
	require.NoError(t, SettleUserSubscriptionRequestTarget(settledRequestID, subscriptionID, 100, true))
	require.NoError(t, SettleUserSubscriptionRequestTarget(refundedRequestID, subscriptionID, 0, true))

	expiredAt := GetDBTimestamp() - 3_600
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id IN ?", []string{settledRequestID, refundedRequestID}).
		UpdateColumns(map[string]any{"finalized_at": expiredAt, "updated_at": expiredAt}).Error)
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", consumedRequestID).
		UpdateColumn("updated_at", expiredAt).Error)
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", unknownRequestID).
		UpdateColumns(map[string]any{"status": "future-state", "updated_at": expiredAt}).Error)

	deleted, err := CleanupSubscriptionPreConsumeRecords(60)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)

	var remaining []string
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Order("id ASC").Pluck("request_id", &remaining).Error)
	sort.Strings(remaining)
	require.Equal(t, []string{consumedRequestID, unknownRequestID}, remaining)
}

func TestCleanupSubscriptionPreConsumeRecordsUsesExclusiveFinalizedAtCutoff(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	subscriptionID := completed.CreditBalance.UserSubscriptionId

	const retentionSeconds int64 = 60
	requestIDs := []string{
		"cleanup-cutoff-before",
		"cleanup-cutoff-equal",
		"cleanup-cutoff-after",
	}
	for _, requestID := range requestIDs {
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 10)
		require.NoError(t, err)
		require.Equal(t, subscriptionID, preConsumed.UserSubscriptionId)
		require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, subscriptionID, 10, true))
	}

	now := GetDBTimestamp()
	cutoff := now - retentionSeconds
	for requestID, finalizedAt := range map[string]int64{
		requestIDs[0]: cutoff - 1,
		requestIDs[1]: cutoff,
		requestIDs[2]: cutoff + 1,
	} {
		require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
			Where("request_id = ?", requestID).
			UpdateColumns(map[string]any{
				"finalized_at": finalizedAt,
				"updated_at":   cutoff - 3_600,
			}).Error)
	}
	dbTimestampCache.Store(now)
	dbTimestampCacheUnixNano.Store(time.Now().UnixNano())

	deleted, err := CleanupSubscriptionPreConsumeRecords(retentionSeconds)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	var remaining []string
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Order("id ASC").Pluck("request_id", &remaining).Error)
	require.Equal(t, requestIDs[1:], remaining)
}
