package model

import (
	"database/sql"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type creditValuationPriceBackfillSnapshotRow struct {
	PlanID            int           `gorm:"column:plan_id"`
	PriceType         string        `gorm:"column:price_type"`
	PriceLiteral      string        `gorm:"column:price_literal"`
	PriceAmountMicros sql.NullInt64 `gorm:"column:price_amount_micros"`
}

type creditValuationPriceBackfillSnapshot struct {
	TotalChanges int64
	Rows         []creditValuationPriceBackfillSnapshotRow
}

func openCreditValuationPriceBackfillTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Exec(`CREATE TABLE subscription_plans (id integer PRIMARY KEY, price_amount NUMERIC NOT NULL, price_amount_micros bigint)`).Error)
	return db
}

func takeCreditValuationPriceBackfillSnapshot(t *testing.T, db *gorm.DB) creditValuationPriceBackfillSnapshot {
	t.Helper()
	var snapshot creditValuationPriceBackfillSnapshot
	require.NoError(t, db.Raw(`SELECT total_changes()`).Scan(&snapshot.TotalChanges).Error)
	require.NoError(t, db.Raw(`SELECT id AS plan_id, typeof(price_amount) AS price_type, quote(price_amount) AS price_literal, price_amount_micros FROM subscription_plans ORDER BY id`).Scan(&snapshot.Rows).Error)
	return snapshot
}

func TestRunCreditValuationPlanPriceMigrationDryRunApplyAndRerun(t *testing.T) {
	db := openCreditValuationPriceBackfillTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO subscription_plans (id, price_amount, price_amount_micros) VALUES
		(1, 12.345678, NULL),
		(2, 7.000001, 7000001),
		(3, 0, NULL),
		(4, 19.5, NULL)`).Error)

	before := takeCreditValuationPriceBackfillSnapshot(t, db)
	firstDryRun, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{BatchSize: 1})
	require.NoError(t, err)
	secondDryRun, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{BatchSize: 1})
	require.NoError(t, err)
	afterDryRuns := takeCreditValuationPriceBackfillSnapshot(t, db)

	wantDryRun := CreditValuationPlanPriceMigrationReport{
		RowsTotal:        4,
		RowsAlreadyExact: 1,
		Diagnostics:      []CreditValuationPlanPriceDiagnostic{},
		Batches: []CreditValuationMigrationBatchBoundary{
			{Entity: creditValuationPlanPriceMigrationEntity, StartID: 1, EndID: 1, Rows: 1},
			{Entity: creditValuationPlanPriceMigrationEntity, StartID: 3, EndID: 3, Rows: 1},
			{Entity: creditValuationPlanPriceMigrationEntity, StartID: 4, EndID: 4, Rows: 1},
		},
	}
	require.Equal(t, wantDryRun, firstDryRun)
	require.Equal(t, firstDryRun, secondDryRun)
	require.Equal(t, before, afterDryRuns, "dry-run must not write, including on repeated calls")

	applied, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{Apply: true, BatchSize: 1})
	require.NoError(t, err)
	wantApplied := wantDryRun
	wantApplied.RowsBackfilled = 3
	require.Equal(t, wantApplied, applied)

	var exactRows []struct {
		PlanID            int           `gorm:"column:plan_id"`
		PriceAmountMicros sql.NullInt64 `gorm:"column:price_amount_micros"`
	}
	require.NoError(t, db.Raw(`SELECT id AS plan_id, price_amount_micros FROM subscription_plans ORDER BY id`).Scan(&exactRows).Error)
	require.Equal(t, []struct {
		PlanID            int           `gorm:"column:plan_id"`
		PriceAmountMicros sql.NullInt64 `gorm:"column:price_amount_micros"`
	}{
		{PlanID: 1, PriceAmountMicros: sql.NullInt64{Int64: 12345678, Valid: true}},
		{PlanID: 2, PriceAmountMicros: sql.NullInt64{Int64: 7000001, Valid: true}},
		{PlanID: 3, PriceAmountMicros: sql.NullInt64{Int64: 0, Valid: true}},
		{PlanID: 4, PriceAmountMicros: sql.NullInt64{Int64: 19500000, Valid: true}},
	}, exactRows, "zero is valid and existing exact micros must remain unchanged")

	rerun, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{Apply: true})
	require.NoError(t, err)
	require.Equal(t, CreditValuationPlanPriceMigrationReport{
		RowsTotal:        4,
		RowsAlreadyExact: 4,
		Diagnostics:      []CreditValuationPlanPriceDiagnostic{},
		Batches:          []CreditValuationMigrationBatchBoundary{},
	}, rerun)
}

func TestRunCreditValuationPlanPriceMigrationRejectsExistingMicrosMismatch(t *testing.T) {
	db := openCreditValuationPriceBackfillTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO subscription_plans (id, price_amount, price_amount_micros) VALUES
		(1, 12.5, 12500000),
		(2, 7.25, 7000000),
		(3, -1.25, 0)`).Error)

	before := takeCreditValuationPriceBackfillSnapshot(t, db)
	report, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{BatchSize: 2})
	require.NoError(t, err)
	require.Equal(t, CreditValuationPlanPriceMigrationReport{
		RowsTotal:        3,
		RowsAlreadyExact: 1,
		RowsInvalid:      2,
		Diagnostics: []CreditValuationPlanPriceDiagnostic{
			{PlanID: 2, RawValue: "7.25", Reason: SubscriptionPlanPriceDiagnosticRoundtripMismatch},
			{PlanID: 3, RawValue: "-1.25", Reason: SubscriptionPlanPriceDiagnosticNegative},
		},
		Batches: []CreditValuationMigrationBatchBoundary{},
	}, report)

	applyReport, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{Apply: true, BatchSize: 2})
	require.ErrorIs(t, err, ErrSubscriptionPlanPriceInvalid)
	require.Equal(t, report, applyReport)
	require.Equal(t, before, takeCreditValuationPriceBackfillSnapshot(t, db), "mismatched existing micros must block apply without writes")
}

