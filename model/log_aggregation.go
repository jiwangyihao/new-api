package model

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	logAggregationNameLogUsageHourly              = "log_usage_hourly"
	logAggregationNameFreeSubscriptionUsageHourly = "free_subscription_usage_hourly"

	logAggregationEventStatusPending    = "pending"
	logAggregationEventStatusProcessing = "processing"
	logAggregationEventStatusApplied    = "applied"
	logAggregationEventStatusFailed     = "failed"

	logAggregationEventStatusIDIndex = "idx_log_aggregation_events_status_id"
)

type LogAggregationEvent struct {
	Id            int    `json:"id" gorm:"index:idx_log_aggregation_events_status_id,priority:2"`
	LogID         int    `json:"log_id" gorm:"column:log_id;not null;uniqueIndex:idx_log_agg_event_unique,priority:1"`
	AggregateName string `json:"aggregate_name" gorm:"type:varchar(64);not null;uniqueIndex:idx_log_agg_event_unique,priority:2"`
	Status        string `json:"status" gorm:"type:varchar(16);not null;default:'pending';index;index:idx_log_aggregation_events_status_id,priority:1"`
	Error         string `json:"error" gorm:"type:text"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint;not null;default:0"`
	UpdatedAt     int64  `json:"updated_at" gorm:"bigint;not null;default:0"`
}

func (LogAggregationEvent) TableName() string {
	return "log_aggregation_events"
}

type FreeSubscriptionUsageHourly struct {
	SubscriptionID int   `json:"subscription_id" gorm:"column:subscription_id;not null;uniqueIndex:idx_free_subscription_usage_hourly_unique,priority:1"`
	UserID         int   `json:"user_id" gorm:"column:user_id;not null"`
	HourIndex      int   `json:"hour_index" gorm:"not null;uniqueIndex:idx_free_subscription_usage_hourly_unique,priority:2"`
	Tokens         int64 `json:"tokens" gorm:"bigint;not null;default:0"`
	UpdatedAt      int64 `json:"updated_at" gorm:"bigint;not null;default:0"`
}

func (FreeSubscriptionUsageHourly) TableName() string {
	return "free_subscription_usage_hourly"
}

type LogUsageHourly struct {
	BucketStart         int64  `json:"bucket_start" gorm:"bigint;not null;default:0;uniqueIndex:idx_log_usage_hourly_unique,priority:1"`
	UserID              int    `json:"user_id" gorm:"column:user_id;not null;default:0;uniqueIndex:idx_log_usage_hourly_unique,priority:2"`
	TokenID             int    `json:"token_id" gorm:"column:token_id;not null;default:0;uniqueIndex:idx_log_usage_hourly_unique,priority:3"`
	ChannelID           int    `json:"channel_id" gorm:"column:channel_id;not null;default:0;uniqueIndex:idx_log_usage_hourly_unique,priority:4"`
	Status              string `json:"status" gorm:"type:varchar(16);not null;default:'';uniqueIndex:idx_log_usage_hourly_unique,priority:5"`
	ModelKeyHash        string `json:"model_key_hash" gorm:"type:char(64);not null;default:'';uniqueIndex:idx_log_usage_hourly_unique,priority:6"`
	ModelName           string `json:"model_name" gorm:"type:text;not null"`
	RequestCount        int64  `json:"request_count" gorm:"bigint;not null;default:0"`
	QuotaSum            int64  `json:"quota_sum" gorm:"bigint;not null;default:0"`
	MeteredTokensSum    int64  `json:"metered_tokens_sum" gorm:"bigint;not null;default:0"`
	PromptTokensSum     int64  `json:"prompt_tokens_sum" gorm:"bigint;not null;default:0"`
	CompletionTokensSum int64  `json:"completion_tokens_sum" gorm:"bigint;not null;default:0"`
	UpdatedAt           int64  `json:"updated_at" gorm:"bigint;not null;default:0"`
}

func (LogUsageHourly) TableName() string {
	return "log_usage_hourly"
}
func incrementLogAggregationColumn(table string, name string, delta int64) clause.Expr {
	return gorm.Expr("? + ?", clause.Column{Table: table, Name: name}, delta)
}

