package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPTAbuseControllerTrimsReasonAndAllowsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/gpt-abuse/users/81601/reset-warnings", map[string]any{
		"reason":           "  manual review  ",
		"clear_suspension": true,
	}, 1)

	reason, ok := normalizeGPTAbuseControllerReason(ctx, "  manual review  ")
	require.True(t, ok)
	assert.Equal(t, "manual review", reason)
	assert.Equal(t, http.StatusOK, recorder.Code)

	reason, ok = normalizeGPTAbuseControllerReason(ctx, "  ")
	require.True(t, ok)
	assert.Equal(t, "manual_review", reason)
}

func TestGPTAbuseControllerRecordsAdminManageLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 81611, Username: "gpt-abuse-log-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AffCode: "aff-gpt-abuse-log-admin"}).Error)
	ctx, _ := newAuthenticatedContext(t, http.MethodPost, "/api/gpt-abuse/users/81612/reset-warnings", nil, 81611)
	ctx.Set("username", "gpt-abuse-log-admin")
	ctx.Params = gin.Params{{Key: "id", Value: "81612"}}

	recordGPTAbuseManageLog(ctx, 81612, "reset_warnings", "manual review")

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeManage).First(&log).Error)
	assert.Equal(t, 81612, log.UserId)
	assert.Contains(t, log.Content, "reset_warnings")
	assert.Contains(t, log.Other, "manual review")
	assert.Contains(t, log.Other, "gpt-abuse-log-admin")
}

func TestGPTAbuseControllerRejectsInvalidReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/gpt-abuse/users/81601/reset-warnings", map[string]any{
		"reason":           strings.Repeat("x", 256),
		"clear_suspension": true,
	}, 1)
	ctx.Params = gin.Params{{Key: "id", Value: "81601"}}

	ResetGPTAbuseWarnings(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	response := decodeAPIResponse(t, recorder)
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "reason")
}
