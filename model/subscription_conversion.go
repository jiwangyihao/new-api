package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type subscriptionConversionHookPhase string

const (
	subscriptionConversionAfterQuotePhase            subscriptionConversionHookPhase = "after_quote"
	subscriptionConversionAfterEligibilityGuardPhase subscriptionConversionHookPhase = "after_eligibility_guard"
)

type subscriptionConversionHooks struct {
	at func(subscriptionConversionHookPhase) error
}

type SubscriptionConversion struct {
	Id                         int    `json:"id"`
	UserId                     int    `json:"user_id" gorm:"not null;index;uniqueIndex:idx_subscription_conversion_user_key,priority:1"`
	IdempotencyKey             string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex:idx_subscription_conversion_user_key,priority:2"`
	SourceSubscriptionId       int    `json:"source_subscription_id" gorm:"not null;uniqueIndex:idx_subscription_conversions_source_subscription_id"`
	SourcePlanId               int    `json:"source_plan_id" gorm:"not null;index"`
	SourcePlanTitle            string `json:"source_plan_title" gorm:"type:varchar(255);not null"`
	TargetSubscriptionId       int    `json:"target_subscription_id" gorm:"not null;index"`
	TargetPlanId               int    `json:"target_plan_id" gorm:"not null;index"`
	LedgerId                   int    `json:"ledger_id" gorm:"not null;uniqueIndex"`
	SourceStatus               string `json:"source_status" gorm:"type:varchar(32);not null"`
	GrantSource                string `json:"grant_source" gorm:"type:varchar(32);not null"`
	DatabaseNow                int64  `json:"database_now" gorm:"type:bigint;not null"`
	SourceStartTime            int64  `json:"source_start_time" gorm:"type:bigint;not null"`
	SourceEndTime              int64  `json:"source_end_time" gorm:"type:bigint;not null"`
	RemainingSeconds           int64  `json:"remaining_seconds" gorm:"type:bigint;not null"`
	Full31DayBlocks            int64  `json:"full_31_day_blocks" gorm:"type:bigint;not null"`
	CreditBasis                int64  `json:"credit_basis" gorm:"type:bigint;not null"`
	CreditBasisSource          string `json:"credit_basis_source" gorm:"type:varchar(32);not null"`
	CurrentRemainingCredit     int64  `json:"current_remaining_credit" gorm:"type:bigint;not null"`
	GrossCredit                int64  `json:"gross_credit" gorm:"type:bigint;not null"`
	DebtOffset                 int64  `json:"debt_offset" gorm:"type:bigint;not null"`
	NetAvailableCredit         int64  `json:"net_available_credit" gorm:"type:bigint;not null"`
	AvailableCreditAfter       int64  `json:"available_credit_after" gorm:"type:bigint;not null"`
	SettlementDebtAfter        int64  `json:"settlement_debt_after" gorm:"type:bigint;not null"`
	BalanceBefore              int64  `json:"balance_before" gorm:"type:bigint;not null"`
	BalanceAfter               int64  `json:"balance_after" gorm:"type:bigint;not null"`
	LastGrantedAt              int64  `json:"last_granted_at" gorm:"type:bigint;not null"`
	LastGrantTimeSource        string `json:"last_grant_time_source" gorm:"type:varchar(64);not null"`
	LastGrantSource            string `json:"last_grant_source" gorm:"type:varchar(32);not null"`
	ConvertedAt                int64  `json:"converted_at" gorm:"type:bigint;not null;index"`
	ValuationCurrency          string `json:"valuation_currency" gorm:"type:varchar(8);not null;default:''"`
	ValuationSourcePriceMicros int64  `json:"valuation_source_price_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationCreditBasis       int64  `json:"valuation_credit_basis" gorm:"type:bigint;not null;default:0"`
	ValuationGrossCostMicros   int64  `json:"valuation_gross_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationNetCostMicros     int64  `json:"valuation_net_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationConfidence        string `json:"valuation_confidence" gorm:"type:varchar(16);not null;default:''"`
	ValuationRuleVersion       int    `json:"valuation_rule_version" gorm:"not null;default:0"`
	ValuationStateVersionAfter int64  `json:"-" gorm:"->;-:migration"`
	FxSourceCurrency           string `json:"fx_source_currency" gorm:"type:varchar(8);not null;default:''"`
	FxRateNumerator            int64  `json:"fx_rate_numerator,string" gorm:"type:bigint;not null;default:0"`
	FxRateDenominator          int64  `json:"fx_rate_denominator,string" gorm:"type:bigint;not null;default:0"`
	FxCapturedAt               int64  `json:"fx_captured_at" gorm:"type:bigint;not null;default:0"`
	CreatedAt                  int64  `json:"created_at" gorm:"type:bigint;not null"`
}

