package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

type tokenPayload struct {
	model.Token
	Group           *string `json:"group"`
	CrossGroupRetry *bool   `json:"cross_group_retry"`
}

const maxAPIKeyTokenLimit int64 = 10_000_000_000_000

type tokenResponse struct {
	*model.Token
	TokenLimitEnabled bool  `json:"token_limit_enabled"`
	TokenLimit        int64 `json:"token_limit"`
	TokenUsed         int64 `json:"token_used"`
	TokenRemaining    int64 `json:"token_remaining"`
	TokenUnlimited    bool  `json:"token_unlimited"`
}

func buildMaskedTokenResponse(token *model.Token) *tokenResponse {
	if token == nil {
		return nil
	}
	maskedToken := *token
	maskedToken.Key = token.GetMaskedKey()
	view := maskedToken.BuildTokenLimitView()
	return &tokenResponse{
		Token:             &maskedToken,
		TokenLimitEnabled: view.TokenLimitEnabled,
		TokenLimit:        view.TokenLimit,
		TokenUsed:         view.TokenUsed,
		TokenRemaining:    view.TokenRemaining,
		TokenUnlimited:    view.TokenUnlimited,
	}
}

func buildMaskedTokenResponses(tokens []*model.Token) []*tokenResponse {
	maskedTokens := make([]*tokenResponse, 0, len(tokens))
	for _, token := range tokens {
		maskedTokens = append(maskedTokens, buildMaskedTokenResponse(token))
	}
	return maskedTokens
}

func normalizeTokenLimitFields(token *model.Token) error {
	if token == nil {
		return errors.New("token is nil")
	}
	if !token.TokenLimitEnabled {
		token.TokenLimit = 0
		return nil
	}
	if token.TokenLimit <= 0 {
		return errors.New("token limit must be greater than 0")
	}
	if token.TokenLimit > maxAPIKeyTokenLimit {
		return fmt.Errorf("token limit exceeds max: %d", maxAPIKeyTokenLimit)
	}
	return nil
}

func GetOpenCodeOpenAIModels(c *gin.Context) {
	if _, ok := c.GetQuery("api_key"); ok {
		writeConfigGuideError(c, http.StatusBadRequest, "token_id is required")
		return
	}
	tokenIDRaw := strings.TrimSpace(c.Query("token_id"))
	if tokenIDRaw == "" {
		writeConfigGuideError(c, http.StatusBadRequest, "token_id is required")
		return
	}
	tokenID, err := strconv.Atoi(tokenIDRaw)
	if err != nil || tokenID <= 0 {
		writeConfigGuideError(c, http.StatusBadRequest, "token_id is required")
		return
	}

	token, ok := loadConfigGuideTokenByID(c, tokenID, c.GetInt("id"))
	if !ok {
		return
	}
	user, _, ok := validateConfigGuideTokenUsability(c, token)
	if !ok {
		return
	}
	metadata, ok := requireConfigGuideOpenAIModels(c)
	if !ok {
		return
	}
	effective, err := buildConfigGuideEffectiveModels(configGuideEffectiveModelsInput{
		Client:          configGuideClientOpenCode,
		Metadata:        metadata,
		AvailableModels: availableConfigGuideModelsForToken(token, user),
	})
	if err != nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
		return
	}
	common.ApiSuccess(c, gin.H{"models": effective})
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userId)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")

	pageInfo := common.GetPageQuery(c)

	tokens, total, err := model.SearchUserTokens(userId, keyword, token, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildMaskedTokenResponse(token))
}

func GetTokenKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"key": token.GetFullKey(),
	})
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	view := token.BuildTokenLimitView()
	c.JSON(http.StatusOK, gin.H{
		"object":              "credit_summary",
		"total_granted":       token.RemainQuota,
		"total_used":          token.UsedQuota,
		"total_available":     token.RemainQuota,
		"expires_at":          expiredAt * 1000,
		"token_limit_enabled": view.TokenLimitEnabled,
		"token_limit":         view.TokenLimit,
		"token_used":          view.TokenUsed,
		"token_remaining":     view.TokenRemaining,
		"token_unlimited":     view.TokenUnlimited,
	})
}

