package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	appmodel "github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/streamgate"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const responsesWSEventTypeResponseCreate = "response.create"
const responsesWSWriteTimeout = 30 * time.Second

// ResponsesWSRequestRunner executes the existing authentication and rate-limit
// middleware around one complete request, without a second HTTP connection.
type ResponsesWSRequestRunner func(*http.Request, string, func(*gin.Context) *types.NewAPIError) *types.NewAPIError

type responsesWSCreateEvent struct {
	Type     string          `json:"type"`
	EventID  string          `json:"event_id,omitempty"`
	StreamID json.RawMessage `json:"stream_id,omitempty"`
	Generate json.RawMessage `json:"generate,omitempty"`
	Request  json.RawMessage `json:"response,omitempty"`
}

type responsesWSCreateRequest struct {
	Request  dto.OpenAIResponsesRequest
	Body     []byte
	Generate json.RawMessage
	StreamID string
}

type responsesWSErrorEvent struct {
	Type       string             `json:"type"`
	Status     int                `json:"status"`
	EventID    string             `json:"event_id,omitempty"`
	StreamID   string             `json:"stream_id,omitempty"`
	ResponseID string             `json:"response_id,omitempty"`
	Error      *types.OpenAIError `json:"error"`
}

type responsesWSMessage struct {
	kind   int
	body   []byte
	err    error
	target *websocket.Conn
}

// responsesWSControl is a client control event (response.cancel) whose
// envelope the read loop already parsed.
type responsesWSControl struct {
	body              []byte
	eventID, streamID string
}

// Only the request worker reads or changes billing state. Socket readers pass
// bounded messages to it; cancellation never performs an independent refund.
type responsesWSCallState struct {
	inbox         chan responsesWSMessage
	controls      chan responsesWSControl
	done          chan struct{}
	terminal      *responsesWSMessage
	closeAfter    bool
	suppressError bool
}

type responsesWSSession struct {
	ctx            context.Context
	cancel         context.CancelFunc
	client         *websocket.Conn
	runner         ResponsesWSRequestRunner
	request        *http.Request
	requestID      string
	nextEventIndex int
	workers        sync.WaitGroup

	clientWriteMu sync.Mutex
	targetWriteMu sync.Mutex
	connectionMu  sync.Mutex
	target        *websocket.Conn
	stateMu       sync.Mutex
	current       *responsesWSCallState

	// These fields belong to the serial request worker and describe the actual
	// established connection. Per-request token/user data is never stored here.
	recentResponseIDs   [32]string
	recentResponseIndex int
	lockedModel         string
	lockedChannelID     int
	lockedGroup         string
	lockedKey           string
	lockedConfig        string
	lockedProfile       appmodel.ChannelBillingProfile
	lockedUserID        int
	lockedTokenID       int
	lockedProMarker     bool
}

func ResponsesWebSocketHelper(c *gin.Context, client *websocket.Conn, runner ResponsesWSRequestRunner) *types.NewAPIError {
	ctx, cancel := context.WithCancel(c.Request.Context())
	s := &responsesWSSession{ctx: ctx, cancel: cancel, client: client, runner: runner,
		request: c.Request.Clone(ctx), requestID: c.GetString(common.RequestIdKey)}
	if s.requestID == "" {
		s.requestID = common.GetUUID()
	}
	maxMB := appconstant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	client.SetReadLimit(int64(maxMB) << 20)
	defer func() {
		s.shutdown()
		s.workers.Wait()
	}()

	for {
		_, message, err := client.ReadMessage()
		if err != nil {
			return nil
		}
		envelope, streamID, err := parseResponsesWSEnvelope(message)
		eventType := envelope.Type
		if err != nil {
			s.sendError(envelope.EventID, streamID, newResponsesWSInvalidRequestError(err))
			continue
		}
		if eventType != responsesWSEventTypeResponseCreate {
			// Controls are owned by the active request too. In particular a cancel
			// arriving during authentication must not precede its upstream create.
			state := s.getCurrent()
			if eventType != "response.cancel" || state == nil {
				s.sendError(envelope.EventID, streamID, newResponsesWSInvalidRequestError(fmt.Errorf("unsupported websocket event %q", eventType)))
				continue
			}
			select {
			case state.controls <- responsesWSControl{body: message, eventID: envelope.EventID, streamID: streamID}:
			case <-state.done:
			case <-s.ctx.Done():
				return nil
			default:
				s.sendError(envelope.EventID, streamID, newResponsesWSInvalidRequestError(errors.New("a response control event is already pending")))
			}
			continue
		}
		state := &responsesWSCallState{inbox: make(chan responsesWSMessage), controls: make(chan responsesWSControl, 1), done: make(chan struct{})}
		if !s.tryReserveCurrent(state) {
			s.sendError(envelope.EventID, streamID, types.NewErrorWithStatusCode(errors.New("another response.create is already in progress on this websocket connection"), types.ErrorCodeInvalidRequest, http.StatusConflict, types.ErrOptionWithSkipRetry()))
			continue
		}
		requestID := fmt.Sprintf("%s-ws-%d", s.requestID, s.nextEventIndex)
		s.nextEventIndex++
		s.workers.Go(func() { s.runRequest(state, message, envelope, streamID, requestID) })
	}
}

