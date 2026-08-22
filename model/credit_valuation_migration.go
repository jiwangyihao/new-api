package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	CreditValuationMigrationPending   = "pending"
	CreditValuationMigrationRunning   = "running"
	CreditValuationMigrationReady     = "ready"
	CreditValuationMigrationFailed    = "failed"
	CreditValuationMigrationSuspended = "suspended"
)

const (
	creditValuationMigrationBlockerPreConsume          = "non_terminal_preconsume"
	creditValuationMigrationBlockerAsyncTaskIdentity   = "active_subscription_task_missing_request_identity"
	creditValuationMigrationBlockerLegacyWriterSession = "legacy_writer_session_active"
	creditValuationMigrationBlockerCreditPlanMissing   = "credit_plan_missing"
	creditValuationMigrationBlockerCreditPlanAmbiguous = "credit_plan_ambiguous"
	creditValuationMigrationBlockerValuationCurrency   = "valuation_currency_invalid"
	creditValuationMigrationBlockerFXMissing           = "fx_option_missing"
	creditValuationMigrationBlockerFXInvalid           = "fx_option_invalid"
	creditValuationMigrationFailureRepairRequiresApply = "repair_requires_verify_apply"
	creditValuationMigrationFailureExecution           = "migration_execution_failed"
	creditValuationMigrationFailureBlocked             = "migration_blocked"
	creditValuationMigrationLegacyWriterSessionOption  = "CreditValuationLegacyWriterSession"
	creditValuationMigrationWriterSessionOption        = "CreditValuationWriterSession"
	creditValuationMigrationRunLeaseSeconds            = int64(300)
)

type CreditValuationMigrationMode string

const (
	CreditValuationMigrationModeDryRun                 CreditValuationMigrationMode = "dry_run"
	CreditValuationMigrationModeApply                  CreditValuationMigrationMode = "apply"
	CreditValuationMigrationModeVerify                 CreditValuationMigrationMode = "verify"
	CreditValuationMigrationModeRepairMissingAsUnknown CreditValuationMigrationMode = "repair_missing_as_unknown"
	CreditValuationMigrationModeSuspend                CreditValuationMigrationMode = "suspend"
)

type CreditValuationMigrationRequest struct {
	Mode      CreditValuationMigrationMode
	Version   int
	BatchSize int
	Reason    string
}

type CreditValuationMigrationFXSnapshot struct {
	SourceCurrency    string `json:"source_currency"`
	ValuationCurrency string `json:"valuation_currency"`
	Numerator         int64  `json:"numerator,string"`
	Denominator       int64  `json:"denominator,string"`
	CapturedAt        int64  `json:"captured_at"`
}

type CreditValuationMigrationReasonCount struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

type CreditValuationMigrationBlocker struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

type CreditValuationMigrationBatchBoundary struct {
	Entity  string `json:"entity"`
	StartID int64  `json:"start_id"`
	EndID   int64  `json:"end_id"`
	Rows    int64  `json:"rows"`
}

type CreditValuationMigrationReport struct {
	Version           int                                                `json:"version"`
	Mode              CreditValuationMigrationMode                       `json:"mode"`
	Status            string                                             `json:"status"`
	ValuationCurrency string                                             `json:"valuation_currency"`
	FX                CreditValuationMigrationFXSnapshot                 `json:"fx"`
	Price             CreditValuationPlanPriceMigrationReport            `json:"price"`
	Credit            CreditValuationHistoricalBackfillReport            `json:"credit"`
	Timed             TimedSubscriptionValuationHistoricalBackfillReport `json:"timed"`
	Reasons           []CreditValuationMigrationReasonCount              `json:"reasons"`
	Blockers          []CreditValuationMigrationBlocker                  `json:"blockers"`
	Batches           []CreditValuationMigrationBatchBoundary            `json:"batches"`
	Checksum          string                                             `json:"checksum"`
	ReadOnly          bool                                               `json:"read_only"`
	Changed           bool                                               `json:"changed"`
	Ready             bool                                               `json:"ready"`
}

func ValidateCreditValuationMigrationRequest(request CreditValuationMigrationRequest) error {
	if request.Version <= 0 {
		return ErrCreditValuationMigrationVersionRequired
	}
	if request.BatchSize <= 0 {
		return ErrCreditValuationMigrationBatchInvalid
	}
	reason := strings.TrimSpace(request.Reason)
	switch request.Mode {
	case CreditValuationMigrationModeDryRun,
		CreditValuationMigrationModeApply,
		CreditValuationMigrationModeVerify,
		CreditValuationMigrationModeRepairMissingAsUnknown:
		if reason != "" {
			return ErrCreditValuationMigrationReasonInvalid
		}
		return nil
	case CreditValuationMigrationModeSuspend:
		if reason == "" {
			return ErrCreditValuationMigrationReasonInvalid
		}
		return nil
	default:
		return ErrCreditValuationMigrationModeInvalid
	}
}

type CreditValuationMigration struct {
	Version             int    `json:"version" gorm:"primaryKey;autoIncrement:false"`
	Status              string `json:"status" gorm:"type:varchar(16);not null;index:idx_credit_valuation_migrations_status"`
	ValuationCurrency   string `json:"valuation_currency" gorm:"type:varchar(8);not null;default:''"`
	FxRateNumerator     int64  `json:"fx_rate_numerator,string" gorm:"type:bigint;not null;default:0"`
	FxRateDenominator   int64  `json:"fx_rate_denominator,string" gorm:"type:bigint;not null;default:0"`
	FxCapturedAt        int64  `json:"fx_captured_at" gorm:"type:bigint;not null;default:0"`
	CreditRowsTotal     int64  `json:"credit_rows_total" gorm:"type:bigint;not null;default:0"`
	CreditRowsEstimated int64  `json:"credit_rows_estimated" gorm:"type:bigint;not null;default:0"`
	CreditRowsUnknown   int64  `json:"credit_rows_unknown" gorm:"type:bigint;not null;default:0"`
	TimedRowsTotal      int64  `json:"timed_rows_total" gorm:"type:bigint;not null;default:0"`
	TimedRowsEstimated  int64  `json:"timed_rows_estimated" gorm:"type:bigint;not null;default:0"`
	TimedRowsUnknown    int64  `json:"timed_rows_unknown" gorm:"type:bigint;not null;default:0"`
	Checksum            string `json:"checksum" gorm:"type:varchar(64);not null;default:''"`
	PreflightChecksum   string `json:"preflight_checksum" gorm:"type:varchar(64);not null;default:''"`
	RunLeaseExpiresAt   int64  `json:"run_lease_expires_at" gorm:"type:bigint;not null;default:0"`
	StartedAt           int64  `json:"started_at" gorm:"type:bigint;not null;default:0"`
	CompletedAt         int64  `json:"completed_at" gorm:"type:bigint;not null;default:0"`
	FailureReason       string `json:"failure_reason" gorm:"type:text"`
	SuspendedAt         int64  `json:"suspended_at" gorm:"type:bigint;not null;default:0"`
	SuspendedReason     string `json:"suspended_reason" gorm:"type:text"`
}

type creditValuationMigrationSnapshot struct {
	report               CreditValuationMigrationReport
	verificationFailures []CreditValuationMigrationReasonCount
}

