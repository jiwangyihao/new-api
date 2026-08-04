package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TimedSubscriptionValuationGrant struct {
	Id                    int    `json:"id"`
	IdempotencyKey        string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex:uidx_timed_valuation_grants_idempotency_key"`
	UserSubscriptionId    int    `json:"user_subscription_id" gorm:"not null;index:idx_timed_valuation_grants_subscription_id"`
	UserId                int    `json:"user_id" gorm:"not null;index:idx_timed_valuation_grants_user_id"`
	PlanId                int    `json:"plan_id" gorm:"not null;index:idx_timed_valuation_grants_plan_id"`
	SourceType            string `json:"source_type" gorm:"type:varchar(32);not null;uniqueIndex:uidx_timed_valuation_grants_source,priority:1"`
	SourceKey             string `json:"source_key" gorm:"type:varchar(160);not null;uniqueIndex:uidx_timed_valuation_grants_source,priority:2"`
	SourceId              int    `json:"source_id" gorm:"not null;default:0"`
	EventStartTime        int64  `json:"event_start_time" gorm:"type:bigint;not null;default:0"`
	EventEndTime          int64  `json:"event_end_time" gorm:"type:bigint;not null;default:0"`
	GrantCredit           int64  `json:"grant_credit" gorm:"type:bigint;not null;default:0"`
	SourcePriceMicros     int64  `json:"source_price_micros,string" gorm:"type:bigint;not null;default:0"`
	SourceCurrency        string `json:"source_currency" gorm:"type:varchar(8);not null"`
	ValuationAmountMicros int64  `json:"valuation_amount_micros,string" gorm:"type:bigint;not null;default:0"`
	ValuationCurrency     string `json:"valuation_currency" gorm:"type:varchar(8);not null"`
	Confidence            string `json:"confidence" gorm:"type:varchar(16);not null"`
	RuleVersion           int    `json:"rule_version" gorm:"not null"`
	FxRateNumerator       int64  `json:"fx_rate_numerator,string" gorm:"type:bigint;not null"`
	FxRateDenominator     int64  `json:"fx_rate_denominator,string" gorm:"type:bigint;not null"`
	FxCapturedAt          int64  `json:"fx_captured_at" gorm:"type:bigint;not null;default:0"`
	SourceSnapshot        string `json:"source_snapshot" gorm:"type:text"`
	CreatedAt             int64  `json:"created_at" gorm:"type:bigint;not null"`
}

func (*TimedSubscriptionValuationGrant) BeforeUpdate(*gorm.DB) error {
	return ErrTimedSubscriptionGrantImmutable
}

func (*TimedSubscriptionValuationGrant) BeforeDelete(*gorm.DB) error {
	return ErrTimedSubscriptionGrantImmutable
}

const (
	TimedSubscriptionGrantSourceOrder      = "subscription_order"
	TimedSubscriptionGrantSourceRedemption = "redemption"
	TimedSubscriptionGrantSourceAdmin      = "admin"

	TimedSubscriptionValuationConfidenceExact = "exact"
)

var (
	ErrTimedSubscriptionGrantInvalid             = errors.New("timed_subscription_grant_invalid")
	ErrTimedSubscriptionGrantIdempotencyMismatch = errors.New("timed_subscription_grant_idempotency_mismatch")
	ErrTimedSubscriptionGrantImmutable           = errors.New("timed_subscription_grant_immutable")
)

type TimedSubscriptionGrantRequest struct {
	UserId         int
	PlanId         int
	IdempotencyKey string
	SourceType     string
	SourceId       int
	Reason         string
}

type timedSubscriptionGrantSourceSnapshot struct {
	IdempotencyKey          string `json:"idempotency_key"`
	SourceType              string `json:"source_type"`
	SourceKey               string `json:"source_key"`
	SourceId                int    `json:"source_id"`
	UserId                  int    `json:"user_id"`
	PlanId                  int    `json:"plan_id"`
	SourcePriceMicros       int64  `json:"source_price_micros,string"`
	SourceCurrency          string `json:"source_currency"`
	Reason                  string `json:"reason,omitempty"`
	GrantCredit             int64  `json:"grant_credit"`
	DurationUnit            string `json:"duration_unit"`
	DurationValue           int    `json:"duration_value"`
	CustomSeconds           int64  `json:"custom_seconds"`
	QuotaResetPeriod        string `json:"quota_reset_period"`
	QuotaResetCustomSeconds int64  `json:"quota_reset_custom_seconds"`
	ValuationRuleVersion    int    `json:"valuation_rule_version"`
}

