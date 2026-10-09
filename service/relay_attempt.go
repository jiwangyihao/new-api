package service

import (
	"context"
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// StreamAttemptError reconciles protocol errors and transport termination before
// either settlement or retry. It never treats prelude delivery as output.
func StreamAttemptError(info *relaycommon.RelayInfo) *types.NewAPIError {
	if info == nil || info.StreamGate == nil {
		return nil
	}
	gate := info.StreamGate
	if info.StreamStatus == nil {
		return gate.AttemptError()
	}
	outcome := info.StreamStatus.OutcomeSnapshot()
	if info.StreamStatus.AvailabilityIncomplete() {
		// A handler still shutting down cannot safely share the next attempt's
		// writer/state. Preserve the unverified availability outcome, not a
		// guessed protocol failure.
		gate.MarkUnsafe()
		gate.SetAttemptError(types.NewOpenAIError(errors.New("stream completion could not be verified"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry()))
		return gate.AttemptError()
	}
	if outcome.Response == relaycommon.ResponseOutcomeCancelled {
		gate.MarkUnsafe()
		gate.SetAttemptError(types.NewOpenAIError(context.Canceled, types.ErrorCodeDoRequestFailed, 499, types.ErrOptionWithSkipRetry()))
		return gate.AttemptError()
	}
	if outcome.EndReason == relaycommon.StreamEndReasonClientGone || outcome.EndReason == relaycommon.StreamEndReasonPingFail {
		gate.MarkUnsafe()
		info.StreamStatus.MarkCancelled()
		gate.SetAttemptError(types.NewOpenAIError(context.Canceled, types.ErrorCodeDoRequestFailed, 499, types.ErrOptionWithSkipRetry()))
		return gate.AttemptError()
	}
	if gate.AttemptError() == nil && outcome.Response == relaycommon.ResponseOutcomeFailed {
		status := outcome.ErrorStatus
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		gate.SetAttemptError(types.WithOpenAIError(types.OpenAIError{Type: outcome.ErrorType, Code: outcome.ErrorCode, Message: "upstream response failed"}, status))
	}
	if gate.AttemptError() == nil && (outcome.EndReason == relaycommon.StreamEndReasonTimeout || outcome.EndReason == relaycommon.StreamEndReasonScannerErr || outcome.EndReason == relaycommon.StreamEndReasonPanic) {
		gate.SetAttemptError(types.NewOpenAIError(errors.New("upstream stream ended before completion"), types.ErrorCodeBadResponse, http.StatusBadGateway))
	}
	gate.EndAttempt()
	if apiErr := gate.AttemptError(); apiErr != nil {
		info.StreamStatus.MarkFailed(string(apiErr.GetErrorCode()), apiErr.ToOpenAIError().Type, apiErr.StatusCode)
		return apiErr
	}
	return nil
}

// PrepareRelaySettlement leaves an unconsumed reservation intact across failed
// output-free attempts. The controller either retries or refunds it once.
func PrepareRelaySettlement(c *gin.Context, info *relaycommon.RelayInfo) bool {
	apiErr := StreamAttemptError(info)
	FinishRouteAttempt(c, info, apiErr)
	return apiErr == nil || info == nil || info.StreamGate == nil || info.StreamGate.Meaningful()
}

// RelayOutputCommitted is the refund boundary, not the HTTP commit boundary.
func RelayOutputCommitted(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	if info.StreamGate != nil {
		return info.StreamGate.Meaningful()
	}
	return info.HasSendResponse()
}

func relaySettlementSucceeded(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	clientGone := c != nil && c.Request != nil && c.Request.Context().Err() != nil
	result, _ := ClassifyAvailabilityResult(StreamAttemptError(info), info.StreamStatus, info.IsStream, clientGone)
	return result == model.AvailabilitySuccess
}

func appendRelayAttemptAdminInfo(c *gin.Context, other map[string]interface{}) {
	if other == nil {
		return
	}
	admin, ok := other["admin_info"].(map[string]interface{})
	if !ok {
		admin = make(map[string]interface{})
		other["admin_info"] = admin
	}
	AppendRouteHealthAdminInfo(c, admin)
}
