package controller

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestSubscriptionStripeRequiresExplicitPurchaseMode(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9584, false, true, 40)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/checkout"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9584}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "购买模式必须明确选择")
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", 8801, 9584).Count(&count).Error)
	assert.Zero(t, count)
}

func TestStripeSubscriptionOrderStoresResolvedPriceSnapshotBeforeCheckout(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9564, false, true, 40)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/checkout"}, nil
	})
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9564,"purchase_mode":"timed"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9564).First(&order).Error)
	assert.Equal(t, int64(4000), order.AmountCents)
	assert.Equal(t, "CNY", order.Currency)
	var snapshot model.SubscriptionEntitlementSnapshot
	require.NoError(t, common.UnmarshalJsonStr(order.EntitlementSnapshot, &snapshot))
	assert.Equal(t, model.SubscriptionPurchaseModeTimed, snapshot.PurchaseMode)
	assert.Equal(t, model.PaymentProviderStripe, snapshot.PaymentProvider)
	assert.Equal(t, "price_test", snapshot.ProviderProductID)
	assert.Equal(t, int64(4000), snapshot.PaymentAmountCents)
	assert.Equal(t, "CNY", snapshot.PaymentCurrency)
}

func TestStripeSubscriptionCheckoutParamsBindPriceIdentity(t *testing.T) {
	params := stripeSubscriptionCheckoutParams("sub_ref_test", "", "buyer@example.com", "price_bound")

	assert.Equal(t, "sub_ref_test", *params.ClientReferenceID)
	assert.Equal(t, "price_bound", params.Metadata[stripeSubscriptionProductMetadataKey])
	require.Len(t, params.LineItems, 1)
	assert.Equal(t, "price_bound", *params.LineItems[0].Price)
}

