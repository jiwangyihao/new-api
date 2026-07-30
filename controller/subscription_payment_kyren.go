package controller

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const kyrenWebhookTolerance = 5 * time.Minute

const kyrenProductStatusActive = "ACTIVE"

type kyrenProductSyncRequest struct {
	Mode string `json:"mode"`
}

type kyrenProductSyncResponse struct {
	ProductID  string `json:"product_id"`
	Status     string `json:"status"`
	Price      string `json:"price"`
	Currency   string `json:"currency"`
	Synced     bool   `json:"synced"`
	LocalError string `json:"local_error,omitempty"`
}

type SubscriptionKyrenPayRequest struct {
	PlanId       int    `json:"plan_id"`
	PurchaseMode string `json:"purchase_mode"`
}

type kyrenWebhookEvent struct {
	ID   string           `json:"id"`
	Type string           `json:"type"`
	Data kyrenWebhookData `json:"data"`
}

type kyrenWebhookData struct {
	ID             string            `json:"id"`
	OrderID        string            `json:"order_id"`
	ProductID      string            `json:"productId"`
	ProductIDSnake string            `json:"product_id"`
	Amount         string            `json:"amount"`
	Currency       string            `json:"currency"`
	Metadata       map[string]string `json:"metadata"`
}

func (d kyrenWebhookData) kyrenOrderID() string {
	if strings.TrimSpace(d.OrderID) != "" {
		return d.OrderID
	}
	return d.ID
}

func (d kyrenWebhookData) kyrenProductID() string {
	if strings.TrimSpace(d.ProductID) != "" {
		return d.ProductID
	}
	return d.ProductIDSnake
}

func verifyKyrenWebhookSignature(payload []byte, signature string, timestamp string, secret string) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return false
	}
	timestamp = strings.TrimSpace(timestamp)
	signature = strings.TrimSpace(signature)
	if timestamp == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	millis, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || millis <= 0 {
		return false
	}
	eventTime := time.UnixMilli(millis)
	now := time.Now()
	if now.Sub(eventTime) > kyrenWebhookTolerance || eventTime.Sub(now) > kyrenWebhookTolerance {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	return hmac.Equal(provided, mac.Sum(nil))
}

