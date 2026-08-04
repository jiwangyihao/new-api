package service

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupInvitationCommissionServiceDB(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"_invitation_commission?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.UserSubscription{}, &model.Redemption{}, &model.InvitationMonthlyEntitlement{}, &model.InvitationRewardEvent{}, &model.InvitationCommissionAccount{}, &model.InvitationCommissionRecord{}, &model.InvitationCommissionLedger{}, &model.InvitationCommissionWithdrawal{}, &model.TopUp{}))
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		_ = sqlDB.Close()
	})
}

func seedCommissionRewardEvent(t *testing.T, inviterId int, inviteeId int, sourceId int, amountCents int64, currency string) model.InvitationRewardEvent {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{Id: inviterId, Username: fmt.Sprintf("inviter-%d", inviterId), Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("aff-%d", inviterId), InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: inviteeId, Username: fmt.Sprintf("invitee-%d", inviteeId), Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("aff-%d", inviteeId), InviterId: inviterId}).Error)
	now := common.GetTimestamp()
	planId := sourceId + 100000
	subscriptionId := sourceId + 200000
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planId, Title: fmt.Sprintf("commission-plan-%d", sourceId), PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subscriptionId, UserId: inviteeId, PlanId: planId, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	event := model.InvitationRewardEvent{InviterId: inviterId, InviteeId: inviteeId, SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: sourceId, SourceOrderId: sourceId, SourceSubscriptionId: subscriptionId, SourceAmountCents: amountCents, SourceCurrency: currency, EventStartTime: now, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.DB.Create(&event).Error)
	return event
}

func seedCommissionAccount(t *testing.T, userId int, available int64, pending int64, withdrawn int64, transferred int64) {
	t.Helper()
	account := model.InvitationCommissionAccount{UserId: userId, AvailableCents: available, PendingCents: pending, WithdrawnCents: withdrawn, TransferredCents: transferred, CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(&account).Error)
}

func setInvitationCommissionSettingForTest(t *testing.T, value operation_setting.InvitationCommissionSetting) {
	t.Helper()
	setting := operation_setting.GetInvitationCommissionSetting()
	old := *setting
	*setting = value
	t.Cleanup(func() { *setting = old })
}

func TestCreateInvitationCommissionRequiresTimedPlanAndSubscriptionIdentity(t *testing.T) {
	tests := []struct {
		name                    string
		planEntitlement         string
		subscriptionEntitlement string
	}{
		{name: "Credit plan with timed subscription", planEntitlement: model.SubscriptionEntitlementCreditBalance, subscriptionEntitlement: model.SubscriptionEntitlementTimed},
		{name: "timed plan with Credit subscription", planEntitlement: model.SubscriptionEntitlementTimed, subscriptionEntitlement: model.SubscriptionEntitlementCreditBalance},
		{name: "Credit plan with Credit subscription", planEntitlement: model.SubscriptionEntitlementCreditBalance, subscriptionEntitlement: model.SubscriptionEntitlementCreditBalance},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupInvitationCommissionServiceDB(t)
			setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
			event := seedCommissionRewardEvent(t, 9391, 9392, 9393, 10000, "CNY")
			require.NoError(t, model.DB.Model(&model.UserSubscription{}).
				Where("id = ?", event.SourceSubscriptionId).
				UpdateColumn("entitlement_type", test.subscriptionEntitlement).Error)
			require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).
				Where("id = ?", event.SourceId+100000).
				UpdateColumn("entitlement_type", test.planEntitlement).Error)

			require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
			require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

			var records int64
			require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
			assert.Zero(t, records)
			var accounts int64
			require.NoError(t, model.DB.Model(&model.InvitationCommissionAccount{}).Where("user_id = ?", event.InviterId).Count(&accounts).Error)
			assert.Zero(t, accounts)
		})
	}
}

