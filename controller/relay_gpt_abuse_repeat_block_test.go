package controller

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
	sqlmysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
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

	db := openRouteRelayTestDatabase(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o":1}`))
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Model{}, &model.ChannelGroup{}, &model.ChannelGroupChannel{}, &model.TokenGroupBinding{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.TokenLimitPreConsumeRecord{}, &model.GPTAbuseSignalLog{}, &model.GPTAbuseRepeatBlockLog{}, &model.GPTAbuseUserSuspension{}, &model.GPTAbuseWarningReset{}, &model.Log{}))
	require.NoError(t, model.MigrateAvailability(db))

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
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channelID).Update("settings", `{"supported_endpoint_types":["openai","openai-response"]}`).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: channelID, Enabled: true}).Error)

	t.Cleanup(func() {
		model.FlushSubscriptionTokenDeltaUpdates()
		model.FlushConsumeLogUpdates()
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

func TestRelayBusinessRejectionStopsRetryAndBlocksRepeatedRequest(t *testing.T) {
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
	var rejection struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &rejection))
	assert.Equal(t, "cyber_policy", rejection.Error.Code)
	// A business rejection is not retried, but the recorded warning still
	// prevents a later identical request from reaching upstream.
	blocked := service.CheckGPTAbuseRepeatBlock(c, nil)
	require.NotNil(t, blocked)
	assert.Equal(t, types.ErrorCodeGPTAbuseRepeatedWarningRequest, blocked.GetErrorCode())
	var signalCount int64
	require.NoError(t, model.DB.Model(&model.GPTAbuseSignalLog{}).Count(&signalCount).Error)
	assert.Equal(t, int64(1), signalCount)
	var repeatCount int64
	require.NoError(t, model.DB.Model(&model.GPTAbuseRepeatBlockLog{}).Count(&repeatCount).Error)
	assert.Equal(t, int64(1), repeatCount)
}

