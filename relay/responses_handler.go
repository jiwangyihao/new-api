package relay

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func releaseResponsesRequestResources(c *gin.Context, info *relaycommon.RelayInfo, httpResp *http.Response) {
	common.CleanupBodyStorage(c)
	if c != nil {
		c.Set(string(appconstant.ContextKeyOpenAIResponsesRequest), nil)
		c.Set(string(appconstant.ContextKeyOpenAIResponsesCompactionRequest), nil)
	}
	if info != nil {
		info.Request = nil
	}
	if httpResp == nil || httpResp.Request == nil {
		return
	}
	if httpResp.Request.Body != nil {
		_ = httpResp.Request.Body.Close()
		httpResp.Request.Body = nil
	}
	httpResp.Request.GetBody = nil
}

func tryBuildDirectDiskResponsesBody(c *gin.Context, info *relaycommon.RelayInfo, convertedRequest any) (relaycommon.ReplayableRequestBodyReader, bool, error) {
	if common.DebugEnabled || c == nil || info == nil {
		return nil, false, nil
	}
	storageValue, exists := c.Get(common.KeyBodyStorage)
	storage, ok := storageValue.(common.BodyStorage)
	if !exists || !ok || !storage.IsDisk() || !common.IsDiskCacheAvailable(storage.Size()) {
		return nil, false, nil
	}
	var request dto.OpenAIResponsesRequest
	switch value := convertedRequest.(type) {
	case dto.OpenAIResponsesRequest:
		request = value
	case *dto.OpenAIResponsesRequest:
		if value == nil {
			return nil, false, nil
		}
		request = *value
	default:
		return nil, false, nil
	}
	if !relaycommon.CanEncodeResponsesRequestDirectlyToDisk(request, info) {
		return nil, false, nil
	}
	body, err := relaycommon.NewDiskReleasableRequestBodyFromJSON(convertedRequest)
	if err != nil {
		return nil, false, nil
	}
	reader, err := body.Reader()
	if err != nil {
		body.Release()
		return nil, false, nil
	}
	if err := relaycommon.ApplyResponsesHeaderOnlyOverride(info); err != nil {
		_ = reader.Close()
		reader.Release()
		return nil, true, err
	}
	return reader, true, nil
}

func doResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo) (channel.Adaptor, *http.Response, *types.NewAPIError) {
	var responsesReq *dto.OpenAIResponsesRequest
	switch req := info.Request.(type) {
	case *dto.OpenAIResponsesRequest:
		responsesReq = req
	case *dto.OpenAIResponsesCompactionRequest:
		responsesReq = &dto.OpenAIResponsesRequest{
			Model:              req.Model,
			Input:              req.Input,
			Instructions:       req.Instructions,
			PreviousResponseID: req.PreviousResponseID,
		}
	default:
		return nil, nil, types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected dto.OpenAIResponsesRequest or dto.OpenAIResponsesCompactionRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	adaptor, requestBody, closer, apiErr := PrepareResponsesRequest(c, info, responsesReq)
	if apiErr != nil {
		return nil, nil, apiErr
	}
	defer closer.Close()

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	if resp == nil {
		return nil, nil, types.NewOpenAIError(fmt.Errorf("upstream response is nil"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return nil, nil, types.NewOpenAIError(fmt.Errorf("unexpected upstream response type %T", resp), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	return adaptor, httpResp, nil
}

func ResponsesHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		switch info.ApiType {
		case appconstant.APITypeOpenAI, appconstant.APITypeCodex:
		default:
			return types.NewErrorWithStatusCode(
				fmt.Errorf("unsupported endpoint %q for api type %d", "/v1/responses/compact", info.ApiType),
				types.ErrorCodeInvalidRequest,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	adaptor, httpResp, newAPIError := doResponsesRequest(c, info)
	if newAPIError != nil {
		return newAPIError
	}
	statusCodeMappingStr := c.GetString("status_code_mapping")
	if info.StreamGate == nil {
		releaseResponsesRequestResources(c, info, httpResp)
	} else {
		info.StreamRequestCleanup = func() { releaseResponsesRequestResources(c, info, httpResp) }
		defer func() {
			if !info.StreamGate.CanRetry() && info.StreamRequestCleanup != nil {
				cleanup := info.StreamRequestCleanup
				info.StreamRequestCleanup = nil
				cleanup()
			}
		}()
	}

	if httpResp.StatusCode != http.StatusOK {
		newAPIError = service.GPTAwareRelayErrorHandler(c, info, httpResp, false)
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}
	if streamErr := service.StreamAttemptError(info); streamErr != nil && info.StreamGate != nil && !info.StreamGate.Meaningful() {
		return streamErr
	}

	usageDto, _ := usage.(*dto.Usage)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		originModelName := info.OriginModelName
		originPriceData := info.PriceData

		_, err := helper.ModelPriceHelper(c, info, info.GetEstimatePromptTokens(), &types.TokenCountMeta{})
		if err != nil {
			info.OriginModelName = originModelName
			info.PriceData = originPriceData
			return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry(), types.ErrOptionWithStatusCode(http.StatusBadRequest))
		}
		if err := service.PostTextConsumeQuota(c, info, usageDto, nil); err != nil {
			info.OriginModelName = originModelName
			info.PriceData = originPriceData
			return service.PostSettleErrorToOpenAIError(info, err)
		}

		info.OriginModelName = originModelName
		info.PriceData = originPriceData
		return nil
	}

	if usageDto != nil && strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") && info.BillingSource != service.BillingSourceSubscription {
		service.PostAudioConsumeQuota(c, info, usageDto, "")
	} else {
		if err := service.PostTextConsumeQuota(c, info, usageDto, nil); err != nil {
			return service.PostSettleErrorToOpenAIError(info, err)
		}
	}
	return nil
}
