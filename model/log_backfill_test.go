package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLogBackfillTestDBs(t *testing.T) {
	t.Helper()

	oldDB := DB
	oldLogDB := LOG_DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	oldLogSQLType := common.LogSqlType

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite

	businessDB := openLogMigrationSQLiteDB(t, "backfill_business")
	logDB := openLogMigrationSQLiteDB(t, "backfill_logs")
	DB = businessDB
	LOG_DB = logDB
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.AutoMigrate(&Log{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		common.LogSqlType = oldLogSQLType
		logDerivedColumnsBackfillBeforeUpdateHook = nil
		closeLogMigrationSQLiteDB(t, businessDB)
		closeLogMigrationSQLiteDB(t, logDB)
	})
}

func TestLogDerivedColumnsBackfillUsesDBCheckpointAndLOGDB(t *testing.T) {
	setupLogBackfillTestDBs(t)

	businessLog := Log{Id: 1, Other: common.MapToJsonStr(map[string]interface{}{
		"subscription_id":              11,
		"subscription_tokens_consumed": 22,
		"billing_source":               "business-db",
		"endpoint":                     "/business",
	})}
	require.NoError(t, DB.Create(&businessLog).Error)

	firstLog := Log{Other: common.MapToJsonStr(map[string]interface{}{
		"subscription_id":              101,
		"subscription_tokens_consumed": 202,
		"billing_source":               "subscription",
		"endpoint":                     "/v1/logdb/first",
	})}
	secondLog := Log{Other: common.MapToJsonStr(map[string]interface{}{
		"subscription_id":              303,
		"subscription_tokens_consumed": 404,
		"billing_source":               "subscription",
		"endpoint":                     "/v1/logdb/second",
	})}
	require.NoError(t, LOG_DB.Create(&firstLog).Error)
	require.NoError(t, LOG_DB.Create(&secondLog).Error)

	processed, complete, err := BackfillLogDerivedColumnsBatch(1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, processed)
	assert.False(t, complete)
	assertBackfillOption(t, LogDerivedColumnsBackfillCheckpoint, "1")
	assertBackfillOptionMissing(t, LogDerivedColumnsBackfillComplete)

	var storedFirst Log
	require.NoError(t, LOG_DB.First(&storedFirst, firstLog.Id).Error)
	require.NotNil(t, storedFirst.SubscriptionID)
	assert.Equal(t, 101, *storedFirst.SubscriptionID)
	require.NotNil(t, storedFirst.SubscriptionTokensConsumed)
	assert.EqualValues(t, 202, *storedFirst.SubscriptionTokensConsumed)
	require.NotNil(t, storedFirst.BillingSource)
	assert.Equal(t, "subscription", *storedFirst.BillingSource)
	require.NotNil(t, storedFirst.Endpoint)
	assert.Equal(t, "/v1/logdb/first", *storedFirst.Endpoint)

	var storedSecondBefore Log
	require.NoError(t, LOG_DB.First(&storedSecondBefore, secondLog.Id).Error)
	assert.Nil(t, storedSecondBefore.SubscriptionID)
	assert.Nil(t, storedSecondBefore.SubscriptionTokensConsumed)
	assert.Nil(t, storedSecondBefore.BillingSource)
	assert.Nil(t, storedSecondBefore.Endpoint)

	var storedBusiness Log
	require.NoError(t, DB.First(&storedBusiness, businessLog.Id).Error)
	assert.Nil(t, storedBusiness.SubscriptionID)
	assert.Nil(t, storedBusiness.SubscriptionTokensConsumed)
	assert.Nil(t, storedBusiness.BillingSource)
	assert.Nil(t, storedBusiness.Endpoint)

	processed, complete, err = BackfillLogDerivedColumnsBatch(10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, processed)
	assert.True(t, complete)
	assertBackfillOption(t, LogDerivedColumnsBackfillCheckpoint, "2")
	assertBackfillOption(t, LogDerivedColumnsBackfillComplete, "true")

	var storedSecond Log
	require.NoError(t, LOG_DB.First(&storedSecond, secondLog.Id).Error)
	require.NotNil(t, storedSecond.SubscriptionID)
	assert.Equal(t, 303, *storedSecond.SubscriptionID)
	require.NotNil(t, storedSecond.SubscriptionTokensConsumed)
	assert.EqualValues(t, 404, *storedSecond.SubscriptionTokensConsumed)
	require.NotNil(t, storedSecond.Endpoint)
	assert.Equal(t, "/v1/logdb/second", *storedSecond.Endpoint)
}

