package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRedemptionCNYTestDB(t *testing.T) {
	t.Helper()
	require.NoError(t, i18n.Init())
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Redemption{}, &model.Log{}, &model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.TimedSubscriptionValuationGrant{}, &model.InvitationRewardEvent{}))

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})
}

func TestValidateRedemptionWalletCentsUsesSubmittedCents(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	quota, err := validateRedemptionWalletCents(4000)

	require.NoError(t, err)
	assert.Equal(t, 4000, quota)
}

func TestBuildWalletRedemptionUsesSubmittedCents(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	redemption := model.Redemption{Name: "forty-cny", Type: model.RedemptionTypeWallet, Quota: 4000, Count: 1}
	created, err := buildRedemptionsForCreate(1, redemption, func() string { return "fixed-redemption-key" })

	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, 4000, created[0].Quota)
}

func TestAddRedemptionAssignsOneBatchIdPerCreateRequest(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	keys := []string{"batch-key-a", "batch-key-b", "batch-key-c"}
	next := 0

	created, err := buildRedemptionsForCreate(1, model.Redemption{Name: "batch-cny", Quota: 1000, Count: len(keys)}, func() string {
		key := keys[next]
		next++
		return key
	})

	require.NoError(t, err)
	require.Len(t, created, 3)
	require.NotEmpty(t, created[0].BatchId)
	assert.Equal(t, created[0].BatchId, created[1].BatchId)
	assert.Equal(t, created[0].BatchId, created[2].BatchId)
}

func TestApplyWalletRedemptionUpdateUsesSubmittedCents(t *testing.T) {
	setupRedemptionCNYTestDB(t)

	existing := &model.Redemption{Name: "old", Type: model.RedemptionTypeWallet, Quota: 1, Count: 1}
	update := model.Redemption{Name: "new", Type: model.RedemptionTypeWallet, Quota: 4000, ExpiredTime: 0}
	err := applyRedemptionUpdate(existing, update)

	require.NoError(t, err)
	assert.Equal(t, 4000, existing.Quota)
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

func TestAddSubscriptionRedemptionStoresPlanAmountSnapshot(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	code := "snapshot-redemption-plan"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9574, Title: "Snapshot Redemption", PriceAmount: 80, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 2, BusinessCode: &code}).Error)

	created, err := buildRedemptionsForCreate(1, model.Redemption{Name: "sub-snapshot", Type: model.RedemptionTypeSubscription, PlanId: 9574, Count: 1}, func() string { return "sub-snapshot-key" })

	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, model.RedemptionTypeSubscription, created[0].Type)
	assert.Equal(t, 9574, created[0].PlanId)
	assert.Zero(t, created[0].Quota)
	assert.Equal(t, int64(8000), created[0].AmountCents)
	assert.Equal(t, "CNY", created[0].Currency)
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

func TestUpdateSubscriptionRedemptionRefreshesPlanAmountSnapshot(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	code := "update-snapshot-redemption-plan"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9575, Title: "Update Snapshot Redemption", PriceAmount: 120, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 2000, ConcurrencyLimit: 3, BusinessCode: &code}).Error)
	existing := &model.Redemption{Name: "old", Type: model.RedemptionTypeWallet, Quota: 1000, Count: 1}

	err := applyRedemptionUpdate(existing, model.Redemption{Name: "new", Type: model.RedemptionTypeSubscription, PlanId: 9575, ExpiredTime: 0})

	require.NoError(t, err)
	assert.Equal(t, model.RedemptionTypeSubscription, existing.Type)
	assert.Equal(t, 9575, existing.PlanId)
	assert.Zero(t, existing.Quota)
	assert.Equal(t, int64(12000), existing.AmountCents)
	assert.Equal(t, "CNY", existing.Currency)
}

