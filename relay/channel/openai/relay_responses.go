package openai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

func markCodexProServedCandidateFromResponseTrailer(info *relaycommon.RelayInfo, resp *http.Response) {
	if info == nil || resp == nil {
		return
	}
	info.MarkCodexProServedCandidateFromTrailers(resp.Trailer)
}

func openAIResponseStatusCompleted(status []byte) bool {
	return strings.EqualFold(strings.TrimSpace(common.JsonRawMessageToString(status)), "completed")
}

func openAIResponsesCompletedWithUsage(resp *dto.OpenAIResponsesResponse) bool {
	return resp != nil && resp.Usage != nil && openAIResponseStatusCompleted(resp.Status)
}

// responsesSoftErrorMinOutputTokens 是软错误结束时仍然计费所需的最小输出 token 数。
// 上游建立连接后以软错误（response.error/failed/incomplete/cancelled 等）结束，
// 且观察到的输出 token 低于该阈值时，视为未产生有效输出，不进行计费。
const responsesSoftErrorMinOutputTokens = 20

type responsesStreamEventHeader struct {
	Type string `json:"type"`
}

func requiresFullResponsesStreamDecode(eventType string) bool {
	switch eventType {
	case "response.completed",
		"response.error",
		"response.failed",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
		dto.ResponsesOutputTypeItemDone:
		return true
	default:
		return false
	}
}

func parseResponsesStreamEvent(data string) (dto.ResponsesStreamResponse, error) {
	return parseResponsesStreamEventBytes(common.StringToByteSlice(data))
}

