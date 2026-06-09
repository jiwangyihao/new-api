package service

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGPTAbuseAdminServiceTest(t *testing.T) {
	t.Helper()
	setupGPTAbuseSignalServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}, &model.Channel{}, &model.Log{}, &model.GPTAbuseRepeatBlockLog{}))
	for _, tableName := range []string{"gpt_abuse_repeat_block_logs", "logs", "tokens", "channels"} {
		_ = model.DB.Exec("DELETE FROM " + tableName).Error
	}
	model.ClearPrimaryBillableSubscriptionCacheForTest()
	t.Cleanup(model.ClearPrimaryBillableSubscriptionCacheForTest)
}

func TestListGPTAbuseUsersReturnsRawAndEffectiveCounts(t *testing.T) {
	runListGPTAbuseUsersReturnsRawAndEffectiveCounts(t)
}

func TestGPTAbuseAdminListUsersReturnsRawAndEffectiveCounts(t *testing.T) {
	runListGPTAbuseUsersReturnsRawAndEffectiveCounts(t)
}

func runListGPTAbuseUsersReturnsRawAndEffectiveCounts(t *testing.T) {
	t.Helper()
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81201
	planID := 81202
	businessCode := "gpt-abuse-admin-list"
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "gpt-abuse-list-user", Email: "abuse-list@example.com", Status: common.UserStatusEnabled, AffCode: "aff-gpt-abuse-list"}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "GPT Abuse Admin Plan", Enabled: true, ConcurrencyLimit: 1, GPTAbuseWarningLimit: 5, BusinessCode: &businessCode}).Error)
	model.InvalidateSubscriptionPlanCache(planID)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 81203, UserId: userID, PlanId: planID, Status: "active", StartTime: now - 60, EndTime: now + 3600, TokenLimit: 1000, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)

	logs := []model.GPTAbuseSignalLog{
		{CreatedAt: start + 10, UserId: userID, Username: "gpt-abuse-list-user", UserEmail: "abuse-list@example.com", RequestId: "req-warning-1", RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelId: 41, ChannelName: "Sub2API", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-list-1"},
		{CreatedAt: start + 20, UserId: userID, Username: "gpt-abuse-list-user", UserEmail: "abuse-list@example.com", RequestId: "req-warning-2", RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelId: 42, ChannelName: "Sub2API Secondary", Source: model.GPTAbuseSignalSourceSSEMetadata, Kind: model.GPTAbuseKindHighRiskCyberReroute, Severity: model.GPTAbuseSeverityMedium, CountEligible: true, DedupeKey: "admin-list-2"},
		{CreatedAt: start + 30, UserId: userID, Username: "gpt-abuse-list-user", UserEmail: "abuse-list@example.com", RequestId: "req-warning-3", RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelId: 43, ChannelName: "Sub2API Latest", Source: model.GPTAbuseSignalSourceSSEResponseFailed, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-list-3"},
	}
	for i := range logs {
		require.NoError(t, model.DB.Create(&logs[i]).Error)
	}
	require.NoError(t, model.DB.Create(&model.GPTAbuseWarningReset{UserId: userID, WindowStart: start, WindowEnd: end, ResetAt: start + 25, ResetBy: 9001, PreviousRawCount: 2, PreviousCount: 2, CutoffSignalLogID: logs[1].Id, Reason: "manual review"}).Error)
	activeUserID := userID
	require.NoError(t, model.DB.Create(&model.GPTAbuseUserSuspension{UserId: userID, ActiveUserId: &activeUserID, Status: model.GPTAbuseSuspensionStatusActive, Reason: "gpt_abuse_daily_limit", SuspendedUntil: end, TriggerLogId: logs[2].Id, DailyCount: 5, DailyLimit: 5}).Error)

	response, err := ListGPTAbuseUsers(context.Background(), dto.GPTAbuseUserListQuery{StartTimestamp: start, EndTimestamp: end, Limit: 10})

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Len(t, response.Items, 1)
	item := response.Items[0]
	assert.Equal(t, userID, item.UserID)
	assert.Equal(t, "gpt-abuse-list-user", item.Username)
	assert.Equal(t, "abuse-list@example.com", item.UserEmail)
	assert.Equal(t, 3, item.WarningCount)
	assert.Equal(t, 1, item.EffectiveWarningCount)
	assert.Equal(t, 5, item.DailyLimit)
	assert.Equal(t, 4, item.RemainingWarningCount)
	assert.Equal(t, 2, item.HighCount)
	assert.Equal(t, 1, item.MediumCount)
	assert.Equal(t, model.GPTAbuseSeverityHigh, item.MaxSeverity)
	assert.Equal(t, logs[2].CreatedAt, item.LatestWarningAt)
	assert.Equal(t, logs[2].Kind, item.LatestKind)
	assert.Equal(t, logs[2].Source, item.LatestSource)
	assert.Equal(t, logs[2].RequestedModel, item.LatestRequestedModel)
	assert.Equal(t, logs[2].UpstreamModel, item.LatestUpstreamModel)
	assert.Equal(t, logs[2].ChannelId, item.LatestChannelID)
	assert.Equal(t, logs[2].ChannelName, item.LatestChannelName)
	assert.Equal(t, model.GPTAbuseSuspensionStatusActive, item.SuspensionStatus)
	require.NotNil(t, item.ActiveSuspension)
	assert.Equal(t, "gpt_abuse_daily_limit", item.ActiveSuspension.Reason)
	assert.Equal(t, end, item.ActiveSuspension.SuspendedUntil)
	assert.Equal(t, 5, item.ActiveSuspension.DailyCount)
	assert.Equal(t, 5, item.ActiveSuspension.DailyLimit)
	assert.Equal(t, int64(0), item.LatestRepeatBlockAt)
	assert.Equal(t, 0, item.RepeatBlockCount)
}

func TestListGPTAbuseUsersUsesCurrentSubscriptionPlanLimit(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81211
	planID := 81212
	businessCode := "gpt-abuse-admin-plan-limit"
	reason := model.SubscriptionGrantOrder
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "limit-plan-user", Email: "limit-plan@example.com", Status: common.UserStatusEnabled, AffCode: "aff-limit-plan"}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "GPT Abuse Limit Plan", Enabled: true, ConcurrencyLimit: 1, GPTAbuseWarningLimit: 7, BusinessCode: &businessCode}).Error)
	model.InvalidateSubscriptionPlanCache(planID)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 81213, UserId: userID, PlanId: planID, Status: "active", StartTime: now - 60, EndTime: now + 3600, TokenLimit: 1000, GrantReason: reason, Source: reason}).Error)
	require.NoError(t, model.DB.Create(&model.GPTAbuseSignalLog{CreatedAt: start + 40, UserId: userID, Username: "limit-plan-user", UserEmail: "limit-plan@example.com", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-list-plan-limit"}).Error)

	response, err := ListGPTAbuseUsers(context.Background(), dto.GPTAbuseUserListQuery{StartTimestamp: start, EndTimestamp: end, UserID: userID, Limit: 10})

	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	assert.Equal(t, 7, response.Items[0].DailyLimit)
	assert.Equal(t, 6, response.Items[0].RemainingWarningCount)
}

func TestListGPTAbuseUsersUsesDefaultLimitWhenPlanLimitUnset(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81214
	planID := 81215
	businessCode := "gpt-abuse-admin-unset-limit"
	reason := model.SubscriptionGrantOrder
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "unset-plan-user", Email: "unset-plan@example.com", Status: common.UserStatusEnabled, AffCode: "aff-unset-plan"}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: planID, Title: "Unset GPT Abuse Limit Plan", Enabled: true, ConcurrencyLimit: common.GPTAbuseDefaultWarningLimit + 10, GPTAbuseWarningLimit: 0, BusinessCode: &businessCode}).Error)
	model.InvalidateSubscriptionPlanCache(planID)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 81215, UserId: userID, PlanId: planID, Status: "active", StartTime: now - 60, EndTime: now + 3600, TokenLimit: 1000, GrantReason: reason, Source: reason}).Error)
	require.NoError(t, model.DB.Create(&model.GPTAbuseSignalLog{CreatedAt: start + 42, UserId: userID, Username: "unset-plan-user", UserEmail: "unset-plan@example.com", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-list-unset-plan-limit"}).Error)

	response, err := ListGPTAbuseUsers(context.Background(), dto.GPTAbuseUserListQuery{StartTimestamp: start, EndTimestamp: end, UserID: userID, Limit: 10})

	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	assert.Equal(t, common.GPTAbuseDefaultWarningLimit, response.Items[0].DailyLimit)
}

func TestListGPTAbuseUserLogsReturnsOnlySanitizedUpstreamWarningExtra(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81243
	extra, err := common.Marshal(map[string]any{
		"prompt": "do not expose",
		"upstream_warning": map[string]any{
			"event_type":      "response.failed",
			"error_code":      "cyber_policy",
			"error_type":      "invalid_request_error",
			"response_status": "200",
			"error_message":   "do not expose message",
			"raw_error":       strings.Repeat("x", 1200),
			"prompt":          "do not expose nested",
		},
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "log-sanitize-user", Status: common.UserStatusEnabled, AffCode: "aff-log-sanitize"}).Error)
	require.NoError(t, model.DB.Create(&model.GPTAbuseSignalLog{CreatedAt: start + 12, UserId: userID, Username: "log-sanitize-user", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, Extra: string(extra), DedupeKey: "admin-log-sanitize"}).Error)

	response, err := ListGPTAbuseUserLogs(context.Background(), userID, dto.GPTAbuseLogQuery{StartTimestamp: start, EndTimestamp: end, Limit: 10})

	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	encoded, err := common.Marshal(response.Items[0].Extra)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "upstream_warning")
	assert.Contains(t, string(encoded), "cyber_policy")
	assert.NotContains(t, string(encoded), "do not expose")
	assert.NotContains(t, string(encoded), "raw_error")
	assert.NotContains(t, string(encoded), "error_message")
	assert.Contains(t, string(encoded), "response.failed")
}

