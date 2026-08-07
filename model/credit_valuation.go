package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const CreditValuationRuleVersion = 1

const (
	CreditValuationConfidenceExact     = "exact"
	CreditValuationConfidenceEstimated = "estimated"
	CreditValuationConfidenceUnknown   = "unknown"

	CreditValuationMutationGrant   = "grant"
	CreditValuationMutationConsume = "consume"
	CreditValuationMutationRestore = "restore"
)

type CreditValuationState struct {
	UserSubscriptionId  int    `json:"user_subscription_id" gorm:"primaryKey;autoIncrement:false"`
	UserId              int    `json:"user_id" gorm:"not null;uniqueIndex:uidx_credit_valuation_states_user_id"`
	AvailableCredit     int64  `json:"available_credit" gorm:"type:bigint;not null;default:0"`
	ExactCostMicros     int64  `json:"exact_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	EstimatedCostMicros int64  `json:"estimated_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	UnknownCredit       int64  `json:"unknown_credit" gorm:"type:bigint;not null;default:0"`
	Currency            string `json:"currency" gorm:"type:varchar(8);not null"`
	RuleVersion         int    `json:"rule_version" gorm:"not null"`
	MigrationVersion    int    `json:"migration_version" gorm:"not null;default:0"`
	StateVersion        int64  `json:"state_version" gorm:"type:bigint;not null"`
	LastMutationType    string `json:"last_mutation_type" gorm:"type:varchar(32);not null;default:''"`
	CreatedAt           int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt           int64  `json:"updated_at" gorm:"type:bigint;not null;index:idx_credit_valuation_states_updated_at"`
}

// CreditValuationSourceSnapshot contains authoritative source facts. Callers
// never choose confidence or calculate a cost amount.
type CreditValuationSourceSnapshot struct {
	SourcePriceMicros int64
	SourcePlanCredit  int64
	GrossCredit       int64
	SourceCurrency    string
	ValuationCurrency string
	RuleVersion       int
	FXRateSnapshot    *CreditFXRateSnapshot
}

type creditValuationIngress struct {
	grossCredit              int64
	grossCostMicros          int64
	currency                 string
	confidence               string
	ruleVersion              int
	fxSourceCurrency         string
	fxRateNumerator          int64
	fxRateDenominator        int64
	fxCapturedAt             int64
	unitValueNumeratorMicros int64
	unitValueDenominator     int64
}

type CreditValuationMutationResult struct {
	StateVersionAfter          int64
	NetAvailableCredit         int64
	GrossCostMicros            int64
	NetCostMicros              int64
	DebtOffset                 int64
	RemovedExactCostMicros     int64
	RemovedEstimatedCostMicros int64
	RemovedUnknownCredit       int64
}

func newForwardCreditValuationIngress(source CreditValuationSourceSnapshot) (creditValuationIngress, error) {
	if source.SourcePriceMicros <= 0 || source.SourcePlanCredit <= 0 || source.GrossCredit <= 0 || source.RuleVersion != CreditValuationRuleVersion {
		return creditValuationIngress{}, ErrCreditValuationSourceInvalid
	}
	sourceCurrency, err := NormalizeCreditValuationCurrency(source.SourceCurrency)
	if err != nil {
		return creditValuationIngress{}, err
	}
	valuationCurrency, err := NormalizeCreditValuationCurrency(source.ValuationCurrency)
	if err != nil {
		return creditValuationIngress{}, err
	}
	fxSnapshot := source.FXRateSnapshot
	if fxSnapshot == nil {
		if sourceCurrency != valuationCurrency {
			return creditValuationIngress{}, ErrCreditValuationUnsupportedCurrency
		}
		identity, identityErr := ParseCreditFXRateSnapshot(CreditFXRateSnapshotInput{
			SourceCurrency: sourceCurrency, ValuationCurrency: valuationCurrency,
			Direction: CreditFXDirectionIdentity, CapturedAt: 1,
		})
		if identityErr != nil {
			return creditValuationIngress{}, identityErr
		}
		fxSnapshot = &identity
	}
	if fxSnapshot.SourceCurrency != sourceCurrency || fxSnapshot.ValuationCurrency != valuationCurrency {
		return creditValuationIngress{}, ErrCreditValuationSourceInvalid
	}
	unitValueNumeratorMicros, unitValueDenominator, err := creditValuationUnitValueRatio(
		source.SourcePriceMicros,
		source.SourcePlanCredit,
		fxSnapshot.Numerator,
		fxSnapshot.Denominator,
	)
	if err != nil {
		return creditValuationIngress{}, err
	}
	sourceGrossCostMicros, err := mulDivFloor(source.SourcePriceMicros, source.GrossCredit, source.SourcePlanCredit)
	if err != nil {
		return creditValuationIngress{}, err
	}
	grossCostMicros, err := fxSnapshot.ConvertMicros(sourceGrossCostMicros)
	if err != nil {
		return creditValuationIngress{}, err
	}
	return creditValuationIngress{
		grossCredit:              source.GrossCredit,
		grossCostMicros:          grossCostMicros,
		currency:                 valuationCurrency,
		confidence:               CreditValuationConfidenceExact,
		ruleVersion:              source.RuleVersion,
		fxSourceCurrency:         fxSnapshot.SourceCurrency,
		fxRateNumerator:          fxSnapshot.Numerator,
		fxRateDenominator:        fxSnapshot.Denominator,
		fxCapturedAt:             fxSnapshot.CapturedAt,
		unitValueNumeratorMicros: unitValueNumeratorMicros,
		unitValueDenominator:     unitValueDenominator,
	}, nil
}

