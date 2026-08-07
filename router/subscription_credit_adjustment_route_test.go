package router

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCreditAdjustmentRoute(t *testing.T) (*gin.Engine, string, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupSubscriptionPublicPlansRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.CreditBalanceLedger{}, &model.CreditBalanceAdjustment{}, &model.InvitationRewardEvent{}, &model.Log{}))
	accessToken := "credit-adjustment-admin-token"
	const adminID = 9961
	const userID = 9962
	require.NoError(t, db.Create(&model.User{Id: adminID, Username: "credit-adjustment-admin", Status: common.UserStatusEnabled, Role: common.RoleAdminUser, AccessToken: &accessToken, AffCode: "credit-adjustment-admin"}).Error)
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "credit-adjustment-user", Status: common.UserStatusEnabled, AffCode: "credit-adjustment-user"}).Error)
	code := "credit-adjustment-balance"
	require.NoError(t, db.Create(&model.SubscriptionPlan{Id: 9963, Title: "Credit 余额套餐", EntitlementType: model.SubscriptionEntitlementCreditBalance, Enabled: true, CreditBalanceConfigured: true, BusinessCode: &code}).Error)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("credit-adjustment-route-secret"))))
	SetApiRouter(engine)
	return engine, accessToken, userID
}

func performCreditAdjustmentRouteRequest(engine *gin.Engine, token string, userID int, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/subscription/admin/users/%d/credit-balance/adjustments", userID), bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("New-Api-User", "9961")
	engine.ServeHTTP(recorder, request)
	return recorder
}

func performSubscriptionOrderRecoveryRouteRequest(engine *gin.Engine, token string, userID int, tradeNo, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/subscription/admin/users/%d/orders/%s/recovery", userID, tradeNo), bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("New-Api-User", "9961")
	engine.ServeHTTP(recorder, request)
	return recorder
}

func performSubscriptionOrderRecoveryPreviewRouteRequest(engine *gin.Engine, token string, userID int, tradeNo string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/subscription/admin/users/%d/orders/%s/recovery-preview", userID, tradeNo), nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("New-Api-User", "9961")
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminCreditAdjustmentRouteCreatesDebtThenIncreaseOffsetsIt(t *testing.T) {
	engine, token, userID := setupCreditAdjustmentRoute(t)
	decrease := performCreditAdjustmentRouteRequest(engine, token, userID, `{"operation":"decrease","amount":300,"idempotency_key":"admin-decrease-empty","reason":"manual correction"}`)
	require.Equal(t, http.StatusOK, decrease.Code, decrease.Body.String())
	assert.Contains(t, decrease.Body.String(), `"settlement_debt":300`)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(0), balance.TokenLimit)
	assert.Equal(t, int64(300), balance.TokenUsed)

	increase := performCreditAdjustmentRouteRequest(engine, token, userID, `{"operation":"increase","amount":500,"idempotency_key":"admin-increase-offset","reason":"approved correction"}`)
	require.Equal(t, http.StatusOK, increase.Code, increase.Body.String())
	assert.Contains(t, increase.Body.String(), `"debt_offset":300`)
	assert.Contains(t, increase.Body.String(), `"available_credit":200`)
	require.NoError(t, model.DB.First(&balance, balance.Id).Error)
	assert.Equal(t, int64(500), balance.TokenLimit)
	assert.Equal(t, int64(300), balance.TokenUsed)
}