func TestRelaySkipsGPTAbuseRepeatBlockWhenDisabled(t *testing.T) {
	var upstreamCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req-upstream-repeat-disabled")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"This request has been flagged for possible cybersecurity risk.","type":"invalid_request_error","code":"cyber_policy"}}`))
	}))
	defer upstream.Close()
	setupRelayGPTAbuseRepeatBlockTest(t, upstream.URL)
	service.GPTAbuseRepeatBlockEnabled = false

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"blocked"}]}`
	channel := &model.Channel{Id: 87103, Type: constant.ChannelTypeOpenAI, Key: "sk-upstream", Status: common.ChannelStatusEnabled, Name: "relay-repeat-channel", Models: "gpt-4o", BaseURL: common.GetPointer(upstream.URL), AutoBan: common.GetPointer(0)}
	for _, requestID := range []string{"req-relay-repeat-disabled-1", "req-relay-repeat-disabled-2"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.RequestIdKey, requestID)
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
		require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "gpt-4o"))

		Relay(c, types.RelayFormatOpenAI)

		require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		assert.NotContains(t, recorder.Body.String(), string(types.ErrorCodeGPTAbuseRepeatedWarningRequest))
	}
	assert.Equal(t, int32(2), atomic.LoadInt32(&upstreamCalls))
	var signalCount int64
	require.NoError(t, model.DB.Model(&model.GPTAbuseSignalLog{}).Count(&signalCount).Error)
	assert.Equal(t, int64(2), signalCount)
	var repeatCount int64
	require.NoError(t, model.DB.Model(&model.GPTAbuseRepeatBlockLog{}).Count(&repeatCount).Error)
	assert.Zero(t, repeatCount)
}

// TEST_ROUTE_SQL_DSN points only to a disposable loopback database server.
// Each scenario owns a fresh database and never mutates the supplied database.
func openRouteRelayTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	previousMaster, previousSQLite := common.IsMasterNode, common.SQLitePath
	t.Cleanup(func() {
		common.IsMasterNode, common.SQLitePath = previousMaster, previousSQLite
	})
	common.IsMasterNode = false
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = false, false, false
	t.Setenv("LOG_SQL_DSN", "")
	dsn := os.Getenv("TEST_ROUTE_SQL_DSN")
	if dsn == "" {
		common.SQLitePath = "file:route_" + common.GetUUID() + "?mode=memory&cache=shared"
		t.Setenv("SQL_DSN", "local")
	} else {
		name := "route_port_" + common.GetUUID()
		var admin *gorm.DB
		var err error
		if strings.HasPrefix(dsn, "postgres") {
			parsed, parseErr := url.Parse(dsn)
			require.NoError(t, parseErr)
			require.True(t, net.ParseIP(parsed.Hostname()).IsLoopback(), "verification must use an isolated loopback server")
			admin, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
			parsed.Path = "/" + name
			dsn = parsed.String()
		} else {
			parsed, parseErr := sqlmysql.ParseDSN(dsn)
			require.NoError(t, parseErr)
			require.Equal(t, "tcp", parsed.Net)
			host, _, splitErr := net.SplitHostPort(parsed.Addr)
			require.NoError(t, splitErr)
			require.True(t, net.ParseIP(host).IsLoopback(), "verification must use an isolated loopback server")
			admin, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, admin.Exec("CREATE DATABASE "+name+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error)
			parsed.DBName = name
			dsn = parsed.FormatDSN()
		}
		t.Cleanup(func() {
			require.NoError(t, admin.Exec("DROP DATABASE "+name).Error)
			connection, err := admin.DB()
			require.NoError(t, err)
			require.NoError(t, connection.Close())
		})
		t.Setenv("SQL_DSN", dsn)
	}
	require.NoError(t, model.InitDB())
	db := model.DB
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	return db
}

func TestRouteHealthResponsesPreservesProductionAccounting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		first      string
		status     int
		backupFail bool
		mismatch   bool
		calls      int64
		credits    int64
		terminal   string
	}{
		{name: "prelude failover", calls: 2, credits: 20, terminal: "response.completed"},
		{name: "HTTP rejection", status: http.StatusServiceUnavailable, calls: 2, credits: 20, terminal: "response.completed"},
		{name: "output commits route", first: `{"type":"response.output_text.delta","delta":"partial"}`, calls: 1, credits: 20, terminal: "response.failed"},
		{name: "tool execution blocks replay", first: `{"type":"response.code_interpreter_call.in_progress","item_id":"tool1","output_index":0}`, calls: 1, terminal: "response.failed"},
		{name: "different billing profile cannot failover", mismatch: true, calls: 1, terminal: "response.failed"},
		{name: "exhausted routes refund", backupFail: true, calls: 2, terminal: "response.failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var firstCalls, backupCalls atomic.Int64
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				firstCalls.Add(1)
				if tc.status != 0 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, `{"error":{"type":"server_error","code":"server_error","message":"unavailable"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"first\",\"status\":\"in_progress\"}}\n\n: PING\n\n")
				w.(http.Flusher).Flush()
				if tc.first != "" {
					_, _ = fmt.Fprintf(w, "data: %s\n\n", tc.first)
				}
				usage := "null"
				if tc.credits > 0 && tc.calls == 1 {
					usage = `{"input_tokens":12,"output_tokens":8,"total_tokens":20}`
				}
				_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.failed\",\"sequence_number\":2,\"response\":{\"id\":\"first\",\"status\":\"failed\",\"usage\":%s,\"error\":{\"type\":\"server_error\",\"code\":\"server_error\",\"message\":\"unavailable\"}}}\n\ndata: [DONE]\n\n", usage)
			}))
			t.Cleanup(primary.Close)
			setupRelayGPTAbuseRepeatBlockTest(t, primary.URL)
			service.GPTAbuseRepeatBlockEnabled = false
			operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 599}}
			previousCount, previousLog, previousBatch := constant.CountToken, common.LogConsumeEnabled, common.BatchUpdateEnabled
			constant.CountToken, common.LogConsumeEnabled, common.BatchUpdateEnabled = false, true, false
			t.Cleanup(func() {
				constant.CountToken, common.LogConsumeEnabled, common.BatchUpdateEnabled = previousCount, previousLog, previousBatch
			})
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 87102).Updates(map[string]any{"token_limit_enabled": true, "token_limit": 1000}).Error)
			group := &model.ChannelGroup{Name: model.DefaultChannelGroupName, Enabled: true}
			require.NoError(t, group.Insert())
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 87103).Update("priority", 10).Error)
			require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 87103).Update("priority", 10).Error)
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				backupCalls.Add(1)
				body, err := io.ReadAll(r.Body)
				if !assert.NoError(t, err) {
					return
				}
				var request dto.OpenAIResponsesRequest
				if !assert.NoError(t, common.Unmarshal(body, &request)) {
					return
				}
				assert.Equal(t, "gpt-4o", request.Model)
				w.Header().Set("Content-Type", "text/event-stream")
				if tc.backupFail {
					_, _ = io.WriteString(w, "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"backup\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"unavailable\"}}}\n\n")
					return
				}
				_, _ = io.WriteString(w, "data: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"backup\",\"status\":\"in_progress\"}}\n\ndata: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"delta\":\"recovered\"}\n\ndata: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"backup\",\"status\":\"completed\",\"usage\":{\"input_tokens\":12,\"output_tokens\":8,\"total_tokens\":20}}}\n\ndata: [DONE]\n\n")
			}))
			t.Cleanup(backup.Close)
			channel := &model.Channel{Id: 87106, Type: constant.ChannelTypeOpenAI, Key: "backup-key", Status: common.ChannelStatusEnabled, Name: "route-backup", Models: "gpt-4o", BaseURL: &backup.URL, AutoBan: common.GetPointer(0)}
			channel.OtherSettings = `{"supported_endpoint_types":["openai","openai-response"]}`
			if tc.mismatch {
				channel.CreditBillingMode, channel.FixedRequestCredits = "fixed_request", 30
			}
			require.NoError(t, model.DB.Create(channel).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: model.DefaultChannelGroupName, Model: "gpt-4o", ChannelId: channel.Id, Enabled: true}).Error)
			model.RefreshEndpointSupportCache()
			engine := gin.New()
			engine.POST("/v1/responses", middleware.BodyStorageCleanup(), func(c *gin.Context) {
				c.Set(common.RequestIdKey, "route-test-"+common.GetUUID())
				common.SetContextKey(c, constant.ContextKeyUserId, 87101)
				common.SetContextKey(c, constant.ContextKeyUserName, "relay-repeat-user")
				common.SetContextKey(c, constant.ContextKeyUserQuota, 100000)
				common.SetContextKey(c, constant.ContextKeyTokenId, 87102)
				common.SetContextKey(c, constant.ContextKeyTokenKey, "sk-relay-repeat")
				common.SetContextKey(c, constant.ContextKeyTokenGroups, []string{model.DefaultChannelGroupName})
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "subscription_only"})
				c.Next()
			}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIResponses) })
			request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-4o","input":"test","stream":true,"max_output_tokens":10}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.Equal(t, int64(1), firstCalls.Load())
			assert.Equal(t, tc.calls-1, backupCalls.Load())
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"`+tc.terminal+`"`), recorder.Body.String())
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.created"`), recorder.Body.String())
			if tc.terminal == "response.completed" {
				assert.NotContains(t, recorder.Body.String(), "response.failed")
			}
			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				model.FlushSubscriptionTokenDeltaUpdates()
				model.FlushConsumeLogUpdates()
				var sub model.UserSubscription
				var token model.Token
				if !assert.NoError(collect, model.DB.First(&sub, 87105).Error) || !assert.NoError(collect, model.DB.First(&token, 87102).Error) {
					return
				}
				assert.Equal(collect, tc.credits, sub.TokenUsed)
				assert.Equal(collect, tc.credits, token.TokenUsed)
				assert.Equal(collect, 100000, token.RemainQuota, "subscription-only billing must not charge token wallet quota")
			}, 3*time.Second, 10*time.Millisecond)
			var user model.User
			require.NoError(t, model.DB.First(&user, 87101).Error)
			assert.Equal(t, 100000, user.Quota, "subscription-only billing must not charge the wallet")
		})
	}
}
