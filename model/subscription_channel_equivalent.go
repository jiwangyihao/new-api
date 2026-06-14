package model

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/creditbilling"
	"github.com/QuantumNous/new-api/pkg/tokenbilling"
)

const (
	ChannelCreditEquivalentKindUsageTokens  = creditbilling.ModeUsageTokens
	ChannelCreditEquivalentKindFixedRequest = creditbilling.ModeFixedRequest
	ChannelCreditEquivalentKindUnlimited    = "unlimited"

	ChannelCreditEquivalentValueTypeSingle    = "single"
	ChannelCreditEquivalentValueTypeRange     = "range"
	ChannelCreditEquivalentValueTypeUnlimited = "unlimited"

	// Compatibility constants for older tests and integrations. New responses use
	// kind + value_type through channel_credit_equivalents.
	ChannelTokenEquivalentKindSingle    = ChannelCreditEquivalentValueTypeSingle
	ChannelTokenEquivalentKindRange     = ChannelCreditEquivalentValueTypeRange
	ChannelTokenEquivalentKindUnlimited = ChannelCreditEquivalentKindUnlimited
)

type PlanChannelCreditEquivalent struct {
	Kind                      string   `json:"kind"`
	ValueType                 string   `json:"value_type"`
	ChannelType               int      `json:"channel_type"`
	ChannelTypeName           string   `json:"channel_type_name"`
	ChannelTypeLabelKey       string   `json:"channel_type_label_key,omitempty"`
	VariantCount              int      `json:"variant_count"`
	Multiplier                *float64 `json:"multiplier,omitempty"`
	MinMultiplier             *float64 `json:"min_multiplier,omitempty"`
	MaxMultiplier             *float64 `json:"max_multiplier,omitempty"`
	EquivalentTokenLimit      *int64   `json:"equivalent_token_limit,omitempty"`
	EquivalentTokenLimitMin   *int64   `json:"equivalent_token_limit_min,omitempty"`
	EquivalentTokenLimitMax   *int64   `json:"equivalent_token_limit_max,omitempty"`
	FixedRequestCredits       *int64   `json:"fixed_request_credits,omitempty"`
	FixedRequestCreditsMin    *int64   `json:"fixed_request_credits_min,omitempty"`
	FixedRequestCreditsMax    *int64   `json:"fixed_request_credits_max,omitempty"`
	EquivalentRequestLimit    *int64   `json:"equivalent_request_limit,omitempty"`
	EquivalentRequestLimitMin *int64   `json:"equivalent_request_limit_min,omitempty"`
	EquivalentRequestLimitMax *int64   `json:"equivalent_request_limit_max,omitempty"`
	CreditUnlimited           bool     `json:"credit_unlimited,omitempty"`
	TokenUnlimited            bool     `json:"token_unlimited,omitempty"`
}

type SubscriptionChannelCreditEquivalent struct {
	Kind                          string   `json:"kind"`
	ValueType                     string   `json:"value_type"`
	ChannelType                   int      `json:"channel_type"`
	ChannelTypeName               string   `json:"channel_type_name"`
	ChannelTypeLabelKey           string   `json:"channel_type_label_key,omitempty"`
	VariantCount                  int      `json:"variant_count"`
	Multiplier                    *float64 `json:"multiplier,omitempty"`
	MinMultiplier                 *float64 `json:"min_multiplier,omitempty"`
	MaxMultiplier                 *float64 `json:"max_multiplier,omitempty"`
	EquivalentTokenLimit          *int64   `json:"equivalent_token_limit,omitempty"`
	EquivalentTokenLimitMin       *int64   `json:"equivalent_token_limit_min,omitempty"`
	EquivalentTokenLimitMax       *int64   `json:"equivalent_token_limit_max,omitempty"`
	EquivalentTokenRemaining      *int64   `json:"equivalent_token_remaining,omitempty"`
	EquivalentTokenRemainingMin   *int64   `json:"equivalent_token_remaining_min,omitempty"`
	EquivalentTokenRemainingMax   *int64   `json:"equivalent_token_remaining_max,omitempty"`
	FixedRequestCredits           *int64   `json:"fixed_request_credits,omitempty"`
	FixedRequestCreditsMin        *int64   `json:"fixed_request_credits_min,omitempty"`
	FixedRequestCreditsMax        *int64   `json:"fixed_request_credits_max,omitempty"`
	EquivalentRequestLimit        *int64   `json:"equivalent_request_limit,omitempty"`
	EquivalentRequestLimitMin     *int64   `json:"equivalent_request_limit_min,omitempty"`
	EquivalentRequestLimitMax     *int64   `json:"equivalent_request_limit_max,omitempty"`
	EquivalentRequestRemaining    *int64   `json:"equivalent_request_remaining,omitempty"`
	EquivalentRequestRemainingMin *int64   `json:"equivalent_request_remaining_min,omitempty"`
	EquivalentRequestRemainingMax *int64   `json:"equivalent_request_remaining_max,omitempty"`
	CreditUnlimited               bool     `json:"credit_unlimited,omitempty"`
	TokenUnlimited                bool     `json:"token_unlimited,omitempty"`
}

