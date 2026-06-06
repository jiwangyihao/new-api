package controller

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

func TestStripeSubscriptionOrderStoresEmptySnapshotWhenCheckoutAmountNotVerified(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9564, false, true, 40)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/checkout", AmountCents: 0, Currency: ""}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9564}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9564).First(&order).Error)
	assert.Equal(t, int64(0), order.AmountCents)
	assert.Equal(t, "", order.Currency)
}

func TestStripeSubscriptionOrderStoresCheckoutAmountSnapshotWhenVerified(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9568, false, true, 40)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/checkout", AmountCents: 4000, Currency: "CNY"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9568}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9568).First(&order).Error)
	assert.Equal(t, int64(4000), order.AmountCents)
	assert.Equal(t, "CNY", order.Currency)
}

func TestStripeSubscriptionCompletionRejectsAmountCurrencyMismatch(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9565, false, true, 40)

	for _, tc := range []struct {
		name        string
		amountTotal string
		currency    string
	}{
		{name: "amount", amountTotal: "4100", currency: "CNY"},
		{name: "currency", amountTotal: "4000", currency: "USD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tradeNo := "stripe-snapshot-mismatch-" + tc.name
			require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
				UserId:          8801,
				PlanId:          9565,
				Money:           40,
				AmountCents:     4000,
				Currency:        "CNY",
				TradeNo:         tradeNo,
				PaymentProvider: model.PaymentProviderStripe,
				PaymentMethod:   model.PaymentMethodStripe,
				Status:          common.TopUpStatusPending,
				CreateTime:      common.GetTimestamp(),
			}).Error)

			err := completeSubscriptionOrderAndEvaluateInvitation(tradeNo, `{"amount_total":"`+tc.amountTotal+`","currency":"`+tc.currency+`"}`, model.PaymentProviderStripe, model.PaymentMethodStripe)

			require.Error(t, err)
			var order model.SubscriptionOrder
			require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
			var events int64
			require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&events).Error)
			assert.Equal(t, int64(0), events)
			var records int64
			require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", order.Id).Count(&records).Error)
			assert.Equal(t, int64(0), records)
		})
	}
}

func stripeCheckoutCompletedEventForSubscriptionTest(tradeNo string, amountCents int64, currency string) stripe.Event {
	return stripe.Event{Type: stripe.EventTypeCheckoutSessionCompleted, Data: &stripe.EventData{Object: map[string]any{"customer": "cus_" + tradeNo, "client_reference_id": tradeNo, "status": "complete", "payment_status": "paid", "amount_total": amountCents, "currency": currency}}}
}

func signedStripeWebhookRecorderForSubscriptionTest(t *testing.T, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewReader(payload))
	now := time.Now()
	signature := webhook.ComputeSignature(now, payload, setting.StripeWebhookSecret)
	ctx.Request.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(signature)))
	StripeWebhook(ctx)
	return recorder
}

func stripeCheckoutCompletedPayloadForSubscriptionTest(t *testing.T, tradeNo string, amountCents int64, currency string) []byte {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"id":   "evt_" + tradeNo,
		"type": string(stripe.EventTypeCheckoutSessionCompleted),
		"data": map[string]any{"object": map[string]any{"customer": "cus_" + tradeNo, "client_reference_id": tradeNo, "status": "complete", "payment_status": "paid", "amount_total": amountCents, "currency": currency}},
	})
	require.NoError(t, err)
	return payload
}

func TestStripeSubscriptionWebhookAcksAmountCurrencyMismatchWithoutRetry(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9576, false, true, 40)
	order := model.SubscriptionOrder{UserId: 8801, PlanId: 9576, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: "stripe-mismatch-ack", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(&order).Error)
	event := stripeCheckoutCompletedEventForSubscriptionTest(order.TradeNo, 4100, "cny")

	err := sessionCompleted(t.Context(), event, "127.0.0.1")

	require.NoError(t, err)
	var saved model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&saved).Error)
	assert.Equal(t, common.TopUpStatusPending, saved.Status)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", saved.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(0), eventCount)
	var recordCount int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", saved.Id).Count(&recordCount).Error)
	assert.Equal(t, int64(0), recordCount)
}

