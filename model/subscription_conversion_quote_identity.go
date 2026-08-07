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
	subscriptionConversionQuoteFactsVersion = 1
	subscriptionConversionQuoteTTLSeconds   = int64(5 * 60)
)

// SubscriptionConversionQuote persists the server-issued identity and canonical
// authoritative facts reviewed by the user before conversion confirmation.
type SubscriptionConversionQuote struct {
	QuoteId              string `json:"quote_id" gorm:"type:varchar(64);primaryKey"`
	UserId               int    `json:"user_id" gorm:"not null;index:idx_subscription_conversion_quote_owner,priority:1"`
	SourceSubscriptionId int    `json:"source_subscription_id" gorm:"not null;index:idx_subscription_conversion_quote_owner,priority:2"`
	CreatedAt            int64  `json:"created_at" gorm:"type:bigint;not null"`
	ExpiresAt            int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	FactsFingerprint     string `json:"facts_fingerprint" gorm:"type:varchar(64);not null;index"`
	FactsSnapshot        string `json:"-" gorm:"type:text;not null"`
}

type subscriptionConversionQuoteFacts struct {
	Version                       int    `json:"version"`
	UserId                        int    `json:"user_id"`
	SourceSubscriptionId          int    `json:"source_subscription_id"`
	SourcePlanId                  int    `json:"source_plan_id"`
	TargetPlanId                  int    `json:"target_plan_id"`
	TargetSubscriptionId          int    `json:"target_subscription_id"`
	EntitlementType               string `json:"entitlement_type"`
	GrantSource                   string `json:"grant_source"`
	SourceStatus                  string `json:"source_status"`
	SourceStartTime               int64  `json:"source_start_time"`
	SourceEndTime                 int64  `json:"source_end_time"`
	SourceTokenLimit              int64  `json:"source_token_limit"`
	SourceTokenUsed               int64  `json:"source_token_used"`
	SourceConversionId            int    `json:"source_conversion_id"`
	ConvertedToSubscriptionId     int    `json:"converted_to_subscription_id"`
	LastGrantedAt                 int64  `json:"last_granted_at"`
	LastGrantTimeSource           string `json:"last_grant_time_source"`
	LastGrantSource               string `json:"last_grant_source"`
	SourcePlanEnabled             bool   `json:"source_plan_enabled"`
	SourcePlanTimedConversion     bool   `json:"source_plan_timed_conversion"`
	SourcePlanTrial               bool   `json:"source_plan_trial"`
	SourcePlanInviteTrial         bool   `json:"source_plan_invite_trial"`
	SourceDurationUnit            string `json:"source_duration_unit"`
	SourceDurationValue           int    `json:"source_duration_value"`
	SourceCustomSeconds           int64  `json:"source_custom_seconds"`
	SourceQuotaResetPeriod        string `json:"source_quota_reset_period"`
	SourceQuotaResetCustomSeconds int64  `json:"source_quota_reset_custom_seconds"`
	SourcePlanMonthlyCredit       int64  `json:"source_plan_monthly_credit"`
	TargetPlanEnabled             bool   `json:"target_plan_enabled"`
	TargetPlanConfigured          bool   `json:"target_plan_configured"`
	TargetPlanConversionEnabled   bool   `json:"target_plan_conversion_enabled"`
	Full31DayBlocks               int64  `json:"full_31_day_blocks"`
	CreditBasis                   int64  `json:"credit_basis"`
	CreditBasisSource             string `json:"credit_basis_source"`
	CurrentRemainingCredit        int64  `json:"current_remaining_credit"`
	GrossCredit                   int64  `json:"gross_credit"`
	CurrentDebt                   int64  `json:"current_debt"`
	EstimatedDebtOffset           int64  `json:"estimated_debt_offset"`
	NetAvailableCredit            int64  `json:"net_available_credit"`
	SourcePriceMicros             int64  `json:"source_price_micros"`
	SourceCurrency                string `json:"source_currency"`
	TargetCurrency                string `json:"target_currency"`
	ValuationCreditBasis          int64  `json:"valuation_credit_basis"`
	GrossCostMicros               int64  `json:"gross_cost_micros"`
	NetCostMicros                 int64  `json:"net_cost_micros"`
	UnitValueNumeratorMicros      int64  `json:"unit_value_numerator_micros"`
	UnitValueDenominator          int64  `json:"unit_value_denominator"`
	RuleVersion                   int    `json:"rule_version"`
	FxRateNumerator               int64  `json:"fx_rate_numerator"`
	FxRateDenominator             int64  `json:"fx_rate_denominator"`
	FxCapturedAt                  int64  `json:"fx_captured_at"`
}