type creditValuationMigrationChecksumPayload struct {
	Version           int                                                `json:"version"`
	ValuationCurrency string                                             `json:"valuation_currency"`
	FX                creditValuationMigrationChecksumFX                 `json:"fx"`
	Price             CreditValuationPlanPriceMigrationReport            `json:"price"`
	Credit            CreditValuationHistoricalBackfillReport            `json:"credit"`
	Timed             TimedSubscriptionValuationHistoricalBackfillReport `json:"timed"`
	Reasons           []CreditValuationMigrationReasonCount              `json:"reasons"`
	Blockers          []CreditValuationMigrationBlocker                  `json:"blockers"`
	Batches           []CreditValuationMigrationBatchBoundary            `json:"batches"`
}

type creditValuationMigrationChecksumFX struct {
	SourceCurrency    string `json:"source_currency"`
	ValuationCurrency string `json:"valuation_currency"`
	Numerator         int64  `json:"numerator,string"`
	Denominator       int64  `json:"denominator,string"`
}

func RunCreditValuationMigration(db *gorm.DB, request CreditValuationMigrationRequest) (CreditValuationMigrationReport, error) {
	if db == nil {
		return CreditValuationMigrationReport{}, ErrDatabase
	}
	if err := ValidateCreditValuationMigrationRequest(request); err != nil {
		return CreditValuationMigrationReport{}, err
	}
	if !db.Migrator().HasTable(&CreditValuationMigration{}) {
		return CreditValuationMigrationReport{}, ErrCreditValuationMigrationNotReady
	}
	switch request.Mode {
	case CreditValuationMigrationModeDryRun:
		return runCreditValuationMigrationReadOnly(db, request, false)
	case CreditValuationMigrationModeVerify:
		return runCreditValuationMigrationReadOnly(db, request, true)
	case CreditValuationMigrationModeApply:
		return runCreditValuationMigrationApply(db, request)
	case CreditValuationMigrationModeRepairMissingAsUnknown:
		return runCreditValuationMigrationRepair(db, request)
	case CreditValuationMigrationModeSuspend:
		return runCreditValuationMigrationSuspend(db, request)
	default:
		return CreditValuationMigrationReport{}, ErrCreditValuationMigrationModeInvalid
	}
}

func runCreditValuationMigrationReadOnly(db *gorm.DB, request CreditValuationMigrationRequest, verify bool) (CreditValuationMigrationReport, error) {
	marker, markerFound, err := findCreditValuationMigration(db, request.Version)
	if err != nil {
		return CreditValuationMigrationReport{}, err
	}
	if verify {
		highest, highestFound, highestErr := findHighestCreditValuationMigration(db)
		if highestErr != nil {
			return CreditValuationMigrationReport{}, highestErr
		}
		if !markerFound || !highestFound || highest.Version != request.Version || marker.Status != CreditValuationMigrationReady {
			report := creditValuationMigrationReportFromMarker(marker, request.Mode, true, false)
			if !markerFound {
				report.Version = request.Version
				report.Status = CreditValuationMigrationPending
			}
			report.Ready = false
			return report, ErrCreditValuationMigrationNotReady
		}
	}
	fx, currency, initialBlockers, err := migrationSnapshotInputs(db, marker, markerFound, true)
	if err != nil {
		return CreditValuationMigrationReport{}, err
	}
	snapshot, err := buildCreditValuationMigrationSnapshot(db, request, currency, fx, initialBlockers, true)
	if err != nil {
		return snapshot.report, err
	}
	report := snapshot.report
	report.ReadOnly = true
	report.Changed = false
	if markerFound {
		report.Status = marker.Status
		report.Ready = marker.Status == CreditValuationMigrationReady
	} else {
		report.Status = CreditValuationMigrationPending
	}
	if !verify {
		return report, nil
	}
	if !markerFound {
		return report, ErrCreditValuationMigrationNotReady
	}
	if err := verifyCreditValuationMigrationSnapshot(snapshot, marker, true); err != nil {
		return report, err
	}
	return report, nil
}

func runCreditValuationMigrationApply(db *gorm.DB, request CreditValuationMigrationRequest) (CreditValuationMigrationReport, error) {
	existing, found, err := findCreditValuationMigration(db, request.Version)
	if err != nil {
		return CreditValuationMigrationReport{}, err
	}
	if found && existing.Status == CreditValuationMigrationReady {
		highest, highestFound, highestErr := findHighestCreditValuationMigration(db)
		if highestErr != nil {
			return CreditValuationMigrationReport{}, highestErr
		}
		if !highestFound || highest.Version != request.Version {
			return creditValuationMigrationReportFromMarker(existing, request.Mode, false, false), ErrCreditValuationMigrationConflict
		}
		report, verifyErr := runCreditValuationMigrationReadOnly(db, CreditValuationMigrationRequest{
			Mode: CreditValuationMigrationModeVerify, Version: request.Version, BatchSize: request.BatchSize,
		}, true)
		report.Mode = CreditValuationMigrationModeApply
		report.ReadOnly = false
		report.Changed = false
		return report, verifyErr
	}
	fx, currency, initialBlockers, err := migrationSnapshotInputs(db, existing, found, true)
	if err != nil {
		return CreditValuationMigrationReport{}, err
	}
	preflight, err := buildCreditValuationMigrationSnapshot(db, request, currency, fx, initialBlockers, false)
	if err != nil {
		return preflight.report, err
	}
	marker, err := claimCreditValuationMigration(db, request, currency, fx, preflight.report.Checksum, false)
	if err != nil {
		return preflight.report, err
	}
	if len(preflight.report.Blockers) > 0 || preflight.report.Price.RowsInvalid > 0 {
		if markerErr := markCreditValuationMigrationFailed(db, marker, creditValuationMigrationFailureBlocked); markerErr != nil {
			return preflight.report, errors.Join(ErrCreditValuationMigrationBlocked, markerErr)
		}
		preflight.report.Status = CreditValuationMigrationFailed
		return preflight.report, ErrCreditValuationMigrationBlocked
	}

	var final CreditValuationMigrationReport
	applyErr := db.Transaction(func(tx *gorm.DB) error {
		priceReport, err := RunCreditValuationPlanPriceMigration(tx, CreditValuationPlanPriceMigrationRequest{Apply: true, BatchSize: request.BatchSize})
		if err != nil {
			return err
		}
		if !creditValuationPriceMigrationApplyComplete(priceReport) {
			return ErrCreditValuationMigrationVerifyFailed
		}
		if err := persistCreditValuationCurrencyFallbackTx(tx, currency); err != nil {
			return err
		}
		backfillRequest := CreditValuationHistoricalBackfillRequest{
			Apply: true, MigrationVersion: request.Version, BatchSize: request.BatchSize,
			ValuationCurrency: currency, FX: fx,
		}
		if _, err := RunCreditValuationHistoricalBackfill(tx, backfillRequest); err != nil {
			return err
		}
		if _, err := RunTimedSubscriptionValuationHistoricalBackfill(tx, backfillRequest); err != nil {
			return err
		}
		post, err := buildCreditValuationMigrationSnapshot(tx, request, currency, fx, nil, true)
		if err != nil {
			return err
		}
		if err := verifyCreditValuationMigrationSnapshot(post, marker, false); err != nil {
			return err
		}
		post.report.Mode = CreditValuationMigrationModeApply
		post.report.Status = CreditValuationMigrationReady
		post.report.ReadOnly = false
		post.report.Changed = true
		post.report.Ready = true
		if err := completeCreditValuationMigrationTx(tx, marker, post.report); err != nil {
			return err
		}
		final = post.report
		return nil
	})
	if applyErr != nil {
		markerErr := markCreditValuationMigrationFailed(db, marker, creditValuationMigrationFailureCode(applyErr))
		preflight.report.Status = CreditValuationMigrationFailed
		preflight.report.ReadOnly = false
		if markerErr != nil {
			return preflight.report, errors.Join(applyErr, markerErr)
		}
		return preflight.report, applyErr
	}
	return final, nil
}

