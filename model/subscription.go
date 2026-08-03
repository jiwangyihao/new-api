package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Subscription duration units
const (
	SubscriptionDurationYear   = "year"
	SubscriptionDurationMonth  = "month"
	SubscriptionDurationDay    = "day"
	SubscriptionDurationHour   = "hour"
	SubscriptionDurationCustom = "custom"
)

// Subscription quota reset period
const (
	SubscriptionResetNever   = "never"
	SubscriptionResetDaily   = "daily"
	SubscriptionResetWeekly  = "weekly"
	SubscriptionResetMonthly = "monthly"
	SubscriptionResetCustom  = "custom"
)

const (
	SubscriptionEntitlementTimed         = "timed"
	SubscriptionEntitlementCreditBalance = "credit_balance"
	creditBalancePlanSingletonKey        = "global"
	creditBalanceUserSingletonKey        = "credit_balance"
)
const (
	SubscriptionStatusActive    = "active"
	SubscriptionStatusExpired   = "expired"
	SubscriptionStatusCancelled = "cancelled"
	SubscriptionStatusConverted = "converted"
)

const (
	SubscriptionPurchaseModeTimed         = SubscriptionEntitlementTimed
	SubscriptionPurchaseModeCreditBalance = SubscriptionEntitlementCreditBalance
)

const (
	SubscriptionBillingStrategySingleActive   = "single_active"
	SubscriptionBillingStrategyActiveFallback = "active_fallback"
	SubscriptionBillingStrategyTimedFirst     = "timed_first"
)

func NormalizeSubscriptionBillingStrategy(strategy string) string {
	switch strings.TrimSpace(strategy) {
	case SubscriptionBillingStrategySingleActive,
		SubscriptionBillingStrategyActiveFallback,
		SubscriptionBillingStrategyTimedFirst:
		return strings.TrimSpace(strategy)
	default:
		return SubscriptionBillingStrategySingleActive
	}
}

func ValidateSubscriptionBillingStrategy(strategy string) error {
	switch strings.TrimSpace(strategy) {
	case SubscriptionBillingStrategySingleActive,
		SubscriptionBillingStrategyActiveFallback,
		SubscriptionBillingStrategyTimedFirst:
		return nil
	default:
		return errors.New("invalid subscription billing strategy")
	}
}

const (
	SubscriptionGrantOrder                    = "order"
	SubscriptionGrantRedemption               = "redemption"
	SubscriptionGrantAdmin                    = "admin"
	SubscriptionGrantCompensation             = "compensation"
	SubscriptionGrantMonthlyInviteEntitlement = "monthly_invite_entitlement"
)

var (
	ErrSubscriptionOrderNotFound         = errors.New("subscription order not found")
	ErrSubscriptionOrderStatusInvalid    = errors.New("subscription order status invalid")
	ErrSubscriptionOrderSnapshotMismatch = errors.New("subscription order entitlement snapshot mismatch")
	ErrNoActiveSubscription              = errors.New("no active subscription")
)

const (
	PaymentProviderBalance      = "balance"
	PaymentMethodAccountBalance = "account_balance"
)

const (
	subscriptionPlanCacheNamespace     = "new-api:subscription_plan:v1"
	subscriptionPlanInfoCacheNamespace = "new-api:subscription_plan_info:v1"
)

const primaryBillableSubscriptionOrder = "CASE WHEN grant_reason IN ('trial_code', 'invite_trial') AND token_limit = 0 THEN 1 WHEN grant_reason = 'admin' AND token_limit = 0 THEN 2 ELSE 0 END asc, end_time asc, id asc"

var (
	subscriptionPlanCacheOnce     sync.Once
	subscriptionPlanInfoCacheOnce sync.Once

	subscriptionPlanCache            *cachex.HybridCache[SubscriptionPlan]
	subscriptionPlanInfoCache        *cachex.HybridCache[SubscriptionPlanInfo]
	primaryBillableSubscriptionCache sync.Map
)

func subscriptionPlanCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_TTL", 300)
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanInfoCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_TTL", 120)
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_CAP", 5000)
	if capacity <= 0 {
		capacity = 5000
	}
	return capacity
}

func subscriptionPlanInfoCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_CAP", 10000)
	if capacity <= 0 {
		capacity = 10000
	}
	return capacity
}

