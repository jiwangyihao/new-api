package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGPTAbuseAuthTestDB(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled
	oldGPTAbuseLimitEnabled := common.GPTAbuseLimitEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.GPTAbuseLimitEnabled = true

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+safeName+"_gpt_abuse_auth?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.GPTAbuseUserSuspension{}))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		common.GPTAbuseLimitEnabled = oldGPTAbuseLimitEnabled
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

func seedGPTAbuseAuthUserAndToken(t *testing.T, userID int, tokenKey string) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "gpt_abuse_auth_user", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: userID + 1000, UserId: userID, Key: tokenKey, Status: common.TokenStatusEnabled, Name: "gpt-abuse-token", ExpiredTime: -1, UnlimitedQuota: true}).Error)
}

func performGPTAbuseTokenAuthRequest(tokenKey string) *httptest.ResponseRecorder {
	engine := gin.New()
	engine.Use(TokenAuth())
	engine.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("Authorization", "Bearer sk-"+tokenKey)
	engine.ServeHTTP(recorder, req)
	return recorder
}

func TestTokenAuthAllowsActiveGPTAbuseSuspensionForRouteLevelEnforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseAuthTestDB(t)
	seedGPTAbuseAuthUserAndToken(t, 9101, "gptabuseactive")
	require.NoError(t, model.UpsertGPTAbuseSuspension(9101, 1, 5, 5, time.Now().Add(time.Hour).Unix()))

	recorder := performGPTAbuseTokenAuthRequest("gptabuseactive")

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestTokenAuthAllowsExpiredGPTAbuseSuspensionForRouteLevelEnforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGPTAbuseAuthTestDB(t)
	seedGPTAbuseAuthUserAndToken(t, 9102, "gptabuseexpired")
	require.NoError(t, model.UpsertGPTAbuseSuspension(9102, 1, 5, 5, time.Now().Add(-time.Hour).Unix()))

	recorder := performGPTAbuseTokenAuthRequest("gptabuseexpired")

	assert.Equal(t, http.StatusOK, recorder.Code)
}
