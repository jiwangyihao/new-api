package common

import (
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/pkg/streamgate"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
)

func TestStreamStatus_SetEndReason_FirstWins(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	s.SetEndReason(StreamEndReasonDone, nil)
	s.SetEndReason(StreamEndReasonTimeout, nil)
	s.SetEndReason(StreamEndReasonClientGone, fmt.Errorf("context canceled"))

	assert.Equal(t, StreamEndReasonDone, s.EndReason)
	assert.Nil(t, s.EndError)
}

func TestStreamStatus_SetEndReason_WithError(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	expectedErr := fmt.Errorf("read: connection reset")
	s.SetEndReason(StreamEndReasonScannerErr, expectedErr)

	assert.Equal(t, StreamEndReasonScannerErr, s.EndReason)
	assert.Equal(t, expectedErr, s.EndError)
}

func TestStreamStatus_SetEndReason_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	s.SetEndReason(StreamEndReasonDone, nil)
}

func TestStreamStatus_SetEndReason_Concurrent(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	reasons := []StreamEndReason{
		StreamEndReasonDone,
		StreamEndReasonTimeout,
		StreamEndReasonClientGone,
		StreamEndReasonScannerErr,
		StreamEndReasonHandlerStop,
		StreamEndReasonEOF,
		StreamEndReasonPanic,
		StreamEndReasonPingFail,
	}

	var wg sync.WaitGroup
	for _, r := range reasons {
		wg.Add(1)
		go func(reason StreamEndReason) {
			defer wg.Done()
			s.SetEndReason(reason, nil)
		}(r)
	}
	wg.Wait()

	assert.NotEqual(t, StreamEndReasonNone, s.EndReason)
}

func TestStreamStatus_RecordError_Basic(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	s.RecordError("bad json")
	s.RecordError("another bad json")
	s.RecordError("client gone")

	assert.True(t, s.HasErrors())
	assert.Equal(t, 3, s.TotalErrorCount())
	assert.Len(t, s.Errors, 3)
}

func TestStreamStatus_RecordError_CapAtMax(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	for i := 0; i < 30; i++ {
		s.RecordError(fmt.Sprintf("error_%d", i))
	}

	assert.Equal(t, maxStreamErrorEntries, len(s.Errors))
	assert.Equal(t, 30, s.TotalErrorCount())
}

func TestStreamStatus_RecordError_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	s.RecordError("should not panic")
}

func TestStreamStatus_RecordError_Concurrent(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.RecordError(fmt.Sprintf("error_%d", idx))
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 100, s.TotalErrorCount())
	assert.LessOrEqual(t, len(s.Errors), maxStreamErrorEntries)
}

func TestStreamStatus_HasErrors_Empty(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()
	assert.False(t, s.HasErrors())
	assert.Equal(t, 0, s.TotalErrorCount())
}

func TestStreamStatus_HasErrors_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.False(t, s.HasErrors())
	assert.Equal(t, 0, s.TotalErrorCount())
}

func TestStreamStatus_IsNormalEnd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reason StreamEndReason
		normal bool
	}{
		{StreamEndReasonDone, true},
		{StreamEndReasonEOF, false},
		{StreamEndReasonHandlerStop, true},
		{StreamEndReasonTimeout, false},
		{StreamEndReasonClientGone, false},
		{StreamEndReasonScannerErr, false},
		{StreamEndReasonPanic, false},
		{StreamEndReasonPingFail, false},
		{StreamEndReasonNone, false},
	}
	for _, tt := range tests {
		s := NewStreamStatus()
		s.SetEndReason(tt.reason, nil)
		assert.Equal(t, tt.normal, s.IsNormalEnd(), "reason=%s", tt.reason)
	}
}

func TestStreamStatus_IsNormalEnd_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.True(t, s.IsNormalEnd())
}

func TestStreamStatus_Summary(t *testing.T) {
	t.Parallel()

	s := NewStreamStatus()
	s.SetEndReason(StreamEndReasonDone, nil)
	summary := s.Summary()
	assert.Contains(t, summary, "reason=done")
	assert.NotContains(t, summary, "soft_errors")

	s2 := NewStreamStatus()
	s2.SetEndReason(StreamEndReasonTimeout, nil)
	s2.RecordError("bad json")
	s2.RecordError("write failed")
	summary2 := s2.Summary()
	assert.Contains(t, summary2, "reason=timeout")
	assert.Contains(t, summary2, "soft_errors=2")
}

func TestStreamStatus_Summary_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.Equal(t, "StreamStatus<nil>", s.Summary())
}

