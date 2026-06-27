package service

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
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
	groups := param.TokenGroups
	if len(groups) == 0 && param.TokenGroup != "" {
		groups = []string{param.TokenGroup}
	}
	channel, err := model.GetRandomSatisfiedChannelForEndpointWithGroups(groups, param.ModelName, param.GetRetry(), param.EndpointType, param.UsedChannelIds, param.FrozenTokenBillingMultiplier, param.RequireSameTokenBillingMultiplier, param.FrozenBillingProfile, param.RequireSameBillingProfile)
	return channel, "default", err
}
