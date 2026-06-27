package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func IsChannelEnabledForGroupModel(group string, modelName string, channelID int) bool {
	var groups []string
	if group != "" {
		groups = []string{group}
	}
	return IsChannelEnabledForAnyGroupModel(groups, modelName, channelID)
}

func IsChannelEnabledForAnyGroupModel(groups []string, modelName string, channelID int) bool {
	if modelName == "" || channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		return isChannelEnabledForGroupModelDB(groups, modelName, channelID)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	candidates := candidateChannelIDsForGroupsLocked(groups, modelName)
	return isChannelIDInList(candidates, channelID)
}

func isChannelEnabledForGroupModelDB(groups []string, modelName string, channelID int) bool {
	countFor := func(model string) int64 {
		var count int64
		query := DB.Model(&Ability{}).
			Where("model = ? and channel_id = ? and enabled = ?", model, channelID, true)
		if len(groups) > 0 {
			query = query.Where("`group` IN ?", groups)
		}
		if err := query.Count(&count).Error; err != nil {
			return 0
		}
		return count
	}
	if countFor(modelName) > 0 {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized == "" || normalized == modelName {
		return false
	}
	return countFor(normalized) > 0
}

func isChannelIDInList(list []int, channelID int) bool {
	for _, id := range list {
		if id == channelID {
			return true
		}
	}
	return false
}
