package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	"gorm.io/gorm/clause"
)

type kyrenTopUpProductsListResponse struct {
	Products []kyrenTopUpProduct `json:"products"`
	Version  string              `json:"version"`
}

type kyrenTopUpProductsUpdateRequest struct {
	Products []kyrenTopUpProduct `json:"products"`
	Version  string              `json:"version"`
}

type kyrenTopUpProductStatusResponse struct {
	ProductID       string `json:"product_id"`
	Status          string `json:"status"`
	Price           string `json:"price"`
	Currency        string `json:"currency"`
	PriceMatches    bool   `json:"price_matches"`
	CurrencyMatches bool   `json:"currency_matches"`
	Version         string `json:"version"`
}

type kyrenTopUpProductSyncResponse struct {
	Products   []kyrenTopUpProduct `json:"products"`
	Version    string              `json:"version"`
	ProductID  string              `json:"product_id"`
	Status     string              `json:"status"`
	Price      string              `json:"price"`
	Currency   string              `json:"currency"`
	Synced     bool                `json:"synced"`
	LocalError string              `json:"local_error,omitempty"`
}

type KyrenPayRequest struct {
	ProductId string `json:"product_id"`
}

type kyrenTopUpSnapshot struct {
	LocalTopUpID string `json:"local_topup_id"`
	ProductID    string `json:"product_id"`
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	Quota        int64  `json:"quota"`
}

func RequestKyrenPay(c *gin.Context) {
	var req KyrenPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := ensureKyrenPaymentConfigured(); err != nil {
		common.ApiError(c, err)
		return
	}
	localProductID := strings.TrimSpace(req.ProductId)
	if localProductID == "" {
		common.ApiErrorMsg(c, "请选择充值档位")
		return
	}
	_, products, err := loadLatestKyrenTopUpProducts()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	product, ok := findKyrenTopUpProductByID(products, localProductID)
	if !ok || !product.Enabled {
		common.ApiErrorMsg(c, "充值档位不可用")
		return
	}
	product.ProductID = strings.TrimSpace(product.ProductID)
	product.Amount = strings.TrimSpace(product.Amount)
	product.Currency = strings.ToUpper(strings.TrimSpace(product.Currency))
	if product.ProductID == "" || product.Quota <= 0 || product.Amount == "" || product.Currency != kyrenCurrencyCNY {
		common.ApiErrorMsg(c, "充值档位配置错误")
		return
	}
	client, err := newKyrenClientForController()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateKyrenRemoteProduct(c.Request.Context(), client, product.ProductID, product.Amount, product.Currency); err != nil {
		common.ApiError(c, err)
		return
	}
	userID := c.GetInt("id")
	user, err := model.GetUserById(userID, false)
	if err != nil || user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	tradeNo := fmt.Sprintf("KYUSR%dNO%s%d", userID, common.GetRandomString(6), time.Now().Unix())
	snapshot, err := marshalKyrenTopUpSnapshot(kyrenTopUpSnapshot{LocalTopUpID: product.ID, ProductID: product.ProductID, Amount: product.Amount, Currency: product.Currency, Quota: product.Quota})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	money, err := strconv.ParseFloat(product.Amount, 64)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          product.Quota,
		AmountUnit:      model.TopUpAmountUnitAccountBalanceCents,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodKyren,
		PaymentProvider: model.PaymentProviderKyren,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
		KyrenSnapshot:   snapshot,
	}
	if err := topUp.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	checkout, err := client.createCheckout(c.Request.Context(), kyrenCreateCheckoutRequest{
		ProductID:     product.ProductID,
		SuccessURL:    paymentReturnPath("/console/topup?pay=success"),
		CancelURL:     paymentReturnPath("/console/topup?pay=cancel"),
		CustomerEmail: user.Email,
		CustomerName:  user.Username,
		Metadata: map[string]string{
			"kind":             "topup",
			"trade_no":         tradeNo,
			"user_id":          strconv.Itoa(userID),
			"topup_product_id": product.ID,
		},
	})
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderKyren, common.TopUpStatusExpired)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Kyren 创建充值 Checkout 失败 user_id=%d trade_no=%s product_id=%s error=%q", userID, tradeNo, product.ID, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if checkout == nil || strings.TrimSpace(checkout.URL) == "" {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderKyren, common.TopUpStatusExpired)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	common.ApiSuccess(c, gin.H{"checkout_url": checkout.URL, "url": checkout.URL, "order_id": tradeNo})
}

