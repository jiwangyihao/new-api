package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Log struct {
	Id                         int     `json:"id" gorm:"index:idx_created_at_id,priority:1;index:idx_user_id_id,priority:2"`
	UserId                     int     `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt                  int64   `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type                       int     `json:"type" gorm:"index:idx_created_at_type"`
	Content                    string  `json:"content"`
	Username                   string  `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName                  string  `json:"token_name" gorm:"index;default:''"`
	ModelName                  string  `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota                      int     `json:"quota" gorm:"default:0"`
	PromptTokens               int     `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens           int     `json:"completion_tokens" gorm:"default:0"`
	MeteredTokens              *int    `json:"metered_tokens" gorm:"default:null"`
	UseTime                    int     `json:"use_time" gorm:"default:0"`
	IsStream                   bool    `json:"is_stream"`
	ChannelId                  int     `json:"channel" gorm:"index"`
	ChannelName                string  `json:"channel_name" gorm:"->"`
	TokenId                    int     `json:"token_id" gorm:"default:0;index"`
	Group                      string  `json:"-" gorm:"index"` // legacy business group column; ignored at runtime
	Ip                         string  `json:"ip" gorm:"index;default:''"`
	RequestId                  string  `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId          string  `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	SubscriptionID             *int    `json:"subscription_id,omitempty" gorm:"column:subscription_id;type:integer"`
	SubscriptionTokensConsumed *int64  `json:"subscription_tokens_consumed,omitempty" gorm:"column:subscription_tokens_consumed"`
	BillingSource              *string `json:"billing_source,omitempty" gorm:"column:billing_source;type:varchar(32)"`
	Endpoint                   *string `json:"endpoint,omitempty" gorm:"column:endpoint;type:varchar(255)"`
	Other                      string  `json:"other"`
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		// 用户侧绝不暴露上游渠道身份：清空渠道名与渠道 id（json:"channel"）。
		logs[i].ChannelName = ""
		logs[i].ChannelId = 0
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			// delete(otherMap, "reject_reason")
			delete(otherMap, "stream_status")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = startIdx + i + 1
	}
}

func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	err = LOG_DB.Model(&Log{}).Where("token_id = ?", tokenId).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err == nil {
		if queueErr := queueLogAggregationEventsForLogs([]*Log{log}); queueErr != nil {
			common.SysError("failed to queue log aggregation events: " + queueErr.Error())
			requestMissingLogAggregationReplay()
		}
	}
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// RecordLogWithAdminInfo 记录操作日志，并将管理员相关信息存入 Other.admin_info，
func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo map[string]interface{}) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	if len(adminInfo) > 0 {
		other := map[string]interface{}{
			"admin_info": adminInfo,
		}
		log.Other = common.MapToJsonStr(other)
	}
	if err := LOG_DB.Create(log).Error; err != nil {
		common.SysLog("failed to record log: " + err.Error())
	} else if queueErr := queueLogAggregationEventsForLogs([]*Log{log}); queueErr != nil {
		common.SysError("failed to queue log aggregation events: " + queueErr.Error())
		requestMissingLogAggregationReplay()
	}
}

func RecordTopupLog(userId int, content string, callerIp string, paymentMethod string, callbackPaymentMethod string) {
	username, _ := GetUsernameById(userId, false)
	adminInfo := map[string]interface{}{
		"server_ip":               common.GetIp(),
		"node_name":               common.NodeName,
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	}
	other := map[string]interface{}{
		"admin_info": adminInfo,
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Ip:        callerIp,
		Other:     common.MapToJsonStr(other),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record topup log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            "",
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err == nil {
		if queueErr := queueLogAggregationEventsForLogs([]*Log{log}); queueErr != nil {
			common.SysError("failed to queue log aggregation events: " + queueErr.Error())
			requestMissingLogAggregationReplay()
		}
	}
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(params.Other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	meteredTokens := normalizedMeteredTokens(params.PromptTokens, params.CompletionTokens, params.Other)
	logger.LogInfo(c, fmt.Sprintf(
		"record consume log: userId=%d, model=%s, tokenId=%d, channelId=%d, quota=%d, prompt=%d, completion=%d, metered=%d, useTime=%d, stream=%t",
		userId,
		params.ModelName,
		params.TokenId,
		params.ChannelId,
		params.Quota,
		params.PromptTokens,
		params.CompletionTokens,
		meteredTokens,
		params.UseTimeSeconds,
		params.IsStream,
	))
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		MeteredTokens:    &meteredTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            "",
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := consumeLogCoalescer.add(log)
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), meteredTokens)
		})
	}
}

func normalizedMeteredTokens(promptTokens, completionTokens int, other map[string]interface{}) int {
	if other != nil {
		for _, key := range []string{"metered_tokens", "raw_metered_tokens"} {
			if v, ok := intFromMapValue(other[key]); ok {
				if v < 0 {
					return 0
				}
				return v
			}
		}
		if v, ok := intFromMapValue(other["subscription_tokens_consumed"]); ok {
			if v < 0 {
				return 0
			}
			return v
		}
	}
	total := promptTokens + completionTokens
	if total < 0 {
		return 0
	}
	return total
}

func intFromMapValue(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		parsed, err := strconv.ParseInt(v.String(), 10, 64)
		if err == nil {
			return int(parsed), true
		}
		floatParsed, err := strconv.ParseFloat(v.String(), 64)
		if err != nil {
			return 0, false
		}
		return int(floatParsed), true
	default:
		return 0, false
	}
}

func intFromLogDerivedMapValue(value interface{}) (int, bool) {
	if v, ok := intFromMapValue(value); ok {
		return v, true
	}
	str, ok := value.(string)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(str), 10, 64)
	if err == nil {
		return int(parsed), true
	}
	floatParsed, err := strconv.ParseFloat(strings.TrimSpace(str), 64)
	if err != nil {
		return 0, false
	}
	return int(floatParsed), true
}

func int64FromMapValue(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return parsed, true
		}
		floatParsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		return int64(floatParsed), true
	case json.Number:
		parsed, err := strconv.ParseInt(v.String(), 10, 64)
		if err == nil {
			return parsed, true
		}
		floatParsed, err := strconv.ParseFloat(v.String(), 64)
		if err != nil {
			return 0, false
		}
		return int64(floatParsed), true
	default:
		return 0, false
	}
}

func stringFromMapValue(value interface{}) (string, bool) {
	v, ok := value.(string)
	return v, ok
}

func fillLogDerivedFields(log *Log) {
	if log == nil || strings.TrimSpace(log.Other) == "" {
		return
	}
	var other map[string]interface{}
	if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
		return
	}
	if log.SubscriptionID == nil {
		if v, ok := intFromLogDerivedMapValue(other["subscription_id"]); ok && v > 0 {
			log.SubscriptionID = &v
		}
	}
	if log.SubscriptionTokensConsumed == nil {
		if v, ok := int64FromMapValue(other["subscription_tokens_consumed"]); ok && v >= 0 {
			log.SubscriptionTokensConsumed = &v
		}
	}
	if log.BillingSource == nil {
		if value, ok := stringFromMapValue(other["billing_source"]); ok {
			log.BillingSource = &value
		}
	}
	if log.Endpoint == nil {
		if value, ok := stringFromMapValue(other["endpoint"]); ok {
			log.Endpoint = &value
		} else if value, ok := stringFromMapValue(other["request_path"]); ok {
			log.Endpoint = &value
		}
	}
}

type RecordTaskBillingLogParams struct {
	UserId    int
	LogType   int
	Content   string
	ChannelId int
	ModelName string
	Quota     int
	TokenId   int
	Group     string
	Other     map[string]interface{}
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      params.LogType,
		Content:   params.Content,
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     "",
		Other:     common.MapToJsonStr(params.Other),
	}
	fillLogDerivedFields(log)
	err := LOG_DB.Create(log).Error
	if err == nil {
		if queueErr := queueLogAggregationEventsForLogs([]*Log{log}); queueErr != nil {
			common.SysError("failed to queue log aggregation events: " + queueErr.Error())
			requestMissingLogAggregationReplay()
		}
	}
	if err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if username != "" {
		tx = tx.Where("logs.username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if common.MemoryCacheEnabled {
			// Cache get channel
			for _, channelId := range channelIds.Items() {
				if cacheChannel, err := CacheGetChannel(channelId); err == nil {
					channels = append(channels, struct {
						Id   int    `gorm:"column:id"`
						Name string `gorm:"column:name"`
					}{
						Id:   channelId,
						Name: cacheChannel.Name,
					})
				}
			}
		} else {
			// Bulk query channels from DB
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
				return logs, total, err
			}
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

const logSearchCountLimit = 10000

func GetAllLogsWithFilter(filter LogFilter, startIdx int, num int) (logs []*Log, total int64, err error) {
	tx, err := applyLogFilters(LOG_DB, filter, true)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if common.MemoryCacheEnabled {
			// Cache get channel
			for _, channelId := range channelIds.Items() {
				if cacheChannel, err := CacheGetChannel(channelId); err == nil {
					channels = append(channels, struct {
						Id   int    `gorm:"column:id"`
						Name string `gorm:"column:name"`
					}{
						Id:   channelId,
						Name: cacheChannel.Name,
					})
				}
			}
		} else {
			// Bulk query channels from DB
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
				return logs, total, err
			}
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

type LogFilter struct {
	LogType           int
	StartTimestamp    int64
	EndTimestamp      int64
	ModelName         string
	Username          string
	TokenName         string
	Channel           int
	RequestId         string
	UpstreamRequestId string
	TokenId           *int
	IsStream          *bool
	Status            string
	SelfUserId        *int
	UserId            *int
}

func applyLogFilters(tx *gorm.DB, filter LogFilter, qualify bool) (*gorm.DB, error) {
	col := func(name string) string {
		if qualify {
			return "logs." + name
		}
		return name
	}
	if filter.SelfUserId != nil {
		tx = tx.Where(col("user_id")+" = ?", *filter.SelfUserId)
	} else if filter.UserId != nil {
		tx = tx.Where(col("user_id")+" = ?", *filter.UserId)
	} else if filter.Username != "" {
		tx = tx.Where(col("username")+" = ?", filter.Username)
	}
	logType := filter.LogType
	switch filter.Status {
	case UsageAnalyticsStatusSuccess:
		logType = LogTypeConsume
	case UsageAnalyticsStatusError:
		logType = LogTypeError
	}
	if logType != LogTypeUnknown {
		tx = tx.Where(col("type")+" = ?", logType)
	}
	if filter.ModelName != "" {
		modelNamePattern, err := sanitizeLikePattern(filter.ModelName)
		if err != nil {
			return nil, err
		}
		tx = tx.Where(col("model_name")+" LIKE ? ESCAPE '!'", modelNamePattern)
	}
	if filter.TokenName != "" {
		tx = tx.Where(col("token_name")+" = ?", filter.TokenName)
	}
	if filter.RequestId != "" {
		tx = tx.Where(col("request_id")+" = ?", filter.RequestId)
	}
	if filter.UpstreamRequestId != "" {
		tx = tx.Where(col("upstream_request_id")+" = ?", filter.UpstreamRequestId)
	}
	if filter.StartTimestamp != 0 {
		tx = tx.Where(col("created_at")+" >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp != 0 {
		tx = tx.Where(col("created_at")+" <= ?", filter.EndTimestamp)
	}
	if filter.Channel != 0 {
		tx = tx.Where(col("channel_id")+" = ?", filter.Channel)
	}
	if filter.TokenId != nil {
		tx = tx.Where(col("token_id")+" = ?", *filter.TokenId)
	}
	if filter.IsStream != nil {
		tx = tx.Where(col("is_stream")+" = ?", *filter.IsStream)
	}
	return tx, nil
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	selfUserID := userId
	tx, err := applyLogFilters(LOG_DB, LogFilter{LogType: logType, StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, ModelName: modelName, TokenName: tokenName, RequestId: requestId, UpstreamRequestId: upstreamRequestId, SelfUserId: &selfUserID}, true)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

type Stat struct {
	Quota       int `json:"quota"`
	TotalTokens int `json:"total_tokens"`
	Rpm         int `json:"rpm"`
	Tpm         int `json:"tpm"`
}

func GetUserLogsWithFilter(filter LogFilter, startIdx int, num int) (logs []*Log, total int64, err error) {
	tx, err := applyLogFilters(LOG_DB, filter, true)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

func SumUsedQuotaWithFilter(filter LogFilter) (Stat, error) {
	return SumUsedQuotaWithFilterOptions(filter, LogStatOptions{})
}

func sumUsedQuotaWithFilterDirect(filter LogFilter) (stat Stat, err error) {
	if filter.LogType == LogTypeUnknown && filter.Status == "" {
		filter.LogType = LogTypeConsume
	}
	tx := LOG_DB.Table("logs").Select("COALESCE(SUM(quota), 0) AS quota, " + meteredTokensExpr() + " AS total_tokens")
	rpmTpmQuery := LOG_DB.Table("logs").Select("COUNT(*) AS rpm, " + meteredTokensExpr() + " AS tpm")
	tx, err = applyLogFilters(tx, filter, false)
	if err != nil {
		return stat, err
	}
	recentFilter := filter
	recentFilter.StartTimestamp = time.Now().Add(-60 * time.Second).Unix()
	recentFilter.EndTimestamp = 0
	rpmTpmQuery, err = applyLogFilters(rpmTpmQuery, recentFilter, false)
	if err != nil {
		return stat, err
	}

	if err := tx.Scan(&stat).Error; err != nil {
		common.SysError("failed to query log stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	if err := rpmTpmQuery.Scan(&stat).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}

	return stat, nil
}

func RunLogDBReadSnapshot(fn func(tx *gorm.DB) error) error {
	if LOG_DB == nil {
		return errors.New("log database is nil")
	}
	switch LOG_DB.Dialector.Name() {
	case "sqlite", "sqlite3":
		return LOG_DB.Transaction(fn)
	default:
		return LOG_DB.Transaction(fn, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	}
}

func sumUsedQuotaWithFilterAggregated(filter LogFilter) (Stat, bool, error) {
	if !canUseLogUsageHourlyAggregation(filter) || LOG_DB == nil || !LOG_DB.Migrator().HasTable(&LogUsageHourly{}) || !LOG_DB.Migrator().HasTable(&LogAggregationEvent{}) {
		return Stat{}, false, nil
	}
	normalized := normalizeLogStatFilter(filter)
	start := normalized.StartTimestamp
	end := normalized.EndTimestamp
	if end == 0 {
		end = common.GetTimestamp()
	}
	if end < start {
		return Stat{}, true, nil
	}
	firstFullHour := ((start + 3599) / 3600) * 3600
	lastFullHour := (end / 3600) * 3600
	if firstFullHour >= lastFullHour {
		stat, err := sumUsedQuotaWithFilterDirect(filter)
		return stat, true, err
	}

	var totals logUsageStatTotals
	var rpmTpm Stat
	err := RunLogDBReadSnapshot(func(tx *gorm.DB) error {
		if firstFullHour > start {
			head, err := sumLogUsageDetailStats(tx, normalized, start, firstFullHour, false, false)
			if err != nil {
				return err
			}
			totals.add(head)
		}
		aggregated, err := sumLogUsageHourlyStats(tx, normalized, firstFullHour, lastFullHour)
		if err != nil {
			return err
		}
		totals.add(aggregated)
		fallback, err := sumLogUsageDetailStats(tx, normalized, firstFullHour, lastFullHour, false, true)
		if err != nil {
			return err
		}
		totals.add(fallback)
		tail, err := sumLogUsageDetailStats(tx, normalized, lastFullHour, end, true, false)
		if err != nil {
			return err
		}
		totals.add(tail)
		rpmTpm, err = sumLogUsageRecentDirect(tx, filter)
		return err
	})
	if err != nil {
		return Stat{}, true, err
	}

	stat := Stat{Quota: int(totals.Quota), TotalTokens: int(totals.MeteredTokens), Rpm: rpmTpm.Rpm, Tpm: rpmTpm.Tpm}
	return stat, true, nil
}

func normalizeLogStatFilter(filter LogFilter) LogFilter {
	if filter.LogType == LogTypeUnknown && filter.Status == "" {
		filter.LogType = LogTypeConsume
	}
	return filter
}

func canUseLogUsageHourlyAggregation(filter LogFilter) bool {
	if filter.Username != "" || filter.TokenName != "" || filter.RequestId != "" || filter.UpstreamRequestId != "" || filter.IsStream != nil {
		return false
	}
	logType := filter.LogType
	switch filter.Status {
	case UsageAnalyticsStatusSuccess:
		logType = LogTypeConsume
	case UsageAnalyticsStatusError:
		logType = LogTypeError
	case "":
	default:
		return false
	}
	return logType == LogTypeUnknown || logType == LogTypeConsume || logType == LogTypeError
}

type logUsageStatTotals struct {
	Requests      int64
	Quota         int64
	MeteredTokens int64
}

func (totals *logUsageStatTotals) add(other logUsageStatTotals) {
	totals.Requests += other.Requests
	totals.Quota += other.Quota
	totals.MeteredTokens += other.MeteredTokens
}

func sumLogUsageHourlyStats(db *gorm.DB, filter LogFilter, startBucket int64, endBucket int64) (logUsageStatTotals, error) {
	if endBucket <= startBucket {
		return logUsageStatTotals{}, nil
	}
	query := db.Table("log_usage_hourly").Select("COALESCE(SUM(request_count), 0) AS requests, COALESCE(SUM(quota_sum), 0) AS quota, COALESCE(SUM(metered_tokens_sum), 0) AS metered_tokens").
		Where("bucket_start >= ?", startBucket).
		Where("bucket_start < ?", endBucket)
	query, err := applyLogUsageHourlyFilters(query, filter)
	if err != nil {
		return logUsageStatTotals{}, err
	}
	var totals logUsageStatTotals
	if err := query.Scan(&totals).Error; err != nil {
		common.SysError("failed to query log usage hourly stat: " + err.Error())
		return logUsageStatTotals{}, errors.New("查询统计数据失败")
	}
	return totals, nil
}

func sumLogUsageDetailStats(db *gorm.DB, filter LogFilter, start int64, end int64, inclusiveEnd bool, onlyUnapplied bool) (logUsageStatTotals, error) {
	if inclusiveEnd {
		if end < start {
			return logUsageStatTotals{}, nil
		}
	} else if end <= start {
		return logUsageStatTotals{}, nil
	}
	query := db.Table("logs").Select("COUNT(*) AS requests, COALESCE(SUM(quota), 0) AS quota, " + meteredTokensExpr() + " AS metered_tokens")
	filter.StartTimestamp = 0
	filter.EndTimestamp = 0
	var err error
	query, err = applyLogFilters(query, filter, false)
	if err != nil {
		return logUsageStatTotals{}, err
	}
	query = query.Where("created_at >= ?", start)
	if inclusiveEnd {
		query = query.Where("created_at <= ?", end)
	} else {
		query = query.Where("created_at < ?", end)
	}
	if onlyUnapplied {
		query = query.Where("NOT EXISTS (SELECT 1 FROM log_aggregation_events WHERE log_aggregation_events.log_id = logs.id AND log_aggregation_events.aggregate_name = ? AND log_aggregation_events.status = ?)", logAggregationNameLogUsageHourly, logAggregationEventStatusApplied)
	}
	var totals logUsageStatTotals
	if err := query.Scan(&totals).Error; err != nil {
		common.SysError("failed to query log detail stat: " + err.Error())
		return logUsageStatTotals{}, errors.New("查询统计数据失败")
	}
	return totals, nil
}

func sumLogUsageRecentDirect(db *gorm.DB, filter LogFilter) (Stat, error) {
	recentFilter := filter
	recentFilter.StartTimestamp = time.Now().Add(-60 * time.Second).Unix()
	recentFilter.EndTimestamp = 0
	query := db.Table("logs").Select("COUNT(*) AS rpm, " + meteredTokensExpr() + " AS tpm")
	var err error
	query, err = applyLogFilters(query, recentFilter, false)
	if err != nil {
		return Stat{}, err
	}
	var stat Stat
	if err := query.Scan(&stat).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat: " + err.Error())
		return Stat{}, errors.New("查询统计数据失败")
	}
	return stat, nil
}

func applyLogUsageHourlyFilters(query *gorm.DB, filter LogFilter) (*gorm.DB, error) {
	if filter.SelfUserId != nil {
		query = query.Where("user_id = ?", *filter.SelfUserId)
	} else if filter.UserId != nil {
		query = query.Where("user_id = ?", *filter.UserId)
	}
	logType := filter.LogType
	switch filter.Status {
	case UsageAnalyticsStatusSuccess:
		logType = LogTypeConsume
	case UsageAnalyticsStatusError:
		logType = LogTypeError
	}
	if logType == LogTypeError {
		query = query.Where("status = ?", UsageAnalyticsStatusError)
	} else if logType == LogTypeConsume || logType == LogTypeUnknown {
		query = query.Where("status = ?", UsageAnalyticsStatusSuccess)
	}
	if filter.ModelName != "" {
		modelNamePattern, err := sanitizeLikePattern(filter.ModelName)
		if err != nil {
			return nil, err
		}
		query = query.Where("model_name LIKE ? ESCAPE '!'", modelNamePattern)
	}
	if filter.Channel != 0 {
		query = query.Where("channel_id = ?", filter.Channel)
	}
	if filter.TokenId != nil {
		query = query.Where("token_id = ?", *filter.TokenId)
	}
	return query, nil
}

func meteredTokensExpr() string {
	return "COALESCE(SUM(CASE WHEN metered_tokens IS NOT NULL THEN metered_tokens ELSE prompt_tokens + completion_tokens END), 0)"
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat, err error) {
	return sumUsedQuotaWithFilterDirect(LogFilter{LogType: logType, StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, ModelName: modelName, Username: username, TokenName: tokenName, Channel: channel})
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
