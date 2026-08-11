package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CreditBalanceRecoveryRequest struct {
	UserId               int
	GrossCredit          int64
	IdempotencyKey       string
	SourceType           string
	SourceId             int
	SourceKey            string
	Operation            string
	TerminalState        string
	SourceSnapshot       string
	Type                 string
	TargetPlanId         int
	PaymentProvider      string
	OperatorUserId       int
	Reason               string
	ParameterFingerprint string
}

type CreditBalanceRecoveryResult struct {
	UserSubscriptionId         int    `json:"user_subscription_id"`
	PlanId                     int    `json:"plan_id"`
	GrossCredit                int64  `json:"gross_credit"`
	ConsumedAvailableCredit    int64  `json:"consumed_available_credit"`
	RemovedExactCostMicros     int64  `json:"removed_exact_cost_micros,string"`
	RemovedEstimatedCostMicros int64  `json:"removed_estimated_cost_micros,string"`
	RemovedUnknownCredit       int64  `json:"removed_unknown_credit"`
	ValuationCurrency          string `json:"valuation_currency"`
	RuleVersion                int    `json:"rule_version"`
	StateVersionAfter          int64  `json:"state_version_after"`
	Operation                  string `json:"operation"`
	TerminalState              string `json:"terminal_state"`
	DebtFormed                 int64  `json:"debt_formed"`
	AvailableCredit            int64  `json:"available_credit"`
	SettlementDebt             int64  `json:"settlement_debt"`
	BalanceBefore              int64  `json:"balance_before"`
	BalanceAfter               int64  `json:"balance_after"`
	LedgerId                   int    `json:"ledger_id"`
	Status                     string `json:"status"`
	Replayed                   bool   `json:"replayed"`
}