func TestListGPTAbuseUsersUsesSelectedActiveSubscriptionLimit(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81216
	lowPlanID := 81217
	highPlanID := 81218
	lowCode := "gpt-abuse-low-limit"
	highCode := "gpt-abuse-high-limit"
	user := model.User{Id: userID, Username: "selected-plan-user", Email: "selected-plan@example.com", Status: common.UserStatusEnabled, AffCode: "aff-selected-plan"}
	setting := user.GetSetting()
	setting.ActiveSubscriptionId = 81220
	user.SetSetting(setting)
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: lowPlanID, Title: "Low GPT Abuse Limit", Enabled: true, GPTAbuseWarningLimit: 2, BusinessCode: &lowCode}).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: highPlanID, Title: "High GPT Abuse Limit", Enabled: true, GPTAbuseWarningLimit: 9, BusinessCode: &highCode}).Error)
	model.InvalidateSubscriptionPlanCache(lowPlanID)
	model.InvalidateSubscriptionPlanCache(highPlanID)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 81219, UserId: userID, PlanId: lowPlanID, Status: "active", StartTime: now - 60, EndTime: now + 7200, TokenLimit: 1000, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 81220, UserId: userID, PlanId: highPlanID, Status: "active", StartTime: now - 60, EndTime: now + 3600, TokenLimit: 1000, GrantReason: model.SubscriptionGrantOrder, Source: model.SubscriptionGrantOrder}).Error)
	require.NoError(t, model.DB.Create(&model.GPTAbuseSignalLog{CreatedAt: start + 45, UserId: userID, Username: "selected-plan-user", UserEmail: "selected-plan@example.com", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-list-selected-limit"}).Error)

	response, err := ListGPTAbuseUsers(context.Background(), dto.GPTAbuseUserListQuery{StartTimestamp: start, EndTimestamp: end, UserID: userID, Limit: 10})

	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	assert.Equal(t, 9, response.Items[0].DailyLimit)
	assert.Equal(t, 8, response.Items[0].RemainingWarningCount)
}

func TestListGPTAbuseUsersUsesLatestLogIDWhenTimestampsTie(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81246
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "latest-tie-user", Status: common.UserStatusEnabled, AffCode: "aff-latest-tie"}).Error)
	logs := []model.GPTAbuseSignalLog{
		{CreatedAt: start + 60, UserId: userID, Username: "latest-tie-user", RequestId: "req-old-id", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityMedium, RequestedModel: "gpt-5.5-old", CountEligible: true, DedupeKey: "admin-list-latest-tie-1"},
		{CreatedAt: start + 60, UserId: userID, Username: "latest-tie-user", RequestId: "req-new-id", Source: model.GPTAbuseSignalSourceSSEResponseFailed, Kind: model.GPTAbuseKindGenericAbuseSecurityWarning, Severity: model.GPTAbuseSeverityHigh, RequestedModel: "gpt-5.5-new", CountEligible: true, DedupeKey: "admin-list-latest-tie-2"},
	}
	require.NoError(t, model.DB.Create(&logs).Error)

	response, err := ListGPTAbuseUsers(context.Background(), dto.GPTAbuseUserListQuery{StartTimestamp: start, EndTimestamp: end, UserID: userID, Limit: 10})

	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	assert.Equal(t, model.GPTAbuseSignalSourceSSEResponseFailed, response.Items[0].LatestSource)
	assert.Equal(t, model.GPTAbuseKindGenericAbuseSecurityWarning, response.Items[0].LatestKind)
	assert.Equal(t, "gpt-5.5-new", response.Items[0].LatestRequestedModel)
}

