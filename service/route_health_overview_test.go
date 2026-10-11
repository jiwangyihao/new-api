package service

import (
	"context"
	"encoding/json"
	"fmt"
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

func overviewRow(t *testing.T, report *RouteHealthReport, channelID int, upstream string) RouteHealthRow {
	t.Helper()
	var matches []RouteHealthRow
	for _, row := range report.Rows {
		if row.ChannelID == channelID && row.UpstreamModel == upstream {
			matches = append(matches, row)
		}
	}
	require.Len(t, matches, 1, "expected one inventory row for channel %d model %q", channelID, upstream)
	return matches[0]
}

func overviewSelections(t *testing.T, channel *model.Channel, alias, path string) []routeSelection {
	t.Helper()
	selections, err := buildRouteSelections(&gin.Context{}, []*model.Channel{channel}, "default", alias, path, 0)
	require.NoError(t, err)
	require.NotEmpty(t, selections)
	return selections
}

func overviewFinish(t *testing.T, manager *routehealth.Manager, policy routehealth.Policy, selection routeSelection, result routehealth.Result) {
	t.Helper()
	lease, _, err := manager.Acquire(context.Background(), []routehealth.Candidate{selection.candidate}, policy)
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.NoError(t, lease.Finish(context.Background(), result))
}

func TestRouteHealthOverviewInventoryAndScopeIsolation(t *testing.T) {
	ctx := context.Background()
	manager := routehealth.New(nil)
	policy := routehealth.DefaultPolicy()
	policy.FailureThreshold = 1
	mapping := `{"alias-a":"actual","alias-b":"actual"}`
	channel := &model.Channel{Id: 9751, Type: constant.ChannelTypeCodex, Name: "overview", Key: "key-a\nkey-a\nkey-b", Models: "alias-b,alias-a,other,alias-b", ModelMapping: &mapping, Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyPollingIndex: 2}}
	original := *channel
	report := func() *RouteHealthReport {
		got, err := buildRouteHealthOverview(ctx, []*model.Channel{channel}, manager, policy, time.Now(), false, 7)
		require.NoError(t, err)
		return got
	}
	initial := report()
	require.Len(t, initial.Rows, 2)
	assert.Equal(t, 7, initial.RetryTimes)
	assert.Equal(t, 8, initial.MaxAttempts)
	row := overviewRow(t, initial, channel.Id, "actual")
	assert.Equal(t, "9751/model/actual", row.ID)
	assert.Equal(t, []string{"alias-a", "alias-b"}, row.Models)
	assert.Equal(t, "unobserved", row.State)
	assert.Equal(t, 2, row.TotalCredentials)
	assert.Equal(t, 2, row.ReadyCredentials)
	assert.NotNil(t, row.Attempts)
	assert.Empty(t, row.Attempts)
	require.Len(t, row.Resources, 4)
	assert.Equal(t, "channel", row.Resources[0].Scope)
	assert.Equal(t, "model", row.Resources[1].Scope)
	assert.Equal(t, "credential", row.Resources[2].Scope)
	assert.Equal(t, 1, row.Resources[2].CredentialIndex)
	assert.Equal(t, 3, row.Resources[3].CredentialIndex)
	assert.Equal(t, original, *channel, "inventory must not mutate source models, status, or polling cursor")
	selections := overviewSelections(t, channel, "alias-a", "/v1/responses")
	require.Len(t, selections, 2)
	overviewFinish(t, manager, policy, selections[0], routehealth.Result{Verdict: routehealth.VerdictSuccess})
	assert.Equal(t, "healthy", overviewRow(t, report(), channel.Id, "actual").State)
	assert.Equal(t, "unobserved", overviewRow(t, report(), channel.Id, "other").State, "shared success is not model evidence")
	overviewFinish(t, manager, policy, selections[0], routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeCredential})
	row = overviewRow(t, report(), channel.Id, "actual")
	assert.Equal(t, "healthy", row.State)
	assert.Equal(t, 1, row.ReadyCredentials)
	assert.Equal(t, "cooling", row.Resources[2].State)
	assert.Zero(t, row.Resources[0].Failures)
	assert.Zero(t, row.Resources[1].Failures)
	overviewFinish(t, manager, policy, selections[1], routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeModel})
	row = overviewRow(t, report(), channel.Id, "actual")
	assert.Equal(t, "cooling", row.State)
	assert.Zero(t, row.ReadyCredentials)
	assert.Equal(t, uint64(1), row.Resources[1].Failures)
	other := overviewSelections(t, channel, "other", "/v1/responses")
	overviewFinish(t, manager, policy, other[1], routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeChannel})
	row = overviewRow(t, report(), channel.Id, "other")
	assert.Equal(t, "cooling", row.State)
	assert.Equal(t, uint64(1), row.Resources[0].Failures)
	assert.Zero(t, row.Resources[1].Failures)
	channel.Status = common.ChannelStatusManuallyDisabled
	row = overviewRow(t, report(), channel.Id, "actual")
	assert.Equal(t, "manually_disabled", row.State)
	assert.Zero(t, row.ReadyCredentials)
	assert.Equal(t, 2, row.TotalCredentials)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, channel.Status)
	policy.Enabled = false
	assert.Equal(t, "manually_disabled", overviewRow(t, report(), channel.Id, "actual").State)
	channel.Status = common.ChannelStatusEnabled
	row = overviewRow(t, report(), channel.Id, "actual")
	assert.Equal(t, "protection_disabled", row.State)
	assert.Equal(t, 2, row.ReadyCredentials)
	assert.Empty(t, row.Attempts)
	for _, resource := range row.Resources {
		assert.Zero(t, resource.Successes)
		assert.Zero(t, resource.Failures)
		assert.Zero(t, resource.Inflight)
	}
}

