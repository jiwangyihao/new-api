package model

import (
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNormalizeInvitationRewardModeDefaultsToSubscription(t *testing.T) {
	assert.Equal(t, InvitationRewardModeSubscription, NormalizeInvitationRewardMode(""))
	assert.Equal(t, InvitationRewardModeSubscription, NormalizeInvitationRewardMode("bad"))
	assert.Equal(t, InvitationRewardModeSubscription, NormalizeInvitationRewardMode(" subscription "))
	assert.Equal(t, InvitationRewardModeCommission, NormalizeInvitationRewardMode("commission"))
}

func TestUserInvitationRewardModeDefaultMigratesAsSubscription(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}))
	modeUser := invitationCommissionTestUser(9101, "mode-default")
	require.NoError(t, DB.Create(&modeUser).Error)

	var user User
	require.NoError(t, DB.First(&user, 9101).Error)
	assert.Equal(t, InvitationRewardModeSubscription, user.NormalizedInvitationRewardMode())
}

func TestUserUpdatePreservesInvitationRewardMode(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, DB.Create(&User{Id: 9102, Username: "mode-preserve", Status: common.UserStatusEnabled, InvitationRewardMode: InvitationRewardModeSubscription}).Error)

	updated := User{Id: 9102, Username: "mode-preserve", DisplayName: "self", InvitationRewardMode: InvitationRewardModeCommission}
	require.NoError(t, updated.Update(false))

	var user User
	require.NoError(t, DB.First(&user, 9102).Error)
	assert.Equal(t, InvitationRewardModeSubscription, user.InvitationRewardMode)
	assert.Equal(t, "self", user.DisplayName)
}

func TestUserInvitationSummariesIncludeSparseCommissionStats(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &InvitationRewardEvent{}, &InvitationCommissionRecord{}, &InvitationCommissionAccount{}, &InvitationMonthlyEntitlement{}))
	now := common.GetTimestamp()
	plan := seedInvitationCommissionPlan(t, 9121, "sparse_commission_plan", 80, "CNY")

	commissionUser := invitationCommissionTestUser(9122, "commission-stats")
	commissionUser.InvitationRewardMode = InvitationRewardModeCommission
	require.NoError(t, DB.Create(&commissionUser).Error)
	require.NoError(t, DB.Create(&InvitationCommissionAccount{UserId: commissionUser.Id, AvailableCents: 1200, PendingCents: 300, WithdrawnCents: 400, TransferredCents: 500, CreatedAt: now, UpdatedAt: now}).Error)
	commissionInvitee := invitationCommissionTestUser(9123, "commission-stats-child")
	commissionInvitee.InviterId = commissionUser.Id
	require.NoError(t, DB.Create(&commissionInvitee).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9124, UserId: commissionInvitee.Id, PlanId: plan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&InvitationRewardEvent{Id: 9125, InviterId: commissionUser.Id, InviteeId: commissionInvitee.Id, SourceType: InvitationRewardEventSourceLegacySubscription, SourceId: 9124, SourceSubscriptionId: 9124, SourceAmountCents: 8000, SourceCurrency: "CNY", EventStartTime: now - 3600, EventEndTime: now + 86400, Status: InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now}).Error)
	require.NoError(t, DB.Create(&InvitationCommissionRecord{Id: 9126, EventId: 9125, InviterId: commissionUser.Id, InviteeId: commissionInvitee.Id, SourceType: InvitationCommissionSourceLegacySubscription, SourceId: 9124, SourceAmountCents: 8000, SourceCurrency: "CNY", CommissionRateBps: 1000, CommissionCents: 800, Status: InvitationCommissionStatusAvailable, CreatedAt: now, AvailableAt: now}).Error)

	prospectUser := invitationCommissionTestUser(9127, "commission-prospect")
	prospectUser.InvitationRewardMode = InvitationRewardModeSubscription
	require.NoError(t, DB.Create(&prospectUser).Error)
	prospectInvitee := invitationCommissionTestUser(9128, "commission-prospect-child")
	prospectInvitee.InviterId = prospectUser.Id
	require.NoError(t, DB.Create(&prospectInvitee).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9129, UserId: prospectInvitee.Id, PlanId: plan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&InvitationRewardEvent{Id: 9130, InviterId: prospectUser.Id, InviteeId: prospectInvitee.Id, SourceType: InvitationRewardEventSourceLegacySubscription, SourceId: 9129, SourceSubscriptionId: 9129, SourceAmountCents: 8000, SourceCurrency: "CNY", EventStartTime: now - 3600, EventEndTime: now + 86400, Status: InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now}).Error)
	for i := 0; i < 10; i++ {
		subscriptionId := 9131 + i
		eventId := 9141 + i
		require.NoError(t, DB.Create(&UserSubscription{Id: subscriptionId, UserId: prospectInvitee.Id, PlanId: plan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
		require.NoError(t, DB.Create(&InvitationRewardEvent{Id: eventId, InviterId: prospectUser.Id, InviteeId: prospectInvitee.Id, SourceType: InvitationRewardEventSourceLegacySubscription, SourceId: subscriptionId, SourceSubscriptionId: subscriptionId, SourceAmountCents: 1, SourceCurrency: "CNY", EventStartTime: now - 3600, EventEndTime: now + 86400, Status: InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now}).Error)
	}

	users, _, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	byID := make(map[int]*User, len(users))
	for _, user := range users {
		byID[user.Id] = user
	}

	require.Contains(t, byID, commissionUser.Id)
	assert.Equal(t, int64(1200), byID[commissionUser.Id].InvitationCommissionAvailableCents)
	assert.Equal(t, int64(300), byID[commissionUser.Id].InvitationCommissionPendingCents)
	assert.Equal(t, int64(400), byID[commissionUser.Id].InvitationCommissionWithdrawnCents)
	assert.Equal(t, int64(500), byID[commissionUser.Id].InvitationCommissionTransferredCents)
	assert.Equal(t, int64(2400), byID[commissionUser.Id].InvitationCommissionEarnedCents)
	assert.Equal(t, int64(0), byID[commissionUser.Id].InvitationCommissionEstimatedCents)

	require.Contains(t, byID, prospectUser.Id)
	assert.Equal(t, int64(0), byID[prospectUser.Id].InvitationCommissionAvailableCents)
	assert.Equal(t, 11, byID[prospectUser.Id].InvitationCommissionEstimatedEventCount)
	assert.Equal(t, int64(8010), byID[prospectUser.Id].InvitationCommissionEstimatedSourceAmountCents)
	assert.Equal(t, int64(800), byID[prospectUser.Id].InvitationCommissionEstimatedCents)
}

func TestInvitationCommissionModelsAutoMigrateAndSourceUniqueness(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(
		&InvitationCommissionAccount{},
		&InvitationRewardEvent{},
		&InvitationCommissionRecord{},
		&InvitationCommissionLedger{},
		&InvitationCommissionWithdrawal{},
	))

	first := InvitationRewardEvent{
		InviterId: 1, InviteeId: 2,
		SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 100,
		Status: InvitationRewardEventStatusActive, CreatedAt: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&first).Error)
	duplicate := first
	duplicate.Id = 0
	require.Error(t, DB.Create(&duplicate).Error)

	redemptionEvent := InvitationRewardEvent{
		InviterId: 1, InviteeId: 3,
		SourceType: InvitationRewardEventSourceSubscriptionRedemption, SourceId: 101, SourceRedemptionId: 101,
		Status: InvitationRewardEventStatusActive, CreatedAt: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&redemptionEvent).Error)

	account := InvitationCommissionAccount{UserId: 1, AvailableCents: 10}
	require.NoError(t, DB.Create(&account).Error)
	require.Error(t, DB.Create(&InvitationCommissionAccount{UserId: 1}).Error)
}

