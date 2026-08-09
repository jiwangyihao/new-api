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
	CreditBalanceLedgerSourceSubscriptionOrder         = "subscription_order"
	CreditBalanceLedgerSourceRedemption                = "redemption"
	CreditBalanceLedgerSourceSubscriptionConversion    = "subscription_conversion"
	CreditBalanceLedgerTypePurchase                    = "purchase"
	CreditBalanceLedgerTypeRedemption                  = "redemption"
	CreditBalanceLedgerTypeSubscriptionConversion      = "subscription_conversion"
	CreditBalanceLedgerSourceSubscriptionOrderRecovery = "subscription_order_recovery"
	CreditBalanceLedgerSourceAdminAdjustment           = "admin_adjustment"
	CreditBalanceLedgerTypeRefund                      = "refund"
	CreditBalanceLedgerTypeChargeback                  = "chargeback"
	CreditBalanceLedgerTypeAdminIncrease               = "admin_increase"
	CreditBalanceLedgerTypeAdminDecrease               = "admin_decrease"
)

const (
	CreditBalanceStatusAvailable = "available"
	CreditBalanceStatusExhausted = "exhausted"
	CreditBalanceStatusDebt      = "debt"
)

func captureCreditPositiveIngressFXRateSnapshot(sourceCurrency string, valuationCurrency string, capturedAt int64) (*CreditFXRateSnapshot, error) {
	snapshot, err := CurrentCreditFXRateSnapshot(sourceCurrency, valuationCurrency, capturedAt)
	if err != nil {
		return nil, creditPositiveIngressFXError(err)
	}
	if err := validateCreditPositiveIngressFXRateSnapshot(&snapshot, sourceCurrency, valuationCurrency); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func validateCreditPositiveIngressFXRateSnapshot(snapshot *CreditFXRateSnapshot, sourceCurrency string, valuationCurrency string) error {
	sourceCurrency, err := NormalizeCreditValuationCurrency(sourceCurrency)
	if err != nil {
		return err
	}
	valuationCurrency, err = NormalizeCreditValuationCurrency(valuationCurrency)
	if err != nil {
		return err
	}
	if snapshot == nil || snapshot.SourceCurrency != sourceCurrency || snapshot.ValuationCurrency != valuationCurrency || snapshot.Numerator <= 0 || snapshot.Denominator <= 0 || snapshot.CapturedAt <= 0 {
		return ErrCreditValuationInvalidFX
	}
	expectedDirection := CreditFXDirectionIdentity
	if sourceCurrency == "CNY" && valuationCurrency == "USD" {
		expectedDirection = CreditFXDirectionCNYtoUSD
	} else if sourceCurrency == "USD" && valuationCurrency == "CNY" {
		expectedDirection = CreditFXDirectionUSDtoCNY
	} else if sourceCurrency != valuationCurrency {
		return ErrCreditValuationUnsupportedCurrency
	}
	if snapshot.Direction != expectedDirection || (expectedDirection == CreditFXDirectionIdentity && (snapshot.Numerator != 1 || snapshot.Denominator != 1)) {
		return ErrCreditValuationInvalidFX
	}
	return nil
}

func creditPositiveIngressFXError(err error) error {
	switch {
	case errors.Is(err, ErrCreditFXOverflow), errors.Is(err, ErrCreditValuationOverflow):
		return ErrCreditValuationOverflow
	case errors.Is(err, ErrCreditFXUnsupportedCurrency), errors.Is(err, ErrCreditValuationUnsupportedCurrency):
		return ErrCreditValuationUnsupportedCurrency
	default:
		return ErrCreditValuationInvalidFX
	}
}

type CreditBalanceLedger struct {
	Id                                int    `json:"id"`
	UserId                            int    `json:"user_id" gorm:"not null;index;uniqueIndex:idx_credit_balance_ledger_user_key,priority:1"`
	UserSubscriptionId                int    `json:"user_subscription_id" gorm:"not null;index"`
	Type                              string `json:"type" gorm:"type:varchar(32);not null;index"`
	IdempotencyKey                    string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex:idx_credit_balance_ledger_user_key,priority:2"`
	SourceType                        string `json:"source_type" gorm:"type:varchar(32);not null;uniqueIndex:idx_credit_balance_ledger_source,priority:1;index"`
	SourceId                          int    `json:"source_id" gorm:"not null;uniqueIndex:idx_credit_balance_ledger_source,priority:2"`
	SourceSnapshot                    string `json:"source_snapshot,omitempty" gorm:"type:text"`
	SourceKey                         string `json:"source_key" gorm:"type:varchar(160);not null;default:'';index"`
	SourceStatus                      string `json:"source_status" gorm:"type:varchar(32);not null;default:'';index"`
	Operation                         string `json:"operation" gorm:"type:varchar(32);not null;default:'';index"`
	TerminalState                     string `json:"terminal_state" gorm:"type:varchar(32);not null;default:'';index"`
	PlanId                            int    `json:"plan_id" gorm:"not null;default:0;index"`
	GrossCredit                       int64  `json:"gross_credit" gorm:"type:bigint;not null"`
	NetCredit                         int64  `json:"net_credit" gorm:"type:bigint;not null;default:0"`
	SourcePriceMicros                 int64  `json:"source_price_micros,string" gorm:"type:bigint;not null;default:0"`
	SourcePlanCredit                  int64  `json:"source_plan_credit" gorm:"type:bigint;not null;default:0"`
	DebtOffset                        int64  `json:"debt_offset" gorm:"type:bigint;not null;default:0"`
	DebtFormed                        int64  `json:"debt_formed" gorm:"type:bigint;not null;default:0"`
	ConsumedAvailableCredit           int64  `json:"consumed_available_credit" gorm:"type:bigint;not null;default:0"`
	SettlementDebtFormed              int64  `json:"settlement_debt_formed" gorm:"type:bigint;not null;default:0"`
	RemovedExactCostMicros            int64  `json:"removed_exact_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	RemovedEstimatedCostMicros        int64  `json:"removed_estimated_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	RemovedUnknownCredit              int64  `json:"removed_unknown_credit" gorm:"type:bigint;not null;default:0"`
	AvailableCreditBefore             int64  `json:"available_credit_before" gorm:"type:bigint;not null;default:0"`
	SettlementDebtBefore              int64  `json:"settlement_debt_before" gorm:"type:bigint;not null;default:0"`
	BalanceBefore                     int64  `json:"balance_before" gorm:"type:bigint;not null"`
	BalanceAfter                      int64  `json:"balance_after" gorm:"type:bigint;not null"`
	AvailableCreditAfter              int64  `json:"available_credit_after" gorm:"type:bigint;not null;default:0"`
	SettlementDebtAfter               int64  `json:"settlement_debt_after" gorm:"type:bigint;not null;default:0"`
	OperatorUserId                    int    `json:"operator_user_id" gorm:"not null;default:0"`
	PaymentProvider                   string `json:"payment_provider,omitempty" gorm:"type:varchar(50);not null;default:''"`
	ParameterFingerprint              string `json:"-" gorm:"type:varchar(64);not null;default:''"`
	SourcePlanId                      int    `json:"source_plan_id" gorm:"not null;default:0;index"`
	TargetPlanId                      int    `json:"target_plan_id" gorm:"not null;default:0;index"`
	SourceTokenLimit                  int64  `json:"source_token_limit,string" gorm:"type:bigint;not null;default:0"`
	SourceTokenUsed                   int64  `json:"source_token_used,string" gorm:"type:bigint;not null;default:0"`
	SourceStartTime                   int64  `json:"source_start_time,string" gorm:"type:bigint;not null;default:0"`
	SourceEndTime                     int64  `json:"source_end_time,string" gorm:"type:bigint;not null;default:0"`
	Full31DayBlocks                   int64  `json:"full_31_day_blocks,string" gorm:"type:bigint;not null;default:0"`
	CurrentRemainingCredit            int64  `json:"current_remaining_credit,string" gorm:"type:bigint;not null;default:0"`
	CreditBasisSource                 string `json:"credit_basis_source" gorm:"type:varchar(32);not null;default:''"`
	SourceDurationUnit                string `json:"source_duration_unit" gorm:"type:varchar(16);not null;default:''"`
	SourceDurationValue               int    `json:"source_duration_value" gorm:"not null;default:0"`
	SourceCustomSeconds               int64  `json:"source_custom_seconds,string" gorm:"type:bigint;not null;default:0"`
	SourceQuotaResetPeriod            string `json:"source_quota_reset_period" gorm:"type:varchar(16);not null;default:''"`
	SourceQuotaResetCustomSeconds     int64  `json:"source_quota_reset_custom_seconds,string" gorm:"type:bigint;not null;default:0"`
	ValuationSourcePriceMicros        int64  `json:"valuation_source_price_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationCreditBasis              int64  `json:"valuation_credit_basis,string" gorm:"type:bigint;not null;default:0"`
	ValuationUnitValueNumeratorMicros int64  `json:"valuation_unit_value_numerator_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationUnitValueDenominator     int64  `json:"valuation_unit_value_denominator,string" gorm:"type:bigint;not null;default:0"`
	NetGrantedCredit                  int64  `json:"net_granted_credit,string" gorm:"type:bigint;not null;default:0"`
	Reason                            string `json:"reason" gorm:"type:varchar(255);not null;default:''"`
	ValuationCurrency                 string `json:"valuation_currency" gorm:"type:varchar(8);not null;default:''"`
	ValuationGrossCostMicros          int64  `json:"valuation_gross_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationNetCostMicros            int64  `json:"valuation_net_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationConfidence               string `json:"valuation_confidence" gorm:"type:varchar(16);not null;default:''"`
	ValuationRuleVersion              int    `json:"valuation_rule_version" gorm:"not null;default:0"`
	ValuationStateVersionAfter        int64  `json:"valuation_state_version_after" gorm:"type:bigint;not null;default:0"`
	FxSourceCurrency                  string `json:"fx_source_currency" gorm:"type:varchar(8);not null;default:''"`
	FxRateNumerator                   int64  `json:"fx_rate_numerator,string" gorm:"type:bigint;not null;default:0"`
	FxRateDenominator                 int64  `json:"fx_rate_denominator,string" gorm:"type:bigint;not null;default:0"`
	FxCapturedAt                      int64  `json:"fx_captured_at" gorm:"type:bigint;not null;default:0"`
	FxDirection                       string `json:"fx_direction" gorm:"type:varchar(16);not null;default:''"`
	CreatedAt                         int64  `json:"created_at" gorm:"type:bigint;not null;index"`
}

type CreditBalanceLedgerHistoryItem struct {
	CreditBalanceLedger
	PaymentMethod string `json:"payment_method,omitempty"`
	PurchaseMode  string `json:"purchase_mode,omitempty"`
}

func (l *CreditBalanceLedger) BeforeUpdate(_ *gorm.DB) error {
	return errors.New("credit balance ledger is immutable")
}

func (l *CreditBalanceLedger) BeforeDelete(_ *gorm.DB) error {
	return errors.New("credit balance ledger is immutable")
}

type CreditBalanceConversionSourceFacts struct {
	ConversionIdempotencyKey string
	SourcePlanId             int
	SourceTokenLimit         int64
	SourceTokenUsed          int64
	SourceStatus             string
	SourceStartTime          int64
	SourceEndTime            int64
	Full31DayBlocks          int64
	CurrentRemainingCredit   int64
	CreditBasisSource        string
	DurationUnit             string
	DurationValue            int
	CustomSeconds            int64
	QuotaResetPeriod         string
	QuotaResetCustomSeconds  int64
}

type creditBalanceConversionFingerprintFacts struct {
	Version                       int    `json:"version"`
	UserId                        int    `json:"user_id"`
	ConversionIdempotencyKey      string `json:"conversion_idempotency_key"`
	SourceSubscriptionId          int    `json:"source_subscription_id"`
	SourcePlanId                  int    `json:"source_plan_id"`
	TargetPlanId                  int    `json:"target_plan_id"`
	SourceTokenLimit              int64  `json:"source_token_limit"`
	SourceTokenUsed               int64  `json:"source_token_used"`
	SourceStatus                  string `json:"source_status"`
	SourceStartTime               int64  `json:"source_start_time"`
	SourceEndTime                 int64  `json:"source_end_time"`
	Full31DayBlocks               int64  `json:"full_31_day_blocks"`
	CurrentRemainingCredit        int64  `json:"current_remaining_credit"`
	CreditBasisSource             string `json:"credit_basis_source"`
	SourceDurationUnit            string `json:"source_duration_unit"`
	SourceDurationValue           int    `json:"source_duration_value"`
	SourceCustomSeconds           int64  `json:"source_custom_seconds"`
	SourceQuotaResetPeriod        string `json:"source_quota_reset_period"`
	SourceQuotaResetCustomSeconds int64  `json:"source_quota_reset_custom_seconds"`
	SourcePriceMicros             int64  `json:"source_price_micros"`
	ValuationCreditBasis          int64  `json:"valuation_credit_basis"`
	GrossCredit                   int64  `json:"gross_credit"`
	SourceCurrency                string `json:"source_currency"`
	ValuationCurrency             string `json:"valuation_currency"`
	RuleVersion                   int    `json:"rule_version"`
	FxRateNumerator               int64  `json:"fx_rate_numerator"`
	FxRateDenominator             int64  `json:"fx_rate_denominator"`
	FxCapturedAt                  int64  `json:"fx_captured_at"`
	UnitValueNumeratorMicros      int64  `json:"unit_value_numerator_micros"`
	UnitValueDenominator          int64  `json:"unit_value_denominator"`
}

func creditBalanceConversionParameterFingerprint(request CreditBalanceGrantRequest, ingress creditValuationIngress) (string, error) {
	source := request.ConversionSource
	if source == nil || strings.TrimSpace(source.ConversionIdempotencyKey) == "" {
		return "", ErrCreditValuationSourceInvalid
	}
	facts := creditBalanceConversionFingerprintFacts{
		Version:                       1,
		UserId:                        request.UserId,
		ConversionIdempotencyKey:      strings.TrimSpace(source.ConversionIdempotencyKey),
		SourceSubscriptionId:          request.SourceId,
		SourcePlanId:                  source.SourcePlanId,
		TargetPlanId:                  request.TargetPlanId,
		SourceTokenLimit:              source.SourceTokenLimit,
		SourceTokenUsed:               source.SourceTokenUsed,
		SourceStatus:                  strings.TrimSpace(source.SourceStatus),
		SourceStartTime:               source.SourceStartTime,
		SourceEndTime:                 source.SourceEndTime,
		Full31DayBlocks:               source.Full31DayBlocks,
		CurrentRemainingCredit:        source.CurrentRemainingCredit,
		CreditBasisSource:             strings.TrimSpace(source.CreditBasisSource),
		SourceDurationUnit:            strings.TrimSpace(source.DurationUnit),
		SourceDurationValue:           source.DurationValue,
		SourceCustomSeconds:           source.CustomSeconds,
		SourceQuotaResetPeriod:        NormalizeResetPeriod(source.QuotaResetPeriod),
		SourceQuotaResetCustomSeconds: source.QuotaResetCustomSeconds,
		GrossCredit:                   request.GrossCredit,
	}
	if valuation := request.ValuationSource; valuation != nil {
		facts.SourcePriceMicros = valuation.SourcePriceMicros
		facts.ValuationCreditBasis = valuation.SourcePlanCredit
		facts.SourceCurrency = ingress.fxSourceCurrency
		facts.ValuationCurrency = ingress.currency
		facts.RuleVersion = ingress.ruleVersion
		facts.FxRateNumerator = ingress.fxRateNumerator
		facts.FxRateDenominator = ingress.fxRateDenominator
		facts.FxCapturedAt = ingress.fxCapturedAt
		facts.UnitValueNumeratorMicros = ingress.unitValueNumeratorMicros
		facts.UnitValueDenominator = ingress.unitValueDenominator
	}
	payload, err := common.Marshal(facts)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", common.Sha256Raw(payload)), nil
}

// ConversionIdempotencyKey lives in ConversionSource; the ledger idempotency
// key remains the source-derived allocation identity.
type CreditBalanceGrantRequest struct {
	UserId                  int
	GrossCredit             int64
	IdempotencyKey          string
	SourceType              string
	SourceId                int
	SourceSnapshot          string
	SourceKey               string
	SourceStatus            string
	SourcePlanId            int
	ParameterFingerprint    string
	Type                    string
	TargetPlanId            int
	OperatorUserId          int
	Reason                  string
	PaymentProvider         string
	TargetPlanSnapshot      *SubscriptionPlan
	ValuationSource         *CreditValuationSourceSnapshot
	ConversionSource        *CreditBalanceConversionSourceFacts
	PreserveActiveSelection bool
	parameterFingerprint    string
}

type CreditBalanceGrantResult struct {
	UserSubscriptionId         int    `json:"user_subscription_id"`
	PlanId                     int    `json:"plan_id"`
	GrossCredit                int64  `json:"gross_credit"`
	NetCredit                  int64  `json:"net_credit"`
	GrossAmountMicros          int64  `json:"gross_amount_micros,string"`
	NetAmountMicros            int64  `json:"net_amount_micros,string"`
	ValuationCurrency          string `json:"valuation_currency"`
	SourceCurrency             string `json:"source_currency"`
	ValuationConfidence        string `json:"confidence"`
	FxRateNumerator            int64  `json:"fx_rate_numerator,string"`
	FxRateDenominator          int64  `json:"fx_rate_denominator,string"`
	FxCapturedAt               int64  `json:"fx_captured_at"`
	FxDirection                string `json:"fx_direction"`
	ValuationRuleVersion       int    `json:"rule_version"`
	ValuationStateVersionAfter int64  `json:"state_version_after"`
	DebtOffset                 int64  `json:"debt_offset"`
	ConsumedAvailableCredit    int64  `json:"consumed_available_credit"`
	DebtFormed                 int64  `json:"debt_formed"`
	RemovedExactCostMicros     int64  `json:"removed_exact_cost_micros,string"`
	RemovedEstimatedCostMicros int64  `json:"removed_estimated_cost_micros,string"`
	RemovedUnknownCredit       int64  `json:"removed_unknown_credit"`
	Operation                  string `json:"operation"`
	TerminalState              string `json:"terminal_state"`
	AvailableCredit            int64  `json:"available_credit"`
	SettlementDebt             int64  `json:"settlement_debt"`
	BalanceBefore              int64  `json:"balance_before"`
	BalanceAfter               int64  `json:"balance_after"`
	Active                     bool   `json:"active"`
	LedgerId                   int    `json:"ledger_id"`
	Status                     string `json:"status"`
	Replayed                   bool   `json:"replayed"`
}

func NormalizeSubscriptionPurchaseMode(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case SubscriptionPurchaseModeTimed:
		return SubscriptionPurchaseModeTimed, nil
	case SubscriptionPurchaseModeCreditBalance:
		return SubscriptionPurchaseModeCreditBalance, nil
	default:
		return "", errors.New("购买模式必须明确选择计时套餐或 Credit 余额")
	}
}

