package streamgate

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newTestGate(format string) (*Gate, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	gate := New(context.Writer, format)
	gate.Header().Set("Content-Type", "text/event-stream")
	return gate, recorder
}

func send(t *testing.T, gate *Gate, data string) {
	t.Helper()
	frame := "data: " + data + "\n\n"
	n, err := gate.WriteString(frame)
	require.NoError(t, err)
	require.Equal(t, len(frame), n)
	gate.Flush()
}

func payloads(body string) []gjson.Result {
	var out []gjson.Result
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") && gjson.Valid(line[6:]) {
			out = append(out, gjson.Parse(line[6:]))
		}
	}
	return out
}

func TestResponsesLogicalRetry(t *testing.T) {
	g, rec := newTestGate(types.RelayFormatOpenAIResponses)
	send(t, g, `{"type":"response.created","sequence_number":0,"response":{"id":"resp_A","output":[]}}`)
	require.Contains(t, rec.Body.String(), "resp_A")
	require.True(t, g.Written())
	require.True(t, g.CanRetry())
	_, err := g.WriteString(": PING\n\n")
	require.NoError(t, err)
	send(t, g, `{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"item_A","content":[]}}`)
	require.NotContains(t, rec.Body.String(), "item_A")
	send(t, g, `{"type":"response.failed","sequence_number":2,"response":{"id":"resp_A","error":{"type":"server_error","code":"overloaded","message":"busy"}}}`)
	send(t, g, `[DONE]`)
	require.NotContains(t, rec.Body.String(), "response.failed")
	require.NotContains(t, rec.Body.String(), "[DONE]")
	require.Equal(t, "overloaded", string(g.AttemptError().GetErrorCode()))
	require.True(t, g.CanRetry())
	g.BeginAttempt()
	require.Nil(t, g.AttemptError())
	send(t, g, `{"type":"response.created","sequence_number":0,"response":{"id":"resp_B","output":[]}}`)
	require.Equal(t, "resp_B", g.UpstreamResponseID())
	require.Equal(t, "resp_A", g.ResponseID())
	send(t, g, `{"type":"response.in_progress","sequence_number":1,"response":{"id":"resp_B","output":[]}}`)
	send(t, g, `{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"type":"message","id":"item_B","content":[]}}`)
	send(t, g, `{"type":"response.content_part.added","sequence_number":3,"item_id":"item_B","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`)
	send(t, g, `{"type":"response.output_text.delta","sequence_number":4,"response_id":"resp_B","item_id":"item_B","output_index":0,"content_index":0,"delta":"你好","vendor_extension":{"keep":true}}`)
	require.False(t, g.CanRetry())
	send(t, g, `{"type":"response.completed","sequence_number":5,"response":{"id":"resp_B","output":[{"type":"message","id":"item_B","content":[{"type":"output_text","text":"你好"}]}]}}`)
	send(t, g, `[DONE]`)
	send(t, g, `[DONE]`)
	g.EndAttempt()
	require.Nil(t, g.AttemptError())
	require.True(t, g.Terminal())
	body := rec.Body.String()
	assert.Equal(t, 1, strings.Count(body, `"type":"response.created"`))
	assert.Equal(t, 1, strings.Count(body, "[DONE]"))
	assert.NotContains(t, body, "resp_B")
	assert.NotContains(t, body, "item_A")
	assert.Contains(t, body, `"vendor_extension":{"keep":true}`)
	for i, event := range payloads(body) {
		assert.Equal(t, int64(i), event.Get("sequence_number").Int())
	}
}

