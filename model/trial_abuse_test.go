package model

import (
	"fmt"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedTrialAbusePlan(t *testing.T, id int, price float64, trial bool) {
	t.Helper()
	code := fmt.Sprintf("trial-abuse-plan-%d", id)
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: id, Title: fmt.Sprintf("plan-%d", id), Enabled: true, PriceAmount: price, IsTrial: trial, InviteTrial: trial, BusinessCode: &code}).Error)
}

func seedTrialAbuseUser(t *testing.T, id int, username string, inviterID int, role int, createdAt int64) {
	t.Helper()
	if role == 0 {
		role = common.RoleCommonUser
	}
	require.NoError(t, DB.Create(&User{Id: id, Username: username, Status: common.UserStatusEnabled, Role: role, AffCode: fmt.Sprintf("trial-abuse-aff-%d", id), InviterId: inviterID, CreatedAt: createdAt}).Error)
}

func seedTrialAbuseSubscription(t *testing.T, id int, userID int, planID int, start int64, end int64, grantReason string, source string) {
	t.Helper()
	require.NoError(t, DB.Create(&UserSubscription{Id: id, UserId: userID, PlanId: planID, Status: "active", StartTime: start, EndTime: end, GrantReason: grantReason, Source: source}).Error)
}

func trialAbuseIntPtr(value int) *int {
	return &value
}

func seedTrialAbuseConsumeLogs(t *testing.T, userID int, firstCreatedAt int64, count int, ip string) {
	t.Helper()
	for i := 0; i < count; i++ {
		require.NoError(t, LOG_DB.Create(&Log{UserId: userID, Username: fmt.Sprintf("u%d", userID), CreatedAt: firstCreatedAt + int64(i), Type: LogTypeConsume, Quota: 10, MeteredTokens: trialAbuseIntPtr(20), Ip: ip}).Error)
	}
}

func trialAbuseTestQuery(now int64) TrialAbuseQuery {
	return TrialAbuseQuery{TrialEndStart: now - 1000, TrialEndEnd: now, SnapshotAt: now, MinConsumeCount: 2, MinClusterSize: 2, RiskLimit: 50, GroupLimit: 20}
}

func trialAbuseHasWarning(response *dto.TrialAbuseSummaryResponse, reason string) bool {
	for _, warning := range response.Warnings {
		if warning.Reason == reason {
			return true
		}
	}
	return false
}

func requireTrialAbuseInviterCluster(t *testing.T, response *dto.TrialAbuseSummaryResponse, inviterID int) dto.TrialAbuseInviterCluster {
	t.Helper()
	for _, cluster := range response.InviterClusters {
		if cluster.InviterID == inviterID {
			return cluster
		}
	}
	t.Fatalf("missing inviter cluster %d in %#v", inviterID, response.InviterClusters)
	return dto.TrialAbuseInviterCluster{}
}

