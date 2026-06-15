package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func performUpdateCodexProModeRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/subscription/self/codex-pro-mode", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	UpdateCodexProMode(ctx)
	return recorder
}

func subscriptionCodexProData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok, "response data must be an object")
	return data
}

func seedCodexProPaidSubscription(t *testing.T, userID int, planID int, subscriptionID int) {
	t.Helper()
	seedCodexProSubscriptionWithSources(t, userID, planID, subscriptionID, model.SubscriptionGrantOrder, model.SubscriptionGrantOrder)
}

func seedCodexProSubscriptionWithSources(t *testing.T, userID int, planID int, subscriptionID int, grantReason string, source string) {
	t.Helper()
	code := "codex_pro_paid"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id:                planID,
		Title:             "Codex Pro Paid",
		PriceAmount:       19.9,
		Currency:          "CNY",
		DurationUnit:      model.SubscriptionDurationMonth,
		DurationValue:     1,
		Enabled:           true,
		PublicVisible:     true,
		MonthlyTokenLimit: 1000,
		ConcurrencyLimit:  2,
		BusinessCode:      &code,
	}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id:          subscriptionID,
		UserId:      userID,
		PlanId:      planID,
		Status:      "active",
		TokenLimit:  1000,
		TokenUsed:   10,
		StartTime:   common.GetTimestamp() - 60,
		EndTime:     common.GetTimestamp() + 86400,
		GrantReason: grantReason,
		Source:      source,
	}).Error)
}

func userSettingWithCodexProMode(t *testing.T, mode string) string {
	t.Helper()
	setting, err := common.Marshal(map[string]any{"codex_pro_mode": mode})
	require.NoError(t, err)
	return string(setting)
}

func userSettingWithCodexProModeAndBillingPreference(t *testing.T, mode string, billingPreference string) string {
	t.Helper()
	setting, err := common.Marshal(map[string]any{"codex_pro_mode": mode, "billing_preference": billingPreference})
	require.NoError(t, err)
	return string(setting)
}

func requireRawUserSetting(t *testing.T, userID int) map[string]interface{} {
	t.Helper()
	var raw string
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Select("setting").Scan(&raw).Error)
	var setting map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(raw), &setting))
	return setting
}

func setupSubscriptionControllerRedis(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	oldRDB := common.RDB
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
		server.Close()
	})
}

func seedUserCacheForSubscriptionControllerTest(t *testing.T, user model.User) {
	t.Helper()
	require.NoError(t, common.RedisHSetObj(fmt.Sprintf("user:%d", user.Id), user.ToBaseUser(), time.Duration(common.RedisKeyCacheSeconds())*time.Second))
}

func TestGetSubscriptionSelfReturnsDefaultCodexProModeAndEligibilityFields(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9861
	seedSubscriptionSelfSummaryUser(t, userID, "codex_pro_default")
	seedCodexProPaidSubscription(t, userID, 9862, 9863)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	data := subscriptionSelfSummaryData(t, recorder)
	assert.Equal(t, "flexible", data["codex_pro_mode"])
	assert.Equal(t, true, data["codex_pro_eligible"])
	assert.Equal(t, "", data["codex_pro_unavailable_reason"])
}

func TestGetSubscriptionSelfHidesCodexProWhenGloballyDisabled(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	oldHidden := common.CodexProFeaturesHidden
	common.CodexProFeaturesHidden = true
	t.Cleanup(func() { common.CodexProFeaturesHidden = oldHidden })
	const userID = 9961
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_hidden", Status: common.UserStatusEnabled, Setting: userSettingWithCodexProMode(t, "all")}).Error)
	seedCodexProPaidSubscription(t, userID, 9962, 9963)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	data := subscriptionSelfSummaryData(t, recorder)
	assert.Equal(t, true, data["codex_pro_features_hidden"])
	assert.Equal(t, "off", data["codex_pro_mode"])
	assert.Equal(t, false, data["codex_pro_eligible"])
	assert.Equal(t, "features_hidden", data["codex_pro_unavailable_reason"])
	assert.Equal(t, "all", requireRawUserSetting(t, userID)["codex_pro_mode"])
}

func TestGetSubscriptionSelfCodexProEligibilityDoesNotDependOnOffMode(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9864
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_off", Status: common.UserStatusEnabled, Setting: userSettingWithCodexProMode(t, "off")}).Error)
	seedCodexProPaidSubscription(t, userID, 9865, 9866)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	data := subscriptionSelfSummaryData(t, recorder)
	assert.Equal(t, "off", data["codex_pro_mode"])
	assert.Equal(t, true, data["codex_pro_eligible"])
	assert.Equal(t, "", data["codex_pro_unavailable_reason"])
}

