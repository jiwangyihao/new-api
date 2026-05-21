package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestAdminAnalyticsRiskHighExhaustionBoundary(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 1, Title: "Basic", Enabled: true}).Error)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "u1", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-risk-1"}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "u2", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-risk-2"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 1, UserId: 1, PlanId: 1, Status: "active", StartTime: now - 100, EndTime: now + 100, TokenLimit: 1000, TokenUsed: 899, GrantReason: "order"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 2, UserId: 2, PlanId: 1, Status: "active", StartTime: now - 100, EndTime: now + 100, TokenLimit: 1000, TokenUsed: 900, GrantReason: "order"}).Error)

	res, err := GetAdminAnalyticsRisks(AdminAnalyticsQuery{SnapshotAt: now, StartTimestamp: now - 100, EndTimestamp: now, Limit: 20})
	require.NoError(t, err)
	keys := adminRiskKeySet(res.Data)
	_, ok := keys["high_exhaustion_risk"]
	require.True(t, ok, adminRiskDebugKeys(res.Data))
}

func TestAdminAnalyticsRiskSystemAndResetBoundaries(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 1, Title: "Basic", Enabled: true}).Error)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "u1", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-sys-1"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 1, UserId: 1, PlanId: 1, Status: "active", StartTime: now - 100, EndTime: now + 100, TokenLimit: -1, TokenUsed: 0, GrantReason: "order", NextResetTime: now - 25*3600}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 2, UserId: 1, PlanId: 1, Status: "active", StartTime: now - 100, EndTime: now + 100, TokenLimit: 100, TokenUsed: 1, GrantReason: "order", NextResetTime: now - 25*3600}).Error)

	res, err := GetAdminAnalyticsRisks(AdminAnalyticsQuery{SnapshotAt: now, StartTimestamp: now - 100, EndTimestamp: now, Limit: 20})
	require.NoError(t, err)
	keys := adminRiskKeySet(res.Data)
	_, invalid := keys["invalid_negative_token_quota"]
	_, reset := keys["reset_overdue"]
	require.True(t, invalid, adminRiskDebugKeys(res.Data))
	require.True(t, reset, adminRiskDebugKeys(res.Data))
}

func TestAdminAnalyticsRiskManyInvitesLowQualified(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&User{Id: 1, Username: "inviter", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-inviter"}).Error)
	for i := 0; i < adminRiskInviteMinDirect; i++ {
		require.NoError(t, DB.Create(&User{Id: 100 + i, Username: "invitee" + string(rune('a'+i)), Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-invitee" + string(rune('a'+i)), InviterId: 1}).Error)
	}

	res, err := GetAdminAnalyticsRisks(AdminAnalyticsQuery{SnapshotAt: now, StartTimestamp: now - 100, EndTimestamp: now, Limit: 20})
	require.NoError(t, err)
	_, ok := adminRiskKeySet(res.Data)["many_invites_low_qualified"]
	require.True(t, ok, adminRiskDebugKeys(res.Data))
}

func TestAdminAnalyticsRiskRewardSubscriptionNeverUsed(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&SubscriptionPlan{Id: 1, Title: "Reward", Enabled: true}).Error)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "reward", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-reward"}).Error)
	require.NoError(t, DB.Create(&UserSubscription{Id: 1, UserId: 1, PlanId: 1, Status: "active", StartTime: now - adminRiskRewardNeverUsedMinAgeSeconds - 1, EndTime: now + 100, TokenLimit: 100, TokenUsed: 0, GrantReason: SubscriptionGrantMonthlyInviteEntitlement}).Error)

	res, err := GetAdminAnalyticsRisks(AdminAnalyticsQuery{SnapshotAt: now, StartTimestamp: now - 100, EndTimestamp: now, Limit: 20})
	require.NoError(t, err)
	_, ok := adminRiskKeySet(res.Data)["reward_subscription_never_used"]
	require.True(t, ok, adminRiskDebugKeys(res.Data))
}

func TestAdminAnalyticsRiskIncludesCandidateLogLimitExceededSystemRisk(t *testing.T) {
	setupAdminAnalyticsTestDBs(t)
	items := []dto.AdminAnalyticsRiskItem{newAdminRisk("candidate_log_limit_exceeded", dto.AdminAnalyticsRiskSeverityWarning, dto.AdminAnalyticsRiskCategorySystem, "candidate log limit exceeded", adminAnalyticsCandidateLogLimit, float64(adminAnalyticsCandidateLogLimit), nil)}
	list := adminRiskList(items, AdminAnalyticsQuery{Limit: 20}, dto.AdminAnalyticsRiskCategorySystem)
	require.Len(t, list.Items, 1)
	require.Equal(t, "candidate_log_limit_exceeded", list.Items[0].RiskKey)
}
