package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGPTAbuseSignalServiceTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.GPTAbuseSignalLog{}, &model.GPTAbuseUserSuspension{}))
	oldEnabled := common.GPTAbuseLimitEnabled
	oldDefault := common.GPTAbuseDefaultWarningLimit
	common.GPTAbuseLimitEnabled = true
	common.GPTAbuseDefaultWarningLimit = 1
	model.ClearPrimaryBillableSubscriptionCacheForTest()
	t.Cleanup(func() {
		model.ClearPrimaryBillableSubscriptionCacheForTest()
		common.GPTAbuseLimitEnabled = oldEnabled
		common.GPTAbuseDefaultWarningLimit = oldDefault
		for _, tableName := range []string{"gpt_abuse_user_suspensions", "gpt_abuse_signal_logs", "user_subscriptions", "subscription_plans", "users", "tokens", "channels"} {
			_ = model.DB.Exec("DELETE FROM " + tableName).Error
		}
	})
}

func newGPTAbuseSignalTestContext(path string) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Set(common.RequestIdKey, "req-local")
	common.SetContextKey(c, constant.ContextKeyUserId, 7001)
	common.SetContextKey(c, constant.ContextKeyUserEmail, "user@example.com")
	common.SetContextKey(c, constant.ContextKeyUserName, "abuse-user")
	common.SetContextKey(c, constant.ContextKeyTokenId, 8001)
	c.Set("token_name", "safe-token-name")
	common.SetContextKey(c, constant.ContextKeyChannelId, 9001)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 2)
	c.Set("channel_name", "OpenAI Primary")
	return c
}

func newGPTAbuseSignalTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		TokenId:         8001,
		UserId:          7001,
		RequestId:       "req-local",
		OriginModelName: "gpt-4o",
		RequestURLPath:  "/v1/chat/completions",
		RelayMode:       relayconstant.RelayModeChatCompletions,
		IsStream:        true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            9001,
			ChannelType:          constant.ChannelTypeOpenAI,
			ChannelMultiKeyIndex: 2,
			UpstreamModelName:    "gpt-4o-mini",
		},
	}
}

func seedGPTAbuseSubscription(t *testing.T, userID int, planLimit int) {
	t.Helper()
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "abuse-user", Email: "user@example.com", Status: common.UserStatusEnabled, AffCode: "abuse-aff"}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: userID + 100, Title: "GPT Abuse Plan", Enabled: true, ConcurrencyLimit: 1, GPTAbuseWarningLimit: planLimit}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: userID + 200, UserId: userID, PlanId: userID + 100, Status: "active", StartTime: now - 60, EndTime: now + 3600, TokenLimit: 1000, TokenUsed: 0, GrantReason: "order", Source: "order"}).Error)
}

func TestClassifyGPTAbuseSignalFromHTTPErrorCyberPolicy(t *testing.T) {
	body := []byte(`{"error":{"message":"Possible cybersecurity risk detected","type":"invalid_request_error","code":"cyber_policy"}}`)

	signal := ClassifyGPTAbuseSignalFromHTTPError(http.StatusBadRequest, body)

	assert.True(t, signal.Matched)
	assert.Equal(t, GPTAbuseKindCyberPolicy, signal.Kind)
	assert.Equal(t, GPTAbuseSeverityHigh, signal.Severity)
	assert.Equal(t, GPTAbuseSourceHTTPError, signal.Source)
	assert.Equal(t, "cyber_policy", signal.ErrorCode)
	assert.Equal(t, "invalid_request_error", signal.ErrorType)
	assert.True(t, signal.CountEligible)
}

func TestClassifyGPTAbuseSignalFromHTTPErrorInvalidPromptSafety(t *testing.T) {
	body := []byte(`{"error":{"message":"Your request was rejected as a result of our safety system. Your prompt may contain text that is not allowed by our safety system.","type":"invalid_request_error","code":"invalid_prompt"}}`)

	signal := ClassifyGPTAbuseSignalFromHTTPError(http.StatusBadRequest, body)

	assert.True(t, signal.Matched)
	assert.Equal(t, GPTAbuseKindInvalidPromptSafety, signal.Kind)
	assert.Equal(t, GPTAbuseSeverityMedium, signal.Severity)
	assert.True(t, signal.CountEligible)
}

