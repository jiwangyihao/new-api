package dto

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/types"
)

type OpenAIResponsesCompactionResponse struct {
	ID            string          `json:"id"`
	Object        string          `json:"object"`
	CreatedAt     int             `json:"created_at"`
	Status        json.RawMessage `json:"status"`
	Output        json.RawMessage `json:"output"`
	Usage         *Usage          `json:"usage"`
	NewAPIBilling *NewAPIBilling  `json:"newapi_billing,omitempty"`
	Error         any             `json:"error,omitempty"`
}

func (o *OpenAIResponsesCompactionResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(o.Error)
}
