package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSubscriptionPreConsumeLookupBenchmark(b *testing.B) *gorm.DB {
	b.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(b.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{
		PrepareStmt: true,
		Logger:      logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatal(err)
	}
	if err := db.AutoMigrate(&SubscriptionPreConsumeRecord{}); err != nil {
		b.Fatal(err)
	}
	record := SubscriptionPreConsumeRecord{
		RequestId:                 "lookup-hit",
		RequestFingerprintVersion: subscriptionPreConsumeRequestFingerprintVersion,
		RequestFingerprint:        strings.Repeat("f", 64),
		UserId:                    901,
		UserSubscriptionId:        902,
		PreConsumed:               903,
		AppliedCredit:             904,
		DeductedAvailableCredit:   905,
		DebtFormedCredit:          906,
		ValuationSubscriptionId:   907,
		Status:                    "consumed",
	}
	if err := db.Create(&record).Error; err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func loadSubscriptionPreConsumeReplayGORM(db *gorm.DB, requestID string) (SubscriptionPreConsumeRecord, bool, error) {
	var record SubscriptionPreConsumeRecord
	query := db.Where("request_id = ?", requestID).Limit(1).Find(&record)
	return record, query.RowsAffected > 0, query.Error
}

func loadSubscriptionPreConsumeReplayProjectedRow(db *gorm.DB, requestID string) (SubscriptionPreConsumeRecord, bool, error) {
	return loadSubscriptionPreConsumeReplayTx(db, requestID)
}

func TestSubscriptionPreConsumeReplayProjectionMatchesUsedFields(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	record := SubscriptionPreConsumeRecord{
		RequestId:                 "projection-contract",
		RequestFingerprintVersion: subscriptionPreConsumeRequestFingerprintVersion,
		RequestFingerprint:        strings.Repeat("a", 64),
		UserSubscriptionId:        101,
		PreConsumed:               102,
		ValuationSubscriptionId:   103,
		AppliedCredit:             104,
		Status:                    "consumed",
	}
	require.NoError(t, db.Create(&record).Error)

	full, fullFound, fullErr := loadSubscriptionPreConsumeReplayGORM(db, record.RequestId)
	projected, projectedFound, projectedErr := loadSubscriptionPreConsumeReplayProjectedRow(db, record.RequestId)

	require.NoError(t, fullErr)
	require.NoError(t, projectedErr)
	require.True(t, fullFound)
	require.True(t, projectedFound)
	require.Equal(t, full.RequestFingerprintVersion, projected.RequestFingerprintVersion)
	require.Equal(t, full.RequestFingerprint, projected.RequestFingerprint)
	require.Equal(t, full.Status, projected.Status)
	require.Equal(t, full.UserSubscriptionId, projected.UserSubscriptionId)
	require.Equal(t, full.PreConsumed, projected.PreConsumed)
	require.Equal(t, full.ValuationSubscriptionId, projected.ValuationSubscriptionId)
	require.Equal(t, full.AppliedCredit, projected.AppliedCredit)

	_, found, err := loadSubscriptionPreConsumeReplayProjectedRow(db, "projection-miss")
	require.NoError(t, err)
	require.False(t, found)
}

func BenchmarkSubscriptionPreConsumeReplayLookup(b *testing.B) {
	db := setupSubscriptionPreConsumeLookupBenchmark(b)
	for _, requestID := range []string{"lookup-hit", "lookup-miss"} {
		b.Run(requestID, func(b *testing.B) {
			b.Run("gorm_find", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					_, _, err := loadSubscriptionPreConsumeReplayGORM(db, requestID)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("projected_row", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					_, _, err := loadSubscriptionPreConsumeReplayTx(db, requestID)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
