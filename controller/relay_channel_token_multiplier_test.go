package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayMultiplierFreezeOccursBeforePreconsumeWithoutInitializingChannelMeta(t *testing.T) {
	ctx, _ := newRelayTokenLimitTestContext(t)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 97201)
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(ctx, constant.ContextKeyChannelTokenBillingMultiplier, 2.5)
	common.SetContextKey(ctx, constant.ContextKeyChannelCreditBillingMode, "fixed_request")
	common.SetContextKey(ctx, constant.ContextKeyChannelFixedRequestCredits, 80000)
	common.SetContextKey(ctx, constant.ContextKeyChannelDynamicBillingMultiplierEnabled, true)
	relayInfo := &relaycommon.RelayInfo{}

	require.NoError(t, relayInfo.FreezeChannelTokenBillingSnapshot(ctx))

	assert.Nil(t, relayInfo.ChannelMeta, "billing snapshot must not initialize final channel meta")
	assert.InDelta(t, 2.5, relayInfo.FrozenChannelTokenBillingMultiplier(), 1e-9)
	assert.Equal(t, "fixed_request", relayInfo.CreditBillingMode)
	assert.Equal(t, int64(80000), relayInfo.FixedRequestCredits)
	assert.True(t, relayInfo.DynamicBillingMultiplierEnabled)
	assert.Equal(t, 97201, relayInfo.InitialChannelId)
	assert.Equal(t, constant.ChannelTypeOpenAI, relayInfo.InitialChannelType)
}

func TestRetryMultiplierGetChannelPassesSameMultiplierAndUsedChannels(t *testing.T) {
	db := setupChannelTokenMultiplierControllerTestDB(t)
	const modelName = "gpt-retry-controller-multiplier"
	priority := int64(100)
	seedChannelTokenMultiplier(t, db, &model.Channel{Id: 97211, Type: constant.ChannelTypeOpenAI, Key: "sk-used", Status: common.ChannelStatusEnabled, Name: "used", Models: modelName, TokenBillingMultiplier: 2, CreditBillingMode: "fixed_request", FixedRequestCredits: 80000, DynamicBillingMultiplierEnabled: true, Priority: &priority})
	seedChannelTokenMultiplier(t, db, &model.Channel{Id: 97212, Type: constant.ChannelTypeOpenAI, Key: "sk-diff", Status: common.ChannelStatusEnabled, Name: "diff", Models: modelName, TokenBillingMultiplier: 2, CreditBillingMode: "usage_tokens", Priority: &priority})
	seedChannelTokenMultiplier(t, db, &model.Channel{Id: 97213, Type: constant.ChannelTypeOpenAI, Key: "sk-same", Status: common.ChannelStatusEnabled, Name: "same", Models: modelName, TokenBillingMultiplier: 2, CreditBillingMode: "fixed_request", FixedRequestCredits: 80000, DynamicBillingMultiplierEnabled: true, Priority: &priority})
	ctx, _ := newRelayTokenLimitTestContext(t)
	relayInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, OriginModelName: modelName, BillingSource: service.BillingSourceSubscription, SubscriptionDistributorTokenBilling: true}
	relayInfo.ChannelTokenBillingMultiplier = 2
	relayInfo.CreditBillingMode = "fixed_request"
	relayInfo.FixedRequestCredits = 80000
	relayInfo.DynamicBillingMultiplierEnabled = true
	addUsedChannel(ctx, 97211)
	retry := 0
	param := &service.RetryParam{Ctx: ctx, ModelName: relayInfo.OriginModelName, Retry: &retry}

	channel, apiErr := getChannel(ctx, relayInfo, param)

	require.Nil(t, apiErr)
	require.NotNil(t, channel)
	assert.Equal(t, 97213, channel.Id)
	assert.True(t, param.RequireSameTokenBillingMultiplier)
	assert.InDelta(t, 2, param.FrozenTokenBillingMultiplier, 1e-9)
	assert.True(t, param.RequireSameBillingProfile)
	assert.Equal(t, "fixed_request", param.FrozenBillingProfile.CreditBillingMode)
	assert.Equal(t, int64(80000), param.FrozenBillingProfile.FixedRequestCredits)
	assert.True(t, param.FrozenBillingProfile.DynamicBillingMultiplierEnabled)
	assert.Equal(t, []int{97211}, param.UsedChannelIds)
}

func TestRetryMultiplierNoSameMultiplierCandidatePreservesOriginalError(t *testing.T) {
	db := setupChannelTokenMultiplierControllerTestDB(t)
	const modelName = "gpt-retry-controller-no-same-multiplier"
	priority := int64(100)
	seedChannelTokenMultiplier(t, db, &model.Channel{Id: 97221, Type: constant.ChannelTypeOpenAI, Key: "sk-used-same", Status: common.ChannelStatusEnabled, Name: "used-same", Models: modelName, TokenBillingMultiplier: 2, Priority: &priority})
	seedChannelTokenMultiplier(t, db, &model.Channel{Id: 97222, Type: constant.ChannelTypeOpenAI, Key: "sk-unused-diff", Status: common.ChannelStatusEnabled, Name: "unused-diff", Models: modelName, TokenBillingMultiplier: 1, Priority: &priority})
	ctx, _ := newRelayTokenLimitTestContext(t)
	originalErr := types.NewOpenAIError(assert.AnError, types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	relayInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, OriginModelName: modelName, BillingSource: service.BillingSourceSubscription, SubscriptionDistributorTokenBilling: true, LastError: originalErr}
	relayInfo.ChannelTokenBillingMultiplier = 2
	addUsedChannel(ctx, 97221)
	retry := 0
	param := &service.RetryParam{Ctx: ctx, ModelName: relayInfo.OriginModelName, Retry: &retry}

	channel, channelErr := getChannel(ctx, relayInfo, param)

	require.Nil(t, channel)
	require.NotNil(t, channelErr)
	assert.Same(t, originalErr, retryChannelSelectionErrorForResponse(relayInfo, channelErr))
}

func TestRetryMultiplierNotRequiredForTaskRelay(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:                         types.RelayFormatTask,
		ChannelTokenBillingMultiplier:       2,
		BillingSource:                       service.BillingSourceSubscription,
		SubscriptionDistributorTokenBilling: true,
	}

	assert.False(t, shouldRequireSameTokenBillingMultiplier(relayInfo))
}

func TestRetryMultiplierRequiredForDistributorTokenBilling(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:                         types.RelayFormatOpenAI,
		ChannelTokenBillingMultiplier:       2,
		BillingSource:                       service.BillingSourceSubscription,
		SubscriptionDistributorTokenBilling: true,
	}

	assert.True(t, shouldRequireSameTokenBillingMultiplier(relayInfo))
}

func TestRetryBillingProfileRequiredForDistributorTokenBilling(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:                         types.RelayFormatOpenAI,
		ChannelTokenBillingMultiplier:       2,
		CreditBillingMode:                   "fixed_request",
		FixedRequestCredits:                 80000,
		DynamicBillingMultiplierEnabled:     true,
		BillingSource:                       service.BillingSourceSubscription,
		SubscriptionDistributorTokenBilling: true,
	}

	assert.True(t, shouldRequireSameBillingProfile(relayInfo))
	assert.Equal(t, "fixed_request", relayInfoBillingProfile(relayInfo).CreditBillingMode)
	assert.Equal(t, int64(80000), relayInfoBillingProfile(relayInfo).FixedRequestCredits)
	assert.True(t, relayInfoBillingProfile(relayInfo).DynamicBillingMultiplierEnabled)
}