func getSubscriptionPlanCache() *cachex.HybridCache[SubscriptionPlan] {
	subscriptionPlanCacheOnce.Do(func() {
		ttl := subscriptionPlanCacheTTL()
		subscriptionPlanCache = cachex.NewHybridCache[SubscriptionPlan](cachex.HybridCacheConfig[SubscriptionPlan]{
			Namespace: cachex.Namespace(subscriptionPlanCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlan]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlan] {
				return hot.NewHotCache[string, SubscriptionPlan](hot.LRU, subscriptionPlanCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanCache
}

func getSubscriptionPlanInfoCache() *cachex.HybridCache[SubscriptionPlanInfo] {
	subscriptionPlanInfoCacheOnce.Do(func() {
		ttl := subscriptionPlanInfoCacheTTL()
		subscriptionPlanInfoCache = cachex.NewHybridCache[SubscriptionPlanInfo](cachex.HybridCacheConfig[SubscriptionPlanInfo]{
			Namespace: cachex.Namespace(subscriptionPlanInfoCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlanInfo]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlanInfo] {
				return hot.NewHotCache[string, SubscriptionPlanInfo](hot.LRU, subscriptionPlanInfoCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanInfoCache
}

func subscriptionPlanCacheKey(id int) string {
	if id <= 0 {
		return ""
	}
	return strconv.Itoa(id)
}

func InvalidateSubscriptionPlanCache(planId int) {
	if planId <= 0 {
		return
	}
	cache := getSubscriptionPlanCache()
	_, _ = cache.DeleteMany([]string{subscriptionPlanCacheKey(planId)})
	infoCache := getSubscriptionPlanInfoCache()
	_ = infoCache.Purge()
}

// ClearSubscriptionPlanCacheForTest clears both plan caches between isolated test databases.
func ClearSubscriptionPlanCacheForTest() {
	_ = getSubscriptionPlanCache().Purge()
	_ = getSubscriptionPlanInfoCache().Purge()
}

// Subscription plan
type SubscriptionPlan struct {
	Id int `json:"id"`

	Title    string `json:"title" gorm:"type:varchar(128);not null"`
	Subtitle string `json:"subtitle" gorm:"type:varchar(255);default:''"`

	// PriceAmount is the legacy display amount; a non-nil PriceAmountMicros is authoritative for valuation.
	PriceAmount       float64 `json:"price_amount" gorm:"precision:19;scale:6;not null;default:0"`
	PriceAmountMicros *int64  `json:"price_amount_micros,string" gorm:"type:bigint"`
	Currency          string  `json:"currency" gorm:"type:varchar(8);not null;default:'USD'"`
	ValuationCurrency *string `json:"valuation_currency" gorm:"type:varchar(8)"`

	DurationUnit  string `json:"duration_unit" gorm:"type:varchar(16);not null;default:'month'"`
	DurationValue int    `json:"duration_value" gorm:"type:int;not null;default:1"`
	CustomSeconds int64  `json:"custom_seconds" gorm:"type:bigint;not null;default:0"`

	Enabled   bool `json:"enabled" gorm:"default:true"`
	SortOrder int  `json:"sort_order" gorm:"type:int;default:0"`

	StripePriceId  string `json:"stripe_price_id" gorm:"type:varchar(128);default:''"`
	CreemProductId string `json:"creem_product_id" gorm:"type:varchar(128);default:''"`
	KyrenProductId string `json:"kyren_product_id" gorm:"type:varchar(128);default:''"`

	// Max purchases per user (0 = unlimited)
	MaxPurchasePerUser int `json:"max_purchase_per_user" gorm:"type:int;default:0"`

	// Legacy business group column kept for old databases; ignored by runtime.
	UpgradeGroup string `json:"-" gorm:"type:varchar(64);default:''"`

	// Total quota (amount in quota units, 0 = unlimited)
	TotalAmount int64 `json:"total_amount" gorm:"type:bigint;not null;default:0"`

	MonthlyTokenLimit    int64   `json:"monthly_token_limit" gorm:"type:bigint;not null;default:0"`
	ConcurrencyLimit     int     `json:"concurrency_limit" gorm:"type:int;not null;default:0"`
	QueueCapacity        int     `json:"queue_capacity" gorm:"type:int;not null;default:0"`
	GPTAbuseWarningLimit int     `json:"gpt_abuse_warning_limit" gorm:"type:int;not null;default:0"`
	IsTrial              bool    `json:"is_trial" gorm:"default:false"`
	InviteTrial          bool    `json:"invite_trial" gorm:"default:false"`
	PublicVisible        bool    `json:"public_visible" gorm:"default:true"`
	TrialDurationHours   int     `json:"trial_duration_hours" gorm:"type:int;not null;default:0"`
	RewardEligible       bool    `json:"reward_eligible" gorm:"default:true"`
	BusinessCode         *string `json:"business_code" gorm:"type:varchar(64);uniqueIndex"`

	EntitlementType                string  `json:"entitlement_type" gorm:"type:varchar(32);not null;default:'timed';index"`
	SingletonKey                   *string `json:"-" gorm:"type:varchar(32);uniqueIndex:idx_subscription_plans_singleton_key"`
	ModelLimits                    string  `json:"-" gorm:"type:text"` // Legacy storage only; subscription billing ignores model scope.
	CreditBalanceConfigured        bool    `json:"credit_balance_configured" gorm:"not null;default:false"`
	CreditBalancePurchaseEnabled   bool    `json:"credit_balance_purchase_enabled" gorm:"not null;default:false"`
	CreditBalanceRedemptionEnabled bool    `json:"credit_balance_redemption_enabled" gorm:"not null;default:false"`
	CreditBalanceConversionEnabled bool    `json:"credit_balance_conversion_enabled" gorm:"not null;default:false"`
	UnlimitedPurchaseEnabled       bool    `json:"unlimited_purchase_enabled" gorm:"not null;default:false"`
	TimedConversionEnabled         bool    `json:"timed_conversion_enabled" gorm:"not null;default:false"`
	ConversionGuardVersion         int64   `json:"-" gorm:"type:bigint;not null;default:0"`

	// Quota reset period for plan
	QuotaResetPeriod        string `json:"quota_reset_period" gorm:"type:varchar(16);default:'never'"`
	QuotaResetCustomSeconds int64  `json:"quota_reset_custom_seconds" gorm:"type:bigint;default:0"`

	CreatedAt                int64                         `json:"created_at" gorm:"bigint"`
	UpdatedAt                int64                         `json:"updated_at" gorm:"bigint"`
	ChannelCreditEquivalents []PlanChannelCreditEquivalent `json:"channel_credit_equivalents" gorm:"-"`
	ChannelTokenEquivalents  []PlanChannelTokenEquivalent  `json:"channel_token_equivalents" gorm:"-"`
}

func (p *SubscriptionPlan) BeforeCreate(tx *gorm.DB) error {
	if err := p.normalizeEntitlementIdentity(); err != nil {
		return err
	}
	now := common.GetTimestamp()
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (p *SubscriptionPlan) BeforeUpdate(tx *gorm.DB) error {
	if err := p.normalizeEntitlementIdentity(); err != nil {
		return err
	}
	p.UpdatedAt = common.GetTimestamp()
	return nil
}
func (p *SubscriptionPlan) normalizeEntitlementIdentity() error {
	switch strings.TrimSpace(p.EntitlementType) {
	case "", SubscriptionEntitlementTimed:
		p.EntitlementType = SubscriptionEntitlementTimed
		p.SingletonKey = nil
	case SubscriptionEntitlementCreditBalance:
		p.EntitlementType = SubscriptionEntitlementCreditBalance
		key := creditBalancePlanSingletonKey
		p.SingletonKey = &key
	default:
		return fmt.Errorf("invalid subscription entitlement type: %s", p.EntitlementType)
	}
	return nil
}

// Subscription order (payment -> webhook -> create UserSubscription)
type SubscriptionOrder struct {
	Id                      int     `json:"id"`
	UserId                  int     `json:"user_id" gorm:"index"`
	PlanId                  int     `json:"plan_id" gorm:"index"`
	Money                   float64 `json:"money"`
	AmountCents             int64   `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
	Currency                string  `json:"currency" gorm:"type:varchar(8);not null;default:''"`
	CreditGrantAmount       int64   `json:"-" gorm:"type:bigint;not null;default:0"`
	CreditTargetPlanID      int     `json:"-" gorm:"type:int;not null;default:0"`
	FulfilledSubscriptionID int     `json:"-" gorm:"type:int;not null;default:0;index"`
	RecoveryType            string  `json:"recovery_type,omitempty" gorm:"type:varchar(32);not null;default:''"`
	RecoveryTime            int64   `json:"recovery_time,omitempty" gorm:"type:bigint;not null;default:0;index"`
	RecoveryLedgerID        int     `json:"recovery_ledger_id,omitempty" gorm:"type:int;not null;default:0;index"`
	RecoveryReason          string  `json:"recovery_reason,omitempty" gorm:"type:varchar(255);not null;default:''"`
	ProviderTransactionID   string  `json:"-" gorm:"type:varchar(255);not null;default:'';index"`
	ProviderOrderID         string  `json:"-" gorm:"type:varchar(255);not null;default:'';index"`
	ProviderInvoiceID       string  `json:"-" gorm:"type:varchar(255);not null;default:'';index"`
	ProviderSubscriptionID  string  `json:"-" gorm:"type:varchar(255);not null;default:'';index"`

	TradeNo         string `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	Status          string `json:"status"`
	CreateTime      int64  `json:"create_time"`
	CompleteTime    int64  `json:"complete_time"`

	ProviderPayload     string `json:"provider_payload" gorm:"type:text"`
	KyrenSnapshot       string `json:"kyren_snapshot" gorm:"type:text"`
	EntitlementSnapshot string `json:"entitlement_snapshot" gorm:"type:text"`
}

func SubscriptionPlanAmountSnapshot(plan *SubscriptionPlan) (amountCents int64, currency string, ok bool) {
	if plan == nil || math.IsNaN(plan.PriceAmount) || math.IsInf(plan.PriceAmount, 0) || plan.PriceAmount < 0 {
		return 0, "", false
	}
	currency = strings.ToUpper(strings.TrimSpace(plan.Currency))
	if currency == "" {
		return 0, "", false
	}
	amount, err := decimal.NewFromString(strconv.FormatFloat(plan.PriceAmount, 'f', 6, 64))
	if err != nil {
		return 0, "", false
	}
	cents := amount.Mul(decimal.NewFromInt(100))
	if !cents.IsInteger() || cents.LessThan(decimal.Zero) || !cents.BigInt().IsInt64() || cents.Cmp(decimal.NewFromInt(math.MaxInt64)) > 0 {
		return 0, "", false
	}
	return cents.IntPart(), currency, true
}

func (o *SubscriptionOrder) Insert() error {
	if o.CreateTime == 0 {
		o.CreateTime = common.GetTimestamp()
	}
	return DB.Create(o).Error
}

func (o *SubscriptionOrder) Update() error {
	return DB.Save(o).Error
}

func GetSubscriptionOrderByTradeNo(tradeNo string) *SubscriptionOrder {
	if tradeNo == "" {
		return nil
	}
	var order SubscriptionOrder
	if err := DB.Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return nil
	}
	return &order
}

var ErrKyrenSubscriptionOrderLeaseMismatch = errors.New("kyren subscription order lease mismatch")

func ClaimPendingKyrenSubscriptionOrder(tradeNo string) (bool, int64, error) {
	if tradeNo == "" {
		return false, 0, ErrSubscriptionOrderNotFound
	}
	leaseTime := common.GetTimestamp()
	result := DB.Model(&SubscriptionOrder{}).
		Where("trade_no = ? AND payment_provider = ? AND status = ?", tradeNo, PaymentProviderKyren, common.TopUpStatusPending).
		Updates(map[string]any{"status": common.TopUpStatusFailed, "complete_time": leaseTime})
	if result.Error != nil {
		return false, 0, result.Error
	}
	if result.RowsAffected == 0 {
		return false, 0, nil
	}
	return true, leaseTime, nil
}

func RecoverStaleClaimedKyrenSubscriptionOrder(tradeNo string, staleBefore int64) (bool, int64, error) {
	if tradeNo == "" {
		return false, 0, ErrSubscriptionOrderNotFound
	}
	leaseTime := common.GetTimestamp()
	result := DB.Model(&SubscriptionOrder{}).
		Where("trade_no = ? AND payment_provider = ? AND status = ? AND complete_time > 0 AND complete_time <= ?", tradeNo, PaymentProviderKyren, common.TopUpStatusFailed, staleBefore).
		Update("complete_time", leaseTime)
	if result.Error != nil {
		return false, 0, result.Error
	}
	if result.RowsAffected == 0 {
		return false, 0, nil
	}
	return true, leaseTime, nil
}

func MarkClaimedKyrenSubscriptionOrderSuccessTx(tx *gorm.DB, order *SubscriptionOrder, leaseTime int64) error {
	if tx == nil || order == nil || order.TradeNo == "" || leaseTime <= 0 {
		return errors.New("invalid subscription order")
	}
	updates := map[string]any{
		"status":        common.TopUpStatusSuccess,
		"complete_time": order.CompleteTime,
	}
	if order.ProviderPayload != "" {
		updates["provider_payload"] = order.ProviderPayload
	}
	if order.PaymentMethod != "" {
		updates["payment_method"] = order.PaymentMethod
	}
	result := tx.Model(&SubscriptionOrder{}).
		Where("trade_no = ? AND payment_provider = ? AND status = ? AND complete_time = ?", order.TradeNo, PaymentProviderKyren, common.TopUpStatusFailed, leaseTime).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrKyrenSubscriptionOrderLeaseMismatch
	}
	order.Status = common.TopUpStatusSuccess
	return nil
}

func RestoreClaimedKyrenSubscriptionOrder(tradeNo string, leaseTime int64) error {
	if tradeNo == "" || leaseTime <= 0 {
		return nil
	}
	updates := map[string]any{"status": common.TopUpStatusPending, "complete_time": int64(0)}
	return DB.Model(&SubscriptionOrder{}).
		Where("trade_no = ? AND payment_provider = ? AND status = ? AND complete_time = ?", tradeNo, PaymentProviderKyren, common.TopUpStatusFailed, leaseTime).
		Updates(updates).Error
}

// User subscription instance
type UserSubscription struct {
	Id              int     `json:"id" gorm:"index:idx_user_sub_active_order,priority:4"`
	UserId          int     `json:"user_id" gorm:"index;index:idx_user_sub_active,priority:1;index:idx_user_sub_active_order,priority:1;uniqueIndex:idx_user_subscription_singleton,priority:1"`
	PlanId          int     `json:"plan_id" gorm:"index"`
	EntitlementType string  `json:"entitlement_type" gorm:"type:varchar(32);not null;default:'timed';index"`
	SingletonKey    *string `json:"-" gorm:"type:varchar(32);uniqueIndex:idx_user_subscription_singleton,priority:2"`

	AmountTotal int64 `json:"amount_total" gorm:"type:bigint;not null;default:0"`
	AmountUsed  int64 `json:"amount_used" gorm:"type:bigint;not null;default:0"`

	TokenLimit              int64  `json:"token_limit" gorm:"type:bigint;not null;default:0"`
	TokenUsed               int64  `json:"token_used" gorm:"type:bigint;not null;default:0"`
	ConcurrencyLimit        int    `json:"concurrency_limit" gorm:"type:int;not null;default:0"`
	GrantReason             string `json:"grant_reason" gorm:"type:varchar(32);default:'';index"`
	GrantSourceUserId       int    `json:"grant_source_user_id" gorm:"type:int;default:0;index"`
	LastGrantedAt           int64  `json:"-" gorm:"type:bigint;not null;default:0;index"`
	LastGrantCreditSnapshot *int64 `json:"-" gorm:"type:bigint"`
	LastGrantTimeSource     string `json:"-" gorm:"type:varchar(64);not null;default:''"`
	LastGrantSource         string `json:"-" gorm:"type:varchar(32);not null;default:''"`

	StartTime                 int64  `json:"start_time" gorm:"bigint"`
	EndTime                   int64  `json:"end_time" gorm:"bigint;index;index:idx_user_sub_active,priority:3;index:idx_user_sub_active_order,priority:3"`
	Status                    string `json:"status" gorm:"type:varchar(32);index;index:idx_user_sub_active,priority:2;index:idx_user_sub_active_order,priority:2"` // active/expired/cancelled
	ConvertedAt               int64  `json:"converted_at,omitempty" gorm:"type:bigint;not null;default:0;index"`
	ConversionId              int    `json:"-" gorm:"not null;default:0;index"`
	ConvertedToSubscriptionId int    `json:"-" gorm:"not null;default:0;index"`

	Source string `json:"source" gorm:"type:varchar(32);default:'order'"` // order/admin

	LastResetTime int64 `json:"last_reset_time" gorm:"type:bigint;default:0"`
	NextResetTime int64 `json:"next_reset_time" gorm:"type:bigint;default:0;index"`

	UpgradeGroup  string `json:"-" gorm:"type:varchar(64);default:''"`
	PrevUserGroup string `json:"-" gorm:"type:varchar(64);default:''"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (s *UserSubscription) BeforeCreate(tx *gorm.DB) error {
	if err := s.normalizeEntitlementIdentity(); err != nil {
		return err
	}
	now := common.GetTimestamp()
	s.CreatedAt = now
	s.UpdatedAt = now
	return nil
}

func (s *UserSubscription) BeforeUpdate(tx *gorm.DB) error {
	if err := s.normalizeEntitlementIdentity(); err != nil {
		return err
	}
	s.UpdatedAt = common.GetTimestamp()
	return nil
}
func (s *UserSubscription) normalizeEntitlementIdentity() error {
	switch strings.TrimSpace(s.EntitlementType) {
	case "", SubscriptionEntitlementTimed:
		s.EntitlementType = SubscriptionEntitlementTimed
		s.SingletonKey = nil
	case SubscriptionEntitlementCreditBalance:
		s.EntitlementType = SubscriptionEntitlementCreditBalance
		key := creditBalanceUserSingletonKey
		s.SingletonKey = &key
	default:
		return fmt.Errorf("invalid subscription entitlement type: %s", s.EntitlementType)
	}
	return nil
}

type UserSubscriptionCreationResult struct {
	Subscription   *UserSubscription
	EventStartTime int64
	EventEndTime   int64
}

type SubscriptionOrderCompletionResult struct {
	Subscription         *UserSubscription         `json:"subscription,omitempty"`
	CreditBalance        *CreditBalanceGrantResult `json:"credit_balance,omitempty"`
	PurchaseMode         string                    `json:"purchase_mode"`
	Transitioned         bool                      `json:"transitioned"`
	SourceSubscriptionId int                       `json:"source_subscription_id"`
	EventStartTime       int64                     `json:"event_start_time"`
	EventEndTime         int64                     `json:"event_end_time"`
	InviterId            int                       `json:"inviter_id"`
}

type SubscriptionSummary struct {
	Subscription *UserSubscription `json:"subscription"`
	Plan         *SubscriptionPlan `json:"plan,omitempty"`
}

type SubscriptionConversionAudit struct {
	ConversionId         int
	SourceSubscriptionId int
	TargetSubscriptionId int
	SourceStatusBefore   string
	SourceStatusAfter    string
	TargetStatus         string
	ConvertedAt          int64
}

type AdminSubscriptionSummary struct {
	Subscription    *UserSubscription
	Plan            *SubscriptionPlan
	ConversionAudit *SubscriptionConversionAudit
}

type PublicUserSubscription struct {
	Id                int    `json:"id"`
	UserId            int    `json:"user_id"`
	PlanId            int    `json:"plan_id"`
	EntitlementType   string `json:"entitlement_type"`
	AmountTotal       int64  `json:"amount_total"`
	AmountUsed        int64  `json:"amount_used"`
	TokenLimit        int64  `json:"token_limit"`
	TokenUsed         int64  `json:"token_used"`
	ConcurrencyLimit  int    `json:"concurrency_limit"`
	QueueCapacity     int    `json:"queue_capacity"`
	GrantReason       string `json:"grant_reason"`
	GrantSourceUserId int    `json:"grant_source_user_id"`
	StartTime         int64  `json:"start_time"`
	EndTime           int64  `json:"end_time"`
	EffectiveEndTime  int64  `json:"effective_end_time,omitempty"`
	Status            string `json:"status"`
	Source            string `json:"source"`
	LastResetTime     int64  `json:"last_reset_time"`
	NextResetTime     int64  `json:"next_reset_time"`
	UpgradeGroup      string `json:"-"`
	PrevUserGroup     string `json:"-"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
	IsActiveSelected  bool   `json:"is_active_selected,omitempty"`
	CanResetQuota     bool   `json:"can_reset_quota,omitempty"`
	SourceLabel       string `json:"source_label,omitempty"`
}

type PublicSubscriptionSummary struct {
	Subscription *PublicUserSubscription `json:"subscription"`
	Plan         *SubscriptionPlan       `json:"plan,omitempty"`
}

type SelfSubscriptionSummary struct {
	ActiveSubscriptionId     int                                   `json:"active_subscription_id,omitempty"`
	BillingStrategy          string                                `json:"billing_strategy"`
	BillingCandidateIds      []int                                 `json:"billing_candidate_subscription_ids"`
	ActiveCount              int                                   `json:"active_count"`
	SubscriptionId           int                                   `json:"subscription_id"`
	PlanId                   int                                   `json:"plan_id"`
	PrimaryPlanTitle         string                                `json:"primary_plan_title"`
	TokenLimit               int64                                 `json:"token_limit"`
	TokenUsed                int64                                 `json:"token_used"`
	TokenRemaining           int64                                 `json:"token_remaining"`
	TokenUnlimited           bool                                  `json:"token_unlimited"`
	ConcurrencyLimit         int                                   `json:"concurrency_limit"`
	QueueCapacity            int                                   `json:"queue_capacity"`
	GPTAbuseWarningLimit     int                                   `json:"gpt_abuse_warning_limit"`
	GPTAbuseWarningCount     int                                   `json:"gpt_abuse_warning_count"`
	GPTAbuseWarningRemaining int                                   `json:"gpt_abuse_warning_remaining"`
	GPTAbuseSuspendedUntil   int64                                 `json:"gpt_abuse_suspended_until,omitempty"`
	GPTAbuseLimitEnabled     bool                                  `json:"gpt_abuse_limit_enabled"`
	NextResetTime            int64                                 `json:"next_reset_time,omitempty"`
	EndTime                  int64                                 `json:"end_time,omitempty"`
	ChannelCreditEquivalents []SubscriptionChannelCreditEquivalent `json:"channel_credit_equivalents" gorm:"-"`
	ChannelTokenEquivalents  []SubscriptionChannelTokenEquivalent  `json:"channel_token_equivalents" gorm:"-"`
}

func calcPlanEndTime(start time.Time, plan *SubscriptionPlan) (int64, error) {
	if plan == nil {
		return 0, errors.New("plan is nil")
	}
	if plan.DurationValue <= 0 && plan.DurationUnit != SubscriptionDurationCustom {
		return 0, errors.New("duration_value must be > 0")
	}
	switch plan.DurationUnit {
	case SubscriptionDurationYear:
		return start.AddDate(plan.DurationValue, 0, 0).Unix(), nil
	case SubscriptionDurationMonth:
		return start.AddDate(0, plan.DurationValue, 0).Unix(), nil
	case SubscriptionDurationDay:
		return start.Add(time.Duration(plan.DurationValue) * 24 * time.Hour).Unix(), nil
	case SubscriptionDurationHour:
		return start.Add(time.Duration(plan.DurationValue) * time.Hour).Unix(), nil
	case SubscriptionDurationCustom:
		if plan.CustomSeconds <= 0 {
			return 0, errors.New("custom_seconds must be > 0")
		}
		return start.Add(time.Duration(plan.CustomSeconds) * time.Second).Unix(), nil
	default:
		return 0, fmt.Errorf("invalid duration_unit: %s", plan.DurationUnit)
	}
}

func NormalizeResetPeriod(period string) string {
	switch strings.TrimSpace(period) {
	case SubscriptionResetDaily, SubscriptionResetWeekly, SubscriptionResetMonthly, SubscriptionResetCustom:
		return strings.TrimSpace(period)
	default:
		return SubscriptionResetNever
	}
}

func calcNextResetTime(base time.Time, plan *SubscriptionPlan, endUnix int64) int64 {
	if plan == nil {
		return 0
	}
	period := NormalizeResetPeriod(plan.QuotaResetPeriod)
	if period == SubscriptionResetNever {
		return 0
	}
	var next time.Time
	switch period {
	case SubscriptionResetDaily:
		next = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).
			AddDate(0, 0, 1)
	case SubscriptionResetWeekly:
		// Align to next Monday 00:00
		weekday := int(base.Weekday()) // Sunday=0
		// Convert to Monday=1..Sunday=7
		if weekday == 0 {
			weekday = 7
		}
		daysUntil := 8 - weekday
		next = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).
			AddDate(0, 0, daysUntil)
	case SubscriptionResetMonthly:
		// Align to first day of next month 00:00
		next = time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, base.Location()).
			AddDate(0, 1, 0)
	case SubscriptionResetCustom:
		if plan.QuotaResetCustomSeconds <= 0 {
			return 0
		}
		next = base.Add(time.Duration(plan.QuotaResetCustomSeconds) * time.Second)
	default:
		return 0
	}
	if endUnix > 0 && next.Unix() > endUnix {
		return 0
	}
	return next.Unix()
}

func GetSubscriptionPlanById(id int) (*SubscriptionPlan, error) {
	return getSubscriptionPlanByIdTx(nil, id)
}

func getSubscriptionPlanByIdTx(tx *gorm.DB, id int) (*SubscriptionPlan, error) {
	if id <= 0 {
		return nil, errors.New("invalid plan id")
	}
	key := subscriptionPlanCacheKey(id)
	if key != "" {
		if cached, found, err := getSubscriptionPlanCache().Get(key); err == nil && found {
			return &cached, nil
		}
	}
	var plan SubscriptionPlan
	query := DB
	if tx != nil {
		query = tx
	}
	if err := query.Where("id = ?", id).First(&plan).Error; err != nil {
		return nil, err
	}
	_ = getSubscriptionPlanCache().SetWithTTL(key, plan, subscriptionPlanCacheTTL())
	return &plan, nil
}

func CountUserSubscriptionsByPlan(userId int, planId int) (int64, error) {
	if userId <= 0 || planId <= 0 {
		return 0, errors.New("invalid userId or planId")
	}
	var count int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", userId, planId).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func CreateUserSubscriptionFromPlanTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, source string) (*UserSubscription, error) {
	result, err := CreateUserSubscriptionFromPlanWithResultTx(tx, userId, plan, source)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.Subscription, nil
}

func CreateUserSubscriptionFromPlanWithResultTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, source string) (*UserSubscriptionCreationResult, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if plan == nil || plan.Id == 0 {
		return nil, errors.New("invalid plan")
	}
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if err := plan.normalizeEntitlementIdentity(); err != nil {
		return nil, err
	}
	if plan.EntitlementType == SubscriptionEntitlementCreditBalance {
		return nil, errors.New("credit balance entitlements must use the dedicated credit service")
	}
	nowUnix, err := getDBTimestampStrictTx(tx)
	if err != nil {
		return nil, err
	}
	now := time.Unix(nowUnix, 0)
	var existing UserSubscription
	var sameTierPaid UserSubscription
	sameTierPaidQuery := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ? AND grant_reason = ?", userId, plan.Id, "active", nowUnix, SubscriptionGrantOrder).
		Order("end_time desc, id desc").
		Limit(1).
		Find(&sameTierPaid)
	if sameTierPaidQuery.Error != nil {
		return nil, sameTierPaidQuery.Error
	}
	existingQuery := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userId, plan.Id, "active", nowUnix).
		Order("end_time desc, id desc").
		Limit(1).
		Find(&existing)
	if existingQuery.Error != nil {
		return nil, existingQuery.Error
	}
	if isInvitationRewardSource(source) && sameTierPaidQuery.RowsAffected > 0 && sameTierPaid.EndTime > nowUnix {
		existingQuery.RowsAffected = 0
	}
	start := now
	if existingQuery.RowsAffected > 0 {
		start = time.Unix(existing.EndTime, 0)
	}
	eventStartTime := start.Unix()
	endUnix, err := calcPlanEndTime(start, plan)
	if err != nil {
		return nil, err
	}
	resetBase := now
	nextReset := calcNextResetTime(resetBase, plan, endUnix)
	lastReset := int64(0)
	if nextReset > 0 {
		lastReset = now.Unix()
	}
	grantCreditSnapshot := plan.MonthlyTokenLimit
	if existingQuery.RowsAffected > 0 {
		existing.EndTime = endUnix
		existing.TokenLimit = plan.MonthlyTokenLimit
		existing.ConcurrencyLimit = plan.ConcurrencyLimit
		if strings.TrimSpace(existing.GrantReason) == "" {
			existing.GrantReason = source
		}
		if strings.TrimSpace(existing.Source) == "" {
			existing.Source = source
		}
		existing.NextResetTime = nextReset
		existing.LastGrantedAt = nowUnix
		existing.LastGrantCreditSnapshot = &grantCreditSnapshot
		existing.LastGrantTimeSource = SubscriptionGrantTimeSourceLive
		existing.LastGrantSource = strings.TrimSpace(source)
		existing.UpdatedAt = nowUnix
		fields := []string{
			"end_time",
			"token_limit",
			"concurrency_limit",
			"next_reset_time",
			"updated_at",
			"last_granted_at",
			"last_grant_credit_snapshot",
			"last_grant_time_source",
			"last_grant_source",
		}
		if strings.TrimSpace(existing.GrantReason) == source {
			fields = append(fields, "grant_reason")
		}
		if strings.TrimSpace(existing.Source) == source {
			fields = append(fields, "source")
		}

		if err := tx.Model(&existing).Select(fields).Updates(existing).Error; err != nil {
			return nil, err
		}
		return &UserSubscriptionCreationResult{Subscription: &existing, EventStartTime: eventStartTime, EventEndTime: endUnix}, nil
	}

	sub := &UserSubscription{
		UserId:                  userId,
		PlanId:                  plan.Id,
		EntitlementType:         plan.EntitlementType,
		AmountTotal:             plan.TotalAmount,
		AmountUsed:              0,
		TokenLimit:              plan.MonthlyTokenLimit,
		TokenUsed:               0,
		ConcurrencyLimit:        plan.ConcurrencyLimit,
		GrantReason:             source,
		LastGrantedAt:           nowUnix,
		LastGrantCreditSnapshot: &grantCreditSnapshot,
		LastGrantTimeSource:     SubscriptionGrantTimeSourceLive,
		LastGrantSource:         strings.TrimSpace(source),
		StartTime:               now.Unix(),
		EndTime:                 endUnix,
		Status:                  "active",
		Source:                  source,
		LastResetTime:           lastReset,
		NextResetTime:           nextReset,
		UpgradeGroup:            "",
		PrevUserGroup:           "",
		CreatedAt:               nowUnix,
		UpdatedAt:               nowUnix,
	}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	return &UserSubscriptionCreationResult{Subscription: sub, EventStartTime: sub.StartTime, EventEndTime: sub.EndTime}, nil
}

func validateSubscriptionOrderEntitlementSnapshot(tx *gorm.DB, order *SubscriptionOrder, snapshot SubscriptionEntitlementSnapshot, purchaseMode string, actualPaymentMethod string) error {
	if tx == nil || order == nil || snapshot.PlanID <= 0 || snapshot.PlanID != order.PlanId {
		return ErrSubscriptionOrderSnapshotMismatch
	}
	snapshotProvider := strings.TrimSpace(snapshot.PaymentProvider)
	snapshotMethod := strings.TrimSpace(snapshot.ProviderPaymentMethod)
	if snapshotProvider != "" {
		if snapshotProvider != strings.TrimSpace(order.PaymentProvider) || snapshot.PaymentAmountCents <= 0 || snapshot.PaymentAmountCents != order.AmountCents || !strings.EqualFold(strings.TrimSpace(snapshot.PaymentCurrency), strings.TrimSpace(order.Currency)) || snapshotMethod == "" || snapshotMethod != strings.TrimSpace(order.PaymentMethod) {
			return ErrSubscriptionOrderSnapshotMismatch
		}
		if method := strings.TrimSpace(actualPaymentMethod); method != "" && method != snapshotMethod {
			return ErrSubscriptionOrderSnapshotMismatch
		}
	}
	if purchaseMode == SubscriptionPurchaseModeCreditBalance {
		if snapshotProvider == "" || snapshot.MonthlyTokenLimit <= 0 || order.CreditGrantAmount <= 0 || snapshot.MonthlyTokenLimit != order.CreditGrantAmount || snapshot.TargetCreditBalancePlanID <= 0 || order.CreditTargetPlanID <= 0 || snapshot.TargetCreditBalancePlanID != order.CreditTargetPlanID || strings.TrimSpace(snapshot.PlanEntitlementType) != SubscriptionEntitlementTimed {
			return ErrSubscriptionOrderSnapshotMismatch
		}
		return nil
	}
	if order.CreditGrantAmount != 0 || order.CreditTargetPlanID != 0 {
		return ErrSubscriptionOrderSnapshotMismatch
	}
	return nil
}

type subscriptionOrderProviderIdentityPayload struct {
	ProviderTransactionID  string `json:"provider_transaction_id"`
	ProviderOrderID        string `json:"provider_order_id"`
	ProviderInvoiceID      string `json:"provider_invoice_id"`
	ProviderSubscriptionID string `json:"provider_subscription_id"`
}

func subscriptionOrderProviderIdentityUpdates(providerPayload string) map[string]any {
	providerPayload = strings.TrimSpace(providerPayload)
	if providerPayload == "" {
		return nil
	}
	var payload subscriptionOrderProviderIdentityPayload
	if err := common.UnmarshalJsonStr(providerPayload, &payload); err != nil {
		return nil
	}
	updates := make(map[string]any, 4)
	for column, value := range map[string]string{
		"provider_transaction_id":  payload.ProviderTransactionID,
		"provider_order_id":        payload.ProviderOrderID,
		"provider_invoice_id":      payload.ProviderInvoiceID,
		"provider_subscription_id": payload.ProviderSubscriptionID,
	} {
		if value = strings.TrimSpace(value); value != "" {
			updates[column] = value
		}
	}
	return updates
}

func CompleteSubscriptionOrderTx(tx *gorm.DB, order *SubscriptionOrder, providerPayload string, actualPaymentMethod string) (*SubscriptionOrderCompletionResult, error) {
	if tx == nil || order == nil {
		return nil, errors.New("invalid subscription order")
	}
	purchaseMode := SubscriptionPurchaseModeTimed
	var snapshot SubscriptionEntitlementSnapshot
	var plan *SubscriptionPlan
	var err error
	hasSnapshot := strings.TrimSpace(order.EntitlementSnapshot) != ""
	if hasSnapshot {
		snapshot, err = UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(snapshot.PurchaseMode) != "" {
			purchaseMode, err = NormalizeSubscriptionPurchaseMode(snapshot.PurchaseMode)
			if err != nil {
				return nil, err
			}
		}
		if err := validateSubscriptionOrderEntitlementSnapshot(tx, order, snapshot, purchaseMode, actualPaymentMethod); err != nil {
			return nil, err
		}
	} else if order.CreditGrantAmount != 0 || order.CreditTargetPlanID != 0 {
		return nil, ErrSubscriptionOrderSnapshotMismatch
	}
	if order.Status == common.TopUpStatusSuccess {
		return subscriptionOrderCompletionResultFromExistingFulfillmentTx(tx, order, false)
	}
	if order.Status != common.TopUpStatusPending {
		return nil, ErrSubscriptionOrderStatusInvalid
	}
	if hasSnapshot {
		plan, err = SubscriptionPlanFromEntitlementSnapshot(snapshot)
	} else {
		plan, err = getSubscriptionPlanByIdTx(tx, order.PlanId)
	}
	if err != nil {
		return nil, err
	}
	completeTime, completeTimeErr := getDBTimestampStrictTx(tx)
	if completeTimeErr != nil {
		return nil, completeTimeErr
	}
	updates := map[string]any{
		"status":        common.TopUpStatusSuccess,
		"complete_time": completeTime,
	}
	if providerPayload != "" {
		updates["provider_payload"] = providerPayload
		for column, value := range subscriptionOrderProviderIdentityUpdates(providerPayload) {
			updates[column] = value
		}
	}
	if actualPaymentMethod != "" {
		updates["payment_method"] = actualPaymentMethod
	}
	claim := tx.Model(&SubscriptionOrder{}).Where("id = ? AND status = ?", order.Id, common.TopUpStatusPending).Updates(updates)
	if claim.Error != nil {
		return nil, claim.Error
	}
	if claim.RowsAffected == 0 {
		var existing SubscriptionOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", order.Id).First(&existing).Error; err != nil {
			return nil, err
		}
		if existing.Status == common.TopUpStatusSuccess {
			return subscriptionOrderCompletionResultFromExistingFulfillmentTx(tx, &existing, false)
		}
		return nil, ErrSubscriptionOrderStatusInvalid
	}
	order.Status = common.TopUpStatusSuccess
	order.CompleteTime = completeTime
	if providerPayload != "" {
		order.ProviderPayload = providerPayload
	}
	if actualPaymentMethod != "" {
		order.PaymentMethod = actualPaymentMethod
	}

	if purchaseMode == SubscriptionPurchaseModeCreditBalance {
		targetPlan, err := CreditBalancePlanFromEntitlementSnapshot(snapshot)
		if err != nil {
			return nil, err
		}
		reason := "外部支付购买 Credit 余额"
		if order.PaymentProvider == PaymentProviderBalance {
			reason = "人民币账户余额购买 Credit 余额"
		}
		var valuationSource *CreditValuationSourceSnapshot
		valuationCurrency := strings.TrimSpace(snapshot.TargetCreditBalanceValuationCurrency)
		if snapshot.ListPriceMicros != nil && *snapshot.ListPriceMicros > 0 && snapshot.MonthlyTokenLimit > 0 && valuationCurrency != "" {
			valuationSource = &CreditValuationSourceSnapshot{
				SourcePriceMicros: *snapshot.ListPriceMicros,
				SourcePlanCredit:  snapshot.MonthlyTokenLimit,
				GrossCredit:       order.CreditGrantAmount,
				SourceCurrency:    snapshot.ListPriceCurrency,
				ValuationCurrency: valuationCurrency,
				RuleVersion:       snapshot.ValuationRuleVersion,
			}
		}
		grant, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
			UserId:             order.UserId,
			GrossCredit:        order.CreditGrantAmount,
			IdempotencyKey:     order.TradeNo,
			SourceType:         CreditBalanceLedgerSourceSubscriptionOrder,
			SourceId:           order.Id,
			SourceSnapshot:     order.EntitlementSnapshot,
			Type:               CreditBalanceLedgerTypePurchase,
			TargetPlanId:       order.CreditTargetPlanID,
			TargetPlanSnapshot: targetPlan,
			ValuationSource:    valuationSource,
			PaymentProvider:    order.PaymentProvider,
			Reason:             reason,
		})
		if err != nil {
			return nil, err
		}
		order.FulfilledSubscriptionID = grant.UserSubscriptionId
		if err := tx.Model(&SubscriptionOrder{}).Where("id = ?", order.Id).Update("fulfilled_subscription_id", grant.UserSubscriptionId).Error; err != nil {
			return nil, err
		}
		if order.PaymentProvider != PaymentProviderBalance {
			if err := upsertSubscriptionTopUpTx(tx, order); err != nil {
				return nil, err
			}
		}
		if err := SetUserLastSubscriptionPurchaseModeTx(tx, order.UserId, purchaseMode); err != nil {
			return nil, err
		}
		return &SubscriptionOrderCompletionResult{
			CreditBalance: grant,
			PurchaseMode:  purchaseMode,
			Transitioned:  true,
		}, nil
	}

	creation, err := CreateUserSubscriptionFromPlanWithResultTx(tx, order.UserId, plan, SubscriptionGrantOrder)
	if err != nil {
		return nil, err
	}
	if creation != nil && creation.Subscription != nil {
		order.FulfilledSubscriptionID = creation.Subscription.Id
		if err := tx.Model(&SubscriptionOrder{}).Where("id = ?", order.Id).Update("fulfilled_subscription_id", creation.Subscription.Id).Error; err != nil {
			return nil, err
		}
	}
	if order.PaymentProvider != PaymentProviderBalance {
		if err := upsertSubscriptionTopUpTx(tx, order); err != nil {
			return nil, err
		}
	}
	result := subscriptionOrderCompletionResultFromSubscription(order, creation, true)
	result.PurchaseMode = purchaseMode
	if err := createInvitationRewardEventForSubscriptionOrderTx(tx, order, plan, result); err != nil {
		return nil, err
	}
	if err := SetUserLastSubscriptionPurchaseModeTx(tx, order.UserId, purchaseMode); err != nil {
		return nil, err
	}
	return result, nil
}
func subscriptionOrderCompletionResultFromSubscription(order *SubscriptionOrder, creation *UserSubscriptionCreationResult, transitioned bool) *SubscriptionOrderCompletionResult {
	if order == nil {
		return nil
	}
	result := &SubscriptionOrderCompletionResult{PurchaseMode: SubscriptionPurchaseModeTimed, Transitioned: transitioned}
	if creation != nil {
		result.Subscription = creation.Subscription
		result.EventStartTime = creation.EventStartTime
		result.EventEndTime = creation.EventEndTime
		if creation.Subscription != nil {
			result.SourceSubscriptionId = creation.Subscription.Id
		}
	}
	return result
}

func subscriptionOrderCompletionResultFromExistingFulfillmentTx(tx *gorm.DB, order *SubscriptionOrder, transitioned bool) (*SubscriptionOrderCompletionResult, error) {
	if tx == nil || order == nil {
		return nil, errors.New("invalid subscription order")
	}
	if strings.TrimSpace(order.EntitlementSnapshot) != "" {
		snapshot, err := UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(snapshot.PurchaseMode) == SubscriptionPurchaseModeCreditBalance {
			grant, err := FindCreditBalanceGrantBySourceTx(tx, CreditBalanceLedgerSourceSubscriptionOrder, order.Id)
			if err != nil {
				return nil, err
			}
			return &SubscriptionOrderCompletionResult{CreditBalance: grant, PurchaseMode: SubscriptionPurchaseModeCreditBalance, Transitioned: transitioned}, nil
		}
	}
	result, err := subscriptionOrderCompletionResultFromExistingEventTx(tx, order, transitioned)
	if result != nil {
		result.PurchaseMode = SubscriptionPurchaseModeTimed
	}
	return result, err
}
func subscriptionOrderCompletionResultFromExistingEventTx(tx *gorm.DB, order *SubscriptionOrder, transitioned bool) (*SubscriptionOrderCompletionResult, error) {
	if tx == nil || order == nil {
		return nil, errors.New("invalid subscription order")
	}
	var event InvitationRewardEvent
	err := tx.Where("source_type = ? AND source_id = ?", InvitationRewardEventSourceSubscriptionOrder, order.Id).First(&event).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &SubscriptionOrderCompletionResult{Transitioned: transitioned}, nil
		}
		return nil, err
	}
	result := &SubscriptionOrderCompletionResult{
		Transitioned:         transitioned,
		SourceSubscriptionId: event.SourceSubscriptionId,
		EventStartTime:       event.EventStartTime,
		EventEndTime:         event.EventEndTime,
		InviterId:            event.InviterId,
	}
	if event.SourceSubscriptionId > 0 {
		var sub UserSubscription
		if err := tx.Where("id = ?", event.SourceSubscriptionId).First(&sub).Error; err == nil {
			result.Subscription = &sub
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return result, nil
}

func createInvitationRewardEventForSubscriptionOrderTx(tx *gorm.DB, order *SubscriptionOrder, plan *SubscriptionPlan, result *SubscriptionOrderCompletionResult) error {
	if tx == nil || order == nil || plan == nil || result == nil || result.SourceSubscriptionId <= 0 {
		return nil
	}
	if shouldSkipInvitationRewardEventForPlan(plan) {
		return nil
	}
	var invitee User
	if err := tx.Select("id", "inviter_id").Where("id = ?", order.UserId).First(&invitee).Error; err != nil {
		return err
	}
	if invitee.InviterId <= 0 {
		return nil
	}
	now := common.GetTimestamp()
	event := InvitationRewardEvent{
		InviterId:            invitee.InviterId,
		InviteeId:            order.UserId,
		SourceType:           InvitationRewardEventSourceSubscriptionOrder,
		SourceId:             order.Id,
		SourceOrderId:        order.Id,
		SourceSubscriptionId: result.SourceSubscriptionId,
		SourceAmountCents:    order.AmountCents,
		SourceCurrency:       order.Currency,
		EventStartTime:       result.EventStartTime,
		EventEndTime:         result.EventEndTime,
		Status:               InvitationRewardEventStatusActive,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := createOrLoadInvitationRewardEventTx(tx, &event); err != nil {
		return err
	}
	result.InviterId = event.InviterId
	result.SourceSubscriptionId = event.SourceSubscriptionId
	result.EventStartTime = event.EventStartTime
	result.EventEndTime = event.EventEndTime
	return nil
}

func RecordInvitationRewardEventForSubscriptionOrderTx(tx *gorm.DB, order *SubscriptionOrder, plan *SubscriptionPlan, creation *UserSubscriptionCreationResult, transitioned bool) (*SubscriptionOrderCompletionResult, error) {
	result := subscriptionOrderCompletionResultFromSubscription(order, creation, transitioned)
	if err := createInvitationRewardEventForSubscriptionOrderTx(tx, order, plan, result); err != nil {
		return nil, err
	}
	return result, nil
}

func createInvitationRewardEventForSubscriptionRedemptionTx(tx *gorm.DB, redemption *Redemption, userId int, plan *SubscriptionPlan, creation *UserSubscriptionCreationResult) error {
	if tx == nil || redemption == nil || plan == nil || creation == nil || creation.Subscription == nil {
		return nil
	}
	if shouldSkipInvitationRewardEventForPlan(plan) {
		return nil
	}
	var invitee User
	if err := tx.Select("id", "inviter_id").Where("id = ?", userId).First(&invitee).Error; err != nil {
		return err
	}
	if invitee.InviterId <= 0 {
		return nil
	}
	now := common.GetTimestamp()
	event := InvitationRewardEvent{
		InviterId:            invitee.InviterId,
		InviteeId:            userId,
		SourceType:           InvitationRewardEventSourceSubscriptionRedemption,
		SourceId:             redemption.Id,
		SourceRedemptionId:   redemption.Id,
		SourceSubscriptionId: creation.Subscription.Id,
		SourceAmountCents:    redemption.AmountCents,
		SourceCurrency:       redemption.Currency,
		EventStartTime:       creation.EventStartTime,
		EventEndTime:         creation.EventEndTime,
		Status:               InvitationRewardEventStatusActive,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	return createOrLoadInvitationRewardEventTx(tx, &event)
}

func shouldSkipInvitationRewardEventForPlan(plan *SubscriptionPlan) bool {
	if plan == nil || plan.IsTrial || plan.InviteTrial {
		return true
	}
	if plan.BusinessCode != nil && strings.TrimSpace(*plan.BusinessCode) == SubscriptionGrantMonthlyInviteEntitlement {
		return true
	}
	return false
}

func createOrLoadInvitationRewardEventTx(tx *gorm.DB, event *InvitationRewardEvent) error {
	if tx == nil || event == nil {
		return errors.New("invalid invitation reward event")
	}
	if event.Status == "" {
		event.Status = InvitationRewardEventStatusActive
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = common.GetTimestamp()
	}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_type"}, {Name: "source_id"}}, DoNothing: true}).Create(event)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	return tx.Where("source_type = ? AND source_id = ?", event.SourceType, event.SourceId).First(event).Error
}

// Complete a subscription order (idempotent). Creates a UserSubscription snapshot from the plan.
// expectedPaymentProvider guards against cross-gateway callback attacks (empty skips the check).
// actualPaymentMethod updates the order's PaymentMethod to reflect the real payment type used (empty skips update).
func CompleteSubscriptionOrder(tradeNo string, providerPayload string, expectedPaymentProvider string, actualPaymentMethod string) (*SubscriptionOrderCompletionResult, error) {
	if tradeNo == "" {
		return nil, errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}
	var result *SubscriptionOrderCompletionResult
	var logUserId int
	var logPlanTitle string
	var logMoney float64
	var logPaymentMethod string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var order SubscriptionOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(refCol+" = ?", tradeNo).First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if order.Status != common.TopUpStatusPending && order.Status != common.TopUpStatusSuccess {
			return ErrSubscriptionOrderStatusInvalid
		}
		if snapshotPayload := strings.TrimSpace(order.EntitlementSnapshot); snapshotPayload != "" {
			snapshot, err := UnmarshalSubscriptionEntitlementSnapshot(snapshotPayload)
			if err != nil {
				return err
			}
			logPlanTitle = snapshot.PlanTitle
		} else {
			plan, err := getSubscriptionPlanByIdTx(tx, order.PlanId)
			if err != nil {
				return err
			}
			logPlanTitle = plan.Title
		}
		completion, err := CompleteSubscriptionOrderTx(tx, &order, providerPayload, actualPaymentMethod)
		if err != nil {
			return err
		}
		result = completion
		if completion != nil && completion.Transitioned {
			logUserId = order.UserId
			logMoney = order.Money
			logPaymentMethod = order.PaymentMethod
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if logUserId > 0 {
		invalidateUserCacheBestEffort(logUserId)
	}
	if logUserId > 0 {
		msg := fmt.Sprintf("订阅购买成功，套餐: %s，支付金额: %.2f，支付方式: %s", logPlanTitle, logMoney, logPaymentMethod)
		RecordLog(logUserId, LogTypeTopup, msg)
	}
	return result, nil
}

func upsertSubscriptionTopUpTx(tx *gorm.DB, order *SubscriptionOrder) error {
	if tx == nil || order == nil {
		return errors.New("invalid subscription order")
	}
	now := common.GetTimestamp()
	var topup TopUp
	if err := tx.Where("trade_no = ?", order.TradeNo).First(&topup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			topup = TopUp{
				UserId:          order.UserId,
				Amount:          0,
				Money:           order.Money,
				TradeNo:         order.TradeNo,
				PaymentMethod:   order.PaymentMethod,
				PaymentProvider: order.PaymentProvider,
				KyrenSnapshot:   order.KyrenSnapshot,
				CreateTime:      order.CreateTime,
				CompleteTime:    now,
				Status:          common.TopUpStatusSuccess,
			}
			return tx.Create(&topup).Error
		}
		return err
	}
	if topup.PaymentProvider != "" && topup.PaymentProvider != order.PaymentProvider {
		return ErrPaymentMethodMismatch
	}
	if topup.PaymentProvider == "" {
		topup.PaymentProvider = order.PaymentProvider
	}
	if topup.KyrenSnapshot == "" {
		topup.KyrenSnapshot = order.KyrenSnapshot
	}
	topup.Money = order.Money
	if topup.PaymentMethod == "" {
		topup.PaymentMethod = order.PaymentMethod
	} else if topup.PaymentMethod != order.PaymentMethod {
		return ErrPaymentMethodMismatch
	}
	if topup.CreateTime == 0 {
		topup.CreateTime = order.CreateTime
	}
	topup.CompleteTime = now
	topup.Status = common.TopUpStatusSuccess
	return tx.Save(&topup).Error
}

func FailSubscriptionOrder(tradeNo string, expectedPaymentProvider string) error {
	if strings.TrimSpace(tradeNo) == "" {
		return errors.New("tradeNo is empty")
	}
	result := DB.Model(&SubscriptionOrder{}).
		Where("trade_no = ? AND payment_provider = ? AND status = ?", tradeNo, expectedPaymentProvider, common.TopUpStatusPending).
		Updates(map[string]any{"status": common.TopUpStatusFailed, "complete_time": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var order SubscriptionOrder
	if err := DB.Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return ErrSubscriptionOrderNotFound
	}
	if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
		return ErrPaymentMethodMismatch
	}
	return nil
}

func ExpireSubscriptionOrder(tradeNo string, expectedPaymentProvider string) error {
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var order SubscriptionOrder
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if order.Status != common.TopUpStatusPending {
			return nil
		}
		order.Status = common.TopUpStatusExpired
		order.CompleteTime = common.GetTimestamp()
		return tx.Save(&order).Error
	})
}

// Admin bind (no payment). Creates a UserSubscription from a plan.
func AdminBindSubscription(userId int, planId int, sourceNote string) (string, error) {
	if userId <= 0 || planId <= 0 {
		return "", errors.New("invalid userId or planId")
	}
	plan, err := GetSubscriptionPlanById(planId)
	if err != nil {
		return "", err
	}
	if plan.EntitlementType == SubscriptionEntitlementCreditBalance {
		return "", errors.New("Credit 余额套餐不能通过普通绑定接口创建权益")
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		_, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, "admin")
		return err
	})
	if err != nil {
		return "", err
	}
	return "", nil
}

