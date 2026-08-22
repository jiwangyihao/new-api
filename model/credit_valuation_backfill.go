package model

import (
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	creditValuationHistoricalDefaultBatchSize = 100
	creditValuationHistoricalStateEntity      = "credit_valuation_state"

	creditValuationHistoricalReasonNoSource        = "credit_source_missing"
	creditValuationHistoricalReasonSourceInvalid   = "credit_source_invalid"
	creditValuationHistoricalReasonSourceDuplicate = "credit_source_duplicate"
	creditValuationHistoricalReasonCurrency        = "credit_source_currency_invalid"
	creditValuationHistoricalReasonFX              = "credit_source_fx_invalid"
	creditValuationHistoricalReasonRepair          = "credit_state_repair_missing_as_unknown"
)

type CreditValuationHistoricalBackfillRequest struct {
	Apply                  bool
	RepairMissingAsUnknown bool
	RevalueHistorical      bool
	MigrationVersion       int
	BatchSize              int
	ValuationCurrency      string
	FX                     CreditValuationMigrationFXSnapshot
}

type CreditValuationHistoricalBackfillDiagnostic struct {
	UserSubscriptionID int    `json:"user_subscription_id"`
	Reason             string `json:"reason"`
	SourceType         string `json:"source_type,omitempty"`
	SourceKey          string `json:"source_key,omitempty"`
	SourceID           int    `json:"source_id,omitempty"`
	UnknownCredit      int64  `json:"unknown_credit,omitempty"`
}

type CreditValuationHistoricalBackfillReport struct {
	RowsTotal           int64                                         `json:"rows_total"`
	RowsEstimated       int64                                         `json:"rows_estimated"`
	RowsUnknown         int64                                         `json:"rows_unknown"`
	RowsSkippedExisting int64                                         `json:"rows_skipped_existing"`
	AmbiguousRows       int64                                         `json:"ambiguous_rows"`
	InvalidRows         int64                                         `json:"invalid_rows"`
	EstimatedCostMicros int64                                         `json:"estimated_cost_micros,string"`
	UnknownCredit       int64                                         `json:"unknown_credit"`
	Reasons             []CreditValuationMigrationReasonCount         `json:"reasons"`
	Diagnostics         []CreditValuationHistoricalBackfillDiagnostic `json:"diagnostics"`
	Batches             []CreditValuationMigrationBatchBoundary       `json:"batches"`
}

type historicalCreditSource struct {
	Identity      string
	SourceType    string
	SourceKey     string
	SourceID      int
	NetCredit     int64
	PriceMicros   int64
	PlanCredit    int64
	Currency      string
	FXNumerator   int64
	FXDenominator int64
}

type historicalCreditCandidate struct {
	Subscription UserSubscription
	Available    int64
	Estimated    int64
	Unknown      int64
	ValidSource  bool
	Reason       string
}

func newCreditValuationHistoricalBackfillReport() CreditValuationHistoricalBackfillReport {
	return CreditValuationHistoricalBackfillReport{
		Reasons:     make([]CreditValuationMigrationReasonCount, 0),
		Diagnostics: make([]CreditValuationHistoricalBackfillDiagnostic, 0),
		Batches:     make([]CreditValuationMigrationBatchBoundary, 0),
	}
}

