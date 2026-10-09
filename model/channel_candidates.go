package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// GetSatisfiedChannelCandidates preserves production authorization and frozen billing
// constraints, leaving priority and weight selection until after health filtering.
func GetSatisfiedChannelCandidates(groups []string, modelName string, endpoint constant.EndpointType, used []int, multiplier float64, sameMultiplier bool, profile ChannelBillingProfile, sameProfile bool) ([]*Channel, error) {
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		var ids []int
		var err error
		if endpoint == "" {
			ids = candidateChannelIDsForGroupsLocked(groups, modelName)
		} else {
			ids, err = getCachedChannelIDsForEndpoint(modelName, endpoint)
			if err != nil {
				return nil, err
			}
			ids = intersectWithGroupCandidatesLocked(ids, groups, modelName)
		}
		ids = filterCachedChannelIDsByRetryConstraints(ids, groups, used, multiplier, sameMultiplier, profile, sameProfile)
		result := make([]*Channel, 0, len(ids))
		for _, id := range ids {
			if ch := channelsIDM[id]; ch != nil && ch.Status == common.ChannelStatusEnabled {
				result = append(result, ch)
			}
		}
		return result, nil
	}
	var abilities []Ability
	var err error
	if endpoint == "" {
		query := DB.Where("model = ? AND enabled = ?", modelName, true)
		if len(groups) > 0 && !onlyDefaultGroupWithoutExplicitMembers(groups) {
			query = query.Where(commonGroupCol+" IN ?", groups)
		}
		if err = query.Find(&abilities).Error; err != nil {
			return nil, err
		}
		abilities = uniqueAbilitiesByChannelID(abilities)
	} else {
		abilities, err = getEndpointFilteredAbilitiesForGroups(groups, modelName, endpoint)
		if err != nil {
			return nil, err
		}
		if len(abilities) == 0 {
			normalized := ratio_setting.FormatMatchingModelName(modelName)
			if normalized != "" && normalized != modelName {
				abilities, err = getEndpointFilteredAbilitiesForGroups(groups, normalized, endpoint)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	abilities, err = filterAbilitiesByRetryConstraints(abilities, groups, used, multiplier, sameMultiplier, profile, sameProfile)
	if err != nil {
		return nil, err
	}
	result := make([]*Channel, 0, len(abilities))
	for _, ability := range abilities {
		var ch Channel
		if err = DB.First(&ch, "id = ?", ability.ChannelId).Error; err != nil {
			return nil, err
		}
		if ch.Status == common.ChannelStatusEnabled {
			result = append(result, &ch)
		}
	}
	return result, nil
}