func TestTrialAbuseSummaryExcludesActiveTrialPaidAndLowUsage(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700000000)
	seedTrialAbusePlan(t, 1, 0, true)
	seedTrialAbusePlan(t, 2, 10, false)

	seedTrialAbuseUser(t, 1, "active-trial", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseUser(t, 2, "paid-order", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseUser(t, 3, "paid-redemption", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseUser(t, 4, "paid-admin", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseUser(t, 5, "paid-monthly", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseUser(t, 6, "low-usage", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseUser(t, 7, "outside-window", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseUser(t, 8, "active-paid-order", 0, common.RoleCommonUser, now-500)

	seedTrialAbuseSubscription(t, 1, 1, 1, now-20, now+100, "trial_code", "")
	seedTrialAbuseSubscription(t, 2, 2, 1, now-200, now-100, "trial_code", "")
	seedTrialAbuseSubscription(t, 3, 3, 1, now-200, now-100, "trial_code", "")
	seedTrialAbuseSubscription(t, 4, 4, 1, now-200, now-100, "trial_code", "")
	seedTrialAbuseSubscription(t, 5, 5, 1, now-200, now-100, "trial_code", "")
	seedTrialAbuseSubscription(t, 6, 6, 1, now-200, now-100, "trial_code", "")
	seedTrialAbuseSubscription(t, 7, 7, 1, now-3000, now-2000, "trial_code", "")
	seedTrialAbuseSubscription(t, 8, 8, 1, now-20, now+100, "trial_code", "")

	seedTrialAbuseSubscription(t, 12, 2, 2, now-50, now+5000, SubscriptionGrantOrder, "")
	seedTrialAbuseSubscription(t, 13, 3, 2, now-50, now+5000, "", "redemption")
	seedTrialAbuseSubscription(t, 14, 4, 2, now-50, now+5000, " admin ", "")
	seedTrialAbuseSubscription(t, 15, 5, 2, now-50, now+5000, SubscriptionGrantMonthlyInviteEntitlement, "")
	seedTrialAbuseSubscription(t, 16, 8, 2, now-50, now+5000, SubscriptionGrantOrder, "")

	seedTrialAbuseConsumeLogs(t, 2, now-199, 3, "")
	seedTrialAbuseConsumeLogs(t, 3, now-199, 3, "")
	seedTrialAbuseConsumeLogs(t, 4, now-199, 3, "")
	seedTrialAbuseConsumeLogs(t, 5, now-199, 3, "")
	seedTrialAbuseConsumeLogs(t, 6, now-199, 1, "")
	seedTrialAbuseConsumeLogs(t, 7, now-2999, 3, "")

	query := trialAbuseTestQuery(now)
	query.TrialEndEnd = now + 200
	response, err := GetTrialAbuseSummary(query)

	require.NoError(t, err)
	assert.Equal(t, 3, response.Overview.TotalTrialUsers)
	assert.Equal(t, 1, response.Overview.ExpiredTrialUsers)
	assert.Equal(t, 1, response.Overview.ActiveTrialUsers)
	assert.Equal(t, 1, response.Overview.ExpiredUnpaidTrialUsers)
	assert.Equal(t, 0, response.Overview.HighUsageCandidateUsers)
	assert.Empty(t, response.RiskUsers)
}

func TestTrialAbuseSourceNormalizationUsesGrantReasonBeforeSource(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	assert.Equal(t, "trial_code", normalizeTrialAbuseSource(" trial_code ", " order "))
	assert.Equal(t, "order", normalizeTrialAbuseSource(" ", " order "))
	assert.True(t, isTrialAbusePaidEntitlementSource(normalizeTrialAbuseSource("", " redemption ")))
	assert.False(t, isTrialAbusePaidEntitlementSource(normalizeTrialAbuseSource(" invite_trial ", " order ")))

	now := int64(1700001000)
	seedTrialAbusePlan(t, 21, 0, true)
	seedTrialAbusePlan(t, 22, 20, false)
	seedTrialAbuseUser(t, 20, "source-inviter", 0, common.RoleCommonUser, now-1000)
	seedTrialAbuseUser(t, 21, "source-a", 20, common.RoleCommonUser, now-500)
	seedTrialAbuseUser(t, 22, "source-b", 20, common.RoleCommonUser, now-500)
	seedTrialAbuseSubscription(t, 21, 21, 21, now-200, now-100, "invite_trial", "")
	seedTrialAbuseSubscription(t, 22, 22, 21, now-200, now-100, "invite_trial", "")
	seedTrialAbuseSubscription(t, 23, 21, 22, now-50, now+1000, " trial_code ", " order ")
	seedTrialAbuseConsumeLogs(t, 21, now-199, 2, "")
	seedTrialAbuseConsumeLogs(t, 22, now-199, 2, "")

	response, err := GetTrialAbuseSummary(trialAbuseTestQuery(now))

	require.NoError(t, err)
	assert.Equal(t, 2, response.Overview.ExpiredUnpaidTrialUsers)
	assert.Equal(t, 2, response.Overview.HighUsageCandidateUsers)
}

func TestTrialAbuseRowsUseSourceFallbackForTrialGrants(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700001500)
	seedTrialAbusePlan(t, 26, 0, true)
	seedTrialAbuseUser(t, 26, "source-fallback-trial", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseSubscription(t, 26, 26, 26, now-200, now-100, "", "trial_code")
	seedTrialAbuseConsumeLogs(t, 26, now-199, 2, "")

	response, err := GetTrialAbuseSummary(trialAbuseTestQuery(now))

	require.NoError(t, err)
	assert.Equal(t, 1, response.Overview.ExpiredUnpaidTrialUsers)
	assert.Equal(t, 1, response.Overview.HighUsageCandidateUsers)
}

func TestTrialAbusePaidEntitlementUsesSourceFallback(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700001800)
	seedTrialAbusePlan(t, 27, 0, true)
	seedTrialAbusePlan(t, 28, 30, false)
	seedTrialAbuseUser(t, 27, "paid-source-fallback", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseSubscription(t, 27, 27, 27, now-200, now-100, "trial_code", "")
	seedTrialAbuseSubscription(t, 28, 27, 28, now-50, now+1000, "", "redemption")
	seedTrialAbuseConsumeLogs(t, 27, now-199, 2, "")

	response, err := GetTrialAbuseSummary(trialAbuseTestQuery(now))

	require.NoError(t, err)
	assert.Equal(t, 0, response.Overview.ExpiredUnpaidTrialUsers)
	assert.Empty(t, response.RiskUsers)
}

func TestTrialAbuseInviterConversionUsesExpiredInviteesDenominator(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700002000)
	seedTrialAbusePlan(t, 31, 0, true)
	seedTrialAbusePlan(t, 32, 30, false)
	seedTrialAbuseUser(t, 30, "conversion-inviter", 0, common.RoleCommonUser, now-1000)
	for i := 0; i < 10; i++ {
		userID := 300 + i
		seedTrialAbuseUser(t, userID, fmt.Sprintf("candidate-%d", i), 30, common.RoleCommonUser, now-500)
		seedTrialAbuseSubscription(t, 300+i, userID, 31, now-200, now-100, "invite_trial", "")
		seedTrialAbuseConsumeLogs(t, userID, now-199, 1, "")
	}
	seedTrialAbuseUser(t, 399, "paid-invitee", 30, common.RoleCommonUser, now-500)
	seedTrialAbuseSubscription(t, 399, 399, 31, now-200, now-100, "invite_trial", "")
	seedTrialAbuseSubscription(t, 499, 399, 32, now-50, now+5000, SubscriptionGrantOrder, "")

	query := trialAbuseTestQuery(now)
	query.MinConsumeCount = 1
	response, err := GetTrialAbuseSummary(query)

	require.NoError(t, err)
	cluster := requireTrialAbuseInviterCluster(t, response, 30)
	assert.Equal(t, 10, cluster.CandidateCount)
	assert.Equal(t, 11, cluster.ExpiredTrialInviteeCount)
	assert.Equal(t, 1, cluster.PaidEntitlementCount)
	assert.InDelta(t, 1.0/11.0, cluster.PaidConversionRate, 0.0001)
	assert.Equal(t, 10, response.RiskCounts.Medium)
}

func TestTrialAbuseInviterClusterWithoutInviteTrialDenominatorIsDisplayOnly(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700002500)
	seedTrialAbusePlan(t, 36, 0, true)
	seedTrialAbuseUser(t, 36, "trial-code-inviter", 0, common.RoleCommonUser, now-1000)
	for i := 0; i < 10; i++ {
		userID := 360 + i
		seedTrialAbuseUser(t, userID, fmt.Sprintf("trial-code-candidate-%d", i), 36, common.RoleCommonUser, now-500)
		seedTrialAbuseSubscription(t, 360+i, userID, 36, now-200, now-100, "trial_code", "")
		seedTrialAbuseConsumeLogs(t, userID, now-199, 1, "")
	}

	query := trialAbuseTestQuery(now)
	query.MinConsumeCount = 1
	response, err := GetTrialAbuseSummary(query)

	require.NoError(t, err)
	cluster := requireTrialAbuseInviterCluster(t, response, 36)
	assert.Equal(t, 0, cluster.ExpiredTrialInviteeCount)
	assert.Equal(t, "display_only", cluster.RiskParticipation)
	assert.Empty(t, response.RiskUsers)
}

func TestTrialAbuseDeduplicatesUsersBeforeInviterRiskClustering(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700002700)
	seedTrialAbusePlan(t, 37, 0, true)
	seedTrialAbuseUser(t, 37, "dedupe-inviter", 0, common.RoleCommonUser, now-1000)
	for i := 0; i < 5; i++ {
		userID := 370 + i
		seedTrialAbuseUser(t, userID, fmt.Sprintf("dedupe-candidate-%d", i), 37, common.RoleCommonUser, now-500)
		seedTrialAbuseSubscription(t, 3700+i*2, userID, 37, now-300, now-250, "invite_trial", "")
		seedTrialAbuseSubscription(t, 3701+i*2, userID, 37, now-200, now-100, "invite_trial", "")
		seedTrialAbuseConsumeLogs(t, userID, now-280, 1, "")
		seedTrialAbuseConsumeLogs(t, userID, now-180, 1, "")
	}

	query := trialAbuseTestQuery(now)
	query.MinConsumeCount = 1
	response, err := GetTrialAbuseSummary(query)

	require.NoError(t, err)
	cluster := requireTrialAbuseInviterCluster(t, response, 37)
	assert.Equal(t, 5, response.Overview.ExpiredUnpaidTrialUsers)
	assert.Equal(t, 5, response.Overview.HighUsageCandidateUsers)
	assert.Equal(t, 5, cluster.CandidateCount)
	assert.Equal(t, 5, cluster.ExpiredTrialInviteeCount)
	assert.Equal(t, "display_only", cluster.RiskParticipation)
	assert.Empty(t, response.RiskUsers)
}

func TestTrialAbuseDoesNotMarkPartialWhenDuplicateRowsHitRawLimit(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldCandidateLimit := trialAbuseCandidateLimit
	oldLogConsumeEnabled := common.LogConsumeEnabled
	trialAbuseCandidateLimit = 1
	common.LogConsumeEnabled = true
	t.Cleanup(func() {
		trialAbuseCandidateLimit = oldCandidateLimit
		common.LogConsumeEnabled = oldLogConsumeEnabled
	})

	now := int64(1700002800)
	seedTrialAbusePlan(t, 38, 0, true)
	seedTrialAbuseUser(t, 38, "raw-limit-duplicate", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseSubscription(t, 3800, 38, 38, now-300, now-250, "invite_trial", "")
	seedTrialAbuseSubscription(t, 3801, 38, 38, now-200, now-100, "invite_trial", "")
	seedTrialAbuseConsumeLogs(t, 38, now-280, 1, "")
	seedTrialAbuseConsumeLogs(t, 38, now-180, 1, "")

	query := trialAbuseTestQuery(now)
	query.MinConsumeCount = 1
	response, err := GetTrialAbuseSummary(query)

	require.NoError(t, err)
	assert.False(t, trialAbuseHasWarning(response, dto.TrialAbuseWarningCandidateLimitExceeded))
	assert.False(t, response.Overview.Partial)
}

func TestTrialAbuseMarksPartialWhenRawDuplicateRowsHideDistinctUsers(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldCandidateLimit := trialAbuseCandidateLimit
	oldLogConsumeEnabled := common.LogConsumeEnabled
	trialAbuseCandidateLimit = 2
	common.LogConsumeEnabled = true
	t.Cleanup(func() {
		trialAbuseCandidateLimit = oldCandidateLimit
		common.LogConsumeEnabled = oldLogConsumeEnabled
	})

	now := int64(1700002850)
	seedTrialAbusePlan(t, 39, 0, true)
	for i := 0; i < 3; i++ {
		userID := 390 + i
		seedTrialAbuseUser(t, userID, fmt.Sprintf("raw-limit-distinct-%d", i), 0, common.RoleCommonUser, now-500)
	}
	seedTrialAbuseSubscription(t, 3900, 390, 39, now-300, now-250, "trial_code", "")
	seedTrialAbuseSubscription(t, 3901, 390, 39, now-290, now-240, "trial_code", "")
	seedTrialAbuseSubscription(t, 3902, 391, 39, now-280, now-230, "trial_code", "")
	seedTrialAbuseSubscription(t, 3903, 392, 39, now-270, now-220, "trial_code", "")

	response, err := GetTrialAbuseSummary(trialAbuseTestQuery(now))

	require.NoError(t, err)
	assert.True(t, trialAbuseHasWarning(response, dto.TrialAbuseWarningCandidateLimitExceeded))
	assert.True(t, response.Overview.Partial)
}

func TestTrialAbuseManagedInviterDoesNotPromoteToMedium(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700003000)
	seedTrialAbusePlan(t, 41, 0, true)
	seedTrialAbuseUser(t, 40, "managed-inviter", 0, common.RoleAdminUser, now-1000)
	for i := 0; i < 10; i++ {
		userID := 400 + i
		seedTrialAbuseUser(t, userID, fmt.Sprintf("managed-candidate-%d", i), 40, common.RoleCommonUser, now-500)
		seedTrialAbuseSubscription(t, 400+i, userID, 41, now-200, now-100, "invite_trial", "")
		seedTrialAbuseConsumeLogs(t, userID, now-199, 1, "")
	}

	query := trialAbuseTestQuery(now)
	query.MinConsumeCount = 1
	response, err := GetTrialAbuseSummary(query)

	require.NoError(t, err)
	cluster := requireTrialAbuseInviterCluster(t, response, 40)
	assert.True(t, cluster.Managed)
	assert.Equal(t, 0, response.RiskCounts.High)
	assert.Equal(t, 0, response.RiskCounts.Medium)
}

func TestTrialAbuseManagedInviterDoesNotAddDisplayOnlyRiskUsers(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700003500)
	seedTrialAbusePlan(t, 46, 0, true)
	seedTrialAbuseUser(t, 46, "managed-display-inviter", 0, common.RoleRootUser, now-1000)
	for i := 0; i < 10; i++ {
		userID := 460 + i
		seedTrialAbuseUser(t, userID, fmt.Sprintf("managed-display-candidate-%d", i), 46, common.RoleCommonUser, now-500)
		seedTrialAbuseSubscription(t, 460+i, userID, 46, now-200, now-100, "invite_trial", "")
		seedTrialAbuseConsumeLogs(t, userID, now-199, 1, "")
	}

	query := trialAbuseTestQuery(now)
	query.MinConsumeCount = 1
	response, err := GetTrialAbuseSummary(query)

	require.NoError(t, err)
	assert.Empty(t, response.RiskUsers)
	assert.Equal(t, 0, response.Overview.RiskUserCount)
}

func TestTrialAbuseLogUnavailableReturnsWarning(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700004000)
	seedTrialAbusePlan(t, 51, 0, true)
	seedTrialAbuseUser(t, 51, "log-disabled", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseSubscription(t, 51, 51, 51, now-200, now-100, "trial_code", "")

	response, err := GetTrialAbuseSummary(trialAbuseTestQuery(now))

	require.NoError(t, err)
	assert.True(t, trialAbuseHasWarning(response, dto.TrialAbuseWarningLogUnavailable))
	assert.Contains(t, response.PartialSections[dto.TrialAbuseSectionUsageDistribution], dto.TrialAbuseWarningLogUnavailable)
}

func TestTrialAbuseRegistrationIPUnavailableDisablesStrongIPRisk(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700005000)
	seedTrialAbusePlan(t, 61, 0, true)
	for i := 0; i < 2; i++ {
		userID := 610 + i
		seedTrialAbuseUser(t, userID, fmt.Sprintf("same-ip-%d", i), 0, common.RoleCommonUser, now-500)
		seedTrialAbuseSubscription(t, 610+i, userID, 61, now-200, now-100, "trial_code", "")
		seedTrialAbuseConsumeLogs(t, userID, now-199, 2, "203.0.113.9")
	}

	response, err := GetTrialAbuseSummary(trialAbuseTestQuery(now))

	require.NoError(t, err)
	assert.True(t, trialAbuseHasWarning(response, dto.TrialAbuseWarningRegistrationIPUnavailable))
	assert.Empty(t, response.RiskUsers)
	require.NotEmpty(t, response.IPClusters)
	assert.Equal(t, "203.0.113.9", response.IPClusters[0].ObservedIP)
	assert.Equal(t, "consume_log", response.IPClusters[0].IPSource)
	assert.False(t, response.IPClusters[0].RegistrationIPAvailable)
}

func TestTrialAbuseIPClustersOnlyShowClusteredCandidates(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })

	now := int64(1700005500)
	seedTrialAbusePlan(t, 66, 0, true)
	seedTrialAbuseUser(t, 66, "single-observed-ip", 0, common.RoleCommonUser, now-500)
	seedTrialAbuseSubscription(t, 66, 66, 66, now-200, now-100, "trial_code", "")
	seedTrialAbuseConsumeLogs(t, 66, now-199, 2, "203.0.113.66")

	response, err := GetTrialAbuseSummary(trialAbuseTestQuery(now))

	require.NoError(t, err)
	assert.Empty(t, response.IPClusters)
}

