package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type kyrenCheckoutFakeAPI struct {
	retrieveProductFunc func(context.Context, string) (*kyrenProduct, error)
	createCheckoutFunc  func(context.Context, kyrenCreateCheckoutRequest) (*kyrenCheckoutSession, error)

	retrieveIDs            []string
	createCheckoutRequests []kyrenCreateCheckoutRequest
}

func (f *kyrenCheckoutFakeAPI) createProduct(context.Context, kyrenCreateProductRequest) (*kyrenProduct, error) {
	return nil, errors.New("unexpected createProduct call")
}

func (f *kyrenCheckoutFakeAPI) updateProduct(context.Context, string, kyrenUpdateProductRequest) (*kyrenProduct, error) {
	return nil, errors.New("unexpected updateProduct call")
}

func (f *kyrenCheckoutFakeAPI) retrieveProduct(ctx context.Context, id string) (*kyrenProduct, error) {
	f.retrieveIDs = append(f.retrieveIDs, id)
	if f.retrieveProductFunc != nil {
		return f.retrieveProductFunc(ctx, id)
	}
	return &kyrenProduct{ID: id, Status: kyrenProductStatusActive, Price: "40.00", Currency: kyrenCurrencyCNY}, nil
}

func (f *kyrenCheckoutFakeAPI) listProducts(context.Context, string, int, int) (*kyrenProductList, error) {
	return nil, errors.New("unexpected listProducts call")
}

func (f *kyrenCheckoutFakeAPI) createCheckout(ctx context.Context, req kyrenCreateCheckoutRequest) (*kyrenCheckoutSession, error) {
	f.createCheckoutRequests = append(f.createCheckoutRequests, req)
	if f.createCheckoutFunc != nil {
		return f.createCheckoutFunc(ctx, req)
	}
	return &kyrenCheckoutSession{ID: "chk_test", URL: "https://pay.kyren.test/checkout", Status: "open", ExpiresAt: time.Now().Add(time.Hour).Unix()}, nil
}

func withKyrenCheckoutFakeControllerClient(t *testing.T, fake kyrenAPI) {
	t.Helper()
	original := newKyrenClientForController
	newKyrenClientForController = func() (kyrenAPI, error) { return fake, nil }
	t.Cleanup(func() { newKyrenClientForController = original })
}

func setupKyrenPaymentControllerTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.SubscriptionPlan{},
		&model.SubscriptionOrder{},
		&model.UserSubscription{},
		&model.TopUp{},
		&model.Log{},
		&model.Option{},
		&model.InvitationMonthlyEntitlement{},
		&model.InvitationRewardEvent{},
		&model.InvitationCommissionRecord{},
		&model.InvitationCommissionAccount{},
		&model.InvitationCommissionLedger{},
		&model.InvitationCommissionWithdrawal{},
	))

	originalAPIKey := setting.KyrenApiKey
	originalWebhookSecret := setting.KyrenWebhookSecret
	originalTopUps := setting.KyrenTopUpProducts
	originalOptionMap := common.OptionMap
	setting.KyrenApiKey = "kyren_test_key"
	setting.KyrenWebhookSecret = "kyren_webhook_secret"
	setting.KyrenTopUpProducts = "[]"
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{"KyrenTopUpProducts": "[]"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		setting.KyrenApiKey = originalAPIKey
		setting.KyrenWebhookSecret = originalWebhookSecret
		setting.KyrenTopUpProducts = originalTopUps
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})
}

func seedKyrenPaymentUser(t *testing.T, id int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{Id: id, Username: fmt.Sprintf("kyren-user-%d", id), Email: fmt.Sprintf("kyren-%d@example.com", id), Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("kyren-aff-%d", id)}).Error)
}

