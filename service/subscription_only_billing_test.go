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
			assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
			assert.True(t, strings.Contains(apiErr.Error(), "subscription") || strings.Contains(apiErr.Error(), "active subscription is required"), apiErr.Error())
			assert.Equal(t, 1_000_000, getUserQuota(t, userID))
			assert.Equal(t, 1_000_000, getTokenRemainQuota(t, tokenID))
		})
	}
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
