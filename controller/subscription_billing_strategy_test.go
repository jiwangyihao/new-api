package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func performUpdateSubscriptionBillingStrategyRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/subscription/self/billing-strategy", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	UpdateSubscriptionBillingStrategy(ctx)
	return recorder
}

func TestUpdateSubscriptionBillingStrategyPersistsAccountSetting(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9891
	settingJSON, err := common.Marshal(map[string]any{
		"codex_pro_mode":         "all",
		"active_subscription_id": 9893,
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "billing-strategy", Status: common.UserStatusEnabled, Setting: string(settingJSON)}).Error)

	recorder := performUpdateSubscriptionBillingStrategyRequest(t, userID, `{"billing_strategy":"active_fallback"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, model.SubscriptionBillingStrategyActiveFallback, data["billing_strategy"])
	setting := requireRawUserSetting(t, userID)
	assert.Equal(t, model.SubscriptionBillingStrategyActiveFallback, setting["subscription_billing_strategy"])
	assert.Equal(t, "all", setting["codex_pro_mode"])
	assert.Equal(t, float64(9893), setting["active_subscription_id"])
}

func TestUpdateSubscriptionBillingStrategyRejectsInvalidValue(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9892
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "invalid-billing-strategy", Status: common.UserStatusEnabled}).Error)

	recorder := performUpdateSubscriptionBillingStrategyRequest(t, userID, `{"billing_strategy":"split_all"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, false, payload["success"])
	var persisted model.User
	require.NoError(t, model.DB.First(&persisted, userID).Error)
	assert.Empty(t, persisted.GetSetting().SubscriptionBillingStrategy)
}

func TestSubscriptionSettingMutationsDoNotOverwriteConcurrentFields(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9894
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "billing-strategy-concurrent", Status: common.UserStatusEnabled}).Error)

	start := make(chan struct{})
	errorsByMutation := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	mutate := func(apply func(setting *dto.UserSetting)) {
		ready.Done()
		<-start
		_, err := model.MutateUserSetting(userID, func(setting *dto.UserSetting) error {
			apply(setting)
			return nil
		})
		errorsByMutation <- err
	}
	go mutate(func(setting *dto.UserSetting) {
		setting.SubscriptionBillingStrategy = model.SubscriptionBillingStrategyTimedFirst
	})
	go mutate(func(setting *dto.UserSetting) {
		setting.CodexProMode = "all"
	})
	ready.Wait()
	close(start)

	require.NoError(t, <-errorsByMutation)
	require.NoError(t, <-errorsByMutation)
	var persisted model.User
	require.NoError(t, model.DB.First(&persisted, userID).Error)
	setting := persisted.GetSetting()
	assert.Equal(t, model.SubscriptionBillingStrategyTimedFirst, setting.SubscriptionBillingStrategy)
	assert.Equal(t, "all", setting.CodexProMode)
}
