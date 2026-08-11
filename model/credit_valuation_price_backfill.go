package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const (
	creditValuationPlanPriceMigrationDefaultBatchSize = 100
	creditValuationPlanPriceMigrationEntity           = "subscription_plan_price"
)

type CreditValuationPlanPriceMigrationRequest struct {
	Apply     bool
	BatchSize int
}

type CreditValuationPlanPriceDiagnostic struct {
	PlanID   int    `json:"plan_id"`
	RawValue string `json:"raw_value"`
	Reason   string `json:"reason"`
}

type CreditValuationPlanPriceMigrationReport struct {
	RowsTotal        int64                                   `json:"rows_total"`
	RowsAlreadyExact int64                                   `json:"rows_already_exact"`
	RowsBackfilled   int64                                   `json:"rows_backfilled"`
	RowsInvalid      int64                                   `json:"rows_invalid"`
	Diagnostics      []CreditValuationPlanPriceDiagnostic    `json:"diagnostics"`
	Batches          []CreditValuationMigrationBatchBoundary `json:"batches"`
}

type creditValuationPlanPriceMigrationRow struct {
	PlanID            int            `gorm:"column:plan_id"`
	RawValue          sql.NullString `gorm:"column:price_text"`
	PriceAmountMicros sql.NullInt64  `gorm:"column:price_amount_micros"`
}

type creditValuationPlanPricePendingRow struct {
	PlanID       int
	RawValue     string
	AmountMicros int64
}

// RunCreditValuationPlanPriceMigration diagnoses every subscription plan from
// its database decimal text and optionally backfills missing exact micros.
// Apply mode performs the complete diagnosis before its first update and keeps
// every update in one transaction.
func RunCreditValuationPlanPriceMigration(db *gorm.DB, request CreditValuationPlanPriceMigrationRequest) (CreditValuationPlanPriceMigrationReport, error) {
	if db == nil {
		db = DB
	}
	if db == nil {
		return newCreditValuationPlanPriceMigrationReport(), ErrDatabase
	}
	batchSize := request.BatchSize
	if batchSize <= 0 {
		batchSize = creditValuationPlanPriceMigrationDefaultBatchSize
	}
	if !request.Apply {
		report, _, err := inspectCreditValuationPlanPrices(db, batchSize)
		return report, err
	}

	report := newCreditValuationPlanPriceMigrationReport()
	var rowsBackfilled int64
	err := db.Transaction(func(tx *gorm.DB) error {
		var pending []creditValuationPlanPricePendingRow
		var err error
		report, pending, err = inspectCreditValuationPlanPrices(tx, batchSize)
		if err != nil {
			return err
		}
		if report.RowsInvalid != 0 {
			return fmt.Errorf("%w: %d invalid price row(s)", ErrSubscriptionPlanPriceInvalid, report.RowsInvalid)
		}
		for start := 0; start < len(pending); start += batchSize {
			end := start + batchSize
			if end > len(pending) {
				end = len(pending)
			}
			var batchRowsAffected int64
			for _, row := range pending[start:end] {
				result := tx.Table("subscription_plans").
					Where("id = ? AND price_amount_micros IS NULL AND price_amount = ?", row.PlanID, row.RawValue).
					Update("price_amount_micros", row.AmountMicros)
				if result.Error != nil {
					return result.Error
				}
				batchRowsAffected += result.RowsAffected
			}
			batchSizeExpected := int64(end - start)
			if batchRowsAffected != batchSizeExpected {
				return fmt.Errorf("%w: price batch %d-%d updated %d of %d rows", ErrSubscriptionPlanPriceMismatch, pending[start].PlanID, pending[end-1].PlanID, batchRowsAffected, batchSizeExpected)
			}
			rowsBackfilled += batchRowsAffected
		}
		return nil
	})
	if err != nil {
		return report, err
	}
	report.RowsBackfilled = rowsBackfilled
	return report, nil
}

func newCreditValuationPlanPriceMigrationReport() CreditValuationPlanPriceMigrationReport {
	return CreditValuationPlanPriceMigrationReport{
		Diagnostics: make([]CreditValuationPlanPriceDiagnostic, 0),
		Batches:     make([]CreditValuationMigrationBatchBoundary, 0),
	}
}

