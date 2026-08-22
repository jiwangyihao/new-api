package model

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PaymentOrderKindSubscription = "subscription"
	PaymentOrderKindTopUp        = "topup"

	PaymentProviderEventProcessing = "processing"
	PaymentProviderEventApplied    = "applied"
	PaymentProviderEventIgnored    = "ignored"
	PaymentProviderEventConflict   = "conflict"
)

var (
	ErrPaymentProviderOrderNotFound   = errors.New("payment provider order not found")
	ErrPaymentProviderOrderConflict   = errors.New("payment provider order identity conflict")
	ErrPaymentProviderEventInProgress = errors.New("payment provider event is being processed")
)

type PaymentProviderOrder struct {
	ID int64 `json:"id"`

	Provider     string `json:"provider" gorm:"type:varchar(32);not null;uniqueIndex:idx_provider_trade,priority:1;uniqueIndex:idx_provider_order_id,priority:1;uniqueIndex:idx_provider_checkout_id,priority:1;uniqueIndex:idx_provider_local_order,priority:1"`
	OrderKind    string `json:"order_kind" gorm:"type:varchar(32);not null;uniqueIndex:idx_provider_local_order,priority:2;index"`
	LocalOrderID int    `json:"local_order_id" gorm:"not null;uniqueIndex:idx_provider_local_order,priority:3"`
	TradeNo      string `json:"trade_no" gorm:"type:varchar(255);not null;uniqueIndex:idx_provider_trade,priority:2"`
	UserID       int    `json:"user_id" gorm:"not null;index"`
	PlanID       int    `json:"plan_id" gorm:"not null;default:0;index"`

	ProviderOrderID     *string `json:"provider_order_id,omitempty" gorm:"type:varchar(255);uniqueIndex:idx_provider_order_id,priority:2"`
	ProviderCheckoutID  *string `json:"provider_checkout_id,omitempty" gorm:"type:varchar(255);uniqueIndex:idx_provider_checkout_id,priority:2"`
	ProviderCheckoutURL *string `json:"-" gorm:"type:varchar(2048)"`

	CreatedAt int64 `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt int64 `json:"updated_at" gorm:"type:bigint;not null"`
}

type PaymentProviderCreationLock struct {
	LockKey   string `json:"-" gorm:"primaryKey;type:varchar(255)"`
	CreatedAt int64  `json:"-" gorm:"type:bigint;not null"`
}

type PaymentProviderEvent struct {
	ID int64 `json:"id"`

	Provider        string `json:"provider" gorm:"type:varchar(32);not null;uniqueIndex:idx_provider_event,priority:1"`
	EventID         string `json:"event_id" gorm:"type:varchar(255);not null;uniqueIndex:idx_provider_event,priority:2"`
	EventType       string `json:"event_type" gorm:"type:varchar(64);not null"`
	PayloadHash     string `json:"payload_hash" gorm:"type:varchar(64);not null"`
	TradeNo         string `json:"trade_no" gorm:"type:varchar(255);not null;default:'';index"`
	OrderKind       string `json:"order_kind" gorm:"type:varchar(32);not null;default:'';index"`
	ProviderOrderID string `json:"provider_order_id" gorm:"type:varchar(255);not null;default:'';index"`
	LocalOrderID    int    `json:"local_order_id" gorm:"not null;default:0;index"`
	UserID          int    `json:"user_id" gorm:"not null;default:0;index"`

	Status              string `json:"status" gorm:"type:varchar(32);not null;index"`
	ProcessingToken     string `json:"-" gorm:"type:varchar(64);not null;default:''"`
	ProcessingStartedAt int64  `json:"processing_started_at" gorm:"type:bigint;not null;default:0;index"`
	ProcessedAt         int64  `json:"processed_at" gorm:"type:bigint;not null;default:0;index"`
	OutcomeReason       string `json:"outcome_reason" gorm:"type:varchar(255);not null;default:''"`
	ResultPurchaseMode  string `json:"result_purchase_mode" gorm:"type:varchar(32);not null;default:''"`

	ConflictCount       int    `json:"conflict_count" gorm:"not null;default:0"`
	ConflictPayloadHash string `json:"conflict_payload_hash" gorm:"type:varchar(64);not null;default:''"`
	ConflictTradeNo     string `json:"conflict_trade_no" gorm:"type:varchar(255);not null;default:''"`
	LastConflictAt      int64  `json:"last_conflict_at" gorm:"type:bigint;not null;default:0"`

	CreatedAt int64 `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt int64 `json:"updated_at" gorm:"type:bigint;not null"`
}

