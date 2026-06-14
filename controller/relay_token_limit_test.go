package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRelayTokenLimitTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return ctx, recorder
}

type relayTokenLimitFake struct {
	refundReasons []string
	markReasons   []string
}

func (f *relayTokenLimitFake) PreConsume(tokens int64) *types.NewAPIError { return nil }
func (f *relayTokenLimitFake) Settle(actualTokens int64) error            { return nil }
func (f *relayTokenLimitFake) SettleForAudit(actualTokens int64, reason string) error {
	return f.MarkSettleFailed(actualTokens, reason)
}
func (f *relayTokenLimitFake) MarkSettleFailed(actualTokens int64, reason string) error {
	f.markReasons = append(f.markReasons, reason)
	return nil
}
func (f *relayTokenLimitFake) Refund(reason string) {
	f.refundReasons = append(f.refundReasons, reason)
}
func (f *relayTokenLimitFake) ConsumeIncrement(tokens int64) (int64, *types.NewAPIError) {
	return 0, nil
}
func (f *relayTokenLimitFake) RefundIncrement(sequence int64, reason string) {}
func (f *relayTokenLimitFake) CommitIncrement(sequence int64) {}
func (f *relayTokenLimitFake) PreConsumedTokens() int64                      { return 10 }

type relayBillingFake struct {
	refundCount int
	commitCount int
}

func (f *relayBillingFake) Settle(actualQuota int) error  { return nil }
func (f *relayBillingFake) Refund(c *gin.Context)         { f.refundCount++ }
func (f *relayBillingFake) CommitPreConsumedOnFailure()   { f.commitCount++ }
func (f *relayBillingFake) NeedsRefund() bool             { return true }
func (f *relayBillingFake) GetPreConsumedQuota() int      { return 10 }
func (f *relayBillingFake) Reserve(targetQuota int) error { return nil }

func newRelayTokenLimitInfo(fake *relayTokenLimitFake, billing *relayBillingFake) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		StartTime:         httptestNewTimeForRelayTokenLimit(),
		FirstResponseTime: httptestNewTimeForRelayTokenLimit().Add(-1),
		TokenLimit:        fake,
		Billing:           billing,
	}
}

func httptestNewTimeForRelayTokenLimit() time.Time { return time.Unix(1700000000, 0) }

func TestRelayFailureAfterTokenLimitPreConsumeRefundsKeyCap(t *testing.T) {
	ctx, _ := newRelayTokenLimitTestContext(t)
	tokenLimit := &relayTokenLimitFake{}
	billing := &relayBillingFake{}
	relayInfo := newRelayTokenLimitInfo(tokenLimit, billing)
	origin := types.NewError(errors.New("channel failed"), types.ErrorCodeGetChannelFailed)

	result := handleRelayErrorForTokenLimit(ctx, relayInfo, origin)

	require.NotNil(t, result)
	assert.Equal(t, 1, billing.refundCount)
	assert.Equal(t, []string{string(types.ErrorCodeGetChannelFailed)}, tokenLimit.refundReasons)
	assert.Empty(t, tokenLimit.markReasons)
}

func TestRelayErrorAfterResponseAuditsWithoutRefundOrSecondWrite(t *testing.T) {
	ctx, recorder := newRelayTokenLimitTestContext(t)
	tokenLimit := &relayTokenLimitFake{}
	billing := &relayBillingFake{}
	relayInfo := newRelayTokenLimitInfo(tokenLimit, billing)
	relayInfo.FirstResponseTime = relayInfo.StartTime.Add(time.Second)
	ctx.Writer.WriteHeader(http.StatusOK)
	origin := types.NewError(errors.New("late failure"), types.ErrorCodeDoRequestFailed)

	result := handleRelayErrorForTokenLimit(ctx, relayInfo, origin)
	wrote := writeRelayErrorResponse(ctx, types.RelayFormatOpenAI, nil, "req-after", relayInfo, result)

	require.Nil(t, result)
	assert.False(t, wrote)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, 0, billing.refundCount)
	assert.Empty(t, tokenLimit.refundReasons)
	assert.Equal(t, []string{"error_after_response"}, tokenLimit.markReasons)
}

func TestRelayTokenLimitErrorAfterResponsePreservedForRealtimeWriter(t *testing.T) {
	ctx, _ := newRelayTokenLimitTestContext(t)
	tokenLimit := &relayTokenLimitFake{}
	billing := &relayBillingFake{}
	relayInfo := newRelayTokenLimitInfo(tokenLimit, billing)
	relayInfo.FirstResponseTime = relayInfo.StartTime.Add(time.Second)
	ctx.Writer.WriteHeader(http.StatusOK)
	origin := types.NewOpenAIError(errors.New("api key token limit exhausted"), types.ErrorCodeAPIKeyTokenLimitExhausted, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())

	result := handleRelayErrorForTokenLimit(ctx, relayInfo, origin)

	require.NotNil(t, result)
	assert.Equal(t, types.ErrorCodeAPIKeyTokenLimitExhausted, result.GetErrorCode())
	assert.Equal(t, 0, billing.refundCount)
	assert.Empty(t, tokenLimit.refundReasons)
	assert.Equal(t, []string{"error_after_response"}, tokenLimit.markReasons)
}

