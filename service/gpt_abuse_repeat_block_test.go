package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func setupGPTAbuseRepeatBlockServiceTest(t *testing.T) {
	t.Helper()
	setupGPTAbuseSignalServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.GPTAbuseRepeatBlockLog{}))
	_ = model.DB.Exec("DELETE FROM gpt_abuse_repeat_block_logs").Error
	oldEnabled := GPTAbuseRepeatBlockEnabled
	oldTTL := GPTAbuseRepeatBlockTTLSeconds
	oldRequireRedis := GPTAbuseRepeatBlockRequireRedis
	oldRedisEnabled := common.RedisEnabled
	GPTAbuseRepeatBlockEnabled = true
	GPTAbuseRepeatBlockTTLSeconds = 900
	GPTAbuseRepeatBlockRequireRedis = false
	common.RedisEnabled = false
	ResetGPTAbuseRepeatBlockCacheForTest()
	t.Cleanup(func() {
		GPTAbuseRepeatBlockEnabled = oldEnabled
		GPTAbuseRepeatBlockTTLSeconds = oldTTL
		GPTAbuseRepeatBlockRequireRedis = oldRequireRedis
		common.RedisEnabled = oldRedisEnabled
		ResetGPTAbuseRepeatBlockCacheForTest()
	})
}

func TestGPTAbuseRepeatBlockFeatureFlagDefaultsDisabled(t *testing.T) {
	t.Setenv(gptAbuseRepeatBlockEnabledEnv, "")
	assert.False(t, gptAbuseRepeatBlockEnabledFromEnv())
	t.Setenv(gptAbuseRepeatBlockEnabledEnv, "false")
	assert.False(t, gptAbuseRepeatBlockEnabledFromEnv())
	t.Setenv(gptAbuseRepeatBlockEnabledEnv, "true")
	assert.True(t, gptAbuseRepeatBlockEnabledFromEnv())
}

func TestGPTAbuseRepeatBlockDisabledShortCircuitsCaptureCheckAndStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseRepeatBlockServiceTest(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("not read"))
	closedStorage, err := common.CreateBodyStorage([]byte("not read"))
	require.NoError(t, err)
	require.NoError(t, closedStorage.Close())
	info := newGPTAbuseSignalTestRelayInfo()
	GPTAbuseRepeatBlockEnabled = false
	require.NoError(t, CaptureGPTAbuseRepeatBlockFingerprint(c, info, closedStorage))
	_, captured := GPTAbuseRepeatBlockContextFromGin(c)
	assert.False(t, captured)

	GPTAbuseRepeatBlockEnabled = true
	c, info = newGPTAbuseRepeatBlockContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"blocked"}]}`)
	GPTAbuseRepeatBlockEnabled = false
	StoreGPTAbuseRepeatBlock(c, info, &model.GPTAbuseSignalLog{Id: 100, CreatedAt: 1700000000, CountEligible: true})
	assert.Nil(t, CheckGPTAbuseRepeatBlock(c, info))
	GPTAbuseRepeatBlockEnabled = true
	assert.Nil(t, CheckGPTAbuseRepeatBlock(c, info), "disabled Store must not populate the repeat-block cache")
}

func newGPTAbuseRepeatBlockContext(t *testing.T, body string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, 7001)
	common.SetContextKey(c, constant.ContextKeyTokenId, 8001)
	c.Set("username", "abuse-user")
	c.Set("token_name", "safe-token-name")
	info := newGPTAbuseSignalTestRelayInfo()
	info.RequestId = "req-repeat-current"
	info.RequestURLPath = "/v1/chat/completions"
	info.RelayMode = relayconstant.RelayModeChatCompletions
	storage, err := common.CreateBodyStorage([]byte(body))
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })
	require.NoError(t, CaptureGPTAbuseRepeatBlockFingerprint(c, info, storage))
	return c, info
}

func TestGPTAbuseRepeatBlockFingerprintCanonicalizesJSONObjects(t *testing.T) {
	a := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"x"}],"stream":true}`)
	b := []byte(`{"stream":true,"input":[{"content":"x","role":"user"}],"model":"gpt-5.5"}`)
	fpA, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", a)
	require.NoError(t, err)
	fpB, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", b)
	require.NoError(t, err)
	assert.Equal(t, fpA.Value, fpB.Value)
}

