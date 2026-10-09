package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/routehealth"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func overviewRow(t *testing.T, report *RouteHealthReport, upstream, operation string) RouteHealthRow {
	t.Helper()
	for _, row := range report.Rows {
		if row.UpstreamModel == upstream && row.Operation == operation {
			return row
		}
	}
	t.Fatalf("missing inventory row %q %q", upstream, operation)
	return RouteHealthRow{}
}

func overviewSelections(t *testing.T, channel *model.Channel, alias, path string) []routeSelection {
	t.Helper()
	selections, err := buildRouteSelections(&gin.Context{}, []*model.Channel{channel}, "default", alias, path, 0)
	require.NoError(t, err)
	return selections
}

func TestRouteHealthOverviewInventoryAndScopeIsolation(t *testing.T) {
	ctx := context.Background()
	manager := routehealth.New(nil)
	policy := routehealth.DefaultPolicy()
	policy.FailureThreshold = 1
	mapping := `{"alias-a":"actual","alias-b":"actual"}`
	channel := &model.Channel{Id: 9751, Type: constant.ChannelTypeCodex, Name: "overview", Key: "key-a\nkey-a\nkey-b", Models: "alias-b,alias-a,other", ModelMapping: &mapping, Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
	report := func() *RouteHealthReport {
		got, err := buildRouteHealthOverview(ctx, []*model.Channel{channel}, manager, policy, time.Now(), false, 7)
		require.NoError(t, err)
		return got
	}
	initial := report()
	assert.Equal(t, 7, initial.RetryTimes)
	assert.Equal(t, 8, initial.MaxAttempts)
	row := overviewRow(t, initial, "actual", "/v1/responses")
	assert.Equal(t, []string{"alias-a", "alias-b"}, row.Models)
	assert.Equal(t, "unobserved", row.State)
	assert.Equal(t, 2, row.TotalCredentials)
	assert.Equal(t, 2, row.ReadyCredentials)
	require.Len(t, row.Resources, 4)
	assert.Equal(t, 1, row.Resources[2].CredentialIndex)
	assert.Equal(t, 3, row.Resources[3].CredentialIndex)
	selections := overviewSelections(t, channel, "alias-a", "/v1/responses")
	finish := func(selection routeSelection, result routehealth.Result) {
		lease, _, err := manager.Acquire(ctx, []routehealth.Candidate{selection.candidate}, policy)
		require.NoError(t, err)
		require.NotNil(t, lease)
		require.NoError(t, lease.Finish(ctx, result))
	}
	finish(selections[0], routehealth.Result{Verdict: routehealth.VerdictSuccess})
	assert.Equal(t, "healthy", overviewRow(t, report(), "actual", "/v1/responses").State)
	assert.Equal(t, "unobserved", overviewRow(t, report(), "other", "/v1/responses").State)
	finish(selections[0], routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeCredential})
	row = overviewRow(t, report(), "actual", "/v1/responses")
	assert.Equal(t, "healthy", row.State)
	assert.Equal(t, 1, row.ReadyCredentials)
	assert.Equal(t, "cooling", row.Resources[2].State)
	assert.Zero(t, row.Resources[0].Failures)
	assert.Zero(t, row.Resources[1].Failures)
	finish(selections[1], routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeRoute})
	row = overviewRow(t, report(), "actual", "/v1/responses")
	assert.Equal(t, "cooling", row.State)
	assert.Zero(t, row.ReadyCredentials)
	assert.Equal(t, uint64(1), row.Resources[1].Failures)
	assert.Equal(t, "unobserved", overviewRow(t, report(), "actual", "/v1/responses/compact").State)
	other := overviewSelections(t, channel, "other", "/v1/responses")
	finish(other[1], routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeChannel})
	assert.Equal(t, "cooling", overviewRow(t, report(), "actual", "/v1/responses/compact").State)
	channel.Status = common.ChannelStatusManuallyDisabled
	row = overviewRow(t, report(), "actual", "/v1/responses")
	assert.Equal(t, "manually_disabled", row.State)
	assert.Zero(t, row.ReadyCredentials)
	assert.Equal(t, 2, row.TotalCredentials)
	policy.Enabled = false
	assert.Equal(t, "manually_disabled", overviewRow(t, report(), "actual", "/v1/responses").State)
	channel.Status = common.ChannelStatusEnabled
	assert.Equal(t, "protection_disabled", overviewRow(t, report(), "actual", "/v1/responses").State)
}

