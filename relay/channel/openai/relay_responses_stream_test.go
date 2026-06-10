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

type trailerOnEOFBodyForOpenAITest struct {
	reader *strings.Reader
	onEOF  func()
}

func (b *trailerOnEOFBodyForOpenAITest) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if err == io.EOF && b.onEOF != nil {
		b.onEOF()
		b.onEOF = nil
	}
	return n, err
}

func (b *trailerOnEOFBodyForOpenAITest) Close() error {
	return nil
}

func newCodexProTrailerResponseForOpenAITest(body string, trailerAck string) *http.Response {
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Trailer: http.Header{}}
	resp.Body = &trailerOnEOFBodyForOpenAITest{
		reader: strings.NewReader(body),
		onEOF: func() {
			if trailerAck != "" {
				resp.Trailer.Set("X-NewAPI-Pro-Served", trailerAck)
			}
		},
	}
	return resp
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

func TestOaiResponsesHandlerCodexProServedAckRequiresTrailerAndSuccessfulUsage(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		requestMarker bool
		headerAck     string
		trailerAck    string
		body          string
		wantCandidate bool
		wantFinal     bool
	}{
		{name: "request_marker_and_exact_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`, wantCandidate: true, wantFinal: true},
		{name: "ordinary_header_ack_ignored", requestMarker: true, headerAck: "codex-pro", body: `{"status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "trailer_ack_without_request_marker", trailerAck: "codex-pro", body: `{"status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "missing_trailer_ack", requestMarker: true, body: `{"status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "wrong_trailer_ack_pro", requestMarker: true, trailerAck: "pro", body: `{"status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "wrong_trailer_ack_true", requestMarker: true, trailerAck: "true", body: `{"status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "wrong_trailer_ack_2x", requestMarker: true, trailerAck: "2x", body: `{"status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "empty_trailer_ack", requestMarker: true, trailerAck: "", body: `{"status":"completed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "incomplete_status_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"incomplete","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "failed_status_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"failed","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "cancelled_status_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"cancelled","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}`},
		{name: "parse_failure_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{not-json`},
		{name: "upstream_error_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"failed","error":{"type":"server_error","message":"upstream failed"},"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`},
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
			resp := newCodexProTrailerResponseForOpenAITest(tc.body, tc.trailerAck)
			if tc.headerAck != "" {
				resp.Header.Set("X-NewAPI-Pro-Served", tc.headerAck)
			}

			_, apiErr := OaiResponsesHandler(c, info, resp)

			if tc.name == "parse_failure_ignores_trailer_ack" || tc.name == "upstream_error_ignores_trailer_ack" {
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

func TestOaiResponsesCompactionHandlerCodexProServedAckRequiresTrailerAndSuccess(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		requestMarker bool
		headerAck     string
		trailerAck    string
		body          string
		wantCandidate bool
		wantFinal     bool
	}{
		{name: "success", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"completed","usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7},"output":[]}`, wantCandidate: true, wantFinal: true},
		{name: "ordinary_header_ack_ignored", requestMarker: true, headerAck: "codex-pro", body: `{"status":"completed","usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7},"output":[]}`},
		{name: "wrong_trailer_ack", requestMarker: true, trailerAck: "true", body: `{"status":"completed","usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7},"output":[]}`},
		{name: "incomplete_status_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"incomplete","usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7},"output":[]}`},
		{name: "failed_status_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"failed","usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7},"output":[]}`},
		{name: "cancelled_status_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"cancelled","usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7},"output":[]}`},
		{name: "parse_failure_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{bad-json`},
		{name: "upstream_error_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: `{"status":"failed","error":{"type":"server_error","message":"upstream failed"},"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7}}`},
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
			resp := newCodexProTrailerResponseForOpenAITest(tc.body, tc.trailerAck)
			if tc.headerAck != "" {
				resp.Header.Set("X-NewAPI-Pro-Served", tc.headerAck)
			}

			_, apiErr := OaiResponsesCompactionHandler(c, resp)

			if tc.name == "parse_failure_ignores_trailer_ack" || tc.name == "upstream_error_ignores_trailer_ack" {
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
		headerAck     string
		trailerAck    string
		body          string
		cancelBefore  bool
		wantCandidate bool
		wantFinal     bool
	}{
		{name: "completed", requestMarker: true, trailerAck: "codex-pro", body: strings.Join([]string{`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n"), wantCandidate: true, wantFinal: true},
		{name: "ordinary_header_ack_ignored", requestMarker: true, headerAck: "codex-pro", body: strings.Join([]string{`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n")},
		{name: "missing_completed", requestMarker: true, trailerAck: "codex-pro", body: strings.Join([]string{`data: {"type":"response.output_text.delta","delta":"hello"}`, ""}, "\n")},
		{name: "upstream_failed_event", requestMarker: true, trailerAck: "codex-pro", body: strings.Join([]string{`data: {"type":"response.failed","response":{"error":{"type":"server_error","message":"upstream failed"}}}`, "data: [DONE]", ""}, "\n")},
		{name: "upstream_incomplete_event", requestMarker: true, trailerAck: "codex-pro", body: strings.Join([]string{`data: {"type":"response.incomplete","response":{"id":"resp_incomplete","status":"incomplete","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`, "data: [DONE]", ""}, "\n")},
		{name: "upstream_cancelled_event", requestMarker: true, trailerAck: "codex-pro", body: strings.Join([]string{`data: {"type":"response.cancelled","response":{"id":"resp_cancelled","status":"cancelled","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`, "data: [DONE]", ""}, "\n")},
		{name: "failed_then_completed_does_not_ack", requestMarker: true, trailerAck: "codex-pro", body: strings.Join([]string{`data: {"type":"response.failed","response":{"error":{"type":"server_error","message":"upstream failed"}}}`, `data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n")},
		{name: "request_cancelled", requestMarker: true, trailerAck: "codex-pro", body: strings.Join([]string{`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n"), cancelBefore: true},
		{name: "wrong_trailer_ack", requestMarker: true, trailerAck: "pro", body: strings.Join([]string{`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n")},
		{name: "trailer_ack_without_request_marker", trailerAck: "codex-pro", body: strings.Join([]string{`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`, "data: [DONE]", ""}, "\n")},
		{name: "parse_failure_ignores_trailer_ack", requestMarker: true, trailerAck: "codex-pro", body: strings.Join([]string{`data: {not-json`, "data: [DONE]", ""}, "\n")},
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
			resp := newCodexProTrailerResponseForOpenAITest(tc.body, tc.trailerAck)
			if tc.headerAck != "" {
				resp.Header.Set("X-NewAPI-Pro-Served", tc.headerAck)
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
