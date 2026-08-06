package model

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type subscriptionConversionReplayPostgresPlan struct {
	Id                int    `gorm:"primaryKey"`
	PriceAmountMicros int64  `gorm:"type:bigint;not null"`
	Currency          string `gorm:"type:varchar(8);not null"`
}

func (subscriptionConversionReplayPostgresPlan) TableName() string { return "subscription_plans" }

type subscriptionConversionReplayPostgresLedger struct {
	Id                         int   `gorm:"primaryKey"`
	ValuationStateVersionAfter int64 `gorm:"type:bigint;not null"`
}

func (subscriptionConversionReplayPostgresLedger) TableName() string {
	return "credit_balance_ledgers"
}

type subscriptionConversionReplayPostgresConversion struct {
	Id                         int    `gorm:"primaryKey"`
	UserId                     int    `gorm:"not null"`
	IdempotencyKey             string `gorm:"type:varchar(128);not null"`
	SourceSubscriptionId       int    `gorm:"not null"`
	SourcePlanId               int    `gorm:"not null"`
	LedgerId                   int    `gorm:"not null"`
	CreditBasis                int64  `gorm:"type:bigint;not null"`
	GrossCredit                int64  `gorm:"type:bigint;not null"`
	ValuationSourcePriceMicros int64  `gorm:"type:bigint;not null"`
	ValuationCreditBasis       int64  `gorm:"type:bigint;not null"`
	ValuationRuleVersion       int    `gorm:"not null"`
	FxSourceCurrency           string `gorm:"type:varchar(8);not null"`
}

func (subscriptionConversionReplayPostgresConversion) TableName() string {
	return "subscription_conversions"
}

func TestFindCommittedSubscriptionConversionPostgreSQLLocksOnlyConversionRow(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run the real PostgreSQL replay-lock compatibility test")
	}

	postgresDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := postgresDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var serverVersion string
	require.NoError(t, postgresDB.Raw("SELECT version()").Scan(&serverVersion).Error)
	t.Logf("PostgreSQL server version: %s", serverVersion)

	var identity struct {
		Database string
		User     string
	}
	require.NoError(t, postgresDB.Raw("SELECT current_database() AS database, current_user AS user").Scan(&identity).Error)
	require.Contains(t, strings.ToLower(identity.Database), "issue26", "refusing to use a database not dedicated to issue 26")
	require.Contains(t, strings.ToLower(identity.User), "issue26", "refusing to use a user not dedicated to issue 26")
	existingTables, err := postgresDB.Migrator().GetTables()
	require.NoError(t, err)
	require.Empty(t, existingTables, "refusing to run against a non-empty PostgreSQL database")

	require.NoError(t, postgresDB.AutoMigrate(
		&subscriptionConversionReplayPostgresPlan{},
		&subscriptionConversionReplayPostgresLedger{},
		&subscriptionConversionReplayPostgresConversion{},
	))
	t.Cleanup(func() {
		_ = postgresDB.Migrator().DropTable(
			&subscriptionConversionReplayPostgresConversion{},
			&subscriptionConversionReplayPostgresLedger{},
			&subscriptionConversionReplayPostgresPlan{},
		)
	})

	const (
		userID               = 26_901
		sourcePlanID         = 26_902
		sourceSubscriptionID = 26_903
		ledgerID             = 26_904
		conversionID         = 26_905
		idempotencyKey       = "postgres-replay-lock"
	)
	require.NoError(t, postgresDB.Create(&subscriptionConversionReplayPostgresPlan{
		Id: sourcePlanID, PriceAmountMicros: 40_000_000, Currency: "CNY",
	}).Error)
	require.NoError(t, postgresDB.Create(&subscriptionConversionReplayPostgresLedger{
		Id: ledgerID, ValuationStateVersionAfter: 7,
	}).Error)
	require.NoError(t, postgresDB.Create(&subscriptionConversionReplayPostgresConversion{
		Id: conversionID, UserId: userID, IdempotencyKey: idempotencyKey,
		SourceSubscriptionId: sourceSubscriptionID, SourcePlanId: sourcePlanID, LedgerId: ledgerID,
		CreditBasis: 100, GrossCredit: 100, ValuationSourcePriceMicros: 40_000_000,
		ValuationCreditBasis: 100, ValuationRuleVersion: CreditValuationRuleVersion, FxSourceCurrency: "CNY",
	}).Error)

	oldDB := DB
	DB = postgresDB
	t.Cleanup(func() { DB = oldDB })

	replay, err := findCommittedSubscriptionConversion(userID, sourceSubscriptionID, idempotencyKey)
	require.NoError(t, err)
	require.NotNil(t, replay)
	require.Equal(t, int64(7), replay.ValuationStateVersionAfter)

	require.NoError(t, postgresDB.Model(&subscriptionConversionReplayPostgresPlan{}).
		Where("id = ?", sourcePlanID).
		UpdateColumn("price_amount_micros", int64(41_000_000)).Error)
	replay, err = findCommittedSubscriptionConversion(userID, sourceSubscriptionID, idempotencyKey)
	require.ErrorIs(t, err, ErrConversionIdempotencyConflict)
	require.Nil(t, replay)
}