func normalizeCreditValuationHistoricalBackfillRequest(request CreditValuationHistoricalBackfillRequest) (CreditValuationHistoricalBackfillRequest, int, error) {
	if request.MigrationVersion <= 0 {
		return CreditValuationHistoricalBackfillRequest{}, 0, ErrCreditValuationMigrationVersionRequired
	}
	batchSize := request.BatchSize
	if batchSize <= 0 {
		batchSize = creditValuationHistoricalDefaultBatchSize
	}
	currency := strings.TrimSpace(request.ValuationCurrency)
	if currency == "" {
		currency = strings.TrimSpace(request.FX.ValuationCurrency)
	}
	if currency == "" {
		return CreditValuationHistoricalBackfillRequest{}, 0, ErrCreditValuationMigrationFXUnavailable
	}
	var err error
	currency, err = NormalizeCreditValuationCurrency(currency)
	if err != nil {
		return CreditValuationHistoricalBackfillRequest{}, 0, ErrCreditValuationMigrationFXUnavailable
	}
	fx := request.FX
	fx.ValuationCurrency = strings.TrimSpace(fx.ValuationCurrency)
	if fx.ValuationCurrency == "" {
		fx.ValuationCurrency = currency
	}
	fx.SourceCurrency = strings.TrimSpace(fx.SourceCurrency)
	if fx.SourceCurrency == "" {
		fx.SourceCurrency = "USD"
	}
	fx.SourceCurrency, err = NormalizeCreditValuationCurrency(fx.SourceCurrency)
	if err != nil {
		return CreditValuationHistoricalBackfillRequest{}, 0, ErrCreditValuationMigrationFXUnavailable
	}
	fx.ValuationCurrency, err = NormalizeCreditValuationCurrency(fx.ValuationCurrency)
	if err != nil || (fx.ValuationCurrency != currency && fx.SourceCurrency != currency) || fx.Numerator <= 0 || fx.Denominator <= 0 || fx.CapturedAt <= 0 {
		return CreditValuationHistoricalBackfillRequest{}, 0, ErrCreditValuationMigrationFXUnavailable
	}
	request.ValuationCurrency = currency
	request.FX = fx
	request.BatchSize = batchSize
	if (request.RepairMissingAsUnknown || request.RevalueHistorical) && !request.Apply {
		return CreditValuationHistoricalBackfillRequest{}, 0, ErrCreditValuationMigrationRepairInvalid
	}
	if request.RepairMissingAsUnknown && request.RevalueHistorical {
		return CreditValuationHistoricalBackfillRequest{}, 0, ErrCreditValuationMigrationRepairInvalid
	}
	return request, batchSize, nil
}
func validateHistoricalCreditRepairVersion(db *gorm.DB, request CreditValuationHistoricalBackfillRequest) error {
	if !request.RepairMissingAsUnknown && !request.RevalueHistorical {
		return nil
	}
	var stateVersion int
	if db.Migrator().HasTable(&CreditValuationState{}) {
		if err := db.Model(&CreditValuationState{}).Select("COALESCE(MAX(migration_version), 0)").Scan(&stateVersion).Error; err != nil {
			return err
		}
	}
	var markerVersion int
	if db.Migrator().HasTable(&CreditValuationMigration{}) {
		if err := db.Model(&CreditValuationMigration{}).Where("version <> ?", request.MigrationVersion).Select("COALESCE(MAX(version), 0)").Scan(&markerVersion).Error; err != nil {
			return err
		}
	}
	if request.MigrationVersion <= stateVersion || request.MigrationVersion <= markerVersion {
		return ErrCreditValuationMigrationRepairInvalid
	}
	return nil
}

func historicalCreditFXForCurrency(request CreditValuationHistoricalBackfillRequest, sourceCurrency string) (CreditFXRateSnapshot, error) {
	source, err := NormalizeCreditValuationCurrency(sourceCurrency)
	if err != nil {
		return CreditFXRateSnapshot{}, ErrCreditValuationUnsupportedCurrency
	}
	target, err := NormalizeCreditValuationCurrency(request.ValuationCurrency)
	if err != nil {
		return CreditFXRateSnapshot{}, ErrCreditValuationUnsupportedCurrency
	}
	fxSource, err := NormalizeCreditValuationCurrency(request.FX.SourceCurrency)
	if err != nil {
		return CreditFXRateSnapshot{}, ErrCreditValuationUnsupportedCurrency
	}
	fxValuation, err := NormalizeCreditValuationCurrency(request.FX.ValuationCurrency)
	if err != nil || request.FX.CapturedAt <= 0 || request.FX.Numerator <= 0 || request.FX.Denominator <= 0 {
		return CreditFXRateSnapshot{}, ErrCreditValuationMigrationFXUnavailable
	}
	if source == target {
		return CreditFXRateSnapshot{
			SourceCurrency: source, ValuationCurrency: target,
			Numerator: 1, Denominator: 1, CapturedAt: request.FX.CapturedAt,
			Direction: CreditFXDirectionIdentity,
		}, nil
	}
	if source == fxSource && fxValuation == target {
		return CreditFXRateSnapshot{
			SourceCurrency: source, ValuationCurrency: target,
			Numerator: request.FX.Numerator, Denominator: request.FX.Denominator,
			CapturedAt: request.FX.CapturedAt,
			Direction:  creditFXDirection(source, target),
		}, nil
	}
	if source == fxValuation && fxSource == target {
		return CreditFXRateSnapshot{
			SourceCurrency: source, ValuationCurrency: target,
			Numerator: request.FX.Denominator, Denominator: request.FX.Numerator,
			CapturedAt: request.FX.CapturedAt,
			Direction:  creditFXDirection(source, target),
		}, nil
	}
	return CreditFXRateSnapshot{}, ErrCreditValuationUnsupportedCurrency
}