func TestStripeSubscriptionOrderStoresCheckoutAmountSnapshotWhenVerified(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9568, false, true, 40)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/checkout"}, nil
	})
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9568,"purchase_mode":"timed"}`)

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

func TestStripeSubscriptionCompletionRejectsProductMismatch(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9582, false, true, 40)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&plan, 9582).Error)
	snapshot := model.NewSubscriptionEntitlementSnapshot(&plan, model.SubscriptionPurchaseModeTimed, 0)
	snapshot.SetPaymentSnapshot(model.PaymentProviderStripe, "price_expected", model.PaymentMethodStripe, 4000, "CNY")
	serialized, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := model.SubscriptionOrder{
		UserId:              8801,
		PlanId:              plan.Id,
		Money:               40,
		AmountCents:         4000,
		Currency:            "CNY",
		TradeNo:             "stripe-product-mismatch",
		PaymentProvider:     model.PaymentProviderStripe,
		PaymentMethod:       model.PaymentMethodStripe,
		Status:              common.TopUpStatusPending,
		CreateTime:          common.GetTimestamp(),
		EntitlementSnapshot: serialized,
	}
	require.NoError(t, model.DB.Create(&order).Error)

	err = completeSubscriptionOrderAndEvaluateInvitation(order.TradeNo, `{"amount_total":4000,"currency":"CNY","provider_product_id":"price_wrong"}`, model.PaymentProviderStripe, model.PaymentMethodStripe)

	require.ErrorIs(t, err, errSubscriptionOrderAmountSnapshotMismatch)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}

func TestStripeSubscriptionCompletionRejectsMissingProductSnapshot(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9580, false, true, 40)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&plan, 9580).Error)
	tests := []struct {
		name     string
		snapshot string
	}{
		{name: "missing entitlement snapshot"},
	}
	snapshot := model.NewSubscriptionEntitlementSnapshot(&plan, model.SubscriptionPurchaseModeTimed, 0)
	snapshot.SetPaymentSnapshot(model.PaymentProviderStripe, "", model.PaymentMethodStripe, 4000, "CNY")
	serialized, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	tests = append(tests, struct {
		name     string
		snapshot string
	}{name: "missing provider product id", snapshot: serialized})

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			order := model.SubscriptionOrder{UserId: 8801, PlanId: plan.Id, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: fmt.Sprintf("stripe-missing-product-snapshot-%d", index), PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(), EntitlementSnapshot: test.snapshot}
			require.NoError(t, model.DB.Create(&order).Error)

			err := completeSubscriptionOrderAndEvaluateInvitation(order.TradeNo, `{"amount_total":4000,"currency":"CNY","provider_product_id":"price_test"}`, model.PaymentProviderStripe, model.PaymentMethodStripe)

			require.ErrorIs(t, err, errSubscriptionOrderAmountSnapshotMismatch)
			require.NoError(t, model.DB.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
		})
	}
}

func TestCompletedSubscriptionOrderReplayStillValidatesAmountAndCurrency(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedAuthoritativeSubscriptionPurchasePlan(t, 9583, true, 40_000_000)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&plan, 9583).Error)
	entitlementSnapshot := model.NewSubscriptionEntitlementSnapshotFromPlan(&plan)
	entitlementSnapshot.SetPaymentSnapshot(model.PaymentProviderStripe, "price_test", model.PaymentMethodStripe, 4000, "CNY")
	snapshot, err := model.MarshalSubscriptionEntitlementSnapshot(entitlementSnapshot)
	require.NoError(t, err)
	order := model.SubscriptionOrder{
		UserId:              8801,
		PlanId:              9583,
		Money:               40,
		AmountCents:         4000,
		Currency:            "CNY",
		TradeNo:             "stripe-success-replay-amount-validation",
		PaymentProvider:     model.PaymentProviderStripe,
		PaymentMethod:       model.PaymentMethodStripe,
		Status:              common.TopUpStatusPending,
		CreateTime:          common.GetTimestamp(),
		EntitlementSnapshot: snapshot,
	}
	require.NoError(t, model.DB.Create(&order).Error)
	require.NoError(t, completeSubscriptionOrderAndEvaluateInvitation(order.TradeNo, `{"amount_total":4000,"currency":"CNY","provider_product_id":"price_test"}`, model.PaymentProviderStripe, model.PaymentMethodStripe))

	err = completeSubscriptionOrderAndEvaluateInvitation(order.TradeNo, `{"amount_total":4100,"currency":"USD","provider_product_id":"price_test"}`, model.PaymentProviderStripe, model.PaymentMethodStripe)

	require.ErrorIs(t, err, errSubscriptionOrderAmountSnapshotMismatch)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
}

func stripeCheckoutCompletedEventForSubscriptionTest(tradeNo string, amountCents int64, currency string) stripe.Event {
	return stripe.Event{Type: stripe.EventTypeCheckoutSessionCompleted, Data: &stripe.EventData{Object: map[string]any{"customer": "cus_" + tradeNo, "client_reference_id": tradeNo, "status": "complete", "payment_status": "paid", "amount_total": amountCents, "currency": currency, "metadata": map[string]any{stripeSubscriptionProductMetadataKey: "price_test"}}}}
}

func signedStripeWebhookRecorderForSubscriptionTest(t *testing.T, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	return signedStripeWebhookRecorderForSubscriptionPayload(payload, setting.StripeWebhookSecret)
}

func signedStripeWebhookRecorderForSubscriptionPayload(payload []byte, secret string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewReader(payload))
	now := time.Now()
	signature := webhook.ComputeSignature(now, payload, secret)
	ctx.Request.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(signature)))
	StripeWebhook(ctx)
	return recorder
}

func stripeCheckoutCompletedPayloadForSubscriptionTest(t *testing.T, tradeNo string, amountCents int64, currency string) []byte {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"id":   "evt_" + tradeNo,
		"type": string(stripe.EventTypeCheckoutSessionCompleted),
		"data": map[string]any{"object": map[string]any{"customer": "cus_" + tradeNo, "client_reference_id": tradeNo, "status": "complete", "payment_status": "paid", "amount_total": amountCents, "currency": currency, "metadata": map[string]any{stripeSubscriptionProductMetadataKey: "price_test"}}},
	})
	require.NoError(t, err)
	return payload
}

func stripeCheckoutCompletedPayloadWithEventIDForSubscriptionTest(t *testing.T, eventID string, tradeNo string, amountCents int64, currency string) []byte {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"id":   eventID,
		"type": string(stripe.EventTypeCheckoutSessionCompleted),
		"data": map[string]any{"object": map[string]any{"customer": "cus_" + tradeNo, "client_reference_id": tradeNo, "status": "complete", "payment_status": "paid", "amount_total": amountCents, "currency": currency, "metadata": map[string]any{stripeSubscriptionProductMetadataKey: "price_test"}}},
	})
	require.NoError(t, err)
	return payload
}

func stripeWebhookPayloadForSubscriptionTest(t *testing.T, eventID string, eventType stripe.EventType, object map[string]any) []byte {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"id":   eventID,
		"type": string(eventType),
		"data": map[string]any{"object": object},
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
	seedAuthoritativeSubscriptionPurchasePlan(t, 9579, true, 40_000_000)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&plan, 9579).Error)
	entitlementSnapshot := model.NewSubscriptionEntitlementSnapshot(&plan, model.SubscriptionPurchaseModeTimed, 0)
	entitlementSnapshot.SetPaymentSnapshot(model.PaymentProviderStripe, "price_test", model.PaymentMethodStripe, 4000, "CNY")
	snapshot, err := model.MarshalSubscriptionEntitlementSnapshot(entitlementSnapshot)
	require.NoError(t, err)
	inviter := model.User{Id: 9580, Username: "stripe-http-handler-inviter", Status: common.UserStatusEnabled, AffCode: "stripe-http-handler-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}
	require.NoError(t, model.DB.Create(&inviter).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 8801).Update("inviter_id", inviter.Id).Error)
	order := model.SubscriptionOrder{UserId: 8801, PlanId: 9579, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: "stripe-handler-failure-http", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(), EntitlementSnapshot: snapshot}
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
	seedAuthoritativeSubscriptionPurchasePlan(t, 9574, true, 40_000_000)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&plan, 9574).Error)
	entitlementSnapshot := model.NewSubscriptionEntitlementSnapshot(&plan, model.SubscriptionPurchaseModeTimed, 0)
	entitlementSnapshot.SetPaymentSnapshot(model.PaymentProviderStripe, "price_test", model.PaymentMethodStripe, 4000, "CNY")
	snapshot, err := model.MarshalSubscriptionEntitlementSnapshot(entitlementSnapshot)
	require.NoError(t, err)
	inviter := model.User{Id: 9575, Username: "stripe-handler-inviter", Status: common.UserStatusEnabled, AffCode: "stripe-handler-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}
	require.NoError(t, model.DB.Create(&inviter).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 8801).Update("inviter_id", inviter.Id).Error)
	order := model.SubscriptionOrder{UserId: 8801, PlanId: 9574, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: "stripe-handler-failure", PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(), EntitlementSnapshot: snapshot}
	require.NoError(t, model.DB.Create(&order).Error)
	SetInvitationRewardOrderHandlerForTest(t, func(orderId int) error { return errors.New("handler unavailable") })
	event := stripeCheckoutCompletedEventForSubscriptionTest(order.TradeNo, 4000, "cny")

	err = sessionCompleted(t.Context(), event, "127.0.0.1")

	require.Error(t, err)
	var saved model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&saved).Error)
	assert.Equal(t, common.TopUpStatusSuccess, saved.Status)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", saved.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestSubscriptionStripeCreditPurchasePersistsFullSnapshotBeforeCheckout(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9606, 9607)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	var observedOrder model.SubscriptionOrder
	var observedErr error
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		observedErr = model.DB.Where("trade_no = ?", referenceId).First(&observedOrder).Error
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/credit-checkout"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9606,"purchase_mode":"credit_balance"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"message":"success"`)
	require.NoError(t, observedErr, "pending order and entitlement snapshot must exist before Stripe checkout is requested")
	assert.Equal(t, common.TopUpStatusPending, observedOrder.Status)
	var snapshot model.SubscriptionEntitlementSnapshot
	require.NoError(t, common.UnmarshalJsonStr(observedOrder.EntitlementSnapshot, &snapshot))
	assert.Equal(t, model.SubscriptionPurchaseModeCreditBalance, snapshot.PurchaseMode)
	assert.Equal(t, 9606, snapshot.PlanID)
	assert.Equal(t, int64(1000), snapshot.MonthlyTokenLimit)
	assert.Equal(t, model.PaymentProviderStripe, snapshot.PaymentProvider)
	assert.Equal(t, "price_test", snapshot.ProviderProductID)
	assert.Equal(t, int64(4000), snapshot.PaymentAmountCents)
	assert.Equal(t, "CNY", snapshot.PaymentCurrency)
	assert.Equal(t, 9607, snapshot.TargetCreditBalancePlanID)
	assert.Equal(t, "Global Credit Balance", snapshot.TargetCreditBalanceTitle)
	assert.Equal(t, "gpt-4o", snapshot.TargetCreditBalanceModelLimits)
	assert.Equal(t, 7, snapshot.TargetCreditBalanceConcurrencyLimit)
	assert.Equal(t, 11, snapshot.TargetCreditBalanceQueueCapacity)
}