func creditValuationUnitValueRatio(sourcePriceMicros int64, sourcePlanCredit int64, fxNumerator int64, fxDenominator int64) (int64, int64, error) {
	if sourcePriceMicros <= 0 || sourcePlanCredit <= 0 || fxNumerator <= 0 || fxDenominator <= 0 {
		return 0, 0, ErrCreditValuationSourceInvalid
	}
	priceDivisor := greatestCommonDivisor(sourcePriceMicros, sourcePlanCredit)
	numerator := sourcePriceMicros / priceDivisor
	denominator := sourcePlanCredit / priceDivisor
	fxDivisor := greatestCommonDivisor(fxNumerator, denominator)
	fxNumerator /= fxDivisor
	denominator /= fxDivisor
	rateDivisor := greatestCommonDivisor(numerator, fxDenominator)
	numerator /= rateDivisor
	fxDenominator /= rateDivisor
	combinedNumerator, ok := checkedMulNonNegativeInt64(numerator, fxNumerator)
	if !ok {
		return 0, 0, ErrCreditValuationOverflow
	}
	combinedDenominator, ok := checkedMulNonNegativeInt64(denominator, fxDenominator)
	if !ok || combinedNumerator <= 0 || combinedDenominator <= 0 {
		return 0, 0, ErrCreditValuationOverflow
	}
	return combinedNumerator, combinedDenominator, nil
}

// CreditValuationRuntimeReadyTx only observes the existing marker. Marker
// creation and every lifecycle transition remain owned by Issue #27.
func CreditValuationRuntimeReadyTx(tx *gorm.DB) (bool, error) {
	if tx == nil {
		return false, ErrDatabase
	}
	if !tx.Migrator().HasTable(&CreditValuationMigration{}) {
		return false, nil
	}
	var marker CreditValuationMigration
	query := tx.Select("version", "status").Order("version desc").Limit(1).Find(&marker)
	if query.Error != nil {
		return false, query.Error
	}
	return query.RowsAffected == 1 && marker.Status == CreditValuationMigrationReady, nil
}