func logUsageHourlyUpsertClause(row LogUsageHourly) clause.OnConflict {
	return clause.OnConflict{
		Columns: []clause.Column{
			{Name: "bucket_start"},
			{Name: "user_id"},
			{Name: "token_id"},
			{Name: "channel_id"},
			{Name: "status"},
			{Name: "model_key_hash"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"model_name":            row.ModelName,
			"request_count":         incrementLogAggregationColumn("log_usage_hourly", "request_count", row.RequestCount),
			"quota_sum":             incrementLogAggregationColumn("log_usage_hourly", "quota_sum", row.QuotaSum),
			"metered_tokens_sum":    incrementLogAggregationColumn("log_usage_hourly", "metered_tokens_sum", row.MeteredTokensSum),
			"prompt_tokens_sum":     incrementLogAggregationColumn("log_usage_hourly", "prompt_tokens_sum", row.PromptTokensSum),
			"completion_tokens_sum": incrementLogAggregationColumn("log_usage_hourly", "completion_tokens_sum", row.CompletionTokensSum),
			"updated_at":            row.UpdatedAt,
		}),
	}
}

func freeSubscriptionUsageHourlyUpsertClause(row FreeSubscriptionUsageHourly) clause.OnConflict {
	return clause.OnConflict{
		Columns: []clause.Column{{Name: "subscription_id"}, {Name: "hour_index"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"tokens":     incrementLogAggregationColumn("free_subscription_usage_hourly", "tokens", row.Tokens),
			"updated_at": row.UpdatedAt,
		}),
	}
}

func logAggregationEventsTableReady(db *gorm.DB) bool {
	return db != nil && db.Migrator().HasTable(&LogAggregationEvent{})
}

func queueLogAggregationEventsForLogs(logs []*Log) error {
	if err := queueLogAggregationEventsForLogsDB(LOG_DB, logs); err != nil {
		return err
	}
	triggerLogAggregationDrain()
	return nil
}

