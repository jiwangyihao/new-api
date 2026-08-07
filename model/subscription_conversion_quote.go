package model

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"gorm.io/gorm"
)

const (
	TimedSubscriptionConversionBlockSeconds    int64 = 31 * 24 * 60 * 60
	TimedSubscriptionConversionCooldownSeconds int64 = 24 * 60 * 60
	TimedSubscriptionConversionGraceSeconds    int64 = 336 * 60 * 60
)

const (
	ConversionCreditBasisGrantSnapshot = "grant_snapshot"
	ConversionCreditBasisCurrentPlan   = "current_plan_fallback"
	ConversionCreditBasisUnavailable   = "unavailable"
)

const (
	ConversionQuoteCategoryConvertible = "convertible"
	ConversionQuoteCategoryGrace       = "expired_grace"
	ConversionQuoteCategoryExcluded    = "excluded"
)

const (
	ConversionQuoteCooldownReady   = "ready"
	ConversionQuoteCooldownActive  = "active"
	ConversionQuoteCooldownUnknown = "unknown"
)

const (
	ConversionQuoteGraceNotStarted = "not_started"
	ConversionQuoteGraceActive     = "active"
	ConversionQuoteGraceExpired    = "expired"
)

const (
	ConversionQuoteReasonGlobalDisabled       = "global_conversion_disabled"
	ConversionQuoteReasonEntitlementNotTimed  = "entitlement_not_timed"
	ConversionQuoteReasonPlanNotFound         = "plan_not_found"
	ConversionQuoteReasonPlanNotTimed         = "plan_not_timed"
	ConversionQuoteReasonDurationNotOneMonth  = "duration_not_one_month"
	ConversionQuoteReasonResetNotMonthly      = "reset_not_monthly"
	ConversionQuoteReasonMonthlyCreditInvalid = "monthly_credit_not_positive"
	ConversionQuoteReasonTrialPlan            = "trial_plan"
	ConversionQuoteReasonMonthlyInvitePlan    = "monthly_invite_plan"
	ConversionQuoteReasonTrialSource          = "trial_source"
	ConversionQuoteReasonMonthlyInviteSource  = "monthly_invite_source"
	ConversionQuoteReasonSourceNotEligible    = "source_not_eligible"
	ConversionQuoteReasonPlanDisabled         = "plan_conversion_disabled"
	ConversionQuoteReasonStatusNotEligible    = "status_not_eligible"
	ConversionQuoteReasonNotStarted           = "subscription_not_started"
	ConversionQuoteReasonOutsideGrace         = "outside_grace_period"
	ConversionQuoteReasonGrantTimeMissing     = "grant_time_missing"
	ConversionQuoteReasonCooldownActive       = "cooldown_active"
	ConversionQuoteReasonGrossNotPositive     = "gross_credit_not_positive"
	ConversionQuoteReasonCalculationFailed    = "calculation_failed"
)

const (
	ConversionQuoteCalculationArithmeticOverflow = "arithmetic_overflow"
	ConversionQuoteCalculationInvalidData        = "invalid_data"
	ConversionQuoteCalculationBasisUnavailable   = "credit_basis_unavailable"
)

const (
	SubscriptionGrantTimeSourceLive           = "live_grant"
	SubscriptionGrantTimeSourceOrder          = "successful_order"
	SubscriptionGrantTimeSourceRedemption     = "used_redemption"
	SubscriptionGrantTimeSourceReliableRecord = "reliable_grant_record"
	SubscriptionGrantTimeSourceConservative   = "conservative_migration"
)

type TimedSubscriptionConversionQuoteReason struct {
	Code string         `json:"code"`
	Data map[string]any `json:"data,omitempty"`
}