func TestSubscriptionStripePriceSnapshotFailureCreatesNoOrderOrCheckout(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9608, 9609)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	checkoutCalled := false
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) {
		return 0, "", errors.New("price unavailable")
	})
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		checkoutCalled = true
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/unexpected"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9608,"purchase_mode":"credit_balance"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Stripe 套餐价格快照无效")
	assert.False(t, checkoutCalled)
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("plan_id = ?", 9608).Count(&count).Error)
	assert.Zero(t, count)
}

func TestStripeCreditCheckoutFailureExpiresOrder(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9646, 9647)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{}, errors.New("checkout unavailable")
	})

	recorder := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9646,"purchase_mode":"credit_balance"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "拉起支付失败")
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9646).First(&order).Error)
	assert.Equal(t, common.TopUpStatusExpired, order.Status)
}

func TestStripeCreditWebhookFulfillsImmutableSnapshotWithoutInvitation(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	setupSubscriptionControllerRedis(t)
	seedExternalCreditPurchasePlans(t, 9610, 9611)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	inviter := model.User{Id: 9612, Username: "stripe-credit-inviter", Status: common.UserStatusEnabled, AffCode: "stripe-credit-inviter"}
	require.NoError(t, model.DB.Create(&inviter).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 8801).Update("inviter_id", inviter.Id).Error)
	var buyer model.User
	require.NoError(t, model.DB.First(&buyer, 8801).Error)
	seedUserCacheForSubscriptionControllerTest(t, buyer)
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/credit-checkout"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9610,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9610).First(&order).Error)
	var snapshot model.SubscriptionEntitlementSnapshot
	require.NoError(t, common.UnmarshalJsonStr(order.EntitlementSnapshot, &snapshot))
	assert.Equal(t, "Purchase Plan", snapshot.PlanTitle)
	assert.Equal(t, "gpt-4o", snapshot.TargetCreditBalanceModelLimits)
	assert.Equal(t, 7, snapshot.TargetCreditBalanceConcurrencyLimit)
	assert.Equal(t, 11, snapshot.TargetCreditBalanceQueueCapacity)
	assert.NotEmpty(t, snapshot.TargetCreditBalanceBusinessCode)
	assert.Equal(t, 5, snapshot.TargetCreditBalanceGPTAbuseWarningLimit)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9610).Updates(map[string]any{"title": "Changed", "monthly_token_limit": 9000, "enabled": false, "public_visible": false, "unlimited_purchase_enabled": false}).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9611).Updates(map[string]any{"enabled": false, "credit_balance_purchase_enabled": false, "model_limits": "changed", "concurrency_limit": 1, "queue_capacity": 1}).Error)
	require.NoError(t, model.DB.Delete(&model.SubscriptionPlan{}, 9610).Error)
	require.NoError(t, model.DB.Delete(&model.SubscriptionPlan{}, 9611).Error)
	model.InvalidateSubscriptionPlanCache(9610)
	model.InvalidateSubscriptionPlanCache(9611)

	recorder := signedStripeWebhookRecorderForSubscriptionTest(t, stripeCheckoutCompletedPayloadForSubscriptionTest(t, order.TradeNo, 4000, "cny"))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, 9611, balance.PlanId)
	assert.Equal(t, int64(1000), balance.TokenLimit)
	cachedBuyer, err := model.GetUserCache(8801)
	require.NoError(t, err)
	assert.Equal(t, model.SubscriptionPurchaseModeCreditBalance, cachedBuyer.GetSetting().LastSubscriptionPurchaseMode)
	assert.Equal(t, balance.Id, cachedBuyer.GetSetting().ActiveSubscriptionId)
	var ledger model.CreditBalanceLedger
	require.NoError(t, model.DB.Where("source_id = ?", order.Id).First(&ledger).Error)
	assert.Equal(t, int64(1000), ledger.GrossCredit)
	var invitations int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&invitations).Error)
	assert.Zero(t, invitations)
}

