package model

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/tokenbilling"
)

const (
	ChannelTokenEquivalentKindSingle    = "single"
	ChannelTokenEquivalentKindRange     = "range"
	ChannelTokenEquivalentKindUnlimited = "unlimited"
)

type PlanChannelTokenEquivalent struct {
	Kind                    string   `json:"kind"`
	ChannelType             int      `json:"channel_type"`
	ChannelTypeName         string   `json:"channel_type_name"`
	ChannelTypeLabelKey     string   `json:"channel_type_label_key,omitempty"`
	VariantCount            int      `json:"variant_count"`
	Multiplier              *float64 `json:"multiplier,omitempty"`
	MinMultiplier           *float64 `json:"min_multiplier,omitempty"`
	MaxMultiplier           *float64 `json:"max_multiplier,omitempty"`
	EquivalentTokenLimit    *int64   `json:"equivalent_token_limit,omitempty"`
	EquivalentTokenLimitMin *int64   `json:"equivalent_token_limit_min,omitempty"`
	EquivalentTokenLimitMax *int64   `json:"equivalent_token_limit_max,omitempty"`
	TokenUnlimited          bool     `json:"token_unlimited,omitempty"`
}

type SubscriptionChannelTokenEquivalent struct {
	Kind                        string   `json:"kind"`
	ChannelType                 int      `json:"channel_type"`
	ChannelTypeName             string   `json:"channel_type_name"`
	ChannelTypeLabelKey         string   `json:"channel_type_label_key,omitempty"`
	VariantCount                int      `json:"variant_count"`
	Multiplier                  *float64 `json:"multiplier,omitempty"`
	MinMultiplier               *float64 `json:"min_multiplier,omitempty"`
	MaxMultiplier               *float64 `json:"max_multiplier,omitempty"`
	EquivalentTokenLimit        *int64   `json:"equivalent_token_limit,omitempty"`
	EquivalentTokenLimitMin     *int64   `json:"equivalent_token_limit_min,omitempty"`
	EquivalentTokenLimitMax     *int64   `json:"equivalent_token_limit_max,omitempty"`
	EquivalentTokenRemaining    *int64   `json:"equivalent_token_remaining,omitempty"`
	EquivalentTokenRemainingMin *int64   `json:"equivalent_token_remaining_min,omitempty"`
	EquivalentTokenRemainingMax *int64   `json:"equivalent_token_remaining_max,omitempty"`
	TokenUnlimited              bool     `json:"token_unlimited,omitempty"`
}

type ChannelTokenBillingMultiplierGroup struct {
	ChannelType     int
	ChannelTypeName string
	VariantCount    int
	MinMultiplier   float64
	MaxMultiplier   float64
	multipliers     []float64
}

type channelTokenBillingMultiplierRow struct {
	Type                   int
	TokenBillingMultiplier float64
}

func ListEnabledChannelTokenBillingMultiplierGroups() ([]ChannelTokenBillingMultiplierGroup, error) {
	var rows []channelTokenBillingMultiplierRow
	if err := DB.Model(&Channel{}).
		Select("type", "token_billing_multiplier").
		Where("status = ?", common.ChannelStatusEnabled).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ChannelTokenBillingMultiplierGroup{}, nil
	}

	groupsByType := make(map[int]*ChannelTokenBillingMultiplierGroup)
	for _, row := range rows {
		multiplier := tokenbilling.EffectiveMultiplier(row.TokenBillingMultiplier)
		group := groupsByType[row.Type]
		if group == nil {
			group = &ChannelTokenBillingMultiplierGroup{
				ChannelType:     row.Type,
				ChannelTypeName: constant.GetChannelTypeName(row.Type),
				MinMultiplier:   multiplier,
				MaxMultiplier:   multiplier,
				multipliers:     []float64{multiplier},
			}
			groupsByType[row.Type] = group
			continue
		}
		if multiplier < group.MinMultiplier {
			group.MinMultiplier = multiplier
		}
		if multiplier > group.MaxMultiplier {
			group.MaxMultiplier = multiplier
		}
		if !containsSameChannelMultiplier(group.multipliers, multiplier) {
			group.multipliers = append(group.multipliers, multiplier)
		}
	}

	groups := make([]ChannelTokenBillingMultiplierGroup, 0, len(groupsByType))
	for _, group := range groupsByType {
		sort.Float64s(group.multipliers)
		group.VariantCount = len(group.multipliers)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].ChannelTypeName == groups[j].ChannelTypeName {
			return groups[i].ChannelType < groups[j].ChannelType
		}
		return groups[i].ChannelTypeName < groups[j].ChannelTypeName
	})
	return groups, nil
}