type TimedSubscriptionConversionQuote struct {
	SourceSubscriptionId              int                                      `json:"source_subscription_id"`
	PlanId                            int                                      `json:"plan_id"`
	PlanTitle                         string                                   `json:"plan_title"`
	EntitlementType                   string                                   `json:"entitlement_type"`
	GrantSource                       string                                   `json:"grant_source"`
	Status                            string                                   `json:"status"`
	Category                          string                                   `json:"category"`
	DatabaseNow                       int64                                    `json:"database_now"`
	StartTime                         int64                                    `json:"start_time"`
	EndTime                           int64                                    `json:"end_time"`
	RemainingSeconds                  int64                                    `json:"remaining_seconds"`
	Full31DayBlocks                   int64                                    `json:"full_31_day_blocks"`
	CreditBasis                       int64                                    `json:"credit_basis"`
	CreditBasisSource                 string                                   `json:"credit_basis_source"`
	CurrentRemainingCredit            int64                                    `json:"current_remaining_credit"`
	GrossCredit                       int64                                    `json:"gross_credit"`
	CurrentDebt                       int64                                    `json:"current_debt"`
	EstimatedDebtOffset               int64                                    `json:"estimated_debt_offset"`
	NetAvailableCredit                int64                                    `json:"net_available_credit"`
	LastGrantedAt                     int64                                    `json:"last_granted_at"`
	LastGrantTimeSource               string                                   `json:"last_grant_time_source"`
	LastGrantSource                   string                                   `json:"last_grant_source"`
	CooldownStatus                    string                                   `json:"cooldown_status"`
	CooldownRemainingSeconds          int64                                    `json:"cooldown_remaining_seconds"`
	GraceStatus                       string                                   `json:"grace_status"`
	GraceRemainingSeconds             int64                                    `json:"grace_remaining_seconds"`
	Expired                           bool                                     `json:"expired"`
	WithinGrace                       bool                                     `json:"within_grace"`
	Eligible                          bool                                     `json:"eligible"`
	CanConfirm                        bool                                     `json:"can_confirm"`
	ReasonCodes                       []string                                 `json:"reason_codes"`
	Reasons                           []TimedSubscriptionConversionQuoteReason `json:"reasons"`
	CalculationErrorCode              string                                   `json:"calculation_error_code,omitempty"`
	ValuationSourcePriceMicros        int64                                    `json:"valuation_source_price_micros"`
	ValuationSourceCurrency           string                                   `json:"valuation_source_currency"`
	ValuationCurrency                 string                                   `json:"valuation_currency"`
	ValuationCreditBasis              int64                                    `json:"valuation_credit_basis"`
	ValuationGrossCostMicros          int64                                    `json:"valuation_gross_cost_micros"`
	ValuationNetCostMicros            int64                                    `json:"valuation_net_cost_micros"`
	ValuationUnitValueNumeratorMicros int64                                    `json:"valuation_unit_value_numerator_micros"`
	ValuationUnitValueDenominator     int64                                    `json:"valuation_unit_value_denominator"`
	ValuationRuleVersion              int                                      `json:"valuation_rule_version"`
	FxRateNumerator                   int64                                    `json:"fx_rate_numerator"`
	FxRateDenominator                 int64                                    `json:"fx_rate_denominator"`
	FxCapturedAt                      int64                                    `json:"fx_captured_at"`
	FxDirection                       string                                   `json:"fx_direction"`
}

type TimedSubscriptionConversionQuoteList struct {
	DatabaseNow int64                              `json:"database_now"`
	Quotes      []TimedSubscriptionConversionQuote `json:"quotes"`
	Conversions []SubscriptionConversion           `json:"conversions"`
}

const timedSubscriptionConversionQuoteSelect = `
	id,
	user_id,
	plan_id,
	COALESCE(entitlement_type, '') AS entitlement_type,
	COALESCE(token_limit, 0) AS token_limit,
	COALESCE(token_used, 0) AS token_used,
	COALESCE(grant_reason, '') AS grant_reason,
	COALESCE(last_granted_at, 0) AS last_granted_at,
	last_grant_credit_snapshot,
	COALESCE(last_grant_time_source, '') AS last_grant_time_source,
	COALESCE(last_grant_source, '') AS last_grant_source,
	COALESCE(start_time, 0) AS start_time,
	COALESCE(end_time, 0) AS end_time,
	COALESCE(status, '') AS status,
	COALESCE(source, '') AS source`