func TestMeaningfulKinds(t *testing.T) {
	cases := []struct{ name, format, data string }{
		{"text", "openai_responses", `{"type":"response.output_text.delta","delta":"x"}`},
		{"reasoning", "openai_responses", `{"type":"response.reasoning_summary_text.delta","delta":"reason"}`},
		{"refusal", "openai_responses", `{"type":"response.refusal.delta","delta":"no"}`},
		{"tool", "openai_responses", `{"type":"response.output_item.added","item":{"type":"function_call","name":"f","call_id":"call_1","arguments":""}}`},
		{"media", "openai_responses", `{"type":"response.output_item.added","item":{"type":"image_generation_call","result":"b64"}}`},
		{"unknown", "openai_responses", `{"type":"response.vendor.result","extension":{}}`},
		{"created_partial", "openai_responses", `{"type":"response.created","response":{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}}`},
		{"chat_reasoning", "openai", `{"choices":[{"delta":{"reasoning_content":"reason"}}]}`},
		{"chat_tool", "openai", `{"choices":[{"delta":{"tool_calls":[{"id":"call","function":{"name":"f"}}]}}]}`},
		{"chat_audio", "openai", `{"choices":[{"delta":{"audio":{"data":"b64"}}}]}`},
		{"chat_annotations", "openai", `{"choices":[{"delta":{"annotations":[{"url":"https://example.com"}]}}]}`},
		{"claude_tool", "claude", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool","name":"f","input":{}}}`},
		{"claude_thinking", "claude", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reason"}}`},
		{"gemini_media", "gemini", `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"b64"}}]}}]}`},
		{"gemini_tool", "gemini", `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"f","args":{}}}]}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := newTestGate(tc.format)
			send(t, g, tc.data)
			assert.True(t, g.Meaningful())
			assert.False(t, g.CanRetry())
		})
	}
}

func TestEmptyTerminalAndHeartbeat(t *testing.T) {
	for _, data := range []string{
		`{"type":"response.completed","response":{"id":"r","output":[]}}`,
		`{"type":"response.incomplete","response":{"id":"r","output":[],"incomplete_details":{"reason":"max_output_tokens"}}}`,
		`{"type":"response.incomplete","response":{"id":"r","output":[],"incomplete_details":{"reason":"content_filter"}}}`,
	} {
		g, _ := newTestGate("openai_responses")
		send(t, g, `{"type":"ping"}`)
		require.True(t, g.CanRetry())
		send(t, g, data)
		assert.False(t, g.Meaningful())
		assert.True(t, g.Terminal())
		assert.False(t, g.CanRetry())
		assert.Nil(t, g.AttemptError())
	}
}

func TestFragmentedCRLFMultilineAndPing(t *testing.T) {
	g, rec := newTestGate("openai_responses")
	for _, piece := range []string{"event: response.created\r\n", ": PING\n\n", "data: {\r\n", "data: \"type\":\"response.created\",\"response\":{\"id\":\"r\",\"output\":[]}}\r", "\n\r\n"} {
		n, err := g.WriteString(piece)
		require.NoError(t, err)
		require.Equal(t, len(piece), n)
	}
	g.Flush()
	assert.Contains(t, rec.Body.String(), ": PING")
	assert.Contains(t, rec.Body.String(), "response.created")
	assert.True(t, g.CanRetry())
	_, err := g.WriteString("data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"r\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"output\":[]}}\n\n")
	require.NoError(t, err)
	assert.True(t, g.Terminal())
}

func TestFailureFinalization(t *testing.T) {
	g, rec := newTestGate("openai_responses")
	send(t, g, `{"type":"response.created","response":{"id":"first","output":[]}}`)
	send(t, g, `{"type":"response.failed","response":{"id":"first","error":{"type":"server_error","code":"busy","message":"try later"}}}`)
	require.NoError(t, g.Fail(nil))
	require.NoError(t, g.Fail(nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.failed"`))
	assert.Equal(t, "first", payloads(rec.Body.String())[1].Get("response.id").String())
	assert.False(t, g.CanRetry())
	for _, format := range []string{"openai", "claude", "gemini"} {
		g, rec := newTestGate(format)
		err := types.NewErrorWithStatusCode(errors.New("busy"), "overloaded", http.StatusServiceUnavailable)
		require.NoError(t, g.Fail(err))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
		assert.True(t, gjson.Valid(rec.Body.String()))
	}
}

func TestPartialFailureAndUnsafe(t *testing.T) {
	g, rec := newTestGate("openai_responses")
	send(t, g, `{"type":"response.failed","response":{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}],"error":{"type":"server_error","code":"broken","message":"failed"}}}`)
	assert.Contains(t, rec.Body.String(), "partial")
	assert.True(t, g.Meaningful())
	assert.True(t, g.Terminal())
	assert.False(t, g.CanRetry())
	assert.NotNil(t, g.AttemptError())
	g, _ = newTestGate("openai_responses")
	g.MarkUnsafe()
	send(t, g, `{"type":"response.created","response":{"id":"r","output":[]}}`)
	assert.False(t, g.CanRetry())
	assert.False(t, g.Meaningful())
}

func TestChatAndClaudeRetry(t *testing.T) {
	g, rec := newTestGate("openai")
	send(t, g, `{"id":"A","created":100,"choices":[{"delta":{"role":"assistant","content":""}}]}`)
	require.True(t, g.CanRetry())
	send(t, g, `{"error":{"type":"server_error","code":"busy","message":"busy"}}`)
	send(t, g, `[DONE]`)
	assert.NotContains(t, rec.Body.String(), "[DONE]")
	g.BeginAttempt()
	send(t, g, `{"id":"B","created":200,"choices":[{"delta":{"content":"ok"}}]}`)
	send(t, g, `{"id":"B","created":200,"choices":[{"delta":{},"finish_reason":"stop"}]}`)
	send(t, g, `{"id":"B","created":200,"choices":[],"usage":{"total_tokens":1}}`)
	send(t, g, `[DONE]`)
	for _, p := range payloads(rec.Body.String()) {
		assert.Equal(t, "A", p.Get("id").String())
		assert.Equal(t, int64(100), p.Get("created").Int())
	}
	assert.Contains(t, rec.Body.String(), "total_tokens")
	g, rec = newTestGate("claude")
	send(t, g, `{"type":"message_start","message":{"id":"A","role":"assistant","content":[]}}`)
	send(t, g, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	require.True(t, g.CanRetry())
	send(t, g, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`)
	assert.Equal(t, types.ErrorTypeClaudeError, g.AttemptError().GetErrorType())
	g.BeginAttempt()
	send(t, g, `{"type":"message_start","message":{"id":"B","role":"assistant","content":[]}}`)
	send(t, g, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	send(t, g, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`)
	send(t, g, `{"type":"content_block_stop","index":0}`)
	send(t, g, `{"type":"message_stop"}`)
	assert.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"message_start"`))
	assert.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"content_block_start"`))
	assert.True(t, g.Terminal())
	assert.False(t, g.CanRetry())
}

type brokenWriter struct{ gin.ResponseWriter }

func (w brokenWriter) Write([]byte) (int, error) { return 0, errors.New("client disconnected") }

func TestTransportFailureAndEOF(t *testing.T) {
	g, _ := newTestGate("openai_responses")
	g.ResponseWriter = brokenWriter{g.ResponseWriter}
	_, err := g.WriteString("data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n")
	require.Error(t, err)
	assert.False(t, g.CanRetry())
	assert.Error(t, g.Fail(nil))
	g, _ = newTestGate("openai_responses")
	_, err = g.WriteString("data: {\"type\":")
	require.NoError(t, err)
	g.EndAttempt()
	require.NotNil(t, g.AttemptError())
	assert.True(t, g.CanRetry())
	assert.Contains(t, g.AttemptError().Error(), "truncated")
	g.BeginAttempt()
	send(t, g, `{"type":"response.output_text.delta","delta":"hi"}`)
	g.EndAttempt()
	require.NotNil(t, g.AttemptError())
	assert.False(t, g.CanRetry())
}

func TestWebSocketPreludeAndUnknownExtension(t *testing.T) {
	g, _ := newTestGate("openai_responses")
	_, emit, err := g.TransformEvent([]byte(`{"type":"response.output_item.added","sequence_number":0,"item":{"id":"item","type":"message","content":[]},"output_index":0}`))
	require.NoError(t, err)
	require.False(t, emit)
	require.True(t, g.CanRetry())
	out, emit, err := g.TransformEvent([]byte(`{"type":"response.output_text.delta","sequence_number":1,"item_id":"item","output_index":0,"content_index":0,"delta":"hello"}`))
	require.NoError(t, err)
	require.True(t, emit)
	prelude := g.DrainPrelude()
	require.Len(t, prelude, 1)
	assert.Equal(t, int64(0), gjson.GetBytes(prelude[0], "sequence_number").Int())
	assert.Equal(t, int64(1), gjson.GetBytes(out, "sequence_number").Int())
	assert.Empty(t, g.DrainPrelude())
}

func TestPlainHTTPAndConcurrentPings(t *testing.T) {
	g, rec := newTestGate("openai")
	g.Header().Set("Content-Type", "application/json")
	g.WriteHeader(http.StatusCreated)
	_, err := g.WriteString(`{"result":"ok"}`)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.JSONEq(t, `{"result":"ok"}`, rec.Body.String())
	assert.True(t, g.Meaningful())
	g, rec = newTestGate("openai_responses")
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				_, _ = g.WriteString(": PING\n\n")
				g.Flush()
			}
		}()
	}
	wg.Wait()
	assert.True(t, g.CanRetry())
	assert.Equal(t, 80, strings.Count(rec.Body.String(), ": PING"))
}

