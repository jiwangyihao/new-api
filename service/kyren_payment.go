package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const kyrenPaymentEventLeaseSeconds int64 = 5 * 60

const (
	KyrenPaymentEventPaid     = "order.paid"
	KyrenPaymentEventFailed   = "order.failed"
	KyrenPaymentEventClosed   = "order.closed"
	KyrenPaymentEventRefunded = "order.refunded"
)

var (
	ErrKyrenPaymentEventConflict   = errors.New("kyren payment event identity or payment snapshot conflict")
	ErrKyrenPaymentEventInProgress = errors.New("kyren payment event is already being processed")
)

type KyrenPaymentEventRequest struct {
	EventID         string
	EventType       string
	PayloadHash     string
	TradeNo         string
	OrderKind       string
	ProviderOrderID string
	ProductID       string
	Amount          string
	Currency        string
	ProviderPayload string
	CallerIP        string
	MetadataUserID  int
	MetadataPlanID  int

	InvitationRewardHandler func(orderID int) error
}

type KyrenPaymentEventResult struct {
	Outcome         model.PaymentProviderEventClaimOutcome
	EventStatus     string
	EventID         string
	EventType       string
	OrderKind       string
	TradeNo         string
	ProviderOrderID string
	LocalOrderID    int
	UserID          int
	PlanID          int
	PurchaseMode    string
	Transitioned    bool
	CreditedCents   int64
	PaidMoney       float64

	NeedsInvitationPostProcessing bool
	NeedsManualAction             bool
	ManualActionReason            string
}

func CreateKyrenSubscriptionPaymentOrder(order *model.SubscriptionOrder) error {
	if order == nil || order.UserId <= 0 || order.PlanId <= 0 || strings.TrimSpace(order.TradeNo) == "" || order.PaymentProvider != model.PaymentProviderKyren || order.Status != common.TopUpStatusPending {
		return errors.New("invalid Kyren subscription payment order")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		return model.CreatePaymentProviderOrderTx(tx, &model.PaymentProviderOrder{
			Provider: model.PaymentProviderKyren, OrderKind: model.PaymentOrderKindSubscription,
			LocalOrderID: order.Id, TradeNo: order.TradeNo, UserID: order.UserId, PlanID: order.PlanId,
		})
	})
}

func CreateKyrenTopUpPaymentOrder(order *model.TopUp) error {
	if order == nil || order.UserId <= 0 || strings.TrimSpace(order.TradeNo) == "" || order.PaymentProvider != model.PaymentProviderKyren || order.Status != common.TopUpStatusPending {
		return errors.New("invalid Kyren top-up payment order")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		return model.CreatePaymentProviderOrderTx(tx, &model.PaymentProviderOrder{
			Provider: model.PaymentProviderKyren, OrderKind: model.PaymentOrderKindTopUp,
			LocalOrderID: order.Id, TradeNo: order.TradeNo, UserID: order.UserId,
		})
	})
}

func BindKyrenPaymentCheckout(tradeNo string, checkoutID string) error {
	return model.BindPaymentProviderCheckoutID(model.PaymentProviderKyren, tradeNo, checkoutID)
}

