package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	CreditBalanceLedgerSourceSubscriptionOrder         = "subscription_order"
	CreditBalanceLedgerSourceRedemption                = "redemption"
	CreditBalanceLedgerSourceSubscriptionConversion    = "subscription_conversion"
	CreditBalanceLedgerTypePurchase                    = "purchase"
	CreditBalanceLedgerTypeRedemption                  = "redemption"
	CreditBalanceLedgerTypeSubscriptionConversion      = "subscription_conversion"
	CreditBalanceLedgerSourceSubscriptionOrderRecovery = "subscription_order_recovery"
	CreditBalanceLedgerSourceAdminAdjustment           = "admin_adjustment"
	CreditBalanceLedgerTypeRefund                      = "refund"
	CreditBalanceLedgerTypeChargeback                  = "chargeback"
	CreditBalanceLedgerTypeAdminIncrease               = "admin_increase"
	CreditBalanceLedgerTypeAdminDecrease               = "admin_decrease"
)

const (
	CreditBalanceStatusAvailable = "available"
	CreditBalanceStatusExhausted = "exhausted"
	CreditBalanceStatusDebt      = "debt"
)

type CreditBalanceLedger struct {
	Id                         int    `json:"id"`
	UserId                     int    `json:"user_id" gorm:"not null;index;uniqueIndex:idx_credit_balance_ledger_user_key,priority:1"`
	UserSubscriptionId         int    `json:"user_subscription_id" gorm:"not null;index"`
	Type                       string `json:"type" gorm:"type:varchar(32);not null;index"`
	IdempotencyKey             string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex:idx_credit_balance_ledger_user_key,priority:2"`
	SourceType                 string `json:"source_type" gorm:"type:varchar(32);not null;uniqueIndex:idx_credit_balance_ledger_source,priority:1;index"`
	SourceId                   int    `json:"source_id" gorm:"not null;uniqueIndex:idx_credit_balance_ledger_source,priority:2"`
	SourceSnapshot             string `json:"source_snapshot,omitempty" gorm:"type:text"`
	GrossCredit                int64  `json:"gross_credit" gorm:"type:bigint;not null"`
	DebtOffset                 int64  `json:"debt_offset" gorm:"type:bigint;not null;default:0"`
	DebtFormed                 int64  `json:"debt_formed" gorm:"type:bigint;not null;default:0"`
	AvailableCreditBefore      int64  `json:"available_credit_before" gorm:"type:bigint;not null;default:0"`
	SettlementDebtBefore       int64  `json:"settlement_debt_before" gorm:"type:bigint;not null;default:0"`
	BalanceBefore              int64  `json:"balance_before" gorm:"type:bigint;not null"`
	BalanceAfter               int64  `json:"balance_after" gorm:"type:bigint;not null"`
	AvailableCreditAfter       int64  `json:"available_credit_after" gorm:"type:bigint;not null;default:0"`
	SettlementDebtAfter        int64  `json:"settlement_debt_after" gorm:"type:bigint;not null;default:0"`
	OperatorUserId             int    `json:"operator_user_id" gorm:"not null;default:0"`
	PaymentProvider            string `json:"payment_provider,omitempty" gorm:"type:varchar(50);not null;default:''"`
	ParameterFingerprint       string `json:"-" gorm:"type:varchar(64);not null;default:''"`
	Reason                     string `json:"reason" gorm:"type:varchar(255);not null;default:''"`
	ValuationCurrency          string `json:"valuation_currency" gorm:"type:varchar(8);not null;default:''"`
	ValuationGrossCostMicros   int64  `json:"valuation_gross_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationNetCostMicros     int64  `json:"valuation_net_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationConfidence        string `json:"valuation_confidence" gorm:"type:varchar(16);not null;default:''"`
	ValuationRuleVersion       int    `json:"valuation_rule_version" gorm:"not null;default:0"`
	ValuationStateVersionAfter int64  `json:"valuation_state_version_after" gorm:"type:bigint;not null;default:0"`
	FxSourceCurrency           string `json:"fx_source_currency" gorm:"type:varchar(8);not null;default:''"`
	FxRateNumerator            int64  `json:"fx_rate_numerator,string" gorm:"type:bigint;not null;default:0"`
	FxRateDenominator          int64  `json:"fx_rate_denominator,string" gorm:"type:bigint;not null;default:0"`
	FxCapturedAt               int64  `json:"fx_captured_at" gorm:"type:bigint;not null;default:0"`
	CreatedAt                  int64  `json:"created_at" gorm:"type:bigint;not null;index"`
}