func TestEmptyDeltasAndPrematureDone(t *testing.T) {
	g, _ := newTestGate("openai_responses")
	send(t, g, `{"type":"response.output_text.delta","delta":""}`)
	require.True(t, g.CanRetry())
	send(t, g, `[DONE]`)
	require.NotNil(t, g.AttemptError())
	require.False(t, g.Terminal())
	require.True(t, g.CanRetry())
	g, rec := newTestGate("openai")
	send(t, g, `{"choices":[{"delta":{},"finish_reason":"stop"}]}`)
	require.True(t, g.Terminal())
	require.False(t, g.CanRetry())
	require.False(t, g.Meaningful())
	send(t, g, `[DONE]`)
	assert.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
}

func TestByteFragmentationPreservesUTF8(t *testing.T) {
	g, rec := newTestGate("openai_responses")
	frame := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"你好🌍\"}\r\n\r\n")
	for _, b := range frame {
		n, err := g.Write([]byte{b})
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}
	require.True(t, g.Meaningful())
	require.Contains(t, rec.Body.String(), "你好🌍")
}

func TestFinalStreamingProtocolErrors(t *testing.T) {
	for _, format := range []string{"openai", "claude", "gemini"} {
		t.Run(format, func(t *testing.T) {
			g, rec := newTestGate(format)
			_, err := g.WriteString(": PING\n\n")
			require.NoError(t, err)
			g.Flush()
			failure := types.NewErrorWithStatusCode(errors.New("busy"), "overloaded", http.StatusServiceUnavailable)
			require.NoError(t, g.Fail(failure))
			require.NoError(t, g.Fail(failure))
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Len(t, payloads(rec.Body.String()), 1)
			if format == "openai" {
				assert.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
			}
			if format == "claude" {
				assert.Contains(t, rec.Body.String(), "event: error")
			}
		})
	}
}