func TestUpdateSubscriptionRedemptionPreservesSnapshotWhenPlanUnchanged(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	planID := 9580
	redemptionID := 9581
	code := "preserve-snapshot-redemption-plan"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Preserve Snapshot Redemption", PriceAmount: 40, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 2, BusinessCode: &code}).Error)
	require.NoError(t, model.DB.Create(&model.Redemption{Id: redemptionID, UserId: 1, Name: "original snapshot", Key: "preserve-snapshot-key", Type: model.RedemptionTypeSubscription, PlanId: planID, AmountCents: 4000, Currency: "CNY", Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp(), ExpiredTime: common.GetTimestamp() + 86400}).Error)
	model.InvalidateSubscriptionPlanCache(planID)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", planID).Updates(map[string]any{"price_amount": 60}).Error)
	model.InvalidateSubscriptionPlanCache(planID)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/redemption/", map[string]any{
		"id":           redemptionID,
		"name":         "renamed snapshot",
		"type":         model.RedemptionTypeSubscription,
		"plan_id":      planID,
		"expired_time": common.GetTimestamp() + 172800,
	}, 1)

	UpdateRedemption(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var saved model.Redemption
	require.NoError(t, model.DB.First(&saved, redemptionID).Error)
	assert.Equal(t, "renamed snapshot", saved.Name)
	assert.Equal(t, planID, saved.PlanId)
	assert.Equal(t, int64(4000), saved.AmountCents)
	assert.Equal(t, "CNY", saved.Currency)
}

func TestUpdateRedemptionStatusOnlyDoesNotBackfillMissingSnapshot(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	redemption := model.Redemption{
		Id: 9582, UserId: 1, Name: "legacy status only", Key: "legacy-status-only",
		Type: model.RedemptionTypeSubscription, PlanId: 999999,
		Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(&redemption).Error)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/redemption/?status_only=true", map[string]any{
		"id": redemption.Id, "status": common.RedemptionCodeStatusDisabled,
	}, 1)

	UpdateRedemption(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var saved model.Redemption
	require.NoError(t, model.DB.First(&saved, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusDisabled, saved.Status)
	assert.Empty(t, saved.FulfillmentSnapshot)
	assert.Zero(t, saved.FulfillmentSubscriptionId)
}

func TestUpdateUsedSubscriptionRedemptionRejectsSnapshotMutation(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	oldCode := "used-snapshot-old-plan"
	newCode := "used-snapshot-new-plan"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9576, Title: "Used Snapshot Old", PriceAmount: 80, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, ConcurrencyLimit: 2, BusinessCode: &oldCode}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9577, Title: "Used Snapshot New", PriceAmount: 120, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 2000, ConcurrencyLimit: 3, BusinessCode: &newCode}).Error)
	existing := &model.Redemption{Id: 9578, Name: "used", Type: model.RedemptionTypeSubscription, PlanId: 9576, AmountCents: 8000, Currency: "CNY", Status: common.RedemptionCodeStatusUsed}

	err := applyRedemptionUpdate(existing, model.Redemption{Name: "mutated", Type: model.RedemptionTypeSubscription, PlanId: 9577})

	require.Error(t, err)
	assert.Equal(t, model.RedemptionTypeSubscription, existing.Type)
	assert.Equal(t, 9576, existing.PlanId)
	assert.Equal(t, int64(8000), existing.AmountCents)
	assert.Equal(t, "CNY", existing.Currency)
}

func TestRedeemSubscriptionCodeCreatesUserSubscription(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	userID := 9604
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "redeem_plan", Quota: 1000, Status: common.UserStatusEnabled}).Error)
	code := "redeem-subscription"
	priceMicros := int64(40_000_000)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9605, Title: "Redeem Subscription", EntitlementType: model.SubscriptionEntitlementTimed, PriceAmount: 40, PriceAmountMicros: &priceMicros, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 7, MonthlyTokenLimit: 3000, ConcurrencyLimit: 4, QuotaResetPeriod: model.SubscriptionResetNever, BusinessCode: &code}).Error)
	redemption := &model.Redemption{UserId: 1, Name: "sub", Key: "sub-key", Type: model.RedemptionTypeSubscription, PlanId: 9605, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	result, err := model.Redeem("sub-key", userID, model.RedemptionModeTimed)

	require.NoError(t, err)
	assert.Equal(t, model.RedemptionTypeSubscription, result.Type)
	assert.Zero(t, result.Quota)
	require.NotNil(t, result.Plan)
	assert.Equal(t, 9605, result.Plan.Id)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 1000, user.Quota)
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