func TestClassifyGPTAbuseSignalFromHTTPErrorExcludesRateLimit(t *testing.T) {
	body := []byte(`{"error":{"message":"Rate limit reached for requests","type":"rate_limit_error","code":"rate_limit_exceeded"}}`)

	signal := ClassifyGPTAbuseSignalFromHTTPError(http.StatusTooManyRequests, body)

	assert.False(t, signal.Matched)
	assert.False(t, signal.CountEligible)
}

func TestClassifyGPTAbuseSignalFromHTTPErrorExcludesInsufficientQuota(t *testing.T) {
	body := []byte(`{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`)

	signal := ClassifyGPTAbuseSignalFromHTTPError(http.StatusTooManyRequests, body)

	assert.False(t, signal.Matched)
	assert.False(t, signal.CountEligible)
}
func TestClassifyGPTAbuseSignalFromHTTPErrorExcludesAllowedParameter(t *testing.T) {
	body := []byte(`{"error":{"message":"Parameter 'seed' is not allowed for this model","type":"invalid_request_error","code":"invalid_request_error"}}`)
	signal := ClassifyGPTAbuseSignalFromHTTPError(http.StatusBadRequest, body)

	assert.False(t, signal.Matched)
	assert.False(t, signal.CountEligible)
}

func TestClassifyGPTAbuseSignalFromHTTPErrorExcludesContentParameterValidation(t *testing.T) {
	body := []byte(`{"error":{"message":"Unsupported parameter: messages[0].content is not allowed for this model","type":"invalid_request_error","code":"invalid_request_error"}}`)
	signal := ClassifyGPTAbuseSignalFromHTTPError(http.StatusBadRequest, body)

	assert.False(t, signal.Matched)
	assert.False(t, signal.CountEligible)
}

func TestClassifyGPTAbuseSignalFromSSETrustedAccessForCyber(t *testing.T) {
	payload := []byte(`{"type":"response.metadata","response":{"metadata":{"openai_verification_recommendation":["trusted_access_for_cyber"]}}}`)

	signal := ClassifyGPTAbuseSignalFromSSEEvent("response.metadata", payload)

	assert.True(t, signal.Matched)
	assert.Equal(t, GPTAbuseKindHighRiskCyberReroute, signal.Kind)
	assert.Equal(t, GPTAbuseSeverityHigh, signal.Severity)
	assert.Equal(t, GPTAbuseSourceSSEMetadata, signal.Source)
	assert.True(t, signal.CountEligible)
}

func TestGPTUpstreamRequestIDPrefersOpenAIRequestID(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Oneapi-Request-Id", "local-compatible")
	headers.Set("x-request-id", "req_openai")

	assert.Equal(t, "req_openai", GPTUpstreamRequestID(headers))
}

func TestResolveGPTAbuseWarningLimit(t *testing.T) {
	oldDefault := common.GPTAbuseDefaultWarningLimit
	common.GPTAbuseDefaultWarningLimit = 5
	t.Cleanup(func() { common.GPTAbuseDefaultWarningLimit = oldDefault })

	tests := []struct {
		name string
		plan *model.SubscriptionPlan
		want int
	}{
		{name: "nil plan uses default minimum", plan: nil, want: 5},
		{name: "low concurrency uses default minimum", plan: &model.SubscriptionPlan{ConcurrencyLimit: 1}, want: 5},
		{name: "high concurrency uses concurrency", plan: &model.SubscriptionPlan{ConcurrencyLimit: 10}, want: 10},
		{name: "explicit below minimum clamps", plan: &model.SubscriptionPlan{ConcurrencyLimit: 10, GPTAbuseWarningLimit: 3}, want: 5},
		{name: "explicit above minimum wins", plan: &model.SubscriptionPlan{ConcurrencyLimit: 10, GPTAbuseWarningLimit: 8}, want: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveGPTAbuseWarningLimit(tt.plan))
		})
	}
}

