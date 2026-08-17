package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type kyrenReconciliationFakeAPI struct {
	retrieveCheckoutFunc func(context.Context, string) (*kyrenCheckoutSession, error)
	retrieveOrderFunc    func(context.Context, string) (*kyrenOrder, error)
	retrieveCheckoutIDs  []string
	retrieveOrderIDs     []string
}

func (f *kyrenReconciliationFakeAPI) createProduct(context.Context, kyrenCreateProductRequest) (*kyrenProduct, error) {
	return nil, errors.New("unexpected createProduct call")
}

func (f *kyrenReconciliationFakeAPI) updateProduct(context.Context, string, kyrenUpdateProductRequest) (*kyrenProduct, error) {
	return nil, errors.New("unexpected updateProduct call")
}

func (f *kyrenReconciliationFakeAPI) retrieveProduct(context.Context, string) (*kyrenProduct, error) {
	return nil, errors.New("unexpected retrieveProduct call")
}

func (f *kyrenReconciliationFakeAPI) listProducts(context.Context, string, int, int) (*kyrenProductList, error) {
	return nil, errors.New("unexpected listProducts call")
}

func (f *kyrenReconciliationFakeAPI) createCheckout(context.Context, kyrenCreateCheckoutRequest) (*kyrenCheckoutSession, error) {
	return nil, errors.New("unexpected createCheckout call")
}

func (f *kyrenReconciliationFakeAPI) retrieveCheckout(ctx context.Context, id string) (*kyrenCheckoutSession, error) {
	f.retrieveCheckoutIDs = append(f.retrieveCheckoutIDs, id)
	if f.retrieveCheckoutFunc == nil {
		return nil, errors.New("unexpected retrieveCheckout call")
	}
	return f.retrieveCheckoutFunc(ctx, id)
}

func (f *kyrenReconciliationFakeAPI) retrieveOrder(ctx context.Context, id string) (*kyrenOrder, error) {
	f.retrieveOrderIDs = append(f.retrieveOrderIDs, id)
	if f.retrieveOrderFunc == nil {
		return nil, errors.New("unexpected retrieveOrder call")
	}
	return f.retrieveOrderFunc(ctx, id)
}

func seedKyrenReconciliationSubscription(t *testing.T, tradeNo string, checkoutID string, userID int, plan *model.SubscriptionPlan) model.SubscriptionOrder {
	t.Helper()
	seedPendingKyrenSubscriptionOrder(t, tradeNo, userID, plan)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		_, err := model.EnsurePaymentProviderOrderTx(tx, model.PaymentProviderKyren, model.PaymentOrderKindSubscription, tradeNo)
		return err
	}))
	require.NoError(t, service.BindKyrenPaymentCheckout(tradeNo, checkoutID))
	return order
}

func ageKyrenReconciliationMapping(t *testing.T, tradeNo string) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.PaymentProviderOrder{}).
		Where("provider = ? AND trade_no = ?", model.PaymentProviderKyren, tradeNo).
		Update("updated_at", common.GetTimestamp()-60).Error)
}

func performKyrenSubscriptionOrderStatusRequest(userID int, tradeNo string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/orders/"+tradeNo, nil)
	ctx.Set("id", userID)
	ctx.Params = gin.Params{{Key: "trade_no", Value: tradeNo}}
	GetSubscriptionOrderStatus(ctx)
	return recorder
}