func marshalKyrenTopUpSnapshot(snapshot kyrenTopUpSnapshot) (string, error) {
	payload, err := common.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalKyrenTopUpSnapshot(payload string) (kyrenTopUpSnapshot, error) {
	var snapshot kyrenTopUpSnapshot
	if err := common.UnmarshalJsonStr(payload, &snapshot); err != nil {
		return kyrenTopUpSnapshot{}, err
	}
	return snapshot, nil
}

func completeKyrenTopUpWithSnapshot(tradeNo string, event kyrenWebhookEvent, callerIP string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}
	claimed, err := model.ClaimPendingKyrenTopUp(tradeNo)
	if err != nil {
		return err
	}
	if !claimed {
		topUp, lookupErr := findKyrenTopUpByTradeNo(tradeNo)
		if lookupErr != nil {
			return lookupErr
		}
		if topUp == nil {
			return model.ErrTopUpNotFound
		}
		if topUp.PaymentProvider != model.PaymentProviderKyren {
			return model.ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		return model.ErrTopUpStatusInvalid
	}
	defer func() {
		if err != nil {
			_ = model.RestoreClaimedKyrenTopUp(tradeNo)
		}
	}()
	var creditedQuota int64
	var topUpUserID int
	var topUpMoney float64
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var topUp model.TopUp
		if err := tx.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
			return err
		}
		if topUp.PaymentProvider != model.PaymentProviderKyren {
			return model.ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusFailed {
			return model.ErrTopUpStatusInvalid
		}
		snapshot, err := unmarshalKyrenTopUpSnapshot(topUp.KyrenSnapshot)
		if err != nil {
			return fmt.Errorf("%w: Kyren 充值订单快照无效: %v", errKyrenPermanentFulfillmentFailure, err)
		}
		if !kyrenTopUpSnapshotMatches(snapshot, event.Data.kyrenProductID(), event.Data.Amount, event.Data.Currency) {
			return fmt.Errorf("%w: Kyren 充值订单金额、币种或产品不匹配", errKyrenPermanentFulfillmentFailure)
		}
		if snapshot.Quota <= 0 {
			return fmt.Errorf("%w: 无效的充值额度", errKyrenPermanentFulfillmentFailure)
		}
		amountToCredit := int(snapshot.Quota)
		topUp.Amount = snapshot.Quota
		topUp.AmountUnit = model.TopUpAmountUnitAccountBalanceCents
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		result := tx.Model(&model.TopUp{}).
			Where("trade_no = ? AND payment_provider = ? AND status = ?", topUp.TradeNo, model.PaymentProviderKyren, common.TopUpStatusFailed).
			Updates(map[string]any{
				"amount":        topUp.Amount,
				"amount_unit":   model.TopUpAmountUnitAccountBalanceCents,
				"complete_time": topUp.CompleteTime,
				"status":        common.TopUpStatusSuccess,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return model.ErrTopUpStatusInvalid
		}
		if err := model.IncreaseUserAccountBalanceTx(tx, topUp.UserId, amountToCredit); err != nil {
			return err
		}
		creditedQuota = snapshot.Quota
		topUpUserID = topUp.UserId
		topUpMoney = topUp.Money
		return nil
	})
	if err != nil {
		return err
	}
	if topUpUserID <= 0 || creditedQuota <= 0 {
		return nil
	}
	if err := model.InvalidateUserCache(topUpUserID); err != nil {
		return err
	}
	if creditedQuota > 0 {
		balanceCNY := model.AccountBalanceCNYFromCents(int(creditedQuota)).StringFixed(2)
		model.RecordTopupLog(topUpUserID, fmt.Sprintf("Kyren充值成功，充值额度: %s，支付金额: %.2f", balanceCNY, topUpMoney), callerIP, model.PaymentMethodKyren, model.PaymentMethodKyren)
	}
	return nil
}

func kyrenTopUpSnapshotMatches(snapshot kyrenTopUpSnapshot, productID string, amount string, currency string) bool {
	return strings.TrimSpace(snapshot.ProductID) == strings.TrimSpace(productID) &&
		kyrenDecimalEqual(snapshot.Amount, amount) &&
		strings.EqualFold(strings.TrimSpace(snapshot.Currency), strings.TrimSpace(currency))
}

func AdminListKyrenTopUpProducts(c *gin.Context) {
	normalized, products, err := loadLatestKyrenTopUpProducts()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, kyrenTopUpProductsListResponse{Products: products, Version: kyrenTopUpProductsVersion(normalized)})
}