func TestGPTAbuseRepeatBlockFingerprintCanonicalizesEquivalentJSONNumbers(t *testing.T) {
	a := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"x"}],"n":1,"m":1.2300}`)
	b := []byte(`{"m":1.23e0,"input":[{"content":"x","role":"user"}],"n":1.0,"model":"gpt-5.5"}`)
	fpA, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", a)
	require.NoError(t, err)
	fpB, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", b)
	require.NoError(t, err)
	assert.Equal(t, fpA.Value, fpB.Value)
}

func TestCanonicalGPTAbuseRepeatBlockJSONPreservesFingerprintSemantics(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		canonical string
		valid     bool
	}{
		{name: "object order", input: `{"b":2,"a":1}`, canonical: `{"a":1,"b":2}`, valid: true},
		{name: "nested values", input: ` { "z":[{"b":true,"a":null},3.0], "a":"x" } `, canonical: `{"a":"x","z":[{"a":null,"b":true},3]}`, valid: true},
		{name: "equivalent numbers", input: `{"d":-0.0,"c":1e3,"b":1.2300,"a":1.0}`, canonical: `{"a":1,"b":1.23,"c":1000,"d":0}`, valid: true},
		{name: "large precise numbers", input: `{"n":9007199254740993,"d":0.123456789012345678901234567890}`, canonical: `{"d":0.12345678901234567890123456789,"n":9007199254740993}`, valid: true},
		{name: "large exponents", input: `{"small":1e-1025,"large":1e1025}`, canonical: `{"large":1e1025,"small":1e-1025}`, valid: true},
		{name: "duplicate key last wins", input: `{"a":1,"a":2}`, canonical: `{"a":2}`, valid: true},
		{name: "escaped duplicate key last wins", input: `{"a":1,"\u0061":2}`, canonical: `{"a":2}`, valid: true},
		{name: "escaped strings", input: `{"s":"<tag>&\u2028\\\""}`, canonical: `{"s":"\u003ctag\u003e\u0026\u2028\\\""}`, valid: true},
		{name: "root array", input: `[3.0,{"b":2,"a":1},false]`, canonical: `[3,{"a":1,"b":2},false]`, valid: true},
		{name: "root scalar", input: `1.2300`, canonical: `1.23`, valid: true},
		{name: "invalid truncated", input: `{"a":`, valid: false},
		{name: "invalid trailing value", input: `{"a":1} {"b":2}`, valid: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			canonical, ok := canonicalGPTAbuseRepeatBlockJSON([]byte(testCase.input))
			require.Equal(t, testCase.valid, ok)
			if !testCase.valid {
				require.Nil(t, canonical)
				return
			}
			require.Equal(t, testCase.canonical, string(canonical))
		})
	}
}

func TestCaptureGPTAbuseRepeatBlockSkipsNonGPTTargetWithoutReadingBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseRepeatBlockServiceTest(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader("not read"))
	storage, err := common.CreateBodyStorage([]byte("not read"))
	require.NoError(t, err)
	require.NoError(t, storage.Close())
	info := newGPTAbuseSignalTestRelayInfo()
	info.RequestURLPath = "/v1/images/edits"
	info.RelayMode = relayconstant.RelayModeImagesEdits
	info.OriginModelName = "gpt-image-1"

	require.NoError(t, CaptureGPTAbuseRepeatBlockFingerprint(c, info, storage))
	_, ok := GPTAbuseRepeatBlockContextFromGin(c)
	assert.False(t, ok)
}

func TestGPTAbuseRepeatBlockFingerprintChangesWhenPromptChanges(t *testing.T) {
	a := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"x"}],"stream":true}`)
	b := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"y"}],"stream":true}`)
	fpA, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", a)
	require.NoError(t, err)
	fpB, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", b)
	require.NoError(t, err)
	assert.NotEqual(t, fpA.Value, fpB.Value)
}

func TestGPTAbuseRepeatBlockFingerprintPreservesJSONNumbers(t *testing.T) {
	a := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"x"}],"n":9007199254740992}`)
	b := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"x"}],"n":9007199254740993}`)
	fpA, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", a)
	require.NoError(t, err)
	fpB, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", b)
	require.NoError(t, err)
	assert.NotEqual(t, fpA.Value, fpB.Value)

	decimalA := []byte(`{"model":"gpt-5.5","temperature":0.123456789012345678901234567890,"input":[{"role":"user","content":"x"}]}`)
	decimalB := []byte(`{"model":"gpt-5.5","temperature":0.123456789012345678901234567891,"input":[{"role":"user","content":"x"}]}`)
	fpDecimalA, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", decimalA)
	require.NoError(t, err)
	fpDecimalB, err := BuildGPTAbuseRepeatBlockFingerprint(7001, 8001, "/v1/responses", 0, "gpt-5.5", "application/json", decimalB)
	require.NoError(t, err)
	assert.NotEqual(t, fpDecimalA.Value, fpDecimalB.Value)
}

func TestCheckGPTAbuseRepeatBlockReturnsLocalOpenAIError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseRepeatBlockServiceTest(t)
	c, info := newGPTAbuseRepeatBlockContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"blocked"}]}`)
	StoreGPTAbuseRepeatBlock(c, info, &model.GPTAbuseSignalLog{Id: 101, CreatedAt: 1700000000, UserId: 7001, TokenId: 8001, RequestId: "req-first-warning", UpstreamRequestId: "req-upstream", Source: GPTAbuseSourceHTTPError, Kind: GPTAbuseKindCyberPolicy, Severity: GPTAbuseSeverityHigh, CountEligible: true})

	apiErr := CheckGPTAbuseRepeatBlock(c, info)

	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
	openAIError := apiErr.ToOpenAIError()
	assert.Equal(t, "invalid_request_error", openAIError.Type)
	assert.Equal(t, string(types.ErrorCodeGPTAbuseRepeatedWarningRequest), openAIError.Code)
	assert.Contains(t, openAIError.Message, "blocked locally")
	assert.Contains(t, openAIError.Message, "not sent upstream again")
}