func runCreditValuationMigrationRepair(db *gorm.DB, request CreditValuationMigrationRequest) (CreditValuationMigrationReport, error) {
	fx, currency, initialBlockers, err := migrationSnapshotInputs(db, CreditValuationMigration{}, false, true)
	if err != nil {
		return CreditValuationMigrationReport{}, err
	}
	preflight, err := buildCreditValuationMigrationSnapshot(db, request, currency, fx, initialBlockers, false)
	if err != nil {
		return preflight.report, err
	}
	stateFailures, err := verifyCreditValuationStates(db, currency, true)
	if err != nil {
		return preflight.report, err
	}
	if len(preflight.report.Blockers) > 0 || creditValuationPriceMigrationIncomplete(preflight.report.Price) || len(stateFailures) > 0 {
		preflight.report.Reasons = mergeCreditValuationMigrationReasons(preflight.report.Reasons, stateFailures)
		checksumErr := setCreditValuationMigrationChecksum(&preflight.report)
		if checksumErr != nil {
			return preflight.report, checksumErr
		}
		return preflight.report, ErrCreditValuationMigrationRepairInvalid
	}
	marker, err := claimCreditValuationMigration(db, request, currency, fx, preflight.report.Checksum, true)
	if err != nil {
		return preflight.report, err
	}

	var final CreditValuationMigrationReport
	repairErr := db.Transaction(func(tx *gorm.DB) error {
		backfillRequest := CreditValuationHistoricalBackfillRequest{
			Apply: true, RepairMissingAsUnknown: true, MigrationVersion: request.Version,
			BatchSize: request.BatchSize, ValuationCurrency: currency, FX: fx,
		}
		repairReport, err := RunCreditValuationHistoricalBackfill(tx, backfillRequest)
		if err != nil {
			return err
		}
		post, err := buildCreditValuationMigrationSnapshot(tx, request, currency, fx, nil, true)
		if err != nil {
			return err
		}
		failures, err := verifyCreditValuationStates(tx, currency, false)
		if err != nil {
			return err
		}
		if len(post.report.Blockers) > 0 || len(failures) > 0 {
			return ErrCreditValuationMigrationRepairInvalid
		}
		post.report.Mode = CreditValuationMigrationModeRepairMissingAsUnknown
		post.report.Status = CreditValuationMigrationFailed
		post.report.ReadOnly = false
		post.report.Changed = repairReport.RowsTotal > repairReport.RowsSkippedExisting
		post.report.Ready = false
		if err := persistCreditValuationMigrationNonReadyTx(tx, marker, post.report, creditValuationMigrationFailureRepairRequiresApply); err != nil {
			return err
		}
		final = post.report
		return nil
	})
	if repairErr != nil {
		markerErr := markCreditValuationMigrationFailed(db, marker, creditValuationMigrationFailureCode(repairErr))
		preflight.report.Status = CreditValuationMigrationFailed
		preflight.report.ReadOnly = false
		if markerErr != nil {
			return preflight.report, errors.Join(repairErr, markerErr)
		}
		return preflight.report, repairErr
	}
	return final, nil
}

func runCreditValuationMigrationSuspend(db *gorm.DB, request CreditValuationMigrationRequest) (CreditValuationMigrationReport, error) {
	var marker CreditValuationMigration
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("version DESC").First(&marker).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCreditValuationMigrationSuspendInvalid
			}
			return err
		}
		if marker.Version != request.Version || marker.Status != CreditValuationMigrationReady {
			return ErrCreditValuationMigrationSuspendInvalid
		}
		now := getDBTimestampTx(tx)
		result := tx.Model(&CreditValuationMigration{}).
			Where("version = ? AND status = ?", marker.Version, CreditValuationMigrationReady).
			Updates(map[string]any{
				"status": CreditValuationMigrationSuspended, "suspended_at": now,
				"suspended_reason": strings.TrimSpace(request.Reason),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCreditValuationMigrationCASConflict
		}
		marker.Status = CreditValuationMigrationSuspended
		return nil
	})
	if err != nil {
		return CreditValuationMigrationReport{}, err
	}
	return creditValuationMigrationReportFromMarker(marker, request.Mode, false, true), nil
}

func migrationSnapshotInputs(db *gorm.DB, marker CreditValuationMigration, markerFound bool, captured bool) (CreditValuationMigrationFXSnapshot, string, []CreditValuationMigrationBlocker, error) {
	blockers := make([]CreditValuationMigrationBlocker, 0)
	empty, err := creditValuationMigrationBusinessDatabaseEmpty(db)
	if err != nil {
		return CreditValuationMigrationFXSnapshot{}, "", nil, err
	}
	if empty {
		fx := CreditValuationMigrationFXSnapshot{}
		currency := strings.TrimSpace(marker.ValuationCurrency)
		if markerFound {
			fx = creditValuationMigrationFXFromMarker(marker)
			currency = fx.ValuationCurrency
		}
		if normalized, normalizeErr := NormalizeCreditValuationCurrency(currency); normalizeErr == nil {
			currency = normalized
		}
		return fx, currency, blockers, nil
	}
	currency, currencyBlockers, err := creditValuationMigrationCurrency(db, empty)
	if err != nil {
		return CreditValuationMigrationFXSnapshot{}, "", nil, err
	}
	blockers = append(blockers, currencyBlockers...)
	if markerFound && marker.FxRateNumerator > 0 && marker.FxRateDenominator > 0 {
		if marker.ValuationCurrency != "" {
			currency = marker.ValuationCurrency
		}
		if normalized, normalizeErr := NormalizeCreditValuationCurrency(currency); normalizeErr == nil {
			currency = normalized
		}
		return creditValuationMigrationFXFromMarker(marker), currency, blockers, nil
	}
	captureTime := int64(0)
	if captured {
		captureTime = getDBTimestampTx(db)
	}
	fx, fxBlockers, err := creditValuationMigrationFXFromOption(db, captureTime, currency)
	if err != nil {
		return CreditValuationMigrationFXSnapshot{}, "", nil, err
	}
	blockers = append(blockers, fxBlockers...)
	sortCreditValuationMigrationBlockers(blockers)
	return fx, currency, blockers, nil
}