func seedKyrenPaymentPlan(t *testing.T, id int, productID string, tokenLimit int64, concurrency int) model.SubscriptionPlan {
	t.Helper()
	model.InvalidateSubscriptionPlanCache(id)
	t.Cleanup(func() { model.InvalidateSubscriptionPlanCache(id) })
	businessCode := fmt.Sprintf("kyren_payment_%d", id)
	plan := model.SubscriptionPlan{
		Id:                      id,
		Title:                   fmt.Sprintf("Kyren Plan %d", id),
		PriceAmount:             40,
		Currency:                kyrenCurrencyCNY,
		DurationUnit:            model.SubscriptionDurationMonth,
		DurationValue:           1,
		Enabled:                 true,
		PublicVisible:           true,
		TotalAmount:             8000,
		MonthlyTokenLimit:       tokenLimit,
		ConcurrencyLimit:        concurrency,
		QueueCapacity:           concurrency + 2,
		QuotaResetPeriod:        model.SubscriptionResetNever,
		BusinessCode:            &businessCode,
		KyrenProductId:          productID,
		RewardEligible:          true,
		MaxPurchasePerUser:      0,
		TrialDurationHours:      0,
		QuotaResetCustomSeconds: 0,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	return plan
}

func kyrenPaymentSnapshotJSON(t *testing.T, productID string, amount string, currency string) string {
	t.Helper()
	payload, err := model.MarshalKyrenPaymentSnapshot(model.KyrenPaymentSnapshot{ProductID: productID, Amount: amount, Currency: currency})
	require.NoError(t, err)
	return payload
}

func kyrenEntitlementSnapshotJSON(t *testing.T, plan *model.SubscriptionPlan) string {
	t.Helper()
	payload, err := model.MarshalSubscriptionEntitlementSnapshot(model.NewSubscriptionEntitlementSnapshotFromPlan(plan))
	require.NoError(t, err)
	return payload
}

func seedPendingKyrenSubscriptionOrder(t *testing.T, tradeNo string, userID int, plan *model.SubscriptionPlan) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId:              userID,
		PlanId:              plan.Id,
		Money:               plan.PriceAmount,
		TradeNo:             tradeNo,
		PaymentMethod:       model.PaymentMethodKyren,
		PaymentProvider:     model.PaymentProviderKyren,
		Status:              common.TopUpStatusPending,
		CreateTime:          common.GetTimestamp(),
		KyrenSnapshot:       kyrenPaymentSnapshotJSON(t, plan.KyrenProductId, "40.00", kyrenCurrencyCNY),
		EntitlementSnapshot: kyrenEntitlementSnapshotJSON(t, plan),
	}).Error)
}

func signKyrenWebhookPayload(payload []byte, timestamp string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func kyrenWebhookEventPayload(t *testing.T, eventType string, kind string, tradeNo string, productID string, amount string, currency string) []byte {
	t.Helper()
	metadata := map[string]string{}
	if kind != "" {
		metadata["kind"] = kind
	}
	if tradeNo != "" {
		metadata["trade_no"] = tradeNo
	}
	event := map[string]any{
		"id":   "evt_" + strings.ReplaceAll(tradeNo, "-", "_"),
		"type": eventType,
		"data": map[string]any{
			"id":        "ord_" + strings.ReplaceAll(tradeNo, "-", "_"),
			"productId": productID,
			"amount":    amount,
			"currency":  currency,
			"metadata":  metadata,
		},
	}
	payload, err := common.Marshal(event)
	require.NoError(t, err)
	return payload
}

func performSignedKyrenWebhook(t *testing.T, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/kyren/webhook", bytes.NewReader(payload))
	ctx.Request.Header.Set("Kyren-Timestamp", timestamp)
	ctx.Request.Header.Set("Kyren-Signature", signKyrenWebhookPayload(payload, timestamp, setting.KyrenWebhookSecret))
	KyrenWebhook(ctx)
	return recorder
}

func TestVerifyKyrenWebhookSignature(t *testing.T) {
	payload := []byte(`{"id":"evt_valid"}`)
	secret := "whsec_kyren"
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signature := signKyrenWebhookPayload(payload, timestamp, secret)

	assert.True(t, verifyKyrenWebhookSignature(payload, signature, timestamp, secret))
}

func TestVerifyKyrenWebhookSignatureRejectsEmptySecret(t *testing.T) {
	payload := []byte(`{"id":"evt_no_secret"}`)
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signature := signKyrenWebhookPayload(payload, timestamp, "secret")

	assert.False(t, verifyKyrenWebhookSignature(payload, signature, timestamp, ""))
}

func TestVerifyKyrenWebhookSignatureRejectsTamperedBody(t *testing.T) {
	payload := []byte(`{"id":"evt_original"}`)
	tampered := []byte(`{"id":"evt_tampered"}`)
	secret := "whsec_kyren"
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signature := signKyrenWebhookPayload(payload, timestamp, secret)

	assert.False(t, verifyKyrenWebhookSignature(tampered, signature, timestamp, secret))
}

func TestVerifyKyrenWebhookSignatureRejectsExpiredTimestamp(t *testing.T) {
	payload := []byte(`{"id":"evt_expired"}`)
	secret := "whsec_kyren"
	timestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).UnixMilli(), 10)
	signature := signKyrenWebhookPayload(payload, timestamp, secret)

	assert.False(t, verifyKyrenWebhookSignature(payload, signature, timestamp, secret))
}

