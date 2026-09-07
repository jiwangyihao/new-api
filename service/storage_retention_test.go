package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupStorageRetentionServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "retention.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = true, false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = oldSQLite, oldMySQL, oldPostgres
		assert.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.UserSubscription{}))
	require.NoError(t, model.MigrateCreditValuationSchema(db))
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Log{}, &model.LogAggregationEvent{}, &model.LogUsageHourly{}))
	return db
}

func TestStorageRetentionDrainsMultipleBatchesAndKeepsProtectedRows(t *testing.T) {
	db := setupStorageRetentionServiceDB(t)
	old := time.Now().Add(-40 * 24 * time.Hour).Unix()
	records := []model.SubscriptionPreConsumeRecord{
		{RequestId: "protected-first", Status: "consumed", ValuationSubscriptionId: 1},
	}
	for i := range 5 {
		records = append(records, model.SubscriptionPreConsumeRecord{RequestId: fmt.Sprintf("expired-%d", i), Status: "settled", FinalizedAt: old})
	}
	require.NoError(t, db.Create(&records).Error)
	logs := []model.Log{{Id: 1, Type: model.LogTypeConsume, CreatedAt: old}}
	for i := 2; i <= 6; i++ {
		logs = append(logs, model.Log{Id: i, Type: model.LogTypeConsume, CreatedAt: old})
	}
	require.NoError(t, db.Create(&logs).Error)
	for i := 2; i <= 6; i++ {
		require.NoError(t, db.Create(&model.LogAggregationEvent{LogID: i, AggregateName: "log_usage_hourly", Status: "applied"}).Error)
	}
	cursor := storageRetentionCursor{}
	preDeleted, logsDeleted, err := runStorageRetentionOnce(context.Background(), storageRetentionConfig{LogDays: 30, BatchSize: 2}, &cursor)
	require.NoError(t, err)
	assert.Equal(t, int64(5), preDeleted)
	assert.Equal(t, int64(5), logsDeleted)
	var remaining []string
	require.NoError(t, db.Model(&model.SubscriptionPreConsumeRecord{}).Pluck("request_id", &remaining).Error)
	assert.Equal(t, []string{"protected-first"}, remaining)
	var remainingLogIDs []int
	require.NoError(t, db.Model(&model.Log{}).Pluck("id", &remainingLogIDs).Error)
	assert.Equal(t, []int{1}, remainingLogIDs)
}

func TestStorageRetentionDisabledLogsAndCancelledBudgetDoNotDelete(t *testing.T) {
	db := setupStorageRetentionServiceDB(t)
	old := time.Now().Add(-40 * 24 * time.Hour).Unix()
	record := model.SubscriptionPreConsumeRecord{RequestId: "expired-budget", Status: "settled", FinalizedAt: old}
	require.NoError(t, db.Create(&record).Error)
	log := model.Log{Type: model.LogTypeConsume, CreatedAt: old}
	require.NoError(t, db.Create(&log).Error)
	require.NoError(t, db.Create(&model.LogAggregationEvent{LogID: log.Id, AggregateName: "log_usage_hourly", Status: "applied"}).Error)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cursor := storageRetentionCursor{}
	_, _, err := runStorageRetentionOnce(ctx, storageRetentionConfig{LogDays: 30, BatchSize: 2}, &cursor)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, db.First(&record, record.Id).Error)
	require.NoError(t, db.First(&log, log.Id).Error)
	preDeleted, logsDeleted, err := runStorageRetentionOnce(context.Background(), storageRetentionConfig{LogDays: 0, BatchSize: 2}, &cursor)
	require.NoError(t, err)
	assert.Equal(t, int64(1), preDeleted)
	assert.Zero(t, logsDeleted)
	require.NoError(t, db.First(&log, log.Id).Error)
}