func TestSubscriptionRedemptionStoresAmountSnapshot(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Redemption{}))

	redemption := Redemption{UserId: 1, Name: "sub-snapshot", Key: "sub-snapshot-key", Type: RedemptionTypeSubscription, PlanId: 9101, AmountCents: 8000, Currency: "CNY", Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, DB.Create(&redemption).Error)

	var saved Redemption
	require.NoError(t, DB.Where("`key` = ?", "sub-snapshot-key").First(&saved).Error)
	assert.Equal(t, int64(8000), saved.AmountCents)
	assert.Equal(t, "CNY", saved.Currency)
}

func TestSubscriptionPlanPriceAmountSnapshotUsesDecimalCents(t *testing.T) {
	cases := []struct {
		name         string
		plan         SubscriptionPlan
		wantCents    int64
		wantCurrency string
		wantOK       bool
	}{
		{name: "one cent", plan: SubscriptionPlan{PriceAmount: 0.01, Currency: "CNY"}, wantCents: 1, wantCurrency: "CNY", wantOK: true},
		{name: "decimal price", plan: SubscriptionPlan{PriceAmount: 39.99, Currency: "CNY"}, wantCents: 3999, wantCurrency: "CNY", wantOK: true},
		{name: "non cny", plan: SubscriptionPlan{PriceAmount: 39.99, Currency: "USD"}, wantCents: 3999, wantCurrency: "USD", wantOK: true},
		{name: "overflow", plan: SubscriptionPlan{PriceAmount: math.MaxFloat64, Currency: "CNY"}, wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cents, currency, ok := SubscriptionPlanAmountSnapshot(&tc.plan)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantCents, cents)
			assert.Equal(t, tc.wantCurrency, currency)
		})
	}
}

func invitationCommissionTestUser(id int, username string) User {
	return User{Id: id, Username: username, Status: common.UserStatusEnabled, AffCode: username}
}

func seedInvitationCommissionPlan(t *testing.T, id int, code string, price float64, currency string) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{Id: id, Title: code, PriceAmount: price, Currency: currency, Enabled: true, PublicVisible: true, RewardEligible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code, DurationUnit: SubscriptionDurationDay, DurationValue: 30}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func resetInvitationCommissionModelTables(t *testing.T) {
	t.Helper()
	cleanup := func() {
		DB.Exec("DELETE FROM invitation_commission_withdrawals")
		DB.Exec("DELETE FROM invitation_commission_ledgers")
		DB.Exec("DELETE FROM invitation_commission_records")
		DB.Exec("DELETE FROM invitation_commission_accounts")
		DB.Exec("DELETE FROM invitation_reward_events")
		DB.Exec("DELETE FROM redemptions")
		DB.Exec("DELETE FROM user_subscriptions")
		DB.Exec("DELETE FROM subscription_orders")
		DB.Exec("DELETE FROM subscription_plans")
		DB.Exec("DELETE FROM users")
		primaryBillableSubscriptionCache = sync.Map{}
	}
	cleanup()
	t.Cleanup(cleanup)
}

func assertInvitationRewardEventHasNoRewardModeColumn(t *testing.T) {
	t.Helper()
	columns, err := DB.Migrator().ColumnTypes(&InvitationRewardEvent{})
	require.NoError(t, err)
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name())
	}
	assert.NotContains(t, names, "reward_mode")
}

