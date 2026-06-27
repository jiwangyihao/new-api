package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/tokenbilling"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var model2channels map[string][]int                 // enabled channel ids keyed by model (union of all groups)
var groupModel2channels map[string]map[string][]int // group -> model -> enabled channel ids
var defaultGroupHasExplicitMembersCache bool        // whether default group has explicit member rows
var channelsIDM map[int]*Channel                    // all channels include disabled
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}
	newModel2channels := make(map[string][]int)
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		seenModels := make(map[string]struct{})
		for _, model := range strings.Split(channel.Models, ",") {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, ok := seenModels[model]; ok {
				continue
			}
			seenModels[model] = struct{}{}
			newModel2channels[model] = append(newModel2channels[model], channel.Id)
		}
	}

	// sort by priority
	for model, channels := range newModel2channels {
		sort.Slice(channels, func(i, j int) bool {
			return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
		})
		newModel2channels[model] = channels
	}

	// build group -> model -> channels using explicit membership; default group with no explicit
	// members is handled at selection time as "all channels for model".
	membership, defaultHasExplicit, membershipErr := LoadChannelGroupMembership()
	newGroupModel2channels := make(map[string]map[string][]int)
	if membershipErr != nil {
		common.SysLog("failed to load channel group membership: " + membershipErr.Error())
	} else {
		for _, channel := range channels {
			if channel.Status != common.ChannelStatusEnabled {
				continue
			}
			groupNames := membership[channel.Id]
			if len(groupNames) == 0 {
				continue
			}
			seenModels := make(map[string]struct{})
			for _, model := range strings.Split(channel.Models, ",") {
				model = strings.TrimSpace(model)
				if model == "" {
					continue
				}
				if _, ok := seenModels[model]; ok {
					continue
				}
				seenModels[model] = struct{}{}
				for _, g := range groupNames {
					if newGroupModel2channels[g] == nil {
						newGroupModel2channels[g] = make(map[string][]int)
					}
					newGroupModel2channels[g][model] = append(newGroupModel2channels[g][model], channel.Id)
				}
			}
		}
		for _, modelMap := range newGroupModel2channels {
			for model, ids := range modelMap {
				sort.Slice(ids, func(i, j int) bool {
					return newChannelId2channel[ids[i]].GetPriority() > newChannelId2channel[ids[j]].GetPriority()
				})
				modelMap[model] = ids
			}
		}
	}

	channelSyncLock.Lock()
	model2channels = newModel2channels
	groupModel2channels = newGroupModel2channels
	defaultGroupHasExplicitMembersCache = defaultHasExplicit
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

// candidateChannelIDsForModelLocked 返回某 model 的全部启用渠道（按 priority 已排序）。调用者须持有 channelSyncLock。
func candidateChannelIDsForModelLocked(model string) []int {
	channels := model2channels[model]
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = model2channels[normalizedModel]
	}
	return channels
}

// candidateChannelIDsForGroupModelLocked 返回单个分组在某 model 下的候选渠道 id。
// 默认分组无显式成员时语义为“全部渠道”，回落 model2channels。调用者须持有 channelSyncLock。
func candidateChannelIDsForGroupModelLocked(group string, model string) []int {
	if group == DefaultChannelGroupName && !defaultGroupHasExplicitMembersCache {
		return candidateChannelIDsForModelLocked(model)
	}
	modelMap := groupModel2channels[group]
	if modelMap == nil {
		return nil
	}
	channels := modelMap[model]
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = modelMap[normalizedModel]
	}
	return channels
}

// candidateChannelIDsForGroupsLocked 返回多个分组在某 model 下候选渠道 id 的并集（去重，保留 priority 排序）。
// groups 为空时回落全部渠道。调用者须持有 channelSyncLock。
func candidateChannelIDsForGroupsLocked(groups []string, model string) []int {
	if len(groups) == 0 {
		return candidateChannelIDsForModelLocked(model)
	}
	seen := make(map[int]struct{})
	union := make([]int, 0)
	for _, g := range groups {
		for _, id := range candidateChannelIDsForGroupModelLocked(g, model) {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			union = append(union, id)
		}
	}
	// 并集后按 priority 重新排序，保证 selectCachedChannelByPriority 分桶正确。
	sort.Slice(union, func(i, j int) bool {
		ci, oki := channelsIDM[union[i]]
		cj, okj := channelsIDM[union[j]]
		if !oki || !okj {
			return false
		}
		return ci.GetPriority() > cj.GetPriority()
	})
	return union
}

func GetRandomSatisfiedChannel(model string, retry int) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(model, retry)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	channels := model2channels[model]

	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = model2channels[normalizedModel]
	}

	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, model: %s, priority: %d", model, targetPriority))
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

func getCachedChannelIDsForEndpoint(model string, endpointType constant.EndpointType) ([]int, error) {
	channels := model2channels[model]
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = model2channels[normalizedModel]
	}
	if len(channels) == 0 {
		return nil, nil
	}
	filtered := make([]int, 0, len(channels))
	for _, channelID := range channels {
		channel, ok := channelsIDM[channelID]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
		}
		if ChannelSupportsEndpoint(channel, model, endpointType) {
			filtered = append(filtered, channelID)
		}
	}
	return filtered, nil
}

func channelIDSet(ids []int) map[int]struct{} {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			set[id] = struct{}{}
		}
	}
	return set
}