func AdminUpdateKyrenTopUpProducts(c *gin.Context) {
	var req kyrenTopUpProductsUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	payload, err := common.Marshal(req.Products)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	normalized, err := normalizeKyrenTopUpProductsJSON(string(payload))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var savedProducts []kyrenTopUpProduct
	if err := common.UnmarshalJsonStr(normalized, &savedProducts); err != nil {
		common.ApiError(c, err)
		return
	}
	savedNormalized, savedProducts, conflicted, err := saveKyrenTopUpProductsOptionCAS(strings.TrimSpace(req.Version), normalized)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if conflicted {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Kyren 充值档位配置已被其他操作修改，请刷新后重试",
			"data":    kyrenTopUpProductsListResponse{Products: savedProducts, Version: kyrenTopUpProductsVersion(savedNormalized)},
		})
		return
	}
	recordKyrenManageLog(c, "更新 Kyren 充值档位配置")
	common.ApiSuccess(c, kyrenTopUpProductsListResponse{Products: savedProducts, Version: kyrenTopUpProductsVersion(savedNormalized)})
}

func AdminGetKyrenTopUpProductStatus(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		common.ApiErrorMsg(c, "无效的充值档位 ID")
		return
	}
	normalized, products, err := loadLatestKyrenTopUpProducts()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	product, ok := findKyrenTopUpProductByID(products, id)
	if !ok {
		common.ApiErrorMsg(c, "充值档位不存在")
		return
	}
	response := kyrenTopUpProductStatusResponse{ProductID: product.ProductID, Version: kyrenTopUpProductsVersion(normalized)}
	if strings.TrimSpace(product.ProductID) == "" {
		common.ApiSuccess(c, response)
		return
	}
	client, err := newKyrenClientForController()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	remote, err := client.retrieveProduct(c.Request.Context(), product.ProductID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response.Status = remote.Status
	response.Price = remote.Price
	response.Currency = remote.Currency
	response.PriceMatches = kyrenDecimalEqual(remote.Price, product.Amount)
	response.CurrencyMatches = strings.EqualFold(strings.TrimSpace(remote.Currency), product.Currency)
	common.ApiSuccess(c, response)
}

func AdminSyncKyrenTopUpProduct(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		common.ApiErrorMsg(c, "无效的充值档位 ID")
		return
	}
	var req kyrenProductSyncRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	mode := normalizeKyrenProductSyncMode(req.Mode)
	_, products, err := loadLatestKyrenTopUpProducts()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	localProduct, ok := findKyrenTopUpProductByID(products, id)
	if !ok {
		common.ApiErrorMsg(c, "充值档位不存在")
		return
	}
	client, err := newKyrenClientForController()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	remoteReq := kyrenCreateProductRequest{
		Name:        localProduct.Name,
		Description: localProduct.Description,
		Price:       localProduct.Amount,
		Currency:    localProduct.Currency,
		Metadata:    kyrenTopUpProductMetadata(localProduct),
	}
	remote, err := syncKyrenProductForLocalBinding(c.Request.Context(), client, strings.TrimSpace(localProduct.ProductID), mode, remoteReq, func(product kyrenProduct) bool {
		return kyrenProductMetadataMatches(product.Metadata, "wallet_topup", "topup_product_id", localProduct.ID)
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if remote == nil || strings.TrimSpace(remote.ID) == "" {
		common.ApiErrorMsg(c, "Kyren 产品响应无效")
		return
	}

	savedNormalized, savedProducts, err := mergeKyrenTopUpProductIDAndSave(id, remote.ID)
	if err != nil {
		respondKyrenTopUpLocalSaveFailure(c, remote, err)
		return
	}
	recordKyrenManageLog(c, fmt.Sprintf("同步 Kyren 充值档位产品：topup_product_id=%s product_id=%s", id, remote.ID))
	common.ApiSuccess(c, kyrenTopUpProductSyncResponse{
		Products:  savedProducts,
		Version:   kyrenTopUpProductsVersion(savedNormalized),
		ProductID: remote.ID,
		Status:    remote.Status,
		Price:     remote.Price,
		Currency:  remote.Currency,
		Synced:    true,
	})
}

func kyrenTopUpProductsVersion(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func loadLatestKyrenTopUpProducts() (string, []kyrenTopUpProduct, error) {
	raw := strings.TrimSpace(setting.KyrenTopUpProducts)
	var option model.Option
	if err := model.DB.Where("key = ?", "KyrenTopUpProducts").First(&option).Error; err == nil {
		raw = option.Value
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil, err
	}
	if strings.TrimSpace(raw) == "" {
		raw = "[]"
	}
	normalized, err := normalizeKyrenTopUpProductsJSON(raw)
	if err != nil {
		return "", nil, err
	}
	var products []kyrenTopUpProduct
	if err := common.UnmarshalJsonStr(normalized, &products); err != nil {
		return "", nil, err
	}
	return normalized, products, nil
}

func loadLatestKyrenTopUpProductsTx(tx *gorm.DB) (string, []kyrenTopUpProduct, error) {
	if tx == nil {
		return loadLatestKyrenTopUpProducts()
	}
	option := model.Option{Key: "KyrenTopUpProducts", Value: "[]"}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
		return "", nil, err
	}
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("key = ?", "KyrenTopUpProducts").First(&option).Error; err != nil {
		return "", nil, err
	}
	raw := option.Value
	if strings.TrimSpace(raw) == "" {
		raw = "[]"
	}
	normalized, err := normalizeKyrenTopUpProductsJSON(raw)
	if err != nil {
		return "", nil, err
	}
	if option.Value != normalized {
		option.Value = normalized
		if err := tx.Save(&option).Error; err != nil {
			return "", nil, err
		}
		if err := applyKyrenRuntimeOption("KyrenTopUpProducts", normalized); err != nil {
			return "", nil, err
		}
		common.OptionMapRWMutex.Lock()
		common.OptionMap["KyrenTopUpProducts"] = normalized
		common.OptionMapRWMutex.Unlock()
	}
	var products []kyrenTopUpProduct
	if err := common.UnmarshalJsonStr(normalized, &products); err != nil {
		return "", nil, err
	}
	return normalized, products, nil
}

func saveKyrenTopUpProductsOption(normalized string) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		return saveKyrenTopUpProductsOptionTx(tx, normalized)
	})
}