func decodeKyrenSubscriptionOrderStatus(t *testing.T, recorder *httptest.ResponseRecorder) SubscriptionOrderStatusResponse {
	t.Helper()
	var response struct {
		Success bool                            `json:"success"`
		Data    SubscriptionOrderStatusResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	require.True(t, response.Success, recorder.Body.String())
	return response.Data
}

func kyrenReconciliationCheckout(checkoutID string, orderID string, productID string, status string) *kyrenCheckoutSession {
	return &kyrenCheckoutSession{
		ID: checkoutID, ProductID: productID, Amount: "40.00", Currency: kyrenCurrencyCNY,
		Status: status, OrderID: orderID, ExpiresAt: common.GetTimestamp()*1000 + 60_000,
	}
}

func kyrenReconciliationOrder(providerOrderID string, checkoutID string, productID string, status string, tradeNo string, userID int, planID int) *kyrenOrder {
	return &kyrenOrder{
		ID: providerOrderID, CheckoutSessionID: checkoutID, ProductID: productID,
		Amount: "40.00", Currency: kyrenCurrencyCNY, Status: status,
		Metadata: map[string]string{
			"kind": "subscription", "trade_no": tradeNo,
			"user_id": strconv.Itoa(userID), "plan_id": strconv.Itoa(planID),
		},
		UpdatedAt: common.GetTimestamp() * 1000,
	}
}

func TestGetSubscriptionOrderStatusReconcilesPaidKyrenOrderAndLaterWebhookIsIdempotent(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6211
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6212, "prod_reconcile_paid", 1200, 2)
	tradeNo := "kyren-reconcile-paid"
	checkoutID := "cs_reconcile_paid"
	providerOrderID := "order_reconcile_paid"
	seedKyrenReconciliationSubscription(t, tradeNo, checkoutID, userID, &plan)
	ageKyrenReconciliationMapping(t, tradeNo)
	fake := &kyrenReconciliationFakeAPI{
		retrieveCheckoutFunc: func(context.Context, string) (*kyrenCheckoutSession, error) {
			return kyrenReconciliationCheckout(checkoutID, providerOrderID, plan.KyrenProductId, "COMPLETE"), nil
		},
		retrieveOrderFunc: func(context.Context, string) (*kyrenOrder, error) {
			return kyrenReconciliationOrder(providerOrderID, checkoutID, plan.KyrenProductId, "PAID", tradeNo, userID, plan.Id), nil
		},
	}
	withKyrenCheckoutFakeControllerClient(t, fake)

	first := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)

	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	status := decodeKyrenSubscriptionOrderStatus(t, first)
	assert.Equal(t, common.TopUpStatusSuccess, status.Status)
	assert.Equal(t, []string{checkoutID}, fake.retrieveCheckoutIDs)
	assert.Equal(t, []string{providerOrderID}, fake.retrieveOrderIDs)
	var order model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Positive(t, order.FulfilledSubscriptionID)
	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, plan.Id).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)

	webhookPayload, err := common.Marshal(map[string]any{
		"id": "evt_paid_after_reconciliation", "type": service.KyrenPaymentEventPaid,
		"data": map[string]any{
			"order_id": providerOrderID, "product_id": plan.KyrenProductId,
			"amount": "40.00", "currency": kyrenCurrencyCNY,
			"metadata": map[string]string{
				"kind": "subscription", "trade_no": tradeNo,
				"user_id": strconv.Itoa(userID), "plan_id": strconv.Itoa(plan.Id),
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, performSignedKyrenWebhook(t, webhookPayload).Code)

	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, plan.Id).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
	var events []model.PaymentProviderEvent
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).Order("id").Find(&events).Error)
	require.Len(t, events, 2)
	assert.True(t, strings.HasPrefix(events[0].EventID, "reconcile_"))
	assert.Equal(t, model.PaymentProviderEventApplied, events[0].Status)
	assert.Equal(t, model.PaymentProviderEventApplied, events[1].Status)
}

func TestGetSubscriptionOrderStatusReconcilesFailedKyrenOrder(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6221
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6222, "prod_reconcile_failed", 1000, 1)
	tradeNo := "kyren-reconcile-failed"
	checkoutID := "cs_reconcile_failed"
	providerOrderID := "order_reconcile_failed"
	seedKyrenReconciliationSubscription(t, tradeNo, checkoutID, userID, &plan)
	ageKyrenReconciliationMapping(t, tradeNo)
	fake := &kyrenReconciliationFakeAPI{
		retrieveCheckoutFunc: func(context.Context, string) (*kyrenCheckoutSession, error) {
			return kyrenReconciliationCheckout(checkoutID, providerOrderID, plan.KyrenProductId, "OPEN"), nil
		},
		retrieveOrderFunc: func(context.Context, string) (*kyrenOrder, error) {
			return kyrenReconciliationOrder(providerOrderID, checkoutID, plan.KyrenProductId, "FAILED", tradeNo, userID, plan.Id), nil
		},
	}
	withKyrenCheckoutFakeControllerClient(t, fake)

	recorder := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusFailed, decodeKyrenSubscriptionOrderStatus(t, recorder).Status)
	assert.Equal(t, common.TopUpStatusFailed, model.GetSubscriptionOrderByTradeNo(tradeNo).Status)
	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)
}

func TestGetSubscriptionOrderStatusExpiresKyrenCheckoutWithoutProviderOrder(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6231
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6232, "prod_reconcile_expired", 1000, 1)
	tradeNo := "kyren-reconcile-expired"
	checkoutID := "cs_reconcile_expired"
	seedKyrenReconciliationSubscription(t, tradeNo, checkoutID, userID, &plan)
	ageKyrenReconciliationMapping(t, tradeNo)
	fake := &kyrenReconciliationFakeAPI{
		retrieveCheckoutFunc: func(context.Context, string) (*kyrenCheckoutSession, error) {
			return kyrenReconciliationCheckout(checkoutID, "", plan.KyrenProductId, "EXPIRED"), nil
		},
	}
	withKyrenCheckoutFakeControllerClient(t, fake)

	recorder := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusExpired, decodeKyrenSubscriptionOrderStatus(t, recorder).Status)
	assert.Empty(t, fake.retrieveOrderIDs)
	var event model.PaymentProviderEvent
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&event).Error)
	assert.Equal(t, service.KyrenPaymentEventClosed, event.EventType)
	assert.Empty(t, event.ProviderOrderID)
	assert.Equal(t, model.PaymentProviderEventApplied, event.Status)
}

