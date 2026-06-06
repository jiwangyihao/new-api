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
	"gorm.io/gorm"
)

var errSubscriptionOrderAmountSnapshotMismatch = errors.New("subscription order provider amount or currency mismatch")

const kyrenSubscriptionOrderClaimLeaseSeconds int64 = 5 * 60

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
	AmountTotal any    `json:"amount_total"`
	Amount      any    `json:"amount"`
	Money       any    `json:"money"`
	Currency    string `json:"currency"`
	Object      struct {
		Order struct {
			AmountPaid any    `json:"amount_paid"`
			Amount     any    `json:"amount"`
			Currency   string `json:"currency"`
		} `json:"order"`
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
	completion, err := model.CompleteSubscriptionOrder(order.TradeNo, providerPayload, expectedPaymentProvider, actualPaymentMethod)
	if err != nil {
		return err
	}
	if completion != nil {
		if err := handleInvitationRewardForCompletedSubscriptionOrder(order.Id); err != nil {
			common.SysError("failed to handle invitation reward: " + err.Error())
			return err
		}
	}
	return nil
}

func validateSubscriptionOrderProviderAmountSnapshot(order *model.SubscriptionOrder, providerPayload string) error {
	if order == nil || order.Status != common.TopUpStatusPending || !subscriptionOrderRequiresProviderAmountValidation(order) {
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

func decimalMoneyStringToCents(raw string) (int64, bool) {
	return decimalMoneyValueToCents(raw)
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

func kyrenOrderAmountSnapshotFromPaymentSnapshot(snapshot model.KyrenPaymentSnapshot) (int64, string, error) {
	if normalizeProviderSnapshotCurrency(snapshot.Currency) != kyrenCurrencyCNY {
		return 0, "", nil
	}
	amountCents, ok := decimalMoneyStringToCents(snapshot.Amount)
	if !ok {
		return 0, "", errSubscriptionOrderAmountSnapshotMismatch
	}
	return amountCents, kyrenCurrencyCNY, nil
}

func completeKyrenSubscriptionOrderWithSnapshotAndEvaluateInvitation(tradeNo string, providerPayload string, expectedPaymentProvider string, actualPaymentMethod string) error {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}
	var logUserID int
	var logPlanID int
	var logMoney float64
	var logPaymentMethod string
	var completedOrderID int
	claimed, leaseTime, err := model.ClaimPendingKyrenSubscriptionOrder(tradeNo)
	if err != nil {
		return err
	}
	if !claimed {
		recovered, recoveredLeaseTime, recoverErr := model.RecoverStaleClaimedKyrenSubscriptionOrder(tradeNo, common.GetTimestamp()-kyrenSubscriptionOrderClaimLeaseSeconds)
		if recoverErr != nil {
			return recoverErr
		}
		if recovered {
			claimed = true
			leaseTime = recoveredLeaseTime
		}
	}
	if !claimed {
		order, lookupErr := findKyrenSubscriptionOrderByTradeNo(tradeNo)
		if lookupErr != nil {
			return lookupErr
		}
		if order == nil {
			return model.ErrSubscriptionOrderNotFound
		}
		if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
			return model.ErrPaymentMethodMismatch
		}
		if order.Status == common.TopUpStatusSuccess {
			if err := handleInvitationRewardForCompletedSubscriptionOrder(order.Id); err != nil {
				common.SysError("failed to handle invitation reward: " + err.Error())
				return err
			}
			return nil
		}
		if order.Status == common.TopUpStatusFailed {
			return errKyrenSubscriptionOrderClaimed
		}
		return model.ErrSubscriptionOrderStatusInvalid
	}
	restoreClaimOnFailure := true
	defer func() {
		if err != nil && restoreClaimOnFailure {
			_ = model.RestoreClaimedKyrenSubscriptionOrder(tradeNo, leaseTime)
		}
	}()

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var order model.SubscriptionOrder
		if err := tx.Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
			return err
		}
		if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
			return model.ErrPaymentMethodMismatch
		}
		if order.Status != common.TopUpStatusFailed {
			return model.ErrSubscriptionOrderStatusInvalid
		}
		paymentSnapshot, err := model.UnmarshalKyrenPaymentSnapshot(order.KyrenSnapshot)
		if err != nil {
			return err
		}
		amountCents, currency, err := kyrenOrderAmountSnapshotFromPaymentSnapshot(paymentSnapshot)
		if err != nil {
			return err
		}
		order.AmountCents = amountCents
		order.Currency = currency
		snapshot, err := model.UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
		if err != nil {
			return err
		}
		plan := kyrenSubscriptionPlanFromEntitlementSnapshot(snapshot)
		creation, err := model.CreateUserSubscriptionFromPlanWithResultTx(tx, order.UserId, plan, model.SubscriptionGrantOrder)
		if err != nil {
			return err
		}
		order.CompleteTime = common.GetTimestamp()
		if providerPayload != "" {
			order.ProviderPayload = providerPayload
		}
		if actualPaymentMethod != "" && order.PaymentMethod != actualPaymentMethod {
			order.PaymentMethod = actualPaymentMethod
		}
		if err := upsertKyrenSubscriptionTopUpTx(tx, &order); err != nil {
			return err
		}
		if err := tx.Model(&model.SubscriptionOrder{}).Where("id = ?", order.Id).Updates(map[string]any{"amount_cents": order.AmountCents, "currency": order.Currency}).Error; err != nil {
			return err
		}
		if err := model.MarkClaimedKyrenSubscriptionOrderSuccessTx(tx, &order, leaseTime); err != nil {
			return err
		}
		if _, err := model.RecordInvitationRewardEventForSubscriptionOrderTx(tx, &order, plan, creation, true); err != nil {
			return err
		}
		completedOrderID = order.Id
		logUserID = order.UserId
		logPlanID = order.PlanId
		logMoney = order.Money
		logPaymentMethod = order.PaymentMethod
		return nil
	})
	if err != nil {
		return err
	}
	restoreClaimOnFailure = false
	if completedOrderID > 0 {
		if err := handleInvitationRewardForCompletedSubscriptionOrder(completedOrderID); err != nil {
			common.SysError("failed to handle invitation reward: " + err.Error())
			return err
		}
	}
	if logUserID > 0 {
		if logPlanID > 0 && logMoney > 0 && logPaymentMethod != "" {
			model.RecordLog(logUserID, model.LogTypeTopup, fmt.Sprintf("订阅购买成功，套餐ID: %d，支付金额: %.2f，支付方式: %s", logPlanID, logMoney, logPaymentMethod))
		}
	}
	return nil
}

