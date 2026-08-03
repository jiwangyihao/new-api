package model

import (
	"errors"

	"gorm.io/gorm"
)

const (
	SubscriptionPlanPriceDiagnosticInvalid   = "invalid_decimal"
	SubscriptionPlanPriceDiagnosticNegative  = "negative"
	SubscriptionPlanPriceDiagnosticPrecision = "precision_exceeds_six"
	SubscriptionPlanPriceDiagnosticOverflow  = "overflow"
)

type SubscriptionPlanPriceDiagnostic struct {
	PlanId int    `json:"plan_id"`
	Reason string `json:"reason"`
}

func DiagnosePendingSubscriptionPlanPrices(db *gorm.DB) ([]SubscriptionPlanPriceDiagnostic, error) {
	if db == nil {
		db = DB
	}
	var rows []struct {
		PlanId    int    `gorm:"column:plan_id"`
		PriceText string `gorm:"column:price_text"`
	}
	query := `SELECT id AS plan_id, CAST(price_amount AS TEXT) AS price_text FROM subscription_plans WHERE price_amount_micros IS NULL ORDER BY id`
	if err := db.Raw(query).Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]SubscriptionPlanPriceDiagnostic, 0)
	for _, row := range rows {
		_, err := ParseDecimalAmountMicros(row.PriceText)
		if err == nil {
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
