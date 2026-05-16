package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWelcomePopupRouter(t *testing.T, authenticated bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	router.GET("/api/user/welcome-popup", func(c *gin.Context) {
		if authenticated {
			session := sessions.Default(c)
			session.Set("id", 1001)
			session.Set("username", "welcome-user")
			session.Set("role", common.RoleCommonUser)
			session.Set("status", common.UserStatusEnabled)
			session.Set("group", "default")
			require.NoError(t, session.Save())
		}
	}, middleware.UserAuth(), GetWelcomePopup)
	return router
}

func withWelcomePopupSetting(t *testing.T, mutate func(*console_setting.ConsoleSetting)) {
	t.Helper()
	setting := console_setting.GetConsoleSetting()
	old := *setting
	mutate(setting)
	t.Cleanup(func() { *setting = old })
}

func TestGetWelcomePopupRequiresLogin(t *testing.T) {
	router := setupWelcomePopupRouter(t, false)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/welcome-popup", nil)

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), console_setting.DefaultWelcomePopupContent)
}

func TestGetWelcomePopupReturnsEnabledConfig(t *testing.T) {
	withWelcomePopupSetting(t, func(s *console_setting.ConsoleSetting) {
		s.WelcomePopupEnabled = true
		s.WelcomePopupContent = "欢迎 **回来**"
		s.WelcomePopupFrequency = console_setting.WelcomePopupFrequencyOncePerDay
	})
	router := setupWelcomePopupRouter(t, true)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/welcome-popup", nil)
	req.Header.Set("New-Api-User", "1001")

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"enabled":true`)
	assert.Contains(t, recorder.Body.String(), `"content":"欢迎 **回来**"`)
	assert.Contains(t, recorder.Body.String(), `"frequency":"once_per_day"`)
}

func TestGetWelcomePopupPreservesConfiguredContentWhitespace(t *testing.T) {
	withWelcomePopupSetting(t, func(s *console_setting.ConsoleSetting) {
		s.WelcomePopupEnabled = true
		s.WelcomePopupContent = "  欢迎 **回来**  "
		s.WelcomePopupFrequency = console_setting.WelcomePopupFrequencyOncePerVersion
	})
	router := setupWelcomePopupRouter(t, true)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/welcome-popup", nil)
	req.Header.Set("New-Api-User", "1001")

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"content":"  欢迎 **回来**  "`)
}

func TestGetWelcomePopupHidesDisabledOrEmptyContent(t *testing.T) {
	withWelcomePopupSetting(t, func(s *console_setting.ConsoleSetting) {
		s.WelcomePopupEnabled = false
		s.WelcomePopupContent = "secret"
		s.WelcomePopupFrequency = "bad"
	})
	router := setupWelcomePopupRouter(t, true)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/welcome-popup", nil)
	req.Header.Set("New-Api-User", "1001")

	router.ServeHTTP(recorder, req)

	assert.Contains(t, recorder.Body.String(), `"enabled":false`)
	assert.Contains(t, recorder.Body.String(), `"content":""`)
	assert.Contains(t, recorder.Body.String(), `"frequency":"once_per_version"`)
	assert.NotContains(t, recorder.Body.String(), "secret")
}

func TestGetWelcomePopupHidesTrimmedEmptyContent(t *testing.T) {
	withWelcomePopupSetting(t, func(s *console_setting.ConsoleSetting) {
		s.WelcomePopupEnabled = true
		s.WelcomePopupContent = " \n\t "
		s.WelcomePopupFrequency = console_setting.WelcomePopupFrequencyEverySession
	})
	router := setupWelcomePopupRouter(t, true)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/welcome-popup", nil)
	req.Header.Set("New-Api-User", "1001")

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"enabled":true`)
	assert.Contains(t, recorder.Body.String(), `"content":""`)
	assert.Contains(t, recorder.Body.String(), `"frequency":"every_session"`)
}

func TestStatusDoesNotExposeWelcomePopupContent(t *testing.T) {
	withWelcomePopupSetting(t, func(s *console_setting.ConsoleSetting) {
		s.WelcomePopupEnabled = true
		s.WelcomePopupContent = "public leak check"
	})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(ctx)

	assert.NotContains(t, recorder.Body.String(), "welcome_popup_content")
	assert.NotContains(t, recorder.Body.String(), "welcome_popup_frequency")
	assert.NotContains(t, recorder.Body.String(), "welcome_popup")
	assert.NotContains(t, recorder.Body.String(), "public leak check")
}

func TestUpdateOptionRejectsInvalidWelcomePopupFrequency(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{
		Key:   "console_setting.welcome_popup_frequency",
		Value: console_setting.WelcomePopupFrequencyOncePerVersion,
	}).Error)
	oldFrequency := console_setting.GetConsoleSetting().WelcomePopupFrequency
	console_setting.GetConsoleSetting().WelcomePopupFrequency = console_setting.WelcomePopupFrequencyOncePerVersion
	t.Cleanup(func() {
		console_setting.GetConsoleSetting().WelcomePopupFrequency = oldFrequency
	})

	requestBody := bytes.NewBufferString(`{"key":"console_setting.welcome_popup_frequency","value":"every_login"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option", requestBody)

	UpdateOption(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), "欢迎公告展示频率不合法")

	var saved model.Option
	require.NoError(t, db.First(&saved, "key = ?", "console_setting.welcome_popup_frequency").Error)
	assert.Equal(t, console_setting.WelcomePopupFrequencyOncePerVersion, saved.Value)
	assert.NotEqual(t, "every_login", console_setting.GetConsoleSetting().WelcomePopupFrequency)
}
