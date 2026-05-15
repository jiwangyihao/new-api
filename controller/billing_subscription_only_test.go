package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBillingSubscriptionOnlyTestDB(t *testing.T) {
	t.Helper()
	initModelListColumnNames(t)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))
}

func withSubscriptionBillingTokenDisplay(t *testing.T) {
	t.Helper()
	oldDisplayTokenStat := common.DisplayTokenStatEnabled
	oldQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.DisplayTokenStatEnabled = false
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	t.Cleanup(func() {
		common.DisplayTokenStatEnabled = oldDisplayTokenStat
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldQuotaDisplayType
	})
}

func TestOpenAIBillingSubscriptionUsesActiveSubscriptionTokens(t *testing.T) {
	setupBillingSubscriptionOnlyTestDB(t)
	withSubscriptionBillingTokenDisplay(t)
	const userID = 9401
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "billing_sub", Quota: 99999900, UsedQuota: 123, Status: common.UserStatusEnabled}).Error)
	code := "billing-basic"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9402, Title: "Basic", Enabled: true, MonthlyTokenLimit: 3210, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9403, UserId: userID, PlanId: 9402, TokenLimit: 3210, TokenUsed: 250, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order"}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/dashboard/billing/subscription", nil)
	ctx.Set("id", userID)

	GetSubscription(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"hard_limit_usd":3210`)
	assert.NotContains(t, body, "999999")
}

func TestOpenAIUsageUsesSubscriptionTokenUsed(t *testing.T) {
	setupBillingSubscriptionOnlyTestDB(t)
	withSubscriptionBillingTokenDisplay(t)
	const userID = 9411
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "usage_sub", Quota: 99999900, UsedQuota: 888888, Status: common.UserStatusEnabled}).Error)
	code := "usage-basic"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9412, Title: "Basic", Enabled: true, MonthlyTokenLimit: 2000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9413, UserId: userID, PlanId: 9412, TokenLimit: 2000, TokenUsed: 333, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), GrantReason: "order"}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/dashboard/billing/usage", nil)
	ctx.Set("id", userID)

	GetUsage(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "33300")
	assert.NotContains(t, recorder.Body.String(), "888888")
}
