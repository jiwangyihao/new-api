package model

const (
	CreditValuationMigrationPending   = "pending"
	CreditValuationMigrationRunning   = "running"
	CreditValuationMigrationReady     = "ready"
	CreditValuationMigrationFailed    = "failed"
	CreditValuationMigrationSuspended = "suspended"
)

type CreditValuationMigration struct {
	Version             int    `json:"version" gorm:"primaryKey;autoIncrement:false"`
	Status              string `json:"status" gorm:"type:varchar(16);not null;index:idx_credit_valuation_migrations_status"`
	ValuationCurrency   string `json:"valuation_currency" gorm:"type:varchar(8);not null;default:''"`
	FxRateNumerator     int64  `json:"fx_rate_numerator,string" gorm:"type:bigint;not null;default:0"`
	FxRateDenominator   int64  `json:"fx_rate_denominator,string" gorm:"type:bigint;not null;default:0"`
	FxCapturedAt        int64  `json:"fx_captured_at" gorm:"type:bigint;not null;default:0"`
	CreditRowsTotal     int64  `json:"credit_rows_total" gorm:"type:bigint;not null;default:0"`
	CreditRowsEstimated int64  `json:"credit_rows_estimated" gorm:"type:bigint;not null;default:0"`
	CreditRowsUnknown   int64  `json:"credit_rows_unknown" gorm:"type:bigint;not null;default:0"`
	TimedRowsTotal      int64  `json:"timed_rows_total" gorm:"type:bigint;not null;default:0"`
	TimedRowsEstimated  int64  `json:"timed_rows_estimated" gorm:"type:bigint;not null;default:0"`
	TimedRowsUnknown    int64  `json:"timed_rows_unknown" gorm:"type:bigint;not null;default:0"`
	Checksum            string `json:"checksum" gorm:"type:varchar(64);not null;default:''"`
	StartedAt           int64  `json:"started_at" gorm:"type:bigint;not null;default:0"`
	CompletedAt         int64  `json:"completed_at" gorm:"type:bigint;not null;default:0"`
	FailureReason       string `json:"failure_reason" gorm:"type:text"`
	SuspendedAt         int64  `json:"suspended_at" gorm:"type:bigint;not null;default:0"`
	SuspendedReason     string `json:"suspended_reason" gorm:"type:text"`
}
