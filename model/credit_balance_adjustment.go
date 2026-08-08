package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	CreditBalanceAdjustmentIncrease = "increase"
	CreditBalanceAdjustmentDecrease = "decrease"
)

const MaxCreditBalanceAdjustmentAmount int64 = 1_000_000_000_000

type CreditBalanceAdjustment struct {
	Id                   int    `json:"id"`
	IdempotencyKey       string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	ParameterFingerprint string `json:"-" gorm:"type:varchar(64);not null"`
	UserId               int    `json:"user_id" gorm:"not null;index"`
	Operation            string `json:"operation" gorm:"type:varchar(32);not null;index"`
	Amount               int64  `json:"amount" gorm:"type:bigint;not null"`
	PlanId               int    `json:"plan_id" gorm:"not null;default:0;index"`
	OperatorUserId       int    `json:"operator_user_id" gorm:"not null;index"`
	Reason               string `json:"reason" gorm:"type:varchar(255);not null"`
	LedgerId             int    `json:"ledger_id" gorm:"not null;default:0;index"`
	CreatedAt            int64  `json:"created_at" gorm:"type:bigint;not null;index"`
}

func (a *CreditBalanceAdjustment) BeforeUpdate(_ *gorm.DB) error {
	return errors.New("credit balance adjustment is immutable")
}

func (a *CreditBalanceAdjustment) BeforeDelete(_ *gorm.DB) error {
	return errors.New("credit balance adjustment is immutable")
}

type CreditBalanceAdjustmentRequest struct {
	UserId         int
	Operation      string
	Amount         int64
	PlanId         int
	IdempotencyKey string
	OperatorUserId int
	Reason         string
}

type CreditBalanceAdjustmentAuthoritativeResult struct {
	PlanId                     int    `json:"plan_id"`
	GrossCredit                int64  `json:"gross_credit"`
	NetCredit                  int64  `json:"net_credit"`
	GrossAmountMicros          int64  `json:"gross_amount_micros,string"`
	NetAmountMicros            int64  `json:"net_amount_micros,string"`
	ValuationCurrency          string `json:"valuation_currency"`
	SourceCurrency             string `json:"source_currency"`
	Confidence                 string `json:"confidence"`
	FxRateNumerator            int64  `json:"fx_rate_numerator,string"`
	FxRateDenominator          int64  `json:"fx_rate_denominator,string"`
	FxCapturedAt               int64  `json:"fx_captured_at"`
	FxDirection                string `json:"fx_direction"`
	RuleVersion                int    `json:"rule_version"`
	StateVersionAfter          int64  `json:"state_version_after"`
	ConsumedAvailableCredit    int64  `json:"consumed_available_credit"`
	DebtFormed                 int64  `json:"debt_formed"`
	RemovedExactCostMicros     int64  `json:"removed_exact_cost_micros,string"`
	RemovedEstimatedCostMicros int64  `json:"removed_estimated_cost_micros,string"`
	RemovedUnknownCredit       int64  `json:"removed_unknown_credit"`
	Operation                  string `json:"operation"`
	TerminalState              string `json:"terminal_state"`
	DebtOffset                 int64  `json:"debt_offset"`
	AvailableCredit            int64  `json:"available_credit"`
	SettlementDebt             int64  `json:"settlement_debt"`
	BalanceBefore              int64  `json:"balance_before"`
	BalanceAfter               int64  `json:"balance_after"`
	Replayed                   bool   `json:"replayed"`
	Preview                    bool   `json:"preview"`
}

type CreditBalanceAdjustmentResult struct {
	CreditBalanceAdjustmentAuthoritativeResult
	Adjustment    *CreditBalanceAdjustment  `json:"adjustment"`
	CreditBalance *CreditBalanceGrantResult `json:"credit_balance"`
}

type CreditBalanceAdjustmentPreviewRequest struct {
	UserId    int
	Operation string
	Amount    int64
	PlanId    int
}

type CreditBalanceAdjustmentPreviewResult struct {
	CreditBalanceAdjustmentAuthoritativeResult
	CreditBalance *CreditBalanceGrantResult `json:"credit_balance"`
}

