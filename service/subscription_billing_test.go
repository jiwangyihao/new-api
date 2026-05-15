package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ensureSubscriptionBillingTables(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Migrator().DropTable(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
}

func seedDistributorPlan(t *testing.T, id int, code string, tokenLimit int64) {
	t.Helper()
	ensureSubscriptionBillingTables(t)
	plan := &model.SubscriptionPlan{
		Id:                id,
		Title:             code,
		Enabled:           true,
		MonthlyTokenLimit: tokenLimit,
		ConcurrencyLimit:  1,
		BusinessCode:      &code,
	}
	require.NoError(t, model.DB.Create(plan).Error)
}

func seedLegacySubscriptionPlan(t *testing.T, id int, title string) {
	t.Helper()
	ensureSubscriptionBillingTables(t)
	plan := &model.SubscriptionPlan{
		Id:      id,
		Title:   title,
		Enabled: true,
	}
	require.NoError(t, model.DB.Create(plan).Error)
}

func seedLegacySubscription(t *testing.T, id int, userId int, planId int, amountTotal int64, amountUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		PlanId:      planId,
		AmountTotal: amountTotal,
		AmountUsed:  amountUsed,
		Status:      "active",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedDistributorSubscription(t *testing.T, id int, userId int, planId int, tokenLimit int64, tokenUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		PlanId:      planId,
		AmountTotal: 1,
		TokenLimit:  tokenLimit,
		TokenUsed:   tokenUsed,
		Status:      "active",
		GrantReason: "order",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func getSubscriptionTokenUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("token_used").Where("id = ?", id).First(&sub).Error)
	return sub.TokenUsed
}

func newBillingTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	return ctx
}

func newBillingTestRelayInfo(userId, tokenId int, tokenKey string, requestId string, pref string) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		TokenId:         tokenId,
		TokenKey:        tokenKey,
		UserId:          userId,
		OriginModelName: "gpt-4o",
		RequestId:       requestId,
		StartTime:       time.Now(),
		UserSetting: dto.UserSetting{
			BillingPreference: pref,
		},
		UserQuota: 10_000,
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			CacheRatio:      1,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
		RelayMode:   relayconstant.RelayModeChatCompletions,
	}
	return info
}

func preConsumeForBillingTest(t *testing.T, ctx *gin.Context, relayInfo *relaycommon.RelayInfo, estimated int) {
	t.Helper()
	apiErr := PreConsumeBilling(ctx, estimated, relayInfo)
	require.Nil(t, apiErr)
}

func assertSubscriptionBillingSettle(t *testing.T, name string, usage *dto.Usage, wantTokens int64) {
	t.Helper()
	truncate(t)
	const userID = 8001
	const tokenID = 8002
	const planID = 8003
	const subID = 8004
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-subscription-"+name, 10_000)
	seedDistributorPlan(t, planID, "plan-"+name, 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-subscription-"+name, "req-"+name, "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{
		WalletQuota:        999,
		SubscriptionTokens: wantTokens,
		UsageEstimated:     false,
	}))

	assert.Equal(t, wantTokens, getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getUserQuota(t, userID), "subscription billing must not deduct wallet quota")
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not deduct token key quota")
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID), "subscription billing must not update token key used quota")
	assert.Equal(t, int64(1_000), relayInfo.SubscriptionAmountTotal, "compat field must expose token limit for distributor subscription")
	assert.Equal(t, wantTokens, relayInfo.SubscriptionAmountUsedAfterPreConsume+relayInfo.SubscriptionPostDelta, "compat field must expose token used for distributor subscription")
	assert.Equal(t, wantTokens, relayInfo.SubscriptionPreConsumed+relayInfo.SubscriptionPostDelta, "subscription consumed compatibility field must use token units")
}

