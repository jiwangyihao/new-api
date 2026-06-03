package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSubscriptionOnlyBillingTestDB(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Migrator().DropTable(&model.SubscriptionPreConsumeRecord{}, &model.UserSubscription{}, &model.SubscriptionPlan{}, &model.Token{}, &model.User{}))
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))
}

func subscriptionOnlyTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Set("token_quota", 0)
	return ctx
}

func subscriptionOnlyRelayInfo(userID int, tokenID int, tokenKey string, pref string) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
		RelayMode:       relayconstant.RelayModeChatCompletions,
		RequestId:       fmt.Sprintf("req-sub-only-%d", time.Now().UnixNano()),
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        tokenKey,
		OriginModelName: "gpt-4o-mini",
		UserSetting:     dto.UserSetting{BillingPreference: pref},
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}
	info.SetEstimatePromptTokens(10)
	return info
}

func TestNewBillingSessionRequiresSubscriptionWhenWalletPreferenceSet(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	const userID = 9301
	const tokenID = 9302
	const tokenKey = "sk-wallet-pref"
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "wallet_pref", Quota: 1_000_000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: tokenKey, Status: common.TokenStatusEnabled, RemainQuota: 1_000_000}).Error)

	for _, pref := range []string{"wallet_first", "wallet_only", "subscription_first", ""} {
		t.Run(pref, func(t *testing.T) {
			ctx := subscriptionOnlyTestContext(t)
			relayInfo := subscriptionOnlyRelayInfo(userID, tokenID, tokenKey, pref)
			session, apiErr := NewBillingSession(ctx, relayInfo, 10)

			require.Nil(t, session)
			require.NotNil(t, apiErr)
			assert.Equal(t, types.ErrorCodeSubscriptionRequired, apiErr.GetErrorCode())
			assert.True(t, strings.Contains(apiErr.Error(), "subscription") || strings.Contains(apiErr.Error(), "active subscription is required"), apiErr.Error())
			assert.Equal(t, 1_000_000, getUserQuota(t, userID))
			assert.Equal(t, 1_000_000, getTokenRemainQuota(t, tokenID))
		})
	}
}

func TestNewBillingSessionReturnsSubscriptionRequiredCode(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	const userID = 9305
	const tokenID = 9306
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "no_sub_code", Quota: 1_000_000, Status: common.UserStatusEnabled, AffCode: "aff9305"}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-no-sub-code", Status: common.TokenStatusEnabled, RemainQuota: 1_000_000}).Error)
	ctx := subscriptionOnlyTestContext(t)
	relayInfo := subscriptionOnlyRelayInfo(userID, tokenID, "sk-no-sub-code", "wallet_only")

	session, apiErr := NewBillingSession(ctx, relayInfo, 10)

	require.Nil(t, session)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeSubscriptionRequired, apiErr.GetErrorCode())
	oaiErr := apiErr.ToOpenAIError()
	assert.Equal(t, "insufficient_quota", oaiErr.Type)
	assert.Equal(t, string(types.ErrorCodeSubscriptionRequired), oaiErr.Code)
}

func TestNewBillingSessionReturnsSubscriptionTokenExhaustedCode(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	const userID = 9307
	const tokenID = 9308
	const planID = 9309
	const subID = 9310
	planCode := "sub-exhausted-basic"
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "sub_exhausted", Quota: 1_000_000, Status: common.UserStatusEnabled, AffCode: "aff9307"}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-sub-exhausted", Status: common.TokenStatusEnabled, RemainQuota: 1_000_000}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Basic", Enabled: true, MonthlyTokenLimit: 10, ConcurrencyLimit: 1, BusinessCode: &planCode}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subID, UserId: userID, PlanId: planID, TokenLimit: 10, TokenUsed: 9, ConcurrencyLimit: 1, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order"}).Error)
	ctx := subscriptionOnlyTestContext(t)
	relayInfo := subscriptionOnlyRelayInfo(userID, tokenID, "sk-sub-exhausted", "subscription_only")
	relayInfo.SetEstimatePromptTokens(2)

	session, apiErr := NewBillingSession(ctx, relayInfo, 2)

	require.Nil(t, session)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeSubscriptionTokenExhausted, apiErr.GetErrorCode())
	oaiErr := apiErr.ToOpenAIError()
	assert.Equal(t, "insufficient_quota", oaiErr.Type)
	assert.Equal(t, string(types.ErrorCodeSubscriptionTokenExhausted), oaiErr.Code)
}

