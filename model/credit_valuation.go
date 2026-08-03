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
}

type creditValuationIngress struct {
	grossCredit     int64
	grossCostMicros int64
	currency        string
	confidence      string
	ruleVersion     int
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
	// Cross-currency runtime valuation is owned by Issue #26. Issue #22 only
	// accepts authoritative source facts already expressed in the pool currency.
	if sourceCurrency != valuationCurrency {
		return creditValuationIngress{}, ErrCreditValuationUnsupportedCurrency
	}
	grossCostMicros, err := mulDivFloor(source.SourcePriceMicros, source.GrossCredit, source.SourcePlanCredit)
	if err != nil {
		return creditValuationIngress{}, err
	}
	return creditValuationIngress{
		grossCredit:     source.GrossCredit,
		grossCostMicros: grossCostMicros,
		currency:        valuationCurrency,
		confidence:      CreditValuationConfidenceExact,
		ruleVersion:     source.RuleVersion,
	}, nil
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
