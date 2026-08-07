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
	Id                                int    `json:"id"`
	UserId                            int    `json:"user_id" gorm:"not null;index;uniqueIndex:idx_subscription_conversion_user_key,priority:1"`
	IdempotencyKey                    string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex:idx_subscription_conversion_user_key,priority:2"`
	ParameterFingerprint              string `json:"-" gorm:"type:varchar(64);not null;default:''"`
	SourceSubscriptionId              int    `json:"source_subscription_id" gorm:"not null;uniqueIndex:idx_subscription_conversions_source_subscription_id"`
	SourcePlanId                      int    `json:"source_plan_id" gorm:"not null;index"`
	SourcePlanTitle                   string `json:"source_plan_title" gorm:"type:varchar(255);not null"`
	TargetSubscriptionId              int    `json:"target_subscription_id" gorm:"not null;index"`
	TargetPlanId                      int    `json:"target_plan_id" gorm:"not null;index"`
	LedgerId                          int    `json:"ledger_id" gorm:"not null;uniqueIndex"`
	SourceStatus                      string `json:"source_status" gorm:"type:varchar(32);not null"`
	GrantSource                       string `json:"grant_source" gorm:"type:varchar(32);not null"`
	DatabaseNow                       int64  `json:"database_now" gorm:"type:bigint;not null"`
	SourceStartTime                   int64  `json:"source_start_time" gorm:"type:bigint;not null"`
	SourceEndTime                     int64  `json:"source_end_time" gorm:"type:bigint;not null"`
	SourceTokenLimit                  int64  `json:"source_token_limit,string" gorm:"type:bigint;not null;default:0"`
	SourceTokenUsed                   int64  `json:"source_token_used,string" gorm:"type:bigint;not null;default:0"`
	SourceDurationUnit                string `json:"source_duration_unit" gorm:"type:varchar(16);not null;default:''"`
	SourceDurationValue               int    `json:"source_duration_value" gorm:"not null;default:0"`
	SourceCustomSeconds               int64  `json:"source_custom_seconds,string" gorm:"type:bigint;not null;default:0"`
	SourceQuotaResetPeriod            string `json:"source_quota_reset_period" gorm:"type:varchar(16);not null;default:''"`
	SourceQuotaResetCustomSeconds     int64  `json:"source_quota_reset_custom_seconds,string" gorm:"type:bigint;not null;default:0"`
	RemainingSeconds                  int64  `json:"remaining_seconds" gorm:"type:bigint;not null"`
	Full31DayBlocks                   int64  `json:"full_31_day_blocks" gorm:"type:bigint;not null"`
	CreditBasis                       int64  `json:"credit_basis" gorm:"type:bigint;not null"`
	CreditBasisSource                 string `json:"credit_basis_source" gorm:"type:varchar(32);not null"`
	CurrentRemainingCredit            int64  `json:"current_remaining_credit" gorm:"type:bigint;not null"`
	GrossCredit                       int64  `json:"gross_credit" gorm:"type:bigint;not null"`
	DebtOffset                        int64  `json:"debt_offset" gorm:"type:bigint;not null"`
	NetAvailableCredit                int64  `json:"net_available_credit" gorm:"type:bigint;not null"`
	AvailableCreditAfter              int64  `json:"available_credit_after" gorm:"type:bigint;not null"`
	SettlementDebtAfter               int64  `json:"settlement_debt_after" gorm:"type:bigint;not null"`
	BalanceBefore                     int64  `json:"balance_before" gorm:"type:bigint;not null"`
	BalanceAfter                      int64  `json:"balance_after" gorm:"type:bigint;not null"`
	LastGrantedAt                     int64  `json:"last_granted_at" gorm:"type:bigint;not null"`
	LastGrantTimeSource               string `json:"last_grant_time_source" gorm:"type:varchar(64);not null"`
	LastGrantSource                   string `json:"last_grant_source" gorm:"type:varchar(32);not null"`
	ConvertedAt                       int64  `json:"converted_at" gorm:"type:bigint;not null;index"`
	ValuationCurrency                 string `json:"valuation_currency" gorm:"type:varchar(8);not null;default:''"`
	ValuationSourcePriceMicros        int64  `json:"valuation_source_price_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationCreditBasis              int64  `json:"valuation_credit_basis" gorm:"type:bigint;not null;default:0"`
	ValuationUnitValueNumeratorMicros int64  `json:"valuation_unit_value_numerator_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationUnitValueDenominator     int64  `json:"valuation_unit_value_denominator,string" gorm:"type:bigint;not null;default:0"`
	ValuationGrossCostMicros          int64  `json:"valuation_gross_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationNetCostMicros            int64  `json:"valuation_net_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationConfidence               string `json:"valuation_confidence" gorm:"type:varchar(16);not null;default:''"`
	ValuationRuleVersion              int    `json:"valuation_rule_version" gorm:"not null;default:0"`
	ValuationStateVersionAfter        int64  `json:"-" gorm:"->;-:migration"`
	FxSourceCurrency                  string `json:"fx_source_currency" gorm:"type:varchar(8);not null;default:''"`
	FxRateNumerator                   int64  `json:"fx_rate_numerator,string" gorm:"type:bigint;not null;default:0"`
	FxRateDenominator                 int64  `json:"fx_rate_denominator,string" gorm:"type:bigint;not null;default:0"`
	FxCapturedAt                      int64  `json:"fx_captured_at" gorm:"type:bigint;not null;default:0"`
	CreatedAt                         int64  `json:"created_at" gorm:"type:bigint;not null"`
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
			return fmt.Errorf("%w: source subscription already converted", ErrConversionIdempotencyConflict)
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
		var sourcePlan SubscriptionPlan
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", quote.PlanId).First(&sourcePlan).Error; err != nil {
			return err
		}
		var valuationSource *CreditValuationSourceSnapshot
		valuationReady, err := CreditValuationRuntimeReadyTx(tx)
		if err != nil {
			return err
		}
		if valuationReady {
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
		var inFlightRequests []timedConversionInFlightRequest
		if valuationSource != nil {
			inFlightRequests, err = prepareTimedConversionInFlightRequestsTx(tx, sourceSubscriptionId)
			if err != nil {
				return err
			}
		}
		snapshotBytes, err := common.Marshal(quote)
		if err != nil {
			return err
		}
		grant, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
			UserId:          userId,
			GrossCredit:     quote.GrossCredit,
			IdempotencyKey:  fmt.Sprintf("subscription_conversion:%d", sourceSubscriptionId),
			SourceType:      CreditBalanceLedgerSourceSubscriptionConversion,
			SourceId:        sourceSubscriptionId,
			SourceSnapshot:  string(snapshotBytes),
			Type:            CreditBalanceLedgerTypeSubscriptionConversion,
			TargetPlanId:    creditPlan.Id,
			Reason:          "计时套餐转换为 Credit 余额",
			ValuationSource: valuationSource,
			ConversionSource: &CreditBalanceConversionSourceFacts{
				ConversionIdempotencyKey: idempotencyKey,
				SourcePlanId:             sourcePlan.Id,
				SourceTokenLimit:         source.TokenLimit,
				SourceTokenUsed:          source.TokenUsed,
				SourceStatus:             source.Status,
				SourceStartTime:          source.StartTime,
				SourceEndTime:            source.EndTime,
				Full31DayBlocks:          quote.Full31DayBlocks,
				CurrentRemainingCredit:   quote.CurrentRemainingCredit,
				CreditBasisSource:        quote.CreditBasisSource,
				DurationUnit:             sourcePlan.DurationUnit,
				DurationValue:            sourcePlan.DurationValue,
				CustomSeconds:            sourcePlan.CustomSeconds,
				QuotaResetPeriod:         sourcePlan.QuotaResetPeriod,
				QuotaResetCustomSeconds:  sourcePlan.QuotaResetCustomSeconds,
			},
			PreserveActiveSelection: true,
		})
		if err != nil {
			return err
		}
		if valuationSource != nil {
			if err := applyTimedConversionInFlightRequestsTx(
				tx,
				inFlightRequests,
				grant.UserSubscriptionId,
				valuationSource,
			); err != nil {
				return err
			}
		}

		var ledger CreditBalanceLedger
		if err := tx.Where("id = ?", grant.LedgerId).First(&ledger).Error; err != nil {
			return err
		}
		conversion := &SubscriptionConversion{
			UserId: userId, IdempotencyKey: idempotencyKey, ParameterFingerprint: ledger.ParameterFingerprint,
			SourceSubscriptionId: sourceSubscriptionId, SourcePlanId: quote.PlanId, SourcePlanTitle: quote.PlanTitle,
			TargetSubscriptionId: grant.UserSubscriptionId, TargetPlanId: grant.PlanId, LedgerId: grant.LedgerId,
			SourceStatus: quote.Status, GrantSource: quote.GrantSource, DatabaseNow: quote.DatabaseNow,
			SourceStartTime: quote.StartTime, SourceEndTime: quote.EndTime,
			SourceTokenLimit: source.TokenLimit, SourceTokenUsed: source.TokenUsed,
			SourceDurationUnit: sourcePlan.DurationUnit, SourceDurationValue: sourcePlan.DurationValue,
			SourceCustomSeconds: sourcePlan.CustomSeconds, SourceQuotaResetPeriod: NormalizeResetPeriod(sourcePlan.QuotaResetPeriod),
			SourceQuotaResetCustomSeconds: sourcePlan.QuotaResetCustomSeconds,
			RemainingSeconds:              quote.RemainingSeconds,
			Full31DayBlocks:               quote.Full31DayBlocks, CreditBasis: quote.CreditBasis, CreditBasisSource: quote.CreditBasisSource,
			CurrentRemainingCredit: quote.CurrentRemainingCredit, GrossCredit: quote.GrossCredit,
			DebtOffset: grant.DebtOffset, NetAvailableCredit: quote.GrossCredit - grant.DebtOffset,
			AvailableCreditAfter: grant.AvailableCredit, SettlementDebtAfter: grant.SettlementDebt,
			BalanceBefore: grant.BalanceBefore, BalanceAfter: grant.BalanceAfter,
			LastGrantedAt: quote.LastGrantedAt, LastGrantTimeSource: quote.LastGrantTimeSource, LastGrantSource: quote.LastGrantSource,
			ConvertedAt: dbNow, CreatedAt: dbNow,
		}
		if valuationSource != nil {
			conversion.ValuationCurrency = ledger.ValuationCurrency
			conversion.ValuationSourcePriceMicros = ledger.ValuationSourcePriceMicros
			conversion.ValuationCreditBasis = ledger.ValuationCreditBasis
			conversion.ValuationUnitValueNumeratorMicros = ledger.ValuationUnitValueNumeratorMicros
			conversion.ValuationUnitValueDenominator = ledger.ValuationUnitValueDenominator
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
		return fmt.Errorf("%w: %s", ErrConversionIneligible, ConversionQuoteReasonPlanDisabled)
	}
	targetGuard := tx.Model(&SubscriptionPlan{}).
		Where("id = ? AND enabled = ? AND credit_balance_configured = ? AND credit_balance_conversion_enabled = ?", targetPlanId, true, true, true).
		UpdateColumn("conversion_guard_version", gorm.Expr("conversion_guard_version + ?", 1))
	if targetGuard.Error != nil {
		return targetGuard.Error
	}
	if targetGuard.RowsAffected != 1 {
		return fmt.Errorf("%w: %s", ErrConversionIneligible, ConversionQuoteReasonGlobalDisabled)
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
	if conversion.ValuationRuleVersion != CreditValuationRuleVersion || conversion.ParameterFingerprint == "" {
		return ErrConversionIdempotencyConflict
	}

	var source UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", conversion.SourceSubscriptionId, conversion.UserId).
		First(&source).Error; err != nil {
		return ErrConversionIdempotencyConflict
	}
	if source.PlanId != conversion.SourcePlanId ||
		source.Status != SubscriptionStatusConverted ||
		source.ConversionId != conversion.Id ||
		source.ConvertedToSubscriptionId != conversion.TargetSubscriptionId ||
		source.TokenLimit != conversion.SourceTokenLimit ||
		source.TokenUsed != conversion.SourceTokenUsed ||
		source.StartTime != conversion.SourceStartTime ||
		source.EndTime != conversion.SourceEndTime {
		return ErrConversionIdempotencyConflict
	}

	var sourcePlan SubscriptionPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", conversion.SourcePlanId).First(&sourcePlan).Error; err != nil {
		return ErrConversionIdempotencyConflict
	}
	if sourcePlan.PriceAmountMicros == nil ||
		*sourcePlan.PriceAmountMicros != conversion.ValuationSourcePriceMicros ||
		sourcePlan.DurationUnit != conversion.SourceDurationUnit ||
		sourcePlan.DurationValue != conversion.SourceDurationValue ||
		sourcePlan.CustomSeconds != conversion.SourceCustomSeconds ||
		NormalizeResetPeriod(sourcePlan.QuotaResetPeriod) != conversion.SourceQuotaResetPeriod ||
		sourcePlan.QuotaResetCustomSeconds != conversion.SourceQuotaResetCustomSeconds {
		return ErrConversionIdempotencyConflict
	}
	sourceCurrency, err := NormalizeCreditValuationCurrency(sourcePlan.Currency)
	if err != nil || sourceCurrency != conversion.FxSourceCurrency {
		return ErrConversionIdempotencyConflict
	}

	creditBasis := sourcePlan.MonthlyTokenLimit
	creditBasisSource := ConversionCreditBasisCurrentPlan
	if source.LastGrantCreditSnapshot != nil {
		creditBasis = *source.LastGrantCreditSnapshot
		creditBasisSource = ConversionCreditBasisGrantSnapshot
	}
	currentRemaining, ok := checkedNonNegativeDifference(source.TokenLimit, source.TokenUsed)
	if !ok {
		return ErrConversionIdempotencyConflict
	}
	remainingSeconds, ok := checkedNonNegativeDifference(source.EndTime, conversion.DatabaseNow)
	if !ok {
		return ErrConversionIdempotencyConflict
	}
	full31DayBlocks := remainingSeconds / TimedSubscriptionConversionBlockSeconds
	blockCredit, ok := checkedMulNonNegativeInt64(full31DayBlocks, creditBasis)
	if !ok {
		return ErrConversionIdempotencyConflict
	}
	grossCredit, ok := checkedAddInt64(blockCredit, currentRemaining)
	if !ok || grossCredit <= 0 ||
		creditBasis != conversion.CreditBasis ||
		creditBasis != conversion.ValuationCreditBasis ||
		creditBasisSource != conversion.CreditBasisSource ||
		currentRemaining != conversion.CurrentRemainingCredit ||
		remainingSeconds != conversion.RemainingSeconds ||
		full31DayBlocks != conversion.Full31DayBlocks ||
		grossCredit != conversion.GrossCredit {
		return ErrConversionIdempotencyConflict
	}

	var target UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", conversion.TargetSubscriptionId, conversion.UserId).
		First(&target).Error; err != nil {
		return ErrConversionIdempotencyConflict
	}
	if target.PlanId != conversion.TargetPlanId || target.EntitlementType != SubscriptionEntitlementCreditBalance {
		return ErrConversionIdempotencyConflict
	}

	var ledger CreditBalanceLedger
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", conversion.LedgerId).First(&ledger).Error; err != nil {
		return ErrConversionIdempotencyConflict
	}
	if ledger.UserId != conversion.UserId ||
		ledger.UserSubscriptionId != conversion.TargetSubscriptionId ||
		ledger.SourceType != CreditBalanceLedgerSourceSubscriptionConversion ||
		ledger.SourceId != conversion.SourceSubscriptionId ||
		ledger.Type != CreditBalanceLedgerTypeSubscriptionConversion ||
		ledger.SourcePlanId != conversion.SourcePlanId ||
		ledger.TargetPlanId != conversion.TargetPlanId ||
		ledger.SourceTokenLimit != conversion.SourceTokenLimit ||
		ledger.SourceTokenUsed != conversion.SourceTokenUsed ||
		ledger.SourceStatus != conversion.SourceStatus ||
		ledger.SourceStartTime != conversion.SourceStartTime ||
		ledger.SourceEndTime != conversion.SourceEndTime ||
		ledger.Full31DayBlocks != conversion.Full31DayBlocks ||
		ledger.CurrentRemainingCredit != conversion.CurrentRemainingCredit ||
		ledger.CreditBasisSource != conversion.CreditBasisSource ||
		ledger.SourceDurationUnit != conversion.SourceDurationUnit ||
		ledger.SourceDurationValue != conversion.SourceDurationValue ||
		ledger.SourceCustomSeconds != conversion.SourceCustomSeconds ||
		ledger.SourceQuotaResetPeriod != conversion.SourceQuotaResetPeriod ||
		ledger.SourceQuotaResetCustomSeconds != conversion.SourceQuotaResetCustomSeconds ||
		ledger.GrossCredit != conversion.GrossCredit ||
		ledger.DebtOffset != conversion.DebtOffset ||
		ledger.NetGrantedCredit != conversion.NetAvailableCredit ||
		ledger.ValuationSourcePriceMicros != conversion.ValuationSourcePriceMicros ||
		ledger.ValuationCreditBasis != conversion.ValuationCreditBasis ||
		ledger.ValuationUnitValueNumeratorMicros != conversion.ValuationUnitValueNumeratorMicros ||
		ledger.ValuationUnitValueDenominator != conversion.ValuationUnitValueDenominator ||
		ledger.ValuationCurrency != conversion.ValuationCurrency ||
		ledger.ValuationGrossCostMicros != conversion.ValuationGrossCostMicros ||
		ledger.ValuationNetCostMicros != conversion.ValuationNetCostMicros ||
		ledger.ValuationConfidence != conversion.ValuationConfidence ||
		ledger.ValuationRuleVersion != conversion.ValuationRuleVersion ||
		ledger.FxSourceCurrency != conversion.FxSourceCurrency ||
		ledger.FxRateNumerator != conversion.FxRateNumerator ||
		ledger.FxRateDenominator != conversion.FxRateDenominator ||
		ledger.FxCapturedAt != conversion.FxCapturedAt ||
		ledger.ParameterFingerprint != conversion.ParameterFingerprint {
		return ErrConversionIdempotencyConflict
	}
	unitValueNumerator, unitValueDenominator, err := creditValuationUnitValueRatio(
		*sourcePlan.PriceAmountMicros,
		creditBasis,
		conversion.FxRateNumerator,
		conversion.FxRateDenominator,
	)
	if err != nil ||
		unitValueNumerator != conversion.ValuationUnitValueNumeratorMicros ||
		unitValueDenominator != conversion.ValuationUnitValueDenominator {
		return ErrConversionIdempotencyConflict
	}

	fingerprint, err := creditBalanceConversionParameterFingerprint(CreditBalanceGrantRequest{
		UserId:       conversion.UserId,
		GrossCredit:  grossCredit,
		SourceId:     conversion.SourceSubscriptionId,
		TargetPlanId: conversion.TargetPlanId,
		ValuationSource: &CreditValuationSourceSnapshot{
			SourcePriceMicros: *sourcePlan.PriceAmountMicros,
			SourcePlanCredit:  creditBasis,
			GrossCredit:       grossCredit,
		},
		ConversionSource: &CreditBalanceConversionSourceFacts{
			ConversionIdempotencyKey: conversion.IdempotencyKey,
			SourcePlanId:             conversion.SourcePlanId,
			SourceTokenLimit:         source.TokenLimit,
			SourceTokenUsed:          source.TokenUsed,
			SourceStatus:             conversion.SourceStatus,
			SourceStartTime:          source.StartTime,
			SourceEndTime:            source.EndTime,
			Full31DayBlocks:          full31DayBlocks,
			CurrentRemainingCredit:   currentRemaining,
			CreditBasisSource:        creditBasisSource,
			DurationUnit:             sourcePlan.DurationUnit,
			DurationValue:            sourcePlan.DurationValue,
			CustomSeconds:            sourcePlan.CustomSeconds,
			QuotaResetPeriod:         sourcePlan.QuotaResetPeriod,
			QuotaResetCustomSeconds:  sourcePlan.QuotaResetCustomSeconds,
		},
	}, creditValuationIngress{
		currency:                 conversion.ValuationCurrency,
		ruleVersion:              conversion.ValuationRuleVersion,
		fxSourceCurrency:         conversion.FxSourceCurrency,
		fxRateNumerator:          conversion.FxRateNumerator,
		fxRateDenominator:        conversion.FxRateDenominator,
		fxCapturedAt:             conversion.FxCapturedAt,
		unitValueNumeratorMicros: unitValueNumerator,
		unitValueDenominator:     unitValueDenominator,
	})
	if err != nil || fingerprint != conversion.ParameterFingerprint {
		return ErrConversionIdempotencyConflict
	}
	return nil
}

