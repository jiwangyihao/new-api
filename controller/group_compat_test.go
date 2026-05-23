package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupEndpointsReturnNoopCompatibilityPayloads(t *testing.T) {
	setupTokenControllerTestDB(t)

	for _, target := range []string{"/api/group", "/api/group/"} {
		ctx, recorder := newAuthenticatedContext(t, http.MethodGet, target, nil, 1)
		GetGroups(ctx)

		var groupResp struct {
			Success bool     `json:"success"`
			Message string   `json:"message"`
			Data    []string `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &groupResp), target)
		require.Equal(t, http.StatusOK, recorder.Code, target)
		require.True(t, groupResp.Success, target)
		require.Empty(t, groupResp.Message, target)
		require.Empty(t, groupResp.Data, target)
	}

	for _, target := range []string{"/api/user/groups", "/api/user/self/groups"} {
		ctx, recorder := newAuthenticatedContext(t, http.MethodGet, target, nil, 1)
		GetUserGroups(ctx)

		var userResp struct {
			Success bool                   `json:"success"`
			Message string                 `json:"message"`
			Data    map[string]interface{} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &userResp), target)
		require.Equal(t, http.StatusOK, recorder.Code, target)
		require.True(t, userResp.Success, target)
		require.Empty(t, userResp.Message, target)
		require.Empty(t, userResp.Data, target)
	}
}

func TestGroupRouteAcceptsSlashlessAndSlashPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/group", GetGroups)
	router.GET("/api/group/", GetGroups)

	for _, target := range []string{"/api/group", "/api/group/"} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		router.ServeHTTP(recorder, req)

		var groupResp struct {
			Success bool     `json:"success"`
			Data    []string `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &groupResp), target)
		require.Equal(t, http.StatusOK, recorder.Code, target)
		require.True(t, groupResp.Success, target)
		require.Empty(t, groupResp.Data, target)
	}
}