func TestRouteHealthOverviewCountsDisabledConfiguredCredentials(t *testing.T) {
	channel := &model.Channel{Id: 9750, Type: constant.ChannelTypeCodex, Name: "credential-inventory", Key: "key-a\nkey-b", Models: "model", Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{1: common.ChannelStatusManuallyDisabled}}}
	report, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, routehealth.New(nil), routehealth.DefaultPolicy(), time.Now(), false, 2)
	require.NoError(t, err)
	row := overviewRow(t, report, "model", "/v1/responses")
	assert.Equal(t, 2, row.TotalCredentials)
	assert.Equal(t, 1, row.ReadyCredentials)
	require.Len(t, row.Resources, 3)
	assert.Equal(t, 1, row.Resources[2].CredentialIndex)
}

func TestRouteHealthOverviewReportsNoCredentialsWhenAllConfiguredKeysAreDisabled(t *testing.T) {
	channel := &model.Channel{Id: 9749, Type: constant.ChannelTypeCodex, Name: "all-disabled", Key: "key-a\nkey-b", Models: "model", Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusManuallyDisabled}}}
	report, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, routehealth.New(nil), routehealth.DefaultPolicy(), time.Now(), false, 2)
	require.NoError(t, err)
	row := overviewRow(t, report, "model", "/v1/responses")
	assert.Equal(t, "no_credentials", row.State)
	assert.Equal(t, 2, row.TotalCredentials)
	assert.Zero(t, row.ReadyCredentials)
	require.Len(t, row.Resources, 2)
}

func TestRouteHealthOverviewMappingAndSafeDTO(t *testing.T) {
	policy := routehealth.DefaultPolicy()
	manager := routehealth.New(nil)
	mapping := `{"alias":"upstream"}`
	channel := &model.Channel{Id: 9752, Type: constant.ChannelTypeCodex, Name: "inventory", Models: "alias", Key: "secret-credential", Status: common.ChannelStatusEnabled, ModelMapping: &mapping}
	build := func() *RouteHealthReport {
		got, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, manager, policy, time.Now(), false, 2)
		require.NoError(t, err)
		return got
	}
	got := build()
	assert.Equal(t, "unobserved", overviewRow(t, got, "upstream", "/v1/responses").State)
	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), channel.Key)
	assert.NotContains(t, string(encoded), RouteConfigurationFingerprint(channel))
	assert.NotContains(t, string(encoded), RouteCredentialFingerprint(channel.Key))
	settings := channel.GetSetting()
	settings.PassThroughBodyEnabled = true
	channel.SetSetting(settings)
	assert.Equal(t, "alias", build().Rows[0].UpstreamModel)
	channel.Setting = nil
	mapping = `{"alias":"other","other":"alias"}`
	for _, row := range build().Rows {
		assert.Equal(t, "invalid_mapping", row.State)
	}
	mapping = `{"alias":"upstream"}`
	malformed := `{invalid`
	channel.Setting = &malformed
	assert.Equal(t, "invalid_mapping", build().Rows[0].State)
	assert.Equal(t, malformed, *channel.Setting, "inspection must not repair or persist settings")
	channel.Setting = nil
	channel.OtherSettings = malformed
	assert.Equal(t, "invalid_mapping", build().Rows[0].State)
	assert.Equal(t, malformed, channel.OtherSettings)
	channel.OtherSettings = `{"supported_endpoint_types":["custom-unknown"]}`
	assert.Equal(t, "invalid_mapping", build().Rows[0].State)
	channel.OtherSettings = ""
	channel.Key = ""
	assert.Equal(t, "no_credentials", build().Rows[0].State)
}

