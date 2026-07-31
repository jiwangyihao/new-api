package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertedSubscriptionRedirectsInFlightSettlementAndRejectsNewPreConsume(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(
		&User{},
		&SubscriptionPreConsumeRecord{},
		&SubscriptionConversion{},
	))
	ClearPrimaryBillableSubscriptionCacheForTest()

	const userID = 10_101
	const sourceID = 10_102
	const planID = 10_103
	user := User{Id: userID, Username: "converted-in-flight", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = sourceID
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategySingleActive
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	plan := seedConversionQuoteTimedPlan(t, planID, 100)
	plan.ModelLimits = "gpt-4o"
	require.NoError(t, DB.Save(plan).Error)

	now := GetDBTimestamp()
	basis := int64(100)
	source := UserSubscription{
		Id: sourceID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit: 100, TokenUsed: 0, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 48*60*60, EndTime: now + 60*60, Status: SubscriptionStatusActive,
		LastGrantedAt: now - 48*60*60, LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: SubscriptionGrantTimeSourceLive, LastGrantSource: SubscriptionGrantOrder,
	}
	require.NoError(t, DB.Create(&source).Error)

	pre, err := PreConsumeUserSubscription("converted-in-flight-request", userID, "gpt-4o", 0, 10)
	require.NoError(t, err)
	require.Equal(t, sourceID, pre.UserSubscriptionId)

	converted, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "converted-in-flight-key")
	require.NoError(t, err)
	require.False(t, converted.Replayed)
	targetID := converted.Conversion.TargetSubscriptionId
	require.Positive(t, targetID)

	// The source keeps its original request usage and identity. Final positive delta
	// is applied to the target Credit balance even when it creates settlement debt.
	require.NoError(t, PostConsumeUserSubscriptionTokenDelta(sourceID, 100))
	FlushSubscriptionTokenDeltaUpdates()
	var persistedSource UserSubscription
	require.NoError(t, DB.First(&persistedSource, sourceID).Error)
	assert.Equal(t, SubscriptionStatusConverted, persistedSource.Status)
	assert.Equal(t, int64(10), persistedSource.TokenUsed)
	var balance UserSubscription
	require.NoError(t, DB.First(&balance, targetID).Error)
	assert.Equal(t, int64(90), balance.TokenLimit)
	assert.Equal(t, int64(100), balance.TokenUsed)

	var originalRecord SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "converted-in-flight-request").First(&originalRecord).Error)
	assert.Equal(t, sourceID, originalRecord.UserSubscriptionId)

	_, err = PreConsumeUserSubscription("converted-debt-new-request", userID, "gpt-4o", 0, 1)
	require.Error(t, err)
	var deniedCount int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", "converted-debt-new-request").Count(&deniedCount).Error)
	assert.Zero(t, deniedCount)
	require.NoError(t, DB.First(&balance, targetID).Error)
	assert.Equal(t, int64(90), balance.TokenLimit)
	assert.Equal(t, int64(100), balance.TokenUsed)

	// A refund of the original pre-consume also follows the mapping and does not
	// mutate the historical source subscription.
	require.NoError(t, RefundSubscriptionPreConsume("converted-in-flight-request"))
	require.NoError(t, DB.First(&persistedSource, sourceID).Error)
	assert.Equal(t, int64(10), persistedSource.TokenUsed)
	require.NoError(t, DB.First(&balance, targetID).Error)
	assert.Equal(t, int64(90), balance.TokenUsed)
	require.NoError(t, DB.Where("request_id = ?", "converted-in-flight-request").First(&originalRecord).Error)
	assert.Equal(t, "refunded", originalRecord.Status)
	assert.Equal(t, sourceID, originalRecord.UserSubscriptionId)
}