// ListTimedSubscriptionConversionQuotes returns a fresh, read-only quote for every
// non-Credit-balance subscription instance owned by the user.
func ListTimedSubscriptionConversionQuotes(userId int) (*TimedSubscriptionConversionQuoteList, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if DB == nil {
		return nil, errors.New("database is nil")
	}
	result := &TimedSubscriptionConversionQuoteList{
		Quotes:      []TimedSubscriptionConversionQuote{},
		Conversions: []SubscriptionConversion{},
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		databaseNow, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		result.DatabaseNow = databaseNow
		var subscriptions []UserSubscription
		if err := tx.Select(timedSubscriptionConversionQuoteSelect).
			Where("user_id = ? AND (entitlement_type IS NULL OR entitlement_type <> ?) AND (status IS NULL OR status <> ?)", userId, SubscriptionEntitlementCreditBalance, SubscriptionStatusConverted).
			Order("id asc").Find(&subscriptions).Error; err != nil {
			return err
		}
		result.Quotes = make([]TimedSubscriptionConversionQuote, 0, len(subscriptions))
		for i := range subscriptions {
			quote, err := recalculateTimedSubscriptionConversionQuoteForSubscriptionTx(tx, &subscriptions[i], databaseNow)
			if err != nil {
				return err
			}
			result.Quotes = append(result.Quotes, *quote)
		}
		if err := tx.Model(&SubscriptionConversion{}).
			Select("subscription_conversions.*, credit_balance_ledgers.valuation_state_version_after AS valuation_state_version_after").
			Joins("LEFT JOIN credit_balance_ledgers ON credit_balance_ledgers.id = subscription_conversions.ledger_id").
			Where("subscription_conversions.user_id = ?", userId).
			Order("subscription_conversions.id desc").Limit(100).Find(&result.Conversions).Error; err != nil {
			return err
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RecalculateTimedSubscriptionConversionQuoteTx is the reusable transaction seam
// for conversion confirmation. Callers supply the same database timestamp used by
// their transaction; this function only reads and never grants or mutates benefits.
func RecalculateTimedSubscriptionConversionQuoteTx(tx *gorm.DB, userId int, sourceSubscriptionId int, dbNow int64) (*TimedSubscriptionConversionQuote, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if userId <= 0 || sourceSubscriptionId <= 0 || dbNow <= 0 {
		return nil, errors.New("invalid conversion quote request")
	}
	var subscription UserSubscription
	if err := tx.Select(timedSubscriptionConversionQuoteSelect).
		Where("id = ? AND user_id = ?", sourceSubscriptionId, userId).
		First(&subscription).Error; err != nil {
		return nil, err
	}
	return recalculateTimedSubscriptionConversionQuoteForSubscriptionTx(tx, &subscription, dbNow)
}

func recalculateTimedSubscriptionConversionQuoteForSubscriptionTx(tx *gorm.DB, subscription *UserSubscription, dbNow int64) (*TimedSubscriptionConversionQuote, error) {
	if tx == nil || subscription == nil || dbNow <= 0 {
		return nil, errors.New("invalid conversion quote state")
	}
	quote := &TimedSubscriptionConversionQuote{
		SourceSubscriptionId: subscription.Id,
		PlanId:               subscription.PlanId,
		EntitlementType:      strings.TrimSpace(subscription.EntitlementType),
		Status:               strings.TrimSpace(subscription.Status),
		DatabaseNow:          dbNow,
		StartTime:            subscription.StartTime,
		EndTime:              subscription.EndTime,
		LastGrantedAt:        subscription.LastGrantedAt,
		LastGrantTimeSource:  strings.TrimSpace(subscription.LastGrantTimeSource),
		LastGrantSource:      strings.TrimSpace(subscription.LastGrantSource),
		CooldownStatus:       ConversionQuoteCooldownUnknown,
		GraceStatus:          ConversionQuoteGraceExpired,
		CreditBasisSource:    ConversionCreditBasisUnavailable,
		Category:             ConversionQuoteCategoryExcluded,
		ReasonCodes:          []string{},
		Reasons:              []TimedSubscriptionConversionQuoteReason{},
	}
	if quote.LastGrantSource != "" {
		quote.GrantSource = quote.LastGrantSource
	} else {
		quote.GrantSource = normalizedSubscriptionGrantSource(subscription)
	}

	creditPlan, err := getConversionCreditBalancePlanTx(tx)
	if err != nil {
		return nil, err
	}
	if creditPlan == nil || !creditPlan.Enabled || !creditPlan.CreditBalanceConfigured || !creditPlan.CreditBalanceConversionEnabled {
		quote.addReason(ConversionQuoteReasonGlobalDisabled, map[string]any{"configured": creditPlan != nil && creditPlan.CreditBalanceConfigured})
	}

	if quote.EntitlementType != SubscriptionEntitlementTimed {
		quote.addReason(ConversionQuoteReasonEntitlementNotTimed, map[string]any{"entitlement_type": quote.EntitlementType})
	}

	var plan SubscriptionPlan
	planQuery := tx.Where("id = ?", subscription.PlanId).Limit(1).Find(&plan)
	if planQuery.Error != nil {
		return nil, planQuery.Error
	}
	planFound := planQuery.RowsAffected > 0
	if !planFound {
		quote.addReason(ConversionQuoteReasonPlanNotFound, map[string]any{"plan_id": subscription.PlanId})
	} else {
		quote.PlanTitle = plan.Title
		if strings.TrimSpace(plan.EntitlementType) != SubscriptionEntitlementTimed {
			quote.addReason(ConversionQuoteReasonPlanNotTimed, map[string]any{"entitlement_type": plan.EntitlementType})
		}
		if plan.DurationUnit != SubscriptionDurationMonth || plan.DurationValue != 1 {
			quote.addReason(ConversionQuoteReasonDurationNotOneMonth, map[string]any{"duration_unit": plan.DurationUnit, "duration_value": plan.DurationValue})
		}
		if NormalizeResetPeriod(plan.QuotaResetPeriod) != SubscriptionResetMonthly {
			quote.addReason(ConversionQuoteReasonResetNotMonthly, map[string]any{"quota_reset_period": plan.QuotaResetPeriod})
		}
		if plan.MonthlyTokenLimit <= 0 {
			quote.addReason(ConversionQuoteReasonMonthlyCreditInvalid, map[string]any{"monthly_credit": plan.MonthlyTokenLimit})
		}
		if plan.IsTrial {
			quote.addReason(ConversionQuoteReasonTrialPlan, nil)
		}
		if plan.InviteTrial {
			quote.addReason(ConversionQuoteReasonMonthlyInvitePlan, nil)
		}
		if !plan.Enabled {
			quote.addReason(ConversionQuoteReasonPlanDisabled, nil)
		}
		if !plan.TimedConversionEnabled {
			quote.addReason(ConversionQuoteReasonPlanDisabled, nil)
		}
	}

	switch quote.GrantSource {
	case SubscriptionGrantOrder, SubscriptionGrantRedemption, SubscriptionGrantAdmin, SubscriptionGrantCompensation:
	case "trial_code", "invite_trial":
		quote.addReason(ConversionQuoteReasonTrialSource, map[string]any{"source": quote.GrantSource})
	case SubscriptionGrantMonthlyInviteEntitlement:
		quote.addReason(ConversionQuoteReasonMonthlyInviteSource, map[string]any{"source": quote.GrantSource})
	default:
		quote.addReason(ConversionQuoteReasonSourceNotEligible, map[string]any{"source": quote.GrantSource})
	}

	if subscription.StartTime > dbNow {
		quote.addReason(ConversionQuoteReasonNotStarted, map[string]any{
			"start_time":   subscription.StartTime,
			"database_now": dbNow,
		})
	}

	remainingSeconds, ok := checkedNonNegativeDifference(subscription.EndTime, dbNow)
	if !ok {
		quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
	} else {
		quote.RemainingSeconds = remainingSeconds
	}
	quote.Expired = subscription.EndTime <= dbNow
	if !quote.Expired {
		quote.GraceStatus = ConversionQuoteGraceNotStarted
	} else {
		graceStart, graceStartOK := checkedSubInt64(dbNow, TimedSubscriptionConversionGraceSeconds)
		if !graceStartOK {
			quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
		} else if subscription.EndTime >= graceStart {
			quote.WithinGrace = true
			quote.GraceStatus = ConversionQuoteGraceActive
			graceEnd, graceEndOK := checkedAddInt64(subscription.EndTime, TimedSubscriptionConversionGraceSeconds)
			if !graceEndOK {
				quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
			} else if graceRemaining, graceRemainingOK := checkedNonNegativeDifference(graceEnd, dbNow); graceRemainingOK {
				quote.GraceRemainingSeconds = graceRemaining
			} else {
				quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
			}
		} else {
			quote.GraceStatus = ConversionQuoteGraceExpired
			quote.addReason(ConversionQuoteReasonOutsideGrace, map[string]any{"grace_seconds": TimedSubscriptionConversionGraceSeconds})
		}
	}
	if quote.Status != "active" && !(quote.Status == "expired" && quote.Expired) {
		quote.addReason(ConversionQuoteReasonStatusNotEligible, map[string]any{"status": quote.Status})
	}

	if subscription.LastGrantedAt <= 0 {
		quote.CooldownStatus = ConversionQuoteCooldownUnknown
		quote.addReason(ConversionQuoteReasonGrantTimeMissing, nil)
	} else {
		cooldownEnd, cooldownEndOK := checkedAddInt64(subscription.LastGrantedAt, TimedSubscriptionConversionCooldownSeconds)
		if !cooldownEndOK {
			quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
		} else if dbNow >= cooldownEnd {
			quote.CooldownStatus = ConversionQuoteCooldownReady
		} else {
			quote.CooldownStatus = ConversionQuoteCooldownActive
			cooldownRemaining, cooldownRemainingOK := checkedNonNegativeDifference(cooldownEnd, dbNow)
			if !cooldownRemainingOK {
				quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
			} else {
				quote.CooldownRemainingSeconds = cooldownRemaining
			}
			quote.addReason(ConversionQuoteReasonCooldownActive, map[string]any{"remaining_seconds": quote.CooldownRemainingSeconds})
		}
	}

	if subscription.TokenLimit < 0 || subscription.TokenUsed < 0 {
		quote.setCalculationError(ConversionQuoteCalculationInvalidData)
	} else if currentRemaining, currentRemainingOK := checkedNonNegativeDifference(subscription.TokenLimit, subscription.TokenUsed); currentRemainingOK {
		quote.CurrentRemainingCredit = currentRemaining
	} else {
		quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
	}

	if subscription.LastGrantCreditSnapshot != nil {
		quote.CreditBasis = *subscription.LastGrantCreditSnapshot
		quote.CreditBasisSource = ConversionCreditBasisGrantSnapshot
	} else if planFound {
		quote.CreditBasis = plan.MonthlyTokenLimit
		quote.CreditBasisSource = ConversionCreditBasisCurrentPlan
	} else {
		quote.setCalculationError(ConversionQuoteCalculationBasisUnavailable)
	}
	if quote.CreditBasis < 0 {
		quote.setCalculationError(ConversionQuoteCalculationInvalidData)
	}

	quote.Full31DayBlocks = quote.RemainingSeconds / TimedSubscriptionConversionBlockSeconds
	if quote.CalculationErrorCode == "" {
		blockCredit, multiplyOK := checkedMulNonNegativeInt64(quote.Full31DayBlocks, quote.CreditBasis)
		if !multiplyOK {
			quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
		} else if gross, addOK := checkedAddInt64(blockCredit, quote.CurrentRemainingCredit); !addOK {
			quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
		} else {
			quote.GrossCredit = gross
		}
	}

	debt, debtErr := currentCreditBalanceDebtTx(tx, subscription.UserId)
	if debtErr != nil {
		return nil, debtErr
	}
	quote.CurrentDebt = debt
	if quote.CalculationErrorCode == "" {
		quote.EstimatedDebtOffset = minInt64(quote.GrossCredit, quote.CurrentDebt)
		net, netOK := checkedSubInt64(quote.GrossCredit, quote.EstimatedDebtOffset)
		if !netOK {
			quote.setCalculationError(ConversionQuoteCalculationArithmeticOverflow)
		} else {
			quote.NetAvailableCredit = net
		}
	}
	if quote.CalculationErrorCode == "" && quote.GrossCredit > 0 && planFound && creditPlan != nil {
		if err := populateTimedSubscriptionConversionQuoteValuationTx(tx, quote, &plan, creditPlan, dbNow); err != nil {
			quote.setCalculationError(conversionQuoteValuationErrorCode(err))
		}
	}

	if quote.GrossCredit <= 0 && quote.CalculationErrorCode == "" {
		quote.addReason(ConversionQuoteReasonGrossNotPositive, map[string]any{"gross_credit": quote.GrossCredit})
	}
	quote.Eligible = len(quote.ReasonCodes) == 0 && quote.CalculationErrorCode == ""
	quote.CanConfirm = quote.Eligible
	if quote.Expired && quote.WithinGrace {
		quote.Category = ConversionQuoteCategoryGrace
	} else if quote.CanConfirm {
		quote.Category = ConversionQuoteCategoryConvertible
	} else {
		quote.Category = ConversionQuoteCategoryExcluded
	}
	return quote, nil
}

func populateTimedSubscriptionConversionQuoteValuationTx(tx *gorm.DB, quote *TimedSubscriptionConversionQuote, sourcePlan *SubscriptionPlan, creditPlan *SubscriptionPlan, dbNow int64) error {
	ready, err := CreditValuationRuntimeReadyTx(tx)
	if err != nil || !ready {
		return err
	}
	if quote == nil || sourcePlan == nil || creditPlan == nil || sourcePlan.PriceAmountMicros == nil {
		return ErrSubscriptionPlanPriceRequired
	}
	if *sourcePlan.PriceAmountMicros <= 0 || quote.CreditBasis <= 0 || quote.GrossCredit <= 0 {
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
	ingress, err := newForwardCreditValuationIngress(CreditValuationSourceSnapshot{
		SourcePriceMicros: *sourcePlan.PriceAmountMicros,
		SourcePlanCredit:  quote.CreditBasis,
		GrossCredit:       quote.GrossCredit,
		SourceCurrency:    sourceCurrency,
		ValuationCurrency: valuationCurrency,
		RuleVersion:       CreditValuationRuleVersion,
		FXRateSnapshot:    &fxSnapshot,
	})
	if err != nil {
		return err
	}
	netCostMicros, err := prorateFloor(ingress.grossCostMicros, quote.NetAvailableCredit, quote.GrossCredit)
	if err != nil {
		return err
	}
	quote.ValuationSourcePriceMicros = *sourcePlan.PriceAmountMicros
	quote.ValuationSourceCurrency = sourceCurrency
	quote.ValuationCurrency = valuationCurrency
	quote.ValuationCreditBasis = quote.CreditBasis
	quote.ValuationGrossCostMicros = ingress.grossCostMicros
	quote.ValuationNetCostMicros = netCostMicros
	quote.ValuationUnitValueNumeratorMicros = ingress.unitValueNumeratorMicros
	quote.ValuationUnitValueDenominator = ingress.unitValueDenominator
	quote.ValuationRuleVersion = ingress.ruleVersion
	quote.FxRateNumerator = ingress.fxRateNumerator
	quote.FxRateDenominator = ingress.fxRateDenominator
	quote.FxCapturedAt = ingress.fxCapturedAt
	quote.FxDirection = creditFXDirection(sourceCurrency, valuationCurrency)
	return nil
}

func conversionQuoteValuationErrorCode(err error) string {
	if err == nil {
		return ""
	}
	for _, stable := range []error{
		ErrSubscriptionPlanPriceRequired,
		ErrCreditValuationSourceInvalid,
		ErrCreditValuationCurrencyRequired,
		ErrCreditValuationUnsupportedCurrency,
		ErrCreditValuationOverflow,
		ErrCreditFXRateMissing,
		ErrCreditFXRateEmpty,
		ErrCreditFXInvalidDecimal,
		ErrCreditFXPrecisionExceeded,
		ErrCreditFXNonPositive,
		ErrCreditFXUnsupportedCurrency,
		ErrCreditFXDirectionMismatch,
		ErrCreditFXOverflow,
	} {
		if errors.Is(err, stable) {
			return stable.Error()
		}
	}
	return ConversionQuoteCalculationInvalidData
}

func (quote *TimedSubscriptionConversionQuote) addReason(code string, data map[string]any) {
	if quote == nil || strings.TrimSpace(code) == "" {
		return
	}
	for _, existing := range quote.ReasonCodes {
		if existing == code {
			return
		}
	}
	quote.ReasonCodes = append(quote.ReasonCodes, code)
	quote.Reasons = append(quote.Reasons, TimedSubscriptionConversionQuoteReason{Code: code, Data: data})
}

func (quote *TimedSubscriptionConversionQuote) setCalculationError(code string) {
	if quote == nil || quote.CalculationErrorCode != "" {
		return
	}
	quote.CalculationErrorCode = code
	quote.addReason(ConversionQuoteReasonCalculationFailed, map[string]any{"error_code": code})
}

func getConversionCreditBalancePlanTx(tx *gorm.DB) (*SubscriptionPlan, error) {
	var plan SubscriptionPlan
	query := tx.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).Limit(1).Find(&plan)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, nil
	}
	return &plan, nil
}

func currentCreditBalanceDebtTx(tx *gorm.DB, userId int) (int64, error) {
	var balance UserSubscription
	query := tx.Select("token_limit", "token_used").Where("user_id = ? AND entitlement_type = ?", userId, SubscriptionEntitlementCreditBalance).Limit(1).Find(&balance)
	if query.Error != nil {
		return 0, query.Error
	}
	if query.RowsAffected == 0 {
		return 0, nil
	}
	if balance.TokenLimit < 0 || balance.TokenUsed < 0 {
		return 0, fmt.Errorf("invalid credit balance aggregate for user %d", userId)
	}
	if balance.TokenUsed <= balance.TokenLimit {
		return 0, nil
	}
	debt, ok := checkedSubInt64(balance.TokenUsed, balance.TokenLimit)
	if !ok {
		return 0, errors.New("credit balance debt overflow")
	}
	return debt, nil
}

func checkedNonNegativeDifference(left int64, right int64) (int64, bool) {
	difference, ok := checkedSubInt64(left, right)
	if !ok {
		return 0, false
	}
	if difference < 0 {
		return 0, true
	}
	return difference, true
}

func checkedAddInt64(left int64, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right {
		return 0, false
	}
	if right < 0 && left < math.MinInt64-right {
		return 0, false
	}
	return left + right, true
}

func checkedSubInt64(left int64, right int64) (int64, bool) {
	if right > 0 && left < math.MinInt64+right {
		return 0, false
	}
	if right < 0 && left > math.MaxInt64+right {
		return 0, false
	}
	return left - right, true
}

func checkedMulNonNegativeInt64(left int64, right int64) (int64, bool) {
	if left < 0 || right < 0 {
		return 0, false
	}
	if left != 0 && right > math.MaxInt64/left {
		return 0, false
	}
	return left * right, true
}
