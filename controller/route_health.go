package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetRouteHealth is registered exclusively inside the administrator channel group.
func GetRouteHealth(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	report, err := service.BuildRouteHealthOverview(ctx)
	if err != nil {
		// Storage and configuration errors can contain sensitive context.
		logger.LogError(c, "route health overview query failed")
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Route health overview is temporarily unavailable"})
		return
	}
	common.ApiSuccess(c, report)
}