func ProcessKyrenPaymentEvent(request KyrenPaymentEventRequest) (*KyrenPaymentEventResult, error) {
	normalizeKyrenPaymentEventRequest(&request)
	if request.EventID == "" || request.EventType == "" || len(request.PayloadHash) != 64 {
		return nil, errors.New("invalid Kyren payment event")
	}
	result := &KyrenPaymentEventResult{
		EventID: request.EventID, EventType: request.EventType, OrderKind: request.OrderKind,
		TradeNo: request.TradeNo, ProviderOrderID: request.ProviderOrderID,
	}
	var claimedEvent *model.PaymentProviderEvent
	deferredSubscriptionRefund := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		event, outcome, err := model.ClaimPaymentProviderEventTx(tx, model.PaymentProviderEventClaimRequest{
			Provider: model.PaymentProviderKyren, EventID: request.EventID, EventType: request.EventType,
			PayloadHash: request.PayloadHash, TradeNo: request.TradeNo, OrderKind: request.OrderKind,
			ProviderOrderID: request.ProviderOrderID, StaleBefore: common.GetTimestamp() - kyrenPaymentEventLeaseSeconds,
		})
		if err != nil {
			return err
		}
		claimedEvent = event
		result.Outcome = outcome
		result.EventStatus = event.Status
		populateKyrenPaymentEventResultTx(tx, event, result)
		switch outcome {
		case model.PaymentProviderEventDuplicate:
			return nil
		case model.PaymentProviderEventConflicted:
			result.NeedsManualAction = true
			result.ManualActionReason = "event ID was replayed with conflicting identity"
			return nil
		case model.PaymentProviderEventInProgress:
			return ErrKyrenPaymentEventInProgress
		case model.PaymentProviderEventClaimed:
		default:
			return fmt.Errorf("invalid Kyren payment event claim outcome: %s", outcome)
		}

		if request.TradeNo == "" || (request.OrderKind != model.PaymentOrderKindSubscription && request.OrderKind != model.PaymentOrderKindTopUp) {
			result.NeedsManualAction = true
			result.ManualActionReason = fmt.Sprintf("missing or unsupported local order identity: kind=%s", request.OrderKind)
			return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventIgnored, result.ManualActionReason, "")
		}
		mapping, err := model.EnsurePaymentProviderOrderTx(tx, model.PaymentProviderKyren, request.OrderKind, request.TradeNo)
		if err != nil {
			if errors.Is(err, model.ErrPaymentProviderOrderNotFound) || errors.Is(err, model.ErrPaymentProviderOrderConflict) {
				return conflictKyrenPaymentEventTx(tx, event, result, "local payment order identity not found or conflicted")
			}
			return err
		}
		if request.ProviderOrderID == "" {
			return conflictKyrenPaymentEventTx(tx, event, result, "provider order ID is missing")
		}
		if err := model.BindPaymentProviderOrderIDTx(tx, mapping, request.ProviderOrderID); err != nil {
			if errors.Is(err, model.ErrPaymentProviderOrderConflict) {
				return conflictKyrenPaymentEventTx(tx, event, result, "provider order ID conflicts with local mapping")
			}
			return err
		}
		if request.MetadataUserID > 0 && request.MetadataUserID != mapping.UserID {
			return conflictKyrenPaymentEventTx(tx, event, result, "metadata user ID conflicts with local mapping")
		}
		if request.OrderKind == model.PaymentOrderKindSubscription && request.MetadataPlanID > 0 && request.MetadataPlanID != mapping.PlanID {
			return conflictKyrenPaymentEventTx(tx, event, result, "metadata plan ID conflicts with local mapping")
		}
		if err := model.BindPaymentProviderEventOrderTx(tx, event, mapping); err != nil {
			return err
		}
		result.LocalOrderID = mapping.LocalOrderID
		result.UserID = mapping.UserID
		result.PlanID = mapping.PlanID

		switch request.EventType {
		case KyrenPaymentEventPaid:
			return processKyrenPaidEventTx(tx, event, mapping, request, result)
		case KyrenPaymentEventFailed:
			return processKyrenTerminalEventTx(tx, event, mapping, common.TopUpStatusFailed, request, result)
		case KyrenPaymentEventClosed:
			return processKyrenTerminalEventTx(tx, event, mapping, common.TopUpStatusExpired, request, result)
		case KyrenPaymentEventRefunded:
			if request.OrderKind == model.PaymentOrderKindTopUp {
				result.NeedsManualAction = true
				result.ManualActionReason = "top-up refund requires manual balance recovery"
				return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventIgnored, result.ManualActionReason, "")
			}
			deferredSubscriptionRefund = true
			return nil
		default:
			return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventIgnored, "unsupported event type", "")
		}
	})
	if err != nil {
		return nil, err
	}
	if deferredSubscriptionRefund {
		if err := finishKyrenSubscriptionRefund(claimedEvent, request, result); err != nil {
			return nil, err
		}
	}
	if err := postProcessKyrenPaymentEvent(request, result); err != nil {
		return result, err
	}
	return result, nil
}

