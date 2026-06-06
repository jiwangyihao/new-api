package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InvitationRewardEventSourceSubscriptionOrder      = "subscription_order"
	InvitationRewardEventSourceSubscriptionRedemption = "subscription_redemption"
	InvitationRewardEventSourceLegacySubscription     = "legacy_user_subscription"

	InvitationRewardEventStatusActive    = "active"
	InvitationRewardEventStatusCancelled = "cancelled"
)

const (
	InvitationCommissionSourceSubscriptionOrder      = InvitationRewardEventSourceSubscriptionOrder
	InvitationCommissionSourceSubscriptionRedemption = InvitationRewardEventSourceSubscriptionRedemption
	InvitationCommissionSourceLegacySubscription     = InvitationRewardEventSourceLegacySubscription

	InvitationCommissionStatusAvailable = "available"
	InvitationCommissionStatusSkipped   = "skipped"
	InvitationCommissionStatusCancelled = "cancelled"
)

const (
	InvitationCommissionReasonUnsupportedCurrency = "unsupported_currency"
	InvitationCommissionReasonInvalidSourceAmount = "invalid_source_amount"
	InvitationCommissionReasonCommissionOverflow  = "commission_overflow"
)

const (
	InvitationCommissionLedgerEarned              = "earned"
	InvitationCommissionLedgerTransferred         = "transferred_to_balance"
	InvitationCommissionLedgerWithdrawalCreated   = "withdrawal_created"
	InvitationCommissionLedgerWithdrawalRejected  = "withdrawal_rejected"
	InvitationCommissionLedgerWithdrawalCompleted = "withdrawal_completed"
)

const (
	InvitationCommissionWithdrawalPending   = "pending"
	InvitationCommissionWithdrawalCompleted = "completed"
	InvitationCommissionWithdrawalRejected  = "rejected"

	InvitationCommissionWithdrawalMethodManual = "manual"
)

type InvitationCommissionAccount struct {
	Id               int   `json:"id"`
	UserId           int   `json:"user_id" gorm:"type:int;not null;uniqueIndex"`
	AvailableCents   int64 `json:"available_cents" gorm:"type:bigint;not null;default:0"`
	PendingCents     int64 `json:"pending_cents" gorm:"type:bigint;not null;default:0"`
	WithdrawnCents   int64 `json:"withdrawn_cents" gorm:"type:bigint;not null;default:0"`
	TransferredCents int64 `json:"transferred_cents" gorm:"type:bigint;not null;default:0"`
	CreatedAt        int64 `json:"created_at" gorm:"type:bigint;not null;default:0"`
	UpdatedAt        int64 `json:"updated_at" gorm:"type:bigint;not null;default:0"`
}

type InvitationRewardEvent struct {
	Id                   int    `json:"id"`
	InviterId            int    `json:"inviter_id" gorm:"type:int;not null;index"`
	InviteeId            int    `json:"invitee_id" gorm:"type:int;not null;index"`
	SourceType           string `json:"source_type" gorm:"type:varchar(64);not null;index:idx_invitation_reward_event_source,unique"`
	SourceId             int    `json:"source_id" gorm:"type:int;not null;index:idx_invitation_reward_event_source,unique"`
	SourceOrderId        int    `json:"source_order_id" gorm:"type:int;not null;default:0;index"`
	SourceRedemptionId   int    `json:"source_redemption_id" gorm:"type:int;not null;default:0;index"`
	SourceSubscriptionId int    `json:"source_subscription_id" gorm:"type:int;not null;default:0;index"`
	SourceAmountCents    int64  `json:"source_amount_cents" gorm:"type:bigint;not null;default:0"`
	SourceCurrency       string `json:"source_currency" gorm:"type:varchar(8);not null;default:''"`
	EventStartTime       int64  `json:"event_start_time" gorm:"type:bigint;not null;default:0;index"`
	EventEndTime         int64  `json:"event_end_time" gorm:"type:bigint;not null;default:0;index"`
	Status               string `json:"status" gorm:"type:varchar(32);not null;default:'active';index"`
	Reason               string `json:"reason" gorm:"type:varchar(255);not null;default:''"`
	CreatedAt            int64  `json:"created_at" gorm:"type:bigint;not null;default:0"`
	UpdatedAt            int64  `json:"updated_at" gorm:"type:bigint;not null;default:0"`
}

