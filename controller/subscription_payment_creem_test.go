package controller

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreemSubscriptionOrderStoresCheckoutAmountSnapshot(t *testing.T) {
	setupSubscriptionTrialPurchaseTest(t)
	seedSubscriptionPurchasePlan(t, 9566, false, true, 40)
	setting.CreemWebhookSecret = "creem_secret"
	SetCreemSubscriptionCheckoutForTest(t, func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
		return CreemSubscriptionCheckoutResult{URL: "https://creem.test/checkout", AmountCents: 4000, Currency: "CNY"}, nil
	})

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9566}`)

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
	fakeTransport := &creemCheckoutSuccessTransport{t: t}
	oldHTTPClient := creemHTTPClient
	creemHTTPClient = &http.Client{Transport: fakeTransport}
	t.Cleanup(func() { creemHTTPClient = oldHTTPClient })

	recorder := performSubscriptionJSON(SubscriptionRequestCreemPay, `{"plan_id":9579}`)

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
