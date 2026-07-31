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
	UserId          int
	GrossCredit     int64
	IdempotencyKey  string
	SourceType      string
	SourceId        int
	SourceSnapshot  string
	Type            string
	TargetPlanId    int
	PaymentProvider string
	OperatorUserId  int
	Reason          string
}

type CreditBalanceRecoveryResult struct {
	UserSubscriptionId int    `json:"user_subscription_id"`
	PlanId             int    `json:"plan_id"`
	GrossCredit        int64  `json:"gross_credit"`
	DebtFormed         int64  `json:"debt_formed"`
	AvailableCredit    int64  `json:"available_credit"`
	SettlementDebt     int64  `json:"settlement_debt"`
	BalanceBefore      int64  `json:"balance_before"`
	BalanceAfter       int64  `json:"balance_after"`
	LedgerId           int    `json:"ledger_id"`
	Status             string `json:"status"`
	Replayed           bool   `json:"replayed"`
}

func RecoverCreditBalanceTx(tx *gorm.DB, request CreditBalanceRecoveryRequest) (*CreditBalanceRecoveryResult, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.SourceType = strings.TrimSpace(request.SourceType)
	request.SourceSnapshot = strings.TrimSpace(request.SourceSnapshot)
	request.Type = strings.TrimSpace(request.Type)
	request.PaymentProvider = strings.TrimSpace(request.PaymentProvider)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.UserId <= 0 || request.GrossCredit <= 0 || request.IdempotencyKey == "" || request.SourceType == "" || request.SourceId <= 0 || request.Type == "" || request.TargetPlanId <= 0 || request.Reason == "" {
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
	if request.GrossCredit > math.MaxInt64-balance.TokenUsed {
		return nil, errors.New("credit balance overflow")
	}

	balanceBefore := balance.TokenLimit - balance.TokenUsed
	availableBefore := maxInt64(balanceBefore, 0)
	debtBefore := maxInt64(-balanceBefore, 0)
	newUsed := balance.TokenUsed + request.GrossCredit
	balanceAfter := balance.TokenLimit - newUsed
	availableAfter := maxInt64(balanceAfter, 0)
	debtAfter := maxInt64(-balanceAfter, 0)
	debtFormed := maxInt64(debtAfter-debtBefore, 0)
	if err := tx.Model(&UserSubscription{}).Where("id = ?", balance.Id).Updates(map[string]any{
		"token_used": newUsed,
		"updated_at": getDBTimestampTx(tx),
	}).Error; err != nil {
		return nil, err
	}

	ledger := CreditBalanceLedger{
		UserId:                request.UserId,
		UserSubscriptionId:    balance.Id,
		Type:                  request.Type,
		IdempotencyKey:        request.IdempotencyKey,
		SourceType:            request.SourceType,
		SourceId:              request.SourceId,
		SourceSnapshot:        request.SourceSnapshot,
		GrossCredit:           -request.GrossCredit,
		DebtFormed:            debtFormed,
		AvailableCreditBefore: availableBefore,
		SettlementDebtBefore:  debtBefore,
		BalanceBefore:         balanceBefore,
		BalanceAfter:          balanceAfter,
		AvailableCreditAfter:  availableAfter,
		SettlementDebtAfter:   debtAfter,
		OperatorUserId:        request.OperatorUserId,
		PaymentProvider:       request.PaymentProvider,
		Reason:                request.Reason,
		CreatedAt:             getDBTimestampTx(tx),
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
	if ledger.UserId != request.UserId || ledger.IdempotencyKey != request.IdempotencyKey || ledger.SourceType != request.SourceType || ledger.SourceId != request.SourceId || ledger.GrossCredit != -request.GrossCredit || ledger.Type != request.Type || ledger.PaymentProvider != request.PaymentProvider {
		return nil, false, errors.New("credit balance recovery idempotency key mismatch")
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
		UserSubscriptionId: ledger.UserSubscriptionId,
		PlanId:             planId,
		GrossCredit:        gross,
		DebtFormed:         ledger.DebtFormed,
		AvailableCredit:    ledger.AvailableCreditAfter,
		SettlementDebt:     ledger.SettlementDebtAfter,
		BalanceBefore:      ledger.BalanceBefore,
		BalanceAfter:       ledger.BalanceAfter,
		LedgerId:           ledger.Id,
		Status:             creditBalanceStatus(ledger.BalanceAfter),
		Replayed:           replayed,
	}
}

func creditBalanceRecoveryIdempotencyKey(orderId int) string {
	return fmt.Sprintf("subscription_order_recovery:%d", orderId)
}
