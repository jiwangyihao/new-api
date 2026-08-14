package openai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chunkThenErrorReadCloser struct {
	resp *http.Response
	sent bool
}

func (r *chunkThenErrorReadCloser) Read(p []byte) (int, error) {
	if r.sent {
		r.resp.Trailer = http.Header{
			relaycommon.DynamicBillingMultiplierHeaderName:       []string{"1.75"},
			relaycommon.DynamicBillingMultiplierSourceHeaderName: []string{"trailer_priority"},
		}
		return 0, errors.New("upstream read failed")
	}
	r.sent = true
	return copy(p, "audio-chunk"), nil
}

func (r *chunkThenErrorReadCloser) Close() error { return nil }

func TestOpenaiTTSHandlerStreamsBodyWhenLocalTokenCountingDisabled(t *testing.T) {
	oldCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() { constant.CountToken = oldCountToken })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
	}
	resp.Body = &chunkThenErrorReadCloser{resp: resp}
	info := &relaycommon.RelayInfo{DynamicBillingMultiplierEnabled: true}

	usage := OpenaiTTSHandler(ctx, resp, info)

	require.NotNil(t, usage)
	assert.Zero(t, usage.TotalTokens)
	assert.Equal(t, "audio-chunk", recorder.Body.String())
	assert.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
	assert.InDelta(t, 1.75, info.FrozenDynamicBillingMultiplier(), 1e-9)
	assert.Equal(t, "trailer_priority", info.FrozenDynamicBillingMultiplierSource())
}
