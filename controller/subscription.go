package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---- Shared types ----

type SubscriptionPlanDTO struct {
	Plan model.SubscriptionPlan `json:"plan"`
}

type AdminSubscriptionPlanDTO struct {
	Plan                          model.SubscriptionPlan `json:"plan"`
	ExistingTimedEntitlementCount int64                  `json:"existing_timed_entitlement_count"`
}

type PublicSubscriptionPlan struct {
	Id                       int                                 `json:"id"`
	Title                    string                              `json:"title"`
	Subtitle                 string                              `json:"subtitle"`
	PriceAmount              float64                             `json:"price_amount"`
	PriceAmountMicros        *int64                              `json:"price_amount_micros,string"`
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
			PriceAmountMicros:        p.PriceAmountMicros,
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

type SubscriptionBillingStrategyRequest struct {
	BillingStrategy string `json:"billing_strategy"`
}

type ActiveSubscriptionRequest struct {
	SubscriptionId int `json:"subscription_id"`
}

type CodexProModeRequest struct {
	Mode string `json:"mode"`
}

type SubscriptionOrderStatusResponse struct {
	TradeNo         string                          `json:"trade_no"`
	PlanId          int                             `json:"plan_id"`
	PaymentProvider string                          `json:"payment_provider"`
	PaymentMethod   string                          `json:"payment_method"`
	PurchaseMode    string                          `json:"purchase_mode"`
	Status          string                          `json:"status"`
	CreateTime      int64                           `json:"create_time"`
	CompleteTime    int64                           `json:"complete_time"`
	CreditBalance   *model.CreditBalanceGrantResult `json:"credit_balance,omitempty"`
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
	if err := model.DB.Where("enabled = ? AND public_visible = ? AND is_trial = ? AND entitlement_type = ?", true, true, false, model.SubscriptionEntitlementTimed).Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
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
	if err := model.DB.Where("enabled = ? AND public_visible = ? AND is_trial = ? AND entitlement_type = ?", true, true, false, model.SubscriptionEntitlementTimed).Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
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

func GetCreditBalanceLedger(c *gin.Context) {
	entries, err := model.ListCreditBalanceLedgerFiltered(model.CreditBalanceLedgerFilter{
		UserId: c.GetInt("id"), SourceType: c.Query("source_type"), Type: c.Query("type"),
		StartTime: parseInt64Query(c.Query("start_time")), EndTime: parseInt64Query(c.Query("end_time")), Limit: 100,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, entries)
}

func GetSubscriptionConversionQuotes(c *gin.Context) {
	quotes, err := model.ListTimedSubscriptionConversionQuotes(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, toSubscriptionConversionQuoteListResponse(quotes))
}
func GetSubscriptionOrderStatus(c *gin.Context) {
	userId := c.GetInt("id")
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	if userId <= 0 || tradeNo == "" {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "订单不存在"})
		return
	}
	var order model.SubscriptionOrder
	if err := model.DB.Select("id", "user_id", "plan_id", "trade_no", "payment_method", "payment_provider", "status", "create_time", "complete_time", "entitlement_snapshot").
		Where("user_id = ? AND trade_no = ?", userId, tradeNo).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "订单不存在"})
			return
		}
		common.ApiError(c, err)
		return
	}
	purchaseMode := model.SubscriptionPurchaseModeTimed
	var snapshot model.SubscriptionEntitlementSnapshot
	if strings.TrimSpace(order.EntitlementSnapshot) != "" {
		var err error
		snapshot, err = model.UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if strings.TrimSpace(snapshot.PurchaseMode) != "" {
			purchaseMode, err = model.NormalizeSubscriptionPurchaseMode(snapshot.PurchaseMode)
			if err != nil {
				common.ApiError(c, err)
				return
			}
		}
	}
	response := SubscriptionOrderStatusResponse{
		TradeNo:         order.TradeNo,
		PlanId:          order.PlanId,
		PaymentProvider: order.PaymentProvider,
		PaymentMethod:   order.PaymentMethod,
		PurchaseMode:    purchaseMode,
		Status:          order.Status,
		CreateTime:      order.CreateTime,
		CompleteTime:    order.CompleteTime,
	}
	if order.Status == common.TopUpStatusSuccess && purchaseMode == model.SubscriptionPurchaseModeCreditBalance {
		var ledger model.CreditBalanceLedger
		if err := model.DB.Where("user_id = ? AND source_type = ? AND source_id = ?", userId, model.CreditBalanceLedgerSourceSubscriptionOrder, order.Id).First(&ledger).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		status := model.CreditBalanceStatusExhausted
		if ledger.SettlementDebtAfter > 0 {
			status = model.CreditBalanceStatusDebt
		} else if ledger.AvailableCreditAfter > 0 {
			status = model.CreditBalanceStatusAvailable
		}
		settingMap, _ := model.GetUserSetting(userId, false)
		response.CreditBalance = &model.CreditBalanceGrantResult{
			UserSubscriptionId: ledger.UserSubscriptionId,
			PlanId:             snapshot.TargetCreditBalancePlanID,
			GrossCredit:        ledger.GrossCredit,
			DebtOffset:         ledger.DebtOffset,
			AvailableCredit:    ledger.AvailableCreditAfter,
			SettlementDebt:     ledger.SettlementDebtAfter,
			BalanceBefore:      ledger.BalanceBefore,
			BalanceAfter:       ledger.BalanceAfter,
			Active:             settingMap.ActiveSubscriptionId == ledger.UserSubscriptionId,
			LedgerId:           ledger.Id,
			Status:             status,
		}
	}
	common.ApiSuccess(c, response)
}

