package model

import (
	"database/sql"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDiagnosePendingSubscriptionPlanPricesIsReadOnlyAndDeterministic(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:price-diagnostic?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Exec(`CREATE TABLE subscription_plans (id integer PRIMARY KEY, price_amount text NOT NULL, price_amount_micros bigint)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO subscription_plans (id, price_amount, price_amount_micros) VALUES (9, '1.0000001', NULL), (2, '-1.000000', NULL), (5, '40.123456', NULL), (7, 'not-a-price', NULL), (11, '10.000000', 10000000)`).Error)

	before := []sql.NullInt64{}
	require.NoError(t, db.Raw(`SELECT price_amount_micros FROM subscription_plans ORDER BY id`).Scan(&before).Error)
	diagnostics, err := DiagnosePendingSubscriptionPlanPrices(db)
	require.NoError(t, err)
	after := []sql.NullInt64{}
	require.NoError(t, db.Raw(`SELECT price_amount_micros FROM subscription_plans ORDER BY id`).Scan(&after).Error)

	require.Equal(t, []SubscriptionPlanPriceDiagnostic{
		{PlanId: 2, Reason: SubscriptionPlanPriceDiagnosticNegative},
		{PlanId: 7, Reason: SubscriptionPlanPriceDiagnosticInvalid},
		{PlanId: 9, Reason: SubscriptionPlanPriceDiagnosticPrecision},
	}, diagnostics)
	require.Equal(t, before, after)
}
