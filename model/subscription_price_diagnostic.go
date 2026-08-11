package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const (
	SubscriptionPlanPriceDiagnosticInvalid           = "invalid_decimal"
	SubscriptionPlanPriceDiagnosticNegative          = "negative"
	SubscriptionPlanPriceDiagnosticPrecision         = "precision_exceeds_six"
	SubscriptionPlanPriceDiagnosticOverflow          = "overflow"
	SubscriptionPlanPriceDiagnosticRoundtripMismatch = "roundtrip_mismatch"
)

type SubscriptionPlanPriceDiagnostic struct {
	PlanId int    `json:"plan_id"`
	Reason string `json:"reason"`
}

func subscriptionPlanPriceDiagnosticQuery(dialect string) (string, error) {
	cast := "CAST(price_amount AS TEXT)"
	if dialect == "mysql" {
		cast = "CAST(price_amount AS CHAR)"
	} else if dialect != "sqlite" && dialect != "postgres" {
		return "", fmt.Errorf("unsupported database dialect: %s", dialect)
	}
	return "SELECT id AS plan_id, " + cast + " AS price_text FROM subscription_plans WHERE price_amount_micros IS NULL ORDER BY id", nil
}

func sqliteSubscriptionPlanPriceRoundtripMatches(db *gorm.DB, planId int, amountMicros int64) (bool, error) {
	canonicalPrice := fmt.Sprintf("%d.%06d", amountMicros/amountMicrosPerUnit, amountMicros%amountMicrosPerUnit)
	var matches int
	err := db.Raw(
		`SELECT CASE WHEN price_amount = CAST(? AS NUMERIC) THEN 1 ELSE 0 END FROM subscription_plans WHERE id = ?`,
		canonicalPrice,
		planId,
	).Scan(&matches).Error
	return matches == 1, err
}
func DiagnosePendingSubscriptionPlanPrices(db *gorm.DB) ([]SubscriptionPlanPriceDiagnostic, error) {
	if db == nil {
		db = DB
	}
	var rows []struct {
		PlanId    int    `gorm:"column:plan_id"`
		PriceText string `gorm:"column:price_text"`
	}
	query, err := subscriptionPlanPriceDiagnosticQuery(db.Dialector.Name())
	if err != nil {
		return nil, err
	}
	if err := db.Raw(query).Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]SubscriptionPlanPriceDiagnostic, 0)
	for _, row := range rows {
		amountMicros, err := ParseDecimalAmountMicros(row.PriceText)
		if err == nil {
			if db.Dialector.Name() == "sqlite" {
				matches, compareErr := sqliteSubscriptionPlanPriceRoundtripMatches(db, row.PlanId, amountMicros)
				if compareErr != nil {
					return nil, compareErr
				}
				if !matches {
					result = append(result, SubscriptionPlanPriceDiagnostic{PlanId: row.PlanId, Reason: SubscriptionPlanPriceDiagnosticRoundtripMismatch})
				}
			}
			continue
		}
		reason := SubscriptionPlanPriceDiagnosticInvalid
		switch {
		case errors.Is(err, ErrSubscriptionPlanPriceNegative):
			reason = SubscriptionPlanPriceDiagnosticNegative
		case errors.Is(err, ErrSubscriptionPlanPricePrecision):
			reason = SubscriptionPlanPriceDiagnosticPrecision
		case errors.Is(err, ErrCreditValuationOverflow):
			reason = SubscriptionPlanPriceDiagnosticOverflow
		}
		result = append(result, SubscriptionPlanPriceDiagnostic{PlanId: row.PlanId, Reason: reason})
	}
	return result, nil
}
