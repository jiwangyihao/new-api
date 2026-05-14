package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/model"
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

	recorder := performAdminSubscriptionPlanUpdate(t, 8901, `{"plan":{"title":"Basic Updated","price_amount":40,"duration_unit":"month","duration_value":1,"enabled":true,"sort_order":9,"max_purchase_per_user":0,"total_amount":0,"monthly_token_limit":1000000000,"concurrency_limit":3,"is_trial":true,"public_visible":false,"trial_duration_hours":24,"reward_eligible":false,"business_code":"basic_monthly_updated"}}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var plan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&plan, 8901).Error)
	assert.Equal(t, int64(1_000_000_000), plan.MonthlyTokenLimit)
	assert.Equal(t, 3, plan.ConcurrencyLimit)
	assert.True(t, plan.IsTrial)
	assert.False(t, plan.PublicVisible)
	assert.Equal(t, 24, plan.TrialDurationHours)
	assert.False(t, plan.RewardEligible)
	require.NotNil(t, plan.BusinessCode)
	assert.Equal(t, "basic_monthly_updated", *plan.BusinessCode)
}