func GetSubscriptionSelf(c *gin.Context) {
	userId := c.GetInt("id")
	settingMap, _ := model.GetUserSetting(userId, false)
	pref := common.NormalizeBillingPreference(settingMap.BillingPreference)
	mode := common.NormalizeCodexProMode(settingMap.CodexProMode)
	codexProEligible, codexProUnavailableReason, err := model.GetCodexProEligibility(userId, settingMap)
	if common.CodexProFeaturesHidden {
		mode = common.CodexProModeOff
		codexProEligible = false
		codexProUnavailableReason = common.CodexProUnavailableReasonFeaturesHidden
	}
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

	effectiveActiveSubscriptionId := summary.ActiveSubscriptionId
	creditBalance, _, err := model.CreditBalanceStateForUser(userId, effectiveActiveSubscriptionId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ledger := []model.CreditBalanceLedgerHistoryItem{}
	if creditBalance != nil {
		ledger, err = model.ListCreditBalanceLedger(userId, 100)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	var creditBalancePlan any
	creditBalancePurchaseEnabled := false
	if plan, planErr := model.GetCreditBalancePlanTx(model.DB); planErr == nil {
		creditBalancePurchaseEnabled = plan.Enabled && plan.CreditBalanceConfigured && plan.CreditBalancePurchaseEnabled
		creditBalancePlan = gin.H{
			"concurrency_limit": plan.ConcurrencyLimit,
			"queue_capacity":    plan.QueueCapacity,
		}
	} else if !errors.Is(planErr, gorm.ErrRecordNotFound) {
		common.ApiError(c, planErr)
		return
	}
	common.ApiSuccess(c, gin.H{
		"active_subscription_id":             effectiveActiveSubscriptionId,
		"billing_preference":                 pref,
		"billing_strategy":                   summary.BillingStrategy,
		"billing_candidate_subscription_ids": summary.BillingCandidateIds,
		"last_subscription_purchase_mode":    settingMap.LastSubscriptionPurchaseMode,
		"codex_pro_mode":                     mode,
		"codex_pro_eligible":                 codexProEligible,
		"codex_pro_features_hidden":          common.CodexProFeaturesHidden,
		"codex_pro_unavailable_reason":       codexProUnavailableReason,
		"subscriptions":                      model.BuildPublicSubscriptionSummaries(activeSubscriptions, effectiveActiveSubscriptionId),
		"all_subscriptions":                  model.BuildPublicSubscriptionSummaries(allSubscriptions, effectiveActiveSubscriptionId),
		"summary":                            summary,
		"credit_balance":                     creditBalance,
		"credit_balance_ledger":              ledger,
		"credit_balance_purchase_enabled":    creditBalancePurchaseEnabled,
		"credit_balance_plan":                creditBalancePlan,
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
	if _, err := model.MutateUserSetting(userId, func(setting *dto.UserSetting) error {
		setting.BillingPreference = pref
		return nil
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"billing_preference": pref})
}

func UpdateSubscriptionBillingStrategy(c *gin.Context) {
	userId := c.GetInt("id")
	var req SubscriptionBillingStrategyRequest
	if err := c.ShouldBindJSON(&req); err != nil || model.ValidateSubscriptionBillingStrategy(req.BillingStrategy) != nil {
		common.ApiErrorMsg(c, "无效的套餐扣费策略")
		return
	}
	strategy := model.NormalizeSubscriptionBillingStrategy(req.BillingStrategy)
	if _, err := model.MutateUserSetting(userId, func(setting *dto.UserSetting) error {
		setting.SubscriptionBillingStrategy = strategy
		return nil
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"billing_strategy": strategy})
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
	if common.CodexProFeaturesHidden {
		common.ApiSuccess(c, gin.H{
			"codex_pro_mode":               common.CodexProModeOff,
			"codex_pro_eligible":           false,
			"codex_pro_unavailable_reason": common.CodexProUnavailableReasonFeaturesHidden,
			"codex_pro_features_hidden":    true,
		})
		return
	}
	if err := common.ValidateCodexProModeForUpdate(req.Mode); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	mode := strings.TrimSpace(req.Mode)
	current, err := model.MutateUserSetting(userId, func(setting *dto.UserSetting) error {
		setting.CodexProMode = mode
		return nil
	})
	if err != nil {
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
		"codex_pro_features_hidden":    false,
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

func activeTimedEntitlementsQuery(db *gorm.DB, now int64) *gorm.DB {
	graceStart := now - model.TimedSubscriptionConversionGraceSeconds
	if graceStart < 0 {
		graceStart = 0
	}
	return db.Model(&model.UserSubscription{}).Where(
		"entitlement_type = ? AND ((status = ? AND end_time >= ?) OR (status = ? AND end_time >= ? AND end_time <= ?))",
		model.SubscriptionEntitlementTimed,
		model.SubscriptionStatusActive, graceStart,
		model.SubscriptionStatusExpired, graceStart, now,
	)
}

func AdminListSubscriptionPlans(c *gin.Context) {
	var plans []model.SubscriptionPlan
	if err := model.DB.Where("entitlement_type = ?", model.SubscriptionEntitlementTimed).Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	type entitlementCount struct {
		PlanId int   `gorm:"column:plan_id"`
		Count  int64 `gorm:"column:entitlement_count"`
	}
	var countRows []entitlementCount
	if err := activeTimedEntitlementsQuery(model.DB, model.GetDBTimestamp()).
		Select("plan_id, COUNT(*) AS entitlement_count").
		Group("plan_id").Scan(&countRows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	counts := make(map[int]int64, len(countRows))
	for _, row := range countRows {
		counts[row.PlanId] = row.Count
	}
	groups, err := model.ListEnabledChannelCreditBillingGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]AdminSubscriptionPlanDTO, 0, len(plans))
	for _, p := range plans {
		if err := model.PopulateSubscriptionPlanChannelCreditEquivalents(&p, groups); err != nil {
			common.ApiError(c, err)
			return
		}
		result = append(result, AdminSubscriptionPlanDTO{
			Plan: p, ExistingTimedEntitlementCount: counts[p.Id],
		})
	}
	common.ApiSuccess(c, result)
}

type AdminUpsertSubscriptionPlanRequest struct {
	Plan          model.SubscriptionPlan `json:"plan"`
	RiskConfirmed bool                   `json:"risk_confirmed"`
	RiskReason    string                 `json:"risk_reason"`
	PriceProvided bool                   `json:"-"`
}

type rawAdminUpsertSubscriptionPlanRequest struct {
	Plan          json.RawMessage `json:"plan"`
	RiskConfirmed bool            `json:"risk_confirmed"`
	RiskReason    string          `json:"risk_reason"`
}

func decodeAdminUpsertSubscriptionPlanRequest(c *gin.Context) (AdminUpsertSubscriptionPlanRequest, error) {
	var rawRequest rawAdminUpsertSubscriptionPlanRequest
	if err := common.DecodeJson(c.Request.Body, &rawRequest); err != nil || common.GetJsonType(rawRequest.Plan) != "object" {
		return AdminUpsertSubscriptionPlanRequest{}, model.ErrSubscriptionPlanPriceInvalid
	}
	var rawPlan map[string]json.RawMessage
	if err := common.Unmarshal(rawRequest.Plan, &rawPlan); err != nil {
		return AdminUpsertSubscriptionPlanRequest{}, model.ErrSubscriptionPlanPriceInvalid
	}
	displayRaw, displayProvided := rawPlan["price_amount"]
	microsRaw, microsProvided := rawPlan["price_amount_micros"]
	if microsProvided && common.GetJsonType(microsRaw) != "string" {
		return AdminUpsertSubscriptionPlanRequest{}, model.ErrSubscriptionPlanPriceInvalid
	}
	price, err := model.NormalizeSubscriptionPlanPrice(model.SubscriptionPlanPriceInput{
		DisplayAmount:         common.JsonRawMessageToString(displayRaw),
		DisplayAmountProvided: displayProvided,
		AmountMicros:          common.JsonRawMessageToString(microsRaw),
		AmountMicrosProvided:  microsProvided,
	})
	if err != nil {
		return AdminUpsertSubscriptionPlanRequest{}, err
	}
	delete(rawPlan, "price_amount")
	delete(rawPlan, "price_amount_micros")
	planPayload, err := common.Marshal(rawPlan)
	if err != nil {
		return AdminUpsertSubscriptionPlanRequest{}, model.ErrSubscriptionPlanPriceInvalid
	}
	request := AdminUpsertSubscriptionPlanRequest{
		RiskConfirmed: rawRequest.RiskConfirmed,
		RiskReason:    rawRequest.RiskReason,
		PriceProvided: displayProvided || microsProvided,
	}
	if err := common.Unmarshal(planPayload, &request.Plan); err != nil {
		return AdminUpsertSubscriptionPlanRequest{}, model.ErrSubscriptionPlanPriceInvalid
	}
	request.Plan.PriceAmount = price.DisplayAmount
	request.Plan.PriceAmountMicros = price.AmountMicros
	return request, nil
}

func respondSubscriptionPlanPriceError(c *gin.Context, err error) {
	message := "套餐价格无效"
	switch {
	case errors.Is(err, model.ErrSubscriptionPlanPriceRequired):
		message = "有价套餐必须提供精确微单位价格"
	case errors.Is(err, model.ErrSubscriptionPlanPriceNegative):
		message = "价格不能为负数"
	case errors.Is(err, model.ErrSubscriptionPlanPricePrecision):
		message = "价格最多支持六位小数"
	case errors.Is(err, model.ErrCreditValuationOverflow):
		message = "价格超出支持范围"
	case errors.Is(err, model.ErrSubscriptionPlanPriceMismatch):
		message = "精确价格与兼容价格不一致"
	}
	c.JSON(http.StatusOK, gin.H{"success": false, "message": message, "code": err.Error()})
}

type AdminUpdateCreditBalancePlanRequest struct {
	ConcurrencyLimit  int    `json:"concurrency_limit"`
	QueueCapacity     int    `json:"queue_capacity"`
	BusinessCode      string `json:"business_code"`
	Configured        bool   `json:"configured"`
	PurchaseEnabled   bool   `json:"purchase_enabled"`
	RedemptionEnabled bool   `json:"redemption_enabled"`
	ConversionEnabled bool   `json:"conversion_enabled"`
	ValuationCurrency string `json:"valuation_currency"`
}

func getCreditBalancePlan() (*model.SubscriptionPlan, error) {
	var plan model.SubscriptionPlan
	if err := model.DB.Where("entitlement_type = ?", model.SubscriptionEntitlementCreditBalance).First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func AdminGetCreditBalancePlan(c *gin.Context) {
	plan, err := getCreditBalancePlan()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plan)
}

func AdminUpdateCreditBalancePlan(c *gin.Context) {
	var req AdminUpdateCreditBalancePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if req.ConcurrencyLimit < 0 {
		common.ApiErrorMsg(c, "并发上限不能为负数")
		return
	}
	if req.QueueCapacity < 0 {
		common.ApiErrorMsg(c, "排队容量不能为负数")
		return
	}
	businessCode := strings.TrimSpace(req.BusinessCode)
	if req.Configured && businessCode == "" {
		common.ApiErrorMsg(c, "配置完成前必须设置 BusinessCode")
		return
	}
	if (req.PurchaseEnabled || req.RedemptionEnabled || req.ConversionEnabled) && !req.Configured {
		common.ApiErrorMsg(c, "必须先确认 Credit 余额套餐配置")
		return
	}
	currency, err := model.NormalizeCreditValuationCurrency(req.ValuationCurrency)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Credit 估值币种必须为 CNY 或 USD", "code": err.Error()})
		return
	}
	var updated model.SubscriptionPlan
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		plan, err := model.GuardCreditValuationCurrencyUpdateTx(tx, currency)
		if err != nil {
			return err
		}
		var businessCodeValue any
		if businessCode != "" {
			businessCodeValue = businessCode
		}
		updates := map[string]any{
			"model_limits":                      "",
			"valuation_currency":                currency,
			"concurrency_limit":                 req.ConcurrencyLimit,
			"queue_capacity":                    req.QueueCapacity,
			"business_code":                     businessCodeValue,
			"credit_balance_configured":         req.Configured,
			"credit_balance_purchase_enabled":   req.PurchaseEnabled,
			"credit_balance_redemption_enabled": req.RedemptionEnabled,
			"credit_balance_conversion_enabled": req.ConversionEnabled,
			"updated_at":                        common.GetTimestamp(),
		}
		if err := tx.Model(&model.SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(updates).Error; err != nil {
			return err
		}
		return tx.First(&updated, plan.Id).Error
	})
	if err != nil {
		if errors.Is(err, model.ErrCreditValuationCurrencyLocked) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "已有 Credit 估值数据，不能修改估值币种", "code": err.Error()})
			return
		}
		common.ApiError(c, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(updated.Id)
	common.ApiSuccess(c, &updated)
}

func validateTimedPlanCreditEligibility(plan *model.SubscriptionPlan) string {
	if plan == nil || (!plan.UnlimitedPurchaseEnabled && !plan.TimedConversionEnabled) {
		return ""
	}
	if plan.DurationUnit != model.SubscriptionDurationMonth || plan.DurationValue != 1 {
		return "只有期限恰好 1 个月的计时套餐可开启 Credit 资格"
	}
	if model.NormalizeResetPeriod(plan.QuotaResetPeriod) != model.SubscriptionResetMonthly {
		return "只有按月重置的计时套餐可开启 Credit 资格"
	}
	if plan.MonthlyTokenLimit <= 0 {
		return "月 Credit 必须为正且有限"
	}
	if plan.IsTrial || plan.InviteTrial {
		return "试用套餐和每月邀请套餐不能开启 Credit 资格"
	}
	return ""
}

func rejectCreditBalancePlanMutation(c *gin.Context, id int) bool {
	var plan model.SubscriptionPlan
	if err := model.DB.Select("id", "entitlement_type").First(&plan, id).Error; err != nil {
		common.ApiError(c, err)
		return true
	}
	if plan.EntitlementType == model.SubscriptionEntitlementCreditBalance {
		common.ApiErrorMsg(c, "Credit 余额套餐只能通过专用接口配置")
		return true
	}
	return false
}

func AdminCreateSubscriptionPlan(c *gin.Context) {
	req, err := decodeAdminUpsertSubscriptionPlanRequest(c)
	if err != nil {
		respondSubscriptionPlanPriceError(c, err)
		return
	}
	if req.Plan.EntitlementType == model.SubscriptionEntitlementCreditBalance {
		common.ApiErrorMsg(c, "Credit 余额套餐只能通过专用接口配置")
		return
	}
	req.Plan.EntitlementType = model.SubscriptionEntitlementTimed
	req.Plan.ModelLimits = ""
	req.Plan.Id = 0
	if strings.TrimSpace(req.Plan.Title) == "" {
		common.ApiErrorMsg(c, "套餐标题不能为空")
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
	if message := validateTimedPlanCreditEligibility(&req.Plan); message != "" {
		common.ApiErrorMsg(c, message)
		return
	}
	err = model.DB.Create(&req.Plan).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(req.Plan.Id)
	common.ApiSuccess(c, req.Plan)
}

var errSubscriptionPlanCreditRiskConfirmationRequired = errors.New("修改存在存量权益的套餐月 Credit 需要确认续期归并风险并填写原因")

func AdminUpdateSubscriptionPlan(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	if rejectCreditBalancePlanMutation(c, id) {
		return
	}
	req, err := decodeAdminUpsertSubscriptionPlanRequest(c)
	if err != nil {
		respondSubscriptionPlanPriceError(c, err)
		return
	}
	if strings.TrimSpace(req.Plan.Title) == "" {
		common.ApiErrorMsg(c, "套餐标题不能为空")
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
	req.Plan.ModelLimits = ""
	if req.Plan.QueueCapacity < 0 {
		common.ApiErrorMsg(c, "排队容量不能为负数")
		return
	}
	req.Plan.QuotaResetPeriod = model.NormalizeResetPeriod(req.Plan.QuotaResetPeriod)
	if req.Plan.QuotaResetPeriod == model.SubscriptionResetCustom && req.Plan.QuotaResetCustomSeconds <= 0 {
		common.ApiErrorMsg(c, "自定义重置周期需大于0秒")
		return
	}
	if message := validateTimedPlanCreditEligibility(&req.Plan); message != "" {
		common.ApiErrorMsg(c, message)
		return
	}

	var previousMonthlyCredit int64
	var existingTimedEntitlementCount int64
	riskSnapshotTime := model.GetDBTimestamp()
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var current model.SubscriptionPlan
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "title", "entitlement_type", "monthly_token_limit").First(&current, id).Error; err != nil {
			return err
		}
		if current.EntitlementType == model.SubscriptionEntitlementCreditBalance {
			return errors.New("Credit 余额套餐只能通过专用接口配置")
		}
		previousMonthlyCredit = current.MonthlyTokenLimit
		if previousMonthlyCredit != req.Plan.MonthlyTokenLimit {
			if err := activeTimedEntitlementsQuery(tx, riskSnapshotTime).
				Where("plan_id = ?", id).
				Count(&existingTimedEntitlementCount).Error; err != nil {
				return err
			}
			if existingTimedEntitlementCount > 0 && (!req.RiskConfirmed || strings.TrimSpace(req.RiskReason) == "") {
				return errSubscriptionPlanCreditRiskConfirmationRequired
			}
		}
		// update plan (allow zero values updates with map)
		updateMap := map[string]interface{}{
			"title":                      req.Plan.Title,
			"subtitle":                   req.Plan.Subtitle,
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
			"model_limits":               "",
			"queue_capacity":             req.Plan.QueueCapacity,
			"is_trial":                   req.Plan.IsTrial,
			"invite_trial":               req.Plan.InviteTrial,
			"public_visible":             req.Plan.PublicVisible,
			"trial_duration_hours":       req.Plan.TrialDurationHours,
			"reward_eligible":            req.Plan.RewardEligible,
			"business_code":              req.Plan.BusinessCode,
			"unlimited_purchase_enabled": req.Plan.UnlimitedPurchaseEnabled,
			"timed_conversion_enabled":   req.Plan.TimedConversionEnabled,
			"updated_at":                 common.GetTimestamp(),
		}
		if req.PriceProvided {
			updateMap["price_amount"] = req.Plan.PriceAmount
			updateMap["price_amount_micros"] = req.Plan.PriceAmountMicros
		}
		if err := tx.Model(&model.SubscriptionPlan{}).Where("id = ?", id).Updates(updateMap).Error; err != nil {
			return err
		}
		if existingTimedEntitlementCount > 0 && previousMonthlyCredit != req.Plan.MonthlyTokenLimit {
			return model.CreateSubscriptionPlanCreditChangeAuditTx(tx, &model.SubscriptionPlanCreditChangeAudit{
				PlanId: id, PlanTitle: req.Plan.Title,
				AdminUserId: c.GetInt("id"), AdminUsername: c.GetString("username"),
				OldMonthlyCredit: previousMonthlyCredit, NewMonthlyCredit: req.Plan.MonthlyTokenLimit,
				ExistingTimedEntitlementCount: existingTimedEntitlementCount,
				RiskConfirmed:                 true, RiskReason: strings.TrimSpace(req.RiskReason),
			})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errSubscriptionPlanCreditRiskConfirmationRequired) {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		common.ApiError(c, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(id)
	if existingTimedEntitlementCount > 0 && previousMonthlyCredit != req.Plan.MonthlyTokenLimit {
		model.RecordLogWithAdminInfo(c.GetInt("id"), model.LogTypeManage,
			"subscription plan monthly Credit risk confirmed plan_id="+strconv.Itoa(id), map[string]any{
				"admin_id": c.GetInt("id"), "admin_username": c.GetString("username"),
				"plan_id": id, "plan_title": req.Plan.Title,
				"old_monthly_credit": previousMonthlyCredit, "new_monthly_credit": req.Plan.MonthlyTokenLimit,
				"existing_timed_entitlement_count": existingTimedEntitlementCount,
				"risk_confirmed":                   true, "risk_reason": strings.TrimSpace(req.RiskReason),
			})
	}
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
	if rejectCreditBalancePlanMutation(c, id) {
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
	UserId         int    `json:"user_id"`
	PlanId         int    `json:"plan_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
}

func AdminBindSubscription(c *gin.Context) {
	var req AdminBindSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UserId <= 0 || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	msg, err := model.AdminBindSubscription(model.AdminTimedSubscriptionGrantRequest{
		UserId: req.UserId, PlanId: req.PlanId, IdempotencyKey: req.IdempotencyKey, Reason: req.Reason,
	})
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

type adminSubscriptionConversionAuditResponse struct {
	ConversionId         string `json:"conversion_id"`
	SourceSubscriptionId string `json:"source_subscription_id"`
	TargetSubscriptionId string `json:"target_subscription_id"`
	SourceStatusBefore   string `json:"source_status_before"`
	SourceStatusAfter    string `json:"source_status_after"`
	TargetStatus         string `json:"target_status"`
	ConvertedAt          string `json:"converted_at"`
}

type adminUserSubscriptionResponse struct {
	Subscription    *model.UserSubscription                   `json:"subscription"`
	Plan            *model.SubscriptionPlan                   `json:"plan,omitempty"`
	ConversionAudit *adminSubscriptionConversionAuditResponse `json:"conversion_audit,omitempty"`
}

func toAdminUserSubscriptionResponses(input []model.AdminSubscriptionSummary) []adminUserSubscriptionResponse {
	output := make([]adminUserSubscriptionResponse, 0, len(input))
	for _, summary := range input {
		item := adminUserSubscriptionResponse{
			Subscription: summary.Subscription,
			Plan:         summary.Plan,
		}
		if summary.ConversionAudit != nil {
			audit := summary.ConversionAudit
			item.ConversionAudit = &adminSubscriptionConversionAuditResponse{
				ConversionId:         strconv.Itoa(audit.ConversionId),
				SourceSubscriptionId: strconv.Itoa(audit.SourceSubscriptionId),
				TargetSubscriptionId: strconv.Itoa(audit.TargetSubscriptionId),
				SourceStatusBefore:   audit.SourceStatusBefore,
				SourceStatusAfter:    audit.SourceStatusAfter,
				TargetStatus:         audit.TargetStatus,
				ConvertedAt:          strconv.FormatInt(audit.ConvertedAt, 10),
			}
		}
		output = append(output, item)
	}
	return output
}

func AdminListUserSubscriptions(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	if userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}
	subs, err := model.GetAdminUserSubscriptions(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, toAdminUserSubscriptionResponses(subs))
}

type AdminCreditBalanceAdjustmentRequest struct {
	Operation      string `json:"operation"`
	Amount         int64  `json:"amount"`
	PlanId         int    `json:"plan_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
}

func AdminPreviewUserCreditBalance(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	var req AdminCreditBalanceAdjustmentRequest
	if userId <= 0 || c.ShouldBindJSON(&req) != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	result, err := service.PreviewCreditBalanceAdjustment(service.CreditBalanceAdjustmentPreviewRequest{
		UserId: userId, Operation: req.Operation, Amount: req.Amount, PlanId: req.PlanId,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func AdminAdjustUserCreditBalance(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	var req AdminCreditBalanceAdjustmentRequest
	if userId <= 0 || c.ShouldBindJSON(&req) != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	result, err := service.AdjustCreditBalance(service.CreditBalanceAdjustmentRequest{
		UserId: userId, Operation: req.Operation, Amount: req.Amount, PlanId: req.PlanId,
		IdempotencyKey: req.IdempotencyKey, OperatorUserId: c.GetInt("id"), Reason: req.Reason,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func AdminGetUserCreditBalanceLedger(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	if userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}
	entries, err := model.ListCreditBalanceLedgerFiltered(model.CreditBalanceLedgerFilter{
		UserId: userId, SourceType: c.Query("source_type"), Type: c.Query("type"),
		StartTime: parseInt64Query(c.Query("start_time")), EndTime: parseInt64Query(c.Query("end_time")), Limit: 100,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, entries)
}

func parseInt64Query(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

type AdminSubscriptionOrderRecoveryRequest struct {
	RecoveryType string `json:"recovery_type"`
	Reason       string `json:"reason"`
}

func AdminGetSubscriptionOrderRecoveryPreview(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	if userId <= 0 || tradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	preview, err := model.GetSubscriptionOrderRecoveryPreview(tradeNo, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, preview)
}

func AdminRecoverSubscriptionOrder(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	var req AdminSubscriptionOrderRecoveryRequest
	if userId <= 0 || tradeNo == "" || c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Reason) == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	result, err := service.RecoverSubscriptionOrder(service.SubscriptionOrderRecoveryRequest{
		TradeNo: tradeNo, ExpectedUserId: userId, RecoveryType: req.RecoveryType,
		OperatorUserId: c.GetInt("id"), Reason: req.Reason,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLogWithAdminInfo(userId, model.LogTypeManage,
		"subscription order financial recovery trade_no="+tradeNo, map[string]any{
			"admin_id": c.GetInt("id"), "trade_no": tradeNo, "expected_user_id": userId,
			"recovery_type": result.RecoveryType, "reason": strings.TrimSpace(req.Reason),
		})
	common.ApiSuccess(c, result)
}

type AdminCreateUserSubscriptionRequest struct {
	PlanId         int    `json:"plan_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
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
	msg, err := model.AdminBindSubscription(model.AdminTimedSubscriptionGrantRequest{
		UserId: userId, PlanId: req.PlanId, IdempotencyKey: req.IdempotencyKey, Reason: req.Reason,
	})
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
