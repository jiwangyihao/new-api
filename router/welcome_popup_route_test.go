package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWelcomePopupRouteRequiresUserAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldGlobalApiRateLimitEnable := common.GlobalApiRateLimitEnable
	oldRedisEnabled := common.RedisEnabled
	common.GlobalApiRateLimitEnable = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = oldGlobalApiRateLimitEnable
		common.RedisEnabled = oldRedisEnabled
	})

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/welcome-popup", nil)
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), console_setting.DefaultWelcomePopupContent)
}
