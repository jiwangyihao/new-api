package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type usageAnalyticsModelTestDBs struct {
	DB    *gorm.DB
	LogDB *gorm.DB
}

func setupUsageAnalyticsModelTestDBs(t *testing.T) usageAnalyticsModelTestDBs {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	oldRedis := common.RedisEnabled
	oldLogSQLType := common.LogSqlType

	if common.LogSqlType == "" {
		common.LogSqlType = common.DatabaseTypeSQLite
	}
	common.UsingSQLite = common.LogSqlType == common.DatabaseTypeSQLite
	common.UsingMySQL = common.LogSqlType == common.DatabaseTypeMySQL
	common.UsingPostgreSQL = common.LogSqlType == common.DatabaseTypePostgreSQL
	common.RedisEnabled = false
	initCol()

	safeName := strings.ReplaceAll(t.Name(), "/", "_")
	businessDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_business?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_logs?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	DB = businessDB
	LOG_DB = logDB
	require.NoError(t, DB.AutoMigrate(&Token{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		common.RedisEnabled = oldRedis
		common.LogSqlType = oldLogSQLType
		initCol()
		businessSQL, businessErr := businessDB.DB()
		if businessErr == nil {
			_ = businessSQL.Close()
		}
		logSQL, logErr := logDB.DB()
		if logErr == nil {
			_ = logSQL.Close()
		}
	})

	return usageAnalyticsModelTestDBs{DB: businessDB, LogDB: logDB}
}

func intPtrForUsageAnalyticsTest(value int) *int { return &value }

func seedUsageAnalyticsLog(t *testing.T, log *Log) {
	t.Helper()
	require.NoError(t, LOG_DB.Create(log).Error)
}

func seedUsageAnalyticsToken(t *testing.T, token *Token) {
	t.Helper()
	require.NoError(t, DB.Create(token).Error)
}

func usageAnalyticsNow() int64 { return time.Now().Unix() }

func TestUsageAnalyticsSummaryUsesMeteredTokensAndFallback(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 10, PromptTokens: 7, CompletionTokens: 3, MeteredTokens: intPtrForUsageAnalyticsTest(80), UseTime: 1})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 9, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 20, PromptTokens: 5, CompletionTokens: 6, UseTime: 2})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 8, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 30, PromptTokens: 100, CompletionTokens: 200, MeteredTokens: intPtrForUsageAnalyticsTest(0), UseTime: 3})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 7, Type: LogTypeError, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 999, PromptTokens: 999, CompletionTokens: 999, UseTime: 4})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 6, Type: LogTypeConsume, TokenId: 1, TokenName: "key-a", ModelName: "gpt-a", Quota: 40, PromptTokens: -5, CompletionTokens: 2, UseTime: 5})

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Metric: UsageAnalyticsMetricTotalTokens, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 5, res.Total.RequestCount)
	require.Equal(t, 4, res.Total.SuccessCount)
	require.Equal(t, 1, res.Total.ErrorCount)
	require.Equal(t, 100, res.Total.Quota)
	require.Equal(t, 91, res.Total.TotalTokens)
	require.Equal(t, 91, res.Total.MeteredTokens)
	require.Len(t, res.Groups, 1)
	require.Equal(t, 91, res.Groups[0].TotalTokens)
}

func TestUsageAnalyticsGroupsTokenByIDNotName(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 20, Type: LogTypeConsume, TokenId: 7, TokenName: "old-name", Quota: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 7, TokenName: "new-name", Quota: 1, MeteredTokens: intPtrForUsageAnalyticsTest(20)})

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Len(t, res.Groups, 1)
	require.Equal(t, "token:7", res.Groups[0].GroupKey)
	require.Equal(t, 30, res.Groups[0].TotalTokens)
}

func TestUsageAnalyticsTokenSupplementWorksWithSeparatedLogDB(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsToken(t, &Token{Id: 8, UserId: 101, Name: "live-key", Key: "sk-live-1234567890", Status: 1, RemainQuota: 100, Group: "default"})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 8, TokenName: "historical", Quota: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Len(t, res.Groups, 1)
	require.NotNil(t, res.Groups[0].Token)
	require.Equal(t, "live-key", res.Groups[0].Token.Name)
	require.NotNil(t, res.Groups[0].Token.MaskedKey)
}

