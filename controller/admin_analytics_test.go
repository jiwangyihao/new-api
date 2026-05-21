package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAdminAnalyticsControllerTestDBs(t *testing.T) *gorm.DB {
	t.Helper()

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.InvitationMonthlyEntitlement{}))
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	return db
}

func performAdminAnalyticsRequest(t *testing.T, rawURL string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)
	ctx.Set("id", 1)
	handler(ctx)
	return recorder
}

func TestAdminAnalyticsRejectsInvalidTimeRange(t *testing.T) {
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/overview?start_timestamp=200&end_timestamp=100", GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid time range")
}

func TestAdminAnalyticsRejectsRangeOver365Days(t *testing.T) {
	start := int64(1000)
	end := start + 366*24*60*60
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/overview?start_timestamp=1000&end_timestamp="+strconv.FormatInt(end, 10), GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "time range exceeds 365 days")
}

func TestAdminAnalyticsRejectsInvalidRepeatedPlanIDs(t *testing.T) {
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/overview?plan_ids=1&plan_ids=abc", GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid plan_ids")
}

func TestAdminAnalyticsRejectsInvalidSources(t *testing.T) {
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/overview?sources=bad", GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid source")
}

func TestAdminAnalyticsSnapshotAtUsesHistoricalRangeEnd(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	end := time.Now().Unix() - 24*60*60
	start := end - 60
	target := "/api/admin-analytics/overview?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(end, 10)
	recorder := performAdminAnalyticsRequest(t, target, GetAdminAnalyticsOverview)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"snapshot_at":`+strconv.FormatInt(end, 10))
}

func TestAdminAnalyticsRejectsUnsupportedSortBy(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/plan-distribution?sort_by=not_a_column", GetAdminAnalyticsPlanDistribution)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "unsupported sort_by")
}

func TestAdminAnalyticsDrilldownRejectsInvalidUserID(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/drilldown/users?user_id=abc", GetAdminAnalyticsDrilldownUsers)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid user_id")
}

func TestAdminAnalyticsDrilldownUsersAcceptsRepeatedUserID(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	seedAdminAnalyticsControllerUsers(t, 3)

	recorder := performAdminAnalyticsRequest(t, "/api/admin-analytics/drilldown/users?user_id=1&user_id=2", GetAdminAnalyticsDrilldownUsers)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	require.Contains(t, body, `"total":2`)
	require.Contains(t, body, `"user_id":1`)
	require.Contains(t, body, `"user_id":2`)
	require.NotContains(t, body, `"user_id":3`)
}

func TestAdminAnalyticsDrilldownUsersClampsLimitTo100(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	seedAdminAnalyticsControllerUsers(t, 150)

	now := time.Now().Unix()
	target := "/api/admin-analytics/drilldown/users?start_timestamp=" + strconv.FormatInt(now-60, 10) + "&end_timestamp=" + strconv.FormatInt(now, 10) + "&limit=500"
	recorder := performAdminAnalyticsRequest(t, target, GetAdminAnalyticsDrilldownUsers)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"limit":100`)
	require.Contains(t, recorder.Body.String(), `"total":150`)
}

func TestAdminAnalyticsDrilldownUsersDoesNotExposeSensitiveFields(t *testing.T) {
	setupAdminAnalyticsControllerTestDBs(t)
	accessToken := "access-token-secret"
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.User{
		Id:             1,
		Username:       "safe-admin-analytics-user",
		Password:       "password-secret",
		DisplayName:    "safe user",
		Email:          "safe@example.com",
		Status:         1,
		Role:           1,
		Group:          "default",
		AccessToken:    &accessToken,
		AffCode:        "aff-secret",
		Setting:        `{"secret":true}`,
		Remark:         "remark-secret",
		StripeCustomer: "stripe-customer-secret",
		CreatedAt:      now - 10,
		LastLoginAt:    now - 5,
	}).Error)

	target := "/api/admin-analytics/drilldown/users?start_timestamp=" + strconv.FormatInt(now-60, 10) + "&end_timestamp=" + strconv.FormatInt(now, 10)
	recorder := performAdminAnalyticsRequest(t, target, GetAdminAnalyticsDrilldownUsers)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	for _, field := range []string{"password", "access_token", "stripe_customer", "aff_code", "setting", "remark", "key", "allow_ips"} {
		require.NotContains(t, body, `"`+field+`"`)
	}
	for _, secret := range []string{"password-secret", "access-token-secret", "aff-secret", "remark-secret", "stripe-customer-secret"} {
		require.NotContains(t, body, secret)
	}
}

func seedAdminAnalyticsControllerUsers(t *testing.T, count int) {
	t.Helper()

	users := make([]model.User, 0, count)
	for i := 1; i <= count; i++ {
		users = append(users, model.User{Id: i, Username: "analytics-user-" + strconv.Itoa(i), Status: 1, Group: "default", AffCode: "analytics-aff-" + strconv.Itoa(i)})
	}
	require.NoError(t, model.DB.Create(&users).Error)
}
