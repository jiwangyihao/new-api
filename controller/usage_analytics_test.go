package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type usageAnalyticsControllerTestDBs struct {
	DB    *gorm.DB
	LogDB *gorm.DB
}

func setupUsageAnalyticsControllerTestDBs(t *testing.T) usageAnalyticsControllerTestDBs {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	oldRedis := common.RedisEnabled

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	safeName := strings.NewReplacer("/", "_", "?", "_", "&", "_", "=", "_", ":", "_").Replace(t.Name())
	businessDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_business?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_logs?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	model.DB = businessDB
	model.LOG_DB = logDB
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		common.RedisEnabled = oldRedis
		if sqlDB, err := businessDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := logDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return usageAnalyticsControllerTestDBs{DB: businessDB, LogDB: logDB}
}

func intPtrForUsageAnalyticsControllerTest(value int) *int { return &value }

func usageAnalyticsControllerNow() int64 { return time.Now().Unix() }

func seedUsageAnalyticsControllerLog(t *testing.T, log *model.Log) {
	t.Helper()
	require.NoError(t, model.LOG_DB.Create(log).Error)
}

func seedUsageAnalyticsControllerToken(t *testing.T, token *model.Token) {
	t.Helper()
	require.NoError(t, model.DB.Create(token).Error)
}

func newUsageAnalyticsTestRouter(userID int) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
	})
	usageAnalyticsRoute := router.Group("/api/usage-analytics")
	{
		usageAnalyticsRoute.GET("/summary", GetUsageAnalyticsSummary)
		usageAnalyticsRoute.GET("/timeseries", GetUsageAnalyticsTimeseries)
		usageAnalyticsRoute.GET("/breakdown", GetUsageAnalyticsBreakdown)
	}
	return router
}

func performUsageAnalyticsRequest(t *testing.T, userID int, rawURL string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, rawURL, nil)
	newUsageAnalyticsTestRouter(userID).ServeHTTP(recorder, request)
	return recorder
}

func parseUsageAnalyticsRawQueryForTest(rawQuery string) (model.UsageAnalyticsQuery, error) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/usage-analytics/summary?"+rawQuery, nil)
	ctx.Set("id", 101)
	return parseUsageAnalyticsQuery(ctx)
}

func TestUsageAnalyticsSummaryDefaultsToRecentSevenDays(t *testing.T) {
	setupUsageAnalyticsControllerTestDBs(t)
	now := usageAnalyticsControllerNow()
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 24*60*60, Type: model.LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsControllerTest(10)})
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 202, CreatedAt: now - 24*60*60, Type: model.LogTypeConsume, TokenId: 2, MeteredTokens: intPtrForUsageAnalyticsControllerTest(999)})

	recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"group_by":"token"`)
	require.Contains(t, recorder.Body.String(), `"total_tokens":10`)
	require.NotContains(t, recorder.Body.String(), "999")
}

func TestUsageAnalyticsRejectsPartialTimeRange(t *testing.T) {
	recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?start_timestamp=1778716800")
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder = performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?end_timestamp=1778716800")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestUsageAnalyticsRejectsRangeOverThirtyOneDays(t *testing.T) {
	recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?start_timestamp=1778716800&end_timestamp=1781481601")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestUsageAnalyticsRejectsUnsupportedPhaseOneParams(t *testing.T) {
	for _, rawURL := range []string{
		"/api/usage-analytics/summary?group_by=endpoint",
		"/api/usage-analytics/summary?billing_source=wallet",
		"/api/usage-analytics/summary?billing_tier=gold",
		"/api/usage-analytics/summary?modality=image",
	} {
		recorder := performUsageAnalyticsRequest(t, 101, rawURL)
		require.Equal(t, http.StatusBadRequest, recorder.Code, rawURL)
		require.Contains(t, recorder.Body.String(), "unsupported")
	}
}

func TestUsageAnalyticsIgnoresUserIDAndUsernameQuery(t *testing.T) {
	setupUsageAnalyticsControllerTestDBs(t)
	now := usageAnalyticsControllerNow()
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsControllerTest(10)})
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 202, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 2, MeteredTokens: intPtrForUsageAnalyticsControllerTest(999)})

	recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?user_id=202&username=other")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"total_tokens":10`)
	require.NotContains(t, recorder.Body.String(), "999")
}

func TestUsageAnalyticsRejectsForeignTokenID(t *testing.T) {
	setupUsageAnalyticsControllerTestDBs(t)
	now := usageAnalyticsControllerNow()
	seedUsageAnalyticsControllerToken(t, &model.Token{Id: 77, UserId: 202, Name: "foreign", Key: "sk-foreign-1234567890"})

	recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?start_timestamp="+strconv.FormatInt(now-60, 10)+"&end_timestamp="+strconv.FormatInt(now, 10)+"&token_ids=77")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "foreign")
	require.NotContains(t, recorder.Body.String(), "sk-foreign")
}

func TestUsageAnalyticsParsesRepeatedParamsBeforeCommaFallback(t *testing.T) {
	query, err := parseUsageAnalyticsRawQueryForTest("model_names=gpt-4&model_names=claude&groups=a%2Cb&groups=default")
	require.NoError(t, err)
	require.Equal(t, []string{"claude", "gpt-4"}, query.ModelNames)
}

func TestUsageAnalyticsParsesCommaFallbackAndLimits(t *testing.T) {
	query, err := parseUsageAnalyticsRawQueryForTest("token_ids=2,1&streams=true,false&statuses=success,error&limit=500&sort_order=asc")
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, query.TokenIDs)
	require.Equal(t, []bool{false, true}, query.Streams)
	require.Equal(t, []string{"error", "success"}, query.Statuses)
	require.Equal(t, 50, query.Limit)
	require.Equal(t, "asc", query.SortOrder)
}

func TestUsageAnalyticsValidatesSortFields(t *testing.T) {
	query, err := parseUsageAnalyticsRawQueryForTest("metric=quota")
	require.NoError(t, err)
	require.Equal(t, "quota", query.SortBy)

	query, err = parseUsageAnalyticsRawQueryForTest("sort_by=request_count&sort_order=asc")
	require.NoError(t, err)
	require.Equal(t, "request_count", query.SortBy)
	require.Equal(t, "asc", query.SortOrder)

	recorder := performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?sort_by=bad")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	recorder = performUsageAnalyticsRequest(t, 101, "/api/usage-analytics/summary?sort_order=sideways")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestUsageAnalyticsRouterRegistersUserRoutes(t *testing.T) {
	setupUsageAnalyticsControllerTestDBs(t)
	router := gin.New()
	api := router.Group("/api")
	usageAnalyticsRoute := api.Group("/usage-analytics")
	usageAnalyticsRoute.Use(func(c *gin.Context) {
		c.Set("id", 101)
	})
	{
		usageAnalyticsRoute.GET("/summary", GetUsageAnalyticsSummary)
		usageAnalyticsRoute.GET("/timeseries", GetUsageAnalyticsTimeseries)
		usageAnalyticsRoute.GET("/breakdown", GetUsageAnalyticsBreakdown)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/usage-analytics/summary", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
}
