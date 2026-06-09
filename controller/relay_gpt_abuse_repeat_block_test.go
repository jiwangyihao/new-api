package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRelayGPTAbuseRepeatBlockTest(t *testing.T, upstreamURL string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	oldRetryTimes := common.RetryTimes
	oldGPTAbuseLimitEnabled := common.GPTAbuseLimitEnabled
	oldRepeatBlockEnabled := service.GPTAbuseRepeatBlockEnabled
	oldRepeatBlockTTL := service.GPTAbuseRepeatBlockTTLSeconds
	oldRepeatBlockRequireRedis := service.GPTAbuseRepeatBlockRequireRedis
	oldRetryStatusRanges := operation_setting.AutomaticRetryStatusCodeRanges
	oldModelRatio := ratio_setting.ModelRatio2JSONString()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	common.RetryTimes = 1
	common.GPTAbuseLimitEnabled = false
	service.GPTAbuseRepeatBlockEnabled = true
	service.GPTAbuseRepeatBlockTTLSeconds = 900
	service.GPTAbuseRepeatBlockRequireRedis = false
	service.ResetGPTAbuseRepeatBlockCacheForTest()
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: http.StatusBadRequest, End: http.StatusBadRequest}}
	service.InitHttpClient()

	dsn := "file:" + strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o":1}`))
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.GPTAbuseSignalLog{}, &model.GPTAbuseRepeatBlockLog{}, &model.GPTAbuseUserSuspension{}, &model.GPTAbuseWarningReset{}, &model.Log{}))

	const userID = 87101
	const tokenID = 87102
	const channelID = 87103
	const planID = 87104
	const subscriptionID = 87105
	planCode := "relay-repeat-block-plan"
	autoBan := 0
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "relay-repeat-user", Email: "relay-repeat@example.com", Status: common.UserStatusEnabled, AffCode: "relay-repeat", Quota: 100000}).Error)
	require.NoError(t, db.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-relay-repeat", Name: "relay-repeat-token", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100000}).Error)
	require.NoError(t, db.Create(&model.SubscriptionPlan{Id: planID, Title: "Relay Repeat Plan", Enabled: true, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &planCode}).Error)
	model.InvalidateSubscriptionPlanCache(planID)
	require.NoError(t, db.Create(&model.UserSubscription{Id: subscriptionID, UserId: userID, PlanId: planID, Status: "active", GrantReason: "order", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), AmountTotal: 1, TokenLimit: 1000}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: channelID, Type: constant.ChannelTypeOpenAI, Key: "sk-upstream", Status: common.ChannelStatusEnabled, Name: "relay-repeat-channel", Models: "gpt-4o", BaseURL: common.GetPointer(upstreamURL), AutoBan: &autoBan}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: channelID, Enabled: true}).Error)

	t.Cleanup(func() {
		service.ResetGPTAbuseRepeatBlockCacheForTest()
		model.ClearPrimaryBillableSubscriptionCacheForTest()
		model.InvalidateSubscriptionPlanCache(planID)
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.RedisEnabled = oldRedisEnabled
		common.RetryTimes = oldRetryTimes
		common.GPTAbuseLimitEnabled = oldGPTAbuseLimitEnabled
		service.GPTAbuseRepeatBlockEnabled = oldRepeatBlockEnabled
		service.GPTAbuseRepeatBlockTTLSeconds = oldRepeatBlockTTL
		service.GPTAbuseRepeatBlockRequireRedis = oldRepeatBlockRequireRedis
		operation_setting.AutomaticRetryStatusCodeRanges = oldRetryStatusRanges
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
	})
}

func TestRelayChecksGPTAbuseRepeatBlockOnEveryRetryAttempt(t *testing.T) {
	var upstreamCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&upstreamCalls, 1) > 1 {
			t.Fatalf("repeat-block hit must not request upstream again")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req-upstream-repeat")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"This request has been flagged for possible cybersecurity risk.","type":"invalid_request_error","code":"cyber_policy"}}`))
	}))
	defer upstream.Close()
	setupRelayGPTAbuseRepeatBlockTest(t, upstream.URL)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"blocked"}]}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, "req-relay-repeat")
	common.SetContextKey(c, constant.ContextKeyUserId, 87101)
	common.SetContextKey(c, constant.ContextKeyUserEmail, "relay-repeat@example.com")
	common.SetContextKey(c, constant.ContextKeyUserName, "relay-repeat-user")
	common.SetContextKey(c, constant.ContextKeyUserQuota, 100000)
	common.SetContextKey(c, constant.ContextKeyTokenId, 87102)
	common.SetContextKey(c, constant.ContextKeyTokenKey, "sk-relay-repeat")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "subscription_only"})
	c.Set("token_name", "relay-repeat-token")
	channel := &model.Channel{Id: 87103, Type: constant.ChannelTypeOpenAI, Key: "sk-upstream", Status: common.ChannelStatusEnabled, Name: "relay-repeat-channel", Models: "gpt-4o", BaseURL: common.GetPointer(upstream.URL), AutoBan: common.GetPointer(0)}
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "gpt-4o"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	assert.Equal(t, int32(1), atomic.LoadInt32(&upstreamCalls))
	assert.Contains(t, recorder.Body.String(), string(types.ErrorCodeGPTAbuseRepeatedWarningRequest))
	assert.Contains(t, recorder.Body.String(), "not sent upstream again")
	var signalCount int64
	require.NoError(t, model.DB.Model(&model.GPTAbuseSignalLog{}).Count(&signalCount).Error)
	assert.Equal(t, int64(1), signalCount)
	var repeatCount int64
	require.NoError(t, model.DB.Model(&model.GPTAbuseRepeatBlockLog{}).Count(&repeatCount).Error)
	assert.Equal(t, int64(1), repeatCount)
}
