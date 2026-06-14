package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func freezeChannelMultiplierForServiceTest(t *testing.T, ctx *gin.Context, relayInfo *relaycommon.RelayInfo, channelID int, channelType int, multiplier float64) {
	t.Helper()
	common.SetContextKey(ctx, constant.ContextKeyChannelId, channelID)
	common.SetContextKey(ctx, constant.ContextKeyChannelType, channelType)
	common.SetContextKey(ctx, constant.ContextKeyChannelTokenBillingMultiplier, multiplier)
	require.NoError(t, relayInfo.FreezeChannelTokenBillingSnapshot(ctx))
	relayInfo.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: channelID, ChannelType: channelType}
	relayInfo.ChannelId = channelID
	relayInfo.ChannelType = channelType
}

func TestChannelTokenMultiplierPreConsumeUsesEstimatedRawTokensForSubscriptionAndApiKeyCap(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97001, 97002, 97003, 97004, 97005
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-channel-multiplier-preconsume", 1_000, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1_000),
		"token_used":          int64(0),
	}).Error)
	seedChannel(t, channelID)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-channel-multiplier-preconsume", "req-channel-multiplier-preconsume", "subscription_only")
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, channelID, constant.ChannelTypeOpenAI, 2)
	relayInfo.SetEstimatePromptTokens(10)
	relayInfo.PriceData.ModelRatio = 9
	relayInfo.PriceData.ModelPrice = 123

	preConsumeForBillingTest(t, ctx, relayInfo, 777)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	assert.Equal(t, int64(20), relayInfo.SubscriptionPreConsumed)
	assert.Equal(t, 20, relayInfo.FinalPreConsumedQuota)
	assert.Equal(t, int64(20), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(20), getTokenUsed(t, tokenID))
	assert.Equal(t, int64(10), relayInfo.EstimatedRawTokens)
	assert.Equal(t, 2.0, relayInfo.FrozenChannelTokenBillingMultiplier())
}

func TestChannelTokenMultiplierPreConsumeDoesNotUseLegacyQuotaWhenEstimateMissing(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97061, 97062, 97063, 97064, 97065
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-channel-multiplier-no-estimate", 2_000, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(2_000),
		"token_used":          int64(0),
	}).Error)
	seedChannel(t, channelID)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-channel-multiplier-no-estimate", "req-channel-multiplier-no-estimate", "subscription_only")
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, channelID, constant.ChannelTypeOpenAI, 2)
	relayInfo.PriceData.ModelRatio = 9
	relayInfo.PriceData.ModelPrice = 123

	preConsumeForBillingTest(t, ctx, relayInfo, 777)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	assert.Equal(t, int64(2), relayInfo.SubscriptionPreConsumed)
	assert.Equal(t, 2, relayInfo.FinalPreConsumedQuota)
	assert.Equal(t, int64(2), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(2), getTokenUsed(t, tokenID))
	assert.Equal(t, int64(1), relayInfo.EstimatedRawTokens)
}

func TestPostTextConsumeQuotaChannelMultiplierRefundsWhenActualBillableBelowPreconsumeAndLogsSnapshot(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97011, 97012, 97013, 97014, 97015
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-channel-multiplier-refund", 1_000, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1_000),
		"token_used":          int64(0),
	}).Error)
	seedChannel(t, channelID)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-channel-multiplier-refund", "req-channel-multiplier-refund", "subscription_only")
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, channelID, constant.ChannelTypeOpenAI, 2)
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 777)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))

	model.FlushSubscriptionTokenDeltaUpdates()
	model.FlushConsumeLogUpdates()
	assert.Equal(t, int64(16), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(16), getTokenUsed(t, tokenID))
	assert.Equal(t, int64(8), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(16), relayInfo.ChannelBillableTokens)
	assert.Equal(t, int64(16), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(16), relayInfo.SubscriptionBillableTokens)
	log := getLastLog(t)
	require.NotNil(t, log)
	require.NotNil(t, log.MeteredTokens)
	assert.Equal(t, 8, *log.MeteredTokens)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(2), other["channel_token_billing_multiplier"])
	assert.Equal(t, float64(8), other["raw_metered_tokens"])
	assert.Equal(t, float64(16), other["channel_billable_tokens"])
	assert.Equal(t, float64(16), other["api_key_billable_tokens"])
	assert.Equal(t, float64(16), other["subscription_billable_tokens"])
	assert.Equal(t, "usage_tokens", other["credit_billing_mode"])
	assert.Equal(t, float64(0), other["fixed_request_credits"])
	assert.Equal(t, float64(16), other["base_credits"])
	assert.Equal(t, float64(16), other["api_key_credits"])
	assert.Equal(t, float64(16), other["subscription_credits"])
	assert.Equal(t, float64(16), other["final_credits"])
	assert.Equal(t, float64(1), other["dynamic_billing_multiplier"])
	assert.Equal(t, "default", other["dynamic_billing_multiplier_source"])
	assert.Equal(t, float64(10), other["estimated_raw_tokens"])
	assert.Equal(t, float64(channelID), other["initial_channel_id"])
	assert.Equal(t, float64(constant.ChannelTypeOpenAI), other["initial_channel_type"])
	assert.Equal(t, float64(16), other["subscription_tokens_consumed"])
}