func initializeCreditValuationStateTx(tx *gorm.DB, lockedSub *UserSubscription, currency string) error {
	if tx == nil || lockedSub == nil || lockedSub.Id <= 0 || lockedSub.UserId <= 0 || lockedSub.EntitlementType != SubscriptionEntitlementCreditBalance || lockedSub.TokenLimit != 0 || lockedSub.TokenUsed != 0 {
		return ErrCreditValuationStateMismatch
	}
	currency, err := NormalizeCreditValuationCurrency(currency)
	if err != nil {
		return err
	}
	now := getDBTimestampTx(tx)
	state := CreditValuationState{
		UserSubscriptionId: lockedSub.Id,
		UserId:             lockedSub.UserId,
		Currency:           currency,
		RuleVersion:        CreditValuationRuleVersion,
		StateVersion:       0,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := tx.Create(&state).Error; err != nil {
		return err
	}
	return nil
}

func ApplyCreditValuationIngressTx(tx *gorm.DB, lockedSub *UserSubscription, ingress creditValuationIngress) (CreditValuationMutationResult, error) {
	if tx == nil || lockedSub == nil || lockedSub.Id <= 0 || lockedSub.EntitlementType != SubscriptionEntitlementCreditBalance {
		return CreditValuationMutationResult{}, ErrCreditValuationStateMismatch
	}
	if lockedSub.TokenLimit < 0 || lockedSub.TokenUsed < 0 || ingress.grossCredit <= 0 || ingress.grossCostMicros < 0 || ingress.confidence != CreditValuationConfidenceExact {
		return CreditValuationMutationResult{}, ErrCreditValuationStateMismatch
	}
	var state CreditValuationState
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_subscription_id = ?", lockedSub.Id).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CreditValuationMutationResult{}, ErrCreditValuationStateMissing
	}
	if err != nil {
		return CreditValuationMutationResult{}, err
	}
	if err := validateCreditValuationState(lockedSub, &state); err != nil {
		return CreditValuationMutationResult{}, err
	}
	if !strings.EqualFold(state.Currency, ingress.currency) || state.RuleVersion != ingress.ruleVersion {
		return CreditValuationMutationResult{}, ErrCreditValuationStateMismatch
	}

	settlementDebtBefore := maxInt64(lockedSub.TokenUsed-lockedSub.TokenLimit, 0)
	debtOffset := minInt64(ingress.grossCredit, settlementDebtBefore)
	netCredit := ingress.grossCredit - debtOffset
	netCostMicros, err := prorateFloor(ingress.grossCostMicros, netCredit, ingress.grossCredit)
	if err != nil {
		return CreditValuationMutationResult{}, err
	}
	newLimit, ok := checkedAddInt64(lockedSub.TokenLimit, ingress.grossCredit)
	if !ok {
		return CreditValuationMutationResult{}, ErrCreditValuationOverflow
	}
	newExactCost, ok := checkedAddInt64(state.ExactCostMicros, netCostMicros)
	if !ok {
		return CreditValuationMutationResult{}, ErrCreditValuationOverflow
	}
	newStateVersion, ok := checkedAddInt64(state.StateVersion, 1)
	if !ok {
		return CreditValuationMutationResult{}, ErrCreditValuationOverflow
	}
	now := getDBTimestampTx(tx)
	if err := tx.Model(&UserSubscription{}).Where("id = ?", lockedSub.Id).Updates(map[string]any{
		"token_limit": newLimit,
		"updated_at":  now,
	}).Error; err != nil {
		return CreditValuationMutationResult{}, err
	}
	lockedSub.TokenLimit = newLimit
	lockedSub.UpdatedAt = now
	state.AvailableCredit = maxInt64(newLimit-lockedSub.TokenUsed, 0)
	state.ExactCostMicros = newExactCost
	state.StateVersion = newStateVersion
	state.LastMutationType = CreditValuationMutationGrant
	state.UpdatedAt = now
	if err := validateCreditValuationState(lockedSub, &state); err != nil {
		return CreditValuationMutationResult{}, err
	}
	if err := tx.Model(&CreditValuationState{}).Where("user_subscription_id = ?", state.UserSubscriptionId).Updates(map[string]any{
		"available_credit":      state.AvailableCredit,
		"exact_cost_micros":     state.ExactCostMicros,
		"estimated_cost_micros": state.EstimatedCostMicros,
		"unknown_credit":        state.UnknownCredit,
		"state_version":         state.StateVersion,
		"last_mutation_type":    state.LastMutationType,
		"updated_at":            state.UpdatedAt,
	}).Error; err != nil {
		return CreditValuationMutationResult{}, err
	}
	return CreditValuationMutationResult{
		StateVersionAfter:  state.StateVersion,
		NetAvailableCredit: netCredit,
		GrossCostMicros:    ingress.grossCostMicros,
		NetCostMicros:      netCostMicros,
		DebtOffset:         debtOffset,
	}, nil
}

func validateCreditValuationState(sub *UserSubscription, state *CreditValuationState) error {
	if sub == nil || state == nil || state.UserSubscriptionId != sub.Id || state.UserId != sub.UserId || sub.EntitlementType != SubscriptionEntitlementCreditBalance {
		return ErrCreditValuationStateMismatch
	}
	if sub.TokenLimit < 0 || sub.TokenUsed < 0 || state.ExactCostMicros < 0 || state.EstimatedCostMicros < 0 || state.UnknownCredit < 0 || state.StateVersion < 0 {
		return ErrCreditValuationStateMismatch
	}
	expectedAvailable := maxInt64(sub.TokenLimit-sub.TokenUsed, 0)
	if state.AvailableCredit != expectedAvailable || state.UnknownCredit > expectedAvailable || state.RuleVersion != CreditValuationRuleVersion {
		return ErrCreditValuationStateMismatch
	}
	if _, err := NormalizeCreditValuationCurrency(state.Currency); err != nil {
		return ErrCreditValuationStateMismatch
	}
	return nil
}

