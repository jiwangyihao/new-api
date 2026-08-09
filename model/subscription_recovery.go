package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SubscriptionOrderRecoveryRefund     = "refund"
	SubscriptionOrderRecoveryChargeback = "chargeback"
)

var ErrSubscriptionOrderRecoveryConflict = errors.New("subscription order recovery conflict")
var ErrSubscriptionOrderProviderIdentityAmbiguous = errors.New("subscription order provider identity is ambiguous")
var ErrSubscriptionOrderCreditRecoveryNotApplicable = errors.New("subscription order has no Credit recovery")
var ErrSubscriptionOrderRecoveryInvalid = errors.New("invalid subscription order recovery request")

type SubscriptionOrderProviderIdentity struct {
	TransactionID  string
	OrderID        string
	InvoiceID      string
	SubscriptionID string
}

func FindSubscriptionOrderByProviderIdentity(paymentProvider string, identity SubscriptionOrderProviderIdentity) (*SubscriptionOrder, error) {
	paymentProvider = strings.TrimSpace(paymentProvider)
	identity.TransactionID = strings.TrimSpace(identity.TransactionID)
	identity.OrderID = strings.TrimSpace(identity.OrderID)
	identity.InvoiceID = strings.TrimSpace(identity.InvoiceID)
	identity.SubscriptionID = strings.TrimSpace(identity.SubscriptionID)
	if paymentProvider == "" {
		return nil, ErrSubscriptionOrderNotFound
	}
	query := DB.Where("payment_provider = ?", paymentProvider)
	conditions := make([]string, 0, 4)
	arguments := make([]any, 0, 4)
	for column, value := range map[string]string{
		"provider_transaction_id":  identity.TransactionID,
		"provider_order_id":        identity.OrderID,
		"provider_invoice_id":      identity.InvoiceID,
		"provider_subscription_id": identity.SubscriptionID,
	} {
		if value != "" {
			conditions = append(conditions, column+" = ?")
			arguments = append(arguments, value)
		}
	}
	if len(conditions) == 0 {
		return nil, ErrSubscriptionOrderNotFound
	}
	var orders []SubscriptionOrder
	if err := query.Where("("+strings.Join(conditions, " OR ")+")", arguments...).Limit(2).Find(&orders).Error; err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, ErrSubscriptionOrderNotFound
	}
	if len(orders) != 1 {
		return nil, ErrSubscriptionOrderProviderIdentityAmbiguous
	}
	return &orders[0], nil
}

type SubscriptionOrderRecoveryRequest struct {
	TradeNo                 string
	ExpectedUserId          int
	ExpectedPaymentProvider string
	RecoveryType            string
	ProviderPayload         string
	ProviderRecoveryKey     string
	OperatorUserId          int
	Reason                  string
	CreditRecoveryOnly      bool
}

type SubscriptionOrderRecoveryResult struct {
	OrderId                    int    `json:"order_id"`
	TradeNo                    string `json:"trade_no"`
	Status                     string `json:"status"`
	RecoveryType               string `json:"recovery_type"`
	GrossCredit                int64  `json:"gross_credit"`
	ConsumedAvailableCredit    int64  `json:"consumed_available_credit"`
	RemovedExactCostMicros     int64  `json:"removed_exact_cost_micros,string"`
	RemovedEstimatedCostMicros int64  `json:"removed_estimated_cost_micros,string"`
	RemovedUnknownCredit       int64  `json:"removed_unknown_credit"`
	ValuationCurrency          string `json:"valuation_currency"`
	RuleVersion                int    `json:"rule_version"`
	StateVersionAfter          int64  `json:"state_version_after"`
	Operation                  string `json:"operation"`
	TerminalState              string `json:"terminal_state"`
	DebtFormed                 int64  `json:"debt_formed"`
	AvailableCredit            int64  `json:"available_credit"`
	SettlementDebt             int64  `json:"settlement_debt"`
	BalanceBefore              int64  `json:"balance_before"`
	BalanceAfter               int64  `json:"balance_after"`
	LedgerId                   int    `json:"ledger_id"`
	Replayed                   bool   `json:"replayed"`
}

type SubscriptionOrderRecoveryPreview struct {
	OrderId         int     `json:"order_id"`
	UserId          int     `json:"user_id"`
	Username        string  `json:"username"`
	PlanId          int     `json:"plan_id"`
	PlanTitle       string  `json:"plan_title"`
	TradeNo         string  `json:"trade_no"`
	Money           float64 `json:"money"`
	AmountCents     int64   `json:"amount_cents"`
	Currency        string  `json:"currency"`
	PaymentProvider string  `json:"payment_provider"`
	PaymentMethod   string  `json:"payment_method"`
	PurchaseMode    string  `json:"purchase_mode"`
	Status          string  `json:"status"`
	CompleteTime    int64   `json:"complete_time"`
	GrossCredit     int64   `json:"gross_credit"`
}