func creditBalanceAdjustmentAuthoritativeResult(planId int, balance *CreditBalanceGrantResult, preview bool, replayed bool) CreditBalanceAdjustmentAuthoritativeResult {
	result := CreditBalanceAdjustmentAuthoritativeResult{PlanId: planId, Preview: preview, Replayed: replayed}
	if balance == nil {
		return result
	}
	result.GrossCredit = balance.GrossCredit
	result.NetCredit = balance.NetCredit
	result.GrossAmountMicros = balance.GrossAmountMicros
	result.NetAmountMicros = balance.NetAmountMicros
	result.ValuationCurrency = balance.ValuationCurrency
	result.SourceCurrency = balance.SourceCurrency
	result.Confidence = balance.ValuationConfidence
	result.FxRateNumerator = balance.FxRateNumerator
	result.FxRateDenominator = balance.FxRateDenominator
	result.FxCapturedAt = balance.FxCapturedAt
	result.FxDirection = balance.FxDirection
	result.RuleVersion = balance.ValuationRuleVersion
	result.StateVersionAfter = balance.ValuationStateVersionAfter
	result.ConsumedAvailableCredit = balance.ConsumedAvailableCredit
	result.DebtFormed = balance.DebtFormed
	result.RemovedExactCostMicros = balance.RemovedExactCostMicros
	result.RemovedEstimatedCostMicros = balance.RemovedEstimatedCostMicros
	result.RemovedUnknownCredit = balance.RemovedUnknownCredit
	result.Operation = balance.Operation
	result.TerminalState = balance.TerminalState
	result.DebtOffset = balance.DebtOffset
	result.AvailableCredit = balance.AvailableCredit
	result.SettlementDebt = balance.SettlementDebt
	result.BalanceBefore = balance.BalanceBefore
	result.BalanceAfter = balance.BalanceAfter
	return result
}