func TestNormalizeGPTAbuseWindowCapsCustomRange(t *testing.T) {
	start, end := normalizeGPTAbuseWindow(100, 100+gptAbuseAdminMaxWindowSeconds+60)

	assert.Equal(t, int64(100), start)
	assert.Equal(t, int64(100+gptAbuseAdminMaxWindowSeconds), end)
}

func TestListGPTAbuseUsersUsesDefaultLimitWithoutPlan(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81221
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "limit-default-user", Email: "limit-default@example.com", Status: common.UserStatusEnabled, AffCode: "aff-limit-default"}).Error)
	require.NoError(t, model.DB.Create(&model.GPTAbuseSignalLog{CreatedAt: start + 50, UserId: userID, Username: "limit-default-user", UserEmail: "limit-default@example.com", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-list-default-limit"}).Error)

	response, err := ListGPTAbuseUsers(context.Background(), dto.GPTAbuseUserListQuery{StartTimestamp: start, EndTimestamp: end, UserID: userID, Limit: 10})

	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	assert.Equal(t, common.GPTAbuseDefaultWarningLimit, response.Items[0].DailyLimit)
	assert.Equal(t, common.GPTAbuseDefaultWarningLimit-1, response.Items[0].RemainingWarningCount)
}

func TestListGPTAbuseUsersFiltersSuspensionStatus(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	suspendedID := 81231
	warningOnlyID := 81232
	for _, user := range []model.User{
		{Id: suspendedID, Username: "status-suspended", Status: common.UserStatusEnabled, AffCode: "aff-status-suspended"},
		{Id: warningOnlyID, Username: "status-warning", Status: common.UserStatusEnabled, AffCode: "aff-status-warning"},
	} {
		require.NoError(t, model.DB.Create(&user).Error)
	}
	require.NoError(t, model.DB.Create(&[]model.GPTAbuseSignalLog{
		{CreatedAt: start + 10, UserId: suspendedID, Username: "status-suspended", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-list-status-suspended"},
		{CreatedAt: start + 20, UserId: warningOnlyID, Username: "status-warning", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-list-status-warning"},
	}).Error)
	activeUserID := suspendedID
	require.NoError(t, model.DB.Create(&model.GPTAbuseUserSuspension{UserId: suspendedID, ActiveUserId: &activeUserID, Status: model.GPTAbuseSuspensionStatusActive, Reason: "gpt_abuse_daily_limit", SuspendedUntil: end, DailyCount: 1, DailyLimit: 1}).Error)

	suspended, err := ListGPTAbuseUsers(context.Background(), dto.GPTAbuseUserListQuery{StartTimestamp: start, EndTimestamp: end, Status: "active_suspended", Limit: 10})
	require.NoError(t, err)
	require.Len(t, suspended.Items, 1)
	assert.Equal(t, suspendedID, suspended.Items[0].UserID)
	warningOnly, err := ListGPTAbuseUsers(context.Background(), dto.GPTAbuseUserListQuery{StartTimestamp: start, EndTimestamp: end, Status: "warning_only", Limit: 10})
	require.NoError(t, err)
	require.Len(t, warningOnly.Items, 1)
	assert.Equal(t, warningOnlyID, warningOnly.Items[0].UserID)
}

func TestListGPTAbuseUserLogsReturnsExtraAndEligibilityFilter(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81241
	extra, err := common.Marshal(map[string]any{"upstream_warning": map[string]any{"code": "x"}})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "log-detail-user", Status: common.UserStatusEnabled, AffCode: "aff-log-detail"}).Error)
	require.NoError(t, model.DB.Create(&[]model.GPTAbuseSignalLog{
		{CreatedAt: start + 10, UserId: userID, Username: "log-detail-user", RequestId: "req-log-1", UpstreamRequestId: "req-up-log-1", Endpoint: "/v1/responses", RelayMode: 1, RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelId: 42, ChannelName: "Primary", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, StatusCode: 400, ErrorCode: "policy", ErrorType: "invalid_request_error", CountEligible: true, Extra: string(extra), DedupeKey: "admin-log-detail-1"},
		{CreatedAt: start + 20, UserId: userID, Username: "log-detail-user", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: false, DedupeKey: "admin-log-detail-2"},
	}).Error)

	response, err := ListGPTAbuseUserLogs(context.Background(), userID, dto.GPTAbuseLogQuery{StartTimestamp: start, EndTimestamp: end, CountEligible: "true", Limit: 10})

	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	item := response.Items[0]
	assert.Equal(t, "req-log-1", item.RequestID)
	assert.Equal(t, "req-up-log-1", item.UpstreamRequestID)
	assert.Equal(t, "/v1/responses", item.Endpoint)
	assert.Equal(t, 1, item.RelayMode)
	assert.Equal(t, "gpt-5.5", item.RequestedModel)
	assert.Equal(t, "gpt-5.5", item.UpstreamModel)
	assert.Equal(t, 42, item.ChannelID)
	assert.Equal(t, "Primary", item.ChannelName)
	assert.True(t, item.CountEligible)
	assert.NotNil(t, item.Extra)
}

func TestListGPTAbuseRepeatBlocksReturnsFingerprintPrefixAndWarningAttribution(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81301
	fullFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "repeat-admin-user", Status: common.UserStatusEnabled, AffCode: "aff-repeat-admin"}).Error)
	require.NoError(t, model.DB.Create(&model.GPTAbuseRepeatBlockLog{CreatedAt: start + 60, UserId: userID, Username: "repeat-admin-user", TokenId: 81302, TokenName: "admin-token", RequestId: "req-repeat-current", Endpoint: "/v1/responses", RelayMode: 0, RequestedModel: "gpt-5.5", BodyFingerprint: fullFingerprint, FirstWarningLogId: 81303, FirstWarningAt: start + 30, FirstWarningRequestId: "req-first-warning", FirstWarningUpstreamRequestId: "req-upstream-warning", FirstWarningSource: model.GPTAbuseSignalSourceSSEResponseFailed, FirstWarningKind: model.GPTAbuseKindCyberPolicy, FirstWarningSeverity: model.GPTAbuseSeverityHigh, ChannelId: 81304, ChannelName: "Sub2API"}).Error)

	response, err := ListGPTAbuseRepeatBlocks(context.Background(), userID, dto.GPTAbuseRepeatBlockQuery{StartTimestamp: start, EndTimestamp: end, Limit: 10})

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Len(t, response.Items, 1)
	item := response.Items[0]
	assert.Equal(t, userID, item.UserID)
	assert.Equal(t, "0123456789ab", item.BodyFingerprintPrefix)
	assert.Equal(t, 81303, item.FirstWarningLogID)
	assert.Equal(t, start+30, item.FirstWarningAt)
	assert.Equal(t, "req-first-warning", item.FirstWarningRequestID)
	assert.Equal(t, "req-upstream-warning", item.FirstWarningUpstreamRequestID)
	assert.Equal(t, model.GPTAbuseSignalSourceSSEResponseFailed, item.FirstWarningSource)
	assert.Equal(t, model.GPTAbuseKindCyberPolicy, item.FirstWarningKind)
	assert.Equal(t, model.GPTAbuseSeverityHigh, item.FirstWarningSeverity)
	assert.Equal(t, 81304, item.ChannelID)
	assert.Equal(t, "Sub2API", item.ChannelName)
	assert.Equal(t, 81302, item.TokenID)
	assert.Equal(t, "admin-token", item.TokenName)
	encoded, err := common.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), fullFingerprint)
}