func CreditBalancePlanFromEntitlementSnapshot(snapshot SubscriptionEntitlementSnapshot) (*SubscriptionPlan, error) {
	if snapshot.TargetCreditBalancePlanID <= 0 {
		return nil, errors.New("invalid credit balance target snapshot")
	}
	businessCode := strings.TrimSpace(snapshot.TargetCreditBalanceBusinessCode)
	plan := &SubscriptionPlan{
		Id:                      snapshot.TargetCreditBalancePlanID,
		Title:                   snapshot.TargetCreditBalanceTitle,
		EntitlementType:         SubscriptionEntitlementCreditBalance,
		Enabled:                 true,
		ModelLimits:             snapshot.TargetCreditBalanceModelLimits,
		ConcurrencyLimit:        snapshot.TargetCreditBalanceConcurrencyLimit,
		QueueCapacity:           snapshot.TargetCreditBalanceQueueCapacity,
		GPTAbuseWarningLimit:    snapshot.TargetCreditBalanceGPTAbuseWarningLimit,
		CreditBalanceConfigured: true,
	}
	valuationCurrency := strings.ToUpper(strings.TrimSpace(snapshot.TargetCreditBalanceValuationCurrency))
	if valuationCurrency != "" {
		plan.ValuationCurrency = &valuationCurrency
	}
	if businessCode != "" {
		plan.BusinessCode = &businessCode
	}
	return plan, nil
}
func SubscriptionPlanFromEntitlementSnapshot(snapshot SubscriptionEntitlementSnapshot) (*SubscriptionPlan, error) {
	if snapshot.PlanID <= 0 {
		return nil, errors.New("invalid subscription entitlement snapshot")
	}
	businessCode := strings.TrimSpace(snapshot.BusinessCode)
	plan := &SubscriptionPlan{
		Id:                      snapshot.PlanID,
		Title:                   snapshot.PlanTitle,
		PriceAmount:             snapshot.PriceAmount,
		PriceAmountMicros:       snapshot.ListPriceMicros,
		Currency:                snapshot.Currency,
		EntitlementType:         snapshot.PlanEntitlementType,
		TotalAmount:             snapshot.TotalAmount,
		MonthlyTokenLimit:       snapshot.MonthlyTokenLimit,
		ConcurrencyLimit:        snapshot.ConcurrencyLimit,
		QueueCapacity:           snapshot.QueueCapacity,
		ModelLimits:             snapshot.ModelLimits,
		GPTAbuseWarningLimit:    snapshot.GPTAbuseWarningLimit,
		DurationUnit:            snapshot.DurationUnit,
		DurationValue:           snapshot.DurationValue,
		CustomSeconds:           snapshot.CustomSeconds,
		QuotaResetPeriod:        snapshot.QuotaResetPeriod,
		QuotaResetCustomSeconds: snapshot.QuotaResetCustomSeconds,
		MaxPurchasePerUser:      snapshot.MaxPurchasePerUser,
		IsTrial:                 snapshot.IsTrial,
		InviteTrial:             snapshot.InviteTrial,
		RewardEligible:          snapshot.RewardEligible,
	}
	if strings.TrimSpace(plan.EntitlementType) == "" {
		plan.EntitlementType = SubscriptionEntitlementTimed
	}
	if businessCode != "" {
		plan.BusinessCode = &businessCode
	}
	return plan, nil
}