func PreviewCreditBalanceAdjustment(request CreditBalanceAdjustmentPreviewRequest) (*CreditBalanceAdjustmentPreviewResult, error) {
	request.Operation = strings.TrimSpace(request.Operation)
	if request.UserId <= 0 || request.Amount <= 0 || request.Amount > MaxCreditBalanceAdjustmentAmount {
		return nil, errors.New("invalid or out-of-range Credit balance adjustment preview request")
	}
	if request.Operation != CreditBalanceAdjustmentIncrease {
		return nil, ErrCreditValuationPlanIneligible
	}
	if request.PlanId <= 0 {
		return nil, ErrCreditValuationPlanRequired
	}

	var result *CreditBalanceAdjustmentPreviewResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		ready, err := CreditValuationRuntimeReadyTx(tx)
		if err != nil {
			return err
		}
		if !ready {
			return ErrCreditValuationMigrationNotReady
		}
		creditPlan, err := lockCreditBalancePlanPreviewTx(tx)
		if err != nil {
			return err
		}
		sourcePlan, err := lockCreditBalanceAdjustmentPlanPreviewTx(tx, request.PlanId)
		if err != nil {
			return err
		}
		var user User
		userQuery := tx.Select("id").Where("id = ?", request.UserId)
		if tx.Dialector != nil && tx.Dialector.Name() != "sqlite" && tx.Dialector.Name() != "sqlite3" {
			userQuery = userQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := userQuery.First(&user).Error; err != nil {
			return err
		}
		facts := creditBalanceAdjustmentFacts(sourcePlan, creditPlan)
		sourceCurrency, valuationCurrency, err := validateCreditBalanceAdjustmentPlanFacts(sourcePlan, facts)
		if err != nil {
			return err
		}
		capturedAt, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		frozenFX, err := captureCreditPositiveIngressFXRateSnapshot(sourceCurrency, valuationCurrency, capturedAt)
		if err != nil {
			return err
		}
		ingress, err := newForwardCreditValuationIngress(CreditValuationSourceSnapshot{
			SourcePriceMicros: *facts.SourcePriceMicros,
			SourcePlanCredit:  facts.SourcePlanCredit,
			GrossCredit:       request.Amount,
			SourceCurrency:    sourceCurrency,
			ValuationCurrency: valuationCurrency,
			RuleVersion:       facts.ValuationRuleVersion,
			FXRateSnapshot:    frozenFX,
		})
		if err != nil {
			return creditBalanceAdjustmentIngressError(err)
		}

		balanceBefore := int64(0)
		stateVersionAfter := int64(1)
		var balance UserSubscription
		balanceQuery := tx.Where("user_id = ? AND entitlement_type = ?", request.UserId, SubscriptionEntitlementCreditBalance).Limit(1)
		if tx.Dialector != nil && tx.Dialector.Name() != "sqlite" && tx.Dialector.Name() != "sqlite3" {
			balanceQuery = balanceQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		query := balanceQuery.Find(&balance)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			if balance.TokenLimit < 0 || balance.TokenUsed < 0 {
				return ErrCreditValuationStateMismatch
			}
			balanceBefore = balance.TokenLimit - balance.TokenUsed
			var state CreditValuationState
			stateQuery := tx.Where("user_subscription_id = ?", balance.Id)
			if tx.Dialector != nil && tx.Dialector.Name() != "sqlite" && tx.Dialector.Name() != "sqlite3" {
				stateQuery = stateQuery.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := stateQuery.First(&state).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrCreditValuationStateMissing
				}
				return err
			}
			if err := validateCreditValuationState(&balance, &state); err != nil {
				return err
			}
			if !strings.EqualFold(state.Currency, valuationCurrency) {
				return ErrCreditValuationStateMismatch
			}
			var ok bool
			stateVersionAfter, ok = checkedAddInt64(state.StateVersion, 1)
			if !ok {
				return ErrCreditValuationOverflow
			}
		}
		debtOffset := minInt64(request.Amount, maxInt64(-balanceBefore, 0))
		netCredit := request.Amount - debtOffset
		netCostMicros, err := prorateFloor(ingress.grossCostMicros, netCredit, request.Amount)
		if err != nil {
			return err
		}
		balanceAfter, ok := checkedAddInt64(balanceBefore, request.Amount)
		if !ok {
			return ErrCreditValuationOverflow
		}
		previewBalance := &CreditBalanceGrantResult{
			PlanId: creditPlan.Id, GrossCredit: request.Amount, NetCredit: netCredit,
			GrossAmountMicros: ingress.grossCostMicros, NetAmountMicros: netCostMicros,
			ValuationCurrency: valuationCurrency, SourceCurrency: sourceCurrency,
			ValuationConfidence: ingress.confidence, FxRateNumerator: frozenFX.Numerator,
			FxRateDenominator: frozenFX.Denominator, FxCapturedAt: frozenFX.CapturedAt,
			FxDirection: frozenFX.Direction, ValuationRuleVersion: ingress.ruleVersion,
			ValuationStateVersionAfter: stateVersionAfter, DebtOffset: debtOffset,
			AvailableCredit: maxInt64(balanceAfter, 0), SettlementDebt: maxInt64(-balanceAfter, 0),
			BalanceBefore: balanceBefore, BalanceAfter: balanceAfter, Status: creditBalanceStatus(balanceAfter),
		}
		result = &CreditBalanceAdjustmentPreviewResult{
			CreditBalanceAdjustmentAuthoritativeResult: creditBalanceAdjustmentAuthoritativeResult(request.PlanId, previewBalance, true, false),
			CreditBalance: previewBalance,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func lockCreditBalancePlanPreviewTx(tx *gorm.DB) (*SubscriptionPlan, error) {
	if tx == nil {
		return nil, ErrDatabase
	}
	query := tx.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance)
	if tx.Dialector != nil && tx.Dialector.Name() != "sqlite" && tx.Dialector.Name() != "sqlite3" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var plan SubscriptionPlan
	if err := query.First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func lockCreditBalanceAdjustmentPlanPreviewTx(tx *gorm.DB, planId int) (*SubscriptionPlan, error) {
	if tx == nil || planId <= 0 {
		return nil, ErrCreditValuationPlanRequired
	}
	query := tx.Where("id = ?", planId)
	if tx.Dialector != nil && tx.Dialector.Name() != "sqlite" && tx.Dialector.Name() != "sqlite3" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var plan SubscriptionPlan
	if err := query.First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCreditValuationPlanIneligible
		}
		return nil, err
	}
	return &plan, nil
}

type creditBalanceAdjustmentValuationFacts struct {
	PlanId               int    `json:"plan_id"`
	SourcePriceMicros    *int64 `json:"source_price_micros"`
	SourcePlanCredit     int64  `json:"source_plan_credit"`
	SourceCurrency       string `json:"source_currency"`
	ValuationCurrency    string `json:"valuation_currency"`
	FxRateNumerator      int64  `json:"fx_rate_numerator"`
	FxRateDenominator    int64  `json:"fx_rate_denominator"`
	FxCapturedAt         int64  `json:"fx_captured_at"`
	FxDirection          string `json:"fx_direction"`
	ValuationRuleVersion int    `json:"valuation_rule_version"`
}