func TestBackfillLogDerivedColumnsBatchPreservesExplicitZero(t *testing.T) {
	setupLogBackfillTestDBs(t)

	log := Log{Other: common.MapToJsonStr(map[string]interface{}{
		"subscription_tokens_consumed": 0,
	})}
	require.NoError(t, LOG_DB.Create(&log).Error)

	processed, complete, err := BackfillLogDerivedColumnsBatch(100)
	require.NoError(t, err)
	assert.EqualValues(t, 1, processed)
	assert.True(t, complete)
	assertBackfillOption(t, LogDerivedColumnsBackfillCheckpoint, "1")

	var stored Log
	require.NoError(t, LOG_DB.First(&stored, log.Id).Error)
	require.NotNil(t, stored.SubscriptionTokensConsumed)
	assert.EqualValues(t, 0, *stored.SubscriptionTokensConsumed)
}

func TestBackfillLogDerivedColumnsBatchAdvancesCursorWithoutOther(t *testing.T) {
	setupLogBackfillTestDBs(t)

	log := Log{Other: "not-json"}
	require.NoError(t, LOG_DB.Create(&log).Error)

	processed, complete, err := BackfillLogDerivedColumnsBatch(100)
	require.NoError(t, err)
	assert.EqualValues(t, 1, processed)
	assert.True(t, complete)
	assertBackfillOption(t, LogDerivedColumnsBackfillCheckpoint, "1")

	var stored Log
	require.NoError(t, LOG_DB.First(&stored, log.Id).Error)
	assert.Nil(t, stored.SubscriptionID)
	assert.Nil(t, stored.SubscriptionTokensConsumed)
	assert.Nil(t, stored.BillingSource)
	assert.Nil(t, stored.Endpoint)
}

func TestBackfillLogDerivedColumnsBatchDoesNotOverwriteExistingDerivedColumns(t *testing.T) {
	setupLogBackfillTestDBs(t)

	existingSubscriptionID := 5
	existingBillingSource := "existing"
	meteredTokens := 42
	log := Log{
		SubscriptionID: &existingSubscriptionID,
		BillingSource:  &existingBillingSource,
		MeteredTokens:  &meteredTokens,
		Other: common.MapToJsonStr(map[string]interface{}{
			"subscription_id":              99,
			"subscription_tokens_consumed": 7,
			"billing_source":               "from-other",
			"endpoint":                     "/from-other",
		}),
	}
	require.NoError(t, LOG_DB.Create(&log).Error)

	processed, complete, err := BackfillLogDerivedColumnsBatch(100)
	require.NoError(t, err)
	assert.EqualValues(t, 1, processed)
	assert.True(t, complete)

	var stored Log
	require.NoError(t, LOG_DB.First(&stored, log.Id).Error)
	require.NotNil(t, stored.SubscriptionID)
	assert.Equal(t, existingSubscriptionID, *stored.SubscriptionID)
	require.NotNil(t, stored.SubscriptionTokensConsumed)
	assert.EqualValues(t, 7, *stored.SubscriptionTokensConsumed)
	require.NotNil(t, stored.BillingSource)
	assert.Equal(t, existingBillingSource, *stored.BillingSource)
	require.NotNil(t, stored.Endpoint)
	assert.Equal(t, "/from-other", *stored.Endpoint)
	require.NotNil(t, stored.MeteredTokens)
	assert.Equal(t, meteredTokens, *stored.MeteredTokens)
}

