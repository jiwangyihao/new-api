package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvitationCommissionRoutesRegisteredAndRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	SetApiRouter(engine)

	registered := make(map[string]map[string]struct{})
	for _, route := range engine.Routes() {
		if registered[route.Path] == nil {
			registered[route.Path] = make(map[string]struct{})
		}
		registered[route.Path][route.Method] = struct{}{}
	}
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/user/invitation-commission/summary"},
		{http.MethodGet, "/api/user/invitation-commission/records"},
		{http.MethodPost, "/api/user/invitation-commission/transfer"},
		{http.MethodGet, "/api/user/invitation-commission/withdrawals"},
		{http.MethodPost, "/api/user/invitation-commission/withdrawals"},
		{http.MethodGet, "/api/admin/invitation-commission/withdrawals"},
		{http.MethodPost, "/api/admin/invitation-commission/withdrawals/:id/complete"},
		{http.MethodPost, "/api/admin/invitation-commission/withdrawals/:id/reject"},
		{http.MethodGet, "/api/admin/tasks/summary"},
	} {
		methods := registered[route.path]
		require.NotNil(t, methods, route.path)
		_, ok := methods[route.method]
		assert.True(t, ok, route.method+" "+route.path)
	}

	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/user/invitation-commission/summary"},
		{http.MethodGet, "/api/admin/invitation-commission/withdrawals"},
		{http.MethodGet, "/api/admin/tasks/summary"},
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(request.method, request.path, nil)
		engine.ServeHTTP(recorder, req)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code, request.path)
	}
}

func TestInvitationCommissionRouteSourceKeepsCriticalRateLimits(t *testing.T) {
	raw, err := os.ReadFile("api-router.go")
	require.NoError(t, err)
	source := string(raw)
	for _, want := range []string{
		`selfRoute.POST("/invitation-commission/transfer", middleware.CriticalRateLimit(), controller.TransferInvitationCommission)`,
		`selfRoute.POST("/invitation-commission/withdrawals", middleware.CriticalRateLimit(), controller.CreateInvitationCommissionWithdrawal)`,
		`adminCommissionRoute.POST("/withdrawals/:id/complete", middleware.CriticalRateLimit(), controller.AdminCompleteInvitationCommissionWithdrawal)`,
		`adminCommissionRoute.POST("/withdrawals/:id/reject", middleware.CriticalRateLimit(), controller.AdminRejectInvitationCommissionWithdrawal)`,
	} {
		assert.True(t, strings.Contains(source, want), want)
	}
	assert.True(t, strings.Contains(source, `adminCommissionRoute.Use(middleware.AdminAuth())`))
	assert.True(t, strings.Contains(source, `adminTasksRoute.Use(middleware.AdminAuth())`))
}
