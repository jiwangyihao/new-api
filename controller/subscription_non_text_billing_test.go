package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRelayTaskDistributorDoesNotFallbackToWallet(t *testing.T) {
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
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "不支持分销订阅扣费")
	assert.Empty(t, relayInfo.BillingSource)
	assert.Zero(t, relayInfo.SubscriptionId)
	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, 10_000, getControllerTestUserQuota(t, userID))
	assert.Equal(t, 10_000, getControllerTestTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(0), getControllerTestSubscriptionTokenUsed(t, subID))

	task := model.InitTask(constant.TaskPlatform("video"), relayInfo)
	task.PrivateData.BillingSource = relayInfo.BillingSource
	task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
	assert.NotEqual(t, service.BillingSourceSubscription, task.PrivateData.BillingSource)
	assert.Zero(t, task.PrivateData.SubscriptionId)
}

func TestRelayTaskDistributorDoesNotChargeWalletOrTokenKey(t *testing.T) {
	originalDB := model.DB
	db := setupModelListControllerTestDB(t)
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))

	const userID = 9201
	const tokenID = 9202
	const subID = 9203
	seedQuota := 100
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "task_exact", Quota: seedQuota, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-task-exact", Status: common.TokenStatusEnabled, RemainQuota: seedQuota}).Error)
	code := "controller-task-exact"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9204, Title: "Task Distributor", Enabled: true, MonthlyTokenLimit: 1_000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subID, PlanId: 9204, UserId: userID, TokenLimit: 1_000, ConcurrencyLimit: 1, Status: "active", StartTime: time.Now().Unix(), EndTime: time.Now().Add(24 * time.Hour).Unix()}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/videos/generations", nil)
	relayInfo := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatTask, RelayMode: relayconstant.RelayModeVideoSubmit, RequestId: "req-task-exact", UserId: userID, UserQuota: seedQuota, TokenId: tokenID, TokenKey: "sk-task-exact", OriginModelName: "video-model", UserSetting: dto.UserSetting{BillingPreference: "subscription_first"}, ChannelMeta: &relaycommon.ChannelMeta{}}
	relayInfo.SetEstimatePromptTokens(6)

	apiErr := service.PreConsumeBilling(ctx, seedQuota, relayInfo)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "不支持分销订阅扣费")
	assert.Empty(t, relayInfo.BillingSource)
	assert.Zero(t, relayInfo.SubscriptionId)
	assert.Nil(t, relayInfo.Billing)
	assert.Equal(t, seedQuota, getControllerTestUserQuota(t, userID))

	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, seedQuota, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Equal(t, int64(0), getControllerTestSubscriptionTokenUsed(t, subID))
}

func TestAsyncTaskRefundDoesNotWriteAccountBalance(t *testing.T) {
	originalDB := model.DB
	db := setupModelListControllerTestDB(t)
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Channel{}, &model.Task{}, &model.Midjourney{}, &model.Log{}))

	const userID = 9301
	const tokenID = 9302
	const channelID = 9303
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "async_refund", Quota: 4000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-async-refund", Status: common.TokenStatusEnabled, RemainQuota: 9000}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: channelID, Name: "async-channel", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled}).Error)

	task := &model.Task{TaskID: "video-refund", UserId: userID, ChannelId: channelID, Quota: 500000, Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{BillingSource: service.BillingSourceWallet, TokenId: tokenID}}
	require.NoError(t, model.DB.Create(task).Error)

	err := updateVideoSingleTask(context.Background(), fakeVideoTaskAdaptor{}, &model.Channel{Id: channelID, Type: constant.ChannelTypeOpenAI, Key: "sk-channel"}, task.TaskID, map[string]*model.Task{task.TaskID: task})

	require.NoError(t, err)
	assert.Equal(t, 4000, getControllerTestUserQuota(t, userID))
	assert.Equal(t, 9000, getControllerTestTokenRemainQuota(t, tokenID))
	assertControllerSystemLogContains(t, userID, "legacy wallet refund requires manual handling")

	serviceTask := &model.Task{TaskID: "service-refund", UserId: userID, ChannelId: channelID, Quota: 500000, Status: model.TaskStatusFailure, PrivateData: model.TaskPrivateData{BillingSource: service.BillingSourceWallet, TokenId: tokenID}}
	service.RefundTaskQuota(context.Background(), serviceTask, "video failed")
	assert.Equal(t, 4000, getControllerTestUserQuota(t, userID))
	assert.Equal(t, 9000, getControllerTestTokenRemainQuota(t, tokenID))
}

type fakeVideoTaskAdaptor struct{}

func (fakeVideoTaskAdaptor) Init(_ *relaycommon.RelayInfo) {}
func (fakeVideoTaskAdaptor) ValidateRequestAndSetAction(_ *gin.Context, _ *relaycommon.RelayInfo) *dto.TaskError {
	return nil
}
func (fakeVideoTaskAdaptor) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	return nil
}
func (fakeVideoTaskAdaptor) AdjustBillingOnSubmit(_ *relaycommon.RelayInfo, _ []byte) map[string]float64 {
	return nil
}
func (fakeVideoTaskAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}
func (fakeVideoTaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) { return "", nil }
func (fakeVideoTaskAdaptor) BuildRequestHeader(_ *gin.Context, _ *http.Request, _ *relaycommon.RelayInfo) error {
	return nil
}
func (fakeVideoTaskAdaptor) BuildRequestBody(_ *gin.Context, _ *relaycommon.RelayInfo) (io.Reader, error) {
	return nil, nil
}
func (fakeVideoTaskAdaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ io.Reader) (*http.Response, error) {
	return nil, nil
}
func (fakeVideoTaskAdaptor) DoResponse(_ *gin.Context, _ *http.Response, _ *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	return "", nil, nil
}
func (fakeVideoTaskAdaptor) GetModelList() []string { return nil }
func (fakeVideoTaskAdaptor) GetChannelName() string { return "fake" }
func (fakeVideoTaskAdaptor) FetchTask(_, _ string, _ map[string]any, _ string) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":"FAILURE","reason":"failed"}`))}, nil
}
func (fakeVideoTaskAdaptor) ParseTaskResult(_ []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{TaskID: "video-refund", Status: model.TaskStatusFailure, Reason: "failed"}, nil
}

func assertControllerSystemLogContains(t *testing.T, userID int, expected string) {
	t.Helper()
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeSystem).Order("id DESC").First(&log).Error)
	assert.Contains(t, log.Content, expected)
}

func getControllerTestUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return user.Quota
}

func getControllerTestTokenRemainQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("remain_quota").Where("id = ?", id).First(&token).Error)
	return token.RemainQuota
}

func getControllerTestSubscriptionTokenUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("token_used").Where("id = ?", id).First(&sub).Error)
	return sub.TokenUsed
}
