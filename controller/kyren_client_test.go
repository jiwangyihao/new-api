package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeKyrenBaseURL(t *testing.T) {
	got, err := normalizeKyrenBaseURL("https://api.kyren.top/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://api.kyren.top" {
		t.Fatalf("expected trimmed production URL, got %q", got)
	}
}

func TestNormalizeKyrenBaseURLRejectsUntrustedHost(t *testing.T) {
	if _, err := normalizeKyrenBaseURL("https://evil.example.com"); err == nil {
		t.Fatal("expected untrusted host to be rejected")
	}
}

func TestNormalizeKyrenBaseURLRejectsEmptyQueryDelimiter(t *testing.T) {
	if _, err := normalizeKyrenBaseURL("https://api.kyren.top?"); err == nil {
		t.Fatal("expected empty query delimiter to be rejected")
	}
}

func TestFormatKyrenAmountCNY(t *testing.T) {
	got, err := formatKyrenAmountFromFloat(40)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "40.00" {
		t.Fatalf("expected 40.00, got %q", got)
	}
}

func TestNormalizeKyrenAmountString(t *testing.T) {
	got, err := normalizeKyrenAmountString("9.9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "9.90" {
		t.Fatalf("expected 9.90, got %q", got)
	}
}

func TestNormalizeKyrenAmountStringRejectsMoreThanTwoDecimals(t *testing.T) {
	if _, err := normalizeKyrenAmountString("1.234"); err == nil {
		t.Fatal("expected amount with more than two decimals to be rejected")
	}
}

func TestNormalizeKyrenTopUpProductsJSONRejectsInvalidProducts(t *testing.T) {
	invalid := `[{"id":"topup","name":"bad","amount":"0","currency":"USD","quota":0,"enabled":true}]`
	if _, err := normalizeKyrenTopUpProductsJSON(invalid); err == nil {
		t.Fatal("expected invalid top-up products to be rejected")
	}
}

func TestValidateKyrenOptionBeforePersistRejectsTopUpProducts(t *testing.T) {
	_, persist, err := validateKyrenOptionBeforePersist("KyrenTopUpProducts", "[]")
	if err == nil || persist {
		t.Fatalf("expected KyrenTopUpProducts to require the versioned API, persist=%v err=%v", persist, err)
	}
}

func TestApplyKyrenRuntimeOptionDoesNotOverwriteSecretWithEmptyValue(t *testing.T) {
	old := setting.KyrenApiKey
	defer func() { setting.KyrenApiKey = old }()
	setting.KyrenApiKey = "kyren_live_existing"

	if err := applyKyrenRuntimeOption("KyrenApiKey", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setting.KyrenApiKey != "kyren_live_existing" {
		t.Fatalf("empty key overwrote runtime value: %q", setting.KyrenApiKey)
	}
}

func TestKyrenClientCreateProductUsesAPIKey(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/products" {
			t.Fatalf("expected /v1/products path, got %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("x-api-key"); got != "kyren_live_test" {
			t.Fatalf("expected x-api-key kyren_live_test, got %q", got)
		}
		var req kyrenCreateProductRequest
		if err := common.DecodeJson(r.Body, &req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Name != "Basic" || req.Price != "40.00" || req.Currency != kyrenCurrencyCNY {
			t.Fatalf("unexpected create request: %+v", req)
		}
		payload, err := common.Marshal(kyrenAPIResponse[kyrenProduct]{
			Code:    0,
			Message: "success",
			Data: kyrenProduct{
				ID:       "prod_kyren_basic",
				Name:     "Basic",
				Price:    "40.00",
				Currency: kyrenCurrencyCNY,
				Status:   "ACTIVE",
			},
		})
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)

	client := &kyrenClient{baseURL: server.URL, apiKey: "kyren_live_test", httpClient: server.Client()}
	product, err := client.createProduct(context.Background(), kyrenCreateProductRequest{
		Name:     "Basic",
		Price:    "40.00",
		Currency: kyrenCurrencyCNY,
	})
	if err != nil {
		t.Fatalf("unexpected create error: %v", err)
	}
	if product == nil || product.ID != "prod_kyren_basic" || product.Status != "ACTIVE" {
		t.Fatalf("unexpected product response: %+v", product)
	}
}

func TestKyrenClientRetrievesCheckoutAndOrder(t *testing.T) {
	requestedPaths := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "kyren_live_test", r.Header.Get("x-api-key"))

		var payload []byte
		var err error
		switch r.URL.Path {
		case "/v1/checkouts/cs_retrieve":
			payload, err = common.Marshal(kyrenAPIResponse[kyrenCheckoutSession]{
				Code: 0, Message: "success",
				Data: kyrenCheckoutSession{
					ID: "cs_retrieve", ProductID: "prod_retrieve", Amount: "40.00",
					Currency: "CNY", Status: "COMPLETE", OrderID: "order_retrieve",
					ExpiresAt: 1736934000000, CreatedAt: 1736932200000,
				},
			})
		case "/v1/orders/order_retrieve":
			payload, err = common.Marshal(kyrenAPIResponse[kyrenOrder]{
				Code: 0, Message: "success",
				Data: kyrenOrder{
					ID: "order_retrieve", CheckoutSessionID: "cs_retrieve", ProductID: "prod_retrieve",
					Amount: "40.00", Currency: "CNY", Status: "PAID", PaidAt: 1736932500000,
					Metadata: map[string]string{"kind": "subscription", "trade_no": "trade_retrieve"},
				},
			})
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	client := &kyrenClient{baseURL: server.URL, apiKey: "kyren_live_test", httpClient: server.Client()}

	checkout, err := client.retrieveCheckout(context.Background(), "cs_retrieve")
	require.NoError(t, err)
	require.NotNil(t, checkout)
	assert.Equal(t, "order_retrieve", checkout.OrderID)
	assert.Equal(t, "prod_retrieve", checkout.ProductID)
	assert.Equal(t, "40.00", checkout.Amount)

	order, err := client.retrieveOrder(context.Background(), checkout.OrderID)
	require.NoError(t, err)
	require.NotNil(t, order)
	assert.Equal(t, "cs_retrieve", order.CheckoutSessionID)
	assert.Equal(t, "PAID", order.Status)
	assert.Equal(t, "trade_retrieve", order.Metadata["trade_no"])
	assert.Equal(t, []string{"/v1/checkouts/cs_retrieve", "/v1/orders/order_retrieve"}, requestedPaths)
}

func TestKyrenClientListsOrdersWithSupportedFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/orders", r.URL.Path)
		assert.Equal(t, "PENDING", r.URL.Query().Get("status"))
		assert.Equal(t, "prod_pending", r.URL.Query().Get("productId"))
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		assert.Equal(t, "100", r.URL.Query().Get("size"))
		payload, err := common.Marshal(kyrenAPIResponse[kyrenOrderList]{
			Code: 0, Message: "success",
			Data: kyrenOrderList{
				Items:      []kyrenOrder{{ID: "order_pending", Status: "PENDING", ProductID: "prod_pending"}},
				Pagination: kyrenPagination{Page: 1, Size: 100, Total: 1, TotalPages: 1},
			},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	client := &kyrenClient{baseURL: server.URL, apiKey: "kyren_live_test", httpClient: server.Client()}

	orders, err := client.listOrders(context.Background(), "PENDING", "prod_pending", 1, 100)

	require.NoError(t, err)
	require.NotNil(t, orders)
	require.Len(t, orders.Items, 1)
	assert.Equal(t, "order_pending", orders.Items[0].ID)
}
