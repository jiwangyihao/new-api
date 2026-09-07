package model

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPreConsumeCleanupKeepsRecentLegacyRefunds(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	now := GetDBTimestamp()
	records := []SubscriptionPreConsumeRecord{
		{RequestId: "recent-refund", Status: "refunded", CreatedAt: now - 10, UpdatedAt: now},
		{RequestId: "old-refund", Status: "refunded", CreatedAt: now - 3600, UpdatedAt: now - 3600},
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&records).Error)
	_, err := CleanupSubscriptionPreConsumeRecords(context.Background(), 60, 100, 0)
	require.NoError(t, err)
	var remaining []string
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).Order("id ASC").Pluck("request_id", &remaining).Error)
	assert.Equal(t, []string{"recent-refund"}, remaining)
}

func TestPreConsumeCleanupRequiresExactLegacyCompletionEvidence(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}, &Log{}))
	old := GetDBTimestamp() - 3600
	subscriptionID := 20
	records := []SubscriptionPreConsumeRecord{
		{RequestId: "completed", UserId: 10, UserSubscriptionId: subscriptionID, Status: "consumed", CreatedAt: old, UpdatedAt: old},
		{RequestId: "wrong-user", UserId: 10, UserSubscriptionId: subscriptionID, Status: "consumed", CreatedAt: old, UpdatedAt: old},
		{RequestId: "wrong-subscription", UserId: 10, UserSubscriptionId: subscriptionID, Status: "consumed", CreatedAt: old, UpdatedAt: old},
		{RequestId: "only-error", UserId: 10, UserSubscriptionId: subscriptionID, Status: "consumed", CreatedAt: old, UpdatedAt: old},
		{RequestId: "unfinalized-credit", UserId: 10, UserSubscriptionId: subscriptionID, ValuationSubscriptionId: 21, AppliedCredit: 30, Status: "consumed", CreatedAt: old, UpdatedAt: old},
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&records).Error)
	otherSubscriptionID := 99
	logs := []Log{
		{RequestId: "completed", UserId: 10, SubscriptionID: &subscriptionID, Type: LogTypeConsume, CreatedAt: old},
		{RequestId: "wrong-user", UserId: 99, SubscriptionID: &subscriptionID, Type: LogTypeConsume, CreatedAt: old},
		{RequestId: "wrong-subscription", UserId: 10, SubscriptionID: &otherSubscriptionID, Type: LogTypeConsume, CreatedAt: old},
		{RequestId: "only-error", UserId: 10, SubscriptionID: &subscriptionID, Type: LogTypeError, CreatedAt: old},
		{RequestId: "unfinalized-credit", UserId: 10, SubscriptionID: &subscriptionID, Type: LogTypeConsume, CreatedAt: old},
	}
	require.NoError(t, db.Create(&logs).Error)
	_, err := CleanupSubscriptionPreConsumeRecords(context.Background(), 60, 100, 0)
	require.NoError(t, err)
	var remaining []string
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).Order("id ASC").Pluck("request_id", &remaining).Error)
	assert.Equal(t, []string{"wrong-user", "wrong-subscription", "only-error", "unfinalized-credit"}, remaining)
}

func TestPreConsumeCleanupCursorAdvancesPastProtectedRecords(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	old := GetDBTimestamp() - 3600
	records := []SubscriptionPreConsumeRecord{
		{RequestId: "protected-first", Status: "consumed", ValuationSubscriptionId: 1},
		{RequestId: "protected-second", Status: "consumed", ValuationSubscriptionId: 1},
		{RequestId: "expired-after-protected", Status: "settled", FinalizedAt: old},
	}
	require.NoError(t, db.Create(&records).Error)
	first, err := CleanupSubscriptionPreConsumeRecords(context.Background(), 60, 2, 0)
	require.NoError(t, err)
	assert.Zero(t, first.Deleted)
	assert.False(t, first.Done)
	second, err := CleanupSubscriptionPreConsumeRecords(context.Background(), 60, 2, first.LastID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), second.Deleted)
	assert.True(t, second.Done)
	var remaining []string
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).Order("id ASC").Pluck("request_id", &remaining).Error)
	assert.Equal(t, []string{"protected-first", "protected-second"}, remaining)
}

