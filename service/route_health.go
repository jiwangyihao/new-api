package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/routehealth"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const routeAttemptContextKey = "route_health_attempts"

type routeSelection struct {
	channel   *model.Channel
	group     string
	key       string
	index     int
	candidate routehealth.Candidate
}

type routeAttemptState struct {
	started            time.Time
	selected           *routeSelection
	lease              *routehealth.Lease
	excluded           map[string]struct{}
	excludedCandidates map[string]struct{}
	attempted          map[int]bool
	cancel             context.CancelFunc
	done               chan struct{}
	startedUpstream    bool
	upstreamStatus     int
	retryAfter         time.Duration
	events             []map[string]any
}

var routeManagers struct {
	sync.Mutex
	client  *redis.Client
	manager *routehealth.Manager
}

func routeManager() *routehealth.Manager {
	var client *redis.Client
	if common.RedisEnabled {
		client = common.RDB
	}
	routeManagers.Lock()
	defer routeManagers.Unlock()
	if routeManagers.manager == nil || routeManagers.client != client {
		routeManagers.client, routeManagers.manager = client, routehealth.New(client)
	}
	return routeManagers.manager
}

func routeState(c *gin.Context) *routeAttemptState {
	if value, found := c.Get(routeAttemptContextKey); found {
		return value.(*routeAttemptState)
	}
	state := &routeAttemptState{
		started:            time.Now(),
		excluded:           map[string]struct{}{},
		excludedCandidates: map[string]struct{}{},
		attempted:          map[int]bool{},
	}
	c.Set(routeAttemptContextKey, state)
	return state
}

func routeFingerprint(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func routeConfigValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func RouteCredentialFingerprint(key string) string { return routeFingerprint(key) }

func RouteConfigurationFingerprint(channel *model.Channel) string {
	return routeFingerprint(strconv.Itoa(channel.Type), routeConfigValue(channel.OpenAIOrganization), channel.GetBaseURL(), channel.Other, channel.OtherSettings, channel.GetModelMapping(), channel.Key, routeConfigValue(channel.Setting), routeConfigValue(channel.ParamOverride), routeConfigValue(channel.HeaderOverride))
}

func routeOperation(c *gin.Context, channel *model.Channel, modelName, path string) string {
	if strings.Contains(path, "/models/") {
		if _, operation, found := strings.Cut(path, ":"); found {
			return "gemini:" + operation
		}
	}
	if path == "/v1/chat/completions" && ShouldChatCompletionsUseResponsesGlobal(channel.Id, channel.Type, modelName) {
		return "/v1/responses"
	}
	if c != nil && path == "/v1/responses" && (c.GetBool("relay_native_responses_websocket") || c.Request != nil && c.Request.Method == http.MethodGet) {
		return "websocket:/v1/responses"
	}
	return path
}

func buildRouteSelections(c *gin.Context, channels []*model.Channel, group, modelName, path string, preferred int) ([]routeSelection, error) {
	state := routeState(c)
	selections := make([]routeSelection, 0, len(channels))
	for _, channel := range channels {
		if channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		mapped, _, err := common.ResolveModelMapping(modelName, channel.GetModelMapping(), nil)
		if err != nil {
			return nil, err
		}
		if model_setting.GetGlobalSettings().PassThroughRequestEnabled || channel.GetSetting().PassThroughBodyEnabled {
			mapped = modelName
		}
		configVersion := RouteConfigurationFingerprint(channel)
		prefix := fmt.Sprintf("route-health:{%d}:%s", channel.Id, configVersion)
		channelResource := routehealth.Resource{Key: prefix + ":channel", Scope: routehealth.ScopeChannel}
		routeResource := routehealth.Resource{Key: prefix + ":route:" + routeFingerprint(mapped, routeOperation(c, channel, modelName, path)), Scope: routehealth.ScopeRoute}
		credentials := channel.EnabledCredentials()
		preferredKey := channel.PreviewNextEnabledKey()
		start := 0
		for i, credential := range credentials {
			if credential.Key == preferredKey {
				start = i
				break
			}
		}
		seenCredentials := make(map[string]struct{}, len(credentials))
		for i, credential := range credentials {
			if _, duplicate := seenCredentials[credential.Key]; duplicate {
				continue
			}
			seenCredentials[credential.Key] = struct{}{}
			candidateID := routeResource.Key + ":" + routeFingerprint(credential.Key)
			keyResource := routehealth.Resource{Key: prefix + ":credential:" + routeFingerprint(credential.Key), Scope: routehealth.ScopeCredential}
			resources := []routehealth.Resource{channelResource, keyResource, routeResource}
			if _, blocked := state.excludedCandidates[candidateID]; blocked {
				continue
			}
			if slices.ContainsFunc(resources, func(resource routehealth.Resource) bool {
				_, blocked := state.excluded[resource.Key]
				return blocked
			}) {
				continue
			}
			if slices.Contains(c.GetStringSlice("use_channel"), strconv.Itoa(channel.Id)) && !state.attempted[channel.Id] {
				continue
			}
			candidate := routehealth.Candidate{
				ID:        candidateID,
				Resources: resources,
				Priority:  channel.GetPriority(),
				Weight:    channel.GetWeight(),
				Pool:      routeResource.Key,
				Preferred: channel.Id == preferred,
			}
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				candidate.CredentialOrder = (i - start + len(credentials)) % len(credentials)
			} else if channel.ChannelInfo.MultiKeyMode != constant.MultiKeyModeRandom {
				candidate.CredentialOrder = i
			}
			selections = append(selections, routeSelection{channel: channel, group: group, key: credential.Key, index: credential.Index, candidate: candidate})
		}
	}
	return selections, nil
}

