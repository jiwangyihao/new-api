package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetAvailability exposes only aggregate operational observations. Authentication
// is enforced on its route; neither credentials nor per-request records are returned.
func GetAvailability(c *gin.Context) {
	window := c.DefaultQuery("range", "24h")
	switch window {
	case "1h", "24h", "7d", "30d":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid availability range"})
		return
	}
	c.Header("Cache-Control", "private, no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	report, err := model.GetAvailabilityReport(ctx, window, time.Now())
	if err != nil {
		logger.LogError(c, "availability query failed: "+err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Availability statistics are temporarily unavailable"})
		return
	}
	common.ApiSuccess(c, report)
}
