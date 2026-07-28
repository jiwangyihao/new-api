package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSubscriptionAdminPlanFieldsTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}))
}

func performAdminSubscriptionPlanUpdate(t *testing.T, id int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/subscription/admin/plans", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	AdminUpdateSubscriptionPlan(ctx)
	return recorder
}

func TestAdminUpdateSubscriptionPlanPersistsDistributorFields(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	code := "basic_monthly"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:             8901,
		Title:          "Basic",
		Enabled:        true,
		DurationUnit:   model.SubscriptionDurationMonth,
		DurationValue:  1,
		BusinessCode:   &code,
		PublicVisible:  true,
		RewardEligible: true,
	}).Error)

	recorder := performAdminSubscriptionPlanUpdate(t, 8901, `{"plan":{"title":"Basic Updated","price_amount":40,"duration_unit":"month","duration_value":1,"enabled":true,"sort_order":9,"max_purchase_per_user":0,"total_amount":0,"monthly_token_limit":1000000000,"concurrency_limit":3,"queue_capacity":12,"gpt_abuse_warning_limit":7,"is_trial":true,"invite_trial":true,"public_visible":false,"trial_duration_hours":24,"reward_eligible":false,"business_code":"basic_monthly_updated","kyren_product_id":"prod_kyren_plan"}}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&plan, 8901).Error)
	assert.Equal(t, int64(1_000_000_000), plan.MonthlyTokenLimit)
	assert.Equal(t, 3, plan.ConcurrencyLimit)
	assert.Equal(t, 12, plan.QueueCapacity)
	assert.Equal(t, 7, plan.GPTAbuseWarningLimit)
	assert.True(t, plan.IsTrial)
	assert.True(t, plan.InviteTrial)
	assert.False(t, plan.PublicVisible)
	assert.Equal(t, 24, plan.TrialDurationHours)
	assert.False(t, plan.RewardEligible)
	require.NotNil(t, plan.BusinessCode)
	assert.Equal(t, "basic_monthly_updated", *plan.BusinessCode)
	assert.Equal(t, "prod_kyren_plan", plan.KyrenProductId)
}

func TestAdminUpdateSubscriptionPlanPersistsCNYCurrency(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	code := "standard_monthly"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:             8902,
		Title:          "Standard",
		Enabled:        true,
		DurationUnit:   model.SubscriptionDurationMonth,
		DurationValue:  1,
		BusinessCode:   &code,
		PublicVisible:  true,
		RewardEligible: true,
		Currency:       "USD",
	}).Error)

	recorder := performAdminSubscriptionPlanUpdate(t, 8902, `{"plan":{"title":"Standard Updated","price_amount":80,"currency":"CNY","duration_unit":"month","duration_value":1,"enabled":true,"sort_order":8,"max_purchase_per_user":0,"total_amount":0,"monthly_token_limit":2000000000,"concurrency_limit":5,"is_trial":false,"public_visible":true,"trial_duration_hours":0,"reward_eligible":true,"business_code":"standard_monthly"}}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&plan, 8902).Error)
	assert.Equal(t, "CNY", plan.Currency)
}

func TestAdminCreateSubscriptionPlanDefaultsCurrencyToCNY(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/admin/plans", bytes.NewBufferString(`{"plan":{"title":"Basic","price_amount":40,"duration_unit":"month","duration_value":1,"enabled":true,"sort_order":9,"max_purchase_per_user":0,"total_amount":0,"monthly_token_limit":1000000000,"concurrency_limit":1,"queue_capacity":6,"is_trial":false,"public_visible":true,"trial_duration_hours":0,"reward_eligible":true,"business_code":"basic_monthly","kyren_product_id":"prod_kyren_create"}}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	AdminCreateSubscriptionPlan(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.Where("business_code = ?", "basic_monthly").First(&plan).Error)
	assert.Equal(t, "CNY", plan.Currency)
	assert.Equal(t, 6, plan.QueueCapacity)
	assert.Equal(t, "prod_kyren_create", plan.KyrenProductId)
}