func TestAttemptFailureCannotBecomeSuccess(t *testing.T) {
	g, rec := newTestGate("openai_responses")
	send(t, g, `{"type":"response.created","sequence_number":0,"response":{"id":"A","output":[]}}`)
	send(t, g, `{"type":"response.failed","sequence_number":1,"response":{"id":"A","error":{"code":"server_error","message":"busy"}}}`)
	send(t, g, `{"type":"response.completed","sequence_number":2,"response":{"id":"A","output":[]}}`)
	send(t, g, `[DONE]`)
	assert.NotContains(t, rec.Body.String(), "response.completed")
	assert.NotContains(t, rec.Body.String(), "[DONE]")
	require.True(t, g.CanRetry())
	g.BeginAttempt()
	send(t, g, `{"type":"response.created","sequence_number":0,"response":{"id":"B","output":[]}}`)
	send(t, g, `{"type":"response.output_text.delta","sequence_number":1,"delta":"answer"}`)
	g.EndAttempt()
	require.NoError(t, g.Fail(nil))
	assert.NotContains(t, rec.Body.String(), "busy")
	assert.Contains(t, rec.Body.String(), "answer")
	assert.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.failed"`))
}

func TestRetryDiscardsFragmentAndOwnsHeldEvents(t *testing.T) {
	g, rec := newTestGate("openai_responses")
	_, err := g.WriteString(`data: {"type":`)
	require.NoError(t, err)
	g.BeginAttempt()
	send(t, g, `{"type":"response.created","response":{"id":"B","output":[]}}`)
	assert.Contains(t, rec.Body.String(), "response.created")
	held := []byte(`{"type":"response.output_item.added","item":{"id":"owned-item","type":"message","content":[]}}`)
	_, emit, err := g.TransformEvent(held)
	require.NoError(t, err)
	require.False(t, emit)
	for i := range held {
		held[i] = 'x'
	}
	_, emit, err = g.TransformEvent([]byte(`{"type":"response.in_progress","response":{"id":"B","output":[]}}`))
	require.NoError(t, err)
	require.True(t, emit)
	assert.Empty(t, g.DrainPrelude(), "progress must not publish empty item scaffolding")
	_, emit, err = g.TransformEvent([]byte(`{"type":"response.output_text.delta","item_id":"owned-item","delta":"hello"}`))
	require.NoError(t, err)
	require.True(t, emit)
	prelude := g.DrainPrelude()
	require.Len(t, prelude, 1)
	assert.Equal(t, "owned-item", gjson.GetBytes(prelude[0], "item.id").String())
}