func GetSubscriptionOrderRecoveryPreview(tradeNo string, expectedUserId int) (*SubscriptionOrderRecoveryPreview, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" || expectedUserId <= 0 {
		return nil, ErrSubscriptionOrderNotFound
	}
	var order SubscriptionOrder
	if err := DB.Where("trade_no = ? AND user_id = ?", tradeNo, expectedUserId).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSubscriptionOrderNotFound
		}
		return nil, err
	}
	var username string
	if err := DB.Model(&User{}).Where("id = ?", order.UserId).Select("username").Scan(&username).Error; err != nil {
		return nil, err
	}
	var planTitle string
	if err := DB.Model(&SubscriptionPlan{}).Where("id = ?", order.PlanId).Select("title").Scan(&planTitle).Error; err != nil {
		return nil, err
	}
	purchaseMode := SubscriptionPurchaseModeTimed
	amountCents := order.AmountCents
	currency := order.Currency
	if payload := strings.TrimSpace(order.EntitlementSnapshot); payload != "" {
		if snapshot, err := UnmarshalSubscriptionEntitlementSnapshot(payload); err == nil {
			if mode := strings.TrimSpace(snapshot.PurchaseMode); mode != "" {
				purchaseMode = mode
			}
			if snapshot.PlanTitle != "" {
				planTitle = snapshot.PlanTitle
			}
			if amountCents <= 0 {
				amountCents = snapshot.PaymentAmountCents
			}
			if strings.TrimSpace(currency) == "" {
				currency = snapshot.PaymentCurrency
			}
		}
	}
	grossCredit, _, _, eligible, err := subscriptionOrderCreditRecoveryIdentityTx(DB, &order)
	if err != nil {
		return nil, err
	}
	if !eligible {
		grossCredit = 0
	}
	return &SubscriptionOrderRecoveryPreview{
		OrderId: order.Id, UserId: order.UserId, Username: username,
		PlanId: order.PlanId, PlanTitle: planTitle, TradeNo: order.TradeNo,
		Money: order.Money, AmountCents: amountCents, Currency: currency,
		PaymentProvider: order.PaymentProvider, PaymentMethod: order.PaymentMethod,
		PurchaseMode: purchaseMode, Status: order.Status, CompleteTime: order.CompleteTime,
		GrossCredit: grossCredit,
	}, nil
}

