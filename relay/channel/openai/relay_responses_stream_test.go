package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flushableRecorder struct {
	*httptest.ResponseRecorder
}

func (r flushableRecorder) Flush() {
	r.ResponseRecorder.Flush()
}

func TestOaiResponsesStreamHandler_ResponseCompletedEOFMarksNormalEnd(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5},"output":[]}}`,
		"data: [DONE]",
		"",
	}, "\n")
	recorder := flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{
		ChannelMeta:  &relaycommon.ChannelMeta{},
		StreamStatus: relaycommon.NewStreamStatus(),
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 5, usage.TotalTokens)
	require.NotNil(t, info.StreamStatus)
	assert.Eventually(t, func() bool {
		return info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.IsNormalEnd())
	assert.False(t, info.StreamStatus.HasErrors())
}
