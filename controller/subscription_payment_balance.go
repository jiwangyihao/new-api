package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
	amountCents, snapshotCurrency, ok := model.SubscriptionPlanAmountSnapshot(plan)
	if !ok || snapshotCurrency != "CNY" {
		common.ApiErrorMsg(c, "套餐价格无效")
		return
	}
	if amountCents > int64(math.MaxInt) {
		common.ApiErrorMsg(c, "套餐价格无效")
		return
	}
	amount := int(amountCents)

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

	order, completion, _, err := createBalanceSubscriptionOrder(userId, plan, tradeNo, amount)
	if err != nil {
		if strings.Contains(err.Error(), "余额不足") {
			common.ApiErrorMsg(c, "余额不足")
			return
		}
		common.ApiError(c, err)
		return
	}
	if completion != nil {
		if err := handleInvitationRewardForCompletedSubscriptionOrder(order.Id); err != nil {
			common.SysLog(fmt.Sprintf("failed to handle balance subscription invitation reward order_id=%d: %s", order.Id, err.Error()))
		}
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

func createBalanceSubscriptionOrder(userId int, plan *model.SubscriptionPlan, tradeNo string, amount int) (*model.SubscriptionOrder, *model.SubscriptionOrderCompletionResult, bool, error) {
	var order model.SubscriptionOrder
	var completion *model.SubscriptionOrderCompletionResult
	created := false
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		created, completion, err = createBalanceSubscriptionOrderTx(tx, userId, plan, tradeNo, amount, &order)
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

func createBalanceSubscriptionOrderTx(tx *gorm.DB, userId int, plan *model.SubscriptionPlan, tradeNo string, amount int, order *model.SubscriptionOrder) (bool, *model.SubscriptionOrderCompletionResult, error) {
	if tx == nil || order == nil || plan == nil {
		return false, nil, errors.New("invalid balance subscription order")
	}
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("trade_no = ?", tradeNo).First(order).Error; err == nil {
		if order.UserId != userId || order.PlanId != plan.Id || order.PaymentProvider != model.PaymentProviderBalance {
			return false, nil, model.ErrPaymentMethodMismatch
		}
		if order.Status == common.TopUpStatusPending {
			return false, nil, nil
		}
		completion, err := model.CompleteSubscriptionOrderTx(tx, order, "", model.PaymentMethodAccountBalance)
		return false, completion, err
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil, err
	}

	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", userId).First(&user).Error; err != nil {
		return false, nil, err
	}

	if err := validateSubscriptionPurchaseLimitTx(tx, userId, plan); err != nil {
		return false, nil, err
	}

	if err := model.DeductUserAccountBalanceTx(tx, userId, amount); err != nil {
		return false, nil, err
	}

	now := common.GetTimestamp()
	*order = model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           plan.PriceAmount,
		TradeNo:         tradeNo,
		AmountCents:     int64(amount),
		Currency:        "CNY",
		PaymentMethod:   model.PaymentMethodAccountBalance,
		PaymentProvider: model.PaymentProviderBalance,
		CreateTime:      now,
		Status:          common.TopUpStatusPending,
	}
	if err := tx.Create(order).Error; err != nil {
		return false, nil, err
	}
	completion, err := model.CompleteSubscriptionOrderTx(tx, order, "", model.PaymentMethodAccountBalance)
	if err != nil {
		return false, nil, err
	}
	return true, completion, nil

}