func TestKyrenWebhookCompletesSubscriptionOrder(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6101
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6102, "prod_sub_basic", 1000, 2)
	tradeNo := "kyren-sub-complete"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, &plan)
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, "prod_sub_basic", "40.00", kyrenCurrencyCNY)

	first := performSignedKyrenWebhook(t, payload)
	second := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, model.PaymentMethodKyren, order.PaymentMethod)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, plan.Id).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
	var sub model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, plan.Id).First(&sub).Error)
	assert.Equal(t, int64(1000), sub.TokenLimit)
	assert.Equal(t, 2, sub.ConcurrencyLimit)
	var topUp model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&topUp).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, model.PaymentProviderKyren, topUp.PaymentProvider)
}

func TestKyrenWebhookSkipsInvitationRewardEventForTrialSubscriptionPlan(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	inviterID := 6103
	inviteeID := 6104
	require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "kyren-trial-inviter", Status: common.UserStatusEnabled, AffCode: "kyren-trial-inviter"}).Error)
	seedKyrenPaymentUser(t, inviteeID)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviteeID).Update("inviter_id", inviterID).Error)
	plan := seedKyrenPaymentPlan(t, 6105, "prod_sub_trial", 1000, 2)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{"is_trial": true, "reward_eligible": true}).Error)
	plan.IsTrial = true
	tradeNo := "kyren-sub-trial-skip-event"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, inviteeID, &plan)
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, plan.KyrenProductId, "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(0), eventCount)
}

func TestKyrenWebhookSkipsInvitationRewardEventForInviteTrialSnapshot(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	inviterID := 6106
	inviteeID := 6107
	require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "kyren-invite-trial-inviter", Status: common.UserStatusEnabled, AffCode: "kyren-invite-trial-inviter"}).Error)
	seedKyrenPaymentUser(t, inviteeID)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviteeID).Update("inviter_id", inviterID).Error)
	plan := seedKyrenPaymentPlan(t, 6108, "prod_sub_invite_trial_snapshot", 1000, 2)
	plan.InviteTrial = true
	tradeNo := "kyren-sub-invite-trial-snapshot"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, inviteeID, &plan)
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, plan.KyrenProductId, "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", inviteeID, plan.Id).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(0), eventCount)
}

func TestKyrenWebhookRecordsInvitationRewardEventForRewardIneligibleSnapshot(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	inviterID := 6111
	inviteeID := 6112
	require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "kyren-ineligible-inviter", Status: common.UserStatusEnabled, AffCode: "kyren-ineligible-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	seedKyrenPaymentUser(t, inviteeID)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviteeID).Update("inviter_id", inviterID).Error)
	plan := seedKyrenPaymentPlan(t, 6113, "prod_sub_reward_ineligible_snapshot", 1000, 2)
	plan.RewardEligible = false
	tradeNo := "kyren-sub-reward-ineligible-snapshot"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, inviteeID, &plan)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{"is_trial": true, "reward_eligible": true}).Error)
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, plan.KyrenProductId, "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var event model.InvitationRewardEvent
	require.NoError(t, model.DB.Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error)
	assert.Equal(t, inviterID, event.InviterId)
	assert.Equal(t, inviteeID, event.InviteeId)
	assert.Equal(t, int64(4000), event.SourceAmountCents)
	assert.Equal(t, kyrenCurrencyCNY, event.SourceCurrency)
}

func TestKyrenWebhookReturnsRetryableStatusWhenSubscriptionFulfillmentFails(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6109
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6110, "prod_sub_retryable", 1000, 1)
	tradeNo := "kyren-sub-retryable-failure"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, &plan)
	require.NoError(t, model.DB.Migrator().DropTable(&model.UserSubscription{}))
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, "prod_sub_retryable", "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}