// GetAllActiveUserSubscriptions returns all active subscriptions for a user.
func GetAllActiveUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var subs []UserSubscription
	err := DB.Where("user_id = ? AND status = ? AND (entitlement_type = ? OR end_time > ?)", userId, "active", SubscriptionEntitlementCreditBalance, now).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs)
}

// HasActiveUserSubscription returns whether the user has any billable active subscription.
// Subscription plans do not restrict models; API-key model restrictions remain independent.
func HasActiveUserSubscription(userId int) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid userId")
	}
	now := GetDBTimestamp()
	var repairedSettingJSON string
	hasSubscription := false
	err := transactionWithUserSettingCASRetry(func(tx *gorm.DB) error {
		outcome, err := selectPrimaryBillableSubscriptionTx(tx, userId, now, 1, true, true)
		if err != nil {
			return err
		}
		repairedSettingJSON = outcome.RepairedSettingJSON
		hasSubscription = outcome.Selection != nil
		return nil
	})
	if err != nil {
		return false, err
	}
	if repairedSettingJSON != "" {
		syncSubscriptionSelectionSettingCacheAfterCommit(userId, repairedSettingJSON)
	}
	return hasSubscription, nil
}

type ActiveSubscriptionUsage struct {
	TokenLimit int64
	TokenUsed  int64
	EndTime    int64
	Unlimited  bool
}

