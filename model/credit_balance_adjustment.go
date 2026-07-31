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
	fingerprint := creditBalanceAdjustmentFingerprint(request)
	var result *CreditBalanceAdjustmentResult
	run := func(tx *gorm.DB) error {
		result = nil
		var existing CreditBalanceAdjustment
		lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("idempotency_key = ?", request.IdempotencyKey).Limit(1).Find(&existing)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected > 0 {
			if existing.ParameterFingerprint != fingerprint {
				return errors.New("Credit balance adjustment idempotency key parameter mismatch")
			}
			loaded, err := creditBalanceAdjustmentResultTx(tx, &existing, true)
			if err != nil {
				return err
			}
			result = loaded
			return nil
		}
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", request.UserId).First(&user).Error; err != nil {
			return err
		}
		plan, err := GetCreditBalancePlanTx(tx)
		if err != nil {
			return err
		}
		now, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		adjustment := &CreditBalanceAdjustment{
			IdempotencyKey: request.IdempotencyKey, ParameterFingerprint: fingerprint,
			UserId: request.UserId, Operation: request.Operation, Amount: request.Amount,
			OperatorUserId: request.OperatorUserId, Reason: request.Reason, CreatedAt: now,
		}
		if err := tx.Create(adjustment).Error; err != nil {
			return err
		}
		snapshotBytes, err := common.Marshal(map[string]any{
			"operation": request.Operation, "amount": request.Amount,
			"operator_user_id": request.OperatorUserId, "reason": request.Reason,
		})
		if err != nil {
			return err
		}
		ledgerKey := fmt.Sprintf("admin_adjustment:%d", adjustment.Id)
		if request.Operation == CreditBalanceAdjustmentIncrease {
			grant, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
				UserId: request.UserId, GrossCredit: request.Amount,
				IdempotencyKey: ledgerKey, SourceType: CreditBalanceLedgerSourceAdminAdjustment,
				SourceId: adjustment.Id, SourceSnapshot: string(snapshotBytes), Type: CreditBalanceLedgerTypeAdminIncrease,
				TargetPlanId: plan.Id, OperatorUserId: request.OperatorUserId, Reason: request.Reason,
			})
			if err != nil {
				return err
			}
			adjustment.LedgerId = grant.LedgerId
			result = &CreditBalanceAdjustmentResult{Adjustment: adjustment, CreditBalance: grant}
		} else {
			if _, err := getOrCreateCreditBalanceSubscriptionTx(tx, request.UserId, plan); err != nil {
				return err
			}
			recovery, err := RecoverCreditBalanceTx(tx, CreditBalanceRecoveryRequest{
				UserId: request.UserId, GrossCredit: request.Amount,
				IdempotencyKey: ledgerKey, SourceType: CreditBalanceLedgerSourceAdminAdjustment,
				SourceId: adjustment.Id, SourceSnapshot: string(snapshotBytes), Type: CreditBalanceLedgerTypeAdminDecrease,
				TargetPlanId: plan.Id, OperatorUserId: request.OperatorUserId, Reason: request.Reason,
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
		if replay, replayErr := findCommittedCreditBalanceAdjustment(request.IdempotencyKey, fingerprint); replayErr == nil && replay != nil {
			return replay, nil
		}
		return nil, err
	}
	primaryBillableSubscriptionCache.Delete(primaryBillableSubscriptionCacheKey(request.UserId))
	invalidateUserCacheBestEffort(request.UserId)
	return result, nil
}

func creditBalanceAdjustmentFingerprint(request CreditBalanceAdjustmentRequest) string {
	payload := fmt.Sprintf("%d\x00%s\x00%d\x00%d\x00%s", request.UserId, request.Operation, request.Amount, request.OperatorUserId, request.Reason)
	return common.Sha1([]byte(payload))
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
		return nil, errors.New("Credit balance adjustment idempotency key parameter mismatch")
	}
	return creditBalanceAdjustmentResultTx(DB, &adjustment, true)
}
