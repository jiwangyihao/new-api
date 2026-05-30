package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type adminOpsModelTestDBs struct {
	DB    *gorm.DB
	LogDB *gorm.DB
}

func setupAdminOpsModelTestDBs(t *testing.T) adminOpsModelTestDBs {
	t.Helper()

	oldDB := DB
	oldLogDB := LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldLogSQLType := common.LogSqlType

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite
	initCol()

	safeName := strings.ReplaceAll(t.Name(), "/", "_")
	businessDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_admin_ops_business?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_admin_ops_logs?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	DB = businessDB
	LOG_DB = logDB
	require.NoError(t, DB.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.LogSqlType = oldLogSQLType
		initCol()
		if sqlDB, err := businessDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := logDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return adminOpsModelTestDBs{DB: businessDB, LogDB: logDB}
}

func TestGetAdminOpsUserConcurrencyLimitsPrefersPlanValues(t *testing.T) {
	setupAdminOpsModelTestDBs(t)
	now := GetDBTimestamp()
	code := "admin-ops-plan"
	require.NoError(t, DB.Create(&User{Id: 7101, Username: "ops-user", Status: common.UserStatusEnabled, AffCode: "ops-user-aff"}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7102, Title: "Ops Plan", Enabled: true, MonthlyTokenLimit: 500, ConcurrencyLimit: 7, QueueCapacity: 9, BusinessCode: &code}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7103, UserId: 7101, PlanId: 7102, Status: "active", StartTime: now - 60, EndTime: now + 3600, TokenLimit: 500, TokenUsed: 0, ConcurrencyLimit: 2, GrantReason: "order"}).Error)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{7101})

	require.NoError(t, err)
	assert.Equal(t, 7, limits[7101].Limit)
	assert.Equal(t, 9, limits[7101].QueueCapacity)
}

func TestGetAdminOpsUserConcurrencyLimitsIncludesPlanAndUsage(t *testing.T) {
	setupAdminOpsModelTestDBs(t)
	now := GetDBTimestamp()
	code := "ops-plus"
	require.NoError(t, DB.Create(&User{Id: 7301, Username: "ops-plus-user", Status: common.UserStatusEnabled, AffCode: "ops-plus-aff"}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 7302, Title: "Ops Plus", Enabled: true, TotalAmount: 1000, MonthlyTokenLimit: 500, ConcurrencyLimit: 8, QueueCapacity: 10, BusinessCode: &code}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7303, UserId: 7301, PlanId: 7302, Status: "active", StartTime: now - 60, EndTime: now + 3600, AmountTotal: 1000, AmountUsed: 250, TokenLimit: 500, TokenUsed: 125, ConcurrencyLimit: 2, GrantReason: "order"}).Error)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{7301})

	require.NoError(t, err)
	limit := limits[7301]
	assert.Equal(t, 7302, limit.PlanID)
	assert.Equal(t, "Ops Plus", limit.PlanTitle)
	assert.Equal(t, code, limit.PlanCode)
	assert.EqualValues(t, 1000, limit.AmountTotal)
	assert.EqualValues(t, 250, limit.AmountUsed)
	assert.EqualValues(t, 500, limit.TokenLimit)
	assert.EqualValues(t, 125, limit.TokenUsed)
}

func TestGetAdminOpsUserConcurrencyLimitsFallsBackToRuntimeDefaultQueueCapacity(t *testing.T) {
	setupAdminOpsModelTestDBs(t)
	now := GetDBTimestamp()
	oldDefaultQueueCapacity := common.SubscriptionConcurrencyQueueCapacity
	common.SubscriptionConcurrencyQueueCapacity = 6
	t.Cleanup(func() { common.SubscriptionConcurrencyQueueCapacity = oldDefaultQueueCapacity })
	require.NoError(t, DB.Create(&User{Id: 7104, Username: "ops-fallback", Status: common.UserStatusEnabled, AffCode: "ops-fallback-aff"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 7105, UserId: 7104, Status: "active", StartTime: now - 60, EndTime: now + 3600, TokenLimit: 100, TokenUsed: 0, ConcurrencyLimit: 2, GrantReason: "order"}).Error)

	limits, err := GetAdminOpsUserConcurrencyLimits([]int{7104})

	require.NoError(t, err)
	assert.Equal(t, 2, limits[7104].Limit)
	assert.Equal(t, 6, limits[7104].QueueCapacity)
}

func TestGetAdminOpsTrafficStatsCountsRequestsErrorsAndTokens(t *testing.T) {
	setupAdminOpsModelTestDBs(t)
	now := time.Now().Unix()
	meteredTokens := 11
	logs := []Log{
		{Id: 1, UserId: 7201, Username: "ops-traffic", CreatedAt: now - 30, Type: LogTypeConsume, ModelName: "gpt-ops", PromptTokens: 3, CompletionTokens: 4},
		{Id: 2, UserId: 7202, Username: "ops-traffic-metered", CreatedAt: now - 20, Type: LogTypeConsume, ModelName: "gpt-ops", PromptTokens: 100, CompletionTokens: 100, MeteredTokens: &meteredTokens},
		{Id: 3, UserId: 7203, Username: "ops-error", CreatedAt: now - 10, Type: LogTypeError, ModelName: "gpt-ops", PromptTokens: 5, CompletionTokens: 6},
		{Id: 4, UserId: 7204, Username: "ops-old", CreatedAt: now - 600, Type: LogTypeConsume, ModelName: "gpt-ops", PromptTokens: 50, CompletionTokens: 50},
	}
	for i := range logs {
		require.NoError(t, LOG_DB.Create(&logs[i]).Error)
	}

	stats, err := GetAdminOpsTrafficStats(now-60, now)

	require.NoError(t, err)
	assert.EqualValues(t, 3, stats.Requests)
	assert.EqualValues(t, 1, stats.Errors)
	assert.EqualValues(t, 29, stats.TotalTokens)
}
