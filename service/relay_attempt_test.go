package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/creditbilling"
	"github.com/QuantumNous/new-api/pkg/streamgate"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func attachSettlementTestGate(ctx *gin.Context, info *relaycommon.RelayInfo) {
	info.RelayFormat = types.RelayFormatOpenAIResponses
	info.RelayMode = relayconstant.RelayModeResponses
	info.IsStream = true
	info.StreamGate = streamgate.New(ctx.Writer, string(types.RelayFormatOpenAIResponses))
	ctx.Writer = info.StreamGate
	info.StreamGate.Header().Set("Content-Type", "text/event-stream")
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RequireTerminal()
}

func writeSettlementTestEvent(t *testing.T, info *relaycommon.RelayInfo, event string) {
	t.Helper()
	_, err := info.StreamGate.WriteString("data: " + event + "\n\n")
	require.NoError(t, err)
}

func consumeLogCountForAttemptTest(t *testing.T, userID int) int64 {
	t.Helper()
	model.FlushConsumeLogUpdates()
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", userID, model.LogTypeConsume).Count(&count).Error)
	return count
}

func TestRelaySettlementFailedPreludeKeepsReservationUntilOneSuccessfulCharge(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 98751, 98752, 98753, 98754, 98755
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-stream-retry-credit", 1000, 0)
	seedReadyCreditValuationForServiceTest(t, userID, subID, 1000)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{"token_limit_enabled": true, "token_limit": int64(100), "token_used": int64(0)}).Error)
	ctx := newBillingTestContext(t)
	info := newBillingTestRelayInfo(userID, tokenID, "sk-stream-retry-credit", "req-stream-retry-credit", "subscription_only")
	freezeCreditBillingForServiceTest(t, ctx, info, channelID, creditbilling.ModeFixedRequest, 80, false)
	info.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, info, 999)
	info.TokenLimit = NewTokenLimitSession(info)
	require.Nil(t, info.TokenLimit.PreConsume(info.SubscriptionPreConsumedTokens()))
	attachSettlementTestGate(ctx, info)
	writeSettlementTestEvent(t, info, `{"type":"response.created","response":{"id":"A","output":[]}}`)
	writeSettlementTestEvent(t, info, `{"type":"response.failed","response":{"error":{"code":"overloaded","type":"server_error"}}}`)
	info.StreamStatus.MarkFailed("overloaded", "server_error", 503)
	require.NoError(t, PostTextConsumeQuota(ctx, info, nil, nil))
	PostAudioConsumeQuota(ctx, info, nil, "")
	require.True(t, info.Billing.NeedsRefund())
	require.False(t, RelayOutputCommitted(info))
	require.True(t, info.StreamGate.CanRetry())
	require.Equal(t, int64(80), getSubscriptionTokenUsed(t, subID))
	require.Equal(t, int64(80), getTokenUsed(t, tokenID))
	require.Equal(t, int64(0), consumeLogCountForAttemptTest(t, userID))
	billing, tokenLimit := info.Billing, info.TokenLimit
	info.InitChannelMeta(ctx)
	require.Same(t, billing, info.Billing)
	require.Same(t, tokenLimit, info.TokenLimit)
	require.Equal(t, int64(80), info.FixedRequestCredits)
	info.StreamGate.BeginAttempt()
	info.StreamStatus = relaycommon.NewStreamStatus()
	writeSettlementTestEvent(t, info, `{"type":"response.created","response":{"id":"B","output":[]}}`)
	writeSettlementTestEvent(t, info, `{"type":"response.output_text.delta","delta":"x"}`)
	writeSettlementTestEvent(t, info, `{"type":"response.completed","response":{"id":"B","output":[]}}`)
	info.StreamStatus.MarkCompleted()
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	info.StreamStatus.FinalizeEOF()
	require.NoError(t, PostTextConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4}, nil))
	model.FlushSubscriptionTokenDeltaUpdates()
	require.Nil(t, StreamAttemptError(info))
	require.False(t, info.Billing.NeedsRefund())
	require.Equal(t, int64(80), getSubscriptionTokenUsed(t, subID))
	require.Equal(t, int64(80), getTokenUsed(t, tokenID))
	require.Equal(t, int64(1), consumeLogCountForAttemptTest(t, userID))
	require.Equal(t, int64(4), info.RawMeteredTokens)
	require.Equal(t, int64(80), info.SubscriptionBillableTokens)
	require.Equal(t, int64(80), info.ApiKeyBillableTokens)
	record := getTokenLimitRecordForTest(t, "req-stream-retry-credit")
	require.Equal(t, model.TokenLimitPreConsumeStatusSettled, record.Status)
	require.Equal(t, int64(80), record.ActualTokens)
}

func TestRelaySettlementSingleCharacterFailureChargesTrustedProductionCreditsWithoutSuccess(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 98761, 98762, 98763, 98764, 98765
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-stream-partial-credit", 1000, 0)
	ctx := newBillingTestContext(t)
	info := newBillingTestRelayInfo(userID, tokenID, "sk-stream-partial-credit", "req-stream-partial-credit", "subscription_only")
	freezeCreditBillingForServiceTest(t, ctx, info, channelID, creditbilling.ModeFixedRequest, 80, false)
	info.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, info, 999)
	attachSettlementTestGate(ctx, info)
	writeSettlementTestEvent(t, info, `{"type":"response.output_text.delta","delta":"x"}`)
	writeSettlementTestEvent(t, info, `{"type":"response.failed","response":{"error":{"code":"broken","type":"server_error"}}}`)
	info.StreamStatus.MarkFailed("broken", "server_error", 502)
	require.True(t, RelayOutputCommitted(info))
	require.False(t, info.StreamGate.CanRetry())
	require.NoError(t, PostTextConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4}, nil))
	model.FlushSubscriptionTokenDeltaUpdates()
	require.Equal(t, int64(80), getSubscriptionTokenUsed(t, subID))
	require.Equal(t, int64(4), info.RawMeteredTokens)
	require.Equal(t, int64(1), consumeLogCountForAttemptTest(t, userID))
	require.False(t, relaySettlementSucceeded(ctx, info))
	result, _ := ClassifyAvailabilityResult(StreamAttemptError(info), info.StreamStatus, true, false)
	require.Equal(t, model.AvailabilityFailure, result)
}

func TestRelaySettlementCancellationAndModerationAreExcluded(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		info := &relaycommon.RelayInfo{}
		attachSettlementTestGate(ctx, info)
		if cancel {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, context.Canceled)
		} else {
			writeSettlementTestEvent(t, info, `{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"content_filter"}}}`)
			info.StreamStatus.MarkFailed("content_filter", "invalid_request_error", 400)
		}
		require.False(t, PrepareRelaySettlement(ctx, info))
		require.False(t, info.StreamGate.CanRetry())
		result, _ := ClassifyAvailabilityResult(StreamAttemptError(info), info.StreamStatus, true, cancel)
		require.Equal(t, model.AvailabilityExcluded, result)
		require.False(t, relaySettlementSucceeded(ctx, info))
	}
}