type normalizedTimedSubscriptionGrantRequest struct {
	request     TimedSubscriptionGrantRequest
	sourceKey   string
	grantSource string
}

type authoritativeTimedSubscriptionGrant struct {
	request        TimedSubscriptionGrantRequest
	plan           *SubscriptionPlan
	sourceKey      string
	grantSource    string
	sourceCurrency string
	priceMicros    int64
	snapshot       string
}

func GrantTimedSubscriptionTx(tx *gorm.DB, request TimedSubscriptionGrantRequest) (*UserSubscriptionCreationResult, error) {
	if tx == nil {
		return nil, ErrTimedSubscriptionGrantInvalid
	}
	normalized, err := normalizeTimedSubscriptionGrantRequest(request)
	if err != nil {
		return nil, err
	}
	planGuard := tx.Model(&SubscriptionPlan{}).
		Where("id = ?", normalized.request.PlanId).
		UpdateColumn("conversion_guard_version", gorm.Expr("conversion_guard_version + ?", 1))
	if planGuard.Error != nil {
		return nil, planGuard.Error
	}
	if planGuard.RowsAffected != 1 {
		return nil, ErrTimedSubscriptionGrantInvalid
	}
	if existing, found, err := findTimedSubscriptionGrantReplayTx(tx, normalized); err != nil {
		return nil, err
	} else if found {
		return existing, nil
	}

	plan, requireEnabled, err := authoritativeTimedSubscriptionGrantPlanTx(tx, normalized)
	if err != nil {
		return nil, err
	}
	authoritative, err := freezeAuthoritativeTimedSubscriptionGrant(normalized, plan, requireEnabled)
	if err != nil {
		return nil, err
	}

	creation, err := CreateUserSubscriptionFromPlanWithResultTx(tx, authoritative.request.UserId, authoritative.plan, authoritative.grantSource)
	if err != nil {
		return nil, err
	}
	if creation == nil || creation.Subscription == nil || creation.EventEndTime <= creation.EventStartTime {
		return nil, ErrTimedSubscriptionGrantInvalid
	}
	createdAt, err := getDBTimestampStrictTx(tx)
	if err != nil {
		return nil, err
	}
	grant := &TimedSubscriptionValuationGrant{
		IdempotencyKey:        authoritative.request.IdempotencyKey,
		UserSubscriptionId:    creation.Subscription.Id,
		UserId:                authoritative.request.UserId,
		PlanId:                authoritative.plan.Id,
		SourceType:            authoritative.request.SourceType,
		SourceKey:             authoritative.sourceKey,
		SourceId:              authoritative.request.SourceId,
		EventStartTime:        creation.EventStartTime,
		EventEndTime:          creation.EventEndTime,
		GrantCredit:           authoritative.plan.MonthlyTokenLimit,
		SourcePriceMicros:     authoritative.priceMicros,
		SourceCurrency:        authoritative.sourceCurrency,
		ValuationAmountMicros: authoritative.priceMicros,
		ValuationCurrency:     authoritative.sourceCurrency,
		Confidence:            TimedSubscriptionValuationConfidenceExact,
		RuleVersion:           CreditValuationRuleVersion,
		FxRateNumerator:       1,
		FxRateDenominator:     1,
		FxCapturedAt:          createdAt,
		SourceSnapshot:        authoritative.snapshot,
		CreatedAt:             createdAt,
	}
	if err := tx.Create(grant).Error; err != nil {
		return nil, err
	}
	return creation, nil
}
func normalizeTimedSubscriptionGrantRequest(request TimedSubscriptionGrantRequest) (normalizedTimedSubscriptionGrantRequest, error) {
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.SourceType = strings.TrimSpace(request.SourceType)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.UserId <= 0 || request.PlanId <= 0 || request.IdempotencyKey == "" {
		return normalizedTimedSubscriptionGrantRequest{}, ErrTimedSubscriptionGrantInvalid
	}

	var sourceKey string
	var grantSource string
	switch request.SourceType {
	case TimedSubscriptionGrantSourceOrder:
		if request.SourceId <= 0 {
			return normalizedTimedSubscriptionGrantRequest{}, ErrTimedSubscriptionGrantInvalid
		}
		sourceKey = request.SourceType + ":" + strconv.Itoa(request.SourceId)
		grantSource = SubscriptionGrantOrder
	case TimedSubscriptionGrantSourceRedemption:
		if request.SourceId <= 0 {
			return normalizedTimedSubscriptionGrantRequest{}, ErrTimedSubscriptionGrantInvalid
		}
		sourceKey = request.SourceType + ":" + strconv.Itoa(request.SourceId)
		grantSource = SubscriptionGrantRedemption
	case TimedSubscriptionGrantSourceAdmin:
		if request.Reason == "" {
			return normalizedTimedSubscriptionGrantRequest{}, ErrTimedSubscriptionGrantInvalid
		}
		sourceKey = TimedSubscriptionGrantSourceAdmin + ":" + request.IdempotencyKey
		grantSource = SubscriptionGrantAdmin
	default:
		return normalizedTimedSubscriptionGrantRequest{}, ErrTimedSubscriptionGrantInvalid
	}

	return normalizedTimedSubscriptionGrantRequest{
		request: request, sourceKey: sourceKey, grantSource: grantSource,
	}, nil
}