func TestConvertedTimedPurchasePreservesRewardHistoryUntilRealRefund(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.CreditBalanceLedger{}, &model.SubscriptionConversion{}))
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})

	const inviterID = 9_901
	const convertedInviteeID = 9_902
	const activeInviteeID = 9_903
	const timedPlanID = 9_904
	const creditPlanID = 9_905
	const sourceSubscriptionID = 9_906
	now := common.GetTimestamp()
	timedCode := "converted-invitation-history"
	creditCode := "converted-invitation-credit"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id: timedPlanID, Title: "Converted invitation history", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, RewardEligible: true, BusinessCode: &timedCode,
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100,
		TimedConversionEnabled: true,
	}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id: creditPlanID, Title: "Converted invitation Credit", EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Enabled: true, BusinessCode: &creditCode, CreditBalanceConfigured: true,
		CreditBalanceConversionEnabled: true,
	}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: inviterID, Username: "converted-history-inviter", Status: common.UserStatusEnabled,
		AffCode: "converted-history-inviter", InvitationRewardMode: model.InvitationRewardModeCommission,
	}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: convertedInviteeID, Username: "converted-history-invitee", Status: common.UserStatusEnabled,
		AffCode: "converted-history-invitee", InviterId: inviterID,
	}).Error)
	basis := int64(100)
	source := model.UserSubscription{
		Id: sourceSubscriptionID, UserId: convertedInviteeID, PlanId: timedPlanID,
		EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusActive,
		TokenLimit: 100, TokenUsed: 10, StartTime: now - 48*60*60, EndTime: now + 60*60,
		GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
		LastGrantedAt: now - 48*60*60, LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: model.SubscriptionGrantTimeSourceLive, LastGrantSource: model.SubscriptionGrantOrder,
	}
	require.NoError(t, model.DB.Create(&source).Error)
	snapshot := model.NewSubscriptionEntitlementSnapshot(&model.SubscriptionPlan{
		Id: timedPlanID, Title: "Converted invitation history", EntitlementType: model.SubscriptionEntitlementTimed,
		Enabled: true, RewardEligible: true, BusinessCode: &timedCode,
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100,
		TimedConversionEnabled: true,
	}, model.SubscriptionPurchaseModeTimed, 0)
	snapshot.SetPaymentSnapshot(model.PaymentProviderStripe, "price-converted-invitation", model.PaymentMethodStripe, 10000, "CNY")
	snapshotJSON, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := model.SubscriptionOrder{
		UserId: convertedInviteeID, PlanId: timedPlanID, TradeNo: "converted-invitation-order",
		AmountCents: 10000, Currency: "CNY", PaymentProvider: model.PaymentProviderStripe,
		PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusSuccess, CompleteTime: now,
		FulfilledSubscriptionID: sourceSubscriptionID, EntitlementSnapshot: snapshotJSON,
	}
	require.NoError(t, model.DB.Create(&order).Error)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		_, recordErr := model.RecordInvitationRewardEventForSubscriptionOrderTx(tx, &order, &model.SubscriptionPlan{
			Id: timedPlanID, Title: "Converted invitation history", EntitlementType: model.SubscriptionEntitlementTimed,
			Enabled: true, RewardEligible: true, BusinessCode: &timedCode,
			DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
			QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 100,
			TimedConversionEnabled: true,
		}, &model.UserSubscriptionCreationResult{
			Subscription: &source, EventStartTime: now - 60, EventEndTime: now + 60*60,
		}, true)
		return recordErr
	}))
	var event model.InvitationRewardEvent
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error)
	historicalEventCreatedAt := now - 10
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("id = ?", event.Id).UpdateColumn("created_at", historicalEventCreatedAt).Error)
	event.CreatedAt = historicalEventCreatedAt
	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
	var commission model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&commission).Error)
	require.Equal(t, model.InvitationCommissionStatusAvailable, commission.Status)
	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", inviterID).First(&account).Error)
	require.Equal(t, int64(1000), account.AvailableCents)

	delayedOrder := model.SubscriptionOrder{
		Id: 90_001, UserId: convertedInviteeID, PlanId: timedPlanID, TradeNo: "converted-invitation-delayed-order",
		AmountCents: 5000, Currency: "CNY", PaymentProvider: model.PaymentProviderStripe,
		PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusSuccess, CompleteTime: now - 5,
		FulfilledSubscriptionID: sourceSubscriptionID, EntitlementSnapshot: snapshotJSON,
	}
	require.NoError(t, model.DB.Create(&delayedOrder).Error)
	delayedEvent := model.InvitationRewardEvent{
		InviterId: inviterID, InviteeId: convertedInviteeID,
		SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: delayedOrder.Id,
		SourceOrderId: delayedOrder.Id, SourceSubscriptionId: sourceSubscriptionID,
		SourceAmountCents: delayedOrder.AmountCents, SourceCurrency: delayedOrder.Currency,
		EventStartTime: now - 5, EventEndTime: now + 60*60,
		Status: model.InvitationRewardEventStatusActive, CreatedAt: now - 5, UpdatedAt: now - 5,
	}
	require.NoError(t, model.DB.Create(&delayedEvent).Error)

	conversion, err := model.ConfirmTimedSubscriptionConversion(convertedInviteeID, sourceSubscriptionID, "converted-invitation-history")
	require.NoError(t, err)
	require.False(t, conversion.Replayed)
	require.Less(t, event.CreatedAt, conversion.Conversion.ConvertedAt)
	require.Less(t, delayedEvent.CreatedAt, conversion.Conversion.ConvertedAt)
	require.NoError(t, model.DB.First(&event, event.Id).Error)
	assert.Equal(t, model.InvitationRewardEventStatusActive, event.Status)
	require.NoError(t, model.DB.First(&commission, commission.Id).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, commission.Status)
	assert.Equal(t, int64(1000), commission.CommissionCents)
	require.NoError(t, model.DB.First(&account, account.Id).Error)
	assert.Equal(t, int64(1000), account.AvailableCents)

	require.NoError(t, CreateInvitationCommissionForRewardEvent(delayedEvent.Id))
	var delayedCommission model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("event_id = ?", delayedEvent.Id).First(&delayedCommission).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, delayedCommission.Status)
	assert.Equal(t, int64(500), delayedCommission.CommissionCents)
	require.NoError(t, model.DB.First(&account, account.Id).Error)
	assert.Equal(t, int64(1500), account.AvailableCents)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("invitee_id = ?", convertedInviteeID).Count(&eventCount).Error)
	assert.Equal(t, int64(2), eventCount, "conversion must not create another reward event")
	for index, createdAt := range []int64{conversion.Conversion.ConvertedAt, conversion.Conversion.ConvertedAt + 1} {
		futureEvent := model.InvitationRewardEvent{
			InviterId: inviterID, InviteeId: convertedInviteeID,
			SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: order.Id + 100_000 + index,
			SourceSubscriptionId: sourceSubscriptionID, SourceAmountCents: 10000, SourceCurrency: "CNY",
			EventStartTime: createdAt, EventEndTime: createdAt + 60,
			Status: model.InvitationRewardEventStatusActive, CreatedAt: createdAt, UpdatedAt: createdAt,
		}
		require.NoError(t, model.DB.Create(&futureEvent).Error)
		require.NoError(t, CreateInvitationCommissionForRewardEvent(futureEvent.Id))
		var futureCommissionCount int64
		require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", futureEvent.Id).Count(&futureCommissionCount).Error)
		assert.Zero(t, futureCommissionCount, "events created at or after conversion must not earn commission")
	}

	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviterID).Update("invitation_reward_mode", model.InvitationRewardModeSubscription).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: activeInviteeID, Username: "converted-history-active", Status: common.UserStatusEnabled,
		AffCode: "converted-history-active", InviterId: inviterID,
	}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id: 19_907, UserId: activeInviteeID, PlanId: timedPlanID,
		EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusActive,
		StartTime: now - 60, EndTime: now + 60*60,
		GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
	}).Error)
	qualification, err := EnsureMonthlyInvitationEntitlement(inviterID, time.Unix(now, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, 2, qualification.DirectInviteCount)
	assert.Equal(t, 1, qualification.QualifiedActiveCount)
	assert.False(t, qualification.Entitled)
	assert.Zero(t, qualification.RewardSubscriptionId)

	recovery, err := model.RecoverSubscriptionOrder(model.SubscriptionOrderRecoveryRequest{
		TradeNo: order.TradeNo, ExpectedPaymentProvider: model.PaymentProviderStripe,
		RecoveryType: model.SubscriptionOrderRecoveryRefund, Reason: "real payment refund after conversion",
	})
	require.NoError(t, err)
	require.False(t, recovery.Replayed)
	require.NoError(t, model.DB.First(&event, event.Id).Error)
	assert.Equal(t, model.InvitationRewardEventStatusCancelled, event.Status)
	require.NoError(t, model.DB.First(&commission, commission.Id).Error)
	assert.Equal(t, model.InvitationCommissionStatusCancelled, commission.Status)
	assert.Equal(t, model.InvitationCommissionReversalStatusRecovered, commission.ReversalStatus)
	assert.Equal(t, int64(1000), commission.RecoveredCents)
	require.NoError(t, model.DB.First(&account, account.Id).Error)
	assert.Equal(t, int64(500), account.AvailableCents)
	require.NoError(t, model.DB.First(&delayedCommission, delayedCommission.Id).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, delayedCommission.Status)
	require.NoError(t, model.DB.First(&delayedEvent, delayedEvent.Id).Error)
	assert.Equal(t, model.InvitationRewardEventStatusActive, delayedEvent.Status)
}