func (c *SubscriptionConversion) BeforeUpdate(_ *gorm.DB) error {
	return errors.New("subscription conversion is immutable")
}

func (c *SubscriptionConversion) BeforeDelete(_ *gorm.DB) error {
	return errors.New("subscription conversion is immutable")
}

type SubscriptionConversionResult struct {
	Conversion *SubscriptionConversion `json:"conversion"`
	Replayed   bool                    `json:"replayed"`
}

func ConfirmTimedSubscriptionConversion(userId int, sourceSubscriptionId int, idempotencyKey string) (*SubscriptionConversionResult, error) {
	return confirmTimedSubscriptionConversion(userId, sourceSubscriptionId, idempotencyKey, nil)
}

func confirmTimedSubscriptionConversion(userId int, sourceSubscriptionId int, idempotencyKey string, hooks *subscriptionConversionHooks) (*SubscriptionConversionResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if userId <= 0 || sourceSubscriptionId <= 0 || idempotencyKey == "" || len(idempotencyKey) > 128 {
		return nil, errors.New("invalid subscription conversion request")
	}
	if DB == nil {
		return nil, errors.New("database is nil")
	}

	var result *SubscriptionConversionResult
	run := func(tx *gorm.DB) error {
		result = nil
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "setting").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}

		if replay, found, err := findSubscriptionConversionByIdempotencyTx(tx, userId, idempotencyKey); err != nil {
			return err
		} else if found {
			if replay.SourceSubscriptionId != sourceSubscriptionId {
				return ErrConversionIdempotencyConflict
			}
			if err := validateSubscriptionConversionReplayFactsTx(tx, replay); err != nil {
				return err
			}
			result = &SubscriptionConversionResult{Conversion: replay, Replayed: true}
			return nil
		}
		if existing, found, err := findSubscriptionConversionBySourceTx(tx, sourceSubscriptionId); err != nil {
			return err
		} else if found {
			if existing.UserId == userId && existing.IdempotencyKey == idempotencyKey {
				if err := validateSubscriptionConversionReplayFactsTx(tx, existing); err != nil {
					return err
				}
				result = &SubscriptionConversionResult{Conversion: existing, Replayed: true}
				return nil
			}
			return errors.New("source subscription already converted")
		}

		var source UserSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", sourceSubscriptionId, userId).First(&source).Error; err != nil {
			return err
		}
		dbNow, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		quote, err := RecalculateTimedSubscriptionConversionQuoteTx(tx, userId, sourceSubscriptionId, dbNow)
		if err != nil {
			return err
		}
		if hooks != nil && hooks.at != nil {
			if err := hooks.at(subscriptionConversionAfterQuotePhase); err != nil {
				return err
			}
		}
		if !quote.CanConfirm {
			return subscriptionConversionRejection(quote)
		}

		creditPlan, err := GetCreditBalancePlanTx(tx)
		if err != nil {
			return err
		}
		if err := guardSubscriptionConversionPlansTx(tx, quote.PlanId, creditPlan.Id); err != nil {
			return err
		}
		if hooks != nil && hooks.at != nil {
			if err := hooks.at(subscriptionConversionAfterEligibilityGuardPhase); err != nil {
				return err
			}
		}
		quote, err = RecalculateTimedSubscriptionConversionQuoteTx(tx, userId, sourceSubscriptionId, dbNow)
		if err != nil {
			return err
		}
		if !quote.CanConfirm {
			return subscriptionConversionRejection(quote)
		}
		var valuationSource *CreditValuationSourceSnapshot
		valuationReady, err := CreditValuationRuntimeReadyTx(tx)
		if err != nil {
			return err
		}
		if valuationReady {
			var sourcePlan SubscriptionPlan
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", quote.PlanId).First(&sourcePlan).Error; err != nil {
				return err
			}
			if sourcePlan.PriceAmountMicros == nil {
				return ErrSubscriptionPlanPriceRequired
			}
			if *sourcePlan.PriceAmountMicros <= 0 {
				return ErrCreditValuationSourceInvalid
			}
			sourceCurrency, err := NormalizeCreditValuationCurrency(sourcePlan.Currency)
			if err != nil {
				return err
			}
			if creditPlan.ValuationCurrency == nil {
				return ErrCreditValuationCurrencyRequired
			}
			valuationCurrency, err := NormalizeCreditValuationCurrency(*creditPlan.ValuationCurrency)
			if err != nil {
				return err
			}
			fxSnapshot, err := CurrentCreditFXRateSnapshot(sourceCurrency, valuationCurrency, dbNow)
			if err != nil {
				return err
			}
			valuationSource = &CreditValuationSourceSnapshot{
				SourcePriceMicros: *sourcePlan.PriceAmountMicros,
				SourcePlanCredit:  quote.CreditBasis,
				GrossCredit:       quote.GrossCredit,
				SourceCurrency:    sourceCurrency,
				ValuationCurrency: valuationCurrency,
				RuleVersion:       CreditValuationRuleVersion,
				FXRateSnapshot:    &fxSnapshot,
			}
		}
		snapshotBytes, err := common.Marshal(quote)
		if err != nil {
			return err
		}
		grant, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
			UserId:                  userId,
			GrossCredit:             quote.GrossCredit,
			IdempotencyKey:          fmt.Sprintf("subscription_conversion:%d", sourceSubscriptionId),
			SourceType:              CreditBalanceLedgerSourceSubscriptionConversion,
			SourceId:                sourceSubscriptionId,
			SourceSnapshot:          string(snapshotBytes),
			Type:                    CreditBalanceLedgerTypeSubscriptionConversion,
			TargetPlanId:            creditPlan.Id,
			Reason:                  "计时套餐转换为 Credit 余额",
			PreserveActiveSelection: true,
			ValuationSource:         valuationSource,
		})
		if err != nil {
			return err
		}

		conversion := &SubscriptionConversion{
			UserId: userId, IdempotencyKey: idempotencyKey,
			SourceSubscriptionId: sourceSubscriptionId, SourcePlanId: quote.PlanId, SourcePlanTitle: quote.PlanTitle,
			TargetSubscriptionId: grant.UserSubscriptionId, TargetPlanId: grant.PlanId, LedgerId: grant.LedgerId,
			SourceStatus: quote.Status, GrantSource: quote.GrantSource, DatabaseNow: quote.DatabaseNow,
			SourceStartTime: quote.StartTime, SourceEndTime: quote.EndTime, RemainingSeconds: quote.RemainingSeconds,
			Full31DayBlocks: quote.Full31DayBlocks, CreditBasis: quote.CreditBasis, CreditBasisSource: quote.CreditBasisSource,
			CurrentRemainingCredit: quote.CurrentRemainingCredit, GrossCredit: quote.GrossCredit,
			DebtOffset: grant.DebtOffset, NetAvailableCredit: quote.GrossCredit - grant.DebtOffset,
			AvailableCreditAfter: grant.AvailableCredit, SettlementDebtAfter: grant.SettlementDebt,
			BalanceBefore: grant.BalanceBefore, BalanceAfter: grant.BalanceAfter,
			LastGrantedAt: quote.LastGrantedAt, LastGrantTimeSource: quote.LastGrantTimeSource, LastGrantSource: quote.LastGrantSource,
			ConvertedAt: dbNow, CreatedAt: dbNow,
		}
		if valuationSource != nil {
			var ledger CreditBalanceLedger
			if err := tx.Where("id = ?", grant.LedgerId).First(&ledger).Error; err != nil {
				return err
			}
			conversion.ValuationCurrency = ledger.ValuationCurrency
			conversion.ValuationSourcePriceMicros = valuationSource.SourcePriceMicros
			conversion.ValuationCreditBasis = valuationSource.SourcePlanCredit
			conversion.ValuationGrossCostMicros = ledger.ValuationGrossCostMicros
			conversion.ValuationNetCostMicros = ledger.ValuationNetCostMicros
			conversion.ValuationConfidence = ledger.ValuationConfidence
			conversion.ValuationRuleVersion = ledger.ValuationRuleVersion
			conversion.ValuationStateVersionAfter = ledger.ValuationStateVersionAfter
			conversion.FxSourceCurrency = ledger.FxSourceCurrency
			conversion.FxRateNumerator = ledger.FxRateNumerator
			conversion.FxRateDenominator = ledger.FxRateDenominator
			conversion.FxCapturedAt = ledger.FxCapturedAt
		}
		if err := tx.Create(conversion).Error; err != nil {
			return err
		}
		if valuationSource != nil {
			if err := freezeTimedConversionInFlightRequestsTx(
				tx,
				sourceSubscriptionId,
				grant.UserSubscriptionId,
				valuationSource,
			); err != nil {
				return err
			}
		}
		statusUpdate := tx.Model(&UserSubscription{}).
			Where("id = ? AND user_id = ? AND status = ?", sourceSubscriptionId, userId, source.Status).
			Updates(map[string]any{
				"status":                       SubscriptionStatusConverted,
				"converted_at":                 dbNow,
				"conversion_id":                conversion.Id,
				"converted_to_subscription_id": grant.UserSubscriptionId,
				"updated_at":                   dbNow,
			})
		if statusUpdate.Error != nil {
			return statusUpdate.Error
		}
		if statusUpdate.RowsAffected != 1 {
			return errors.New("source subscription changed during conversion")
		}

		setting := user.GetSetting()
		if source.Status == SubscriptionStatusActive && setting.ActiveSubscriptionId == sourceSubscriptionId {
			setting.ActiveSubscriptionId = grant.UserSubscriptionId
			settingBytes, err := common.Marshal(setting)
			if err != nil {
				return err
			}
			if err := tx.Model(&User{}).Where("id = ?", userId).Update("setting", string(settingBytes)).Error; err != nil {
				return err
			}
		}
		result = &SubscriptionConversionResult{Conversion: conversion, Replayed: false}
		return nil
	}

	err := transactionWithUserSettingCASRetry(run)
	if err != nil {
		if errors.Is(err, ErrConversionIdempotencyConflict) {
			return nil, err
		}
		replay, replayErr := findCommittedSubscriptionConversion(userId, sourceSubscriptionId, idempotencyKey)
		if replayErr == nil && replay != nil {
			return &SubscriptionConversionResult{Conversion: replay, Replayed: true}, nil
		}
		if errors.Is(replayErr, ErrConversionIdempotencyConflict) {
			return nil, replayErr
		}
		return nil, err
	}
	primaryBillableSubscriptionCache.Delete(primaryBillableSubscriptionCacheKey(userId))
	if err := InvalidateUserCache(userId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate subscription conversion user cache for user %d: %s", userId, err.Error()))
	}
	return result, nil
}