func TestClassifyTrialAbuseWithRegistrationIPAvailable(t *testing.T) {
	selfInvite := classifyTrialAbuseRisks(trialAbuseClassificationInput{RegistrationIPAvailable: true, SameRegistrationIPCandidateCount: 2, SameRegistrationIPSelfInviteCandidateCount: 3, MinClusterSize: 2})
	assert.Equal(t, dto.TrialAbuseRiskLevelHigh, selfInvite.RiskLevel)
	assert.Contains(t, selfInvite.RiskReasons, dto.TrialAbuseRiskReasonSameRegistrationIPSelfInviteChain)

	largeIPCluster := classifyTrialAbuseRisks(trialAbuseClassificationInput{RegistrationIPAvailable: true, SameRegistrationIPCandidateCount: 5, MinClusterSize: 2})
	assert.Equal(t, dto.TrialAbuseRiskLevelHigh, largeIPCluster.RiskLevel)
	assert.Contains(t, largeIPCluster.RiskReasons, dto.TrialAbuseRiskReasonSameRegistrationIPCluster)

	mediumIPCluster := classifyTrialAbuseRisks(trialAbuseClassificationInput{RegistrationIPAvailable: true, SameRegistrationIPCandidateCount: 3, MinClusterSize: 2})
	assert.Equal(t, dto.TrialAbuseRiskLevelMedium, mediumIPCluster.RiskLevel)
}

