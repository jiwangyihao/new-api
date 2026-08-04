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
	return errors.New("timed subscription valuation grant is immutable")
}

func (*TimedSubscriptionValuationGrant) BeforeDelete(*gorm.DB) error {
	return errors.New("timed subscription valuation grant is immutable")
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
)

type TimedSubscriptionGrantRequest struct {
	UserId            int
	Plan              *SubscriptionPlan
	IdempotencyKey    string
	SourceType        string
	SourceId          int
	SourcePriceMicros int64
	SourceCurrency    string
	Reason            string
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
	request        TimedSubscriptionGrantRequest
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
		Where("id = ?", normalized.request.Plan.Id).
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
	var eligibility SubscriptionPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", request.Plan.Id).First(&eligibility).Error; err != nil {
		return nil, err
	}
	if !eligibility.Enabled || eligibility.EntitlementType != SubscriptionEntitlementTimed || eligibility.IsTrial || eligibility.InviteTrial {
		return nil, ErrTimedSubscriptionGrantInvalid
	}
	request.Plan = &eligibility
	normalized, err = normalizeTimedSubscriptionGrantRequest(request)
	if err != nil {
		return nil, err
	}

	creation, err := CreateUserSubscriptionFromPlanWithResultTx(tx, request.UserId, request.Plan, normalized.grantSource)
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
		IdempotencyKey:        normalized.request.IdempotencyKey,
		UserSubscriptionId:    creation.Subscription.Id,
		UserId:                normalized.request.UserId,
		PlanId:                normalized.request.Plan.Id,
		SourceType:            normalized.request.SourceType,
		SourceKey:             normalized.sourceKey,
		SourceId:              normalized.request.SourceId,
		EventStartTime:        creation.EventStartTime,
		EventEndTime:          creation.EventEndTime,
		GrantCredit:           normalized.request.Plan.MonthlyTokenLimit,
		SourcePriceMicros:     normalized.priceMicros,
		SourceCurrency:        normalized.sourceCurrency,
		ValuationAmountMicros: normalized.priceMicros,
		ValuationCurrency:     normalized.sourceCurrency,
		Confidence:            TimedSubscriptionValuationConfidenceExact,
		RuleVersion:           CreditValuationRuleVersion,
		FxRateNumerator:       1,
		FxRateDenominator:     1,
		FxCapturedAt:          createdAt,
		SourceSnapshot:        normalized.snapshot,
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
	request.SourceCurrency = strings.ToUpper(strings.TrimSpace(request.SourceCurrency))
	request.Reason = strings.TrimSpace(request.Reason)
	if request.UserId <= 0 || request.Plan == nil || request.Plan.Id <= 0 || request.Plan.EntitlementType != SubscriptionEntitlementTimed || request.SourcePriceMicros <= 0 || request.SourceCurrency == "" || request.Plan.MonthlyTokenLimit <= 0 || request.IdempotencyKey == "" {
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

	snapshot := timedSubscriptionGrantSourceSnapshot{
		IdempotencyKey: request.IdempotencyKey, SourceType: request.SourceType, SourceKey: sourceKey, SourceId: request.SourceId,
		UserId: request.UserId, PlanId: request.Plan.Id, SourcePriceMicros: request.SourcePriceMicros,
		SourceCurrency: request.SourceCurrency, Reason: request.Reason, GrantCredit: request.Plan.MonthlyTokenLimit,
		DurationUnit: request.Plan.DurationUnit, DurationValue: request.Plan.DurationValue, CustomSeconds: request.Plan.CustomSeconds,
		QuotaResetPeriod: NormalizeResetPeriod(request.Plan.QuotaResetPeriod), QuotaResetCustomSeconds: request.Plan.QuotaResetCustomSeconds,
		ValuationRuleVersion: CreditValuationRuleVersion,
	}
	payload, err := common.Marshal(snapshot)
	if err != nil {
		return normalizedTimedSubscriptionGrantRequest{}, err
	}
	return normalizedTimedSubscriptionGrantRequest{
		request: request, sourceKey: sourceKey, grantSource: grantSource,
		sourceCurrency: request.SourceCurrency, priceMicros: request.SourcePriceMicros, snapshot: string(payload),
	}, nil
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
	return grant.IdempotencyKey == request.IdempotencyKey &&
		grant.UserId == request.UserId && grant.PlanId == request.Plan.Id &&
		grant.SourceType == request.SourceType && grant.SourceKey == normalized.sourceKey && grant.SourceId == request.SourceId &&
		grant.GrantCredit == request.Plan.MonthlyTokenLimit && grant.SourcePriceMicros == normalized.priceMicros &&
		grant.SourceCurrency == normalized.sourceCurrency && grant.ValuationAmountMicros == normalized.priceMicros &&
		grant.ValuationCurrency == normalized.sourceCurrency && grant.Confidence == TimedSubscriptionValuationConfidenceExact &&
		grant.RuleVersion == CreditValuationRuleVersion && grant.FxRateNumerator == 1 && grant.FxRateDenominator == 1 &&
		grant.SourceSnapshot == normalized.snapshot
}
