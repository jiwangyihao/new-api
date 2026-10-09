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
	"github.com/QuantumNous/new-api/pkg/streamgate"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

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

// Legacy ungated settlement threshold only; replay always uses delivered payload.
const responsesSoftErrorMinOutputTokens = 20

type responsesStreamEventHeader struct {
	Type string `json:"type"`
}

func requiresFullResponsesStreamDecode(eventType string) bool {
	switch eventType {
	case "response.completed",
		"error",
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
	var protocolErr *types.NewAPIError
	oaiError := responsesResponse.GetOpenAIError()
	gated := info != nil && info.StreamGate != nil
	failed := strings.EqualFold(strings.TrimSpace(common.JsonRawMessageToString(responsesResponse.Status)), "failed") || oaiError != nil
	if oaiError != nil && oaiError.Type != "" || gated && failed {
		if service.ShouldMonitorGPTAbuse(info) {
			signal := service.ClassifyGPTAbuseSignalFromHTTPError(resp.StatusCode, responseBody)
			signal.UpstreamRequestId = c.GetString(common.UpstreamRequestIdKey)
			if info != nil {
				signal.RequestedModel = info.OriginModelName
				signal.UpstreamModel = info.UpstreamModelName
			}
			service.RecordGPTAbuseSignal(c, info, signal)
		}
		if !gated {
			return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
		}
		protocolErr = streamgate.ResponsesFailure(responseBody)
		if !streamgate.ResponsesMeaningfulOutput(responseBody) {
			return nil, protocolErr
		}
		// Preserve output/usage, but replace provider diagnostics with the same
		// masked public error used by the streaming gate.
		responseBody, err = sjson.SetBytes(responseBody, "error", protocolErr.ToOpenAIError())
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
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
	if gated && writeOK {
		info.StreamGate.FinishPlainResponse(protocolErr)
		if protocolErr != nil {
			if info.StreamStatus == nil {
				info.StreamStatus = relaycommon.NewStreamStatus()
			}
			info.StreamStatus.MarkFailed(string(protocolErr.GetErrorCode()), protocolErr.ToOpenAIError().Type, protocolErr.StatusCode)
		}
		releaseCommittedStreamRequest(info)
	}

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
	settlementUsage := &usage
	if gated && responsesResponse.Usage == nil {
		settlementUsage = nil
	}
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return settlementUsage, nil
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
	return settlementUsage, nil
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
	var completedStreamData []byte
	if upID := service.GPTUpstreamRequestID(resp.Header); upID != "" {
		c.Set(common.UpstreamRequestIdKey, upID)
	}
	if info != nil {
		info.ApplyDynamicBillingMultiplierFromHeaders(resp.Header, relaycommon.DynamicBillingMultiplierSourceHeader)
	}
	doneBuffer := beginResponsesDoneBuffering(c)
	if info != nil {
		if info.StreamStatus == nil {
			info.StreamStatus = relaycommon.NewStreamStatus()
		}
		info.StreamStatus.RequireTerminal()
	}

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

		observeResponsesProtocolOutcome(info, streamResponse.Type, data)
		applyResponsesStreamUsage(usage, streamResponse.Response)
		if info != nil && streamResponse.Response != nil && streamResponse.Response.Usage != nil {
			info.HasTrustedUsage = true
		}
		shouldDelayCompleted := streamResponse.Type == "response.completed" && openAIResponsesCompletedWithUsage(streamResponse.Response)
		if shouldDelayCompleted {
			completedCopy := streamResponse
			completedStreamResponse = &completedCopy
			completedStreamData = bytes.Clone(data)
		} else if err := sendResponsesStreamBytes(c, streamResponse, data); err != nil {
			sr.Stop(err)
			return
		}
		releaseCommittedStreamRequest(info)
		if streamResponse.Type == "error" || streamResponse.Type == "response.error" || streamResponse.Type == "response.failed" || responsesIncompleteFailure(streamResponse.Type, data) {
			// The gate retains failure until the controller decides retry/finalize.
			// Record diagnostics for legacy streams without treating normal limits
			// or client/moderation cancellation as an upstream outage.
			sr.Error(fmt.Errorf("responses stream terminal error: %s", streamResponse.Type))
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
			if data, err := sjson.SetBytes(completedStreamData, "newapi_billing", billing); err == nil {
				completedStreamData = data
			} else {
				logger.LogError(c, "failed to marshal responses completed stream billing metadata: "+err.Error())
			}
		}
		if err := sendResponsesStreamBytes(c, *completedStreamResponse, completedStreamData); err != nil {
			finalWriteOK = false
			if info != nil && info.StreamStatus != nil {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
			}
		}
		releaseCommittedStreamRequest(info)
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
	if info != nil && info.StreamGate != nil {
		// Trailer-aware delayed completion must reach the gate before EOF.
		service.StreamAttemptError(info)
	}

	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	// 软错误结束且未观察到有效输出（输出 token < responsesSoftErrorMinOutputTokens）时不计费：
	// 返回 nil usage，使结算层将其视为无可信 usage，fixed_request 退预扣、usage-token 扣 0，且不注入 NewAPIBilling。
	if info != nil && info.StreamGate == nil && info.StreamStatus != nil && info.StreamStatus.HasErrors() && usage.OutputTokens < responsesSoftErrorMinOutputTokens {
		return nil, nil
	}

	if info != nil && info.StreamGate != nil && !info.HasTrustedUsage {
		return nil, nil
	}
	return usage, nil
}

func releaseCommittedStreamRequest(info *relaycommon.RelayInfo) {
	if info == nil || info.StreamGate == nil || info.StreamGate.CanRetry() || info.StreamRequestCleanup == nil {
		return
	}
	cleanup := info.StreamRequestCleanup
	info.StreamRequestCleanup = nil
	cleanup()
}

func responsesIncompleteFailure(eventType string, data []byte) bool {
	if eventType != "response.incomplete" {
		return false
	}
	switch gjson.GetBytes(data, "response.incomplete_details.reason").String() {
	case "server_error", "overloaded", "rate_limit_exceeded", "timeout":
		return true
	}
	return false
}

func observeResponsesProtocolOutcome(info *relaycommon.RelayInfo, eventType string, data []byte) {
	if info == nil || info.StreamStatus == nil {
		return
	}
	switch eventType {
	case "response.completed", "response.done":
		info.StreamStatus.MarkCompleted()
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	case "response.cancelled", "response.canceled":
		info.StreamStatus.MarkCancelled()
	case "response.incomplete":
		reason := gjson.GetBytes(data, "response.incomplete_details.reason").String()
		if responsesIncompleteFailure(eventType, data) {
			info.StreamStatus.MarkFailed(reason, "upstream_error", http.StatusBadGateway)
		} else {
			info.StreamStatus.MarkIncomplete(reason)
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
		}
	case "error", "response.error", "response.failed":
		errValue := gjson.GetBytes(data, "response.error")
		if !errValue.Exists() || errValue.Type == gjson.Null {
			errValue = gjson.GetBytes(data, "error")
		}
		if !errValue.Exists() || errValue.Type == gjson.Null {
			errValue = gjson.ParseBytes(data)
		}
		status := int(errValue.Get("status_code").Int())
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		info.StreamStatus.MarkFailed(errValue.Get("code").String(), errValue.Get("type").String(), status)
	}
}

func applyResponsesStreamUsage(usage *dto.Usage, response *dto.OpenAIResponsesResponse) {
	if usage == nil || response == nil || response.Usage == nil {
		return
	}
	usage.PromptTokens = response.Usage.InputTokens
	usage.InputTokens = response.Usage.InputTokens
	usage.CompletionTokens = response.Usage.OutputTokens
	usage.OutputTokens = response.Usage.OutputTokens
	usage.TotalTokens = response.Usage.TotalTokens
	usage.CompletionTokenDetails = response.Usage.CompletionTokenDetails
	if response.Usage.InputTokensDetails != nil {
		usage.PromptTokensDetails.CachedTokens = response.Usage.InputTokensDetails.CachedTokens
		usage.PromptTokensDetails.ImageTokens = response.Usage.InputTokensDetails.ImageTokens
		usage.PromptTokensDetails.AudioTokens = response.Usage.InputTokensDetails.AudioTokens
	}
}