func LockPaymentProviderCreationTx(tx *gorm.DB, lockKey string) error {
	lockKey = strings.TrimSpace(lockKey)
	if tx == nil || lockKey == "" {
		return errors.New("invalid payment provider creation lock")
	}
	now, err := getDBTimestampStrictTx(tx)
	if err != nil {
		return err
	}
	lock := PaymentProviderCreationLock{LockKey: lockKey, CreatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lock).Error; err != nil {
		return err
	}
	return LockForUpdate(tx).Where("lock_key = ?", lockKey).First(&lock).Error
}

func CreatePaymentProviderOrderTx(tx *gorm.DB, order *PaymentProviderOrder) error {
	if tx == nil || order == nil {
		return errors.New("invalid payment provider order")
	}
	order.Provider = strings.TrimSpace(order.Provider)
	order.OrderKind = strings.TrimSpace(order.OrderKind)
	order.TradeNo = strings.TrimSpace(order.TradeNo)
	if order.Provider == "" || order.TradeNo == "" || order.LocalOrderID <= 0 || order.UserID <= 0 || (order.OrderKind != PaymentOrderKindSubscription && order.OrderKind != PaymentOrderKindTopUp) {
		return errors.New("invalid payment provider order identity")
	}
	if order.OrderKind == PaymentOrderKindSubscription && order.PlanID <= 0 {
		return errors.New("invalid subscription payment provider order")
	}
	if order.OrderKind == PaymentOrderKindTopUp {
		order.PlanID = 0
	}
	now := int64(0)
	var err error
	if now, err = getDBTimestampStrictTx(tx); err != nil {
		return err
	}
	if order.CreatedAt == 0 {
		order.CreatedAt = now
	}
	order.UpdatedAt = now
	return tx.Create(order).Error
}

func EnsurePaymentProviderOrderTx(tx *gorm.DB, provider string, orderKind string, tradeNo string) (*PaymentProviderOrder, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	provider = strings.TrimSpace(provider)
	orderKind = strings.TrimSpace(orderKind)
	tradeNo = strings.TrimSpace(tradeNo)
	if provider == "" || tradeNo == "" {
		return nil, ErrPaymentProviderOrderNotFound
	}
	var mapping PaymentProviderOrder
	query := LockForUpdate(tx).Where("provider = ? AND trade_no = ?", provider, tradeNo).Limit(1).Find(&mapping)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected > 0 {
		if mapping.OrderKind != orderKind {
			return nil, ErrPaymentProviderOrderConflict
		}
		return &mapping, nil
	}

	candidate := PaymentProviderOrder{Provider: provider, OrderKind: orderKind, TradeNo: tradeNo}
	switch orderKind {
	case PaymentOrderKindSubscription:
		var order SubscriptionOrder
		if err := LockForUpdate(tx).Where("trade_no = ? AND payment_provider = ?", tradeNo, provider).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrPaymentProviderOrderNotFound
			}
			return nil, err
		}
		candidate.LocalOrderID = order.Id
		candidate.UserID = order.UserId
		candidate.PlanID = order.PlanId
		candidate.ProviderOrderID = nonEmptyStringPointer(order.ProviderOrderID)
	case PaymentOrderKindTopUp:
		var order TopUp
		if err := LockForUpdate(tx).Where("trade_no = ? AND payment_provider = ?", tradeNo, provider).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrPaymentProviderOrderNotFound
			}
			return nil, err
		}
		candidate.LocalOrderID = order.Id
		candidate.UserID = order.UserId
	default:
		return nil, ErrPaymentProviderOrderConflict
	}

	create := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate)
	if create.Error != nil {
		return nil, create.Error
	}
	if create.RowsAffected > 0 {
		return &candidate, nil
	}
	if err := LockForUpdate(tx).Where("provider = ? AND trade_no = ?", provider, tradeNo).First(&mapping).Error; err != nil {
		return nil, err
	}
	if mapping.OrderKind != candidate.OrderKind || mapping.LocalOrderID != candidate.LocalOrderID || mapping.UserID != candidate.UserID || mapping.PlanID != candidate.PlanID {
		return nil, ErrPaymentProviderOrderConflict
	}
	return &mapping, nil
}