func TestCheckGPTAbuseRepeatBlockWritesRepeatBlockLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseRepeatBlockServiceTest(t)
	c, info := newGPTAbuseRepeatBlockContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"blocked"}]}`)
	StoreGPTAbuseRepeatBlock(c, info, &model.GPTAbuseSignalLog{Id: 102, CreatedAt: 1700000001, UserId: 7001, TokenId: 8001, RequestId: "req-first-warning", UpstreamRequestId: "req-upstream", Source: GPTAbuseSourceSSEResponseFailed, Kind: GPTAbuseKindCyberPolicy, Severity: GPTAbuseSeverityHigh, CountEligible: true, ChannelId: 9001, ChannelName: "OpenAI Primary", ChannelType: constant.ChannelTypeOpenAI})

	err := CheckGPTAbuseRepeatBlock(c, info)

	require.NotNil(t, err)
	var log model.GPTAbuseRepeatBlockLog
	require.NoError(t, model.DB.First(&log).Error)
	assert.Equal(t, 7001, log.UserId)
	assert.Equal(t, 8001, log.TokenId)
	assert.Equal(t, 102, log.FirstWarningLogId)
	assert.Equal(t, GPTAbuseKindCyberPolicy, log.FirstWarningKind)
	assert.Equal(t, GPTAbuseSeverityHigh, log.FirstWarningSeverity)
	assert.Equal(t, 9001, log.ChannelId)
	assert.Equal(t, "OpenAI Primary", log.ChannelName)
	assert.Equal(t, constant.ChannelTypeOpenAI, log.ChannelType)
	dayStart, dayEnd := model.GPTAbuseDayWindow(common.GetTimestamp())
	count, errCount := model.CountGPTAbuseSignalsForUserRaw(7001, dayStart, dayEnd)
	require.NoError(t, errCount)
	assert.Equal(t, 0, count)
}

func TestGPTAbuseRepeatBlockRedisMissChecksMemoryFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseRepeatBlockServiceTest(t)
	c, info := newGPTAbuseRepeatBlockContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"blocked"}]}`)
	common.RedisEnabled = false
	StoreGPTAbuseRepeatBlock(c, info, &model.GPTAbuseSignalLog{Id: 103, CreatedAt: 1700000002, UserId: 7001, TokenId: 8001, RequestId: "req-first-warning", UpstreamRequestId: "req-upstream", Source: GPTAbuseSourceHTTPError, Kind: GPTAbuseKindCyberPolicy, Severity: GPTAbuseSeverityHigh, CountEligible: true})
	common.RedisEnabled = true

	apiErr := CheckGPTAbuseRepeatBlock(c, info)

	require.NotNil(t, apiErr)
	assert.Equal(t, string(types.ErrorCodeGPTAbuseRepeatedWarningRequest), apiErr.ToOpenAIError().Code)
}

func TestGPTAbuseRepeatBlockMemoryCacheExpires(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseRepeatBlockServiceTest(t)
	oldTTL := GPTAbuseRepeatBlockTTLSeconds
	GPTAbuseRepeatBlockTTLSeconds = 1
	t.Cleanup(func() { GPTAbuseRepeatBlockTTLSeconds = oldTTL })
	c, info := newGPTAbuseRepeatBlockContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"blocked"}]}`)
	StoreGPTAbuseRepeatBlock(c, info, &model.GPTAbuseSignalLog{Id: 104, CreatedAt: 1700000003, UserId: 7001, TokenId: 8001, RequestId: "req-first-warning", UpstreamRequestId: "req-upstream", Source: GPTAbuseSourceHTTPError, Kind: GPTAbuseKindCyberPolicy, Severity: GPTAbuseSeverityHigh, CountEligible: true})

	require.NotNil(t, CheckGPTAbuseRepeatBlock(c, info))
	time.Sleep(1100 * time.Millisecond)
	assert.Nil(t, CheckGPTAbuseRepeatBlock(c, info))
}
