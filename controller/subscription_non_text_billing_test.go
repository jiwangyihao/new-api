package controller

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayTaskDoesNotPreConsumeDistributorSubscription(t *testing.T) {
	originalDB := model.DB
	db := setupModelListControllerTestDB(t)
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))

	const userID = 9101
	const tokenID = 9102
	const subID = 9103
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "task_dist", Quota: 10_000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-task-dist", Status: common.TokenStatusEnabled, RemainQuota: 10_000}).Error)
	code := "controller-task-dist"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9104, Title: "Task Distributor", Enabled: true, MonthlyTokenLimit: 1_000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id:               subID,
		PlanId:           9104,
		UserId:           userID,
		TokenLimit:       1_000,
		ConcurrencyLimit: 1,
		Status:           "active",
		StartTime:        time.Now().Unix(),
		EndTime:          time.Now().Add(24 * time.Hour).Unix(),
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/videos/generations", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserId, userID)
	common.SetContextKey(ctx, constant.ContextKeyUserQuota, 10_000)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, tokenID)
	common.SetContextKey(ctx, constant.ContextKeyTokenKey, "sk-task-dist")
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "video-model")
	common.SetContextKey(ctx, constant.ContextKeyEstimatedTokens, 6)
	common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "subscription_first"})

	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatTask,
		RelayMode:       relayconstant.RelayModeVideoSubmit,
		RequestId:       "req-task-dist",
		UserId:          userID,
		UserQuota:       10_000,
		TokenId:         tokenID,
		TokenKey:        "sk-task-dist",
		OriginModelName: "video-model",
		UserSetting:     dto.UserSetting{BillingPreference: "subscription_first"},
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}
	relayInfo.SetEstimatePromptTokens(6)

	apiErr := service.PreConsumeBilling(ctx, 100, relayInfo)
	require.Nil(t, apiErr)
	assert.Equal(t, service.BillingSourceWallet, relayInfo.BillingSource)
	assert.Zero(t, relayInfo.SubscriptionId)

	task := model.InitTask(constant.TaskPlatform("video"), relayInfo)
	task.PrivateData.BillingSource = relayInfo.BillingSource
	task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
	assert.NotEqual(t, service.BillingSourceSubscription, task.PrivateData.BillingSource)
	assert.Zero(t, task.PrivateData.SubscriptionId)
}