type timedConversionInFlightRequest struct {
	id          int
	requestID   string
	preConsumed int64
}

func prepareTimedConversionInFlightRequestsTx(tx *gorm.DB, sourceSubscriptionId int) ([]timedConversionInFlightRequest, error) {
	if tx == nil || sourceSubscriptionId <= 0 {
		return nil, ErrCreditValuationSourceInvalid
	}
	var records []SubscriptionPreConsumeRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_subscription_id = ? AND status = ? AND finalized_at = 0 AND valuation_subscription_id = 0", sourceSubscriptionId, "consumed").
		Order("request_id asc").Order("id asc").
		Find(&records).Error; err != nil {
		return nil, err
	}
	requests := make([]timedConversionInFlightRequest, len(records))
	for index := range records {
		record := records[index]
		if record.Id <= 0 || strings.TrimSpace(record.RequestId) == "" || record.PreConsumed <= 0 || record.AppliedCredit != 0 || record.ValuationSubscriptionId != 0 || record.FinalizedAt != 0 {
			return nil, ErrCreditValuationTargetConflict
		}
		requests[index] = timedConversionInFlightRequest{
			id:          record.Id,
			requestID:   record.RequestId,
			preConsumed: record.PreConsumed,
		}
	}
	return requests, nil
}

func applyTimedConversionInFlightRequestsTx(tx *gorm.DB, requests []timedConversionInFlightRequest, valuationSubscriptionId int, valuationSource *CreditValuationSourceSnapshot) error {
	if tx == nil || valuationSubscriptionId <= 0 || valuationSource == nil || valuationSource.FXRateSnapshot == nil {
		return ErrCreditValuationSourceInvalid
	}
	for _, request := range requests {
		if request.id <= 0 || strings.TrimSpace(request.requestID) == "" || request.preConsumed <= 0 {
			return ErrCreditValuationTargetConflict
		}
		sourceCostMicros, err := mulDivFloor(valuationSource.SourcePriceMicros, request.preConsumed, valuationSource.SourcePlanCredit)
		if err != nil {
			return err
		}
		exactCostMicros, err := valuationSource.FXRateSnapshot.ConvertMicros(sourceCostMicros)
		if err != nil {
			return err
		}
		now := getDBTimestampTx(tx)
		updated := tx.Model(&SubscriptionPreConsumeRecord{}).
			Where("id = ? AND request_id = ? AND status = ? AND applied_credit = 0 AND valuation_subscription_id = 0 AND finalized_at = 0", request.id, request.requestID, "consumed").
			Updates(map[string]any{
				"applied_credit":                 request.preConsumed,
				"deducted_available_credit":      request.preConsumed,
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
		return ErrConversionIneligible
	}
	if quote.CalculationErrorCode != "" {
		return fmt.Errorf("%w: %s", ErrConversionIneligible, quote.CalculationErrorCode)
	}
	return fmt.Errorf("%w: %s", ErrConversionIneligible, strings.Join(quote.ReasonCodes, ","))
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
