package controller

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/shopspring/decimal"
)

var errSubscriptionOrderAmountSnapshotMismatch = errors.New("subscription order provider amount or currency mismatch")

type testCleanup interface {
	Cleanup(func())
}

var handleInvitationRewardForCompletedSubscriptionOrder = defaultInvitationRewardOrderHandler

func defaultInvitationRewardOrderHandler(orderId int) error {
	if orderId <= 0 {
		return nil
	}
	return service.HandleInvitationRewardForCompletedSubscriptionOrder(orderId)
}

func SetInvitationRewardOrderHandlerForTest(t testCleanup, handler func(orderId int) error) {
	previous := handleInvitationRewardForCompletedSubscriptionOrder
	handleInvitationRewardForCompletedSubscriptionOrder = handler
	t.Cleanup(func() { handleInvitationRewardForCompletedSubscriptionOrder = previous })
}

type subscriptionProviderAmountPayload struct {
	AmountTotal       any    `json:"amount_total"`
	Amount            any    `json:"amount"`
	Money             any    `json:"money"`
	Currency          string `json:"currency"`
	ProviderProductID string `json:"provider_product_id"`
	Object            struct {
		Order struct {
			AmountPaid any    `json:"amount_paid"`
			Amount     any    `json:"amount"`
			Currency   string `json:"currency"`
		} `json:"order"`
		Product struct {
			ID string `json:"id"`
		} `json:"product"`
	} `json:"object"`
}

func completeSubscriptionOrderAndEvaluateInvitation(tradeNo string, providerPayload string, expectedPaymentProvider string, actualPaymentMethod string) error {
	order := model.GetSubscriptionOrderByTradeNo(strings.TrimSpace(tradeNo))
	if order == nil {
		return model.ErrSubscriptionOrderNotFound
	}
	if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
		return model.ErrPaymentMethodMismatch
	}
	if err := validateSubscriptionOrderProviderAmountSnapshot(order, providerPayload); err != nil {
		return err
	}
	switch order.PaymentProvider {
	case model.PaymentProviderStripe:
		if err := validateSubscriptionOrderProviderProductSnapshot(order, providerPayload); err != nil {
			return err
		}
	case model.PaymentProviderCreem:
		if err := validateSubscriptionOrderProviderProductSnapshot(order, providerPayload); err != nil {
			return err
		}
	}
	completion, err := model.CompleteSubscriptionOrder(order.TradeNo, providerPayload, expectedPaymentProvider, actualPaymentMethod)
	if err != nil {
		return err
	}
	if completion != nil && completion.PurchaseMode == model.SubscriptionPurchaseModeTimed {
		if err := handleInvitationRewardForCompletedSubscriptionOrder(order.Id); err != nil {
			common.SysError("failed to handle invitation reward: " + err.Error())
			return err
		}
	}
	return nil
}

func validateSubscriptionOrderProviderAmountSnapshot(order *model.SubscriptionOrder, providerPayload string) error {
	if order == nil || !subscriptionOrderRequiresProviderAmountValidation(order) {
		return nil
	}
	switch order.PaymentProvider {
	case model.PaymentProviderEpay:
		return validateEpaySubscriptionOrderProviderAmountSnapshot(order, providerPayload)
	case model.PaymentProviderStripe, model.PaymentProviderCreem:
		return validateMinorUnitSubscriptionOrderProviderAmountSnapshot(order, providerPayload)
	default:
		return nil
	}
}

func subscriptionOrderRequiresProviderAmountValidation(order *model.SubscriptionOrder) bool {
	return order != nil && (order.AmountCents > 0 || strings.TrimSpace(order.Currency) != "")
}

