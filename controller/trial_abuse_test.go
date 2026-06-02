package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTrialAbuseControllerTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.Log{}))
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
}

func TestGetTrialAbuseSummaryRejectsInvalidWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	now := time.Now().Unix()
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/trial-abuse/summary?trial_end_start="+strconv.FormatInt(now, 10)+"&trial_end_end="+strconv.FormatInt(now-1, 10), nil)

	GetTrialAbuseSummary(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid trial end range")
}

func TestGetTrialAbuseSummaryReturnsUnifiedSuccessResponse(t *testing.T) {
	setupTrialAbuseControllerTestDB(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/trial-abuse/summary", nil)

	GetTrialAbuseSummary(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"data"`)
	assert.Contains(t, recorder.Body.String(), `"registration_ip_unavailable"`)
}

func TestGetTrialAbuseSummaryRequiresAdmin(t *testing.T) {
	setupTrialAbuseControllerTestDB(t)

	unauthenticatedEngine := gin.New()
	unauthenticatedEngine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	unauthenticatedEngine.GET("/api/trial-abuse/summary", middleware.AdminAuth(), GetTrialAbuseSummary)
	unauthenticated := httptest.NewRecorder()
	unauthenticatedReq := httptest.NewRequest(http.MethodGet, "/api/trial-abuse/summary", nil)
	unauthenticatedEngine.ServeHTTP(unauthenticated, unauthenticatedReq)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	userEngine := gin.New()
	userEngine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	userEngine.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 1001)
		session.Set("username", "common-user")
		session.Set("role", common.RoleCommonUser)
		session.Set("status", common.UserStatusEnabled)
		require.NoError(t, session.Save())
	})
	userEngine.GET("/api/trial-abuse/summary", middleware.AdminAuth(), GetTrialAbuseSummary)
	user := httptest.NewRecorder()
	userReq := httptest.NewRequest(http.MethodGet, "/api/trial-abuse/summary", nil)
	userReq.Header.Set("New-Api-User", "1001")
	userEngine.ServeHTTP(user, userReq)
	require.Equal(t, http.StatusOK, user.Code)
	assert.Contains(t, user.Body.String(), `"success":false`)

	adminEngine := gin.New()
	adminEngine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	adminEngine.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 1002)
		session.Set("username", "admin-user")
		session.Set("role", common.RoleAdminUser)
		session.Set("status", common.UserStatusEnabled)
		require.NoError(t, session.Save())
	})
	adminEngine.GET("/api/trial-abuse/summary", middleware.AdminAuth(), GetTrialAbuseSummary)
	admin := httptest.NewRecorder()
	adminReq := httptest.NewRequest(http.MethodGet, "/api/trial-abuse/summary", nil)
	adminReq.Header.Set("New-Api-User", "1002")
	adminEngine.ServeHTTP(admin, adminReq)
	require.Equal(t, http.StatusOK, admin.Code, admin.Body.String())
	assert.Contains(t, admin.Body.String(), `"success":true`)
}