func TestPostTextConsumeQuotaCodexProChannelMultiplierTokenLimitUsesChannelBillableOnly(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97021, 97022, 97023, 97024, 97025
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-channel-multiplier-codex", 10_000)
	seedChannel(t, channelID)
	seedCodexProBillingPlan(t, planID, "channel-multiplier-codex", 1_000, 100, false, false)
	seedCodexProBillingSubscription(t, subID, userID, planID, 1_000, 0, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder, "active", common.GetTimestamp()+3600)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1_000),
		"token_used":          int64(0),
	}).Error)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-channel-multiplier-codex", "req-channel-multiplier-codex", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeResponses
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, channelID, constant.ChannelTypeOpenAI, 2)
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))
	setBoolFieldForTest(t, relayInfo, "CodexProServed", true)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))
	model.FlushSubscriptionTokenDeltaUpdates()

	assert.Equal(t, int64(16), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(16), getTokenUsed(t, tokenID))
	record := getTokenLimitRecordForTest(t, "req-channel-multiplier-codex")
	assert.Equal(t, int64(16), record.ActualTokens)
	assert.Equal(t, int64(16), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(16), relayInfo.SubscriptionBillableTokens)
}

func TestUsageEstimatedSubscriptionWithChannelMultiplierDoesNotChargeAndRefundsPreconsume(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97031, 97032, 97033, 97034, 97035
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-channel-multiplier-estimated", 1_000, 0)
	seedChannel(t, channelID)

	ctx := newBillingTestContext(t)
	common.SetContextKey(ctx, constant.ContextKeyLocalCountTokens, true)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-channel-multiplier-estimated", "req-channel-multiplier-estimated", "subscription_only")
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, channelID, constant.ChannelTypeOpenAI, 2)
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil))
	model.FlushConsumeLogUpdates()

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(0), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(0), relayInfo.ChannelBillableTokens)
	assert.Equal(t, int64(0), relayInfo.SubscriptionBillableTokens)
	assert.Equal(t, int64(10), relayInfo.EstimatedRawTokens)
	log := getLastLog(t)
	require.NotNil(t, log)
	require.NotNil(t, log.MeteredTokens)
	assert.Equal(t, 0, *log.MeteredTokens)
}

func TestChannelMultiplierZeroUsageSubscriptionDoesNotChargeAndRefundsPreconsume(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97051, 97052, 97053, 97054, 97055
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-channel-multiplier-zero", 1_000, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1_000),
		"token_used":          int64(0),
	}).Error)
	seedChannel(t, channelID)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-channel-multiplier-zero", "req-channel-multiplier-zero", "subscription_only")
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, channelID, constant.ChannelTypeOpenAI, 2)
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 0}, nil))
	model.FlushConsumeLogUpdates()

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(0), getTokenUsed(t, tokenID))
	assert.Equal(t, int64(0), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(0), relayInfo.ChannelBillableTokens)
	record := getTokenLimitRecordForTest(t, "req-channel-multiplier-zero")
	assert.Equal(t, model.TokenLimitPreConsumeStatusSettled, record.Status)
	assert.Equal(t, int64(0), record.ActualTokens)
	assert.Equal(t, int64(0), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(0), relayInfo.SubscriptionBillableTokens)
	assert.Equal(t, int64(0), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(0), relayInfo.ChannelBillableTokens)
	log := getLastLog(t)
	require.NotNil(t, log)
	require.NotNil(t, log.MeteredTokens)
	assert.Equal(t, 0, *log.MeteredTokens)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Nil(t, other["usage_unavailable"])
	assert.Equal(t, float64(0), other["raw_metered_tokens"])
	assert.Equal(t, float64(0), other["channel_billable_tokens"])
}
func TestNewAPIBillingFromUsageAppliesChannelMultiplierToApiKeySnapshot(t *testing.T) {
	truncate(t)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(97071, 97072, "sk-channel-multiplier-newapi-billing", "req-channel-multiplier-newapi-billing", "subscription_only")
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, 97075, constant.ChannelTypeOpenAI, 2)

	billing := NewAPIBillingFromUsage(relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8})
	require.NotNil(t, billing)
	SeedNewAPIBillingRelayInfo(relayInfo, *billing)

	assert.Equal(t, int64(8), billing.MeteredTokens)
	assert.Equal(t, int64(16), billing.BillableTokens)
	assert.Equal(t, int64(8), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(16), relayInfo.ChannelBillableTokens)
	assert.Equal(t, int64(16), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(16), relayInfo.SubscriptionBillableTokens)
}

