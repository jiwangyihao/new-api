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

// ShouldRetryRouteAttempt is shared by HTTP and native Responses WebSocket.
// Admission and billing remain separate; a retry never resets either budget.
func ShouldRetryRouteAttempt(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError, remaining int) bool {
	if apiErr == nil || remaining <= 0 || types.IsSkipRetryError(apiErr) {
		return false
	}
	ctx := context.Background()
	if c != nil {
		if c.Request != nil {
			ctx = c.Request.Context()
			if ctx.Err() != nil {
				return false
			}
		}
		if value, found := c.Get("relay_stream_gate"); found && !value.(*streamgate.Gate).CanRetry() {
			return false
		}
		if !RouteRetryBudgetAvailable(c) || ShouldSkipRetryAfterChannelAffinityFailure(c) {
			return false
		}
		if _, pinned := c.Get("specific_channel_id"); pinned {
			return false
		}
		if _, bound := c.Get("response_route_binding"); bound {
			return false
		}
	}
	if info != nil && info.StreamGate != nil && !info.StreamGate.CanRetry() {
		return false
	}
	if types.IsChannelError(apiErr) {
		return true
	}
	status := apiErr.StatusCode
	if status >= 200 && status < 300 {
		return false
	}
	if status < 100 || status > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(apiErr.GetErrorCode()) {
		return false
	}
	if ClassifyRouteAttempt(ctx, info, apiErr, true).Verdict == routehealth.VerdictIgnored {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(status)
}
