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

type CreditBalanceAdjustmentResult struct {
	Adjustment    *CreditBalanceAdjustment  `json:"adjustment"`
	CreditBalance *CreditBalanceGrantResult `json:"credit_balance"`
	DebtFormed    int64                     `json:"debt_formed"`
	Replayed      bool                      `json:"replayed"`
}
type creditBalanceAdjustmentValuationFacts struct {
	PlanId               int    `json:"plan_id"`
	SourcePriceMicros    *int64 `json:"source_price_micros"`
	SourcePlanCredit     int64  `json:"source_plan_credit"`
	SourceCurrency       string `json:"source_currency"`
	ValuationCurrency    string `json:"valuation_currency"`
	FxRateNumerator      int64  `json:"fx_rate_numerator"`
	FxRateDenominator    int64  `json:"fx_rate_denominator"`
	ValuationRuleVersion int    `json:"valuation_rule_version"`
}

type creditBalanceAdjustmentSourceSnapshot struct {
	Operation      string                                `json:"operation"`
	Amount         int64                                 `json:"amount"`
	OperatorUserId int                                   `json:"operator_user_id"`
	Reason         string                                `json:"reason"`
	IdempotencyKey string                                `json:"idempotency_key"`
	FxCapturedAt   int64                                 `json:"fx_captured_at"`
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
		fingerprint, err = creditBalanceAdjustmentFingerprint(request, facts)
		if err != nil {
			return err
		}

		var existing CreditBalanceAdjustment
		lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("idempotency_key = ?", request.IdempotencyKey).Limit(1).Find(&existing)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected > 0 {
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

		var valuationSource *CreditValuationSourceSnapshot
		if request.Operation == CreditBalanceAdjustmentIncrease {
			valuationSource, err = validateCreditBalanceAdjustmentFacts(sourcePlan, facts, request.Amount)
			if err != nil {
				return err
			}
		}
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", request.UserId).First(&user).Error; err != nil {
			return err
		}
		now, err := getDBTimestampStrictTx(tx)
		if err != nil {
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
			Reason: request.Reason, IdempotencyKey: request.IdempotencyKey, FxCapturedAt: now, Valuation: facts,
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
				return err
			}
			adjustment.LedgerId = grant.LedgerId
			result = &CreditBalanceAdjustmentResult{Adjustment: adjustment, CreditBalance: grant}
		} else {
			ledgerKey := fmt.Sprintf("admin_adjustment:%d", adjustment.Id)
			if _, _, err := getOrCreateCreditBalanceSubscriptionTx(tx, request.UserId, creditPlan); err != nil {
				return err
			}
			recovery, err := RecoverCreditBalanceTx(tx, CreditBalanceRecoveryRequest{
				UserId: request.UserId, GrossCredit: request.Amount,
				IdempotencyKey: ledgerKey, SourceType: CreditBalanceLedgerSourceAdminAdjustment,
				SourceId: adjustment.Id, SourceSnapshot: string(snapshotBytes), Type: CreditBalanceLedgerTypeAdminDecrease,
				TargetPlanId: creditPlan.Id, OperatorUserId: request.OperatorUserId, Reason: request.Reason,
			})
			if err != nil {
				return err
			}
			adjustment.LedgerId = recovery.LedgerId
			result = &CreditBalanceAdjustmentResult{
				Adjustment: adjustment,
				CreditBalance: &CreditBalanceGrantResult{
					UserSubscriptionId: recovery.UserSubscriptionId, PlanId: recovery.PlanId,
					GrossCredit: -recovery.GrossCredit, AvailableCredit: recovery.AvailableCredit,
					SettlementDebt: recovery.SettlementDebt, BalanceBefore: recovery.BalanceBefore,
					BalanceAfter: recovery.BalanceAfter, LedgerId: recovery.LedgerId, Status: recovery.Status,
				},
				DebtFormed: recovery.DebtFormed,
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
		OperatorUserId int                                   `json:"operator_user_id"`
		Reason         string                                `json:"reason"`
		Valuation      creditBalanceAdjustmentValuationFacts `json:"valuation"`
	}{
		UserId: request.UserId, Operation: request.Operation, Amount: request.Amount,
		PlanId: request.PlanId, OperatorUserId: request.OperatorUserId, Reason: request.Reason,
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
	facts := creditBalanceAdjustmentValuationFacts{
		FxRateNumerator: 1, FxRateDenominator: 1, ValuationRuleVersion: CreditValuationRuleVersion,
	}
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

func validateCreditBalanceAdjustmentFacts(sourcePlan *SubscriptionPlan, facts creditBalanceAdjustmentValuationFacts, amount int64) (*CreditValuationSourceSnapshot, error) {
	if sourcePlan == nil || sourcePlan.Id != facts.PlanId || !sourcePlan.Enabled || sourcePlan.EntitlementType != SubscriptionEntitlementTimed || sourcePlan.IsTrial || sourcePlan.InviteTrial || !sourcePlan.UnlimitedPurchaseEnabled || facts.SourcePriceMicros == nil || *facts.SourcePriceMicros <= 0 || facts.SourcePlanCredit <= 0 {
		return nil, ErrCreditValuationPlanIneligible
	}
	sourceCurrency, err := NormalizeCreditValuationCurrency(facts.SourceCurrency)
	if err != nil {
		return nil, err
	}
	valuationCurrency, err := NormalizeCreditValuationCurrency(facts.ValuationCurrency)
	if err != nil {
		return nil, err
	}
	if sourceCurrency != valuationCurrency {
		return nil, ErrCreditValuationUnsupportedCurrency
	}
	return &CreditValuationSourceSnapshot{
		SourcePriceMicros: *facts.SourcePriceMicros, SourcePlanCredit: facts.SourcePlanCredit,
		GrossCredit: amount, SourceCurrency: sourceCurrency, ValuationCurrency: valuationCurrency,
		RuleVersion: facts.ValuationRuleVersion,
	}, nil
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
	return &CreditBalanceAdjustmentResult{Adjustment: adjustment, CreditBalance: grant, DebtFormed: ledger.DebtFormed, Replayed: replayed}, nil
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
