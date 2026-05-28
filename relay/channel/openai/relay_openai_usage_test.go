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
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(string(body)))}

	usage, apiErr := OpenaiHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 4, usage.TotalTokens)
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
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OaiResponsesToChatHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.PromptTokens)
	require.Equal(t, 0, usage.CompletionTokens)
	require.Equal(t, 0, usage.TotalTokens)
}