func kyrenSubscriptionPlanFromEntitlementSnapshot(snapshot model.SubscriptionEntitlementSnapshot) *model.SubscriptionPlan {
	businessCode := strings.TrimSpace(snapshot.BusinessCode)
	plan := &model.SubscriptionPlan{
		Id:                      snapshot.PlanID,
		TotalAmount:             snapshot.TotalAmount,
		MonthlyTokenLimit:       snapshot.MonthlyTokenLimit,
		ConcurrencyLimit:        snapshot.ConcurrencyLimit,
		QueueCapacity:           snapshot.QueueCapacity,
		DurationUnit:            snapshot.DurationUnit,
		DurationValue:           snapshot.DurationValue,
		CustomSeconds:           snapshot.CustomSeconds,
		QuotaResetPeriod:        snapshot.QuotaResetPeriod,
		QuotaResetCustomSeconds: snapshot.QuotaResetCustomSeconds,
		MaxPurchasePerUser:      snapshot.MaxPurchasePerUser,
		IsTrial:                 snapshot.IsTrial,
		InviteTrial:             snapshot.InviteTrial,
		RewardEligible:          snapshot.RewardEligible,
	}
	if businessCode != "" {
		plan.BusinessCode = &businessCode
	}
	return plan
}

func upsertKyrenSubscriptionTopUpTx(tx *gorm.DB, order *model.SubscriptionOrder) error {
	if tx == nil || order == nil {
		return errors.New("invalid subscription order")
	}
	now := common.GetTimestamp()
	var topup model.TopUp
	if err := tx.Where("trade_no = ?", order.TradeNo).First(&topup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			topup = model.TopUp{
				UserId:          order.UserId,
				Amount:          0,
				Money:           order.Money,
				TradeNo:         order.TradeNo,
				PaymentMethod:   order.PaymentMethod,
				PaymentProvider: order.PaymentProvider,
				CreateTime:      order.CreateTime,
				CompleteTime:    now,
				Status:          common.TopUpStatusSuccess,
				KyrenSnapshot:   order.KyrenSnapshot,
			}
			return tx.Create(&topup).Error
		}
		return err
	}
	if topup.PaymentProvider != "" && topup.PaymentProvider != order.PaymentProvider {
		return model.ErrPaymentMethodMismatch
	}
	topup.Money = order.Money
	topup.PaymentMethod = order.PaymentMethod
	topup.PaymentProvider = order.PaymentProvider
	if topup.CreateTime == 0 {
		topup.CreateTime = order.CreateTime
	}
	topup.CompleteTime = now
	topup.Status = common.TopUpStatusSuccess
	if topup.KyrenSnapshot == "" {
		topup.KyrenSnapshot = order.KyrenSnapshot
	}
	return tx.Save(&topup).Error
}