func RecoverSubscriptionOrder(request SubscriptionOrderRecoveryRequest) (*SubscriptionOrderRecoveryResult, error) {
	request.TradeNo = strings.TrimSpace(request.TradeNo)
	request.ExpectedPaymentProvider = strings.TrimSpace(request.ExpectedPaymentProvider)
	request.RecoveryType = strings.TrimSpace(request.RecoveryType)
	request.ProviderPayload = strings.TrimSpace(request.ProviderPayload)
	request.ProviderRecoveryKey = strings.TrimSpace(request.ProviderRecoveryKey)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.TradeNo == "" || request.Reason == "" {
		return nil, ErrSubscriptionOrderRecoveryInvalid
	}
	status := ""
	ledgerType := ""
	switch request.RecoveryType {
	case SubscriptionOrderRecoveryRefund:
		status = common.TopUpStatusRefunded
		ledgerType = CreditBalanceLedgerTypeRefund
	case SubscriptionOrderRecoveryChargeback:
		status = common.TopUpStatusChargeback
		ledgerType = CreditBalanceLedgerTypeChargeback
	default:
		return nil, ErrSubscriptionOrderRecoveryInvalid
	}

	var result *SubscriptionOrderRecoveryResult
	run := func(tx *gorm.DB) error {
		result = nil
		var order SubscriptionOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("trade_no = ?", request.TradeNo).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSubscriptionOrderNotFound
			}
			return err
		}
		if request.ExpectedUserId > 0 && order.UserId != request.ExpectedUserId {
			return ErrSubscriptionOrderNotFound
		}
		if request.ExpectedPaymentProvider != "" && order.PaymentProvider != request.ExpectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if order.Status == common.TopUpStatusRefunded || order.Status == common.TopUpStatusChargeback {
			fingerprintFacts, err := committedSubscriptionOrderRecoveryFingerprintFactsTx(tx, &order)
			if err != nil {
				return err
			}
			recoveryFingerprint, err := subscriptionOrderRecoveryFingerprint(request, fingerprintFacts)
			if err != nil {
				return err
			}
			if err := advanceSubscriptionOrderRecoveryTerminalTx(tx, &order, request, recoveryFingerprint); err != nil {
				return err
			}
			return loadSubscriptionOrderRecoveryResultTx(tx, &order, request, recoveryFingerprint, true, &result)
		}
		if order.Status != common.TopUpStatusSuccess {
			return ErrSubscriptionOrderStatusInvalid
		}
		credit, targetPlanId, sourceSnapshot, eligible, err := subscriptionOrderCreditRecoveryIdentityTx(tx, &order)
		if err != nil {
			return err
		}
		targetSubscriptionId := 0
		if eligible {
			targetSubscriptionId, err = subscriptionOrderRecoveryTargetSubscriptionIdTx(tx, order.UserId, targetPlanId)
			if err != nil {
				return err
			}
		}
		recoveryFingerprint, err := subscriptionOrderRecoveryFingerprint(request, subscriptionOrderRecoveryFingerprintFacts{
			SourceType: CreditBalanceLedgerSourceSubscriptionOrderRecovery,
			SourceId:   order.Id, SourceKey: creditBalanceRecoveryIdempotencyKey(order.Id),
			UserId: order.UserId, PaymentProvider: order.PaymentProvider,
			GrossCredit: credit, TargetPlanId: targetPlanId, TargetSubscriptionId: targetSubscriptionId,
			SourceSnapshot: sourceSnapshot, RuleVersion: CreditValuationRuleVersion,
		})
		if err != nil {
			return err
		}
		if !eligible {
			if request.CreditRecoveryOnly {
				return ErrSubscriptionOrderCreditRecoveryNotApplicable
			}
			if err := cancelInvitationRewardForSubscriptionOrderTx(tx, &order, request.Reason); err != nil {
				return err
			}
			now, err := getDBTimestampStrictTx(tx)
			if err != nil {
				return err
			}
			claim := tx.Model(&SubscriptionOrder{}).Where("id = ? AND status = ? AND recovery_ledger_id = ?", order.Id, common.TopUpStatusSuccess, 0).Updates(map[string]any{
				"status": status, "recovery_type": request.RecoveryType, "recovery_time": now, "recovery_reason": request.Reason,
				"recovery_fingerprint": recoveryFingerprint,
			})
			if claim.Error != nil {
				return claim.Error
			}
			if claim.RowsAffected != 1 {
				return fmt.Errorf("%w: subscription order changed concurrently", ErrSubscriptionOrderRecoveryConflict)
			}
			result = &SubscriptionOrderRecoveryResult{OrderId: order.Id, TradeNo: order.TradeNo, Status: status, RecoveryType: request.RecoveryType}
			order.RecoveryFingerprint = recoveryFingerprint
			return nil
		}
		recovery, err := RecoverCreditBalanceTx(tx, CreditBalanceRecoveryRequest{
			UserId:               order.UserId,
			GrossCredit:          credit,
			IdempotencyKey:       creditBalanceRecoveryIdempotencyKey(order.Id),
			SourceType:           CreditBalanceLedgerSourceSubscriptionOrderRecovery,
			SourceId:             order.Id,
			SourceKey:            creditBalanceRecoveryIdempotencyKey(order.Id),
			Operation:            request.RecoveryType,
			TerminalState:        status,
			SourceSnapshot:       sourceSnapshot,
			Type:                 ledgerType,
			TargetPlanId:         targetPlanId,
			PaymentProvider:      order.PaymentProvider,
			OperatorUserId:       request.OperatorUserId,
			Reason:               request.Reason,
			ParameterFingerprint: recoveryFingerprint,
		})
		if err != nil {
			return err
		}
		if err := cancelInvitationRewardForSubscriptionOrderTx(tx, &order, request.Reason); err != nil {
			return err
		}
		now, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		updates := map[string]any{
			"status":               status,
			"recovery_type":        request.RecoveryType,
			"recovery_time":        now,
			"recovery_ledger_id":   recovery.LedgerId,
			"recovery_reason":      request.Reason,
			"recovery_fingerprint": recoveryFingerprint,
		}
		if request.ProviderPayload != "" {
			updates["provider_payload"] = request.ProviderPayload
		}
		claim := tx.Model(&SubscriptionOrder{}).Where("id = ? AND status = ? AND recovery_ledger_id = ?", order.Id, common.TopUpStatusSuccess, 0).Updates(updates)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return fmt.Errorf("%w: subscription order changed concurrently", ErrSubscriptionOrderRecoveryConflict)
		}
		order.Status = status
		order.RecoveryType = request.RecoveryType
		order.RecoveryTime = now
		order.RecoveryLedgerID = recovery.LedgerId
		order.RecoveryReason = request.Reason
		order.RecoveryFingerprint = recoveryFingerprint
		result = &SubscriptionOrderRecoveryResult{
			OrderId: order.Id, TradeNo: order.TradeNo, Status: order.Status, RecoveryType: request.RecoveryType,
			GrossCredit: recovery.GrossCredit, ConsumedAvailableCredit: recovery.ConsumedAvailableCredit,
			RemovedExactCostMicros: recovery.RemovedExactCostMicros, RemovedEstimatedCostMicros: recovery.RemovedEstimatedCostMicros,
			RemovedUnknownCredit: recovery.RemovedUnknownCredit, ValuationCurrency: recovery.ValuationCurrency,
			RuleVersion: recovery.RuleVersion, StateVersionAfter: recovery.StateVersionAfter,
			Operation: request.RecoveryType, TerminalState: order.Status,
			DebtFormed: recovery.DebtFormed, AvailableCredit: recovery.AvailableCredit,
			SettlementDebt: recovery.SettlementDebt, BalanceBefore: recovery.BalanceBefore, BalanceAfter: recovery.BalanceAfter,
			LedgerId: recovery.LedgerId,
		}
		return nil
	}
	if err := transactionWithUserSettingCASRetry(run); err != nil {
		if replay, replayErr := findCommittedSubscriptionOrderRecovery(request); replayErr == nil && replay != nil {
			return replay, nil
		}
		return nil, err
	}
	if result != nil {
		primaryBillableSubscriptionCache.Delete(primaryBillableSubscriptionCacheKey(resultOrderUserId(result.OrderId)))
	}
	var order SubscriptionOrder
	if result != nil && DB.Select("user_id").First(&order, result.OrderId).Error == nil {
		invalidateUserCacheBestEffort(order.UserId)
	}
	return result, nil
}