func TestUsageAnalyticsDeletedTokenDoesNotReturnMaskedKey(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	token := &Token{Id: 9, UserId: 101, Name: "deleted-key", Key: "sk-deleted-1234567890", Status: 1}
	seedUsageAnalyticsToken(t, token)
	require.NoError(t, DB.Delete(token).Error)
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 9, TokenName: "deleted-history", Quota: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Len(t, res.Groups, 1)
	require.NotNil(t, res.Groups[0].Token)
	require.True(t, res.Groups[0].Token.Deleted)
	require.Nil(t, res.Groups[0].Token.MaskedKey)
	require.Equal(t, "deleted-history", res.Groups[0].GroupLabel)
}

func TestUsageAnalyticsFiltersRejectForeignToken(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsToken(t, &Token{Id: 10, UserId: 202, Name: "foreign", Key: "sk-foreign-1234567890"})

	_, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, TokenIDs: []int{10}, Limit: 10})
	require.ErrorIs(t, err, ErrUsageAnalyticsInvalidToken)
}

func TestUsageAnalyticsFiltersAllowDeletedTokenWithOwnHistory(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	token := &Token{Id: 14, UserId: 101, Name: "deleted-filter", Key: "sk-deleted-filter-1234567890"}
	seedUsageAnalyticsToken(t, token)
	require.NoError(t, DB.Delete(token).Error)
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 14, TokenName: "deleted-filter-history", MeteredTokens: intPtrForUsageAnalyticsTest(10)})

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, TokenIDs: []int{14}, Limit: 10})
	require.NoError(t, err)
	require.Len(t, res.Groups, 1)
	require.Equal(t, "token:14", res.Groups[0].GroupKey)
}

func TestUsageAnalyticsGroupsStatusStreamModelAndGroup(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 30, Type: LogTypeConsume, TokenId: 1, ModelName: "gpt-a", Group: "default", IsStream: true, MeteredTokens: intPtrForUsageAnalyticsTest(10), UseTime: 1})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 20, Type: LogTypeError, TokenId: 2, ModelName: "gpt-b", Group: "vip", IsStream: false, UseTime: 2})

	for _, groupBy := range []UsageAnalyticsGroupBy{UsageAnalyticsGroupByStatus, UsageAnalyticsGroupByStream, UsageAnalyticsGroupByModel, UsageAnalyticsGroupByToken} {
		res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: groupBy, Limit: 10})
		require.NoError(t, err)
		require.Len(t, res.Groups, 2)
	}
}

func TestUsageAnalyticsTimeseriesBucketsInGo(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	start := int64(1778716800)
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 10, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(10)})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3700, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(20)})

	res, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: start, EndTimestamp: start + 7200, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, start, res.Points[0].Timestamp)
	require.Equal(t, start+3600, res.Points[1].Timestamp)
}

func TestUsageAnalyticsP95UsesCeilAlgorithm(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	for i, useTime := range []int{1, 2, 3, 4, 5} {
		seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - int64(10-i), Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1), UseTime: useTime})
	}

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 5000, res.Total.P95LatencyMs)
}

func TestUsageAnalyticsBreakdownMergesOther(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	for tokenID := 1; tokenID <= 3; tokenID++ {
		seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - int64(tokenID), Type: LogTypeConsume, TokenId: tokenID, TokenName: "key", MeteredTokens: intPtrForUsageAnalyticsTest(tokenID * 10), Quota: tokenID})
	}

	res, err := GetUsageAnalyticsBreakdown(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Metric: UsageAnalyticsMetricTotalTokens, Limit: 2})
	require.NoError(t, err)
	require.Equal(t, 3, res.TotalGroups)
	require.Len(t, res.Groups, 2)
	require.NotNil(t, res.Other)
	require.Nil(t, res.Other.Drilldown)
	require.Equal(t, 10, res.Other.TotalTokens)
}