func TestKyrenWebhookRetriesInvitationRewardHandlerForSuccessfulSubscriptionOrder(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	inviterID := 6121
	inviteeID := 6122
	inviter := model.User{Id: inviterID, Username: "kyren-retry-inviter", Status: common.UserStatusEnabled, AffCode: "kyren-retry-inviter", InvitationRewardMode: model.InvitationRewardModeSubscription}
	require.NoError(t, model.DB.Create(&inviter).Error)
	seedKyrenPaymentUser(t, inviteeID)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviteeID).Update("inviter_id", inviterID).Error)
	plan := seedKyrenPaymentPlan(t, 6123, "prod_sub_handler_retry", 1000, 1)
	tradeNo := "kyren-sub-handler-retry"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, inviteeID, &plan)
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, "prod_sub_handler_retry", "40.00", kyrenCurrencyCNY)

	calledOrderIDs := make([]int, 0, 2)
	SetInvitationRewardOrderHandlerForTest(t, func(orderId int) error {
		calledOrderIDs = append(calledOrderIDs, orderId)
		if len(calledOrderIDs) == 1 {
			return errors.New("injected Kyren invitation reward handler failure")
		}
		return nil
	})

	first := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusInternalServerError, first.Code, first.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, []int{order.Id}, calledOrderIDs)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", inviteeID, plan.Id).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)

	second := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Equal(t, []int{order.Id, order.Id}, calledOrderIDs)
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_type = ? AND source_id = ?", model.InvitationRewardEventSourceSubscriptionOrder, order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", inviteeID, plan.Id).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
}

func TestKyrenWebhookReturnsRetryableStatusWhileSubscriptionOrderClaimed(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	inviterID := 6131
	inviteeID := 6132
	require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "kyren-claimed-inviter", Status: common.UserStatusEnabled, AffCode: "kyren-claimed-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	seedKyrenPaymentUser(t, inviteeID)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviteeID).Update("inviter_id", inviterID).Error)
	plan := seedKyrenPaymentPlan(t, 6133, "prod_sub_claimed_retry", 1000, 1)
	tradeNo := "kyren-sub-claimed-retry"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, inviteeID, &plan)
	claimed, _, err := model.ClaimPendingKyrenSubscriptionOrder(tradeNo)
	require.NoError(t, err)
	require.True(t, claimed)
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, plan.KyrenProductId, "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", inviteeID, plan.Id).Count(&subCount).Error)
	assert.Equal(t, int64(0), subCount)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(0), eventCount)
	var recordCount int64
	require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", order.Id).Count(&recordCount).Error)
	assert.Equal(t, int64(0), recordCount)
}

func TestKyrenWebhookRecoversStaleClaimedSubscriptionOrder(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	inviterID := 6141
	inviteeID := 6142
	require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "kyren-stale-claim-inviter", Status: common.UserStatusEnabled, AffCode: "kyren-stale-claim-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	seedKyrenPaymentUser(t, inviteeID)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviteeID).Update("inviter_id", inviterID).Error)
	plan := seedKyrenPaymentPlan(t, 6143, "prod_sub_stale_claim", 1000, 1)
	tradeNo := "kyren-sub-stale-claim"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, inviteeID, &plan)
	claimed, _, err := model.ClaimPendingKyrenSubscriptionOrder(tradeNo)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("trade_no = ?", tradeNo).Update("complete_time", common.GetTimestamp()-600).Error)
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, plan.KyrenProductId, "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", inviteeID, plan.Id).Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestKyrenWebhookReturnsRetryableStatusWhenSubscriptionLookupFails(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6113
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6114, "prod_sub_lookup_failure", 1000, 1)
	tradeNo := "kyren-sub-lookup-failure"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, &plan)
	require.NoError(t, model.DB.Migrator().DropTable(&model.SubscriptionOrder{}))
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, "prod_sub_lookup_failure", "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
}

func TestKyrenRefundedTopUpRecordsManualActionForTopUpUser(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6119
	seedKyrenPaymentUser(t, userID)
	tradeNo := "kyren-topup-refund-user"
	seedPendingKyrenTopUp(t, tradeNo, userID, "topup_refund", "prod_topup_refund", "10.00", 5000000)
	payload := kyrenWebhookEventPayload(t, "order.refunded", "topup", tradeNo, "prod_topup_refund", "10.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var refundLog model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeRefund).First(&refundLog).Error)
	assert.Contains(t, refundLog.Content, tradeNo)
}

func TestKyrenWebhookCompletesSubscriptionOrderUsingEntitlementSnapshot(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6111
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6112, "prod_sub_snapshot", 1234, 3)
	tradeNo := "kyren-sub-snapshot"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, &plan)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{"monthly_token_limit": int64(9999), "concurrency_limit": 9, "total_amount": int64(99999)}).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, "prod_sub_snapshot", "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var sub model.UserSubscription
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, plan.Id).First(&sub).Error)
	assert.Equal(t, int64(1234), sub.TokenLimit)
	assert.Equal(t, 3, sub.ConcurrencyLimit)
	assert.Equal(t, int64(8000), sub.AmountTotal)
}

