package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLogWriteDerivedTestDBs(t *testing.T) {
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

	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite
	consumeLogCoalescer = newConsumeLogCoalescer(0)

	businessDB := openLogMigrationSQLiteDB(t, "write_derived_business")
	logDB := openLogMigrationSQLiteDB(t, "write_derived_logs")
	DB = businessDB
	LOG_DB = logDB
	require.NoError(t, DB.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))

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
		closeLogMigrationSQLiteDB(t, businessDB)
		closeLogMigrationSQLiteDB(t, logDB)
	})
}

func assertStoredLogDerivedColumns(t *testing.T, log Log, subscriptionID int, subscriptionTokens int64, billingSource string, endpoint string) {
	t.Helper()
	require.NotNil(t, log.SubscriptionID)
	assert.Equal(t, subscriptionID, *log.SubscriptionID)
	require.NotNil(t, log.SubscriptionTokensConsumed)
	assert.Equal(t, subscriptionTokens, *log.SubscriptionTokensConsumed)
	require.NotNil(t, log.BillingSource)
	assert.Equal(t, billingSource, *log.BillingSource)
	require.NotNil(t, log.Endpoint)
	assert.Equal(t, endpoint, *log.Endpoint)
}

func TestRecordConsumeLogFillsDerivedColumnsFromOther(t *testing.T) {
	setupLogWriteDerivedTestDBs(t)
	require.NoError(t, DB.Create(&User{Id: 12001, Username: "record-consume-derived", Status: common.UserStatusEnabled}).Error)

	ctx := testRecordConsumeLogContext(t, "record-consume-derived")
	RecordConsumeLog(ctx, 12001, RecordConsumeLogParams{
		ChannelId:        12002,
		PromptTokens:     11,
		CompletionTokens: 7,
		ModelName:        "gpt-5.5",
		TokenName:        "consume-derived-token",
		Quota:            99,
		Content:          "consume derived write",
		TokenId:          12003,
		UseTimeSeconds:   4,
		IsStream:         true,
		Other: map[string]interface{}{
			"subscription_id":              456,
			"subscription_tokens_consumed": 0,
			"billing_source":               "subscription",
			"request_path":                 "/v1/responses/fallback",
		},
	})
	FlushConsumeLogUpdates()

	var stored Log
	require.NoError(t, LOG_DB.Where("username = ?", "record-consume-derived").First(&stored).Error)
	assertStoredLogDerivedColumns(t, stored, 456, 0, "subscription", "/v1/responses/fallback")
	require.NotNil(t, stored.MeteredTokens)
	assert.Equal(t, 0, *stored.MeteredTokens)
}

func TestRecordTaskBillingLogFillsDerivedColumnsFromOther(t *testing.T) {
	setupLogWriteDerivedTestDBs(t)
	require.NoError(t, DB.Create(&User{Id: 12101, Username: "record-task-derived", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&Token{Id: 12102, UserId: 12101, Name: "task-derived-token", Key: "task-derived-key", Status: common.TokenStatusEnabled}).Error)

	RecordTaskBillingLog(RecordTaskBillingLogParams{
		UserId:    12101,
		LogType:   LogTypeConsume,
		Content:   "task derived write",
		ChannelId: 12103,
		ModelName: "gpt-5.5",
		Quota:     77,
		TokenId:   12102,
		Other: map[string]interface{}{
			"subscription_id":              "789",
			"subscription_tokens_consumed": "0",
			"billing_source":               "subscription",
			"request_path":                 "/v1/task/fallback",
		},
	})

	var stored Log
	require.NoError(t, LOG_DB.Where("user_id = ?", 12101).First(&stored).Error)
	assertStoredLogDerivedColumns(t, stored, 789, 0, "subscription", "/v1/task/fallback")
}

func TestInsertConsumeLogsDirectFillsDerivedColumnsForBatch(t *testing.T) {
	setupLogWriteDerivedTestDBs(t)
	logs := []*Log{
		{
			UserId:   12201,
			Username: "batch-derived-one",
			Type:     LogTypeConsume,
			Other: common.MapToJsonStr(map[string]interface{}{
				"subscription_id":              901,
				"subscription_tokens_consumed": 0,
				"billing_source":               "subscription",
				"request_path":                 "/v1/batch/fallback-one",
			}),
		},
		{
			UserId:   12202,
			Username: "batch-derived-two",
			Type:     LogTypeConsume,
			Other: common.MapToJsonStr(map[string]interface{}{
				"subscription_id":              902,
				"subscription_tokens_consumed": 33,
				"billing_source":               "subscription",
				"endpoint":                     "/v1/batch/endpoint-two",
			}),
		},
	}

	require.NoError(t, insertConsumeLogsDirect(logs))

	var stored []Log
	require.NoError(t, LOG_DB.Where("user_id IN ?", []int{12201, 12202}).Order("user_id ASC").Find(&stored).Error)
	require.Len(t, stored, 2)
	assertStoredLogDerivedColumns(t, stored[0], 901, 0, "subscription", "/v1/batch/fallback-one")
	assertStoredLogDerivedColumns(t, stored[1], 902, 33, "subscription", "/v1/batch/endpoint-two")
}