func SubscriptionRequestKyrenPay(c *gin.Context) {
	var req SubscriptionKyrenPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	purchaseMode, err := model.NormalizeSubscriptionPurchaseMode(req.PurchaseMode)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := ensureKyrenPaymentConfigured(); err != nil {
		common.ApiError(c, err)
		return
	}
	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiErrorMsg(c, "套餐不存在")
		return
	}
	if msg := validatePurchasableSubscriptionPlan(plan); msg != "" {
		common.ApiErrorMsg(c, msg)
		return
	}
	if normalizeSubscriptionPlanCurrency(plan.Currency) != kyrenCurrencyCNY {
		common.ApiErrorMsg(c, "Kyren 仅支持 CNY 套餐")
		return
	}
	productID := strings.TrimSpace(plan.KyrenProductId)
	if productID == "" {
		common.ApiErrorMsg(c, "套餐未绑定 Kyren 产品")
		return
	}
	userID := c.GetInt("id")
	user, err := model.GetUserById(userID, false)
	if err != nil || user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	expectedAmount, err := formatKyrenAmountFromFloat(plan.PriceAmount)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	client, err := newKyrenClientForController()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateKyrenRemoteProduct(c.Request.Context(), client, productID, expectedAmount, kyrenCurrencyCNY); err != nil {
		common.ApiError(c, err)
		return
	}
	tradeNo := fmt.Sprintf("KYSUBUSR%dNO%s%d", userID, common.GetRandomString(6), time.Now().Unix())
	kyrenSnapshot, err := model.MarshalKyrenPaymentSnapshot(model.KyrenPaymentSnapshot{ProductID: productID, Amount: expectedAmount, Currency: kyrenCurrencyCNY})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	amountCents, currency, ok := model.SubscriptionPlanAmountSnapshot(plan)
	if !ok || currency != kyrenCurrencyCNY {
		common.ApiError(c, errors.New("Kyren 订阅金额快照无效"))
		return
	}
	preparedSnapshot, err := prepareExternalSubscriptionEntitlementSnapshot(plan, purchaseMode)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	entitlementSnapshot, err := marshalExternalSubscriptionEntitlementSnapshot(preparedSnapshot, model.PaymentProviderKyren, productID, model.PaymentMethodKyren, amountCents, currency)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	creditGrantAmount, creditTargetPlanID := preparedSnapshot.CreditGrantIdentity()
	order := &model.SubscriptionOrder{
		UserId:              userID,
		PlanId:              plan.Id,
		Money:               plan.PriceAmount,
		AmountCents:         amountCents,
		Currency:            currency,
		CreditGrantAmount:   creditGrantAmount,
		CreditTargetPlanID:  creditTargetPlanID,
		TradeNo:             tradeNo,
		PaymentMethod:       model.PaymentMethodKyren,
		PaymentProvider:     model.PaymentProviderKyren,
		Status:              common.TopUpStatusPending,
		CreateTime:          common.GetTimestamp(),
		KyrenSnapshot:       kyrenSnapshot,
		EntitlementSnapshot: entitlementSnapshot,
	}
	if err := order.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	checkout, err := client.createCheckout(c.Request.Context(), kyrenCreateCheckoutRequest{
		ProductID:     productID,
		SuccessURL:    paymentReturnPath("/console/topup?pay=success"),
		CancelURL:     paymentReturnPath("/console/topup?pay=cancel"),
		CustomerEmail: user.Email,
		CustomerName:  user.Username,
		Metadata: map[string]string{
			"kind":     "subscription",
			"trade_no": tradeNo,
			"user_id":  strconv.Itoa(userID),
			"plan_id":  strconv.Itoa(plan.Id),
		},
	})
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderKyren)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Kyren 创建订阅 Checkout 失败 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if checkout == nil || strings.TrimSpace(checkout.URL) == "" {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderKyren)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	common.ApiSuccess(c, gin.H{"checkout_url": checkout.URL, "url": checkout.URL, "order_id": tradeNo})
}

func KyrenWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if !verifyKyrenWebhookSignature(payload, kyrenWebhookSignatureHeader(c), kyrenWebhookTimestampHeader(c), setting.KyrenWebhookSecret) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Kyren webhook 验签失败 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var event kyrenWebhookEvent
	if err := common.Unmarshal(payload, &event); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Kyren webhook 解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	metadata := event.Data.Metadata
	tradeNo := strings.TrimSpace(metadata["trade_no"])
	kind := strings.TrimSpace(metadata["kind"])
	if tradeNo == "" || kind == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Kyren webhook 缺少 metadata event_id=%s event_type=%s", event.ID, event.Type))
		c.Status(http.StatusOK)
		return
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	summary := kyrenControlledProviderPayload(event)
	if !isSupportedKyrenWebhookKind(kind) {
		recordKyrenUnsupportedKindManualAction(kind, tradeNo)
		c.Status(http.StatusOK)
		return
	}
	switch event.Type {
	case "order.paid":
		if kind == "subscription" {
			if err := handleKyrenSubscriptionPaid(tradeNo, event, summary); err != nil {
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("Kyren 订阅入账未完成 trade_no=%s error=%q", tradeNo, err.Error()))
				if isKyrenRetryableFulfillmentError(err) {
					c.AbortWithStatus(http.StatusInternalServerError)
					return
				}
			}
		} else if kind == "topup" {
			if err := completeKyrenTopUpWithSnapshot(tradeNo, event, c.ClientIP()); err != nil {
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("Kyren 充值入账未完成 trade_no=%s error=%q", tradeNo, err.Error()))
				if isKyrenRetryableFulfillmentError(err) {
					c.AbortWithStatus(http.StatusInternalServerError)
					return
				}
			}
		}
	case "order.closed":
		if kind == "subscription" {
			_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderKyren)
		} else if kind == "topup" {
			_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderKyren, common.TopUpStatusExpired)
		}
	case "order.refunded":
		recordKyrenRefundManualAction(kind, tradeNo)
	default:
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Kyren webhook 忽略事件 event_type=%s trade_no=%s", event.Type, tradeNo))
	}
	c.Status(http.StatusOK)
}

