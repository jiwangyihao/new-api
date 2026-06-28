package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/creditbilling"
	"github.com/QuantumNous/new-api/pkg/tokenbilling"

	"github.com/samber/lo"
	"gorm.io/gorm"
)

// DefaultChannelGroupName 是固定存在的默认分组名。用户未为 API Key 选择分组时落到该分组。
// 该分组不可删除、不可改名、不可禁用；管理员可编辑其计费 profile 与成员。
const DefaultChannelGroupName = "__default__"

// DisabledTokenGroupSentinel 是一个保留分组名，绝不对应任何真实分组或渠道。
// 当 API Key 已绑定分组但所有绑定分组都被禁用时，GetEffectiveGroupNamesByToken 返回它，
// 使渠道选择在并集 / abilities 过滤中匹配到空集合（拒绝），从而不会回落到默认分组而扩大渠道范围。
// 它必须是非空集合（区别于“未绑定 = 回落默认分组”），但又不匹配任何渠道。
const DisabledTokenGroupSentinel = "__disabled__"

// GroupCreditBillingModeInherit 表示分组未配置计费方式（第三态，空串），结算时回落到具体渠道。
const GroupCreditBillingModeInherit = ""

// ChannelGroup 是渠道分组实体。管理员把若干渠道打包进一个分组；用户为 API Key 选择一个或多个分组。
// 用户只能看到分组（id/name/description），不能知道分组背后的真实上游渠道。
type ChannelGroup struct {
	Id          int    `json:"id"`
	Name        string `json:"name" gorm:"size:64;not null;uniqueIndex:uk_channel_group_name,where:deleted_at IS NULL"`
	Description string `json:"description" gorm:"type:varchar(255)"`
	Enabled     bool   `json:"enabled" gorm:"not null;default:true"`

	// 计费 profile。CreditBillingMode 为空串（inherit）表示不覆盖渠道计费方式，结算时回落渠道。
	CreditBillingMode               string  `json:"credit_billing_mode" gorm:"type:varchar(32);not null;default:''"`
	FixedRequestCredits             int64   `json:"fixed_request_credits" gorm:"not null;default:0"`
	DynamicBillingMultiplierEnabled bool    `json:"dynamic_billing_multiplier_enabled" gorm:"not null;default:false"`
	TokenBillingMultiplier          float64 `json:"token_billing_multiplier" gorm:"not null;default:1"`

	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// ChannelGroupChannel 是渠道↔分组多对多关联行。
type ChannelGroupChannel struct {
	ChannelGroupId int `json:"channel_group_id" gorm:"primaryKey;autoIncrement:false;index"`
	ChannelId      int `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
}

// TokenGroupBinding 是 API Key↔分组多对多绑定行。
type TokenGroupBinding struct {
	TokenId        int `json:"token_id" gorm:"primaryKey;autoIncrement:false;index"`
	ChannelGroupId int `json:"channel_group_id" gorm:"primaryKey;autoIncrement:false;index"`
}

// normalizeGroupCreditBillingMode 校验分组计费 mode，空串（inherit）保持为空串，绝不归一成 usage_tokens。
func normalizeGroupCreditBillingMode(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	switch mode {
	case GroupCreditBillingModeInherit:
		return GroupCreditBillingModeInherit, nil
	case creditbilling.ModeUsageTokens, creditbilling.ModeFixedRequest:
		return mode, nil
	default:
		return "", fmt.Errorf("channel group credit billing mode must be empty (inherit), %q, or %q", creditbilling.ModeUsageTokens, creditbilling.ModeFixedRequest)
	}
}

// IsDefault 报告该分组是否为固定默认分组。
func (g *ChannelGroup) IsDefault() bool {
	return g != nil && g.Name == DefaultChannelGroupName
}

// OverridesBilling 报告分组是否覆盖计费方式（非 inherit）。
func (g *ChannelGroup) OverridesBilling() bool {
	return g != nil && strings.TrimSpace(g.CreditBillingMode) != GroupCreditBillingModeInherit
}

// BillingProfile 返回分组自身的计费 profile（仅在 OverridesBilling() 为 true 时有意义）。
func (g *ChannelGroup) BillingProfile() ChannelBillingProfile {
	return ChannelBillingProfile{
		CreditBillingMode:               g.CreditBillingMode,
		FixedRequestCredits:             g.FixedRequestCredits,
		TokenBillingMultiplier:          g.TokenBillingMultiplier,
		DynamicBillingMultiplierEnabled: g.DynamicBillingMultiplierEnabled,
	}.Normalize()
}

// Validate 校验分组的计费 profile 合法性，并归一字段。
func (g *ChannelGroup) Validate() error {
	if g == nil {
		return errors.New("channel group cannot be empty")
	}
	mode, err := normalizeGroupCreditBillingMode(g.CreditBillingMode)
	if err != nil {
		return err
	}
	g.CreditBillingMode = mode
	if mode == GroupCreditBillingModeInherit {
		// inherit：分组不覆盖计费，profile 字段不参与，仅做非负保护。
		if g.FixedRequestCredits < 0 {
			return errors.New("fixed request credits must not be negative")
		}
		return nil
	}
	if err := creditbilling.ValidateBillingMode(mode); err != nil {
		return err
	}
	if err := creditbilling.ValidateFixedRequestCredits(mode, g.FixedRequestCredits); err != nil {
		return err
	}
	g.TokenBillingMultiplier = tokenbilling.EffectiveMultiplier(g.TokenBillingMultiplier)
	return nil
}

// ResolveEffectiveBillingProfile 解析生效计费 profile：
// 分组为 nil 或 inherit（未配置计费方式）时回落到选中渠道的计费 profile；否则用分组 profile 覆盖。
func ResolveEffectiveBillingProfile(group *ChannelGroup, channel *Channel) ChannelBillingProfile {
	if group == nil || !group.OverridesBilling() {
		if channel == nil {
			return DefaultChannelBillingProfile()
		}
		return channel.BillingProfile()
	}
	return group.BillingProfile()
}

// effectiveBillingProfileForChannel 在 token 所选分组语境下解析候选渠道的生效计费 profile。
// 用于 retry same-profile 过滤：必须按候选渠道的生效分组（而非渠道自身 profile）比较，
// 否则分组级覆盖计费时 failover 会因渠道 profile 不匹配而误排除同分组内的可用渠道。
func effectiveBillingProfileForChannel(groups []string, channel *Channel) (ChannelBillingProfile, error) {
	if channel == nil {
		return DefaultChannelBillingProfile(), nil
	}
	group, err := ResolveEffectiveGroupForChannel(groups, channel.Id)
	if err != nil {
		// 生效分组无法解析（如默认分组行缺失）等价于 inherit：回落渠道自身计费 profile，
		// 绝不因此让 retry same-profile 过滤报错而中断 failover。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return channel.BillingProfile(), nil
		}
		return ChannelBillingProfile{}, err
	}
	return ResolveEffectiveBillingProfile(group, channel), nil
}

// onlyDefaultGroupWithoutExplicitMembers 判断所选分组是否仅为默认分组且默认分组无显式成员。
// 此时默认分组语义为“全部渠道”，DB 选择路径必须跳过 group IN (?) 过滤——否则升级前已有的
// legacy ability 行（group 空串）匹配不到 "__default__"，未绑定分组的既有 API Key 在
// 非 memory-cache 部署下查不到任何渠道（升级回归）。
func onlyDefaultGroupWithoutExplicitMembers(groups []string) bool {
	if len(groups) != 1 || groups[0] != DefaultChannelGroupName {
		return false
	}
	hasExplicit, err := DefaultGroupHasExplicitMembers()
	if err != nil {
		return false
	}
	return !hasExplicit
}

// ---- CRUD ----

func (g *ChannelGroup) Insert() error {
	if err := g.Validate(); err != nil {
		return err
	}
	now := common.GetTimestamp()
	g.CreatedTime = now
	g.UpdatedTime = now
	return DB.Create(g).Error
}

func (g *ChannelGroup) Update() error {
	if err := g.Validate(); err != nil {
		return err
	}
	g.UpdatedTime = common.GetTimestamp()
	return DB.Model(g).Select(
		"name", "description", "enabled",
		"credit_billing_mode", "fixed_request_credits",
		"dynamic_billing_multiplier_enabled", "token_billing_multiplier",
		"updated_time",
	).Updates(g).Error
}

func IsChannelGroupNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&ChannelGroup{}).Where("name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

func DeleteChannelGroupByID(id int) error {
	group, err := GetChannelGroupByID(id)
	if err != nil {
		return err
	}
	if group.IsDefault() {
		return errors.New("default channel group cannot be deleted")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_group_id = ?", id).Delete(&ChannelGroupChannel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_group_id = ?", id).Delete(&TokenGroupBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&ChannelGroup{}, id).Error
	})
}

func GetAllChannelGroups() ([]*ChannelGroup, error) {
	var groups []*ChannelGroup
	if err := DB.Model(&ChannelGroup{}).Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func GetChannelGroupByID(id int) (*ChannelGroup, error) {
	var group ChannelGroup
	if err := DB.First(&group, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func GetChannelGroupByName(name string) (*ChannelGroup, error) {
	var group ChannelGroup
	if err := DB.First(&group, "name = ?", name).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

// GetChannelGroupsByIDs 按 id 列表批量加载分组。
func GetChannelGroupsByIDs(ids []int) ([]*ChannelGroup, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var groups []*ChannelGroup
	if err := DB.Where("id IN ?", ids).Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

// ---- 渠道成员 ----

// SetChannelGroupChannels 全量覆盖某分组的成员渠道集合。
func SetChannelGroupChannels(groupId int, channelIds []int) error {
	unique := lo.Uniq(channelIds)
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_group_id = ?", groupId).Delete(&ChannelGroupChannel{}).Error; err != nil {
			return err
		}
		if len(unique) == 0 {
			return nil
		}
		rows := make([]ChannelGroupChannel, 0, len(unique))
		for _, cid := range unique {
			if cid <= 0 {
				continue
			}
			rows = append(rows, ChannelGroupChannel{ChannelGroupId: groupId, ChannelId: cid})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

// GetChannelIdsByGroup 返回分组的显式成员渠道 id。
func GetChannelIdsByGroup(groupId int) ([]int, error) {
	var ids []int
	err := DB.Model(&ChannelGroupChannel{}).Where("channel_group_id = ?", groupId).Pluck("channel_id", &ids).Error
	return ids, err
}

// GetGroupIdsByChannel 返回渠道所属的分组 id。
func GetGroupIdsByChannel(channelId int) ([]int, error) {
	var ids []int
	err := DB.Model(&ChannelGroupChannel{}).Where("channel_id = ?", channelId).Pluck("channel_group_id", &ids).Error
	return ids, err
}

// DefaultGroupHasExplicitMembers 报告默认分组是否被管理员配置了显式成员。
// 无显式成员时默认分组语义为“允许所有渠道”。
func DefaultGroupHasExplicitMembers() (bool, error) {
	group, err := GetChannelGroupByName(DefaultChannelGroupName)
	if err != nil {
		return false, err
	}
	var cnt int64
	if err := DB.Model(&ChannelGroupChannel{}).Where("channel_group_id = ?", group.Id).Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// GetGroupNamesByChannel 返回某渠道生效所属的分组名集合（用于重建 abilities）。
// 默认分组语义：默认分组无显式成员时所有渠道都属于它；有显式成员时按成员判断。
func GetGroupNamesByChannel(channelId int) ([]string, error) {
	var rows []ChannelGroupChannel
	if err := DB.Where("channel_id = ?", channelId).Find(&rows).Error; err != nil {
		return nil, err
	}
	groupIds := lo.Map(rows, func(r ChannelGroupChannel, _ int) int { return r.ChannelGroupId })
	groups, err := GetChannelGroupsByIDs(groupIds)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(groups)+1)
	for _, g := range groups {
		names = append(names, g.Name)
	}
	// 默认分组：无显式成员时所有渠道隐式属于它。
	hasExplicit, err := DefaultGroupHasExplicitMembers()
	if err != nil {
		return nil, err
	}
	if !hasExplicit && !lo.Contains(names, DefaultChannelGroupName) {
		names = append(names, DefaultChannelGroupName)
	}
	return lo.Uniq(names), nil
}

// ---- token 绑定 ----

// SetTokenGroupBindings 全量覆盖某 API Key 绑定的分组集合。
func SetTokenGroupBindings(tokenId int, groupIds []int) error {
	unique := lo.Uniq(groupIds)
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("token_id = ?", tokenId).Delete(&TokenGroupBinding{}).Error; err != nil {
			return err
		}
		if len(unique) == 0 {
			return nil
		}
		rows := make([]TokenGroupBinding, 0, len(unique))
		for _, gid := range unique {
			if gid <= 0 {
				continue
			}
			rows = append(rows, TokenGroupBinding{TokenId: tokenId, ChannelGroupId: gid})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

// GetGroupIdsByToken 返回某 API Key 绑定的分组 id。
func GetGroupIdsByToken(tokenId int) ([]int, error) {
	var ids []int
	err := DB.Model(&TokenGroupBinding{}).Where("token_id = ?", tokenId).Pluck("channel_group_id", &ids).Error
	return ids, err
}

// GetEffectiveGroupNamesByToken 返回 API Key 生效的分组名集合（用于渠道选择）。
// 未绑定任何分组时回落默认分组。仅返回 enabled 分组；已绑定但全部被禁用时返回禁用哨兵分组（拒绝），绝不回落默认分组以免扩大渠道范围。
func GetEffectiveGroupNamesByToken(tokenId int) ([]string, error) {
	ids, err := GetGroupIdsByToken(tokenId)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []string{DefaultChannelGroupName}, nil
	}
	groups, err := GetChannelGroupsByIDs(ids)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		if g.Enabled {
			names = append(names, g.Name)
		}
	}
	if len(names) == 0 {
		// 已绑定分组但全部被禁用：返回禁用哨兵（非空但不匹配任何渠道），渠道选择据此拒绝服务，
		// 绝不回落默认分组以免把范围扩大到全部渠道。
		return []string{DisabledTokenGroupSentinel}, nil
	}
	return lo.Uniq(names), nil
}

// ensureDefaultChannelGroup 保证固定默认分组存在。迁移后调用。
func ensureDefaultChannelGroup() error {
	var cnt int64
	if err := DB.Model(&ChannelGroup{}).Where("name = ?", DefaultChannelGroupName).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	now := common.GetTimestamp()
	group := ChannelGroup{
		Name:              DefaultChannelGroupName,
		Description:       "默认分组：允许所有渠道（可由管理员配置）",
		Enabled:           true,
		CreditBillingMode: GroupCreditBillingModeInherit,
		CreatedTime:       now,
		UpdatedTime:       now,
	}
	return DB.Create(&group).Error
}

// LoadChannelGroupMembership 批量加载渠道↔分组名映射，用于缓存重建。
// 返回 channelId -> 显式所属分组名集合，以及默认分组是否有显式成员。
func LoadChannelGroupMembership() (map[int][]string, bool, error) {
	var rows []ChannelGroupChannel
	if err := DB.Find(&rows).Error; err != nil {
		return nil, false, err
	}
	groups, err := GetAllChannelGroups()
	if err != nil {
		return nil, false, err
	}
	idToName := make(map[int]string, len(groups))
	defaultGroupId := 0
	for _, g := range groups {
		idToName[g.Id] = g.Name
		if g.Name == DefaultChannelGroupName {
			defaultGroupId = g.Id
		}
	}
	membership := make(map[int][]string)
	defaultHasExplicit := false
	for _, r := range rows {
		name, ok := idToName[r.ChannelGroupId]
		if !ok {
			continue
		}
		if defaultGroupId != 0 && r.ChannelGroupId == defaultGroupId {
			defaultHasExplicit = true
		}
		membership[r.ChannelId] = append(membership[r.ChannelId], name)
	}
	return membership, defaultHasExplicit, nil
}

// GetTokenGroupView 返回某 API Key 绑定分组的脱敏视图（ids + names），绝不含渠道信息。
func GetTokenGroupView(tokenId int) ([]int, []string, error) {
	ids, err := GetGroupIdsByToken(tokenId)
	if err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		return []int{}, []string{}, nil
	}
	groups, err := GetChannelGroupsByIDs(ids)
	if err != nil {
		return nil, nil, err
	}
	outIds := make([]int, 0, len(groups))
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		outIds = append(outIds, g.Id)
		names = append(names, g.Name)
	}
	return outIds, names, nil
}

// ValidateChannelGroupIds 确认所有 id 均指向存在的分组。
func ValidateChannelGroupIds(ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	groups, err := GetChannelGroupsByIDs(ids)
	if err != nil {
		return err
	}
	if len(groups) != len(lo.Uniq(ids)) {
		return errors.New("one or more channel group ids do not exist")
	}
	return nil
}

// ResolveEffectiveGroupForChannel 在 API Key 所选分组中，找出选中渠道实际命中的“生效分组”。
// 规则：生效分组 = 渠道所属 ∩ token 所选分组；优先非默认分组，再按 id 升序；都不匹配则回落默认分组。
// 用于决定本次请求使用哪个分组的计费 profile（inherit 时回落渠道）。
func ResolveEffectiveGroupForChannel(tokenGroupNames []string, channelId int) (*ChannelGroup, error) {
	if len(tokenGroupNames) == 0 {
		tokenGroupNames = []string{DefaultChannelGroupName}
	}
	// 渠道生效所属分组名（含默认分组特判）。
	channelGroupNames, err := GetGroupNamesByChannel(channelId)
	if err != nil {
		return nil, err
	}
	channelSet := make(map[string]struct{}, len(channelGroupNames))
	for _, n := range channelGroupNames {
		channelSet[n] = struct{}{}
	}
	// token 所选 ∩ 渠道所属。
	candidates := make([]string, 0, len(tokenGroupNames))
	for _, n := range tokenGroupNames {
		if _, ok := channelSet[n]; ok {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		// 渠道不在 token 任何分组内（理论上不应发生，因为选择已过滤）；回落默认分组。
		candidates = []string{DefaultChannelGroupName}
	}
	groups, err := loadChannelGroupsByNames(candidates)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return GetChannelGroupByName(DefaultChannelGroupName)
	}
	// 优先非默认分组，再按 id 升序。
	var chosen *ChannelGroup
	for _, g := range groups {
		if g.IsDefault() {
			continue
		}
		if chosen == nil || g.Id < chosen.Id {
			chosen = g
		}
	}
	if chosen != nil {
		return chosen, nil
	}
	return GetChannelGroupByName(DefaultChannelGroupName)
}

func loadChannelGroupsByNames(names []string) ([]*ChannelGroup, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var groups []*ChannelGroup
	if err := DB.Where("name IN ?", names).Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}