func RunCreditValuationHistoricalBackfill(db *gorm.DB, request CreditValuationHistoricalBackfillRequest) (CreditValuationHistoricalBackfillReport, error) {
	report := newCreditValuationHistoricalBackfillReport()
	if db == nil {
		return report, ErrDatabase
	}
	normalized, batchSize, err := normalizeCreditValuationHistoricalBackfillRequest(request)
	if err != nil {
		return report, err
	}
	if err := validateHistoricalCreditRepairVersion(db, normalized); err != nil {
		return report, err
	}
	if !db.Migrator().HasTable(&UserSubscription{}) {
		return report, nil
	}
	var subscriptions []UserSubscription
	if err := db.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).
		Order("id ASC").Find(&subscriptions).Error; err != nil {
		return report, err
	}
	report.RowsTotal = int64(len(subscriptions))
	if len(subscriptions) == 0 {
		return report, nil
	}

	states := make(map[int]CreditValuationState)
	if db.Migrator().HasTable(&CreditValuationState{}) {
		var rows []CreditValuationState
		if err := db.Order("user_subscription_id ASC").Find(&rows).Error; err != nil {
			return report, err
		}
		for _, state := range rows {
			states[state.UserSubscriptionId] = state
		}
	} else if normalized.Apply {
		return report, ErrCreditValuationStateMissing
	}

	ids := make([]int, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		ids = append(ids, subscription.Id)
	}
	ledgersBySubscription, err := historicalCreditLedgers(db, ids)
	if err != nil {
		return report, err
	}
	reasons := make(map[string]int64)
	candidates := make([]historicalCreditCandidate, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		state, exists := states[subscription.Id]
		revalue := exists && normalized.RevalueHistorical && historicalCreditStateCanRevalue(state, normalized.MigrationVersion)
		if exists && !revalue {
			report.RowsSkippedExisting++
			if state.MigrationVersion > 0 {
				if state.EstimatedCostMicros > 0 {
					report.RowsEstimated++
					updated, ok := checkedAddInt64(report.EstimatedCostMicros, state.EstimatedCostMicros)
					if !ok {
						return report, ErrCreditValuationOverflow
					}
					report.EstimatedCostMicros = updated
				}
				if state.UnknownCredit > 0 {
					report.RowsUnknown++
					updated, ok := checkedAddInt64(report.UnknownCredit, state.UnknownCredit)
					if !ok {
						return report, ErrCreditValuationOverflow
					}
					report.UnknownCredit = updated
				}
			}
			continue
		}
		available, ok := checkedSubInt64(subscription.TokenLimit, subscription.TokenUsed)
		if !ok || subscription.TokenLimit < 0 || subscription.TokenUsed < 0 {
			candidate := historicalCreditCandidate{Subscription: subscription, Available: 0, Unknown: 0, Reason: "credit_quantity_invalid"}
			candidates = append(candidates, candidate)
			report.InvalidRows++
			reasons[candidate.Reason]++
			report.Diagnostics = append(report.Diagnostics, CreditValuationHistoricalBackfillDiagnostic{UserSubscriptionID: subscription.Id, Reason: candidate.Reason})
			continue
		}
		available = maxInt64(available, 0)
		candidate := historicalCreditCandidate{Subscription: subscription, Available: available}
		if normalized.RepairMissingAsUnknown {
			candidate.Unknown = available
			candidate.Reason = creditValuationHistoricalReasonRepair
		} else {
			candidate.Estimated, candidate.Unknown, candidate.ValidSource, candidate.Reason = estimateHistoricalCredit(normalized, available, ledgersBySubscription[subscription.Id])
		}
		if candidate.ValidSource {
			report.RowsEstimated++
			var addOK bool
			report.EstimatedCostMicros, addOK = checkedAddInt64(report.EstimatedCostMicros, candidate.Estimated)
			if !addOK {
				return report, ErrCreditValuationOverflow
			}
		}
		if candidate.Unknown > 0 {
			report.RowsUnknown++
			var addOK bool
			report.UnknownCredit, addOK = checkedAddInt64(report.UnknownCredit, candidate.Unknown)
			if !addOK {
				return report, ErrCreditValuationOverflow
			}
		}
		if candidate.Reason != "" {
			reasons[candidate.Reason]++
			if candidate.Reason != creditValuationHistoricalReasonRepair {
				report.InvalidRows++
			}
			report.Diagnostics = append(report.Diagnostics, CreditValuationHistoricalBackfillDiagnostic{UserSubscriptionID: subscription.Id, Reason: candidate.Reason, UnknownCredit: candidate.Unknown})
		}
		candidates = append(candidates, candidate)
	}
	for start := 0; start < len(candidates); start += batchSize {
		end := start + batchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		report.Batches = append(report.Batches, CreditValuationMigrationBatchBoundary{
			Entity:  creditValuationHistoricalStateEntity,
			StartID: int64(candidates[start].Subscription.Id), EndID: int64(candidates[end-1].Subscription.Id), Rows: int64(end - start),
		})
	}
	report.Reasons = sortedCreditValuationMigrationReasonCounts(reasons)
	if !normalized.Apply {
		return report, nil
	}
	if err := writeHistoricalCreditStates(db, normalized, candidates); err != nil {
		return report, err
	}
	return report, nil
}