// Compatibility aliases. New code should use the Credit names above.
type PlanChannelTokenEquivalent = PlanChannelCreditEquivalent
type SubscriptionChannelTokenEquivalent = SubscriptionChannelCreditEquivalent

type ChannelCreditBillingGroup struct {
	ChannelType     int
	ChannelTypeName string
	Kind            string
	VariantCount    int
	MinMultiplier   float64
	MaxMultiplier   float64
	MinFixedCredits int64
	MaxFixedCredits int64
	multipliers     []float64
	fixedCredits    []int64
}

// Compatibility alias for existing call sites; it now groups complete credit
// billing profiles rather than only token multipliers.
type ChannelTokenBillingMultiplierGroup = ChannelCreditBillingGroup

type channelCreditBillingRow struct {
	Type                   int
	TokenBillingMultiplier float64
	CreditBillingMode      string
	FixedRequestCredits    int64
}

func ListEnabledChannelCreditBillingGroups() ([]ChannelCreditBillingGroup, error) {
	var rows []channelCreditBillingRow
	if err := DB.Model(&Channel{}).
		Select("type", "token_billing_multiplier", "credit_billing_mode", "fixed_request_credits").
		Where("status = ?", common.ChannelStatusEnabled).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ChannelCreditBillingGroup{}, nil
	}

	groupsByKey := make(map[struct {
		channelType int
		kind        string
	}]*ChannelCreditBillingGroup)
	for _, row := range rows {
		kind := row.CreditBillingMode
		if kind == "" {
			kind = creditbilling.ModeUsageTokens
		}
		if err := creditbilling.ValidateBillingMode(kind); err != nil {
			continue
		}
		if err := creditbilling.ValidateFixedRequestCredits(kind, row.FixedRequestCredits); err != nil {
			continue
		}
		key := struct {
			channelType int
			kind        string
		}{channelType: row.Type, kind: kind}
		group := groupsByKey[key]
		if group == nil {
			group = &ChannelCreditBillingGroup{
				ChannelType:     row.Type,
				ChannelTypeName: constant.GetChannelTypeName(row.Type),
				Kind:            kind,
			}
			groupsByKey[key] = group
		}
		if kind == creditbilling.ModeFixedRequest {
			addFixedCreditsToGroup(group, row.FixedRequestCredits)
			continue
		}
		addMultiplierToGroup(group, tokenbilling.EffectiveMultiplier(row.TokenBillingMultiplier))
	}

	groups := make([]ChannelCreditBillingGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		if group.Kind == creditbilling.ModeFixedRequest {
			sort.Slice(group.fixedCredits, func(i, j int) bool { return group.fixedCredits[i] < group.fixedCredits[j] })
			group.VariantCount = len(group.fixedCredits)
		} else {
			sort.Float64s(group.multipliers)
			group.VariantCount = len(group.multipliers)
		}
		if group.VariantCount == 0 {
			continue
		}
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].ChannelTypeName == groups[j].ChannelTypeName {
			if groups[i].ChannelType == groups[j].ChannelType {
				return groups[i].Kind < groups[j].Kind
			}
			return groups[i].ChannelType < groups[j].ChannelType
		}
		return groups[i].ChannelTypeName < groups[j].ChannelTypeName
	})
	return groups, nil
}

func ListEnabledChannelTokenBillingMultiplierGroups() ([]ChannelTokenBillingMultiplierGroup, error) {
	return ListEnabledChannelCreditBillingGroups()
}