func parseResponsesStreamEventBytes(data []byte) (dto.ResponsesStreamResponse, error) {
	if !gjson.ValidBytes(data) {
		return dto.ResponsesStreamResponse{}, fmt.Errorf("invalid responses stream JSON")
	}

	var eventType string
	// GJSON returns the first duplicate key, whereas encoding/json uses the last.
	// Escaped keys also require the standard decoder to preserve that contract.
	if bytes.Count(data, []byte(`"type"`)) > 1 || bytes.Contains(data, []byte(`\u`)) {
		var header responsesStreamEventHeader
		if err := common.Unmarshal(data, &header); err != nil {
			return dto.ResponsesStreamResponse{}, err
		}
		eventType = header.Type
	} else {
		typeResult := gjson.GetBytes(data, "type")
		if typeResult.Type != gjson.Null && typeResult.Type != gjson.String {
			return dto.ResponsesStreamResponse{}, fmt.Errorf("responses stream type must be a string")
		}
		eventType = typeResult.String()
	}
	if !requiresFullResponsesStreamDecode(eventType) {
		return dto.ResponsesStreamResponse{Type: eventType}, nil
	}

	var event dto.ResponsesStreamResponse
	if err := common.Unmarshal(data, &event); err != nil {
		return dto.ResponsesStreamResponse{}, err
	}
	return event, nil
}

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if upID := service.GPTUpstreamRequestID(resp.Header); upID != "" {
		c.Set(common.UpstreamRequestIdKey, upID)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		if service.ShouldMonitorGPTAbuse(info) {
			signal := service.ClassifyGPTAbuseSignalFromHTTPError(resp.StatusCode, responseBody)
			signal.UpstreamRequestId = c.GetString(common.UpstreamRequestIdKey)
			if info != nil {
				signal.RequestedModel = info.OriginModelName
				signal.UpstreamModel = info.UpstreamModelName
			}
			service.RecordGPTAbuseSignal(c, info, signal)
		}
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.InputTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.OutputTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
		}
	}
	applyDynamicBillingMultiplierFromHTTPResponse(info, resp, responseBody, relaycommon.DynamicBillingMultiplierSourceBody)
	if openAIResponsesCompletedWithUsage(&responsesResponse) && info != nil {
		markCodexProServedCandidateFromResponseTrailer(info, resp)
		info.ConfirmCodexProServed()
		if billing := service.NewAPIBillingFromUsage(info, &usage); billing != nil {
			responsesResponse.NewAPIBilling = billing
			service.SeedNewAPIBillingRelayInfo(info, *billing)
			if responseBody, err = common.Marshal(responsesResponse); err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			}
		}
	}

	// 写入新的 response body
	writeOK := service.IOCopyBytesGracefully(c, resp, responseBody)

	if writeOK && responsesResponse.NewAPIBilling != nil && info != nil {
		info.CodexProServed = responsesResponse.NewAPIBilling.CodexProServed
	}
	if !writeOK && info != nil {
		info.ClearCodexProServedCandidate()
		info.CodexProServed = false
	}
	relayBillingMetadataInjected := responsesResponse.NewAPIBilling != nil

	if !relayBillingMetadataInjected && writeOK && openAIResponsesCompletedWithUsage(&responsesResponse) && info != nil {
		markCodexProServedCandidateFromResponseTrailer(info, resp)
		info.ConfirmCodexProServed()
	}
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	completedWithUsage := false
	var completedStreamResponse *dto.ResponsesStreamResponse
	var completedStreamData string
	if upID := service.GPTUpstreamRequestID(resp.Header); upID != "" {
		c.Set(common.UpstreamRequestIdKey, upID)
	}
	if info != nil {
		info.ApplyDynamicBillingMultiplierFromHeaders(resp.Header, relaycommon.DynamicBillingMultiplierSourceHeader)
	}
	doneBuffer := beginResponsesDoneBuffering(c)

	helper.StreamScannerBytesHandler(c, resp, info, func(data []byte, sr *helper.StreamResult) {
		streamResponse, err := parseResponsesStreamEventBytes(data)
		if err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		if service.ShouldMonitorGPTAbuse(info) {
			signal := service.ClassifyGPTAbuseSignalFromSSEEventBytes(streamResponse.Type, data)
			if signal.Matched {
				signal.StatusCode = resp.StatusCode
				signal.UpstreamRequestId = c.GetString(common.UpstreamRequestIdKey)
				if info != nil {
					signal.RequestedModel = info.OriginModelName
					signal.UpstreamModel = info.UpstreamModelName
				}
				service.RecordGPTAbuseSignal(c, info, signal)
			}
		}

		shouldDelayCompleted := streamResponse.Type == "response.completed" && openAIResponsesCompletedWithUsage(streamResponse.Response)
		if shouldDelayCompleted {
			completedCopy := streamResponse
			completedStreamResponse = &completedCopy
			completedStreamData = string(data)
		} else if err := sendResponsesStreamBytes(c, streamResponse, data); err != nil {
			sr.Stop(err)
			return
		}
		if streamResponse.Type == "response.error" || streamResponse.Type == "response.failed" || streamResponse.Type == "response.incomplete" || streamResponse.Type == "response.cancelled" || streamResponse.Type == "response.canceled" {
			streamErr := fmt.Errorf("responses stream terminal error: %s", streamResponse.Type)
			if streamResponse.Response != nil {
				if oaiErr := streamResponse.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = fmt.Errorf("responses stream terminal error: %s: %s", streamResponse.Type, oaiErr.Message)
				}
			}
			sr.Error(streamErr)
			return
		}
		switch streamResponse.Type {
		case "response.completed":
			if info != nil {
				info.ApplyDynamicBillingMultiplierFromBody(data, relaycommon.DynamicBillingMultiplierSourceSSE)
			}
			completedWithUsage = openAIResponsesCompletedWithUsage(streamResponse.Response)
			if streamResponse.Response != nil {
				if streamResponse.Response.Usage != nil {
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
						usage.InputTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
						usage.OutputTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
					}
				}
				if streamResponse.Response.HasImageGenerationCall() {
					c.Set("image_generation_call", true)
					c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
					c.Set("image_generation_call_size", streamResponse.Response.GetSize())
				}
			}
		case dto.ResponsesOutputTypeItemDone:
			if streamResponse.Item != nil && streamResponse.Item.Type == dto.BuildInCallWebSearchCall {
				if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
					if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
						webSearchTool.CallCount++
					}
				}
			}
		}
	})
	if info != nil {
		info.ApplyDynamicBillingMultiplierFromHeaders(resp.Trailer, relaycommon.DynamicBillingMultiplierSourceTrailer)
	}
	if doneBuffer != nil {
		c.Writer = doneBuffer.ResponseWriter
	}
	if completedWithUsage && info != nil && info.StreamStatus != nil && info.StreamStatus.Completed && info.StreamStatus.DrainedToEOF && !info.StreamStatus.HasErrors() {
		markCodexProServedCandidateFromResponseTrailer(info, resp)
		info.ConfirmCodexProServed()
	}
	if completedStreamResponse != nil {
		finalWriteOK := true
		if billing := service.NewAPIBillingFromUsage(info, usage); billing != nil {
			completedStreamResponse.NewAPIBilling = billing
			if info != nil {
				service.SeedNewAPIBillingRelayInfo(info, *billing)
			}
			if data, err := common.Marshal(completedStreamResponse); err == nil {
				completedStreamData = string(data)
			} else {
				logger.LogError(c, "failed to marshal responses completed stream billing metadata: "+err.Error())
			}
		}
		if err := sendResponsesStreamData(c, *completedStreamResponse, completedStreamData); err != nil {
			finalWriteOK = false
			if info != nil && info.StreamStatus != nil {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
			}
		}
		if finalWriteOK && doneBuffer != nil {
			if err := doneBuffer.flushBufferedDone(); err != nil {
				finalWriteOK = false
				if info != nil && info.StreamStatus != nil {
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
				}
			}
		}
		if !finalWriteOK && info != nil {
			info.ClearCodexProServedCandidate()
			info.CodexProServed = false
		}
	} else if doneBuffer != nil {
		if err := doneBuffer.flushBufferedDone(); err != nil && info != nil && info.StreamStatus != nil {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
		}
	}

	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	// 软错误结束且未观察到有效输出（输出 token < responsesSoftErrorMinOutputTokens）时不计费：
	// 返回 nil usage，使结算层将其视为无可信 usage，fixed_request 退预扣、usage-token 扣 0，且不注入 NewAPIBilling。
	if info != nil && info.StreamStatus != nil && info.StreamStatus.HasErrors() && usage.OutputTokens < responsesSoftErrorMinOutputTokens {
		return nil, nil
	}

	return usage, nil
}
