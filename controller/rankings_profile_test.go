package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func performUpdateSelfRankingsDisplayName(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/self", bytes.NewBufferString(body))

	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	UpdateSelf(ctx)
	return recorder
}

func performUpdateUserSetting(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	UpdateUserSetting(ctx)
	return recorder
}

func TestUpdateSelfRankingsDisplayNamePersistsInUserSettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Id: 9961, Username: "rank-profile", DisplayName: "Profile Name", Status: common.UserStatusEnabled}
	user.SetSetting(dto.UserSetting{Language: "zh"})
	require.NoError(t, model.DB.Create(&user).Error)

	recorder := performUpdateSelfRankingsDisplayName(t, user.Id, `{"rankings_display_name":"  榜单玩家  "}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	updated, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	settings := updated.GetSetting()
	assert.Equal(t, "榜单玩家", settings.RankingsDisplayName)
	assert.Equal(t, "zh", settings.Language)
	assert.Equal(t, "Profile Name", updated.DisplayName)
}

func TestUpdateSelfRankingsDisplayNameRejectsOverlongName(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Id: 9962, Username: "rank-profile-long", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&user).Error)

	recorder := performUpdateSelfRankingsDisplayName(t, user.Id, `{"rankings_display_name":"123456789012345678901"}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	updated, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Empty(t, updated.GetSetting().RankingsDisplayName)
}

func TestUpdateUserSettingPreservesRankingsDisplayName(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Id: 9963, Username: "rank-profile-setting", Status: common.UserStatusEnabled}
	user.SetSetting(dto.UserSetting{RankingsDisplayName: "榜单玩家", Language: "zh"})
	require.NoError(t, model.DB.Create(&user).Error)

	recorder := performUpdateUserSetting(t, user.Id, `{"notify_type":"email","quota_warning_threshold":0.5,"notification_email":"rank@example.com"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	updated, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	settings := updated.GetSetting()
	assert.Equal(t, "榜单玩家", settings.RankingsDisplayName)
	assert.Equal(t, "email", settings.NotifyType)
	assert.Equal(t, "rank@example.com", settings.NotificationEmail)
}

func TestUpdateUserSettingPreservesCodexProMode(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Id: 9966, Username: "codex-pro-setting", Status: common.UserStatusEnabled}
	settingJSON, err := common.Marshal(map[string]any{"codex_pro_mode": "all", "billing_preference": "wallet_first"})
	require.NoError(t, err)
	user.Setting = string(settingJSON)
	require.NoError(t, model.DB.Create(&user).Error)

	recorder := performUpdateUserSetting(t, user.Id, `{"notify_type":"email","quota_warning_threshold":0.5,"notification_email":"codex@example.com"}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var rawSetting string
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Select("setting").Scan(&rawSetting).Error)
	var preserved map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(rawSetting), &preserved))
	assert.Equal(t, "all", preserved["codex_pro_mode"])
	assert.Equal(t, "wallet_first", preserved["billing_preference"])
	updated, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	settings := updated.GetSetting()
	assert.Equal(t, "email", settings.NotifyType)
	assert.Equal(t, "codex@example.com", settings.NotificationEmail)
}

func TestUpdateSelfRankingsDisplayNameFlushesRankingsCache(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.QuotaData{}, &model.Log{}))
	service.FlushRankingsCacheForTest()
	freeCode := "profile-cache-free"
	plan := &model.SubscriptionPlan{Id: 9964, Title: "Profile Cache Free", Enabled: true, PriceAmount: 0, MonthlyTokenLimit: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	user := model.User{Id: 9965, Username: "rank-profile-cache", Status: common.UserStatusEnabled, AffCode: "aff9965"}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9966, UserId: user.Id, PlanId: plan.Id, Status: "active", TokenUsed: 100, StartTime: common.GetTimestamp() - 60, EndTime: common.GetTimestamp() + 60, GrantReason: "trial_code"}).Error)

	before, err := service.GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, before.FreeUsers, 1)
	assert.False(t, before.FreeUsers[0].Named)

	recorder := performUpdateSelfRankingsDisplayName(t, user.Id, `{"rankings_display_name":"缓存刷新玩家"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	after, err := service.GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, after.FreeUsers, 1)
	assert.Equal(t, "缓存刷新玩家", after.FreeUsers[0].DisplayName)
	assert.True(t, after.FreeUsers[0].Named)
}