func TestAdminCreateSubscriptionPlanRejectsNegativeQueueCapacity(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/admin/plans", bytes.NewBufferString(`{"plan":{"title":"Bad Queue","price_amount":40,"duration_unit":"month","duration_value":1,"queue_capacity":-1}}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	AdminCreateSubscriptionPlan(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "排队容量不能为负数")
}

func seedCreditBalancePlanForAdminTest(t *testing.T) model.SubscriptionPlan {
	t.Helper()
	plan := model.SubscriptionPlan{
		Title:            "Credit 余额套餐",
		EntitlementType:  model.SubscriptionEntitlementCreditBalance,
		Enabled:          true,
		PublicVisible:    false,
		RewardEligible:   false,
		DurationUnit:     model.SubscriptionDurationMonth,
		DurationValue:    1,
		QuotaResetPeriod: model.SubscriptionResetNever,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	return plan
}

func performAuthenticatedCreditBalancePlanRequest(t *testing.T, role int, method string, body string) *httptest.ResponseRecorder {
	t.Helper()
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("credit-balance-admin-test"))))
	engine.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 9910)
		session.Set("username", "credit-balance-admin")
		session.Set("role", role)
		session.Set("status", common.UserStatusEnabled)
		require.NoError(t, session.Save())
	})
	group := engine.Group("/api/subscription/admin")
	group.Use(middleware.AdminAuth())
	group.GET("/credit-balance-plan", AdminGetCreditBalancePlan)
	group.PUT("/credit-balance-plan", AdminUpdateCreditBalancePlan)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/api/subscription/admin/credit-balance-plan", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("New-Api-User", "9910")
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminCreditBalancePlanLifecycleUsesDedicatedAuthenticatedAPI(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	seedCreditBalancePlanForAdminTest(t)

	update := performAuthenticatedCreditBalancePlanRequest(t, common.RoleAdminUser, http.MethodPut, `{"model_limits":" gpt-4o, claude-3-7-sonnet,gpt-4o ","concurrency_limit":7,"queue_capacity":13,"business_code":" credit_balance_global ","configured":true,"purchase_enabled":true,"redemption_enabled":false,"conversion_enabled":true}`)
	require.Equal(t, http.StatusOK, update.Code, update.Body.String())
	assert.Contains(t, update.Body.String(), `"success":true`)

	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.Where("entitlement_type = ?", model.SubscriptionEntitlementCreditBalance).First(&plan).Error)
	assert.Equal(t, "gpt-4o,claude-3-7-sonnet", plan.ModelLimits)
	assert.Equal(t, 7, plan.ConcurrencyLimit)
	assert.Equal(t, 13, plan.QueueCapacity)
	require.NotNil(t, plan.BusinessCode)
	assert.Equal(t, "credit_balance_global", *plan.BusinessCode)
	assert.True(t, plan.CreditBalanceConfigured)
	assert.True(t, plan.CreditBalancePurchaseEnabled)
	assert.False(t, plan.CreditBalanceRedemptionEnabled)
	assert.True(t, plan.CreditBalanceConversionEnabled)

	get := performAuthenticatedCreditBalancePlanRequest(t, common.RoleAdminUser, http.MethodGet, "")
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	assert.Contains(t, get.Body.String(), `"model_limits":"gpt-4o,claude-3-7-sonnet"`)
	assert.Contains(t, get.Body.String(), `"credit_balance_purchase_enabled":true`)
}