func TestUsageAnalyticsTimeseriesUsesGlobalTopNAndOther(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	start := int64(1778716800)
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 10, Type: LogTypeConsume, TokenId: 1, TokenName: "top", MeteredTokens: intPtrForUsageAnalyticsTest(100)})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3700, Type: LogTypeConsume, TokenId: 2, TokenName: "other-a", MeteredTokens: intPtrForUsageAnalyticsTest(20)})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3710, Type: LogTypeConsume, TokenId: 3, TokenName: "other-b", MeteredTokens: intPtrForUsageAnalyticsTest(30)})

	res, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: start, EndTimestamp: start + 7200, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Metric: UsageAnalyticsMetricTotalTokens, Limit: 1})
	require.NoError(t, err)
	keys := make(map[string]bool)
	for _, point := range res.Points {
		keys[point.GroupKey] = true
		if point.GroupKey == "other" {
			require.Nil(t, point.Drilldown)
		}
	}
	require.True(t, keys["token:1"])
	require.True(t, keys["other"])
}

func TestUsageAnalyticsTokenContractAppliesToTimeseriesBreakdownAndTPM(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(0), PromptTokens: 100, CompletionTokens: 100})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 9, Type: LogTypeConsume, TokenId: 1, PromptTokens: 5, CompletionTokens: 6})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 8, Type: LogTypeError, TokenId: 1, PromptTokens: 999, CompletionTokens: 999})

	summary, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 11, summary.Total.TotalTokens)
	require.Equal(t, 11, summary.Total.Tpm)

	timeseries, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 11, timeseries.Points[0].TotalTokens)

	breakdown, err := GetUsageAnalyticsBreakdown(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 11, breakdown.Groups[0].TotalTokens)
}

func TestUsageAnalyticsActiveKeyCountIncludesDeletedHistory(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1)})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 9, Type: LogTypeError, TokenId: 2})

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, res.Total.ActiveKeyCount)
}

func TestUsageAnalyticsRPMAndTPMUseRecentMinute(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 10, Type: LogTypeConsume, TokenId: 1, ModelName: "gpt-a", MeteredTokens: intPtrForUsageAnalyticsTest(10)})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 9, Type: LogTypeError, TokenId: 1, ModelName: "gpt-a"})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 70, Type: LogTypeConsume, TokenId: 1, ModelName: "gpt-a", MeteredTokens: intPtrForUsageAnalyticsTest(999)})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - 8, Type: LogTypeConsume, TokenId: 2, ModelName: "gpt-b", MeteredTokens: intPtrForUsageAnalyticsTest(999)})

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 3600, EndTimestamp: now - 120, GroupBy: UsageAnalyticsGroupByToken, ModelNames: []string{"gpt-a"}, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, res.Total.Rpm)
	require.Equal(t, 10, res.Total.Tpm)
}

func TestUsageAnalyticsTimeseriesP95UsesBucketSamples(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	start := int64(1778716800)
	for _, useTime := range []int{1, 2, 3} {
		seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + int64(useTime), Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1), UseTime: useTime})
	}
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3601, Type: LogTypeError, TokenId: 1, UseTime: 7})
	seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: start + 3602, Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1), UseTime: 8})

	res, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: start, EndTimestamp: start + 7200, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 3000, res.Points[0].P95LatencyMs)
	require.Equal(t, 8000, res.Points[1].P95LatencyMs)
}

