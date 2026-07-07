package helper

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImageRequestRejectsJSONNAboveMax(t *testing.T) {
	ctx := newOpenAIImageJSONContext(t, fmt.Sprintf(`{"model":"gpt-image-1","prompt":"draw","n":%d}`, dto.MaxImageN+1))

	request, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesGenerations)

	require.Error(t, err)
	require.Nil(t, request)
}

func TestOpenAIImageRequestDefaultsMissingJSONNToOne(t *testing.T) {
	ctx := newOpenAIImageJSONContext(t, `{"model":"gpt-image-1","prompt":"draw"}`)

	request, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesGenerations)

	require.NoError(t, err)
	require.NotNil(t, request.N)
	require.Equal(t, uint(1), *request.N)
}

func TestOpenAIImageRequestAcceptsJSONNAtMax(t *testing.T) {
	ctx := newOpenAIImageJSONContext(t, fmt.Sprintf(`{"model":"gpt-image-1","prompt":"draw","n":%d}`, dto.MaxImageN))

	request, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesGenerations)

	require.NoError(t, err)
	require.NotNil(t, request.N)
	require.Equal(t, uint(dto.MaxImageN), *request.N)
}

func TestOpenAIImageEditRejectsNegativeMultipartN(t *testing.T) {
	ctx := newOpenAIImageMultipartContext(t, map[string]string{
		"model":  "gpt-image-1",
		"prompt": "edit",
		"image":  "data:image/png;base64,AAAA",
		"n":      "-1",
	})

	request, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesEdits)

	require.Error(t, err)
	require.Nil(t, request)
}

func TestOpenAIImageEditAcceptsMultipartNAtMax(t *testing.T) {
	ctx := newOpenAIImageMultipartContext(t, map[string]string{
		"model":  "gpt-image-1",
		"prompt": "edit",
		"image":  "data:image/png;base64,AAAA",
		"n":      fmt.Sprintf("%d", dto.MaxImageN),
	})

	request, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesEdits)

	require.NoError(t, err)
	require.NotNil(t, request.N)
	require.Equal(t, uint(dto.MaxImageN), *request.N)
}

func newOpenAIImageJSONContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}

func newOpenAIImageMultipartContext(t *testing.T, fields map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return ctx
}