func ApplyCreditValuationOutflowTx(tx *gorm.DB, lockedSub *UserSubscription, credit int64, mutationType string) (CreditValuationMutationResult, error) {
	mutationType = strings.TrimSpace(mutationType)
	if tx == nil || lockedSub == nil || lockedSub.Id <= 0 || lockedSub.EntitlementType != SubscriptionEntitlementCreditBalance || credit <= 0 || mutationType == "" {
		return CreditValuationMutationResult{}, ErrCreditValuationStateMismatch
	}
	var state CreditValuationState
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_subscription_id = ?", lockedSub.Id).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CreditValuationMutationResult{}, ErrCreditValuationStateMissing
	}
	if err != nil {
		return CreditValuationMutationResult{}, err
	}
	if err := validateCreditValuationState(lockedSub, &state); err != nil {
		return CreditValuationMutationResult{}, err
	}
	consumedAvailable := minInt64(credit, state.AvailableCredit)
	removedExact := int64(0)
	removedEstimated := int64(0)
	removedUnknown := int64(0)
	if consumedAvailable > 0 {
		var prorateErr error
		removedExact, prorateErr = prorateFloor(state.ExactCostMicros, consumedAvailable, state.AvailableCredit)
		if prorateErr != nil {
			return CreditValuationMutationResult{}, prorateErr
		}
		removedEstimated, prorateErr = prorateFloor(state.EstimatedCostMicros, consumedAvailable, state.AvailableCredit)
		if prorateErr != nil {
			return CreditValuationMutationResult{}, prorateErr
		}
		removedUnknown, prorateErr = prorateFloor(state.UnknownCredit, consumedAvailable, state.AvailableCredit)
		if prorateErr != nil {
			return CreditValuationMutationResult{}, prorateErr
		}
	}
	newTokenUsed, ok := checkedAddInt64(lockedSub.TokenUsed, credit)
	if !ok {
		return CreditValuationMutationResult{}, ErrCreditValuationOverflow
	}
	newStateVersion, ok := checkedAddInt64(state.StateVersion, 1)
	if !ok {
		return CreditValuationMutationResult{}, ErrCreditValuationOverflow
	}
	now := getDBTimestampTx(tx)
	if err := tx.Model(&UserSubscription{}).Where("id = ?", lockedSub.Id).Updates(map[string]any{
		"token_used": newTokenUsed,
		"updated_at": now,
	}).Error; err != nil {
		return CreditValuationMutationResult{}, err
	}
	lockedSub.TokenUsed = newTokenUsed
	lockedSub.UpdatedAt = now
	state.AvailableCredit = maxInt64(lockedSub.TokenLimit-newTokenUsed, 0)
	state.ExactCostMicros -= removedExact
	state.EstimatedCostMicros -= removedEstimated
	state.UnknownCredit -= removedUnknown
	state.StateVersion = newStateVersion
	state.LastMutationType = mutationType
	state.UpdatedAt = now
	if err := validateCreditValuationState(lockedSub, &state); err != nil {
		return CreditValuationMutationResult{}, err
	}
	if err := tx.Model(&CreditValuationState{}).Where("user_subscription_id = ?", state.UserSubscriptionId).Updates(map[string]any{
		"available_credit":      state.AvailableCredit,
		"exact_cost_micros":     state.ExactCostMicros,
		"estimated_cost_micros": state.EstimatedCostMicros,
		"unknown_credit":        state.UnknownCredit,
		"state_version":         state.StateVersion,
		"last_mutation_type":    state.LastMutationType,
		"updated_at":            state.UpdatedAt,
	}).Error; err != nil {
		return CreditValuationMutationResult{}, err
	}
	return CreditValuationMutationResult{
		StateVersionAfter:          state.StateVersion,
		NetAvailableCredit:         state.AvailableCredit,
		RemovedExactCostMicros:     removedExact,
		RemovedEstimatedCostMicros: removedEstimated,
		RemovedUnknownCredit:       removedUnknown,
	}, nil
}

