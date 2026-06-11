package model

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var logDerivedColumnNames = []string{
	"subscription_id",
	"subscription_tokens_consumed",
	"billing_source",
	"endpoint",
}

type sqliteLogColumnInfo struct {
	CID        int            `gorm:"column:cid"`
	Name       string         `gorm:"column:name"`
	Type       string         `gorm:"column:type"`
	NotNull    int            `gorm:"column:notnull"`
	DefaultVal sql.NullString `gorm:"column:dflt_value"`
	PK         int            `gorm:"column:pk"`
}

func openLogMigrationSQLiteDB(t *testing.T, suffix string) *gorm.DB {
	t.Helper()
	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s_%s?mode=memory&cache=shared", safeName, suffix)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	return db
}

func closeLogMigrationSQLiteDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func createLegacyLogsTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("CREATE TABLE logs (id integer primary key autoincrement, other text)").Error)
}

func requireLogDerivedColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, column := range logDerivedColumnNames {
		assert.Truef(t, db.Migrator().HasColumn(&Log{}, column), "expected logs.%s to exist", column)
	}
}

func requireNoLogDerivedColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, column := range logDerivedColumnNames {
		assert.Falsef(t, db.Migrator().HasColumn(&Log{}, column), "expected logs.%s to stay absent", column)
	}
}

func logMigrationColumnsByName(t *testing.T, db *gorm.DB) map[string]sqliteLogColumnInfo {
	t.Helper()
	var columns []sqliteLogColumnInfo
	require.NoError(t, db.Raw("PRAGMA table_info(logs)").Scan(&columns).Error)
	byName := make(map[string]sqliteLogColumnInfo, len(columns))
	for _, column := range columns {
		byName[column.Name] = column
	}
	return byName
}

func TestMigrateLogDerivedColumnsUsesLOGDB(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	businessDB := openLogMigrationSQLiteDB(t, "business")
	logDB := openLogMigrationSQLiteDB(t, "logs")
	DB = businessDB
	LOG_DB = logDB
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		closeLogMigrationSQLiteDB(t, businessDB)
		closeLogMigrationSQLiteDB(t, logDB)
	})

	createLegacyLogsTable(t, DB)
	createLegacyLogsTable(t, LOG_DB)
	requireNoLogDerivedColumns(t, DB)
	requireNoLogDerivedColumns(t, LOG_DB)

	require.NoError(t, migrateLogSchema(LOG_DB))

	requireLogDerivedColumns(t, LOG_DB)
	requireNoLogDerivedColumns(t, DB)
}

func TestMigrateDefaultDBRunsLogSchemaWhenLogDBIsPrimaryDB(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	db := openLogMigrationSQLiteDB(t, "primary")
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		closeLogMigrationSQLiteDB(t, db)
	})

	createLegacyLogsTable(t, DB)
	requireNoLogDerivedColumns(t, DB)

	require.NoError(t, migrateLogSchema(DB))

	requireLogDerivedColumns(t, DB)
}

func TestMigrateLogDerivedColumnsAreNullableWithoutDefaults(t *testing.T) {
	db := openLogMigrationSQLiteDB(t, "nullable")
	t.Cleanup(func() { closeLogMigrationSQLiteDB(t, db) })
	createLegacyLogsTable(t, db)

	require.NoError(t, migrateLogSchema(db))

	columns := logMigrationColumnsByName(t, db)
	for _, columnName := range logDerivedColumnNames {
		column, ok := columns[columnName]
		require.Truef(t, ok, "expected logs.%s to exist", columnName)
		assert.Equalf(t, 0, column.NotNull, "logs.%s must remain nullable", columnName)
		assert.Falsef(t, column.DefaultVal.Valid, "logs.%s must not define a default", columnName)
	}
}