func TestGetSubscriptionSelfCodexProEligibilityIgnoresWalletOnlyPreferenceWhenPaidSubscriptionExists(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9868
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_wallet_only_paid", Status: common.UserStatusEnabled, Setting: userSettingWithCodexProModeAndBillingPreference(t, "flexible", "wallet_only")}).Error)
	seedCodexProPaidSubscription(t, userID, 9869, 9870)

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	data := subscriptionSelfSummaryData(t, recorder)
	assert.Equal(t, "wallet_only", data["billing_preference"])
	assert.Equal(t, "flexible", data["codex_pro_mode"])
	assert.Equal(t, true, data["codex_pro_eligible"])
	assert.Equal(t, "", data["codex_pro_unavailable_reason"])
}

func TestGetSubscriptionSelfCodexProEligibilityChecksGrantReasonAndSourceIndependently(t *testing.T) {
	for i, tc := range []struct {
		name        string
		grantReason string
		source      string
		wantReason  string
	}{
		{name: "trial_grant_reason", grantReason: "trial_code", source: model.SubscriptionGrantOrder, wantReason: "trial_subscription"},
		{name: "trial_source", grantReason: model.SubscriptionGrantOrder, source: "invite_trial", wantReason: "trial_subscription"},
		{name: "reward_grant_reason", grantReason: model.SubscriptionGrantMonthlyInviteEntitlement, source: model.SubscriptionGrantOrder, wantReason: "reward_subscription"},
		{name: "reward_source", grantReason: model.SubscriptionGrantOrder, source: model.SubscriptionGrantMonthlyInviteEntitlement, wantReason: "reward_subscription"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupSubscriptionSelfSummaryTestDB(t)
			userID := 9891 + i
			planID := 9901 + i
			subscriptionID := 9911 + i
			require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_source_" + tc.name, Status: common.UserStatusEnabled}).Error)
			seedCodexProSubscriptionWithSources(t, userID, planID, subscriptionID, tc.grantReason, tc.source)

			recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

			data := subscriptionSelfSummaryData(t, recorder)
			assert.Equal(t, false, data["codex_pro_eligible"])
			assert.Equal(t, tc.wantReason, data["codex_pro_unavailable_reason"])
		})
	}
}

func TestGetSubscriptionSelfReturnsCodexProIneligibleForUserWithoutPaidSubscription(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9867
	seedSubscriptionSelfSummaryUser(t, userID, "codex_pro_no_paid")

	recorder := performGetSubscriptionSelfSummaryRequest(t, userID)

	data := subscriptionSelfSummaryData(t, recorder)
	assert.Equal(t, "flexible", data["codex_pro_mode"])
	assert.Equal(t, false, data["codex_pro_eligible"])
	assert.Equal(t, "no_paid_subscription", data["codex_pro_unavailable_reason"])
}

func TestUpdateCodexProModeSavesAllowedModesAndReportsEligibility(t *testing.T) {
	for _, mode := range []string{"all", "flexible", "off"} {
		t.Run(mode, func(t *testing.T) {
			setupSubscriptionSelfSummaryTestDB(t)
			userID := 9870 + len(mode)
			require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_update_" + mode, Status: common.UserStatusEnabled}).Error)
			seedCodexProPaidSubscription(t, userID, 9880+len(mode), 9890+len(mode))

			recorder := performUpdateCodexProModeRequest(t, userID, `{"mode":"`+mode+`"}`)

			data := subscriptionCodexProData(t, recorder)
			assert.Equal(t, mode, data["codex_pro_mode"])
			assert.Equal(t, true, data["codex_pro_eligible"])
			assert.Equal(t, "", data["codex_pro_unavailable_reason"])
			assert.Equal(t, mode, requireRawUserSetting(t, userID)["codex_pro_mode"])
		})
	}
}