func TestCompleteSubscriptionOrderTxCreatesInvitationRewardEventAtTransition(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &InvitationRewardEvent{}))
	inviter := invitationCommissionTestUser(9201, "inviter")
	inviter.InvitationRewardMode = InvitationRewardModeCommission
	invitee := invitationCommissionTestUser(9202, "invitee")
	invitee.InviterId = 9201
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	_ = seedInvitationCommissionPlan(t, 9203, "commission_plan", 100, "CNY")
	order := SubscriptionOrder{UserId: 9202, PlanId: 9203, Money: 100, AmountCents: 10000, Currency: "CNY", TradeNo: "source-at-transition", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, DB.Create(&order).Error)

	var result *SubscriptionOrderCompletionResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var locked SubscriptionOrder
		require.NoError(t, tx.Where("trade_no = ?", order.TradeNo).First(&locked).Error)
		var err error
		result, err = CompleteSubscriptionOrderTx(tx, &locked, "{}", "alipay")
		return err
	}))

	require.NotNil(t, result)
	assert.True(t, result.Transitioned)
	assert.Equal(t, 9201, result.InviterId)
	require.Greater(t, result.SourceSubscriptionId, 0)
	assert.Greater(t, result.EventEndTime, result.EventStartTime)
	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error)
	assert.Equal(t, 9201, event.InviterId)
	assert.Equal(t, 9202, event.InviteeId)
	assert.Equal(t, order.Id, event.SourceOrderId)
	assert.Equal(t, order.Id, event.SourceId)
	assert.Equal(t, result.SourceSubscriptionId, event.SourceSubscriptionId)
	assert.Equal(t, result.EventStartTime, event.EventStartTime)
	assert.Equal(t, result.EventEndTime, event.EventEndTime)
	assert.Equal(t, int64(10000), event.SourceAmountCents)
	assert.Equal(t, "CNY", event.SourceCurrency)
	assertInvitationRewardEventHasNoRewardModeColumn(t)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", 9201).Update("invitation_reward_mode", InvitationRewardModeSubscription).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var completed SubscriptionOrder
		require.NoError(t, tx.Where("trade_no = ?", order.TradeNo).First(&completed).Error)
		retry, err := CompleteSubscriptionOrderTx(tx, &completed, "{}", "alipay")
		require.NoError(t, err)
		require.NotNil(t, retry)
		assert.False(t, retry.Transitioned)
		assert.Equal(t, result.InviterId, retry.InviterId)
		assert.Equal(t, result.SourceSubscriptionId, retry.SourceSubscriptionId)
		assert.Equal(t, result.EventStartTime, retry.EventStartTime)
		assert.Equal(t, result.EventEndTime, retry.EventEndTime)
		return nil
	}))
	var count int64
	require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestCompleteSubscriptionOrderTxEventIntervalUsesOnlyRenewalDelta(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &InvitationRewardEvent{}))
	now := common.GetTimestamp()
	inviter := invitationCommissionTestUser(9211, "renew-inviter")
	invitee := invitationCommissionTestUser(9212, "renew-invitee")
	invitee.InviterId = 9211
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	_ = seedInvitationCommissionPlan(t, 9213, "renew_plan", 50, "CNY")
	require.NoError(t, DB.Create(&UserSubscription{UserId: 9212, PlanId: 9213, Status: "active", StartTime: now - 100, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	order := SubscriptionOrder{UserId: 9212, PlanId: 9213, AmountCents: 5000, Currency: "CNY", TradeNo: "renew-delta", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: now}
	require.NoError(t, DB.Create(&order).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var locked SubscriptionOrder
		require.NoError(t, tx.Where("trade_no = ?", order.TradeNo).First(&locked).Error)
		_, err := CompleteSubscriptionOrderTx(tx, &locked, "{}", "alipay")
		return err
	}))

	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error)
	assert.Equal(t, now+86400, event.EventStartTime)
	assert.Greater(t, event.EventEndTime, event.EventStartTime)
}

func TestRedeemSubscriptionRedemptionCreatesInvitationRewardEvent(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &Redemption{}, &InvitationRewardEvent{}))
	inviter := invitationCommissionTestUser(9221, "redeem-inviter")
	inviter.InvitationRewardMode = InvitationRewardModeCommission
	invitee := invitationCommissionTestUser(9222, "redeem-invitee")
	invitee.InviterId = 9221
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	_ = seedInvitationCommissionPlan(t, 9223, "redeem_plan", 80, "CNY")
	redemption := Redemption{Id: 9224, Key: "redeem-source-key", Status: common.RedemptionCodeStatusEnabled, Type: RedemptionTypeSubscription, PlanId: 9223, AmountCents: 8000, Currency: "CNY", CreatedTime: common.GetTimestamp()}
	require.NoError(t, DB.Create(&redemption).Error)

	result, err := Redeem("redeem-source-key", 9222)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, RedemptionTypeSubscription, result.Type)
	assert.Equal(t, redemption.Id, result.RedemptionId)
	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionRedemption, redemption.Id).First(&event).Error)
	assert.Equal(t, 9221, event.InviterId)
	assert.Equal(t, 9222, event.InviteeId)
	assert.Equal(t, redemption.Id, event.SourceRedemptionId)
	assert.Equal(t, redemption.Id, event.SourceId)
	require.Greater(t, event.SourceSubscriptionId, 0)
	assert.Equal(t, int64(8000), event.SourceAmountCents)
	assert.Equal(t, "CNY", event.SourceCurrency)
}

