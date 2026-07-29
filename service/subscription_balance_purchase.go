package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ReplayBalanceSubscriptionOrder(userId int, planId int, tradeNo string, purchaseMode string) (*model.SubscriptionOrder, *model.SubscriptionOrderCompletionResult, bool, error) {
	var order model.SubscriptionOrder
	var completion *model.SubscriptionOrderCompletionResult
	found := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("trade_no = ?", tradeNo).First(&order)
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		if query.Error != nil {
			return query.Error
		}
		found = true
		var err error
		completion, err = completeExistingBalanceSubscriptionOrderTx(tx, &order, userId, planId, purchaseMode)
		return err
	})
	if err != nil {
		return nil, nil, found, err
	}
	if !found {
		return nil, nil, false, nil
	}
	return &order, completion, true, nil
}

func completeExistingBalanceSubscriptionOrderTx(tx *gorm.DB, order *model.SubscriptionOrder, userId int, planId int, purchaseMode string) (*model.SubscriptionOrderCompletionResult, error) {
	if tx == nil || order == nil {
		return nil, errors.New("invalid balance subscription order")
	}
	if order.UserId != userId || order.PlanId != planId || order.PaymentProvider != model.PaymentProviderBalance {
		return nil, model.ErrPaymentMethodMismatch
	}
	storedMode := model.SubscriptionPurchaseModeTimed
	if strings.TrimSpace(order.EntitlementSnapshot) != "" {
		storedSnapshot, err := model.UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(storedSnapshot.PurchaseMode) != "" {
			storedMode, err = model.NormalizeSubscriptionPurchaseMode(storedSnapshot.PurchaseMode)
			if err != nil {
				return nil, err
			}
		}
	}
	if storedMode != purchaseMode {
		return nil, errors.New("幂等键已绑定其他购买模式或订单快照")
	}
	if order.Status == common.TopUpStatusPending {
		return nil, model.ErrSubscriptionOrderStatusInvalid
	}
	return model.CompleteSubscriptionOrderTx(tx, order, "", model.PaymentMethodAccountBalance)
}

func CreateBalanceSubscriptionOrder(userId int, plan *model.SubscriptionPlan, tradeNo string, amount int, purchaseMode string, entitlementSnapshot string) (*model.SubscriptionOrder, *model.SubscriptionOrderCompletionResult, bool, error) {
	var order model.SubscriptionOrder
	var completion *model.SubscriptionOrderCompletionResult
	created := false
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		created, completion, err = CreateBalanceSubscriptionOrderTx(tx, userId, plan, tradeNo, amount, purchaseMode, entitlementSnapshot, &order)
		return err
	}); err != nil {
		return nil, nil, false, err
	}
	if created {
		_ = model.InvalidateUserCache(userId)
		model.RecordLog(userId, model.LogTypeTopup, "账户余额购买订阅套餐："+plan.Title)
	}
	return &order, completion, created, nil
}

func CreateBalanceSubscriptionOrderTx(tx *gorm.DB, userId int, plan *model.SubscriptionPlan, tradeNo string, amount int, purchaseMode string, entitlementSnapshot string, order *model.SubscriptionOrder) (bool, *model.SubscriptionOrderCompletionResult, error) {
	if tx == nil || order == nil || plan == nil {
		return false, nil, errors.New("invalid balance subscription order")
	}
	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "setting").Where("id = ?", userId).First(&user).Error; err != nil {
		return false, nil, err
	}

	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("trade_no = ?", tradeNo).First(order).Error; err == nil {
		completion, err := completeExistingBalanceSubscriptionOrderTx(tx, order, userId, plan.Id, purchaseMode)
		return false, completion, err
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil, err
	}

	if err := model.DeductUserAccountBalanceTx(tx, userId, amount); err != nil {
		return false, nil, err
	}

	now := common.GetTimestamp()
	*order = model.SubscriptionOrder{
		UserId:              userId,
		PlanId:              plan.Id,
		Money:               plan.PriceAmount,
		TradeNo:             tradeNo,
		AmountCents:         int64(amount),
		Currency:            "CNY",
		PaymentMethod:       model.PaymentMethodAccountBalance,
		PaymentProvider:     model.PaymentProviderBalance,
		CreateTime:          now,
		Status:              common.TopUpStatusPending,
		EntitlementSnapshot: entitlementSnapshot,
	}
	if err := tx.Create(order).Error; err != nil {
		return false, nil, err
	}
	completion, err := model.CompleteSubscriptionOrderTx(tx, order, "", model.PaymentMethodAccountBalance)
	if err != nil {
		return false, nil, err
	}
	if err := model.SetUserLastSubscriptionPurchaseModeTx(tx, userId, purchaseMode); err != nil {
		return false, nil, err
	}
	return true, completion, nil
}