func TestCreditFulfillmentPathsDoNotCreateInvitationBenefits(t *testing.T) {
	tests := []struct {
		name string
		kind string
	}{
		{name: "paid Credit purchase", kind: "purchase"},
		{name: "Credit redemption", kind: "redemption"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupInvitationCommissionServiceDB(t)
			require.NoError(t, model.DB.AutoMigrate(&model.CreditBalanceLedger{}, &model.Log{}))
			setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})

			const inviterID = 20_001
			const creditInviteeID = 20_002
			const timedInviteeID = 20_003
			const optionPlanID = 20_004
			const creditPlanID = 20_005
			now := common.GetTimestamp()
			optionCode := "credit-invitation-option"
			creditCode := "credit-invitation-balance"
			optionPriceMicros := int64(30_000_000)
			optionPlan := model.SubscriptionPlan{
				Id: optionPlanID, Title: "Credit invitation option", EntitlementType: model.SubscriptionEntitlementTimed,
				Enabled: true, RewardEligible: true, BusinessCode: &optionCode,
				DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
				QuotaResetPeriod: model.SubscriptionResetMonthly, MonthlyTokenLimit: 500,
				UnlimitedPurchaseEnabled: true,
				PriceAmount:              30, PriceAmountMicros: &optionPriceMicros, Currency: "CNY",
			}
			creditPlan := model.SubscriptionPlan{
				Id: creditPlanID, Title: "Credit invitation balance", EntitlementType: model.SubscriptionEntitlementCreditBalance,
				Enabled: true, RewardEligible: true, BusinessCode: &creditCode,
				CreditBalanceConfigured: true, CreditBalancePurchaseEnabled: true,
				CreditBalanceRedemptionEnabled: true,
			}
			require.NoError(t, model.DB.Create(&optionPlan).Error)
			require.NoError(t, model.DB.Create(&creditPlan).Error)
			require.NoError(t, model.DB.Create(&model.User{
				Id: inviterID, Username: "credit-path-inviter", Status: common.UserStatusEnabled,
				AffCode: "credit-path-inviter", InvitationRewardMode: model.InvitationRewardModeCommission,
			}).Error)
			require.NoError(t, model.DB.Create(&model.User{
				Id: creditInviteeID, Username: "credit-path-invitee", Status: common.UserStatusEnabled,
				AffCode: "credit-path-invitee", InviterId: inviterID,
			}).Error)
			require.NoError(t, model.DB.Create(&model.User{
				Id: timedInviteeID, Username: "credit-path-control", Status: common.UserStatusEnabled,
				AffCode: "credit-path-control", InviterId: inviterID,
			}).Error)
			require.NoError(t, model.DB.Create(&model.UserSubscription{
				Id: 20_006, UserId: timedInviteeID, PlanId: optionPlanID,
				EntitlementType: model.SubscriptionEntitlementTimed, Status: model.SubscriptionStatusActive,
				StartTime: now - 60, EndTime: now + 60*60,
				GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
			}).Error)

			switch test.kind {
			case "purchase":
				snapshot := model.NewSubscriptionEntitlementSnapshot(&optionPlan, model.SubscriptionPurchaseModeCreditBalance, creditPlanID)
				snapshot.SetTargetCreditBalancePlanSnapshot(&creditPlan)
				snapshot.SetPaymentSnapshot(model.PaymentProviderStripe, "price-credit-invitation", model.PaymentMethodStripe, 3000, "CNY")
				snapshotJSON, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
				require.NoError(t, err)
				order := model.SubscriptionOrder{
					UserId: creditInviteeID, PlanId: optionPlanID, TradeNo: "credit-invitation-purchase",
					AmountCents: 3000, Currency: "CNY", CreditGrantAmount: 500, CreditTargetPlanID: creditPlanID,
					PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe,
					Status: common.TopUpStatusPending, EntitlementSnapshot: snapshotJSON,
				}
				require.NoError(t, model.DB.Create(&order).Error)
				completion, err := model.CompleteSubscriptionOrder(order.TradeNo, "", model.PaymentProviderStripe, model.PaymentMethodStripe)
				require.NoError(t, err)
				require.NotNil(t, completion)
				require.NotNil(t, completion.CreditBalance)
				assert.Equal(t, model.SubscriptionPurchaseModeCreditBalance, completion.PurchaseMode)
				require.NoError(t, HandleInvitationRewardForCompletedSubscriptionOrder(order.Id))
			case "redemption":
				redemption := model.Redemption{
					Id: 20_007, Key: "credit-invitation-redemption", Type: model.RedemptionTypeSubscription,
					PlanId: optionPlanID, Status: common.RedemptionCodeStatusEnabled,
					AmountCents: 3000, Currency: "CNY", CreatedTime: now,
				}
				require.NoError(t, redemption.Insert())
				require.NotEmpty(t, strings.TrimSpace(redemption.FulfillmentSnapshot))
				result, err := model.Redeem(redemption.Key, creditInviteeID, model.RedemptionModeCreditBalance)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.NotNil(t, result.CreditBalance)
				assert.Equal(t, model.RedemptionModeCreditBalance, result.RedemptionMode)
				require.NoError(t, HandleInvitationRewardForSubscriptionRedemption(redemption.Id))
			default:
				t.Fatalf("unknown Credit fulfillment kind %q", test.kind)
			}

			var balances []model.UserSubscription
			require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", creditInviteeID, model.SubscriptionEntitlementCreditBalance).Find(&balances).Error)
			require.Len(t, balances, 1)
			var rewardEvents int64
			require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("invitee_id = ?", creditInviteeID).Count(&rewardEvents).Error)
			assert.Zero(t, rewardEvents)
			var commissions int64
			require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("invitee_id = ?", creditInviteeID).Count(&commissions).Error)
			assert.Zero(t, commissions)
			var commissionAccounts int64
			require.NoError(t, model.DB.Model(&model.InvitationCommissionAccount{}).Where("user_id = ?", inviterID).Count(&commissionAccounts).Error)
			assert.Zero(t, commissionAccounts)
			qualification, err := EnsureMonthlyInvitationEntitlement(inviterID, time.Unix(now, 0).UTC())
			require.NoError(t, err)
			assert.Equal(t, 2, qualification.DirectInviteCount)
			assert.Equal(t, 1, qualification.QualifiedActiveCount)
			assert.False(t, qualification.Entitled)
			assert.Zero(t, qualification.RewardSubscriptionId)
		})
	}
}
func TestCreateInvitationCommissionForRewardEventCreditsAvailableOnce(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	event := seedCommissionRewardEvent(t, 9401, 9402, 9403, 10000, "CNY")

	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9401).First(&account).Error)
	assert.Equal(t, int64(1000), account.AvailableCents)
	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
	assert.Equal(t, int64(1), records)
	var ledgers int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND type = ?", 9401, model.InvitationCommissionLedgerEarned).Count(&ledgers).Error)
	assert.Equal(t, int64(1), ledgers)
}

func TestHandleInvitationRewardForSubscriptionRedemptionCreditsCommission(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	require.NoError(t, model.DB.Create(&model.User{Id: 9407, Username: "redemption-inviter", Status: common.UserStatusEnabled, AffCode: "redemption-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9408, Username: "redemption-child", Status: common.UserStatusEnabled, AffCode: "redemption-child", InviterId: 9407}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9409, Title: "Redemption Commission", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9410, UserId: 9408, PlanId: 9409, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: "redemption", Source: "redemption"}).Error)
	redemption := model.Redemption{Id: 9411, Key: "commission-redemption-key", Status: common.RedemptionCodeStatusUsed, Type: model.RedemptionTypeSubscription, PlanId: 9409, AmountCents: 10000, Currency: "CNY", UsedUserId: 9408, CreatedTime: now, RedeemedTime: now}
	require.NoError(t, model.DB.Create(&redemption).Error)
	event := model.InvitationRewardEvent{InviterId: 9407, InviteeId: 9408, SourceType: model.InvitationRewardEventSourceSubscriptionRedemption, SourceId: redemption.Id, SourceRedemptionId: redemption.Id, SourceSubscriptionId: 9410, SourceAmountCents: 10000, SourceCurrency: "CNY", EventStartTime: now - 3600, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.DB.Create(&event).Error)

	require.NoError(t, HandleInvitationRewardForSubscriptionRedemption(redemption.Id))

	var record model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
	assert.Equal(t, model.InvitationRewardEventSourceSubscriptionRedemption, record.SourceType)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
	assert.Equal(t, int64(1000), record.CommissionCents)
}

func TestCreateInvitationCommissionForRewardEventUsesCurrentInviterModeFreshly(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	event := seedCommissionRewardEvent(t, 9404, 9405, 9406, 10000, "CNY")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9404).Update("invitation_reward_mode", model.InvitationRewardModeSubscription).Error)

	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
	assert.Equal(t, int64(0), records)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9404).Update("invitation_reward_mode", model.InvitationRewardModeCommission).Error)
	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ? AND status = ?", event.Id, model.InvitationCommissionStatusAvailable).Count(&records).Error)
	assert.Equal(t, int64(1), records)
}

