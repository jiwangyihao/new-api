package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAdminTasksSummary(c *gin.Context) {
	pendingCommissionWithdrawals, err := service.CountPendingInvitationCommissionWithdrawals()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"pending_commission_withdrawals": pendingCommissionWithdrawals,
	})
}
