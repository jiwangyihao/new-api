package service

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const availabilityContextKey = "availability_observation"

// AvailabilityCapture lives for one request. The directory snapshot is never
// reloaded during retries, so membership changes cannot rewrite its ownership.
type AvailabilityCapture struct {
	observation   model.AvailabilityObservation
	catalog       *model.AvailabilityCatalog
	selectedGroup int
	attempted     bool
	knownModel    bool
	final         bool
	deferred      bool
}

func BeginAvailability(c *gin.Context, modelName string, groups []string) *AvailabilityCapture {
	if model.DB == nil || model.LOG_DB == nil || modelName == "" {
		return nil
	}
	started := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if started.IsZero() {
		started = time.Now()
	}
	capture := &AvailabilityCapture{observation: model.AvailabilityObservation{
		ID: uuid.NewString(), StartedAt: started.Unix(), ModelName: modelName, Outcome: "pending",
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	catalog, err := model.LoadAvailabilityCatalog(ctx)
	if err != nil {
		model.MarkAvailabilityGap(started.Unix(), time.Now().Unix()+1)
		logger.LogError(c, "availability directory unavailable: "+err.Error())
	} else {
		capture.catalog = catalog
		owners := make(map[int]struct{})
		for _, name := range groups {
			for _, channelID := range catalog.NamedGroups[name] {
				if !slices.Contains(catalog.ChannelModels[channelID], modelName) {
					continue
				}
				capture.knownModel = true
				if owner := catalog.ChannelGroups[channelID]; owner > 0 {
					owners[owner] = struct{}{}
				}
			}
		}
		if len(owners) == 1 {
			for owner := range owners {
				capture.observation.GroupID = owner
			}
		}
	}
	c.Set(availabilityContextKey, capture)
	if err := model.RecordAvailability(ctx, capture.observation); err != nil {
		model.MarkAvailabilityGap(started.Unix(), time.Now().Unix()+1)
		logger.LogError(c, "availability request start not recorded: "+err.Error())
	}
	return capture
}

func availabilityCapture(c *gin.Context) *AvailabilityCapture {
	if c == nil {
		return nil
	}
	value, exists := c.Get(availabilityContextKey)
	if !exists {
		return nil
	}
	capture, _ := value.(*AvailabilityCapture)
	return capture
}

// SelectAvailabilityChannel records a selected channel without claiming that an
// upstream attempt occurred. A subsequent selection failure retains the last try.
func SelectAvailabilityChannel(c *gin.Context, channelID int) {
	capture := availabilityCapture(c)
	if capture == nil || capture.catalog == nil {
		return
	}
	capture.selectedGroup = capture.catalog.ChannelGroups[channelID]
	if !capture.attempted {
		capture.observation.GroupID = capture.selectedGroup
	}
}

func AttemptAvailabilityChannel(c *gin.Context, channelID int) {
	capture := availabilityCapture(c)
	if capture == nil {
		return
	}
	capture.attempted = true
	if capture.catalog != nil {
		capture.observation.GroupID = capture.catalog.ChannelGroups[channelID]
	}
}

func ObserveAvailabilityResult(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) {
	capture := availabilityCapture(c)
	if capture == nil {
		return
	}
	var stream *relaycommon.StreamStatus
	isStream := false
	if info != nil {
		stream = info.StreamStatus
		isStream = info.IsStream
		if info.HasSendResponse() {
			ms := info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
			if ms >= 0 {
				capture.observation.FirstResponseMs = &ms
			}
		}
	}
	clientGone := c.Request != nil && c.Request.Context().Err() != nil
	capture.observation.Outcome, capture.observation.Reason = ClassifyAvailabilityResult(apiErr, stream, isStream, clientGone)
	// A successful delivery cannot be inferred from a handler which never tried
	// an upstream. Parsing/dispatch errors are finalized by the distributor.
	if apiErr == nil && !capture.attempted {
		capture.observation.Outcome = model.AvailabilityUnknown
		capture.observation.Reason = "completion_unverified"
	}
	capture.final = true
}

func (capture *AvailabilityCapture) Finish(c *gin.Context) {
	if capture == nil || capture.deferred {
		return
	}
	now := time.Now().Unix()
	if !capture.final {
		capture.observation.Outcome = model.AvailabilityUnknown
		capture.observation.Reason = "completion_unverified"
		status := c.Writer.Status()
		if status >= 500 {
			capture.observation.Outcome = model.AvailabilityFailure
			capture.observation.Reason = "dispatch_error"
			if capture.catalog != nil && !capture.knownModel && !capture.attempted {
				capture.observation.Outcome = model.AvailabilityExcluded
				capture.observation.Reason = "unsupported_model"
			}
		} else if status >= 400 {
			capture.observation.Outcome = model.AvailabilityExcluded
			capture.observation.Reason = "user_condition"
		}
	}
	if capture.catalog == nil || (capture.observation.GroupID == 0 && capture.observation.Outcome != model.AvailabilityExcluded) {
		capture.observation.Outcome = model.AvailabilityUnknown
		capture.observation.Reason = "attribution_unverified"
	}
	capture.observation.CompletedAt = now
	if capture.observation.Outcome != model.AvailabilitySuccess {
		capture.observation.FirstResponseMs = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := model.RecordAvailability(ctx, capture.observation); err != nil {
		model.MarkAvailabilityGap(now, now+1)
		logger.LogError(c, "availability request outcome not recorded: "+err.Error())
	}
}

// DeferTaskAvailability transfers a real request's identity and frozen ownership
// to durable task metadata. Acceptance alone is not successful generation.
func DeferTaskAvailability(c *gin.Context) *model.AvailabilityObservation {
	capture := availabilityCapture(c)
	if capture == nil {
		return nil
	}
	capture.deferred = true
	observation := capture.observation
	observation.Outcome = "pending"
	observation.Reason = model.AvailabilityAsyncPendingReason
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := model.MarkAvailabilityDeferred(ctx, observation.ID); err != nil {
		logger.LogError(c, "availability async marker not recorded: "+err.Error())
	}
	return &observation
}

func AvailabilityTaskInsertFailed(c *gin.Context) {
	capture := availabilityCapture(c)
	if capture == nil {
		return
	}
	capture.deferred = false
	capture.final = true
	capture.observation.Outcome = model.AvailabilityFailure
	capture.observation.Reason = "task_persistence_error"
}

var availabilityMaintenanceOnce sync.Once

func StartAvailabilityMaintenance() {
	availabilityMaintenanceOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				now := time.Now()
				if err := model.RecoverOrphanedAvailability(ctx, now); err != nil {
					logger.LogError(context.Background(), "availability orphan recovery failed: "+err.Error())
				}
				if err := model.MaintainAvailability(ctx, now); err != nil {
					logger.LogError(context.Background(), "availability maintenance failed: "+err.Error())
				}
				if err := model.ReplayTaskAvailability(ctx); err != nil {
					logger.LogError(context.Background(), "availability task replay failed: "+err.Error())
				}
				cancel()
				<-ticker.C
			}
		}()
	})
}

func ObserveAvailabilityTaskError(c *gin.Context, code string, status int, local bool) {
	capture := availabilityCapture(c)
	if capture == nil {
		return
	}
	capture.final = true
	capture.observation.Outcome = model.AvailabilityFailure
	capture.observation.Reason = "task_service_error"
	if local && status >= http.StatusBadRequest && status < 500 && status != http.StatusTooManyRequests {
		capture.observation.Outcome = model.AvailabilityExcluded
		capture.observation.Reason = "user_condition"
	}
	// Internal failures can be surfaced with a 400 by provider adaptors. Do not
	// mistake persistence, transport or setup failure for a user's bad input.
	switch code {
	case "gen_relay_info_failed", "get_channel_failed", "setup_locked_channel_failed", "insert_midjourney_task_failed", "unmarshal_response_body_failed", "copy_response_body_failed":
		capture.observation.Outcome = model.AvailabilityFailure
		capture.observation.Reason = "task_service_error"
	}
}
