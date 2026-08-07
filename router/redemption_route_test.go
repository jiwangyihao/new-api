package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type redemptionRouteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Result  *struct {
		Type           string                          `json:"type"`
		RedemptionMode string                          `json:"redemption_mode"`
		CreditBalance  *model.CreditBalanceGrantResult `json:"credit_balance"`
		Replayed       bool                            `json:"replayed"`
	} `json:"result"`
}

func TestTopUpSubscriptionRequiresExplicitRedemptionMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Redemption{},
		&model.UserSubscription{},
		&model.InvitationRewardEvent{},
		&model.Log{},
	))
	model.ClearSubscriptionPlanCacheForTest()

	accessToken := "redemption-explicit-mode-token"
	user := model.User{Id: 9961, Username: "redemption-explicit-mode", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
	require.NoError(t, model.DB.Create(&user).Error)
	plan := model.SubscriptionPlan{Id: 9962, Title: "Explicit mode", Enabled: true, PublicVisible: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 7, MonthlyTokenLimit: 1000, QuotaResetPeriod: model.SubscriptionResetDaily}
	require.NoError(t, model.DB.Create(&plan).Error)
	redemption := model.Redemption{Id: 9963, Key: "explicit-mode-redemption", Type: model.RedemptionTypeSubscription, PlanId: plan.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-route-secret"))))
	SetApiRouter(engine)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(`{"key":"explicit-mode-redemption"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", "9961")
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload redemptionRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.NotEmpty(t, payload.Message)
	assert.Equal(t, "redemption_mode_required", payload.Code)

	var saved model.Redemption
	require.NoError(t, model.DB.First(&saved, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, saved.Status)
	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)
}

func TestTopUpSubscriptionCreditBalanceModeGrantsCreditAndWritesRedemptionLedger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Redemption{},
		&model.UserSubscription{},
		&model.CreditBalanceLedger{},
		&model.InvitationRewardEvent{},
		&model.InvitationCommissionRecord{},
		&model.InvitationMonthlyEntitlement{},
		&model.Log{},
	))
	model.ClearSubscriptionPlanCacheForTest()

	inviter := model.User{Id: 9970, Username: "redemption-credit-inviter", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "redemption-credit-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}
	require.NoError(t, model.DB.Create(&inviter).Error)
	accessToken := "redemption-credit-balance-token"
	user := model.User{Id: 9971, Username: "redemption-credit-balance", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken, InviterId: inviter.Id}
	require.NoError(t, model.DB.Create(&user).Error)
	creditPlan := model.SubscriptionPlan{
		Id:                             9972,
		Title:                          "Credit balance",
		Enabled:                        true,
		EntitlementType:                model.SubscriptionEntitlementCreditBalance,
		CreditBalanceConfigured:        true,
		CreditBalanceRedemptionEnabled: true,
	}
	require.NoError(t, model.DB.Create(&creditPlan).Error)
	option := model.SubscriptionPlan{
		Id:                       9973,
		Title:                    "Standard monthly",
		Enabled:                  true,
		PublicVisible:            true,
		EntitlementType:          model.SubscriptionEntitlementTimed,
		DurationUnit:             model.SubscriptionDurationMonth,
		DurationValue:            1,
		MonthlyTokenLimit:        4200,
		QuotaResetPeriod:         model.SubscriptionResetMonthly,
		UnlimitedPurchaseEnabled: true,
	}
	require.NoError(t, model.DB.Create(&option).Error)
	redemption := model.Redemption{Id: 9974, Key: "credit-balance-redemption", Type: model.RedemptionTypeSubscription, PlanId: option.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-credit-route-secret"))))
	SetApiRouter(engine)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(`{"key":"credit-balance-redemption","redemption_mode":"credit_balance"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", "9971")
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload redemptionRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	assert.Equal(t, model.RedemptionTypeSubscription, payload.Result.Type)
	assert.Equal(t, model.RedemptionModeCreditBalance, payload.Result.RedemptionMode)
	require.NotNil(t, payload.Result.CreditBalance)
	assert.Equal(t, int64(4200), payload.Result.CreditBalance.GrossCredit)
	assert.Equal(t, int64(4200), payload.Result.CreditBalance.AvailableCredit)
	assert.Equal(t, creditPlan.Id, payload.Result.CreditBalance.PlanId)
	assert.True(t, payload.Result.CreditBalance.Active)

	var saved model.Redemption
	require.NoError(t, model.DB.First(&saved, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusUsed, saved.Status)
	assert.Equal(t, model.RedemptionModeCreditBalance, saved.FulfillmentMode)
	assert.NotEmpty(t, saved.FulfillmentSnapshot)
	assert.Equal(t, payload.Result.CreditBalance.UserSubscriptionId, saved.FulfillmentSubscriptionId)

	var timedCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND entitlement_type = ?", user.Id, model.SubscriptionEntitlementTimed).Count(&timedCount).Error)
	assert.Zero(t, timedCount)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", user.Id, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(4200), balance.TokenLimit)
	assert.Zero(t, balance.TokenUsed)

	var ledger model.CreditBalanceLedger
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceRedemption, redemption.Id).First(&ledger).Error)
	assert.Equal(t, model.CreditBalanceLedgerTypeRedemption, ledger.Type)
	assert.Equal(t, int64(4200), ledger.GrossCredit)
	assert.Equal(t, int64(4200), ledger.AvailableCreditAfter)
	assert.Equal(t, balance.Id, ledger.UserSubscriptionId)
	assert.Contains(t, ledger.SourceSnapshot, `"purchase_mode":"credit_balance"`)
	assert.Contains(t, ledger.SourceSnapshot, `"monthly_token_limit":4200`)
	var savedUser model.User
	require.NoError(t, model.DB.First(&savedUser, user.Id).Error)
	assert.Equal(t, balance.Id, savedUser.GetSetting().ActiveSubscriptionId)
	var invitationEventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_redemption_id = ?", redemption.Id).Count(&invitationEventCount).Error)
	assert.Zero(t, invitationEventCount)
	var commissionCount int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("invitee_id = ?", user.Id).Count(&commissionCount).Error)
	assert.Zero(t, commissionCount)
	var invitationEntitlementCount int64
	require.NoError(t, model.DB.Model(&model.InvitationMonthlyEntitlement{}).Where("inviter_id = ?", inviter.Id).Count(&invitationEntitlementCount).Error)
	assert.Zero(t, invitationEntitlementCount)
}

func TestTopUpUsedSubscriptionStillRejectsMissingOrInvalidRedemptionMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Redemption{},
		&model.UserSubscription{},
		&model.InvitationRewardEvent{},
		&model.Log{},
	))
	model.ClearSubscriptionPlanCacheForTest()

	accessToken := "redemption-replay-mode-token"
	user := model.User{Id: 9981, Username: "redemption-replay-mode", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
	require.NoError(t, model.DB.Create(&user).Error)
	priceMicros := int64(10_000_000)
	plan := model.SubscriptionPlan{
		Id: 9982, Title: "Replay mode", Enabled: true, PublicVisible: true,
		EntitlementType: model.SubscriptionEntitlementTimed,
		PriceAmount:     10, PriceAmountMicros: &priceMicros, Currency: "CNY",
		DurationUnit: model.SubscriptionDurationDay, DurationValue: 7,
		MonthlyTokenLimit: 1000, QuotaResetPeriod: model.SubscriptionResetDaily,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	redemption := model.Redemption{Id: 9983, Key: "replay-mode-redemption", Type: model.RedemptionTypeSubscription, PlanId: plan.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-replay-mode-secret"))))
	SetApiRouter(engine)
	post := func(body string) redemptionRouteResponse {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("New-Api-User", "9981")
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
		var payload redemptionRouteResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		return payload
	}

	first := post(`{"key":"replay-mode-redemption","redemption_mode":"timed"}`)
	require.True(t, first.Success, first.Message)

	for name, body := range map[string]string{
		"missing": `{"key":"replay-mode-redemption"}`,
		"invalid": `{"key":"replay-mode-redemption","redemption_mode":"forever"}`,
	} {
		t.Run(name, func(t *testing.T) {
			payload := post(body)
			assert.False(t, payload.Success)
			assert.NotEmpty(t, payload.Message)
		})
	}

	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
}

func TestTopUpConcurrentSameCodeReturnsOneFulfillmentAndOneReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Redemption{},
		&model.UserSubscription{},
		&model.CreditBalanceLedger{},
		&model.InvitationRewardEvent{},
		&model.Log{},
	))
	model.ClearSubscriptionPlanCacheForTest()

	accessToken := "redemption-concurrent-token"
	user := model.User{Id: 9991, Username: "redemption-concurrent", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
	require.NoError(t, model.DB.Create(&user).Error)
	creditPlan := model.SubscriptionPlan{Id: 9992, Title: "Concurrent credit", Enabled: true, EntitlementType: model.SubscriptionEntitlementCreditBalance, CreditBalanceConfigured: true, CreditBalanceRedemptionEnabled: true}
	require.NoError(t, model.DB.Create(&creditPlan).Error)
	option := model.SubscriptionPlan{Id: 9993, Title: "Concurrent option", Enabled: true, EntitlementType: model.SubscriptionEntitlementTimed, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 2500, QuotaResetPeriod: model.SubscriptionResetMonthly, UnlimitedPurchaseEnabled: true}
	require.NoError(t, model.DB.Create(&option).Error)
	redemption := model.Redemption{Id: 9994, Key: "concurrent-credit-redemption", Type: model.RedemptionTypeSubscription, PlanId: option.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-concurrent-secret"))))
	SetApiRouter(engine)
	start := make(chan struct{})
	responses := make(chan redemptionRouteResponse, 2)
	for range 2 {
		go func() {
			<-start
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(`{"key":"concurrent-credit-redemption","redemption_mode":"credit_balance"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+accessToken)
			request.Header.Set("New-Api-User", "9991")
			engine.ServeHTTP(recorder, request)
			var payload redemptionRouteResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			responses <- payload
		}()
	}
	close(start)
	first := <-responses
	second := <-responses
	require.True(t, first.Success, first.Message)
	require.True(t, second.Success, second.Message)
	assert.NotEqual(t, first.Result.Replayed, second.Result.Replayed)
	for _, payload := range []redemptionRouteResponse{first, second} {
		assert.Equal(t, model.RedemptionModeCreditBalance, payload.Result.RedemptionMode)
		require.NotNil(t, payload.Result.CreditBalance)
		assert.Equal(t, int64(2500), payload.Result.CreditBalance.GrossCredit)
	}

	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceRedemption, redemption.Id).Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", user.Id, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(2500), balance.TokenLimit)
}

func TestTopUpWalletKeepsLegacyModeAndReplayContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Redemption{}, &model.Log{}))

	accessToken := "redemption-wallet-replay-token"
	user := model.User{Id: 10001, Username: "redemption-wallet-replay", Quota: 1000, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
	require.NoError(t, model.DB.Create(&user).Error)
	redemption := model.Redemption{Id: 10002, Key: "wallet-replay-redemption", Type: model.RedemptionTypeWallet, Quota: 4000, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, model.DB.Create(&redemption).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-wallet-replay-secret"))))
	SetApiRouter(engine)
	post := func() redemptionRouteResponse {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(`{"key":"wallet-replay-redemption"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("New-Api-User", "10001")
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
		var payload redemptionRouteResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		return payload
	}

	first := post()
	require.True(t, first.Success, first.Message)
	require.NotNil(t, first.Result)
	assert.Equal(t, model.RedemptionTypeWallet, first.Result.Type)
	assert.Empty(t, first.Result.RedemptionMode)
	second := post()
	assert.False(t, second.Success)
	assert.Equal(t, i18n.MsgRedeemFailed, second.Message)
	assert.Empty(t, second.Code)
	assert.Nil(t, second.Result)

	var savedUser model.User
	require.NoError(t, model.DB.Select("quota").First(&savedUser, user.Id).Error)
	assert.Equal(t, 5000, savedUser.Quota)
}

func TestTopUpCreditBalanceUsesLatestPlanCreditAndReplaysFrozenResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Redemption{},
		&model.UserSubscription{},
		&model.CreditBalanceLedger{},
		&model.InvitationRewardEvent{},
		&model.Log{},
	))
	model.ClearSubscriptionPlanCacheForTest()

	accessToken := "redemption-latest-credit-token"
	user := model.User{Id: 10011, Username: "redemption-latest-credit", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
	require.NoError(t, model.DB.Create(&user).Error)
	creditPlan := model.SubscriptionPlan{Id: 10012, Title: "Latest credit balance", Enabled: true, EntitlementType: model.SubscriptionEntitlementCreditBalance, CreditBalanceConfigured: true, CreditBalanceRedemptionEnabled: true}
	require.NoError(t, model.DB.Create(&creditPlan).Error)
	option := model.SubscriptionPlan{Id: 10013, Title: "Latest monthly option", Enabled: true, EntitlementType: model.SubscriptionEntitlementTimed, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, QuotaResetPeriod: model.SubscriptionResetMonthly, UnlimitedPurchaseEnabled: true}
	require.NoError(t, model.DB.Create(&option).Error)
	redemption := model.Redemption{Id: 10014, Key: "latest-credit-redemption", Type: model.RedemptionTypeSubscription, PlanId: option.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	_, err := model.GetSubscriptionPlanById(option.Id)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", option.Id).Update("monthly_token_limit", 3500).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-latest-credit-secret"))))
	SetApiRouter(engine)
	post := func(mode string) redemptionRouteResponse {
		recorder := httptest.NewRecorder()
		body := []byte(`{"key":"latest-credit-redemption","redemption_mode":"` + mode + `"}`)
		request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("New-Api-User", "10011")
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
		var payload redemptionRouteResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		return payload
	}

	first := post(model.RedemptionModeCreditBalance)
	require.True(t, first.Success, first.Message)
	require.NotNil(t, first.Result.CreditBalance)
	assert.Equal(t, int64(1000), first.Result.CreditBalance.GrossCredit)
	assert.False(t, first.Result.Replayed)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", option.Id).Update("monthly_token_limit", 9000).Error)

	conflict := post(model.RedemptionModeTimed)
	require.False(t, conflict.Success)
	assert.Equal(t, "redemption_already_used", conflict.Code)
	require.NotNil(t, conflict.Result)
	assert.True(t, conflict.Result.Replayed)
	assert.Equal(t, model.RedemptionModeCreditBalance, conflict.Result.RedemptionMode)

	replay := post(model.RedemptionModeCreditBalance)
	require.True(t, replay.Success, replay.Message)
	require.NotNil(t, replay.Result)
	assert.True(t, replay.Result.Replayed)
	assert.Equal(t, model.RedemptionModeCreditBalance, replay.Result.RedemptionMode)
	require.NotNil(t, replay.Result.CreditBalance)
	assert.Equal(t, int64(1000), replay.Result.CreditBalance.GrossCredit)
	assert.Equal(t, first.Result.CreditBalance.LedgerId, replay.Result.CreditBalance.LedgerId)

	var saved model.Redemption
	require.NoError(t, model.DB.First(&saved, redemption.Id).Error)
	assert.Equal(t, model.RedemptionModeCreditBalance, saved.FulfillmentMode)
	assert.Contains(t, saved.FulfillmentSnapshot, `"monthly_token_limit":1000`)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", user.Id, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(1000), balance.TokenLimit)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceRedemption, redemption.Id).Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
}

