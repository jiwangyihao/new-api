package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ---- Shared types ----

type SubscriptionPlanDTO struct {
	Plan model.SubscriptionPlan `json:"plan"`
}

type PublicSubscriptionPlan struct {
	Id                       int                                 `json:"id"`
	Title                    string                              `json:"title"`
	Subtitle                 string                              `json:"subtitle"`
	PriceAmount              float64                             `json:"price_amount"`
	Currency                 string                              `json:"currency"`
	DurationUnit             string                              `json:"duration_unit"`
	DurationValue            int                                 `json:"duration_value"`
	CustomSeconds            int64                               `json:"custom_seconds"`
	MonthlyTokenLimit        int64                               `json:"monthly_token_limit"`
	ConcurrencyLimit         int                                 `json:"concurrency_limit"`
	GPTAbuseWarningLimit     int                                 `json:"gpt_abuse_warning_limit"`
	PublicVisible            bool                                `json:"public_visible"`
	QueueCapacity            int                                 `json:"queue_capacity"`
	KyrenProductId           string                              `json:"kyren_product_id"`
	ChannelCreditEquivalents []model.PlanChannelCreditEquivalent `json:"channel_credit_equivalents" gorm:"-"`
	ChannelTokenEquivalents  []model.PlanChannelTokenEquivalent  `json:"channel_token_equivalents" gorm:"-"`
}

type PublicSubscriptionPlanDTO struct {
	Plan PublicSubscriptionPlan `json:"plan"`
}

func toPublicSubscriptionPlan(p model.SubscriptionPlan) PublicSubscriptionPlanDTO {
	return PublicSubscriptionPlanDTO{
		Plan: PublicSubscriptionPlan{
			Id:                       p.Id,
			Title:                    p.Title,
			Subtitle:                 p.Subtitle,
			PriceAmount:              p.PriceAmount,
			Currency:                 p.Currency,
			DurationUnit:             p.DurationUnit,
			DurationValue:            p.DurationValue,
			CustomSeconds:            p.CustomSeconds,
			MonthlyTokenLimit:        p.MonthlyTokenLimit,
			ConcurrencyLimit:         p.ConcurrencyLimit,
			GPTAbuseWarningLimit:     p.GPTAbuseWarningLimit,
			QueueCapacity:            p.QueueCapacity,
			PublicVisible:            p.PublicVisible,
			KyrenProductId:           p.KyrenProductId,
			ChannelCreditEquivalents: p.ChannelCreditEquivalents,
			ChannelTokenEquivalents:  p.ChannelTokenEquivalents,
		},
	}
}

type BillingPreferenceRequest struct {
	BillingPreference string `json:"billing_preference"`
}

type ActiveSubscriptionRequest struct {
	SubscriptionId int `json:"subscription_id"`
}

type CodexProModeRequest struct {
	Mode string `json:"mode"`
}

func normalizeSubscriptionPlanCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return "CNY"
	}
	return currency
}

// ---- User APIs ----

func GetSubscriptionPlans(c *gin.Context) {
	var plans []model.SubscriptionPlan
	if err := model.DB.Where("enabled = ? AND public_visible = ? AND is_trial = ?", true, true, false).Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	groups, err := model.ListEnabledChannelTokenBillingMultiplierGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]SubscriptionPlanDTO, 0, len(plans))
	for _, p := range plans {
		if err := model.PopulateSubscriptionPlanChannelTokenEquivalents(&p, groups); err != nil {
			common.ApiError(c, err)
			return
		}
		result = append(result, SubscriptionPlanDTO{
			Plan: p,
		})
	}
	common.ApiSuccess(c, result)
}

func GetPublicSubscriptionPlans(c *gin.Context) {
	var plans []model.SubscriptionPlan
	if err := model.DB.Where("enabled = ? AND public_visible = ? AND is_trial = ?", true, true, false).Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	groups, err := model.ListEnabledChannelTokenBillingMultiplierGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]PublicSubscriptionPlanDTO, 0, len(plans))
	for _, p := range plans {
		if err := model.PopulateSubscriptionPlanChannelTokenEquivalents(&p, groups); err != nil {
			common.ApiError(c, err)
			return
		}
		result = append(result, toPublicSubscriptionPlan(p))
	}
	common.ApiSuccess(c, result)
}

