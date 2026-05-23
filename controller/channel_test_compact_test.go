package controller

import (
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
)

func TestNormalizeChannelTestEndpointCodexDoesNotRequireCompactSuffix(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeCodex}

	got := normalizeChannelTestEndpoint(channel, "gpt-5.5", string(constant.EndpointTypeOpenAIResponseCompact))

	assert.Equal(t, string(constant.EndpointTypeOpenAIResponseCompact), got)
}

func TestChannelTestCompactEndpointKeepsClientModel(t *testing.T) {
	oldRatio := ratio_setting.ModelRatio2JSONString()
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.5-openai-compact":1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
		common.RedisEnabled = oldRedis
	})
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "channel_test_user", Group: "default", Status: common.UserStatusEnabled, AffCode: "channel_test_aff"}).Error)
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/backend-api/codex/responses/compact", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response","created_at":1,"output":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(server.Close)
	channel := &model.Channel{
		Id:      2401,
		Type:    constant.ChannelTypeCodex,
		Status:  common.ChannelStatusEnabled,
		Models:  "gpt-5.5",
		Group:   "default",
		Key:     `{"access_token":"token","account_id":"account"}`,
		Name:    "codex",
		BaseURL: common.GetPointer(server.URL),
	}

	result := testChannel(channel, "gpt-5.5", string(constant.EndpointTypeOpenAIResponseCompact), false)

	require.NotNil(t, result.context)
	require.NoError(t, result.localErr)
	require.Nil(t, result.newAPIError)
	assert.Equal(t, "gpt-5.5", result.context.GetString("original_model"))
	assert.Equal(t, relayconstant.RelayModeResponsesCompact, result.context.GetInt("relay_mode"))
}