func TestKyrenWebhookCapturesOrderIDFromSnakeCasePayload(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6115
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6116, "prod_sub_order_id", 1000, 1)
	tradeNo := "kyren-sub-order-id"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, &plan)
	payload, err := common.Marshal(map[string]any{
		"id":   "evt_order_id",
		"type": "order.paid",
		"data": map[string]any{
			"order_id":   "ord_kyren_external",
			"product_id": "prod_sub_order_id",
			"amount":     "40.00",
			"currency":   kyrenCurrencyCNY,
			"metadata": map[string]string{
				"kind":     "subscription",
				"trade_no": tradeNo,
			},
		},
	})
	require.NoError(t, err)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
	var providerPayload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(order.ProviderPayload, &providerPayload))
	assert.Equal(t, "ord_kyren_external", providerPayload["order_id"])
}

func TestKyrenWebhookUnsupportedKindRecordsManualAction(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6117
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6118, "prod_sub_unknown_kind", 1000, 1)
	tradeNo := "kyren-sub-unsupported-kind"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, &plan)
	payload := kyrenWebhookEventPayload(t, "order.paid", "wallet", tradeNo, "prod_sub_unknown_kind", "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var manualLog model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeError).First(&manualLog).Error)
	assert.Contains(t, manualLog.Content, tradeNo)
	assert.Contains(t, manualLog.Content, "wallet")
}

func TestKyrenWebhookMissingMetadataDoesNotFulfillOrder(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6121
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6122, "prod_sub_missing_meta", 1000, 1)
	tradeNo := "kyren-sub-missing-metadata"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, &plan)
	payload, err := common.Marshal(map[string]any{
		"id":   "evt_missing_metadata",
		"type": "order.paid",
		"data": map[string]any{"productId": "prod_sub_missing_meta", "amount": "40.00", "currency": kyrenCurrencyCNY},
	})
	require.NoError(t, err)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&subCount).Error)
	assert.Equal(t, int64(0), subCount)
}

func TestKyrenWebhookRejectsProviderMismatch(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6131
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6132, "prod_sub_provider", 1000, 1)
	tradeNo := "kyren-sub-provider-mismatch"
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId:              userID,
		PlanId:              plan.Id,
		Money:               40,
		TradeNo:             tradeNo,
		PaymentMethod:       model.PaymentMethodStripe,
		PaymentProvider:     model.PaymentProviderStripe,
		Status:              common.TopUpStatusPending,
		CreateTime:          common.GetTimestamp(),
		KyrenSnapshot:       kyrenPaymentSnapshotJSON(t, "prod_sub_provider", "40.00", kyrenCurrencyCNY),
		EntitlementSnapshot: kyrenEntitlementSnapshotJSON(t, &plan),
	}).Error)
	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, "prod_sub_provider", "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&subCount).Error)
	assert.Equal(t, int64(0), subCount)
}

func TestKyrenWebhookRejectsAmountCurrencyOrProductMismatch(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	for i, tc := range []struct {
		name      string
		productID string
		amount    string
		currency  string
	}{
		{name: "product", productID: "prod_wrong", amount: "40.00", currency: kyrenCurrencyCNY},
		{name: "amount", productID: "prod_sub_mismatch", amount: "39.99", currency: kyrenCurrencyCNY},
		{name: "currency", productID: "prod_sub_mismatch", amount: "40.00", currency: "USD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userID := 6141 + i
			planID := 6142 + i
			seedKyrenPaymentUser(t, userID)
			plan := seedKyrenPaymentPlan(t, planID, "prod_sub_mismatch", 1000, 1)
			tradeNo := "kyren-sub-mismatch-" + tc.name
			seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, &plan)
			payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, tc.productID, tc.amount, tc.currency)

			recorder := performSignedKyrenWebhook(t, payload)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			order := model.GetSubscriptionOrderByTradeNo(tradeNo)
			require.NotNil(t, order)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
			var subCount int64
			require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&subCount).Error)
			assert.Equal(t, int64(0), subCount)
		})
	}
}

