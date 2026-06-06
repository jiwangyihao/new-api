package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserForPaymentGuardTest(t *testing.T, id int, quota int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "payment_guard_user",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertSubscriptionPlanForPaymentGuardTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Guard Plan",
		PriceAmount:   9.99,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionOrderForPaymentGuardTest(t *testing.T, tradeNo string, userID int, planID int, paymentProvider string) {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:          userID,
		PlanId:          planID,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
}

func insertTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, paymentProvider string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func getTopUpStatusForPaymentGuardTest(t *testing.T, tradeNo string) string {
	t.Helper()
	topUp := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	return topUp.Status
}

func countUserSubscriptionsForPaymentGuardTest(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func getUserQuotaForPaymentGuardTest(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func TestRechargeWaffoPancake_RejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 101, 0)
	insertTopUpForPaymentGuardTest(t, "waffo-pancake-guard", 101, PaymentProviderStripe)

	err := RechargeWaffoPancake("waffo-pancake-guard")
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("waffo-pancake-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 101))
}

func TestUpdatePendingTopUpStatus_RejectsMismatchedPaymentProvider(t *testing.T) {
	testCases := []struct {
		name                    string
		tradeNo                 string
		storedPaymentProvider   string
		expectedPaymentProvider string
		targetStatus            string
	}{
		{
			name:                    "stripe expire",
			tradeNo:                 "stripe-expire-guard",
			storedPaymentProvider:   PaymentProviderCreem,
			expectedPaymentProvider: PaymentProviderStripe,
			targetStatus:            common.TopUpStatusExpired,
		},
		{
			name:                    "waffo failed",
			tradeNo:                 "waffo-failed-guard",
			storedPaymentProvider:   PaymentProviderStripe,
			expectedPaymentProvider: PaymentProviderWaffo,
			targetStatus:            common.TopUpStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 150, 0)
			insertTopUpForPaymentGuardTest(t, tc.tradeNo, 150, tc.storedPaymentProvider)

			err := UpdatePendingTopUpStatus(tc.tradeNo, tc.expectedPaymentProvider, tc.targetStatus)
			require.ErrorIs(t, err, ErrPaymentMethodMismatch)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tc.tradeNo))
		})
	}
}

func TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 301)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-guard-order", 202, plan.Id, PaymentProviderStripe)

	_, err := CompleteSubscriptionOrder("sub-guard-order", `{"provider":"epay"}`, PaymentProviderEpay, "alipay")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-guard-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 202))

	topUp := GetTopUpByTradeNo("sub-guard-order")
	assert.Nil(t, topUp)
}

func TestCompleteSubscriptionOrder_RejectsInvalidStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
	}{
		{name: "failed", status: common.TopUpStatusFailed},
		{name: "expired", status: common.TopUpStatusExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			require.NoError(t, DB.AutoMigrate(&InvitationRewardEvent{}))
			require.NoError(t, DB.Exec("DELETE FROM invitation_reward_events").Error)
			userID := 260
			planID := 360
			tradeNo := "sub-invalid-status-" + tc.name
			insertUserForPaymentGuardTest(t, userID, 0)
			plan := insertSubscriptionPlanForPaymentGuardTest(t, planID)
			order := &SubscriptionOrder{
				UserId:          userID,
				PlanId:          plan.Id,
				Money:           9.99,
				AmountCents:     999,
				Currency:        "CNY",
				TradeNo:         tradeNo,
				PaymentMethod:   PaymentMethodStripe,
				PaymentProvider: PaymentProviderStripe,
				Status:          tc.status,
				CreateTime:      time.Now().Unix(),
			}
			require.NoError(t, order.Insert())

			_, err := CompleteSubscriptionOrder(tradeNo, `{"amount_total":"999","currency":"CNY"}`, PaymentProviderStripe, PaymentMethodStripe)

			require.ErrorIs(t, err, ErrSubscriptionOrderStatusInvalid)
			orderAfter := GetSubscriptionOrderByTradeNo(tradeNo)
			require.NotNil(t, orderAfter)
			assert.Equal(t, tc.status, orderAfter.Status)
			assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, userID))
			var eventCount int64
			require.NoError(t, DB.Model(&InvitationRewardEvent{}).Where("source_order_id = ?", order.Id).Count(&eventCount).Error)
			assert.Equal(t, int64(0), eventCount)
			topUp := GetTopUpByTradeNo(tradeNo)
			assert.Nil(t, topUp)
		})
	}
}

func TestCompleteSubscriptionOrderRejectsRenewalWhenPurchaseLimitReached(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 404, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 405)
	require.NoError(t, DB.Model(plan).Update("max_purchase_per_user", 1).Error)
	initialEnd := time.Now().Add(24 * time.Hour).Unix()
	existing := &UserSubscription{
		UserId:      404,
		PlanId:      plan.Id,
		Status:      "active",
		StartTime:   time.Now().Add(-time.Hour).Unix(),
		EndTime:     initialEnd,
		AmountTotal: 1000,
		AmountUsed:  123,
		GrantReason: "order",
		Source:      "order",
	}
	require.NoError(t, DB.Create(existing).Error)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-extend-order", 404, plan.Id, PaymentProviderStripe)

	_, err := CompleteSubscriptionOrder("sub-extend-order", `{"provider":"stripe"}`, PaymentProviderStripe, "stripe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已达到该套餐购买上限")

	var sub UserSubscription
	require.NoError(t, DB.First(&sub, existing.Id).Error)
	assert.Equal(t, initialEnd, sub.EndTime)
	assert.Equal(t, int64(123), sub.AmountUsed)
	assert.Equal(t, "order", sub.GrantReason)
	assert.Equal(t, "order", sub.Source)
	assert.Equal(t, int64(1), countUserSubscriptionsForPaymentGuardTest(t, 404))
	order := GetSubscriptionOrderByTradeNo("sub-extend-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}

func TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 303, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 401)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-expire-guard", 303, plan.Id, PaymentProviderStripe)

	err := ExpireSubscriptionOrder("sub-expire-guard", PaymentProviderCreem)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-expire-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}
