package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupLogAggregationTestDBs(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldDataExportEnabled := common.DataExportEnabled
	oldRedisEnabled := common.RedisEnabled
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	oldLogSQLType := common.LogSqlType
	oldCoalescer := consumeLogCoalescer
	oldDrainTrigger := getLogAggregationDrainTriggerForTest()

	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite
	consumeLogCoalescer = newConsumeLogCoalescer(0)
	setLogAggregationDrainTriggerForTest(nil)
	logAggregationDrainRunning.Store(false)
	logAggregationDrainWakeup.Store(false)
	logAggregationReplayRequested.Store(false)

	businessDB := openLogMigrationSQLiteDB(t, "aggregation_business")
	logDB := openLogMigrationSQLiteDB(t, "aggregation_logs")
	DB = businessDB
	LOG_DB = logDB
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}))
	require.NoError(t, migrateLogSchema(LOG_DB))

	t.Cleanup(func() {
		consumeLogCoalescer.drain()
		DB = oldDB
		LOG_DB = oldLogDB
		common.LogConsumeEnabled = oldLogConsumeEnabled
		common.DataExportEnabled = oldDataExportEnabled
		common.RedisEnabled = oldRedisEnabled
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		common.LogSqlType = oldLogSQLType
		consumeLogCoalescer = oldCoalescer
		setLogAggregationDrainTriggerForTest(oldDrainTrigger)
		logAggregationDrainRunning.Store(false)
		logAggregationDrainWakeup.Store(false)
		logAggregationReplayRequested.Store(false)
		closeLogMigrationSQLiteDB(t, businessDB)
		closeLogMigrationSQLiteDB(t, logDB)
	})
}

func getLogAggregationDrainTriggerForTest() func() {
	logAggregationDrainTriggerMu.Lock()
	defer logAggregationDrainTriggerMu.Unlock()
	return logAggregationDrainTrigger
}

func setLogAggregationDrainTriggerForTest(trigger func()) {
	logAggregationDrainTriggerMu.Lock()
	defer logAggregationDrainTriggerMu.Unlock()
	logAggregationDrainTrigger = trigger
}

func logAggregationIntPtr(value int) *int { return &value }

func logAggregationInt64Ptr(value int64) *int64 { return &value }

type logAggregationEventSelectCounter struct {
	logger.Interface
	eventSelects atomic.Int64
	logSelects   atomic.Int64
}

func (l *logAggregationEventSelectCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	normalized := strings.ToLower(strings.Join(strings.Fields(sql), " "))
	isSelect := strings.HasPrefix(normalized, "select")
	selectsEvents := isSelect && (strings.Contains(normalized, "from `log_aggregation_events`") ||
		strings.Contains(normalized, `from "log_aggregation_events"`) ||
		strings.Contains(normalized, "from log_aggregation_events"))
	if selectsEvents {
		l.eventSelects.Add(1)
	}
	selectsLogs := isSelect && (strings.Contains(normalized, "from `logs`") ||
		strings.Contains(normalized, `from "logs"`) ||
		strings.Contains(normalized, "from logs"))
	if selectsLogs {
		l.logSelects.Add(1)
	}
	if l.Interface != nil {
		l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
	}
}

