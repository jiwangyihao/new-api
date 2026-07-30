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
	ErrRedemptionModeRequired             = errors.New("套餐型兑换码必须明确选择兑换模式")
	ErrRedemptionModeInvalid              = errors.New("兑换模式必须是 timed 或 credit_balance")
	ErrCreditBalanceRedemptionUnavailable = errors.New("Credit 余额兑换入口未开启")
	ErrRedemptionPlanIneligible           = errors.New("该套餐不符合 Credit 余额兑换资格")
	ErrRedemptionAlreadyUsed              = errors.New("该兑换码已被使用")
)

// 2FA errors
var ErrTwoFANotEnabled = errors.New("2fa not enabled")
