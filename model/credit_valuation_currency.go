package model

import (
	"strings"

	"gorm.io/gorm"
)

func NormalizeCreditValuationCurrency(currency string) (string, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return "", ErrCreditValuationCurrencyRequired
	}
	if currency != "CNY" && currency != "USD" {
		return "", ErrCreditValuationUnsupportedCurrency
	}
	return currency, nil
}

func CreditValuationCurrencyLockedTx(tx *gorm.DB) (bool, error) {
	if tx == nil {
		tx = DB
	}
	var count int64
	if err := tx.Model(&UserSubscription{}).
		Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).
		Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	for _, table := range []string{"credit_valuation_states", "credit_balance_ledgers"} {
		if !tx.Migrator().HasTable(table) {
			continue
		}
		if err := tx.Table(table).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}