func normalizeKyrenPaymentEventRequest(request *KyrenPaymentEventRequest) {
	if request == nil {
		return
	}
	request.EventID = strings.TrimSpace(request.EventID)
	request.EventType = strings.TrimSpace(request.EventType)
	request.PayloadHash = strings.ToLower(strings.TrimSpace(request.PayloadHash))
	request.TradeNo = strings.TrimSpace(request.TradeNo)
	request.OrderKind = strings.TrimSpace(request.OrderKind)
	request.ProviderOrderID = strings.TrimSpace(request.ProviderOrderID)
	request.ProductID = strings.TrimSpace(request.ProductID)
	request.Amount = strings.TrimSpace(request.Amount)
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	request.ProviderPayload = strings.TrimSpace(request.ProviderPayload)
	request.CallerIP = strings.TrimSpace(request.CallerIP)
}

func populateKyrenPaymentEventResultTx(tx *gorm.DB, event *model.PaymentProviderEvent, result *KyrenPaymentEventResult) {
	if tx == nil || event == nil || result == nil {
		return
	}
	result.EventStatus = event.Status
	result.LocalOrderID = event.LocalOrderID
	result.UserID = event.UserID
	result.PurchaseMode = event.ResultPurchaseMode
	if event.LocalOrderID <= 0 {
		return
	}
	switch event.OrderKind {
	case model.PaymentOrderKindSubscription:
		var order model.SubscriptionOrder
		if tx.Select("id", "user_id", "plan_id", "money").First(&order, event.LocalOrderID).Error == nil {
			result.UserID = order.UserId
			result.PlanID = order.PlanId
			result.PaidMoney = order.Money
		}
		result.NeedsInvitationPostProcessing = event.EventType == KyrenPaymentEventPaid && event.Status == model.PaymentProviderEventApplied && event.ResultPurchaseMode == model.SubscriptionPurchaseModeTimed
	case model.PaymentOrderKindTopUp:
		var topUp model.TopUp
		if tx.Select("id", "user_id", "money").First(&topUp, event.LocalOrderID).Error == nil {
			result.UserID = topUp.UserId
			result.PaidMoney = topUp.Money
		}
	}
}

func finishKyrenPaymentEventTx(tx *gorm.DB, event *model.PaymentProviderEvent, result *KyrenPaymentEventResult, status string, reason string, purchaseMode string) error {
	if err := model.FinishPaymentProviderEventTx(tx, event, status, reason, purchaseMode); err != nil {
		return err
	}
	result.EventStatus = status
	result.PurchaseMode = strings.TrimSpace(purchaseMode)
	return nil
}

func conflictKyrenPaymentEventTx(tx *gorm.DB, event *model.PaymentProviderEvent, result *KyrenPaymentEventResult, reason string) error {
	result.NeedsManualAction = true
	result.ManualActionReason = strings.TrimSpace(reason)
	return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventConflict, result.ManualActionReason, "")
}

func processKyrenPaidEventTx(tx *gorm.DB, event *model.PaymentProviderEvent, mapping *model.PaymentProviderOrder, request KyrenPaymentEventRequest, result *KyrenPaymentEventResult) error {
	switch mapping.OrderKind {
	case model.PaymentOrderKindSubscription:
		return processKyrenPaidSubscriptionTx(tx, event, mapping, request, result)
	case model.PaymentOrderKindTopUp:
		return processKyrenPaidTopUpTx(tx, event, mapping, request, result)
	default:
		return conflictKyrenPaymentEventTx(tx, event, result, "unsupported local order kind")
	}
}

