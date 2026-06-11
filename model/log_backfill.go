package model

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const (
	defaultLogDerivedColumnsBackfillLimit = 1000
	LogDerivedColumnsBackfillCheckpoint   = "LogDerivedColumnsBackfillCheckpoint"
	LogDerivedColumnsBackfillComplete     = "LogDerivedColumnsBackfillComplete"
)

var logDerivedColumnsBackfillBeforeUpdateHook func(log *Log) error

// BackfillLogDerivedColumnsBatch scans LOG_DB.logs by id and persists its checkpoint in DB.options.
func BackfillLogDerivedColumnsBatch(limit int) (processed int64, complete bool, err error) {
	return backfillLogDerivedColumnsBatch(context.Background(), limit)
}

func backfillLogDerivedColumnsBatch(ctx context.Context, limit int) (processed int64, complete bool, err error) {
	if limit <= 0 {
		limit = defaultLogDerivedColumnsBackfillLimit
	}

	if alreadyComplete, err := logDerivedColumnsBackfillIsComplete(); err != nil {
		return 0, false, err
	} else if alreadyComplete {
		return 0, true, nil
	}

	checkpoint, err := logDerivedColumnsBackfillCheckpoint()
	if err != nil {
		return 0, false, err
	}

	var logs []Log
	if err = LOG_DB.WithContext(ctx).
		Where("id > ?", checkpoint).
		Order("id ASC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return 0, false, err
	}

	lastID := checkpoint
	for i := range logs {
		if logs[i].Id > lastID {
			lastID = logs[i].Id
		}
		if err = backfillLogDerivedColumnsForLog(ctx, &logs[i]); err != nil {
			return 0, false, err
		}
	}

	complete = len(logs) < limit
	if !complete && len(logs) > 0 {
		hasMore, err := logDerivedColumnsBackfillHasMore(ctx, lastID)
		if err != nil {
			return 0, false, err
		}
		complete = !hasMore
	}

	if err = persistLogDerivedColumnsBackfillProgress(lastID, len(logs) > 0, complete); err != nil {
		return 0, false, err
	}

	return int64(len(logs)), complete, nil
}

func logDerivedColumnsBackfillIsComplete() (bool, error) {
	value, ok, err := logDerivedColumnsBackfillOptionValue(LogDerivedColumnsBackfillComplete)
	if err != nil || !ok {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value), "true"), nil
}

func logDerivedColumnsBackfillCheckpoint() (int, error) {
	value, ok, err := logDerivedColumnsBackfillOptionValue(LogDerivedColumnsBackfillCheckpoint)
	if err != nil || !ok || strings.TrimSpace(value) == "" {
		return 0, err
	}
	checkpoint, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if checkpoint < 0 {
		return 0, nil
	}
	return checkpoint, nil
}

func logDerivedColumnsBackfillOptionValue(key string) (string, bool, error) {
	var option Option
	err := DB.Where(commonKeyCol+" = ?", key).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return option.Value, true, nil
}

func logDerivedColumnsBackfillHasMore(ctx context.Context, afterID int) (bool, error) {
	var next Log
	err := LOG_DB.WithContext(ctx).
		Select("id").
		Where("id > ?", afterID).
		Order("id ASC").
		Take(&next).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

func persistLogDerivedColumnsBackfillProgress(lastID int, advanceCheckpoint bool, complete bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if advanceCheckpoint {
			if err := upsertOptionTx(tx, LogDerivedColumnsBackfillCheckpoint, strconv.Itoa(lastID)); err != nil {
				return err
			}
		}
		if complete {
			return upsertOptionTx(tx, LogDerivedColumnsBackfillComplete, "true")
		}
		return nil
	})
}

func backfillLogDerivedColumnsForLog(ctx context.Context, log *Log) error {
	updates := logDerivedColumnBackfillUpdates(log)
	if len(updates) == 0 {
		return nil
	}
	if logDerivedColumnsBackfillBeforeUpdateHook != nil {
		if err := logDerivedColumnsBackfillBeforeUpdateHook(log); err != nil {
			return err
		}
	}
	guardedUpdates := make(map[string]interface{}, len(updates))
	for column, value := range updates {
		guardedUpdates[column] = gorm.Expr("CASE WHEN "+column+" IS NULL THEN ? ELSE "+column+" END", value)
	}
	return LOG_DB.WithContext(ctx).Model(&Log{}).Where("id = ?", log.Id).Updates(guardedUpdates).Error
}

func logDerivedColumnBackfillUpdates(log *Log) map[string]interface{} {
	before := Log{
		SubscriptionID:             log.SubscriptionID,
		SubscriptionTokensConsumed: log.SubscriptionTokensConsumed,
		BillingSource:              log.BillingSource,
		Endpoint:                   log.Endpoint,
	}
	fillLogDerivedFields(log)

	updates := make(map[string]interface{}, 4)
	addLogDerivedColumnBackfillUpdate(updates, "subscription_id", before.SubscriptionID, log.SubscriptionID)
	addLogDerivedColumnBackfillUpdate(updates, "subscription_tokens_consumed", before.SubscriptionTokensConsumed, log.SubscriptionTokensConsumed)
	addLogDerivedColumnBackfillUpdate(updates, "billing_source", before.BillingSource, log.BillingSource)
	addLogDerivedColumnBackfillUpdate(updates, "endpoint", before.Endpoint, log.Endpoint)
	return updates
}

func addLogDerivedColumnBackfillUpdate[T comparable](updates map[string]interface{}, column string, before *T, after *T) {
	if before != nil || after == nil {
		return
	}
	updates[column] = *after
}