func TestKyrenSubscriptionOrderStoresEmptySnapshotWhenCurrencyUnsupported(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 9561
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 9562, "prod_kyren_snapshot", 1000, 1)

	tradeNo := "kyren-sub-empty-snapshot"
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId: userID, PlanId: plan.Id, Money: 40, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
		Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
		KyrenSnapshot:       kyrenPaymentSnapshotJSON(t, plan.KyrenProductId, "40.00", "USD"),
		EntitlementSnapshot: kyrenEntitlementSnapshotJSON(t, &plan),
	}).Error)

	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, plan.KyrenProductId, "40.00", "USD")
	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
	assert.Equal(t, int64(0), order.AmountCents)
	assert.Equal(t, "", order.Currency)
}

func TestKyrenSubscriptionOrderStoresCNYAmountSnapshot(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 9568
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 9570, "prod_kyren_cny_snapshot", 1000, 1)
	tradeNo := "kyren-sub-cny-snapshot"
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId: userID, PlanId: plan.Id, Money: 40, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
		Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
		KyrenSnapshot:       kyrenPaymentSnapshotJSON(t, plan.KyrenProductId, "40.00", "CNY"),
		EntitlementSnapshot: kyrenEntitlementSnapshotJSON(t, &plan),
	}).Error)

	payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, plan.KyrenProductId, "40.00", "CNY")
	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
	assert.Equal(t, int64(4000), order.AmountCents)
	assert.Equal(t, "CNY", order.Currency)
}

func TestKyrenSubscriptionCompletionRejectsProductAmountCurrencyMismatch(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	inviterID := 9571
	inviteeID := 9572
	require.NoError(t, model.DB.Create(&model.User{Id: inviterID, Username: "kyren-mismatch-inviter", Status: common.UserStatusEnabled, AffCode: "kyren-mismatch-inviter", InvitationRewardMode: model.InvitationRewardModeCommission}).Error)
	seedKyrenPaymentUser(t, inviteeID)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviteeID).Update("inviter_id", inviterID).Error)
	plan := seedKyrenPaymentPlan(t, 9573, "prod_kyren_mismatch_snapshot", 1000, 1)
	for _, tc := range []struct {
		name      string
		productID string
		amount    string
		currency  string
	}{
		{name: "product", productID: "prod_kyren_other_snapshot", amount: "40.00", currency: "CNY"},
		{name: "amount", productID: plan.KyrenProductId, amount: "41.00", currency: "CNY"},
		{name: "currency", productID: plan.KyrenProductId, amount: "40.00", currency: "USD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tradeNo := "kyren-sub-snapshot-mismatch-" + tc.name
			require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
				UserId: inviteeID, PlanId: plan.Id, Money: 40, AmountCents: 4000, Currency: "CNY", TradeNo: tradeNo,
				PaymentProvider: model.PaymentProviderKyren, PaymentMethod: model.PaymentMethodKyren,
				Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
				KyrenSnapshot:       kyrenPaymentSnapshotJSON(t, plan.KyrenProductId, "40.00", "CNY"),
				EntitlementSnapshot: kyrenEntitlementSnapshotJSON(t, &plan),
			}).Error)

			payload := kyrenWebhookEventPayload(t, "order.paid", "subscription", tradeNo, tc.productID, tc.amount, tc.currency)
			recorder := performSignedKyrenWebhook(t, payload)

			require.Equal(t, http.StatusOK, recorder.Code)
			var order model.SubscriptionOrder
			require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
			var events int64
			require.NoError(t, model.DB.Model(&model.InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&events).Error)
			assert.Equal(t, int64(0), events)
			var records int64
			require.NoError(t, model.DB.Model(&model.InvitationCommissionRecord{}).Where("source_id = ?", order.Id).Count(&records).Error)
			assert.Equal(t, int64(0), records)
		})
	}
}