func TestSubscriptionBillingPreConsumesEstimatedTokens(t *testing.T) {
	truncate(t)
	const userID = 8041
	const tokenID = 8042
	const planID = 8043
	const subID = 8044
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-estimated-preconsume", 10_000)
	seedDistributorPlan(t, planID, "plan-estimated-preconsume", 5_000)
	seedDistributorSubscription(t, subID, userID, planID, 5_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-estimated-preconsume", "req-estimated-preconsume", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	preConsumeForBillingTest(t, ctx, relayInfo, 1000)

	assert.Equal(t, int64(10), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
}

func TestSubscriptionBillingReserveDoesNotDoubleCountCompatibilityFields(t *testing.T) {
	truncate(t)
	const userID = 8051
	const tokenID = 8052
	const planID = 8053
	const subID = 8054
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-reserve", 10_000)
	seedDistributorPlan(t, planID, "plan-reserve", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-reserve", "req-reserve", "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	require.NotNil(t, relayInfo.Billing)
	require.NoError(t, relayInfo.Billing.Reserve(100))
	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{WalletQuota: 100, SubscriptionTokens: 100}))

	assert.Equal(t, int64(100), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, int64(100), relayInfo.SubscriptionAmountUsedAfterPreConsume+relayInfo.SubscriptionPostDelta)
	assert.Equal(t, int64(100), relayInfo.SubscriptionPreConsumed+relayInfo.SubscriptionPostDelta)
}

func TestSettleBillingWrapperDoesNotSynthesizeSubscriptionTokens(t *testing.T) {
	truncate(t)
	const userID = 8071
	const tokenID = 8072
	const planID = 8073
	const subID = 8074
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-wrapper", 10_000)
	seedDistributorPlan(t, planID, "plan-wrapper", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-wrapper", "req-wrapper", "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	require.NoError(t, SettleBilling(ctx, relayInfo, 999))

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID), "legacy/non-text settlement wrapper must not synthesize distributor token usage from wallet quota")
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not settle token key quota")
	assert.Equal(t, int64(-10), relayInfo.SubscriptionPostDelta)
}

func TestSettleBillingWrapperRejectsLegacySubscriptionQuota(t *testing.T) {
	truncate(t)
	const userID = 8081
	const tokenID = 8082
	const planID = 8083
	const subID = 8084
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-legacy-wrapper", 10_000)
	seedLegacySubscriptionPlan(t, planID, "legacy-wrapper")
	seedLegacySubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-legacy-wrapper", "req-legacy-wrapper", "subscription_only")

	apiErr := PreConsumeBilling(ctx, 10, relayInfo)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "subscription")
	require.NoError(t, SettleBilling(ctx, relayInfo, 0))

	assert.Equal(t, int64(0), getSubscriptionUsed(t, subID), "legacy amount subscription must not participate in request billing")
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not settle token key quota")
	assert.Empty(t, relayInfo.BillingSource)
}