func TestTopUpCreditBalanceRejectsClosedGlobalRedemptionEntryWithoutConsumingCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Redemption{},
		&model.UserSubscription{},
		&model.CreditBalanceLedger{},
		&model.InvitationRewardEvent{},
		&model.Log{},
	))
	model.ClearSubscriptionPlanCacheForTest()

	accessToken := "redemption-closed-entry-token"
	user := model.User{Id: 10021, Username: "redemption-closed-entry", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
	require.NoError(t, model.DB.Create(&user).Error)
	creditPlan := model.SubscriptionPlan{Id: 10022, Title: "Closed credit balance", Enabled: true, EntitlementType: model.SubscriptionEntitlementCreditBalance, CreditBalanceConfigured: true, CreditBalanceRedemptionEnabled: false}
	require.NoError(t, model.DB.Create(&creditPlan).Error)
	option := model.SubscriptionPlan{Id: 10023, Title: "Eligible monthly option", Enabled: true, EntitlementType: model.SubscriptionEntitlementTimed, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 2000, QuotaResetPeriod: model.SubscriptionResetMonthly, UnlimitedPurchaseEnabled: true}
	require.NoError(t, model.DB.Create(&option).Error)
	redemption := model.Redemption{Id: 10024, Key: "closed-entry-redemption", Type: model.RedemptionTypeSubscription, PlanId: option.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-closed-entry-secret"))))
	SetApiRouter(engine)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(`{"key":"closed-entry-redemption","redemption_mode":"credit_balance"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", "10021")
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload redemptionRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.Equal(t, "credit_balance_redemption_unavailable", payload.Code)
	var saved model.Redemption
	require.NoError(t, model.DB.First(&saved, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, saved.Status)
	assert.Empty(t, saved.FulfillmentMode)
	assert.Contains(t, saved.FulfillmentSnapshot, `"monthly_token_limit":2000`)
	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceRedemption, redemption.Id).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
}

func TestTopUpCreditBalanceRejectsEveryIneligiblePlanShape(t *testing.T) {
	tests := []struct {
		name                  string
		mutateCredit          func(*model.SubscriptionPlan)
		mutateOption          func(*model.SubscriptionPlan)
		persistCreditDisabled bool
		persistOptionDisabled bool
		rawOptionType         string
		wantCode              string
	}{
		{name: "balance plan not configured", mutateCredit: func(plan *model.SubscriptionPlan) { plan.CreditBalanceConfigured = false }, wantCode: "credit_balance_redemption_unavailable"},
		{name: "balance redemption disabled", mutateCredit: func(plan *model.SubscriptionPlan) { plan.CreditBalanceRedemptionEnabled = false }, wantCode: "credit_balance_redemption_unavailable"},
		{name: "balance plan paused", persistCreditDisabled: true, wantCode: "credit_balance_redemption_unavailable"},
		{name: "target plan paused", persistOptionDisabled: true, wantCode: "redemption_plan_ineligible"},
		{name: "target is not timed", rawOptionType: "unsupported", wantCode: "redemption_plan_ineligible"},
		{name: "non monthly duration", mutateOption: func(plan *model.SubscriptionPlan) { plan.DurationUnit = model.SubscriptionDurationYear }, wantCode: "redemption_plan_ineligible"},
		{name: "multiple months", mutateOption: func(plan *model.SubscriptionPlan) { plan.DurationValue = 2 }, wantCode: "redemption_plan_ineligible"},
		{name: "non monthly reset", mutateOption: func(plan *model.SubscriptionPlan) { plan.QuotaResetPeriod = model.SubscriptionResetDaily }, wantCode: "redemption_plan_ineligible"},
		{name: "zero monthly Credit", mutateOption: func(plan *model.SubscriptionPlan) { plan.MonthlyTokenLimit = 0 }, wantCode: "redemption_plan_ineligible"},
		{name: "trial plan", mutateOption: func(plan *model.SubscriptionPlan) { plan.IsTrial = true }, wantCode: "redemption_plan_ineligible"},
		{name: "invite trial plan", mutateOption: func(plan *model.SubscriptionPlan) { plan.InviteTrial = true }, wantCode: "redemption_plan_ineligible"},
		{name: "unlimited purchase ineligible", mutateOption: func(plan *model.SubscriptionPlan) { plan.UnlimitedPurchaseEnabled = false }, wantCode: "redemption_plan_ineligible"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			db := setupSubscriptionPublicPlansRouteTestDB(t)
			require.NoError(t, db.AutoMigrate(
				&model.User{},
				&model.Redemption{},
				&model.UserSubscription{},
				&model.CreditBalanceLedger{},
				&model.InvitationRewardEvent{},
				&model.Log{},
			))
			model.ClearSubscriptionPlanCacheForTest()

			accessToken := "redemption-ineligible-token"
			user := model.User{Id: 10031, Username: "redemption-ineligible", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
			require.NoError(t, model.DB.Create(&user).Error)
			creditPlan := model.SubscriptionPlan{Id: 10032, Title: "Credit balance", Enabled: true, EntitlementType: model.SubscriptionEntitlementCreditBalance, CreditBalanceConfigured: true, CreditBalanceRedemptionEnabled: true}
			option := model.SubscriptionPlan{Id: 10033, Title: "Monthly option", Enabled: true, EntitlementType: model.SubscriptionEntitlementTimed, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 2000, QuotaResetPeriod: model.SubscriptionResetMonthly, UnlimitedPurchaseEnabled: true}
			if test.mutateCredit != nil {
				test.mutateCredit(&creditPlan)
			}
			if test.mutateOption != nil {
				test.mutateOption(&option)
			}
			require.NoError(t, model.DB.Create(&creditPlan).Error)
			require.NoError(t, model.DB.Create(&option).Error)
			if test.persistCreditDisabled {
				require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", creditPlan.Id).Update("enabled", false).Error)
			}
			if test.persistOptionDisabled {
				require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", option.Id).Update("enabled", false).Error)
			}
			if test.rawOptionType != "" {
				require.NoError(t, model.DB.Exec("UPDATE subscription_plans SET entitlement_type = ?, singleton_key = NULL WHERE id = ?", test.rawOptionType, option.Id).Error)
			}
			redemption := model.Redemption{Id: 10034, Key: "ineligible-credit-redemption", Type: model.RedemptionTypeSubscription, PlanId: option.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
			require.NoError(t, redemption.Insert())

			engine := gin.New()
			engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-ineligible-secret"))))
			SetApiRouter(engine)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(`{"key":"ineligible-credit-redemption","redemption_mode":"credit_balance"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+accessToken)
			request.Header.Set("New-Api-User", "10031")
			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload redemptionRouteResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.False(t, payload.Success)
			assert.Equal(t, test.wantCode, payload.Code)
			var saved model.Redemption
			require.NoError(t, model.DB.First(&saved, redemption.Id).Error)
			assert.Equal(t, common.RedemptionCodeStatusEnabled, saved.Status)
			var subscriptionCount int64
			require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptionCount).Error)
			assert.Zero(t, subscriptionCount)
			var ledgerCount int64
			require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Count(&ledgerCount).Error)
			assert.Zero(t, ledgerCount)
		})
	}
}

