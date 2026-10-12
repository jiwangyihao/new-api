package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

const pqapiHTTPTimeout = 30 * time.Second

type pqapiClient struct {
	baseURL, siteID, secret string
	httpClient              *http.Client
}

type pqapiCreateCheckoutRequest struct {
	MerchantOrderNo string `json:"merchant_order_no"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	Description     string `json:"description"`
}

type pqapiCheckout struct {
	PaymentOrderNo  string `json:"payment_order_no"`
	MerchantOrderNo string `json:"merchant_order_no"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	CheckoutURL     string `json:"checkout_url"`
}

type pqapiCheckoutResponse struct {
	OK       bool           `json:"ok"`
	Checkout pqapiCheckout `json:"checkout"`
	Error    string         `json:"error"`
}

func newPQAPIClient() (*pqapiClient, error) {
	baseURL, err := setting.NormalizePQAPIBaseURL(setting.PQAPIBaseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(setting.PQAPISiteID) == "" || strings.TrimSpace(setting.PQAPISecret) == "" {
		return nil, errors.New("PQAPI credentials are not configured")
	}
	return &pqapiClient{
		baseURL:    baseURL,
		siteID:     strings.TrimSpace(setting.PQAPISiteID),
		secret:     strings.TrimSpace(setting.PQAPISecret),
		httpClient: &http.Client{Timeout: pqapiHTTPTimeout},
	}, nil
}

func (c *pqapiClient) createCheckout(ctx context.Context, input pqapiCreateCheckoutRequest) (*pqapiCheckout, error) {
	raw, err := common.Marshal(input)
	if err != nil {
		return nil, err
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := hex.EncodeToString(nonceBytes)
	signature := pqapiHMAC(c.secret, timestamp+"\n"+nonce+"\n"+string(raw))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/api/v1/checkout-sessions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Site-Id", c.siteID)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", signature)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var decoded pqapiCheckoutResponse
	if err := common.DecodeJson(resp.Body, &decoded); err != nil {
		return nil, fmt.Errorf("decode PQAPI response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !decoded.OK {
		return nil, fmt.Errorf("PQAPI HTTP %d: %s", resp.StatusCode, strings.TrimSpace(decoded.Error))
	}
	return &decoded.Checkout, nil
}

func pqapiHMAC(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func pqapiCheckoutURLAllowed(baseURL, checkoutURL string) bool {
	base, baseErr := url.Parse(strings.TrimSpace(baseURL))
	checkout, checkoutErr := url.Parse(strings.TrimSpace(checkoutURL))
	return baseErr == nil && checkoutErr == nil && checkout.Scheme == "https" &&
		strings.EqualFold(base.Hostname(), checkout.Hostname()) && checkout.User == nil
}