func BindPaymentProviderOrderIDTx(tx *gorm.DB, mapping *PaymentProviderOrder, providerOrderID string) error {
	if tx == nil || mapping == nil {
		return errors.New("invalid payment provider order")
	}
	providerOrderID = strings.TrimSpace(providerOrderID)
	if providerOrderID == "" {
		return ErrPaymentProviderOrderConflict
	}
	if mapping.ProviderOrderID != nil {
		if *mapping.ProviderOrderID == providerOrderID {
			return nil
		}
		return ErrPaymentProviderOrderConflict
	}
	var conflicting PaymentProviderOrder
	query := tx.Where("provider = ? AND provider_order_id = ? AND id <> ?", mapping.Provider, providerOrderID, mapping.ID).Limit(1).Find(&conflicting)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected > 0 {
		return ErrPaymentProviderOrderConflict
	}
	now, err := getDBTimestampStrictTx(tx)
	if err != nil {
		return err
	}
	update := tx.Model(&PaymentProviderOrder{}).
		Where("id = ? AND provider_order_id IS NULL", mapping.ID).
		Updates(map[string]any{"provider_order_id": providerOrderID, "provider_checkout_url": nil, "updated_at": now})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected == 0 {
		return ErrPaymentProviderOrderConflict
	}
	mapping.ProviderOrderID = &providerOrderID
	mapping.UpdatedAt = now
	if mapping.OrderKind == PaymentOrderKindSubscription {
		localUpdate := tx.Model(&SubscriptionOrder{}).
			Where("id = ? AND (provider_order_id = ? OR provider_order_id = ?)", mapping.LocalOrderID, "", providerOrderID).
			Update("provider_order_id", providerOrderID)
		if localUpdate.Error != nil {
			return localUpdate.Error
		}
		if localUpdate.RowsAffected == 0 {
			return ErrPaymentProviderOrderConflict
		}
	}
	return nil
}

func BindPaymentProviderOrderID(provider string, orderKind string, tradeNo string, providerOrderID string) error {
	provider = strings.TrimSpace(provider)
	orderKind = strings.TrimSpace(orderKind)
	tradeNo = strings.TrimSpace(tradeNo)
	providerOrderID = strings.TrimSpace(providerOrderID)
	if provider == "" || tradeNo == "" || providerOrderID == "" {
		return ErrPaymentProviderOrderConflict
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		mapping, err := EnsurePaymentProviderOrderTx(tx, provider, orderKind, tradeNo)
		if err != nil {
			return err
		}
		return BindPaymentProviderOrderIDTx(tx, mapping, providerOrderID)
	})
}

func BindPaymentProviderCheckout(provider string, tradeNo string, checkoutID string, checkoutURL string) error {
	provider = strings.TrimSpace(provider)
	tradeNo = strings.TrimSpace(tradeNo)
	checkoutID = strings.TrimSpace(checkoutID)
	checkoutURL = strings.TrimSpace(checkoutURL)
	if provider == "" || tradeNo == "" || checkoutID == "" {
		return ErrPaymentProviderOrderConflict
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var mapping PaymentProviderOrder
		if err := LockForUpdate(tx).Where("provider = ? AND trade_no = ?", provider, tradeNo).First(&mapping).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPaymentProviderOrderNotFound
			}
			return err
		}
		if mapping.ProviderCheckoutID != nil && *mapping.ProviderCheckoutID != checkoutID {
			return ErrPaymentProviderOrderConflict
		}
		var conflicting PaymentProviderOrder
		query := tx.Where("provider = ? AND provider_checkout_id = ? AND id <> ?", provider, checkoutID, mapping.ID).Limit(1).Find(&conflicting)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			return ErrPaymentProviderOrderConflict
		}
		now, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		updates := map[string]any{"provider_checkout_id": checkoutID, "updated_at": now}
		if checkoutURL != "" {
			updates["provider_checkout_url"] = checkoutURL
		}
		result := tx.Model(&PaymentProviderOrder{}).
			Where("id = ? AND (provider_checkout_id IS NULL OR provider_checkout_id = ?)", mapping.ID, checkoutID).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrPaymentProviderOrderConflict
		}
		return nil
	})
}

func BindPaymentProviderCheckoutID(provider string, tradeNo string, checkoutID string) error {
	return BindPaymentProviderCheckout(provider, tradeNo, checkoutID, "")
}

