package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedTrialPlanForTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	code := "trial_24h"
	plan := &SubscriptionPlan{
		Id:                 id,
		Title:              "Trial",
		DurationUnit:       SubscriptionDurationHour,
		DurationValue:      24,
		Enabled:            true,
		MonthlyTokenLimit:  0,
		ConcurrencyLimit:   1,
		IsTrial:            true,
		PublicVisible:      false,
		TrialDurationHours: 24,
		RewardEligible:     false,
		BusinessCode:       &code,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func seedTrialCodeForTest(t *testing.T, id int, rawCode string, planId int) *TrialCode {
	t.Helper()
	trialCode := &TrialCode{Id: id, Code: rawCode, PlanId: planId, Enabled: true}
	require.NoError(t, DB.Create(trialCode).Error)
	return trialCode
}

func TestConsumeTrialCode_CreatesRedemptionAndNormalizesCode(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&TrialCode{}, &TrialRedemption{}))
	require.NoError(t, DB.Create(&User{Id: 7601, Username: "trial_user", Status: common.UserStatusEnabled}).Error)
	plan := seedTrialPlanForTest(t, 7602)
	seedTrialCodeForTest(t, 7603, " trial2026 ", plan.Id)

	var consumed *TrialCode
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		consumed, err = ConsumeTrialCode(tx, 7601, " trial2026 ")
		return err
	}))
	require.NotNil(t, consumed)
	assert.Equal(t, "TRIAL2026", consumed.Code)
	assert.Equal(t, 1, consumed.RedeemedCount)

	var redemption TrialRedemption
	require.NoError(t, DB.Where("user_id = ? AND trial_code_id = ?", 7601, consumed.Id).First(&redemption).Error)
	assert.Equal(t, "TRIAL2026", redemption.Code)
}

func TestConsumeTrialCode_RejectsDisabledExpiredMaxAndDuplicateTrial(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&TrialCode{}, &TrialRedemption{}))
	plan := seedTrialPlanForTest(t, 7611)
	require.NoError(t, DB.Create(&User{Id: 7612, Username: "trial_disabled", Status: common.UserStatusEnabled, AffCode: "aff7612"}).Error)
	require.NoError(t, DB.Create(&User{Id: 7613, Username: "trial_expired", Status: common.UserStatusEnabled, AffCode: "aff7613"}).Error)
	require.NoError(t, DB.Create(&User{Id: 7614, Username: "trial_max", Status: common.UserStatusEnabled, AffCode: "aff7614"}).Error)
	require.NoError(t, DB.Create(&User{Id: 7615, Username: "trial_duplicate", Status: common.UserStatusEnabled, AffCode: "aff7615"}).Error)

	disabled := &TrialCode{Id: 7621, Code: "DISABLED", PlanId: plan.Id, Enabled: false}
	expired := &TrialCode{Id: 7622, Code: "EXPIRED", PlanId: plan.Id, Enabled: true, ExpiresAt: common.GetTimestamp() - 1}
	maxed := &TrialCode{Id: 7623, Code: "MAXED", PlanId: plan.Id, Enabled: true, MaxRedemptions: 1, RedeemedCount: 1}
	duplicate := &TrialCode{Id: 7624, Code: "DUPLICATE", PlanId: plan.Id, Enabled: true}
	require.NoError(t, DB.Create(disabled).Error)
	require.NoError(t, DB.Create(expired).Error)
	require.NoError(t, DB.Create(maxed).Error)
	require.NoError(t, DB.Create(duplicate).Error)
	require.NoError(t, DB.Model(&TrialCode{}).Where("id = ?", disabled.Id).Update("enabled", false).Error)
	require.NoError(t, DB.Create(&UserSubscription{UserId: 7615, PlanId: plan.Id, Status: "active", StartTime: common.GetTimestamp(), EndTime: common.GetTimestamp() + 3600, GrantReason: "trial_code"}).Error)

	cases := []struct {
		name   string
		userId int
		code   string
	}{
		{name: "disabled", userId: 7612, code: "DISABLED"},
		{name: "expired", userId: 7613, code: "EXPIRED"},
		{name: "maxed", userId: 7614, code: "MAXED"},
		{name: "duplicate", userId: 7615, code: "DUPLICATE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := DB.Transaction(func(tx *gorm.DB) error {
				_, err := ConsumeTrialCode(tx, tc.userId, tc.code)
				return err
			})
			require.Error(t, err)
		})
	}
}

func TestAutoMigrateTrialCodeTables(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TrialCode{}, &TrialRedemption{}))
	assert.True(t, DB.Migrator().HasTable(&TrialCode{}))
	assert.True(t, DB.Migrator().HasTable(&TrialRedemption{}))
}

func TestReserveTrialCodeRedemptionSlotRejectsMaxedWithoutChangingCounter(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&TrialCode{}, &TrialRedemption{}))
	plan := seedTrialPlanForTest(t, 7641)
	trialCode := &TrialCode{Id: 7642, Code: "MAXGUARD", PlanId: plan.Id, Enabled: true, MaxRedemptions: 1, RedeemedCount: 1}
	require.NoError(t, DB.Create(trialCode).Error)

	err := DB.Transaction(func(tx *gorm.DB) error {
		return reserveTrialCodeRedemptionSlot(tx, trialCode, common.GetTimestamp())
	})
	require.Error(t, err)

	var after TrialCode
	require.NoError(t, DB.First(&after, trialCode.Id).Error)
	assert.Equal(t, 1, after.RedeemedCount)
}
