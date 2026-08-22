package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/shopspring/decimal"
)

const (
	kyrenReconciliationSource                     = "kyren_reconciliation"
	kyrenReconciliationMinIntervalSeconds         = int64(15)
	kyrenReconciliationTaskDefaultBatchSize       = 50
	kyrenReconciliationTaskDefaultIntervalSeconds = 300
)

var kyrenReconciliationTaskOnce sync.Once

type kyrenReconciliationSweepResult struct {
	Scanned int
	Failed  int
}

type kyrenReconciliationFact struct {
	Source     string            `json:"source"`
	EventType  string            `json:"event_type"`
	CheckoutID string            `json:"checkout_id"`
	OrderID    string            `json:"order_id,omitempty"`
	ProductID  string            `json:"product_id"`
	Amount     string            `json:"amount"`
	Currency   string            `json:"currency"`
	Status     string            `json:"status"`
	UpdatedAt  int64             `json:"updated_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func reconcilePendingKyrenSubscriptionOrder(ctx context.Context, localOrder *model.SubscriptionOrder, callerIP string) (string, error) {
	if localOrder == nil || localOrder.Id <= 0 || localOrder.Status != common.TopUpStatusPending || localOrder.PaymentProvider != model.PaymentProviderKyren {
		return "", nil
	}
	mapping, claimed, err := model.ClaimPaymentProviderReconciliation(
		model.PaymentProviderKyren,
		model.PaymentOrderKindSubscription,
		localOrder.TradeNo,
		kyrenReconciliationMinIntervalSeconds,
	)
	if err != nil || mapping == nil {
		return "", err
	}
	if mapping.LocalOrderID != localOrder.Id || mapping.UserID != localOrder.UserId || mapping.PlanID != localOrder.PlanId || mapping.TradeNo != localOrder.TradeNo {
		return "", fmt.Errorf("Kyren reconciliation local order identity mismatch")
	}
	if mapping.ProviderCheckoutID == nil || strings.TrimSpace(*mapping.ProviderCheckoutID) == "" {
		return "", nil
	}
	if !claimed {
		if mapping.ProviderCheckoutURL != nil && safeKyrenCheckoutURL(*mapping.ProviderCheckoutURL) {
			return strings.TrimSpace(*mapping.ProviderCheckoutURL), nil
		}
		return "", nil
	}
	checkoutID := strings.TrimSpace(*mapping.ProviderCheckoutID)
	paymentSnapshot, err := model.UnmarshalKyrenPaymentSnapshot(localOrder.KyrenSnapshot)
	if err != nil {
		return "", fmt.Errorf("decode Kyren payment snapshot: %w", err)
	}
	if err := validateKyrenReconciliationPaymentSnapshot(localOrder, paymentSnapshot); err != nil {
		return "", err
	}
	client, err := newKyrenClientForController()
	if err != nil {
		return "", err
	}
	checkout, err := client.retrieveCheckout(ctx, checkoutID)
	if err != nil {
		return "", err
	}
	if err := validateKyrenReconciliationCheckout(checkout, checkoutID, paymentSnapshot); err != nil {
		return "", err
	}

	providerOrderID := ""
	if mapping.ProviderOrderID != nil {
		providerOrderID = strings.TrimSpace(*mapping.ProviderOrderID)
	}
	checkoutOrderID := strings.TrimSpace(checkout.OrderID)
	if providerOrderID != "" && checkoutOrderID != "" && providerOrderID != checkoutOrderID {
		return "", fmt.Errorf("Kyren reconciliation checkout order identity mismatch")
	}
	if checkoutOrderID != "" {
		providerOrderID = checkoutOrderID
	}

	checkoutStatus := strings.ToUpper(strings.TrimSpace(checkout.Status))
	if providerOrderID == "" {
		hiddenOrder, err := findPendingKyrenProviderOrder(ctx, client, localOrder, mapping, checkout, paymentSnapshot)
		if err != nil {
			return "", err
		}
		if hiddenOrder != nil {
			providerOrderID = strings.TrimSpace(hiddenOrder.ID)
			if err := model.BindPaymentProviderOrderID(model.PaymentProviderKyren, model.PaymentOrderKindSubscription, localOrder.TradeNo, providerOrderID); err != nil {
				return "", err
			}
			mapping.ProviderOrderID = &providerOrderID
		} else {
			if reusableKyrenSubscriptionCheckout(checkout, checkoutID, paymentSnapshot.ProductID, paymentSnapshot.Amount, paymentSnapshot.Currency) {
				if err := service.BindKyrenPaymentCheckoutURL(localOrder.TradeNo, checkoutID, checkout.URL); err != nil {
					return "", err
				}
				return strings.TrimSpace(checkout.URL), nil
			}
			if checkoutStatus != "EXPIRED" {
				return "", nil
			}
			fact := kyrenReconciliationFact{
				Source: kyrenReconciliationSource, EventType: service.KyrenPaymentEventClosed,
				CheckoutID: checkoutID, ProductID: strings.TrimSpace(checkout.ProductID),
				Amount: strings.TrimSpace(checkout.Amount), Currency: strings.ToUpper(strings.TrimSpace(checkout.Currency)),
				Status: checkoutStatus, UpdatedAt: checkout.ExpiresAt,
			}
			return "", applyKyrenReconciliationFact(localOrder, mapping, fact, callerIP, true, "")
		}
	}

	providerOrder, err := client.retrieveOrder(ctx, providerOrderID)
	if err != nil {
		return "", err
	}
	if err := validateKyrenReconciliationOrder(providerOrder, providerOrderID, checkoutID, paymentSnapshot, mapping); err != nil {
		return "", err
	}
	eventType, manualReason, err := kyrenReconciliationEventForOrderStatus(providerOrder.Status)
	if err != nil {
		return "", err
	}
	if eventType == "" && checkoutStatus == "EXPIRED" {
		eventType = service.KyrenPaymentEventClosed
	}
	if eventType == "" {
		return "", nil
	}
	updatedAt := providerOrder.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = providerOrder.PaidAt
	}
	if updatedAt <= 0 {
		updatedAt = providerOrder.SettledAt
	}
	if updatedAt <= 0 {
		updatedAt = providerOrder.CreatedAt
	}
	factStatus := strings.ToUpper(strings.TrimSpace(providerOrder.Status))
	if eventType == service.KyrenPaymentEventClosed && factStatus == "PENDING" {
		factStatus = checkoutStatus
		updatedAt = checkout.ExpiresAt
	}
	fact := kyrenReconciliationFact{
		Source: kyrenReconciliationSource, EventType: eventType,
		CheckoutID: checkoutID, OrderID: providerOrderID,
		ProductID: strings.TrimSpace(providerOrder.ProductID), Amount: strings.TrimSpace(providerOrder.Amount),
		Currency: strings.ToUpper(strings.TrimSpace(providerOrder.Currency)), Status: factStatus,
		UpdatedAt: updatedAt, Metadata: providerOrder.Metadata,
	}
	return "", applyKyrenReconciliationFact(localOrder, mapping, fact, callerIP, false, manualReason)
}

func findPendingKyrenProviderOrder(ctx context.Context, client kyrenAPI, localOrder *model.SubscriptionOrder, mapping *model.PaymentProviderOrder, checkout *kyrenCheckoutSession, snapshot model.KyrenPaymentSnapshot) (*kyrenOrder, error) {
	if client == nil || localOrder == nil || mapping == nil || checkout == nil {
		return nil, fmt.Errorf("invalid Kyren provider order discovery input")
	}
	checkoutID := strings.TrimSpace(checkout.ID)
	if checkoutID == "" {
		return nil, fmt.Errorf("Kyren provider order discovery checkout is missing")
	}
	var match *kyrenOrder
	for _, status := range []string{"PENDING", "CREATING"} {
		for pageNumber := 1; pageNumber <= 100; pageNumber++ {
			page, err := client.listOrders(ctx, status, snapshot.ProductID, pageNumber, 100)
			if err != nil {
				return nil, err
			}
			if page == nil {
				return nil, fmt.Errorf("Kyren provider order discovery returned no page")
			}
			for index := range page.Items {
				candidate := &page.Items[index]
				if !kyrenProviderOrderMatchesLocal(candidate, checkoutID, localOrder, mapping, snapshot) {
					continue
				}
				if match != nil && match.ID != candidate.ID {
					return nil, fmt.Errorf("Kyren provider order discovery is ambiguous")
				}
				copy := *candidate
				match = &copy
			}
			if page.Pagination.TotalPages > 0 {
				if pageNumber >= page.Pagination.TotalPages {
					break
				}
			} else if len(page.Items) < 100 {
				break
			}
			if pageNumber == 100 {
				return nil, fmt.Errorf("Kyren provider order discovery exceeded page limit")
			}
		}
	}
	return match, nil
}

func kyrenProviderOrderMatchesLocal(providerOrder *kyrenOrder, checkoutID string, localOrder *model.SubscriptionOrder, mapping *model.PaymentProviderOrder, snapshot model.KyrenPaymentSnapshot) bool {
	if providerOrder == nil || localOrder == nil || mapping == nil || strings.TrimSpace(providerOrder.ID) == "" || strings.TrimSpace(providerOrder.CheckoutSessionID) != checkoutID {
		return false
	}
	if strings.TrimSpace(providerOrder.ProductID) != strings.TrimSpace(snapshot.ProductID) || !kyrenReconciliationAmountEqual(providerOrder.Amount, snapshot.Amount) || !strings.EqualFold(strings.TrimSpace(providerOrder.Currency), strings.TrimSpace(snapshot.Currency)) {
		return false
	}
	metadata := providerOrder.Metadata
	return metadata != nil && metadata["kind"] == model.PaymentOrderKindSubscription && metadata["trade_no"] == mapping.TradeNo && metadata["user_id"] == strconv.Itoa(localOrder.UserId) && metadata["plan_id"] == strconv.Itoa(localOrder.PlanId)
}

func validateKyrenReconciliationPaymentSnapshot(localOrder *model.SubscriptionOrder, snapshot model.KyrenPaymentSnapshot) error {
	if localOrder == nil || strings.TrimSpace(snapshot.ProductID) == "" {
		return fmt.Errorf("Kyren reconciliation payment snapshot is incomplete")
	}
	amount, err := normalizeKyrenAmountString(snapshot.Amount)
	if err != nil {
		return fmt.Errorf("Kyren reconciliation payment amount is invalid: %w", err)
	}
	currency := strings.ToUpper(strings.TrimSpace(snapshot.Currency))
	if currency == "" {
		return fmt.Errorf("Kyren reconciliation payment currency is missing")
	}
	parsed, err := decimal.NewFromString(amount)
	if err != nil {
		return fmt.Errorf("Kyren reconciliation payment amount is invalid: %w", err)
	}
	cents := parsed.Mul(decimal.NewFromInt(100))
	if !cents.IsInteger() || !cents.BigInt().IsInt64() || cents.IntPart() <= 0 {
		return fmt.Errorf("Kyren reconciliation payment amount is invalid")
	}
	if localOrder.AmountCents > 0 && localOrder.AmountCents != cents.IntPart() {
		return fmt.Errorf("Kyren reconciliation local amount mismatch")
	}
	if strings.TrimSpace(localOrder.Currency) != "" && !strings.EqualFold(localOrder.Currency, currency) {
		return fmt.Errorf("Kyren reconciliation local currency mismatch")
	}
	return nil
}

func validateKyrenReconciliationCheckout(checkout *kyrenCheckoutSession, checkoutID string, snapshot model.KyrenPaymentSnapshot) error {
	if checkout == nil || strings.TrimSpace(checkout.ID) != checkoutID {
		return fmt.Errorf("Kyren reconciliation checkout identity mismatch")
	}
	status := strings.ToUpper(strings.TrimSpace(checkout.Status))
	switch status {
	case "OPEN", "COMPLETE", "EXPIRED":
	default:
		return fmt.Errorf("Kyren reconciliation checkout status is unsupported: %s", status)
	}
	if strings.TrimSpace(checkout.ProductID) != strings.TrimSpace(snapshot.ProductID) || !kyrenReconciliationAmountEqual(checkout.Amount, snapshot.Amount) || !strings.EqualFold(strings.TrimSpace(checkout.Currency), strings.TrimSpace(snapshot.Currency)) {
		return fmt.Errorf("Kyren reconciliation checkout payment identity mismatch")
	}
	return nil
}

func validateKyrenReconciliationOrder(providerOrder *kyrenOrder, providerOrderID string, checkoutID string, snapshot model.KyrenPaymentSnapshot, mapping *model.PaymentProviderOrder) error {
	if providerOrder == nil || mapping == nil || strings.TrimSpace(providerOrder.ID) != providerOrderID || strings.TrimSpace(providerOrder.CheckoutSessionID) != checkoutID {
		return fmt.Errorf("Kyren reconciliation provider order identity mismatch")
	}
	if strings.TrimSpace(providerOrder.ProductID) != strings.TrimSpace(snapshot.ProductID) || !kyrenReconciliationAmountEqual(providerOrder.Amount, snapshot.Amount) || !strings.EqualFold(strings.TrimSpace(providerOrder.Currency), strings.TrimSpace(snapshot.Currency)) {
		return fmt.Errorf("Kyren reconciliation provider payment identity mismatch")
	}
	metadata := providerOrder.Metadata
	if metadata == nil || metadata["kind"] != model.PaymentOrderKindSubscription || metadata["trade_no"] != mapping.TradeNo || metadata["user_id"] != strconv.Itoa(mapping.UserID) || metadata["plan_id"] != strconv.Itoa(mapping.PlanID) {
		return fmt.Errorf("Kyren reconciliation provider metadata identity mismatch")
	}
	return nil
}

func kyrenReconciliationAmountEqual(left string, right string) bool {
	leftNormalized, leftErr := normalizeKyrenAmountString(left)
	rightNormalized, rightErr := normalizeKyrenAmountString(right)
	return leftErr == nil && rightErr == nil && leftNormalized == rightNormalized
}

func kyrenReconciliationEventForOrderStatus(raw string) (string, string, error) {
	status := strings.ToUpper(strings.TrimSpace(raw))
	switch status {
	case "PAID", "SETTLED":
		return service.KyrenPaymentEventPaid, "", nil
	case "FAILED":
		return service.KyrenPaymentEventFailed, "", nil
	case "CLOSED", "REVOKED", "EXPIRED":
		return service.KyrenPaymentEventClosed, "", nil
	case "REFUNDED":
		return service.KyrenPaymentEventRefunded, "", nil
	case "DISPUTED":
		return "order.disputed", "provider order is disputed and requires manual review", nil
	case "CHARGEBACK":
		return "order.chargeback", "provider order is charged back and requires manual recovery", nil
	case "CREATING", "PENDING":
		return "", "", nil
	default:
		return "", "", fmt.Errorf("Kyren reconciliation order status is unsupported: %s", status)
	}
}

func applyKyrenReconciliationFact(localOrder *model.SubscriptionOrder, mapping *model.PaymentProviderOrder, fact kyrenReconciliationFact, callerIP string, allowMissingProviderOrderID bool, manualReason string) error {
	payload, eventID, payloadHash, err := marshalKyrenReconciliationFact(fact)
	if err != nil {
		return err
	}
	result, err := service.ProcessKyrenPaymentEvent(service.KyrenPaymentEventRequest{
		EventID: eventID, EventType: fact.EventType, PayloadHash: payloadHash,
		TradeNo: mapping.TradeNo, OrderKind: mapping.OrderKind, ProviderOrderID: fact.OrderID,
		ProductID: fact.ProductID, Amount: fact.Amount, Currency: fact.Currency,
		ProviderPayload: payload, CallerIP: callerIP,
		MetadataUserID: mapping.UserID, MetadataPlanID: mapping.PlanID,
		AllowMissingProviderOrderID: allowMissingProviderOrderID,
		InvitationRewardHandler:     handleInvitationRewardForCompletedSubscriptionOrder,
	})
	if err != nil {
		return err
	}
	if result != nil && result.Outcome == model.PaymentProviderEventClaimed {
		reason := strings.TrimSpace(manualReason)
		if reason == "" && result.NeedsManualAction {
			reason = result.ManualActionReason
		}
		if reason != "" {
			recordKyrenPaymentManualAction(localOrder.UserId, localOrder.TradeNo, fact.EventType, reason)
		}
	}
	return nil
}

func marshalKyrenReconciliationFact(fact kyrenReconciliationFact) (string, string, string, error) {
	fact.Source = strings.TrimSpace(fact.Source)
	fact.EventType = strings.TrimSpace(fact.EventType)
	fact.CheckoutID = strings.TrimSpace(fact.CheckoutID)
	fact.OrderID = strings.TrimSpace(fact.OrderID)
	fact.ProductID = strings.TrimSpace(fact.ProductID)
	fact.Amount = strings.TrimSpace(fact.Amount)
	fact.Currency = strings.ToUpper(strings.TrimSpace(fact.Currency))
	fact.Status = strings.ToUpper(strings.TrimSpace(fact.Status))
	if fact.Source == "" || fact.EventType == "" || fact.CheckoutID == "" || fact.ProductID == "" || fact.Amount == "" || fact.Currency == "" || fact.Status == "" {
		return "", "", "", fmt.Errorf("invalid Kyren reconciliation fact")
	}
	encoded, err := common.Marshal(fact)
	if err != nil {
		return "", "", "", err
	}
	digest := sha256.Sum256(encoded)
	payloadHash := hex.EncodeToString(digest[:])
	return string(encoded), "reconcile_" + payloadHash, payloadHash, nil
}

func runKyrenReconciliationSweep(ctx context.Context, batchSize int) (kyrenReconciliationSweepResult, error) {
	if batchSize < 1 {
		batchSize = kyrenReconciliationTaskDefaultBatchSize
	}
	result := kyrenReconciliationSweepResult{}
	lastID := 0
	for {
		var orders []model.SubscriptionOrder
		query := model.DB.Table("subscription_orders AS so").
			Select("so.*").
			Joins("JOIN payment_provider_orders AS ppo ON ppo.provider = ? AND ppo.order_kind = ? AND ppo.local_order_id = so.id", model.PaymentProviderKyren, model.PaymentOrderKindSubscription).
			Where("so.id > ? AND so.payment_provider = ? AND so.status = ?", lastID, model.PaymentProviderKyren, common.TopUpStatusPending).
			Where("ppo.provider_checkout_id IS NOT NULL AND ppo.provider_checkout_id <> ?", "").
			Order("so.id ASC").
			Limit(batchSize).
			Find(&orders)
		if query.Error != nil {
			return result, query.Error
		}
		if len(orders) == 0 {
			return result, nil
		}
		for index := range orders {
			order := &orders[index]
			lastID = order.Id
			result.Scanned++
			_, err := reconcilePendingKyrenSubscriptionOrder(ctx, order, "")
			if err != nil {
				result.Failed++
				logger.LogWarn(ctx, fmt.Sprintf("Kyren 后台对账单笔失败 local_order_id=%d trade_no=%s error=%q", order.Id, order.TradeNo, err.Error()))
			}
		}
		if len(orders) < batchSize {
			return result, nil
		}
	}
}

func runKyrenReconciliationTaskOnce(batchSize int) {
	result, err := runKyrenReconciliationSweep(context.Background(), batchSize)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("Kyren 后台对账批次失败 scanned=%d failed=%d error=%q", result.Scanned, result.Failed, err.Error()))
		return
	}
	common.SysLog(fmt.Sprintf("Kyren 后台对账完成 scanned=%d failed=%d", result.Scanned, result.Failed))
}

func StartKyrenReconciliationTask() {
	kyrenReconciliationTaskOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		if !common.GetEnvOrDefaultBool("KYREN_RECONCILIATION_TASK_ENABLED", true) {
			common.SysLog("Kyren reconciliation task disabled by KYREN_RECONCILIATION_TASK_ENABLED")
			return
		}
		intervalSeconds := common.GetEnvOrDefault("KYREN_RECONCILIATION_TASK_INTERVAL_SECONDS", kyrenReconciliationTaskDefaultIntervalSeconds)
		if intervalSeconds < int(kyrenReconciliationMinIntervalSeconds) {
			intervalSeconds = kyrenReconciliationTaskDefaultIntervalSeconds
		}
		batchSize := common.GetEnvOrDefault("KYREN_RECONCILIATION_TASK_BATCH_SIZE", kyrenReconciliationTaskDefaultBatchSize)
		if batchSize < 1 {
			batchSize = kyrenReconciliationTaskDefaultBatchSize
		}
		if batchSize > 500 {
			batchSize = 500
		}
		interval := time.Duration(intervalSeconds) * time.Second
		go func() {
			common.SysLog(fmt.Sprintf("Kyren reconciliation task started: interval=%s batch_size=%d", interval, batchSize))
			runKyrenReconciliationTaskOnce(batchSize)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				runKyrenReconciliationTaskOnce(batchSize)
			}
		}()
	})
}
