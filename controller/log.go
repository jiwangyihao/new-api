package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, status, err := parseLogStatusType(c)
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	tokenID, err := parseOptionalPositiveIntQuery(c, "token_id")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	isStream, err := parseOptionalBoolQuery(c, "is_stream")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	userID, err := parseOptionalPositiveIntQuery(c, "user_id")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	filter := model.LogFilter{LogType: logType, StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, ModelName: modelName, Username: username, TokenName: tokenName, Channel: channel, RequestId: requestId, UpstreamRequestId: upstreamRequestId, TokenId: tokenID, IsStream: isStream, Status: status, UserId: userID}
	logs, total, err := model.GetAllLogsWithFilter(filter, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func writeLogBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"message": message,
	})
}

func parseLogStatusType(c *gin.Context) (int, string, error) {
	logType := model.LogTypeUnknown
	if rawType := strings.TrimSpace(c.Query("type")); rawType != "" {
		parsed, err := strconv.Atoi(rawType)
		if err != nil {
			return 0, "", err
		}
		logType = parsed
	}
	status := strings.TrimSpace(c.Query("status"))
	if status == "" {
		return logType, "", nil
	}
	var statusType int
	switch status {
	case model.UsageAnalyticsStatusSuccess:
		statusType = model.LogTypeConsume
	case model.UsageAnalyticsStatusError:
		statusType = model.LogTypeError
	default:
		return 0, "", errors.New("invalid status")
	}
	if logType != model.LogTypeUnknown && logType != statusType {
		return 0, "", errors.New("status conflicts with type")
	}
	return logType, status, nil
}

func parseOptionalBoolQuery(c *gin.Context, key string) (*bool, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, nil
	}
	switch raw {
	case "true":
		value := true
		return &value, nil
	case "false":
		value := false
		return &value, nil
	default:
		return nil, errors.New("invalid " + key)
	}
}

func parseOptionalPositiveIntQuery(c *gin.Context, key string) (*int, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return nil, errors.New("invalid " + key)
	}
	return &value, nil
}

func validateSelfLogTokenFilter(userID int, tokenID int, startTimestamp int64, endTimestamp int64) error {
	var token model.Token
	err := model.DB.Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error
	if err == nil {
		return nil
	}
	var count int64
	query := model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND token_id = ?", userID, tokenID)
	if startTimestamp != 0 {
		query = query.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		query = query.Where("created_at <= ?", endTimestamp)
	}
	if err := query.Limit(1).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New("invalid token_id")
	}
	return nil
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, status, err := parseLogStatusType(c)
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	tokenID, err := parseOptionalPositiveIntQuery(c, "token_id")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	isStream, err := parseOptionalBoolQuery(c, "is_stream")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	if tokenID != nil {
		if err := validateSelfLogTokenFilter(userId, *tokenID, startTimestamp, endTimestamp); err != nil {
			writeLogBadRequest(c, "invalid token_id")
			return
		}
	}
	filter := model.LogFilter{LogType: logType, StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, ModelName: modelName, TokenName: tokenName, RequestId: requestId, UpstreamRequestId: upstreamRequestId, TokenId: tokenID, IsStream: isStream, Status: status, SelfUserId: &userId}
	logs, total, err := model.GetUserLogsWithFilter(filter, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	logType, status, err := parseLogStatusType(c)
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	tokenID, err := parseOptionalPositiveIntQuery(c, "token_id")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	isStream, err := parseOptionalBoolQuery(c, "is_stream")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	userID, err := parseOptionalPositiveIntQuery(c, "user_id")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	filter := model.LogFilter{LogType: logType, StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, ModelName: modelName, Username: username, TokenName: tokenName, Channel: channel, TokenId: tokenID, IsStream: isStream, Status: status, UserId: userID}
	stat, err := model.SumUsedQuotaWithFilter(filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota":        stat.Quota,
			"total_tokens": stat.TotalTokens,
			"rpm":          stat.Rpm,
			"tpm":          stat.Tpm,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	userId := c.GetInt("id")
	logType, status, err := parseLogStatusType(c)
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	tokenID, err := parseOptionalPositiveIntQuery(c, "token_id")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	isStream, err := parseOptionalBoolQuery(c, "is_stream")
	if err != nil {
		writeLogBadRequest(c, err.Error())
		return
	}
	if tokenID != nil {
		if err := validateSelfLogTokenFilter(userId, *tokenID, startTimestamp, endTimestamp); err != nil {
			writeLogBadRequest(c, "invalid token_id")
			return
		}
	}
	filter := model.LogFilter{LogType: logType, StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, ModelName: modelName, TokenName: tokenName, Channel: channel, TokenId: tokenID, IsStream: isStream, Status: status, SelfUserId: &userId}
	quotaNum, err := model.SumUsedQuotaWithFilter(filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	data := gin.H{
		"quota":        quotaNum.Quota,
		"total_tokens": quotaNum.TotalTokens,
		"rpm":          quotaNum.Rpm,
		"tpm":          quotaNum.Tpm,
		//"token": tokenNum,
	}
	if tokenID != nil {
		data["token_id"] = *tokenID
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
	return
}

func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	count, err := model.DeleteOldLog(c.Request.Context(), targetTimestamp, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}
