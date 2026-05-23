package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionPurchaseDoesNotUpdateUserGroup(t *testing.T) {
	setupSubscriptionBalancePurchaseTestDB(t)
	userID := 9701
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "grouped_buyer", Group: "vip", Quota: int(common.QuotaPerUnit * 100), Status: common.UserStatusEnabled}).Error)
	code := "group-removal-plan"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9702, Title: "No group", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, UpgradeGroup: "svip", MonthlyTokenLimit: 1000, ConcurrencyLimit: 1, BusinessCode: &code}).Error)

	recorder := performBalancePayRequest(t, userID, `{"plan_id":9702,"idempotency_key":"no-group-change"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"message":"success"`)
	var updated model.User
	require.NoError(t, model.DB.First(&updated, userID).Error)
	require.Equal(t, "vip", updated.Group)
	var sub model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9702).First(&sub).Error)
	require.Empty(t, sub.UpgradeGroup)
	require.Empty(t, sub.PrevUserGroup)
}

func TestTopupAmountIgnoresGroupRatio(t *testing.T) {
	originalTopupRatio := common.TopupGroupRatio2JSONString()
	originalPrice := operation_setting.Price
	originalStripeUnitPrice := setting.StripeUnitPrice
	originalWaffoUnitPrice := setting.WaffoUnitPrice
	originalWaffoPancakeUnitPrice := setting.WaffoPancakeUnitPrice
	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscounts := make(map[int]float64, len(operation_setting.GetPaymentSetting().AmountDiscount))
	for k, v := range operation_setting.GetPaymentSetting().AmountDiscount {
		originalDiscounts[k] = v
	}
	t.Cleanup(func() {
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupRatio))
		operation_setting.Price = originalPrice
		setting.StripeUnitPrice = originalStripeUnitPrice
		setting.WaffoUnitPrice = originalWaffoUnitPrice
		setting.WaffoPancakeUnitPrice = originalWaffoPancakeUnitPrice
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
	})
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":9,"default":1}`))
	operation_setting.Price = 2
	setting.StripeUnitPrice = 3
	setting.WaffoUnitPrice = 4
	setting.WaffoPancakeUnitPrice = 5
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}

	require.Equal(t, getPayMoney(10, "default"), getPayMoney(10, "vip"))
	require.Equal(t, getStripePayMoney(10, "default"), getStripePayMoney(10, "vip"))
	require.Equal(t, GetChargedAmount(10, model.User{Group: "default"}), GetChargedAmount(10, model.User{Group: "vip"}))
	require.Equal(t, getWaffoPayMoney(10, "default"), getWaffoPayMoney(10, "vip"))
	require.Equal(t, getWaffoPancakePayMoney(10, "default"), getWaffoPancakePayMoney(10, "vip"))
}

func TestUserGroupRemovalCompatibility(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.InvitationMonthlyEntitlement{}))
	require.NoError(t, db.Create(&model.User{Id: 9801, Username: "legacy_group_user", Password: "password", DisplayName: "Legacy Group", Group: "vip", Status: common.UserStatusEnabled, AffCode: "legacy-group"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "legacy", Model: "gpt-user-groupless", ChannelId: 19801, Enabled: true}).Error)

	searchCtx, searchRecorder := newAuthenticatedContext(t, http.MethodGet, "/api/user/search?keyword=legacy_group_user&group=missing&user_group=missing&p=1&size=10", nil, 1)
	SearchUsers(searchCtx)
	require.Equal(t, http.StatusOK, searchRecorder.Code, searchRecorder.Body.String())
	require.Contains(t, searchRecorder.Body.String(), "legacy_group_user")
	require.NotContains(t, searchRecorder.Body.String(), `"group"`)

	detailCtx, detailRecorder := newAuthenticatedContext(t, http.MethodGet, "/api/user/self", nil, 9801)
	GetSelf(detailCtx)
	require.Equal(t, http.StatusOK, detailRecorder.Code, detailRecorder.Body.String())
	require.NotContains(t, detailRecorder.Body.String(), `"group"`)

	modelsRecorder := httptest.NewRecorder()
	modelsCtx, _ := gin.CreateTestContext(modelsRecorder)
	modelsCtx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models", nil)
	modelsCtx.Params = gin.Params{{Key: "id", Value: "9801"}}
	GetUserModels(modelsCtx)
	require.Equal(t, http.StatusOK, modelsRecorder.Code, modelsRecorder.Body.String())
	require.Contains(t, modelsRecorder.Body.String(), "gpt-user-groupless")

	updateBody := map[string]any{"id": 9801, "username": "legacy_group_user", "display_name": "Legacy Group Updated", "role": common.RoleCommonUser, "status": common.UserStatusEnabled, "group": "svip"}
	updateCtx, updateRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/user/", updateBody, common.RoleAdminUser)
	updateCtx.Set("role", common.RoleAdminUser)
	UpdateUser(updateCtx)
	require.Equal(t, http.StatusOK, updateRecorder.Code, updateRecorder.Body.String())
	var updated model.User
	require.NoError(t, db.First(&updated, 9801).Error)
	require.Equal(t, "vip", updated.Group)
	require.Equal(t, "Legacy Group Updated", updated.DisplayName)

	createBody := map[string]any{"username": "created_group_user", "password": "Password123!", "display_name": "Created", "role": common.RoleCommonUser, "group": "svip"}
	createCtx, createRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/user/", createBody, common.RoleAdminUser)
	createCtx.Set("role", common.RoleAdminUser)
	CreateUser(createCtx)
	require.Equal(t, http.StatusOK, createRecorder.Code, createRecorder.Body.String())
	var created model.User
	require.NoError(t, db.Where("username = ?", "created_group_user").First(&created).Error)
	require.Empty(t, created.Group)
}
