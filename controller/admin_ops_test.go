package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestParseAdminOpsSnapshotQueryNormalizesBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin-ops/snapshot?window_seconds=999&top=999", nil)

	query := parseAdminOpsSnapshotQuery(ctx)

	assert.EqualValues(t, 300, query.WindowSeconds)
	assert.Equal(t, 20, query.Top)
}

func TestParseAdminOpsConcurrencyQueryNormalizesBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin-ops/concurrency?limit=999&include_users=false&min_active_or_queued=0", nil)

	query := parseAdminOpsConcurrencyQuery(ctx)

	assert.Equal(t, 100, query.Limit)
	assert.False(t, query.IncludeUsers)
	assert.EqualValues(t, 0, query.MinActiveOrQueued)
}
