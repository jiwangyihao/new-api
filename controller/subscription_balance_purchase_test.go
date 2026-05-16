package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSubscriptionBalancePurchaseTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	model.InvalidateSubscriptionPlanCache(9502)
	model.InvalidateSubscriptionPlanCache(9512)
	model.InvalidateSubscriptionPlanCache(9522)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.UserSubscription{}, &model.Log{}, &model.TopUp{}, &model.InvitationMonthlyEntitlement{}))
}

func performBalancePayRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/balance/pay", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	SubscriptionRequestBalance(ctx)
	return recorder
}

func TestSubscriptionBalancePayCreatesSubscriptionAndDeductsBalance(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9501
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_buyer", Quota: 100, Status: common.UserStatusEnabled}).Error)
	code := "balance-basic"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9502, Title: "Basic", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9502,"idempotency_key":"balance-key-1"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 60, user.Quota)
	var sub model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9502).First(&sub).Error)
	assert.Equal(t, "order", sub.GrantReason)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9502).First(&order).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, model.PaymentProviderBalance, order.PaymentProvider)
	assert.Equal(t, model.PaymentMethodAccountBalance, order.PaymentMethod)
	assert.Equal(t, 40.0, order.Money)
	assert.NotZero(t, order.CompleteTime)
	assert.Equal(t, "BALSUBUSR9501NO"+common.Sha1([]byte("balance-key-1")), order.TradeNo)

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeTopup).First(&log).Error)
	assert.Contains(t, log.Content, "账户余额购买订阅套餐：Basic")
}

func TestSubscriptionBalancePayInsufficientBalanceDoesNotDeduct(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9511
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_low", Quota: 10, Status: common.UserStatusEnabled}).Error)
	code := "balance-pro"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9512, Title: "Pro", PriceAmount: 160, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9512,"idempotency_key":"balance-low"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "余额不足")
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 10, user.Quota)
	var orderCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", userID, 9512).Count(&orderCount).Error)
	assert.Equal(t, int64(0), orderCount)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9512).Count(&subCount).Error)
	assert.Equal(t, int64(0), subCount)
}

func TestSubscriptionBalancePayExistingPendingDoesNotDeduct(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9531
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_pending", Quota: 100, Status: common.UserStatusEnabled}).Error)
	code := "balance-pending"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9532, Title: "Pending", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	tradeNo := "BALSUBUSR9531NO" + common.Sha1([]byte("pending-key"))
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{UserId: userID, PlanId: 9532, Money: 40, TradeNo: tradeNo, PaymentProvider: model.PaymentProviderBalance, PaymentMethod: model.PaymentMethodAccountBalance, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9532,"idempotency_key":"pending-key"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"pending"`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 100, user.Quota)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9532).Count(&subCount).Error)
	assert.Equal(t, int64(0), subCount)
}

func TestSubscriptionBalancePayIdempotent(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9521
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "balance_idem", Quota: 100, Status: common.UserStatusEnabled}).Error)
	code := "balance-standard"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9522, Title: "Standard", PriceAmount: 80, Currency: "CNY", Enabled: true, PublicVisible: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

	first := performBalancePayRequest(t, userID, `{"plan_id":9522,"idempotency_key":"idem-key"}`)
	second := performBalancePayRequest(t, userID, `{"plan_id":9522,"idempotency_key":"idem-key"}`)

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	assert.Contains(t, first.Body.String(), `"success":true`)
	assert.Contains(t, second.Body.String(), `"success":true`)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 20, user.Quota)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9522).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
	var orderCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", userID, 9522).Count(&orderCount).Error)
	assert.Equal(t, int64(1), orderCount)
}
