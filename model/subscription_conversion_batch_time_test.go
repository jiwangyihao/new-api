package model

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTimedConversionBatchTest(t testing.TB, count int) (*gorm.DB, []timedConversionInFlightRequest, *CreditValuationSourceSnapshot, *dbTimeCountingLogger) {
	t.Helper()
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	capture := &dbTimeCountingLogger{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "conversion-batch.db")) + "?_pragma=busy_timeout(5000)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: capture})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	requests := make([]timedConversionInFlightRequest, 0, count)
	for i := 0; i < count; i++ {
		record := SubscriptionPreConsumeRecord{
			Id: i + 1, RequestId: fmt.Sprintf("conversion-batch-%d", i), UserId: 1,
			UserSubscriptionId: 2, Status: "consumed", PreConsumed: int64(i + 1),
		}
		require.NoError(t, db.Create(&record).Error)
		requests = append(requests, timedConversionInFlightRequest{id: record.Id, requestID: record.RequestId, preConsumed: record.PreConsumed})
	}
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
	})
	fx := &CreditFXRateSnapshot{Numerator: 1, Denominator: 1}
	source := &CreditValuationSourceSnapshot{SourcePriceMicros: 1000, SourcePlanCredit: 100, FXRateSnapshot: fx}
	return db, requests, source, capture
}

func TestApplyTimedConversionInFlightRequestsUsesOneDatabaseTimestamp(t *testing.T) {
	db, requests, source, capture := setupTimedConversionBatchTest(t, 8)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return applyTimedConversionInFlightRequestsTx(tx, requests, 99, source)
	}))
	require.Equal(t, int64(1), capture.timestampSelects.Load())
	var records []SubscriptionPreConsumeRecord
	require.NoError(t, db.Order("id").Find(&records).Error)
	require.Len(t, records, len(requests))
	for i := range records {
		require.Equal(t, records[0].UpdatedAt, records[i].UpdatedAt)
		require.Equal(t, 99, records[i].ValuationSubscriptionId)
	}
}

func TestApplyTimedConversionInFlightRequestsEmptyBatchSkipsDatabaseTimestamp(t *testing.T) {
	db, _, source, capture := setupTimedConversionBatchTest(t, 0)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return applyTimedConversionInFlightRequestsTx(tx, nil, 99, source)
	}))
	require.Zero(t, capture.timestampSelects.Load())
}

func TestApplyTimedConversionInFlightRequestsConflictRollsBackBatch(t *testing.T) {
	db, requests, source, _ := setupTimedConversionBatchTest(t, 2)
	var before []SubscriptionPreConsumeRecord
	require.NoError(t, db.Order("id").Find(&before).Error)
	requests[1].requestID = "conflicting-request"

	err := db.Transaction(func(tx *gorm.DB) error {
		return applyTimedConversionInFlightRequestsTx(tx, requests, 99, source)
	})
	require.ErrorIs(t, err, ErrCreditValuationTargetConflict)
	var after []SubscriptionPreConsumeRecord
	require.NoError(t, db.Order("id").Find(&after).Error)
	require.Equal(t, before, after)
}
