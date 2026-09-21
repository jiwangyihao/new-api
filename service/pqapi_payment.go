package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func CreatePQAPITopUpPaymentOrder(order *model.TopUp) error {
	if order == nil || order.UserId <= 0 || order.TradeNo == "" || order.PaymentProvider != model.PaymentProviderPQAPI || order.Status != common.TopUpStatusPending {
		return errors.New("invalid PQAPI top-up payment order")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		return model.CreatePaymentProviderOrderTx(tx, &model.PaymentProviderOrder{
			Provider: model.PaymentProviderPQAPI, OrderKind: model.PaymentOrderKindTopUp,
			LocalOrderID: order.Id, TradeNo: order.TradeNo, UserID: order.UserId,
		})
	})
}