func SetUserLastSubscriptionPurchaseModeTx(tx *gorm.DB, userId int, purchaseMode string) error {
	if tx == nil || userId <= 0 {
		return errors.New("invalid user purchase mode update")
	}
	mode, err := NormalizeSubscriptionPurchaseMode(purchaseMode)
	if err != nil {
		return err
	}
	var user User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("setting").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	setting := user.GetSetting()
	setting.LastSubscriptionPurchaseMode = mode
	settingJSON, err := marshalUserSetting(setting)
	if err != nil {
		return err
	}
	return tx.Model(&User{}).Where("id = ?", userId).Update("setting", settingJSON).Error
}

func GetCreditBalancePlanTx(tx *gorm.DB) (*SubscriptionPlan, error) {
	if tx == nil {
		tx = DB
	}
	var plan SubscriptionPlan
	if err := tx.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func authorizedCreditBalanceOrderPlanSnapshot(request CreditBalanceGrantRequest) (*SubscriptionPlan, bool) {
	plan := request.TargetPlanSnapshot
	if request.SourceType != CreditBalanceLedgerSourceSubscriptionOrder || plan == nil || plan.Id != request.TargetPlanId || plan.EntitlementType != SubscriptionEntitlementCreditBalance {
		return nil, false
	}
	return plan, true
}

func GrantCreditBalanceTx(tx *gorm.DB, request CreditBalanceGrantRequest) (*CreditBalanceGrantResult, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.SourceType = strings.TrimSpace(request.SourceType)
	request.Type = strings.TrimSpace(request.Type)
	request.SourceSnapshot = strings.TrimSpace(request.SourceSnapshot)
	request.PaymentProvider = strings.TrimSpace(request.PaymentProvider)
	request.Reason = strings.TrimSpace(request.Reason)
	request.SourceKey = strings.TrimSpace(request.SourceKey)
	request.SourceStatus = strings.TrimSpace(request.SourceStatus)
	request.ParameterFingerprint = strings.TrimSpace(request.ParameterFingerprint)
	if request.UserId <= 0 || request.GrossCredit <= 0 || request.IdempotencyKey == "" || request.SourceType == "" || request.SourceId <= 0 || request.Type == "" || request.TargetPlanId <= 0 {
		return nil, errors.New("invalid credit balance grant")
	}

	guardedPlan, err := AcquireCreditBalancePlanGuardTx(tx)
	authorizedPlan, hasAuthorizedPlan := authorizedCreditBalanceOrderPlanSnapshot(request)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) || !hasAuthorizedPlan {
			return nil, err
		}
		guardedPlan = nil
	} else if guardedPlan.Id != request.TargetPlanId {
		return nil, errors.New("credit balance target plan mismatch")
	}
	plan := guardedPlan
	if hasAuthorizedPlan {
		plan = authorizedPlan
	}
	valuationReady, err := CreditValuationRuntimeReadyTx(tx)
	if err != nil {
		return nil, err
	}
	var valuationIngress creditValuationIngress
	if valuationReady {
		if request.ValuationSource == nil {
			return nil, ErrCreditValuationSourceRequired
		}
		valuationIngress, err = newForwardCreditValuationIngress(*request.ValuationSource)
		if err != nil {
			return nil, err
		}
		if valuationIngress.grossCredit != request.GrossCredit {
			return nil, ErrCreditValuationSourceInvalid
		}
	}
	if request.SourceType == CreditBalanceLedgerSourceSubscriptionConversion {
		if request.ConversionSource == nil {
			return nil, ErrCreditValuationSourceInvalid
		}
		request.parameterFingerprint, err = creditBalanceConversionParameterFingerprint(request, valuationIngress)
		if err != nil {
			return nil, err
		}
		request.ParameterFingerprint = request.parameterFingerprint
	}

	var user User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "setting").Where("id = ?", request.UserId).First(&user).Error; err != nil {
		return nil, err
	}
	if result, found, err := findCreditBalanceGrantResultTx(tx, request); err != nil {
		return nil, err
	} else if found {
		return result, nil
	}
	if !hasAuthorizedPlan && !plan.Enabled {
		return nil, ErrCreditBalanceAllocationUnavailable
	}

	hadUsableSubscription, err := hasUsableSubscriptionTx(tx, request.UserId)
	if err != nil {
		return nil, err
	}

	balance, created, err := getOrCreateCreditBalanceSubscriptionTx(tx, request.UserId, plan)
	if err != nil {
		return nil, err
	}
	if balance.TokenLimit < 0 || balance.TokenUsed < 0 {
		return nil, errors.New("invalid credit balance aggregate")
	}
	balanceBefore := balance.TokenLimit - balance.TokenUsed
	settlementDebtBefore := maxInt64(-balanceBefore, 0)
	var valuationMutation CreditValuationMutationResult
	if valuationReady {
		if created {
			if err := initializeCreditValuationStateTx(tx, balance, valuationIngress.currency); err != nil {
				return nil, err
			}
		}
		valuationMutation, err = ApplyCreditValuationIngressTx(tx, balance, valuationIngress)
		if err != nil {
			return nil, err
		}
	} else {
		newLimit, ok := checkedAddInt64(balance.TokenLimit, request.GrossCredit)
		if !ok {
			return nil, ErrCreditValuationOverflow
		}
		if err := tx.Model(&UserSubscription{}).Where("id = ?", balance.Id).Updates(map[string]any{
			"token_limit": newLimit,
			"updated_at":  common.GetTimestamp(),
		}).Error; err != nil {
			return nil, err
		}
		balance.TokenLimit = newLimit
	}
	debtOffset := minInt64(request.GrossCredit, settlementDebtBefore)
	balanceAfter := balance.TokenLimit - balance.TokenUsed
	availableAfter := maxInt64(balanceAfter, 0)
	debtAfter := maxInt64(-balanceAfter, 0)

	setting := user.GetSetting()
	if !hadUsableSubscription && !request.PreserveActiveSelection {
		setting.ActiveSubscriptionId = balance.Id
		settingJSON, err := marshalUserSetting(setting)
		if err != nil {
			return nil, err
		}
		if err := tx.Model(&User{}).Where("id = ?", request.UserId).Update("setting", settingJSON).Error; err != nil {
			return nil, err
		}
	}

	ledger := CreditBalanceLedger{
		UserId:                request.UserId,
		UserSubscriptionId:    balance.Id,
		Type:                  request.Type,
		IdempotencyKey:        request.IdempotencyKey,
		SourceType:            request.SourceType,
		SourceId:              request.SourceId,
		SourceSnapshot:        request.SourceSnapshot,
		SourceKey:             request.SourceKey,
		SourceStatus:          request.SourceStatus,
		PlanId:                request.SourcePlanId,
		GrossCredit:           request.GrossCredit,
		NetCredit:             request.GrossCredit - debtOffset,
		AvailableCreditBefore: maxInt64(balanceBefore, 0),
		SettlementDebtBefore:  settlementDebtBefore,
		DebtOffset:            debtOffset,
		BalanceBefore:         balanceBefore,
		BalanceAfter:          balanceAfter,
		AvailableCreditAfter:  availableAfter,
		SettlementDebtAfter:   debtAfter,
		OperatorUserId:        request.OperatorUserId,
		PaymentProvider:       request.PaymentProvider,
		ParameterFingerprint:  request.ParameterFingerprint,
		TargetPlanId:          request.TargetPlanId,
		Reason:                request.Reason,
		CreatedAt:             getDBTimestampTx(tx),
	}
	if request.ValuationSource != nil {
		ledger.SourcePriceMicros = request.ValuationSource.SourcePriceMicros
		ledger.SourcePlanCredit = request.ValuationSource.SourcePlanCredit
		ledger.FxSourceCurrency = strings.ToUpper(strings.TrimSpace(request.ValuationSource.SourceCurrency))
		ledger.FxRateNumerator = 1
		ledger.FxRateDenominator = 1
		ledger.FxCapturedAt = ledger.CreatedAt
		ledger.FxDirection = CreditFXDirectionIdentity
	}
	if valuationReady {
		ledger.ValuationCurrency = valuationIngress.currency
		ledger.ValuationGrossCostMicros = valuationMutation.GrossCostMicros
		ledger.ValuationNetCostMicros = valuationMutation.NetCostMicros
		ledger.ValuationConfidence = valuationIngress.confidence
		ledger.ValuationRuleVersion = valuationIngress.ruleVersion
		ledger.ValuationStateVersionAfter = valuationMutation.StateVersionAfter
		ledger.FxSourceCurrency = valuationIngress.fxSourceCurrency
		ledger.FxRateNumerator = valuationIngress.fxRateNumerator
		ledger.FxRateDenominator = valuationIngress.fxRateDenominator
		ledger.FxCapturedAt = valuationIngress.fxCapturedAt
		if request.ValuationSource != nil && request.ValuationSource.FXRateSnapshot != nil {
			ledger.FxDirection = request.ValuationSource.FXRateSnapshot.Direction
		} else if strings.EqualFold(ledger.FxSourceCurrency, ledger.ValuationCurrency) {
			ledger.FxDirection = CreditFXDirectionIdentity
		}
	}
	if request.SourceType == CreditBalanceLedgerSourceSubscriptionConversion {
		source := request.ConversionSource
		ledger.SourcePlanId = source.SourcePlanId
		ledger.SourceTokenLimit = source.SourceTokenLimit
		ledger.SourceTokenUsed = source.SourceTokenUsed
		ledger.SourceStatus = strings.TrimSpace(source.SourceStatus)
		ledger.SourceStartTime = source.SourceStartTime
		ledger.SourceEndTime = source.SourceEndTime
		ledger.Full31DayBlocks = source.Full31DayBlocks
		ledger.CurrentRemainingCredit = source.CurrentRemainingCredit
		ledger.CreditBasisSource = strings.TrimSpace(source.CreditBasisSource)
		ledger.SourceDurationUnit = strings.TrimSpace(source.DurationUnit)
		ledger.SourceDurationValue = source.DurationValue
		ledger.SourceCustomSeconds = source.CustomSeconds
		ledger.SourceQuotaResetPeriod = NormalizeResetPeriod(source.QuotaResetPeriod)
		ledger.SourceQuotaResetCustomSeconds = source.QuotaResetCustomSeconds
		ledger.NetGrantedCredit = request.GrossCredit - debtOffset
		if valuationReady {
			ledger.ValuationSourcePriceMicros = request.ValuationSource.SourcePriceMicros
			ledger.ValuationCreditBasis = request.ValuationSource.SourcePlanCredit
			ledger.ValuationUnitValueNumeratorMicros = valuationIngress.unitValueNumeratorMicros
			ledger.ValuationUnitValueDenominator = valuationIngress.unitValueDenominator
		}
	}
	if err := tx.Create(&ledger).Error; err != nil {
		return nil, err
	}
	return creditBalanceGrantResult(&ledger, plan.Id, setting.ActiveSubscriptionId == balance.Id), nil
}