func (s *responsesWSSession) runRequest(state *responsesWSCallState, message []byte, envelope responsesWSCreateEvent, streamID string, requestID string) {
	create, parseErr := normalizeResponsesWSCreateEvent(message, envelope, streamID)
	var apiErr *types.NewAPIError
	defer func() {
		if recovered := recover(); recovered != nil {
			apiErr = types.NewError(fmt.Errorf("responses websocket request panic: %v", recovered), types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
			state.closeAfter = true
		}
		// Finish the middleware count before publishing the terminal event. Hold
		// admission while writing it so an immediate next create cannot race
		// the current request's release; socket close needs neither lock.
		outgoing := state.terminal
		if apiErr != nil && outgoing == nil && !state.suppressError {
			if body, err := buildResponsesWSErrorPayload(envelope.EventID, streamID, apiErr); err == nil {
				outgoing = &responsesWSMessage{kind: websocket.TextMessage, body: body}
			}
		}
		s.clientWriteMu.Lock()
		s.stateMu.Lock()
		if outgoing != nil {
			if err := s.client.SetWriteDeadline(time.Now().Add(responsesWSWriteTimeout)); err != nil {
				state.closeAfter = true
			} else if err := s.client.WriteMessage(outgoing.kind, outgoing.body); err != nil {
				state.closeAfter = true
			}
		}
		s.current = nil
		close(state.done)
		s.stateMu.Unlock()
		s.clientWriteMu.Unlock()
		if state.closeAfter {
			s.shutdown()
		}
	}()
	request := s.request.Clone(s.ctx)
	body := message
	if parseErr == nil {
		body = create.Body
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.Header.Set("Content-Type", "application/json")
	apiErr = s.runner(request, requestID, func(c *gin.Context) *types.NewAPIError {
		if parseErr != nil {
			return newResponsesWSInvalidRequestError(parseErr)
		}
		validated, err := helper.GetAndValidateRequest(c, types.RelayFormatOpenAIResponses)
		if err != nil {
			return newResponsesWSInvalidRequestError(err)
		}
		create.Request = *validated.(*dto.OpenAIResponsesRequest)
		return s.runCall(c, state, create)
	})
	if apiErr != nil && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
		state.closeAfter = true
	}
}

func (s *responsesWSSession) runCall(c *gin.Context, state *responsesWSCallState, create responsesWSCreateRequest) (apiErr *types.NewAPIError) {
	c.Set("relay_request_success", false)
	c.Set("relay_native_responses_websocket", true)
	defer service.ReleaseRouteAttempt(c)
	gate := streamgate.New(c.Writer, string(types.RelayFormatOpenAIResponses))
	c.Set("relay_stream_gate", gate)
	modelName := create.Request.Model
	retry := &service.RetryParam{Ctx: c, TokenGroups: responsesWSTokenGroups(c), ModelName: modelName, RequestPath: "/v1/responses", EndpointType: appconstant.EndpointTypeOpenAIResponse, RequireResponsesWebSocket: true, Retry: common.GetPointer(0)}
	boundAtStart := s.lockedChannelID != 0 || create.Request.PreviousResponseID != ""
	if _, pinned := c.Get("specific_channel_id"); pinned {
		boundAtStart = true
	}
	var info *relaycommon.RelayInfo
	var billing *responsesWSBilling
	var usage *dto.Usage
	var capture *service.AvailabilityCapture
	gate.BindResponseIdentity("resp_route_"+common.GetUUID(), func(publicID, upstreamID string) error {
		return service.SaveResponseBinding(c, info, publicID, upstreamID)
	})
	defer func() {
		if recovered := recover(); recovered != nil {
			apiErr = types.NewError(errors.New("responses websocket worker failed"), types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
			gate.MarkUnsafe()
			gate.SetAttemptError(apiErr)
			state.closeAfter = true
		}
		if info != nil {
			if s.ctx.Err() != nil && info.StreamStatus != nil && state.terminal == nil {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, s.ctx.Err())
			}
			if info.StreamStatus != nil {
				if protocolErr := service.StreamAttemptError(info); protocolErr != nil && apiErr == nil {
					apiErr = protocolErr
				}
			}
			service.FinishRouteAttempt(c, info, apiErr)
			if billing != nil && (apiErr == nil || gate.Meaningful()) {
				if settleErr := billing.settle(c, info, usage); settleErr != nil {
					apiErr = settleErr
					state.terminal = nil
					state.closeAfter = true
				}
			}
			if apiErr == nil && state.terminal != nil && usage != nil {
				if metadata := service.NewAPIBillingFromUsage(info, usage); metadata != nil {
					if body, err := sjson.SetBytes(state.terminal.body, "newapi_billing", metadata); err == nil {
						state.terminal.body = body
					}
				}
			}
			billing.finish(c, info)
			outcome, _ := service.ClassifyAvailabilityResult(apiErr, info.StreamStatus, true, s.ctx.Err() != nil)
			if apiErr == nil && outcome == appmodel.AvailabilitySuccess {
				c.Set("relay_request_success", true)
				service.RecordChannelAffinity(c, info.ChannelId)
			}
		}
		if apiErr != nil && state.terminal == nil {
			if raw, ok := gate.FailureEvent(); ok {
				state.terminal = &responsesWSMessage{kind: websocket.TextMessage, body: raw}
			}
		}
		if id := gate.UpstreamResponseID(); id != "" {
			s.recentResponseIDs[s.recentResponseIndex] = id
			s.recentResponseIndex = (s.recentResponseIndex + 1) % len(s.recentResponseIDs)
		}
		service.ObserveAvailabilityResult(c, info, apiErr)
		capture.Finish(c)
	}()
	if modelName == "" {
		return newResponsesWSInvalidRequestError(errors.New("model is required"))
	}
	if s.lockedModel != "" && modelName != s.lockedModel {
		state.closeAfter = true
		return newResponsesWSInvalidRequestError(errors.New("websocket model changed; reconnect required"))
	}
	if apiErr = checkResponsesWSModelAccess(c, modelName); apiErr != nil {
		return apiErr
	}
	if create.Request.PreviousResponseID != "" {
		if apiErr = service.ResponseRouteBinding(c, create.Request.PreviousResponseID, modelName); apiErr != nil {
			return apiErr
		}
	}
	common.SetContextKey(c, appconstant.ContextKeyOriginalModel, modelName)
	capture = service.BeginAvailability(c, modelName, responsesWSTokenGroups(c))
	for retry.GetRetry() <= common.RetryTimes {
		if retry.GetRetry() > 0 && !service.RouteRetryBudgetAvailable(c) {
			return info.LastError
		}
		var selected *appmodel.Channel
		if s.lockedChannelID != 0 {
			if apiErr = s.restoreConnectionContext(c, modelName); apiErr != nil {
				state.closeAfter = true
				return apiErr
			}
		} else {
			selected, apiErr = selectResponsesWSChannel(c, modelName, retry)
			if apiErr != nil {
				return apiErr
			}
			retry.UsedChannelIds = append(retry.UsedChannelIds, selected.Id)
		}
		if info == nil {
			info = relaycommon.GenRelayInfoResponses(c, &create.Request)
			info.InitChannelMeta(c)
			info.ChannelId = info.ChannelMeta.ChannelId
			info.ChannelType = info.ChannelMeta.ChannelType
			info.StreamGate = gate
			info.IsStream = true
			info.UsingGroup = common.GetContextKeyString(c, appconstant.ContextKeyUsingGroup)
			common.SetContextKey(c, appconstant.ContextKeyIsStream, true)
			billing, apiErr = prepareResponsesWSBilling(c, info)
			if apiErr != nil {
				return apiErr
			}
			retry.FrozenTokenBillingMultiplier = info.FrozenChannelTokenBillingMultiplier()
			retry.FrozenBillingProfile = responsesWSBillingProfile(info)
			retry.RequireSameBillingProfile = true
			retry.RequireSameTokenBillingMultiplier = info.SubscriptionDistributorTokenBilling
		} else {
			info.InitChannelMeta(c)
			info.UsingGroup = common.GetContextKeyString(c, appconstant.ContextKeyUsingGroup)
			info.PriceData.QuotaMultiplierInfo = helper.HandleQuotaMultiplier(c, info)
		}
		info.ChannelId = info.ChannelMeta.ChannelId
		info.ChannelType = info.ChannelMeta.ChannelType
		info.RetryIndex = retry.GetRetry()
		usage = nil
		payload, prepErr := buildResponsesWSCreatePayload(c, info, create.Request, create.Generate, create.StreamID)
		if prepErr != nil {
			return prepErr
		}
		if s.getTarget() != nil {
			info.FinalizeCodexProRequestMarker()
			if info.CodexProRequestSent != s.lockedProMarker {
				state.closeAfter = true
				return newResponsesWSInvalidRequestError(errors.New("upstream connection billing marker changed; reconnect required"))
			}
		}
		if apiErr = service.EnforceGPTAbuseSuspension(c, info); apiErr != nil {
			return apiErr
		}
		storage, storageErr := common.GetBodyStorage(c)
		if storageErr != nil {
			return newResponsesWSInvalidRequestError(storageErr)
		}
		if err := service.CaptureGPTAbuseRepeatBlockFingerprint(c, info, storage); err != nil {
			return newResponsesWSInvalidRequestError(err)
		}
		if apiErr = service.CheckGPTAbuseRepeatBlock(c, info); apiErr != nil {
			return apiErr
		}
		gate.BeginAttempt()
		// Background/unknown execution is never silently replayed.
		if gjson.GetBytes(create.Body, "background").Bool() {
			gate.MarkUnsafe()
		}
		service.AttemptAvailabilityChannel(c, info.ChannelId)
		if s.getTarget() == nil {
			adaptor := GetAdaptor(info.ApiType)
			adaptor.Init(info)
			target, dialErr := relaychannel.DialResponsesWebSocket(adaptor, c, info)
			if dialErr != nil {
				service.ResetStatusCode(dialErr, c.GetString("status_code_mapping"))
				service.FinishRouteAttempt(c, info, dialErr)
				info.LastError = dialErr
				if !boundAtStart && service.ShouldRetryRouteAttempt(c, info, dialErr, common.RetryTimes-retry.GetRetry()) {
					retry.IncreaseRetry()
					continue
				}
				return dialErr
			}
			if !s.setTarget(target) {
				return types.NewOpenAIError(context.Canceled, types.ErrorCodeBadResponse, 499, types.ErrOptionWithSkipRetry())
			}
			s.startTargetReader(target)
		}
		service.MarkRouteUpstreamStarted(c)
		if err := s.writeTarget(websocket.TextMessage, payload); err != nil {
			s.closeTarget()
			apiErr = types.NewOpenAIError(errors.New("upstream websocket write failed"), types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
			service.FinishRouteAttempt(c, info, apiErr)
			info.LastError = apiErr
			if !boundAtStart && service.ShouldRetryRouteAttempt(c, info, apiErr, common.RetryTimes-retry.GetRetry()) {
				retry.IncreaseRetry()
				continue
			}
			state.closeAfter = true
			return apiErr
		}
		if selected != nil {
			s.lockedModel, s.lockedChannelID, s.lockedGroup = modelName, selected.Id, info.UsingGroup
			s.lockedKey = info.ApiKey
			s.lockedConfig = service.RouteConfigurationFingerprint(selected)
			s.lockedProfile = responsesWSBillingProfile(info)
			s.lockedUserID, s.lockedTokenID = info.UserId, info.TokenId
			s.lockedProMarker = info.CodexProRequestSent
		}
		info.StreamStatus = relaycommon.NewStreamStatus()
		info.StreamStatus.RequireTerminal()
		timeout := time.Duration(appconstant.StreamingTimeout) * time.Second
		if timeout <= 0 {
			timeout = 300 * time.Second
		}
		idle := time.NewTimer(timeout)
		defer idle.Stop()
		accepted := false
		var pendingControl []byte
		var sentControl []byte
		var responseID string
	readAttempt:
		for {
			select {
			case incoming := <-state.inbox:
				if incoming.target != nil && incoming.target != s.getTarget() {
					continue
				}
				idle.Reset(timeout)
				if incoming.err != nil {
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, incoming.err)
					apiErr = types.NewOpenAIError(incoming.err, types.ErrorCodeBadResponse, http.StatusBadGateway)
					gate.SetAttemptError(apiErr)
					break readAttempt
				}
				info.SetFirstResponseTime()
				var event struct {
					dto.ResponsesStreamResponse
					StreamID string `json:"stream_id"`
				}
				if err := common.Unmarshal(incoming.body, &event); err != nil {
					info.StreamStatus.RecordError("invalid upstream websocket event")
				} else {
					if event.Type != "error" && event.StreamID != "" && event.StreamID != create.StreamID {
						continue
					}
					// A repeated terminal from the previous response must never finish
					// a subsequent request on this persistent connection.
					if event.Response != nil && s.isPreviousResponse(event.Response.ID) {
						continue
					}
					if s.isPreviousResponse(gjson.GetBytes(incoming.body, "response_id").String()) {
						continue
					}
					if event.Type == "error" {
						var rejection responsesWSErrorEvent
						_ = common.Unmarshal(incoming.body, &rejection)
						if s.isPreviousResponse(rejection.ResponseID) {
							continue
						}
						terminal, ambiguous, controlError := responsesWSErrorEndsRequest(rejection, create.StreamID, responseID, sentControl)
						if !terminal {
							if err := s.writeClient(incoming.kind, incoming.body); err != nil {
								s.shutdown()
							}
							// Only a control error in this stream resolves its pending control.
							if controlError {
								sentControl = nil
							}
							continue
						}
						state.closeAfter = ambiguous
						if ambiguous {
							gate.MarkUnsafe()
						}
						if !accepted && rejection.Error == nil {
							rejection.Error = &types.OpenAIError{Type: "invalid_request_error", Message: gjson.GetBytes(incoming.body, "message").String(), Code: gjson.GetBytes(incoming.body, "code").String()}
							var marshalErr error
							incoming.body, marshalErr = common.Marshal(rejection)
							if marshalErr != nil {
								return types.NewError(marshalErr, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
							}
						}
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
					}
					if strings.HasPrefix(event.Type, "response.") {
						accepted = true
						if event.Response != nil && event.Response.ID != "" {
							responseID = event.Response.ID
						}
					}
					usage = observeResponsesWSUsage(c, info, &event.ResponsesStreamResponse, incoming.body, usage)
				}
				outgoing, emit, transformErr := gate.TransformEvent(incoming.body)
				if transformErr != nil {
					apiErr = types.NewError(transformErr, types.ErrorCodeBadResponseBody)
					break readAttempt
				}
				if gate.AttemptError() != nil && !emit {
					apiErr = gate.AttemptError()
					break readAttempt
				}
				if !emit {
					continue
				}
				for _, prelude := range gate.DrainPrelude() {
					if err := s.writeClient(incoming.kind, prelude); err != nil {
						s.shutdown()
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, context.Canceled)
						return types.NewOpenAIError(context.Canceled, types.ErrorCodeBadResponse, 499, types.ErrOptionWithSkipRetry())
					}
				}
				incoming.body = outgoing
				switch event.Type {
				case "error", "response.error", "response.completed", "response.done", "response.incomplete", "response.failed", "response.cancelled", "response.canceled":
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
					state.terminal = &incoming
					return gate.AttemptError()
				}
				if err := s.writeClient(incoming.kind, incoming.body); err != nil {
					s.shutdown()
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, context.Canceled)
					return types.NewOpenAIError(context.Canceled, types.ErrorCodeBadResponse, 499, types.ErrOptionWithSkipRetry())
				}
				if accepted && pendingControl != nil {
					gate.MarkUnsafe()
					if gjson.GetBytes(pendingControl, "response_id").String() == gate.ResponseID() && gate.UpstreamResponseID() != "" {
						pendingControl, _ = sjson.SetBytes(pendingControl, "response_id", gate.UpstreamResponseID())
					}
					if err := s.writeTarget(websocket.TextMessage, pendingControl); err != nil {
						s.shutdown()
					}
					sentControl = pendingControl
					pendingControl = nil
				}
			case control := <-state.controls:
				gate.MarkUnsafe()
				if gjson.GetBytes(control.body, "response_id").String() == gate.ResponseID() && gate.UpstreamResponseID() != "" {
					control.body, _ = sjson.SetBytes(control.body, "response_id", gate.UpstreamResponseID())
				}
				if pendingControl != nil || sentControl != nil {
					s.sendError(control.eventID, control.streamID, newResponsesWSInvalidRequestError(errors.New("a response control event is already pending")))
					continue
				}
				if !accepted {
					pendingControl = control.body
					continue
				}
				if err := s.writeTarget(websocket.TextMessage, control.body); err != nil {
					s.shutdown()
				}
				sentControl = control.body
			case <-idle.C:
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, context.DeadlineExceeded)
				apiErr = types.NewOpenAIError(context.DeadlineExceeded, types.ErrorCodeBadResponse, http.StatusBadGateway)
				gate.SetAttemptError(apiErr)
				break readAttempt
			case <-s.ctx.Done():
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, s.ctx.Err())
				return types.NewOpenAIError(context.Canceled, types.ErrorCodeBadResponse, 499, types.ErrOptionWithSkipRetry())
			}
		}
		idle.Stop()
		if streamErr := service.StreamAttemptError(info); streamErr != nil {
			apiErr = streamErr
		}
		service.FinishRouteAttempt(c, info, apiErr)
		if !boundAtStart && service.ShouldRetryRouteAttempt(c, info, apiErr, common.RetryTimes-retry.GetRetry()) {
			s.closeTarget()
			s.lockedChannelID, s.lockedModel, s.lockedGroup = 0, "", ""
			s.lockedKey, s.lockedConfig = "", ""
			info.LastError = apiErr
			retry.IncreaseRetry()
			continue
		}
		outcome := info.StreamStatus.OutcomeSnapshot()
		if outcome.EndReason == relaycommon.StreamEndReasonTimeout || outcome.EndReason == relaycommon.StreamEndReasonScannerErr {
			state.closeAfter = true
		}
		if raw, ok := gate.FailureEvent(); ok {
			state.terminal = &responsesWSMessage{kind: websocket.TextMessage, body: raw}
		}

		return apiErr
	}
	return info.LastError
}