func TestRedeemSubscriptionRedemptionRecordsEventForRewardIneligiblePlan(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &Redemption{}, &InvitationRewardEvent{}))
	inviter := invitationCommissionTestUser(9225, "redeem-ineligible-inviter")
	invitee := invitationCommissionTestUser(9226, "redeem-ineligible-invitee")
	invitee.InviterId = 9225
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	plan := seedInvitationCommissionPlan(t, 9227, "redeem_ineligible_plan", 80, "CNY")
	require.NoError(t, DB.Model(plan).Update("reward_eligible", false).Error)
	redemption := Redemption{Id: 9228, Key: "redeem-ineligible-key", Status: common.RedemptionCodeStatusEnabled, Type: RedemptionTypeSubscription, PlanId: 9227, AmountCents: 8000, Currency: "CNY", CreatedTime: common.GetTimestamp()}
	require.NoError(t, DB.Create(&redemption).Error)

	result, err := Redeem("redeem-ineligible-key", 9226)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, RedemptionTypeSubscription, result.Type)
	assert.Equal(t, redemption.Id, result.RedemptionId)
	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionRedemption, redemption.Id).First(&event).Error)
	assert.Equal(t, redemption.Id, event.SourceRedemptionId)
	require.Greater(t, event.SourceSubscriptionId, 0)
	assert.Equal(t, int64(8000), event.SourceAmountCents)
	assert.Equal(t, "CNY", event.SourceCurrency)
}

