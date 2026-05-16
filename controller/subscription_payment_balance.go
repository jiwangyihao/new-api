package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
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
	quotaAmount := decimal.NewFromFloat(price).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Round(0).IntPart()
	if quotaAmount <= 0 || quotaAmount > int64(math.MaxInt) {
		return 0, errors.New("套餐价格无效")
	}
	return int(quotaAmount), nil
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
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("trade_no = ?", tradeNo).First(&order).Error; err == nil {
			if order.UserId != userId || order.PlanId != plan.Id || order.PaymentProvider != model.PaymentProviderBalance {
				return model.ErrPaymentMethodMismatch
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if plan.MaxPurchasePerUser > 0 {
			var count int64
			if err := tx.Model(&model.UserSubscription{}).
				Where("user_id = ? AND plan_id = ?", userId, plan.Id).
				Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(plan.MaxPurchasePerUser) {
				return errors.New("已达到该套餐购买上限")
			}
		}

		if err := model.DeductUserAccountBalanceTx(tx, userId, amount); err != nil {
			return err
		}

		now := common.GetTimestamp()
		order = model.SubscriptionOrder{
			UserId:          userId,
			PlanId:          plan.Id,
			Money:           plan.PriceAmount,
			TradeNo:         tradeNo,
			PaymentMethod:   model.PaymentMethodAccountBalance,
			PaymentProvider: model.PaymentProviderBalance,
			CreateTime:      now,
			Status:          common.TopUpStatusPending,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		if _, err := model.CompleteSubscriptionOrderTx(tx, &order, "", model.PaymentMethodAccountBalance); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if created {
		_ = model.InvalidateUserCache(userId)
		if strings.TrimSpace(plan.UpgradeGroup) != "" {
			_ = model.UpdateUserGroupCache(userId, plan.UpgradeGroup)
		}
		model.RecordLog(userId, model.LogTypeTopup, "账户余额购买订阅套餐："+plan.Title)
	}
	return &order, created, nil
}