func filterCachedChannelIDsByRetryConstraints(channels []int, usedChannelIDs []int, frozenMultiplier float64, requireSameMultiplier bool, frozenProfile ChannelBillingProfile, requireSameProfile bool) []int {
	if len(channels) == 0 {
		return channels
	}
	used := channelIDSet(usedChannelIDs)
	filtered := make([]int, 0, len(channels))
	for _, channelID := range channels {
		if _, ok := used[channelID]; ok {
			continue
		}
		channel, ok := channelsIDM[channelID]
		if !ok {
			continue
		}
		if requireSameMultiplier && !tokenbilling.SameMultiplier(channel.GetTokenBillingMultiplier(), tokenbilling.EffectiveMultiplier(frozenMultiplier)) {
			continue
		}
		if requireSameProfile && !SameChannelBillingProfile(channel.BillingProfile(), frozenProfile) {
			continue
		}
		filtered = append(filtered, channelID)
	}
	return filtered
}

func retryIndexAfterRetryConstraints(retry int, usedChannelIDs []int, requireSameMultiplier bool, requireSameProfile bool) int {
	if len(usedChannelIDs) > 0 || requireSameMultiplier || requireSameProfile {
		return 0
	}
	return retry
}

func selectCachedChannelByPriority(channels []int, retry int, model string) (*Channel, error) {
	if len(channels) == 0 {
		return nil, nil
	}
	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}
	uniquePriorities := make(map[int]bool)
	for _, channelID := range channels {
		if channel, ok := channelsIDM[channelID]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
		}
	}
	sortedUniquePriorities := make([]int, 0, len(uniquePriorities))
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))
	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])
	sumWeight := 0
	targetChannels := make([]*Channel, 0, len(channels))
	for _, channelID := range channels {
		if channel, ok := channelsIDM[channelID]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
		}
	}
	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, model: %s, priority: %d", model, targetPriority))
	}
	smoothingFactor := 1
	smoothingAdjustment := 0
	if sumWeight == 0 {
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		smoothingFactor = 100
	}
	randomWeight := rand.Intn(sumWeight * smoothingFactor)
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	return nil, errors.New("channel not found")
}

func GetRandomSatisfiedChannelForEndpointWithRetryConstraints(group string, model string, retry int, endpointType constant.EndpointType, usedChannelIDs []int, frozenMultiplier float64, requireSameMultiplier bool, frozenProfile ChannelBillingProfile, requireSameProfile bool) (*Channel, error) {
	var groups []string
	if group != "" {
		groups = []string{group}
	}
	return GetRandomSatisfiedChannelForEndpointWithGroups(groups, model, retry, endpointType, usedChannelIDs, frozenMultiplier, requireSameMultiplier, frozenProfile, requireSameProfile)
}

// GetRandomSatisfiedChannelForEndpointWithGroups 在所选分组并集内选择渠道；groups 为空时回落全部渠道。
func GetRandomSatisfiedChannelForEndpointWithGroups(groups []string, model string, retry int, endpointType constant.EndpointType, usedChannelIDs []int, frozenMultiplier float64, requireSameMultiplier bool, frozenProfile ChannelBillingProfile, requireSameProfile bool) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelForEndpointWithGroups(groups, model, retry, endpointType, usedChannelIDs, frozenMultiplier, requireSameMultiplier, frozenProfile, requireSameProfile)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	var channels []int
	var err error
	if endpointType == "" {
		channels = candidateChannelIDsForGroupsLocked(groups, model)
	} else {
		channels, err = getCachedChannelIDsForEndpoint(model, endpointType)
		if err != nil {
			return nil, err
		}
		channels = intersectWithGroupCandidatesLocked(channels, groups, model)
	}
	channels = filterCachedChannelIDsByRetryConstraints(channels, usedChannelIDs, frozenMultiplier, requireSameMultiplier, frozenProfile, requireSameProfile)
	retry = retryIndexAfterRetryConstraints(retry, usedChannelIDs, requireSameMultiplier, requireSameProfile)
	return selectCachedChannelByPriority(channels, retry, model)
}

// intersectWithGroupCandidatesLocked 把 endpoint 过滤后的渠道集合再按分组候选集求交。
// groups 为空（无分组限制）时不过滤。调用者须持有 channelSyncLock。
func intersectWithGroupCandidatesLocked(channels []int, groups []string, model string) []int {
	if len(groups) == 0 {
		return channels
	}
	allowed := make(map[int]struct{})
	for _, id := range candidateChannelIDsForGroupsLocked(groups, model) {
		allowed[id] = struct{}{}
	}
	filtered := make([]int, 0, len(channels))
	for _, id := range channels {
		if _, ok := allowed[id]; ok {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func GetRandomSatisfiedChannelForEndpoint(group string, model string, retry int, endpointType constant.EndpointType) (*Channel, error) {
	if endpointType == "" {
		return GetRandomSatisfiedChannel(model, retry)
	}
	if !common.MemoryCacheEnabled {
		return GetChannelForEndpoint(group, model, retry, endpointType)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	channels, err := getCachedChannelIDsForEndpoint(model, endpointType)
	if err != nil {
		return nil, err
	}
	return selectCachedChannelByPriority(channels, retry, model)
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from model2channels
		for model, channels := range model2channels {
			for i, channelId := range channels {
				if channelId == id {
					model2channels[model] = append(channels[:i], channels[i+1:]...)
					break
				}
			}
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}

	println("CacheUpdateChannel:", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)

	println("before:", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
	channelsIDM[channel.Id] = channel
	println("after :", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
}
