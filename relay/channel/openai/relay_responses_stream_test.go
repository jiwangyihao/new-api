package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flushableRecorder struct {
	*httptest.ResponseRecorder
}

func (r flushableRecorder) Flush() {
	r.ResponseRecorder.Flush()
}

func TestOaiResponsesStreamHandler_ResponseCompletedEOFMarksNormalEnd(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`,
		"data: [DONE]",
		"",
	}, "\n")
	recorder := flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{
		ChannelMeta:  &relaycommon.ChannelMeta{},
		StreamStatus: relaycommon.NewStreamStatus(),
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 5, usage.TotalTokens)
	assert.Equal(t, 3, usage.InputTokens)
	assert.Equal(t, 2, usage.OutputTokens)
	require.NotNil(t, info.StreamStatus)
	assert.Eventually(t, func() bool {
		return info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.IsNormalEnd())
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestOaiResponsesStreamHandlerDoesNotEstimatePromptTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.completed","response":{"usage":{"output_tokens":2,"total_tokens":2},"output":[]}}`,
		"data: [DONE]",
		"",
	}, "\n")
	recorder := flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{
		ChannelMeta:  &relaycommon.ChannelMeta{},
		StreamStatus: relaycommon.NewStreamStatus(),
	}
	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 0, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 2, usage.TotalTokens)
}

func TestOaiResponsesHandlerCodexProServedAckRequiresRequestMarkerAndSuccessfulUsage(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		requestMarker bool
		ack           string
		body          string
		wantCandidate bool
		wantFinal     bool
	}{
		{name: "request_marker_and_exact_ack", requestMarker: true, ack: "codex-pro", body: `{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`, wantCandidate: true, wantFinal: true},
		{name: "ack_without_request_marker", ack: "codex-pro", body: `{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "missing_ack", requestMarker: true, body: `{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "wrong_ack_pro", requestMarker: true, ack: "pro", body: `{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "wrong_ack_true", requestMarker: true, ack: "true", body: `{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "wrong_ack_2x", requestMarker: true, ack: "2x", body: `{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "empty_ack", requestMarker: true, ack: "", body: `{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "parse_failure", requestMarker: true, ack: "codex-pro", body: `{not-json`, wantCandidate: true},
		{name: "upstream_error", requestMarker: true, ack: "codex-pro", body: `{"error":{"type":"server_error","message":"upstream failed"},"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`, wantCandidate: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			markCodexProRequestSentForOpenAITest(t, info, tc.requestMarker)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}
			if tc.ack != "" || tc.name == "empty_ack" {
				resp.Header.Set("X-NewAPI-Pro-Served", tc.ack)
			}

			_, apiErr := OaiResponsesHandler(c, info, resp)

			if tc.name == "parse_failure" || tc.name == "upstream_error" {
				require.NotNil(t, apiErr)
			} else {
				require.Nil(t, apiErr)
			}
			assert.Equal(t, tc.wantFinal, getCodexProBoolFieldForOpenAITest(t, info, "CodexProServed"))
			assert.Equal(t, tc.wantCandidate, getCodexProBoolFieldForOpenAITest(t, info, "CodexProServedCandidate"))
			require.Empty(t, recorder.Header().Get("X-NewAPI-Pro-Served"))
		})
	}
}

func TestOaiResponsesCompactionHandlerCodexProServedAckRequiresSuccess(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		requestMarker bool
		ack           string
		body          string
		wantCandidate bool
		wantFinal     bool
	}{
		{name: "success", requestMarker: true, ack: "codex-pro", body: `{"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7},"output":[]}`, wantCandidate: true, wantFinal: true},
		{name: "wrong_ack", requestMarker: true, ack: "true", body: `{"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7},"output":[]}`},
		{name: "parse_failure", requestMarker: true, ack: "codex-pro", body: `{bad-json`, wantCandidate: true},
		{name: "upstream_error", requestMarker: true, ack: "codex-pro", body: `{"error":{"type":"server_error","message":"upstream failed"},"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7}}`, wantCandidate: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			markCodexProRequestSentForOpenAITest(t, info, tc.requestMarker)
			c.Set("relay_info", info)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}
			if tc.ack != "" {
				resp.Header.Set("X-NewAPI-Pro-Served", tc.ack)
			}

			_, apiErr := OaiResponsesCompactionHandler(c, resp)

			if tc.name == "parse_failure" || tc.name == "upstream_error" {
				require.NotNil(t, apiErr)
			} else {
				require.Nil(t, apiErr)
			}
			assert.Equal(t, tc.wantFinal, getCodexProBoolFieldForOpenAITest(t, info, "CodexProServed"))
			assert.Equal(t, tc.wantCandidate, getCodexProBoolFieldForOpenAITest(t, info, "CodexProServedCandidate"))
			require.Empty(t, recorder.Header().Get("X-NewAPI-Pro-Served"))
		})
	}
}