func processKyrenPaidSubscriptionTx(tx *gorm.DB, event *model.PaymentProviderEvent, mapping *model.PaymentProviderOrder, request KyrenPaymentEventRequest, result *KyrenPaymentEventResult) error {
	var order model.SubscriptionOrder
	if err := model.LockForUpdate(tx).Where("id = ?", mapping.LocalOrderID).First(&order).Error; err != nil {
		return err
	}
	if order.TradeNo != mapping.TradeNo || order.UserId != mapping.UserID || order.PlanId != mapping.PlanID || order.PaymentProvider != model.PaymentProviderKyren {
		return conflictKyrenPaymentEventTx(tx, event, result, "subscription order conflicts with provider mapping")
	}
	snapshot, err := model.UnmarshalKyrenPaymentSnapshot(order.KyrenSnapshot)
	if err != nil || !kyrenPaymentSnapshotMatches(snapshot.ProductID, snapshot.Amount, snapshot.Currency, request) {
		return conflictKyrenPaymentEventTx(tx, event, result, "subscription payment snapshot does not match provider event")
	}
	amountCents, ok := decimalMoneyStringToCents(snapshot.Amount)
	currency := strings.ToUpper(strings.TrimSpace(snapshot.Currency))
	if !ok || amountCents <= 0 || currency == "" {
		return conflictKyrenPaymentEventTx(tx, event, result, "subscription payment snapshot is invalid")
	}
	if (order.AmountCents > 0 && order.AmountCents != amountCents) || (strings.TrimSpace(order.Currency) != "" && !strings.EqualFold(order.Currency, currency)) {
		return conflictKyrenPaymentEventTx(tx, event, result, "subscription amount identity conflicts with checkout snapshot")
	}
	if order.Status != common.TopUpStatusPending && order.Status != common.TopUpStatusSuccess {
		return conflictKyrenPaymentEventTx(tx, event, result, "paid event arrived after a different local terminal status")
	}
	storeAmountCents := int64(0)
	storeCurrency := ""
	if currency == "CNY" {
		storeAmountCents = amountCents
		storeCurrency = currency
	}
	if order.Status == common.TopUpStatusPending && (order.AmountCents != storeAmountCents || order.Currency != storeCurrency) {
		update := tx.Model(&model.SubscriptionOrder{}).
			Where("id = ? AND status = ?", order.Id, common.TopUpStatusPending).
			Updates(map[string]any{"amount_cents": storeAmountCents, "currency": storeCurrency})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return conflictKyrenPaymentEventTx(tx, event, result, "subscription order changed while applying payment snapshot")
		}
	}
	order.AmountCents = storeAmountCents
	order.Currency = storeCurrency
	completion, err := model.CompleteSubscriptionOrderTx(tx, &order, request.ProviderPayload, model.PaymentMethodKyren)
	if err != nil {
		if errors.Is(err, model.ErrSubscriptionOrderSnapshotMismatch) || errors.Is(err, model.ErrSubscriptionOrderStatusInvalid) || errors.Is(err, model.ErrPaymentMethodMismatch) {
			return conflictKyrenPaymentEventTx(tx, event, result, "subscription fulfillment identity validation failed")
		}
		return err
	}
	if completion == nil {
		return errors.New("Kyren subscription completion returned no result")
	}
	result.LocalOrderID = order.Id
	result.UserID = order.UserId
	result.PlanID = order.PlanId
	result.PurchaseMode = completion.PurchaseMode
	result.Transitioned = completion.Transitioned
	result.PaidMoney = order.Money
	result.NeedsInvitationPostProcessing = completion.PurchaseMode == model.SubscriptionPurchaseModeTimed
	return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventApplied, "subscription payment applied", completion.PurchaseMode)
}

func processKyrenPaidTopUpTx(tx *gorm.DB, event *model.PaymentProviderEvent, mapping *model.PaymentProviderOrder, request KyrenPaymentEventRequest, result *KyrenPaymentEventResult) error {
	var topUp model.TopUp
	if err := model.LockForUpdate(tx).Where("id = ?", mapping.LocalOrderID).First(&topUp).Error; err != nil {
		return err
	}
	if topUp.TradeNo != mapping.TradeNo || topUp.UserId != mapping.UserID || topUp.PaymentProvider != model.PaymentProviderKyren {
		return conflictKyrenPaymentEventTx(tx, event, result, "top-up order conflicts with provider mapping")
	}
	snapshot, err := model.UnmarshalKyrenTopUpSnapshot(topUp.KyrenSnapshot)
	if err != nil || !kyrenPaymentSnapshotMatches(snapshot.ProductID, snapshot.Amount, snapshot.Currency, request) {
		return conflictKyrenPaymentEventTx(tx, event, result, "top-up payment snapshot does not match provider event")
	}
	maxInt := int64(^uint(0) >> 1)
	if snapshot.Quota <= 0 || snapshot.Quota > maxInt {
		return conflictKyrenPaymentEventTx(tx, event, result, "top-up credit snapshot is invalid")
	}
	if topUp.Status == common.TopUpStatusSuccess {
		result.LocalOrderID = topUp.Id
		result.UserID = topUp.UserId
		result.PaidMoney = topUp.Money
		return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventApplied, "top-up payment was already applied", "")
	}
	if topUp.Status != common.TopUpStatusPending {
		return conflictKyrenPaymentEventTx(tx, event, result, "paid event arrived after a different local terminal status")
	}
	completeTime := common.GetTimestamp()
	claim := tx.Model(&model.TopUp{}).
		Where("id = ? AND payment_provider = ? AND status = ?", topUp.Id, model.PaymentProviderKyren, common.TopUpStatusPending).
		Updates(map[string]any{
			"amount": snapshot.Quota, "amount_unit": model.TopUpAmountUnitAccountBalanceCents,
			"status": common.TopUpStatusSuccess, "complete_time": completeTime,
		})
	if claim.Error != nil {
		return claim.Error
	}
	if claim.RowsAffected == 0 {
		return errors.New("Kyren top-up changed while applying payment")
	}
	if err := model.IncreaseUserAccountBalanceTx(tx, topUp.UserId, int(snapshot.Quota)); err != nil {
		return err
	}
	result.LocalOrderID = topUp.Id
	result.UserID = topUp.UserId
	result.Transitioned = true
	result.CreditedCents = snapshot.Quota
	result.PaidMoney = topUp.Money
	return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventApplied, "top-up payment applied", "")
}