func TestConvertedAmountSubscriptionRedirectsInFlightSettlement(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionConversion{}))

	const userID = 10_111
	const sourceID = 10_112
	const planID = 10_113
	require.NoError(t, DB.Create(&User{Id: userID, Username: "converted-amount-in-flight", Status: common.UserStatusEnabled}).Error)
	plan := SubscriptionPlan{
		Id: planID, Title: "Amount based convertible", EntitlementType: SubscriptionEntitlementTimed,
		Enabled: true, DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: SubscriptionResetMonthly, MonthlyTokenLimit: 100, TimedConversionEnabled: true,
	}
	require.NoError(t, DB.Create(&plan).Error)

	now := GetDBTimestamp()
	basis := int64(100)
	source := UserSubscription{
		Id: sourceID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed,
		AmountTotal: 100, AmountUsed: 10, TokenLimit: 0, TokenUsed: 0,
		GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 48*60*60, EndTime: now + TimedSubscriptionConversionBlockSeconds + 60,
		Status: SubscriptionStatusActive, LastGrantedAt: now - 48*60*60,
		LastGrantCreditSnapshot: &basis, LastGrantTimeSource: SubscriptionGrantTimeSourceLive,
		LastGrantSource: SubscriptionGrantOrder,
	}
	require.NoError(t, DB.Create(&source).Error)

	converted, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "converted-amount-in-flight-key")
	require.NoError(t, err)
	targetID := converted.Conversion.TargetSubscriptionId

	require.NoError(t, PostConsumeUserSubscriptionAmountDelta(sourceID, 25))
	var persistedSource UserSubscription
	require.NoError(t, DB.First(&persistedSource, sourceID).Error)
	assert.Equal(t, SubscriptionStatusConverted, persistedSource.Status)
	assert.Equal(t, int64(10), persistedSource.AmountUsed)
	var balance UserSubscription
	require.NoError(t, DB.First(&balance, targetID).Error)
	assert.Equal(t, int64(25), balance.TokenUsed)

	require.NoError(t, PostConsumeUserSubscriptionAmountDelta(sourceID, -15))
	require.NoError(t, DB.First(&persistedSource, sourceID).Error)
	assert.Equal(t, int64(10), persistedSource.AmountUsed)
	require.NoError(t, DB.First(&balance, targetID).Error)
	assert.Equal(t, int64(10), balance.TokenUsed)
}

func TestConvertedSubscriptionRefundBeyondTargetUsageRestoresAvailableCredit(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionConversion{}))

	const userID = 10_121
	const sourceID = 10_122
	const planID = 10_123
	require.NoError(t, DB.Create(&User{Id: userID, Username: "converted-first-refund", Status: common.UserStatusEnabled}).Error)
	plan := seedConversionQuoteTimedPlan(t, planID, 100)
	plan.ModelLimits = "gpt-4o"
	require.NoError(t, DB.Save(plan).Error)

	now := GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: sourceID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit: 100, TokenUsed: 10, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 48*60*60, EndTime: now + 60*60, Status: SubscriptionStatusActive,
		LastGrantedAt: now - 48*60*60, LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: SubscriptionGrantTimeSourceLive, LastGrantSource: SubscriptionGrantOrder,
	}).Error)

	converted, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "converted-first-refund-key")
	require.NoError(t, err)
	targetID := converted.Conversion.TargetSubscriptionId
	var balance UserSubscription
	require.NoError(t, DB.First(&balance, targetID).Error)
	require.Equal(t, int64(90), balance.TokenLimit)
	require.Zero(t, balance.TokenUsed)

	require.NoError(t, PostConsumeUserSubscriptionTokenDelta(sourceID, -10))
	require.NoError(t, DB.First(&balance, targetID).Error)
	assert.Equal(t, int64(100), balance.TokenLimit)
	assert.Zero(t, balance.TokenUsed)
	var source UserSubscription
	require.NoError(t, DB.First(&source, sourceID).Error)
	assert.Equal(t, int64(10), source.TokenUsed)
}

func TestCompletedTimedOrderConvertsThenRefundsThroughPersistedMapping(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}))

	const userID = 10_131
	const sourceID = 10_132
	const planID = 10_133
	require.NoError(t, DB.Create(&User{Id: userID, Username: "converted-order-refund", Status: common.UserStatusEnabled}).Error)
	plan := seedConversionQuoteTimedPlan(t, planID, 100)
	now := GetDBTimestamp()
	basis := int64(100)
	source := UserSubscription{
		Id: sourceID, UserId: userID, PlanId: planID, EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit: 100, TokenUsed: 10, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 48*60*60, EndTime: now + 60*60, Status: SubscriptionStatusActive,
		LastGrantedAt: now - 48*60*60, LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource: SubscriptionGrantTimeSourceLive, LastGrantSource: SubscriptionGrantOrder,
	}
	require.NoError(t, DB.Create(&source).Error)
	snapshot := NewSubscriptionEntitlementSnapshot(plan, SubscriptionPurchaseModeTimed, 0)
	snapshot.SetPaymentSnapshot(PaymentProviderStripe, "price_converted_refund", PaymentMethodStripe, 2000, "CNY")
	snapshotJSON, err := MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := SubscriptionOrder{
		UserId: userID, PlanId: planID, TradeNo: "converted-order-real-flow",
		AmountCents: 2000, Currency: "CNY", PaymentProvider: PaymentProviderStripe,
		PaymentMethod: PaymentMethodStripe, Status: common.TopUpStatusSuccess,
		CompleteTime: now, FulfilledSubscriptionID: sourceID, EntitlementSnapshot: snapshotJSON,
	}
	require.NoError(t, DB.Create(&order).Error)

	converted, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "converted-order-real-flow")
	require.NoError(t, err)
	require.False(t, converted.Replayed)
	require.Equal(t, sourceID, converted.Conversion.SourceSubscriptionId)
	var balance UserSubscription
	require.NoError(t, DB.First(&balance, converted.Conversion.TargetSubscriptionId).Error)
	require.Equal(t, int64(90), balance.TokenLimit)
	require.Zero(t, balance.TokenUsed)

	recovered, err := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
		TradeNo: order.TradeNo, ExpectedPaymentProvider: PaymentProviderStripe,
		RecoveryType: SubscriptionOrderRecoveryRefund, Reason: "provider refund after conversion",
	})
	require.NoError(t, err)
	assert.False(t, recovered.Replayed)
	assert.Equal(t, int64(100), recovered.GrossCredit)
	assert.Equal(t, int64(10), recovered.SettlementDebt)
	require.NoError(t, DB.First(&balance, balance.Id).Error)
	assert.Equal(t, int64(90), balance.TokenLimit)
	assert.Equal(t, int64(100), balance.TokenUsed)
	require.NoError(t, DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusRefunded, order.Status)
	assert.Equal(t, recovered.LedgerId, order.RecoveryLedgerID)
	var recoveryLedgerCount int64
	require.NoError(t, DB.Model(&CreditBalanceLedger{}).
		Where("source_type = ? AND source_id = ?", CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).
		Count(&recoveryLedgerCount).Error)
	assert.Equal(t, int64(1), recoveryLedgerCount)
}

