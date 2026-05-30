package controller

import "github.com/QuantumNous/new-api/setting"

type kyrenAPIResponse[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type kyrenProduct struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Image       string            `json:"image"`
	Price       string            `json:"price"`
	Currency    string            `json:"currency"`
	Status      string            `json:"status"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   int64             `json:"createdAt"`
	UpdatedAt   int64             `json:"updatedAt"`
}

type kyrenCreateProductRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Image       string            `json:"image,omitempty"`
	Price       string            `json:"price"`
	Currency    string            `json:"currency,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type kyrenUpdateProductRequest struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Image       string            `json:"image,omitempty"`
	Price       string            `json:"price,omitempty"`
	Currency    string            `json:"currency,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type kyrenCreateCheckoutRequest struct {
	ProductID     string            `json:"productId"`
	SuccessURL    string            `json:"successUrl"`
	CancelURL     string            `json:"cancelUrl,omitempty"`
	CustomerEmail string            `json:"customerEmail,omitempty"`
	CustomerName  string            `json:"customerName,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type kyrenCheckoutSession struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	ExpiresAt int64  `json:"expiresAt"`
}

type kyrenTopUpProduct = setting.KyrenTopUpProduct
