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
	require.NoError(t, DB.AutoMigrate(&User{}, &Token{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &InvitationMonthlyEntitlement{}, &InvitationRewardEvent{}, &Channel{}))
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

func TestPaginateAdminAnalyticsListNoLimitReturnsAllItems(t *testing.T) {
	items := []int{1, 2, 3, 4}

	paged, page := paginateAdminAnalyticsList(items, AdminAnalyticsNoLimit, 0)

	require.Equal(t, items, paged)
	require.Equal(t, 4, page.Limit)
	require.Equal(t, 4, page.Total)
	require.False(t, page.HasMore)
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

func TestAdminAnalyticsActiveScopeSeparatesTimedHistoryFromCreditBalance(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC).Unix()
	timedCode := "analytics-timed-boundary"
	creditCode := "analytics-credit-boundary"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 11, Title: "Timed", Enabled: true, EntitlementType: SubscriptionEntitlementTimed, BusinessCode: &timedCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 12, Title: "Credit balance", Enabled: true, EntitlementType: SubscriptionEntitlementCreditBalance, BusinessCode: &creditCode}).Error)

	tests := []struct {
		name            string
		status          string
		entitlementType string
		startTime       int64
		endTime         int64
		tokenUsed       int64
		wantActive      bool
	}{
		{name: "active timed entitlement", status: SubscriptionStatusActive, entitlementType: SubscriptionEntitlementTimed, startTime: snapshot - 60, endTime: snapshot + 60, wantActive: true},
		{name: "timed entitlement ended at snapshot", status: SubscriptionStatusActive, entitlementType: SubscriptionEntitlementTimed, startTime: snapshot - 60, endTime: snapshot},
		{name: "expired entitlement inside conversion grace", status: SubscriptionStatusExpired, entitlementType: SubscriptionEntitlementTimed, startTime: snapshot - 60, endTime: snapshot - TimedSubscriptionConversionGraceSeconds + 1},
		{name: "expired entitlement at conversion grace boundary", status: SubscriptionStatusExpired, entitlementType: SubscriptionEntitlementTimed, startTime: snapshot - 60, endTime: snapshot - TimedSubscriptionConversionGraceSeconds},
		{name: "expired entitlement outside conversion grace", status: SubscriptionStatusExpired, entitlementType: SubscriptionEntitlementTimed, startTime: snapshot - 60, endTime: snapshot - TimedSubscriptionConversionGraceSeconds - 1},
		{name: "converted source remains history", status: SubscriptionStatusConverted, entitlementType: SubscriptionEntitlementTimed, startTime: snapshot - 60, endTime: snapshot + 60},
		{name: "credit balance has no expiry", status: SubscriptionStatusActive, entitlementType: SubscriptionEntitlementCreditBalance, startTime: snapshot - 60, endTime: 0, wantActive: true},
		{name: "exhausted credit balance is not operationally active", status: SubscriptionStatusActive, entitlementType: SubscriptionEntitlementCreditBalance, startTime: snapshot - 60, endTime: 0, tokenUsed: 100},
		{name: "credit settlement debt is not operationally active", status: SubscriptionStatusActive, entitlementType: SubscriptionEntitlementCreditBalance, startTime: snapshot - 60, endTime: 0, tokenUsed: 120},
		{name: "future credit balance is not active yet", status: SubscriptionStatusActive, entitlementType: SubscriptionEntitlementCreditBalance, startTime: snapshot + 1, endTime: 0},
	}

	wantIDs := make(map[int]struct{})
	for index, test := range tests {
		userID := 100 + index
		subscriptionID := 200 + index
		planID := 11
		if test.entitlementType == SubscriptionEntitlementCreditBalance {
			planID = 12
		}
		require.NoError(t, DB.Create(&User{Id: userID, Username: test.name, Status: common.UserStatusEnabled, Group: "default", AffCode: "analytics-boundary-" + string(rune('a'+index))}).Error)
		require.NoError(t, DB.Create(&UserSubscription{Id: subscriptionID, UserId: userID, PlanId: planID, EntitlementType: test.entitlementType, Status: test.status, StartTime: test.startTime, EndTime: test.endTime, TokenLimit: 100, TokenUsed: test.tokenUsed, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
		if test.wantActive {
			wantIDs[subscriptionID] = struct{}{}
		}
	}

	rows, err := loadAdminActiveSubscriptions(AdminAnalyticsQuery{SnapshotAt: snapshot, Limit: 20})
	require.NoError(t, err)
	require.Len(t, rows, len(wantIDs))
	for _, row := range rows {
		_, ok := wantIDs[row.Subscription.Id]
		require.Truef(t, ok, "subscription %d must not cross the active/history boundary", row.Subscription.Id)
	}

	historyOverview, err := GetAdminAnalyticsOverview(AdminAnalyticsQuery{
		SnapshotAt:           snapshot,
		Limit:                20,
		SubscriptionStatuses: []string{SubscriptionStatusConverted, SubscriptionStatusExpired},
	})
	require.NoError(t, err)
	require.Zero(t, historyOverview.Data.Summary.Subscriptions.ActiveCount, "history status filters must not broaden the operational active scope")

	activeOverview, err := GetAdminAnalyticsOverview(AdminAnalyticsQuery{
		SnapshotAt:           snapshot,
		Limit:                20,
		SubscriptionStatuses: []string{SubscriptionStatusActive},
	})
	require.NoError(t, err)
	require.Equal(t, len(wantIDs), activeOverview.Data.Summary.Subscriptions.ActiveCount)
}

func TestAdminAnalyticsConversionTargetRequiresCompleteOwnershipChain(t *testing.T) {
	tests := []struct {
		name                      string
		sourcePlanID              int
		sourceConvertedTargetID   int
		conversionSourceID        int
		conversionSourcePlanID    int
		conversionTargetID        int
		conversionTargetPlanID    int
		targetUserID              int
		targetPlanID              int
		targetPlanEntitlementType string
		createTargetPlan          bool
	}{
		{name: "conversion source mismatch", sourcePlanID: 71, sourceConvertedTargetID: 712, conversionSourceID: 999, conversionSourcePlanID: 71, conversionTargetID: 712, conversionTargetPlanID: 72, targetUserID: 711, targetPlanID: 72, targetPlanEntitlementType: SubscriptionEntitlementCreditBalance, createTargetPlan: true},
		{name: "conversion source plan mismatch", sourcePlanID: 71, sourceConvertedTargetID: 712, conversionSourceID: 711, conversionSourcePlanID: 999, conversionTargetID: 712, conversionTargetPlanID: 72, targetUserID: 711, targetPlanID: 72, targetPlanEntitlementType: SubscriptionEntitlementCreditBalance, createTargetPlan: true},
		{name: "source target identity missing", sourcePlanID: 71, sourceConvertedTargetID: 0, conversionSourceID: 711, conversionSourcePlanID: 71, conversionTargetID: 712, conversionTargetPlanID: 72, targetUserID: 711, targetPlanID: 72, targetPlanEntitlementType: SubscriptionEntitlementCreditBalance, createTargetPlan: true},
		{name: "target belongs to another user", sourcePlanID: 71, sourceConvertedTargetID: 712, conversionSourceID: 711, conversionSourcePlanID: 71, conversionTargetID: 712, conversionTargetPlanID: 72, targetUserID: 713, targetPlanID: 72, targetPlanEntitlementType: SubscriptionEntitlementCreditBalance, createTargetPlan: true},
		{name: "target plan identity mismatch", sourcePlanID: 71, sourceConvertedTargetID: 712, conversionSourceID: 711, conversionSourcePlanID: 71, conversionTargetID: 712, conversionTargetPlanID: 999, targetUserID: 711, targetPlanID: 72, targetPlanEntitlementType: SubscriptionEntitlementCreditBalance, createTargetPlan: true},
		{name: "target plan is not Credit", sourcePlanID: 71, sourceConvertedTargetID: 712, conversionSourceID: 711, conversionSourcePlanID: 71, conversionTargetID: 712, conversionTargetPlanID: 72, targetUserID: 711, targetPlanID: 72, targetPlanEntitlementType: SubscriptionEntitlementTimed, createTargetPlan: true},
		{name: "target plan is missing", sourcePlanID: 71, sourceConvertedTargetID: 712, conversionSourceID: 711, conversionSourcePlanID: 71, conversionTargetID: 712, conversionTargetPlanID: 72, targetUserID: 711, targetPlanID: 72, targetPlanEntitlementType: SubscriptionEntitlementCreditBalance},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupAdminAnalyticsTestDBs(t)
			require.NoError(t, DB.AutoMigrate(&SubscriptionConversion{}))
			snapshot := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC).Unix()
			sourceCode := "analytics-chain-source"
			targetCode := "analytics-chain-target"
			require.NoError(t, DB.Create(&SubscriptionPlan{Id: 71, Title: "Timed source", Enabled: true, EntitlementType: SubscriptionEntitlementTimed, BusinessCode: &sourceCode}).Error)
			if test.createTargetPlan {
				require.NoError(t, DB.Create(&SubscriptionPlan{Id: 72, Title: "Target plan", Enabled: true, EntitlementType: test.targetPlanEntitlementType, BusinessCode: &targetCode}).Error)
			}
			require.NoError(t, DB.Create(&User{Id: 711, Username: "chain-source", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-chain-source"}).Error)
			if test.targetUserID != 711 {
				require.NoError(t, DB.Create(&User{Id: test.targetUserID, Username: "chain-target", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-chain-target"}).Error)
			}
			require.NoError(t, DB.Create(&UserSubscription{Id: 711, UserId: 711, PlanId: test.sourcePlanID, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusConverted, StartTime: snapshot - 3600, EndTime: snapshot + 3600, TokenLimit: 100, TokenUsed: 20, ConversionId: 811, ConvertedToSubscriptionId: test.sourceConvertedTargetID, ConvertedAt: snapshot - 30, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
			require.NoError(t, DB.Create(&UserSubscription{Id: 712, UserId: test.targetUserID, PlanId: test.targetPlanID, EntitlementType: SubscriptionEntitlementCreditBalance, Status: SubscriptionStatusActive, StartTime: snapshot - 30, TokenLimit: 100, TokenUsed: 25, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
			require.NoError(t, DB.Create(&SubscriptionConversion{Id: 811, UserId: 711, IdempotencyKey: "analytics-chain", SourceSubscriptionId: test.conversionSourceID, SourcePlanId: test.conversionSourcePlanID, SourcePlanTitle: "Timed source", TargetSubscriptionId: test.conversionTargetID, TargetPlanId: test.conversionTargetPlanID, LedgerId: 911, SourceStatus: SubscriptionStatusActive, GrantSource: SubscriptionGrantOrder, DatabaseNow: snapshot - 30, SourceStartTime: snapshot - 3600, SourceEndTime: snapshot + 3600, ConvertedAt: snapshot - 30, CreatedAt: snapshot - 30}).Error)

			response, err := GetAdminAnalyticsDrilldownSubscriptions(AdminAnalyticsQuery{SnapshotAt: snapshot, Limit: 20, UserIDs: []int{711}, SubscriptionStatuses: []string{SubscriptionStatusConverted}}, AdminAnalyticsDrilldownFilter{})

			require.NoError(t, err)
			require.Len(t, response.Data.Subscriptions.Items, 1)
			item := response.Data.Subscriptions.Items[0]
			require.Equal(t, 711, item.SubscriptionID)
			require.Zero(t, item.TargetSubscriptionID)
			require.Zero(t, item.TargetUserID)
			require.Zero(t, item.TargetPlanID)
			require.Empty(t, item.TargetPlanTitle)
		})
	}
}
func TestAdminAnalyticsNormalizesSubscriptionSourceAcrossDomains(t *testing.T) {
	require.Equal(t, dto.AdminAnalyticsSourceAdmin, normalizeAdminSubscriptionSource("admin", "order"))
	require.Equal(t, dto.AdminAnalyticsSourceOrder, normalizeAdminSubscriptionSource("", "order"))
	require.Equal(t, dto.AdminAnalyticsSourceRedemption, normalizeAdminSubscriptionSource("", "redemption"))
	require.Equal(t, dto.AdminAnalyticsSourceRedemption, normalizeAdminSubscriptionSource("redemption", ""))
	require.Equal(t, dto.AdminAnalyticsSourceSystem, normalizeAdminSubscriptionSource("system", ""))
	require.Equal(t, dto.AdminAnalyticsSourceUnknown, normalizeAdminSubscriptionSource("", "mystery"))
}

func TestAdminPaidSubscriptionAnalyticsQueryNormalizationPreservesSnapshotAndAllHistoryRange(t *testing.T) {
	snapshot := normalizeAdminPaidSubscriptionAnalyticsQuery(AdminAnalyticsQuery{RangeMode: AdminAnalyticsRangeModeSnapshot, SnapshotAt: 0})
	require.Equal(t, int64(0), snapshot.StartTimestamp)
	require.Equal(t, int64(0), snapshot.EndTimestamp)
	require.Equal(t, int64(0), snapshot.SnapshotAt)
	require.Equal(t, dto.AdminAnalyticsGranularityDay, snapshot.Granularity)
	require.Equal(t, AdminAnalyticsDefaultLimit, snapshot.Limit)
	require.Equal(t, dto.AdminAnalyticsSortDesc, snapshot.SortOrder)

	allHistory := normalizeAdminPaidSubscriptionAnalyticsQuery(AdminAnalyticsQuery{RangeMode: AdminAnalyticsRangeModeAllHistory, SnapshotAt: 123})
	require.Equal(t, int64(0), allHistory.StartTimestamp)
	require.Equal(t, int64(123), allHistory.EndTimestamp)
	require.Equal(t, int64(123), allHistory.SnapshotAt)

	explicitRange := normalizeAdminPaidSubscriptionAnalyticsQuery(AdminAnalyticsQuery{RangeMode: AdminAnalyticsRangeModeAllHistory, SnapshotAt: 123, StartTimestamp: 10, EndTimestamp: 20, TimeRangeExplicit: true})
	require.Equal(t, int64(10), explicitRange.StartTimestamp)
	require.Equal(t, int64(20), explicitRange.EndTimestamp)
	require.Equal(t, int64(123), explicitRange.SnapshotAt)
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

func TestClassifyAdminCreditLifecycle(t *testing.T) {
	tests := []struct {
		name          string
		tokenLimit    int64
		tokenUsed     int64
		wantState     string
		wantAvailable int64
		wantDebt      int64
	}{
		{name: "positive balance", tokenLimit: 100, tokenUsed: 25, wantState: "active_credit", wantAvailable: 75},
		{name: "zero balance", tokenLimit: 100, tokenUsed: 100, wantState: "exhausted_credit"},
		{name: "negative balance", tokenLimit: 100, tokenUsed: 130, wantState: "credit_debt", wantDebt: 30},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classification := classifyAdminCreditLifecycle(test.tokenLimit, test.tokenUsed)
			require.Equal(t, test.wantState, classification.State)
			require.Equal(t, test.wantAvailable, classification.AvailableCredit)
			require.Equal(t, test.wantDebt, classification.SettlementDebt)
		})
	}
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

func TestAdminAnalyticsOverviewSeparatesCreditAvailabilityAndSettlementDebt(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC).Unix()
	creditCode := "analytics-credit-summary"
	timedCode := "analytics-timed-summary"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 31, Title: "Credit balance", Enabled: true, EntitlementType: SubscriptionEntitlementCreditBalance, BusinessCode: &creditCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 32, Title: "Timed", Enabled: true, EntitlementType: SubscriptionEntitlementTimed, BusinessCode: &timedCode}).Error)
	fixtures := []struct {
		userID          int
		subscriptionID  int
		planID          int
		entitlementType string
		tokenLimit      int64
		tokenUsed       int64
		endTime         int64
	}{
		{userID: 301, subscriptionID: 401, planID: 31, entitlementType: SubscriptionEntitlementCreditBalance, tokenLimit: 100, tokenUsed: 25},
		{userID: 302, subscriptionID: 402, planID: 31, entitlementType: SubscriptionEntitlementCreditBalance, tokenLimit: 100, tokenUsed: 100},
		{userID: 303, subscriptionID: 403, planID: 31, entitlementType: SubscriptionEntitlementCreditBalance, tokenLimit: 100, tokenUsed: 130},
		{userID: 304, subscriptionID: 404, planID: 32, entitlementType: SubscriptionEntitlementTimed, tokenLimit: 200, tokenUsed: 50, endTime: snapshot + 3600},
	}
	for _, fixture := range fixtures {
		require.NoError(t, DB.Create(&User{Id: fixture.userID, Username: "credit-summary-" + string(rune(fixture.userID)), Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-credit-summary-" + string(rune(fixture.userID))}).Error)
		require.NoError(t, DB.Create(&UserSubscription{Id: fixture.subscriptionID, UserId: fixture.userID, PlanId: fixture.planID, EntitlementType: fixture.entitlementType, Status: SubscriptionStatusActive, StartTime: snapshot - 60, EndTime: fixture.endTime, TokenLimit: fixture.tokenLimit, TokenUsed: fixture.tokenUsed, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	}

	response, err := GetAdminAnalyticsOverview(AdminAnalyticsQuery{SnapshotAt: snapshot, StartTimestamp: snapshot - 3600, EndTimestamp: snapshot, Limit: 20})

	require.NoError(t, err)
	assertions := response.Data.Summary
	require.Equal(t, 2, assertions.Subscriptions.ActiveCount)
	require.Equal(t, 1, assertions.Subscriptions.TimedActiveCount)
	require.Equal(t, 3, assertions.Subscriptions.CreditBalanceCount)
	require.Equal(t, 1, assertions.Subscriptions.CreditAvailableCount)
	require.Equal(t, 1, assertions.Subscriptions.CreditExhaustedCount)
	require.Equal(t, 1, assertions.Subscriptions.CreditDebtCount)
	require.Equal(t, int64(75), assertions.Quota.AvailableCredit)
	require.Equal(t, int64(30), assertions.Quota.SettlementDebt)
	require.Equal(t, int64(300), assertions.Quota.TokenLimit)
	require.Equal(t, int64(75), assertions.Quota.TokenUsed)
	require.Equal(t, int64(225), assertions.Quota.RemainingTokens)

	filtered, err := GetAdminAnalyticsOverview(AdminAnalyticsQuery{SnapshotAt: snapshot, StartTimestamp: snapshot - 3600, EndTimestamp: snapshot, Limit: 20, PlanIDs: []int{31}, UserIDs: []int{301}})
	require.NoError(t, err)
	require.Equal(t, 1, filtered.Data.Summary.Subscriptions.ActiveCount)
	require.Equal(t, 1, filtered.Data.Summary.Subscriptions.CreditBalanceCount)
	require.Equal(t, 1, filtered.Data.Summary.Subscriptions.CreditAvailableCount)
	require.Zero(t, filtered.Data.Summary.Subscriptions.CreditExhaustedCount)
	require.Zero(t, filtered.Data.Summary.Subscriptions.CreditDebtCount)
	require.Equal(t, int64(75), filtered.Data.Summary.Quota.AvailableCredit)
	require.Zero(t, filtered.Data.Summary.Quota.SettlementDebt)
}

func TestAdminAnalyticsQuotaRankingExposesCreditIdentityAndAvailability(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := time.Date(2026, 7, 31, 9, 30, 0, 0, time.UTC).Unix()
	creditCode := "analytics-credit-ranking"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 35, Title: "Credit ranking", Enabled: true, EntitlementType: SubscriptionEntitlementCreditBalance, BusinessCode: &creditCode}).Error)
	require.NoError(t, DB.Create(&User{Id: 305, Username: "credit-ranking", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-credit-ranking"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 405, UserId: 305, PlanId: 35, EntitlementType: SubscriptionEntitlementCreditBalance, Status: SubscriptionStatusActive, StartTime: snapshot - 60, TokenLimit: 100, TokenUsed: 25, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)

	response, err := GetAdminAnalyticsQuotaDistribution(AdminAnalyticsQuery{SnapshotAt: snapshot, Limit: 20})

	require.NoError(t, err)
	require.Len(t, response.Data.HighUsageUsers.Items, 1)
	item := response.Data.HighUsageUsers.Items[0]
	require.Equal(t, SubscriptionEntitlementCreditBalance, item.EntitlementType)
	require.Equal(t, "active_credit", item.LifecycleState)
	require.Equal(t, int64(75), item.AvailableCredit)
	require.Zero(t, item.SettlementDebt)
}

func TestAdminAnalyticsDrilldownSeparatesActiveCreditFromTimedHistory(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	require.NoError(t, DB.AutoMigrate(&SubscriptionConversion{}))
	snapshot := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC).Unix()
	timedCode := "analytics-history-timed"
	creditCode := "analytics-history-credit"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 41, Title: "Timed history", Enabled: true, EntitlementType: SubscriptionEntitlementTimed, BusinessCode: &timedCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 42, Title: "Credit target", Enabled: true, EntitlementType: SubscriptionEntitlementCreditBalance, BusinessCode: &creditCode}).Error)
	for id := 502; id <= 506; id++ {
		require.NoError(t, DB.Create(&User{Id: id, Username: "history-" + string(rune(id)), Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-history-" + string(rune(id))}).Error)
	}
	require.NoError(t, DB.Create(&UserSubscription{Id: 601, UserId: 504, PlanId: 42, EntitlementType: SubscriptionEntitlementCreditBalance, Status: SubscriptionStatusActive, StartTime: snapshot - 60, EndTime: 0, TokenLimit: 100, TokenUsed: 25, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 605, UserId: 505, PlanId: 42, EntitlementType: SubscriptionEntitlementCreditBalance, Status: SubscriptionStatusActive, StartTime: snapshot - 60, EndTime: 0, TokenLimit: 100, TokenUsed: 100, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 606, UserId: 506, PlanId: 42, EntitlementType: SubscriptionEntitlementCreditBalance, Status: SubscriptionStatusActive, StartTime: snapshot - 60, EndTime: 0, TokenLimit: 100, TokenUsed: 130, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 602, UserId: 502, PlanId: 41, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusExpired, StartTime: snapshot - 40*24*60*60, EndTime: snapshot - TimedSubscriptionConversionGraceSeconds + 1, TokenLimit: 100, TokenUsed: 20, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 603, UserId: 503, PlanId: 41, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusExpired, StartTime: snapshot - 40*24*60*60, EndTime: snapshot - TimedSubscriptionConversionGraceSeconds - 1, TokenLimit: 100, TokenUsed: 20, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 604, UserId: 504, PlanId: 41, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusConverted, StartTime: snapshot - 40*24*60*60, EndTime: snapshot + 3600, TokenLimit: 100, TokenUsed: 20, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder, ConvertedAt: snapshot - 30, ConversionId: 701, ConvertedToSubscriptionId: 601}).Error)
	require.NoError(t, DB.Create(&SubscriptionConversion{Id: 701, UserId: 504, IdempotencyKey: "analytics-history", SourceSubscriptionId: 604, SourcePlanId: 41, SourcePlanTitle: "Timed history", TargetSubscriptionId: 601, TargetPlanId: 42, LedgerId: 801, SourceStatus: SubscriptionStatusActive, GrantSource: SubscriptionGrantOrder, DatabaseNow: snapshot - 30, SourceStartTime: snapshot - 40*24*60*60, SourceEndTime: snapshot + 3600, ConvertedAt: snapshot - 30, CreatedAt: snapshot - 30}).Error)
	var source, target UserSubscription
	var conversion SubscriptionConversion
	require.NoError(t, DB.First(&source, 604).Error)
	require.NoError(t, DB.First(&target, 601).Error)
	require.NoError(t, DB.First(&conversion, 701).Error)
	require.Equal(t, source.UserId, target.UserId)
	require.Equal(t, source.UserId, conversion.UserId)

	response, err := GetAdminAnalyticsDrilldownSubscriptions(AdminAnalyticsQuery{SnapshotAt: snapshot, Limit: 20}, AdminAnalyticsDrilldownFilter{})

	require.NoError(t, err)
	require.Len(t, response.Data.Subscriptions.Items, 5)
	itemsByID := make(map[int]dto.AdminAnalyticsDrilldownSubscriptionItem)
	for _, item := range response.Data.Subscriptions.Items {
		itemsByID[item.SubscriptionID] = item
	}
	credit := itemsByID[601]
	require.Equal(t, SubscriptionEntitlementCreditBalance, credit.EntitlementType)
	require.Equal(t, "active_credit", credit.LifecycleState)
	require.Equal(t, int64(75), credit.AvailableCredit)
	require.Zero(t, credit.SettlementDebt)
	exhausted := itemsByID[605]
	require.Equal(t, "exhausted_credit", exhausted.LifecycleState)
	require.Zero(t, exhausted.AvailableCredit)
	require.Zero(t, exhausted.SettlementDebt)
	debt := itemsByID[606]
	require.Equal(t, "credit_debt", debt.LifecycleState)
	require.Zero(t, debt.AvailableCredit)
	require.Equal(t, int64(30), debt.SettlementDebt)
	grace := itemsByID[602]
	require.Equal(t, SubscriptionEntitlementTimed, grace.EntitlementType)
	require.Equal(t, "expired_grace", grace.LifecycleState)
	require.Equal(t, int64(1), grace.GraceRemainingSeconds)
	converted := itemsByID[604]
	require.Equal(t, "converted", converted.LifecycleState)
	require.Equal(t, 701, converted.ConversionID)
	require.Equal(t, 601, converted.TargetSubscriptionID)
	require.Equal(t, converted.UserID, converted.TargetUserID)
	require.Equal(t, 42, converted.TargetPlanID)
	require.Equal(t, "Credit target", converted.TargetPlanTitle)
	_, outsideGrace := itemsByID[603]
	require.False(t, outsideGrace)
}
func TestAdminAnalyticsDrilldownSubscriptionStatusesExcludeActiveRows(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	snapshot := time.Date(2026, 7, 31, 11, 0, 0, 0, time.UTC).Unix()
	code := "analytics-status-filter"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 61, Title: "Status filter", Enabled: true, EntitlementType: SubscriptionEntitlementTimed, BusinessCode: &code}).Error)
	for id := 521; id <= 524; id++ {
		require.NoError(t, DB.Create(&User{Id: id, Username: "status-" + string(rune(id)), Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-status-" + string(rune(id))}).Error)
	}
	require.NoError(t, DB.Create(&UserSubscription{Id: 621, UserId: 521, PlanId: 61, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusActive, StartTime: snapshot - 3600, EndTime: snapshot + 3600, TokenLimit: 100, TokenUsed: 10}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 622, UserId: 522, PlanId: 61, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusExpired, StartTime: snapshot - 40*24*60*60, EndTime: snapshot - 1, TokenLimit: 100, TokenUsed: 10}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 623, UserId: 523, PlanId: 61, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusConverted, StartTime: snapshot - 3600, EndTime: snapshot + 3600, TokenLimit: 100, TokenUsed: 10}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 624, UserId: 524, PlanId: 61, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusExpired, StartTime: snapshot - 60*24*60*60, EndTime: snapshot - TimedSubscriptionConversionGraceSeconds - 1, TokenLimit: 100, TokenUsed: 10}).Error)

	response, err := GetAdminAnalyticsDrilldownSubscriptions(AdminAnalyticsQuery{SnapshotAt: snapshot, Limit: 20, SubscriptionStatuses: []string{SubscriptionStatusConverted, SubscriptionStatusExpired}}, AdminAnalyticsDrilldownFilter{})

	require.NoError(t, err)
	require.Len(t, response.Data.Subscriptions.Items, 2)
	statuses := map[string]bool{}
	for _, item := range response.Data.Subscriptions.Items {
		statuses[item.Status] = true
	}
	require.Equal(t, map[string]bool{SubscriptionStatusConverted: true, SubscriptionStatusExpired: true}, statuses)
}

func TestAdminAnalyticsDrilldownDoesNotExposeMismatchedConversionTarget(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	require.NoError(t, DB.AutoMigrate(&SubscriptionConversion{}))
	snapshot := time.Date(2026, 7, 31, 10, 30, 0, 0, time.UTC).Unix()
	timedCode := "analytics-mismatched-timed"
	creditCode := "analytics-mismatched-credit"
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 51, Title: "Timed source", Enabled: true, EntitlementType: SubscriptionEntitlementTimed, BusinessCode: &timedCode}).Error)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 52, Title: "Secret Credit target", Enabled: true, EntitlementType: SubscriptionEntitlementCreditBalance, BusinessCode: &creditCode}).Error)
	require.NoError(t, DB.Create(&User{Id: 511, Username: "source-user", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-source-user"}).Error)
	require.NoError(t, DB.Create(&User{Id: 512, Username: "target-user", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-target-user"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 611, UserId: 511, PlanId: 51, EntitlementType: SubscriptionEntitlementTimed, Status: SubscriptionStatusConverted, StartTime: snapshot - 3600, EndTime: snapshot + 3600, TokenLimit: 100, TokenUsed: 20, ConversionId: 711, ConvertedToSubscriptionId: 612, ConvertedAt: snapshot - 30, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 612, UserId: 512, PlanId: 52, EntitlementType: SubscriptionEntitlementCreditBalance, Status: SubscriptionStatusActive, StartTime: snapshot - 60, TokenLimit: 100, TokenUsed: 25, GrantReason: SubscriptionGrantOrder, Source: SubscriptionGrantOrder}).Error)
	require.NoError(t, DB.Create(&SubscriptionConversion{Id: 711, UserId: 512, IdempotencyKey: "mismatched-conversion", SourceSubscriptionId: 999, SourcePlanId: 51, SourcePlanTitle: "Timed source", TargetSubscriptionId: 612, TargetPlanId: 52, LedgerId: 811, SourceStatus: SubscriptionStatusActive, GrantSource: SubscriptionGrantOrder, DatabaseNow: snapshot - 30, SourceStartTime: snapshot - 3600, SourceEndTime: snapshot + 3600, ConvertedAt: snapshot - 30, CreatedAt: snapshot - 30}).Error)

	response, err := GetAdminAnalyticsDrilldownSubscriptions(AdminAnalyticsQuery{SnapshotAt: snapshot, Limit: 20, UserIDs: []int{511}}, AdminAnalyticsDrilldownFilter{})

	require.NoError(t, err)
	require.Len(t, response.Data.Subscriptions.Items, 1)
	item := response.Data.Subscriptions.Items[0]
	require.Equal(t, 611, item.SubscriptionID)
	require.Equal(t, "converted", item.LifecycleState)
	require.Zero(t, item.TargetSubscriptionID)
	require.Zero(t, item.TargetUserID)
	require.Zero(t, item.TargetPlanID)
	require.Empty(t, item.TargetPlanTitle)
}

func TestAdminAnalyticsUserLifecycleOmitsBusinessGroupDistributions(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	require.NoError(t, DB.Create(&User{Id: 21, Username: "vip", Status: common.UserStatusEnabled, Group: "vip", AffCode: "aff-vip"}).Error)
	require.NoError(t, DB.Create(&User{Id: 22, Username: "default", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-default"}).Error)
	res, err := GetAdminAnalyticsUserLifecycle(AdminAnalyticsQuery{Limit: 20})
	require.NoError(t, err)
	require.Len(t, res.Data.Users.Items, 2)
}

func TestAdminAnalyticsSQLBuilderAvoidsDialectSpecificFunctions(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	query := AdminAnalyticsQuery{Limit: 20}
	_, err := GetAdminAnalyticsOverview(query)
	require.NoError(t, err)
	_, err = GetAdminAnalyticsPlanDistribution(query)
	require.NoError(t, err)
}