// ClaimPaymentProviderReconciliation atomically reserves a Kyren mapping for a
// provider status lookup. The mapping timestamp is the shared cross-instance
// throttle; callers must not perform the provider request before this claim.
func ClaimPaymentProviderReconciliation(provider string, orderKind string, tradeNo string, minIntervalSeconds int64) (*PaymentProviderOrder, bool, error) {
	provider = strings.TrimSpace(provider)
	orderKind = strings.TrimSpace(orderKind)
	tradeNo = strings.TrimSpace(tradeNo)
	if DB == nil || provider == "" || tradeNo == "" {
		return nil, false, ErrPaymentProviderOrderNotFound
	}
	var mapping PaymentProviderOrder
	claimed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var current PaymentProviderOrder
		if err := LockForUpdate(tx).
			Where("provider = ? AND order_kind = ? AND trade_no = ?", provider, orderKind, tradeNo).
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if current.ProviderCheckoutID == nil || strings.TrimSpace(*current.ProviderCheckoutID) == "" {
			mapping = current
			return nil
		}
		now, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		if minIntervalSeconds > 0 && current.UpdatedAt > 0 && now-current.UpdatedAt < minIntervalSeconds {
			mapping = current
			return nil
		}
		update := tx.Model(&PaymentProviderOrder{}).
			Where("id = ? AND updated_at = ?", current.ID, current.UpdatedAt).
			Updates(map[string]any{"updated_at": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrPaymentProviderOrderConflict
		}
		current.UpdatedAt = now
		mapping = current
		claimed = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if mapping.ID <= 0 {
		return nil, false, nil
	}
	return &mapping, claimed, nil
}

func nonEmptyStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

type PaymentProviderEventClaimRequest struct {
	Provider        string
	EventID         string
	EventType       string
	PayloadHash     string
	TradeNo         string
	OrderKind       string
	ProviderOrderID string
	StaleBefore     int64
}

type PaymentProviderEventClaimOutcome string

const (
	PaymentProviderEventClaimed    PaymentProviderEventClaimOutcome = "claimed"
	PaymentProviderEventDuplicate  PaymentProviderEventClaimOutcome = "duplicate"
	PaymentProviderEventInProgress PaymentProviderEventClaimOutcome = "in_progress"
	PaymentProviderEventConflicted PaymentProviderEventClaimOutcome = "conflict"
)

func ClaimPaymentProviderEventTx(tx *gorm.DB, request PaymentProviderEventClaimRequest) (*PaymentProviderEvent, PaymentProviderEventClaimOutcome, error) {
	if tx == nil {
		return nil, "", errors.New("tx is nil")
	}
	request.Provider = strings.TrimSpace(request.Provider)
	request.EventID = strings.TrimSpace(request.EventID)
	request.EventType = strings.TrimSpace(request.EventType)
	request.PayloadHash = strings.TrimSpace(request.PayloadHash)
	request.TradeNo = strings.TrimSpace(request.TradeNo)
	request.OrderKind = strings.TrimSpace(request.OrderKind)
	request.ProviderOrderID = strings.TrimSpace(request.ProviderOrderID)
	if request.Provider == "" || request.EventID == "" || request.EventType == "" || len(request.PayloadHash) != 64 {
		return nil, "", errors.New("invalid payment provider event identity")
	}
	processingToken, err := newKyrenProcessingToken()
	if err != nil {
		return nil, "", err
	}
	now, err := getDBTimestampStrictTx(tx)
	if err != nil {
		return nil, "", err
	}
	candidate := PaymentProviderEvent{
		Provider:            request.Provider,
		EventID:             request.EventID,
		EventType:           request.EventType,
		PayloadHash:         request.PayloadHash,
		TradeNo:             request.TradeNo,
		OrderKind:           request.OrderKind,
		ProviderOrderID:     request.ProviderOrderID,
		Status:              PaymentProviderEventProcessing,
		ProcessingToken:     processingToken,
		ProcessingStartedAt: now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	create := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}, {Name: "event_id"}},
		DoNothing: true,
	}).Create(&candidate)
	if create.Error != nil {
		return nil, "", create.Error
	}
	if create.RowsAffected > 0 {
		return &candidate, PaymentProviderEventClaimed, nil
	}

	var existing PaymentProviderEvent
	if err := lockForUpdate(tx).Where("provider = ? AND event_id = ?", request.Provider, request.EventID).First(&existing).Error; err != nil {
		return nil, "", err
	}
	if existing.PayloadHash != request.PayloadHash || existing.EventType != request.EventType || existing.TradeNo != request.TradeNo || existing.OrderKind != request.OrderKind || existing.ProviderOrderID != request.ProviderOrderID {
		updates := map[string]any{
			"conflict_count":        gorm.Expr("conflict_count + ?", 1),
			"conflict_payload_hash": request.PayloadHash,
			"conflict_trade_no":     request.TradeNo,
			"last_conflict_at":      now,
			"updated_at":            now,
		}
		result := tx.Model(&PaymentProviderEvent{}).
			Where("id = ?", existing.ID).
			Updates(updates)
		if result.Error != nil {
			return nil, "", result.Error
		}
		if result.RowsAffected == 0 {
			return nil, "", gorm.ErrRecordNotFound
		}
		existing.ConflictCount++
		existing.ConflictPayloadHash = request.PayloadHash
		existing.ConflictTradeNo = request.TradeNo
		existing.LastConflictAt = now
		existing.UpdatedAt = now
		return &existing, PaymentProviderEventConflicted, nil
	}

	switch existing.Status {
	case PaymentProviderEventApplied, PaymentProviderEventIgnored:
		return &existing, PaymentProviderEventDuplicate, nil
	case PaymentProviderEventConflict:
		return &existing, PaymentProviderEventConflicted, nil
	case PaymentProviderEventProcessing:
		if existing.ProcessingStartedAt > 0 && existing.ProcessingStartedAt <= request.StaleBefore {
			result := tx.Model(&PaymentProviderEvent{}).
				Where("id = ? AND status = ? AND processing_token = ? AND processing_started_at = ?", existing.ID, PaymentProviderEventProcessing, existing.ProcessingToken, existing.ProcessingStartedAt).
				Updates(map[string]any{"processing_token": processingToken, "processing_started_at": now, "updated_at": now})
			if result.Error != nil {
				return nil, "", result.Error
			}
			if result.RowsAffected == 0 {
				return &existing, PaymentProviderEventInProgress, nil
			}
			existing.ProcessingToken = processingToken
			existing.ProcessingStartedAt = now
			existing.UpdatedAt = now
			return &existing, PaymentProviderEventClaimed, nil
		}
		return &existing, PaymentProviderEventInProgress, nil
	default:
		return nil, "", fmt.Errorf("invalid payment provider event status: %s", existing.Status)
	}
}

