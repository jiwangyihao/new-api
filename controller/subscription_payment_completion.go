package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"gorm.io/gorm"
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

func completeKyrenSubscriptionOrderWithSnapshotAndEvaluateInvitation(tradeNo string, providerPayload string, expectedPaymentProvider string, actualPaymentMethod string) error {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}
	var logUserID int
	var logPlanID int
	var logMoney float64
	var logPaymentMethod string
	claimed, err := model.ClaimPendingKyrenSubscriptionOrder(tradeNo)
	if err != nil {
		return err
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
			logUserID = order.UserId
			return nil
		}
		return model.ErrSubscriptionOrderStatusInvalid
	}
	defer func() {
		if err != nil {
			_ = model.RestoreClaimedKyrenSubscriptionOrder(tradeNo)
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
		snapshot, err := model.UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
		if err != nil {
			return err
		}
		plan := kyrenSubscriptionPlanFromEntitlementSnapshot(snapshot)
		if _, err := model.CreateUserSubscriptionFromPlanTx(tx, order.UserId, plan, model.SubscriptionGrantOrder); err != nil {
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
		if err := model.MarkClaimedKyrenSubscriptionOrderSuccessTx(tx, &order); err != nil {
			return err
		}
		logUserID = order.UserId
		logPlanID = order.PlanId
		logMoney = order.Money
		logPaymentMethod = order.PaymentMethod
		return nil
	})
	if err != nil {
		return err
	}
	if logUserID > 0 {
		service.TryEnsureInvitationEntitlementForPaidUser(logUserID)
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
