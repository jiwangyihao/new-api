package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionCreemRequiresExplicitPurchaseMode(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9621, false, true, 40)
	setting.CreemWebhookSecret = "creem_secret"
	checkoutCalled := false
	SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
		checkoutCalled = true
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/checkout", AmountCents: 4000, Currency: "CNY"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9621}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "购买模式必须明确选择")
	assert.False(t, checkoutCalled)
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", 8801, 9621).Count(&count).Error)
	assert.Zero(t, count)
}
func TestSubscriptionCreemProductSnapshotFailureCreatesNoOrderOrCheckout(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9632, 9633)
	setting.CreemWebhookSecret = "creem_secret"
	checkoutCalled := false
	SetCreemSubscriptionProductSnapshotForTest(t, func(ctx context.Context, productID string) (int64, string, error) {
		return 0, "", errors.New("product unavailable")
	})
	SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
		checkoutCalled = true
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/unexpected"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9632,"purchase_mode":"credit_balance"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "Creem 套餐价格快照无效")
	assert.False(t, checkoutCalled)
	var count int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("plan_id = ?", 9632).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSubscriptionCreemCreditPurchasePersistsSnapshotBeforeCheckout(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9622, 9623)
	setting.CreemWebhookSecret = "creem_secret"
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	var observedOrder model.SubscriptionOrder
	var orderLookupErr error
	SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
		orderLookupErr = model.DB.Where("trade_no = ?", referenceId).First(&observedOrder).Error
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/credit", AmountCents: 4000, Currency: "CNY"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9622,"purchase_mode":"credit_balance"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	require.NoError(t, orderLookupErr)
	assert.Equal(t, int64(4000), observedOrder.AmountCents)
	assert.Equal(t, "CNY", observedOrder.Currency)
	var snapshot model.SubscriptionEntitlementSnapshot
	require.NoError(t, common.UnmarshalJsonStr(observedOrder.EntitlementSnapshot, &snapshot))
	assert.Equal(t, model.SubscriptionPurchaseModeCreditBalance, snapshot.PurchaseMode)
	assert.Equal(t, 9623, snapshot.TargetCreditBalancePlanID)
	assert.Equal(t, model.PaymentProviderCreem, snapshot.PaymentProvider)
	assert.Equal(t, "prod_test", snapshot.ProviderProductID)
	assert.Equal(t, model.PaymentMethodCreem, snapshot.ProviderPaymentMethod)
	assert.Equal(t, int64(4000), snapshot.PaymentAmountCents)
	assert.Equal(t, "CNY", snapshot.PaymentCurrency)
	assert.Equal(t, "Global Credit Balance", snapshot.TargetCreditBalanceTitle)
}

func performSignedCreemSubscriptionWebhook(t *testing.T, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/creem/webhook", bytes.NewReader(payload))
	ctx.Request.Header.Set(CreemSignatureHeader, generateCreemSignature(string(payload), setting.CreemWebhookSecret))
	CreemWebhook(ctx)
	return recorder
}

