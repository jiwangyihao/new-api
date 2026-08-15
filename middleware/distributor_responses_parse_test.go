package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResponsesModelPreludeAndFullValidationReuseSameBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-5.5","input":"` + strings.Repeat("x", 32<<10) + `","stream":false}`
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(ctx) })

	modelRequest, err := getModelFromRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", modelRequest.Model)
	cachedRequest, ok := common.GetContextKeyType[*dto.OpenAIResponsesRequest](ctx, constant.ContextKeyOpenAIResponsesRequest)
	require.True(t, ok)
	require.NotNil(t, cachedRequest)
	storage, err := common.GetBodyStorage(ctx)
	require.NoError(t, err)
	bodyAfterPrelude, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, body, string(bodyAfterPrelude))

	request, err := helper.GetAndValidateResponsesRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", request.Model)
	require.NotNil(t, request.Stream)
	require.Same(t, cachedRequest, request)
	require.False(t, *request.Stream)
	bodyAfterValidation, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, body, string(bodyAfterValidation))
}

func TestResponsesCompactionModelPreludeAndValidationReuseSameRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-5.5","input":[{"role":"user","content":"` + strings.Repeat("x", 32<<10) + `"}]}`
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(ctx) })

	modelRequest, err := getModelFromRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", modelRequest.Model)
	cachedRequest, ok := common.GetContextKeyType[*dto.OpenAIResponsesCompactionRequest](ctx, constant.ContextKeyOpenAIResponsesCompactionRequest)
	require.True(t, ok)
	require.NotNil(t, cachedRequest)
	storage, err := common.GetBodyStorage(ctx)
	require.NoError(t, err)
	bodyAfterPrelude, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, body, string(bodyAfterPrelude))

	request, err := helper.GetAndValidateResponsesCompactionRequest(ctx)
	require.NoError(t, err)
	require.Same(t, cachedRequest, request)
	bodyAfterValidation, err := storage.Bytes()
	require.NoError(t, err)
	require.Equal(t, body, string(bodyAfterValidation))
}

func TestNonResponsesModelPreludeDoesNotCacheResponsesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}]}`
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(ctx) })

	modelRequest, err := getModelFromRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", modelRequest.Model)
	_, cached := common.GetContextKeyType[*dto.OpenAIResponsesRequest](ctx, constant.ContextKeyOpenAIResponsesRequest)
	require.False(t, cached)
}

func TestResponsesModelPreludePreservesMalformedFieldError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-5.5","input":"hello","stream":"not-a-bool"}`

	validationCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	validationCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	validationCtx.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(validationCtx) })
	_, validationErr := helper.GetAndValidateResponsesRequest(validationCtx)
	require.Error(t, validationErr)

	preludeCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	preludeCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	preludeCtx.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(preludeCtx) })
	modelRequest, preludeErr := getModelFromRequest(preludeCtx)
	require.NoError(t, preludeErr)
	require.Equal(t, "gpt-5.5", modelRequest.Model)
	_, helperErr := helper.GetAndValidateResponsesRequest(preludeCtx)
	require.Error(t, helperErr)
	require.Equal(t, validationErr.Error(), helperErr.Error())
	_, cached := common.GetContextKeyType[*dto.OpenAIResponsesRequest](preludeCtx, constant.ContextKeyOpenAIResponsesRequest)
	require.False(t, cached)
}