func TestStripeCreditWebhookRetriesAfterTransactionalFailure(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9637, 9638)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/retry"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9637,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9637).First(&order).Error)
	payload := stripeCheckoutCompletedPayloadWithEventIDForSubscriptionTest(t, "evt_transaction_retry", order.TradeNo, 4000, "cny")
	require.NoError(t, model.DB.Exec(`CREATE TRIGGER reject_external_credit_ledger BEFORE INSERT ON credit_balance_ledgers BEGIN SELECT RAISE(FAIL, 'injected ledger failure'); END`).Error)
	t.Cleanup(func() { _ = model.DB.Exec(`DROP TRIGGER IF EXISTS reject_external_credit_ledger`).Error })

	first := signedStripeWebhookRecorderForSubscriptionTest(t, payload)

	require.Equal(t, http.StatusInternalServerError, first.Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var count int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, model.DB.Exec(`DROP TRIGGER reject_external_credit_ledger`).Error)

	second := signedStripeWebhookRecorderForSubscriptionTest(t, payload)

	require.Equal(t, http.StatusOK, second.Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestStripeCreditWebhookConcurrentReplayFulfillsOnce(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9613, 9614)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/concurrent"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9613,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9613).First(&order).Error)
	payloads := [][]byte{
		stripeCheckoutCompletedPayloadWithEventIDForSubscriptionTest(t, "evt_concurrent_a", order.TradeNo, 4000, "cny"),
		stripeCheckoutCompletedPayloadWithEventIDForSubscriptionTest(t, "evt_concurrent_b", order.TradeNo, 4000, "cny"),
	}
	secret := setting.StripeWebhookSecret
	start := make(chan struct{})
	statuses := make(chan int, 2)
	var callbacks sync.WaitGroup
	for _, payload := range payloads {
		callbacks.Add(1)
		go func(body []byte) {
			defer callbacks.Done()
			<-start
			statuses <- signedStripeWebhookRecorderForSubscriptionPayload(body, secret).Code
		}(append([]byte(nil), payload...))
	}
	close(start)
	callbacks.Wait()
	close(statuses)
	for status := range statuses {
		assert.Equal(t, http.StatusOK, status)
	}
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(1000), balance.TokenLimit)
	var count int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementCreditBalance).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestStripeCreditAsyncPaymentFailureFinalizesOrderWithoutCredit(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9615, 9616)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/async"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9615,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9615).First(&order).Error)
	payload := stripeWebhookPayloadForSubscriptionTest(t, "evt_async_failed_credit", stripe.EventTypeCheckoutSessionAsyncPaymentFailed, map[string]any{
		"client_reference_id": order.TradeNo,
		"status":              "complete",
		"payment_status":      "unpaid",
		"amount_total":        4000,
		"currency":            "cny",
	})

	recorder := signedStripeWebhookRecorderForSubscriptionTest(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
	var count int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementCreditBalance).Count(&count).Error)
	assert.Zero(t, count)
}

