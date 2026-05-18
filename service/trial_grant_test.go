package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedTrialGrantPlanForTest(t *testing.T, id int, code string) *model.SubscriptionPlan {
	t.Helper()
	plan := &model.SubscriptionPlan{
		Id:                 id,
		Title:              code,
		DurationUnit:       model.SubscriptionDurationHour,
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
	require.NoError(t, model.DB.Create(plan).Error)
	return plan
}

func TestGrantTrialOnRegistration_InviteTrialWithoutTrialCode(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TrialCode{}, &model.TrialRedemption{}))
	plan := seedTrialGrantPlanForTest(t, 7701, "trial_24h")
	require.NoError(t, model.DB.Create(&model.User{Id: 7702, Username: "invitee_trial", Status: common.UserStatusEnabled, AffCode: "aff7702"}).Error)

	var sub *model.UserSubscription
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		created, err := GrantTrialOnRegistration(tx, TrialGrantInput{UserId: 7702, InviterId: 42})
		sub = created
		return err
	}))
	require.NotNil(t, sub)
	assert.Equal(t, plan.Id, sub.PlanId)
	assert.Equal(t, "invite_trial", sub.GrantReason)
	assert.Equal(t, 42, sub.GrantSourceUserId)
	assert.Equal(t, int64(0), sub.TokenLimit)
	assert.Equal(t, 1, sub.ConcurrencyLimit)
}

func TestGrantTrialOnRegistration_UsesInviteTrialPlanWhenConfigured(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TrialCode{}, &model.TrialRedemption{}))
	seedTrialGrantPlanForTest(t, 7721, "trial_early")
	invitePlan := seedTrialGrantPlanForTest(t, 7722, "trial_invite")
	require.NoError(t, model.DB.Model(invitePlan).Update("invite_trial", true).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 7723, Username: "invitee_configured_trial", Status: common.UserStatusEnabled, AffCode: "aff7723"}).Error)

	var sub *model.UserSubscription
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		created, err := GrantTrialOnRegistration(tx, TrialGrantInput{UserId: 7723, InviterId: 42})
		sub = created
		return err
	}))
	require.NotNil(t, sub)
	assert.Equal(t, invitePlan.Id, sub.PlanId)
	assert.Equal(t, "invite_trial", sub.GrantReason)
	assert.Equal(t, 42, sub.GrantSourceUserId)
}
func TestGrantTrialOnRegistration_ConsumesTrialCode(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TrialCode{}, &model.TrialRedemption{}))
	plan := seedTrialGrantPlanForTest(t, 7711, "trial_24h")
	require.NoError(t, model.DB.Create(&model.User{Id: 7712, Username: "code_trial", Status: common.UserStatusEnabled, AffCode: "aff7712"}).Error)
	require.NoError(t, model.DB.Create(&model.TrialCode{Id: 7713, Code: "CODE7713", PlanId: plan.Id, Enabled: true, MaxRedemptions: 1}).Error)

	var sub *model.UserSubscription
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		created, err := GrantTrialOnRegistration(tx, TrialGrantInput{UserId: 7712, TrialCode: " code7713 ", InviterId: 99})
		sub = created
		return err
	}))
	require.NotNil(t, sub)
	assert.Equal(t, "trial_code", sub.GrantReason)
	assert.Equal(t, 0, sub.GrantSourceUserId)

	var redemption model.TrialRedemption
	require.NoError(t, model.DB.Where("user_id = ?", 7712).First(&redemption).Error)
	assert.Equal(t, "CODE7713", redemption.Code)
}