func GetSubscriptionSelf(c *gin.Context) {
	userId := c.GetInt("id")
	settingMap, _ := model.GetUserSetting(userId, false)
	pref := common.NormalizeBillingPreference(settingMap.BillingPreference)
	mode := common.NormalizeCodexProMode(settingMap.CodexProMode)
	codexProEligible, codexProUnavailableReason, err := model.GetCodexProEligibility(userId, settingMap)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Get all subscriptions (including expired)
	allSubscriptions, err := model.GetAllUserSubscriptions(userId)
	if err != nil {
		allSubscriptions = []model.SubscriptionSummary{}
	}

	// Get active subscriptions for backward compatibility
	activeSubscriptions, err := model.GetAllActiveUserSubscriptions(userId)
	if err != nil {
		activeSubscriptions = []model.SubscriptionSummary{}
	}

	groups, err := model.ListEnabledChannelCreditBillingGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.PopulateSubscriptionSummaryPlanChannelCreditEquivalents(allSubscriptions, groups); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.PopulateSubscriptionSummaryPlanChannelCreditEquivalents(activeSubscriptions, groups); err != nil {
		common.ApiError(c, err)
		return
	}
	summary, err := model.GetSubscriptionSelfSummary(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if summary.ActiveCount > 0 {
		summary.ChannelCreditEquivalents, err = model.BuildSubscriptionChannelCreditEquivalents(summary.TokenLimit, summary.TokenUsed, summary.TokenUnlimited, groups)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		summary.ChannelTokenEquivalents = summary.ChannelCreditEquivalents
	} else {
		summary.ChannelCreditEquivalents = []model.SubscriptionChannelCreditEquivalent{}
		summary.ChannelTokenEquivalents = []model.SubscriptionChannelTokenEquivalent{}
	}

	common.ApiSuccess(c, gin.H{
		"active_subscription_id":       settingMap.ActiveSubscriptionId,
		"billing_preference":           pref,
		"codex_pro_mode":               mode,
		"codex_pro_eligible":           codexProEligible,
		"codex_pro_unavailable_reason": codexProUnavailableReason,
		"subscriptions":                model.BuildPublicSubscriptionSummaries(activeSubscriptions, settingMap.ActiveSubscriptionId), // all active subscriptions
		"all_subscriptions":            model.BuildPublicSubscriptionSummaries(allSubscriptions, settingMap.ActiveSubscriptionId),    // all subscriptions including expired
		"summary":                      summary,
	})
}

func UpdateSubscriptionPreference(c *gin.Context) {
	userId := c.GetInt("id")
	var req BillingPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	pref := common.NormalizeBillingPreference(req.BillingPreference)

	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	current := user.GetSetting()
	current.BillingPreference = pref
	if _, err := model.SaveUserSetting(userId, current); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"billing_preference": pref})
}

func SetActiveSubscription(c *gin.Context) {
	userId := c.GetInt("id")
	var req ActiveSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.SubscriptionId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := model.SetUserActiveSubscription(userId, req.SubscriptionId); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"active_subscription_id": req.SubscriptionId})
}

func UpdateCodexProMode(c *gin.Context) {
	userId := c.GetInt("id")
	var req CodexProModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := common.ValidateCodexProModeForUpdate(req.Mode); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	mode := strings.TrimSpace(req.Mode)

	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	current := user.GetSetting()
	current.CodexProMode = mode
	if _, err := model.SaveUserSetting(userId, current); err != nil {
		common.ApiError(c, err)
		return
	}
	codexProEligible, codexProUnavailableReason, err := model.GetCodexProEligibility(userId, current)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"codex_pro_mode":               mode,
		"codex_pro_eligible":           codexProEligible,
		"codex_pro_unavailable_reason": codexProUnavailableReason,
	})
}

func ResetSubscriptionQuota(c *gin.Context) {
	userId := c.GetInt("id")
	subscriptionId, err := strconv.Atoi(c.Param("id"))
	if err != nil || subscriptionId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	result, err := model.ResetUserSubscriptionQuota(userId, subscriptionId)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, result)
}

// ---- Admin APIs ----

func AdminListSubscriptionPlans(c *gin.Context) {
	var plans []model.SubscriptionPlan
	if err := model.DB.Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	groups, err := model.ListEnabledChannelCreditBillingGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]SubscriptionPlanDTO, 0, len(plans))
	for _, p := range plans {
		if err := model.PopulateSubscriptionPlanChannelCreditEquivalents(&p, groups); err != nil {
			common.ApiError(c, err)
			return
		}
		result = append(result, SubscriptionPlanDTO{
			Plan: p,
		})
	}
	common.ApiSuccess(c, result)
}

