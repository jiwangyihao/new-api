package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/creditbilling"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func freezeCreditBillingForServiceTest(t *testing.T, ctx *gin.Context, relayInfo *relaycommon.RelayInfo, channelID int, mode string, fixedCredits int64, dynamicEnabled bool) {
	t.Helper()
	common.SetContextKey(ctx, constant.ContextKeyChannelId, channelID)
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(ctx, constant.ContextKeyChannelTokenBillingMultiplier, 1.0)
	common.SetContextKey(ctx, constant.ContextKeyChannelCreditBillingMode, mode)
	common.SetContextKey(ctx, constant.ContextKeyChannelFixedRequestCredits, fixedCredits)
	common.SetContextKey(ctx, constant.ContextKeyChannelDynamicBillingMultiplierEnabled, dynamicEnabled)
	require.NoError(t, relayInfo.FreezeChannelTokenBillingSnapshot(ctx))
	relayInfo.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: channelID, ChannelType: constant.ChannelTypeOpenAI}
	relayInfo.ChannelId = channelID
	relayInfo.ChannelType = constant.ChannelTypeOpenAI
}

func seedCreditBillingRuntime(t *testing.T, userID, tokenID, planID, subID, channelID int, tokenKey string, subLimit, subUsed int64) {
	t.Helper()
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, tokenKey, subLimit, subUsed)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", planID).Updates(map[string]any{
		"entitlement_type":                 model.SubscriptionEntitlementCreditBalance,
		"credit_balance_configured":        true,
		"credit_balance_purchase_enabled":  false,
		"credit_balance_redemption_enabled": false,
		"credit_balance_conversion_enabled": false,
	}).Error)
	model.InvalidateSubscriptionPlanCache(planID)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", subID).Updates(map[string]any{
		"entitlement_type": model.SubscriptionEntitlementCreditBalance,
		"end_time":         0,
	}).Error)
	seedChannel(t, channelID)
}

func TestTextFixedRequestChargesConfiguredCreditsOnce(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97501, 97502, 97503, 97504, 97505
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-fixed-text", 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-fixed-text", "req-credit-fixed-text", "subscription_only")
	freezeCreditBillingForServiceTest(t, ctx, relayInfo, channelID, creditbilling.ModeFixedRequest, 80, false)
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))
	model.FlushSubscriptionTokenDeltaUpdates()
	model.FlushConsumeLogUpdates()

	assert.Equal(t, int64(80), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(8), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(80), relayInfo.CreditBillingBaseCredits)
	assert.Equal(t, int64(80), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(80), relayInfo.SubscriptionBillableTokens)
	log := getLastLog(t)
	require.NotNil(t, log)
	require.NotNil(t, log.MeteredTokens)
	assert.Equal(t, 8, *log.MeteredTokens)
	assert.Equal(t, 8, log.Quota, "logs.quota remains legacy quota/cost, not fixed-request credits")
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, creditbilling.ModeFixedRequest, other["credit_billing_mode"])
	assert.Equal(t, float64(80), other["fixed_request_credits"])
	assert.Equal(t, true, other["has_trusted_usage"])
	assert.Equal(t, float64(8), other["raw_metered_tokens"])
	assert.Equal(t, float64(80), other["base_credits"])
	assert.Equal(t, float64(80), other["api_key_credits"])
	assert.Equal(t, float64(80), other["subscription_credits"])
	assert.Equal(t, float64(80), other["final_credits"])
	assert.Equal(t, float64(80), other["api_key_credits_consumed"])
	assert.Equal(t, float64(80), other["subscription_credits_consumed"])
}

