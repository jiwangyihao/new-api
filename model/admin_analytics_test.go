package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type adminAnalyticsModelTestDBs struct {
	DB    *gorm.DB
	LogDB *gorm.DB
}

func setupAdminAnalyticsTestDBs(t *testing.T) adminAnalyticsModelTestDBs {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgres := common.UsingPostgreSQL
	oldRedis := common.RedisEnabled
	oldLogSQLType := common.LogSqlType

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.LogSqlType = common.DatabaseTypeSQLite
	common.RedisEnabled = false
	initCol()

	safeName := strings.ReplaceAll(t.Name(), "/", "_")
	businessDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_business?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open("file:"+safeName+"_logs?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	DB = businessDB
	LOG_DB = logDB
	require.NoError(t, DB.AutoMigrate(&User{}, &Token{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &InvitationMonthlyEntitlement{}, &Channel{}))
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
		if sqlDB, err := businessDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := logDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return adminAnalyticsModelTestDBs{DB: businessDB, LogDB: logDB}
}

func TestAdminAnalyticsActiveSubscriptionScopeIsSharedBySnapshotDomains(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	planCode := "basic"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 1, Title: "Basic", Enabled: true, BusinessCode: &planCode}).Error)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "active", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-active"}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "expired", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-expired"}).Error)
	require.NoError(t, DB.Create(&User{Id: 3, Username: "future", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-future"}).Error)
	require.NoError(t, DB.Create(&User{Id: 4, Username: "inactive", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-inactive"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 1, UserId: 1, PlanId: 1, Status: "active", StartTime: now - 10, EndTime: now + 10, TokenLimit: 100, TokenUsed: 10, GrantReason: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 2, UserId: 2, PlanId: 1, Status: "active", StartTime: now - 20, EndTime: now, TokenLimit: 100, TokenUsed: 10, GrantReason: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 3, UserId: 3, PlanId: 1, Status: "active", StartTime: now + 10, EndTime: now + 100, TokenLimit: 100, TokenUsed: 10, GrantReason: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 4, UserId: 4, PlanId: 1, Status: "expired", StartTime: now - 100, EndTime: now + 100, TokenLimit: 100, TokenUsed: 10, GrantReason: "order"}).Error)

	query := AdminAnalyticsQuery{SnapshotAt: now, StartTimestamp: now - 100, EndTimestamp: now, Limit: 20}
	overview, err := GetAdminAnalyticsOverview(query)
	require.NoError(t, err)
	require.Equal(t, 1, overview.Data.Summary.Subscriptions.ActiveCount)
	plans, err := GetAdminAnalyticsPlanDistribution(query)
	require.NoError(t, err)
	require.Len(t, plans.Data.Groups.Items, 1)
	require.Equal(t, 1, plans.Data.Groups.Items[0].SubscriptionCount)
	quota, err := GetAdminAnalyticsQuotaDistribution(query)
	require.NoError(t, err)
	require.Len(t, quota.Data.HighUsageUsers.Items, 1)
}

func TestAdminAnalyticsNormalizesSubscriptionSourceAcrossDomains(t *testing.T) {
	require.Equal(t, dto.AdminAnalyticsSourceAdmin, normalizeAdminSubscriptionSource("admin", "order"))
	require.Equal(t, dto.AdminAnalyticsSourceOrder, normalizeAdminSubscriptionSource("", "order"))
	require.Equal(t, dto.AdminAnalyticsSourceRedemption, normalizeAdminSubscriptionSource("", "redemption"))
	require.Equal(t, dto.AdminAnalyticsSourceRedemption, normalizeAdminSubscriptionSource("redemption", ""))
	require.Equal(t, dto.AdminAnalyticsSourceSystem, normalizeAdminSubscriptionSource("system", ""))
	require.Equal(t, dto.AdminAnalyticsSourceUnknown, normalizeAdminSubscriptionSource("", "mystery"))
}

func TestAdminAnalyticsActiveSubscriptionFiltersUserStatusAndBusinessCode(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	basicCode := "basic"
	proCode := "pro"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 1, Title: "Basic", Enabled: true, BusinessCode: &basicCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 2, Title: "Pro", Enabled: true, BusinessCode: &proCode}).Error)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "enabled", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-enabled"}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "disabled", Status: common.UserStatusDisabled, Group: "default", AffCode: "aff-disabled"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 1, UserId: 1, PlanId: 1, Status: "active", StartTime: now - 60, EndTime: now + 60, TokenLimit: 100, TokenUsed: 10, GrantReason: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 2, UserId: 2, PlanId: 2, Status: "active", StartTime: now - 60, EndTime: now + 60, TokenLimit: 100, TokenUsed: 10, GrantReason: "order"}).Error)

	rows, err := loadAdminActiveSubscriptions(AdminAnalyticsQuery{SnapshotAt: now, Limit: 20, UserStatuses: []int{common.UserStatusEnabled}, BusinessCodes: []string{"basic"}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 1, rows[0].Subscription.UserId)
}

func TestAdminAnalyticsActiveSubscriptionFiltersResetStatus(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 1, Title: "Basic", Enabled: true}).Error)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "due", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-due"}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "not-due", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-not-due"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 1, UserId: 1, PlanId: 1, Status: "active", StartTime: now - 60, EndTime: now + 60, TokenLimit: 100, TokenUsed: 10, GrantReason: "order", NextResetTime: now - 10}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 2, UserId: 2, PlanId: 1, Status: "active", StartTime: now - 60, EndTime: now + 60, TokenLimit: 100, TokenUsed: 10, GrantReason: "order", NextResetTime: now + 10}).Error)

	rows, err := loadAdminActiveSubscriptions(AdminAnalyticsQuery{SnapshotAt: now, Limit: 20, ResetStatuses: []string{"due"}})

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 1, rows[0].Subscription.UserId)
}