func issueTimedSubscriptionConversionQuoteTx(tx *gorm.DB, quote *TimedSubscriptionConversionQuote, source *UserSubscription) error {
	if tx == nil || quote == nil || source == nil || !quote.CanConfirm {
		return nil
	}
	var sourcePlan SubscriptionPlan
	if err := tx.Where("id = ?", quote.PlanId).First(&sourcePlan).Error; err != nil {
		return err
	}
	targetPlan, err := getConversionCreditBalancePlanTx(tx)
	if err != nil || targetPlan == nil {
		return err
	}
	facts, err := buildSubscriptionConversionQuoteFactsTx(tx, quote, source, &sourcePlan, targetPlan)
	if err != nil {
		return err
	}
	fingerprint, snapshot, err := marshalSubscriptionConversionQuoteFacts(facts)
	if err != nil {
		return err
	}
	expiresAt, ok := checkedAddInt64(quote.DatabaseNow, subscriptionConversionQuoteTTLSeconds)
	if !ok {
		return ErrCreditValuationOverflow
	}
	record := SubscriptionConversionQuote{
		QuoteId:              common.GetUUID(),
		UserId:               source.UserId,
		SourceSubscriptionId: source.Id,
		CreatedAt:            quote.DatabaseNow,
		ExpiresAt:            expiresAt,
		FactsFingerprint:     fingerprint,
		FactsSnapshot:        snapshot,
	}
	if err := tx.Create(&record).Error; err != nil {
		return err
	}
	quote.QuoteId = record.QuoteId
	quote.CreatedAt = record.CreatedAt
	quote.ExpiresAt = record.ExpiresAt
	quote.FactsFingerprint = record.FactsFingerprint
	return nil
}

func lockTimedSubscriptionConversionQuoteTx(tx *gorm.DB, quoteId string, userId int, sourceSubscriptionId int) (*SubscriptionConversionQuote, error) {
	quoteId = strings.TrimSpace(quoteId)
	if tx == nil || quoteId == "" || userId <= 0 || sourceSubscriptionId <= 0 {
		return nil, ErrConversionQuoteStale
	}
	var record SubscriptionConversionQuote
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("quote_id = ? AND user_id = ? AND source_subscription_id = ?", quoteId, userId, sourceSubscriptionId).
		Limit(1).Find(&record)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected != 1 {
		return nil, ErrConversionQuoteStale
	}
	return &record, nil
}

func validateTimedSubscriptionConversionQuoteFactsTx(tx *gorm.DB, record *SubscriptionConversionQuote, dbNow int64, quote *TimedSubscriptionConversionQuote, source *UserSubscription, sourcePlan *SubscriptionPlan, targetPlan *SubscriptionPlan) error {
	if tx == nil || record == nil || quote == nil || source == nil || sourcePlan == nil || targetPlan == nil ||
		record.CreatedAt <= 0 || record.ExpiresAt <= record.CreatedAt || dbNow < record.CreatedAt || dbNow >= record.ExpiresAt || !quote.CanConfirm {
		return ErrConversionQuoteStale
	}
	var quotedFacts subscriptionConversionQuoteFacts
	if err := common.UnmarshalJsonStr(record.FactsSnapshot, &quotedFacts); err != nil {
		return ErrConversionQuoteStale
	}
	quotedFingerprint, _, err := marshalSubscriptionConversionQuoteFacts(quotedFacts)
	if err != nil || quotedFingerprint != record.FactsFingerprint {
		return ErrConversionQuoteStale
	}
	if quote.ValuationSourceCurrency == quote.ValuationCurrency && quotedFacts.SourceCurrency == quotedFacts.TargetCurrency {
		quote.FxCapturedAt = quotedFacts.FxCapturedAt
	}
	currentFacts, err := buildSubscriptionConversionQuoteFactsTx(tx, quote, source, sourcePlan, targetPlan)
	if err != nil {
		return err
	}
	currentFingerprint, _, err := marshalSubscriptionConversionQuoteFacts(currentFacts)
	if err != nil || currentFingerprint != record.FactsFingerprint {
		return ErrConversionQuoteStale
	}
	quote.QuoteId = record.QuoteId
	quote.CreatedAt = record.CreatedAt
	quote.ExpiresAt = record.ExpiresAt
	quote.FactsFingerprint = record.FactsFingerprint
	return nil
}

