package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminCreateSubscriptionPlanRoundTripsExactPriceMicros(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/admin/plans", bytes.NewBufferString(`{"plan":{"title":"Exact price","price_amount":40.123456,"price_amount_micros":"40123456","currency":"CNY","duration_unit":"month","duration_value":1,"enabled":true,"monthly_token_limit":1000}}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	AdminCreateSubscriptionPlan(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"price_amount_micros":"40123456"`)
	var stored int64
	require.NoError(t, model.DB.Raw("SELECT price_amount_micros FROM subscription_plans WHERE title = ?", "Exact price").Scan(&stored).Error)
	assert.Equal(t, int64(40_123_456), stored)
}

func performAdminSubscriptionPlanCreate(body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/admin/plans", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	AdminCreateSubscriptionPlan(ctx)
	return recorder
}

func TestAdminCreateSubscriptionPlanRejectsInvalidExactPricesAtomically(t *testing.T) {
	tests := []struct {
		name      string
		priceJSON string
		wantCode  string
	}{
		{name: "missing exact paid price", priceJSON: `"price_amount":40`, wantCode: "subscription_plan_price_micros_required"},
		{name: "negative exact price", priceJSON: `"price_amount":-0.000001,"price_amount_micros":"-1"`, wantCode: "subscription_plan_price_negative"},
		{name: "more than six decimals", priceJSON: `"price_amount":1.0000001,"price_amount_micros":"1000000"`, wantCode: "subscription_plan_price_precision"},
		{name: "micros overflow", priceJSON: `"price_amount_micros":"9223372036854775808"`, wantCode: "credit_valuation_overflow"},
		{name: "compatibility mismatch", priceJSON: `"price_amount":40.000001,"price_amount_micros":"40000000"`, wantCode: "subscription_plan_price_mismatch"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupSubscriptionAdminPlanFieldsTest(t)
			body := fmt.Sprintf(`{"plan":{"title":"Invalid exact %d",%s,"currency":"CNY","duration_unit":"month","duration_value":1,"enabled":true}}`, index, test.priceJSON)

			recorder := performAdminSubscriptionPlanCreate(body)

			assert.Contains(t, recorder.Body.String(), `"success":false`)
			assert.Contains(t, recorder.Body.String(), `"code":"`+test.wantCode+`"`)
			var count int64
			require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("title = ?", fmt.Sprintf("Invalid exact %d", index)).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestAdminUpdateSubscriptionPlanRoundTripsExactPriceMicros(t *testing.T) {
	setupSubscriptionAdminPlanFieldsTest(t)
	initialMicros := int64(40_000_000)
	plan := model.SubscriptionPlan{Title: "Exact update", PriceAmount: 40, PriceAmountMicros: &initialMicros, Currency: "CNY", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, Enabled: true}
	require.NoError(t, model.DB.Create(&plan).Error)

	recorder := performAdminSubscriptionPlanUpdate(t, plan.Id, `{"plan":{"title":"Exact update","price_amount":40.654321,"price_amount_micros":"40654321","currency":"CNY","duration_unit":"month","duration_value":1,"enabled":true}}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	require.NoError(t, model.DB.First(&plan, plan.Id).Error)
	require.NotNil(t, plan.PriceAmountMicros)
	assert.Equal(t, int64(40_654_321), *plan.PriceAmountMicros)
	assert.Equal(t, 40.654321, plan.PriceAmount)

	listRecorder := httptest.NewRecorder()
	listContext, _ := gin.CreateTestContext(listRecorder)
	AdminListSubscriptionPlans(listContext)
	assert.Contains(t, listRecorder.Body.String(), `"price_amount_micros":"40654321"`)
}