func TestCompleteSubscriptionOrderReturnsResultForSuccessRetry(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &InvitationRewardEvent{}))
	inviter := invitationCommissionTestUser(9231, "retry-inviter")
	inviter.InvitationRewardMode = InvitationRewardModeCommission
	invitee := invitationCommissionTestUser(9232, "retry-invitee")
	invitee.InviterId = 9231
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	_ = seedInvitationCommissionPlan(t, 9233, "retry_plan", 60, "CNY")
	order := SubscriptionOrder{UserId: 9232, PlanId: 9233, AmountCents: 6000, Currency: "CNY", TradeNo: "retry-existing-event", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, DB.Create(&order).Error)

	first, err := CompleteSubscriptionOrder("retry-existing-event", "{}", PaymentProviderEpay, "alipay")
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.True(t, first.Transitioned)
	assert.Equal(t, 9231, first.InviterId)
	require.Greater(t, first.SourceSubscriptionId, 0)

	retry, err := CompleteSubscriptionOrder("retry-existing-event", "{}", PaymentProviderEpay, "alipay")
	require.NoError(t, err)
	require.NotNil(t, retry)
	assert.False(t, retry.Transitioned)
	assert.Equal(t, first.InviterId, retry.InviterId)
	assert.Equal(t, first.SourceSubscriptionId, retry.SourceSubscriptionId)
	assert.Equal(t, first.EventStartTime, retry.EventStartTime)
	assert.Equal(t, first.EventEndTime, retry.EventEndTime)

	var count int64
	require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestCompleteSubscriptionOrderRecordsEventForRewardIneligiblePlan(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &TopUp{}, &InvitationRewardEvent{}))
	inviter := invitationCommissionTestUser(9261, "ineligible-order-inviter")
	invitee := invitationCommissionTestUser(9262, "ineligible-order-invitee")
	invitee.InviterId = inviter.Id
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	plan := seedInvitationCommissionPlan(t, 9263, "ineligible_order_plan", 110, "CNY")
	require.NoError(t, DB.Model(plan).Update("reward_eligible", false).Error)
	order := SubscriptionOrder{UserId: invitee.Id, PlanId: plan.Id, Money: 110, AmountCents: 11000, Currency: "CNY", TradeNo: "reward-ineligible-order-event", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, DB.Create(&order).Error)

	result, err := CompleteSubscriptionOrder(order.TradeNo, `{"money":"110.00","currency":"CNY"}`, PaymentProviderEpay, "alipay")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Transitioned)
	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error)
	assert.Equal(t, inviter.Id, event.InviterId)
	assert.Equal(t, invitee.Id, event.InviteeId)
	assert.Equal(t, order.Id, event.SourceOrderId)
	assert.Equal(t, result.SourceSubscriptionId, event.SourceSubscriptionId)
	assert.Equal(t, int64(11000), event.SourceAmountCents)
	assert.Equal(t, "CNY", event.SourceCurrency)
	var eventCount int64
	require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestCompleteSubscriptionOrderConcurrentClaimCreatesSingleSubscriptionAndEvent(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &InvitationRewardEvent{}))
	inviter := invitationCommissionTestUser(9241, "concurrent-inviter")
	inviter.InvitationRewardMode = InvitationRewardModeCommission
	invitee := invitationCommissionTestUser(9242, "concurrent-invitee")
	invitee.InviterId = 9241
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	_ = seedInvitationCommissionPlan(t, 9243, "concurrent_order_plan", 70, "CNY")
	order := SubscriptionOrder{UserId: 9242, PlanId: 9243, AmountCents: 7000, Currency: "CNY", TradeNo: "concurrent-order", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, DB.Create(&order).Error)

	const workers = 8
	results := make(chan *SubscriptionOrderCompletionResult, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := CompleteSubscriptionOrder("concurrent-order", "{}", PaymentProviderEpay, "alipay")
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	transitioned := 0
	completed := 0
	for result := range results {
		if result == nil {
			continue
		}
		completed++
		if result.Transitioned {
			transitioned++
		}
	}
	require.GreaterOrEqual(t, completed+len(errs), 1)
	assert.Equal(t, 1, transitioned)
	var orderAfter SubscriptionOrder
	require.NoError(t, DB.Where("trade_no = ?", "concurrent-order").First(&orderAfter).Error)
	assert.Equal(t, common.TopUpStatusSuccess, orderAfter.Status)
	var subscriptionCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", 9242, 9243).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
	var eventCount int64
	require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestRedeemSubscriptionRedemptionConcurrentClaimCreatesSingleSubscriptionAndEvent(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &Redemption{}, &InvitationRewardEvent{}))
	inviter := invitationCommissionTestUser(9251, "redeem-race-inviter")
	inviter.InvitationRewardMode = InvitationRewardModeCommission
	inviteeA := invitationCommissionTestUser(9252, "redeem-race-invitee-a")
	inviteeA.InviterId = 9251
	inviteeB := invitationCommissionTestUser(9253, "redeem-race-invitee-b")
	inviteeB.InviterId = 9251
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&inviteeA).Error)
	require.NoError(t, DB.Create(&inviteeB).Error)
	_ = seedInvitationCommissionPlan(t, 9254, "concurrent_redemption_plan", 90, "CNY")
	redemption := Redemption{Id: 9255, Key: "redeem-race-key", Status: common.RedemptionCodeStatusEnabled, Type: RedemptionTypeSubscription, PlanId: 9254, AmountCents: 9000, Currency: "CNY", CreatedTime: common.GetTimestamp()}
	require.NoError(t, DB.Create(&redemption).Error)

	userIDs := []int{9252, 9253}
	successes := make(chan int, len(userIDs))
	errs := make(chan error, len(userIDs))
	var wg sync.WaitGroup
	for _, userID := range userIDs {
		userID := userID
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := Redeem("redeem-race-key", userID)
			if err != nil {
				errs <- err
				return
			}
			if result == nil || result.Type != RedemptionTypeSubscription || result.RedemptionId != redemption.Id {
				errs <- assert.AnError
				return
			}
			successes <- userID
		}()
	}
	wg.Wait()
	close(successes)
	close(errs)

	successfulUsers := make([]int, 0, len(userIDs))
	for userID := range successes {
		successfulUsers = append(successfulUsers, userID)
	}
	require.Len(t, successfulUsers, 1)
	winnerID := successfulUsers[0]
	var saved Redemption
	require.NoError(t, DB.First(&saved, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusUsed, saved.Status)
	assert.Equal(t, winnerID, saved.UsedUserId)
	var subscriptionCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("plan_id = ?", 9254).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
	var winnerSubscriptionCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", winnerID, 9254).Count(&winnerSubscriptionCount).Error)
	assert.Equal(t, int64(1), winnerSubscriptionCount)
	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionRedemption, redemption.Id).First(&event).Error)
	assert.Equal(t, winnerID, event.InviteeId)
	assert.Equal(t, redemption.Id, event.SourceRedemptionId)
	require.Greater(t, event.SourceSubscriptionId, 0)
	var eventCount int64
	require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionRedemption, redemption.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestBackfillLegacyInvitationRewardEventsPreservesExistingCommissionSources(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &InvitationRewardEvent{}))
	now := common.GetTimestamp()
	paidPlan := seedInvitationCommissionPlan(t, 9331, "legacy_paid", 80, "CNY")
	trialPlan := seedInvitationCommissionPlan(t, 9332, "legacy_trial", 0, "CNY")
	require.NoError(t, DB.Model(trialPlan).Update("is_trial", true).Error)
	rewardPlan := seedInvitationCommissionPlan(t, 9333, "legacy_reward", 0, "CNY")
	ineligiblePlan := seedInvitationCommissionPlan(t, 9330, "legacy_ineligible", 80, "CNY")
	require.NoError(t, DB.Model(ineligiblePlan).Update("reward_eligible", false).Error)
	require.NoError(t, DB.Create(&User{Id: 9334, Username: "legacy-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-inviter"}).Error)
	for _, userID := range []int{9335, 9336, 9337, 9338, 9349, 9352, 9353, 9360, 9363, 9366, 9369} {
		child := invitationCommissionTestUser(userID, fmt.Sprintf("legacy-child-%d", userID))
		child.InviterId = 9334
		require.NoError(t, DB.Create(&child).Error)
	}
	require.NoError(t, DB.Create(&UserSubscription{Id: 9339, UserId: 9335, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9340, UserId: 9335, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-paid-order", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9341, UserId: 9336, PlanId: trialPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: "trial_code", Source: "trial_code"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9364, UserId: 9363, PlanId: trialPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9365, UserId: 9363, PlanId: trialPlan.Id, Status: common.TopUpStatusSuccess, Money: 0, AmountCents: 0, Currency: "CNY", TradeNo: "legacy-trial-plan-order", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9367, UserId: 9366, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: "trial_code", Source: "trial_code"}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9368, UserId: 9366, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-trial-code-source", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9370, UserId: 9369, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: "invite_trial", Source: "invite_trial"}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9371, UserId: 9369, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-invite-trial-source", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9342, UserId: 9337, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: "admin", Source: "admin"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9343, UserId: 9338, PlanId: rewardPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantMonthlyInviteEntitlement, Source: SubscriptionGrantMonthlyInviteEntitlement}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9350, UserId: 9349, PlanId: ineligiblePlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9351, UserId: 9349, PlanId: ineligiblePlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-ineligible-order", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9354, UserId: 9352, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9355, UserId: 9352, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-ambiguous-a", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9356, UserId: 9352, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 90, AmountCents: 9000, Currency: "CNY", TradeNo: "legacy-ambiguous-b", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3400}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9357, UserId: 9353, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9361, UserId: 9360, PlanId: paidPlan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9362, UserId: 9360, PlanId: paidPlan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-existing-order", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)
	require.NoError(t, DB.Create(&InvitationRewardEvent{InviterId: 9334, InviteeId: 9360, SourceType: InvitationRewardEventSourceSubscriptionOrder, SourceId: 9362, SourceOrderId: 9362, SourceSubscriptionId: 9361, SourceAmountCents: 8000, SourceCurrency: "CNY", EventStartTime: now - 3600, EventEndTime: now + 86400, Status: InvitationRewardEventStatusActive, CreatedAt: now}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))

	var events []InvitationRewardEvent
	require.NoError(t, DB.Find(&events).Error)
	require.Len(t, events, 5)
	bySource := map[int]InvitationRewardEvent{}
	for _, event := range events {
		bySource[event.SourceId] = event
	}
	require.Contains(t, bySource, 9339)
	require.Contains(t, bySource, 9350)
	require.Contains(t, bySource, 9354)
	require.Contains(t, bySource, 9357)
	require.Contains(t, bySource, 9362)
	assert.NotContains(t, bySource, 9341, "combined trial plan and trial_code source must not be backfilled")
	assert.NotContains(t, bySource, 9364, "is_trial plan with order source must not be backfilled")
	assert.NotContains(t, bySource, 9367, "trial_code source on a paid plan must not be backfilled")
	assert.NotContains(t, bySource, 9370, "invite_trial source on a paid plan must not be backfilled")
	paidEvent := bySource[9339]
	assert.Equal(t, InvitationRewardEventSourceLegacySubscription, paidEvent.SourceType)
	assert.Equal(t, 9339, paidEvent.SourceSubscriptionId)
	assert.Equal(t, 9334, paidEvent.InviterId)
	assert.Equal(t, 9335, paidEvent.InviteeId)
	assert.Equal(t, InvitationRewardEventStatusActive, paidEvent.Status)
	assert.Equal(t, int64(8000), paidEvent.SourceAmountCents)
	assert.Equal(t, "CNY", paidEvent.SourceCurrency)
	ineligibleEvent := bySource[9350]
	assert.Equal(t, InvitationRewardEventSourceLegacySubscription, ineligibleEvent.SourceType)
	assert.Equal(t, 9350, ineligibleEvent.SourceSubscriptionId)
	assert.Equal(t, 9334, ineligibleEvent.InviterId)
	assert.Equal(t, 9349, ineligibleEvent.InviteeId)
	assert.Equal(t, int64(8000), ineligibleEvent.SourceAmountCents)
	assert.Equal(t, "CNY", ineligibleEvent.SourceCurrency)
	ambiguousEvent := bySource[9354]
	assert.Equal(t, InvitationRewardEventSourceLegacySubscription, ambiguousEvent.SourceType)
	assert.Equal(t, int64(0), ambiguousEvent.SourceAmountCents)
	assert.Equal(t, "", ambiguousEvent.SourceCurrency)
	noSnapshotEvent := bySource[9357]
	assert.Equal(t, InvitationRewardEventSourceLegacySubscription, noSnapshotEvent.SourceType)
	assert.Equal(t, int64(0), noSnapshotEvent.SourceAmountCents)
	assert.Equal(t, "", noSnapshotEvent.SourceCurrency)
	existingOrderEvent := bySource[9362]
	assert.Equal(t, InvitationRewardEventSourceSubscriptionOrder, existingOrderEvent.SourceType)
	assert.Equal(t, 9361, existingOrderEvent.SourceSubscriptionId)
	var duplicateLegacyEvents int64
	require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_type = ? AND source_subscription_id = ?", InvitationRewardEventSourceLegacySubscription, 9361).Count(&duplicateLegacyEvents).Error)
	assert.Equal(t, int64(0), duplicateLegacyEvents)
}