func historicalCreditLedgers(db *gorm.DB, subscriptionIDs []int) (map[int][]CreditBalanceLedger, error) {
	result := make(map[int][]CreditBalanceLedger)
	if len(subscriptionIDs) == 0 || !db.Migrator().HasTable(&CreditBalanceLedger{}) {
		return result, nil
	}
	index, err := loadHistoricalCreditSourceIndex(db)
	if err != nil {
		return nil, err
	}
	var rows []CreditBalanceLedger
	if err := db.Where("user_subscription_id IN ?", subscriptionIDs).
		Order("user_subscription_id ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		row = recoverHistoricalCreditLedger(row, index)
		result[row.UserSubscriptionId] = append(result[row.UserSubscriptionId], row)
	}
	return result, nil
}

func estimateHistoricalCredit(request CreditValuationHistoricalBackfillRequest, available int64, ledgers []CreditBalanceLedger) (int64, int64, bool, string) {
	if available == 0 && len(ledgers) == 0 {
		return 0, 0, true, ""
	}
	if len(ledgers) == 0 {
		return 0, available, false, creditValuationHistoricalReasonNoSource
	}
	seen := make(map[string]historicalCreditSource)
	for _, ledger := range ledgers {
		if ledger.GrossCredit < 0 {
			continue
		}
		if ledger.GrossCredit == 0 || ledger.NetCredit < 0 || ledger.NetCredit > ledger.GrossCredit || strings.TrimSpace(ledger.SourceType) == "" {
			return 0, available, false, creditValuationHistoricalReasonSourceInvalid
		}
		if ledger.NetCredit == 0 {
			continue
		}
		sourceKey := strings.TrimSpace(ledger.SourceKey)
		identity := ""
		if sourceKey != "" {
			identity = strings.TrimSpace(ledger.SourceType) + "|key|" + sourceKey
		} else if ledger.SourceId > 0 {
			identity = strings.TrimSpace(ledger.SourceType) + "|id|" + strconv.Itoa(ledger.SourceId)
		} else {
			return 0, available, false, creditValuationHistoricalReasonSourceInvalid
		}
		price := ledger.ValuationSourcePriceMicros
		if price <= 0 {
			price = ledger.SourcePriceMicros
		}
		basis := ledger.ValuationCreditBasis
		if basis <= 0 {
			basis = ledger.SourcePlanCredit
		}
		currency := strings.TrimSpace(ledger.FxSourceCurrency)
		if currency == "" {
			currency = historicalCreditLedgerCurrency(ledger.SourceSnapshot)
		}
		candidate := historicalCreditSource{Identity: identity, SourceType: ledger.SourceType, SourceKey: sourceKey, SourceID: ledger.SourceId, NetCredit: ledger.NetCredit, PriceMicros: price, PlanCredit: basis, Currency: currency, FXNumerator: ledger.FxRateNumerator, FXDenominator: ledger.FxRateDenominator}
		if previous, exists := seen[identity]; exists {
			if previous.NetCredit != candidate.NetCredit || previous.PriceMicros != candidate.PriceMicros || previous.PlanCredit != candidate.PlanCredit || previous.Currency != candidate.Currency || previous.FXNumerator != candidate.FXNumerator || previous.FXDenominator != candidate.FXDenominator {
				return 0, available, false, creditValuationHistoricalReasonSourceDuplicate
			}
			continue
		}
		seen[identity] = candidate
	}
	if len(seen) == 0 {
		return 0, available, false, creditValuationHistoricalReasonNoSource
	}
	sources := make([]historicalCreditSource, 0, len(seen))
	for _, source := range seen {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Identity < sources[j].Identity })
	var knownCredit int64
	var unknownSourceCredit int64
	var totalCost int64
	reason := ""
	markUnknown := func(source historicalCreditSource, sourceReason string) bool {
		var ok bool
		unknownSourceCredit, ok = checkedAddInt64(unknownSourceCredit, source.NetCredit)
		if reason == "" {
			reason = sourceReason
		}
		return ok
	}
	for _, source := range sources {
		currency, currencyErr := NormalizeCreditValuationCurrency(source.Currency)
		if currencyErr != nil || source.PriceMicros <= 0 || source.PlanCredit <= 0 {
			if !markUnknown(source, creditValuationHistoricalReasonCurrency) {
				return 0, available, false, creditValuationHistoricalReasonSourceInvalid
			}
			continue
		}
		cost, costErr := mulDivFloor(source.PriceMicros, source.NetCredit, source.PlanCredit)
		fx, fxErr := historicalCreditFXForCurrency(request, currency)
		if costErr != nil || fxErr != nil {
			if !markUnknown(source, creditValuationHistoricalReasonFX) {
				return 0, available, false, creditValuationHistoricalReasonSourceInvalid
			}
			continue
		}
		cost, costErr = fx.ConvertMicros(cost)
		if costErr != nil {
			if !markUnknown(source, creditValuationHistoricalReasonFX) {
				return 0, available, false, creditValuationHistoricalReasonSourceInvalid
			}
			continue
		}
		var ok bool
		knownCredit, ok = checkedAddInt64(knownCredit, source.NetCredit)
		if !ok {
			return 0, available, false, creditValuationHistoricalReasonSourceInvalid
		}
		totalCost, ok = checkedAddInt64(totalCost, cost)
		if !ok {
			return 0, available, false, creditValuationHistoricalReasonSourceInvalid
		}
	}
	totalCredit, ok := checkedAddInt64(knownCredit, unknownSourceCredit)
	if !ok || totalCredit <= 0 {
		return 0, available, false, creditValuationHistoricalReasonSourceInvalid
	}
	remaining := minInt64(available, totalCredit)
	estimated, err := mulDivFloor(totalCost, remaining, totalCredit)
	if err != nil {
		return 0, available, false, creditValuationHistoricalReasonSourceInvalid
	}
	unknown, err := mulDivFloor(unknownSourceCredit, remaining, totalCredit)
	if err != nil {
		return 0, available, false, creditValuationHistoricalReasonSourceInvalid
	}
	unknown, ok = checkedAddInt64(unknown, maxInt64(available-totalCredit, 0))
	if !ok {
		return 0, available, false, creditValuationHistoricalReasonSourceInvalid
	}
	return estimated, unknown, knownCredit > 0, reason
}

