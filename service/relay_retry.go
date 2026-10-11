package service

import (
	"context"

	"github.com/QuantumNous/new-api/pkg/routehealth"
	"github.com/QuantumNous/new-api/pkg/streamgate"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type RouteRetryDecision struct {
	Retry  bool              `json:"retry"`
	Scope  routehealth.Scope `json:"scope"`
	Reason string            `json:"reason"`
}

// DecideRouteRetry 统一 HTTP 和原生 Responses WebSocket 的重试边界及审计原因。
// 不猜测下游等待时长；保留请求取消、次数、输出安全与绑定约束。
func DecideRouteRetry(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError, remaining int) (decision RouteRetryDecision) {
	defer func() {
		if c != nil {
			routeState(c).retryDecision = decision
		}
	}()
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	result := ClassifyRouteAttempt(ctx, info, apiErr, true)
	decision.Scope = result.Scope
	if apiErr == nil {
		decision.Reason = "request_completed"
		return
	}
	if ctx.Err() != nil {
		decision.Reason = "client_cancelled"
		return
	}
	if remaining <= 0 {
		decision.Reason = "attempt_limit_reached"
		return
	}
	if types.IsSkipRetryError(apiErr) {
		decision.Reason = "error_disallows_retry"
		return
	}
	if c != nil {
		if value, found := c.Get("relay_stream_gate"); found && !value.(*streamgate.Gate).CanRetry() {
			decision.Reason = "output_not_replayable"
			return
		}
		if ShouldSkipRetryAfterChannelAffinityFailure(c) {
			decision.Reason = "strict_session"
			return
		}
		if _, pinned := c.Get("specific_channel_id"); pinned {
			decision.Reason = "specific_channel"
			return
		}
		if _, bound := c.Get("response_route_binding"); bound {
			decision.Reason = "response_binding"
			return
		}
	}
	if info != nil && info.StreamGate != nil && !info.StreamGate.CanRetry() {
		decision.Reason = "output_not_replayable"
		return
	}
	if types.IsChannelError(apiErr) {
		decision.Retry, decision.Reason = true, "try_another_channel"
		return
	}
	status := apiErr.StatusCode
	if operation_setting.IsAlwaysSkipRetryCode(apiErr.GetErrorCode()) || result.Verdict == routehealth.VerdictIgnored {
		decision.Reason = "error_not_retryable"
		return
	}
	if status >= 200 && status < 300 || status >= 100 && status <= 599 && !operation_setting.ShouldRetryByStatusCode(status) {
		decision.Reason = "status_not_retryable"
		return
	}
	decision.Retry, decision.Reason = true, "try_another_channel"
	if result.Scope == routehealth.ScopeCredential {
		decision.Reason = "try_another_credential"
	}
	return
}