func TestNewAPIBillingFromUsageTreatsZeroUsageObjectAsTrustedZeroUsage(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	billing := NewAPIBillingFromUsage(relayInfo, &dto.Usage{PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0})

	require.NotNil(t, billing)
	assert.Equal(t, int64(0), billing.MeteredTokens)
	assert.Equal(t, int64(0), billing.BillableTokens)
	assert.Equal(t, float64(1), billing.BillingMultiplier)
	assert.Equal(t, "default", billing.BillingMultiplierSource)
	assert.True(t, relayInfo.HasTrustedUsage)
	assert.Equal(t, int64(0), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(0), relayInfo.ChannelBillableTokens)
	assert.Equal(t, int64(0), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(0), relayInfo.SubscriptionBillableTokens)
}

func TestNewAPIBillingFromUsageKeepsCodexProOffApiKeySnapshot(t *testing.T) {
	truncate(t)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(97081, 97082, "sk-channel-multiplier-newapi-codex", "req-channel-multiplier-newapi-codex", "subscription_only")
	seedRuntimeDistributorBilling(t, 97081, 97082, 97083, 97084, "sk-channel-multiplier-newapi-codex", 1_000, 0)
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, 97085, constant.ChannelTypeOpenAI, 2)
	relayInfo.SetEstimatePromptTokens(8)
	preConsumeForBillingTest(t, ctx, relayInfo, 8)
	setBoolFieldForTest(t, relayInfo, "CodexProServed", true)

	billing := NewAPIBillingFromUsage(relayInfo, &dto.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8})
	require.NotNil(t, billing)
	SeedNewAPIBillingRelayInfo(relayInfo, *billing)

	assert.Equal(t, int64(8), billing.MeteredTokens)
	assert.Equal(t, int64(16), billing.BillableTokens)
	assert.Equal(t, int64(16), relayInfo.ChannelBillableTokens)
	assert.Equal(t, int64(16), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(16), relayInfo.SubscriptionBillableTokens)
}

func TestWssChannelMultiplierIncrementAppliesOnce(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID, channelID = 97041, 97042, 97043, 97044, 97045
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, "sk-channel-multiplier-wss", 1_000, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1_000),
		"token_used":          int64(0),
	}).Error)
	seedChannel(t, channelID)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-channel-multiplier-wss", "req-channel-multiplier-wss", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, channelID, constant.ChannelTypeOpenAI, 2)
	relayInfo.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx, relayInfo, 999)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4}))
	require.NoError(t, PostWssConsumeQuota(ctx, relayInfo, relayInfo.OriginModelName, &dto.RealtimeUsage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4}, ""))
	model.FlushSubscriptionTokenDeltaUpdates()
	model.FlushConsumeLogUpdates()

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(8), getTokenUsed(t, tokenID))
	preRecord := getTokenLimitRecordForTest(t, "req-channel-multiplier-wss")
	assert.Equal(t, model.TokenLimitPreConsumeStatusSettled, preRecord.Status)
	assert.Equal(t, int64(2), preRecord.PreConsumedTokens)
	assert.Equal(t, int64(0), preRecord.ActualTokens)
	incrementRecord := getTokenLimitRecordForTest(t, "req-channel-multiplier-wss:realtime:1")
	assert.Equal(t, model.TokenLimitPreConsumeStatusSettled, incrementRecord.Status)
	assert.Equal(t, int64(8), incrementRecord.ActualTokens)
	assert.Equal(t, int64(4), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(8), relayInfo.ChannelBillableTokens)
	assert.Equal(t, int64(8), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(8), relayInfo.SubscriptionBillableTokens)
	log := getLastLog(t)
	require.NotNil(t, log)
	require.NotNil(t, log.MeteredTokens)
	assert.Equal(t, 4, *log.MeteredTokens)
}