func TestAdminCreditAdjustmentRouteValidatesReasonBoundsAndIdempotency(t *testing.T) {
	engine, token, userID := setupCreditAdjustmentRoute(t)
	missingReason := performCreditAdjustmentRouteRequest(engine, token, userID, `{"operation":"increase","amount":1,"idempotency_key":"missing-reason","reason":""}`)
	assert.Contains(t, missingReason.Body.String(), `"success":false`)
	zero := performCreditAdjustmentRouteRequest(engine, token, userID, `{"operation":"increase","amount":0,"idempotency_key":"zero","reason":"reason"}`)
	assert.Contains(t, zero.Body.String(), `"success":false`)
	negative := performCreditAdjustmentRouteRequest(engine, token, userID, `{"operation":"decrease","amount":-1,"idempotency_key":"negative","reason":"reason"}`)
	assert.Contains(t, negative.Body.String(), `"success":false`)
	maxAllowed := performCreditAdjustmentRouteRequest(engine, token, userID, fmt.Sprintf(`{"operation":"increase","amount":%d,"idempotency_key":"max-allowed","reason":"boundary"}`, model.MaxCreditBalanceAdjustmentAmount))
	assert.Contains(t, maxAllowed.Body.String(), `"success":true`)
	overflow := performCreditAdjustmentRouteRequest(engine, token, userID, fmt.Sprintf(`{"operation":"increase","amount":%d,"idempotency_key":"too-large","reason":"overflow"}`, model.MaxCreditBalanceAdjustmentAmount+1))
	assert.Contains(t, overflow.Body.String(), `"success":false`)
	maxInt := performCreditAdjustmentRouteRequest(engine, token, userID, fmt.Sprintf(`{"operation":"increase","amount":%d,"idempotency_key":"max-int","reason":"overflow"}`, int64(math.MaxInt64)))
	assert.Contains(t, maxInt.Body.String(), `"success":false`)

	first := performCreditAdjustmentRouteRequest(engine, token, userID, `{"operation":"decrease","amount":25,"idempotency_key":"replay-key","reason":"same"}`)
	require.Contains(t, first.Body.String(), `"success":true`)
	replay := performCreditAdjustmentRouteRequest(engine, token, userID, `{"operation":"decrease","amount":25,"idempotency_key":"replay-key","reason":"same"}`)
	require.Contains(t, replay.Body.String(), `"replayed":true`)
	conflict := performCreditAdjustmentRouteRequest(engine, token, userID, `{"operation":"decrease","amount":26,"idempotency_key":"replay-key","reason":"same"}`)
	assert.Contains(t, conflict.Body.String(), `"success":false`)
	var adjustmentCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceAdjustment{}).Where("idempotency_key = ?", "replay-key").Count(&adjustmentCount).Error)
	assert.Equal(t, int64(1), adjustmentCount)
}

func TestCreditBalanceAdjustmentIdempotencyKeyRejectsDifferentOperator(t *testing.T) {
	_, _, userID := setupCreditAdjustmentRoute(t)
	request := model.CreditBalanceAdjustmentRequest{
		UserId: userID, Operation: model.CreditBalanceAdjustmentIncrease, Amount: 25,
		IdempotencyKey: "operator-bound-adjustment", OperatorUserId: 9961, Reason: "verified correction",
	}
	first, err := model.AdjustCreditBalance(request)
	require.NoError(t, err)
	require.False(t, first.Replayed)

	request.OperatorUserId = 9971
	second, err := model.AdjustCreditBalance(request)
	require.Nil(t, second)
	require.ErrorContains(t, err, "idempotency key parameter mismatch")

	var adjustmentCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceAdjustment{}).
		Where("idempotency_key = ?", request.IdempotencyKey).
		Count(&adjustmentCount).Error)
	assert.Equal(t, int64(1), adjustmentCount)
	var ledgerCount int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).
		Where("source_type = ?", model.CreditBalanceLedgerSourceAdminAdjustment).
		Count(&ledgerCount).Error)
	assert.Equal(t, int64(1), ledgerCount)
	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(25), balance.TokenLimit)
	assert.Zero(t, balance.TokenUsed)
}

func TestCreditAdjustmentRouteRequiresAdminAuthentication(t *testing.T) {
	engine, _, userID := setupCreditAdjustmentRoute(t)
	unauthorized := performCreditAdjustmentRouteRequest(engine, "invalid", userID, `{"operation":"increase","amount":1,"idempotency_key":"unauthorized","reason":"reason"}`)
	assert.Contains(t, unauthorized.Body.String(), `"success":false`)
	var adjustments int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceAdjustment{}).Count(&adjustments).Error)
	assert.Zero(t, adjustments)
}