// A single generation is active, but a cancel can fail independently. Correlate
// available IDs first; uncorrelated control/server errors require a bounded
// connection close because continuing could leave the generation stuck forever.
func responsesWSErrorEndsRequest(event responsesWSErrorEvent, streamID, responseID string, control []byte) (terminal, ambiguous, controlError bool) {
	if len(control) > 0 {
		var pending struct {
			EventID    string `json:"event_id"`
			StreamID   string `json:"stream_id"`
			ResponseID string `json:"response_id"`
		}
		_ = common.Unmarshal(control, &pending)
		if event.EventID != "" && event.EventID == pending.EventID {
			return false, false, true
		}
		if event.ResponseID != "" && event.ResponseID == pending.ResponseID && event.ResponseID != responseID {
			return false, false, true
		}
		if (event.StreamID == "" || event.StreamID == pending.StreamID) && event.Error != nil && event.Error.Type == "invalid_request_error" {
			switch event.Error.Code {
			case "response_not_found", "response_not_active", "response_already_completed":
				return false, false, true
			}
		}
	}
	if event.StreamID != "" && event.StreamID != streamID || responseID != "" && event.ResponseID != "" && event.ResponseID != responseID {
		return false, false, false
	}
	return true, len(control) > 0 && event.ResponseID == "", false
}