func TestChannelTokenMultiplierEndToEndBillingLogSnapshotSurvivesChannelMultiplierChange(t *testing.T) {
	truncate(t)
	const userID, tokenID, planID, subID = 97101, 97102, 97103, 97104
	const channelAID, channelBID = 97105, 97106
	const tokenKey = "sk-channel-multiplier-e2e"
	const requestID = "req-channel-multiplier-e2e"
	seedRuntimeDistributorBilling(t, userID, tokenID, planID, subID, tokenKey, 1_000_000, 0)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1_000_000),
		"token_used":          int64(0),
	}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: channelAID, Type: constant.ChannelTypeOpenAI, Key: "sk-channel-multiplier-a", Status: common.ChannelStatusEnabled, Name: "channel-multiplier-a", Models: "gpt-4o", TokenBillingMultiplier: 1}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: channelBID, Type: constant.ChannelTypeGemini, Key: "sk-channel-multiplier-b", Status: common.ChannelStatusEnabled, Name: "channel-multiplier-b", Models: "gpt-4o", TokenBillingMultiplier: 2}).Error)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, tokenKey, requestID, "subscription_only")
	freezeChannelMultiplierForServiceTest(t, ctx, relayInfo, channelBID, constant.ChannelTypeGemini, 2)
	relayInfo.SetEstimatePromptTokens(10_000)
	preConsumeForBillingTest(t, ctx, relayInfo, 10_000)
	relayInfo.TokenLimit = NewTokenLimitSession(relayInfo)
	require.Nil(t, relayInfo.TokenLimit.PreConsume(relayInfo.SubscriptionPreConsumedTokens()))

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6_000, CompletionTokens: 4_000, TotalTokens: 10_000}, nil))
	model.FlushSubscriptionTokenDeltaUpdates()
	model.FlushConsumeLogUpdates()

	assert.Equal(t, int64(20_000), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(20_000), getTokenUsed(t, tokenID))
	record := getTokenLimitRecordForTest(t, requestID)
	assert.Equal(t, model.TokenLimitPreConsumeStatusSettled, record.Status)
	assert.Equal(t, int64(20_000), record.PreConsumedTokens)
	assert.Equal(t, int64(20_000), record.ActualTokens)
	assert.Equal(t, int64(0), record.DeltaTokens)
	assert.Equal(t, int64(10_000), relayInfo.RawMeteredTokens)
	assert.Equal(t, int64(20_000), relayInfo.ChannelBillableTokens)
	assert.Equal(t, int64(20_000), relayInfo.ApiKeyBillableTokens)
	assert.Equal(t, int64(20_000), relayInfo.SubscriptionBillableTokens)

	log := getLastLog(t)
	require.NotNil(t, log)
	require.NotNil(t, log.MeteredTokens)
	assert.Equal(t, channelBID, log.ChannelId)
	assert.Equal(t, 10_000, *log.MeteredTokens)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(2), other["channel_token_billing_multiplier"])
	assert.Equal(t, float64(10_000), other["raw_metered_tokens"])
	assert.Equal(t, float64(20_000), other["channel_billable_tokens"])
	assert.Equal(t, float64(20_000), other["api_key_billable_tokens"])
	assert.Equal(t, float64(20_000), other["subscription_billable_tokens"])
	assert.Equal(t, float64(channelBID), other["initial_channel_id"])
	assert.Equal(t, float64(constant.ChannelTypeGemini), other["initial_channel_type"])

	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelBID).Update("token_billing_multiplier", 1.5).Error)
	var storedLog model.Log
	require.NoError(t, model.LOG_DB.Where("id = ?", log.Id).First(&storedLog).Error)
	storedOther, err := common.StrToMap(storedLog.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(2), storedOther["channel_token_billing_multiplier"])
	assert.Equal(t, float64(20_000), storedOther["channel_billable_tokens"])
	assert.Equal(t, float64(20_000), storedOther["api_key_billable_tokens"])
	assert.Equal(t, float64(20_000), storedOther["subscription_billable_tokens"])
}