func authoritativeTimedSubscriptionGrantPlanTx(tx *gorm.DB, normalized normalizedTimedSubscriptionGrantRequest) (*SubscriptionPlan, bool, error) {
	if normalized.request.SourceType == TimedSubscriptionGrantSourceOrder {
		var order SubscriptionOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", normalized.request.SourceId).First(&order).Error; err != nil {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		if order.UserId != normalized.request.UserId || order.PlanId != normalized.request.PlanId || order.Status != common.TopUpStatusSuccess || strings.TrimSpace(order.EntitlementSnapshot) == "" {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		snapshot, err := UnmarshalSubscriptionEntitlementSnapshot(order.EntitlementSnapshot)
		if err != nil {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		purchaseMode, err := NormalizeSubscriptionPurchaseMode(snapshot.PurchaseMode)
		if err != nil || purchaseMode != SubscriptionPurchaseModeTimed {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		if err := validateSubscriptionOrderEntitlementSnapshot(tx, &order, snapshot, purchaseMode, order.PaymentMethod); err != nil {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		plan, err := SubscriptionPlanFromEntitlementSnapshot(snapshot)
		if err != nil {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		return plan, false, nil
	}
	if normalized.request.SourceType == TimedSubscriptionGrantSourceRedemption {
		var redemption Redemption
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", normalized.request.SourceId).First(&redemption).Error; err != nil {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		if redemption.UsedUserId != normalized.request.UserId || redemption.PlanId != normalized.request.PlanId || redemption.Status != common.RedemptionCodeStatusUsed || redemption.FulfillmentMode != RedemptionModeTimed || strings.TrimSpace(redemption.FulfillmentSnapshot) == "" {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		var fulfillment RedemptionFulfillmentSnapshot
		if err := common.UnmarshalJsonStr(redemption.FulfillmentSnapshot, &fulfillment); err != nil {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		plan, err := SubscriptionPlanFromEntitlementSnapshot(fulfillment.Entitlement)
		if err != nil || plan.Id != redemption.PlanId {
			return nil, false, ErrTimedSubscriptionGrantInvalid
		}
		return plan, false, nil
	}

	var plan SubscriptionPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", normalized.request.PlanId).First(&plan).Error; err != nil {
		return nil, false, ErrTimedSubscriptionGrantInvalid
	}
	return &plan, true, nil
}

func freezeAuthoritativeTimedSubscriptionGrant(normalized normalizedTimedSubscriptionGrantRequest, plan *SubscriptionPlan, requireEnabled bool) (authoritativeTimedSubscriptionGrant, error) {
	if plan == nil || plan.Id != normalized.request.PlanId || (requireEnabled && !plan.Enabled) || plan.EntitlementType != SubscriptionEntitlementTimed || plan.IsTrial || plan.InviteTrial || plan.PriceAmountMicros == nil || *plan.PriceAmountMicros <= 0 || plan.MonthlyTokenLimit <= 0 {
		return authoritativeTimedSubscriptionGrant{}, ErrTimedSubscriptionGrantInvalid
	}
	if !validTimedSubscriptionGrantDuration(plan) || !validTimedSubscriptionGrantReset(plan) {
		return authoritativeTimedSubscriptionGrant{}, ErrTimedSubscriptionGrantInvalid
	}
	currency := strings.ToUpper(strings.TrimSpace(plan.Currency))
	if currency != "CNY" && currency != "USD" {
		return authoritativeTimedSubscriptionGrant{}, ErrTimedSubscriptionGrantInvalid
	}

	snapshot := timedSubscriptionGrantSourceSnapshot{
		IdempotencyKey: normalized.request.IdempotencyKey, SourceType: normalized.request.SourceType, SourceKey: normalized.sourceKey, SourceId: normalized.request.SourceId,
		UserId: normalized.request.UserId, PlanId: plan.Id, SourcePriceMicros: *plan.PriceAmountMicros,
		SourceCurrency: currency, Reason: normalized.request.Reason, GrantCredit: plan.MonthlyTokenLimit,
		DurationUnit: plan.DurationUnit, DurationValue: plan.DurationValue, CustomSeconds: plan.CustomSeconds,
		QuotaResetPeriod: plan.QuotaResetPeriod, QuotaResetCustomSeconds: plan.QuotaResetCustomSeconds,
		ValuationRuleVersion: CreditValuationRuleVersion,
	}
	payload, err := common.Marshal(snapshot)
	if err != nil {
		return authoritativeTimedSubscriptionGrant{}, err
	}
	return authoritativeTimedSubscriptionGrant{
		request: normalized.request, plan: plan, sourceKey: normalized.sourceKey, grantSource: normalized.grantSource,
		sourceCurrency: currency, priceMicros: *plan.PriceAmountMicros, snapshot: string(payload),
	}, nil
}

func validTimedSubscriptionGrantDuration(plan *SubscriptionPlan) bool {
	switch plan.DurationUnit {
	case SubscriptionDurationYear, SubscriptionDurationMonth, SubscriptionDurationDay, SubscriptionDurationHour:
		return plan.DurationValue > 0
	case SubscriptionDurationCustom:
		return plan.CustomSeconds > 0
	default:
		return false
	}
}

func validTimedSubscriptionGrantReset(plan *SubscriptionPlan) bool {
	switch plan.QuotaResetPeriod {
	case SubscriptionResetNever, SubscriptionResetDaily, SubscriptionResetWeekly, SubscriptionResetMonthly:
		return true
	case SubscriptionResetCustom:
		return plan.QuotaResetCustomSeconds > 0
	default:
		return false
	}
}

func findTimedSubscriptionGrantReplayTx(tx *gorm.DB, normalized normalizedTimedSubscriptionGrantRequest) (*UserSubscriptionCreationResult, bool, error) {
	var grants []TimedSubscriptionValuationGrant
	if err := tx.Where("idempotency_key = ? OR (source_type = ? AND source_key = ?)", normalized.request.IdempotencyKey, normalized.request.SourceType, normalized.sourceKey).
		Order("id asc").Limit(2).Find(&grants).Error; err != nil {
		return nil, false, err
	}
	if len(grants) == 0 {
		return nil, false, nil
	}
	if len(grants) != 1 || !timedSubscriptionGrantMatchesRequest(grants[0], normalized) {
		return nil, false, ErrTimedSubscriptionGrantIdempotencyMismatch
	}
	var subscription UserSubscription
	if err := tx.Where("id = ?", grants[0].UserSubscriptionId).First(&subscription).Error; err != nil {
		return nil, false, err
	}
	return &UserSubscriptionCreationResult{
		Subscription: &subscription, EventStartTime: grants[0].EventStartTime, EventEndTime: grants[0].EventEndTime,
	}, true, nil
}

func timedSubscriptionGrantMatchesRequest(grant TimedSubscriptionValuationGrant, normalized normalizedTimedSubscriptionGrantRequest) bool {
	request := normalized.request
	if grant.IdempotencyKey != request.IdempotencyKey || grant.UserId != request.UserId || grant.PlanId != request.PlanId ||
		grant.SourceType != request.SourceType || grant.SourceKey != normalized.sourceKey || grant.SourceId != request.SourceId {
		return false
	}
	var snapshot timedSubscriptionGrantSourceSnapshot
	if err := common.UnmarshalJsonStr(grant.SourceSnapshot, &snapshot); err != nil {
		return false
	}
	return snapshot.IdempotencyKey == request.IdempotencyKey && snapshot.SourceType == request.SourceType &&
		snapshot.SourceKey == normalized.sourceKey && snapshot.SourceId == request.SourceId &&
		snapshot.UserId == request.UserId && snapshot.PlanId == request.PlanId && snapshot.Reason == request.Reason
}