func TestStreamStatus_ConcurrentCompletionAndQueries(t *testing.T) {
	for _, reason := range []StreamEndReason{StreamEndReasonDone, StreamEndReasonHandlerStop} {
		t.Run(string(reason), func(t *testing.T) {
			const count = 128
			statuses := make([]*StreamStatus, count)
			for i := range statuses {
				statuses[i] = NewStreamStatus()
			}
			var endErr error
			if reason == StreamEndReasonHandlerStop {
				endErr = fmt.Errorf("downstream write failed")
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(3)
			go func() {
				defer wg.Done()
				<-start
				for _, status := range statuses {
					status.FinalizeEOF()
					status.SetEndReason(reason, endErr)
				}
			}()
			for i := 0; i < 2; i++ {
				go func() {
					defer wg.Done()
					<-start
					for _, status := range statuses {
						status.IsNormalEnd()
						status.Summary()
					}
				}()
			}
			close(start)
			wg.Wait()
			for _, status := range statuses {
				assert.True(t, status.IsNormalEnd())
				assert.Contains(t, status.Summary(), "reason="+string(reason))
				if endErr != nil {
					assert.Contains(t, status.Summary(), `end_error="downstream write failed"`)
					assert.Equal(t, 1, status.TotalErrorCount())
				} else {
					assert.NotContains(t, status.Summary(), "end_error")
					assert.Zero(t, status.TotalErrorCount())
				}
			}
		})
	}
}

func TestStreamStatusProtocolOutcomePreservesTransportEvidence(t *testing.T) {
	s := NewStreamStatus()
	s.RequireTerminal()
	s.FinalizeEOF()
	s.MarkCompleted()
	assert.Equal(t, ResponseOutcomeCompleted, s.OutcomeSnapshot().Response)
	assert.True(t, s.OutcomeSnapshot().ExpectsTerminal)
	assert.True(t, s.DrainedToEOF)
	assert.False(t, s.Completed, "protocol outcome does not overwrite transport evidence")
	s.SetEndReason(StreamEndReasonDone, nil)
	assert.True(t, s.Completed)
	assert.Equal(t, StreamEndReasonDone, s.EndReason)
	s.MarkFailed("overloaded", "server_error", 503)
	s.MarkFailed("", "", 0)
	s.MarkCompleted()
	outcome := s.OutcomeSnapshot()
	assert.Equal(t, ResponseOutcomeFailed, outcome.Response)
	assert.Equal(t, "overloaded", outcome.ErrorCode)
	assert.Equal(t, "server_error", outcome.ErrorType)
	assert.Equal(t, 503, outcome.ErrorStatus)
	assert.True(t, s.Completed, "protocol failure must not erase transport completion")
	s.MarkAvailabilityIncomplete()
	assert.True(t, s.AvailabilityIncomplete())
}

func TestInitChannelMetaResetsAttemptButPreservesBillingReservation(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	g := streamgate.New(c.Writer, string(types.RelayFormatOpenAI))
	cleanup := func() {}
	info := &RelayInfo{
		RelayFormat:                     types.RelayFormatOpenAI,
		StreamGate:                      g,
		StreamStatus:                    NewStreamStatus(),
		StreamRequestCleanup:            cleanup,
		RequestConversionChain:          []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses},
		FinalRequestRelayFormat:         types.RelayFormatOpenAIResponses,
		ClaudeConvertInfo:               &ClaudeConvertInfo{Done: true, Index: 7},
		ThinkingContentInfo:             ThinkingContentInfo{HasSentThinkingContent: true},
		CreditBillingMode:               "fixed_request",
		ChannelTokenBillingMultiplier:   2,
		FixedRequestCredits:             80,
		InitialChannelId:                12,
		DynamicBillingMultiplierEnabled: true,
		FinalPreConsumedQuota:           80,
		SubscriptionId:                  13,
		SubscriptionPreConsumed:         80,
		SubscriptionTokenLimit:          1000,
		TokenGroups:                     []string{"actual"},
		CodexProServed:                  true,
		CodexProRequestSent:             true,
	}
	info.StreamStatus.MarkFailed("busy", "server_error", 503)
	info.InitChannelMeta(c)
	assert.Nil(t, info.StreamStatus)
	assert.Same(t, g, info.StreamGate)
	assert.NotNil(t, info.StreamRequestCleanup)
	assert.Equal(t, []types.RelayFormat{types.RelayFormatOpenAI}, info.RequestConversionChain)
	assert.Empty(t, info.FinalRequestRelayFormat)
	assert.Nil(t, info.ClaudeConvertInfo)
	assert.True(t, info.IsFirstThinkingContent)
	assert.False(t, info.HasSentThinkingContent)
	assert.False(t, info.CodexProServed)
	assert.False(t, info.CodexProRequestSent)
	assert.Equal(t, "fixed_request", info.CreditBillingMode)
	assert.Equal(t, float64(2), info.ChannelTokenBillingMultiplier)
	assert.Equal(t, int64(80), info.FixedRequestCredits)
	assert.Equal(t, 12, info.InitialChannelId)
	assert.True(t, info.DynamicBillingMultiplierEnabled)
	assert.Equal(t, 80, info.FinalPreConsumedQuota)
	assert.Equal(t, 13, info.SubscriptionId)
	assert.Equal(t, int64(80), info.SubscriptionPreConsumed)
	assert.Equal(t, int64(1000), info.SubscriptionTokenLimit)
	assert.Equal(t, []string{"actual"}, info.TokenGroups)
}