func subscriptionOrderCreditRecoveryIdentityTx(tx *gorm.DB, order *SubscriptionOrder) (int64, int, string, bool, error) {
	if tx == nil || order == nil {
		return 0, 0, "", false, errors.New("invalid subscription order recovery identity")
	}
	purchase, found, err := subscriptionOrderCreditPurchaseLedgerTx(tx, order)
	if err != nil {
		return 0, 0, "", false, err
	}
	if found {
		return purchase.GrossCredit, purchase.TargetPlanId, purchase.SourceSnapshot, true, nil
	}
	var snapshot SubscriptionEntitlementSnapshot
	snapshotReliable := false
	if payload := strings.TrimSpace(order.EntitlementSnapshot); payload != "" {
		parsed, parseErr := UnmarshalSubscriptionEntitlementSnapshot(payload)
		if parseErr == nil {
			snapshot = parsed
			mode := strings.TrimSpace(snapshot.PurchaseMode)
			if mode == "" {
				mode = SubscriptionPurchaseModeTimed
			}
			if mode == SubscriptionPurchaseModeCreditBalance {
				if order.CreditGrantAmount <= 0 || order.CreditTargetPlanID <= 0 || snapshot.MonthlyTokenLimit != order.CreditGrantAmount || snapshot.TargetCreditBalancePlanID != order.CreditTargetPlanID {
					return 0, 0, "", false, ErrSubscriptionOrderSnapshotMismatch
				}
				return order.CreditGrantAmount, order.CreditTargetPlanID, order.EntitlementSnapshot, true, nil
			}
			snapshotReliable = mode == SubscriptionPurchaseModeTimed && snapshot.PlanID == order.PlanId && snapshot.MonthlyTokenLimit > 0
		} else if order.CreditGrantAmount != 0 || order.CreditTargetPlanID != 0 {
			return 0, 0, "", false, ErrSubscriptionOrderSnapshotMismatch
		}
	}
	if order.CreditGrantAmount != 0 || order.CreditTargetPlanID != 0 {
		return 0, 0, "", false, ErrSubscriptionOrderSnapshotMismatch
	}

	source, conversion, found, err := recoverableConvertedSubscriptionForOrderTx(tx, order)
	if err != nil || !found {
		return 0, 0, "", false, err
	}
	var target UserSubscription
	if err := tx.Select("id", "plan_id", "entitlement_type").Where("id = ? AND user_id = ?", conversion.TargetSubscriptionId, order.UserId).First(&target).Error; err != nil {
		return 0, 0, "", false, err
	}
	if target.EntitlementType != SubscriptionEntitlementCreditBalance || target.PlanId <= 0 {
		return 0, 0, "", false, errors.New("converted subscription target is not a Credit balance")
	}
	if conversion.GrossCredit <= 0 {
		return 0, 0, "", false, ErrSubscriptionOrderSnapshotMismatch
	}
	if snapshotReliable {
		return conversion.GrossCredit, target.PlanId, order.EntitlementSnapshot, true, nil
	}
	var plan SubscriptionPlan
	if err := tx.Select("id", "monthly_token_limit").Where("id = ? AND entitlement_type = ?", order.PlanId, SubscriptionEntitlementTimed).First(&plan).Error; err != nil {
		return 0, 0, "", false, err
	}
	if plan.MonthlyTokenLimit <= 0 {
		return 0, 0, "", false, errors.New("historical subscription order has no recoverable monthly Credit")
	}
	fallbackBytes, err := common.Marshal(map[string]any{
		"fallback": "current_plan_monthly_credit", "plan_id": plan.Id,
		"monthly_token_limit": plan.MonthlyTokenLimit, "source_subscription_id": source.Id,
	})
	if err != nil {
		return 0, 0, "", false, err
	}
	return conversion.GrossCredit, target.PlanId, string(fallbackBytes), true, nil
}

