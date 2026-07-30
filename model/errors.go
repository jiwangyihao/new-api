package model

import "errors"

// Common errors
var (
	ErrDatabase = errors.New("database error")
)

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