func TestMigrateLogSchemaDoesNotCreateLargeLogIndexes(t *testing.T) {
	db := openLogMigrationSQLiteDB(t, "large-indexes")
	t.Cleanup(func() { closeLogMigrationSQLiteDB(t, db) })

	createLegacyLogsTable(t, db)
	require.NoError(t, migrateLogSchema(db))

	var indexes []struct {
		Name string `gorm:"column:name"`
	}
	require.NoError(t, db.Raw("PRAGMA index_list('logs')").Scan(&indexes).Error)
	indexNames := make(map[string]struct{}, len(indexes))
	for _, index := range indexes {
		indexNames[index.Name] = struct{}{}
	}
	for _, name := range []string{
		"idx_created_at_id",
		"idx_user_id_id",
		"idx_created_at_type",
		"index_username_model_name",
		"idx_logs_request_id",
		"idx_logs_upstream_request_id",
		"idx_logs_user_created_id",
		"idx_logs_user_type_created_id",
		"idx_logs_type_created_id",
		"idx_logs_subscription_created",
	} {
		assert.NotContainsf(t, indexNames, name, "startup log migration must not create large logs index %s", name)
	}

	logType := reflect.TypeOf(Log{})
	subscriptionID, ok := logType.FieldByName("SubscriptionID")
	require.True(t, ok, "Log.SubscriptionID field must exist")
	assert.Contains(t, strings.ToLower(subscriptionID.Tag.Get("gorm")), "type:integer", "Log.SubscriptionID gorm tag must match manual DDL type")

	for _, fieldName := range []string{"SubscriptionID", "SubscriptionTokensConsumed", "BillingSource", "Endpoint"} {
		field, ok := logType.FieldByName(fieldName)
		require.Truef(t, ok, "Log.%s field must exist", fieldName)
		gormTag := strings.ToLower(field.Tag.Get("gorm"))
		assert.NotContainsf(t, gormTag, "index", "Log.%s gorm tag must not create startup indexes", fieldName)
		assert.NotContainsf(t, gormTag, "default", "Log.%s gorm tag must not create defaults", fieldName)
		assert.NotContainsf(t, gormTag, "not null", "Log.%s gorm tag must keep the column nullable", fieldName)
		assert.NotContainsf(t, strings.ReplaceAll(gormTag, " ", ""), "notnull", "Log.%s gorm tag must keep the column nullable", fieldName)
	}
}