func seedSecondQualifiedInviteeForEntitlement(t *testing.T, inviterId int, inviteeId int, planId int) {
	t.Helper()
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.User{Id: inviteeId, Username: fmt.Sprintf("second-invitee-%d", inviteeId), Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("second-aff-%d", inviteeId), InviterId: inviterId}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: inviteeId + 200000, UserId: inviteeId, PlanId: planId, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
}

func TestInvitationRewardHandlersPreserveSubscriptionRewardPackagePath(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	event := seedCommissionRewardEvent(t, 9455, 9456, 9457, 10000, "CNY")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9455).Update("invitation_reward_mode", model.InvitationRewardModeSubscription).Error)
	var sourceSub model.UserSubscription
	require.NoError(t, model.DB.First(&sourceSub, event.SourceSubscriptionId).Error)
	seedSecondQualifiedInviteeForEntitlement(t, 9455, 9466, sourceSub.PlanId)

	require.NoError(t, HandleInvitationRewardForCompletedSubscriptionOrder(event.SourceOrderId))

	var commissionRecords int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&commissionRecords).Error)
	assert.Equal(t, int64(0), commissionRecords)
	var entitlementCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ? AND status = ?", 9455, model.InvitationEntitlementStatusQualified).Count(&entitlementCount).Error)
	assert.Equal(t, int64(1), entitlementCount)
}

func TestInvitationRewardHandlersSkipSubscriptionPackageForCommissionMode(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	event := seedCommissionRewardEvent(t, 9458, 9459, 9460, 10000, "CNY")
	var sourceSub model.UserSubscription
	require.NoError(t, model.DB.First(&sourceSub, event.SourceSubscriptionId).Error)
	seedSecondQualifiedInviteeForEntitlement(t, 9458, 9467, sourceSub.PlanId)

	require.NoError(t, HandleInvitationRewardForCompletedSubscriptionOrder(event.SourceOrderId))

	var record model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
	var entitlementCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 9458).Count(&entitlementCount).Error)
	assert.Equal(t, int64(0), entitlementCount)
}

func TestInvitationRewardDispatchCreditsDisabledCommissionInviter(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	event := seedCommissionRewardEvent(t, 9468, 9469, 9470, 10000, "CNY")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9468).Update("status", common.UserStatusDisabled).Error)

	require.NoError(t, HandleInvitationRewardForCompletedSubscriptionOrder(event.SourceOrderId))

	var record model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
	assert.Equal(t, int64(1000), record.CommissionCents)
	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9468).First(&account).Error)
	assert.Equal(t, int64(1000), account.AvailableCents)
}

func TestCreateInvitationCommissionForRewardEventDoesNotConsumeWhenCommissionDisabled(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: false, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	event := seedCommissionRewardEvent(t, 9441, 9442, 9443, 10000, "CNY")

	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
	assert.Equal(t, int64(0), records)
	var accounts int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionAccount{}).Where("user_id = ?", 9441).Count(&accounts).Error)
	assert.Equal(t, int64(0), accounts)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ? AND status = ?", event.Id, model.InvitationCommissionStatusAvailable).Count(&records).Error)
	assert.Equal(t, int64(1), records)
}

func TestCreateInvitationCommissionForRewardEventDoesNotConsumeRewardIneligibleSource(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	event := seedCommissionRewardEvent(t, 9444, 9445, 9446, 10000, "CNY")
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, event.SourceSubscriptionId).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", sub.PlanId).Update("reward_eligible", false).Error)

	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
	assert.Equal(t, int64(0), records)
	var accounts int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionAccount{}).Where("user_id = ?", 9444).Count(&accounts).Error)
	assert.Equal(t, int64(0), accounts)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", sub.PlanId).Update("reward_eligible", true).Error)
	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ? AND status = ?", event.Id, model.InvitationCommissionStatusAvailable).Count(&records).Error)
	assert.Equal(t, int64(1), records)
}

func TestCreateInvitationCommissionForRewardEventSkipsInvalidSourceAmountAndRate(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	cases := []struct {
		name        string
		eventAmount int64
		currency    string
		rateBps     int
		wantReason  string
	}{
		{name: "non cny", eventAmount: 10000, currency: "USD", rateBps: 1000, wantReason: model.InvitationCommissionReasonUnsupportedCurrency},
		{name: "zero amount", eventAmount: 0, currency: "CNY", rateBps: 1000, wantReason: model.InvitationCommissionReasonInvalidSourceAmount},
		{name: "overflow", eventAmount: math.MaxInt64, currency: "CNY", rateBps: 10000, wantReason: model.InvitationCommissionReasonCommissionOverflow},
	}
	for idx, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: tc.rateBps, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
			event := seedCommissionRewardEvent(t, 9411+idx, 9421+idx, 9431+idx, tc.eventAmount, tc.currency)
			require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
			var record model.InvitationCommissionRecord
			require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
			assert.Equal(t, model.InvitationCommissionStatusSkipped, record.Status)
			assert.Equal(t, tc.wantReason, record.Reason)
		})
	}
}

func TestCreateInvitationCommissionForRewardEventDoesNotConsumeWhenRateDisabled(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 0, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	event := seedCommissionRewardEvent(t, 9447, 9448, 9449, 10000, "CNY")

	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))

	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
	assert.Equal(t, int64(0), records)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	require.NoError(t, CreateInvitationCommissionForRewardEvent(event.Id))
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ? AND status = ?", event.Id, model.InvitationCommissionStatusAvailable).Count(&records).Error)
	assert.Equal(t, int64(1), records)
}

func TestTransferInvitationCommissionToBalanceCompletesImmediately(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	seedCommissionAccount(t, 9451, 5000, 0, 0, 0)
	require.NoError(t, model.DB.Create(&model.User{Id: 9451, Username: "transfer-user", Status: common.UserStatusEnabled, AffCode: "transfer-user", InvitationRewardMode: model.InvitationRewardModeCommission, Quota: 100}).Error)
	var topUpsBefore int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9451).Count(&topUpsBefore).Error)

	result, err := TransferInvitationCommissionToBalance(9451, 1200)

	require.NoError(t, err)
	assert.Equal(t, int64(3800), result.AvailableCents)
	assert.Equal(t, int64(1200), result.TransferredCents)
	assert.Equal(t, 1300, result.UserQuota)
	var user model.User
	require.NoError(t, model.DB.First(&user, 9451).Error)
	assert.Equal(t, 1300, user.Quota)
	var topUpsAfter int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9451).Count(&topUpsAfter).Error)
	assert.Equal(t, topUpsBefore, topUpsAfter)
	var withdrawals int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionWithdrawal{}).Where("user_id = ?", 9451).Count(&withdrawals).Error)
	assert.Equal(t, int64(0), withdrawals)
}