func acquireRoute(c *gin.Context, selections []routeSelection) (*model.Channel, string, error) {
	state := routeState(c)
	if state.lease != nil {
		ReleaseRouteAttempt(c)
	}
	candidates := make([]routehealth.Candidate, len(selections))
	for i := range selections {
		candidates[i] = selections[i].candidate
	}
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	policy := operation_setting.GetRouteHealthSetting().Policy()
	lease, retryAfter, err := routeManager().Acquire(ctx, candidates, policy)
	if err != nil {
		return nil, "", err
	}
	if lease == nil {
		if retryAfter > 0 && c.Writer != nil {
			c.Header("Retry-After", strconv.FormatInt(max(1, int64((retryAfter+time.Second-1)/time.Second)), 10))
		}
		return nil, "", nil
	}
	for i := range selections {
		selected := &selections[i]
		if selected.candidate.ID != lease.CandidateID {
			continue
		}
		state.selected, state.lease = selected, lease
		state.startedUpstream, state.upstreamStatus, state.retryAfter = false, 0, 0
		state.cancel, state.done = nil, nil
		if c.Request != nil {
			leaseCtx, cancel := context.WithCancel(c.Request.Context())
			state.cancel, state.done = cancel, make(chan struct{})
			done := state.done
			go func() {
				defer close(done)
				ticker := time.NewTicker(max(time.Second, policy.LeaseDuration/3))
				defer ticker.Stop()
				for {
					select {
					case <-leaseCtx.Done():
						return
					case <-ticker.C:
						if err := lease.Renew(leaseCtx); err != nil {
							return
						}
					}
				}
			}()
		}
		health := "available"
		if lease.Probe {
			health = "recovery_probe"
		}
		if lease.Degraded {
			health = "local_protection"
		}
		state.events = append(state.events, map[string]any{"channel_id": selected.channel.Id, "group": selected.group, "health": health, "action": "admit"})
		return selected.channel, selected.group, nil
	}
	_ = lease.Finish(ctx, routehealth.Result{Verdict: routehealth.VerdictIgnored})
	return nil, "", errors.New("route health returned an unknown candidate")
}

func SelectedRouteCredential(c *gin.Context, channelID int) (string, int, bool) {
	value, found := c.Get(routeAttemptContextKey)
	if !found {
		return "", 0, false
	}
	state := value.(*routeAttemptState)
	if state.selected == nil || state.selected.channel.Id != channelID || state.lease == nil {
		return "", 0, false
	}
	return state.selected.key, state.selected.index, true
}