func SettleCreditRequestTargetTx(tx *gorm.DB, route *SubscriptionPreConsumeRecord, targetCredit int64, final bool) error {
	if tx == nil || route == nil || route.Id <= 0 || strings.TrimSpace(route.RequestId) == "" || targetCredit < 0 {
		return ErrCreditValuationTargetConflict
	}
	expectedRoute := *route
	var record SubscriptionPreConsumeRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", expectedRoute.RequestId).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCreditValuationRequestNotFound
		}
		return err
	}
	if record.Id != expectedRoute.Id || record.RequestId != expectedRoute.RequestId || record.UserId != expectedRoute.UserId || record.UserSubscriptionId != expectedRoute.UserSubscriptionId || record.ValuationSubscriptionId != expectedRoute.ValuationSubscriptionId {
		return ErrCreditValuationMappingConflict
	}
	*route = record
	if route.ValuationSubscriptionId <= 0 || route.ValuationRuleVersion != CreditValuationRuleVersion {
		return ErrCreditValuationStateMismatch
	}
	if targetCredit < route.AppliedCredit {
		return restoreCreditRequestTargetTx(tx, route, targetCredit, final)
	}

	delta := targetCredit - route.AppliedCredit
	if delta == 0 {
		if record.FinalizedAt > 0 {
			if final && ((targetCredit == 0 && record.Status == "refunded") || (targetCredit > 0 && record.Status == "settled")) {
				return nil
			}
			return ErrCreditValuationFinalizedConflict
		}
		if !final {
			return nil
		}
		now := getDBTimestampTx(tx)
		status := "settled"
		if targetCredit == 0 {
			status = "refunded"
		}
		result := tx.Model(&SubscriptionPreConsumeRecord{}).
			Where("id = ? AND applied_credit = ? AND finalized_at = 0", record.Id, targetCredit).
			Updates(map[string]any{
				"status":       status,
				"finalized_at": now,
				"updated_at":   now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCreditValuationTargetConflict
		}
		*route = record
		route.Status = status
		route.FinalizedAt = now
		route.UpdatedAt = now
		return nil
	}
	if route.FinalizedAt > 0 {
		return ErrCreditValuationFinalizedConflict
	}

	var subscription UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", route.ValuationSubscriptionId).First(&subscription).Error; err != nil {
		return err
	}
	if subscription.EntitlementType != SubscriptionEntitlementCreditBalance {
		return ErrCreditValuationStateMismatch
	}
	availableBefore := maxInt64(subscription.TokenLimit-subscription.TokenUsed, 0)
	deductedAvailable := minInt64(delta, availableBefore)
	debtFormed := delta - deductedAvailable
	mutation, err := ApplyCreditValuationOutflowTx(tx, &subscription, delta, CreditValuationMutationConsume)
	if err != nil {
		return err
	}

	newDeductedAvailable, ok := checkedAddInt64(record.DeductedAvailableCredit, deductedAvailable)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newDebtFormed, ok := checkedAddInt64(record.DebtFormedCredit, debtFormed)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newDeductedExact, ok := checkedAddInt64(record.DeductedExactCostMicros, mutation.RemovedExactCostMicros)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newDeductedEstimated, ok := checkedAddInt64(record.DeductedEstimatedCostMicros, mutation.RemovedEstimatedCostMicros)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newDeductedUnknown, ok := checkedAddInt64(record.DeductedUnknownCredit, mutation.RemovedUnknownCredit)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newSettlementVersion, ok := checkedAddInt64(record.SettlementVersion, 1)
	if !ok {
		return ErrCreditValuationOverflow
	}
	status := "consumed"
	finalizedAt := int64(0)
	if final {
		status = "settled"
		finalizedAt = getDBTimestampTx(tx)
	}
	now := getDBTimestampTx(tx)
	result := tx.Model(&SubscriptionPreConsumeRecord{}).
		Where("id = ? AND applied_credit = ? AND finalized_at = 0", record.Id, record.AppliedCredit).
		Updates(map[string]any{
			"applied_credit":                 targetCredit,
			"deducted_available_credit":      newDeductedAvailable,
			"debt_formed_credit":             newDebtFormed,
			"deducted_exact_cost_micros":     newDeductedExact,
			"deducted_estimated_cost_micros": newDeductedEstimated,
			"deducted_unknown_credit":        newDeductedUnknown,
			"settlement_version":             newSettlementVersion,
			"status":                         status,
			"finalized_at":                   finalizedAt,
			"updated_at":                     now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCreditValuationTargetConflict
	}
	*route = record
	route.AppliedCredit = targetCredit
	route.DeductedAvailableCredit = newDeductedAvailable
	route.DebtFormedCredit = newDebtFormed
	route.DeductedExactCostMicros = newDeductedExact
	route.DeductedEstimatedCostMicros = newDeductedEstimated
	route.DeductedUnknownCredit = newDeductedUnknown
	route.SettlementVersion = newSettlementVersion
	route.Status = status
	route.FinalizedAt = finalizedAt
	route.UpdatedAt = now
	return nil
}

