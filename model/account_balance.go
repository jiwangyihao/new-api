package model

import (
	"errors"

	"gorm.io/gorm"
)

func DeductUserAccountBalanceTx(tx *gorm.DB, userId int, amount int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if userId <= 0 {
		return errors.New("invalid user id")
	}
	if amount <= 0 {
		return errors.New("invalid amount")
	}
	result := tx.Model(&User{}).
		Where("id = ? AND quota >= ?", userId, amount).
		Update("quota", gorm.Expr("quota - ?", amount))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("余额不足")
	}
	return nil
}
