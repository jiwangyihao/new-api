package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldCopyUpstreamHeaderFiltersCodexProServedAck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}

	for _, name := range []string{
		"X-NewAPI-Pro-Served",
		"x-newapi-pro-served",
		"X-NEWAPI-PRO-SERVED",
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, ShouldCopyUpstreamHeader(ctx, name, []string{"codex-pro"}))
		})
	}
}

func TestIOCopyBytesGracefullyDoesNotExposeCodexProServedAck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"X-NewAPI-Pro-Served": []string{"codex-pro"},
			"X-Upstream-Trace":    []string{"trace-1"},
		},
	}

	IOCopyBytesGracefully(ctx, resp, []byte(`{"ok":true}`))

	require.Empty(t, recorder.Header().Get("X-NewAPI-Pro-Served"))
	require.Equal(t, "trace-1", recorder.Header().Get("X-Upstream-Trace"))
}
