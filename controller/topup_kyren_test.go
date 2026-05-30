package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kyrenTopUpSnapshotJSON(t *testing.T, localID string, productID string, amount string, currency string, quota int64) string {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"local_topup_id": localID,
		"product_id":     productID,
		"amount":         amount,
		"currency":       currency,
		"quota":          quota,
	})
	require.NoError(t, err)
	return string(payload)
}

func seedPendingKyrenTopUp(t *testing.T, tradeNo string, userID int, localID string, productID string, amount string, quota int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:          userID,
		Amount:          quota,
		Money:           10,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodKyren,
		PaymentProvider: model.PaymentProviderKyren,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
		KyrenSnapshot:   kyrenTopUpSnapshotJSON(t, localID, productID, amount, kyrenCurrencyCNY, quota),
	}).Error)
}

func TestKyrenWebhookCompletesTopUpUsingSnapshot(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6201
	seedKyrenPaymentUser(t, userID)
	tradeNo := "kyren-topup-snapshot"
	seedPendingKyrenTopUp(t, tradeNo, userID, "topup_snapshot", "prod_topup_snapshot", "10.00", 5000000)
	setKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{{
		ID:        "topup_snapshot",
		Name:      "Changed topup",
		ProductID: "prod_topup_changed",
		Amount:    "99.00",
		Currency:  kyrenCurrencyCNY,
		Quota:     1,
		Enabled:   true,
	}})
	payload := kyrenWebhookEventPayload(t, "order.paid", "topup", tradeNo, "prod_topup_snapshot", "10.00", kyrenCurrencyCNY)

	first := performSignedKyrenWebhook(t, payload)
	second := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	topUp := model.GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, int64(5000000), topUp.Amount)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 5000000, user.Quota)
}

func performKyrenTopUpPayRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/kyren/pay", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	RequestKyrenPay(ctx)
	return recorder
}

func seedKyrenPayTopUpProduct(t *testing.T, id string, productID string, amount string, quota int64) kyrenTopUpProduct {
	t.Helper()
	product := kyrenTopUpProduct{
		ID:        id,
		Name:      fmt.Sprintf("Topup %s", id),
		ProductID: productID,
		Amount:    amount,
		Currency:  kyrenCurrencyCNY,
		Quota:     quota,
		Enabled:   true,
	}
	setKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{product})
	return product
}

func TestGetTopUpInfoIncludesKyrenProducts(t *testing.T) {
	t.Run("enabled configuration returns only safe local products", func(t *testing.T) {
		setupKyrenPaymentControllerTestDB(t)

		setRawKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{
			{
				ID:          "topup_cny_10",
				Name:        "CNY 10",
				Description: "Wallet balance top-up",
				ProductID:   "prod_topup_cny_10",
				Amount:      "10.00",
				Currency:    kyrenCurrencyCNY,
				Quota:       5000000,
				Enabled:     true,
			},
			{
				ID:        "topup_disabled",
				Name:      "Disabled",
				ProductID: "prod_topup_disabled",
				Amount:    "20.00",
				Currency:  kyrenCurrencyCNY,
				Quota:     10000000,
				Enabled:   false,
			},
			{
				ID:        "topup_usd",
				Name:      "USD",
				ProductID: "prod_topup_usd",
				Amount:    "30.00",
				Currency:  "USD",
				Quota:     15000000,
				Enabled:   true,
			},
			{
				ID:        "topup_missing_product",
				Name:      "Missing Product",
				ProductID: "",
				Amount:    "40.00",
				Currency:  kyrenCurrencyCNY,
				Quota:     20000000,
				Enabled:   true,
			},
			{
				ID:        "topup_bad_amount",
				Name:      "Bad Amount",
				ProductID: "prod_topup_bad_amount",
				Amount:    "bad",
				Currency:  kyrenCurrencyCNY,
				Quota:     25000000,
				Enabled:   true,
			},
		})

		response := performTopUpInfoRequest(t)
		require.True(t, response.Success, response.Message)
		require.True(t, response.Data.EnableKyrenTopup)
		require.True(t, response.Data.EnableKyrenSubscription)
		require.Len(t, response.Data.KyrenTopUpProducts, 1)
		product := response.Data.KyrenTopUpProducts[0]
		assert.Equal(t, "topup_cny_10", product.ID)
		assert.Equal(t, "CNY 10", product.Name)
		assert.Equal(t, "10.00", product.Amount)
		assert.Equal(t, kyrenCurrencyCNY, product.Currency)
		assert.Equal(t, int64(5000000), product.Quota)
		assert.Equal(t, "Wallet balance top-up", product.Description)
		assert.True(t, product.Enabled)
		assert.NotContains(t, response.Body, "product_id")
		assert.NotContains(t, response.Body, "prod_topup_cny_10")
	})

	t.Run("missing webhook secret disables Kyren user payment", func(t *testing.T) {
		setupKyrenPaymentControllerTestDB(t)
		setting.KyrenWebhookSecret = ""
		setRawKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{{
			ID:        "topup_cny_10",
			Name:      "CNY 10",
			ProductID: "prod_topup_cny_10",
			Amount:    "10.00",
			Currency:  kyrenCurrencyCNY,
			Quota:     5000000,
			Enabled:   true,
		}})

		response := performTopUpInfoRequest(t)
		require.True(t, response.Success, response.Message)
		assert.False(t, response.Data.EnableKyrenTopup)
		assert.False(t, response.Data.EnableKyrenSubscription)
		assert.Empty(t, response.Data.KyrenTopUpProducts)
	})
}

func setRawKyrenTopUpProductsOptionForTest(t *testing.T, products []kyrenTopUpProduct) {
	t.Helper()
	payload, err := common.Marshal(products)
	require.NoError(t, err)
	option := model.Option{Key: "KyrenTopUpProducts"}
	require.NoError(t, model.DB.FirstOrCreate(&option, model.Option{Key: "KyrenTopUpProducts"}).Error)
	option.Value = string(payload)
	require.NoError(t, model.DB.Save(&option).Error)
	setting.KyrenTopUpProducts = string(payload)
	common.OptionMapRWMutex.Lock()
	common.OptionMap["KyrenTopUpProducts"] = string(payload)
	common.OptionMapRWMutex.Unlock()
}

type topUpInfoTestResponse struct {
	Success bool                 `json:"success"`
	Message string               `json:"message"`
	Data    topUpInfoTestPayload `json:"data"`
	Body    string               `json:"-"`
}

type topUpInfoTestPayload struct {
	EnableKyrenTopup        bool                           `json:"enable_kyren_topup"`
	EnableKyrenSubscription bool                           `json:"enable_kyren_subscription"`
	KyrenTopUpProducts      []topUpInfoKyrenProductForTest `json:"kyren_topup_products"`
}

type topUpInfoKyrenProductForTest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	Quota       int64  `json:"quota"`
	Enabled     bool   `json:"enabled"`
}

func performTopUpInfoRequest(t *testing.T) topUpInfoTestResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	GetTopUpInfo(ctx)

	var response topUpInfoTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	response.Body = recorder.Body.String()
	return response
}