func TestPreConsumeCleanupProtectsQueuedTaskIdentity(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	requestID := "queued-task-retention"
	record := SubscriptionPreConsumeRecord{RequestId: requestID, Status: "settled", FinalizedAt: GetDBTimestamp() - 3600}
	require.NoError(t, db.Create(&record).Error)
	task := Task{TaskID: "queued-retention", Status: TaskStatusQueued, SubscriptionRequestId: &requestID}
	require.NoError(t, db.Create(&task).Error)
	batch, err := CleanupSubscriptionPreConsumeRecords(context.Background(), 60, 100, 0)
	require.NoError(t, err)
	assert.Zero(t, batch.Deleted)
	var stored SubscriptionPreConsumeRecord
	require.NoError(t, db.First(&stored, record.Id).Error)
	assert.Equal(t, requestID, stored.RequestId)
}

func TestFinalizePreConsumePreservesFundingAndTerminalTime(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	subscription := UserSubscription{Id: 20, UserId: 10, TokenUsed: 75, AmountUsed: 80, EntitlementType: SubscriptionEntitlementTimed}
	require.NoError(t, db.Create(&subscription).Error)
	record := SubscriptionPreConsumeRecord{RequestId: "finalize-without-recharging", UserId: 10, UserSubscriptionId: subscription.Id, PreConsumed: 50, Status: "consumed"}
	require.NoError(t, db.Create(&record).Error)
	require.NoError(t, FinalizeSubscriptionPreConsume(record.RequestId, subscription.Id, false))
	require.NoError(t, db.First(&record, record.Id).Error)
	assert.Equal(t, "settled", record.Status)
	assert.Positive(t, record.FinalizedAt)
	oldFinalized := record.FinalizedAt - 30
	require.NoError(t, db.Model(&record).UpdateColumn("finalized_at", oldFinalized).Error)
	require.NoError(t, FinalizeSubscriptionPreConsume(record.RequestId, subscription.Id, false))
	require.NoError(t, db.First(&record, record.Id).Error)
	assert.Equal(t, oldFinalized, record.FinalizedAt)
	assert.ErrorIs(t, FinalizeSubscriptionPreConsume(record.RequestId, subscription.Id+1, false), ErrCreditValuationMappingConflict)
	var stored UserSubscription
	require.NoError(t, db.First(&stored, subscription.Id).Error)
	assert.Equal(t, int64(75), stored.TokenUsed)
	assert.Equal(t, int64(80), stored.AmountUsed)
}