func TestTopUpCreditBalanceKeepsExistingActivityAndBillingStrategy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Redemption{},
		&model.UserSubscription{},
		&model.CreditBalanceLedger{},
		&model.InvitationRewardEvent{},
		&model.Log{},
	))
	model.ClearSubscriptionPlanCacheForTest()

	accessToken := "redemption-existing-active-token"
	user := model.User{Id: 10041, Username: "redemption-existing-active", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = 10044
	setting.SubscriptionBillingStrategy = model.SubscriptionBillingStrategyTimedFirst
	settingPayload, err := common.Marshal(setting)
	require.NoError(t, err)
	user.Setting = string(settingPayload)
	require.NoError(t, model.DB.Create(&user).Error)
	creditPlan := model.SubscriptionPlan{Id: 10042, Title: "Existing activity credit", Enabled: true, EntitlementType: model.SubscriptionEntitlementCreditBalance, CreditBalanceConfigured: true, CreditBalanceRedemptionEnabled: true}
	option := model.SubscriptionPlan{Id: 10043, Title: "Existing activity option", Enabled: true, EntitlementType: model.SubscriptionEntitlementTimed, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1800, QuotaResetPeriod: model.SubscriptionResetMonthly, UnlimitedPurchaseEnabled: true}
	require.NoError(t, model.DB.Create(&creditPlan).Error)
	require.NoError(t, model.DB.Create(&option).Error)
	existing := model.UserSubscription{Id: 10044, UserId: user.Id, PlanId: option.Id, EntitlementType: model.SubscriptionEntitlementTimed, Status: "active", StartTime: common.GetTimestamp() - 60, EndTime: common.GetTimestamp() + 86400, TokenLimit: 100, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}
	require.NoError(t, model.DB.Create(&existing).Error)
	redemption := model.Redemption{Id: 10045, Key: "existing-active-redemption", Type: model.RedemptionTypeSubscription, PlanId: option.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-existing-active-secret"))))
	SetApiRouter(engine)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(`{"key":"existing-active-redemption","redemption_mode":"credit_balance"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", "10041")
	engine.ServeHTTP(recorder, request)

	var payload redemptionRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	require.NotNil(t, payload.Result.CreditBalance)
	assert.False(t, payload.Result.CreditBalance.Active)
	var savedUser model.User
	require.NoError(t, model.DB.First(&savedUser, user.Id).Error)
	savedSetting := savedUser.GetSetting()
	assert.Equal(t, existing.Id, savedSetting.ActiveSubscriptionId)
	assert.Equal(t, model.SubscriptionBillingStrategyTimedFirst, savedSetting.SubscriptionBillingStrategy)
}

func TestTopUpCreditBalanceOffsetsSettlementDebtAndReturnsReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Redemption{},
		&model.UserSubscription{},
		&model.CreditBalanceLedger{},
		&model.InvitationRewardEvent{},
		&model.Log{},
	))
	model.ClearSubscriptionPlanCacheForTest()

	accessToken := "redemption-debt-offset-token"
	user := model.User{Id: 10051, Username: "redemption-debt-offset", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
	require.NoError(t, model.DB.Create(&user).Error)
	creditPlan := model.SubscriptionPlan{Id: 10052, Title: "Debt credit balance", Enabled: true, EntitlementType: model.SubscriptionEntitlementCreditBalance, CreditBalanceConfigured: true, CreditBalanceRedemptionEnabled: true}
	option := model.SubscriptionPlan{Id: 10053, Title: "Debt monthly option", Enabled: true, EntitlementType: model.SubscriptionEntitlementTimed, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1000, QuotaResetPeriod: model.SubscriptionResetMonthly, UnlimitedPurchaseEnabled: true}
	require.NoError(t, model.DB.Create(&creditPlan).Error)
	require.NoError(t, model.DB.Create(&option).Error)
	balance := model.UserSubscription{Id: 10054, UserId: user.Id, PlanId: creditPlan.Id, EntitlementType: model.SubscriptionEntitlementCreditBalance, Status: "active", TokenLimit: 200, TokenUsed: 500, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}
	require.NoError(t, model.DB.Create(&balance).Error)
	redemption := model.Redemption{Id: 10055, Key: "debt-offset-redemption", Type: model.RedemptionTypeSubscription, PlanId: option.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-debt-offset-secret"))))
	SetApiRouter(engine)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(`{"key":"debt-offset-redemption","redemption_mode":"credit_balance"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", "10051")
	engine.ServeHTTP(recorder, request)

	var payload redemptionRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	require.NotNil(t, payload.Result.CreditBalance)
	assert.Equal(t, int64(1000), payload.Result.CreditBalance.GrossCredit)
	assert.Equal(t, int64(300), payload.Result.CreditBalance.DebtOffset)
	assert.Equal(t, int64(700), payload.Result.CreditBalance.AvailableCredit)
	assert.Zero(t, payload.Result.CreditBalance.SettlementDebt)
	assert.Equal(t, int64(-300), payload.Result.CreditBalance.BalanceBefore)
	assert.Equal(t, int64(700), payload.Result.CreditBalance.BalanceAfter)

	var savedBalance model.UserSubscription
	require.NoError(t, model.DB.First(&savedBalance, balance.Id).Error)
	assert.Equal(t, int64(1200), savedBalance.TokenLimit)
	assert.Equal(t, int64(500), savedBalance.TokenUsed)
	var ledger model.CreditBalanceLedger
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceRedemption, redemption.Id).First(&ledger).Error)
	assert.Equal(t, int64(300), ledger.DebtOffset)
	assert.Equal(t, int64(-300), ledger.BalanceBefore)
	assert.Equal(t, int64(700), ledger.BalanceAfter)
	assert.Equal(t, int64(700), ledger.AvailableCreditAfter)
	assert.Zero(t, ledger.SettlementDebtAfter)
}

func TestTopUpCreditBalanceRollsBackWhenLedgerWriteFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Redemption{},
		&model.UserSubscription{},
		&model.CreditBalanceLedger{},
		&model.InvitationRewardEvent{},
		&model.Log{},
	))
	model.ClearSubscriptionPlanCacheForTest()

	accessToken := "redemption-ledger-failure-token"
	user := model.User{Id: 10061, Username: "redemption-ledger-failure", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &accessToken}
	require.NoError(t, model.DB.Create(&user).Error)
	creditPlan := model.SubscriptionPlan{Id: 10062, Title: "Rollback credit", Enabled: true, EntitlementType: model.SubscriptionEntitlementCreditBalance, CreditBalanceConfigured: true, CreditBalanceRedemptionEnabled: true}
	option := model.SubscriptionPlan{Id: 10063, Title: "Rollback option", Enabled: true, EntitlementType: model.SubscriptionEntitlementTimed, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, MonthlyTokenLimit: 1600, QuotaResetPeriod: model.SubscriptionResetMonthly, UnlimitedPurchaseEnabled: true}
	require.NoError(t, model.DB.Create(&creditPlan).Error)
	require.NoError(t, model.DB.Create(&option).Error)
	redemption := model.Redemption{Id: 10064, Key: "ledger-failure-redemption", Type: model.RedemptionTypeSubscription, PlanId: option.Id, Status: common.RedemptionCodeStatusEnabled, CreatedTime: common.GetTimestamp()}
	require.NoError(t, redemption.Insert())

	triggerName := "fail_redemption_ledger_10064"
	require.NoError(t, model.DB.Exec("CREATE TRIGGER "+triggerName+" BEFORE INSERT ON credit_balance_ledgers WHEN NEW.source_type = 'redemption' AND NEW.source_id = 10064 BEGIN SELECT RAISE(ABORT, 'injected redemption ledger failure'); END").Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS " + triggerName).Error })

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("redemption-ledger-failure-secret"))))
	SetApiRouter(engine)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewBufferString(`{"key":"ledger-failure-redemption","redemption_mode":"credit_balance"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", "10061")
	engine.ServeHTTP(recorder, request)

	var payload redemptionRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	var saved model.Redemption
	require.NoError(t, model.DB.First(&saved, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, saved.Status)
	assert.Empty(t, saved.FulfillmentMode)
	assert.Contains(t, saved.FulfillmentSnapshot, `"monthly_token_limit":1600`)
	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
	var savedUser model.User
	require.NoError(t, model.DB.First(&savedUser, user.Id).Error)
	assert.Zero(t, savedUser.GetSetting().ActiveSubscriptionId)
}