func buildSubscriptionConversionQuoteFactsTx(tx *gorm.DB, quote *TimedSubscriptionConversionQuote, source *UserSubscription, sourcePlan *SubscriptionPlan, targetPlan *SubscriptionPlan) (subscriptionConversionQuoteFacts, error) {
	if tx == nil || quote == nil || source == nil || sourcePlan == nil || targetPlan == nil {
		return subscriptionConversionQuoteFacts{}, ErrConversionQuoteStale
	}
	var target UserSubscription
	query := tx.Select("id").Where("user_id = ? AND entitlement_type = ?", source.UserId, SubscriptionEntitlementCreditBalance).Limit(1).Find(&target)
	if query.Error != nil {
		return subscriptionConversionQuoteFacts{}, query.Error
	}
	return subscriptionConversionQuoteFacts{
		Version:                       subscriptionConversionQuoteFactsVersion,
		UserId:                        source.UserId,
		SourceSubscriptionId:          source.Id,
		SourcePlanId:                  sourcePlan.Id,
		TargetPlanId:                  targetPlan.Id,
		TargetSubscriptionId:          target.Id,
		EntitlementType:               strings.TrimSpace(source.EntitlementType),
		GrantSource:                   quote.GrantSource,
		SourceStatus:                  strings.TrimSpace(source.Status),
		SourceStartTime:               source.StartTime,
		SourceEndTime:                 source.EndTime,
		SourceTokenLimit:              source.TokenLimit,
		SourceTokenUsed:               source.TokenUsed,
		SourceConversionId:            source.ConversionId,
		ConvertedToSubscriptionId:     source.ConvertedToSubscriptionId,
		LastGrantedAt:                 source.LastGrantedAt,
		LastGrantTimeSource:           strings.TrimSpace(source.LastGrantTimeSource),
		LastGrantSource:               strings.TrimSpace(source.LastGrantSource),
		SourcePlanEnabled:             sourcePlan.Enabled,
		SourcePlanTimedConversion:     sourcePlan.TimedConversionEnabled,
		SourcePlanTrial:               sourcePlan.IsTrial,
		SourcePlanInviteTrial:         sourcePlan.InviteTrial,
		SourceDurationUnit:            sourcePlan.DurationUnit,
		SourceDurationValue:           sourcePlan.DurationValue,
		SourceCustomSeconds:           sourcePlan.CustomSeconds,
		SourceQuotaResetPeriod:        NormalizeResetPeriod(sourcePlan.QuotaResetPeriod),
		SourceQuotaResetCustomSeconds: sourcePlan.QuotaResetCustomSeconds,
		SourcePlanMonthlyCredit:       sourcePlan.MonthlyTokenLimit,
		TargetPlanEnabled:             targetPlan.Enabled,
		TargetPlanConfigured:          targetPlan.CreditBalanceConfigured,
		TargetPlanConversionEnabled:   targetPlan.CreditBalanceConversionEnabled,
		Full31DayBlocks:               quote.Full31DayBlocks,
		CreditBasis:                   quote.CreditBasis,
		CreditBasisSource:             quote.CreditBasisSource,
		CurrentRemainingCredit:        quote.CurrentRemainingCredit,
		GrossCredit:                   quote.GrossCredit,
		CurrentDebt:                   quote.CurrentDebt,
		EstimatedDebtOffset:           quote.EstimatedDebtOffset,
		NetAvailableCredit:            quote.NetAvailableCredit,
		SourcePriceMicros:             quote.ValuationSourcePriceMicros,
		SourceCurrency:                quote.ValuationSourceCurrency,
		TargetCurrency:                quote.ValuationCurrency,
		ValuationCreditBasis:          quote.ValuationCreditBasis,
		GrossCostMicros:               quote.ValuationGrossCostMicros,
		NetCostMicros:                 quote.ValuationNetCostMicros,
		UnitValueNumeratorMicros:      quote.ValuationUnitValueNumeratorMicros,
		UnitValueDenominator:          quote.ValuationUnitValueDenominator,
		RuleVersion:                   quote.ValuationRuleVersion,
		FxRateNumerator:               quote.FxRateNumerator,
		FxRateDenominator:             quote.FxRateDenominator,
		FxCapturedAt:                  quote.FxCapturedAt,
	}, nil
}

func marshalSubscriptionConversionQuoteFacts(facts subscriptionConversionQuoteFacts) (string, string, error) {
	if facts.Version != subscriptionConversionQuoteFactsVersion {
		return "", "", errors.New("invalid subscription conversion quote facts version")
	}
	payload, err := common.Marshal(facts)
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%x", common.Sha256Raw(payload)), string(payload), nil
}