func TestStripeSubscriptionWebhookAcksPaymentProviderMismatchWithoutRetry(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TopUp{}))
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	seedSubscriptionPurchasePlan(t, 9577, false, true, 40)
	order := model.SubscriptionOrder{UserId: 8801, PlanId: 9577, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: "stripe-provider-mismatch-ack", PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(&order).Error)
	payload := stripeCheckoutCompletedPayloadForSubscriptionTest(t, order.TradeNo, 4000, "cny")

	recorder := signedStripeWebhookRecorderForSubscriptionTest(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var saved model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&saved).Error)
	assert.Equal(t, common.TopUpStatusPending, saved.Status)
	var events int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", saved.Id).Count(&events).Error)
	assert.Equal(t, int64(0), events)
	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", saved.Id).Count(&records).Error)
	assert.Equal(t, int64(0), records)
	var topups int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ? AND status = ?", order.TradeNo, common.TopUpStatusSuccess).Count(&topups).Error)
	assert.Equal(t, int64(0), topups)
}

func TestStripeSubscriptionWebhookAcksInvalidStatusWithoutRetry(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TopUp{}))
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	seedSubscriptionPurchasePlan(t, 9578, false, true, 40)
	order := model.SubscriptionOrder{UserId: 8801, PlanId: 9578, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: "stripe-invalid-status-ack", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusExpired, CreateTime: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(&order).Error)
	payload := stripeCheckoutCompletedPayloadForSubscriptionTest(t, order.TradeNo, 4000, "cny")

	recorder := signedStripeWebhookRecorderForSubscriptionTest(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var saved model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&saved).Error)
	assert.Equal(t, common.TopUpStatusExpired, saved.Status)
	var events int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", saved.Id).Count(&events).Error)
	assert.Equal(t, int64(0), events)
	var records int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", saved.Id).Count(&records).Error)
	assert.Equal(t, int64(0), records)
	var topups int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ? AND status = ?", order.TradeNo, common.TopUpStatusSuccess).Count(&topups).Error)
	assert.Equal(t, int64(0), topups)
}