func TestAdminSubscriptionOrderRecoveryRouteCompensatesEpayOnce(t *testing.T) {
	engine, token, userID := setupCreditAdjustmentRoute(t)
	optionCode := "epay-admin-recovery-option"
	optionPlan := model.SubscriptionPlan{
		Id: 9964, Title: "Epay recovery option", EntitlementType: model.SubscriptionEntitlementTimed,
		MonthlyTokenLimit: 1000, Enabled: true, BusinessCode: &optionCode,
	}
	require.NoError(t, model.DB.Create(&optionPlan).Error)
	var creditPlan model.SubscriptionPlan
	require.NoError(t, model.DB.First(&creditPlan, 9963).Error)
	snapshot := model.NewSubscriptionEntitlementSnapshot(&optionPlan, model.SubscriptionPurchaseModeCreditBalance, creditPlan.Id)
	snapshot.SetTargetCreditBalancePlanSnapshot(&creditPlan)
	snapshot.SetPaymentSnapshot(model.PaymentProviderEpay, "", "alipay", 4000, "CNY")
	snapshotJSON, err := model.MarshalSubscriptionEntitlementSnapshot(snapshot)
	require.NoError(t, err)
	order := model.SubscriptionOrder{
		UserId: userID, PlanId: optionPlan.Id, AmountCents: 4000, Currency: "CNY",
		CreditGrantAmount: 1000, CreditTargetPlanID: creditPlan.Id, TradeNo: "epay-admin-recovery-order",
		PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess,
		CompleteTime: common.GetTimestamp(), EntitlementSnapshot: snapshotJSON,
	}
	require.NoError(t, model.DB.Create(&order).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		UserId: userID, PlanId: creditPlan.Id, EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Status: model.SubscriptionStatusActive, TokenLimit: 1000, TokenUsed: 250,
		GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder,
	}).Error)

	preview := performSubscriptionOrderRecoveryPreviewRouteRequest(engine, token, userID, order.TradeNo)
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	assert.Contains(t, preview.Body.String(), `"user_id":9962`)
	assert.Contains(t, preview.Body.String(), `"amount_cents":4000`)
	assert.Contains(t, preview.Body.String(), `"gross_credit":1000`)
	wrongUser := performSubscriptionOrderRecoveryRouteRequest(engine, token, userID+1, order.TradeNo, `{"recovery_type":"refund","reason":"wrong owner"}`)
	assert.Contains(t, wrongUser.Body.String(), `"success":false`)
	first := performSubscriptionOrderRecoveryRouteRequest(engine, token, userID, order.TradeNo, `{"recovery_type":"refund","reason":"verified Epay refund"}`)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), `"replayed":false`)
	assert.Contains(t, first.Body.String(), `"gross_credit":1000`)
	replay := performSubscriptionOrderRecoveryRouteRequest(engine, token, userID, order.TradeNo, `{"recovery_type":"refund","reason":"verified Epay refund"}`)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	assert.Contains(t, replay.Body.String(), `"replayed":true`)

	var balance model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND entitlement_type = ?", userID, model.SubscriptionEntitlementCreditBalance).First(&balance).Error)
	assert.Equal(t, int64(1250), balance.TokenUsed)
	require.NoError(t, model.DB.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusRefunded, order.Status)
	assert.Equal(t, "verified Epay refund", order.RecoveryReason)
	var recoveryLedgers int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).
		Where("source_type = ? AND source_id = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).
		Count(&recoveryLedgers).Error)
	assert.Equal(t, int64(1), recoveryLedgers)
}

func TestAdminSubscriptionOrderRecoveryRouteValidatesAndRequiresAdmin(t *testing.T) {
	engine, token, _ := setupCreditAdjustmentRoute(t)
	unauthorized := performSubscriptionOrderRecoveryRouteRequest(engine, "invalid", 9962, "missing", `{"recovery_type":"refund","reason":"verified"}`)
	assert.Contains(t, unauthorized.Body.String(), `"success":false`)
	missingReason := performSubscriptionOrderRecoveryRouteRequest(engine, token, 9962, "missing", `{"recovery_type":"refund","reason":""}`)
	assert.Contains(t, missingReason.Body.String(), `"success":false`)
	invalidType := performSubscriptionOrderRecoveryRouteRequest(engine, token, 9962, "missing", `{"recovery_type":"cancel","reason":"verified"}`)
	assert.Contains(t, invalidType.Body.String(), `"success":false`)
	var recoveryLedgers int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("source_type = ?", model.CreditBalanceLedgerSourceSubscriptionOrderRecovery).Count(&recoveryLedgers).Error)
	assert.Zero(t, recoveryLedgers)
}