func TestInvitationCommissionWithdrawalLifecycle(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	seedCommissionAccount(t, 9461, 6000, 0, 0, 0)
	require.NoError(t, model.DB.Create(&model.User{Id: 9461, Username: "withdraw-user", Status: common.UserStatusEnabled, AffCode: "withdraw-user", InvitationRewardMode: model.InvitationRewardModeCommission, Quota: 777}).Error)

	withdrawal, err := RequestInvitationCommissionWithdrawal(9461, InvitationCommissionWithdrawalRequest{
		AmountCents: 5000,
		Contact:     InvitationCommissionContact{Type: "wechat", Value: "user-contact"},
		Remark:      "请联系我",
	})
	require.NoError(t, err)
	assert.Equal(t, model.InvitationCommissionWithdrawalPending, withdrawal.Status)

	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9461).First(&account).Error)
	assert.Equal(t, int64(1000), account.AvailableCents)
	assert.Equal(t, int64(5000), account.PendingCents)

	var quotaBefore int
	require.NoError(t, model.DB.Model(&model.User{}).Select("quota").Where("id = ?", 9461).Scan(&quotaBefore).Error)
	var topUpsBefore int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9461).Count(&topUpsBefore).Error)

	require.NoError(t, CompleteInvitationCommissionWithdrawal(withdrawal.Id, 1001, "已线下转账"))
	require.Error(t, CompleteInvitationCommissionWithdrawal(withdrawal.Id, 1001, "重复完成"))
	require.NoError(t, model.DB.Where("user_id = ?", 9461).First(&account).Error)
	assert.Equal(t, int64(0), account.PendingCents)
	assert.Equal(t, int64(5000), account.WithdrawnCents)

	var completed model.InvitationCommissionWithdrawal
	require.NoError(t, model.DB.First(&completed, withdrawal.Id).Error)
	assert.Equal(t, 1001, completed.ReviewerId)
	assert.Equal(t, 1001, completed.CompletedBy)
	assert.NotZero(t, completed.ReviewedAt)
	assert.NotZero(t, completed.CompletedAt)
	assert.Equal(t, "已线下转账", completed.AdminRemark)
	var quotaAfter int
	require.NoError(t, model.DB.Model(&model.User{}).Select("quota").Where("id = ?", 9461).Scan(&quotaAfter).Error)
	assert.Equal(t, quotaBefore, quotaAfter)
	var topUpsAfter int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9461).Count(&topUpsAfter).Error)
	assert.Equal(t, topUpsBefore, topUpsAfter)
	var completedLedgers int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND type = ? AND reference_id = ?", 9461, model.InvitationCommissionLedgerWithdrawalCompleted, withdrawal.Id).Count(&completedLedgers).Error)
	assert.Equal(t, int64(1), completedLedgers)
}

func TestRejectInvitationCommissionWithdrawalReturnsPendingToAvailable(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	seedCommissionAccount(t, 9471, 1000, 5000, 0, 0)
	require.NoError(t, model.DB.Create(&model.User{Id: 9471, Username: "reject-user", Status: common.UserStatusEnabled, AffCode: "reject-user", InvitationRewardMode: model.InvitationRewardModeCommission, Quota: 888}).Error)
	contactSnapshot, err := common.Marshal(map[string]string{"type": "telegram", "value": "reject-user"})
	require.NoError(t, err)
	now := common.GetTimestamp()
	withdrawal := model.InvitationCommissionWithdrawal{UserId: 9471, AmountCents: 5000, Status: model.InvitationCommissionWithdrawalPending, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.DB.Create(&withdrawal).Error)
	var quotaBefore int
	require.NoError(t, model.DB.Model(&model.User{}).Select("quota").Where("id = ?", 9471).Scan(&quotaBefore).Error)
	var topUpsBefore int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9471).Count(&topUpsBefore).Error)

	require.NoError(t, RejectInvitationCommissionWithdrawal(withdrawal.Id, 1002, "资料不完整"))

	var quotaAfter int
	require.NoError(t, model.DB.Model(&model.User{}).Select("quota").Where("id = ?", 9471).Scan(&quotaAfter).Error)
	assert.Equal(t, quotaBefore, quotaAfter)
	var topUpsAfter int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", 9471).Count(&topUpsAfter).Error)
	assert.Equal(t, topUpsBefore, topUpsAfter)
	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9471).First(&account).Error)
	assert.Equal(t, int64(6000), account.AvailableCents)
	assert.Equal(t, int64(0), account.PendingCents)
	var rejected model.InvitationCommissionWithdrawal
	require.NoError(t, model.DB.First(&rejected, withdrawal.Id).Error)
	assert.Equal(t, model.InvitationCommissionWithdrawalRejected, rejected.Status)
	assert.Equal(t, 1002, rejected.ReviewerId)
	assert.NotZero(t, rejected.ReviewedAt)
	assert.Equal(t, "资料不完整", rejected.AdminRemark)
	var ledger model.InvitationCommissionLedger
	require.NoError(t, model.DB.Where("user_id = ? AND type = ? AND reference_id = ?", 9471, model.InvitationCommissionLedgerWithdrawalRejected, withdrawal.Id).First(&ledger).Error)
	assert.Equal(t, int64(6000), ledger.AvailableAfterCents)
	assert.Equal(t, int64(0), ledger.PendingAfterCents)
}

