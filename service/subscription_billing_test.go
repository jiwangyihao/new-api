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
	assert.Equal(t, 10_000-999, getTokenRemainQuota(t, tokenID), "token key quota must use wallet quota")
	assert.Equal(t, 999, getTokenUsedQuota(t, tokenID), "token key used quota must use wallet quota")
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
	assert.Equal(t, 10_000-1000, getTokenRemainQuota(t, tokenID))
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
	assert.Equal(t, 10_000-999, getTokenRemainQuota(t, tokenID), "token key quota still settles wallet quota")
	assert.Equal(t, int64(-10), relayInfo.SubscriptionPostDelta)
}

func TestSettleBillingWrapperPreservesLegacySubscriptionQuota(t *testing.T) {
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
	preConsumeForBillingTest(t, ctx, relayInfo, 10)
	require.NoError(t, SettleBilling(ctx, relayInfo, 999))

	assert.Equal(t, int64(999), getSubscriptionUsed(t, subID), "legacy wrapper settlement must continue using quota amount")
	assert.Equal(t, 10_000-999, getTokenRemainQuota(t, tokenID), "token key quota still settles wallet quota")
	assert.Equal(t, int64(989), relayInfo.SubscriptionPostDelta)
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

func TestPostTextConsumeQuotaPreservesLegacySubscriptionQuota(t *testing.T) {
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
	preConsumeForBillingTest(t, ctx, relayInfo, 10)

	PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 10, TotalTokens: 10}, nil)

	assert.Equal(t, int64(20), getSubscriptionUsed(t, subID))
	assert.Equal(t, 10_000-20, getTokenRemainQuota(t, tokenID), "token key quota still uses wallet quota")
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

	PostTextConsumeQuota(ctx, relayInfo, nil, nil)

	assert.Equal(t, int64(6), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000-6, getTokenRemainQuota(t, tokenID), "token key quota must use estimated wallet quota from summary")
	log := getLastLog(t)
	require.NotNil(t, log)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	assert.Equal(t, true, other["usage_estimated"])
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
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	PostTextConsumeQuota(ctx, relayInfo, &dto.Usage{PromptTokens: 6, TotalTokens: 6}, nil)

	assert.Equal(t, int64(0), getSubscriptionTokenUsed(t, subID))
	assert.Equal(t, 10_000-6, getTokenRemainQuota(t, tokenID), "token key quota still uses wallet quota")
}

func TestWalletBillingStillUsesQuota(t *testing.T) {
	truncate(t)
	const userID = 8021
	const tokenID = 8022
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-wallet", 10_000)

	ctx := newBillingTestContext(t)
	relayInfo := newBillingTestRelayInfo(userID, tokenID, "sk-wallet", "req-wallet", "wallet_only")
	preConsumeForBillingTest(t, ctx, relayInfo, 6)

	require.NoError(t, SettleBillingWithInput(ctx, relayInfo, BillingSettleInput{
		WalletQuota:        999,
		SubscriptionTokens: 8,
		UsageEstimated:     false,
	}))

	assert.Equal(t, 10_000-999, getUserQuota(t, userID))
	assert.Equal(t, 10_000-999, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 999, getTokenUsedQuota(t, tokenID))
}

func TestTokenKeyQuotaStillUsesQuotaWhenSubscriptionUsesTokens(t *testing.T) {
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
	assert.Equal(t, 10_000-999, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 999, getTokenUsedQuota(t, tokenID))
}