func AdmitBoundRoute(c *gin.Context, channel *model.Channel, modelName, group, key string) *types.NewAPIError {
	selected, _, err := SelectRouteForChannel(c, channel, &RetryParam{Ctx: c, TokenGroups: []string{group}, ModelName: modelName, RequiredCredential: key})
	if err != nil {
		var apiErr *types.NewAPIError
		if errors.As(err, &apiErr) {
			types.ErrOptionWithSkipRetry()(apiErr)
			return apiErr
		}
		return types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if selected == nil {
		return routeUnavailableError()
	}
	return nil
}

func RouteRetryBudgetAvailable(c *gin.Context) bool {
	state := routeState(c)
	return (c.Request == nil || c.Request.Context().Err() == nil) && time.Since(state.started) < time.Duration(operation_setting.GetRouteHealthSetting().RetryBudgetSeconds)*time.Second
}

func MarkRouteUpstreamStarted(c *gin.Context) {
	if c == nil {
		return
	}
	if value, found := c.Get(routeAttemptContextKey); found {
		value.(*routeAttemptState).startedUpstream = true
	}
}

func ObserveRouteHTTPResponse(c *gin.Context, response *http.Response) {
	if c == nil || response == nil {
		return
	}
	if value, found := c.Get(routeAttemptContextKey); found {
		state := value.(*routeAttemptState)
		state.upstreamStatus = response.StatusCode
		if seconds, err := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 64); err == nil && seconds > 0 {
			state.retryAfter = time.Duration(min(seconds, 86400)) * time.Second
		} else if deadline, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
			state.retryAfter = min(max(time.Until(deadline), 0), 24*time.Hour)
		}
	}
}

func FinishRouteAttempt(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) {
	if c == nil {
		return
	}
	value, found := c.Get(routeAttemptContextKey)
	if !found {
		return
	}
	state := value.(*routeAttemptState)
	if state.lease == nil {
		return
	}
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	result := ClassifyRouteAttempt(ctx, info, apiErr, state.startedUpstream)
	result.RetryAfter = state.retryAfter
	if result.Verdict == routehealth.VerdictFailure && state.selected != nil {
		state.attempted[state.selected.channel.Id] = true
		state.excludedCandidates[state.selected.candidate.ID] = struct{}{}
		for _, resource := range state.selected.candidate.Resources {
			if resource.Scope == result.Scope {
				state.excluded[resource.Key] = struct{}{}
			}
		}
	}
	finishRouteLease(state, result)
	channelID := 0
	if state.selected != nil {
		channelID = state.selected.channel.Id
	}
	state.events = append(state.events, map[string]any{"channel_id": channelID, "health": string(result.Verdict), "reason": result.Reason, "action": "observe"})
}

func ReleaseRouteAttempt(c *gin.Context) {
	if c == nil {
		return
	}
	if value, found := c.Get(routeAttemptContextKey); found {
		finishRouteLease(value.(*routeAttemptState), routehealth.Result{Verdict: routehealth.VerdictIgnored})
	}
}

func finishRouteLease(state *routeAttemptState, result routehealth.Result) {
	if state.cancel != nil {
		state.cancel()
		<-state.done
		state.cancel, state.done = nil, nil
	}
	if state.lease == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := state.lease.Finish(ctx, result); err != nil {
		common.SysError("route health result: " + err.Error())
	}
	state.lease = nil
}