func guardSubscriptionConversionPlansTx(tx *gorm.DB, sourcePlanId int, targetPlanId int) error {
	if tx == nil || sourcePlanId <= 0 || targetPlanId <= 0 || sourcePlanId == targetPlanId {
		return errors.New("invalid subscription conversion plan guard")
	}
	sourceGuard := tx.Model(&SubscriptionPlan{}).
		Where("id = ? AND enabled = ? AND timed_conversion_enabled = ?", sourcePlanId, true, true).
		UpdateColumn("conversion_guard_version", gorm.Expr("conversion_guard_version + ?", 1))
	if sourceGuard.Error != nil {
		return sourceGuard.Error
	}
	if sourceGuard.RowsAffected != 1 {
		return fmt.Errorf("subscription conversion rejected: %s", ConversionQuoteReasonPlanDisabled)
	}
	targetGuard := tx.Model(&SubscriptionPlan{}).
		Where("id = ? AND enabled = ? AND credit_balance_configured = ? AND credit_balance_conversion_enabled = ?", targetPlanId, true, true, true).
		UpdateColumn("conversion_guard_version", gorm.Expr("conversion_guard_version + ?", 1))
	if targetGuard.Error != nil {
		return targetGuard.Error
	}
	if targetGuard.RowsAffected != 1 {
		return fmt.Errorf("subscription conversion rejected: %s", ConversionQuoteReasonGlobalDisabled)
	}
	return nil
}

