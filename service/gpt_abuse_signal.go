package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	GPTAbuseSeverityHigh   = "high"
	GPTAbuseSeverityMedium = "medium"

	GPTAbuseKindCyberPolicy                 = "cyber_policy"
	GPTAbuseKindHighRiskCyberReroute        = "high_risk_cyber_reroute"
	GPTAbuseKindInvalidPromptSafety         = "invalid_prompt_safety"
	GPTAbuseKindContentPolicyViolation      = "content_policy_violation"
	GPTAbuseKindGenericPolicyViolation      = "generic_policy_violation"
	GPTAbuseKindGenericAbuseSecurityWarning = "generic_abuse_security_warning"

	GPTAbuseSourceHTTPError         = "http_error"
	GPTAbuseSourceSSEResponseFailed = "sse_response_failed"
	GPTAbuseSourceSSEMetadata       = "sse_metadata"
	GPTAbuseSourceModelReroute      = "model_reroute"
)

type GPTAbuseSignal struct {
	Matched           bool
	Kind              string
	Severity          string
	Source            string
	StatusCode        int
	ErrorCode         string
	ErrorType         string
	RequestedModel    string
	UpstreamModel     string
	UpstreamRequestId string
	Stream            bool
	CountEligible     bool
	Extra             string
}

type gptErrorEnvelope struct {
	Error gptErrorObject `json:"error"`
}

type gptErrorObject struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

func ClassifyGPTAbuseSignalFromHTTPError(statusCode int, body []byte) GPTAbuseSignal {
	errorObject := parseGPTErrorObject(body)
	return classifyGPTAbuseError(statusCode, errorObject, GPTAbuseSourceHTTPError)
}

func ClassifyGPTAbuseSignalFromSSEEvent(eventType string, data string) GPTAbuseSignal {
	return ClassifyGPTAbuseSignalFromSSEEventBytes(eventType, common.StringToByteSlice(data))
}

func ClassifyGPTAbuseSignalFromSSEEventBytes(eventType string, data []byte) GPTAbuseSignal {
	eventType = strings.TrimSpace(eventType)
	if eventType == "response.metadata" && containsTrustedAccessForCyberBytes(data) {
		return GPTAbuseSignal{Matched: true, Kind: GPTAbuseKindHighRiskCyberReroute, Severity: GPTAbuseSeverityHigh, Source: GPTAbuseSourceSSEMetadata, CountEligible: true, Stream: true, Extra: gptAbuseUpstreamWarningExtra(eventType, "", gptErrorObject{}, `{"openai_verification_recommendation":["trusted_access_for_cyber"]}`)}
	}
	if eventType != "response.failed" && eventType != "response.error" {
		return GPTAbuseSignal{}
	}
	errorObject, responseStatus := parseGPTSSEErrorObject(data)
	signal := classifyGPTAbuseError(http.StatusInternalServerError, errorObject, GPTAbuseSourceSSEResponseFailed)
	signal.Stream = true
	if signal.Matched {
		signal.Extra = gptAbuseUpstreamWarningExtra(eventType, responseStatus, errorObject, gptAbuseErrorObjectRaw(errorObject))
	}
	return signal
}

func GPTUpstreamRequestID(headers http.Header) string {
	if headers == nil {
		return ""
	}
	for _, key := range []string{"x-request-id", "X-Request-ID", "openai-request-id", common.RequestIdKey} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func ResolveGPTAbuseWarningLimit(plan *model.SubscriptionPlan) int {
	return model.ResolveGPTAbuseWarningLimit(plan)
}

func GPTAwareRelayErrorHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, showBodyWhenFail bool) *types.NewAPIError {
	newApiErr := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	defer CloseResponseBodyGracefully(resp)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return newApiErr
	}
	upstreamRequestId := GPTUpstreamRequestID(resp.Header)
	if c != nil && upstreamRequestId != "" {
		c.Set(common.UpstreamRequestIdKey, upstreamRequestId)
	}
	if ShouldMonitorGPTAbuse(info) {
		signal := ClassifyGPTAbuseSignalFromHTTPError(resp.StatusCode, responseBody)
		signal.UpstreamRequestId = upstreamRequestId
		if info != nil {
			signal.Stream = info.IsStream
			signal.RequestedModel = info.OriginModelName
			signal.UpstreamModel = info.UpstreamModelName
		}
		RecordGPTAbuseSignal(c, info, signal)
	}
	ctx := contextFromGin(c)
	return relayErrorHandlerFromBody(ctx, resp.StatusCode, responseBody, showBodyWhenFail)
}

