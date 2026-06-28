package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/tokenbilling"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const legacyAbilityGroup = ""

type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`
}

type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"`
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(group string) []string {
	if group == "" {
		return GetEnabledModels()
	}
	var models []string
	DB.Table("abilities").Where("enabled = ? and `group` = ?", true, group).Distinct("model").Pluck("model", &models)
	return models
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

func getPriority(model string, retry int) (int, error) {
	var priorities []int
	err := DB.Model(&Ability{}).
		Select("DISTINCT(priority)").
		Where("model = ? and enabled = ?", model, true).
		Order("priority DESC").
		Pluck("priority", &priorities).Error
	if err != nil {
		return 0, err
	}
	if len(priorities) == 0 {
		return 0, errors.New("数据库一致性被破坏")
	}
	if retry >= len(priorities) {
		return priorities[len(priorities)-1], nil
	}
	return priorities[retry], nil
}

func getChannelQuery(model string, retry int) (*gorm.DB, error) {
	maxPrioritySubQuery := DB.Model(&Ability{}).Select("MAX(priority)").Where("model = ? and enabled = ?", model, true)
	channelQuery := DB.Where("model = ? and enabled = ? and priority = (?)", model, true, maxPrioritySubQuery)
	if retry != 0 {
		priority, err := getPriority(model, retry)
		if err != nil {
			return nil, err
		}
		channelQuery = DB.Where("model = ? and enabled = ? and priority = ?", model, true, priority)
	}
	return channelQuery, nil
}

func GetChannel(model string, retry int) (*Channel, error) {
	var abilities []Ability
	channelQuery, err := getChannelQuery(model, retry)
	if err != nil {
		return nil, err
	}
	err = channelQuery.Order("weight DESC").Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	abilities = uniqueAbilitiesByChannelID(abilities)
	if len(abilities) == 0 {
		return nil, nil
	}
	channel := Channel{}
	weightSum := uint(0)
	for _, ability := range abilities {
		weightSum += ability.Weight + 10
	}
	weight := common.GetRandomInt(int(weightSum))
	for _, ability := range abilities {
		weight -= int(ability.Weight) + 10
		if weight <= 0 {
			channel.Id = ability.ChannelId
			break
		}
	}
	err = DB.First(&channel, "id = ?", channel.Id).Error
	return &channel, err
}

func filterAbilitiesByRetryConstraints(abilities []Ability, groups []string, usedChannelIDs []int, frozenMultiplier float64, requireSameMultiplier bool, frozenProfile ChannelBillingProfile, requireSameProfile bool) ([]Ability, error) {
	if len(abilities) == 0 {
		return abilities, nil
	}
	used := channelIDSet(usedChannelIDs)
	filtered := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		if _, ok := used[ability.ChannelId]; ok {
			continue
		}
		if requireSameMultiplier || requireSameProfile {
			channel := Channel{}
			if err := DB.First(&channel, "id = ?", ability.ChannelId).Error; err != nil {
				return nil, err
			}
			if requireSameMultiplier && !tokenbilling.SameMultiplier(channel.GetTokenBillingMultiplier(), tokenbilling.EffectiveMultiplier(frozenMultiplier)) {
				continue
			}
			effectiveProfile, perr := effectiveBillingProfileForChannel(groups, &channel)
			if perr != nil {
				return nil, perr
			}
			if requireSameProfile && !SameChannelBillingProfile(effectiveProfile, frozenProfile) {
				continue
			}
		}
		filtered = append(filtered, ability)
	}
	return filtered, nil
}

func GetChannelForEndpoint(group string, model string, retry int, endpointType constant.EndpointType) (*Channel, error) {
	if endpointType == "" {
		return GetChannel(model, retry)
	}
	abilities, err := getEndpointFilteredAbilities(model, endpointType)
	if err != nil {
		return nil, err
	}
	if len(abilities) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		if normalizedModel != "" && normalizedModel != model {
			abilities, err = getEndpointFilteredAbilities(normalizedModel, endpointType)
			if err != nil {
				return nil, err
			}
		}
	}
	return selectChannelFromEndpointFilteredAbilities(abilities, retry)
}

