package service

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

// ClassifyAvailabilityResult classifies the final user request, not an upstream
// attempt or a billing log. Reasons are bounded categories, never error text.
func ClassifyAvailabilityResult(apiErr *types.NewAPIError, stream *relaycommon.StreamStatus, isStream, clientGone bool) (string, string) {
	if stream != nil {
		_, _, _, serviceFailure := stream.AvailabilityEvidence()
		if serviceFailure {
			return model.AvailabilityFailure, "stream_service_error"
		}
	}
	if stream != nil && stream.AvailabilityIncomplete() {
		return model.AvailabilityUnknown, "completion_unverified"
	}
	if apiErr != nil {
		if clientGone && apiErr.GetErrorCode() == types.ErrorCodeDoRequestFailed && errors.Is(apiErr, context.Canceled) && !errors.Is(apiErr, context.DeadlineExceeded) {
			return model.AvailabilityExcluded, "client_disconnect"
		}
		switch apiErr.GetErrorCode() {
		case types.ErrorCodeInvalidRequest, types.ErrorCodeReadRequestBodyFailed,
			types.ErrorCodeBadRequestBody, types.ErrorCodeSensitiveWordsDetected,
			types.ErrorCodePromptBlocked, types.ErrorCodeViolationFeeGrokCSAM,
			types.ErrorCodeInsufficientUserQuota, types.ErrorCodeSubscriptionRequired,
			types.ErrorCodeSubscriptionTokenExhausted, types.ErrorCodeSubscriptionConcurrencyExceeded,
			types.ErrorCodeAPIKeyTokenLimitExhausted, types.ErrorCodeGPTAbuseSuspended,
			types.ErrorCodeGPTAbuseRepeatedWarningRequest:
			// Locally defined user restrictions must not be inferred from an upstream
			// 401/403/429, which can mean invalid provider credentials or saturation.
			if apiErr.IsLocal() {
				return model.AvailabilityExcluded, "user_condition"
			}
		}
		return model.AvailabilityFailure, "service_error"
	}
	if stream != nil {
		reason, completed, hasErrors, serviceFailure := stream.AvailabilityEvidence()
		if serviceFailure {
			return model.AvailabilityFailure, "stream_service_error"
		}
		if reason == relaycommon.StreamEndReasonClientGone || reason == relaycommon.StreamEndReasonPingFail {
			return model.AvailabilityExcluded, "client_disconnect"
		}
		if completed {
			return model.AvailabilitySuccess, "completed"
		}
		if reason == relaycommon.StreamEndReasonEOF || hasErrors {
			return model.AvailabilityFailure, "stream_incomplete"
		}
		return model.AvailabilityUnknown, "completion_unverified"
	}
	if clientGone {
		return model.AvailabilityExcluded, "client_disconnect"
	}
	if isStream {
		return model.AvailabilityUnknown, "completion_unverified"
	}
	return model.AvailabilitySuccess, "completed"
}
