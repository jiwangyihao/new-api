package model

import "gorm.io/gorm"

const CreditValuationRuleVersion = 1

type CreditValuationState struct {
	UserSubscriptionId  int    `json:"user_subscription_id" gorm:"primaryKey;autoIncrement:false"`
	UserId              int    `json:"user_id" gorm:"not null;uniqueIndex:uidx_credit_valuation_states_user_id"`
	AvailableCredit     int64  `json:"available_credit" gorm:"type:bigint;not null;default:0"`
	ExactCostMicros     int64  `json:"exact_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	EstimatedCostMicros int64  `json:"estimated_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	UnknownCredit       int64  `json:"unknown_credit" gorm:"type:bigint;not null;default:0"`
	Currency            string `json:"currency" gorm:"type:varchar(8);not null"`
	RuleVersion         int    `json:"rule_version" gorm:"not null"`
	MigrationVersion    int    `json:"migration_version" gorm:"not null;default:0"`
	StateVersion        int64  `json:"state_version" gorm:"type:bigint;not null"`
	LastMutationType    string `json:"last_mutation_type" gorm:"type:varchar(32);not null;default:''"`
	CreatedAt           int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt           int64  `json:"updated_at" gorm:"type:bigint;not null;index:idx_credit_valuation_states_updated_at"`
}

func migrateCreditValuationSchema(db *gorm.DB) error {
	if db == nil {
		return ErrDatabase
	}
	return db.AutoMigrate(
		&CreditValuationState{},
		&CreditValuationMigration{},
		&TimedSubscriptionValuationGrant{},
		&SubscriptionPreConsumeRecord{},
		&CreditBalanceLedger{},
		&SubscriptionConversion{},
	)
}