func GetChannelForEndpointWithRetryConstraints(group string, model string, retry int, endpointType constant.EndpointType, usedChannelIDs []int, frozenMultiplier float64, requireSameMultiplier bool, frozenProfile ChannelBillingProfile, requireSameProfile bool) (*Channel, error) {
	var groups []string
	if group != "" {
		groups = []string{group}
	}
	return GetChannelForEndpointWithGroups(groups, model, retry, endpointType, usedChannelIDs, frozenMultiplier, requireSameMultiplier, frozenProfile, requireSameProfile)
}

// GetChannelForEndpointWithGroups 是 DB 路径的分组并集选择。groups 为空时不按分组过滤（全部渠道）。
// abilities 已按分组名编码（含默认分组特判写入的行），故直接以 abilities.group IN (?) 过滤即可。
func GetChannelForEndpointWithGroups(groups []string, model string, retry int, endpointType constant.EndpointType, usedChannelIDs []int, frozenMultiplier float64, requireSameMultiplier bool, frozenProfile ChannelBillingProfile, requireSameProfile bool) (*Channel, error) {
	var abilities []Ability
	var err error
	groupFilter := func(tx *gorm.DB) *gorm.DB {
		if len(groups) == 0 {
			return tx
		}
		// 默认分组无显式成员时语义为“全部渠道”；此时既有 legacy ability 行（group 空串）
		// 不会匹配 group IN ("__default__")，必须跳过分组过滤，否则未绑定分组的既有 API Key
		// 在非 memory-cache 部署下查不到任何渠道（升级回归）。
		if onlyDefaultGroupWithoutExplicitMembers(groups) {
			return tx
		}
		return tx.Where("`group` IN ?", groups)
	}
	if endpointType == "" {
		query := groupFilter(DB.Where("model = ? and enabled = ?", model, true))
		if err = query.Find(&abilities).Error; err != nil {
			return nil, err
		}
		abilities = uniqueAbilitiesByChannelID(abilities)
	} else {
		abilities, err = getEndpointFilteredAbilitiesForGroups(groups, model, endpointType)
		if err != nil {
			return nil, err
		}
		if len(abilities) == 0 {
			normalizedModel := ratio_setting.FormatMatchingModelName(model)
			if normalizedModel != "" && normalizedModel != model {
				abilities, err = getEndpointFilteredAbilitiesForGroups(groups, normalizedModel, endpointType)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	abilities, err = filterAbilitiesByRetryConstraints(abilities, groups, usedChannelIDs, frozenMultiplier, requireSameMultiplier, frozenProfile, requireSameProfile)
	if err != nil {
		return nil, err
	}
	retry = retryIndexAfterRetryConstraints(retry, usedChannelIDs, requireSameMultiplier, requireSameProfile)
	return selectChannelFromEndpointFilteredAbilities(abilities, retry)
}

func getEndpointFilteredAbilities(model string, endpointType constant.EndpointType) ([]Ability, error) {
	return getEndpointFilteredAbilitiesForGroups(nil, model, endpointType)
}

func getEndpointFilteredAbilitiesForGroups(groups []string, model string, endpointType constant.EndpointType) ([]Ability, error) {
	var abilities []Ability
	query := DB.Where("model = ? and enabled = ?", model, true)
	if len(groups) > 0 {
		query = query.Where("`group` IN ?", groups)
	}
	if err := query.Find(&abilities).Error; err != nil {
		return nil, err
	}
	abilities = uniqueAbilitiesByChannelID(abilities)
	filtered := abilities[:0]
	for _, ability := range abilities {
		channel := Channel{}
		if err := DB.First(&channel, "id = ?", ability.ChannelId).Error; err != nil {
			return nil, err
		}
		if channel.Status == common.ChannelStatusEnabled && ChannelSupportsEndpoint(&channel, model, endpointType) {
			filtered = append(filtered, ability)
		}
	}
	return filtered, nil
}

func selectChannelFromEndpointFilteredAbilities(abilities []Ability, retry int) (*Channel, error) {
	if len(abilities) == 0 {
		return nil, nil
	}
	prioritySet := map[int64]struct{}{}
	for _, ability := range abilities {
		priority := int64(0)
		if ability.Priority != nil {
			priority = *ability.Priority
		}
		prioritySet[priority] = struct{}{}
	}
	priorities := make([]int64, 0, len(prioritySet))
	for priority := range prioritySet {
		priorities = append(priorities, priority)
	}
	sort.Slice(priorities, func(i, j int) bool { return priorities[i] > priorities[j] })
	if retry >= len(priorities) {
		retry = len(priorities) - 1
	}
	targetPriority := priorities[retry]
	targetAbilities := abilities[:0]
	for _, ability := range abilities {
		priority := int64(0)
		if ability.Priority != nil {
			priority = *ability.Priority
		}
		if priority == targetPriority {
			targetAbilities = append(targetAbilities, ability)
		}
	}
	weightSum := uint(0)
	for _, ability := range targetAbilities {
		weightSum += ability.Weight + 10
	}
	weight := common.GetRandomInt(int(weightSum))
	selectedID := 0
	for _, ability := range targetAbilities {
		weight -= int(ability.Weight) + 10
		if weight <= 0 {
			selectedID = ability.ChannelId
			break
		}
	}
	if selectedID == 0 {
		return nil, nil
	}
	channel := Channel{}
	err := DB.First(&channel, "id = ?", selectedID).Error
	return &channel, err
}

func uniqueAbilitiesByChannelID(abilities []Ability) []Ability {
	seen := make(map[int]struct{}, len(abilities))
	unique := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		if _, ok := seen[ability.ChannelId]; ok {
			continue
		}
		seen[ability.ChannelId] = struct{}{}
		unique = append(unique, ability)
	}
	return unique
}

// channelAbilityGroups 返回该渠道重建 abilities 时应写入的分组名集合。
// 复用 GetGroupNamesByChannel（含默认分组特判）；无任何分组时回落 legacy 空串，保证升级前行为。
func (channel *Channel) channelAbilityGroups() []string {
	names, err := GetGroupNamesByChannel(channel.Id)
	if err != nil || len(names) == 0 {
		return []string{legacyAbilityGroup}
	}
	return names
}

// buildChannelAbilities 为该渠道按 (分组 × 模型) 生成 ability 行。
func (channel *Channel) buildChannelAbilities(groups []string) []Ability {
	models_ := strings.Split(channel.Models, ",")
	seenModels := make(map[string]struct{}, len(models_))
	uniqueModels := make([]string, 0, len(models_))
	for _, model := range models_ {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, exists := seenModels[model]; exists {
			continue
		}
		seenModels[model] = struct{}{}
		uniqueModels = append(uniqueModels, model)
	}
	enabled := channel.Status == common.ChannelStatusEnabled
	abilities := make([]Ability, 0, len(uniqueModels)*len(groups))
	for _, group := range groups {
		for _, model := range uniqueModels {
			abilities = append(abilities, Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   enabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			})
		}
	}
	return abilities
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	abilities := channel.buildChannelAbilities(channel.channelAbilityGroups())
	if len(abilities) == 0 {
		return nil
	}
	// choose DB or provided tx
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities (per group × model)
	abilities := channel.buildChannelAbilities(channel.channelAbilityGroups())
	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		return tx.Commit().Error
	}

	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityStatusByTag(tag string, status bool) error {
	return DB.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	ability := Ability{}
	if newTag != nil {
		ability.Tag = newTag
	}
	if priority != nil {
		ability.Priority = priority
	}
	if weight != nil {
		ability.Weight = *weight
	}
	return DB.Model(&Ability{}).Where("tag = ?", tag).Updates(ability).Error
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingSQLite {
		err := DB.Exec("DELETE FROM abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(fmt.Sprintf("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}