func TestTrialAbusePartialWarningsForLimits(t *testing.T) {
	t.Run("candidate limit", func(t *testing.T) {
		setupAdminAnalyticsTestDBs(t)
		oldCandidateLimit := trialAbuseCandidateLimit
		oldLogConsumeEnabled := common.LogConsumeEnabled
		trialAbuseCandidateLimit = 1
		common.LogConsumeEnabled = true
		t.Cleanup(func() {
			trialAbuseCandidateLimit = oldCandidateLimit
			common.LogConsumeEnabled = oldLogConsumeEnabled
		})

		now := int64(1700006000)
		seedTrialAbusePlan(t, 71, 0, true)
		for i := 0; i < 2; i++ {
			userID := 710 + i
			seedTrialAbuseUser(t, userID, fmt.Sprintf("limit-%d", i), 0, common.RoleCommonUser, now-500)
			seedTrialAbuseSubscription(t, 710+i, userID, 71, now-200, now-100, "trial_code", "")
		}

		response, err := GetTrialAbuseSummary(trialAbuseTestQuery(now))

		require.NoError(t, err)
		assert.True(t, trialAbuseHasWarning(response, dto.TrialAbuseWarningCandidateLimitExceeded))
		assert.Contains(t, response.PartialSections[dto.TrialAbuseSectionOverview], dto.TrialAbuseWarningCandidateLimitExceeded)
		assert.True(t, response.Overview.Partial)
		assert.Contains(t, response.PartialSections[dto.TrialAbuseSectionRiskCounts], dto.TrialAbuseWarningCandidateLimitExceeded)
		assert.True(t, response.RiskCounts.Partial)
	})

	t.Run("log scan limit", func(t *testing.T) {
		setupAdminAnalyticsTestDBs(t)
		oldLogScanLimit := trialAbuseLogScanLimit
		oldLogConsumeEnabled := common.LogConsumeEnabled
		trialAbuseLogScanLimit = 1
		common.LogConsumeEnabled = true
		t.Cleanup(func() {
			trialAbuseLogScanLimit = oldLogScanLimit
			common.LogConsumeEnabled = oldLogConsumeEnabled
		})

		now := int64(1700007000)
		seedTrialAbusePlan(t, 81, 0, true)
		seedTrialAbuseUser(t, 81, "log-limit", 0, common.RoleCommonUser, now-500)
		seedTrialAbuseSubscription(t, 81, 81, 81, now-200, now-100, "trial_code", "")
		seedTrialAbuseConsumeLogs(t, 81, now-199, 2, "")

		query := trialAbuseTestQuery(now)
		query.MinConsumeCount = 1
		response, err := GetTrialAbuseSummary(query)

		require.NoError(t, err)
		assert.True(t, trialAbuseHasWarning(response, dto.TrialAbuseWarningLogLimitExceeded))
		assert.Contains(t, response.PartialSections[dto.TrialAbuseSectionOverview], dto.TrialAbuseWarningLogLimitExceeded)
		assert.Contains(t, response.PartialSections[dto.TrialAbuseSectionInviterClusters], dto.TrialAbuseWarningLogLimitExceeded)
		assert.Contains(t, response.PartialSections[dto.TrialAbuseSectionUsageDistribution], dto.TrialAbuseWarningLogLimitExceeded)
	})
}

func TestTrialAbusePercentileHandlesEmptyAndSingleSamples(t *testing.T) {
	assert.Equal(t, 0, trialAbusePercentile(nil, 95))
	assert.Equal(t, 7, trialAbusePercentile([]int{7}, 95))
	assert.Equal(t, 10, trialAbusePercentile([]int{1, 10}, 75))
	assert.Equal(t, 10, trialAbusePercentile([]int{1, 10}, int(math.Ceil(75))))
}