func kyrenWebhookSignatureHeader(c *gin.Context) string {
	if signature := c.GetHeader("Kyren-Signature"); signature != "" {
		return signature
	}
	return c.GetHeader("X-Kyren-Signature")
}

func kyrenWebhookTimestampHeader(c *gin.Context) string {
	if timestamp := c.GetHeader("Kyren-Timestamp"); timestamp != "" {
		return timestamp
	}
	return c.GetHeader("X-Kyren-Timestamp")
}

func ensureKyrenPaymentConfigured() error {
	if strings.TrimSpace(setting.KyrenApiKey) == "" {
		return errors.New("Kyren API Key 未配置")
	}
	if strings.TrimSpace(setting.KyrenWebhookSecret) == "" {
		return errors.New("Kyren Webhook Secret 未配置")
	}
	return nil
}

func validateKyrenRemoteProduct(ctx context.Context, client kyrenAPI, productID string, amount string, currency string) error {
	product, err := client.retrieveProduct(ctx, productID)
	if err != nil {
		return err
	}
	if product == nil || !strings.EqualFold(strings.TrimSpace(product.Status), kyrenProductStatusActive) {
		return errors.New("Kyren 产品不可用")
	}
	if !kyrenDecimalEqual(product.Price, amount) || !strings.EqualFold(strings.TrimSpace(product.Currency), currency) {
		return errors.New("Kyren 产品价格或币种不匹配")
	}
	return nil
}

var errKyrenPermanentFulfillmentFailure = errors.New("permanent Kyren fulfillment failure")
var errKyrenSubscriptionOrderClaimed = errors.New("kyren subscription order is already being processed")

func isKyrenRetryableFulfillmentError(err error) bool {
	if err == nil {
		return false
	}
	return !errors.Is(err, errKyrenPermanentFulfillmentFailure) &&
		!errors.Is(err, model.ErrSubscriptionOrderNotFound) &&
		!errors.Is(err, model.ErrSubscriptionOrderStatusInvalid) &&
		!errors.Is(err, model.ErrTopUpNotFound) &&
		!errors.Is(err, model.ErrTopUpStatusInvalid) &&
		!errors.Is(err, model.ErrPaymentMethodMismatch)
}

func handleKyrenSubscriptionPaid(tradeNo string, event kyrenWebhookEvent, providerPayload string) error {
	order, err := findKyrenSubscriptionOrderByTradeNo(tradeNo)
	if err != nil {
		return err
	}
	if order == nil {
		return model.ErrSubscriptionOrderNotFound
	}
	if order.PaymentProvider != model.PaymentProviderKyren {
		return model.ErrPaymentMethodMismatch
	}
	snapshot, err := model.UnmarshalKyrenPaymentSnapshot(order.KyrenSnapshot)
	if err != nil {
		return fmt.Errorf("%w: Kyren 订阅订单快照无效: %v", errKyrenPermanentFulfillmentFailure, err)
	}
	if !kyrenPaymentSnapshotMatches(snapshot, event.Data.kyrenProductID(), event.Data.Amount, event.Data.Currency) {
		return fmt.Errorf("%w: Kyren 订阅订单金额、币种或产品不匹配", errKyrenPermanentFulfillmentFailure)
	}
	return completeKyrenSubscriptionOrderWithSnapshotAndEvaluateInvitation(tradeNo, providerPayload, model.PaymentProviderKyren, model.PaymentMethodKyren)
}

