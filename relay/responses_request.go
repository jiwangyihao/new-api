package relay

import (
	"bytes"
	"fmt"
	"io"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

type responsesRequestBody struct {
	relaycommon.ReplayableRequestBodyReader
}

func (b responsesRequestBody) Close() error {
	err := b.ReplayableRequestBodyReader.Close()
	b.Release()
	return err
}

// PrepareResponsesRequest shares mapping, overrides and continuation identity
// between HTTP and native WebSocket without mutating the replayable client body.
func PrepareResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, responsesReq *dto.OpenAIResponsesRequest) (channel.Adaptor, io.Reader, io.Closer, *types.NewAPIError) {
	request := responsesReq.CloneForRelay()
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return nil, nil, nil, types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	var binding *service.ResponseBinding
	if value, found := c.Get("response_route_binding"); found {
		entry := value.(service.ResponseBinding)
		binding = &entry
		request.PreviousResponseID = entry.UpstreamID
	}
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, nil, nil, types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, nil, nil, types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
		}
		var body io.Reader = common.ReaderOnly(storage)
		if binding != nil {
			raw, err := storage.Bytes()
			if err != nil {
				return nil, nil, nil, types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
			}
			patched, err := sjson.SetBytes(raw, "previous_response_id", binding.UpstreamID)
			if err != nil {
				return nil, nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			body = bytes.NewReader(patched)
		}
		return adaptor, body, io.NopCloser(body), nil
	}
	convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
	if err != nil {
		return nil, nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
	// Continuations must be patched after overrides, so their route cannot be
	// replaced by a channel template carrying a different response reference.
	if binding == nil {
		directBody, direct, err := tryBuildDirectDiskResponsesBody(c, info, convertedRequest)
		if err != nil {
			return nil, nil, nil, newAPIErrorFromParamOverride(err)
		}
		if direct {
			body := responsesRequestBody{directBody}
			return adaptor, directBody, body, nil
		}
	}
	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return nil, nil, nil, newAPIErrorFromParamOverride(err)
		}
	}
	if binding != nil {
		jsonData, err = sjson.SetBytes(jsonData, "previous_response_id", binding.UpstreamID)
		if err != nil {
			return nil, nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
	}
	if common.DebugEnabled {
		println("requestBody: ", string(jsonData))
	}
	body := relaycommon.NewAdaptiveReplayableRequestBody(jsonData)
	return adaptor, body, responsesRequestBody{body}, nil
}
