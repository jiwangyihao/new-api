package controller

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	waffo "github.com/waffo-com/waffo-go"
	waffonet "github.com/waffo-com/waffo-go/net"
	"github.com/waffo-com/waffo-go/utils"
	"gorm.io/gorm"
)

const topUpCentsUserID = 7701

type topUpCentsWaffoTransport struct {
	t              *testing.T
	responseBody   string
	responseSigner string
}

func (transport *topUpCentsWaffoTransport) Send(_ context.Context, _ *waffonet.HttpRequest) (*waffonet.HttpResponse, error) {
	transport.t.Helper()
	signature, err := utils.Sign(transport.responseBody, transport.responseSigner)
	require.NoError(transport.t, err)
	return waffonet.NewHttpResponse(http.StatusOK, map[string]string{"X-SIGNATURE": signature}, []byte(transport.responseBody)), nil
}

type topUpCentsCreemTransport struct {
	t    *testing.T
	hits int
}

func (transport *topUpCentsCreemTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport.t.Helper()
	transport.hits++
	if req.URL.Host != "test-api.creem.io" || req.URL.Path != "/v1/checkouts" {
		return nil, fmt.Errorf("unexpected Creem request url: %s", req.URL.String())
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"checkout_url":"https://pay.creem.test/checkout","id":"chk_creem"}`)),
		Request:    req,
	}, nil
}

type topUpCentsProviderCase struct {
	name     string
	create   func(t *testing.T) string
	complete func(tradeNo string) error
}

func setupTopUpCentsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.Option{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.UserSubscription{}, &model.InvitationMonthlyEntitlement{}))
	require.NoError(t, db.Create(&model.User{Id: topUpCentsUserID, Username: "topup-cents", Email: "topup-cents@example.com", Status: common.UserStatusEnabled, AffCode: "topup-cents"}).Error)

	oldPayAddress := operation_setting.PayAddress
	oldEpayID := operation_setting.EpayId
	oldEpayKey := operation_setting.EpayKey
	oldPayMethods := operation_setting.PayMethods
	oldMinTopUp := operation_setting.MinTopUp
	oldPrice := operation_setting.Price
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldPaymentSetting := *operation_setting.GetPaymentSetting()
	oldStripeApiSecret := setting.StripeApiSecret
	oldStripePriceID := setting.StripePriceId
	oldStripeUnitPrice := setting.StripeUnitPrice
	oldStripeMinTopUp := setting.StripeMinTopUp
	oldWaffoEnabled := setting.WaffoEnabled
	oldWaffoSandbox := setting.WaffoSandbox
	oldWaffoSandboxApiKey := setting.WaffoSandboxApiKey
	oldWaffoSandboxPrivateKey := setting.WaffoSandboxPrivateKey
	oldWaffoSandboxPublicCert := setting.WaffoSandboxPublicCert
	oldWaffoMerchantID := setting.WaffoMerchantId
	oldWaffoCurrency := setting.WaffoCurrency
	oldWaffoUnitPrice := setting.WaffoUnitPrice
	oldWaffoMinTopUp := setting.WaffoMinTopUp
	oldWaffoNotifyURL := setting.WaffoNotifyUrl
	oldWaffoReturnURL := setting.WaffoReturnUrl
	oldWaffoPancakeEnabled := setting.WaffoPancakeEnabled
	oldWaffoPancakeSandbox := setting.WaffoPancakeSandbox
	oldWaffoPancakeMerchantID := setting.WaffoPancakeMerchantID
	oldWaffoPancakePrivateKey := setting.WaffoPancakePrivateKey
	oldWaffoPancakeWebhookPublicKey := setting.WaffoPancakeWebhookPublicKey
	oldWaffoPancakeWebhookTestKey := setting.WaffoPancakeWebhookTestKey
	oldWaffoPancakeStoreID := setting.WaffoPancakeStoreID
	oldWaffoPancakeProductID := setting.WaffoPancakeProductID
	oldWaffoCustomTransportForTest := waffoCustomTransportForTest
	oldWaffoPancakeCurrency := setting.WaffoPancakeCurrency
	oldWaffoPancakeUnitPrice := setting.WaffoPancakeUnitPrice
	oldWaffoPancakeMinTopUp := setting.WaffoPancakeMinTopUp
	oldWaffoPancakeReturnURL := setting.WaffoPancakeReturnURL
	oldCreemApiKey := setting.CreemApiKey
	oldCreemWebhookSecret := setting.CreemWebhookSecret
	oldCreemTestMode := setting.CreemTestMode
	oldCreemProducts := setting.CreemProducts
	oldKyrenApiKey := setting.KyrenApiKey
	oldKyrenWebhookSecret := setting.KyrenWebhookSecret
	oldKyrenTopUpProducts := setting.KyrenTopUpProducts
	oldOptionMap := common.OptionMap
	oldHTTPClient := http.DefaultClient
	oldCreemHTTPClient := creemHTTPClient

	operation_setting.PayAddress = "https://epay.test"
	operation_setting.EpayId = "pid_test"
	operation_setting.EpayKey = "key_test"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "Alipay"}}
	operation_setting.MinTopUp = 1
	operation_setting.Price = 1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	operation_setting.GetPaymentSetting().AmountOptions = []int{}
	setting.StripeApiSecret = "sk_test_topup_cents"
	setting.StripePriceId = "price_topup_cents"
	setting.StripeUnitPrice = 1
	setting.StripeMinTopUp = 1
	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoMerchantId = "merchant_topup_cents"
	setting.WaffoCurrency = "USD"
	setting.WaffoUnitPrice = 1
	setting.WaffoMinTopUp = 1
	setting.WaffoNotifyUrl = ""
	setting.WaffoReturnUrl = ""
	setting.WaffoPancakeEnabled = true
	setting.WaffoPancakeSandbox = true
	setting.WaffoPancakeMerchantID = "merchant_topup_cents"
	setting.WaffoPancakeStoreID = "store_topup_cents"
	setting.WaffoPancakeProductID = "product_topup_cents"
	setting.WaffoPancakeCurrency = "USD"
	setting.WaffoPancakeUnitPrice = 1
	setting.WaffoPancakeMinTopUp = 1
	setting.WaffoPancakeReturnURL = ""
	setting.CreemApiKey = "creem_api_key"
	setting.CreemWebhookSecret = "creem_webhook_secret"
	setting.CreemTestMode = true
	setting.CreemProducts = "[]"
	setting.KyrenApiKey = "kyren_api_key"
	setting.KyrenWebhookSecret = "kyren_webhook_secret"
	setting.KyrenTopUpProducts = "[]"
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{"KyrenTopUpProducts": "[]"}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		operation_setting.PayAddress = oldPayAddress
		operation_setting.EpayId = oldEpayID
		operation_setting.EpayKey = oldEpayKey
		operation_setting.PayMethods = oldPayMethods
		operation_setting.MinTopUp = oldMinTopUp
		operation_setting.Price = oldPrice
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		*operation_setting.GetPaymentSetting() = oldPaymentSetting
		setting.StripeApiSecret = oldStripeApiSecret
		setting.StripePriceId = oldStripePriceID
		setting.StripeUnitPrice = oldStripeUnitPrice
		setting.StripeMinTopUp = oldStripeMinTopUp
		setting.WaffoEnabled = oldWaffoEnabled
		setting.WaffoSandbox = oldWaffoSandbox
		setting.WaffoSandboxApiKey = oldWaffoSandboxApiKey
		setting.WaffoSandboxPrivateKey = oldWaffoSandboxPrivateKey
		setting.WaffoSandboxPublicCert = oldWaffoSandboxPublicCert
		setting.WaffoMerchantId = oldWaffoMerchantID
		setting.WaffoCurrency = oldWaffoCurrency
		setting.WaffoUnitPrice = oldWaffoUnitPrice
		setting.WaffoMinTopUp = oldWaffoMinTopUp
		setting.WaffoNotifyUrl = oldWaffoNotifyURL
		setting.WaffoReturnUrl = oldWaffoReturnURL
		setting.WaffoPancakeEnabled = oldWaffoPancakeEnabled
		setting.WaffoPancakeSandbox = oldWaffoPancakeSandbox
		setting.WaffoPancakeMerchantID = oldWaffoPancakeMerchantID
		setting.WaffoPancakePrivateKey = oldWaffoPancakePrivateKey
		setting.WaffoPancakeWebhookPublicKey = oldWaffoPancakeWebhookPublicKey
		setting.WaffoPancakeWebhookTestKey = oldWaffoPancakeWebhookTestKey
		setting.WaffoPancakeStoreID = oldWaffoPancakeStoreID
		setting.WaffoPancakeProductID = oldWaffoPancakeProductID
		setting.WaffoPancakeCurrency = oldWaffoPancakeCurrency
		setting.WaffoPancakeUnitPrice = oldWaffoPancakeUnitPrice
		setting.WaffoPancakeMinTopUp = oldWaffoPancakeMinTopUp
		waffoCustomTransportForTest = oldWaffoCustomTransportForTest
		setting.WaffoPancakeReturnURL = oldWaffoPancakeReturnURL
		setting.CreemApiKey = oldCreemApiKey
		setting.CreemWebhookSecret = oldCreemWebhookSecret
		setting.CreemTestMode = oldCreemTestMode
		setting.CreemProducts = oldCreemProducts
		setting.KyrenApiKey = oldKyrenApiKey
		setting.KyrenWebhookSecret = oldKyrenWebhookSecret
		setting.KyrenTopUpProducts = oldKyrenTopUpProducts
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
		http.DefaultClient = oldHTTPClient
		creemHTTPClient = oldCreemHTTPClient
	})
	return db
}

func setQuotaDisplayTypeForTopUpTest(t *testing.T, displayType string) {
	t.Helper()
	operation_setting.GetGeneralSetting().QuotaDisplayType = displayType
}

func requestEpayForTopUpCentsTest(t *testing.T, amount int64) *httptest.ResponseRecorder {
	t.Helper()
	return performTopUpCentsJSONRequest(t, RequestEpay, topUpCentsUserID, fmt.Sprintf(`{"amount":%d,"payment_method":"alipay"}`, amount))
}

func requestStripeForTopUpCentsTest(t *testing.T, amount int64) *httptest.ResponseRecorder {
	t.Helper()
	withStripeSessionServerForTopUpCentsTest(t)
	return performTopUpCentsJSONRequest(t, RequestStripePay, topUpCentsUserID, fmt.Sprintf(`{"amount":%d,"payment_method":"stripe"}`, amount))
}

func requestStripeAmountForTopUpCentsTest(t *testing.T, amount int64) *httptest.ResponseRecorder {
	t.Helper()
	withStripeSessionServerForTopUpCentsTest(t)
	return performTopUpCentsJSONRequest(t, RequestStripeAmount, topUpCentsUserID, fmt.Sprintf(`{"amount":%d}`, amount))
}

func requestWaffoForTopUpCentsTest(t *testing.T, amount int64) *httptest.ResponseRecorder {
	t.Helper()
	responsePrivateKey := configureWaffoKeysForTopUpCentsTest(t)
	waffoCustomTransportForTest = &topUpCentsWaffoTransport{
		t:              t,
		responseBody:   `{"code":"0","msg":"success","data":{"orderAction":"https://pay.waffo.test/checkout"}}`,
		responseSigner: responsePrivateKey,
	}
	return performTopUpCentsJSONRequest(t, RequestWaffoPay, topUpCentsUserID, fmt.Sprintf(`{"amount":%d}`, amount))
}

func requestWaffoPancakeForTopUpCentsTest(t *testing.T, amount int64) *httptest.ResponseRecorder {
	t.Helper()
	return requestWaffoPancakeForTopUpCentsTestWithPayload(t, amount, `{"data":{"sessionId":"sess_topup_cents","checkoutUrl":"https://pay.waffo-pancake.test/checkout","expiresAt":"2099-01-01T00:00:00Z","orderId":"remote_order"}}`)
}

var waffoPancakeCreateSessionBodyForTopUpCentsTest string

func requestWaffoPancakeForTopUpCentsTestWithPayload(t *testing.T, amount int64, payload string) *httptest.ResponseRecorder {
	t.Helper()
	configureWaffoPancakeKeysForTopUpCentsTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		waffoPancakeCreateSessionBodyForTopUpCentsTest = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(func() {
		waffoPancakeCreateSessionBodyForTopUpCentsTest = ""
		server.Close()
	})
	transport := &rewriteHostRoundTripper{target: server.URL, base: http.DefaultTransport}
	http.DefaultClient = &http.Client{Transport: transport}
	return performTopUpCentsJSONRequest(t, RequestWaffoPancakePay, topUpCentsUserID, fmt.Sprintf(`{"amount":%d}`, amount))
}

func requestCreemForTopUpCentsTest(t *testing.T, amountCents int64) *httptest.ResponseRecorder {
	t.Helper()
	productID := fmt.Sprintf("prod_creem_%d", amountCents)
	setting.CreemProducts = fmt.Sprintf(`[{"productId":%q,"name":"Creem %d","price":%.2f,"currency":"CNY","quota":%d}]`, productID, amountCents, float64(amountCents)/100, amountCents)
	fakeTransport := &topUpCentsCreemTransport{t: t}
	creemHTTPClient = &http.Client{Transport: fakeTransport}
	t.Cleanup(func() { creemHTTPClient = &http.Client{Timeout: 30 * time.Second} })
	recorder := performTopUpCentsJSONRequest(t, RequestCreemPay, topUpCentsUserID, fmt.Sprintf(`{"product_id":%q,"payment_method":"creem"}`, productID))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, fakeTransport.hits)
	require.Contains(t, recorder.Body.String(), "https://pay.creem.test/checkout")
	require.Contains(t, recorder.Body.String(), `"message":"success"`)
	return recorder
}

func requestKyrenForTopUpCentsTest(t *testing.T, amountCents int64) *httptest.ResponseRecorder {
	t.Helper()
	product := seedKyrenPayTopUpProduct(t, fmt.Sprintf("topup_%d", amountCents), fmt.Sprintf("prod_kyren_%d", amountCents), fmt.Sprintf("%.2f", float64(amountCents)/100), amountCents)
	fake := &kyrenCheckoutFakeAPI{retrieveProductFunc: func(_ context.Context, id string) (*kyrenProduct, error) {
		return &kyrenProduct{ID: id, Status: kyrenProductStatusActive, Price: product.Amount, Currency: kyrenCurrencyCNY}, nil
	}}
	withKyrenCheckoutFakeControllerClient(t, fake)
	return performKyrenTopUpPayRequest(t, topUpCentsUserID, fmt.Sprintf(`{"product_id":%q}`, product.ID))
}

func performTopUpCentsJSONRequest(t *testing.T, handler func(*gin.Context), userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup", strings.NewReader(body))

	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	handler(ctx)
	return recorder
}

var stripeSessionFormForTopUpCentsTest url.Values
var stripePriceFormForTopUpCentsTest url.Values
var stripeBasePriceCurrencyForTopUpCentsTest = "usd"
var stripePriceGetAuthorizationForTopUpCentsTest string

func getStripePriceFormValueForTopUpCentsTest(t *testing.T, key string) string {
	t.Helper()
	require.NotNil(t, stripePriceFormForTopUpCentsTest)
	return stripePriceFormForTopUpCentsTest.Get(key)
}

func getStripeSessionFormValueForTopUpCentsTest(t *testing.T, key string) string {
	t.Helper()
	require.NotNil(t, stripeSessionFormForTopUpCentsTest)
	return stripeSessionFormForTopUpCentsTest.Get(key)
}

func withStripeSessionServerForTopUpCentsTest(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prices/price_topup_cents":
			stripePriceGetAuthorizationForTopUpCentsTest = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"price_topup_cents","object":"price","currency":%q,"unit_amount":100,"product":"prod_topup_cents"}`, stripeBasePriceCurrencyForTopUpCentsTest)))
		case "/v1/prices":
			require.NoError(t, r.ParseForm())
			stripePriceFormForTopUpCentsTest = r.Form
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"price_topup_dynamic","object":"price","currency":%q,"unit_amount":16000,"product":"prod_topup_cents"}`, stripeBasePriceCurrencyForTopUpCentsTest)))
		case "/v1/checkout/sessions":
			require.NoError(t, r.ParseForm())
			stripeSessionFormForTopUpCentsTest = r.Form
			if value := r.Form.Get("line_items[0][price]"); value != "price_topup_dynamic" {
				t.Errorf("unexpected Stripe checkout price: %q", value)
			}
			if value := r.Form.Get("line_items[0][quantity]"); value != "1" {
				t.Errorf("unexpected Stripe quantity: %q", value)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cs_topup_cents","object":"checkout.session","url":"https://checkout.stripe.test/session"}`))
		default:
			t.Errorf("unexpected Stripe path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	oldBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(server.URL), MaxNetworkRetries: stripe.Int64(0)}))
	t.Cleanup(func() {
		stripeBasePriceCurrencyForTopUpCentsTest = "usd"
		stripePriceFormForTopUpCentsTest = nil
		stripePriceGetAuthorizationForTopUpCentsTest = ""
		stripeSessionFormForTopUpCentsTest = nil
		stripe.SetBackend(stripe.APIBackend, oldBackend)
	})
}

