package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

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