func TestUsageLogRetentionPreservesUnfinishedWorkAndSummaries(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}, &Log{}, &LogAggregationEvent{}, &LogUsageHourly{}))
	cutoff := GetDBTimestamp() - 3600
	subscriptionID := 20
	tokens := int64(10)
	logs := []Log{
		{Id: 1, UserId: 10, CreatedAt: cutoff - 1, Type: LogTypeConsume},
		{Id: 2, UserId: 10, CreatedAt: cutoff, Type: LogTypeConsume},
		{Id: 3, UserId: 10, CreatedAt: cutoff - 1, Type: LogTypeConsume},
		{Id: 4, UserId: 10, CreatedAt: cutoff - 1, Type: LogTypeConsume, SubscriptionID: &subscriptionID, SubscriptionTokensConsumed: &tokens},
		{Id: 5, UserId: 10, CreatedAt: cutoff - 1, Type: LogTypeConsume, RequestId: "retain-completion-evidence", SubscriptionID: &subscriptionID},
		{Id: 6, UserId: 10, CreatedAt: cutoff - 1, Type: LogTypeTopup},
		{Id: 7, UserId: 10, CreatedAt: cutoff - 1, Type: LogTypeConsume},
		{Id: 8, UserId: 10, CreatedAt: cutoff - 1, Type: LogTypeError},
	}
	require.NoError(t, db.Create(&logs).Error)
	events := []LogAggregationEvent{
		{LogID: 1, AggregateName: logAggregationNameLogUsageHourly, Status: logAggregationEventStatusApplied},
		{LogID: 3, AggregateName: logAggregationNameLogUsageHourly, Status: logAggregationEventStatusPending},
		{LogID: 4, AggregateName: logAggregationNameLogUsageHourly, Status: logAggregationEventStatusApplied},
		{LogID: 5, AggregateName: logAggregationNameLogUsageHourly, Status: logAggregationEventStatusApplied},
		{LogID: 8, AggregateName: logAggregationNameLogUsageHourly, Status: logAggregationEventStatusApplied},
	}
	require.NoError(t, db.Create(&events).Error)
	record := SubscriptionPreConsumeRecord{RequestId: "retain-completion-evidence", UserId: 10, UserSubscriptionId: subscriptionID, Status: "consumed"}
	require.NoError(t, db.Create(&record).Error)
	summary := LogUsageHourly{UserID: 10, BucketStart: cutoff - 1, ModelName: "retained-model", RequestCount: 50, QuotaSum: 700}
	require.NoError(t, db.Create(&summary).Error)
	batch, err := DeleteExpiredUsageLogs(context.Background(), cutoff, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), batch.Deleted)
	var remaining []int
	require.NoError(t, db.Model(&Log{}).Order("id ASC").Pluck("id", &remaining).Error)
	assert.Equal(t, []int{2, 3, 4, 5, 6, 7}, remaining)
	var eventLogIDs []int
	require.NoError(t, db.Model(&LogAggregationEvent{}).Order("log_id ASC").Pluck("log_id", &eventLogIDs).Error)
	assert.Equal(t, []int{3, 4, 5}, eventLogIDs)
	var storedSummary LogUsageHourly
	require.NoError(t, db.First(&storedSummary).Error)
	assert.Equal(t, summary, storedSummary)
}

func TestUsageLogRetentionRollsBackMarkersOnDeleteFailure(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Log{}, &LogAggregationEvent{}))
	log := Log{CreatedAt: GetDBTimestamp() - 3600, Type: LogTypeConsume}
	require.NoError(t, db.Create(&log).Error)
	event := LogAggregationEvent{LogID: log.Id, AggregateName: logAggregationNameLogUsageHourly, Status: logAggregationEventStatusApplied}
	require.NoError(t, db.Create(&event).Error)
	injected := errors.New("retention log deletion rejected")
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("retention:reject-log-delete", func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			tx.AddError(injected)
		}
	}))
	batch, err := DeleteExpiredUsageLogs(context.Background(), GetDBTimestamp()-60, 100, 0)
	require.ErrorIs(t, err, injected)
	assert.Zero(t, batch.Deleted)
	require.NoError(t, db.First(&log, log.Id).Error)
	require.NoError(t, db.First(&event, event.Id).Error)
	assert.Equal(t, logAggregationEventStatusApplied, event.Status)
}

func TestUsageLogRetentionProtectsMainDatabaseEvidenceWhenLogsAreSeparate(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	logDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := logDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
	LOG_DB = logDB
	require.NoError(t, logDB.AutoMigrate(&Log{}, &LogAggregationEvent{}))
	subscriptionID := 20
	log := Log{RequestId: "split-log-evidence", UserId: 10, SubscriptionID: &subscriptionID, Type: LogTypeConsume, CreatedAt: GetDBTimestamp() - 3600}
	require.NoError(t, logDB.Create(&log).Error)
	event := LogAggregationEvent{LogID: log.Id, AggregateName: logAggregationNameLogUsageHourly, Status: logAggregationEventStatusApplied}
	require.NoError(t, logDB.Create(&event).Error)
	record := SubscriptionPreConsumeRecord{RequestId: log.RequestId, UserId: log.UserId, UserSubscriptionId: subscriptionID, Status: "consumed"}
	require.NoError(t, db.Create(&record).Error)
	batch, err := DeleteExpiredUsageLogs(context.Background(), GetDBTimestamp()-60, 100, 0)
	require.NoError(t, err)
	assert.Zero(t, batch.Deleted)
	require.NoError(t, db.Delete(&record).Error)
	batch, err = DeleteExpiredUsageLogs(context.Background(), GetDBTimestamp()-60, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), batch.Deleted)
}