func TestClaudeBlockClosureAndPlainHTTPCompletion(t *testing.T) {
	g, rec := newTestGate("claude")
	for _, event := range []string{
		`{"type":"message_start","message":{"id":"m","content":[]}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
		`{"type":"message_stop"}`,
	} {
		send(t, g, event)
	}
	var kinds []string
	for _, event := range payloads(rec.Body.String()) {
		kinds = append(kinds, event.Get("type").String())
	}
	assert.Equal(t, []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}, kinds)
	g, rec = newTestGate("openai")
	g.Header().Set("Content-Type", "application/json")
	_, err := g.WriteString(`{"result":"ok"}`)
	require.NoError(t, err)
	g.EndAttempt()
	assert.Nil(t, g.AttemptError())
	assert.JSONEq(t, `{"result":"ok"}`, rec.Body.String())
}

func TestSyntheticFailureProtocolsAndMasking(t *testing.T) {
	for _, format := range []string{"openai_responses", "claude", "gemini"} {
		t.Run(format, func(t *testing.T) {
			g, rec := newTestGate(format)
			_, err := g.WriteString(": PING\n\n")
			require.NoError(t, err)
			failure := types.NewErrorWithStatusCode(errors.New("upstream https://private.example.com/path?token=secret"), "server_error", http.StatusServiceUnavailable)
			require.NoError(t, g.Fail(failure))
			require.NoError(t, g.Fail(failure))
			actual := payloads(rec.Body.String())
			require.Len(t, actual, 1)
			assert.NotContains(t, rec.Body.String(), "token=secret")
			switch format {
			case "openai_responses":
				assert.Equal(t, "response.failed", actual[0].Get("type").String())
				assert.Equal(t, "server_error", actual[0].Get("response.error.code").String())
			case "claude":
				assert.Equal(t, "error", actual[0].Get("type").String())
				assert.True(t, actual[0].Get("error.message").Exists())
			case "gemini":
				assert.Equal(t, int64(503), actual[0].Get("error.code").Int())
				assert.Equal(t, "UNAVAILABLE", actual[0].Get("error.status").String())
			}
		})
	}
}

func TestResponseBindingFailureClosesReplay(t *testing.T) {
	g, rec := newTestGate("openai_responses")
	g.BindResponseIdentity("public", func(public, upstream string) error {
		require.Equal(t, "public", public)
		require.Equal(t, "private", upstream)
		return errors.New("shared response store unavailable")
	})
	_, err := g.WriteString("data: {\"type\":\"response.created\",\"response\":{\"id\":\"private\",\"output\":[]}}\n\n")
	require.Error(t, err)
	require.NotNil(t, g.AttemptError())
	require.True(t, types.IsSkipRetryError(g.AttemptError()))
	require.False(t, g.CanRetry())
	require.False(t, g.Meaningful())
	require.Empty(t, rec.Body.String())
}

func TestReplaySafetySurvivesDroppedConvertedEvents(t *testing.T) {
	for _, raw := range []string{
		`{"type":"response.created","response":{"id":"r","background":true,"output":[]}}`,
		`{"type":"response.queued","response":{"id":"r","output":[]}}`,
		`{"type":"response.created","response":{"id":"r","output":[],"vendor_extension":{"accepted":true}}}`,
		`{"type":"response.vendor.result","extension":{}}`,
		`{"type":"response.reasoning_text.delta","delta":"x"}`,
		`{"type":"response.output_item.added","item":{"type":"web_search_call","id":"side_effect"}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			g, rec := newTestGate("openai")
			require.Nil(t, g.ObserveReplaySafety([]byte(raw), "openai_responses"))
			require.False(t, g.CanRetry())
			require.False(t, g.Meaningful(), "observing execution safety does not claim downstream delivery")
			require.Empty(t, rec.Body.String())
		})
	}
	g, _ := newTestGate("openai")
	require.Nil(t, g.ObserveReplaySafety([]byte(`{"type":"response.created","response":{"id":"r","output":[]}}`), "openai_responses"))
	require.True(t, g.CanRetry())
	err := g.ObserveReplaySafety([]byte(`{"type":"error","code":"overloaded","message":"busy"}`), "openai_responses")
	require.NotNil(t, err)
	require.Equal(t, "overloaded", string(err.GetErrorCode()))
	require.True(t, g.CanRetry())
}

