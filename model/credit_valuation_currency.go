package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// AcquireCreditBalancePlanGuardTx is the shared serialization point for
// Credit allocation and valuation-currency mutation.
func AcquireCreditBalancePlanGuardTx(tx *gorm.DB) (*SubscriptionPlan, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	query := tx.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance)
	if tx.Dialector != nil && (tx.Dialector.Name() == "sqlite" || tx.Dialector.Name() == "sqlite3") {
		guard := tx.Model(&SubscriptionPlan{}).
			Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).
			UpdateColumn("conversion_guard_version", gorm.Expr("conversion_guard_version"))
		if guard.Error != nil {
			return nil, guard.Error
		}
		if guard.RowsAffected == 0 {
			return nil, gorm.ErrRecordNotFound
		}
	} else {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var plan SubscriptionPlan
	if err := query.First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func GuardCreditValuationCurrencyUpdateTx(tx *gorm.DB, currency string) (*SubscriptionPlan, error) {
	normalized, err := NormalizeCreditValuationCurrency(currency)
	if err != nil {
		return nil, err
	}
	plan, err := AcquireCreditBalancePlanGuardTx(tx)
	if err != nil {
		return nil, err
	}
	if plan.ValuationCurrency == nil || strings.EqualFold(*plan.ValuationCurrency, normalized) {
		return plan, nil
	}
	locked, err := CreditValuationCurrencyLockedTx(tx)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, ErrCreditValuationCurrencyLocked
	}
	return plan, nil
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