type AdminUpsertSubscriptionPlanRequest struct {
	Plan model.SubscriptionPlan `json:"plan"`
}

func AdminCreateSubscriptionPlan(c *gin.Context) {
	var req AdminUpsertSubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	req.Plan.Id = 0
	if strings.TrimSpace(req.Plan.Title) == "" {
		common.ApiErrorMsg(c, "套餐标题不能为空")
		return
	}
	if req.Plan.PriceAmount < 0 {
		common.ApiErrorMsg(c, "价格不能为负数")
		return
	}
	if req.Plan.PriceAmount > 9999 {
		common.ApiErrorMsg(c, "价格不能超过9999")
		return
	}
	req.Plan.Currency = normalizeSubscriptionPlanCurrency(req.Plan.Currency)
	if req.Plan.DurationUnit == "" {
		req.Plan.DurationUnit = model.SubscriptionDurationMonth
	}
	if req.Plan.DurationValue <= 0 && req.Plan.DurationUnit != model.SubscriptionDurationCustom {
		req.Plan.DurationValue = 1
	}
	if req.Plan.MaxPurchasePerUser < 0 {
		common.ApiErrorMsg(c, "购买上限不能为负数")
		return
	}
	if req.Plan.TotalAmount < 0 {
		common.ApiErrorMsg(c, "总额度不能为负数")
		return
	}
	if req.Plan.GPTAbuseWarningLimit < 0 {
		common.ApiErrorMsg(c, "安全警告次数不能为负数")
		return
	}
	req.Plan.UpgradeGroup = ""
	if req.Plan.QueueCapacity < 0 {
		common.ApiErrorMsg(c, "排队容量不能为负数")
		return
	}
	req.Plan.QuotaResetPeriod = model.NormalizeResetPeriod(req.Plan.QuotaResetPeriod)
	if req.Plan.QuotaResetPeriod == model.SubscriptionResetCustom && req.Plan.QuotaResetCustomSeconds <= 0 {
		common.ApiErrorMsg(c, "自定义重置周期需大于0秒")
		return
	}
	err := model.DB.Create(&req.Plan).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(req.Plan.Id)
	common.ApiSuccess(c, req.Plan)
}

func AdminUpdateSubscriptionPlan(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	var req AdminUpsertSubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if strings.TrimSpace(req.Plan.Title) == "" {
		common.ApiErrorMsg(c, "套餐标题不能为空")
		return
	}
	if req.Plan.PriceAmount < 0 {
		common.ApiErrorMsg(c, "价格不能为负数")
		return
	}
	if req.Plan.PriceAmount > 9999 {
		common.ApiErrorMsg(c, "价格不能超过9999")
		return
	}
	req.Plan.Id = id
	req.Plan.Currency = normalizeSubscriptionPlanCurrency(req.Plan.Currency)
	if req.Plan.DurationUnit == "" {
		req.Plan.DurationUnit = model.SubscriptionDurationMonth
	}
	if req.Plan.DurationValue <= 0 && req.Plan.DurationUnit != model.SubscriptionDurationCustom {
		req.Plan.DurationValue = 1
	}
	if req.Plan.MaxPurchasePerUser < 0 {
		common.ApiErrorMsg(c, "购买上限不能为负数")
		return
	}
	if req.Plan.TotalAmount < 0 {
		common.ApiErrorMsg(c, "总额度不能为负数")
		return
	}
	if req.Plan.GPTAbuseWarningLimit < 0 {
		common.ApiErrorMsg(c, "安全警告次数不能为负数")
		return
	}

	req.Plan.UpgradeGroup = ""
	if req.Plan.QueueCapacity < 0 {
		common.ApiErrorMsg(c, "排队容量不能为负数")
		return
	}
	req.Plan.QuotaResetPeriod = model.NormalizeResetPeriod(req.Plan.QuotaResetPeriod)
	if req.Plan.QuotaResetPeriod == model.SubscriptionResetCustom && req.Plan.QuotaResetCustomSeconds <= 0 {
		common.ApiErrorMsg(c, "自定义重置周期需大于0秒")
		return
	}

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// update plan (allow zero values updates with map)
		updateMap := map[string]interface{}{
			"title":                      req.Plan.Title,
			"subtitle":                   req.Plan.Subtitle,
			"price_amount":               req.Plan.PriceAmount,
			"currency":                   req.Plan.Currency,
			"duration_unit":              req.Plan.DurationUnit,
			"duration_value":             req.Plan.DurationValue,
			"custom_seconds":             req.Plan.CustomSeconds,
			"enabled":                    req.Plan.Enabled,
			"sort_order":                 req.Plan.SortOrder,
			"stripe_price_id":            req.Plan.StripePriceId,
			"creem_product_id":           req.Plan.CreemProductId,
			"kyren_product_id":           req.Plan.KyrenProductId,
			"max_purchase_per_user":      req.Plan.MaxPurchasePerUser,
			"total_amount":               req.Plan.TotalAmount,
			"upgrade_group":              "",
			"quota_reset_period":         req.Plan.QuotaResetPeriod,
			"quota_reset_custom_seconds": req.Plan.QuotaResetCustomSeconds,
			"gpt_abuse_warning_limit":    req.Plan.GPTAbuseWarningLimit,
			"monthly_token_limit":        req.Plan.MonthlyTokenLimit,
			"concurrency_limit":          req.Plan.ConcurrencyLimit,
			"queue_capacity":             req.Plan.QueueCapacity,
			"is_trial":                   req.Plan.IsTrial,
			"invite_trial":               req.Plan.InviteTrial,
			"public_visible":             req.Plan.PublicVisible,
			"trial_duration_hours":       req.Plan.TrialDurationHours,
			"reward_eligible":            req.Plan.RewardEligible,
			"business_code":              req.Plan.BusinessCode,
			"updated_at":                 common.GetTimestamp(),
		}
		if err := tx.Model(&model.SubscriptionPlan{}).Where("id = ?", id).Updates(updateMap).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(id)
	common.ApiSuccess(c, nil)
}