type subscriptionTransactionHooks struct {
	afterUsageStateResolved    func()
	onPreConsumeAttemptStarted func()
}

func GetActiveDistributorSubscriptionUsage(userId int) (*ActiveSubscriptionUsage, error) {
	return getActiveDistributorSubscriptionUsage(userId, nil)
}

func getActiveDistributorSubscriptionUsage(userId int, hooks *subscriptionTransactionHooks) (*ActiveSubscriptionUsage, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	now := GetDBTimestamp()
	var state resolvedSubscriptionBillingStrategyState
	err := transactionWithUserSettingCASRetry(func(tx *gorm.DB) error {
		var user User
		if err := tx.Select("setting").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		resolved, err := resolveSubscriptionBillingStrategyStateTx(tx, userId, user.GetSetting(), user.Setting, now, true, true)
		if err != nil {
			return err
		}
		state = resolved
		if hooks != nil && hooks.afterUsageStateResolved != nil {
			hooks.afterUsageStateResolved()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if state.RepairedSettingJSON != "" {
		syncSubscriptionSelectionSettingCacheAfterCommit(userId, state.RepairedSettingJSON)
	}
	if len(state.OrderedCandidates) == 0 {
		return &ActiveSubscriptionUsage{}, nil
	}
	candidate := state.OrderedCandidates[0]
	return &ActiveSubscriptionUsage{
		TokenLimit: candidate.sub.TokenLimit,
		TokenUsed:  candidate.sub.TokenUsed,
		EndTime:    candidate.sub.EndTime,
		Unlimited:  isUnlimitedTrialSubscription(&candidate.sub, candidate.plan),
	}, nil
}

// GetAllUserSubscriptions returns all subscriptions (active and expired) for a user.
func GetAllUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	var subs []UserSubscription
	err := DB.Where("user_id = ?", userId).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs)
}

func GetAdminUserSubscriptions(userId int) ([]AdminSubscriptionSummary, error) {
	summaries, err := GetAllUserSubscriptions(userId)
	if err != nil {
		return nil, err
	}
	result := make([]AdminSubscriptionSummary, 0, len(summaries))
	if len(summaries) == 0 {
		return result, nil
	}

	statusBySubscriptionId := make(map[int]string, len(summaries))
	convertedSourceIds := make([]int, 0)
	for _, summary := range summaries {
		if summary.Subscription == nil {
			continue
		}
		statusBySubscriptionId[summary.Subscription.Id] = summary.Subscription.Status
		if summary.Subscription.Status == SubscriptionStatusConverted {
			convertedSourceIds = append(convertedSourceIds, summary.Subscription.Id)
		}
	}

	conversionsBySourceId := make(map[int]SubscriptionConversion, len(convertedSourceIds))
	if len(convertedSourceIds) > 0 {
		var conversions []SubscriptionConversion
		if err := DB.Where("user_id = ? AND source_subscription_id IN ?", userId, convertedSourceIds).Find(&conversions).Error; err != nil {
			return nil, err
		}
		for _, conversion := range conversions {
			conversionsBySourceId[conversion.SourceSubscriptionId] = conversion
		}
	}

	for _, summary := range summaries {
		adminSummary := AdminSubscriptionSummary{
			Subscription: summary.Subscription,
			Plan:         summary.Plan,
		}
		if summary.Subscription != nil && summary.Subscription.Status == SubscriptionStatusConverted {
			conversion, found := conversionsBySourceId[summary.Subscription.Id]
			if !found {
				return nil, fmt.Errorf("conversion audit missing for subscription %d", summary.Subscription.Id)
			}
			adminSummary.ConversionAudit = &SubscriptionConversionAudit{
				ConversionId:         conversion.Id,
				SourceSubscriptionId: conversion.SourceSubscriptionId,
				TargetSubscriptionId: conversion.TargetSubscriptionId,
				SourceStatusBefore:   conversion.SourceStatus,
				SourceStatusAfter:    summary.Subscription.Status,
				TargetStatus:         statusBySubscriptionId[conversion.TargetSubscriptionId],
				ConvertedAt:          conversion.ConvertedAt,
			}
		}
		result = append(result, adminSummary)
	}
	return result, nil
}

func buildSubscriptionSummaries(subs []UserSubscription) ([]SubscriptionSummary, error) {
	if len(subs) == 0 {
		return []SubscriptionSummary{}, nil
	}
	planIds := make([]int, 0, len(subs))
	seenPlanIds := make(map[int]struct{}, len(subs))
	for _, sub := range subs {
		if sub.PlanId <= 0 {
			continue
		}
		if _, ok := seenPlanIds[sub.PlanId]; ok {
			continue
		}
		seenPlanIds[sub.PlanId] = struct{}{}
		planIds = append(planIds, sub.PlanId)
	}

	plansById := make(map[int]SubscriptionPlan, len(planIds))
	if len(planIds) > 0 {
		var plans []SubscriptionPlan
		if err := DB.Where("id IN ?", planIds).Find(&plans).Error; err != nil {
			return nil, err
		}
		for _, plan := range plans {
			plansById[plan.Id] = plan
		}
	}

	result := make([]SubscriptionSummary, 0, len(subs))
	for _, sub := range subs {
		subCopy := sub
		summary := SubscriptionSummary{Subscription: &subCopy}
		if plan, ok := plansById[sub.PlanId]; ok {
			planCopy := plan
			summary.Plan = &planCopy
		}
		result = append(result, summary)
	}
	return result, nil
}

func BuildPublicSubscriptionSummaries(subs []SubscriptionSummary, activeSubscriptionId int) []PublicSubscriptionSummary {
	return buildPublicSubscriptionSummaries(subs, activeSubscriptionId, GetDBTimestamp())
}
func buildPublicSubscriptionSummaries(subs []SubscriptionSummary, activeSubscriptionId int, now int64) []PublicSubscriptionSummary {
	if len(subs) == 0 {
		return []PublicSubscriptionSummary{}
	}
	paidRemainderByTier := make(map[string]int64)
	for _, summary := range subs {
		if summary.Subscription == nil || summary.Plan == nil || !isActiveResettableSubscription(summary.Subscription, now) || !isPaidEquivalentSubscription(summary.Subscription, summary.Plan) {
			continue
		}
		remaining := summary.Subscription.EndTime - now
		if remaining <= 0 {
			continue
		}
		tier := subscriptionTierKey(summary.Plan)
		if tier == "" {
			continue
		}
		if remaining > paidRemainderByTier[tier] {
			paidRemainderByTier[tier] = remaining
		}
	}
	result := make([]PublicSubscriptionSummary, 0, len(subs))
	for _, summary := range subs {
		publicSummary := PublicSubscriptionSummary{Plan: summary.Plan}
		publicSummary.Subscription = toPublicUserSubscription(summary.Subscription, summary.Plan, activeSubscriptionId, paidRemainderByTier, now)
		result = append(result, publicSummary)
	}
	return result
}

func toPublicUserSubscription(sub *UserSubscription, plan *SubscriptionPlan, activeSubscriptionId int, paidRemainderByTier map[string]int64, now int64) *PublicUserSubscription {
	if sub == nil {
		return nil
	}
	return &PublicUserSubscription{
		Id:                sub.Id,
		UserId:            sub.UserId,
		PlanId:            sub.PlanId,
		AmountTotal:       sub.AmountTotal,
		EntitlementType:   sub.EntitlementType,
		AmountUsed:        sub.AmountUsed,
		TokenLimit:        sub.TokenLimit,
		TokenUsed:         sub.TokenUsed,
		ConcurrencyLimit:  livePlanConcurrencyLimit(sub, plan),
		QueueCapacity:     livePlanQueueCapacity(plan),
		GrantReason:       sub.GrantReason,
		GrantSourceUserId: sub.GrantSourceUserId,
		StartTime:         sub.StartTime,
		EndTime:           sub.EndTime,
		EffectiveEndTime:  effectiveSubscriptionEndTime(sub, plan, paidRemainderByTier),
		Status:            sub.Status,
		Source:            sub.Source,
		LastResetTime:     sub.LastResetTime,
		NextResetTime:     sub.NextResetTime,
		UpgradeGroup:      "",
		PrevUserGroup:     "",
		CreatedAt:         sub.CreatedAt,
		UpdatedAt:         sub.UpdatedAt,
		IsActiveSelected:  activeSubscriptionId > 0 && sub.Id == activeSubscriptionId,
		CanResetQuota:     canResetSubscriptionQuota(sub, plan, paidRemainderByTier, now),
		SourceLabel:       subscriptionSourceLabel(sub, plan),
	}
}

func subscriptionSourceLabel(sub *UserSubscription, plan *SubscriptionPlan) string {
	source := normalizedSubscriptionGrantSource(sub)
	if source == "" {
		return ""
	}
	if isPaidEquivalentSubscription(sub, plan) {
		return "paid"
	}
	if isInvitationRewardSubscription(sub) {
		return "invitation_reward"
	}
	if source == "trial_code" || source == "invite_trial" || (source == "admin" && plan != nil && (plan.IsTrial || plan.InviteTrial)) {
		return "trial"
	}
	return source
}

// AdminInvalidateUserSubscription marks a user subscription as cancelled and ends it immediately.
func AdminInvalidateUserSubscription(userSubscriptionId int) (string, error) {
	if userSubscriptionId <= 0 {
		return "", errors.New("invalid userSubscriptionId")
	}
	now := common.GetTimestamp()

	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		if sub.EntitlementType == SubscriptionEntitlementCreditBalance {
			return errors.New("Credit 余额权益不能通过普通接口失效")
		}
		if sub.Status == SubscriptionStatusConverted {
			return errors.New("converted 权益不能通过普通接口失效")
		}

		if err := tx.Model(&sub).Updates(map[string]interface{}{
			"status":     "cancelled",
			"end_time":   now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return "", err
	}
	return "", nil
}

// AdminDeleteUserSubscription hard-deletes a user subscription.
func AdminDeleteUserSubscription(userSubscriptionId int) (string, error) {
	if userSubscriptionId <= 0 {
		return "", errors.New("invalid userSubscriptionId")
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		if sub.EntitlementType == SubscriptionEntitlementCreditBalance {
			return errors.New("Credit 余额权益不能通过普通接口删除")
		}
		if sub.Status == SubscriptionStatusConverted {
			return errors.New("converted 权益不能通过普通接口删除")
		}

		if err := tx.Where("id = ?", userSubscriptionId).Delete(&UserSubscription{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return "", nil
}

type SubscriptionPreConsumeResult struct {
	UserSubscriptionId         int
	PreConsumed                int64
	AmountTotal                int64
	AmountUsedBefore           int64
	AmountUsedAfter            int64
	TokenLimit                 int64
	TokenUsedBefore            int64
	TokenUsedAfter             int64
	TokenRemaining             int64
	DistributorTokenBilling    bool
	CreditValuationTracked     bool
	ConcurrencyLimit           int
	QueueCapacity              int
	PlanId                     int
	EntitlementType            string
	PlanIsTrial                bool
	PlanTitle                  string
	PlanPriceAmount            float64
	PlanInviteTrial            bool
	SubscriptionSource         string
	SubscriptionGrantReason    string
	SubscriptionStatus         string
	SubscriptionEndTime        int64
	SubscriptionTokenRemaining int64
}

// ExpireDueSubscriptions marks expired subscriptions and handles group downgrade.
func ExpireDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("status = ? AND (entitlement_type IS NULL OR entitlement_type <> ?) AND end_time > 0 AND end_time <= ?", "active", SubscriptionEntitlementCreditBalance, now).
		Order("end_time asc, id asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	expiredCount := 0
	userIds := make(map[int]struct{}, len(subs))
	for _, sub := range subs {
		if sub.UserId > 0 {
			userIds[sub.UserId] = struct{}{}
		}
	}
	for userId := range userIds {

		err := DB.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&UserSubscription{}).
				Where("user_id = ? AND status = ? AND (entitlement_type IS NULL OR entitlement_type <> ?) AND end_time > 0 AND end_time <= ?", userId, "active", SubscriptionEntitlementCreditBalance, now).
				Updates(map[string]interface{}{
					"status":     "expired",
					"updated_at": common.GetTimestamp(),
				})
			if res.Error != nil {
				return res.Error
			}
			expiredCount += int(res.RowsAffected)

			return nil
		})
		if err != nil {
			return expiredCount, err
		}
	}
	return expiredCount, nil
}

// SubscriptionPreConsumeRecord stores idempotent pre-consume operations per request.
type SubscriptionPreConsumeRecord struct {
	Id                                 int    `json:"id"`
	RequestId                          string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId                             int    `json:"user_id" gorm:"index"`
	UserSubscriptionId                 int    `json:"user_subscription_id" gorm:"index"`
	PreConsumed                        int64  `json:"pre_consumed" gorm:"type:bigint;not null;default:0"`
	AppliedCredit                      int64  `json:"applied_credit" gorm:"type:bigint;not null;default:0"`
	DeductedAvailableCredit            int64  `json:"deducted_available_credit" gorm:"type:bigint;not null;default:0"`
	DebtFormedCredit                   int64  `json:"debt_formed_credit" gorm:"type:bigint;not null;default:0"`
	ValuationSubscriptionId            int    `json:"valuation_subscription_id" gorm:"not null;default:0;index"`
	DeductedExactCostMicros            int64  `json:"deducted_exact_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	DeductedEstimatedCostMicros        int64  `json:"deducted_estimated_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	DeductedUnknownCredit              int64  `json:"deducted_unknown_credit" gorm:"type:bigint;not null;default:0"`
	AbsorbedRestoreUnknownCredit       int64  `json:"absorbed_restore_unknown_credit" gorm:"type:bigint;not null;default:0"`
	AbsorbedRestoreExactCostMicros     int64  `json:"absorbed_restore_exact_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	AbsorbedRestoreEstimatedCostMicros int64  `json:"absorbed_restore_estimated_cost_micros,string" gorm:"type:bigint;not null;default:0"`
	RestoredUnknownCredit              int64  `json:"restored_unknown_credit" gorm:"type:bigint;not null;default:0"`
	ValuationRuleVersion               int    `json:"valuation_rule_version" gorm:"not null;default:0"`
	SettlementVersion                  int64  `json:"settlement_version" gorm:"type:bigint;not null;default:0"`
	FinalizedAt                        int64  `json:"finalized_at" gorm:"type:bigint;not null;default:0"`
	Status                             string `json:"status" gorm:"type:varchar(32);index"`
	CreatedAt                          int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt                          int64  `json:"updated_at" gorm:"bigint;index"`
}

func (r *SubscriptionPreConsumeRecord) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	r.CreatedAt = now
	r.UpdatedAt = now
	return nil
}

func (r *SubscriptionPreConsumeRecord) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	return nil
}

func isDistributorSubscription(sub *UserSubscription, plan *SubscriptionPlan) bool {
	if sub == nil {
		return false
	}
	if sub.TokenLimit > 0 || sub.ConcurrencyLimit > 0 || (plan != nil && plan.ConcurrencyLimit > 0) {
		return true
	}
	if plan != nil && plan.BusinessCode != nil && strings.TrimSpace(*plan.BusinessCode) != "" {
		return true
	}
	return false
}

func isUnlimitedTrialSubscription(sub *UserSubscription, plan *SubscriptionPlan) bool {
	if sub == nil || sub.TokenLimit != 0 {
		return false
	}
	reason := strings.TrimSpace(sub.GrantReason)
	if reason == "trial_code" || reason == "invite_trial" {
		return true
	}
	return reason == "admin" && plan != nil && plan.IsTrial
}

// IsTimedSubscriptionConversionIdentityEligible enforces identity-level conversion bans.
// Product switches and time-based eligibility are evaluated separately by conversion flows.
func IsTimedSubscriptionConversionIdentityEligible(sub *UserSubscription, plan *SubscriptionPlan) bool {
	if sub == nil || plan == nil {
		return false
	}
	if sub.EntitlementType == SubscriptionEntitlementCreditBalance || plan.EntitlementType == SubscriptionEntitlementCreditBalance {
		return false
	}
	if plan.IsTrial || plan.InviteTrial {
		return false
	}
	switch normalizedSubscriptionGrantSource(sub) {
	case "trial_code", "invite_trial", SubscriptionGrantMonthlyInviteEntitlement:
		return false
	default:
		return true
	}
}

func normalizedSubscriptionGrantSource(sub *UserSubscription) string {
	if sub == nil {
		return ""
	}
	if reason := strings.TrimSpace(sub.GrantReason); reason != "" {
		return reason
	}
	return strings.TrimSpace(sub.Source)
}

func isPaidEquivalentSubscription(sub *UserSubscription, plan *SubscriptionPlan) bool {
	switch normalizedSubscriptionGrantSource(sub) {
	case SubscriptionGrantOrder, "redemption":
		return true
	case "admin":
		return plan != nil && plan.PriceAmount > 0 && !plan.IsTrial && !plan.InviteTrial
	default:
		return false
	}
}

func isPaidSubscription(sub *UserSubscription) bool {
	switch normalizedSubscriptionGrantSource(sub) {
	case SubscriptionGrantOrder, "redemption":
		return true
	default:
		return false
	}
}

func isActiveResettableSubscription(sub *UserSubscription, now int64) bool {
	return sub != nil && sub.Status == "active" && sub.EndTime > now
}

func isInvitationRewardSubscription(sub *UserSubscription) bool {
	return normalizedSubscriptionGrantSource(sub) == SubscriptionGrantMonthlyInviteEntitlement
}

func subscriptionTierKey(plan *SubscriptionPlan) string {
	if plan == nil {
		return ""
	}
	if plan.BusinessCode != nil {
		return strings.TrimSpace(*plan.BusinessCode)
	}
	return ""
}

func isInvitationRewardSource(source string) bool {
	return strings.TrimSpace(source) == SubscriptionGrantMonthlyInviteEntitlement
}

func effectiveSubscriptionEndTime(sub *UserSubscription, plan *SubscriptionPlan, paidRemainderByTier map[string]int64) int64 {
	if sub == nil {
		return 0
	}
	endTime := sub.EndTime
	if !isInvitationRewardSubscription(sub) || plan == nil || paidRemainderByTier == nil {
		return endTime
	}
	if remainder := paidRemainderByTier[subscriptionTierKey(plan)]; remainder > 0 {
		endTime += remainder
	}
	return endTime
}

func canResetSubscriptionQuota(sub *UserSubscription, plan *SubscriptionPlan, paidRemainderByTier map[string]int64, now int64) bool {
	if !isActiveResettableSubscription(sub, now) {
		return false
	}
	if isPaidEquivalentSubscription(sub, plan) {
		return true
	}
	if isInvitationRewardSubscription(sub) && plan != nil && paidRemainderByTier != nil {
		return paidRemainderByTier[subscriptionTierKey(plan)] >= oneMonthSecondsFrom(now)
	}
	return false
}

func oneMonthSecondsFrom(now int64) int64 {
	return 30 * 86400
}

func isBillableSubscriptionCandidate(sub *UserSubscription, plan *SubscriptionPlan, requiredTokens int64) (bool, bool) {
	if sub == nil {
		return false, false
	}
	if sub.TokenLimit > 0 {
		return sub.TokenLimit-sub.TokenUsed >= requiredTokens, false
	}
	if isUnlimitedTrialSubscription(sub, plan) {
		return true, true
	}
	return false, false
}

func sortAutomaticBillingCandidates(candidates []billableSubscriptionCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i].sub
		right := candidates[j].sub
		leftCredit := left.EntitlementType == SubscriptionEntitlementCreditBalance
		rightCredit := right.EntitlementType == SubscriptionEntitlementCreditBalance
		if leftCredit != rightCredit {
			return !leftCredit
		}
		if left.EndTime != right.EndTime {
			return left.EndTime < right.EndTime
		}
		return left.Id < right.Id
	})
}

func loadBillableSubscriptionCandidatesTx(tx *gorm.DB, userId int, now int64, forUpdate bool, resetDue bool) ([]billableSubscriptionCandidate, error) {
	if resetDue && !forUpdate {
		return nil, errors.New("subscription quota reset requires locked candidates")
	}
	query := tx
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var subscriptions []UserSubscription
	if err := query.Where("user_id = ? AND status = ? AND start_time <= ? AND (entitlement_type = ? OR end_time > ?)", userId, "active", now, SubscriptionEntitlementCreditBalance, now).
		Find(&subscriptions).Error; err != nil {
		return nil, err
	}
	candidates := make([]billableSubscriptionCandidate, 0, len(subscriptions))
	for i := range subscriptions {
		subscription := subscriptions[i]
		plan, err := getSubscriptionPlanByIdTx(tx, subscription.PlanId)
		if err != nil {
			return nil, err
		}
		if resetDue {
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &subscription, plan, now); err != nil {
				return nil, err
			}
		}
		if !isDistributorSubscription(&subscription, plan) {
			continue
		}
		candidates = append(candidates, billableSubscriptionCandidate{
			sub:         subscription,
			plan:        plan,
			distributor: true,
			index:       i,
		})
	}
	return candidates, nil
}

func automaticBillingCandidateOrder(candidates []billableSubscriptionCandidate) []billableSubscriptionCandidate {
	ordered := append([]billableSubscriptionCandidate(nil), candidates...)
	sortAutomaticBillingCandidates(ordered)
	return ordered
}

func findBillableSubscriptionCandidate(candidates []billableSubscriptionCandidate, subscriptionId int) (billableSubscriptionCandidate, bool) {
	if subscriptionId <= 0 {
		return billableSubscriptionCandidate{}, false
	}
	for _, candidate := range candidates {
		if candidate.sub.Id == subscriptionId {
			return candidate, true
		}
	}
	return billableSubscriptionCandidate{}, false
}

func orderBillingStrategyCandidates(strategy string, active billableSubscriptionCandidate, activeFound bool, automatic []billableSubscriptionCandidate) []billableSubscriptionCandidate {
	switch strategy {
	case SubscriptionBillingStrategyTimedFirst:
		return automatic
	case SubscriptionBillingStrategyActiveFallback:
		if !activeFound {
			return automatic
		}
		ordered := make([]billableSubscriptionCandidate, 0, len(automatic))
		ordered = append(ordered, active)
		for _, candidate := range automatic {
			if candidate.sub.Id != active.sub.Id {
				ordered = append(ordered, candidate)
			}
		}
		return ordered
	default:
		if activeFound {
			return []billableSubscriptionCandidate{active}
		}
		return nil
	}
}

func saveSubscriptionSelectionSettingTx(tx *gorm.DB, userId int, oldSettingJSON string, repairedActiveId int) (dto.UserSetting, string, error) {
	oldSetting, err := ParseUserSettingString(oldSettingJSON)
	if err != nil {
		return dto.UserSetting{}, "", err
	}
	setting, settingJSON, err := mutateUserSettingCASAttempt(tx, userId, func(current *dto.UserSetting) error {
		if current.ActiveSubscriptionId != oldSetting.ActiveSubscriptionId {
			return ErrUserSettingCASConflict
		}
		current.ActiveSubscriptionId = repairedActiveId
		return nil
	})
	return setting, settingJSON, err
}

func syncSubscriptionSelectionSettingCacheAfterCommit(userId int, settingJSON string) {
	if userId <= 0 || settingJSON == "" {
		return
	}
	primaryBillableSubscriptionCache.Delete(primaryBillableSubscriptionCacheKey(userId))
	if err := invalidateUserCache(userId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate repaired subscription selection setting cache for user %d: %s", userId, err.Error()))
	}
}

type resolvedSubscriptionBillingStrategyState struct {
	Strategy             string
	ActiveSubscriptionId int
	SettingJSON          string
	RepairedSettingJSON  string
	OrderedCandidates    []billableSubscriptionCandidate
}

func resolveSubscriptionBillingStrategyStateTx(tx *gorm.DB, userId int, userSetting dto.UserSetting, settingJSON string, now int64, forUpdate bool, resetDue bool) (resolvedSubscriptionBillingStrategyState, error) {
	state := resolvedSubscriptionBillingStrategyState{
		Strategy:             NormalizeSubscriptionBillingStrategy(userSetting.SubscriptionBillingStrategy),
		ActiveSubscriptionId: userSetting.ActiveSubscriptionId,
		SettingJSON:          settingJSON,
	}
	candidates, err := loadBillableSubscriptionCandidatesTx(tx, userId, now, forUpdate, resetDue)
	if err != nil {
		return state, err
	}
	automaticOrder := automaticBillingCandidateOrder(candidates)
	activeCandidate, activeFound := findBillableSubscriptionCandidate(candidates, state.ActiveSubscriptionId)
	if !activeFound {
		repairedActiveId := 0
		if len(automaticOrder) > 0 {
			repairedActiveId = automaticOrder[0].sub.Id
			activeCandidate = automaticOrder[0]
			activeFound = true
		}
		if state.ActiveSubscriptionId != repairedActiveId {
			updatedSetting, updatedSettingJSON, saveErr := saveSubscriptionSelectionSettingTx(tx, userId, settingJSON, repairedActiveId)
			if saveErr != nil {
				return state, saveErr
			}
			state.Strategy = NormalizeSubscriptionBillingStrategy(updatedSetting.SubscriptionBillingStrategy)
			state.SettingJSON = updatedSettingJSON
			state.RepairedSettingJSON = updatedSettingJSON
			state.ActiveSubscriptionId = repairedActiveId
		}
	}
	state.OrderedCandidates = orderBillingStrategyCandidates(state.Strategy, activeCandidate, activeFound, automaticOrder)
	return state, nil
}

type primaryBillableSelectionOutcome struct {
	Selection                       *primaryBillableSubscription
	SawDistributorSubscription      bool
	ActiveSubscriptionId            int
	RepairedSettingJSON             string
	BillingStrategy                 string
	BillingCandidateSubscriptionIds []int
}

type billableSubscriptionCandidate struct {
	sub         UserSubscription
	plan        *SubscriptionPlan
	distributor bool
	unlimited   bool
	index       int
}

func buildPrimaryBillableSubscription(candidate billableSubscriptionCandidate) *primaryBillableSubscription {
	return &primaryBillableSubscription{
		Subscription:     candidate.sub,
		Plan:             candidate.plan,
		Distributor:      candidate.distributor,
		TokenUnlimited:   candidate.unlimited,
		AmountUsedBefore: candidate.sub.AmountUsed,
		TokenUsedBefore:  candidate.sub.TokenUsed,
	}
}

type primaryBillableSubscription struct {
	Subscription     UserSubscription
	Plan             *SubscriptionPlan
	Distributor      bool
	TokenUnlimited   bool
	AmountUsedBefore int64
	TokenUsedBefore  int64
}

type primaryBillableSubscriptionCacheEntry struct {
	setting string
	loaded  primaryBillableSubscription
}

func primaryBillableSubscriptionCacheKey(userId int) string {
	return strconv.Itoa(userId)
}

func getCachedPrimaryBillableSubscription(tx *gorm.DB, userId int, setting string, requiredTokens int64, now int64) (*primaryBillableSubscription, bool) {
	if tx == nil {
		return nil, false
	}
	value, ok := primaryBillableSubscriptionCache.Load(primaryBillableSubscriptionCacheKey(userId))
	if !ok {
		return nil, false
	}
	entry, ok := value.(primaryBillableSubscriptionCacheEntry)
	if !ok || entry.setting != setting {
		return nil, false
	}
	if entry.loaded.Subscription.NextResetTime > 0 && entry.loaded.Subscription.NextResetTime <= now {
		return nil, false
	}
	selection := entry.loaded
	var sub UserSubscription
	if err := tx.Select("id", "user_id", "plan_id", "entitlement_type", "amount_total", "amount_used", "token_limit", "token_used", "concurrency_limit", "grant_reason", "grant_source_user_id", "start_time", "end_time", "status", "source", "last_reset_time", "next_reset_time", "created_at", "updated_at").Where("id = ? AND user_id = ?", selection.Subscription.Id, userId).Take(&sub).Error; err != nil {
		return nil, false
	}
	if sub.Status != "active" || sub.StartTime > now || (sub.EntitlementType != SubscriptionEntitlementCreditBalance && sub.EndTime <= now) {
		return nil, false
	}
	if sub.NextResetTime > 0 && sub.NextResetTime <= now {
		return nil, false
	}
	plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
	if err != nil {
		return nil, false
	}
	selection.Plan = plan
	if ok, unlimited := isBillableSubscriptionCandidate(&sub, plan, requiredTokens); !ok {
		return nil, false
	} else {
		selection.TokenUnlimited = unlimited
	}
	selection.Subscription = sub
	selection.AmountUsedBefore = sub.AmountUsed
	selection.TokenUsedBefore = sub.TokenUsed
	return &selection, true
}

func setCachedPrimaryBillableSubscription(userId int, setting string, selection *primaryBillableSubscription) {
	if selection == nil || selection.Subscription.Status != SubscriptionStatusActive {
		return
	}
	loaded := *selection
	primaryBillableSubscriptionCache.Store(primaryBillableSubscriptionCacheKey(userId), primaryBillableSubscriptionCacheEntry{setting: setting, loaded: loaded})
}

func ClearPrimaryBillableSubscriptionCacheForTest() {
	primaryBillableSubscriptionCache = sync.Map{}
}

func cachePrimaryBillableSelectionTx(tx *gorm.DB, userId int, sub *UserSubscription, plan *SubscriptionPlan, distributor bool) {
	if tx == nil || sub == nil || userId <= 0 || sub.Status != SubscriptionStatusActive {
		return
	}
	var user User
	if err := tx.Select("setting").Where("id = ?", userId).First(&user).Error; err != nil {
		return
	}
	setCachedPrimaryBillableSubscription(userId, user.Setting, &primaryBillableSubscription{
		Subscription:     *sub,
		Plan:             plan,
		Distributor:      distributor,
		TokenUnlimited:   isUnlimitedTrialSubscription(sub, plan),
		AmountUsedBefore: sub.AmountUsed,
		TokenUsedBefore:  sub.TokenUsed,
	})
}

func selectPrimaryBillableSubscriptionTx(tx *gorm.DB, userId int, now int64, requiredTokens int64, forUpdate bool, resetDue bool) (primaryBillableSelectionOutcome, error) {
	if tx == nil {
		tx = DB
	}
	var user User
	userQuery := tx.Select("setting").Where("id = ?", userId)
	if forUpdate {
		userQuery = userQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := userQuery.First(&user).Error; err != nil {
		return primaryBillableSelectionOutcome{}, err
	}
	userSetting := user.GetSetting()
	billingStrategy := NormalizeSubscriptionBillingStrategy(userSetting.SubscriptionBillingStrategy)
	if forUpdate && billingStrategy == SubscriptionBillingStrategySingleActive && userSetting.ActiveSubscriptionId > 0 {
		if cached, ok := getCachedPrimaryBillableSubscription(tx, userId, user.Setting, requiredTokens, now); ok && cached.Subscription.Id == userSetting.ActiveSubscriptionId {
			return primaryBillableSelectionOutcome{
				Selection:                       cached,
				SawDistributorSubscription:      true,
				ActiveSubscriptionId:            userSetting.ActiveSubscriptionId,
				BillingStrategy:                 billingStrategy,
				BillingCandidateSubscriptionIds: []int{userSetting.ActiveSubscriptionId},
			}, nil
		}
	}

	state, err := resolveSubscriptionBillingStrategyStateTx(tx, userId, userSetting, user.Setting, now, forUpdate, resetDue)
	if err != nil {
		return primaryBillableSelectionOutcome{}, err
	}
	outcome := primaryBillableSelectionOutcome{
		SawDistributorSubscription:      len(state.OrderedCandidates) > 0,
		ActiveSubscriptionId:            state.ActiveSubscriptionId,
		RepairedSettingJSON:             state.RepairedSettingJSON,
		BillingStrategy:                 state.Strategy,
		BillingCandidateSubscriptionIds: make([]int, 0, len(state.OrderedCandidates)),
	}
	for _, candidate := range state.OrderedCandidates {
		outcome.BillingCandidateSubscriptionIds = append(outcome.BillingCandidateSubscriptionIds, candidate.sub.Id)
	}
	for _, candidate := range state.OrderedCandidates {
		ok, unlimited := isBillableSubscriptionCandidate(&candidate.sub, candidate.plan, requiredTokens)
		if !ok {
			if state.Strategy == SubscriptionBillingStrategySingleActive {
				return outcome, nil
			}
			continue
		}
		candidate.unlimited = unlimited
		outcome.Selection = buildPrimaryBillableSubscription(candidate)
		if forUpdate {
			setCachedPrimaryBillableSubscription(userId, state.SettingJSON, outcome.Selection)
		}
		return outcome, nil
	}
	return outcome, nil
}

func ResolveGPTAbuseWarningLimit(plan *SubscriptionPlan) int {
	minimum := common.GPTAbuseDefaultWarningLimit
	if minimum < 1 {
		minimum = 1
	}
	if plan == nil {
		return minimum
	}
	if plan.GPTAbuseWarningLimit > 0 {
		return maxInt(plan.GPTAbuseWarningLimit, minimum)
	}
	return maxInt(plan.ConcurrencyLimit, minimum)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func GetSubscriptionSelfSummary(userId int) (SelfSubscriptionSummary, error) {
	if userId <= 0 {
		return SelfSubscriptionSummary{}, errors.New("invalid userId")
	}
	now := GetDBTimestamp()
	setting, err := GetUserSetting(userId, true)
	if err != nil {
		return SelfSubscriptionSummary{}, err
	}
	summary := SelfSubscriptionSummary{ActiveSubscriptionId: setting.ActiveSubscriptionId, GPTAbuseLimitEnabled: common.GPTAbuseLimitEnabled}
	var repairedSettingJSON string
	err = transactionWithUserSettingCASRetry(func(tx *gorm.DB) error {
		summary = SelfSubscriptionSummary{GPTAbuseLimitEnabled: common.GPTAbuseLimitEnabled}
		repairedSettingJSON = ""
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("setting").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		state, err := resolveSubscriptionBillingStrategyStateTx(tx, userId, user.GetSetting(), user.Setting, now, true, true)
		if err != nil {
			return err
		}
		repairedSettingJSON = state.RepairedSettingJSON
		summary.ActiveSubscriptionId = state.ActiveSubscriptionId
		summary.BillingStrategy = state.Strategy
		summary.BillingCandidateIds = make([]int, 0, len(state.OrderedCandidates))
		for _, candidate := range state.OrderedCandidates {
			summary.BillingCandidateIds = append(summary.BillingCandidateIds, candidate.sub.Id)
		}
		if len(state.OrderedCandidates) == 0 {
			return nil
		}
		candidate := state.OrderedCandidates[0]
		sub := candidate.sub
		summary.ActiveCount = 1
		summary.SubscriptionId = sub.Id
		summary.PlanId = sub.PlanId
		summary.PrimaryPlanTitle = candidate.plan.Title
		summary.TokenLimit = sub.TokenLimit
		summary.TokenUsed = sub.TokenUsed
		if isUnlimitedTrialSubscription(&sub, candidate.plan) {
			summary.TokenUnlimited = true
		} else if sub.TokenLimit > sub.TokenUsed {
			summary.TokenRemaining = sub.TokenLimit - sub.TokenUsed
		}
		summary.ConcurrencyLimit = livePlanConcurrencyLimit(&sub, candidate.plan)
		summary.QueueCapacity = livePlanQueueCapacity(candidate.plan)
		summary.GPTAbuseWarningLimit = ResolveGPTAbuseWarningLimit(candidate.plan)
		summary.NextResetTime = sub.NextResetTime
		summary.EndTime = sub.EndTime
		return nil
	})
	if err != nil {
		return summary, err
	}
	if repairedSettingJSON != "" {
		syncSubscriptionSelectionSettingCacheAfterCommit(userId, repairedSettingJSON)
	}
	if summary.GPTAbuseWarningLimit <= 0 {
		summary.GPTAbuseWarningLimit = ResolveGPTAbuseWarningLimit(nil)
	}
	dayStart, dayEnd := GPTAbuseDayWindow(now)
	count, countErr := CountGPTAbuseSignalsForUser(userId, dayStart, dayEnd)
	if countErr != nil {
		return summary, countErr
	}
	summary.GPTAbuseWarningCount = count
	if summary.GPTAbuseWarningLimit > count {
		summary.GPTAbuseWarningRemaining = summary.GPTAbuseWarningLimit - count
	}
	if susp, suspErr := GetActiveGPTAbuseSuspension(userId, now); suspErr != nil {
		return summary, suspErr
	} else if susp != nil {
		summary.GPTAbuseSuspendedUntil = susp.SuspendedUntil
	}
	return summary, nil
}

func GetCodexProEligibility(userId int, _ dto.UserSetting) (bool, string, error) {
	if userId <= 0 {
		return false, "", errors.New("invalid userId")
	}
	now := GetDBTimestamp()
	var selection *primaryBillableSubscription
	var repairedSettingJSON string
	err := transactionWithUserSettingCASRetry(func(tx *gorm.DB) error {
		outcome, err := selectPrimaryBillableSubscriptionTx(tx, userId, now, 1, true, true)
		if err != nil {
			return err
		}
		repairedSettingJSON = outcome.RepairedSettingJSON
		selection = outcome.Selection
		return nil
	})
	if err != nil {
		return false, "", err
	}
	if repairedSettingJSON != "" {
		syncSubscriptionSelectionSettingCacheAfterCommit(userId, repairedSettingJSON)
	}
	if selection == nil {
		return false, "no_paid_subscription", nil
	}
	sub := &selection.Subscription
	plan := selection.Plan
	if codexProTrialSubscriptionSource(sub.GrantReason) || codexProTrialSubscriptionSource(sub.Source) || (plan != nil && (plan.IsTrial || plan.InviteTrial)) {
		return false, "trial_subscription", nil
	}
	if codexProRewardSubscriptionSource(sub.GrantReason) || codexProRewardSubscriptionSource(sub.Source) {
		return false, "reward_subscription", nil
	}
	if codexProPaidEquivalentSubscription(sub, plan) {
		return true, "", nil
	}
	return false, "no_paid_subscription", nil
}

func codexProPaidEquivalentSubscription(sub *UserSubscription, plan *SubscriptionPlan) bool {
	if sub == nil || plan == nil || plan.PriceAmount <= 0 || plan.IsTrial || plan.InviteTrial {
		return false
	}
	if codexProTrialSubscriptionSource(sub.GrantReason) || codexProTrialSubscriptionSource(sub.Source) || codexProRewardSubscriptionSource(sub.GrantReason) || codexProRewardSubscriptionSource(sub.Source) {
		return false
	}
	switch normalizedSubscriptionGrantSource(sub) {
	case SubscriptionGrantOrder, "redemption", "admin":
		return true
	default:
		return false
	}
}

func codexProTrialSubscriptionSource(value string) bool {
	switch strings.TrimSpace(value) {
	case "trial_code", "invite_trial":
		return true
	default:
		return false
	}
}

func codexProRewardSubscriptionSource(value string) bool {
	return strings.TrimSpace(value) == SubscriptionGrantMonthlyInviteEntitlement
}

func HasActiveDistributorSubscription(userId int) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid userId")
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("user_id = ? AND status = ? AND (entitlement_type = ? OR end_time > ?)", userId, "active", SubscriptionEntitlementCreditBalance, now).
		Order("end_time asc, id asc").
		Find(&subs).Error; err != nil {
		return false, err
	}
	for _, sub := range subs {
		plan, err := getSubscriptionPlanByIdTx(DB, sub.PlanId)
		if err != nil {
			continue
		}
		if isDistributorSubscription(&sub, plan) {
			return true, nil
		}
	}
	return false, nil
}

func fillSubscriptionPreConsumeResult(result *SubscriptionPreConsumeResult, sub *UserSubscription, plan *SubscriptionPlan, preConsumed int64, amountBefore int64, tokenBefore int64, distributor bool) {
	if result == nil || sub == nil {
		return
	}
	result.UserSubscriptionId = sub.Id
	result.PreConsumed = preConsumed
	result.AmountTotal = sub.AmountTotal
	result.AmountUsedBefore = amountBefore
	result.AmountUsedAfter = sub.AmountUsed
	result.TokenLimit = sub.TokenLimit
	result.ConcurrencyLimit = livePlanConcurrencyLimit(sub, plan)
	result.QueueCapacity = livePlanQueueCapacity(plan)
	result.TokenUsedBefore = tokenBefore
	result.TokenUsedAfter = sub.TokenUsed
	result.DistributorTokenBilling = distributor
	result.PlanId = sub.PlanId
	result.EntitlementType = sub.EntitlementType
	result.PlanIsTrial = false
	result.PlanInviteTrial = false
	result.PlanPriceAmount = 0
	if plan != nil {
		result.PlanIsTrial = plan.IsTrial
		result.PlanInviteTrial = plan.InviteTrial
		result.PlanPriceAmount = plan.PriceAmount
		result.PlanTitle = plan.Title
	}
	result.SubscriptionSource = sub.Source
	result.SubscriptionGrantReason = sub.GrantReason
	result.SubscriptionStatus = sub.Status
	result.SubscriptionEndTime = sub.EndTime
	if sub.TokenLimit > 0 {
		remaining := sub.TokenLimit - sub.TokenUsed
		if remaining < 0 {
			remaining = 0
		}
		result.TokenRemaining = remaining
		result.SubscriptionTokenRemaining = remaining
	} else {
		result.TokenRemaining = 0
		result.SubscriptionTokenRemaining = 0
	}
}

func livePlanConcurrencyLimit(sub *UserSubscription, plan *SubscriptionPlan) int {
	if plan != nil {
		return plan.ConcurrencyLimit
	}
	if sub != nil {
		return sub.ConcurrencyLimit
	}
	return 0
}

func livePlanQueueCapacity(plan *SubscriptionPlan) int {
	if plan != nil {
		return plan.QueueCapacity
	}
	return 0
}
func maybeResetUserSubscriptionWithPlanTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan, now int64) error {
	if tx == nil || sub == nil || plan == nil {
		return errors.New("invalid reset args")
	}
	if sub.EntitlementType == SubscriptionEntitlementCreditBalance || plan.EntitlementType == SubscriptionEntitlementCreditBalance {
		return nil
	}
	if sub.NextResetTime > 0 && sub.NextResetTime > now {
		return nil
	}
	if NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
		return nil
	}
	baseUnix := sub.LastResetTime
	if baseUnix <= 0 {
		baseUnix = sub.StartTime
	}
	base := time.Unix(baseUnix, 0)
	next := calcNextResetTime(base, plan, sub.EndTime)
	advanced := false
	for next > 0 && next <= now {
		advanced = true
		base = time.Unix(next, 0)
		next = calcNextResetTime(base, plan, sub.EndTime)
	}
	if !advanced {
		if sub.NextResetTime == 0 && next > 0 {
			sub.NextResetTime = next
			sub.LastResetTime = base.Unix()
			return tx.Save(sub).Error
		}
		return nil
	}
	sub.AmountUsed = 0
	sub.TokenUsed = 0
	sub.LastResetTime = base.Unix()
	sub.NextResetTime = next
	return tx.Save(sub).Error
}

type SubscriptionResetResult struct {
	SubscriptionId int   `json:"subscription_id"`
	EndTime        int64 `json:"end_time"`
	NextResetTime  int64 `json:"next_reset_time,omitempty"`
}

func SetUserActiveSubscription(userId int, subscriptionId int) error {
	if userId <= 0 || subscriptionId <= 0 {
		return errors.New("invalid userId or subscriptionId")
	}
	now := GetDBTimestamp()
	err := transactionWithUserSettingCASRetry(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := tx.Where("id = ? AND user_id = ? AND status = ? AND start_time <= ? AND (entitlement_type = ? OR end_time > ?)", subscriptionId, userId, "active", now, SubscriptionEntitlementCreditBalance, now).First(&sub).Error; err != nil {
			return err
		}
		_, _, err := mutateUserSettingCASAttempt(tx, userId, func(setting *dto.UserSetting) error {
			setting.ActiveSubscriptionId = subscriptionId
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	primaryBillableSubscriptionCache.Delete(primaryBillableSubscriptionCacheKey(userId))
	return invalidateUserCache(userId)
}

func ResetUserSubscriptionQuota(userId int, subscriptionId int) (*SubscriptionResetResult, error) {
	if userId <= 0 || subscriptionId <= 0 {
		return nil, errors.New("invalid userId or subscriptionId")
	}
	now := GetDBTimestamp()
	var result *SubscriptionResetResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		target, targetPlan, err := lockActiveUserSubscriptionWithPlanTx(tx, userId, subscriptionId, now)
		if err != nil {
			return err
		}
		if target.EntitlementType == SubscriptionEntitlementCreditBalance {
			return errors.New("Credit balance quota cannot be reset")
		}
		payer := target
		if !isPaidEquivalentSubscription(target, targetPlan) {
			if !isInvitationRewardSubscription(target) {
				return errors.New("subscription quota reset requires paid subscription")
			}
			payer, err = findResetQuotaPaidSubscriptionTx(tx, userId, target.Id, subscriptionTierKey(targetPlan), now)
			if err != nil {
				return err
			}
		}
		monthSeconds := oneMonthSecondsFrom(now)
		if payer.EndTime-now < monthSeconds {
			return errors.New("paid subscription remaining time is less than one month")
		}
		target.TokenUsed = 0
		target.AmountUsed = 0
		target.LastResetTime = now
		target.NextResetTime = calcNextResetTime(time.Unix(now, 0), targetPlan, target.EndTime)
		if target.Id == payer.Id {
			target.EndTime -= monthSeconds
			payer = target
			if err := tx.Save(target).Error; err != nil {
				return err
			}
		} else {
			payer.EndTime -= monthSeconds
			if err := tx.Save(payer).Error; err != nil {
				return err
			}
			if err := tx.Save(target).Error; err != nil {
				return err
			}
		}
		result = &SubscriptionResetResult{SubscriptionId: target.Id, EndTime: target.EndTime, NextResetTime: target.NextResetTime}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func lockActiveUserSubscriptionWithPlanTx(tx *gorm.DB, userId int, subscriptionId int, now int64) (*UserSubscription, *SubscriptionPlan, error) {
	if tx == nil {
		return nil, nil, errors.New("tx is nil")
	}
	var sub UserSubscription
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ? AND user_id = ? AND status = ? AND (entitlement_type = ? OR end_time > ?)", subscriptionId, userId, "active", SubscriptionEntitlementCreditBalance, now).First(&sub).Error; err != nil {
		return nil, nil, err
	}
	plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
	if err != nil {
		return nil, nil, err
	}
	return &sub, plan, nil
}

func findResetQuotaPaidSubscriptionTx(tx *gorm.DB, userId int, excludeSubscriptionId int, tier string, now int64) (*UserSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if tier == "" {
		return nil, errors.New("subscription tier is empty")
	}
	var subs []UserSubscription
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("user_id = ? AND status = ? AND end_time > ? AND id <> ?", userId, "active", now, excludeSubscriptionId).Order("end_time desc, id desc").Find(&subs).Error; err != nil {
		return nil, err
	}
	for i := range subs {
		sub := &subs[i]
		plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
		if err != nil {
			return nil, err
		}
		if !isPaidEquivalentSubscription(sub, plan) {
			continue
		}
		if subscriptionTierKey(plan) == tier {
			return sub, nil
		}
	}
	return nil, errors.New("same tier paid subscription not found")
}

// PreConsumeUserSubscription pre-consumes from active subscription token quota.
func PreConsumeUserSubscription(requestId string, userId int, modelName string, quotaType int, amount int64) (*SubscriptionPreConsumeResult, error) {
	return PreConsumeUserSubscriptionByUnits(requestId, userId, modelName, quotaType, 0, amount)
}

// PreConsumeUserSubscriptionByUnits pre-consumes from active subscription token quota.
// legacyAmount is kept for compatibility with older callers, but API request billing no longer falls back to amount_total.
func PreConsumeUserSubscriptionByUnits(requestId string, userId int, modelName string, quotaType int, legacyAmount int64, distributorAmount int64) (*SubscriptionPreConsumeResult, error) {
	return preConsumeUserSubscriptionByUnits(requestId, userId, modelName, quotaType, legacyAmount, distributorAmount, nil)
}

func preConsumeUserSubscriptionByUnits(requestId string, userId int, modelName string, quotaType int, legacyAmount int64, distributorAmount int64, hooks *subscriptionTransactionHooks) (*SubscriptionPreConsumeResult, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	_ = legacyAmount
	if strings.TrimSpace(requestId) == "" {
		return nil, errors.New("requestId is empty")
	}
	if distributorAmount <= 0 {
		return nil, errors.New("amount must be > 0")
	}
	now := GetDBTimestamp()
	returnValue := &SubscriptionPreConsumeResult{}
	var repairedSettingJSON string
	var selectionBusinessErr error

	runPreConsume := func(tx *gorm.DB) error {
		*returnValue = SubscriptionPreConsumeResult{}
		repairedSettingJSON = ""
		selectionBusinessErr = nil
		if hooks != nil && hooks.onPreConsumeAttemptStarted != nil {
			hooks.onPreConsumeAttemptStarted()
		}
		var existing SubscriptionPreConsumeRecord
		query := tx.Where("request_id = ?", requestId).Limit(1).Find(&existing)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			if existing.Status == "refunded" {
				return errors.New("subscription pre-consume already refunded")
			}
			var sub UserSubscription
			if err := tx.Where("id = ?", existing.UserSubscriptionId).First(&sub).Error; err != nil {
				return err
			}
			plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
			if err != nil {
				return err
			}
			fillSubscriptionPreConsumeResult(returnValue, &sub, plan, existing.PreConsumed, sub.AmountUsed, sub.TokenUsed, isDistributorSubscription(&sub, plan))
			returnValue.CreditValuationTracked = existing.ValuationSubscriptionId > 0
			cachePrimaryBillableSelectionTx(tx, userId, &sub, plan, returnValue.DistributorTokenBilling)
			return nil
		}

		outcome, selectionErr := selectPrimaryBillableSubscriptionTx(tx, userId, now, distributorAmount, true, true)
		if selectionErr != nil {
			return selectionErr
		}
		repairedSettingJSON = outcome.RepairedSettingJSON
		selection := outcome.Selection
		if selection == nil {
			switch {
			case !outcome.SawDistributorSubscription:
				selectionBusinessErr = ErrNoActiveSubscription
			default:
				selectionBusinessErr = fmt.Errorf("subscription token quota insufficient, need=%d", distributorAmount)
			}
			return nil
		}
		sub := selection.Subscription
		amountUsedBefore := selection.AmountUsedBefore
		tokenUsedBefore := selection.TokenUsedBefore
		distributor := selection.Distributor
		consumeAmount := distributorAmount
		record := &SubscriptionPreConsumeRecord{
			RequestId:          requestId,
			UserId:             userId,
			UserSubscriptionId: sub.Id,
			PreConsumed:        consumeAmount,
			Status:             "consumed",
		}
		valuationReady, err := CreditValuationRuntimeReadyTx(tx)
		if err != nil {
			return err
		}
		if valuationReady && sub.EntitlementType == SubscriptionEntitlementCreditBalance {
			if !distributor {
				return ErrCreditValuationStateMismatch
			}
			mutation, err := ApplyCreditValuationOutflowTx(tx, &sub, consumeAmount, CreditValuationMutationConsume)
			if err != nil {
				return err
			}
			record.AppliedCredit = consumeAmount
			record.DeductedAvailableCredit = consumeAmount
			record.ValuationSubscriptionId = sub.Id
			record.DeductedExactCostMicros = mutation.RemovedExactCostMicros
			record.DeductedEstimatedCostMicros = mutation.RemovedEstimatedCostMicros
			record.DeductedUnknownCredit = mutation.RemovedUnknownCredit
			record.ValuationRuleVersion = CreditValuationRuleVersion
			record.SettlementVersion = 1
		} else {
			rows, err := applySubscriptionPreConsumeUpdateTx(tx, sub.Id, distributor, consumeAmount)
			if err != nil {
				return err
			}
			if rows == 0 {
				if distributor {
					return fmt.Errorf("subscription token quota insufficient, need=%d", distributorAmount)
				}
				return fmt.Errorf("subscription quota insufficient, need=%d", consumeAmount)
			}
			if distributor {
				sub.TokenUsed += consumeAmount
			} else {
				sub.AmountUsed += consumeAmount
			}
		}
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		fillSubscriptionPreConsumeResult(returnValue, &sub, selection.Plan, consumeAmount, amountUsedBefore, tokenUsedBefore, distributor)
		returnValue.CreditValuationTracked = record.ValuationSubscriptionId > 0
		cachePrimaryBillableSelectionTx(tx, userId, &sub, selection.Plan, distributor)
		return nil
	}
	err := transactionWithUserSettingCASRetry(runPreConsume)
	if err != nil {
		return nil, err
	}
	if repairedSettingJSON != "" {
		syncSubscriptionSelectionSettingCacheAfterCommit(userId, repairedSettingJSON)
	}
	if selectionBusinessErr != nil {
		return nil, selectionBusinessErr
	}
	return returnValue, nil
}

// RefundSubscriptionPreConsume is idempotent and refunds pre-consumed subscription quota by requestId.
func RefundSubscriptionPreConsume(requestId string) error {
	if strings.TrimSpace(requestId) == "" {
		return errors.New("requestId is empty")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var record SubscriptionPreConsumeRecord
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("request_id = ?", requestId).First(&record).Error; err != nil {
			return err
		}
		if record.Status == "refunded" {
			return nil
		}
		if record.PreConsumed <= 0 {
			record.Status = "refunded"
			return tx.Save(&record).Error
		}
		if err := postConsumeUserSubscriptionDeltaTx(tx, record.UserSubscriptionId, -record.PreConsumed); err != nil {
			return err
		}
		record.Status = "refunded"
		return tx.Save(&record).Error
	})
}

// ResetDueSubscriptions resets subscriptions whose next_reset_time has passed.
func ResetDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("next_reset_time > 0 AND next_reset_time <= ? AND status = ? AND (entitlement_type IS NULL OR entitlement_type <> ?)", now, "active", SubscriptionEntitlementCreditBalance).
		Order("next_reset_time asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	resetCount := 0
	for _, sub := range subs {
		subCopy := sub
		plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
		if err != nil || plan == nil {
			continue
		}
		err = DB.Transaction(func(tx *gorm.DB) error {
			var locked UserSubscription
			if err := tx.Set("gorm:query_option", "FOR UPDATE").
				Where("id = ? AND next_reset_time > 0 AND next_reset_time <= ? AND status = ? AND (entitlement_type IS NULL OR entitlement_type <> ?)", subCopy.Id, now, "active", SubscriptionEntitlementCreditBalance).
				First(&locked).Error; err != nil {
				return nil
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &locked, plan, now); err != nil {
				return err
			}
			resetCount++
			return nil
		})
		if err != nil {
			return resetCount, err
		}
	}
	return resetCount, nil
}

// CleanupSubscriptionPreConsumeRecords removes old idempotency records to keep table small.
func CleanupSubscriptionPreConsumeRecords(olderThanSeconds int64) (int64, error) {
	if olderThanSeconds <= 0 {
		olderThanSeconds = 7 * 24 * 3600
	}
	cutoff := GetDBTimestamp() - olderThanSeconds
	res := DB.Where("updated_at < ?", cutoff).Delete(&SubscriptionPreConsumeRecord{})
	return res.RowsAffected, res.Error
}

type SubscriptionPlanInfo struct {
	PlanId    int
	PlanTitle string
}

func GetSubscriptionPlanInfoByUserSubscriptionId(userSubscriptionId int) (*SubscriptionPlanInfo, error) {
	if userSubscriptionId <= 0 {
		return nil, errors.New("invalid userSubscriptionId")
	}
	cacheKey := fmt.Sprintf("sub:%d", userSubscriptionId)
	if cached, found, err := getSubscriptionPlanInfoCache().Get(cacheKey); err == nil && found {
		return &cached, nil
	}
	var sub UserSubscription
	if err := DB.Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
		return nil, err
	}
	plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
	if err != nil {
		return nil, err
	}
	info := &SubscriptionPlanInfo{
		PlanId:    sub.PlanId,
		PlanTitle: plan.Title,
	}
	_ = getSubscriptionPlanInfoCache().SetWithTTL(cacheKey, *info, subscriptionPlanInfoCacheTTL())
	return info, nil
}
func tokenUsedDeltaExpr(delta int64) clause.Expr {
	if delta < 0 {
		return gorm.Expr("CASE WHEN ? + ? < 0 THEN 0 ELSE ? + ? END", clause.Column{Name: "token_used"}, delta, clause.Column{Name: "token_used"}, delta)
	}
	return gorm.Expr("? + ?", clause.Column{Name: "token_used"}, delta)
}

func amountUsedDeltaExpr(delta int64) clause.Expr {
	if delta < 0 {
		return gorm.Expr("CASE WHEN ? + ? < 0 THEN 0 ELSE ? + ? END", clause.Column{Name: "amount_used"}, delta, clause.Column{Name: "amount_used"}, delta)
	}
	return gorm.Expr("? + ?", clause.Column{Name: "amount_used"}, delta)
}

func applySubscriptionPreConsumeUpdateTx(tx *gorm.DB, userSubscriptionId int, distributor bool, consumeAmount int64) (int64, error) {
	if tx == nil {
		return 0, errors.New("tx is nil")
	}
	updates := map[string]interface{}{"updated_at": common.GetTimestamp()}
	query := tx.Model(&UserSubscription{}).Where("id = ?", userSubscriptionId)
	if distributor {
		updates["token_used"] = tokenUsedDeltaExpr(consumeAmount)
		query = query.Where("(entitlement_type <> ? AND token_limit <= 0) OR token_used + ? <= token_limit", SubscriptionEntitlementCreditBalance, consumeAmount)
	} else {
		updates["amount_used"] = amountUsedDeltaExpr(consumeAmount)
		query = query.Where("amount_total <= 0 OR amount_used + ? <= amount_total", consumeAmount)
	}
	res := query.Updates(updates)
	return res.RowsAffected, res.Error
}

const convertedSubscriptionSettlementMaxAttempts = 20

var ErrConvertedSubscriptionSettlementConflict = errors.New("converted subscription target changed during settlement")

func isRetryableConvertedSubscriptionSettlementError(err error) bool {
	if errors.Is(err, ErrConvertedSubscriptionSettlementConflict) {
		return true
	}
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite_locked")
}

// Update subscription token_used by delta (positive consume more, negative refund).
func PostConsumeUserSubscriptionDelta(userSubscriptionId int, delta int64) error {
	return PostConsumeUserSubscriptionTokenDelta(userSubscriptionId, delta)
}

func PostConsumeUserSubscriptionTokenDelta(userSubscriptionId int, delta int64) error {
	if userSubscriptionId <= 0 {
		return errors.New("invalid userSubscriptionId")
	}
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		if delta > int64(^uint(0)>>1) {
			return fmt.Errorf("subscription token delta out of int range: %d", delta)
		}
		return subscriptionTokenDeltaCoalescer.add(userSubscriptionId, delta)
	}
	return postConsumeUserSubscriptionTokenDeltaDirect(userSubscriptionId, delta)
}

func postConsumeUserSubscriptionTokenDeltaDirect(userSubscriptionId int, delta int64) error {
	return runConvertedSubscriptionSettlementWithRetry(func() error {
		return DB.Transaction(func(tx *gorm.DB) error {
			return postConsumeUserSubscriptionDeltaTx(tx, userSubscriptionId, delta)
		})
	})
}

func runConvertedSubscriptionSettlementWithRetry(run func() error) error {
	if run == nil {
		return errors.New("converted subscription settlement is nil")
	}
	var lastErr error
	for attempt := range convertedSubscriptionSettlementMaxAttempts {
		lastErr = run()
		if !isRetryableConvertedSubscriptionSettlementError(lastErr) {
			return lastErr
		}
		if attempt+1 < convertedSubscriptionSettlementMaxAttempts {
			time.Sleep(time.Duration(attempt+1) * time.Millisecond)
		}
	}
	return lastErr
}

func postConsumeUserSubscriptionDeltaTx(tx *gorm.DB, userSubscriptionId int, delta int64) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	updatedAt := common.GetTimestamp()
	updates := map[string]interface{}{
		"token_used": tokenUsedDeltaExpr(delta),
		"updated_at": updatedAt,
	}
	query := tx.Model(&UserSubscription{}).
		Where("id = ? AND (status IS NULL OR status <> ?)", userSubscriptionId, SubscriptionStatusConverted)
	if delta > 0 {
		query = query.Where("entitlement_type = ? OR token_limit <= 0 OR token_used + ? <= token_limit", SubscriptionEntitlementCreditBalance, delta)
	}
	res := query.Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	var source UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "status", "conversion_id", "converted_to_subscription_id").
		Where("id = ?", userSubscriptionId).First(&source).Error; err != nil {
		return err
	}
	if source.Status == SubscriptionStatusConverted {
		return applyConvertedSubscriptionTokenDeltaTx(tx, &source, delta)
	}
	if delta > 0 {
		return fmt.Errorf("subscription token used exceeds limit, delta=%d", delta)
	}
	return nil
}

func applyConvertedSubscriptionTokenDeltaTx(tx *gorm.DB, source *UserSubscription, delta int64) error {
	if tx == nil || source == nil || source.Id <= 0 || source.ConversionId <= 0 || source.ConvertedToSubscriptionId <= 0 {
		return errors.New("invalid converted subscription mapping")
	}
	var conversion SubscriptionConversion
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND source_subscription_id = ? AND target_subscription_id = ?", source.ConversionId, source.Id, source.ConvertedToSubscriptionId).
		First(&conversion).Error; err != nil {
		return err
	}
	var target UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "entitlement_type", "token_limit", "token_used").
		Where("id = ?", conversion.TargetSubscriptionId).First(&target).Error; err != nil {
		return err
	}
	if target.EntitlementType != SubscriptionEntitlementCreditBalance {
		return errors.New("converted subscription target is not Credit balance")
	}
	updatedTokenLimit := target.TokenLimit
	updatedTokenUsed, ok := checkedAddInt64(target.TokenUsed, delta)
	if !ok {
		return errors.New("converted subscription settlement overflow")
	}
	if updatedTokenUsed < 0 {
		refundedBeyondUsage, ok := checkedSubInt64(0, updatedTokenUsed)
		if !ok {
			return errors.New("converted subscription refund overflow")
		}
		updatedTokenLimit, ok = checkedAddInt64(target.TokenLimit, refundedBeyondUsage)
		if !ok {
			return errors.New("converted subscription refund overflow")
		}
		updatedTokenUsed = 0
	}
	update := tx.Model(&UserSubscription{}).
		Where("id = ? AND entitlement_type = ? AND token_limit = ? AND token_used = ?", target.Id, SubscriptionEntitlementCreditBalance, target.TokenLimit, target.TokenUsed).
		Updates(map[string]any{
			"token_limit": updatedTokenLimit,
			"token_used":  updatedTokenUsed,
			"updated_at":  common.GetTimestamp(),
		})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return fmt.Errorf("%w: target=%d", ErrConvertedSubscriptionSettlementConflict, target.Id)
	}
	return nil
}

func PostConsumeUserSubscriptionAmountDelta(userSubscriptionId int, delta int64) error {
	if userSubscriptionId <= 0 {
		return errors.New("invalid userSubscriptionId")
	}
	if delta == 0 {
		return nil
	}
	return runConvertedSubscriptionSettlementWithRetry(func() error {
		return DB.Transaction(func(tx *gorm.DB) error {
			return postConsumeUserSubscriptionAmountDeltaTx(tx, userSubscriptionId, delta)
		})
	})
}

func postConsumeUserSubscriptionAmountDeltaTx(tx *gorm.DB, userSubscriptionId int, delta int64) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	updatedAt := common.GetTimestamp()
	updates := map[string]interface{}{
		"amount_used": amountUsedDeltaExpr(delta),
		"updated_at":  updatedAt,
	}
	query := tx.Model(&UserSubscription{}).
		Where("id = ? AND (status IS NULL OR status <> ?)", userSubscriptionId, SubscriptionStatusConverted)
	if delta > 0 {
		query = query.Where("amount_total <= 0 OR amount_used + ? <= amount_total", delta)
	}
	res := query.Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	var source UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "status", "conversion_id", "converted_to_subscription_id").
		Where("id = ?", userSubscriptionId).First(&source).Error; err != nil {
		return err
	}
	if source.Status == SubscriptionStatusConverted {
		return applyConvertedSubscriptionTokenDeltaTx(tx, &source, delta)
	}
	if delta > 0 {
		return fmt.Errorf("subscription used exceeds total, delta=%d", delta)
	}
	return nil
}