type CreditBalanceLedgerHistoryItem struct {
	CreditBalanceLedger
	PaymentMethod string `json:"payment_method,omitempty"`
	PurchaseMode  string `json:"purchase_mode,omitempty"`
}

func (l *CreditBalanceLedger) BeforeUpdate(_ *gorm.DB) error {
	return errors.New("credit balance ledger is immutable")
}

func (l *CreditBalanceLedger) BeforeDelete(_ *gorm.DB) error {
	return errors.New("credit balance ledger is immutable")
}

type CreditBalanceGrantRequest struct {
	UserId                  int
	GrossCredit             int64
	IdempotencyKey          string
	SourceType              string
	SourceId                int
	SourceSnapshot          string
	Type                    string
	TargetPlanId            int
	OperatorUserId          int
	Reason                  string
	PaymentProvider         string
	TargetPlanSnapshot      *SubscriptionPlan
	PreserveActiveSelection bool
}

type CreditBalanceGrantResult struct {
	UserSubscriptionId int    `json:"user_subscription_id"`
	PlanId             int    `json:"plan_id"`
	GrossCredit        int64  `json:"gross_credit"`
	DebtOffset         int64  `json:"debt_offset"`
	AvailableCredit    int64  `json:"available_credit"`
	SettlementDebt     int64  `json:"settlement_debt"`
	BalanceBefore      int64  `json:"balance_before"`
	BalanceAfter       int64  `json:"balance_after"`
	Active             bool   `json:"active"`
	LedgerId           int    `json:"ledger_id"`
	Status             string `json:"status"`
}

func NormalizeSubscriptionPurchaseMode(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case SubscriptionPurchaseModeTimed:
		return SubscriptionPurchaseModeTimed, nil
	case SubscriptionPurchaseModeCreditBalance:
		return SubscriptionPurchaseModeCreditBalance, nil
	default:
		return "", errors.New("购买模式必须明确选择计时套餐或 Credit 余额")
	}
}

