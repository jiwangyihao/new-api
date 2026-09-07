package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

type storageRetentionConfig struct {
	LogDays   int
	BatchSize int
	Budget    time.Duration
}

type storageRetentionCursor struct {
	PreConsumeID int
	LogID        int
}

var storageRetentionOnce sync.Once

func storageRetentionConfigFromEnv() (storageRetentionConfig, error) {
	config := storageRetentionConfig{
		LogDays:   common.GetEnvOrDefault("LOG_RETENTION_DAYS", 0),
		BatchSize: common.GetEnvOrDefault("STORAGE_RETENTION_BATCH_SIZE", 1000),
	}
	seconds := common.GetEnvOrDefault("STORAGE_RETENTION_TIME_BUDGET_SECONDS", 20)
	if config.LogDays < 0 || config.LogDays > 3650 {
		return config, errors.New("LOG_RETENTION_DAYS must be between 0 and 3650")
	}
	if config.BatchSize < 1 || config.BatchSize > 5000 {
		return config, errors.New("STORAGE_RETENTION_BATCH_SIZE must be between 1 and 5000")
	}
	if seconds < 1 || seconds > 55 {
		return config, errors.New("STORAGE_RETENTION_TIME_BUDGET_SECONDS must be between 1 and 55")
	}
	config.Budget = time.Duration(seconds) * time.Second
	return config, nil
}

// StartStorageRetentionTask gives maintenance a bounded share of every minute.
// Cursors survive each time budget so protected rows cannot starve later work.
func StartStorageRetentionTask() {
	if !common.IsMasterNode {
		return
	}
	storageRetentionOnce.Do(func() {
		config, err := storageRetentionConfigFromEnv()
		if err != nil {
			logger.LogError(context.Background(), "storage retention disabled: "+err.Error())
			return
		}
		go func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("storage retention started: log_days=%d preconsume_days=7 batch=%d budget=%s", config.LogDays, config.BatchSize, config.Budget))
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			var cursor storageRetentionCursor
			for {
				ctx, cancel := context.WithTimeout(context.Background(), config.Budget)
				preDeleted, logsDeleted, err := runStorageRetentionOnce(ctx, config, &cursor)
				cancel()
				budgetReached := errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
				if err != nil && !budgetReached {
					logger.LogError(context.Background(), "storage retention failed: "+err.Error())
				}
				if preDeleted > 0 || logsDeleted > 0 || budgetReached {
					logger.LogInfo(context.Background(), fmt.Sprintf("storage retention: preconsume_deleted=%d logs_deleted=%d preconsume_cursor=%d log_cursor=%d budget_reached=%t", preDeleted, logsDeleted, cursor.PreConsumeID, cursor.LogID, budgetReached))
				}
				<-ticker.C
			}
		}()
	})
}

func runStorageRetentionOnce(ctx context.Context, config storageRetentionConfig, cursor *storageRetentionCursor) (int64, int64, error) {
	var preDeleted, logsDeleted int64
	preDone := false
	logsDone := config.LogDays == 0
	cutoff := common.GetTimestamp() - int64(config.LogDays)*24*3600
	for {
		if err := ctx.Err(); err != nil {
			return preDeleted, logsDeleted, err
		}
		if !preDone {
			batch, err := model.CleanupSubscriptionPreConsumeRecords(ctx, 7*24*3600, config.BatchSize, cursor.PreConsumeID)
			if err != nil {
				return preDeleted, logsDeleted, fmt.Errorf("pre-consume retention: %w", err)
			}
			preDeleted += batch.Deleted
			cursor.PreConsumeID = batch.LastID
			preDone = batch.Done
			if preDone {
				cursor.PreConsumeID = 0
			}
		}
		if !logsDone {
			batch, err := model.DeleteExpiredUsageLogs(ctx, cutoff, config.BatchSize, cursor.LogID)
			if err != nil {
				return preDeleted, logsDeleted, fmt.Errorf("usage log retention: %w", err)
			}
			logsDeleted += batch.Deleted
			cursor.LogID = batch.LastID
			logsDone = batch.Done
			if logsDone {
				cursor.LogID = 0
			}
		}
		if preDone && logsDone {
			return preDeleted, logsDeleted, nil
		}
	}
}