func kyrenPaymentSnapshotMatches(productID string, amount string, currency string, request KyrenPaymentEventRequest) bool {
	if strings.TrimSpace(productID) != request.ProductID || !strings.EqualFold(strings.TrimSpace(currency), request.Currency) {
		return false
	}
	expected, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil {
		return false
	}
	actual, err := decimal.NewFromString(request.Amount)
	return err == nil && expected.Equal(actual)
}

func decimalMoneyStringToCents(raw string) (int64, bool) {
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || amount.IsNegative() {
		return 0, false
	}
	cents := amount.Mul(decimal.NewFromInt(100))
	if !cents.IsInteger() || !cents.BigInt().IsInt64() {
		return 0, false
	}
	return cents.IntPart(), true
}

func processKyrenTerminalEventTx(tx *gorm.DB, event *model.PaymentProviderEvent, mapping *model.PaymentProviderOrder, targetStatus string, request KyrenPaymentEventRequest, result *KyrenPaymentEventResult) error {
	if targetStatus != common.TopUpStatusFailed && targetStatus != common.TopUpStatusExpired {
		return errors.New("invalid Kyren payment terminal status")
	}
	now := common.GetTimestamp()
	switch mapping.OrderKind {
	case model.PaymentOrderKindSubscription:
		var order model.SubscriptionOrder
		if err := model.LockForUpdate(tx).Where("id = ?", mapping.LocalOrderID).First(&order).Error; err != nil {
			return err
		}
		if order.TradeNo != mapping.TradeNo || order.UserId != mapping.UserID || order.PlanId != mapping.PlanID || order.PaymentProvider != model.PaymentProviderKyren {
			return conflictKyrenPaymentEventTx(tx, event, result, "subscription order conflicts with provider mapping")
		}
		result.LocalOrderID, result.UserID, result.PlanID, result.PaidMoney = order.Id, order.UserId, order.PlanId, order.Money
		if order.Status == common.TopUpStatusPending {
			claim := tx.Model(&model.SubscriptionOrder{}).
				Where("id = ? AND payment_provider = ? AND status = ?", order.Id, model.PaymentProviderKyren, common.TopUpStatusPending).
				Updates(map[string]any{"status": targetStatus, "complete_time": now})
			if claim.Error != nil {
				return claim.Error
			}
			if claim.RowsAffected == 0 {
				return errors.New("Kyren subscription order changed while applying terminal event")
			}
			result.Transitioned = true
		} else if order.Status != targetStatus {
			return conflictKyrenPaymentEventTx(tx, event, result, "terminal event conflicts with existing subscription status")
		}
	case model.PaymentOrderKindTopUp:
		var topUp model.TopUp
		if err := model.LockForUpdate(tx).Where("id = ?", mapping.LocalOrderID).First(&topUp).Error; err != nil {
			return err
		}
		if topUp.TradeNo != mapping.TradeNo || topUp.UserId != mapping.UserID || topUp.PaymentProvider != model.PaymentProviderKyren {
			return conflictKyrenPaymentEventTx(tx, event, result, "top-up order conflicts with provider mapping")
		}
		result.LocalOrderID, result.UserID, result.PaidMoney = topUp.Id, topUp.UserId, topUp.Money
		if topUp.Status == common.TopUpStatusPending {
			claim := tx.Model(&model.TopUp{}).
				Where("id = ? AND payment_provider = ? AND status = ?", topUp.Id, model.PaymentProviderKyren, common.TopUpStatusPending).
				Updates(map[string]any{"status": targetStatus, "complete_time": now})
			if claim.Error != nil {
				return claim.Error
			}
			if claim.RowsAffected == 0 {
				return errors.New("Kyren top-up changed while applying terminal event")
			}
			result.Transitioned = true
		} else if topUp.Status != targetStatus {
			return conflictKyrenPaymentEventTx(tx, event, result, "terminal event conflicts with existing top-up status")
		}
	default:
		return conflictKyrenPaymentEventTx(tx, event, result, "unsupported local order kind")
	}
	reason := "provider order failed"
	if request.EventType == KyrenPaymentEventClosed {
		reason = "provider order closed"
	}
	return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventApplied, reason, "")
}

