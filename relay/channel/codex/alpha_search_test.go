package codex

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLAlphaSearch(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAlphaSearch,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeCodex,
			ChannelBaseUrl: "https://chatgpt.com",
		},
	}

	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://chatgpt.com/backend-api/codex/alpha/search", url)
}

func TestSetupRequestHeaderAlphaSearchUsesCodexOAuthHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", nil)
	headers := make(http.Header)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAlphaSearch,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeCodex,
			ApiType:     constant.APITypeCodex,
			ApiKey:      `{"access_token":"access-token","account_id":"account-id"}`,
		},
	}

	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, info)
	require.NoError(t, err)
	assert.Equal(t, "Bearer access-token", headers.Get("Authorization"))
	assert.Equal(t, "account-id", headers.Get("chatgpt-account-id"))
	assert.Equal(t, "responses=experimental", headers.Get("OpenAI-Beta"))
	assert.Equal(t, "codex_cli_rs", headers.Get("originator"))
	assert.Equal(t, "application/json", headers.Get("Content-Type"))
	assert.Equal(t, "application/json", headers.Get("Accept"))
}
