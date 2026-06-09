package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func performUpdateSubscriptionPreferenceRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/subscription/self/preference", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	UpdateSubscriptionPreference(ctx)
	return recorder
}

func TestUpdateSubscriptionPreferencePreservesCodexProMode(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9881
	settingJSON, err := common.Marshal(map[string]any{"codex_pro_mode": "all", "active_subscription_id": 9883})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_preference", Status: common.UserStatusEnabled, Setting: string(settingJSON)}).Error)

	recorder := performUpdateSubscriptionPreferenceRequest(t, userID, `{"billing_preference":"wallet_only"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	setting := requireRawUserSetting(t, userID)
	assert.Equal(t, "all", setting["codex_pro_mode"])
	assert.Equal(t, "wallet_only", setting["billing_preference"])
	assert.Equal(t, float64(9883), setting["active_subscription_id"])
}