type InvitationCommissionRecord struct {
	Id                int    `json:"id"`
	EventId           int    `json:"event_id" gorm:"type:int;not null;default:0;index"`
	InviterId         int    `json:"inviter_id" gorm:"type:int;not null;index"`
	InviteeId         int    `json:"invitee_id" gorm:"type:int;not null;index"`
	SourceType        string `json:"source_type" gorm:"type:varchar(64);not null;index:idx_invitation_commission_source,unique"`
	SourceId          int    `json:"source_id" gorm:"type:int;not null;index:idx_invitation_commission_source,unique"`
	SourceTradeNo     string `json:"source_trade_no" gorm:"type:varchar(128);not null;default:''"`
	SourceAmountCents int64  `json:"source_amount_cents" gorm:"type:bigint;not null;default:0"`
	SourceCurrency    string `json:"source_currency" gorm:"type:varchar(8);not null;default:''"`
	CommissionRateBps int    `json:"commission_rate_bps" gorm:"type:int;not null;default:0"`
	CommissionCents   int64  `json:"commission_cents" gorm:"type:bigint;not null;default:0"`
	Status            string `json:"status" gorm:"type:varchar(32);not null;default:'available';index"`
	Reason            string `json:"reason" gorm:"type:varchar(255);not null;default:''"`
	CreatedAt         int64  `json:"created_at" gorm:"type:bigint;not null;default:0"`
	AvailableAt       int64  `json:"available_at" gorm:"type:bigint;not null;default:0"`
	CancelledAt       int64  `json:"cancelled_at" gorm:"type:bigint;not null;default:0"`
}