func TestFinalResponseRouteBindingUsesSuccessfulAttempt(t *testing.T) {
	g, rec := newTestGate("openai_responses")
	var upstreams []string
	g.BindResponseIdentity("public", func(public, upstream string) error {
		require.Equal(t, "public", public)
		upstreams = append(upstreams, upstream)
		return nil
	})
	send(t, g, `{"type":"response.created","sequence_number":0,"response":{"id":"A","output":[]}}`)
	send(t, g, `{"type":"response.failed","response":{"id":"A","error":{"code":"busy"}}}`)
	g.BeginAttempt()
	send(t, g, `{"type":"response.created","sequence_number":0,"response":{"id":"B","output":[]}}`)
	send(t, g, `{"type":"response.output_text.delta","sequence_number":1,"response_id":"B","delta":"x"}`)
	send(t, g, `{"type":"response.completed","sequence_number":2,"response":{"id":"B","output":[]}}`)
	require.Equal(t, []string{"A", "B"}, upstreams)
	require.Equal(t, "B", g.UpstreamResponseID())
	require.Equal(t, "public", g.ResponseID())
	require.NotContains(t, rec.Body.String(), `"id":"B"`)
	require.NotContains(t, rec.Body.String(), `"response_id":"B"`)
	require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.created"`))
}