func creditValuationMigrationFXFromOption(db *gorm.DB, capturedAt int64, valuationCurrency string) (CreditValuationMigrationFXSnapshot, []CreditValuationMigrationBlocker, error) {
	normalizedCurrency, err := NormalizeCreditValuationCurrency(valuationCurrency)
	if err != nil {
		return CreditValuationMigrationFXSnapshot{}, []CreditValuationMigrationBlocker{{Code: creditValuationMigrationBlockerValuationCurrency, Count: 1}}, nil
	}
	parseCapturedAt := capturedAt
	if parseCapturedAt <= 0 {
		parseCapturedAt = 1
	}
	if normalizedCurrency == "CNY" {
		return CreditValuationMigrationFXSnapshot{
			SourceCurrency: "CNY", ValuationCurrency: "CNY",
			Numerator: 1, Denominator: 1, CapturedAt: parseCapturedAt,
		}, nil, nil
	}
	if !db.Migrator().HasTable(&Option{}) {
		return CreditValuationMigrationFXSnapshot{}, []CreditValuationMigrationBlocker{{Code: creditValuationMigrationBlockerFXMissing, Count: 1}}, nil
	}
	var option Option
	query := db.Select(commonKeyCol, "value").Where(commonKeyCol+" = ?", "USDExchangeRate").Limit(1).Find(&option)
	if query.Error != nil {
		return CreditValuationMigrationFXSnapshot{}, nil, query.Error
	}
	if query.RowsAffected != 1 || strings.TrimSpace(option.Value) == "" {
		return CreditValuationMigrationFXSnapshot{}, []CreditValuationMigrationBlocker{{Code: creditValuationMigrationBlockerFXMissing, Count: 1}}, nil
	}
	sourceCurrency, targetCurrency := creditValuationMigrationFXCurrencies(normalizedCurrency)
	parsed, err := ParseCreditFXRateSnapshot(CreditFXRateSnapshotInput{
		SourceCurrency: sourceCurrency, ValuationCurrency: targetCurrency,
		Direction: creditFXDirection(sourceCurrency, targetCurrency), RateText: &option.Value,
		CapturedAt: parseCapturedAt,
	})
	if err != nil || parsed.Numerator <= 0 || parsed.Denominator <= 0 {
		return CreditValuationMigrationFXSnapshot{}, []CreditValuationMigrationBlocker{{Code: creditValuationMigrationBlockerFXInvalid, Count: 1}}, nil
	}
	return CreditValuationMigrationFXSnapshot{
		SourceCurrency: parsed.SourceCurrency, ValuationCurrency: parsed.ValuationCurrency,
		Numerator: parsed.Numerator, Denominator: parsed.Denominator, CapturedAt: parseCapturedAt,
	}, nil, nil
}

func creditValuationMigrationFXFromMarker(marker CreditValuationMigration) CreditValuationMigrationFXSnapshot {
	valuationCurrency := strings.TrimSpace(marker.ValuationCurrency)
	if normalized, err := NormalizeCreditValuationCurrency(valuationCurrency); err == nil {
		valuationCurrency = normalized
	}
	if valuationCurrency == "CNY" {
		return CreditValuationMigrationFXSnapshot{
			SourceCurrency: "CNY", ValuationCurrency: "CNY",
			Numerator: 1, Denominator: 1,
			CapturedAt: marker.FxCapturedAt,
		}
	}
	if marker.FxCapturedAt <= 0 {
		return CreditValuationMigrationFXSnapshot{
			ValuationCurrency: valuationCurrency, Numerator: marker.FxRateNumerator,
			Denominator: marker.FxRateDenominator, CapturedAt: marker.FxCapturedAt,
		}
	}
	sourceCurrency, targetCurrency := creditValuationMigrationFXCurrencies(valuationCurrency)
	return CreditValuationMigrationFXSnapshot{
		SourceCurrency: sourceCurrency, ValuationCurrency: targetCurrency,
		Numerator: marker.FxRateNumerator, Denominator: marker.FxRateDenominator,
		CapturedAt: marker.FxCapturedAt,
	}
}

func creditValuationMigrationCurrency(db *gorm.DB, empty bool) (string, []CreditValuationMigrationBlocker, error) {
	if !db.Migrator().HasTable(&SubscriptionPlan{}) {
		if empty {
			return "", nil, nil
		}
		return "", []CreditValuationMigrationBlocker{{Code: creditValuationMigrationBlockerCreditPlanMissing, Count: 1}}, nil
	}
	var plans []SubscriptionPlan
	if err := db.Select("id", "currency", "valuation_currency").Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Order("id ASC").Find(&plans).Error; err != nil {
		return "", nil, err
	}
	if len(plans) == 0 {
		if empty {
			return "", nil, nil
		}
		return "", []CreditValuationMigrationBlocker{{Code: creditValuationMigrationBlockerCreditPlanMissing, Count: 1}}, nil
	}
	if len(plans) != 1 {
		return "", []CreditValuationMigrationBlocker{{Code: creditValuationMigrationBlockerCreditPlanAmbiguous, Count: int64(len(plans))}}, nil
	}
	currency := strings.TrimSpace(plans[0].Currency)
	if plans[0].ValuationCurrency != nil && strings.TrimSpace(*plans[0].ValuationCurrency) != "" {
		currency = strings.TrimSpace(*plans[0].ValuationCurrency)
	}
	currency, err := NormalizeCreditValuationCurrency(currency)
	if err != nil {
		return "", []CreditValuationMigrationBlocker{{Code: creditValuationMigrationBlockerValuationCurrency, Count: 1}}, nil
	}
	return currency, nil, nil
}

func persistCreditValuationCurrencyFallbackTx(tx *gorm.DB, currency string) error {
	if tx == nil {
		return ErrDatabase
	}
	currency, err := NormalizeCreditValuationCurrency(currency)
	if err != nil {
		return err
	}
	result := tx.Model(&SubscriptionPlan{}).
		Where("entitlement_type = ? AND (valuation_currency IS NULL OR TRIM(valuation_currency) = '')", SubscriptionEntitlementCreditBalance).
		Update("valuation_currency", currency)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 1 {
		return ErrCreditValuationMigrationConflict
	}
	return nil
}

func creditValuationMigrationFXCurrencies(valuationCurrency string) (string, string) {
	if strings.EqualFold(strings.TrimSpace(valuationCurrency), "USD") {
		return "CNY", "USD"
	}
	return "USD", "CNY"
}

func buildCreditValuationMigrationSnapshot(db *gorm.DB, request CreditValuationMigrationRequest, currency string, fx CreditValuationMigrationFXSnapshot, initialBlockers []CreditValuationMigrationBlocker, includeVerification bool) (creditValuationMigrationSnapshot, error) {
	report := CreditValuationMigrationReport{
		Version: request.Version, Mode: request.Mode, Status: CreditValuationMigrationPending,
		ValuationCurrency: currency, FX: fx, ReadOnly: request.Mode == CreditValuationMigrationModeDryRun || request.Mode == CreditValuationMigrationModeVerify,
		Reasons: make([]CreditValuationMigrationReasonCount, 0), Blockers: make([]CreditValuationMigrationBlocker, 0),
		Batches: make([]CreditValuationMigrationBatchBoundary, 0),
	}
	price, err := RunCreditValuationPlanPriceMigration(db, CreditValuationPlanPriceMigrationRequest{Apply: false, BatchSize: request.BatchSize})
	if err != nil {
		return creditValuationMigrationSnapshot{report: report}, err
	}
	report.Price = price
	blockers, err := creditValuationMigrationBlockers(db)
	if err != nil {
		return creditValuationMigrationSnapshot{report: report}, err
	}
	report.Blockers = append(report.Blockers, initialBlockers...)
	report.Blockers = append(report.Blockers, blockers...)

	inputsValid := currency != "" && fx.Numerator > 0 && fx.Denominator > 0
	if inputsValid {
		backfillRequest := CreditValuationHistoricalBackfillRequest{
			Apply: false, MigrationVersion: request.Version, BatchSize: request.BatchSize,
			ValuationCurrency: currency, FX: fx,
		}
		creditReport, creditErr := RunCreditValuationHistoricalBackfill(db, backfillRequest)
		if creditErr != nil {
			return creditValuationMigrationSnapshot{report: report}, creditErr
		}
		timedReport, timedErr := RunTimedSubscriptionValuationHistoricalBackfill(db, backfillRequest)
		if timedErr != nil {
			return creditValuationMigrationSnapshot{report: report}, timedErr
		}
		report.Credit = creditReport
		report.Timed = timedReport
	}

	sortCreditValuationPlanPriceReport(&report.Price)
	sortCreditValuationHistoricalReport(&report.Credit)
	sortTimedSubscriptionValuationHistoricalReport(&report.Timed)
	report.Reasons = mergeCreditValuationMigrationReasons(report.Credit.Reasons, report.Timed.Reasons)
	report.Batches = append(report.Batches, report.Price.Batches...)
	report.Batches = append(report.Batches, report.Credit.Batches...)
	report.Batches = append(report.Batches, report.Timed.Batches...)
	sortCreditValuationMigrationBatches(report.Batches)
	report.Blockers = mergeCreditValuationMigrationBlockers(report.Blockers)

	failures := make([]CreditValuationMigrationReasonCount, 0)
	if includeVerification && inputsValid {
		stateFailures, verifyErr := verifyCreditValuationStates(db, currency, false)
		if verifyErr != nil {
			return creditValuationMigrationSnapshot{report: report}, verifyErr
		}
		sourceFailures, verifyErr := verifyCreditValuationMigrationSources(db)
		if verifyErr != nil {
			return creditValuationMigrationSnapshot{report: report}, verifyErr
		}
		failures = mergeCreditValuationMigrationReasons(stateFailures, sourceFailures)
		report.Reasons = mergeCreditValuationMigrationReasons(report.Reasons, failures)
	}
	if err := setCreditValuationMigrationChecksum(&report); err != nil {
		return creditValuationMigrationSnapshot{report: report}, err
	}
	return creditValuationMigrationSnapshot{report: report, verificationFailures: failures}, nil
}