func TestRelayRealtimeTokenLimitErrorAfterResponseWritesWebSocketError(t *testing.T) {
	written := make(chan bool, 1)
	serverErr := make(chan error, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer ws.Close()

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = r
		ctx.Set(common.RequestIdKey, "req-wss-token-limit")
		ctx.Writer.WriteHeader(http.StatusOK)
		relayInfo := newRelayTokenLimitInfo(&relayTokenLimitFake{}, &relayBillingFake{})
		origin := types.NewOpenAIError(errors.New("api key token limit exhausted"), types.ErrorCodeAPIKeyTokenLimitExhausted, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
		written <- writeRelayErrorResponse(ctx, types.RelayFormatOpenAIRealtime, ws, "req-wss-token-limit", relayInfo, origin)
	}))
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer client.Close()

	select {
	case err := <-serverErr:
		require.NoError(t, err)
	case ok := <-written:
		require.True(t, ok)
	}
	require.NoError(t, client.SetReadDeadline(time.Now().Add(time.Second)))
	_, payload, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(payload), string(types.ErrorCodeAPIKeyTokenLimitExhausted))
}

func TestRelayRegistersTokenLimitCleanupBeforePreConsume(t *testing.T) {
	source, err := os.ReadFile("relay.go")
	require.NoError(t, err)
	text := string(source)
	cleanupIndex := strings.Index(text, "handleRelayPanicForTokenLimit")
	preconsumeIndex := strings.Index(text, "relayInfo.TokenLimit = service.NewTokenLimitSession(relayInfo)")
	require.NotEqual(t, -1, cleanupIndex)
	require.NotEqual(t, -1, preconsumeIndex)
	assert.Less(t, cleanupIndex, preconsumeIndex)
}

func TestRelayPanicAfterTokenLimitPreConsumeRefundsKeyCap(t *testing.T) {
	ctx, _ := newRelayTokenLimitTestContext(t)
	tokenLimit := &relayTokenLimitFake{}
	billing := &relayBillingFake{}
	relayInfo := newRelayTokenLimitInfo(tokenLimit, billing)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		handleRelayPanicForTokenLimit(ctx, relayInfo, "boom")
	}()

	assert.Equal(t, "boom", recovered)
	assert.Equal(t, 1, billing.refundCount)
	assert.Equal(t, []string{"panic"}, tokenLimit.refundReasons)
	assert.Empty(t, tokenLimit.markReasons)
}

func TestRelayPanicAfterResponseAuditsWithoutRefund(t *testing.T) {
	ctx, _ := newRelayTokenLimitTestContext(t)
	tokenLimit := &relayTokenLimitFake{}
	billing := &relayBillingFake{}
	relayInfo := newRelayTokenLimitInfo(tokenLimit, billing)
	relayInfo.FirstResponseTime = relayInfo.StartTime.Add(time.Second)
	ctx.Writer.WriteHeader(http.StatusOK)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		handleRelayPanicForTokenLimit(ctx, relayInfo, "boom-after")
	}()

	assert.Equal(t, "boom-after", recovered)
	assert.Equal(t, 0, billing.refundCount)
	assert.Empty(t, tokenLimit.refundReasons)
	assert.Equal(t, []string{"panic_after_response"}, tokenLimit.markReasons)
}

func TestRelayClientGoneBeforeResponseRefundsKeyCap(t *testing.T) {
	ctx, _ := newRelayTokenLimitTestContext(t)
	reqCtx, cancel := context.WithCancel(ctx.Request.Context())
	cancel()
	ctx.Request = ctx.Request.WithContext(reqCtx)
	tokenLimit := &relayTokenLimitFake{}
	billing := &relayBillingFake{}
	relayInfo := newRelayTokenLimitInfo(tokenLimit, billing)

	handleRelayClientGoneForTokenLimit(ctx, relayInfo)

	assert.Equal(t, 1, billing.refundCount)
	assert.Equal(t, []string{"client_gone_before_response"}, tokenLimit.refundReasons)
	assert.Empty(t, tokenLimit.markReasons)
}

func TestRelayStreamingClientGoneBeforeFirstChunkRefundsKeyCap(t *testing.T) {
	ctx, _ := newRelayTokenLimitTestContext(t)
	tokenLimit := &relayTokenLimitFake{}
	reqCtx, cancel := context.WithCancel(ctx.Request.Context())
	cancel()
	ctx.Request = ctx.Request.WithContext(reqCtx)
	billing := &relayBillingFake{}
	relayInfo := newRelayTokenLimitInfo(tokenLimit, billing)
	relayInfo.IsStream = true

	handleRelayClientGoneForTokenLimit(ctx, relayInfo)

	assert.Equal(t, 1, billing.refundCount)
	assert.Equal(t, []string{"client_gone_before_response"}, tokenLimit.refundReasons)
	assert.Empty(t, tokenLimit.markReasons)
}

func TestRelayClientGoneAfterResponseAuditsKeyCap(t *testing.T) {
	ctx, _ := newRelayTokenLimitTestContext(t)
	tokenLimit := &relayTokenLimitFake{}
	billing := &relayBillingFake{}
	reqCtx, cancel := context.WithCancel(ctx.Request.Context())
	cancel()
	ctx.Request = ctx.Request.WithContext(reqCtx)
	relayInfo := newRelayTokenLimitInfo(tokenLimit, billing)
	relayInfo.FirstResponseTime = relayInfo.StartTime.Add(time.Second)
	ctx.Writer.WriteHeader(http.StatusOK)

	handleRelayClientGoneForTokenLimit(ctx, relayInfo)

	assert.Equal(t, 0, billing.refundCount)
	assert.Empty(t, tokenLimit.refundReasons)
	assert.Equal(t, []string{"client_gone_after_response"}, tokenLimit.markReasons)
}