func TestUsageAnalyticsCandidateLimit(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	for i := 0; i < 50001; i++ {
		seedUsageAnalyticsLog(t, &Log{UserId: 101, CreatedAt: now - int64(i%60), Type: LogTypeConsume, TokenId: 1, MeteredTokens: intPtrForUsageAnalyticsTest(1)})
	}

	_, err := GetUsageAnalyticsTimeseries(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, Granularity: UsageAnalyticsGranularityHour, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.ErrorIs(t, err, ErrUsageAnalyticsTooManyLogs)
}

func TestUsageAnalyticsSQLAvoidsDatabaseSpecificFunctions(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			sql := buildUsageAnalyticsDryRunSQLForTest(t, dialect, UsageAnalyticsGroupByModel)
			upperSQL := strings.ToUpper(sql)
			forbidden := []string{"DATE_TRUNC", "FROM_UNIXTIME", "STRFTIME", "PERCENTILE_CONT", " OVER ", "->", "JSON_EXTRACT", "GROUP_CONCAT", "IFNULL"}
			for _, fragment := range forbidden {
				require.NotContains(t, upperSQL, strings.ToUpper(fragment))
			}
			requireNoBareGroupColumnForUsageAnalyticsTest(t, sql, dialect)
		})
	}
}

func buildUsageAnalyticsDryRunSQLForTest(t *testing.T, dialect string, groupBy UsageAnalyticsGroupBy) string {
	t.Helper()
	testDBs := setupUsageAnalyticsModelTestDBs(t)
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	oldLogSQLType := common.LogSqlType
	t.Cleanup(func() {
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		common.LogSqlType = oldLogSQLType
		initCol()
	})

	var dialector gorm.Dialector
	switch dialect {
	case "sqlite":
		common.UsingSQLite = true
		common.UsingMySQL = false
		common.UsingPostgreSQL = false
		common.LogSqlType = common.DatabaseTypeSQLite
		dialector = sqlite.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "_dryrun?mode=memory&cache=shared")
	case "mysql":
		common.UsingSQLite = false
		common.UsingMySQL = true
		common.UsingPostgreSQL = false
		common.LogSqlType = common.DatabaseTypeMySQL
		dialector = mysql.New(mysql.Config{DSN: "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local", Conn: testDBs.LogDB.ConnPool, SkipInitializeWithVersion: true})
	case "postgres":
		common.UsingSQLite = false
		common.UsingMySQL = false
		common.UsingPostgreSQL = true
		common.LogSqlType = common.DatabaseTypePostgreSQL
		dialector = postgres.New(postgres.Config{DSN: "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable TimeZone=UTC", Conn: testDBs.LogDB.ConnPool, PreferSimpleProtocol: true})
	default:
		t.Fatalf("unsupported dialect %s", dialect)
	}
	initCol()

	dryRunDB, err := gorm.Open(dialector, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	query := UsageAnalyticsQuery{UserID: 101, StartTimestamp: 1778716800, EndTimestamp: 1779321600, GroupBy: groupBy, Limit: 10}
	groupExpr, ok := usageAnalyticsGroupExpr(groupBy)
	require.True(t, ok)
	stmt := usageAnalyticsBaseLogQuery(dryRunDB, query, true).Select(groupExpr + " AS group_value").Group(groupExpr).Find(&[]struct{ GroupValue string }{}).Statement
	require.NoError(t, stmt.Error)
	return stmt.SQL.String()
}

func requireNoBareGroupColumnForUsageAnalyticsTest(t *testing.T, sql string, dialect string) {
	t.Helper()
	lowerSQL := strings.ToLower(sql)
	require.NotContains(t, lowerSQL, "select group,")
	require.NotContains(t, lowerSQL, "where group =")
	require.NotContains(t, lowerSQL, "group by group")
	require.NotContains(t, sql, `"group"`)
	require.NotContains(t, sql, "`group`")
}

func TestUsageAnalyticsDoesNotReadQuotaDataForTokenDimension(t *testing.T) {
	setupUsageAnalyticsModelTestDBs(t)
	now := usageAnalyticsNow()
	require.NoError(t, DB.AutoMigrate(&QuotaData{}))
	require.NoError(t, DB.Create(&QuotaData{Username: "user-a", ModelName: "fake", Quota: 999, CreatedAt: now}).Error)

	res, err := GetUsageAnalyticsSummary(UsageAnalyticsQuery{UserID: 101, StartTimestamp: now - 60, EndTimestamp: now, GroupBy: UsageAnalyticsGroupByToken, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 0, res.Total.RequestCount)
	require.Empty(t, res.Groups)
}