func restoreCreditRequestTargetTx(tx *gorm.DB, route *SubscriptionPreConsumeRecord, targetCredit int64, final bool) error {
	record := *route
	if targetCredit < 0 || targetCredit >= record.AppliedCredit {
		return ErrCreditValuationTargetConflict
	}
	if record.FinalizedAt > 0 && !final {
		return ErrCreditValuationFinalizedConflict
	}

	convertedVirtual := record.UserSubscriptionId != record.ValuationSubscriptionId
	if convertedVirtual {
		if record.PreConsumed <= 0 {
			return ErrCreditValuationMappingConflict
		}
		var source UserSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", record.UserSubscriptionId).First(&source).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCreditValuationMappingConflict
			}
			return err
		}
		if source.Status != SubscriptionStatusConverted || source.ConversionId <= 0 || source.ConvertedToSubscriptionId != record.ValuationSubscriptionId {
			return ErrCreditValuationMappingConflict
		}
		var conversion SubscriptionConversion
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND source_subscription_id = ? AND target_subscription_id = ?", source.ConversionId, source.Id, record.ValuationSubscriptionId).
			First(&conversion).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCreditValuationMappingConflict
			}
			return err
		}
		if conversion.ValuationRuleVersion != record.ValuationRuleVersion {
			return ErrCreditValuationMappingConflict
		}
	}

	var subscription UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", route.ValuationSubscriptionId).First(&subscription).Error; err != nil {
		return err
	}
	if subscription.EntitlementType != SubscriptionEntitlementCreditBalance {
		return ErrCreditValuationStateMismatch
	}

	var state CreditValuationState
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_subscription_id = ?", subscription.Id).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrCreditValuationStateMissing
	}
	if err != nil {
		return err
	}
	if err := validateCreditValuationState(&subscription, &state); err != nil {
		return err
	}

	refund := record.AppliedCredit - targetCredit
	targetUsedRefund := refund
	virtualRefund := int64(0)
	if convertedVirtual {
		targetUsedBefore := maxInt64(record.AppliedCredit-record.PreConsumed, 0)
		targetUsedAfter := maxInt64(targetCredit-record.PreConsumed, 0)
		targetUsedRefund = targetUsedBefore - targetUsedAfter
		virtualRefund = refund - targetUsedRefund
	}
	if targetUsedRefund > subscription.TokenUsed {
		return ErrCreditValuationStateMismatch
	}
	debtRefund := minInt64(refund, record.DebtFormedCredit)
	snapshotRefund := refund - debtRefund
	if snapshotRefund > record.DeductedAvailableCredit {
		return ErrCreditValuationStateMismatch
	}

	removedExact := int64(0)
	removedEstimated := int64(0)
	removedUnknown := int64(0)
	if snapshotRefund > 0 {
		removedExact, err = prorateFloor(record.DeductedExactCostMicros, snapshotRefund, record.DeductedAvailableCredit)
		if err != nil {
			return err
		}
		removedEstimated, err = prorateFloor(record.DeductedEstimatedCostMicros, snapshotRefund, record.DeductedAvailableCredit)
		if err != nil {
			return err
		}
		removedUnknown, err = prorateFloor(record.DeductedUnknownCredit, snapshotRefund, record.DeductedAvailableCredit)
		if err != nil {
			return err
		}
	}

	availableBefore := maxInt64(subscription.TokenLimit-subscription.TokenUsed, 0)
	newTokenLimit, ok := checkedAddInt64(subscription.TokenLimit, virtualRefund)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newTokenUsed := subscription.TokenUsed - targetUsedRefund
	availableAfter := maxInt64(newTokenLimit-newTokenUsed, 0)
	newlyAvailable := availableAfter - availableBefore
	restorableSnapshotCredit := minInt64(snapshotRefund, newlyAvailable)

	restoredExact := int64(0)
	restoredEstimated := int64(0)
	restoredSnapshotUnknown := int64(0)
	if snapshotRefund > 0 && restorableSnapshotCredit > 0 {
		restoredExact, err = prorateFloor(removedExact, restorableSnapshotCredit, snapshotRefund)
		if err != nil {
			return err
		}
		restoredEstimated, err = prorateFloor(removedEstimated, restorableSnapshotCredit, snapshotRefund)
		if err != nil {
			return err
		}
		restoredSnapshotUnknown, err = prorateFloor(removedUnknown, restorableSnapshotCredit, snapshotRefund)
		if err != nil {
			return err
		}
	}
	absorbedExact := removedExact - restoredExact
	absorbedEstimated := removedEstimated - restoredEstimated
	absorbedUnknown := removedUnknown - restoredSnapshotUnknown
	restoredDebtUnknown := newlyAvailable - restorableSnapshotCredit

	newExactCost, ok := checkedAddInt64(state.ExactCostMicros, restoredExact)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newEstimatedCost, ok := checkedAddInt64(state.EstimatedCostMicros, restoredEstimated)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newUnknownCredit, ok := checkedAddInt64(state.UnknownCredit, restoredSnapshotUnknown)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newUnknownCredit, ok = checkedAddInt64(newUnknownCredit, restoredDebtUnknown)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newStateVersion, ok := checkedAddInt64(state.StateVersion, 1)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newSettlementVersion, ok := checkedAddInt64(record.SettlementVersion, 1)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newAbsorbedExact, ok := checkedAddInt64(record.AbsorbedRestoreExactCostMicros, absorbedExact)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newAbsorbedEstimated, ok := checkedAddInt64(record.AbsorbedRestoreEstimatedCostMicros, absorbedEstimated)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newAbsorbedUnknown, ok := checkedAddInt64(record.AbsorbedRestoreUnknownCredit, absorbedUnknown)
	if !ok {
		return ErrCreditValuationOverflow
	}
	newRestoredUnknown, ok := checkedAddInt64(record.RestoredUnknownCredit, restoredDebtUnknown)
	if !ok {
		return ErrCreditValuationOverflow
	}

	now := getDBTimestampTx(tx)
	if err := tx.Model(&UserSubscription{}).Where("id = ?", subscription.Id).Updates(map[string]any{
		"token_limit": newTokenLimit,
		"token_used":  newTokenUsed,
		"updated_at":  now,
	}).Error; err != nil {
		return err
	}
	subscription.TokenLimit = newTokenLimit
	subscription.TokenUsed = newTokenUsed
	subscription.UpdatedAt = now
	state.AvailableCredit = availableAfter
	state.ExactCostMicros = newExactCost
	state.EstimatedCostMicros = newEstimatedCost
	state.UnknownCredit = newUnknownCredit
	state.StateVersion = newStateVersion
	state.LastMutationType = CreditValuationMutationRestore
	state.UpdatedAt = now
	if err := validateCreditValuationState(&subscription, &state); err != nil {
		return err
	}
	if err := tx.Model(&CreditValuationState{}).Where("user_subscription_id = ?", state.UserSubscriptionId).Updates(map[string]any{
		"available_credit":      state.AvailableCredit,
		"exact_cost_micros":     state.ExactCostMicros,
		"estimated_cost_micros": state.EstimatedCostMicros,
		"unknown_credit":        state.UnknownCredit,
		"state_version":         state.StateVersion,
		"last_mutation_type":    state.LastMutationType,
		"updated_at":            state.UpdatedAt,
	}).Error; err != nil {
		return err
	}

	status := "consumed"
	finalizedAt := int64(0)
	if final {
		status = "settled"
		if targetCredit == 0 {
			status = "refunded"
		}
		finalizedAt = now
	}
	result := tx.Model(&SubscriptionPreConsumeRecord{}).
		Where("id = ? AND applied_credit = ?", record.Id, record.AppliedCredit).
		Updates(map[string]any{
			"applied_credit":                         targetCredit,
			"deducted_available_credit":              record.DeductedAvailableCredit - snapshotRefund,
			"debt_formed_credit":                     record.DebtFormedCredit - debtRefund,
			"deducted_exact_cost_micros":             record.DeductedExactCostMicros - removedExact,
			"deducted_estimated_cost_micros":         record.DeductedEstimatedCostMicros - removedEstimated,
			"deducted_unknown_credit":                record.DeductedUnknownCredit - removedUnknown,
			"absorbed_restore_exact_cost_micros":     newAbsorbedExact,
			"absorbed_restore_estimated_cost_micros": newAbsorbedEstimated,
			"absorbed_restore_unknown_credit":        newAbsorbedUnknown,
			"restored_unknown_credit":                newRestoredUnknown,
			"settlement_version":                     newSettlementVersion,
			"status":                                 status,
			"finalized_at":                           finalizedAt,
			"updated_at":                             now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCreditValuationTargetConflict
	}
	record.AppliedCredit = targetCredit
	record.DeductedAvailableCredit -= snapshotRefund
	record.DebtFormedCredit -= debtRefund
	record.DeductedExactCostMicros -= removedExact
	record.DeductedEstimatedCostMicros -= removedEstimated
	record.DeductedUnknownCredit -= removedUnknown
	record.AbsorbedRestoreExactCostMicros = newAbsorbedExact
	record.AbsorbedRestoreEstimatedCostMicros = newAbsorbedEstimated
	record.AbsorbedRestoreUnknownCredit = newAbsorbedUnknown
	record.RestoredUnknownCredit = newRestoredUnknown
	record.SettlementVersion = newSettlementVersion
	record.Status = status
	record.FinalizedAt = finalizedAt
	record.UpdatedAt = now
	*route = record
	return nil
}