func TestSubscriptionBillingAllowsCompletionsTextRelay(t *testing.T) {
	truncate(t)
	const userID = 8085
	const tokenID = 8086
	const planID = 8087
	const subID = 8088
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-completions", 10_000)
	seedDistributorPlan(t, planID, "plan-completions", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-completions", "req-completions", "subscription_only")
	relayInfo.RelayMode = relayconstant.RelayModeCompletions
	relayInfo.SetEstimatePromptTokens(7)

	preConsumeForBillingTest(t, ctx, relayInfo, 7)
	session, ok := relayInfo.Billing.(*BillingSession)
	require.True(t, ok)
	require.NoError(t, session.SettleWithInput(BillingSettleInput{SubscriptionTokens: 11}))

	assert.Equal(t, int64(11), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not consume token key quota")
}

func TestSubscriptionBillingSettlesNativeClaudeTextRelay(t *testing.T) {
	truncate(t)
	const userID = 8089
	const tokenID = 8090
	const planID = 8096
	const subID = 8097
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-claude-native", 10_000)
	seedDistributorPlan(t, planID, "plan-claude-native", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-claude-native", "req-claude-native", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatClaude
	relayInfo.RelayMode = relayconstant.RelayModeUnknown
	relayInfo.SetEstimatePromptTokens(5)

	preConsumeForBillingTest(t, ctx, relayInfo, 5)
	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 5, TotalTokens: 9}, nil))

	assert.Equal(t, int64(9), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not consume token key quota")
}

func TestSubscriptionBillingSettlesNativeGeminiTextRelay(t *testing.T) {
	truncate(t)
	const userID = 8098
	const tokenID = 8099
	const planID = 8100
	const subID = 8105
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-gemini-native", 10_000)
	seedDistributorPlan(t, planID, "plan-gemini-native", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	ctx.Request = httptest.NewRequest("POST", "/v1beta/models/gemini-pro:generateContent", nil)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-gemini-native", "req-gemini-native", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatGemini
	relayInfo.RelayMode = relayconstant.RelayModeGemini
	relayInfo.RequestURLPath = "/v1beta/models/gemini-pro:generateContent"
	relayInfo.SetEstimatePromptTokens(5)

	preConsumeForBillingTest(t, ctx, relayInfo, 5)
	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{UsageSemantic: "gemini", PromptTokens: 5, TotalTokens: 9}, nil))

	assert.Equal(t, int64(9), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not consume token key quota")
}

func TestSubscriptionBillingRejectsNativeGeminiEmbeddingRelay(t *testing.T) {
	truncate(t)
	const userID = 8106
	const tokenID = 8107
	const planID = 8108
	const subID = 8109
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-gemini-embed", 10_000)
	seedDistributorPlan(t, planID, "plan-gemini-embed", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)
	ctx := newBillingTestContext(t)
	ctx.Request = httptest.NewRequest("POST", "/v1beta/models/text-embedding-004:embedContent", nil)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-gemini-embed", "req-gemini-embed", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatGemini
	relayInfo.RelayMode = relayconstant.RelayModeGemini
	relayInfo.RequestURLPath = "/v1beta/models/text-embedding-004:embedContent"
	relayInfo.SetEstimatePromptTokens(5)

	apiErr := PreConsumeBilling(ctx, 5, relayInfo)
	require.NotNil(t, apiErr)
	assert.Empty(t, relayInfo.BillingSource)
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not consume token key quota")
}

func TestLegacySubscriptionPreConsumeUsesQuotaUnits(t *testing.T) {
	truncate(t)
	const userID = 8101
	const tokenID = 8102
	const planID = 8103
	const subID = 8104
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-legacy-preconsume", 10_000)
	seedLegacySubscriptionPlan(t, planID, "legacy-preconsume")
	seedLegacySubscription(t, subID, userID, planID, 200, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-legacy-preconsume", "req-legacy-preconsume", "subscription_only")
	relayInfo.SetEstimatePromptTokens(10)
	apiErr := PreConsumeBilling(ctx, 1_000, relayInfo)

	require.NotNil(t, apiErr)
	assert.Equal(t, int64(0), getSubscriptionUsed(t, subID))
}

func TestPostTextConsumeQuotaRejectsLegacySubscriptionQuota(t *testing.T) {
	truncate(t)
	const userID = 8111
	const tokenID = 8112
	const planID = 8113
	const subID = 8114
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-legacy-text", 10_000)
	seedChannel(t, 8115)
	seedLegacySubscriptionPlan(t, planID, "legacy-text")
	seedLegacySubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-legacy-text", "req-legacy-text", "subscription_only")
	relayInfo.ChannelId = 8115
	relayInfo.RelayMode = relayconstant.RelayModeChatCompletions
	relayInfo.PriceData.ModelRatio = 2
	apiErr := PreConsumeBilling(ctx, 10, relayInfo)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeSubscriptionRequired, apiErr.GetErrorCode())

	if relayInfo.Billing != nil {
		require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 10, TotalTokens: 10}, nil))
	}

	assert.Equal(t, int64(0), getSubscriptionUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not update token key quota")
}

func TestLegacySubscriptionNotificationUsesQuotaFormatting(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		BillingSource:                         BillingSourceSubscription,
		SubscriptionId:                        1,
		SubscriptionPreConsumed:               0,
		SubscriptionAmountTotal:               100,
		SubscriptionAmountUsedAfterPreConsume: 99,
		SubscriptionPostDelta:                 0,
	}
	remaining := relayInfo.SubscriptionAmountTotal - (relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta)
	remainingText := subscriptionRemainingText(false, remaining)
	assert.Equal(t, logger.FormatQuota(1), remainingText)
}

func TestDistributorSubscriptionNotificationUsesTokenFormatting(t *testing.T) {
	truncate(t)
	const userID = 8061
	const tokenID = 8062
	const planID = 8063
	const subID = 8064
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-notify", 10_000)
	seedDistributorPlan(t, planID, "plan-notify", 100)
	seedDistributorSubscription(t, subID, userID, planID, 100, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-notify", "req-notify", "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 99)

	remaining := relayInfo.SubscriptionAmountTotal - relayInfo.SubscriptionAmountUsedAfterPreConsume
	assert.Equal(t, "1 tokens", subscriptionRemainingText(true, remaining))
}