func TestCommissionDisabledStillAllowsHistoricalBalanceOperations(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: false, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	seedCommissionAccount(t, 9472, 5000, 0, 0, 0)
	require.NoError(t, model.DB.Create(&model.User{Id: 9472, Username: "disabled-history", Status: common.UserStatusEnabled, AffCode: "disabled-history", InvitationRewardMode: model.InvitationRewardModeSubscription, Quota: 0}).Error)

	disabledSummary, err := GetInvitationCommissionSummary(9472)
	require.NoError(t, err)
	assert.True(t, disabledSummary.CanTransfer)
	assert.True(t, disabledSummary.CanRequestWithdrawal)

	transfer, err := TransferInvitationCommissionToBalance(9472, 1000)
	require.NoError(t, err)
	assert.Equal(t, int64(4000), transfer.AvailableCents)
	assert.Equal(t, int64(1000), transfer.TransferredCents)

	withdrawal, err := RequestInvitationCommissionWithdrawal(9472, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "email", Value: "disabled@example.com"}, Remark: "disabled but historical"})
	require.NoError(t, err)
	assert.Equal(t, model.InvitationCommissionWithdrawalPending, withdrawal.Status)
	require.NoError(t, CompleteInvitationCommissionWithdrawal(withdrawal.Id, 1001, "已线下返现"))

	rejected, err := RequestInvitationCommissionWithdrawal(9472, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "wechat", Value: "disabled-reject"}})
	require.NoError(t, err)
	require.NoError(t, RejectInvitationCommissionWithdrawal(rejected.Id, 1002, "资料不完整"))

	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9472).First(&account).Error)
	assert.Equal(t, int64(3000), account.AvailableCents)
	assert.Equal(t, int64(0), account.PendingCents)
	assert.Equal(t, int64(1000), account.WithdrawnCents)
	assert.Equal(t, int64(1000), account.TransferredCents)

	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 0, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	rateZeroSummary, err := GetInvitationCommissionSummary(9472)
	require.NoError(t, err)
	assert.True(t, rateZeroSummary.CanTransfer)
	assert.True(t, rateZeroSummary.CanRequestWithdrawal)
	_, err = TransferInvitationCommissionToBalance(9472, 1000)
	require.NoError(t, err)
	zeroRateWithdrawal, err := RequestInvitationCommissionWithdrawal(9472, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "email", Value: "zero-rate@example.com"}, Remark: "rate zero but historical"})
	require.NoError(t, err)
	require.NoError(t, CompleteInvitationCommissionWithdrawal(zeroRateWithdrawal.Id, 1004, "费率关闭但历史余额可正常完成"))
	zeroRateRejected, err := RequestInvitationCommissionWithdrawal(9472, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "email", Value: "zero-rate-reject@example.com"}, Remark: "rate zero but historical reject"})
	require.NoError(t, err)
	require.NoError(t, RejectInvitationCommissionWithdrawal(zeroRateRejected.Id, 1003, "费率关闭但历史余额可退回"))
	require.NoError(t, model.DB.Where("user_id = ?", 9472).First(&account).Error)
	assert.Equal(t, int64(1000), account.AvailableCents)
	assert.Equal(t, int64(0), account.PendingCents)
	assert.Equal(t, int64(2000), account.WithdrawnCents)
	assert.Equal(t, int64(2000), account.TransferredCents)
}

func TestSubscriptionModeUserWithHistoricalCommissionAccountCanRequestWithdrawal(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	seedCommissionAccount(t, 9452, 5000, 0, 0, 0)
	require.NoError(t, model.DB.Create(&model.User{Id: 9452, Username: "history-withdraw-user", Status: common.UserStatusEnabled, AffCode: "history-withdraw-user", InvitationRewardMode: model.InvitationRewardModeSubscription}).Error)

	withdrawal, err := RequestInvitationCommissionWithdrawal(9452, InvitationCommissionWithdrawalRequest{AmountCents: 2000, Contact: InvitationCommissionContact{Type: "wechat", Value: "history-contact"}, Remark: "历史返佣返现"})

	require.NoError(t, err)
	assert.Equal(t, model.InvitationCommissionWithdrawalPending, withdrawal.Status)
	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9452).First(&account).Error)
	assert.Equal(t, int64(3000), account.AvailableCents)
	assert.Equal(t, int64(2000), account.PendingCents)
}

func TestInvitationCommissionConcurrentOperationsRemainIdempotentAndNonNegative(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	event := seedCommissionRewardEvent(t, 9481, 9482, 9483, 10000, "CNY")

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- CreateInvitationCommissionForRewardEvent(event.Id)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9481).First(&account).Error)
	assert.Equal(t, int64(1000), account.AvailableCents)
	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", event.Id).Count(&records).Error)
	assert.Equal(t, int64(1), records)

	seedCommissionAccount(t, 9484, 1500, 0, 0, 0)
	require.NoError(t, model.DB.Create(&model.User{Id: 9484, Username: "race-user", Status: common.UserStatusEnabled, AffCode: "race-user", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	raceErrs := make(chan error, 2)
	var raceWg sync.WaitGroup
	raceWg.Add(2)
	go func() {
		defer raceWg.Done()
		_, err := TransferInvitationCommissionToBalance(9484, 1000)
		raceErrs <- err
	}()
	go func() {
		defer raceWg.Done()
		_, err := RequestInvitationCommissionWithdrawal(9484, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "email", Value: "race@example.com"}})
		raceErrs <- err
	}()
	raceWg.Wait()
	close(raceErrs)
	successes := 0
	for err := range raceErrs {
		if err == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes)
	var raceAccount model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9484).First(&raceAccount).Error)
	assert.GreaterOrEqual(t, raceAccount.AvailableCents, int64(0))
	assert.Equal(t, int64(1500), raceAccount.AvailableCents+raceAccount.PendingCents+raceAccount.TransferredCents)
}

func TestRetryPendingInvitationRewardEventsProcessesExistingEvents(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	commissionEvent := seedCommissionRewardEvent(t, 9485, 9486, 9487, 10000, "CNY")
	ineligibleEvent := seedCommissionRewardEvent(t, 9488, 9489, 9490, 10000, "CNY")
	var ineligibleSub model.UserSubscription
	require.NoError(t, model.DB.First(&ineligibleSub, ineligibleEvent.SourceSubscriptionId).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", ineligibleSub.PlanId).Update("reward_eligible", false).Error)
	now := common.GetTimestamp()
	code := "retry_subscription_plan"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9491, Title: "Retry Subscription", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9492, Username: "retry-subscription-inviter", Status: common.UserStatusEnabled, AffCode: "retry-subscription-inviter", InvitationRewardMode: model.InvitationRewardModeSubscription}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9493, Username: "retry-subscription-child", Status: common.UserStatusEnabled, AffCode: "retry-subscription-child", InviterId: 9492}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9494, UserId: 9493, PlanId: 9491, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	subscriptionEvent := model.InvitationRewardEvent{SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: 9495, InviterId: 9492, InviteeId: 9493, SourceSubscriptionId: 9494, EventStartTime: now - 3600, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, SourceAmountCents: 10000, SourceCurrency: "CNY", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.DB.Create(&subscriptionEvent).Error)

	processed, err := RetryPendingInvitationRewardEvents(10)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	var record model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("event_id = ?", commissionEvent.Id).First(&record).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9485).First(&account).Error)
	assert.Equal(t, int64(1000), account.AvailableCents)
	var ineligibleRecords int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", ineligibleEvent.Id).Count(&ineligibleRecords).Error)
	assert.Equal(t, int64(0), ineligibleRecords)
	var subscriptionRecords int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id = ?", subscriptionEvent.Id).Count(&subscriptionRecords).Error)
	assert.Equal(t, int64(0), subscriptionRecords)
	var entitlementCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", 9492).Count(&entitlementCount).Error)
	assert.Equal(t, int64(0), entitlementCount)

	processedAgain, err := RetryPendingInvitationRewardEvents(10)
	require.NoError(t, err)
	assert.Equal(t, 0, processedAgain)
}

