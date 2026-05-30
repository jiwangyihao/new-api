package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
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