func RecoverCreditBalanceTx(tx *gorm.DB, request CreditBalanceRecoveryRequest) (*CreditBalanceRecoveryResult, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.SourceType = strings.TrimSpace(request.SourceType)
	request.SourceKey = strings.TrimSpace(request.SourceKey)
	request.Operation = strings.TrimSpace(request.Operation)
	request.TerminalState = strings.TrimSpace(request.TerminalState)
	request.SourceSnapshot = strings.TrimSpace(request.SourceSnapshot)
	request.Type = strings.TrimSpace(request.Type)
	request.PaymentProvider = strings.TrimSpace(request.PaymentProvider)
	request.Reason = strings.TrimSpace(request.Reason)
	request.ParameterFingerprint = strings.TrimSpace(request.ParameterFingerprint)
	if request.UserId <= 0 || request.GrossCredit <= 0 || request.IdempotencyKey == "" || request.SourceType == "" || request.SourceId <= 0 || request.SourceKey == "" || request.Operation == "" || request.TerminalState == "" || request.Type == "" || request.TargetPlanId <= 0 || request.Reason == "" || request.ParameterFingerprint == "" {
		return nil, errors.New("invalid credit balance recovery")
	}

	var user User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", request.UserId).First(&user).Error; err != nil {
		return nil, err
	}
	if result, found, err := findCreditBalanceRecoveryResultTx(tx, request); err != nil {
		return nil, err
	} else if found {
		result.Replayed = true
		return result, nil
	}

	var balance UserSubscription
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND plan_id = ? AND entitlement_type = ?", request.UserId, request.TargetPlanId, SubscriptionEntitlementCreditBalance).
		Limit(1).
		Find(&balance)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected != 1 {
		return nil, errors.New("credit balance aggregate not found")
	}
	if balance.TokenLimit < 0 || balance.TokenUsed < 0 {
		return nil, errors.New("invalid credit balance aggregate")
	}
	balanceBefore := balance.TokenLimit - balance.TokenUsed
	availableBefore := maxInt64(balanceBefore, 0)
	debtBefore := maxInt64(-balanceBefore, 0)
	valuationReady, err := CreditValuationWriterReadyTx(tx)
	if err != nil {
		return nil, err
	}
	valuationMutation := CreditValuationMutationResult{}
	if valuationReady {
		valuationMutation, err = ApplyCreditValuationOutflowTx(tx, &balance, request.GrossCredit, request.Type)
		if err != nil {
			return nil, err
		}
	} else {
		if request.GrossCredit > math.MaxInt64-balance.TokenUsed {
			return nil, errors.New("credit balance overflow")
		}
		balance.TokenUsed += request.GrossCredit
		if err := tx.Model(&UserSubscription{}).Where("id = ?", balance.Id).Updates(map[string]any{
			"token_used": balance.TokenUsed,
			"updated_at": getDBTimestampTx(tx),
		}).Error; err != nil {
			return nil, err
		}
	}

	balanceAfter := balance.TokenLimit - balance.TokenUsed
	availableAfter := maxInt64(balanceAfter, 0)
	debtAfter := maxInt64(-balanceAfter, 0)
	debtFormed := maxInt64(debtAfter-debtBefore, 0)
	consumedAvailable := minInt64(request.GrossCredit, availableBefore)
	removedCostMicros, ok := checkedAddInt64(valuationMutation.RemovedExactCostMicros, valuationMutation.RemovedEstimatedCostMicros)
	if !ok {
		return nil, ErrCreditValuationOverflow
	}
	valuationCurrency := ""
	valuationConfidence := ""
	valuationRuleVersion := 0
	if valuationReady {
		var state CreditValuationState
		if err := tx.Select("currency").Where("user_subscription_id = ?", balance.Id).First(&state).Error; err != nil {
			return nil, err
		}
		valuationCurrency = state.Currency
		valuationRuleVersion = CreditValuationRuleVersion
		switch {
		case valuationMutation.RemovedUnknownCredit > 0:
			valuationConfidence = CreditValuationConfidenceUnknown
		case valuationMutation.RemovedEstimatedCostMicros > 0:
			valuationConfidence = CreditValuationConfidenceEstimated
		case valuationMutation.RemovedExactCostMicros > 0:
			valuationConfidence = CreditValuationConfidenceExact
		}
	}

	ledger := CreditBalanceLedger{
		UserId:                     request.UserId,
		UserSubscriptionId:         balance.Id,
		Type:                       request.Type,
		IdempotencyKey:             request.IdempotencyKey,
		SourceType:                 request.SourceType,
		SourceId:                   request.SourceId,
		SourceSnapshot:             request.SourceSnapshot,
		SourceKey:                  request.SourceKey,
		SourceStatus:               request.TerminalState,
		Operation:                  request.Operation,
		TerminalState:              request.TerminalState,
		TargetPlanId:               request.TargetPlanId,
		GrossCredit:                -request.GrossCredit,
		NetCredit:                  -minInt64(request.GrossCredit, availableBefore),
		DebtFormed:                 debtFormed,
		ConsumedAvailableCredit:    consumedAvailable,
		SettlementDebtFormed:       debtFormed,
		RemovedExactCostMicros:     valuationMutation.RemovedExactCostMicros,
		RemovedEstimatedCostMicros: valuationMutation.RemovedEstimatedCostMicros,
		RemovedUnknownCredit:       valuationMutation.RemovedUnknownCredit,
		ValuationCurrency:          valuationCurrency,
		ValuationGrossCostMicros:   removedCostMicros,
		ValuationNetCostMicros:     removedCostMicros,
		ValuationConfidence:        valuationConfidence,
		ValuationRuleVersion:       valuationRuleVersion,
		ValuationStateVersionAfter: valuationMutation.StateVersionAfter,
		AvailableCreditBefore:      availableBefore,
		SettlementDebtBefore:       debtBefore,
		BalanceBefore:              balanceBefore,
		BalanceAfter:               balanceAfter,
		AvailableCreditAfter:       availableAfter,
		SettlementDebtAfter:        debtAfter,
		OperatorUserId:             request.OperatorUserId,
		PaymentProvider:            request.PaymentProvider,
		Reason:                     request.Reason,
		ParameterFingerprint:       request.ParameterFingerprint,
		CreatedAt:                  getDBTimestampTx(tx),
	}
	if request.SourceType == CreditBalanceLedgerSourceSubscriptionOrderRecovery {
		purchase, found, err := subscriptionOrderCreditPurchaseLedgerTx(tx, &SubscriptionOrder{Id: request.SourceId, UserId: request.UserId})
		if err != nil {
			return nil, err
		}
		if found {
			if purchase.TargetPlanId != request.TargetPlanId || purchase.GrossCredit != request.GrossCredit {
				return nil, ErrSubscriptionOrderSnapshotMismatch
			}
			ledger.PlanId = purchase.PlanId
			ledger.SourcePlanId = purchase.SourcePlanId
			ledger.SourcePriceMicros = purchase.SourcePriceMicros
			ledger.SourcePlanCredit = purchase.SourcePlanCredit
			ledger.FxSourceCurrency = purchase.FxSourceCurrency
			ledger.FxRateNumerator = purchase.FxRateNumerator
			ledger.FxRateDenominator = purchase.FxRateDenominator
			ledger.FxCapturedAt = purchase.FxCapturedAt
			ledger.FxDirection = purchase.FxDirection
		} else {
			var order SubscriptionOrder
			if err := tx.Where("id = ? AND user_id = ?", request.SourceId, request.UserId).First(&order).Error; err != nil {
				return nil, ErrSubscriptionOrderSnapshotMismatch
			}
			source, conversion, converted, err := recoverableConvertedSubscriptionForOrderTx(tx, &order)
			if err != nil {
				return nil, err
			}
			if !converted || source == nil || conversion == nil || conversion.TargetPlanId != request.TargetPlanId || conversion.GrossCredit != request.GrossCredit {
				return nil, ErrSubscriptionOrderSnapshotMismatch
			}
			ledger.PlanId = order.PlanId
			ledger.SourcePlanId = conversion.SourcePlanId
			ledger.SourcePriceMicros = conversion.ValuationSourcePriceMicros
			ledger.SourcePlanCredit = conversion.ValuationCreditBasis
			ledger.FxSourceCurrency = conversion.FxSourceCurrency
			ledger.FxRateNumerator = conversion.FxRateNumerator
			ledger.FxRateDenominator = conversion.FxRateDenominator
			ledger.FxCapturedAt = conversion.FxCapturedAt
			ledger.FxDirection = creditFXDirection(conversion.FxSourceCurrency, conversion.ValuationCurrency)
		}
	}
	if err := tx.Create(&ledger).Error; err != nil {
		return nil, err
	}
	return creditBalanceRecoveryResult(&ledger, request.TargetPlanId, false), nil
}