func TestAdminCreditBalancePlanRejectsEnabledEntryBeforeConfiguration(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	seedCreditBalancePlanForAdminTest(t)

	recorder := performAuthenticatedCreditBalancePlanRequest(t, common.RoleAdminUser, http.MethodPut, `{"model_limits":"gpt-4o","concurrency_limit":1,"queue_capacity":0,"business_code":"credit_balance_global","configured":false,"purchase_enabled":true,"redemption_enabled":false,"conversion_enabled":false}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "必须先确认 Credit 余额套餐配置")
}

func TestAdminOrdinaryPlanAPIsCannotMutateCreditBalanceIdentity(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	plan := seedCreditBalancePlanForAdminTest(t)

	update := performAdminSubscriptionPlanUpdate(t, plan.Id, `{"plan":{"title":"Disguised timed plan","price_amount":40,"duration_unit":"month","duration_value":1,"enabled":false,"monthly_token_limit":1000,"concurrency_limit":1,"public_visible":true,"business_code":"disguised"}}`)
	assert.Contains(t, update.Body.String(), "Credit 余额套餐只能通过专用接口配置")

	statusRecorder := httptest.NewRecorder()
	statusContext, _ := gin.CreateTestContext(statusRecorder)
	statusContext.Params = gin.Params{{Key: "id", Value: strconv.Itoa(plan.Id)}}
	statusContext.Request = httptest.NewRequest(http.MethodPatch, "/api/subscription/admin/plans/"+strconv.Itoa(plan.Id), bytes.NewBufferString(`{"enabled":false}`))
	statusContext.Request.Header.Set("Content-Type", "application/json")
	AdminUpdateSubscriptionPlanStatus(statusContext)
	assert.Contains(t, statusRecorder.Body.String(), "Credit 余额套餐只能通过专用接口配置")

	createRecorder := httptest.NewRecorder()
	createContext, _ := gin.CreateTestContext(createRecorder)
	createContext.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/admin/plans", bytes.NewBufferString(`{"plan":{"title":"Second balance","entitlement_type":"credit_balance"}}`))
	createContext.Request.Header.Set("Content-Type", "application/json")
	AdminCreateSubscriptionPlan(createContext)
	assert.Contains(t, createRecorder.Body.String(), "Credit 余额套餐只能通过专用接口配置")

	require.NoError(t, model.DB.First(&plan, plan.Id).Error)
	assert.Equal(t, model.SubscriptionEntitlementCreditBalance, plan.EntitlementType)
	assert.True(t, plan.Enabled)
	assert.Equal(t, float64(0), plan.PriceAmount)
}

func TestAdminTimedPlanCreditEligibilityRequiresStandardMonthlyPlan(t *testing.T) {
	tests := []struct {
		name              string
		durationUnit      string
		durationValue     int
		resetPeriod       string
		monthlyTokenLimit int64
		isTrial           bool
		inviteTrial       bool
		wantMessage       string
	}{
		{name: "non-month duration", durationUnit: model.SubscriptionDurationDay, durationValue: 31, resetPeriod: model.SubscriptionResetMonthly, monthlyTokenLimit: 1000, wantMessage: "期限恰好 1 个月"},
		{name: "multiple months", durationUnit: model.SubscriptionDurationMonth, durationValue: 2, resetPeriod: model.SubscriptionResetMonthly, monthlyTokenLimit: 1000, wantMessage: "期限恰好 1 个月"},
		{name: "daily reset", durationUnit: model.SubscriptionDurationMonth, durationValue: 1, resetPeriod: model.SubscriptionResetDaily, monthlyTokenLimit: 1000, wantMessage: "按月重置"},
		{name: "weekly reset", durationUnit: model.SubscriptionDurationMonth, durationValue: 1, resetPeriod: model.SubscriptionResetWeekly, monthlyTokenLimit: 1000, wantMessage: "按月重置"},
		{name: "custom reset", durationUnit: model.SubscriptionDurationMonth, durationValue: 1, resetPeriod: model.SubscriptionResetCustom, monthlyTokenLimit: 1000, wantMessage: "按月重置"},
		{name: "never reset", durationUnit: model.SubscriptionDurationMonth, durationValue: 1, resetPeriod: model.SubscriptionResetNever, monthlyTokenLimit: 1000, wantMessage: "按月重置"},
		{name: "zero or unlimited credit", durationUnit: model.SubscriptionDurationMonth, durationValue: 1, resetPeriod: model.SubscriptionResetMonthly, monthlyTokenLimit: 0, wantMessage: "月 Credit 必须为正"},
		{name: "trial", durationUnit: model.SubscriptionDurationMonth, durationValue: 1, resetPeriod: model.SubscriptionResetMonthly, monthlyTokenLimit: 1000, isTrial: true, wantMessage: "试用套餐"},
		{name: "monthly invite", durationUnit: model.SubscriptionDurationMonth, durationValue: 1, resetPeriod: model.SubscriptionResetMonthly, monthlyTokenLimit: 1000, inviteTrial: true, wantMessage: "每月邀请套餐"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupSubscriptionAdminPlanFieldsTest(t)
			body := `{"plan":{"title":"Invalid eligibility","price_amount":40,"duration_unit":"` + test.durationUnit + `","duration_value":` + strconv.Itoa(test.durationValue) + `,"quota_reset_period":"` + test.resetPeriod + `","quota_reset_custom_seconds":3600,"monthly_token_limit":` + strconv.FormatInt(test.monthlyTokenLimit, 10) + `,"is_trial":` + strconv.FormatBool(test.isTrial) + `,"invite_trial":` + strconv.FormatBool(test.inviteTrial) + `,"unlimited_purchase_enabled":true,"timed_conversion_enabled":` + strconv.FormatBool(index%2 == 0) + `}}`
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/admin/plans", bytes.NewBufferString(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			AdminCreateSubscriptionPlan(ctx)
			assert.Contains(t, recorder.Body.String(), test.wantMessage)
		})
	}
}
func TestAdminTimedPlanCreditEligibilitySwitchesRemainIndependent(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	code := "independent_credit_switches"
	plan := model.SubscriptionPlan{
		Title:                    "Independent switches",
		EntitlementType:          model.SubscriptionEntitlementTimed,
		PriceAmount:              40,
		Currency:                 "CNY",
		DurationUnit:             model.SubscriptionDurationMonth,
		DurationValue:            1,
		QuotaResetPeriod:         model.SubscriptionResetMonthly,
		MonthlyTokenLimit:        1000,
		BusinessCode:             &code,
		Enabled:                  true,
		UnlimitedPurchaseEnabled: true,
		TimedConversionEnabled:   false,
	}
	require.NoError(t, model.DB.Create(&plan).Error)

	first := performAdminSubscriptionPlanUpdate(t, plan.Id, `{"plan":{"title":"Independent switches","price_amount":40,"currency":"CNY","duration_unit":"month","duration_value":1,"quota_reset_period":"monthly","monthly_token_limit":1000,"enabled":true,"unlimited_purchase_enabled":false,"timed_conversion_enabled":true,"business_code":"independent_credit_switches"}}`)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.NoError(t, model.DB.First(&plan, plan.Id).Error)
	assert.False(t, plan.UnlimitedPurchaseEnabled)
	assert.True(t, plan.TimedConversionEnabled)

	second := performAdminSubscriptionPlanUpdate(t, plan.Id, `{"plan":{"title":"Independent switches","price_amount":40,"currency":"CNY","duration_unit":"month","duration_value":1,"quota_reset_period":"monthly","monthly_token_limit":1000,"enabled":true,"unlimited_purchase_enabled":true,"timed_conversion_enabled":false,"business_code":"independent_credit_switches"}}`)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.NoError(t, model.DB.First(&plan, plan.Id).Error)
	assert.True(t, plan.UnlimitedPurchaseEnabled)
	assert.False(t, plan.TimedConversionEnabled)
}

func TestAdminPlanListExcludesDedicatedCreditBalancePlan(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	seedCreditBalancePlanForAdminTest(t)
	code := "admin_timed_visible"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Title: "Admin timed", EntitlementType: model.SubscriptionEntitlementTimed, Enabled: true, BusinessCode: &code}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/admin/plans", nil)
	AdminListSubscriptionPlans(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "Admin timed")
	assert.NotContains(t, recorder.Body.String(), "Credit 余额套餐")
}

func TestPublicPlanListNeverExposesCreditBalancePlan(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	plan := seedCreditBalancePlanForAdminTest(t)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{"public_visible": true, "enabled": true}).Error)
	code := "timed_visible"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Title: "Visible timed", EntitlementType: model.SubscriptionEntitlementTimed, Enabled: true, PublicVisible: true, BusinessCode: &code}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/public/plans", nil)
	GetPublicSubscriptionPlans(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "Visible timed")
	assert.NotContains(t, recorder.Body.String(), "Credit 余额套餐")
}