func BuildPlanChannelCreditEquivalents(standardCredits int64, groups []ChannelCreditBillingGroup) ([]PlanChannelCreditEquivalent, error) {
	if len(groups) == 0 {
		return []PlanChannelCreditEquivalent{}, nil
	}
	equivalents := make([]PlanChannelCreditEquivalent, 0, len(groups))
	for _, group := range groups {
		base := PlanChannelCreditEquivalent{
			Kind:            group.Kind,
			ChannelType:     group.ChannelType,
			ChannelTypeName: group.ChannelTypeName,
			VariantCount:    group.VariantCount,
		}
		if standardCredits <= 0 {
			base.Kind = ChannelCreditEquivalentKindUnlimited
			base.ValueType = ChannelCreditEquivalentValueTypeUnlimited
			base.CreditUnlimited = true
			base.TokenUnlimited = true
			equivalents = append(equivalents, base)
			continue
		}
		if group.Kind == creditbilling.ModeFixedRequest {
			equivalents = append(equivalents, buildPlanFixedRequestEquivalent(base, standardCredits, group))
			continue
		}
		eq, err := buildPlanUsageTokensEquivalent(base, standardCredits, group)
		if err != nil {
			return nil, err
		}
		equivalents = append(equivalents, eq)
	}
	return equivalents, nil
}

func BuildPlanChannelTokenEquivalents(standardTokens int64, groups []ChannelTokenBillingMultiplierGroup) ([]PlanChannelTokenEquivalent, error) {
	return BuildPlanChannelCreditEquivalents(standardTokens, groups)
}

func BuildSubscriptionChannelCreditEquivalents(creditLimit int64, creditUsed int64, creditUnlimited bool, groups []ChannelCreditBillingGroup) ([]SubscriptionChannelCreditEquivalent, error) {
	if len(groups) == 0 {
		return []SubscriptionChannelCreditEquivalent{}, nil
	}
	remaining := creditLimit - creditUsed
	if remaining < 0 {
		remaining = 0
	}
	equivalents := make([]SubscriptionChannelCreditEquivalent, 0, len(groups))
	for _, group := range groups {
		base := SubscriptionChannelCreditEquivalent{
			Kind:            group.Kind,
			ChannelType:     group.ChannelType,
			ChannelTypeName: group.ChannelTypeName,
			VariantCount:    group.VariantCount,
		}
		if creditUnlimited {
			base.Kind = ChannelCreditEquivalentKindUnlimited
			base.ValueType = ChannelCreditEquivalentValueTypeUnlimited
			base.CreditUnlimited = true
			base.TokenUnlimited = true
			equivalents = append(equivalents, base)
			continue
		}
		if group.Kind == creditbilling.ModeFixedRequest {
			equivalents = append(equivalents, buildSubscriptionFixedRequestEquivalent(base, creditLimit, remaining, group))
			continue
		}
		eq, err := buildSubscriptionUsageTokensEquivalent(base, creditLimit, remaining, group)
		if err != nil {
			return nil, err
		}
		equivalents = append(equivalents, eq)
	}
	return equivalents, nil
}

func BuildSubscriptionChannelTokenEquivalents(tokenLimit int64, tokenUsed int64, tokenUnlimited bool, groups []ChannelTokenBillingMultiplierGroup) ([]SubscriptionChannelTokenEquivalent, error) {
	return BuildSubscriptionChannelCreditEquivalents(tokenLimit, tokenUsed, tokenUnlimited, groups)
}

func PopulateSubscriptionPlanChannelCreditEquivalents(plan *SubscriptionPlan, groups []ChannelCreditBillingGroup) error {
	if plan == nil {
		return nil
	}
	equivalents, err := BuildPlanChannelCreditEquivalents(plan.MonthlyTokenLimit, groups)
	if err != nil {
		return err
	}
	plan.ChannelCreditEquivalents = equivalents
	plan.ChannelTokenEquivalents = equivalents
	return nil
}

func PopulateSubscriptionPlanChannelTokenEquivalents(plan *SubscriptionPlan, groups []ChannelTokenBillingMultiplierGroup) error {
	return PopulateSubscriptionPlanChannelCreditEquivalents(plan, groups)
}