func TestRunCreditValuationPlanPriceMigrationDiagnosesAllInvalidRowsBeforeApply(t *testing.T) {
	db := openCreditValuationPriceBackfillTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO subscription_plans (id, price_amount, price_amount_micros) VALUES
		(2, -1.25, NULL),
		(4, 2.5, NULL),
		(6, CAST('40.12345600000001' AS REAL), NULL),
		(7, 'not-a-price', NULL),
		(8, CAST('9223372036854.775808' AS BLOB), NULL),
		(9, 1.0000001, NULL),
		(11, 10, 10000000)`).Error)

	var roundtripFixture struct {
		RawValue   string `gorm:"column:price_text"`
		RoundTrips int    `gorm:"column:round_trips"`
	}
	require.NoError(t, db.Raw(`SELECT CAST(price_amount AS TEXT) AS price_text, CASE WHEN price_amount = CAST('40.123456' AS NUMERIC) THEN 1 ELSE 0 END AS round_trips FROM subscription_plans WHERE id = 6`).Scan(&roundtripFixture).Error)
	require.Equal(t, "40.123456", roundtripFixture.RawValue)
	require.Zero(t, roundtripFixture.RoundTrips)

	before := takeCreditValuationPriceBackfillSnapshot(t, db)
	dryRun, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{BatchSize: 2})
	require.NoError(t, err)
	want := CreditValuationPlanPriceMigrationReport{
		RowsTotal:        7,
		RowsAlreadyExact: 1,
		RowsInvalid:      5,
		Diagnostics: []CreditValuationPlanPriceDiagnostic{
			{PlanID: 2, RawValue: "-1.25", Reason: SubscriptionPlanPriceDiagnosticNegative},
			{PlanID: 6, RawValue: "40.123456", Reason: SubscriptionPlanPriceDiagnosticRoundtripMismatch},
			{PlanID: 7, RawValue: "not-a-price", Reason: SubscriptionPlanPriceDiagnosticInvalid},
			{PlanID: 8, RawValue: "9223372036854.775808", Reason: SubscriptionPlanPriceDiagnosticOverflow},
			{PlanID: 9, RawValue: "1.0000001", Reason: SubscriptionPlanPriceDiagnosticPrecision},
		},
		Batches: []CreditValuationMigrationBatchBoundary{
			{Entity: creditValuationPlanPriceMigrationEntity, StartID: 2, EndID: 4, Rows: 2},
			{Entity: creditValuationPlanPriceMigrationEntity, StartID: 6, EndID: 7, Rows: 2},
			{Entity: creditValuationPlanPriceMigrationEntity, StartID: 8, EndID: 9, Rows: 2},
		},
	}
	require.Equal(t, want, dryRun)

	applyReport, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{Apply: true, BatchSize: 2})
	require.ErrorIs(t, err, ErrSubscriptionPlanPriceInvalid)
	require.Equal(t, want, applyReport, "invalid apply retains all diagnostics and records no successful updates")
	require.Zero(t, applyReport.RowsBackfilled)
	after := takeCreditValuationPriceBackfillSnapshot(t, db)
	require.Equal(t, before, after, "invalid apply must fail closed before updating the valid pending row")
}

func TestRunCreditValuationPlanPriceMigrationRollsBackFailedBatch(t *testing.T) {
	db := openCreditValuationPriceBackfillTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO subscription_plans (id, price_amount, price_amount_micros) VALUES (1, 1.25, NULL), (2, 2.5, NULL)`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_second_price_backfill BEFORE UPDATE OF price_amount_micros ON subscription_plans WHEN OLD.id = 2 BEGIN SELECT RAISE(ABORT, 'forced price update failure'); END`).Error)

	report, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{Apply: true, BatchSize: 2})
	require.ErrorContains(t, err, "forced price update failure")
	require.Zero(t, report.RowsBackfilled)
	require.Equal(t, []CreditValuationMigrationBatchBoundary{
		{Entity: creditValuationPlanPriceMigrationEntity, StartID: 1, EndID: 2, Rows: 2},
	}, report.Batches)

	var populated int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM subscription_plans WHERE price_amount_micros IS NOT NULL`).Scan(&populated).Error)
	require.Zero(t, populated, "an update failure must roll back earlier updates from the transaction")
}