func TestRouteHealthOverviewModelCooldownCoversEveryOperation(t *testing.T) {
	for _, failedOperation := range []string{"/v1/responses", "/v1/chat/completions"} {
		t.Run(failedOperation, func(t *testing.T) {
			ctx := context.Background()
			manager := routehealth.New(nil)
			policy := routehealth.DefaultPolicy()
			policy.FailureThreshold = 1
			mapping := `{"alias-a":"actual","alias-b":"actual"}`
			channel := &model.Channel{Id: 9760, Type: constant.ChannelTypeOpenAI, Key: "key", Models: "alias-b,other,alias-a", ModelMapping: &mapping, Status: common.ChannelStatusEnabled,
				OtherSettings: `{"supported_endpoint_types":["openai","openai-response","openai-response-compact"]}`}
			otherChannel := *channel
			otherChannel.Id++
			failed := overviewSelections(t, channel, "alias-a", failedOperation)[0]
			overviewFinish(t, manager, policy, failed, routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeModel})
			for _, operation := range []string{"/v1/responses", "/v1/responses/compact", "/v1/chat/completions"} {
				selection := overviewSelections(t, channel, "alias-b", operation)[0]
				lease, wait, err := manager.Acquire(ctx, []routehealth.Candidate{selection.candidate}, policy)
				require.NoError(t, err)
				assert.Nil(t, lease, "operation %s must not bypass model cooldown", operation)
				assert.Positive(t, wait)
			}
			overviewFinish(t, manager, policy, overviewSelections(t, channel, "other", "/v1/responses")[0], routehealth.Result{Verdict: routehealth.VerdictSuccess})
			overviewFinish(t, manager, policy, overviewSelections(t, &otherChannel, "alias-b", "/v1/responses/compact")[0], routehealth.Result{Verdict: routehealth.VerdictSuccess})
			report, err := buildRouteHealthOverview(ctx, []*model.Channel{&otherChannel, channel}, manager, policy, time.Now(), false, 2)
			require.NoError(t, err)
			require.Len(t, report.Rows, 4)
			assert.Equal(t, channel.Id, report.Rows[0].ChannelID)
			assert.Equal(t, "cooling", overviewRow(t, report, channel.Id, "actual").State)
			assert.Equal(t, "healthy", overviewRow(t, report, channel.Id, "other").State)
			assert.Equal(t, "healthy", overviewRow(t, report, otherChannel.Id, "actual").State)
			assert.Equal(t, "unobserved", overviewRow(t, report, otherChannel.Id, "other").State)
		})
	}
}