func recoverableConvertedSubscriptionForOrderTx(tx *gorm.DB, order *SubscriptionOrder) (*UserSubscription, *SubscriptionConversion, bool, error) {
	sourceSubscriptionId := order.FulfilledSubscriptionID
	if sourceSubscriptionId <= 0 {
		var found bool
		var err error
		sourceSubscriptionId, found, err = historicalSubscriptionOrderFulfillmentTx(tx, order)
		if err != nil || !found {
			return nil, nil, false, err
		}
	}
	var conversions []SubscriptionConversion
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND source_plan_id = ? AND source_subscription_id = ?", order.UserId, order.PlanId, sourceSubscriptionId).
		Limit(2).Find(&conversions).Error; err != nil {
		return nil, nil, false, err
	}
	if len(conversions) == 0 {
		return nil, nil, false, nil
	}
	if len(conversions) != 1 {
		return nil, nil, false, errors.New("historical subscription order conversion is ambiguous")
	}
	conversion := &conversions[0]
	var source UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ? AND plan_id = ?", conversion.SourceSubscriptionId, order.UserId, order.PlanId).
		First(&source).Error; err != nil {
		return nil, nil, false, err
	}
	if source.Status != SubscriptionStatusConverted || conversion.TargetSubscriptionId <= 0 {
		return nil, nil, false, nil
	}
	if source.ConversionId > 0 && source.ConversionId != conversion.Id {
		return nil, nil, false, errors.New("converted subscription conversion mapping mismatch")
	}
	if source.ConvertedToSubscriptionId > 0 && source.ConvertedToSubscriptionId != conversion.TargetSubscriptionId {
		return nil, nil, false, errors.New("converted subscription target mapping mismatch")
	}
	return &source, conversion, true, nil
}

func historicalSubscriptionOrderFulfillmentTx(tx *gorm.DB, order *SubscriptionOrder) (int, bool, error) {
	if tx == nil || order == nil || order.Id <= 0 || order.UserId <= 0 || order.PlanId <= 0 {
		return 0, false, nil
	}
	var rewardEvents []InvitationRewardEvent
	if err := tx.Select("source_subscription_id").
		Where("source_type = ? AND source_id = ? AND source_subscription_id > ?", InvitationRewardEventSourceSubscriptionOrder, order.Id, 0).
		Limit(2).Find(&rewardEvents).Error; err != nil {
		return 0, false, err
	}
	if len(rewardEvents) == 1 {
		return rewardEvents[0].SourceSubscriptionId, true, nil
	}
	if len(rewardEvents) > 1 || order.CompleteTime <= 0 {
		return 0, false, nil
	}
	var candidates []UserSubscription
	if err := tx.Select("id").
		Where("user_id = ? AND plan_id = ? AND entitlement_type = ?", order.UserId, order.PlanId, SubscriptionEntitlementTimed).
		Where("(start_time = ? OR start_time <= ?) AND (end_time = ? OR end_time >= ?)", 0, order.CompleteTime, 0, order.CompleteTime).
		Limit(2).Find(&candidates).Error; err != nil {
		return 0, false, err
	}
	if len(candidates) != 1 {
		return 0, false, nil
	}
	return candidates[0].Id, true, nil
}