func PopulateSubscriptionSummaryPlanChannelCreditEquivalents(summaries []SubscriptionSummary, groups []ChannelCreditBillingGroup) error {
	for i := range summaries {
		if summaries[i].Plan == nil {
			continue
		}
		if err := PopulateSubscriptionPlanChannelCreditEquivalents(summaries[i].Plan, groups); err != nil {
			return err
		}
	}
	return nil
}

func PopulateSubscriptionSummaryPlanChannelTokenEquivalents(summaries []SubscriptionSummary, groups []ChannelTokenBillingMultiplierGroup) error {
	return PopulateSubscriptionSummaryPlanChannelCreditEquivalents(summaries, groups)
}

func buildPlanUsageTokensEquivalent(base PlanChannelCreditEquivalent, standardCredits int64, group ChannelCreditBillingGroup) (PlanChannelCreditEquivalent, error) {
	base.Kind = ChannelCreditEquivalentKindUsageTokens
	if group.VariantCount <= 1 {
		eq, err := tokenbilling.EquivalentTokens(standardCredits, group.MinMultiplier)
		if err != nil {
			return PlanChannelCreditEquivalent{}, err
		}
		multiplier := group.MinMultiplier
		base.ValueType = ChannelCreditEquivalentValueTypeSingle
		base.Multiplier = &multiplier
		base.EquivalentTokenLimit = &eq
		return base, nil
	}
	minLimit, err := tokenbilling.EquivalentTokens(standardCredits, group.MaxMultiplier)
	if err != nil {
		return PlanChannelCreditEquivalent{}, err
	}
	maxLimit, err := tokenbilling.EquivalentTokens(standardCredits, group.MinMultiplier)
	if err != nil {
		return PlanChannelCreditEquivalent{}, err
	}
	minMultiplier := group.MinMultiplier
	maxMultiplier := group.MaxMultiplier
	base.ValueType = ChannelCreditEquivalentValueTypeRange
	base.MinMultiplier = &minMultiplier
	base.MaxMultiplier = &maxMultiplier
	base.EquivalentTokenLimitMin = &minLimit
	base.EquivalentTokenLimitMax = &maxLimit
	return base, nil
}

func buildPlanFixedRequestEquivalent(base PlanChannelCreditEquivalent, standardCredits int64, group ChannelCreditBillingGroup) PlanChannelCreditEquivalent {
	base.Kind = ChannelCreditEquivalentKindFixedRequest
	if group.VariantCount <= 1 {
		credits := group.MinFixedCredits
		limit := equivalentRequests(standardCredits, credits)
		base.ValueType = ChannelCreditEquivalentValueTypeSingle
		base.FixedRequestCredits = &credits
		base.EquivalentRequestLimit = &limit
		return base
	}
	minCredits := group.MinFixedCredits
	maxCredits := group.MaxFixedCredits
	minLimit := equivalentRequests(standardCredits, maxCredits)
	maxLimit := equivalentRequests(standardCredits, minCredits)
	base.ValueType = ChannelCreditEquivalentValueTypeRange
	base.FixedRequestCreditsMin = &minCredits
	base.FixedRequestCreditsMax = &maxCredits
	base.EquivalentRequestLimitMin = &minLimit
	base.EquivalentRequestLimitMax = &maxLimit
	return base
}

