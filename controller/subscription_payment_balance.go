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
)

type SubscriptionBalancePayRequest struct {
	PlanId         int    `json:"plan_id"`
	PurchaseMode   string `json:"purchase_mode"`
	IdempotencyKey string `json:"idempotency_key"`
}

type SubscriptionBalancePayResponseData struct {
	Order         *model.SubscriptionOrder        `json:"order"`
	PurchaseMode  string                          `json:"purchase_mode"`
	CreditBalance *model.CreditBalanceGrantResult `json:"credit_balance,omitempty"`
}

func SubscriptionRequestBalance(c *gin.Context) {
	var req SubscriptionBalancePayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	purchaseMode, err := model.NormalizeSubscriptionPurchaseMode(req.PurchaseMode)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.IdempotencyKey == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	userId := c.GetInt("id")
	if userId <= 0 {
		common.ApiErrorMsg(c, "用户未登录")
		return
	}

	tradeNo := subscriptionBalanceTradeNo(userId, req.IdempotencyKey)
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	order, completion, replayed, err := service.ReplayBalanceSubscriptionOrder(userId, req.PlanId, tradeNo, purchaseMode)
	if err != nil {
		writeSubscriptionBalanceOrderError(c, err)
		return
	}
	if replayed {
		writeSubscriptionBalanceOrderSuccess(c, order, completion)
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
	targetCreditPlanID := 0
	if purchaseMode == model.SubscriptionPurchaseModeCreditBalance {
		creditPlan, err := model.GetCreditBalancePlanTx(model.DB)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if err := model.ValidateCreditBalancePurchaseOption(plan, creditPlan); err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		targetCreditPlanID = creditPlan.Id
	}
	if strings.ToUpper(strings.TrimSpace(plan.Currency)) != "CNY" {
		common.ApiErrorMsg(c, "账户余额仅支持 CNY 套餐")
		return
	}
	amountCents, snapshotCurrency, ok := model.SubscriptionPlanAmountSnapshot(plan)
	if !ok || snapshotCurrency != "CNY" || amountCents > int64(math.MaxInt) {
		common.ApiErrorMsg(c, "套餐价格无效")
		return
	}
	entitlementSnapshot := model.NewSubscriptionEntitlementSnapshot(plan, purchaseMode, targetCreditPlanID)
	entitlementSnapshot.MaxPurchasePerUser = 0
	snapshot, err := model.MarshalSubscriptionEntitlementSnapshot(entitlementSnapshot)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	order, completion, _, err = service.CreateBalanceSubscriptionOrder(userId, plan, tradeNo, int(amountCents), purchaseMode, snapshot)
	if err != nil {
		writeSubscriptionBalanceOrderError(c, err)
		return
	}
	writeSubscriptionBalanceOrderSuccess(c, order, completion)
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

func writeSubscriptionBalanceOrderError(c *gin.Context, err error) {
	if strings.Contains(err.Error(), "余额不足") {
		common.ApiErrorMsg(c, "余额不足")
		return
	}
	common.ApiError(c, err)
}

func writeSubscriptionBalanceOrderSuccess(c *gin.Context, order *model.SubscriptionOrder, completion *model.SubscriptionOrderCompletionResult) {
	if order == nil || completion == nil || completion.PurchaseMode == "" {
		common.ApiError(c, errors.New("订阅订单履约结果无效"))
		return
	}
	if completion.PurchaseMode == model.SubscriptionPurchaseModeTimed {
		if err := handleInvitationRewardForCompletedSubscriptionOrder(order.Id); err != nil {
			common.SysLog(fmt.Sprintf("failed to handle balance subscription invitation reward order_id=%d: %s", order.Id, err.Error()))
		}
	}
	data := SubscriptionBalancePayResponseData{
		Order:         order,
		PurchaseMode:  completion.PurchaseMode,
		CreditBalance: completion.CreditBalance,
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "success", "data": data})
}