func TestSubscriptionCreemCreditWebhookRejectsProductMismatch(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9634, 9635)
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemApiKey = "creem_api_key"
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/credit"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9634,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9634).First(&order).Error)
	payload, err := common.Marshal(map[string]any{
		"id":        "evt_creem_wrong_product",
		"eventType": "checkout.completed",
		"object": map[string]any{
			"request_id": order.TradeNo,
			"order":      map[string]any{"id": "ord_creem_wrong_product", "status": "paid", "amount_paid": 4000, "currency": "CNY"},
			"product":    map[string]any{"id": "prod_other"},
		},
	})
	require.NoError(t, err)

	recorder := performSignedCreemSubscriptionWebhook(t, payload)

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
}
func TestSubscriptionCreemCompletionRejectsMissingProductSnapshot(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9636, false, true, 40)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&plan, 9636).Error)

	tests := []struct {
		name     string
		snapshot string
	}{
		{name: "missing entitlement snapshot"},
	}
	snapshot := model.NewSubscriptionEntitlementSnapshot(&plan, model.SubscriptionPurchaseModeTimed, 0)
	snapshot.SetPaymentSnapshot(model.PaymentProviderCreem, "", model.PaymentMethodCreem, 4000, "CNY")
	serialized, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	tests = append(tests, struct {
		name     string
		snapshot string
	}{name: "missing provider product id", snapshot: serialized})

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			order := model.SubscriptionOrder{UserId: 8801, PlanId: plan.Id, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: fmt.Sprintf("creem-missing-product-snapshot-%d", index), PaymentProvider: model.PaymentProviderCreem, PaymentMethod: model.PaymentMethodCreem, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(), EntitlementSnapshot: test.snapshot}
			require.NoError(t, model.DB.Create(&order).Error)

			err := completeSubscriptionOrderAndEvaluateInvitation(order.TradeNo, `{"amount_total":4000,"currency":"CNY","object":{"product":{"id":"prod_test"}}}`, model.PaymentProviderCreem, model.PaymentMethodCreem)

			require.ErrorIs(t, err, errSubscriptionOrderAmountSnapshotMismatch)
			require.NoError(t, model.DB.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
		})
	}
}

func TestSubscriptionCreemCreditWebhookCompletesFromSnapshotWithoutInvitation(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9624, 9625)
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemApiKey = "creem_api_key"
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/credit", AmountCents: 4000, Currency: "CNY"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9624,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9624).First(&order).Error)
	payload, err := common.Marshal(map[string]any{
		"id":        "evt_creem_credit",
		"eventType": "checkout.completed",
		"object": map[string]any{
			"request_id": order.TradeNo,
			"order":      map[string]any{"id": "ord_creem_credit", "status": "paid", "amount_paid": 4000, "currency": "CNY"},
			"product":    map[string]any{"id": "prod_test"},
		},
	})
	require.NoError(t, err)

	recorder := performSignedCreemSubscriptionWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var ledger model.CreditBalanceLedger
	require.NoError(t, model.DB.Where("source_id = ?", order.Id).First(&ledger).Error)
	assert.Equal(t, int64(1000), ledger.GrossCredit)
	var count int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSubscriptionCreemCreditWebhookRejectsInvalidSignatureAndFinalizesExplicitTerminalStatuses(t *testing.T) {
	for index, test := range []struct {
		name           string
		status         string
		validSign      bool
		expectedStatus string
	}{
		{name: "invalid signature", status: "paid", expectedStatus: common.TopUpStatusPending},
		{name: "pending", status: "pending", validSign: true, expectedStatus: common.TopUpStatusPending},
		{name: "closed", status: "closed", validSign: true, expectedStatus: common.TopUpStatusFailed},
		{name: "failed", status: "failed", validSign: true, expectedStatus: common.TopUpStatusFailed},
		{name: "canceled", status: "canceled", validSign: true, expectedStatus: common.TopUpStatusFailed},
		{name: "expired", status: "expired", validSign: true, expectedStatus: common.TopUpStatusFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupSubscriptionTrialPurchaseTest(t)
			planID := 9640 + index*2
			seedExternalCreditPurchasePlans(t, planID, planID+1)
			setting.CreemWebhookSecret = "creem_secret"
			setting.CreemApiKey = "creem_api_key"
			SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
			SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
				return CreemSubscriptionCheckoutResult{URL: "https://creem.test/rejected"}, nil
			})
			create := performSubscriptionJSON(SubscriptionRequestCreemPay, fmt.Sprintf(`{"plan_id":%d,"purchase_mode":"credit_balance"}`, planID))
			require.Contains(t, create.Body.String(), `"message":"success"`)
			var order model.SubscriptionOrder
			require.NoError(t, model.DB.Where("plan_id = ?", planID).First(&order).Error)
			payload, err := common.Marshal(map[string]any{
				"id":        fmt.Sprintf("evt_creem_rejected_%d", index),
				"eventType": "checkout.completed",
				"object": map[string]any{
					"request_id": order.TradeNo,
					"order":      map[string]any{"id": "ord_creem_rejected", "status": test.status, "amount_paid": 4000, "currency": "CNY"},
					"product":    map[string]any{"id": "prod_test"},
				},
			})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/creem/webhook", bytes.NewReader(payload))
			signature := "invalid"
			if test.validSign {
				signature = generateCreemSignature(string(payload), setting.CreemWebhookSecret)
			}
			ctx.Request.Header.Set(CreemSignatureHeader, signature)

			CreemWebhook(ctx)

			if test.validSign {
				require.Equal(t, http.StatusOK, recorder.Code)
			} else {
				require.Equal(t, http.StatusUnauthorized, recorder.Code)
			}
			require.NoError(t, model.DB.First(&order, order.Id).Error)
			assert.Equal(t, test.expectedStatus, order.Status)
			var count int64
			require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestCreemNonPaidSubscriptionEventLeavesTopUpPending(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	setting.CreemWebhookSecret = "creem_secret"
	tradeNo := "creem-topup-non-paid"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:          8801,
		Amount:          1000,
		Money:           10,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodCreem,
		PaymentProvider: model.PaymentProviderCreem,
		Status:          common.TopUpStatusPending,
		CreateTime:      common.GetTimestamp(),
	}).Error)
	payload, err := common.Marshal(map[string]any{
		"id":        "evt_creem_topup_non_paid",
		"eventType": "checkout.completed",
		"object": map[string]any{
			"request_id": tradeNo,
			"order":      map[string]any{"id": "ord_creem_topup_non_paid", "status": "failed", "amount_paid": 1000, "currency": "CNY", "type": "onetime"},
		},
	})
	require.NoError(t, err)

	recorder := performSignedCreemSubscriptionWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code)
	var topUp model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&topUp).Error)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
}

