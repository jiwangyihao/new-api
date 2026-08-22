package service

import (
	"encoding/base64"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func appendRequestPath(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if other == nil {
		return
	}
	if ctx != nil && ctx.Request != nil && ctx.Request.URL != nil {
		if path := ctx.Request.URL.Path; path != "" {
			other["request_path"] = path
			return
		}
	}
	if relayInfo != nil && relayInfo.RequestURLPath != "" {
		path := relayInfo.RequestURLPath
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}
		other["request_path"] = path
	}
}

func noteQuotaClamp(relayInfo *relaycommon.RelayInfo, clamp *common.QuotaClamp) {
	if relayInfo == nil || clamp == nil || relayInfo.QuotaClamp != nil {
		return
	}
	relayInfo.QuotaClamp = clamp
}

func attachQuotaClamp(other map[string]interface{}, clamp *common.QuotaClamp) {
	if other == nil || clamp == nil {
		return
	}
	adminInfo, _ := other["admin_info"].(map[string]interface{})
	if adminInfo == nil {
		adminInfo = make(map[string]interface{})
		other["admin_info"] = adminInfo
	}
	adminInfo["quota_saturation"] = clamp.AuditMap()
}

func attachQuotaSaturation(other map[string]interface{}, relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil {
		return
	}
	attachQuotaClamp(other, relayInfo.QuotaClamp)
}

func GenerateTextOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, modelPrice float64) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_ratio"] = modelRatio
	other["completion_ratio"] = completionRatio
	other["cache_tokens"] = cacheTokens
	other["cache_ratio"] = cacheRatio
	other["model_price"] = modelPrice
	if relayInfo.HasSendResponse() {
		other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
	}
	if ctx != nil {
		if bufferTimeMs := common.GetContextKeyInt(ctx, constant.ContextKeyRequestBufferTimeMs); bufferTimeMs > 0 {
			other["request_buffer_time_ms"] = bufferTimeMs
		}
	}
	if relayInfo.ReasoningEffort != "" {
		other["reasoning_effort"] = relayInfo.ReasoningEffort
	}
	if relayInfo.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = relayInfo.UpstreamModelName
	}

	isSystemPromptOverwritten := common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride)
	if isSystemPromptOverwritten {
		other["is_system_prompt_overwritten"] = true
	}

	adminInfo := make(map[string]interface{})
	adminInfo["use_channel"] = ctx.GetStringSlice("use_channel")
	isMultiKey := common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey)
	if isMultiKey {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex)
	}

	isLocalCountTokens := common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens)
	if isLocalCountTokens {
		adminInfo["local_count_tokens"] = isLocalCountTokens
	}

	AppendChannelAffinityAdminInfo(ctx, adminInfo)

	other["admin_info"] = adminInfo
	appendRequestPath(ctx, relayInfo, other)
	appendRequestConversionChain(relayInfo, other)
	appendFinalRequestFormat(relayInfo, other)
	appendBillingInfo(relayInfo, other)
	appendParamOverrideInfo(relayInfo, other)
	appendStreamStatus(relayInfo, other)
	appendTokenLimitAuditInfo(relayInfo, other)
	attachQuotaSaturation(other, relayInfo)
	return other
}

func appendTokenLimitAuditInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || relayInfo.TokenLimit == nil {
		return
	}
	type tokenLimitAudit interface {
		AuditInfo() (bool, int64, int64, string)
	}
	audit, ok := relayInfo.TokenLimit.(tokenLimitAudit)
	if !ok {
		return
	}
	failed, actualTokens, preConsumed, failureCode := audit.AuditInfo()
	if !failed {
		return
	}
	other["api_key_token_limit_settle_failed"] = true
	other["api_key_token_limit_actual_tokens"] = actualTokens
	other["api_key_token_limit_pre_consumed"] = preConsumed
	other["api_key_token_limit_failure_code"] = failureCode
}

func appendParamOverrideInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || len(relayInfo.ParamOverrideAudit) == 0 {
		return
	}
	other["po"] = relayInfo.ParamOverrideAudit
}

func appendStreamStatus(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || !relayInfo.IsStream || relayInfo.StreamStatus == nil {
		return
	}
	ss := relayInfo.StreamStatus
	status := "ok"
	if !ss.IsNormalEnd() || ss.HasErrors() {
		status = "error"
	}
	streamInfo := map[string]interface{}{
		"status":     status,
		"end_reason": string(ss.EndReason),
	}
	if ss.EndError != nil {
		streamInfo["end_error"] = ss.EndError.Error()
	}
	if ss.ErrorCount > 0 {
		streamInfo["error_count"] = ss.ErrorCount
		messages := make([]string, 0, len(ss.Errors))
		for _, e := range ss.Errors {
			messages = append(messages, e.Message)
		}
		streamInfo["errors"] = messages
	}
	other["stream_status"] = streamInfo
}

func appendChannelTokenBillingSnapshotInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	other["channel_token_billing_multiplier"] = relayInfo.FrozenChannelTokenBillingMultiplier()
	other["credit_billing_mode"] = relayInfo.FrozenCreditBillingMode()
	other["fixed_request_credits"] = relayInfo.FixedRequestCredits
	other["dynamic_billing_multiplier_enabled"] = relayInfo.DynamicBillingMultiplierEnabled
	other["has_trusted_usage"] = relayInfo.HasTrustedUsage
	other["raw_metered_tokens"] = relayInfo.RawMeteredTokens
	other["channel_billable_tokens"] = relayInfo.ChannelBillableTokens
	other["api_key_billable_tokens"] = relayInfo.ApiKeyBillableTokens
	other["subscription_billable_tokens"] = relayInfo.SubscriptionBillableTokens
	other["base_credits"] = relayInfo.CreditBillingBaseCredits
	other["api_key_credits"] = relayInfo.ApiKeyBillableTokens
	other["subscription_credits"] = relayInfo.SubscriptionBillableTokens
	other["final_credits"] = relayInfo.SubscriptionBillableTokens
	other["api_key_credits_consumed"] = relayInfo.ApiKeyBillableTokens
	other["subscription_credits_consumed"] = relayInfo.SubscriptionBillableTokens
	if relayInfo.CreditBillingZeroReason != "" {
		other["credit_billing_zero_reason"] = relayInfo.CreditBillingZeroReason
	}
	other["estimated_raw_tokens"] = relayInfo.EstimatedRawTokens
	other["initial_channel_id"] = relayInfo.InitialChannelId
	other["initial_channel_type"] = relayInfo.InitialChannelType
}

func appendNewAPIBillingInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	billing := NewAPIBillingFromRelayInfo(relayInfo)
	other["billing_multiplier"] = billing.BillingMultiplier
	other["billing_multiplier_source"] = billing.BillingMultiplierSource
	other["dynamic_billing_multiplier"] = relayInfo.FrozenDynamicBillingMultiplier()
	other["dynamic_billing_multiplier_source"] = relayInfo.FrozenDynamicBillingMultiplierSource()
	if relayInfo.DynamicBillingMultiplierIgnoredReason != "" {
		other["dynamic_billing_multiplier_ignored_reason"] = relayInfo.DynamicBillingMultiplierIgnoredReason
	}
	other["metered_tokens"] = billing.MeteredTokens
	other["billable_tokens"] = billing.BillableTokens
	other["codex_pro_requested"] = billing.CodexProRequested
	other["codex_pro_served"] = billing.CodexProServed
}

func appendBillingInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	appendChannelTokenBillingSnapshotInfo(relayInfo, other)
	appendNewAPIBillingInfo(relayInfo, other)
	// billing_source: "wallet" or "subscription"
	if relayInfo.BillingSource != "" {
		other["billing_source"] = relayInfo.BillingSource
	}
	if relayInfo.UserSetting.BillingPreference != "" {
		other["billing_preference"] = relayInfo.UserSetting.BillingPreference
	}
	if relayInfo.BillingSource == "subscription" {
		if relayInfo.SubscriptionId != 0 {
			other["subscription_id"] = relayInfo.SubscriptionId
		}
		if relayInfo.SubscriptionPreConsumed > 0 {
			other["subscription_pre_consumed"] = relayInfo.SubscriptionPreConsumed
		}
		// post_delta: settlement delta applied after actual usage is known (can be negative for refund)
		if relayInfo.SubscriptionPostDelta != 0 {
			other["subscription_post_delta"] = relayInfo.SubscriptionPostDelta
		}
		if relayInfo.SubscriptionPlanId != 0 {
			other["subscription_plan_id"] = relayInfo.SubscriptionPlanId
		}
		if relayInfo.SubscriptionPlanTitle != "" {
			other["subscription_plan_title"] = relayInfo.SubscriptionPlanTitle
		}
		if relayInfo.SubscriptionDistributorTokenBilling {
			appendSubscriptionTokenInfo(relayInfo, other)
		}
		// Compute legacy subscription consumed + remaining for compatibility.
		consumed := relayInfo.SubscriptionPreConsumed + relayInfo.SubscriptionPostDelta
		usedFinal := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		if consumed < 0 {
			consumed = 0
		}
		if usedFinal < 0 {
			usedFinal = 0
		}
		if relayInfo.SubscriptionAmountTotal > 0 {
			remain := relayInfo.SubscriptionAmountTotal - usedFinal
			if remain < 0 {
				remain = 0
			}
			other["subscription_total"] = relayInfo.SubscriptionAmountTotal
			other["subscription_used"] = usedFinal
			other["subscription_remain"] = remain
		}
		if consumed > 0 {
			other["subscription_consumed"] = consumed
		}
		// Wallet quota is not deducted when billed from subscription.
		other["wallet_quota_deducted"] = 0
	}
}

func appendSubscriptionTokenInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	consumed := relayInfo.SubscriptionBillableTokens
	if consumed == 0 {
		consumed = relayInfo.SubscriptionPreConsumed + relayInfo.SubscriptionPostDelta
	}
	if consumed < 0 {
		consumed = 0
	}
	usedFinal := relayInfo.SubscriptionTokenUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
	if usedFinal < 0 {
		usedFinal = 0
	}
	remaining := int64(0)
	if !relayInfo.SubscriptionTokenUnlimited && relayInfo.SubscriptionTokenLimit > 0 {
		remaining = relayInfo.SubscriptionTokenLimit - usedFinal
		if remaining < 0 {
			remaining = 0
		}
	}
	other["subscription_token_limit"] = relayInfo.SubscriptionTokenLimit
	other["subscription_token_used"] = usedFinal
	other["subscription_token_remaining"] = remaining
	other["subscription_token_unlimited"] = relayInfo.SubscriptionTokenUnlimited
	other["subscription_tokens_consumed"] = consumed
}

func appendRequestConversionChain(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if len(relayInfo.RequestConversionChain) == 0 {
		return
	}
	chain := make([]string, 0, len(relayInfo.RequestConversionChain))
	for _, f := range relayInfo.RequestConversionChain {
		switch f {
		case types.RelayFormatOpenAI:
			chain = append(chain, "OpenAI Compatible")
		case types.RelayFormatClaude:
			chain = append(chain, "Claude Messages")
		case types.RelayFormatGemini:
			chain = append(chain, "Google Gemini")
		case types.RelayFormatOpenAIResponses:
			chain = append(chain, "OpenAI Responses")
		default:
			chain = append(chain, string(f))
		}
	}
	if len(chain) == 0 {
		return
	}
	other["request_conversion"] = chain
}

func appendFinalRequestFormat(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		// claude indicates the final upstream request format is Claude Messages.
		// Frontend log rendering uses this to keep the original Claude input display.
		other["claude"] = true
	}
}

func GenerateWssOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage, modelRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, completionRatio, 0, 0.0, modelPrice)
	info["ws"] = true
	info["audio_input"] = usage.InputTokenDetails.AudioTokens
	info["audio_output"] = usage.OutputTokenDetails.AudioTokens
	info["text_input"] = usage.InputTokenDetails.TextTokens
	info["text_output"] = usage.OutputTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateAudioOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, modelRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, completionRatio, 0, 0.0, modelPrice)
	info["audio"] = true
	info["audio_input"] = usage.PromptTokensDetails.AudioTokens
	info["audio_output"] = usage.CompletionTokenDetails.AudioTokens
	info["text_input"] = usage.PromptTokensDetails.TextTokens
	info["text_output"] = usage.CompletionTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateClaudeOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64,
	cacheCreationTokens int, cacheCreationRatio float64,
	cacheCreationTokens5m int, cacheCreationRatio5m float64,
	cacheCreationTokens1h int, cacheCreationRatio1h float64,
	modelPrice float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, completionRatio, cacheTokens, cacheRatio, modelPrice)
	info["claude"] = true
	info["cache_creation_tokens"] = cacheCreationTokens
	info["cache_creation_ratio"] = cacheCreationRatio
	if cacheCreationTokens5m != 0 {
		info["cache_creation_tokens_5m"] = cacheCreationTokens5m
		info["cache_creation_ratio_5m"] = cacheCreationRatio5m
	}
	if cacheCreationTokens1h != 0 {
		info["cache_creation_tokens_1h"] = cacheCreationTokens1h
		info["cache_creation_ratio_1h"] = cacheCreationRatio1h
	}
	return info
}

func GenerateMjOtherInfo(relayInfo *relaycommon.RelayInfo, priceData types.PriceData) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_price"] = priceData.ModelPrice
	appendRequestPath(nil, relayInfo, other)
	return other
}

// InjectTieredBillingInfo overlays tiered billing fields onto an existing
// module-specific other map. Call this after GenerateTextOtherInfo /
// GenerateClaudeOtherInfo / etc. when the request used tiered_expr billing.
func InjectTieredBillingInfo(other map[string]interface{}, relayInfo *relaycommon.RelayInfo, result *billingexpr.TieredResult) {
	if relayInfo == nil || other == nil {
		return
	}
	snap := relayInfo.TieredBillingSnapshot
	if snap == nil {
		return
	}
	other["billing_mode"] = "tiered_expr"
	other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(snap.ExprString))
	if result != nil {
		other["matched_tier"] = result.MatchedTier
	}
}
