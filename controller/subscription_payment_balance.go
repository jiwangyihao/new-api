package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SubscriptionBalancePayRequest struct {
	PlanId         int    `json:"plan_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

func SubscriptionRequestBalance(c *gin.Context) {
	var req SubscriptionBalancePayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.IdempotencyKey == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if msg := validatePurchasableSubscriptionPlan(plan); msg != "" {
		common.ApiErrorMsg(c, msg)
		return
	}
	if strings.ToUpper(strings.TrimSpace(plan.Currency)) != "CNY" {
		common.ApiErrorMsg(c, "账户余额仅支持 CNY 套餐")
		return
	}
	amount, err := subscriptionBalancePayAmount(plan.PriceAmount)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	userId := c.GetInt("id")
	if userId <= 0 {
		common.ApiErrorMsg(c, "用户未登录")
		return
	}

	planLockKey := subscriptionBalancePlanLockKey(userId, req.PlanId)
	LockOrder(planLockKey)
	defer UnlockOrder(planLockKey)
	tradeNo := subscriptionBalanceTradeNo(userId, req.IdempotencyKey)
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	order, created, err := createBalanceSubscriptionOrder(userId, plan, tradeNo, amount)
	if err != nil {
		if strings.Contains(err.Error(), "余额不足") {
			common.ApiErrorMsg(c, "余额不足")
			return
		}
		common.ApiError(c, err)
		return
	}
	if created {
		service.TryEnsureInvitationEntitlementForPaidUser(userId)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "success", "data": order})
}

func subscriptionBalancePayAmount(price float64) (int, error) {
	if price <= 0 {
		return 0, errors.New("套餐不可购买")
	}
	amount, err := model.AccountBalanceCentsFromCNY(decimal.NewFromFloat(price))
	if err != nil {
		return 0, errors.New("套餐价格无效")
	}
	return amount, nil
}

func subscriptionBalancePlanLockKey(userId int, planId int) string {
	return fmt.Sprintf("BALSUBUSR%dPLAN%d", userId, planId)
}

func subscriptionBalanceTradeNo(userId int, idempotencyKey string) string {
	return fmt.Sprintf("BALSUBUSR%dNO%s", userId, common.Sha1([]byte(idempotencyKey)))
}

func createBalanceSubscriptionOrder(userId int, plan *model.SubscriptionPlan, tradeNo string, amount int) (*model.SubscriptionOrder, bool, error) {
	var order model.SubscriptionOrder
	created := false
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		created, err = createBalanceSubscriptionOrderTx(tx, userId, plan, tradeNo, amount, &order)
		return err
	}); err != nil {
		return nil, false, err
	}
	if created {
		_ = model.InvalidateUserCache(userId)
		model.RecordLog(userId, model.LogTypeTopup, "账户余额购买订阅套餐："+plan.Title)
	}
	return &order, created, nil
}

func createBalanceSubscriptionOrderTx(tx *gorm.DB, userId int, plan *model.SubscriptionPlan, tradeNo string, amount int, order *model.SubscriptionOrder) (bool, error) {
	if tx == nil || order == nil || plan == nil {
		return false, errors.New("invalid balance subscription order")
	}
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("trade_no = ?", tradeNo).First(order).Error; err == nil {
		if order.UserId != userId || order.PlanId != plan.Id || order.PaymentProvider != model.PaymentProviderBalance {
			return false, model.ErrPaymentMethodMismatch
		}
		return false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}

	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", userId).First(&user).Error; err != nil {
		return false, err
	}

	if err := validateSubscriptionPurchaseLimitTx(tx, userId, plan); err != nil {
		return false, err
	}

	if err := model.DeductUserAccountBalanceTx(tx, userId, amount); err != nil {
		return false, err
	}

	now := common.GetTimestamp()
	*order = model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           plan.PriceAmount,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodAccountBalance,
		PaymentProvider: model.PaymentProviderBalance,
		CreateTime:      now,
		Status:          common.TopUpStatusPending,
	}
	if err := tx.Create(order).Error; err != nil {
		return false, err
	}
	if _, err := model.CompleteSubscriptionOrderTx(tx, order, "", model.PaymentMethodAccountBalance); err != nil {
		return false, err
	}
	return true, nil

}