type creditBalanceAdjustmentSourceSnapshot struct {
	Operation      string                                `json:"operation"`
	Amount         int64                                 `json:"amount"`
	OperatorUserId int                                   `json:"operator_user_id"`
	Reason         string                                `json:"reason"`
	IdempotencyKey string                                `json:"idempotency_key"`
	Valuation      creditBalanceAdjustmentValuationFacts `json:"valuation"`
}

func AdjustCreditBalance(request CreditBalanceAdjustmentRequest) (*CreditBalanceAdjustmentResult, error) {
	request.Operation = strings.TrimSpace(request.Operation)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.UserId <= 0 || request.OperatorUserId <= 0 || request.Amount <= 0 || request.Amount > MaxCreditBalanceAdjustmentAmount || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 128 || request.Reason == "" || len(request.Reason) > 255 {
		return nil, errors.New("invalid or out-of-range Credit balance adjustment request")
	}
	if request.Operation != CreditBalanceAdjustmentIncrease && request.Operation != CreditBalanceAdjustmentDecrease {
		return nil, errors.New("Credit balance adjustment operation must be increase or decrease")
	}
	if request.Operation == CreditBalanceAdjustmentIncrease && request.PlanId <= 0 {
		return nil, ErrCreditValuationPlanRequired
	}
	if request.Operation == CreditBalanceAdjustmentDecrease && request.PlanId != 0 {
		return nil, ErrCreditValuationPlanIneligible
	}

	var result *CreditBalanceAdjustmentResult
	var fingerprint string
	run := func(tx *gorm.DB) error {
		result = nil
		var existing CreditBalanceAdjustment
		lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("idempotency_key = ?", request.IdempotencyKey).Limit(1).Find(&existing)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected > 0 {
			facts, err := creditBalanceAdjustmentFrozenFactsTx(tx, &existing)
			if err != nil {
				return err
			}
			fingerprint, err = creditBalanceAdjustmentFingerprint(request, facts)
			if err != nil {
				return err
			}
			if existing.ParameterFingerprint != fingerprint {
				return ErrCreditValuationIdempotencyMismatch
			}
			loaded, err := creditBalanceAdjustmentResultTx(tx, &existing, true)
			if err != nil {
				return err
			}
			result = loaded
			return nil
		}

		creditPlan, err := AcquireCreditBalancePlanGuardTx(tx)
		if err != nil {
			return err
		}
		var sourcePlan *SubscriptionPlan
		facts := creditBalanceAdjustmentValuationFacts{}
		if request.Operation == CreditBalanceAdjustmentIncrease {
			sourcePlan, err = lockCreditBalanceAdjustmentPlanTx(tx, request.PlanId)
			if err != nil {
				return err
			}
			facts = creditBalanceAdjustmentFacts(sourcePlan, creditPlan)
		}

		now, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		var valuationSource *CreditValuationSourceSnapshot
		if request.Operation == CreditBalanceAdjustmentIncrease {
			sourceCurrency, valuationCurrency, err := validateCreditBalanceAdjustmentPlanFacts(sourcePlan, facts)
			if err != nil {
				return err
			}
			frozenFX, err := captureCreditPositiveIngressFXRateSnapshot(sourceCurrency, valuationCurrency, now)
			if err != nil {
				return err
			}
			applyCreditBalanceAdjustmentFXFacts(&facts, frozenFX)
			valuationSource = &CreditValuationSourceSnapshot{
				SourcePriceMicros: *facts.SourcePriceMicros,
				SourcePlanCredit:  facts.SourcePlanCredit,
				GrossCredit:       request.Amount,
				SourceCurrency:    sourceCurrency,
				ValuationCurrency: valuationCurrency,
				RuleVersion:       facts.ValuationRuleVersion,
				FXRateSnapshot:    frozenFX,
			}
		}
		fingerprint, err = creditBalanceAdjustmentFingerprint(request, facts)
		if err != nil {
			return err
		}
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", request.UserId).First(&user).Error; err != nil {
			return err
		}
		adjustment := &CreditBalanceAdjustment{
			IdempotencyKey: request.IdempotencyKey, ParameterFingerprint: fingerprint,
			UserId: request.UserId, Operation: request.Operation, Amount: request.Amount, PlanId: request.PlanId,
			OperatorUserId: request.OperatorUserId, Reason: request.Reason, CreatedAt: now,
		}
		if err := tx.Create(adjustment).Error; err != nil {
			return err
		}
		snapshotBytes, err := common.Marshal(creditBalanceAdjustmentSourceSnapshot{
			Operation: request.Operation, Amount: request.Amount, OperatorUserId: request.OperatorUserId,
			Reason: request.Reason, IdempotencyKey: request.IdempotencyKey, Valuation: facts,
		})
		if err != nil {
			return err
		}
		if request.Operation == CreditBalanceAdjustmentIncrease {
			sourceKey := "admin_adjustment:" + request.IdempotencyKey
			grant, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
				UserId: request.UserId, GrossCredit: request.Amount,
				IdempotencyKey: request.IdempotencyKey, SourceType: CreditBalanceLedgerSourceAdminAdjustment,
				SourceId: adjustment.Id, SourceKey: sourceKey, SourceStatus: "completed",
				SourcePlanId: request.PlanId, ParameterFingerprint: fingerprint,
				SourceSnapshot: string(snapshotBytes), Type: CreditBalanceLedgerTypeAdminIncrease,
				TargetPlanId: creditPlan.Id, OperatorUserId: request.OperatorUserId, Reason: request.Reason,
				ValuationSource: valuationSource,
			})
			if err != nil {
				return creditBalanceAdjustmentIngressError(err)
			}
			adjustment.LedgerId = grant.LedgerId
			result = &CreditBalanceAdjustmentResult{
				CreditBalanceAdjustmentAuthoritativeResult: creditBalanceAdjustmentAuthoritativeResult(request.PlanId, grant, false, false),
				Adjustment:    adjustment,
				CreditBalance: grant,
			}
		} else {
			ledgerKey := fmt.Sprintf("admin_adjustment:%d", adjustment.Id)
			balance, created, err := getOrCreateCreditBalanceSubscriptionTx(tx, request.UserId, creditPlan)
			if err != nil {
				return err
			}
			valuationReady, err := CreditValuationRuntimeReadyTx(tx)
			if err != nil {
				return err
			}
			if valuationReady && created {
				if creditPlan.ValuationCurrency == nil {
					return ErrCreditValuationCurrencyRequired
				}
				if err := initializeCreditValuationStateTx(tx, balance, *creditPlan.ValuationCurrency); err != nil {
					return err
				}
			}
			recovery, err := RecoverCreditBalanceTx(tx, CreditBalanceRecoveryRequest{
				UserId: request.UserId, GrossCredit: request.Amount,
				IdempotencyKey: ledgerKey, SourceType: CreditBalanceLedgerSourceAdminAdjustment,
				SourceId: adjustment.Id, SourceKey: ledgerKey, Operation: request.Operation, TerminalState: "completed",
				SourceSnapshot: string(snapshotBytes), Type: CreditBalanceLedgerTypeAdminDecrease,
				TargetPlanId: creditPlan.Id, OperatorUserId: request.OperatorUserId, Reason: request.Reason,
				ParameterFingerprint: fingerprint,
			})
			if err != nil {
				return err
			}
			adjustment.LedgerId = recovery.LedgerId
			result, err = creditBalanceAdjustmentResultTx(tx, adjustment, false)
			if err != nil {
				return err
			}
		}
		if err := tx.Model(&CreditBalanceAdjustment{}).Where("id = ?", adjustment.Id).UpdateColumn("ledger_id", adjustment.LedgerId).Error; err != nil {
			return err
		}
		return nil
	}
	if err := transactionWithUserSettingCASRetry(run); err != nil {
		if fingerprint != "" {
			replay, replayErr := findCommittedCreditBalanceAdjustment(request.IdempotencyKey, fingerprint)
			if replayErr == nil && replay != nil {
				return replay, nil
			}
			if errors.Is(replayErr, ErrCreditValuationIdempotencyMismatch) {
				return nil, replayErr
			}
		}
		return nil, err
	}
	primaryBillableSubscriptionCache.Delete(primaryBillableSubscriptionCacheKey(request.UserId))
	invalidateUserCacheBestEffort(request.UserId)
	return result, nil
}