func TestStripeCreditWebhookFailedThenSuccessDoesNotFulfill(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9617, 9618)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/out-of-order"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9617,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9617).First(&order).Error)
	failedPayload := stripeWebhookPayloadForSubscriptionTest(t, "evt_failed_first", stripe.EventTypeCheckoutSessionAsyncPaymentFailed, map[string]any{"client_reference_id": order.TradeNo, "payment_status": "unpaid"})
	require.Equal(t, http.StatusOK, signedStripeWebhookRecorderForSubscriptionTest(t, failedPayload).Code)

	success := signedStripeWebhookRecorderForSubscriptionTest(t, stripeCheckoutCompletedPayloadWithEventIDForSubscriptionTest(t, "evt_success_late", order.TradeNo, 4000, "cny"))

	require.Equal(t, http.StatusOK, success.Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
	var count int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestStripeCreditWebhookSuccessThenFailedPreservesSingleFulfillment(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9619, 9620)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionPriceSnapshotForTest(t, func(priceId string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/out-of-order"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9619,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9619).First(&order).Error)
	require.Equal(t, http.StatusOK, signedStripeWebhookRecorderForSubscriptionTest(t, stripeCheckoutCompletedPayloadWithEventIDForSubscriptionTest(t, "evt_success_first", order.TradeNo, 4000, "cny")).Code)
	failedPayload := stripeWebhookPayloadForSubscriptionTest(t, "evt_failed_late", stripe.EventTypeCheckoutSessionAsyncPaymentFailed, map[string]any{"client_reference_id": order.TradeNo, "payment_status": "unpaid"})

	failed := signedStripeWebhookRecorderForSubscriptionTest(t, failedPayload)

	require.Equal(t, http.StatusOK, failed.Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var count int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(1000), balance.TokenLimit)
}

func TestStripeCreditRefundBeforeCompletionRetriesThenRecoversOnce(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9648, 9649)
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	SetStripeSubscriptionPriceSnapshotForTest(t, func(string) (int64, string, error) { return 4000, "CNY", nil })
	SetStripeSubscriptionCheckoutForTest(t, func(string, string, string, string) (StripeSubscriptionCheckoutResult, error) {
		return StripeSubscriptionCheckoutResult{URL: "https://stripe.test/refund-ordering"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestStripePay, `{"plan_id":9648,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9648).First(&order).Error)
	SetStripeFinancialChargeIdentityForTest(t, func(chargeID string) (stripeFinancialChargeIdentity, error) {
		require.Equal(t, "ch_refund_early", chargeID)
		return stripeFinancialChargeIdentity{
			TradeNo: order.TradeNo,
			Identity: model.SubscriptionOrderProviderIdentity{
				TransactionID:  "pi_refund_early",
				InvoiceID:      "in_refund_early",
				SubscriptionID: "sub_refund_early",
			},
		}, nil
	})
	refundPayload := stripeWebhookPayloadForSubscriptionTest(t, "evt_refund_early", stripe.EventTypeRefundCreated, map[string]any{
		"id": "re_refund_early", "charge": "ch_refund_early", "payment_intent": "pi_refund_early", "status": "succeeded", "amount": 4000, "currency": "cny",
	})
	for _, status := range []string{"pending", "failed", "canceled"} {
		nonTerminalPayload := stripeWebhookPayloadForSubscriptionTest(t, "evt_refund_early_"+status, stripe.EventTypeRefundCreated, map[string]any{
			"id": "re_refund_early_" + status, "charge": "ch_refund_early", "payment_intent": "pi_refund_early", "status": status,
		})
		nonTerminal := signedStripeWebhookRecorderForSubscriptionTest(t, nonTerminalPayload)
		require.Equal(t, http.StatusOK, nonTerminal.Code)
		require.NoError(t, model.DB.First(&order, order.Id).Error)
		assert.Equal(t, common.TopUpStatusPending, order.Status)
	}

	early := signedStripeWebhookRecorderForSubscriptionTest(t, refundPayload)
	require.Equal(t, http.StatusInternalServerError, early.Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)

	completionPayload := stripeWebhookPayloadForSubscriptionTest(t, "evt_complete_after_refund", stripe.EventTypeCheckoutSessionCompleted, map[string]any{
		"customer": "cus_refund_early", "client_reference_id": order.TradeNo,
		"status": "complete", "payment_status": "paid", "amount_total": 4000, "currency": "cny",
		"payment_intent": "pi_refund_early", "invoice": "in_refund_early", "subscription": "sub_refund_early",
		"metadata": map[string]any{stripeSubscriptionProductMetadataKey: "price_test"},
	})
	require.Equal(t, http.StatusOK, signedStripeWebhookRecorderForSubscriptionTest(t, completionPayload).Code)

	retry := signedStripeWebhookRecorderForSubscriptionTest(t, refundPayload)
	require.Equal(t, http.StatusOK, retry.Code)
	replay := signedStripeWebhookRecorderForSubscriptionTest(t, refundPayload)
	require.Equal(t, http.StatusOK, replay.Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusRefunded, order.Status)
	assert.Equal(t, "pi_refund_early", order.ProviderTransactionID)
	assert.Equal(t, "in_refund_early", order.ProviderInvoiceID)
	assert.Equal(t, "sub_refund_early", order.ProviderSubscriptionID)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", order.UserId, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(1000), balance.TokenLimit)
	assert.Equal(t, int64(1000), balance.TokenUsed)
	var recoveryCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Count(&recoveryCount).Error)
	assert.Equal(t, int64(1), recoveryCount)
}
