package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

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

func TestOaiStreamHandlerErrorProbeSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const first = `{"id":"chatcmpl-test","choices":[{"index":0,"delta":{"content":"before"}}]}`
	const pending = `{"id":"chatcmpl-test","choices":[{"index":0,"delta":{"content":"pending"}}]}`
	const final = `{"id":"chatcmpl-test","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`
	for _, tc := range []struct {
		name      string
		data      string
		errorType string
	}{
		{name: "escaped_error_key", data: `{"\u0065rror":{"type":"server_error","message":"failed"}}`, errorType: "server_error"},
		{name: "case_insensitive_error_key", data: `{"ERROR":{"type":"server_error","message":"failed"}}`, errorType: "server_error"},
		{name: "string_error", data: `{"error":"failed"}`, errorType: "error"},
		{name: "scalar_error", data: `{"error":false}`, errorType: "unknown_error"},
		{name: "null_clears_error", data: `{"error":{"type":"server_error"},"error":null}`},
		{name: "error_replaces_null", data: `{"error":null,"error":{"type":"server_error"}}`, errorType: "server_error"},
		{name: "duplicate_object_replaces_type", data: `{"error":{"type":"server_error"},"error":{"message":"not classified"}}`},
		{name: "nested_error_is_content", data: `{"choices":[{"index":0,"delta":{"content":"error","tool_calls":[{"index":0,"function":{"arguments":"{\"error\":true}"}}]}}],"metadata":{"error":{"type":"server_error"}}}`},
		{name: "non_string_error_type", data: `{"error":{"type":42,"message":"not classified"}}`},
		{name: "malformed_json", data: `{"error":{"type":"server_error"},"choices":[}`},
		{name: "trailing_json", data: `{"error":{"type":"server_error"}} {}`},
		{name: "invalid_choices_preserves_validation", data: `{"error":{"type":"server_error"},"choices":"invalid"}`},
		{name: "invalid_usage_preserves_validation", data: `{"usage":{"total_tokens":"invalid"},"error":{"type":"server_error"}}`},
		{name: "overflow_preserves_validation", data: `{"choices":[{"logprobs":1e1000}],"error":{"type":"server_error"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "data: " + first + "\n\ndata: " + pending + "\n\ndata: " + tc.data + "\n\ndata: " + final + "\n\ndata: [DONE]\n\n"
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				RelayFormat:        types.RelayFormatOpenAI,
				ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
				StreamStatus:       relaycommon.NewStreamStatus(),
				ShouldIncludeUsage: true,
				DisablePing:        true,
			}
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			usage, apiErr := OaiStreamHandler(ctx, info, resp)
			if tc.errorType != "" {
				require.NotNil(t, apiErr)
				require.Equal(t, tc.errorType, apiErr.ToOpenAIError().Type)
				require.Nil(t, usage, "failed streams must not settle usage")
				require.Equal(t, "data: "+first+"\n\n", recorder.Body.String(), "the error must stop output without a success terminator")
				return
			}
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.Equal(t, 3, usage.PromptTokens)
			require.Equal(t, 2, usage.CompletionTokens)
			require.Equal(t, 5, usage.TotalTokens)
			require.False(t, info.StreamStatus.HasErrors())
			require.Equal(t, strings.ReplaceAll(body, "data: [DONE]\n\n", ""), strings.ReplaceAll(recorder.Body.String(), "data: [DONE]\n\n", ""), "non-error data chunks must retain their original wire bytes and order")
			require.True(t, strings.HasSuffix(recorder.Body.String(), "data: [DONE]\n\n"))
			require.True(t, recorder.Flushed)
		})
	}
}

func BenchmarkOaiChatStream(b *testing.B) {
	gin.SetMode(gin.TestMode)
	for _, size := range []struct{ total, chunk int }{{4096, 64}, {1 << 20, 1024}} {
		for _, tools := range []bool{false, true} {
			for _, forceFormat := range []bool{false, true} {
				b.Run(fmt.Sprintf("bytes=%d/chunk=%d/tools=%t/format=%t", size.total, size.chunk, tools, forceFormat), func(b *testing.B) {
					delta := `"content":"` + strings.Repeat("x", size.chunk) + `"`
					if tools {
						delta = `"tool_calls":[{"index":0,"id":"call_test","type":"function","function":{"arguments":"` + strings.Repeat("x", size.chunk) + `"}}]`
					}
					event := "data: " + `{"id":"chatcmpl-test","object":"chat.completion.chunk","model":"gpt-test","choices":[{"index":0,"delta":{` + delta + `}}]}` + "\n\n"
					input := strings.Repeat(event, size.total/size.chunk) + "data: " + `{"id":"chatcmpl-test","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}` + "\n\ndata: [DONE]\n\n"
					b.ReportAllocs()
					b.SetBytes(int64(size.total))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						recorder := httptest.NewRecorder()
						recorder.Body = nil
						ctx, _ := gin.CreateTestContext(recorder)
						ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
						info := &relaycommon.RelayInfo{
							RelayFormat: types.RelayFormatOpenAI,
							ChannelMeta: &relaycommon.ChannelMeta{
								UpstreamModelName: "gpt-test",
								ChannelSetting:    dto.ChannelSettings{ForceFormat: forceFormat},
							},
							ShouldIncludeUsage: true,
							DisablePing:        true,
						}
						resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(input))}
						usage, apiErr := OaiStreamHandler(ctx, info, resp)
						if apiErr != nil || usage == nil || usage.TotalTokens != 5 || !recorder.Flushed {
							b.Fatalf("unexpected stream result: usage=%+v error=%v flushed=%t", usage, apiErr, recorder.Flushed)
						}
					}
				})
			}
		}
	}
}
