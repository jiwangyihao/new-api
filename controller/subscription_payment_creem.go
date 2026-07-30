package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/thanhpk/randstr"
)

type SubscriptionCreemPayRequest struct {
	PlanId       int    `json:"plan_id"`
	PurchaseMode string `json:"purchase_mode"`
}

type CreemSubscriptionCheckoutResult struct {
	URL         string
	AmountCents int64
	Currency    string
}

type creemSubscriptionCheckoutFunc func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error)

var createCreemSubscriptionCheckout creemSubscriptionCheckoutFunc = defaultCreemSubscriptionCheckout

type creemSubscriptionProductSnapshotFunc func(ctx context.Context, productID string) (int64, string, error)

var loadCreemSubscriptionProductSnapshot creemSubscriptionProductSnapshotFunc = defaultCreemSubscriptionProductSnapshot

func SetCreemSubscriptionCheckoutForTest(t stripeTestHandle, fake func(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error)) {
	t.Helper()
	old := createCreemSubscriptionCheckout
	createCreemSubscriptionCheckout = creemSubscriptionCheckoutFunc(fake)
	t.Cleanup(func() { createCreemSubscriptionCheckout = old })
}

func SetCreemSubscriptionProductSnapshotForTest(t stripeTestHandle, fake func(ctx context.Context, productID string) (int64, string, error)) {
	t.Helper()
	old := loadCreemSubscriptionProductSnapshot
	loadCreemSubscriptionProductSnapshot = creemSubscriptionProductSnapshotFunc(fake)
	t.Cleanup(func() { loadCreemSubscriptionProductSnapshot = old })
}

func SubscriptionRequestCreemPay(c *gin.Context) {
	var req SubscriptionCreemPayRequest

	// Keep body for debugging consistency (like RequestCreemPay)
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 订阅支付请求读取失败 error=%q", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "read query error"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
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
	if plan.CreemProductId == "" {
		common.ApiErrorMsg(c, "该套餐未配置 CreemProductId")
		return
	}
	if setting.CreemWebhookSecret == "" && !setting.CreemTestMode {
		common.ApiErrorMsg(c, "Creem Webhook 未配置")
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

	amountCents, currency, err := loadCreemSubscriptionProductSnapshot(c.Request.Context(), plan.CreemProductId)
	if err != nil {
		common.ApiErrorMsg(c, "Creem 套餐价格快照无效")
		return
	}
	amountCents, currency = normalizeProviderCheckoutSnapshot(amountCents, currency)
	if amountCents <= 0 || currency == "" {
		common.ApiErrorMsg(c, "Creem 套餐价格快照无效")
		return
	}
	entitlementSnapshot, err := prepareExternalSubscriptionEntitlementSnapshot(plan, purchaseMode)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	serializedSnapshot, err := marshalExternalSubscriptionEntitlementSnapshot(entitlementSnapshot, model.PaymentProviderCreem, plan.CreemProductId, model.PaymentMethodCreem, amountCents, currency)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	reference := "sub-creem-ref-" + randstr.String(6)
	referenceId := "sub_ref_" + common.Sha1([]byte(reference+time.Now().String()+user.Username))
	product := &CreemProduct{
		ProductId: plan.CreemProductId,
		Name:      plan.Title,
		Price:     plan.PriceAmount,
		Currency:  currency,
		Quota:     0,
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
		PaymentMethod:       model.PaymentMethodCreem,
		PaymentProvider:     model.PaymentProviderCreem,
		CreateTime:          time.Now().Unix(),
		Status:              common.TopUpStatusPending,
		EntitlementSnapshot: serializedSnapshot,
	}
	if err := order.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	checkout, err := createCreemSubscriptionCheckout(referenceId, product, user.Email, user.Username)
	if err != nil || strings.TrimSpace(checkout.URL) == "" {
		_ = model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderCreem)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 订阅支付链接创建失败 trade_no=%s product_id=%s error=%q", referenceId, product.ProductId, err.Error()))
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if checkout.AmountCents > 0 || strings.TrimSpace(checkout.Currency) != "" {
		checkoutAmountCents, checkoutCurrency := normalizeProviderCheckoutSnapshot(checkout.AmountCents, checkout.Currency)
		if checkoutAmountCents != amountCents || checkoutCurrency != currency {
			_ = model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderCreem)
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Creem 套餐价格快照不匹配"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url": checkout.URL,
			"order_id":     referenceId,
		},
	})

}

type creemProductSnapshotResponse struct {
	ID       string `json:"id"`
	Price    int64  `json:"price"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
}

func defaultCreemSubscriptionProductSnapshot(ctx context.Context, productID string) (int64, string, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" || strings.TrimSpace(setting.CreemApiKey) == "" {
		return 0, "", fmt.Errorf("Creem 产品或 API Key 未配置")
	}
	baseURL := "https://api.creem.io"
	if setting.CreemTestMode {
		baseURL = "https://test-api.creem.io"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/products/"+url.PathEscape(productID), nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("x-api-key", setting.CreemApiKey)
	resp, err := creemHTTPClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	if resp.StatusCode/100 != 2 {
		return 0, "", fmt.Errorf("Creem 产品查询返回 HTTP %d", resp.StatusCode)
	}
	var product creemProductSnapshotResponse
	if err := common.Unmarshal(body, &product); err != nil {
		return 0, "", err
	}
	currency := normalizeProviderSnapshotCurrency(product.Currency)
	if strings.TrimSpace(product.ID) != productID || !strings.EqualFold(strings.TrimSpace(product.Status), "active") || product.Price <= 0 || currency == "" {
		return 0, "", fmt.Errorf("Creem 产品价格快照无效")
	}
	return product.Price, currency, nil
}

func defaultCreemSubscriptionCheckout(referenceId string, product *CreemProduct, email string, username string) (CreemSubscriptionCheckoutResult, error) {
	checkoutURL, err := genCreemLink(context.Background(), referenceId, product, email, username)
	if err != nil {
		return CreemSubscriptionCheckoutResult{}, err
	}
	return CreemSubscriptionCheckoutResult{URL: checkoutURL}, nil
}