func TestRecordGPTAbuseSignalStoresAttributionWithoutSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseSignalServiceTest(t)
	common.GPTAbuseLimitEnabled = false
	c := newGPTAbuseSignalTestContext("/v1/chat/completions")
	info := newGPTAbuseSignalTestRelayInfo()

	RecordGPTAbuseSignal(c, info, GPTAbuseSignal{
		Matched:           true,
		Kind:              GPTAbuseKindCyberPolicy,
		Severity:          GPTAbuseSeverityHigh,
		Source:            GPTAbuseSourceHTTPError,
		StatusCode:        http.StatusBadRequest,
		ErrorCode:         "cyber_policy",
		ErrorType:         "invalid_request_error",
		UpstreamRequestId: "req-upstream",
		CountEligible:     true,
	})

	var got model.GPTAbuseSignalLog
	require.NoError(t, model.DB.First(&got).Error)
	assert.Equal(t, 7001, got.UserId)
	assert.Equal(t, "abuse-user", got.Username)
	assert.Equal(t, "user@example.com", got.UserEmail)
	assert.Equal(t, 8001, got.TokenId)
	assert.Equal(t, "safe-token-name", got.TokenName)
	assert.Equal(t, 9001, got.ChannelId)
	assert.Equal(t, constant.ChannelTypeOpenAI, got.ChannelType)
	assert.Equal(t, 2, got.ChannelMultiKeyIndex)
	assert.Equal(t, "OpenAI Primary", got.ChannelName)
	assert.Equal(t, "req-local", got.RequestId)
	assert.Equal(t, "req-upstream", got.UpstreamRequestId)
	assert.Equal(t, "/v1/chat/completions", got.Endpoint)
	assert.Equal(t, "gpt-4o", got.RequestedModel)
	assert.Equal(t, "gpt-4o-mini", got.UpstreamModel)
	assert.True(t, got.IsStream)
	assert.NotContains(t, got.DedupeKey, got.TokenName)
	assert.NotContains(t, got.DedupeKey, got.UserEmail)
}

func TestRecordGPTAbuseSignalCreatesSuspensionAtDailyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseSignalServiceTest(t)
	seedGPTAbuseSubscription(t, 7001, 2)
	c := newGPTAbuseSignalTestContext("/v1/responses")
	info := newGPTAbuseSignalTestRelayInfo()

	RecordGPTAbuseSignal(c, info, GPTAbuseSignal{Matched: true, Kind: GPTAbuseKindCyberPolicy, Severity: GPTAbuseSeverityHigh, Source: GPTAbuseSourceHTTPError, StatusCode: http.StatusBadRequest, ErrorCode: "cyber_policy", UpstreamRequestId: "req-upstream-1", CountEligible: true})
	RecordGPTAbuseSignal(c, info, GPTAbuseSignal{Matched: true, Kind: GPTAbuseKindContentPolicyViolation, Severity: GPTAbuseSeverityMedium, Source: GPTAbuseSourceHTTPError, StatusCode: http.StatusBadRequest, ErrorCode: "content_policy_violation", UpstreamRequestId: "req-upstream-2", CountEligible: true})

	var susp model.GPTAbuseUserSuspension
	require.NoError(t, model.DB.Where("user_id = ?", 7001).First(&susp).Error)
	assert.Equal(t, model.GPTAbuseSuspensionStatusActive, susp.Status)
	assert.Equal(t, 2, susp.DailyCount)
	assert.Equal(t, 2, susp.DailyLimit)
	_, dayEnd := model.GPTAbuseDayWindow(common.GetTimestamp())
	assert.Equal(t, dayEnd, susp.SuspendedUntil)
}

func TestRecordGPTAbuseSignalDoesNotSuspendWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseSignalServiceTest(t)
	common.GPTAbuseLimitEnabled = false
	seedGPTAbuseSubscription(t, 7001, 1)
	c := newGPTAbuseSignalTestContext("/v1/responses")
	info := newGPTAbuseSignalTestRelayInfo()

	RecordGPTAbuseSignal(c, info, GPTAbuseSignal{Matched: true, Kind: GPTAbuseKindCyberPolicy, Severity: GPTAbuseSeverityHigh, Source: GPTAbuseSourceHTTPError, StatusCode: http.StatusBadRequest, ErrorCode: "cyber_policy", UpstreamRequestId: "req-upstream-disabled", CountEligible: true})

	var count int64
	require.NoError(t, model.DB.Model(&model.GPTAbuseSignalLog{}).Where("user_id = ?", 7001).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, model.DB.Model(&model.GPTAbuseUserSuspension{}).Where("user_id = ?", 7001).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestEnforceGPTAbuseSuspensionRejectsGPTRequestOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseSignalServiceTest(t)
	seedGPTAbuseSubscription(t, 7001, 2)
	require.NoError(t, model.UpsertGPTAbuseSuspension(7001, 1, 2, 2, common.GetTimestamp()+3600))
	c := newGPTAbuseSignalTestContext("/v1/chat/completions")
	info := newGPTAbuseSignalTestRelayInfo()

	err := EnforceGPTAbuseSuspension(c, info)

	require.NotNil(t, err)
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.Equal(t, types.ErrorCodeGPTAbuseSuspended, err.GetErrorCode())
	openAIError := err.ToOpenAIError()
	assert.Equal(t, string(types.ErrorCodeGPTAbuseSuspended), openAIError.Type)
	assert.Equal(t, types.ErrorCodeGPTAbuseSuspended, openAIError.Code)
}