func TestExpiredNonActiveConversionDoesNotAutoSelectCreditBalance(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}))

	const userID = 10_151
	const sourceID = 10_152
	const planID = 10_153
	user := User{Id: userID, Username: "conversion-non-active", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = 0
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyTimedFirst
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	seedConversionQuoteTimedPlan(t, planID, 100)
	now := GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: sourceID, UserId: userID, PlanId: planID,
		EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit:      100, TokenUsed: 25,
		GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 40*24*60*60, EndTime: now - 60,
		Status:                  SubscriptionStatusExpired,
		LastGrantedAt:           now - 40*24*60*60,
		LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)

	result, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "non-active-preserve-key")
	require.NoError(t, err)
	require.NotNil(t, result)
	var persisted User
	require.NoError(t, DB.First(&persisted, userID).Error)
	assert.Zero(t, persisted.GetSetting().ActiveSubscriptionId)
	assert.Equal(t, SubscriptionBillingStrategyTimedFirst, persisted.GetSetting().SubscriptionBillingStrategy)
}

func TestExpiredSelectedConversionPreservesStoredSelection(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}))

	const userID = 10_161
	const sourceID = 10_162
	const planID = 10_163
	user := User{Id: userID, Username: "conversion-expired-selected", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = sourceID
	setting.SubscriptionBillingStrategy = SubscriptionBillingStrategyTimedFirst
	user.SetSetting(setting)
	require.NoError(t, DB.Create(&user).Error)
	seedConversionQuoteTimedPlan(t, planID, 100)
	now := GetDBTimestamp()
	basis := int64(100)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: sourceID, UserId: userID, PlanId: planID,
		EntitlementType: SubscriptionEntitlementTimed,
		TokenLimit:      100, TokenUsed: 25,
		GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder,
		StartTime: now - 40*24*60*60, EndTime: now - 60,
		Status:                  SubscriptionStatusExpired,
		LastGrantedAt:           now - 40*24*60*60,
		LastGrantCreditSnapshot: &basis,
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         SubscriptionGrantOrder,
	}).Error)

	result, err := ConfirmTimedSubscriptionConversion(userID, sourceID, "expired-selected-preserve-key")
	require.NoError(t, err)
	require.NotNil(t, result)
	var persisted User
	require.NoError(t, DB.First(&persisted, userID).Error)
	assert.Equal(t, sourceID, persisted.GetSetting().ActiveSubscriptionId)
	assert.Equal(t, SubscriptionBillingStrategyTimedFirst, persisted.GetSetting().SubscriptionBillingStrategy)
}

func TestConvertedSubscriptionRejectsOrdinaryAdminMutation(t *testing.T) {
	setupSubscriptionConversionQuoteTestDB(t)
	require.NoError(t, DB.Create(&UserSubscription{
		Id: 10_131, UserId: 10_132, PlanId: 10_133,
		EntitlementType: SubscriptionEntitlementTimed,
		Status:          SubscriptionStatusConverted,
		ConvertedAt:     GetDBTimestamp(), ConversionId: 10_134,
		ConvertedToSubscriptionId: 10_135,
	}).Error)

	_, err := AdminInvalidateUserSubscription(10_131)
	require.ErrorContains(t, err, "converted")
	_, err = AdminDeleteUserSubscription(10_131)
	require.ErrorContains(t, err, "converted")

	var persisted UserSubscription
	require.NoError(t, DB.First(&persisted, 10_131).Error)
	assert.Equal(t, SubscriptionStatusConverted, persisted.Status)
	assert.NotZero(t, persisted.ConvertedAt)
	assert.Equal(t, 10_134, persisted.ConversionId)
	assert.Equal(t, 10_135, persisted.ConvertedToSubscriptionId)
}
