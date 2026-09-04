package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

func SupportsAlphaSearchAPIType(apiType int) bool {
	return apiType == constant.APITypeOpenAI || apiType == constant.APITypeCodex
}

func AlphaSearchHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	info.InitChannelMeta(c)
	if !SupportsAlphaSearchAPIType(info.ApiType) {
		return types.NewError(errors.New("channel does not support /v1/alpha/search"), types.ErrorCodeInvalidRequest)
	}

	request, ok := info.Request.(*dto.AlphaSearchRequest)
	if !ok {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected *dto.AlphaSearchRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	jsonData, err := BuildAlphaSearchRequestBody(request.RawBody, info.OriginModelName, info.UpstreamModelName)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return newAPIErrorFromParamOverride(err)
		}
	}

	requestBody := relaycommon.NewAdaptiveReplayableRequestBody(jsonData)
	defer func() {
		_ = requestBody.Close()
		requestBody.Release()
	}()

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewOpenAIError(errors.New("invalid http response"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		apiErr := service.GPTAwareRelayErrorHandler(c, info, httpResp, false)
		service.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}

	info.ApplyDynamicBillingMultiplierFromHeaders(httpResp.Header, relaycommon.DynamicBillingMultiplierSourceHeader)
	copyAlphaSearchResponseHeaders(c, httpResp.Header)
	c.Writer.WriteHeader(httpResp.StatusCode)
	if _, err := io.Copy(c.Writer, httpResp.Body); err != nil {
		return types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
	}
	info.ApplyDynamicBillingMultiplierFromHeaders(httpResp.Trailer, relaycommon.DynamicBillingMultiplierSourceTrailer)

	if err := service.PostTextConsumeQuota(c, info, &dto.Usage{}, nil); err != nil {
		return service.PostSettleErrorToOpenAIError(info, err)
	}
	return nil
}

func copyAlphaSearchResponseHeaders(c *gin.Context, headers http.Header) {
	for key, values := range headers {
		if !service.ShouldCopyUpstreamHeader(c, key, values) {
			continue
		}
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
}

// BuildAlphaSearchRequestBody preserves the original request unless model mapping requires a top-level model rewrite.
func BuildAlphaSearchRequestBody(rawBody []byte, originModel, upstreamModel string) ([]byte, error) {
	if len(rawBody) == 0 {
		return nil, errors.New("empty alpha search request body")
	}
	if upstreamModel == "" || upstreamModel == originModel {
		return rawBody, nil
	}
	return sjson.SetBytes(rawBody, "model", upstreamModel)
}
