package helper

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
)

func ResolveMappedModelName(originModelName string, modelMapping string) (string, bool, error) {
	if modelMapping == "" || modelMapping == "{}" {
		return originModelName, false, nil
	}
	modelMap := make(map[string]string)
	if err := common.Unmarshal([]byte(modelMapping), &modelMap); err != nil {
		return "", false, fmt.Errorf("unmarshal_model_mapping_failed")
	}
	currentModel := originModelName
	visitedModels := map[string]bool{currentModel: true}
	isMapped := false
	for {
		mappedModel, exists := modelMap[currentModel]
		if !exists || mappedModel == "" {
			break
		}
		if visitedModels[mappedModel] {
			if mappedModel == currentModel {
				return currentModel, isMapped || currentModel != originModelName, nil
			}
			return "", false, errors.New("model_mapping_contains_cycle")
		}
		visitedModels[mappedModel] = true
		currentModel = mappedModel
		isMapped = true
	}
	return currentModel, isMapped, nil
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
