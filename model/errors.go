package model

import "errors"

// Common errors
var (
	ErrDatabase = errors.New("database error")
)

// Credit valuation errors
var (
	ErrCreditValuationDivisionByZero      = errors.New("credit_valuation_division_by_zero")
	ErrCreditValuationOverflow            = errors.New("credit_valuation_overflow")
	ErrCreditValuationNegativeInput       = errors.New("credit_valuation_negative_input")
	ErrCreditValuationCurrencyRequired    = errors.New("credit_valuation_currency_required")
	ErrCreditValuationUnsupportedCurrency = errors.New("credit_valuation_unsupported_currency")
	ErrCreditValuationCurrencyLocked      = errors.New("credit_valuation_currency_locked")
	ErrCreditValuationSourceRequired      = errors.New("credit_valuation_source_required")
	ErrCreditValuationSourceInvalid       = errors.New("credit_valuation_source_invalid")
	ErrCreditValuationStateMissing        = errors.New("credit_valuation_state_missing")
	ErrCreditValuationStateMismatch       = errors.New("credit_valuation_state_mismatch")
	ErrCreditValuationTargetConflict      = errors.New("credit_valuation_target_conflict")
	ErrCreditValuationMappingConflict     = errors.New("credit_valuation_mapping_conflict")
	ErrCreditValuationRequestNotFound     = errors.New("credit_valuation_request_not_found")
	ErrCreditValuationFinalizedConflict   = errors.New("credit_valuation_finalized_conflict")
	ErrCreditValuationBatchRolledBack     = errors.New("credit_valuation_batch_rolled_back")
	ErrCreditBalanceAllocationUnavailable = errors.New("credit_balance_allocation_unavailable")
	ErrCreditValuationPlanRequired        = errors.New("credit_valuation_plan_required")
	ErrCreditValuationPlanIneligible      = errors.New("credit_valuation_plan_ineligible")
	ErrCreditValuationInvalidFX           = errors.New("credit_valuation_invalid_fx")
	ErrCreditValuationIdempotencyMismatch = errors.New("credit_valuation_idempotency_mismatch")
	ErrSubscriptionPlanPriceInvalid       = errors.New("subscription_plan_price_invalid")
	ErrSubscriptionPlanPriceNegative      = errors.New("subscription_plan_price_negative")
	ErrSubscriptionPlanPricePrecision     = errors.New("subscription_plan_price_precision")
	ErrSubscriptionPlanPriceRequired      = errors.New("subscription_plan_price_micros_required")
	ErrSubscriptionPlanPriceMismatch      = errors.New("subscription_plan_price_mismatch")
)

var ErrConversionIdempotencyConflict = errors.New("subscription_conversion_idempotency_conflict")

// User auth errors
var (
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrUserEmptyCredentials = errors.New("empty credentials")
)

// Token auth errors
var (
	ErrTokenNotProvided = errors.New("token not provided")
	ErrTokenInvalid     = errors.New("token invalid")
)

// Redemption errors
var (
	ErrRedeemFailed                       = errors.New("redeem.failed")
	ErrRedemptionModeRequired             = errors.New("redemption.mode_required")
	ErrRedemptionModeInvalid              = errors.New("redemption.mode_invalid")
	ErrCreditBalanceRedemptionUnavailable = errors.New("redemption.credit_balance_unavailable")
	ErrRedemptionPlanIneligible           = errors.New("redemption.plan_ineligible")
	ErrRedemptionAlreadyUsed              = errors.New("redemption.used")
)

// 2FA errors
var ErrTwoFANotEnabled = errors.New("2fa not enabled")
