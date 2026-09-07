package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// StorageRetentionBatchResult contains only committed progress. A full page may
// delete nothing: protected records must not prevent the next page being scanned.
type StorageRetentionBatchResult struct {
	LastID  int
	Scanned int
	Deleted int64
	Done    bool
}

func storageRetentionBatchSize(db *gorm.DB, requested int) int {
	if requested <= 0 {
		requested = subscriptionPreConsumeCleanupBatchSize
	}
	requested = min(requested, 5000)
	if db.Dialector.Name() == "sqlite" {
		requested = min(requested, 500)
	}
	return requested
}

// DeleteExpiredUsageLogs preserves financial entities, aggregate results and
// unfinished work. It deletes source logs together with their completed markers.
func DeleteExpiredUsageLogs(ctx context.Context, cutoff int64, batchSize int, afterID int) (StorageRetentionBatchResult, error) {
	return deleteOldLogPage(ctx, cutoff, batchSize, afterID, true)
}

func deleteOldLogPage(ctx context.Context, cutoff int64, batchSize int, afterID int, usageOnly bool) (StorageRetentionBatchResult, error) {
	if err := ctx.Err(); err != nil {
		return StorageRetentionBatchResult{}, err
	}
	if LOG_DB == nil || DB == nil {
		return StorageRetentionBatchResult{}, errors.New("retention databases are not initialized")
	}
	if cutoff <= 0 {
		return StorageRetentionBatchResult{}, errors.New("log retention cutoff must be positive")
	}
	batchSize = storageRetentionBatchSize(LOG_DB, batchSize)
	var result StorageRetentionBatchResult
	err := LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		hasEvents := tx.Migrator().HasTable(&LogAggregationEvent{})
		if err := ctx.Err(); err != nil {
			return err
		}
		if usageOnly && !hasEvents {
			return errors.New("log aggregation schema is required for automatic retention")
		}
		query := tx.Model(&Log{}).Where("id > ? AND created_at < ?", afterID, cutoff)
		if usageOnly {
			query = query.Where("type IN ?", []int{LogTypeConsume, LogTypeError})
		}
		var logs []Log
		if err := lockForUpdate(query).
			Select("id", "request_id", "user_id", "type", "subscription_id", "subscription_tokens_consumed", "other").
			Order("id ASC").Limit(batchSize).Find(&logs).Error; err != nil {
			return err
		}
		result.Scanned = len(logs)
		result.Done = len(logs) < batchSize
		if len(logs) == 0 {
			return nil
		}
		result.LastID = logs[len(logs)-1].Id
		ids := make([]int, 0, len(logs))
		requestIDs := make([]string, 0, len(logs))
		for i := range logs {
			fillLogDerivedFields(&logs[i])
			ids = append(ids, logs[i].Id)
			if logs[i].RequestId != "" {
				requestIDs = append(requestIDs, logs[i].RequestId)
			}
		}

		type aggregationState struct {
			usageDone bool
			freeDone  bool
			blocked   bool
		}
		states := make(map[int]aggregationState, len(logs))
		if hasEvents {
			var events []LogAggregationEvent
			if err := lockForUpdate(tx.Model(&LogAggregationEvent{})).
				Select("id", "log_id", "aggregate_name", "status").
				Where("log_id IN ?", ids).Order("id ASC").Find(&events).Error; err != nil {
				return err
			}
			for _, event := range events {
				state := states[event.LogID]
				if event.Status != logAggregationEventStatusApplied {
					state.blocked = true
				} else if event.AggregateName == logAggregationNameLogUsageHourly {
					state.usageDone = true
				} else if event.AggregateName == logAggregationNameFreeSubscriptionUsageHourly {
					state.freeDone = true
				}
				states[event.LogID] = state
			}
		}

		// A source log may still be the only completion proof for a legacy
		// pre-consume record. Query the main DB separately when logs are split.
		type requestKey struct {
			requestID      string
			userID         int
			subscriptionID int
		}
		protected := make(map[requestKey]struct{})
		primary := DB.WithContext(ctx)
		if DB == LOG_DB {
			primary = tx
		}
		if len(requestIDs) > 0 {
			var records []SubscriptionPreConsumeRecord
			if err := primary.Select("request_id", "user_id", "user_subscription_id").
				Where("request_id IN ?", requestIDs).Find(&records).Error; err != nil {
				return err
			}
			for _, record := range records {
				protected[requestKey{record.RequestId, record.UserId, record.UserSubscriptionId}] = struct{}{}
			}
		}
		eligible := make([]int, 0, len(logs))
		for _, log := range logs {
			state := states[log.Id]
			if state.blocked {
				continue
			}
			if hasEvents && (log.Type == LogTypeConsume || log.Type == LogTypeError) {
				if !state.usageDone {
					continue
				}
				if log.SubscriptionID != nil && *log.SubscriptionID > 0 && log.SubscriptionTokensConsumed != nil && *log.SubscriptionTokensConsumed > 0 && !state.freeDone {
					continue
				}
			}
			if log.SubscriptionID != nil {
				if _, exists := protected[requestKey{log.RequestId, log.UserId, *log.SubscriptionID}]; exists {
					continue
				}
			}
			eligible = append(eligible, log.Id)
		}
		if len(eligible) == 0 {
			return nil
		}
		if hasEvents {
			if err := tx.Where("log_id IN ?", eligible).Delete(&LogAggregationEvent{}).Error; err != nil {
				return err
			}
		}
		deleted := tx.Where("id IN ?", eligible).Delete(&Log{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected != int64(len(eligible)) {
			return fmt.Errorf("log retention deleted %d of %d locked rows", deleted.RowsAffected, len(eligible))
		}
		result.Deleted = deleted.RowsAffected
		return nil
	})
	if err != nil {
		return StorageRetentionBatchResult{}, err
	}
	return result, nil
}