func loadSubscriptionOrderRecoveryResultTx(tx *gorm.DB, order *SubscriptionOrder, request SubscriptionOrderRecoveryRequest, recoveryFingerprint string, replayed bool, destination **SubscriptionOrderRecoveryResult) error {
	if tx == nil || order == nil || destination == nil {
		return ErrSubscriptionOrderStatusInvalid
	}
	if request.ExpectedUserId > 0 && order.UserId != request.ExpectedUserId {
		return ErrSubscriptionOrderNotFound
	}
	if request.ExpectedPaymentProvider != "" && order.PaymentProvider != request.ExpectedPaymentProvider {
		return ErrPaymentMethodMismatch
	}
	if order.RecoveryFingerprint == "" || order.RecoveryFingerprint != recoveryFingerprint {
		return fmt.Errorf("%w: recovery facts mismatch", ErrSubscriptionOrderRecoveryConflict)
	}
	if order.RecoveryType != request.RecoveryType {
		return fmt.Errorf("%w: recovery terminal mismatch", ErrSubscriptionOrderRecoveryConflict)
	}
	if order.RecoveryLedgerID <= 0 {
		*destination = &SubscriptionOrderRecoveryResult{OrderId: order.Id, TradeNo: order.TradeNo, Status: order.Status, RecoveryType: order.RecoveryType, Replayed: replayed}
		return nil
	}
	var ledger CreditBalanceLedger
	if err := tx.Where("id = ? AND source_type = ? AND source_id = ?", order.RecoveryLedgerID, CreditBalanceLedgerSourceSubscriptionOrderRecovery, order.Id).First(&ledger).Error; err != nil {
		return err
	}
	gross := ledger.GrossCredit
	if ledger.ParameterFingerprint == "" {
		return fmt.Errorf("%w: recovery ledger facts missing", ErrSubscriptionOrderRecoveryConflict)
	}
	if ledger.Type == request.RecoveryType {
		if ledger.Operation != request.RecoveryType || ledger.TerminalState != order.Status || ledger.ParameterFingerprint != recoveryFingerprint {
			return fmt.Errorf("%w: recovery ledger facts mismatch", ErrSubscriptionOrderRecoveryConflict)
		}
	} else if ledger.Type != CreditBalanceLedgerTypeRefund || ledger.Operation != SubscriptionOrderRecoveryRefund || ledger.TerminalState != common.TopUpStatusRefunded || request.RecoveryType != SubscriptionOrderRecoveryChargeback {
		return fmt.Errorf("%w: recovery terminal history mismatch", ErrSubscriptionOrderRecoveryConflict)
	}
	if gross < 0 {
		gross = -gross
	}
	*destination = &SubscriptionOrderRecoveryResult{
		OrderId: order.Id, TradeNo: order.TradeNo, Status: order.Status, RecoveryType: order.RecoveryType,
		GrossCredit: gross, ConsumedAvailableCredit: ledger.ConsumedAvailableCredit,
		RemovedExactCostMicros: ledger.RemovedExactCostMicros, RemovedEstimatedCostMicros: ledger.RemovedEstimatedCostMicros,
		RemovedUnknownCredit: ledger.RemovedUnknownCredit, ValuationCurrency: ledger.ValuationCurrency,
		RuleVersion: ledger.ValuationRuleVersion, StateVersionAfter: ledger.ValuationStateVersionAfter,
		Operation: request.RecoveryType, TerminalState: order.Status,
		DebtFormed: creditBalanceLedgerDebtFormed(&ledger), AvailableCredit: ledger.AvailableCreditAfter,
		SettlementDebt: ledger.SettlementDebtAfter, BalanceBefore: ledger.BalanceBefore, BalanceAfter: ledger.BalanceAfter,
		LedgerId: ledger.Id, Replayed: replayed,
	}
	return nil
}

func advanceSubscriptionOrderRecoveryTerminalTx(tx *gorm.DB, order *SubscriptionOrder, request SubscriptionOrderRecoveryRequest, recoveryFingerprint string) error {
	if tx == nil || order == nil {
		return ErrSubscriptionOrderStatusInvalid
	}
	if order.RecoveryType == request.RecoveryType || order.RecoveryType == SubscriptionOrderRecoveryChargeback {
		return nil
	}
	if order.RecoveryFingerprint == "" {
		return fmt.Errorf("%w: recovery terminal facts missing", ErrSubscriptionOrderRecoveryConflict)
	}
	if order.RecoveryType != SubscriptionOrderRecoveryRefund || request.RecoveryType != SubscriptionOrderRecoveryChargeback {
		return fmt.Errorf("%w: invalid recovery terminal transition", ErrSubscriptionOrderRecoveryConflict)
	}
	now := getDBTimestampTx(tx)
	updates := map[string]any{
		"status": common.TopUpStatusChargeback, "recovery_type": SubscriptionOrderRecoveryChargeback,
		"recovery_time": now, "recovery_reason": request.Reason, "recovery_fingerprint": recoveryFingerprint,
	}
	if request.ProviderPayload != "" {
		updates["provider_payload"] = request.ProviderPayload
	}
	update := tx.Model(&SubscriptionOrder{}).
		Where("id = ? AND recovery_type = ? AND recovery_fingerprint = ?", order.Id, SubscriptionOrderRecoveryRefund, order.RecoveryFingerprint).
		Updates(updates)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return fmt.Errorf("%w: recovery terminal changed concurrently", ErrSubscriptionOrderRecoveryConflict)
	}
	order.Status = common.TopUpStatusChargeback
	order.RecoveryType = SubscriptionOrderRecoveryChargeback
	order.RecoveryTime = now
	order.RecoveryReason = request.Reason
	order.RecoveryFingerprint = recoveryFingerprint
	return nil
}