func TestTextFixedRequestNoTrustedUsageRefundsPreconsume(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97511, 97512, 97513, 97514, 97515
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-fixed-missing", 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-fixed-missing", "req-credit-fixed-missing", "subscription_only")
	freezeCreditBillingForServiceTest(t, ctx, relayInfo, channelID, creditbilling.ModeFixedRequest, 80, false)
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, nil, nil))
	model.FlushSubscriptionTokenDeltaUpdates()
	model.FlushConsumeLogUpdates()

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.False(t, relayInfo.HasTrustedUsage)
	assert.Equal(t, int64(0), relayInfo.SubscriptionBillableTokens)
	assert.Equal(t, creditbilling.ZeroReasonNoTrustedUsage, relayInfo.CreditBillingZeroReason)
	other, err := common.StrToMap(getLastLog(t).Other)
	require.NoError(t, err)
	assert.Equal(t, true, other["usage_unavailable"])
	assert.Equal(t, false, other["has_trusted_usage"])
	assert.Equal(t, creditbilling.ZeroReasonNoTrustedUsage, other["credit_billing_zero_reason"])
}

func TestTextTrustedZeroUsageDiffersByBillingMode(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         string
		fixedCredits int64
		wantCredits  int64
	}{
		{name: "usage", mode: creditbilling.ModeUsageTokens, wantCredits: 0},
		{name: "fixed", mode: creditbilling.ModeFixedRequest, fixedCredits: 80, wantCredits: 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			userID := 97520 + len(tc.name)
			tokenID := userID + 100
			planID := userID + 200
			subID := userID + 300
			channelID := userID + 400
			tokenKey := "sk-credit-zero-" + tc.name
			seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, tokenKey, 1_000, 0)

			ctx := newBillingTestContext(t)
			relayInfo := newBillingTestRelayInfo(userID, tokenID, tokenKey, "req-credit-zero-"+tc.name, "subscription_only")
			freezeCreditBillingForServiceTest(t, ctx, relayInfo, channelID, tc.mode, tc.fixedCredits, false)
			relayInfo.SetEstimatePromptTokens(10)
			preConsumeForBillingTest(t, ctx, relayInfo, 999)

			require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0}, nil))
			model.FlushSubscriptionTokenDeltaUpdates()

			assert.True(t, relayInfo.HasTrustedUsage)
			assert.Equal(t, int64(0), relayInfo.RawMeteredTokens)
			assert.Equal(t, tc.wantCredits, getSubscriptionTokenUsed(t, subID))
			assert.Equal(t, tc.wantCredits, relayInfo.SubscriptionBillableTokens)
		})
	}
}

func TestTextDynamicMultiplierDisabledIgnoresUpstreamMultiplier(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97531, 97532, 97533, 97534, 97535
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-dynamic-disabled", 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-dynamic-disabled", "req-credit-dynamic-disabled", "subscription_only")
	freezeCreditBillingForServiceTest(t, ctx, relayInfo, channelID, creditbilling.ModeUsageTokens, 0, false)
	relayInfo.DynamicBillingMultiplier = 2
	relayInfo.DynamicBillingMultiplierSource = "upstream_test"
	relayInfo.SetEstimatePromptTokens(8)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))
	model.FlushSubscriptionTokenDeltaUpdates()

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(8), relayInfo.SubscriptionBillableTokens)
	assert.Equal(t, float64(1), relayInfo.FrozenDynamicBillingMultiplier())
	assert.Equal(t, creditbilling.DynamicMultiplierDefaultSource, relayInfo.FrozenDynamicBillingMultiplierSource())
}

