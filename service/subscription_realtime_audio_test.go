package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostWssConsumeQuotaPreservesRealtimePreconsumeOnSuccess(t *testing.T) {
	truncate(t)
	const userID = 8151
	const tokenID = 8152
	const planID = 8153
	const subID = 8154
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-realtime-final", 10_000)
	seedDistributorPlan(t, planID, "plan-realtime-final", 100)
	seedDistributorSubscription(t, subID, userID, planID, 100, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-realtime-final", "req-realtime-final", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	relayInfo.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx, relayInfo, 1)

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 4}))
	require.NoError(t, PostWssConsumeQuota(ctx, relayInfo, relayInfo.OriginModelName, &dto.RealtimeUsage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4}, ""))

	assert.Equal(t, int64(4), getSubscriptionTokenUsed(t, subID))
}

func TestRealtimeRefundReturnsInitialPreconsumeAfterChunkSettlementFailure(t *testing.T) {
	truncate(t)
	const userID = 8161
	const tokenID = 8162
	const planID = 8163
	const subID = 8164
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-realtime-refund", 10_000)
	seedDistributorPlan(t, planID, "plan-realtime-refund", 5)
	seedDistributorSubscription(t, subID, userID, planID, 5, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-realtime-refund", "req-realtime-refund", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	relayInfo.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx, relayInfo, 1)

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 3}))
	err := PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 2})
	require.Error(t, err)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, types.ErrorCodeSubscriptionTokenExhausted, apiErr.GetErrorCode())
	relayInfo.Billing.Refund(ctx)
	require.Eventually(t, func() bool {
		var sub model.UserSubscription
		require.NoError(t, model.DB.Select("token_used").Where("id = ?", subID).First(&sub).Error)
		return sub.TokenUsed == 0
	}, time.Second, 10*time.Millisecond)
}

func TestPostTextConsumeQuotaSettlesAudioChatWithSubscriptionTokens(t *testing.T) {
	truncate(t)
	const userID = 8181
	const tokenID = 8182
	const planID = 8183
	const subID = 8184
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-audio-chat", 10_000)
	seedDistributorPlan(t, planID, "plan-audio-chat", 100)
	seedDistributorSubscription(t, subID, userID, planID, 100, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audio-chat", "req-audio-chat", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeChatCompletions
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	usage := &dto.Usage{
		PromptTokens:           4,
		CompletionTokens:       8,
		TotalTokens:            12,
		PromptTokensDetails:    dto.InputTokenDetails{AudioTokens: 3},
		CompletionTokenDetails: dto.OutputTokenDetails{AudioTokens: 5},
	}
	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, usage, nil))

	assert.Equal(t, int64(20), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "audio chat subscription settlement must not consume token key quota")
}

func TestPostSettleErrorToOpenAIErrorPreventsRefundAfterDeliveredResponse(t *testing.T) {
	truncate(t)
	const userID = 8191
	const tokenID = 8192
	const planID = 8193
	const subID = 8194
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-post-settle-lock", 10_000)
	seedDistributorPlan(t, planID, "plan-post-settle-lock", 6)
	seedDistributorSubscription(t, subID, userID, planID, 6, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-post-settle-lock", "req-post-settle-lock", "subscription_only")
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	settleErr := PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6, CompletionTokens: 1, TotalTokens: 7}, nil)
	require.Error(t, settleErr)
	apiErr := PostSettleErrorToOpenAIError(relayInfo, settleErr)
	require.NotNil(t, apiErr)
	relayInfo.Billing.Refund(ctx)
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, int64(6), getSubscriptionTokenUsed(t, subID))
}
