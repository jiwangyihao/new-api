package service

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"slices"
	"strconv"
)

type RetryParam struct {
	Ctx                               *gin.Context
	TokenGroup                        string
	TokenGroups                       []string
	ModelName                         string
	Retry                             *int
	resetNextTry                      bool
	EndpointType                      constant.EndpointType
	FrozenTokenBillingMultiplier      float64
	FrozenBillingProfile              model.ChannelBillingProfile
	UsedChannelIds                    []int
	RequireSameTokenBillingMultiplier bool
	RequireSameBillingProfile         bool
	RequestPath                       string
	PreferredChannelID                int
	RequiredCredential                string
	RequireResponsesWebSocket         bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	if param == nil {
		return nil, "", nil
	}
	return selectRouteCandidates(param.Ctx, nil, param)
}

// SelectRouteForChannel enforces the same authorization, endpoint and frozen-profile
// constraints for hard bindings as normal selection, without a cross-channel fallback.
func SelectRouteForChannel(c *gin.Context, channel *model.Channel, param *RetryParam) (*model.Channel, string, error) {
	if channel == nil || param == nil {
		return nil, "", routeUnavailableError()
	}
	return selectRouteCandidates(c, channel, param)
}

func selectRouteCandidates(c *gin.Context, bound *model.Channel, param *RetryParam) (*model.Channel, string, error) {
	if c == nil {
		c = &gin.Context{}
	}
	groups := param.TokenGroups
	if len(groups) == 0 && param.TokenGroup != "" {
		groups = []string{param.TokenGroup}
	}
	if len(groups) == 0 {
		groups = c.GetStringSlice(string(constant.ContextKeyTokenGroups))
	}
	if slices.Contains(groups, model.DisabledTokenGroupSentinel) {
		return nil, "", types.NewErrorWithStatusCode(errors.New("token groups are disabled"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	var binding *ResponseBinding
	if value, exists := c.Get("response_route_binding"); exists {
		entry := value.(ResponseBinding)
		binding = &entry
		groups = []string{entry.Group}
	}
	used := param.UsedChannelIds
	if value, ok := c.Get(routeAttemptContextKey); ok {
		state := value.(*routeAttemptState)
		used = slices.DeleteFunc(slices.Clone(used), func(id int) bool { return state.attempted[id] })
	}
	channels, err := model.GetSatisfiedChannelCandidates(groups, param.ModelName, param.EndpointType, used, param.FrozenTokenBillingMultiplier, param.RequireSameTokenBillingMultiplier, param.FrozenBillingProfile, param.RequireSameBillingProfile)
	if err != nil {
		return nil, "", err
	}
	filtered := channels[:0]
	for _, channel := range channels {
		if bound != nil && channel.Id != bound.Id {
			continue
		}
		if binding != nil && (channel.Id != binding.ChannelID || RouteConfigurationFingerprint(channel) != binding.ConfigFingerprint) {
			continue
		}
		if param.RequireResponsesWebSocket {
			apiType, supported := common.ChannelType2APIType(channel.Type)
			if !channel.GetSetting().ResponsesWebSocketEnabled || !supported || (apiType != constant.APITypeOpenAI && apiType != constant.APITypeCodex) {
				continue
			}
		}
		filtered = append(filtered, channel)
	}
	if len(filtered) == 0 {
		return nil, "", types.NewErrorWithStatusCode(errors.New("no channel supports the requested model and endpoint"), types.ErrorCodeModelNotFound, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	if !RouteRetryBudgetAvailable(c) {
		return nil, "", routeUnavailableError()
	}
	selections := make([]routeSelection, 0, len(filtered))
	for _, channel := range filtered {
		group := model.DefaultChannelGroupName
		effective, groupErr := model.ResolveEffectiveGroupForChannel(groups, channel.Id)
		if groupErr != nil && !errors.Is(groupErr, gorm.ErrRecordNotFound) {
			return nil, "", groupErr
		}
		if effective != nil {
			group = effective.Name
		} else if len(groups) == 1 {
			group = groups[0]
		}
		if binding != nil {
			group = binding.Group
		}
		choices, buildErr := buildRouteSelections(c, []*model.Channel{channel}, group, param.ModelName, routeRequestPath(c, param.RequestPath), param.PreferredChannelID)
		if buildErr != nil {
			return nil, "", buildErr
		}
		for _, choice := range choices {
			if binding != nil && RouteCredentialFingerprint(choice.key) != binding.KeyFingerprint {
				continue
			}
			if param.RequiredCredential == "" || choice.key == param.RequiredCredential {
				selections = append(selections, choice)
			}
		}
	}
	channel, group, err := acquireRoute(c, selections)
	if err == nil && channel == nil {
		if c.Writer != nil && c.Writer.Header().Get("Retry-After") == "" {
			c.Header("Retry-After", strconv.Itoa(1))
		}
		err = routeUnavailableError()
	}
	return channel, group, err
}