func TestOaiResponsesStreamHandlerCodexProServedAckFinalOnlyAfterCompletedNormalEnd(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		requestMarker bool
		ack           string
		body          string
		cancelBefore  bool
		wantCandidate bool
		wantFinal     bool
	}{
		{name: "completed", requestMarker: true, ack: "codex-pro", body: strings.Join([]string{`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n"), wantCandidate: true, wantFinal: true},
		{name: "missing_completed", requestMarker: true, ack: "codex-pro", body: strings.Join([]string{`data: {"type":"response.output_text.delta","delta":"hello"}`, ""}, "\n"), wantCandidate: true},
		{name: "upstream_failed_event", requestMarker: true, ack: "codex-pro", body: strings.Join([]string{`data: {"type":"response.failed","response":{"error":{"type":"server_error","message":"upstream failed"}}}`, "data: [DONE]", ""}, "\n"), wantCandidate: true},
		{name: "request_cancelled", requestMarker: true, ack: "codex-pro", body: strings.Join([]string{`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n"), cancelBefore: true, wantCandidate: true},
		{name: "wrong_ack", requestMarker: true, ack: "pro", body: strings.Join([]string{`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n")},
		{name: "ack_without_request_marker", ack: "codex-pro", body: strings.Join([]string{`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n")},
		{name: "parse_failure", requestMarker: true, ack: "codex-pro", body: strings.Join([]string{`data: {not-json`, "data: [DONE]", ""}, "\n"), wantCandidate: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gin.SetMode(gin.TestMode)
			recorder := flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			if tc.cancelBefore {
				requestCtx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(requestCtx)
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, StreamStatus: relaycommon.NewStreamStatus()}
			markCodexProRequestSentForOpenAITest(t, info, tc.requestMarker)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}
			if tc.ack != "" {
				resp.Header.Set("X-NewAPI-Pro-Served", tc.ack)
			}

			_, apiErr := OaiResponsesStreamHandler(c, info, resp)

			require.Nil(t, apiErr)
			assert.Equal(t, tc.wantFinal, getCodexProBoolFieldForOpenAITest(t, info, "CodexProServed"))
			assert.Equal(t, tc.wantCandidate, getCodexProBoolFieldForOpenAITest(t, info, "CodexProServedCandidate"))
			require.Empty(t, recorder.Header().Get("X-NewAPI-Pro-Served"))
		})
	}
}

func markCodexProRequestSentForOpenAITest(t *testing.T, info *relaycommon.RelayInfo, enabled bool) {
	t.Helper()
	if !enabled {
		return
	}
	method := reflect.ValueOf(info).MethodByName("MarkCodexProRequestSent")
	if method.IsValid() {
		method.Call(nil)
		return
	}
	setCodexProBoolFieldForOpenAITest(t, info, "CodexProRequestSent", true)
	setCodexProStringFieldForOpenAITest(t, info, "CodexProRequestMarker", "codex-pro")
}

func setCodexProBoolFieldForOpenAITest(t *testing.T, info *relaycommon.RelayInfo, fieldName string, value bool) {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	if field.CanSet() {
		field.SetBool(value)
	}
}

func setCodexProStringFieldForOpenAITest(t *testing.T, info *relaycommon.RelayInfo, fieldName string, value string) {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	if field.IsValid() && field.CanSet() {
		field.SetString(value)
	}
}

func getCodexProBoolFieldForOpenAITest(t *testing.T, info *relaycommon.RelayInfo, fieldName string) bool {
	t.Helper()
	field := reflect.ValueOf(info).Elem().FieldByName(fieldName)
	require.Truef(t, field.IsValid(), "RelayInfo must expose %s", fieldName)
	return field.Bool()
}
