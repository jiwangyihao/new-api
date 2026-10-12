package controller

import (
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
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type pqapiPayRequest struct {
	Amount int64 `json:"amount"`
}

type pqapiWebhookEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		PaymentOrderNo        string `json:"payment_order_no"`
		MerchantOrderNo       string `json:"merchant_order_no"`
		Amount                int64  `json:"amount"`
		Currency              string `json:"currency"`
		Status                string `json:"status"`
		Provider              string `json:"provider"`
		ProviderTransactionID string `json:"provider_transaction_id"`
	} `json:"data"`
}

func RequestPQAPIPay(c *gin.Context) {
	var req pqapiPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount < getMinTopup() {
		common.ApiErrorMsg(c, fmt.Sprintf("充值数量不能小于 %d", getMinTopup()))
		return
	}
	client, err := newPQAPIClient()
	if err != nil {
		common.ApiErrorMsg(c, "PQAPI 支付未配置")
		return
	}
	payMoney := getPayMoney(req.Amount, "")
	amountCents := int64(decimal.NewFromFloat(payMoney).Mul(decimal.NewFromInt(100)).Round(0).IntPart())
	if amountCents <= 0 {
		common.ApiErrorMsg(c, "充值金额过低")
		return
	}
	creditCents, err := model.AccountBalanceCentsFromCNY(decimal.NewFromInt(req.Amount))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userID := c.GetInt("id")
	tradeNo := fmt.Sprintf("PQUSR%dNO%s%d", userID, common.GetRandomString(6), time.Now().Unix())
	order := &model.TopUp{
		UserId: userID, Amount: int64(creditCents), AmountUnit: model.TopUpAmountUnitAccountBalanceCents,
		Money: payMoney, TradeNo: tradeNo, PaymentMethod: model.PaymentMethodPQAPI,
		PaymentProvider: model.PaymentProviderPQAPI, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	if err := service.CreatePQAPITopUpPaymentOrder(order); err != nil {
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}
	checkout, err := client.createCheckout(c.Request.Context(), pqapiCreateCheckoutRequest{MerchantOrderNo: tradeNo, Amount: amountCents, Currency: "CNY", Description: fmt.Sprintf("账户充值 %d", req.Amount)})
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderPQAPI, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("PQAPI 创建支付失败 trade_no=%s error=%q", tradeNo, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if checkout == nil || checkout.PaymentOrderNo == "" || !pqapiCheckoutURLAllowed(client.baseURL, checkout.CheckoutURL) || checkout.Amount != amountCents || !strings.EqualFold(checkout.Currency, "CNY") {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderPQAPI, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "支付服务返回的订单无效")
		return
	}
	if err := model.BindPaymentProviderOrderID(model.PaymentProviderPQAPI, model.PaymentOrderKindTopUp, tradeNo, checkout.PaymentOrderNo); err != nil {
		common.ApiErrorMsg(c, "绑定支付订单失败")
		return
	}
	if err := model.BindPendingPaymentProviderCheckout(model.PaymentProviderPQAPI, model.PaymentOrderKindTopUp, tradeNo, checkout.PaymentOrderNo, checkout.CheckoutURL); err != nil {
		common.ApiErrorMsg(c, "绑定收银台失败")
		return
	}
	common.ApiSuccess(c, gin.H{"checkout_url": checkout.CheckoutURL, "url": checkout.CheckoutURL, "order_id": tradeNo, "payment_order_no": checkout.PaymentOrderNo})
}

func PQAPIWebhook(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	timestamp := strings.TrimSpace(c.GetHeader("X-Timestamp"))
	eventID := strings.TrimSpace(c.GetHeader("X-Event-Id"))
	siteID := strings.TrimSpace(c.GetHeader("X-Site-Id"))
	received := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Signature")))
	ts, parseErr := strconv.ParseInt(timestamp, 10, 64)
	expected := pqapiHMAC(setting.PQAPISecret, timestamp+"\n"+eventID+"\n"+string(raw))
	if strings.TrimSpace(setting.PQAPISecret) == "" || parseErr != nil || eventID == "" || siteID != strings.TrimSpace(setting.PQAPISiteID) || time.Now().Unix()-ts > 300 || ts-time.Now().Unix() > 300 || !hmac.Equal([]byte(expected), []byte(received)) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var event pqapiWebhookEvent
	if err := common.Unmarshal(raw, &event); err != nil || event.ID != eventID || event.Type != "payment.paid" || event.Data.Status != "PAID" || event.Data.Amount <= 0 || !strings.EqualFold(event.Data.Currency, "CNY") {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	payloadHash := sha256.Sum256(raw)
	claim := model.PaymentProviderEventClaimRequest{Provider: model.PaymentProviderPQAPI, EventID: eventID, EventType: event.Type, PayloadHash: hex.EncodeToString(payloadHash[:]), TradeNo: event.Data.MerchantOrderNo, OrderKind: model.PaymentOrderKindTopUp, ProviderOrderID: event.Data.PaymentOrderNo, StaleBefore: common.GetTimestamp() - 300}
	var providerEvent *model.PaymentProviderEvent
	var outcome model.PaymentProviderEventClaimOutcome
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		providerEvent, outcome, err = model.ClaimPaymentProviderEventTx(tx, claim)
		return err
	})
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if outcome == model.PaymentProviderEventDuplicate {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	if outcome != model.PaymentProviderEventClaimed {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		mapping, err := model.EnsurePaymentProviderOrderTx(tx, model.PaymentProviderPQAPI, model.PaymentOrderKindTopUp, event.Data.MerchantOrderNo)
		if err != nil || mapping.ProviderOrderID == nil || *mapping.ProviderOrderID != event.Data.PaymentOrderNo {
			return model.ErrPaymentProviderOrderConflict
		}
		return model.BindPaymentProviderEventOrderTx(tx, providerEvent, mapping)
	})
	if err != nil {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	if err := model.CompletePQAPITopUp(event.Data.MerchantOrderNo, event.Data.Amount, event.Data.ProviderTransactionID, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("PQAPI webhook 处理失败 event_id=%s trade_no=%s error=%q", eventID, event.Data.MerchantOrderNo, err.Error()))
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		return model.FinishPaymentProviderEventTx(tx, providerEvent, model.PaymentProviderEventApplied, "PQAPI payment applied", "")
	})
	if err != nil && !errors.Is(err, model.ErrPaymentProviderEventInProgress) {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
