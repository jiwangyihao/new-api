package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Chat Completions streaming response that fails mid-stream after HTTP 200
// is signalled (by upstream and by this project's router) as a top-level
// `data: {"error": {...}}` chunk followed by stream close. The Chat wire format
// has no formal terminal-error event, so OaiStreamHandler must detect the
// top-level error chunk, classify it, and return a real NewAPIError — otherwise
// the error chunk is stored as ordinary content and the request is reported as
// a false success (mis-billed, mis-classified). Regression for the chat-port
// soft-error gap that surfaced as usage_limit / invalid_request failures
// looking like successes downstream.
func TestOaiStreamHandlerDetectsTopLevelErrorChunk(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	// A few normal content chunks, then a top-level error chunk and close. No
	// `[DONE]` — a failed stream closes without the normal terminator.
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-0","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		`data: {"id":"chatcmpl-0","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"partial"}}]}`,
		`data: {"error":{"message":"You have hit your usage limit.","type":"usage_limit_reached","code":"usage_limit_reached"}}`,
		"",
	}, "\n")

	recorder := flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{
		ChannelMeta:  &relaycommon.ChannelMeta{},
		StreamStatus: relaycommon.NewStreamStatus(),
	}

	usage, apiErr := OaiStreamHandler(c, info, resp)

	// The handler must surface the soft error as a real NewAPIError carrying the
	// upstream classification, NOT a nil error with phantom success.
	require.NotNil(t, apiErr, "top-level error chunk must produce a NewAPIError, not a false success")
	assert.Nil(t, usage, "a failed stream must not settle usage")
	assert.Contains(t, apiErr.Error(), "usage limit", "error message preserved: %s", apiErr.Error())
	assert.Equal(t, "usage_limit_reached", string(apiErr.GetErrorCode()), "upstream error code preserved")
}

// A normal Chat Completions stream (no top-level error) must pass through
// untouched: nil NewAPIError and a clean normal end. Guards against the
// error-detection probe mis-firing on ordinary content chunks.
func TestOaiStreamHandlerNormalStreamHasNoError(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-0","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		`data: {"id":"chatcmpl-0","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`data: {"id":"chatcmpl-0","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"data: [DONE]",
		"",
	}, "\n")

	recorder := flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{
		ChannelMeta:  &relaycommon.ChannelMeta{},
		StreamStatus: relaycommon.NewStreamStatus(),
	}

	_, apiErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, apiErr, "a normal stream must not be flagged as an error")
	assert.False(t, info.StreamStatus.HasErrors(), "no soft errors recorded on a clean stream")
}