func BuildPlanChannelTokenEquivalents(standardTokens int64, groups []ChannelTokenBillingMultiplierGroup) ([]PlanChannelTokenEquivalent, error) {
	if len(groups) == 0 {
		return []PlanChannelTokenEquivalent{}, nil
	}
	equivalents := make([]PlanChannelTokenEquivalent, 0, len(groups))
	for _, group := range groups {
		base := PlanChannelTokenEquivalent{
			ChannelType:     group.ChannelType,
			ChannelTypeName: group.ChannelTypeName,
			VariantCount:    group.VariantCount,
		}
		if standardTokens <= 0 {
			base.Kind = ChannelTokenEquivalentKindUnlimited
			base.TokenUnlimited = true
			equivalents = append(equivalents, base)
			continue
		}
		if group.VariantCount <= 1 {
			eq, err := tokenbilling.EquivalentTokens(standardTokens, group.MinMultiplier)
			if err != nil {
				return nil, err
			}
			multiplier := group.MinMultiplier
			base.Kind = ChannelTokenEquivalentKindSingle
			base.Multiplier = &multiplier
			base.EquivalentTokenLimit = &eq
			equivalents = append(equivalents, base)
			continue
		}
		minLimit, err := tokenbilling.EquivalentTokens(standardTokens, group.MaxMultiplier)
		if err != nil {
			return nil, err
		}
		maxLimit, err := tokenbilling.EquivalentTokens(standardTokens, group.MinMultiplier)
		if err != nil {
			return nil, err
		}
		minMultiplier := group.MinMultiplier
		maxMultiplier := group.MaxMultiplier
		base.Kind = ChannelTokenEquivalentKindRange
		base.MinMultiplier = &minMultiplier
		base.MaxMultiplier = &maxMultiplier
		base.EquivalentTokenLimitMin = &minLimit
		base.EquivalentTokenLimitMax = &maxLimit
		equivalents = append(equivalents, base)
	}
	return equivalents, nil
}

func BuildSubscriptionChannelTokenEquivalents(tokenLimit int64, tokenUsed int64, tokenUnlimited bool, groups []ChannelTokenBillingMultiplierGroup) ([]SubscriptionChannelTokenEquivalent, error) {
	if len(groups) == 0 {
		return []SubscriptionChannelTokenEquivalent{}, nil
	}
	remaining := tokenLimit - tokenUsed
	if remaining < 0 {
		remaining = 0
	}
	equivalents := make([]SubscriptionChannelTokenEquivalent, 0, len(groups))
	for _, group := range groups {
		base := SubscriptionChannelTokenEquivalent{
			ChannelType:     group.ChannelType,
			ChannelTypeName: group.ChannelTypeName,
			VariantCount:    group.VariantCount,
		}
		if tokenUnlimited {
			base.Kind = ChannelTokenEquivalentKindUnlimited
			base.TokenUnlimited = true
			equivalents = append(equivalents, base)
			continue
		}
		if group.VariantCount <= 1 {
			limit, err := tokenbilling.EquivalentTokens(tokenLimit, group.MinMultiplier)
			if err != nil {
				return nil, err
			}
			remainingEquivalent, err := tokenbilling.EquivalentTokens(remaining, group.MinMultiplier)
			if err != nil {
				return nil, err
			}
			multiplier := group.MinMultiplier
			base.Kind = ChannelTokenEquivalentKindSingle
			base.Multiplier = &multiplier
			base.EquivalentTokenLimit = &limit
			base.EquivalentTokenRemaining = &remainingEquivalent
			equivalents = append(equivalents, base)
			continue
		}
		limitMin, err := tokenbilling.EquivalentTokens(tokenLimit, group.MaxMultiplier)
		if err != nil {
			return nil, err
		}
		limitMax, err := tokenbilling.EquivalentTokens(tokenLimit, group.MinMultiplier)
		if err != nil {
			return nil, err
		}
		remainingMin, err := tokenbilling.EquivalentTokens(remaining, group.MaxMultiplier)
		if err != nil {
			return nil, err
		}
		remainingMax, err := tokenbilling.EquivalentTokens(remaining, group.MinMultiplier)
		if err != nil {
			return nil, err
		}
		minMultiplier := group.MinMultiplier
		maxMultiplier := group.MaxMultiplier
		base.Kind = ChannelTokenEquivalentKindRange
		base.MinMultiplier = &minMultiplier
		base.MaxMultiplier = &maxMultiplier
		base.EquivalentTokenLimitMin = &limitMin
		base.EquivalentTokenLimitMax = &limitMax
		base.EquivalentTokenRemainingMin = &remainingMin
		base.EquivalentTokenRemainingMax = &remainingMax
		equivalents = append(equivalents, base)
	}
	return equivalents, nil
}

func PopulateSubscriptionPlanChannelTokenEquivalents(plan *SubscriptionPlan, groups []ChannelTokenBillingMultiplierGroup) error {
	if plan == nil {
		return nil
	}
	equivalents, err := BuildPlanChannelTokenEquivalents(plan.MonthlyTokenLimit, groups)
	if err != nil {
		return err
	}
	plan.ChannelTokenEquivalents = equivalents
	return nil
}

func PopulateSubscriptionSummaryPlanChannelTokenEquivalents(summaries []SubscriptionSummary, groups []ChannelTokenBillingMultiplierGroup) error {
	for i := range summaries {
		if summaries[i].Plan == nil {
			continue
		}
		if err := PopulateSubscriptionPlanChannelTokenEquivalents(summaries[i].Plan, groups); err != nil {
			return err
		}
	}
	return nil
}

func containsSameChannelMultiplier(values []float64, multiplier float64) bool {
	for _, value := range values {
		if tokenbilling.SameMultiplier(value, multiplier) {
			return true
		}
	}
	return false
}