func findCreditBalanceGrantResultTx(tx *gorm.DB, request CreditBalanceGrantRequest) (*CreditBalanceGrantResult, bool, error) {
	var ledger CreditBalanceLedger
	query := tx.Where("(user_id = ? AND idempotency_key = ?) OR (source_type = ? AND source_id = ?)", request.UserId, request.IdempotencyKey, request.SourceType, request.SourceId).Limit(1).Find(&ledger)
	if query.Error != nil {
		return nil, false, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, false, nil
	}
	if ledger.UserId != request.UserId || ledger.IdempotencyKey != request.IdempotencyKey || ledger.SourceType != request.SourceType || ledger.SourceId != request.SourceId || ledger.SourceKey != request.SourceKey || ledger.SourceStatus != request.SourceStatus || ledger.PlanId != request.SourcePlanId || ledger.GrossCredit != request.GrossCredit || ledger.Type != request.Type || ledger.SourceSnapshot != request.SourceSnapshot || ledger.PaymentProvider != strings.TrimSpace(request.PaymentProvider) || ledger.ParameterFingerprint != request.ParameterFingerprint {
		return nil, false, ErrCreditValuationIdempotencyMismatch
	}
	if request.parameterFingerprint != "" && ledger.ParameterFingerprint != request.parameterFingerprint {
		return nil, false, ErrConversionIdempotencyConflict
	}
	var balance UserSubscription
	if err := tx.Select("id", "plan_id").Where("id = ?", ledger.UserSubscriptionId).First(&balance).Error; err != nil {
		return nil, false, err
	}
	if balance.PlanId != request.TargetPlanId {
		return nil, false, errors.New("credit balance idempotency target plan mismatch")
	}
	var user User
	if err := tx.Select("setting").Where("id = ?", request.UserId).First(&user).Error; err != nil {
		return nil, false, err
	}
	result := creditBalanceGrantResult(&ledger, balance.PlanId, user.GetSetting().ActiveSubscriptionId == balance.Id)
	result.Replayed = true
	return result, true, nil
}