func TestAdminAnalyticsQuotaBucketsHandleZeroLimitUnlimitedAndOver100(t *testing.T) {
	trial := classifyAdminSubscriptionQuota(0, 0, dto.AdminAnalyticsSourceTrialCode)
	require.True(t, trial.TokenUnlimited)
	require.Nil(t, trial.UsageRate)
	require.False(t, trial.SystemRisk)
	require.Equal(t, "unlimited_or_invalid", trial.Bucket)

	admin := classifyAdminSubscriptionQuota(0, 0, dto.AdminAnalyticsSourceAdmin)
	require.Equal(t, "zero_limit", admin.Bucket)
	require.True(t, admin.SystemRisk)

	over := classifyAdminSubscriptionQuota(100, 120, dto.AdminAnalyticsSourceOrder)
	require.Equal(t, "over_100", over.Bucket)
	require.NotNil(t, over.RemainingTokens)
	require.Equal(t, int64(0), *over.RemainingTokens)

	invalid := classifyAdminSubscriptionQuota(-1, 0, dto.AdminAnalyticsSourceOrder)
	require.True(t, invalid.SystemRisk)
}

func TestAdminAnalyticsPlanDistributionAggregatesTokenQuotaOnly(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	planCode := "basic"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 10, Title: "Basic", Enabled: true, BusinessCode: &planCode}).Error)
	require.NoError(t, DB.Create(&User{Id: 10, Username: "u10", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-u10"}).Error)
	require.NoError(t, DB.Create(&User{Id: 11, Username: "u11", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-u11"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 10, UserId: 10, PlanId: 10, Status: "active", StartTime: now - 60, EndTime: now + 60, TokenLimit: 100, TokenUsed: 20, GrantReason: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 11, UserId: 11, PlanId: 10, Status: "active", StartTime: now - 60, EndTime: now + 60, TokenLimit: 200, TokenUsed: 30, GrantReason: "order"}).Error)

	res, err := GetAdminAnalyticsPlanDistribution(AdminAnalyticsQuery{SnapshotAt: now, StartTimestamp: now - 100, EndTimestamp: now, Limit: 20})
	require.NoError(t, err)
	require.Len(t, res.Data.Groups.Items, 1)
	group := res.Data.Groups.Items[0]
	require.Equal(t, int64(300), group.TokenLimit)
	require.Equal(t, int64(50), group.TokenUsed)
	require.Equal(t, int64(250), group.RemainingTokens)
}

func TestAdminAnalyticsSeparatesUserGroupsAndRequestGroups(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	require.NoError(t, DB.Create(&User{Id: 21, Username: "vip", Status: common.UserStatusEnabled, Group: "vip", AffCode: "aff-vip"}).Error)
	require.NoError(t, DB.Create(&User{Id: 22, Username: "default", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-default"}).Error)
	res, err := GetAdminAnalyticsUserLifecycle(AdminAnalyticsQuery{Limit: 20})
	require.NoError(t, err)
	require.Len(t, res.Data.UserGroups, 2)
	require.Empty(t, res.Data.RequestGroups)
}

func TestAdminAnalyticsSQLBuilderAvoidsDialectSpecificFunctions(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	query := AdminAnalyticsQuery{Limit: 20}
	_, err := GetAdminAnalyticsOverview(query)
	require.NoError(t, err)
	_, err = GetAdminAnalyticsPlanDistribution(query)
	require.NoError(t, err)
}