type creditBalanceLedgerRouteResponse struct {
	Success bool                                   `json:"success"`
	Message string                                 `json:"message"`
	Data    []model.CreditBalanceLedgerHistoryItem `json:"data"`
}

func performCreditBalanceLedgerRouteRequest(engine *gin.Engine, token string, userID int, path string) creditBalanceLedgerRouteResponse {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("New-Api-User", fmt.Sprintf("%d", userID))
	engine.ServeHTTP(recorder, request)
	var response creditBalanceLedgerRouteResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		panic(fmt.Sprintf("decode ledger response %q: %v", recorder.Body.String(), err))
	}
	return response
}

func TestCreditBalanceLedgerRoutesApplyAuthenticatedSourceTypeAndTimeFilters(t *testing.T) {
	engine, adminToken, userID := setupCreditAdjustmentRoute(t)
	userToken := "credit-ledger-user-token"
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("access_token", userToken).Error)
	balance := model.UserSubscription{
		UserId: userID, PlanId: 9963, EntitlementType: model.SubscriptionEntitlementCreditBalance,
		Status: model.SubscriptionStatusActive,
	}
	require.NoError(t, model.DB.Create(&balance).Error)
	entries := []model.CreditBalanceLedger{
		{UserId: userID, UserSubscriptionId: balance.Id, Type: model.CreditBalanceLedgerTypeAdminIncrease, IdempotencyKey: "ledger-route-match", SourceType: model.CreditBalanceLedgerSourceAdminAdjustment, SourceId: 9971, GrossCredit: 100, BalanceAfter: 100, AvailableCreditAfter: 100, CreatedAt: 200},
		{UserId: userID, UserSubscriptionId: balance.Id, Type: model.CreditBalanceLedgerTypeRedemption, IdempotencyKey: "ledger-route-wrong-type", SourceType: model.CreditBalanceLedgerSourceRedemption, SourceId: 9972, GrossCredit: 50, BalanceBefore: 100, BalanceAfter: 150, AvailableCreditAfter: 150, CreatedAt: 200},
		{UserId: userID, UserSubscriptionId: balance.Id, Type: model.CreditBalanceLedgerTypeAdminIncrease, IdempotencyKey: "ledger-route-outside-time", SourceType: model.CreditBalanceLedgerSourceAdminAdjustment, SourceId: 9973, GrossCredit: 25, BalanceBefore: 150, BalanceAfter: 175, AvailableCreditAfter: 175, CreatedAt: 100},
	}
	require.NoError(t, model.DB.Create(&entries).Error)
	query := "?source_type=admin_adjustment&type=admin_increase&start_time=150&end_time=250"

	adminResponse := performCreditBalanceLedgerRouteRequest(
		engine, adminToken, 9961,
		fmt.Sprintf("/api/subscription/admin/users/%d/credit-balance/ledger%s", userID, query),
	)
	require.True(t, adminResponse.Success, adminResponse.Message)
	require.Len(t, adminResponse.Data, 1)
	assert.Equal(t, "ledger-route-match", adminResponse.Data[0].IdempotencyKey)

	userResponse := performCreditBalanceLedgerRouteRequest(
		engine, userToken, userID,
		"/api/subscription/self/credit-balance/ledger"+query,
	)
	require.True(t, userResponse.Success, userResponse.Message)
	require.Len(t, userResponse.Data, 1)
	assert.Equal(t, "ledger-route-match", userResponse.Data[0].IdempotencyKey)
}
func seedAdminCreditAdjustmentValuationRoute(t *testing.T) int {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.CreditValuationMigration{}, &model.CreditValuationState{}))
	require.NoError(t, model.DB.Create(&model.CreditValuationMigration{
		Version: model.CreditValuationRuleVersion, Status: model.CreditValuationMigrationReady,
		ValuationCurrency: "CNY",
	}).Error)
	priceMicros := int64(40_000_000)
	optionCode := "admin-preview-option"
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id: 9965, Title: "40 CNY / 1,000 Credit", PriceAmount: 40,
		PriceAmountMicros: &priceMicros, Currency: "CNY", Enabled: true,
		EntitlementType: model.SubscriptionEntitlementTimed,
		DurationUnit:    model.SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod:  model.SubscriptionResetMonthly,
		MonthlyTokenLimit: 1_000, UnlimitedPurchaseEnabled: true,
		BusinessCode: &optionCode,
	}).Error)
	valuationCurrency := "CNY"
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9963).Update("valuation_currency", valuationCurrency).Error)
	return 9965
}