func finishKyrenSubscriptionRefund(event *model.PaymentProviderEvent, request KyrenPaymentEventRequest, result *KyrenPaymentEventResult) error {
	if event == nil || event.ProcessingToken == "" {
		return errors.New("invalid Kyren refund event claim")
	}
	_, err := RecoverSubscriptionOrder(SubscriptionOrderRecoveryRequest{
		TradeNo: request.TradeNo, ExpectedPaymentProvider: model.PaymentProviderKyren,
		RecoveryType: SubscriptionOrderRecoveryRefund, ProviderPayload: request.ProviderPayload,
		Reason: "Kyren provider refund", CreditRecoveryOnly: true,
	})
	if err != nil {
		if errors.Is(err, model.ErrSubscriptionOrderCreditRecoveryNotApplicable) {
			result.NeedsManualAction = true
			result.ManualActionReason = "subscription refund requires manual entitlement recovery"
			return model.DB.Transaction(func(tx *gorm.DB) error {
				return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventIgnored, result.ManualActionReason, "")
			})
		}
		return err
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		result.Transitioned = true
		return finishKyrenPaymentEventTx(tx, event, result, model.PaymentProviderEventApplied, "subscription refund recovered", "")
	})
}

func postProcessKyrenPaymentEvent(request KyrenPaymentEventRequest, result *KyrenPaymentEventResult) error {
	if result == nil || result.EventStatus != model.PaymentProviderEventApplied || result.Outcome == model.PaymentProviderEventConflicted {
		return nil
	}
	if result.UserID > 0 {
		if err := model.InvalidateUserCache(result.UserID); err != nil {
			common.SysLog(fmt.Sprintf("failed to invalidate user cache after Kyren event user_id=%d trade_no=%s: %s", result.UserID, result.TradeNo, err.Error()))
		}
	}
	if result.NeedsInvitationPostProcessing && request.InvitationRewardHandler != nil && result.LocalOrderID > 0 {
		if err := request.InvitationRewardHandler(result.LocalOrderID); err != nil {
			return err
		}
	}
	if !result.Transitioned || result.UserID <= 0 {
		return nil
	}
	switch result.OrderKind {
	case model.PaymentOrderKindSubscription:
		if result.EventType == KyrenPaymentEventPaid && result.PlanID > 0 && result.PaidMoney > 0 {
			model.RecordLog(result.UserID, model.LogTypeTopup, fmt.Sprintf("订阅购买成功，套餐ID: %d，支付金额: %.2f，支付方式: %s", result.PlanID, result.PaidMoney, model.PaymentMethodKyren))
		}
	case model.PaymentOrderKindTopUp:
		if result.EventType == KyrenPaymentEventPaid && result.CreditedCents > 0 {
			balanceCNY := model.AccountBalanceCNYFromCents(int(result.CreditedCents)).StringFixed(2)
			model.RecordTopupLog(result.UserID, fmt.Sprintf("Kyren充值成功，充值额度: %s，支付金额: %.2f", balanceCNY, result.PaidMoney), request.CallerIP, model.PaymentMethodKyren, model.PaymentMethodKyren)
		}
	}
	return nil
}