func TestRedeemWalletCodeUsesAccountBalanceCents(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	userID := 9606
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "redeem_wallet", Quota: 1000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Redemption{UserId: 1, Name: "wallet", Key: "wallet-key", Type: model.RedemptionTypeWallet, Quota: 4000, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup", strings.NewReader(`{"key":"wallet-key"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)

	TopUp(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"type":"wallet"`)
	assert.Contains(t, recorder.Body.String(), `"quota":4000`)
	assert.Contains(t, recorder.Body.String(), `"data":4000`)
	assert.Equal(t, 5000, getControllerUserQuotaForRedemptionTest(t, userID))
	assertRedemptionTopupLogContains(t, userID, "40.00")
}

func TestRedeemWalletCodeIgnoresCacheInvalidationFailure(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	setupControllerBrokenRedis(t)
	userID := 9611
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "redeem_wallet_broken_cache", Quota: 1000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Redemption{UserId: 1, Name: "wallet-broken-cache", Key: "wallet-broken-cache-key", Type: model.RedemptionTypeWallet, Quota: 4000, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}).Error)

	result, err := model.Redeem("wallet-broken-cache-key", userID, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.RedemptionTypeWallet, result.Type)
	assert.Equal(t, 4000, result.Quota)
	assert.Equal(t, 5000, getControllerUserQuotaForRedemptionTest(t, userID))
	var redeemed model.Redemption
	require.NoError(t, model.DB.Where("`key` = ?", "wallet-broken-cache-key").First(&redeemed).Error)
	assert.Equal(t, common.RedemptionCodeStatusUsed, redeemed.Status)
}

func getControllerUserQuotaForRedemptionTest(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").First(&user, userID).Error)
	return user.Quota
}

func assertRedemptionTopupLogContains(t *testing.T, userID int, expected string) {
	t.Helper()
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeTopup).Order("id DESC").First(&log).Error)
	assert.Contains(t, log.Content, expected)
	assert.NotContains(t, log.Content, "500000")
}

func TestRedeemLegacySubscriptionWithoutSnapshotReturnsDedicatedError(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	userID := 9609
	planID := 9610
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "legacy_snapshot_user", Status: common.UserStatusEnabled}).Error)
	priceMicros := int64(40_000_000)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id: planID, Title: "Legacy Snapshot", EntitlementType: model.SubscriptionEntitlementTimed,
		PriceAmount: 40, PriceAmountMicros: &priceMicros, Currency: "CNY", Enabled: true,
		DurationUnit: model.SubscriptionDurationDay, DurationValue: 30,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: model.SubscriptionResetMonthly,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Redemption{
		Id: 9611, UserId: 1, Name: "legacy snapshot", Key: "legacy-snapshot-key",
		Type: model.RedemptionTypeSubscription, PlanId: planID,
		Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp(),
	}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup", strings.NewReader(`{"key":"legacy-snapshot-key","redemption_mode":"timed"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	ctx.Set(string(constant.ContextKeyLanguage), i18n.LangZhCN)

	TopUp(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"redemption_snapshot_unavailable"`)
	assert.Contains(t, recorder.Body.String(), "该兑换码缺少历史授权快照，请联系管理员")
	assert.NotContains(t, recorder.Body.String(), "Credit 余额兑换资格")
	var saved model.Redemption
	require.NoError(t, model.DB.First(&saved, 9611).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, saved.Status)
	assert.Zero(t, saved.UsedUserId)
}

func TestRedeemSubscriptionCodeResponseIncludesPlanResult(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	userID := 9609
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "redeem_plan_response", Status: common.UserStatusEnabled}).Error)
	code := "redeem-response"
	priceMicros := int64(40_000_000)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9610, Title: "Response Plan", EntitlementType: model.SubscriptionEntitlementTimed, PriceAmount: 40, PriceAmountMicros: &priceMicros, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 7, MonthlyTokenLimit: 1000, QuotaResetPeriod: model.SubscriptionResetNever, BusinessCode: &code}).Error)
	redemption := &model.Redemption{UserId: 1, Name: "sub-response", Key: "sub-response-key", Type: model.RedemptionTypeSubscription, PlanId: 9610, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup", strings.NewReader(`{"key":"sub-response-key","redemption_mode":"timed"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)

	TopUp(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"type":"subscription"`)
	assert.Contains(t, recorder.Body.String(), `"title":"Response Plan"`)
}

func TestRedeemSubscriptionRedemptionInvokesInvitationRewardHandlerAfterCommit(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	inviterID := 9631
	inviteeID := 9632
	redeemerCode := "redeem-handler-plan"
	inviter := model.User{Id: inviterID, Username: "redeem-handler-inviter", Status: common.UserStatusEnabled, AffCode: "redeem-handler-inviter", InvitationRewardMode: model.InvitationRewardModeSubscription}
	invitee := model.User{Id: inviteeID, Username: "redeem-handler-invitee", Status: common.UserStatusEnabled, AffCode: "redeem-handler-invitee", InviterId: inviterID}
	require.NoError(t, model.DB.Create(&inviter).Error)
	require.NoError(t, model.DB.Create(&invitee).Error)
	priceMicros := int64(40_000_000)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9633, Title: "Redeem Handler Plan", PriceAmount: 40, PriceAmountMicros: &priceMicros, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 7, MonthlyTokenLimit: 3000, ConcurrencyLimit: 4, BusinessCode: &redeemerCode}).Error)
	redemption := model.Redemption{Id: 9634, UserId: 1, Name: "sub-handler", Key: "sub-handler-key", Type: model.RedemptionTypeSubscription, PlanId: 9633, AmountCents: 4000, Currency: "CNY", Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	calledRedemptionIDs := make([]int, 0, 1)
	SetInvitationRewardRedemptionHandlerForTest(t, func(redemptionId int) error {
		calledRedemptionIDs = append(calledRedemptionIDs, redemptionId)
		var redemption model.Redemption
		require.NoError(t, model.DB.First(&redemption, redemptionId).Error)
		assert.Equal(t, common.RedemptionCodeStatusUsed, redemption.Status)
		assert.Equal(t, inviteeID, redemption.UsedUserId)
		var event model.InvitationRewardEvent
		require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionRedemption, redemptionId).First(&event).Error)
		assert.Equal(t, inviterID, event.InviterId)
		assert.Equal(t, inviteeID, event.InviteeId)
		assert.Equal(t, redemptionId, event.SourceRedemptionId)
		assert.Equal(t, int64(4000), event.SourceAmountCents)
		assert.Equal(t, "CNY", event.SourceCurrency)
		return nil
	})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup", strings.NewReader(`{"key":"sub-handler-key","redemption_mode":"timed"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", inviteeID)

	TopUp(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, []int{9634}, calledRedemptionIDs)
	assert.Contains(t, recorder.Body.String(), `"type":"subscription"`)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionRedemption, 9634).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestDefaultInvitationRewardRedemptionHandlerUsesFormalService(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.InvitationCommissionAccount{}, &model.InvitationCommissionRecord{}, &model.InvitationCommissionLedger{}, &model.InvitationCommissionWithdrawal{}, &model.InvitationMonthlyEntitlement{}))
	setting := operation_setting.GetInvitationCommissionSetting()
	oldSetting := *setting
	*setting = operation_setting.InvitationCommissionSetting{Enabled: true, RateBps: 1000, MinimumTransferCents: 1, MinimumWithdrawCents: 1000}
	t.Cleanup(func() { *setting = oldSetting })

	inviterID := 9641
	inviteeID := 9642
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "default-redemption-commission-inviter", Status: common.UserStatusEnabled, AffCode: "default-redemption-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: inviteeID, Username: "default-redemption-commission-invitee", Status: common.UserStatusEnabled, AffCode: "default-redemption-invitee", InviterId: inviterID}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9643, Title: "Default Redemption Commission", PriceAmount: 100, Currency: "CNY", Enabled: true, PublicVisible: true, RewardEligible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 30, MonthlyTokenLimit: 1000, ConcurrencyLimit: 1}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 9644, UserId: inviteeID, PlanId: 9643, Status: "active", StartTime: now - 60, EndTime: now + 86400, GrantReason: "redemption", Source: "redemption"}).Error)
	redemption := model.Redemption{Id: 9645, UserId: 1, Name: "default-redemption", Key: "default-redemption-formal-service", Type: model.RedemptionTypeSubscription, PlanId: 9643, AmountCents: 10000, Currency: "CNY", Status: common.RedemptionCodeStatusUsed, UsedUserId: inviteeID, CreatedTime: now - 120, RedeemedTime: now - 30}
	require.NoError(t, model.DB.Create(&redemption).Error)
	event := model.InvitationRewardEvent{InviterId: inviterID, InviteeId: inviteeID, SourceType: model.InvitationRewardEventSourceSubscriptionRedemption, SourceId: redemption.Id, SourceRedemptionId: redemption.Id, SourceSubscriptionId: 9644, SourceAmountCents: 10000, SourceCurrency: "CNY", EventStartTime: now - 60, EventEndTime: now + 86400, Status: model.InvitationRewardEventStatusActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.DB.Create(&event).Error)

	require.NoError(t, handleInvitationRewardForSubscriptionRedemption(redemption.Id))

	var record model.InvitationCommissionRecord
	require.NoError(t, model.DB.Where("event_id = ?", event.Id).First(&record).Error)
	assert.Equal(t, model.InvitationCommissionStatusAvailable, record.Status)
	assert.Equal(t, int64(1000), record.CommissionCents)
	var entitlementCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", inviterID).Count(&entitlementCount).Error)
	assert.Equal(t, int64(0), entitlementCount)
}
func TestRedeemSubscriptionCodeAllowsRenewalWhenHistoricalPurchaseLimitReached(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	userID := 9607
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "redeem_limit", Status: common.UserStatusEnabled}).Error)
	code := "redeem-limit"
	priceMicros := int64(40_000_000)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 9608, Title: "Limit", EntitlementType: model.SubscriptionEntitlementTimed, PriceAmount: 40, PriceAmountMicros: &priceMicros, Currency: "CNY", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 7, MonthlyTokenLimit: 3000, QuotaResetPeriod: model.SubscriptionResetNever, MaxPurchasePerUser: 1, BusinessCode: &code}).Error)
	initialEnd := common.GetTimestamp() + 3600
	existing := &model.UserSubscription{UserId: userID, PlanId: 9608, Status: "active", StartTime: common.GetTimestamp() - 10, EndTime: initialEnd, TokenLimit: 3000, TokenUsed: 250, GrantReason: "order", Source: "order"}
	require.NoError(t, model.DB.Create(existing).Error)
	redemption := &model.Redemption{UserId: 1, Name: "limit", Key: "limit-key", Type: model.RedemptionTypeSubscription, PlanId: 9608, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	result, err := model.Redeem("limit-key", userID, model.RedemptionModeTimed)

	require.NoError(t, err)
	require.NotNil(t, result)
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, existing.Id).Error)
	assert.Equal(t, initialEnd+7*86400, sub.EndTime)
	assert.Equal(t, int64(250), sub.TokenUsed)
	assert.Equal(t, "order", sub.GrantReason)
	assert.Equal(t, "order", sub.Source)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, 9608).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
	var redeemed model.Redemption
	require.NoError(t, model.DB.Where("`key` = ?", "limit-key").First(&redeemed).Error)
	assert.Equal(t, common.RedemptionCodeStatusUsed, redeemed.Status)
	assert.Equal(t, userID, redeemed.UsedUserId)
}