func creditValuationMigrationBlockers(db *gorm.DB) ([]CreditValuationMigrationBlocker, error) {
	blockers := make([]CreditValuationMigrationBlocker, 0, 3)
	if db.Migrator().HasTable(&SubscriptionPreConsumeRecord{}) && db.Migrator().HasTable(&Task{}) {
		activeTaskReference := db.Model(&Task{}).
			Select("1").
			Where("tasks.status IN ?", []TaskStatus{TaskStatusQueued, TaskStatusSubmitted, TaskStatusInProgress}).
			Where("tasks.subscription_request_id = subscription_pre_consume_records.request_id")
		var count int64
		if err := db.Model(&SubscriptionPreConsumeRecord{}).
			Where("status IS NULL OR status NOT IN ?", []string{"settled", "refunded"}).
			Where("EXISTS (?)", activeTaskReference).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			blockers = append(blockers, CreditValuationMigrationBlocker{Code: creditValuationMigrationBlockerPreConsume, Count: count})
		}
	}
	if db.Migrator().HasTable(&Task{}) {
		var tasks []Task
		if err := db.Select("id", "status", "private_data", "subscription_request_id").
			Where("status IN ?", []TaskStatus{TaskStatusQueued, TaskStatusSubmitted, TaskStatusInProgress}).
			Order("id ASC").Find(&tasks).Error; err != nil {
			return nil, err
		}
		var count int64
		for index := range tasks {
			task := &tasks[index]
			isSubscriptionTask := strings.TrimSpace(task.PrivateData.BillingSource) == "subscription" || task.PrivateData.SubscriptionId > 0
			projected := task.SubscriptionRequestId != nil && strings.TrimSpace(*task.SubscriptionRequestId) != ""
			if isSubscriptionTask && !projected {
				count++
			}
		}
		if count > 0 {
			blockers = append(blockers, CreditValuationMigrationBlocker{Code: creditValuationMigrationBlockerAsyncTaskIdentity, Count: count})
		}
	}
	if db.Migrator().HasTable(&Option{}) {
		var count int64
		if err := db.Model(&Option{}).
			Where(commonKeyCol+" IN ? AND value IS NOT NULL AND value <> ''", []string{creditValuationMigrationLegacyWriterSessionOption, creditValuationMigrationWriterSessionOption}).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			blockers = append(blockers, CreditValuationMigrationBlocker{Code: creditValuationMigrationBlockerLegacyWriterSession, Count: count})
		}
	}
	sortCreditValuationMigrationBlockers(blockers)
	return blockers, nil
}

func verifyCreditValuationStates(db *gorm.DB, currency string, allowMissing bool) ([]CreditValuationMigrationReasonCount, error) {
	if !db.Migrator().HasTable(&UserSubscription{}) || !db.Migrator().HasTable(&CreditValuationState{}) {
		if allowMissing {
			return nil, nil
		}
		return []CreditValuationMigrationReasonCount{{Reason: "credit_valuation_state_table_missing", Count: 1}}, nil
	}
	var subscriptions []UserSubscription
	if err := db.Select("id", "user_id", "entitlement_type", "token_limit", "token_used").
		Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Order("id ASC").Find(&subscriptions).Error; err != nil {
		return nil, err
	}
	var states []CreditValuationState
	if err := db.Order("user_subscription_id ASC").Find(&states).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int64)
	stateIndex := 0
	for _, subscription := range subscriptions {
		for stateIndex < len(states) && states[stateIndex].UserSubscriptionId < subscription.Id {
			counts["credit_valuation_state_orphan"]++
			stateIndex++
		}
		if stateIndex >= len(states) || states[stateIndex].UserSubscriptionId != subscription.Id {
			if !allowMissing {
				counts["credit_valuation_state_missing"]++
			}
			continue
		}
		state := states[stateIndex]
		stateIndex++
		available, ok := checkedSubInt64(subscription.TokenLimit, subscription.TokenUsed)
		if !ok {
			counts["credit_valuation_quantity_overflow"]++
			continue
		}
		available = maxInt64(available, 0)
		if state.UserId != subscription.UserId || state.AvailableCredit != available {
			counts["credit_valuation_state_quantity_mismatch"]++
		}
		if state.Currency != currency {
			counts["credit_valuation_state_currency_mismatch"]++
		}
		if subscription.TokenLimit < 0 || subscription.TokenUsed < 0 || state.ExactCostMicros < 0 || state.EstimatedCostMicros < 0 || state.UnknownCredit < 0 {
			counts["credit_valuation_state_negative"]++
		}
		if state.UnknownCredit > available {
			counts["credit_valuation_unknown_exceeds_available"]++
		}
		if state.StateVersion < 0 || state.MigrationVersion < 0 || state.RuleVersion != CreditValuationRuleVersion {
			counts["credit_valuation_state_version_invalid"]++
		}
		if state.MigrationVersion == 0 && state.StateVersion == 0 && available > 0 {
			counts["credit_valuation_forward_state_version_missing"]++
		}
	}
	for stateIndex < len(states) {
		counts["credit_valuation_state_orphan"]++
		stateIndex++
	}
	return sortedCreditValuationMigrationReasonCounts(counts), nil
}

