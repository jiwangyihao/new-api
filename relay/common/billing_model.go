package common

import (
	"strings"

	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const compactModelSuffix = "-openai-compact"

func WithCompactBillingModelSuffix(modelName string) string {
	if modelName == "" || strings.HasSuffix(modelName, compactModelSuffix) {
		return modelName
	}
	return modelName + compactModelSuffix
}

func ResolveBillingModelName(info *RelayInfo) string {
	if info == nil {
		return ""
	}
	if strings.TrimSpace(info.BillingModelName) != "" && compactBillingModelConfigured(info) {
		return info.BillingModelName
	}
	return info.OriginModelName
}

func compactBillingModelConfigured(info *RelayInfo) bool {
	if info == nil || info.RelayMode != constant.RelayModeResponsesCompact {
		return true
	}
	billingModelName := strings.TrimSpace(info.BillingModelName)
	if billingModelName == "" {
		return false
	}
	if _, ok := ratio_setting.GetModelPrice(billingModelName, false); ok {
		return true
	}
	if _, ok, _ := ratio_setting.GetModelRatio(billingModelName); ok {
		return true
	}
	return billing_setting.GetBillingMode(billingModelName) == billing_setting.BillingModeTieredExpr
}