func historicalCreditLedgerCurrency(payload string) string {
	if strings.TrimSpace(payload) == "" {
		return ""
	}
	var snapshot SubscriptionEntitlementSnapshot
	if err := common.UnmarshalJsonStr(payload, &snapshot); err != nil {
		return ""
	}
	return snapshot.ListPriceCurrency
}

func writeHistoricalCreditStates(db *gorm.DB, request CreditValuationHistoricalBackfillRequest, candidates []historicalCreditCandidate) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, candidate := range candidates {
			query := tx.Where("id = ? AND entitlement_type = ?", candidate.Subscription.Id, SubscriptionEntitlementCreditBalance)
			if historicalCreditCanLock(tx) {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			var current UserSubscription
			if err := query.First(&current).Error; err != nil {
				return err
			}
			if current.UserId != candidate.Subscription.UserId || current.TokenLimit != candidate.Subscription.TokenLimit || current.TokenUsed != candidate.Subscription.TokenUsed {
				return ErrCreditValuationStateMismatch
			}
			var existing CreditValuationState
			existingQuery := tx.Where("user_subscription_id = ?", current.Id).Limit(1).Find(&existing)
			if existingQuery.Error != nil {
				return existingQuery.Error
			}
			now, err := getDBTimestampStrictTx(tx)
			if err != nil {
				return err
			}
			if existingQuery.RowsAffected == 1 {
				if !request.RevalueHistorical || !historicalCreditStateCanRevalue(existing, request.MigrationVersion) {
					return ErrCreditValuationMigrationConflict
				}
				newVersion, ok := checkedAddInt64(existing.StateVersion, 1)
				if !ok {
					return ErrCreditValuationOverflow
				}
				result := tx.Model(&CreditValuationState{}).
					Where("user_subscription_id = ? AND migration_version = ? AND state_version = ? AND available_credit = ? AND exact_cost_micros = ? AND estimated_cost_micros = ? AND unknown_credit = ?", existing.UserSubscriptionId, existing.MigrationVersion, existing.StateVersion, existing.AvailableCredit, int64(0), int64(0), existing.UnknownCredit).
					Updates(map[string]any{
						"available_credit": candidate.Available, "estimated_cost_micros": candidate.Estimated,
						"unknown_credit": candidate.Unknown, "migration_version": request.MigrationVersion,
						"state_version": newVersion, "last_mutation_type": "historical_revaluation", "updated_at": now,
					})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return ErrCreditValuationMigrationCASConflict
				}
				continue
			}
			mutation := "historical_backfill"
			if request.RepairMissingAsUnknown {
				mutation = "repair_missing_as_unknown"
			}
			state := CreditValuationState{UserSubscriptionId: current.Id, UserId: current.UserId, AvailableCredit: candidate.Available, ExactCostMicros: 0, EstimatedCostMicros: candidate.Estimated, UnknownCredit: candidate.Unknown, Currency: request.ValuationCurrency, RuleVersion: CreditValuationRuleVersion, MigrationVersion: request.MigrationVersion, StateVersion: 0, LastMutationType: mutation, CreatedAt: now, UpdatedAt: now}
			if err := validateCreditValuationState(&current, &state); err != nil {
				return err
			}
			if err := tx.Create(&state).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func historicalCreditStateCanRevalue(state CreditValuationState, migrationVersion int) bool {
	if state.MigrationVersion <= 0 || migrationVersion <= state.MigrationVersion || state.ExactCostMicros != 0 || state.EstimatedCostMicros != 0 {
		return false
	}
	return state.UnknownCredit == state.AvailableCredit
}

func historicalCreditCanLock(db *gorm.DB) bool {
	return db != nil && db.Dialector != nil && db.Dialector.Name() != "sqlite" && db.Dialector.Name() != "sqlite3"
}

func sortHistoricalCreditDiagnostics(diagnostics []CreditValuationHistoricalBackfillDiagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].UserSubscriptionID != diagnostics[j].UserSubscriptionID {
			return diagnostics[i].UserSubscriptionID < diagnostics[j].UserSubscriptionID
		}
		return diagnostics[i].Reason < diagnostics[j].Reason
	})
}

func (r *CreditValuationHistoricalBackfillReport) normalize() {
	if r == nil {
		return
	}
	if r.Reasons == nil {
		r.Reasons = make([]CreditValuationMigrationReasonCount, 0)
	}
	if r.Diagnostics == nil {
		r.Diagnostics = make([]CreditValuationHistoricalBackfillDiagnostic, 0)
	}
	if r.Batches == nil {
		r.Batches = make([]CreditValuationMigrationBatchBoundary, 0)
	}
	sortHistoricalCreditDiagnostics(r.Diagnostics)
	sortCreditValuationMigrationBatches(r.Batches)
}