func CreditBalancePlanFromEntitlementSnapshot(snapshot SubscriptionEntitlementSnapshot) (*SubscriptionPlan, error) {
	if snapshot.TargetCreditBalancePlanID <= 0 {
		return nil, errors.New("invalid credit balance target snapshot")
	}
	businessCode := strings.TrimSpace(snapshot.TargetCreditBalanceBusinessCode)
	plan := &SubscriptionPlan{
		Id:                      snapshot.TargetCreditBalancePlanID,
		Title:                   snapshot.TargetCreditBalanceTitle,
		EntitlementType:         SubscriptionEntitlementCreditBalance,
		Enabled:                 true,
		ModelLimits:             snapshot.TargetCreditBalanceModelLimits,
		ConcurrencyLimit:        snapshot.TargetCreditBalanceConcurrencyLimit,
		QueueCapacity:           snapshot.TargetCreditBalanceQueueCapacity,
		GPTAbuseWarningLimit:    snapshot.TargetCreditBalanceGPTAbuseWarningLimit,
		CreditBalanceConfigured: true,
	}
	if businessCode != "" {
		plan.BusinessCode = &businessCode
	}
	return plan, nil
}
func SubscriptionPlanFromEntitlementSnapshot(snapshot SubscriptionEntitlementSnapshot) (*SubscriptionPlan, error) {
	if snapshot.PlanID <= 0 {
		return nil, errors.New("invalid subscription entitlement snapshot")
	}
	businessCode := strings.TrimSpace(snapshot.BusinessCode)
	plan := &SubscriptionPlan{
		Id:                      snapshot.PlanID,
		Title:                   snapshot.PlanTitle,
		PriceAmount:             snapshot.PriceAmount,
		Currency:                snapshot.Currency,
		EntitlementType:         snapshot.PlanEntitlementType,
		TotalAmount:             snapshot.TotalAmount,
		MonthlyTokenLimit:       snapshot.MonthlyTokenLimit,
		ConcurrencyLimit:        snapshot.ConcurrencyLimit,
		QueueCapacity:           snapshot.QueueCapacity,
		ModelLimits:             snapshot.ModelLimits,
		GPTAbuseWarningLimit:    snapshot.GPTAbuseWarningLimit,
		DurationUnit:            snapshot.DurationUnit,
		DurationValue:           snapshot.DurationValue,
		CustomSeconds:           snapshot.CustomSeconds,
		QuotaResetPeriod:        snapshot.QuotaResetPeriod,
		QuotaResetCustomSeconds: snapshot.QuotaResetCustomSeconds,
		MaxPurchasePerUser:      snapshot.MaxPurchasePerUser,
		IsTrial:                 snapshot.IsTrial,
		InviteTrial:             snapshot.InviteTrial,
		RewardEligible:          snapshot.RewardEligible,
	}
	if strings.TrimSpace(plan.EntitlementType) == "" {
		plan.EntitlementType = SubscriptionEntitlementTimed
	}
	if businessCode != "" {
		plan.BusinessCode = &businessCode
	}
	return plan, nil
}

func SetUserLastSubscriptionPurchaseModeTx(tx *gorm.DB, userId int, purchaseMode string) error {
	if tx == nil || userId <= 0 {
		return errors.New("invalid user purchase mode update")
	}
	mode, err := NormalizeSubscriptionPurchaseMode(purchaseMode)
	if err != nil {
		return err
	}
	var user User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("setting").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	setting := user.GetSetting()
	setting.LastSubscriptionPurchaseMode = mode
	settingJSON, err := marshalUserSetting(setting)
	if err != nil {
		return err
	}
	return tx.Model(&User{}).Where("id = ?", userId).Update("setting", settingJSON).Error
}