func GetTokenUsage(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No Authorization header",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid Bearer token",
		})
		return
	}
	tokenKey := parts[1]

	token, err := model.GetTokenByKey(strings.TrimPrefix(tokenKey, "sk-"), false)
	if err != nil {
		common.SysError("failed to get token by key: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	view := token.BuildTokenLimitView()
	legacyTotalGranted := token.RemainQuota + token.UsedQuota
	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":                 "token_usage",
			"name":                   token.Name,
			"total_granted":          legacyTotalGranted,
			"total_used":             token.UsedQuota,
			"total_available":        token.RemainQuota,
			"legacy_total_granted":   legacyTotalGranted,
			"legacy_total_used":      token.UsedQuota,
			"legacy_total_available": token.RemainQuota,
			"unlimited_quota":        token.UnlimitedQuota,
			"model_limits":           token.GetModelLimitsMap(),
			"model_limits_enabled":   token.ModelLimitsEnabled,
			"expires_at":             expiredAt,
			"token_limit_enabled":    view.TokenLimitEnabled,
			"token_limit":            view.TokenLimit,
			"token_used":             view.TokenUsed,
			"token_remaining":        view.TokenRemaining,
			"token_unlimited":        view.TokenUnlimited,
		},
	})
}

func AddToken(c *gin.Context) {
	token := model.Token{}
	err := c.ShouldBindJSON(&token)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	// Legacy quota fields are kept for compatibility only; API key token cap is validated separately.
	if token.RemainQuota < 0 {
		common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
		return
	}
	if err := normalizeTokenLimitFields(&token); err != nil {
		common.ApiError(c, err)
		return
	}
	// 检查用户令牌数量是否已达上限
	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= maxTokens {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("已达到最大令牌数量限制 (%d)", maxTokens),
		})
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	cleanToken := model.Token{
		UserId:             c.GetInt("id"),
		Name:               token.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           token.AllowIps,
		TokenLimitEnabled:  token.TokenLimitEnabled,
		TokenLimit:         token.TokenLimit,
		TokenUsed:          0,
	}
	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(&cleanToken),
	})
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	err := model.DeleteTokenById(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	statusOnly := c.Query("status_only")
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req tokenPayload
	err = common.Unmarshal(body, &req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var rawPayload map[string]any
	if err = common.Unmarshal(body, &rawPayload); err != nil {
		common.ApiError(c, err)
		return
	}
	token := req.Token
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	// Legacy quota fields are kept for compatibility only; API key token cap is validated separately.
	_, hasRemainQuota := rawPayload["remain_quota"]
	_, hasUnlimitedQuota := rawPayload["unlimited_quota"]
	_, hasTokenLimitEnabled := rawPayload["token_limit_enabled"]
	_, hasTokenLimit := rawPayload["token_limit"]
	if hasRemainQuota && token.RemainQuota < 0 {
		common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
		return
	}
	cleanToken, err := model.GetTokenByIds(token.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if token.Status == common.TokenStatusEnabled {
		if cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			common.ApiErrorI18n(c, i18n.MsgTokenExpiredCannotEnable)
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = token.Status
	} else {
		// If you add more fields, please also update token.Update()
		cleanToken.Name = token.Name
		cleanToken.ExpiredTime = token.ExpiredTime
		if hasRemainQuota {
			cleanToken.RemainQuota = token.RemainQuota
		}
		if hasUnlimitedQuota {
			cleanToken.UnlimitedQuota = token.UnlimitedQuota
		}
		cleanToken.ModelLimitsEnabled = token.ModelLimitsEnabled
		cleanToken.ModelLimits = token.ModelLimits
		cleanToken.AllowIps = token.AllowIps
		if hasTokenLimitEnabled {
			cleanToken.TokenLimitEnabled = token.TokenLimitEnabled
		}
		if hasTokenLimit {
			cleanToken.TokenLimit = token.TokenLimit
		}
		if hasTokenLimitEnabled || hasTokenLimit {
			if err := normalizeTokenLimitFields(cleanToken); err != nil {
				common.ApiError(c, err)
				return
			}
		}

	}
	err = cleanToken.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(cleanToken),
	})
}

func ResetTokenUsage(c *gin.Context) {
	tokenId, err := strconv.Atoi(c.Param("id"))
	if err != nil || tokenId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userId := c.GetInt("id")
	before, err := model.ResetTokenUsage(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLogWithAdminInfo(userId, model.LogTypeManage, "reset token usage", map[string]interface{}{
		"token_id":          tokenId,
		"operator_user_id":  userId,
		"before_token_used": before,
		"after_token_used":  int64(0),
		"reset_at":          common.GetTimestamp(),
	})
	common.ApiSuccess(c, buildMaskedTokenResponse(token))
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}

func GetTokenKeysBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(tokenBatch.Ids) > 100 {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": 100})
		return
	}
	userId := c.GetInt("id")
	tokens, err := model.GetTokenKeysByIds(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	keysMap := make(map[int]string)
	for _, t := range tokens {
		keysMap[t.Id] = t.GetFullKey()
	}
	common.ApiSuccess(c, gin.H{"keys": keysMap})
}
