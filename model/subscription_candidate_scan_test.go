package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

func setupSubscriptionCandidateScanBenchmark(b *testing.B) *gorm.DB {
	b.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(b.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{
		PrepareStmt: true,
		Logger:      logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatal(err)
	}
	if err := db.AutoMigrate(&UserSubscription{}); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		singleton := fmt.Sprintf("singleton-%d", i)
		credit := int64(i * 10)
		sub := UserSubscription{
			UserId:                    1,
			PlanId:                    i + 10,
			EntitlementType:           SubscriptionEntitlementTimed,
			SingletonKey:              &singleton,
			AmountTotal:               1000,
			AmountUsed:                int64(i),
			TokenLimit:                10000,
			TokenUsed:                 int64(i),
			ConcurrencyLimit:          3,
			GrantReason:               "benchmark",
			GrantSourceUserId:         2,
			LastGrantedAt:             3,
			LastGrantCreditSnapshot:   &credit,
			LastGrantTimeSource:       "source",
			LastGrantSource:           "admin",
			StartTime:                 1,
			EndTime:                   1<<62 - 1,
			Status:                    SubscriptionStatusActive,
			ConvertedAt:               4,
			ConversionId:              5,
			ConvertedToSubscriptionId: 6,
			Source:                    "admin",
			LastResetTime:             7,
			NextResetTime:             8,
			UpgradeGroup:              "pro",
			PrevUserGroup:             "default",
		}
		if err := db.Create(&sub).Error; err != nil {
			b.Fatal(err)
		}
	}
	b.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func loadSubscriptionCandidatesGORMForBenchmark(db *gorm.DB) ([]UserSubscription, error) {
	var subscriptions []UserSubscription
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND status = ? AND start_time <= ? AND (entitlement_type = ? OR end_time > ?)", 1, SubscriptionStatusActive, int64(100), SubscriptionEntitlementCreditBalance, int64(100)).
		Find(&subscriptions).Error
	return subscriptions, err
}

func TestLoadBillableSubscriptionRowsPreservesRequiredFields(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	singleton := "credit_balance"
	credit := int64(321)
	want := UserSubscription{UserId: 9101, PlanId: 9102, EntitlementType: SubscriptionEntitlementCreditBalance, SingletonKey: &singleton, AmountTotal: 11, AmountUsed: 12, TokenLimit: 13, TokenUsed: 14, ConcurrencyLimit: 15, GrantReason: "reason", GrantSourceUserId: 16, LastGrantedAt: 17, LastGrantCreditSnapshot: &credit, LastGrantTimeSource: "clock", LastGrantSource: "source", StartTime: 18, EndTime: 19, Status: SubscriptionStatusActive, ConvertedAt: 20, ConversionId: 21, ConvertedToSubscriptionId: 22, Source: "admin", LastResetTime: 23, NextResetTime: 24, UpgradeGroup: "pro", PrevUserGroup: "default"}
	require.NoError(t, db.Create(&want).Error)
	got, err := loadBillableSubscriptionRowsTx(db, want.UserId, 18, true)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, want.Id, got[0].Id)
	require.Equal(t, want.AmountTotal, got[0].AmountTotal)
	require.Equal(t, want.TokenLimit, got[0].TokenLimit)
	require.Equal(t, want.GrantReason, got[0].GrantReason)
	require.Equal(t, want.NextResetTime, got[0].NextResetTime)
}

func TestMaybeResetUserSubscriptionUpdatesOnlyResetFields(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	now := int64(1_800_000_000)
	plan := SubscriptionPlan{QuotaResetPeriod: SubscriptionResetDaily}
	singleton := "preserve-singleton"
	credit := int64(777)
	sub := UserSubscription{UserId: 9201, PlanId: 9202, EntitlementType: SubscriptionEntitlementTimed, SingletonKey: &singleton, AmountTotal: 100, AmountUsed: 50, TokenLimit: 200, TokenUsed: 75, ConcurrencyLimit: 3, GrantReason: "preserve-reason", GrantSourceUserId: 9, LastGrantedAt: 10, LastGrantCreditSnapshot: &credit, LastGrantTimeSource: "preserve-time", LastGrantSource: "preserve-source", StartTime: now - 3*86400, EndTime: now + 10*86400, Status: SubscriptionStatusActive, ConvertedAt: 11, ConversionId: 12, ConvertedToSubscriptionId: 13, Source: "admin", LastResetTime: now - 2*86400, NextResetTime: now - 86400, UpgradeGroup: "pro", PrevUserGroup: "default"}
	require.NoError(t, db.Create(&sub).Error)
	partialRows, err := loadBillableSubscriptionRowsTx(db, sub.UserId, now, true)
	require.NoError(t, err)
	require.Len(t, partialRows, 1)
	require.NoError(t, maybeResetUserSubscriptionWithPlanTx(db, &partialRows[0], &plan, now))
	var persisted UserSubscription
	require.NoError(t, db.First(&persisted, sub.Id).Error)
	require.Equal(t, sub.SingletonKey, persisted.SingletonKey)
	require.Equal(t, sub.LastGrantCreditSnapshot, persisted.LastGrantCreditSnapshot)
	require.Equal(t, sub.LastGrantTimeSource, persisted.LastGrantTimeSource)
	require.Equal(t, sub.UpgradeGroup, persisted.UpgradeGroup)
	require.Zero(t, persisted.AmountUsed)
	require.Zero(t, persisted.TokenUsed)
}

func BenchmarkLoadBillableSubscriptionRows(b *testing.B) {
	db := setupSubscriptionCandidateScanBenchmark(b)
	b.Run("gorm_find", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := loadSubscriptionCandidatesGORMForBenchmark(db); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("rows_scan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := loadBillableSubscriptionRowsTx(db, 1, 100, true); err != nil {
				b.Fatal(err)
			}
		}
	})
}
