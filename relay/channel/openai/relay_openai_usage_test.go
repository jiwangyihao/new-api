package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenaiHandlerDynamicBillingMultiplierDisabledIgnored(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	body := `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5},"newapi_billing":{"billing_multiplier":1.5,"billing_multiplier_source":"priority_tier"}}`
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OpenaiHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.TotalTokens)
	require.Equal(t, 1.0, info.FrozenDynamicBillingMultiplier())
	require.Equal(t, "default", info.FrozenDynamicBillingMultiplierSource())
	require.Equal(t, relaycommon.DynamicBillingMultiplierIgnoredReasonDisabled, info.DynamicBillingMultiplierIgnoredReason)
}

func TestOpenaiHandlerDynamicBillingMultiplierEnabledAcceptsHeaderAndBody(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	body := `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5},"newapi_billing":{"billing_multiplier":1.5,"billing_multiplier_source":"priority_tier"}}`
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}, DynamicBillingMultiplierEnabled: true}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{relaycommon.DynamicBillingMultiplierSpecHeaderName: []string{"1.25"}}, Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OpenaiHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.TotalTokens)
	require.Equal(t, 1.5, info.FrozenDynamicBillingMultiplier())
	require.Equal(t, "priority_tier", info.FrozenDynamicBillingMultiplierSource())
	require.Empty(t, recorder.Header().Get(relaycommon.DynamicBillingMultiplierHeaderName))
	require.Empty(t, recorder.Header().Get(relaycommon.DynamicBillingMultiplierSpecHeaderName))
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	billing, ok := response["newapi_billing"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(1.5), billing["billing_multiplier"])
	require.Equal(t, "priority_tier", billing["billing_multiplier_source"])
}

func TestOpenaiHandlerInvalidDynamicBillingMultiplierFallsBackToDefault(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	body := `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}, DynamicBillingMultiplierEnabled: true}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{relaycommon.DynamicBillingMultiplierHeaderName: []string{"100.01"}}, Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OpenaiHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 1.0, info.FrozenDynamicBillingMultiplier())
	require.Equal(t, "default", info.FrozenDynamicBillingMultiplierSource())
	require.Equal(t, relaycommon.DynamicBillingMultiplierIgnoredReasonInvalid, info.DynamicBillingMultiplierIgnoredReason)
}

func TestOpenaiHandlerMissingUsageIsNotTrusted(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	body := `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OpenaiHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Nil(t, usage)
	require.False(t, info.HasTrustedUsage)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotContains(t, response, "usage")
	require.NotContains(t, response, "newapi_billing")
}

func TestOpenaiHandlerExplicitZeroUsageIsTrusted(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	body := `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OpenaiHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.TotalTokens)
	require.True(t, info.HasTrustedUsage)
}

func TestOpenaiHandlerWithUsageExposesDynamicBillingMultiplier(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	body := `{"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5},"newapi_billing":{"billing_multiplier":1.25,"billing_multiplier_source":"image_priority"}}`
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, DynamicBillingMultiplierEnabled: true}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.TotalTokens)
	require.Equal(t, 1.25, info.FrozenDynamicBillingMultiplier())
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	billing, ok := response["newapi_billing"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(1.25), billing["billing_multiplier"])
	require.Equal(t, "image_priority", billing["billing_multiplier_source"])
}

func TestOpenaiHandlerDoesNotEstimateMissingPromptUsage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	payload := dto.OpenAITextResponse{
		Id:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: 1,
		Model:   "gpt-test",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role: "assistant",
				},
				FinishReason: "stop",
			},
		},
		Usage: dto.Usage{
			CompletionTokens: 4,
			TotalTokens:      4,
		},
	}
	payload.Choices[0].Message.SetStringContent("pong")
	body, err := common.Marshal(payload)
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	info.SetEstimatePromptTokens(99)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}

	usage, apiErr := OpenaiHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 4, usage.TotalTokens)
}

func TestOpenaiResponsesToChatHandlerExposesDynamicBillingMultiplier(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	body := `{"id":"resp_test","object":"response","status":"completed","created_at":1,"model":"gpt-test","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"newapi_billing":{"billing_multiplier":1.5,"billing_multiplier_source":"responses_chat"}}`
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}, DynamicBillingMultiplierEnabled: true}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OaiResponsesToChatHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.TotalTokens)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	billing, ok := response["newapi_billing"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(1.5), billing["billing_multiplier"])
	require.Equal(t, "responses_chat", billing["billing_multiplier_source"])
}

func TestOpenaiResponsesToChatHandlerDoesNotEstimateMissingUsage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	body := `{"id":"resp_test","object":"response","created_at":1,"model":"gpt-test","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}]}`
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	info.SetEstimatePromptTokens(99)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OaiResponsesToChatHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Nil(t, usage)
	require.False(t, info.HasTrustedUsage)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotContains(t, response, "newapi_billing")
}

func TestOpenaiResponsesToChatStreamHandlerMissingUsageIsNotTrusted(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"pong"}`,
		`data: {"type":"response.completed","response":{"output":[]}}`,
		"data: [DONE]",
		"",
	}, "\n")
	recorder := flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat:        types.RelayFormatOpenAI,
		ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
		ShouldIncludeUsage: true,
	}
	info.SetEstimatePromptTokens(99)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Nil(t, usage)
	require.False(t, info.HasTrustedUsage)
	require.NotContains(t, recorder.Body.String(), "newapi_billing")
}