func TestSubscriptionFinalizationRetryDoesNotRepeatFunding(t *testing.T) {
	db := setupStorageRetentionServiceDB(t)
	subscription := model.UserSubscription{Id: 20, UserId: 10, EntitlementType: model.SubscriptionEntitlementTimed, TokenLimit: 100, TokenUsed: 10, Status: "active"}
	require.NoError(t, db.Create(&subscription).Error)
	record := model.SubscriptionPreConsumeRecord{RequestId: "marker-retry", UserId: 10, UserSubscriptionId: subscription.Id, PreConsumed: 10, Status: "consumed"}
	require.NoError(t, db.Create(&record).Error)
	funding := &SubscriptionFunding{requestId: record.RequestId, subscriptionId: subscription.Id, preConsumed: 10, TokenLimit: 100, TokenUsedAfter: 10, DistributorTokenBilling: true, EntitlementType: model.SubscriptionEntitlementTimed}
	info := newBillingTestRelayInfo(10, 0, "", record.RequestId, "subscription_only")
	session := &BillingSession{funding: funding, relayInfo: info, preConsumedSubscription: 10}
	injected := errors.New("completion marker temporarily unavailable")
	fail := true
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("retention:fail-marker-once", func(tx *gorm.DB) {
		if updates, ok := tx.Statement.Dest.(map[string]interface{}); ok && tx.Statement.Table == "subscription_pre_consume_records" && updates["finalized_at"] != nil && fail {
			fail = false
			tx.AddError(injected)
		}
	}))
	input := BillingSettleInput{SubscriptionTokens: 15}
	require.ErrorIs(t, session.SettleWithInput(input), injected)
	require.NoError(t, db.First(&subscription, subscription.Id).Error)
	assert.Equal(t, int64(15), subscription.TokenUsed)
	require.NoError(t, session.SettleWithInput(input))
	require.NoError(t, db.First(&subscription, subscription.Id).Error)
	assert.Equal(t, int64(15), subscription.TokenUsed)
	assert.Equal(t, int64(5), info.SubscriptionPostDelta)
	require.NoError(t, db.First(&record, record.Id).Error)
	assert.Equal(t, "settled", record.Status)
	assert.Positive(t, record.FinalizedAt)
}

func TestSubscriptionZeroDeltaFinalizesOnlyCompletedRequests(t *testing.T) {
	for _, asyncTask := range []bool{false, true} {
		t.Run(fmt.Sprintf("async=%t", asyncTask), func(t *testing.T) {
			db := setupStorageRetentionServiceDB(t)
			subscription := model.UserSubscription{Id: 20, UserId: 10, EntitlementType: model.SubscriptionEntitlementTimed, TokenLimit: 100, TokenUsed: 10, Status: "active"}
			require.NoError(t, db.Create(&subscription).Error)
			record := model.SubscriptionPreConsumeRecord{RequestId: "zero-delta-final", UserId: 10, UserSubscriptionId: subscription.Id, PreConsumed: 10, Status: "consumed"}
			require.NoError(t, db.Create(&record).Error)
			funding := &SubscriptionFunding{requestId: record.RequestId, subscriptionId: subscription.Id, preConsumed: 10, DistributorTokenBilling: true}
			info := newBillingTestRelayInfo(10, 0, "", record.RequestId, "subscription_only")
			if asyncTask {
				info.RelayFormat = types.RelayFormatTask
			}
			session := &BillingSession{funding: funding, relayInfo: info, preConsumedSubscription: 10}
			require.NoError(t, session.SettleWithInput(BillingSettleInput{SubscriptionTokens: 10}))
			require.NoError(t, db.First(&record, record.Id).Error)
			if asyncTask {
				assert.Equal(t, "consumed", record.Status)
				assert.Zero(t, record.FinalizedAt)
			} else {
				assert.Equal(t, "settled", record.Status)
				assert.Positive(t, record.FinalizedAt)
			}
			require.NoError(t, db.First(&subscription, subscription.Id).Error)
			assert.Equal(t, int64(10), subscription.TokenUsed)
		})
	}
}