func TestStripeSubscriptionWebhookDoesNotFallbackToTopUpForPermanentErrors(t *testing.T) {
	for _, tc := range []struct {
		name               string
		tradeNo            string
		orderProvider      string
		orderMethod        string
		orderStatus        string
		webhookAmountCents int64
		webhookCurrency    string
	}{
		{name: "amount_currency_mismatch", tradeNo: "stripe-permanent-no-topup-amount", orderProvider: model.PaymentProviderStripe, orderMethod: model.PaymentMethodStripe, orderStatus: common.TopUpStatusPending, webhookAmountCents: 4100, webhookCurrency: "cny"},
		{name: "payment_provider_mismatch", tradeNo: "stripe-permanent-no-topup-provider", orderProvider: model.PaymentProviderEpay, orderMethod: "alipay", orderStatus: common.TopUpStatusPending, webhookAmountCents: 4000, webhookCurrency: "cny"},
		{name: "invalid_status", tradeNo: "stripe-permanent-no-topup-status", orderProvider: model.PaymentProviderStripe, orderMethod: model.PaymentMethodStripe, orderStatus: common.TopUpStatusExpired, webhookAmountCents: 4000, webhookCurrency: "cny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupSubscriptionTrialPurchaseTest(t)
			require.NoError(t, model.DB.AutoMigrate(&model.TopUp{}))
			setting.StripeApiSecret = "sk_test_123"
			setting.StripeWebhookSecret = "whsec_test"
			seedSubscriptionPurchasePlan(t, 9581, false, true, 40)
			order := model.SubscriptionOrder{UserId: 8801, PlanId: 9581, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: tc.tradeNo, PaymentProvider: tc.orderProvider, PaymentMethod: tc.orderMethod, Status: tc.orderStatus, CreateTime: common.GetTimestamp()}
			require.NoError(t, model.DB.Create(&order).Error)
			topUp := model.TopUp{UserId: 8801, Amount: 4000, AmountUnit: model.TopUpAmountUnitAccountBalanceCents, Money: 40, TradeNo: tc.tradeNo, PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
			require.NoError(t, model.DB.Create(&topUp).Error)
			payload := stripeCheckoutCompletedPayloadForSubscriptionTest(t, order.TradeNo, tc.webhookAmountCents, tc.webhookCurrency)

			recorder := signedStripeWebhookRecorderForSubscriptionTest(t, payload)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var savedOrder model.SubscriptionOrder
			require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&savedOrder).Error)
			assert.Equal(t, tc.orderStatus, savedOrder.Status)
			var savedTopUp model.TopUp
			require.NoError(t, model.DB.Where("trade_no = ?", tc.tradeNo).First(&savedTopUp).Error)
			assert.Equal(t, common.TopUpStatusPending, savedTopUp.Status)
			assert.Equal(t, int64(0), savedTopUp.CompleteTime)
			var user model.User
			require.NoError(t, model.DB.Select("quota", "stripe_customer").Where("id = ?", 8801).First(&user).Error)
			assert.Equal(t, 0, user.Quota)
			assert.Equal(t, "", user.StripeCustomer)
			var events int64
			require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", savedOrder.Id).Count(&events).Error)
			assert.Equal(t, int64(0), events)
			var records int64
			require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", savedOrder.Id).Count(&records).Error)
			assert.Equal(t, int64(0), records)
		})
	}
}

func TestStripeSubscriptionWebhookPropagatesInvitationRewardHandlerFailureOverHTTP(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TopUp{}))
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	seedSubscriptionPurchasePlan(t, 9579, false, true, 40)
	inviter := model.User{Id: 9580, Username: "stripe-http-handler-inviter", Status: common.UserStatusEnabled, AffCode: "stripe-http-handler-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}
	require.NoError(t, model.DB.Create(&inviter).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 8801).Update("inviter_id", inviter.Id).Error)
	order := model.SubscriptionOrder{UserId: 8801, PlanId: 9579, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: "stripe-handler-failure-http", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(&order).Error)
	SetInvitationRewardOrderHandlerForTest(t, func(orderId int) error { return errors.New("handler unavailable") })
	payload := stripeCheckoutCompletedPayloadForSubscriptionTest(t, order.TradeNo, 4000, "cny")

	recorder := signedStripeWebhookRecorderForSubscriptionTest(t, payload)

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	var saved model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&saved).Error)
	assert.Equal(t, common.TopUpStatusSuccess, saved.Status)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", saved.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestStripeSubscriptionWebhookPropagatesInvitationRewardHandlerFailure(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TopUp{}))
	seedSubscriptionPurchasePlan(t, 9574, false, true, 40)
	inviter := model.User{Id: 9575, Username: "stripe-handler-inviter", Status: common.UserStatusEnabled, AffCode: "stripe-handler-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}
	require.NoError(t, model.DB.Create(&inviter).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 8801).Update("inviter_id", inviter.Id).Error)
	order := model.SubscriptionOrder{UserId: 8801, PlanId: 9574, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: "stripe-handler-failure", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(&order).Error)
	SetInvitationRewardOrderHandlerForTest(t, func(orderId int) error { return errors.New("handler unavailable") })
	event := stripeCheckoutCompletedEventForSubscriptionTest(order.TradeNo, 4000, "cny")

	err := sessionCompleted(t.Context(), event, "127.0.0.1")

	require.Error(t, err)
	var saved model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&saved).Error)
	assert.Equal(t, common.TopUpStatusSuccess, saved.Status)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", saved.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}
