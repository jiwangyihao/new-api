package model

import (
	"database/sql"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type subscriptionPriceDiagnosticRowSnapshot struct {
	PlanId            int           `gorm:"column:plan_id"`
	PriceType         string        `gorm:"column:price_type"`
	PriceLiteral      string        `gorm:"column:price_literal"`
	PriceAmountMicros sql.NullInt64 `gorm:"column:price_amount_micros"`
}

type subscriptionPriceDiagnosticDatabaseSnapshot struct {
	TotalChanges int64
	Plans        []subscriptionPriceDiagnosticRowSnapshot
}

func takeSubscriptionPriceDiagnosticDatabaseSnapshot(t *testing.T, db *gorm.DB) subscriptionPriceDiagnosticDatabaseSnapshot {
	t.Helper()
	var snapshot subscriptionPriceDiagnosticDatabaseSnapshot
	require.NoError(t, db.Raw(`SELECT total_changes()`).Scan(&snapshot.TotalChanges).Error)
	require.NoError(t, db.Raw(`SELECT id AS plan_id, typeof(price_amount) AS price_type, quote(price_amount) AS price_literal, price_amount_micros FROM subscription_plans ORDER BY id`).Scan(&snapshot.Plans).Error)
	return snapshot
}

func TestDiagnosePendingSubscriptionPlanPricesIsReadOnlyAndDeterministic(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:price-diagnostic?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Exec(`CREATE TABLE subscription_plans (id integer PRIMARY KEY, price_amount NUMERIC NOT NULL, price_amount_micros bigint)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO subscription_plans (id, price_amount, price_amount_micros) VALUES (9, 1.0000001, NULL), (2, -1.000000, NULL), (5, 40.123456, NULL), (6, CAST('40.12345600000001' AS REAL), NULL), (7, 'not-a-price', NULL), (11, 10.000000, 10000000)`).Error)

	var fixture struct {
		PriceText  string `gorm:"column:price_text"`
		RoundTrips int    `gorm:"column:round_trips"`
	}
	require.NoError(t, db.Raw(`SELECT CAST(price_amount AS TEXT) AS price_text, CASE WHEN price_amount = CAST('40.123456' AS NUMERIC) THEN 1 ELSE 0 END AS round_trips FROM subscription_plans WHERE id = 6`).Scan(&fixture).Error)
	require.Equal(t, "40.123456", fixture.PriceText, "SQLite exposes a parseable six-decimal surface text")
	require.Zero(t, fixture.RoundTrips, "the stored REAL is numerically distinct from the micros-reconstructed value")

	before := takeSubscriptionPriceDiagnosticDatabaseSnapshot(t, db)
	first, err := DiagnosePendingSubscriptionPlanPrices(db)
	require.NoError(t, err)
	second, err := DiagnosePendingSubscriptionPlanPrices(db)
	require.NoError(t, err)
	after := takeSubscriptionPriceDiagnosticDatabaseSnapshot(t, db)

	want := []SubscriptionPlanPriceDiagnostic{
		{PlanId: 2, Reason: SubscriptionPlanPriceDiagnosticNegative},
		{PlanId: 6, Reason: SubscriptionPlanPriceDiagnosticRoundtripMismatch},
		{PlanId: 7, Reason: SubscriptionPlanPriceDiagnosticInvalid},
		{PlanId: 9, Reason: SubscriptionPlanPriceDiagnosticPrecision},
	}
	require.Equal(t, want, first)
	require.Equal(t, want, second)
	require.Equal(t, before, after, "read-only diagnostics must leave the entire fixture database unchanged")
}

func TestSubscriptionPlanPriceDiagnosticQuerySupportsAllDialects(t *testing.T) {
	tests := []struct {
		dialect string
		cast    string
	}{
		{dialect: "sqlite", cast: "CAST(price_amount AS TEXT)"},
		{dialect: "postgres", cast: "CAST(price_amount AS TEXT)"},
		{dialect: "mysql", cast: "CAST(price_amount AS CHAR)"},
	}
	for _, test := range tests {
		t.Run(test.dialect, func(t *testing.T) {
			query, err := subscriptionPlanPriceDiagnosticQuery(test.dialect)
			require.NoError(t, err)
			require.Contains(t, query, test.cast)
			require.Contains(t, query, "WHERE price_amount_micros IS NULL ORDER BY id")
		})
	}
	_, err := subscriptionPlanPriceDiagnosticQuery("unsupported")
	require.Error(t, err)
}
