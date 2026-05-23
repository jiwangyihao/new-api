package model

import (
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

type endpointMetaCache struct {
	exact    map[string]*Model
	prefix   []*Model
	contains []*Model
	suffix   []*Model
}

var (
	endpointSupportLock  sync.RWMutex
	endpointSupportCache endpointMetaCache
	endpointSupportReady bool
)

func RefreshEndpointSupportCache() {
	var models []Model
	_ = DB.Find(&models).Error

	cache := endpointMetaCache{exact: make(map[string]*Model)}
	for i := range models {
		m := &models[i]
		switch m.NameRule {
		case NameRuleExact:
			cache.exact[m.ModelName] = m
		case NameRulePrefix:
			cache.prefix = append(cache.prefix, m)
		case NameRuleSuffix:
			cache.suffix = append(cache.suffix, m)
		case NameRuleContains:
			cache.contains = append(cache.contains, m)
		}
	}

	endpointSupportLock.Lock()
	endpointSupportCache = cache
	endpointSupportReady = true
	endpointSupportLock.Unlock()
}

func getEndpointSupportCache() endpointMetaCache {
	endpointSupportLock.RLock()
	ready := endpointSupportReady
	cache := endpointSupportCache
	endpointSupportLock.RUnlock()
	if ready {
		return cache
	}
	RefreshEndpointSupportCache()
	endpointSupportLock.RLock()
	cache = endpointSupportCache
	endpointSupportLock.RUnlock()
	return cache
}

func matchEndpointMeta(modelName string, cache endpointMetaCache) *Model {
	if meta, ok := cache.exact[modelName]; ok {
		return meta
	}
	for _, meta := range cache.prefix {
		if strings.HasPrefix(modelName, meta.ModelName) {
			return meta
		}
	}
	for _, meta := range cache.suffix {
		if strings.HasSuffix(modelName, meta.ModelName) {
			return meta
		}
	}
	for _, meta := range cache.contains {
		if strings.Contains(modelName, meta.ModelName) {
			return meta
		}
	}
	return nil
}

func endpointTypesFromOverride(endpoints string) []constant.EndpointType {
	if strings.TrimSpace(endpoints) == "" {
		return nil
	}
	var raw map[string]any
	if err := common.Unmarshal([]byte(endpoints), &raw); err != nil {
		return nil
	}
	out := make([]constant.EndpointType, 0, len(raw))
	for key, value := range raw {
		switch value.(type) {
		case string, map[string]any:
			out = append(out, constant.EndpointType(key))
		}
	}
	return out
}

func GetEffectiveEndpointTypes(channelType int, modelName string) []constant.EndpointType {
	cache := getEndpointSupportCache()
	if meta := matchEndpointMeta(modelName, cache); meta != nil {
		if meta.Status != 1 {
			return []constant.EndpointType{}
		}
		if endpoints := endpointTypesFromOverride(meta.Endpoints); len(endpoints) > 0 {
			return endpoints
		}
	}
	return common.GetEndpointTypesByChannelType(channelType, modelName)
}

func GetEffectiveEndpointTypesForModel(modelName string) []constant.EndpointType {
	if modelName == "" {
		return []constant.EndpointType{}
	}
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ? and abilities.model = ?", true, modelName).
		Scan(&abilities).Error
	if err != nil {
		return []constant.EndpointType{}
	}
	endpoints := make([]constant.EndpointType, 0)
	seen := make(map[constant.EndpointType]struct{})
	for _, ability := range abilities {
		for _, endpoint := range GetEffectiveEndpointTypes(ability.ChannelType, ability.Model) {
			if _, ok := seen[endpoint]; ok {
				continue
			}
			seen[endpoint] = struct{}{}
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
}

func ChannelSupportsEndpoint(channel *Channel, modelName string, endpointType constant.EndpointType) bool {
	if channel == nil {
		return false
	}
	for _, endpoint := range GetEffectiveEndpointTypes(channel.Type, modelName) {
		if endpoint == endpointType {
			return true
		}
	}
	return false
}

func GetModelSupportEndpointTypesForGroups(modelName string, usableGroups map[string]string) []constant.EndpointType {
	if modelName == "" || len(usableGroups) == 0 {
		return []constant.EndpointType{}
	}
	var abilities []Ability
	if err := DB.Where("model = ? and enabled = ?", modelName, true).Find(&abilities).Error; err != nil {
		return []constant.EndpointType{}
	}
	endpoints := make([]constant.EndpointType, 0)
	seen := make(map[constant.EndpointType]struct{})
	for _, ability := range abilities {
		if ability.Group != "all" {
			if _, ok := usableGroups[ability.Group]; !ok {
				continue
			}
		}
		channel := Channel{}
		if err := DB.First(&channel, "id = ?", ability.ChannelId).Error; err != nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		for _, endpoint := range GetEffectiveEndpointTypes(channel.Type, modelName) {
			if _, ok := seen[endpoint]; ok {
				continue
			}
			seen[endpoint] = struct{}{}
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
}