func TestSubscriptionBillingUsesMeteredTokens(t *testing.T) {
	tests := []struct {
		name  string
		usage *dto.Usage
		want  int64
	}{
		{
			name:  "openai_total_tokens",
			usage: &dto.Usage{TotalTokens: 8},
			want:  8,
		},
		{
			name: "responses_cached_tokens",
			usage: &dto.Usage{
				PromptTokens:        100,
				CompletionTokens:    50,
				TotalTokens:         150,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 40},
			},
			want: 150,
		},
		{
			name: "gemini_cached_content",
			usage: &dto.Usage{
				UsageSemantic:       "gemini",
				PromptTokens:        100,
				CompletionTokens:    50,
				TotalTokens:         150,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 40},
			},
			want: 150,
		},
		{
			name: "claude_cache_creation_remainder",
			usage: &dto.Usage{
				UsageSemantic:    "anthropic",
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
				PromptTokensDetails: dto.InputTokenDetails{
					CachedTokens:         30,
					CachedCreationTokens: 50,
				},
				ClaudeCacheCreation5mTokens: 7,
				ClaudeCacheCreation1hTokens: 11,
			},
			want: 230,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSubscriptionBillingSettle(t, tt.name, tt.usage, tt.want)
		})
	}
}

func TestSubscriptionBillingUsesEstimatedTokensWhenUsageMissing(t *testing.T) {
	truncate(t)
	const userID = 8011
	const tokenID = 8012
	const planID = 8013
	const subID = 8014
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-estimated", 10_000)
	seedChannel(t, 8015)
	seedDistributorPlan(t, planID, "plan-estimated", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-estimated", "req-estimated", "subscription_only")
	relayInfo.ChannelId = 8015
	relayInfo.RelayMode = relayconstant.RelayModeChatCompletions
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, nil, nil))

	assert.Equal(t, int64(6), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not use token key quota")
	log := getLastLog(t)
	require.NotNil(t, log)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, true, other["usage_estimated"])
}

func TestPostTextConsumeQuotaReturnsSettleErrorWhenSubscriptionTokensExhausted(t *testing.T) {
	truncate(t)
	const userID = 8016
	const tokenID = 8017
	const planID = 8018
	const subID = 8019
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-settle-exhausted", 10_000)
	seedChannel(t, 8020)
	seedDistributorPlan(t, planID, "plan-settle-exhausted", 10)
	seedDistributorSubscription(t, subID, userID, planID, 10, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-settle-exhausted", "req-settle-exhausted", "subscription_only")
	relayInfo.ChannelId = 8020
	relayInfo.RelayMode = relayconstant.RelayModeChatCompletions
	relayInfo.SetEstimatePromptTokens(6)
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	err := PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6, TotalTokens: 11}, nil)

	require.Error(t, err)
	assert.Equal(t, int64(6), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "subscription billing must not use token key quota")
}

func TestPreWssConsumeQuotaUsesSubscriptionTokensOnly(t *testing.T) {
	truncate(t)
	const userID = 8021
	const tokenID = 8022
	const planID = 8023
	const subID = 8024
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-realtime-sub", 10_000)
	seedDistributorPlan(t, planID, "plan-realtime-sub", 100)
	seedDistributorSubscription(t, subID, userID, planID, 100, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-realtime-sub", "req-realtime-sub", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	relayInfo.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx, relayInfo, 1)

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 4, InputTokens: 2, OutputTokens: 2}))

	assert.Equal(t, int64(5), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "realtime subscription billing must not consume token key quota")
	assert.Equal(t, 10_000, getUserQuota(t, userID), "realtime subscription billing must not consume wallet quota")
}

func TestPreWssConsumeQuotaAccumulatesMultipleRealtimeChunks(t *testing.T) {
	truncate(t)
	const userID = 8027
	const tokenID = 8028
	const planID = 8029
	const subID = 8030
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-realtime-multi", 10_000)
	seedDistributorPlan(t, planID, "plan-realtime-multi", 100)
	seedDistributorSubscription(t, subID, userID, planID, 100, 0)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-realtime-multi", "req-realtime-multi", "subscription_only")
	relayInfo.RelayFormat = types.RelayFormatOpenAIRealtime
	relayInfo.RelayMode = relayconstant.RelayModeRealtime
	relayInfo.SetEstimatePromptTokens(1)
	preConsumeForBillingTest(t, ctx, relayInfo, 1)

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 4}))
	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 3}))

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "realtime subscription billing must not consume token key quota")
	assert.Equal(t, 10_000, getUserQuota(t, userID), "realtime subscription billing must not consume wallet quota")
}

