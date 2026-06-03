package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"

	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	AmountUnit      string  `json:"amount_unit" gorm:"size:32;default:''"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time"`
	Status          string  `json:"status"`
	KyrenSnapshot   string  `json:"kyren_snapshot" gorm:"type:text"`
}

type TopUpHistoryItem struct {
	TopUp
	CreditedBalanceCents   int64  `json:"credited_balance_cents"`
	CreditedBalanceDisplay string `json:"credited_balance_display"`
	AmountUnit             string `json:"amount_unit"`
	IsAccountBalanceCents  bool   `json:"is_account_balance_cents"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodKyren        = "kyren"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderKyren        = "kyren"
)

const TopUpAmountUnitAccountBalanceCents = "account_balance_cents"

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
)

type completedTopUp struct {
	UserId        int
	Amount        int
	Money         float64
	PaymentMethod string
}

func claimPendingTopUpSuccessTx(tx *gorm.DB, tradeNo string, expectedPaymentProvider string, updates map[string]any) (*TopUp, bool, error) {
	if tx == nil {
		return nil, false, errors.New("tx is nil")
	}
	if tradeNo == "" {
		return nil, false, ErrTopUpNotFound
	}
	if updates == nil {
		updates = map[string]any{}
	}
	updates["status"] = common.TopUpStatusSuccess
	updates["complete_time"] = common.GetTimestamp()

	query := tx.Model(&TopUp{}).Where("trade_no = ? AND status = ?", tradeNo, common.TopUpStatusPending)
	if expectedPaymentProvider != "" {
		query = query.Where("payment_provider = ?", expectedPaymentProvider)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		var current TopUp
		if err := tx.Where("trade_no = ?", tradeNo).First(&current).Error; err != nil {
			return nil, false, ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && current.PaymentProvider != expectedPaymentProvider {
			return nil, false, ErrPaymentMethodMismatch
		}
		if current.Status == common.TopUpStatusSuccess {
			return &current, false, nil
		}
		return nil, false, ErrTopUpStatusInvalid
	}

	var topUp TopUp
	if err := tx.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
		return nil, false, err
	}
	return &topUp, true, nil
}

func completePendingTopUpTx(tx *gorm.DB, tradeNo string, expectedPaymentProvider string, updates map[string]any) (*completedTopUp, bool, error) {
	topUp, claimed, err := claimPendingTopUpSuccessTx(tx, tradeNo, expectedPaymentProvider, updates)
	if err != nil || !claimed {
		return nil, false, err
	}
	amountToCredit := int(topUp.Amount)
	if amountToCredit <= 0 {
		return nil, false, errors.New("无效的充值额度")
	}
	if err := IncreaseUserAccountBalanceTx(tx, topUp.UserId, amountToCredit); err != nil {
		return nil, false, err
	}
	return &completedTopUp{
		UserId:        topUp.UserId,
		Amount:        amountToCredit,
		Money:         topUp.Money,
		PaymentMethod: topUp.PaymentMethod,
	}, true, nil
}

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func ClaimPendingKyrenTopUp(tradeNo string) (bool, error) {
	if tradeNo == "" {
		return false, ErrTopUpNotFound
	}
	result := DB.Model(&TopUp{}).
		Where("trade_no = ? AND payment_provider = ? AND status = ?", tradeNo, PaymentProviderKyren, common.TopUpStatusPending).
		Update("status", common.TopUpStatusFailed)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func RestoreClaimedKyrenTopUp(tradeNo string) error {
	if tradeNo == "" {
		return nil
	}
	return DB.Model(&TopUp{}).
		Where("trade_no = ? AND payment_provider = ? AND status = ?", tradeNo, PaymentProviderKyren, common.TopUpStatusFailed).
		Update("status", common.TopUpStatusPending).Error
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var completed *completedTopUp
	var claimed bool
	err = DB.Transaction(func(tx *gorm.DB) error {
		var err error
		completed, claimed, err = completePendingTopUpTx(tx, referenceId, PaymentProviderStripe, nil)
		if err != nil || !claimed {
			return err
		}
		if customerId != "" {
			return tx.Model(&User{}).Where("id = ?", completed.UserId).Update("stripe_customer", customerId).Error
		}
		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if !claimed {
		return nil
	}

	if err := InvalidateUserCache(completed.UserId); err != nil {
		return err
	}
	amountCNY := AccountBalanceCNYFromCents(completed.Amount).StringFixed(2)
	RecordTopupLog(completed.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %s，支付金额：%.2f", amountCNY, completed.Money), callerIp, completed.PaymentMethod, PaymentMethodStripe)

	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

const topUpHistoryAmountUnitLegacy = "legacy"
const topUpHistoryKyrenCentsQuotaMax int64 = 100000

type topUpHistoryKyrenSnapshot struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Quota    any    `json:"quota"`
}

type topUpHistoryCreemProduct struct {
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
	Quota    int64   `json:"quota"`
}

type topUpHistoryContext struct {
	creemProductsLoaded bool
	creemProducts       []topUpHistoryCreemProduct
	quotaPerUnitLoaded  bool
	quotaPerUnit        decimal.Decimal
	quotaPerUnitOK      bool
}

func (ctx *topUpHistoryContext) getCreemProducts() []topUpHistoryCreemProduct {
	if !ctx.creemProductsLoaded {
		ctx.creemProducts = topUpHistoryCreemProducts()
		ctx.creemProductsLoaded = true
	}
	return ctx.creemProducts
}

func (ctx *topUpHistoryContext) getQuotaPerUnit() (decimal.Decimal, bool) {
	if !ctx.quotaPerUnitLoaded {
		ctx.quotaPerUnit, ctx.quotaPerUnitOK = topUpHistoryQuotaPerUnit()
		ctx.quotaPerUnitLoaded = true
	}
	return ctx.quotaPerUnit, ctx.quotaPerUnitOK
}

func GetUserTopUpHistoryItems(userId int, pageInfo *common.PageInfo) (items []TopUpHistoryItem, total int64, err error) {
	topups, total, err := GetUserTopUps(userId, pageInfo)
	if err != nil {
		return nil, 0, err
	}
	return topUpsToHistoryItems(topups), total, nil
}

func GetAllTopUpHistoryItems(pageInfo *common.PageInfo) (items []TopUpHistoryItem, total int64, err error) {
	topups, total, err := GetAllTopUps(pageInfo)
	if err != nil {
		return nil, 0, err
	}
	return topUpsToHistoryItems(topups), total, nil
}

func SearchUserTopUpHistoryItems(userId int, keyword string, pageInfo *common.PageInfo) (items []TopUpHistoryItem, total int64, err error) {
	topups, total, err := SearchUserTopUps(userId, keyword, pageInfo)
	if err != nil {
		return nil, 0, err
	}
	return topUpsToHistoryItems(topups), total, nil
}

func SearchAllTopUpHistoryItems(keyword string, pageInfo *common.PageInfo) (items []TopUpHistoryItem, total int64, err error) {
	topups, total, err := SearchAllTopUps(keyword, pageInfo)
	if err != nil {
		return nil, 0, err
	}
	return topUpsToHistoryItems(topups), total, nil
}

func topUpsToHistoryItems(topups []*TopUp) []TopUpHistoryItem {
	if len(topups) == 0 {
		return []TopUpHistoryItem{}
	}
	items := make([]TopUpHistoryItem, len(topups))
	ctx := topUpHistoryContext{}
	for i, topUp := range topups {
		if topUp == nil {
			items[i].AmountUnit = topUpHistoryAmountUnitLegacy
			items[i].CreditedBalanceDisplay = topUpHistoryRawAuditDisplay(nil)
			continue
		}
		items[i] = topUpHistoryItemFromTopUp(topUp, &ctx)
	}
	return items
}

func topUpHistoryItemFromTopUp(topUp *TopUp, ctx *topUpHistoryContext) TopUpHistoryItem {
	item := TopUpHistoryItem{TopUp: *topUp}
	if topUp.AmountUnit == TopUpAmountUnitAccountBalanceCents {
		item.CreditedBalanceCents = topUp.Amount
		item.CreditedBalanceDisplay = topUpHistoryCNYDisplay(topUp.Amount)
		item.AmountUnit = TopUpAmountUnitAccountBalanceCents
		item.IsAccountBalanceCents = true
		return item
	}
	item.AmountUnit = topUpHistoryAmountUnitLegacy
	if creditedCents, ok := legacyTopUpCreditedBalanceCents(topUp, ctx); ok {
		item.CreditedBalanceCents = creditedCents
		item.CreditedBalanceDisplay = topUpHistoryCNYDisplay(creditedCents)
		return item
	}
	item.CreditedBalanceDisplay = topUpHistoryRawAuditDisplay(topUp)
	return item
}

func legacyTopUpCreditedBalanceCents(topUp *TopUp, ctx *topUpHistoryContext) (int64, bool) {
	if topUp == nil {
		return 0, false
	}
	if topUp.PaymentProvider == PaymentProviderKyren || topUp.PaymentMethod == PaymentMethodKyren {
		return kyrenLegacyTopUpCreditedBalanceCents(topUp, ctx)
	}
	if topUp.PaymentProvider == PaymentProviderCreem || topUp.PaymentMethod == PaymentMethodCreem {
		return creemLegacyTopUpCreditedBalanceCents(topUp, ctx)
	}
	if isLegacyCNYTopUp(topUp) && topUp.Amount <= math.MaxInt64/100 && topUp.Amount >= math.MinInt64/100 {
		return topUp.Amount * 100, true
	}
	return 0, false
}

func isLegacyCNYTopUp(topUp *TopUp) bool {
	switch strings.ToLower(strings.TrimSpace(topUp.PaymentProvider)) {
	case PaymentProviderEpay, PaymentProviderStripe, PaymentProviderWaffo, PaymentProviderWaffoPancake:
		return true
	}
	switch strings.ToLower(strings.TrimSpace(topUp.PaymentMethod)) {
	case "alipay", "wxpay", "qqpay", "bank", PaymentMethodStripe, PaymentMethodWaffo, PaymentMethodWaffoPancake:
		return true
	}
	return false
}

func kyrenLegacyTopUpCreditedBalanceCents(topUp *TopUp, ctx *topUpHistoryContext) (int64, bool) {
	if strings.TrimSpace(topUp.KyrenSnapshot) == "" {
		return 0, false
	}
	var snapshot topUpHistoryKyrenSnapshot
	if err := common.UnmarshalJsonStr(topUp.KyrenSnapshot, &snapshot); err != nil {
		return 0, false
	}
	quota, ok, err := accountBalanceMigrationOptionNumber(snapshot.Quota)
	if err != nil || !ok || quota <= 0 {
		return 0, false
	}
	if topUpHistoryKyrenSnapshotQuotaIsCents(snapshot, topUp, quota) {
		return quota, true
	}
	quotaPerUnit, ok := ctx.getQuotaPerUnit()
	if !ok {
		return 0, false
	}
	converted, err := legacyQuotaToCentsInt64(quota, quotaPerUnit)
	if err != nil || converted <= 0 {
		return 0, false
	}
	return converted, true
}

func topUpHistoryKyrenSnapshotQuotaIsCents(snapshot topUpHistoryKyrenSnapshot, topUp *TopUp, quota int64) bool {
	if expectedCents, ok := topUpHistorySnapshotAmountCents(snapshot.Amount, snapshot.Currency, topUp.Money); ok {
		return quota == expectedCents
	}
	return quota <= topUpHistoryCentsQuotaThreshold()
}

func topUpHistorySnapshotAmountCents(amount string, currency string, fallbackMoney float64) (int64, bool) {
	if trimmedCurrency := strings.ToUpper(strings.TrimSpace(currency)); trimmedCurrency != "" && trimmedCurrency != "CNY" {
		return 0, false
	}
	if trimmedAmount := strings.TrimSpace(amount); trimmedAmount != "" {
		parsed, err := decimal.NewFromString(trimmedAmount)
		if err == nil && parsed.GreaterThan(decimal.Zero) {
			return parsed.Mul(decimal.NewFromInt(100)).Round(0).IntPart(), true
		}
	}
	if fallbackMoney > 0 {
		return decimal.NewFromFloat(fallbackMoney).Mul(decimal.NewFromInt(100)).Round(0).IntPart(), true
	}
	return 0, false
}

func creemLegacyTopUpCreditedBalanceCents(topUp *TopUp, ctx *topUpHistoryContext) (int64, bool) {
	products := ctx.getCreemProducts()
	if len(products) == 0 {
		return 0, false
	}
	topUpMoney := decimal.NewFromFloat(topUp.Money).Round(2)
	var matchedQuota int64
	matches := 0
	for i := range products {
		if products[i].Quota <= 0 || !topUpHistoryCreemCurrencyCompatible(products[i].Currency) {
			continue
		}
		if !decimal.NewFromFloat(products[i].Price).Round(2).Equal(topUpMoney) {
			continue
		}
		matchedQuota = products[i].Quota
		matches++
		if matches > 1 {
			return 0, false
		}
	}
	if matches != 1 {
		return 0, false
	}
	return matchedQuota, true
}

func topUpHistoryCreemProducts() []topUpHistoryCreemProduct {
	raw := strings.TrimSpace(setting.CreemProducts)
	if DB != nil {
		value, ok, err := accountBalanceMigrationDBOptionValue("CreemProducts")
		if err == nil && ok && strings.TrimSpace(value) != "" {
			raw = value
		}
	}
	if raw == "" {
		return nil
	}
	var products []topUpHistoryCreemProduct
	if err := common.UnmarshalJsonStr(raw, &products); err != nil {
		return nil
	}
	return products
}

func topUpHistoryCreemCurrencyCompatible(currency string) bool {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "", "CNY", "USD":
		return true
	default:
		return false
	}
}

func topUpHistoryQuotaPerUnit() (decimal.Decimal, bool) {
	value, ok, err := accountBalanceMigrationDBOptionValue(accountBalanceMigrationQuotaPerUnitOption)
	if err != nil || !ok || strings.TrimSpace(value) == "" {
		return decimal.Zero, false
	}
	quotaPerUnit, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil || quotaPerUnit.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, false
	}
	return quotaPerUnit, true
}

func topUpHistoryCentsQuotaThreshold() int64 {
	return topUpHistoryKyrenCentsQuotaMax
}

func topUpHistoryCNYDisplay(cents int64) string {
	if cents > int64(math.MaxInt) || cents < -int64(math.MaxInt)-1 {
		return "¥" + decimal.NewFromInt(cents).Div(decimal.NewFromInt(100)).StringFixed(2)
	}
	return "¥" + AccountBalanceCNYFromCents(int(cents)).StringFixed(2)
}

func topUpHistoryRawAuditDisplay(topUp *TopUp) string {
	if topUp == nil {
		return "legacy/raw amount: unavailable"
	}
	return fmt.Sprintf("legacy/raw amount: %d, money: %.2f", topUp.Amount, topUp.Money)
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	var completed *completedTopUp
	var claimed bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		completed, claimed, err = completePendingTopUpTx(tx, tradeNo, "", nil)
		return err
	})

	if err != nil {
		return err
	}

	// 事务外记录日志，避免阻塞
	if !claimed {
		return nil
	}
	if err := InvalidateUserCache(completed.UserId); err != nil {
		return err
	}
	amountCNY := AccountBalanceCNYFromCents(completed.Amount).StringFixed(2)
	RecordTopupLog(completed.UserId, fmt.Sprintf("管理员补单成功，充值金额: %s，支付金额：%.2f", amountCNY, completed.Money), callerIp, completed.PaymentMethod, "admin")
	return nil
}

func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var completed *completedTopUp
	var claimed bool
	err = DB.Transaction(func(tx *gorm.DB) error {
		completedTopUp, didClaim, err := completePendingTopUpTx(tx, referenceId, PaymentProviderCreem, nil)
		if err != nil || !didClaim {
			completed = completedTopUp
			claimed = didClaim
			return err
		}

		if customerEmail != "" {
			var user User
			if err := tx.Where("id = ?", completedTopUp.UserId).First(&user).Error; err != nil {
				return err
			}
			if user.Email == "" {
				if err := tx.Model(&User{}).Where("id = ?", completedTopUp.UserId).Update("email", customerEmail).Error; err != nil {
					return err
				}
			}
		}

		completed = completedTopUp
		claimed = didClaim
		return nil
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if !claimed {
		return nil
	}

	if err := InvalidateUserCache(completed.UserId); err != nil {
		return err
	}
	amountCNY := AccountBalanceCNYFromCents(completed.Amount).StringFixed(2)
	RecordTopupLog(completed.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %s，支付金额：%.2f", amountCNY, completed.Money), callerIp, completed.PaymentMethod, PaymentMethodCreem)

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var completed *completedTopUp
	var claimed bool
	err = DB.Transaction(func(tx *gorm.DB) error {
		var err error
		completed, claimed, err = completePendingTopUpTx(tx, tradeNo, PaymentProviderWaffo, nil)
		return err
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if !claimed {
		return nil
	}
	if err := InvalidateUserCache(completed.UserId); err != nil {
		return err
	}
	if completed.Amount > 0 {
		amountCNY := AccountBalanceCNYFromCents(completed.Amount).StringFixed(2)
		RecordTopupLog(completed.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %s，支付金额: %.2f", amountCNY, completed.Money), callerIp, completed.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var completed *completedTopUp
	var claimed bool
	err = DB.Transaction(func(tx *gorm.DB) error {
		var err error
		completed, claimed, err = completePendingTopUpTx(tx, tradeNo, PaymentProviderWaffoPancake, nil)
		return err
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if !claimed {
		return nil
	}
	if err := InvalidateUserCache(completed.UserId); err != nil {
		return err
	}
	if completed.Amount > 0 {
		amountCNY := AccountBalanceCNYFromCents(completed.Amount).StringFixed(2)
		RecordLog(completed.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %s，支付金额: %.2f", amountCNY, completed.Money))
	}

	return nil
}