func TestBackfillLegacyInvitationRewardEventsDoesNotCopyUnrelatedOrderAmount(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &Redemption{}, &InvitationRewardEvent{}))
	now := common.GetTimestamp()
	plan := seedInvitationCommissionPlan(t, 9381, "legacy_unrelated_order_plan", 42, "CNY")
	require.NoError(t, DB.Create(&User{Id: 9382, Username: "legacy-unrelated-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-unrelated-inviter"}).Error)
	invitee := invitationCommissionTestUser(9383, "legacy-unrelated-invitee")
	invitee.InviterId = 9382
	require.NoError(t, DB.Create(&invitee).Error)

	start := now - 3600
	end := now + 86400
	require.NoError(t, DB.Create(&UserSubscription{
		Id:          9384,
		UserId:      invitee.Id,
		PlanId:      plan.Id,
		Status:      "active",
		StartTime:   start,
		EndTime:     end,
		GrantReason: "legacy_import",
		Source:      "legacy_import",
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{
		Id:              9385,
		UserId:          invitee.Id,
		PlanId:          plan.Id,
		Status:          common.TopUpStatusSuccess,
		Money:           42,
		AmountCents:     4200,
		Currency:        "CNY",
		TradeNo:         "legacy-unrelated-success-order",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      start - 86400*30,
	}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))

	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceLegacySubscription, 9384).First(&event).Error)
	assert.Equal(t, 9382, event.InviterId)
	assert.Equal(t, invitee.Id, event.InviteeId)
	assert.Equal(t, 9384, event.SourceSubscriptionId)
	assert.Equal(t, int64(0), event.SourceAmountCents)
	assert.Equal(t, "", event.SourceCurrency)
	assert.Equal(t, 0, event.SourceOrderId)
}

