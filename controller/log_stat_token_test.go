package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLogStatTokenControllerTestDB(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	model.ResetLogStatCacheForTest()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
		model.ResetLogStatCacheForTest()
	})
}

func intPtrForControllerLogStatTokenTest(value int) *int {
	return &value
}

type logStatTokenResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Quota       int `json:"quota"`
		TotalTokens int `json:"total_tokens"`
		Rpm         int `json:"rpm"`
		Tpm         int `json:"tpm"`
	} `json:"data"`
}

func decodeLogStatTokenResponse(t *testing.T, recorder *httptest.ResponseRecorder) logStatTokenResponse {
	t.Helper()

	require.Equal(t, http.StatusOK, recorder.Code)
	var response logStatTokenResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	return response
}

func TestLogStatControllerPassesRefreshOptionToModel(t *testing.T) {
	content, err := os.ReadFile(filepath.Clean(filepath.Join("..", "controller", "log.go")))
	require.NoError(t, err)
	text := string(content)
	assert.Contains(t, text, `refresh := c.Query("refresh") == "true"`)

	for _, functionName := range []string{"func GetLogsStat", "func GetLogsSelfStat"} {
		start := strings.Index(text, functionName)
		require.NotEqual(t, -1, start, functionName)
		next := strings.Index(text[start+len(functionName):], "\nfunc ")
		end := len(text)
		if next >= 0 {
			end = start + len(functionName) + next
		}
		body := text[start:end]
		assert.Contains(t, body, "SumUsedQuotaWithFilterOptions", functionName)
		assert.Contains(t, body, "LogStatOptions{Refresh: refresh}", functionName)
	}
}

func TestGetLogsStatReturnsTotalTokensAndTpm(t *testing.T) {
	setupLogStatTokenControllerTestDB(t)

	now := time.Now()
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:           4001,
		Username:         "admin-log-stat-user",
		CreatedAt:        now.Add(-10 * time.Second).Unix(),
		Type:             model.LogTypeConsume,
		ModelName:        "admin-log-stat-model",
		Quota:            100,
		PromptTokens:     10,
		CompletionTokens: 5,
		MeteredTokens:    intPtrForControllerLogStatTokenTest(80),
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/stat?type=2&start_timestamp=0&end_timestamp=0", nil)
	ctx.Set("id", 4001)
	ctx.Set("username", "root")
	ctx.Set("role", common.RoleRootUser)

	GetLogsStat(ctx)

	response := decodeLogStatTokenResponse(t, recorder)
	assert.Equal(t, 100, response.Data.Quota)
	assert.Equal(t, 80, response.Data.TotalTokens)
	assert.Equal(t, 80, response.Data.Tpm)
	assert.Equal(t, 1, response.Data.Rpm)
}

func TestGetLogsSelfStatReturnsTotalTokensAndTpm(t *testing.T) {
	setupLogStatTokenControllerTestDB(t)

	now := time.Now()
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:           5001,
		Username:         "self-log-stat-user",
		CreatedAt:        now.Add(-10 * time.Second).Unix(),
		Type:             model.LogTypeConsume,
		ModelName:        "self-log-stat-model",
		Quota:            100,
		PromptTokens:     10,
		CompletionTokens: 5,
		MeteredTokens:    intPtrForControllerLogStatTokenTest(80),
	}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:           5002,
		Username:         "other-log-stat-user",
		CreatedAt:        now.Add(-10 * time.Second).Unix(),
		Type:             model.LogTypeConsume,
		ModelName:        "other-log-stat-model",
		Quota:            200,
		PromptTokens:     20,
		CompletionTokens: 10,
		MeteredTokens:    intPtrForControllerLogStatTokenTest(160),
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/stat?type=2", nil)
	ctx.Set("username", "self-log-stat-user")
	ctx.Set("id", 5001)

	GetLogsSelfStat(ctx)

	response := decodeLogStatTokenResponse(t, recorder)
	assert.Equal(t, 100, response.Data.Quota)
	assert.Equal(t, 80, response.Data.TotalTokens)
	assert.Equal(t, 80, response.Data.Tpm)
	assert.Equal(t, 1, response.Data.Rpm)
}

func TestLogStatRefreshBypassesAndUpdatesCache(t *testing.T) {
	setupLogStatTokenControllerTestDB(t)

	now := time.Now().Unix()
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:        6001,
		Username:      "refresh-log-stat-user",
		CreatedAt:     now - 10,
		Type:          model.LogTypeConsume,
		Quota:         10,
		MeteredTokens: intPtrForControllerLogStatTokenTest(10),
	}).Error)

	first := performSelfLogRequest(t, 6001, "/api/log/self/stat?type=2")
	assert.Equal(t, 10, decodeLogStatTokenResponse(t, first).Data.TotalTokens)

	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:        6001,
		Username:      "refresh-log-stat-user",
		CreatedAt:     now - 9,
		Type:          model.LogTypeConsume,
		Quota:         10,
		MeteredTokens: intPtrForControllerLogStatTokenTest(10),
	}).Error)

	cached := performSelfLogRequest(t, 6001, "/api/log/self/stat?type=2")
	assert.Equal(t, 10, decodeLogStatTokenResponse(t, cached).Data.TotalTokens)

	refreshed := performSelfLogRequest(t, 6001, "/api/log/self/stat?type=2&refresh=true")
	assert.Equal(t, 20, decodeLogStatTokenResponse(t, refreshed).Data.TotalTokens)

	updatedCache := performSelfLogRequest(t, 6001, "/api/log/self/stat?type=2")
	assert.Equal(t, 20, decodeLogStatTokenResponse(t, updatedCache).Data.TotalTokens)
}

func TestAdminLogStatRefreshBypassesAndUpdatesCache(t *testing.T) {
	setupLogStatTokenControllerTestDB(t)

	now := time.Now().Unix()
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:        7001,
		Username:      "admin-refresh-log-stat-user",
		CreatedAt:     now - 10,
		Type:          model.LogTypeConsume,
		Quota:         10,
		MeteredTokens: intPtrForControllerLogStatTokenTest(10),
	}).Error)

	firstCtx, first := newAuthenticatedContext(t, http.MethodGet, "/api/log/stat?type=2&user_id=7001", nil, 1)
	GetLogsStat(firstCtx)
	assert.Equal(t, 10, decodeLogStatTokenResponse(t, first).Data.TotalTokens)

	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:        7001,
		Username:      "admin-refresh-log-stat-user",
		CreatedAt:     now - 9,
		Type:          model.LogTypeConsume,
		Quota:         10,
		MeteredTokens: intPtrForControllerLogStatTokenTest(10),
	}).Error)

	cachedCtx, cached := newAuthenticatedContext(t, http.MethodGet, "/api/log/stat?type=2&user_id=7001", nil, 1)
	GetLogsStat(cachedCtx)
	assert.Equal(t, 10, decodeLogStatTokenResponse(t, cached).Data.TotalTokens)

	refreshCtx, refreshed := newAuthenticatedContext(t, http.MethodGet, "/api/log/stat?type=2&user_id=7001&refresh=true", nil, 1)
	GetLogsStat(refreshCtx)
	assert.Equal(t, 20, decodeLogStatTokenResponse(t, refreshed).Data.TotalTokens)

	updatedCtx, updatedCache := newAuthenticatedContext(t, http.MethodGet, "/api/log/stat?type=2&user_id=7001", nil, 1)
	GetLogsStat(updatedCtx)
	assert.Equal(t, 20, decodeLogStatTokenResponse(t, updatedCache).Data.TotalTokens)
}