func TestSubscriptionCreemCreditWebhookReplayFulfillsOnce(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9644, 9645)
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemApiKey = "creem_api_key"
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/replay"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9644,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9644).First(&order).Error)
	payload, err := common.Marshal(map[string]any{
		"id":        "evt_creem_replay",
		"eventType": "checkout.completed",
		"object": map[string]any{
			"request_id": order.TradeNo,
			"order":      map[string]any{"id": "ord_creem_replay", "status": "paid", "amount_paid": 4000, "currency": "CNY"},
			"product":    map[string]any{"id": "prod_test"},
		},
	})
	require.NoError(t, err)

	first := performSignedCreemSubscriptionWebhook(t, payload)
	second := performSignedCreemSubscriptionWebhook(t, payload)

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	var count int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_id = ?", order.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", 8801, model.SubscriptionEntitlementCreditBalance).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestCreemCreditCheckoutFailureExpiresOrder(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9648, 9649)
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemApiKey = "creem_api_key"
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
		return CreemSubscriptionCheckoutResult{}, errors.New("checkout unavailable")
	})

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9648,"purchase_mode":"credit_balance"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "拉起支付失败")
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9648).First(&order).Error)
	assert.Equal(t, common.TopUpStatusExpired, order.Status)
}

func TestCreemSubscriptionOrderStoresCheckoutAmountSnapshot(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9566, false, true, 40)
	setting.CreemWebhookSecret = "creem_secret"
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/checkout", AmountCents: 4000, Currency: "CNY"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9566,"purchase_mode":"timed"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9566).First(&order).Error)
	assert.Equal(t, int64(4000), order.AmountCents)
	assert.Equal(t, "CNY", order.Currency)
}

type creemCheckoutSuccessTransport struct {
	t    *testing.T
	hits int
}

