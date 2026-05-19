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

func setupSubscriptionActiveResetTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))
}

func performSetActiveSubscriptionRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/subscription/self/active", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	SetActiveSubscription(ctx)
	return recorder
}

func performResetSubscriptionQuotaRequest(t *testing.T, userID int, subID string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/self/"+subID+"/reset-quota", nil)
	ctx.Set("id", userID)
	ctx.Params = gin.Params{{Key: "id", Value: subID}}
	ResetSubscriptionQuota(ctx)
	return recorder
}

func TestSetActiveSubscriptionPersistsUserChoice(t *testing.T) {
	setupSubscriptionActiveResetTestDB(t)
	userID := 9701
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "active_user", Status: common.UserStatusEnabled}).Error)
	code := "pro_monthly"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9702, Title: "Pro", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9703, UserId: userID, PlanId: 9702, Status: "active", TokenLimit: 100, EndTime: common.GetTimestamp() + 86400, GrantReason: "order", Source: "order"}).Error)

	recorder := performSetActiveSubscriptionRequest(t, userID, `{"subscription_id":9703}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"active_subscription_id":9703`)
	user, err := model.GetUserById(userID, false)
	require.NoError(t, err)
	assert.Equal(t, 9703, user.GetSetting().ActiveSubscriptionId)
}

func TestGetSubscriptionSelfReturnsActiveSelectionAndEffectiveRewardEndTime(t *testing.T) {
	setupSubscriptionActiveResetTestDB(t)
	userID := 9721
	user := model.User{Id: userID, Username: "self_active", Status: common.UserStatusEnabled}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = 9724
	user.SetSetting(setting)
	require.NoError(t, model.DB.Create(&user).Error)
	code := "basic_monthly"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9722, Title: "Basic", Enabled: true, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9723, UserId: userID, PlanId: 9722, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: now + 30*86400, GrantReason: "order", Source: "order"}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9724, UserId: userID, PlanId: 9722, Status: "active", TokenLimit: 100, TokenUsed: 0, EndTime: now + 3*86400, GrantReason: "monthly_invite_entitlement", Source: "monthly_invite_entitlement"}).Error)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	data := payload["data"].(map[string]interface{})
	assert.Equal(t, float64(9724), data["active_subscription_id"])
	subscriptions := data["subscriptions"].([]interface{})
	var reward map[string]interface{}
	for _, item := range subscriptions {
		record := item.(map[string]interface{})
		sub := record["subscription"].(map[string]interface{})
		if sub["id"] == float64(9724) {
			reward = sub
			break
		}
	}
	require.NotNil(t, reward)
	assert.Equal(t, true, reward["is_active_selected"])
	assert.Equal(t, "invitation_reward", reward["source_label"])
	assert.Equal(t, float64(now+33*86400), reward["effective_end_time"])
	assert.Equal(t, true, reward["can_reset_quota"])
}

func TestResetSubscriptionQuotaEndpointResetsAndConsumesPaidMonth(t *testing.T) {
	setupSubscriptionActiveResetTestDB(t)
	userID := 9711
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "reset_user", Status: common.UserStatusEnabled}).Error)
	code := "basic_monthly"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9712, Title: "Basic", Enabled: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 100, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	end := common.GetTimestamp() + 70*86400
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9713, UserId: userID, PlanId: 9712, Status: "active", TokenLimit: 100, TokenUsed: 90, EndTime: end, GrantReason: "order", Source: "order"}).Error)

	recorder := performResetSubscriptionQuotaRequest(t, userID, "9713")

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"subscription_id":9713`)
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, 9713).Error)
	assert.Equal(t, int64(0), sub.TokenUsed)
	assert.Less(t, sub.EndTime, end)
}