func kyrenPaymentSnapshotMatches(snapshot model.KyrenPaymentSnapshot, productID string, amount string, currency string) bool {
	return strings.TrimSpace(snapshot.ProductID) == strings.TrimSpace(productID) &&
		kyrenDecimalEqual(snapshot.Amount, amount) &&
		strings.EqualFold(strings.TrimSpace(snapshot.Currency), strings.TrimSpace(currency))
}

func kyrenControlledProviderPayload(event kyrenWebhookEvent) string {
	payload := map[string]any{
		"event_id":   event.ID,
		"event_type": event.Type,
		"order_id":   event.Data.kyrenOrderID(),
		"product_id": event.Data.kyrenProductID(),
		"amount":     event.Data.Amount,
		"currency":   strings.ToUpper(strings.TrimSpace(event.Data.Currency)),
	}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func isSupportedKyrenWebhookKind(kind string) bool {
	return kind == "subscription" || kind == "topup"
}

func recordKyrenUnsupportedKindManualAction(kind string, tradeNo string) {
	userID := findKyrenTradeUserID(tradeNo)
	model.RecordLog(userID, model.LogTypeError, fmt.Sprintf("Kyren webhook 类型无法自动处理，订单号：%s，类型：%s", tradeNo, kind))
}

func findKyrenTradeUserID(tradeNo string) int {
	if order, err := findKyrenSubscriptionOrderByTradeNo(tradeNo); err == nil && order != nil {
		return order.UserId
	}
	if topUp, err := findKyrenTopUpByTradeNo(tradeNo); err == nil && topUp != nil {
		return topUp.UserId
	}
	return 0
}

func findKyrenSubscriptionOrderByTradeNo(tradeNo string) (*model.SubscriptionOrder, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, nil
	}
	var order model.SubscriptionOrder
	err := model.DB.Where("trade_no = ?", tradeNo).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

func findKyrenTopUpByTradeNo(tradeNo string) (*model.TopUp, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, nil
	}
	var topUp model.TopUp
	err := model.DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &topUp, nil
}

func recordKyrenRefundManualAction(kind string, tradeNo string) {
	userID := findKyrenTradeUserID(tradeNo)
	model.RecordLog(userID, model.LogTypeRefund, fmt.Sprintf("Kyren 退款事件需人工处理，订单号：%s，类型：%s", tradeNo, kind))
}

type kyrenSubscriptionProductStatusResponse struct {
	Bound           bool   `json:"bound"`
	ProductID       string `json:"product_id"`
	Status          string `json:"status"`
	Price           string `json:"price"`
	Currency        string `json:"currency"`
	PriceMatches    bool   `json:"price_matches"`
	CurrencyMatches bool   `json:"currency_matches"`
}

func AdminListKyrenProducts(c *gin.Context) {
	client, err := newKyrenClientForController()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status := strings.TrimSpace(c.DefaultQuery("status", kyrenProductStatusActive))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	} else if size > 100 {
		size = 100
	}
	products, err := client.listProducts(c.Request.Context(), status, page, size)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, products)
}

func AdminGetKyrenProduct(c *gin.Context) {
	productID := strings.TrimSpace(c.Param("id"))
	if productID == "" {
		common.ApiErrorMsg(c, "无效的 Kyren 产品 ID")
		return
	}
	client, err := newKyrenClientForController()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	product, err := client.retrieveProduct(c.Request.Context(), productID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, product)
}

