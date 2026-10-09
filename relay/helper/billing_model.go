package helper

import (
	"github.com/QuantumNous/new-api/common"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
)

func ResolveMappedModelName(originModelName string, modelMapping string) (string, bool, error) {
	return common.ResolveModelMapping(originModelName, modelMapping, nil)
}

func SetCompactBillingModelFromMapping(info *relaycommon.RelayInfo, modelMapping string) error {
	if info == nil || info.RelayMode != relayconstant.RelayModeResponsesCompact {
		return nil
	}
	modelName, _, err := ResolveMappedModelName(info.OriginModelName, modelMapping)
	if err != nil {
		return err
	}
	info.BillingModelName = relaycommon.WithCompactBillingModelSuffix(modelName)
	return nil
}