// SettleLegacyCreditTaskRequestTarget creates the request snapshot that old
// persisted tasks lack, then settles through the same request-aware module.
// The historical pre-consume cost is unknowable, so its active snapshot is
// classified as unknown; only later positive deltas consume the current pool.
func SettleLegacyCreditTaskRequestTarget(requestId string, originalSubscriptionId int, initialAppliedCredit int64, targetCredit int64, final bool) error {
	requestId = strings.TrimSpace(requestId)
	if requestId == "" || originalSubscriptionId <= 0 {
		return ErrCreditValuationTargetConflict
	}
	if initialAppliedCredit < 0 || targetCredit < 0 {
		return ErrCreditValuationNegativeInput
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var subscription UserSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", originalSubscriptionId).First(&subscription).Error; err != nil {
			return err
		}
		if subscription.EntitlementType != SubscriptionEntitlementCreditBalance || subscription.TokenUsed < initialAppliedCredit {
			return ErrCreditValuationStateMismatch
		}

		var state CreditValuationState
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_subscription_id = ?", subscription.Id).First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCreditValuationStateMissing
		}
		if err != nil {
			return err
		}
		if err := validateCreditValuationState(&subscription, &state); err != nil {
			return err
		}
		var existingRoute SubscriptionPreConsumeRecord
		existingQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", requestId).Limit(1).Find(&existingRoute)
		if existingQuery.Error != nil {
			return existingQuery.Error
		}
		if existingQuery.RowsAffected > 0 {
			if existingRoute.UserId != subscription.UserId || existingRoute.UserSubscriptionId != originalSubscriptionId || existingRoute.ValuationSubscriptionId != originalSubscriptionId {
				return ErrCreditValuationMappingConflict
			}
			if existingRoute.PreConsumed != initialAppliedCredit || existingRoute.ValuationRuleVersion != CreditValuationRuleVersion {
				return ErrCreditValuationTargetConflict
			}
			return SettleCreditRequestTargetTx(tx, &existingRoute, targetCredit, final)
		}

		var activeRoutes []SubscriptionPreConsumeRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("valuation_subscription_id = ? AND applied_credit > 0", subscription.Id).Order("id asc").Find(&activeRoutes).Error; err != nil {
			return err
		}
		knownAppliedCredit := int64(0)
		for i := range activeRoutes {
			var ok bool
			knownAppliedCredit, ok = checkedAddInt64(knownAppliedCredit, activeRoutes[i].AppliedCredit)
			if !ok {
				return ErrCreditValuationOverflow
			}
		}
		if knownAppliedCredit > subscription.TokenUsed || subscription.TokenUsed-knownAppliedCredit < initialAppliedCredit {
			return ErrCreditValuationStateMismatch
		}

		now := getDBTimestampTx(tx)
		candidate := SubscriptionPreConsumeRecord{
			RequestId:               requestId,
			UserId:                  subscription.UserId,
			UserSubscriptionId:      subscription.Id,
			PreConsumed:             initialAppliedCredit,
			AppliedCredit:           initialAppliedCredit,
			DeductedAvailableCredit: initialAppliedCredit,
			ValuationSubscriptionId: subscription.Id,
			DeductedUnknownCredit:   initialAppliedCredit,
			ValuationRuleVersion:    CreditValuationRuleVersion,
			SettlementVersion:       1,
			Status:                  "consumed",
			CreatedAt:               now,
			UpdatedAt:               now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "request_id"}},
			DoNothing: true,
		}).Create(&candidate).Error; err != nil {
			return err
		}

		var route SubscriptionPreConsumeRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", requestId).First(&route).Error; err != nil {
			return err
		}
		if route.UserId != subscription.UserId || route.UserSubscriptionId != originalSubscriptionId || route.ValuationSubscriptionId != originalSubscriptionId {
			return ErrCreditValuationMappingConflict
		}
		if route.PreConsumed != initialAppliedCredit || route.ValuationRuleVersion != CreditValuationRuleVersion {
			return ErrCreditValuationTargetConflict
		}
		return SettleCreditRequestTargetTx(tx, &route, targetCredit, final)
	})
}

