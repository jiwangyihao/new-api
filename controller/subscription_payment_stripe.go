package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	stripeprice "github.com/stripe/stripe-go/v81/price"
	"github.com/thanhpk/randstr"
)

const stripeSubscriptionProductMetadataKey = "provider_product_id"
const stripeSubscriptionReferenceMetadataKey = "new_api_subscription_trade_no"

type stripeTestHandle interface {
	Helper()
	Cleanup(func())
}

type SubscriptionStripePayRequest struct {
	PlanId       int    `json:"plan_id"`
	PurchaseMode string `json:"purchase_mode"`
}

type StripeSubscriptionCheckoutResult struct {
	URL string
}

type stripeSubscriptionCheckoutFunc func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error)

var createStripeSubscriptionCheckout stripeSubscriptionCheckoutFunc = defaultStripeSubscriptionCheckout

type stripeSubscriptionPriceSnapshotFunc func(priceId string) (int64, string, error)

var loadStripeSubscriptionPriceSnapshot stripeSubscriptionPriceSnapshotFunc = stripeSubscriptionPriceSnapshot

func SetStripeSubscriptionCheckoutForTest(t stripeTestHandle, fake func(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error)) {
	t.Helper()
	old := createStripeSubscriptionCheckout
	createStripeSubscriptionCheckout = stripeSubscriptionCheckoutFunc(fake)
	t.Cleanup(func() { createStripeSubscriptionCheckout = old })
}

func SetStripeSubscriptionPriceSnapshotForTest(t stripeTestHandle, fake func(priceId string) (int64, string, error)) {
	t.Helper()
	old := loadStripeSubscriptionPriceSnapshot
	loadStripeSubscriptionPriceSnapshot = stripeSubscriptionPriceSnapshotFunc(fake)
	t.Cleanup(func() { loadStripeSubscriptionPriceSnapshot = old })
}

func SubscriptionRequestStripePay(c *gin.Context) {
	var req SubscriptionStripePayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	purchaseMode, err := model.NormalizeSubscriptionPurchaseMode(req.PurchaseMode)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
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
	if plan.StripePriceId == "" {
		common.ApiErrorMsg(c, "该套餐未配置 StripePriceId")
		return
	}
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		common.ApiErrorMsg(c, "Stripe 未配置或密钥无效")
		return
	}
	if setting.StripeWebhookSecret == "" {
		common.ApiErrorMsg(c, "Stripe Webhook 未配置")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

	reference := fmt.Sprintf("sub-stripe-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "sub_ref_" + common.Sha1([]byte(reference))
	stripe.Key = setting.StripeApiSecret
	amountCents, currency, err := loadStripeSubscriptionPriceSnapshot(plan.StripePriceId)
	if err != nil {
		common.ApiErrorMsg(c, "Stripe 套餐价格快照无效")
		return
	}
	amountCents, currency = normalizeProviderCheckoutSnapshot(amountCents, currency)
	if amountCents <= 0 || currency == "" {
		common.ApiErrorMsg(c, "Stripe 套餐价格快照无效")
		return
	}
	entitlementSnapshot, err := prepareExternalSubscriptionEntitlementSnapshot(plan, purchaseMode)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	serializedSnapshot, err := marshalExternalSubscriptionEntitlementSnapshot(entitlementSnapshot, model.PaymentProviderStripe, plan.StripePriceId, model.PaymentMethodStripe, amountCents, currency)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	creditGrantAmount, creditTargetPlanID := entitlementSnapshot.CreditGrantIdentity()
	order := &model.SubscriptionOrder{
		UserId:              userId,
		PlanId:              plan.Id,
		Money:               plan.PriceAmount,
		AmountCents:         amountCents,
		Currency:            currency,
		CreditGrantAmount:   creditGrantAmount,
		CreditTargetPlanID:  creditTargetPlanID,
		TradeNo:             referenceId,
		PaymentMethod:       model.PaymentMethodStripe,
		PaymentProvider:     model.PaymentProviderStripe,
		CreateTime:          time.Now().Unix(),
		Status:              common.TopUpStatusPending,
		EntitlementSnapshot: serializedSnapshot,
	}
	if err := order.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	checkout, err := createStripeSubscriptionCheckout(referenceId, user.StripeCustomer, user.Email, plan.StripePriceId)
	if err != nil || strings.TrimSpace(checkout.URL) == "" {
		_ = model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderStripe)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 订阅支付链接创建失败 trade_no=%s plan_id=%d error=%q", referenceId, plan.Id, err.Error()))
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": checkout.URL,
			"order_id": referenceId,
		},
	})
}

func failPendingStripeSubscriptionOrder(tradeNo string) (bool, error) {
	order := model.GetSubscriptionOrderByTradeNo(strings.TrimSpace(tradeNo))
	if order == nil {
		return false, nil
	}
	if order.PaymentProvider != model.PaymentProviderStripe {
		return true, model.ErrPaymentMethodMismatch
	}
	return true, model.FailSubscriptionOrder(order.TradeNo, model.PaymentProviderStripe)
}

func defaultStripeSubscriptionCheckout(referenceId string, customerId string, email string, priceId string) (StripeSubscriptionCheckoutResult, error) {
	stripe.Key = setting.StripeApiSecret
	params := stripeSubscriptionCheckoutParams(referenceId, customerId, email, priceId)
	result, err := session.New(params)
	if err != nil {
		return StripeSubscriptionCheckoutResult{}, err
	}
	return StripeSubscriptionCheckoutResult{URL: result.URL}, nil
}

func stripeSubscriptionCheckoutParams(referenceId string, customerId string, email string, priceId string) *stripe.CheckoutSessionParams {
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(paymentReturnPath("/console/topup")),
		CancelURL:         stripe.String(paymentReturnPath("/console/topup")),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceId),
				Quantity: stripe.Int64(1),
			},
		},
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
	}
	params.AddMetadata(stripeSubscriptionProductMetadataKey, priceId)
	params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{Metadata: map[string]string{
		stripeSubscriptionReferenceMetadataKey: referenceId,
	}}
	if customerId == "" {
		if email != "" {
			params.CustomerEmail = stripe.String(email)
		}
		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}
	return params
}

func stripeSubscriptionPriceSnapshot(priceId string) (int64, string, error) {
	priceId = strings.TrimSpace(priceId)
	if priceId == "" {
		return 0, "", errors.New("Stripe price id is empty")
	}
	stripePrice, err := stripeprice.Get(priceId, nil)
	if err != nil {
		return 0, "", err
	}
	if stripePrice == nil || stripePrice.UnitAmount <= 0 {
		return 0, "", errors.New("Stripe price amount is invalid")
	}
	currency := normalizeProviderSnapshotCurrency(string(stripePrice.Currency))
	if currency == "" {
		return 0, "", errors.New("Stripe price currency is invalid")
	}
	return stripePrice.UnitAmount, currency, nil
}

func genStripeSubscriptionLink(referenceId string, customerId string, email string, priceId string) (string, error) {
	result, err := defaultStripeSubscriptionCheckout(referenceId, customerId, email, priceId)
	if err != nil {
		return "", err
	}
	return result.URL, nil
}
