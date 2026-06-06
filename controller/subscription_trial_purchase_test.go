package controller

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
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
	require.NoError(t, db.AutoMigrate(
		&model.SubscriptionPlan{},
		&model.SubscriptionOrder{},
		&model.User{},
		&model.UserSubscription{},
		&model.InvitationRewardEvent{},
		&model.InvitationCommissionRecord{},
	))
	require.NoError(t, model.DB.Create(&model.User{Id: 8801, Username: "buyer", Status: common.UserStatusEnabled}).Error)
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "Alipay"}}
	t.Cleanup(func() {
		setting.StripeApiSecret = ""
		setting.StripeWebhookSecret = ""
		setting.CreemWebhookSecret = ""
		setting.CreemTestMode = false
		operation_setting.PayMethods = nil
		operation_setting.PayAddress = ""
		operation_setting.EpayId = ""
		operation_setting.EpayKey = ""
		operation_setting.CustomCallbackAddress = ""
	})
}

func seedSubscriptionPurchasePlan(t *testing.T, id int, trial bool, visible bool, price float64) {
	t.Helper()
	code := "plan_purchase_" + string(rune('a'+id%26))
	plan := &model.SubscriptionPlan{Id: id, Title: "Purchase Plan", Enabled: true, PublicVisible: visible, IsTrial: trial, PriceAmount: price, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, RewardEligible: true, BusinessCode: &code, StripePriceId: "price_test", CreemProductId: "prod_test"}
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

func signedEpaySubscriptionCallbackForSnapshotTest(t *testing.T, tradeNo string, money string) url.Values {
	t.Helper()
	params := map[string]string{
		"pid":          operation_setting.EpayId,
		"type":         "alipay",
		"out_trade_no": tradeNo,
		"trade_status": "TRADE_SUCCESS",
		"trade_no":     "epay_" + tradeNo,
		"name":         "subscription",
		"money":        money,
	}
	signed := signEpaySubscriptionCallbackForSnapshotTest(params)
	form := url.Values{}
	for key, value := range signed {
		form.Set(key, value)
	}
	return form
}

func signEpaySubscriptionCallbackForSnapshotTest(params map[string]string) map[string]string {
	key := operation_setting.EpayKey
	if client := GetEpayClient(); client != nil {
		key = client.Config.Key
	}
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	sum := md5.Sum([]byte(b.String() + key))
	params["sign"] = hex.EncodeToString(sum[:])
	params["sign_type"] = "MD5"
	return params
}

func performEpaySubscriptionCallbackForSnapshotTest(t *testing.T, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/epay/notify", strings.NewReader(form.Encode()))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	SubscriptionEpayNotify(ctx)
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

func TestSubscriptionStripeRejectsRenewalWhenPurchaseLimitReached(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8851, false, true, 40)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 8851).Update("max_purchase_per_user", 1).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: 8801, PlanId: 8851, Status: "active", StartTime: common.GetTimestamp() - 10, EndTime: common.GetTimestamp() + 3600, GrantReason: "order", Source: "order"}).Error)

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":8851}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "已达到该套餐购买上限")
}

func TestSubscriptionCreemRejectsRenewalWhenPurchaseLimitReached(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8861, false, true, 40)
	setting.CreemWebhookSecret = "creem_secret"
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 8861).Update("max_purchase_per_user", 1).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: 8801, PlanId: 8861, Status: "active", StartTime: common.GetTimestamp() - 10, EndTime: common.GetTimestamp() + 3600, GrantReason: "order", Source: "order"}).Error)

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":8861}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "已达到该套餐购买上限")
}

func TestSubscriptionEpayRejectsRenewalWhenPurchaseLimitReached(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 8871, false, true, 40)
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 8871).Update("max_purchase_per_user", 1).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: 8801, PlanId: 8871, Status: "active", StartTime: common.GetTimestamp() - 10, EndTime: common.GetTimestamp() + 3600, GrantReason: "order", Source: "order"}).Error)

	recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":8871,"payment_method":"alipay"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "已达到该套餐购买上限")
}

func TestSubscriptionEpayStoresSubmittedCNYAmountSnapshot(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9563, false, true, 40)
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	operation_setting.CustomCallbackAddress = "https://callback.example.com"

	recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9563,"payment_method":"alipay"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9563).First(&order).Error)
	assert.Equal(t, int64(4000), order.AmountCents)
	assert.Equal(t, "CNY", order.Currency)
}

func TestSubscriptionEpayRejectsAmountMismatch(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9569, false, true, 40)
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	operation_setting.CustomCallbackAddress = "https://callback.example.com"

	recorder := performSubscriptionJSON(SubscriptionRequestEpay, `{"plan_id":9569,"payment_method":"alipay"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9569).First(&order).Error)
	require.Equal(t, int64(4000), order.AmountCents)
	require.Equal(t, "CNY", order.Currency)

	callback := signedEpaySubscriptionCallbackForSnapshotTest(t, order.TradeNo, "39.99")
	callbackRecorder := performEpaySubscriptionCallbackForSnapshotTest(t, callback)

	require.Equal(t, http.StatusOK, callbackRecorder.Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.NotEqual(t, common.TopUpStatusSuccess, order.Status)
	var events int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&events).Error)
	assert.Equal(t, int64(0), events)
	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", order.Id).Count(&records).Error)
	assert.Equal(t, int64(0), records)
}