func configureWaffoKeysForTopUpCentsTest(t *testing.T) string {
	t.Helper()
	sdkKeys, err := waffo.GenerateKeyPair()
	require.NoError(t, err)
	responseKeys, err := waffo.GenerateKeyPair()
	require.NoError(t, err)
	setting.WaffoSandboxApiKey = "waffo_api_key"
	setting.WaffoSandboxPrivateKey = sdkKeys.PrivateKey
	setting.WaffoSandboxPublicCert = responseKeys.PublicKey
	return responseKeys.PrivateKey
}

func configureWaffoPancakeKeysForTopUpCentsTest(t *testing.T) {
	t.Helper()
	privateKey, publicKey := generateRSAKeyPairPEMForTopUpCentsTest(t)
	setting.WaffoPancakePrivateKey = privateKey
	setting.WaffoPancakeWebhookTestKey = publicKey
	setting.WaffoPancakeWebhookPublicKey = publicKey
}

func generateRSAKeyPairPEMForTopUpCentsTest(t *testing.T) (string, string) {
	t.Helper()
	keyPair, err := utils.GenerateKeyPair()
	require.NoError(t, err)
	privateDER, err := base64.StdEncoding.DecodeString(keyPair.PrivateKey)
	require.NoError(t, err)
	publicDER, err := base64.StdEncoding.DecodeString(keyPair.PublicKey)
	require.NoError(t, err)
	privateKey, err := x509.ParsePKCS8PrivateKey(privateDER)
	require.NoError(t, err)
	rsaPrivate, ok := privateKey.(*rsa.PrivateKey)
	require.True(t, ok)
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaPrivate)}))
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
	return privatePEM, publicPEM
}

