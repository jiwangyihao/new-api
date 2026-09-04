package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChannelTestAlphaSearchUsesNativeCodexProtocol(t *testing.T) {
	oldRatio := ratio_setting.ModelRatio2JSONString()
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-alpha":1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
		common.RedisEnabled = oldRedis
	})

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "alpha_channel_test_user", Group: "default", Status: common.UserStatusEnabled, AffCode: "alpha_channel_test_aff"}).Error)
	service.InitHttpClient()
	receivedBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		receivedBody <- body
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/backend-api/codex/alpha/search", r.URL.Path)
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		assert.Equal(t, "account", r.Header.Get("chatgpt-account-id"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"encrypted_output":"opaque","output":[],"results":[{"url":"https://example.com"}]}`))
	}))
	t.Cleanup(server.Close)

	modelMapping := `{"gpt-alpha":"gpt-alpha-upstream"}`
	channel := &model.Channel{
		Id:           2411,
		Type:         constant.ChannelTypeCodex,
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-alpha",
		Group:        "default",
		Key:          `{"access_token":"token","account_id":"account"}`,
		Name:         "codex-alpha",
		BaseURL:      common.GetPointer(server.URL),
		ModelMapping: &modelMapping,
	}

	result := testChannel(channel, "gpt-alpha", string(constant.EndpointTypeOpenAIAlphaSearch), true)

	require.NotNil(t, result.context)
	require.NoError(t, result.localErr)
	require.Nil(t, result.newAPIError)
	assert.Equal(t, relayconstant.RelayModeAlphaSearch, result.context.GetInt("relay_mode"))
	body := <-receivedBody
	assert.Equal(t, "new-api-channel-test", gjson.GetBytes(body, "id").String())
	assert.Equal(t, "gpt-alpha-upstream", gjson.GetBytes(body, "model").String())
	assert.Equal(t, "OpenAI", gjson.GetBytes(body, "commands.search_query.0.q").String())
	assert.Equal(t, int64(16), gjson.GetBytes(body, "max_output_tokens").Int())
}

func TestChannelTestAlphaSearchSupportsOpenAIChannel(t *testing.T) {
	oldRatio := ratio_setting.ModelRatio2JSONString()
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-alpha-openai":1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
		common.RedisEnabled = oldRedis
	})

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "alpha_openai_channel_test_user", Group: "default", Status: common.UserStatusEnabled, AffCode: "alpha_openai_channel_test_aff"}).Error)
	service.InitHttpClient()
	receivedBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		receivedBody <- body
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/alpha/search", r.URL.Path)
		assert.Equal(t, "Bearer sk-alpha-openai", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"encrypted_output":"opaque","output":[],"results":[{"url":"https://example.com"}]}`))
	}))
	t.Cleanup(server.Close)

	modelMapping := `{"gpt-alpha-openai":"gpt-alpha-openai-upstream"}`
	channel := &model.Channel{
		Id:           2412,
		Type:         constant.ChannelTypeOpenAI,
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-alpha-openai",
		Group:        "default",
		Key:          "sk-alpha-openai",
		Name:         "openai-alpha",
		BaseURL:      common.GetPointer(server.URL),
		ModelMapping: &modelMapping,
	}

	result := testChannel(channel, "gpt-alpha-openai", string(constant.EndpointTypeOpenAIAlphaSearch), false)

	require.NotNil(t, result.context)
	require.NoError(t, result.localErr)
	require.Nil(t, result.newAPIError)
	assert.Equal(t, relayconstant.RelayModeAlphaSearch, result.context.GetInt("relay_mode"))
	body := <-receivedBody
	assert.Equal(t, "new-api-channel-test", gjson.GetBytes(body, "id").String())
	assert.Equal(t, "gpt-alpha-openai-upstream", gjson.GetBytes(body, "model").String())
	assert.Equal(t, "OpenAI", gjson.GetBytes(body, "commands.search_query.0.q").String())
	assert.Equal(t, int64(16), gjson.GetBytes(body, "max_output_tokens").Int())
}
