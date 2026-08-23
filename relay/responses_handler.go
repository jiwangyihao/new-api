package relay

import (
	"fmt"
	"io"
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
	"github.com/QuantumNous/new-api/setting/model_setting"
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

	request := responsesReq.CloneForRelay()
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, nil, types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader
	var replayBody relaycommon.ReplayableRequestBodyReader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.ReaderOnly(storage)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
		if err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		directBody, direct, err := tryBuildDirectDiskResponsesBody(c, info, convertedRequest)
		if err != nil {
			return nil, nil, newAPIErrorFromParamOverride(err)
		}
		if direct {
			replayBody = directBody
			requestBody = directBody
		} else {
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
			if err != nil {
				return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if err != nil {
					return nil, nil, newAPIErrorFromParamOverride(err)
				}
			}
			if common.DebugEnabled {
				println("requestBody: ", string(jsonData))
			}
			replayBody = relaycommon.NewAdaptiveReplayableRequestBody(jsonData)
			requestBody = replayBody
			jsonData = nil
		}
	}
	if replayBody != nil {
		defer func() {
			_ = replayBody.Close()
			replayBody.Release()
		}()
	}

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
	releaseResponsesRequestResources(c, info, httpResp)

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