func (s *responsesWSSession) isPreviousResponse(id string) bool {
	if id == "" {
		return false
	}
	for _, previous := range s.recentResponseIDs {
		if previous == id {
			return true
		}
	}
	return false
}

func responsesWSTokenGroups(c *gin.Context) []string {
	if value, ok := common.GetContextKey(c, appconstant.ContextKeyTokenGroups); ok {
		if groups, ok := value.([]string); ok && len(groups) > 0 {
			return groups
		}
	}
	return []string{appmodel.DefaultChannelGroupName}
}

func responsesWSBillingProfile(info *relaycommon.RelayInfo) appmodel.ChannelBillingProfile {
	return (appmodel.ChannelBillingProfile{CreditBillingMode: info.CreditBillingMode, FixedRequestCredits: info.FixedRequestCredits, TokenBillingMultiplier: info.FrozenChannelTokenBillingMultiplier(), DynamicBillingMultiplierEnabled: info.DynamicBillingMultiplierEnabled}).Normalize()
}

func (s *responsesWSSession) restoreConnectionContext(c *gin.Context, modelName string) *types.NewAPIError {
	reject := func() *types.NewAPIError {
		return types.NewErrorWithStatusCode(errors.New("locked websocket identity or route changed; reconnect required"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	if common.GetContextKeyInt(c, appconstant.ContextKeyUserId) != s.lockedUserID || common.GetContextKeyInt(c, appconstant.ContextKeyTokenId) != s.lockedTokenID {
		return reject()
	}
	channel, err := appmodel.CacheGetChannel(s.lockedChannelID)
	if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled || !channel.GetSetting().ResponsesWebSocketEnabled {
		return reject()
	}
	if service.RouteConfigurationFingerprint(channel) != s.lockedConfig {
		return reject()
	}
	groups := responsesWSTokenGroups(c)
	group, err := appmodel.ResolveEffectiveGroupForChannel(groups, channel.Id)
	if err != nil || group == nil || group.Name != s.lockedGroup || !appmodel.IsChannelEnabledForAnyGroupModel(groups, modelName, channel.Id) {
		return reject()
	}
	if appmodel.ResolveEffectiveBillingProfile(group, channel).Normalize() != s.lockedProfile {
		return reject()
	}
	if bindingValue, found := c.Get("response_route_binding"); found {
		binding := bindingValue.(service.ResponseBinding)
		if binding.ChannelID != s.lockedChannelID || binding.Group != s.lockedGroup || binding.KeyFingerprint != service.RouteCredentialFingerprint(s.lockedKey) || binding.ConfigFingerprint != s.lockedConfig {
			return reject()
		}
	}
	if pin, found := c.Get("specific_channel_id"); found && fmt.Sprint(pin) != fmt.Sprint(s.lockedChannelID) {
		return reject()
	}
	if apiErr := service.AdmitBoundRoute(c, channel, modelName, s.lockedGroup, s.lockedKey); apiErr != nil {
		return apiErr
	}
	common.SetContextKey(c, appconstant.ContextKeyUsingGroup, s.lockedGroup)
	return middleware.SetupContextForSelectedChannel(c, channel, modelName)
}

func buildResponsesWSCreatePayload(c *gin.Context, info *relaycommon.RelayInfo, req dto.OpenAIResponsesRequest, generate json.RawMessage, streamID string) ([]byte, *types.NewAPIError) {
	_, body, closer, apiErr := PrepareResponsesRequest(c, info, &req)
	if apiErr != nil {
		return nil, apiErr
	}
	defer closer.Close()
	jsonData, err := io.ReadAll(body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
	}
	// Reuse the production quantity validator after channel overrides as well.
	validationContext := &gin.Context{}
	var prepared dto.OpenAIResponsesRequest
	if err := common.Unmarshal(jsonData, &prepared); err != nil {
		return nil, newResponsesWSInvalidRequestError(err)
	}
	common.SetContextKey(validationContext, appconstant.ContextKeyOpenAIResponsesRequest, &prepared)
	if _, err := helper.GetAndValidateResponsesRequest(validationContext); err != nil {
		return nil, newResponsesWSInvalidRequestError(err)
	}
	event, err := buildResponsesWSCreateEvent(jsonData, generate, streamID)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	return event, nil
}

func (s *responsesWSSession) startTargetReader(target *websocket.Conn) {
	limit := int64(helper.DefaultMaxScannerBufferSize)
	if appconstant.StreamScannerMaxBufferMB > 0 {
		limit = int64(appconstant.StreamScannerMaxBufferMB) << 20
	}
	target.SetReadLimit(limit)
	s.workers.Go(func() {
		for {
			kind, body, err := target.ReadMessage()
			if target != s.getTarget() {
				return
			}
			active := s.getCurrent()
			if state := active; state != nil {
				select {
				case state.inbox <- responsesWSMessage{kind: kind, body: body, err: err, target: target}:
				case <-state.done:
				case <-s.ctx.Done():
					return
				}
			}
			if err != nil {
				if active == nil {
					s.shutdown()
				}
				return
			}
		}
	})
}

func (s *responsesWSSession) getCurrent() *responsesWSCallState {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.current
}

func (s *responsesWSSession) tryReserveCurrent(state *responsesWSCallState) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.current != nil {
		return false
	}
	s.current = state
	return true
}

func (s *responsesWSSession) getTarget() *websocket.Conn {
	s.connectionMu.Lock()
	defer s.connectionMu.Unlock()
	return s.target
}

func (s *responsesWSSession) setTarget(target *websocket.Conn) bool {
	s.connectionMu.Lock()
	defer s.connectionMu.Unlock()
	if s.ctx.Err() != nil {
		_ = target.Close()
		return false
	}
	s.target = target
	return true
}

func (s *responsesWSSession) writeTarget(kind int, message []byte) error {
	s.targetWriteMu.Lock()
	defer s.targetWriteMu.Unlock()
	target := s.getTarget()
	if target == nil {
		return errors.New("responses websocket upstream is not connected")
	}
	if err := target.SetWriteDeadline(time.Now().Add(responsesWSWriteTimeout)); err != nil {
		return err
	}
	return target.WriteMessage(kind, message)
}

func (s *responsesWSSession) writeClient(kind int, message []byte) error {
	s.clientWriteMu.Lock()
	defer s.clientWriteMu.Unlock()
	if err := s.client.SetWriteDeadline(time.Now().Add(responsesWSWriteTimeout)); err != nil {
		return err
	}
	return s.client.WriteMessage(kind, message)
}

func (s *responsesWSSession) sendError(eventID, streamID string, apiErr *types.NewAPIError) {
	if apiErr == nil {
		return
	}
	payload, err := buildResponsesWSErrorPayload(eventID, streamID, apiErr)
	if err == nil {
		if err := s.writeClient(websocket.TextMessage, payload); err != nil {
			s.shutdown()
		}
	}
}

func (s *responsesWSSession) closeTarget() {
	s.connectionMu.Lock()
	target := s.target
	s.target = nil
	s.connectionMu.Unlock()
	if target != nil {
		_ = target.Close()
	}
}

func (s *responsesWSSession) shutdown() {
	s.cancel()
	s.closeTarget()
	_ = s.client.Close()
}

// Stream identity belongs to the WebSocket envelope. For the legacy wrapped
// input, the top-level field takes precedence over response.stream_id.
func parseResponsesWSEnvelope(message []byte) (responsesWSCreateEvent, string, error) {
	var event responsesWSCreateEvent
	if err := common.Unmarshal(message, &event); err != nil {
		return event, "", fmt.Errorf("invalid websocket event json: %w", err)
	}
	streamRaw := event.StreamID
	if len(streamRaw) == 0 && len(event.Request) > 0 {
		var wrapped struct {
			StreamID json.RawMessage `json:"stream_id"`
		}
		if err := common.Unmarshal(event.Request, &wrapped); err == nil {
			streamRaw = wrapped.StreamID
		}
	}
	var streamID string
	if len(streamRaw) > 0 {
		if err := common.Unmarshal(streamRaw, &streamID); err != nil || len(streamID) < 1 || len(streamID) > 256 {
			return event, "", errors.New("stream_id must contain 1-256 ASCII letters, digits, underscores, hyphens, or periods")
		}
		for _, char := range streamID {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' || char == '.') {
				return event, "", errors.New("stream_id must contain only ASCII letters, digits, underscores, hyphens, or periods")
			}
		}
	}
	if strings.TrimSpace(event.Type) == "" {
		return event, streamID, errors.New("websocket event type is required")
	}
	return event, streamID, nil
}

func newResponsesWSInvalidRequestError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// normalizeResponsesWSCreateEvent builds the HTTP-shaped request body from a
// response.create whose envelope the read loop already parsed and validated.
// The message is decoded once more as a raw map (the body source) and once as
// the typed request; the envelope is not parsed again.
func normalizeResponsesWSCreateEvent(message []byte, event responsesWSCreateEvent, streamID string) (responsesWSCreateRequest, error) {
	create := responsesWSCreateRequest{StreamID: streamID, Generate: event.Generate}
	// Decode the wrapped request on its own so outer fields cannot enter the HTTP body.
	source := message
	if len(event.Request) > 0 {
		source = event.Request
	}
	var raw map[string]json.RawMessage
	if err := common.Unmarshal(source, &raw); err != nil {
		return create, err
	}
	if len(event.Request) > 0 {
		if len(create.Generate) == 0 {
			create.Generate = raw["generate"]
		}
	} else {
		for _, key := range []string{"type", "event_id", "stream", "stream_options"} {
			delete(raw, key)
		}
	}
	delete(raw, "generate")
	delete(raw, "stream_id")
	payload, err := common.Marshal(raw)
	if err != nil {
		return create, err
	}
	if err := common.Unmarshal(payload, &create.Request); err != nil {
		return create, err
	}
	create.Request.Stream = nil
	create.Request.StreamOptions = nil
	create.Body = payload
	return create, nil
}

func buildResponsesWSCreateEvent(jsonData []byte, generate json.RawMessage, streamID string) ([]byte, error) {
	var event map[string]json.RawMessage
	if err := common.Unmarshal(jsonData, &event); err != nil {
		return nil, err
	}
	typeData, err := common.Marshal(responsesWSEventTypeResponseCreate)
	if err != nil {
		return nil, err
	}
	event["type"] = typeData
	// The normalized body never carries stream_id; a channel parameter
	// override may inject one, and it must not shadow the envelope's value.
	delete(event, "stream_id")
	if streamID != "" {
		streamData, err := common.Marshal(streamID)
		if err != nil {
			return nil, err
		}
		event["stream_id"] = streamData
	}
	delete(event, "event_id")
	delete(event, "background")
	delete(event, "stream")
	delete(event, "stream_options")
	if len(generate) > 0 {
		event["generate"] = generate
	}
	return common.Marshal(event)
}

func buildResponsesWSErrorPayload(eventID, streamID string, apiErr *types.NewAPIError) ([]byte, error) {
	if apiErr == nil {
		return nil, errors.New("api error is nil")
	}
	status := apiErr.StatusCode
	if status == 0 {
		status = http.StatusInternalServerError
	}
	openaiErr := apiErr.ToOpenAIError()
	return common.Marshal(&responsesWSErrorEvent{
		Type:     "error",
		Status:   status,
		EventID:  eventID,
		StreamID: streamID,
		Error:    &openaiErr,
	})
}

// checkResponsesWSModelAccess applies the token model limit with the same name
// matching as the HTTP distributor. Unlike HTTP, it also runs for requests
// pinned to a channel: the check is stricter there on purpose, because a
// persistent connection keeps serving the model after the pin was resolved.
func checkResponsesWSModelAccess(c *gin.Context, modelName string) *types.NewAPIError {
	if !common.GetContextKeyBool(c, appconstant.ContextKeyTokenModelLimitEnabled) {
		return nil
	}
	raw, ok := common.GetContextKey(c, appconstant.ContextKeyTokenModelLimit)
	if !ok {
		return types.NewErrorWithStatusCode(errors.New("token has no model access"), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	tokenModelLimit, ok := raw.(map[string]bool)
	if !ok {
		tokenModelLimit = map[string]bool{}
	}
	if _, allowed := tokenModelLimit[ratio_setting.FormatMatchingModelName(modelName)]; !allowed {
		return types.NewErrorWithStatusCode(fmt.Errorf("token is not allowed to use model %s", modelName), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	return nil
}

func selectResponsesWSChannel(c *gin.Context, modelName string, retry *service.RetryParam) (*appmodel.Channel, *types.NewAPIError) {
	var selected *appmodel.Channel
	var group string
	var err error
	if pin, found := c.Get("specific_channel_id"); found {
		var channelID int
		if _, err := fmt.Sscan(fmt.Sprint(pin), &channelID); err != nil {
			return nil, newResponsesWSInvalidRequestError(errors.New("invalid pinned channel"))
		}
		pinned, loadErr := appmodel.CacheGetChannel(channelID)
		if loadErr != nil || pinned == nil {
			return nil, newResponsesWSInvalidRequestError(errors.New("pinned channel not found"))
		}
		selected, group, err = service.SelectRouteForChannel(c, pinned, retry)
	} else {
		selected, group, err = service.CacheGetRandomSatisfiedChannel(retry)
	}
	if err != nil {
		var apiErr *types.NewAPIError
		if errors.As(err, &apiErr) {
			return nil, apiErr
		}
		return nil, types.NewOpenAIError(errors.New("no available Responses WebSocket route"), types.ErrorCodeGetChannelFailed, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	if selected == nil {
		return nil, types.NewOpenAIError(errors.New("no available Responses WebSocket route"), types.ErrorCodeGetChannelFailed, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	common.SetContextKey(c, appconstant.ContextKeyUsingGroup, group)
	if apiErr := middleware.SetupContextForSelectedChannel(c, selected, modelName); apiErr != nil {
		return nil, apiErr
	}
	return selected, nil
}

// Only upstream usage is a billing fact. Text deltas are never locally counted.
func observeResponsesWSUsage(c *gin.Context, info *relaycommon.RelayInfo, event *dto.ResponsesStreamResponse, raw []byte, usage *dto.Usage) *dto.Usage {
	if service.ShouldMonitorGPTAbuse(info) {
		signal := service.ClassifyGPTAbuseSignalFromSSEEventBytes(event.Type, raw)
		if signal.Matched {
			signal.RequestedModel = info.OriginModelName
			signal.UpstreamModel = info.UpstreamModelName
			signal.UpstreamRequestId = c.GetString(common.UpstreamRequestIdKey)
			service.RecordGPTAbuseSignal(c, info, signal)
		}
	}
	switch event.Type {
	case "response.completed", "response.done":
		info.StreamStatus.MarkCompleted()
	case "response.cancelled", "response.canceled":
		info.StreamStatus.MarkCancelled()
	case "response.incomplete":
		reason := gjson.GetBytes(raw, "response.incomplete_details.reason").String()
		switch reason {
		case "max_output_tokens", "content_filter", "safety", "stop", "stop_sequence":
			info.StreamStatus.MarkIncomplete(reason)
		default:
			info.StreamStatus.MarkFailed(reason, "upstream_error", http.StatusBadGateway)
		}
	case "error", "response.error", "response.failed":
		failure := gjson.GetBytes(raw, "response.error")
		if !failure.Exists() || failure.Type == gjson.Null {
			failure = gjson.GetBytes(raw, "error")
		}
		status := int(gjson.GetBytes(raw, "status").Int())
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		info.StreamStatus.MarkFailed(failure.Get("code").String(), failure.Get("type").String(), status)
	}
	if event.Response != nil && event.Response.Usage != nil {
		upstream := event.Response.Usage
		if upstream.InputTokens >= 0 && upstream.OutputTokens >= 0 && upstream.TotalTokens >= 0 {
			copyUsage := *upstream
			copyUsage.PromptTokens = upstream.InputTokens
			copyUsage.CompletionTokens = upstream.OutputTokens
			if upstream.InputTokensDetails != nil {
				copyUsage.PromptTokensDetails = *upstream.InputTokensDetails
			}
			usage = &copyUsage
			info.HasTrustedUsage = true
		}
	}
	if event.Response != nil && event.Response.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", event.Response.GetQuality())
		c.Set("image_generation_call_size", event.Response.GetSize())
	}
	if event.Type == dto.ResponsesOutputTypeItemDone && event.Item != nil && event.Item.Type == dto.BuildInCallWebSearchCall && info.ResponsesUsageInfo != nil {
		if tool := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; tool != nil {
			tool.CallCount++
		}
	}
	info.ApplyDynamicBillingMultiplierFromBody(raw, relaycommon.DynamicBillingMultiplierSourceSSE)
	// Native WS has no HTTP trailers. A completed frame may carry the equivalent
	// upstream billing fact; never infer Pro service from a request marker alone.
	if (event.Type == "response.completed" || event.Type == "response.done") && usage != nil && info.CodexProRequestSent {
		metadata := event.NewAPIBilling
		if metadata == nil && event.Response != nil {
			metadata = event.Response.NewAPIBilling
		}
		if metadata != nil && metadata.CodexProServed {
			info.CodexProServedCandidate = true
			info.ConfirmCodexProServed()
		}
	}
	return usage
}
