package model

import (
	"errors"

	"gorm.io/gorm"
)

type TimedSubscriptionValuationGrant struct {
	Id                    int    `json:"id"`
	IdempotencyKey        string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex:uidx_timed_valuation_grants_idempotency_key"`
	UserSubscriptionId    int    `json:"user_subscription_id" gorm:"not null;index:idx_timed_valuation_grants_subscription_id"`
	UserId                int    `json:"user_id" gorm:"not null;index:idx_timed_valuation_grants_user_id"`
	PlanId                int    `json:"plan_id" gorm:"not null;index:idx_timed_valuation_grants_plan_id"`
	SourceType            string `json:"source_type" gorm:"type:varchar(32);not null;uniqueIndex:uidx_timed_valuation_grants_source,priority:1"`
	SourceKey             string `json:"source_key" gorm:"type:varchar(160);not null;uniqueIndex:uidx_timed_valuation_grants_source,priority:2"`
	SourceId              int    `json:"source_id" gorm:"not null;default:0"`
	EventStartTime        int64  `json:"event_start_time" gorm:"type:bigint;not null;default:0"`
	EventEndTime          int64  `json:"event_end_time" gorm:"type:bigint;not null;default:0"`
	GrantCredit           int64  `json:"grant_credit" gorm:"type:bigint;not null;default:0"`
	SourcePriceMicros     int64  `json:"source_price_micros,string" gorm:"type:bigint;not null;default:0"`
	SourceCurrency        string `json:"source_currency" gorm:"type:varchar(8);not null"`
	ValuationAmountMicros int64  `json:"valuation_amount_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationCurrency     string `json:"valuation_currency" gorm:"type:varchar(8);not null"`
	Confidence            string `json:"confidence" gorm:"type:varchar(16);not null"`
	RuleVersion           int    `json:"rule_version" gorm:"not null"`
	FxRateNumerator       int64  `json:"fx_rate_numerator,string" gorm:"type:bigint;not null"`
	FxRateDenominator     int64  `json:"fx_rate_denominator,string" gorm:"type:bigint;not null"`
	FxCapturedAt          int64  `json:"fx_captured_at" gorm:"type:bigint;not null;default:0"`
	SourceSnapshot        string `json:"source_snapshot" gorm:"type:text"`
	CreatedAt             int64  `json:"created_at" gorm:"type:bigint;not null"`
}

func (*TimedSubscriptionValuationGrant) BeforeUpdate(*gorm.DB) error {
	return errors.New("timed subscription valuation grant is immutable")
}

func (*TimedSubscriptionValuationGrant) BeforeDelete(*gorm.DB) error {
	return errors.New("timed subscription valuation grant is immutable")
}
