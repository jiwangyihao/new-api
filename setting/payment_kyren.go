package setting

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

const (
	KyrenDefaultBaseURL = "https://api.kyren.top"
	KyrenStagingBaseURL = "https://staging-api.kyren.top"
	KyrenCurrencyCNY    = "CNY"
)

var KyrenApiKey = ""
var KyrenWebhookSecret = ""
var KyrenBaseURL = KyrenDefaultBaseURL
var KyrenTopUpProducts = "[]"

type KyrenTopUpProduct struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ProductID   string `json:"product_id,omitempty"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	Quota       int64  `json:"quota"`
	Enabled     bool   `json:"enabled"`
}

func NormalizeKyrenBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = KyrenDefaultBaseURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid Kyren base URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return "", errors.New("Kyren base URL must use https")
	}
	if parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Kyren base URL must not include query or fragment")
	}
	host := strings.ToLower(parsed.Host)
	switch host {
	case "api.kyren.top", "staging-api.kyren.top":
	default:
		return "", errors.New("untrusted Kyren base URL host")
	}
	parsed.Host = host
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path != "" {
		return "", errors.New("Kyren base URL must not include a path")
	}
	return parsed.String(), nil
}

func NormalizeKyrenAmountString(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	amount, err := decimal.NewFromString(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid Kyren amount: %w", err)
	}
	if amount.LessThan(decimal.NewFromInt(0)) || amount.Equal(decimal.Zero) {
		return "", errors.New("Kyren amount must be at least 0.01")
	}
	minimum := decimal.NewFromInt(1).Div(decimal.NewFromInt(100))
	if amount.LessThan(minimum) {
		return "", errors.New("Kyren amount must be at least 0.01")
	}
	if amount.Exponent() < -2 {
		return "", errors.New("Kyren amount must use at most two decimal places")
	}
	return amount.StringFixed(2), nil
}

func NormalizeKyrenTopUpProductsJSON(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = "[]"
	}
	var products []KyrenTopUpProduct
	if err := common.UnmarshalJsonStr(trimmed, &products); err != nil {
		return "", fmt.Errorf("invalid Kyren top-up products JSON: %w", err)
	}
	seenIDs := make(map[string]struct{}, len(products))
	for i := range products {
		product := &products[i]
		product.ID = strings.TrimSpace(product.ID)
		product.Name = strings.TrimSpace(product.Name)
		product.Description = strings.TrimSpace(product.Description)
		product.ProductID = strings.TrimSpace(product.ProductID)
		product.Currency = strings.ToUpper(strings.TrimSpace(product.Currency))
		if product.Currency == "" {
			product.Currency = KyrenCurrencyCNY
		}
		if product.ID == "" {
			return "", errors.New("Kyren top-up product id is required")
		}
		if _, ok := seenIDs[product.ID]; ok {
			return "", fmt.Errorf("duplicate Kyren top-up product id %q", product.ID)
		}
		seenIDs[product.ID] = struct{}{}
		if product.Name == "" {
			return "", fmt.Errorf("Kyren top-up product %q name is required", product.ID)
		}
		if product.Currency != KyrenCurrencyCNY {
			return "", errors.New("Kyren top-up products only support CNY")
		}
		if product.Quota <= 0 {
			return "", fmt.Errorf("Kyren top-up product %q quota must be positive", product.ID)
		}
		amount, err := NormalizeKyrenAmountString(product.Amount)
		if err != nil {
			return "", fmt.Errorf("Kyren top-up product %q amount: %w", product.ID, err)
		}
		product.Amount = amount
	}
	payload, err := common.Marshal(products)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func ValidateKyrenOptionBeforePersist(key string, value string) (normalized string, persist bool, err error) {
	switch key {
	case "KyrenApiKey", "KyrenWebhookSecret":
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return "", false, nil
		}
		return trimmed, true, nil
	case "KyrenBaseURL":
		normalized, err := NormalizeKyrenBaseURL(value)
		return normalized, err == nil, err
	case "KyrenTopUpProducts":
		return "", false, errors.New("KyrenTopUpProducts must be updated via /api/payment/kyren/topup-products")
	default:
		return value, true, nil
	}
}

func PrepareKyrenRuntimeOption(key string, value string) (normalized string, apply bool, err error) {
	switch key {
	case "KyrenApiKey", "KyrenWebhookSecret":
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return "", false, nil
		}
		return trimmed, true, nil
	case "KyrenBaseURL":
		normalized, err := NormalizeKyrenBaseURL(value)
		return normalized, err == nil, err
	case "KyrenTopUpProducts":
		normalized, err := NormalizeKyrenTopUpProductsJSON(value)
		return normalized, err == nil, err
	default:
		return value, true, nil
	}
}

func ApplyKyrenRuntimeOption(key, value string) error {
	normalized, apply, err := PrepareKyrenRuntimeOption(key, value)
	if err != nil || !apply {
		return err
	}
	switch key {
	case "KyrenApiKey":
		KyrenApiKey = normalized
	case "KyrenWebhookSecret":
		KyrenWebhookSecret = normalized
	case "KyrenBaseURL":
		KyrenBaseURL = normalized
	case "KyrenTopUpProducts":
		KyrenTopUpProducts = normalized
	}
	return nil
}