func TestBackfillLegacyInvitationRewardEventsRequiresAuditableOrderWindow(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &InvitationRewardEvent{}))
	now := common.GetTimestamp()
	plan := seedInvitationCommissionPlan(t, 9391, "legacy_invalid_window_plan", 42, "CNY")
	require.NoError(t, DB.Create(&User{Id: 9392, Username: "legacy-window-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-window-inviter"}).Error)
	invitee := invitationCommissionTestUser(9393, "legacy-window-invitee")
	invitee.InviterId = 9392
	require.NoError(t, DB.Create(&invitee).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:          9394,
		UserId:      invitee.Id,
		PlanId:      plan.Id,
		Status:      "active",
		StartTime:   0,
		EndTime:     now + 86400,
		GrantReason: SubscriptionGrantOrder,
		Source:      SubscriptionGrantOrder,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{
		Id:              9395,
		UserId:          invitee.Id,
		PlanId:          plan.Id,
		Status:          common.TopUpStatusSuccess,
		Money:           42,
		AmountCents:     4200,
		Currency:        "CNY",
		TradeNo:         "legacy-invalid-window-order",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      now - 3600,
	}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))

	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceLegacySubscription, 9394).First(&event).Error)
	assert.Equal(t, int64(0), event.SourceAmountCents)
	assert.Equal(t, "", event.SourceCurrency)
}

func TestBackfillLegacyInvitationRewardEventsDoesNotIgnoreUnsnapshottedCandidateOrders(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &InvitationRewardEvent{}))
	now := common.GetTimestamp()
	plan := seedInvitationCommissionPlan(t, 9401, "legacy_mixed_snapshot_plan", 42, "CNY")
	require.NoError(t, DB.Create(&User{Id: 9402, Username: "legacy-mixed-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-mixed-inviter"}).Error)
	invitee := invitationCommissionTestUser(9403, "legacy-mixed-invitee")
	invitee.InviterId = 9402
	require.NoError(t, DB.Create(&invitee).Error)
	start := now - 3600
	end := now + 86400
	require.NoError(t, DB.Create(&UserSubscription{
		Id:          9404,
		UserId:      invitee.Id,
		PlanId:      plan.Id,
		Status:      "active",
		StartTime:   start,
		EndTime:     end,
		GrantReason: SubscriptionGrantOrder,
		Source:      SubscriptionGrantOrder,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9405, UserId: invitee.Id, PlanId: plan.Id, Status: common.TopUpStatusSuccess, Money: 42, AmountCents: 0, Currency: "", TradeNo: "legacy-mixed-no-snapshot", PaymentProvider: PaymentProviderEpay, CreateTime: start + 60}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9406, UserId: invitee.Id, PlanId: plan.Id, Status: common.TopUpStatusSuccess, Money: 42, AmountCents: 4200, Currency: "CNY", TradeNo: "legacy-mixed-with-snapshot", PaymentProvider: PaymentProviderEpay, CreateTime: start + 120}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))

	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceLegacySubscription, 9404).First(&event).Error)
	assert.Equal(t, int64(0), event.SourceAmountCents)
	assert.Equal(t, "", event.SourceCurrency)
}

func TestBackfillLegacyInvitationRewardEventsCopiesAuditableRedemptionSnapshot(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &Redemption{}, &InvitationRewardEvent{}))
	now := common.GetTimestamp()
	plan := seedInvitationCommissionPlan(t, 9411, "legacy_redemption_plan", 42, "CNY")
	require.NoError(t, DB.Create(&User{Id: 9412, Username: "legacy-redemption-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-redemption-inviter"}).Error)
	invitee := invitationCommissionTestUser(9413, "legacy-redemption-invitee")
	invitee.InviterId = 9412
	require.NoError(t, DB.Create(&invitee).Error)
	start := now - 3600
	end := now + 86400
	require.NoError(t, DB.Create(&UserSubscription{
		Id:          9414,
		UserId:      invitee.Id,
		PlanId:      plan.Id,
		Status:      "active",
		StartTime:   start,
		EndTime:     end,
		GrantReason: "redemption",
		Source:      "redemption",
	}).Error)
	require.NoError(t, DB.Create(&Redemption{
		Id:           9415,
		Key:          "legacy-redemption-snapshot",
		Status:       common.RedemptionCodeStatusUsed,
		Type:         RedemptionTypeSubscription,
		PlanId:       plan.Id,
		AmountCents:  4200,
		Currency:     "CNY",
		CreatedTime:  start - 60,
		RedeemedTime: start + 60,
		UsedUserId:   invitee.Id,
	}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))

	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceLegacySubscription, 9414).First(&event).Error)
	assert.Equal(t, int64(4200), event.SourceAmountCents)
	assert.Equal(t, "CNY", event.SourceCurrency)
}

func TestBackfillLegacyInvitationRewardEventsDoesNotCopyAmbiguousRedemptionSnapshot(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &Redemption{}, &InvitationRewardEvent{}))
	now := common.GetTimestamp()
	plan := seedInvitationCommissionPlan(t, 9431, "legacy_ambiguous_redemption_plan", 42, "CNY")
	require.NoError(t, DB.Create(&User{Id: 9432, Username: "legacy-ambiguous-redemption-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-ambiguous-redemption-inviter"}).Error)
	invitee := invitationCommissionTestUser(9433, "legacy-ambiguous-redemption-invitee")
	invitee.InviterId = 9432
	require.NoError(t, DB.Create(&invitee).Error)
	start := now - 3600
	end := now + 86400
	require.NoError(t, DB.Create(&UserSubscription{Id: 9434, UserId: invitee.Id, PlanId: plan.Id, Status: "active", StartTime: start, EndTime: end, GrantReason: "redemption", Source: "redemption"}).Error)
	require.NoError(t, DB.Create(&Redemption{Id: 9435, Key: "legacy-ambiguous-redemption-a", Status: common.RedemptionCodeStatusUsed, Type: RedemptionTypeSubscription, PlanId: plan.Id, AmountCents: 4200, Currency: "CNY", CreatedTime: start - 60, RedeemedTime: start + 60, UsedUserId: invitee.Id}).Error)
	require.NoError(t, DB.Create(&Redemption{Id: 9436, Key: "legacy-ambiguous-redemption-b", Status: common.RedemptionCodeStatusUsed, Type: RedemptionTypeSubscription, PlanId: plan.Id, AmountCents: 5200, Currency: "CNY", CreatedTime: start - 30, RedeemedTime: start + 120, UsedUserId: invitee.Id}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))

	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceLegacySubscription, 9434).First(&event).Error)
	assert.Equal(t, int64(0), event.SourceAmountCents)
	assert.Equal(t, "", event.SourceCurrency)
}