func GetCreditBalancePlanTx(tx *gorm.DB) (*SubscriptionPlan, error) {
	if tx == nil {
		tx = DB
	}
	var plan SubscriptionPlan
	if err := tx.Where("entitlement_type = ?", SubscriptionEntitlementCreditBalance).First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func GrantCreditBalanceTx(tx *gorm.DB, request CreditBalanceGrantRequest) (*CreditBalanceGrantResult, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.SourceType = strings.TrimSpace(request.SourceType)
	request.Type = strings.TrimSpace(request.Type)
	request.SourceSnapshot = strings.TrimSpace(request.SourceSnapshot)
	request.PaymentProvider = strings.TrimSpace(request.PaymentProvider)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.UserId <= 0 || request.GrossCredit <= 0 || request.IdempotencyKey == "" || request.SourceType == "" || request.SourceId <= 0 || request.Type == "" || request.TargetPlanId <= 0 {
		return nil, errors.New("invalid credit balance grant")
	}

	var user User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "setting").Where("id = ?", request.UserId).First(&user).Error; err != nil {
		return nil, err
	}
	if result, found, err := findCreditBalanceGrantResultTx(tx, request); err != nil {
		return nil, err
	} else if found {
		return result, nil
	}

	plan := request.TargetPlanSnapshot
	if plan == nil {
		var err error
		plan, err = GetCreditBalancePlanTx(tx)
		if err != nil {
			return nil, err
		}
	}
	if plan.Id != request.TargetPlanId || plan.EntitlementType != SubscriptionEntitlementCreditBalance {
		return nil, errors.New("credit balance target plan mismatch")
	}
	hadUsableSubscription, err := hasUsableSubscriptionTx(tx, request.UserId)
	if err != nil {
		return nil, err
	}

	balance, err := getOrCreateCreditBalanceSubscriptionTx(tx, request.UserId, plan)
	if err != nil {
		return nil, err
	}
	if balance.TokenLimit < 0 || balance.TokenUsed < 0 {
		return nil, errors.New("invalid credit balance aggregate")
	}
	if request.GrossCredit > math.MaxInt64-balance.TokenLimit {
		return nil, errors.New("credit balance overflow")
	}
	balanceBefore := balance.TokenLimit - balance.TokenUsed
	settlementDebtBefore := int64(0)
	if balanceBefore < 0 {
		settlementDebtBefore = -balanceBefore
	}
	debtOffset := minInt64(request.GrossCredit, settlementDebtBefore)
	newLimit := balance.TokenLimit + request.GrossCredit
	balanceAfter := newLimit - balance.TokenUsed
	availableAfter := maxInt64(balanceAfter, 0)
	debtAfter := maxInt64(-balanceAfter, 0)
	if err := tx.Model(&UserSubscription{}).Where("id = ?", balance.Id).Updates(map[string]any{
		"token_limit": newLimit,
		"updated_at":  common.GetTimestamp(),
	}).Error; err != nil {
		return nil, err
	}
	balance.TokenLimit = newLimit

	setting := user.GetSetting()
	if !hadUsableSubscription && !request.PreserveActiveSelection {
		setting.ActiveSubscriptionId = balance.Id
		settingJSON, err := marshalUserSetting(setting)
		if err != nil {
			return nil, err
		}
		if err := tx.Model(&User{}).Where("id = ?", request.UserId).Update("setting", settingJSON).Error; err != nil {
			return nil, err
		}
	}

	ledger := CreditBalanceLedger{
		UserId:                request.UserId,
		UserSubscriptionId:    balance.Id,
		Type:                  request.Type,
		IdempotencyKey:        request.IdempotencyKey,
		SourceType:            request.SourceType,
		SourceId:              request.SourceId,
		SourceSnapshot:        request.SourceSnapshot,
		GrossCredit:           request.GrossCredit,
		AvailableCreditBefore: maxInt64(balanceBefore, 0),
		SettlementDebtBefore:  settlementDebtBefore,
		DebtOffset:            debtOffset,
		BalanceBefore:         balanceBefore,
		BalanceAfter:          balanceAfter,
		AvailableCreditAfter:  availableAfter,
		SettlementDebtAfter:   debtAfter,
		OperatorUserId:        request.OperatorUserId,
		PaymentProvider:       request.PaymentProvider,
		Reason:                request.Reason,
		CreatedAt:             getDBTimestampTx(tx),
	}
	if err := tx.Create(&ledger).Error; err != nil {
		return nil, err
	}
	return creditBalanceGrantResult(&ledger, plan.Id, setting.ActiveSubscriptionId == balance.Id), nil
}

