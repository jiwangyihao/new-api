package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRedeemLegacySubscriptionWithoutSnapshotRejectsWithoutWrites(t *testing.T) {
	setupTimedSubscriptionValuationTestDB(t)
	require.NoError(t, DB.AutoMigrate(&InvitationRewardEvent{}, &CreditBalanceLedger{}, &Log{}))
	priceMicros := int64(40_000_000)
	user := User{Id: 21_801, Username: "legacy-redemption", Status: common.UserStatusEnabled, AffCode: "legacy-redemption-aff"}
	plan := SubscriptionPlan{
		Id: 21_802, Title: "Legacy source", Enabled: true, EntitlementType: SubscriptionEntitlementTimed,
		PriceAmount: 40, PriceAmountMicros: &priceMicros, Currency: "CNY",
		DurationUnit: SubscriptionDurationCustom, CustomSeconds: 3600,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: SubscriptionResetNever,
	}
	redemption := Redemption{Id: 21_803, Key: "legacy-without-snapshot", Type: RedemptionTypeSubscription, PlanId: plan.Id, Status: common.RedemptionCodeStatusEnabled}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&plan).Error)
	require.NoError(t, DB.Create(&redemption).Error)
	newPriceMicros := int64(80_000_000)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{
		"price_amount": 80, "price_amount_micros": newPriceMicros, "currency": "USD",
	}).Error)

	result, err := Redeem(redemption.Key, user.Id, RedemptionModeTimed)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrRedemptionPlanIneligible)
	var saved Redemption
	require.NoError(t, DB.First(&saved, redemption.Id).Error)
	require.Equal(t, common.RedemptionCodeStatusEnabled, saved.Status)
	require.Zero(t, saved.UsedUserId)
	require.Zero(t, saved.RedeemedTime)
	require.Empty(t, saved.FulfillmentMode)
	require.Empty(t, saved.FulfillmentSnapshot)
	require.Zero(t, saved.FulfillmentSubscriptionId)
	var subscriptionCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Count(&subscriptionCount).Error)
	require.Zero(t, subscriptionCount)
	var grantCount int64
	require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
	require.Zero(t, grantCount)
}

func TestRedeemUsedSubscriptionRejectsConflictingModeWithoutWrites(t *testing.T) {
	tests := []struct {
		name      string
		firstMode string
		otherMode string
	}{
		{name: "timed to credit balance", firstMode: RedemptionModeTimed, otherMode: RedemptionModeCreditBalance},
		{name: "credit balance to timed", firstMode: RedemptionModeCreditBalance, otherMode: RedemptionModeTimed},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupTimedSubscriptionValuationTestDB(t)
			require.NoError(t, DB.AutoMigrate(&InvitationRewardEvent{}, &CreditBalanceLedger{}, &Log{}))
			userID := 21_820 + index*10
			optionPlanID := userID + 1
			redemptionID := userID + 2
			require.NoError(t, DB.Create(&User{Id: userID, Username: "mode-conflict", Status: common.UserStatusEnabled, AffCode: "mode-conflict-aff"}).Error)
			priceMicros := int64(40_000_000)
			option := SubscriptionPlan{
				Id: optionPlanID, Title: "Mode option", Enabled: true, EntitlementType: SubscriptionEntitlementTimed,
				PriceAmount: 40, PriceAmountMicros: &priceMicros, Currency: "CNY",
				DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
				MonthlyTokenLimit: 2700, QuotaResetPeriod: SubscriptionResetMonthly,
				UnlimitedPurchaseEnabled: true,
			}
			require.NoError(t, DB.Create(&option).Error)
			if test.firstMode == RedemptionModeCreditBalance {
				require.NoError(t, DB.Create(&SubscriptionPlan{
					Id: optionPlanID + 100, Title: "Credit balance", Enabled: true,
					EntitlementType:         SubscriptionEntitlementCreditBalance,
					CreditBalanceConfigured: true, CreditBalanceRedemptionEnabled: true,
				}).Error)
			}
			redemption := Redemption{Id: redemptionID, Key: "mode-conflict-" + test.firstMode, Type: RedemptionTypeSubscription, PlanId: option.Id, Status: common.RedemptionCodeStatusEnabled}
			require.NoError(t, redemption.Insert())
			first, err := Redeem(redemption.Key, userID, test.firstMode)
			require.NoError(t, err)
			require.NotNil(t, first)
			require.False(t, first.Replayed)
			var before Redemption
			require.NoError(t, DB.First(&before, redemption.Id).Error)
			var subscriptionCountBefore int64
			require.NoError(t, DB.Model(&UserSubscription{}).Count(&subscriptionCountBefore).Error)
			var timedGrantCountBefore int64
			require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&timedGrantCountBefore).Error)
			var creditGrantCountBefore int64
			require.NoError(t, DB.Model(&CreditBalanceLedger{}).Count(&creditGrantCountBefore).Error)

			conflict, err := Redeem(redemption.Key, userID, test.otherMode)

			require.ErrorIs(t, err, ErrRedemptionAlreadyUsed)
			require.NotNil(t, conflict)
			var after Redemption
			require.NoError(t, DB.First(&after, redemption.Id).Error)
			require.Equal(t, before.Status, after.Status)
			require.Equal(t, before.FulfillmentMode, after.FulfillmentMode)
			require.Equal(t, before.FulfillmentSnapshot, after.FulfillmentSnapshot)
			require.Equal(t, before.FulfillmentSubscriptionId, after.FulfillmentSubscriptionId)
			var subscriptionCountAfter int64
			require.NoError(t, DB.Model(&UserSubscription{}).Count(&subscriptionCountAfter).Error)
			require.Equal(t, subscriptionCountBefore, subscriptionCountAfter)
			var timedGrantCountAfter int64
			require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&timedGrantCountAfter).Error)
			require.Equal(t, timedGrantCountBefore, timedGrantCountAfter)
			var creditGrantCountAfter int64
			require.NoError(t, DB.Model(&CreditBalanceLedger{}).Count(&creditGrantCountAfter).Error)
			require.Equal(t, creditGrantCountBefore, creditGrantCountAfter)
		})
	}
}

