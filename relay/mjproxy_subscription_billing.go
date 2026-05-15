package relay

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func EnsureMidjourneySubscriptionBilling(c *gin.Context, info *relaycommon.RelayInfo, preConsumedQuota int) *dto.MidjourneyResponse {
	if info == nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "active subscription is required")
	}
	session, apiErr := service.NewBillingSession(c, info, preConsumedQuota)
	if apiErr == nil && session != nil {
		session.Refund(c)
	}
	if apiErr == nil {
		return service.MidjourneyErrorWrapper(constant.MjRequestError, "midjourney subscription billing is not supported")
	}
	description := apiErr.Error()
	if !strings.Contains(strings.ToLower(description), "subscription") {
		description = "active subscription is required: " + description
	}
	return service.MidjourneyErrorWrapper(constant.MjRequestError, description)
}