func FindCreditBalanceGrantBySourceTx(tx *gorm.DB, sourceType string, sourceId int) (*CreditBalanceGrantResult, error) {
	if tx == nil || strings.TrimSpace(sourceType) == "" || sourceId <= 0 {
		return nil, errors.New("invalid credit balance source")
	}
	var ledger CreditBalanceLedger
	if err := tx.Where("source_type = ? AND source_id = ?", strings.TrimSpace(sourceType), sourceId).First(&ledger).Error; err != nil {
		return nil, err
	}
	var balance UserSubscription
	if err := tx.Select("id", "plan_id").Where("id = ?", ledger.UserSubscriptionId).First(&balance).Error; err != nil {
		return nil, err
	}
	var user User
	if err := tx.Select("setting").Where("id = ?", ledger.UserId).First(&user).Error; err != nil {
		return nil, err
	}
	return creditBalanceGrantResult(&ledger, balance.PlanId, user.GetSetting().ActiveSubscriptionId == balance.Id), nil
}

type CreditBalanceLedgerFilter struct {
	UserId     int
	SourceType string
	Type       string
	StartTime  int64
	EndTime    int64
	Limit      int
}

func ListCreditBalanceLedger(userId int, limit int) ([]CreditBalanceLedgerHistoryItem, error) {
	return ListCreditBalanceLedgerFiltered(CreditBalanceLedgerFilter{UserId: userId, Limit: limit})
}

