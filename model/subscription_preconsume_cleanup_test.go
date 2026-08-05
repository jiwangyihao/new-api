package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestCleanupSubscriptionPreConsumeRecordsRollsBackBatchOnDeleteFailure(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	subscriptionID := completed.CreditBalance.UserSubscriptionId

	requestIDs := []string{"cleanup-rollback-first", "cleanup-rollback-second"}
	for _, requestID := range requestIDs {
		preConsumed, err := PreConsumeUserSubscriptionByUnits(requestID, user.Id, "gpt-4o", 0, 0, 10)
		require.NoError(t, err)
		require.Equal(t, subscriptionID, preConsumed.UserSubscriptionId)
		require.NoError(t, SettleUserSubscriptionRequestTarget(requestID, subscriptionID, 10, true))
	}
	expiredAt := GetDBTimestamp() - 3_600
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id IN ?", requestIDs).
		UpdateColumn("finalized_at", expiredAt).Error)

	injectedErr := errors.New("injected cleanup delete failure")
	const callbackName = "issue23:inject_cleanup_delete_failure"
	require.NoError(t, db.Callback().Delete().After("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "SubscriptionPreConsumeRecord" {
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() { db.Callback().Delete().Remove(callbackName) })

	deleted, err := cleanupSubscriptionPreConsumeRecordsBatch(60, len(requestIDs))
	require.ErrorIs(t, err, injectedErr)
	require.Zero(t, deleted)

	var remaining []string
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Order("id ASC").Pluck("request_id", &remaining).Error)
	require.Equal(t, requestIDs, remaining)
}

func TestCleanupSubscriptionPreConsumeRecordsSerializesWithTerminalTaskReplays(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	oldRedis := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	initCol()
	resetDBTimestampCacheForTest()
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "cleanup-concurrency.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &Task{}))
	require.NoError(t, migrateCreditValuationSchema(db))
	require.NoError(t, db.Create(&CreditValuationMigration{Version: 1, Status: CreditValuationMigrationReady, ValuationCurrency: "CNY"}).Error)
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		common.RedisEnabled = oldRedis
		resetDBTimestampCacheForTest()
		initCol()
		_ = sqlDB.Close()
	})

	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	subscriptionID := completed.CreditBalance.UserSubscriptionId
	requests := []struct {
		requestID string
		target    int64
		status    TaskStatus
	}{
		{requestID: "cleanup-concurrent-final-replay", target: 10, status: TaskStatusSubmitted},
		{requestID: "cleanup-concurrent-refund-replay", target: 0, status: TaskStatusInProgress},
	}
	for _, request := range requests {
		preConsumed, preConsumeErr := PreConsumeUserSubscriptionByUnits(request.requestID, user.Id, "gpt-4o", 0, 0, 10)
		require.NoError(t, preConsumeErr)
		require.Equal(t, subscriptionID, preConsumed.UserSubscriptionId)
		require.NoError(t, SettleUserSubscriptionRequestTarget(request.requestID, subscriptionID, request.target, true))
		task := &Task{
			TaskID: "task-" + request.requestID,
			UserId: user.Id,
			Status: request.status,
			PrivateData: TaskPrivateData{
				SubscriptionRequestId: request.requestID,
			},
		}
		require.NoError(t, task.Insert())
	}
	expiredAt := GetDBTimestamp() - 3_600
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id IN ?", []string{requests[0].requestID, requests[1].requestID}).
		UpdateColumn("finalized_at", expiredAt).Error)

	ready := make(chan struct{}, len(requests)+1)
	start := make(chan struct{})
	errs := make(chan error, len(requests)+1)
	var deleted int64
	var wg sync.WaitGroup
	for _, request := range requests {
		request := request
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			errs <- SettleUserSubscriptionRequestTarget(request.requestID, subscriptionID, request.target, true)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ready <- struct{}{}
		<-start
		var cleanupErr error
		deleted, cleanupErr = CleanupSubscriptionPreConsumeRecords(60)
		errs <- cleanupErr
	}()
	for range cap(ready) {
		<-ready
	}
	close(start)
	wg.Wait()
	close(errs)
	for concurrentErr := range errs {
		require.NoError(t, concurrentErr)
	}
	require.Zero(t, deleted)

	var remaining []string
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Order("id ASC").Pluck("request_id", &remaining).Error)
	require.ElementsMatch(t, []string{requests[0].requestID, requests[1].requestID}, remaining)
}

func TestPreviewSubscriptionPreConsumeCleanupIsStableAndReadOnly(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	subscriptionID := completed.CreditBalance.UserSubscriptionId

	requests := []struct {
		requestID string
		target    int64
		status    TaskStatus
	}{
		{requestID: "cleanup-preview-settled", target: 10},
		{requestID: "cleanup-preview-refunded", target: 0},
		{requestID: "cleanup-preview-protected", target: 10, status: TaskStatusSubmitted},
	}
	for _, request := range requests {
		preConsumed, err := PreConsumeUserSubscriptionByUnits(request.requestID, user.Id, "gpt-4o", 0, 0, 10)
		require.NoError(t, err)
		require.Equal(t, subscriptionID, preConsumed.UserSubscriptionId)
		require.NoError(t, SettleUserSubscriptionRequestTarget(request.requestID, subscriptionID, request.target, true))
		if request.status != "" {
			task := &Task{
				TaskID: "task-" + request.requestID,
				UserId: user.Id,
				Status: request.status,
				PrivateData: TaskPrivateData{
					SubscriptionRequestId: request.requestID,
				},
			}
			require.NoError(t, task.Insert())
		}
	}
	expiredAt := GetDBTimestamp() - 3_600
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id IN ?", []string{requests[0].requestID, requests[1].requestID, requests[2].requestID}).
		UpdateColumn("finalized_at", expiredAt).Error)

	before := sqliteTotalChanges(t, db)
	first, err := PreviewSubscriptionPreConsumeCleanup(60, 2)
	require.NoError(t, err)
	second, err := PreviewSubscriptionPreConsumeCleanup(60, 2)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, before, sqliteTotalChanges(t, db))
	require.Equal(t, GetDBTimestamp()-60, first.Cutoff)
	require.Equal(t, 2, first.BatchSize)
	require.Equal(t, int64(2), first.CandidateCount)
	require.Equal(t, int64(1), first.ProtectedCount)
	require.Equal(t, map[string]int64{"refunded": 1, "settled": 1}, first.TerminalCounts)
	require.Equal(t, map[string]int64{"active_task_reference": 1}, first.ProtectionReasons)

	var count int64
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).Count(&count).Error)
	require.Equal(t, int64(len(requests)), count)
}

func sqliteTotalChanges(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var total int64
	require.NoError(t, db.Raw("SELECT total_changes()").Scan(&total).Error)
	return total
}