type InvitationCommissionLedger struct {
	Id                  int    `json:"id"`
	UserId              int    `json:"user_id" gorm:"type:int;not null;index"`
	Type                string `json:"type" gorm:"type:varchar(64);not null;index"`
	AmountCents         int64  `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
	AvailableAfterCents int64  `json:"available_after_cents" gorm:"type:bigint;not null;default:0"`
	PendingAfterCents   int64  `json:"pending_after_cents" gorm:"type:bigint;not null;default:0"`
	ReferenceType       string `json:"reference_type" gorm:"type:varchar(64);not null;default:'';index"`
	ReferenceId         int    `json:"reference_id" gorm:"type:int;not null;default:0;index"`
	CreatedAt           int64  `json:"created_at" gorm:"type:bigint;not null;default:0"`
}

type InvitationCommissionWithdrawal struct {
	Id              int    `json:"id"`
	UserId          int    `json:"user_id" gorm:"type:int;not null;index"`
	AmountCents     int64  `json:"amount_cents" gorm:"type:bigint;not null;default:0"`
	Status          string `json:"status" gorm:"type:varchar(32);not null;default:'pending';index"`
	Method          string `json:"method" gorm:"type:varchar(32);not null;default:'manual'"`
	ContactSnapshot string `json:"contact_snapshot" gorm:"type:text"`
	UserRemark      string `json:"user_remark" gorm:"type:text"`
	AdminRemark     string `json:"admin_remark" gorm:"type:text"`
	ReviewerId      int    `json:"reviewer_id" gorm:"type:int;not null;default:0"`
	ReviewedAt      int64  `json:"reviewed_at" gorm:"type:bigint;not null;default:0"`
	CompletedBy     int    `json:"completed_by" gorm:"type:int;not null;default:0"`
	CompletedAt     int64  `json:"completed_at" gorm:"type:bigint;not null;default:0"`
	CreatedAt       int64  `json:"created_at" gorm:"type:bigint;not null;default:0"`
	UpdatedAt       int64  `json:"updated_at" gorm:"type:bigint;not null;default:0"`
}

type legacyInvitationRewardSubscriptionRow struct {
	Id               int
	UserId           int
	PlanId           int
	InviterId        int
	StartTime        int64
	EndTime          int64
	GrantReason      string
	Source           string
	PlanBusinessCode *string
}

func BackfillLegacyInvitationRewardEventsTx(tx *gorm.DB, now int64) error {
	if tx == nil {
		return errors.New("backfill legacy invitation reward events tx is nil")
	}
	var rows []legacyInvitationRewardSubscriptionRow
	if err := tx.Table("user_subscriptions").
		Select("user_subscriptions.id, user_subscriptions.user_id, user_subscriptions.plan_id, users.inviter_id, user_subscriptions.start_time, user_subscriptions.end_time, user_subscriptions.grant_reason, user_subscriptions.source, subscription_plans.business_code AS plan_business_code").
		Joins("JOIN users ON users.id = user_subscriptions.user_id").
		Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Where("user_subscriptions.status = ?", "active").
		Where("user_subscriptions.start_time <= ? AND user_subscriptions.end_time > ?", now, now).
		Where("users.inviter_id > 0").
		Where("subscription_plans.is_trial = ?", false).
		Where("subscription_plans.invite_trial = ?", false).
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if !isLegacyInvitationRewardSubscriptionSource(row.GrantReason, row.Source, row.PlanBusinessCode) {
			continue
		}
		existingNonLegacy, err := hasNonLegacyInvitationRewardEventForSubscriptionTx(tx, row.Id)
		if err != nil {
			return err
		}
		if existingNonLegacy {
			continue
		}
		amountCents, currency, err := legacyInvitationRewardSubscriptionAmountSnapshotTx(tx, row.UserId, row.PlanId, row.StartTime, row.EndTime, row.GrantReason, row.Source)
		if err != nil {
			return err
		}
		event := InvitationRewardEvent{
			InviterId:            row.InviterId,
			InviteeId:            row.UserId,
			SourceType:           InvitationRewardEventSourceLegacySubscription,
			SourceId:             row.Id,
			SourceSubscriptionId: row.Id,
			SourceAmountCents:    amountCents,
			SourceCurrency:       currency,
			EventStartTime:       row.StartTime,
			EventEndTime:         row.EndTime,
			Status:               InvitationRewardEventStatusActive,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_type"}, {Name: "source_id"}}, DoNothing: true}).Create(&event).Error; err != nil {
			return err
		}
	}
	return nil
}

func isLegacyInvitationRewardSubscriptionSource(grantReason string, source string, planBusinessCode *string) bool {
	if isExcludedLegacyInvitationRewardSubscriptionSource(grantReason) || isExcludedLegacyInvitationRewardSubscriptionSource(source) {
		return false
	}
	if planBusinessCode != nil && strings.TrimSpace(*planBusinessCode) == SubscriptionGrantMonthlyInviteEntitlement {
		return false
	}
	return true
}

func isExcludedLegacyInvitationRewardSubscriptionSource(value string) bool {
	switch strings.TrimSpace(value) {
	case "trial_code", "invite_trial", "admin", SubscriptionGrantMonthlyInviteEntitlement:
		return true
	default:
		return false
	}
}

func hasNonLegacyInvitationRewardEventForSubscriptionTx(tx *gorm.DB, subscriptionId int) (bool, error) {
	var count int64
	if err := tx.Model(&InvitationRewardEvent{}).
		Where("source_subscription_id = ? AND source_type <> ?", subscriptionId, InvitationRewardEventSourceLegacySubscription).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func legacyInvitationRewardSubscriptionAmountSnapshotTx(tx *gorm.DB, userId int, planId int, startTime int64, endTime int64, grantReason string, source string) (int64, string, error) {
	if startTime <= 0 || endTime <= startTime {
		return 0, "", nil
	}
	if isRedemptionSubscriptionSource(grantReason) || isRedemptionSubscriptionSource(source) {
		return legacyInvitationRewardSubscriptionRedemptionAmountSnapshotTx(tx, userId, planId, startTime, endTime)
	}
	if strings.TrimSpace(grantReason) == SubscriptionGrantOrder || strings.TrimSpace(source) == SubscriptionGrantOrder {
		return legacyInvitationRewardSubscriptionOrderAmountSnapshotTx(tx, userId, planId, startTime, endTime)
	}
	return 0, "", nil
}

func legacyInvitationRewardSubscriptionOrderAmountSnapshotTx(tx *gorm.DB, userId int, planId int, startTime int64, endTime int64) (int64, string, error) {
	var orders []SubscriptionOrder
	if err := tx.Select("id", "amount_cents", "currency", "create_time", "complete_time").
		Where("user_id = ? AND plan_id = ? AND status = ?", userId, planId, common.TopUpStatusSuccess).
		Where("((complete_time > 0 AND complete_time >= ? AND complete_time <= ?) OR (complete_time = 0 AND create_time > 0 AND create_time >= ? AND create_time <= ?))", startTime, endTime, startTime, endTime).
		Limit(2).
		Find(&orders).Error; err != nil {
		return 0, "", err
	}
	if len(orders) != 1 {
		return 0, "", nil
	}
	currency := strings.TrimSpace(orders[0].Currency)
	if orders[0].AmountCents <= 0 || currency == "" {
		return 0, "", nil
	}
	return orders[0].AmountCents, currency, nil
}

func legacyInvitationRewardSubscriptionRedemptionAmountSnapshotTx(tx *gorm.DB, userId int, planId int, startTime int64, endTime int64) (int64, string, error) {
	var redemptions []Redemption
	if err := tx.Select("id", "amount_cents", "currency", "redeemed_time").
		Where("used_user_id = ? AND plan_id = ? AND type = ? AND status = ?", userId, planId, RedemptionTypeSubscription, common.RedemptionCodeStatusUsed).
		Where("redeemed_time >= ? AND redeemed_time <= ?", startTime, endTime).
		Limit(2).
		Find(&redemptions).Error; err != nil {
		return 0, "", err
	}
	if len(redemptions) != 1 {
		return 0, "", nil
	}
	currency := strings.TrimSpace(redemptions[0].Currency)
	if redemptions[0].AmountCents <= 0 || currency == "" {
		return 0, "", nil
	}
	return redemptions[0].AmountCents, currency, nil
}

func isRedemptionSubscriptionSource(source string) bool {
	return strings.TrimSpace(source) == "redemption"
}