func ListCreditBalanceLedgerFiltered(filter CreditBalanceLedgerFilter) ([]CreditBalanceLedgerHistoryItem, error) {
	if filter.UserId <= 0 {
		return nil, errors.New("invalid userId")
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 100
	}
	query := DB.Where("user_id = ?", filter.UserId)
	if sourceType := strings.TrimSpace(filter.SourceType); sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	if entryType := strings.TrimSpace(filter.Type); entryType != "" {
		query = query.Where("type = ?", entryType)
	}
	if filter.StartTime > 0 {
		query = query.Where("created_at >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("created_at <= ?", filter.EndTime)
	}
	var entries []CreditBalanceLedger
	if err := query.Order("id desc").Limit(filter.Limit).Find(&entries).Error; err != nil {
		return nil, err
	}
	return hydrateCreditBalanceLedgerHistory(filter.UserId, entries)
}

func hydrateCreditBalanceLedgerHistory(userId int, entries []CreditBalanceLedger) ([]CreditBalanceLedgerHistoryItem, error) {
	result := make([]CreditBalanceLedgerHistoryItem, len(entries))
	orderIds := make([]int, 0, len(entries))
	for index := range entries {
		result[index].CreditBalanceLedger = entries[index]
		if (entries[index].SourceType == CreditBalanceLedgerSourceSubscriptionOrder || entries[index].SourceType == CreditBalanceLedgerSourceSubscriptionOrderRecovery) && entries[index].SourceId > 0 {
			orderIds = append(orderIds, entries[index].SourceId)
		}
	}
	if len(orderIds) == 0 {
		return result, nil
	}
	var orders []SubscriptionOrder
	if err := DB.Select("id", "payment_provider", "payment_method", "entitlement_snapshot").Where("user_id = ? AND id IN ?", userId, orderIds).Find(&orders).Error; err != nil {
		return nil, err
	}
	ordersById := make(map[int]SubscriptionOrder, len(orders))
	for index := range orders {
		ordersById[orders[index].Id] = orders[index]
	}
	for index := range result {
		order, ok := ordersById[result[index].SourceId]
		if !ok {
			continue
		}
		result[index].CreditBalanceLedger.PaymentProvider = order.PaymentProvider
		result[index].PaymentMethod = order.PaymentMethod
		result[index].PurchaseMode = SubscriptionPurchaseModeCreditBalance
		if snapshot, err := UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot); err == nil {
			if mode, err := NormalizeSubscriptionPurchaseMode(snapshot.PurchaseMode); err == nil {
				result[index].PurchaseMode = mode
			}
		}
	}
	return result, nil
}