func TestUpdateCodexProModeRefreshesCachedUserSetting(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	setupSubscriptionControllerRedis(t)
	const userID = 9921
	oldSetting := userSettingWithCodexProModeAndBillingPreference(t, "all", "wallet_first")
	user := model.User{Id: userID, Username: "codex_pro_cache", Status: common.UserStatusEnabled, Setting: oldSetting}
	require.NoError(t, model.DB.Create(&user).Error)
	seedCodexProPaidSubscription(t, userID, 9922, 9923)
	seedUserCacheForSubscriptionControllerTest(t, user)
	cachedBefore, err := model.GetUserCache(userID)
	require.NoError(t, err)
	assert.Equal(t, "all", cachedBefore.GetSetting().CodexProMode)

	recorder := performUpdateCodexProModeRequest(t, userID, `{"mode":"off"}`)

	data := subscriptionCodexProData(t, recorder)
	assert.Equal(t, "off", data["codex_pro_mode"])
	cachedAfter, err := model.GetUserCache(userID)
	require.NoError(t, err)
	setting := cachedAfter.GetSetting()
	assert.Equal(t, "off", setting.CodexProMode)
	assert.Equal(t, "wallet_first", setting.BillingPreference)
	assert.Equal(t, "off", requireRawUserSetting(t, userID)["codex_pro_mode"])
}

func TestUpdateCodexProModeAllowsSavingModeWhenUserIsNotEligible(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9875
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_update_ineligible", Status: common.UserStatusEnabled}).Error)

	recorder := performUpdateCodexProModeRequest(t, userID, `{"mode":"all"}`)

	data := subscriptionCodexProData(t, recorder)
	assert.Equal(t, "all", data["codex_pro_mode"])
	assert.Equal(t, false, data["codex_pro_eligible"])
	assert.Equal(t, "no_paid_subscription", data["codex_pro_unavailable_reason"])
	assert.Equal(t, "all", requireRawUserSetting(t, userID)["codex_pro_mode"])
}

func TestUpdateCodexProModeReturnsOffWhenGloballyDisabled(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	oldHidden := common.CodexProFeaturesHidden
	common.CodexProFeaturesHidden = true
	t.Cleanup(func() { common.CodexProFeaturesHidden = oldHidden })
	const userID = 9971
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_update_hidden", Status: common.UserStatusEnabled, Setting: userSettingWithCodexProModeAndBillingPreference(t, "all", "wallet_first")}).Error)
	seedCodexProPaidSubscription(t, userID, 9972, 9973)

	recorder := performUpdateCodexProModeRequest(t, userID, `{"mode":"flexible"}`)

	data := subscriptionCodexProData(t, recorder)
	assert.Equal(t, "off", data["codex_pro_mode"])
	assert.Equal(t, false, data["codex_pro_eligible"])
	assert.Equal(t, "features_hidden", data["codex_pro_unavailable_reason"])
	setting := requireRawUserSetting(t, userID)
	assert.Equal(t, "all", setting["codex_pro_mode"])
	assert.Equal(t, "wallet_first", setting["billing_preference"])
}

func TestUpdateCodexProModeHiddenShortCircuitsInvalidMode(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	oldHidden := common.CodexProFeaturesHidden
	common.CodexProFeaturesHidden = true
	t.Cleanup(func() { common.CodexProFeaturesHidden = oldHidden })
	const userID = 9974
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_update_hidden_invalid", Status: common.UserStatusEnabled, Setting: userSettingWithCodexProMode(t, "all")}).Error)

	recorder := performUpdateCodexProModeRequest(t, userID, `{"mode":"codex-pro"}`)

	data := subscriptionCodexProData(t, recorder)
	assert.Equal(t, "off", data["codex_pro_mode"])
	assert.Equal(t, false, data["codex_pro_eligible"])
	assert.Equal(t, "features_hidden", data["codex_pro_unavailable_reason"])
	assert.Equal(t, true, data["codex_pro_features_hidden"])
	assert.Equal(t, "all", requireRawUserSetting(t, userID)["codex_pro_mode"])
}

func TestUpdateCodexProModeRejectsInvalidModeAndKeepsExistingSetting(t *testing.T) {
	setupSubscriptionSelfSummaryTestDB(t)
	const userID = 9876
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "codex_pro_invalid", Status: common.UserStatusEnabled, Setting: userSettingWithCodexProModeAndBillingPreference(t, "all", "wallet_first")}).Error)

	recorder := performUpdateCodexProModeRequest(t, userID, `{"mode":"codex-pro"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, false, payload["success"])
	assert.Equal(t, "参数错误", payload["message"])
	setting := requireRawUserSetting(t, userID)
	assert.Equal(t, "all", setting["codex_pro_mode"])
	assert.Equal(t, "wallet_first", setting["billing_preference"])
}