func ClassifyRouteAttempt(ctx context.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError, upstreamStarted bool) routehealth.Result {
	ignored := routehealth.Result{Verdict: routehealth.VerdictIgnored, Scope: routehealth.ScopeRoute, Reason: "not_upstream_failure"}
	if !upstreamStarted || (ctx != nil && errors.Is(ctx.Err(), context.Canceled)) {
		return ignored
	}
	status, code, errorType := 0, "", ""
	if apiErr != nil {
		root := apiErr
		for {
			var inner *types.NewAPIError
			if !errors.As(root.Unwrap(), &inner) || inner == root {
				break
			}
			root = inner
		}
		status, code, errorType = root.StatusCode, string(root.GetErrorCode()), root.ToOpenAIError().Type
		if root.IsLocal() && root.GetErrorCode() != types.ErrorCodeDoRequestFailed && root.GetErrorCode() != types.ErrorCodeBadResponse && root.GetErrorCode() != types.ErrorCodeBadResponseBody && root.GetErrorCode() != types.ErrorCodeReadResponseBodyFailed && root.GetErrorCode() != types.ErrorCodeBadResponseStatusCode && root.GetErrorCode() != types.ErrorCodeEmptyResponse {
			return ignored
		}
	}
	if info != nil && info.StreamStatus != nil {
		stream := info.StreamStatus.OutcomeSnapshot()
		if info.StreamStatus.AvailabilityIncomplete() && stream.Response != relaycommon.ResponseOutcomeFailed {
			return ignored
		}
		if stream.EndReason == relaycommon.StreamEndReasonClientGone || stream.EndReason == relaycommon.StreamEndReasonPingFail || stream.Response == relaycommon.ResponseOutcomeCancelled {
			return ignored
		}
		if stream.Response == relaycommon.ResponseOutcomeFailed {
			code, errorType = stream.ErrorCode, stream.ErrorType
			if stream.ErrorStatus != 0 {
				status = stream.ErrorStatus
			}
		} else if apiErr == nil {
			if stream.Response == relaycommon.ResponseOutcomeIncomplete {
				if stream.IncompleteReason == "content_filter" || stream.IncompleteReason == "safety" {
					return ignored
				}
				if stream.IncompleteReason != "max_output_tokens" && stream.IncompleteReason != "max_tokens" {
					return routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeRoute, Reason: "incomplete_upstream_response"}
				}
			}
			if stream.HasErrors || stream.EndReason == relaycommon.StreamEndReasonTimeout || stream.EndReason == relaycommon.StreamEndReasonScannerErr || stream.EndReason == relaycommon.StreamEndReasonPanic || stream.ExpectsTerminal && stream.Response == relaycommon.ResponseOutcomeUnknown {
				return routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeRoute, Reason: "upstream_stream_interrupted"}
			}
			return routehealth.Result{Verdict: routehealth.VerdictSuccess, Scope: routehealth.ScopeRoute, Reason: "upstream_completed"}
		}
	}
	if apiErr == nil && code == "" {
		return routehealth.Result{Verdict: routehealth.VerdictSuccess, Scope: routehealth.ScopeRoute, Reason: "upstream_completed"}
	}
	if protocolBusinessRejection(code, errorType) {
		return ignored
	}
	for _, value := range []string{strings.ToLower(code), strings.ToLower(errorType)} {
		switch value {
		case "invalid_api_key", "api_key_invalid", "api_key_expired", "invalid_authentication", "authentication_error", "unauthenticated", "insufficient_quota", "quota_exceeded":
			return routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeCredential, Reason: "upstream_credential_unavailable"}
		case "context_length_exceeded", "invalid_request", "invalid_request_error", "invalid_argument", "prompt_blocked", "content_filter", "content_policy_violation", "safety":
			return ignored
		case "model_not_found", "model_not_supported", "unsupported_model", "rate_limit_exceeded", "rate_limit_error", "resource_exhausted", "overloaded_error", "server_error", "internal_error", "service_unavailable":
			return routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeRoute, Reason: "upstream_route_unavailable"}
		}
	}
	switch status {
	case 400, 405, 409, 413, 415, 422:
		return ignored
	case 401:
		return routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeCredential, Reason: "upstream_credential_unavailable"}
	}
	if apiErr != nil && apiErr.GetErrorCode() == types.ErrorCodeDoRequestFailed {
		return routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeChannel, Reason: "upstream_transport_unavailable"}
	}
	return routehealth.Result{Verdict: routehealth.VerdictFailure, Scope: routehealth.ScopeRoute, Reason: "upstream_route_unavailable"}
}

// AppendRouteHealthAdminInfo adds non-secret per-attempt decisions to admin-only logs.
func AppendRouteHealthAdminInfo(c *gin.Context, admin map[string]any) {
	if c == nil || admin == nil {
		return
	}
	if value, ok := c.Get(routeAttemptContextKey); ok {
		admin["route_health"] = value.(*routeAttemptState).events
	}
}

func routeRequestPath(c *gin.Context, path string) string {
	if path != "" {
		return path
	}
	if c != nil && c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

func AdmitChannelRoute(c *gin.Context, channel *model.Channel, modelName string, endpoint constant.EndpointType) *types.NewAPIError {
	groups := c.GetStringSlice(string(constant.ContextKeyTokenGroups))
	selected, _, err := SelectRouteForChannel(c, channel, &RetryParam{Ctx: c, TokenGroups: groups, ModelName: modelName, EndpointType: endpoint})
	if err != nil {
		var apiErr *types.NewAPIError
		if errors.As(err, &apiErr) {
			return apiErr
		}
		return types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if selected == nil {
		return routeUnavailableError()
	}
	return nil
}

func routeUnavailableError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New("upstream routes are temporarily unavailable"), "route_temporarily_unavailable", http.StatusServiceUnavailable)
}