func TestRetryPendingInvitationRewardEventsCreditsBackfilledLegacySubscriptionAfterModeSwitch(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.User{Id: 9701, Username: "legacy-switch-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-switch-inviter", InvitationRewardMode: model.InvitationRewardModeSubscription}).Error)
	for _, userID := range []int{9702, 9703, 9704} {
		require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: fmt.Sprintf("legacy-switch-child-%d", userID), Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("legacy-switch-child-%d", userID), InviterId: 9701}).Error)
	}
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9705, Title: "Legacy Switch Paid", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9706, Title: "Legacy Switch Ineligible", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: false, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9706).Update("reward_eligible", false).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9707, UserId: 9702, PlanId: 9705, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{Id: 9708, UserId: 9702, PlanId: 9705, Status: common.TopUpStatusSuccess, Money: 100, AmountCents: 10000, Currency: "CNY", TradeNo: "legacy-switch-paid", PaymentProvider: model.PaymentProviderEpay, CreateTime: now - 3500}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9709, UserId: 9703, PlanId: 9706, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{Id: 9710, UserId: 9703, PlanId: 9706, Status: common.TopUpStatusSuccess, Money: 100, AmountCents: 10000, Currency: "CNY", TradeNo: "legacy-switch-ineligible", PaymentProvider: model.PaymentProviderEpay, CreateTime: now - 3500}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9711, UserId: 9704, PlanId: 9705, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)

	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error { return model.BackfillLegacyInvitationRewardEventsTx(tx, now) }))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9701).Update("invitation_reward_mode", model.InvitationRewardModeCommission).Error)

	processed, err := RetryPendingInvitationRewardEvents(10)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	var record model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceLegacySubscription, 9707).First(&record).Error)
	assert.Equal(t, model.InvitationRewardEventSourceLegacySubscription, record.SourceType)
	assert.Equal(t, int64(10000), record.SourceAmountCents)
	assert.Equal(t, int64(1000), record.CommissionCents)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
	var ledger model.InvitationCommissionLedger
	require.NoError(t, model.DB.Where("user_id = ? AND type = ? AND reference_id = ?", 9701, model.InvitationCommissionLedgerEarned, record.Id).First(&ledger).Error)
	assert.Equal(t, int64(1000), ledger.AvailableAfterCents)
	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9701).First(&account).Error)
	assert.Equal(t, int64(1000), account.AvailableCents)
	var ineligibleRecords int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceLegacySubscription, 9709).Count(&ineligibleRecords).Error)
	assert.Equal(t, int64(0), ineligibleRecords)
	var invalidRecord model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceLegacySubscription, 9711).First(&invalidRecord).Error)
	assert.Equal(t, model.InvitationCommissionStatusSkipped, invalidRecord.Status)
	assert.Equal(t, model.InvitationCommissionReasonInvalidSourceAmount, invalidRecord.Reason)
	assert.Equal(t, int64(0), invalidRecord.SourceAmountCents)
	assert.Equal(t, "", invalidRecord.SourceCurrency)
}

func TestRetryPendingInvitationRewardEventsSkipsUnprocessableHeadOfLine(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	setInvitationCommissionSettingForTest(t, operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000})
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.User{Id: 9801, Username: "retry-head-subscription", Status: common.UserStatusEnabled, AffCode: "retry-head-subscription", InvitationRewardMode: model.InvitationRewardModeSubscription}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9802, Username: "retry-head-subscription-child", Status: common.UserStatusEnabled, AffCode: "retry-head-subscription-child", InviterId: 9801}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9803, Title: "Retry Head Subscription", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9804, UserId: 9802, PlanId: 9803, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	require.NoError(t, model.DB.Create(&model.InvitationRewardEvent{Id: 9805, InviterId: 9801, InviteeId: 9802, SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: 9805, SourceOrderId: 9805, SourceSubscriptionId: 9804, SourceAmountCents: 10000, SourceCurrency: "CNY", EventStartTime: now - 3600, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now}).Error)

	require.NoError(t, model.DB.Create(&model.User{Id: 9806, Username: "retry-head-ineligible", Status: common.UserStatusEnabled, AffCode: "retry-head-ineligible", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9807, Username: "retry-head-ineligible-child", Status: common.UserStatusEnabled, AffCode: "retry-head-ineligible-child", InviterId: 9806}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9808, Title: "Retry Head Ineligible", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9808).Update("reward_eligible", false).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9809, UserId: 9807, PlanId: 9808, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	require.NoError(t, model.DB.Create(&model.InvitationRewardEvent{Id: 9810, InviterId: 9806, InviteeId: 9807, SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: 9810, SourceOrderId: 9810, SourceSubscriptionId: 9809, SourceAmountCents: 10000, SourceCurrency: "CNY", EventStartTime: now - 3600, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now}).Error)

	event := seedCommissionRewardEvent(t, 9811, 9812, 9813, 10000, "CNY")

	processed, err := RetryPendingInvitationRewardEvents(1)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	var record model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
	var blockedRecords int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("event_id IN ?", []int{9805, 9810}).Count(&blockedRecords).Error)
	assert.Equal(t, int64(0), blockedRecords)
}

func TestInvitationCommissionListQueriesParseContactAndIsolateUsers(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	now := common.GetTimestamp()
	for _, user := range []model.User{
		{Id: 9821, Username: "list-user", DisplayName: "List User", Status: common.UserStatusEnabled, AffCode: "list-user", InvitationRewardMode: model.InvitationRewardModeCommission},
		{Id: 9822, Username: "list-other", DisplayName: "List Other", Status: common.UserStatusEnabled, AffCode: "list-other", InvitationRewardMode: model.InvitationRewardModeCommission},
	} {
		require.NoError(t, model.DB.Create(&user).Error)
	}
	require.NoError(t, model.DB.Create(&model.InvitationCommissionRecord{Id: 9823, EventId: 9824, InviterId: 9821, InviteeId: 9825, SourceType: model.InvitationCommissionSourceSubscriptionOrder, SourceId: 9826, SourceTradeNo: "list-trade", SourceAmountCents: 4000, SourceCurrency: "CNY", CommissionRateBps: 1000, CommissionCents: 400, Status: model.InvitationCommissionStatusAvailable, CreatedAt: now, AvailableAt: now}).Error)
	require.NoError(t, model.DB.Create(&model.InvitationCommissionRecord{Id: 9827, EventId: 9828, InviterId: 9822, InviteeId: 9829, SourceType: model.InvitationCommissionSourceSubscriptionOrder, SourceId: 9830, SourceTradeNo: "other-trade", SourceAmountCents: 4000, SourceCurrency: "CNY", CommissionRateBps: 1000, CommissionCents: 400, Status: model.InvitationCommissionStatusAvailable, CreatedAt: now, AvailableAt: now}).Error)
	contactSnapshot, err := common.Marshal(InvitationCommissionContact{Type: "wechat", Value: "list-contact"})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.InvitationCommissionWithdrawal{Id: 9831, UserId: 9821, AmountCents: 1000, Status: model.InvitationCommissionWithdrawalPending, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), UserRemark: "用户备注", AdminRemark: "", CreatedAt: now, UpdatedAt: now}).Error)
	require.NoError(t, model.DB.Create(&model.InvitationCommissionWithdrawal{Id: 9832, UserId: 9822, AmountCents: 1000, Status: model.InvitationCommissionWithdrawalCompleted, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), CreatedAt: now, UpdatedAt: now}).Error)

	records, err := ListInvitationCommissionRecords(9821, 1, 20)
	require.NoError(t, err)
	require.Len(t, records.Items, 1)
	assert.Equal(t, int64(1), records.Total)
	assert.Equal(t, 1, records.Page)
	assert.Equal(t, 20, records.PageSize)
	assert.Equal(t, "list-trade", records.Items[0].SourceTradeNo)
	assert.Equal(t, now, records.Items[0].AvailableAt)

	withdrawals, err := ListInvitationCommissionWithdrawals(9821, 1, 20)
	require.NoError(t, err)
	require.Len(t, withdrawals.Items, 1)
	assert.Equal(t, int64(1), withdrawals.Total)
	assert.Equal(t, InvitationCommissionContact{Type: "wechat", Value: "list-contact"}, withdrawals.Items[0].Contact)
	assert.Equal(t, "用户备注", withdrawals.Items[0].UserRemark)

	adminWithdrawals, err := AdminListInvitationCommissionWithdrawals(InvitationCommissionWithdrawalFilter{Status: model.InvitationCommissionWithdrawalPending})
	require.NoError(t, err)
	require.Len(t, adminWithdrawals.Items, 1)
	assert.Equal(t, int64(1), adminWithdrawals.Total)
	assert.Equal(t, "list-user", adminWithdrawals.Items[0].Username)
	assert.Equal(t, "List User", adminWithdrawals.Items[0].DisplayName)
	assert.Equal(t, InvitationCommissionContact{Type: "wechat", Value: "list-contact"}, adminWithdrawals.Items[0].Contact)

	pending, err := CountPendingInvitationCommissionWithdrawals()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pending)
}