func TestKyrenWebhookClosedOnlyExpiresPendingOrder(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6151
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6152, "prod_sub_closed", 1000, 1)
	pendingTradeNo := "kyren-sub-closed-pending"
	successTradeNo := "kyren-sub-closed-success"
	seedPendingKyrenSubscriptionOrder(t, pendingTradeNo, userID, &plan)
	seedPendingKyrenSubscriptionOrder(t, successTradeNo, userID, &plan)
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("trade_no = ?", successTradeNo).Updates(map[string]any{"status": common.TopUpStatusSuccess, "complete_time": common.GetTimestamp()}).Error)
	pendingPayload := kyrenWebhookEventPayload(t, "order.closed", "subscription", pendingTradeNo, "prod_sub_closed", "40.00", kyrenCurrencyCNY)
	successPayload := kyrenWebhookEventPayload(t, "order.closed", "subscription", successTradeNo, "prod_sub_closed", "40.00", kyrenCurrencyCNY)

	pendingRecorder := performSignedKyrenWebhook(t, pendingPayload)
	successRecorder := performSignedKyrenWebhook(t, successPayload)

	require.Equal(t, http.StatusOK, pendingRecorder.Code, pendingRecorder.Body.String())
	require.Equal(t, http.StatusOK, successRecorder.Code, successRecorder.Body.String())
	pendingOrder := model.GetSubscriptionOrderByTradeNo(pendingTradeNo)
	successOrder := model.GetSubscriptionOrderByTradeNo(successTradeNo)
	require.NotNil(t, pendingOrder)
	require.NotNil(t, successOrder)
	assert.Equal(t, common.TopUpStatusExpired, pendingOrder.Status)
	assert.Equal(t, common.TopUpStatusSuccess, successOrder.Status)
}

func TestKyrenWebhookRefundedRecordsManualActionAndReturnsSuccess(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6161
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6162, "prod_sub_refund", 1000, 1)
	tradeNo := "kyren-sub-refunded"
	seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, &plan)
	require.NoError(t, completeKyrenSubscriptionOrderWithSnapshotAndEvaluateInvitation(tradeNo, `{}`, model.PaymentProviderKyren, model.PaymentMethodKyren))
	payload := kyrenWebhookEventPayload(t, "order.refunded", "subscription", tradeNo, "prod_sub_refund", "40.00", kyrenCurrencyCNY)

	recorder := performSignedKyrenWebhook(t, payload)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var subCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND status = ?", userID, "active").Count(&subCount).Error)
	assert.Equal(t, int64(1), subCount)
	var refundLog model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeRefund).First(&refundLog).Error)
	assert.Contains(t, refundLog.Content, "Kyren")
	assert.Contains(t, refundLog.Content, tradeNo)
}

func TestKyrenCheckoutFailureFinalizesPendingOrder(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6171
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6172, "prod_sub_checkout_fail", 1000, 1)
	fake := &kyrenCheckoutFakeAPI{createCheckoutFunc: func(context.Context, kyrenCreateCheckoutRequest) (*kyrenCheckoutSession, error) {
		return nil, errors.New("checkout unavailable")
	}}
	withKyrenCheckoutFakeControllerClient(t, fake)

	recorder := performKyrenSubscriptionPayRequest(t, userID, fmt.Sprintf(`{"plan_id":%d}`, plan.Id))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "拉起支付失败")
	var pendingCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ? AND status = ?", userID, plan.Id, common.TopUpStatusPending).Count(&pendingCount).Error)
	assert.Equal(t, int64(0), pendingCount)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("user_id = ? AND plan_id = ?", userID, plan.Id).First(&order).Error)
	assert.NotEqual(t, common.TopUpStatusPending, order.Status)
	assert.Equal(t, model.PaymentProviderKyren, order.PaymentProvider)
}

func TestKyrenCheckoutRejectsArchivedOrMismatchedProductBeforeOrderCreation(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6181
	seedKyrenPaymentUser(t, userID)
	archivedPlan := seedKyrenPaymentPlan(t, 6182, "prod_sub_archived_checkout", 1000, 1)
	fake := &kyrenCheckoutFakeAPI{retrieveProductFunc: func(_ context.Context, id string) (*kyrenProduct, error) {
		return &kyrenProduct{ID: id, Status: "ARCHIVED", Price: "40.00", Currency: kyrenCurrencyCNY}, nil
	}}
	withKyrenCheckoutFakeControllerClient(t, fake)

	archivedRecorder := performKyrenSubscriptionPayRequest(t, userID, fmt.Sprintf(`{"plan_id":%d}`, archivedPlan.Id))

	require.Equal(t, http.StatusOK, archivedRecorder.Code, archivedRecorder.Body.String())
	assert.Contains(t, archivedRecorder.Body.String(), "不可用")
	var orderCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", userID, archivedPlan.Id).Count(&orderCount).Error)
	assert.Equal(t, int64(0), orderCount)
	assert.Empty(t, fake.createCheckoutRequests)

	mismatchPlan := seedKyrenPaymentPlan(t, 6183, "prod_sub_mismatch_checkout", 1000, 1)
	fake.retrieveProductFunc = func(_ context.Context, id string) (*kyrenProduct, error) {
		return &kyrenProduct{ID: id, Status: kyrenProductStatusActive, Price: "39.99", Currency: kyrenCurrencyCNY}, nil
	}
	mismatchRecorder := performKyrenSubscriptionPayRequest(t, userID, fmt.Sprintf(`{"plan_id":%d}`, mismatchPlan.Id))

	require.Equal(t, http.StatusOK, mismatchRecorder.Code, mismatchRecorder.Body.String())
	assert.Contains(t, mismatchRecorder.Body.String(), "不匹配")
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", userID, mismatchPlan.Id).Count(&orderCount).Error)
	assert.Equal(t, int64(0), orderCount)
}

