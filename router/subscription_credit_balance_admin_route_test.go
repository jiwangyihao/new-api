package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreditBalanceAdminRoutePersistsDedicatedConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))

	accessToken := "credit-balance-admin-route-token"
	require.NoError(t, db.Create(&model.User{
		Id:          9951,
		Username:    "credit-balance-admin-route",
		Status:      common.UserStatusEnabled,
		Role:        common.RoleAdminUser,
		AccessToken: &accessToken,
	}).Error)
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Title:            "Credit 余额套餐",
		EntitlementType:  model.SubscriptionEntitlementCreditBalance,
		Enabled:          true,
		PublicVisible:    false,
		RewardEligible:   false,
		DurationUnit:     model.SubscriptionDurationMonth,
		DurationValue:    1,
		QuotaResetPeriod: model.SubscriptionResetNever,
	}).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("credit-balance-route-secret"))))
	SetApiRouter(engine)

	updateRecorder := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(
		http.MethodPut,
		"/api/subscription/admin/credit-balance-plan",
		bytes.NewBufferString(`{"model_limits":" gpt-4o,claude-3-7-sonnet,gpt-4o ","concurrency_limit":7,"queue_capacity":13,"business_code":"credit_balance_global","configured":true,"purchase_enabled":true,"redemption_enabled":false,"conversion_enabled":true}`),
	)
	updateRequest.Header.Set("Content-Type", "application/json")
	updateRequest.Header.Set("Authorization", "Bearer "+accessToken)
	updateRequest.Header.Set("New-Api-User", "9951")
	engine.ServeHTTP(updateRecorder, updateRequest)

	require.Equal(t, http.StatusOK, updateRecorder.Code, updateRecorder.Body.String())
	assert.Contains(t, updateRecorder.Body.String(), `"success":true`)

	getRecorder := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/subscription/admin/credit-balance-plan", nil)
	getRequest.Header.Set("Authorization", "Bearer "+accessToken)
	getRequest.Header.Set("New-Api-User", "9951")
	engine.ServeHTTP(getRecorder, getRequest)

	require.Equal(t, http.StatusOK, getRecorder.Code, getRecorder.Body.String())
	assert.Contains(t, getRecorder.Body.String(), `"model_limits":"gpt-4o,claude-3-7-sonnet"`)
	assert.Contains(t, getRecorder.Body.String(), `"credit_balance_configured":true`)
	assert.Contains(t, getRecorder.Body.String(), `"credit_balance_purchase_enabled":true`)
	assert.Contains(t, getRecorder.Body.String(), `"credit_balance_redemption_enabled":false`)
	assert.Contains(t, getRecorder.Body.String(), `"credit_balance_conversion_enabled":true`)

	var plan model.SubscriptionPlan
	require.NoError(t, db.Where("entitlement_type = ?", model.SubscriptionEntitlementCreditBalance).First(&plan).Error)
	assert.Equal(t, "gpt-4o,claude-3-7-sonnet", plan.ModelLimits)
	assert.Equal(t, 7, plan.ConcurrencyLimit)
	assert.Equal(t, 13, plan.QueueCapacity)
	require.NotNil(t, plan.BusinessCode)
	assert.Equal(t, "credit_balance_global", *plan.BusinessCode)
}