func TestInvitationCommissionWithdrawalTerminalStateRejectsDuplicateTransitions(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	seedCommissionAccount(t, 9841, 0, 2000, 0, 0)
	require.NoError(t, model.DB.Create(&model.User{Id: 9841, Username: "terminal-user", Status: common.UserStatusEnabled, AffCode: "terminal-user", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	contactSnapshot, err := common.Marshal(InvitationCommissionContact{Type: "email", Value: "terminal@example.com"})
	require.NoError(t, err)
	now := common.GetTimestamp()
	withdrawal := model.InvitationCommissionWithdrawal{UserId: 9841, AmountCents: 2000, Status: model.InvitationCommissionWithdrawalPending, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.DB.Create(&withdrawal).Error)

	require.NoError(t, RejectInvitationCommissionWithdrawal(withdrawal.Id, 1001, "拒绝"))
	require.Error(t, RejectInvitationCommissionWithdrawal(withdrawal.Id, 1001, "重复拒绝"))
	require.Error(t, CompleteInvitationCommissionWithdrawal(withdrawal.Id, 1001, "拒绝后完成"))

	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9841).First(&account).Error)
	assert.Equal(t, int64(2000), account.AvailableCents)
	assert.Equal(t, int64(0), account.PendingCents)
	assert.Equal(t, int64(0), account.WithdrawnCents)
	var rejectedLedgers int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND type = ? AND reference_id = ?", 9841, model.InvitationCommissionLedgerWithdrawalRejected, withdrawal.Id).Count(&rejectedLedgers).Error)
	assert.Equal(t, int64(1), rejectedLedgers)
	var completedLedgers int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND type = ? AND reference_id = ?", 9841, model.InvitationCommissionLedgerWithdrawalCompleted, withdrawal.Id).Count(&completedLedgers).Error)
	assert.Equal(t, int64(0), completedLedgers)
	var terminalLedgers int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND reference_id = ?", 9841, withdrawal.Id).Count(&terminalLedgers).Error)
	assert.Equal(t, int64(1), terminalLedgers)

	seedCommissionAccount(t, 9842, 0, 2000, 0, 0)
	require.NoError(t, model.DB.Create(&model.User{Id: 9842, Username: "terminal-completed-user", Status: common.UserStatusEnabled, AffCode: "terminal-completed-user", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	completedWithdrawal := model.InvitationCommissionWithdrawal{UserId: 9842, AmountCents: 2000, Status: model.InvitationCommissionWithdrawalPending, Method: model.InvitationCommissionWithdrawalMethodManual, ContactSnapshot: string(contactSnapshot), CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.DB.Create(&completedWithdrawal).Error)

	require.NoError(t, CompleteInvitationCommissionWithdrawal(completedWithdrawal.Id, 1002, "完成"))
	require.Error(t, RejectInvitationCommissionWithdrawal(completedWithdrawal.Id, 1002, "完成后拒绝"))
	account = model.InvitationCommissionAccount{}
	require.NoError(t, model.DB.Where("user_id = ?", 9842).First(&account).Error)
	assert.Equal(t, int64(0), account.AvailableCents)
	assert.Equal(t, int64(0), account.PendingCents)
	assert.Equal(t, int64(2000), account.WithdrawnCents)
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND type = ? AND reference_id = ?", 9842, model.InvitationCommissionLedgerWithdrawalCompleted, completedWithdrawal.Id).Count(&completedLedgers).Error)
	assert.Equal(t, int64(1), completedLedgers)
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND type = ? AND reference_id = ?", 9842, model.InvitationCommissionLedgerWithdrawalRejected, completedWithdrawal.Id).Count(&rejectedLedgers).Error)
	assert.Equal(t, int64(0), rejectedLedgers)
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ? AND reference_id = ?", 9842, completedWithdrawal.Id).Count(&terminalLedgers).Error)
	assert.Equal(t, int64(1), terminalLedgers)
}

func TestInvitationCommissionBalanceOperationsFailWithoutSideEffectsWhenInsufficient(t *testing.T) {
	setupInvitationCommissionServiceDB(t)
	seedCommissionAccount(t, 9851, 500, 0, 0, 0)
	require.NoError(t, model.DB.Create(&model.User{Id: 9851, Username: "insufficient-user", Status: common.UserStatusEnabled, AffCode: "insufficient-user", InvitationRewardMode: model.InvitationRewardModeCommission, Quota: 700}).Error)

	_, err := TransferInvitationCommissionToBalance(9851, 1000)
	require.Error(t, err)
	_, err = RequestInvitationCommissionWithdrawal(9851, InvitationCommissionWithdrawalRequest{AmountCents: 1000, Contact: InvitationCommissionContact{Type: "email", Value: "insufficient@example.com"}})
	require.Error(t, err)

	var account model.InvitationCommissionAccount
	require.NoError(t, model.DB.Where("user_id = ?", 9851).First(&account).Error)
	assert.Equal(t, int64(500), account.AvailableCents)
	assert.Equal(t, int64(0), account.PendingCents)
	assert.Equal(t, int64(0), account.TransferredCents)
	assert.Equal(t, int64(0), account.WithdrawnCents)
	var user model.User
	require.NoError(t, model.DB.First(&user, 9851).Error)
	assert.Equal(t, 700, user.Quota)
	var withdrawals int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionWithdrawal{}).Where("user_id = ?", 9851).Count(&withdrawals).Error)
	assert.Equal(t, int64(0), withdrawals)
	var ledgers int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionLedger{}).Where("user_id = ?", 9851).Count(&ledgers).Error)
	assert.Equal(t, int64(0), ledgers)
}