func validateSubscriptionConversionReplayFactsTx(tx *gorm.DB, conversion *SubscriptionConversion) error {
	if tx == nil || conversion == nil {
		return ErrConversionIdempotencyConflict
	}
	if conversion.ValuationRuleVersion == 0 {
		return nil
	}
	var sourcePlan SubscriptionPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", conversion.SourcePlanId).First(&sourcePlan).Error; err != nil {
		return ErrConversionIdempotencyConflict
	}
	if sourcePlan.PriceAmountMicros == nil || *sourcePlan.PriceAmountMicros != conversion.ValuationSourcePriceMicros {
		return ErrConversionIdempotencyConflict
	}
	sourceCurrency, err := NormalizeCreditValuationCurrency(sourcePlan.Currency)
	if err != nil || sourceCurrency != conversion.FxSourceCurrency {
		return ErrConversionIdempotencyConflict
	}
	if conversion.ValuationCreditBasis != conversion.CreditBasis || conversion.GrossCredit <= 0 {
		return ErrConversionIdempotencyConflict
	}
	return nil
}

func freezeTimedConversionInFlightRequestsTx(tx *gorm.DB, sourceSubscriptionId int, valuationSubscriptionId int, valuationSource *CreditValuationSourceSnapshot) error {
	if tx == nil || sourceSubscriptionId <= 0 || valuationSubscriptionId <= 0 || valuationSource == nil || valuationSource.FXRateSnapshot == nil {
		return ErrCreditValuationSourceInvalid
	}
	var records []SubscriptionPreConsumeRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_subscription_id = ? AND status = ? AND finalized_at = 0 AND valuation_subscription_id = 0", sourceSubscriptionId, "consumed").
		Order("id asc").
		Find(&records).Error; err != nil {
		return err
	}
	for index := range records {
		record := &records[index]
		if record.PreConsumed <= 0 || record.AppliedCredit != 0 {
			return ErrCreditValuationTargetConflict
		}
		sourceCostMicros, err := mulDivFloor(valuationSource.SourcePriceMicros, record.PreConsumed, valuationSource.SourcePlanCredit)
		if err != nil {
			return err
		}
		exactCostMicros, err := valuationSource.FXRateSnapshot.ConvertMicros(sourceCostMicros)
		if err != nil {
			return err
		}
		now := getDBTimestampTx(tx)
		updated := tx.Model(&SubscriptionPreConsumeRecord{}).
			Where("id = ? AND applied_credit = 0 AND valuation_subscription_id = 0 AND finalized_at = 0", record.Id).
			Updates(map[string]any{
				"applied_credit":                 record.PreConsumed,
				"deducted_available_credit":      record.PreConsumed,
				"valuation_subscription_id":      valuationSubscriptionId,
				"deducted_exact_cost_micros":     exactCostMicros,
				"deducted_estimated_cost_micros": 0,
				"deducted_unknown_credit":        0,
				"valuation_rule_version":         CreditValuationRuleVersion,
				"settlement_version":             1,
				"updated_at":                     now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrCreditValuationTargetConflict
		}
	}
	return nil
}

func subscriptionConversionRejection(quote *TimedSubscriptionConversionQuote) error {
	if quote == nil {
		return errors.New("subscription conversion rejected")
	}
	if quote.CalculationErrorCode != "" {
		return fmt.Errorf("subscription conversion rejected: %s", quote.CalculationErrorCode)
	}
	return fmt.Errorf("subscription conversion rejected: %s", strings.Join(quote.ReasonCodes, ","))
}

func findSubscriptionConversionByIdempotencyTx(tx *gorm.DB, userId int, idempotencyKey string) (*SubscriptionConversion, bool, error) {
	var conversion SubscriptionConversion
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND idempotency_key = ?", userId, idempotencyKey).
		Limit(1).Find(&conversion)
	if query.Error != nil || query.RowsAffected == 0 {
		return &conversion, false, query.Error
	}
	if err := hydrateSubscriptionConversionValuationStateVersionTx(tx, &conversion); err != nil {
		return nil, false, err
	}
	return &conversion, true, nil
}

func findSubscriptionConversionBySourceTx(tx *gorm.DB, sourceSubscriptionId int) (*SubscriptionConversion, bool, error) {
	var conversion SubscriptionConversion
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("source_subscription_id = ?", sourceSubscriptionId).
		Limit(1).Find(&conversion)
	if query.Error != nil || query.RowsAffected == 0 {
		return &conversion, false, query.Error
	}
	if err := hydrateSubscriptionConversionValuationStateVersionTx(tx, &conversion); err != nil {
		return nil, false, err
	}
	return &conversion, true, nil
}

func hydrateSubscriptionConversionValuationStateVersionTx(tx *gorm.DB, conversion *SubscriptionConversion) error {
	if tx == nil || conversion == nil || conversion.LedgerId <= 0 {
		return nil
	}
	var ledger struct {
		ValuationStateVersionAfter int64
	}
	query := tx.Model(&CreditBalanceLedger{}).
		Select("valuation_state_version_after").
		Where("id = ?", conversion.LedgerId).
		Limit(1).Find(&ledger)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected > 0 {
		conversion.ValuationStateVersionAfter = ledger.ValuationStateVersionAfter
	}
	return nil
}

func findCommittedSubscriptionConversion(userId int, sourceSubscriptionId int, idempotencyKey string) (*SubscriptionConversion, error) {
	var conversion *SubscriptionConversion
	err := DB.Transaction(func(tx *gorm.DB) error {
		replay, found, err := findSubscriptionConversionByIdempotencyTx(tx, userId, idempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if replay.SourceSubscriptionId != sourceSubscriptionId {
				return ErrConversionIdempotencyConflict
			}
			if err := validateSubscriptionConversionReplayFactsTx(tx, replay); err != nil {
				return err
			}
			conversion = replay
			return nil
		}

		if _, found, err := findSubscriptionConversionBySourceTx(tx, sourceSubscriptionId); err != nil {
			return err
		} else if found {
			return ErrConversionIdempotencyConflict
		}
		return gorm.ErrRecordNotFound
	})
	if err != nil {
		return nil, err
	}
	return conversion, nil
}

func ListSubscriptionConversions(userId int, limit int) ([]SubscriptionConversion, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var conversions []SubscriptionConversion
	if err := DB.Model(&SubscriptionConversion{}).
		Select("subscription_conversions.*, credit_balance_ledgers.valuation_state_version_after AS valuation_state_version_after").
		Joins("LEFT JOIN credit_balance_ledgers ON credit_balance_ledgers.id = subscription_conversions.ledger_id").
		Where("subscription_conversions.user_id = ?", userId).
		Order("subscription_conversions.id desc").Limit(limit).Find(&conversions).Error; err != nil {
		return nil, err
	}
	return conversions, nil
}
