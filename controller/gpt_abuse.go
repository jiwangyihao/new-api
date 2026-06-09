package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func ListGPTAbuseUsers(c *gin.Context) {
	query, ok := parseGPTAbuseUserListQuery(c)
	if !ok {
		return
	}
	response, err := service.ListGPTAbuseUsers(c.Request.Context(), query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func ListGPTAbuseUserLogs(c *gin.Context) {
	userID, ok := parseGPTAbuseUserIDParam(c)
	if !ok {
		return
	}
	query, ok := parseGPTAbuseLogQuery(c)
	if !ok {
		return
	}
	response, err := service.ListGPTAbuseUserLogs(c.Request.Context(), userID, query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func ListGPTAbuseRepeatBlocks(c *gin.Context) {
	userID, ok := parseGPTAbuseUserIDParam(c)
	if !ok {
		return
	}
	query, ok := parseGPTAbuseRepeatBlockQuery(c)
	if !ok {
		return
	}
	response, err := service.ListGPTAbuseRepeatBlocks(c.Request.Context(), userID, query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func ClearGPTAbuseSuspension(c *gin.Context) {
	userID, ok := parseGPTAbuseUserIDParam(c)
	if !ok {
		return
	}
	var request dto.GPTAbuseClearSuspensionRequest
	if !bindGPTAbuseJSON(c, &request) {
		return
	}
	reason, ok := normalizeGPTAbuseControllerReason(c, request.Reason)
	if !ok {
		return
	}
	response, err := service.ClearGPTAbuseSuspension(c.Request.Context(), c.GetInt("id"), userID, reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordGPTAbuseManageLog(c, userID, "clear_suspension", reason)
	common.ApiSuccess(c, response)
}

func ResetGPTAbuseWarnings(c *gin.Context) {
	userID, ok := parseGPTAbuseUserIDParam(c)
	if !ok {
		return
	}
	var request dto.GPTAbuseResetWarningsRequest
	if !bindGPTAbuseJSON(c, &request) {
		return
	}
	reason, ok := normalizeGPTAbuseControllerReason(c, request.Reason)
	if !ok {
		return
	}
	response, err := service.ResetGPTAbuseWarnings(c.Request.Context(), c.GetInt("id"), userID, reason, request.ClearSuspension)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordGPTAbuseManageLog(c, userID, "reset_warnings", reason)
	common.ApiSuccess(c, response)
}

func parseGPTAbuseUserListQuery(c *gin.Context) (dto.GPTAbuseUserListQuery, bool) {
	start, ok := parseGPTAbuseOptionalInt64Query(c, "start_timestamp")
	if !ok {
		return dto.GPTAbuseUserListQuery{}, false
	}
	end, ok := parseGPTAbuseOptionalInt64Query(c, "end_timestamp")
	if !ok {
		return dto.GPTAbuseUserListQuery{}, false
	}
	userID, ok := parseGPTAbuseOptionalIntQuery(c, "user_id")
	if !ok {
		return dto.GPTAbuseUserListQuery{}, false
	}
	limit, ok := parseGPTAbuseOptionalIntQuery(c, "limit")
	if !ok {
		return dto.GPTAbuseUserListQuery{}, false
	}
	offset, ok := parseGPTAbuseOptionalIntQuery(c, "offset")
	if !ok {
		return dto.GPTAbuseUserListQuery{}, false
	}
	return dto.GPTAbuseUserListQuery{StartTimestamp: start, EndTimestamp: end, Keyword: strings.TrimSpace(c.Query("keyword")), UserID: userID, Status: strings.TrimSpace(c.Query("status")), Kind: strings.TrimSpace(c.Query("kind")), Severity: strings.TrimSpace(c.Query("severity")), Source: strings.TrimSpace(c.Query("source")), Limit: limit, Offset: offset, SortBy: strings.TrimSpace(c.Query("sort_by")), SortOrder: strings.TrimSpace(c.Query("sort_order"))}, true
}

func parseGPTAbuseLogQuery(c *gin.Context) (dto.GPTAbuseLogQuery, bool) {
	start, ok := parseGPTAbuseOptionalInt64Query(c, "start_timestamp")
	if !ok {
		return dto.GPTAbuseLogQuery{}, false
	}
	end, ok := parseGPTAbuseOptionalInt64Query(c, "end_timestamp")
	if !ok {
		return dto.GPTAbuseLogQuery{}, false
	}
	limit, ok := parseGPTAbuseOptionalIntQuery(c, "limit")
	if !ok {
		return dto.GPTAbuseLogQuery{}, false
	}
	offset, ok := parseGPTAbuseOptionalIntQuery(c, "offset")
	if !ok {
		return dto.GPTAbuseLogQuery{}, false
	}
	return dto.GPTAbuseLogQuery{StartTimestamp: start, EndTimestamp: end, Source: strings.TrimSpace(c.Query("source")), Kind: strings.TrimSpace(c.Query("kind")), Severity: strings.TrimSpace(c.Query("severity")), CountEligible: strings.TrimSpace(c.Query("count_eligible")), Limit: limit, Offset: offset}, true
}

func parseGPTAbuseRepeatBlockQuery(c *gin.Context) (dto.GPTAbuseRepeatBlockQuery, bool) {
	start, ok := parseGPTAbuseOptionalInt64Query(c, "start_timestamp")
	if !ok {
		return dto.GPTAbuseRepeatBlockQuery{}, false
	}
	end, ok := parseGPTAbuseOptionalInt64Query(c, "end_timestamp")
	if !ok {
		return dto.GPTAbuseRepeatBlockQuery{}, false
	}
	limit, ok := parseGPTAbuseOptionalIntQuery(c, "limit")
	if !ok {
		return dto.GPTAbuseRepeatBlockQuery{}, false
	}
	offset, ok := parseGPTAbuseOptionalIntQuery(c, "offset")
	if !ok {
		return dto.GPTAbuseRepeatBlockQuery{}, false
	}
	return dto.GPTAbuseRepeatBlockQuery{StartTimestamp: start, EndTimestamp: end, Limit: limit, Offset: offset}, true
}

func parseGPTAbuseUserIDParam(c *gin.Context) (int, bool) {
	userID, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || userID <= 0 {
		writeGPTAbuseBadRequest(c, "invalid user id")
		return 0, false
	}
	return userID, true
}

func recordGPTAbuseManageLog(c *gin.Context, targetUserID int, action string, reason string) {
	if c == nil {
		return
	}
	adminID := c.GetInt("id")
	model.RecordLogWithAdminInfo(targetUserID, model.LogTypeManage, "GPT abuse "+action+" user_id="+strconv.Itoa(targetUserID), map[string]any{
		"admin_id":       adminID,
		"admin_username": c.GetString("username"),
		"target_user_id": targetUserID,
		"action":         action,
		"reason":         reason,
		"ip":             c.ClientIP(),
	})
}

func parseGPTAbuseOptionalInt64Query(c *gin.Context, key string) (int64, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		writeGPTAbuseBadRequest(c, "invalid "+key)
		return 0, false
	}
	return value, true
}

func parseGPTAbuseOptionalIntQuery(c *gin.Context, key string) (int, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		writeGPTAbuseBadRequest(c, "invalid "+key)
		return 0, false
	}
	return value, true
}

func bindGPTAbuseJSON(c *gin.Context, v any) bool {
	if c.Request == nil || c.Request.Body == nil || c.Request.ContentLength == 0 {
		return true
	}
	if err := c.ShouldBindJSON(v); err != nil {
		writeGPTAbuseBadRequest(c, "invalid request body")
		return false
	}
	return true
}

func normalizeGPTAbuseControllerReason(c *gin.Context, reason string) (string, bool) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > 255 {
		writeGPTAbuseBadRequest(c, "reason must be at most 255 characters")
		return "", false
	}
	if reason == "" {
		return "manual_review", true
	}
	return reason, true
}

func writeGPTAbuseBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": message})
}