func TestListRedemptionsFiltersByTypeStatusAndBatch(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	batchID := "batch-filter"
	now := common.GetTimestamp()
	records := []model.Redemption{
		{UserId: 1, Name: "batch-wallet", Key: "filter-wallet-a", Type: model.RedemptionTypeWallet, BatchId: batchID, Quota: 1000, Status: common.RedemptionCodeStatusEnabled, CreatedTime: now},
		{UserId: 1, Name: "batch-wallet", Key: "filter-wallet-b", Type: model.RedemptionTypeWallet, BatchId: batchID, Quota: 1000, Status: common.RedemptionCodeStatusUsed, CreatedTime: now},
		{UserId: 1, Name: "other-sub", Key: "filter-sub", Type: model.RedemptionTypeSubscription, BatchId: "other-batch", Status: common.RedemptionCodeStatusEnabled, CreatedTime: now},
	}
	require.NoError(t, model.DB.Create(&records).Error)

	items, total, err := model.ListRedemptions(model.RedemptionListOptions{
		Keyword:  "batch",
		Type:     model.RedemptionTypeWallet,
		Status:   common.RedemptionCodeStatusEnabled,
		BatchId:  batchID,
		StartIdx: 0,
		Num:      10,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, "filter-wallet-a", items[0].Key)
}

func TestRedemptionListOptionsAcceptsLegacySearchQuery(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/redemption/search?search=legacy-name", nil)

	options := redemptionListOptionsFromQuery(ctx, 0, 10)

	assert.Equal(t, "legacy-name", options.Keyword)
}

func TestGetRedemptionsByBatchReturnsAllRowsForBatch(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	batchID := "batch-full-fetch"
	now := common.GetTimestamp()
	records := []model.Redemption{
		{UserId: 1, Name: "full-batch", Key: "full-batch-a", Type: model.RedemptionTypeWallet, BatchId: batchID, Quota: 1000, Status: common.RedemptionCodeStatusEnabled, CreatedTime: now},
		{UserId: 1, Name: "full-batch", Key: "full-batch-b", Type: model.RedemptionTypeWallet, BatchId: batchID, Quota: 1000, Status: common.RedemptionCodeStatusDisabled, CreatedTime: now},
		{UserId: 1, Name: "other-batch", Key: "full-batch-other", Type: model.RedemptionTypeWallet, BatchId: "other-batch", Quota: 1000, Status: common.RedemptionCodeStatusEnabled, CreatedTime: now},
	}
	require.NoError(t, model.DB.Create(&records).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/redemption/batch/"+batchID, nil)
	ctx.Params = gin.Params{{Key: "batch_id", Value: batchID}}

	GetRedemptionsByBatch(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool               `json:"success"`
		Data    []model.Redemption `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data, 2)
	assert.Equal(t, "full-batch-b", response.Data[0].Key)
	assert.Equal(t, "full-batch-a", response.Data[1].Key)
}

func TestBatchAndAllDeleteRedemptions(t *testing.T) {
	setupRedemptionCNYTestDB(t)
	now := common.GetTimestamp()
	records := []model.Redemption{
		{Id: 9701, UserId: 1, Name: "enabled", Key: "delete-enabled", Type: model.RedemptionTypeWallet, Quota: 1000, Status: common.RedemptionCodeStatusEnabled, CreatedTime: now},
		{Id: 9702, UserId: 1, Name: "disabled", Key: "delete-disabled", Type: model.RedemptionTypeWallet, Quota: 1000, Status: common.RedemptionCodeStatusDisabled, CreatedTime: now},
		{Id: 9703, UserId: 1, Name: "used", Key: "delete-used", Type: model.RedemptionTypeWallet, Quota: 1000, Status: common.RedemptionCodeStatusUsed, CreatedTime: now},
	}
	require.NoError(t, model.DB.Create(&records).Error)

	rows, err := model.BatchDeleteRedemptions([]int{9701, 9702})

	require.NoError(t, err)
	assert.Equal(t, int64(2), rows)
	var remaining int64
	require.NoError(t, model.DB.Model(&model.Redemption{}).Count(&remaining).Error)
	assert.Equal(t, int64(1), remaining)

	rows, err = model.DeleteAllRedemptions()

	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)
	require.NoError(t, model.DB.Model(&model.Redemption{}).Count(&remaining).Error)
	assert.Zero(t, remaining)
}