func verifyCreditValuationMigrationSources(db *gorm.DB) ([]CreditValuationMigrationReasonCount, error) {
	counts := make(map[string]int64)
	if db.Migrator().HasTable(&CreditBalanceLedger{}) {
		var duplicateSources []struct{ Count int64 }
		if err := db.Model(&CreditBalanceLedger{}).Select("COUNT(*) AS count").
			Where("source_type <> '' AND source_key <> ''").Group("source_type, source_key").Having("COUNT(*) > 1").Scan(&duplicateSources).Error; err != nil {
			return nil, err
		}
		for _, duplicate := range duplicateSources {
			counts["credit_valuation_source_duplicate"] += duplicate.Count
		}
	}
	if db.Migrator().HasTable(&TimedSubscriptionValuationGrant{}) {
		var grants []TimedSubscriptionValuationGrant
		if err := db.Order("id ASC").Find(&grants).Error; err != nil {
			return nil, err
		}
		seenSources := make(map[string]struct{}, len(grants))
		for _, grant := range grants {
			identity := timedHistoricalGrantIdentity(grant.SourceType, grant.SourceKey)
			if _, exists := seenSources[identity]; exists {
				counts["timed_valuation_grant_duplicate"]++
			} else {
				seenSources[identity] = struct{}{}
			}
			if strings.TrimSpace(grant.SourceType) == "" || strings.TrimSpace(grant.SourceKey) == "" || strings.TrimSpace(grant.IdempotencyKey) == "" {
				counts["timed_valuation_grant_source_missing"]++
				continue
			}
			if !validTimedSubscriptionValuationGrant(grant) {
				counts["timed_valuation_grant_invalid"]++
			}
		}
	}
	return sortedCreditValuationMigrationReasonCounts(counts), nil
}

func validTimedSubscriptionValuationGrant(grant TimedSubscriptionValuationGrant) bool {
	if grant.UserSubscriptionId <= 0 || grant.UserId <= 0 || grant.PlanId <= 0 ||
		grant.EventStartTime <= 0 || grant.EventEndTime <= grant.EventStartTime ||
		grant.GrantCredit <= 0 || grant.SourcePriceMicros <= 0 || grant.ValuationAmountMicros <= 0 ||
		grant.RuleVersion != CreditValuationRuleVersion || grant.FxCapturedAt <= 0 ||
		grant.FxRateNumerator <= 0 || grant.FxRateDenominator <= 0 {
		return false
	}
	sourceCurrency, sourceErr := NormalizeCreditValuationCurrency(grant.SourceCurrency)
	valuationCurrency, valuationErr := NormalizeCreditValuationCurrency(grant.ValuationCurrency)
	if sourceErr != nil || valuationErr != nil {
		return false
	}
	if sourceCurrency == valuationCurrency && (grant.FxRateNumerator != 1 || grant.FxRateDenominator != 1) {
		return false
	}
	fx := CreditFXRateSnapshot{
		SourceCurrency: sourceCurrency, ValuationCurrency: valuationCurrency,
		Numerator: grant.FxRateNumerator, Denominator: grant.FxRateDenominator,
		CapturedAt: grant.FxCapturedAt, Direction: creditFXDirection(sourceCurrency, valuationCurrency),
	}
	converted, err := fx.ConvertMicros(grant.SourcePriceMicros)
	if err != nil || converted != grant.ValuationAmountMicros {
		return false
	}
	return grant.Confidence == TimedSubscriptionValuationConfidenceExact || grant.Confidence == CreditValuationConfidenceEstimated
}

func verifyCreditValuationMigrationSnapshot(snapshot creditValuationMigrationSnapshot, marker CreditValuationMigration, requireChecksum bool) error {
	if len(snapshot.report.Blockers) > 0 {
		return ErrCreditValuationMigrationBlocked
	}
	if creditValuationPriceMigrationIncomplete(snapshot.report.Price) || len(snapshot.verificationFailures) > 0 {
		return ErrCreditValuationMigrationVerifyFailed
	}
	if requireChecksum {
		if marker.Checksum == "" || marker.Checksum != snapshot.report.Checksum {
			return ErrCreditValuationMigrationChecksumMismatch
		}
	}
	return nil
}

func claimCreditValuationMigration(db *gorm.DB, request CreditValuationMigrationRequest, currency string, fx CreditValuationMigrationFXSnapshot, preflightChecksum string, requireHigher bool) (CreditValuationMigration, error) {
	if preflightChecksum == "" {
		return CreditValuationMigration{}, ErrCreditValuationMigrationChecksumMismatch
	}
	var claimed CreditValuationMigration
	err := db.Transaction(func(tx *gorm.DB) error {
		var highest CreditValuationMigration
		highestQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("version DESC").Limit(1).Find(&highest)
		if highestQuery.Error != nil {
			return highestQuery.Error
		}
		if requireHigher && highestQuery.RowsAffected == 1 && request.Version <= highest.Version {
			return ErrCreditValuationMigrationRepairInvalid
		}
		if !requireHigher && highestQuery.RowsAffected == 1 && highest.Version > request.Version {
			return ErrCreditValuationMigrationConflict
		}

		now, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		leaseExpiresAt, ok := checkedAddInt64(now, creditValuationMigrationRunLeaseSeconds)
		if !ok {
			return ErrCreditValuationOverflow
		}
		var marker CreditValuationMigration
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("version = ?", request.Version).Limit(1).Find(&marker)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected == 0 {
			marker = CreditValuationMigration{
				Version: request.Version, Status: CreditValuationMigrationPending, ValuationCurrency: currency,
				FxRateNumerator: fx.Numerator, FxRateDenominator: fx.Denominator, FxCapturedAt: fx.CapturedAt,
				PreflightChecksum: preflightChecksum, StartedAt: now,
			}
			if err := tx.Create(&marker).Error; err != nil {
				return err
			}
		}
		if marker.ValuationCurrency != "" && marker.ValuationCurrency != currency {
			return ErrCreditValuationMigrationConflict
		}
		if marker.FxRateNumerator != 0 && (marker.FxRateNumerator != fx.Numerator || marker.FxRateDenominator != fx.Denominator || marker.FxCapturedAt != fx.CapturedAt) {
			return ErrCreditValuationMigrationChecksumMismatch
		}
		switch marker.Status {
		case CreditValuationMigrationPending:
			if marker.PreflightChecksum != "" && marker.PreflightChecksum != preflightChecksum {
				return ErrCreditValuationMigrationChecksumMismatch
			}
		case CreditValuationMigrationFailed:
			if marker.PreflightChecksum != preflightChecksum || marker.FxRateNumerator != fx.Numerator || marker.FxRateDenominator != fx.Denominator || marker.FxCapturedAt != fx.CapturedAt {
				return ErrCreditValuationMigrationChecksumMismatch
			}
		case CreditValuationMigrationRunning:
			if marker.RunLeaseExpiresAt <= 0 || marker.RunLeaseExpiresAt > now {
				return ErrCreditValuationMigrationConflict
			}
			if marker.PreflightChecksum != preflightChecksum || marker.FxRateNumerator != fx.Numerator || marker.FxRateDenominator != fx.Denominator || marker.FxCapturedAt != fx.CapturedAt {
				return ErrCreditValuationMigrationChecksumMismatch
			}
		default:
			return ErrCreditValuationMigrationConflict
		}

		result := tx.Model(&CreditValuationMigration{}).
			Where("version = ? AND status = ? AND preflight_checksum = ? AND run_lease_expires_at = ?", request.Version, marker.Status, marker.PreflightChecksum, marker.RunLeaseExpiresAt).
			Updates(map[string]any{
				"status": CreditValuationMigrationRunning, "valuation_currency": currency,
				"fx_rate_numerator": fx.Numerator, "fx_rate_denominator": fx.Denominator,
				"fx_captured_at": fx.CapturedAt, "preflight_checksum": preflightChecksum,
				"run_lease_expires_at": leaseExpiresAt, "started_at": now, "completed_at": int64(0),
				"failure_reason": "", "suspended_at": int64(0), "suspended_reason": "",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCreditValuationMigrationCASConflict
		}
		marker.Status = CreditValuationMigrationRunning
		marker.ValuationCurrency = currency
		marker.FxRateNumerator = fx.Numerator
		marker.FxRateDenominator = fx.Denominator
		marker.FxCapturedAt = fx.CapturedAt
		marker.PreflightChecksum = preflightChecksum
		marker.RunLeaseExpiresAt = leaseExpiresAt
		marker.StartedAt = now
		claimed = marker
		return nil
	})
	return claimed, err
}

func completeCreditValuationMigrationTx(tx *gorm.DB, marker CreditValuationMigration, report CreditValuationMigrationReport) error {
	now, err := getDBTimestampStrictTx(tx)
	if err != nil {
		return err
	}
	updates := creditValuationMigrationReportUpdates(report, CreditValuationMigrationReady, now, "")
	updates["run_lease_expires_at"] = int64(0)
	result := tx.Model(&CreditValuationMigration{}).
		Where("version = ? AND status = ? AND preflight_checksum = ? AND run_lease_expires_at = ?", marker.Version, CreditValuationMigrationRunning, marker.PreflightChecksum, marker.RunLeaseExpiresAt).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCreditValuationMigrationCASConflict
	}
	return nil
}

