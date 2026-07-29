package model

import (
	"errors"
	"fmt"
	"math"
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
	SubscriptionPurchaseModeTimed         = SubscriptionEntitlementTimed
	SubscriptionPurchaseModeCreditBalance = SubscriptionEntitlementCreditBalance
)

const (
	SubscriptionGrantOrder                    = "order"
	SubscriptionGrantMonthlyInviteEntitlement = "monthly_invite_entitlement"
)

var (
	ErrSubscriptionOrderNotFound      = errors.New("subscription order not found")
	ErrSubscriptionOrderStatusInvalid = errors.New("subscription order status invalid")
	ErrNoActiveSubscription           = errors.New("no active subscription")
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

	// Display money amount (follow existing code style: float64 for money)
	PriceAmount float64 `json:"price_amount" gorm:"precision:10;scale:6;not null;default:0"`
	Currency    string  `json:"currency" gorm:"type:varchar(8);not null;default:'USD'"`

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
	ModelLimits                    string  `json:"model_limits" gorm:"type:text"`
	CreditBalanceConfigured        bool    `json:"credit_balance_configured" gorm:"not null;default:false"`
	CreditBalancePurchaseEnabled   bool    `json:"credit_balance_purchase_enabled" gorm:"not null;default:false"`
	CreditBalanceRedemptionEnabled bool    `json:"credit_balance_redemption_enabled" gorm:"not null;default:false"`
	CreditBalanceConversionEnabled bool    `json:"credit_balance_conversion_enabled" gorm:"not null;default:false"`
	UnlimitedPurchaseEnabled       bool    `json:"unlimited_purchase_enabled" gorm:"not null;default:false"`
	TimedConversionEnabled         bool    `json:"timed_conversion_enabled" gorm:"not null;default:false"`

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
	Id          int     `json:"id"`
	UserId      int     `json:"user_id" gorm:"index"`
	PlanId      int     `json:"plan_id" gorm:"index"`
	Money       float64 `json:"money"`
	AmountCents int64   `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
	Currency    string  `json:"currency" gorm:"type:varchar(8);not null;default:''"`

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

	TokenLimit        int64  `json:"token_limit" gorm:"type:bigint;not null;default:0"`
	TokenUsed         int64  `json:"token_used" gorm:"type:bigint;not null;default:0"`
	ConcurrencyLimit  int    `json:"concurrency_limit" gorm:"type:int;not null;default:0"`
	GrantReason       string `json:"grant_reason" gorm:"type:varchar(32);default:'';index"`
	GrantSourceUserId int    `json:"grant_source_user_id" gorm:"type:int;default:0;index"`

	StartTime int64  `json:"start_time" gorm:"bigint"`
	EndTime   int64  `json:"end_time" gorm:"bigint;index;index:idx_user_sub_active,priority:3;index:idx_user_sub_active_order,priority:3"`
	Status    string `json:"status" gorm:"type:varchar(32);index;index:idx_user_sub_active,priority:2;index:idx_user_sub_active_order,priority:2"` // active/expired/cancelled

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
	nowUnix := getDBTimestampTx(tx)
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
	if plan.MaxPurchasePerUser > 0 {
		var count int64
		if err := tx.Model(&UserSubscription{}).
			Where("user_id = ? AND plan_id = ?", userId, plan.Id).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			return nil, errors.New("已达到该套餐购买上限")
		}
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

		existing.UpdatedAt = common.GetTimestamp()
		fields := []string{
			"end_time",
			"token_limit",
			"concurrency_limit",
			"next_reset_time",
			"updated_at",
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
		UserId:           userId,
		PlanId:           plan.Id,
		EntitlementType:  plan.EntitlementType,
		AmountTotal:      plan.TotalAmount,
		AmountUsed:       0,
		TokenLimit:       plan.MonthlyTokenLimit,
		TokenUsed:        0,
		ConcurrencyLimit: plan.ConcurrencyLimit,
		GrantReason:      source,
		StartTime:        now.Unix(),
		EndTime:          endUnix,
		Status:           "active",
		Source:           source,
		LastResetTime:    lastReset,
		NextResetTime:    nextReset,
		UpgradeGroup:     "",
		PrevUserGroup:    "",
		CreatedAt:        common.GetTimestamp(),
		UpdatedAt:        common.GetTimestamp(),
	}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	return &UserSubscriptionCreationResult{Subscription: sub, EventStartTime: sub.StartTime, EventEndTime: sub.EndTime}, nil
}

func CompleteSubscriptionOrderTx(tx *gorm.DB, order *SubscriptionOrder, providerPayload string, actualPaymentMethod string) (*SubscriptionOrderCompletionResult, error) {
	if tx == nil || order == nil {
		return nil, errors.New("invalid subscription order")
	}
	if order.Status == common.TopUpStatusSuccess {
		return subscriptionOrderCompletionResultFromExistingFulfillmentTx(tx, order, false)
	}
	if order.Status != common.TopUpStatusPending {
		return nil, ErrSubscriptionOrderStatusInvalid
	}
	completeTime := common.GetTimestamp()
	updates := map[string]any{
		"status":        common.TopUpStatusSuccess,
		"complete_time": completeTime,
	}
	if providerPayload != "" {
		updates["provider_payload"] = providerPayload
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
		if err := tx.Where("id = ?", order.Id).First(&existing).Error; err != nil {
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

	purchaseMode := SubscriptionPurchaseModeTimed
	var snapshot SubscriptionEntitlementSnapshot
	var plan *SubscriptionPlan
	var err error
	if strings.TrimSpace(order.EntitlementSnapshot) != "" {
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
		plan, err = SubscriptionPlanFromEntitlementSnapshot(snapshot)
	} else {
		plan, err = getSubscriptionPlanByIdTx(tx, order.PlanId)
	}
	if err != nil {
		return nil, err
	}

	if purchaseMode == SubscriptionPurchaseModeCreditBalance {
		if order.PaymentProvider != PaymentProviderBalance {
			return nil, errors.New("Credit 余额仅支持人民币账户余额购买")
		}
		grant, err := GrantCreditBalanceTx(tx, CreditBalanceGrantRequest{
			UserId:         order.UserId,
			GrossCredit:    snapshot.MonthlyTokenLimit,
			IdempotencyKey: order.TradeNo,
			SourceType:     CreditBalanceLedgerSourceSubscriptionOrder,
			SourceId:       order.Id,
			Type:           CreditBalanceLedgerTypePurchase,
			TargetPlanId:   snapshot.TargetCreditBalancePlanID,
			Reason:         "人民币账户余额购买 Credit 余额",
		})
		if err != nil {
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
		if err := tx.Where(refCol+" = ?", tradeNo).First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if order.Status != common.TopUpStatusPending && order.Status != common.TopUpStatusSuccess {
			return ErrSubscriptionOrderStatusInvalid
		}
		plan, err := getSubscriptionPlanByIdTx(tx, order.PlanId)
		if err != nil {
			return err
		}
		if !plan.Enabled {
			// still allow completion for already purchased orders
		}
		completion, err := CompleteSubscriptionOrderTx(tx, &order, providerPayload, actualPaymentMethod)
		if err != nil {
			return err
		}
		result = completion
		if completion != nil && completion.Transitioned {
			logUserId = order.UserId
			logPlanTitle = plan.Title
			logMoney = order.Money
			logPaymentMethod = order.PaymentMethod
		}
		return nil
	})
	if err != nil {
		return nil, err
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
				UserId:        order.UserId,
				Amount:        0,
				Money:         order.Money,
				TradeNo:       order.TradeNo,
				PaymentMethod: order.PaymentMethod,
				CreateTime:    order.CreateTime,
				CompleteTime:  now,
				Status:        common.TopUpStatusSuccess,
			}
			return tx.Create(&topup).Error
		}
		return err
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
// This is a lightweight existence check to avoid heavy pre-consume transactions.
func HasActiveUserSubscription(userId int) (bool, error) {
	return hasActiveUserSubscriptionForModel(userId, "")
}

// HasActiveUserSubscriptionForModel returns whether the user has a billable active
// subscription whose model scope includes modelName.
func HasActiveUserSubscriptionForModel(userId int, modelName string) (bool, error) {
	return hasActiveUserSubscriptionForModel(userId, modelName)
}

func hasActiveUserSubscriptionForModel(userId int, modelName string) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var subs []UserSubscription
	if err := DB.Where("user_id = ? AND status = ? AND (entitlement_type = ? OR end_time > ?)", userId, "active", SubscriptionEntitlementCreditBalance, now).Find(&subs).Error; err != nil {
		return false, err
	}
	for i := range subs {
		plan, err := getSubscriptionPlanByIdTx(DB, subs[i].PlanId)
		if err != nil {
			return false, err
		}
		if !subscriptionPlanAllowsModel(plan, modelName) {
			continue
		}
		if usable, _ := isBillableSubscriptionCandidate(&subs[i], plan, 1); usable {
			return true, nil
		}
	}
	return false, nil
}

type ActiveSubscriptionUsage struct {
	TokenLimit int64
	TokenUsed  int64
	EndTime    int64
	Unlimited  bool
}

func GetActiveDistributorSubscriptionUsage(userId int) (*ActiveSubscriptionUsage, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var subs []UserSubscription
	if err := DB.Where("user_id = ? AND status = ? AND (entitlement_type = ? OR end_time > ?)", userId, "active", SubscriptionEntitlementCreditBalance, now).
		Order(primaryBillableSubscriptionOrder).
		Find(&subs).Error; err != nil {
		return nil, err
	}
	for _, sub := range subs {
		plan, err := GetSubscriptionPlanById(sub.PlanId)
		if err != nil {
			return nil, err
		}
		if isUnlimitedTrialSubscription(&sub, plan) {
			return &ActiveSubscriptionUsage{TokenLimit: sub.TokenLimit, TokenUsed: sub.TokenUsed, EndTime: sub.EndTime, Unlimited: true}, nil
		}
		if isDistributorSubscription(&sub, plan) {
			return &ActiveSubscriptionUsage{TokenLimit: sub.TokenLimit, TokenUsed: sub.TokenUsed, EndTime: sub.EndTime}, nil
		}
	}
	return &ActiveSubscriptionUsage{}, nil
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
	if err := DB.Where("status = ? AND end_time > 0 AND end_time <= ?", "active", now).
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
				Where("user_id = ? AND status = ? AND end_time > 0 AND end_time <= ?", userId, "active", now).
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
	Id                 int    `json:"id"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId             int    `json:"user_id" gorm:"index"`
	UserSubscriptionId int    `json:"user_subscription_id" gorm:"index"`
	PreConsumed        int64  `json:"pre_consumed" gorm:"type:bigint;not null;default:0"`
	Status             string `json:"status" gorm:"type:varchar(32);index"` // consumed/refunded
	CreatedAt          int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt          int64  `json:"updated_at" gorm:"bigint;index"`
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

func subscriptionPlanAllowsModel(plan *SubscriptionPlan, modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || plan == nil || strings.TrimSpace(plan.ModelLimits) == "" {
		return true
	}

	for _, allowed := range strings.Split(plan.ModelLimits, ",") {
		if strings.TrimSpace(allowed) == modelName {
			return true
		}
	}
	return false
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

func getCachedPrimaryBillableSubscription(tx *gorm.DB, userId int, setting string, requiredTokens int64, now int64, modelName string) (*primaryBillableSubscription, bool) {
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
	selection := entry.loaded
	sub := selection.Subscription
	if sub.Status != "active" || (sub.EntitlementType != SubscriptionEntitlementCreditBalance && sub.EndTime <= now) {
		return nil, false
	}
	var usage struct {
		AmountUsed int64
		TokenUsed  int64
	}
	if err := tx.Model(&UserSubscription{}).Select("amount_used", "token_used").Where("id = ?", sub.Id).Take(&usage).Error; err != nil {
		return nil, false
	}
	sub.AmountUsed = usage.AmountUsed
	sub.TokenUsed = usage.TokenUsed
	plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
	if err != nil {
		return nil, false
	}
	selection.Plan = plan
	if !subscriptionPlanAllowsModel(plan, modelName) {
		return nil, false
	}
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
	if selection == nil {
		return
	}
	loaded := *selection
	primaryBillableSubscriptionCache.Store(primaryBillableSubscriptionCacheKey(userId), primaryBillableSubscriptionCacheEntry{setting: setting, loaded: loaded})
}

func ClearPrimaryBillableSubscriptionCacheForTest() {
	primaryBillableSubscriptionCache = sync.Map{}
}

func cachePrimaryBillableSelectionTx(tx *gorm.DB, userId int, sub *UserSubscription, plan *SubscriptionPlan, distributor bool) {
	if tx == nil || sub == nil || userId <= 0 {
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

func selectPrimaryBillableSubscriptionTx(tx *gorm.DB, userId int, now int64, requiredTokens int64, forUpdate bool, resetDue bool, modelName string) (*primaryBillableSubscription, bool, bool, error) {
	if tx == nil {
		tx = DB
	}
	var user User
	if err := tx.Select("setting").Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, false, false, err
	}
	setting := user.Setting
	activeSubscriptionId := user.GetSetting().ActiveSubscriptionId
	if forUpdate {
		if cached, ok := getCachedPrimaryBillableSubscription(tx, userId, setting, requiredTokens, now, modelName); ok && (activeSubscriptionId <= 0 || cached.Subscription.Id == activeSubscriptionId) {
			return cached, true, true, nil
		}
	}
	var subs []UserSubscription
	query := tx
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}
	if err := query.Where("user_id = ? AND status = ? AND (entitlement_type = ? OR end_time > ?)", userId, "active", SubscriptionEntitlementCreditBalance, now).
		Order(primaryBillableSubscriptionOrder).
		Find(&subs).Error; err != nil {
		return nil, false, false, err
	}
	sawDistributorSubscription := false
	sawModelAllowedSubscription := false
	candidates := make([]billableSubscriptionCandidate, 0, len(subs))
	for i, candidate := range subs {
		sub := candidate
		plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
		if err != nil {
			return nil, sawDistributorSubscription, sawModelAllowedSubscription, err
		}
		if resetDue {
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &sub, plan, now); err != nil {
				return nil, sawDistributorSubscription, sawModelAllowedSubscription, err
			}
		}
		distributor := isDistributorSubscription(&sub, plan)
		if !distributor {
			continue
		}
		sawDistributorSubscription = true
		if !subscriptionPlanAllowsModel(plan, modelName) {
			continue
		}
		sawModelAllowedSubscription = true
		ok, unlimited := isBillableSubscriptionCandidate(&sub, plan, requiredTokens)
		if !ok {
			continue
		}
		entry := billableSubscriptionCandidate{sub: sub, plan: plan, distributor: distributor, unlimited: unlimited, index: i}
		if activeSubscriptionId > 0 && sub.Id == activeSubscriptionId {
			selection := buildPrimaryBillableSubscription(entry)
			if forUpdate {
				setCachedPrimaryBillableSubscription(userId, setting, selection)
			}
			return selection, sawDistributorSubscription, sawModelAllowedSubscription, nil
		}
		candidates = append(candidates, entry)
	}
	if len(candidates) > 0 && isPaidEquivalentSubscription(&candidates[0].sub, candidates[0].plan) {
		tier := subscriptionTierKey(candidates[0].plan)
		if tier != "" {
			for i := 1; i < len(candidates); i++ {
				candidate := candidates[i]
				if isInvitationRewardSubscription(&candidate.sub) && subscriptionTierKey(candidate.plan) == tier {
					selection := buildPrimaryBillableSubscription(candidate)
					if forUpdate {
						setCachedPrimaryBillableSubscription(userId, setting, selection)
					}
					return selection, sawDistributorSubscription, sawModelAllowedSubscription, nil
				}
			}
		}
	}
	if len(candidates) > 0 {
		selection := buildPrimaryBillableSubscription(candidates[0])
		if forUpdate {
			setCachedPrimaryBillableSubscription(userId, setting, selection)
		}
		return selection, sawDistributorSubscription, sawModelAllowedSubscription, nil
	}
	return nil, sawDistributorSubscription, sawModelAllowedSubscription, nil
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
	err = DB.Transaction(func(tx *gorm.DB) error {
		selection, _, _, err := selectPrimaryBillableSubscriptionTx(tx, userId, now, 1, true, true, "")
		if err != nil {
			return err
		}
		if selection == nil {
			return nil
		}
		sub := selection.Subscription
		summary.ActiveCount = 1
		summary.SubscriptionId = sub.Id
		summary.PlanId = sub.PlanId
		if selection.Plan != nil {
			summary.PrimaryPlanTitle = selection.Plan.Title
		}
		summary.TokenLimit = sub.TokenLimit
		summary.TokenUsed = sub.TokenUsed
		if selection.TokenUnlimited {
			summary.TokenUnlimited = true
		} else if sub.TokenLimit > sub.TokenUsed {
			summary.TokenRemaining = sub.TokenLimit - sub.TokenUsed
		}
		summary.ConcurrencyLimit = livePlanConcurrencyLimit(&sub, selection.Plan)
		summary.QueueCapacity = livePlanQueueCapacity(selection.Plan)
		summary.GPTAbuseWarningLimit = ResolveGPTAbuseWarningLimit(selection.Plan)
		summary.NextResetTime = sub.NextResetTime
		summary.EndTime = sub.EndTime
		return nil
	})
	if err != nil {
		return summary, err
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
	err := DB.Transaction(func(tx *gorm.DB) error {
		selected, _, _, err := selectPrimaryBillableSubscriptionTx(tx, userId, now, 1, false, true, "")
		if err != nil {
			return err
		}
		selection = selected
		return nil
	})
	if err != nil {
		return false, "", err
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
	return DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := tx.Where("id = ? AND user_id = ? AND status = ? AND (entitlement_type = ? OR end_time > ?)", subscriptionId, userId, "active", SubscriptionEntitlementCreditBalance, now).First(&sub).Error; err != nil {
			return err
		}
		var user User
		if err := tx.Select("setting").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		setting := user.GetSetting()
		setting.ActiveSubscriptionId = subscriptionId
		settingBytes, err := common.Marshal(setting)
		if err != nil {
			return err
		}
		settingJSON := string(settingBytes)
		if err := tx.Model(&User{}).Where("id = ?", userId).Update("setting", settingJSON).Error; err != nil {
			return err
		}
		return updateUserSettingCache(userId, settingJSON)
	})
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
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ? AND user_id = ? AND status = ? AND end_time > ?", subscriptionId, userId, "active", now).First(&sub).Error; err != nil {
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

	err := DB.Transaction(func(tx *gorm.DB) error {
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
			cachePrimaryBillableSelectionTx(tx, userId, &sub, plan, returnValue.DistributorTokenBilling)
			return nil
		}

		selection, sawDistributorSubscription, sawModelAllowedSubscription, err := selectPrimaryBillableSubscriptionTx(tx, userId, now, distributorAmount, true, true, modelName)
		if err != nil {
			return err
		}
		if selection == nil {
			if !sawDistributorSubscription {
				return ErrNoActiveSubscription
			}
			if !sawModelAllowedSubscription {
				return fmt.Errorf("subscription model not allowed: %s", modelName)
			}
			return fmt.Errorf("subscription token quota insufficient, need=%d", distributorAmount)
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
		if err := tx.Create(record).Error; err != nil {
			var dup SubscriptionPreConsumeRecord
			if err2 := tx.Where("request_id = ?", requestId).First(&dup).Error; err2 == nil {
				if dup.Status == "refunded" {
					return errors.New("subscription pre-consume already refunded")
				}
				fillSubscriptionPreConsumeResult(returnValue, &sub, selection.Plan, dup.PreConsumed, sub.AmountUsed, sub.TokenUsed, distributor)
				cachePrimaryBillableSelectionTx(tx, userId, &sub, selection.Plan, distributor)
				return nil
			}
			return err
		}
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
		fillSubscriptionPreConsumeResult(returnValue, &sub, selection.Plan, consumeAmount, amountUsedBefore, tokenUsedBefore, distributor)
		cachePrimaryBillableSelectionTx(tx, userId, &sub, selection.Plan, distributor)
		return nil
	})
	if err != nil {
		return nil, err
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
	if err := DB.Where("next_reset_time > 0 AND next_reset_time <= ? AND status = ?", now, "active").
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
				Where("id = ? AND next_reset_time > 0 AND next_reset_time <= ?", subCopy.Id, now).
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
	return DB.Transaction(func(tx *gorm.DB) error {
		return postConsumeUserSubscriptionDeltaTx(tx, userSubscriptionId, delta)
	})
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
	query := tx.Model(&UserSubscription{}).Where("id = ?", userSubscriptionId)
	if delta > 0 {
		query = query.Where("entitlement_type = ? OR token_limit <= 0 OR token_used + ? <= token_limit", SubscriptionEntitlementCreditBalance, delta)
	}
	res := query.Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 && delta > 0 {
		return fmt.Errorf("subscription token used exceeds limit, delta=%d", delta)
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
	return DB.Transaction(func(tx *gorm.DB) error {
		return postConsumeUserSubscriptionAmountDeltaTx(tx, userSubscriptionId, delta)
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
	query := tx.Model(&UserSubscription{}).Where("id = ?", userSubscriptionId)
	if delta > 0 {
		query = query.Where("amount_total <= 0 OR amount_used + ? <= amount_total", delta)
	}
	res := query.Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 && delta > 0 {
		return fmt.Errorf("subscription used exceeds total, delta=%d", delta)
	}
	return nil
}