type rewriteHostRoundTripper struct {
	target string
	base   http.RoundTripper
}

func (rt *rewriteHostRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	targetURL, err := url.Parse(rt.target + req.URL.Path)
	if err != nil {
		return nil, err
	}
	clone.URL = targetURL
	clone.Host = targetURL.Host
	if rt.base == nil {
		rt.base = http.DefaultTransport
	}
	return rt.base.RoundTrip(clone)
}

func getLatestTopUpForUserTest(t *testing.T, userID int) model.TopUp {
	t.Helper()
	var topUp model.TopUp
	require.NoError(t, model.DB.Where("user_id = ?", userID).Order("id DESC").First(&topUp).Error)
	return topUp
}

func getTopUpByTradeNoForTopUpCentsTest(t *testing.T, tradeNo string) model.TopUp {
	t.Helper()
	var topUp model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&topUp).Error)
	return topUp
}

func getUserQuotaForTopUpCentsTest(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func getUserStripeCustomerForTopUpCentsTest(t *testing.T, userID int) string {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("stripe_customer").Where("id = ?", userID).First(&user).Error)
	return user.StripeCustomer
}

func getUserEmailForTopUpCentsTest(t *testing.T, userID int) string {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("email").Where("id = ?", userID).First(&user).Error)
	return user.Email
}

