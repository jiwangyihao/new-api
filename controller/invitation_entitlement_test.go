package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvitationEntitlementStatusResponse(t *testing.T) {
	originalDB := model.DB
	db := setupModelListControllerTestDB(t)
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.InvitationMonthlyEntitlement{}))

	at := time.Now().UTC()
	inviterId := 8101
	inviteeA := 8102
	inviteeB := 8103
	require.NoError(t, model.DB.Create(&model.User{Id: inviterId, Username: "entitlement-inviter", Status: common.UserStatusEnabled, AffCode: "entitlement-inviter"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: inviteeA, Username: "entitlement-a", Status: common.UserStatusEnabled, AffCode: "entitlement-a", InviterId: inviterId}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: inviteeB, Username: "entitlement-b", Status: common.UserStatusEnabled, AffCode: "entitlement-b", InviterId: inviterId}).Error)
	basicCode := "basic_monthly"
	paidCode := "standard_monthly"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 8111, Title: "Basic", Enabled: true, PriceAmount: 40, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1_000, ConcurrencyLimit: 1, RewardEligible: true, BusinessCode: &basicCode}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 8112, Title: "Standard", Enabled: true, PriceAmount: 80, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, RewardEligible: true, BusinessCode: &paidCode}).Error)
	start := at.Add(-time.Hour).Unix()
	end := at.Add(time.Hour).Unix()
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: inviteeA, PlanId: 8112, Status: "active", StartTime: start, EndTime: end, GrantReason: "order", Source: "order"}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: inviteeB, PlanId: 8112, Status: "active", StartTime: start, EndTime: end, GrantReason: "order", Source: "order"}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: inviteeA, PlanId: 8112, Money: 80, TradeNo: "entitlement-a-order", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: start, CompleteTime: start}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: inviteeB, PlanId: 8112, Money: 80, TradeNo: "entitlement-b-order", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: start, CompleteTime: start}).Error)
	_, err := service.EnsureMonthlyInvitationEntitlement(inviterId, at)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/aff/entitlement", nil)
	ctx.Set("id", inviterId)

	GetInvitationEntitlement(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"direct_invite_count":2`)
	assert.Contains(t, body, `"qualified_active_count":2`)
	assert.Contains(t, body, `"entitled":true`)
	assert.Contains(t, body, `"reward_month":"`+at.Format("2006-01")+`"`)
}

func TestCompleteSubscriptionOrderTriggersInvitationEntitlement(t *testing.T) {
	originalDB := model.DB
	db := setupModelListControllerTestDB(t)
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.TopUp{}, &model.Log{}, &model.InvitationRewardEvent{}, &model.InvitationMonthlyEntitlement{}))

	inviterId := 8201
	inviteeA := 8202
	inviteeB := 8203
	require.NoError(t, model.DB.Create(&model.User{Id: inviterId, Username: "complete-inviter", Status: common.UserStatusEnabled, AffCode: "complete-inviter"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: inviteeA, Username: "complete-a", Status: common.UserStatusEnabled, AffCode: "complete-a", InviterId: inviterId}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: inviteeB, Username: "complete-b", Status: common.UserStatusEnabled, AffCode: "complete-b", InviterId: inviterId}).Error)
	basicCode := "basic_monthly"
	paidCode := "standard_monthly"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 8211, Title: "Basic", Enabled: true, PriceAmount: 40, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1_000, ConcurrencyLimit: 1, RewardEligible: true, BusinessCode: &basicCode}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 8212, Title: "Standard", Enabled: true, PriceAmount: 80, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, RewardEligible: true, BusinessCode: &paidCode}).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: inviteeA, PlanId: 8212, Status: "active", StartTime: now - 60, EndTime: now + 3600, GrantReason: "order", Source: "order"}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: inviteeA, PlanId: 8212, Money: 80, TradeNo: "complete-a-order", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: now - 60, CompleteTime: now - 60}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: inviteeB, PlanId: 8212, Money: 80, TradeNo: "sub-entitlement-complete", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: now}).Error)

	require.NoError(t, completeSubscriptionOrderAndEvaluateInvitation("sub-entitlement-complete", `{}`, model.PaymentProviderEpay, "alipay"))

	status := model.GetSubscriptionOrderByTradeNo("sub-entitlement-complete")
	require.NotNil(t, status)
	assert.Equal(t, common.TopUpStatusSuccess, status.Status)
	var entitlement model.InvitationMonthlyEntitlement
	require.NoError(t, model.DB.Where("inviter_id = ?", inviterId).First(&entitlement).Error)
	assert.Equal(t, model.InvitationEntitlementStatusQualified, entitlement.Status)
	assert.NotZero(t, entitlement.RewardSubscriptionId)
}

func TestCompleteSubscriptionOrderRetriesInvitationRewardHandlerForSuccessfulOrder(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	db := setupModelListControllerTestDB(t)
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.TopUp{}, &model.Log{}, &model.InvitationRewardEvent{}, &model.InvitationMonthlyEntitlement{}))

	inviterID := 8261
	inviteeID := 8262
	rewardCode := "retry_reward_handler_plan"
	inviter := model.User{Id: inviterID, Username: "retry-handler-inviter", Status: common.UserStatusEnabled, AffCode: "retry-handler-inviter", InvitationRewardMode: model.InvitationRewardModeSubscription}
	invitee := model.User{Id: inviteeID, Username: "retry-handler-invitee", Status: common.UserStatusEnabled, AffCode: "retry-handler-invitee", InviterId: inviterID}
	require.NoError(t, model.DB.Create(&inviter).Error)
	require.NoError(t, model.DB.Create(&invitee).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 8263, Title: "Retry Handler Plan", Enabled: true, PriceAmount: 80, Currency: "CNY", DurationUnit: model.SubscriptionDurationDay, DurationValue: 30, MonthlyTokenLimit: 1_000, ConcurrencyLimit: 1, RewardEligible: true, BusinessCode: &rewardCode}).Error)
	order := model.SubscriptionOrder{UserId: inviteeID, PlanId: 8263, Money: 80, AmountCents: 8000, Currency: "CNY", TradeNo: "sub-retry-handler-order", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(&order).Error)

	calledOrderIDs := make([]int, 0, 2)
	SetInvitationRewardOrderHandlerForTest(t, func(orderId int) error {
		calledOrderIDs = append(calledOrderIDs, orderId)
		if len(calledOrderIDs) == 1 {
			return errors.New("injected invitation reward handler failure")
		}
		return nil
	})

	providerPayload := `{"money":"80.00","currency":"CNY"}`
	firstErr := completeSubscriptionOrderAndEvaluateInvitation(order.TradeNo, providerPayload, model.PaymentProviderEpay, "alipay")
	require.Error(t, firstErr)
	assert.Equal(t, []int{order.Id}, calledOrderIDs)
	var completedOrder model.SubscriptionOrder
	require.NoError(t, model.DB.First(&completedOrder, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, completedOrder.Status)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)

	retryErr := completeSubscriptionOrderAndEvaluateInvitation(order.TradeNo, providerPayload, model.PaymentProviderEpay, "alipay")
	require.NoError(t, retryErr)
	assert.Equal(t, []int{order.Id, order.Id}, calledOrderIDs)
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestDefaultInvitationRewardOrderHandlerUsesFormalService(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	db := setupModelListControllerTestDB(t)
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
	})
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.InvitationRewardEvent{}, &model.InvitationMonthlyEntitlement{}, &model.InvitationCommissionAccount{}, &model.InvitationCommissionRecord{}, &model.InvitationCommissionLedger{}, &model.InvitationCommissionWithdrawal{}))
	setting := operation_setting.GetInvitationCommissionSetting()
	oldSetting := *setting
	*setting = operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000}
	t.Cleanup(func() { *setting = oldSetting })

	inviterID := 8271
	inviteeID := 8272
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "default-order-commission-inviter", Status: common.UserStatusEnabled, AffCode: "default-order-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: inviteeID, Username: "default-order-commission-invitee", Status: common.UserStatusEnabled, AffCode: "default-order-invitee", InviterId: inviterID}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 8273, Title: "Default Order Commission", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 30, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 8274, UserId: inviteeID, PlanId: 8273, Status: "active", StartTime: now - 60, EndTime: now + 86400, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	order := model.SubscriptionOrder{Id: 8275, UserId: inviteeID, PlanId: 8273, Money: 100, AmountCents: 10000, Currency: "CNY", TradeNo: "default-order-formal-service", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CreateTime: now - 60, CompleteTime: now - 30}
	require.NoError(t, model.DB.Create(&order).Error)
	event := model.InvitationRewardEvent{InviterId: inviterID, InviteeId: inviteeID, SourceType: model.InvitationRewardEventSourceSubscriptionOrder, SourceId: order.Id, SourceOrderId: order.Id, SourceSubscriptionId: 8274, SourceAmountCents: 10000, SourceCurrency: "CNY", EventStartTime: now - 60, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.DB.Create(&event).Error)

	require.NoError(t, handleInvitationRewardForCompletedSubscriptionOrder(order.Id))

	var record model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
	assert.Equal(t, int64(1000), record.CommissionCents)
	var entitlementCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", inviterID).Count(&entitlementCount).Error)
	assert.Equal(t, int64(0), entitlementCount)
}
