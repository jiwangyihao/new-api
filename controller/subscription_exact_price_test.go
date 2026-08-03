package controller

import (
	"bytes"
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