func findCreditBalanceGrantResultTx(tx *gorm.DB, request CreditBalanceGrantRequest) (*CreditBalanceGrantResult, bool, error) {
	var ledger CreditBalanceLedger
	query := tx.Where("(user_id = ? AND idempotency_key = ?) OR (source_type = ? AND source_id = ?)", request.UserId, request.IdempotencyKey, request.SourceType, request.SourceId).Limit(1).Find(&ledger)
	if query.Error != nil {
		return nil, false, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, false, nil
	}
	if ledger.UserId != request.UserId || ledger.IdempotencyKey != request.IdempotencyKey || ledger.SourceType != request.SourceType || ledger.SourceId != request.SourceId || ledger.GrossCredit != request.GrossCredit || ledger.Type != request.Type || ledger.SourceSnapshot != request.SourceSnapshot || ledger.PaymentProvider != strings.TrimSpace(request.PaymentProvider) {
		return nil, false, errors.New("credit balance idempotency key mismatch")
	}
	var balance UserSubscription
	if err := tx.Select("id", "plan_id").Where("id = ?", ledger.UserSubscriptionId).First(&balance).Error; err != nil {
		return nil, false, err
	}
	if balance.PlanId != request.TargetPlanId {
		return nil, false, errors.New("credit balance idempotency target plan mismatch")
	}
	var user User
	if err := tx.Select("setting").Where("id = ?", request.UserId).First(&user).Error; err != nil {
		return nil, false, err
	}
	return creditBalanceGrantResult(&ledger, balance.PlanId, user.GetSetting().ActiveSubscriptionId == balance.Id), true, nil
}

func FindCreditBalanceGrantBySourceTx(tx *gorm.DB, sourceType string, sourceId int) (*CreditBalanceGrantResult, error) {
	if tx == nil || strings.TrimSpace(sourceType) == "" || sourceId <= 0 {
		return nil, errors.New("invalid credit balance source")
	}
	var ledger CreditBalanceLedger
	if err := tx.Where("source_type = ? AND source_id = ?", strings.TrimSpace(sourceType), sourceId).First(&ledger).Error; err != nil {
		return nil, err
	}
	var balance UserSubscription
	if err := tx.Select("id", "plan_id").Where("id = ?", ledger.UserSubscriptionId).First(&balance).Error; err != nil {
		return nil, err
	}
	var user User
	if err := tx.Select("setting").Where("id = ?", ledger.UserId).First(&user).Error; err != nil {
		return nil, err
	}
	return creditBalanceGrantResult(&ledger, balance.PlanId, user.GetSetting().ActiveSubscriptionId == balance.Id), nil
}

type CreditBalanceLedgerFilter struct {
	UserId     int
	SourceType string
	Type       string
	StartTime  int64
	EndTime    int64
	Limit      int
}

func ListCreditBalanceLedger(userId int, limit int) ([]CreditBalanceLedgerHistoryItem, error) {
	return ListCreditBalanceLedgerFiltered(CreditBalanceLedgerFilter{UserId: userId, Limit: limit})
}

func ListCreditBalanceLedgerFiltered(filter CreditBalanceLedgerFilter) ([]CreditBalanceLedgerHistoryItem, error) {
	if filter.UserId <= 0 {
		return nil, errors.New("invalid userId")
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 100
	}
	query := DB.Where("user_id = ?", filter.UserId)
	if sourceType := strings.TrimSpace(filter.SourceType); sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	if entryType := strings.TrimSpace(filter.Type); entryType != "" {
		query = query.Where("type = ?", entryType)
	}
	if filter.StartTime > 0 {
		query = query.Where("created_at >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("created_at <= ?", filter.EndTime)
	}
	var entries []CreditBalanceLedger
	if err := query.Order("id desc").Limit(filter.Limit).Find(&entries).Error; err != nil {
		return nil, err
	}
	return hydrateCreditBalanceLedgerHistory(filter.UserId, entries)
}

func hydrateCreditBalanceLedgerHistory(userId int, entries []CreditBalanceLedger) ([]CreditBalanceLedgerHistoryItem, error) {
	result := make([]CreditBalanceLedgerHistoryItem, len(entries))
	orderIds := make([]int, 0, len(entries))
	for index := range entries {
		result[index].CreditBalanceLedger = entries[index]
		if (entries[index].SourceType == CreditBalanceLedgerSourceSubscriptionOrder || entries[index].SourceType == CreditBalanceLedgerSourceSubscriptionOrderRecovery) && entries[index].SourceId > 0 {
			orderIds = append(orderIds, entries[index].SourceId)
		}
	}
	if len(orderIds) == 0 {
		return result, nil
	}
	var orders []SubscriptionOrder
	if err := DB.Select("id", "payment_provider", "payment_method", "entitlement_snapshot").Where("user_id = ? AND id IN ?", userId, orderIds).Find(&orders).Error; err != nil {
		return nil, err
	}
	ordersById := make(map[int]SubscriptionOrder, len(orders))
	for index := range orders {
		ordersById[orders[index].Id] = orders[index]
	}
	for index := range result {
		order, ok := ordersById[result[index].SourceId]
		if !ok {
			continue
		}
		result[index].CreditBalanceLedger.PaymentProvider = order.PaymentProvider
		result[index].PaymentMethod = order.PaymentMethod
		result[index].PurchaseMode = SubscriptionPurchaseModeCreditBalance
		if snapshot, err := UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot); err == nil {
			if mode, err := NormalizeSubscriptionPurchaseMode(snapshot.PurchaseMode); err == nil {
				result[index].PurchaseMode = mode
			}
		}
	}
	return result, nil
}