func getOrCreateCreditBalanceSubscriptionTx(tx *gorm.DB, userId int, plan *SubscriptionPlan) (*UserSubscription, bool, error) {
	var balance UserSubscription
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND entitlement_type = ?", userId, SubscriptionEntitlementCreditBalance).Limit(1).Find(&balance)
	if query.Error != nil {
		return nil, false, query.Error
	}
	if query.RowsAffected > 0 {
		return &balance, false, nil
	}
	now := getDBTimestampTx(tx)
	balance = UserSubscription{
		UserId:           userId,
		PlanId:           plan.Id,
		EntitlementType:  SubscriptionEntitlementCreditBalance,
		TokenLimit:       0,
		TokenUsed:        0,
		ConcurrencyLimit: plan.ConcurrencyLimit,
		GrantReason:      SubscriptionGrantOrder,
		StartTime:        now,
		EndTime:          0,
		Status:           "active",
		Source:           SubscriptionGrantOrder,
		LastResetTime:    0,
		NextResetTime:    0,
	}
	create := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&balance)
	if create.Error != nil {
		return nil, false, create.Error
	}
	if create.RowsAffected == 1 {
		return &balance, true, nil
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND entitlement_type = ?", userId, SubscriptionEntitlementCreditBalance).
		First(&balance).Error; err != nil {
		return nil, false, err
	}
	return &balance, false, nil
}

func hasUsableSubscriptionTx(tx *gorm.DB, userId int) (bool, error) {
	now := getDBTimestampTx(tx)
	var subscriptions []UserSubscription
	if err := tx.Where("user_id = ? AND status = ? AND ((entitlement_type = ? AND end_time > ?) OR entitlement_type = ?)", userId, "active", SubscriptionEntitlementTimed, now, SubscriptionEntitlementCreditBalance).Find(&subscriptions).Error; err != nil {
		return false, err
	}
	for i := range subscriptions {
		subscription := &subscriptions[i]
		plan, err := getSubscriptionPlanByIdTx(tx, subscription.PlanId)
		if err != nil {
			return false, err
		}
		if !plan.Enabled {
			continue
		}
		if usable, _ := isBillableSubscriptionCandidate(subscription, plan, 1); usable {
			return true, nil
		}
	}
	return false, nil
}

