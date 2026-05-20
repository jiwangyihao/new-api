package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newSelfLogTestRouter(userID int) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
		c.Set("username", "self-log-user")
	})
	router.GET("/api/log/self", GetUserLogs)
	router.GET("/api/log/self/stat", GetLogsSelfStat)
	return router
}

func performSelfLogRequest(t *testing.T, userID int, rawURL string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, rawURL, nil)
	newSelfLogTestRouter(userID).ServeHTTP(recorder, request)
	return recorder
}

func TestSelfLogsFiltersByTokenIDStreamAndStatus(t *testing.T) {
	setupUsageAnalyticsControllerTestDBs(t)
	now := usageAnalyticsControllerNow()
	seedUsageAnalyticsControllerToken(t, &model.Token{Id: 11, UserId: 101, Name: "owned", Key: "sk-owned-1234567890"})
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 11, IsStream: true, MeteredTokens: intPtrForUsageAnalyticsControllerTest(10)})
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 9, Type: model.LogTypeError, TokenId: 11, IsStream: false})

	list := performSelfLogRequest(t, 101, "/api/log/self?token_id=11&is_stream=true&status=success")
	require.Equal(t, http.StatusOK, list.Code)
	require.Contains(t, list.Body.String(), `"token_id":11`)
	require.NotContains(t, list.Body.String(), `"type":`+strconv.Itoa(model.LogTypeError))

	stat := performSelfLogRequest(t, 101, "/api/log/self/stat?token_id=11&is_stream=true&status=success")
	require.Equal(t, http.StatusOK, stat.Code)
	require.Contains(t, stat.Body.String(), `"total_tokens":10`)
}

func TestSelfLogsStatusConflictsWithType(t *testing.T) {
	recorder := performSelfLogRequest(t, 101, "/api/log/self?status=success&type="+strconv.Itoa(model.LogTypeError))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "status conflicts with type")
}

func TestSelfLogsRejectsForeignTokenID(t *testing.T) {
	for _, path := range []string{"/api/log/self?token_id=12", "/api/log/self/stat?token_id=12"} {
		t.Run(path, func(t *testing.T) {
			setupUsageAnalyticsControllerTestDBs(t)
			seedUsageAnalyticsControllerToken(t, &model.Token{Id: 12, UserId: 202, Name: "foreign", Key: "sk-foreign-1234567890"})

			recorder := performSelfLogRequest(t, 101, path)
			require.Equal(t, http.StatusBadRequest, recorder.Code, path)
			require.NotContains(t, recorder.Body.String(), "foreign")
			require.NotContains(t, recorder.Body.String(), "sk-foreign")
		})
	}
}

func TestSelfLogsAllowsDeletedTokenWithOwnHistory(t *testing.T) {
	for _, path := range []string{"/api/log/self?token_id=13", "/api/log/self/stat?token_id=13"} {
		t.Run(path, func(t *testing.T) {
			setupUsageAnalyticsControllerTestDBs(t)
			now := usageAnalyticsControllerNow()
			token := &model.Token{Id: 13, UserId: 101, Name: "deleted", Key: "sk-deleted-1234567890"}
			seedUsageAnalyticsControllerToken(t, token)
			require.NoError(t, model.DB.Delete(token).Error)
			seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 13, TokenName: "deleted-history", MeteredTokens: intPtrForUsageAnalyticsControllerTest(10)})

			recorder := performSelfLogRequest(t, 101, path)
			require.Equal(t, http.StatusOK, recorder.Code, path)
			require.Contains(t, recorder.Body.String(), `"token_id":13`)
		})
	}
}

func TestSelfLogsStatUsesUserIDInsteadOfUsername(t *testing.T) {
	setupUsageAnalyticsControllerTestDBs(t)
	now := usageAnalyticsControllerNow()
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, Username: "same", CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsControllerTest(10)})
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 202, Username: "same", CreatedAt: now - 9, Type: model.LogTypeConsume, TokenId: 2, MeteredTokens: intPtrForUsageAnalyticsControllerTest(999)})

	recorder := performSelfLogRequest(t, 101, "/api/log/self/stat?username=same")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"total_tokens":10`)
	require.NotContains(t, recorder.Body.String(), "999")
}

func TestAdminLogsFiltersByTokenIDStreamAndStatus(t *testing.T) {
	setupUsageAnalyticsControllerTestDBs(t)
	now := usageAnalyticsControllerNow()
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 10, Type: model.LogTypeConsume, TokenId: 21, IsStream: true, MeteredTokens: intPtrForUsageAnalyticsControllerTest(10)})
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 9, Type: model.LogTypeError, TokenId: 21, IsStream: true, MeteredTokens: intPtrForUsageAnalyticsControllerTest(999)})
	seedUsageAnalyticsControllerLog(t, &model.Log{UserId: 101, CreatedAt: now - 8, Type: model.LogTypeConsume, TokenId: 22, IsStream: false, MeteredTokens: intPtrForUsageAnalyticsControllerTest(777)})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/log?token_id=21&is_stream=true&status=success", nil, 1)
	GetAllLogs(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"token_id":21`)
	require.NotContains(t, recorder.Body.String(), `"token_id":22`)
	require.NotContains(t, recorder.Body.String(), `"type":`+strconv.Itoa(model.LogTypeError))

	ctx, recorder = newAuthenticatedContext(t, http.MethodGet, "/api/log/stat?token_id=21&is_stream=true&status=success", nil, 1)
	GetLogsStat(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"total_tokens":10`)
	require.NotContains(t, recorder.Body.String(), "999")
	require.NotContains(t, recorder.Body.String(), "777")
}

func TestAdminLogsStatusConflictsWithType(t *testing.T) {
	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/log?status=success&type="+strconv.Itoa(model.LogTypeError), nil, 1)
	GetAllLogs(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "status conflicts with type")

	ctx, recorder = newAuthenticatedContext(t, http.MethodGet, "/api/log/stat?status=success&type="+strconv.Itoa(model.LogTypeError), nil, 1)
	GetLogsStat(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "status conflicts with type")
}