func creditBalanceAdjustmentFingerprint(request CreditBalanceAdjustmentRequest, facts creditBalanceAdjustmentValuationFacts) (string, error) {
	payload, err := common.Marshal(struct {
		UserId         int                                   `json:"user_id"`
		Operation      string                                `json:"operation"`
		Amount         int64                                 `json:"amount"`
		PlanId         int                                   `json:"plan_id"`
		IdempotencyKey string                                `json:"idempotency_key"`
		OperatorUserId int                                   `json:"operator_user_id"`
		Reason         string                                `json:"reason"`
		Valuation      creditBalanceAdjustmentValuationFacts `json:"valuation"`
	}{
		UserId: request.UserId, Operation: request.Operation, Amount: request.Amount,
		PlanId: request.PlanId, IdempotencyKey: request.IdempotencyKey,
		OperatorUserId: request.OperatorUserId, Reason: request.Reason,
		Valuation: facts,
	})
	if err != nil {
		return "", err
	}
	return common.Sha1(payload), nil
}
func lockCreditBalanceAdjustmentPlanTx(tx *gorm.DB, planId int) (*SubscriptionPlan, error) {
	if tx == nil || planId <= 0 {
		return nil, ErrCreditValuationPlanRequired
	}
	var plan SubscriptionPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", planId).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCreditValuationPlanIneligible
		}
		return nil, err
	}
	return &plan, nil
}