func TestSubscriptionBillingDoesNotConsumeTokenKeyQuota(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	const userID = 9311
	const tokenID = 9312
	const planID = 9313
	const subID = 9314
	const tokenKey = "sk-sub-only"
	planCode := "sub-only-basic"
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "sub_only", Quota: 0, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: tokenKey, Status: common.TokenStatusEnabled, RemainQuota: 0}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Basic", Enabled: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &planCode}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id:               subID,
		UserId:           userID,
		PlanId:           planID,
		TokenLimit:       1000,
		TokenUsed:        0,
		ConcurrencyLimit: 1,
		Status:           "active",
		StartTime:        time.Now().Add(-time.Hour).Unix(),
		EndTime:          time.Now().Add(time.Hour).Unix(),
		GrantReason:      "order",
		Source:           "order",
	}).Error)

	ctx := subscriptionOnlyTestContext(t)
	relayInfo := subscriptionOnlyRelayInfo(userID, tokenID, tokenKey, "wallet_only")
	session, apiErr := NewBillingSession(ctx, relayInfo, 10)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	require.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
	require.NoError(t, session.SettleWithInput(BillingSettleInput{WalletQuota: 20, SubscriptionTokens: 20}))

	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, 0, token.RemainQuota)
	assert.Equal(t, 0, token.UsedQuota)
	assert.Equal(t, int64(20), getSubscriptionTokenUsed(t, subID))
}

func TestPostConsumeQuotaSubscriptionDoesNotConsumeTokenKeyQuota(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	const userID = 9321
	const tokenID = 9322
	const planID = 9323
	const subID = 9324
	planCode := "sub-post-basic"
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "sub_post", Quota: 0, Status: common.UserStatusEnabled, AffCode: "aff9321"}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-sub-post", Status: common.TokenStatusEnabled, RemainQuota: 0}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Basic", Enabled: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &planCode}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subID, UserId: userID, PlanId: planID, TokenLimit: 1000, TokenUsed: 0, ConcurrencyLimit: 1, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order"}).Error)
	relayInfo := subscriptionOnlyRelayInfo(userID, tokenID, "sk-sub-post", "wallet_only")
	relayInfo.BillingSource = BillingSourceSubscription
	relayInfo.SubscriptionId = subID
	relayInfo.SubscriptionDistributorTokenBilling = true

	require.NoError(t, PostConsumeQuota(relayInfo, 7, 0, false))

	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, 0, token.RemainQuota)
	assert.Equal(t, 0, token.UsedQuota)
	assert.Equal(t, int64(7), getSubscriptionTokenUsed(t, subID))
}

func TestPostConsumeQuotaLegacySubscriptionUsesAmountUsed(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	const userID = 9331
	const tokenID = 9332
	const planID = 9333
	const subID = 9334
	rewardEligible := false
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "legacy_sub_post", Quota: 0, Status: common.UserStatusEnabled, AffCode: "aff9331"}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-legacy-sub-post", Status: common.TokenStatusEnabled, RemainQuota: 0}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Legacy", Enabled: true, TotalAmount: 100, RewardEligible: rewardEligible}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subID, UserId: userID, PlanId: planID, AmountTotal: 100, AmountUsed: 10, TokenLimit: 0, TokenUsed: 0, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order"}).Error)
	relayInfo := subscriptionOnlyRelayInfo(userID, tokenID, "sk-legacy-sub-post", "wallet_only")
	relayInfo.BillingSource = BillingSourceSubscription
	relayInfo.SubscriptionId = subID

	require.NoError(t, PostConsumeQuota(relayInfo, 7, 0, false))

	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, subID).Error)
	assert.Equal(t, int64(17), sub.AmountUsed)
	assert.Equal(t, int64(0), sub.TokenUsed)
	assert.Equal(t, int64(7), relayInfo.SubscriptionPostDelta)
}

func TestPostConsumeQuotaRejectsLegacyWalletFallback(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	const userID = 9341
	const tokenID = 9342
	const tokenKey = "sk-legacy-wallet-post"
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "legacy_wallet_post", Quota: 4000, Status: common.UserStatusEnabled, AffCode: "aff9341"}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: tokenKey, Status: common.TokenStatusEnabled, RemainQuota: 9000}).Error)
	relayInfo := subscriptionOnlyRelayInfo(userID, tokenID, tokenKey, "wallet_only")
	relayInfo.BillingSource = BillingSourceWallet

	err := PostConsumeQuota(relayInfo, 700, 0, false)

	require.ErrorIs(t, err, ErrLegacyWalletFundingDisabled)
	assert.Equal(t, 4000, getUserQuota(t, userID))
	assert.Equal(t, 9000, getTokenRemainQuota(t, tokenID))
}

func TestWalletFundingDoesNotWriteAccountBalanceForRelay(t *testing.T) {
	setupSubscriptionOnlyBillingTestDB(t)
	const userID = 9351
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "legacy_wallet_funding", Quota: 4000, Status: common.UserStatusEnabled, AffCode: "aff9351"}).Error)
	funding := &WalletFunding{userId: userID}

	require.ErrorIs(t, funding.PreConsume(100), ErrLegacyWalletFundingDisabled)
	require.ErrorIs(t, funding.Settle(100), ErrLegacyWalletFundingDisabled)
	funding.consumed = 100
	require.ErrorIs(t, funding.Refund(), ErrLegacyWalletFundingDisabled)
	assert.Equal(t, 4000, getUserQuota(t, userID))
}
