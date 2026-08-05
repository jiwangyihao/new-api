package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreditValuationSchemaSQLiteMigrationIsAdditiveAndRepeatable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:valuation-schema?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, migrateCreditValuationSchema(db))
	require.NoError(t, migrateCreditValuationSchema(db))
	for _, modelValue := range []any{&CreditValuationState{}, &CreditValuationMigration{}, &TimedSubscriptionValuationGrant{}} {
		require.True(t, db.Migrator().HasTable(modelValue))
	}
	for _, column := range []string{"request_fingerprint_version", "request_fingerprint", "applied_credit", "deducted_exact_cost_micros", "finalized_at"} {
		require.True(t, db.Migrator().HasColumn(&SubscriptionPreConsumeRecord{}, column), column)
	}
	for _, column := range []string{"valuation_currency", "valuation_gross_cost_micros", "fx_rate_denominator"} {
		require.True(t, db.Migrator().HasColumn(&CreditBalanceLedger{}, column), column)
	}
	for _, column := range []string{"valuation_source_price_micros", "valuation_credit_basis", "fx_captured_at"} {
		require.True(t, db.Migrator().HasColumn(&SubscriptionConversion{}, column), column)
	}
}

func TestCreditValuationSchemaSQLiteUniqueConstraints(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:valuation-unique?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, migrateCreditValuationSchema(db))

	state := CreditValuationState{UserSubscriptionId: 101, UserId: 201, Currency: "CNY", RuleVersion: 1, StateVersion: 1}
	require.NoError(t, db.Create(&state).Error)
	require.Error(t, db.Create(&CreditValuationState{UserSubscriptionId: 102, UserId: 201, Currency: "CNY", RuleVersion: 1, StateVersion: 1}).Error)

	grant := TimedSubscriptionValuationGrant{IdempotencyKey: "grant-key", SourceType: "subscription_order", SourceKey: "order:1", SourceCurrency: "CNY", ValuationCurrency: "CNY", Confidence: "exact", RuleVersion: 1, FxRateNumerator: 1, FxRateDenominator: 1}
	require.NoError(t, db.Create(&grant).Error)
	grant.Id = 0
	grant.SourceKey = "order:2"
	require.Error(t, db.Create(&grant).Error)
	grant.IdempotencyKey = "grant-key-2"
	grant.SourceKey = "order:1"
	require.Error(t, db.Create(&grant).Error)
}