func saveKyrenTopUpProductsOptionCAS(expectedVersion string, normalized string) (string, []kyrenTopUpProduct, bool, error) {
	var savedNormalized string
	var savedProducts []kyrenTopUpProduct
	var conflicted bool
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		latestNormalized, latestProducts, err := loadLatestKyrenTopUpProductsTx(tx)
		if err != nil {
			return err
		}
		if strings.TrimSpace(expectedVersion) == "" || strings.TrimSpace(expectedVersion) != kyrenTopUpProductsVersion(latestNormalized) {
			savedNormalized = latestNormalized
			savedProducts = latestProducts
			conflicted = true
			return nil
		}
		if err := saveKyrenTopUpProductsOptionTx(tx, normalized); err != nil {
			return err
		}
		if err := common.UnmarshalJsonStr(normalized, &savedProducts); err != nil {
			return err
		}
		savedNormalized = normalized
		return nil
	})
	return savedNormalized, savedProducts, conflicted, err
}

func saveKyrenTopUpProductsOptionTx(tx *gorm.DB, normalized string) error {
	if _, err := normalizeKyrenTopUpProductsJSON(normalized); err != nil {
		return err
	}
	option := model.Option{Key: "KyrenTopUpProducts"}
	if err := tx.FirstOrCreate(&option, model.Option{Key: "KyrenTopUpProducts"}).Error; err != nil {
		return err
	}
	option.Value = normalized
	if err := tx.Save(&option).Error; err != nil {
		return err
	}
	if err := applyKyrenRuntimeOption("KyrenTopUpProducts", normalized); err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	common.OptionMap["KyrenTopUpProducts"] = normalized
	common.OptionMapRWMutex.Unlock()
	return nil
}

func mergeKyrenTopUpProductIDAndSave(id string, productID string) (string, []kyrenTopUpProduct, error) {
	var savedNormalized string
	var savedProducts []kyrenTopUpProduct
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		_, latestProducts, err := loadLatestKyrenTopUpProductsTx(tx)
		if err != nil {
			return err
		}
		found := false
		for i := range latestProducts {
			if latestProducts[i].ID == id {
				latestProducts[i].ProductID = productID
				found = true
				break
			}
		}
		if !found {
			return errors.New("充值档位不存在")
		}
		payload, err := common.Marshal(latestProducts)
		if err != nil {
			return err
		}
		normalized, err := normalizeKyrenTopUpProductsJSON(string(payload))
		if err != nil {
			return err
		}
		if err := saveKyrenTopUpProductsOptionTx(tx, normalized); err != nil {
			return err
		}
		if err := common.UnmarshalJsonStr(normalized, &savedProducts); err != nil {
			return err
		}
		savedNormalized = normalized
		return nil
	})
	return savedNormalized, savedProducts, err
}

func findKyrenTopUpProductByID(products []kyrenTopUpProduct, id string) (kyrenTopUpProduct, bool) {
	for i := range products {
		if products[i].ID == id {
			return products[i], true
		}
	}
	return kyrenTopUpProduct{}, false
}

func kyrenTopUpProductMetadata(product kyrenTopUpProduct) map[string]string {
	return map[string]string{
		"source":           "new-api",
		"kind":             "wallet_topup",
		"topup_product_id": product.ID,
		"quota":            strconv.FormatInt(product.Quota, 10),
	}
}

func respondKyrenTopUpLocalSaveFailure(c *gin.Context, product *kyrenProduct, err error) {
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
		"data": kyrenTopUpProductSyncResponse{
			ProductID:  productID,
			Status:     status,
			Price:      price,
			Currency:   currency,
			Synced:     false,
			LocalError: localErr,
		},
	})
}
