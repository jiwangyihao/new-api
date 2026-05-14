package model

import (
	"errors"

	"gorm.io/gorm"
)

type distributorDefaultPlan struct {
	BusinessCode       string
	Title              string
	PriceAmount        float64
	Currency           string
	DurationUnit       string
	DurationValue      int
	MonthlyTokenLimit  int64
	ConcurrencyLimit   int
	IsTrial            bool
	PublicVisible      bool
	TrialDurationHours int
	RewardEligible     bool
	SortOrder          int
	QuotaResetPeriod   string
}

var distributorDefaultPlans = []distributorDefaultPlan{
	{BusinessCode: "trial_24h", Title: "Trial", PriceAmount: 0, Currency: "CNY", DurationUnit: SubscriptionDurationHour, DurationValue: 24, MonthlyTokenLimit: 0, ConcurrencyLimit: 1, IsTrial: true, PublicVisible: false, TrialDurationHours: 24, RewardEligible: false, SortOrder: 1000, QuotaResetPeriod: SubscriptionResetNever},
	{BusinessCode: "basic_monthly", Title: "Basic", PriceAmount: 40, Currency: "CNY", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1_000_000_000, ConcurrencyLimit: 1, IsTrial: false, PublicVisible: true, TrialDurationHours: 0, RewardEligible: true, SortOrder: 900, QuotaResetPeriod: SubscriptionResetMonthly},
	{BusinessCode: "standard_monthly", Title: "Standard", PriceAmount: 80, Currency: "CNY", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 2_000_000_000, ConcurrencyLimit: 5, IsTrial: false, PublicVisible: true, TrialDurationHours: 0, RewardEligible: true, SortOrder: 800, QuotaResetPeriod: SubscriptionResetMonthly},
	{BusinessCode: "pro_monthly", Title: "Pro", PriceAmount: 160, Currency: "CNY", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 5_000_000_000, ConcurrencyLimit: 10, IsTrial: false, PublicVisible: true, TrialDurationHours: 0, RewardEligible: true, SortOrder: 700, QuotaResetPeriod: SubscriptionResetMonthly},
	{BusinessCode: "max_monthly", Title: "Max", PriceAmount: 660, Currency: "CNY", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 10_000_000_000, ConcurrencyLimit: 50, IsTrial: false, PublicVisible: true, TrialDurationHours: 0, RewardEligible: true, SortOrder: 600, QuotaResetPeriod: SubscriptionResetMonthly},
}

func EnsureDistributorDefaultPlans() error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, seed := range distributorDefaultPlans {
			var count int64
			if err := tx.Model(&SubscriptionPlan{}).Where("business_code = ?", seed.BusinessCode).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			businessCode := seed.BusinessCode
			plan := &SubscriptionPlan{
				Title:              seed.Title,
				PriceAmount:        seed.PriceAmount,
				Currency:           seed.Currency,
				DurationUnit:       seed.DurationUnit,
				DurationValue:      seed.DurationValue,
				Enabled:            true,
				SortOrder:          seed.SortOrder,
				MonthlyTokenLimit:  seed.MonthlyTokenLimit,
				ConcurrencyLimit:   seed.ConcurrencyLimit,
				IsTrial:            seed.IsTrial,
				PublicVisible:      seed.PublicVisible,
				TrialDurationHours: seed.TrialDurationHours,
				RewardEligible:     seed.RewardEligible,
				BusinessCode:       &businessCode,
				QuotaResetPeriod:   seed.QuotaResetPeriod,
			}
			if err := tx.Select("Title", "PriceAmount", "Currency", "DurationUnit", "DurationValue", "Enabled", "SortOrder", "MonthlyTokenLimit", "ConcurrencyLimit", "IsTrial", "PublicVisible", "TrialDurationHours", "RewardEligible", "BusinessCode", "QuotaResetPeriod").Create(plan).Error; err != nil {
				return err
			}
			if err := tx.Model(plan).Updates(map[string]any{"is_trial": seed.IsTrial, "public_visible": seed.PublicVisible, "reward_eligible": seed.RewardEligible}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