func findCreditBalanceRecoveryResultTx(tx *gorm.DB, request CreditBalanceRecoveryRequest) (*CreditBalanceRecoveryResult, bool, error) {
	var ledger CreditBalanceLedger
	query := tx.Where("(user_id = ? AND idempotency_key = ?) OR (source_type = ? AND source_id = ?)", request.UserId, request.IdempotencyKey, request.SourceType, request.SourceId).Limit(1).Find(&ledger)
	if query.Error != nil {
		return nil, false, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, false, nil
	}
	if ledger.UserId != request.UserId || ledger.IdempotencyKey != request.IdempotencyKey || ledger.SourceType != request.SourceType || ledger.SourceId != request.SourceId || ledger.SourceKey != request.SourceKey || ledger.Operation != request.Operation || ledger.TerminalState != request.TerminalState || ledger.GrossCredit != -request.GrossCredit || ledger.Type != request.Type || ledger.PaymentProvider != request.PaymentProvider || ledger.ParameterFingerprint != request.ParameterFingerprint {
		return nil, false, ErrCreditValuationIdempotencyMismatch
	}
	var balance UserSubscription
	if err := tx.Select("id", "plan_id").Where("id = ?", ledger.UserSubscriptionId).First(&balance).Error; err != nil {
		return nil, false, err
	}
	if balance.PlanId != request.TargetPlanId {
		return nil, false, errors.New("credit balance recovery target plan mismatch")
	}
	return creditBalanceRecoveryResult(&ledger, balance.PlanId, true), true, nil
}

func creditBalanceRecoveryResult(ledger *CreditBalanceLedger, planId int, replayed bool) *CreditBalanceRecoveryResult {
	if ledger == nil {
		return nil
	}
	gross := ledger.GrossCredit
	if gross < 0 {
		gross = -gross
	}
	return &CreditBalanceRecoveryResult{
		UserSubscriptionId:         ledger.UserSubscriptionId,
		PlanId:                     planId,
		GrossCredit:                gross,
		ConsumedAvailableCredit:    ledger.ConsumedAvailableCredit,
		RemovedExactCostMicros:     ledger.RemovedExactCostMicros,
		RemovedEstimatedCostMicros: ledger.RemovedEstimatedCostMicros,
		RemovedUnknownCredit:       ledger.RemovedUnknownCredit,
		ValuationCurrency:          ledger.ValuationCurrency,
		RuleVersion:                ledger.ValuationRuleVersion,
		StateVersionAfter:          ledger.ValuationStateVersionAfter,
		Operation:                  ledger.Operation,
		TerminalState:              ledger.TerminalState,
		DebtFormed:                 ledger.DebtFormed,
		AvailableCredit:            ledger.AvailableCreditAfter,
		SettlementDebt:             ledger.SettlementDebtAfter,
		BalanceBefore:              ledger.BalanceBefore,
		BalanceAfter:               ledger.BalanceAfter,
		LedgerId:                   ledger.Id,
		Status:                     creditBalanceStatus(ledger.BalanceAfter),
		Replayed:                   replayed,
	}
}

func subscriptionOrderCreditPurchaseLedgerTx(tx *gorm.DB, order *SubscriptionOrder) (*CreditBalanceLedger, bool, error) {
	if tx == nil || order == nil || order.Id <= 0 || order.UserId <= 0 {
		return nil, false, errors.New("invalid subscription order purchase facts")
	}
	var ledger CreditBalanceLedger
	query := tx.Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrder, order.Id).Limit(1).Find(&ledger)
	if query.Error != nil {
		return nil, false, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, false, nil
	}
	if ledger.UserId != order.UserId || ledger.Type != CreditBalanceLedgerTypePurchase || ledger.GrossCredit <= 0 || ledger.TargetPlanId <= 0 || ledger.UserSubscriptionId <= 0 {
		return nil, false, ErrSubscriptionOrderSnapshotMismatch
	}
	return &ledger, true, nil
}

func creditBalanceRecoveryIdempotencyKey(orderId int) string {
	return fmt.Sprintf("subscription_order_recovery:%d", orderId)
}
