package helper

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMaxTokenValidatorsRejectOverflowInputs(t *testing.T) {
	overLimit := math.MaxInt32/2 + 1

	tests := []struct {
		name     string
		body     string
		validate func(*gin.Context) error
	}{
		{
			name: "OpenAI max_completion_tokens",
			body: fmt.Sprintf(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":%d}`, overLimit),
			validate: func(ctx *gin.Context) error {
				_, err := GetAndValidateTextRequest(ctx, relayconstant.RelayModeChatCompletions)
				return err
			},
		},
		{
			name: "Claude max_tokens_to_sample",
			body: fmt.Sprintf(`{"model":"claude-3-opus","messages":[{"role":"user","content":"hi"}],"max_tokens_to_sample":%d}`, overLimit),
			validate: func(ctx *gin.Context) error {
				_, err := GetAndValidateClaudeRequest(ctx)
				return err
			},
		},
		{
			name: "Responses max_output_tokens",
			body: fmt.Sprintf(`{"model":"gpt-4o","input":"hi","max_output_tokens":%d}`, overLimit),
			validate: func(ctx *gin.Context) error {
				_, err := GetAndValidateResponsesRequest(ctx)
				return err
			},
		},
		{
			name: "Alpha Search max_output_tokens",
			body: fmt.Sprintf(`{"id":"req_1","model":"gpt-4o","commands":{"search_query":[{"q":"hi"}]},"max_output_tokens":%d}`, overLimit),
			validate: func(ctx *gin.Context) error {
				_, err := GetAndValidateAlphaSearchRequest(ctx)
				return err
			},
		},
		{
			name: "Gemini generationConfig.maxOutputTokens",
			body: fmt.Sprintf(`{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":%d}}`, overLimit),
			validate: func(ctx *gin.Context) error {
				_, err := GetAndValidateGeminiRequest(ctx)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate(newMaxTokenJSONContext(t, tt.body))
			require.Error(t, err)
		})
	}
}

func TestMaxTokenValidatorsAcceptLimitBoundary(t *testing.T) {
	limit := math.MaxInt32 / 2
	tests := []struct {
		name     string
		body     string
		validate func(*gin.Context) error
	}{
		{
			name: "OpenAI max_completion_tokens",
			body: fmt.Sprintf(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":%d}`, limit),
			validate: func(ctx *gin.Context) error {
				_, err := GetAndValidateTextRequest(ctx, relayconstant.RelayModeChatCompletions)
				return err
			},
		},
		{
			name: "Claude max_tokens_to_sample",
			body: fmt.Sprintf(`{"model":"claude-3-opus","messages":[{"role":"user","content":"hi"}],"max_tokens_to_sample":%d}`, limit),
			validate: func(ctx *gin.Context) error {
				_, err := GetAndValidateClaudeRequest(ctx)
				return err
			},
		},
		{
			name: "Responses max_output_tokens",
			body: fmt.Sprintf(`{"model":"gpt-4o","input":"hi","max_output_tokens":%d}`, limit),
			validate: func(ctx *gin.Context) error {
				_, err := GetAndValidateResponsesRequest(ctx)
				return err
			},
		},
		{
			name: "Alpha Search max_output_tokens",
			body: fmt.Sprintf(`{"id":"req_1","model":"gpt-4o","commands":{"search_query":[{"q":"hi"}]},"max_output_tokens":%d}`, limit),
			validate: func(ctx *gin.Context) error {
				request, err := GetAndValidateAlphaSearchRequest(ctx)
				if err != nil {
					return err
				}
				if request.MaxOutputTokens == nil || *request.MaxOutputTokens != uint(limit) || !strings.Contains(string(request.RawBody), `"commands"`) {
					return fmt.Errorf("alpha search request did not preserve validated fields")
				}
				return nil
			},
		},
		{
			name: "Gemini generationConfig.maxOutputTokens",
			body: fmt.Sprintf(`{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":%d}}`, limit),
			validate: func(ctx *gin.Context) error {
				_, err := GetAndValidateGeminiRequest(ctx)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate(newMaxTokenJSONContext(t, tt.body))
			require.NoError(t, err)
		})
	}
}

func newMaxTokenJSONContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}
