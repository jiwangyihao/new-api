package controller

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetInvitationEntitlement(c *gin.Context) {
	userId := c.GetInt("id")
	status, err := service.GetInvitationEntitlementStatus(userId, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}
