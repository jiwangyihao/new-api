package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/kling"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayTaskSubmitFreeModelStillRequiresSubscription(t *testing.T) {
	setupMidjourneySubscriptionOnlyTestDB(t)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"free-task-model":0}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(ratio_setting.InitRatioSettings)

	const userID = 9821
	const tokenID = 9822
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "task_free_no_sub", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-task-free-no-sub", Status: common.TokenStatusEnabled, RemainQuota: 1000000}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", bytes.NewBufferString(`{"model":"free-task-model","prompt":"hello"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	ctx.Set("platform", "50")

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatTask,
		RelayMode:       relayconstant.RelayModeVideoSubmit,
		RequestId:       "req-task-free-no-sub",
		UserId:          userID,
		UserQuota:       1000000,
		TokenId:         tokenID,
		TokenKey:        "sk-task-free-no-sub",
		OriginModelName: "free-task-model",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeKling, UpstreamModelName: "free-task-model"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		UserGroup:       "default",
		UsingGroup:      "default",
		UserSetting:     dto.UserSetting{BillingPreference: "wallet_only", AcceptUnsetRatioModel: true},
	}

	result, taskErr := RelayTaskSubmit(ctx, info)

	require.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "分销订阅扣费")
	assert.Empty(t, info.BillingSource)
	assert.Nil(t, info.Billing)
}

func TestRelayTaskSubmitFreeModelWithSubscriptionDoesNotReachUnsupportedUpstream(t *testing.T) {
	setupMidjourneySubscriptionOnlyTestDB(t)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"free-task-model":0}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(ratio_setting.InitRatioSettings)

	const userID = 9831
	const tokenID = 9832
	const planID = 9833
	const subID = 9834
	code := "task-free-sub"
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "task_free_sub", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-task-free-sub", Status: common.TokenStatusEnabled, RemainQuota: 1000000}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Task Free Plan", Enabled: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: subID, UserId: userID, PlanId: planID, TokenLimit: 1000, TokenUsed: 0, ConcurrencyLimit: 1, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order"}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", bytes.NewBufferString(`{"model":"free-task-model","prompt":"hello"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	ctx.Set("platform", "50")

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatTask,
		RelayMode:       relayconstant.RelayModeVideoSubmit,
		RequestId:       "req-task-free-sub",
		UserId:          userID,
		UserQuota:       1000000,
		TokenId:         tokenID,
		TokenKey:        "sk-task-free-sub",
		OriginModelName: "free-task-model",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeKling, UpstreamModelName: "free-task-model"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		UserGroup:       "default",
		UsingGroup:      "default",
		UserSetting:     dto.UserSetting{BillingPreference: "subscription_first", AcceptUnsetRatioModel: true},
	}

	result, taskErr := RelayTaskSubmit(ctx, info)

	require.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "分销订阅扣费")
	assert.Empty(t, info.BillingSource)
	assert.Nil(t, info.Billing)
	assert.Equal(t, int64(0), getMidjourneySubscriptionTokenUsed(t, subID))
}

func getMidjourneySubscriptionTokenUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("token_used").Where("id = ?", id).First(&sub).Error)
	return sub.TokenUsed
}

func TestKlingAdaptorStillSatisfiesTaskAdaptor(t *testing.T) {
	var _ interface {
		Init(*relaycommon.RelayInfo)
		ValidateRequestAndSetAction(*gin.Context, *relaycommon.RelayInfo) *dto.TaskError
		EstimateBilling(*gin.Context, *relaycommon.RelayInfo) map[string]float64
		BuildRequestURL(*relaycommon.RelayInfo) (string, error)
		BuildRequestHeader(*gin.Context, *http.Request, *relaycommon.RelayInfo) error
		BuildRequestBody(*gin.Context, *relaycommon.RelayInfo) (io.Reader, error)
		DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (*http.Response, error)
		DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (string, []byte, *dto.TaskError)
	} = (*kling.TaskAdaptor)(nil)
}