func creditBalanceGrantResult(ledger *CreditBalanceLedger, planId int, active bool) *CreditBalanceGrantResult {
	return &CreditBalanceGrantResult{
		UserSubscriptionId:         ledger.UserSubscriptionId,
		PlanId:                     planId,
		GrossCredit:                ledger.GrossCredit,
		NetCredit:                  ledger.NetCredit,
		GrossAmountMicros:          ledger.ValuationGrossCostMicros,
		NetAmountMicros:            ledger.ValuationNetCostMicros,
		ValuationCurrency:          ledger.ValuationCurrency,
		SourceCurrency:             ledger.FxSourceCurrency,
		ValuationConfidence:        ledger.ValuationConfidence,
		FxRateNumerator:            ledger.FxRateNumerator,
		FxRateDenominator:          ledger.FxRateDenominator,
		FxCapturedAt:               ledger.FxCapturedAt,
		FxDirection:                ledger.FxDirection,
		ValuationRuleVersion:       ledger.ValuationRuleVersion,
		ValuationStateVersionAfter: ledger.ValuationStateVersionAfter,
		DebtOffset:                 ledger.DebtOffset,
		ConsumedAvailableCredit:    ledger.ConsumedAvailableCredit,
		DebtFormed:                 creditBalanceLedgerDebtFormed(ledger),
		RemovedExactCostMicros:     ledger.RemovedExactCostMicros,
		RemovedEstimatedCostMicros: ledger.RemovedEstimatedCostMicros,
		RemovedUnknownCredit:       ledger.RemovedUnknownCredit,
		Operation:                  ledger.Operation,
		TerminalState:              ledger.TerminalState,
		AvailableCredit:            ledger.AvailableCreditAfter,
		SettlementDebt:             ledger.SettlementDebtAfter,
		BalanceBefore:              ledger.BalanceBefore,
		BalanceAfter:               ledger.BalanceAfter,
		Active:                     active,
		LedgerId:                   ledger.Id,
		Status:                     creditBalanceStatus(ledger.BalanceAfter),
	}
}

func creditBalanceLedgerDebtFormed(ledger *CreditBalanceLedger) int64 {
	if ledger == nil {
		return 0
	}
	if ledger.SettlementDebtFormed != 0 {
		return ledger.SettlementDebtFormed
	}
	return ledger.DebtFormed
}

func creditBalanceStatus(signedBalance int64) string {
	if signedBalance < 0 {
		return CreditBalanceStatusDebt
	}
	if signedBalance == 0 {
		return CreditBalanceStatusExhausted
	}
	return CreditBalanceStatusAvailable
}

func marshalUserSetting(setting interface{}) (string, error) {
	payload, err := common.Marshal(setting)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func minInt64(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func ValidateCreditBalancePurchaseOption(plan *SubscriptionPlan, creditPlan *SubscriptionPlan) error {
	if plan == nil || creditPlan == nil {
		return errors.New("Credit 余额购买配置不存在")
	}
	if plan.EntitlementType == SubscriptionEntitlementCreditBalance {
		return errors.New("Credit 余额套餐不能作为充值选项")
	}
	if !plan.Enabled || !plan.PublicVisible {
		return errors.New("套餐不可购买")
	}
	if plan.DurationUnit != SubscriptionDurationMonth || plan.DurationValue != 1 || NormalizeResetPeriod(plan.QuotaResetPeriod) != SubscriptionResetMonthly || plan.MonthlyTokenLimit <= 0 || plan.IsTrial || plan.InviteTrial {
		return errors.New("只有标准单月计时套餐可购买 Credit 余额")
	}
	if !plan.UnlimitedPurchaseEnabled {
		return errors.New("该套餐未开启 Credit 余额购买资格")
	}
	if !creditPlan.Enabled || !creditPlan.CreditBalanceConfigured || !creditPlan.CreditBalancePurchaseEnabled {
		return errors.New("Credit 余额购买入口未开启")
	}
	return nil
}

func ValidateCreditBalanceRedemptionOption(plan *SubscriptionPlan, creditPlan *SubscriptionPlan) error {
	if plan == nil || creditPlan == nil {
		return ErrCreditBalanceRedemptionUnavailable
	}
	if !creditPlan.Enabled || !creditPlan.CreditBalanceConfigured || !creditPlan.CreditBalanceRedemptionEnabled {
		return ErrCreditBalanceRedemptionUnavailable
	}
	if !plan.Enabled || strings.TrimSpace(plan.EntitlementType) != SubscriptionEntitlementTimed {
		return ErrRedemptionPlanIneligible
	}
	if plan.DurationUnit != SubscriptionDurationMonth || plan.DurationValue != 1 || NormalizeResetPeriod(plan.QuotaResetPeriod) != SubscriptionResetMonthly || plan.MonthlyTokenLimit <= 0 || plan.IsTrial || plan.InviteTrial || !plan.UnlimitedPurchaseEnabled {
		return ErrRedemptionPlanIneligible
	}
	return nil
}

func CreditBalanceStateForUser(userId int, activeSubscriptionId int) (*CreditBalanceGrantResult, *UserSubscription, error) {
	if userId <= 0 {
		return nil, nil, errors.New("invalid userId")
	}
	var balance UserSubscription
	query := DB.Where("user_id = ? AND entitlement_type = ?", userId, SubscriptionEntitlementCreditBalance).Limit(1).Find(&balance)
	if query.Error != nil {
		return nil, nil, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, nil, nil
	}
	if balance.TokenLimit < 0 || balance.TokenUsed < 0 {
		return nil, nil, fmt.Errorf("invalid credit balance aggregate for subscription %d", balance.Id)
	}
	signed := balance.TokenLimit - balance.TokenUsed
	state := &CreditBalanceGrantResult{
		UserSubscriptionId: balance.Id,
		PlanId:             balance.PlanId,
		AvailableCredit:    maxInt64(signed, 0),
		SettlementDebt:     maxInt64(-signed, 0),
		BalanceAfter:       signed,
		Active:             activeSubscriptionId == balance.Id,
		Status:             creditBalanceStatus(signed),
	}
	return state, &balance, nil
}
