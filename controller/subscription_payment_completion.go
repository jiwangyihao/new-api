package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

func completeSubscriptionOrderAndEvaluateInvitation(tradeNo string, providerPayload string, expectedPaymentProvider string, actualPaymentMethod string) error {
	order := model.GetSubscriptionOrderByTradeNo(strings.TrimSpace(tradeNo))
	if order == nil {
		return model.ErrSubscriptionOrderNotFound
	}
	if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
		return model.ErrPaymentMethodMismatch
	}
	if err := model.CompleteSubscriptionOrder(order.TradeNo, providerPayload, expectedPaymentProvider, actualPaymentMethod); err != nil {
		return err
	}
	service.TryEnsureInvitationEntitlementForPaidUser(order.UserId)
	return nil
}