func TestAdminCreditAdjustmentPreviewRouteReturnsAuthoritativeMicrosWithoutWrites(t *testing.T) {
	engine, token, userID := setupCreditAdjustmentRoute(t)
	planID := seedAdminCreditAdjustmentValuationRoute(t)
	var writeCallbacks atomic.Int64
	const createCallback = "issue24:preview-create-guard"
	const updateCallback = "issue24:preview-update-guard"
	const deleteCallback = "issue24:preview-delete-guard"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(createCallback, func(_ *gorm.DB) {
		writeCallbacks.Add(1)
	}))
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(updateCallback, func(_ *gorm.DB) {
		writeCallbacks.Add(1)
	}))
	require.NoError(t, model.DB.Callback().Delete().Before("gorm:delete").Register(deleteCallback, func(_ *gorm.DB) {
		writeCallbacks.Add(1)
	}))
	t.Cleanup(func() {
		_ = model.DB.Callback().Create().Remove(createCallback)
		_ = model.DB.Callback().Update().Remove(updateCallback)
		_ = model.DB.Callback().Delete().Remove(deleteCallback)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/subscription/admin/users/%d/credit-balance/adjustments/preview", userID),
		bytes.NewBufferString(fmt.Sprintf(`{"operation":"increase","amount":800,"plan_id":%d}`, planID)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("New-Api-User", "9961")
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"plan_id":9965`)
	assert.Contains(t, recorder.Body.String(), `"gross_credit":800`)
	assert.Contains(t, recorder.Body.String(), `"gross_amount_micros":"32000000"`)
	assert.Contains(t, recorder.Body.String(), `"net_amount_micros":"32000000"`)
	assert.Contains(t, recorder.Body.String(), `"valuation_currency":"CNY"`)
	assert.Contains(t, recorder.Body.String(), `"source_currency":"CNY"`)
	assert.Contains(t, recorder.Body.String(), `"confidence":"exact"`)
	assert.Contains(t, recorder.Body.String(), `"preview":true`)

	var adjustments, ledgers, balances int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceAdjustment{}).Count(&adjustments).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Count(&ledgers).Error)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&balances).Error)
	assert.Zero(t, adjustments)
	assert.Zero(t, ledgers)
	assert.Zero(t, balances)
	assert.Zero(t, writeCallbacks.Load(), "preview must not execute create, update, or delete callbacks")
}

func TestAdminCreditAdjustmentCommitRouteForwardsPlanAndReturnsAuthoritativeResult(t *testing.T) {
	engine, token, userID := setupCreditAdjustmentRoute(t)
	planID := seedAdminCreditAdjustmentValuationRoute(t)
	response := performCreditAdjustmentRouteRequest(engine, token, userID, fmt.Sprintf(
		`{"operation":"increase","amount":800,"plan_id":%d,"idempotency_key":"admin-http-commit-800","reason":"after-sales grant"}`,
		planID,
	))

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"success":true`)
	assert.Contains(t, response.Body.String(), `"plan_id":9965`)
	assert.Contains(t, response.Body.String(), `"gross_credit":800`)
	assert.Contains(t, response.Body.String(), `"gross_amount_micros":"32000000"`)
	assert.Contains(t, response.Body.String(), `"net_amount_micros":"32000000"`)
	assert.Contains(t, response.Body.String(), `"state_version_after":1`)
	assert.Contains(t, response.Body.String(), `"replayed":false`)

	var adjustments, ledgers int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceAdjustment{}).Where("plan_id = ?", planID).Count(&adjustments).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("plan_id = ?", planID).Count(&ledgers).Error)
	assert.Equal(t, int64(1), adjustments)
	assert.Equal(t, int64(1), ledgers)
}