func queueLogAggregationEventsForLogsDB(db *gorm.DB, logs []*Log) error {
	if db == nil {
		return errors.New("log database is nil")
	}
	if len(logs) == 0 || !logAggregationEventsTableReady(db) {
		return nil
	}
	now := common.GetTimestamp()
	events := make([]LogAggregationEvent, 0, len(logs)*2)
	for _, log := range logs {
		if log == nil || log.Id <= 0 || (log.Type != LogTypeConsume && log.Type != LogTypeError) {
			continue
		}
		fillLogDerivedFields(log)
		events = append(events, LogAggregationEvent{
			LogID:         log.Id,
			AggregateName: logAggregationNameLogUsageHourly,
			Status:        logAggregationEventStatusPending,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if log.SubscriptionID != nil && *log.SubscriptionID > 0 && log.SubscriptionTokensConsumed != nil && *log.SubscriptionTokensConsumed > 0 {
			events = append(events, LogAggregationEvent{
				LogID:         log.Id,
				AggregateName: logAggregationNameFreeSubscriptionUsageHourly,
				Status:        logAggregationEventStatusPending,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
		}
	}
	if len(events) == 0 {
		return nil
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&events).Error
}

func replayMissingLogAggregationEvents(limit int) (int, error) {
	if LOG_DB == nil {
		return 0, errors.New("log database is nil")
	}
	if !logAggregationEventsTableReady(LOG_DB) {
		return 0, nil
	}
	if limit <= 0 {
		limit = logAggregationDrainBatchLimit
	}
	var logs []Log
	if err := LOG_DB.Model(&Log{}).
		Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
		Where("NOT EXISTS (SELECT 1 FROM log_aggregation_events WHERE log_aggregation_events.log_id = logs.id AND log_aggregation_events.aggregate_name = ?)", logAggregationNameLogUsageHourly).
		Order("id ASC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return 0, err
	}
	if len(logs) == 0 {
		return 0, nil
	}
	logPtrs := make([]*Log, 0, len(logs))
	for i := range logs {
		logPtrs = append(logPtrs, &logs[i])
	}
	if err := queueLogAggregationEventsForLogsDB(LOG_DB, logPtrs); err != nil {
		return 0, err
	}
	return len(logPtrs), nil
}

func ApplyPendingLogAggregationEvents(limit int) error {
	_, _, err := applyPendingLogAggregationEventsBatch(limit)
	return err
}

func applyPendingLogAggregationEventsBatch(limit int) (int, int, error) {
	if LOG_DB == nil {
		return 0, 0, errors.New("log database is nil")
	}
	if limit <= 0 {
		limit = logAggregationDrainBatchLimit
	}
	events, pendingCount, err := logAggregationEventsForProcessing(limit)
	if err != nil {
		return 0, 0, err
	}
	logsByID, err := logsForAggregationEvents(events)
	if err != nil {
		return 0, 0, err
	}
	var firstErr error
	for i := range events {
		event := events[i]
		log := logsByID[event.LogID]
		if err := applyLogAggregationEvent(event.Id, event.LogID, event.AggregateName, log); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return pendingCount, len(events), firstErr
}

func logsForAggregationEvents(events []LogAggregationEvent) (map[int]*Log, error) {
	if len(events) == 0 {
		return nil, nil
	}
	logIDs := make([]int, 0, len(events))
	seen := make(map[int]struct{}, len(events))
	for i := range events {
		if events[i].LogID <= 0 {
			continue
		}
		if _, exists := seen[events[i].LogID]; exists {
			continue
		}
		seen[events[i].LogID] = struct{}{}
		logIDs = append(logIDs, events[i].LogID)
	}
	if len(logIDs) == 0 {
		return nil, nil
	}
	var logs []Log
	if err := LOG_DB.Where("id IN ?", logIDs).Find(&logs).Error; err != nil {
		return nil, err
	}
	logsByID := make(map[int]*Log, len(logs))
	for i := range logs {
		fillLogDerivedFields(&logs[i])
		logsByID[logs[i].Id] = &logs[i]
	}
	return logsByID, nil
}

func logAggregationEventsForProcessing(limit int) ([]LogAggregationEvent, int, error) {
	if limit <= 0 || !logAggregationEventsTableReady(LOG_DB) {
		return nil, 0, nil
	}
	var events []LogAggregationEvent
	if err := LOG_DB.Model(&LogAggregationEvent{}).
		Where("status IN ?", []string{logAggregationEventStatusPending, logAggregationEventStatusFailed}).
		Clauses(clause.OrderBy{Expression: clause.Expr{
			SQL:  "CASE WHEN status = ? THEN 0 ELSE 1 END, id ASC",
			Vars: []interface{}{logAggregationEventStatusPending},
		}}).
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, 0, err
	}
	pendingCount := 0
	for i := range events {
		if events[i].Status == logAggregationEventStatusPending {
			pendingCount++
		}
	}
	return events, pendingCount, nil
}

func applyLogAggregationEventByID(eventID int, logID int, aggregateName string) error {
	return applyLogAggregationEvent(eventID, logID, aggregateName, nil)
}

func applyLogAggregationEvent(eventID int, logID int, aggregateName string, prefetchedLog *Log) error {
	return LOG_DB.Transaction(func(transaction *gorm.DB) error {
		tx := transaction.Session(&gorm.Session{SkipDefaultTransaction: true})
		now := common.GetTimestamp()
		res := tx.Model(&LogAggregationEvent{}).
			Where("id = ? AND status IN ?", eventID, []string{logAggregationEventStatusPending, logAggregationEventStatusFailed}).
			Updates(map[string]interface{}{
				"status":     logAggregationEventStatusProcessing,
				"error":      "",
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return nil
		}

		log := prefetchedLog
		if log == nil || log.Id != logID {
			var loaded Log
			if err := tx.First(&loaded, logID).Error; err != nil {
				return markLogAggregationEventFailedTx(tx, eventID, err)
			}
			fillLogDerivedFields(&loaded)
			log = &loaded
		}

		var err error
		switch aggregateName {
		case logAggregationNameLogUsageHourly:
			err = applyLogUsageHourlyAggregationEventTx(tx, log)
		case logAggregationNameFreeSubscriptionUsageHourly:
			err = applyFreeSubscriptionUsageHourlyAggregationEventTx(tx, log)
		default:
			err = fmt.Errorf("unknown log aggregation %q", aggregateName)
		}
		if err != nil {
			return markLogAggregationEventFailedTx(tx, eventID, err)
		}
		return tx.Model(&LogAggregationEvent{}).
			Where("id = ?", eventID).
			Updates(map[string]interface{}{
				"status":     logAggregationEventStatusApplied,
				"error":      "",
				"updated_at": common.GetTimestamp(),
			}).Error
	})
}

func markLogAggregationEventFailedTx(tx *gorm.DB, eventID int, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return tx.Model(&LogAggregationEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]interface{}{
			"status":     logAggregationEventStatusFailed,
			"error":      message,
			"updated_at": common.GetTimestamp(),
		}).Error
}

func applyLogUsageHourlyAggregationEventTx(tx *gorm.DB, log *Log) error {
	if log == nil || (log.Type != LogTypeConsume && log.Type != LogTypeError) {
		return nil
	}
	bucketStart := log.CreatedAt - log.CreatedAt%3600
	status := "success"
	if log.Type == LogTypeError {
		status = "error"
	}
	meteredTokens := int64(log.PromptTokens + log.CompletionTokens)
	if log.MeteredTokens != nil {
		meteredTokens = int64(*log.MeteredTokens)
	}
	modelHash := sha256.Sum256([]byte(log.ModelName))
	now := common.GetTimestamp()
	row := LogUsageHourly{
		BucketStart:         bucketStart,
		UserID:              log.UserId,
		TokenID:             log.TokenId,
		ChannelID:           log.ChannelId,
		Status:              status,
		ModelKeyHash:        fmt.Sprintf("%x", modelHash),
		ModelName:           log.ModelName,
		RequestCount:        1,
		QuotaSum:            int64(log.Quota),
		MeteredTokensSum:    meteredTokens,
		PromptTokensSum:     int64(log.PromptTokens),
		CompletionTokensSum: int64(log.CompletionTokens),
		UpdatedAt:           now,
	}
	return tx.Clauses(logUsageHourlyUpsertClause(row)).Create(&row).Error
}

type freeSubscriptionAggregationIdentity struct {
	SubscriptionID int
	UserID         int
	PlanID         int
	StartTime      int64
	EndTime        int64
	GrantReason    string
	Source         string
	PriceAmount    float64
	IsTrial        bool
}

func loadFreeSubscriptionAggregationIdentity(db *gorm.DB, subscriptionID int) (freeSubscriptionAggregationIdentity, bool, error) {
	if db == nil {
		return freeSubscriptionAggregationIdentity{}, false, errors.New("business database is nil")
	}
	var identity freeSubscriptionAggregationIdentity
	err := db.Raw(`
		SELECT s.id, s.user_id, s.plan_id, s.start_time, s.end_time,
		       s.grant_reason, s.source, p.price_amount, p.is_trial
		FROM user_subscriptions AS s
		JOIN users AS u ON u.id = s.user_id AND u.deleted_at IS NULL
		JOIN subscription_plans AS p ON p.id = s.plan_id
		WHERE s.id = ?
		LIMIT 1`, subscriptionID).Row().Scan(
		&identity.SubscriptionID,
		&identity.UserID,
		&identity.PlanID,
		&identity.StartTime,
		&identity.EndTime,
		&identity.GrantReason,
		&identity.Source,
		&identity.PriceAmount,
		&identity.IsTrial,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return freeSubscriptionAggregationIdentity{}, false, nil
	}
	return identity, err == nil, err
}

func applyFreeSubscriptionUsageHourlyAggregationEventTx(tx *gorm.DB, log *Log) error {
	if log == nil || log.SubscriptionID == nil || *log.SubscriptionID <= 0 || log.SubscriptionTokensConsumed == nil || *log.SubscriptionTokensConsumed <= 0 {
		return nil
	}
	businessDB := DB
	if businessDB == LOG_DB {
		businessDB = tx
	}
	identity, found, err := loadFreeSubscriptionAggregationIdentity(businessDB, *log.SubscriptionID)
	if err != nil {
		return err
	}
	if !found || identity.UserID != log.UserId || log.CreatedAt < identity.StartTime || (identity.EndTime > 0 && log.CreatedAt >= identity.EndTime) {
		return nil
	}
	grantSource := strings.TrimSpace(identity.GrantReason)
	if grantSource == "" {
		grantSource = strings.TrimSpace(identity.Source)
	}
	if identity.PriceAmount != 0 || grantSource == SubscriptionGrantMonthlyInviteEntitlement {
		return nil
	}
	if !logAggregationTrialSubscriptionSource(identity.GrantReason) && !logAggregationTrialSubscriptionSource(identity.Source) && !identity.IsTrial {
		return nil
	}
	hourIndex := int((log.CreatedAt - identity.StartTime) / 3600)
	if hourIndex < 0 || hourIndex >= 24 {
		return nil
	}
	row := FreeSubscriptionUsageHourly{
		SubscriptionID: identity.SubscriptionID,
		UserID:         identity.UserID,
		HourIndex:      hourIndex,
		Tokens:         *log.SubscriptionTokensConsumed,
		UpdatedAt:      common.GetTimestamp(),
	}
	return tx.Clauses(freeSubscriptionUsageHourlyUpsertClause(row)).Create(&row).Error
}

func logAggregationTrialSubscriptionSource(value string) bool {
	switch strings.TrimSpace(value) {
	case "trial_code", "invite_trial":
		return true
	default:
		return false
	}
}

var logAggregationDrainTriggerMu sync.Mutex

var logAggregationDrainTrigger = triggerPendingLogAggregationDrain

var logAggregationDrainRunning atomic.Bool

var logAggregationDrainWakeup atomic.Bool
var logAggregationReplayRequested atomic.Bool

const (
	logAggregationDrainBatchLimit        = 100
	logAggregationDrainMaxPendingBatches = 1000
)

func triggerLogAggregationDrain() {
	logAggregationDrainTriggerMu.Lock()
	drainTrigger := logAggregationDrainTrigger
	logAggregationDrainTriggerMu.Unlock()
	if drainTrigger != nil {
		drainTrigger()
	}
}

func triggerPendingLogAggregationDrain() {
	logAggregationDrainWakeup.Store(true)
	if !logAggregationDrainRunning.CompareAndSwap(false, true) {
		return
	}
	go runPendingLogAggregationDrain()
}

func requestMissingLogAggregationReplay() {
	logAggregationReplayRequested.Store(true)
	triggerLogAggregationDrain()
}
func runPendingLogAggregationDrain() {
	for {
		logAggregationDrainWakeup.Store(false)
		reachedBatchBoundary := false
		for batch := 0; batch < logAggregationDrainMaxPendingBatches; batch++ {
			replayed := 0
			if logAggregationReplayRequested.Swap(false) {
				var replayErr error
				replayed, replayErr = replayMissingLogAggregationEvents(logAggregationDrainBatchLimit)
				if replayErr != nil {
					common.SysError("failed to replay missing log aggregation events: " + replayErr.Error())
				}
				if replayed >= logAggregationDrainBatchLimit {
					logAggregationReplayRequested.Store(true)
				}
			}
			pendingProcessed, _, applyErr := applyPendingLogAggregationEventsBatch(logAggregationDrainBatchLimit)
			if applyErr != nil {
				common.SysError("failed to apply pending log aggregation events: " + applyErr.Error())
			}
			if pendingProcessed < logAggregationDrainBatchLimit && replayed < logAggregationDrainBatchLimit {
				break
			}
			if batch == logAggregationDrainMaxPendingBatches-1 {
				reachedBatchBoundary = true
			}
		}
		if reachedBatchBoundary {
			common.SysError("log aggregation drain reached batch limit before pending queue was empty")
			logAggregationDrainWakeup.Store(true)
		}
		logAggregationDrainRunning.Store(false)
		if !logAggregationDrainWakeup.Load() && !logAggregationReplayRequested.Load() {
			return
		}
		if !logAggregationDrainRunning.CompareAndSwap(false, true) {
			return
		}
	}
}