func TestEnforceGPTAbuseSuspensionAllowsNonGPTRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseSignalServiceTest(t)
	seedGPTAbuseSubscription(t, 7001, 2)
	require.NoError(t, model.UpsertGPTAbuseSuspension(7001, 1, 2, 2, common.GetTimestamp()+3600))
	c := newGPTAbuseSignalTestContext("/v1/messages")
	info := newGPTAbuseSignalTestRelayInfo()
	info.OriginModelName = "claude-3-5-sonnet"
	info.ChannelMeta.ChannelType = constant.ChannelTypeAnthropic
	info.ChannelMeta.UpstreamModelName = "claude-3-5-sonnet"

	assert.Nil(t, EnforceGPTAbuseSuspension(c, info))
}

func TestEnforceGPTAbuseSuspensionAllowsOpenAINonGPTContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseSignalServiceTest(t)
	seedGPTAbuseSubscription(t, 7001, 2)
	require.NoError(t, model.UpsertGPTAbuseSuspension(7001, 1, 2, 2, common.GetTimestamp()+3600))
	c := newGPTAbuseSignalTestContext("/v1/embeddings")
	info := &relaycommon.RelayInfo{UserId: 7001, OriginModelName: "text-embedding-3-large", RelayMode: relayconstant.RelayModeEmbeddings}
	assert.Nil(t, EnforceGPTAbuseSuspension(c, info))
}

func TestEnforceGPTAbuseSuspensionAllowsOpenAIImageModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseSignalServiceTest(t)
	seedGPTAbuseSubscription(t, 7001, 2)
	require.NoError(t, model.UpsertGPTAbuseSuspension(7001, 1, 2, 2, common.GetTimestamp()+3600))
	c := newGPTAbuseSignalTestContext("/v1/images/generations")
	info := &relaycommon.RelayInfo{UserId: 7001, OriginModelName: "gpt-image-1", RelayMode: relayconstant.RelayModeImagesGenerations}

	assert.Nil(t, EnforceGPTAbuseSuspension(c, info))
}

func TestEnforceGPTAbuseSuspensionRejectsUpstreamGPTAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseSignalServiceTest(t)
	seedGPTAbuseSubscription(t, 7001, 2)
	require.NoError(t, model.UpsertGPTAbuseSuspension(7001, 1, 2, 2, common.GetTimestamp()+3600))
	c := newGPTAbuseSignalTestContext("/v1/chat/completions")
	info := &relaycommon.RelayInfo{
		UserId:          7001,
		OriginModelName: "corp-gpt-alias",
		RelayMode:       relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeAnthropic,
			UpstreamModelName: "gpt-4o-mini",
		},
	}
	assert.NotNil(t, EnforceGPTAbuseSuspension(c, info))
}

func TestEnforceGPTAbuseSuspensionAllowsMappedNonGPTAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseSignalServiceTest(t)
	seedGPTAbuseSubscription(t, 7001, 2)
	require.NoError(t, model.UpsertGPTAbuseSuspension(7001, 1, 2, 2, common.GetTimestamp()+3600))
	c := newGPTAbuseSignalTestContext("/v1/chat/completions")
	info := &relaycommon.RelayInfo{
		UserId:          7001,
		OriginModelName: "gpt-corp-alias",
		RelayMode:       relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeAnthropic,
			UpstreamModelName: "claude-3-5-sonnet",
		},
	}

	assert.Nil(t, EnforceGPTAbuseSuspension(c, info))
}

type failingReadCloser struct {
	closed bool
}

func (f *failingReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (f *failingReadCloser) Close() error {
	f.closed = true
	return nil
}

func TestGPTAwareRelayErrorHandlerClosesBodyOnReadError(t *testing.T) {
	body := &failingReadCloser{}
	resp := &http.Response{StatusCode: http.StatusBadGateway, Body: body, Header: http.Header{}}

	err := GPTAwareRelayErrorHandler(nil, newGPTAbuseSignalTestRelayInfo(), resp, false)

	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.True(t, body.closed)
}
