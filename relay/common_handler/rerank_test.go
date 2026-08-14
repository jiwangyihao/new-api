package common_handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXinferenceRerankDoesNotMarkLocalUsageWhenCountingDisabled(t *testing.T) {
	oldCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() { constant.CountToken = oldCountToken })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"results":[{"index":0,"relevance_score":0.5}]}`)),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta:  &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeXinference},
		RerankerInfo: &relaycommon.RerankerInfo{},
	}

	usage, apiErr := RerankHandler(ctx, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Zero(t, usage.TotalTokens)
	assert.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
}
