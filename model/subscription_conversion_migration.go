package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type timedSubscriptionGrantCandidate struct {
	GrantedAt      int64
	TimeSource     string
	GrantSource    string
	CreditSnapshot *int64
	priority       int
}

// BackfillTimedSubscriptionGrantMetadata initializes stable conversion metadata
// after UserSubscription has been migrated. Unknown histories start a conservative
// cooldown at the migration's database timestamp.
func BackfillTimedSubscriptionGrantMetadata() error {
	if DB == nil {
		return errors.New("database is nil")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		databaseNow, err := getDBTimestampStrictTx(tx)
		if err != nil {
			return err
		}
		return BackfillTimedSubscriptionGrantMetadataTx(tx, databaseNow)
	})
}

func BackfillTimedSubscriptionGrantMetadataTx(tx *gorm.DB, databaseNow int64) error {
	if tx == nil || databaseNow <= 0 {
		return errors.New("invalid timed subscription grant metadata migration")
	}
	var subscriptions []UserSubscription
	if err := tx.Where("entitlement_type = ? AND last_granted_at = ?", SubscriptionEntitlementTimed, 0).
		Order("id asc").Find(&subscriptions).Error; err != nil {
		return err
	}
	for i := range subscriptions {
		subscription := &subscriptions[i]
		candidate, err := latestReliableTimedSubscriptionGrantCandidateTx(tx, subscription)
		if err != nil {
			return err
		}
		updates := map[string]any{
			"last_granted_at":        databaseNow,
			"last_grant_time_source": SubscriptionGrantTimeSourceConservative,
			"last_grant_source":      normalizedSubscriptionGrantSource(subscription),
		}
		if candidate != nil {
			updates["last_granted_at"] = candidate.GrantedAt
			updates["last_grant_time_source"] = candidate.TimeSource
			if candidate.GrantSource != "" {
				updates["last_grant_source"] = candidate.GrantSource
			}
			if candidate.CreditSnapshot != nil {
				updates["last_grant_credit_snapshot"] = *candidate.CreditSnapshot
			}
		}
		result := tx.Model(&UserSubscription{}).
			Where("id = ? AND last_granted_at = ?", subscription.Id, 0).
			UpdateColumns(updates)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

func latestReliableTimedSubscriptionGrantCandidateTx(tx *gorm.DB, subscription *UserSubscription) (*timedSubscriptionGrantCandidate, error) {
	if tx == nil || subscription == nil {
		return nil, errors.New("invalid timed subscription grant candidate lookup")
	}
	var best *timedSubscriptionGrantCandidate
	consider := func(candidate timedSubscriptionGrantCandidate) {
		if candidate.GrantedAt <= 0 || !grantTimestampBelongsToSubscription(candidate.GrantedAt, subscription) {
			return
		}
		if best == nil || candidate.GrantedAt > best.GrantedAt || (candidate.GrantedAt == best.GrantedAt && candidate.priority > best.priority) {
			copy := candidate
			best = &copy
		}
	}

	var orders []SubscriptionOrder
	if err := tx.Select("id", "complete_time", "entitlement_snapshot").
		Where("user_id = ? AND plan_id = ? AND status = ? AND complete_time > ?", subscription.UserId, subscription.PlanId, common.TopUpStatusSuccess, 0).
		Order("complete_time desc, id desc").Find(&orders).Error; err != nil {
		return nil, err
	}
	for i := range orders {
		candidate := timedSubscriptionGrantCandidate{
			GrantedAt:   orders[i].CompleteTime,
			TimeSource:  SubscriptionGrantTimeSourceOrder,
			GrantSource: SubscriptionGrantOrder,
			priority:    3,
		}
		if payload := strings.TrimSpace(orders[i].EntitlementSnapshot); payload != "" {
			if snapshot, err := UnmarshalSubscriptionEntitlementSnapshot(payload); err == nil && snapshot.MonthlyTokenLimit > 0 {
				credit := snapshot.MonthlyTokenLimit
				candidate.CreditSnapshot = &credit
			}
		}
		unique, err := grantTimestampUniquelyIdentifiesSubscriptionTx(tx, subscription, candidate.GrantedAt)
		if err != nil {
			return nil, err
		}
		if !unique {
			continue
		}
		consider(candidate)
	}

	var redemptions []Redemption
	if err := tx.Select("id", "redeemed_time").
		Where("used_user_id = ? AND plan_id = ? AND type = ? AND status = ? AND redeemed_time > ?", subscription.UserId, subscription.PlanId, RedemptionTypeSubscription, common.RedemptionCodeStatusUsed, 0).
		Order("redeemed_time desc, id desc").Find(&redemptions).Error; err != nil {
		return nil, err
	}
	for i := range redemptions {
		unique, err := grantTimestampUniquelyIdentifiesSubscriptionTx(tx, subscription, redemptions[i].RedeemedTime)
		if err != nil {
			return nil, err
		}
		if !unique {
			continue
		}
		consider(timedSubscriptionGrantCandidate{
			GrantedAt:   redemptions[i].RedeemedTime,
			TimeSource:  SubscriptionGrantTimeSourceRedemption,
			GrantSource: SubscriptionGrantRedemption,
			priority:    2,
		})
	}

	var events []InvitationRewardEvent
	if err := tx.Select("id", "source_type", "created_at").
		Where("source_subscription_id = ? AND source_type IN ? AND created_at > ?", subscription.Id, []string{InvitationRewardEventSourceSubscriptionOrder, InvitationRewardEventSourceSubscriptionRedemption}, 0).
		Order("created_at desc, id desc").Find(&events).Error; err != nil {
		return nil, err
	}
	for i := range events {
		source := SubscriptionGrantOrder
		if events[i].SourceType == InvitationRewardEventSourceSubscriptionRedemption {
			source = SubscriptionGrantRedemption
		}
		consider(timedSubscriptionGrantCandidate{
			GrantedAt:   events[i].CreatedAt,
			TimeSource:  SubscriptionGrantTimeSourceReliableRecord,
			GrantSource: source,
			priority:    1,
		})
	}
	return best, nil
}

func grantTimestampUniquelyIdentifiesSubscriptionTx(tx *gorm.DB, subscription *UserSubscription, timestamp int64) (bool, error) {
	if tx == nil || subscription == nil || timestamp <= 0 {
		return false, nil
	}
	var count int64
	err := tx.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ? AND entitlement_type = ?", subscription.UserId, subscription.PlanId, SubscriptionEntitlementTimed).
		Where("(start_time = ? OR start_time <= ?) AND (end_time = ? OR end_time >= ?)", 0, timestamp, 0, timestamp).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count == 1, nil
}

func grantTimestampBelongsToSubscription(timestamp int64, subscription *UserSubscription) bool {
	if timestamp <= 0 || subscription == nil {
		return false
	}
	if subscription.StartTime > 0 && timestamp < subscription.StartTime {
		return false
	}
	if subscription.EndTime > 0 && timestamp > subscription.EndTime {
		return false
	}
	return true
}