func TestRouteHealthOverviewProjectsOnlyCompletedModelAttempts(t *testing.T) {
	manager := routehealth.New(nil)
	policy := routehealth.DefaultPolicy()
	mapping := `{"alias-a":"actual","alias-b":"actual"}`
	channel := &model.Channel{Id: 9762, Type: constant.ChannelTypeCodex, Key: "secret-a\nsecret-a\nsecret-b", Models: "alias-a,alias-b,unrun", ModelMapping: &mapping, Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
	started := time.Now().Add(-time.Second)
	want := make([]RouteHealthAttempt, 0, 10)
	for index := 1; index <= 10; index++ {
		alias, operation := "alias-a", "/v1/responses"
		if index%2 == 0 {
			alias, operation = "alias-b", "/v1/responses/compact"
		}
		selection := overviewSelections(t, channel, alias, operation)[index%2]
		result := routehealth.Result{Verdict: routehealth.VerdictSuccess, Scope: routehealth.ScopeModel, Reason: "upstream_success"}
		status := 200
		if index%3 == 0 {
			result.Verdict, result.Reason, status = routehealth.VerdictFailure, "upstream_server_error", 503
		} else if index%3 == 1 {
			result.Verdict, result.Reason, status = routehealth.VerdictIgnored, "client_cancelled", 0
		}
		attempt := &routehealth.Attempt{RequestID: fmt.Sprintf("request-%d", index), Index: index, ChannelID: channel.Id, MappedModel: selection.mappedModel,
			Operation: selection.operation, CredentialIndex: selection.index + 1, StatusCode: status, StartedAt: started.Unix(), CompletedAt: started.Add(25 * time.Millisecond).Unix(), DurationMS: 25}
		result.Attempt = attempt
		overviewFinish(t, manager, policy, selection, result)
		want = append(want, RouteHealthAttempt{RequestID: attempt.RequestID, Index: index, ChannelID: channel.Id, MappedModel: "actual", Operation: operation,
			CredentialIndex: selection.index + 1, Verdict: string(result.Verdict), Scope: string(result.Scope), Reason: result.Reason, StatusCode: status,
			StartedAt: attempt.StartedAt, CompletedAt: attempt.CompletedAt, DurationMS: 25})
	}
	report, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, manager, policy, time.Now(), false, 2)
	require.NoError(t, err)
	require.Len(t, report.Rows, 2)
	row := overviewRow(t, report, channel.Id, "actual")
	assert.Equal(t, want[2:], row.Attempts, "only the latest eight real completions belong to this model")
	unrun := overviewRow(t, report, channel.Id, "unrun")
	assert.NotNil(t, unrun.Attempts)
	assert.Empty(t, unrun.Attempts, "endpoint capabilities must not create audit records")
	encoded, err := common.Marshal(report)
	require.NoError(t, err)
	var payload struct {
		Rows []map[string]json.RawMessage `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(encoded, &payload))
	for _, item := range payload.Rows {
		assert.NotContains(t, item, "operation")
		assert.Contains(t, item, "attempts")
		assert.NotEqual(t, "null", string(item["attempts"]))
	}
	for _, secret := range []string{"secret-a", "secret-b", RouteConfigurationFingerprint(channel), RouteCredentialFingerprint("secret-a"), RouteCredentialFingerprint("secret-b")} {
		assert.NotContains(t, string(encoded), secret)
	}
	for _, selection := range overviewSelections(t, channel, "alias-a", "/v1/responses") {
		assert.NotContains(t, string(encoded), selection.candidate.ID)
		for _, resource := range selection.candidate.Resources {
			assert.NotContains(t, string(encoded), resource.Key)
		}
	}
}

func TestRouteHealthOverviewConfiguredCredentialInventory(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  map[int]int
		indices []int
		state   string
	}{
		{"duplicate", nil, []int{1, 3}, "unobserved"},
		{"disabled first duplicate", map[int]int{0: common.ChannelStatusManuallyDisabled}, []int{2, 3}, "unobserved"},
		{"disabled distinct key", map[int]int{2: common.ChannelStatusManuallyDisabled}, []int{1}, "unobserved"},
		{"all disabled", map[int]int{0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusManuallyDisabled, 2: common.ChannelStatusManuallyDisabled}, []int{}, "no_credentials"},
	} {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{Id: 9750, Type: constant.ChannelTypeCodex, Name: "credential-inventory", Key: "key-a\nkey-a\nkey-b", Models: "model", Status: common.ChannelStatusEnabled,
				ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyStatusList: test.status, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyPollingIndex: 2}}
			before, err := common.Marshal(channel)
			require.NoError(t, err)
			report, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, routehealth.New(nil), routehealth.DefaultPolicy(), time.Now(), false, 2)
			require.NoError(t, err)
			row := overviewRow(t, report, channel.Id, "model")
			assert.Equal(t, test.state, row.State)
			assert.Equal(t, 2, row.TotalCredentials)
			assert.Equal(t, len(test.indices), row.ReadyCredentials)
			require.Len(t, row.Resources, 2+len(test.indices))
			for index, credentialIndex := range test.indices {
				assert.Equal(t, credentialIndex, row.Resources[index+2].CredentialIndex)
			}
			after, err := common.Marshal(channel)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestRouteHealthOverviewMappingAndInvalidInventory(t *testing.T) {
	policy := routehealth.DefaultPolicy()
	manager := routehealth.New(nil)
	mapping := `{"alias":"upstream/actual"}`
	channel := &model.Channel{Id: 9752, Type: constant.ChannelTypeCodex, Name: "inventory", Models: "alias", Key: "secret-credential", Status: common.ChannelStatusEnabled, ModelMapping: &mapping}
	build := func() *RouteHealthReport {
		got, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, manager, policy, time.Now(), false, 2)
		require.NoError(t, err)
		return got
	}
	got := build()
	row := overviewRow(t, got, channel.Id, "upstream/actual")
	assert.Equal(t, "9752/model/upstream%2Factual", row.ID)
	assert.Equal(t, "unobserved", row.State)
	settings := channel.GetSetting()
	settings.PassThroughBodyEnabled = true
	channel.SetSetting(settings)
	assert.Equal(t, "alias", build().Rows[0].UpstreamModel)
	channel.Setting = nil
	mapping = `{"alias":"other","other":"alias"}`
	row = build().Rows[0]
	assert.Equal(t, "invalid_mapping", row.State)
	assert.Equal(t, "9752/invalid/alias", row.ID)
	assert.Zero(t, row.ReadyCredentials)
	assert.NotNil(t, row.Attempts)
	assert.Empty(t, row.Attempts)
	mapping = `{"alias":"upstream/actual"}`
	for _, malformed := range []string{`{invalid`, `[]`} {
		channel.Setting = &malformed
		assert.Equal(t, "invalid_mapping", build().Rows[0].State)
		assert.Equal(t, malformed, *channel.Setting, "inspection must not repair or persist settings")
		channel.Setting = nil
		channel.OtherSettings = malformed
		assert.Equal(t, "invalid_mapping", build().Rows[0].State)
		assert.Equal(t, malformed, channel.OtherSettings)
		channel.OtherSettings = ""
	}
	channel.OtherSettings = `{"supported_endpoint_types":["custom-unknown"]}`
	row = build().Rows[0]
	assert.Equal(t, "invalid_mapping", row.State)
	assert.Zero(t, row.ReadyCredentials)
	assert.Equal(t, "9752/invalid/alias", row.ID)
	channel.OtherSettings = `{"supported_endpoint_types":["custom-unknown","openai-response"]}`
	assert.Equal(t, "unobserved", build().Rows[0].State)
	channel.OtherSettings = ""
	channel.Models = "alias,,alias"
	got = build()
	require.Len(t, got.Rows, 2)
	assert.Equal(t, []string{"alias"}, overviewRow(t, got, channel.Id, "upstream/actual").Models)
	invalid := overviewRow(t, got, channel.Id, "")
	assert.Equal(t, "invalid_mapping", invalid.State)
	assert.Equal(t, "9752/invalid/", invalid.ID)
	channel.Models = ""
	got = build()
	require.Len(t, got.Rows, 1)
	assert.Equal(t, "invalid_mapping", got.Rows[0].State)
	channel.Status = common.ChannelStatusManuallyDisabled
	assert.Equal(t, "manually_disabled", build().Rows[0].State)
	channel.Status = common.ChannelStatusEnabled
	channel.Models = "alias"
	channel.Key = ""
	assert.Equal(t, "no_credentials", build().Rows[0].State)
}

func TestRouteHealthOverviewSupportedCapabilitiesDoNotExpandRows(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeGemini, constant.ChannelTypeCodex, constant.ChannelTypeSora} {
		channel := &model.Channel{Id: 9753, Type: channelType, Key: "key", Models: "model", Status: common.ChannelStatusEnabled, Setting: common.GetPointer(`{"responses_websocket_enabled":true}`)}
		report, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, routehealth.New(nil), routehealth.DefaultPolicy(), time.Now(), false, 2)
		require.NoError(t, err)
		require.Len(t, report.Rows, 1)
		assert.Equal(t, "unobserved", report.Rows[0].State)
		assert.Equal(t, 1, report.Rows[0].ReadyCredentials)
		assert.Empty(t, report.Rows[0].Attempts)
	}
	global := model_setting.GetGlobalSettings()
	oldPassThrough := global.PassThroughRequestEnabled
	global.PassThroughRequestEnabled = true
	t.Cleanup(func() { global.PassThroughRequestEnabled = oldPassThrough })
	mapping := `{"alias":"different"}`
	channel := &model.Channel{Id: 9753, Type: constant.ChannelTypeCodex, Key: "key", Models: "alias", ModelMapping: &mapping, Status: common.ChannelStatusEnabled}
	report, err := buildRouteHealthOverview(context.Background(), []*model.Channel{channel}, routehealth.New(nil), routehealth.DefaultPolicy(), time.Now(), false, 2)
	require.NoError(t, err)
	assert.Equal(t, "alias", report.Rows[0].UpstreamModel)
}

func TestRouteHealthOverviewCredentialInvalidRecovery(t *testing.T) {
	for _, keys := range []string{"bad-key", "bad-key\ngood-key"} {
		t.Run(keys, func(t *testing.T) {
			ctx := context.Background()
			manager := routehealth.New(nil)
			policy := routehealth.DefaultPolicy()
			channel := &model.Channel{Id: 9763, Type: constant.ChannelTypeCodex, Key: keys, Models: "model", Status: common.ChannelStatusEnabled, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
			selections := overviewSelections(t, channel, "model", "/v1/responses")
			if len(selections) > 1 {
				overviewFinish(t, manager, policy, selections[1], routehealth.Result{Verdict: routehealth.VerdictSuccess})
			}
			overviewFinish(t, manager, policy, selections[0], routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeCredential, CredentialInvalid: true, Reason: "invalid_api_key"})
			resource := routeResources(channel).credential("bad-key")
			snapshot, err := manager.Inspect(ctx, resource, policy)
			require.NoError(t, err)
			require.True(t, snapshot.CredentialInvalid)
			build := func(at time.Time) RouteHealthRow {
				report, err := buildRouteHealthOverview(ctx, []*model.Channel{channel}, manager, policy, at, false, 2)
				require.NoError(t, err)
				return overviewRow(t, report, channel.Id, "model")
			}
			row := build(time.Now())
			assert.Equal(t, len(selections)-1, row.ReadyCredentials)
			if len(selections) == 1 {
				assert.Equal(t, "credential_invalid", row.State)
			} else {
				assert.Equal(t, "healthy", row.State, "a bad key cannot hide a normally usable key")
			}
			require.Len(t, row.Resources, 2+len(selections))
			bad := row.Resources[2]
			assert.Equal(t, "credential_invalid", bad.State)
			assert.True(t, bad.CredentialInvalid)
			assert.Equal(t, snapshot.LastFailureAt, bad.LastFailureAt)
			assert.Equal(t, "invalid_api_key", bad.LastFailureReason)
			assert.Equal(t, uint64(1), bad.Failures)
			assert.Zero(t, row.Resources[1].Failures, "credential failure is not a model failure")
			readyAt := snapshot.OpenedUntil
			if snapshot.NextProbe.After(readyAt) {
				readyAt = snapshot.NextProbe
			}
			recovered := build(readyAt)
			assert.Equal(t, "recovering", recovered.Resources[2].State)
			assert.True(t, recovered.Resources[2].CredentialInvalid, "quarantine evidence remains until successful probes")
			assert.Equal(t, len(selections), recovered.ReadyCredentials)
			if len(selections) == 1 {
				assert.Equal(t, "recovering", recovered.State)
			}
			after, err := manager.Inspect(ctx, resource, policy)
			require.NoError(t, err)
			assert.Equal(t, snapshot, after, "future inspection must not mutate state or acquire a probe")
		})
	}
}

func TestRouteHealthOverviewRecoveryAndSaturation(t *testing.T) {
	policy := routehealth.DefaultPolicy()
	now := time.Unix(1700000000, 0)
	for _, test := range []struct {
		snapshot routehealth.Snapshot
		state    string
		ready    bool
	}{
		{routehealth.Snapshot{}, "unobserved", true},
		{routehealth.Snapshot{Failures: 1}, "healthy", true},
		{routehealth.Snapshot{Inflight: policy.MaxInflight}, "saturated", false},
		{routehealth.Snapshot{Open: true, OpenedUntil: now.Add(time.Second)}, "cooling", false},
		{routehealth.Snapshot{Open: true, CredentialInvalid: true, OpenedUntil: now.Add(time.Second)}, "credential_invalid", false},
		{routehealth.Snapshot{Open: true, CredentialInvalid: true, OpenedUntil: now}, "recovering", true},
		{routehealth.Snapshot{Open: true, CredentialInvalid: true, NextProbe: now.Add(time.Second)}, "recovering", false},
		{routehealth.Snapshot{Open: true, CredentialInvalid: true, Probe: true}, "recovering", false},
		{routehealth.Snapshot{Open: true, NextProbe: now.Add(time.Second)}, "recovering", false},
		{routehealth.Snapshot{Open: true, Probe: true}, "recovering", false},
		{routehealth.Snapshot{Open: true, RecoverySuccesses: 1}, "recovering", true},
	} {
		assert.Equal(t, test.state, routeHealthDynamicState(test.snapshot, policy, now))
		assert.Equal(t, test.ready, routeHealthResourceReady(test.snapshot, policy, now))
	}
}

func TestRouteHealthOverviewInspectionDoesNotAcquireOrAdvance(t *testing.T) {
	ctx := context.Background()
	manager := routehealth.New(nil)
	policy := routehealth.DefaultPolicy()
	policy.MaxInflight = 1
	channel := &model.Channel{Id: 9764, Type: constant.ChannelTypeCodex, Key: "key-a\nkey-b", Models: "model", Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyPollingIndex: 1}}
	selection := overviewSelections(t, channel, "model", "/v1/responses")[0]
	lease, _, err := manager.Acquire(ctx, []routehealth.Candidate{selection.candidate}, policy)
	require.NoError(t, err)
	require.NotNil(t, lease)
	t.Cleanup(func() { require.NoError(t, lease.Finish(ctx, routehealth.Result{Verdict: routehealth.VerdictIgnored})) })
	at := time.Now()
	before, err := manager.InspectMany(ctx, selection.candidate.Resources, policy, at)
	require.NoError(t, err)
	first, err := buildRouteHealthOverview(ctx, []*model.Channel{nil, channel}, manager, policy, at, false, 2)
	require.NoError(t, err)
	second, err := buildRouteHealthOverview(ctx, []*model.Channel{channel}, manager, policy, at, false, 2)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	require.Len(t, first.Rows, 1)
	assert.Equal(t, "saturated", first.Rows[0].State)
	assert.Zero(t, first.Rows[0].ReadyCredentials)
	assert.Empty(t, first.Rows[0].Attempts, "an unfinished lease is not a completed attempt")
	after, err := manager.InspectMany(ctx, selection.candidate.Resources, policy, at)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, 1, channel.ChannelInfo.MultiKeyPollingIndex)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	report, err := buildRouteHealthOverview(cancelled, []*model.Channel{channel}, manager, policy, at, false, 2)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, report)
	oldRedis := common.RDB
	common.RDB = nil
	t.Cleanup(func() { common.RDB = oldRedis })
	report, err = buildRouteHealthOverview(ctx, []*model.Channel{channel}, manager, policy, at, true, 2)
	require.NoError(t, err)
	assert.Equal(t, "redis", report.Storage)
	assert.True(t, report.Degraded)
	policy.Enabled = false
	report, err = buildRouteHealthOverview(ctx, []*model.Channel{channel}, manager, policy, at, true, 2)
	require.NoError(t, err)
	assert.False(t, report.Degraded)
	assert.Equal(t, "protection_disabled", report.Rows[0].State)
}