func TestMigrateLogSchemaAddsManualLogColumnsWithoutIndexesAndAllowsWrites(t *testing.T) {
	db := openLogMigrationSQLiteDB(t, "manual-columns")
	t.Cleanup(func() { closeLogMigrationSQLiteDB(t, db) })

	createLegacyLogsTable(t, db)
	require.NoError(t, migrateLogSchema(db))

	columns := logMigrationColumnsByName(t, db)
	for _, columnName := range []string{
		"user_id",
		"created_at",
		"type",
		"content",
		"username",
		"token_name",
		"model_name",
		"quota",
		"prompt_tokens",
		"completion_tokens",
		"metered_tokens",
		"use_time",
		"is_stream",
		"channel_id",
		"token_id",
		"group",
		"ip",
		"request_id",
		"upstream_request_id",
		"other",
	} {
		column, ok := columns[columnName]
		require.Truef(t, ok, "expected logs.%s to exist", columnName)
		assert.Equalf(t, 0, column.NotNull, "logs.%s must remain nullable", columnName)
	}

	var indexes []struct {
		Name string `gorm:"column:name"`
	}
	require.NoError(t, db.Raw("PRAGMA index_list('logs')").Scan(&indexes).Error)
	indexNames := make(map[string]struct{}, len(indexes))
	for _, index := range indexes {
		indexNames[index.Name] = struct{}{}
	}
	for _, name := range []string{
		"idx_created_at_id",
		"idx_user_id_id",
		"idx_created_at_type",
		"index_username_model_name",
		"idx_logs_request_id",
		"idx_logs_upstream_request_id",
	} {
		assert.NotContainsf(t, indexNames, name, "manual legacy logs migration must not create index %s", name)
	}

	meteredTokens := 42
	log := &Log{
		UserId:            1802,
		CreatedAt:         1778716800,
		Type:              LogTypeConsume,
		Content:           "manual column write",
		Username:          "manual-column-user",
		TokenName:         "manual-column-token",
		ModelName:         "gpt-manual-column",
		Quota:             10,
		PromptTokens:      11,
		CompletionTokens:  12,
		MeteredTokens:     &meteredTokens,
		UseTime:           13,
		IsStream:          true,
		ChannelId:         2802,
		TokenId:           3802,
		Group:             "legacy-group",
		Ip:                "127.0.0.1",
		RequestId:         "req-manual-column",
		UpstreamRequestId: "upstream-manual-column",
		Other:             "{}",
	}
	require.NoError(t, db.Create(log).Error)

	var stored Log
	require.NoError(t, db.First(&stored, log.Id).Error)
	require.NotNil(t, stored.MeteredTokens)
	assert.Equal(t, meteredTokens, *stored.MeteredTokens)
	assert.Equal(t, log.RequestId, stored.RequestId)
	assert.Equal(t, log.UpstreamRequestId, stored.UpstreamRequestId)
}
func TestFillLogDerivedFieldsFromOther(t *testing.T) {
	log := &Log{Other: common.MapToJsonStr(map[string]interface{}{
		"subscription_id":              float64(123),
		"subscription_tokens_consumed": "456",
		"billing_source":               "subscription",
		"request_path":                 "/v1/responses",
	})}
	fillLogDerivedFields(log)

	require.NotNil(t, log.SubscriptionID)
	assert.Equal(t, 123, *log.SubscriptionID)
	require.NotNil(t, log.SubscriptionTokensConsumed)
	assert.EqualValues(t, 456, *log.SubscriptionTokensConsumed)
	require.NotNil(t, log.BillingSource)
	assert.Equal(t, "subscription", *log.BillingSource)
	require.NotNil(t, log.Endpoint)
	assert.Equal(t, "/v1/responses", *log.Endpoint)

	jsonNumberLog := &Log{Other: `{"subscription_id":789,"subscription_tokens_consumed":1234,"billing_source":"subscription","endpoint":"/v1/chat/completions"}`}
	fillLogDerivedFields(jsonNumberLog)
	require.NotNil(t, jsonNumberLog.SubscriptionID)
	assert.Equal(t, 789, *jsonNumberLog.SubscriptionID)
	require.NotNil(t, jsonNumberLog.SubscriptionTokensConsumed)
	assert.EqualValues(t, 1234, *jsonNumberLog.SubscriptionTokensConsumed)
	require.NotNil(t, jsonNumberLog.Endpoint)
	assert.Equal(t, "/v1/chat/completions", *jsonNumberLog.Endpoint)

	db := openLogMigrationSQLiteDB(t, "fill")
	t.Cleanup(func() { closeLogMigrationSQLiteDB(t, db) })
	require.NoError(t, migrateLogSchema(db))

	oldLog := Log{Other: common.MapToJsonStr(map[string]interface{}{"request_path": "/legacy"})}
	require.NoError(t, db.Create(&oldLog).Error)

	var stored Log
	require.NoError(t, db.First(&stored, oldLog.Id).Error)
	assert.Nil(t, stored.SubscriptionID)
	assert.Nil(t, stored.SubscriptionTokensConsumed)
	assert.Nil(t, stored.BillingSource)
	assert.Nil(t, stored.Endpoint)
}

func TestFillLogDerivedFieldsPreservesExplicitDerivedColumns(t *testing.T) {
	subscriptionID := 321
	tokensConsumed := int64(654)
	billingSource := "explicit"
	endpoint := "/explicit"
	log := &Log{
		SubscriptionID:             &subscriptionID,
		SubscriptionTokensConsumed: &tokensConsumed,
		BillingSource:              &billingSource,
		Endpoint:                   &endpoint,
		Other: common.MapToJsonStr(map[string]interface{}{
			"subscription_id":              123,
			"subscription_tokens_consumed": 456,
			"billing_source":               "other",
			"endpoint":                     "/other",
			"request_path":                 "/fallback",
		}),
	}

	fillLogDerivedFields(log)

	require.NotNil(t, log.SubscriptionID)
	assert.Equal(t, 321, *log.SubscriptionID)
	require.NotNil(t, log.SubscriptionTokensConsumed)
	assert.EqualValues(t, 654, *log.SubscriptionTokensConsumed)
	require.NotNil(t, log.BillingSource)
	assert.Equal(t, "explicit", *log.BillingSource)
	require.NotNil(t, log.Endpoint)
	assert.Equal(t, "/explicit", *log.Endpoint)
}