func getOrCreateCreditBalanceSubscriptionTx(tx *gorm.DB, userId int, plan *SubscriptionPlan) (*UserSubscription, error) {
	var balance UserSubscription
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND entitlement_type = ?", userId, SubscriptionEntitlementCreditBalance).Limit(1).Find(&balance)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected > 0 {
		return &balance, nil
	}
	now := getDBTimestampTx(tx)
	balance = UserSubscription{
		UserId:           userId,
		PlanId:           plan.Id,
		EntitlementType:  SubscriptionEntitlementCreditBalance,
		TokenLimit:       0,
		TokenUsed:        0,
		ConcurrencyLimit: plan.ConcurrencyLimit,
		GrantReason:      SubscriptionGrantOrder,
		StartTime:        now,
		EndTime:          0,
		Status:           "active",
		Source:           SubscriptionGrantOrder,
		LastResetTime:    0,
		NextResetTime:    0,
	}
	create := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&balance)
	if create.Error != nil {
		return nil, create.Error
	}
	if create.RowsAffected == 1 {
		return &balance, nil
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND entitlement_type = ?", userId, SubscriptionEntitlementCreditBalance).
		First(&balance).Error; err != nil {
		return nil, err
	}
	return &balance, nil
}

func hasUsableSubscriptionTx(tx *gorm.DB, userId int) (bool, error) {
	now := getDBTimestampTx(tx)
	var subscriptions []UserSubscription
	if err := tx.Where("user_id = ? AND status = ? AND ((entitlement_type = ? AND end_time > ?) OR entitlement_type = ?)", userId, "active", SubscriptionEntitlementTimed, now, SubscriptionEntitlementCreditBalance).Find(&subscriptions).Error; err != nil {
		return false, err
	}
	for i := range subscriptions {
		subscription := &subscriptions[i]
		plan, err := getSubscriptionPlanByIdTx(tx, subscription.PlanId)
		if err != nil {
			return false, err
		}
		if !plan.Enabled {
			continue
		}
		if usable, _ := isBillableSubscriptionCandidate(subscription, plan, 1); usable {
			return true, nil
		}
	}
	return false, nil
}

func creditBalanceGrantResult(ledger *CreditBalanceLedger, planId int, active bool) *CreditBalanceGrantResult {
	return &CreditBalanceGrantResult{
		UserSubscriptionId: ledger.UserSubscriptionId,
		PlanId:             planId,
		GrossCredit:        ledger.GrossCredit,
		DebtOffset:         ledger.DebtOffset,
		AvailableCredit:    ledger.AvailableCreditAfter,
		SettlementDebt:     ledger.SettlementDebtAfter,
		BalanceBefore:      ledger.BalanceBefore,
		BalanceAfter:       ledger.BalanceAfter,
		Active:             active,
		LedgerId:           ledger.Id,
		Status:             creditBalanceStatus(ledger.BalanceAfter),
	}
}