func TestKyrenCheckoutMetadataIncludesStableBusinessFields(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6187
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6188, "prod_sub_metadata", 1000, 1)
	fake := &kyrenCheckoutFakeAPI{}
	withKyrenCheckoutFakeControllerClient(t, fake)

	response := performKyrenSubscriptionPayRequest(t, userID, fmt.Sprintf(`{"plan_id":%d}`, plan.Id))

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Len(t, fake.createCheckoutRequests, 1)
	metadata := fake.createCheckoutRequests[0].Metadata
	assert.Equal(t, "subscription", metadata["kind"])
	assert.NotEmpty(t, metadata["trade_no"])
	assert.Equal(t, strconv.Itoa(userID), metadata["user_id"])
	assert.Equal(t, strconv.Itoa(plan.Id), metadata["plan_id"])
}

func TestKyrenTopUpCheckoutMetadataIncludesStableBusinessFields(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6189
	seedKyrenPaymentUser(t, userID)
	product := seedKyrenPayTopUpProduct(t, "topup_metadata", "prod_topup_metadata", "10.00", 5000000)
	fake := &kyrenCheckoutFakeAPI{retrieveProductFunc: func(_ context.Context, id string) (*kyrenProduct, error) {
		return &kyrenProduct{ID: id, Status: kyrenProductStatusActive, Price: product.Amount, Currency: kyrenCurrencyCNY}, nil
	}}
	withKyrenCheckoutFakeControllerClient(t, fake)

	response := performKyrenTopUpPayRequest(t, userID, `{"product_id":"topup_metadata"}`)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Len(t, fake.createCheckoutRequests, 1)
	metadata := fake.createCheckoutRequests[0].Metadata
	assert.Equal(t, "topup", metadata["kind"])
	assert.NotEmpty(t, metadata["trade_no"])
	assert.Equal(t, strconv.Itoa(userID), metadata["user_id"])
	assert.Equal(t, product.ID, metadata["topup_product_id"])
}

func TestKyrenPayRejectsMissingWebhookSecret(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	setting.KyrenApiKey = "kyren_test_key"
	setting.KyrenWebhookSecret = ""
	userID := 6191
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6192, "prod_sub_missing_secret", 1000, 1)
	fake := &kyrenCheckoutFakeAPI{}
	withKyrenCheckoutFakeControllerClient(t, fake)

	subRecorder := performKyrenSubscriptionPayRequest(t, userID, fmt.Sprintf(`{"plan_id":%d}`, plan.Id))
	topUpRecorder := performKyrenTopUpPayRequest(t, userID, `{"product_id":"topup_missing_secret"}`)

	require.Equal(t, http.StatusOK, subRecorder.Code, subRecorder.Body.String())
	assert.Contains(t, subRecorder.Body.String(), "Webhook Secret")
	require.Equal(t, http.StatusOK, topUpRecorder.Code, topUpRecorder.Body.String())
	assert.Contains(t, topUpRecorder.Body.String(), "Webhook Secret")
	var orderCount int64
	require.NoError(t, model.DB.Model(&model.SubscriptionOrder{}).Where("user_id = ?", userID).Count(&orderCount).Error)
	assert.Equal(t, int64(0), orderCount)
	var topUpCount int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("user_id = ?", userID).Count(&topUpCount).Error)
	assert.Equal(t, int64(0), topUpCount)
	assert.Empty(t, fake.retrieveIDs)
}

func performKyrenSubscriptionPayRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/kyren/pay", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	SubscriptionRequestKyrenPay(ctx)
	return recorder
}