func TestRouteHealthOverviewOperationsMatchAdmission(t *testing.T) {
	channel := &model.Channel{Id: 9753, Type: constant.ChannelTypeGemini, Key: "key", Models: "gemini-test", Status: common.ChannelStatusEnabled, Setting: common.GetPointer(`{"responses_websocket_enabled":true}`)}
	manager := routehealth.New(nil)
	policy := routehealth.DefaultPolicy()
	report, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, manager, policy, time.Now(), false, 2)
	require.NoError(t, err)
	for _, action := range []string{"generateContent", "streamGenerateContent", "embedContent", "batchEmbedContents", "predict"} {
		path := "/v1beta/models/gemini-test:" + action
		row := overviewRow(t, report, "gemini-test", routeOperation(nil, channel, "gemini-test", path))
		assert.Equal(t, "unobserved", row.State)
		assert.NotContains(t, row.Operation, "?")
	}
	channel.Type = constant.ChannelTypeCodex
	report, err = buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, manager, policy, time.Now(), false, 2)
	require.NoError(t, err)
	websocket := &gin.Context{Request: &http.Request{Method: http.MethodGet}}
	assert.Equal(t, "unobserved", overviewRow(t, report, "gemini-test", routeOperation(websocket, channel, "gemini-test", "/v1/responses")).State)
	channel.Type = constant.ChannelTypeSora
	report, err = buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, manager, policy, time.Now(), false, 2)
	require.NoError(t, err)
	require.Len(t, report.Rows, 2)
	assert.Equal(t, "unobserved", overviewRow(t, report, "gemini-test", "/v1/videos").State)
	assert.Equal(t, "unobserved", overviewRow(t, report, "gemini-test", "/v1/video/generations").State)
	global := model_setting.GetGlobalSettings()
	oldPassThrough := global.PassThroughRequestEnabled
	global.PassThroughRequestEnabled = true
	t.Cleanup(func() { global.PassThroughRequestEnabled = oldPassThrough })
	mapping := `{"gemini-test":"different"}`
	channel.ModelMapping = &mapping
	report, err = buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, manager, policy, time.Now(), false, 2)
	require.NoError(t, err)
	assert.Equal(t, "gemini-test", report.Rows[0].UpstreamModel)
}

func TestRouteHealthOverviewRecoveryAndSaturation(t *testing.T) {
	policy := routehealth.DefaultPolicy()
	now := time.Now()
	for _, test := range []struct {
		snapshot routehealth.Snapshot
		state    string
		ready    bool
	}{
		{routehealth.Snapshot{}, "unobserved", true},
		{routehealth.Snapshot{Failures: 1}, "healthy", true},
		{routehealth.Snapshot{Inflight: policy.MaxInflight}, "saturated", false},
		{routehealth.Snapshot{Open: true, OpenedUntil: now.Add(time.Second)}, "cooling", false},
		{routehealth.Snapshot{Open: true, NextProbe: now.Add(time.Second)}, "recovering", false},
		{routehealth.Snapshot{Open: true, Probe: true}, "recovering", false},
		{routehealth.Snapshot{Open: true, RecoverySuccesses: 1}, "recovering", true},
	} {
		assert.Equal(t, test.state, routeHealthDynamicState(test.snapshot, policy, now))
		assert.Equal(t, test.ready, routeHealthResourceReady(test.snapshot, policy, now))
	}
}

func TestRouteHealthOverviewChatConversionSharesResponses(t *testing.T) {
	global := model_setting.GetGlobalSettings()
	old := global.ChatCompletionsToResponsesPolicy
	global.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: true, AllChannels: true, ModelPatterns: []string{".*"}}
	t.Cleanup(func() { global.ChatCompletionsToResponsesPolicy = old })
	channel := &model.Channel{Id: 9754, Type: constant.ChannelTypeOpenAI, Key: "key", Models: "chat-model", Status: common.ChannelStatusEnabled,
		OtherSettings: `{"supported_endpoint_types":["openai","openai-response"]}`}
	manager := routehealth.New(nil)
	policy := routehealth.DefaultPolicy()
	selection := overviewSelections(t, channel, "chat-model", "/v1/chat/completions")[0]
	lease, _, err := manager.Acquire(context.Background(), []routehealth.Candidate{selection.candidate}, policy)
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.NoError(t, lease.Finish(context.Background(), routehealth.Result{Verdict: routehealth.VerdictSuccess}))
	report, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, manager, policy, time.Now(), false, 2)
	require.NoError(t, err)
	row := overviewRow(t, report, "chat-model", "/v1/responses")
	assert.Equal(t, "healthy", row.State)
	assert.Equal(t, []string{"chat-model"}, row.Models)
	for _, row := range report.Rows {
		assert.NotEqual(t, "/v1/chat/completions", row.Operation)
	}
}