func getTopUpStatusForTopUpCentsTest(t *testing.T, tradeNo string) string {
	t.Helper()
	return getTopUpByTradeNoForTopUpCentsTest(t, tradeNo).Status
}

func seedUserEmailForTopUpCentsTest(t *testing.T, userID int, email string) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("email", email).Error)
}

func seedStripeTopUpForWebhookTest(t *testing.T, tradeNo string, amount int64, money float64) {
	t.Helper()
	seedTopUpForProviderTest(t, tradeNo, model.PaymentProviderStripe, model.PaymentMethodStripe, amount, money, common.TopUpStatusPending)
}

func seedCreemTopUpForWebhookTest(t *testing.T, amount int64, money float64) string {
	t.Helper()
	tradeNo := fmt.Sprintf("creem-cents-%d-%d", amount, time.Now().UnixNano())
	seedTopUpForProviderTest(t, tradeNo, model.PaymentProviderCreem, model.PaymentMethodCreem, amount, money, common.TopUpStatusPending)
	return tradeNo
}

func seedExpiredTopUpForProviderTest(t *testing.T, tradeNo string, provider string, method string, amount int64, money float64) {
	t.Helper()
	seedTopUpForProviderTest(t, tradeNo, provider, method, amount, money, common.TopUpStatusExpired)
}

func seedTopUpForProviderTest(t *testing.T, tradeNo string, provider string, method string, amount int64, money float64, status string) {
	t.Helper()
	topUp := &model.TopUp{
		UserId:          topUpCentsUserID,
		Amount:          amount,
		AmountUnit:      model.TopUpAmountUnitAccountBalanceCents,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   method,
		PaymentProvider: provider,
		CreateTime:      common.GetTimestamp(),
		Status:          status,
	}
	if provider == model.PaymentProviderKyren {
		topUp.KyrenSnapshot = kyrenTopUpSnapshotJSON(t, "topup_cents", "prod_kyren_cents", fmt.Sprintf("%.2f", money), kyrenCurrencyCNY, amount)
	}
	require.NoError(t, model.DB.Create(topUp).Error)
}

func completeStripeTopUpForTest(tradeNo string) error {
	return completeStripeTopUpForTestWithCustomer(tradeNo, "cus_topup_cents")
}

func completeStripeTopUpForTestWithCustomer(tradeNo string, customerID string) error {
	return model.Recharge(tradeNo, customerID, "127.0.0.1")
}

func completeCreemTopUpForTest(tradeNo string) error {
	return completeCreemTopUpForTestWithCustomer(tradeNo, "paid@example.com", "Paid User")
}

func completeCreemTopUpForTestWithCustomer(tradeNo string, customerEmail string, customerName string) error {
	return model.RechargeCreem(tradeNo, customerEmail, customerName, "127.0.0.1")
}

func completeWaffoTopUpForTest(tradeNo string) error {
	return model.RechargeWaffo(tradeNo, "127.0.0.1")
}

func completeWaffoPancakeTopUpForTest(tradeNo string) error {
	return model.RechargeWaffoPancake(tradeNo)
}

func completeEpayTopUpForTest(tradeNo string) error {
	params := map[string]string{
		"pid":          operation_setting.EpayId,
		"type":         "alipay",
		"out_trade_no": tradeNo,
		"trade_status": "TRADE_SUCCESS",
		"trade_no":     "epay_" + tradeNo,
		"name":         "topup",
		"money":        "39.90",
	}
	signed := epayGenerateParamsForTopUpCentsTest(params, operation_setting.EpayKey)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	form := make([]string, 0, len(signed))
	for key, value := range signed {
		form = append(form, fmt.Sprintf("%s=%s", key, value))
	}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/epay/notify", strings.NewReader(strings.Join(form, "&")))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	EpayNotify(ctx)
	if recorder.Body.String() != "success" {
		return fmt.Errorf("epay notify failed: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if getTopUpStatusForTopUpCentsNoFail(tradeNo) != common.TopUpStatusSuccess {
		return fmt.Errorf("epay topup not completed")
	}
	return nil
}

func epayGenerateParamsForTopUpCentsTest(params map[string]string, key string) map[string]string {
	client := GetEpayClient()
	if client == nil {
		params["sign"] = ""
		params["sign_type"] = "MD5"
		return params
	}
	return epayGenerateParamsWithClientForTopUpCentsTest(client.Config.Key, params)
}

func epayGenerateParamsWithClientForTopUpCentsTest(key string, params map[string]string) map[string]string {
	filtered := make(map[string]string, len(params))
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		filtered[k] = v
		keys = append(keys, k)
	}
	sortStringsForTopUpCentsTest(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(filtered[k])
	}
	params["sign"] = epayMD5ForTopUpCentsTest(b.String() + key)
	params["sign_type"] = "MD5"
	return params
}