func validateEpaySubscriptionOrderProviderAmountSnapshot(order *model.SubscriptionOrder, providerPayload string) error {
	var payload subscriptionProviderAmountPayload
	if strings.TrimSpace(providerPayload) == "" {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	if err := common.UnmarshalJsonStr(providerPayload, &payload); err != nil {
		return err
	}
	amountCents, ok := decimalMoneyValueToCents(payload.Money)
	if !ok {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	if normalizeProviderSnapshotCurrency(order.Currency) != "CNY" || amountCents != order.AmountCents {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	if payloadCurrency := normalizeProviderSnapshotCurrency(payload.Currency); payloadCurrency != "" && payloadCurrency != "CNY" {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	return nil
}

func validateSubscriptionOrderProviderProductSnapshot(order *model.SubscriptionOrder, providerPayload string) error {
	if order == nil || strings.TrimSpace(order.EntitlementSnapshot) == "" {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	snapshot, err := model.UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
	if err != nil {
		return err
	}
	if strings.TrimSpace(snapshot.ProviderProductID) == "" {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	var payload subscriptionProviderAmountPayload
	if err := common.UnmarshalJsonStr(providerPayload, &payload); err != nil {
		return err
	}
	providerProductID := strings.TrimSpace(payload.ProviderProductID)
	if providerProductID == "" {
		providerProductID = strings.TrimSpace(payload.Object.Product.ID)
	}
	if providerProductID != strings.TrimSpace(snapshot.ProviderProductID) {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	return nil
}

func validateMinorUnitSubscriptionOrderProviderAmountSnapshot(order *model.SubscriptionOrder, providerPayload string) error {
	var payload subscriptionProviderAmountPayload
	if strings.TrimSpace(providerPayload) == "" {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	if err := common.UnmarshalJsonStr(providerPayload, &payload); err != nil {
		return err
	}
	amountValue := payload.AmountTotal
	if amountValue == nil {
		amountValue = payload.Amount
	}
	if amountValue == nil {
		amountValue = payload.Object.Order.AmountPaid
	}
	if amountValue == nil {
		amountValue = payload.Object.Order.Amount
	}
	amountCents, ok := decimalProviderValueToMinorUnits(amountValue)
	if !ok {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	currency := normalizeProviderSnapshotCurrency(payload.Currency)
	if currency == "" {
		currency = normalizeProviderSnapshotCurrency(payload.Object.Order.Currency)
	}
	if currency == "" || currency != normalizeProviderSnapshotCurrency(order.Currency) || amountCents != order.AmountCents {
		return errSubscriptionOrderAmountSnapshotMismatch
	}
	return nil
}

func decimalProviderValue(value any) (decimal.Decimal, bool) {
	switch v := value.(type) {
	case nil:
		return decimal.Zero, false
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return decimal.Zero, false
		}
		parsed, err := decimal.NewFromString(trimmed)
		return parsed, err == nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return decimal.Zero, false
		}
		return decimal.NewFromFloat(v), true
	case int:
		return decimal.NewFromInt(int64(v)), true
	case int64:
		return decimal.NewFromInt(v), true
	case int32:
		return decimal.NewFromInt(int64(v)), true
	case uint:
		if uint64(v) > math.MaxInt64 {
			return decimal.Zero, false
		}
		return decimal.NewFromInt(int64(v)), true
	case uint64:
		if v > math.MaxInt64 {
			return decimal.Zero, false
		}
		return decimal.NewFromInt(int64(v)), true
	case uint32:
		return decimal.NewFromInt(int64(v)), true
	default:
		parsed, err := decimal.NewFromString(strings.TrimSpace(fmt.Sprint(value)))
		return parsed, err == nil
	}
}

func decimalProviderValueToMinorUnits(value any) (int64, bool) {
	parsed, ok := decimalProviderValue(value)
	if !ok {
		return 0, false
	}
	return exactNonNegativeInt64(parsed)
}

func decimalMoneyValueToCents(value any) (int64, bool) {
	parsed, ok := decimalProviderValue(value)
	if !ok {
		return 0, false
	}
	return exactNonNegativeInt64(parsed.Mul(decimal.NewFromInt(100)))
}

func exactNonNegativeInt64(value decimal.Decimal) (int64, bool) {
	if !value.IsInteger() || value.LessThan(decimal.Zero) || !value.BigInt().IsInt64() {
		return 0, false
	}
	return value.IntPart(), true
}

func formatCentsAsDecimalString(cents int64) string {
	return decimal.NewFromInt(cents).Div(decimal.NewFromInt(100)).StringFixed(2)
}

func normalizeProviderSnapshotCurrency(currency string) string {
	return strings.ToUpper(strings.TrimSpace(currency))
}

func normalizeProviderCheckoutSnapshot(amountCents int64, currency string) (int64, string) {
	currency = normalizeProviderSnapshotCurrency(currency)
	if amountCents <= 0 || currency == "" {
		return 0, ""
	}
	return amountCents, currency
}
