package model

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanupSubscriptionPreConsumeRecordsDeletesOnlyExpiredTerminalRecords(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
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
	require.NoError(t, db.AutoMigrate(&Task{}))
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

func TestCleanupSubscriptionPreConsumeRecordsProtectsActiveTaskReferences(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	subscriptionID := completed.CreditBalance.UserSubscriptionId

	const (
		settledRequestID    = "cleanup-task-settled"
		refundedRequestID   = "cleanup-task-refunded"
		unreferencedRequest = "cleanup-task-unreferenced"
	)
	for requestID, target := range map[string]int64{
		settledRequestID:    10,
		refundedRequestID:   0,
		unreferencedRequest: 10,
	} {
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 10)
		require.NoError(t, err)
		require.Equal(t, subscriptionID, preConsumed.UserSubscriptionId)
		require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, subscriptionID, target, true))
	}

	for requestID, status := range map[string]TaskStatus{
		settledRequestID:  TaskStatusSubmitted,
		refundedRequestID: TaskStatusInProgress,
	} {
		task := &Task{
			TaskID: "task-ref-" + requestID,
			UserId: user.Id,
			Status: status,
			PrivateData: TaskPrivateData{
				SubscriptionRequestId: requestID,
			},
		}
		require.NoError(t, task.Insert())
	}

	expiredAt := GetDBTimestamp() - 3_600
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id IN ?", []string{settledRequestID, refundedRequestID, unreferencedRequest}).
		UpdateColumn("finalized_at", expiredAt).Error)

	deleted, err := CleanupSubscriptionPreConsumeRecords(60)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	var remaining []string
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Order("id ASC").Pluck("request_id", &remaining).Error)
	require.ElementsMatch(t, []string{settledRequestID, refundedRequestID}, remaining)
}

func TestCleanupSubscriptionPreConsumeRecordsFailsClosedOnAmbiguousActiveTaskReference(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	subscriptionID := completed.CreditBalance.UserSubscriptionId

	const requestID = "cleanup-task-ambiguous-null"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 10)
	require.NoError(t, err)
	require.Equal(t, subscriptionID, preConsumed.UserSubscriptionId)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, subscriptionID, 10, true))
	expiredAt := GetDBTimestamp() - 3_600
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", requestID).
		UpdateColumn("finalized_at", expiredAt).Error)
	ambiguous := &Task{
		TaskID: "task-ref-ambiguous-null",
		UserId: user.Id,
		Status: TaskStatusInProgress,
		PrivateData: TaskPrivateData{
			SubscriptionId:        subscriptionID,
			SubscriptionRequestId: requestID,
		},
	}
	require.NoError(t, db.Create(ambiguous).Error)
	require.False(t, taskProjectionValue(t, db, ambiguous.ID).Valid)

	deleted, err := CleanupSubscriptionPreConsumeRecords(60)
	require.ErrorIs(t, err, ErrSubscriptionPreConsumeCleanupAmbiguousTaskReference)
	require.Zero(t, deleted)
	var count int64
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", requestID).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestCleanupSubscriptionPreConsumeRecordsAllowsProvenTimedTaskWithoutRequestIdentity(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	creditSubscriptionID := completed.CreditBalance.UserSubscriptionId

	const requestID = "cleanup-task-proven-timed-null"
	preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 10)
	require.NoError(t, err)
	require.Equal(t, creditSubscriptionID, preConsumed.UserSubscriptionId)
	require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, creditSubscriptionID, 10, true))
	expiredAt := GetDBTimestamp() - 3_600
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", requestID).
		UpdateColumn("finalized_at", expiredAt).Error)

	timedSubscription := &UserSubscription{
		UserId:          user.Id,
		PlanId:          91_004,
		EntitlementType: SubscriptionEntitlementTimed,
		Status:          "active",
	}
	require.NoError(t, db.Create(timedSubscription).Error)
	timedTask := &Task{
		TaskID: "task-ref-proven-timed-null",
		UserId: user.Id,
		Status: TaskStatusSubmitted,
		PrivateData: TaskPrivateData{
			SubscriptionId: timedSubscription.Id,
		},
	}
	require.NoError(t, timedTask.Insert())

	deleted, err := CleanupSubscriptionPreConsumeRecords(60)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
}

func TestCleanupSubscriptionPreConsumeRecordsUsesStableBoundedBatches(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	subscriptionID := completed.CreditBalance.UserSubscriptionId

	const expectedBatchSize = 2
	requestIDs := make([]string, 0, expectedBatchSize+1)
	for i := 0; i <= expectedBatchSize; i++ {
		requestID := fmt.Sprintf("cleanup-batch-%03d", i)
		requestIDs = append(requestIDs, requestID)
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 1)
		require.NoError(t, err)
		require.Equal(t, subscriptionID, preConsumed.UserSubscriptionId)
		require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, subscriptionID, 1, true))
	}
	expiredAt := GetDBTimestamp() - 3_600
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id IN ?", requestIDs).
		UpdateColumn("finalized_at", expiredAt).Error)

	deleted, err := cleanupSubscriptionPreConsumeRecordsBatch(60, expectedBatchSize)
	require.NoError(t, err)
	require.Equal(t, int64(expectedBatchSize), deleted)
	var remaining []string
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Order("id ASC").Pluck("request_id", &remaining).Error)
	require.Equal(t, requestIDs[expectedBatchSize:], remaining)

	deleted, err = cleanupSubscriptionPreConsumeRecordsBatch(60, expectedBatchSize)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	deleted, err = cleanupSubscriptionPreConsumeRecordsBatch(60, expectedBatchSize)
	require.NoError(t, err)
	require.Zero(t, deleted)
}