func TestGetSubscriptionOrderStatusRejectsMismatchedKyrenIdentityWithoutFulfillment(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6241
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6242, "prod_reconcile_mismatch", 1000, 1)
	tradeNo := "kyren-reconcile-mismatch"
	checkoutID := "cs_reconcile_mismatch"
	providerOrderID := "order_reconcile_mismatch"
	seedKyrenReconciliationSubscription(t, tradeNo, checkoutID, userID, &plan)
	ageKyrenReconciliationMapping(t, tradeNo)
	fake := &kyrenReconciliationFakeAPI{
		retrieveCheckoutFunc: func(context.Context, string) (*kyrenCheckoutSession, error) {
			return kyrenReconciliationCheckout(checkoutID, providerOrderID, plan.KyrenProductId, "COMPLETE"), nil
		},
		retrieveOrderFunc: func(context.Context, string) (*kyrenOrder, error) {
			return kyrenReconciliationOrder(providerOrderID, checkoutID, plan.KyrenProductId, "PAID", tradeNo, userID+1, plan.Id), nil
		},
	}
	withKyrenCheckoutFakeControllerClient(t, fake)

	recorder := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusPending, decodeKyrenSubscriptionOrderStatus(t, recorder).Status)
	assert.Equal(t, common.TopUpStatusPending, model.GetSubscriptionOrderByTradeNo(tradeNo).Status)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.PaymentProviderEvent{}).Where("trade_no = ?", tradeNo).Count(&eventCount).Error)
	assert.Zero(t, eventCount)
	var mapping model.PaymentProviderOrder
	require.NoError(t, model.DB.Where("provider = ? AND trade_no = ?", model.PaymentProviderKyren, tradeNo).First(&mapping).Error)
	assert.Nil(t, mapping.ProviderOrderID)
	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)
}

func TestGetSubscriptionOrderStatusThrottlesUnavailableKyrenProvider(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6251
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6252, "prod_reconcile_throttle", 1000, 1)
	tradeNo := "kyren-reconcile-throttle"
	checkoutID := "cs_reconcile_throttle"
	seedKyrenReconciliationSubscription(t, tradeNo, checkoutID, userID, &plan)
	fake := &kyrenReconciliationFakeAPI{
		retrieveCheckoutFunc: func(context.Context, string) (*kyrenCheckoutSession, error) {
			return nil, errors.New("provider unavailable")
		},
	}
	withKyrenCheckoutFakeControllerClient(t, fake)

	first := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Equal(t, common.TopUpStatusPending, decodeKyrenSubscriptionOrderStatus(t, first).Status)
	assert.Empty(t, fake.retrieveCheckoutIDs)

	ageKyrenReconciliationMapping(t, tradeNo)
	second := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)
	third := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)

	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.Equal(t, http.StatusOK, third.Code, third.Body.String())
	assert.Equal(t, common.TopUpStatusPending, decodeKyrenSubscriptionOrderStatus(t, second).Status)
	assert.Equal(t, common.TopUpStatusPending, decodeKyrenSubscriptionOrderStatus(t, third).Status)
	assert.Equal(t, []string{checkoutID}, fake.retrieveCheckoutIDs)
}

func TestGetSubscriptionOrderStatusRecordsKyrenChargebackForManualActionOnce(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6261
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6262, "prod_reconcile_chargeback", 1000, 1)
	tradeNo := "kyren-reconcile-chargeback"
	checkoutID := "cs_reconcile_chargeback"
	providerOrderID := "order_reconcile_chargeback"
	seedKyrenReconciliationSubscription(t, tradeNo, checkoutID, userID, &plan)
	ageKyrenReconciliationMapping(t, tradeNo)
	fake := &kyrenReconciliationFakeAPI{
		retrieveCheckoutFunc: func(context.Context, string) (*kyrenCheckoutSession, error) {
			return kyrenReconciliationCheckout(checkoutID, providerOrderID, plan.KyrenProductId, "COMPLETE"), nil
		},
		retrieveOrderFunc: func(context.Context, string) (*kyrenOrder, error) {
			return kyrenReconciliationOrder(providerOrderID, checkoutID, plan.KyrenProductId, "CHARGEBACK", tradeNo, userID, plan.Id), nil
		},
	}
	withKyrenCheckoutFakeControllerClient(t, fake)

	first := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Equal(t, common.TopUpStatusPending, decodeKyrenSubscriptionOrderStatus(t, first).Status)
	var event model.PaymentProviderEvent
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&event).Error)
	assert.Equal(t, "order.chargeback", event.EventType)
	assert.Equal(t, model.PaymentProviderEventIgnored, event.Status)
	var logCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", userID, model.LogTypeError).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)

	ageKyrenReconciliationMapping(t, tradeNo)
	second := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", userID, model.LogTypeError).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)
}

