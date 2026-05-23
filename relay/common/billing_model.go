package common

import "strings"

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
	if strings.TrimSpace(info.BillingModelName) != "" {
		return info.BillingModelName
	}
	return info.OriginModelName
}