func persistCreditValuationMigrationNonReadyTx(tx *gorm.DB, marker CreditValuationMigration, report CreditValuationMigrationReport, reason string) error {
	now, err := getDBTimestampStrictTx(tx)
	if err != nil {
		return err
	}
	updates := creditValuationMigrationReportUpdates(report, CreditValuationMigrationFailed, now, reason)
	updates["preflight_checksum"] = report.Checksum
	updates["run_lease_expires_at"] = int64(0)
	result := tx.Model(&CreditValuationMigration{}).
		Where("version = ? AND status = ? AND preflight_checksum = ? AND run_lease_expires_at = ?", marker.Version, CreditValuationMigrationRunning, marker.PreflightChecksum, marker.RunLeaseExpiresAt).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCreditValuationMigrationCASConflict
	}
	return nil
}

func creditValuationMigrationReportUpdates(report CreditValuationMigrationReport, status string, completedAt int64, failureReason string) map[string]any {
	return map[string]any{
		"status": status, "valuation_currency": report.ValuationCurrency,
		"fx_rate_numerator": report.FX.Numerator, "fx_rate_denominator": report.FX.Denominator,
		"fx_captured_at": report.FX.CapturedAt, "credit_rows_total": report.Credit.RowsTotal,
		"credit_rows_estimated": report.Credit.RowsEstimated, "credit_rows_unknown": report.Credit.RowsUnknown,
		"timed_rows_total": report.Timed.RowsTotal, "timed_rows_estimated": report.Timed.RowsEstimated,
		"timed_rows_unknown": report.Timed.RowsUnknown, "checksum": report.Checksum,
		"completed_at": completedAt, "failure_reason": failureReason,
	}
}

func markCreditValuationMigrationFailed(db *gorm.DB, marker CreditValuationMigration, reason string) error {
	now, err := getDBTimestampStrictTx(db)
	if err != nil {
		return err
	}
	result := db.Model(&CreditValuationMigration{}).
		Where("version = ? AND status = ? AND preflight_checksum = ? AND run_lease_expires_at = ?", marker.Version, CreditValuationMigrationRunning, marker.PreflightChecksum, marker.RunLeaseExpiresAt).
		Updates(map[string]any{
			"status": CreditValuationMigrationFailed, "completed_at": now,
			"failure_reason": reason, "run_lease_expires_at": int64(0),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCreditValuationMigrationCASConflict
	}
	return nil
}

func creditValuationMigrationFailureCode(err error) string {
	if errors.Is(err, ErrCreditValuationMigrationBlocked) {
		return creditValuationMigrationFailureBlocked
	}
	return creditValuationMigrationFailureExecution
}

func findCreditValuationMigration(db *gorm.DB, version int) (CreditValuationMigration, bool, error) {
	var marker CreditValuationMigration
	query := db.Where("version = ?", version).Limit(1).Find(&marker)
	return marker, query.RowsAffected == 1, query.Error
}
func findHighestCreditValuationMigration(db *gorm.DB) (CreditValuationMigration, bool, error) {
	var marker CreditValuationMigration
	query := db.Order("version DESC").Limit(1).Find(&marker)
	return marker, query.RowsAffected == 1, query.Error
}

func creditValuationMigrationPriceMigrationIncomplete(report CreditValuationPlanPriceMigrationReport) bool {
	return report.RowsInvalid > 0 || report.RowsTotal != report.RowsAlreadyExact
}

func creditValuationPriceMigrationIncomplete(report CreditValuationPlanPriceMigrationReport) bool {
	return creditValuationMigrationPriceMigrationIncomplete(report)
}

func creditValuationPriceMigrationApplyComplete(report CreditValuationPlanPriceMigrationReport) bool {
	return report.RowsInvalid == 0 && report.RowsTotal == report.RowsAlreadyExact+report.RowsBackfilled
}

func ensureCreditValuationMigration(db *gorm.DB) error {
	if db == nil {
		return ErrDatabase
	}
	if !db.Migrator().HasTable(&CreditValuationMigration{}) {
		return ErrCreditValuationMigrationNotReady
	}
	empty, err := creditValuationMigrationBusinessDatabaseEmpty(db)
	if err != nil || !empty {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var marker CreditValuationMigration
		query := tx.Where("version = ?", CreditValuationRuleVersion).Limit(1).Find(&marker)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected == 1 {
			return nil
		}
		currency, blockers, err := creditValuationMigrationCurrency(tx, true)
		if err != nil {
			return err
		}
		if len(blockers) > 0 {
			return ErrCreditValuationMigrationBlocked
		}
		if currency == "" {
			currency = "CNY"
		}
		now := getDBTimestampTx(tx)
		fx := CreditValuationMigrationFXSnapshot{
			SourceCurrency: currency, ValuationCurrency: currency,
			Numerator: 1, Denominator: 1, CapturedAt: now,
		}
		snapshot, err := buildCreditValuationMigrationSnapshot(tx, CreditValuationMigrationRequest{
			Mode: CreditValuationMigrationModeApply, Version: CreditValuationRuleVersion, BatchSize: creditValuationPlanPriceMigrationDefaultBatchSize,
		}, currency, fx, nil, true)
		if err != nil {
			return err
		}
		if err := verifyCreditValuationMigrationSnapshot(snapshot, CreditValuationMigration{}, false); err != nil {
			return err
		}
		report := snapshot.report
		report.Status = CreditValuationMigrationReady
		report.Ready = true
		return tx.Create(&CreditValuationMigration{
			Version: report.Version, Status: report.Status, ValuationCurrency: currency,
			FxRateNumerator: fx.Numerator, FxRateDenominator: fx.Denominator, FxCapturedAt: fx.CapturedAt,
			StartedAt: now, CompletedAt: now, Checksum: report.Checksum,
		}).Error
	})
}

func creditValuationMigrationBusinessDatabaseEmpty(db *gorm.DB) (bool, error) {
	models := []any{
		&SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &Redemption{},
		&CreditBalanceLedger{}, &TimedSubscriptionValuationGrant{}, &SubscriptionConversion{},
	}
	for _, model := range models {
		if !db.Migrator().HasTable(model) {
			continue
		}
		var count int64
		query := db.Model(model)
		switch model.(type) {
		case *SubscriptionPlan:
			query = query.Where("entitlement_type <> ? OR singleton_key IS NULL OR singleton_key <> ?", SubscriptionEntitlementCreditBalance, creditBalancePlanSingletonKey)
		case *SubscriptionOrder:
			query = query.Where("status = ?", common.TopUpStatusSuccess)
		case *Redemption:
			query = query.Where("redeemed_time > 0 OR fulfillment_subscription_id > 0")
		}
		if err := query.Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}
	return true, nil
}