func TestTextDynamicMultiplierEnabledAppliesUpstreamMultiplier(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97541, 97542, 97543, 97544, 97545
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-dynamic-enabled", 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-dynamic-enabled", "req-credit-dynamic-enabled", "subscription_only")
	freezeCreditBillingForServiceTest(t, ctx, relayInfo, channelID, creditbilling.ModeUsageTokens, 0, true)
	require.True(t, relayInfo.SetDynamicBillingMultiplier(1.5, "upstream_test"))
	relayInfo.SetEstimatePromptTokens(8)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))
	model.FlushSubscriptionTokenDeltaUpdates()
	model.FlushConsumeLogUpdates()

	assert.Equal(t, int64(12), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(8), relayInfo.CreditBillingBaseCredits)
	assert.Equal(t, int64(12), relayInfo.SubscriptionBillableTokens)
	assert.Equal(t, int64(12), relayInfo.ApiKeyBillableTokens)
	assert.InDelta(t, 1.5, relayInfo.FrozenDynamicBillingMultiplier(), 1e-9)
	assert.Equal(t, "upstream_test", relayInfo.FrozenDynamicBillingMultiplierSource())
	log := getLastLog(t)
	require.NotNil(t, log)
	require.NotNil(t, log.MeteredTokens)
	assert.Equal(t, 8, *log.MeteredTokens)
	assert.Equal(t, 8, log.Quota, "logs.quota remains legacy quota/cost, not dynamic-multiplied credits")
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, creditbilling.ModeUsageTokens, other["credit_billing_mode"])
	assert.Equal(t, float64(8), other["raw_metered_tokens"])
	assert.Equal(t, float64(8), other["base_credits"])
	assert.Equal(t, float64(12), other["api_key_credits"])
	assert.Equal(t, float64(12), other["subscription_credits"])
	assert.Equal(t, float64(12), other["final_credits"])
	assert.Equal(t, float64(12), other["api_key_credits_consumed"])
	assert.Equal(t, float64(12), other["subscription_credits_consumed"])
	assert.Equal(t, float64(1.5), other["dynamic_billing_multiplier"])
	assert.Equal(t, "upstream_test", other["dynamic_billing_multiplier_source"])
}

func TestWssFixedRequestMultipleUsageIncrementsChargesOnce(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97551, 97552, 97553, 97554, 97555
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-wss-fixed", 1_000, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1_000),
		"token_used":          int64(0),
	}).Error)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-wss-fixed", "req-credit-wss-fixed", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	freezeCreditBillingForServiceTest(t, ctx, relayInfo, channelID, creditbilling.ModeFixedRequest, 80, false)
	relayInfo.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4}))
	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}))
	require.NoError(t, PostWssConsumeQuota(ctx, relayInfo, relayInfo.OriginModelName, &dto.RealtimeUsage{InputTokens: 3, OutputTokens: 3, TotalTokens: 6}, ""))
	model.FlushSubscriptionTokenDeltaUpdates()
	model.FlushConsumeLogUpdates()

	assert.Equal(t, int64(80), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(80), getTokenUsed(t, tokenID))
	assert.Equal(t, int64(6), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(80), relayInfo.CreditBillingBaseCredits)
	assert.Equal(t, int64(80), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(80), relayInfo.SubscriptionBillableTokens)
	var incrementCount int64
	require.NoError(t, model.DB.Model(&model.TokenLimitPreConsumeRecord{}).Where("request_id LIKE ?", "req-credit-wss-fixed:realtime:%").Count(&incrementCount).Error)
	assert.Equal(t, int64(0), incrementCount)
}

func TestCodexProServedHeaderDoesNotImplyDynamicMultiplier(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97561, 97562, 97563, 97564, 97565
	seedCreditBillingRuntime(t, userID, tokenID, planID, subID, channelID, "sk-credit-codex-served", 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-credit-codex-served", "req-credit-codex-served", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	freezeCreditBillingForServiceTest(t, ctx, relayInfo, channelID, creditbilling.ModeUsageTokens, 0, true)
	relayInfo.CodexProRequestSent = true
	relayInfo.CodexProServed = true
	relayInfo.SetEstimatePromptTokens(8)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))
	model.FlushSubscriptionTokenDeltaUpdates()

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(8), relayInfo.SubscriptionBillableTokens)
	assert.Equal(t, float64(1), relayInfo.FrozenDynamicBillingMultiplier())
	assert.Equal(t, creditbilling.DynamicMultiplierDefaultSource, relayInfo.FrozenDynamicBillingMultiplierSource())
}