func TestAdminCreditAdjustmentRoutesExposeStableCodesAndReplayCommittedResult(t *testing.T) {
	engine, token, userID := setupCreditAdjustmentRoute(t)
	planID := seedAdminCreditAdjustmentValuationRoute(t)

	missingPlan := httptest.NewRecorder()
	missingPlanRequest := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/subscription/admin/users/%d/credit-balance/adjustments/preview", userID),
		bytes.NewBufferString(`{"operation":"increase","amount":800}`))
	missingPlanRequest.Header.Set("Content-Type", "application/json")
	missingPlanRequest.Header.Set("Authorization", "Bearer "+token)
	missingPlanRequest.Header.Set("New-Api-User", "9961")
	engine.ServeHTTP(missingPlan, missingPlanRequest)
	require.Equal(t, http.StatusOK, missingPlan.Code, missingPlan.Body.String())
	assert.Contains(t, missingPlan.Body.String(), `"success":false`)
	assert.Contains(t, missingPlan.Body.String(), `"code":"credit_valuation_plan_required"`)

	requestBody := fmt.Sprintf(
		`{"operation":"increase","amount":800,"plan_id":%d,"idempotency_key":"admin-http-replay-800","reason":"after-sales grant"}`,
		planID,
	)
	first := performCreditAdjustmentRouteRequest(engine, token, userID, requestBody)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), `"success":true`)
	assert.Contains(t, first.Body.String(), `"replayed":false`)
	replay := performCreditAdjustmentRouteRequest(engine, token, userID, requestBody)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	assert.Contains(t, replay.Body.String(), `"success":true`)
	assert.Contains(t, replay.Body.String(), `"replayed":true`)
	assert.Contains(t, replay.Body.String(), `"gross_amount_micros":"32000000"`)
	assert.Contains(t, replay.Body.String(), `"state_version_after":1`)

	conflict := performCreditAdjustmentRouteRequest(engine, token, userID, fmt.Sprintf(
		`{"operation":"increase","amount":801,"plan_id":%d,"idempotency_key":"admin-http-replay-800","reason":"after-sales grant"}`,
		planID,
	))
	require.Equal(t, http.StatusOK, conflict.Code, conflict.Body.String())
	assert.Contains(t, conflict.Body.String(), `"success":false`)
	assert.Contains(t, conflict.Body.String(), `"code":"credit_valuation_idempotency_mismatch"`)

	var adjustments, ledgers int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceAdjustment{}).Where("idempotency_key = ?", "admin-http-replay-800").Count(&adjustments).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("idempotency_key = ?", "admin-http-replay-800").Count(&ledgers).Error)
	assert.Equal(t, int64(1), adjustments)
	assert.Equal(t, int64(1), ledgers)
}

type adminCreditAdjustmentAuthoritativeRouteResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Data    struct {
		PlanId            int    `json:"plan_id"`
		GrossCredit       int64  `json:"gross_credit"`
		NetCredit         int64  `json:"net_credit"`
		GrossAmountMicros int64  `json:"gross_amount_micros,string"`
		NetAmountMicros   int64  `json:"net_amount_micros,string"`
		ValuationCurrency string `json:"valuation_currency"`
		SourceCurrency    string `json:"source_currency"`
		Confidence        string `json:"confidence"`
		FxRateNumerator   int64  `json:"fx_rate_numerator,string"`
		FxRateDenominator int64  `json:"fx_rate_denominator,string"`
		FxCapturedAt      int64  `json:"fx_captured_at"`
		FxDirection       string `json:"fx_direction"`
		RuleVersion       int    `json:"rule_version"`
		StateVersionAfter int64  `json:"state_version_after"`
		DebtOffset        int64  `json:"debt_offset"`
		AvailableCredit   int64  `json:"available_credit"`
		SettlementDebt    int64  `json:"settlement_debt"`
		BalanceBefore     int64  `json:"balance_before"`
		BalanceAfter      int64  `json:"balance_after"`
		Replayed          bool   `json:"replayed"`
		Preview           bool   `json:"preview"`
	} `json:"data"`
}

func decodeAdminCreditAdjustmentAuthoritativeRouteResponse(t *testing.T, recorder *httptest.ResponseRecorder) adminCreditAdjustmentAuthoritativeRouteResponse {
	t.Helper()
	var response adminCreditAdjustmentAuthoritativeRouteResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response
}