func TestResetGPTAbuseWarningsClearsSuspensionTransactionally(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81401
	adminID := 81499
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "reset-target", Status: common.UserStatusEnabled, AffCode: "aff-reset-target"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: adminID, Username: "reset-admin", Status: common.UserStatusEnabled, Role: common.RoleAdminUser, AffCode: "aff-reset-admin"}).Error)
	logs := []model.GPTAbuseSignalLog{
		{CreatedAt: start + 100, UserId: userID, Username: "reset-target", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-reset-1"},
		{CreatedAt: start + 110, UserId: userID, Username: "reset-target", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-reset-2"},
		{CreatedAt: start + 120, UserId: userID, Username: "reset-target", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-reset-3"},
	}
	for i := range logs {
		require.NoError(t, model.DB.Create(&logs[i]).Error)
	}
	activeUserID := userID
	require.NoError(t, model.DB.Create(&model.GPTAbuseUserSuspension{UserId: userID, ActiveUserId: &activeUserID, Status: model.GPTAbuseSuspensionStatusActive, Reason: "gpt_abuse_daily_limit", SuspendedUntil: end, TriggerLogId: logs[2].Id, DailyCount: 3, DailyLimit: 3}).Error)

	response, err := ResetGPTAbuseWarnings(context.Background(), adminID, userID, "  manual review  ", true)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, userID, response.UserID)
	assert.NotZero(t, response.ResetID)
	assert.Equal(t, start, response.WindowStart)
	assert.Equal(t, end, response.WindowEnd)
	assert.Equal(t, 3, response.PreviousRawCount)
	assert.Equal(t, 3, response.PreviousEffectiveCount)
	assert.Equal(t, 0, response.EffectiveWarningCount)
	assert.Equal(t, logs[2].Id, response.CutoffSignalLogID)
	assert.True(t, response.HadActiveSuspension)
	assert.True(t, response.SuspensionCleared)
	assert.NotZero(t, response.ClearedSuspensionID)

	var reset model.GPTAbuseWarningReset
	require.NoError(t, model.DB.First(&reset, response.ResetID).Error)
	assert.Equal(t, "manual review", reset.Reason)
	assert.Equal(t, adminID, reset.ResetBy)
	assert.Equal(t, logs[2].Id, reset.CutoffSignalLogID)

	var suspension model.GPTAbuseUserSuspension
	require.NoError(t, model.DB.First(&suspension, response.ClearedSuspensionID).Error)
	assert.Equal(t, model.GPTAbuseSuspensionStatusCleared, suspension.Status)
	assert.Nil(t, suspension.ActiveUserId)
	assert.Equal(t, adminID, suspension.ClearedBy)
	assert.NotZero(t, suspension.ClearedAt)

	var persisted []model.GPTAbuseSignalLog
	require.NoError(t, model.DB.Order("id asc").Find(&persisted, "user_id = ?", userID).Error)
	require.Len(t, persisted, 3)
	for _, log := range persisted {
		assert.True(t, log.CountEligible)
	}
}

func TestClearGPTAbuseSuspensionIsIdempotent(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	userID := 81501
	adminID := 81599
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "clear-no-active", Status: common.UserStatusEnabled, AffCode: "aff-clear-no-active"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: adminID, Username: "clear-admin", Status: common.UserStatusEnabled, Role: common.RoleAdminUser, AffCode: "aff-clear-admin"}).Error)

	response, err := ClearGPTAbuseSuspension(context.Background(), adminID, userID, "no active suspension")

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, userID, response.UserID)
	assert.False(t, response.HadActiveSuspension)
	assert.False(t, response.SuspensionCleared)
	assert.Zero(t, response.ClearedSuspensionID)
}