func TestPreWssConsumeQuotaRequiresSubscriptionBilling(t *testing.T) {
	truncate(t)
	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(8025, 8026, "sk-realtime-missing", "req-realtime-missing", "wallet_only")

	err := PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 4})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscription")
}

func TestPostTextConsumeQuotaSkipsDistributorTokensForNonTextRelay(t *testing.T) {
	truncate(t)
	const userID = 8091
	const tokenID = 8092
	const planID = 8093
	const subID = 8094
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-embedding", 10_000)
	seedChannel(t, 8095)
	seedDistributorPlan(t, planID, "plan-embedding", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-embedding", "req-embedding", "subscription_only")
	relayInfo.ChannelId = 8095
	relayInfo.RelayMode = relayconstant.RelayModeEmbeddings
	relayInfo.SetEstimatePromptTokens(6)
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)

	if relayInfo.Billing != nil {
		require.NoError(t, PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6, TotalTokens: 6}, nil))
	}

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID), "token key quota must be refunded when distributor subscription is rejected")
}

func TestPostAudioConsumeQuotaDoesNotConsumeDistributorSubscription(t *testing.T) {
	truncate(t)
	const userID = 8121
	const tokenID = 8122
	const planID = 8123
	const subID = 8124
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-audio", 10_000)
	seedChannel(t, 8125)
	seedDistributorPlan(t, planID, "plan-audio", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audio", "req-audio", "subscription_only")
	relayInfo.ChannelId = 8125
	relayInfo.RelayMode = relayconstant.RelayModeAudioSpeech
	relayInfo.SetEstimatePromptTokens(6)
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)

	PostAudioConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6, TotalTokens: 6}, "")
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
}

func TestPostAudioConsumeQuotaRejectsDistributorSubscriptionFirstWithoutWalletFallback(t *testing.T) {
	truncate(t)
	const userID = 8131
	const tokenID = 8132
	const planID = 8133
	const subID = 8134
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-audio-first", 10_000)
	seedChannel(t, 8135)
	seedDistributorPlan(t, planID, "plan-audio-first", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audio-first", "req-audio-first", "subscription_first")
	relayInfo.ChannelId = 8135
	relayInfo.RelayMode = relayconstant.RelayModeAudioSpeech
	relayInfo.SetEstimatePromptTokens(6)
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)
	assert.Empty(t, relayInfo.BillingSource)
	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
}

func TestPostAudioConsumeQuotaRejectsExhaustedDistributorSubscriptionFirstWithoutWalletFallback(t *testing.T) {
	truncate(t)
	const userID = 8141
	const tokenID = 8142
	const planID = 8143
	const subID = 8144
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-audio-exhausted", 10_000)
	seedChannel(t, 8145)
	seedDistributorPlan(t, planID, "plan-audio-exhausted", 5)
	seedDistributorSubscription(t, subID, userID, planID, 5, 5)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-audio-exhausted", "req-audio-exhausted", "subscription_first")
	relayInfo.ChannelId = 8145
	relayInfo.RelayMode = relayconstant.RelayModeAudioSpeech
	relayInfo.SetEstimatePromptTokens(6)
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)
	assert.Empty(t, relayInfo.BillingSource)
	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(5), getSubscriptionTokenUsed(t, subID))
}

func TestWalletOnlyPreferenceRequiresSubscriptionForRequestBilling(t *testing.T) {
	truncate(t)
	const userID = 8021
	const tokenID = 8022
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-wallet", 10_000)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-wallet", "req-wallet", "wallet_only")
	apiErr := PreConsumeBilling(ctx, 6, relayInfo)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "active subscription is required")
	assert.Empty(t, relayInfo.BillingSource)
	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID))
}

func TestTokenKeyQuotaDoesNotChangeWhenSubscriptionUsesTokens(t *testing.T) {
	truncate(t)
	const userID = 8031
	const tokenID = 8032
	const planID = 8033
	const subID = 8034
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-token-key", 10_000)
	seedDistributorPlan(t, planID, "plan-token-key", 1_000)
	seedDistributorSubscription(t, subID, userID, planID, 1_000, 0)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-token-key", "req-token-key", "subscription_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{
		WalletQuota:        999,
		SubscriptionTokens: 8,
		UsageEstimated:     false,
	}))

	assert.Equal(t, int64(8), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 0, getTokenUsedQuota(t, tokenID))
}
