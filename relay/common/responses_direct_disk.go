package common

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
)

func CanEncodeResponsesRequestDirectlyToDisk(request dto.OpenAIResponsesRequest, info *RelayInfo) bool {
	if info == nil {
		return false
	}
	settings := dto.ChannelOtherSettings{}
	if info.ChannelMeta != nil {
		settings = info.ChannelOtherSettings
	}
	if !settings.AllowServiceTier && request.ServiceTier != "" {
		return false
	}
	if settings.DisableStore && len(request.Store) != 0 {
		return false
	}
	if !settings.AllowSafetyIdentifier && len(request.SafetyIdentifier) != 0 {
		return false
	}
	if !settings.AllowIncludeObfuscation && request.StreamOptions != nil && request.StreamOptions.IncludeObfuscation {
		return false
	}
	paramOverride := getParamOverrideMap(info)
	if len(paramOverride) == 0 {
		return true
	}
	operations, ok := tryParseOperations(paramOverride)
	return ok && len(buildLegacyParamOverride(paramOverride)) == 0 && lo.EveryBy(operations, isHeaderOnlyOperation)
}

func ApplyResponsesHeaderOnlyOverride(info *RelayInfo) error {
	if info == nil || len(getParamOverrideMap(info)) == 0 {
		return nil
	}
	_, err := ApplyParamOverrideWithRelayInfo(nil, info)
	return err
}