func setCreditValuationMigrationChecksum(report *CreditValuationMigrationReport) error {
	if report == nil {
		return ErrCreditValuationMigrationVerifyFailed
	}
	payload := creditValuationMigrationChecksumPayload{
		Version: report.Version, ValuationCurrency: report.ValuationCurrency,
		FX: creditValuationMigrationChecksumFX{
			SourceCurrency: report.FX.SourceCurrency, ValuationCurrency: report.FX.ValuationCurrency,
			Numerator: report.FX.Numerator, Denominator: report.FX.Denominator,
		},
		Price: report.Price, Credit: report.Credit, Timed: report.Timed,
		Reasons: report.Reasons, Blockers: report.Blockers, Batches: report.Batches,
	}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	report.Checksum = hex.EncodeToString(digest[:])
	return nil
}

func sortCreditValuationPlanPriceReport(report *CreditValuationPlanPriceMigrationReport) {
	if report.Diagnostics == nil {
		report.Diagnostics = make([]CreditValuationPlanPriceDiagnostic, 0)
	}
	if report.Batches == nil {
		report.Batches = make([]CreditValuationMigrationBatchBoundary, 0)
	}
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		if report.Diagnostics[i].PlanID != report.Diagnostics[j].PlanID {
			return report.Diagnostics[i].PlanID < report.Diagnostics[j].PlanID
		}
		if report.Diagnostics[i].Reason != report.Diagnostics[j].Reason {
			return report.Diagnostics[i].Reason < report.Diagnostics[j].Reason
		}
		return report.Diagnostics[i].RawValue < report.Diagnostics[j].RawValue
	})
	sortCreditValuationMigrationBatches(report.Batches)
}

func sortCreditValuationHistoricalReport(report *CreditValuationHistoricalBackfillReport) {
	if report.Reasons == nil {
		report.Reasons = make([]CreditValuationMigrationReasonCount, 0)
	}
	if report.Diagnostics == nil {
		report.Diagnostics = make([]CreditValuationHistoricalBackfillDiagnostic, 0)
	}
	if report.Batches == nil {
		report.Batches = make([]CreditValuationMigrationBatchBoundary, 0)
	}
	sort.Slice(report.Reasons, func(i, j int) bool { return report.Reasons[i].Reason < report.Reasons[j].Reason })
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		if report.Diagnostics[i].UserSubscriptionID != report.Diagnostics[j].UserSubscriptionID {
			return report.Diagnostics[i].UserSubscriptionID < report.Diagnostics[j].UserSubscriptionID
		}
		return report.Diagnostics[i].Reason < report.Diagnostics[j].Reason
	})
	sortCreditValuationMigrationBatches(report.Batches)
}

func sortTimedSubscriptionValuationHistoricalReport(report *TimedSubscriptionValuationHistoricalBackfillReport) {
	if report.Reasons == nil {
		report.Reasons = make([]CreditValuationMigrationReasonCount, 0)
	}
	if report.Diagnostics == nil {
		report.Diagnostics = make([]TimedSubscriptionValuationHistoricalBackfillDiagnostic, 0)
	}
	if report.Batches == nil {
		report.Batches = make([]CreditValuationMigrationBatchBoundary, 0)
	}
	sort.Slice(report.Reasons, func(i, j int) bool { return report.Reasons[i].Reason < report.Reasons[j].Reason })
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		left, right := report.Diagnostics[i], report.Diagnostics[j]
		if left.SourceType != right.SourceType {
			return left.SourceType < right.SourceType
		}
		if left.SourceKey != right.SourceKey {
			return left.SourceKey < right.SourceKey
		}
		if left.SourceID != right.SourceID {
			return left.SourceID < right.SourceID
		}
		return left.Reason < right.Reason
	})
	sortCreditValuationMigrationBatches(report.Batches)
}

func mergeCreditValuationMigrationReasons(groups ...[]CreditValuationMigrationReasonCount) []CreditValuationMigrationReasonCount {
	counts := make(map[string]int64)
	for _, group := range groups {
		for _, reason := range group {
			if reason.Reason != "" && reason.Count > 0 {
				counts[reason.Reason] += reason.Count
			}
		}
	}
	return sortedCreditValuationMigrationReasonCounts(counts)
}

func sortedCreditValuationMigrationReasonCounts(counts map[string]int64) []CreditValuationMigrationReasonCount {
	reasons := make([]CreditValuationMigrationReasonCount, 0, len(counts))
	for reason, count := range counts {
		if reason != "" && count > 0 {
			reasons = append(reasons, CreditValuationMigrationReasonCount{Reason: reason, Count: count})
		}
	}
	sort.Slice(reasons, func(i, j int) bool { return reasons[i].Reason < reasons[j].Reason })
	return reasons
}

func mergeCreditValuationMigrationBlockers(blockers []CreditValuationMigrationBlocker) []CreditValuationMigrationBlocker {
	counts := make(map[string]int64)
	for _, blocker := range blockers {
		if blocker.Code != "" && blocker.Count > 0 {
			counts[blocker.Code] += blocker.Count
		}
	}
	merged := make([]CreditValuationMigrationBlocker, 0, len(counts))
	for code, count := range counts {
		merged = append(merged, CreditValuationMigrationBlocker{Code: code, Count: count})
	}
	sortCreditValuationMigrationBlockers(merged)
	return merged
}

func sortCreditValuationMigrationBlockers(blockers []CreditValuationMigrationBlocker) {
	sort.Slice(blockers, func(i, j int) bool { return blockers[i].Code < blockers[j].Code })
}

func sortCreditValuationMigrationBatches(batches []CreditValuationMigrationBatchBoundary) {
	sort.Slice(batches, func(i, j int) bool {
		if batches[i].Entity != batches[j].Entity {
			return batches[i].Entity < batches[j].Entity
		}
		if batches[i].StartID != batches[j].StartID {
			return batches[i].StartID < batches[j].StartID
		}
		if batches[i].EndID != batches[j].EndID {
			return batches[i].EndID < batches[j].EndID
		}
		return batches[i].Rows < batches[j].Rows
	})
}

func creditValuationMigrationReportFromMarker(marker CreditValuationMigration, mode CreditValuationMigrationMode, readOnly bool, changed bool) CreditValuationMigrationReport {
	return CreditValuationMigrationReport{
		Version: marker.Version, Mode: mode, Status: marker.Status, ValuationCurrency: marker.ValuationCurrency,
		FX:       creditValuationMigrationFXFromMarker(marker),
		Checksum: marker.Checksum, ReadOnly: readOnly, Changed: changed,
		Ready:   marker.Status == CreditValuationMigrationReady,
		Reasons: make([]CreditValuationMigrationReasonCount, 0), Blockers: make([]CreditValuationMigrationBlocker, 0),
		Batches: make([]CreditValuationMigrationBatchBoundary, 0),
	}
}

func creditValuationMigrationErrorWithSentinel(sentinel error, err error) error {
	if err == nil {
		return sentinel
	}
	return fmt.Errorf("%w: %v", sentinel, err)
}