func SettleUserSubscriptionRequestTarget(requestId string, originalSubscriptionId int, targetCredit int64, final bool) error {
	requestId = strings.TrimSpace(requestId)
	if requestId == "" || originalSubscriptionId <= 0 {
		return ErrCreditValuationTargetConflict
	}
	if targetCredit < 0 {
		return ErrCreditValuationNegativeInput
	}
	return subscriptionTokenDeltaCoalescer.addRequestTarget(requestId, originalSubscriptionId, targetCredit, final)
}

func settleUserSubscriptionRequestTargetDirect(requestId string, originalSubscriptionId int, targetCredit int64, final bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var route SubscriptionPreConsumeRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", requestId).First(&route).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCreditValuationRequestNotFound
			}
			return err
		}
		if route.RequestId != requestId || route.UserSubscriptionId != originalSubscriptionId {
			return ErrCreditValuationMappingConflict
		}
		return SettleCreditRequestTargetTx(tx, &route, targetCredit, final)
	})
}

func migrateCreditValuationSchema(db *gorm.DB) error {
	if db == nil {
		return ErrDatabase
	}
	return db.AutoMigrate(
		&CreditValuationState{},
		&CreditValuationMigration{},
		&TimedSubscriptionValuationGrant{},
		&SubscriptionPreConsumeRecord{},
		&CreditBalanceLedger{},
		&SubscriptionConversion{},
	)
}
