package relay

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/streamgate"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesWSControlErrorsDoNotFinishGeneration(t *testing.T) {
	control := []byte(`{"type":"response.cancel","event_id":"cancel_1","response_id":"previous"}`)
	for _, tc := range []struct {
		name                         string
		event                        responsesWSErrorEvent
		terminal, ambiguous, control bool
	}{
		{"correlated control", responsesWSErrorEvent{EventID: "cancel_1"}, false, false, true},
		{"previous response", responsesWSErrorEvent{ResponseID: "previous"}, false, false, true},
		{"other stream", responsesWSErrorEvent{StreamID: "other"}, false, false, false},
		{"generation", responsesWSErrorEvent{ResponseID: "current"}, true, false, false},
		{"ambiguous", responsesWSErrorEvent{}, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			terminal, ambiguous, controlError := responsesWSErrorEndsRequest(tc.event, "stream", "current", control)
			require.Equal(t, tc.terminal, terminal)
			require.Equal(t, tc.ambiguous, ambiguous)
			require.Equal(t, tc.control, controlError)
		})
	}
}

func TestResponsesWSGateRetriesPreludeWithSingleIdentityAndTerminal(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	gate := streamgate.New(c.Writer, string(types.RelayFormatOpenAIResponses))
	gate.BindResponseIdentity("resp_public", func(public, upstream string) error { return nil })
	gate.BeginAttempt()
	created, emit, err := gate.TransformEvent([]byte(`{"type":"response.created","sequence_number":0,"response":{"id":"a","output":[]}}`))
	require.NoError(t, err)
	require.True(t, emit)
	require.Equal(t, "resp_public", gjson.GetBytes(created, "response.id").String())
	_, emit, err = gate.TransformEvent([]byte(`{"type":"response.failed","sequence_number":1,"response":{"id":"a","error":{"type":"server_error","code":"server_error","message":"retry"}}}`))
	require.NoError(t, err)
	require.False(t, emit)
	require.True(t, gate.CanRetry())
	gate.BeginAttempt()
	_, emit, err = gate.TransformEvent([]byte(`{"type":"response.created","sequence_number":0,"response":{"id":"b","output":[]}}`))
	require.NoError(t, err)
	require.False(t, emit)
	delta, emit, err := gate.TransformEvent([]byte(`{"type":"response.output_text.delta","sequence_number":1,"delta":"hello"}`))
	require.NoError(t, err)
	require.True(t, emit)
	require.False(t, gate.CanRetry())
	terminal, emit, err := gate.TransformEvent([]byte(`{"type":"response.completed","sequence_number":2,"response":{"id":"b","output":[]}}`))
	require.NoError(t, err)
	require.True(t, emit)
	require.Equal(t, "resp_public", gjson.GetBytes(terminal, "response.id").String())
	require.Greater(t, gjson.GetBytes(terminal, "sequence_number").Int(), gjson.GetBytes(delta, "sequence_number").Int())
}

func TestResponsesWSUsageNeverEstimatesAndPreservesDetails(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus(), ChannelMeta: &relaycommon.ChannelMeta{}}
	delta := dto.ResponsesStreamResponse{Type: "response.output_text.delta", Delta: "not billable without upstream usage"}
	require.Nil(t, observeResponsesWSUsage(c, info, &delta, []byte(`{"type":"response.output_text.delta"}`), nil))
	completed := dto.ResponsesStreamResponse{Type: "response.completed", Response: &dto.OpenAIResponsesResponse{Usage: &dto.Usage{InputTokens: 20, OutputTokens: 5, TotalTokens: 25, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 7}}}}
	usage := observeResponsesWSUsage(c, info, &completed, []byte(`{"type":"response.completed"}`), nil)
	require.True(t, info.HasTrustedUsage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 5, usage.CompletionTokens)
	require.Equal(t, 7, usage.PromptTokensDetails.CachedTokens)
}

type responsesWSCountingBilling struct{ refunds int }

func (b *responsesWSCountingBilling) Settle(int) error            { return nil }
func (b *responsesWSCountingBilling) Refund(*gin.Context)         { b.refunds++ }
func (b *responsesWSCountingBilling) CommitPreConsumedOnFailure() {}
func (b *responsesWSCountingBilling) NeedsRefund() bool           { return true }
func (b *responsesWSCountingBilling) GetPreConsumedQuota() int    { return 1 }
func (b *responsesWSCountingBilling) Reserve(int) error           { return nil }

type responsesWSCountingLease struct{ released int }

func (l *responsesWSCountingLease) Release(context.Context) error { l.released++; return nil }

func TestResponsesWSBillingFinishExactlyOnce(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	funding := &responsesWSCountingBilling{}
	lease := &responsesWSCountingLease{}
	lifecycle := &responsesWSBilling{lease: lease}
	info := &relaycommon.RelayInfo{Billing: funding}
	lifecycle.finish(c, info)
	lifecycle.finish(c, info)
	require.Equal(t, 1, funding.refunds)
	require.Equal(t, 1, lease.released)
	settled := &responsesWSBilling{settled: true}
	settled.finish(c, info)
	require.Equal(t, 1, funding.refunds)
}

func TestResponsesWSNativeEnvelopeKeepsContinuationAndStreamIdentity(t *testing.T) {
	raw := []byte(`{"type":"response.create","stream_id":"stream_1","model":"gpt-4o","input":"hello","previous_response_id":"resp_public"}`)
	envelope, streamID, err := parseResponsesWSEnvelope(raw)
	require.NoError(t, err)
	create, err := normalizeResponsesWSCreateEvent(raw, envelope, streamID)
	require.NoError(t, err)
	require.Equal(t, "resp_public", create.Request.PreviousResponseID)
	payload, err := buildResponsesWSCreateEvent([]byte(`{"model":"mapped","previous_response_id":"upstream_exact","stream_id":"override"}`), json.RawMessage(`true`), streamID)
	require.NoError(t, err)
	require.Equal(t, "stream_1", gjson.GetBytes(payload, "stream_id").String())
	require.Equal(t, "upstream_exact", gjson.GetBytes(payload, "previous_response_id").String())
}

func TestResponsesWSLateTerminalDoesNotBelongToNextGeneration(t *testing.T) {
	session := &responsesWSSession{}
	session.recentResponseIDs[0] = "first"
	session.recentResponseIDs[1] = "second"
	require.True(t, session.isPreviousResponse("first"))
	require.True(t, session.isPreviousResponse("second"))
	require.False(t, session.isPreviousResponse("current"))
	require.False(t, session.isPreviousResponse(""))
}