func TestLogDerivedColumnsBackfillDoesNotAdvanceCheckpointOnUpdateError(t *testing.T) {
	setupLogBackfillTestDBs(t)

	firstLog := Log{Other: common.MapToJsonStr(map[string]interface{}{"endpoint": "/first"})}
	secondLog := Log{Other: common.MapToJsonStr(map[string]interface{}{"endpoint": "/second"})}
	require.NoError(t, LOG_DB.Create(&firstLog).Error)
	require.NoError(t, LOG_DB.Create(&secondLog).Error)

	injectedErr := errors.New("injected update failure")
	logDerivedColumnsBackfillBeforeUpdateHook = func(log *Log) error {
		if log.Id == secondLog.Id {
			return injectedErr
		}
		return nil
	}

	processed, complete, err := BackfillLogDerivedColumnsBatch(2)
	require.ErrorIs(t, err, injectedErr)
	assert.EqualValues(t, 0, processed)
	assert.False(t, complete)
	assertBackfillOptionMissing(t, LogDerivedColumnsBackfillCheckpoint)
	assertBackfillOptionMissing(t, LogDerivedColumnsBackfillComplete)
}

func TestLogDerivedColumnsBackfillGuardsNullColumnsAtWriteTime(t *testing.T) {
	setupLogBackfillTestDBs(t)

	log := Log{Other: common.MapToJsonStr(map[string]interface{}{
		"subscription_id":              99,
		"subscription_tokens_consumed": 88,
		"billing_source":               "from-other",
		"endpoint":                     "/from-other",
	})}
	require.NoError(t, LOG_DB.Create(&log).Error)

	existingSubscriptionID := 5
	existingTokens := int64(6)
	existingBillingSource := "already-written"
	existingEndpoint := "/already-written"
	logDerivedColumnsBackfillBeforeUpdateHook = func(selected *Log) error {
		if selected.Id != log.Id {
			return nil
		}
		return LOG_DB.Model(&Log{}).Where("id = ?", selected.Id).Updates(map[string]interface{}{
			"subscription_id":              existingSubscriptionID,
			"subscription_tokens_consumed": existingTokens,
			"billing_source":               existingBillingSource,
			"endpoint":                     existingEndpoint,
		}).Error
	}

	processed, complete, err := BackfillLogDerivedColumnsBatch(100)
	require.NoError(t, err)
	assert.EqualValues(t, 1, processed)
	assert.True(t, complete)

	var stored Log
	require.NoError(t, LOG_DB.First(&stored, log.Id).Error)
	require.NotNil(t, stored.SubscriptionID)
	assert.Equal(t, existingSubscriptionID, *stored.SubscriptionID)
	require.NotNil(t, stored.SubscriptionTokensConsumed)
	assert.Equal(t, existingTokens, *stored.SubscriptionTokensConsumed)
	require.NotNil(t, stored.BillingSource)
	assert.Equal(t, existingBillingSource, *stored.BillingSource)
	require.NotNil(t, stored.Endpoint)
	assert.Equal(t, existingEndpoint, *stored.Endpoint)
}

func assertBackfillOption(t *testing.T, key string, expected string) {
	t.Helper()
	value, ok := getBackfillOption(t, key)
	require.Truef(t, ok, "expected option %s to exist", key)
	assert.Equal(t, expected, value)
}

func assertBackfillOptionMissing(t *testing.T, key string) {
	t.Helper()
	_, ok := getBackfillOption(t, key)
	assert.Falsef(t, ok, "expected option %s to be missing", key)
}

func getBackfillOption(t *testing.T, key string) (string, bool) {
	t.Helper()
	var option Option
	err := DB.Where(commonKeyCol+" = ?", key).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false
	}
	require.NoError(t, err)
	return option.Value, true
}
