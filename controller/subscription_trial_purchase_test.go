package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSubscriptionTrialPurchaseTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.User{}))
	require.NoError(t, model.DB.Create(&model.User{Id: 8801, Username: "buyer", Status: common.UserStatusEnabled}).Error)
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "Alipay"}}
	t.Cleanup(func() {
		setting.StripeApiSecret = ""
		setting.StripeWebhookSecret = ""
		setting.CreemWebhookSecret = ""
		setting.CreemTestMode = false
		operation_setting.PayMethods = nil
	})
}

func seedSubscriptionPurchasePlan(t *testing.T, id int, trial bool, visible bool, price float64) {
	t.Helper()
	code := "plan_purchase_" + string(rune('a'+id%26))
	plan := &model.SubscriptionPlan{Id: id, Title: "Purchase Plan", Enabled: true, PublicVisible: visible, IsTrial: trial, PriceAmount: price, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, RewardEligible: true, BusinessCode: &code, StripePriceId: "price_test", CreemProductId: "prod_test"}
	require.NoError(t, model.DB.Create(plan).Error)
	require.NoError(t, model.DB.Model(plan).Updates(map[string]interface{}{"is_trial": trial, "public_visible": visible}).Error)
}

func performSubscriptionJSON(handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/pay", bytes.NewBufferString(body))
	ctx.Set("id", 8801)
	handler(ctx)
	return recorder
}

func TestGetSubscriptionPlans_HidesTrialPlans(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8811, false, true, 40)
	seedSubscriptionPurchasePlan(t, 8812, true, false, 0)
	seedSubscriptionPurchasePlan(t, 8813, false, false, 40)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil)
	GetSubscriptionPlans(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"id":8811`)
	assert.NotContains(t, body, `"id":8812`)
	assert.NotContains(t, body, `"id":8813`)
}

func TestSubscriptionEpayRejectsTrialPlan(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8821, true, false, 0)

	recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":8821,"payment_method":"alipay"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "套餐不可购买")
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSubscriptionStripeRejectsTrialPlan(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8831, true, false, 0)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":8831}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "套餐不可购买")
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSubscriptionCreemRejectsTrialPlan(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8841, true, false, 0)
	setting.CreemWebhookSecret = "creem_secret"

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":8841}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "套餐不可购买")
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}