func creditBalanceStatus(signedBalance int64) string {
	if signedBalance < 0 {
		return CreditBalanceStatusDebt
	}
	if signedBalance == 0 {
		return CreditBalanceStatusExhausted
	}
	return CreditBalanceStatusAvailable
}

func marshalUserSetting(setting interface{}) (string, error) {
	payload, err := common.Marshal(setting)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func minInt64(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func ValidateCreditBalancePurchaseOption(plan *SubscriptionPlan, creditPlan *SubscriptionPlan) error {
	if plan == nil || creditPlan == nil {
		return errors.New("Credit 余额购买配置不存在")
	}
	if plan.EntitlementType == SubscriptionEntitlementCreditBalance {
		return errors.New("Credit 余额套餐不能作为充值选项")
	}
	if !plan.Enabled || !plan.PublicVisible {
		return errors.New("套餐不可购买")
	}
	if plan.DurationUnit != SubscriptionDurationMonth || plan.DurationValue != 1 || NormalizeResetPeriod(plan.QuotaResetPeriod) != SubscriptionResetMonthly || plan.MonthlyTokenLimit <= 0 || plan.IsTrial || plan.InviteTrial {
		return errors.New("只有标准单月计时套餐可购买 Credit 余额")
	}
	if !plan.UnlimitedPurchaseEnabled {
		return errors.New("该套餐未开启 Credit 余额购买资格")
	}
	if !creditPlan.Enabled || !creditPlan.CreditBalanceConfigured || !creditPlan.CreditBalancePurchaseEnabled {
		return errors.New("Credit 余额购买入口未开启")
	}
	return nil
}

func ValidateCreditBalanceRedemptionOption(plan *SubscriptionPlan, creditPlan *SubscriptionPlan) error {
	if plan == nil || creditPlan == nil {
		return ErrCreditBalanceRedemptionUnavailable
	}
	if !creditPlan.Enabled || !creditPlan.CreditBalanceConfigured || !creditPlan.CreditBalanceRedemptionEnabled {
		return ErrCreditBalanceRedemptionUnavailable
	}
	if !plan.Enabled || strings.TrimSpace(plan.EntitlementType) != SubscriptionEntitlementTimed {
		return ErrRedemptionPlanIneligible
	}
	if plan.DurationUnit != SubscriptionDurationMonth || plan.DurationValue != 1 || NormalizeResetPeriod(plan.QuotaResetPeriod) != SubscriptionResetMonthly || plan.MonthlyTokenLimit <= 0 || plan.IsTrial || plan.InviteTrial || !plan.UnlimitedPurchaseEnabled {
		return ErrRedemptionPlanIneligible
	}
	return nil
}

func CreditBalanceStateForUser(userId int, activeSubscriptionId int) (*CreditBalanceGrantResult, *UserSubscription, error) {
	if userId <= 0 {
		return nil, nil, errors.New("invalid userId")
	}
	var balance UserSubscription
	query := DB.Where("user_id = ? AND entitlement_type = ?", userId, SubscriptionEntitlementCreditBalance).Limit(1).Find(&balance)
	if query.Error != nil {
		return nil, nil, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, nil, nil
	}
	if balance.TokenLimit < 0 || balance.TokenUsed < 0 {
		return nil, nil, fmt.Errorf("invalid credit balance aggregate for subscription %d", balance.Id)
	}
	signed := balance.TokenLimit - balance.TokenUsed
	state := &CreditBalanceGrantResult{
		UserSubscriptionId: balance.Id,
		PlanId:             balance.PlanId,
		AvailableCredit:    maxInt64(signed, 0),
		SettlementDebt:     maxInt64(-signed, 0),
		BalanceAfter:       signed,
		Active:             activeSubscriptionId == balance.Id,
		Status:             creditBalanceStatus(signed),
	}
	return state, &balance, nil
}