type AdminUpdateSubscriptionPlanStatusRequest struct {
	Enabled *bool `json:"enabled"`
}

func AdminUpdateSubscriptionPlanStatus(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	var req AdminUpdateSubscriptionPlanStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", id).Update("enabled", *req.Enabled).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(id)
	common.ApiSuccess(c, nil)
}

type AdminBindSubscriptionRequest struct {
	UserId int `json:"user_id"`
	PlanId int `json:"plan_id"`
}

func AdminBindSubscription(c *gin.Context) {
	var req AdminBindSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UserId <= 0 || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	msg, err := model.AdminBindSubscription(req.UserId, req.PlanId, "")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if msg != "" {
		common.ApiSuccess(c, gin.H{"message": msg})
		return
	}
	common.ApiSuccess(c, nil)
}

// ---- Admin: user subscription management ----

func AdminListUserSubscriptions(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	if userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}
	subs, err := model.GetAllUserSubscriptions(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, subs)
}

type AdminCreateUserSubscriptionRequest struct {
	PlanId int `json:"plan_id"`
}

// AdminCreateUserSubscription creates a new user subscription from a plan (no payment).
func AdminCreateUserSubscription(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	if userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}
	var req AdminCreateUserSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	msg, err := model.AdminBindSubscription(userId, req.PlanId, "")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if msg != "" {
		common.ApiSuccess(c, gin.H{"message": msg})
		return
	}
	common.ApiSuccess(c, nil)
}

// AdminInvalidateUserSubscription cancels a user subscription immediately.
func AdminInvalidateUserSubscription(c *gin.Context) {
	subId, _ := strconv.Atoi(c.Param("id"))
	if subId <= 0 {
		common.ApiErrorMsg(c, "无效的订阅ID")
		return
	}
	msg, err := model.AdminInvalidateUserSubscription(subId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if msg != "" {
		common.ApiSuccess(c, gin.H{"message": msg})
		return
	}
	common.ApiSuccess(c, nil)
}

// AdminDeleteUserSubscription hard-deletes a user subscription.
func AdminDeleteUserSubscription(c *gin.Context) {
	subId, _ := strconv.Atoi(c.Param("id"))
	if subId <= 0 {
		common.ApiErrorMsg(c, "无效的订阅ID")
		return
	}
	msg, err := model.AdminDeleteUserSubscription(subId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if msg != "" {
		common.ApiSuccess(c, gin.H{"message": msg})
		return
	}
	common.ApiSuccess(c, nil)
}