func waitForLogUsageHourlyAggregation(t *testing.T, bucketStart int64, userID int, tokenID int, channelID int, status string, modelName string) LogUsageHourly {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var usage LogUsageHourly
		err := LOG_DB.Where("bucket_start = ? AND user_id = ? AND token_id = ? AND channel_id = ? AND status = ? AND model_name = ?", bucketStart, userID, tokenID, channelID, status, modelName).First(&usage).Error
		if err == nil {
			return usage
		}
		if time.Now().After(deadline) {
			require.NoError(t, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForLogAggregationDrainAppliedRequestCount(t *testing.T, bucketStart int64, userID int, tokenID int, channelID int, status string, modelName string, wantRequests int64) LogUsageHourly {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var usage LogUsageHourly
	var usageErr error
	var activeEvents int64
	for {
		usage = LogUsageHourly{}
		usageErr = LOG_DB.Where("bucket_start = ? AND user_id = ? AND token_id = ? AND channel_id = ? AND status = ? AND model_name = ?", bucketStart, userID, tokenID, channelID, status, modelName).First(&usage).Error
		require.NoError(t, LOG_DB.Model(&LogAggregationEvent{}).Where("status <> ?", logAggregationEventStatusApplied).Count(&activeEvents).Error)
		if usageErr == nil && usage.RequestCount == wantRequests && activeEvents == 0 {
			return usage
		}
		if time.Now().After(deadline) {
			require.NoError(t, usageErr)
			assert.Equal(t, wantRequests, usage.RequestCount)
			assert.Equal(t, int64(0), activeEvents)
			return usage
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func seedLogAggregationFreeSubscription(t *testing.T, userID int, subscriptionID int, start int64) UserSubscription {
	t.Helper()
	require.NoError(t, DB.FirstOrCreate(&User{Id: userID, Username: fmt.Sprintf("free-user-%d", userID), Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("free-user-%d", userID)}, "id = ?", userID).Error)
	plan := SubscriptionPlan{Id: subscriptionID + 1000, Title: fmt.Sprintf("trial-%d", subscriptionID), Enabled: true, PriceAmount: 0, IsTrial: true, MonthlyTokenLimit: 100000}
	require.NoError(t, DB.Create(&plan).Error)
	sub := UserSubscription{Id: subscriptionID, UserId: userID, PlanId: plan.Id, TokenLimit: plan.MonthlyTokenLimit, GrantReason: "trial_code", StartTime: start, EndTime: start + 24*3600, Status: "active", Source: "trial_code"}
	require.NoError(t, DB.Create(&sub).Error)
	return sub
}

func TestRecordErrorLogAggregationEventQueuedAfterInsert(t *testing.T) {
	setupLogAggregationTestDBs(t)
	require.NoError(t, DB.Create(&User{Id: 1101, Username: "error-aggregation-user", Status: common.UserStatusEnabled}).Error)
	ctx := testRecordConsumeLogContext(t, "error-aggregation-user")

	RecordErrorLog(ctx, 1101, 2201, "gpt-error", "error-token", "failed upstream", 3301, 4, false, "", nil)

	var stored Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", 1101, LogTypeError).First(&stored).Error)
	var eventCount int64
	require.NoError(t, LOG_DB.Model(&LogAggregationEvent{}).Where("log_id = ? AND aggregate_name = ? AND status = ?", stored.Id, logAggregationNameLogUsageHourly, logAggregationEventStatusPending).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)

}
func TestRecordLogAggregationEventQueuedForGenericErrorLog(t *testing.T) {
	setupLogAggregationTestDBs(t)
	require.NoError(t, DB.Create(&User{Id: 1102, Username: "generic-error-user", Status: common.UserStatusEnabled}).Error)

	RecordLog(1102, LogTypeError, "generic error")

	var stored Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", 1102, LogTypeError).First(&stored).Error)
	var eventCount int64
	require.NoError(t, LOG_DB.Model(&LogAggregationEvent{}).Where("log_id = ? AND aggregate_name = ? AND status = ?", stored.Id, logAggregationNameLogUsageHourly, logAggregationEventStatusPending).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestLogAggregationScenarios(t *testing.T) {
	t.Run("log usage event is idempotent", runApplyLogUsageAggregationEventIsIdempotentByLogIDAndAggregateName)
	t.Run("free subscription event is idempotent", runApplyFreeSubscriptionUsageAggregationIsIdempotentByLogIDAndAggregateName)
	t.Run("failed event remains retryable", runFailedAggregationEventRemainsRetryableWithoutDoubleApply)
	t.Run("hourly aggregation matches detail scan at boundaries", runLogUsageHourlyAggregationMatchesDetailScanAtBoundaries)
}

func TestApplyLogUsageAggregationEventIsIdempotentByLogIDAndAggregateName(t *testing.T) {
	runApplyLogUsageAggregationEventIsIdempotentByLogIDAndAggregateName(t)
}

func runApplyLogUsageAggregationEventIsIdempotentByLogIDAndAggregateName(t *testing.T) {
	t.Helper()
	setupLogAggregationTestDBs(t)
	bucketStart := int64(1778716800)
	log := &Log{
		UserId:           101,
		Username:         "usage-idempotent",
		CreatedAt:        bucketStart + 123,
		Type:             LogTypeConsume,
		TokenId:          201,
		ChannelId:        301,
		ModelName:        "gpt-5.5-super-long-model-name",
		Quota:            17,
		PromptTokens:     11,
		CompletionTokens: 13,
		MeteredTokens:    logAggregationIntPtr(0),
	}
	require.NoError(t, LOG_DB.Create(log).Error)
	require.NoError(t, queueLogAggregationEventsForLogs([]*Log{log}))
	require.NoError(t, queueLogAggregationEventsForLogs([]*Log{log}))

	var eventCount int64
	require.NoError(t, LOG_DB.Model(&LogAggregationEvent{}).Where("log_id = ? AND aggregate_name = ?", log.Id, logAggregationNameLogUsageHourly).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)

	require.NoError(t, ApplyPendingLogAggregationEvents(100))
	require.NoError(t, ApplyPendingLogAggregationEvents(100))

	var usage LogUsageHourly
	require.NoError(t, LOG_DB.Where("bucket_start = ? AND user_id = ? AND token_id = ? AND channel_id = ? AND status = ?", bucketStart, log.UserId, log.TokenId, log.ChannelId, "success").First(&usage).Error)
	assert.Equal(t, int64(1), usage.RequestCount)
	assert.Equal(t, int64(17), usage.QuotaSum)
	assert.Equal(t, int64(0), usage.MeteredTokensSum, "explicit metered_tokens=0 must not fall back to prompt+completion")
	assert.Equal(t, int64(11), usage.PromptTokensSum)
	assert.Equal(t, int64(13), usage.CompletionTokensSum)
	assert.Equal(t, log.ModelName, usage.ModelName)
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(log.ModelName))), usage.ModelKeyHash)

	require.NoError(t, LOG_DB.Model(&LogAggregationEvent{}).Where("log_id = ? AND aggregate_name = ?", log.Id, logAggregationNameLogUsageHourly).Where("status = ?", logAggregationEventStatusApplied).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestApplyPendingLogAggregationEventsPrefetchesBatchEvents(t *testing.T) {
	setupLogAggregationTestDBs(t)
	const totalLogs = 4
	const bucketStart = int64(1778716800)
	logs := make([]*Log, 0, totalLogs)
	for i := 0; i < totalLogs; i++ {
		log := &Log{
			UserId:        109,
			Username:      "batch-event-prefetch",
			CreatedAt:     bucketStart + int64(i),
			Type:          LogTypeConsume,
			TokenId:       209,
			ChannelId:     309,
			ModelName:     "gpt-batch-event-prefetch",
			Quota:         1,
			MeteredTokens: logAggregationIntPtr(1),
		}
		require.NoError(t, LOG_DB.Create(log).Error)
		logs = append(logs, log)
	}
	require.NoError(t, queueLogAggregationEventsForLogs(logs))

	counter := &logAggregationEventSelectCounter{Interface: logger.Default.LogMode(logger.Silent)}
	LOG_DB = LOG_DB.Session(&gorm.Session{Logger: counter})
	require.NoError(t, ApplyPendingLogAggregationEvents(totalLogs))
	eventSelects := counter.eventSelects.Load()
	assert.Equal(t, int64(1), eventSelects, "the batch lookup must be the only SELECT from log_aggregation_events")
	assert.Equal(t, int64(1), counter.logSelects.Load(), "the batch must prefetch all referenced logs in one SELECT")

	var usage LogUsageHourly
	require.NoError(t, LOG_DB.Where("bucket_start = ? AND user_id = ? AND token_id = ? AND channel_id = ? AND status = ?", bucketStart, 109, 209, 309, "success").First(&usage).Error)
	assert.Equal(t, int64(totalLogs), usage.RequestCount)
}

func TestApplyLogAggregationEventConcurrentWorkersAggregateOnce(t *testing.T) {
	setupLogAggregationTestDBs(t)
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "log-aggregation-workers.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	concurrentDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := concurrentDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	require.NoError(t, migrateLogSchema(concurrentDB))
	previousLogDB := LOG_DB
	LOG_DB = concurrentDB
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		require.NoError(t, sqlDB.Close())
	})

	const bucketStart = int64(1778716800)
	log := &Log{
		UserId:        110,
		Username:      "concurrent-aggregation-workers",
		CreatedAt:     bucketStart + 42,
		Type:          LogTypeConsume,
		TokenId:       210,
		ChannelId:     310,
		ModelName:     "gpt-concurrent-aggregation-workers",
		Quota:         17,
		MeteredTokens: logAggregationIntPtr(19),
	}
	require.NoError(t, LOG_DB.Create(log).Error)
	require.NoError(t, queueLogAggregationEventsForLogsDB(LOG_DB, []*Log{log}))
	events, pendingCount, err := logAggregationEventsForProcessing(1)
	require.NoError(t, err)
	require.Equal(t, 1, pendingCount)
	require.Len(t, events, 1)
	event := events[0]

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- applyLogAggregationEventByID(event.Id, event.LogID, event.AggregateName)
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	for workerErr := range results {
		require.NoError(t, workerErr)
	}

	var usage LogUsageHourly
	require.NoError(t, LOG_DB.Where("bucket_start = ? AND user_id = ? AND token_id = ? AND channel_id = ? AND status = ?", bucketStart, log.UserId, log.TokenId, log.ChannelId, "success").First(&usage).Error)
	assert.Equal(t, int64(1), usage.RequestCount)
	assert.Equal(t, int64(17), usage.QuotaSum)
	assert.Equal(t, int64(19), usage.MeteredTokensSum)
	var storedEvent LogAggregationEvent
	require.NoError(t, LOG_DB.First(&storedEvent, event.Id).Error)
	assert.Equal(t, logAggregationEventStatusApplied, storedEvent.Status)
}

func BenchmarkApplyLogAggregationEventTransaction(b *testing.B) {
	for _, skipDefaultTransaction := range []bool{false, true} {
		name := "default_callbacks"
		if skipDefaultTransaction {
			name = "skip_default_callbacks"
		}
		b.Run(name, func(b *testing.B) {
			dsn := fmt.Sprintf("file:aggregation-benchmark-%t?mode=memory&cache=shared", skipDefaultTransaction)
			db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(b, err)
			sqlDB, err := db.DB()
			require.NoError(b, err)
			b.Cleanup(func() { _ = sqlDB.Close() })
			require.NoError(b, db.AutoMigrate(&LogAggregationEvent{}))

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				event := LogAggregationEvent{LogID: i + 1, AggregateName: logAggregationNameLogUsageHourly, Status: logAggregationEventStatusPending}
				if err := db.Create(&event).Error; err != nil {
					b.Fatal(err)
				}
				if err := db.Transaction(func(transaction *gorm.DB) error {
					tx := transaction
					if skipDefaultTransaction {
						tx = transaction.Session(&gorm.Session{SkipDefaultTransaction: true})
					}
					return tx.Model(&LogAggregationEvent{}).Where("id = ?", event.Id).Updates(map[string]interface{}{
						"status":     logAggregationEventStatusApplied,
						"updated_at": int64(i),
					}).Error
				}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func TestLogUsageHourlyUpsertPostgresSQLQualifiesConflictColumns(t *testing.T) {
	setupLogAggregationTestDBs(t)
	row := LogUsageHourly{
		BucketStart:         1778716800,
		UserID:              101,
		TokenID:             201,
		ChannelID:           301,
		Status:              "success",
		ModelKeyHash:        fmt.Sprintf("%x", sha256.Sum256([]byte("gpt-pg-upsert"))),
		ModelName:           "gpt-pg-upsert",
		RequestCount:        1,
		QuotaSum:            17,
		MeteredTokensSum:    24,
		PromptTokensSum:     11,
		CompletionTokensSum: 13,
		UpdatedAt:           1778717000,
	}
	stmt := LOG_DB.Session(&gorm.Session{DryRun: true}).Clauses(logUsageHourlyUpsertClause(row)).Create(&row).Statement
	require.NoError(t, stmt.Error)

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=localhost user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
		Conn:                 stmt.ConnPool,
	}), &gorm.Config{DryRun: true})
	require.NoError(t, err)
	pgStmt := db.Clauses(logUsageHourlyUpsertClause(row)).Create(&row).Statement
	require.NoError(t, pgStmt.Error)
	sql := pgStmt.SQL.String()

	for _, column := range []string{"request_count", "quota_sum", "metered_tokens_sum", "prompt_tokens_sum", "completion_tokens_sum"} {
		if strings.Contains(sql, column+"="+column+" +") || strings.Contains(sql, `"`+column+`"="`+column+`" +`) {
			t.Fatalf("PostgreSQL log_usage_hourly upsert must qualify %s, got SQL: %s", column, sql)
		}
		if !strings.Contains(sql, `"log_usage_hourly"."`+column+`"`) {
			t.Fatalf("PostgreSQL log_usage_hourly upsert should qualify %s, got SQL: %s", column, sql)
		}
	}
}

func TestFreeSubscriptionUsageHourlyUpsertPostgresSQLQualifiesConflictColumns(t *testing.T) {
	setupLogAggregationTestDBs(t)
	row := FreeSubscriptionUsageHourly{
		SubscriptionID: 502,
		UserID:         102,
		HourIndex:      1,
		Tokens:         31,
		UpdatedAt:      1778717000,
	}
	stmt := LOG_DB.Session(&gorm.Session{DryRun: true}).Clauses(freeSubscriptionUsageHourlyUpsertClause(row)).Create(&row).Statement
	require.NoError(t, stmt.Error)

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=localhost user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
		Conn:                 stmt.ConnPool,
	}), &gorm.Config{DryRun: true})
	require.NoError(t, err)
	pgStmt := db.Clauses(freeSubscriptionUsageHourlyUpsertClause(row)).Create(&row).Statement
	require.NoError(t, pgStmt.Error)
	sql := pgStmt.SQL.String()

	if strings.Contains(sql, "tokens=tokens +") || strings.Contains(sql, `"tokens"="tokens" +`) {
		t.Fatalf("PostgreSQL free_subscription_usage_hourly upsert must qualify tokens, got SQL: %s", sql)
	}
	if !strings.Contains(sql, `"free_subscription_usage_hourly"."tokens"`) {
		t.Fatalf("PostgreSQL free_subscription_usage_hourly upsert should qualify tokens, got SQL: %s", sql)
	}
}

func TestApplyFreeSubscriptionUsageAggregationIsIdempotentByLogIDAndAggregateName(t *testing.T) {
	runApplyFreeSubscriptionUsageAggregationIsIdempotentByLogIDAndAggregateName(t)
}

func runApplyFreeSubscriptionUsageAggregationIsIdempotentByLogIDAndAggregateName(t *testing.T) {
	t.Helper()
	setupLogAggregationTestDBs(t)
	start := int64(1778716800)
	sub := seedLogAggregationFreeSubscription(t, 102, 502, start)
	log := &Log{
		UserId:                     sub.UserId,
		Username:                   "free-idempotent",
		CreatedAt:                  start + 3700,
		Type:                       LogTypeConsume,
		TokenId:                    202,
		ChannelId:                  302,
		ModelName:                  "gpt-free",
		Quota:                      23,
		PromptTokens:               100,
		CompletionTokens:           200,
		MeteredTokens:              logAggregationIntPtr(0),
		SubscriptionID:             &sub.Id,
		SubscriptionTokensConsumed: logAggregationInt64Ptr(31),
	}
	require.NoError(t, LOG_DB.Create(log).Error)
	require.NoError(t, queueLogAggregationEventsForLogs([]*Log{log}))
	require.NoError(t, queueLogAggregationEventsForLogs([]*Log{log}))

	var eventCount int64
	require.NoError(t, LOG_DB.Model(&LogAggregationEvent{}).Where("log_id = ? AND aggregate_name = ?", log.Id, logAggregationNameFreeSubscriptionUsageHourly).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)

	require.NoError(t, ApplyPendingLogAggregationEvents(100))
	require.NoError(t, ApplyPendingLogAggregationEvents(100))

	var usage FreeSubscriptionUsageHourly
	require.NoError(t, LOG_DB.Where("subscription_id = ? AND hour_index = ?", sub.Id, 1).First(&usage).Error)
	assert.Equal(t, sub.UserId, usage.UserID)
	assert.Equal(t, int64(31), usage.Tokens)
}

func TestApplyFreeSubscriptionUsageAggregationSkipsSoftDeletedUsers(t *testing.T) {
	setupLogAggregationTestDBs(t)
	start := int64(1778716800)
	require.NoError(t, DB.Create(&User{Id: 1202, Username: "deleted-free-user", Status: common.UserStatusEnabled}).Error)
	sub := seedLogAggregationFreeSubscription(t, 1202, 602, start)
	require.NoError(t, DB.Delete(&User{}, 1202).Error)
	log := &Log{
		UserId:                     sub.UserId,
		Username:                   "deleted-free-user",
		CreatedAt:                  start + 120,
		Type:                       LogTypeConsume,
		MeteredTokens:              logAggregationIntPtr(10),
		SubscriptionID:             &sub.Id,
		SubscriptionTokensConsumed: logAggregationInt64Ptr(10),
	}
	require.NoError(t, LOG_DB.Create(log).Error)
	require.NoError(t, queueLogAggregationEventsForLogs([]*Log{log}))

	require.NoError(t, ApplyPendingLogAggregationEvents(100))

	var count int64
	require.NoError(t, LOG_DB.Model(&FreeSubscriptionUsageHourly{}).Where("subscription_id = ?", sub.Id).Count(&count).Error)
	assert.Equal(t, int64(0), count)

}
func TestApplyFreeSubscriptionUsageAggregationUsesLogOtherFallback(t *testing.T) {
	setupLogAggregationTestDBs(t)
	start := int64(1778716800)
	sub := seedLogAggregationFreeSubscription(t, 1302, 702, start)
	log := &Log{
		UserId:        sub.UserId,
		Username:      "fallback-free-user",
		CreatedAt:     start + 1800,
		Type:          LogTypeConsume,
		MeteredTokens: logAggregationIntPtr(10),
		Other: common.MapToJsonStr(map[string]interface{}{
			"subscription_id":              sub.Id,
			"subscription_tokens_consumed": 12,
		}),
	}
	require.NoError(t, LOG_DB.Create(log).Error)
	require.NoError(t, queueLogAggregationEventsForLogs([]*Log{log}))

	require.NoError(t, ApplyPendingLogAggregationEvents(100))

	var usage FreeSubscriptionUsageHourly
	require.NoError(t, LOG_DB.Where("subscription_id = ? AND hour_index = ?", sub.Id, 0).First(&usage).Error)
	assert.Equal(t, int64(12), usage.Tokens)
}

func TestApplyFreeSubscriptionUsageAggregationSkipsInviteTrialPlanWithoutTrialGrantSource(t *testing.T) {
	setupLogAggregationTestDBs(t)
	start := int64(1778716800)
	userID := 1402
	subscriptionID := 802
	require.NoError(t, DB.Create(&User{Id: userID, Username: "invite-trial-plan-admin-source", Status: common.UserStatusEnabled}).Error)
	plan := SubscriptionPlan{Id: 1802, Title: "invite trial marker with paid source", Enabled: true, PriceAmount: 0, IsTrial: false, InviteTrial: true, MonthlyTokenLimit: 100000}
	require.NoError(t, DB.Create(&plan).Error)
	sub := UserSubscription{Id: subscriptionID, UserId: userID, PlanId: plan.Id, TokenLimit: plan.MonthlyTokenLimit, GrantReason: "admin", Source: SubscriptionGrantOrder, StartTime: start, EndTime: start + 24*3600, Status: "active"}
	require.NoError(t, DB.Create(&sub).Error)
	log := &Log{
		UserId:                     userID,
		Username:                   "invite-trial-plan-admin-source",
		CreatedAt:                  start + 3600,
		Type:                       LogTypeConsume,
		TokenId:                    2402,
		ChannelId:                  3402,
		ModelName:                  "gpt-invite-marker",
		MeteredTokens:              logAggregationIntPtr(44),
		SubscriptionID:             &sub.Id,
		SubscriptionTokensConsumed: logAggregationInt64Ptr(44),
	}
	require.NoError(t, LOG_DB.Create(log).Error)
	require.NoError(t, queueLogAggregationEventsForLogs([]*Log{log}))

	require.NoError(t, ApplyPendingLogAggregationEvents(100))

	var count int64
	require.NoError(t, LOG_DB.Model(&FreeSubscriptionUsageHourly{}).Where("subscription_id = ?", sub.Id).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestRecordConsumeLogDrainsLogUsageAggregationAfterQueue(t *testing.T) {
	setupLogAggregationTestDBs(t)
	userID := 1502
	require.NoError(t, DB.Create(&User{Id: userID, Username: "auto-drain-user", Status: common.UserStatusEnabled}).Error)

	drainCalls := 0
	setLogAggregationDrainTriggerForTest(func() {
		drainCalls++
		require.NoError(t, ApplyPendingLogAggregationEvents(100))
	})
	RecordConsumeLog(testRecordConsumeLogContext(t, "auto-drain-user"), userID, RecordConsumeLogParams{
		ChannelId:        3502,
		PromptTokens:     4,
		CompletionTokens: 6,
		ModelName:        "gpt-auto-drain",
		TokenName:        "auto-drain-token",
		Quota:            12,
		Content:          "auto drain aggregation",
		TokenId:          2502,
		UseTimeSeconds:   3,
		Other:            map[string]interface{}{"billing_source": "quota"},
	})

	var stored Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", userID, LogTypeConsume).First(&stored).Error)
	usage := waitForLogUsageHourlyAggregation(t, stored.CreatedAt-stored.CreatedAt%3600, userID, stored.TokenId, stored.ChannelId, "success", stored.ModelName)
	assert.Equal(t, 1, drainCalls)
	assert.Equal(t, int64(1), usage.RequestCount)
	assert.Equal(t, int64(12), usage.QuotaSum)
	assert.Equal(t, int64(10), usage.MeteredTokensSum)
}

func TestDefaultLogAggregationDrainConsumesMoreThanOneBatch(t *testing.T) {
	setupLogAggregationTestDBs(t)
	setLogAggregationDrainTriggerForTest(triggerPendingLogAggregationDrain)
	logAggregationDrainRunning.Store(false)

	const totalLogs = 125
	const userID = 1602
	const tokenID = 2602
	const channelID = 3602
	const bucketStart = int64(1778716800)
	modelName := "gpt-auto-drain-multi-batch"
	logs := make([]*Log, 0, totalLogs)
	for i := 0; i < totalLogs; i++ {
		log := &Log{
			UserId:        userID,
			Username:      "auto-drain-many",
			CreatedAt:     bucketStart + int64(i),
			Type:          LogTypeConsume,
			TokenId:       tokenID,
			ChannelId:     channelID,
			ModelName:     modelName,
			Quota:         1,
			MeteredTokens: logAggregationIntPtr(1),
		}
		require.NoError(t, LOG_DB.Create(log).Error)
		logs = append(logs, log)
	}

	require.NoError(t, queueLogAggregationEventsForLogs(logs))

	usage := waitForLogAggregationDrainAppliedRequestCount(t, bucketStart, userID, tokenID, channelID, "success", modelName, totalLogs)
	assert.Equal(t, int64(totalLogs), usage.QuotaSum)
	assert.Equal(t, int64(totalLogs), usage.MeteredTokensSum)
}

func TestApplyPendingLogAggregationEventsPrioritizesPendingOverFailedRetries(t *testing.T) {
	setupLogAggregationTestDBs(t)
	now := common.GetTimestamp()
	for i := 0; i < 5; i++ {
		require.NoError(t, LOG_DB.Create(&LogAggregationEvent{
			LogID:         9100 + i,
			AggregateName: logAggregationNameLogUsageHourly,
			Status:        logAggregationEventStatusFailed,
			CreatedAt:     now,
			UpdatedAt:     now,
		}).Error)
	}

	log := &Log{
		UserId:        1702,
		Username:      "pending-not-starved",
		CreatedAt:     1778716800 + 240,
		Type:          LogTypeConsume,
		TokenId:       2702,
		ChannelId:     3702,
		ModelName:     "gpt-pending-not-starved",
		Quota:         13,
		MeteredTokens: logAggregationIntPtr(13),
	}
	require.NoError(t, LOG_DB.Create(log).Error)
	require.NoError(t, queueLogAggregationEventsForLogs([]*Log{log}))

	require.NoError(t, ApplyPendingLogAggregationEvents(5))

	var event LogAggregationEvent
	require.NoError(t, LOG_DB.Where("log_id = ? AND aggregate_name = ?", log.Id, logAggregationNameLogUsageHourly).First(&event).Error)
	assert.Equal(t, logAggregationEventStatusApplied, event.Status)
	usage := waitForLogUsageHourlyAggregation(t, log.CreatedAt-log.CreatedAt%3600, log.UserId, log.TokenId, log.ChannelId, "success", log.ModelName)
	assert.Equal(t, int64(1), usage.RequestCount)
}

func TestDefaultLogAggregationDrainReplaysLogMissingOutboxEvent(t *testing.T) {
	setupLogAggregationTestDBs(t)
	setLogAggregationDrainTriggerForTest(triggerPendingLogAggregationDrain)
	logAggregationDrainRunning.Store(false)
	logAggregationDrainWakeup.Store(false)

	bucketStart := int64(1778716800)
	log := &Log{
		UserId:        1802,
		Username:      "missing-outbox",
		CreatedAt:     bucketStart + 123,
		Type:          LogTypeConsume,
		TokenId:       2802,
		ChannelId:     3802,
		ModelName:     "gpt-missing-outbox",
		Quota:         19,
		MeteredTokens: logAggregationIntPtr(19),
	}
	require.NoError(t, LOG_DB.Create(log).Error)

	var eventCount int64
	require.NoError(t, LOG_DB.Model(&LogAggregationEvent{}).Where("log_id = ?", log.Id).Count(&eventCount).Error)
	require.Equal(t, int64(0), eventCount)

	requestMissingLogAggregationReplay()
	usage := waitForLogUsageHourlyAggregation(t, bucketStart, log.UserId, log.TokenId, log.ChannelId, "success", log.ModelName)
	assert.Equal(t, int64(1), usage.RequestCount)
	assert.Equal(t, int64(19), usage.MeteredTokensSum)
	require.NoError(t, LOG_DB.Model(&LogAggregationEvent{}).Where("log_id = ? AND aggregate_name = ? AND status = ?", log.Id, logAggregationNameLogUsageHourly, logAggregationEventStatusApplied).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
}

func TestFailedAggregationEventRemainsRetryableWithoutDoubleApply(t *testing.T) {
	runFailedAggregationEventRemainsRetryableWithoutDoubleApply(t)
}

func runFailedAggregationEventRemainsRetryableWithoutDoubleApply(t *testing.T) {
	t.Helper()
	setupLogAggregationTestDBs(t)
	const delayedLogID = 9001
	require.NoError(t, LOG_DB.Create(&LogAggregationEvent{LogID: delayedLogID, AggregateName: logAggregationNameLogUsageHourly, Status: logAggregationEventStatusPending}).Error)

	require.NoError(t, ApplyPendingLogAggregationEvents(100))
	var event LogAggregationEvent
	require.NoError(t, LOG_DB.Where("log_id = ? AND aggregate_name = ?", delayedLogID, logAggregationNameLogUsageHourly).First(&event).Error)
	assert.Equal(t, logAggregationEventStatusFailed, event.Status)
	assert.NotEmpty(t, event.Error)

	log := &Log{
		Id:               delayedLogID,
		UserId:           103,
		Username:         "retryable",
		CreatedAt:        1778716800 + 222,
		Type:             LogTypeConsume,
		TokenId:          203,
		ChannelId:        303,
		ModelName:        "gpt-retryable",
		Quota:            29,
		PromptTokens:     3,
		CompletionTokens: 4,
	}
	require.NoError(t, LOG_DB.Create(log).Error)

	require.NoError(t, ApplyPendingLogAggregationEvents(100))
	require.NoError(t, ApplyPendingLogAggregationEvents(100))
	require.NoError(t, LOG_DB.Where("log_id = ? AND aggregate_name = ?", delayedLogID, logAggregationNameLogUsageHourly).First(&event).Error)
	assert.Equal(t, logAggregationEventStatusApplied, event.Status)

	var usage LogUsageHourly
	require.NoError(t, LOG_DB.Where("bucket_start = ? AND user_id = ? AND token_id = ? AND channel_id = ? AND status = ?", int64(1778716800), log.UserId, log.TokenId, log.ChannelId, "success").First(&usage).Error)
	assert.Equal(t, int64(1), usage.RequestCount)
	assert.Equal(t, int64(7), usage.MeteredTokensSum)
}

func TestLogUsageHourlyAggregationMatchesDetailScanAtBoundaries(t *testing.T) {
	runLogUsageHourlyAggregationMatchesDetailScanAtBoundaries(t)
}

func runLogUsageHourlyAggregationMatchesDetailScanAtBoundaries(t *testing.T) {
	t.Helper()
	setupLogAggregationTestDBs(t)
	base := int64(1778716800)
	logs := []*Log{
		{UserId: 201, Username: "outside-before", CreatedAt: base + 9, Type: LogTypeConsume, TokenId: 1, ChannelId: 1, ModelName: "gpt-a", Quota: 999, MeteredTokens: logAggregationIntPtr(999)},
		{UserId: 201, Username: "head-start", CreatedAt: base + 10, Type: LogTypeConsume, TokenId: 1, ChannelId: 1, ModelName: "gpt-a", Quota: 5, PromptTokens: 2, CompletionTokens: 3},
		{UserId: 201, Username: "head-end", CreatedAt: base + 3599, Type: LogTypeConsume, TokenId: 1, ChannelId: 1, ModelName: "gpt-a", Quota: 6, PromptTokens: 100, CompletionTokens: 100, MeteredTokens: logAggregationIntPtr(0)},
		{UserId: 201, Username: "full-hour-a", CreatedAt: base + 3600, Type: LogTypeConsume, TokenId: 1, ChannelId: 1, ModelName: "gpt-a", Quota: 7, MeteredTokens: logAggregationIntPtr(7)},
		{UserId: 201, Username: "full-hour-b", CreatedAt: base + 5400, Type: LogTypeConsume, TokenId: 2, ChannelId: 1, ModelName: "gpt-b", Quota: 8, PromptTokens: 4, CompletionTokens: 5},
		{UserId: 202, Username: "full-hour-error", CreatedAt: base + 7200, Type: LogTypeError, TokenId: 1, ChannelId: 2, ModelName: "gpt-a", Quota: 0, PromptTokens: 6, CompletionTokens: 7},
		{UserId: 201, Username: "full-hour-late", CreatedAt: base + 10799, Type: LogTypeConsume, TokenId: 1, ChannelId: 1, ModelName: "gpt-a", Quota: 9, MeteredTokens: logAggregationIntPtr(9)},
		{UserId: 201, Username: "tail-exact-end", CreatedAt: base + 10800, Type: LogTypeConsume, TokenId: 1, ChannelId: 1, ModelName: "gpt-a", Quota: 10, MeteredTokens: logAggregationIntPtr(11)},
		{UserId: 201, Username: "outside-after", CreatedAt: base + 10801, Type: LogTypeConsume, TokenId: 1, ChannelId: 1, ModelName: "gpt-a", Quota: 999, MeteredTokens: logAggregationIntPtr(999)},
	}
	for _, log := range logs {
		require.NoError(t, LOG_DB.Create(log).Error)
	}
	require.NoError(t, queueLogAggregationEventsForLogs(logs))
	require.NoError(t, ApplyPendingLogAggregationEvents(100))

	start := base + 10
	end := base + 10800
	assert.Equal(t, scanLogAggregationDetailForTest(t, start, end), hybridLogAggregationForTest(t, start, end), "complete-hour aggregation plus detail boundaries must equal full detail scan and include created_at == end")
	assert.Empty(t, hybridLogAggregationForTest(t, base-7200, base-7100), "empty ranges should not synthesize aggregation rows")
}

type logAggregationTestKey struct {
	UserID    int
	TokenID   int
	ChannelID int
	ModelName string
	Status    string
}

type logAggregationTestTotals struct {
	Requests   int64
	Quota      int64
	Metered    int64
	Prompt     int64
	Completion int64
}

func scanLogAggregationDetailForTest(t *testing.T, start int64, end int64) map[logAggregationTestKey]logAggregationTestTotals {
	t.Helper()
	return scanLogAggregationDetailRangeForTest(t, start, end, true)
}

func scanLogAggregationDetailRangeForTest(t *testing.T, start int64, end int64, inclusiveEnd bool) map[logAggregationTestKey]logAggregationTestTotals {
	t.Helper()
	if inclusiveEnd && end < start {
		return map[logAggregationTestKey]logAggregationTestTotals{}
	}
	if !inclusiveEnd && end <= start {
		return map[logAggregationTestKey]logAggregationTestTotals{}
	}
	query := LOG_DB.Where("type IN ?", []int{LogTypeConsume, LogTypeError}).Where("created_at >= ?", start)
	if inclusiveEnd {
		query = query.Where("created_at <= ?", end)
	} else {
		query = query.Where("created_at < ?", end)
	}
	var logs []Log
	require.NoError(t, query.Order("id ASC").Find(&logs).Error)
	result := map[logAggregationTestKey]logAggregationTestTotals{}
	for _, log := range logs {
		key := logAggregationDetailKeyForTest(log)
		totals := result[key]
		totals.Requests++
		totals.Quota += int64(log.Quota)
		totals.Metered += logAggregationMeteredTokensForTest(log)
		totals.Prompt += int64(log.PromptTokens)
		totals.Completion += int64(log.CompletionTokens)
		result[key] = totals
	}
	return result
}

func logAggregationDetailKeyForTest(log Log) logAggregationTestKey {
	status := "success"
	if log.Type == LogTypeError {
		status = "error"
	}
	return logAggregationTestKey{UserID: log.UserId, TokenID: log.TokenId, ChannelID: log.ChannelId, ModelName: log.ModelName, Status: status}
}

func logAggregationMeteredTokensForTest(log Log) int64 {
	if log.MeteredTokens != nil {
		return int64(*log.MeteredTokens)
	}
	return int64(log.PromptTokens + log.CompletionTokens)
}

func hybridLogAggregationForTest(t *testing.T, start int64, end int64) map[logAggregationTestKey]logAggregationTestTotals {
	t.Helper()
	if end < start {
		return map[logAggregationTestKey]logAggregationTestTotals{}
	}
	firstFullHour := ((start + 3599) / 3600) * 3600
	lastFullHour := (end / 3600) * 3600
	if firstFullHour >= lastFullHour {
		return scanLogAggregationDetailForTest(t, start, end)
	}
	result := scanLogAggregationDetailRangeForTest(t, start, firstFullHour, false)
	mergeLogAggregationTotalsForTest(result, scanLogAggregationRowsForTest(t, firstFullHour, lastFullHour))
	mergeLogAggregationTotalsForTest(result, scanLogAggregationDetailForTest(t, lastFullHour, end))
	return result
}

func scanLogAggregationRowsForTest(t *testing.T, startBucket int64, endBucket int64) map[logAggregationTestKey]logAggregationTestTotals {
	t.Helper()
	var rows []LogUsageHourly
	require.NoError(t, LOG_DB.Where("bucket_start >= ? AND bucket_start < ?", startBucket, endBucket).Find(&rows).Error)
	result := map[logAggregationTestKey]logAggregationTestTotals{}
	for _, row := range rows {
		key := logAggregationTestKey{UserID: row.UserID, TokenID: row.TokenID, ChannelID: row.ChannelID, ModelName: row.ModelName, Status: row.Status}
		totals := result[key]
		totals.Requests += row.RequestCount
		totals.Quota += row.QuotaSum
		totals.Metered += row.MeteredTokensSum
		totals.Prompt += row.PromptTokensSum
		totals.Completion += row.CompletionTokensSum
		result[key] = totals
	}
	return result
}

func mergeLogAggregationTotalsForTest(dst map[logAggregationTestKey]logAggregationTestTotals, src map[logAggregationTestKey]logAggregationTestTotals) {
	for key, value := range src {
		totals := dst[key]
		totals.Requests += value.Requests
		totals.Quota += value.Quota
		totals.Metered += value.Metered
		totals.Prompt += value.Prompt
		totals.Completion += value.Completion
		dst[key] = totals
	}
}