func TestCreditValuationPlanPriceMigrationQuerySupportsAllDialects(t *testing.T) {
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
			query, err := creditValuationPlanPriceMigrationQuery(test.dialect)
			require.NoError(t, err)
			require.Contains(t, query, test.cast)
			require.Contains(t, query, "price_amount_micros FROM subscription_plans ORDER BY id")
			require.NotContains(t, query, "float")
		})
	}
	_, err := creditValuationPlanPriceMigrationQuery("unsupported")
	require.Error(t, err)
}

func TestCreditValuationPlanPriceMigrationReportJSONContract(t *testing.T) {
	payload, err := common.Marshal(CreditValuationPlanPriceMigrationReport{
		RowsTotal:   1,
		RowsInvalid: 1,
		Diagnostics: []CreditValuationPlanPriceDiagnostic{{
			PlanID: 7, RawValue: "bad", Reason: SubscriptionPlanPriceDiagnosticInvalid,
		}},
		Batches: []CreditValuationMigrationBatchBoundary{},
	})
	require.NoError(t, err)
	require.Equal(t, "{\"rows_total\":1,\"rows_already_exact\":0,\"rows_backfilled\":0,\"rows_invalid\":1,\"diagnostics\":[{\"plan_id\":7,\"raw_value\":\"bad\",\"reason\":\"invalid_decimal\"}],\"batches\":[]}", string(payload))
}
