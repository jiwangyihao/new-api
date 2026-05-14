package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
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
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.TopUp{}, &model.Log{}, &model.InvitationMonthlyEntitlement{}))

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