func TestGetSubscriptionOrderStatusFinalizesPendingKyrenRefund(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6271
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6272, "prod_reconcile_refund", 1000, 1)
	tradeNo := "kyren-reconcile-refund"
	checkoutID := "cs_reconcile_refund"
	providerOrderID := "order_reconcile_refund"
	seedKyrenReconciliationSubscription(t, tradeNo, checkoutID, userID, &plan)
	ageKyrenReconciliationMapping(t, tradeNo)
	fake := &kyrenReconciliationFakeAPI{
		retrieveCheckoutFunc: func(context.Context, string) (*kyrenCheckoutSession, error) {
			return kyrenReconciliationCheckout(checkoutID, providerOrderID, plan.KyrenProductId, "COMPLETE"), nil
		},
		retrieveOrderFunc: func(context.Context, string) (*kyrenOrder, error) {
			return kyrenReconciliationOrder(providerOrderID, checkoutID, plan.KyrenProductId, "REFUNDED", tradeNo, userID, plan.Id), nil
		},
	}
	withKyrenCheckoutFakeControllerClient(t, fake)

	recorder := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusRefunded, decodeKyrenSubscriptionOrderStatus(t, recorder).Status)
	assert.Equal(t, common.TopUpStatusRefunded, model.GetSubscriptionOrderByTradeNo(tradeNo).Status)
	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)
	var event model.PaymentProviderEvent
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&event).Error)
	assert.Equal(t, model.PaymentProviderEventApplied, event.Status)
	assert.Equal(t, service.KyrenPaymentEventRefunded, event.EventType)
}

func TestGetSubscriptionOrderStatusLeavesOpenKyrenOrderPending(t *testing.T) {
	setupKyrenPaymentControllerTestDB(t)
	userID := 6281
	seedKyrenPaymentUser(t, userID)
	plan := seedKyrenPaymentPlan(t, 6282, "prod_reconcile_open", 1000, 1)
	tradeNo := "kyren-reconcile-open"
	checkoutID := "cs_reconcile_open"
	seedKyrenReconciliationSubscription(t, tradeNo, checkoutID, userID, &plan)
	ageKyrenReconciliationMapping(t, tradeNo)
	fake := &kyrenReconciliationFakeAPI{
		retrieveCheckoutFunc: func(context.Context, string) (*kyrenCheckoutSession, error) {
			return kyrenReconciliationCheckout(checkoutID, "", plan.KyrenProductId, "OPEN"), nil
		},
	}
	withKyrenCheckoutFakeControllerClient(t, fake)

	recorder := performKyrenSubscriptionOrderStatusRequest(userID, tradeNo)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusPending, decodeKyrenSubscriptionOrderStatus(t, recorder).Status)
	assert.Empty(t, fake.retrieveOrderIDs)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.PaymentProviderEvent{}).Where("trade_no = ?", tradeNo).Count(&eventCount).Error)
	assert.Zero(t, eventCount)
}

func TestKyrenReconciliationFactFormattingIsStable(t *testing.T) {
	fact := kyrenReconciliationFact{
		Source: "kyren_reconciliation", EventType: service.KyrenPaymentEventPaid,
		CheckoutID: "cs_stable", OrderID: "order_stable", ProductID: "prod_stable",
		Amount: "40.00", Currency: "CNY", Status: "PAID", UpdatedAt: 123,
		Metadata: map[string]string{"plan_id": "2", "user_id": "1", "trade_no": "trade", "kind": "subscription"},
	}
	firstPayload, firstEventID, firstHash, err := marshalKyrenReconciliationFact(fact)
	require.NoError(t, err)
	secondPayload, secondEventID, secondHash, err := marshalKyrenReconciliationFact(fact)
	require.NoError(t, err)
	assert.Equal(t, firstPayload, secondPayload)
	assert.Equal(t, firstEventID, secondEventID)
	assert.Equal(t, firstHash, secondHash)
	assert.True(t, strings.HasPrefix(firstEventID, "reconcile_"))
	assert.Len(t, firstHash, 64)
	assert.Contains(t, firstPayload, fmt.Sprintf(`"updated_at":%d`, fact.UpdatedAt))
}