func inspectCreditValuationPlanPrices(db *gorm.DB, batchSize int) (CreditValuationPlanPriceMigrationReport, []creditValuationPlanPricePendingRow, error) {
	report := newCreditValuationPlanPriceMigrationReport()
	query, err := creditValuationPlanPriceMigrationQuery(db.Dialector.Name())
	if err != nil {
		return report, nil, err
	}
	var rows []creditValuationPlanPriceMigrationRow
	if err := db.Raw(query).Scan(&rows).Error; err != nil {
		return report, nil, err
	}
	report.RowsTotal = int64(len(rows))
	pending := make([]creditValuationPlanPricePendingRow, 0)
	for _, row := range rows {
		rawValue := ""
		if row.RawValue.Valid {
			rawValue = row.RawValue.String
		}
		candidate := creditValuationPlanPricePendingRow{PlanID: row.PlanID, RawValue: rawValue}
		amountMicros, parseErr := ParseDecimalAmountMicros(rawValue)
		if !row.RawValue.Valid {
			parseErr = ErrSubscriptionPlanPriceInvalid
		}
		if parseErr != nil {
			report.Diagnostics = append(report.Diagnostics, CreditValuationPlanPriceDiagnostic{
				PlanID: row.PlanID, RawValue: rawValue, Reason: creditValuationPlanPriceDiagnosticReason(parseErr),
			})
			if !row.PriceAmountMicros.Valid {
				pending = append(pending, candidate)
			}
			continue
		}
		candidate.AmountMicros = amountMicros
		if db.Dialector.Name() == "sqlite" {
			matches, compareErr := sqliteSubscriptionPlanPriceRoundtripMatches(db, row.PlanID, amountMicros)
			if compareErr != nil {
				return report, nil, compareErr
			}
			if !matches {
				report.Diagnostics = append(report.Diagnostics, CreditValuationPlanPriceDiagnostic{
					PlanID: row.PlanID, RawValue: rawValue, Reason: SubscriptionPlanPriceDiagnosticRoundtripMismatch,
				})
				if !row.PriceAmountMicros.Valid {
					pending = append(pending, candidate)
				}
				continue
			}
		}
		if row.PriceAmountMicros.Valid {
			if row.PriceAmountMicros.Int64 != amountMicros {
				report.Diagnostics = append(report.Diagnostics, CreditValuationPlanPriceDiagnostic{
					PlanID: row.PlanID, RawValue: rawValue, Reason: SubscriptionPlanPriceDiagnosticRoundtripMismatch,
				})
			} else {
				report.RowsAlreadyExact++
			}
			continue
		}
		pending = append(pending, candidate)
	}
	report.RowsInvalid = int64(len(report.Diagnostics))
	for start := 0; start < len(pending); start += batchSize {
		end := start + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		report.Batches = append(report.Batches, CreditValuationMigrationBatchBoundary{
			Entity:  creditValuationPlanPriceMigrationEntity,
			StartID: int64(pending[start].PlanID),
			EndID:   int64(pending[end-1].PlanID),
			Rows:    int64(end - start),
		})
	}
	return report, pending, nil
}

func creditValuationPlanPriceMigrationQuery(dialect string) (string, error) {
	pendingQuery, err := subscriptionPlanPriceDiagnosticQuery(dialect)
	if err != nil {
		return "", err
	}
	const pendingSuffix = " FROM subscription_plans WHERE price_amount_micros IS NULL ORDER BY id"
	if !strings.HasSuffix(pendingQuery, pendingSuffix) {
		return "", fmt.Errorf("subscription plan price diagnostic query contract changed")
	}
	return strings.TrimSuffix(pendingQuery, pendingSuffix) +
		", price_amount_micros FROM subscription_plans ORDER BY id", nil
}

func creditValuationPlanPriceDiagnosticReason(err error) string {
	switch {
	case errors.Is(err, ErrSubscriptionPlanPriceNegative):
		return SubscriptionPlanPriceDiagnosticNegative
	case errors.Is(err, ErrSubscriptionPlanPricePrecision):
		return SubscriptionPlanPriceDiagnosticPrecision
	case errors.Is(err, ErrCreditValuationOverflow):
		return SubscriptionPlanPriceDiagnosticOverflow
	default:
		return SubscriptionPlanPriceDiagnosticInvalid
	}
}
