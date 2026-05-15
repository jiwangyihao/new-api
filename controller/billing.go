package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func subscriptionTokensToBillingAmount(tokens int64) float64 {
	if tokens <= 0 {
		return 0
	}
	amount := float64(tokens)
	// OpenAI 兼容接口字段名保留 *_usd，但这里映射的是订阅套餐 token 语义。
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		amount = amount / common.QuotaPerUnit * operation_setting.USDExchangeRate
	case operation_setting.QuotaDisplayTypeTokens:
		// amount 保持 token 数值，避免暴露钱包余额语义。
	default:
		amount = amount / common.QuotaPerUnit
	}
	return amount
}

func GetSubscription(c *gin.Context) {
	userId := c.GetInt("id")
	usage, err := model.GetActiveDistributorSubscriptionUsage(userId)
	expiredTime := int64(0)
	limitTokens := int64(0)
	if usage != nil {
		limitTokens = usage.TokenLimit
		expiredTime = usage.EndTime
		if usage.Unlimited {
			limitTokens = 100000000
		}
	}
	if expiredTime <= 0 {
		expiredTime = 0
	}
	if err != nil {
		openAIError := types.OpenAIError{
			Message: err.Error(),
			Type:    "upstream_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	amount := subscriptionTokensToBillingAmount(limitTokens)
	subscription := OpenAISubscriptionResponse{
		Object:             "billing_subscription",
		HasPaymentMethod:   true,
		SoftLimitUSD:       amount,
		HardLimitUSD:       amount,
		SystemHardLimitUSD: amount,
		AccessUntil:        expiredTime,
	}
	c.JSON(200, subscription)
	return
}

func GetUsage(c *gin.Context) {
	userId := c.GetInt("id")
	usageInfo, err := model.GetActiveDistributorSubscriptionUsage(userId)
	usedTokens := int64(0)
	if usageInfo != nil {
		usedTokens = usageInfo.TokenUsed
	}
	if err != nil {
		openAIError := types.OpenAIError{
			Message: err.Error(),
			Type:    "new_api_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	amount := subscriptionTokensToBillingAmount(usedTokens)
	usage := OpenAIUsageResponse{
		Object:     "list",
		TotalUsage: amount * 100,
	}
	c.JSON(200, usage)
	return
}
