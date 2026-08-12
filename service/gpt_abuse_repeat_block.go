package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/tidwall/gjson"
)

const gptAbuseRepeatBlockEnabledEnv = "GPT_ABUSE_REPEAT_BLOCK_ENABLED"

var GPTAbuseRepeatBlockEnabled = gptAbuseRepeatBlockEnabledFromEnv()
var GPTAbuseRepeatBlockTTLSeconds = 900
var GPTAbuseRepeatBlockRequireRedis = false

const gptAbuseRepeatBlockContextKey = "gpt_abuse_repeat_block_context"
const gptAbuseRepeatBlockRedisKeyPrefix = "gpt_abuse:warned_request:v1"
const gptAbuseRepeatBlockFingerprintPrefixLength = 12

type GPTAbuseRepeatBlockFingerprint struct {
	Value  string
	Prefix string
}

type GPTAbuseRepeatBlockContext struct {
	Fingerprint  GPTAbuseRepeatBlockFingerprint
	EndpointPath string
	RelayMode    int
	OriginModel  string
	UserID       int
	TokenID      int
	Username     string
	TokenName    string
}

type GPTAbuseRepeatBlockCacheValue struct {
	FirstWarningLogID int    `json:"first_warning_log_id"`
	UserID            int    `json:"user_id"`
	TokenID           int    `json:"token_id"`
	RequestID         string `json:"request_id"`
	UpstreamRequestID string `json:"upstream_request_id"`
	Source            string `json:"source"`
	Kind              string `json:"kind"`
	Severity          string `json:"severity"`
	CreatedAt         int64  `json:"created_at"`
	ChannelID         int    `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	ChannelType       int    `json:"channel_type"`
	UpstreamModel     string `json:"upstream_model,omitempty"`
}

type gptAbuseRepeatBlockMemoryEntry struct {
	value     GPTAbuseRepeatBlockCacheValue
	expiresAt int64
}

var gptAbuseRepeatBlockMemoryCache = struct {
	sync.Mutex
	items map[string]gptAbuseRepeatBlockMemoryEntry
}{items: map[string]gptAbuseRepeatBlockMemoryEntry{}}

func gptAbuseRepeatBlockEnabledFromEnv() bool {
	return common.GetEnvOrDefaultBool(gptAbuseRepeatBlockEnabledEnv, false)
}

func BuildGPTAbuseRepeatBlockFingerprint(userID int, tokenID int, endpointPath string, relayMode int, originModel string, contentType string, body []byte) (GPTAbuseRepeatBlockFingerprint, error) {
	contentTypeSemantic, jsonBody := gptAbuseRepeatBlockContentTypeSemantic(contentType)
	bodyForDigest := body
	bodyMode := "raw"
	if jsonBody {
		if canonicalBody, ok := canonicalGPTAbuseRepeatBlockJSON(body); ok {
			bodyForDigest = canonicalBody
			bodyMode = "json"
		}
	}
	bodyDigest := sha256.Sum256(bodyForDigest)

	var input strings.Builder
	writeGPTAbuseRepeatBlockFingerprintPart(&input, "user_id", strconv.Itoa(userID))
	writeGPTAbuseRepeatBlockFingerprintPart(&input, "token_id", strconv.Itoa(tokenID))
	writeGPTAbuseRepeatBlockFingerprintPart(&input, "endpoint_path", normalizeGPTAbuseRepeatBlockEndpointPath(endpointPath))
	writeGPTAbuseRepeatBlockFingerprintPart(&input, "relay_mode", strconv.Itoa(relayMode))
	writeGPTAbuseRepeatBlockFingerprintPart(&input, "origin_model", strings.TrimSpace(originModel))
	writeGPTAbuseRepeatBlockFingerprintPart(&input, "content_type", contentTypeSemantic)
	writeGPTAbuseRepeatBlockFingerprintPart(&input, "body_mode", bodyMode)
	writeGPTAbuseRepeatBlockFingerprintPart(&input, "body_digest", hex.EncodeToString(bodyDigest[:]))

	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	_, _ = mac.Write([]byte(input.String()))
	value := hex.EncodeToString(mac.Sum(nil))
	return GPTAbuseRepeatBlockFingerprint{Value: value, Prefix: gptAbuseRepeatBlockFingerprintPrefix(value)}, nil
}

func CaptureGPTAbuseRepeatBlockFingerprint(c *gin.Context, info *relaycommon.RelayInfo, bodyStorage common.BodyStorage) error {
	if !GPTAbuseRepeatBlockEnabled || c == nil {
		return nil
	}
	if _, ok := GPTAbuseRepeatBlockContextFromGin(c); ok {
		return nil
	}
	if !shouldCaptureGPTAbuseRepeatBlock(info) {
		return nil
	}
	if bodyStorage == nil {
		return errors.New("body storage is nil")
	}
	body, err := bodyStorage.Bytes()
	if err != nil {
		return err
	}
	userID := 0
	tokenID := 0
	relayMode := 0
	originModel := ""
	endpointPath := ""
	if info != nil {
		userID = info.UserId
		tokenID = info.TokenId
		relayMode = info.RelayMode
		originModel = info.OriginModelName
		endpointPath = info.RequestURLPath
	}
	if userID <= 0 {
		userID = common.GetContextKeyInt(c, constant.ContextKeyUserId)
	}
	if tokenID <= 0 {
		tokenID = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	}
	if originModel == "" {
		originModel = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	}
	if c.Request != nil && c.Request.URL != nil {
		endpointPath = c.Request.URL.Path
	}
	contentType := ""
	if c.Request != nil {
		contentType = c.Request.Header.Get("Content-Type")
	}
	fingerprint, err := BuildGPTAbuseRepeatBlockFingerprint(userID, tokenID, endpointPath, relayMode, originModel, contentType, body)
	if err != nil {
		return err
	}
	c.Set(gptAbuseRepeatBlockContextKey, GPTAbuseRepeatBlockContext{
		Fingerprint:  fingerprint,
		EndpointPath: normalizeGPTAbuseRepeatBlockEndpointPath(endpointPath),
		RelayMode:    relayMode,
		OriginModel:  strings.TrimSpace(originModel),
		UserID:       userID,
		TokenID:      tokenID,
		Username:     ginString(c, string(constant.ContextKeyUserName)),
		TokenName:    ginString(c, "token_name"),
	})
	return nil
}

func GPTAbuseRepeatBlockContextFromGin(c *gin.Context) (GPTAbuseRepeatBlockContext, bool) {
	if c == nil {
		return GPTAbuseRepeatBlockContext{}, false
	}
	value, ok := c.Get(gptAbuseRepeatBlockContextKey)
	if !ok {
		return GPTAbuseRepeatBlockContext{}, false
	}
	switch typed := value.(type) {
	case GPTAbuseRepeatBlockContext:
		return typed, true
	case *GPTAbuseRepeatBlockContext:
		if typed != nil {
			return *typed, true
		}
	}
	return GPTAbuseRepeatBlockContext{}, false
}

func CheckGPTAbuseRepeatBlock(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if !GPTAbuseRepeatBlockEnabled {
		return nil
	}
	captured, ok := GPTAbuseRepeatBlockContextFromGin(c)
	if !ok || strings.TrimSpace(captured.Fingerprint.Value) == "" {
		return nil
	}
	cacheValue, ok := getGPTAbuseRepeatBlockCacheValue(c, gptAbuseRepeatBlockCacheKey(captured))
	if !ok {
		return nil
	}
	requestID := gptAbuseRepeatBlockRequestID(c, info)
	repeatLog := &model.GPTAbuseRepeatBlockLog{
		UserId:                        captured.UserID,
		Username:                      captured.Username,
		TokenId:                       captured.TokenID,
		TokenName:                     captured.TokenName,
		RequestId:                     requestID,
		Endpoint:                      captured.EndpointPath,
		RelayMode:                     captured.RelayMode,
		RequestedModel:                captured.OriginModel,
		BodyFingerprint:               captured.Fingerprint.Value,
		FirstWarningLogId:             cacheValue.FirstWarningLogID,
		FirstWarningAt:                cacheValue.CreatedAt,
		FirstWarningRequestId:         cacheValue.RequestID,
		FirstWarningUpstreamRequestId: cacheValue.UpstreamRequestID,
		FirstWarningSource:            cacheValue.Source,
		FirstWarningKind:              cacheValue.Kind,
		FirstWarningSeverity:          cacheValue.Severity,
		ChannelId:                     cacheValue.ChannelID,
		ChannelName:                   cacheValue.ChannelName,
		ChannelType:                   cacheValue.ChannelType,
	}
	if err := model.RecordGPTAbuseRepeatBlockLog(repeatLog); err != nil {
		logger.LogWarn(contextFromGin(c), "record GPT abuse repeat block log failed: "+err.Error())
	}
	message := fmt.Sprintf("Repeated request blocked locally: this exact request recently triggered an upstream GPT safety warning. The request was not sent upstream again. Please review and change the request content before retrying. request_id=%s; first_warning_log_id=%d; first_warning_at=%d", requestID, cacheValue.FirstWarningLogID, cacheValue.CreatedAt)
	return types.WithOpenAIError(types.OpenAIError{Message: message, Type: "invalid_request_error", Code: string(types.ErrorCodeGPTAbuseRepeatedWarningRequest)}, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

func StoreGPTAbuseRepeatBlock(c *gin.Context, info *relaycommon.RelayInfo, log *model.GPTAbuseSignalLog) {
	if !GPTAbuseRepeatBlockEnabled || log == nil || log.Id <= 0 || !log.CountEligible {
		return
	}
	captured, ok := GPTAbuseRepeatBlockContextFromGin(c)
	if !ok || strings.TrimSpace(captured.Fingerprint.Value) == "" {
		return
	}
	cacheValue := GPTAbuseRepeatBlockCacheValue{
		FirstWarningLogID: log.Id,
		UserID:            captured.UserID,
		TokenID:           captured.TokenID,
		RequestID:         log.RequestId,
		UpstreamRequestID: log.UpstreamRequestId,
		Source:            log.Source,
		Kind:              log.Kind,
		Severity:          log.Severity,
		CreatedAt:         log.CreatedAt,
		ChannelID:         log.ChannelId,
		ChannelName:       log.ChannelName,
		ChannelType:       log.ChannelType,
		UpstreamModel:     log.UpstreamModel,
	}
	if cacheValue.UpstreamModel == "" && info != nil {
		cacheValue.UpstreamModel = info.UpstreamModelName
	}
	setGPTAbuseRepeatBlockCacheValue(c, gptAbuseRepeatBlockCacheKey(captured), cacheValue)
}

func ResetGPTAbuseRepeatBlockCacheForTest() {
	gptAbuseRepeatBlockMemoryCache.Lock()
	defer gptAbuseRepeatBlockMemoryCache.Unlock()
	gptAbuseRepeatBlockMemoryCache.items = map[string]gptAbuseRepeatBlockMemoryEntry{}
}

func writeGPTAbuseRepeatBlockFingerprintPart(b *strings.Builder, name string, value string) {
	b.WriteString(name)
	b.WriteByte('=')
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
	b.WriteByte('\n')
}

func gptAbuseRepeatBlockContentTypeSemantic(contentType string) (string, bool) {
	trimmed := strings.TrimSpace(contentType)
	if trimmed == "" {
		return "", false
	}
	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(trimmed, ";")[0])
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType, mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func canonicalGPTAbuseRepeatBlockJSON(body []byte) ([]byte, bool) {
	if !gjson.ValidBytes(body) {
		return nil, false
	}
	var buffer bytes.Buffer
	buffer.Grow(len(body))
	if err := writeCanonicalGPTAbuseRepeatBlockJSONValue(&buffer, gjson.ParseBytes(body)); err != nil {
		return nil, false
	}
	return buffer.Bytes(), true
}

func writeCanonicalGPTAbuseRepeatBlockJSONValue(buffer *bytes.Buffer, value gjson.Result) error {
	switch value.Type {
	case gjson.Null:
		buffer.WriteString("null")
	case gjson.True:
		buffer.WriteString("true")
	case gjson.False:
		buffer.WriteString("false")
	case gjson.String:
		encoded, err := common.Marshal(value.String())
		if err != nil {
			return err
		}
		buffer.Write(encoded)
	case gjson.Number:
		normalized, err := canonicalGPTAbuseRepeatBlockJSONNumber(json.Number(value.Raw))
		if err != nil {
			return err
		}
		buffer.WriteString(normalized)
	case gjson.JSON:
		trimmed := strings.TrimSpace(value.Raw)
		if strings.HasPrefix(trimmed, "[") {
			buffer.WriteByte('[')
			first := true
			var writeErr error
			value.ForEach(func(_, item gjson.Result) bool {
				if !first {
					buffer.WriteByte(',')
				}
				first = false
				writeErr = writeCanonicalGPTAbuseRepeatBlockJSONValue(buffer, item)
				return writeErr == nil
			})
			if writeErr != nil {
				return writeErr
			}
			buffer.WriteByte(']')
			return nil
		}

		fields := make(map[string]gjson.Result)
		value.ForEach(func(key, item gjson.Result) bool {
			fields[key.String()] = item
			return true
		})
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buffer.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				buffer.WriteByte(',')
			}
			encodedKey, err := common.Marshal(key)
			if err != nil {
				return err
			}
			buffer.Write(encodedKey)
			buffer.WriteByte(':')
			if err := writeCanonicalGPTAbuseRepeatBlockJSONValue(buffer, fields[key]); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
	}
	return nil
}

func canonicalGPTAbuseRepeatBlockJSONNumber(number json.Number) (string, error) {
	raw := strings.TrimSpace(number.String())
	if raw == "" {
		return "", errors.New("empty JSON number")
	}
	sign := ""
	if raw[0] == '-' {
		sign = "-"
		raw = raw[1:]
	}
	exponent := 0
	if idx := strings.IndexAny(raw, "eE"); idx >= 0 {
		parsed, err := strconv.Atoi(raw[idx+1:])
		if err != nil {
			return "", err
		}
		exponent = parsed
		raw = raw[:idx]
	}
	intPart := raw
	fracPart := ""
	if idx := strings.IndexByte(raw, '.'); idx >= 0 {
		intPart = raw[:idx]
		fracPart = raw[idx+1:]
	}
	digits := strings.TrimLeft(intPart+fracPart, "0")
	if digits == "" {
		return "0", nil
	}
	decimalExponent := exponent - len(fracPart)
	for strings.HasSuffix(digits, "0") {
		digits = digits[:len(digits)-1]
		decimalExponent++
	}
	if decimalExponent > 1024 || decimalExponent < -1024 {
		return sign + digits + "e" + strconv.Itoa(decimalExponent), nil
	}
	if decimalExponent >= 0 {
		return sign + digits + strings.Repeat("0", decimalExponent), nil
	}
	point := len(digits) + decimalExponent
	if point > 0 {
		return sign + digits[:point] + "." + digits[point:], nil
	}
	return sign + "0." + strings.Repeat("0", -point) + digits, nil
}

func shouldCaptureGPTAbuseRepeatBlock(info *relaycommon.RelayInfo) bool {
	if info == nil || !ShouldMonitorGPTAbuse(info) {
		return false
	}
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeCompletions, relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		return true
	default:
		return false
	}
}

func gptAbuseRepeatBlockFingerprintPrefix(value string) string {
	if len(value) <= gptAbuseRepeatBlockFingerprintPrefixLength {
		return value
	}
	return value[:gptAbuseRepeatBlockFingerprintPrefixLength]
}

func normalizeGPTAbuseRepeatBlockEndpointPath(endpointPath string) string {
	endpointPath = strings.TrimSpace(endpointPath)
	if endpointPath == "" {
		return ""
	}
	if idx := strings.IndexByte(endpointPath, '?'); idx >= 0 {
		endpointPath = endpointPath[:idx]
	}
	return endpointPath
}

func gptAbuseRepeatBlockCacheKey(captured GPTAbuseRepeatBlockContext) string {
	return fmt.Sprintf("%s:%d:%d:%s:%d:%s:%s", gptAbuseRepeatBlockRedisKeyPrefix, captured.UserID, captured.TokenID, captured.EndpointPath, captured.RelayMode, captured.OriginModel, captured.Fingerprint.Value)
}

func getGPTAbuseRepeatBlockCacheValue(c *gin.Context, key string) (GPTAbuseRepeatBlockCacheValue, bool) {
	if !GPTAbuseRepeatBlockEnabled || key == "" {
		return GPTAbuseRepeatBlockCacheValue{}, false
	}
	if common.RedisEnabled {
		if common.RDB != nil {
			payload, err := common.RDB.Get(contextFromGin(c), key).Result()
			if err == nil {
				var value GPTAbuseRepeatBlockCacheValue
				if unmarshalErr := common.Unmarshal([]byte(payload), &value); unmarshalErr != nil {
					logger.LogWarn(contextFromGin(c), "unmarshal GPT abuse repeat block cache failed: "+unmarshalErr.Error())
				} else {
					return value, true
				}
			} else if !errors.Is(err, redis.Nil) {
				logger.LogWarn(contextFromGin(c), "read GPT abuse repeat block Redis cache failed: "+err.Error())
			}
		} else {
			logger.LogWarn(contextFromGin(c), "read GPT abuse repeat block Redis cache failed: redis client is nil")
		}
		if GPTAbuseRepeatBlockRequireRedis {
			return GPTAbuseRepeatBlockCacheValue{}, false
		}
	} else if GPTAbuseRepeatBlockRequireRedis {
		return GPTAbuseRepeatBlockCacheValue{}, false
	}
	return getGPTAbuseRepeatBlockMemoryValue(key)
}

func setGPTAbuseRepeatBlockCacheValue(c *gin.Context, key string, value GPTAbuseRepeatBlockCacheValue) {
	if !GPTAbuseRepeatBlockEnabled || key == "" || GPTAbuseRepeatBlockTTLSeconds <= 0 {
		return
	}
	payload, err := common.Marshal(value)
	if err != nil {
		logger.LogWarn(contextFromGin(c), "marshal GPT abuse repeat block cache failed: "+err.Error())
		return
	}
	ttl := time.Duration(GPTAbuseRepeatBlockTTLSeconds) * time.Second
	if common.RedisEnabled {
		if common.RDB != nil {
			stored, err := common.RDB.SetNX(contextFromGin(c), key, string(payload), ttl).Result()
			if err == nil {
				if stored {
					return
				}
				return
			}
			logger.LogWarn(contextFromGin(c), "write GPT abuse repeat block Redis cache failed: "+err.Error())
		} else {
			logger.LogWarn(contextFromGin(c), "write GPT abuse repeat block Redis cache failed: redis client is nil")
		}
		if GPTAbuseRepeatBlockRequireRedis {
			return
		}
	} else if GPTAbuseRepeatBlockRequireRedis {
		return
	}
	setGPTAbuseRepeatBlockMemoryValue(key, value, common.GetTimestamp()+int64(GPTAbuseRepeatBlockTTLSeconds))
}

func getGPTAbuseRepeatBlockMemoryValue(key string) (GPTAbuseRepeatBlockCacheValue, bool) {
	now := common.GetTimestamp()
	gptAbuseRepeatBlockMemoryCache.Lock()
	defer gptAbuseRepeatBlockMemoryCache.Unlock()
	entry, ok := gptAbuseRepeatBlockMemoryCache.items[key]
	if !ok {
		return GPTAbuseRepeatBlockCacheValue{}, false
	}
	if entry.expiresAt <= now {
		delete(gptAbuseRepeatBlockMemoryCache.items, key)
		return GPTAbuseRepeatBlockCacheValue{}, false
	}
	return entry.value, true
}

func setGPTAbuseRepeatBlockMemoryValue(key string, value GPTAbuseRepeatBlockCacheValue, expiresAt int64) {
	now := common.GetTimestamp()
	gptAbuseRepeatBlockMemoryCache.Lock()
	defer gptAbuseRepeatBlockMemoryCache.Unlock()
	for itemKey, entry := range gptAbuseRepeatBlockMemoryCache.items {
		if entry.expiresAt <= now {
			delete(gptAbuseRepeatBlockMemoryCache.items, itemKey)
		}
	}
	if existing, ok := gptAbuseRepeatBlockMemoryCache.items[key]; ok && existing.expiresAt > now {
		return
	}
	gptAbuseRepeatBlockMemoryCache.items[key] = gptAbuseRepeatBlockMemoryEntry{value: value, expiresAt: expiresAt}
}

func gptAbuseRepeatBlockRequestID(c *gin.Context, info *relaycommon.RelayInfo) string {
	if info != nil && strings.TrimSpace(info.RequestId) != "" {
		return strings.TrimSpace(info.RequestId)
	}
	return ginString(c, common.RequestIdKey)
}
