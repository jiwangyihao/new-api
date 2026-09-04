package dto

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// AlphaSearchRequest is the Codex standalone web search request.
// RawBody preserves the original JSON so unknown fields are forwarded intact.
type AlphaSearchRequest struct {
	Model           string          `json:"model"`
	Id              string          `json:"id,omitempty"`
	Stream          *bool           `json:"stream,omitempty"`
	MaxOutputTokens *uint           `json:"max_output_tokens,omitempty"`
	RawBody         json.RawMessage `json:"-"`
}

func (r *AlphaSearchRequest) GetTokenCountMeta() *types.TokenCountMeta {
	meta := &types.TokenCountMeta{
		CombineText: string(r.RawBody),
		TokenType:   types.TokenTypeTokenizer,
	}
	if r.MaxOutputTokens != nil {
		meta.MaxTokens = int(*r.MaxOutputTokens)
	}
	return meta
}

func (r *AlphaSearchRequest) IsStream(_ *gin.Context) bool {
	return false
}

func (r *AlphaSearchRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}