func (transport *creemCheckoutSuccessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport.t.Helper()
	transport.hits++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"checkout_url":"https://creem.test/checkout","id":"chk_subscription"}`)),
		Request:    req,
	}, nil
}

func TestCreemSubscriptionOrderStoresPlanSnapshotWhenCheckoutDoesNotReturnAmount(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9579, false, true, 40)
	oldAPIKey := setting.CreemApiKey
	setting.CreemApiKey = "creem_api_key"
	t.Cleanup(func() { setting.CreemApiKey = oldAPIKey })
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemTestMode = true
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	fakeTransport := &creemCheckoutSuccessTransport{t: t}
	oldHTTPClient := creemHTTPClient
	creemHTTPClient = &http.Client{Transport: fakeTransport}
	t.Cleanup(func() { creemHTTPClient = oldHTTPClient })

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9579,"purchase_mode":"timed"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, fakeTransport.hits)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", 8801, 9579).First(&order).Error)
	assert.Equal(t, int64(4000), order.AmountCents)
	assert.Equal(t, "CNY", order.Currency)
}

func TestCreemSubscriptionCompletionRejectsAmountCurrencyMismatch(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9567, false, true, 40)

	for _, tc := range []struct {
		name        string
		amountTotal string
		currency    string
	}{
		{name: "amount", amountTotal: "4100", currency: "CNY"},
		{name: "currency", amountTotal: "4000", currency: "USD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tradeNo := "creem-snapshot-mismatch-" + tc.name
			require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
				UserId:          8801,
				PlanId:          9567,
				Money:           40,
				AmountCents:     4000,
				Currency:        "CNY",
				TradeNo:         tradeNo,
				PaymentProvider: model.PaymentProviderCreem,
				PaymentMethod:   model.PaymentMethodCreem,
				Status:          common.TopUpStatusPending,
				CreateTime:      common.GetTimestamp(),
			}).Error)

			err := completeSubscriptionOrderAndEvaluateInvitation(tradeNo, `{"amount_total":"`+tc.amountTotal+`","currency":"`+tc.currency+`"}`, model.PaymentProviderCreem, model.PaymentMethodCreem)

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

func TestCreemCreditRefundAndChargebackRecoverOnce(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9650, 9651)
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemApiKey = "creem_api_key"
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	SetCreemSubscriptionCheckoutForTest(t, func(string, *CreemProduct, string, string) (CreemSubscriptionCheckoutResult, error) {
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/recovery"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9650,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9650).First(&order).Error)
	completion, err := common.Marshal(map[string]any{
		"id": "evt_creem_recovery_paid", "eventType": "checkout.completed",
		"object": map[string]any{"request_id": order.TradeNo, "order": map[string]any{"id": "ord_creem_recovery", "transaction": "tran_creem_recovery", "status": "paid", "amount_paid": 4000, "currency": "CNY"}, "product": map[string]any{"id": "prod_test"}},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, performSignedCreemSubscriptionWebhook(t, completion).Code)
	refund, err := common.Marshal(map[string]any{
		"id": "evt_creem_recovery_refund", "eventType": "refund.created",
		"object": map[string]any{"request_id": order.TradeNo, "status": "succeeded", "refund_amount": 100, "refund_currency": "CNY", "transaction": map[string]any{"id": "tran_creem_recovery", "status": "refunded"}, "subscription": map[string]any{"id": "sub_creem_recovery", "status": "canceled"}},
	})
	require.NoError(t, err)
	for _, status := range []string{"pending", "failed", "canceled"} {
		nonTerminal, marshalErr := common.Marshal(map[string]any{
			"id": "evt_creem_recovery_" + status, "eventType": "refund.created",
			"object": map[string]any{"request_id": order.TradeNo, "status": status, "transaction": map[string]any{"id": "tran_creem_recovery", "status": status}},
		})
		require.NoError(t, marshalErr)
		require.Equal(t, http.StatusOK, performSignedCreemSubscriptionWebhook(t, nonTerminal).Code)
		require.NoError(t, model.DB.First(&order, order.Id).Error)
		assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	}

	require.Equal(t, http.StatusOK, performSignedCreemSubscriptionWebhook(t, refund).Code)
	dispute, err := common.Marshal(map[string]any{
		"id": "evt_creem_recovery_dispute", "eventType": "dispute.created",
		"object": map[string]any{"request_id": order.TradeNo, "amount": 50, "currency": "CNY", "transaction": map[string]any{"id": "tran_creem_recovery", "status": "chargeback"}, "subscription": map[string]any{"id": "sub_creem_recovery", "status": "active"}},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, performSignedCreemSubscriptionWebhook(t, dispute).Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusChargeback, order.Status)
	var count int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestCreemTopUpRefundAcknowledgesWithoutSubscriptionRecoveryRetry(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemApiKey = "creem_api_key"
	tradeNo := "creem-topup-refund-ack"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 9652, TradeNo: tradeNo, PaymentProvider: model.PaymentProviderCreem,
		PaymentMethod: model.PaymentMethodCreem, Status: common.TopUpStatusSuccess,
	}).Error)
	payload, err := common.Marshal(map[string]any{
		"id": "evt_creem_topup_refund", "eventType": "refund.created",
		"object": map[string]any{"request_id": tradeNo, "status": "succeeded", "transaction": map[string]any{"status": "refunded"}},
	})
	require.NoError(t, err)

	recorder := performSignedCreemSubscriptionWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code)
	var orderCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("trade_no = ?", tradeNo).Count(&orderCount).Error)
	assert.Zero(t, orderCount)
}

func TestCreemSubscriptionRefundWinsWhenTopUpSharesTradeNumber(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedExternalCreditPurchasePlans(t, 9653, 9654)
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemApiKey = "creem_api_key"
	SetCreemSubscriptionProductSnapshotForTest(t, func(context.Context, string) (int64, string, error) { return 4000, "CNY", nil })
	SetCreemSubscriptionCheckoutForTest(t, func(string, *CreemProduct, string, string) (CreemSubscriptionCheckoutResult, error) {
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/shared-trade"}, nil
	})
	create := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9653,"purchase_mode":"credit_balance"}`)
	require.Contains(t, create.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("plan_id = ?", 9653).First(&order).Error)
	completion, err := common.Marshal(map[string]any{
		"id": "evt_creem_shared_paid", "eventType": "checkout.completed",
		"object": map[string]any{"request_id": order.TradeNo, "order": map[string]any{"id": "ord_creem_shared", "transaction": "tran_creem_shared", "status": "paid", "amount_paid": 4000, "currency": "CNY"}, "product": map[string]any{"id": "prod_test"}},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, performSignedCreemSubscriptionWebhook(t, completion).Code)
	topUp := model.GetTopUpByTradeNo(order.TradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, model.PaymentProviderCreem, topUp.PaymentProvider)
	refund, err := common.Marshal(map[string]any{
		"id": "evt_creem_shared_refund", "eventType": "refund.created",
		"object": map[string]any{"request_id": order.TradeNo, "status": "succeeded", "transaction": map[string]any{"id": "tran_creem_shared", "status": "refunded"}},
	})
	require.NoError(t, err)

	recorder := performSignedCreemSubscriptionWebhook(t, refund)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusRefunded, order.Status)
	var recoveryLedgers int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).
		Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).
		Count(&recoveryLedgers).Error)
	assert.Equal(t, int64(1), recoveryLedgers)
}

func TestCreemRefundWithoutReliableOrderReturnsRetryableError(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemApiKey = "creem_api_key"
	payload, err := common.Marshal(map[string]any{
		"id": "evt_creem_unmapped_refund", "eventType": "refund.created",
		"object": map[string]any{"status": "succeeded", "transaction": map[string]any{"id": "tran_unmapped", "status": "refunded"}},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, performSignedCreemSubscriptionWebhook(t, payload).Code)
}
