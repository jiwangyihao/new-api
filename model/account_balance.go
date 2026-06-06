package model

import (
	"errors"
	"math"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func AccountBalanceCentsFromCNY(amount decimal.Decimal) (int, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("invalid amount")
	}
	cents := amount.Mul(decimal.NewFromInt(100)).Round(0)
	if cents.LessThanOrEqual(decimal.Zero) || cents.GreaterThan(decimal.NewFromInt(int64(math.MaxInt))) {
		return 0, errors.New("invalid amount")
	}
	return int(cents.IntPart()), nil
}

func AccountBalanceIntFromCents(amountCents int64) (int, error) {
	if amountCents <= 0 || amountCents > int64(math.MaxInt) {
		return 0, errors.New("invalid amount")
	}
	return int(amountCents), nil
}

func AccountBalanceCNYFromCents(cents int) decimal.Decimal {
	return decimal.NewFromInt(int64(cents)).Div(decimal.NewFromInt(100))
}

func DeductUserAccountBalanceTx(tx *gorm.DB, userId int, cents int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if userId <= 0 {
		return errors.New("invalid user id")
	}
	if cents <= 0 {
		return errors.New("invalid amount")
	}
	result := tx.Model(&User{}).
		Where("id = ? AND quota >= ?", userId, cents).
		Update("quota", gorm.Expr("quota - ?", cents))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("余额不足")
	}
	return nil
}

func IncreaseUserAccountBalanceTx(tx *gorm.DB, userId int, cents int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if userId <= 0 {
		return errors.New("invalid user id")
	}
	if cents <= 0 {
		return errors.New("invalid amount")
	}
	result := tx.Model(&User{}).
		Where("id = ?", userId).
		Update("quota", gorm.Expr("quota + ?", cents))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("用户不存在")
	}
	return nil
}