func TestAdminCreditAdjustmentReplayUsesFrozenFactsAfterPlanChanges(t *testing.T) {
	engine, token, userID := setupCreditAdjustmentRoute(t)
	planID := seedAdminCreditAdjustmentValuationRoute(t)
	requestBody := fmt.Sprintf(
		`{"operation":"increase","amount":800,"plan_id":%d,"idempotency_key":"admin-http-frozen-replay","reason":"after-sales grant"}`,
		planID,
	)
	firstRecorder := performCreditAdjustmentRouteRequest(engine, token, userID, requestBody)
	require.Equal(t, http.StatusOK, firstRecorder.Code, firstRecorder.Body.String())
	first := decodeAdminCreditAdjustmentAuthoritativeRouteResponse(t, firstRecorder)
	require.True(t, first.Success, firstRecorder.Body.String())
	require.False(t, first.Data.Replayed)
	require.Equal(t, int64(32_000_000), first.Data.GrossAmountMicros)
	require.Equal(t, "CNY", first.Data.SourceCurrency)
	require.Equal(t, "CNY", first.Data.ValuationCurrency)
	require.Equal(t, int64(1), first.Data.StateVersionAfter)

	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", planID).Updates(map[string]any{
		"enabled":             false,
		"price_amount_micros": int64(80_000_000),
		"monthly_token_limit": int64(2_000),
		"currency":            "USD",
	}).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 9963).Update("valuation_currency", "USD").Error)

	replayRecorder := performCreditAdjustmentRouteRequest(engine, token, userID, requestBody)
	require.Equal(t, http.StatusOK, replayRecorder.Code, replayRecorder.Body.String())
	replay := decodeAdminCreditAdjustmentAuthoritativeRouteResponse(t, replayRecorder)
	require.True(t, replay.Success, replayRecorder.Body.String())
	wantReplay := first.Data
	wantReplay.Replayed = true
	assert.Equal(t, wantReplay, replay.Data, "replay must return every authoritative field from the frozen committed ledger")

	conflictRecorder := performCreditAdjustmentRouteRequest(engine, token, userID, fmt.Sprintf(
		`{"operation":"increase","amount":801,"plan_id":%d,"idempotency_key":"admin-http-frozen-replay","reason":"after-sales grant"}`,
		planID,
	))
	require.Equal(t, http.StatusOK, conflictRecorder.Code, conflictRecorder.Body.String())
	conflict := decodeAdminCreditAdjustmentAuthoritativeRouteResponse(t, conflictRecorder)
	assert.False(t, conflict.Success)
	assert.Equal(t, "credit_valuation_idempotency_mismatch", conflict.Code)

	var adjustments, ledgers int64
	require.NoError(t, model.DB.Model(&model.CreditBalanceAdjustment{}).Where("idempotency_key = ?", "admin-http-frozen-replay").Count(&adjustments).Error)
	require.NoError(t, model.DB.Model(&model.CreditBalanceLedger{}).Where("idempotency_key = ?", "admin-http-frozen-replay").Count(&ledgers).Error)
	assert.Equal(t, int64(1), adjustments)
	assert.Equal(t, int64(1), ledgers)
}

func TestAdminCreditAdjustmentPreviewRequiresReadyValuationMarker(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T)
	}{
		{
			name: "missing marker",
			mutate: func(t *testing.T) {
				require.NoError(t, model.DB.Where("version = ?", model.CreditValuationRuleVersion).Delete(&model.CreditValuationMigration{}).Error)
			},
		},
		{
			name: "pending marker",
			mutate: func(t *testing.T) {
				require.NoError(t, model.DB.Model(&model.CreditValuationMigration{}).Where("version = ?", model.CreditValuationRuleVersion).Update("status", model.CreditValuationMigrationPending).Error)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine, token, userID := setupCreditAdjustmentRoute(t)
			planID := seedAdminCreditAdjustmentValuationRoute(t)
			test.mutate(t)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost,
				fmt.Sprintf("/api/subscription/admin/users/%d/credit-balance/adjustments/preview", userID),
				bytes.NewBufferString(fmt.Sprintf(`{"operation":"increase","amount":800,"plan_id":%d}`, planID)))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("New-Api-User", "9961")
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), `"success":false`)
			assert.Contains(t, response.Body.String(), `"code":"credit_valuation_migration_not_ready"`)
		})
	}
}