func TestRedeemDisabledTrialPlansRejectWithoutWrites(t *testing.T) {
	tests := []struct {
		name        string
		isTrial     bool
		inviteTrial bool
	}{
		{name: "trial", isTrial: true},
		{name: "invite trial", inviteTrial: true},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupTimedSubscriptionValuationTestDB(t)
			require.NoError(t, DB.AutoMigrate(&InvitationRewardEvent{}, &Log{}))
			userID := 21_850 + index*10
			planID := userID + 1
			redemptionID := userID + 2
			require.NoError(t, DB.Create(&User{Id: userID, Username: "disabled-trial", Status: common.UserStatusEnabled, AffCode: "disabled-trial-aff"}).Error)
			plan := SubscriptionPlan{
				Id: planID, Title: "Disabled trial", Enabled: true, EntitlementType: SubscriptionEntitlementTimed,
				DurationUnit: SubscriptionDurationHour, DurationValue: 1, TrialDurationHours: 1,
				MonthlyTokenLimit: 1000, QuotaResetPeriod: SubscriptionResetNever,
				IsTrial: test.isTrial, InviteTrial: test.inviteTrial,
			}
			require.NoError(t, DB.Create(&plan).Error)
			redemption := Redemption{Id: redemptionID, Key: "disabled-trial-" + test.name, Type: RedemptionTypeSubscription, PlanId: planID, Status: common.RedemptionCodeStatusEnabled}
			require.NoError(t, redemption.Insert())
			require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", planID).Update("enabled", false).Error)

			result, err := Redeem(redemption.Key, userID, RedemptionModeTimed)

			require.Nil(t, result)
			require.ErrorIs(t, err, ErrRedemptionPlanIneligible)
			var saved Redemption
			require.NoError(t, DB.First(&saved, redemptionID).Error)
			require.Equal(t, common.RedemptionCodeStatusEnabled, saved.Status)
			require.Zero(t, saved.UsedUserId)
			require.Empty(t, saved.FulfillmentMode)
			require.Zero(t, saved.FulfillmentSubscriptionId)
			var subscriptionCount int64
			require.NoError(t, DB.Model(&UserSubscription{}).Count(&subscriptionCount).Error)
			require.Zero(t, subscriptionCount)
			var grantCount int64
			require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
			require.Zero(t, grantCount)
			var eventCount int64
			require.NoError(t, DB.Model(&InvitationRewardEvent{}).Count(&eventCount).Error)
			require.Zero(t, eventCount)
		})
	}
}

func TestRedemptionUpdatePreservesCommittedFulfillmentAfterStaleRead(t *testing.T) {
	setupTimedSubscriptionValuationTestDB(t)
	require.NoError(t, DB.AutoMigrate(&InvitationRewardEvent{}, &Log{}))
	priceMicros := int64(40_000_000)
	user := User{Id: 21_881, Username: "stale-redemption", Status: common.UserStatusEnabled, AffCode: "stale-redemption-aff"}
	plan := SubscriptionPlan{
		Id: 21_882, Title: "Stale update", Enabled: true, EntitlementType: SubscriptionEntitlementTimed,
		PriceAmount: 40, PriceAmountMicros: &priceMicros, Currency: "CNY",
		DurationUnit: SubscriptionDurationCustom, CustomSeconds: 3600,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: SubscriptionResetNever,
	}
	redemption := Redemption{Id: 21_883, Key: "stale-update-redemption", Name: "before", Type: RedemptionTypeSubscription, PlanId: plan.Id, Status: common.RedemptionCodeStatusEnabled}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&plan).Error)
	require.NoError(t, redemption.Insert())
	var stale Redemption
	require.NoError(t, DB.First(&stale, redemption.Id).Error)
	first, err := Redeem(redemption.Key, user.Id, RedemptionModeTimed)
	require.NoError(t, err)
	require.NotNil(t, first)
	var committed Redemption
	require.NoError(t, DB.First(&committed, redemption.Id).Error)
	stale.Name = "after"

	require.NoError(t, stale.Update())

	var saved Redemption
	require.NoError(t, DB.First(&saved, redemption.Id).Error)
	require.Equal(t, "after", saved.Name)
	require.Equal(t, common.RedemptionCodeStatusUsed, saved.Status)
	require.Equal(t, user.Id, saved.UsedUserId)
	require.Equal(t, committed.RedeemedTime, saved.RedeemedTime)
	require.Equal(t, committed.FulfillmentMode, saved.FulfillmentMode)
	require.Equal(t, committed.FulfillmentSnapshot, saved.FulfillmentSnapshot)
	require.Equal(t, committed.FulfillmentSubscriptionId, saved.FulfillmentSubscriptionId)
	var grantCount int64
	require.NoError(t, DB.Model(&TimedSubscriptionValuationGrant{}).Count(&grantCount).Error)
	require.Equal(t, int64(1), grantCount)
}