func FinishPaymentProviderEventTx(tx *gorm.DB, event *PaymentProviderEvent, status string, reason string, purchaseMode string) error {
	if tx == nil || event == nil || event.ID <= 0 || event.ProcessingToken == "" {
		return errors.New("invalid payment provider event claim")
	}
	if status != PaymentProviderEventApplied && status != PaymentProviderEventIgnored && status != PaymentProviderEventConflict {
		return errors.New("invalid payment provider event terminal status")
	}
	now, err := getDBTimestampStrictTx(tx)
	if err != nil {
		return err
	}
	result := tx.Model(&PaymentProviderEvent{}).
		Where("id = ? AND status = ? AND processing_token = ?", event.ID, PaymentProviderEventProcessing, event.ProcessingToken).
		Updates(map[string]any{
			"status":                status,
			"processing_token":      "",
			"processing_started_at": int64(0),
			"processed_at":          now,
			"outcome_reason":        strings.TrimSpace(reason),
			"result_purchase_mode":  strings.TrimSpace(purchaseMode),
			"updated_at":            now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPaymentProviderEventInProgress
	}
	event.Status = status
	event.ProcessingToken = ""
	event.ProcessingStartedAt = 0
	event.ProcessedAt = now
	event.OutcomeReason = strings.TrimSpace(reason)
	event.ResultPurchaseMode = strings.TrimSpace(purchaseMode)
	return nil
}

func BindPaymentProviderEventOrderTx(tx *gorm.DB, event *PaymentProviderEvent, mapping *PaymentProviderOrder) error {
	if tx == nil || event == nil || mapping == nil || event.ID <= 0 || mapping.ID <= 0 {
		return errors.New("invalid payment provider event order")
	}
	result := tx.Model(&PaymentProviderEvent{}).
		Where("id = ? AND status = ? AND processing_token = ?", event.ID, PaymentProviderEventProcessing, event.ProcessingToken).
		Updates(map[string]any{"local_order_id": mapping.LocalOrderID, "user_id": mapping.UserID})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPaymentProviderEventInProgress
	}
	event.LocalOrderID = mapping.LocalOrderID
	event.UserID = mapping.UserID
	return nil
}