func buildSubscriptionUsageTokensEquivalent(base SubscriptionChannelCreditEquivalent, creditLimit int64, remaining int64, group ChannelCreditBillingGroup) (SubscriptionChannelCreditEquivalent, error) {
	base.Kind = ChannelCreditEquivalentKindUsageTokens
	if group.VariantCount <= 1 {
		limit, err := tokenbilling.EquivalentTokens(creditLimit, group.MinMultiplier)
		if err != nil {
			return SubscriptionChannelCreditEquivalent{}, err
		}
		remainingEquivalent, err := tokenbilling.EquivalentTokens(remaining, group.MinMultiplier)
		if err != nil {
			return SubscriptionChannelCreditEquivalent{}, err
		}
		multiplier := group.MinMultiplier
		base.ValueType = ChannelCreditEquivalentValueTypeSingle
		base.Multiplier = &multiplier
		base.EquivalentTokenLimit = &limit
		base.EquivalentTokenRemaining = &remainingEquivalent
		return base, nil
	}
	limitMin, err := tokenbilling.EquivalentTokens(creditLimit, group.MaxMultiplier)
	if err != nil {
		return SubscriptionChannelCreditEquivalent{}, err
	}
	limitMax, err := tokenbilling.EquivalentTokens(creditLimit, group.MinMultiplier)
	if err != nil {
		return SubscriptionChannelCreditEquivalent{}, err
	}
	remainingMin, err := tokenbilling.EquivalentTokens(remaining, group.MaxMultiplier)
	if err != nil {
		return SubscriptionChannelCreditEquivalent{}, err
	}
	remainingMax, err := tokenbilling.EquivalentTokens(remaining, group.MinMultiplier)
	if err != nil {
		return SubscriptionChannelCreditEquivalent{}, err
	}
	minMultiplier := group.MinMultiplier
	maxMultiplier := group.MaxMultiplier
	base.ValueType = ChannelCreditEquivalentValueTypeRange
	base.MinMultiplier = &minMultiplier
	base.MaxMultiplier = &maxMultiplier
	base.EquivalentTokenLimitMin = &limitMin
	base.EquivalentTokenLimitMax = &limitMax
	base.EquivalentTokenRemainingMin = &remainingMin
	base.EquivalentTokenRemainingMax = &remainingMax
	return base, nil
}

func buildSubscriptionFixedRequestEquivalent(base SubscriptionChannelCreditEquivalent, creditLimit int64, remaining int64, group ChannelCreditBillingGroup) SubscriptionChannelCreditEquivalent {
	base.Kind = ChannelCreditEquivalentKindFixedRequest
	if group.VariantCount <= 1 {
		credits := group.MinFixedCredits
		limit := equivalentRequests(creditLimit, credits)
		remainingEquivalent := equivalentRequests(remaining, credits)
		base.ValueType = ChannelCreditEquivalentValueTypeSingle
		base.FixedRequestCredits = &credits
		base.EquivalentRequestLimit = &limit
		base.EquivalentRequestRemaining = &remainingEquivalent
		return base
	}
	minCredits := group.MinFixedCredits
	maxCredits := group.MaxFixedCredits
	limitMin := equivalentRequests(creditLimit, maxCredits)
	limitMax := equivalentRequests(creditLimit, minCredits)
	remainingMin := equivalentRequests(remaining, maxCredits)
	remainingMax := equivalentRequests(remaining, minCredits)
	base.ValueType = ChannelCreditEquivalentValueTypeRange
	base.FixedRequestCreditsMin = &minCredits
	base.FixedRequestCreditsMax = &maxCredits
	base.EquivalentRequestLimitMin = &limitMin
	base.EquivalentRequestLimitMax = &limitMax
	base.EquivalentRequestRemainingMin = &remainingMin
	base.EquivalentRequestRemainingMax = &remainingMax
	return base
}

func addMultiplierToGroup(group *ChannelCreditBillingGroup, multiplier float64) {
	if group == nil || containsSameChannelMultiplier(group.multipliers, multiplier) {
		return
	}
	if len(group.multipliers) == 0 || multiplier < group.MinMultiplier {
		group.MinMultiplier = multiplier
	}
	if len(group.multipliers) == 0 || multiplier > group.MaxMultiplier {
		group.MaxMultiplier = multiplier
	}
	group.multipliers = append(group.multipliers, multiplier)
}

func addFixedCreditsToGroup(group *ChannelCreditBillingGroup, credits int64) {
	if group == nil || containsSameFixedCredits(group.fixedCredits, credits) {
		return
	}
	if len(group.fixedCredits) == 0 || credits < group.MinFixedCredits {
		group.MinFixedCredits = credits
	}
	if len(group.fixedCredits) == 0 || credits > group.MaxFixedCredits {
		group.MaxFixedCredits = credits
	}
	group.fixedCredits = append(group.fixedCredits, credits)
}

func equivalentRequests(credits int64, fixedRequestCredits int64) int64 {
	if credits <= 0 || fixedRequestCredits <= 0 {
		return 0
	}
	return credits / fixedRequestCredits
}

func containsSameChannelMultiplier(values []float64, multiplier float64) bool {
	for _, value := range values {
		if tokenbilling.SameMultiplier(value, multiplier) {
			return true
		}
	}
	return false
}

func containsSameFixedCredits(values []int64, credits int64) bool {
	for _, value := range values {
		if value == credits {
			return true
		}
	}
	return false
}