func sortStringsForTopUpCentsTest(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func epayMD5ForTopUpCentsTest(value string) string {
	return fmt.Sprintf("%x", md5SumForTopUpCentsTest(value))
}

func md5SumForTopUpCentsTest(value string) [16]byte {
	return md5.Sum([]byte(value))
}

func completeKyrenTopUpForTest(tradeNo string) error {
	topUp := getTopUpByTradeNoForTopUpCentsNoFail(tradeNo)
	if topUp == nil {
		return model.ErrTopUpNotFound
	}
	payload := kyrenWebhookEventPayloadFromTopUpCentsTest(topUp)
	recorder := performSignedKyrenWebhookNoFail(payload)
	if recorder.Code != http.StatusOK {
		return fmt.Errorf("kyren webhook failed: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
	fresh := getTopUpByTradeNoForTopUpCentsNoFail(tradeNo)
	if fresh == nil || fresh.Status != common.TopUpStatusSuccess {
		return model.ErrTopUpStatusInvalid
	}
	return nil
}

func kyrenWebhookEventPayloadFromTopUpCentsTest(topUp *model.TopUp) []byte {
	productID := "prod_kyren_cents"
	amount := fmt.Sprintf("%.2f", topUp.Money)
	if strings.TrimSpace(topUp.KyrenSnapshot) != "" {
		var snapshot kyrenTopUpSnapshot
		if err := common.UnmarshalJsonStr(topUp.KyrenSnapshot, &snapshot); err == nil {
			productID = snapshot.ProductID
			amount = snapshot.Amount
		}
	}
	payload, _ := common.Marshal(map[string]any{
		"id":   "evt_" + strings.ReplaceAll(topUp.TradeNo, "-", "_"),
		"type": "order.paid",
		"data": map[string]any{
			"id":        "ord_" + strings.ReplaceAll(topUp.TradeNo, "-", "_"),
			"productId": productID,
			"amount":    amount,
			"currency":  kyrenCurrencyCNY,
			"metadata": map[string]string{
				"kind":     "topup",
				"trade_no": topUp.TradeNo,
			},
		},
	})
	return payload
}

func performSignedKyrenWebhookNoFail(payload []byte) *httptest.ResponseRecorder {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/kyren/webhook", bytes.NewReader(payload))
	ctx.Request.Header.Set("Kyren-Timestamp", timestamp)
	ctx.Request.Header.Set("Kyren-Signature", signKyrenWebhookPayload(payload, timestamp, setting.KyrenWebhookSecret))
	KyrenWebhook(ctx)
	return recorder
}

func getTopUpByTradeNoForTopUpCentsNoFail(tradeNo string) *model.TopUp {
	var topUp model.TopUp
	if err := model.DB.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
		return nil
	}
	return &topUp
}

func getTopUpStatusForTopUpCentsNoFail(tradeNo string) string {
	if topUp := getTopUpByTradeNoForTopUpCentsNoFail(tradeNo); topUp != nil {
		return topUp.Status
	}
	return ""
}

func newEpayCentsProviderCase(amountCents int64) topUpCentsProviderCase {
	return topUpCentsProviderCase{name: "epay", create: func(t *testing.T) string {
		recorder := requestEpayForTopUpCentsTest(t, amountCents/100)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		return getLatestTopUpForUserTest(t, topUpCentsUserID).TradeNo
	}, complete: completeEpayTopUpForTest}
}

func newWaffoCentsProviderCase(amountCents int64) topUpCentsProviderCase {
	return topUpCentsProviderCase{name: "waffo", create: func(t *testing.T) string {
		recorder := requestWaffoForTopUpCentsTest(t, amountCents/100)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		return getLatestTopUpForUserTest(t, topUpCentsUserID).TradeNo
	}, complete: completeWaffoTopUpForTest}
}

func newWaffoPancakeCentsProviderCase(amountCents int64) topUpCentsProviderCase {
	return topUpCentsProviderCase{name: "waffo-pancake", create: func(t *testing.T) string {
		recorder := requestWaffoPancakeForTopUpCentsTest(t, amountCents/100)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		return getLatestTopUpForUserTest(t, topUpCentsUserID).TradeNo
	}, complete: completeWaffoPancakeTopUpForTest}
}

func newCreemCentsProviderCase(amountCents int64) topUpCentsProviderCase {
	return topUpCentsProviderCase{name: "creem", create: func(t *testing.T) string {
		recorder := requestCreemForTopUpCentsTest(t, amountCents)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		return getLatestTopUpForUserTest(t, topUpCentsUserID).TradeNo
	}, complete: completeCreemTopUpForTest}
}

func newKyrenCentsProviderCase(amountCents int64) topUpCentsProviderCase {
	return topUpCentsProviderCase{name: "kyren", create: func(t *testing.T) string {
		recorder := requestKyrenForTopUpCentsTest(t, amountCents)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		return getLatestTopUpForUserTest(t, topUpCentsUserID).TradeNo
	}, complete: completeKyrenTopUpForTest}
}

func assertTopUpSuccessLogUsesAccountBalanceFormat(t *testing.T, tradeNo string, expected string) {
	t.Helper()
	topUp := getTopUpByTradeNoForTopUpCentsTest(t, tradeNo)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", topUp.UserId, model.LogTypeTopup).Order("id DESC").Find(&logs).Error)
	expectedFields := []string{"充值金额: " + expected, "充值额度: " + expected}
	for _, log := range logs {
		if !topUpLogMatchesPaymentMethod(t, log, topUp.PaymentMethod) {
			continue
		}
		for _, expectedField := range expectedFields {
			if containsTopUpBalanceAmountField(log.Content, expectedField) {
				return
			}
		}
	}
	require.Failf(t, "missing account balance topup log", "trade_no=%s expected=%s payment_method=%s logs=%v", tradeNo, expected, topUp.PaymentMethod, logs)
}
func containsTopUpBalanceAmountField(content string, field string) bool {
	start := strings.Index(content, field)
	if start < 0 {
		return false
	}
	suffix := strings.TrimSpace(content[start+len(field):])
	if suffix == "" {
		return true
	}
	for _, separator := range []string{"，", ",", ";", "；", "\n", "\r"} {
		if strings.HasPrefix(suffix, separator) {
			return true
		}
	}
	lowerSuffix := strings.ToLower(suffix)
	if strings.HasPrefix(lowerSuffix, "m") || strings.HasPrefix(lowerSuffix, "k") || strings.HasPrefix(lowerSuffix, "token") || strings.HasPrefix(lowerSuffix, "cent") {
		return false
	}
	next := suffix[0]
	return (next < '0' || next > '9') && next != '.'
}

func topUpLogMatchesPaymentMethod(t *testing.T, log model.Log, paymentMethod string) bool {
	t.Helper()
	if strings.TrimSpace(log.Other) == "" {
		return paymentMethod == model.PaymentMethodWaffoPancake
	}
	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &payload))
	adminInfo, ok := payload["admin_info"].(map[string]any)
	if !ok {
		return paymentMethod == model.PaymentMethodWaffoPancake
	}
	loggedMethod, _ := adminInfo["payment_method"].(string)
	return loggedMethod == paymentMethod
}

