package router

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type gptAbuseRouteAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func TestGPTAbuseRoutesRequireAdminAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseRouteTestDB(t)
	adminToken := "gpt-abuse-admin-token"
	commonToken := "gpt-abuse-common-token"
	seedGPTAbuseRouteUser(t, 81701, common.RoleAdminUser, adminToken)
	seedGPTAbuseRouteUser(t, 81702, common.RoleCommonUser, commonToken)
	seedGPTAbuseRouteUser(t, 81703, common.RoleCommonUser, "target-token")

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	protected := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/gpt-abuse/users", nil},
		{http.MethodGet, "/api/gpt-abuse/users/81703/logs", nil},
		{http.MethodGet, "/api/gpt-abuse/users/81703/repeat-blocks", nil},
		{http.MethodPost, "/api/gpt-abuse/users/81703/reset-warnings", map[string]any{"reason": "manual review", "clear_suspension": true}},
		{http.MethodPost, "/api/gpt-abuse/users/81703/clear-suspension", map[string]any{"reason": "manual review"}},
	}

	for _, request := range protected {
		unauth := performGPTAbuseRouteRequest(t, engine, request.method, request.path, request.body, "", 0)
		assert.Equal(t, http.StatusUnauthorized, unauth.Code, request.path)

		commonUser := performGPTAbuseRouteRequest(t, engine, request.method, request.path, request.body, commonToken, 81702)
		assert.Equal(t, http.StatusOK, commonUser.Code, request.path)
		assert.False(t, decodeGPTAbuseRouteAPIResponse(t, commonUser).Success, request.path)

		admin := performGPTAbuseRouteRequest(t, engine, request.method, request.path, request.body, adminToken, 81701)
		assert.Equal(t, http.StatusOK, admin.Code, request.path)
		assert.True(t, decodeGPTAbuseRouteAPIResponse(t, admin).Success, request.path+": "+admin.Body.String())
	}
}

func setupGPTAbuseRouteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled
	oldGlobalRateLimit := common.GlobalApiRateLimitEnable
	oldGPTAbuseLimitEnabled := common.GPTAbuseLimitEnabled
	oldGPTAbuseDefaultWarningLimit := common.GPTAbuseDefaultWarningLimit

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.GlobalApiRateLimitEnable = false
	common.GPTAbuseLimitEnabled = true
	common.GPTAbuseDefaultWarningLimit = 1

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.GPTAbuseSignalLog{}, &model.GPTAbuseUserSuspension{}, &model.GPTAbuseWarningReset{}, &model.GPTAbuseRepeatBlockLog{}))

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		common.GlobalApiRateLimitEnable = oldGlobalRateLimit
		common.GPTAbuseLimitEnabled = oldGPTAbuseLimitEnabled
		common.GPTAbuseDefaultWarningLimit = oldGPTAbuseDefaultWarningLimit
	})
	return db
}

func seedGPTAbuseRouteUser(t *testing.T, userID int, role int, accessToken string) {
	t.Helper()
	user := model.User{Id: userID, Username: fmt.Sprintf("gpt-abuse-route-%d", userID), Role: role, Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("aff-route-%d", userID)}
	user.SetAccessToken(accessToken)
	require.NoError(t, model.DB.Create(&user).Error)
}

func performGPTAbuseRouteRequest(t *testing.T, engine *gin.Engine, method string, path string, body any, accessToken string, userID int) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := common.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(payload)
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("New-Api-User", fmt.Sprintf("%d", userID))
	}
	engine.ServeHTTP(recorder, req)
	return recorder
}

func decodeGPTAbuseRouteAPIResponse(t *testing.T, recorder *httptest.ResponseRecorder) gptAbuseRouteAPIResponse {
	t.Helper()
	var response gptAbuseRouteAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response
}
