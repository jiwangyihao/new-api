package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminCreateTimedSubscriptionRequiresRetryableAuditAndReplays(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.TimedSubscriptionValuationGrant{}))
	priceMicros := int64(40_000_000)
	user := model.User{Id: 21_601, Username: "timed-admin", Status: common.UserStatusEnabled, AffCode: "timed-admin-aff"}
	plan := model.SubscriptionPlan{
		Id: 21_602, Title: "Timed Admin", Enabled: true,
		EntitlementType: model.SubscriptionEntitlementTimed,
		PriceAmount:     40, PriceAmountMicros: &priceMicros, Currency: "CNY",
		DurationUnit: model.SubscriptionDurationCustom, CustomSeconds: 3600,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: model.SubscriptionResetNever,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&plan).Error)

	missingAudit := performAdminCreateTimedSubscription(t, user.Id, `{"plan_id":21602}`)
	require.Equal(t, http.StatusOK, missingAudit.Code)
	require.Contains(t, missingAudit.Body.String(), `"success":false`)
	var count int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Count(&count).Error)
	require.Zero(t, count)

	payload := `{"plan_id":21602,"reason":"售后履约纠偏","idempotency_key":"admin-timed-21603","source_price_micros":"25000000","source_currency":"USD"}`
	first := performAdminCreateTimedSubscription(t, user.Id, payload)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Contains(t, first.Body.String(), `"success":true`)

	var grant model.TimedSubscriptionValuationGrant
	require.NoError(t, model.DB.Where("source_type = ? AND source_key = ?", model.TimedSubscriptionGrantSourceAdmin, "admin:admin-timed-21603").First(&grant).Error)
	require.Equal(t, int64(25_000_000), grant.SourcePriceMicros)
	require.Equal(t, "USD", grant.SourceCurrency)
	require.Contains(t, grant.SourceSnapshot, "售后履约纠偏")
	firstEnd := grant.EventEndTime

	replay := performAdminCreateTimedSubscription(t, user.Id, payload)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	require.Contains(t, replay.Body.String(), `"success":true`)
	var subscription model.UserSubscription
	require.NoError(t, model.DB.First(&subscription, grant.UserSubscriptionId).Error)
	require.Equal(t, firstEnd, subscription.EndTime)
	require.NoError(t, model.DB.Model(&model.TimedSubscriptionValuationGrant{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func performAdminCreateTimedSubscription(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "21601"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/admin/users/21601/subscriptions", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 9901)
	ctx.Set("username", "timed-admin-operator")
	AdminCreateUserSubscription(ctx)
	return recorder
}