func TestEpayTopUpCreatesAmountInCentsWhenTokenDisplayEnabled(t *testing.T) {
	setupTopUpCentsTestDB(t)
	oldDisplay := operation_setting.GetQuotaDisplayType()
	oldQPU := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	setQuotaDisplayTypeForTopUpTest(t, operation_setting.QuotaDisplayTypeTokens)
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQPU
		setQuotaDisplayTypeForTopUpTest(t, oldDisplay)
	})

	recorder := requestEpayForTopUpCentsTest(t, 40)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	topUp := getLatestTopUpForUserTest(t, topUpCentsUserID)
	assert.EqualValues(t, 4000, topUp.Amount)
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
}

func TestStripeTopUpCreatesAmountInCents(t *testing.T) {
	setupTopUpCentsTestDB(t)
	recorder := requestStripeForTopUpCentsTest(t, 40)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	topUp := getLatestTopUpForUserTest(t, topUpCentsUserID)
	assert.EqualValues(t, 4000, topUp.Amount)
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
}

func TestStripeTopUpCentsPersistsPaymentMoney(t *testing.T) {
	setupTopUpCentsTestDB(t)
	oldUnitPrice := setting.StripeUnitPrice
	setting.StripeUnitPrice = 8
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{40: 0.5}
	t.Cleanup(func() {
		setting.StripeUnitPrice = oldUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	})

	assertStripeTopUpCentsPayment(t, 160.0, "16000")
}

func TestStripeTopUpCentsRoundsStoredPaymentMoneyToCheckoutAmount(t *testing.T) {
	setupTopUpCentsTestDB(t)
	oldUnitPrice := setting.StripeUnitPrice
	setting.StripeUnitPrice = 0.333
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	t.Cleanup(func() {
		setting.StripeUnitPrice = oldUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	})

	assertStripeTopUpCentsPayment(t, 13.32, "1332")
}

func TestStripeTopUpCentsUsesZeroDecimalCurrencyMinorUnits(t *testing.T) {
	setupTopUpCentsTestDB(t)
	stripeBasePriceCurrencyForTopUpCentsTest = "jpy"
	oldUnitPrice := setting.StripeUnitPrice
	setting.StripeUnitPrice = 8
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{40: 0.5}
	t.Cleanup(func() {
		setting.StripeUnitPrice = oldUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	})

	assertStripeTopUpCentsPayment(t, 160.0, "160")
}

func TestStripeTopUpCentsRoundsStoredPaymentMoneyToZeroDecimalCheckoutAmount(t *testing.T) {
	setupTopUpCentsTestDB(t)
	stripeBasePriceCurrencyForTopUpCentsTest = "jpy"
	oldUnitPrice := setting.StripeUnitPrice
	setting.StripeUnitPrice = 0.333
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	t.Cleanup(func() {
		setting.StripeUnitPrice = oldUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	})

	assertStripeTopUpCentsPayment(t, 13.0, "13")
}

func TestStripeTopUpCentsRoundsSpecialIntegerCurrencyToCheckoutAmount(t *testing.T) {
	setupTopUpCentsTestDB(t)
	stripeBasePriceCurrencyForTopUpCentsTest = "isk"
	oldUnitPrice := setting.StripeUnitPrice
	setting.StripeUnitPrice = 0.333
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	t.Cleanup(func() {
		setting.StripeUnitPrice = oldUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	})

	assertStripeTopUpCentsPayment(t, 13.0, "1300")
}

func TestStripeTopUpAmountPreviewUsesCheckoutQuantizedMoney(t *testing.T) {
	setupTopUpCentsTestDB(t)
	stripeBasePriceCurrencyForTopUpCentsTest = "jpy"
	oldUnitPrice := setting.StripeUnitPrice
	setting.StripeUnitPrice = 0.333
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	t.Cleanup(func() {
		setting.StripeUnitPrice = oldUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	})

	recorder := requestStripeAmountForTopUpCentsTest(t, 40)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"data":"13.00"`)
}

func TestStripeTopUpAmountPreviewSetsStripeKeyBeforePriceLookup(t *testing.T) {
	setupTopUpCentsTestDB(t)
	stripe.Key = ""
	oldUnitPrice := setting.StripeUnitPrice
	setting.StripeUnitPrice = 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	t.Cleanup(func() {
		setting.StripeUnitPrice = oldUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	})

	recorder := requestStripeAmountForTopUpCentsTest(t, 40)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"data":"40.00"`)
	assert.Equal(t, "Bearer sk_test_topup_cents", stripePriceGetAuthorizationForTopUpCentsTest)
}

