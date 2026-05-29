package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func parseAdminOpsSnapshotQuery(c *gin.Context) service.AdminOpsSnapshotQuery {
	query := service.AdminOpsSnapshotQuery{WindowSeconds: 300, Top: 5}
	if raw := strings.TrimSpace(c.Query("window_seconds")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			switch parsed {
			case 60, 300, 900, 3600:
				query.WindowSeconds = parsed
			}
		}
	}
	if raw := strings.TrimSpace(c.Query("top")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			if parsed < 1 {
				parsed = 1
			}
			if parsed > 20 {
				parsed = 20
			}
			query.Top = parsed
		}
	}
	return query
}

func parseAdminOpsConcurrencyQuery(c *gin.Context) service.AdminOpsConcurrencyQuery {
	query := service.AdminOpsConcurrencyQuery{Limit: 20, IncludeUsers: true, MinActiveOrQueued: 1}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			if parsed < 1 {
				parsed = 1
			}
			if parsed > 100 {
				parsed = 100
			}
			query.Limit = parsed
		}
	}
	if raw := strings.TrimSpace(c.Query("include_users")); raw != "" {
		query.IncludeUsers = raw != "false" && raw != "0"
	}
	if raw := strings.TrimSpace(c.Query("min_active_or_queued")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed >= 0 {
			query.MinActiveOrQueued = parsed
		}
	}
	return query
}

func GetAdminOpsSnapshot(c *gin.Context) {
	data, err := service.GetAdminOpsSnapshot(c.Request.Context(), parseAdminOpsSnapshotQuery(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func GetAdminOpsConcurrency(c *gin.Context) {
	data, err := service.GetAdminOpsConcurrency(c.Request.Context(), parseAdminOpsConcurrencyQuery(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}
