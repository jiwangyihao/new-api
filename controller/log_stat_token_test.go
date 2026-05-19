package controller

import (
	"net/http"
	"net/http/httptest"
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
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
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

	GetLogsSelfStat(ctx)

	response := decodeLogStatTokenResponse(t, recorder)
	assert.Equal(t, 100, response.Data.Quota)
	assert.Equal(t, 80, response.Data.TotalTokens)
	assert.Equal(t, 80, response.Data.Tpm)
	assert.Equal(t, 1, response.Data.Rpm)
}