func assertStripeTopUpCentsPayment(t *testing.T, expectedMoney float64, expectedUnitAmount string) {
	t.Helper()

	recorder := requestStripeForTopUpCentsTest(t, 40)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	topUp := getLatestTopUpForUserTest(t, topUpCentsUserID)
	assert.EqualValues(t, 4000, topUp.Amount)
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
	assert.Equal(t, expectedMoney, topUp.Money)
	assert.Equal(t, expectedUnitAmount, getStripePriceFormValueForTopUpCentsTest(t, "unit_amount"))
	assert.Equal(t, "price_topup_dynamic", getStripeSessionFormValueForTopUpCentsTest(t, "line_items[0][price]"))
	assert.Equal(t, "1", getStripeSessionFormValueForTopUpCentsTest(t, "line_items[0][quantity]"))
}

func TestWaffoPancakeTopUpPersistsRemoteOrderID(t *testing.T) {
	setupTopUpCentsTestDB(t)

	recorder := requestWaffoPancakeForTopUpCentsTest(t, 40)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	topUp := getLatestTopUpForUserTest(t, topUpCentsUserID)
	assert.Equal(t, "remote_order", topUp.TradeNo)
	var response struct {
		Data struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	assert.Equal(t, "remote_order", response.Data.OrderID)
}

func TestWaffoPancakeTopUpKeepsLocalTradeNoWhenRemoteOrderIDMissing(t *testing.T) {
	setupTopUpCentsTestDB(t)

	recorder := requestWaffoPancakeForTopUpCentsTestWithPayload(t, 40, `{"data":{"sessionId":"sess_without_order_id","checkoutUrl":"https://pay.waffo-pancake.test/checkout","expiresAt":"2099-01-01T00:00:00Z"}}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	topUp := getLatestTopUpForUserTest(t, topUpCentsUserID)
	assert.True(t, strings.HasPrefix(topUp.TradeNo, "WAFFO_PANCAKE-"), topUp.TradeNo)
	assert.NotEqual(t, "sess_without_order_id", topUp.TradeNo)

	var request struct {
		OrderMerchantExternalID string `json:"orderMerchantExternalId"`
	}
	require.NoError(t, common.UnmarshalJsonStr(waffoPancakeCreateSessionBodyForTopUpCentsTest, &request))
	assert.Equal(t, topUp.TradeNo, request.OrderMerchantExternalID)
}

func TestStripeWebhookCreditsTopUpAmountCents(t *testing.T) {
	setupTopUpCentsTestDB(t)
	seedStripeTopUpForWebhookTest(t, "stripe-cents", 4000, 40)
	require.NoError(t, completeStripeTopUpForTest("stripe-cents"))
	topUp := getTopUpByTradeNoForTopUpCentsTest(t, "stripe-cents")
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
	assert.Equal(t, 4000, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
}

func TestStripeTopUpCentsWebhookPreservesCustomerID(t *testing.T) {
	setupTopUpCentsTestDB(t)
	seedStripeTopUpForWebhookTest(t, "stripe-customer", 4000, 40)
	require.NoError(t, completeStripeTopUpForTestWithCustomer("stripe-customer", "cus_balance_cents"))
	assert.Equal(t, "cus_balance_cents", getUserStripeCustomerForTopUpCentsTest(t, topUpCentsUserID))
	assert.Equal(t, 4000, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
}

func TestStripeWebhookIgnoresCacheInvalidationFailureAfterCommit(t *testing.T) {
	setupTopUpCentsTestDB(t)
	setupControllerBrokenRedis(t)
	seedStripeTopUpForWebhookTest(t, "stripe-broken-cache", 4000, 40)

	require.NoError(t, completeStripeTopUpForTest("stripe-broken-cache"))

	assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForTopUpCentsTest(t, "stripe-broken-cache"))
	assert.Equal(t, 4000, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
}

func TestCreemWebhookIgnoresCacheInvalidationFailureAfterCommit(t *testing.T) {
	setupTopUpCentsTestDB(t)
	setupControllerBrokenRedis(t)
	tradeNo := seedCreemTopUpForWebhookTest(t, 4000, 40)

	require.NoError(t, completeCreemTopUpForTest(tradeNo))

	assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForTopUpCentsTest(t, tradeNo))
	assert.Equal(t, 4000, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
}

func TestKyrenWebhookIgnoresCacheInvalidationFailureAfterCommit(t *testing.T) {
	setupTopUpCentsTestDB(t)
	setupControllerBrokenRedis(t)
	seedTopUpForProviderTest(t, "kyren-broken-cache", model.PaymentProviderKyren, model.PaymentMethodKyren, 4000, 40, common.TopUpStatusPending)

	require.NoError(t, completeKyrenTopUpForTest("kyren-broken-cache"))

	assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForTopUpCentsTest(t, "kyren-broken-cache"))
	assert.Equal(t, 4000, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
}

func TestCreemTopUpCentsWebhookPreservesEmailBackfill(t *testing.T) {
	setupTopUpCentsTestDB(t)
	seedUserEmailForTopUpCentsTest(t, topUpCentsUserID, "")
	tradeNo := seedCreemTopUpForWebhookTest(t, 3990, 40)
	require.NoError(t, completeCreemTopUpForTestWithCustomer(tradeNo, "paid@example.com", "Paid User"))
	assert.Equal(t, "paid@example.com", getUserEmailForTopUpCentsTest(t, topUpCentsUserID))
	assert.Equal(t, 3990, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
}

func TestEveryTopUpProviderCreatesAndCreditsAccountBalanceCents(t *testing.T) {
	setupTopUpCentsTestDB(t)
	oldQPU := common.QuotaPerUnit
	common.QuotaPerUnit = 987654
	t.Cleanup(func() { common.QuotaPerUnit = oldQPU })
	providers := []topUpCentsProviderCase{
		newEpayCentsProviderCase(4000),
		newWaffoCentsProviderCase(4000),
		newWaffoPancakeCentsProviderCase(4000),
		newCreemCentsProviderCase(4000),
		newKyrenCentsProviderCase(4000),
	}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			tradeNo := provider.create(t)
			topUp := getTopUpByTradeNoForTopUpCentsTest(t, tradeNo)
			assert.EqualValues(t, 4000, topUp.Amount)
			assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
			before := getUserQuotaForTopUpCentsTest(t, topUp.UserId)
			require.NoError(t, provider.complete(tradeNo))
			assert.Equal(t, before+4000, getUserQuotaForTopUpCentsTest(t, topUp.UserId))
			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForTopUpCentsTest(t, tradeNo))
			assertTopUpSuccessLogUsesAccountBalanceFormat(t, tradeNo, "40.00")
		})
	}
}

func TestTopUpCentsCompletionIsIdempotent(t *testing.T) {
	setupTopUpCentsTestDB(t)
	providers := []struct {
		name     string
		provider string
		method   string
		complete func(tradeNo string) error
	}{
		{name: "epay", provider: model.PaymentProviderEpay, method: "alipay", complete: completeEpayTopUpForTest},
		{name: "stripe", provider: model.PaymentProviderStripe, method: model.PaymentMethodStripe, complete: completeStripeTopUpForTest},
		{name: "waffo", provider: model.PaymentProviderWaffo, method: model.PaymentMethodWaffo, complete: completeWaffoTopUpForTest},
		{name: "waffo-pancake", provider: model.PaymentProviderWaffoPancake, method: model.PaymentMethodWaffoPancake, complete: completeWaffoPancakeTopUpForTest},
		{name: "creem", provider: model.PaymentProviderCreem, method: model.PaymentMethodCreem, complete: completeCreemTopUpForTest},
		{name: "manual", provider: model.PaymentProviderEpay, method: "alipay", complete: func(tradeNo string) error { return model.ManualCompleteTopUp(tradeNo, "127.0.0.1") }},
	}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			tradeNo := "idempotent-" + provider.name
			seedTopUpForProviderTest(t, tradeNo, provider.provider, provider.method, 4000, 40, common.TopUpStatusPending)
			require.NoError(t, provider.complete(tradeNo))
			before := getUserQuotaForTopUpCentsTest(t, topUpCentsUserID)
			require.NoError(t, provider.complete(tradeNo))
			assert.Equal(t, before, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
		})
	}
}

func TestNewTopUpPersistsAccountBalanceCentsAmountUnit(t *testing.T) {
	setupTopUpCentsTestDB(t)
	recorder := requestEpayForTopUpCentsTest(t, 40)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	topUp := getLatestTopUpForUserTest(t, topUpCentsUserID)
	assert.EqualValues(t, 4000, topUp.Amount)
	assert.Equal(t, model.TopUpAmountUnitAccountBalanceCents, topUp.AmountUnit)
}

func TestNormalizeKyrenTopUpProductsRejectsInvalidBalanceProducts(t *testing.T) {
	invalidCases := []string{
		`[{"id":"zero","name":"Zero","amount":"10.00","currency":"CNY","quota":0,"enabled":true}]`,
		`[{"id":"negative","name":"Negative","amount":"10.00","currency":"CNY","quota":-1,"enabled":true}]`,
		`[{"id":"usd","name":"USD","amount":"10.00","currency":"USD","quota":1000,"enabled":true}]`,
		`[{"id":"bad-amount","name":"Bad","amount":"bad","currency":"CNY","quota":1000,"enabled":true}]`,
		`[{"id":"too-many-decimals","name":"Bad","amount":"1.234","currency":"CNY","quota":1000,"enabled":true}]`,
	}
	for _, raw := range invalidCases {
		_, err := setting.NormalizeKyrenTopUpProductsJSON(raw)
		require.Error(t, err, raw)
	}
	normalized, err := setting.NormalizeKyrenTopUpProductsJSON(`[{"id":"ok","name":"OK","amount":"10","currency":"CNY","quota":1000,"enabled":true}]`)
	require.NoError(t, err)
	assert.Contains(t, normalized, `"amount":"10.00"`)
}

func TestExpiredTopUpCannotBeCompletedAfterCentsMigration(t *testing.T) {
	setupTopUpCentsTestDB(t)
	seedExpiredTopUpForProviderTest(t, "expired-manual", model.PaymentProviderEpay, "alipay", 20000000, 40)
	before := getUserQuotaForTopUpCentsTest(t, topUpCentsUserID)

	err := model.ManualCompleteTopUp("expired-manual", "127.0.0.1")

	require.Error(t, err)
	assert.Equal(t, before, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
	assert.Equal(t, common.TopUpStatusExpired, getTopUpStatusForTopUpCentsTest(t, "expired-manual"))
}

func TestExpiredProviderTopUpsCannotBeCreditedByLateWebhook(t *testing.T) {
	setupTopUpCentsTestDB(t)
	providers := []struct {
		name     string
		provider string
		method   string
		complete func(tradeNo string) error
	}{
		{name: "epay", provider: model.PaymentProviderEpay, method: "alipay", complete: completeEpayTopUpForTest},
		{name: "stripe", provider: model.PaymentProviderStripe, method: model.PaymentMethodStripe, complete: completeStripeTopUpForTest},
		{name: "waffo", provider: model.PaymentProviderWaffo, method: model.PaymentMethodWaffo, complete: completeWaffoTopUpForTest},
		{name: "waffo-pancake", provider: model.PaymentProviderWaffoPancake, method: model.PaymentMethodWaffoPancake, complete: completeWaffoPancakeTopUpForTest},
		{name: "creem", provider: model.PaymentProviderCreem, method: model.PaymentMethodCreem, complete: completeCreemTopUpForTest},
		{name: "kyren", provider: model.PaymentProviderKyren, method: model.PaymentMethodKyren, complete: completeKyrenTopUpForTest},
	}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			tradeNo := "expired-" + provider.name
			seedExpiredTopUpForProviderTest(t, tradeNo, provider.provider, provider.method, 20000000, 40)
			before := getUserQuotaForTopUpCentsTest(t, topUpCentsUserID)

			err := provider.complete(tradeNo)

			require.Error(t, err)
			assert.Equal(t, before, getUserQuotaForTopUpCentsTest(t, topUpCentsUserID))
			assert.Equal(t, common.TopUpStatusExpired, getTopUpStatusForTopUpCentsTest(t, tradeNo))
		})
	}
}