func TestBackfillLegacyInvitationRewardEventsDoesNotCopyUnsnapshottedRedemptionCandidate(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &Redemption{}, &InvitationRewardEvent{}))
	now := common.GetTimestamp()
	plan := seedInvitationCommissionPlan(t, 9441, "legacy_unsnapshotted_redemption_plan", 42, "CNY")
	require.NoError(t, DB.Create(&User{Id: 9442, Username: "legacy-unsnapshotted-redemption-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-unsnapshotted-redemption-inviter"}).Error)
	invitee := invitationCommissionTestUser(9443, "legacy-unsnapshotted-redemption-invitee")
	invitee.InviterId = 9442
	require.NoError(t, DB.Create(&invitee).Error)
	start := now - 3600
	end := now + 86400
	require.NoError(t, DB.Create(&UserSubscription{Id: 9444, UserId: invitee.Id, PlanId: plan.Id, Status: "active", StartTime: start, EndTime: end, GrantReason: "redemption", Source: "redemption"}).Error)
	require.NoError(t, DB.Create(&Redemption{Id: 9445, Key: "legacy-unsnapshotted-redemption", Status: common.RedemptionCodeStatusUsed, Type: RedemptionTypeSubscription, PlanId: plan.Id, AmountCents: 0, Currency: "", CreatedTime: start - 60, RedeemedTime: start + 60, UsedUserId: invitee.Id}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))

	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceLegacySubscription, 9444).First(&event).Error)
	assert.Equal(t, int64(0), event.SourceAmountCents)
	assert.Equal(t, "", event.SourceCurrency)
}

func TestBackfillLegacyInvitationRewardEventsPrefersRedemptionSourceOverOrderDefault(t *testing.T) {
	resetInvitationCommissionModelTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &Redemption{}, &InvitationRewardEvent{}))
	now := common.GetTimestamp()
	plan := seedInvitationCommissionPlan(t, 9421, "legacy_redemption_mixed_plan", 42, "CNY")
	require.NoError(t, DB.Create(&User{Id: 9422, Username: "legacy-redemption-mixed-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-redemption-mixed-inviter"}).Error)
	invitee := invitationCommissionTestUser(9423, "legacy-redemption-mixed-invitee")
	invitee.InviterId = 9422
	require.NoError(t, DB.Create(&invitee).Error)
	start := now - 3600
	end := now + 86400
	require.NoError(t, DB.Create(&UserSubscription{
		Id:          9424,
		UserId:      invitee.Id,
		PlanId:      plan.Id,
		Status:      "active",
		StartTime:   start,
		EndTime:     end,
		GrantReason: "redemption",
		Source:      SubscriptionGrantOrder,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9425, UserId: invitee.Id, PlanId: plan.Id, Status: common.TopUpStatusSuccess, Money: 10, AmountCents: 1000, Currency: "CNY", TradeNo: "legacy-mixed-order-default", PaymentProvider: PaymentProviderEpay, CreateTime: start + 30}).Error)
	require.NoError(t, DB.Create(&Redemption{
		Id:           9426,
		Key:          "legacy-redemption-mixed-snapshot",
		Status:       common.RedemptionCodeStatusUsed,
		Type:         RedemptionTypeSubscription,
		PlanId:       plan.Id,
		AmountCents:  4200,
		Currency:     "CNY",
		CreatedTime:  start - 60,
		RedeemedTime: start + 60,
		UsedUserId:   invitee.Id,
	}).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return BackfillLegacyInvitationRewardEventsTx(tx, now) }))

	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceLegacySubscription, 9424).First(&event).Error)
	assert.Equal(t, int64(4200), event.SourceAmountCents)
	assert.Equal(t, "CNY", event.SourceCurrency)
}

func TestMigrateDBFastBackfillRunsAfterSubscriptionPlanMigration(t *testing.T) {
	oldDB := DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	db, err := gorm.Open(sqlite.Open("file:migrate_fast_backfill?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		_ = sqlDB.Close()
	})
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}))
	now := common.GetTimestamp()
	plan := seedInvitationCommissionPlan(t, 9344, "legacy_fast_paid", 80, "CNY")
	require.NoError(t, DB.Create(&User{Id: 9345, Username: "legacy-fast-inviter", Status: common.UserStatusEnabled, AffCode: "legacy-fast-inviter"}).Error)
	child := invitationCommissionTestUser(9346, "legacy-fast-child")
	child.InviterId = 9345
	require.NoError(t, DB.Create(&child).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 9347, UserId: 9346, PlanId: plan.Id, Status: "active", StartTime: now - 3600, EndTime: now + 86400, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9348, UserId: 9346, PlanId: plan.Id, Status: common.TopUpStatusSuccess, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "legacy-fast-order", PaymentProvider: PaymentProviderEpay, CreateTime: now - 3500}).Error)

	require.NoError(t, migrateDBFast())

	var event InvitationRewardEvent
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceLegacySubscription, 9347).First(&event).Error)
	assert.Equal(t, 9345, event.InviterId)
	assert.Equal(t, int64(8000), event.SourceAmountCents)
}