func creditBalanceAdjustmentFacts(sourcePlan *SubscriptionPlan, creditPlan *SubscriptionPlan) creditBalanceAdjustmentValuationFacts {
	facts := creditBalanceAdjustmentValuationFacts{ValuationRuleVersion: CreditValuationRuleVersion}
	if sourcePlan != nil {
		facts.PlanId = sourcePlan.Id
		facts.SourcePriceMicros = sourcePlan.PriceAmountMicros
		facts.SourcePlanCredit = sourcePlan.MonthlyTokenLimit
		facts.SourceCurrency = strings.ToUpper(strings.TrimSpace(sourcePlan.Currency))
	}
	if creditPlan != nil && creditPlan.ValuationCurrency != nil {
		facts.ValuationCurrency = strings.ToUpper(strings.TrimSpace(*creditPlan.ValuationCurrency))
	}
	return facts
}

func validateCreditBalanceAdjustmentPlanFacts(sourcePlan *SubscriptionPlan, facts creditBalanceAdjustmentValuationFacts) (string, string, error) {
	if sourcePlan == nil || sourcePlan.Id != facts.PlanId || !sourcePlan.Enabled || sourcePlan.EntitlementType != SubscriptionEntitlementTimed || sourcePlan.IsTrial || sourcePlan.InviteTrial || !sourcePlan.UnlimitedPurchaseEnabled || facts.SourcePriceMicros == nil || *facts.SourcePriceMicros <= 0 || facts.SourcePlanCredit <= 0 {
		return "", "", ErrCreditValuationPlanIneligible
	}
	sourceCurrency, err := NormalizeCreditValuationCurrency(facts.SourceCurrency)
	if err != nil {
		return "", "", err
	}
	valuationCurrency, err := NormalizeCreditValuationCurrency(facts.ValuationCurrency)
	if err != nil {
		return "", "", err
	}
	return sourceCurrency, valuationCurrency, nil
}

func applyCreditBalanceAdjustmentFXFacts(facts *creditBalanceAdjustmentValuationFacts, snapshot *CreditFXRateSnapshot) {
	if facts == nil || snapshot == nil {
		return
	}
	facts.FxRateNumerator = snapshot.Numerator
	facts.FxRateDenominator = snapshot.Denominator
	facts.FxCapturedAt = snapshot.CapturedAt
	facts.FxDirection = snapshot.Direction
}