func cancelInvitationRewardForSubscriptionOrderTx(tx *gorm.DB, order *SubscriptionOrder, reason string) error {
	if tx == nil || order == nil || order.Id <= 0 {
		return errors.New("invalid subscription order reward cancellation")
	}
	now := getDBTimestampTx(tx)
	var event InvitationRewardEvent
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).Limit(1).Find(&event)
	if query.Error != nil || query.RowsAffected == 0 || event.Status == InvitationRewardEventStatusCancelled {
		return query.Error
	}
	if err := tx.Model(&InvitationRewardEvent{}).Where("id = ? AND status = ?", event.Id, InvitationRewardEventStatusActive).Updates(map[string]any{
		"status": InvitationRewardEventStatusCancelled, "reason": reason, "updated_at": now,
	}).Error; err != nil {
		return err
	}
	var commission InvitationCommissionRecord
	commissionQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_type = ? AND source_id = ?", InvitationCommissionSourceSubscriptionOrder, order.Id).Limit(1).Find(&commission)
	if commissionQuery.Error != nil || commissionQuery.RowsAffected == 0 || commission.Status != InvitationCommissionStatusAvailable || commission.CommissionCents <= 0 {
		return commissionQuery.Error
	}
	var account InvitationCommissionAccount
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", commission.InviterId).First(&account).Error; err != nil {
		return err
	}
	recoveredCents := account.AvailableCents
	if recoveredCents < 0 {
		recoveredCents = 0
	}
	if recoveredCents > commission.CommissionCents {
		recoveredCents = commission.CommissionCents
	}
	unrecoveredCents := commission.CommissionCents - recoveredCents
	if recoveredCents > 0 {
		accountUpdate := tx.Model(&InvitationCommissionAccount{}).
			Where("id = ? AND available_cents >= ?", account.Id, recoveredCents).
			Updates(map[string]any{"available_cents": gorm.Expr("available_cents - ?", recoveredCents), "updated_at": now})
		if accountUpdate.Error != nil {
			return accountUpdate.Error
		}
		if accountUpdate.RowsAffected != 1 {
			return errors.New("invitation commission account changed concurrently")
		}
		account.AvailableCents -= recoveredCents
	}
	recordUpdates := map[string]any{
		"recovered_cents": recoveredCents, "unrecovered_cents": unrecoveredCents,
		"reversal_reason": reason, "reversed_at": now,
	}
	if unrecoveredCents == 0 {
		recordUpdates["status"] = InvitationCommissionStatusCancelled
		recordUpdates["reason"] = reason
		recordUpdates["cancelled_at"] = now
		recordUpdates["reversal_status"] = InvitationCommissionReversalStatusRecovered
	} else {
		recordUpdates["status"] = InvitationCommissionStatusUnrecoverable
		recordUpdates["reversal_status"] = InvitationCommissionReversalStatusUnrecoverable
	}
	if err := tx.Model(&InvitationCommissionRecord{}).
		Where("id = ? AND status = ?", commission.Id, InvitationCommissionStatusAvailable).
		Updates(recordUpdates).Error; err != nil {
		return err
	}
	if recoveredCents > 0 {
		if err := tx.Create(&InvitationCommissionLedger{
			UserId: commission.InviterId, Type: InvitationCommissionLedgerRefundReversal,
			AmountCents: -recoveredCents, AvailableAfterCents: account.AvailableCents,
			PendingAfterCents: account.PendingCents, ReferenceType: "commission_record",
			ReferenceId: commission.Id, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
	}
	if unrecoveredCents > 0 {
		return tx.Create(&InvitationCommissionLedger{
			UserId: commission.InviterId, Type: InvitationCommissionLedgerRefundUnrecoverable,
			AmountCents: unrecoveredCents, AvailableAfterCents: account.AvailableCents,
			PendingAfterCents: account.PendingCents, ReferenceType: "commission_record",
			ReferenceId: commission.Id, CreatedAt: now,
		}).Error
	}
	return nil
}

type subscriptionOrderRecoveryFingerprintFacts struct {
	SourceType           string `json:"source_type"`
	SourceId             int    `json:"source_id"`
	SourceKey            string `json:"source_key"`
	UserId               int    `json:"user_id"`
	PaymentProvider      string `json:"payment_provider"`
	GrossCredit          int64  `json:"gross_credit"`
	TargetPlanId         int    `json:"target_plan_id"`
	TargetSubscriptionId int    `json:"target_subscription_id"`
	SourceSnapshot       string `json:"source_snapshot"`
	RuleVersion          int    `json:"rule_version"`
}

func subscriptionOrderRecoveryTargetSubscriptionIdTx(tx *gorm.DB, userId int, targetPlanId int) (int, error) {
	if tx == nil || userId <= 0 || targetPlanId <= 0 {
		return 0, errors.New("invalid subscription order recovery target")
	}
	var targets []UserSubscription
	if err := tx.Select("id").
		Where("user_id = ? AND plan_id = ? AND entitlement_type = ?", userId, targetPlanId, SubscriptionEntitlementCreditBalance).
		Limit(2).Find(&targets).Error; err != nil {
		return 0, err
	}
	if len(targets) != 1 {
		return 0, errors.New("subscription order recovery target is not unique")
	}
	return targets[0].Id, nil
}

func committedSubscriptionOrderRecoveryFingerprintFactsTx(tx *gorm.DB, order *SubscriptionOrder) (subscriptionOrderRecoveryFingerprintFacts, error) {
	if tx == nil || order == nil || order.Id <= 0 || order.UserId <= 0 {
		return subscriptionOrderRecoveryFingerprintFacts{}, errors.New("invalid committed subscription order recovery")
	}
	facts := subscriptionOrderRecoveryFingerprintFacts{
		SourceType: CreditBalanceLedgerSourceSubscriptionOrderRecovery,
		SourceId:   order.Id, SourceKey: creditBalanceRecoveryIdempotencyKey(order.Id),
		UserId: order.UserId, PaymentProvider: order.PaymentProvider,
		RuleVersion: CreditValuationRuleVersion,
	}
	if order.RecoveryLedgerID <= 0 {
		return facts, nil
	}
	var ledger CreditBalanceLedger
	if err := tx.Where("id = ? AND source_type = ? AND source_id = ?", order.RecoveryLedgerID, facts.SourceType, order.Id).First(&ledger).Error; err != nil {
		return subscriptionOrderRecoveryFingerprintFacts{}, err
	}
	if ledger.UserId != order.UserId || ledger.SourceKey != facts.SourceKey || ledger.UserSubscriptionId <= 0 || ledger.TargetPlanId <= 0 || ledger.GrossCredit >= 0 || ledger.PaymentProvider != order.PaymentProvider || (ledger.ValuationRuleVersion != 0 && ledger.ValuationRuleVersion != CreditValuationRuleVersion) {
		return subscriptionOrderRecoveryFingerprintFacts{}, fmt.Errorf("%w: persisted recovery facts mismatch", ErrSubscriptionOrderRecoveryConflict)
	}
	facts.PaymentProvider = ledger.PaymentProvider
	facts.GrossCredit = -ledger.GrossCredit
	facts.TargetPlanId = ledger.TargetPlanId
	facts.TargetSubscriptionId = ledger.UserSubscriptionId
	facts.SourceSnapshot = ledger.SourceSnapshot
	return facts, nil
}

func subscriptionOrderRecoveryFingerprint(request SubscriptionOrderRecoveryRequest, facts subscriptionOrderRecoveryFingerprintFacts) (string, error) {
	terminalState := ""
	switch request.RecoveryType {
	case SubscriptionOrderRecoveryRefund:
		terminalState = common.TopUpStatusRefunded
	case SubscriptionOrderRecoveryChargeback:
		terminalState = common.TopUpStatusChargeback
	default:
		return "", ErrSubscriptionOrderRecoveryInvalid
	}
	providerRecoveryKey := request.ProviderRecoveryKey
	if providerRecoveryKey == "" {
		providerRecoveryKey = request.ProviderPayload
	}
	payload, err := common.Marshal(struct {
		Operation           string                                    `json:"operation"`
		TerminalState       string                                    `json:"terminal_state"`
		ProviderRecoveryKey string                                    `json:"provider_recovery_key"`
		OperatorUserId      int                                       `json:"operator_user_id"`
		Reason              string                                    `json:"reason"`
		CreditOnly          bool                                      `json:"credit_recovery_only"`
		Facts               subscriptionOrderRecoveryFingerprintFacts `json:"facts"`
	}{
		Operation: request.RecoveryType, TerminalState: terminalState,
		ProviderRecoveryKey: providerRecoveryKey, OperatorUserId: request.OperatorUserId,
		Reason: request.Reason, CreditOnly: request.CreditRecoveryOnly, Facts: facts,
	})
	if err != nil {
		return "", err
	}
	return common.Sha1(payload), nil
}

func findCommittedSubscriptionOrderRecovery(request SubscriptionOrderRecoveryRequest) (*SubscriptionOrderRecoveryResult, error) {
	var order SubscriptionOrder
	if err := DB.Where("trade_no = ?", request.TradeNo).First(&order).Error; err != nil {
		return nil, err
	}
	fingerprintFacts, err := committedSubscriptionOrderRecoveryFingerprintFactsTx(DB, &order)
	if err != nil {
		return nil, err
	}
	recoveryFingerprint, err := subscriptionOrderRecoveryFingerprint(request, fingerprintFacts)
	if err != nil {
		return nil, err
	}
	var result *SubscriptionOrderRecoveryResult
	if err := loadSubscriptionOrderRecoveryResultTx(DB, &order, request, recoveryFingerprint, true, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func resultOrderUserId(orderId int) int {
	if orderId <= 0 {
		return 0
	}
	var order SubscriptionOrder
	if err := DB.Select("user_id").First(&order, orderId).Error; err != nil {
		return 0
	}
	return order.UserId
}

func subscriptionOrderRecoveryDescription(order *SubscriptionOrder, recoveryType string) string {
	if order == nil {
		return ""
	}
	return fmt.Sprintf("%s subscription order %s", recoveryType, order.TradeNo)
}
