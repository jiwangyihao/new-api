package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
)

const (
	kyrenDefaultBaseURL = setting.KyrenDefaultBaseURL
	kyrenStagingBaseURL = setting.KyrenStagingBaseURL
	kyrenCurrencyCNY    = setting.KyrenCurrencyCNY
)

const kyrenHTTPTimeout = 15 * time.Second

type kyrenProductList struct {
	Items      []kyrenProduct  `json:"items"`
	Pagination kyrenPagination `json:"pagination"`
}

type kyrenOrderList struct {
	Items      []kyrenOrder    `json:"items"`
	Pagination kyrenPagination `json:"pagination"`
}

type kyrenPagination struct {
	Page       int `json:"page"`
	Size       int `json:"size"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

type kyrenAPI interface {
	createProduct(ctx context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error)
	updateProduct(ctx context.Context, id string, req kyrenUpdateProductRequest) (*kyrenProduct, error)
	retrieveProduct(ctx context.Context, id string) (*kyrenProduct, error)
	listProducts(ctx context.Context, status string, page int, size int) (*kyrenProductList, error)
	listOrders(ctx context.Context, status string, productID string, page int, size int) (*kyrenOrderList, error)
	createCheckout(ctx context.Context, req kyrenCreateCheckoutRequest) (*kyrenCheckoutSession, error)
	retrieveCheckout(ctx context.Context, id string) (*kyrenCheckoutSession, error)
	retrieveOrder(ctx context.Context, id string) (*kyrenOrder, error)
}

type kyrenClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

var newKyrenClientForController = func() (kyrenAPI, error) {
	return newKyrenClient()
}

func newKyrenClient() (*kyrenClient, error) {
	baseURL, err := normalizeKyrenBaseURL(setting.KyrenBaseURL)
	if err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(setting.KyrenApiKey)
	if apiKey == "" {
		return nil, errors.New("Kyren API key is not configured")
	}
	return &kyrenClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: kyrenHTTPTimeout,
		},
	}, nil
}

func (c *kyrenClient) createProduct(ctx context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error) {
	return kyrenDo[kyrenProduct](ctx, c, http.MethodPost, "/v1/products", nil, req)
}

func (c *kyrenClient) updateProduct(ctx context.Context, id string, req kyrenUpdateProductRequest) (*kyrenProduct, error) {
	return kyrenDo[kyrenProduct](ctx, c, http.MethodPatch, "/v1/products/"+url.PathEscape(strings.TrimSpace(id)), nil, req)
}

func (c *kyrenClient) retrieveProduct(ctx context.Context, id string) (*kyrenProduct, error) {
	return kyrenDo[kyrenProduct](ctx, c, http.MethodGet, "/v1/products/"+url.PathEscape(strings.TrimSpace(id)), nil, nil)
}

func (c *kyrenClient) listProducts(ctx context.Context, status string, page int, size int) (*kyrenProductList, error) {
	query := url.Values{}
	if trimmed := strings.TrimSpace(status); trimmed != "" {
		query.Set("status", trimmed)
	}
	if page > 0 {
		query.Set("page", fmt.Sprintf("%d", page))
	}
	if size > 0 {
		query.Set("size", fmt.Sprintf("%d", size))
	}
	return kyrenDo[kyrenProductList](ctx, c, http.MethodGet, "/v1/products", query, nil)
}

func (c *kyrenClient) listOrders(ctx context.Context, status string, productID string, page int, size int) (*kyrenOrderList, error) {
	query := url.Values{}
	if trimmed := strings.TrimSpace(status); trimmed != "" {
		query.Set("status", trimmed)
	}
	if trimmed := strings.TrimSpace(productID); trimmed != "" {
		query.Set("productId", trimmed)
	}
	if page > 0 {
		query.Set("page", fmt.Sprintf("%d", page))
	}
	if size > 0 {
		query.Set("size", fmt.Sprintf("%d", size))
	}
	return kyrenDo[kyrenOrderList](ctx, c, http.MethodGet, "/v1/orders", query, nil)
}

func (c *kyrenClient) createCheckout(ctx context.Context, req kyrenCreateCheckoutRequest) (*kyrenCheckoutSession, error) {
	return kyrenDo[kyrenCheckoutSession](ctx, c, http.MethodPost, "/v1/checkouts", nil, req)
}

func (c *kyrenClient) retrieveCheckout(ctx context.Context, id string) (*kyrenCheckoutSession, error) {
	return kyrenDo[kyrenCheckoutSession](ctx, c, http.MethodGet, "/v1/checkouts/"+url.PathEscape(strings.TrimSpace(id)), nil, nil)
}

func (c *kyrenClient) retrieveOrder(ctx context.Context, id string) (*kyrenOrder, error) {
	return kyrenDo[kyrenOrder](ctx, c, http.MethodGet, "/v1/orders/"+url.PathEscape(strings.TrimSpace(id)), nil, nil)
}

func kyrenDo[T any](ctx context.Context, c *kyrenClient, method string, path string, query url.Values, payload any) (*T, error) {
	if c == nil {
		return nil, errors.New("Kyren client is nil")
	}
	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: kyrenHTTPTimeout}
	}
	endpoint, err := url.Parse(strings.TrimRight(c.baseURL, "/") + path)
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}
	var body io.Reader
	if payload != nil {
		encoded, err := common.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var wrapper kyrenAPIResponse[T]
	if err := common.DecodeJson(resp.Body, &wrapper); err != nil {
		return nil, fmt.Errorf("decode Kyren response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if wrapper.Message != "" {
			return nil, &kyrenHTTPError{statusCode: resp.StatusCode, message: wrapper.Message}
		}
		return nil, &kyrenHTTPError{statusCode: resp.StatusCode, message: resp.Status}
	}
	if wrapper.Code != 0 {
		message := strings.TrimSpace(wrapper.Message)
		if message == "" {
			message = fmt.Sprintf("Kyren API error code %d", wrapper.Code)
		}
		return nil, errors.New(message)
	}
	return &wrapper.Data, nil
}

type kyrenHTTPError struct {
	statusCode int
	message    string
}

func (e *kyrenHTTPError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("Kyren API HTTP %d: %s", e.statusCode, e.message)
}

func kyrenIsHTTPStatus(err error, statusCode int) bool {
	var httpErr *kyrenHTTPError
	return errors.As(err, &httpErr) && httpErr.statusCode == statusCode
}

func normalizeKyrenBaseURL(raw string) (string, error) {
	return setting.NormalizeKyrenBaseURL(raw)
}

func normalizeKyrenAmountString(raw string) (string, error) {
	return setting.NormalizeKyrenAmountString(raw)
}

func formatKyrenAmountFromFloat(raw float64) (string, error) {
	return normalizeKyrenAmountString(decimal.NewFromFloat(raw).String())
}

func kyrenDecimalEqual(a string, b string) bool {
	left, err := decimal.NewFromString(strings.TrimSpace(a))
	if err != nil {
		return false
	}
	right, err := decimal.NewFromString(strings.TrimSpace(b))
	if err != nil {
		return false
	}
	return left.Round(2).Equal(right.Round(2))
}

func normalizeKyrenTopUpProductsJSON(raw string) (string, error) {
	return setting.NormalizeKyrenTopUpProductsJSON(raw)
}

func validateKyrenOptionBeforePersist(key string, value string) (normalized string, persist bool, err error) {
	return setting.ValidateKyrenOptionBeforePersist(key, value)
}

func applyKyrenRuntimeOption(key, value string) error {
	return setting.ApplyKyrenRuntimeOption(key, value)
}