func TestClearGPTAbuseSuspensionDoesNotChangeWarningCounts(t *testing.T) {
	setupGPTAbuseAdminServiceTest(t)

	now := common.GetTimestamp()
	start, end := model.GPTAbuseDayWindow(now)
	userID := 81511
	adminID := 81599
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "clear-target", Status: common.UserStatusEnabled, AffCode: "aff-clear-target"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: adminID, Username: "clear-admin", Status: common.UserStatusEnabled, Role: common.RoleAdminUser, AffCode: "aff-clear-admin-2"}).Error)
	require.NoError(t, model.DB.Create(&[]model.GPTAbuseSignalLog{
		{CreatedAt: start + 100, UserId: userID, Username: "clear-target", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-clear-count-1"},
		{CreatedAt: start + 110, UserId: userID, Username: "clear-target", Source: model.GPTAbuseSignalSourceHTTPError, Kind: model.GPTAbuseKindCyberPolicy, Severity: model.GPTAbuseSeverityHigh, CountEligible: true, DedupeKey: "admin-clear-count-2"},
	}).Error)
	activeUserID := userID
	require.NoError(t, model.DB.Create(&model.GPTAbuseUserSuspension{UserId: userID, ActiveUserId: &activeUserID, Status: model.GPTAbuseSuspensionStatusActive, Reason: "gpt_abuse_daily_limit", SuspendedUntil: end, DailyCount: 2, DailyLimit: 2}).Error)

	response, err := ClearGPTAbuseSuspension(context.Background(), adminID, userID, "manual review")

	require.NoError(t, err)
	assert.True(t, response.SuspensionCleared)
	raw, err := model.CountGPTAbuseSignalsForUserRaw(userID, start, end)
	require.NoError(t, err)
	effective, _, err := model.CountEffectiveGPTAbuseSignalsForUser(userID, start, end)
	require.NoError(t, err)
	assert.Equal(t, 2, raw)
	assert.Equal(t, 2, effective)
}
