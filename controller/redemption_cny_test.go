package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRedemptionCNYTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Redemption{}, &model.Log{}, &model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}))

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})
}

func TestRedemptionCNYAmountToQuotaUsesOneToOneWalletBalance(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	quota, err := redemptionCNYAmountToQuota(40)

	require.NoError(t, err)
	assert.Equal(t, int(common.QuotaPerUnit*40), quota)
}

func TestAddRedemptionStoresCNYAmountAsWalletQuota(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	redemption := model.Redemption{Name: "forty-cny", Quota: 40, Count: 1}
	created, err := buildRedemptionsForCreate(1, redemption, func() string { return "fixed-redemption-key" })

	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, int(common.QuotaPerUnit*40), created[0].Quota)
}

func TestUpdateRedemptionStoresCNYAmountAsWalletQuota(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	existing := &model.Redemption{Name: "old", Quota: 1, Count: 1}
	update := model.Redemption{Name: "new", Quota: 40, ExpiredTime: 0}
	err := applyRedemptionUpdate(existing, update)

	require.NoError(t, err)
	assert.Equal(t, int(common.QuotaPerUnit*40), existing.Quota)
}

func TestAddRedemptionStoresSubscriptionPlanReference(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	code := "redemption-plan"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9602, Title: "Redemption Plan", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 2, BusinessCode: &code}).Error)

	redemption := model.Redemption{Name: "plan-code", Type: model.RedemptionTypeSubscription, PlanId: 9602, Quota: 40, Count: 1}
	created, err := buildRedemptionsForCreate(1, redemption, func() string { return "fixed-plan-key" })

	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, model.RedemptionTypeSubscription, created[0].Type)
	assert.Equal(t, 9602, created[0].PlanId)
	assert.Zero(t, created[0].Quota)
}

func TestAddRedemptionRejectsMissingSubscriptionPlan(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	_, err := buildRedemptionsForCreate(1, model.Redemption{Name: "bad-plan", Type: model.RedemptionTypeSubscription, Count: 1}, func() string { return "unused" })

	require.Error(t, err)
	assert.Contains(t, err.Error(), "套餐不存在")
}

func TestUpdateRedemptionStoresSubscriptionPlanReference(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	code := "update-redemption-plan"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9603, Title: "Updated Plan", PriceAmount: 50, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 2000, ConcurrencyLimit: 3, BusinessCode: &code}).Error)
	existing := &model.Redemption{Name: "old", Type: model.RedemptionTypeWallet, Quota: 1, Count: 1}
	update := model.Redemption{Name: "new", Type: model.RedemptionTypeSubscription, PlanId: 9603, Quota: 40, ExpiredTime: 0}

	err := applyRedemptionUpdate(existing, update)

	require.NoError(t, err)
	assert.Equal(t, model.RedemptionTypeSubscription, existing.Type)
	assert.Equal(t, 9603, existing.PlanId)
	assert.Zero(t, existing.Quota)
}

func TestRedeemSubscriptionCodeCreatesUserSubscription(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	userID := 9604
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "redeem_plan", Quota: int(common.QuotaPerUnit * 10), Status: common.UserStatusEnabled}).Error)
	code := "redeem-subscription"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9605, Title: "Redeem Subscription", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 7, MonthlyTokenLimit: 3000, ConcurrencyLimit: 4, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.Redemption{UserId: 1, Name: "sub", Key: "sub-key", Type: model.RedemptionTypeSubscription, PlanId: 9605, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}).Error)

	result, err := model.Redeem("sub-key", userID)

	require.NoError(t, err)
	assert.Equal(t, model.RedemptionTypeSubscription, result.Type)
	assert.Zero(t, result.Quota)
	require.NotNil(t, result.Plan)
	assert.Equal(t, 9605, result.Plan.Id)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, int(common.QuotaPerUnit*10), user.Quota)
	var sub model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, 9605).First(&sub).Error)
	assert.Equal(t, "redemption", sub.GrantReason)
	assert.Equal(t, int64(3000), sub.TokenLimit)
	assert.Equal(t, 4, sub.ConcurrencyLimit)
	assert.Greater(t, sub.EndTime, time.Now().Unix())
	var redeemed model.Redemption
	require.NoError(t, model.DB.Where("`key` = ?", "sub-key").First(&redeemed).Error)
	assert.Equal(t, common.RedemptionCodeStatusUsed, redeemed.Status)
	assert.Equal(t, userID, redeemed.UsedUserId)
}

func TestRedeemWalletCodeResponseStaysBackwardCompatible(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	userID := 9606
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "redeem_wallet", Quota: 0, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Redemption{UserId: 1, Name: "wallet", Key: "wallet-key", Type: model.RedemptionTypeWallet, Quota: int(common.QuotaPerUnit * 12), Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup", strings.NewReader(`{"key":"wallet-key"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)

	TopUp(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"type":"wallet"`)
	assert.Contains(t, recorder.Body.String(), `"quota":6000000`)
	assert.Contains(t, recorder.Body.String(), `"data":6000000`)
}

func TestRedeemSubscriptionCodeResponseIncludesPlanResult(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	userID := 9609
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "redeem_plan_response", Status: common.UserStatusEnabled}).Error)
	code := "redeem-response"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9610, Title: "Response Plan", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 7, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.Redemption{UserId: 1, Name: "sub-response", Key: "sub-response-key", Type: model.RedemptionTypeSubscription, PlanId: 9610, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup", strings.NewReader(`{"key":"sub-response-key"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)

	TopUp(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"type":"subscription"`)
	assert.Contains(t, recorder.Body.String(), `"title":"Response Plan"`)
}

func TestRedeemReturnsOriginalErrorForSubscriptionLimit(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	userID := 9607
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "redeem_limit", Status: common.UserStatusEnabled}).Error)
	code := "redeem-limit"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9608, Title: "Limit", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 7, MaxPurchasePerUser: 1, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{UserId: userID, PlanId: 9608, Status: "active", StartTime: common.GetTimestamp() - 10, EndTime: common.GetTimestamp() + 3600, GrantReason: "order"}).Error)
	require.NoError(t, model.DB.Create(&model.Redemption{UserId: 1, Name: "limit", Key: "limit-key", Type: model.RedemptionTypeSubscription, PlanId: 9608, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}).Error)

	_, err := model.Redeem("limit-key", userID)

	require.Error(t, err)
	assert.True(t, errors.Is(err, model.ErrRedeemFailed))
	var redeemed model.Redemption
	require.NoError(t, model.DB.Where("`key` = ?", "limit-key").First(&redeemed).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, redeemed.Status)
}