func AdminGetSubscriptionKyrenProduct(c *gin.Context) {
	plan, err := kyrenSubscriptionPlanFromParam(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expectedPrice, err := validateKyrenSubscriptionPlanForProduct(plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	productID := strings.TrimSpace(plan.KyrenProductId)
	if productID == "" {
		common.ApiSuccess(c, kyrenSubscriptionProductStatusResponse{Bound: false})
		return
	}
	client, err := newKyrenClientForController()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	product, err := client.retrieveProduct(c.Request.Context(), productID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, kyrenSubscriptionProductStatusResponse{
		Bound:           true,
		ProductID:       product.ID,
		Status:          product.Status,
		Price:           product.Price,
		Currency:        product.Currency,
		PriceMatches:    kyrenDecimalEqual(product.Price, expectedPrice),
		CurrencyMatches: strings.EqualFold(strings.TrimSpace(product.Currency), kyrenCurrencyCNY),
	})
}

func AdminSyncSubscriptionKyrenProduct(c *gin.Context) {
	plan, err := kyrenSubscriptionPlanFromParam(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	price, err := validateKyrenSubscriptionPlanForProduct(plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req kyrenProductSyncRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	mode := normalizeKyrenProductSyncMode(req.Mode)
	client, err := newKyrenClientForController()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	productReq := kyrenCreateProductRequest{
		Name:        strings.TrimSpace(plan.Title),
		Description: kyrenSubscriptionProductDescription(plan),
		Price:       price,
		Currency:    kyrenCurrencyCNY,
		Metadata:    kyrenSubscriptionProductMetadata(plan),
	}
	product, err := syncKyrenProductForLocalBinding(c.Request.Context(), client, strings.TrimSpace(plan.KyrenProductId), mode, productReq, func(product kyrenProduct) bool {
		return kyrenProductMetadataMatches(product.Metadata, "subscription_plan", "plan_id", strconv.Itoa(plan.Id))
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if product == nil || strings.TrimSpace(product.ID) == "" {
		common.ApiErrorMsg(c, "Kyren 产品响应无效")
		return
	}

	if err := model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", plan.Id).Update("kyren_product_id", product.ID).Error; err != nil {
		respondKyrenLocalSaveFailure(c, product, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(plan.Id)
	recordKyrenManageLog(c, fmt.Sprintf("同步 Kyren 订阅产品：plan_id=%d product_id=%s", plan.Id, product.ID))
	common.ApiSuccess(c, kyrenProductSyncResponse{ProductID: product.ID, Status: product.Status, Price: product.Price, Currency: product.Currency, Synced: true})
}

func kyrenSubscriptionPlanFromParam(c *gin.Context) (*model.SubscriptionPlan, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return nil, errors.New("无效的套餐 ID")
	}
	var plan model.SubscriptionPlan
	if err := model.DB.Where("id = ?", id).First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func validateKyrenSubscriptionPlanForProduct(plan *model.SubscriptionPlan) (string, error) {
	if plan == nil {
		return "", errors.New("套餐不存在")
	}
	if plan.EntitlementType == model.SubscriptionEntitlementCreditBalance {
		return "", errors.New("Credit 余额套餐不能同步为普通定价商品")
	}
	currency := normalizeSubscriptionPlanCurrency(plan.Currency)
	if currency != kyrenCurrencyCNY {
		return "", errors.New("Kyren 产品同步仅支持 CNY 套餐")
	}
	price, err := formatKyrenAmountFromFloat(plan.PriceAmount)
	if err != nil {
		return "", err
	}
	return price, nil
}

func normalizeKyrenProductSyncMode(raw string) string {
	switch strings.TrimSpace(raw) {
	case "", "create_or_update":
		return "create_or_update"
	case "create_new", "update_existing":
		return strings.TrimSpace(raw)
	default:
		return "create_or_update"
	}
}

func syncKyrenProductForLocalBinding(ctx context.Context, client kyrenAPI, boundProductID string, mode string, req kyrenCreateProductRequest, matches func(kyrenProduct) bool) (*kyrenProduct, error) {
	if client == nil {
		return nil, errors.New("Kyren client is nil")
	}
	boundProductID = strings.TrimSpace(boundProductID)
	if mode == "update_existing" && boundProductID == "" {
		return nil, errors.New("未绑定 Kyren 产品")
	}
	if mode != "create_new" && boundProductID != "" {
		product, err := client.retrieveProduct(ctx, boundProductID)
		if err != nil {
			if kyrenIsHTTPStatus(err, http.StatusNotFound) {
				return nil, fmt.Errorf("已绑定 Kyren 产品不存在：%s", boundProductID)
			}
			return nil, err
		}
		if product == nil || !strings.EqualFold(product.Status, kyrenProductStatusActive) {
			return nil, fmt.Errorf("已绑定 Kyren 产品不可用：%s", boundProductID)
		}
		return client.updateProduct(ctx, boundProductID, kyrenUpdateProductRequest{Name: req.Name, Description: req.Description, Image: req.Image, Price: req.Price, Currency: req.Currency, Metadata: req.Metadata})
	}
	if mode != "update_existing" {
		if existing, err := findKyrenMetadataMatchedProduct(ctx, client, matches); err != nil {
			return nil, err
		} else if existing != nil {
			return existing, nil
		}
	}
	return client.createProduct(ctx, req)
}

func findKyrenMetadataMatchedProduct(ctx context.Context, client kyrenAPI, matches func(kyrenProduct) bool) (*kyrenProduct, error) {
	if matches == nil {
		return nil, nil
	}
	page := 1
	for {
		list, err := client.listProducts(ctx, kyrenProductStatusActive, page, 100)
		if err != nil {
			return nil, err
		}
		if list == nil || len(list.Items) == 0 {
			return nil, nil
		}
		for i := range list.Items {
			product := list.Items[i]
			if strings.EqualFold(product.Status, kyrenProductStatusActive) && matches(product) {
				return &product, nil
			}
		}
		if list.Pagination.TotalPages <= page || len(list.Items) < 100 {
			return nil, nil
		}
		page++
	}
}

func kyrenProductMetadataMatches(metadata map[string]string, kind string, idKey string, idValue string) bool {
	return metadata != nil && metadata["source"] == "new-api" && metadata["kind"] == kind && metadata[idKey] == idValue
}

func kyrenSubscriptionProductMetadata(plan *model.SubscriptionPlan) map[string]string {
	metadata := map[string]string{
		"source":  "new-api",
		"kind":    "subscription_plan",
		"plan_id": strconv.Itoa(plan.Id),
	}
	if plan.BusinessCode != nil && strings.TrimSpace(*plan.BusinessCode) != "" {
		metadata["business_code"] = strings.TrimSpace(*plan.BusinessCode)
	}
	return metadata
}

func kyrenSubscriptionProductDescription(plan *model.SubscriptionPlan) string {
	if strings.TrimSpace(plan.Subtitle) != "" {
		return strings.TrimSpace(plan.Subtitle)
	}
	unit := strings.TrimSpace(plan.DurationUnit)
	if unit == "" {
		unit = model.SubscriptionDurationMonth
	}
	return fmt.Sprintf("订阅套餐：%s，周期：%d %s", strings.TrimSpace(plan.Title), plan.DurationValue, unit)
}

func respondKyrenLocalSaveFailure(c *gin.Context, product *kyrenProduct, err error) {
	productID := ""
	status := ""
	price := ""
	currency := ""
	if product != nil {
		productID = product.ID
		status = product.Status
		price = product.Price
		currency = product.Currency
	}
	localErr := err.Error()
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": fmt.Sprintf("Kyren 产品已创建或复用，但本地保存失败，请手动绑定 product_id=%s：%s", productID, localErr),
		"data": kyrenProductSyncResponse{
			ProductID:  productID,
			Status:     status,
			Price:      price,
			Currency:   currency,
			Synced:     false,
			LocalError: localErr,
		},
	})
}

func recordKyrenManageLog(c *gin.Context, content string) {
	adminID := c.GetInt("id")
	adminUsername := c.GetString("username")
	model.RecordLogWithAdminInfo(adminID, model.LogTypeManage, content, map[string]interface{}{
		"admin_id":       adminID,
		"admin_username": adminUsername,
	})
}