func EnforceGPTAbuseSuspension(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if !common.GPTAbuseLimitEnabled || !shouldEnforceGPTAbuseSuspension(c, info) {
		return nil
	}
	userID := 0
	if info != nil {
		userID = info.UserId
	}
	if userID <= 0 && c != nil {
		userID = common.GetContextKeyInt(c, constant.ContextKeyUserId)
	}
	if userID <= 0 {
		return nil
	}
	suspension, err := model.GetActiveGPTAbuseSuspension(userID, common.GetTimestamp())
	if err != nil {
		logger.LogWarn(contextFromGin(c), "check GPT abuse suspension failed: "+err.Error())
		return types.NewErrorWithStatusCode(err, types.ErrorCodeQueryDataError, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	if suspension == nil {
		return nil
	}
	return types.WithOpenAIError(types.OpenAIError{Message: "当前账号因触发 GPT 安全策略警告已暂停服务，请于次日恢复后重试", Type: string(types.ErrorCodeGPTAbuseSuspended), Code: types.ErrorCodeGPTAbuseSuspended}, http.StatusForbidden, types.ErrOptionWithSkipRetry())
}

func ShouldMonitorGPTAbuse(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	modelName := strings.ToLower(strings.TrimSpace(info.OriginModelName))
	if isGPTAbuseModelName(modelName) {
		return true
	}
	switch info.ChannelType {
	case constant.ChannelTypeOpenAI, constant.ChannelTypeAzure, constant.ChannelTypeOpenAIMax, constant.ChannelTypeOhMyGPT, constant.ChannelTypeAPI2GPT, constant.ChannelTypeAIProxy, constant.ChannelTypeAIProxyLibrary, constant.ChannelTypeCodex:
		return true
	default:
		return false
	}
}

func RecordGPTAbuseSignal(c *gin.Context, info *relaycommon.RelayInfo, signal GPTAbuseSignal) {
	if !signal.Matched {
		return
	}
	log := buildGPTAbuseSignalLog(c, info, signal)
	inserted, err := model.RecordGPTAbuseSignalLog(log)
	if err != nil {
		logger.LogWarn(contextFromGin(c), "record GPT abuse signal failed: "+err.Error())
		return
	}
	if !inserted {
		return
	}
	StoreGPTAbuseRepeatBlock(c, info, log)
	if !log.CountEligible || !common.GPTAbuseLimitEnabled {
		return
	}
	applyGPTAbuseLimit(c, log)
}

func buildGPTAbuseSignalLog(c *gin.Context, info *relaycommon.RelayInfo, signal GPTAbuseSignal) *model.GPTAbuseSignalLog {
	now := common.GetTimestamp()
	userId := 0
	tokenId := 0
	channelId := 0
	channelType := 0
	channelMultiKeyIndex := 0
	requestId := ""
	upstreamRequestId := strings.TrimSpace(signal.UpstreamRequestId)
	endpoint := ""
	relayMode := 0
	requestedModel := strings.TrimSpace(signal.RequestedModel)
	upstreamModel := strings.TrimSpace(signal.UpstreamModel)
	isStream := signal.Stream
	if info != nil {
		userId = info.UserId
		tokenId = info.TokenId
		requestId = info.RequestId
		endpoint = info.RequestURLPath
		relayMode = info.RelayMode
		isStream = isStream || info.IsStream
		if requestedModel == "" {
			requestedModel = info.OriginModelName
		}
		if upstreamModel == "" {
			upstreamModel = info.UpstreamModelName
		}
		if info.ChannelMeta != nil {
			channelId = info.ChannelId
			channelType = info.ChannelType
			channelMultiKeyIndex = info.ChannelMultiKeyIndex
		}
	}
	if c != nil {
		if userId == 0 {
			userId = common.GetContextKeyInt(c, constant.ContextKeyUserId)
		}
		if tokenId == 0 {
			tokenId = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
		}
		if channelId == 0 {
			channelId = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		}
		if channelType == 0 {
			channelType = common.GetContextKeyInt(c, constant.ContextKeyChannelType)
		}
		if channelMultiKeyIndex == 0 {
			channelMultiKeyIndex = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		if requestId == "" {
			requestId = common.GetContextKeyString(c, common.RequestIdKey)
		}
		if upstreamRequestId == "" {
			upstreamRequestId = c.GetString(common.UpstreamRequestIdKey)
		}
		if endpoint == "" && c.Request != nil && c.Request.URL != nil {
			endpoint = c.Request.URL.Path
		}
	}
	log := &model.GPTAbuseSignalLog{
		CreatedAt:            now,
		UserId:               userId,
		Username:             ginString(c, "username"),
		UserEmail:            ginString(c, string(constant.ContextKeyUserEmail)),
		TokenId:              tokenId,
		TokenName:            ginString(c, "token_name"),
		ChannelId:            channelId,
		ChannelName:          ginString(c, "channel_name"),
		ChannelType:          channelType,
		ChannelMultiKeyIndex: channelMultiKeyIndex,
		RequestId:            requestId,
		UpstreamRequestId:    upstreamRequestId,
		Endpoint:             endpoint,
		RelayMode:            relayMode,
		RequestedModel:       requestedModel,
		UpstreamModel:        upstreamModel,
		IsStream:             isStream,
		Source:               signal.Source,
		Kind:                 signal.Kind,
		Severity:             signal.Severity,
		Extra:                signal.Extra,
		StatusCode:           signal.StatusCode,
		ErrorCode:            signal.ErrorCode,
		ErrorType:            signal.ErrorType,
		CountEligible:        signal.CountEligible,
	}
	log.DedupeKey = gptAbuseDedupeKey(log)
	return log
}

func applyGPTAbuseLimit(c *gin.Context, log *model.GPTAbuseSignalLog) {
	if log == nil || log.UserId <= 0 {
		return
	}
	summary, err := model.GetSubscriptionSelfSummary(log.UserId)
	if err != nil {
		logger.LogWarn(contextFromGin(c), "count GPT abuse warnings failed: "+err.Error())
		return
	}
	if summary.GPTAbuseWarningLimit <= 0 || summary.GPTAbuseWarningCount < summary.GPTAbuseWarningLimit {
		return
	}
	_, dayEnd := model.GPTAbuseDayWindow(log.CreatedAt)
	if err := model.UpsertGPTAbuseSuspension(log.UserId, log.Id, summary.GPTAbuseWarningCount, summary.GPTAbuseWarningLimit, dayEnd); err != nil {
		logger.LogWarn(contextFromGin(c), "create GPT abuse suspension failed: "+err.Error())
	}
}

func shouldEnforceGPTAbuseSuspension(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	if !isGPTAbuseSuspensionRelayMode(info.RelayMode) {
		return false
	}
	if info.ChannelMeta != nil {
		upstreamModel := strings.TrimSpace(info.ChannelMeta.UpstreamModelName)
		if upstreamModel != "" {
			return isGPTAbuseModelName(upstreamModel)
		}
	}
	return isGPTAbuseModelName(info.OriginModelName)
}

func isGPTAbuseSuspensionRelayMode(relayMode int) bool {
	switch relayMode {
	case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeCompletions, relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact, relayconstant.RelayModeRealtime:
		return true
	default:
		return false
	}
}

func isGPTAbuseModelName(modelName string) bool {
	modelName = strings.TrimPrefix(modelName, "openai/")
	return strings.HasPrefix(modelName, "gpt-") || strings.HasPrefix(modelName, "chatgpt-") || strings.HasPrefix(modelName, "o1") || strings.HasPrefix(modelName, "o3") || strings.HasPrefix(modelName, "o4") || strings.HasPrefix(modelName, "codex-")
}

func ginString(c *gin.Context, key string) string {
	if c == nil {
		return ""
	}
	return c.GetString(key)
}

func contextFromGin(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func gptAbuseDedupeKey(log *model.GPTAbuseSignalLog) string {
	var b strings.Builder
	b.WriteString(common.Interface2String(log.UserId))
	b.WriteByte('|')
	b.WriteString(common.Interface2String(log.TokenId))
	b.WriteByte('|')
	b.WriteString(common.Interface2String(log.ChannelId))
	b.WriteByte('|')
	b.WriteString(log.RequestId)
	b.WriteByte('|')
	b.WriteString(log.UpstreamRequestId)
	b.WriteByte('|')
	b.WriteString(log.Source)
	b.WriteByte('|')
	b.WriteString(log.Kind)
	return b.String()
}

func parseGPTErrorObject(body []byte) gptErrorObject {
	var envelope gptErrorEnvelope
	if err := common.Unmarshal(body, &envelope); err != nil {
		return gptErrorObject{Message: string(body)}
	}
	return envelope.Error
}

func parseGPTSSEErrorObject(data []byte) (gptErrorObject, string) {
	var payload struct {
		Error    gptErrorObject `json:"error"`
		Response struct {
			Status string         `json:"status"`
			Error  gptErrorObject `json:"error"`
		} `json:"response"`
	}
	if err := common.Unmarshal(data, &payload); err != nil {
		return gptErrorObject{Message: string(data)}, ""
	}
	if payload.Response.Error.Message != "" || payload.Response.Error.Type != "" || payload.Response.Error.Code != nil {
		return payload.Response.Error, payload.Response.Status
	}
	return payload.Error, payload.Response.Status
}

func classifyGPTAbuseError(statusCode int, errObj gptErrorObject, source string) GPTAbuseSignal {
	code := strings.TrimSpace(anyToString(errObj.Code))
	errType := strings.TrimSpace(errObj.Type)
	message := strings.TrimSpace(errObj.Message)
	lowerCode := strings.ToLower(code)
	lowerType := strings.ToLower(errType)
	lowerMessage := strings.ToLower(message)

	if isExcludedGPTAbuseError(lowerCode, lowerType, lowerMessage) {
		return GPTAbuseSignal{StatusCode: statusCode, ErrorCode: code, ErrorType: errType, Source: source}
	}

	base := GPTAbuseSignal{Matched: true, Source: source, StatusCode: statusCode, ErrorCode: code, ErrorType: errType, CountEligible: true}
	switch {
	case lowerCode == "cyber_policy" || strings.Contains(lowerMessage, "possible cybersecurity risk") || strings.Contains(lowerMessage, "high-risk cyber activity"):
		base.Kind = GPTAbuseKindCyberPolicy
		base.Severity = GPTAbuseSeverityHigh
	case lowerCode == "invalid_prompt" && containsAny(lowerMessage, "safety", "policy", "disallowed", "not allowed"):
		base.Kind = GPTAbuseKindInvalidPromptSafety
		base.Severity = GPTAbuseSeverityMedium
	case containsAny(lowerCode, "content_policy", "policy_violation", "safety_violation", "moderation_blocked", "content_filter") || containsAny(lowerType, "content_policy", "policy_violation", "safety_violation", "moderation_blocked", "content_filter") || containsAny(lowerMessage, "content policy", "policy violation", "safety violation", "moderation", "content filter"):
		base.Kind = GPTAbuseKindContentPolicyViolation
		base.Severity = GPTAbuseSeverityMedium
	case isGenericGPTPolicyViolationMessage(lowerMessage):
		base.Kind = GPTAbuseKindGenericPolicyViolation
		base.Severity = GPTAbuseSeverityMedium
	case containsAny(lowerMessage, "network security warning", "cyber abuse", "abuse policy"):
		base.Kind = GPTAbuseKindGenericAbuseSecurityWarning
		base.Severity = GPTAbuseSeverityMedium
	default:
		base.Matched = false
		base.CountEligible = false
	}
	if base.Matched && base.Extra == "" {
		base.Extra = gptAbuseUpstreamWarningExtra(source, "", errObj, "")
	}
	return base
}

func isExcludedGPTAbuseError(code string, errType string, message string) bool {
	return containsAny(code,
		"rate_limit_exceeded",
		"insufficient_quota",
		"server_is_overloaded",
		"overloaded",
		"slow_down",
		"context_length_exceeded",
		"unsupported_parameter",
		"invalid_image",
		"previous_response_not_found",
		"invalid_encrypted_content",
		"quota_exceeded",
	) || containsAny(errType, "rate_limit", "insufficient_quota") || containsAny(message, "rate limit", "current quota", "overloaded", "context length", "unsupported parameter", "not allowed for this model")
}

func isGenericGPTPolicyViolationMessage(message string) bool {
	if containsAny(message, "usage policy", "policy violation") {
		return true
	}
	if strings.Contains(message, "not allowed") && containsAny(message, "policy", "safety", "abuse", "cyber") {
		return true
	}
	if strings.Contains(message, "violat") && containsAny(message, "policy", "safety", "abuse", "cyber") {
		return true
	}
	return false
}

func gptAbuseErrorObjectRaw(errObj gptErrorObject) string {
	fields := map[string]any{}
	if message := strings.TrimSpace(errObj.Message); message != "" {
		fields["message"] = truncateGPTAbuseWarningDetail(message)
	}
	if errType := strings.TrimSpace(errObj.Type); errType != "" {
		fields["type"] = truncateGPTAbuseWarningDetail(errType)
	}
	if code := strings.TrimSpace(anyToString(errObj.Code)); code != "" {
		fields["code"] = truncateGPTAbuseWarningDetail(code)
	}
	if len(fields) == 0 {
		return ""
	}
	data, err := common.Marshal(fields)
	if err != nil {
		return ""
	}
	return string(data)
}

func gptAbuseUpstreamWarningExtra(eventType string, responseStatus string, errObj gptErrorObject, rawError string) string {
	warning := map[string]any{
		"event_type":    strings.TrimSpace(eventType),
		"error_code":    truncateGPTAbuseWarningDetail(strings.TrimSpace(anyToString(errObj.Code))),
		"error_type":    truncateGPTAbuseWarningDetail(strings.TrimSpace(errObj.Type)),
		"error_message": truncateGPTAbuseWarningDetail(strings.TrimSpace(errObj.Message)),
	}
	if status := strings.TrimSpace(responseStatus); status != "" {
		warning["response_status"] = status
	}
	if raw := truncateGPTAbuseWarningDetail(strings.TrimSpace(rawError)); raw != "" {
		warning["raw_error"] = raw
	}
	extra, err := common.Marshal(map[string]any{"upstream_warning": warning})
	if err != nil {
		return ""
	}
	return string(extra)
}

func truncateGPTAbuseWarningDetail(value string) string {
	const maxLen = 4096
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func containsTrustedAccessForCyber(value string) bool {
	return strings.Contains(strings.ToLower(value), "trusted_access_for_cyber")
}

func containsTrustedAccessForCyberBytes(value []byte) bool {
	return bytes.Contains(bytes.ToLower(value), []byte("trusted_access_for_cyber"))
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func anyToString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return common.Interface2String(v)
	}
}

// no local helpers
