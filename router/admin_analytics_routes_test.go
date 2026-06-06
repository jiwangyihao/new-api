package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminAnalyticsPaidSubscriptionRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		if route.Method == http.MethodGet {
			registered[route.Path] = struct{}{}
		}
	}

	for _, path := range []string{
		"/api/admin-analytics/paid-subscription-value/summary",
		"/api/admin-analytics/paid-subscription-value/users",
		"/api/admin-analytics/paid-subscription-value/subscriptions",
		"/api/admin-analytics/paid-subscription-value/breakdown/plans",
		"/api/admin-analytics/paid-subscription-value/breakdown/sources",
		"/api/admin-analytics/invitation-paid-subscriptions/summary",
		"/api/admin-analytics/invitation-paid-subscriptions/inviters",
		"/api/admin-analytics/invitation-paid-subscriptions/invitees",
		"/api/admin-analytics/invitation-paid-subscriptions/subscriptions",
	} {
		_, ok := registered[path]
		require.Truef(t, ok, "route %s is not registered", path)
	}
}