func creditBalanceAdjustmentFXSnapshotTx(tx *gorm.DB, adjustment *CreditBalanceAdjustment) (*CreditFXRateSnapshot, error) {
	if tx == nil || adjustment == nil || adjustment.Id <= 0 || adjustment.LedgerId <= 0 {
		return nil, ErrCreditValuationInvalidFX
	}
	var ledger CreditBalanceLedger
	if err := tx.Where("id = ? AND source_type = ? AND source_id = ?", adjustment.LedgerId, CreditBalanceLedgerSourceAdminAdjustment, adjustment.Id).First(&ledger).Error; err != nil {
		return nil, err
	}
	snapshot := &CreditFXRateSnapshot{
		SourceCurrency: ledger.FxSourceCurrency, ValuationCurrency: ledger.ValuationCurrency,
		Numerator: ledger.FxRateNumerator, Denominator: ledger.FxRateDenominator,
		CapturedAt: ledger.FxCapturedAt, Direction: ledger.FxDirection,
	}
	if err := validateCreditPositiveIngressFXRateSnapshot(snapshot, snapshot.SourceCurrency, snapshot.ValuationCurrency); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func creditBalanceAdjustmentFrozenFactsTx(tx *gorm.DB, adjustment *CreditBalanceAdjustment) (creditBalanceAdjustmentValuationFacts, error) {
	if tx == nil || adjustment == nil || adjustment.Id <= 0 || adjustment.LedgerId <= 0 {
		return creditBalanceAdjustmentValuationFacts{}, ErrCreditValuationIdempotencyMismatch
	}
	var ledger CreditBalanceLedger
	if err := tx.Select("source_snapshot").Where(
		"id = ? AND source_type = ? AND source_id = ?",
		adjustment.LedgerId,
		CreditBalanceLedgerSourceAdminAdjustment,
		adjustment.Id,
	).First(&ledger).Error; err != nil {
		return creditBalanceAdjustmentValuationFacts{}, err
	}
	var snapshot creditBalanceAdjustmentSourceSnapshot
	if strings.TrimSpace(ledger.SourceSnapshot) == "" || common.UnmarshalJsonStr(ledger.SourceSnapshot, &snapshot) != nil {
		return creditBalanceAdjustmentValuationFacts{}, ErrCreditValuationIdempotencyMismatch
	}
	if snapshot.Operation != adjustment.Operation ||
		snapshot.Amount != adjustment.Amount ||
		snapshot.OperatorUserId != adjustment.OperatorUserId ||
		snapshot.Reason != adjustment.Reason ||
		snapshot.IdempotencyKey != adjustment.IdempotencyKey ||
		snapshot.Valuation.PlanId != adjustment.PlanId {
		return creditBalanceAdjustmentValuationFacts{}, ErrCreditValuationIdempotencyMismatch
	}
	return snapshot.Valuation, nil
}

func creditBalanceAdjustmentIngressError(err error) error {
	switch {
	case errors.Is(err, ErrCreditFXOverflow), errors.Is(err, ErrCreditValuationOverflow):
		return ErrCreditValuationOverflow
	case errors.Is(err, ErrCreditFXRateInvalid),
		errors.Is(err, ErrCreditFXRateMissing),
		errors.Is(err, ErrCreditFXRateEmpty),
		errors.Is(err, ErrCreditFXInvalidDecimal),
		errors.Is(err, ErrCreditFXPrecisionExceeded),
		errors.Is(err, ErrCreditFXNonPositive),
		errors.Is(err, ErrCreditFXDirectionMismatch):
		return ErrCreditValuationInvalidFX
	default:
		return err
	}
}

func creditBalanceAdjustmentResultTx(tx *gorm.DB, adjustment *CreditBalanceAdjustment, replayed bool) (*CreditBalanceAdjustmentResult, error) {
	if tx == nil || adjustment == nil || adjustment.LedgerId <= 0 {
		return nil, errors.New("invalid committed Credit balance adjustment")
	}
	var ledger CreditBalanceLedger
	if err := tx.Where("id = ? AND source_type = ? AND source_id = ?", adjustment.LedgerId, CreditBalanceLedgerSourceAdminAdjustment, adjustment.Id).First(&ledger).Error; err != nil {
		return nil, err
	}
	var balance UserSubscription
	if err := tx.Select("id", "plan_id").Where("id = ?", ledger.UserSubscriptionId).First(&balance).Error; err != nil {
		return nil, err
	}
	grant := creditBalanceGrantResult(&ledger, balance.PlanId, false)
	grant.Replayed = replayed
	return &CreditBalanceAdjustmentResult{
		CreditBalanceAdjustmentAuthoritativeResult: creditBalanceAdjustmentAuthoritativeResult(adjustment.PlanId, grant, false, replayed),
		Adjustment:    adjustment,
		CreditBalance: grant,
	}, nil
}

func findCommittedCreditBalanceAdjustment(idempotencyKey string, fingerprint string) (*CreditBalanceAdjustmentResult, error) {
	var adjustment CreditBalanceAdjustment
	if err := DB.Where("idempotency_key = ?", idempotencyKey).First(&adjustment).Error; err != nil {
		return nil, err
	}
	if adjustment.ParameterFingerprint != fingerprint {
		return nil, ErrCreditValuationIdempotencyMismatch
	}
	return creditBalanceAdjustmentResultTx(DB, &adjustment, true)
}
